package qmc_test

import (
	"fmt"
	"testing"

	"github.com/cwbudde/qmc"
)

// Benchmarks for the calls a caller makes in an inner loop.
//
// The justfile ships a `bench` recipe (`go test -bench=. -benchmem ./...`).
// Until this file existed that recipe matched nothing and reported success, so
// "benchmarks pass" meant only that no benchmarks ran. These give it something
// to measure.
//
// The allocation counts are the load-bearing part, which is why every
// benchmark calls b.ReportAllocs. The package's whole reason for offering
// NextInto and AtInto alongside Next is that an optimizer sampling a few
// hundred thousand points cannot afford one slice allocation per point. If a
// refactor ever made the *Into paths allocate, nothing else in the suite would
// notice — the values would still be correct — but the API would have lost the
// only thing that distinguishes it from Next.
//
// Reference figures on the machine these were written on, at 39 dimensions:
// Next 1074 ns/op with 320 B/op in 1 alloc, AtInto 800 ns/op with 0 allocs,
// AtInto scrambled 1279 ns/op with 0 allocs. Scrambling costs roughly 60%
// because the digit loop gains a permutation lookup and the geometric tail.
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

// BenchmarkAtIntoLeaped measures what WithLeap actually costs, which is not
// what it looks like it should cost.
//
// The implementation is one multiply in fill, and a multiply amortised over 39
// coordinates should be unmeasurable. Measured at a leap of 173 it is 634
// ns/op against AtInto's 512 — about 24%. The multiply is not where that goes:
// a leap reaches raw index skip+1+i*leap instead of skip+1+i, so at the same
// point count it is working on indices 173 times larger, and a radical inverse
// costs one loop iteration per digit. Dimension 0 gains about 7.4 base-2 digits
// and dimension 38 about one base-167 digit.
//
// So the cost of leaping is proportional to log(leap), not to the multiply, and
// it is paid by every coordinate. Worth knowing before choosing a large leap
// for its own sake.
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

// Discrepancy benchmarks.
//
// These are not inner-loop calls — a caller measures a point set once — but
// they are the only benchmarks in this package whose numbers become part of
// the API. starBoxBudget is a wall-clock promise expressed in leaf counts, and
// the only honest way to set it is to divide a measured time by the
// C(N+s,s) leaves that shape actually has. BenchmarkStarDiscrepancy is where
// that division is done; see the constant's comment for the arithmetic.
//
// The two shapes are chosen to bracket the tree: 1024 points in 2 dimensions
// is wide and shallow (5.26e5 leaves, dominated by the per-node sort), 160
// points in 4 dimensions is narrow and deep (2.91e7 leaves, dominated by the
// leaves themselves). They come out at 27.7 and 26.3 nanoseconds per leaf, so
// the per-leaf cost is flat across shapes — which is the fact that makes a
// leaf count usable as a wall-clock budget at all, and would stop being true
// if the pruner or the sort were changed.
//
// The 4-dimensional shape sits just under starBoxBudget on purpose. Raise it
// much and the benchmark trips the refusal it is calibrating.
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
