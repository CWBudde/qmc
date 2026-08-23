package qmc

// A Sequence is a quasi-random point generator over a fixed number of
// dimensions.
//
// This interface is deliberately small, and it deliberately excludes everything
// that is specific to one construction. Halton exposes Bases and Permutation;
// Sobol has neither, because it works in base 2 in every dimension. Putting
// either on this interface would force one generator to answer a question that
// does not apply to it, and the honest answer to an inapplicable question is
// not a zero value — it is that the caller is holding the wrong type. Code that
// needs the prime bases wants a *Halton and should say so.
//
// What is here is the part every construction shares: how many dimensions, the
// stateful cursor (Next, NextInto, Reset), and the stateless index-addressed
// form (At, AtInto). The split matters more than it looks. At(i) depends only
// on i and the generator's configuration, so it is the reproducible entry point
// and the one safe to call concurrently; Next carries a cursor and is not.
//
// The contract every implementation owes:
//
//   - every coordinate lies in [0,1), strictly below 1;
//   - At(i) is independent of how many points have been drawn;
//   - a dst shorter than Dims() panics rather than being truncated, because a
//     short write leaves the tail coordinates holding stale values that look
//     like plausible positions;
//   - the points depend on the configuration alone, never on GOARCH.
type Sequence interface {
	// Dims returns the number of dimensions.
	Dims() int

	// Next returns the next point in a freshly allocated slice.
	Next() []float64

	// NextInto writes the next point into dst, allocating nothing.
	NextInto(dst []float64)

	// Reset rewinds the cursor so the next call to Next returns point 0.
	Reset()

	// At returns point i, counting from 0, without touching the cursor.
	At(i int) []float64

	// AtInto is At without the allocation.
	AtInto(i int, dst []float64)
}

// Compile-time proof that every generator in this package satisfies the
// interface. This is the assertion that keeps Sequence honest: a method
// renamed on one generator and not the other stops the build here rather than
// at some caller's type switch. Every new generator adds a line here.
var _ Sequence = (*Halton)(nil)
