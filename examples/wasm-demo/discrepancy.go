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
// It exists for the same reason every other limit in this package does: an
// unclamped n arrives from a console or a stale script as a plain double, and
// a discrepancy call allocates an n-by-dims matrix before it computes
// anything. 8192 sits above every ceiling below it at two dimensions and up,
// so it never pre-empts a refusal the library would have explained better. At
// one dimension the library's leaf count is only N+1 and it would accept
// millions; there this is the binding limit, and it is the probe buffer's size
// as much as anything — starMaxPoints allocates one row per candidate point.
const maxDiscrepancyPoints = 8192

// minDiscrepancyPoints is 2 because both statistics are defined for one point
// and neither says anything with it: the sweep's first rung has to be a point
// set, not a point.
const minDiscrepancyPoints = 2

// cd2CallBudgetNs is how long one centred-L2 call may hold the browser's only
// thread, in nanoseconds. It is the lever the n ceiling is derived from.
//
// 150 ms is a visible pause and not a hang. It is not a mathematical limit —
// CenteredL2Discrepancy will measure a hundred thousand points if you have the
// patience — it is a statement about responsiveness, and that distinction is
// what the note on screen has to make, because this is the only ceiling on
// either page that MOVES when another control moves.
const cd2CallBudgetNs = 150e6

// cd2NsPerTerm and cd2NsPerPair are the measured cost of one jsDiscrepancy
// call under js/wasm — BOTH point sets, sequence and pseudorandom, since that
// is what a rung of the sweep actually costs.
//
// Measured in Chrome through qmc.discrepancy() from the console, over dims
// 1..64 and N 64..7072. The per-pair cost is affine in the dimension count
// rather than proportional to it, because the double sum's loop overhead does
// not shrink when s does:
//
//	 s     N      measured   ns per pair   the model
//	 1   7072      332 ms        13.3         13.2
//	 2   5001      220 ms        17.6         18.9
//	 4   3536      176 ms        28.1         30.3
//	16   1768      173 ms       111           98.7
//	39   1133      150 ms       234          229
//	64    884      146 ms       373          372
//
// so cost(N,s) = N*(N-1)/2 * (5.7*s + 7.5) nanoseconds. Fitting only the
// proportional term would have made the ceiling at one or two dimensions three
// times too generous: 7072 points at one dimension is 332 ms, not 150.
const (
	cd2NsPerTerm = 5.7
	cd2NsPerPair = 7.5
)

// starDemoPoints caps N per dimension count on top of whatever
// qmc.StarDiscrepancy itself accepts.
//
// The library's ceiling is a statement about what is COMPUTABLE. Under
// js/wasm, at that ceiling, it is also a statement about a frozen tab:
// measured in the browser, one call at the library's own limit takes 1.96 s at
// 2 dimensions, 5.57 s at 3, 5.04 s at 4, 4.80 s at 5 and 4.97 s at 6. So the
// demo needs a second, smaller ceiling, and it is about responsiveness rather
// than tractability. It cannot be modelled from outside either: the pruner
// makes the cost depend on the point set and not only on its shape.
//
// The entries below are MEASURED, one per dimension count, chosen so that a
// single rung costs about the same everywhere — 353 ms at 2 dimensions, 351 at
// 3, 378 at 4, 355 at 5, 420 at 6. Star gets a larger per-call budget than
// centred L2 because its ladder is short: at 4 dimensions it is six rungs from
// 16 points to 84, and only the last two cost anything, so the whole sweep
// still finishes in about a second. At 6 dimensions it is three rungs.
//
// Note what this table is NOT: it is not a copy of the library's limit. The
// library is still asked, by starMaxPoints below, and whichever ceiling is
// lower wins. A dimension count absent from the map is one the library refuses
// outright, and nothing here has to know which ones those are.
var starDemoPoints = map[int]int{
	1: 4096,
	2: 1792,
	3: 224,
	4: 84,
	5: 46,
	6: 32,
}

// separatesRatio is the random-to-sequence ratio at which the panel says the
// statistic is still telling you something.
//
// The threshold is a judgement about a decay, not a cliff, so it has to be
// picked from the measured curve rather than derived. At N=1024, over ten
// scrambling seeds, the centred-L2 ratio of pseudorandom to scrambled Halton
// runs:
//
//	 s     ratio
//	 2     12.5x
//	 5      6.3x
//	10      2.4x
//	15      1.6x
//	20      1.28x
//	30      1.08x
//	39      1.02x
//
// 1.5 lands between 15 and 20 dimensions, which is exactly where
// CenteredL2Discrepancy's own doc comment puts the transition from "weak" to
// "dead". Deciding this in Go rather than thresholding it in JavaScript is the
// same discipline the leap refusal follows: the sentence the page prints is
// the one the code that knows the numbers wrote.
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

	// analytic is the expectation of the metric over N independent uniform
	// points, in closed form, or nil when there is no such form. It costs
	// nothing to evaluate and it is what makes saturation visible: when the
	// sequence's curve lies on top of it, the statistic is measuring the
	// marginals and not the point set.
	analytic func(dims, n int) float64
}

// discrepancyOrder fixes the order the page lists them in; a map alone would
// reshuffle the dropdown on every load.
var discrepancyOrder = []string{"cl2", "star"}

var discrepancies = map[string]discrepancySpec{
	"cl2": {
		key:         "cl2",
		label:       "Centred L2 (CD2)",
		noun:        "centred L2 discrepancy",
		description: "Hickernell's centred L2 discrepancy, accumulated over all 2^s-1 coordinate projections. O(N^2 s), defined in every dimension, and saturating: above roughly twenty dimensions it scores a low-discrepancy set within 30% of a random one.",
		measure:     qmc.CenteredL2Discrepancy,
		maxPoints:   cd2MaxPoints,

		// E[CD2^2] = ((5/4)^s - (13/12)^s)/N, exact, derived in
		// CenteredL2Discrepancy's doc comment. Confirmed against a measured
		// random baseline to 0.6% at 39 dimensions.
		analytic: func(dims, n int) float64 {
			s := float64(dims)

			return math.Sqrt((math.Pow(1.25, s) - math.Pow(13.0/12.0, s)) / float64(n))
		},
	},
	"star": {
		key:         "star",
		label:       "Star discrepancy (exact)",
		noun:        "star discrepancy",
		description: "The exact supremum over origin-anchored boxes: the quantity the Koksma-Hlawka bound multiplies by an integrand's variation. It does not saturate, and it is NP-hard in the dimension, so the library refuses it above a few dimensions and a few hundred points.",
		measure:     qmc.StarDiscrepancy,
		maxPoints:   starMaxPoints,

		// No closed form for the expected star discrepancy of N uniform
		// points exists in elementary terms, and the known asymptotics
		// (sqrt(s/N) up to constants nobody agrees on) would be a curve the
		// page could not defend. So there is no third series here, and the
		// chart draws two.
		analytic: nil,
	},
}

// cd2MaxPoints solves the measured cost model above for N: the largest point
// count whose N*(N-1)/2 pairs, at 5.7*dims + 7.5 nanoseconds each, fit inside
// cd2CallBudgetNs.
//
// The result moves with the dimension slider, which no other control on either
// page does, so the panel says so in words. It is a browser-responsiveness
// limit and not a mathematical one: the library will measure as many points as
// you have patience for.
func cd2MaxPoints(dims int) int {
	if dims < 1 {
		dims = 1
	}

	perPair := cd2NsPerTerm*float64(dims) + cd2NsPerPair
	n := int(math.Sqrt(2 * cd2CallBudgetNs / perPair))

	return clampInt(n, minDiscrepancyPoints, maxDiscrepancyPoints)
}

// starMaxPoints reports the largest point count that both the library will
// accept and the browser will sit still for, at this dimension count.
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

	// The cheap refusal first, on a two-row matrix. Above six dimensions the
	// library refuses at any point count, and allocating the full probe buffer
	// — 8192 rows times a dimension count that may be 64 — to discover that
	// would be four megabytes of a 32-bit heap thrown away.
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
// and — for centred L2 — the analytic expectation of the random one.
//
// One n per call, for exactly the reason converge.go gives at length: a call
// into wasm occupies the browser's only thread for its whole duration, so the
// sweep loop has to live in JavaScript, where it can yield, and that yield is
// the only cancellation mechanism there is.
//
// And here is the limit that pattern does not reach.
// CenteredL2Discrepancy(points) is ATOMIC. A single n cannot be subdivided
// without reimplementing Hickernell's double sum inside the demo — precisely
// the duplication this module avoids everywhere else — so the n ceiling is the
// only lever, and cd2MaxPoints is that lever. It moves when the dimension
// slider moves, which is unlike every other control on either page, and the
// panel has to say so on screen or it reads as a bug.
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
	// skip + 1 + i*leap, decided inside the generator), and re-deriving that
	// convention here is a bug digits.go already has. It allocates one backing
	// array for the whole matrix, which is what the metrics want anyway — both
	// of them walk it more than once.
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
		"metric":      spec.key,
		"label":       spec.label,
		"n":           n,
		"dims":        dims,
		"maxPoints":   ceiling,
		"value":       jsNumber(value),
		"randomValue": jsNumber(randomValue),
		"analytic":    analytic,
		"ratio":       jsNumber(ratio),
		"separates":   separates,
		"verdict":     discrepancyVerdict(spec, dims, ratio, separates),

		"source":        source,
		"randomization": randomization,
		"skip":          skip,
		"leap":          leap,
		"seed":          float64(seed),
	}
}

// discrepancyVerdict writes the sentence under the headline ratio.
//
// It is written here and not in JavaScript for the same reason the leap
// refusal is: the threshold and the measured table it was picked from live in
// this file, and a page that re-decided "is 1.28 close to 1?" for itself would
// be a second copy of a judgement, free to disagree with the one the constant
// encodes.
func discrepancyVerdict(spec discrepancySpec, dims int, ratio float64, separates bool) string {
	if math.IsInf(ratio, 1) {
		return fmt.Sprintf(
			"The sequence scores exactly 0 on this metric at %d dimensions, so the ratio is unbounded. "+
				"That is a degenerate point set, not a perfect one — raise N.", dims,
		)
	}

	if separates {
		return fmt.Sprintf(
			"The pseudorandom set scores %.2fx the sequence's %s. At %d dimensions this statistic "+
				"still sees the difference between an evenly spread point set and an unstructured one.",
			ratio, spec.noun, dims,
		)
	}

	if spec.key == "star" {
		return fmt.Sprintf(
			"The pseudorandom set scores only %.2fx the sequence's star discrepancy at %d dimensions. "+
				"Star does not saturate, so this is a fact about these two point sets and not about the "+
				"statistic — at this few points the sequence has not yet pulled away.",
			ratio, dims,
		)
	}

	return fmt.Sprintf(
		"The pseudorandom set scores only %.2fx the sequence's centred L2 discrepancy at %d dimensions. "+
			"The (5/4)^s diagonal term dominates the expectation here, so the number is about each "+
			"coordinate's marginal spread and not about how the points sit relative to one another. "+
			"Drop to 4 dimensions and star discrepancy becomes available and separates the same two sets "+
			"by about 2x at the point counts it can afford; drop centred L2 to 2 dimensions and it "+
			"separates them by more than 12x.",
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
			"key":         spec.key,
			"label":       spec.label,
			"description": spec.description,
			"dims":        dims,
			"analytic":    spec.analytic != nil,
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
// Downward and one step at a time, because the answer is small — the library's
// star ceiling is six dimensions — and because a probe is a two-point matrix
// whose validation is the whole cost. Returns nil rather than 0 when there is
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
