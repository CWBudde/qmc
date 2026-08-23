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

// TestUnscrambledStaysBelowOne covers the indices where the plain radical
// inverse rounds up. The true value is 1 - base^-m, so an index whose digits
// are all maximal reaches 1 once base^-m falls under an ulp; base 167 even
// overshoots to 1.0000000000000002. Nothing reaches these indices in practice,
// but [0,1) is what the package promises.
func TestUnscrambledStaysBelowOne(t *testing.T) {
	cases := []struct {
		index int
		base  int
	}{
		{2384185791015624, 5},
		{45949729863572160, 11},
		{604967116961135040, 167},
	}
	for _, tc := range cases {
		got := radicalInverse(tc.index, tc.base)
		if got < 0 || got >= 1 {
			t.Fatalf("radicalInverse(%d, %d) = %v, want [0,1)", tc.index, tc.base, got)
		}

		if got != oneMinusEpsilon {
			t.Fatalf("radicalInverse(%d, %d) = %v, want the clamp %v", tc.index, tc.base, got, oneMinusEpsilon)
		}
	}
}

// TestScrambledRefusesToAlias pins that an index too long to reverse is
// refused rather than folded onto a shorter one.
//
// Truncating the reversal used to return the exactly-correct value of a
// different index: at base 48611, index 5583907571905733386 produced the same
// point as index 12345. Two far-apart indices aliasing onto one point is the
// kind of defect a low-discrepancy sequence exists to avoid, and it left no
// trace.
func TestScrambledRefusesToAlias(t *testing.T) {
	const base = 48611

	perm := newPermutation(base, 1, 0)
	if got := scrambledRadicalInverse(12345, base, perm); got < 0 || got >= 1 {
		t.Fatalf("a reachable index must still work, got %v", got)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("an index too long to reverse must be refused, not aliased onto a shorter one")
		}
	}()

	scrambledRadicalInverse(5583907571905733386, base, perm)
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
