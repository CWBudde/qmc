package qmc_test

import (
	"testing"

	"github.com/cwbudde/qmc"
)

// Reusing the same binary permutation at every position sends 0.1000... to
// either 0.1000... or 0.0111..., both 0.5. This counterexample uses the public
// API, without assuming which permutation a seed selects.
func TestFixedDigitScramblingDoesNotGiveUniformMarginals(t *testing.T) {
	const seeds = 1024
	for seed := range seeds {
		g, err := qmc.NewHalton(1, qmc.WithScrambling(uint64(seed)))
		if err != nil {
			t.Fatal(err)
		}

		x := g.At(0)[0]
		if x != 0.5 || x*x != 0.25 {
			t.Fatalf("seed %d: first point %g, square %g; want 0.5 and 0.25", seed, x, x*x)
		}
	}
	// Every estimate is 0.25, so the seed variance is zero despite a bias of
	// 0.25 - 1/3 = -1/12 relative to the continuous integral of x squared.
}
