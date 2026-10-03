package qmc

import (
	"fmt"
	"io"
	"math"
	"math/bits"
	"strings"
	"sync"
)

// twoPowMinus32 scales a 32-bit direction-number accumulator into a
// coordinate.
//
// Sobol needs no equivalent of Halton's oneMinusEpsilon clamp, and that is a
// property of the construction rather than luck: the accumulator is a uint32,
// so the largest coordinate it can produce is (2^32-1)/2^32, which is exactly
// representable and strictly below 1. Halton needs its clamp because a radical
// inverse is a sum of rounded terms that can creep up to 1.0; here there is no
// rounding to creep. Nothing downstream should add a clamp "for symmetry" — it
// would be dead code pretending to guard something.
const twoPowMinus32 = 1.0 / 4294967296.0

// Every entry point uses the same raw-index ceiling, including on 386 where
// int cannot represent the upper half of the direction-number index range.
const maxSobolRawIndex = min(uint64(math.MaxInt), uint64(math.MaxUint32))

// Sobol generates points of the Sobol sequence in a fixed number of
// dimensions, using the Joe-Kuo direction numbers.
//
// The sequence uses Gray-code order: point i is the direct-form point at
// gray(i) = i XOR (i>>1). This matches the published reference values used in
// the tests. Consecutive Gray codes differ in one bit, so this implementation
// advances with one direction-number XOR per dimension. Other orderings can
// use different precomputed recurrences; changing the ordering would change
// the public sequence's indexed outputs.
//
// The reordering costs nothing that matters. Gray coding is a bijection on
// [0, 2^m), so the first 2^m points are the same point set either way and the
// (t,m,s)-net balance property — the thing the sequence is for — is untouched.
//
// WithLeap above one forfeits the general aligned-block guarantee: a
// leaped run visits a strided subset of the raw indices, which is not a block
// of 2^m of them however it is aligned. It also costs Next the recurrence
// above. Both are written out at WithLeap, along with why an even leap is
// refused outright.
//
// # Balance holds on aligned blocks, not on any window
//
// A base-2 (t,m,s)-net places 2^t points in each elementary interval of
// volume 2^(t-m); one-point occupancy requires t=0. This property concerns
// a block of 2^m raw indices that begins on a multiple of 2^m. Arbitrary
// windows of 2^m consecutive points lack the same general guarantee.
//
// Point i has raw index skip+1+i with leap 1. Default skip 0 starts at raw
// index 1 and does not provide the usual first aligned power-of-two block.
// For N=2^m points, WithSkip(q*N-1), q>=1, selects an aligned later block,
// provided the entire block fits the supported raw-index range. The origin
// block is not exposed by this API. Arbitrary unaligned windows lack the
// general net guarantee, though a particular window may satisfy a net property.
// See docs/api-design.md for checked examples and the compatibility decision.
//
// # Not every two-dimensional projection is a t=0 net
//
// Joe and Kuo's D(6) criterion optimizes two-dimensional projections without
// making them all t=0 nets. A projection inherits the full net's t guarantee
// and may have a better t. Digital shifting and nested dyadic scrambling
// preserve these counts rather than repairing a poor direction-number table.
// Projection measurements and their sample blocks are in
// docs/choosing-a-sequence.md.
//
// A Sobol generator is not safe for concurrent use through its stateful
// methods (Next, NextInto, Reset). At and AtInto are stateless and may be
// called from any number of goroutines at once, which is the way to drive one
// shared sequence from a worker pool: have the workers claim indices from an
// atomic counter and call AtInto.
type Sobol struct {
	dims int

	// directions holds every dimension's 32 direction numbers in one flat
	// slice, dimension d occupying [d*sobolBits, (d+1)*sobolBits). A slice of
	// slices would be the obvious shape and would match Halton's perms field,
	// but the inner loop of a point walks one dimension's 32 words and then
	// moves to the next dimension's, so a flat layout keeps that walk
	// sequential in memory. At 1024 dimensions the difference is between 128 KB
	// of contiguous words and 1024 separately allocated runs.
	directions []uint32

	// shift is one random word per dimension, XORed into every point, or nil
	// when the generator is not randomized. See WithDigitalShift.
	shift []uint32

	// owen is one hash seed per dimension, or nil unless Owen scrambling is
	// on. This API chooses one seeded randomization scheme at construction,
	// so it is never combined with shift. Node flips already randomize each
	// digit, including the first-digit flip at the root.
	//
	// Unlike shift, it cannot be folded into the accumulator. A digital shift
	// is an XOR and therefore commutes with the Gray-code recurrence, so
	// carrying it in state costs nothing per point; an Owen scramble is not
	// linear, and pre-applying it would make the next XOR of a direction
	// number land on scrambled bits and produce a point off the sequence
	// entirely. So it is applied where the accumulator becomes a coordinate,
	// and state stays raw. See NextInto.
	owen []uint32

	skip int

	// leap is 1 unless WithLeap is on: point i is raw index skip+1+i*leap. Any
	// value above 1 costs this generator its Gray-code fast path and the
	// general aligned-block guarantee, both explained at WithLeap.
	leap int

	// counter is the raw sequence index of the point Next will return, and
	// state is that point's accumulator, already carrying shift. Together they
	// are the whole of the Gray-code recurrence's state.
	//
	// They are the cursor only when leap is 1. A leaping generator cannot use
	// the recurrence at all, so it counts points in cursor instead and reaches
	// them through fill. The two never run at once — leap is fixed at
	// construction — which is why one of the pair is always dead rather than
	// the two needing to be kept in step.
	counter   uint32
	state     []uint32
	exhausted bool

	// cursor is the index of the next point NextInto will return, used only
	// when leap is above 1.
	cursor int
}

// embeddedTable parses the built-in direction numbers once per process.
//
// The parse itself is cheap, but validateDirectionRows tests 1023 polynomials
// for primitivity, and repeating that for every generator a caller constructs
// would put a fixed cost on a constructor that otherwise does almost nothing.
// Once per process is the right frequency for a check whose input is a
// compiled-in constant: the bytes cannot change between constructions, so a
// second run could not reach a different verdict. A caller-supplied table is
// validated on every call, because its bytes are not a constant.
var embeddedTable = sync.OnceValues(func() ([]directionRow, error) {
	return parseDirectionNumbers(strings.NewReader(embeddedDirectionNumbers))
})

// WithDirectionNumbers supplies a Joe-Kuo direction-number table in place of
// the embedded one, read from r in the upstream text format: a header line,
// then rows of `d s a m_1 ... m_s` for d = 2, 3, 4, ...
//
// The reason to reach for this is dimension count. The embedded table stops at
// 1024 because that is what fits the package's size budget; upstream publishes
// the same construction out to 21201, and a caller who needs more can pass the
// full file. It is also the way to use a different search criterion — upstream
// ships D(5) and D(7) alongside the D(6) set embedded here.
//
// r goes through exactly the same parser and the same validator as the
// embedded table, so a file that is truncated, column-shifted or corrupted is
// refused at construction rather than turned into points. See
// validateDirectionRows for what that check does and does not prove.
// Construction consumes r without closing it. Do not share one reader between
// concurrent constructions or reuse it after consumption without rewinding it.
// For reusable table bytes, create a fresh reader and option for each constructor.
//
// # The file format
//
// Upstream's files live at https://web.maths.unsw.edu.au/~fkuo/sobol/; the
// embedded table is the first 1024 dimensions of the D(6) family,
// new-joe-kuo-6.21201, and passing that file whole is the supported way to go
// past the embedded ceiling. The format is whitespace-separated text:
//
//	d       s       a       m_i
//	2       1       0       1
//	3       2       1       1 3
//	4       3       1       1 3 1
//
// One header line, then one row per dimension from d = 2 upward. Dimension 1
// has no row: its polynomial is the empty one and all of its m_i are 1, which
// every Sobol implementation special-cases. A row is
//
//	d s a m_1 m_2 ... m_s
//
// where d is the dimension, s the degree of that dimension's primitive
// polynomial, a the polynomial's s-1 interior coefficients packed into an
// integer (bit s-1-k holds the coefficient of x^(s-k)), and m_1..m_s the
// initial direction numbers — exactly s of them, no more and no fewer. The
// optional header must be d s a m_i on the first nonempty line. Headerless
// numeric rows are accepted and blank lines are ignored. Other leading text
// is rejected instead of silently discarding a potentially malformed row.
//
// # What a caller-generated table must satisfy
//
// A table that fails any of these is refused at construction, by name, and the
// reasoning behind each is in validateDirectionRows:
//
//   - d runs contiguously from 2, with no gaps and no repeats. A row's
//     position in the file is what selects the dimension it is used for, so a
//     single missing line moves every later dimension onto another dimension's
//     polynomial — valid numbers, wrong dimension, no visible symptom.
//   - each row carries exactly s direction numbers.
//   - every m_i is odd. An even one clears V_i's new pivot bit and destroys
//     the linear independence the net property rests on.
//   - every m_i is below 2^i, so that m_i << (32-i) does not shift bits off
//     the top of the word.
//   - a fits exactly s-1 interior coefficient bits; at s=1 only a=0 is valid.
//   - the polynomial 1<<s | a<<1 | 1 is primitive over GF(2), not merely
//     irreducible. This rejects nonprimitive coefficients, but cannot authenticate
//     provenance or exclude valid-looking corruption.
//   - s is between 1 and 32. Above 32 there is nowhere to put the initial
//     direction numbers. The largest degree anywhere in the embedded 1024
//     dimensions is 13, so this bound only fires on a file that is not a
//     Joe-Kuo table at all.
//
// Nothing here proves the numbers are *good* — that they came from Joe and
// Kuo's search rather than from anywhere else — and nothing could. Primitivity
// and the m_i bounds are what makes a table usable; the quality of its
// two-dimensional projections is a search result, and a self-generated table
// that passes every check above will still have projections nobody optimised.
// TestDirectionTableBeyondTheEmbeddedCeiling drives a synthesised table of
// 1200 dimensions through this option and checks the resulting sequence is a
// net in every one of them, so the path above 1024 is exercised rather than
// assumed.
func WithDirectionNumbers(r io.Reader) Option {
	return func(s *settings) {
		s.directions = r
	}
}

// WithDigitalShift turns on digital shifting with the given seed: one seeded
// pseudorandom 32-bit word per dimension, XORed into every point's accumulator.
//
// The fixed XOR can be folded into the stateful accumulator at reset.
// Indexed/stateful timing comparisons are documented in docs/performance.md.
//
// With independent uniform shift words, each point would be uniform on the
// 32-bit grid, so estimates would be unbiased for that grid average, not
// necessarily for the continuous integral. Here words come from one finite
// seed. Independent randomly selected seeds permit an empirical estimate of
// seed variability; that does not include grid or PRNG bias. The XOR maps
// dyadic intervals onto intervals of the same size and preserves the t value
// of an aligned net to the supported bit depth.
//
// What it does not do is repair a bad projection. A digital shift translates
// the whole net; if two dimensions' direction numbers give a poor
// two-dimensional projection, every shift of it has the same dyadic counts.
// Nested Owen scrambling also preserves dyadic occupancy quality; it changes
// positions within that constraint, rather than repairing the table.
func WithDigitalShift(seed uint64) Option {
	return func(s *settings) {
		s.randomize = randomizeDigitalShift
		s.seed = seed
	}
}

// NewSobol returns a generator over dims dimensions.
//
// dims is limited by the direction-number table: 1024 with the embedded one,
// or whatever the table passed to WithDirectionNumbers covers. Exceeding it is
// an error rather than a wrap-around onto lower dimensions, because reusing a
// dimension's direction numbers would make two coordinates of every point
// identical — a defect that a caller integrating in a few hundred dimensions
// would have no way to see in the output.
// A skip that leaves no representable first raw index is an error.
func NewSobol(dims int, opts ...Option) (*Sobol, error) {
	if dims < 1 {
		return nil, fmt.Errorf("qmc: dims must be >= 1, got %d", dims)
	}

	var cfg settings
	for _, opt := range opts {
		opt(&cfg)
	}

	// A randomization that this generator does not implement is refused by
	// name rather than ignored. Ignoring it would hand back a deterministic
	// sequence from a call that asked for a randomized one, and the caller's
	// next step is usually to average over seeds — which would silently
	// average over identical runs and report an error estimate of zero.
	switch cfg.randomize {
	case randomizeNone, randomizeDigitalShift, randomizeOwen:
	default:
		return nil, fmt.Errorf("qmc: %s does not apply to a Sobol generator", cfg.randomize)
	}

	rows, err := loadDirectionRows(cfg.directions)
	if err != nil {
		return nil, err
	}

	// rows starts at dimension 2, so the table covers len(rows)+1 dimensions
	// once the implicit first one is counted.
	if available := len(rows) + 1; dims > available {
		return nil, fmt.Errorf(
			"qmc: dims must be <= %d for this direction-number table, got %d; "+
				"pass a larger table with WithDirectionNumbers", available, dims,
		)
	}

	// The raw index of point 0 is skip+1, and every raw index must fit the 32
	// bits of direction numbers. Checking the skip here rather than at the
	// first Next means a caller finds out at construction, where the mistake
	// is, instead of part-way through a run.
	if uint64(cfg.skip)+1 > maxSobolRawIndex {
		return nil, fmt.Errorf(
			"qmc: skip %d puts point 0 beyond the raw Sobol index limit %d on this platform",
			cfg.skip, maxSobolRawIndex,
		)
	}

	// Every Sobol base is 2, so the shared-factor trap reduces to one
	// condition: the leap must be odd. An even leap holds the lowest bit of the
	// raw index fixed, and that bit is the parity of the population count of
	// the Gray code — which is the leading bit of every coordinate whose
	// direction numbers all carry their own leading bit. Dimension 1 is one of
	// those in the embedded table, so an even leap confines it to a half of
	// [0,1). Neither a
	// digital shift nor an Owen scramble rescues it: both rewrite that leading
	// bit, but they rewrite it the same way for every point, so it stays
	// constant. The full argument is at WithLeap.
	leap := leapOf(cfg)
	if base, dim, conflict := leapConflict(leap, []int{2}); conflict {
		return nil, errLeapConflict(leap, base, dim, 2)
	}

	s := &Sobol{
		dims:       dims,
		directions: make([]uint32, dims*sobolBits),
		skip:       cfg.skip,
		leap:       leap,
		state:      make([]uint32, dims),
	}

	for d := 0; d < dims; d++ {
		var row *directionRow
		if d > 0 {
			row = &rows[d-1]
		}

		expandDirections(row, s.directions[d*sobolBits:(d+1)*sobolBits])
	}

	// The configuration selects one seeded randomization scheme. Keep the two
	// state representations mutually exclusive.
	switch cfg.randomize {
	case randomizeDigitalShift:
		s.shift = newDigitalShift(dims, cfg.seed)
	case randomizeOwen:
		s.owen = newOwenSeeds(dims, cfg.seed)
	}

	s.Reset()

	return s, nil
}

// loadDirectionRows returns the table to build from: the caller's if one was
// supplied, the embedded one otherwise.
func loadDirectionRows(r io.Reader) ([]directionRow, error) {
	if r == nil {
		return embeddedTable()
	}

	return parseDirectionNumbers(r)
}

// newDigitalShift derives one seeded pseudorandom word per dimension.
//
// Consecutive draws from one stream preserve shared-coordinate prefixes when
// the requested dimension count changes. The low 32 bits are the established
// output convention; the generator and these warm-up draws are reproducible.
func newDigitalShift(dims int, seed uint64) []uint32 {
	rng := splitMix64(seed)
	rng.next()
	rng.next()

	shift := make([]uint32, dims)
	for d := range shift {
		shift[d] = uint32(rng.next())
	}

	return shift
}

// Dims returns the number of dimensions.
func (s *Sobol) Dims() int { return s.dims }

// Next returns the next point of the sequence in a freshly allocated slice.
func (s *Sobol) Next() []float64 {
	out := make([]float64, s.dims)
	s.NextInto(out)

	return out
}

// NextInto writes the next point into dst and advances the cursor. It
// allocates nothing.
//
// This is the path the Gray-code ordering exists for: one XOR per dimension
// against a single direction number, chosen by the lowest zero bit of the
// counter. Compare AtInto, which has to XOR one direction number per set bit
// of the index — up to 32 of them.
//
// dst must have room for Dims() coordinates; a shorter one panics. Absorbing
// it instead would leave the tail coordinates holding zeros or stale values,
// which look like plausible positions and would steer a search silently.
func (s *Sobol) NextInto(dst []float64) {
	// A leaping generator cannot use the recurrence below at all: it advances
	// by one XOR because consecutive raw indices differ in exactly one
	// Gray-code bit, and indices leap apart do not. So it walks its own point
	// cursor through fill, which is the same work AtInto does — the cost
	// WithLeap documents. fill's index guards stand in for the exhaustion
	// panic below.
	if s.leap > 1 {
		s.fill(s.cursor, dst)
		s.cursor++

		return
	}

	if s.exhausted || uint64(s.counter) > maxSobolRawIndex {
		panic(fmt.Sprintf("qmc: the Sobol sequence is exhausted at raw index %d", maxSobolRawIndex))
	}

	// The branch is hoisted out of the loop rather than tested per dimension.
	// This avoids testing the randomization branch once per coordinate.
	if s.owen == nil {
		for d, x := range s.state {
			dst[d] = float64(x) * twoPowMinus32
		}
	} else {
		for d, x := range s.state {
			dst[d] = float64(owenScramble(x, s.owen[d])) * twoPowMinus32
		}
	}

	// Return the final admissible point normally. Only the subsequent draw
	// fails, matching At and the leaped path. A separate flag avoids wrapping
	// the counter or computing a direction number beyond its 32-bit range.
	if uint64(s.counter) == maxSobolRawIndex {
		s.exhausted = true
		return
	}

	k := lowestZeroBit(s.counter)
	for d := range s.state {
		s.state[d] ^= s.directions[d*sobolBits+k]
	}

	s.counter++
}

// Reset rewinds the stateful cursor so the next call to Next returns point 0
// again. The sequence itself is unchanged: a generator always yields the same
// points for the same configuration.
func (s *Sobol) Reset() {
	s.cursor = 0
	s.exhausted = false
	s.counter = uint32(s.skip + 1)
	s.accumulate(s.counter, s.state)
}

// At returns point i of the sequence, counting from 0, without touching the
// cursor. It is the reproducible entry point: At(i) depends only on i and the
// generator's configuration, never on how many points have been drawn, and it
// is safe to call from several goroutines at once.
//
// Point i corresponds to raw Sobol index skip+1+i*leap — skip+1+i unless
// WithLeap is in effect — matching Halton's convention in this package. Raw index 0 is the all-zeros origin — the same
// degenerate point Halton's convention exists to avoid, arrived at for a
// different reason: Halton's index 0 has no digits to invert, Sobol's selects
// no direction numbers at all. Either way it is a corner of the cube that no
// caller wants as their first sample, and with an unshifted generator it is
// exactly (0, 0, ..., 0).
//
// With leap 1, N=2^m indexed points form a guaranteed aligned net block when
// skip+1 is a multiple of N. WithSkip(q*N-1), q>=1, selects such a later block
// if all N raw indices fit the ceiling. Default skip 0 and arbitrary unaligned
// windows lack that general guarantee; leap above one also forfeits it.
// See the type documentation and docs/api-design.md.
//
// Negative i is treated as 0. The raw index must be at most the smaller of
// math.MaxInt and 2^32-1; exceeding this limit panics. Next and NextInto return
// the final admissible point normally and panic on subsequent draws until Reset.
func (s *Sobol) At(i int) []float64 {
	out := make([]float64, s.dims)
	s.fill(i, out)

	return out
}

// AtInto is At without the allocation. As with NextInto, dst shorter than
// Dims() panics rather than being silently truncated.
func (s *Sobol) AtInto(i int, dst []float64) { s.fill(i, dst) }

func (s *Sobol) fill(i int, dst []float64) {
	if i < 0 {
		i = 0
	}

	// skip is non-negative (WithSkip clamps), leap is at least 1 (WithLeap
	// clamps) and i has just been clamped, so skip+1+i*leap can only leave the
	// representable range by overflowing upwards. It is refused rather than
	// clamped, for the reason fill in halton.go gives: a wrapped sum goes
	// negative, and a negative index would be clamped back to 0 and hand back
	// the origin — the one point At documents it never returns — so the caller
	// would get a plausible-looking point that is not the point they asked
	// for. Clamping to MaxInt is the same failure wearing a different hat:
	// every index past the limit would alias onto one point with nothing to
	// show for it.
	//
	// The division is the exact test for i*leap <= MaxInt-1-skip, and it is
	// done before the multiplication rather than after, since the
	// multiplication is what would overflow. On a 64-bit platform the 32-bit
	// check below fires first for every leap; on a 32-bit one this is the
	// check that fires.
	remaining := math.MaxInt - 1 - s.skip
	if remaining < 0 || i > remaining/s.leap {
		panic(fmt.Sprintf(
			"qmc: point index %d with skip %d and leap %d overflows the raw Sobol index",
			i, s.skip, s.leap,
		))
	}

	raw := s.skip + 1 + i*s.leap

	// The direction numbers cover 32 bits of index and no more, so index 2^32
	// is not a point this generator has. Aliasing it onto index 0 by
	// truncating to uint32 is what the obvious conversion would do, and it
	// would be silent: the caller would get the origin back, or with a digital
	// shift a perfectly ordinary-looking point, for every index from 2^32
	// upwards. On a 32-bit platform int cannot reach this value at all and the
	// check is free; on a 64-bit one it is the only thing standing between a
	// long run and a sequence that quietly restarts.
	if uint64(raw) >= 1<<sobolBits {
		panic(fmt.Sprintf(
			"qmc: raw Sobol index %d needs more than %d bits; the direction numbers cover 2^%d points",
			raw, sobolBits, sobolBits,
		))
	}

	s.accumulateInto(uint32(raw), dst)
}

// accumulate writes the raw accumulator of the point at raw index n into dst.
func (s *Sobol) accumulate(n uint32, dst []uint32) {
	set, count := grayBits(n)

	for d := 0; d < s.dims; d++ {
		v := s.directions[d*sobolBits : (d+1)*sobolBits]

		var x uint32
		for j := 0; j < count; j++ {
			x ^= v[set[j]]
		}

		if s.shift != nil {
			x ^= s.shift[d]
		}

		dst[d] = x
	}
}

// accumulateInto is accumulate with the scaling folded in, so the direct path
// never materialises an intermediate uint32 slice. It is deliberately a near
// copy of accumulate rather than a wrapper around it: a wrapper would need a
// per-call scratch slice of dims words, and AtInto's entire reason for
// existing is that it allocates nothing.
func (s *Sobol) accumulateInto(n uint32, dst []float64) {
	set, count := grayBits(n)

	for d := 0; d < s.dims; d++ {
		v := s.directions[d*sobolBits : (d+1)*sobolBits]

		var x uint32
		for j := 0; j < count; j++ {
			x ^= v[set[j]]
		}

		if s.shift != nil {
			x ^= s.shift[d]
		}

		if s.owen != nil {
			x = owenScramble(x, s.owen[d])
		}

		dst[d] = float64(x) * twoPowMinus32
	}
}

// grayBits returns the positions of the set bits of gray(n), and how many
// there are.
//
// It is computed once per point rather than once per dimension because the
// selection of direction numbers is the same in every dimension — only the
// numbers themselves differ. At 39 dimensions that turns 39 bit-scans into
// one. The array is returned by value and stays on the stack; a slice would
// escape and put an allocation in the middle of AtInto.
func grayBits(n uint32) ([sobolBits]int, int) {
	var (
		set   [sobolBits]int
		count int
	)

	for c := n ^ n>>1; c != 0; c &= c - 1 {
		set[count] = bits.TrailingZeros32(c)
		count++
	}

	return set, count
}
