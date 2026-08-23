package qmc

import "math/bits"

// This file implements Owen scrambling for the Sobol sequence, in base 2.
//
// The digital shift in sobol.go XORs one random word per dimension into every
// point. That is enough to make the sequence randomized — the estimator
// becomes unbiased and averaging over shifts gives an error estimate — but it
// is a rigid motion. Every point moves the same way, so a shift cannot repair
// a projection that is badly distributed to begin with, and the sequence keeps
// whatever structure it had.
//
// Owen scrambling is the stronger construction. Read a coordinate as a binary
// tree: the first bit chooses a half, the second chooses a half of that, and
// so on. Owen scrambling draws an independent random bit flip at every node of
// that tree, so the decision to swap the two halves at depth k depends on the
// path taken through depths 0..k-1. Because the flip at each node maps the
// node's two children onto each other, every elementary interval maps onto
// another elementary interval of the same size: the point set is still the
// same (t,m,s)-net, so the low-discrepancy structure survives intact while the
// correlations between coordinates do not.
//
// The obvious implementation stores the tree, which is not affordable: 2^32
// nodes per dimension. Burley's construction replaces the stored random bits
// with a hash, computed in a handful of ALU operations and with no memory at
// all, by exploiting the fact that in the bit-reversed domain "bit i depends
// on bits above i" becomes "bit i depends on bits below i" — which is what
// ordinary integer arithmetic already does, since a carry only ever propagates
// upwards.
//
// What is exact here and what is not, because the distinction matters and is
// easy to overstate: the *nesting structure* is exact. The scramble provably
// maps elementary intervals onto elementary intervals, and that is asserted in
// owen_test.go rather than asserted in prose. What is an approximation is the
// *uniformity* of the random permutation at each node — the flips come from a
// hash of the path rather than from independent uniform draws, so they are
// deterministic given the seed and not statistically independent in the way
// the theory assumes. This is the standard practical trade and it is why the
// literature calls it hash-based Owen scrambling rather than Owen scrambling.
//
// Reference: Burley, B. (2020), "Practical Hash-based Owen Scrambling",
// Journal of Computer Graphics Techniques 9(4). The permutation constants are
// Laine and Karras's, reported there.

// WithOwenScrambling turns on hash-based Owen scrambling with the given seed.
//
// It applies to Sobol generators only. Halton is not base 2 in any dimension
// but its first, so this construction has nothing to permute there; the
// nearest equivalent for Halton is WithNestedScrambling, and NewHalton says so
// by name rather than ignoring the option.
//
// Prefer this to WithDigitalShift unless the cost matters. Both make the
// generator a randomized QMC sequence and both leave the (t,m,s)-net structure
// intact, but a digital shift translates the whole point set rigidly, so a
// poorly distributed projection stays poorly distributed under every shift it
// could be given. Owen scrambling actually redistributes, which is why it is
// the construction the theory's better convergence rates are stated for.
//
// It subsumes the digital shift: the flip at the root of the tree is a random
// bit flip of the whole coordinate, which is what a one-bit digital shift is.
// So there is no reason to want both, and the two options are mutually
// exclusive rather than combinable — see the randomization type in options.go.
//
// What it buys and what it costs, both measured at 39 dimensions. On the
// integrand in sobol_integration_test.go it is 1.08x more accurate than a
// digital shift over ten streams — a real but small margin, and small is the
// honest word for it on an integrand this smooth; the gap widens on functions
// whose projections are where the difficulty lives, which is the case Owen
// scrambling is for.
//
// The cost is lopsided, and which entry point you use decides it. On AtInto it
// is nearly free: 369.6 ns/op against 359.9 for a digital shift, because that
// path already XORs one direction number per set bit of the index and a few
// more ALU operations disappear into it. On NextInto it is 196.6 ns/op against
// 65.2, a factor of three — because the Gray-code recurrence is exactly what
// cannot carry a non-linear scramble, so every coordinate has to be hashed on
// the way out and the cheap path stops being cheap. A caller drawing points
// with Next in an inner loop should price that before choosing.
func WithOwenScrambling(seed uint64) Option {
	return func(s *settings) {
		s.randomize = randomizeOwen
		s.seed = seed
	}
}

// newOwenSeeds derives one scrambling seed per dimension.
//
// The dimension is mixed into the stream seed rather than drawn from one
// shared stream, for the reason newPermutation in scramble.go gives: a
// generator built for 5 dimensions and one built for 1024 must agree on the
// scrambling of their first 5, or a caller who widens their search space
// silently changes every coordinate they had already computed.
func newOwenSeeds(dims int, seed uint64) []uint32 {
	out := make([]uint32, dims)
	for d := range out {
		rng := splitMix64(seed ^ (uint64(d)+1)*0x2545F4914F6CDD1D)
		rng.next()
		rng.next()

		out[d] = uint32(rng.next())
	}

	return out
}

// owenScramble applies the nested scramble to one 32-bit coordinate.
//
// The two bit reversals are the whole trick and are not an implementation
// detail that could be optimised away. Owen scrambling needs output bit i to
// depend on input bits 0..i-1 — the bits *above* it, since bit 0 is the most
// significant here and names the first branch of the tree. Arithmetic gives
// the opposite dependency for free, because carries and multiplies propagate
// from the low end upwards and never back down. So the value is reversed into
// the domain where the dependency arithmetic already has is the one Owen
// wants, permuted there, and reversed back.
//
// The permutation itself must fix nothing and must be a bijection on all 2^32
// values, or two distinct coordinates would collide and the point set would
// stop being a net. Each x ^= x * C step is a bijection precisely because C is
// even: the low bit of x*C is then always 0, so bit 0 of x survives, and by
// induction each bit is determined by the bits below it plus itself. Changing
// a constant to an odd number would break that silently — the sequence would
// still look plausible.
func owenScramble(x, seed uint32) uint32 {
	x = bits.Reverse32(x)

	x += seed
	x ^= x * 0x6C50B47C
	x ^= x * 0xB82F1E52
	x ^= x * 0xC7AFE638
	x ^= x * 0x8D22F6E6

	return bits.Reverse32(x)
}
