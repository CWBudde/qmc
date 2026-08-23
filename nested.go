package qmc

// This file implements nested digit scrambling.
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
// The permutation at each node is a genuine uniform draw from all p! of them,
// by the same Fisher-Yates over the same rejection sampler random-digit
// scrambling uses. There are p^k nodes at depth k so none of them can be
// precomputed, and materialising one costs O(p); nestedDigit explains how that
// is avoided without giving up the uniformity, and nestedRadicalInverse
// explains why the obvious cache is not the way to avoid it.
//
// # This used to be an affine construction, and the swap is measured
//
// Until this version the permutation at a node was drawn not from all p! but
// from the p(p-1) affine maps x -> (a*x + b) mod p, a in [1,p). That is free
// of shuffles — p is prime and a is never 0, so every such map is a bijection
// by arithmetic — and it made the scheme about five times cheaper than it is
// now. What it cost was the shape of the correlation distribution. At 600
// points a large-base coordinate has only its first digit varying, and on that
// digit an affine map is a ramp of another slope rather than a scattering; two
// neighbouring dimensions that draw commensurate slopes then ramp together
// much as the unscrambled ones did. The affine family is also a vanishingly
// thin slice of the permutations it stands in for — 20 of 120 at base 5, and
// worse from there.
//
// Both constructions measured at 39 dimensions on this machine, against
// random-digit scrambling. Integration is the RMS relative error of the
// product integrand at n=4096 as a factor against plain Monte Carlo, over 10
// and over 40 scrambling seeds; correlation is the worst adjacent-pair |r| at
// 600 points after skipping 64, over 30 seeds; cost is one 39-dimensional
// point, median of seven runs.
//
//	                     integration          adjacent-pair |r|       one point
//	                     10 str.  40 str.   median    p90    worst        ns
//	random-digit          17.7x    24.4x     0.093  0.126    0.161        548
//	nested affine (was)   53.2x    49.9x     0.090  0.195    0.373       4038
//	nested full (is)      31.9x    41.1x     0.089  0.123    0.141      20881
//
// The affine row's cost is that construction's digit loop timed on its own,
// since it is no longer reachable through AtInto; the two harnesses agree to
// within half a percent on the row that both can measure.
//
// The correlation tail is gone: 0.141 worst against affine's 0.373, now under
// random-digit's own 0.161 rather than more than double it, and the 90th
// percentile has come back from 0.195 to 0.123. That is the whole reason for
// the change, and it is what the affine restriction was suspected of causing.
//
// Integration is worse than affine and better than everything else. The
// 10-seed figures are too noisy to read — the same 10 seeds gave 44.0x for a
// full-permutation variant that differed only in the direction of the shuffle
// — so the 40-seed column is the one to compare: 41.1x against affine's 49.9x
// and random-digit's 24.4x, with 80 seeds giving 41.9x against 26.2x. So about
// a sixth of the integration advantage was paid for the tail. That is the
// trade this change makes, and it is deliberate: the callers most likely to
// reach for a stronger scrambling are the ones a 0.37 correlation would
// mislead.
//
// The cost column is the part that is not a trade. Five times the affine
// price, and thirty-eight times random-digit's, buys the uniformity; there is
// no version of a full permutation that is as cheap as two hashed integers.
//
// Reference: Owen, A. B. (1995), "Randomly permuted (t,m,s)-nets and
// (t,s)-sequences".

// WithNestedScrambling turns on nested digit scrambling with the given seed.
// Like WithScrambling it makes the generator a randomized QMC sequence: still
// low-discrepancy, but no longer identical across seeds.
//
// It is not a free upgrade over WithScrambling and it is deliberately not the
// default. Measured at 39 dimensions, against random-digit scrambling:
//
//   - Integration is roughly twice as accurate. RMS relative error at n=4096
//     is 41x better than Monte Carlo over 40 seeds against random-digit's 24x,
//     and 42x against 26x over 80. Over 10 seeds the measurement is too noisy
//     to quote — it read 32x against 18x, and a variant differing only in the
//     direction of a shuffle read 44x on the same seeds.
//   - Adjacent-pair correlation at small point counts is a shade better rather
//     than worse. Over 30 seeds at 600 points the median is 0.089 against
//     0.093, the 90th percentile 0.123 against 0.126, and the worst 0.141
//     against 0.161.
//   - It costs about forty times as much per point. AtInto at 39 dimensions
//     measured 20881 ns/op against 548 for random-digit scrambling and 467
//     unscrambled, medians of seven runs on one machine — the ratio is the
//     part that travels. It is about 484 tree nodes per point, of which 366
//     are the leading-zero tails of the small bases, and each one now costs a
//     draw from a uniform permutation rather than a table lookup.
//
// So: reach for it when the budget is spent on an integral or an expectation,
// where the extra digit-level uniformity is what is being paid for, and when
// the integrand rather than the point count is what the wall clock is going
// on. Keep WithScrambling when the points are cheap to consume — a parameter
// sweep, a set of trial configurations — where forty times the cost per point
// buys an improvement in a statistic that was already acceptable.
//
// Before this version this option drew its per-node permutations from the
// affine family x -> a*x+b mod p rather than from all p!. It integrated
// somewhat better and had a much heavier correlation tail; the points it
// produces have changed. See the top of nested.go for both measurements.
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

// nestedPermutation writes the node's uniform permutation of
// {0..len(perm)-1} into perm.
//
// This is newPermutation's Fisher-Yates over newPermutation's rejection
// sampler — uniformBelow, unchanged, because a modulo bias here is a bias in
// the point set rather than in a diagnostic — with two deliberate differences.
// It is seeded from a node hash rather than from (seed, dim), and it writes
// into a caller-supplied buffer rather than allocating, because it runs once
// per digit rather than once per dimension.
//
// The third difference is the load-bearing one: the swap loop runs upwards,
// i = 0, 1, ... n-1 with j drawn uniformly from [i, n), where newPermutation
// runs downwards. Both orders give a uniform permutation. Only the upward one
// finishes position i at step i and never touches it again, which is what lets
// nestedDigit evaluate a single entry without running the rest — see there.
// The two functions must agree entry for entry, and
// TestNestedLazyDigitMatchesTheFullShuffle is what holds them together.
func nestedPermutation(node uint64, perm []int32) {
	for i := range perm {
		perm[i] = int32(i)
	}

	rng := splitMix64(node)
	for i := 0; i < len(perm); i++ {
		j := i + int(uniformBelow(&rng, uint64(len(perm)-i)))
		perm[i], perm[j] = perm[j], perm[i]
	}
}

// nestedDigit returns the image of digit under the node's permutation, without
// building the rest of it.
//
// The permutation is defined by the whole shuffle; this evaluates it lazily.
// Because the upward Fisher-Yates in nestedPermutation settles position i at
// step i and every later step draws from [i+1, n), the entry at position digit
// is final once step digit has run, and the steps after it cannot move it. So
// the answer is the same one nestedPermutation would write, from the same
// prefix of the same stream — this is not an approximation of the permutation,
// it is the permutation, read at one point.
//
// That matters for cost, not for tidiness. Measured at 39 dimensions, 366 of
// the 484 nodes a point visits are in the leading-zero tails, and every one of
// those asks for digit 0. Digit 0 is settled by the very first draw, out of an
// identity array, so it needs no array at all and no shuffle: one uniformBelow
// and done. Over the same 39-dimensional digit loop, medians of seven runs,
// evaluating the full permutation at every node costs 129740 ns against this
// version's 20790. The difference is almost entirely the 64-bit division
// uniformBelow does per draw: the full shuffle pays it base-1 times per node,
// digit 0 pays it once, and a digit d pays it d+1 times.
//
// scratch must have room for base entries; only positions 0..digit and the
// swap partners drawn above them are touched, but the identity fill is over
// the whole of it because a swap partner can be anywhere.
func nestedDigit(node uint64, base int, digit int, scratch []int32) int32 {
	rng := splitMix64(node)

	if digit == 0 {
		return int32(uniformBelow(&rng, uint64(base)))
	}

	perm := scratch[:base]
	for i := range perm {
		perm[i] = int32(i)
	}

	for i := 0; i <= digit; i++ {
		j := i + int(uniformBelow(&rng, uint64(base-i)))
		perm[i], perm[j] = perm[j], perm[i]
	}

	return perm[digit]
}

// nestedPermStack is the size of the on-stack scratch array the digit loop
// shuffles into. 512 covers every base a 97-dimensional generator uses (the
// 97th prime is 509), which is comfortably past the dimension counts this
// package is built for, at 2 KiB of frame. Above it the buffer is allocated
// once per coordinate — never once per node, which is the allocation that
// would actually hurt.
const nestedPermStack = 512

// nestedRadicalInverse applies the nested scramble to every digit of the
// base-b radical inverse of index, including the infinitely many leading
// zeros.
//
// Those zeros are the part that is easy to get wrong, and this scheme cannot
// borrow scrambledRadicalInverse's answer for them. There the tail closes to
// invBase*perm[0]/(1-invBase) precisely because one permutation is reused at
// every position, so every zero digit contributes the same perm[0] and the
// series is geometric. Here the permutation changes with depth: writing the
// explicit digits as d_0..d_(m-1), the digits at positions k >= m are all 0
// but each is rewritten by a different node's permutation — the chain keeps
// descending through digit 0 — so the tail is
//
//	sum_(j>=0) s_(m+j) * p^-(m+j+1)
//
// with the s's varying. That is not a geometric series and has no closed form.
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
// the correlation test, 0.0014 in base 2 and 0.0005 in base 167 — and, far
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
//
// # There is no permutation cache, and the reason is measured
//
// The natural way to amortise a shuffle per digit is a cache of permutations
// keyed by node: the shallow nodes are shared by many indices, so the O(p)
// work would be paid once per node instead of once per visit. The premise
// people reach for is that the tree depth is bounded by the digit count, so
// the cache stays small. The depth is indeed bounded. The node count is not,
// and it is the node count that a cache holds.
//
// Counted on the 39-dimension, 4096-point workload the benchmarks use, the
// walk touches 1,982,974 nodes of which 1,544,674 are distinct: a reuse factor
// of 1.28, and 382 MB of permutation arrays to hold them. The leading-zero
// tail is why. It is 366 of the 484 nodes a point visits, every
// one of those tails hangs below a different index's explicit digits, and no
// node in one is ever reached twice. The distinct-node count therefore grows
// with the number of points drawn, not with the number of digits.
// BenchmarkNestedNodeCache measures both numbers so the claim stays checked.
//
// A 28% ceiling does not pay for 382 MB, and it does not pay for the other
// cost either. Halton.At is documented as safe to call from any number of
// goroutines at once — it is how this package tells callers to drive one
// sequence from a worker pool — and a map populated on first visit would end
// that, whether it were fixed afterwards with a mutex, a sync.Map, or a race
// nobody noticed. Keeping the scramble stateless keeps that promise exactly as
// written. The O(p) shuffle is instead avoided by not running it: see
// nestedDigit.
func nestedRadicalInverse(index int, base int, root uint64) float64 {
	if base < 2 || index < 0 {
		return 0
	}

	var stack [nestedPermStack]int32

	scratch := stack[:]
	if base > len(stack) {
		scratch = make([]int32, base)
	}

	invBase := 1 / float64(base)
	place := invBase
	node := root
	result := 0.0

	for i := index; i > 0; i /= base {
		digit := i % base

		result += float64(nestedDigit(node, base, digit, scratch)) * place
		node = nestedChild(node, uint64(digit))
		place *= invBase
	}

	for place > 0 && result+float64(base)*place != result {
		result += float64(nestedDigit(node, base, 0, scratch)) * place
		node = nestedChild(node, 0)
		place *= invBase
	}

	if result >= oneMinusEpsilon {
		return oneMinusEpsilon
	}

	return result
}
