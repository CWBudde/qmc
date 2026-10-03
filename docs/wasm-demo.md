# The WebAssembly demo

[`examples/wasm-demo`](../examples/wasm-demo) is a separate Go module using a
local replacement for the real library. The Pages workflow builds a static site
for <https://cwbudde.github.io/qmc/>. The **Point Lab** explores scatter projections
and Halton digits; the **Discrepancy Bench** measures correlation, convergence,
and discrepancy. Sequence generation and numerical statistics run in Go/WASM;
JavaScript owns controls, worker coordination, accessibility, and rendering.

## Computation and result ownership

Heavy exports run in worker-hosted Go instances. Point Lab has one computation
channel; the Bench separates correlation from its shared sweep channel. A
replaced active request terminates its worker, and generation IDs reject stale
responses. Stop can terminate a running rung while retaining completed rows.
Idle workers are reused. Typed output buffers transfer to the DOM thread without
detaching already displayed data. Calling a heavy `qmc` export directly from the
console remains synchronous in the calling realm.

Range controls debounce reduced previews and later request the full selected
budget. All four heavy exports have maximum-workload browser regressions,
including a reproducible one-CPU execution profile. Measurements and limitations
are recorded under DEMO-03 in [PLAN.md](../PLAN.md); the
[demo README](../examples/wasm-demo/README.md) describes worker and deadline policy.
Discrepancy ceilings bound computation work rather than universal execution time.

A sweep records its configuration alongside completed results. Changing a sweep
setting cancels and clears that panel, and starting the other sweep supersedes
the current one. Correlation changes remain independent. Gaussian metadata and
integrals use the selected dimensions. Missing/null/non-object options retain
their documented fallbacks. Matched output buffers are validated before reuse.
Recovered requests leave Go usable; actual runtime termination disables compute
controls and offers a reload action.

Both pages use `WasmRuntime.load` in runtime.js for byte-stream progress and the
non-reader/reduced-motion instantiation path. When streaming instantiation is
unavailable, that path reads bytes without replacing a browser global. Runtime
startup remains separate so normal exit, traps, request recovery, and Reload
retain the same terminal-state handling. `just test-browser` first runs the
offline loader/DOM regressions, then tests production pages and a separate Go
runtime fixture. The fixture checks 36 digit-inspector configurations against
independently constructed Halton generators, including all three offered
randomizations, skip/leap, clamping, maximum indices/dimensions, raw expansions,
fixed-permutation tails, and rejected requests. No library internals or new
production exports were added solely for this inspection.

## Accessible inspection

The heatmap has a textual correlation grid with row/column dimension headers,
keyboard cell navigation, and one Tab stop. Every canvas has a current summary;
both sweeps expose named progressbars and full result tables. The Point Lab
reveal slider and digit inspector provide keyboard inspection. Reduced-motion
behavior, visible focus, and throttled final announcements are browser-tested.
The demo README contains keyboard steps and limits of the automated checks.

## Rendering maintenance decision

Retain full hover redraws for now. On 2026-10-03, Chrome 144.0.7559.109 on
Linux/amd64 with an i7-1255U measured 100 redraws after ten warmups at the
48-dimension limit. At a 295.5-pixel square and DPR 1, the heatmap median/p95
was 1.115/2.070 ms and the legend 0.115/0.275 ms. At a 344.34-pixel square and
DPR 2, the heatmap was 0.830/1.020 ms and the legend 0.155/0.195 ms. The shared
browser recipe reports these samples; they measure JavaScript/canvas command
submission in headless Chrome, not end-to-end frame presentation or every device.
Runs were sequential, with no concurrent benchmark workload.

Reproduce with `just test-browser` using the publishing compiler Go 1.26.1
and Node 18.19.1. The fixture renders a 48-dimensional correlation matrix from
64 points with skip 0, plain Halton, seed 1, and leap 1;
`scripts/test-demo-browser.mjs` is the generating harness. It sorts
100 samples and reports element 50 as median and element 95 as p95. These are
finite order statistics, with no confidence interval. Geometry and DPR are
recorded in its output; the second profile uses a 1280×900 emulated viewport.

These modest costs do not presently justify a retained bitmap and its separate
data/size/theme ownership. Revisit caching if a representative slower device
or larger layout demonstrates a material problem. Current redraws always use
current matrix data and canvas geometry; resize and DPR callbacks redraw, and
DPR callbacks call `Render.invalidateTheme`. CSS-variable reads already have
an explicit theme cache invalidator. There is no theme-switching control: any
future theme change must invalidate that cache and redraw all canvases. Browser
regressions check changed theme colours and resized/DPR-scaled backing stores.

## Quality gates

`just check-wasm-demo` explicitly verifies/tidies this module, builds production
js/wasm, vets js/wasm with the runtime fixture, and builds the native stub.
`just lint-wasm-demo` covers its real production/fixture code under the library's
lint rules. Compiling the stub alone does not validate the WASM implementation.

`just test-browser` checks both pages in actual Chrome: configuration ownership,
source/randomization changes, numerical references, buffers, panic/exit recovery,
asset failures, worker cancellation, responsiveness, keyboard input, accessibility
semantics, and network/console/runtime errors. PR and Pages jobs invoke the same
recipes; Pages checks the exact build before upload. Compiler compatibility and
the pinned publishing toolchain are explained in [toolchain](toolchain.md).

The demo uses system fonts and same-origin static downloads. Computation is
local, with no analytics, submissions, or third-party font requests.

## Review decisions

DEMO-01 through DEMO-10, PERF-01, and API-01 have implementation or decision
evidence in [PLAN.md](../PLAN.md). DOC-01/02 reconcile claims and contributor
guidance. Keep future remediation status in that plan, with explanations here
when the change affects demo behavior. Hover bitmap caching remains a measured
deferral rather than an unimplemented requirement.
