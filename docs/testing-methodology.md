# Testing methodology

Tests distinguish API contracts, mathematical structure, finite seeded quality,
and actual browser behavior. Passing one category does not establish the others.
The remediation history and measured verification runs live in
[PLAN.md](../PLAN.md); current performance evidence lives in
[Performance](performance.md).

## Empirical gates and baselines

Smooth-product tests use conservative margins against Monte Carlo for their
specified integrand and sample budget. A measured advantage is logged rather
than pinned as an exact assertion. Other functions include nonlinear moments,
early/late interactions, reversed weights, a localized Gaussian, and a
discontinuous triangle. The broad sweep permits deterioration on difficult
functions; it does not require universal QMC superiority.

`mcRMSError` uses consecutive draws from one `math/rand` source seeded with 20240823. It is reproducible pseudorandom sampling. The canonical performance
campaign instead constructs a fresh source seeded with `20240823 + seed` for
each replicate. These are different finite baselines; their numerical ratios
must not be merged. Neither policy proves ideal independence.

Large-budget integration fixtures use 40 seeds. The small-sample fixtures use
200 seeds. RMS standard errors, where reported, apply the delta method to the
replicate squared errors under an independent-replicate model. They describe
finite-sample variability, not randomization bias or a confidence guarantee
for arbitrary integrands. The small-sample comparison permits Owen within
20% of the measured leader rather than requiring an exact winner.

Correlation tests examine each seed's worst adjacent-pair absolute Pearson
correlation at their specified window. Zero correlation does not imply
independence. Summary conventions are stated by the producing fixtures:
fixed-digit correlation uses the average of the middle observations and
nearest-rank p90; some nested/leap fixtures retain upper-middle or alternative
quantile indices. Do not compare rounded summaries as though these conventions
were identical.

## Negative controls and independent references

Plain Halton retains a high-base correlation negative control, and seeded MC
must fail the conservative QMC smooth-product gate. The CD2 saturation fixture
compares particular QMC and pseudorandom sets, then integrates the same sets;
it checks contrast for that workload rather than a dimensional impossibility.
For the analytic control, compare mean **squared** CD2 with `E[CD2²]`.
The root of that expectation is an RMS norm; mean CD2 need not equal it.

Independent references include direct anchored-box enumeration, numerical
integration of the centered-discrepancy definition, exact rational tensor
grids, slow radical inverses, and a test-only ideal nested-permutation Owen
implementation. Reference agreement covers the named configurations and
tolerances, not every finite-dimensional floating-point case.

Nonfinite values fail quality summaries and ratio predicates. Boundary tests
cover both integer widths, constructors, reset, cursor limits, huge permuted
digit reversals, discrepancy range errors, and short destination buffers.
Shared options and concurrent indexed readers run under the race detector.

## Conditional structure needs its own tests

A bijective digit transformation can preserve net occupancy while losing the
intended prefix conditioning. Dedicated tests exist for both schemes:
`TestOwenScrambleIsNested` and `TestNestedPermutationsDependOnThePrefix`.
Mutation checks recorded under TEST-01/02 in [PLAN.md](../PLAN.md) confirm that
removing conditioning fails these tests. A partial mutation can still escape a
structure test's sensitivity; deterministic seeded references and distribution
fixtures provide complementary evidence. None proves ideal joint independence
for a finite hash family.

## Reproduction and budgets

```bash
just test-fast                # routine contracts, -short, 3-minute Go timeout
just test-race                # routine contracts with race detector, 5 minutes
just test-statistical         # full ordinary/statistical suite, 10 minutes
just test                    # same full suite plus coverage
just test-race-statistical    # optional full statistical race audit, 40 minutes
```

The PR matrix executes routine tests on amd64 and executable 386 with the
supported Go versions. Scheduled/on-demand jobs run the full statistical suite;
release verification includes it. The optional full statistical race audit is
separate: successful routine race checks do not imply that audit was executed.
Budgets are limits rather than speed guarantees.

For specific logged quality evidence:

```bash
go test -count=1 -v -timeout=10m -run 'Test(IntegrationAcrossReferenceFunctionsAndBudgets|SmallSample.*|ScramblingBreaksHighDimensionalCorrelation|CenteredL2SaturatesAtThirtyNineDimensions)' ./...
```

Use [Performance](performance.md)'s measurement recipe for controlled timing
campaigns. Keep benchmarks separate from concurrent test workloads and record
the compiler, hardware, source revision, seeds, sample windows, repetitions,
aggregation, and uncertainty assumptions alongside published figures.

## Demo verification

The nested demo module has explicit WASM build/vet/lint gates, offline loader
and DOM-contract tests, and actual Chrome checks through `just test-browser`.
The runtime fixture checks digit expansions and constructor capabilities
against the Go library. Production-page checks cover numerical references,
typed-buffer ownership, config snapshots, cancellation, recovery/exit/reload,
asset/cache failures, notices, keyboard input, accessibility-tree semantics,
reduced motion, and unexpected network/console/runtime errors.

`just test-browser dist` checks a particular built artifact. Browser startup,
page readiness, and protocol requests have bounded deadlines; the browser run
has a two-minute budget. `QMC_BROWSER_CPUS=0 just test-browser` offers a
single-permitted-CPU profile on Linux. Automated Chromium accessibility checks
do not replace testing with assistive technology or other browser engines.
See the [demo README](../examples/wasm-demo/README.md) and
[toolchain](toolchain.md) for setup, manual inspection, and shared CI gates.
