package qmc

import (
	"fmt"
	"math"
	"math/bits"
	"math/rand"
	"testing"
)

// These experiments are test-only. Each timing traverses the same 4096-index
// window rather than allowing calibration's b.N to change the digit workload.
const reviewWindow = 4096

var reviewSink float64

func reviewGenerator(name string, dims int) (Sequence, error) {
	opts := []Option{WithSkip(64)}

	switch name {
	case "Halton/fixed":
		opts = append(opts, WithScrambling(1))
	case "Halton/nested":
		opts = append(opts, WithNestedScrambling(1))
	case "Sobol/shift":
		opts = append(opts, WithDigitalShift(1))
	case "Sobol/Owen":
		opts = append(opts, WithOwenScrambling(1))
	case "Halton/leap", "Sobol/leap":
		opts = append(opts, WithLeap(173))
	}

	if name[:6] == "Halton" {
		return NewHalton(dims, opts...)
	}

	return NewSobol(dims, opts...)
}

func BenchmarkReviewPoint(b *testing.B) {
	for _, name := range []string{
		"Halton/plain", "Halton/fixed", "Halton/nested", "Halton/leap",
		"Sobol/plain", "Sobol/shift", "Sobol/Owen", "Sobol/leap",
	} {
		for _, method := range []string{"AtInto", "NextInto"} {
			b.Run(name+"/"+method, func(b *testing.B) {
				g, err := reviewGenerator(name, 39)
				if err != nil {
					b.Fatal(err)
				}

				dst := make([]float64, g.Dims())

				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					if method == "AtInto" {
						g.AtInto(i%reviewWindow, dst)
					} else {
						if i%reviewWindow == 0 {
							g.Reset()
						}

						g.NextInto(dst)
					}

					reviewSink = dst[0]
				}
			})
		}
	}
}

func BenchmarkReviewConstruction(b *testing.B) {
	for _, dims := range []int{39, 500, 1000} {
		for _, name := range []string{"Halton/plain", "Halton/fixed", "Halton/nested", "Sobol/plain"} {
			b.Run(fmt.Sprintf("%s/%dd", name, dims), func(b *testing.B) {
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					g, err := reviewGenerator(name, dims)
					if err != nil {
						b.Fatal(err)
					}

					reviewSink = float64(g.Dims())
				}
			})
		}
	}
}

func reviewBinary(index int) float64 {
	return math.Min(math.Ldexp(float64(bits.Reverse64(uint64(index))), -64), oneMinusEpsilon)
}

func reviewReciprocal(index, base int) float64 {
	result, inv := 0.0, 1/float64(base)
	for place, i := inv, index; i > 0; i /= base {
		result += float64(i%base) * place
		place *= inv
	}

	return math.Min(result, oneMinusEpsilon)
}

// Immutable caches are constructed eagerly under a 64 KiB digit-table budget.
// Slice metadata is separate and O(dimensions). Roots are prioritized over
// children; neither map insertion nor shared mutable scratch occurs on reads.
type reviewPermutationCache struct {
	root     []int32
	children [][]int32
}

func reviewCache(bases []int, roots []uint64, shallow bool) ([]reviewPermutationCache, int) {
	cache := make([]reviewPermutationCache, len(bases))

	remaining := 64 * 1024
	for d, base := range bases {
		if base > remaining/4 {
			break
		}

		cache[d].root = make([]int32, base)
		nestedPermutation(roots[d], cache[d].root)

		remaining -= base * 4
	}

	if shallow {
		for d, base := range bases {
			if base > remaining/4/base {
				break
			}

			cache[d].children = make([][]int32, base)
			for digit := range base {
				cache[d].children[digit] = make([]int32, base)
				nestedPermutation(nestedChild(roots[d], uint64(digit)), cache[d].children[digit])
			}

			remaining -= base * base * 4
		}
	}

	return cache, 64*1024 - remaining
}

func reviewCachedNested(index, base int, root uint64, cache reviewPermutationCache) float64 {
	var stack [nestedPermStack]int32

	scratch := stack[:]
	if base > len(scratch) {
		scratch = make([]int32, base)
	}

	inv := 1 / float64(base)
	place, node, result := inv, root, 0.0

	first, depth := index%base, 0
	for i := index; i > 0; i /= base {
		digit := i % base

		var mapped int32

		switch {
		case depth == 0 && cache.root != nil:
			mapped = cache.root[digit]
		case depth == 1 && cache.children != nil:
			mapped = cache.children[first][digit]
		default:
			mapped = nestedDigit(node, base, digit, scratch)
		}

		result += float64(mapped) * place
		node = nestedChild(node, uint64(digit))
		place *= inv
		depth++
	}

	for place > 0 && result+float64(base)*place != result {
		var mapped int32
		if depth == 1 && cache.children != nil {
			mapped = cache.children[first][0]
		} else {
			mapped = nestedDigit(node, base, 0, scratch)
		}

		result += float64(mapped) * place
		node = nestedChild(node, 0)
		place *= inv
		depth++
	}

	return math.Min(result, oneMinusEpsilon)
}

func BenchmarkReviewArithmetic(b *testing.B) {
	for _, variant := range []string{"original", "binary", "reciprocal"} {
		b.Run(variant, func(b *testing.B) {
			g, err := NewHalton(39, WithSkip(64))
			if err != nil {
				b.Fatal(err)
			}

			dst := make([]float64, 39)

			fill := func(index int) {
				for d, base := range g.bases {
					dst[d] = radicalInverse(index, base)
				}
			}

			switch variant {
			case "binary":
				fill = func(index int) {
					dst[0] = reviewBinary(index)
					for d := 1; d < g.dims; d++ {
						dst[d] = radicalInverse(index, g.bases[d])
					}
				}
			case "reciprocal":
				fill = func(index int) {
					for d, base := range g.bases {
						dst[d] = reviewReciprocal(index, base)
					}
				}
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				fill(65 + i%reviewWindow)

				reviewSink = dst[0]
			}
		})
	}
}

func BenchmarkReviewCacheConstruction(b *testing.B) {
	for _, dims := range []int{39, 1000} {
		for _, shallow := range []bool{false, true} {
			b.Run(fmt.Sprintf("%dd/shallow=%t", dims, shallow), func(b *testing.B) {
				g, err := NewHalton(dims, WithNestedScrambling(1))
				if err != nil {
					b.Fatal(err)
				}

				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					cache, bytes := reviewCache(g.bases, g.nest.roots, shallow)
					reviewSink = float64(len(cache[0].root) + bytes)
				}
			})
		}
	}
}

func BenchmarkReviewCache(b *testing.B) {
	for _, variant := range []string{"uncached", "root", "shallow"} {
		b.Run(variant, func(b *testing.B) {
			g, err := NewHalton(39, WithNestedScrambling(1))
			if err != nil {
				b.Fatal(err)
			}

			cache, bytes := reviewCache(g.bases, g.nest.roots, variant == "shallow")
			dst := make([]float64, 39)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				for d, base := range g.bases {
					if variant == "uncached" {
						dst[d] = nestedRadicalInverse(65+i%reviewWindow, base, g.nest.roots[d])
					} else {
						dst[d] = reviewCachedNested(65+i%reviewWindow, base, g.nest.roots[d], cache[d])
					}
				}

				reviewSink = dst[0]
			}

			b.StopTimer()

			if variant != "uncached" {
				b.ReportMetric(float64(bytes), "table-B")
			}
		})
	}
}

func reviewBulk(g *Halton, dst []float64, dimensionFirst bool) {
	const points = 1024

	if dimensionFirst {
		for d, base := range g.bases {
			for i := range points {
				dst[i*g.dims+d] = radicalInverse(g.skip+1+i*g.leap, base)
			}
		}
	} else {
		for i := range points {
			g.AtInto(i, dst[i*g.dims:(i+1)*g.dims])
		}
	}
}

func BenchmarkReviewBulk(b *testing.B) {
	for _, dimensionFirst := range []bool{false, true} {
		b.Run(fmt.Sprintf("dimension-first=%t", dimensionFirst), func(b *testing.B) {
			g, err := NewHalton(39, WithSkip(64))
			if err != nil {
				b.Fatal(err)
			}

			dst := make([]float64, 1024*39)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				reviewBulk(g, dst, dimensionFirst)
				reviewSink = dst[0]
			}
		})
	}
}

func TestReviewExperimentContracts(t *testing.T) {
	var binaryChanges, reciprocalChanges int

	maxReciprocalError := 0.0

	for _, base := range mustPrimes(39) {
		for _, index := range []int{1, 65, 4160, 1234567, math.MaxInt / 3, math.MaxInt} {
			original := radicalInverse(index, base)
			if base == 2 && reviewBinary(index) != original {
				binaryChanges++
			}

			candidate := reviewReciprocal(index, base)
			if candidate != original {
				reciprocalChanges++
			}

			maxReciprocalError = math.Max(maxReciprocalError, math.Abs(candidate-original))
		}
	}

	t.Logf("arithmetic compatibility: binary changes=%d reciprocal changes=%d max absolute error=%g", binaryChanges, reciprocalChanges, maxReciprocalError)

	rng := splitMix64(31)
	for range 4096 {
		index := int(rng.next() & uint64(math.MaxInt))
		if reviewBinary(index) != radicalInverse(index, 2) {
			binaryChanges++
		}
	}

	t.Logf("binary compatibility including 4096 full-width indices: changes=%d", binaryChanges)

	for _, seed := range []uint64{1, 31} {
		g, err := NewHalton(39, WithSkip(64), WithNestedScrambling(seed))
		if err != nil {
			t.Fatal(err)
		}

		for _, shallow := range []bool{false, true} {
			cache, bytes := reviewCache(g.bases, g.nest.roots, shallow)
			if bytes > 64*1024 {
				t.Fatal("cache exceeded its digit-table budget")
			}

			for _, index := range []int{1, 65, 4160, 1234567, math.MaxInt / 3, math.MaxInt} {
				for d, base := range g.bases {
					got := reviewCachedNested(index, base, g.nest.roots[d], cache[d])

					want := nestedRadicalInverse(index, base, g.nest.roots[d])
					if got != want {
						t.Fatalf("cache changed value: seed=%d shallow=%t index=%d dim=%d: %g/%g", seed, shallow, index, d, got, want)
					}
				}
			}
		}
	}

	g, err := NewHalton(39, WithSkip(64))
	if err != nil {
		t.Fatal(err)
	}

	pointFirst, dimensionFirst := make([]float64, 1024*39), make([]float64, 1024*39)
	reviewBulk(g, pointFirst, false)
	reviewBulk(g, dimensionFirst, true)

	for i := range pointFirst {
		if pointFirst[i] != dimensionFirst[i] {
			t.Fatalf("bulk loop changed coordinate %d", i)
		}
	}
}

// This includes construction, sequence generation, and integrand evaluation.
// RMS-SE is a delta-method estimate across these finite deterministic seeds;
// it does not measure discretization/PRNG bias or predict another integrand.
func BenchmarkReviewIntegration(b *testing.B) {
	for _, name := range []string{"MC", "Halton/fixed", "Halton/nested", "Sobol/shift", "Sobol/Owen"} {
		b.Run(name, func(b *testing.B) {
			const streams = 40

			point := make([]float64, 39)

			var sumSquared, sumFourth float64

			b.ReportAllocs()
			b.ResetTimer()

			for iteration := 0; iteration < b.N; iteration++ {
				sumSquared, sumFourth = 0, 0

				for seed := 1; seed <= streams; seed++ {
					var (
						g   Sequence
						rng *rand.Rand
					)
					if name == "MC" {
						rng = rand.New(rand.NewSource(20240823 + int64(seed))) //nolint:gosec // reproducible statistical baseline
					} else {
						var err error

						switch name {
						case "Halton/fixed":
							g, err = NewHalton(39, WithSkip(64), WithScrambling(uint64(seed)))
						case "Halton/nested":
							g, err = NewHalton(39, WithSkip(64), WithNestedScrambling(uint64(seed)))
						case "Sobol/shift":
							g, err = NewSobol(39, WithSkip(64), WithDigitalShift(uint64(seed)))
						default:
							g, err = NewSobol(39, WithSkip(64), WithOwenScrambling(uint64(seed)))
						}

						if err != nil {
							b.Fatal(err)
						}
					}

					sum := 0.0

					for i := range reviewWindow {
						if rng != nil {
							for d := range point {
								point[d] = rng.Float64()
							}
						} else {
							g.AtInto(i, point)
						}

						sum += nestedIntegrand(point)
					}

					e := sum/float64(reviewWindow) - 1
					sumSquared += e * e
					sumFourth += e * e * e * e
				}

				reviewSink = sumSquared
			}

			b.StopTimer()

			rms := math.Sqrt(sumSquared / streams)
			variance := math.Max(0, (sumFourth-sumSquared*sumSquared/streams)/(streams-1))

			b.ReportMetric(rms, "RMS")
			b.ReportMetric(math.Sqrt(variance/streams)/(2*rms), "RMS-SE")
		})
	}
}
