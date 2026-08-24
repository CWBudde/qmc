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

**Sobol unless you have a reason.** Base 2 in every dimension, no degradation as dimensions
are added, capped at the 1024 dimensions the embedded Joe & Kuo direction numbers cover
(`WithDirectionNumbers` takes your own table for more).

**Halton** has no dimension ceiling — primes are sieved on demand, so `NewHalton(5000)`
works — and its construction is simple enough to reproduce by hand. Above roughly twenty
dimensions it must be randomized to be usable at all.

Measured on a smooth 39-dimensional product integrand at n=4096, against plain Monte Carlo
on the same budget: Sobol with Owen scrambling **32x**, Halton with nested scrambling
**31.9x**, Sobol with a digital shift **29.5x**, Halton with random-digit scrambling
**17.7x**. Those are ten-stream figures and the ordering moves at forty streams;
[Choosing a sequence](docs/choosing-a-sequence.md) has the full table, the caveats, and the
two Sobol balance properties that make a correct sequence look broken when you test it.

At 40 points rather than 4096 — the regime a seeded population actually uses — the advantage
survives but shrinks: 3.4x to 5.2x over Monte Carlo at 30 dimensions, and the gap between the
two best schemes closes to a tie. See [the small-sample regime](docs/small-sample-regime.md).

## Randomization

Each option applies to one generator and is refused by name by the other. They are mutually
exclusive: a generator has one randomization or none. All four keep the low-discrepancy
structure intact and make the generator an RQMC sequence, so averaging over seeds yields an
error estimate; fix the seed and a run is reproducible.

| option                 | generator | what it does                                                              |
| ---------------------- | --------- | ------------------------------------------------------------------------- |
| `WithScrambling`       | Halton    | One digit permutation per dimension (Braaten & Weller 1979)               |
| `WithNestedScrambling` | Halton    | Uniform digit permutation per node, conditioned on the digits above it    |
| `WithDigitalShift`     | Sobol     | One random word per dimension, XORed into every point                     |
| `WithOwenScrambling`   | Sobol     | Hash-based Owen scrambling: an independent flip at every node of the tree |

**Above ~20 Halton dimensions, scrambling is not optional.** The _d_-th coordinate is the
radical inverse in base _p_d_, and for a large base the first _p_d_ points of it are a ramp
rather than a sample — two adjacent high-dimensional coordinates ramp together, measuring a
worst adjacent-pair correlation of 0.81 at 39 dimensions and 600 points. Either scrambling
scheme takes that to 0.14–0.16 over thirty seeds.

[Randomization](docs/randomization.md) has the correlation tables, what each option costs per
point, why nested scrambling moved from affine to uniform permutations, and how far the
hash-based Owen scramble is from an exact one.

## Use a burn-in

The first Halton point is `(1/2, 1/3, 1/5, 1/7, …)`, which sits near a corner of the box in
every coordinate with a large base. `WithSkip(64)` discards the first 64 points. It is
cheap and it is the standard remedy.

## Leaping

`WithLeap(n)` takes every _n_-th point instead of every point: point _i_ becomes raw index
`skip + 1 + i*n`. It is the only deterministic remedy for the Halton defect — no seed, so a
leaped run is plain QMC — and it is the most accurate option in the package for integration:
**54.3x** Monte Carlo at 39 dimensions and 4096 points, against nested scrambling's 41.1x.

**A leap must be coprime to every base in use**, and both constructors refuse one that is
not, by name. If a base _p_ divides the leap, that coordinate is confined to a single strip
of width `1/p` while still producing a plausible spread of values inside it. Pick a prime
above the largest base — above 167 at 39 dimensions. On Sobol every base is 2, so the leap
must be odd; the reason there is _not_ the base-2 restatement it looks like.

The gain is in the average, not the tail: worst adjacent-pair |r| over thirty leaps runs
median 0.097, p90 0.23, worst 0.32, against `WithScrambling`'s 0.093/0.13/0.16. Leaping suits
integration; it does not suit a parameter sweep. [Leaping](docs/leaping.md) has the
measurements, the Gray-code parity argument for Sobol, and the cost.

## Discrepancy

Two ways to measure how evenly a point set fills the cube, failing in opposite directions.
`Draw(seq, n)` collects points into a matrix for either.

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

`StarDiscrepancy` is the exact `D*_N` — a supremum, not a sample and not a lower bound.
Computing it is NP-hard in the dimension, so it **refuses** above 6 dimensions or above a
calibrated budget of 3e7 search-tree leaves (about 0.8 seconds) and returns an error rather
than a partial answer or a hang.

`CenteredL2Discrepancy` is Hickernell's CD2 in closed form, O(N²s) in any dimension. It is
cheap everywhere and **stops meaning anything in high dimensions**: at 39 dimensions a good
point set and a random one differ by 1.02x over the very same points whose integration error
differs by 16.4x. The self-check: compare your number against
`sqrt(((5/4)^s - (13/12)^s)/N)`, the exact expectation for independent uniform points. If it
is not several times below that, the statistic is telling you about your marginals and
nothing else.

[Discrepancy](docs/discrepancy.md) has both measurement tables, the affordable point counts
per dimension, and why CD2 saturates.

## Web demo

[`examples/wasm-demo`](examples/wasm-demo) is a browser demo, published at
<https://cwbudde.github.io/qmc/>. Everything it shows is computed by this library compiled
to `js/wasm` — there is no JavaScript reimplementation of the sequence. It has two pages: a
**Point Lab** (scatter explorer plus digit inspector) and a **Discrepancy Bench**
(correlation heatmap, convergence chart, and a discrepancy sweep against a pseudorandom
baseline — where CD2's saturation is visible as three curves lying on top of one another).

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

## Documentation

The [`docs/`](docs) directory carries the long form: the measurements behind every figure
above, the caveats that only matter once you are relying on a number, and the reasoning
behind choices that are not obvious from the code. [`docs/README.md`](docs/README.md) is the
index. [`PLAN.md`](PLAN.md) is the open backlog.

## License

MIT.

The Sobol direction numbers in [`third_party/joe-kuo`](third_party/joe-kuo) are Frances Y.
Kuo and Stephen Joe's, redistributed under their BSD-3 notice, which is kept alongside them.
