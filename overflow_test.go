package qmc

import (
	"math"
	"testing"
)

// The raw Halton index is skip+1+i, and both halves are caller-supplied. With a
// large skip that sum used to wrap negative without a word: radicalInverse's
// index < 0 guard then returned 0 for every coordinate, so At handed back the
// all-zeros origin — the exact point its doc comment promises is never
// returned. The scrambled path did not agree either; it panicked deep inside
// the digit reversal. These tests pin the overflow to a single, clearly
// labelled refusal on both paths.
//
// math.MaxInt is used rather than a literal so the file also compiles where int
// is 32 bits wide; CI runs a GOARCH=386 leg.

func TestFillRefusesOverflowingIndex(t *testing.T) {
	g, err := NewHalton(3, WithSkip(math.MaxInt-1))
	if err != nil {
		t.Fatalf("NewHalton: %v", err)
	}

	// i == 0 still fits: skip+1+0 == MaxInt.
	if got := g.At(0); got[0] == 0 && got[1] == 0 && got[2] == 0 {
		t.Fatalf("the last representable index must be a real point, got %v", got)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("an index that overflows skip+1+i must be refused, not silently returned as the origin")
		}
	}()

	g.At(5)
}

func TestFillRefusesOverflowingIndexScrambled(t *testing.T) {
	g, err := NewHalton(3, WithSkip(math.MaxInt-1), WithScrambling(7))
	if err != nil {
		t.Fatalf("NewHalton: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("the scrambled path must refuse an overflowing index too")
		}
	}()

	g.At(5)
}

// TestFillOverflowRefusalIsIndependentOfPath pins that the two paths refuse the
// same set of indices. They disagreed before: unscrambled returned zeros where
// scrambled panicked, so which failure a caller saw depended on an option that
// has nothing to do with index arithmetic.
func TestFillOverflowRefusalIsIndependentOfPath(t *testing.T) {
	cases := []struct {
		name string
		opts []Option
	}{
		{"unscrambled", nil},
		{"scrambled", []Option{WithScrambling(7)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := append([]Option{WithSkip(math.MaxInt / 2)}, tc.opts...)

			g, err := NewHalton(2, opts...)
			if err != nil {
				t.Fatalf("NewHalton: %v", err)
			}

			// Comfortably inside the range: must produce a point.
			if got := g.At(1000); got[0] <= 0 || got[0] >= 1 || got[1] <= 0 || got[1] >= 1 {
				t.Fatalf("a representable index must yield a point in (0,1), got %v", got)
			}

			defer func() {
				if recover() == nil {
					t.Fatal("an overflowing index must panic")
				}
			}()

			g.At(math.MaxInt/2 + 1)
		})
	}
}

// TestNextIntoRefusesOverflowingCursor covers the stateful path: the cursor
// walks into the same overflow, and it must stop there rather than start
// emitting the origin over and over.
func TestNextIntoRefusesOverflowingCursor(t *testing.T) {
	g, err := NewHalton(2, WithSkip(math.MaxInt-1))
	if err != nil {
		t.Fatalf("NewHalton: %v", err)
	}

	dst := make([]float64, 2)

	g.NextInto(dst) // cursor 0 -> index MaxInt, still fine

	defer func() {
		if recover() == nil {
			t.Fatal("the cursor walking past the representable index must be refused")
		}
	}()

	g.NextInto(dst)
}
