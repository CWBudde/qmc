//go:build js && wasm

package main

import (
	"runtime"
	"syscall/js"

	"github.com/cwbudde/qmc"
)

// The limits every export clamps against.
//
// They are enforced here, in Go, and not in the page's <input max=""> — an
// attribute is a suggestion that any console, any stale cached script and any
// hand-edited URL can ignore, and the failure it lets through is not a
// mis-rendered chart. Under GOARCH=wasm the linear memory a browser will hand
// out is small, and uintptr is 32 bits wide even though int is 64. An
// unclamped dims of a few million reaches primesUpTo, which sieves 15*dims
// bools and panics when that does not fit; an unclamped count multiplies into
// the float32 buffer size and can overflow int outright, producing a negative
// length and a runtime throw. Clamping rather than rejecting keeps a dragged
// slider from erroring at its own end stop.
const (
	maxDims  = 64
	maxSkip  = 4096
	maxIndex = 1000000

	// maxLeap is small on purpose, and the number that fixes it is Sobol's.
	//
	// A leap multiplies the raw index: point i is raw index skip+1+i*leap, so
	// the largest raw index this page can ask for is maxLeap times the largest
	// count any export offers. Sobol's direction numbers run out at 2^32 and
	// fill panics past that, which guard() turns into a failed request. The
	// binding case is the convergence sweep at
	// maxConvergeN = 200,000 points; 200,000 * 1000 is 2e8, two decimal orders
	// below the ceiling, and every other export is smaller still. Halton has
	// no such wall but does grow a digit per factor of the base, which is the
	// other reason not to offer a leap of a million.
	maxLeap = 1000
)

// The shared defaults. They are not neutral: they aim the demo straight at the
// library's headline defect. A 39-dimensional generator drawn 600 times, with
// the scatter plot showing dimensions 37 and 38 — bases 163 and 167 — is
// the configuration measured in correlation_test.go. At this budget each base
// has completed several leading-digit cycles, but the slower higher digits
// remain poorly explored. The selected pair is illustrative; the worst pair
// across all adjacent dimensions is reported separately by correlate().
const (
	defaultDims   = 39
	defaultCount  = 600
	defaultSkip   = 64
	defaultSeed   = 1
	defaultAxisX  = 37
	defaultAxisY  = 38
	defaultSource = "halton"

	// defaultLeap is 1, which is exactly "no leaping": WithLeap(1) is
	// bit-identical to no option at all, so the page opens on the same point
	// set it did before the control existed.
	defaultLeap = 1

	// The unrandomized entry of every source's menu. Its key is shared across
	// sources so that switching sequence keeps a request valid without the
	// page having to know what the new source's menu contains.
	randomizationNone = "none"

	// defaultMetric aims the discrepancy panel at the defect the same way
	// defaultDims aims the scatter plot at it. Centred L2 at 39 dimensions is
	// the configuration in which the statistic says nothing — the sequence's
	// curve and the pseudorandom one lie on top of each other and the ratio
	// reads about 1.02 — so the page opens on the null result and the note
	// beside it explains how to get a real one.
	defaultMetric = "cl2"
)

// A randomizationSpec is one entry of a source's randomization menu.
//
// option is what turns the key into a library call, and it is nil for
// randomizationNone. Which options a generator accepts is NOT encoded here:
// the constructors already refuse an option that does not apply, by name, and
// a second copy of that policy in the demo would be one that could disagree
// with the library after a release. What is encoded here is only the menu each
// source offers, which is a UI question.
type randomizationSpec struct {
	key         string
	label       string
	description string

	option func(seed uint64) qmc.Option
}

// randomizationOrder fixes the order within a menu; a map alone would
// reshuffle the dropdown on every load.
var randomizationOrder = []string{"none", "scramble", "nested", "shift", "owen"}

// Every description here is taken from the option's doc comment in the
// library, including the parts that are unflattering. A demo that advertised
// nested scrambling as a free upgrade would be contradicting nested.go, which
// measured the case where it is not.
var randomizations = map[string]randomizationSpec{
	randomizationNone: {
		key:         randomizationNone,
		label:       "None",
		description: "The deterministic sequence, identical on every run.",
	},
	"scramble": {
		key:         "scramble",
		label:       "Random-digit scrambling",
		description: "One seeded permutation per dimension, reused at every digit position. Preserves interval structure, but does not give uniform point marginals or guarantee unbiased estimates; seed spread can miss bias.",
		option:      qmc.WithScrambling,
	},
	"nested": {
		key:         "nested",
		label:       "Nested scrambling",
		description: "A seeded Fisher–Yates permutation per node, conditioned on the digits above it. Finite hashes and truncated tails approximate ideal nested randomization; seed spread does not measure its bias. Building permutations per point can be expensive, especially at high prime bases. Accuracy depends on the integrand and sampling budget; compare repeated seeds.",
		option:      qmc.WithNestedScrambling,
	},
	"shift": {
		key:         "shift",
		label:       "Digital shift",
		description: "One seeded pseudorandom 32-bit word per dimension, XORed into every point. Ideal independent words make points uniform on the finite grid, not the continuous cube. It translates the whole net rigidly, so a projection that is poorly distributed stays poorly distributed under every shift.",
		option:      qmc.WithDigitalShift,
	},
	"owen": {
		key:         "owen",
		label:       "Owen scrambling",
		description: "Hash-based nested bit flips on a 32-bit grid. Node flips need not be independent; the scramble preserves dyadic occupancy and cannot repair a poor table. Accuracy and cost comparisons depend on the integrand, sampling budget, and access method.",
		option:      qmc.WithOwenScrambling,
	},
}

// A sourceSpec is one entry of the sequence menu, and it is the only place the
// page learns what a source can and cannot answer.
//
// primeBases and digits exist so the page can hide a panel rather than guess.
// The alternative — letting the page infer "Sobol has no bases" from a null in
// the points result — would leave the prime-base labels showing Halton's 163
// and 167 next to Sobol data until the first response came back, which is the
// one reading the page must never invite.
type sourceSpec struct {
	key         string
	label       string
	description string

	// construct is nil for a source that is not a qmc.Sequence at all. The
	// pseudo-random comparison set is drawn in points.go from math/rand, and
	// giving it a constructor here would mean inventing a Sequence
	// implementation whose only purpose is to be rejected by every other
	// export.
	construct func(dims int, opts ...qmc.Option) (qmc.Sequence, error)

	// maxDims is this source's own ceiling, already clamped into the shared
	// one. See sobolMaxDims for why the two can differ.
	maxDims int

	primeBases bool
	digits     bool

	randomizations []string
}

// sobolMaxDims is the largest dimension count this page offers for Sobol.
//
// The embedded Joe-Kuo table covers 1024 dimensions and NewSobol refuses more,
// but that constant is unexported, so the number is written out here rather
// than read from the library. The minimum against maxDims is what makes it
// safe: today the shared clamp is far below 1024 and binds first, so the
// figure below is not load-bearing, and if a future table were smaller than
// maxDims this is where the page would learn it — from a per-source field the
// controls already respect, not from an error after the fact.
const sobolMaxDims = min(sobolTableDims, maxDims)

// sobolTableDims is the dimension count of the embedded Joe-Kuo table, which
// NewSobol will not exceed.
const sobolTableDims = 1024

// sourceOrder fixes the order the page lists sequences in.
var sourceOrder = []string{"halton", "sobol", "random"}

var sources = map[string]sourceSpec{
	"halton": {
		key:         "halton",
		label:       "Halton",
		description: "Radical inverse of the index in the d-th prime base, one base per dimension.",
		construct: func(dims int, opts ...qmc.Option) (qmc.Sequence, error) {
			// The result is assigned to the interface only after the error is
			// out of the way. Returning the *qmc.Halton unconditionally would
			// hand back a non-nil interface holding a nil pointer on the error
			// path, and every `if generator == nil` upstream would miss it.
			generator, err := qmc.NewHalton(dims, opts...)
			if err != nil {
				return nil, err
			}

			return generator, nil
		},
		maxDims:        maxDims,
		primeBases:     true,
		digits:         true,
		randomizations: []string{randomizationNone, "scramble", "nested"},
	},
	"sobol": {
		key:         "sobol",
		label:       "Sobol",
		description: "Joe-Kuo direction numbers, base 2 in every dimension, generated in Gray-code order.",
		construct: func(dims int, opts ...qmc.Option) (qmc.Sequence, error) {
			generator, err := qmc.NewSobol(dims, opts...)
			if err != nil {
				return nil, err
			}

			return generator, nil
		},
		maxDims:        sobolMaxDims,
		primeBases:     false,
		digits:         false,
		randomizations: []string{randomizationNone, "shift", "owen"},
	},
	"random": {
		key:         "random",
		label:       "Pseudo-random",
		description: "Independent uniform draws from math/rand, seeded the same way, drawn in Go so that the comparison set is reproducible from the seed on screen.",
		maxDims:     maxDims,
	},
}

// jsInfo is the capability table the page builds its controls from.
//
// Every limit, every source, every randomization and every integrand the UI
// offers comes from here rather than from the markup. The <select> elements in
// the static HTML are empty placeholders that the page fills in as soon as this
// call returns, so adding an integrand to converge.go or a randomization to the
// table above puts it in the dropdown without anyone editing a .html file —
// and, more importantly, a limit can never disagree between the slider that
// enforces it and the Go code that actually clamps it.
func jsInfo(opts js.Value) any {
	dims := clampInt(readInt(opts, "dims", defaultDims), 1, maxConvergeDims)
	list := make([]any, 0, len(integrandOrder))

	for _, key := range integrandOrder {
		spec := integrands[key]
		list = append(list, map[string]any{
			"key":         spec.key,
			"label":       spec.label,
			"description": spec.description,

			// Use the same dimension clamp and exact function as converge().
			"exact":   jsNumber(spec.exact(dims)),
			"dims":    dims,
			"minDims": 1,
			"maxDims": maxConvergeDims,
		})
	}

	metricList := make([]any, 0, len(discrepancyOrder))

	for _, key := range discrepancyOrder {
		spec := discrepancies[key]
		metricList = append(metricList, map[string]any{
			"key":         spec.key,
			"label":       spec.label,
			"description": spec.description,
			"analytic":    spec.analytic != nil,
		})
	}

	sourceList := make([]any, 0, len(sourceOrder))

	for _, key := range sourceOrder {
		spec := sources[key]
		sourceList = append(sourceList, map[string]any{
			"key":         spec.key,
			"label":       spec.label,
			"description": spec.description,

			// A source with no constructor is the pseudo-random baseline: it
			// belongs in the comparison panel and not in the sequence menu,
			// and the page decides that from this flag rather than by
			// special-casing the string "random".
			"sequence":       spec.construct != nil,
			"maxDims":        spec.maxDims,
			"primeBases":     spec.primeBases,
			"digits":         spec.digits,
			"randomizations": randomizationList(spec),
		})
	}

	return map[string]any{
		"goVersion": runtime.Version(),
		"goos":      runtime.GOOS,
		"goarch":    runtime.GOARCH,

		"maxDims":            maxDims,
		"maxPoints":          maxPoints,
		"maxSkip":            maxSkip,
		"maxLeap":            maxLeap,
		"maxIndex":           maxIndex,
		"maxCorrelateDims":   maxCorrelateDims,
		"maxCorrelatePoints": maxCorrelatePoints,

		// The hard cap only. There is deliberately no single
		// maxDiscrepancyPoints-per-metric here: both metrics' affordable point
		// counts depend on the dimension count, and publishing one number would
		// be the same trap as reporting integrands.exact at a hardcoded
		// dimension count above. The live number comes from metrics().
		"maxDiscrepancyPoints": maxDiscrepancyPoints,
		"minDiscrepancyPoints": minDiscrepancyPoints,

		"sources": sourceList,

		"integrands": list,

		// The menu only — key, label and description. Whether a metric is
		// available, and how many points it can afford, are questions about
		// the dimension count now selected, so they are answered by metrics()
		// and never cached from here.
		"discrepancies": metricList,

		"defaults": map[string]any{
			"dims":          defaultDims,
			"count":         defaultCount,
			"skip":          defaultSkip,
			"leap":          defaultLeap,
			"seed":          defaultSeed,
			"axisX":         defaultAxisX,
			"axisY":         defaultAxisY,
			"source":        defaultSource,
			"randomization": randomizationNone,
			"metric":        defaultMetric,
		},
	}
}

// randomizationList renders one source's menu, in randomizationOrder.
func randomizationList(spec sourceSpec) []any {
	out := make([]any, 0, len(spec.randomizations))

	for _, key := range randomizationOrder {
		if !hasRandomization(spec, key) {
			continue
		}

		entry := randomizations[key]
		description := entry.description

		if key == randomizationNone {
			switch spec.key {
			case "halton":
				description += " High prime bases can produce long coordinate ramps and strong correlations at small sample budgets. Burn-in does not guarantee a cure."
			case "sobol":
				description += " Uses base 2 in every dimension, without Halton's high-prime ramps. Projection quality depends on the direction table and the sampled block; use aligned power-of-two blocks for net guarantees."
			}
		}

		out = append(out, map[string]any{
			"key":         entry.key,
			"label":       entry.label,
			"description": description,
		})
	}

	return out
}

func hasRandomization(spec sourceSpec, key string) bool {
	for _, offered := range spec.randomizations {
		if offered == key {
			return true
		}
	}

	return false
}
