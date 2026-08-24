# Backlog

The package was extracted from a parameter-search tool that needed exactly one thing: a
39-dimensional Halton sequence that actually fills its box at 600 points. Everything below is
wanted but not yet built, and none of it should land without a measurement that shows it
earning its place.

Items are ordered by consequence within each section, not by effort. The reasoning and the
measurements behind everything already built live in [`docs/`](docs/README.md) — this file is
only what is still open.

## Library

### API surface

- **`Describe()` on `Sequence`.** A caller holding a `Sequence` cannot ask what randomization
  is in effect, how many dimensions the concrete generator can reach, or whether it has prime
  bases. The WebAssembly demo needs all three and gets them from a hand-maintained table on
  the side. A small `Describe()` returning a value type would remove that duplication without
  putting Halton-specific methods on the interface.
- **`type Option func(*settings)` renders as a leak.** Exporting a function type over an
  unexported struct is a deliberate and common way to stop third parties writing options, but
  godoc shows `func(*settings)`. Either document the intent on the type or move to an
  interface with an unexported method.
- **Panics on correctly-typed arguments** (`fill` on index overflow,
  `scrambledRadicalInverse` on an unreversible index). The reasoning is sound and written
  down, but a library that panics is a hard sell. Revisit whether the stateless entry points
  should return `(point, error)` — and note that if they ever do, `NextInto`'s unchecked
  `h.cursor++` needs its own guard, which today is unreachable only because `fill` panics one
  index earlier.

### Correctness and hygiene

- `radicalInverse` and `scrambledRadicalInverse` still carry `base < 2` guards unreachable
  from any in-package call site: bases are always primes. They are cheap, but they are also
  the mechanism that turned an index overflow into silent zeros. Decide whether they are a
  precondition worth keeping or dead weight worth deleting.

### Performance

See [`docs/performance.md`](docs/performance.md) for the current figures and their caveats —
notably that the Halton and Sobol benchmarks were taken on different machines and are not
comparable to each other. Re-measure them together before optimising against them.

- Both inner loops divide by a non-constant base once per digit. A precomputed reciprocal, or
  specialising the base-2 case that dimension 0 always uses, is the obvious first cut.
- **No bulk API.** The WebAssembly demo — the heaviest known consumer — calls `AtInto` in a
  tight loop across tens of thousands of points. An `AtBatch(from, n, dst)` amortising setup
  per dimension rather than per point is worth measuring.
- Scrambled construction is expensive and undocumented as such at the call site:
  `NewHalton(1000, WithScrambling(…))` costs ~32 ms. Callers constructing a generator per task
  need to know.

## Discrepancy

- **No estimator above 6 dimensions.** There is no lower-bound or randomized estimator for the
  star discrepancy above the exact algorithm's ceiling, which is the only way that quantity is
  reachable at the dimension counts this package is aimed at. It would be an approximation
  with its own error to characterise, and nothing in the package needs it yet. Background:
  [`docs/discrepancy.md`](docs/discrepancy.md).

## Testing

Full reasoning in [`docs/testing-methodology.md`](docs/testing-methodology.md).

- **`correlation_test.go` still uses five seeds.** Five is demonstrably not enough — a pure
  re-instantiation moved a five-seed worst case from 0.40 to 0.12. It should quote a median
  and a tail over thirty, as the docs do.
- **The integration tests still use ten streams.** Two statistically identical constructions
  read 44.0x and 31.9x on the same ten seeds. The gates assert an ordering rather than a
  constant, so this is not a flaky-test problem — but no ten-stream number should be quoted as
  a comparison between two good schemes, and the suite should stop producing them.
- **Any new scrambling scheme needs a test that pins conditional structure directly.** A
  stratification test cannot police nesting, and the existing nesting test has a sensitivity
  floor: it catches total loss of conditioning, not partial.
- **The demo module has no tests at all**, and it duplicates library logic, so nothing catches
  the two halves drifting apart.
- The uncovered statements are precisely the defensive guards. That is the normal shape of
  coverage, not a target to chase — but the index-overflow bug lived in exactly that region,
  so the guards deserve tests rather than a higher percentage.

## CI and toolchain

### The format check can pass without checking anything

- `justfile` runs `treefmt --allow-missing-formatter`, which downgrades "formatter binary not
  found" to a warning, and `setup-deps` swallows a prettier install failure with `|| echo`. If
  npm fails on the runner, every Markdown, JSON, YAML, JS, CSS and HTML file is skipped and the
  job still goes green.
- `treefmt.toml` declares `shellcheck` for `*.sh`, but `setup-deps` never installs it, so
  `scripts/build-wasm-demo.sh` has never been shellchecked anywhere. Note that shellcheck never
  writes, so treefmt's change-detection contract does not apply to it.

### Unpinned tools

- `gofumpt`, `gci`, `shfmt` and `prettier` are all unpinned, so an upstream release breaks
  `check-formatted` with no change to this repository.

### Trunk is dead configuration

`.git/info/exclude` hides `/.trunk`, and no file under it is tracked. It duplicates what
treefmt and golangci-lint already do, and it pins `go@1.21.0` against a module requiring 1.23.
Markdown and YAML linting exist _only_ there, which means CI lints neither. Either track it and
drop treefmt, or delete the directory — keeping it untracked and half-wired is the worst of the
three states.

### The demo module has no quality gate

`.golangci.yml` excludes `examples/`, `check-tidy` only tidies the root module, and no job vets
the demo. That is roughly 1500 lines of Go and JavaScript shipping to GitHub Pages with nothing
checking it but the compiler.

### Smaller CI items

- `just ci` calls itself the "full CI pipeline" but omits `test-race`, `check-wasm-demo` and
  the version matrix, and no workflow invokes it, so it can rot undetected.
- `wasm-demo-pages.yml` calls `scripts/build-wasm-demo.sh` directly while local users go
  through the justfile, and the justfile does not forward arguments, so the script's `OUT_DIR`
  parameter is unreachable through `just`. The two paths can drift.
- Pages builds with `go-version-file: go.mod`, so the published demo is compiled by the oldest
  supported toolchain rather than a current one.
- `setup-deps` hardcodes `linux_amd64` (broken on macOS and arm64) and pipes a tarball to
  `sudo tar` with no checksum.
- Not worth adding for this repository: `CODEOWNERS` (does nothing without branch protection),
  `SECURITY.md` (no dependencies, no network, no untrusted parsing), issue and PR templates,
  `CODE_OF_CONDUCT.md`, `doc.go` (the package comment in `halton.go` already does that job). A
  short `CONTRIBUTING.md` is borderline and worth three lines only because the tooling above
  needs explaining.

### scripts/build-wasm-demo.sh

Quoting, `set -euo pipefail`, the nullglob handling and the GOROOT probe are all correct. Open:

- The output directory is never cleaned, only `mkdir -p`'d, so a renamed or deleted asset ships
  to Pages indefinitely.
- `$1` is unvalidated. It cannot delete anything, but `./scripts/build-wasm-demo.sh ~` scatters
  `index.html`, `app.js`, `style.css` and `wasm_exec.js` into that directory, overwriting
  same-named files without confirmation.
- The asset glob is non-recursive and covers no images, icons, fonts or JSON, so a future
  `assets/` subdirectory silently ships nothing.
- No cache-busting. The pages load `app.js` and `qmc.wasm` by bare name, so a returning visitor
  can pair a new script with a cached `.wasm`. A content hash in the filename, or a `?v=<sha>`
  injected at build time, would fix it.
- Do **not** add `.nojekyll`: `upload-pages-artifact` plus `deploy-pages` does not run Jekyll,
  so it would be cargo cult here.

## WebAssembly demo

### Blocking and responsiveness

- The Point Lab runs two synchronous `points` calls per animation frame with no debounce. At
  the limits the UI allows (20 000 points, 64 dimensions) that is 2.5 M radical inverses per
  frame while the pointer is held down. `refreshCorrelation` in the analysis page has the same
  shape. `converge.go` writes a careful explanation of why a blocking call must be sliced, and
  then `points` and `correlate` ignore it. Wants a worker, an `input` debounce, or a reduced
  count during drag.
- Even the sliced sweep only checks for cancellation between steps, and the longest step is
  65 536 points across up to 32 dimensions evaluated twice. Stop is unresponsive for the whole
  of it — the hung tab the slicing exists to prevent.
- `tick` re-arms `requestAnimationFrame` unconditionally, so an idle Point Lab tab wakes 60
  times a second for the life of the page. Start it from `setPlaying(true)` and cancel on
  pause.
- After a panic the page sets `state.dead` and every call returns `null` before any status
  update, so controls keep responding visually while doing nothing. The Go side's comment says
  the page "should offer a reload"; it never does.

### Correctness

- `info()` computes `exact` at a hardcoded 32 dimensions and the analysis page renders it as
  "Exact value over the unit cube". Move the dimensions slider to 4 and the page still shows
  the 32-dimensional value (~7e-5 where the truth is ~0.30). The Go comment predicts this and
  says to read `exact` back from `converge()`; the page does that in the readout but not in the
  note.
- `digits.go` hardcodes `skip + 1 + index`, re-deriving the mapping in `fill`, and `baseDigits`
  re-implements the digit loop of `radicalInverse`. Nothing exports that mapping and nothing
  tests the demo, so a convention change leaves the digit inspector showing the digits of the
  wrong index while the values beside it move. Export the mapping from the library, or test the
  demo against it.
- `index.html`'s DOM-map comment block is hand-maintained against the markup below it and
  nothing checks the two agree — one caption example had already gone stale.
- `sinkFor` checks the float view's length but never that the byte view addresses the same
  `ArrayBuffer` or is large enough, so a mismatched pair yields a half-written buffer and a
  plausible partial plot rather than an error.
- `state.geo` is assigned but absent from the state literal, and `cellAt` depends on it.

### Frontend quality

- Roughly 150 lines are byte-identical between `app.js` and `analysis.js`, including the entire
  WASM loader and the panic gate. Both pages already share `render.js`; they should share a
  `boot.js`.
- The load-failure message blames a missing `Content-Type: application/wasm`, which is
  irrelevant to the `WebAssembly.instantiate(bytes)` path actually taken. Users hitting the
  common failure get a red herring.
- Neither page has a `<noscript>`, and the sliders and selects ship enabled before boot while
  only the buttons ship disabled — so before or after a failed load a user can drag controls
  that do nothing, with no feedback.
- Accessibility: canvases carry static `aria-label`s that never reflect the data drawn, the
  progress bar is a bare `<div>` with no `role="progressbar"`, the heatmap legend is
  `aria-hidden` with no text equivalent, and the heatmap is hover-only with no keyboard path to
  any cell.
- The heatmap re-renders in full on every `mousemove` that changes cell, including a per-pixel
  legend redraw, instead of using an overlay canvas. Both pages also allocate two typed-array
  views per result per frame — the churn the sink machinery exists to avoid, moved to the
  JavaScript side.
- `render.js` caches CSS custom properties and invalidates only on DPR change. Harmless today
  because the stylesheet has no `prefers-color-scheme` block, and a trap for whoever adds one.

## Downstream

- `mayfly.WithQMCInitialPopulation` landed against v0.2.0 and is measured rather than asserted;
  write-up in `mayfly/docs/qmc-initialization.md`. Owen-scrambled Sobol is significantly better
  on two of sixteen problems and worse on none; nested-scrambled Halton reaches p<0.05 nowhere.
  Two hits in thirty-two tests is close to what 0.05 produces by chance, so mayfly kept uniform
  draws as its default. Whether that was the regime's fault or the scheme's is now answered —
  the regime's, mostly: at 40 points and 30 dimensions every randomization still beats Monte
  Carlo (3.4x to 5.2x), but the first-place margin the n=4096 table shows collapses to a tie.
  Measurements in [`docs/small-sample-regime.md`](docs/small-sample-regime.md).
