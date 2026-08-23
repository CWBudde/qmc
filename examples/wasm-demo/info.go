//go:build js && wasm

package main

import (
	"runtime"
	"syscall/js"
)

// The limits every export clamps against.
//
// They are enforced here, in Go, and not in the page's <input max=""> — an
// attribute is a suggestion that any console, any stale cached script and any
// hand-edited URL can ignore, and the failure it lets through is not a
// mis-rendered chart. GOARCH=wasm is a 32-bit target: int is 32 bits wide, and
// the linear memory a browser will hand out is smaller still. An unclamped
// dims of a few million reaches primesUpTo, which sieves 15*dims bools and
// panics when that does not fit; an unclamped count multiplies into the
// float32 buffer size and can overflow int outright, producing a negative
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
// 64-point burn-in. Open the page, switch scrambling off, and the diagonal
// stripe is the first thing you see.
const (
	defaultDims  = 39
	defaultCount = 600
	defaultSkip  = 64
	defaultSeed  = 1
	defaultAxisX = 37
	defaultAxisY = 38
)

// jsInfo is the capability table the page builds its controls from.
//
// Every limit, every source and every integrand the UI offers comes from here
// rather than from the markup. The <select> elements in the static HTML are
// empty placeholders that the page fills in as soon as this call returns, so
// adding an integrand to converge.go puts it in the dropdown without anyone
// editing a .html file — and, more importantly, a limit can never disagree
// between the slider that enforces it and the Go code that actually clamps it.
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

		"sources": []any{
			map[string]any{"key": "halton", "label": "Halton"},
			map[string]any{"key": "random", "label": "Pseudo-random"},
		},

		"integrands": list,

		"defaults": map[string]any{
			"dims":  defaultDims,
			"count": defaultCount,
			"skip":  defaultSkip,
			"seed":  defaultSeed,
			"axisX": defaultAxisX,
			"axisY": defaultAxisY,
		},
	}
}
