# Choosing a sequence

**Sobol is a useful default.** Its base 2 in every dimension avoids Halton's
growing prime bases. Accuracy still depends on effective dimension, projection
quality, and the selected sample block. The embedded Joe–Kuo direction numbers
cover 1024 dimensions; `WithDirectionNumbers` accepts a larger table.

**Halton** has no fixed base-table ceiling. Primes are sieved on demand, subject
to representable sizes and available memory. Its construction is simple to
reproduce. At small budgets, later coordinates can be strongly correlated;
consider scrambling or admissible leaping and measure the workload you care about.

## Comparing error and cost

The [controlled performance report](performance.md) is the canonical comparison:
40 streams, 39 dimensions, 4096 points, skip 64, and the smooth product

```
f(x) = product_k (1 + (x[k] - 0.5)/(k+1))
```

Its integral is 1. The report records seeds, the MC policy, index window,
toolchain, hardware, raw results, and approximate uncertainty. It compares
construction plus indexed generation and function evaluation as well as
per-point throughput. It uses a nonaligned Sobol block, so it is not a comparison
of optimal net usage.

The decaying weights give later dimensions less influence. A different
integrand, reversed importance ordering, sharper peak, discontinuity, or time
budget can change the result. Nested Halton and Owen Sobol merit consideration
when their extra work improves the relevant estimator. Digital shifting and
fixed-permutation Halton can suit cheaper generation budgets; fixed-permutation
scrambling does not give unbiased uniform marginals. See
[Randomization](randomization.md) for statistical assumptions and
[the small-sample regime](small-sample-regime.md) for separate finite-budget gates.

The ordinary smooth-product quality tests use 40 seeds, skip 64, leap 1, and a
`math/rand` baseline of consecutive draws from one source seeded 20240823.
Their broad margins are regressions for those fixtures. Their MC baseline differs
from the canonical performance report, which uses a fresh source per stream.
`TestIntegrationAcrossReferenceFunctionsAndBudgets` broadens coverage to moments,
interactions, reversed weights, a peak, and a discontinuity, with aligned Sobol
blocks. [Testing methodology](testing-methodology.md) describes those gates.

## Sobol alignment and projections

A base-2 `(t,m,s)`-net puts `2^t` points in each dyadic elementary interval of
volume `2^(t-m)` in a qualifying block of `2^m` points. Only `t=0` gives
one-point occupancy. The guarantee concerns a complete block of raw indices
beginning at a multiple of `2^m`, with leap 1.

This API starts at raw index 1 by default. For a later aligned block of
`N = 2^m` points, use `WithSkip(q*N - 1)` with a representable `q >= 1`.
The entire block must fit the raw-index ceiling. Negative skip is clamped to
zero, so `WithSkip(-1)` does not expose the origin block. [API design](api-design.md)
records the decision to retain skip/indexed access rather than add another helper.

Projections inherit the full-dimensional t guarantee and may improve it. The
Joe–Kuo search improves projection quality without making every pair a t=0 net.
The known first-two-dimensional t=0 projection is covered by
`TestFirstTwoDimensionsFormAZeroNet`; other projections can legitimately have
different occupancy. Digital shifts and nested scrambling preserve that occupancy
quality, rather than repairing a poor direction table.

## Dimension and construction limits

Sobol's embedded table covers 1024 dimensions. Upstream publishes Joe–Kuo tables
through 21201 at <https://web.maths.unsw.edu.au/~fkuo/sobol/>.
`WithDirectionNumbers(r io.Reader)` takes the upstream format and consumes its
reader during construction. Rows require contiguous dimensions, exactly s
initial values, odd `m_i < 2^i`, bounded coefficients (`a=0` at degree one),
and a primitive polynomial. The validator checks usability, not provenance or
optimized projection quality. `TestDirectionTableBeyondTheEmbeddedCeiling`
exercises a synthesized 1200-dimensional table.

Halton's fixed digit scrambling retains one int32 permutation per dimension.
Its digit payload is `4 * sum(first d primes)` bytes, before slices, allocator
rounding, generator state, and sieve work; it grows faster than linearly.
Nested scrambling instead has bounded root-table payload plus O(d) root state.
[Performance](performance.md) records constructor measurements and advises
reusing generators. Indexed access can be shared with separate output buffers.
