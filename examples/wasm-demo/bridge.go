//go:build js && wasm

package main

import (
	"fmt"
	"math"
	"syscall/js"
)

// guard wraps a demo entry point so that no failure inside Go can ever reach
// the JavaScript side as a trap.
//
// This matters more under js/wasm than it would anywhere else: a Go panic that
// unwinds out of a js.Func aborts the whole wasm instance. Every subsequent
// call into the module then fails, so a single bad request permanently bricks
// the page until the user reloads.
//
// It is not a theoretical concern here, because qmc has three reachable
// panics, all of which this demo can drive from user input:
//
//   - NextInto and AtInto panic on a dst shorter than Dims(), rather than
//     silently truncating a point;
//   - scrambledRadicalInverse panics when an index has too many base-p digits
//     to reverse without overflowing the accumulator;
//   - primesUpTo panics when the sieve for the requested dimension count does
//     not fit in memory — and under GOARCH=wasm "memory" is a 32-bit address
//     space, so the ceiling is far lower than on a host.
//
// Every one of those is prevented upstream by the clamps in this package, but
// a clamp is a line of code someone can get wrong; the recover is the thing
// that keeps that mistake a red error box instead of a dead page.
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

		opts := js.Undefined()
		if len(args) > 0 {
			opts = args[0]
		}

		return fn(opts)
	})
}

// errorResult is the shape every export returns on a rejected request. It
// carries panic:false to distinguish a request this code refused from one that
// crashed it — the page reports the two differently, and it should, because a
// panic:true means the instance is now suspect and the page should offer a
// reload rather than let the user carry on clicking.
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

	return int(number)
}

func readFloat(opts js.Value, key string, fallback float64) float64 {
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

	return number
}

// readUint64 reads a scrambling seed.
//
// It goes through float64 because that is the only number JavaScript has: a
// seed typed into the page arrives here as a double, never as an integer, so
// reading it with readInt would truncate it against a 32-bit int under
// GOARCH=wasm and quietly hand the library a different seed than the one on
// screen. Negative values clamp to 0 rather than wrapping around to a huge
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

func readBool(opts js.Value, key string, fallback bool) bool {
	if !isObject(opts) {
		return fallback
	}

	value := opts.Get(key)
	if value.Type() != js.TypeBoolean {
		return fallback
	}

	return value.Bool()
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
