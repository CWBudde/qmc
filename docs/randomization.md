# Randomization

Each option applies to one generator and is refused by name by the other, rather than
ignored. They are mutually exclusive: a generator has one randomization or none.

| option                 | generator | what it does                                                              |
| ---------------------- | --------- | ------------------------------------------------------------------------- |
| `WithScrambling`       | Halton    | One digit permutation per dimension (Braaten & Weller 1979)               |
| `WithNestedScrambling` | Halton    | Uniform digit permutation per node, conditioned on the digits above it    |
| `WithDigitalShift`     | Sobol     | One random word per dimension, XORed into every point                     |
| `WithOwenScrambling`   | Sobol     | Hash-based Owen scrambling: an independent flip at every node of the tree |

All four leave the low-discrepancy structure intact — each maps elementary intervals onto
elementary intervals of the same size — and all four make the generator an RQMC sequence, so
averaging over seeds yields an error estimate. Fix the seed and a run is reproducible.

## The defect being cured

The Halton sequence places its _d_-th coordinate by the radical inverse in base _p_d_, the
_d_-th prime. For a large base the first _p_d_ points of that coordinate are simply
`0, 1/p_d, 2/p_d, …` — a ramp, not a sample — and two adjacent high-dimensional coordinates
ramp together.

Measured at 39 dimensions and 600 points, which is what a parameter search over 39 knobs on a
600-evaluation budget actually asks for. These are absolute Pearson correlations over adjacent
dimension pairs, over **thirty** seeds:

| configuration                   | median | p90  | worst    |
| ------------------------------- | ------ | ---- | -------- |
| unscrambled, skip 64            | —      | —    | **0.81** |
| `WithScrambling`, skip 64       | 0.093  | 0.13 | **0.16** |
| `WithNestedScrambling`, skip 64 | 0.089  | 0.12 | **0.14** |

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
dimension's radical inverse (Braaten & Weller 1979). The permutations are independent across
dimensions; within a dimension the same permutation applies at every digit position. A digit
permutation maps each elementary interval onto another of the same size, so the
low-discrepancy structure survives and the ramps do not.

## `WithNestedScrambling` — and why it changed

It draws a uniform permutation **per node** of the scramble tree, conditioned on the digits
above it. Until recently it drew each node's permutation from the affine family
`x → ax+b mod p` rather than from all `p!`, which is free of shuffles but left a tail: worst
adjacent-pair |r| over thirty seeds of 0.37, against random-digit's 0.16.

The cause was understood rather than guessed. At 600 points a large-base coordinate varies
only in its first digit, where an affine map is a ramp of another slope rather than a
scattering, so two dimensions drawing commensurate slopes ramp together again.

Drawing a genuine uniform permutation per node removes the tail — worst over thirty seeds
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

**Per node it is indistinguishable from fair coins.** Worst node bias is 3.85 sigma over 40000
seeds and 8191 nodes, where the largest of 8191 fair coins is expected near 3.9. The flips are
also pairwise independent.

**Jointly across a level it is not.** Flip-count variance runs 0.09 to 3.06 of the binomial
value that the exact construction reproduces to within 4%.

That joint dependence costs at most a tenth of the RMS integration error, so the constants
stand. But this is the first instrument in the package with the resolution to evaluate
changing them, and any future change to the hash should be measured against
`owen_uniformity_test.go`'s `exactOwen` reference rather than eyeballed.

## The two caveats worth carrying to a call site

- `WithNestedScrambling` integrates better than `WithScrambling`, but it costs about 8x per
  point. It suits integration; for a parameter sweep, where the worst case is what you feel,
  the two are now close enough that cost decides.
- `WithOwenScrambling` is nearly free on `AtInto` (370 ns/op against 360 for a digital shift
  at 39 dimensions) but roughly 3x on `NextInto` (197 against 65), because the Gray-code
  recurrence is precisely what cannot carry a non-linear scramble.
