# Roadmap and backlog

The package was extracted from a parameter-search tool that needed exactly one thing: a
39-dimensional Halton sequence that actually fills its box at 600 points. Everything below
is wanted but not yet built, and none of it should land without a measurement that shows
it earning its place.

The backlog in the second half came out of a full review of the repository. Items are
ordered by consequence within each section, not by effort.

## Recently closed

Kept here only until the next release, as context for the sections below.

- **Sobol** landed with Joe-Kuo direction numbers for 1024 dimensions, `WithDirectionNumbers`
  for a caller's own table, direct `At` and a Gray-code `Next`. Output verified bit-identical
  against Joe and Kuo's own `sobol.cc` and against scipy. Measured 29.5x better than Monte
  Carlo at 39 dimensions against scrambled Halton's 17.7x.
- **Owen scrambling** (base-2, hash-based, Burley 2020) and **`WithDigitalShift`** randomize
  Sobol; **`WithNestedScrambling`** randomizes Halton with a uniform digit permutation at
  every node of the scramble tree.
- **The `Sequence` interface**, settled before the second generator landed, since adding one
  afterwards to a package with two concrete types would have been a breaking change.
- The suite could not distinguish this library's output from pseudorandom noise. The
  correlation test passes for `math/rand` (0.124, against a 0.25 threshold — better than
  the real generator's 0.141). `integration_test.go` now pins QMC integration error
  against Monte Carlo at 5x; the same substitution scores 0.9x and fails.
- `fill` computed `skip + 1 + i` unchecked. On overflow the wrapped index hit the
  `index < 0` guard in `radicalInverse` and returned the all-zeros origin — the one point
  the package documents it never returns. It now refuses.
- `readInt` in the demo converted a JS double to `int` without bounding it first.
- Four stale correlation figures and two false claims in the README (per-digit-position
  scrambling; scrambled memory at 5000 dimensions, which is ~475 MB, not "a few megabytes").

---

## Library

### API surface

- `Sequence` now exists and both generators satisfy it, asserted at compile time. What is
  still open is everything it deliberately left off: a caller holding a `Sequence` cannot ask
  what randomization is in effect, how many dimensions the concrete generator can reach, or
  whether it has prime bases — the WebAssembly demo needs all three and gets them from a
  hand-maintained table on the side. A small `Describe()` returning a value type would remove
  that duplication without putting Halton-specific methods on the interface.
- `type Option func(*settings)` exports a function type over an unexported struct. This is
  a deliberate and common way to stop third parties writing options, but it renders in
  godoc as `func(*settings)`, which reads as a leak. Either document the intent on the
  type or move to an interface with an unexported method.
- The package panics on arguments that are correctly typed (`fill` on index overflow,
  `scrambledRadicalInverse` on an unreversible index). The reasoning is sound and written
  down, but a library that panics is a hard sell. Revisit whether the stateless entry
  points should return `(point, error)` — and note that if they ever do, `NextInto`'s
  unchecked `h.cursor++` needs its own guard, which today is unreachable only because
  `fill` panics one index earlier.

### Correctness and hygiene

- `radicalInverse` and `scrambledRadicalInverse` still carry `base < 2` guards that are
  unreachable from any in-package call site: bases are always primes. They are cheap, but
  they are also the mechanism that turned the index overflow into silent zeros. Decide
  whether they are a precondition worth keeping or dead weight worth deleting.
- `oneMinusEpsilon` is a package-level mutable `var`. It should be a `const` via
  `math.Float64frombits(0x3FEFFFFFFFFFFFFF)`.
- Add a fuzz target comparing `radicalInverse` and `scrambledRadicalInverse` against the
  slow reference forms already written in `robustness_test.go`. The reference exists; only
  the fuzzing harness is missing.

### Performance

Benchmarks now exist (`bench_test.go`, `sobol_bench_test.go`); nothing has been optimised
against them yet. Note the Halton figures recorded in `bench_test.go` were taken on a
different machine from the Sobol ones, so they are not comparable to each other — the
same-machine numbers are in `sobol_bench_test.go`'s header. Re-measuring all of them
together is worth doing before anyone optimises against them.

- ~~`fill` re-tests `h.perms == nil` on every coordinate of every point.~~ Done: the branch
  is hoisted and the dispatch is now three-way. `Sobol.NextInto` does the same for its Owen
  branch, which is where it matters most — that call is 65 ns/op across 39 dimensions.
- Both inner loops divide by a non-constant base once per digit. A precomputed reciprocal,
  or specialising the base-2 case that dimension 0 always uses, is the obvious first cut.
- There is no bulk API. The WebAssembly demo — the heaviest known consumer — calls `AtInto`
  in a tight loop across tens of thousands of points. An `AtBatch(from, n, dst)` that
  amortises setup per dimension rather than per point is worth measuring.
- Scrambled construction is expensive and undocumented as such: `NewHalton(1000,
WithScrambling(…))` costs ~32 ms and allocates proportionally to the sum of the first
  1000 primes. Callers constructing a generator per task need to know this.

## New sequences and randomisation

### Sobol — done

- [x] Direction numbers, Gray-code `Next`, direct `At`, digital shift, Owen scrambling
- [x] The path above the embedded 1024-dimension ceiling is documented and tested. The
      Joe-Kuo file format and every invariant the validator enforces are written down at
      `WithDirectionNumbers`, along with what it cannot prove — that the numbers came from
      the authors' search. A synthesised 1200-dimension table, whose polynomials are found
      by running `isPrimitiveOverGF2` rather than asserted, drives the loader end to end.
- [x] The 2^m-alignment of the balance property is stated on the type and on `At`, with the
      measurement behind it: at 40 dimensions and m=8, skip 0 leaves all 40 dimensions
      unbalanced and skip 255 leaves none.
- [x] The projection caveat is in the README and the type doc, with a good pair and a bad
      one named. Re-measured and confirmed: 18 of 780 pairs balanced at every split at m=8,
      4 at m=10, and (12,23) leaves 224 of 256 cells empty where (0,1) fills every one.

### Scrambling — done

- [x] Owen scrambling for Sobol, nested scrambling for Halton, both measured
- [x] The affine construction was replaced by a uniform permutation per node. The tail is
      gone — worst adjacent-pair |r| over thirty seeds 0.14 against affine's 0.37, now
      below random-digit's own 0.16 — at the cost of about a sixth of the integration
      advantage (41.1x against 49.9x over forty streams) and about five times the price per
      point. **The cache this plan assumed does not exist and cannot**: a 39-dimensional
      point at 4096 points visits 1,982,974 nodes of which 1,544,674 are distinct, because
      the leading-zero tail hangs a fresh chain below every index and nothing in one is
      revisited. Distinct nodes grow with the point count, not with the digit count, so
      caching would buy 1.28x reuse for 382 MB — and would cost `At` its documented
      freedom from locks. The O(p) shuffle is avoided instead by evaluating only the digit
      asked for, which is exact because a Fisher-Yates run upward settles position i at
      step i and never revisits it.
- [x] The hash-based Owen scramble's approximation is measured. Per node it is
      indistinguishable from fair coins (worst node 3.85 sigma over 40000 seeds and 8191
      nodes, where the largest of 8191 fair coins is expected near 3.9) and pairwise
      independent. Jointly across a level it is not: flip-count variance runs 0.09 to 3.06
      of the binomial value the exact construction reproduces to within 4%. Costs at most a
      tenth of the RMS integration error, so the constants stand — but this is the first
      instrument in the package with the resolution to evaluate changing them.

### Leaping — done

- [x] `WithLeap`, with the shared-factor trap documented and tested. It measured better than
      expected and is now the most accurate option in the package for integration: 54.3x
      Monte Carlo at 39 dimensions and 4096 points over forty admissible leaps, against
      random-digit scrambling's 24.4x and nested scrambling's 41.1x, all four measured in one
      run. It needs no seed, so it is plain QMC rather than RQMC — which is also its
      limitation, since there is no averaging over seeds to give an error estimate.
- [x] The trap is refused at construction rather than documented and allowed, on both
      generators. If a base _p_ divides the leap, that coordinate's leading base-_p_ digit
      never changes and it spends the whole run in one strip of width 1/_p_ — at 39
      dimensions with a leap of 167, dimension 38 covers 0.6% of its range while still
      returning a plausible spread of values inside the strip. Scrambling does not rescue it:
      a permuted constant digit is still constant, so it moves to a different strip of the
      same width. All three Halton randomizations were measured showing this.
- [x] Sobol's version of the trap is **not** the base-2 restatement it looks like, and this is
      the one thing here that had to be measured before it could be written down. Points are
      generated in Gray-code order, so a stride in the raw index is not a stride in the
      direct-form index and no bit is obviously pinned. What is pinned is the parity of the
      population count of `gray(m)`, which is exactly `m&1` — and that parity is the leading
      bit of every dimension whose direction numbers all carry their own leading bit.
      Dimension 1 is the only such dimension in the first eight of the embedded table (32 of
      32 direction numbers, against dimension 0's 1 of 32), and an even leap pins it to one
      half of [0,1) at every skip tried, taking integration from 2.6e-04 to 1.2e-01.
- [x] The demo exposes it. A leap is an independent numeric knob rather than a randomization,
      so it does not go in `examples/wasm-demo/info.go`'s `randomizations` map; it is a
      `readInt` in each of `points.go`, `correlate.go`, `converge.go` and `digits.go`, a sixth
      parameter on `newGenerator`, and a control on all three panels. `maxLeap` is 1000, fixed
      by Sobol's 2^32 raw-index ceiling against the convergence sweep's 200,000 points.

      The interesting part is a new export, `leaps`, which answers whether a leap is
      admissible for the sequence and dimension count now selected, and which nearby value is.
      Without it the control would look broken rather than sparse: at 39 Halton dimensions the
      smallest admissible leap is 173, so almost every number a slider can reach is refused.
      It decides by building a generator and reading the constructor's error rather than by
      re-deriving coprimality, on the same reasoning `info.go` gives for randomizations — the
      library is the only place that says what a constructor accepts. Verified in a browser:
      the heatmap's worst adjacent |r| goes 0.808 to 0.228 at leap 173, and the refusal on
      screen is the library's own sentence, naming the dimension and the base.

Two things worth knowing before anyone reaches for it:

- **The gain is in the average, not the tail.** Worst adjacent-pair |r| over thirty leaps runs
  median 0.097, p90 0.23, worst 0.32, against `WithScrambling`'s 0.093/0.13/0.16. The medians
  agree and the tail does not, because a leap only reorders the digits a coordinate visits, so
  two dimensions whose bases interact with the leap commensurately still ramp together — the
  same shape of defect the affine nested scrambling had. It suits integration; it does not
  suit a parameter sweep.
- **It is not free, and not because of the multiply.** The multiply is unmeasurable. What
  costs is that a leaped generator works on raw indices _n_ times larger, so every radical
  inverse carries about log(_n_) more digits: `AtInto` at 39 dimensions is 634 ns/op at a leap
  of 173 against 512 unleaped. On Sobol a leap also costs `Next` the Gray-code recurrence
  entirely — 301 ns/op against 46.0 — and forfeits the (t,m,s)-net balance property at any
  leap, so an odd leap is legal there and still measures 8.8e-03 against 1.8e-04 unleaped.

### Discrepancy measurement — done

- [x] `StarDiscrepancy(points)` is the exact `D*_N`, a supremum over origin-anchored boxes
      rather than a sample of them. Both halves are enumerated over the same grid — the
      overshoot with the corner counted strictly, the undershoot with it counted inclusively
      — because taking only one returns a lower bound that happens to be right in exactly the
      small cases anyone would hand-check, which is why the test checks it against a
      brute-force enumeration instead. Measured against `math/rand` over ten seeds each,
      scrambled Halton scores 20.40x better at 1 dimension and N=512, 5.47x at 2 dimensions
      and N=512, 3.89x at 3 and N=512, 2.32x at 4 and N=160. The ratio decays with the
      dimension count and improves with the point count, which is the shape the theory
      predicts.
- [x] **The ceiling is a refusal, and it sees N as well as s.** Restricting each dimension's
      candidates to the surviving points' own coordinates is exact and turns the naive
      (N+1)^s grid into C(N+s,s) ≈ N^s/s! leaves, but the problem is NP-hard in the dimension
      (Gnewuch, Srivastav & Winker 2009), so no amount of tuning moves the limit far. A
      dimension ceiling alone would not have been a gate — 5 dimensions and 3000 points is
      inside it and is 2.0e15 leaves — so there is a work budget of 3e7 leaves beside it. The
      budget is calibrated, not asserted: `BenchmarkStarDiscrepancy` walks the two shapes that
      bracket the tree, 1024 points in 2 dimensions (wide and shallow, 5.26e5 leaves, 14.6 ms)
      and 160 points in 4 dimensions (narrow and deep, 2.91e7 leaves, 764 ms). Those are 27.7
      and 26.3 ns per leaf — flat across shapes, which is the only thing that makes a leaf
      count a usable proxy for wall clock — so 3e7 leaves is about 0.8 seconds. That is the
      line: a wait, not a hang, because a caller cannot tell a hang apart from a slow machine.
      Affordable point counts are 7744 at 2 dimensions, 562 at 3, 161 at 4, 78 at 5 and 49 at
      6, and the refusal names them, names which of the two gates tripped, and points at
      `CenteredL2Discrepancy` **with** its caveat attached rather than bare.
- [x] `CenteredL2Discrepancy(points)` is Hickernell's CD2 in closed form, O(N²s) in any
      dimension, returning the square root: 24.5 ms at 39 dimensions and N=1024, 484 ms at
      N=4096. The two easy mistakes in the construction — anchoring the boxes at the cube's
      centre rather than at the nearest corner, and summing over the full-dimensional
      projection rather than all 2^s - 1 of them — both leave a plausible-looking number, so
      the test integrates the definition numerically rather than trusting the formula.
- [x] The saturation is documented at the call site, as this section asked, and it is
      **provable rather than merely observed**. For N i.i.d. uniform points
      `E[CD2²] = ((5/4)^s - (13/12)^s)/N`, which is 2.4198 at 39 dimensions and N=1024 and
      matches the measured random figure of 2.4046 to 0.6%.

This plan's uncommitted "2.38 against 2.40" measures here as **2.366 against 2.405** — same
conclusion, a slightly wider gap — and its claimed 18x integration contrast is **16.4x** at
N=1024 (QMC RMS 8.06e-04 against Monte Carlo's 1.32e-02). Two things worth knowing before
anyone quotes a CD2 number:

- **The statistic becomes its own diagonal.** At 39 dimensions the `i = j` terms of the double
  sum account for 100.4% of the expectation on their own, and the diagonal depends only on
  each coordinate's marginal spread, not at all on how the points sit relative to one another.
  Everything CD2 was meant to measure lives in the residual. That is the mechanism behind the
  2.37-against-2.40 row, and it is why that row is a fact about the statistic and not about
  the point set — the integration error over the very same points differs by 16.4x.
- **It is a decay, not a cliff**, which is why the function returns a number where
  `StarDiscrepancy` returns an error. Measured at N=1024 over ten seeds, the random-to-Halton
  ratio runs 12.45x at 2 dimensions, 6.30x at 5, 2.39x at 10, 1.63x at 15, 1.28x at 20, 1.08x
  at 30 and 1.02x at 39, with the diagonal's share of the expectation going 402%, 196%, 131%,
  113%, 106%, 101%, 100.4% alongside it. Informative below roughly ten dimensions, weak by
  twenty, dead by thirty, and no dimension at which a refusal would be honest. The caller gets
  a self-check instead: compare the number against `sqrt(((5/4)^s - (13/12)^s)/N)`, and if it
  is not several times below that, use an integration test or `StarDiscrepancy` in a
  projection.

- [x] **The demo exposes both, and the Bench earns its name.** It sweeps either statistic
      against n beside a pseudorandom baseline and, for CD2, the analytic
      `sqrt(((5/4)^s - (13/12)^s)/n)` curve: at 39 dimensions all three lie on top of one
      another at a ratio of 1.02x, which is the saturation argument as a picture rather than
      as a caveat, and at 4 dimensions star separates the same two point sets by 1.71x over
      six rungs. Star's availability is asked of the library and its refusal rendered
      verbatim, on the `leaps` precedent, with the largest admissible dimension count offered
      as the fix. The browser needs a second ceiling below the library's, and the obvious
      cost model is wrong: measured under js/wasm the cost per pair is **affine** in the
      dimension count, `N(N-1)/2 * (5.7s + 7.5)` ns, so a purely proportional model would
      have been three times too generous at one dimension. Star at the library's own budget
      freezes the tab for up to 5.6 seconds, so the panel affords 224 points at 3 dimensions
      and 32 at 6. The sweep runs on `converge.go`'s cancellable ladder — which was extracted
      into a shared `runSweep` and the convergence panel re-pointed at it first, so the page
      lost a copy of that logic rather than gaining a second one.

Still open:

- There is no lower-bound or randomized estimator for the star discrepancy above 6 dimensions,
  which is the only way that quantity is reachable at the dimension counts this package is
  aimed at. It would be an approximation with its own error to characterise, and nothing in
  the package needs it yet.

## Testing

- **A stratification test cannot police nesting.** Measured twice, independently, on both
  new scrambling schemes: a scramble that has stopped being conditional on the digits above
  it still maps elementary intervals onto elementary intervals, so it still produces a valid
  net. Removing both bit reversals from `owenScramble` leaves the net-property test passing
  across all 1024 dimensions at m=4, 8 and 12, along with the bijectivity, per-node
  injectivity and `Next`/`At` agreement tests — only the dedicated nesting test fails. Any
  future scrambling scheme needs a test that pins the conditional structure directly.
- **The nesting test has a sensitivity floor.** Following on from the point above: hashing
  the node down to `node & 0xFF`, so roughly one node in 256 shares a permutation with
  another, leaves the nesting test passing. It was caught instead by a chi-square over all
  120 permutations of base 5 and by the golden-value test. A test that detects total loss of
  conditioning does not detect partial loss of it.
- **Five seeds is not enough for the correlation statistic.** A change that was a pure
  re-instantiation of the nested scrambling, not a change of scheme, moved a five-seed worst
  case from 0.40 to 0.12. `correlation_test.go` still uses five and should quote a median and
  a tail over thirty, as the README now does.
- **Ten streams is not enough for the integration statistic either, and the suite still uses
  ten.** Two full-permutation variants differing only in the direction of a Fisher-Yates
  loop — statistically identical constructions — read 44.0x and 31.9x on the same ten
  seeds. The forty- and eighty-stream figures separate the schemes consistently; the
  ten-stream figure does not, and the README table is a ten-stream table. The gates assert
  an ordering and a factor of five rather than any measured constant, which is what keeps
  this from being a flaky-test problem, but no ten-stream number should be quoted as a
  comparison between two good schemes.
- The demo module has no tests at all, and it duplicates library logic (see below), so
  nothing catches the two halves drifting apart.
- Coverage sits at 93.2%, and the uncovered statements are precisely the defensive guards.
  That is the normal shape of coverage, not a target to chase — but the overflow bug lived
  in exactly that region, so the guards deserve tests rather than a higher percentage.

## CI and toolchain

### The format check can pass without checking anything

- `justfile` runs `treefmt --allow-missing-formatter`, which downgrades "formatter binary
  not found" to a warning, and `setup-deps` swallows a prettier install failure with
  `|| echo`. If npm fails on the runner, every Markdown, JSON, YAML, JS, CSS and HTML file
  is skipped and the job still goes green.
- `treefmt.toml` declares `shellcheck` for `*.sh`, but `setup-deps` never installs it, so
  `scripts/build-wasm-demo.sh` has never been shellchecked anywhere. Note also that
  shellcheck never writes, so treefmt's change-detection contract does not apply to it.

### Tool versions disagree with each other

- golangci-lint is pinned three times and three ways: `v2.12.2` in `test.yml`, `@latest` in
  the justfile, `2.13.1` in `.trunk/trunk.yaml`. `just lint` and CI lint therefore differ
  by construction.
- `gofumpt`, `gci`, `shfmt` and `prettier` are all unpinned, so an upstream release breaks
  `check-formatted` with no change to this repository.
- `--config ./.golangci.yml` is passed by the justfile and `release.yml` but not by
  `test.yml`, which relies on discovery. Three call sites, two spellings.
- GitHub Actions are pinned to floating majors (`@v4`, `@v5`, `@v8`). A `dependabot.yml`
  limited to the `github-actions` ecosystem is worth adding; a `gomod` updater is not,
  since there are no dependencies.

### Trunk is dead configuration

`.git/info/exclude` hides `/.trunk`, and no file under it is tracked. It duplicates what
treefmt and golangci-lint already do, and it pins `go@1.21.0` against a module requiring
1.23. Markdown and YAML linting exist _only_ there, which means CI lints neither. Either
track it and drop treefmt, or delete the directory — keeping it untracked and half-wired
is the worst of the three states.

### The demo module has no quality gate

`.golangci.yml` excludes `examples/`, `check-tidy` only tidies the root module, and no job
vets the demo. That is roughly 1500 lines of Go and JavaScript shipping to GitHub Pages
with nothing checking it but the compiler.

### Smaller CI items

- `just ci` calls itself the "full CI pipeline" but omits `test-race`, `check-wasm-demo`
  and the version matrix, and no workflow invokes it, so it can rot undetected.
- `wasm-demo-pages.yml` calls `scripts/build-wasm-demo.sh` directly while local users go
  through the justfile, and the justfile does not forward arguments, so the script's
  `OUT_DIR` parameter is unreachable through `just`. The two paths can drift.
- Pages builds with `go-version-file: go.mod`, so the published demo is compiled by the
  oldest supported toolchain rather than a current one.
- `setup-deps` hardcodes `linux_amd64` (broken on macOS and arm64) and pipes a tarball to
  `sudo tar` with no checksum.
- `.gitignore` misses `*.test`, `*.prof`, `.trunk/`, editor directories and non-root
  `coverage.out`. Nothing improper is committed today.
- An `.editorconfig` is worth adding: the tree holds JS, CSS, HTML, Markdown, YAML and
  shell, and `shfmt -i 2` encodes an indentation rule nothing communicates to an editor.
- Not worth adding for this repository: `CODEOWNERS` (does nothing without branch
  protection), `SECURITY.md` (no dependencies, no network, no untrusted parsing), issue and
  PR templates, `CODE_OF_CONDUCT.md`, `doc.go` (the package comment in `halton.go` already
  does that job). A short `CONTRIBUTING.md` is borderline and worth three lines only
  because the tooling above needs explaining.

### scripts/build-wasm-demo.sh

Quoting, `set -euo pipefail`, the nullglob handling and the GOROOT probe are all correct.
Open items:

- The output directory is never cleaned, only `mkdir -p`'d, so a renamed or deleted asset
  ships to Pages indefinitely.
- `$1` is unvalidated. It cannot delete anything, but `./scripts/build-wasm-demo.sh ~`
  scatters `index.html`, `app.js`, `style.css` and `wasm_exec.js` into that directory,
  overwriting same-named files without confirmation.
- The asset glob is non-recursive and covers no images, icons, fonts or JSON, so a future
  `assets/` subdirectory silently ships nothing.
- No cache-busting. The pages load `app.js` and `qmc.wasm` by bare name, so a returning
  visitor can pair a new script with a cached `.wasm`. A content hash in the filename, or
  a `?v=<sha>` injected at build time, would fix it.
- Do **not** add `.nojekyll`: `upload-pages-artifact` plus `deploy-pages` does not run
  Jekyll, so it would be cargo cult here.

## WebAssembly demo

### Blocking and responsiveness

- The Point Lab runs two synchronous `points` calls per animation frame with no debounce.
  At the limits the UI itself allows (20 000 points, 64 dimensions) that is 2.5 M radical
  inverses per frame while the pointer is held down. `refreshCorrelation` in the analysis
  page has the same shape. `converge.go` writes a careful explanation of why a blocking
  call must be sliced, and then `points` and `correlate` ignore it. Wants a worker, an
  `input` debounce, or a reduced count during drag.
- Even the sliced sweep only checks for cancellation between steps, and the longest step is
  65 536 points across up to 32 dimensions evaluated twice. Stop is unresponsive for the
  whole of it — the hung tab the slicing exists to prevent.
- `tick` re-arms `requestAnimationFrame` unconditionally, so an idle Point Lab tab wakes 60
  times a second for the life of the page. Start it from `setPlaying(true)` and cancel on
  pause.
- After a panic the page sets `state.dead` and every call returns `null` before any status
  update, so controls keep responding visually while doing nothing. The Go side's comment
  says the page "should offer a reload"; it never does.

### Correctness

- `info()` computes `exact` at a hardcoded 32 dimensions and the analysis page renders it
  as "Exact value over the unit cube". Move the dimensions slider to 4 and the page still
  shows the 32-dimensional value (~7e-5 where the truth is ~0.30). The Go comment predicts
  this and says to read `exact` back from `converge()`; the page does that in the readout
  but not in the note.
- `digits.go` hardcodes `skip + 1 + index`, re-deriving the mapping in `fill`, and
  `baseDigits` re-implements the digit loop of `radicalInverse`. Nothing exports that
  mapping and nothing tests the demo, so a convention change leaves the digit inspector
  showing the digits of the wrong index while the values beside it move. Export the
  mapping from the library, or test the demo against it.
- The digit-inspector label reads `65 (index 0 + skip)` where skip is 64 — off by one, and
  it contradicts the Go comment explaining that the panel exists precisely to explain that
  offset.
- A note asserts "the bases are 163 and 167" for every configuration, beside live readouts
  from `Bases()` that show the real values.
- `analysis.js` calls `.toFixed(3)` on a value the Go bridge is explicitly allowed to
  return as `null`, which would throw out of `refreshCorrelation` uncaught.
- `sinkFor` checks the float view's length but never that the byte view addresses the same
  `ArrayBuffer` or is large enough, so a mismatched pair yields a half-written buffer and a
  plausible partial plot rather than an error.
- `state.geo` is assigned but absent from the state literal, and `cellAt` depends on it.

### Frontend quality

- Roughly 150 lines are byte-identical between `app.js` and `analysis.js`, including the
  entire WASM loader and the panic gate. Both pages already share `render.js`; they should
  share a `boot.js`.
- The load-failure message blames a missing `Content-Type: application/wasm`, which is
  irrelevant to the `WebAssembly.instantiate(bytes)` path actually taken. Users hitting the
  common failure get a red herring.
- Neither page has a `<noscript>`, and the sliders and selects ship enabled before boot
  while only the buttons ship disabled — so before or after a failed load a user can drag
  controls that do nothing, with no feedback.
- Accessibility: canvases carry static `aria-label`s that never reflect the data drawn, the
  progress bar is a bare `<div>` with no `role="progressbar"`, the heatmap legend is
  `aria-hidden` with no text equivalent, and the heatmap is hover-only with no keyboard
  path to any cell.
- The heatmap re-renders in full on every `mousemove` that changes cell, including a
  per-pixel legend redraw, instead of using an overlay canvas. Both pages also allocate two
  typed-array views per result per frame — the churn the sink machinery exists to avoid,
  moved to the JavaScript side.
- `render.js` caches CSS custom properties and invalidates only on DPR change. Harmless
  today because the stylesheet has no `prefers-color-scheme` block, and a trap for whoever
  adds one.
- The demo README describes the Point Lab as showing "a scrambled Halton sequence", which
  contradicts both its own later text and the code: scrambling starts off on purpose.

## Downstream

- [x] `mayfly.WithQMCInitialPopulation` landed against v0.2.0 and was measured rather than
      asserted: Standard MA, 30 runs, 500 iterations, over mayfly's 16-problem benchmark
      suite. Owen-scrambled Sobol is significantly better on two problems (Rastrigin at 30
      dimensions, mean 14.09→9.66 at p<0.001; Ackley at 10 dimensions,
      0.851→0.303 at p=0.007) and significantly worse on none; nested-scrambled Halton
      reaches p<0.05 nowhere. Two hits in thirty-two tests is close to what 0.05 produces
      by chance, so the honest reading is a mildly favorable direction with a large effect
      in two places, and mayfly kept uniform draws as its default. Four of the sixteen
      problems are solved to machine precision by every strategy and say nothing at all —
      worth knowing before anyone measures this again. Write-up in
      `mayfly/docs/qmc-initialization.md`.
- [ ] The population is 40 points in up to 30 dimensions, which is the regime where this
      package has never been measured: `integration_test.go` works at n=4096. A sample that
      small is mostly the first few strata, and whether the scrambling constants are right
      there is an open question the mayfly numbers hint at but cannot answer.
