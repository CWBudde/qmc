# Choosing a sequence

**Sobol unless you have a reason.** It is base 2 in every dimension and does not degrade as
dimensions are added, which is exactly where Halton struggles. It is capped at the 1024
dimensions the embedded direction numbers cover, unless you supply your own table with
`WithDirectionNumbers`.

**Halton** has no dimension ceiling — primes are sieved on demand, so `NewHalton(5000)`
works — and its construction is simple enough to reproduce by hand, which matters if you are
migrating off an existing implementation. Above roughly twenty dimensions it has to be
randomized to be usable at all; see [Randomization](randomization.md).

## The measured comparison

A smooth 39-dimensional product integrand at n=4096 over ten randomization streams, against
the same plain Monte Carlo baseline. Lower error is better; the multiplier is how many times
more accurate than Monte Carlo on the same budget.

| generator                       | RMS relative error | vs Monte Carlo |
| ------------------------------- | ------------------ | -------------- |
| plain Monte Carlo (`math/rand`) | 4.3e-03            | 1x             |
| Halton, random-digit scrambling | 2.4e-04            | **17.7x**      |
| Sobol, digital shift            | 1.5e-04            | **29.5x**      |
| Sobol, Owen scrambling          | 1.4e-04            | **32.0x**      |
| Halton, nested scrambling       | 1.4e-04            | **31.9x**      |

Read that table with two caveats. It is one integrand, and a smooth product with decaying
coefficients is the case nested scrambling suits best. And **ten streams is not many**: the
same measurement over forty seeds moves random-digit Halton to 24.4x and nested Halton to
41.1x, which is a different ordering against Sobol than the ten-stream column shows. The
ratios move with the integrand and with the stream count, which is why the suite gates the
generators at a factor of five against Monte Carlo rather than at any of these numbers — see
[Testing methodology](testing-methodology.md).

At 40 points rather than 4096 the picture is different again; see
[the small-sample regime](small-sample-regime.md).

## Sobol's two balance properties

Each one makes a correct sequence look broken, so both are worth knowing before you test it.

**The (t,m,s)-net property is 2^m-aligned.** The first 2^m points landing one apiece in every
elementary interval holds on a 2^m-_aligned_ block of raw indices, so a stratification check
wants `WithSkip(2^m - 1)`. With the default skip of 0, all 40 of the first 40 dimensions come
out unbalanced at m=8; at skip 255, none of them do. The alignment is stated on the type and
on `At`.

**Not every projection is a net.** The D(6) direction numbers optimise two-dimensional
projections without making them all nets. Of the 780 pairs among the first 40 dimensions, 18
are balanced at every split at m=8 and 4 at m=10. Plot dimensions 0 and 1 and you get one
point per cell at every aspect ratio; plot 12 and 23 over the same 256 points and 224 of the
256 cells of the 16x16 grid are empty while one holds eight. Both are the correct table
behaving correctly.

## Dimension ceilings

**Sobol** covers 1024 dimensions from the embedded Joe & Kuo direction numbers.
`WithDirectionNumbers(r io.Reader)` takes a caller's own table in the same format for more —
upstream publishes the same construction out to 21201 dimensions at
<https://web.maths.unsw.edu.au/~fkuo/sobol/>, and `new-joe-kuo-6.21201` can be passed whole.
The format and the invariants a table has to satisfy (contiguous _d_ from 2, exactly _s_
direction numbers per row, every _m_i_ odd and below 2^_i_, a primitive polynomial) are
documented on `WithDirectionNumbers`; anything failing them is refused at construction. What
the validator cannot prove is that the numbers came from the authors' search — direction
numbers cannot be derived, so a table is the only honest option. A synthesised
1200-dimension table, whose polynomials are found by running `isPrimitiveOverGF2` rather than
asserted, drives the loader end to end in the test suite.

**Halton** has no fixed base table. Primes are sieved on demand, so `NewHalton(500)` works.
Scrambling allocates one permutation per dimension, sized by that dimension's prime, so
memory grows roughly as the sum of the first _d_ primes, four bytes per entry. That is about
12 KB at 39 dimensions and 3.5 MB at 500, but the sum grows faster than _d_ does: 5000
dimensions cost around 475 MB, so scrambling at that scale is a decision to make on purpose.
The construction cost is in [Performance](performance.md).
