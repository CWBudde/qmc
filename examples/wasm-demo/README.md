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
  expectation of the random one.

The organising rule is that **no QMC logic lives in JavaScript**. Every point,
every prime base, every correlation and every integration error comes out of the
Go library. The JavaScript owns the DOM, the canvas and the clock. A demo that
reimplemented the radical inverse in JS would be demonstrating the JS.

## The default view

The Point Lab opens on Halton, 39 dimensions, axes 37 against 38, randomization
**none**. The points sit on a lockstep diagonal ramp, because at 600 points
neither coordinate has finished its first pass through its prime base — 163 and
167 — and the two ramps advance together. Pick a randomization and the diagonal
dissolves into a filled square. That single choice is the argument the library's
README makes in a table; this page makes it in one gesture. Switching the
sequence to Sobol makes the other half of the point: it is base 2 in every
dimension, so it never had a large-base ramp to escape from.

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
Every menu entry's description is the option's own doc comment, unflattering
parts included: nested scrambling integrates about twice as well as
random-digit scrambling and costs about forty times as much per point, and Owen
scrambling is nearly free on `At` and three times the cost on `Next`.

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

It is also the only control here whose legal values depend on the other
controls. _L_ must share no factor with any base in use, and if it does, that
coordinate's leading digit never changes and it spends the whole run inside one
strip of width 1/base — scrambling does not rescue it, because a permuted
constant is still constant. The library refuses that at construction, by name.
At 39 Halton dimensions the smallest admissible leap is therefore **173**, and
Sobol is base 2 in every dimension so it refuses every even leap.

That sparsity is why there is a sixth export, `leaps`. It reports whether the
current number is admissible for the sequence and dimension count now selected,
which nearby values are, and — when it is not — the constructor's own refusal,
naming the dimension and the base. The page renders that sentence verbatim under
the control and does not ask for points, so an illegal leap reads as a sparse
control rather than as a broken one. Admissibility is decided by **building a
generator and reading the error**, not by re-deriving coprimality in the demo:
the library is the only place that says what a constructor accepts, and a second
copy here is the copy that would go stale.

The Bench's comparison against the README's quoted 0.81 and 0.14 is withdrawn as
soon as a leap is set, the same way a changed burn-in withdraws it — those
figures are measured unleaped, and a leaped run is a different experiment.

## Discrepancy

The third panel measures the quantity the phrase "low-discrepancy sequence"
refers to, and the page has been called the Discrepancy Bench since it shipped
while computing no discrepancy at all. It sweeps _N_ the way the convergence
panel does and draws three curves on log–log axes: the sequence's discrepancy,
a pseudo-random set's over the same _N_, and — for centred L2 — the closed-form
expectation of the random one,

    E[CD2] = sqrt(((5/4)^s - (13/12)^s)/N)

which is exact, costs nothing to evaluate, and is the thing that makes the
defect visible. The headline is the ratio **random ÷ sequence**, matching the
convergence panel's **mc ÷ qmc**: a ratio of 1.00 needs no interpretation.

It opens on the defect — 39 dimensions, centred L2 — so the page loads showing
the null result: three superimposed curves and a ratio of about **1.02**.
Centred L2's expectation is dominated by a `(5/4)^s` term that depends only on
each coordinate's marginal spread, which at 39 dimensions is 100.4% of the
total, so the statistic has nothing left to say about how the points sit
relative to one another. Drop the slider to 4 dimensions and star discrepancy
becomes available and separates the same two point sets by about **1.7x**; drop
centred L2 to 2 dimensions and it separates them by more than **12x**.

**Star's ceiling is asked of the library, not restated here.** Exact star
discrepancy is NP-hard in the dimension, and `qmc.StarDiscrepancy` refuses
above six dimensions or above its own work budget with an error naming the
leaf count, the reason, and the affordable point count for each dimension. The
`metrics` export finds out by building the smallest possible request and reading
that error, then prints it verbatim under the menu — exactly the `leaps`
precedent, and for the same reason: the library is the only place that says what
it accepts, and a second copy in the demo is the copy that goes stale. It also
probes downward for the widest cube star still accepts and offers it as a
button, so the page offers the fix and not only the refusal. The metric menu
stays fully populated at every dimension count; Start is what gets disabled.

**The N ceiling moves with the dimension slider**, which nothing else on either
page does, and that needs saying or it reads as a bug. Centred L2 costs
O(*N*²*s*) and the library computes one _N_ in a single atomic call, so it
cannot be sliced the way the sweep itself is — the only lever left is _N_, and
it has to fall as dimensions are added. Both ceilings are **measured on
`js/wasm` in a browser**, not estimated: one centred-L2 rung costs
`N(N-1)/2 * (5.7s + 7.5)` nanoseconds, fitted over dimensions 1 to 64 and _N_ 64
to 7072, and the cap solves that for 150 ms per call — 4767 points at one
dimension, 1142 at 39, 897 at 64. Star's cost cannot be modelled from outside at
all, because its pruner depends on the point set and not only on its shape, so
its cap is a measured table: 1792 points at two dimensions down to 32 at six,
each about 350–420 ms per call. That is a browser-responsiveness limit and not a
mathematical one, and the sentence under the chart says so.

Expect a **short ladder** for star. At four dimensions it is six rungs and the
whole sweep finishes in about a second; at six it is three.

## Build and run

```bash
just run-wasm-demo                            # build into ./dist and serve on :8090
just build-wasm-demo                          # build only
./scripts/build-wasm-demo.sh /tmp/somewhere   # build somewhere else
```

**An HTTP server is required.** Both pages `fetch("qmc.wasm")`, and a `file://`
URL cannot fetch a `.wasm` at all. The server must also send the module as
`Content-Type: application/wasm`; without it `WebAssembly.instantiateStreaming`
refuses the response and the status line says so.

Every asset reference in these pages is a bare relative filename — `style.css`,
`render.js`, `qmc.wasm` — with no leading slash anywhere. That is what lets the
identical directory work at `http://localhost:8090/` and at
`https://cwbudde.github.io/qmc/` with no base-path handling and no build-time
rewriting.

`wasm_exec.js` is copied from your Go toolchain at build time and is never
committed. It is version-locked to the compiler that produced the `.wasm`, and a
stale copy fails at runtime in ways that look like demo bugs rather than like a
version mismatch.

## Layout

| File             | Role                                                                        |
| ---------------- | --------------------------------------------------------------------------- |
| `main.go`        | Export table; publishes `globalThis.qmc`                                    |
| `leap.go`        | The `leaps` export: which leaps a generator accepts                         |
| `discrepancy.go` | The `discrepancy` and `metrics` exports, and both measured browser ceilings |
| `index.html`     | Point Lab markup, with its DOM contract                                     |
| `analysis.html`  | Discrepancy Bench markup, with its DOM contract                             |
| `style.css`      | The shared instrument-rack stylesheet; owns the palette                     |
| `render.js`      | `window.Render` — canvas primitives for both pages                          |
| `app.js`         | Point Lab controller: scatter, transport, digit inspector                   |
| `analysis.js`    | Bench controller: heatmap, hover, two cancellable N-sweeps                  |
| `favicon.svg`    | An even point set and a clumped one, in 32 pixels                           |

The Go side publishes eight exports — `info`, `points`, `correlate`, `converge`,
`digits`, `leaps`, `discrepancy` and `metrics` — each taking one options object
and returning one plain object.
`info()` is the capability table: every `<select>` on both pages ships empty in
the HTML and is filled from it, and every slider's range is overwritten from it
at boot. Raising a limit in the library raises it in the UI without anyone
editing markup, and adding a sequence or a randomization to the table in
`info.go` puts it in both pages' menus with no JavaScript edit at all. Each
source reports its own dimension ceiling, whether it has prime bases and whether
it supports the digit inspector, so the pages hide a panel from data rather than
from a hard-coded list of source names.

`newGenerator` in `converge.go` is the one place a generator is built, for every
export. It returns `qmc.Sequence`, so `points`, `correlate` and `converge` carry
no per-source branch at all. `digits` is the exception: it needs `Bases` and
`Permutation`, which are Halton's and are deliberately not on the interface, so
it recovers the concrete type with an assertion and returns an error — never a
panic — if it is ever reached for anything else.

The palette lives in `style.css` as CSS custom properties, and `render.js` reads
them back through `getComputedStyle`. Change `--halton` in the stylesheet and the
scatter glyphs, the legend swatch and the convergence curve all follow; the
canvas and the stylesheet cannot drift apart.

The quasi-random sequence is a filled circle in teal, pseudo-random is a
diagonal cross in amber, on every page and in every chart. The pairing is
deliberate: colour alone excludes roughly one man in twelve, and
sequence-against-random is the one comparison these pages exist to make.

## Reading the numbers

- **A 2-of-_d_ view is a projection.** The Point Lab plots two coordinates. The
  other *d*−2 are not on screen, and the sequence still varies in every one of
  them. A pair can look like a perfect lattice while the set is badly clumped
  somewhere you cannot see, and it can look like a diagonal while every other
  pair is fine. That is precisely why the Bench draws all pairs at once.
- **Correlation is a symptom, not the disease.** A low-discrepancy sequence
  promises even coverage of the whole box; pairwise correlation catches only the
  most visible way that promise fails. Zero everywhere is necessary, not
  sufficient.
- **A randomized run is seed-dependent by design.** Any of the four
  randomizations makes this randomized quasi-Monte Carlo, not plain QMC: each
  seed gives a different,
  equally valid point set, and both the worst correlated pair and the error
  curve wobble between seeds. The library's quoted 0.14 is the worst of five
  seeds, not the best of one. Fix the seed and everything is reproducible again.
- **The heatmap's colour ramp is eased, not linear.** Magnitudes are raised to
  the 0.65 power before they are coloured, because the interesting range here —
  0.14 against 0.81 — would otherwise both render as ground. The legend says so.
- **The wasm timings are relative only.** Under `js/wasm` everything runs on one
  thread with no SIMD. Nothing on these pages quotes a wall-clock figure as a
  benchmark of the library, and you should not read one into the responsiveness
  of a slider either.
- **Centred L2 saturates, and the panel is built to show it.** Its expectation
  over _N_ uniform points is exactly `sqrt(((5/4)^s - (13/12)^s)/N)`, and the
  `(5/4)^s` term — the diagonal of the double sum, which depends only on each
  coordinate's marginal spread — swamps everything a well-spread point set can
  improve. Measured at _N_ = 1024, the random-to-sequence ratio decays
  smoothly: 12.5x at 2 dimensions, 6.3x at 5, 2.4x at 10, 1.6x at 15, 1.28x at
  20, 1.08x at 30, 1.02x at 39. That is a decay, not a cliff, so there is no
  honest dimension count at which to refuse — the library returns the number
  and documents the caveat, and the page decides in Go whether the ratio still
  counts as separating the two sets rather than thresholding it in JavaScript.
  Star discrepancy does not saturate; it is simply unaffordable above six
  dimensions.
- **The convergence chart is one seed, one integrand, one dimension count.** It
  shows the shape of the two error curves, not a claim about your integrand.
  Change the dimension slider and watch how much less clear-cut the advantage
  becomes as _d_ grows; that is the honest part of the picture.

## Two things that look odd and are not

**`guard()` wraps every Go export.** A panic that unwinds out of a `js.Func`
aborts the whole WebAssembly instance, so one bad request would brick the page
until a reload. Every export returns its failures as `{error, panic}` data
instead, and the JS `call()` wrapper collapses a missing export, a thrown error
and an `{error}` result into the same thing: a message on the status line and a
`null` return. Nothing from the wasm side is ever allowed to throw into a draw
call. When a result carries `panic: true` the instance really is dead, so the
page says so and stops calling rather than filling the console with noise.

**The convergence sweep loops in JavaScript, not in Go.** A synchronous call
into Go blocks the event loop for its whole duration, which means a click on
Stop cannot be dispatched while one is running. `converge` therefore covers
exactly one _N_ per call, and the sweep `await`s a zero-delay timeout between
calls. That gap is the entire cancellation mechanism — delete the yield and the
Stop button becomes decorative. Every sweep also carries a monotonic run id that
is re-checked after each yield, so a sweep restarted while an older one is still
in flight cannot append its points to the new chart.

Both sweeps — convergence and discrepancy — walk that ladder through the same
`runSweep` and share the same run id, deliberately. The page has one thread and
every rung is a blocking call into Go, so two sweeps could not run side by side
even with separate ids: they would interleave, each freezing the other's yields,
and both progress bars would crawl. Sharing the id makes "start the other panel"
mean "cancel this one", which is what the machine was going to do anyway — the
difference is that the cancelled panel's transport is restored rather than left
disabled.
