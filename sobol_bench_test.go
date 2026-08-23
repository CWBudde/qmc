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
// Reference figures at 39 dimensions, measured on an i7-1255U: Sobol AtInto
// 232 ns/op, AtInto with a digital shift 279 ns/op, NextInto 55.0 ns/op, Next
// 125 ns/op with 320 B in 1 alloc, and NewSobol at 1000 dimensions 351 us/op.
// Every *Into figure is 0 allocs.
//
// Comparing those against the Halton figures recorded in bench_test.go would
// be wrong, because that file's numbers were taken on a different machine. On
// this one Halton re-measures at 462 ns/op for AtInto and 587 ns/op scrambled,
// so the honest comparisons are: Sobol's direct path is 2.0x faster than
// Halton's, its shifted direct path 2.1x faster than scrambled Halton, and its
// Gray-code recurrence 8.6x faster than Halton's NextInto at 471 ns/op. The
// direct-path gap is smaller than it looks like it should be because AtInto
// still XORs one direction number per set bit of the index; the recurrence is
// where the ordering pays off.
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

// BenchmarkSobolAtIntoShifted is the configuration the integration gate below
// uses and the one a caller wanting an error estimate will reach for.
//
// It costs 20% over the unshifted path (279 against 232 ns/op), which is more
// than the arithmetic deserves — the shift itself is one XOR per dimension
// against a word already in cache. The rest is the nil check on the shift
// slice, which sits inside the per-dimension loop. Hoisting it into two copies
// of the loop would recover most of that, and is not done: 47 ns across 39
// dimensions is a little over a nanosecond per coordinate, and the duplicated
// loop would be a second place for the accumulate-and-scale step to be
// changed in only one of them. For scale, the same randomization on Halton
// costs 27% (587 against 462 ns/op), so the cheap randomization is still the
// cheap one.
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

// BenchmarkSobolNextInto is the number that justifies the Gray-code ordering.
// AtInto XORs one direction number per set bit of the index — around six on
// average over the first 4096 indices — while NextInto XORs exactly one per
// dimension, and the measured gap is 232 against 55.0 ns/op. If a change ever
// made these two converge, the ordering would have stopped paying for itself
// and should be reconsidered rather than kept out of habit.
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

// BenchmarkNewSobolHighDims covers the constructor. Unlike NewHalton, which
// runs a sieve, this one expands 32 direction numbers per dimension from a
// table that has already been parsed and validated once per process — so the
// number to watch is whether it has stopped being once per process. A
// regression that moved the primitivity checks back into every construction
// would show up here as a jump of orders of magnitude, and nowhere else.
//
// At 1000 dimensions it measures 351 us/op, against 30.6 us for NewHalton and
// 22.7 ms for NewHalton with scrambling. The middle figure is the honest
// comparison and Sobol loses it by 11x: expanding 32 direction numbers for
// each of 1000 dimensions is simply more work than a sieve. It is still three
// orders of magnitude below the cost of drawing the points that follow.
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

// sobolRMSError is qmcRMSError from integration_test.go with a Sobol generator
// and a digital shift in place of a scrambled Halton. It is a separate
// function rather than a parameter on that one because integration_test.go
// belongs to the Halton work and this file must not reach into it; the
// duplication is four lines and the alternative is coupling two agents' files
// together.
//
// The reasoning behind the shape is that file's and is not restated here:
// averaging over independent randomization seeds is what makes the number the
// quantity the theory bounds, rather than a report on whether one seed landed
// well.
