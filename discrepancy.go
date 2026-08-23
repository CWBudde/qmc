package qmc

import (
	"fmt"
	"math"
)

// Discrepancy: how far a point set is from filling the cube evenly.
//
// The identifiers here are the mathematics' own names — s for the dimension
// count, k for a dimension index, b for a box corner, u for |x-1/2|, N for the
// point count. That is the same argument .golangci.yml makes for leaving
// varnamelen off: these formulas are transcribed from Hickernell (1998) and
// from the standard sup-over-boxes definition, and renaming s to
// dimensionCount would move the code away from the source it is checked
// against, not closer to it.

// maxStarDims is the dimension ceiling on the exact star discrepancy.
//
// Computing the exact star discrepancy is NP-hard in the dimension (Gnewuch,
// Srivastav & Winker, "Finding optimal volume subintervals with k points and
// calculating the star discrepancy are NP-hard problems", Journal of
// Complexity 25(2), 2009). The ceiling is therefore a statement about the
// problem, not about this implementation: no amount of tuning moves it far.
const maxStarDims = 6

// starBoxBudget caps the number of boxes the pruned enumeration is allowed to
// visit, measured in leaves of the search tree.
//
// A dimension ceiling on its own is not a gate. Five dimensions and 3000
// points is well inside maxStarDims and is 2.0e15 leaves — a hang, not an
// answer. The budget is what makes the refusal see N as well as s.
//
// The number is measured, not asserted. BenchmarkStarDiscrepancy walks the two
// shapes that bracket the tree: 1024 points in 2 dimensions is wide and
// shallow at 5.26e5 leaves and takes 14.6 ms, and 160 points in 4 dimensions is
// narrow and deep at 2.91e7 leaves and takes 764 ms. Those are 27.7 and 26.3
// nanoseconds per leaf — the cost per leaf is flat across shapes, which is what
// makes a leaf count a usable proxy for wall clock at all. 3e7 leaves is
// therefore about 0.8 seconds on the machine this was measured on (a 12th-gen
// mobile i7): a wait, not a hang, which is the line this constant is drawing.
//
// Raising it is a decision about how long a caller should be made to wait, not
// a way to reach a larger problem: the cost is N^s/s!, so ten times the budget
// buys about 3.2 times the points in two dimensions and 1.5 times in six.
const starBoxBudget = 3e7

// StarDiscrepancy returns the exact star discrepancy D*_N of a point set in
// the unit cube.
//
// For P = {x_1..x_N} in [0,1]^s,
//
//	D*_N(P) = sup over b in (0,1]^s of | vol([0,b)) - A(b)/N |
//
// where vol(b) is the product of the b_k and A(b) counts the points strictly
// inside the half-open box [0,b). It is the worst relative error any
// origin-anchored box makes about how much of the cube it covers, so it is
// the quantity the Koksma-Hlawka bound multiplies by an integrand's variation
// — the number the phrase "low-discrepancy sequence" refers to.
//
// The supremum is not attained at a single grid. Raising b_k past a
// coordinate makes vol grow smoothly while A jumps only after the coordinate
// is passed, so the overshoot vol - A/N peaks with b sitting exactly on
// coordinate values and counted strictly; the undershoot A/N - vol peaks just
// above a coordinate, whose closure is the same corner counted inclusively.
// Both halves are therefore enumerated over the same grid — each dimension's
// coordinate values together with 1 — and only the count differs:
//
//	dPlus  = max over b of ( vol(b) - #{i : x_ik <  b_k for all k}/N )
//	dMinus = max over b of ( #{i : x_ik <= b_k for all k}/N - vol(b) )
//	D*     = max(dPlus, dMinus)
//
// Taking only one of the two halves returns a lower bound that happens to be
// right in exactly the small hand-checkable cases, which is why
// discrepancy_test.go checks this against a brute-force enumeration rather
// than against arithmetic anyone can do on paper.
//
// Closed forms worth knowing, all of which the tests pin. In one dimension
// with the points sorted, D* = 1/(2N) + max_i |x_(i) - (2i-1)/(2N)|; the
// midpoints (2i-1)/(2N) hit its floor of exactly 1/(2N) and the left
// endpoints i/N score exactly 1/N. For a single point x in s dimensions,
// D* = max(max_k x_k, 1 - prod_k x_k).
//
// Duplicate points and repeated coordinates need no special handling: the two
// counts differ by exactly the multiplicity at a corner, which is what makes
// the pair of them exact. A coordinate of exactly 1.0 is accepted and comes
// out right without a special case — it is never strictly below 1, so it sits
// outside every half-open box and shows up as dPlus = 1 - A/N > 0, which is
// the correct answer for a point set that has parked mass on the boundary.
//
// Cost, and the refusal. Restricting each dimension's candidates to the
// surviving points' own coordinates plus 1 is exact and turns the naive
// (N+1)^s grid into C(N+s,s) leaves, but C(N+s,s) is still about N^s/s!.
// StarDiscrepancy therefore refuses above maxStarDims dimensions or above
// starBoxBudget leaves, and returns (0, error) rather than a partial answer.
// At the current budget the affordable point counts are 7744 at 2 dimensions,
// 562 at 3, 161 at 4, 78 at 5 and 49 at 6. Above that, use
// CenteredL2Discrepancy — but read its saturation caveat before believing the
// number it returns.
//
// The walk is products and differences with no multiply-add shape, so the
// result is bit-identical across GOARCH and the tests assert that with ==.
//
// Reference: Gnewuch, M., Srivastav, A. & Winker, P. (2009), "Finding optimal
// volume subintervals with k points and calculating the star discrepancy are
// NP-hard problems", Journal of Complexity 25(2), 115-127.
func StarDiscrepancy(points [][]float64) (float64, error) {
	n, s, err := validatePoints(points, "StarDiscrepancy")
	if err != nil {
		return 0, err
	}

	boxes := starLeafCount(n, s)
	if s > maxStarDims || boxes > starBoxBudget {
		return 0, errStarTooBig(n, s, boxes)
	}

	w := newStarWalk(points, n, s)
	w.recurse(0, w.clo, w.str, 1.0)

	return math.Max(w.dPlus, w.dMinus), nil
}

// CenteredL2Discrepancy returns the centred L2 discrepancy CD2 of a point set
// in the unit cube.
//
// Where StarDiscrepancy takes a supremum over boxes and pays NP-hard prices
// for it, this takes an L2 average over a family of boxes and has a closed
// form that costs O(N^2 s). Two details of that family are what make the
// average closed, and both are easy to get wrong:
//
//	CD2^2 = sum over non-empty u subset of {1..s} of
//	          integral over [0,1]^|u| of
//	            ( #{i : x_i restricted to u lies in J_t}/N - vol(J_t) )^2 dt
//
// First, the boxes are anchored at the *nearest corner* of the cube, not at
// its centre: in each coordinate J_t uses [0,t_k) when t_k < 1/2 and [t_k,1)
// otherwise. That is what buys the statistic its defining symmetry —
// reflecting any coordinate about 1/2 leaves CD2 unchanged — and
// centre-anchored boxes, the natural-sounding reading of the name, do not
// reproduce the closed form.
//
// Second, the outer sum runs over every non-empty subset of the coordinates,
// so CD2 is the accumulated discrepancy of all 2^s - 1 projections and not
// only of the full-dimensional one. That sum is where (13/12)^s comes from —
// it is prod_k (1 + 1/12) expanded — and dropping it leaves a quantity that
// still looks plausible and is out by a factor of 20 in two dimensions.
// discrepancy_test.go integrates the definition above numerically because it
// is the only way either mistake would be caught.
//
// With u_ik = |x_ik - 1/2| the integral evaluates to
//
//	CD2^2 = (13/12)^s
//	      - (2/N)   sum_i     prod_k ( 1 + u_ik/2 - u_ik^2/2 )
//	      + (1/N^2) sum_i,j   prod_k ( 1 + u_ik/2 + u_jk/2 - |x_ik-x_jk|/2 )
//
// and this returns CD2, the square root: it is a norm, the literature
// tabulates the root, and it shares units with StarDiscrepancy. A very well
// distributed small set can push the square a few ulps below zero through
// cancellation, so it is clamped at 0 before the square root.
//
// Sanity values, both pinned by the tests: one point at the centre of the cube
// gives sqrt((13/12)^s - 1), and one point at the origin in one dimension
// gives sqrt(1/3).
//
// # The saturation, which is the whole reason to read this comment
//
// For N independent uniform points the expectation is exactly
//
//	E[CD2^2] = ( (5/4)^s - (13/12)^s ) / N
//
// because each one-dimensional integral collapses: the single sum's factor
// integrates to 13/12, so does the double sum's off-diagonal factor, and the
// double sum's diagonal factor 1 + u integrates to 5/4. At 39 dimensions and
// 1024 points that is (6018.5 - 22.7)/1024 = 5.855, so E[CD2] = 2.4198 — and
// measured over ten scrambling seeds, random comes in at 2.4046 and scrambled
// Halton at 2.3657, a difference of 1.6%.
//
// The mechanism is visible in the same arithmetic. The diagonal i=j terms
// alone contribute (5/4)^s/N = 5.877 of that 5.855 total — 100.4% of it.
// Everything the statistic was supposed to measure lives in a residual of
// -0.02, and the diagonal depends only on each coordinate's marginal spread,
// not at all on how the points sit relative to one another. Over the very same
// point sets the RMS integration error of a smooth product integrand differs
// by 16x.
//
// Measured at N=1024 over ten seeds, the random-to-Halton ratio decays with
// the dimension count and takes the diagonal's share of the expectation with
// it:
//
//	 s     CD2 Halton   CD2 random   ratio   diagonal share
//	 2       0.00137      0.01705    12.4x        402%
//	 5       0.00638      0.04020     6.3x        196%
//	10       0.03374      0.08057     2.4x        131%
//	15       0.09751      0.15911     1.6x        113%
//	20       0.22000      0.28085     1.3x        106%
//	30       0.81877      0.88577     1.08x       101%
//	39       2.36573      2.40461     1.02x       100.4%
//
// Read that as: informative below roughly ten dimensions, weak by twenty, and
// dead by thirty. It is not a cliff, so there is no honest dimension at which
// to refuse — which is why this returns a number and documents the caveat
// where StarDiscrepancy returns an error.
//
// The self-check to run before believing a CD2 number: compare it against
// sqrt(((5/4)^s - (13/12)^s)/N). If your point set is not several times below
// that, the statistic is telling you about your marginals and nothing else,
// and you want an integration test or StarDiscrepancy in a projection instead.
//
// # Precision
//
// The three terms are near-equal and cancel, losing roughly
// log10(N*(13/15)^s) digits. That is worst at *low* s with large N, not high:
// s=1 with N=1e6 loses about 6 of 16 digits, while s=39 with N=1024 loses
// 0.6. There is therefore no high-dimensional precision limit to document, and
// at every size this package can compute in reasonable time at least 9
// significant digits survive.
//
// Unlike StarDiscrepancy this has genuine multiply-add shapes in its inner
// product, so a Go compiler may fuse them on arm64. The result is reproducible
// to within a few ulps across architectures, not bit-identical.
//
// Reference: Hickernell, F.J. (1998), "A generalized discrepancy and
// quadrature error bound", Mathematics of Computation 67(221), 299-322,
// equation 5.3.
func CenteredL2Discrepancy(points [][]float64) (float64, error) {
	n, s, err := validatePoints(points, "CenteredL2Discrepancy")
	if err != nil {
		return 0, err
	}

	nf := float64(n)

	// u_ik = |x_ik - 1/2| and the single sum's per-point product, both in one
	// O(Ns) pass. u is flat and row-aliased for the same cache reason Draw
	// gives: the double sum below reads it N^2/2 times.
	u := make([]float64, n*s)
	single := 0.0

	for i, p := range points {
		row := u[i*s : (i+1)*s : (i+1)*s]
		prod := 1.0

		for k, x := range p {
			uk := math.Abs(x - 0.5)
			row[k] = uk
			prod *= 1 + uk/2 - uk*uk/2
		}

		single += prod
	}

	// The double sum is symmetric and its diagonal is exact in closed form:
	// b(t,t) = 1 + u, so sum_i sum_j = 2*sum_{i<j} + sum_i prod_k (1 + u_ik).
	// Halving it is not an optimisation of the formula, it is the formula.
	diagonal := 0.0

	for i := 0; i < n; i++ {
		prod := 1.0
		for _, uk := range u[i*s : (i+1)*s] {
			prod *= 1 + uk
		}

		diagonal += prod
	}

	// Two-level accumulation: an inner float64 per i folded into the outer
	// total. Given this loop shape it costs nothing and turns O(N^2)*eps
	// rounding growth into O(N)*eps.
	upper := 0.0

	for i := 0; i < n; i++ {
		ui, pi := u[i*s:(i+1)*s], points[i]
		inner := 0.0

		for j := i + 1; j < n; j++ {
			uj, pj := u[j*s:(j+1)*s], points[j]
			prod := 1.0

			for k := 0; k < s; k++ {
				prod *= 1 + ui[k]/2 + uj[k]/2 - math.Abs(pi[k]-pj[k])/2
			}

			inner += prod
		}

		upper += inner
	}

	square := math.Pow(13.0/12.0, float64(s)) - 2*single/nf + (2*upper+diagonal)/(nf*nf)
	if square < 0 {
		square = 0
	}

	return math.Sqrt(square), nil
}

// validatePoints checks the one contract both statistics share: a non-empty,
// rectangular matrix of coordinates in [0,1].
//
// Every case here is a caller mistake about the data, so every case is an
// error. The package reserves panics for contracts that have no error channel
// — AtInto's short dst — and these two functions have one.
func validatePoints(points [][]float64, fn string) (int, int, error) {
	if len(points) == 0 {
		return 0, 0, fmt.Errorf(
			"qmc: %s: the point set is empty; the discrepancy of the empty set is undefined, not 0", fn,
		)
	}

	s := len(points[0])

	for i, p := range points {
		if len(p) == 0 {
			return 0, 0, fmt.Errorf(
				"qmc: %s: point %d has no coordinates; a zero-width point set has no cube to be discrepant in", fn, i,
			)
		}

		if len(p) != s {
			return 0, 0, fmt.Errorf(
				"qmc: %s: point %d has %d coordinates but point 0 has %d; the point set must be rectangular",
				fn, i, len(p), s,
			)
		}

		for k, x := range p {
			// Written as the negation of the good case on purpose. NaN
			// compares false against everything, so `x < 0 || x > 1` lets it
			// straight through; this form rejects it and the branch below then
			// says which mistake it was.
			if !(x >= 0 && x <= 1) {
				if math.IsNaN(x) {
					return 0, 0, fmt.Errorf(
						"qmc: %s: point %d dimension %d is NaN; every comparison against NaN is false, "+
							"so the point would be silently dropped from every box and the answer would still look plausible",
						fn, i, k,
					)
				}

				return 0, 0, fmt.Errorf(
					"qmc: %s: point %d dimension %d is %g; coordinates must lie in [0,1], "+
						"so scale your samples back into the unit cube before measuring",
					fn, i, k, x,
				)
			}
		}
	}

	return len(points), s, nil
}

// starLeafCount returns C(N+s,s), the number of leaves the pruned enumeration
// would visit with no pruning at all: one per choice of a box corner from each
// dimension's candidate grid, counted without regard to order.
//
// It is computed as a product of ratios rather than as factorials so that it
// stays in range: the answer overflows long before any intermediate does, and
// when the answer itself overflows the caller is being refused anyway.
func starLeafCount(n, s int) float64 {
	boxes := 1.0
	for k := 1; k <= s; k++ {
		boxes *= float64(n+k) / float64(k)
	}

	return boxes
}

// errStarTooBig builds both of StarDiscrepancy's refusals, so the dimension
// ceiling and the work budget say the same thing rather than drifting apart —
// the same reason errLeapConflict exists.
//
// It names the cost, the budget, that the limit is a property of the problem
// and not a tuning knob, what is actually affordable per dimension, and the
// alternative *with* its caveat attached. Pointing at CenteredL2Discrepancy
// bare would trade a refusal for a number that means nothing at the dimension
// counts that trigger this message.
func errStarTooBig(n, s int, boxes float64) error {
	// The two gates fail for different reasons and must not claim each other's.
	// Seven dimensions and twenty points is only 8.9e5 leaves — well inside the
	// budget — and telling that caller it is "past the budget" would send them
	// looking for a smaller point count that does not exist.
	reason := fmt.Sprintf("%d dimensions is past the ceiling of %d", s, maxStarDims)
	if boxes > starBoxBudget {
		reason = fmt.Sprintf("that is past this package's budget of %.0e", float64(starBoxBudget))
	}

	return fmt.Errorf(
		"qmc: star discrepancy over %d points in %d dimensions would enumerate %s boxes and %s; "+
			"exact star discrepancy is NP-hard in the dimension, so this is a ceiling on what is "+
			"computable and not a tuning knob. "+
			"Affordable point counts are 7744 at 2 dimensions, 562 at 3, 161 at 4, 78 at 5 and 49 at 6. "+
			"Use CenteredL2Discrepancy above that, but read its saturation caveat first: "+
			"by twenty dimensions it scores a low-discrepancy set within 30%% of a random one and by "+
			"thirty within 8%%",
		n, s, formatBoxes(boxes), reason,
	)
}

// formatBoxes renders a leaf count that may have overflowed float64. At a few
// hundred dimensions the product genuinely is past 1e308, and "+Inf boxes"
// reads like a bug in the message rather than a fact about the problem.
func formatBoxes(boxes float64) string {
	if math.IsInf(boxes, 1) {
		return "more than 1e308"
	}

	return fmt.Sprintf("~%.1e", boxes)
}

// starCandidate is one choice of b_k, together with how many of the parent's
// two survivor lists pass through it.
type starCandidate struct {
	v  float64
	nc int32 // survivors with x_ik <= v, out of the closed list
	ns int32 // survivors with x_ik <  v, out of the strict list
}

// starWalk carries the depth-first enumeration's state.
//
// clo and str are the two survivor lists, and every node works on a *prefix*
// of its parent's arrays rather than on a copy — which is what keeps the walk
// allocation-free once it has started.
type starWalk struct {
	points [][]float64
	n      float64
	dims   int

	clo []int32
	str []int32

	// One candidate table per depth. The table has to be materialised in full
	// before the first child recurses: a child sorts its prefix of clo by the
	// next dimension and thereby permutes the parent's array, so a parent that
	// interleaved its sweep with its recursion would read a table that had
	// moved underneath it. That is the bug this field exists to make
	// impossible.
	cand [][]starCandidate

	dPlus  float64
	dMinus float64
	best   float64
}

func newStarWalk(points [][]float64, n, s int) *starWalk {
	w := &starWalk{
		points: points,
		n:      float64(n),
		dims:   s,
		clo:    make([]int32, n),
		str:    make([]int32, n),
		cand:   make([][]starCandidate, s),
	}

	for i := range w.clo {
		w.clo[i] = int32(i)
		w.str[i] = int32(i)
	}

	for k := range w.cand {
		w.cand[k] = make([]starCandidate, 0, n+1)
	}

	return w
}

func (w *starWalk) recurse(k int, clo, str []int32, vol float64) {
	if k == w.dims {
		if plus := vol - float64(len(str))/w.n; plus > w.dPlus {
			w.dPlus = plus
		}

		if minus := float64(len(clo))/w.n - vol; minus > w.dMinus {
			w.dMinus = minus
		}

		w.best = math.Max(w.dPlus, w.dMinus)

		return
	}

	// Admissible prune. Below this node the volume can only shrink and the
	// closed count can only shrink, so dPlus <= vol and dMinus <= |clo|/N for
	// every descendant leaf. best is updated at leaves, so depth-first
	// siblings inherit the cutoff from the subtrees already finished.
	if vol <= w.best && float64(len(clo))/w.n <= w.best {
		return
	}

	sortByCoord(clo, w.points, k)
	sortByCoord(str, w.points, k)

	table := w.candidates(k, clo, str)

	for _, c := range table {
		w.recurse(k+1, clo[:c.nc], str[:c.ns], vol*c.v)
	}
}

// candidates fills the depth-k table with the distinct coordinates of the
// closed survivor list in dimension k, ascending, followed by 1.
//
// Restricting candidates to the survivors' own coordinates is exact, not a
// heuristic. If v is not one of them then the next larger such value v' has
// the same strict count and more volume, so v' dominates v for dPlus;
// symmetrically the next smaller one dominates for dMinus. That is the step
// that turns (N+1)^s boxes into C(N+s,s).
//
// Both lists arrive sorted by dimension k, so the two counts are found by a
// single merge sweep. Duplicate coordinates are collapsed because a repeated
// candidate generates a byte-identical subtree.
func (w *starWalk) candidates(k int, clo, str []int32) []starCandidate {
	out := w.cand[k][:0]
	ns := 0

	for i := 0; i < len(clo); {
		v := w.points[clo[i]][k]

		j := i
		for j < len(clo) && w.points[clo[j]][k] == v {
			j++
		}

		for ns < len(str) && w.points[str[ns]][k] < v {
			ns++
		}

		out = append(out, starCandidate{v: v, nc: int32(j), ns: int32(ns)})
		i = j
	}

	// b_k = 1 closes the box at the far face of the cube. Every coordinate is
	// <= 1 so the closed count is the whole list; a coordinate of exactly 1 is
	// still not < 1, which is how a point parked on the boundary stays outside
	// the open box. Skipped when some coordinate already is 1, since that
	// candidate is this one.
	if len(out) == 0 || out[len(out)-1].v != 1 {
		for ns < len(str) && w.points[str[ns]][k] < 1 {
			ns++
		}

		out = append(out, starCandidate{v: 1, nc: int32(len(clo)), ns: int32(ns)})
	}

	w.cand[k] = out

	return out
}

// sortByCoord sorts an index list by one coordinate, in place.
//
// It is hand-rolled rather than sort.Slice because it is the walk's inner
// loop and sort.Slice's closure escapes to the heap on every one of the
// C(N+s,s) nodes. Ordering among equal coordinates is not specified and does
// not need to be: the candidate sweep collapses ties, so any two orderings
// produce the same table and the same result.
func sortByCoord(idx []int32, pts [][]float64, k int) {
	for len(idx) > 12 {
		lo, mid, hi := 0, len(idx)/2, len(idx)-1

		if pts[idx[mid]][k] < pts[idx[lo]][k] {
			idx[mid], idx[lo] = idx[lo], idx[mid]
		}

		if pts[idx[hi]][k] < pts[idx[mid]][k] {
			idx[hi], idx[mid] = idx[mid], idx[hi]

			if pts[idx[mid]][k] < pts[idx[lo]][k] {
				idx[mid], idx[lo] = idx[lo], idx[mid]
			}
		}

		pivot := pts[idx[mid]][k]
		i, j := 0, hi

		for i <= j {
			for pts[idx[i]][k] < pivot {
				i++
			}

			for pts[idx[j]][k] > pivot {
				j--
			}

			if i <= j {
				idx[i], idx[j] = idx[j], idx[i]
				i++
				j--
			}
		}

		// Recurse into the smaller side and loop on the larger one, so the
		// stack stays logarithmic even on an adversarial ordering.
		if j+1 < len(idx)-i {
			sortByCoord(idx[:j+1], pts, k)

			idx = idx[i:]
		} else {
			sortByCoord(idx[i:], pts, k)

			idx = idx[:j+1]
		}
	}

	for a := 1; a < len(idx); a++ {
		v := idx[a]
		key := pts[v][k]
		b := a - 1

		for b >= 0 && pts[idx[b]][k] > key {
			idx[b+1] = idx[b]
			b--
		}

		idx[b+1] = v
	}
}
