package qmc_test

import (
	"fmt"
	"math"

	"github.com/cwbudde/qmc"
)

// These examples are the package's documentation on pkg.go.dev, which the
// README links to with a badge. They are also tests: the `// Output:` blocks
// are compared against what the code actually prints, so an example that
// drifts out of date fails the build instead of quietly teaching the wrong
// API. That is the reason to prefer an Example over a code block in a doc
// comment.
//
// They live in package qmc_test, not qmc, so they can only reach the exported
// API — an example that compiled against an unexported helper would not
// compile for the reader copying it out.
//
// Every printed value goes through fmt.Printf with explicit precision. Raw
// float64s would make these outputs hostage to the last bit of a formatting
// change, and four decimals is already more than enough to show that the
// coordinates are where they should be.

// Draw points from a 5-dimensional sequence.
//
// The first point of an unscrambled, unskipped Halton sequence is the classic
// one: 1/2, 1/3, 1/5, 1/7, 1/11 — the reciprocal of each dimension's prime
// base. Index 0 of the raw sequence is the all-zeros origin and is never
// returned, which is why counting starts here.
func ExampleNewHalton() {
	g, err := qmc.NewHalton(5)
	if err != nil {
		panic(err)
	}

	for i := 0; i < 3; i++ {
		p := g.Next()
		fmt.Printf("point %d: %.4f %.4f %.4f %.4f %.4f\n", i, p[0], p[1], p[2], p[3], p[4])
	}
	// Output:
	// point 0: 0.5000 0.3333 0.2000 0.1429 0.0909
	// point 1: 0.2500 0.6667 0.4000 0.2857 0.1818
	// point 2: 0.7500 0.1111 0.6000 0.4286 0.2727
}

// At is the reproducible entry point: it depends only on the index and the
// generator's configuration, never on how many points have been drawn. That is
// what lets a worker pool claim indices from a shared counter and still
// reconstruct exactly which point produced which result — the property that
// makes a failed run re-runnable.
func ExampleHalton_At() {
	g, err := qmc.NewHalton(3)
	if err != nil {
		panic(err)
	}

	// Draw some points through the cursor first; At must not care.
	g.Next()
	g.Next()

	a := g.At(7)
	b := g.At(7)

	fmt.Printf("At(7)      = %.4f %.4f %.4f\n", a[0], a[1], a[2])
	fmt.Printf("again      = %.4f %.4f %.4f\n", b[0], b[1], b[2])
	fmt.Printf("identical  = %v\n", a[0] == b[0] && a[1] == b[1] && a[2] == b[2])
	// Output:
	// At(7)      = 0.0625 0.8889 0.6400
	// again      = 0.0625 0.8889 0.6400
	// identical  = true
}

// WithSkip discards a burn-in. The first few Halton points sit near a corner
// of the box in every large-base coordinate, so a few dozen points of burn-in
// is the standard remedy.
func ExampleWithSkip() {
	plain, err := qmc.NewHalton(3)
	if err != nil {
		panic(err)
	}

	skipped, err := qmc.NewHalton(3, qmc.WithSkip(64))
	if err != nil {
		panic(err)
	}

	p := plain.At(64)
	s := skipped.At(0)

	fmt.Printf("plain.At(64)   = %.4f %.4f %.4f\n", p[0], p[1], p[2])
	fmt.Printf("skipped.At(0)  = %.4f %.4f %.4f\n", s[0], s[1], s[2])
	// Output:
	// plain.At(64)   = 0.5078 0.7284 0.1360
	// skipped.At(0)  = 0.5078 0.7284 0.1360
}

// AtInto writes into a caller-owned buffer and allocates nothing, which is
// what an optimizer's inner loop wants. The buffer must have room for Dims()
// coordinates; a shorter one panics rather than being silently truncated.
func ExampleHalton_AtInto() {
	g, err := qmc.NewHalton(4, qmc.WithSkip(64))
	if err != nil {
		panic(err)
	}

	point := make([]float64, g.Dims())
	for i := 0; i < 2; i++ {
		g.AtInto(i, point)
		fmt.Printf("%d: %.4f %.4f %.4f %.4f\n", i, point[0], point[1], point[2], point[3])
	}
	// Output:
	// 0: 0.5078 0.7284 0.1360 0.3294
	// 1: 0.2578 0.1728 0.3360 0.4723
}

// Bases reports the prime base behind each dimension. It is the number that
// explains a dimension's behaviour, which is why a UI drawing the sequence
// wants it: dimension 8 uses base 23, so unscrambled it needs 23 points before
// it stops looking like a ramp.
func ExampleHalton_Bases() {
	g, err := qmc.NewHalton(10)
	if err != nil {
		panic(err)
	}

	fmt.Println(g.Bases())
	// Output:
	// [2 3 5 7 11 13 17 19 23 29]
}

// Scrambling is what makes the sequence usable above roughly twenty
// dimensions. It keeps the low-discrepancy structure but breaks the lockstep
// ramps of the high dimensions, at the cost of making the points depend on a
// seed — so fix the seed and the run is reproducible again.
func ExampleWithScrambling() {
	for _, seed := range []uint64{1, 2} {
		g, err := qmc.NewHalton(39, qmc.WithSkip(64), qmc.WithScrambling(seed))
		if err != nil {
			panic(err)
		}

		p := g.At(0)
		fmt.Printf("seed %d: dim 0 = %.4f, dim 38 = %.4f\n", seed, p[0], p[38])
	}
	// Output:
	// seed 1: dim 0 = 0.5078, dim 38 = 0.1334
	// seed 2: dim 0 = 0.4922, dim 38 = 0.2366
}

// WithLeap takes every n-th point instead of every point, which decorrelates
// Halton's high dimensions without a seed. The leap must be coprime to every
// base in use, or the coordinate whose base it shares a factor with is confined
// to a strip of width 1/base for the whole run — so the constructor refuses it.
func ExampleWithLeap() {
	// A prime above the largest base in use is coprime to all of them. At three
	// dimensions the bases are 2, 3 and 5.
	leaped, err := qmc.NewHalton(3, qmc.WithLeap(7))
	if err != nil {
		panic(err)
	}

	plain, err := qmc.NewHalton(3)
	if err != nil {
		panic(err)
	}

	l := leaped.At(2)
	p := plain.At(14) // raw index 15 either way: skip+1+i*leap with i=2, leap=7

	fmt.Printf("leaped.At(2)  = %.4f %.4f %.4f\n", l[0], l[1], l[2])
	fmt.Printf("plain.At(14)  = %.4f %.4f %.4f\n", p[0], p[1], p[2])

	// A leap sharing a factor with a base is refused, and the error names it.
	_, err = qmc.NewHalton(3, qmc.WithLeap(6))
	fmt.Println(err)

	// Output:
	// leaped.At(2)  = 0.9375 0.2593 0.1200
	// plain.At(14)  = 0.9375 0.2593 0.1200
	// qmc: leap 6 shares factor 2 with dimension 0's base 2; a leap must be coprime to every base, so pick a prime above 5
}

// StarDiscrepancy is the exact worst-case box error of a point set, and in one
// dimension it has a closed form anyone can check: the midpoints (2i-1)/(2N)
// are optimal at exactly 1/(2N), and the left endpoints i/N score exactly 1/N
// because the box [0, i/N) misses the point sitting on its own upper face.
//
// It is exact, which makes it expensive: the cost is about N^s/s!, so it
// refuses rather than hang. The refusal names what is affordable instead of
// leaving the caller to guess.
func ExampleStarDiscrepancy() {
	const n = 8

	mid := make([][]float64, n)
	left := make([][]float64, n)

	for i := 0; i < n; i++ {
		mid[i] = []float64{(2*float64(i) + 1) / (2 * n)}
		left[i] = []float64{float64(i) / n}
	}

	d, err := qmc.StarDiscrepancy(mid)
	if err != nil {
		panic(err)
	}

	fmt.Printf("midpoints       D* = %.4f (1/2N = %.4f)\n", d, 1.0/(2*n))

	d, err = qmc.StarDiscrepancy(left)
	if err != nil {
		panic(err)
	}

	fmt.Printf("left endpoints  D* = %.4f (1/N  = %.4f)\n", d, 1.0/n)

	// A scrambled Halton set in three dimensions, well inside the affordable
	// range.
	g, err := qmc.NewHalton(3, qmc.WithSkip(64), qmc.WithScrambling(1))
	if err != nil {
		panic(err)
	}

	d, err = qmc.StarDiscrepancy(qmc.Draw(g, 256))
	if err != nil {
		panic(err)
	}

	fmt.Printf("Halton 3d, 256  D* = %.4f\n", d)

	// Beyond six dimensions it refuses rather than enumerate.
	big, err := qmc.NewHalton(12)
	if err != nil {
		panic(err)
	}

	_, err = qmc.StarDiscrepancy(qmc.Draw(big, 64))
	fmt.Println("12 dims refused:", err != nil)

	// Output:
	// midpoints       D* = 0.0625 (1/2N = 0.0625)
	// left endpoints  D* = 0.1250 (1/N  = 0.1250)
	// Halton 3d, 256  D* = 0.0290
	// 12 dims refused: true
}

// CenteredL2Discrepancy has no dimension ceiling, and that is exactly the trap.
//
// For N independent uniform points its expectation is
// sqrt(((5/4)^s - (13/12)^s)/N), and that baseline is the number to compare
// against before believing anything the statistic says. At two dimensions a
// scrambled Halton set comes in far below it; at 39 dimensions the two are
// within a percent of each other, because the statistic has become its own
// diagonal — even though the same two point sets still integrate with a
// sixteenfold difference in error.
func ExampleCenteredL2Discrepancy() {
	// One point at the centre of the cube: CD2 = sqrt((13/12)^s - 1).
	cd2, err := qmc.CenteredL2Discrepancy([][]float64{{0.5, 0.5, 0.5, 0.5}})
	if err != nil {
		panic(err)
	}

	fmt.Printf("centre point, s=4:  CD2 = %.6f\n", cd2)

	for _, dims := range []int{2, 39} {
		g, err := qmc.NewHalton(dims, qmc.WithSkip(64), qmc.WithScrambling(1))
		if err != nil {
			panic(err)
		}

		cd2, err := qmc.CenteredL2Discrepancy(qmc.Draw(g, 1024))
		if err != nil {
			panic(err)
		}

		random := math.Sqrt((math.Pow(1.25, float64(dims)) - math.Pow(13.0/12.0, float64(dims))) / 1024)

		fmt.Printf("s=%2d n=1024: Halton CD2 = %.4f, random expectation = %.4f, ratio = %.2f\n",
			dims, cd2, random, random/cd2)
	}

	// Output:
	// centre point, s=4:  CD2 = 0.614299
	// s= 2 n=1024: Halton CD2 = 0.0014, random expectation = 0.0195, ratio = 13.84
	// s=39 n=1024: Halton CD2 = 2.3606, random expectation = 2.4198, ratio = 1.03
}
