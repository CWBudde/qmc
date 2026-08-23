package qmc_test

import (
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
