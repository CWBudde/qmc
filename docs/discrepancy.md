# Discrepancy

Two ways to measure how evenly a point set fills the cube, and they fail in opposite
directions: one is exact and cannot be computed above a handful of dimensions, the other is
cheap in any dimension and stops meaning anything in high ones.

`Draw(seq, n)` collects `n` points into a matrix for either of them. It is built on `AtInto`,
so it leaves the generator's cursor where it was and the same matrix drawn twice is the same
matrix; the rows alias one backing array, which is two allocations and the difference between
a cache-resident inner loop and a scattered one.

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

## `StarDiscrepancy` — exact, and it refuses

It returns the exact `D*_N`: the largest relative error any origin-anchored box makes about
how much of the cube it covers, which is the quantity the Koksma-Hlawka bound multiplies by
an integrand's variation. It is a supremum, not a sample and not a lower bound.

Both halves are enumerated over the same grid — the overshoot with the corner counted
strictly, the undershoot with it counted inclusively. Taking only one returns a lower bound
that happens to be right in exactly the small cases anyone would hand-check, which is why the
test checks it against a brute-force enumeration instead of against intuition.

Scrambled Halton against `math/rand`, ten seeds each; lower is better:

| s   | N   | scrambled Halton | `math/rand` | ratio      |
| --- | --- | ---------------- | ----------- | ---------- |
| 1   | 512 | 0.001953         | 0.039848    | **20.40x** |
| 2   | 64  | 0.045907         | 0.148041    | 3.22x      |
| 2   | 512 | 0.009727         | 0.053237    | **5.47x**  |
| 3   | 512 | 0.017966         | 0.069954    | 3.89x      |
| 4   | 160 | 0.055348         | 0.128141    | 2.32x      |

The ratio decays with the dimension count and improves with the point count, which is the
shape the theory predicts.

### The refusal sees N as well as s

Restricting each dimension's candidates to the surviving points' own coordinates is exact and
turns the naive (N+1)^s grid into C(N+s,s) ≈ N^s/s! leaves, but the problem is NP-hard in the
dimension (Gnewuch, Srivastav & Winker, _Journal of Complexity_ 25(2), 2009), so no amount of
tuning moves the limit far. The ceiling is arithmetic, not an unfinished optimisation.

A dimension ceiling alone would not have been a gate — 5 dimensions and 3000 points is inside
6 dimensions and is 2.0e15 leaves — so there is a **work budget of 3e7 leaves** beside it.

The budget is calibrated, not asserted. `BenchmarkStarDiscrepancy` walks the two shapes that
bracket the tree: 1024 points in 2 dimensions (wide and shallow, 5.26e5 leaves, 14.6 ms) and
160 points in 4 dimensions (narrow and deep, 2.91e7 leaves, 764 ms). Those are 27.7 and 26.3
ns per leaf — **flat across shapes**, which is the only thing that makes a leaf count a usable
proxy for wall clock. So 3e7 leaves is about 0.8 seconds. That is the line: a wait, not a
hang, because a caller cannot tell a hang apart from a slow machine.

Affordable point counts are **7744 at 2 dimensions, 562 at 3, 161 at 4, 78 at 5 and 49 at 6**
— which is why the s=4 row above stops at 160. The refusal names them, names which of the two
gates tripped, and points at `CenteredL2Discrepancy` _with_ its caveat attached rather than
bare.

## `CenteredL2Discrepancy` — cheap, and it saturates

Hickernell's CD2 in closed form, O(N²s) in any dimension, returning the square root: 24.5 ms
at 39 dimensions and N=1024, 484 ms at N=4096. It averages over the same family of boxes
instead of taking a supremum.

The two easy mistakes in the construction — anchoring the boxes at the cube's centre rather
than at the nearest corner, and summing over the full-dimensional projection rather than all
2^s − 1 of them — both leave a plausible-looking number, so the test integrates the definition
numerically rather than trusting the formula.

### Read this before believing a CD2 number

For N independent uniform points the expectation is exactly

```
E[CD2²] = ((5/4)^s − (13/12)^s) / N
```

which is 2.4198 at 39 dimensions and N=1024, and matches the measured random figure of 2.4046
to 0.6%. It grows fast enough with _s_ that a good point set and a random one converge onto it
together. Measured at N=1024 over ten seeds:

| s   | CD2 Halton | CD2 random | analytic | random ÷ Halton | diagonal share |
| --- | ---------- | ---------- | -------- | --------------- | -------------- |
| 2   | 0.001370   | 0.017054   | 0.019488 | **12.45x**      | 402%           |
| 5   | 0.006376   | 0.040198   | 0.039026 | 6.30x           | 196%           |
| 10  | 0.033736   | 0.080573   | 0.083190 | 2.39x           | 131%           |
| 15  | 0.097509   | 0.159105   | 0.156561 | 1.63x           | 113%           |
| 20  | 0.220000   | 0.280846   | 0.282599 | 1.28x           | 106%           |
| 30  | 0.818767   | 0.885773   | 0.882090 | 1.08x           | 101%           |
| 39  | 2.365729   | 2.404613   | 2.419777 | **1.02x**       | 100.4%         |

**The last row is not a verdict on the sequence**, and taking it as one would contradict the
integration table. Over the very same 39-dimensional 1024-point sets, RMS integration error is
8.06e-04 for QMC against 1.32e-02 for Monte Carlo — **16.4x**. The statistic, not the point
set, is what has stopped working.

**The statistic becomes its own diagonal.** The last column says why: at 39 dimensions the
`i = j` terms of the double sum account for 100.4% of the expectation on their own, and the
diagonal depends only on each coordinate's marginal spread, not at all on how the points sit
relative to one another. Everything CD2 was meant to measure lives in a residual smaller than
the rounding of the terms around it.

**It is a decay, not a cliff** — 12.5x at 2 dimensions, 2.4x at 10, 1.28x at 20, 1.02x at 39.
Informative below roughly ten dimensions, weak by twenty, dead by thirty, and no dimension at
which a refusal would be honest. That is why this function returns a number where
`StarDiscrepancy` returns an error.

**The self-check.** Compare your number against `sqrt(((5/4)^s − (13/12)^s)/N)`. If it is not
several times below that, the statistic is telling you about your marginals and nothing else,
and you want an integration test or `StarDiscrepancy` in a projection instead.

## In the browser demo

The Discrepancy Bench sweeps either statistic against n beside a pseudorandom baseline and,
for CD2, the analytic curve. At 39 dimensions all three lie on top of one another at a ratio
of 1.02x — the saturation argument as a picture rather than as a caveat. At 4 dimensions star
separates the same two point sets by 1.71x over six rungs.

Star's availability is asked of the library and its refusal rendered verbatim, with the
largest admissible dimension count offered as the fix. The browser needs a second ceiling
below the library's, and the obvious cost model is wrong: measured under `js/wasm` the cost
per pair is **affine** in the dimension count, `N(N−1)/2 * (5.7s + 7.5)` ns, so a purely
proportional model would have been three times too generous at one dimension. Star at the
library's own budget freezes the tab for up to 5.6 seconds, so the panel affords 224 points at
3 dimensions and 32 at 6.

## Still open

There is no lower-bound or randomized estimator for the star discrepancy above 6 dimensions,
which is the only way that quantity is reachable at the dimension counts this package is aimed
at. It would be an approximation with its own error to characterise, and nothing in the package
needs it yet.
