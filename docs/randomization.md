# Randomization

Options are mutually exclusive and apply to one generator. Constructors reject
incompatible randomization schemes; the last randomization option wins. Fix a
seed and configuration to reproduce a run.

| Option                 | Generator | Construction                                                               |
| ---------------------- | --------- | -------------------------------------------------------------------------- |
| `WithScrambling`       | Halton    | One seeded digit permutation per dimension, reused at every digit position |
| `WithNestedScrambling` | Halton    | Seeded permutations conditioned on preceding digits                        |
| `WithDigitalShift`     | Sobol     | One seeded 32-bit word per dimension, XORed into its coordinates           |
| `WithOwenScrambling`   | Sobol     | Hash-based nested bit flips, which need not be independent                 |

These transformations preserve elementary-interval structure to the implemented
digit depth. Their statistical assumptions differ.

## What seed variability can tell you

`WithScrambling` does **not** make each indexed point uniform. In one dimension
its first point is 0.5 for every seed: the binary permutations send `0.1000…`
to itself or `0.0111…`, which have the same value. A one-point estimate of the
integral of `x²` is always 0.25 rather than 1/3, with zero seed variance.
`TestFixedDigitScramblingDoesNotGiveUniformMarginals` guards this counterexample.

Independent uniform digital-shift words would give uniform marginals on the
32-bit grid. An estimate would be unbiased for that grid average, which can
differ from the continuous integral, particularly for discontinuous functions.
Here all words are derived from a finite seed.

Ideal nested scrambling uses independent uniform permutations at every node and
an infinite digit tail. Halton derives node seeds by hashing, uses Fisher–Yates
with rejection sampling, and stops the tail at float64 precision. Sobol uses a
32-bit hash permutation. Neither implementation guarantees ideal independent
node randomness or exact continuous unbiasedness. An unbiased shuffle under an
ideal random-word model does not establish independence of hashed nodes.

For independently selected random seeds, replicate estimates describe the
implementation's seed distribution. Their sample variance estimates that
variability; the standard error of the replicate mean is the sample standard
deviation divided by the square root of the replicate count. Consecutive fixed
seeds are reproducible test inputs, not a proof of independence. Confidence
intervals require a justified sampling model and enough replicates. They exclude
bias from finite precision, the hash family, or fixed-permutation scrambling.
Compare against independent integrals and increasing budgets too.

See [Owen's discussion of randomized QMC](https://artowen.su.domains/mc/practicalqmc.pdf)
and [Burley's hash-based construction](https://jcgt.org/published/0009/04/01/paper.pdf).
SCI-01's decision in [PLAN.md](../PLAN.md) retains the existing options and seeded
outputs. A new scheme would need an explicit randomness/precision contract and
independent moment, conditional-structure, and integration checks.

## Balance and accuracy limits

A base-2 `(t,m,s)`-net has `2^m` points and places `2^t` points in each dyadic
elementary interval of volume `2^(t-m)`. Only `t=0` gives one-point occupancy.
Projections inherit that guarantee and may have a smaller t; they are not all
t=0 nets. Digital shifts and nested dyadic scrambles preserve these counts and
t values. They cannot repair a direction table's poor occupancy, even when they
improve an empirical correlation or integral. See
[Sobol' sequences with guaranteed-quality 2D projections](https://perso.liris.cnrs.fr/nicolas.bonneel/paper_sobol.pdf).

Accuracy depends on the integrand, effective dimension, sample budget, and raw
block. The [controlled performance report](performance.md) is the canonical
40-stream comparison of error and cost. It supplies raw results, seed policy,
window, hardware, toolchain, and uncertainty estimates. Its MC streams are
separately seeded; some quality gates use consecutive draws from one fixed
source. Do not mix their error ratios.

## Halton digit scrambling

Plain Halton's coordinate in base p is the radical inverse of its raw index.
For indices smaller than p, it advances in steps of `1/p`; adjacent large-base
coordinates can therefore have strong correlations at small budgets.

`WithScrambling` maps digits through one Fisher–Yates permutation per dimension,
including the infinite leading-zero tail. Reusing that permutation preserves
elementary intervals and changes the ramps, but does not provide uniform point
marginals. The construction follows Braaten and Weller (1979), “An improved
low-discrepancy sequence for multidimensional quasi-Monte Carlo integration.”

`WithNestedScrambling` conditions each permutation on preceding input digits.
Points sharing a prefix receive the same prefix transformations. Fisher–Yates
replaces the earlier restricted affine family; the current bijection, shuffle,
prefix, and correlation regressions test the implemented construction. Historical
variant timings and rankings are not current guarantees.

`TestScramblingBreaksHighDimensionalCorrelation` uses 39 dimensions, 600 points,
skip 64, leap 1, and seeds 1..30. It reports the median of the two middle
observations, nearest-rank p90, and maximum of each seed's worst adjacent-pair
absolute Pearson correlation. `TestUnscrambledStillShowsTheDefect` is its
deterministic negative control.

`TestNestedCorrelationOverThirtySeeds` compares fixed and nested scrambling on
the same dimensions, window, skip, and seeds. Its current median is the upper
middle observation and it also reports the maximum; it does not compute p90.
These summary conventions differ, so their median figures are not interchangeable.
Both gates describe these finite fixtures, not all seeds or projections.

## Bounded caching and scratch

The implementation precomputes a bounded immutable prefix of root permutations.
Its digit-table payload is at most 64 KiB; deeper nodes remain lazy. Indexed
sampling does not populate a shared map or mutate the cache. The
[performance report](performance.md) records why root caching was implemented,
shallow caching deferred, and caching every visited node rejected. The full-tree
estimate does not rule out bounded caches.

Into methods avoid result-slice allocation. Nested Halton still uses per-coordinate
heap scratch above prime base 512: allocation regressions cover 97, 98, and 100
dimensions. [API design](api-design.md) records the deferred workspace decision.

## Assessing hash-based Owen scrambling

The structural tests establish nesting and bijectivity. The statistical sweeps
in `owen_uniformity_test.go` test empirical departures from an independent-bit
reference:

- `TestOwenHashFlipsAreFairAtEveryNode` and
  `TestOwenHashFlipsAreIndependentBetweenNodes` inspect depths 0..12 over
  40000 deterministically derived scrambling seeds (`owenSweepSeed`).
  Passing their finite tolerances does not prove fairness or independence.
- `TestOwenFlipsAreNotJointlyIndependentAcrossALevel` compares flip-count
  variability over 2000 derived seeds through depth 16, with the stored-bit
  reference measured through depth 12. Joint level behavior differs from the
  independent-bit model.
- `TestOwenApproximationCostsNothingOnIntegration` compares hash and reference
  scrambling on the smooth product at 39 dimensions, 4096 points, skip 64,
  and seeds 1..40. `TestOwenApproximationCostsNothingOnCorrelation` compares
  adjacent-pair correlations over seeds 1..30 at 600 points.

These are diagnostic instruments for changes to the hash. A measured difference
on the smooth product is not an upper bound on other integrands. Choose between
digital shifting, Owen scrambling, and Halton schemes using relevant error and
end-to-end cost measurements; the canonical comparison is in
[Performance](performance.md).

Reproduce the statistical fixtures with:

```sh
go test -count=1 -timeout=10m -v -run 'Test(ScramblingBreaksHighDimensionalCorrelation|UnscrambledStillShowsTheDefect|NestedCorrelationOverThirtySeeds|OwenHashFlipsAre.*|OwenFlipsAreNotJointlyIndependentAcrossALevel|OwenApproximation.*)' .
```
