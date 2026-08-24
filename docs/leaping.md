# Leaping

`WithLeap(n)` takes every _n_-th point instead of every point: point _i_ becomes raw index
`skip + 1 + i*n` (Kocis & Whiten 1997). It is the third remedy for the high-dimensional
Halton defect, after a burn-in and the two scrambling schemes, and the only deterministic
one — no seed, so a leaped run is plain QMC and reproducible without recording anything.

That is also its limitation: with no seed there is no averaging over seeds, so a leaped run
gives no error estimate the way an RQMC run does.

## A leap must be coprime to every base in use

If a base _p_ divides _n_, every raw index is congruent to `skip+1` mod _p_, so that
coordinate's leading base-_p_ digit never changes and the coordinate is confined to a single
strip of width `1/p`. Measured at 39 dimensions with a leap of 167, dimension 38 spends the
entire run inside a strip covering **0.6%** of its range — while still producing a plausible
spread of values inside it, which is what makes this worth refusing rather than documenting.

Scrambling does not rescue it: a permuted constant digit is still a constant digit, so the
coordinate moves to a different strip of the same width. All three Halton randomizations were
measured showing this.

`NewHalton` and `NewSobol` therefore **refuse** such a leap at construction, naming the
dimension and the base. In practice, pick a prime above the largest base — above 167 at 39
dimensions. On Sobol every base is 2, so the leap must simply be odd.

## Sobol's trap is not the base-2 restatement it looks like

This is the one claim here that had to be measured before it could be written down.

Points are generated in Gray-code order, so a stride in the raw index is not a stride in the
direct-form index, and no bit is obviously pinned. What _is_ pinned is the parity of the
population count of `gray(m)`, which is exactly `m&1` — and that parity is the leading bit of
every dimension whose direction numbers all carry their own leading bit.

Dimension 1 is the only such dimension in the first eight of the embedded table (32 of 32
direction numbers, against dimension 0's 1 of 32). An even leap pins it to one half of
`[0,1)` at every skip tried, taking integration from 2.6e-04 to 1.2e-01.

## Measured: it is the most accurate option for integration

39 dimensions, 4096 points, over forty admissible leaps against forty scrambling seeds — one
run, so these are a comparison rather than four sittings:

| configuration          | RMS relative error | vs Monte Carlo |
| ---------------------- | ------------------ | -------------- |
| unleaped, unscrambled  | 3.9e-03            | 1.4x           |
| `WithLeap`             | **1.0e-04**        | **54.3x**      |
| `WithScrambling`       | 2.2e-04            | 24.4x          |
| `WithNestedScrambling` | 1.3e-04            | 41.1x          |

## The gain is in the average, not the tail

Worst adjacent-pair |r| at 600 points, over thirty leaps:

| configuration                   | median | p90  | worst    |
| ------------------------------- | ------ | ---- | -------- |
| unleaped, unscrambled, skip 64  | —      | —    | **0.81** |
| `WithLeap`, skip 64             | 0.097  | 0.23 | **0.32** |
| `WithScrambling`, skip 64       | 0.093  | 0.13 | **0.16** |
| `WithNestedScrambling`, skip 64 | 0.089  | 0.12 | **0.14** |

The medians agree; the tail does not. A leap only reorders the digits a coordinate visits, so
two dimensions whose bases interact with the leap in commensurate ways still ramp together,
and roughly one leap in ten draws such a pair — the same shape of defect the affine nested
scrambling had. Leaping suits integration, where the average is what you get; it does not
suit a parameter sweep, where the worst case is what you feel.

## It is not free, and not because of the multiply

The multiply is unmeasurable. What costs is that a leaped generator works on raw indices _n_
times larger, so every radical inverse carries about `log(n)` more digits: `AtInto` at 39
dimensions is **634 ns/op** at a leap of 173, against 512 unleaped.

On **Sobol** the option exists for symmetry and is not the one to reach for — Sobol has no
ramp defect to cure. A leap costs `Next` the Gray-code recurrence entirely (**301 ns/op**
against 46.0) and forfeits the (t,m,s)-net balance property at any leap, so an odd leap is
legal there and still measures 8.8e-03 against 1.8e-04 unleaped at 16 dimensions.

## Tests

`TestLeapingBreaksHighDimensionalCorrelation`,
`TestLeapingIntegratesBetterThanAnUnleapedSequence`,
`TestASharedFactorConfinesTheHaltonCoordinate`, `TestAnEvenLeapConfinesASobolCoordinate` and
`TestAnEvenLeapWrecksSobolIntegration` keep every number and every claim above honest.

## In the browser demo

The demo exposes a leap as an independent numeric knob rather than as a randomization, with
`maxLeap` fixed at 1000 by Sobol's 2^32 raw-index ceiling against the convergence sweep's
200,000 points.

The interesting part is the `leaps` export, which answers whether a leap is admissible for
the sequence and dimension count now selected, and which nearby value is. Without it the
control would look broken rather than sparse: at 39 Halton dimensions the smallest admissible
leap is 173, so almost every number a slider can reach is refused. It decides by building a
generator and reading the constructor's error rather than by re-deriving coprimality — the
library is the only place that says what a constructor accepts.
