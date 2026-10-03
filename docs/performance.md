# Performance

Use the controlled review benchmarks for comparisons. Historical timings in
other source comments were collected with different index ranges and sometimes
different machines. They are not a comparable current baseline; DOC-01 tracks
their remaining consolidation in [PLAN.md](../PLAN.md).

## Reproduce and inspect the evidence

On Linux, select a permitted logical CPU and run:

```sh
taskset -c 2 just measure-performance /tmp/qmc-performance-new-run
```

The output directory must be new. On platforms without taskset, invoke the same
recipe directly. The recorder pins the compiler from tools/go-version, disables
external workspaces/caller GOFLAGS, uses GOMAXPROCS=1, and records platform,
affinity, source commit, dirty-state status, source hashes, exact command, five
sequential 200-ms benchmark repeats, compatibility checks, and a separate CPU
profile. No other tests or benchmarks ran concurrently with these campaigns.
Run ordinary `just bench` for the wider collection, including discrepancy and
scratch-threshold benchmarks; it does not establish a fixed-window comparison.

The 2026-10-03 campaigns used Go 1.26.1, linux/amd64, GOAMD64=v1, an Intel
i7-1255U, CPU-2 affinity, and one Go execution thread. Throughput always cycles
over point indices 0..4095 at 39 dimensions, skip 64, seed 1, and leap 1 unless
specified. Stateful benchmarks reset every 4096 points. Calibration cannot
silently increase the digit workload by extending the sampled indices.

Raw [before](measurements/performance-2026-10-03/before-benchmarks.txt) and
[after](measurements/performance-2026-10-03/after-benchmarks.txt) results have
matching [before](measurements/performance-2026-10-03/before-environment.json) and
[after](measurements/performance-2026-10-03/after-environment.json) input metadata.
The before library is commit `9f64761`, with the added test-only review harness;
both captures explicitly record a dirty worktree. The after capture includes
the root-cache implementation. Source hashes identify each measured snapshot.
Timings varied materially in the first campaign, so a separate
[cache recheck](measurements/performance-2026-10-03/cache-recheck.txt) preceded
implementation. The final campaign corroborates its result. Medians and ranges
below describe these observations, not confidence intervals or speed guarantees
for every compiler, index range, CPU frequency, or machine.

## Current per-point throughput and allocations

All rows below allocate 0 bytes and 0 objects per Into call in this configuration.
Times are nanoseconds; brackets are the minimum and maximum of five repeats.

| Configuration                   | AtInto median [range] | NextInto median [range] |
| ------------------------------- | --------------------- | ----------------------- |
| Halton, plain                   | 284.4 [275.3, 303.6]  | 273.6 [270.7, 283.5]    |
| Halton, fixed digit permutation | 357.6 [348.9, 404.0]  | 357.3 [350.3, 361.3]    |
| Halton, nested with root cache  | 7202 [7124, 7238]     | 7691 [7452, 8505]       |
| Halton, plain, leap 173         | 436.3 [414.1, 457.8]  | 421.3 [408.4, 451.8]    |
| Sobol, plain                    | 162.9 [159.1, 170.7]  | 57.61 [56.27, 68.98]    |
| Sobol, digital shift            | 169.3 [165.6, 172.4]  | 57.98 [54.78, 78.79]    |
| Sobol, hash-based Owen          | 287.6 [280.7, 430.5]  | 192.5 [184.5, 204.3]    |
| Sobol, plain, leap 173          | 227.7 [221.7, 230.8]  | 221.3 [221.0, 231.2]    |

Keep Sobol's Gray-code recurrence and contiguous direction storage. The
stateful plain/shift paths benefit clearly from consecutive raw indices; leap
173 loses that recurrence. For Halton, larger raw indices require more digits.
Nested scrambling costs about 20 times fixed scrambling here after caching,
compared with about 39 times in the before indexed benchmark. Earlier 8x/40x
figures are not portable current constants.

## Construction and retained memory

Repeated constructor benchmarks use already initialized embedded Sobol metadata.
The review wrapper also contributes a few configuration allocations. Current
median times and allocated bytes per construction are:

| Dimensions | Halton plain        | Halton fixed permutation | Halton nested         | Sobol plain           |
| ---------- | ------------------- | ------------------------ | --------------------- | --------------------- |
| 39         | 1.114 µs / 2080 B   | 18.113 µs / 15408 B      | 16.679 µs / 15120 B   | 6.245 µs / 5744 B     |
| 500        | 12.933 µs / 24736 B | 4.950 ms / 3506936 B     | 104.894 µs / 95184 B  | 177.382 µs / 67792 B  |
| 1000       | 52.448 µs / 49312 B | 23.369 ms / 15622264 B   | 139.460 µs / 123856 B | 326.234 µs / 135376 B |

Raw results include ranges and allocation counts. A separate fresh-process,
one-iteration [Sobol first-call measurement](measurements/performance-2026-10-03/cold-sobol.txt)
records 1.226 ms, 445216 B, and 4104 allocations at 39 dimensions, including
the one-time table parse/validation. It is a single cold observation, distinct
from the repeated warm measurements and from process startup.

Fixed digit scrambling allocates one int32 permutation per dimension: payload
is `4 * sum(first d primes)` bytes, plus slices, allocator rounding, generator
state, and sieve work. The growth is faster than linear in dimensions. Reuse
generators across a run instead of constructing one per point or tiny request;
their indexed methods can be shared with separate destination buffers. This
cost is also stated at `NewHalton`'s call site.

The production nested cache uses one flat immutable table and prefix offsets.
Digit-table payload is at most 64 KiB independent of requested dimensions and
sample count. Roots remain O(dimensions); offsets cover only cached roots and
are themselves bounded by the entry budget. At 39 dimensions, the table payload
is 11656 B. Constructor allocation increases from 2456 to 15120 B in the review
wrapper; at 500/1000 dimensions the increase is a fixed 66296 B. Construction
work increases to amortize repeated first-digit evaluation, so small one-point
requests can lose even though reused generators win.

## Decisions from the experiments

The test-only prototypes live in performance_review_test.go. Production keeps
the same valid-input seeded coordinates and the same indexed concurrency contract.

- **Root permutations: implemented.** Production indexed nested throughput
  changes from 15445 ns to 7202 ns median. In the final same-process prototype
  comparison, uncached/root medians are 14896/7727 ns. Complete root shuffles
  replace repeated lazy first-digit work with immutable lookups; children and
  the zero tail retain the original hashes and arithmetic. Exact-value,
  dimension-prefix, maximum-index, architecture, and concurrent-read tests
  cover the change. No map is populated during sampling.
- **Shallow nodes: deferred.** A feasible 64-KiB table-budget prototype measures
  7114 ns versus root-only's 7727 ns, with overlapping timing ranges. At 39
  dimensions its table payload grows from 11656 to 64756 B, prototype allocated
  storage from 14320 to 79944 B, and extra-cache construction from 15.855 to
  86.806 µs. This modest incremental throughput benefit does not justify the
  additional memory, construction work, and path-selection logic by default.
- **Every node: rejected.** The separately rerun
  [counter](measurements/performance-2026-10-03/full-tree.txt) visits 1982974
  nodes, 1544674 distinct, in 39 dimensions over 4096 points: reuse 1.284. Full
  permutation entries plus keys/slice headers would require about 381.9 MB,
  excluding map buckets. This is an estimated full-cache footprint; the counter
  allocates visited-node sets, not all those permutations. A lazy mutable map
  would also need synchronization. This result does not rule out bounded caches.
- **Base-2 bit reversal: deferred.** The full-point prototype measures
  240.4 ns versus original arithmetic's 265.6 ns. It changes 1019 of 4096
  sampled full-width base-2 float results, however: rounding once after integer
  reversal differs from descending floating accumulation. It cannot replace
  the reproducible path as a drop-in performance change.
- **Reciprocal arithmetic: rejected as a replacement.** The candidate measures
  289.5 ns, changes seven boundary/reference coordinates, and has maximum
  observed absolute difference `5.55e-17`. Candidate/compiler/clamp costs are
  part of this timing; this is not a claim that multiplication is intrinsically
  slower than division. Reproducibility already rules out replacing the current
  arithmetic without an intentional versioned output change.
- **Dimension-first bulk fill: no new API.** At 1024 points and 39 dimensions,
  the reused-buffer AtInto loop measures 248426 ns and the specialized plain
  dimension-first prototype 244754 ns, with overlapping ranges and zero
  allocations. The initial noisy run's larger apparent advantage did not
  persist. The evidence does not justify a new cross-generator batch contract.
  Keep caller-owned Into buffers and Draw's contiguous matrix. API-01's
  [design decision](api-design.md) records why another allocation surface is
  deferred until representative consumer evidence exists.

The separate [CPU profile](measurements/performance-2026-10-03/after-profile-top.txt)
covers the arithmetic and bulk candidates, not a universal application workload.
It confirms that digit evaluation dominates these particular loops. Profiled
timings are kept separate from the main results.

## End-to-end integration cost and error

BenchmarkReviewIntegration includes construction, indexed generation, and
evaluation of `product_k(1 + (x[k]-0.5)/(k+1))`, whose integral is 1. Each
operation runs 40 streams of 4096 points in 39 dimensions, skip 64, leap 1;
randomization seeds are 1..40. MC uses a fresh math/rand stream with seed
`20240823 + stream`. These are the current canonical seeds for this comparison.
The sampled Sobol block is not a power-of-two-aligned raw block; the measured
accuracy is not a claim of optimal Sobol net usage.

| Configuration                   | Median time for all 40 streams | RMS relative error | Estimated RMS SE |
| ------------------------------- | ------------------------------ | ------------------ | ---------------- |
| MC                              | 23.317 ms                      | 5.131e-3           | 4.683e-4         |
| Halton, fixed digit permutation | 64.837 ms                      | 2.226e-4           | 1.970e-5         |
| Halton, nested with root cache  | 1.287 s                        | 1.322e-4           | 1.120e-5         |
| Sobol, digital shift            | 36.893 ms                      | 1.568e-4           | 1.980e-5         |
| Sobol, hash-based Owen          | 58.990 ms                      | 1.277e-4           | 1.460e-5         |

All reported error/SE metrics are unchanged by the cache. Nested's before
median time is 2.723 s. It improves this integrand's accuracy over fixed digit
scrambling at substantially greater cost, so a per-point accuracy ranking is
insufficient to choose an option under a wall-clock budget. Sobol's stateful
recurrence is another available cost reduction when sequential access fits.

RMS is `sqrt(mean(error²))` over the stated finite seeds. RMS SE uses the
delta-method variance of error squared, treating streams as independent
replicates; it is an approximate sampling-variability description, not a
confidence claim for arbitrary PRNG hashes, bias, or other integrands. In
particular, fixed digit scrambling is not an unbiased uniform-marginal scheme.
Neither more benchmark repeats nor this seed spread measures its systematic
bias. See [randomization](randomization.md) and the diverse integration gates.

## Into allocation contract

AtInto and NextInto avoid result-slice allocation. Sobol and plain/fixed
digit-scrambled Halton allocate no per-call scratch. Nested Halton uses stack
scratch for prime bases at most 512 and one heap scratch allocation per
coordinate above that threshold: 0 at 97 dimensions, 1 at 98, and 3 at 100.
The root cache preserves this contract. Allocation regressions cover both
methods; BenchmarkNestedHaltonScratchThreshold measures the boundary workload.
Shared mutable scratch would break concurrent indexed reads. The documented
fallback stays in place; [API design](api-design.md) records the decision to
defer a caller-owned workspace until a representative consumer demonstrates need.

## Discrepancy and browser workloads

Discrepancy has separate work/precision budgets in [discrepancy](discrepancy.md).
The demo uses bounded worker computations and measured responsiveness rather
than treating old native or WASM timings as universal deadlines; see
[the demo](wasm-demo.md). The generation experiments above do not recalibrate
discrepancy or establish a browser execution-time guarantee.
