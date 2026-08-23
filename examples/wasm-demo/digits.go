//go:build js && wasm

package main

import (
	"syscall/js"
)

// jsDigits opens up a single coordinate: the base-p digits of the index behind
// it, what scrambling rewrites each of them to, and the two values that come
// out either way.
//
// This is the part of the demo that explains the rest. The heatmap shows that
// high dimensions correlate and that scrambling fixes it; only the digit view
// shows why. At index 0 in base 167 the expansion is a single digit, so the
// unscrambled coordinate is that digit over 167 — the ramp, visible as a
// number — while the scrambled one is the permuted digit plus the contribution
// of the infinitely many leading zeros, which is the term that lifts short
// indices off the coarse lattice.
func jsDigits(opts js.Value) any {
	var (
		index    = clampInt(readInt(opts, "index", 0), 0, maxIndex)
		dim      = clampInt(readInt(opts, "dim", defaultAxisY), 0, maxDims-1)
		skip     = clampInt(readInt(opts, "skip", defaultSkip), 0, maxSkip)
		scramble = readBool(opts, "scramble", true)
		seed     = readUint64(opts, "seed", defaultSeed)
	)

	// dim+1 dimensions, not maxDims: the generator only has to reach the
	// dimension being inspected, and its bases and permutations are the same
	// either way. newPermutation mixes the dimension into the stream seed
	// precisely so that a generator built for 5 dimensions and one built for
	// 39 agree on the permutations of their first 5, so a 38-dimension
	// inspector shows the same permutation the 39-dimension scatter plot used.
	generator, err := newGenerator(dim+1, skip, scramble, seed)
	if err != nil {
		return errorResult("digits: %v", err)
	}

	plain, err := newGenerator(dim+1, skip, false, 0)
	if err != nil {
		return errorResult("digits: %v", err)
	}

	base := generator.Bases()[dim]

	// The index the digits are actually of. At(i) maps point i onto raw Halton
	// index skip+1+i — the +1 because index 0 is the degenerate origin, all
	// zeros before scrambling, which no caller wants back. Both numbers are
	// surfaced because the page has to explain the offset: a user who types 0
	// and sees the digits of 65 needs to be told where the 65 came from.
	rawIndex := skip + 1 + index

	digits := baseDigits(rawIndex, base)

	var permuted, permutation []any

	if perm := generator.Permutation(dim); perm != nil {
		permutation = int32sToJS(perm)

		permuted = make([]any, len(digits))
		for i, digit := range digits {
			permuted[i] = int(perm[digit])
		}
	}

	return map[string]any{
		"index":    index,
		"rawIndex": rawIndex,
		"dim":      dim,
		"base":     base,

		// LEAST SIGNIFICANT FIRST, for both digits and permuted: entry k is
		// the coefficient of base^k in rawIndex, so the radical inverse is
		// sum_k digits[k] * base^-(k+1) and the mirror the page draws reads
		// straight off the array in order. Getting this end backwards costs
		// nothing at run time and renders a silently wrong mirror — the value
		// would still look like a plausible number in [0,1).
		"digits":      intsToJS(digits),
		"permuted":    permuted,
		"permutation": permutation,

		// The coordinate the generator really produces, taken from the library
		// rather than recomputed from the digits above, so the two cannot
		// drift. In particular the scrambled value includes the leading-zero
		// tail, which no finite digit list shows.
		"value":            jsNumber(generator.At(index)[dim]),
		"unscrambledValue": jsNumber(plain.At(index)[dim]),
	}
}

// baseDigits returns the base-b expansion of value, least significant digit
// first. Zero has the single digit 0 rather than an empty list, so the page
// always has something to render.
func baseDigits(value, base int) []int {
	if base < 2 || value <= 0 {
		return []int{0}
	}

	out := make([]int, 0, 8)
	for i := value; i > 0; i /= base {
		out = append(out, i%base)
	}

	return out
}
