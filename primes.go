package qmc

import (
	"fmt"
	"math"
)

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
func primesUpTo(n int) ([]int, error) {
	if n < 1 {
		return nil, nil
	}

	limit := 16

	if n >= 6 {
		// This initial estimate can undershoot. Growth below is checked too.
		if n > math.MaxInt/15 {
			return nil, fmt.Errorf("qmc: %d dimensions overflow the initial prime sieve bound", n)
		}

		limit = 15 * n
	}

	return primesFromLimit(n, limit)
}

// primesFromLimit lets bounded tests force expansion without large allocations.
func primesFromLimit(n, limit int) ([]int, error) {
	for {
		got := sieve(limit)
		if len(got) >= n {
			return got[:n], nil
		}

		next, err := growPrimeLimit(limit)
		if err != nil {
			return nil, err
		}

		limit = next
	}
}

func growPrimeLimit(limit int) (int, error) {
	if limit < 1 || limit > math.MaxInt/2 {
		return 0, fmt.Errorf("qmc: prime sieve bound %d cannot be doubled on this platform", limit)
	}

	return limit * 2, nil
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

		// i*i is computed in uint64 first. Where int is 32 bits the product can
		// wrap to a positive value, pass a `j > 0` guard, and mark a slot it has
		// no business marking — dropping a real prime, so that dimension gets a
		// different base than it would on a 64-bit build. A sequence that
		// depends on GOARCH is the one thing a reproducible generator may not be.
		if uint64(i)*uint64(i) >= uint64(limit) {
			continue
		}

		for j := i * i; j < limit; {
			composite[j] = true
			if j > limit-1-i {
				break
			}

			j += i
		}
	}

	return out
}
