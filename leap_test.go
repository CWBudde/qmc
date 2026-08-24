package qmc

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
)

// Leaping takes every L-th point of the underlying sequence. The whole of the
// implementation is one multiply in fill, so what these tests are for is not
// the arithmetic but the trap around it: a leap sharing a factor with a base
// pins that coordinate's leading digit for the entire run and confines the
// coordinate to one strip of width 1/base, while it goes on producing a
// plausible spread of values inside the strip. That is the failure mode this
// file exists to pin — first that the constructors refuse it, then, separately,
// that it is real, because a guard against a defect nobody has demonstrated is
// a guard that can be relaxed by accident.

// leapPrimesAbove returns n primes strictly above largest.
//
// A prime above every base in use is coprime to all of them, which is the rule
// WithLeap states and the cheapest way to satisfy it. These stand in for the
// seeds a randomized scheme would vary over: a leaped generator is
// deterministic, so the analogue of thirty streams is thirty admissible leaps.
func leapPrimesAbove(largest, n int) []int {
	out := make([]int, 0, n)
	for _, p := range primesUpTo(n * 8) {
		if p > largest {
			out = append(out, p)
			if len(out) == n {
				break
			}
		}
	}

	if len(out) < n {
		panic("qmc: leapPrimesAbove ran out of primes")
	}

	return out
}

func TestWithLeapClampsBelowOne(t *testing.T) {
	// The neutral leap is 1, not 0: a leap of 0 would return the same raw
	// index for every point. This mirrors WithSkip's clamp of negative values
	// to zero, and it is tested on settings directly because the option's own
	// contract is what is being pinned, not a generator's.
	for _, n := range []int{0, -1, -167} {
		var s settings

		WithLeap(n)(&s)

		if got := leapOf(s); got != 1 {
			t.Errorf("WithLeap(%d) must normalise to a leap of 1, got %d", n, got)
		}
	}

	var s settings
	if got := leapOf(s); got != 1 {
		t.Errorf("an unset leap must normalise to 1, got %d", got)
	}
}

// TestLeapOneIsIdenticalToNoLeap is the compatibility guarantee. WithLeap(1)
// has to be bit-identical to an unleaped generator on every path, because that
// is what lets the option exist without invalidating a single recorded output,
// golden value or digest in this package.
func TestLeapOneIsIdenticalToNoLeap(t *testing.T) {
	const points = 256

	haltonCases := []struct {
		name string
		opts []Option
	}{
		{"unscrambled", nil},
		{"scrambled", []Option{WithScrambling(7)}},
		{"nested", []Option{WithNestedScrambling(7)}},
	}

	for _, tc := range haltonCases {
		t.Run("halton/"+tc.name, func(t *testing.T) {
			plain, err := NewHalton(12, append([]Option{WithSkip(64)}, tc.opts...)...)
			if err != nil {
				t.Fatal(err)
			}

			leaped, err := NewHalton(12, append([]Option{WithSkip(64), WithLeap(1)}, tc.opts...)...)
			if err != nil {
				t.Fatal(err)
			}

			for i := 0; i < points; i++ {
				a, b := plain.At(i), leaped.At(i)
				for d := range a {
					if a[d] != b[d] {
						t.Fatalf("point %d dim %d: %v with WithLeap(1), %v without", i, d, b[d], a[d])
					}
				}
			}
		})
	}

	sobolCases := []struct {
		name string
		opts []Option
	}{
		{"plain", nil},
		{"shifted", []Option{WithDigitalShift(7)}},
		{"owen", []Option{WithOwenScrambling(7)}},
	}

	for _, tc := range sobolCases {
		t.Run("sobol/"+tc.name, func(t *testing.T) {
			plain, err := NewSobol(12, append([]Option{WithSkip(64)}, tc.opts...)...)
			if err != nil {
				t.Fatal(err)
			}

			leaped, err := NewSobol(12, append([]Option{WithSkip(64), WithLeap(1)}, tc.opts...)...)
			if err != nil {
				t.Fatal(err)
			}

			// Next as well as At: WithLeap(1) must stay on the Gray-code fast
			// path, not fall into the fill-based one the leap branch uses.
			for i := 0; i < points; i++ {
				a, b := plain.Next(), leaped.Next()
				for d := range a {
					if a[d] != b[d] {
						t.Fatalf("point %d dim %d: %v with WithLeap(1), %v without", i, d, b[d], a[d])
					}
				}
			}
		})
	}
}

// TestLeapIsStridedIndexing pins the mapping WithLeap documents: point i of a
// leaped generator is raw index skip+1+i*L, which is the point an unleaped
// generator returns at index skip+i*L. Everything else in this file relies on
// that equivalence, including the two trap demonstrations, which use an
// unleaped generator at strided indices to reach point sets the constructors
// refuse to build.
func TestLeapIsStridedIndexing(t *testing.T) {
	const (
		dims   = 8
		skip   = 64
		leap   = 173
		points = 64
	)

	t.Run("halton", func(t *testing.T) {
		plain, err := NewHalton(dims, WithScrambling(3))
		if err != nil {
			t.Fatal(err)
		}

		leaped, err := NewHalton(dims, WithSkip(skip), WithLeap(leap), WithScrambling(3))
		if err != nil {
			t.Fatal(err)
		}

		for i := 0; i < points; i++ {
			a, b := plain.At(skip+i*leap), leaped.At(i)
			for d := range a {
				if a[d] != b[d] {
					t.Fatalf("point %d dim %d: leaped %v, strided %v", i, d, b[d], a[d])
				}
			}
		}
	})

	t.Run("sobol", func(t *testing.T) {
		plain, err := NewSobol(dims, WithOwenScrambling(3))
		if err != nil {
			t.Fatal(err)
		}

		leaped, err := NewSobol(dims, WithSkip(skip), WithLeap(leap), WithOwenScrambling(3))
		if err != nil {
			t.Fatal(err)
		}

		for i := 0; i < points; i++ {
			a, b := plain.At(skip+i*leap), leaped.At(i)
			for d := range a {
				if a[d] != b[d] {
					t.Fatalf("point %d dim %d: leaped %v, strided %v", i, d, b[d], a[d])
				}
			}
		}
	})
}

// TestConstructorsRefuseALeapSharingAFactorWithABase checks the refusal and,
// as with the randomization guards, that the message names what the caller has
// to change. An error that says only "invalid leap" would leave them guessing
// which of thirty-nine bases they collided with.
func TestConstructorsRefuseALeapSharingAFactorWithABase(t *testing.T) {
	halton := []struct {
		name string
		dims int
		leap int
		want []string
	}{
		{"largest base", 39, 167, []string{"167", "dimension 38"}},
		{"base 2 via an even leap", 39, 174, []string{"factor 2", "dimension 0"}},
		{"a composite sharing one factor", 39, 3 * 173, []string{"factor 3", "dimension 1"}},
		{"a base below the ceiling", 39, 149, []string{"149", "dimension 34"}},
	}

	for _, tc := range halton {
		t.Run("halton/"+tc.name, func(t *testing.T) {
			_, err := NewHalton(tc.dims, WithLeap(tc.leap))
			if err == nil {
				t.Fatalf("leap %d shares a factor with a base and must be refused", tc.leap)
			}

			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error must mention %q, got %q", want, err.Error())
				}
			}
		})
	}

	t.Run("halton/accepts a prime above every base", func(t *testing.T) {
		if _, err := NewHalton(39, WithLeap(173)); err != nil {
			t.Fatalf("173 is coprime to every base at 39 dimensions: %v", err)
		}
	})

	// A leap legal at one dimension count is illegal at a higher one, because
	// the higher one uses more bases. 173 is a base from dimension 40 upwards.
	t.Run("halton/the legal set shrinks as dimensions grow", func(t *testing.T) {
		if _, err := NewHalton(40, WithLeap(173)); err == nil {
			t.Fatal("173 is dimension 39's base at 40 dimensions and must be refused there")
		}
	})

	t.Run("sobol/even", func(t *testing.T) {
		for _, leap := range []int{2, 4, 174, 1024} {
			_, err := NewSobol(8, WithLeap(leap))
			if err == nil {
				t.Fatalf("every Sobol base is 2, so leap %d must be refused", leap)
			}

			if !strings.Contains(err.Error(), "factor 2") {
				t.Errorf("error must name the shared factor, got %q", err.Error())
			}
		}
	})

	t.Run("sobol/odd is accepted", func(t *testing.T) {
		for _, leap := range []int{1, 3, 167, 173} {
			if _, err := NewSobol(8, WithLeap(leap)); err != nil {
				t.Fatalf("odd leap %d is coprime to base 2: %v", leap, err)
			}
		}
	})
}

// TestASharedFactorConfinesTheHaltonCoordinate demonstrates the defect the
// guard refuses, which is the half of this that a guard alone cannot show.
//
// The point set is reached through an unleaped generator at strided indices —
// the equivalence TestLeapIsStridedIndexing pins — because the constructor will
// not build the leaping generator that produces it. That is the point: this is
// what a caller would have got.
func TestASharedFactorConfinesTheHaltonCoordinate(t *testing.T) {
	const (
		dims   = 39
		base   = 167 // dimension 38's base
		trap   = 38
		points = 2000
	)

	cases := []struct {
		name string
		opts []Option
	}{
		{"unscrambled", nil},
		// Scrambling does not rescue it. A permuted constant digit is still a
		// constant digit, so the coordinate moves to a different strip of the
		// same width — which is worse only in being harder to see.
		{"scrambled", []Option{WithScrambling(11)}},
		{"nested", []Option{WithNestedScrambling(11)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := NewHalton(dims, tc.opts...)
			if err != nil {
				t.Fatal(err)
			}

			strip := -1

			for i := 0; i < points; i++ {
				v := g.At(i * base)[trap]

				got := int(v * base)
				if strip == -1 {
					strip = got

					continue
				}

				if got != strip {
					t.Fatalf(
						"point %d escaped the strip: %v is in strip %d, every earlier point was in %d",
						i, v, got, strip,
					)
				}
			}

			t.Logf(
				"%s: a leap of %d confined dimension %d to [%g, %g) over %d points — %.1f%% of its range unsampled",
				tc.name, base, trap,
				float64(strip)/base, float64(strip+1)/base, points,
				100*float64(base-1)/base,
			)

			// The control: a coordinate whose base does not divide the leap
			// must not be confined, or the assertion above would be measuring
			// the generator rather than the trap.
			free := map[int]bool{}
			for i := 0; i < points; i++ {
				free[int(g.At(i * base)[0]*2)] = true
			}

			if len(free) != 2 {
				t.Errorf("dimension 0's base does not divide %d, so it must not be confined", base)
			}
		})
	}
}

// TestAnEvenLeapConfinesASobolCoordinate is the same demonstration in base 2.
//
// It confines one coordinate rather than all of them, and the reason is worth
// having in a test rather than only in a doc comment. This package generates in
// Gray-code order, so raw index m produces the direct-form point at gray(m) and
// an even stride in m is not an even stride in the direct-form index. What an
// even leap holds fixed is m&1, which is exactly the parity of the population
// count of gray(m) — consecutive Gray codes differ in one bit, so that parity
// alternates with m. A coordinate whose direction numbers all carry their
// leading bit therefore has that parity as its own leading bit, and is pinned.
//
// Dimension 1 is that coordinate in the embedded table, which the first subtest
// checks directly so that the rest is a consequence rather than a coincidence.
func TestAnEvenLeapConfinesASobolCoordinate(t *testing.T) {
	const (
		dims   = 16
		points = 1000
		pinned = 1 // the dimension the mechanism below predicts
	)

	t.Run("the mechanism", func(t *testing.T) {
		g, err := NewSobol(dims)
		if err != nil {
			t.Fatal(err)
		}

		// Every direction number of the pinned dimension carries bit 31, which
		// is what makes its leading output bit the Gray code's parity.
		for j, v := range g.directions[pinned*sobolBits : (pinned+1)*sobolBits] {
			if v&0x80000000 == 0 {
				t.Fatalf("direction number %d of dimension %d does not carry bit 31 (%#08x); "+
					"the confinement below no longer follows and this test needs rewriting", j, pinned, v)
			}
		}

		// And the parity identity the argument rests on.
		for m := uint32(1); m < 1<<16; m++ {
			set, count := grayBits(m)
			_ = set

			if count%2 != int(m&1) {
				t.Fatalf("parity of popcount(gray(%d)) is %d, want %d", m, count%2, m&1)
			}
		}
	})

	cases := []struct {
		name string
		opts []Option
	}{
		{"plain", nil},
		// Neither randomization helps: both rewrite the leading bit, but they
		// rewrite it the same way for every point, so it stays constant.
		{"shifted", []Option{WithDigitalShift(11)}},
		{"owen", []Option{WithOwenScrambling(11)}},
	}

	// Across several burn-ins, because a confinement that held only at skip 0
	// would be a property of one starting index rather than of the leap.
	for _, skip := range []int{0, 1, 64, 255, 1000} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/skip=%d", tc.name, skip), func(t *testing.T) {
				g, err := NewSobol(dims, tc.opts...)
				if err != nil {
					t.Fatal(err)
				}

				half := -1

				for i := 0; i < points; i++ {
					v := g.At(skip + i*2)[pinned]

					h := int(v * 2)
					if half == -1 {
						half = h

						continue
					}

					if h != half {
						t.Fatalf("point %d escaped: %v is in half %d, every earlier point was in %d",
							i, v, h, half)
					}
				}

				// The control: dimension 0's leading bit is the low bit of the
				// Gray code rather than its parity, so an even leap does not
				// pin it. Without this the assertion above could be measuring a
				// broken generator instead of the trap.
				free := map[int]bool{}
				for i := 0; i < points; i++ {
					free[int(g.At(skip + i*2)[0]*2)] = true
				}

				if len(free) != 2 {
					t.Errorf("dimension 0 must not be confined by an even leap; it reached %d of 2 halves",
						len(free))
				}
			})
		}
	}
}

// TestAnEvenLeapWrecksSobolIntegration is the consequence a caller would
// actually feel, and the reason the constructor refuses rather than warns. One
// pinned coordinate out of sixteen is not a curiosity: it moves the integration
// error by more than two orders of magnitude.
func TestAnEvenLeapWrecksSobolIntegration(t *testing.T) {
	const (
		dims = 16
		n    = 4096
	)

	err := func(leap int) float64 {
		t.Helper()

		g, e := NewSobol(dims)
		if e != nil {
			t.Fatal(e)
		}

		point := make([]float64, dims)
		sum := 0.0

		for i := 0; i < n; i++ {
			g.AtInto(i*leap, point)
			sum += nestedIntegrand(point)
		}

		return math.Abs(sum/float64(n) - 1.0)
	}

	unleaped, even, odd := err(1), err(2), err(3)

	t.Logf("at %d dims over %d points: unleaped %.3e, even leap (2) %.3e, odd leap (3) %.3e",
		dims, n, unleaped, even, odd)

	if even < unleaped*50 {
		t.Errorf("an even leap must wreck the sequence, not merely dent it: %.3e against %.3e unleaped",
			even, unleaped)
	}

	// The odd leap is legal and still costs something — it forfeits the net
	// balance, which is why WithLeap says so rather than recommending it here.
	if odd <= unleaped {
		t.Logf("note: the odd leap did not cost anything measurable at this n (%.3e against %.3e)",
			odd, unleaped)
	}
}

// TestLeapedNextMatchesAtAndResetRewinds is the test that pins Sobol's leaping
// path against fill. A leaping Sobol cannot use the Gray-code recurrence, so
// NextInto takes a different branch from the one every other Sobol test
// exercises; without this, that branch could disagree with At and nothing would
// notice.
func TestLeapedNextMatchesAtAndResetRewinds(t *testing.T) {
	const (
		dims   = 8
		leap   = 173
		points = 128
	)

	cases := []struct {
		name string
		make func() (Sequence, error)
	}{
		{"halton", func() (Sequence, error) {
			return NewHalton(dims, WithSkip(64), WithLeap(leap), WithScrambling(5))
		}},
		{"sobol", func() (Sequence, error) {
			return NewSobol(dims, WithSkip(64), WithLeap(leap), WithOwenScrambling(5))
		}},
		{"sobol/unrandomized", func() (Sequence, error) {
			return NewSobol(dims, WithLeap(leap))
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := tc.make()
			if err != nil {
				t.Fatal(err)
			}

			for i := 0; i < points; i++ {
				a, b := g.Next(), g.At(i)
				for d := range a {
					if a[d] != b[d] {
						t.Fatalf("point %d dim %d: Next %v, At %v", i, d, a[d], b[d])
					}
				}
			}

			g.Reset()

			first, want := g.Next(), g.At(0)
			for d := range first {
				if first[d] != want[d] {
					t.Fatalf("after Reset, dim %d: Next %v, At(0) %v", d, first[d], want[d])
				}
			}
		})
	}
}

// TestLeapOverflowIsRefused covers the multiplication the leap adds to the
// index arithmetic. skip+1+i overflowed once before and returned the origin in
// silence (see overflow_test.go); skip+1+i*leap is the same hazard reached
// leap times sooner.
//
// math.MaxInt rather than a literal, so this still compiles where int is 32
// bits wide; CI runs a GOARCH=386 leg.
func TestLeapOverflowIsRefused(t *testing.T) {
	const leap = 173

	t.Run("halton", func(t *testing.T) {
		g, err := NewHalton(2, WithSkip(64), WithLeap(leap))
		if err != nil {
			t.Fatal(err)
		}

		last := (math.MaxInt - 1 - 64) / leap

		// The last admissible index is a real point, not a near miss.
		if got := g.At(last); got[0] <= 0 || got[0] >= 1 {
			t.Fatalf("the last representable index must yield a point in (0,1), got %v", got)
		}

		defer func() {
			if recover() == nil {
				t.Fatal("an index whose leap multiplication overflows must be refused, not wrapped")
			}
		}()

		g.At(last + 1)
	})

	// Sobol has a second, much lower ceiling — the direction numbers cover 32
	// bits of raw index, and a leap reaches it leap times sooner — but reaching
	// it needs indices that do not fit a 32-bit int. See leap_64bit_test.go.
}

// TestLeapingBreaksHighDimensionalCorrelation is the measurement that decides
// whether the option earns its place, at the package's design point: 39 knobs
// on a 600-evaluation budget, where an unscrambled Halton sequence's last
// coordinates have not left their first period and ramp in lockstep at 0.81
// (TestUnscrambledStillShowsTheDefect).
//
// A leaped generator is deterministic, so there are no seeds to average over.
// Thirty admissible leaps play the part thirty seeds play for a scrambling
// scheme, and for the same reason: the statistic is high-variance, and one draw
// of it says nothing. Median and tail, not a single number.
func TestLeapingBreaksHighDimensionalCorrelation(t *testing.T) {
	const (
		leaps     = 30
		tolerance = 0.35
	)

	bases := primesUpTo(corrDims)

	worsts := make([]float64, 0, leaps)

	worstOverall, worstLeap, worstPair := 0.0, 0, 0

	for _, leap := range leapPrimesAbove(bases[corrDims-1], leaps) {
		g, err := NewHalton(corrDims, WithSkip(corrSkip), WithLeap(leap))
		if err != nil {
			t.Fatal(err)
		}

		worst, pair := worstAdjacentCorrelation(Draw(g, corrPoints))
		worsts = append(worsts, worst)

		if worst > worstOverall {
			worstOverall, worstLeap, worstPair = worst, leap, pair
		}
	}

	sort.Float64s(worsts)

	med := worsts[len(worsts)/2]
	p90 := worsts[(len(worsts)*9)/10]

	t.Logf("leaped, unscrambled: worst adjacent-pair |r| over %d leaps — median %.4f, p90 %.4f, worst %.4f (leap %d, dims %d/%d)",
		leaps, med, p90, worstOverall, worstLeap, worstPair, worstPair+1)

	// The gate is deliberately far above the measured tail and far below the
	// 0.81 an unleaped unscrambled generator shows. What it asserts is that
	// leaping removes the ramp defect at all, not any particular constant —
	// the constant is what the log line above is for.
	if worstOverall > tolerance {
		t.Errorf("worst adjacent-pair |r| over %d leaps is %.4f at leap %d, want <= %.2f",
			leaps, worstOverall, worstLeap, tolerance)
	}
}

// TestLeapingIntegratesBetterThanAnUnleapedSequence is the other half of the
// measurement, on the statistic integration_test.go uses.
//
// Forty leaps rather than ten, for the reason recorded in
// docs/testing-methodology.md: a ten-stream figure cannot separate two good
// schemes, and two statistically identical constructions once read 44.0x and
// 31.9x on the same ten seeds.
func TestLeapingIntegratesBetterThanAnUnleapedSequence(t *testing.T) {
	const (
		dims    = 39
		n       = 4096
		streams = 40
	)

	bases := primesUpTo(dims)
	leaps := leapPrimesAbove(bases[dims-1], streams)

	point := make([]float64, dims)

	rmsOver := func(build func(leap int) (*Halton, error)) float64 {
		t.Helper()

		sumSq := 0.0

		for _, leap := range leaps {
			g, err := build(leap)
			if err != nil {
				t.Fatal(err)
			}

			sum := 0.0

			for i := 0; i < n; i++ {
				g.AtInto(i, point)
				sum += nestedIntegrand(point)
			}

			e := sum/float64(n) - 1.0
			sumSq += e * e
		}

		return math.Sqrt(sumSq / float64(streams))
	}

	leapedErr := rmsOver(func(leap int) (*Halton, error) {
		return NewHalton(dims, WithSkip(corrSkip), WithLeap(leap))
	})

	// The unleaped comparison is a single deterministic run, so it is repeated
	// across the same forty configurations only to keep the two numbers the
	// same shape; every term is identical.
	plainErr := rmsOver(func(_ int) (*Halton, error) {
		return NewHalton(dims, WithSkip(corrSkip))
	})

	scrambledErr := nestedRMSError(t, dims, n, streams, WithScrambling)
	nestedErr := nestedRMSError(t, dims, n, streams, WithNestedScrambling)
	mcErr := nestedMCError(dims, n, streams)

	// All four measured in one run, so the README table they feed is a
	// comparison rather than four numbers from four sittings.
	t.Logf("at %d dims, n=%d, over %d streams: unleaped %.3e (%.1fx MC), leaped %.3e (%.1fx MC), scrambled %.3e (%.1fx MC), nested %.3e (%.1fx MC), MC %.3e",
		dims, n, streams,
		plainErr, mcErr/plainErr,
		leapedErr, mcErr/leapedErr,
		scrambledErr, mcErr/scrambledErr,
		nestedErr, mcErr/nestedErr,
		mcErr)

	if leapedErr >= plainErr {
		t.Errorf("leaping must improve on an unleaped unscrambled sequence: leaped %.3e, unleaped %.3e",
			leapedErr, plainErr)
	}

	// No gate against scrambling in either direction. Which of the two wins is
	// the measurement's business, not a property the package promises, and
	// pinning an ordering here would turn a change of scrambling constants into
	// a failure in a file about leaping.
}
