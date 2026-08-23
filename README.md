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
(correlation heatmap plus convergence chart).

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
