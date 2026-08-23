//go:build js && wasm

package main

import (
	"math"
	"syscall/js"

	"github.com/cwbudde/qmc"
)

const (
	maxCorrelateDims   = 48
	maxCorrelatePoints = 5000
)

// jsCorrelate computes the full Pearson correlation matrix over the
// coordinates of one draw: the heatmap that shows the defect as a picture
// rather than as a single number.
//
// The scatter plot can only ever show one pair at a time, which makes the
// failure look like a quirk of dimensions 37 and 38. The matrix shows that it
// is a band: every adjacent high-dimensional pair lights up at once, because
// each of them is still inside its own first period and ramping in lockstep.
// Picking a randomization wipes the band out and leaves only the unit
// diagonal.
//
// The correlation helper is reimplemented here rather than imported. qmc's
// version lives in correlation_test.go and is unexported, so there is no way
// to reach it — and the duplication is worth naming: if the two ever disagree,
// this page is the one that is wrong, because the test is what the library's
// claim is actually measured by.
func jsCorrelate(opts js.Value) any {
	source := readString(opts, "source", defaultSource)

	spec, ok := sources[source]
	if !ok || spec.construct == nil {
		return errorResult("correlate: unknown sequence %q", source)
	}

	var (
		randomization = readString(opts, "randomization", randomizationNone)
		dims          = clampInt(readInt(opts, "dims", defaultDims), 2, min(maxCorrelateDims, spec.maxDims))
		count         = clampInt(readInt(opts, "count", defaultCount), 2, maxCorrelatePoints)
		skip          = clampInt(readInt(opts, "skip", defaultSkip), 0, maxSkip)
		leap          = clampInt(readInt(opts, "leap", defaultLeap), 1, maxLeap)
		seed          = readUint64(opts, "seed", defaultSeed)
	)

	generator, err := newGenerator(source, dims, skip, leap, randomization, seed)
	if err != nil {
		return errorResult("correlate: %v", err)
	}

	// The draw is stored column-major — all of coordinate 0, then all of
	// coordinate 1 — because every subsequent loop walks one coordinate at a
	// time. A row-major [count][dims] would stride across the cache on each of
	// the dims*(dims-1)/2 pairs; at 48 dimensions that is 1,128 passes.
	columns := make([]float64, dims*count)
	point := make([]float64, dims)

	for i := range count {
		generator.AtInto(i, point)

		for d := range dims {
			columns[d*count+i] = point[d]
		}
	}

	means := make([]float64, dims)
	deviations := make([]float64, dims) // sqrt(sum of squared deviations)

	for d := range dims {
		column := columns[d*count : (d+1)*count]

		mean := 0.0
		for _, v := range column {
			mean += v
		}

		mean /= float64(count)

		sum := 0.0

		for _, v := range column {
			delta := v - mean
			sum += delta * delta
		}

		means[d] = mean
		deviations[d] = math.Sqrt(sum)
	}

	matrix := make([]float32, dims*dims)

	worstAdjacent := 0.0
	worstPair := [2]int{0, 0}

	for a := range dims {
		matrix[a*dims+a] = 1

		for b := a + 1; b < dims; b++ {
			r := correlation(columns, count, means, deviations, a, b)

			// Row-major, and symmetric, so the page can index
			// matrix[row*dims + column] in either order.
			matrix[a*dims+b] = float32(r)
			matrix[b*dims+a] = float32(r)

			// Adjacent pairs only, matching what
			// TestScramblingBreaksHighDimensionalCorrelation measures — the
			// number the page prints has to be the same number the library's
			// own claim is made about, or the demo and the README would be
			// quoting different quantities.
			if b == a+1 {
				if absolute := math.Abs(r); absolute > worstAdjacent {
					worstAdjacent, worstPair = absolute, [2]int{a, b}
				}
			}
		}
	}

	out := opts.Get("out")

	// Declared as any so a source without prime bases sends null rather than
	// an empty array: js.ValueOf turns a nil []any into a zero-length JS
	// array, which is an object and therefore truthy, and the page would go on
	// rendering a "bases" clause it has nothing to fill in. Same trap as the
	// permutation field in digits.go.
	var bases any

	if halton, ok := generator.(*qmc.Halton); ok {
		bases = intsToJS(halton.Bases())
	}

	response := map[string]any{
		"dims":          dims,
		"count":         count,
		"bases":         bases,
		"worstAdjacent": jsNumber(worstAdjacent),
		"worstPair":     []any{worstPair[0], worstPair[1]},
		"source":        source,
		"randomization": randomization,
		"seed":          float64(seed),
		"skip":          skip,
		"leap":          leap,
	}

	putFloats(response, out, "matrix", matrix)

	return response
}

// correlation is the Pearson coefficient between two coordinates, computed
// from precomputed means and deviation norms.
//
// A coordinate with zero variance yields 0, not NaN. This is not hypothetical
// at the low end of the controls: with count small enough that a large base
// has not yet produced two distinct digits, an unscrambled coordinate can be
// literally constant. Dividing anyway would put NaN in the matrix, and a NaN
// propagates through the page's own min/max scan and blanks the entire
// heatmap — one degenerate cell destroying 2,000 good ones.
func correlation(columns []float64, count int, means, deviations []float64, a, b int) float64 {
	if deviations[a] == 0 || deviations[b] == 0 {
		return 0
	}

	ca := columns[a*count : (a+1)*count]
	cb := columns[b*count : (b+1)*count]

	covariance := 0.0
	for i := range ca {
		covariance += (ca[i] - means[a]) * (cb[i] - means[b])
	}

	return covariance / (deviations[a] * deviations[b])
}
