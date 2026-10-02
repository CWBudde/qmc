package qmc_test

import (
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/cwbudde/qmc"
)

func TestCenteredL2MidpointGridHasStablePrecision(t *testing.T) {
	for _, n := range []int{1, 3, 16, 4096, 16384} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			points := make([][]float64, n)
			for i := range points {
				points[i] = []float64{(float64(i) + 0.5) / float64(n)}
			}

			got, err := qmc.CenteredL2Discrepancy(points)
			if err != nil {
				t.Fatal(err)
			}

			want := 1 / (math.Sqrt(12) * float64(n))
			if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got/want-1) > 8e-15 {
				t.Fatalf("N=%d: CD2=%.17g, want %.17g; relative error %.3g", n, got, want, got/want-1)
			}
		})
	}
}

func TestStarSinglePointAvoidsGenericWorkGate(t *testing.T) {
	point := make([]float64, 100)
	for i := range point {
		point[i] = 0.99
	}

	got, err := qmc.StarDiscrepancy([][]float64{point})
	if err != nil {
		t.Fatal(err)
	}
	// max(max x_k, 1 - product x_k) = max(0.99, ~0.634) = 0.99.
	if got != 0.99 {
		t.Fatalf("one-point star discrepancy = %g, want 0.99", got)
	}
}

func TestCenteredL2TensorMidpointGridMatchesRationalReference(t *testing.T) {
	// In an even-q midpoint grid the average single-factor is
	// A=13/12+1/(24q²), and the average pair-factor is B=13/12+1/(6q²).
	// The full 2D tensor grid therefore has CD2²=(13/12)²-2A²+B².
	// Evaluate the reference exactly as a rational before rounding its root.
	for _, q := range []int{4, 8, 16} {
		points := make([][]float64, 0, q*q)
		for i := range q {
			for j := range q {
				points = append(points, []float64{(float64(i) + 0.5) / float64(q), (float64(j) + 0.5) / float64(q)})
			}
		}

		q2 := int64(q * q)
		a := big.NewRat(26*q2+1, 24*q2)
		b := big.NewRat(13*q2+2, 12*q2)
		c := big.NewRat(13, 12)
		square := new(big.Rat).Mul(c, c)
		aa := new(big.Rat).Mul(a, a)
		aa.Mul(aa, big.NewRat(2, 1))
		square.Sub(square, aa)
		square.Add(square, new(big.Rat).Mul(b, b))
		value, _ := square.Float64()
		want := math.Sqrt(value)

		got, err := qmc.CenteredL2Discrepancy(points)
		if err != nil {
			t.Fatal(err)
		}

		if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got/want-1) > 1e-11 {
			t.Fatalf("%dx%d grid: CD2=%g, rational reference=%g", q, q, got, want)
		}
	}
}
