# Repository review remediation plan

Date: 2026-10-03. Status: in progress; completed tasks carry verification notes below.

This plan covers the core library, mathematical claims, public API, concurrency,
tests, performance, browser demo, accessibility, privacy, tooling, documentation,
release process, and third-party packaging findings from the repository review.
The overall review rating was 7/10. Each item below has an implementation or
decision task and an acceptance criterion.

## Coverage and current status

There are 32 remediation tasks: 31 have recorded completion evidence and one
remains open. Checkboxes track verified completion; unchecked tasks describe the
work still required. Changes under development count as open until their
acceptance criteria are met.

| Review area                                                  | Tasks                                       | Current status |
| ------------------------------------------------------------ | ------------------------------------------- | -------------- |
| Scientific and statistical claims                            | SCI-01                                      | Verified       |
| Numerical correctness and boundary handling                  | CORE-01, CORE-02, CORE-05, CORE-06, CORE-08 | Verified       |
| Concurrency and allocation contracts                         | CORE-03, CORE-04, CORE-07                   | Verified       |
| Test quality and verification budgets                        | TEST-01 through TEST-03                     | Verified       |
| Demo correctness, failure handling, and browser verification | DEMO-01, DEMO-02, DEMO-04 through DEMO-06   | Verified       |
| Responsiveness, accessibility, privacy, and idle work        | DEMO-03, DEMO-07 through DEMO-09            | Verified       |
| Demo duplication and rendering maintenance                   | DEMO-10                                     | Verified       |
| Reproducible tooling and module coverage                     | TOOL-01 through TOOL-03                     | Verified       |
| Workflow security and release validation                     | TOOL-04                                     | Verified       |
| Build consistency                                            | SHIP-01                                     | Verified       |
| Distribution notices                                         | SHIP-02                                     | Verified       |
| Documentation accuracy and contributor guidance              | DOC-01, DOC-02                              | DOC-02 open    |
| Performance evidence                                         | PERF-01                                     | Verified       |
| API decisions                                                | API-01                                      | Verified       |

The basic improvements are organized around concrete failures first, then
reliable checks and delivery, followed by documentation and measured design
decisions. Each detailed task includes the affected files, specific actions,
and an acceptance criterion so it can be implemented and reviewed independently.

For the remaining work, finish DOC-02's contributor/status reconciliation,
then record the final completion audit and updated category scores.

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

At review time, complete race and browser behavioral verification were outstanding.
Subsequent task verification is recorded below; the final combined verification
and updated category scores remain part of the completion checklist.
Absolute benchmark timings gathered during parallel review work should not be
used as a performance baseline.

## Priorities and execution order

| Priority | Meaning                                                            | Work                                                                                        |
| -------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------- |
| P0       | Scientific guarantees that can mislead ordinary use                | SCI-01                                                                                      |
| P1       | Correctness, API contracts, regressions, and reliable demo results | CORE-01 through CORE-08; TEST-01 through TEST-03; DEMO-01, DEMO-02, DEMO-04 through DEMO-06 |
| P2       | Responsiveness, accessibility, reproducible checks, and delivery   | DEMO-03, DEMO-07 through DEMO-09; TOOL-01 through TOOL-04; SHIP-01, SHIP-02; DOC-01, DOC-02 |
| P3       | Measured optimization and API design decisions                     | DEMO-10; PERF-01; API-01                                                                    |

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

- [x] Apply the coefficient-width bound at degree one, where only `a = 0` is valid.
- [x] Reject rows such as `2 1 1 1` and `2 1 9223372036854775808 1` instead of
      accepting coefficients through OR/truncation.
- [x] Add parser cases for malformed headers, reader errors, row boundaries,
      and coefficient/degree limits. Consider a bounded parser fuzz target.

Evidence: [sobol_direction.go](sobol_direction.go), [Sobol tests](sobol_test.go).

Acceptance: malformed rows fail with useful diagnostics while embedded and valid
caller-supplied tables produce the existing reference values.

Verification (2026-10-03): malformed headers, nonzero degree-one coefficients,
and header handling after blank lines reproduced the defects before the fix.
Focused row/degree/coefficient limits, reader-error propagation, optional headers,
line boundaries, embedded tables, extended tables, and architecture reference
outputs pass on amd64 and executable 386. A bounded parser fuzz target completed
116264 executions in a 10-second run with two workers and no failures
(`go test -run '^$' -fuzz '^FuzzDirectionNumbersParser$' -fuzztime=10s -parallel=2 ./...`).
Root lint/vet and demo js/wasm vet pass. Stricter rejection of malformed headers is
recorded in the changelog; valid table outputs remain unchanged.

## Verification and statistical testing

### TEST-01 — Broaden the evidence for sampling quality (P1)

- [x] Add a small set of independently integrable functions covering nonlinear
      moments, coordinate interactions, reordered dimension weights, localized peaks,
      and a discontinuous case with explicitly limited expected QMC benefit.
- [x] Test several sample budgets, including small populations and aligned
      power-of-two Sobol blocks, rather than inferring a general rate from one budget.
- [x] Keep seeded Monte Carlo baselines and negative controls; record seed counts
      and uncertainty when publishing comparisons.
- [x] Use at least the documented thirty-seed distribution for correlation
      summaries and enough streams to support comparisons between similar schemes.

Evidence: [integration tests](integration_test.go),
[Sobol integration tests](sobol_integration_test.go),
[correlation tests](correlation_test.go), [small-sample tests](small_sample_test.go).

Acceptance: mathematical claims have corresponding evidence; statistical checks
detect meaningful deterioration without enforcing universal superiority on every
integrand or sample count.

Verification (2026-10-03): the seven-function, three-budget, forty-stream sweep
passes on amd64 and executable 386. Its functions include reversed weights,
late-coordinate interactions, a localized analytic Gaussian, and a discontinuity;
Sobol blocks are aligned, and MC/plain-Halton controls remain. All product
comparison tests pass after increasing streams from ten to forty. Correlation
uses thirty streams and reports per-seed-worst median/p90/max 0.0909/0.1139/0.1611.
Commands and uncertainty limitations are documented in testing methodology.
The full ordinary suite at the completed-core milestone passed in 162.077 seconds;
this is a milestone result, not the final repository verification. Root lint passes.

### TEST-02 — Replace brittle rankings and strengthen failure detection (P1)

- [x] Replace the exact-winner assertion in `TestSmallSampleRankingMatchesLargeSample`
      with a justified margin or tie-aware comparison.
- [x] Reject NaN/Inf explicitly in statistical ratios and range assertions.
- [x] Retain direct conditional-structure tests for both nested scramblers and
      clarify their sensitivity to partial loss of conditioning.
- [x] Cover defensive guards and rare branches with focused inputs. Fuzz mappings
      that intentionally exclude panic boundaries need separate boundary tests.

Evidence: [small_sample_test.go](small_sample_test.go), [fuzz_test.go](fuzz_test.go),
[Owen tests](owen_test.go), [nested tests](nested_test.go),
[testing methodology](docs/testing-methodology.md).

Acceptance: near ties do not fail solely because a different scheme wins;
nonfinite measurements cannot pass through false comparisons; identified guard
and conditional-structure regressions fail the intended tests.

Verification (2026-10-03): the 200-stream ranking comparison passes with the
20% near-tie margin (42.492 seconds). Nonfinite correlation inputs are explicitly
rejected; RMS helpers and statistical ratios/range predicates cannot silently
accept NaN/Inf. Defensive-base/index and reversal-overflow boundary tests pass,
including executable 386 where the int64-only reversal case skips explicitly.
Isolated mutations prove direct Owen/nested structure checks reject total loss
of conditioning; the eight-bit child-input mutation demonstrates the direct
test's sensitivity floor and is caught by the seeded reference. Forty-stream
nested/hash/reference integration, thirty-seed correlations, broad integration,
and the new guards pass together (22.490 seconds). Root lint passes; local lint
used its documented parallel-runner flag to avoid a shared temporary lock conflict.

### TEST-03 — Establish complete, affordable verification jobs (P1)

- [x] Separate routine contract/race checks from expensive statistical sweeps.
      Ensure `-short` actually excludes the expensive discrepancy measurements too.
- [x] Give slow jobs explicit Go test timeouts and workflow budgets based on
      measured runtimes. Do not interpret the review timeouts as assertion failures.
- [x] Run statistical sweeps in an appropriate scheduled or explicit validation
      job, with a smaller justified PR gate and a full release check.
- [x] Complete a race run covering the required contract tests and record the
      full-suite race strategy rather than claiming an uncompleted suite passed.
- [x] Complete browser verification after DEMO-06 provides a bounded smoke suite.

Evidence: [test workflow](.github/workflows/test.yml), [justfile](justfile),
[discrepancy tests](discrepancy_test.go), [small-sample tests](small_sample_test.go).

Acceptance: each required job completes within an explicit budget on supported
runners; routine and slow commands are reproducible locally; no verification
status is inferred from incomplete or compile-only runs.

Verification (2026-10-03): added fast/race/statistical/full-statistical-race recipes,
short-mode exclusions for costly discrepancy and high-dimensional integration
sweeps, routine PR race/386 jobs, a scheduled/manual statistical workflow, and
release contract-race plus full ordinary checks. Routine race validation passes
(`just test-race`, 109.685 seconds), as does
executable 386 fast validation (48.608 seconds). YAML parses and root lint passes.
The pre-exclusion short race snapshot exceeded five minutes in the star statistical
sweep; that is a timeout, not an assertion failure. The corrected short suite
completes within its five-minute budget. Full ordinary statistical validation
(`just test-statistical`) passes in 306.323 seconds, within its ten-minute budget.
DEMO-06 now supplies the required real-browser gate: both local entry points pass
within the 120-second browser budget, with the existing-artifact run taking
5.790 seconds. Full statistical race remains an explicitly optional 40-minute
audit rather than an unverified required check; its completion is not claimed.

## Browser demo

### DEMO-01 — Keep sweep results tied to their configuration (P1)

- [x] Cancel the relevant active job when source, randomization, dimensions,
      skip, leap, seed, integrand, metric, or sample budget changes.
- [x] Make resets invalidate old jobs and restore their transport controls.
- [x] Store result configuration and show it with partial/completed results.
- [x] Test transitions between convergence and discrepancy sweeps and between
      available/unavailable metrics.

Evidence: `resetSweep`, `resetDiscSweep`, `runSweep`, and control listeners in
[analysis.js](examples/wasm-demo/analysis.js).

Acceptance: changing controls during a sweep never appends old results beneath
new settings, leaves a stale chart presented as current, or strands disabled controls.

Verification (2026-10-03): the real-Chrome regression fails against the previous
controller with `stale convergence row after convDims`. The corrected controller
passes fifteen control-change cases, both panel-switch directions, an inactive
panel reset, Stop with retained partial configuration, restart without mixed
results, and unavailable/available metric transitions. Run
`node scripts/test-demo-browser.mjs /path/to/built/demo` after building the demo;
the dependency-free CDP runner uses Chrome, bounded startup/request/overall
deadlines, a private profile, and cleanup. Verified with Chrome 144.0.7559.109,
Node 18.19.1, and Go 1.26.1 WASM. Full two-page/error/accessibility coverage and
CI integration remain under DEMO-06/DEMO-07.

### DEMO-02 — Fix dimension-dependent values and source descriptions (P1)

- [x] Compute the Gaussian explanatory exact value for the selected dimensions;
      use the authoritative returned value or a dimension-aware metadata export.
- [x] Preserve the chart's already-correct `converge().exact` calculation.
- [x] Give Halton and Sobol source-specific descriptions for the unrandomized
      option instead of attributing Halton's high-prime defect to both.
- [x] Reconcile the stale five-seed comparison, pair-correlation figures, and
      first-pass claims with the documented sample counts and prime bases.

Evidence: [info.go](examples/wasm-demo/info.go),
[converge.go](examples/wasm-demo/converge.go),
[analysis.js](examples/wasm-demo/analysis.js),
[demo README](examples/wasm-demo/README.md).

Acceptance: Gaussian notes and result readouts agree at dimensions 1, 4, and 32;
source descriptions and displayed comparison figures agree with reproducible evidence.

Verification (2026-10-03): `info({dims})` now uses the same clamp and exact
function as `converge()`. The extended Chrome regression fails against the
previous controller with `stale Gaussian note 1` and passes at dimensions
1/4/32. An independent 10000-interval Simpson integral checks the Gaussian
reference to relative tolerance 1e-12; metadata, note, and result readout agree.
Source-specific unrandomized descriptions are checked in browser. Historical
five-seed and cost/accuracy figures were removed from demo descriptions rather
than treated as current guarantees; the live correlation result names its full
configuration and directs readers to the thirty-seed reproduction methodology.
The base-163/167 explanation now correctly distinguishes leading-digit cycles
from poorly explored higher digits. Existing sweep browser regressions and
explicit js/wasm vet pass. Core sequence outputs and convergence math are unchanged.

### DEMO-03 — Bound work on the browser main thread (P2)

- [x] Debounce expensive controls and use reduced sampling while dragging.
- [x] Evaluate a worker-hosted WASM instance for scatter, correlation, convergence,
      and discrepancy work, with request IDs and cancellation between bounded chunks.
- [x] Until workers are available, cap per-call work using the selected scheme,
      dimensions, sample count, and measured device behavior; test nested scrambling
      at the currently legal large configurations.
- [x] Avoid treating animation-frame coalescing or yielding between large calls
      as a guarantee that Stop can interrupt an individual computation.

Evidence: [app.js](examples/wasm-demo/app.js),
[analysis.js](examples/wasm-demo/analysis.js),
[points.go](examples/wasm-demo/points.go),
[correlate.go](examples/wasm-demo/correlate.go).

Acceptance: interactive controls and cancellation remain responsive under the
supported maximum workloads; measured budgets and worker/partial-result behavior
are recorded, including behavior on a constrained device.

Verification (2026-10-03): the four heavy exports now run in dedicated WASM
workers. One outstanding request per channel is identified and invalidated on
control changes; cancelling an active request terminates its worker, resolves
its pending result to null, and permits a fresh instance. Idle instances are
reused. Transferred typed buffers preserve selected scatter coordinates against
independent indexed-export results, without detaching displayed buffers.
Scatter and correlation sliders debounce for 80 ms, use 64 nested / 256 other
preview points, and request the full selected budget after 350 ms. Twenty rapid
scatter changes produce exactly one preview and one full request. No temporary
scheme-dependent reduction of the legal full budgets is needed now that workers
are used. Sweep charts show completed rungs; interrupted partial rungs are discarded.

Real Chrome 144.0.7559.109, Go 1.26.1, linux/amd64, Intel i7-1255U:
`QMC_BROWSER_CPUS=0 just test-browser` restricts all Chrome threads to one CPU.
At normal / 6× DOM throttling, measured call time and maximum 25-ms UI timer gap:

| Workload (nested Halton except CD2 calculation)   | Call time, normal / 6× | Largest timer gap, normal / 6× |
| ------------------------------------------------- | ---------------------- | ------------------------------ |
| Scatter, 64 dimensions, 20,000 points             | 1.839 / 5.185 s        | 32.5 / 34.8 ms                 |
| Correlation, 48 dimensions, 5,000 points          | 0.266 / 0.595 s        | 28.2 / 27.4 ms                 |
| Convergence export, 32 dimensions, 200,000 points | 7.772 / 18.486 s       | 28.4 / 74.6 ms                 |
| General CD2, 39 dimensions, capped 1,142 points   | 0.223 / 0.759 s        | 25.6 / 32.2 ms                 |

The actual Stop control during the UI's maximum 65,536-point nested convergence
rung responds in 1.5 ms at 6× DOM throttling; restart rejects obsolete results.
The constrained suite passes in 48.81 seconds, including worker loading failures,
all eight asset failure/reload cases, and zero unexpected browser errors.
These are measured workloads on a constrained execution profile, not universal
mobile-device throughput guarantees. Prior direct DOM calls blocked for about
3.1 / 17.2 seconds for maximum scatter and 3.2 / 17.5 seconds for 65,536-point
convergence at normal / 6× throttling on the same host. The worker/client deadlines
and exact reproduction commands are documented in the demo README. A final
unrestricted run against the formatted current sources also passes (23.828 s);
changed JavaScript parses, the diff is clean, and fixture-tagged js/wasm vet passes.

### DEMO-04 — Validate typed-array output buffers (P1)

- [x] Validate typed-array kinds, byte capacities, shared backing buffers,
      offsets, and requested lengths before reusing a caller-provided sink.
- [x] Check the byte count returned by `js.CopyBytesToJS`.
- [x] Reject or safely replace mismatched, detached, and undersized views.

Evidence: `sinkFor` and `float32Sink.write` in
[marshal.go](examples/wasm-demo/marshal.go).

Acceptance: malformed output pairs cannot return plausible unchanged or partially
written floats; the ordinary matched-buffer reuse path remains correct and efficient.

Verification (2026-10-03): the extended real-Chrome test fails against the
previous WASM build with `malformed sink produced invalid floats`. Fifteen
buffer cases now pass: matched nonzero-offset point output with untouched
prefix/tail, matched correlation output, twelve malformed/detached/undersized
pairs, and a SharedArrayBuffer pair. Invalid pairs receive fresh ordinary
ArrayBuffers, while valid pairs retain buffer identity and exact payload length.
The test server enables cross-origin isolation to actually exercise shared
buffers. `write` checks both sink capacity and the exact CopyBytesToJS count;
an incomplete copy becomes a guarded failure rather than successful floats.
Existing sweep/Gaussian browser checks and explicit js/wasm vet pass.

### DEMO-05 — Distinguish recovered request failures from runtime termination (P1)

- [x] Do not permanently mark the instance dead solely because a callback panic
      was recovered. Determine whether subsequent safe calls remain usable.
- [x] Provide an explicit reset/reload action and coherent disabled controls when
      the runtime has actually terminated or cannot be trusted.
- [x] Make documented fallback behavior consistent for missing, null, malformed,
      and nonfinite options, including default calls that later access `opts.Get`.
- [x] Test a rejected request followed by a valid request and a genuine runtime
      termination separately.

Evidence: [bridge.go](examples/wasm-demo/bridge.go),
[points.go](examples/wasm-demo/points.go),
[app.js](examples/wasm-demo/app.js), [analysis.js](examples/wasm-demo/analysis.js).

Acceptance: recoverable failures remain recoverable; terminal failures have a clear
recovery path; default/malformed option handling matches its documented contract.

Verification (2026-10-03): Chrome regressions fail against the previous bridge
with `default/fallback mismatch points`, and against the previous controllers
with `valid sweep after panic`. All eight exports now match their default object
results for missing/null/non-object options, arrays, malformed fields, and
nonfinite numeric fields (56 comparisons, independent of JS property order).
An unknown integrand is explicitly rejected and a subsequent valid request works.
A tagged test-only WASM binary produces a recovered Go panic and invokes actual
`os.Exit(0)`: both pages remain usable after recovery, disable every computation
control after exit, expose Reload WebAssembly, and return to usable fresh instances
when it is clicked. The runner checks that production has no fixture exports.
Existing sweep, Gaussian, and buffer browser checks pass; js/wasm vet with the
fixture tag passes. The shared runtime monitor also handles rejected run promises
and WASM traps without conflating them with recovered request failures.

### DEMO-06 — Add behavioral demo verification (P1)

- [x] Introduce a small browser smoke suite and a deterministic local test server
      with bounded startup/readiness waits and cleanup on failure.
- [x] Cover both pages, source/randomization switching, Gaussian dimensions,
      sweep control changes, Stop/restart, unavailable metrics, error recovery,
      typed-array transfers, and loading failures.
- [x] Test pure helper functions where useful; extract testable Go logic from
      js/wasm-only files when it reduces duplication or enables meaningful tests.
- [x] Capture browser errors and fail the smoke suite on unexpected console/runtime
      errors. Add the suite to PR validation before Pages deployment.

Evidence: [demo module](examples/wasm-demo/go.mod),
[test workflow](.github/workflows/test.yml),
[Pages workflow](.github/workflows/wasm-demo-pages.yml).

Acceptance: runtime boot and key user flows are verified in a real browser;
broken WASM/static assets and the identified result-consistency bugs fail CI;
startup polls cannot hang indefinitely.

Verification (2026-10-03): `just test-browser` builds in an owned temporary
directory, compiles the tagged runtime fixture, runs Chrome, and cleans up.
`just test-browser /path/to/site` checks an existing production artifact with
the same fixture; this run passed in 5.790 seconds. The CDP runner has a five-second
server startup deadline, thirty-second page readiness deadlines, twenty-second
request deadlines, and a 120-second overall browser deadline. It checks both
pages, all six Point Lab source/randomization combinations, transport/scrubbing,
fifteen sweep-setting changes and transitions, Gaussian dimensions 1/4/32,
fifteen buffer cases, eight exports' option fallbacks, rejected requests,
recovered Go panics, actual runtime exit/reload, and six missing/corrupt asset
recovery cases. The shared runtime helper is exercised through both production
controllers and the Go fixture rather than a duplicated mock implementation.
Unexpected console, runtime, resource, and third-party request errors fail the
runner; an isolated `console.error` mutation proves the console gate fails.
Only the deliberately injected missing-asset URLs are exempt during their
specific test phase. PR validation and Pages artifact upload now require this
same command. YAML parsing, shellcheck, shell formatting, and diff checks pass.
Accessibility and constrained-device responsiveness remain DEMO-07/DEMO-03.

### DEMO-07 — Make heatmap and progress information accessible (P2)

- [x] Provide keyboard cell navigation and a textual matrix/table or equivalent
      accessible values for the correlation heatmap.
- [x] Add dynamic canvas summaries and an accessible explanation of its legend.
- [x] Add progressbar roles, names, and current values to both progress indicators.
- [x] Keep reduced-motion behavior, visible focus, and throttled live announcements.

Evidence: [analysis.html](examples/wasm-demo/analysis.html),
[analysis.js](examples/wasm-demo/analysis.js), [style.css](examples/wasm-demo/style.css).

Acceptance: keyboard-only and screen-reader users can inspect correlation values
and follow sweep progress; automated accessibility checks and manual keyboard
verification cover both pages.

Verification (2026-10-03): the full correlation matrix is exposed as an
expandable, labelled data grid with zero-based row/column headers and one tab
stop. Real Chrome Input events verify Enter expansion, Tab entry/exit, every
arrow direction, boundaries, Home/End and Control+Home/End, a two-pixel visible
focus outline, and the synchronized heatmap readout. Sixteen values at four
dimensions and all 2,304 values at the maximum 48 dimensions match independent
Go-export results to their displayed precision. Chrome's accessibility tree
exposes the grid, named gridcells, three described Bench images, and both named
progressbars with current/minimum/maximum values and status text. Point Lab's
two described images, named controls, and native keyboard reveal slider are
also checked. Canvas summaries change with computed results and are cleared
on invalidation. Sweep progress counts retained completed rows, describes Stop
and completion, and resets to zero on configuration changes.

Reduced-motion progress transitions and Point Lab playback are verified in
Chrome. Polite atomic announcements are capped at one per 700 ms; a regression
confirms a burst preserves its final message instead of discarding it. Both
sliders' debounce tests pass, including correlation's [64, 1019] preview/full
requests. The new regression fails against the preceding artifact without the
keyboard explorer. Full browser verification passes with zero unexpected errors;
manual keyboard-check steps and WAI references are in the demo README. These
checks cover actual browser keyboard input and accessibility semantics, rather
than claiming a full WCAG audit or tests with every screen-reader/browser pair.

### DEMO-08 — Remove unnecessary external font requests (P2)

- [x] Use system fonts or self-host the chosen fonts with their required notices.
- [x] Document the demo's actual network behavior and distinguish it from the
      dependency-free, network-free sequence-generation library.

Evidence: [index.html](examples/wasm-demo/index.html),
[analysis.html](examples/wasm-demo/analysis.html), [toolchain docs](docs/toolchain.md).

Acceptance: after local assets load, the demo needs no third-party requests for
fonts or computation; existing absence of analytics and data submission is preserved.

Verification (2026-10-03): the Chrome network gate fails against the previous
HTML and records Google Fonts CSS and Archivo/JetBrains Mono font requests.
After removing both pages' external links and using system font stacks in CSS
and canvas labels, the complete existing browser regression passes with zero
third-party requests across both pages and reloads. No fonts are bundled, so
no new font notices are needed. Demo README and toolchain documentation explain
same-origin asset downloads, local computation, and explicit external-link
navigation separately from the network-free Go library. DEMO-08 was completed
before DEMO-06 because its third-party requests would undermine a clean network
and console-error validation gate.

### DEMO-09 — Stop idle animation wakeups (P2)

- [x] Schedule animation frames only while playing or when a redraw is needed.
- [x] Cancel pending frames on pause and manage background-tab behavior.

Evidence: [app.js](examples/wasm-demo/app.js).

Acceptance: a paused, unchanged page has no perpetual animation loop; play, pause,
scrubbing, and reduced-motion flows continue to work.

Verification (2026-10-03): the real-Chrome regression fails against the previous
controller with `paused page still schedules animation frames`. The corrected
controller passes callback-count checks for idle and paused periods, confirms
Play schedules frames again, and cancels its pending frame on Pause. Opening
and activating a second real browser tab makes the first hidden and pauses its
playback; returning does not skip forward using time spent in the background.
With emulated reduced-motion preference, Play reveals all points statically
and remains paused. Scrubbing, normal playback, runtime termination, and all
existing smoke checks pass together (6.802 seconds). Redraws still occur in
response to control/resize/DPR changes; no idle animation loop remains.

### DEMO-10 — Resolve smaller demo maintenance and rendering gaps (P3)

- [x] Evaluate extracting the duplicated page WASM loaders into a shared helper.
      Preserve progress reporting, streaming fallback, errors, runtime termination,
      and reload behavior; record the decision if extraction adds little value.
- [x] Add a digit-inspector regression comparing its duplicated raw-index and
      base-digit calculations with actual Halton coordinates for skips and leaps,
      including supported randomizations and boundary/error cases. Evaluate shared
      helpers without exporting library internals solely for the demo.
- [x] Reconcile the HTML DOM-contract comments with controller selectors and the
      accessible controls added since the review.
- [x] Profile heatmap and legend redraws during hover. Evaluate retaining the
      unchanged rendering layer; require invalidation on theme, data, resize, and
      device-pixel-ratio changes for any added cache. Record the existing
      theme-invalidation behavior and the decision if caching is unnecessary.

Evidence: [Point Lab controller](examples/wasm-demo/app.js),
[analysis controller](examples/wasm-demo/analysis.js),
[digit inspector](examples/wasm-demo/digits.go),
[renderer](examples/wasm-demo/render.js),
[Point Lab markup](examples/wasm-demo/index.html),
[analysis markup](examples/wasm-demo/analysis.html).

Acceptance: both pages retain their browser-verified loader and failure behavior;
digit descriptions agree with the library; documented DOM contracts match the
markup and selectors; rendering changes have measured benefit and correct cache
invalidation, or a recorded decision explains retaining the current approach.

Verification (2026-10-03): implementation committed as `76f7e2f`. The identical
page loaders now use `WasmRuntime.load`; offline regressions cover byte progress,
unknown lengths, non-reader/reduced-motion paths, missing streaming support,
and fetch/read/instantiation failures without replacing browser globals. HTML
contracts now include recovery and accessible summary/grid controls. The same
regression checks exact documented IDs and literal controller selectors.

The runtime fixture verifies 36 digit configurations against separately
constructed library generators and independent plain/fixed-digit expansions,
including leading-zero tails, skip/leap, both seeds, all three offered
randomizations, maximum indices/dimensions, clamping, and seven refused requests.
These execute before the browser fixture's long-lived runtime starts. Duplicated
base expansion stays private to the demo; it illustrates digits without exporting
library internals or recomputing the reported library coordinate.

Real Chrome checks pass on the production artifact and fixture (29.513 s,
zero unexpected errors), retaining reduced-motion, panic/exit/reload, missing
asset, cache-coherence, accessibility, and worker checks. At 48 dimensions, 100
draws after ten warmups measured heatmap median/p95 of 1.115/2.070 ms at DPR 1
and 0.830/1.020 ms on the larger DPR-2 canvas; legend medians were 0.115/0.155 ms.
The measurement scope and host are recorded in the demo topic. Retained bitmap
caching is deferred because these command-submission costs are modest and a cache
would add data/size/theme ownership. Existing theme invalidation, resize/DPR
redraws, and the absence of a theme-switching control are documented; browser
checks verify changed theme colours and scaled backing stores. Explicit WASM
vet/build/tidy/verify, demo lint, pinned formatting (99 files, zero changes),
and diff checks pass.

## Tooling and CI

### TOOL-01 — Make formatting checks strict and reproducible (P2)

- [x] Pin gofumpt, gci, shfmt, prettier, shellcheck, and treefmt versions.
- [x] Install every required tool and stop swallowing prettier setup failures.
- [x] Remove permissive missing-formatter behavior from required CI checks.
      An explicitly optional local mode may remain if documented.
- [x] Run shellcheck as an explicit diagnostic gate with its exit status respected.
- [x] Verify checks fail when a required formatter is unavailable and when a
      representative Go, Markdown, YAML, JavaScript, CSS, HTML, or shell file is invalid.

Evidence: [justfile](justfile), [treefmt.toml](treefmt.toml),
[test workflow](.github/workflows/test.yml).

Acceptance: successful required checks mean every configured file type was checked;
a fresh installation produces the same results as CI.

Verification (2026-10-03): all formatter versions are pinned in the tracked
tool environment and installed through TOOL-02. `just fmt` and
`just check-formatted` require those exact versions; no required path allows
missing tools or suppressed npm failures. ShellCheck is a separate exit-status
preserving gate, also exposed by `just check-shell`, rather than a non-writing
formatter whose diagnostics could be lost. Required treefmt invocations clear
local `TREEFMT_*` overrides, disable caches, reject missing tools, and cover
non-ignored tracked/new files, including the formerly omitted `.mjs` extension.

`just test-formatting` runs actual pinned formatters in owned temporary Git
worktrees. It verifies failure for malformed/unformatted Go, Markdown, JSON,
YAML, JS, MJS, CSS, HTML, and shell; undefined-variable ShellCheck diagnostics
fail independently of formatting changes. All six required tool executables
are made individually unavailable and each is rejected; version 3.5.30 cannot
stand in for pinned Prettier 3.5.3. Overrides attempting to select only one
formatter or exclude every file cannot weaken the check. Correct fixtures pass
before/after these mutations. Installer failure/reuse tests also pass again.
CI now provisions Node explicitly and runs the same failure gates. The current
whole-repository strict check passes: 94 files processed, zero changes, 1.426 s.
Explicit exclusions and tool prerequisites are documented in the toolchain topic.

### TOOL-02 — Harden and broaden development-tool installation (P2)

- [x] Detect supported OS/architecture combinations instead of hardcoding
      `linux_amd64`; document unsupported platforms clearly.
- [x] Verify downloaded archive checksums before extraction and installation.
- [x] Prefer a user-owned installation directory over unconditional privileged
      extraction; document PATH setup and required host prerequisites.
- [x] Verify an existing tool's version rather than accepting any binary on PATH.

Evidence: `setup-deps` in [justfile](justfile).

Acceptance: setup validates installed versions, handles supported amd64/arm64 and
macOS/Linux environments consistently, and refuses unverifiable downloads.

Verification (2026-10-03): `just setup-deps` now invokes a tracked installer
with one version source, exact Go tool modules/compiler, integrity-locked npm
installation, and eight committed archive SHA-256 pins from official release
asset digests. It selects Linux/macOS amd64/arm64 assets, checks archive hashes
before extracting the named binary, validates compiled/downloaded versions,
and replaces missing or mismatched tools in a dedicated user-owned directory.
Matching PATH tools are reused. No sudo or global npm install is used. Directory
ownership/symlink checks and absolute-path validation precede installation.

`just test-tool-setup` passes offline fixtures for all four platform mappings,
real hash checking and archive extraction, replacement/reuse, corrupted-download
refusal before extraction or replacement, wrong compiled versions, npm failure
propagation, unsupported OS/architecture, and relative destination refusal.
CI now runs these contracts. A real Linux amd64 run in an owned temporary tool
directory deliberately shadowed every required tool with version 0.0.0: both
upstream archives passed their committed checksums, all exact Go tools installed,
and npm installed locked Prettier. A second public-recipe run reused those exact
versions without reinstalling. ShellCheck with external sources, shell formatting,
and diff checks pass. Other platforms have verified asset routing and checksum
pins but were not executed natively on this Linux host. Setup prerequisites,
PATH, version provenance, upgrade steps, and this limitation are documented.
TOOL-02 precedes TOOL-01 because required version enforcement needs a trustworthy
installation path; strict formatter and missing-tool failure gates remain TOOL-01.

### TOOL-03 — Cover the nested module and align local verification with CI (P2)

- [x] Add explicit demo-module tidy checks, js/wasm vet, suitable linting, and
      behavioral tests. Native stub compilation does not validate the WASM code.
- [x] Replace the blanket examples lint exclusion with narrowly justified rules
      for demo production code versus actual teaching examples.
- [x] Define clear local commands for routine CI checks, slow statistical checks,
      WASM checks, and release validation; share those commands with workflows.
- [x] Retain executable 386 testing and supported Go-version coverage. Test a
      deliberate publishing toolchain separately from minimum-version compatibility.
- [x] Remove references to untracked `.trunk` as part of the reproducible setup,
      and update obsolete comments claiming the suite cannot compile for 386.
- [x] Make script arguments, including demo output location, reachable through
      the corresponding just recipe.

Evidence: [.golangci.yml](.golangci.yml), [justfile](justfile),
[test workflow](.github/workflows/test.yml), [toolchain docs](docs/toolchain.md).

Acceptance: local commands and workflows enforce the same documented checks for
both modules; minimum-version compatibility and publishing-toolchain choices are
explicit; no required configuration exists only in one developer's clone.

Verification (2026-10-03): verify/tidy checks cover both modules. The WASM
recipe compiles production code, explicitly vets js/wasm with the runtime-fixture
tag, and separately builds the native stub. A new lint recipe applies the root
rules to production WASM and fixture code; removing the blanket examples
exclusion exposed and resolved ten findings (redundant initialization, two unused
helpers, seven whitespace diagnostics). There is no teaching-example exemption
because the only current example is the production demo. Node syntax/behavior
is covered by strict JS/MJS formatting and the existing real-Chrome suite.

`just check` is the fast local path; `just ci` is the shared routine verification
including race, browser, and tool-gate regressions. Statistical and optional full
statistical-race recipes remain explicit. Workflows use these same individual
recipes. The compatibility matrix keeps executable 386 and Go 1.23/1.24/1.25,
adds the publishing Go 1.26.1, and compiles/vets the nested module for each.
`tools/go-version` is the tracked publishing/development compiler source used
by setup, builds, browser checks, and workflow jobs, independently of minimum
compatibility. Untracked Trunk configuration is no longer required or referenced
as project setup. Stale claims that 386 cannot compile are removed.

Current local evidence: shared `just ci` passes both module checks and lint,
ordinary race contracts (29.785 s), the full browser regression (33.119 s,
zero unexpected errors), installer tests, and formatter failure tests. Nested
verify/tidy/build/vet/stub checks pass on Go 1.23.0, 1.24.0, 1.25.0 and 1.26.1.
Routine amd64 tests pass on Go 1.23.0/1.24.0/1.25.0 and race tests on 1.26.1;
executable 386 tests pass on all four. Isolated snapshots prove a compile-valid
printf defect fails WASM vet and nested go.mod drift fails the shared tidy gate.
`just --justfile /path/to/justfile build-wasm-demo /path/with-spaces` forwards
its destination correctly from a different invoking directory and produces the
WASM/pages/worker assets there. Workflow YAML parses and final strict formatting
passes. These are local results and checked workflow definitions, not a claim
that remote GitHub jobs have run. Release policy/artifacts remain TOOL-04/SHIP.

Follow-up audit (2026-10-03): a broad recipe edit had inadvertently changed the
root tidy check to a write and duplicated the demo check. Restored one `tidy
-diff` per module. `just test-module-gates` now proves both root and demo drift
fail without modifying go.mod or creating go.sum, and preserves the explicit
compile-valid WASM vet mutation. It rejects the preceding committed root recipe
and passes the corrected one. Routine CI and the WASM workflow require it.

### TOOL-04 — Pin workflow dependencies and match release validation to promises (P2)

- [x] Pin GitHub Actions to reviewed commit SHAs, retaining readable version
      comments and Dependabot updates.
- [x] Keep permissions minimal per job, limiting Pages write permissions to jobs
      that need them.
- [x] Ensure release validation includes the required race, statistical, module,
      WASM, browser, and artifact checks with explicit budgets.
- [x] Keep version validation and local release commands consistent and verify
      releases correspond to clean, reviewed source and documented changes.

Evidence: [test workflow](.github/workflows/test.yml),
[release workflow](.github/workflows/release.yml),
[Pages workflow](.github/workflows/wasm-demo-pages.yml),
[Dependabot](.github/dependabot.yml), [justfile](justfile).

Acceptance: workflow dependencies change through reviewable updates; release
checks match the documented support and verification claims; permissions remain
limited to their intended jobs.

Verification (2026-10-03): implementation committed as `5c1cdda`. Eight external
Actions use full upstream-verified commit SHAs with readable major-version
comments and weekly Dependabot updates. Reviewed action metadata exposed a
floating nested uploader in the Pages composite; a verified regular-file TAR
and directly pinned uploader replace it. Every job has a deadline. Workflow
defaults are read-only, checkout credentials are not persisted, and only the
Pages deployment job receives Pages/OIDC write permissions. Just 1.21.0 is
explicitly selected, matching the locally exercised recipe syntax.

Shared local/workflow release policy validates strict ASCII SemVer, an exact
nonempty changelog section, module paths, a clean worktree, and the full reviewed
HEAD SHA before and after the required gates. Existing versions cannot identify
different source. The SHA is the maintainer's explicit review attestation;
these checks do not query or substitute for human PR approval. Tag validation
binds an annotated Reviewed-Commit trailer, checkout, workflow event, and main
ancestry. Local tag creation additionally checks remote main before/after the
gates; no tag was created in this repository. Literal recipe arguments close a
shell injection reproduced against the preceding committed recipe. Required
checks share a 25-minute total deadline within the 30-minute workflow job and
clean up descendant processes on timeout or failure.

`just test-release-gates` passes offline private Git fixtures covering malformed
versions, literal shell payloads, metadata/source drift, stale version tags,
tag attestations, event identity, main ancestry, minimal permissions, floating
dependencies, and deadline cleanup. Archive tests verify exact inventory,
round-trip extraction, required notices, and preservation of caller archives.
A real archive of the SHIP-02 artifact contains 22 regular files and validates
after extraction. No remote workflow, upload, or deployment is claimed executed.

The shared `just release-verify` passes both modules' verify/tidy/lint/build/vet,
routine race contracts (28.986 s), full statistical/ordinary tests (53.997 s),
the actual production Chrome suite (26.933 s, zero unexpected errors), artifact,
installer, formatter, module-drift, and release regressions. The subsequent
version-to-source regression passes its focused fixture suite; final pinned
formatting processes 97 files with zero changes, and diff checks pass.

## Build artifacts and third-party materials

### SHIP-01 — Produce clean, versioned demo artifacts safely (P2)

- [x] Build in a fresh staging directory and publish the resulting complete set
      atomically or with an equivalent deployment boundary.
- [x] Validate output destinations and avoid deleting or overwriting unrelated
      caller-owned content when implementing cleanup.
- [x] Include an explicit recursive asset policy or manifest covering future
      images, fonts, JSON, and subdirectories, as well as existing assets.
- [x] Version/hash HTML references to scripts, styles, WASM, and its compiler-matched
      runtime so a returning visitor cannot mix incompatible builds.
- [x] Add artifact checks for expected pages, linked assets, and absence of removed
      assets; verify paths work from different invoking directories.

Evidence: [build script](scripts/build-wasm-demo.sh),
[Pages workflow](.github/workflows/wasm-demo-pages.yml),
[app.js](examples/wasm-demo/app.js), [analysis.js](examples/wasm-demo/analysis.js).

Acceptance: consecutive builds cannot retain deleted assets; all references resolve
within a consistent build; output handling is safe for caller-owned directories;
the runtime and WASM always originate from the same toolchain.

Verification (2026-10-03): implementation committed as `70e72fc`. Builds use
fresh sibling staging, exact ownership/inventory hashes, a bounded managed lock,
and native atomic directory exchange. A full-content build namespace binds
all payload bytes and compiler metadata; public pages select it through a
relative base URL. The runtime comes from the compiler used for the WASM.

`just test-demo-artifact` passes offline regressions for fresh/existing concurrent
builds, stable content identity, recursive images/fonts/JSON, removed assets,
compile/runtime/compiler-change failures, unresolved references, protected paths,
spaces/different working directories, symlinks/nonregular assets, unrelated lock
content, caller files/edits, and caller edits racing publication. Five hundred
native Linux exchanges under a reader produce only complete old/new entries.
Edited preceding trees are retained instead of deleted after publication.

The public recipe invoked from `/tmp` with a spaced output path produced
build `eed774608ee86475d1d654da9b53abfd5fd0ec06581e46fccf28f6364dac7093`
with 15 verified files and Go 1.26.1. Its full Chrome suite passes on `/`
(26.555 s) and `/qmc/` (26.481 s), including a running old page during a
new-build deployment: the removed original namespace fails cleanly, never
serves new bytes, and Reload recovers into the new namespace. Both report
zero unexpected errors. Required pinned formatting/shell diagnostics and
Python/workflow syntax checks pass. CI requires artifact regressions; Pages
checks the exact artifact before browser verification/upload. Linux publication
was exercised natively; macOS exchange is implemented but not claimed tested
on this host. Unsupported exchange fails preserving the prior output. Notice
packaging remains SHIP-02.

### SHIP-02 — Include distribution notices and credits (P2)

- [x] Copy the Joe–Kuo copyright, conditions, and disclaimer into the demo output.
- [x] Link credits/notices from both pages and include the project license.
- [x] Include notices for any newly self-hosted fonts or other bundled materials.
- [x] Add a build-artifact assertion that required notices are present.

Evidence: [Joe–Kuo notice](third_party/joe-kuo/LICENSE.txt), [LICENSE](LICENSE),
[build script](scripts/build-wasm-demo.sh), [README](README.md).

Acceptance: source and built distributions contain the applicable third-party
materials and users can find them from the demo. This addresses the packaging
omission observed in review without claiming a legal determination.

Verification (2026-10-03): implementation committed as `5426420`. All four notices are copied byte-for-byte from the
project LICENSE, complete Joe–Kuo LICENSE.txt, and the build compiler's
LICENSE/PATENTS. The static credits page needs no WASM startup and is linked
from both interactive pages. No fonts were introduced; system fonts remain in
use, and adding other materials has an explicit notice/credits policy.

`just test-demo-artifact` requires each notice and credits page, rejects missing
or empty notice sources and broken page/notice links, and checks the copied
bytes. It also verifies upgrade of a fully hashed older managed site without
notices: ownership remains recognizable, but that site's distribution gate
fails until rebuilt. All earlier atomic publication/caller-preservation gates
continue to pass. The real Go 1.26.1 artifact contains 21 verified files with
build ID `ac46cf1a92419de83010cd7f231332c37822819fb4d6c6827ad069f28b32d672`;
all four bundled notices match their full source files exactly.

Chrome on `/qmc/` passes all existing checks and navigates through credits from
both pages, downloading all four notices and checking each SHA-256 against the
verified artifact manifest (30.219 s, zero unexpected errors). Shared `just ci`
passes both modules' tidy/verify/lint/build/vet, routine race contracts (40.150 s),
the root-path browser run including eight notice downloads (29.999 s), artifact,
installer, formatter, and read-only module regressions. Strict formatting checks
95 files with zero changes. Source and built distribution attribution is present;
this records packaging evidence rather than a legal determination.

## Documentation and maintainability

### DOC-01 — Consolidate measurements and remove contradictory claims (P2)

- [x] Correct `sqrt(E[CD2²])` versus `E[CD2]` terminology in package comments,
      demo documentation, metadata, and baseline labels.
- [x] Reconcile the nested-scrambling 8×/40× cost discrepancy and the stale
      five-seed/ten-stream figures with one reproducible measurement source.
- [x] Record hardware, toolchain, configuration, seeds, sample sizes, uncertainty,
      and generating commands alongside published measurements.
- [x] Audit blanket statements about independence, dimension limits, burn-in,
      Sobol alignment, convergence, and universal superiority after SCI-01 is complete.
- [x] Move historical variants and repeated machine-specific tables out of source
      comments where they obscure the implementation. Keep invariants, contracts,
      formulas, and relevant references near the code.

Evidence: [README](README.md), [sequence.go](sequence.go), [nested.go](nested.go),
[sobol.go](sobol.go), [discrepancy.go](discrepancy.go),
[documentation index](docs/README.md), [demo README](examples/wasm-demo/README.md).

Acceptance: public descriptions agree across source, docs, and demo; each retained
quantitative comparison identifies reproducible evidence and its limitations;
historical explanations do not masquerade as current guarantees.

Verification (2026-10-03): implementation committed as `4cfb311`. Source,
README, topic docs, demo metadata and displayed labels distinguish
`sqrt(E[CD2²])` from mean CD2. The saturation control now compares sampled
squared CD2 with its analytic squared expectation. Exact independent rational
references for identical marginals with different joint association give
CD2² = 127/576 and 25/144; the new public regression passes. Worked examples
retain their numbers with corrected RMS labels. Existing `analytic` fields
remain compatible; additive kind/label fields identify RMS or no reference.
The star panel hides its unavailable analytic legend/readout, and both pages
provide no-JavaScript guidance.

Current comparisons link the canonical PERF-01 report and raw environment/data
instead of repeating unsupported historical timings or seed rankings. Named
quality fixtures describe their actual seeds, sample windows, aggregation and
distinct MC policies. Unsupported historical numerical narratives are removed
from current source/guidance. Work-budget constants are labeled retained
policies rather than current duration calibrations. Independence, net occupancy,
unaligned windows, dimension/range limits, CD2 interpretation, and option
recommendations now state their assumptions. Refusal wording is corrected and
recorded in the changelog. A read-only token comparison confirms identical root
production code outside the manually reviewed diagnostic body; generated-point
algorithms and valid seeded values are unchanged.

Shared `just release-verify` passes both modules, lint/build/vet, routine race
(23.873 s), full ordinary/statistical tests (46.871 s), real Chrome (22.920 s,
zero unexpected errors), and artifact/tooling/format/module/release regressions.
After the final comment clarifications, fresh Chrome verification passes again
(23.627 s, zero errors) with 21-file artifact
`f717a6f7cfecbd38fe20cec62a1c25d4784479bb7ffa50efefb54ad711f4b33b`.
It checks RMS metadata/formula/labels, conditional reference hiding, and existing
loader, digit, capability, worker, numerical, accessibility, failure, cache and
notice behavior. Minimum Go 1.23.0 routine amd64/executable 386 pass
(5.425/11.408 s); its demo build/vet/stub/module checks also pass. Focused
scientific/example checks and the complete small-sample discrepancy fixture pass.
Pinned formatting checks 106 files with zero changes; diff checks pass.

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

- [x] Measure Halton and Sobol on the same machine/toolchain, using repeated runs
      and reporting allocation counts alongside throughput and constructor memory.
- [x] Document scrambled Halton construction cost at its call site, including
      high-dimensional memory growth and guidance to reuse generators.
- [x] Profile base-2 specialization, reciprocal-based arithmetic, and a bulk fill
      API before choosing optimizations; verify mathematical and reproducibility effects.
- [x] Evaluate bounded, immutable root/shallow-node permutation caches separately
      from caching every nested node. Replace the categorical claim that caching
      "cannot" work with the measured conclusion for each design.
- [x] Compare options by end-to-end work and relevant integration accuracy as well
      as per-point cost; retain the efficient Sobol recurrence and contiguous storage.

Evidence: [bench_test.go](bench_test.go), [sobol_bench_test.go](sobol_bench_test.go),
[nested.go](nested.go), [performance documentation](docs/performance.md).

Acceptance: performance comparisons share a reproducible baseline; each proposed
optimization is implemented with measured benefit or closed with a recorded
reason; allocation, concurrency, accuracy, and compatibility checks remain green.

Verification (2026-10-03): implementation committed as `75c2427`. The public
`just measure-performance NEW_DIRECTORY` recipe records five sequential repeats,
fixed 4096-index workloads, constructor allocations, 40-seed integration error
and work, source/environment hashes, and a separate CPU profile. Both campaigns
use Go 1.26.1 on the same i7-1255U, logical CPU 2, GOMAXPROCS=1. Raw before/after,
the cache recheck, full-tree counter, and cold Sobol observation are committed
under docs/measurements; the performance topic records exact commands, ranges,
seeds, uncertainty assumptions, and limits. Final-campaign Go source hashes
match the implementation. Other test suites and benchmarks were kept separate
from the timed campaigns; host CPU frequency was not locked.

The bounded eager root cache is implemented as immutable contiguous entries and
prefix offsets. It preserves every tested seeded value and child/tail path,
requires no sampling-time mutations, and caps digit tables at 64 KiB. Indexed
nested throughput improves from 15445 to 7202 ns median; the 40-stream integration
workload improves from 2.723 to 1.287 s with unchanged error metrics. Constructor
work/memory increases and is documented at the call site. Scratch allocations
at 97/98/100 dimensions remain 0/1/3. Bounded-memory, exact-value (including
maximum indices and 1000 dimensions), dimension-prefix, and shared-read
regressions verify the cache.

Shallow caching is feasible but deferred: its modest incremental gain has
overlapping timing ranges and materially higher construction/memory. Full-tree
caching is rejected separately using 1.284 node reuse and an estimated 381.9 MB
footprint excluding map buckets. Bit reversal changes 1019/4096 full-width test
values; reciprocal arithmetic changes boundary/reference values. Neither
replaces the reproducible arithmetic. Bulk loop medians differ by less than 2%
with overlapping ranges, so no batch API is added. Sobol's recurrence and
contiguous storage are retained. These decisions and end-to-end accuracy/cost
comparisons replace categorical optimization claims.

Shared `just release-verify` passes both modules, lint, routine race contracts
(24.790 s), full ordinary/statistical tests (43.303 s), real Chrome (28.638 s,
zero unexpected errors), and all artifact/tooling/release regressions. Go 1.23.0
routine amd64 and executable 386 suites pass (3.504/9.674 s), as do its explicit
WASM build/vet/module checks. Go 1.26.1 executable 386 passes (10.009 s), and
the separate routine race suite passes (24.172 s). Formatting checks 103 files
with zero changes; diff checks pass. DOC-01 still consolidates historical source
tables and the remaining documentation measurements; API-01 decides the other
public-surface proposals.

### API-01 — Resolve remaining API proposals explicitly (P3)

- [x] Evaluate optional capability metadata without unnecessarily expanding the
      minimal `Sequence` interface or duplicating library policies in the demo.
- [x] Document why `Option` uses an unexported settings type, and distinguish
      immutable options from options owning consumable readers.
- [x] Decide whether checked indexed-access helpers would benefit callers;
      preserve existing methods and document panic boundaries if no change is warranted.
- [x] Explain and test aligned power-of-two Sobol usage. Evaluate whether a raw-origin
      or aligned-block helper is needed; the current skip facility can already align
      later blocks, so a new API is not automatically required.
- [x] Review internal invalid-base guards and their intended preconditions after
      overflow fixes; remove or retain them with explicit reasoning.
- [x] Coordinate any bulk/workspace proposal with CORE-07 and PERF-01 rather than
      introducing multiple overlapping allocation APIs.

Evidence: [sequence.go](sequence.go), [options.go](options.go), [sobol.go](sobol.go),
[API design documentation](docs/api-design.md),
[choosing a sequence](docs/choosing-a-sequence.md).

Acceptance: each proposal has a recorded decision, compatibility implications,
and evidence for any added API; existing interfaces and deterministic valid-input
behavior remain stable unless an intentional change is documented.

Verification (2026-10-03): implementation committed as `f0289e7`. API design
records a decision for every proposal. Sequence retains its six methods; no
descriptor is added because instance configuration, constructor/table capacity,
and product workload limits have different meanings. The demo's copied 1024
library ceiling is removed while its existing 64-dimension offer stays the same.
The Go browser fixture validates all twelve offered sequence/randomization
endpoint combinations against actual constructors. Prime/digit inspection stays
on concrete Halton methods, and Go remains the validation authority.

Option's private settings type and nil-option policy are documented, retaining
the existing exported type; immutable value options and NewSobol's consumable
reader lifecycle are distinguished. Checked indexed helpers/signature changes
are deferred with explicit compatibility and arbitrary-Sequence limitations.
Documented panic boundaries include the separate very-large-index permuted-digit
reversal limit and potentially partial Into writes. A public-API regression
reproduces reversal refusal at a representable raw index and verifies that the
next stateful draw is still the first point.

Existing skip expresses later aligned Sobol blocks via q\*N-1; no raw-origin or
additional block helper is added. Thirty cases per architecture check blocks
1/2/3/17 and the final complete block, N=16/256, plain/shift/Owen, indexed versus
stateful agreement, and all aspect ratios of the known t=0 first projection.
They pass on amd64 and minimum Go 1.23.0 executable 386. The origin exclusion,
raw bounds, general 2^t occupancy, and Gray-order high-bit mapping are explicit.
Cheap private invalid-base/index guards are retained with constructor preconditions
and their existing direct regression. Bulk/workspace remains deferred using
PERF-01's overlapping timings and CORE-07's allocation/ownership evidence.

Shared `just check` passes both modules' verify/tidy/lint, explicit WASM
build/vet/stub, pinned formatting (105 files, zero changes), and routine tests
(3.629 s). Real Chrome passes production pages and the updated capability/runtime
fixture (29.421 s, zero unexpected errors), retaining digit, loader, accessibility,
worker, failure, cache, and notice checks. Focused public boundary/guard checks,
demo lint, and diff checks pass. Public signatures and valid seeded behavior
are unchanged; remaining historical claim/open-list consolidation stays DOC-01/02.

## Completion checklist

- [x] SCI-01 and CORE-01 through CORE-08 are resolved with regressions and accurate contracts.
- [x] TEST-01 through TEST-03 establish meaningful, bounded verification for supported targets.
- [x] DEMO-01 through DEMO-09 are verified in browser, accessibility, and failure-state checks.
- [x] DEMO-10 resolves the smaller maintenance gaps with regressions or documented decisions.
- [x] TOOL-01 through TOOL-04 run reproducibly in a clean environment for both modules.
- [x] SHIP-01 and SHIP-02 verify complete, consistent artifacts and notices.
- [ ] DOC-01 and DOC-02 reconcile every affected public claim and open-work list.
- [x] PERF-01 and API-01 have measured implementations or documented decisions.
- [ ] Final ordinary, required race, statistical, WASM, browser, formatting, lint,
      and release-artifact checks pass under the agreed budgets.
- [ ] Re-review category scores using evidence from the completed work; do not
      raise scores solely because checklist items were marked complete.
