# Roadmap and backlog

The package was extracted from a parameter-search tool that needed exactly one thing: a
39-dimensional Halton sequence that actually fills its box at 600 points. Everything below
is wanted but not yet built, and none of it should land without a measurement that shows
it earning its place.

The backlog in the second half came out of a full review of the repository. Items are
ordered by consequence within each section, not by effort.

## Recently closed

Kept here only until the next release, as context for the sections below.

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

- The package is called `qmc` and its doc comment promises "sequences", plural, but ships
  one and exposes no interface. `NewSobol` below is described as returning "the same
  generator surface" — that surface does not exist as a type, so nothing enforces it.
  Decide the shape **before** Sobol lands, because adding an interface afterwards to a
  package with two concrete types is a breaking change.
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

Benchmarks now exist (`bench_test.go`); nothing has been optimised against them yet.
Measured at 39 dimensions: `AtInto` 800 ns/op, scrambled 1279 ns/op, `Next` 1074 ns/op
with one 320 B allocation.

- `fill` re-tests `h.perms == nil` on every coordinate of every point. Hoist the branch out
  of the loop, or split into two loops.
- Both inner loops divide by a non-constant base once per digit. A precomputed reciprocal,
  or specialising the base-2 case that dimension 0 always uses, is the obvious first cut.
- There is no bulk API. The WebAssembly demo — the heaviest known consumer — calls `AtInto`
  in a tight loop across tens of thousands of points. An `AtBatch(from, n, dst)` that
  amortises setup per dimension rather than per point is worth measuring.
- Scrambled construction is expensive and undocumented as such: `NewHalton(1000,
  WithScrambling(…))` costs ~32 ms and allocates proportionally to the sum of the first
  1000 primes. Callers constructing a generator per task need to know this.

## New sequences and randomisation

### Sobol

Sobol sequences have better high-dimensional behaviour than Halton and do not degrade with
dimension the way large-prime radical inverses do. Needs direction numbers — Joe & Kuo's
tables are the usual source, and they are large enough that generating or embedding them
is its own decision.

- [ ] Direction numbers (embed Joe & Kuo, or generate from primitive polynomials)
- [ ] Gray-code recurrence for `Next`, direct evaluation for `At`
- [ ] Digital-shift randomization, then Owen scrambling
- [ ] Settle the shared generator interface first (see API surface above)

### Owen scrambling

Nested/Owen scrambling is stronger than the random-digit scrambling implemented here: it
permutes each digit conditionally on the digits above it, which removes correlations that
one permutation per dimension leaves behind. It costs a hash per digit rather than a table
lookup.

- [ ] Owen scrambling as an option alongside `WithScrambling`
- [ ] Measure it against random-digit scrambling on both the correlation test and the new
      integration test before recommending it — the cheaper scrambling already gets the
      worst pair from 0.81 to 0.14 and integrates 18x better than Monte Carlo at 39
      dimensions, and the remaining headroom may not be worth the per-digit hash

### Leaping

Skipping every _L_-th point is sometimes used to decorrelate coordinates in place of
scrambling. It is easy to add (`WithLeap(n)`, one multiply in `fill`) and easy to get
wrong: a leap that shares a factor with a base makes that coordinate worse, not better.

- [ ] `WithLeap`, with the shared-factor trap documented and tested

### Discrepancy measurement

Star discrepancy is the textbook quantity. Note that the centred L2 discrepancy saturates
badly in high dimensions — measured at 39 dimensions and 1024 points it scores scrambled
Halton at 2.38 against 2.40 for pure random, which says nothing useful, while integration
error over the same points differs by 18x. Any discrepancy API should say where it stops
being informative.

- [ ] `Discrepancy(points)` for low dimensions (exact star discrepancy is expensive above
      a handful of dimensions)
- [ ] An `L2` centred discrepancy, which is cheap in any dimension, with its
      high-dimensional saturation documented at the call site

## Testing

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
1.23. Markdown and YAML linting exist *only* there, which means CI lints neither. Either
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

- [ ] `mayfly.WithQMCInitialPopulation` — metaheuristics initialize their populations with
      uniform random draws (`mayfly/helpers.go`), and a low-discrepancy initial population
      is a well-documented cheap improvement. Wants measuring on mayfly's own benchmark
      suite, not asserting.
