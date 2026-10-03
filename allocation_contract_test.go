package qmc_test

import (
	"fmt"
	"testing"

	"github.com/cwbudde/qmc"
)

func TestNestedHaltonScratchAllocationThreshold(t *testing.T) {
	for _, tc := range []struct{ dims, allocations int }{{97, 0}, {98, 1}, {100, 3}} {
		t.Run(fmt.Sprint(tc.dims), func(t *testing.T) {
			g, err := qmc.NewHalton(tc.dims, qmc.WithNestedScrambling(17))
			if err != nil {
				t.Fatal(err)
			}

			dst := make([]float64, tc.dims)
			if got := testing.AllocsPerRun(20, func() { g.AtInto(13, dst) }); got != float64(tc.allocations) {
				t.Errorf("AtInto allocations = %g, want %d", got, tc.allocations)
			}

			if got := testing.AllocsPerRun(20, func() { g.NextInto(dst) }); got != float64(tc.allocations) {
				t.Errorf("NextInto allocations = %g, want %d", got, tc.allocations)
			}
		})
	}
}

func BenchmarkNestedHaltonScratchThreshold(b *testing.B) {
	for _, dims := range []int{97, 98, 100} {
		b.Run(fmt.Sprint(dims), func(b *testing.B) {
			g, err := qmc.NewHalton(dims, qmc.WithNestedScrambling(17))
			if err != nil {
				b.Fatal(err)
			}

			dst := make([]float64, dims)

			b.ReportAllocs()
			b.ResetTimer()

			for i := range b.N {
				g.AtInto(i, dst)
			}
		})
	}
}
