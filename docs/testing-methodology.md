# Testing methodology

Every claim in this repository is meant to be re-runnable. That imposes a shape on the tests,
and the shape has a few rules that are not obvious until a test has already lied once.

## Gates assert ratios and orderings, never constants

`TestScrambledQMCBeatsMonteCarloAt39Dims` measures 19–28x depending on n, and asserts **5x**.
The 5x threshold is an empirical regression margin on that smooth product and those
budgets. It is not a universal QMC convergence guarantee, and independent samples can
occasionally beat it by chance. A gate pinned to the observed 19x would have less room
for seed variability or a different Go version’s `rand`. Use additional integrands and
negative controls when assessing broader quality.

The measured figures go into `t.Logf` rather than into an assertion, so a run still reports
them and a regression is visible before it is fatal.

## Baselines are seeded and shared

`mcRMSError` uses `rand.NewSource` with a fixed constant, never time or the global source, so
a failure is reproducible and a pass is not luck. The streams are consecutive draws from one
source rather than separately seeded generators: separately seeded ones can correlate, which
would flatter the baseline the test is trying to beat honestly.

## Negative controls

`TestUnscrambledStillShowsTheDefect` asserts the _unscrambled_ correlation is still at least
0.5 (it measures ~0.81). Without it, a change that quietly destroyed the measurement would
make the positive test pass more easily. `TestCenteredL2SaturatesAtThirtyNineDimensions` does
the same job for CD2: it asserts the random figure lands within 2% of the analytic
expectation, the QMC-vs-random gap is under 10%, _and_ that the same point sets still give a
5x integration advantage — so it proves something about the statistic rather than about the
points.

The suite once could not distinguish this library's output from pseudorandom noise. The
correlation test passes for `math/rand` (0.124, against a 0.25 threshold — better than the
real generator's 0.141). `integration_test.go` now pins QMC integration error against Monte
Carlo at 5x; the same substitution scores 0.9x and fails.

## Replicate counts and uncertainty

Correlation summaries now use thirty seeds and report the median, nearest-rank
p90, and worst of each seed's worst adjacent-pair absolute correlation. The
previous five-seed sample was too sensitive to re-instantiation of a scramble.
The current fixed-digit test at 39 dimensions and 600 points after skip 64
reports 0.0909 / 0.1139 / 0.1611 for those summaries.

The product integration tests use forty streams. Ten-stream comparisons between
similar schemes proved too noisy to support a ranking: two full-permutation
variants differing only in shuffle direction previously read 44.0x and 31.9x
on the same ten seeds. These historical values describe that old experiment;
they are not measurements of the current validation job.

`TestIntegrationAcrossReferenceFunctionsAndBudgets` adds seven independently
integrable cases: nonlinear moments, early/late interactions, decaying/reversed
weights, a localized Gaussian peak, and a discontinuous triangle. It uses forty
streams at 64/256/1024 points in 24 dimensions, sharing samples across functions.
Sobol cases use power-of-two blocks aligned through `WithSkip(N-1)`. MC is seeded
and shared; plain Halton is a negative control for high-base moment bias.

The sweep logs absolute RMS error and an estimated standard error of that RMS
summary (the delta method applied to replicate squared errors). These are
empirical summaries under an independent-replicate model, not bounds on the
seeded family's bias. The broad 3x-MC deterioration gate does not require universal
superiority on peaks or discontinuities. Only the two smooth low-order cases at
N=1024 require a conservative measured 2x margin. Existing smooth-product gates
continue to require 5x at their specified workloads.

Reproduce with `go test -count=1 -v -run
'Test(IntegrationAcrossReferenceFunctionsAndBudgets|ScramblingBreaksHighDimensionalCorrelation|ScrambledQMCBeatsMonteCarlo.*|ShiftedSobolBeatsMonteCarloAt39Dims|SobolAgainstHaltonAt39Dims|SobolBeatsMonteCarloAtLowDims|OwenBeatsDigitalShiftAt39Dims)' ./...`.
Do not infer a universal convergence rate or general ranking from these cases.

## A stratification test cannot police nesting

Measured twice, independently, on both scrambling schemes: a scramble that has stopped being
conditional on the digits above it still maps elementary intervals onto elementary intervals,
so it still produces a valid net. Removing both bit reversals from `owenScramble` leaves the
net-property test passing across all 1024 dimensions at m=4, 8 and 12, along with the
bijectivity, per-node injectivity and `Next`/`At` agreement tests. Only the dedicated nesting
test fails.

**Any future scrambling scheme needs a test that pins the conditional structure directly.**

## The nesting test has a sensitivity floor

Following on from that: hashing the node down to `node & 0xFF`, so roughly one node in 256
shares a permutation with another, leaves the nesting test passing. It was caught instead by a
chi-square over all 120 permutations of base 5 and by the golden-value test.

**A test that detects total loss of conditioning does not detect partial loss of it.**

## Reference implementations, not intuition

Where a closed form is easy to get subtly wrong, the test compares against something slower
and more obviously correct rather than against a hand-computed expectation:

- `starBruteForce` in `discrepancy_test.go` enumerates boxes directly.
- `integrateCenteredDiscrepancy` integrates CD2's definition numerically.
- `exactOwen` in `owen_uniformity_test.go` is a full nested-permutation Owen scramble, used as
  ground truth for the hash approximation.
- `robustness_test.go` carries slow reference forms of both radical inverses.

## Coverage

Coverage sits around 93%, and the uncovered statements are precisely the defensive guards.
That is the normal shape of coverage, not a target to chase — but the index-overflow bug lived
in exactly that region, so the guards deserve tests rather than a higher percentage.

## Known gaps

The reasoning above is settled; the suite does not yet act on all of it.

- **The demo module has no tests at all**, and it duplicates library logic, so nothing catches
  the two halves drifting apart. See [the WebAssembly demo](wasm-demo.md).
- **The defensive guards deserve tests.** The uncovered statements are precisely those guards.
  That is the normal shape of coverage, not a target to chase — but the index-overflow bug
  lived in exactly that region.

## Ranking and nonfinite safeguards

The large-budget small-sample comparison permits Owen within 20% of the measured
leader instead of forcing an exact winner. The historical top-two gap was well
within replicate uncertainty. This margin is an empirical regression policy,
not a statement that Owen must be optimal for arbitrary integrands.

RMS helpers, statistical ratios, and positive range predicates reject NaN/Inf.
Adjacent-correlation aggregation propagates a nonfinite value to its caller
instead of dropping it through a false max comparison. Reversal overflow beyond
the fuzz mapping is tested separately with a valid reverse permutation in base
167; that raw index fits int64 but not int32, where the test explicitly skips.

Conditional structure tests exist for both scramblers. Mutation checks on
isolated copies confirmed that removing Owen’s bit reversals fails
`TestOwenScrambleIsNested`, and removing nested child conditioning fails
`TestNestedPermutationsDependOnThePrefix`. Masking child-node inputs to eight bits
still passes the latter: it has a sensitivity floor. The seeded reference test
`TestNestedIsArchitectureIndependent` detects that partial mutation. Preserve
both direct structure tests and reproducibility/distribution references; none
alone proves ideal joint independence.
