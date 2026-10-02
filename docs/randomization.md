# Randomization

Each option applies to one generator and is refused by name by the other, rather than
ignored. They are mutually exclusive: a generator has one randomization or none.

| option                 | generator | what it does                                                          |
| ---------------------- | --------- | --------------------------------------------------------------------- |
| `WithScrambling`       | Halton    | One digit permutation per dimension (Braaten & Weller 1979)           |
| `WithNestedScrambling` | Halton    | Seeded digit permutation per node, conditioned on the digits above it |
| `WithDigitalShift`     | Sobol     | One random word per dimension, XORed into every point                 |
| `WithOwenScrambling`   | Sobol     | Hash-based nested bit flips; node flips need not be independent       |

All four leave the low-discrepancy structure intact — each maps elementary intervals onto
elementary intervals of the same size, to the implemented digit depth. They differ in their
randomization guarantees. Fix the seed and a run is reproducible.

## What seed variability can tell you

`WithScrambling` does **not** make each indexed point uniform. In one dimension its
first point is 0.5 for every seed: the two binary permutations send `0.1000…` to
either itself or `0.0111…`, which has the same value. A one-point estimate of the
integral of `x²` is always 0.25, while the integral is 1/3. Its seed variance is zero.
`TestFixedDigitScramblingDoesNotGiveUniformMarginals` guards this counterexample.

Digital shifts with ideal independent uniform words give uniform marginals on the
32-bit grid. This is unbiased for a grid average, which can differ from the continuous
integral, particularly for discontinuous functions. This implementation derives all
words from a finite seed rather than an independent entropy source for each coordinate.

Ideal nested scrambling uses independent uniform permutations at every node and an
infinite digit tail. Here Halton derives node seeds by hashing and uses Fisher–Yates
with rejection sampling; it truncates digits at float64 precision. Sobol uses a
32-bit hash permutation. Neither implementation guarantees ideal independent node
randomness or exact continuous unbiasedness. Uniform shuffle algorithms avoid modulo
bias under their random-word model; that does not prove independence of hashed nodes.

For randomly and independently chosen seeds, replicate estimates describe the
implementation's seed distribution. Their sample variance estimates that variability;
the standard error of the replicate mean is the sample standard deviation divided by
the square root of the replicate count. Fixed consecutive seeds are reproducible test
inputs, not a proof of independence. Confidence intervals need a justified sampling
model and enough replicates; they do not include bias from the grid, truncation, the
hash family, or fixed-permutation scrambling. Compare integrals with independent
references and increasing budgets too. See [Owen's discussion of randomized QMC](https://artowen.su.domains/mc/practicalqmc.pdf)
and [Burley's hash-based construction](https://jcgt.org/published/0009/04/01/paper.pdf).

No new option is introduced for an exact continuous-unbiased contract: the current
finite-output API cannot promise that for arbitrary integrands. Existing seeded
outputs are preserved. A future scheme would require an explicit precision and
randomness contract and independent moment, conditional-structure, and integration tests.

## Balance and accuracy limits

A base-2 `(t,m,s)`-net has `2^m` points and places `2^t` points in each dyadic
elementary interval of volume `2^(t-m)`. Only `t=0` gives one-point occupancy.
Coordinate projections inherit that guarantee and may have a smaller t; they are
not all t=0 nets. Digital shifts and nested dyadic scrambles preserve these counts
and t values. They cannot repair a direction table with poor occupancy, even when
they improve an empirical correlation or integral. See the definition in
[Sobol' sequences with guaranteed-quality 2D projections](https://perso.liris.cnrs.fr/nicolas.bonneel/paper_sobol.pdf).

The tables below describe their integrands, budgets, and seeds. They establish no
universal `1/N` rate or dimension-independent accuracy. Reordering important
coordinates, changing smoothness, or changing the block can change the result.

## The defect being cured

The Halton sequence places its _d_-th coordinate by the radical inverse in base _p_d_, the
_d_-th prime. For a large base the first _p_d_ points of that coordinate are simply
`0, 1/p_d, 2/p_d, …` — a ramp, not a sample — and two adjacent high-dimensional coordinates
ramp together.

Measured at 39 dimensions and 600 points, which is what a parameter search over 39 knobs on a
600-evaluation budget actually asks for. These are absolute Pearson correlations over adjacent
dimension pairs, over **thirty** seeds (median/p90/worst of per-seed worst adjacent pairs):

| configuration                   | median | p90   | worst     |
| ------------------------------- | ------ | ----- | --------- |
| unscrambled, skip 64            | —      | —     | **0.81**  |
| `WithScrambling`, skip 64       | 0.091  | 0.114 | **0.161** |
| `WithNestedScrambling`, skip 64 | 0.089  | 0.12  | **0.14**  |

Thirty rather than a handful because the statistic is high-variance: a change to the
scrambling that was a pure re-instantiation, not a change of scheme, moved a five-seed worst
case from 0.40 to 0.12. Quote the median and the tail, not one draw. See
[Testing methodology](testing-methodology.md).

`TestScramblingBreaksHighDimensionalCorrelation` and `TestUnscrambledStillShowsTheDefect`
keep both halves of that table honest — the second is a negative control, asserting the
defect still measures at least 0.5 without scrambling, so the comparison keeps meaning
something.

## `WithScrambling` — random-digit

One uniform permutation of the digit alphabet per dimension, applied to every digit of that
dimension's radical inverse (Braaten & Weller 1979), generated by seeded Fisher–Yates.
Within a dimension the same permutation applies at every digit position. A digit
permutation maps each elementary interval onto another of the same size, so the
low-discrepancy structure survives and the ramps do not.

## `WithNestedScrambling` — and why it changed

It generates a permutation **per node** of the scramble tree, conditioned on the digits
above it, using seeded pseudorandom words. Until recently it drew each node's permutation from the affine family
`x → ax+b mod p` rather than from all `p!`, which is free of shuffles but left a tail: worst
adjacent-pair |r| over thirty seeds of 0.37, against random-digit's 0.16.

The cause was understood rather than guessed. At 600 points a large-base coordinate varies
only in its first digit, where an affine map is a ramp of another slope rather than a
scattering, so two dimensions drawing commensurate slopes ramp together again.

Using Fisher–Yates permutations per node reduced the measured tail — worst over thirty seeds
0.14, now below random-digit's own 0.16 — at the cost of about a sixth of the integration
advantage (41.1x against 49.9x over forty streams) and about five times the price per point.
The trade is written out at the top of `nested.go`.

### The cache does not exist and cannot

The obvious optimisation is to memoise node permutations. It is not available, and the reason
is worth recording so nobody re-derives it. A 39-dimensional point at 4096 points visits
1,982,974 nodes, of which **1,544,674 are distinct**, because the leading-zero tail hangs a
fresh chain below every index and nothing in one is revisited. Distinct nodes grow with the
point count, not with the digit count — so a cache would buy 1.28x reuse for 382 MB, and
would cost `At` its documented freedom from locks.

The O(p) shuffle is avoided a different way: by evaluating only the digit asked for. That is
exact rather than approximate, because a Fisher-Yates run upward settles position _i_ at step
_i_ and never revisits it.

## `WithOwenScrambling` — how good is the hash?

Sobol's Owen scramble is hash-based (Burley 2020) rather than an exact nested permutation, and
the suite measures the gap rather than assuming it is small.

**Per-node tests did not distinguish it from fair coins.** Worst node bias is 3.85 sigma over 40000
seeds and 8191 nodes, where the largest of 8191 fair coins is expected near 3.9. The flips are
consistent with pairwise independence at the tested resolution; this is not a proof.

**Jointly across a level it is not.** Flip-count variance runs 0.09 to 3.06 of the binomial
value that the exact construction reproduces to within 4%.

On the measured smooth product integrand the RMS integration difference was about a tenth;
this is not an upper bound on other functions. The suite has an instrument to evaluate
changing them, and any future change to the hash should be measured against
`owen_uniformity_test.go`'s `exactOwen` reference rather than eyeballed.

## The two caveats worth carrying to a call site

- `WithNestedScrambling` integrates better than `WithScrambling`, but it costs about 8x per
  point. It suits integration; for a parameter sweep, where the worst case is what you feel,
  the two are now close enough that cost decides.
- `WithOwenScrambling` is nearly free on `AtInto` (370 ns/op against 360 for a digital shift
  at 39 dimensions) but roughly 3x on `NextInto` (197 against 65), because the Gray-code
  recurrence is precisely what cannot carry a non-linear scramble.
