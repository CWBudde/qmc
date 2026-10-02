package qmc_test

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/cwbudde/qmc"
)

func expectIndexPanic(t *testing.T, call func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Error("access beyond the raw-index range must panic")
		}
	}()

	call()
}

func TestConstructorsRejectUnrepresentableFirstIndex(t *testing.T) {
	for _, leap := range []int{1, 3, math.MaxInt} {
		t.Run(fmt.Sprint(leap), func(t *testing.T) {
			opts := []qmc.Option{qmc.WithSkip(math.MaxInt), qmc.WithLeap(leap)}
			if _, err := qmc.NewHalton(1, opts...); err == nil {
				t.Error("Halton accepted skip=MaxInt")
			}

			if _, err := qmc.NewSobol(1, opts...); err == nil {
				t.Error("Sobol accepted skip=MaxInt")
			}
		})
	}
}

func TestLastRawIndexAgreesAcrossAccessMethods(t *testing.T) {
	cases := []struct {
		name string
		max  uint64
		new  func(...qmc.Option) (qmc.Sequence, error)
		opts []qmc.Option
	}{
		{"halton", uint64(math.MaxInt), func(o ...qmc.Option) (qmc.Sequence, error) { return qmc.NewHalton(1, o...) }, nil},
		{"halton/digit", uint64(math.MaxInt), func(o ...qmc.Option) (qmc.Sequence, error) { return qmc.NewHalton(1, o...) }, []qmc.Option{qmc.WithScrambling(7)}},
		{"halton/nested", uint64(math.MaxInt), func(o ...qmc.Option) (qmc.Sequence, error) { return qmc.NewHalton(1, o...) }, []qmc.Option{qmc.WithNestedScrambling(7)}},
		{"sobol", min(uint64(math.MaxInt), uint64(math.MaxUint32)), func(o ...qmc.Option) (qmc.Sequence, error) { return qmc.NewSobol(1, o...) }, nil},
		{"sobol/shift", min(uint64(math.MaxInt), uint64(math.MaxUint32)), func(o ...qmc.Option) (qmc.Sequence, error) { return qmc.NewSobol(1, o...) }, []qmc.Option{qmc.WithDigitalShift(7)}},
		{"sobol/owen", min(uint64(math.MaxInt), uint64(math.MaxUint32)), func(o ...qmc.Option) (qmc.Sequence, error) { return qmc.NewSobol(1, o...) }, []qmc.Option{qmc.WithOwenScrambling(7)}},
	}
	for _, tc := range cases {
		for _, leap := range []int{1, 3} {
			t.Run(fmt.Sprintf("%s/leap%d", tc.name, leap), func(t *testing.T) {
				// Three points ending exactly on the last admissible raw index.
				skip := int(tc.max) - 1 - 2*leap
				opts := append([]qmc.Option{qmc.WithSkip(skip), qmc.WithLeap(leap)}, tc.opts...)

				g, err := tc.new(opts...)
				if err != nil {
					t.Fatal(err)
				}

				for i := range 3 {
					want := g.At(i)
					into := make([]float64, g.Dims())
					g.AtInto(i, into)

					if got := g.Next(); !reflect.DeepEqual(got, want) || !reflect.DeepEqual(into, want) {
						t.Fatalf("point %d: Next=%v, AtInto=%v, At=%v", i, got, into, want)
					}
				}

				expectIndexPanic(t, func() { g.At(3) })
				expectIndexPanic(t, func() { g.AtInto(3, make([]float64, g.Dims())) })
				expectIndexPanic(t, func() { g.Next() })
				expectIndexPanic(t, func() { g.NextInto(make([]float64, g.Dims())) })
				g.Reset()

				if got, want := g.Next(), g.At(0); !reflect.DeepEqual(got, want) {
					t.Fatalf("reset: Next=%v, At(0)=%v", got, want)
				}
			})
		}
	}
}
