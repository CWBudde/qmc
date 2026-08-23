//go:build amd64 || arm64 || ppc64 || ppc64le || mips64 || mips64le || riscv64 || s390x || loong64 || wasm

package qmc

import "testing"

// A leaping Sobol generator reaches the 32-bit ceiling of the direction
// numbers leap times sooner than an unleaped one, and it reaches it through a
// different branch: the leap path in NextInto keeps its own point cursor and
// never touches the Gray-code recurrence, so the counter == MaxUint32 panic
// that stops an unleaped run is not what stops this one. The guards in fill
// are, and these tests are what say so.
//
// The indices involved do not fit a 32-bit int, which is why they live here
// rather than in leap_test.go. See the note at the top of
// robustness_64bit_test.go.

func TestLeapedSobolRefusesAnIndexPastTheDirectionNumbers(t *testing.T) {
	const leap = 173

	g, err := NewSobol(2, WithLeap(leap))
	if err != nil {
		t.Fatal(err)
	}

	// Point i is raw index 1+i*leap, so the last representable point is the
	// largest i with 1+i*leap < 2^32.
	last := ((1 << 32) - 2) / leap

	if got := g.At(last); got[0] < 0 || got[0] >= 1 {
		t.Fatalf("the last representable index must yield a point in [0,1), got %v", got)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("a leaped index past the 32-bit direction numbers must be refused, not aliased onto index 0")
		}
	}()

	g.At(last + 1)
}

func TestLeapedSobolNextIntoRefusesAnOverrunningCursor(t *testing.T) {
	// A skip that leaves exactly one point below the ceiling, so the cursor
	// walks off it on the second call rather than after two billion.
	g, err := NewSobol(2, WithSkip((1<<32)-4), WithLeap(3))
	if err != nil {
		t.Fatal(err)
	}

	dst := make([]float64, 2)

	g.NextInto(dst) // raw (1<<32)-3, the last one that fits

	defer func() {
		if recover() == nil {
			t.Fatal("a leaping cursor walking past the 32-bit index must be refused, not wrapped onto index 0")
		}
	}()

	g.NextInto(dst) // raw 1<<32, gone
}
