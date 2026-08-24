# Testing methodology

Every claim in this repository is meant to be re-runnable. That imposes a shape on the tests,
and the shape has a few rules that are not obvious until a test has already lied once.

## Gates assert ratios and orderings, never constants

`TestScrambledQMCBeatsMonteCarloAt39Dims` measures 19–28x depending on n, and asserts **5x**.
A factor of five cannot be reached by any generator producing independent samples — the gap
between 1/n and 1/sqrt(n) convergence is structural — while leaving room for an unlucky seed,
a different Go version's `rand`, and future changes to the scrambling scheme that shift the
constant without giving up the rate. A test pinned at 19x would fail on noise; one at 5x fails
only if the package has stopped being a QMC package.

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

## Five seeds is not enough for the correlation statistic

A change that was a pure re-instantiation of the nested scrambling, not a change of scheme,
moved a five-seed worst case from **0.40 to 0.12**. `correlation_test.go` still uses five and
should quote a median and a tail over thirty, as the documentation does.

## Ten streams is not enough for the integration statistic

Two full-permutation variants differing only in the direction of a Fisher-Yates loop —
statistically identical constructions — read **44.0x and 31.9x on the same ten seeds**. The
forty- and eighty-stream figures separate the schemes consistently; the ten-stream figure does
not. The gates assert an ordering and a factor of five rather than any measured constant,
which is what keeps this from being a flaky-test problem — but **no ten-stream number should be
quoted as a comparison between two good schemes.**

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
