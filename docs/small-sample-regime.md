# The small-sample regime: 40 points in up to 30 dimensions

Every other measurement in this package is taken where quasi-Monte Carlo is
supposed to win. `integration_test.go` integrates at n=4096, `discrepancy_test.go`
at n=1024, and both report the 1/n-versus-1/sqrt(n) gap in the tens.

The caller that actually shipped against this library does none of that.
`mayfly.WithQMCInitialPopulation` seeds a population of 40 individuals in up to
30 dimensions and never asks for point 41. Forty points in thirty dimensions is
not a low-discrepancy point set in any useful sense — it is the first two levels
of a stratification that would need 2^30 points to complete — and whether the
scrambling constants tuned at n=4096 still buy anything there was an open
question.

This document answers it. Every figure below comes from a `go test -v` run of
`small_sample_test.go` in this repository; the test function that produces each
table is named above it, so any number here can be re-derived with a single
command.

## Method

- Integrand: the smooth product f(x) = prod_k (1 + (x_k - 1/2)/(k+1)), whose
  integral over the unit cube is exactly 1. It is defined once, as
  `productIntegrand` in `integration_test.go`.
- Error: root-mean-square relative error of the estimate over independent
  randomization seeds.
- **200 streams**, for every cell of every table. Ten seeds — what the n=4096
  tests use — is enough to separate a factor of twenty and nowhere near enough
  to separate two good schemes from each other: the RMS of ten squares carries a
  relative standard error near 1/sqrt(2\*10) = 22%, so two schemes 15% apart are
  indistinguishable. At 200 seeds that standard error is about 5%. Two figures
  in these tables that differ by less than roughly 5% should be read as a tie.
- Baseline: `math/rand`, seeded from a fixed constant, consecutive draws from a
  single source (`mcRMSError`, `integration_test.go`).
- All generators are built with `WithSkip(64)`, as the rest of the suite does.
- Fully deterministic: fixed seeds throughout, no time-based randomness.

## Integration error

Produced by `TestSmallSampleIntegration` in `small_sample_test.go`.

RMS relative error, and in parentheses the factor by which it beats the
`math/rand` baseline at the same budget. Higher factors are better.

|   s |   n | Monte Carlo | Halton random-digit | Halton nested       | Sobol Owen          | Sobol digital shift |
| --: | --: | ----------- | ------------------- | ------------------- | ------------------- | ------------------- |
|   2 |  40 | 4.9525e-02  | 1.2516e-02 (3.96x)  | 5.4720e-03 (9.05x)  | 4.1374e-03 (11.97x) | 9.4336e-03 (5.25x)  |
|   2 | 160 | 2.6375e-02  | 2.4344e-03 (10.83x) | 1.0943e-03 (24.10x) | 4.2911e-04 (61.47x) | 2.9667e-03 (8.89x)  |
|  10 |  40 | 5.4336e-02  | 1.4339e-02 (3.79x)  | 8.9587e-03 (6.07x)  | 8.8435e-03 (6.14x)  | 1.4237e-02 (3.82x)  |
|  10 | 160 | 2.7817e-02  | 3.0493e-03 (9.12x)  | 2.1203e-03 (13.12x) | 2.3453e-03 (11.86x) | 3.7724e-03 (7.37x)  |
|  30 |  40 | 5.7161e-02  | 1.6804e-02 (3.40x)  | 1.0999e-02 (5.20x)  | 1.1152e-02 (5.13x)  | 1.5030e-02 (3.80x)  |
|  30 | 160 | 2.8487e-02  | 3.8631e-03 (7.37x)  | 2.7350e-03 (10.42x) | 2.8958e-03 (9.84x)  | 4.2566e-03 (6.69x)  |

Three things this says.

**The advantage survives all the way down.** The worst cell in the grid is
random-digit Halton at s=30, n=40, and even there randomized QMC integrates
3.40x more accurately than independent sampling. Nothing measured is worse than
Monte Carlo. The open question in `PLAN.md` — whether the scrambling constants
are right at n=40 — is answered: they are not merely harmless there. At n=40
they still pay between 3.40x (random-digit Halton at s=30) and 11.97x (Owen
Sobol at s=2).

**Dimension costs more than sample size does.** Going from s=2 to s=30 at fixed
n=40 takes the best scheme from 11.97x down to 5.13x. Going from n=160 to n=40
at fixed s=30 takes it from 9.84x to 5.13x. Both hurt; neither is fatal.

**The rate is still visible at these sizes.** At s=2 every scheme's advantage
grows when the budget goes from 40 to 160 points (3.96 -> 10.83, 9.05 -> 24.10,
11.97 -> 61.47, 5.25 -> 8.89). That growth is the 1/n-versus-1/sqrt(n) gap
appearing directly, and it is one of the two things the test asserts.

### Gates

`TestSmallSampleIntegration` asserts only two things, both far from the measured
values:

1. Every cell of the grid beats Monte Carlo by at least **1.5x** — less than
   half the worst measured value of 3.40x, so a change of constants that costs
   accuracy still passes and only a change that has given up the low-discrepancy
   property fails.
2. At s=2 the advantage does not shrink from n=40 to n=160. This is a trend, not
   a level; a generator whose error had stopped falling faster than sqrt(n)
   would fail it while passing every level gate in the package.

## Does the n=40 ranking match the n=4096 ranking?

Produced by `TestSmallSampleRankingMatchesLargeSample` in `small_sample_test.go`,
which measures both budgets in one run, at s=30, on the same seeds — so this is
not a comparison against a figure quoted from another file that may have drifted.

| Rank | n=40                | RMS        | n=4096              | RMS        |
| ---: | ------------------- | ---------- | ------------------- | ---------- |
|    1 | Halton nested       | 1.0999e-02 | Sobol Owen          | 1.2256e-04 |
|    2 | Sobol Owen          | 1.1152e-02 | Halton nested       | 1.2776e-04 |
|    3 | Sobol digital shift | 1.5030e-02 | Sobol digital shift | 1.4397e-04 |
|    4 | Halton random-digit | 1.6804e-02 | Halton random-digit | 2.1117e-04 |

**The two orderings agree everywhere except in the top pair, and that pair is a
statistical tie at both budgets.** Owen-scrambled Sobol leads nested Halton by
4.2% at n=4096; nested Halton leads Owen Sobol by 1.4% at n=40. Both gaps are
inside the ~5% standard error of an RMS over 200 streams. The bottom two hold
their places exactly, and the gap between the pairs is real at both sizes: the
slower of the top pair beats the faster of the bottom pair by 35% at n=40 and by
13% at n=4096.

So the repo's ranking is not overturned at n=40. What changes is that the
first-place margin, already narrow at n=4096, disappears entirely.

The test asserts only the large-sample half — that Owen-scrambled Sobol is best
of the four at n=4096, since that is the claim the documentation rests on. The
n=40 ordering is logged and compared but not asserted, because pinning an order
across a 1.4% gap would be pinning noise.

## Discrepancy

Produced by `TestSmallSampleDiscrepancy` in `small_sample_test.go`. Centered L2
(CD2) is averaged over the same 200 stream seeds; the analytic column is the
i.i.d. expectation sqrt(((5/4)^s - (13/12)^s)/N). Star discrepancy is computed
exactly, and only at s=2 and s=3, where n=40 and n=160 are far inside
`StarDiscrepancy`'s leaf budget. Lower is better in every column.

### Centered L2

|   s |   n | random  | analytic i.i.d. | Halton random-digit | Halton nested    | Sobol Owen       | Sobol digital shift |
| --: | --: | ------- | --------------- | ------------------- | ---------------- | ---------------- | ------------------- |
|   2 |  40 | 0.09641 | 0.09860         | 0.02706 (0.281x)    | 0.02767 (0.287x) | 0.02561 (0.266x) | 0.02544 (0.264x)    |
|   2 | 160 | 0.04871 | 0.04930         | 0.00702 (0.144x)    | 0.00726 (0.149x) | 0.00741 (0.152x) | 0.00731 (0.150x)    |
|   3 |  40 | 0.13124 | 0.13055         | 0.04218 (0.321x)    | 0.04197 (0.320x) | 0.04218 (0.321x) | 0.04185 (0.319x)    |
|   3 | 160 | 0.06570 | 0.06527         | 0.01196 (0.182x)    | 0.01231 (0.187x) | 0.01353 (0.206x) | 0.01324 (0.202x)    |
|  10 |  40 | 0.42158 | 0.42091         | 0.29290 (0.695x)    | 0.29490 (0.700x) | 0.28469 (0.675x) | 0.28500 (0.676x)    |
|  10 | 160 | 0.20803 | 0.21046         | 0.11934 (0.574x)    | 0.11968 (0.575x) | 0.11958 (0.575x) | 0.11958 (0.575x)    |
|  30 |  40 | 4.45170 | 4.46306         | 4.33268 (0.973x)    | 4.35720 (0.979x) | 4.32191 (0.971x) | 4.30230 (0.966x)    |
|  30 | 160 | 2.23481 | 2.23153         | 2.13713 (0.956x)    | 2.14015 (0.958x) | 2.08524 (0.933x) | 2.08328 (0.932x)    |

### Star discrepancy (exact)

|   s |   n | random  | Halton random-digit | Halton nested    | Sobol Owen       | Sobol digital shift |
| --: | --: | ------- | ------------------- | ---------------- | ---------------- | ------------------- |
|   2 |  40 | 0.18874 | 0.07715 (0.409x)    | 0.08082 (0.428x) | 0.07887 (0.418x) | 0.07702 (0.408x)    |
|   2 | 160 | 0.09749 | 0.02439 (0.250x)    | 0.02615 (0.268x) | 0.02663 (0.273x) | 0.02577 (0.264x)    |
|   3 |  40 | 0.23111 | 0.11575 (0.501x)    | 0.11658 (0.504x) | 0.11742 (0.508x) | 0.11642 (0.504x)    |
|   3 | 160 | 0.11887 | 0.04111 (0.346x)    | 0.04287 (0.361x) | 0.04507 (0.379x) | 0.04394 (0.370x)    |

What the discrepancy view says:

- **The random baseline reproduces the analytic expectation everywhere**, to
  within 2.2% at the worst cell (s=2, n=40). That is the control: it confirms
  the statistic is being computed correctly at these sample sizes, so the QMC
  columns mean something.
- **In low dimensions both statistics separate the point sets cleanly.** At
  s=2, n=40, CD2 puts the QMC sets at 0.26–0.29x the random value and star at
  0.41–0.43x. Forty points really are stratified in two dimensions.
- **CD2 goes blind by s=30 at these sample sizes.** At s=30, n=40 every scheme
  lands between 0.966x and 0.979x of the random baseline — a 2–3% separation,
  when the same point sets integrate 3.4x to 5.2x better. This is the same
  saturation `TestCenteredL2SaturatesAtThirtyNineDimensions` documents at
  n=1024, and small n does not rescue it: the (5/4)^s term dominating
  (13/12)^s is a statement about dimension, and it swamps everything else.
- **Neither statistic ranks the randomizations.** At every (s, n) the four
  schemes' discrepancies sit within a few percent of each other, in an order
  that does not match the integration ranking. Discrepancy at n=40 tells you
  _that_ a point set is not i.i.d. uniform (in low dimensions), not _which_
  randomization to choose.

The only gate here is one-sided and loose: at s=2, every scheme's star
discrepancy must be below the random baseline's. That is the weakest statement
that still means "these are not the same point sets". **No gate is placed on CD2
at all**, because what this test documents is that CD2 _cannot_ tell the point
sets apart at 30 dimensions — asserting a separation would be asserting the
opposite of the finding.

## What this means for a caller

If you are seeding a 40-member population in 30 dimensions:

**Use a randomized QMC draw.** It is worth about 5x in integration accuracy over
independent uniform sampling at exactly that shape, and the worst option in the
package is still worth 3.4x. There is no cell of the measured grid where the
uniform draw is better. The scrambling constants tuned at n=4096 are right in
this regime too; nothing needs retuning for small samples.

**Pick `WithOwenScrambling` on Sobol or `WithNestedScrambling` on Halton.** At
s=30, n=40 they measure 5.13x and 5.20x — a 1.4% difference, which is a tie at
200 streams. The two schemes below them are not a tie: `WithDigitalShift`
(3.80x) and `WithScrambling` (3.40x) give up a third of the advantage. If you
have no other reason to prefer one, Owen-scrambled Sobol is the better default,
because it is also first at n=4096 and its lead grows as the budget does — a
population that is later resampled or extended keeps improving, while the choice
costs nothing at n=40.

**Do not use a discrepancy number to make this decision.** At 30 dimensions CD2
separates a QMC point set from an i.i.d. one by 2–3% while their integration
errors differ by a factor of five, and the ordering it produces among the four
randomizations is not the ordering that matters. Measure the thing you care
about.

**Do not expect the n=4096 headline figures.** The package advertises 19–29x
against Monte Carlo at 39 dimensions and n=4096. At n=40 the honest figure is
about 5x. Both are real; they are measurements of different regimes, and this
document exists so that nobody has to guess which one applies.

## Re-running

```
go test -run TestSmallSample -v .
```

Roughly a minute. All three tests are behind `testing.Short`, so `-short` skips
them.
