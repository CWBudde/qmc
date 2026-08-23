package qmc_test

import (
	"math"
	"testing"

	"github.com/cwbudde/qmc"
)

// The gates that decide whether Sobol earns its place, in the same terms
// integration_test.go set for Halton: RMS integration error against plain
// Monte Carlo on the same budget, over several randomization streams.
//
// This is package qmc_test, not package qmc, because it reuses
// productIntegrand from integration_test.go. sobol_test.go is white-box — most
// of its length is the unexported direction-number table — and a file cannot
// be in two packages, so the two test files are split by what each one needs
// to reach rather than by subject.
//
// Read the long comment at the top of integration_test.go before changing the
// 5x threshold here. The reasoning is the same and it is written down there:
// no generator producing independent samples can reach 5x, because the gap
// between 1/n and 1/sqrt(n) convergence is structural, while a bound pinned at
// the measured value would fail on an unlucky seed.

// randomize builds the option under test from a stream seed. Passing it in
// rather than hardcoding one keeps the two randomizations measured on exactly
// the same integrand, budget and stream seeds — the comparison between them is
// the whole point of TestOwenBeatsDigitalShiftAt39Dims, and it would be worth
// nothing if the two paths differed anywhere else.
func sobolRMSError(t *testing.T, randomize func(uint64) qmc.Option, dims, n, streams int) float64 {
	t.Helper()

	sumSq := 0.0
	point := make([]float64, dims)

	for seed := 1; seed <= streams; seed++ {
		g, err := qmc.NewSobol(dims, qmc.WithSkip(64), randomize(uint64(seed)))
		if err != nil {
			t.Fatal(err)
		}

		sum := 0.0

		for i := 0; i < n; i++ {
			g.AtInto(i, point)
			sum += productIntegrand(point)
		}

		e := sum/float64(n) - 1.0
		sumSq += e * e
	}

	return math.Sqrt(sumSq / float64(streams))
}

// TestShiftedSobolBeatsMonteCarloAt39Dims is Sobol's version of the gate in
// integration_test.go, and it is here for the same reason that one is there:
// every other test in sobol_test.go pins arithmetic, and arithmetic that is
// internally consistent but wrong would pass all of them. Reference values,
// At-versus-Next agreement and the balance property would all survive a
// generator that had quietly stopped integrating well; this is the test that
// would not.
//
// The threshold is 5x, the same figure and the same argument as the Halton
// gate: no generator producing independent samples can reach it, because the
// gap between 1/n and 1/sqrt(n) convergence is structural, while it leaves
// room for an unlucky seed and for a future change to the randomization that
// shifts the constant without giving up the rate. Measured here, shifted Sobol
// comes in at 29.5x against math/rand at these settings, so the margin is
// wide — deliberately, because a test pinned near the measured value would
// fail on noise and a test that fails on noise gets deleted.
func TestShiftedSobolBeatsMonteCarloAt39Dims(t *testing.T) {
	const (
		dims        = 39
		n           = 4096
		streams     = 10
		wantSpeedup = 5.0
	)

	sobolErr := sobolRMSError(t, qmc.WithDigitalShift, dims, n, streams)
	mcErr := mcRMSError(dims, n, streams)

	if sobolErr <= 0 {
		t.Fatalf("Sobol RMS error is %g; an exactly-zero error means the integrand or the estimator collapsed, not that QMC is perfect", sobolErr)
	}

	ratio := mcErr / sobolErr
	if ratio < wantSpeedup {
		t.Fatalf("at %d dims with n=%d over %d streams: Sobol RMS error %.3e vs MC %.3e = %.1fx, want >= %.0fx; "+
			"the generator is no longer integrating better than independent sampling",
			dims, n, streams, sobolErr, mcErr, ratio, wantSpeedup)
	}

	t.Logf("d=%d n=%d streams=%d: Sobol RMS rel. error %.3e vs MC %.3e (%.1fx better)",
		dims, n, streams, sobolErr, mcErr, ratio)
}

// TestSobolAgainstHaltonAt39Dims measures the two generators against each
// other and asserts almost nothing.
//
// The measurement is worth having: it is the number that answers "should I
// switch?", and the answer at this package's design point is 1.67x — Sobol's
// RMS error is a little under two thirds of scrambled Halton's at 39
// dimensions and 4096 points. That is a real improvement and it is smaller
// than the folklore suggests, which is exactly why it should be logged rather
// than remembered.
//
// The assertion is loose on purpose. Which of two low-discrepancy sequences
// wins on a given integrand at a given n is not a stable fact — it moves with
// the integrand's effective dimension, with n relative to powers of two, and
// with the randomization. A test pinned at 1.67x would be a test of this
// integrand rather than of either generator, and it would fail on a change
// that improved Halton. All that is asserted is that Sobol is not
// dramatically worse, which would mean something is broken; the number itself
// goes to the log, where a human can read it.
func TestSobolAgainstHaltonAt39Dims(t *testing.T) {
	const (
		dims    = 39
		n       = 4096
		streams = 10
	)

	sobolErr := sobolRMSError(t, qmc.WithDigitalShift, dims, n, streams)
	haltonErr := qmcRMSError(t, dims, n, streams)

	ratio := haltonErr / sobolErr

	t.Logf("d=%d n=%d streams=%d: Sobol RMS %.3e vs scrambled Halton %.3e (Sobol %.2fx better)",
		dims, n, streams, sobolErr, haltonErr, ratio)

	if ratio < 0.5 {
		t.Fatalf("Sobol RMS error %.3e is more than twice scrambled Halton's %.3e; "+
			"the two should be within a small factor of each other on a smooth integrand, "+
			"so a gap this size means the Sobol construction is damaged rather than merely different",
			sobolErr, haltonErr)
	}
}

// TestSobolBeatsMonteCarloAtLowDims mirrors the low-dimensional Halton case
// and earns its place the same way: it fails differently. At eight dimensions
// and 512 points the index never exceeds ten bits, so only the first handful
// of direction numbers per dimension are ever selected — the ones that come
// straight out of the file. At 39 dimensions and 4096 points the recurrence in
// expandDirections has produced most of what is being used. A break in the
// file parsing shows up here; a break in the recurrence shows up there.
func TestSobolBeatsMonteCarloAtLowDims(t *testing.T) {
	const (
		dims        = 8
		n           = 512
		streams     = 10
		wantSpeedup = 5.0
	)

	sobolErr := sobolRMSError(t, qmc.WithDigitalShift, dims, n, streams)
	mcErr := mcRMSError(dims, n, streams)

	ratio := mcErr / sobolErr
	if ratio < wantSpeedup {
		t.Fatalf("at %d dims with n=%d over %d streams: Sobol RMS error %.3e vs MC %.3e = %.1fx, want >= %.0fx",
			dims, n, streams, sobolErr, mcErr, ratio, wantSpeedup)
	}

	t.Logf("d=%d n=%d streams=%d: Sobol RMS rel. error %.3e vs MC %.3e (%.1fx better)",
		dims, n, streams, sobolErr, mcErr, ratio)
}

// TestOwenBeatsDigitalShiftAt39Dims is the measurement that decides whether
// Owen scrambling earns the code it costs.
//
// The two randomizations are run over the same integrand, the same budget and
// the same stream seeds, so the only difference between them is the one under
// test. Both leave the (t,m,s)-net structure intact — that is checked directly
// in TestFirstPointsAreOneDimensionallyBalanced — so this is asking the
// narrower question the theory actually distinguishes them on: a digital shift
// translates the point set rigidly and cannot improve a projection, while an
// Owen scramble redistributes within it.
//
// The assertion is one-sided and loose. Requiring Owen to be at least as good
// as a digital shift, with room for stream noise, is a claim that can fail if
// the scramble is broken or applied in the wrong place; requiring it to beat
// the shift by the measured factor would be pinning a constant that depends on
// the integrand, and would fail for reasons that say nothing about the code.
func TestOwenBeatsDigitalShiftAt39Dims(t *testing.T) {
	const (
		dims    = 39
		n       = 4096
		streams = 10

		// Owen may come out slightly behind on a given integrand without
		// anything being wrong — the two are close on smooth products, which
		// this integrand is. What would not be noise is Owen coming out
		// several times worse, which is what a scramble applied to the wrong
		// bits, or applied inconsistently between the two evaluation paths,
		// would produce.
		tolerance = 1.5
	)

	owenErr := sobolRMSError(t, qmc.WithOwenScrambling, dims, n, streams)
	shiftErr := sobolRMSError(t, qmc.WithDigitalShift, dims, n, streams)

	if owenErr <= 0 {
		t.Fatalf("Owen RMS error is %g; an exactly-zero error means the estimator collapsed, not that the scramble is perfect", owenErr)
	}

	if owenErr > shiftErr*tolerance {
		t.Fatalf(
			"at %d dims with n=%d over %d streams: Owen RMS error %.3e against a digital shift's %.3e; "+
				"Owen scrambling is meant to be the stronger randomization and is coming out %.2fx worse, "+
				"which is past what stream noise explains",
			dims, n, streams, owenErr, shiftErr, owenErr/shiftErr,
		)
	}

	t.Logf("d=%d n=%d streams=%d: Owen RMS rel. error %.3e vs digital shift %.3e (%.2fx)",
		dims, n, streams, owenErr, shiftErr, shiftErr/owenErr)
}
