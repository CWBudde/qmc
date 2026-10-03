package qmc_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/cwbudde/qmc"
)

// Small-budget statistical fixtures use the stated dimensions/counts and
// two hundred fixed randomization seeds on a smooth product integrand. Their
// empirical level/trend policies do not imply convergence rates or optimizer
// initialization quality for arbitrary functions. Results are logged for
// comparison with docs/small-sample-regime.md.

// smallSampleDims and smallSampleCounts define the measurement grid.
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

// TestSmallSampleIntegration compares product-integrand RMS error at the stated
// small budgets and dimensions over two hundred fixed seeds. Level and trend
// assertions are fixture regression policies, not universal convergence rates.
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
			mcErr := mcRMSError(t, dims, n, smallSampleStreams)

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

				if !finiteMeasurement(ratio) || ratio < wantSpeedup {
					t.Fatalf("d=%d n=%d: %s is only %.2fx better than Monte Carlo (%.4e vs %.4e), want >= %.1fx; "+
						"the fixed small-sample workload lost its configured accuracy margin",
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
// comparison is not against a stale quoted number. The gate permits Owen
// to be within 20% of the measured leader at n=4096 rather than requiring it
// to win a near tie. This conservative margin exceeds the roughly 5% normal
// RMS sampling error at 200 streams; it is specific to this regression.
// At n=40 rankings are reported without enforcing an order.
//
// Ordering can change with seeds, budget, and integrand; this fixture's near-tie
// policy does not establish the best randomization for another application.
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

	want := sobolSchemes[0].name

	candidate := rmsOf(large, want)
	if !finiteMeasurement(candidate) || !finiteMeasurement(large[0].rms) || candidate > 1.2*large[0].rms {
		t.Fatalf("at d=%d n=4096 over %d streams %s has RMS %.4e, beyond the 20%% near-tie margin of the leader %s (%.4e)", dims, smallSampleStreams, want, candidate, large[0].name, large[0].rms)
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

// TestSmallSampleDiscrepancy compares sample mean discrepancies for the stated
// dimensions, budgets, and seeds. It also reports sampled and analytic RMS CD2
// for i.i.d. points; neither is an exact expression for mean CD2.
// The star-discrepancy gate checks low-dimensional separation on this workload;
// no CD2 ordering is asserted for the measured high-dimensional cases.
func TestSmallSampleDiscrepancy(t *testing.T) {
	if testing.Short() {
		t.Skip("exact star discrepancy over 200 point sets; -short skips it")
	}

	for _, dims := range append([]int{3}, smallSampleDims...) {
		for _, n := range smallSampleCounts {
			rng := rand.New(rand.NewSource(20240824)) //nolint:gosec // statistical baseline, not cryptography

			randCD2, randCD2Square, randStar := 0.0, 0.0, 0.0

			for seed := 1; seed <= smallSampleStreams; seed++ {
				pts := randomPoints(rng, n, dims)

				cd2, err := qmc.CenteredL2Discrepancy(pts)
				if err != nil {
					t.Fatal(err)
				}

				randCD2 += cd2
				randCD2Square += cd2 * cd2

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

			analyticRMS := math.Sqrt((math.Pow(1.25, float64(dims)) - math.Pow(13.0/12.0, float64(dims))) / float64(n))
			sampledRMS := math.Sqrt(randCD2Square / smallSampleStreams)

			if dims <= 3 {
				t.Logf("d=%d n=%d streams=%d: random mean CD2 %.5f, sampled RMS CD2 %.5f, analytic i.i.d. RMS CD2 %.5f, mean star %.5f",
					dims, n, smallSampleStreams, randCD2, sampledRMS, analyticRMS, randStar)
			} else {
				t.Logf("d=%d n=%d streams=%d: random mean CD2 %.5f, sampled RMS CD2 %.5f, analytic i.i.d. RMS CD2 %.5f, star not computed above %d dimensions",
					dims, n, smallSampleStreams, randCD2, sampledRMS, analyticRMS, 3)
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
