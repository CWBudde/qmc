//go:build js && wasm

package main

import (
	"fmt"
	"math"
	"syscall/js"
)

// guard turns a recovered Go callback panic into a failed request. Recovery
// does not terminate the runtime: subsequent safe requests can still succeed.
// Runtime throws, traps, and actual program exit are separate terminal events
// observed by the JavaScript runtime monitor; recover cannot handle them all.
func guard(name string, fn func(js.Value) any) js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) (result any) {
		defer func() {
			if r := recover(); r != nil {
				result = js.ValueOf(map[string]any{
					"error": fmt.Sprintf("%s: %v", name, r),
					"panic": true,
				})
			}
		}()

		// Normalize the top-level options as well as individual fields. Exports
		// that access optional output buffers can then safely call opts.Get.
		var opts js.Value
		if len(args) > 0 && isObject(args[0]) {
			opts = args[0]
		} else {
			opts = js.Global().Get("Object").New()
		}

		return fn(opts)
	})
}

// errorResult distinguishes validation failures from recovered callback panics.
// Neither flag by itself indicates that the runtime has terminated.
func errorResult(format string, args ...any) map[string]any {
	return map[string]any{
		"error": fmt.Sprintf(format, args...),
		"panic": false,
	}
}

func isObject(value js.Value) bool {
	return value.Type() == js.TypeObject && !value.IsNull()
}

// The read* helpers are deliberately tolerant: a missing key, a null, or a
// value of the wrong type yields the fallback rather than an error. The page
// sends partial option objects all the time (a control the user has not
// touched yet has nothing to send), and treating that as a failure would mean
// every caller had to fill in defaults the Go side already knows — which is
// exactly the duplication the info() capability table exists to avoid.

// readInt reads a count, a dimension index, a skip — anything the page sends as
// a plain integer option.
//
// The range check before the conversion is the load-bearing part. JavaScript
// has one number type and it is a double, so `{count: 1e300}` typed into the
// console, or left in a stale cached script, arrives here as a perfectly
// ordinary finite value that is nowhere near representable as an int. The Go
// spec leaves float-to-integer conversion implementation-defined when the
// value does not fit the target type, so int(1e300) is not "some big number",
// it is whatever the target happens to produce — and clampInt cannot recognise
// it as out of range afterwards, because it may well land inside the limits.
// That would quietly defeat the whole point of clamping in Go rather than
// trusting the page (see the note above the limits in info.go).
//
// int is 64 bits wide on js/wasm (only uintptr is 32; see the wasm job in
// .github/workflows/test.yml), so the threshold is 2^63, not 2^31 — but the
// page can reach it with one keystroke, and NaN/Inf are already handled above.
//
// So the value is saturated to the int range while it is still a float64, and
// only then converted. Saturating rather than rejecting matches clampInt and
// the rest of the read* helpers: a control dragged to its end stop should hit
// the limit, not raise an error. The bounds come from math.MaxInt/math.MinInt
// so this stays correct on a 64-bit host build as well as under wasm.
func readInt(opts js.Value, key string, fallback int) int {
	if !isObject(opts) {
		return fallback
	}

	value := opts.Get(key)
	if value.Type() != js.TypeNumber {
		return fallback
	}

	number := value.Float()
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return fallback
	}

	if number >= math.MaxInt {
		return math.MaxInt
	}

	if number <= math.MinInt {
		return math.MinInt
	}

	return int(number)
}

// readUint64 reads a scrambling seed.
//
// It goes through float64 because that is the only number JavaScript has: a
// seed typed into the page arrives here as a double, never as an integer, so
// readUint64 keeps seed handling separate from signed point-count options.
// GOARCH=wasm uses a 64-bit int, while JavaScript numbers have 53 bits of
// integer precision. Negative values clamp to 0 rather than wrapping to a huge
// uint64, and values beyond 2^53 are already not exactly representable on the
// JavaScript side, so the page is told to keep seeds small.
func readUint64(opts js.Value, key string, fallback uint64) uint64 {
	if !isObject(opts) {
		return fallback
	}

	value := opts.Get(key)
	if value.Type() != js.TypeNumber {
		return fallback
	}

	number := value.Float()
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return fallback
	}

	if number <= 0 {
		return 0
	}

	if number >= math.MaxUint64 {
		return math.MaxUint64
	}

	return uint64(number)
}

func readString(opts js.Value, key, fallback string) string {
	if !isObject(opts) {
		return fallback
	}

	value := opts.Get(key)
	if value.Type() != js.TypeString {
		return fallback
	}

	return value.String()
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}

	if value > high {
		return high
	}

	return value
}

// jsNumber renders a float for JavaScript. NaN and ±Inf are not representable
// in JSON and arrive in JS as values no formatter handles, so they become null
// and the page renders them as "—" rather than as "NaN".
func jsNumber(value float64) any {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}

	return value
}

// intsToJS builds a plain JS array of numbers, for the short results — a
// prime-base list, a digit expansion, a permutation alphabet — where a typed
// array would cost more in ceremony on both sides than it saves in bytes.
func intsToJS(values []int) []any {
	items := make([]any, len(values))
	for i, value := range values {
		items[i] = value
	}

	return items
}

func int32sToJS(values []int32) []any {
	items := make([]any, len(values))
	for i, value := range values {
		items[i] = int(value)
	}

	return items
}
