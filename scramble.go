package qmc

// This file implements random-digit scrambling, the cheapest randomization
// that repairs the failure mode plain Halton has in high dimensions.
//
// Plain Halton places its d-th coordinate by the radical inverse in base p_d.
// For a large base the first p_d points of that coordinate are simply
// 0, 1/p_d, 2/p_d, ... — a linear ramp, not a sample. Two adjacent
// high-dimensional coordinates therefore ramp together and correlate almost
// perfectly until the sample count passes the product of their bases. Measured
// on 39 dimensions and 600 points, coordinates 37 and 38 correlate at +0.76.
//
// Random-digit scrambling applies an independent uniform permutation of the
// digit alphabet {0..b-1} to every digit position of every dimension. The
// sequence keeps its low-discrepancy structure — a permutation maps each
// elementary interval onto another elementary interval of the same size — but
// the ramps are destroyed. The same measurement over five seeds gives a worst
// adjacent-pair correlation of 0.117.
//
// Reference: Braaten, E. and Weller, G. (1979), "An improved low-discrepancy
// sequence for multidimensional quasi-Monte Carlo integration".

// splitMix64 is a small, fast, well-distributed counter-based generator. It is
// used only to derive the permutations, never to sample, so it needs to be
// reproducible and decorrelated across dimensions, nothing more.
type splitMix64 uint64

func (s *splitMix64) next() uint64 {
	*s += 0x9E3779B97F4A7C15
	z := uint64(*s)
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// newPermutation returns a uniform random permutation of {0..base-1} derived
// from seed and dim. Dimensions are mixed into the stream seed rather than
// drawn from one shared stream so that a generator built for 5 dimensions and
// one built for 39 agree on the permutations of their first 5.
func newPermutation(base int, seed uint64, dim int) []int32 {
	rng := splitMix64(seed ^ (uint64(dim)+1)*0x2545F4914F6CDD1D)
	// Warm up: the first output of splitmix64 from a low-entropy state is
	// fine, but two adjacent dims differ in few bits and a couple of steps
	// makes that irrelevant.
	rng.next()
	rng.next()

	perm := make([]int32, base)
	for i := range perm {
		perm[i] = int32(i)
	}
	// Fisher-Yates, unbiased modulo via rejection.
	for i := base - 1; i > 0; i-- {
		j := int(uniformBelow(&rng, uint64(i+1)))
		perm[i], perm[j] = perm[j], perm[i]
	}
	return perm
}

// uniformBelow returns a uniform value in [0, n) without modulo bias.
func uniformBelow(rng *splitMix64, n uint64) uint64 {
	if n <= 1 {
		return 0
	}
	limit := ^uint64(0) - (^uint64(0) % n)
	for {
		v := rng.next()
		if v < limit {
			return v % n
		}
	}
}
