package qmc_test

import (
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/cwbudde/qmc"
)

// Tests for the two discrepancy statistics.
//
// Three kinds of assertion appear here and they earn their places differently.
// The closed forms are exact and are what a reader can check on paper. The
// brute-force and numerical-integration references are independent
// reimplementations of the *definitions*, and they are what earns the right to
// prune the star walk and to use Hickernell's closed form at all — an
// optimisation checked only against itself is not checked. The statistical
// gates assert an ordering and a factor, never a measured constant, and log
// the real number so a drift is visible without being a failure.

// ---------------------------------------------------------------- closed forms

// TestStarDiscrepancyMatchesTheOneDimensionalClosedForm pins the two point
// sets whose star discrepancy anyone can derive in a line. The midpoints
// (2i-1)/(2N) are the optimal N-point set in one dimension and score exactly
// 1/(2N); the left endpoints i/N score exactly 1/N because the box [0, i/N)
// misses the point sitting on its own upper face.
//
// The tolerance is four ulps, not zero. The identity is exact in real
// arithmetic but the walk does not evaluate it the way the closed form is
// written: at N=3 the maximum is reached as 5/6 - 2/3, which rounds one ulp
// away from the 1/6 the formula names. Four ulps is far too tight to hide the
// off-by-one in the strict/inclusive counts this test exists to catch — that
// mistake moves the answer by 1/N, which is 1e15 ulps.
func TestStarDiscrepancyMatchesTheOneDimensionalClosedForm(t *testing.T) {
	for _, n := range []int{1, 2, 3, 8, 17, 64} {
		mid := make([][]float64, n)
		left := make([][]float64, n)

		for i := 0; i < n; i++ {
			mid[i] = []float64{(2*float64(i) + 1) / (2 * float64(n))}
			left[i] = []float64{float64(i) / float64(n)}
		}

		got, err := qmc.StarDiscrepancy(mid)
		if err != nil {
			t.Fatal(err)
		}

		if want := 1 / (2 * float64(n)); !withinUlps(got, want, 4) {
			t.Fatalf("N=%d midpoints: D* = %v, want %v", n, got, want)
		}

		got, err = qmc.StarDiscrepancy(left)
		if err != nil {
			t.Fatal(err)
		}

		if want := 1 / float64(n); !withinUlps(got, want, 4) {
			t.Fatalf("N=%d left endpoints: D* = %v, want %v", n, got, want)
		}
	}
}

// TestStarDiscrepancyOfASinglePointMatchesItsClosedForm covers the case where
// the two halves of the supremum are visibly different quantities. With one
// point x the largest overshoot is the whole cube minus that point,
// 1 - prod_k x_k, and the largest undershoot is the largest box that still
// contains it on its closed corner, max_k x_k.
func TestStarDiscrepancyOfASinglePointMatchesItsClosedForm(t *testing.T) {
	for _, p := range [][]float64{
		{0.5},
		{0.9},
		{0.1},
		{0.5, 0.5},
		{0.2, 0.8},
		{0.3, 0.3, 0.3},
		{0.9, 0.9, 0.9, 0.9},
		{0.1, 0.4, 0.7, 0.95},
	} {
		got, err := qmc.StarDiscrepancy([][]float64{p})
		if err != nil {
			t.Fatal(err)
		}

		maxCoord, vol := 0.0, 1.0
		for _, x := range p {
			maxCoord = math.Max(maxCoord, x)
			vol *= x
		}

		if want := math.Max(maxCoord, 1-vol); !withinUlps(got, want, 4) {
			t.Fatalf("single point %v: D* = %v, want %v", p, got, want)
		}
	}
}

// TestCenteredL2MatchesItsClosedFormOnOnePoint uses the two single-point cases
// that can be integrated by hand. A point at the centre of the cube has
// u = 0 everywhere and reduces the whole formula to (13/12)^s - 1; the origin
// in one dimension gives 1/3.
func TestCenteredL2MatchesItsClosedFormOnOnePoint(t *testing.T) {
	for s := 1; s <= 6; s++ {
		centre := make([]float64, s)
		for k := range centre {
			centre[k] = 0.5
		}

		got, err := qmc.CenteredL2Discrepancy([][]float64{centre})
		if err != nil {
			t.Fatal(err)
		}

		want := math.Sqrt(math.Pow(13.0/12.0, float64(s)) - 1)
		if math.Abs(got-want) > 1e-14 {
			t.Fatalf("s=%d centre point: CD2 = %v, want %v", s, got, want)
		}
	}

	got, err := qmc.CenteredL2Discrepancy([][]float64{{0}})
	if err != nil {
		t.Fatal(err)
	}

	if want := math.Sqrt(1.0 / 3.0); math.Abs(got-want) > 1e-15 {
		t.Fatalf("s=1 origin: CD2 = %v, want %v", got, want)
	}
}

// ----------------------------------------------------------- independent refs

// starBruteForce is the definition, written as slowly and as literally as
// possible: the full Cartesian product of every dimension's candidate grid,
// with both counts obtained by rescanning the entire point set at every box.
//
// It shares no code with the implementation. That is the point — the pruned
// walk's survivor lists, its candidate restriction and its cutoff are all
// optimisations, and this is the thing they are optimisations *of*.
func starBruteForce(points [][]float64) float64 {
	s := len(points[0])
	n := float64(len(points))

	grids := make([][]float64, s)

	for k := 0; k < s; k++ {
		seen := map[float64]bool{1: true}

		for _, p := range points {
			seen[p[k]] = true
		}

		for v := range seen {
			grids[k] = append(grids[k], v)
		}
	}

	b := make([]float64, s)
	dPlus, dMinus := 0.0, 0.0

	var walk func(k int)

	walk = func(k int) {
		if k == s {
			vol := 1.0
			for _, bk := range b {
				vol *= bk
			}

			lt, le := 0, 0

			for _, p := range points {
				inLT, inLE := true, true

				for j, x := range p {
					if x >= b[j] {
						inLT = false
					}

					if x > b[j] {
						inLE = false
					}
				}

				if inLT {
					lt++
				}

				if inLE {
					le++
				}
			}

			dPlus = math.Max(dPlus, vol-float64(lt)/n)
			dMinus = math.Max(dMinus, float64(le)/n-vol)

			return
		}

		for _, v := range grids[k] {
			b[k] = v

			walk(k + 1)
		}
	}

	walk(0)

	return math.Max(dPlus, dMinus)
}

// TestStarDiscrepancyAgreesWithBruteForceEnumeration is the test that earns
// the pruning.
//
// The adversarial sets at the end are not filler. All-identical points and
// heavy duplicate coordinates are what catch a missing de-duplication in the
// candidate sweep and any confusion between the strict and the inclusive
// count; all-zero and all-one coordinates are the two boundaries where the
// half-open box convention decides the answer on its own.
func TestStarDiscrepancyAgreesWithBruteForceEnumeration(t *testing.T) {
	rng := rand.New(rand.NewSource(20240824)) //nolint:gosec // test fixture, not cryptography

	type set struct {
		name   string
		points [][]float64
	}

	var sets []set

	for _, c := range []struct{ s, n int }{{1, 25}, {2, 25}, {3, 18}, {4, 10}} {
		g, err := qmc.NewHalton(c.s, qmc.WithSkip(64), qmc.WithScrambling(uint64(c.s)))
		if err != nil {
			t.Fatal(err)
		}

		sets = append(sets, set{
			name:   "halton",
			points: qmc.Draw(g, c.n),
		})

		pts := make([][]float64, c.n)
		for i := range pts {
			pts[i] = make([]float64, c.s)
			for k := range pts[i] {
				pts[i][k] = rng.Float64()
			}
		}

		sets = append(sets, set{name: "random", points: pts})
	}

	sets = append(
		sets,
		set{name: "all identical", points: repeated([]float64{0.3, 0.7, 0.1}, 9)},
		set{name: "all zero", points: repeated([]float64{0, 0, 0}, 7)},
		set{name: "all one", points: repeated([]float64{1, 1, 1}, 7)},
		set{name: "heavy duplicate coordinates", points: duplicateHeavy(rng, 3, 20, 3)},
		set{name: "duplicates on the boundary", points: duplicateHeavy(rng, 2, 24, 2)},
	)

	for _, s := range sets {
		got, err := qmc.StarDiscrepancy(s.points)
		if err != nil {
			t.Fatalf("%s (%dx%d): %v", s.name, len(s.points), len(s.points[0]), err)
		}

		want := starBruteForce(s.points)
		if got != want {
			t.Fatalf("%s (%d points, %d dims): D* = %v, brute force says %v",
				s.name, len(s.points), len(s.points[0]), got, want)
		}
	}
}

// withinUlps compares two positive results to within a fixed number of
// units in the last place. It is the right comparison for an identity that
// holds exactly in the reals but is reached along a different arithmetic path.
func withinUlps(got, want float64, ulps int) bool {
	tol := float64(ulps)*math.Nextafter(want, math.Inf(1)) - float64(ulps)*want

	return math.Abs(got-want) <= tol
}

func repeated(p []float64, n int) [][]float64 {
	out := make([][]float64, n)
	for i := range out {
		out[i] = append([]float64(nil), p...)
	}

	return out
}

// duplicateHeavy draws coordinates from a tiny ladder of values, including 0
// and 1, so that almost every coordinate is shared by several points.
func duplicateHeavy(rng *rand.Rand, s, n, levels int) [][]float64 {
	out := make([][]float64, n)
	for i := range out {
		out[i] = make([]float64, s)
		for k := range out[i] {
			out[i][k] = float64(rng.Intn(levels+1)) / float64(levels)
		}
	}

	return out
}

// TestCenteredL2MatchesItsDefiningIntegral is the only test that would catch a
// wrong anchoring convention, and the only one that pins the sum over
// projections.
//
// Hickernell's closed form is the evaluated integral of the squared local
// discrepancy over boxes anchored at the cube's *nearest corner*, summed over
// every non-empty subset of the coordinates. Both halves of that sentence are
// easy to get wrong and neither is visible in any other test here: anchoring
// the boxes at the centre is the natural misreading of the name, and dropping
// the sum over subsets leaves a statistic that still looks plausible and is
// off by a factor of 20 in two dimensions. So this integrates the definition
// numerically and compares.
func TestCenteredL2MatchesItsDefiningIntegral(t *testing.T) {
	oneDim := [][]float64{{0.1}, {0.42}, {0.5}, {0.63}, {0.97}}

	got, err := qmc.CenteredL2Discrepancy(oneDim)
	if err != nil {
		t.Fatal(err)
	}

	want := integrateCenteredDiscrepancy(oneDim, 200000, 2000)
	if rel := math.Abs(got-want) / want; rel > 1e-3 {
		t.Fatalf("1-D: CD2 = %v, numeric integral says %v (%.2e relative)", got, want, rel)
	}

	t.Logf("1-D: closed form %.9f, numeric integral %.9f", got, want)

	twoDim := [][]float64{{0.13, 0.71}, {0.44, 0.09}, {0.68, 0.55}, {0.91, 0.33}, {0.27, 0.88}}

	got, err = qmc.CenteredL2Discrepancy(twoDim)
	if err != nil {
		t.Fatal(err)
	}

	want = integrateCenteredDiscrepancy(twoDim, 200000, 2000)
	if rel := math.Abs(got-want) / want; rel > 1e-3 {
		t.Fatalf("2-D: CD2 = %v, numeric integral says %v (%.2e relative)", got, want, rel)
	}

	t.Logf("2-D: closed form %.9f, numeric integral %.9f", got, want)
}

// integrateCenteredDiscrepancy evaluates the definition
//
//	CD2^2 = sum over non-empty u subset of {1..s} of
//	          integral over [0,1]^|u| of ( #{i : x_iu in J}/N - vol(J) )^2 dt
//
// where J is the |u|-dimensional box anchored at the corner of the cube
// nearest t: [0,t_k) below the half-way mark and [t_k,1) above it. The sum
// over projections is not decoration — it is what makes the closed form's
// (13/12)^s factor into 2^s subset contributions, and a single s-dimensional
// integral reproduces only the last of them.
//
// The midpoint rule with m cells per dimension is used because the integrand
// is piecewise smooth with jumps at the points' own coordinates; the error
// falls off like 1/m, which is why the low-dimensional projections get a much
// finer grid than the full one.
func integrateCenteredDiscrepancy(points [][]float64, m1, mRest int) float64 {
	s := len(points[0])
	total := 0.0

	for mask := 1; mask < 1<<s; mask++ {
		dims := make([]int, 0, s)

		for k := 0; k < s; k++ {
			if mask&(1<<k) != 0 {
				dims = append(dims, k)
			}
		}

		cells := mRest
		if len(dims) == 1 {
			cells = m1
		}

		total += integrateProjection(points, dims, cells)
	}

	return math.Sqrt(total)
}

// integrateProjection is one term of the sum above: the mean squared local
// discrepancy over the coordinates named by dims.
func integrateProjection(points [][]float64, dims []int, m int) float64 {
	n := float64(len(points))
	t := make([]float64, len(dims))
	total, cells := 0.0, 0

	var walk func(d int)

	walk = func(d int) {
		if d == len(dims) {
			vol := 1.0

			for _, tk := range t {
				if tk < 0.5 {
					vol *= tk
				} else {
					vol *= 1 - tk
				}
			}

			count := 0

			for _, p := range points {
				inside := true

				for j, k := range dims {
					x := p[k]
					if t[j] < 0.5 {
						if x >= t[j] {
							inside = false
						}
					} else if x < t[j] {
						inside = false
					}
				}

				if inside {
					count++
				}
			}

			e := float64(count)/n - vol
			total += e * e
			cells++

			return
		}

		for i := 0; i < m; i++ {
			t[d] = (float64(i) + 0.5) / float64(m)

			walk(d + 1)
		}
	}

	walk(0)

	return total / float64(cells)
}

// --------------------------------------------------------------- invariants

// TestDiscrepancyDoesNotDependOnPointOrder pins that both statistics are
// functions of the set, not of the slice. Star is a supremum over boxes and
// comes out bit-identical; CD2 sums N^2 products whose order the shuffle
// changes, so it is held to 1e-12 instead.
func TestDiscrepancyDoesNotDependOnPointOrder(t *testing.T) {
	g, err := qmc.NewHalton(3, qmc.WithSkip(64), qmc.WithScrambling(11))
	if err != nil {
		t.Fatal(err)
	}

	pts := qmc.Draw(g, 200)

	star, err := qmc.StarDiscrepancy(pts)
	if err != nil {
		t.Fatal(err)
	}

	cd2, err := qmc.CenteredL2Discrepancy(pts)
	if err != nil {
		t.Fatal(err)
	}

	rng := rand.New(rand.NewSource(99)) //nolint:gosec // test fixture, not cryptography

	shuffled := append([][]float64(nil), pts...)
	rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

	gotStar, err := qmc.StarDiscrepancy(shuffled)
	if err != nil {
		t.Fatal(err)
	}

	if gotStar != star {
		t.Fatalf("shuffled: D* = %v, want exactly %v", gotStar, star)
	}

	gotCD2, err := qmc.CenteredL2Discrepancy(shuffled)
	if err != nil {
		t.Fatal(err)
	}

	if rel := math.Abs(gotCD2-cd2) / cd2; rel > 1e-12 {
		t.Fatalf("shuffled: CD2 = %v, want %v (%.2e relative)", gotCD2, cd2, rel)
	}
}

// TestCenteredL2IsUnchangedByReflectingACoordinate pins the defining symmetry
// of the *centred* discrepancy. Mapping x -> 1-x in any one dimension swaps
// which corner each box is anchored at, and the statistic is built so that it
// does not notice. A centre-anchored variant would fail this.
func TestCenteredL2IsUnchangedByReflectingACoordinate(t *testing.T) {
	g, err := qmc.NewHalton(6, qmc.WithSkip(64), qmc.WithScrambling(3))
	if err != nil {
		t.Fatal(err)
	}

	pts := qmc.Draw(g, 300)

	want, err := qmc.CenteredL2Discrepancy(pts)
	if err != nil {
		t.Fatal(err)
	}

	for _, k := range []int{0, 2, 5} {
		flipped := make([][]float64, len(pts))
		for i, p := range pts {
			q := append([]float64(nil), p...)
			q[k] = 1 - q[k]
			flipped[i] = q
		}

		got, err := qmc.CenteredL2Discrepancy(flipped)
		if err != nil {
			t.Fatal(err)
		}

		if rel := math.Abs(got-want) / want; rel > 1e-12 {
			t.Fatalf("reflecting dimension %d: CD2 = %v, want %v (%.2e relative)", k, got, want, rel)
		}
	}
}

// TestDiscrepancyIsReproducible is the architecture-independence gate.
//
// The star walk is products and differences with no multiply-add shape, so it
// is bit-identical everywhere and is compared with ==. CD2's inner product is
// 1 + a + b - c multiplied into an accumulator, which a Go compiler is free to
// fuse on arm64, so it gets a relative tolerance instead. Promising bit
// identity there would be a promise the language does not make.
func TestDiscrepancyIsReproducible(t *testing.T) {
	const (
		goldenStar = 0.04834997427983534
		goldenCD2  = 0.015030931896574203
	)

	g, err := qmc.NewHalton(3, qmc.WithSkip(64), qmc.WithScrambling(7))
	if err != nil {
		t.Fatal(err)
	}

	pts := qmc.Draw(g, 128)

	star, err := qmc.StarDiscrepancy(pts)
	if err != nil {
		t.Fatal(err)
	}

	if star != goldenStar {
		t.Fatalf("D* = %v, want exactly %v", star, goldenStar)
	}

	cd2, err := qmc.CenteredL2Discrepancy(pts)
	if err != nil {
		t.Fatal(err)
	}

	if rel := math.Abs(cd2-goldenCD2) / goldenCD2; rel > 1e-12 {
		t.Fatalf("CD2 = %v, want %v (%.2e relative)", cd2, goldenCD2, rel)
	}
}

// ------------------------------------------------------------ the properties

// randomPoints fills an n-by-s matrix from a seeded pseudorandom source. The
// seed is fixed so that a failure is reproducible; the streams are consecutive
// draws from one source rather than separately seeded generators, for the
// reason integration_test.go's mcRMSError gives.
func randomPoints(rng *rand.Rand, n, s int) [][]float64 {
	pts := make([][]float64, n)
	for i := range pts {
		pts[i] = make([]float64, s)
		for k := range pts[i] {
			pts[i][k] = rng.Float64()
		}
	}

	return pts
}

// TestQMCBeatsPseudorandomOnStarDiscrepancy is the positive control for the
// star walk: the statistic has to see the property the package exists for.
//
// The asserted factor is 1.5, far below the measured 3.9x, for the same reason
// integration_test.go asserts 5x against a measured 16x — an unlucky
// scrambling seed must not turn a working package red.
//
// Three seeds rather than ten: this shape is 2.3e7 leaves per call, which is
// most of a second each and roughly seventeen times that under -race, and it
// is by a wide margin the most expensive test in the package. The measured
// spread across seeds is a few percent against an asserted margin of more than
// twofold, so the extra seeds would buy nothing but wall clock.
func TestQMCBeatsPseudorandomOnStarDiscrepancy(t *testing.T) {
	const (
		dims      = 3
		n         = 512
		streams   = 3
		wantRatio = 1.5
	)

	rng := rand.New(rand.NewSource(20240825)) //nolint:gosec // statistical baseline, not cryptography

	qmcSum, mcSum := 0.0, 0.0

	for seed := 1; seed <= streams; seed++ {
		g, err := qmc.NewHalton(dims, qmc.WithSkip(64), qmc.WithScrambling(uint64(seed)))
		if err != nil {
			t.Fatal(err)
		}

		q, err := qmc.StarDiscrepancy(qmc.Draw(g, n))
		if err != nil {
			t.Fatal(err)
		}

		m, err := qmc.StarDiscrepancy(randomPoints(rng, n, dims))
		if err != nil {
			t.Fatal(err)
		}

		qmcSum += q
		mcSum += m
	}

	qmcMean, mcMean := qmcSum/streams, mcSum/streams

	ratio := mcMean / qmcMean
	if ratio < wantRatio {
		t.Fatalf("at %d dims with n=%d over %d streams: scrambled Halton D* = %.5f, random D* = %.5f "+
			"(%.2fx), want >= %.1fx; the statistic no longer sees the low-discrepancy structure",
			dims, n, streams, qmcMean, mcMean, ratio, wantRatio)
	}

	t.Logf("d=%d n=%d streams=%d: D* scrambled Halton %.5f vs random %.5f (%.2fx better)",
		dims, n, streams, qmcMean, mcMean, ratio)
}

// TestCenteredL2SeparatesQMCFromRandomAtLowDimensions is the positive control
// for CD2, and the necessary companion to the saturation test below: it is
// what makes "the statistic stops working at 39 dimensions" a statement about
// the dimension count rather than about the implementation.
func TestCenteredL2SeparatesQMCFromRandomAtLowDimensions(t *testing.T) {
	const (
		n         = 1024
		streams   = 5
		wantRatio = 2.0
	)

	rng := rand.New(rand.NewSource(20240826)) //nolint:gosec // statistical baseline, not cryptography

	for _, dims := range []int{2, 5} {
		qmcSum, mcSum := 0.0, 0.0

		for seed := 1; seed <= streams; seed++ {
			g, err := qmc.NewHalton(dims, qmc.WithSkip(64), qmc.WithScrambling(uint64(seed)))
			if err != nil {
				t.Fatal(err)
			}

			q, err := qmc.CenteredL2Discrepancy(qmc.Draw(g, n))
			if err != nil {
				t.Fatal(err)
			}

			m, err := qmc.CenteredL2Discrepancy(randomPoints(rng, n, dims))
			if err != nil {
				t.Fatal(err)
			}

			qmcSum += q
			mcSum += m
		}

		qmcMean, mcMean := qmcSum/streams, mcSum/streams

		ratio := mcMean / qmcMean
		if ratio < wantRatio {
			t.Fatalf("at %d dims with n=%d: CD2 scrambled Halton %.6f vs random %.6f (%.2fx), want >= %.1fx",
				dims, n, qmcMean, mcMean, ratio, wantRatio)
		}

		t.Logf("d=%d n=%d: CD2 scrambled Halton %.6f vs random %.6f (%.2fx better)",
			dims, n, qmcMean, mcMean, ratio)
	}
}

// TestCenteredL2SaturatesAtThirtyNineDimensions is the negative control, and
// it is the reason CenteredL2Discrepancy's doc comment carries a caveat rather
// than a recommendation.
//
// Three things are asserted together, and they only mean something as a set:
// the random baseline reproduces the analytic expectation
// sqrt(((5/4)^s - (13/12)^s)/N), which pins that the returned quantity really
// is the square root of Hickernell's statistic; the QMC and random values sit
// within 10% of each other, which is the blindness; and over the very same
// point sets the integration error differs by at least 5x, which proves the
// point sets are not in fact equivalent and that it is the statistic, not the
// generator, that has stopped discriminating.
//
// If the two CD2 values ever do separate here, the documentation's caveat is
// wrong and must be rewritten. Do not relax this test to make it pass.
func TestCenteredL2SaturatesAtThirtyNineDimensions(t *testing.T) {
	const (
		dims    = 39
		n       = 1024
		streams = 10
	)

	rng := rand.New(rand.NewSource(20240827)) //nolint:gosec // statistical baseline, not cryptography

	qmcCD2, mcCD2 := 0.0, 0.0
	qmcSqErr, mcSqErr := 0.0, 0.0

	for seed := 1; seed <= streams; seed++ {
		g, err := qmc.NewHalton(dims, qmc.WithSkip(64), qmc.WithScrambling(uint64(seed)))
		if err != nil {
			t.Fatal(err)
		}

		qPts := qmc.Draw(g, n)
		mPts := randomPoints(rng, n, dims)

		q, err := qmc.CenteredL2Discrepancy(qPts)
		if err != nil {
			t.Fatal(err)
		}

		m, err := qmc.CenteredL2Discrepancy(mPts)
		if err != nil {
			t.Fatal(err)
		}

		qmcCD2 += q
		mcCD2 += m

		qe := meanProductIntegrand(qPts) - 1
		me := meanProductIntegrand(mPts) - 1
		qmcSqErr += qe * qe
		mcSqErr += me * me
	}

	qmcMean, mcMean := qmcCD2/streams, mcCD2/streams
	analytic := math.Sqrt((math.Pow(1.25, dims) - math.Pow(13.0/12.0, dims)) / n)

	if rel := math.Abs(mcMean-analytic) / analytic; rel > 0.02 {
		t.Fatalf("random CD2 = %.5f but sqrt(((5/4)^%d - (13/12)^%d)/%d) = %.5f (%.2f%% off); "+
			"either the formula or the decision to return the square root is wrong",
			mcMean, dims, dims, n, analytic, 100*rel)
	}

	if gap := math.Abs(qmcMean-mcMean) / mcMean; gap >= 0.10 {
		t.Fatalf("CD2 separates scrambled Halton (%.5f) from random (%.5f) by %.1f%% at %d dims; "+
			"the saturation caveat in CenteredL2Discrepancy's doc comment is no longer true and must be rewritten",
			qmcMean, mcMean, 100*gap, dims)
	}

	qmcRMS := math.Sqrt(qmcSqErr / streams)
	mcRMS := math.Sqrt(mcSqErr / streams)

	if ratio := mcRMS / qmcRMS; ratio < 5 {
		t.Fatalf("over the same point sets the integration error ratio is only %.1fx; "+
			"the two point sets are no longer distinguishable at all, so this test proves nothing about CD2",
			ratio)
	}

	t.Logf("d=%d n=%d streams=%d: CD2 scrambled Halton %.5f, random %.5f (%.2f%% apart), "+
		"analytic random expectation %.5f, integration RMS error %.3e vs %.3e (%.1fx)",
		dims, n, streams, qmcMean, mcMean, 100*math.Abs(qmcMean-mcMean)/mcMean,
		analytic, qmcRMS, mcRMS, mcRMS/qmcRMS)
}

// meanProductIntegrand averages integration_test.go's smooth product
// integrand, whose exact integral over the cube is 1, over a point set.
func meanProductIntegrand(pts [][]float64) float64 {
	sum := 0.0
	for _, p := range pts {
		sum += productIntegrand(p)
	}

	return sum / float64(len(pts))
}

// TestCenteredL2MatchesTheRandomExpectation pins the closed form itself across
// several sizes: E[CD2^2] = ((5/4)^s - (13/12)^s)/N for N i.i.d. uniform
// points. It is a golden value with no golden constant in it.
func TestCenteredL2MatchesTheRandomExpectation(t *testing.T) {
	rng := rand.New(rand.NewSource(20240828)) //nolint:gosec // statistical baseline, not cryptography

	for _, c := range []struct{ s, n, reps int }{{1, 256, 60}, {3, 256, 60}, {8, 512, 40}, {20, 512, 30}} {
		sum := 0.0

		for r := 0; r < c.reps; r++ {
			cd2, err := qmc.CenteredL2Discrepancy(randomPoints(rng, c.n, c.s))
			if err != nil {
				t.Fatal(err)
			}

			sum += cd2 * cd2
		}

		got := sum / float64(c.reps)
		want := (math.Pow(1.25, float64(c.s)) - math.Pow(13.0/12.0, float64(c.s))) / float64(c.n)

		if rel := math.Abs(got-want) / want; rel > 0.10 {
			t.Fatalf("s=%d N=%d over %d draws: mean CD2^2 = %.6g, closed form says %.6g (%.1f%% off)",
				c.s, c.n, c.reps, got, want, 100*rel)
		}

		t.Logf("s=%2d N=%d: mean CD2^2 = %.6g, ((5/4)^s-(13/12)^s)/N = %.6g", c.s, c.n, got, want)
	}
}

// ------------------------------------------------------------- the refusals

// TestStarDiscrepancyRefusesWhatItCannotAfford pins that both gates fire, and
// that neither of them takes any time to do it.
//
// Two gates are needed, not one. Seven dimensions is over the dimension
// ceiling regardless of size; five dimensions with 3000 points is under any
// plausible dimension ceiling and is still 2e15 leaves. A ceiling on s alone
// would let the second one through and hang the caller.
func TestStarDiscrepancyRefusesWhatItCannotAfford(t *testing.T) {
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // test fixture, not cryptography

	for _, c := range []struct {
		name string
		s, n int
	}{
		{"over the dimension ceiling", 7, 20},
		{"under the dimension ceiling but past the work budget", 5, 3000},
		{"far over both", 39, 1024},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := qmc.StarDiscrepancy(randomPoints(rng, c.n, c.s))
			if err == nil {
				t.Fatalf("%d dims with %d points was accepted; it must be refused", c.s, c.n)
			}

			if got != 0 {
				t.Fatalf("a refusal returned %v; it must return 0 rather than a partial answer", got)
			}

			for _, want := range []string{"CenteredL2Discrepancy", "boxes", "saturation"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("refusal does not mention %q: %v", want, err)
				}
			}

			t.Logf("%d dims, %d points: %v", c.s, c.n, err)
		})
	}
}

// TestDiscrepancyRefusesMalformedInput walks the whole validation table
// against both functions. Every case is a caller mistake about the data, so
// every case is an error and none of them is a panic — the split
// robustness_test.go's short-dst rule sets out.
//
// The messages are asserted on, not just the error's presence. A validation
// error whose text does not name the offending point and dimension leaves the
// caller with a haystack, which for a several-thousand-point matrix is no
// better than no message at all.
func TestDiscrepancyRefusesMalformedInput(t *testing.T) {
	for _, c := range []struct {
		name   string
		points [][]float64
		want   []string
	}{
		{
			name:   "empty set",
			points: [][]float64{},
			want:   []string{"empty", "undefined, not 0"},
		},
		{
			name:   "zero-width point",
			points: [][]float64{{}},
			want:   []string{"point 0", "no coordinates"},
		},
		{
			name:   "zero-width point after a good one",
			points: [][]float64{{0.5, 0.5}, {}},
			want:   []string{"point 1", "no coordinates"},
		},
		{
			name:   "ragged row",
			points: [][]float64{{0.5, 0.5}, {0.5, 0.5}, {0.5}},
			want:   []string{"point 2", "1 coordinates", "point 0 has 2"},
		},
		{
			name:   "coordinate below zero",
			points: [][]float64{{0.5, -0.25}},
			want:   []string{"point 0", "dimension 1", "-0.25", "scale your samples back"},
		},
		{
			name:   "coordinate above one",
			points: [][]float64{{0.5, 0.5}, {1.5, 0.5}},
			want:   []string{"point 1", "dimension 0", "1.5", "scale your samples back"},
		},
		{
			name:   "NaN",
			points: [][]float64{{0.5}, {math.NaN()}},
			want:   []string{"point 1", "dimension 0", "NaN", "silently dropped"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for name, fn := range map[string]func([][]float64) (float64, error){
				"StarDiscrepancy":       qmc.StarDiscrepancy,
				"CenteredL2Discrepancy": qmc.CenteredL2Discrepancy,
			} {
				got, err := fn(c.points)
				if err == nil {
					t.Fatalf("%s accepted %v", name, c.points)
				}

				if got != 0 {
					t.Fatalf("%s returned %v alongside an error", name, got)
				}

				for _, want := range append([]string{name}, c.want...) {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("%s error does not mention %q: %v", name, want, err)
					}
				}
			}
		})
	}
}

// TestCoordinateOfExactlyOneIsAccepted pins the boundary case the half-open
// box convention handles without a special case. A point at the far corner is
// never strictly below 1, so it lies outside every box the definition takes a
// supremum over, and the answer is 1 rather than an error or a 0.
func TestCoordinateOfExactlyOneIsAccepted(t *testing.T) {
	for _, c := range []struct {
		name   string
		points [][]float64
		want   float64
	}{
		{"one point at the far corner in 1-D", [][]float64{{1}}, 1},
		{"one point at the far corner in 3-D", [][]float64{{1, 1, 1}}, 1},
		{"a boundary point among interior ones", [][]float64{{0.25}, {0.5}, {0.75}, {1}}, 0.25},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := qmc.StarDiscrepancy(c.points)
			if err != nil {
				t.Fatal(err)
			}

			if got != c.want {
				t.Fatalf("D* = %v, want exactly %v (brute force: %v)", got, c.want, starBruteForce(c.points))
			}
		})
	}
}

// ------------------------------------------------------------------- Draw

// TestDrawIsIndexAddressedAndContiguous pins the two properties
// CenteredL2Discrepancy depends on: the matrix is At-addressed, so it does not
// move the caller's cursor and does not depend on it, and the rows are
// consecutive windows into one array.
func TestDrawIsIndexAddressedAndContiguous(t *testing.T) {
	g, err := qmc.NewHalton(4, qmc.WithSkip(64))
	if err != nil {
		t.Fatal(err)
	}

	g.Next()
	g.Next()

	pts := qmc.Draw(g, 16)

	for i, p := range pts {
		want := g.At(i)
		for k := range want {
			if p[k] != want[k] {
				t.Fatalf("row %d dim %d = %v, At(%d) says %v", i, k, p[k], i, want[k])
			}
		}
	}

	for i := 0; i+1 < len(pts); i++ {
		if &pts[i][:cap(pts[i])][0] != &pts[i][0] {
			t.Fatal("row header does not start where its window does")
		}

		if cap(pts[i]) != len(pts[i]) {
			t.Fatalf("row %d has cap %d and len %d; an append would reach into the next row",
				i, cap(pts[i]), len(pts[i]))
		}
	}

	if got := qmc.Draw(g, 0); got == nil || len(got) != 0 {
		t.Fatalf("Draw(g, 0) = %v, want a non-nil empty slice", got)
	}

	if got := qmc.Draw(g, -5); got == nil || len(got) != 0 {
		t.Fatalf("Draw(g, -5) = %v, want a non-nil empty slice", got)
	}
}
