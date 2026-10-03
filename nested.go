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
// earlier use different node seeds below the divergence. Ideal independent
// node permutations would give uniform point marginals; this implementation
// derives permutations from finite seeded hashes and truncates the digit tail.
//
// Each node uses Fisher-Yates with rejection sampling, avoiding modulo bias
// under the pseudorandom-word model used by random-digit
// scrambling. There are p^k nodes at depth k, so precomputing the whole tree is
// impractical. A bounded immutable root cache amortizes the reused first digit;
// deeper entries are evaluated lazily by nestedDigit. See docs/performance.md
// for the separate root, shallow-node, and full-tree measurements.
//
// Historical affine permutations, correlation comparisons, and the measured
// lazy-shuffle/cache tradeoffs are documented in docs/randomization.md and
// docs/performance.md. They are workload observations, not uniformity proofs.
//
// Reference: Owen, A. B. (1995), "Randomly permuted (t,m,s)-nets and
// (t,s)-sequences".

// WithNestedScrambling turns on nested digit scrambling with the given seed.
// Each node's permutation depends on the digits above it. The ideal scheme
// with independent uniform permutations and an infinite digit tail gives
// uniform point marginals; this implementation uses finite seeded hashes and
// stops the tail when further digits cannot affect float64 precision. It
// preserves elementary-interval structure, but makes no exact continuous
// unbiasedness guarantee. Independent randomly selected seeds measure seed
// variability, which does not include truncation or PRNG bias.
//
// A bounded immutable root-permutation cache amortizes the first digit.
// Construction retains at most 64 KiB of cached digit entries, plus roots and
// prefix offsets; reuse the generator across a run. Deeper nodes are evaluated
// lazily. Cost and integration accuracy depend on the workload; repeated
// measurements and the cache tradeoff are recorded in docs/performance.md.
func WithNestedScrambling(seed uint64) Option {
	return func(s *settings) {
		s.randomize = randomizeNested
		s.seed = seed
	}
}

// nestedScrambler holds the per-dimension root of the permutation tree.
//
// Roots and a bounded prefix of their permutations are fixed at construction.
// Everything below a root is still derived by walking the original digit path;
// cache hits preserve the same permutation entries and hashes. No cache is
// filled or modified during indexed calls.
type nestedScrambler struct {
	roots            []uint64
	rootPermutations []int32
	rootOffsets      []int
}

// Bound table storage independently of dimensions and indices. Offsets cover
// only the cached prefix, not every requested dimension. These slices are
// immutable after construction and indexed reads require no shared scratch.
const nestedRootCacheEntries = 64 * 1024 / 4

func newNestedScrambler(seed uint64, bases []int) *nestedScrambler {
	entries, cachedDims := 0, 0
	for _, base := range bases {
		if base > nestedRootCacheEntries-entries {
			break
		}

		entries += base
		cachedDims++
	}

	n := &nestedScrambler{
		roots:            make([]uint64, len(bases)),
		rootPermutations: make([]int32, entries),
		rootOffsets:      make([]int, cachedDims+1),
	}
	offset := 0

	for d := range n.roots {
		n.roots[d] = nestedRoot(seed, d)
		if d < cachedDims {
			n.rootOffsets[d] = offset
			nestedPermutation(n.roots[d], n.rootPermutations[offset:offset+bases[d]])
			offset += bases[d]
		}
	}

	n.rootOffsets[cachedDims] = offset

	return n
}

func (n *nestedScrambler) rootPermutation(dim int) []int32 {
	if dim+1 >= len(n.rootOffsets) {
		return nil
	}

	return n.rootPermutations[n.rootOffsets[dim]:n.rootOffsets[dim+1]]
}

// nestedRoot derives the tree root for one dimension.
//
// Per-dimension keying keeps each root a function of seed and dimension,
// independent of the requested dimension count. A stream with one fixed-size
// draw per root could also preserve shared prefixes; keying is the chosen
// construction, not a mathematical requirement.
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
// Hash chaining avoids storing an arbitrarily long base-p digit prefix in an
// integer. The node remains a deterministic function of the original path;
// finite 64-bit hashes can still collide and do not prove independent nodes.
func nestedChild(node uint64, digit uint64) uint64 {
	rng := splitMix64(node ^ (digit+1)*0x9E3779B97F4A7C15)

	return rng.next()
}

// nestedPermutation writes the node's seeded permutation of
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
// runs downwards. Both orders are uniform under independent uniform draws. Only the upward one
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
// Digit zero settles on the first draw and needs no scratch array or shuffle.
// A nonzero digit d evaluates steps 0..d; later steps cannot change its entry.
// See docs/performance.md for measured lazy/full-shuffle comparisons.
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
// The stopping test uses the remaining-tail bound rather than a fixed depth.
// Omitting the tail would instead force short indices onto a coarse lattice;
// a one-digit base-p index would land on a multiple of p^-1.
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
// Roots are cached eagerly under a fixed table budget; deeper nodes remain
// stateless. A full-tree cache has a different cost: BenchmarkNestedNodeCache
// reports node visits, distinct nodes, and estimated full-permutation storage
// on a fixed workload. Its memory grows with the sampled indices and a lazy
// map would require synchronization. Bounded shallow caching is feasible but
// adds construction/memory beyond the root-only choice. The measurements and
// separate decisions are recorded in docs/performance.md.
func nestedRadicalInverse(index int, base int, root uint64) float64 {
	return nestedInverse(index, base, root, nil)
}

func nestedInverse(index int, base int, root uint64, rootPermutation []int32) float64 {
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

	// A full root permutation evaluates exactly the entry nestedDigit would
	// produce. Peel that one digit without changing the remaining arithmetic,
	// child hashes, zero tail, or scratch ownership. For index zero this is the
	// first zero-tail digit, with the same stopping condition as the loop below.
	if rootPermutation != nil {
		digit := index % base
		result += float64(rootPermutation[digit]) * place
		node = nestedChild(node, uint64(digit))
		place *= invBase
		index /= base
	}

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
