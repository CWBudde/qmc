# Leaping

`WithLeap(n)` takes every n-th underlying point. Point i uses raw index
`skip + 1 + i*n`; values below 1 are clamped to 1, and leap 1 preserves the
unleaped sequence.

Leaping is deterministic and can change Halton's large-base ramps without a
seed. It gives no seed variability to estimate. Randomized runs have different
assumptions; their seed spread also does not bound bias. See
[Randomization](randomization.md).

Reference: Kocis and Whiten (1997), “Computational Investigations of
Low-Discrepancy Sequences,” ACM Transactions on Mathematical Software 23(2).

## Coprimality is required

If prime base p divides the leap, every raw index has the same remainder
`skip+1` modulo p. That coordinate's first base-p digit is fixed and its
values remain in one strip of width `1/p`. Scrambling maps the constant digit
to another constant digit; it cannot remove this confinement.

Both constructors reject leaps sharing a factor with their bases.
For Halton, choose a leap coprime to every prime in use; a prime greater than
`Bases()[Dims()-1]` suffices. At 39 dimensions the largest base is 167.
Sobol works in base 2, so accepted leaps must be odd.

`TestASharedFactorConfinesTheHaltonCoordinate` demonstrates confinement for
plain, fixed-permutation, and nested Halton configurations.

## Sobol's Gray-code case

A stride in Sobol's raw index is not a stride in its direct-form index because
this implementation uses Gray-code order. The population-count parity of
`gray(m)` is nevertheless `m & 1`. A coordinate whose direction numbers all
have their leading bit set therefore has that parity as its leading bit.

Dimension 1 in the embedded table has this property, so an even leap fixes its
coordinate in one half of the interval. A leap divisible by 4 also fixes
dimension 0's leading bit. `TestAnEvenLeapConfinesASobolCoordinate` and
`TestAnEvenLeapWrecksSobolIntegration` exercise these failure modes.

An odd leap greater than one is accepted, but it no longer visits the complete
aligned raw block required by the usual net guarantee. It also loses the
single-step Gray-code recurrence on NextInto: generation uses indexed work.
Leap 1 retains the ordinary behavior. Prefer measuring digital shifting or
Owen scrambling before adding a Sobol leap solely for decorrelation.

## Measured Halton quality fixtures

`TestLeapingIntegratesBetterThanAnUnleapedSequence` evaluates the smooth product
with integral 1 at 39 dimensions and 4096 points, skip 64. It uses the first
40 prime leaps above 167 for deterministic configurations and seeds 1..40 for
fixed/nested scrambling. MC uses consecutive draws from one `math/rand` source
seeded 20240823. The plain configuration repeats the same estimate; the spread
over leaps is not an error estimate for a single chosen leap.

The gate requires lower RMS error across these chosen leaps than the plain
configuration. It does not require a ranking against scrambling or establish
that leaping wins on other integrands.

`TestLeapingBreaksHighDimensionalCorrelation` uses the first 30 prime leaps
above 167, 39 dimensions, 600 points, and skip 64. It measures each leap's worst
adjacent-pair absolute Pearson correlation and reports the upper-middle median,
the observation at zero-based index 27 for its p90 summary, and the maximum.
That convention differs from the fixed-scrambling test's average-middle median
and nearest-rank p90. Its gate checks a broad maximum-correlation margin; it
does not establish a universal population of “good leaps.”

These fixtures show why integration error and correlation should both be
considered for a chosen stride. A favorable mean over candidate leaps does not
ensure every candidate suits a parameter sweep. Reproduce them with:

```sh
go test -count=1 -timeout=10m -v -run 'Test(LeapingBreaksHighDimensionalCorrelation|LeapingIntegratesBetterThanAnUnleapedSequence|ASharedFactorConfinesTheHaltonCoordinate|AnEvenLeap.*)' .
```

## Cost and browser controls

A Halton leap reaches larger raw indices and can increase the digit workload.
Sobol leaps greater than one lose the stateful recurrence. The
[controlled performance report](performance.md) measures fixed-window throughput,
including leap 173, on the same machine and toolchain as its other configurations.
Its canonical integration comparison does not include leap accuracy; keep the
separate quality fixtures above scoped to their own MC policy.

The browser treats leap as a configuration control, independent of randomization.
Go-side constructor validation determines admissible values; the UI does not
reimplement coprimality. Browser workload caps and worker execution are described
in [the demo](wasm-demo.md).
