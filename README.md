# qmc

[![Go Reference](https://pkg.go.dev/badge/github.com/cwbudde/qmc.svg)](https://pkg.go.dev/github.com/cwbudde/qmc)
[![Go Report Card](https://goreportcard.com/badge/github.com/cwbudde/qmc)](https://goreportcard.com/report/github.com/cwbudde/qmc)

Quasi-Monte Carlo sequences for Go: Sobol and Halton, with optional seeded
randomization and no runtime dependencies. Structured sampling can improve
integration accuracy; the gain depends on the integrand, projections, and sample
window.

**[Try it in your browser](https://cwbudde.github.io/qmc/)** — explore scatter
projections, digit scrambling, correlation, convergence, and discrepancy using
the library compiled to WebAssembly.

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
    point := g.Next() // len(point) == 39, coordinates in [0,1)
    // Use point in your application.
}
```

Both generators implement `qmc.Sequence`: `Dims`, `Next`, `NextInto`, `Reset`,
`At`, and `AtInto`. Indexed access depends only on the index and configuration
and leaves the cursor unchanged. `At` and `AtInto` are safe to share between
goroutines, with separate destination buffers. Serialize `Next`, `NextInto`,
and `Reset`. The concrete generators document index limits, panic boundaries,
and scratch allocation; Into methods avoid allocating a point slice.

## Choosing a sequence

Sobol is a useful starting choice. It uses base 2 in every dimension; the embedded
Joe–Kuo table supports 1024 dimensions. `WithDirectionNumbers` accepts a caller's
larger table. Projection quality and effective dimension still matter.

Halton has no fixed table ceiling: primes are generated on demand, subject to
arithmetic and memory limits. Large prime bases can produce correlated early
coordinates. Scrambling often helps at small budgets. Fixed digit-permutation
construction can be expensive in high dimensions; reuse the generator.

[Performance](docs/performance.md) is the canonical current measurement report:
one controlled machine/toolchain, repeated timings and allocations, and a
40-stream smooth-product integration comparison at 39 dimensions and 4096
points. It includes raw data, configuration, seeds, uncertainty estimates, and
reproduction commands. These observations do not establish a universal ranking.
[Choosing a sequence](docs/choosing-a-sequence.md) explains Sobol alignment and
projection guarantees; [small budgets](docs/small-sample-regime.md) describes
the separate 40/160-point fixtures.

## Randomization

The options are mutually exclusive and accepted only by their named generator.
They preserve the construction's digit structure within the supported depth.

| Option                 | Generator | Construction                                                               |
| ---------------------- | --------- | -------------------------------------------------------------------------- |
| `WithScrambling`       | Halton    | One seeded digit permutation per dimension, reused at every digit position |
| `WithNestedScrambling` | Halton    | Seeded node permutations conditioned on the preceding digits               |
| `WithDigitalShift`     | Sobol     | One seeded XOR word per dimension                                          |
| `WithOwenScrambling`   | Sobol     | Hash-based nested bit flips                                                |

Fixed digit scrambling does not give uniform point marginals: its first base-2
point is 0.5 for every seed. Digital shifting and nested schemes use finite
precision and seeded pseudorandomness. Seed variability does not measure bias
it cannot detect. [Randomization](docs/randomization.md) distinguishes the
ideal mathematical constructions from these implementations.

## Starting windows and leaping

The first Halton point is `(1/2, 1/3, 1/5, …)`. `WithSkip(64)` omits the first
64 raw points; this does not guarantee improved accuracy or remove high-base
patterns. When Sobol's net balance matters, use a complete power-of-two block
aligned in raw indices. This API omits the origin; later aligned blocks are
available through skip, as explained in [API design](docs/api-design.md).

`WithLeap(n)` selects raw index `skip + 1 + i*n`. Halton requires a leap
coprime to every prime base in use; Sobol requires an odd leap. Both constructors
validate this. Leaping is deterministic and changes the sampled window.
On Sobol, a leap greater than one gives up the general aligned-block guarantee
and the optimized stateful recurrence. [Leaping](docs/leaping.md) describes
the validation and separately scoped quality fixtures. Measure the actual
application before choosing a leap.

## Discrepancy

`Draw(seq, n)` collects indexed points without moving the cursor.
`StarDiscrepancy` returns the exact maximum absolute difference between an
origin-anchored box's volume and its empirical point fraction. One-point and
one-dimensional sets have cheap paths. Generic multipoint enumeration refuses
more than six dimensions or a conservative budget of 30 million search leaves;
the budget is a work limit, not a wall-clock guarantee.

`CenteredL2Discrepancy` returns Hickernell's centered L2 norm. Its cost is
O(N log N) in one dimension and O(N²s) otherwise. It rejects arithmetic outside
its supported float64 range. For N ideal independent uniform points,

```text
E[CD2²] = ((5/4)^s - (13/12)^s) / N
```

The square root is the **uniform-point RMS baseline**, `sqrt(E[CD2²])`, rather
than the mean `E[CD2]`. A ratio near that baseline indicates little contrast
for this statistic on the measured sets; it does not establish independence,
integration accuracy, or a universal dimension at which CD2 becomes useless.
[Discrepancy](docs/discrepancy.md) explains precision, limits, and interpretation.

## Web demo and introspection

The [demo](examples/wasm-demo/README.md) has a Point Lab and a Discrepancy Bench.
Computation runs locally in cancellable Go/WASM workers. Run it with:

```bash
just run-wasm-demo
```

Halton's `Bases()` returns a copy of its prime bases. `Permutation(d)` returns
a copy of the fixed digit permutation, or nil for an invalid dimension, plain
Halton, or nested scrambling. A nested scramble has different permutations at
different nodes; its bounded internal root cache is not a single permutation
for the entire dimension.

## Documentation and contributing

The [documentation index](docs/README.md) links the mathematical contracts,
measurement evidence, tooling, and design decisions.
[CONTRIBUTING.md](CONTRIBUTING.md) describes setup and verification.
[PLAN.md](PLAN.md) tracks review remediation and its completion evidence.

## License

MIT. The [Joe–Kuo direction numbers](third_party/joe-kuo) retain their BSD-3
notice. The browser distribution includes complete project, Joe–Kuo, and Go
toolchain notices, linked from **Credits and licenses** on both pages.
