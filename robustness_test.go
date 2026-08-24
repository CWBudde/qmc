package qmc

import (
	"math"
	"testing"
)

// TestShortDestinationPanics pins that a too-short dst is refused rather than
// partly filled. A truncated point looks like a plausible position — the tail
// coordinates hold zeros or, on a reused buffer, the previous point's values —
// and would steer a search with nothing to show for it. NextInto also advances
// the cursor, so absorbing the mistake would consume a sequence point too.
func TestShortDestinationPanics(t *testing.T) {
	h, err := NewHalton(5)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		call func()
	}{
		{"NextInto", func() { h.NextInto(make([]float64, 2)) }},
		{"AtInto", func() { h.AtInto(0, nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("a short destination must panic, not be silently truncated")
				}
			}()

			tc.call()
		})
	}
}

// TestSieveIsArchitectureIndependent pins the primes around the 32-bit
// wrap point. i = 92683 squares to 8590138489, which mod 2^32 is 203897 — a
// positive value that passed the old `j > 0` guard and marked a slot it had no
// business marking, dropping the prime 203897 on 32-bit builds. Bases that
// depend on GOARCH would make the whole sequence depend on it.
func TestSieveIsArchitectureIndependent(t *testing.T) {
	got := sieve(203898)
	if n := len(got); n != 18294 {
		t.Fatalf("sieve(203898) returned %d primes, want 18294", n)
	}

	if last := got[len(got)-1]; last != 203897 {
		t.Fatalf("largest prime below 203898 = %d, want 203897", last)
	}
}

// TestScrambledMatchesTheDigitSumDefinition brute-forces the closed form
// against the definition it is an optimisation of: the sum of permuted digits
// over their place values, plus the geometric tail for the infinitely many
// leading zeros.
func TestScrambledMatchesTheDigitSumDefinition(t *testing.T) {
	for _, base := range []int{2, 3, 5, 7, 11, 13, 17, 167} {
		perm := newPermutation(base, 12345, 3)
		for index := 0; index < 4000; index++ {
			got := scrambledRadicalInverse(index, base, perm)

			want := digitSumReference(index, base, perm)
			if math.Abs(got-want) > 1e-12 {
				t.Fatalf("base %d index %d: got %v, want %v", base, index, got, want)
			}
		}
	}
}

// digitSumReference is the slow, obvious form of the same definition.
func digitSumReference(index, base int, perm []int32) float64 {
	invBase := 1 / float64(base)

	place := invBase
	sum := 0.0
	i := index

	for ; i > 0; i /= base {
		sum += float64(perm[i%base]) * place
		place *= invBase
	}
	// Every remaining digit is 0 and maps to perm[0]; that tail is geometric.
	sum += float64(perm[0]) * place / (1 - invBase)

	if sum >= oneMinusEpsilon {
		return oneMinusEpsilon
	}

	return sum
}

// TestOneMinusEpsilonIsNextafterOne pins the hex float literal in halton.go
// against the call it replaced. The literal has to be written out because
// math.Nextafter is not a constant expression, and a hand-written bit pattern
// is exactly the kind of thing that survives an edit with one f too few: it
// would still be a plausible number just below 1, every clamp would still
// clamp, and the only symptom would be coordinates landing a few ulps short of
// the boundary the package documents.
func TestOneMinusEpsilonIsNextafterOne(t *testing.T) {
	// Typed uint64, not an untyped constant: %#016x below passes it through
	// an interface{}, where an untyped constant defaults to int and overflows
	// the build on a 32-bit GOARCH.
	const wantBits uint64 = 0x3FEFFFFFFFFFFFFF

	if got := math.Float64bits(oneMinusEpsilon); got != wantBits {
		t.Fatalf("math.Float64bits(oneMinusEpsilon) = %#016x, want %#016x", got, wantBits)
	}

	if want := math.Nextafter(1, 0); oneMinusEpsilon != want {
		t.Fatalf("oneMinusEpsilon = %v, want math.Nextafter(1, 0) = %v", oneMinusEpsilon, want)
	}

	if oneMinusEpsilon >= 1 {
		t.Fatalf("oneMinusEpsilon = %v must be strictly below 1", oneMinusEpsilon)
	}
}
