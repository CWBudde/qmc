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
)

// The shared defaults. They are not neutral: they aim the demo straight at the
// library's headline defect. A 39-dimensional generator drawn 600 times, with
// the scatter plot showing dimensions 37 and 38 — bases 163 and 167 — is
// exactly the configuration measured in correlation_test.go, where the
// unscrambled sequence correlates those two coordinates at 0.65 after a
// 64-point burn-in. Open the page and the diagonal stripe is the first thing
// you see; pick a randomization and it dissolves.
const (
	defaultDims   = 39
	defaultCount  = 600
	defaultSkip   = 64
	defaultSeed   = 1
	defaultAxisX  = 37
	defaultAxisY  = 38
	defaultSource = "halton"

	// The unrandomized entry of every source's menu. Its key is shared across
	// sources so that switching sequence keeps a request valid without the
	// page having to know what the new source's menu contains.
	randomizationNone = "none"
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
		description: "The deterministic sequence, identical on every run. Reproducible, and above roughly twenty dimensions not actually filling the box at practical sample counts.",
	},
	"scramble": {
		key:         "scramble",
		label:       "Random-digit scrambling",
		description: "One uniform permutation of the digit alphabet per dimension, reused at every digit position. Still low-discrepancy, no longer identical across seeds.",
		option:      qmc.WithScrambling,
	},
	"nested": {
		key:         "nested",
		label:       "Nested scrambling",
		description: "A fresh uniform digit permutation per node of the scramble tree, conditioned on the digits above the digit being rewritten. At 39 dimensions it integrates about twice as accurately as random-digit scrambling — 41x against Monte Carlo over 40 seeds, against 24x — and its worst adjacent-pair |r| over 30 seeds is 0.141 against 0.161. It costs roughly forty times as much per point, which is what the uniform draw buys.",
		option:      qmc.WithNestedScrambling,
	},
	"shift": {
		key:         "shift",
		label:       "Digital shift",
		description: "One uniform 32-bit word per dimension, XORed into every point: the cheapest randomization a digital net admits. It translates the whole net rigidly, so a projection that is poorly distributed stays poorly distributed under every shift.",
		option:      qmc.WithDigitalShift,
	},
	"owen": {
		key:         "owen",
		label:       "Owen scrambling",
		description: "An independent bit flip at every node of each coordinate's binary tree, hashed rather than stored. It redistributes rather than translating, and measured 1.08x more accurate than a digital shift on the package's 39-dimensional integrand. Nearly free on At, three times the cost on Next.",
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
func jsInfo(_ js.Value) any {
	list := make([]any, 0, len(integrandOrder))

	for _, key := range integrandOrder {
		spec := integrands[key]
		list = append(list, map[string]any{
			"key":         spec.key,
			"label":       spec.label,
			"description": spec.description,

			// Reported at the dimension count converge() will actually use for
			// the shared default (39 clamped to maxConvergeDims), because the
			// gaussian integrand's exact value depends on d. The authoritative
			// value for a given call is the "exact" field converge() returns;
			// the page must read that back rather than cache this one.
			"exact":   jsNumber(spec.exact(clampInt(defaultDims, 1, maxConvergeDims))),
			"minDims": 1,
			"maxDims": maxConvergeDims,
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
		"maxIndex":           maxIndex,
		"maxCorrelateDims":   maxCorrelateDims,
		"maxCorrelatePoints": maxCorrelatePoints,

		"sources": sourceList,

		"integrands": list,

		"defaults": map[string]any{
			"dims":          defaultDims,
			"count":         defaultCount,
			"skip":          defaultSkip,
			"seed":          defaultSeed,
			"axisX":         defaultAxisX,
			"axisY":         defaultAxisY,
			"source":        defaultSource,
			"randomization": randomizationNone,
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
		out = append(out, map[string]any{
			"key":         entry.key,
			"label":       entry.label,
			"description": entry.description,
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
