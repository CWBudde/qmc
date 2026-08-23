package qmc

// An Option configures a generator at construction time. Options are applied
// in order and the resulting configuration is fixed for the generator's life:
// a sequence whose parameters could change mid-run would not be reproducible,
// which is the whole point of using a quasi-random sequence in the first
// place.
type Option func(*settings)

type settings struct {
	skip     int
	scramble bool
	seed     uint64
}

// WithSkip discards the first n points of the underlying sequence (a burn-in).
//
// The first few Halton points are badly placed almost by construction: point 1
// sits at (1/2, 1/3, 1/5, ...), which is a corner of the box in every
// coordinate that has a large base. Skipping a few dozen points is the
// standard remedy and costs nothing.
//
// Negative values are treated as zero.
func WithSkip(n int) Option {
	return func(s *settings) {
		if n < 0 {
			n = 0
		}
		s.skip = n
	}
}

// WithScrambling turns on random-digit scrambling with the given seed.
//
// This makes the generator a randomized quasi-Monte Carlo (RQMC) sequence:
// still low-discrepancy, but no longer identical across seeds. That trade is
// deliberate. In more than roughly twenty dimensions an unscrambled Halton
// sequence does not fill the box at practical sample counts — its last
// coordinates degenerate into linear ramps that correlate with each other —
// and reproducibility of a sequence that is not actually filling the box is
// not worth much. Fix the seed and the run is reproducible again.
func WithScrambling(seed uint64) Option {
	return func(s *settings) {
		s.scramble = true
		s.seed = seed
	}
}
