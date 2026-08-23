package qmc

import (
	"math"
	"math/rand"
	"sort"
	"testing"
)

// These tests are white-box because most of what nested scrambling has to get
// right is a property of one digit at a time — of nestedRadicalInverse and of
// the per-node permutation — rather than of a point. That choice costs one
// duplication: the integration harness in integration_test.go is in package
// qmc_test and cannot be reached from here, so nestedIntegrand below repeats
// productIntegrand verbatim. Whether the copy is still faithful is not left to
// inspection: the random-digit figure measured through it, 17.7x, is the 18x
// integration_test.go documents, and a drifted copy would not land there.

// nestedTestBases is every base a 64-dimensional generator uses: 2 through
// 311. Nothing here is base-2 folklore, and the properties that hold for base
// 2 are the ones most likely to hold by accident — base 2 has only two
// permutations, so a base-2-only test cannot tell a uniform draw from almost
// any other construction. The large bases are where a permutation scheme is
// actually distinguishable from a cheap stand-in.
func nestedTestBases() []int { return primesUpTo(64) }

// TestNestedPermutationIsABijection is the property the whole construction
// rests on: a digit map that is not a bijection is not a scramble. It maps two
// digits onto one, so it maps two elementary intervals onto one, and the point
// set stops covering the box — silently, because the points still look like
// plausible points.
//
// Fisher-Yates gives the bijection by construction, so what this really pins
// is the loop bounds and the buffer handling: a shuffle that stops at i > 1
// instead of i > 0, or a scratch slice reused across bases without being
// resliced, both leave something that is still nearly a permutation.
func TestNestedPermutationIsABijection(t *testing.T) {
	for _, base := range nestedTestBases() {
		// Nodes are taken from real walks rather than from a counter: the
		// permutation is derived from a node hash, and hashing 0, 1, 2, ...
		// would exercise a part of the input space the sampler never visits.
		node := nestedRoot(4242, base%37)
		perm := make([]int32, base)

		for step := 0; step < 8; step++ {
			nestedPermutation(node, perm)

			seen := make([]bool, base)

			for x, y := range perm {
				if y < 0 || int(y) >= base {
					t.Fatalf("base %d: digit %d maps to %d, which is not a digit", base, x, y)
				}

				if seen[y] {
					t.Fatalf("base %d: digit %d is hit twice, so the map is not a bijection "+
						"and the scramble folds two elementary intervals onto one", base, y)
				}

				seen[y] = true
			}

			node = nestedChild(node, uint64(step%base))
		}
	}
}

// TestNestedPermutationIsUniform is the reason this scheme replaced the affine
// one, so it is worth a direct check rather than trusting Fisher-Yates by
// reputation.
//
// The affine family x -> a*x+b mod p has p(p-1) members against p! — 20 of 120
// at base 5 — and the correlation tail it produced was traced to exactly that
// thinness. A chi-square over all 120 permutations of base 5, drawn from 60000
// real walk nodes, would have to be blind for a family that thin to slip past:
// a construction covering only 20 of the 120 would score around 240000 against
// the 5% critical value of 145.5 for 119 degrees of freedom.
func TestNestedPermutationIsUniform(t *testing.T) {
	const (
		base     = 5
		draws    = 60000
		critical = 145.5 // chi-square, 119 d.o.f., alpha = 0.05
	)

	factorial := 120

	index := func(perm []int32) int {
		// Lehmer code: a bijection from permutations to 0..base!-1.
		code, radix := 0, 1

		for i := base - 1; i >= 0; i-- {
			smaller := 0

			for j := i + 1; j < base; j++ {
				if perm[j] < perm[i] {
					smaller++
				}
			}

			code += smaller * radix
			radix *= base - i
		}

		return code
	}

	counts := make([]int, factorial)
	perm := make([]int32, base)
	node := nestedRoot(20240823, 2)

	for i := 0; i < draws; i++ {
		nestedPermutation(node, perm)
		counts[index(perm)]++
		node = nestedChild(node, uint64(i%base))
	}

	expected := float64(draws) / float64(factorial)

	chi := 0.0

	for _, c := range counts {
		d := float64(c) - expected
		chi += d * d / expected
	}

	if chi > critical {
		t.Fatalf("chi-square over the %d permutations of base %d from %d nodes = %.1f, want <= %.1f; "+
			"the per-node permutation is not uniform, which is the one thing this construction "+
			"pays an O(base) shuffle for", factorial, base, draws, chi, critical)
	}

	t.Logf("base %d: chi-square over %d permutations from %d nodes = %.1f (critical %.1f)",
		base, factorial, draws, chi, critical)
}

// TestNestedLazyDigitMatchesTheFullShuffle is the seam this construction is
// most likely to come apart at.
//
// nestedRadicalInverse never builds a permutation. It calls nestedDigit, which
// runs the upward Fisher-Yates only as far as the position being asked for and
// stops, on the argument that a later step cannot move an entry it has already
// passed. If that argument is wrong — or if someone flips nestedPermutation
// back to the downward loop, where it is wrong — the two stop agreeing, and
// nothing else here would notice: the lazy answers would still be a bijection
// per node, still nested, still in [0,1). They would simply be a different and
// unexamined map.
//
// Digit 0 is checked along with the rest rather than trusted, because it is
// the case that skips the array entirely and so shares no code with the others.
func TestNestedLazyDigitMatchesTheFullShuffle(t *testing.T) {
	for _, base := range nestedTestBases() {
		perm := make([]int32, base)
		scratch := make([]int32, base)
		node := nestedRoot(20240823, base%29)

		for step := 0; step < 6; step++ {
			nestedPermutation(node, perm)

			for digit := 0; digit < base; digit++ {
				if got := nestedDigit(node, base, digit, scratch); got != perm[digit] {
					t.Fatalf("base %d step %d: nestedDigit(%d) = %d but the full shuffle puts %d there; "+
						"the lazy evaluation is no longer reading the same permutation, so the shipped "+
						"scramble is not the one any other test in this file inspects",
						base, step, digit, got, perm[digit])
				}
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

	scratch := make([]int32, base)

	for i := index; i > 0; i /= base {
		digit := i % base

		result += float64(nestedDigit(node, base, digit, scratch)) * place
		node = nestedChild(node, uint64(digit))
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
		{2, 3, 1, 0.19272131504619178},
		{3, 5, 2, 0.1816423753094948},
		{65, 167, 38, 0.6175675940957736},
		{4160, 311, 63, 0.864728652455974},
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
// had to pass to be worth adding at all, and the gate the switch from affine
// to full permutations had to pass to be worth making.
//
// Nested scrambling costs a permutation draw per digit where random-digit
// scrambling costs a table lookup, and the package already had a scrambling
// that integrates 18x better than Monte Carlo. Anything that does not improve
// on that number is buying nothing with the extra work.
//
// Ten streams is what this test can afford, and ten streams is not enough to
// read the gap to a significant figure: measured at 39 dimensions and n=4096
// it gives 32x for nested against 18x for random-digit, while a variant that
// differed only in the direction of the Fisher-Yates loop read 44x on the same
// ten seeds. Run out to 40 streams the same measurement settles at 41x against
// 24x, and at 80 streams 42x against 26x. Those are the figures the doc
// comments quote. What ten streams does establish reliably is the ordering,
// and the ordering is what is asserted.
//
// The affine construction this replaced measured 53.2x over 10 streams and
// 49.9x over 40, so about a sixth of the integration advantage was given up
// for the correlation tail — see TestNestedCorrelationOverThirtySeeds, which
// is the other half of that trade.
//
// The assertion is the weaker claim that it is not worse than random-digit,
// with a quarter's slack: the size of the gap belongs to these seeds and this
// integrand, while the direction belongs to the construction. A regression
// that merely halved the advantage would still be worth knowing about, but not
// worth a red suite.
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
		t.Fatalf("at %d dims with n=%d over %d streams: nested RMS error %.3e against random-digit "+
			"%.3e; nested scrambling costs about forty times as much per point and is no longer paying for it",
			dims, n, streams, nested, digit)
	}

	if ratio := mc / nested; ratio < wantVsMC {
		t.Fatalf("at %d dims with n=%d over %d streams: nested RMS error %.3e vs MC %.3e = %.1fx, want >= %.0fx; "+
			"the generator is no longer integrating better than independent sampling",
			dims, n, streams, nested, mc, ratio, wantVsMC)
	}

	t.Logf("d=%d n=%d streams=%d: MC %.3e | random-digit %.3e (%.1fx) | nested %.3e (%.1fx)",
		dims, n, streams, mc, digit, mc/digit, nested, mc/nested)
}

// TestNestedCorrelationOverThirtySeeds is the measurement that motivated the
// switch away from the affine construction, kept as the gate that stops it
// coming back by accident.
//
// A single-seed test here would mean nothing. The typical seed was always fine
// under affine too — what the affine restriction did was add a tail to the
// distribution over seeds. At 600 points a large-base coordinate has only its
// first digit varying, and on that digit an affine map is a ramp of another
// slope rather than a scattering; two neighbouring dimensions that drew
// commensurate slopes ramped together much as the unscrambled ones did.
//
// Five seeds would not see the tail either. That is not a supposition: a
// change to this scrambling that was a pure re-instantiation, not a change of
// scheme, once moved a five-seed worst case from 0.40 to 0.12. Thirty is the
// smallest count at which the statistic has been stable here.
//
// Measured over 30 seeds at 39 dimensions and 600 points after skipping 64:
//
//	                     median    p90    worst
//	random-digit          0.093  0.126    0.161
//	nested affine (was)   0.090  0.195    0.373
//	nested full (is)      0.089  0.123    0.141
//
// So the median is asserted against random-digit, where nested has no excuse,
// and the worst case against random-digit's worst with a little slack — which
// is the assertion that has teeth. Under the affine construction the worst was
// 2.3 times random-digit's and this test as written would fail on it, which is
// the point: a ceiling loose enough to pass affine would not be a gate, it
// would be a record.
func TestNestedCorrelationOverThirtySeeds(t *testing.T) {
	const (
		seeds       = 30
		medianSlack = 1.3
		worstSlack  = 1.3
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
		t.Fatalf("median worst adjacent-pair |corr| over %d seeds: nested %.4f against random-digit "+
			"%.4f; nested scrambling is meant to cost nothing on the typical seed, and now it does",
			seeds, nestedMedian, digitMedian)
	}

	if nestedWorst > digitWorst*worstSlack {
		t.Fatalf("worst adjacent-pair |corr| over %d seeds: nested %.4f at dims %d/%d against "+
			"random-digit %.4f; the point of drawing a full permutation at every node instead of an "+
			"affine map is that it has no tail random-digit does not have, and this is that tail "+
			"coming back",
			seeds, nestedWorst, at, at+1, digitWorst)
	}

	t.Logf("worst adjacent-pair |corr| over %d seeds: random-digit median %.4f worst %.4f | "+
		"nested median %.4f worst %.4f at dims %d/%d",
		seeds, digitMedian, digitWorst, nestedMedian, nestedWorst, at, at+1)
}

// BenchmarkAtIntoNested is the third leg of bench_test.go's comparison at 39
// dimensions, kept here because the option it measures lives here. On the
// machine and runs the doc comments quote it was 20881 ns/op against 548 for
// random-digit scrambling and 467 unscrambled, medians of seven — a factor of
// about 38, spent on roughly 484 tree nodes per point, of which 366 are the
// leading-zero tails of the small bases. Under the affine construction this
// replaced, the same digit loop on the same machine ran 4038 ns.
func BenchmarkAtIntoNested(b *testing.B) {
	g, err := NewHalton(benchNestedDims, WithSkip(64), WithNestedScrambling(1))
	if err != nil {
		b.Fatal(err)
	}

	dst := make([]float64, benchNestedDims)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g.AtInto(i, dst)
		sinkNested += dst[0]
	}
}

var sinkNested float64

// benchNestedDims is the package's design point and the dimension count every
// figure in this file's doc comments was measured at.
const benchNestedDims = 39

// BenchmarkNewHaltonNested is the construction cost, which is the one number
// this scheme is cheap at: the constructor derives one root hash per dimension
// and nothing else, where random-digit scrambling builds a permutation per
// dimension and so does work proportional to the sum of the bases. Everything
// nested scrambling costs is deferred to the point where a digit is actually
// rewritten.
func BenchmarkNewHaltonNested(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g, err := NewHalton(benchNestedDims, WithSkip(64), WithNestedScrambling(uint64(i)))
		if err != nil {
			b.Fatal(err)
		}

		sinkNested += float64(g.Dims())
	}
}

// BenchmarkNestedNodeCache measures the cache that nestedRadicalInverse
// deliberately does not have.
//
// The argument for caching a permutation per node is that the tree depth is
// bounded by the digit count, so the set of nodes a run touches is small and
// the O(base) shuffle is paid once each rather than once per visit. The depth
// is bounded. The node count is the thing a cache holds, and it is not: the
// leading-zero tail hangs a fresh chain of nodes below every index's explicit
// digits, and nothing in one is ever visited twice.
//
// So this benchmark counts, rather than assumes. It walks exactly the nodes
// nestedRadicalInverse walks over the workload the other benchmarks use — 39
// dimensions, 4096 points, skip 64 — and reports the visits, the distinct
// nodes, the resulting reuse factor, and the memory a map[uint64][]int32
// holding them would need for its keys, slice headers and digit arrays alone.
// Measured at 1982974 visits against 1544674 distinct nodes: a reuse factor of
// 1.28 and 382 MB, for a scheme whose whole appeal was being cheaper.
// If a future change to the digit loop or the tail bound makes those numbers
// look different, this is where it shows up.
//
// It is a benchmark rather than a test because the numbers are a measurement,
// not a threshold — there is no figure here that should fail a build.
func BenchmarkNestedNodeCache(b *testing.B) {
	const (
		points = 4096
		skip   = 64
	)

	bases := primesUpTo(benchNestedDims)

	var visits, distinct, bytes int

	b.ReportAllocs()
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		visits, distinct, bytes = 0, 0, 0

		for d, base := range bases {
			seen := make(map[uint64]struct{})
			root := nestedRoot(1, d)

			for i := 0; i < points; i++ {
				index := skip + 1 + i
				invBase := 1 / float64(base)
				place := invBase
				node := root
				result := 0.0
				scratch := make([]int32, base)

				for k := index; k > 0; k /= base {
					digit := k % base

					seen[node] = struct{}{}
					visits++
					result += float64(nestedDigit(node, base, digit, scratch)) * place
					node = nestedChild(node, uint64(digit))
					place *= invBase
				}

				for place > 0 && result+float64(base)*place != result {
					seen[node] = struct{}{}
					visits++
					result += float64(nestedDigit(node, base, 0, scratch)) * place
					node = nestedChild(node, 0)
					place *= invBase
				}
			}

			distinct += len(seen)
			// 8 bytes of key, 24 of slice header, 4 per digit, and nothing
			// for the map's own buckets — an underestimate on purpose.
			bytes += len(seen) * (8 + 24 + 4*base)
		}
	}

	b.StopTimer()
	b.ReportMetric(float64(visits), "visits")
	b.ReportMetric(float64(distinct), "distinct-nodes")
	b.ReportMetric(float64(visits)/float64(distinct), "reuse")
	b.ReportMetric(float64(bytes)/1e6, "cache-MB")
}
