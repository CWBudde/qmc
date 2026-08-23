package qmc

import (
	"math"
	"math/rand"
	"sort"
	"testing"
)

// These tests are white-box because most of what nested affine scrambling has
// to get right is a property of one digit at a time — of nestedRadicalInverse
// and of the affine map — rather than of a point. That choice costs one
// duplication: the integration harness in integration_test.go is in package
// qmc_test and cannot be reached from here, so nestedIntegrand below repeats
// productIntegrand verbatim. Whether the copy is still faithful is not left to
// inspection: the random-digit figure measured through it, 17.7x, is the 18x
// integration_test.go documents, and a drifted copy would not land there.

// nestedTestBases is every base a 64-dimensional generator uses: 2 through
// 311. Nothing here is base-2 folklore, and the properties that hold for base
// 2 are the ones most likely to hold by accident — for p = 2 the affine family
// is all 2! permutations, so a base-2-only test would not notice the
// restriction this scheme is built on at all.
func nestedTestBases() []int { return primesUpTo(64) }

// TestNestedAffineMapIsABijection is the property the whole construction rests
// on: a digit map that is not a bijection is not a scramble. It maps two
// digits onto one, so it maps two elementary intervals onto one, and the point
// set stops covering the box — silently, because the points still look like
// plausible points.
//
// The bijection here is free rather than constructed: p is prime and a is
// drawn from [1,p), so x -> a*x+b mod p is invertible for arithmetic reasons.
// That is exactly why it is worth testing. There is no shuffle to inspect and
// no permutation array to validate, so a wrong bound on a — a in [0,p), the
// obvious off-by-one — would produce the constant map for a = 0 and nothing in
// the construction would object.
func TestNestedAffineMapIsABijection(t *testing.T) {
	for _, base := range nestedTestBases() {
		// Nodes are taken from real walks rather than from a counter: the
		// values a and b are drawn from are node hashes, and testing hashes of
		// 0, 1, 2, ... would exercise a part of the input space the sampler
		// never visits.
		node := nestedRoot(4242, base%37)

		for step := 0; step < 8; step++ {
			a, b := nestedAffine(node, base)
			if a < 1 || a >= uint64(base) {
				t.Fatalf("base %d: multiplier %d is outside [1,%d); a = 0 collapses the digit map to a constant",
					base, a, base)
			}

			if b >= uint64(base) {
				t.Fatalf("base %d: shift %d is outside [0,%d), so it is not a digit", base, b, base)
			}

			seen := make([]bool, base)
			for x := 0; x < base; x++ {
				y := (a*uint64(x) + b) % uint64(base)
				if seen[y] {
					t.Fatalf("base %d with a=%d b=%d: digit %d is hit twice, so the map is not a bijection "+
						"and the scramble folds two elementary intervals onto one", base, a, b, y)
				}

				seen[y] = true
			}

			node = nestedChild(node, uint64(step%base))
		}
	}
}

// TestNestedScramblingStaysInElementaryIntervals is the important one.
//
// Nesting is what lets a stronger randomization stay low-discrepancy. Two
// indices that agree in their first k base-p digits — that is, two indices
// congruent modulo p^k — have radical inverses in the same elementary interval
// of width p^-k, and the scramble has to keep them there: it must rewrite
// those k digits identically for both, which it does exactly because the
// permutations for positions 0..k-1 hang off nodes addressed by the digits
// above them.
//
// If this fails the sequence is no longer a (t,s)-sequence in any base and the
// package is shipping noise with a low-discrepancy label on it. It is also the
// property that fails first if the node chain stops depending on the digit
// path — drop the digit from nestedChild and every position gets one
// permutation again, which is a legal scramble but not this one.
func TestNestedScramblingStaysInElementaryIntervals(t *testing.T) {
	for _, base := range []int{2, 3, 5, 13, 167, 311} {
		root := nestedRoot(99, 7)

		pk := 1
		for k := 1; k <= 3 && pk < 1<<20; k++ {
			pk *= base

			for i := 0; i < 200; i++ {
				for _, t2 := range []int{1, 2, 7} {
					a := nestedRadicalInverse(i, base, root)
					b := nestedRadicalInverse(i+t2*pk, base, root)

					if int(a*float64(pk)) != int(b*float64(pk)) {
						t.Fatalf("base %d: indices %d and %d share their first %d digits but scramble to "+
							"%v and %v, which are in different intervals of width %v; the nesting is broken "+
							"and the point set is no longer low-discrepancy",
							base, i, i+t2*pk, k, a, b, 1/float64(pk))
					}
				}
			}
		}
	}
}

// TestNestedPermutationsDependOnThePrefix is the other half of nesting, and
// the half the elementary-interval test above cannot see.
//
// Rewriting digit k by a permutation that depends only on k — one per position
// per dimension, ignoring the digits above — keeps every elementary interval
// exactly where it was, so the test above passes with the nesting removed. It
// would be a legal scramble, just a weaker one than random-digit scrambling
// pays for: what nesting buys is that two points which part company at digit k
// are rewritten independently from there down, and that is this test.
//
// Indices d0 + p, for d0 = 0..p-1, all carry the digit 1 in position 1 and
// differ only in position 0. Under a prefix-independent scheme their scrambled
// second digit would be one and the same value; under nesting each descends
// through a different child of the root and lands wherever that child's map
// sends it. The count of distinct values is compared against p/3, well below
// the p(1-1/e) ≈ 0.63p expected from independent draws but far above the 1 a
// prefix-independent scheme produces.
func TestNestedPermutationsDependOnThePrefix(t *testing.T) {
	for _, base := range []int{5, 13, 47, 167, 311} {
		root := nestedRoot(31337, 4)

		seen := make(map[int]bool, base)

		for d0 := 0; d0 < base; d0++ {
			v := nestedRadicalInverse(d0+base, base, root)
			// The explicit digits are two, so floor(v * base^2) is exactly
			// s0*base + s1 — the tail contributes less than one unit of the
			// last place value by construction.
			seen[int(v*float64(base)*float64(base))%base] = true
		}

		if len(seen) <= base/3 {
			t.Fatalf("base %d: the second digit takes only %d distinct values over the %d indices that "+
				"differ only in their first digit; the permutation at a position is not depending on the "+
				"digits above it, so the scramble is one permutation per position and not nested",
				base, len(seen), base)
		}
	}
}

// TestNestedCoordinatesAreInTheUnitInterval covers the range the package
// promises, with the short indices called out separately.
//
// Indices 1, 2 and 3 are where the leading-zero tail is the whole coordinate
// in a large base: index 1 in base 167 has one explicit digit, so everything
// below 1/167 comes from the tail. They are also the indices a caller sees
// first, and a generator that returns something outside [0,1) exactly at the
// start is a generator whose first bug report is about an index-out-of-range
// panic in someone else's bucket table.
func TestNestedCoordinatesAreInTheUnitInterval(t *testing.T) {
	for _, base := range nestedTestBases() {
		root := nestedRoot(11, base%13)

		for _, index := range []int{0, 1, 2, 3, 4, 166, 167, 168, 4160, 1 << 30} {
			v := nestedRadicalInverse(index, base, root)
			if !(v >= 0 && v < 1) {
				t.Fatalf("base %d index %d scrambles to %v, which is not in [0,1)", base, index, v)
			}
		}
	}

	// The same promise through the public API, where the tail is one of 39
	// coordinates and a violation would be easy to miss.
	g, err := NewHalton(39, WithSkip(64), WithNestedScrambling(3))
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2000; i++ {
		for d, v := range g.At(i) {
			if !(v >= 0 && v < 1) {
				t.Fatalf("point %d coordinate %d = %v, which is not in [0,1)", i, d, v)
			}
		}
	}
}

// nestedInverseWithoutTail is nestedRadicalInverse with the leading-zero tail
// deleted, which is the mistake TestNestedIncludesTheLeadingZeroTail exists to
// catch. It is kept here, next to the test, so that deleting the tail from the
// real implementation makes the two agree and the test fail loudly rather than
// making the test quietly compare something to itself.
func nestedInverseWithoutTail(index, base int, root uint64) float64 {
	invBase := 1 / float64(base)
	place := invBase
	node := root
	result := 0.0

	for i := index; i > 0; i /= base {
		digit := uint64(i % base)

		a, b := nestedAffine(node, base)
		result += float64((a*digit+b)%uint64(base)) * place
		node = nestedChild(node, digit)
		place *= invBase
	}

	return result
}

// TestNestedIncludesTheLeadingZeroTail pins the tail by the shape of the
// damage dropping it does, not by an arithmetic identity — there is no closed
// form for it here to compare against.
//
// Without the tail an index with m explicit digits lands exactly on a multiple
// of p^-m. In base 167 that means all 166 one-digit indices sit exactly on the
// coarse lattice k/167, which is the very defect scrambling is applied to
// remove: the coordinate is a relabelled ramp again. With the tail none of
// them do. The second measurement is the bias: the tail is a positive quantity
// that is always dropped, so leaving it out shifts every coordinate low, by a
// measured 0.0013 in base 2 and 0.0005 in base 167 over the 600 indices of the
// correlation test.
func TestNestedIncludesTheLeadingZeroTail(t *testing.T) {
	const base = 167

	root := nestedRoot(5, 38)

	onLattice, offLattice := 0, 0

	for index := 1; index < base; index++ {
		truncated := nestedInverseWithoutTail(index, base, root)
		if math.Abs(truncated*base-math.Round(truncated*base)) > 1e-9 {
			t.Fatalf("index %d: the tail-free value %v is not on the 1/%d lattice; the reference variant "+
				"this test compares against is no longer the mistake it is meant to model", index, truncated, base)
		}

		onLattice++

		full := nestedRadicalInverse(index, base, root)
		if math.Abs(full*base-math.Round(full*base)) > 1e-9 {
			offLattice++
		}
	}

	if offLattice != onLattice {
		t.Fatalf("%d of %d one-digit indices in base %d still land exactly on the 1/%d lattice; "+
			"the leading-zero tail is not being summed, so short indices are back on the coarse "+
			"lattice that scrambling exists to break", onLattice-offLattice, onLattice, base, base)
	}

	for _, tc := range []struct{ base, wantBias int }{{2, 13}, {167, 5}} {
		root := nestedRoot(1, 3)

		bias := 0.0
		for index := corrSkip + 1; index <= corrSkip+corrPoints; index++ {
			bias += nestedRadicalInverse(index, tc.base, root) - nestedInverseWithoutTail(index, tc.base, root)
		}

		bias /= corrPoints
		if bias <= 0 {
			t.Fatalf("base %d: dropping the tail would shift the mean coordinate by %v; the tail is a sum of "+
				"non-negative digits and can only bias low, so a non-positive figure means it is not there",
				tc.base, bias)
		}

		// The measured figures are 0.001338 (base 2) and 0.000524 (base 167).
		// They are asserted only to their leading digit, since they depend on
		// the seed, but the order of magnitude is the point: it is the mean of
		// p^-m/2 over the index lengths in this range, not a rounding error.
		if got := int(math.Round(bias * 10000)); got != tc.wantBias {
			t.Fatalf("base %d: mean tail contribution %.6f, want about %.4f", tc.base, bias, float64(tc.wantBias)/10000)
		}
	}
}

// TestNestedIsDeterministic pins that the same configuration gives the same
// points. Randomized QMC is only reproducible if the randomization is, and the
// node chain is walked afresh for every coordinate of every point — there is
// no permutation table to keep it honest, so a stray dependence on evaluation
// order would not show up as a crash, only as a run that cannot be repeated.
func TestNestedIsDeterministic(t *testing.T) {
	a, err := NewHalton(17, WithSkip(64), WithNestedScrambling(777))
	if err != nil {
		t.Fatal(err)
	}

	b, err := NewHalton(17, WithSkip(64), WithNestedScrambling(777))
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 500; i++ {
		p, q := a.At(i), b.At(i)
		for d := range p {
			if p[d] != q[d] {
				t.Fatalf("two generators with seed 777 disagree at point %d coordinate %d: %v vs %v",
					i, d, p[d], q[d])
			}
		}
	}

	// Out of order and interleaved with the stateful cursor, because At is
	// documented to be independent of how many points have been drawn.
	a.Reset()
	a.Next()

	if got, want := a.At(400)[9], b.At(400)[9]; got != want {
		t.Fatalf("At(400) returned %v after the cursor moved, want %v; At must not depend on the cursor", got, want)
	}
}

// TestNestedIsArchitectureIndependent pins exact float64 values.
//
// The 386 CI leg exists because a GOARCH-dependent sequence once shipped from
// this package (see TestSieveIsArchitectureIndependent). This construction has
// its own way in: the digit map is a*x + b mod p, and a and x are each below
// p, so the product needs 64 bits from base 46341 upwards — primesUpTo has no
// ceiling, and on a 32-bit build an int multiply there would wrap into a
// different digit. Pinning values rather than the multiply keeps the test
// pointed at the observable rather than the mechanism.
func TestNestedIsArchitectureIndependent(t *testing.T) {
	cases := []struct {
		index, base, dim int
		want             float64
	}{
		{1, 2, 0, 0.8610418542560146},
		{2, 3, 1, 0.5260546483795254},
		{3, 5, 2, 0.1816423753094948},
		{65, 167, 38, 0.6475077138562525},
		{4160, 311, 63, 0.7826575407015464},
	}
	for _, tc := range cases {
		got := nestedRadicalInverse(tc.index, tc.base, nestedRoot(20240823, tc.dim))
		if got != tc.want {
			t.Fatalf("index %d base %d dim %d scrambles to %v, want exactly %v; the sequence now depends on "+
				"the build, not only on the seed", tc.index, tc.base, tc.dim, got, tc.want)
		}
	}
}

// TestNestedAgreesAcrossDimensionCounts pins the guarantee newPermutation
// makes for random-digit scrambling, for the same reason and by the same
// means: the tree root is derived from (seed, dim) rather than drawn from one
// shared stream.
//
// Without it, widening a search from 5 knobs to 39 would change the sequence
// in the 5 knobs that had not changed, and the earlier run's results would
// stop being comparable to the later one's with nothing in the API to say so.
func TestNestedAgreesAcrossDimensionCounts(t *testing.T) {
	narrow, err := NewHalton(5, WithSkip(64), WithNestedScrambling(2024))
	if err != nil {
		t.Fatal(err)
	}

	wide, err := NewHalton(39, WithSkip(64), WithNestedScrambling(2024))
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 300; i++ {
		n, w := narrow.At(i), wide.At(i)
		for d := range n {
			if n[d] != w[d] {
				t.Fatalf("point %d coordinate %d: 5-dimensional generator gives %v, 39-dimensional gives %v; "+
					"widening a search must not move the dimensions it did not touch", i, d, n[d], w[d])
			}
		}
	}
}

// nestedIntegrand is productIntegrand from integration_test.go, repeated
// because that file is in package qmc_test and these tests are white-box. See
// the note at the top of this file.
func nestedIntegrand(x []float64) float64 {
	v := 1.0
	for k, xk := range x {
		v *= 1 + (1/float64(k+1))*(xk-0.5)
	}

	return v
}

func nestedRMSError(t *testing.T, dims, n, streams int, randomize func(uint64) Option) float64 {
	t.Helper()

	sumSq := 0.0
	point := make([]float64, dims)

	for seed := 1; seed <= streams; seed++ {
		g, err := NewHalton(dims, WithSkip(64), randomize(uint64(seed)))
		if err != nil {
			t.Fatal(err)
		}

		sum := 0.0

		for i := 0; i < n; i++ {
			g.AtInto(i, point)
			sum += nestedIntegrand(point)
		}

		e := sum/float64(n) - 1.0
		sumSq += e * e
	}

	return math.Sqrt(sumSq / float64(streams))
}

func nestedMCError(dims, n, streams int) float64 {
	rng := rand.New(rand.NewSource(20240823)) //nolint:gosec // statistical baseline, not cryptography

	sumSq := 0.0
	point := make([]float64, dims)

	for s := 0; s < streams; s++ {
		sum := 0.0

		for i := 0; i < n; i++ {
			for d := range point {
				point[d] = rng.Float64()
			}

			sum += nestedIntegrand(point)
		}

		e := sum/float64(n) - 1.0
		sumSq += e * e
	}

	return math.Sqrt(sumSq / float64(streams))
}

// TestNestedIntegratesAtLeastAsWellAsDigitScrambling is the gate the option
// had to pass to be worth adding at all.
//
// Nested scrambling costs a hash per digit where random-digit scrambling costs
// a table lookup, and the package already had a scrambling that integrates 18x
// better than Monte Carlo. Anything that does not improve on that number is
// buying nothing with the extra work. Measured at 39 dimensions and n=4096,
// nested affine reaches 53x against random-digit's 18x over 10 seeds, and 50x
// against 24x over 40 - the RMS error is a third to a half of it, and the
// ordering holds on both stream counts rather than resting on one draw.
//
// The assertion is the weaker claim that it is not worse, with a quarter's
// slack: the size of the gap belongs to these seeds and this integrand, while
// the direction belongs to the construction. A regression that merely halved
// the advantage would still be worth knowing about, but not worth a red suite.
func TestNestedIntegratesAtLeastAsWellAsDigitScrambling(t *testing.T) {
	const (
		dims     = 39
		n        = 4096
		streams  = 10
		slack    = 1.25
		wantVsMC = 5.0
	)

	mc := nestedMCError(dims, n, streams)
	digit := nestedRMSError(t, dims, n, streams, WithScrambling)
	nested := nestedRMSError(t, dims, n, streams, WithNestedScrambling)

	if nested <= 0 {
		t.Fatalf("nested RMS error is %g; an exactly-zero error means the estimator collapsed, "+
			"not that the generator is perfect", nested)
	}

	if nested > digit*slack {
		t.Fatalf("at %d dims with n=%d over %d streams: nested affine RMS error %.3e against random-digit "+
			"%.3e; nested scrambling costs about eight times as much per point and is no longer paying for it",
			dims, n, streams, nested, digit)
	}

	if ratio := mc / nested; ratio < wantVsMC {
		t.Fatalf("at %d dims with n=%d over %d streams: nested RMS error %.3e vs MC %.3e = %.1fx, want >= %.0fx; "+
			"the generator is no longer integrating better than independent sampling",
			dims, n, streams, nested, mc, ratio, wantVsMC)
	}

	t.Logf("d=%d n=%d streams=%d: MC %.3e | random-digit %.3e (%.1fx) | nested affine %.3e (%.1fx)",
		dims, n, streams, mc, digit, mc/digit, nested, mc/nested)
}

// TestNestedCorrelationHasAHeavierTail records the side of the comparison that
// does not favour this option, and keeps it from getting worse unnoticed.
//
// The typical seed is fine - better than random-digit, if anything - so a
// single-seed test here would report good news and mean nothing. What the
// affine restriction actually does is add a tail to the distribution over
// seeds. At 600 points a large-base coordinate has only its first digit
// varying, and on that digit the map is x -> a*x+b mod p: a ramp of another
// slope, not a scattering. When two neighbouring dimensions draw commensurate
// slopes they ramp together much as the unscrambled ones did. Measured over 30
// seeds: median 0.090 against random-digit's 0.093, but 90th percentile 0.195
// against 0.126 and worst 0.373 against 0.161. Lifting the affine restriction
// removes the tail - a full-permutation nested scramble over the same tree
// measured 0.084 median and 0.117 worst - which is what places the blame on
// the restriction rather than on the nesting.
//
// So the median is asserted against random-digit, where nested has no excuse,
// and the worst case is held under a ceiling the measured 0.373 has room under
// but the unscrambled defect (0.81) does not.
func TestNestedCorrelationHasAHeavierTail(t *testing.T) {
	const (
		seeds       = 30
		medianSlack = 1.3
		ceiling     = 0.5
	)

	measure := func(randomize func(uint64) Option) (float64, float64, int) {
		vals := make([]float64, 0, seeds)
		worst, at := 0.0, 0

		for seed := uint64(1); seed <= seeds; seed++ {
			g, err := NewHalton(corrDims, WithSkip(corrSkip), randomize(seed))
			if err != nil {
				t.Fatal(err)
			}

			w, pair := worstAdjacentCorrelation(draw(g, corrPoints))
			if w > worst {
				worst, at = w, pair
			}

			vals = append(vals, w)
		}

		sort.Float64s(vals)

		return vals[len(vals)/2], worst, at
	}

	digitMedian, digitWorst, _ := measure(WithScrambling)

	nestedMedian, nestedWorst, at := measure(WithNestedScrambling)
	if nestedMedian > digitMedian*medianSlack {
		t.Fatalf("median worst adjacent-pair |corr| over %d seeds: nested affine %.4f against random-digit "+
			"%.4f; nested scrambling is meant to cost nothing on the typical seed, and now it does",
			seeds, nestedMedian, digitMedian)
	}

	if nestedWorst > ceiling {
		t.Fatalf("worst adjacent-pair |corr| over %d seeds = %.4f at dims %d/%d, want <= %.2f; the affine "+
			"first digit is a ramp, and this is how far that is allowed to go before the option is "+
			"actively misleading for the callers most likely to reach for a stronger scrambling",
			seeds, nestedWorst, at, at+1, ceiling)
	}

	t.Logf("worst adjacent-pair |corr| over %d seeds: random-digit median %.4f worst %.4f | "+
		"nested affine median %.4f worst %.4f at dims %d/%d",
		seeds, digitMedian, digitWorst, nestedMedian, nestedWorst, at, at+1)
}

// BenchmarkAtIntoNested is the third leg of bench_test.go's comparison at 39
// dimensions, kept here because the option it measures lives here. On the
// machine and run the doc comments quote it was 3968 ns/op against 477 for
// random-digit scrambling and 394 unscrambled — a factor of about 8, spent on
// roughly 484 node hashes per point, of which 366 are the leading-zero tails
// of the small bases.
func BenchmarkAtIntoNested(b *testing.B) {
	g, err := NewHalton(39, WithSkip(64), WithNestedScrambling(1))
	if err != nil {
		b.Fatal(err)
	}

	dst := make([]float64, 39)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g.AtInto(i, dst)
		sinkNested += dst[0]
	}
}

var sinkNested float64
