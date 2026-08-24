# The WebAssembly demo

[`examples/wasm-demo`](../examples/wasm-demo) is published at <https://cwbudde.github.io/qmc/>.
Everything it shows is computed by this library compiled to `js/wasm` — there is no JavaScript
reimplementation of the sequence, which is the point: the demo is a second consumer of the real
API, and it is the heaviest one.

Two pages: a **Point Lab** (scatter explorer plus digit inspector) and a **Discrepancy Bench**
(correlation heatmap, convergence chart, discrepancy sweep).

## What the demo asks of the library, and why

The demo is where several library decisions were pressure-tested, so its structure is worth
knowing even if you never open it.

**It asks the library rather than re-deriving.** The `leaps` export answers whether a leap is
admissible for the currently selected sequence and dimension count, and which nearby value is.
It decides by building a generator and reading the constructor's error, not by re-deriving
coprimality — the library is the only place that says what a constructor accepts. The
`randomizations` map in `info.go` follows the same rule. `StarDiscrepancy`'s refusal is
rendered verbatim rather than paraphrased.

**It needs introspection the interface does not offer.** What randomization is in effect, how
many dimensions the concrete generator reaches, whether it has prime bases — the demo keeps a
hand-maintained table for all three. That duplication is the concrete argument for
`Describe()`; see [API design](api-design.md).

**Its cost model is measured, not assumed.** Under `js/wasm` CD2's cost per pair is _affine_
in the dimension count, `N(N−1)/2 * (5.7s + 7.5)` ns. A purely proportional model would have
been three times too generous at one dimension. See [Performance](performance.md).

**Sweeps run on a cancellable ladder.** `converge.go` slices a blocking computation so the tab
stays responsive; `runSweep` was extracted so the discrepancy panel reuses it rather than
growing a second copy.

## Known problems

None of these is fixed. They are recorded here because the demo has **no tests at all** and
[no quality gate](toolchain.md#the-demo-module-has-no-quality-gate), so the only thing keeping
them visible is this list.

### Blocking and responsiveness

- The Point Lab runs two synchronous `points` calls per animation frame with no debounce. At
  the limits the UI allows (20 000 points, 64 dimensions) that is 2.5 M radical inverses per
  frame while the pointer is held down. `refreshCorrelation` on the analysis page has the same
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
- `digits.go` hardcodes `skip + 1 + index*leap`, re-deriving the mapping in `fill`, and
  `baseDigits` re-implements the digit loop of `radicalInverse`. Nothing exports that mapping
  and nothing tests the demo, so a convention change leaves the digit inspector showing the
  digits of the wrong index while the values beside it move. Export the mapping from the
  library, or test the demo against it. (The inspector's _label_ has been fixed to show the
  full arithmetic, with the skip recovered from the response rather than from the control — but
  the duplicated mapping underneath it remains.)
- `sinkFor` checks the float view's length but never that the byte view addresses the same
  `ArrayBuffer` or is large enough, so a mismatched pair yields a half-written buffer and a
  plausible partial plot rather than an error.
- `state.geo` is assigned but absent from the state literal, and `cellAt` depends on it.
- `index.html`'s DOM-map comment block is hand-maintained against the markup below it and
  nothing checks the two agree — one caption example had already gone stale.

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

## A lesson worth keeping

Two of the bugs already found here were not crashes. `analysis.js` read a nullable correlation
through `Math.abs(null)` and printed a confident `0.000` — the best possible verdict — for a
measurement that did not exist; and the digit inspector's label described an index it had not
computed. Both looked right on screen. That is the failure mode an untested demo produces, and
it is the argument for the quality gate.
