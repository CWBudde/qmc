// Package qmc provides quasi-Monte Carlo sequences: deterministic,
// low-discrepancy point sets that fill a unit hypercube more evenly than
// independent random sampling does.
//
// The package currently implements the Halton sequence, optionally with
// random-digit scrambling. Points are returned as coordinates in [0,1), so a
// caller maps them onto its own parameter ranges.
//
//	g, err := qmc.NewHalton(39, qmc.WithSkip(64), qmc.WithScrambling(seed))
//	if err != nil {
//		return err
//	}
//	for i := 0; i < 600; i++ {
//		point := g.Next() // len(point) == 39, every coordinate in [0,1)
//		...
//	}
//
// Use scrambling above roughly twenty dimensions. See WithScrambling for why.
package qmc

import (
	"fmt"
	"math"
)

// oneMinusEpsilon is the largest float64 strictly below 1. Every coordinate is
// clamped to it so callers can rely on the half-open range [0,1) without
// defending against a rounded-up 1.0 that would index one past the end of a
// bucket table or push a knob past its upper bound.
var oneMinusEpsilon = math.Nextafter(1, 0)

// Halton generates points of the Halton sequence in a fixed number of
// dimensions.
//
// A Halton generator is not safe for concurrent use through its stateful
// methods (Next, NextInto, Reset). At is stateless and may be called from any
// number of goroutines at once, which is the way to drive one shared sequence
// from a worker pool: have the workers claim indices from an atomic counter
// and call At.
type Halton struct {
	dims  int
	bases []int
	perms [][]int32 // nil when unscrambled
	skip  int

	cursor int // index of the next point Next will return
	buf    []float64
}

// NewHalton returns a generator over dims dimensions.
//
// dims is bounded only by how many primes fit in memory; there is no fixed
// base table to run out of.
func NewHalton(dims int, opts ...Option) (*Halton, error) {
	if dims < 1 {
		return nil, fmt.Errorf("qmc: dims must be >= 1, got %d", dims)
	}
	var cfg settings
	for _, opt := range opts {
		opt(&cfg)
	}

	h := &Halton{
		dims:  dims,
		bases: primesUpTo(dims),
		skip:  cfg.skip,
		buf:   make([]float64, dims),
	}
	if cfg.scramble {
		h.perms = make([][]int32, dims)
		for d, base := range h.bases {
			h.perms[d] = newPermutation(base, cfg.seed, d)
		}
	}
	return h, nil
}

// Dims returns the number of dimensions.
func (h *Halton) Dims() int { return h.dims }

// Next returns the next point of the sequence in a freshly allocated slice.
func (h *Halton) Next() []float64 {
	out := make([]float64, h.dims)
	h.NextInto(out)
	return out
}

// NextInto writes the next point into dst, which must have room for Dims()
// coordinates. It allocates nothing, which matters in an optimizer's inner
// loop.
func (h *Halton) NextInto(dst []float64) {
	h.fill(h.cursor, dst)
	h.cursor++
}

// Reset rewinds the stateful cursor so the next call to Next returns point 0
// again. The sequence itself is unchanged: a generator always yields the same
// points for the same configuration.
func (h *Halton) Reset() { h.cursor = 0 }

// At returns point i of the sequence, counting from 0, without touching the
// cursor. It is the reproducible entry point: At(i) depends only on i and the
// generator's configuration, never on how many points have been drawn.
//
// Point i corresponds to raw Halton index skip+1+i, so index 0 — the
// degenerate origin, all zeros before scrambling — is never returned.
//
// Negative i is treated as 0.
func (h *Halton) At(i int) []float64 {
	out := make([]float64, h.dims)
	h.fill(i, out)
	return out
}

// AtInto is At without the allocation.
func (h *Halton) AtInto(i int, dst []float64) { h.fill(i, dst) }

func (h *Halton) fill(i int, dst []float64) {
	if i < 0 {
		i = 0
	}
	index := h.skip + 1 + i
	for d := 0; d < h.dims && d < len(dst); d++ {
		if h.perms == nil {
			dst[d] = radicalInverse(index, h.bases[d])
		} else {
			dst[d] = scrambledRadicalInverse(index, h.bases[d], h.perms[d])
		}
	}
}

// radicalInverse returns the base-b radical inverse of index: the digits of
// index in base b, mirrored around the radix point.
//
// This is deliberately the plain, digit-at-a-time accumulation rather than the
// integer-reversal form used by scrambledRadicalInverse. The two agree
// mathematically but not always in the last bit, and this one is what callers
// migrating off a hand-rolled Halton already have in their recorded outputs.
func radicalInverse(index int, base int) float64 {
	if base < 2 || index < 0 {
		return 0
	}
	result := 0.0
	f := 1.0 / float64(base)
	for i := index; i > 0; i /= base {
		result += float64(i%base) * f
		f /= float64(base)
	}
	return result
}

// scrambledRadicalInverse applies perm to every digit of the radical inverse,
// including the infinitely many leading zeros of index.
//
// Those zeros are the part that is easy to get wrong. Unscrambled they
// contribute nothing, but perm[0] is generally not 0, so every one of them
// adds perm[0] at its own digit position. That tail is a geometric series and
// closes to invBase*perm[0]/(1-invBase), scaled by the place value the
// explicit digits ended on. Dropping it biases every coordinate low and, worse,
// makes short indices land on a coarse lattice — exactly the defect scrambling
// is meant to remove.
//
// The result is clamped strictly below 1: the tail can round up to 1.0 for a
// permutation whose perm[0] is the largest digit.
func scrambledRadicalInverse(index int, base int, perm []int32) float64 {
	if base < 2 || index < 0 {
		return 0
	}
	// Stop reversing before the accumulator could overflow. At base 2 this is
	// 63 digits, far more than any index a sampler will reach.
	limit := ^uint64(0) / uint64(base)

	var reversed uint64
	invBase := 1 / float64(base)
	invBaseN := 1.0

	for i := index; i > 0 && reversed <= limit; i /= base {
		reversed = reversed*uint64(base) + uint64(perm[i%base])
		invBaseN *= invBase
	}

	tail := invBase * float64(perm[0]) / (1 - invBase)
	v := invBaseN * (float64(reversed) + tail)
	if v >= oneMinusEpsilon {
		return oneMinusEpsilon
	}
	return v
}
