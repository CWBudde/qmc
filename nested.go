package qmc

// This file implements nested affine digit scrambling.
//
// Random-digit scrambling (scramble.go) draws one permutation of the digit
// alphabet per dimension and reuses it at every digit position of that
// dimension. Nested scrambling in the sense of Owen (1995) instead draws a
// fresh permutation for each digit position conditionally on the digits above
// it: the digits d_0, d_1, ... of the radical inverse, read outwards from the
// radix point, address a node of a p-ary tree, and the permutation applied to
// digit k is the one hanging off the node reached by d_0..d_(k-1). Two points
// that agree in their first k digits are therefore rewritten by the same k
// permutations and stay together in the same elementary interval of width
// p^-k, which is what keeps the point set low-discrepancy; points that diverge
// earlier are rewritten independently from the divergence downwards, which is
// what removes the correlation one permutation per dimension leaves behind.
//
// A faithful Owen scramble needs a uniform permutation of {0..p-1} at every
// node. The nodes cannot be precomputed — there are p^k of them at depth k —
// so each one costs an O(p) Fisher-Yates shuffle, per digit, per coordinate,
// per point. At base 167 that is not a constant factor worth arguing about,
// it is a different price bracket.
//
// So this is not Owen scrambling and is not called that anywhere. The
// permutation at each node is drawn from the p(p-1) affine maps
//
//	x -> (a*x + b) mod p,   a in [1,p),  b in [0,p)
//
// rather than from all p! permutations. Because p is prime and a is never 0,
// every such map is a bijection of the alphabet for free — no shuffle, no
// permutation array to build or store — and drawing (a,b) is two hashed
// integers. The nesting is genuine; only the family the permutation is drawn
// from is cut down. For p = 2 and p = 3 it is not even cut down: p(p-1) is 2
// and 6, exactly 2! and 3!, so dimensions 0 and 1 get a true Owen scramble.
// The gap opens at base 5, where 20 of the 120 permutations remain, and widens
// from there.
//
// What the restriction costs is measurable. Below, at 39 dimensions, against
// random-digit scrambling and against a full-permutation Owen scramble over
// the same tree — run as a diagnostic to separate the nesting from the affine
// restriction, too slow to ship. Integration is the RMS relative error of
// integration_test.go's product integrand at n=4096, as a factor against plain
// Monte Carlo; correlation is correlation_test.go's worst adjacent-pair |r| at
// 600 points after skipping 64, over 30 seeds (20 for the diagnostic).
//
//	                     integration          adjacent-pair |r|
//	                     10 str.  40 str.   median    p90    worst
//	random-digit          17.7x    24.4x     0.093  0.126    0.161
//	nested affine         53.2x    49.9x     0.090  0.195    0.373
//	full permutations     44.0x        -     0.084      -    0.117
//
// The integration column is what nesting is for and it delivers: the RMS error
// is a third to a half of the random-digit figure, on both stream counts, and
// the ordering does not depend on which seeds are drawn.
//
// The correlation column is where the affine restriction shows. Typically it
// costs nothing — the median is if anything slightly better than random-digit
// — but the distribution has a tail random-digit does not have. At 600 points
// a large-base coordinate has only its first digit varying, and on that digit
// the map is x -> a*x+b mod p: a ramp of a different slope, not a scattering.
// Two dimensions whose slopes happen to be commensurate then ramp together
// much as the unscrambled ones did, and one seed in ten or so puts some pair
// near 0.2, with 0.37 the worst seen in 30. Random-digit scrambling has no
// such failure mode because an arbitrary permutation has no slope. The
// full-permutation row is what identifies the cause: the same nesting over the
// same tree, with the affine restriction lifted, has both the better median
// and no tail.
//
// Reference: Owen, A. B. (1995), "Randomly permuted (t,m,s)-nets and
// (t,s)-sequences".

// WithNestedScrambling turns on nested affine digit scrambling with the given
// seed. Like WithScrambling it makes the generator a randomized QMC sequence:
// still low-discrepancy, but no longer identical across seeds.
//
// It is not a free upgrade over WithScrambling and it is deliberately not the
// default. Measured at 39 dimensions, against random-digit scrambling:
//
//   - Integration is two to three times more accurate. RMS relative error over
//     10 seeds at n=4096 is 53x better than Monte Carlo against random-digit's
//     18x; over 40 seeds, 50x against 24x.
//   - Adjacent-pair correlation at small point counts is usually a shade
//     better and occasionally much worse. Over 30 seeds at 600 points the
//     median is 0.090 against 0.093, but the 90th percentile is 0.195 against
//     0.126 and the worst 0.373 against 0.161. The reason is at the top of
//     nested.go: the affine map turns the first digit into a ramp of another
//     slope rather than scattering it.
//   - It costs roughly eight times as much per point. AtInto at 39 dimensions
//     measured 3968 ns/op against 477 for random-digit scrambling and 394
//     unscrambled, in one run on one machine — the ratio is the part that
//     travels. It is about 484 hashed tree nodes per point, of which 366 are
//     the leading-zero tails of the small bases.
//
// So: reach for it when the budget is spent on an integral or an expectation,
// where the extra digit-level uniformity is what is being paid for. Keep
// WithScrambling when the points are consumed as a design — a parameter sweep,
// a set of trial configurations — where two coordinates ramping together is
// the failure that matters, and where eight times the cost per point buys a
// worse worst case.
func WithNestedScrambling(seed uint64) Option {
	return func(s *settings) {
		s.randomize = randomizeNested
		s.seed = seed
	}
}

// nestedScrambler holds the per-dimension root of the permutation tree.
//
// Only the roots are kept, not the seed they came from. Everything below a
// root is derived by walking the digits, so a retained seed would be a second
// route to the same values — and the first time someone derived a node from
// (seed, dim, depth) instead of by walking, the two would disagree for exactly
// the indices whose digit paths differ, which is most indices but not most
// test cases.
type nestedScrambler struct {
	roots []uint64
}

func newNestedScrambler(seed uint64, dims int) *nestedScrambler {
	n := &nestedScrambler{roots: make([]uint64, dims)}
	for d := range n.roots {
		n.roots[d] = nestedRoot(seed, d)
	}

	return n
}

// nestedRoot derives the tree root for one dimension.
//
// The dimension is mixed into the stream seed rather than drawn from one
// shared stream, for the reason newPermutation does the same: a generator
// built for 5 dimensions and one built for 39 must agree on their first 5.
// Drawing roots consecutively from a single stream would tie every dimension's
// randomization to how many dimensions were asked for, so a caller who widened
// a search from 5 knobs to 39 would silently get a different sequence in the 5
// knobs that had not changed, and the two runs' results would stop being
// comparable with nothing in the API to say so.
func nestedRoot(seed uint64, dim int) uint64 {
	rng := splitMix64(seed ^ (uint64(dim)+1)*0x2545F4914F6CDD1D)
	// The same warm-up as newPermutation: adjacent dimensions differ in few
	// bits of the initial state, and a couple of steps makes that irrelevant.
	rng.next()
	rng.next()

	return rng.next()
}

// nestedChild returns the node reached from node by descending through digit.
//
// The tree is walked rather than addressed. The obvious alternative — hash
// (seed, dim, depth, prefix), with the prefix the integer formed by the digits
// above the current one — is the same idea, but the prefix of a depth-k node
// in base p is a k-digit base-p number and the leading-zero tail below keeps
// extending it: in base 2 it overflows uint64 after 64 digits, and the tail
// alone reaches depth 56. Past that, distinct nodes would alias onto one hash
// and the scramble would quietly stop being nested at the depths where it was
// still contributing digits. Chaining carries the whole path in 64 mixed bits
// at any depth, for the same one mix per digit.
func nestedChild(node uint64, digit uint64) uint64 {
	rng := splitMix64(node ^ (digit+1)*0x9E3779B97F4A7C15)

	return rng.next()
}

// nestedAffine returns the (a, b) of the map x -> (a*x + b) mod base at node,
// with a in [1, base) so that the map is a bijection.
//
// uniformBelow is the package's existing rejection sampler, reused rather than
// cut down to one masked draw: a and b are what the permutation *is*, so a
// modulo bias in them is a bias in the point set, not in a diagnostic.
//
// b is drawn before a because the leading-zero tail needs only b — see
// nestedShift — and drawing it first lets that path stop after one value.
func nestedAffine(node uint64, base int) (uint64, uint64) {
	rng := splitMix64(node)
	b := uniformBelow(&rng, uint64(base))
	a := 1 + uniformBelow(&rng, uint64(base-1))

	return a, b
}

// nestedShift returns just the b of nestedAffine, which is the image of the
// digit 0 under any (a, b). Every digit of the leading-zero tail is 0, so the
// tail never needs a, and three quarters of the nodes visited per point are in
// a tail. Not drawing a there took the 39-dimensional digit loop from 6587 to
// 4753 ns/op.
func nestedShift(node uint64, base int) uint64 {
	rng := splitMix64(node)

	return uniformBelow(&rng, uint64(base))
}

// nestedRadicalInverse applies the nested affine scramble to every digit of
// the base-b radical inverse of index, including the infinitely many leading
// zeros.
//
// Those zeros are the part that is easy to get wrong, and this scheme cannot
// borrow scrambledRadicalInverse's answer for them. There the tail closes to
// invBase*perm[0]/(1-invBase) precisely because one permutation is reused at
// every position, so every zero digit contributes the same perm[0] and the
// series is geometric. Here the permutation changes with depth: writing the
// explicit digits as d_0..d_(m-1), the digits at positions k >= m are all 0
// but each is rewritten by the map hanging off a different node — the chain
// keeps descending through digit 0 — so the tail is
//
//	sum_(j>=0) b_(m+j) * p^-(m+j+1)
//
// with the b's varying. That is not a geometric series and has no closed form.
// It is another base-p number with pseudorandom digits, scaled by p^-m, and it
// is summed rather than solved.
//
// It is summed to exhaustion rather than to a chosen depth. If the next digit
// has place value f, everything still to come is bounded by
//
//	(p-1) * f * (1 + 1/p + 1/p^2 + ...) = p*f
//
// so the loop stops once result + p*f is result: the point past which no
// remaining digit can move the float64, whatever those digits turn out to be.
// That is a statement about the arithmetic rather than a guess at a depth.
// Measured for index 4160 it runs 42 extra digits in base 2 and 6 in base 167,
// worst case over indices 1..20000 being 56 and 8, for 366 of the 484 nodes a
// 39-dimensional point visits.
//
// Stopping earlier would not be a rounding matter. Dropping the tail entirely
// biases every coordinate low by its mean — measured over the 600 indices of
// the correlation test, 0.0013 in base 2 and 0.0005 in base 167 — and, far
// worse, puts short indices back on the coarse lattice that scrambling exists
// to break: without the tail an index with m digits lands exactly on a
// multiple of p^-m, so every one-digit index in base 167 would sit exactly on
// some k/167.
//
// The digits are accumulated straight into the float, most significant first,
// rather than reversed into a uint64 as scrambledRadicalInverse does. That
// sidesteps the aliasing which forces the overflow panic there: a reversal
// that stops early returns the exactly correct value of a shorter, different
// index, while a sum that stops early is short by less than an ulp of what it
// has already accumulated.
//
// The result is clamped strictly below 1 for the same reason as the other two
// inverses: callers are promised [0,1), and a tail of near-maximal digits
// rounds up.
func nestedRadicalInverse(index int, base int, root uint64) float64 {
	if base < 2 || index < 0 {
		return 0
	}

	// The digit map is evaluated in uint64 throughout. a and the digit are
	// both below base, and base is bounded only by the dimension count, so
	// a*x passes 2^31 from base 46341 upwards. A 39-dimensional generator
	// stays far below that, but primesUpTo has no ceiling and int is 32 bits
	// on the 386 build this package tests on, where the product would wrap
	// into a different digit and the sequence would depend on GOARCH.
	wide := uint64(base)

	invBase := 1 / float64(base)
	place := invBase
	node := root
	result := 0.0

	for i := index; i > 0; i /= base {
		digit := uint64(i % base)

		a, b := nestedAffine(node, base)
		result += float64((a*digit+b)%wide) * place
		node = nestedChild(node, digit)
		place *= invBase
	}

	for place > 0 && result+float64(base)*place != result {
		result += float64(nestedShift(node, base)) * place
		node = nestedChild(node, 0)
		place *= invBase
	}

	if result >= oneMinusEpsilon {
		return oneMinusEpsilon
	}

	return result
}
