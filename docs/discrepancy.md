# Discrepancy

Star discrepancy measures the largest origin-anchored box deviation. Centered
L2 discrepancy integrates squared deviations across coordinate projections
before taking a square root. Both describe point sets; neither
determines integration accuracy without considering the integrand.

`Draw(seq, n)` collects points through AtInto, preserving the generator's cursor.
Its rows share contiguous data and each row's capacity ends at its own boundary.

```go
g, err := qmc.NewHalton(3, qmc.WithSkip(64), qmc.WithScrambling(seed))
if err != nil {
    return err
}

d, err := qmc.StarDiscrepancy(qmc.Draw(g, 512))
if err != nil {
    return err // this multipoint case may exceed the dimension or work budget
}
fmt.Printf("D*_512 = %.6f\n", d)
```

## Exact star discrepancy and work limits

`StarDiscrepancy` returns the exact `D*_N` to float64 arithmetic, rather than
a sampled lower bound. Both strict and inclusive counts are needed to find the
supremum over origin-anchored boxes. The Koksma–Hlawka bound relates star
discrepancy to integration error for functions of bounded Hardy–Krause variation;
a small discrepancy does not bound arbitrary integrands without that assumption.

The function handles a single point in O(s) and a one-dimensional set by sorting
a copy in O(N log N) before applying generic gates. These paths can support
cases outside the multipoint enumeration limits. Higher-dimensional multipoint
sets are refused above six dimensions or a conservative budget of `3e7`
search-tree leaves. Candidate reduction bounds the enumeration by
`C(N+s,s)` rather than the full `(N+1)^s` grid.

The leaf budget is a retained conservative work policy. Historical timing
samples lack complete reproduction metadata and do not calibrate a current
wall-clock guarantee, particularly under WebAssembly. Exact computation is
NP-hard in dimension; see Gnewuch, Srivastav and Winker, Journal of Complexity
25(2), 2009. Refusals return an error rather than a partial discrepancy.

`TestStarDiscrepancyAgreesWithBruteForceEnumeration` checks the independent
strict/inclusive reference. Single-point and one-dimensional closed-form tests
cover the cheap paths; `TestStarDiscrepancyRefusesWhatItCannotAfford` covers
multipoint refusals. `TestQMCBeatsPseudorandomOnStarDiscrepancy` is a separate
three-dimensional quality fixture: 512 points, skip 64, scrambling seeds 1..3,
against consecutive random sets from a source seeded 20240825. Its margin
belongs to that fixture, not a general discrepancy ratio.

### Approximate star discrepancy decision

CORE-06 and API-01's review in [PLAN.md](../PLAN.md) retain the exact API.
A sampled lower bound or randomized estimator could serve multipoint sets above
the enumeration budget, but would need a separate result/accuracy contract,
randomness policy, and independent reference validation. No demonstrated caller
currently requires that surface, so it is deferred rather than silently returned
under the exact function's name. Revisit the decision with a concrete workload.

## Centered L2 discrepancy

`CenteredL2Discrepancy` evaluates Hickernell's CD2 closed form in O(N²s) for
general dimensions and returns its square root. Its defining integral includes
the coordinate projections, not just the full-dimensional boxes.
`TestCenteredL2MatchesItsDefiningIntegral` integrates that definition
independently; single-point, reflection, and point-order tests check other
invariants. General workloads still have quadratic pair cost.

### Arithmetic range and precision

Scratch entry and byte counts are checked before allocation. Nonfinite products,
sums, or squared results return an error. The implementation uses direct float64
terms: even when a norm would fit, its squared terms may overflow. A single
origin at 1000 dimensions is supported; 2000 dimensions is a regression for a
finite norm whose intermediate squared product exceeds that range. Larger
nonfinite cases are also refused.

Scaled arithmetic is deferred because it would need to handle cancellation
between differently scaled terms and pass independent reference checks. There
is no fixed dimension-only ceiling; arithmetic range also depends on the
coordinates and point count.

For one dimension, sorted coordinates give the stable identity

```
CD2² = 1/(12N²) + mean((x_(i) - (i-1/2)/N)²),   i = 1..N
```

The terms are nonnegative and the caller's input is not reordered. Midpoint-grid
references through N=16384 test the independent value `1/(sqrt(12)*N)`.

General CD2 uses compensated accumulation but still subtracts near-equal terms.
Product rounding and final cancellation can dominate for sets with squared
discrepancy of order `N^-2`. A random-set `N^-1` error model does not establish
an unconditional significant-digit guarantee. A tiny negative squared value is
clamped to zero, so zero may represent a numerical floor. Exact rational
tensor-grid and defining-integral references check the supported cases.
The [performance report](performance.md) separates generation measurements from
discrepancy costs and does not establish a universal discrepancy timing.

### The independent-uniform reference

For N independent uniform points,

```
E[CD2²] = ((5/4)^s - (13/12)^s) / N
```

Thus `sqrt(E[CD2²])` is the RMS of CD2, not its mean. By Jensen's inequality,
`E[CD2] <= sqrt(E[CD2²])`. A sample mean of CD2 may be close to the RMS when
the distribution is concentrated; closeness is empirical.
`TestCenteredL2MatchesTheRandomExpectation` checks the correct squared
quantity by averaging CD2² over random sets from a source seeded 20240828,
at its specified dimension/count/replicate cases.

### High-dimensional discrimination

`TestCenteredL2SaturatesAtThirtyNineDimensions` uses 39 dimensions, 1024 points,
skip 64, and fixed-scrambling seeds 1..10. Its random sets use consecutive draws
from a source seeded 20240827. It requires the mean CD2 values to remain within
a broad separation margin while the same sets' RMS integration errors differ
on the smooth product. Its comparison of mean random CD2 with the RMS reference
is a finite-fixture proximity check, not an exact expectation identity.

The statistic's rapidly growing diagonal contribution can make relative
separation small in this regime. Each diagonal term includes products across
coordinates of a point; it is not determined solely by independent
one-dimensional marginals. Residual terms still describe relationships between
points, and a small relative difference does not imply they are below arithmetic
roundoff. There is no universal dimension where CD2 becomes meaningless.

Use the independent-uniform RMS reference as a scale comparison. Values close
to that scale need additional evidence about the intended workload; they do not
prove that only marginal spread is being measured. Consider integration against
independent references and manageable projections. Neither these quality
fixtures nor the [canonical integration comparison](performance.md) proves a
ranking for arbitrary functions.

## Browser and reproducible checks

The demo runs bounded computations in workers, renders library refusals, and
labels the analytic CD2 curve as the independent-uniform RMS reference.
[The demo](wasm-demo.md) describes its workload caps, cancellation, and browser
verification. Native benchmark timings are not browser execution deadlines.

The focused mathematical contracts can be reproduced with:

```sh
go test -count=1 -timeout=10m -v -run 'Test(StarDiscrepancy.*|CenteredL2.*|Discrepancy.*|QMCBeatsPseudorandomOnStarDiscrepancy)' .
```

The statistical sweeps are skipped by `-short`; `just test-statistical`
runs the complete suite.
