# The qmc WebAssembly demo

Two pages that run [github.com/cwbudde/qmc](https://github.com/CWBudde/qmc)
compiled to `js/wasm`:

- **`index.html` — Point Lab.** A quasi-random sequence — Halton or Sobol,
  with or without a randomization — and a pseudo-random one drawn side by side
  on the same two axes, at the same count, scrubbable in sequence order. Below
  them a digit inspector: the base-_p_ expansion of an index mirrored around
  the radix point, the permutation that rewrites each digit when random-digit
  scrambling is on, and the resulting coordinate next to the unscrambled one.
  The inspector is Halton's alone and disappears when Sobol is selected.
- **`analysis.html` — Discrepancy Bench.** A correlation heatmap over every
  pair of dimensions, recomputed live as the sequence or its randomization
  changes; a log–log convergence chart of absolute integration error against
  _N_ — quasi-Monte Carlo against pseudo-random Monte Carlo, with reference
  slopes for 1/_N_ and 1/√*N*; and, finally making the page's name true, a
  discrepancy sweep: exact star discrepancy or Hickernell's centred L2, the
  sequence against a pseudo-random set of the same size, against the analytic
  random RMS baseline, `sqrt(E[CD2²])`, for centred L2.

The organising rule is that **no QMC logic lives in JavaScript**. Every point,
every prime base, every correlation and every integration error comes out of the
Go library. The JavaScript owns the DOM, the canvas and the clock. A demo that
reimplemented the radical inverse in JS would be demonstrating the JS.

## The default view

The Point Lab opens on Halton, 39 dimensions, axes 37 against 38, randomization
**none**, with skip 64 and 600 points. These axes use prime bases 163 and 167.
They have completed several leading-digit cycles at this budget; their slower
higher digits remain poorly explored. The selected projection illustrates the
pattern, while the Bench separately searches all adjacent pairs for the worst
correlation. Compare a randomization at the same budget and across several seeds.
Sobol uses base 2 throughout and does not have these high-prime ramps, although
its projection quality still depends on the direction table and sampled block.

## Sequences and randomizations

The sequence menu and the randomization menu are both built from `info()`, and
the second is rebuilt whenever the first changes, because the two do not
overlap:

| Sequence | Randomizations                                   |
| -------- | ------------------------------------------------ |
| Halton   | none, random-digit scrambling, nested scrambling |
| Sobol    | none, digital shift, Owen scrambling             |

The library's constructors refuse an option that does not apply to the
generator being built, naming it, and this page does not duplicate that rule —
it only offers each sequence the menu `info()` reports for it, and falls back to
the unrandomized entry when a selection does not survive a change of sequence.
Menu descriptions summarize the option contracts and name their limitations.
The unrandomized entries describe each source separately. Accuracy and cost
comparisons belong to their measured configuration, not to a menu's promise.
See [testing methodology](../../docs/testing-methodology.md) and
[performance](../../docs/performance.md) for reproduction commands and limitations.

Two things the page hides rather than guesses. Sobol has no prime bases — it is
base 2 everywhere — so the base readouts blank out instead of reporting a number
that would look like an explanation of the picture. And it has no digit alphabet
to permute, so the digit inspector is removed rather than left showing Halton's
last values beside Sobol data.

## Leaping

A leap takes every _L_-th point instead of every point: point _i_ becomes raw
index `skip + 1 + i*L`. It is the one remedy for the Halton defect that needs no
seed, and both pages expose it as a plain number — the Point Lab beside the
burn-in, the Bench once per panel, so the heatmap and the sweep can be leaped
independently.

Its legal values also depend on the other
controls. _L_ must share no factor with any base in use, and if it does, that
coordinate's leading digit never changes and it spends the whole run inside one
strip of width 1/base — scrambling does not rescue it, because a permuted
constant is still constant. The library refuses that at construction, by name.
At 39 Halton dimensions the smallest admissible leap is therefore **173**, and
Sobol is base 2 in every dimension so it refuses every even leap.

The `leaps` export reports whether the
current number is admissible for the sequence and dimension count now selected,
which nearby values are, and — when it is not — the constructor's own refusal,
naming the dimension and the base. The page renders that sentence verbatim under
the control and does not ask for points, so an illegal leap reads as a sparse
control rather than as a broken one. Admissibility is decided by **building a
generator and reading the error**, not by re-deriving coprimality in the demo:
the library is the only place that says what a constructor accepts, and a second
copy here is the copy that would go stale.

The Bench reports the configuration of its current correlation result. A changed
leap or burn-in is a different experiment. Historical five-seed coefficients are
no longer used as UI baselines; current library correlation regressions summarize
thirty seeds at their documented fixed configuration.

## Discrepancy

Press Start to compare the sequence's discrepancy with one seeded pseudorandom
set at each sample budget. Centred L2 also plots an analytic random RMS baseline,
assuming independent continuous uniform points:

    E[CD2²] = ((5/4)^s - (13/12)^s)/N
    RMS(CD2) = sqrt(E[CD2²])

This is generally different from the mean `E[CD2]`. The seeded finite-precision
comparison is one realization, so its score need not lie on this baseline.
The `analytic` response field retains its existing value; `analyticKind: "rms"`
and `analyticLabel` identify the reference in `info`, `metrics` and discrepancy
results. Star has `analyticKind: "none"` and no reference value.

The default is 39 dimensions and centred L2. At high dimensions CD2 can
distinguish useful sequences from random points only weakly. Its squared random
expectation is dominated by `(5/4)^s` as dimensions grow, which does not fix the
ratio for any particular source, seed or point count. Try smaller dimensions,
several seeds and another available metric. No fixed improvement is promised.

The headline **random ÷ sequence** compares these two measured scores. The
panel's 1.5× display threshold is a UI policy, not a significance test or an
integration-error guarantee. A computed zero can reflect floating-point
cancellation or a numerical floor; it does not prove a perfect point set.

**Star's library acceptance is probed, not restated.** Generic multi-point
star computation has dimension and work limits; the library also has cheap
one-point and one-dimensional paths. This demo offers at least two points.
`metrics` probes that minimum, prints the library's refusal when unavailable,
and offers lower dimensions it accepts. Start is disabled when unavailable;
the menu retains all metrics. The demo's additional point caps are a separate policy.

**The N ceiling moves with the dimension slider.** General centred L2 costs
O(*N*²*s*), with a cheaper one-dimensional path. The demo retains conservative
work ceilings derived from its earlier browser policy: 1142 centred-L2 points
at 39 dimensions, and star discrepancy from 1792 points at two dimensions to
32 at six. These are total-work policies, not library limitations or duration
guarantees. Each rung now runs in a worker and Stop can terminate it in progress.

Expect a **short ladder** for star. At four dimensions it is six rungs; at six
it is three. Runtime depends on the point set and device.

## Build and run

```bash
just run-wasm-demo                            # build into ./dist and serve on :8090
just build-wasm-demo                          # build only
just build-wasm-demo /tmp/somewhere           # build somewhere else
just check-demo-artifact /tmp/somewhere      # verify that exact build
just test-demo-artifact                      # exercise publication/safety failures
```

**An HTTP server is required.** Pages and workers fetch `qmc.wasm`; local
`file://` URLs are not a supported serving path. Send the module as
`Content-Type: application/wasm` for streaming instantiation. The shared loader
usually reads bytes for progress, which does not require that MIME type; its
streaming path does. Serve JavaScript and CSS with their normal MIME types too.

The build stages and validates a complete site before publishing it. Stable
`index.html`, `analysis.html`, and `credits.html` entry pages select one immutable
`build-<sha256>/` directory through a relative HTML base URL. That directory
contains the scripts, styles, worker, WASM and runtime, including recursive
static files from `assets/`. A change to any payload file or compiler version
changes the build ID. Page navigation stays at the stable public URLs. The
same output works at `http://localhost:8090/` and a `/qmc/` project path;
the browser regression exercises the latter with `QMC_BROWSER_PATH=/qmc/`.

An old open/cached page keeps requesting its original namespace. If deployment
removed that bundle and the browser lacks its cached resources, requests fail
and Reload obtains the current complete build. An old resource URL cannot
silently resolve to new-build bytes. Configure hosts to revalidate entry HTML
and cache `build-<sha256>/` files as immutable when cache headers are available.

Output must be new, empty, or an intact prior build with `build-manifest.json`.
The manifest lists every file and its content hash. Caller additions, edits,
symlinks and source/private destinations are rejected and preserved. Replacing
a prior build removes obsolete assets through an atomic directory exchange on
supported Linux/macOS filesystems. If exchange is unavailable, the build fails
and preserves the previous output; choose a fresh directory instead. An older
unmanifested `dist` needs to be moved aside before building. See
[the artifact policy](../../docs/toolchain.md#demo-artifact-publication) for the
asset allowlist, locking, and caller-edit handling.

`wasm_exec.js` is copied from the build compiler's toolchain and is never
committed. It must match the compiler that produced the `.wasm`.
The build verifies the compiler identity before and after compilation, and the
artifact's hash binds both runtime bytes and the compiler version.

Both interactive pages link **Credits and licenses**. The built credits page
links the complete project MIT license, Joe–Kuo copyright/conditions/disclaimer,
and the build compiler's Go license and additional patent grant. These files
live in the same immutable bundle under `notices/`. The artifact checker
requires all four nonempty notices and working credits links; the browser test
navigates from both pages and verifies every downloaded notice's content hash.
The builder can safely replace an intact older managed artifact lacking these
notices, while the current distribution gate rejects that older artifact.

No fonts are bundled. When adding other third-party assets, include their
applicable complete notices under `assets/notices/` and link them from the
credits page alongside these existing materials.

## Browser regression checks

The demo uses system fonts and requests only its own static assets and WASM
when it loads. Sequence generation and analysis run locally; there are no
analytics, data submissions, or third-party font requests. Following a source
or documentation link explicitly navigates away. The Go library itself has no
network activity or runtime dependencies; serving/downloading the demo is a
separate browser activity.

Run `just test-browser` from the repository root. It builds a production demo
and the test-only runtime fixture in an owned temporary directory, serves them
on an ephemeral local port, starts Chrome with a private profile, and cleans up
on success or failure. To check an existing build, use `just test-browser dist`.
Node 18 or newer and Chrome on PATH are required (`CHROME_BIN` can select another
Chrome executable). Server startup, page readiness, protocol requests, and the
overall browser run have 5/30/20/120-second deadlines respectively (the batched
responsiveness measurement has a 90-second protocol deadline). Compilation
is also bounded by the CI job's timeout.

The Point Lab schedules reveal-animation frames only during playback. Pause
cancels the pending frame; hiding the tab pauses playback without advancing
through background time. Under reduced motion, Play reveals all points
immediately. Scrubbing and other control changes redraw on demand.

The regression covers both pages and source/randomization switching, control
changes, panel switching,
Stop/restart, unavailable metrics, result configuration snapshots, source-specific
descriptions, and Gaussian metadata/notes/readouts at dimensions 1, 4, and 32
against an independent numerical integral. It also checks typed buffers, option
fallbacks, recovered request failures, actual Go exit/reload, and missing/corrupt
asset recovery. Unexpected console, runtime, resource, and third-party network
errors fail the suite. PR and Pages artifact validation use this same command.

Changing a sweep control clears that panel's results and cancels its active job.
Stop keeps partial results and their displayed configuration. Starting again
clears them; starting the other panel cancels the previous run and restores its
buttons.

`info({dims})` reports integrand exact values at the requested dimension count,
using the same clamp and formula as `converge()`. The Bench refreshes those values
when the dimension or integrand changes. Omitting options retains the shared
default dimension behavior.

The optional output sink is a pair `{f32: Float32Array, u8: Uint8Array}` over the
same ordinary ArrayBuffer at the same byte offset. Both views must have room for
the requested payload. Valid pairs are reused, including nonzero offsets;
invalid, detached, shared, or undersized pairs receive new buffers. Returned
float views contain exactly the requested elements, and bytes outside the payload
are untouched.

## Layout

| File                | Role                                                                           |
| ------------------- | ------------------------------------------------------------------------------ |
| `main.go`           | Export table; publishes `globalThis.qmc`                                       |
| `leap.go`           | The `leaps` export: which leaps a generator accepts                            |
| `discrepancy.go`    | The `discrepancy` and `metrics` exports, library acceptance and demo work caps |
| `index.html`        | Point Lab markup, with its DOM contract                                        |
| `analysis.html`     | Discrepancy Bench markup, with its DOM contract                                |
| `style.css`         | The shared instrument-rack stylesheet; owns the palette                        |
| `render.js`         | `window.Render` — canvas primitives for both pages                             |
| `runtime.js`        | Go runtime monitoring and recoverable request handling in each realm           |
| `compute.js`        | UI worker client, request ownership, deadlines, and cancellation               |
| `compute-worker.js` | Worker-hosted WASM exports and transferred output buffers                      |
| `accessibility.js`  | Shared throttled live announcer, preserving the latest queued message          |
| `app.js`            | Point Lab controller: scatter, transport, digit inspector                      |
| `analysis.js`       | Bench controller: heatmap, hover, two cancellable N-sweeps                     |
| `favicon.svg`       | An even point set and a clumped one, in 32 pixels                              |

The Go side publishes eight exports — `info`, `points`, `correlate`, `converge`,
`digits`, `leaps`, `discrepancy` and `metrics` — each taking one options object
and returning one plain object.
`info()` is the capability table: every `<select>` on both pages ships empty in
the HTML and is filled from it, and every slider's range is overwritten from it
at boot. Updating the demo's capability table updates these controls without
editing markup; library limits and demo work policies remain separate.
Adding a sequence or a randomization to the table in
`info.go` puts it in both pages' menus with no JavaScript edit at all. Each
source reports its own dimension ceiling, whether it has prime bases and whether
it supports the digit inspector, so the pages hide a panel from data rather than
from a hard-coded list of source names.

`newGenerator` in `converge.go` maps requested options to each sequence
constructor and returns `qmc.Sequence`. The point/statistics loops use that
interface; pseudorandom generation and Halton-specific metadata still have
their own branches. `digits` needs `Bases` and
`Permutation`, which are Halton's and are deliberately not on the interface, so
it recovers the concrete type with an assertion and returns an error — never a
panic — if it is ever reached for anything else.

The palette lives in `style.css` as CSS custom properties, and `render.js` reads
them back through `getComputedStyle`. Change `--halton` in the stylesheet and the
scatter glyphs, the legend swatch and the convergence curve all follow; the
canvas and the stylesheet cannot drift apart.

The quasi-random sequence is a filled circle in teal, pseudo-random is a
diagonal cross in amber, on every page and in every chart. The pairing is
deliberate: shape preserves the comparison when users cannot distinguish the
colours, including in greyscale.

## Reading the numbers

- **A 2-of-_d_ view is a projection.** The Point Lab plots two coordinates. The
  other *d*−2 are not on screen, and the sequence still varies in every one of
  them. A pair can look like a perfect lattice while the set is badly clumped
  somewhere you cannot see, and it can look like a diagonal while every other
  pair is fine. That is precisely why the Bench draws all pairs at once.
- **Correlation measures linear association.** Small sample coefficients do
  not establish independence, low discrepancy or integration accuracy. A useful
  finite point set need not have exactly zero correlation between every pair.
- **A randomized run is seed-dependent by design.** Any of the four
  randomizations makes this randomized quasi-Monte Carlo. Different
  seeds can give different point sets, and both the worst correlated pair and
  the error curve can vary. Finite seeded schemes need not give a unique point
  set for every seed. One displayed seed is not a seed-distribution
  summary. Fix the seed and everything is reproducible again.
- **The heatmap's colour ramp is eased, not linear.** Magnitudes are raised to
  the 0.65 power before they are coloured, making small coefficients easier to
  inspect. The legend says so.
- **The wasm timings are relative only.** Each Go instance executes within its
  own realm. Worker placement changes responsiveness; benchmark timings still
  depend on the device, compiler, configuration, and warmup. The controlled
  measurements below concern UI behavior rather than universal library throughput.
- **A discrepancy comparison is one measurement.** CD2 can separate useful
  and random sets only weakly at high dimensions. Compare its measured value
  with another available metric and the random RMS reference; none of these
  supplies an error bar for your integrand. Generic star computation has work
  limits, and CD2 has floating-point range and cancellation limits. The demo's
  point caps are retained work policies, not current runtime calibrations.
- **The convergence chart is one seed, one integrand, one dimension count.** It
  shows the shape of the two error curves, not a claim about your integrand.
  Changes to the function, dimensions, point count or seed can change the ordering.

## Two things that look odd and are not

**`guard()` wraps every Go export.** Recovered callback panics return
`{error, panic: true}` as a failed request; explicit validation failures carry
`panic: false`. Both can be followed by a valid request. The shared `runtime.js`
monitor reports request errors in the status line and observes actual runtime
exit, rejected `go.run()`, and WebAssembly traps separately. Terminal failures
disable the controls and expose **Reload WebAssembly**, which starts a fresh page
and instance. Recovery cannot catch every runtime throw or trap.

Missing, null, or non-object options use defaults. Missing/wrong-type fields and
nonfinite numeric fields use their individual fallback values; finite numeric
values are clamped to the export's documented range. Unknown source, integrand,
randomization, or metric strings are rejected rather than silently substituted.

For the complete request/termination regression, compile the test-only fixture
and pass it as the runner's second argument:

```sh
GOOS=js GOARCH=wasm go test -C examples/wasm-demo -c -tags=qmc_browser_fixture -o /tmp/qmc-runtime-fixture.wasm .
node scripts/test-demo-browser.mjs dist /tmp/qmc-runtime-fixture.wasm
```

Its recovered-panic and `os.Exit(0)` exports exist only in the tagged test binary;
the runner verifies that production builds do not contain them. Both pages are
checked for continued operation after a recovered panic, disabled controls after
exit, and a working reload action.

**Heavy calls run in workers.** Each export is synchronous within its calling
realm. The UI awaits worker responses, checks its generation ID, and renders
completed results. Stop terminates the sweep worker rather than waiting for
an event-loop gap after a Go call. Convergence and discrepancy share one sweep
channel; correlation has another, so changing the heatmap cannot cancel a sweep.
The Point Lab has one scatter channel. Idle workers are reused; active replaced
requests are terminated, and page unload/terminal errors dispose of all channels.
The maximum is one worker in Point Lab and two in the Bench, alongside the
small main-thread Go instance used for metadata, leaps, and digit inspection.

Worker output uses JS-owned typed buffers transferred to the DOM thread.
Displayed buffers are never detached merely to reuse them. The direct `qmc`
exports remain synchronous for console/API callers, including their optional
matched-buffer reuse contract. Calling a heavy export directly on the DOM
thread still blocks that thread; the controllers use the worker client.

## Responsiveness verification

`just test-browser` measures all four worker exports at normal and 6× DOM CPU
throttling, without reducing the supported scatter/correlation/convergence
budgets. The tested convergence export budget is 200,000 points; the sweep UI's maximum
rung is 65,536 points. It asserts that DOM timers continue, verifies selected scatter values
against the Go indexed export, cancels an active request and restarts, and clicks
Stop while the largest nested convergence rung is running. Twenty rapid slider
inputs must produce one reduced preview and one full result. While dragging,
preview counts are capped at 64 for nested scrambling or 256 otherwise; after
350 ms without input the full selected budget runs. Controls debounce previews
for 80 ms and invalidate prior results immediately.

For a constrained Linux run, set `QMC_BROWSER_CPUS` to a permitted CPU affinity
list, for example `QMC_BROWSER_CPUS=0 just test-browser`. All Chrome threads,
including workers, then share that CPU; the runner also applies its 6× DOM
profile. This is a reproducible constrained execution profile, not a claim
about all mobile devices. The runner emits per-call time, timer ticks, maximum
DOM timer gap, and Stop latency. Normal protocol requests have 20-second
budgets; the batched workload measurement gets 90 seconds within the overall
120-second browser budget. Each worker has a 20-second boot and two-minute
computation deadline. Timing thresholds bound responsiveness rather than
promise throughput; see PLAN.md for recorded machine/toolchain evidence.

## Accessibility verification

Expand **Correlation values and keyboard explorer** for the full numerical
matrix. Tab enters one cell; arrows move without wrapping, Home/End move within
a row, and Control + Home/End reach the matrix corners. Tab leaves the matrix.
Row and column headers name zero-based dimensions; each cell's accessible name
includes its pair, bases when applicable, and coefficient. Keyboard focus updates
the same heatmap highlight and readout as pointer inspection. Values are available
at all supported correlation dimensions, including the full 48×48 matrix.

Every canvas has a current textual summary. Convergence and discrepancy retain
their full result tables and configuration snapshots. Both progressbars expose
names, completed/total rungs, and completion, Stop, or reset state. The reveal
slider describes the number of points shown. Focus outlines remain visible;
reduced-motion preference stops playback animation and progress transitions.
Polite, atomic live announcements publish at most every 700 ms and retain the
latest pending message, including completion or cancellation.

`just test-browser` compares textual matrices against Go results, sends real
Chrome keyboard events, inspects Chrome's accessibility tree on both pages,
checks progress and reset values, and verifies reduced-motion and announcement
throttling. It also checks the 48×48 text matrix and correlation preview debounce.
For a manual keyboard check, Tab to the explorer summary, expand it with Enter,
Tab into the values, navigate with the keys above, and Tab out to the next control.
Use the keyboard to start/stop a sweep and operate the Point Lab reveal slider.
These regressions verify browser semantics and keyboard operation; they do not
replace testing with specific screen-reader/browser combinations or constitute
a full WCAG conformance audit. The grid interaction follows the
[WAI data-grid pattern](https://www.w3.org/WAI/ARIA/apg/patterns/grid/), and progress
values follow the [ARIA progressbar definition](https://www.w3.org/TR/wai-aria-1.2/#progressbar).

## Randomization interpretation

The seed selects reproducible outputs. Fixed digit scrambling does not give uniform
point marginals: with skip zero, its first base-2 point is 0.5 for every seed, so a
one-point
estimate of the integral of `x²` is 0.25 rather than 1/3 with zero seed variance.
Digital shifting and nested schemes use finite grids, truncated tails, and seeded
pseudorandomness. Replicate variability cannot bound those sources of bias.
The reference slopes on the convergence chart are guides for comparison, not
universal accuracy guarantees. See [the randomization contract](../../docs/randomization.md).
