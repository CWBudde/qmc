package qmc

import "io"

// An Option configures a generator at construction time. Options are applied
// in order and the resulting configuration is fixed for the generator's life:
// a sequence whose parameters could change mid-run would not be reproducible,
// which is the whole point of using a quasi-random sequence in the first
// place.
type Option func(*settings)

// randomization names the scheme that turns a deterministic sequence into a
// randomized one (RQMC).
//
// It is a single field rather than one bool per scheme because the schemes are
// mutually exclusive: a generator has one randomization or none. Modelling that
// as four independent bools would make "scrambled and digitally shifted" a
// representable state that no constructor knows what to do with, and the usual
// outcome of a representable-but-meaningless state is that some branch silently
// picks a winner. Here the last option applied wins, which is what the
// functional-options ordering already promises, and a scheme that does not
// apply to the generator being built is an error at construction rather than
// something quietly ignored.
type randomization uint8

const (
	randomizeNone randomization = iota

	// randomizeDigitPermutation is Halton's random-digit scrambling: one
	// permutation of the digit alphabet per dimension. See scramble.go.
	randomizeDigitPermutation

	// randomizeNested is Halton's nested affine digit scrambling, which
	// conditions each digit's permutation on the digits above it. See nested.go.
	randomizeNested

	// randomizeDigitalShift is Sobol's digital shift: one random word per
	// dimension, XORed into every point. See sobol.go.
	randomizeDigitalShift

	// randomizeOwen is Sobol's hash-based Owen scrambling. See owen.go.
	randomizeOwen
)

// String reports the option a caller wrote, so a constructor rejecting a
// randomization it does not implement can name it back to them.
func (r randomization) String() string {
	switch r {
	case randomizeNone:
		return "none"
	case randomizeDigitPermutation:
		return "WithScrambling"
	case randomizeNested:
		return "WithNestedScrambling"
	case randomizeDigitalShift:
		return "WithDigitalShift"
	case randomizeOwen:
		return "WithOwenScrambling"
	default:
		return "unknown"
	}
}

type settings struct {
	skip      int
	randomize randomization
	seed      uint64

	// directions carries a caller-supplied Joe-Kuo direction-number table for
	// Sobol. nil means the embedded table. See WithDirectionNumbers.
	directions io.Reader
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
		s.randomize = randomizeDigitPermutation
		s.seed = seed
	}
}
