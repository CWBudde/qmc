package qmc

import (
	"fmt"
	"math"
	"testing"
)

func TestCenteredL2RejectsUnrepresentableArithmetic(t *testing.T) {
	for _, dims := range []int{2000, 6100, 7000, 9000} {
		t.Run(fmt.Sprint(dims), func(t *testing.T) {
			got, err := CenteredL2Discrepancy([][]float64{make([]float64, dims)})
			if err == nil || got != 0 {
				t.Fatalf("unsupported arithmetic returned %g, %v; want 0 and an error", got, err)
			}
		})
	}
}

func TestCenteredL2LargeRepresentableResult(t *testing.T) {
	const dims = 1000

	got, err := CenteredL2Discrepancy([][]float64{make([]float64, dims)})
	if err != nil {
		t.Fatal(err)
	}

	want := math.Sqrt(math.Pow(13.0/12, dims) - 2*math.Pow(9.0/8, dims) + math.Pow(3.0/2, dims))
	if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got/want-1) > 1e-12 {
		t.Fatalf("large finite result = %g, want %g", got, want)
	}
}

func TestDiscrepancyScratchSizeIsChecked(t *testing.T) {
	for _, shape := range [][2]int{{math.MaxInt, 2}, {math.MaxInt/8 + 1, 1}, {0, 1}, {1, 0}} {
		if _, err := discrepancyScratchLen(shape[0], shape[1]); err == nil {
			t.Errorf("scratch size %v was accepted", shape)
		}
	}

	if got, err := discrepancyScratchLen(3, 4); err != nil || got != 12 {
		t.Fatalf("valid scratch size = %d, %v; want 12, nil", got, err)
	}
}
