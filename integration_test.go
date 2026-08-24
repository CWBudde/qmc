package qmc_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/cwbudde/qmc"
)

// The test that makes the rest of the suite mean something.
//
// Every other assertion in this package is a proxy. The correlation test in
// correlation_test.go measures adjacent-dimension Pearson r, and plain
// math/rand scores *better* on that measurement (0.1244) than the scrambled
// generator does (0.1406) — pseudorandom noise passes it comfortably. The same
// is true of the known-value and round-trip tests: they pin the arithmetic,
// not the property the arithmetic is for. Swap the whole generator for
// rand.Float64() and every one of those tests stays green.
//
// This test is the one that would not. A low-discrepancy point set exists to
// integrate better than independent sampling at the same budget: its error
// falls off roughly as 1/n instead of 1/sqrt(n), so at a few thousand points
// in a few dozen dimensions it is an order of magnitude ahead. That gap is not
// something a random number generator can fake, and it is the only property
// here that a caller actually buys.
//
// If this test is deleted, the package loses its only evidence that it does
// what its name claims.

// The integrand is the smooth product
//
//	f(x) = prod_k (1 + c_k * (x_k - 0.5)),   c_k = 1/(k+1)
//
// whose integral over the unit cube is exactly 1: each factor integrates to 1
// over its own coordinate and the factors are independent. Smoothness is the
// point — QMC's advantage rests on bounded variation in the Hardy–Krause
// sense, and a discontinuous or spiky integrand would blunt it for reasons
// that have nothing to do with the generator. The decaying coefficients give
// the later dimensions less weight, which is what real high-dimensional
// integrands look like and what makes 39 dimensions tractable at all.
func productIntegrand(x []float64) float64 {
	v := 1.0
	for k, xk := range x {
		v *= 1 + (1/float64(k+1))*(xk-0.5)
	}

	return v
}

// qmcRMSError returns the root-mean-square relative error of the scrambled
// generator's estimate over streams independent scrambling seeds.
//
// Averaging over several seeds is not decoration. A single randomized-QMC run
// is one draw from a distribution; the RMS over seeds is the quantity the
// theory bounds, and it is what keeps this test from turning on whether one
// lucky seed happened to land well.
//
// The randomize parameter builds the option under test from a stream seed,
// exactly as sobolRMSError does in sobol_integration_test.go. Passing it in
// rather than hardcoding WithScrambling keeps every Halton randomization
// measured on the same integrand, budget and stream seeds, which is what makes
// the comparison between random-digit and nested scrambling in
// small_sample_test.go worth anything.
func qmcRMSError(t *testing.T, randomize func(uint64) qmc.Option, dims, n, streams int) float64 {
	t.Helper()

	sumSq := 0.0
	point := make([]float64, dims)

	for seed := 1; seed <= streams; seed++ {
		g, err := qmc.NewHalton(dims, qmc.WithSkip(64), randomize(uint64(seed)))
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

// mcRMSError is the same measurement driven by math/rand.
//
// The source is seeded from a fixed constant, never from time or the global
// source, so a failure here is reproducible and a passing run is not luck. The
// streams are consecutive draws from one source rather than separately seeded
// generators: separately seeded ones can correlate, which would flatter the
// baseline this test is trying to beat honestly.
func mcRMSError(dims, n, streams int) float64 {
	rng := rand.New(rand.NewSource(20240823)) //nolint:gosec // statistical baseline, not cryptography

	sumSq := 0.0
	point := make([]float64, dims)

	for s := 0; s < streams; s++ {
		sum := 0.0

		for i := 0; i < n; i++ {
			for d := range point {
				point[d] = rng.Float64()
			}

			sum += productIntegrand(point)
		}

		e := sum/float64(n) - 1.0
		sumSq += e * e
	}

	return math.Sqrt(sumSq / float64(streams))
}

// TestScrambledQMCBeatsMonteCarloAt39Dims is the package's design point: 39
// knobs, a budget of a few thousand evaluations. Measured here, scrambled QMC
// comes in around 19-28x more accurate than plain Monte Carlo depending on n.
//
// The asserted margin is deliberately far below that. A factor of 5 still
// cannot be reached by any generator producing independent samples — the gap
// between 1/n and 1/sqrt(n) convergence is structural — while leaving room for
// an unlucky seed, a different Go version's rand, and future changes to the
// scrambling scheme that shift the constant without giving up the rate. A test
// pinned at 19x would fail on noise; one at 5x fails only if the package has
// stopped being a QMC package.
func TestScrambledQMCBeatsMonteCarloAt39Dims(t *testing.T) {
	const (
		dims        = 39
		n           = 4096
		streams     = 10
		wantSpeedup = 5.0
	)

	qmcErr := qmcRMSError(t, qmc.WithScrambling, dims, n, streams)
	mcErr := mcRMSError(dims, n, streams)

	if qmcErr <= 0 {
		t.Fatalf("QMC RMS error is %g; an exactly-zero error means the integrand or the estimator collapsed, not that QMC is perfect", qmcErr)
	}

	ratio := mcErr / qmcErr
	if ratio < wantSpeedup {
		t.Fatalf("at %d dims with n=%d over %d streams: QMC RMS error %.3e vs MC %.3e = %.1fx, want >= %.0fx; "+
			"the generator is no longer integrating better than independent sampling",
			dims, n, streams, qmcErr, mcErr, ratio, wantSpeedup)
	}

	t.Logf("d=%d n=%d streams=%d: QMC RMS rel. error %.3e vs MC %.3e (%.1fx better)",
		dims, n, streams, qmcErr, mcErr, ratio)
}

// TestScrambledQMCBeatsMonteCarloAtLowDims runs the same comparison in eight
// dimensions at a small budget.
//
// It earns its place by failing differently. The 39-dimensional case leans on
// scrambling; without it the high dimensions have not left their first period
// and the sequence is not usable at all. At eight dimensions every base is 19
// or below, the plain sequence is already well distributed, and the advantage
// comes from the low-discrepancy construction itself. So if radicalInverse or
// the prime table breaks, this fails while the 39-dimensional test might still
// scrape past on scrambling alone — and if scrambling breaks, the reverse. The
// pair localises the damage.
func TestScrambledQMCBeatsMonteCarloAtLowDims(t *testing.T) {
	const (
		dims        = 8
		n           = 512
		streams     = 10
		wantSpeedup = 5.0
	)

	qmcErr := qmcRMSError(t, qmc.WithScrambling, dims, n, streams)
	mcErr := mcRMSError(dims, n, streams)

	ratio := mcErr / qmcErr
	if ratio < wantSpeedup {
		t.Fatalf("at %d dims with n=%d over %d streams: QMC RMS error %.3e vs MC %.3e = %.1fx, want >= %.0fx",
			dims, n, streams, qmcErr, mcErr, ratio, wantSpeedup)
	}

	t.Logf("d=%d n=%d streams=%d: QMC RMS rel. error %.3e vs MC %.3e (%.1fx better)",
		dims, n, streams, qmcErr, mcErr, ratio)
}
