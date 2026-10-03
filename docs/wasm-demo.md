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

## Accessible inspection

The heatmap has a textual correlation grid with row/column dimension headers,
keyboard cell navigation, and one Tab stop. Every canvas has a current summary;
both sweeps expose named progressbars and full result tables. The Point Lab
reveal slider and digit inspector provide keyboard inspection. Reduced-motion
behavior, visible focus, and throttled final announcements are browser-tested.
The demo README contains keyboard steps and limits of the automated checks.

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

## Remaining work

All DEMO-01 through DEMO-09 findings have completion evidence in
[PLAN.md](../PLAN.md). Workflow/release hardening remains TOOL-04, consistent
artifacts and distribution notices remain SHIP-01/SHIP-02, and measurement/API
proposals remain DOC-01/PERF-01/API-01. Use the task records and current demo
README for implementation status.
