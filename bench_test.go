package qmc_test

import (
	"fmt"
	"testing"

	"github.com/cwbudde/qmc"
)

// Benchmarks report throughput and allocations for the stated configurations.
// Use docs/performance.md's fixed-workload harness for comparable baselines;
// these adaptive index ranges can vary with benchmark calibration.
const benchDims = 39

// sink keeps the compiler from eliminating the work. The benchmarked calls
// have no other observable effect, and a dead-code-eliminated benchmark
// reports an impressively small number that means nothing.
var sink float64

func BenchmarkNext(b *testing.B) {
	g, err := qmc.NewHalton(benchDims, qmc.WithSkip(64))
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p := g.Next()
		sink += p[0]
	}
}

func BenchmarkAtInto(b *testing.B) {
	g, err := qmc.NewHalton(benchDims, qmc.WithSkip(64))
	if err != nil {
		b.Fatal(err)
	}

	dst := make([]float64, benchDims)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g.AtInto(i, dst)
		sink += dst[0]
	}
}

// BenchmarkAtIntoScrambled is the configuration the package actually
// recommends above twenty dimensions, so it is the number a caller budgeting
// for a 39-knob search should read. The gap against BenchmarkAtInto is the
// price of scrambling.
func BenchmarkAtIntoScrambled(b *testing.B) {
	g, err := qmc.NewHalton(benchDims, qmc.WithSkip(64), qmc.WithScrambling(1))
	if err != nil {
		b.Fatal(err)
	}

	dst := make([]float64, benchDims)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g.AtInto(i, dst)
		sink += dst[0]
	}
}

// BenchmarkNextInto pins that the stateful path is as allocation-free as the
// stateless one. It is the form most callers reach for first, and it would be
// easy to optimise At and leave this behind.
func BenchmarkNextInto(b *testing.B) {
	g, err := qmc.NewHalton(benchDims, qmc.WithSkip(64))
	if err != nil {
		b.Fatal(err)
	}

	dst := make([]float64, benchDims)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g.NextInto(dst)
		sink += dst[0]
	}
}

// Construction benchmarks. NewHalton runs a prime sieve, and with scrambling
// it also builds one permutation per dimension — work that is proportional to
// the sum of the bases, not to the dimension count. At high dimension counts
// that is no longer negligible, and a caller who constructs a generator per
// task (rather than once per run) needs to know it. These are the benchmarks
// that would catch a change turning the sieve quadratic.
func BenchmarkNewHaltonHighDims(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g, err := qmc.NewHalton(1000)
		if err != nil {
			b.Fatal(err)
		}

		sink += float64(g.Dims())
	}
}

func BenchmarkNewHaltonHighDimsScrambled(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g, err := qmc.NewHalton(1000, qmc.WithScrambling(uint64(i)))
		if err != nil {
			b.Fatal(err)
		}

		sink += float64(g.Dims())
	}
}

// BenchmarkAtIntoLeaped includes the digit work from larger raw indices.
// Leaping can increase radical-inverse digit counts as well as index arithmetic.
func BenchmarkAtIntoLeaped(b *testing.B) {
	g, err := qmc.NewHalton(benchDims, qmc.WithSkip(64), qmc.WithLeap(173))
	if err != nil {
		b.Fatal(err)
	}

	dst := make([]float64, benchDims)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g.AtInto(i, dst)
		sink += dst[0]
	}
}

// BenchmarkStarDiscrepancy exercises wide/shallow and narrow/deep searches.
// The generic leaf gate bounds accepted work rather than promising wall clock;
// measurements and workload details are in docs/discrepancy.md.
func BenchmarkStarDiscrepancy(b *testing.B) {
	for _, c := range []struct {
		name string
		dims int
		n    int
	}{
		{"2d-1024", 2, 1024},
		{"4d-160", 4, 160},
	} {
		b.Run(c.name, func(b *testing.B) {
			g, err := qmc.NewHalton(c.dims, qmc.WithSkip(64), qmc.WithScrambling(1))
			if err != nil {
				b.Fatal(err)
			}

			pts := qmc.Draw(g, c.n)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				d, err := qmc.StarDiscrepancy(pts)
				if err != nil {
					b.Fatal(err)
				}

				sink += d
			}
		})
	}
}

// BenchmarkCenteredL2Discrepancy runs at the package's design point, where the
// statistic is O(N^2 s) and says nothing (see CenteredL2Discrepancy's
// saturation caveat). The quadratic is the reason the wasm demo has to slice
// this work: quadrupling n from 1024 to 4096 costs sixteen times as much, and
// there is no way to subdivide a single call.
func BenchmarkCenteredL2Discrepancy(b *testing.B) {
	for _, n := range []int{1024, 4096} {
		b.Run(fmt.Sprintf("39d-%d", n), func(b *testing.B) {
			g, err := qmc.NewHalton(benchDims, qmc.WithSkip(64), qmc.WithScrambling(1))
			if err != nil {
				b.Fatal(err)
			}

			pts := qmc.Draw(g, n)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				d, err := qmc.CenteredL2Discrepancy(pts)
				if err != nil {
					b.Fatal(err)
				}

				sink += d
			}
		})
	}
}

// BenchmarkDraw pins the allocation count that Draw's doc comment promises:
// two, one for the flat backing array and one for the row headers, whatever n
// is. A refactor that gave each row its own array would still be correct and
// would still pass every other test, and this is the only place it would show
// up.
func BenchmarkDraw(b *testing.B) {
	g, err := qmc.NewHalton(benchDims, qmc.WithSkip(64), qmc.WithScrambling(1))
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pts := qmc.Draw(g, 4096)
		sink += pts[0][0]
	}
}
