# Repository review remediation plan

Date: 2026-10-03. Status: in progress; completed tasks carry verification notes below.

This plan covers the core library, mathematical claims, public API, concurrency,
tests, performance, browser demo, accessibility, privacy, tooling, documentation,
release process, and third-party packaging findings from the repository review.
The overall review rating was 7/10. Each item below has an implementation or
decision task and an acceptance criterion.

## Working rules

- Preserve deterministic outputs for existing valid configurations unless a
  correctness fix requires a change. Record output changes and compatibility
  implications in `CHANGELOG.md` and the relevant API documentation.
- Reproduce defects with focused regressions before fixing them. Prefer public
  API tests and independent mathematical references where practical.
- Keep stateless `At` and `AtInto` safe for concurrent use with separate output
  buffers. Performance improvements must preserve this property.
- Distinguish theoretical guarantees, finite-precision limitations, and empirical
  measurements. A benchmark result is evidence for its stated workload.
- Browser findings from source inspection need browser regressions; they were
  not successfully reproduced in a running browser during the review.
- Close an item only after its acceptance criteria are met. For a design proposal,
  a documented decision supported by measurements can close the item without
  adding a new API or optimization.

## Review verification baseline

The review ran locally with Go 1.26.1 on linux/amd64, including executable 386
tests. These observations establish the starting point, not future acceptance:

| Check                                            | Review result                                                        |
| ------------------------------------------------ | -------------------------------------------------------------------- |
| Full ordinary library tests                      | Passed; 95.3% statement coverage; about 179 seconds                  |
| Full 386 library tests                           | Passed; about 205 seconds                                            |
| Root vet and golangci-lint                       | Passed                                                               |
| Root module verification and tidy diff           | Passed                                                               |
| Radical-inverse fuzz targets                     | Both passed 10-second runs; about 654,000 total executions           |
| Selected generator tests under the race detector | Passed                                                               |
| Full race suite                                  | Timed out after 10 minutes in `TestSmallSampleDiscrepancy`           |
| Short race suite                                 | Exceeded a two-minute review budget in discrepancy tests             |
| Shared clamped-option race reproduction          | Detected two data races                                              |
| Demo build and explicit js/wasm vet              | Passed                                                               |
| Browser end-to-end checks                        | Incomplete; localhost serving was blocked and escalation was aborted |

A complete race-clean run and browser behavioral verification remain outstanding.
Absolute benchmark timings gathered during parallel review work should not be
used as a performance baseline.

## Priorities and execution order

| Priority | Meaning                                                            | Work                                                                                        |
| -------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------- |
| P0       | Scientific guarantees that can mislead ordinary use                | SCI-01                                                                                      |
| P1       | Correctness, API contracts, regressions, and reliable demo results | CORE-01 through CORE-08; TEST-01 through TEST-03; DEMO-01, DEMO-02, DEMO-04 through DEMO-06 |
| P2       | Responsiveness, accessibility, reproducible checks, and delivery   | DEMO-03, DEMO-07 through DEMO-09; TOOL-01 through TOOL-04; SHIP-01, SHIP-02; DOC-01, DOC-02 |
| P3       | Measured optimization and API design decisions                     | PERF-01; API-01                                                                             |

Implement in small, reviewable changes:

1. Correct scientific contracts and fix core boundary, race, and numerical errors.
2. Add the missing contract tests and make the verification jobs reproducible.
3. Fix demo result consistency and establish browser smoke tests, then improve
   responsiveness and accessibility.
4. Reconcile documentation, release artifacts, and developer instructions.
5. Evaluate optional performance and API changes against the improved baseline.

## Scientific contracts

### SCI-01 — Correct randomization, uncertainty, and convergence claims (P0)

- [x] Audit package comments, option comments, README, topic documentation, and
      demo descriptions for unbiasedness and seed-based uncertainty claims.
- [x] Explain that `WithScrambling` reuses a fixed permutation at every digit
      position and does not make every indexed point uniform over the unit cube.
      Remove the blanket assertion that all randomizations yield unbiased estimators.
- [x] Add the counterexample: in one dimension, point zero is 0.5 for every
      fixed digit-scrambling seed; estimating the integral of `x²` with that point
      gives 0.25 rather than 1/3, with zero observed seed variance.
- [x] Specify the assumptions and finite-bit limitations for uncertainty estimates
      from digital shifting, nested scrambling, and hash-based Owen scrambling.
      Distinguish ideal independent permutations from the implemented seeded hashes.
- [x] Correct generic `(t,m,s)`-net occupancy statements: qualifying base-2
      intervals contain `2^t` points; one-point occupancy requires `t = 0`.
      Explain projection guarantees and that nested dyadic scrambling preserves
      occupancy quality rather than repairing a poor direction-number table.
- [x] Scope convergence-rate and accuracy recommendations to their mathematical
      assumptions and measured integrands. Remove unconditional claims of `1/n`
      convergence or dimension-independent quality.
- [x] Decide whether an additional randomization option is needed to meet an
      explicitly stated unbiasedness contract. Preserve the existing option's
      reproducible outputs when correcting its documentation.

Evidence: [sequence.go](sequence.go), [options.go](options.go),
[halton.go](halton.go), [sobol.go](sobol.go), [owen.go](owen.go),
[randomization](docs/randomization.md), [choosing a sequence](docs/choosing-a-sequence.md).

Acceptance: the counterexample is covered by a regression; every public
randomization description agrees with the implementation; seed variance is not
presented as a measure of bias it cannot detect; any new scheme has independent
moment, conditional-structure, and integration checks.

Verification (2026-10-03): the public-API 1024-seed counterexample, existing
known-value/determinism tests, and Sobol/Owen balance and structure regressions
passed (`go test -count=1 -run 'Test(FixedDigitScrambling|RadicalInverseKnownValues|ScramblingIsDeterministicPerSeed|NestedIsDeterministic|OwenScrambleIs|FirstPointsAreOneDimensionallyBalanced|FirstTwoDimensionsFormAZeroNet)' ./...`).
Demo js/wasm vet and formatting checks passed. No new randomization scheme was
added: the finite-output API cannot guarantee continuous unbiasedness for arbitrary
integrands; this decision and the precision/randomness assumptions are documented.

## Core library correctness and contracts

### CORE-01 — Make raw-index validation consistent (P1)

- [x] Reject configurations with no representable first point at construction.
      In particular, reject Halton's `skip = MaxInt` before dividing by a leap.
- [x] Fix the negative-numerator truncation case in Halton's overflow guard:
      `NewHalton(1, WithSkip(math.MaxInt), WithLeap(3)).At(0)` currently returns `[0]`.
- [x] Use consistent checked arithmetic for Sobol construction, indexed access,
      stateful advancement, and reset. On 386, `WithSkip(math.MaxInt)` currently
      permits `Next()` while `At(0)` panics.
- [x] Test boundary combinations of skip, leap, point index, and cursor on both
      int widths, including the last admissible point and its successor.

Evidence: [halton.go](halton.go), [sobol.go](sobol.go),
[overflow tests](overflow_test.go), [leap tests](leap_test.go).

Acceptance: `Next`, `At`, and reset agree throughout each supported index range;
invalid configurations fail predictably without wrapped coordinates or aliasing;
amd64 and 386 regressions pass.

Verification (2026-10-03): new public-API constructor and last-index regressions
failed before the fix. Boundary, leap, cursor, reset, and indexed/stateful agreement
checks pass on amd64 and executable 386 (`go test -count=1 -run
'Test(ConstructorsRejectUnrepresentableFirstIndex|LastRawIndexAgreesAcrossAccessMethods|Fill|NextIntoRefuses|SobolRefuses|SkipBeyond|LeapedNext|NextMatchesAt|OwenNext)' ./...`, also with `GOARCH=386`). The final valid Sobol point now returns normally;
only the subsequent draw panics. Compatibility is recorded in the changelog.

### CORE-02 — Repair prime generation growth and overflow handling (P1)

- [x] Replace the signed `(^int(0)>>1)/2` bound with a correct positive bound.
- [x] Check `15*n` before multiplication and ensure growth cannot wrap or enter
      a zero-limit loop. Return a constructor error for impossible dimension inputs.
- [x] Exercise sieve expansion directly with a deliberately small initial bound,
      or an equivalent bounded test; the existing 64/1000-dimension cases do not
      exercise that branch.
- [x] Cover the 637235-dimension reproduction, where the initial sieve is eight
      below the required prime, using an appropriately scoped regression or slow test.
- [x] Replace the misleading test comment claiming the growth path is exercised.

Evidence: [primes.go](primes.go), [Halton tests](halton_test.go).

Acceptance: expansion finds the required primes; impossible inputs fail promptly;
growth and initial-limit arithmetic are covered on 32-bit and 64-bit targets.

Verification (2026-10-03): the 637235-dimension constructor regression reproduced
the false growth panic before the fix. Bounded expansion, initial multiplication,
growth ceilings, the full large reproduction, and architecture-independent sieve
checks pass on amd64 and executable 386 (`go test -count=1 -run
'Test(PrimesUpTo|PrimeSieve|HaltonRejectsOverflowingPrimeLimit|HaltonPrimeSieveExpands|SieveIsArchitectureIndependent)' ./...`). SymPy 1.14.0 independently confirms
`prime(637235) = 9558533`. The large allocation test skips in short mode while
bounded arithmetic and growth regressions remain. Root lint passes.

### CORE-03 — Make reusable options immutable (P1)

- [x] Clamp `WithSkip` and `WithLeap` arguments before creating their closures.
- [x] Add concurrent constructor regressions sharing `WithSkip(-1)` and
      `WithLeap(0)`, plus valid reusable options.
- [x] Document the separate lifecycle of `WithDirectionNumbers`: its reader is
      consumed by construction and is not an immutable reusable table by itself.

Evidence: [options.go](options.go), [leap.go](leap.go),
[direction-number option](sobol.go).

Acceptance: sharing the clamped options between concurrent constructors produces
no race reports and the same configurations as sequential application; reader
ownership and reuse limitations are explicit.

Verification (2026-10-03): concurrent public constructors sharing negative skip,
zero leap, and combined clamped options reproduced data races before the fix.
The same cases plus valid reused options pass three race-detector repetitions
(`go test -race -count=3 -timeout=2m -run
'^TestOptionsAreReusableAcrossConcurrentConstructors$' ./...`). Reader consumption,
close ownership, and fresh-reader reuse are documented in the option and API topic.
Root lint passes.

### CORE-04 — Test the concurrent indexed-access contract (P1)

- [x] Share each generator configuration across goroutines and compare `At` and
      `AtInto` results with a sequential reference using separate destination buffers.
- [x] Include plain, randomized, skipped, leaped, and custom-table configurations.
- [x] Verify indexed calls do not consume or change the stateful cursor.

Evidence: [Sequence](sequence.go), [Halton](halton.go), [Sobol](sobol.go).

Acceptance: these tests run under the race detector and establish the documented
indexed-access guarantee without concurrently invoking unsupported stateful calls.

Verification (2026-10-03): eight goroutines share each of sixteen configurations,
covering both indexed methods, separate buffers, untouched buffer tails, and
cursor preservation after a prior stateful draw. Cases include all randomizations,
skips, leaps, a custom direction table, and nested Halton beyond the stack-scratch
threshold. Three race-detector runs passed alongside option and boundary contracts
(`go test -race -count=3 -timeout=2m -run
'^Test(ConcurrentIndexedAccessPreservesCursor|OptionsAreReusableAcrossConcurrentConstructors|LastRawIndexAgreesAcrossAccessMethods)$' ./...`). Executable 386 access checks and root lint pass.

### CORE-05 — Reject nonfinite discrepancy results (P1)

- [x] Detect nonfinite products, sums, and final values in centered L2 discrepancy.
- [x] Return a meaningful error when the implementation cannot represent a result.
      Evaluate scaled arithmetic if large-dimensional support is worth its complexity.
- [x] Check scratch-size multiplication before allocating `n*s` entries.
- [x] Add valid-input regressions for a one-point origin at 2000 dimensions
      (currently `+Inf, nil` despite a finite result) and larger overflowing cases
      (currently `NaN, nil`).

Evidence: [discrepancy.go](discrepancy.go), [discrepancy tests](discrepancy_test.go).

Acceptance: valid inputs yield a finite answer within the supported range or an
explicit error; overflow never becomes a successful nonfinite or misleading zero
result; allocation-size overflow is rejected before allocation.

Verification (2026-10-03): origin sets at 2000/6100/7000/9000 dimensions
reproduced successful nonfinite answers before the fix and now return explicit
range errors. A 1000-dimensional origin agrees with its independent closed form.
Scratch-entry and byte-size overflow tests pass on amd64 and executable 386.
Existing one-point formulas, defining-integral comparisons, reflection/order
invariance, reproducibility, and malformed-input tests pass, as does root lint.
Scaled arithmetic was evaluated and deferred in the discrepancy topic: direct
float64 terms define the supported range, even when a norm could fit after scaling.

### CORE-06 — Improve discrepancy precision and cheap special cases (P1)

- [x] Replace the unsupported nine-significant-digit guarantee with an accurate
      account of cancellation for well-distributed point sets.
- [x] Add midpoint-grid references using `CD2 = 1/(sqrt(12)*N)`, including
      `N = 16384`, where the review measured about `1.19e-7` relative error.
- [x] Implement or evaluate a stable one-dimensional formula. Assess compensated
      accumulation for general dimensions without assuming it removes final cancellation.
- [x] Evaluate closed-form star-discrepancy paths for one point and one dimension
      before applying the generic dimension/work gate. Document any intentional refusal.
- [x] Treat the leaf budget as a work bound calibrated on a stated machine,
      rather than a universal wall-clock guarantee.

Evidence: [discrepancy.go](discrepancy.go), [discrepancy documentation](docs/discrepancy.md).

Acceptance: numerical tolerances follow independent references and stated precision
limits; cheap supported cases avoid unnecessary generic enumeration; special-case
decisions and work-budget limitations are documented.

Verification (2026-10-03): midpoint references at N=1/3/16/4096/16384 and the
100-dimensional one-point star case failed before the fix and now pass. Exact
rational tensor-grid references, the defining-integral checks, existing closed
forms, strict/inclusive brute-force enumeration, malformed-input checks, and
numerical-range regressions pass on amd64; precision and brute-force checks also
pass on executable 386. The direct-count 1D star formula retains agreement with
the independent reference. Root lint passes. Three before/after 39-dimensional
CD2 benchmark repetitions showed the same allocation counts and variable timings;
compensation is retained for accuracy, with no speed claim. Remaining cancellation
limits and the machine-dependent meaning of the leaf budget are documented.

### CORE-07 — Make allocation guarantees accurate (P1)

- [x] Add allocation regressions for nested Halton at 97, 98, and 100 dimensions.
      The review measured 0, 1, and 3 allocations per `AtInto` call respectively.
- [x] Decide between explicitly documenting the per-coordinate scratch fallback
      and providing caller-owned workspace for large bases.
- [x] Apply the chosen contract consistently to `Sequence`, `Halton`, benchmarks,
      and performance documentation. Preserve concurrent indexed access.

Evidence: [nested.go](nested.go), [sequence.go](sequence.go),
[benchmarks](bench_test.go), [performance](docs/performance.md).

Acceptance: public allocation claims are true for every configuration they cover;
tests guard the threshold; any workspace API has clear ownership and concurrency rules.

Verification (2026-10-03): both Into methods measure exactly 0/1/3 allocations
at 97/98/100 nested-Halton dimensions on amd64 and executable 386
(`go test -count=1 -run '^TestNestedHaltonScratchAllocationThreshold$' ./...`).
The threshold benchmark confirms those counts; it is not used for speed claims.
The chosen contract documents per-coordinate scratch fallback, preserving existing
outputs and concurrent indexed access already tested under CORE-04. Workspace
API evaluation remains coordinated under PERF-01/API-01. Root lint passes.

### CORE-08 — Tighten custom direction-number validation (P1)

- [ ] Apply the coefficient-width bound at degree one, where only `a = 0` is valid.
- [ ] Reject rows such as `2 1 1 1` and `2 1 9223372036854775808 1` instead of
      accepting coefficients through OR/truncation.
- [ ] Add parser cases for malformed headers, reader errors, row boundaries,
      and coefficient/degree limits. Consider a bounded parser fuzz target.

Evidence: [sobol_direction.go](sobol_direction.go), [Sobol tests](sobol_test.go).

Acceptance: malformed rows fail with useful diagnostics while embedded and valid
caller-supplied tables produce the existing reference values.

## Verification and statistical testing

### TEST-01 — Broaden the evidence for sampling quality (P1)

- [ ] Add a small set of independently integrable functions covering nonlinear
      moments, coordinate interactions, reordered dimension weights, localized peaks,
      and a discontinuous case with explicitly limited expected QMC benefit.
- [ ] Test several sample budgets, including small populations and aligned
      power-of-two Sobol blocks, rather than inferring a general rate from one budget.
- [ ] Keep seeded Monte Carlo baselines and negative controls; record seed counts
      and uncertainty when publishing comparisons.
- [ ] Use at least the documented thirty-seed distribution for correlation
      summaries and enough streams to support comparisons between similar schemes.

Evidence: [integration tests](integration_test.go),
[Sobol integration tests](sobol_integration_test.go),
[correlation tests](correlation_test.go), [small-sample tests](small_sample_test.go).

Acceptance: mathematical claims have corresponding evidence; statistical checks
detect meaningful deterioration without enforcing universal superiority on every
integrand or sample count.

### TEST-02 — Replace brittle rankings and strengthen failure detection (P1)

- [ ] Replace the exact-winner assertion in `TestSmallSampleRankingMatchesLargeSample`
      with a justified margin or tie-aware comparison.
- [ ] Reject NaN/Inf explicitly in statistical ratios and range assertions.
- [ ] Retain direct conditional-structure tests for both nested scramblers and
      clarify their sensitivity to partial loss of conditioning.
- [ ] Cover defensive guards and rare branches with focused inputs. Fuzz mappings
      that intentionally exclude panic boundaries need separate boundary tests.

Evidence: [small_sample_test.go](small_sample_test.go), [fuzz_test.go](fuzz_test.go),
[Owen tests](owen_test.go), [nested tests](nested_test.go),
[testing methodology](docs/testing-methodology.md).

Acceptance: near ties do not fail solely because a different scheme wins;
nonfinite measurements cannot pass through false comparisons; identified guard
and conditional-structure regressions fail the intended tests.

### TEST-03 — Establish complete, affordable verification jobs (P1)

- [ ] Separate routine contract/race checks from expensive statistical sweeps.
      Ensure `-short` actually excludes the expensive discrepancy measurements too.
- [ ] Give slow jobs explicit Go test timeouts and workflow budgets based on
      measured runtimes. Do not interpret the review timeouts as assertion failures.
- [ ] Run statistical sweeps in an appropriate scheduled or explicit validation
      job, with a smaller justified PR gate and a full release check.
- [ ] Complete a race run covering the required contract tests and record the
      full-suite race strategy rather than claiming an uncompleted suite passed.
- [ ] Complete browser verification after DEMO-06 provides a bounded smoke suite.

Evidence: [test workflow](.github/workflows/test.yml), [justfile](justfile),
[discrepancy tests](discrepancy_test.go), [small-sample tests](small_sample_test.go).

Acceptance: each required job completes within an explicit budget on supported
runners; routine and slow commands are reproducible locally; no verification
status is inferred from incomplete or compile-only runs.

## Browser demo

### DEMO-01 — Keep sweep results tied to their configuration (P1)

- [ ] Cancel the relevant active job when source, randomization, dimensions,
      skip, leap, seed, integrand, metric, or sample budget changes.
- [ ] Make resets invalidate old jobs and restore their transport controls.
- [ ] Store result configuration and show it with partial/completed results.
- [ ] Test transitions between convergence and discrepancy sweeps and between
      available/unavailable metrics.

Evidence: `resetSweep`, `resetDiscSweep`, `runSweep`, and control listeners in
[analysis.js](examples/wasm-demo/analysis.js).

Acceptance: changing controls during a sweep never appends old results beneath
new settings, leaves a stale chart presented as current, or strands disabled controls.

### DEMO-02 — Fix dimension-dependent values and source descriptions (P1)

- [ ] Compute the Gaussian explanatory exact value for the selected dimensions;
      use the authoritative returned value or a dimension-aware metadata export.
- [ ] Preserve the chart's already-correct `converge().exact` calculation.
- [ ] Give Halton and Sobol source-specific descriptions for the unrandomized
      option instead of attributing Halton's high-prime defect to both.
- [ ] Reconcile the stale five-seed comparison, pair-correlation figures, and
      first-pass claims with the documented sample counts and prime bases.

Evidence: [info.go](examples/wasm-demo/info.go),
[converge.go](examples/wasm-demo/converge.go),
[analysis.js](examples/wasm-demo/analysis.js),
[demo README](examples/wasm-demo/README.md).

Acceptance: Gaussian notes and result readouts agree at dimensions 1, 4, and 32;
source descriptions and displayed comparison figures agree with reproducible evidence.

### DEMO-03 — Bound work on the browser main thread (P2)

- [ ] Debounce expensive controls and use reduced sampling while dragging.
- [ ] Evaluate a worker-hosted WASM instance for scatter, correlation, convergence,
      and discrepancy work, with request IDs and cancellation between bounded chunks.
- [ ] Until workers are available, cap per-call work using the selected scheme,
      dimensions, sample count, and measured device behavior; test nested scrambling
      at the currently legal large configurations.
- [ ] Avoid treating animation-frame coalescing or yielding between large calls
      as a guarantee that Stop can interrupt an individual computation.

Evidence: [app.js](examples/wasm-demo/app.js),
[analysis.js](examples/wasm-demo/analysis.js),
[points.go](examples/wasm-demo/points.go),
[correlate.go](examples/wasm-demo/correlate.go).

Acceptance: interactive controls and cancellation remain responsive under the
supported maximum workloads; measured budgets and worker/partial-result behavior
are recorded, including behavior on a constrained device.

### DEMO-04 — Validate typed-array output buffers (P1)

- [ ] Validate typed-array kinds, byte capacities, shared backing buffers,
      offsets, and requested lengths before reusing a caller-provided sink.
- [ ] Check the byte count returned by `js.CopyBytesToJS`.
- [ ] Reject or safely replace mismatched, detached, and undersized views.

Evidence: `sinkFor` and `float32Sink.write` in
[marshal.go](examples/wasm-demo/marshal.go).

Acceptance: malformed output pairs cannot return plausible unchanged or partially
written floats; the ordinary matched-buffer reuse path remains correct and efficient.

### DEMO-05 — Distinguish recovered request failures from runtime termination (P1)

- [ ] Do not permanently mark the instance dead solely because a callback panic
      was recovered. Determine whether subsequent safe calls remain usable.
- [ ] Provide an explicit reset/reload action and coherent disabled controls when
      the runtime has actually terminated or cannot be trusted.
- [ ] Make documented fallback behavior consistent for missing, null, malformed,
      and nonfinite options, including default calls that later access `opts.Get`.
- [ ] Test a rejected request followed by a valid request and a genuine runtime
      termination separately.

Evidence: [bridge.go](examples/wasm-demo/bridge.go),
[points.go](examples/wasm-demo/points.go),
[app.js](examples/wasm-demo/app.js), [analysis.js](examples/wasm-demo/analysis.js).

Acceptance: recoverable failures remain recoverable; terminal failures have a clear
recovery path; default/malformed option handling matches its documented contract.

### DEMO-06 — Add behavioral demo verification (P1)

- [ ] Introduce a small browser smoke suite and a deterministic local test server
      with bounded startup/readiness waits and cleanup on failure.
- [ ] Cover both pages, source/randomization switching, Gaussian dimensions,
      sweep control changes, Stop/restart, unavailable metrics, error recovery,
      typed-array transfers, and loading failures.
- [ ] Test pure helper functions where useful; extract testable Go logic from
      js/wasm-only files when it reduces duplication or enables meaningful tests.
- [ ] Capture browser errors and fail the smoke suite on unexpected console/runtime
      errors. Add the suite to PR validation before Pages deployment.

Evidence: [demo module](examples/wasm-demo/go.mod),
[test workflow](.github/workflows/test.yml),
[Pages workflow](.github/workflows/wasm-demo-pages.yml).

Acceptance: runtime boot and key user flows are verified in a real browser;
broken WASM/static assets and the identified result-consistency bugs fail CI;
startup polls cannot hang indefinitely.

### DEMO-07 — Make heatmap and progress information accessible (P2)

- [ ] Provide keyboard cell navigation and a textual matrix/table or equivalent
      accessible values for the correlation heatmap.
- [ ] Add dynamic canvas summaries and an accessible explanation of its legend.
- [ ] Add progressbar roles, names, and current values to both progress indicators.
- [ ] Keep reduced-motion behavior, visible focus, and throttled live announcements.

Evidence: [analysis.html](examples/wasm-demo/analysis.html),
[analysis.js](examples/wasm-demo/analysis.js), [style.css](examples/wasm-demo/style.css).

Acceptance: keyboard-only and screen-reader users can inspect correlation values
and follow sweep progress; automated accessibility checks and manual keyboard
verification cover both pages.

### DEMO-08 — Remove unnecessary external font requests (P2)

- [ ] Use system fonts or self-host the chosen fonts with their required notices.
- [ ] Document the demo's actual network behavior and distinguish it from the
      dependency-free, network-free sequence-generation library.

Evidence: [index.html](examples/wasm-demo/index.html),
[analysis.html](examples/wasm-demo/analysis.html), [toolchain docs](docs/toolchain.md).

Acceptance: after local assets load, the demo needs no third-party requests for
fonts or computation; existing absence of analytics and data submission is preserved.

### DEMO-09 — Stop idle animation wakeups (P2)

- [ ] Schedule animation frames only while playing or when a redraw is needed.
- [ ] Cancel pending frames on pause and manage background-tab behavior.

Evidence: [app.js](examples/wasm-demo/app.js).

Acceptance: a paused, unchanged page has no perpetual animation loop; play, pause,
scrubbing, and reduced-motion flows continue to work.

## Tooling and CI

### TOOL-01 — Make formatting checks strict and reproducible (P2)

- [ ] Pin gofumpt, gci, shfmt, prettier, shellcheck, and treefmt versions.
- [ ] Install every required tool and stop swallowing prettier setup failures.
- [ ] Remove permissive missing-formatter behavior from required CI checks.
      An explicitly optional local mode may remain if documented.
- [ ] Run shellcheck as an explicit diagnostic gate with its exit status respected.
- [ ] Verify checks fail when a required formatter is unavailable and when a
      representative Go, Markdown, YAML, JavaScript, CSS, HTML, or shell file is invalid.

Evidence: [justfile](justfile), [treefmt.toml](treefmt.toml),
[test workflow](.github/workflows/test.yml).

Acceptance: successful required checks mean every configured file type was checked;
a fresh installation produces the same results as CI.

### TOOL-02 — Harden and broaden development-tool installation (P2)

- [ ] Detect supported OS/architecture combinations instead of hardcoding
      `linux_amd64`; document unsupported platforms clearly.
- [ ] Verify downloaded archive checksums before extraction and installation.
- [ ] Prefer a user-owned installation directory over unconditional privileged
      extraction; document PATH setup and required host prerequisites.
- [ ] Verify an existing tool's version rather than accepting any binary on PATH.

Evidence: `setup-deps` in [justfile](justfile).

Acceptance: setup validates installed versions, handles supported amd64/arm64 and
macOS/Linux environments consistently, and refuses unverifiable downloads.

### TOOL-03 — Cover the nested module and align local verification with CI (P2)

- [ ] Add explicit demo-module tidy checks, js/wasm vet, suitable linting, and
      behavioral tests. Native stub compilation does not validate the WASM code.
- [ ] Replace the blanket examples lint exclusion with narrowly justified rules
      for demo production code versus actual teaching examples.
- [ ] Define clear local commands for routine CI checks, slow statistical checks,
      WASM checks, and release validation; share those commands with workflows.
- [ ] Retain executable 386 testing and supported Go-version coverage. Test a
      deliberate publishing toolchain separately from minimum-version compatibility.
- [ ] Remove references to untracked `.trunk` as part of the reproducible setup,
      and update obsolete comments claiming the suite cannot compile for 386.
- [ ] Make script arguments, including demo output location, reachable through
      the corresponding just recipe.

Evidence: [.golangci.yml](.golangci.yml), [justfile](justfile),
[test workflow](.github/workflows/test.yml), [toolchain docs](docs/toolchain.md).

Acceptance: local commands and workflows enforce the same documented checks for
both modules; minimum-version compatibility and publishing-toolchain choices are
explicit; no required configuration exists only in one developer's clone.

### TOOL-04 — Pin workflow dependencies and match release validation to promises (P2)

- [ ] Pin GitHub Actions to reviewed commit SHAs, retaining readable version
      comments and Dependabot updates.
- [ ] Keep permissions minimal per job, limiting Pages write permissions to jobs
      that need them.
- [ ] Ensure release validation includes the required race, statistical, module,
      WASM, browser, and artifact checks with explicit budgets.
- [ ] Keep version validation and local release commands consistent and verify
      releases correspond to clean, reviewed source and documented changes.

Evidence: [test workflow](.github/workflows/test.yml),
[release workflow](.github/workflows/release.yml),
[Pages workflow](.github/workflows/wasm-demo-pages.yml),
[Dependabot](.github/dependabot.yml), [justfile](justfile).

Acceptance: workflow dependencies change through reviewable updates; release
checks match the documented support and verification claims; permissions remain
limited to their intended jobs.

## Build artifacts and third-party materials

### SHIP-01 — Produce clean, versioned demo artifacts safely (P2)

- [ ] Build in a fresh staging directory and publish the resulting complete set
      atomically or with an equivalent deployment boundary.
- [ ] Validate output destinations and avoid deleting or overwriting unrelated
      caller-owned content when implementing cleanup.
- [ ] Include an explicit recursive asset policy or manifest covering future
      images, fonts, JSON, and subdirectories, as well as existing assets.
- [ ] Version/hash HTML references to scripts, styles, WASM, and its compiler-matched
      runtime so a returning visitor cannot mix incompatible builds.
- [ ] Add artifact checks for expected pages, linked assets, and absence of removed
      assets; verify paths work from different invoking directories.

Evidence: [build script](scripts/build-wasm-demo.sh),
[Pages workflow](.github/workflows/wasm-demo-pages.yml),
[app.js](examples/wasm-demo/app.js), [analysis.js](examples/wasm-demo/analysis.js).

Acceptance: consecutive builds cannot retain deleted assets; all references resolve
within a consistent build; output handling is safe for caller-owned directories;
the runtime and WASM always originate from the same toolchain.

### SHIP-02 — Include distribution notices and credits (P2)

- [ ] Copy the Joe–Kuo copyright, conditions, and disclaimer into the demo output.
- [ ] Link credits/notices from both pages and include the project license.
- [ ] Include notices for any newly self-hosted fonts or other bundled materials.
- [ ] Add a build-artifact assertion that required notices are present.

Evidence: [Joe–Kuo notice](third_party/joe-kuo/LICENSE.txt), [LICENSE](LICENSE),
[build script](scripts/build-wasm-demo.sh), [README](README.md).

Acceptance: source and built distributions contain the applicable third-party
materials and users can find them from the demo. This addresses the packaging
omission observed in review without claiming a legal determination.

## Documentation and maintainability

### DOC-01 — Consolidate measurements and remove contradictory claims (P2)

- [ ] Correct `sqrt(E[CD2²])` versus `E[CD2]` terminology in package comments,
      demo documentation, metadata, and baseline labels.
- [ ] Reconcile the nested-scrambling 8×/40× cost discrepancy and the stale
      five-seed/ten-stream figures with one reproducible measurement source.
- [ ] Record hardware, toolchain, configuration, seeds, sample sizes, uncertainty,
      and generating commands alongside published measurements.
- [ ] Audit blanket statements about independence, dimension limits, burn-in,
      Sobol alignment, convergence, and universal superiority after SCI-01 is complete.
- [ ] Move historical variants and repeated machine-specific tables out of source
      comments where they obscure the implementation. Keep invariants, contracts,
      formulas, and relevant references near the code.

Evidence: [README](README.md), [sequence.go](sequence.go), [nested.go](nested.go),
[sobol.go](sobol.go), [discrepancy.go](discrepancy.go),
[documentation index](docs/README.md), [demo README](examples/wasm-demo/README.md).

Acceptance: public descriptions agree across source, docs, and demo; each retained
quantitative comparison identifies reproducible evidence and its limitations;
historical explanations do not masquerade as current guarantees.

### DOC-02 — Reconcile open work and document contribution/release commands (P2)

- [ ] Link this plan from the documentation index and reconcile the existing
      distributed "known gaps"/"still open" lists with these task IDs.
- [ ] Remove stale gap statements, including the claim that no conditional-structure
      test exists when direct nesting tests are already present.
- [ ] Document the actual package-comment location and tracked toolchain setup;
      remove references to local-only Trunk configuration as required project state.
- [ ] Add concise contributor instructions covering setup, fast/slow tests,
      browser checks, benchmark reproduction, and release validation.
- [ ] Update completed-task status and related topic pages together, keeping
      explanatory documentation near the design reasoning.

Evidence: [docs/README.md](docs/README.md),
[testing methodology](docs/testing-methodology.md),
[toolchain documentation](docs/toolchain.md), [API design](docs/api-design.md).

Acceptance: contributors have one current checklist and clear reproducible commands;
topic-page open-work lists cannot disagree silently with the implementation or plan.

## Measured optimization and API decisions

### PERF-01 — Re-establish comparable performance evidence (P3)

- [ ] Measure Halton and Sobol on the same machine/toolchain, using repeated runs
      and reporting allocation counts alongside throughput and constructor memory.
- [ ] Document scrambled Halton construction cost at its call site, including
      high-dimensional memory growth and guidance to reuse generators.
- [ ] Profile base-2 specialization, reciprocal-based arithmetic, and a bulk fill
      API before choosing optimizations; verify mathematical and reproducibility effects.
- [ ] Evaluate bounded, immutable root/shallow-node permutation caches separately
      from caching every nested node. Replace the categorical claim that caching
      "cannot" work with the measured conclusion for each design.
- [ ] Compare options by end-to-end work and relevant integration accuracy as well
      as per-point cost; retain the efficient Sobol recurrence and contiguous storage.

Evidence: [bench_test.go](bench_test.go), [sobol_bench_test.go](sobol_bench_test.go),
[nested.go](nested.go), [performance documentation](docs/performance.md).

Acceptance: performance comparisons share a reproducible baseline; each proposed
optimization is implemented with measured benefit or closed with a recorded
reason; allocation, concurrency, accuracy, and compatibility checks remain green.

### API-01 — Resolve remaining API proposals explicitly (P3)

- [ ] Evaluate optional capability metadata without unnecessarily expanding the
      minimal `Sequence` interface or duplicating library policies in the demo.
- [ ] Document why `Option` uses an unexported settings type, and distinguish
      immutable options from options owning consumable readers.
- [ ] Decide whether checked indexed-access helpers would benefit callers;
      preserve existing methods and document panic boundaries if no change is warranted.
- [ ] Explain and test aligned power-of-two Sobol usage. Evaluate whether a raw-origin
      or aligned-block helper is needed; the current skip facility can already align
      later blocks, so a new API is not automatically required.
- [ ] Review internal invalid-base guards and their intended preconditions after
      overflow fixes; remove or retain them with explicit reasoning.
- [ ] Coordinate any bulk/workspace proposal with CORE-07 and PERF-01 rather than
      introducing multiple overlapping allocation APIs.

Evidence: [sequence.go](sequence.go), [options.go](options.go), [sobol.go](sobol.go),
[API design documentation](docs/api-design.md),
[choosing a sequence](docs/choosing-a-sequence.md).

Acceptance: each proposal has a recorded decision, compatibility implications,
and evidence for any added API; existing interfaces and deterministic valid-input
behavior remain stable unless an intentional change is documented.

## Completion checklist

- [ ] SCI-01 and CORE-01 through CORE-08 are resolved with regressions and accurate contracts.
- [ ] TEST-01 through TEST-03 establish meaningful, bounded verification for supported targets.
- [ ] DEMO-01 through DEMO-09 are verified in browser, accessibility, and failure-state checks.
- [ ] TOOL-01 through TOOL-04 run reproducibly in a clean environment for both modules.
- [ ] SHIP-01 and SHIP-02 verify complete, consistent artifacts and notices.
- [ ] DOC-01 and DOC-02 reconcile every affected public claim and open-work list.
- [ ] PERF-01 and API-01 have measured implementations or documented decisions.
- [ ] Final ordinary, required race, statistical, WASM, browser, formatting, lint,
      and release-artifact checks pass under the agreed budgets.
- [ ] Re-review category scores using evidence from the completed work; do not
      raise scores solely because checklist items were marked complete.
