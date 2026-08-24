// Package qmc provides quasi-Monte Carlo sequences: deterministic,
// low-discrepancy point sets that fill a unit hypercube more evenly than
// independent random sampling does.
//
// Two sequences are implemented, both satisfying Sequence. Points are returned
// as coordinates in [0,1), so a caller maps them onto its own parameter
// ranges.
//
//	g, err := qmc.NewSobol(39, qmc.WithSkip(64), qmc.WithOwenScrambling(seed))
//	if err != nil {
//		return err
//	}
//	for i := 0; i < 600; i++ {
//		point := g.Next() // len(point) == 39, every coordinate in [0,1)
//		...
//	}
//
// Which one to reach for. Sobol works in base 2 in every dimension and does
// not degrade as dimensions are added, which makes it the better default above
// a handful of dimensions; it is limited to the 1024 dimensions the embedded
// Joe-Kuo direction numbers cover, unless a caller supplies their own table.
// Halton has no dimension ceiling at all and is the one to keep if you need a
// sequence whose construction is simple enough to reproduce by hand, but above
// roughly twenty dimensions it must be randomized to be usable — its later
// coordinates degenerate into ramps that correlate with each other. See
// WithScrambling.
//
// Every generator here is deterministic given its configuration, and that
// includes the randomizations: they are seeded, not sampled, so a run is
// reproducible across machines, architectures and Go versions. Randomizing is
// what makes these randomized quasi-Monte Carlo (RQMC) sequences — the
// estimator becomes unbiased and averaging over seeds gives an error estimate
// that plain QMC cannot offer.
//
// The measured reason to use any of this, on a smooth 39-dimensional product
// integrand at 4096 points over ten streams: plain Monte Carlo reaches an RMS
// relative error of 4.3e-03, scrambled Halton 2.4e-04, Sobol with a digital
// shift 1.5e-04. The gap is structural — 1/n against 1/sqrt(n) convergence —
// and integration_test.go and sobol_integration_test.go hold it to at least a
// factor of five so that it stays true.
package qmc

import (
	"fmt"
	"math"
)

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
var (
	_ Sequence = (*Halton)(nil)
	_ Sequence = (*Sobol)(nil)
)

// Draw returns the first n points of seq as an n-by-Dims() matrix.
//
// This is the bulk form of At, and it is built on AtInto for the reason the
// interface comment above gives: At(i) depends only on i and the generator's
// configuration, so the matrix is reproducible and the caller's cursor is left
// exactly where it was. Building it from Next instead would have made the
// result depend on how many points the caller had already drawn — the same
// matrix, drawn twice, would not be the same matrix — and would have consumed
// n points of the caller's stream as a side effect of measuring it.
//
// The rows alias one backing array. That is deliberate and it is
// load-bearing: CenteredL2Discrepancy walks this matrix N²/2 times, and a
// matrix of n independently allocated rows scattered across the heap costs
// more in cache misses than the whole rest of the statistic. One allocation
// for the data and one for the row headers is what a caller measuring a few
// thousand points in a few dozen dimensions should see, and BenchmarkDraw
// pins it.
//
// Each row is capped at its own length, so appending to a row allocates a new
// array instead of scribbling into the next row's coordinates. Without the
// three-index slice that mistake would be silent and would corrupt the point
// after the one being appended to.
//
// n <= 0 returns a non-nil empty slice rather than nil, so a caller can range
// over the result unconditionally.
//
// The wasm demo keeps its own flat, column-major fill loops instead of calling
// this. That is not an oversight: on a 32-bit heap it reuses one buffer across
// an animation's frames, and a helper that allocates per call would churn it.
func Draw(seq Sequence, n int) [][]float64 {
	if n <= 0 {
		return [][]float64{}
	}

	dims := seq.Dims()
	if dims <= 0 {
		return [][]float64{}
	}

	// One flat array of n*dims float64s is about to be allocated. On a 32-bit
	// build that product can wrap, and a wrapped length would allocate a short
	// array that the row loop then slices past the end of. This mirrors
	// Halton.fill: the division is the exact test for n*dims <= MaxInt, done
	// before the multiplication rather than after it, because the
	// multiplication is the operation being guarded.
	if n > math.MaxInt/dims {
		panic(fmt.Sprintf(
			"qmc: drawing %d points in %d dimensions overflows the backing array length",
			n, dims,
		))
	}

	flat := make([]float64, n*dims)
	out := make([][]float64, n)

	for i := range out {
		out[i] = flat[i*dims : (i+1)*dims : (i+1)*dims]
		seq.AtInto(i, out[i])
	}

	return out
}
