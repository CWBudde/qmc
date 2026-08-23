# qmc

[![Go Reference](https://pkg.go.dev/badge/github.com/cwbudde/qmc.svg)](https://pkg.go.dev/github.com/cwbudde/qmc)
[![Go Report Card](https://goreportcard.com/badge/github.com/cwbudde/qmc)](https://goreportcard.com/report/github.com/cwbudde/qmc)

Quasi-Monte Carlo sequences for Go: deterministic, low-discrepancy point sets that fill a
unit hypercube more evenly than independent random sampling does.

**[Try it in your browser →](https://cwbudde.github.io/qmc/)** — watch a 39-dimensional
Halton sequence collapse onto a diagonal ramp, and one toggle dissolve it, with the library
itself compiled to WebAssembly.

No dependencies.

```bash
go get github.com/cwbudde/qmc
```

## Usage

```go
g, err := qmc.NewHalton(39, qmc.WithSkip(64), qmc.WithScrambling(seed))
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

`Bases()` returns the prime base of each dimension, in order — the _d_-th base is the
_d_-th prime, so `Bases()[38]` is 167. `Permutation(d)` returns the random-digit scrambling
permutation of `{0…base-1}` applied to dimension _d_, or `nil` for an unscrambled generator
or an out-of-range _d_. Both return copies, so nothing a caller does to the returned slice
can perturb the sequence.

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

| configuration                        | worst \|corr\| over adjacent dimension pairs |
| ------------------------------------ | -------------------------------------------- |
| unscrambled, skip 64                 | **0.81**                                     |
| scrambled, skip 64, worst of 5 seeds | **0.14**                                     |

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

There is no fixed base table. Primes are sieved on demand, so `NewHalton(500)` works.
Scrambling allocates one permutation per dimension, sized by that dimension's prime, so
memory grows roughly as the sum of the first _d_ primes, four bytes per entry. That is about
12 KB at 39 dimensions and 3.5 MB at 500, but the sum grows faster than _d_ does: 5000
dimensions cost around 475 MB, so scrambling at that scale is a decision to make on purpose.

## License

MIT
