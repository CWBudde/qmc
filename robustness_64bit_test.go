//go:build amd64 || arm64 || ppc64 || ppc64le || mips64 || mips64le || riscv64 || s390x || loong64 || wasm

package qmc

import "testing"

// These cases need indices that do not fit in a 32-bit int, so they can only be
// compiled where int is 64 bits wide. Keeping them in the main test file made
// the whole package fail to build under GOARCH=386 — which in turn made it
// impossible to run TestSieveIsArchitectureIndependent on the only kind of
// target whose behaviour it is about.

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
