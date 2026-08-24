package qmc

import (
	"math"
	"testing"
)

// The fuzz targets in this file pin the two production radical inverses in
// halton.go against the slow, obvious form of the same definition —
// digitSumReference in robustness_test.go, which sums permuted digits over
// their place values and closes the leading-zero tail as a geometric series.
//
// The reference is reused rather than rewritten: a second hand-rolled
// reference would be a second thing that can be wrong in the same way as the
// code it checks, and the existing one is already the definition the
// package's own table-driven test is written against.

// fuzzBases is the base table the targets draw from: the same primes the
// generator itself uses, so a fuzz input can never name a base no Halton
// dimension would ever see. 64 is the dimension ceiling the WebAssembly demo
// clamps to and is far past the point where the failure modes differ.
var fuzzBases = primesUpTo(64)

// baseFor maps an arbitrary fuzz int onto a valid base rather than rejecting
// it. Rejecting would throw away almost every input the fuzzer generates and
// leave the targets exercising a handful of hand-written cases at fuzzer
// speed, which is the worst of both approaches.
func baseFor(raw int) int {
	return fuzzBases[int(uint(raw)%uint(len(fuzzBases)))]
}

// safeIndexLimit is the largest index whose base-b digit reversal cannot trip
// the overflow guard in scrambledRadicalInverse.
//
// The guard is deliberately a panic, not a clamp — see the comment there — so
// a fuzz target that fed it an out-of-range index would be reporting the
// library working as designed as a crash. The bound is derived rather than
// guessed: after k reversal steps the accumulator is at most base^k - 1, the
// guard demands it stay at or below limit at the start of every step, so an
// index of m digits is safe exactly when base^(m-1) <= limit+1.
//
// It is not a small number — 2.8e17 at base 311, and the whole int range at
// base 2 — so the interesting territory is still in range: the point where
// base^-m falls below an ulp and radicalInverse's clamp to oneMinusEpsilon
// starts to bite sits well inside it.
func safeIndexLimit(base int) int {
	limit := (^uint64(0)-uint64(base-1))/uint64(base) + 1

	// power tracks base^(m-1); digits tracks m.
	power := uint64(1)
	maxIndex := uint64(0)

	for {
		next := power * uint64(base)
		if next/uint64(base) != power || next-1 > uint64(math.MaxInt) {
			return math.MaxInt // base^m no longer fits; every int index is safe
		}

		maxIndex = next - 1

		if next > limit {
			return int(maxIndex)
		}

		power = next
	}
}

// indexFor folds an arbitrary fuzz value into [0, safeIndexLimit(base)].
func indexFor(raw int64, base int) int {
	span := uint64(safeIndexLimit(base)) + 1

	return int(uint64(raw) % span)
}

// fuzzTolerance is absolute, and it is loose by about two orders of magnitude
// on purpose.
//
// Both values live in [0,1), and both are built from at most a few dozen
// multiply-accumulate steps whose rounding is bounded by an ulp of 1 each, so
// the true disagreement between the closed form and the digit sum cannot
// exceed roughly 1e-14. The closed form additionally rounds float64(reversed)
// once when the reversal exceeds 2^53, which costs another ulp of the result.
// 1e-12 is comfortably above all of that and still far below any difference
// that would mean the two forms had actually diverged — a dropped digit or a
// missing tail term moves the result by at least 1/base.
const fuzzTolerance = 1e-12

// FuzzRadicalInverse checks the plain radical inverse against the digit-sum
// definition with the identity permutation, for which the geometric tail term
// is exactly zero and digitSumReference degenerates to the textbook formula.
func FuzzRadicalInverse(f *testing.F) {
	seeds := []int64{
		0, 1, 2, 3, 7, 63, 64, 65, 166, 167, 168,
		1 << 20, 1 << 40,
		// Around where base^-m drops below an ulp and the clamp to
		// oneMinusEpsilon becomes reachable.
		1 << 53, 1<<53 + 1, 6e17, math.MaxInt64,
		// Negative inputs fold to a valid index rather than being rejected.
		-1, math.MinInt64,
	}

	for _, index := range seeds {
		for _, base := range []int{0, 1, 2, 5, 38, 63} {
			f.Add(index, base)
		}
	}

	f.Fuzz(func(t *testing.T, rawIndex int64, rawBase int) {
		base := baseFor(rawBase)
		index := indexFor(rawIndex, base)

		identity := make([]int32, base)
		for d := range identity {
			identity[d] = int32(d)
		}

		got := radicalInverse(index, base)

		if got < 0 || got >= 1 {
			t.Fatalf("radicalInverse(%d, %d) = %v, outside [0,1)", index, base, got)
		}

		want := digitSumReference(index, base, identity)
		if math.Abs(got-want) > fuzzTolerance {
			t.Fatalf("radicalInverse(%d, %d) = %v, reference %v (delta %v)",
				index, base, got, want, math.Abs(got-want))
		}
	})
}

// FuzzScrambledRadicalInverse checks the integer-reversal closed form against
// the same reference under a real permutation, which is where the
// leading-zero tail is non-zero and therefore load-bearing.
func FuzzScrambledRadicalInverse(f *testing.F) {
	seeds := []int64{
		0, 1, 2, 3, 7, 63, 64, 65, 166, 167, 168,
		1 << 20, 1 << 40, 1 << 53, 1<<53 + 1, 6e17, math.MaxInt64,
		-1, math.MinInt64,
	}

	for _, index := range seeds {
		for _, base := range []int{0, 1, 2, 5, 38, 63} {
			f.Add(index, base, uint64(12345), 3)
		}
	}

	f.Fuzz(func(t *testing.T, rawIndex int64, rawBase int, seed uint64, rawDim int) {
		base := baseFor(rawBase)
		index := indexFor(rawIndex, base)
		dim := int(uint(rawDim) % 64)

		perm := newPermutation(base, seed, dim)

		got := scrambledRadicalInverse(index, base, perm)

		if got < 0 || got >= 1 {
			t.Fatalf("scrambledRadicalInverse(%d, %d) = %v, outside [0,1)", index, base, got)
		}

		want := digitSumReference(index, base, perm)
		if math.Abs(got-want) > fuzzTolerance {
			t.Fatalf("scrambledRadicalInverse(%d, %d) = %v, reference %v (delta %v)",
				index, base, got, want, math.Abs(got-want))
		}
	})
}
