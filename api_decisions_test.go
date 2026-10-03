package qmc_test

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/cwbudde/qmc"
)

// Check later aligned blocks, not only the first non-origin block. This also
// exercises the last complete block on both int widths without allocating a
// large block or depending on Gray order being the identity on its high bits.
func TestSobolLaterAlignedBlocks(t *testing.T) {
	for _, m := range []uint{4, 8} {
		n := 1 << m

		lastBlock := (min(uint64(math.MaxInt), uint64(math.MaxUint32))+1)/uint64(n) - 1
		for _, block := range []uint64{1, 2, 3, 17, lastBlock} {
			for _, randomization := range []struct {
				name   string
				option qmc.Option
			}{
				{"plain", nil}, {"shift", qmc.WithDigitalShift(7)}, {"Owen", qmc.WithOwenScrambling(7)},
			} {
				t.Run(fmt.Sprintf("m=%d/block=%d/%s", m, block, randomization.name), func(t *testing.T) {
					skip := int(block*uint64(n) - 1)

					opts := []qmc.Option{qmc.WithSkip(skip)}
					if randomization.option != nil {
						opts = append(opts, randomization.option)
					}

					g, err := qmc.NewSobol(2, opts...)
					if err != nil {
						t.Fatal(err)
					}

					points := qmc.Draw(g, n)

					point := make([]float64, 2)
					for i := range points {
						g.NextInto(point)

						if point[0] != points[i][0] || point[1] != points[i][1] {
							t.Fatalf("indexed/stateful disagreement at point %d", i)
						}
					}

					for a := uint(0); a <= m; a++ {
						b := m - a
						counts := make([]int, n)

						for _, p := range points {
							cell := int(p[0]*float64(uint64(1)<<a))<<b | int(p[1]*float64(uint64(1)<<b))
							counts[cell]++
						}

						for cell, count := range counts {
							if count != 1 {
								t.Fatalf("split %d/%d cell %d occupancy %d, want 1", a, b, cell, count)
							}
						}
					}
				})
			}
		}
	}
}

func TestScrambledHaltonRepresentableIndexCanRefuseReversal(t *testing.T) {
	var raw uint64 = 1
	for range 8 {
		raw *= 167
	}

	if raw > uint64(math.MaxInt) {
		t.Skip("the representable reversal-overflow reproduction requires int64")
	}

	var g *qmc.Halton

	for seed := uint64(1); seed <= 32; seed++ {
		candidate, err := qmc.NewHalton(39, qmc.WithScrambling(seed))
		if err != nil {
			t.Fatal(err)
		}

		if candidate.Permutation(38)[0] >= 128 {
			g = candidate
			break
		}
	}

	if g == nil {
		t.Fatal("reproduction needs a fixed permutation with a large zero image")
	}

	wantFirst := g.At(0)

	defer func() {
		value := recover()
		if value == nil || !strings.Contains(fmt.Sprint(value), "reverse without overflow") {
			t.Fatalf("representable raw index did not refuse overflowing digit reversal: %v", value)
		}

		if !slices.Equal(g.Next(), wantFirst) {
			t.Fatal("failed indexed reversal consumed or corrupted the stateful stream")
		}
	}()

	g.At(int(raw - 1))
}
