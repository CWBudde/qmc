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

**Halton** has no dimension ceiling — primes are sieved on demand, so `NewHalton(5000)`
works — and its construction is simple enough to reproduce by hand, which matters if you
are migrating off an existing implementation. Above roughly twenty dimensions it has to be
randomized to be usable at all; see below.

Measured on a smooth 39-dimensional product integrand at n=4096 over ten randomization
streams, against the same plain Monte Carlo baseline. Lower error is better; the multiplier
is how many times more accurate than Monte Carlo on the same budget:

| generator                             | RMS relative error | vs Monte Carlo |
| ------------------------------------- | ------------------ | -------------- |
| plain Monte Carlo (`math/rand`)       | 4.3e-03            | 1x             |
| Halton, random-digit scrambling       | 2.4e-04            | **17.7x**      |
| Sobol, digital shift                  | 1.5e-04            | **29.5x**      |
| Sobol, Owen scrambling                | 1.4e-04            | **32.0x**      |
| Halton, nested affine scrambling      | 8.1e-05            | **53.2x**      |

Read that table with one caveat, because it is one integrand. Nested affine Halton coming
out ahead of Sobol here is a real measurement, not a typo, but a smooth product with
decaying coefficients is the case it suits best — and it carries a worse worst-case
correlation than random-digit scrambling does, which the next section covers. The
generators are gated in the test suite at a factor of five against Monte Carlo, not at
these numbers, because the ratios move with the integrand and the assertion should not.

## Randomization

Each option below applies to one generator and is refused by name by the other, rather than
ignored. They are mutually exclusive: a generator has one randomization or none.

| option                    | generator | what it does                                                              |
| ------------------------- | --------- | ------------------------------------------------------------------------- |
| `WithScrambling`          | Halton    | One digit permutation per dimension (Braaten & Weller 1979)                |
| `WithNestedScrambling`    | Halton    | Per-digit affine permutation, conditioned on the digits above it           |
| `WithDigitalShift`        | Sobol     | One random word per dimension, XORed into every point                      |
| `WithOwenScrambling`      | Sobol     | Hash-based Owen scrambling: an independent flip at every node of the tree  |

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

| configuration                          | median | p90  | worst    |
| -------------------------------------- | ------ | ---- | -------- |
| unscrambled, skip 64                   | —      | —    | **0.81** |
| `WithScrambling`, skip 64              | 0.093  | 0.13 | **0.16** |
| `WithNestedScrambling`, skip 64        | 0.090  | 0.20 | **0.37** |

Those are absolute Pearson correlations over adjacent dimension pairs, taken over thirty
seeds. Thirty rather than a handful because the statistic turns out to be high-variance: a
change to the scrambling that was a pure re-instantiation, not a change of scheme, moved a
five-seed worst case from 0.40 to 0.12. Quote the median and the tail, not one draw.

The tail is where nested affine scrambling loses despite integrating better, and the cause
is understood rather than guessed: at 600 points a large-base coordinate varies only in its
first digit, where `x → ax+b mod p` is a ramp of a different slope rather than a scattering,
so two dimensions that draw commensurate slopes ramp together again.

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

## Dimensionality

**Sobol** covers 1024 dimensions from the embedded Joe & Kuo direction numbers.
`WithDirectionNumbers(r io.Reader)` takes a caller's own table in the same format for more.
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
