package qmc

// This file implements seeded random-digit scrambling for Halton sequences.
// One digit permutation per dimension is reused at every digit position.
// This preserves elementary-interval structure to the represented digit depth.
// Fisher-Yates is uniform under independent uniform random-word draws; the
// finite seeded implementation does not prove independent permutations across
// dimensions or exact uniform point marginals. See docs/randomization.md.
// Correlation and cost measurements are recorded there and in docs/performance.md.
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

// newPermutation returns a seeded pseudorandom permutation of {0..base-1} derived
// from seed and dim. Per-dimension keying isolates each dimension's stream
// from the variable number of shuffle/rejection draws used by other dimensions.
// The key does not depend on the total requested dimension count.
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

// uniformBelow avoids modulo bias in [0,n) under the uniform-word model.
// Rejection sampling does not establish independence of the seeded words.
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
