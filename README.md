# qmc

[![Go Reference](https://pkg.go.dev/badge/github.com/cwbudde/qmc.svg)](https://pkg.go.dev/github.com/cwbudde/qmc)
[![Go Report Card](https://goreportcard.com/badge/github.com/cwbudde/qmc)](https://goreportcard.com/report/github.com/cwbudde/qmc)

Quasi-Monte Carlo sequences for Go: deterministic, low-discrepancy point sets that fill a
unit hypercube more evenly than independent random sampling does. Sobol and Halton, each
with optional randomization.

**[Try it in your browser →](https://cwbudde.github.io/qmc/)** — watch a 39-dimensional
Halton sequence collapse onto a diagonal ramp, and one toggle dissolve it, with the library
itself compiled to WebAssembly.

No dependencies.

```bash
go get github.com/cwbudde/qmc
```

## Usage

```go
g, err := qmc.NewSobol(39, qmc.WithSkip(64), qmc.WithOwenScrambling(seed))
if err != nil {
    return err
}
for i := 0; i < 600; i++ {
    point := g.Next() // len(point) == 39, every coordinate in [0,1)
    ...
}
```

`At(i)` is the stateless form. It depends only on `i` and the generator's configuration,
never on how many points have been drawn, so a worker pool can share one sequence by
claiming indices from an atomic counter:

```go
idx := counter.Add(1)
g.AtInto(int(idx)-1, pos)
```

`Next`, `NextInto` and `Reset` are stateful and not safe for concurrent use. `At` and
`AtInto` are.

Both generators satisfy `qmc.Sequence`, which carries exactly those six methods, so code
that just needs points can accept the interface and let the caller choose the sequence.
`Bases()` and `Permutation()` are Halton-only and deliberately not on it — Sobol works in
base 2 in every dimension and has neither, and the honest answer to a question that does
not apply is that you are holding the wrong type, not a zero value.

## Which sequence

**Sobol unless you have a reason.** It is base 2 in every dimension and does not degrade as
dimensions are added, which is exactly where Halton struggles. It is capped at the 1024
dimensions the embedded direction numbers cover, unless you supply your own table with
`WithDirectionNumbers`.

Two things about Sobol's balance are worth knowing before you test it, because each one
makes a correct sequence look broken. The (t,m,s)-net property — the first 2^m points
landing one apiece in every elementary interval — holds on a 2^m-_aligned_ block of raw
indices, so a stratification check wants `WithSkip(2^m - 1)`; with the default skip of 0 all
40 of the first 40 dimensions come out unbalanced at m=8. And the D(6) direction numbers
optimise two-dimensional projections without making them all nets: of the 780 pairs among
the first 40 dimensions, 18 are balanced at every split at m=8 and 4 at m=10. Plot
dimensions 0 and 1 and you get one point per cell at every aspect ratio; plot 12 and 23 over
the same 256 points and 224 of the 256 cells of the 16x16 grid are empty while one holds
eight. Both are the correct table behaving correctly.

**Halton** has no dimension ceiling — primes are sieved on demand, so `NewHalton(5000)`
works — and its construction is simple enough to reproduce by hand, which matters if you
are migrating off an existing implementation. Above roughly twenty dimensions it has to be
randomized to be usable at all; see below.

Measured on a smooth 39-dimensional product integrand at n=4096 over ten randomization
streams, against the same plain Monte Carlo baseline. Lower error is better; the multiplier
is how many times more accurate than Monte Carlo on the same budget:

| generator                       | RMS relative error | vs Monte Carlo |
| ------------------------------- | ------------------ | -------------- |
| plain Monte Carlo (`math/rand`) | 4.3e-03            | 1x             |
| Halton, random-digit scrambling | 2.4e-04            | **17.7x**      |
| Sobol, digital shift            | 1.5e-04            | **29.5x**      |
| Sobol, Owen scrambling          | 1.4e-04            | **32.0x**      |
| Halton, nested scrambling       | 1.4e-04            | **31.9x**      |

Read that table with two caveats. It is one integrand, and a smooth product with decaying
coefficients is the case nested scrambling suits best. And ten streams is not many: the
same measurement over forty seeds moves random-digit Halton to 24.4x and nested Halton to
41.1x, which is a different ordering against Sobol than the ten-stream column shows. The
ratios move with the integrand and with the stream count, so the test suite gates the
generators at a factor of five against Monte Carlo rather than at any of these numbers.

## Randomization

Each option below applies to one generator and is refused by name by the other, rather than
ignored. They are mutually exclusive: a generator has one randomization or none.

| option                 | generator | what it does                                                              |
| ---------------------- | --------- | ------------------------------------------------------------------------- |
| `WithScrambling`       | Halton    | One digit permutation per dimension (Braaten & Weller 1979)               |
| `WithNestedScrambling` | Halton    | Uniform digit permutation per node, conditioned on the digits above it    |
| `WithDigitalShift`     | Sobol     | One random word per dimension, XORed into every point                     |
| `WithOwenScrambling`   | Sobol     | Hash-based Owen scrambling: an independent flip at every node of the tree |

All four leave the low-discrepancy structure intact — each maps elementary intervals onto
elementary intervals of the same size — and all four make the generator an RQMC sequence,
so averaging over seeds yields an error estimate. Fix the seed and a run is reproducible.

Two honest caveats, both measured:

- `WithNestedScrambling` integrates better than `WithScrambling` but its worst-case
  adjacent-pair correlation over thirty seeds is 0.37 against 0.16, and it costs about 8x
  per point. It suits integration; it does not suit a parameter sweep, where the worst case
  is what you feel.
- `WithOwenScrambling` is nearly free on `AtInto` (370 ns/op against 360 for a digital
  shift at 39 dimensions) but roughly 3x on `NextInto` (197 against 65), because the
  Gray-code recurrence is precisely what cannot carry a non-linear scramble.

## Web demo

[`examples/wasm-demo`](examples/wasm-demo) is a browser demo, published at
<https://cwbudde.github.io/qmc/>. Everything it shows is computed by this library compiled
to `js/wasm` — there is no JavaScript reimplementation of the sequence. It has two pages: a
**Point Lab** (scatter explorer plus digit inspector) and a **Discrepancy Bench**
(correlation heatmap, convergence chart, and a discrepancy sweep against a pseudorandom
baseline — where the saturation described below is visible as three curves lying on top of
one another).

```bash
just run-wasm-demo
```

## Introspection

These are Halton-only; Sobol has neither prime bases nor a permutation table.

`Bases()` returns the prime base of each dimension, in order — the _d_-th base is the _d_-th
prime, so `Bases()[38]` is 167. `Permutation(d)` returns the random-digit scrambling
permutation of `{0…base-1}` applied to dimension _d_, or `nil` for an unscrambled generator,
a nested-scrambled one (which has no table to hand out) or an out-of-range _d_. Both return
copies, so nothing a caller does to the returned slice can perturb the sequence.

```go
fmt.Printf("dimension 38 uses base %d\n", g.Bases()[38])
```

## Discrepancy

Two ways to measure how evenly a point set fills the cube, and they fail in opposite
directions: one is exact and cannot be computed above a handful of dimensions, the other is
cheap in any dimension and stops meaning anything in high ones.

`Draw(seq, n)` collects `n` points into a matrix for either of them. It is built on `AtInto`,
so it leaves the generator's cursor where it was and the same matrix drawn twice is the same
matrix; the rows alias one backing array, which is two allocations and the difference between
a cache-resident inner loop and a scattered one.

```go
g, err := qmc.NewHalton(3, qmc.WithSkip(64), qmc.WithScrambling(seed))
if err != nil {
    return err
}

d, err := qmc.StarDiscrepancy(qmc.Draw(g, 512))
if err != nil {
    return err // above 6 dimensions, or past the work budget, this is where you find out
}

fmt.Printf("D*_512 = %.6f\n", d)
```

`StarDiscrepancy` returns the exact `D*_N` — the largest relative error any origin-anchored
box makes about how much of the cube it covers, which is the quantity the Koksma-Hlawka bound
multiplies by an integrand's variation. It is a supremum, not a sample and not a lower bound.
Scrambled Halton against `math/rand`, ten seeds each; lower is better:

| s   | N   | scrambled Halton | `math/rand` | ratio      |
| --- | --- | ---------------- | ----------- | ---------- |
| 1   | 512 | 0.001953         | 0.039848    | **20.40x** |
| 2   | 64  | 0.045907         | 0.148041    | 3.22x      |
| 2   | 512 | 0.009727         | 0.053237    | **5.47x**  |
| 3   | 512 | 0.017966         | 0.069954    | 3.89x      |
| 4   | 160 | 0.055348         | 0.128141    | 2.32x      |

The gaps in that table are the cost. Restricting each dimension's box corners to the points'
own coordinates is exact and still leaves C(N+s,s) ≈ N^s/s! boxes to walk, and computing the
exact star discrepancy is NP-hard in the dimension (Gnewuch, Srivastav & Winker, _Journal of
Complexity_ 25(2), 2009) — so the ceiling is arithmetic, not an unfinished optimisation.
`StarDiscrepancy` therefore **refuses** above 6 dimensions or above a budget of 3e7 search-tree
leaves, and returns an error rather than a partial answer or a hang: at a flat ~27 ns per leaf
the budget is about 0.8 seconds, and a caller cannot tell a hang apart from a slow machine.
Affordable point counts are 7744 at 2 dimensions, 562 at 3, 161 at 4, 78 at 5 and 49 at 6,
which is why the s=4 row above stops at 160. Benchmarks: 14.6 ms at 2 dimensions and N=1024,
764 ms at 4 dimensions and N=160.

`CenteredL2Discrepancy` is the alternative above that. It averages over the same family of
boxes instead of taking a supremum, has Hickernell's closed form at O(N²s) in any dimension,
and returns the square root — 24.5 ms at 39 dimensions and N=1024, 484 ms at N=4096.

**Read this before believing a CD2 number.** For N independent uniform points the expectation
is exactly `E[CD2²] = ((5/4)^s - (13/12)^s)/N`, and it grows fast enough with _s_ that a good
point set and a random one converge onto it together. Measured at N=1024 over ten seeds,
against that analytic value:

| s   | CD2 Halton | CD2 random | analytic | random ÷ Halton | diagonal share |
| --- | ---------- | ---------- | -------- | --------------- | -------------- |
| 2   | 0.001370   | 0.017054   | 0.019488 | **12.45x**      | 402%           |
| 5   | 0.006376   | 0.040198   | 0.039026 | 6.30x           | 196%           |
| 10  | 0.033736   | 0.080573   | 0.083190 | 2.39x           | 131%           |
| 15  | 0.097509   | 0.159105   | 0.156561 | 1.63x           | 113%           |
| 20  | 0.220000   | 0.280846   | 0.282599 | 1.28x           | 106%           |
| 30  | 0.818767   | 0.885773   | 0.882090 | 1.08x           | 101%           |
| 39  | 2.365729   | 2.404613   | 2.419777 | **1.02x**       | 100.4%         |

The last row is not a verdict on the sequence, and taking it as one would contradict the
integration table above. Over the very same 39-dimensional 1024-point sets, RMS integration
error is 8.06e-04 for QMC against 1.32e-02 for Monte Carlo — **16.4x**. The statistic, not the
point set, is what has stopped working.

The last column says why. The `i = j` diagonal terms of the double sum account for 100.4% of
the total at 39 dimensions: the statistic has become entirely its own diagonal, and the
diagonal depends only on each coordinate's marginal spread, not at all on how the points sit
relative to one another. Everything CD2 was meant to measure lives in a residual smaller than
the rounding of the terms around it.

It is a decay and not a cliff — 12.5x at 2 dimensions, 2.4x at 10, 1.28x at 20, 1.02x at 39 —
so there is no honest dimension at which to refuse, which is why this one documents a caveat
where `StarDiscrepancy` returns an error. The self-check: compare your number against
`sqrt(((5/4)^s - (13/12)^s)/N)`. If it is not several times below that, the statistic is
telling you about your marginals and nothing else, and you want an integration test or
`StarDiscrepancy` in a projection instead.

## Use scrambling above ~20 dimensions

The Halton sequence places its _d_-th coordinate by the radical inverse in base _p_d_, the
_d_-th prime. For a large base the first _p_d_ points of that coordinate are simply
`0, 1/p_d, 2/p_d, …` — a ramp, not a sample — and two adjacent high-dimensional
coordinates ramp together.

Measured here at 39 dimensions and 600 points, which is what a parameter search over 39
knobs on a 600-evaluation budget actually asks for:

| configuration                   | median | p90  | worst    |
| ------------------------------- | ------ | ---- | -------- |
| unscrambled, skip 64            | —      | —    | **0.81** |
| `WithScrambling`, skip 64       | 0.093  | 0.13 | **0.16** |
| `WithNestedScrambling`, skip 64 | 0.089  | 0.12 | **0.14** |

Those are absolute Pearson correlations over adjacent dimension pairs, taken over thirty
seeds. Thirty rather than a handful because the statistic turns out to be high-variance: a
change to the scrambling that was a pure re-instantiation, not a change of scheme, moved a
five-seed worst case from 0.40 to 0.12. Quote the median and the tail, not one draw.

That statistic is also what changed `WithNestedScrambling`. Until recently it drew each
node's permutation from the affine family `x → ax+b mod p` rather than from all `p!`, which
is free of shuffles but leaves a tail this table used to show as a worst of 0.37 against
random-digit's 0.16. The cause was understood rather than guessed: at 600 points a
large-base coordinate varies only in its first digit, where an affine map is a ramp of
another slope rather than a scattering, so two dimensions drawing commensurate slopes ramp
together again. Drawing a genuine uniform permutation per node removes the tail, at about
forty times the cost per point and about a sixth of the integration advantage — the trade
is written out at the top of `nested.go`.

`WithScrambling` draws one uniform permutation of the digit alphabet per dimension and maps
every digit of that dimension's radical inverse through it (random-digit scrambling, Braaten
& Weller 1979). The permutations are independent across dimensions; within a dimension the
same permutation applies at every digit position. The sequence keeps its low-discrepancy
structure — a digit permutation maps each elementary interval onto another of the same size
— but the ramps are gone.

The cost is that the sequence becomes seed-dependent: this is randomized quasi-Monte Carlo
(RQMC), not plain QMC. Fix the seed and runs are reproducible again.
`TestScramblingBreaksHighDimensionalCorrelation` and
`TestUnscrambledStillShowsTheDefect` keep both halves of the table above honest.

## Use a burn-in

The first Halton point is `(1/2, 1/3, 1/5, 1/7, …)`, which sits near a corner of the box in
every coordinate with a large base. `WithSkip(64)` discards the first 64 points. It is
cheap and it is the standard remedy.

## Leaping

`WithLeap(n)` takes every _n_-th point instead of every point: point _i_ becomes raw index
`skip + 1 + i*n`. It is the third remedy for the high-dimensional Halton defect, after a
burn-in and the two scrambling schemes, and the only deterministic one — no seed, so a leaped
run is plain QMC and reproducible without recording anything.

**A leap must be coprime to every base in use.** If a base _p_ divides _n_, every raw index is
congruent to `skip+1` mod _p_, so that coordinate's leading base-_p_ digit never changes and
the coordinate is confined to a single strip of width `1/p` — measured at 39 dimensions with a
leap of 167, dimension 38 spends the entire run inside a strip covering 0.6% of its range,
while still producing a plausible spread of values inside it. Scrambling does not rescue it: a
permuted constant digit is still a constant digit, so the coordinate moves to a different
strip of the same width. `NewHalton` and `NewSobol` therefore refuse such a leap by name
rather than let it go wrong quietly. In practice, pick a prime above the largest base — above
167 at 39 dimensions. On Sobol every base is 2, so the leap must simply be odd.

Measured at 39 dimensions and 4096 points, over forty admissible leaps against forty
scrambling seeds — one run, so these are a comparison rather than four sittings:

| configuration          | RMS relative error | vs Monte Carlo |
| ---------------------- | ------------------ | -------------- |
| unleaped, unscrambled  | 3.9e-03            | 1.4x           |
| `WithLeap`             | **1.0e-04**        | **54.3x**      |
| `WithScrambling`       | 2.2e-04            | 24.4x          |
| `WithNestedScrambling` | 1.3e-04            | 41.1x          |

So leaping integrates better than either scrambling scheme, for free and without a seed. The
catch is the other statistic — worst adjacent-pair |r| at 600 points, over thirty leaps:

| configuration                   | median | p90  | worst    |
| ------------------------------- | ------ | ---- | -------- |
| unleaped, unscrambled, skip 64  | —      | —    | **0.81** |
| `WithLeap`, skip 64             | 0.097  | 0.23 | **0.32** |
| `WithScrambling`, skip 64       | 0.093  | 0.13 | **0.16** |
| `WithNestedScrambling`, skip 64 | 0.089  | 0.12 | **0.14** |

The medians agree; the tail does not. A leap only reorders the digits a coordinate visits, so
two dimensions whose bases interact with the leap in commensurate ways still ramp together,
and roughly one leap in ten draws such a pair. Leaping suits integration, where the average is
what you get; it does not suit a parameter sweep, where the worst case is what you feel.

It is not quite free, and not for the reason it looks: the implementation is one multiply, but
a leaped generator works on raw indices _n_ times larger, so its radical inverses carry about
`log(n)` more digits. `AtInto` at 39 dimensions costs 634 ns/op at a leap of 173 against 512
unleaped.

On **Sobol** the option exists for symmetry and is not the one to reach for — Sobol has no ramp
defect to cure. An odd leap forfeits the (t,m,s)-net balance property unconditionally (8.8e-03
against 1.8e-04 unleaped at 16 dimensions) and costs `Next` its Gray-code recurrence (301
ns/op against 46.0). An **even** leap is refused, and the reason is worth stating because it is
not the base-_p_ argument above: this package generates in Gray-code order, and the parity of
the population count of `gray(m)` is exactly `m&1`, which is the leading bit of every
dimension whose direction numbers all carry their own leading bit. Dimension 1 is such a
dimension in the embedded table, so an even leap pins it to one half of `[0,1)` and multiplies
the integration error by several hundred.

`TestLeapingBreaksHighDimensionalCorrelation`, `TestLeapingIntegratesBetterThanAnUnleapedSequence`,
`TestASharedFactorConfinesTheHaltonCoordinate` and `TestAnEvenLeapConfinesASobolCoordinate`
keep every number and every claim above honest.

## Dimensionality

**Sobol** covers 1024 dimensions from the embedded Joe & Kuo direction numbers.
`WithDirectionNumbers(r io.Reader)` takes a caller's own table in the same format for more —
upstream publishes the same construction out to 21201 dimensions at
<https://web.maths.unsw.edu.au/~fkuo/sobol/>, and `new-joe-kuo-6.21201` can be passed whole.
The format and the invariants a table has to satisfy (contiguous _d_ from 2, exactly _s_
direction numbers per row, every _m_i_ odd and below 2^_i_, a primitive polynomial) are
documented on `WithDirectionNumbers`; anything failing them is refused at construction.
Direction numbers cannot be derived — the initial values come from those authors'
numerical search — so a table is the only honest option.

**Halton** has no fixed base table. Primes are sieved on demand, so `NewHalton(500)` works.
Scrambling allocates one permutation per dimension, sized by that dimension's prime, so
memory grows roughly as the sum of the first _d_ primes, four bytes per entry. That is about
12 KB at 39 dimensions and 3.5 MB at 500, but the sum grows faster than _d_ does: 5000
dimensions cost around 475 MB, so scrambling at that scale is a decision to make on purpose.

## License

MIT.

The Sobol direction numbers in [`third_party/joe-kuo`](third_party/joe-kuo) are Frances Y.
Kuo and Stephen Joe's, redistributed under their BSD-3 notice, which is kept alongside them.
