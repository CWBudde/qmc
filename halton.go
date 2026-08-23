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
	perms [][]int32        // nil unless random-digit scrambling is on
	nest  *nestedScrambler // nil unless nested affine scrambling is on
	skip  int

	cursor int // index of the next point Next will return
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

	// A randomization is rejected here rather than ignored. The schemes are
	// not interchangeable — a digital shift is defined on base-2 digits and
	// this generator has no base-2 digits outside dimension 0 — so an option
	// that does not apply is a caller mistake, and the two ways to absorb a
	// caller mistake are both worse than saying so: silently dropping it hands
	// back an unrandomized sequence under a name that promises randomization,
	// and quietly substituting the nearest scheme gives them points that are
	// reproducible but not the ones they asked for.
	switch cfg.randomize {
	case randomizeNone, randomizeDigitPermutation, randomizeNested:
	default:
		return nil, fmt.Errorf("qmc: %s does not apply to a Halton generator", cfg.randomize)
	}

	h := &Halton{
		dims:  dims,
		bases: primesUpTo(dims),
		skip:  cfg.skip,
	}
	switch cfg.randomize {
	case randomizeNested:
		h.nest = newNestedScrambler(cfg.seed, dims)
	case randomizeDigitPermutation:
		h.perms = make([][]int32, dims)
		for d, base := range h.bases {
			h.perms[d] = newPermutation(base, cfg.seed, d)
		}
	}

	return h, nil
}

// Dims returns the number of dimensions.
func (h *Halton) Dims() int { return h.dims }

// Bases returns the prime base of each dimension, in order: the d-th entry is
// the d-th prime, which is the base whose radical inverse produces coordinate d.
// That is the whole of what distinguishes one dimension from another, so it is
// also the number that explains a dimension's behaviour — dimension 38 uses
// base 167, and its first 167 points therefore march up a ramp in steps of
// 1/167 unless scrambling is on.
//
// The returned slice is a fresh copy on every call. The generator reads its
// bases on every coordinate of every point, so handing out the internal slice
// would let a caller who sorts, truncates or edits it turn the sequence into a
// different one — and not visibly: the points would keep looking like plausible
// low-discrepancy points while no longer being the Halton sequence at all. A
// copy of a few hundred ints per call is not worth a failure mode nobody can
// see.
//
// This exists so a UI (the WebAssembly demo in examples/wasm-demo does exactly
// this) can label what it is drawing without re-deriving the prime table the
// library already computed.
func (h *Halton) Bases() []int {
	out := make([]int, len(h.bases))
	copy(out, h.bases)

	return out
}

// Permutation returns the digit permutation applied to dimension dim, or nil
// when the generator is unscrambled.
//
// With WithScrambling in effect, each dimension carries an independent uniform
// permutation of the digit alphabet {0..base-1} for its base, and every digit
// of the radical inverse — including the infinitely many leading zeros — is
// mapped through it. The returned slice is that permutation: entry i is the
// digit that digit i is rewritten to, so it has exactly Bases()[dim] entries
// and each of 0..base-1 appears once.
//
// Nested affine scrambling (WithNestedScrambling) also returns nil, and not
// because it is unscrambled: it has no permutation table to hand out. Its
// permutations depend on the digits above the one being rewritten, so there is
// one per node of a tree with p^k nodes at depth k, derived on the fly and
// never stored.
//
// A dim outside [0, Dims()) returns nil rather than panicking, because the
// callers are display code walking a dimension list that may be out of step
// with the generator by a frame; nil is a thing a renderer can skip, a panic
// in that position takes the whole page down.
//
// As with Bases, the returned slice is a fresh copy: the permutations are read
// on every scrambled coordinate, and a caller mutating one in place would
// silently corrupt every subsequent point of that dimension while the
// generator kept reporting the same configuration.
func (h *Halton) Permutation(dim int) []int32 {
	if h.perms == nil || dim < 0 || dim >= len(h.perms) {
		return nil
	}

	out := make([]int32, len(h.perms[dim]))
	copy(out, h.perms[dim])

	return out
}

// Next returns the next point of the sequence in a freshly allocated slice.
func (h *Halton) Next() []float64 {
	out := make([]float64, h.dims)
	h.NextInto(out)

	return out
}

// NextInto writes the next point into dst. It allocates nothing, which matters
// in an optimizer's inner loop.
//
// dst must have room for Dims() coordinates; a shorter one panics. Absorbing
// it instead would leave the tail coordinates holding zeros or stale values,
// which look like plausible positions and would steer a search silently.
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

// AtInto is At without the allocation. As with NextInto, dst shorter than
// Dims() panics rather than being silently truncated.
func (h *Halton) AtInto(i int, dst []float64) { h.fill(i, dst) }

func (h *Halton) fill(i int, dst []float64) {
	if i < 0 {
		i = 0
	}

	// skip is non-negative (WithSkip clamps) and i has just been clamped, so
	// skip+1+i can only leave the representable range by overflowing upwards.
	// It is refused rather than clamped: a wrapped sum goes negative, and a
	// negative index used to fall through radicalInverse's guard and hand back
	// the all-zeros origin — the one point At documents it never returns — so
	// the caller got a plausible-looking point that was not on the sequence.
	// Clamping to MaxInt would be the same failure in a different disguise:
	// every index past the limit would alias onto one point with nothing to
	// show for it. This mirrors scrambledRadicalInverse, which already refuses
	// an index it cannot represent instead of returning a shorter index's
	// value.
	if i > math.MaxInt-1-h.skip {
		panic(fmt.Sprintf(
			"qmc: point index %d with skip %d overflows the raw Halton index", i, h.skip,
		))
	}

	index := h.skip + 1 + i

	switch {
	case h.nest != nil:
		for d := 0; d < h.dims; d++ {
			dst[d] = nestedRadicalInverse(index, h.bases[d], h.nest.roots[d])
		}
	case h.perms != nil:
		for d := 0; d < h.dims; d++ {
			dst[d] = scrambledRadicalInverse(index, h.bases[d], h.perms[d])
		}
	default:
		for d := 0; d < h.dims; d++ {
			dst[d] = radicalInverse(index, h.bases[d])
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
//
// The index < 0 guard stays even though fill can no longer produce a negative
// index: this is an unexported helper called directly by the package's own
// tests and by scramble-side code, and 0 is the right answer for an index with
// no digits. What it must not be is a silent path for a wrapped index — fill
// now stops that one case before it gets here, so the guard only ever answers
// for a caller that meant it.
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

	// The true value is 1 - base^-m, so an index whose digits are all maximal
	// rounds up to 1 — or past it, as base 167 does — once base^-m falls below
	// an ulp. That needs an index around 1.7e16, far beyond anything a sampler
	// reaches, but [0,1) is what this package promises and a promise that
	// holds only for small inputs is not one. The clamp cannot perturb a
	// reachable index: it fires only where the result had already rounded to
	// 1, so the bit-identical guarantee against the generator this replaced is
	// untouched.
	if result >= oneMinusEpsilon {
		return oneMinusEpsilon
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
	// Stop reversing before the accumulator could overflow. The bound has to
	// leave room for the digit as well as the multiply: at base 167 an
	// accumulator sitting exactly on ^uint64(0)/base wraps on the next step to
	// a small positive value, which passes the guard again, so the loop would
	// carry on with garbage rather than stop.
	//
	// Overrunning is refused rather than broken out of, because breaking out
	// is silently wrong in a worse way: reversed and invBaseN advance together,
	// so a truncated reversal returns the exactly-correct value of a different,
	// shorter index. Two far-apart indices would land on one point with nothing
	// to show for it.
	//
	// The bound is unreachable in practice — it needs an index above 6e17 even
	// at base 167 — so this costs one comparison per digit and never fires.
	limit := (^uint64(0) - uint64(base-1)) / uint64(base)

	var reversed uint64

	invBase := 1 / float64(base)
	invBaseN := 1.0

	for i := index; i > 0; i /= base {
		if reversed > limit {
			panic(fmt.Sprintf(
				"qmc: index %d has too many base-%d digits to reverse without overflow", index, base,
			))
		}

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
