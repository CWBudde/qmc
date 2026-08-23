package qmc

import (
	"math"
	"testing"
)

// The measurement this package exists for.
//
// A 39-dimensional Halton sequence sampled at 600 points is exactly what a
// parameter search over 39 knobs on a 600-evaluation budget asks for. Without
// scrambling the last coordinates have not yet left their first period —
// dimension 38 has base 167, so its first 167 points are 0, 1/167, 2/167, ...
// in order — and adjacent high dimensions therefore ramp in lockstep. The
// measured worst adjacent-pair correlation is 0.76 with no burn-in and still
// 0.65 after skipping 64 points.
//
// Scrambling is the fix, and this test is what keeps it fixed.
const (
	corrDims   = 39
	corrPoints = 600
	corrSkip   = 64
)

func TestScramblingBreaksHighDimensionalCorrelation(t *testing.T) {
	const tolerance = 0.25

	worstOverall := 0.0
	for _, seed := range []uint64{1, 2, 3, 4, 5} {
		g, err := NewHalton(corrDims, WithSkip(corrSkip), WithScrambling(seed))
		if err != nil {
			t.Fatal(err)
		}
		pts := draw(g, corrPoints)
		worst, pair := worstAdjacentCorrelation(pts)
		if worst > tolerance {
			t.Fatalf("seed %d: adjacent dims %d/%d correlate at %.4f, want <= %.2f",
				seed, pair, pair+1, worst, tolerance)
		}
		worstOverall = math.Max(worstOverall, worst)
	}
	t.Logf("scrambled: worst adjacent-pair |corr| over 5 seeds = %.4f", worstOverall)
}

// TestUnscrambledStillShowsTheDefect pins the behaviour the scrambled path is
// measured against. If a future change to the unscrambled generator made this
// pass, the comparison above would be meaningless and the test would be
// silently testing nothing.
func TestUnscrambledStillShowsTheDefect(t *testing.T) {
	g, err := NewHalton(corrDims, WithSkip(corrSkip))
	if err != nil {
		t.Fatal(err)
	}
	worst, pair := worstAdjacentCorrelation(draw(g, corrPoints))
	if worst < 0.5 {
		t.Fatalf("unscrambled worst adjacent-pair |corr| = %.4f at dims %d/%d; "+
			"expected the known defect (~0.65), so the scrambled comparison no longer means anything",
			worst, pair, pair+1)
	}
	t.Logf("unscrambled: worst adjacent-pair |corr| = %.4f at dims %d/%d", worst, pair, pair+1)
}

func draw(g *Halton, n int) [][]float64 {
	out := make([][]float64, n)
	for i := range out {
		out[i] = g.At(i)
	}
	return out
}

// worstAdjacentCorrelation returns the largest |Pearson r| over every pair of
// neighbouring dimensions, and the lower index of that pair.
func worstAdjacentCorrelation(pts [][]float64) (float64, int) {
	if len(pts) == 0 {
		return 0, 0
	}
	dims := len(pts[0])
	worst, at := 0.0, 0
	for d := 0; d+1 < dims; d++ {
		r := math.Abs(pearson(column(pts, d), column(pts, d+1)))
		if r > worst {
			worst, at = r, d
		}
	}
	return worst, at
}

func column(pts [][]float64, d int) []float64 {
	out := make([]float64, len(pts))
	for i, p := range pts {
		out[i] = p[d]
	}
	return out
}

func pearson(a, b []float64) float64 {
	n := float64(len(a))
	var ma, mb float64
	for i := range a {
		ma += a[i]
		mb += b[i]
	}
	ma /= n
	mb /= n

	var cov, va, vb float64
	for i := range a {
		da, db := a[i]-ma, b[i]-mb
		cov += da * db
		va += da * da
		vb += db * db
	}
	if va == 0 || vb == 0 {
		return 0
	}
	return cov / math.Sqrt(va*vb)
}
