package qmc_test

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/cwbudde/qmc"
)

type referenceIntegrand struct {
	name  string
	exact float64
	eval  func([]float64) float64
}

func integrationReferences() []referenceIntegrand {
	gauss1 := func(center float64) float64 {
		return math.Sqrt(math.Pi) / 20 * (math.Erf(10*(1-center)) + math.Erf(10*center))
	}
	weighted := func(reverse bool) func([]float64) float64 {
		return func(x []float64) float64 {
			v := 1.0

			for i, xi := range x {
				weightIndex := i
				if reverse {
					weightIndex = len(x) - 1 - i
				}

				v *= 1 + (xi-0.5)/float64(weightIndex+1)
			}

			return v
		}
	}

	return []referenceIntegrand{
		{"nonlinear moments", 1.0 / 3, func(x []float64) float64 { return (x[0]*x[0] + x[23]*x[23]) / 2 }},
		{"early interaction", 0.25, func(x []float64) float64 { return x[0] * x[1] }},
		{"late interaction", 0.25, func(x []float64) float64 { return x[12] * x[23] }},
		{"decaying weights", 1, weighted(false)},
		{"reversed weights", 1, weighted(true)},
		{"localized peak", gauss1(0.37) * gauss1(0.63), func(x []float64) float64 {
			u, v := x[0]-0.37, x[23]-0.63
			return math.Exp(-100 * (u*u + v*v))
		}},
		{"discontinuous triangle", 0.7 * 0.7 / 2, func(x []float64) float64 {
			if x[0]+x[23] < 0.7 {
				return 1
			}

			return 0
		}},
	}
}

// This is a regression over specified functions and budgets, not a theorem
// that QMC beats Monte Carlo on every integrand. Samples are reused across
// functions so the nonlinear and reordered-coordinate checks stay affordable.
func TestIntegrationAcrossReferenceFunctionsAndBudgets(t *testing.T) {
	if testing.Short() {
		t.Skip("forty-stream integration sweep; run the statistical validation job")
	}

	const dims, streams = 24, 40

	functions := integrationReferences()

	for _, n := range []int{64, 256, 1024} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			baseline := referenceIntegrationErrors(t, nil, n, streams, functions)
			for _, scheme := range []struct {
				name    string
				control bool
				new     func(uint64) (qmc.Sequence, error)
			}{
				{"halton/digit", false, func(s uint64) (qmc.Sequence, error) {
					return qmc.NewHalton(dims, qmc.WithSkip(64), qmc.WithScrambling(s))
				}},
				{"halton/nested", false, func(s uint64) (qmc.Sequence, error) {
					return qmc.NewHalton(dims, qmc.WithSkip(64), qmc.WithNestedScrambling(s))
				}},
				{"sobol/shift/aligned", false, func(s uint64) (qmc.Sequence, error) {
					return qmc.NewSobol(dims, qmc.WithSkip(n-1), qmc.WithDigitalShift(s))
				}},
				{"sobol/owen/aligned", false, func(s uint64) (qmc.Sequence, error) {
					return qmc.NewSobol(dims, qmc.WithSkip(n-1), qmc.WithOwenScrambling(s))
				}},
				{"halton/plain/negative-control", true, func(uint64) (qmc.Sequence, error) { return qmc.NewHalton(dims) }},
			} {
				errors := referenceIntegrationErrors(t, scheme.new, n, streams, functions)
				for i, fn := range functions {
					rms, uncertainty := rmsErrorAndStandardError(errors[i])

					mc, mcUncertainty := rmsErrorAndStandardError(baseline[i])
					if !finiteMeasurement(rms) || !finiteMeasurement(uncertainty) || !finiteMeasurement(mc) || !finiteMeasurement(mcUncertainty) || mc <= 0 {
						t.Fatalf("nonfinite or degenerate error summary: %s / %s", scheme.name, fn.name)
					}
					// A broad deterioration guard, including the discontinuity.
					// No ranking among randomizations or universal advantage is required.
					if !scheme.control && rms > 3*mc {
						t.Errorf("%s / %s: RMS %g exceeds 3x seeded MC %g", scheme.name, fn.name, rms, mc)
					}

					if scheme.control && n == 64 && i == 0 && rms <= 2*mc {
						t.Errorf("negative control lost its high-base moment bias: RMS %g vs MC %g", rms, mc)
					}

					if !scheme.control && n == 1024 && i < 2 && rms >= mc/2 {
						t.Errorf("%s / %s: smooth low-order moments lost their measured 2x margin against MC", scheme.name, fn.name)
					}

					ratio := mc / rms
					if !finiteMeasurement(ratio) {
						t.Fatalf("nonfinite MC/QMC ratio: %s / %s", scheme.name, fn.name)
					}

					t.Logf("N=%d streams=%d %s / %s: absolute RMS %.4g (estimated SE %.2g), MC %.4g (SE %.2g), MC/QMC %.2f", n, streams, scheme.name, fn.name, rms, uncertainty, mc, mcUncertainty, ratio)
				}
			}
		})
	}
}

func finiteMeasurement(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

func referenceIntegrationErrors(t *testing.T, newSequence func(uint64) (qmc.Sequence, error), n, streams int, functions []referenceIntegrand) [][]float64 {
	t.Helper()

	errors := make([][]float64, len(functions))
	for i := range errors {
		errors[i] = make([]float64, streams)
	}

	point := make([]float64, 24)
	rng := rand.New(rand.NewSource(20261003)) //nolint:gosec // fixed statistical baseline

	for stream := range streams {
		var g qmc.Sequence

		if newSequence != nil {
			var err error

			g, err = newSequence(uint64(stream + 1))
			if err != nil {
				t.Fatal(err)
			}
		}

		sums := make([]float64, len(functions))

		for j := range n {
			if g != nil {
				g.AtInto(j, point)
			} else {
				for d := range point {
					point[d] = rng.Float64()
				}
			}

			for i, fn := range functions {
				sums[i] += fn.eval(point)
			}
		}

		for i, fn := range functions {
			errors[i][stream] = sums[i]/float64(n) - fn.exact
		}
	}

	return errors
}

// The delta-method SE describes the finite replicate summary under an
// independent-replicate model. It does not bound the seeded family's bias.
func rmsErrorAndStandardError(errors []float64) (float64, float64) {
	meanSquare := 0.0
	for _, e := range errors {
		meanSquare += e * e
	}

	meanSquare /= float64(len(errors))

	rms := math.Sqrt(meanSquare)
	if rms == 0 {
		return 0, 0
	}

	variance := 0.0

	for _, e := range errors {
		delta := e*e - meanSquare
		variance += delta * delta
	}

	variance /= float64(len(errors) - 1)

	return rms, math.Sqrt(variance/float64(len(errors))) / (2 * rms)
}
