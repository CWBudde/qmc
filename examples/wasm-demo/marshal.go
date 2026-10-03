//go:build js && wasm

package main

import (
	"fmt"
	"syscall/js"
	"unsafe"
)

// The obvious way to hand a Go slice to JavaScript from wasm is a SetIndex
// loop. It costs one JS boundary crossing per element, and this demo moves a
// lot of elements: the scatter plot is up to 20,000 points, which is 40,000
// coordinates, and a 48-dimensional correlation matrix is 2,304 cells that the
// page redraws on every slider drag. One js.CopyBytesToJS per array replaces
// all of those crossings with a single memcpy.
//
// Why the buffer must be JS-owned: Go's wasm heap can grow, and growing the
// WebAssembly.Memory detaches every typed array that was created over its
// buffer. A view the page cached over Go memory would therefore be silently
// invalidated by some later allocation deep inside the library — the symptom
// is an empty canvas after a parameter change that happened to allocate. A
// view over a JS-allocated ArrayBuffer is unaffected, so the page can hold its
// views across calls and reuse them frame after frame.
type float32Sink struct {
	f32      js.Value
	u8       js.Value
	capacity int // in float32 elements
}

// newFloat32Sink allocates a fresh JS-side buffer of n float32 elements.
func newFloat32Sink(n int) float32Sink {
	if n < 0 {
		n = 0
	}

	buffer := js.Global().Get("ArrayBuffer").New(n * 4)

	return float32Sink{
		f32:      js.Global().Get("Float32Array").New(buffer),
		u8:       js.Global().Get("Uint8Array").New(buffer),
		capacity: n,
	}
}

// sinkFor reuses matching, attached Float32Array/Uint8Array views over the same
// ordinary ArrayBuffer at the same byte offset, with room for the payload.
// Invalid, shared, detached, and undersized pairs are replaced. The page passes
// opts.out = {xy: {f32, u8}, ...} so
// that dragging a slider re-renders without allocating a new buffer per frame;
// a first call, or one that raised the point count, silently gets a new one.
func sinkFor(out js.Value, key string, n int) float32Sink {
	if isObject(out) {
		candidate := out.Get(key)
		if isObject(candidate) {
			f32 := candidate.Get("f32")
			u8 := candidate.Get("u8")

			if validFloat32Pair(f32, u8, n) {
				return float32Sink{f32: f32, u8: u8, capacity: f32.Length()}
			}
		}
	}

	return newFloat32Sink(n)
}

func validFloat32Pair(f32, u8 js.Value, n int) bool {
	arrayBuffer := js.Global().Get("ArrayBuffer")

	isView := arrayBuffer.Get("isView")
	if !isView.Invoke(f32).Bool() || !isView.Invoke(u8).Bool() ||
		!f32.InstanceOf(js.Global().Get("Float32Array")) ||
		!u8.InstanceOf(js.Global().Get("Uint8Array")) {
		return false
	}

	buffer := f32.Get("buffer")
	if !buffer.InstanceOf(arrayBuffer) || !buffer.Equal(u8.Get("buffer")) ||
		f32.Get("byteOffset").Int() != u8.Get("byteOffset").Int() {
		return false
	}

	// A detached view has zero capacity. SharedArrayBuffer is intentionally
	// excluded above: another agent must not mutate this output during a copy.
	return n >= 0 && f32.Length() >= n && u8.Get("byteLength").Int() >= n*4
}

// write copies data into the sink and returns the JS view to hand back. When
// the sink is larger than the payload (a reused buffer), the returned view is
// a subarray of exactly the right length, so the page never has to track how
// much of the buffer is live — reading result.xy.length is always correct.
func (s float32Sink) write(data []float32) js.Value {
	if len(data) > s.capacity {
		panic("float32 output exceeds sink capacity")
	}

	if len(data) > 0 {
		payload := float32Bytes(data)
		if copied := js.CopyBytesToJS(s.u8, payload); copied != len(payload) {
			panic(fmt.Sprintf("float32 output: copied %d of %d bytes", copied, len(payload)))
		}
	}

	if s.capacity == len(data) {
		return s.f32
	}

	return s.f32.Call("subarray", 0, len(data))
}

// float32Bytes reinterprets a []float32 as the []byte CopyBytesToJS wants.
// This is a view, not a copy: the whole point of the exercise is to move the
// payload once. js/wasm is little-endian and so is every Float32Array, so the
// byte order needs no fixing up.
func float32Bytes(data []float32) []byte {
	if len(data) == 0 {
		return nil
	}

	return unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), len(data)*4)
}

// putFloats writes one named float32 array into result, reusing a caller-
// supplied view when one fits.
func putFloats(result map[string]any, out js.Value, key string, data []float32) {
	result[key] = sinkFor(out, key, len(data)).write(data)
}
