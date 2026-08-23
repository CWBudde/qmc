//go:build js && wasm

package main

import (
	"fmt"
	"math"
	"math/rand"
	"syscall/js"

	"github.com/cwbudde/qmc"
)

const (
	maxConvergeN    = 200000
	maxConvergeDims = 32

	defaultConvergeN = 1000
)

// gaussianSigma is the width of the gaussian integrand's per-dimension bump.
//
// It is a compromise. Much narrower and a few hundred points miss the peak
// entirely in every dimension at once, so both estimators return near zero and
// the chart shows two flat lines that say nothing about sampling quality. Much
// wider and the integrand is effectively constant, which is what "sum" is
// already for. At 0.35 the bump covers most of the unit interval but still has
// real curvature, so QMC's better coverage shows up as a visibly steeper error
// curve.
const gaussianSigma = 0.35

// An integrand is a test function on the unit cube whose integral is known in
// closed form. Comparing an estimate against an exact value is the only honest
// way to draw a convergence chart: an error measured against a reference
// computed by the same sampler would flatter whichever sampler produced it.
type integrand struct {
	key         string
	label       string
	description string

	// fn evaluates the function at one point of [0,1)^d.
	fn func(point []float64) float64

	// exact returns the true integral over [0,1)^d. Getting this constant
	// wrong does not produce a visible failure — it produces a chart that is
	// simply a lie, with both curves converging to a nonzero floor — so each
	// one is derived in the comment beside it.
	exact func(dims int) float64
}

// integrandOrder fixes the order the page lists them in; a map alone would
// reshuffle the dropdown on every load.
var integrandOrder = []string{"product", "gaussian", "sum"}

var integrands = map[string]integrand{
	// The standard QMC test integrand from the Genz/Sobol' literature, in the
	// a_i = i form:
	//
	//	f(x) = prod_{i=1..d} (|4*x_i - 2| + i) / (1 + i)
	//
	// Each factor integrates to exactly 1 over [0,1): the tent |4x-2| is
	// symmetric about x = 1/2, rises linearly from 0 to 2 on each half, and so
	// has mean 1. That makes the i-th factor's integral (1 + i)/(1 + i) = 1,
	// and the whole product's integral 1 for every d.
	//
	// The a_i = i weighting is what makes it a useful test rather than a toy:
	// the low dimensions dominate the variance and the high ones contribute
	// almost nothing, which is precisely the situation in which a sequence
	// whose high dimensions have degenerated into ramps can still look fine.
	"product": {
		key:         "product",
		label:       "Product (Genz/Sobol')",
		description: "prod (|4x-2| + i)/(1 + i); exact 1 in every dimension. The standard QMC benchmark, weighted so low dimensions dominate.",
		fn: func(point []float64) float64 {
			product := 1.0
			for i, x := range point {
				a := float64(i + 1)
				product *= (math.Abs(4*x-2) + a) / (1 + a)
			}

			return product
		},
		exact: func(int) float64 { return 1 },
	},

	// A separable gaussian bump centred on the middle of the cube:
	//
	//	f(x) = prod_{i} exp(-(x_i - 1/2)^2 / (2*sigma^2))
	//
	// The one-dimensional integral is a truncated gaussian:
	//
	//	int_0^1 exp(-(x-1/2)^2/(2 s^2)) dx
	//	  = int_{-1/2}^{1/2} exp(-t^2/(2 s^2)) dt
	//	  = s*sqrt(2*pi) * erf( 1/(2*s*sqrt(2)) )
	//
	// using int_{-a}^{a} exp(-t^2/(2 s^2)) dt = s*sqrt(2*pi)*erf(a/(s*sqrt2)).
	// The integrand is a product of independent factors, so the d-dimensional
	// integral is that value raised to the d-th power.
	//
	// Note the magnitude: the per-dimension integral is below 1, so the exact
	// value shrinks geometrically with d (about 7e-5 at 32 dimensions with
	// sigma = 0.35). The absolute errors reported here shrink with it. Both
	// estimators are measured on the same scale, so the comparison stays
	// valid, but the page should plot this on a log axis.
	"gaussian": {
		key:         "gaussian",
		label:       "Gaussian bump",
		description: "prod exp(-(x-0.5)^2/(2s^2)) with s = 0.35; exact value is the truncated-gaussian 1-D integral raised to the d-th power.",
		fn: func(point []float64) float64 {
			product := 1.0
			for _, x := range point {
				d := x - 0.5
				product *= math.Exp(-(d * d) / (2 * gaussianSigma * gaussianSigma))
			}

			return product
		},
		exact: func(dims int) float64 {
			one := gaussianSigma * math.Sqrt(2*math.Pi) * math.Erf(1/(2*gaussianSigma*math.Sqrt2))

			return math.Pow(one, float64(dims))
		},
	},

	// The mean of the coordinates. Every x_i has mean 1/2, so the average of d
	// of them has mean 1/2 too, in any dimension.
	//
	// This is the control. It is linear, perfectly smooth and of the lowest
	// possible effective dimension, which is the regime where a plain Monte
	// Carlo estimator is at its least embarrassing — the two curves here run
	// much closer together than on the product integrand, and that contrast is
	// the point of including it.
	"sum": {
		key:         "sum",
		label:       "Mean of coordinates",
		description: "(1/d) * sum x_i; exact 0.5. A trivially smooth control where QMC's advantage is smallest.",
		fn: func(point []float64) float64 {
			if len(point) == 0 {
				return 0
			}

			total := 0.0
			for _, x := range point {
				total += x
			}

			return total / float64(len(point))
		},
		exact: func(int) float64 { return 0.5 },
	},
}

// jsConverge computes ONE point of the convergence chart: the absolute
// integration error at sample size n, for the selected sequence and for a
// pseudo-random sampler, over the same integrand and the same n.
//
// One n per call is deliberate, and it is the whole reason this export is
// shaped the way it is. A call into wasm is synchronous: it occupies the
// browser's single JavaScript thread for its entire duration, and nothing else
// can be dispatched while it runs — not a click, not a timer, not the page's
// own "stop" flag. A sweep computed entirely inside Go would therefore be
// uninterruptible, and at the top of the range (200,000 points in 32
// dimensions, twice) that is long enough to look like a hung tab.
//
// So the sweep loop lives in JavaScript, which awaits a turn of the event loop
// between calls. That gap is not an implementation detail: it IS the
// cancellation mechanism, and the only one available. Batching the n-sweep back
// into Go would remove it.
func jsConverge(opts js.Value) any {
	key := readString(opts, "integrand", "product")

	spec, ok := integrands[key]
	if !ok {
		return errorResult("converge: unknown integrand %q", key)
	}

	var (
		source        = readString(opts, "source", defaultSource)
		randomization = readString(opts, "randomization", randomizationNone)
		dims          = clampInt(readInt(opts, "dims", defaultDims), 1, maxConvergeDims)
		skip          = clampInt(readInt(opts, "skip", defaultSkip), 0, maxSkip)
		seed          = readUint64(opts, "seed", defaultSeed)
		n             = clampInt(readInt(opts, "n", defaultConvergeN), 1, maxConvergeN)
	)

	generator, err := newGenerator(source, dims, skip, randomization, seed)
	if err != nil {
		return errorResult("converge: %v", err)
	}

	exact := spec.exact(dims)

	// One buffer, reused for all 2n evaluations. At n = 200,000 and 32
	// dimensions the allocating form would churn 51 MB of float64 slices
	// through a 32-bit heap for no reason.
	point := make([]float64, dims)

	qmcTotal := 0.0

	for i := range n {
		generator.AtInto(i, point)
		qmcTotal += spec.fn(point)
	}

	// The comparison sampler is seeded from the same seed the sequence uses,
	// so the chart is reproducible end to end: same controls, same two curves.
	rng := rand.New(rand.NewSource(int64(seed))) //nolint:gosec // not cryptography; reproducibility is the requirement

	mcTotal := 0.0

	for range n {
		for d := range point {
			point[d] = rng.Float64()
		}

		mcTotal += spec.fn(point)
	}

	qmcValue := qmcTotal / float64(n)
	mcValue := mcTotal / float64(n)

	return map[string]any{
		"n":         n,
		"dims":      dims,
		"integrand": spec.key,
		"exact":     jsNumber(exact),
		"qmcValue":  jsNumber(qmcValue),
		"qmcError":  jsNumber(math.Abs(qmcValue - exact)),
		"mcValue":   jsNumber(mcValue),
		"mcError":   jsNumber(math.Abs(mcValue - exact)),

		"source":        source,
		"randomization": randomization,
		"seed":          float64(seed),
	}
}

// newGenerator is the one place a qmc.Sequence is built, so the mapping from
// the page's controls to the library's options exists exactly once.
//
// It returns the interface, not a concrete generator, and that is what keeps
// points, correlate and converge free of any per-source branch: all three ask
// for At/AtInto and nothing else. The digit inspector is the exception,
// because it needs Bases and Permutation, which are Halton's alone and are
// deliberately not on the interface — see sequence.go. It recovers them with a
// type assertion rather than by widening the interface here.
//
// A randomization the chosen generator does not accept is passed through to
// the constructor and refused there, by name. Deciding it here instead would
// be a second copy of a policy the library already states, and the copy is the
// one that would go stale.
func newGenerator(source string, dims, skip int, randomization string, seed uint64) (qmc.Sequence, error) {
	spec, ok := sources[source]
	if !ok || spec.construct == nil {
		return nil, fmt.Errorf("unknown sequence %q", source)
	}

	entry, ok := randomizations[randomization]
	if !ok {
		return nil, fmt.Errorf("unknown randomization %q", randomization)
	}

	options := []qmc.Option{qmc.WithSkip(skip)}
	if entry.option != nil {
		options = append(options, entry.option(seed))
	}

	return spec.construct(dims, options...)
}
