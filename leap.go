package qmc

import "fmt"

// WithLeap takes every n-th point of the underlying sequence instead of every
// point.
//
// Point i then corresponds to raw index skip+1+i*n, so WithLeap(1) is exactly
// an unleaped generator and nothing about the sequence changes. Leaping is the
// third remedy this package offers for the Halton defect it exists to fix,
// after WithSkip and the two scrambling schemes, and it is the only
// deterministic one: it needs no seed, so a leaped generator is still plain
// QMC rather than RQMC and two runs of it are identical without anyone having
// to record a seed.
//
// # What it buys, measured
//
// At the package's design point — 39 dimensions, skip 64 — over forty
// admissible leaps against forty scrambling seeds, on the product integrand of
// integration_test.go at 4096 points:
//
//	unleaped, unscrambled     1.4x Monte Carlo
//	WithLeap                 54.3x
//	WithScrambling           24.4x
//	WithNestedScrambling     41.1x
//
// So it integrates better than either scrambling scheme, and it does it without
// a seed. The catch is the other statistic. Worst adjacent-pair |r| at 600
// points, over thirty leaps against thirty seeds:
//
//	unleaped, unscrambled     0.81   (one draw; it is deterministic)
//	WithLeap                  median 0.097, p90 0.23, worst 0.32
//	WithScrambling            median 0.093, p90 0.13, worst 0.16
//	WithNestedScrambling      median 0.089, p90 0.12, worst 0.14
//
// The medians agree; the tail does not. A leap only reorders the digits a
// coordinate visits, so two dimensions whose bases interact with the leap in
// commensurate ways still ramp together, and roughly one leap in ten draws such
// a pair. That is the same shape of defect the affine nested scrambling had,
// and it points the same way: leaping suits integration, where the average is
// what you get, and does not suit a parameter sweep, where the worst case is
// what you feel.
//
// It is not free either, though the reason is not the multiply. A leaped
// generator reaches raw index skip+1+i*n instead of skip+1+i, so its radical
// inverses carry about log(n) more digits: measured at 39 dimensions, AtInto
// costs 634 ns/op at a leap of 173 against 512 unleaped.
//
// # The shared-factor trap
//
// A leap sharing a factor with a base makes that coordinate worse, not better,
// and it does so silently. Every raw index is congruent to skip+1 modulo n, so
// if a prime base p divides n then that coordinate's leading base-p digit is
// the same for every point and the coordinate never leaves one strip of width
// 1/p. It stops sampling (p-1)/p of its range while still producing a
// plausible-looking spread of values inside the strip.
//
// Scrambling does not rescue it. A permuted constant digit is still a constant
// digit, so a scrambled coordinate is confined to a different strip of the same
// width — which is worse than the unscrambled case only in that it is harder to
// spot by eye.
//
// So the leap must be coprime to every base in use. The bases are the primes,
// one per dimension, so on a Halton generator over dims dimensions that means n
// must have no prime factor among the first dims primes: in practice, pick a
// prime larger than Bases()[dims-1] — larger than 167 at 39 dimensions. On a
// Sobol generator every base is 2, so n must be odd.
//
// Rather than let that go wrong quietly, NewHalton and NewSobol refuse a leap
// that shares a factor with one of their bases and name the offending base
// back to the caller. There is no way to ask for it anyway.
//
// # The trap reaches Sobol by a different route
//
// Sobol works in base 2 in every dimension, so the rule is just that n must be
// odd — but the reason is not the one the Halton argument gives, because this
// package generates in Gray-code order: the point at raw index m is the
// direct-form point at index gray(m), so a stride in m is not a stride in the
// direct-form index and no bit of gray(m) is obviously held fixed.
//
// It is held fixed all the same, and here is the mechanism. The parity of the
// population count of gray(m) is exactly m&1 — consecutive Gray codes differ in
// one bit, so the parity alternates with m. A coordinate's leading bit is the
// XOR of the leading bits of the direction numbers the Gray code selects, so
// for any dimension whose direction numbers all carry their leading bit, that
// coordinate's leading bit is that parity, which is to say m&1. An even leap
// holds m&1 constant, and that coordinate never leaves one half of [0,1).
//
// Dimension 1 is such a dimension in the embedded Joe-Kuo table — all 32 of its
// direction numbers carry bit 31 — so an even leap confines it at every skip.
// A leap divisible by 4 additionally pins dimension 0, whose leading bit is the
// low bit of gray(m). Measured at 16 dimensions over 4096 points, the
// integration error of the product integrand goes from 2.6e-04 unleaped to
// 1.2e-01 at a leap of 2 and 4.1e-01 at a leap of 4 — a factor of several
// hundred, from a change that looks like a tuning knob.
//
// # On Sobol it costs two more things, even with an odd leap
//
// It is accepted there for symmetry, and measured, but it is not the remedy to
// reach for: Sobol has no ramp defect to cure, and WithOwenScrambling or
// WithDigitalShift is what decorrelates it. An odd leap still costs Sobol:
//
//   - the (t,m,s)-net balance property, unconditionally. That property is a
//     statement about a block of 2^m consecutive raw indices beginning on a
//     multiple of 2^m — see the Sobol type doc — and a leaped run visits a
//     strided subset of the raw indices, which is not such a block at any n>1.
//     Measured at 16 dimensions over 4096 points, an odd leap of 3 integrates
//     at 8.8e-03 against 1.8e-04 unleaped: legal, and still a factor of fifty
//     worse;
//   - the Gray-code fast path on Next. NextInto advances by one XOR per
//     dimension because consecutive raw indices differ in exactly one bit;
//     indices n apart do not, so a leaped NextInto falls back to the same work
//     AtInto does.
//
// Values below 1 are treated as 1, as WithSkip treats negative values as zero.
//
// Reference: Kocis, L. & Whiten, W. J. (1997), "Computational Investigations of
// Low-Discrepancy Sequences", ACM Transactions on Mathematical Software 23(2).
func WithLeap(n int) Option {
	return func(s *settings) {
		if n < 1 {
			n = 1
		}

		s.leap = n
	}
}

// leapOf normalises the configured leap. The zero value of settings means "no
// option applied", and the neutral leap is 1 rather than 0 — a leap of 0 would
// hand back the same point forever. Normalising here, at the one place each
// constructor reads it, is what keeps the field from needing a second
// normalisation site that could drift from this one.
func leapOf(cfg settings) int {
	if cfg.leap < 1 {
		return 1
	}

	return cfg.leap
}

// leapConflict reports the first base that shares a factor with leap, and the
// dimension using it.
//
// Divisibility is the whole test: every base here is prime, so gcd(leap, p) > 1
// is exactly p | leap, and no gcd routine is needed. A leap of 1 divides
// nothing and always comes back clean.
func leapConflict(leap int, bases []int) (base, dim int, conflict bool) {
	for d, p := range bases {
		if leap%p == 0 {
			return p, d, true
		}
	}

	return 0, 0, false
}

// errLeapConflict builds the refusal both constructors return, so the two say
// the same thing. It names the base, the dimension using it, and the remedy —
// a leap coprime to every base, which for a prime-base generator means a prime
// above the largest base in use.
func errLeapConflict(leap, base, dim, largest int) error {
	return fmt.Errorf(
		"qmc: leap %d shares factor %d with dimension %d's base %d; "+
			"a leap must be coprime to every base, so pick a prime above %d",
		leap, base, dim, base, largest,
	)
}
