package qmc_test

import (
	"testing"

	"github.com/cwbudde/qmc"
)

// Benchmarks for Sobol, in the same shape as bench_test.go.
//
// sink is declared in bench_test.go and deliberately shared: it exists to stop
// the compiler eliminating benchmarked work that has no other observable
// effect, and one variable does that for the whole package.
//
// Comparable fixed-workload baselines are in docs/performance.md.
const sobolBenchDims = 39

func BenchmarkSobolNext(b *testing.B) {
	g, err := qmc.NewSobol(sobolBenchDims, qmc.WithSkip(64))
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

func BenchmarkSobolAtInto(b *testing.B) {
	g, err := qmc.NewSobol(sobolBenchDims, qmc.WithSkip(64))
	if err != nil {
		b.Fatal(err)
	}

	dst := make([]float64, sobolBenchDims)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g.AtInto(i, dst)
		sink += dst[0]
	}
}

// BenchmarkSobolAtIntoShifted measures indexed digital shifting.
func BenchmarkSobolAtIntoShifted(b *testing.B) {
	g, err := qmc.NewSobol(sobolBenchDims, qmc.WithSkip(64), qmc.WithDigitalShift(1))
	if err != nil {
		b.Fatal(err)
	}

	dst := make([]float64, sobolBenchDims)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g.AtInto(i, dst)
		sink += dst[0]
	}
}

// BenchmarkSobolNextInto measures the stateful Gray-code recurrence.
func BenchmarkSobolNextInto(b *testing.B) {
	g, err := qmc.NewSobol(sobolBenchDims, qmc.WithSkip(64))
	if err != nil {
		b.Fatal(err)
	}

	dst := make([]float64, sobolBenchDims)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g.NextInto(dst)
		sink += dst[0]
	}
}

// BenchmarkNewSobolHighDims measures expansion from the cached embedded table.
func BenchmarkNewSobolHighDims(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g, err := qmc.NewSobol(1000)
		if err != nil {
			b.Fatal(err)
		}

		sink += float64(g.Dims())
	}
}

// BenchmarkSobolAtIntoOwen measures indexed hash-based scrambling.
// Accuracy and timing tradeoffs depend on the workload; see docs/performance.md.
func BenchmarkSobolAtIntoOwen(b *testing.B) {
	g, err := qmc.NewSobol(sobolBenchDims, qmc.WithOwenScrambling(1))
	if err != nil {
		b.Fatal(err)
	}

	dst := make([]float64, sobolBenchDims)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g.AtInto(i, dst)
	}

	sink = dst[0]
}

func BenchmarkSobolNextIntoOwen(b *testing.B) {
	g, err := qmc.NewSobol(sobolBenchDims, qmc.WithOwenScrambling(1))
	if err != nil {
		b.Fatal(err)
	}

	dst := make([]float64, sobolBenchDims)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if i%(1<<20) == 0 {
			g.Reset()
		}

		g.NextInto(dst)
	}

	sink = dst[0]
}

// BenchmarkSobolNextIntoLeaped measures the indexed fallback used by leaping.
// Consecutive-state Gray-code updates cannot implement this strided access.
func BenchmarkSobolNextIntoLeaped(b *testing.B) {
	g, err := qmc.NewSobol(sobolBenchDims, qmc.WithSkip(64), qmc.WithLeap(173))
	if err != nil {
		b.Fatal(err)
	}

	dst := make([]float64, sobolBenchDims)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g.NextInto(dst)
		sink += dst[0]

		// The 32-bit raw index is reached 173 times sooner with this leap, so
		// the cursor is rewound well before it can run out. Reset costs one
		// accumulate; reset overhead is included in this benchmark.
		if i%1_000_000 == 999_999 {
			g.Reset()
		}
	}
}
