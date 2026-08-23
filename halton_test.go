package qmc

import (
	"math"
	"testing"
)

func TestRadicalInverseKnownValues(t *testing.T) {
	// Base 2: 1/2, 1/4, 3/4, 1/8, 5/8, 3/8, 7/8.
	base2 := []float64{0.5, 0.25, 0.75, 0.125, 0.625, 0.375, 0.875}
	for i, want := range base2 {
		if got := radicalInverse(i+1, 2); math.Abs(got-want) > 1e-12 {
			t.Fatalf("radicalInverse(%d, 2) = %v, want %v", i+1, got, want)
		}
	}
	// Base 3: 1/3, 2/3, 1/9, 4/9, 7/9.
	base3 := []float64{1.0 / 3, 2.0 / 3, 1.0 / 9, 4.0 / 9, 7.0 / 9}
	for i, want := range base3 {
		if got := radicalInverse(i+1, 3); math.Abs(got-want) > 1e-12 {
			t.Fatalf("radicalInverse(%d, 3) = %v, want %v", i+1, got, want)
		}
	}
}

func TestPrimesUpTo(t *testing.T) {
	want := []int{2, 3, 5, 7, 11, 13, 17, 19, 23, 29}
	got := primesUpTo(len(want))
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("primesUpTo(%d)[%d] = %d, want %d", len(want), i, got[i], want[i])
		}
	}
	// The 64th prime is 311 and the 1000th is 7919; both bounds exercise the
	// sieve's growth loop rather than a hand-checked table.
	if p := primesUpTo(64); p[63] != 311 {
		t.Fatalf("64th prime = %d, want 311", p[63])
	}
	if p := primesUpTo(1000); p[999] != 7919 {
		t.Fatalf("1000th prime = %d, want 7919", p[999])
	}
	if primesUpTo(0) != nil {
		t.Fatalf("primesUpTo(0) should be nil")
	}
}

func TestNewHaltonRejectsZeroDims(t *testing.T) {
	if _, err := NewHalton(0); err == nil {
		t.Fatalf("NewHalton(0) should fail")
	}
	if _, err := NewHalton(-1); err == nil {
		t.Fatalf("NewHalton(-1) should fail")
	}
}

func TestFirstPointIsTheClassicOne(t *testing.T) {
	g, err := NewHalton(5)
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{1.0 / 2, 1.0 / 3, 1.0 / 5, 1.0 / 7, 1.0 / 11}
	got := g.At(0)
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-12 {
			t.Fatalf("At(0)[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// legacyHaltonPoint is the hand-rolled generator this package replaced, kept
// verbatim so the unscrambled path can be proven bit-identical to it. Every
// sweep report recorded before the extraction was produced by this code.
func legacyHaltonPoint(index, dims int) []float64 {
	primes := []int{
		2, 3, 5, 7, 11, 13, 17, 19, 23, 29,
		31, 37, 41, 43, 47, 53, 59, 61, 67, 71,
		73, 79, 83, 89, 97, 101, 103, 107, 109, 113,
		127, 131, 137, 139, 149, 151, 157, 163, 167, 173,
		179, 181, 191, 193, 197, 199, 211, 223, 227, 229,
		233, 239, 241, 251, 257, 263, 269, 271, 277, 281,
		283, 293, 307, 311,
	}
	out := make([]float64, dims)
	for d := 0; d < dims; d++ {
		result, f := 0.0, 1.0/float64(primes[d])
		for i := index; i > 0; i /= primes[d] {
			result += float64(i%primes[d]) * f
			f /= float64(primes[d])
		}
		out[d] = result
	}
	return out
}

func TestUnscrambledIsBitIdenticalToTheLegacyGenerator(t *testing.T) {
	const (
		dims = 39
		skip = 64
	)
	g, err := NewHalton(dims, WithSkip(skip))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 600; i++ {
		want := legacyHaltonPoint(skip+i+1, dims)
		got := g.Next()
		for d := range want {
			if got[d] != want[d] {
				t.Fatalf("point %d dim %d: got %v, want %v (bit-identical required)", i, d, got[d], want[d])
			}
		}
	}
}

func TestCoordinatesStayInTheUnitInterval(t *testing.T) {
	for _, scramble := range []bool{false, true} {
		opts := []Option{WithSkip(64)}
		if scramble {
			opts = append(opts, WithScrambling(0xFFFFFFFFFFFFFFFF))
		}
		g, err := NewHalton(39, opts...)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2000; i++ {
			for d, v := range g.At(i) {
				if v < 0 || v >= 1 {
					t.Fatalf("scramble=%v point %d dim %d = %v, want [0,1)", scramble, i, d, v)
				}
			}
		}
	}
}

func TestScramblingIsDeterministicPerSeed(t *testing.T) {
	build := func(seed uint64) *Halton {
		g, err := NewHalton(20, WithSkip(64), WithScrambling(seed))
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	a, b, c := build(7), build(7), build(8)
	same, differs := 0, 0
	for i := 0; i < 200; i++ {
		pa, pb, pc := a.At(i), b.At(i), c.At(i)
		for d := range pa {
			if pa[d] != pb[d] {
				t.Fatalf("same seed disagrees at point %d dim %d: %v vs %v", i, d, pa[d], pb[d])
			}
			same++
			if pa[d] != pc[d] {
				differs++
			}
		}
	}
	// A different seed must actually produce a different sequence. Some
	// coordinates coincide by chance in small bases, so this asserts the bulk.
	if differs < same/2 {
		t.Fatalf("seeds 7 and 8 agree on %d of %d coordinates; scrambling is not seed-dependent", same-differs, same)
	}
}

func TestNextMatchesAtAndResetRewinds(t *testing.T) {
	g, err := NewHalton(9, WithSkip(13), WithScrambling(42))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		got, want := g.Next(), g.At(i)
		for d := range want {
			if got[d] != want[d] {
				t.Fatalf("Next() point %d dim %d = %v, want At(%d) = %v", i, d, got[d], i, want[d])
			}
		}
	}
	g.Reset()
	first, want := g.Next(), g.At(0)
	for d := range want {
		if first[d] != want[d] {
			t.Fatalf("after Reset, Next()[%d] = %v, want %v", d, first[d], want[d])
		}
	}
}

func TestSkipShiftsTheSequence(t *testing.T) {
	plain, err := NewHalton(5)
	if err != nil {
		t.Fatal(err)
	}
	skipped, err := NewHalton(5, WithSkip(64))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		got, want := skipped.At(i), plain.At(64+i)
		for d := range want {
			if got[d] != want[d] {
				t.Fatalf("skip(64).At(%d)[%d] = %v, want plain.At(%d)[%d] = %v", i, d, got[d], 64+i, d, want[d])
			}
		}
	}
}

func TestNegativeSkipIsClamped(t *testing.T) {
	var s settings
	WithSkip(-5)(&s)
	if s.skip != 0 {
		t.Fatalf("WithSkip(-5) stored %d, want 0", s.skip)
	}
}
