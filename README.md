# qmc

Quasi-Monte Carlo sequences for Go: deterministic, low-discrepancy point sets that fill a
unit hypercube more evenly than independent random sampling does.

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

## Use scrambling above ~20 dimensions

The Halton sequence places its *d*-th coordinate by the radical inverse in base *p_d*, the
*d*-th prime. For a large base the first *p_d* points of that coordinate are simply
`0, 1/p_d, 2/p_d, …` — a ramp, not a sample — and two adjacent high-dimensional
coordinates ramp together.

Measured here at 39 dimensions and 600 points, which is what a parameter search over 39
knobs on a 600-evaluation budget actually asks for:

| configuration | worst \|corr\| over adjacent dimension pairs |
|---|---|
| unscrambled, skip 64 | **0.81** |
| scrambled, skip 64, worst of 5 seeds | **0.14** |

`WithScrambling` applies an independent uniform permutation of the digit alphabet to every
digit position of every dimension (random-digit scrambling, Braaten & Weller 1979). The
sequence keeps its low-discrepancy structure — a digit permutation maps each elementary
interval onto another of the same size — but the ramps are gone.

The cost is that the sequence becomes seed-dependent: this is randomized quasi-Monte Carlo
(RQMC), not plain QMC. Fix the seed and runs are reproducible again. `TestScramblingBreaks
HighDimensionalCorrelation` and `TestUnscrambledStillShowsTheDefect` keep both halves of
the table above honest.

## Use a burn-in

The first Halton point is `(1/2, 1/3, 1/5, 1/7, …)`, which sits near a corner of the box in
every coordinate with a large base. `WithSkip(64)` discards the first 64 points. It is
cheap and it is the standard remedy.

## Dimensionality

There is no fixed base table. Primes are sieved on demand, so `NewHalton(500)` works.
Scrambling allocates one permutation per dimension, sized by that dimension's prime, so
memory grows roughly as the sum of the first *d* primes — a few kilobytes at 39
dimensions, a few megabytes at 5000.

## License

MIT
