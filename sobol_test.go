package qmc

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"math/bits"
	"strings"
	"testing"
)

// These are white-box tests, in package qmc, because the direction-number
// table is the part of Sobol most worth testing and none of it is exported: a
// caller cannot reach parseDirectionNumbers, validateDirectionRows or
// isPrimitiveOverGF2, and testing them only through NewSobol would reduce
// every one of the checks below to "construction succeeded". The integration
// gate and the benchmarks live in sobol_bench_test.go, in package qmc_test,
// because they reuse productIntegrand from integration_test.go.

// referenceDims and referencePoints are the first 16 points of the
// 6-dimensional Sobol sequence.
//
// Source: Joe and Kuo's own generator, sobol.cc from
// https://web.maths.unsw.edu.au/~fkuo/sobol/, compiled unmodified and run as
// `sobol 16 6 new-joe-kuo-6.21201`. Using the authors' program against the
// authors' direction numbers is the only reference that checks the whole chain
// at once — the file format's a-bit convention, the recurrence that extends
// m_1..m_s to 32 direction numbers, the Gray-code ordering, and the scaling —
// rather than checking this package against a paraphrase of itself.
//
// The first two columns are independently corroborated: they are the values
// scipy.stats.qmc.Sobol(d=2, scramble=False).random(4) is documented to
// return, 0, 1/2, 3/4, 1/4 against 0, 1/2, 1/4, 3/4. That matters because the
// 3/4-before-1/4 order in column one is exactly what distinguishes Gray-code
// order from index order, so a reader who expects index order can see from one
// published source that the ordering here is not a mistake.
//
// Row 0 is raw index 0, the all-zeros origin, which the generator never
// returns: with the default skip, point i is raw index i+1 and so row i+1.
const referenceDims = 6

var referencePoints = [16][referenceDims]float64{
	{0, 0, 0, 0, 0, 0},
	{0.5, 0.5, 0.5, 0.5, 0.5, 0.5},
	{0.75, 0.25, 0.25, 0.25, 0.75, 0.75},
	{0.25, 0.75, 0.75, 0.75, 0.25, 0.25},
	{0.375, 0.375, 0.625, 0.875, 0.375, 0.125},
	{0.875, 0.875, 0.125, 0.375, 0.875, 0.625},
	{0.625, 0.125, 0.875, 0.625, 0.625, 0.875},
	{0.125, 0.625, 0.375, 0.125, 0.125, 0.375},
	{0.1875, 0.3125, 0.9375, 0.4375, 0.5625, 0.3125},
	{0.6875, 0.8125, 0.4375, 0.9375, 0.0625, 0.8125},
	{0.9375, 0.0625, 0.6875, 0.1875, 0.3125, 0.5625},
	{0.4375, 0.5625, 0.1875, 0.6875, 0.8125, 0.0625},
	{0.3125, 0.1875, 0.3125, 0.5625, 0.9375, 0.4375},
	{0.8125, 0.6875, 0.8125, 0.0625, 0.4375, 0.9375},
	{0.5625, 0.4375, 0.0625, 0.8125, 0.1875, 0.6875},
	{0.0625, 0.9375, 0.5625, 0.3125, 0.6875, 0.1875},
}

// TestFirstPointsMatchPublishedValues compares against referencePoints
// exactly, not within a tolerance. Every value in the table is a dyadic
// rational that a float64 represents exactly, because a coordinate is an
// integer divided by 2^32; a tolerance here would only hide the kind of defect
// — an off-by-one shift, a dropped direction number — that moves a coordinate
// by a whole power of two.
func TestFirstPointsMatchPublishedValues(t *testing.T) {
	g, err := NewSobol(referenceDims)
	if err != nil {
		t.Fatal(err)
	}

	point := make([]float64, referenceDims)
	for i := 1; i < len(referencePoints); i++ {
		g.AtInto(i-1, point)

		for d, want := range referencePoints[i] {
			if point[d] != want {
				t.Fatalf(
					"point %d (raw index %d), dimension %d: got %v, want %v; "+
						"the generator no longer reproduces Joe and Kuo's own output for their own direction numbers",
					i-1, i, d, point[d], want,
				)
			}
		}
	}
}

// TestOriginIsNeverReturned pins the one point the skip convention exists to
// exclude.
//
// Raw index 0 selects no direction numbers at all, so an unshifted generator
// puts it at the origin — a corner of the cube, in every dimension at once,
// and the single worst first sample available. Halton's convention exists for
// the same reason and this package matches it rather than inventing a second
// one, so that a caller switching generators does not silently shift every
// point by one index.
func TestOriginIsNeverReturned(t *testing.T) {
	g, err := NewSobol(8)
	if err != nil {
		t.Fatal(err)
	}

	for _, i := range []int{-1, 0} {
		point := g.At(i)

		allZero := true

		for _, v := range point {
			if v != 0 {
				allZero = false

				break
			}
		}

		if allZero {
			t.Fatalf("At(%d) returned the all-zeros origin; point 0 must be raw index skip+1, not raw index 0", i)
		}
	}

	// With the default skip, point 0 is raw index 1, which selects V_1 = 2^31
	// in every dimension: the centre of the cube.
	for d, v := range g.At(0) {
		if v != 0.5 {
			t.Fatalf("point 0, dimension %d = %v, want 0.5 (raw index 1 selects V_1 alone in every dimension)", d, v)
		}
	}
}

// TestNextAgreesWithAt is the test this construction most needs.
//
// Next and At compute the same points by genuinely different routes: At XORs
// one direction number per set bit of gray(index), while Next carries an
// accumulator and XORs a single direction number chosen by the lowest zero bit
// of the counter. The recurrence quietly drifting from the direct form is the
// classic Sobol bug, and it is quiet because the drift produces points that
// are still in the cube, still well spread, and still perfectly reproducible —
// there is no symptom to notice except this comparison. A skip is included
// because the recurrence has to be seeded from a direct evaluation at raw
// index skip+1, which is the step where an off-by-one hides.
func TestNextAgreesWithAt(t *testing.T) {
	// The point counts fall with the dimension count only to keep the test
	// quick; the property is per-point, so 1024 dimensions over 512 points
	// exercises the same recurrence as 7 dimensions over 8000. What the wide
	// case adds is that a dimension near the end of the flat direction-number
	// slice is indexed the same way by both paths.
	for _, dims := range []int{1, 2, 7, 39, 1024} {
		points := 8000
		if dims > 100 {
			points = 512
		}

		for _, skip := range []int{0, 1, 64, 4095} {
			g, err := NewSobol(dims, WithSkip(skip))
			if err != nil {
				t.Fatal(err)
			}

			stateful := make([]float64, dims)
			direct := make([]float64, dims)

			for i := 0; i < points; i++ {
				g.NextInto(stateful)
				g.AtInto(i, direct)

				for d := 0; d < dims; d++ {
					if stateful[d] != direct[d] {
						t.Fatalf(
							"dims=%d skip=%d point %d dimension %d: Next gave %v, At gave %v; "+
								"the Gray-code recurrence has drifted from the direct evaluation, "+
								"so the stateful and stateless paths are now different sequences",
							dims, skip, i, d, stateful[d], direct[d],
						)
					}
				}
			}
		}
	}
}

// TestNextAgreesWithAtUnderDigitalShift repeats the comparison with a shift
// on, because the shift enters the two paths at different moments: At XORs it
// after accumulating, while Next folds it into the state once at Reset and
// never touches it again. A shift applied twice, or applied to one path and
// not the other, would leave both paths self-consistent and only this
// comparison would see it.
func TestNextAgreesWithAtUnderDigitalShift(t *testing.T) {
	g, err := NewSobol(39, WithSkip(64), WithDigitalShift(20240823))
	if err != nil {
		t.Fatal(err)
	}

	stateful := make([]float64, 39)
	direct := make([]float64, 39)

	for i := 0; i < 4096; i++ {
		g.NextInto(stateful)
		g.AtInto(i, direct)

		for d := range stateful {
			if stateful[d] != direct[d] {
				t.Fatalf(
					"shifted point %d dimension %d: Next gave %v, At gave %v; "+
						"the digital shift is not reaching both paths identically",
					i, d, stateful[d], direct[d],
				)
			}
		}
	}
}

// TestResetRewinds pins that Reset restores the recurrence exactly, not
// approximately. Reset re-seeds the accumulator by direct evaluation at raw
// index skip+1, so it is the one place where the stateful path can be
// re-derived incorrectly and still produce a plausible-looking run — one that
// simply starts somewhere else in the sequence.
func TestResetRewinds(t *testing.T) {
	g, err := NewSobol(12, WithSkip(7), WithDigitalShift(3))
	if err != nil {
		t.Fatal(err)
	}

	first := make([][]float64, 64)
	for i := range first {
		first[i] = g.Next()
	}

	g.Reset()

	for i := range first {
		again := g.Next()

		for d := range again {
			if again[d] != first[i][d] {
				t.Fatalf(
					"after Reset, point %d dimension %d = %v, was %v; "+
						"Reset must rewind the cursor without changing the sequence",
					i, d, again[d], first[i][d],
				)
			}
		}
	}
}

// TestCoordinatesLieInUnitInterval walks a wide, deep sample and insists on
// the half-open range the package promises.
//
// Sobol cannot reach 1.0 by construction — the accumulator is a uint32 and the
// coordinate is that value over 2^32 — so unlike Halton there is no clamp
// here to test. What this catches instead is a coordinate that has stopped
// being that quotient at all: a sign that leaked in through a signed
// conversion, or a shift that overflowed the accumulator's scaling.
func TestCoordinatesLieInUnitInterval(t *testing.T) {
	for _, shift := range []bool{false, true} {
		opts := []Option{WithSkip(64)}
		if shift {
			opts = append(opts, WithDigitalShift(99))
		}

		g, err := NewSobol(1024, opts...)
		if err != nil {
			t.Fatal(err)
		}

		point := make([]float64, 1024)
		for _, i := range []int{0, 1, 2, 3, 1023, 4096, 65535, 1 << 20} {
			g.AtInto(i, point)

			for d, v := range point {
				if !(v >= 0 && v < 1) {
					t.Fatalf(
						"shift=%v point %d dimension %d = %v, want [0,1); "+
							"callers rely on the half-open range to index bucket tables without a bounds check",
						shift, i, d, v,
					)
				}
			}
		}
	}
}

// TestFirstPointsAreOneDimensionallyBalanced is the defining (t,m,s)-net
// property, in the form that holds universally.
//
// A digital (0,m,1)-net puts exactly one of its 2^m points in each of the 2^m
// intervals [k/2^m, (k+1)/2^m). Sobol satisfies this in every dimension
// independently, and it is precisely what a damaged direction table destroys:
// the V_i of a dimension have to be linearly independent over GF(2) for the
// map from index to interval to be a bijection, and an even m_i, a shifted
// row, or a non-primitive polynomial all break that independence. The points
// then still spread across the cube — that is why a corrupted table is
// invisible in the output — but some intervals get two points and others none.
//
// The block has to be aligned. WithSkip(2^m - 1) puts the points on raw
// indices 2^m .. 2^(m+1)-1, and a dyadic block of 2^m consecutive indices is a
// net only when it starts on a multiple of 2^m. The default skip of 0 gives
// raw indices 1..2^m, which straddles two blocks: measured here, that leaves
// 40 dimensions out of 40 unbalanced at m=8. This is not a defect, it is what
// "the first 2^m points" means, and the test says so rather than quietly
// choosing a skip that works.
//
// Gray-code ordering does not disturb any of this: gray is a bijection that
// preserves the top bit, so it permutes each aligned dyadic block onto itself
// and the point set of a block is the same either way.
func TestFirstPointsAreOneDimensionallyBalanced(t *testing.T) {
	const dims = 1024

	for _, m := range []int{4, 8, 12} {
		n := 1 << m

		for _, shift := range []bool{false, true} {
			opts := []Option{WithSkip(n - 1)}
			if shift {
				opts = append(opts, WithDigitalShift(7))
			}

			g, err := NewSobol(dims, opts...)
			if err != nil {
				t.Fatal(err)
			}

			counts := make([]int, dims*n)
			point := make([]float64, dims)

			for i := 0; i < n; i++ {
				g.AtInto(i, point)

				for d, v := range point {
					counts[d*n+int(v*float64(n))]++
				}
			}

			for d := 0; d < dims; d++ {
				for k := 0; k < n; k++ {
					if got := counts[d*n+k]; got != 1 {
						t.Fatalf(
							"m=%d shift=%v dimension %d: interval [%d/%d, %d/%d) holds %d of the %d points, want exactly 1; "+
								"this dimension's direction numbers are no longer linearly independent over GF(2), "+
								"which means the point set has stopped being a digital net",
							m, shift, d, k, n, k+1, n, got, n,
						)
					}
				}
			}
		}
	}
}

// TestFirstPointsSatisfyPropertyA is the multi-dimensional half of the balance
// check, and the one that would notice a table whose rows are individually
// valid but attached to the wrong dimensions.
//
// Property A asks that among the first 2^s points of an s-dimensional Sobol
// sequence, exactly one lands in each of the 2^s orthants you get by halving
// every coordinate. Joe and Kuo state that their D(6) table satisfies it up to
// dimension 1111, so it is a published guarantee about these specific numbers
// rather than a property of Sobol sequences in general — which makes it a
// check on the table's identity, not just its shape. A row shifted by one
// line leaves every dimension holding a valid primitive polynomial and valid
// direction numbers, just not its own; the one-dimensional test above cannot
// see that, and this one can.
//
// s stops at 16 because the work is 2^s points of s coordinates: 16 costs
// about a million coordinate evaluations, and 20 costs twenty million for no
// additional kind of evidence.
func TestFirstPointsSatisfyPropertyA(t *testing.T) {
	for _, s := range []int{8, 12, 16} {
		n := 1 << s

		g, err := NewSobol(s, WithSkip(n-1))
		if err != nil {
			t.Fatal(err)
		}

		seen := make([]bool, n)
		point := make([]float64, s)

		for i := 0; i < n; i++ {
			g.AtInto(i, point)

			orthant := 0

			for _, v := range point {
				orthant <<= 1
				if v >= 0.5 {
					orthant |= 1
				}
			}

			if seen[orthant] {
				t.Fatalf(
					"at %d dimensions, two of the first %d points share orthant %0*b; "+
						"Property A fails, so the direction numbers are not Joe and Kuo's D(6) table "+
						"for these dimensions — most likely a row has been attached to the wrong dimension",
					s, n, s, orthant,
				)
			}

			seen[orthant] = true
		}
	}
}

// TestFirstTwoDimensionsFormAZeroNet checks the strongest balance property
// this table actually has: every elementary interval, at every aspect ratio.
//
// The first two dimensions of a Sobol sequence form a (0,m,2)-net, so for
// every split m = a+b the 2^m points fall one apiece into the 2^a x 2^b
// rectangles. That is a much stronger statement than the one-dimensional test
// and it is the natural thing to want for all pairs — so it is worth recording
// why the test does not ask for it. Measured over all 780 pairs of the first
// 40 dimensions, 18 are balanced at every split at m=8 and 4 at m=10 — and
// (0,1) is the only pair in both lists. Sobol sequences are (t,m,s)-nets with
// t growing with s, and the
// Joe-Kuo D(6) search optimises the quality of two-dimensional projections
// without making them all t=0. A test demanding it of every pair would be
// asserting something false about a correct table, and the usual fate of such
// a test is to be loosened until it asserts nothing.
func TestFirstTwoDimensionsFormAZeroNet(t *testing.T) {
	for _, m := range []int{4, 6, 8, 10} {
		n := 1 << m

		g, err := NewSobol(2, WithSkip(n-1))
		if err != nil {
			t.Fatal(err)
		}

		points := make([][]float64, n)
		for i := range points {
			points[i] = g.At(i)
		}

		for a := 0; a <= m; a++ {
			b := m - a
			counts := make([]int, n)

			for _, p := range points {
				counts[int(p[0]*float64(int(1)<<a))<<b|int(p[1]*float64(int(1)<<b))]++
			}

			for cell, got := range counts {
				if got != 1 {
					t.Fatalf(
						"m=%d: the %dx%d elementary interval %d holds %d of the %d points, want exactly 1; "+
							"dimensions 0 and 1 no longer form a (0,%d,2)-net",
						m, 1<<a, 1<<b, cell, got, n, m,
					)
				}
			}
		}
	}
}

// TestSobolIsArchitectureIndependent pins the exact output on every platform.
//
// This package already ships robustness_64bit_test.go and a GOARCH=386 CI leg
// because a GOARCH-dependent sequence once shipped from here. Sobol's exposure
// is different from the sieve's but no smaller: every coordinate is derived
// from a uint32 accumulator, and the one place an int enters is the raw index
// skip+1+i, which is 32 bits wide on 386 and 64 on amd64. A shift written
// against int rather than uint32, or a bounds check that only overflows on one
// of the two, would move points on one architecture and not the other.
//
// The digest is a whole-output checksum rather than a spot check because the
// failure it guards against is not localised — a width bug moves whichever
// points happen to have the affected bits set. The explicit values below it
// exist so that a failure is diagnosable: the digest says something changed,
// the values say what.
func TestSobolIsArchitectureIndependent(t *testing.T) {
	const (
		dims = 39
		n    = 4096
	)

	cases := []struct {
		name string
		opts []Option
		want uint64
	}{
		{"plain", []Option{WithSkip(64)}, 0x0c5fd25ba786c0af},
		{"digitally shifted", []Option{WithSkip(64), WithDigitalShift(12345)}, 0x053d2f26c7c23d13},
	}

	for _, tc := range cases {
		g, err := NewSobol(dims, tc.opts...)
		if err != nil {
			t.Fatal(err)
		}

		if got := pointDigest(g, n); got != tc.want {
			t.Fatalf(
				"%s: FNV-1a over the IEEE-754 bits of %d points x %d dimensions = %#016x, want %#016x; "+
					"the sequence depends on something other than its configuration, "+
					"and int width is the first thing to suspect",
				tc.name, n, dims, got, tc.want,
			)
		}
	}

	// Dyadic rationals, exactly representable, taken from the same run that
	// produced the digests above. Index 1000000 needs 20 bits of raw index, so
	// it exercises the direction numbers well past the initial m_1..m_s that
	// come straight out of the file.
	spot := []struct {
		index int
		dim   int
		want  float64
	}{
		{0, 0, 0.5234375},
		{0, 38, 0.0703125},
		{4095, 0, 0.0238037109375},
		{1000000, 38, 0.4969472885131836},
	}

	g, err := NewSobol(dims, WithSkip(64))
	if err != nil {
		t.Fatal(err)
	}

	for _, s := range spot {
		if got := g.At(s.index)[s.dim]; got != s.want {
			t.Fatalf("At(%d)[%d] = %v, want %v", s.index, s.dim, got, s.want)
		}
	}
}

// pointDigest hashes the IEEE-754 bits of the first n points, so the digest is
// sensitive to every bit of every coordinate rather than to a rounded decimal
// rendering of them.
func pointDigest(g *Sobol, n int) uint64 {
	h := fnv.New64a()
	buf := make([]byte, 8)
	point := make([]float64, g.Dims())

	for i := 0; i < n; i++ {
		g.AtInto(i, point)

		for _, v := range point {
			binary.LittleEndian.PutUint64(buf, math.Float64bits(v))
			_, _ = h.Write(buf)
		}
	}

	return h.Sum64()
}

// TestEmbeddedTableSatisfiesItsInvariants re-derives the invariants here
// instead of calling validateDirectionRows.
//
// Calling the validator would be circular: it would prove only that the
// validator agrees with itself, and a bug that made it accept everything would
// pass. The point of this test is that the embedded bytes — which arrived over
// an unauthenticated connection, see the provenance note in
// sobol_direction.go — hold a real Joe-Kuo table, so the properties have to be
// checked by code that is not the code under test.
func TestEmbeddedTableSatisfiesItsInvariants(t *testing.T) {
	rows, err := parseDirectionNumbers(strings.NewReader(embeddedDirectionNumbers))
	if err != nil {
		t.Fatalf("the embedded direction numbers do not parse: %v", err)
	}

	if want := maxSobolDims - 1; len(rows) != want {
		t.Fatalf(
			"embedded table has %d rows, want %d (dimensions 2..%d; dimension 1 is implicit); "+
				"a short table means the committed asset was truncated",
			len(rows), want, maxSobolDims,
		)
	}

	for i, row := range rows {
		dim := i + 2

		if row.degree < 1 || row.degree > sobolBits {
			t.Fatalf("dimension %d: degree %d out of range", dim, row.degree)
		}

		if len(row.m) != row.degree {
			t.Fatalf("dimension %d: degree %d but %d direction numbers", dim, row.degree, len(row.m))
		}

		// The polynomial must carry its implicit leading and trailing 1 bits,
		// and nothing above the leading one. This is the check on the a-bit
		// convention itself: a wrong shift would put the top coefficient in
		// the wrong place and every polynomial would still look like a
		// polynomial.
		if row.poly&1 == 0 || row.poly>>uint(row.degree) != 1 {
			t.Fatalf(
				"dimension %d: polynomial %#b is not a monic degree-%d polynomial with a nonzero constant term",
				dim, row.poly, row.degree,
			)
		}

		for k, m := range row.m {
			if m%2 == 0 {
				t.Fatalf("dimension %d: m_%d = %d is even, so V_%d loses its leading bit", dim, k+1, m, k+1)
			}

			if uint64(m) >= 1<<uint(k+1) {
				t.Fatalf("dimension %d: m_%d = %d is not below 2^%d, so m<<(32-i) would overflow", dim, k+1, m, k+1)
			}
		}

		// Independent of isPrimitiveOverGF2: step x by hand and find the first
		// power that returns to 1. This is the slow definition of order, which
		// is affordable here because the largest degree in the embedded table
		// is 13.
		if got := naiveOrderOfX(row.poly, row.degree); got != uint64(1)<<uint(row.degree)-1 {
			t.Fatalf(
				"dimension %d: x has order %d in GF(2)[x]/(%#b), want 2^%d-1 = %d; "+
					"the polynomial is not primitive, so the direction-number recurrence does not have full period",
				dim, got, row.poly, row.degree, uint64(1)<<uint(row.degree)-1,
			)
		}
	}

	// Upstream's first three data lines, transcribed by hand from
	// new-joe-kuo-6.21201. If the committed asset were a different file, or
	// the columns were read in a different order, these would not survive.
	first := []struct {
		degree int
		poly   uint64
		m      []uint32
	}{
		{1, 0b11, []uint32{1}},         // d=2  s=1 a=0    -> x + 1
		{2, 0b111, []uint32{1, 3}},     // d=3  s=2 a=1    -> x^2 + x + 1
		{3, 0b1011, []uint32{1, 3, 1}}, // d=4 s=3 a=1    -> x^3 + x + 1
	}

	for i, want := range first {
		got := rows[i]
		if got.degree != want.degree || got.poly != want.poly {
			t.Fatalf("dimension %d: got degree %d polynomial %#b, want degree %d polynomial %#b",
				i+2, got.degree, got.poly, want.degree, want.poly)
		}

		for k, m := range want.m {
			if got.m[k] != m {
				t.Fatalf("dimension %d: m_%d = %d, want %d", i+2, k+1, got.m[k], m)
			}
		}
	}
}

// naiveOrderOfX returns the multiplicative order of x in GF(2)[x]/(poly) by
// stepping, or 0 if x is not a unit. It exists only to give
// TestEmbeddedTableSatisfiesItsInvariants a second opinion on primitivity that
// shares no code with isPrimitiveOverGF2's factored order test.
func naiveOrderOfX(poly uint64, s int) uint64 {
	limit := uint64(1)<<uint(s) - 1

	x := uint64(1)
	for k := uint64(1); k <= limit; k++ {
		x <<= 1
		if x>>uint(s)&1 != 0 {
			x ^= poly
		}

		if x == 1 {
			return k
		}
	}

	return 0
}

// TestPrimitivityTestAgreesWithTheDefinition runs isPrimitiveOverGF2 against
// naiveOrderOfX over every monic polynomial with a nonzero constant term up to
// degree 13, and against the count the theory predicts.
//
// The count is the part worth having. The number of primitive polynomials of
// degree n over GF(2) is phi(2^n-1)/n, so degree 13 has phi(8191)/13 =
// 8190/13 = 630 — and 8191 is prime, which is what makes that arithmetic easy
// to check by hand. Agreeing with the naive order test proves the fast test is
// not wrong; hitting 630 out of 4096 candidates proves it is not vacuously
// permissive, which is the failure mode that would let a corrupted table
// through.
func TestPrimitivityTestAgreesWithTheDefinition(t *testing.T) {
	for s := 1; s <= 13; s++ {
		count := 0

		for a := uint64(0); a < 1<<uint(max(s-1, 0)); a++ {
			poly := 1<<uint(s) | a<<1 | 1

			fast := isPrimitiveOverGF2(poly, s)
			slow := naiveOrderOfX(poly, s) == uint64(1)<<uint(s)-1

			if fast != slow {
				t.Fatalf(
					"degree %d, polynomial %#b: the factored order test says primitive=%v, "+
						"stepping the order says %v; the two must never disagree",
					s, poly, fast, slow,
				)
			}

			if fast {
				count++
			}
		}

		if s == 13 && count != 630 {
			t.Fatalf(
				"found %d primitive polynomials of degree 13, want phi(2^13-1)/13 = 8190/13 = 630; "+
					"a test that accepts too many would not catch a corrupted a field",
				count,
			)
		}
	}
}

// TestValidatorRejectsMalformedTables covers each class of damage the
// validator exists to catch, one row at a time, and insists the error names
// which one.
//
// The rows are minimal rather than realistic: dimension 2 alone is a valid
// table, so each case differs from a good table in exactly the property it is
// testing, and a case that passed for the wrong reason would be obvious.
func TestValidatorRejectsMalformedTables(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "even direction number",
			input: "d s a m_i\n2 1 0 2\n",
			want:  "is even",
		},
		{
			name:  "direction number too large for its position",
			input: "d s a m_i\n2 1 0 3\n",
			want:  "must be below 2^1",
		},
		{
			name:  "non-primitive polynomial",
			input: "d s a m_i\n2 1 0 1\n3 2 1 1 3\n4 4 7 1 3 1 15\n",
			want:  "not primitive",
		},
		{
			name:  "too few direction numbers for the degree",
			input: "d s a m_i\n2 1 0 1\n3 2 1 1\n",
			want:  "needs 2 direction numbers",
		},
		{
			name:  "coefficients wider than the degree allows",
			input: "d s a m_i\n2 1 0 1\n3 2 9 1 3\n",
			want:  "do not fit",
		},
		{
			name:  "degree out of range",
			input: "d s a m_i\n2 33 0 1\n",
			want:  "want 1..32",
		},
		{
			name:  "non-numeric field",
			input: "d s a m_i\n2 1 0 one\n",
			want:  "m_1",
		},
		{
			name:  "empty table",
			input: "d s a m_i\n",
			want:  "no dimensions",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseDirectionNumbers(strings.NewReader(tc.input))
			if err == nil {
				t.Fatalf(
					"malformed direction numbers were accepted; the download is unauthenticated, " +
						"so this validator is the only thing standing between a corrupted file and points nobody can tell are wrong",
				)
			}

			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q, so it does not tell the caller what is wrong with their file", err, tc.want)
			}
		})
	}
}

// TestWithDirectionNumbersRoundTrips feeds the embedded table back in through
// the public option and insists on identical points.
//
// This is what proves there is one parser and not two. If WithDirectionNumbers
// ever grew its own reading path, this is the test that would notice, and it
// would notice before a caller passing upstream's full 21201-dimension file
// found out the hard way.
func TestWithDirectionNumbersRoundTrips(t *testing.T) {
	builtin, err := NewSobol(64, WithSkip(9))
	if err != nil {
		t.Fatal(err)
	}

	supplied, err := NewSobol(64, WithSkip(9), WithDirectionNumbers(strings.NewReader(embeddedDirectionNumbers)))
	if err != nil {
		t.Fatalf("the embedded table was rejected when supplied as a caller's file: %v", err)
	}

	a := make([]float64, 64)
	b := make([]float64, 64)

	for i := 0; i < 2048; i++ {
		builtin.AtInto(i, a)
		supplied.AtInto(i, b)

		for d := range a {
			if a[d] != b[d] {
				t.Fatalf(
					"point %d dimension %d: embedded table gave %v, the same bytes through WithDirectionNumbers gave %v; "+
						"the two are no longer going through the same parser",
					i, d, a[d], b[d],
				)
			}
		}
	}
}

// TestWithDirectionNumbersRejectsMalformedInput pins that a caller's bad file
// fails at construction, with an error, rather than at some later point that
// looks fine.
func TestWithDirectionNumbersRejectsMalformedInput(t *testing.T) {
	// The embedded table with dimension 3's row deleted: every remaining row
	// is individually valid, so only the contiguity of d catches it. This is
	// the corruption with no other symptom.
	lines := strings.SplitN(embeddedDirectionNumbers, "\n", 4)

	broken := lines[0] + "\n" + lines[1] + "\n" + lines[3]
	if _, err := NewSobol(8, WithDirectionNumbers(strings.NewReader(broken))); err == nil {
		t.Fatal("a table with a missing dimension was accepted; every dimension after the gap would use another dimension's polynomial")
	}

	if _, err := NewSobol(8, WithDirectionNumbers(strings.NewReader("not a table at all\n"))); err == nil {
		t.Fatal("a file that is not a direction-number table was accepted")
	}

	// A table that parses but is too small for the requested dimension count
	// must be refused rather than silently reusing dimensions.
	small := "d s a m_i\n2 1 0 1\n3 2 1 1 3\n"

	_, err := NewSobol(8, WithDirectionNumbers(strings.NewReader(small)))
	if err == nil {
		t.Fatal("a 3-dimension table was accepted for an 8-dimensional generator")
	}

	if !strings.Contains(err.Error(), "dims must be <= 3") {
		t.Fatalf("error %q does not say how many dimensions the table actually covers", err)
	}
}

// TestNewSobolRejectsInapplicableRandomizations mirrors the switch in
// NewHalton. A randomization that does not apply must be named back to the
// caller, because the alternative — ignoring it — hands back a deterministic
// sequence to code that is about to average over seeds and will report an
// error estimate of exactly zero.
func TestNewSobolRejectsInapplicableRandomizations(t *testing.T) {
	cases := []struct {
		name string
		opt  Option
		want string
	}{
		{"WithScrambling", WithScrambling(1), "WithScrambling does not apply"},
		{"nested scrambling", func(s *settings) { s.randomize = randomizeNested }, "WithNestedScrambling does not apply"},
		{"Owen scrambling", func(s *settings) { s.randomize = randomizeOwen }, "WithOwenScrambling is not implemented"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewSobol(4, tc.opt)
			if err == nil {
				t.Fatal("an inapplicable randomization was accepted, so the generator is deterministic under a name that promises otherwise")
			}

			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not name the option the caller wrote", err)
			}
		})
	}
}

// TestNewSobolRejectsImpossibleConfigurations covers the arguments that cannot
// produce a sequence at all.
func TestNewSobolRejectsImpossibleConfigurations(t *testing.T) {
	if _, err := NewSobol(0); err == nil {
		t.Fatal("a zero-dimensional generator was accepted")
	}

	_, err := NewSobol(maxSobolDims + 1)
	if err == nil {
		t.Fatalf("more dimensions than the embedded table covers were accepted; the extra dimensions would have to repeat earlier ones")
	}

	if !strings.Contains(err.Error(), "WithDirectionNumbers") {
		t.Fatalf("error %q does not point the caller at the way to get more dimensions", err)
	}
}

// TestShortDestinationPanics matches the Halton case in robustness_test.go: a
// truncated point looks like a plausible position, and NextInto also advances
// the cursor, so absorbing the mistake would consume a sequence point too.
func TestShortDestinationPanicsForSobol(t *testing.T) {
	g, err := NewSobol(5)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		call func()
	}{
		{"NextInto", func() { g.NextInto(make([]float64, 2)) }},
		{"AtInto", func() { g.AtInto(0, nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("a short destination must panic, not be silently truncated")
				}
			}()

			tc.call()
		})
	}
}

// TestSobolRefusesIndicesBeyondItsDirectionNumbers pins that running off the
// end of the 32-bit index space is refused rather than aliased.
//
// This is the Sobol-specific half of the overflow story. Halton's index can
// grow until it exhausts an int; Sobol's cannot exceed 2^32, because there are
// only 32 direction numbers per dimension. The tempting conversion —
// uint32(skip+1+i) — would wrap index 2^32 onto index 0 and hand back the
// origin, then walk the entire sequence again as if it were new. Two far-apart
// indices landing on one point is the defect a low-discrepancy sequence exists
// to avoid, and it would leave no trace.
//
// The case only exists on a 64-bit platform: where int is 32 bits wide,
// skip+1+i cannot reach 2^32 in the first place, which is exactly why the
// GOARCH=386 leg must still compile and run this file.
func TestSobolRefusesIndicesBeyondItsDirectionNumbers(t *testing.T) {
	if bits.UintSize < 64 {
		t.Skip("int is 32 bits wide here, so a raw index of 2^32 is unreachable by construction")
	}

	g, err := NewSobol(3)
	if err != nil {
		t.Fatal(err)
	}

	// The bound is held in a uint64 and converted at run time. Writing
	// g.At(math.MaxUint32) directly would be a constant expression that does
	// not fit an int under GOARCH=386, so the whole file would fail to compile
	// there — which is precisely the platform this file has to run on.
	last := uint64(math.MaxUint32)

	// One below the limit still works: raw index 2^32-1 is the last point.
	if p := g.At(int(last - 1)); p[0] < 0 || p[0] >= 1 {
		t.Fatalf("the last representable point must still be produced, got %v", p[0])
	}

	defer func() {
		if recover() == nil {
			t.Fatal("an index past the direction numbers must be refused, not wrapped back onto index 0")
		}
	}()

	g.At(int(last))
}

// TestSobolRefusesToAdvancePastTheLastPoint is the stateful counterpart. The
// Gray-code recurrence needs the lowest zero bit of the counter, and at
// 2^32-1 there is none: continuing would index one past the direction numbers.
func TestSobolRefusesToAdvancePastTheLastPoint(t *testing.T) {
	g, err := NewSobol(2)
	if err != nil {
		t.Fatal(err)
	}

	// Drive the cursor to the last raw index without walking there.
	g.counter = math.MaxUint32
	g.accumulate(g.counter, g.state)

	dst := make([]float64, 2)

	defer func() {
		if recover() == nil {
			t.Fatal("advancing past the last point must be refused, not allowed to wrap the counter and replay the sequence")
		}
	}()

	g.NextInto(dst)
	g.NextInto(dst)
}

// TestSkipBeyondTheIndexSpaceIsRefusedAtConstruction pins that a skip large
// enough to put point 0 out of range fails where the mistake is, rather than
// at the first draw.
func TestSkipBeyondTheIndexSpaceIsRefusedAtConstruction(t *testing.T) {
	if bits.UintSize < 64 {
		t.Skip("int is 32 bits wide here, so such a skip is not representable")
	}

	skip := uint64(math.MaxUint32)
	if _, err := NewSobol(2, WithSkip(int(skip))); err == nil {
		t.Fatal("a skip that puts point 0 past the 32-bit index space was accepted")
	}
}
