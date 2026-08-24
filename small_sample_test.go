package qmc_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/cwbudde/qmc"
)

// The regime nobody measured: forty points.
//
// Everything else in this package is measured where QMC is supposed to win.
// integration_test.go integrates at n=4096, discrepancy_test.go at n=1024, and
// both report the 1/n-versus-1/sqrt(n) gap in the tens. The caller that
// actually shipped against this library does none of that:
// mayfly.WithQMCInitialPopulation seeds a population of 40 individuals in up
// to 30 dimensions and never asks for point 41. Forty points in thirty
// dimensions is not a low-discrepancy point set in any useful sense — it is
// the first two levels of a stratification that would need 2^30 points to
// complete — and whether the scrambling constants tuned at n=4096 are the
// right ones there was an open question.
//
// This file answers it with numbers rather than argument, at s in {2, 10, 30}
// and n in {40, 160} so that a trend in n is visible and not just a single
// point, and it compares the ranking of the four randomizations at n=40
// against the ranking the rest of the repo quotes at n=4096.
//
// Two decisions about method are worth stating because the answers turn on
// them:
//
// Two hundred streams, not ten. Ten seeds is enough to separate a factor of
// twenty (which is what the n=4096 tests do) and nowhere near enough to
// separate two good schemes from each other: the RMS of ten squares has a
// relative standard error near 1/sqrt(2*10) = 22%, so two schemes 15% apart
// are indistinguishable. At n=40 the per-stream spread is much wider again.
// Two hundred seeds brings that standard error to about 5%, and 200 x 40
// points costs nothing.
//
// Honest gates. The measurements below do not all favour QMC, and the
// assertions say so. Each gate is an ordering or a margin far away from the
// measured value, in whichever direction the measurement actually points. A
// gate that had to be tuned to pass would be reporting the author's hopes, not
// the package's behaviour.
//
// Every figure quoted in docs/small-sample-regime.md comes out of
// `go test -run TestSmallSample -v .` on this file.

// smallSampleDims and smallSampleCounts are the grid. 30 dimensions and 40
// points are mayfly's shape exactly; 2 dimensions is where a 40-point set is
// still genuinely stratified (40 points cover a 6x6 grid); 10 sits between,
// and n=160 is the second rung that shows which way each number moves as the
// budget grows.
var (
	smallSampleDims   = []int{2, 10, 30}
	smallSampleCounts = []int{40, 160}
)

// smallSampleStreams is the seed count for every measurement in this file. See
// the file comment for why it is 200 and not 10.
const smallSampleStreams = 200

// haltonScheme and sobolScheme name a randomization together with the option
// that builds it, so the table below can be walked in one loop and the log
// lines name the scheme a caller would actually write.
type haltonScheme struct {
	name      string
	randomize func(uint64) qmc.Option
}

type sobolScheme struct {
	name      string
	randomize func(uint64) qmc.Option
}

var (
	haltonSchemes = []haltonScheme{
		{"Halton random-digit (WithScrambling)", qmc.WithScrambling},
		{"Halton nested (WithNestedScrambling)", qmc.WithNestedScrambling},
	}
	sobolSchemes = []sobolScheme{
		{"Sobol Owen (WithOwenScrambling)", qmc.WithOwenScrambling},
		{"Sobol digital shift (WithDigitalShift)", qmc.WithDigitalShift},
	}
)

// allSchemes flattens the two tables into one list carrying the constructor
// each scheme belongs to, for the loops that do not care which generator a
// randomization came from.
func allSchemes() []scheme {
	all := make([]scheme, 0, len(haltonSchemes)+len(sobolSchemes))
	for _, s := range haltonSchemes {
		all = append(all, scheme{s.name, s.randomize, false})
	}

	for _, s := range sobolSchemes {
		all = append(all, scheme{s.name, s.randomize, true})
	}

	return all
}

// scheme is a randomization together with the generator it applies to.
type scheme struct {
	name      string
	randomize func(uint64) qmc.Option
	sobol     bool
}

// TestSmallSampleIntegration measures RMS integration error of all four
// randomizations against the math/rand baseline on the smooth product
// integrand, across the (s, n) grid, over 200 streams each.
//
// What it would catch: a change to any randomization that quietly destroyed
// its small-sample behaviour while leaving the n=4096 gates green. The two
// regimes are not the same measurement. At n=4096 the asymptotic rate carries
// the result and the scrambling constants barely matter; at n=40 there is no
// asymptotic regime to sit in, only the first two or three strata, and the
// answer is decided entirely by how the randomization places those. A
// generator could keep its 20x at n=4096 and lose everything at n=40, and
// before this test nothing in the package would have noticed.
//
// The measured answer, over 200 streams, is that the advantage survives all
// the way down: the worst cell of the grid is Halton random-digit scrambling
// at s=30, n=40, and even there QMC is 3.40x more accurate than Monte Carlo.
// The best is Owen-scrambled Sobol at s=2, n=160 at 61x. Nothing on the grid
// is worse than independent sampling, so the honest gate is a speedup gate
// rather than a no-pessimisation one — but it is placed at 1.5x, less than
// half the worst measured value, so that a change of constants that costs
// some accuracy still passes and only a change that has given up the
// low-discrepancy property fails.
//
// The second gate is a trend rather than a level. At s=2, where 40 points are
// genuinely stratified, the advantage over Monte Carlo must not shrink when
// the budget goes from 40 to 160 points. That is the 1/n-versus-1/sqrt(n) rate
// showing up directly: measured, the four schemes go from 3.96x, 9.05x,
// 11.97x and 5.25x at n=40 to 10.83x, 24.10x, 61.47x and 8.89x at n=160.
// A generator whose error had stopped falling faster than sqrt would fail this
// while still passing every level gate in the package.
func TestSmallSampleIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("200 streams over six (dims, n) cells and five samplers; -short skips it")
	}

	const wantSpeedup = 1.5

	// ratioAt2Dims holds each scheme's advantage over Monte Carlo at s=2, keyed
	// by scheme name, so the n=160 cell can be compared against the n=40 one.
	ratioAt2Dims := make(map[int]map[string]float64, len(smallSampleCounts))

	for _, dims := range smallSampleDims {
		for _, n := range smallSampleCounts {
			mcErr := mcRMSError(dims, n, smallSampleStreams)

			type result struct {
				name string
				rms  float64
			}

			results := make([]result, 0, len(haltonSchemes)+len(sobolSchemes))

			for _, s := range haltonSchemes {
				results = append(results, result{s.name, qmcRMSError(t, s.randomize, dims, n, smallSampleStreams)})
			}

			for _, s := range sobolSchemes {
				results = append(results, result{s.name, sobolRMSError(t, s.randomize, dims, n, smallSampleStreams)})
			}

			t.Logf("d=%d n=%d streams=%d: Monte Carlo RMS rel. error %.4e", dims, n, smallSampleStreams, mcErr)

			ratioAt2Dims[n] = make(map[string]float64, len(results))

			for _, r := range results {
				if r.rms <= 0 {
					t.Fatalf("d=%d n=%d %s: RMS error is %g; an exactly-zero error means the estimator collapsed, not that QMC is perfect",
						dims, n, r.name, r.rms)
				}

				ratio := mcErr / r.rms
				t.Logf("d=%d n=%d streams=%d: %-38s RMS rel. error %.4e (%.2fx Monte Carlo)",
					dims, n, smallSampleStreams, r.name, r.rms, ratio)

				ratioAt2Dims[n][r.name] = ratio

				if ratio < wantSpeedup {
					t.Fatalf("d=%d n=%d: %s is only %.2fx better than Monte Carlo (%.4e vs %.4e), want >= %.1fx; "+
						"forty points is a small sample but it is not so small that a low-discrepancy set stops "+
						"paying for itself, and the worst cell of this grid measured 3.40x",
						dims, n, r.name, ratio, r.rms, mcErr, wantSpeedup)
				}
			}
		}

		if dims != 2 {
			continue
		}

		for name, small := range ratioAt2Dims[smallSampleCounts[0]] {
			large := ratioAt2Dims[smallSampleCounts[len(smallSampleCounts)-1]][name]
			if large < small {
				t.Fatalf("d=2: %s is %.2fx better than Monte Carlo at n=%d but only %.2fx at n=%d; "+
					"the advantage must not shrink as the budget grows, or the error has stopped falling "+
					"faster than 1/sqrt(n) and the point set is no longer low-discrepancy",
					name, small, smallSampleCounts[0], large, smallSampleCounts[len(smallSampleCounts)-1])
			}
		}
	}
}

// row is one scheme's measured RMS error, for ranking.
type row struct {
	name string
	rms  float64
}

// TestSmallSampleRankingMatchesLargeSample asks whether the ordering of the
// four randomizations at n=40 is the ordering the rest of the repo quotes at
// n=4096.
//
// It matters because the documentation makes a recommendation — Owen-scrambled
// Sobol first — and that recommendation was derived at a budget three orders
// of magnitude away from where mayfly uses it. If the ranking inverts at n=40,
// the advice is wrong for the caller that actually reads it, and a reader
// deciding what to seed a 40-member population with is being pointed at the
// wrong option.
//
// Both budgets are measured here, in the same run, on the same seeds, so the
// comparison is not against a number quoted from another file that may have
// drifted. The test asserts only the part that a scheme change must not break:
// at n=4096 and 30 dimensions, Owen-scrambled Sobol is the best of the four —
// that is the claim the docs make and it should fail loudly if it stops being
// true. At n=40 the ranking is logged and compared but not asserted, because
// the schemes there are close enough together that pinning an order would be
// pinning noise; docs/small-sample-regime.md reports what was measured.
//
// Measured: the two rankings differ only in the top pair. At n=4096 it is
// Owen Sobol (1.2256e-04) then nested Halton (1.2776e-04); at n=40 it is
// nested Halton (1.0999e-02) then Owen Sobol (1.1152e-02). Both gaps are
// around 1.4%, well inside the ~5% standard error of an RMS over 200 streams,
// so the honest reading is that the top two are tied at both budgets and that
// the bottom two — digital-shift Sobol then random-digit Halton — hold their
// places exactly.
func TestSmallSampleRankingMatchesLargeSample(t *testing.T) {
	if testing.Short() {
		t.Skip("four randomizations x 200 streams at n=4096 in 30 dimensions; -short skips it")
	}

	const dims = 30

	rank := func(n int) []row {
		rows := make([]row, 0, len(haltonSchemes)+len(sobolSchemes))
		for _, s := range haltonSchemes {
			rows = append(rows, row{s.name, qmcRMSError(t, s.randomize, dims, n, smallSampleStreams)})
		}

		for _, s := range sobolSchemes {
			rows = append(rows, row{s.name, sobolRMSError(t, s.randomize, dims, n, smallSampleStreams)})
		}

		for i := 1; i < len(rows); i++ {
			for j := i; j > 0 && rows[j].rms < rows[j-1].rms; j-- {
				rows[j], rows[j-1] = rows[j-1], rows[j]
			}
		}

		return rows
	}

	small := rank(40)
	large := rank(4096)

	for i, r := range small {
		t.Logf("n=40   rank %d: %-38s RMS rel. error %.4e", i+1, r.name, r.rms)
	}

	for i, r := range large {
		t.Logf("n=4096 rank %d: %-38s RMS rel. error %.4e", i+1, r.name, r.rms)
	}

	agree := true

	for i := range small {
		if small[i].name != large[i].name {
			agree = false

			break
		}
	}

	t.Logf("d=%d streams=%d: n=40 ranking %s the n=4096 ranking", dims, smallSampleStreams, map[bool]string{true: "matches", false: "does NOT match"}[agree])

	if want := sobolSchemes[0].name; large[0].name != want {
		t.Fatalf("at d=%d n=4096 over %d streams the best randomization is %s (%.4e), not %s (%.4e); "+
			"the recommendation in README.md and docs/small-sample-regime.md rests on Owen coming first here",
			dims, smallSampleStreams, large[0].name, large[0].rms, want, rmsOf(large, want))
	}
}

// rmsOf looks a scheme's error up by name in a ranked table, for use in a
// failure message that has to name both the winner and the expected winner.
func rmsOf(rows []row, name string) float64 {
	for _, r := range rows {
		if r.name == name {
			return r.rms
		}
	}

	return math.NaN()
}

// TestSmallSampleDiscrepancy asks the other half of the question: at n=40, can
// either discrepancy statistic still tell a QMC point set from an i.i.d.
// uniform one?
//
// discrepancy_test.go already shows that centered L2 saturates at 39
// dimensions and n=1024 — QMC and random land within 10% of each other while
// their integration errors differ by more than 5x. That is a statement about
// dimension. This is the statement about sample size, and the two failure
// modes are different: CD2 saturates in s because (5/4)^s runs away from
// (13/12)^s, while at small n every point set looks like noise because there
// are not enough points for the statistic to resolve anything finer than the
// first stratum.
//
// The analytic i.i.d. expectation sqrt(((5/4)^s - (13/12)^s)/N) is reported
// alongside, as discrepancy_test.go does, so a reader can see immediately
// whether the random baseline is behaving and therefore whether the QMC
// column means anything. Star discrepancy is computed exactly at s=2 and s=3,
// where n=40 is far inside the leaf budget, because star is the statistic that
// does not saturate and is the only one here with a chance of separating the
// point sets.
//
// The gate is deliberately one-sided and loose: at s=2 the QMC point sets must
// have a star discrepancy below the random baseline's, which is the weakest
// statement that still means "these are not the same point sets". No gate is
// placed on CD2 at all, because what this test documents is that CD2 cannot
// tell them apart at this size — asserting a separation would be asserting the
// opposite of the finding.
func TestSmallSampleDiscrepancy(t *testing.T) {
	if testing.Short() {
		t.Skip("exact star discrepancy over 200 point sets; -short skips it")
	}

	for _, dims := range append([]int{3}, smallSampleDims...) {
		for _, n := range smallSampleCounts {
			rng := rand.New(rand.NewSource(20240824)) //nolint:gosec // statistical baseline, not cryptography

			randCD2, randStar := 0.0, 0.0

			for seed := 1; seed <= smallSampleStreams; seed++ {
				pts := randomPoints(rng, n, dims)

				cd2, err := qmc.CenteredL2Discrepancy(pts)
				if err != nil {
					t.Fatal(err)
				}

				randCD2 += cd2

				if dims <= 3 {
					star, err := qmc.StarDiscrepancy(pts)
					if err != nil {
						t.Fatal(err)
					}

					randStar += star
				}
			}

			randCD2 /= smallSampleStreams
			randStar /= smallSampleStreams

			analytic := math.Sqrt((math.Pow(1.25, float64(dims)) - math.Pow(13.0/12.0, float64(dims))) / float64(n))

			if dims <= 3 {
				t.Logf("d=%d n=%d streams=%d: random CD2 %.5f (analytic %.5f), star %.5f",
					dims, n, smallSampleStreams, randCD2, analytic, randStar)
			} else {
				t.Logf("d=%d n=%d streams=%d: random CD2 %.5f (analytic %.5f), star not computed above %d dimensions",
					dims, n, smallSampleStreams, randCD2, analytic, 3)
			}

			for _, s := range allSchemes() {
				cd2, star := discrepancyOf(t, s.sobol, s.randomize, dims, n)
				logDiscrepancy(t, s.name, dims, n, cd2, star, randCD2, randStar)

				if dims == 2 && star >= randStar {
					t.Fatalf("d=%d n=%d: %s star discrepancy %.5f is not below the random baseline %.5f; "+
						"in two dimensions at n=%d the point sets must still be distinguishable by the one "+
						"statistic that does not saturate, or the randomization has stopped being low-discrepancy",
						dims, n, s.name, star, randStar, n)
				}
			}
		}
	}
}

// discrepancyOf averages CD2 and (at three dimensions or fewer, where the
// exact walk is affordable) star discrepancy over the same 200 stream seeds
// the integration measurements use.
func discrepancyOf(t *testing.T, sobol bool, randomize func(uint64) qmc.Option, dims, n int) (cd2, star float64) {
	t.Helper()

	for seed := 1; seed <= smallSampleStreams; seed++ {
		var (
			g   qmc.Sequence
			err error
		)

		if sobol {
			g, err = qmc.NewSobol(dims, qmc.WithSkip(64), randomize(uint64(seed)))
		} else {
			g, err = qmc.NewHalton(dims, qmc.WithSkip(64), randomize(uint64(seed)))
		}

		if err != nil {
			t.Fatal(err)
		}

		pts := qmc.Draw(g, n)

		c, err := qmc.CenteredL2Discrepancy(pts)
		if err != nil {
			t.Fatal(err)
		}

		cd2 += c

		if dims <= 3 {
			s, err := qmc.StarDiscrepancy(pts)
			if err != nil {
				t.Fatal(err)
			}

			star += s
		}
	}

	return cd2 / smallSampleStreams, star / smallSampleStreams
}

// logDiscrepancy prints one scheme's row of the discrepancy table, as a
// fraction of the random baseline so the reader can see at a glance whether
// the statistic separated the point sets at all.
func logDiscrepancy(t *testing.T, name string, dims, n int, cd2, star, randCD2, randStar float64) {
	t.Helper()

	if dims <= 3 {
		t.Logf("d=%d n=%d streams=%d: %-38s CD2 %.5f (%.3fx random), star %.5f (%.3fx random)",
			dims, n, smallSampleStreams, name, cd2, cd2/randCD2, star, star/randStar)

		return
	}

	t.Logf("d=%d n=%d streams=%d: %-38s CD2 %.5f (%.3fx random)",
		dims, n, smallSampleStreams, name, cd2, cd2/randCD2)
}
