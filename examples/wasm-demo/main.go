//go:build js && wasm

// Command wasm-demo is the browser front end for github.com/cwbudde/qmc.
//
// The rule this package exists to enforce: no sampling logic lives in
// JavaScript. Every point on the scatter plot, every entry of the correlation
// heatmap, every digit in the inspector and even the pseudo-random comparison
// set is produced by the library compiled to js/wasm. The JavaScript side owns
// the DOM, the canvases and the animation clock, and nothing else. A demo that
// reimplemented the radical inverse in JS would be demonstrating the JS, not
// the library — and the moment the two definitions drifted the page would go
// on drawing plausible-looking points that were no longer the sequence under
// discussion.
package main

import "syscall/js"

// exports lists every function the demo publishes, by its name on the
// namespaced globalThis.qmc object. Each one is wrapped by guard, which is the
// single rule this bridge has: nothing reaches JavaScript without a recover()
// in front of it (see bridge.go).
var exports = map[string]func(js.Value) any{
	"info":      jsInfo,
	"points":    jsPoints,
	"correlate": jsCorrelate,
	"converge":  jsConverge,
	"digits":    jsDigits,
	"leaps":     jsLeaps,

	"discrepancy": jsDiscrepancy,
	"metrics":     jsMetrics,
}

// live keeps the js.Func values referenced so they are never released. A
// js.Func that is garbage collected on the Go side leaves a dead callback slot
// behind, and calling it from the page throws.
var live []js.Func

func main() {
	namespace := js.Global().Get("Object").New()

	for name, fn := range exports {
		wrapped := guard(name, fn)
		live = append(live, wrapped)
		namespace.Set(name, wrapped)
	}

	js.Global().Set("qmc", namespace)

	// main must not return: the Go runtime tears the instance down when it
	// does, taking every exported function with it. The JavaScript side knows
	// this and never awaits go.run().
	select {}
}
