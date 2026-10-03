# The small-sample regime

A population or experiment may use only 40 points in 30 dimensions. Performance
at 4096 points does not establish accuracy at that budget. The fixtures in
small_sample_test.go measure small draws directly; they do not infer an
asymptotic convergence rate or optimizer performance from one integration task.

## Integration method and gates

`TestSmallSampleIntegration` compares all four randomization options on

```
f(x) = product_k (1 + (x[k] - 0.5)/(k+1))
```

The integral is 1. It measures dimensions 2, 10, and 30, with 40 and 160 points,
using seeds 1..200 for every randomized configuration, skip 64, and leap 1.
The MC comparison uses consecutive draws from one `math/rand` source seeded
20240823 for each dimension/count cell. Indexed calls visit points 0..N-1;
the Sobol block is not aligned for a power-of-two net guarantee.

Error is the square root of the mean squared relative estimation error across
these finite seeds. Consecutive seeds make the fixture reproducible; they do
not prove independent replicates or unbiased uniform marginals. See
[Randomization](randomization.md).

The test requires every configuration in this grid to beat its MC baseline by
a conservative factor of 1.5. At two dimensions it also requires the advantage
not to shrink when the count increases from 40 to 160. These gates check behavior
on this smooth product, not universal superiority or a convergence theorem.
Verbose output gives the actual current measurements.

## Rankings and uncertainty

`TestSmallSampleRankingMatchesLargeSample` compares 40 and 4096 points at
30 dimensions over seeds 1..200, with the same skip and leap settings. It
reports both rankings and allows Owen Sobol to be within 20% of the measured
large-budget leader. It does not require an exact winner or enforce the
small-budget ordering.

Close RMS values can change order with the seeds and integrand. A normal,
independent-error approximation gives a relative RMS standard error of about
`1/sqrt(2*replicates)`, but that is a model-dependent guide, not a confidence
interval for this seeded family. The finite-fixture margin does not measure bias
or justify extrapolating a ranking to other functions.

The [controlled performance report](performance.md) is the canonical 40-stream,
39-dimensional comparison of accuracy and end-to-end cost. Its MC seed policy
differs from these small-sample tests, so compare each configuration against the
baseline within its own experiment.

## Discrepancy at small budgets

`TestSmallSampleDiscrepancy` measures 2, 3, 10, and 30 dimensions, with 40 and
160 points. It averages CD2 over seeds 1..200 for the same four schemes, skip 64,
and leap 1. Exact star discrepancy is measured only at two and three dimensions.
The random baseline uses consecutive draws from a source seeded 20240824,
restarted for each dimension/count cell.

For independent uniform points,

```
E[CD2²] = ((5/4)^s - (13/12)^s) / N
```

The analytic curve `sqrt(E[CD2²])` is the RMS of CD2. The test's averages of CD2
estimate `E[CD2]`, a different quantity; Jensen's inequality gives
`E[CD2] <= sqrt(E[CD2²])`. Agreement can be close when CD2 is concentrated,
but is not an exact equality.

The gate requires lower mean star discrepancy than the random baseline at two
dimensions. It places no corresponding separation gate on CD2. CD2 can provide
little separation at high dimension even when integration errors differ on this
product. Neither statistic alone establishes quality for an arbitrary integrand
or optimization algorithm. [Discrepancy](discrepancy.md) explains limits and
numerical precision.

## Choosing a small draw

Measure the intended objective and budget. These smooth-product gates support
considering randomized Sobol or Halton at small counts; they do not establish
that an initial population improves a downstream optimizer. Decide between the
schemes using relevant error, cost, and uncertainty evidence. Into buffers and
generator reuse avoid repeated result allocation and construction.

For Sobol experiments at power-of-two counts, also compare complete aligned raw
blocks rather than assuming a default draw has net balance. Use
`WithSkip(q*N-1)` for a representable `q >= 1` and leap 1, with the whole
block inside the raw-index range. Counts such as 40 are not power-of-two nets.

Reproduce the separate small-sample fixtures with:

```sh
go test -count=1 -timeout=10m -run '^TestSmallSample' -v .
```

All three sweeps are skipped by `-short`. Use `just test-statistical` for
the complete statistical suite; runtime depends on the host and exact
star-discrepancy workload.
