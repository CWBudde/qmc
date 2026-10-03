//go:build js && wasm

package main

import (
	"fmt"
	"math"
	"math/rand"
	"syscall/js"

	"github.com/cwbudde/qmc"
)

// maxDiscrepancyPoints is the hard ceiling on N for either metric, before the
// per-metric, per-dimension ceiling below narrows it further.
//
// A console or stale script can bypass HTML ranges. This bounds the matrices
// allocated before measurement and the buffer used to probe library acceptance.
// The library has cheaper one-dimensional and one-point paths; the demo still
// applies its own smaller point budget.
const maxDiscrepancyPoints = 8192

// minDiscrepancyPoints keeps the demo comparison at two or more points. Both
// library metrics also accept a one-point input, which this panel does not offer.
const minDiscrepancyPoints = 2

// cd2CallBudgetNs retains the demo's historical work ceiling. Heavy calls now
// run in a cancellable worker. This policy bounds total work and is neither a
// library limitation nor a duration guarantee on every device.
const cd2CallBudgetNs = 150e6

// cd2NsPerTerm and cd2NsPerPair retain the earlier browser policy's affine
// coefficients. Their original timing samples lack complete reproduction
// metadata and predate numerical changes, so these are policy weights rather
// than a calibrated cost estimate for the current implementation or device.
// Keeping them preserves the existing point budgets.
const (
	cd2NsPerTerm = 5.7
	cd2NsPerPair = 7.5
)

// starDemoPoints caps N per dimension count on top of whatever
// qmc.StarDiscrepancy itself accepts.
//
// These policy ceilings bound total work separately from the library's generic
// work limit. Worker execution keeps the DOM responsive during each call.
// The pruner's cost depends on the point set as well as its shape.
//
// These retain the historical demo budgets, not current runtime predictions.
// starMaxPoints separately asks the library what it accepts; the smaller limit
// wins. Future library support absent from this table is discovered by probing.
var starDemoPoints = map[int]int{
	1: 4096,
	2: 1792,
	3: 224,
	4: 84,
	5: 46,
	6: 32,
}

// separatesRatio is a display-policy threshold for this one measured ratio.
// It is not a significance test or a guarantee about integration accuracy.
// Historical seed summaries without a complete reproduction record are not
// used as promises for the user's selected source, seed, or sampling budget.
const separatesRatio = 1.5

// A discrepancySpec is one entry of the metric menu, in the shape converge.go
// uses for integrands: what it is called, what it measures, and — the part
// that makes this table earn its keep — how many points it can afford at a
// given dimension count.
//
// maxPoints is a function of dims and not a constant because both metrics'
// costs depend on the dimension count, in opposite directions: centred L2 is
// O(N^2 s) so its ceiling falls slowly as dimensions are added, and star is
// NP-hard in the dimension so its ceiling collapses. A single published number
// would be wrong for every dimension count but one — the same trap info.go
// already fell into with integrands.exact.
type discrepancySpec struct {
	key         string
	label       string
	description string

	// noun is the label as it reads inside a sentence. "the sequence's Star
	// discrepancy (exact)" is what happens when a menu label is dropped into
	// prose, and the verdict below is prose.
	noun string

	measure   func(points [][]float64) (float64, error)
	maxPoints func(dims int) int

	// analytic is a random RMS reference sqrt(E[CD2^2]), not E[CD2]. Nil means
	// this demo provides no analytic reference for the metric. The expectation
	// assumes independent continuous uniform points, while the seeded comparison
	// is one finite-precision realization.
	analytic      func(dims, n int) float64
	analyticKind  string
	analyticLabel string
}

// discrepancyOrder fixes the order the page lists them in; a map alone would
// reshuffle the dropdown on every load.
var discrepancyOrder = []string{"cl2", "star"}

var discrepancies = map[string]discrepancySpec{
	"cl2": {
		key:         "cl2",
		label:       "Centred L2 (CD2)",
		noun:        "centred L2 discrepancy",
		description: "Hickernell's centred L2 discrepancy over coordinate projections. General evaluation costs O(N²s), with a cheaper one-dimensional path and explicit floating-point range limits. At high dimensions this metric can distinguish useful sequences from random points only weakly; inspect the current measurement rather than assuming an accuracy ranking.",
		measure:     qmc.CenteredL2Discrepancy,
		maxPoints:   cd2MaxPoints,

		// E[CD2^2] = ((5/4)^s - (13/12)^s)/N, exact, derived in
		// CenteredL2Discrepancy's doc comment. Taking its square root gives RMS,
		// which generally differs from the mean discrepancy E[CD2].
		analytic: func(dims, n int) float64 {
			s := float64(dims)

			return math.Sqrt((math.Pow(1.25, s) - math.Pow(13.0/12.0, s)) / float64(n))
		},
		analyticKind:  "rms",
		analyticLabel: "Analytic random RMS baseline",
	},
	"star": {
		key:         "star",
		label:       "Star discrepancy (exact)",
		noun:        "star discrepancy",
		description: "The supremum over origin-anchored boxes, evaluated by finite candidate enumeration. The Koksma–Hlawka bound multiplies it by the integrand's Hardy–Krause variation when that variation is finite. Generic multi-point computation has dimension and work limits; the library has cheaper one-point and one-dimensional paths. The demo applies its own work ceilings.",
		measure:     qmc.StarDiscrepancy,
		maxPoints:   starMaxPoints,

		// No analytic random baseline is provided for star; draw the two
		// measured sets without presenting an asymptotic rate as an exact value.
		analytic:      nil,
		analyticKind:  "none",
		analyticLabel: "No analytic random baseline",
	},
}

// cd2MaxPoints converts the retained policy weights into a dimension-dependent
// point budget. It neither estimates current execution time nor defines the
// library's mathematical or floating-point support range.
func cd2MaxPoints(dims int) int {
	if dims < 1 {
		dims = 1
	}

	perPair := cd2NsPerTerm*float64(dims) + cd2NsPerPair
	n := int(math.Sqrt(2 * cd2CallBudgetNs / perPair))

	return clampInt(n, minDiscrepancyPoints, maxDiscrepancyPoints)
}

// starMaxPoints reports the largest point count both library acceptance and
// the demo's retained work policy permit at this dimension count.
//
// The library's half is found by ASKING it — a binary search over point counts,
// each probe a real qmc.StarDiscrepancy call whose error is the answer — and
// not by re-deriving C(N+s,s) against the package's budget constant here. That
// is the leaps() precedent: the library is the only place that says what it
// accepts, and a second copy in the demo is the copy that goes stale after a
// release.
//
// The probe is affordable because the refusal depends only on the SHAPE of the
// input, never on the coordinates: StarDiscrepancy validates, then compares
// C(N+s,s) against its budget, and only then walks. So the probe may use
// whatever point set is cheapest to walk, and a matrix of all-ones is the
// cheapest there is — every dimension's candidate grid collapses to the single
// value 1, so the accepted probes visit one leaf instead of tens of millions
// and cost O(N s) to validate. A probe that returned a real discrepancy would
// be the thing this function is here to avoid.
func starMaxPoints(dims int) int {
	if dims < 1 {
		return 0
	}

	// Probe the minimum demo request before allocating the larger buffer.
	// One-point library shortcuts do not change this panel's two-point minimum.
	if _, err := qmc.StarDiscrepancy(unitMatrix(minDiscrepancyPoints, dims)); err != nil {
		return 0
	}

	// One buffer for every probe. The rows alias it the way qmc.Draw's do, and
	// each is capped at its own length so a probe cannot scribble into the
	// next row.
	flat := make([]float64, maxDiscrepancyPoints*dims)
	for i := range flat {
		flat[i] = 1
	}

	rows := make([][]float64, maxDiscrepancyPoints)
	for i := range rows {
		rows[i] = flat[i*dims : (i+1)*dims : (i+1)*dims]
	}

	accepts := func(n int) bool {
		_, err := qmc.StarDiscrepancy(rows[:n])

		return err == nil
	}

	// Invariant: lo accepts, hi does not.
	lo, hi := minDiscrepancyPoints, maxDiscrepancyPoints+1

	for hi-lo > 1 {
		mid := lo + (hi-lo)/2
		if accepts(mid) {
			lo = mid
		} else {
			hi = mid
		}
	}

	if demo, ok := starDemoPoints[dims]; ok && demo < lo {
		return demo
	}

	return lo
}

// jsDiscrepancy computes ONE n of the discrepancy sweep: the selected metric
// over the sequence, the same metric over a pseudorandom set of the same size,
// and — for centred L2 — the analytic random RMS baseline.
//
// Each call is synchronous in its calling realm. The UI runs it in a worker,
// which Stop can terminate during computation, and renders completed rungs.
// The point ceilings bound total work independently of worker cancellation.
//
// Everything returned is a scalar, so marshal.go's sink machinery is not
// involved at all.
func jsDiscrepancy(opts js.Value) any {
	metric := readString(opts, "metric", defaultMetric)

	spec, ok := discrepancies[metric]
	if !ok {
		return errorResult("discrepancy: unknown metric %q", metric)
	}

	source := readString(opts, "source", defaultSource)

	sourceEntry, ok := sources[source]
	if !ok || sourceEntry.construct == nil {
		return errorResult("discrepancy: unknown sequence %q", source)
	}

	var (
		randomization = readString(opts, "randomization", randomizationNone)
		dims          = clampInt(readInt(opts, "dims", defaultDims), 1, sourceEntry.maxDims)
		skip          = clampInt(readInt(opts, "skip", defaultSkip), 0, maxSkip)
		leap          = clampInt(readInt(opts, "leap", defaultLeap), 1, maxLeap)
		seed          = readUint64(opts, "seed", defaultSeed)
	)

	ceiling := spec.maxPoints(dims)
	if ceiling < minDiscrepancyPoints {
		// The library refuses this metric at this width. Report its own
		// sentence rather than a paraphrase, by asking it once more for the
		// shape the page wanted.
		_, err := spec.measure(unitMatrix(minDiscrepancyPoints, dims))
		if err != nil {
			return errorResult("discrepancy: %v", err)
		}

		return errorResult("discrepancy: %s cannot be computed at %d dimensions", spec.label, dims)
	}

	n := clampInt(readInt(opts, "n", minDiscrepancyPoints), minDiscrepancyPoints, ceiling)

	generator, err := newGenerator(source, dims, skip, leap, randomization, seed)
	if err != nil {
		return errorResult("discrepancy: %v", err)
	}

	// qmc.Draw rather than a hand-rolled loop: it is the library's own
	// statement of the skip/leap index convention (point i is raw index
	// skip + 1 + i*leap, decided inside the generator). The digit inspector's
	// separate explanatory mapping has dedicated agreement regressions. Draw
	// allocates one backing array for the matrix, which both metrics
	// walk more than once.
	value, err := spec.measure(qmc.Draw(generator, n))
	if err != nil {
		return errorResult("discrepancy: %v", err)
	}

	randomValue, err := spec.measure(randomMatrix(n, dims, seed))
	if err != nil {
		return errorResult("discrepancy: random baseline: %v", err)
	}

	var analytic any
	if spec.analytic != nil {
		analytic = jsNumber(spec.analytic(dims, n))
	}

	ratio := math.Inf(1)
	if value > 0 {
		ratio = randomValue / value
	}

	separates := ratio >= separatesRatio

	return map[string]any{
		"metric":        spec.key,
		"label":         spec.label,
		"n":             n,
		"dims":          dims,
		"maxPoints":     ceiling,
		"value":         jsNumber(value),
		"randomValue":   jsNumber(randomValue),
		"analytic":      analytic,
		"analyticKind":  spec.analyticKind,
		"analyticLabel": spec.analyticLabel,
		"ratio":         jsNumber(ratio),
		"separates":     separates,
		"verdict":       discrepancyVerdict(spec, dims, ratio, separates),

		"source":        source,
		"randomization": randomization,
		"skip":          skip,
		"leap":          leap,
		"seed":          float64(seed),
	}
}

// discrepancyVerdict writes the sentence under the headline ratio.
//
// Keep the display threshold and its explanation together. A single ratio
// compares two point sets, not integration error or statistical significance.
func discrepancyVerdict(spec discrepancySpec, dims int, ratio float64, separates bool) string {
	if math.IsInf(ratio, 1) {
		return fmt.Sprintf(
			"The computed sequence discrepancy is zero at %d dimensions, so no finite ratio can be reported. "+
				"Floating-point cancellation or a numerical floor may hide a positive value; zero does not establish a perfect point set.", dims,
		)
	}

	if separates {
		return fmt.Sprintf(
			"The pseudorandom set scores %.2fx the sequence's %s at %d dimensions. "+
				"This meets or exceeds the panel's 1.5x display threshold for these two sets. Compare other seeds and integrands before inferring integration accuracy.",
			ratio, spec.noun, dims,
		)
	}

	if spec.key == "star" {
		return fmt.Sprintf(
			"The pseudorandom set scores %.2fx the sequence's star discrepancy at %d dimensions. "+
				"This is below the panel's 1.5x display threshold. The result describes these two sets; it does not predict another seed, sample budget, or integrand.",
			ratio, dims,
		)
	}

	return fmt.Sprintf(
		"The pseudorandom set scores %.2fx the sequence's centred L2 discrepancy at %d dimensions. "+
			"This is below the panel's 1.5x display threshold. At high dimensions CD2 can distinguish point sets only weakly. Try smaller dimensions or another available metric and inspect the new measurements; no fixed improvement is guaranteed.",
		ratio, dims,
	)
}

// jsMetrics answers which metrics are available at the current dimension count,
// and how many points each can afford there.
//
// It is the discrepancy panel's leaps(): a control whose legal values depend on
// another control cannot answer for itself, and the alternative — letting the
// page find out by asking for a measurement and getting an error back — would
// make the metric menu look broken rather than narrow.
//
// Availability is decided by BUILDING the smallest possible request and reading
// the library's error, not by restating maxStarDims or starBoxBudget here. When
// star refuses, the page prints err.Error() verbatim: it names the dimension
// count, the leaf count, the fact that the ceiling is a property of the problem
// rather than a tuning knob, and the affordable point counts per dimension. No
// paraphrase of that is worth writing.
//
// suggestedDims mirrors leaps()' suggested: the largest dimension count at or
// below the current one where the metric is available, so the page can offer
// the fix and not only the refusal.
func jsMetrics(opts js.Value) any {
	source := readString(opts, "source", defaultSource)

	sourceEntry, ok := sources[source]
	if !ok || sourceEntry.construct == nil {
		return errorResult("metrics: unknown sequence %q", source)
	}

	dims := clampInt(readInt(opts, "dims", defaultDims), 1, sourceEntry.maxDims)

	list := make([]any, 0, len(discrepancyOrder))

	for _, key := range discrepancyOrder {
		spec := discrepancies[key]
		entry := map[string]any{
			"key":           spec.key,
			"label":         spec.label,
			"description":   spec.description,
			"dims":          dims,
			"analytic":      spec.analytic != nil,
			"analyticKind":  spec.analyticKind,
			"analyticLabel": spec.analyticLabel,
		}

		if _, err := spec.measure(unitMatrix(minDiscrepancyPoints, dims)); err != nil {
			entry["available"] = false
			entry["reason"] = err.Error()
			entry["maxPoints"] = 0
			entry["suggestedDims"] = suggestedDims(spec, dims)
		} else {
			entry["available"] = true
			entry["reason"] = nil
			entry["maxPoints"] = spec.maxPoints(dims)
			entry["suggestedDims"] = nil
		}

		list = append(list, entry)
	}

	return map[string]any{
		"source":  source,
		"dims":    dims,
		"metrics": list,
	}
}

// suggestedDims probes downward for the widest cube this metric still accepts.
//
// Probe the panel's two-point minimum; cheap one-point library cases do not
// determine this menu's availability. Returns nil rather than 0 when there is
// no such dimension count, so the page tests the field instead of comparing
// against a number that also means "one dimension is fine".
func suggestedDims(spec discrepancySpec, dims int) any {
	for d := dims - 1; d >= 1; d-- {
		if _, err := spec.measure(unitMatrix(minDiscrepancyPoints, d)); err == nil {
			return d
		}
	}

	return nil
}

// unitMatrix builds the cheapest point set of a given shape: n identical
// corners of the cube.
//
// Both refusals it is used to trigger depend only on n and dims, so the
// coordinates are free to be whatever costs least — and all-ones costs least,
// because it collapses star's candidate grid in every dimension to a single
// value. A coordinate of exactly 1 is inside validatePoints' [0,1] and is
// documented by StarDiscrepancy as needing no special case.
func unitMatrix(n, dims int) [][]float64 {
	flat := make([]float64, n*dims)
	for i := range flat {
		flat[i] = 1
	}

	rows := make([][]float64, n)
	for i := range rows {
		rows[i] = flat[i*dims : (i+1)*dims : (i+1)*dims]
	}

	return rows
}

// randomMatrix is the comparison set, drawn in Go from math/rand.
//
// It cannot go through qmc.Draw: sources["random"] has no constructor, because
// inventing a qmc.Sequence implementation whose only purpose is to be rejected
// by every other export would be worse than this loop. And it is drawn in Go
// rather than from Math.random() in the page for the reason points.go gives:
// reproducibility from the seed on screen is the axis on which the two
// samplers are being compared, and a JavaScript comparison set could not be
// reproduced from it.
func randomMatrix(n, dims int, seed uint64) [][]float64 {
	rng := rand.New(rand.NewSource(int64(seed))) //nolint:gosec // not cryptography; reproducibility is the requirement

	flat := make([]float64, n*dims)
	for i := range flat {
		flat[i] = rng.Float64()
	}

	rows := make([][]float64, n)
	for i := range rows {
		rows[i] = flat[i*dims : (i+1)*dims : (i+1)*dims]
	}

	return rows
}
