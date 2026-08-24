# Performance

Benchmarks live in `bench_test.go` (Halton) and `sobol_bench_test.go` (Sobol). Run them with
`just bench`.

## Read the numbers with this caveat first

**The Halton figures recorded in `bench_test.go` were taken on a different machine from the
Sobol ones**, so they are not comparable to each other. The same-machine numbers are in
`sobol_bench_test.go`'s header. Re-measuring all of them together is worth doing before anyone
optimises against them.

Nothing has been optimised against these benchmarks yet.

## Per-point cost

At 39 dimensions:

| call                                | ns/op |
| ----------------------------------- | ----- |
| Sobol `NextInto`, Owen              | 197   |
| Sobol `NextInto`, digital shift     | 65    |
| Sobol `NextInto`, leaped            | 301   |
| Sobol `NextInto`, unleaped baseline | 46.0  |
| Sobol `AtInto`, Owen                | 370   |
| Sobol `AtInto`, digital shift       | 360   |
| Halton `AtInto`, unleaped           | 512   |
| Halton `AtInto`, leap 173           | 634   |

Two of those are structural rather than incidental:

- Owen scrambling costs `NextInto` roughly 3x because the Gray-code recurrence is precisely
  what cannot carry a non-linear scramble. On `AtInto`, where there is no recurrence to lose,
  it is nearly free.
- A leap costs because the generator works on raw indices _n_ times larger, so every radical
  inverse carries about `log(n)` more digits. The multiply itself is unmeasurable. On Sobol a
  leap additionally costs `Next` the Gray-code recurrence entirely. See
  [Leaping](leaping.md).

## Construction cost

Scrambled construction is expensive and worth knowing about before you build a generator per
task: `NewHalton(1000, WithScrambling(…))` costs about **32 ms** and allocates proportionally
to the sum of the first 1000 primes. Memory grows roughly as that sum, four bytes per entry —
about 12 KB at 39 dimensions, 3.5 MB at 500, and around 475 MB at 5000.

`NewSobol` builds its direction-number table from an embedded file and does not have this
shape.

## Discrepancy cost

- `StarDiscrepancy`: about **27 ns per search-tree leaf**, flat across tree shapes. 14.6 ms at
  2 dimensions and N=1024; 764 ms at 4 dimensions and N=160. The 3e7-leaf budget is calibrated
  against exactly this. See [Discrepancy](discrepancy.md).
- `CenteredL2Discrepancy`: O(N²s). 24.5 ms at 39 dimensions and N=1024; 484 ms at N=4096.

## The WebAssembly demo's cost model

Measured under `js/wasm`, CD2's cost per pair is **affine** in the dimension count:

```
N(N−1)/2 * (5.7s + 7.5) ns
```

A purely proportional model would have been three times too generous at one dimension. This
is why the demo carries its own ceiling below the library's: star discrepancy at the library's
budget freezes the tab for up to 5.6 seconds, so the panel affords 224 points at 3 dimensions
and 32 at 6.

## Known optimisation opportunities

None of these has been measured against the benchmarks yet.

- Both inner loops divide by a non-constant base once per digit. A precomputed reciprocal, or
  specialising the base-2 case that dimension 0 always uses, is the obvious first cut.
- There is no bulk API. The WebAssembly demo — the heaviest known consumer — calls `AtInto` in
  a tight loop across tens of thousands of points. An `AtBatch(from, n, dst)` amortising setup
  per dimension rather than per point is worth measuring.
- The Halton and Sobol benchmarks were taken on different machines and are not comparable to
  each other. Re-measure them together before optimising against either.
- Scrambled construction cost is not documented at the call site. A thousand-dimensional
  scrambled Halton generator costs ~32 ms to build, and a caller constructing one per task
  needs to know that before it shows up as a profile.

Already done: `fill` no longer re-tests `h.perms == nil` on every coordinate of every point —
the branch is hoisted and the dispatch is three-way. `Sobol.NextInto` does the same for its
Owen branch, which is where it matters most.
