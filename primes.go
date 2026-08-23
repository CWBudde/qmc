package qmc

// primesUpTo returns the first n prime numbers.
//
// The bases of a Halton sequence are the primes, one per dimension, so the
// dimensionality a caller may ask for is bounded only by how many primes we
// are willing to compute. Generating them beats a hand-written table: a table
// has to be grown by hand every time a caller adds a dimension, and the growth
// is silent until some run fails at exactly the wrong moment.
//
// The bound is Rosser's theorem, p_n < n*(ln n + ln ln n) for n >= 6, with a
// small constant floor for the first few primes. Overshooting the sieve costs
// a few kilobytes; undershooting would cost correctness, so the loop below
// also grows the limit until it has found enough.
func primesUpTo(n int) []int {
	if n < 1 {
		return nil
	}
	limit := 16
	if n >= 6 {
		// ln is avoided so this stays dependency- and rounding-free: for the
		// sizes involved, 15*n is comfortably above n*(ln n + ln ln n) until
		// n is in the millions, and the loop below covers the rest anyway.
		limit = 15 * n
	}
	for {
		got := sieve(limit)
		if len(got) >= n {
			return got[:n]
		}
		limit *= 2
	}
}

// sieve returns every prime strictly below limit, by sieve of Eratosthenes.
func sieve(limit int) []int {
	if limit < 3 {
		return nil
	}
	composite := make([]bool, limit)
	out := make([]int, 0, limit/4+1)
	for i := 2; i < limit; i++ {
		if composite[i] {
			continue
		}
		out = append(out, i)
		for j := i * i; j < limit && j > 0; j += i {
			composite[j] = true
		}
	}
	return out
}
