package qmc

import (
	"math"
	"math/rand"
	"sort"
	"testing"
)

// What the hash in owenScramble costs, measured rather than argued.
//
// owen.go is careful to say that hash-based Owen scrambling is exact in its
// nesting and approximate in the uniformity of the permutation at each node.
// This file puts a number on the approximation, on the tree directly and then
// on the two instruments the package already trusts. The reference it measures
// against is exactOwen below: a textbook Owen scramble that draws one
// independent fair bit per node of the tree from splitMix64 and stores it,
// which is exactly the thing the hash replaces and exactly the thing that does
// not fit in memory outside a test.
//
// Node by node the hash is indistinguishable from fair coins. Sweeping 40000
// scrambling seeds through every one of the 8191 nodes at depths 0..12, the
// worst node flips with probability 0.00962 away from 1/2 — 3.85 sigma at that
// seed count, where the largest of 8191 independent fair coins is expected to
// land near 3.9 sigma on its own. The worst sibling or parent-child pair
// correlates at |phi| = 0.0198, 3.97 sigma over 12285 pairs. Both are the
// numbers a fair coin gives; the exact reference, measured the same way,
// gives 3.07 sigma. There is no per-node bias and no pairwise dependence to
// find.
//
// Jointly across a level there is, and it is large. For a fixed seed a true
// Owen scramble flips a Binomial(2^k, 1/2) number of the nodes at depth k; the
// reference reproduces that variance to within 4% at every depth measured. The
// hash does not. Over 2000 seeds its flip count per level has variance, as a
// fraction of the binomial value: 1.02 at depth 1, 0.49 at 2, 0.76 at 3, 0.50
// at 4, 0.34 at 5, 0.21 at 6, 0.55 at 7, 0.27 at 8, 0.21 at 9, 0.22 at 10,
// 1.21 at 11, 3.06 at 12, 0.09 at 13, 0.90 at 14, 0.41 at 15, 0.42 at 16. Most
// levels are far more evenly balanced than independent coins would be, depth
// 13 is almost rigidly balanced, and depth 12 is over-dispersed instead. The
// flips at one level are individually fair and pairwise almost independent but
// collectively nothing like independent — which is the honest shape of the
// approximation, and is not visible in any per-node statistic.
//
// What it costs downstream, on the package's own instruments, is very little.
// Integration at 39 dimensions and n=4096 on the integrand of
// integration_test.go, hash against reference, RMS relative error over the
// same stream seeds: 1.06x worse over 10 streams, 1.07x over 40, 1.12x over
// 120. The direction is consistent and the size is at the edge of what stream
// noise explains — the standard error of that ratio at 120 streams is about
// 6%. Against the shared math/rand baseline of integration_test.go the hash
// comes in at 32.0x and the reference at 33.8x over ten streams, 42.1x and
// 47.2x over 120 — the baseline itself moves with the stream count, which is
// why only ratios measured at the same count are comparable. So the price of
// the approximation is on the order of a tenth of the RMS error, on an
// integrand where Owen scrambling as a whole only buys 1.08x over a digital
// shift; it is not something a caller can
// notice. The hash figure is the library's own: 1.350e-04 is what
// TestOwenBeatsDigitalShiftAt39Dims logs for WithOwenScrambling at the same
// settings, so the path built here out of accumulate reproduces the shipped
// one exactly rather than merely resembling it.
//
// The correlation instrument reads the other way and is worth reading
// carefully. Worst adjacent-pair |r| at 39 dimensions and 600 points over
// thirty seeds: hash 0.1116 with a median of 0.0537, reference 0.1104 with a
// median of 0.1043. The reference's median sits at the noise floor of 600
// samples — the largest of 38 sample correlations of independent columns is
// about 0.10 — while the hash's median is half of that, and an unrandomized
// or merely digitally shifted Sobol set scores 0.027 through the same
// harness. The hash is not better here; it is *less randomizing*, leaving the
// point set nearer the deterministic Sobol structure it started from, which
// this statistic rewards.
// A lower number on this measurement does not mean a better scramble, and a
// two-sided gate on it would fail on a correct implementation.
//
// Verdict: measured, and it costs nothing a caller of this package can spend.
// The per-node fairness the theory asks for is there. The joint independence
// across a level is not, by a wide margin, and that is a real difference from
// Owen's construction rather than a rounding error — but it moves integration
// error by at most a tenth and moves the correlation statistic in the
// direction of more structure, not less. Nothing here argues for a fifth
// Laine-Karras round or different constants; the constants are pinned by
// TestOwenPermutationConstantsAreEven and TestOwenIsArchitectureIndependent
// and are not touched. If anyone wants to try, the level-variance table above
// is the instrument to try it on, because it is the only one of these
// measurements with enough resolution to show a change.
//
// The expensive sweeps are behind testing.Short, which no other file in this
// package needed before; go test -short runs the cheap structural half.

// exactOwenTableDepth is how many levels of the binary tree the reference
// implementation stores explicitly.
const exactOwenTableDepth = 16

// exactOwen is a textbook Owen scramble: one independently drawn fair bit per
// node of the binary tree.
type exactOwen struct {
	flips []uint64
	depth uint
	seed  uint64
}

// newExactOwen draws the flip of every node down to depth levels.
func newExactOwen(seed uint64, depth uint) *exactOwen {
	nodes := (uint64(1) << depth) - 1

	e := &exactOwen{
		flips: make([]uint64, (nodes+63)/64),
		depth: depth,
		seed:  seed,
	}

	rng := splitMix64(seed)
	rng.next()
	rng.next()

	for i := uint64(0); i < nodes; i++ {
		if uniformBelow(&rng, 2) == 1 {
			e.flips[i/64] |= 1 << (i % 64)
		}
	}

	return e
}

// flip reports whether the bit at depth k is inverted, given the k input bits
// above it.
func (e *exactOwen) flip(k uint, prefix uint32) uint32 {
	if k < e.depth {
		node := (uint64(1) << k) - 1 + uint64(prefix)

		return uint32(e.flips[node/64]>>(node%64)) & 1
	}

	rng := splitMix64(e.seed ^ (uint64(prefix)|uint64(k)<<32)*0x9E3779B97F4A7C15)
	rng.next()
	rng.next()

	return uint32(rng.next() & 1)
}

// scramble applies the reference scramble to one 32-bit coordinate.
func (e *exactOwen) scramble(x uint32) uint32 {
	var y uint32

	for k := uint(0); k < 32; k++ {
		var (
			prefix = x >> (32 - k)
			bit    = (x >> (31 - k)) & 1
		)

		y |= (bit ^ e.flip(k, prefix)) << (31 - k)
	}

	return y
}

// hashOwenFlip reports the flip Burley's hash makes at depth k under prefix.
func hashOwenFlip(k uint, prefix, seed uint32) uint32 {
	return (owenScramble(prefix<<(32-k), seed) >> (31 - k)) & 1
}

// nodeIndex numbers the nodes of the tree breadth first.
func nodeIndex(k uint, prefix uint32) int {
	return (1 << k) - 1 + int(prefix)
}

// flipStats holds one seed sweep over every node down to maxDepth.
type flipStats struct {
	seeds    int
	maxDepth uint
	count    []int32
	pairs    [][2]int
	joint    []int32
}

// sweepFlips evaluates the flip at every node of depths 0..maxDepth for seeds
// independent seeds, and accumulates marginal and pairwise counts.
//
// The scramble is rebuilt once per seed rather than once per node, which is
// the difference between a test that runs in seconds and one that does not:
// the reference implementation draws a table of node flips up front, and
// drawing it 8191 times per seed instead of once dominates everything.
func sweepFlips(seeds int, maxDepth uint, forSeed func(seedIndex int) func(k uint, prefix uint32) uint32) *flipStats {
	nodes := (1 << (maxDepth + 1)) - 1

	s := &flipStats{
		seeds:    seeds,
		maxDepth: maxDepth,
		count:    make([]int32, nodes),
		pairs:    nodePairs(maxDepth),
	}

	s.joint = make([]int32, len(s.pairs))
	bits := make([]uint32, nodes)

	for si := 0; si < seeds; si++ {
		flip := forSeed(si)

		for k := uint(0); k <= maxDepth; k++ {
			for p := uint32(0); p < 1<<k; p++ {
				var (
					b  = flip(k, p)
					at = nodeIndex(k, p)
				)

				bits[at] = b
				s.count[at] += int32(b)
			}
		}

		for i, pair := range s.pairs {
			s.joint[i] += int32(bits[pair[0]] & bits[pair[1]])
		}
	}

	return s
}

// nodePairs lists the sibling and parent-child pairs whose flips a true Owen
// scramble draws independently.
func nodePairs(maxDepth uint) [][2]int {
	var out [][2]int

	for k := uint(1); k <= maxDepth; k++ {
		for p := uint32(0); p < 1<<k; p++ {
			if p%2 == 0 {
				out = append(out, [2]int{nodeIndex(k, p), nodeIndex(k, p+1)})
			}

			out = append(out, [2]int{nodeIndex(k-1, p>>1), nodeIndex(k, p)})
		}
	}

	return out
}

// worstBias returns the largest |p - 1/2| over all nodes, in units of the
// sampling standard deviation, together with the raw deviation.
func (s *flipStats) worstBias() (z, dev float64, at int) {
	n := float64(s.seeds)
	sigma := 0.5 / math.Sqrt(n)

	for i, c := range s.count {
		d := math.Abs(float64(c)/n - 0.5)
		if d > dev {
			dev, at = d, i
		}
	}

	return dev / sigma, dev, at
}

// worstPairCorrelation returns the largest |phi| over the sibling and
// parent-child pairs, in units of the sampling standard deviation.
func (s *flipStats) worstPairCorrelation() (z, r float64, at [2]int) {
	n := float64(s.seeds)

	for i, pair := range s.pairs {
		var (
			pa = float64(s.count[pair[0]]) / n
			pb = float64(s.count[pair[1]]) / n
			va = pa * (1 - pa)
			vb = pb * (1 - pb)
		)

		if va == 0 || vb == 0 {
			continue
		}

		phi := math.Abs(float64(s.joint[i])/n-pa*pb) / math.Sqrt(va*vb)
		if phi > r {
			r, at = phi, pair
		}
	}

	return r * math.Sqrt(n), r, at
}

// TestOwenHashFlipsAreFairAtEveryNode is measurement one.
//
// It sweeps seeds through owenScramble and asks, node by node, how often the
// bit at that node is flipped. The statistic reported is the worst node, not
// the mean: a hash whose flips were fair on average but systematically stuck
// on a few branches would pass a mean test and produce a point set with a
// permanently mis-stratified region, which is precisely the failure a hash
// substituted for independent draws could have.
func TestOwenHashFlipsAreFairAtEveryNode(t *testing.T) {
	if testing.Short() {
		t.Skip("seed sweep over 2^13 nodes; -short skips it")
	}

	const (
		seeds    = 40000
		maxDepth = 12

		// The worst of 8191 nodes is the maximum of that many standard
		// normals, whose typical value is already near 3.9 sigma; 5.5 leaves
		// room for that maximum's own spread without admitting a real bias.
		tolerance = 5.5
	)

	stats := sweepFlips(seeds, maxDepth, func(si int) func(uint, uint32) uint32 {
		seed := owenSweepSeed(si)

		return func(k uint, prefix uint32) uint32 { return hashOwenFlip(k, prefix, seed) }
	})

	z, dev, at := stats.worstBias()
	t.Logf("hash Owen: worst node bias |p-1/2| = %.5f at node %d, %.2f sigma over %d seeds (1 sigma = %.5f)",
		dev, at, z, seeds, 0.5/math.Sqrt(seeds))

	if z > tolerance {
		t.Fatalf(
			"the worst of the %d nodes flips with probability %.5f away from 1/2, which is %.2f sigma "+
				"at %d seeds; the hash is not delivering a fair coin at that node, so the two halves of "+
				"the corresponding elementary interval are not equally likely to be swapped",
			len(stats.count), dev, z, seeds,
		)
	}
}

// owenSweepSeed maps a sweep index onto a scrambling seed the way a caller
// would get one.
//
// Consecutive seed indices are deliberately not used as scrambling seeds:
// newOwenSeeds runs the stream seed through splitMix64 before it ever reaches
// owenScramble, so consecutive uint32 seeds are not a case the package can
// produce, and measuring them would be measuring a call the library does not
// make.
func owenSweepSeed(i int) uint32 {
	rng := splitMix64(uint64(i) * 0x2545F4914F6CDD1D)
	rng.next()
	rng.next()

	return uint32(rng.next())
}

// TestOwenHashFlipsAreIndependentBetweenNodes is measurement two.
//
// Fair-but-correlated is a distinct failure from biased, and the one a hash is
// a priori more likely to have: four rounds of x ^= x*C is a short mixing
// function, and neighbouring nodes differ in a single input bit. Every sibling
// pair and every parent-child pair down to depth 12 is measured.
func TestOwenHashFlipsAreIndependentBetweenNodes(t *testing.T) {
	if testing.Short() {
		t.Skip("seed sweep over 2^13 nodes; -short skips it")
	}

	const (
		seeds    = 40000
		maxDepth = 12

		// Same argument as the bias tolerance: 12285 pairs, so the largest of
		// them sits near 4 sigma with nothing wrong.
		tolerance = 5.5
	)

	stats := sweepFlips(seeds, maxDepth, func(si int) func(uint, uint32) uint32 {
		seed := owenSweepSeed(si)

		return func(k uint, prefix uint32) uint32 { return hashOwenFlip(k, prefix, seed) }
	})

	z, r, at := stats.worstPairCorrelation()
	t.Logf("hash Owen: worst pairwise flip correlation |phi| = %.5f between nodes %d and %d, %.2f sigma over %d seeds",
		r, at[0], at[1], z, seeds)

	if z > tolerance {
		t.Fatalf(
			"nodes %d and %d flip together with correlation %.5f, %.2f sigma at %d seeds; a true Owen "+
				"scramble draws those two coins independently, and a hash that ties them together "+
				"removes randomization the caller is averaging over",
			at[0], at[1], r, z, seeds,
		)
	}
}

// TestExactOwenReferenceIsFairAndNested validates the reference the downstream
// comparisons are made against, and is the gate the mutation check aims at.
//
// The reference is only worth its cost if it is both a genuine Owen scramble —
// nested, so the comparison is between two scrambles and not between a
// scramble and an arbitrary hash — and genuinely fair, since the whole point
// is to be the thing hash Owen is approximating. Flattening its draw to a
// constant, which is the obvious way to write a reference that silently does
// nothing, fails the fairness half at 200 sigma. Measured, that flattening also
// fails the level-variance check and the correlation comparison — but not the
// integration one, because a reference flattened to the identity map is
// unrandomized Sobol, which integrates perfectly well. The integration
// comparison alone could not tell a broken reference from a good one.
func TestExactOwenReferenceIsFairAndNested(t *testing.T) {
	for k := 1; k <= 32; k++ {
		var (
			mask = ^uint32(0) << (32 - k)
			rng  = splitMix64(uint64(k) * 0x2545F4914F6CDD1D)
			e    = newExactOwen(0x1234567, exactOwenTableDepth)
		)

		prefix := uint32(rng.next()) & mask
		want := e.scramble(prefix) & mask

		for trial := 0; trial < 32; trial++ {
			x := prefix | uint32(rng.next())&^mask
			if got := e.scramble(x) & mask; got != want {
				t.Fatalf("k=%d: the reference scramble is not nested: %#08x and %#08x share their top %d bits "+
					"but land in different nodes at depth %d", k, prefix, x, k, k)
			}
		}
	}

	if testing.Short() {
		return
	}

	const (
		seeds     = 40000
		maxDepth  = 10
		tolerance = 5.5
	)

	stats := sweepFlips(seeds, maxDepth, func(si int) func(uint, uint32) uint32 {
		// Only the levels being measured are drawn: the reference's full
		// 16-level table is 65535 nodes and building one per seed would cost
		// thirty times what the measurement itself does.
		e := newExactOwen(uint64(owenSweepSeed(si)), maxDepth+1)

		return e.flip
	})

	z, dev, at := stats.worstBias()
	t.Logf("exact Owen: worst node bias |p-1/2| = %.5f at node %d, %.2f sigma over %d seeds", dev, at, z, seeds)

	if z > tolerance {
		t.Fatalf(
			"the reference scramble's worst node flips with probability %.5f away from 1/2, %.2f sigma at "+
				"%d seeds; the reference is not the fair coin the hash is being compared against, so every "+
				"comparison below is worthless",
			dev, z, seeds,
		)
	}
}

// levelFlipVarianceRatio measures how variable the number of flipped nodes at
// depth k is, as a fraction of the Binomial(2^k, 1/2) variance a true Owen
// scramble gives.
//
// This is the statistic that separates the hash from independent draws, and it
// is worth saying why the per-node measurements cannot. A level-wide balancing
// effect spreads over 2^k nodes, so it shows up in any single pair as a
// correlation of order 1/2^k — below the noise floor of any affordable seed
// sweep past depth 6 — while the sum over the level accumulates all of it.
func levelFlipVarianceRatio(k uint, seeds int, forSeed func(int) func(uint, uint32) uint32) float64 {
	nodes := 1 << k

	sumSq := 0.0

	for si := 0; si < seeds; si++ {
		flip := forSeed(si)

		ones := 0
		for p := uint32(0); p < uint32(nodes); p++ {
			ones += int(flip(k, p))
		}

		z := (float64(ones) - float64(nodes)/2) / (0.5 * math.Sqrt(float64(nodes)))
		sumSq += z * z
	}

	return sumSq / float64(seeds)
}

// TestOwenFlipsAreNotJointlyIndependentAcrossALevel is measurement three, and
// the one that finds something.
//
// It asserts almost nothing about the hash on purpose. The measured ratios run
// from 0.09 to 2.98 depending on depth and are a stable property of the
// construction, not a defect that could be fixed without changing constants
// this package has pinned; asserting a range around them would be pinning
// Burley's hash rather than testing this package's use of it. What is asserted
// is that the level has not collapsed to a deterministic pattern, which is
// what a scramble whose seed had stopped reaching the deeper levels would look
// like, and that the reference really does behave like independent coins —
// without which every comparison in this file is against nothing.
func TestOwenFlipsAreNotJointlyIndependentAcrossALevel(t *testing.T) {
	if testing.Short() {
		t.Skip("sweeps every node of sixteen levels over 2000 seeds; -short skips it")
	}

	const (
		seeds    = 2000
		maxDepth = 16

		// A level whose flip count never moves is the failure this can catch.
		// The measured minimum is 0.09 at depth 13, so the floor is set an
		// order of magnitude below that.
		floor = 0.01

		// The reference is 2^k independent fair bits by construction, so its
		// ratio is 1 up to the sampling error of 2000 seeds, which is about
		// 3%. The bounds are wide enough that only a broken reference fails.
		refLo, refHi = 0.8, 1.25
	)

	for k := uint(1); k <= maxDepth; k++ {
		hash := levelFlipVarianceRatio(k, seeds, func(si int) func(uint, uint32) uint32 {
			seed := owenSweepSeed(si)

			return func(kk uint, prefix uint32) uint32 { return hashOwenFlip(kk, prefix, seed) }
		})

		// The reference is only measured where drawing a table of 2^(k+1)
		// nodes per seed is affordable; past depth 12 that is 2000 tables of a
		// million nodes and the ratio has already been flat at every depth
		// below it.
		if k > 12 {
			t.Logf("depth %2d (%6d nodes): hash flip-count variance = %.3f of binomial, reference not measured",
				k, 1<<k, hash)

			continue
		}

		exact := levelFlipVarianceRatio(k, seeds, func(si int) func(uint, uint32) uint32 {
			return newExactOwen(uint64(owenSweepSeed(si)), k+1).flip
		})

		t.Logf("depth %2d (%6d nodes): hash flip-count variance = %.3f of binomial, reference = %.3f",
			k, 1<<k, hash, exact)

		if hash < floor {
			t.Fatalf(
				"at depth %d the hash flips a near-constant number of the %d nodes across seeds "+
					"(variance %.4f of binomial); the seed has stopped reaching that level, so the "+
					"randomization a caller averages over is not there",
				k, 1<<k, hash,
			)
		}

		if k <= 12 && (exact < refLo || exact > refHi) {
			t.Fatalf(
				"the reference scramble's flip count at depth %d has variance %.3f of the binomial "+
					"value; it is not drawing independent fair bits per node, so it is not the exact "+
					"Owen scramble the rest of this file compares against",
				k, exact,
			)
		}
	}
}

// coordScrambler scrambles the raw accumulator of one dimension.
type coordScrambler func(dim int, x uint32) uint32

// owenDimSeed derives a per-dimension seed exactly as newOwenSeeds does, so
// the reference and the hash are given the same stream of per-dimension seeds
// and differ only in what they do with them.
func owenDimSeed(seed uint64, dim int) uint64 {
	rng := splitMix64(seed ^ (uint64(dim)+1)*0x2545F4914F6CDD1D)
	rng.next()
	rng.next()

	return rng.next()
}

func hashOwenScrambler(dims int, seed uint64) coordScrambler {
	seeds := newOwenSeeds(dims, seed)

	return func(d int, x uint32) uint32 { return owenScramble(x, seeds[d]) }
}

func exactOwenScrambler(dims int, seed uint64) coordScrambler {
	tables := make([]*exactOwen, dims)
	for d := range tables {
		tables[d] = newExactOwen(owenDimSeed(seed, d), exactOwenTableDepth)
	}

	return func(d int, x uint32) uint32 { return tables[d].scramble(x) }
}

// owenSobolPoints draws n Sobol points with an arbitrary per-coordinate
// scramble applied to the raw accumulator.
//
// It goes through accumulate rather than through AtInto because the scramble
// under test is not one of the package's options: the reference exists only in
// this file. Everything else — direction numbers, Gray code, skip, scaling —
// is the generator's own code, so the two point sets differ in the scramble
// and nowhere else.
func owenSobolPoints(t *testing.T, dims, n, skip int, sc coordScrambler) [][]float64 {
	t.Helper()

	g, err := NewSobol(dims)
	if err != nil {
		t.Fatal(err)
	}

	var (
		raw = make([]uint32, dims)
		out = make([][]float64, n)
	)

	for i := range out {
		g.accumulate(uint32(skip+1+i), raw)

		p := make([]float64, dims)
		for d, x := range raw {
			p[d] = float64(sc(d, x)) * twoPowMinus32
		}

		out[i] = p
	}

	return out
}

// owenIntegrand is the integrand of integration_test.go, repeated here because
// that file is package qmc_test and this one has to be package qmc to reach
// owenScramble. The definition is copied rather than adapted: the figures
// below are only comparable with the package's published ones if the integrand
// and the budget are identical.
func owenIntegrand(x []float64) float64 {
	v := 1.0
	for k, xk := range x {
		v *= 1 + (1/float64(k+1))*(xk-0.5)
	}

	return v
}

func owenRMSError(t *testing.T, build func(int, uint64) coordScrambler, dims, n, streams int) float64 {
	t.Helper()

	sumSq := 0.0

	for seed := 1; seed <= streams; seed++ {
		sum := 0.0
		for _, p := range owenSobolPoints(t, dims, n, 64, build(dims, uint64(seed))) {
			sum += owenIntegrand(p)
		}

		e := sum/float64(n) - 1.0
		sumSq += e * e
	}

	return math.Sqrt(sumSq / float64(streams))
}

// owenMCRMSError is mcRMSError from integration_test.go, repeated for the same
// reason and with the same source seed and the same draw order, so that the
// ratios logged here can be read against the ones that file logs.
func owenMCRMSError(dims, n, streams int) float64 {
	rng := rand.New(rand.NewSource(20240823)) //nolint:gosec // statistical baseline, not cryptography

	sumSq := 0.0
	point := make([]float64, dims)

	for s := 0; s < streams; s++ {
		sum := 0.0

		for i := 0; i < n; i++ {
			for d := range point {
				point[d] = rng.Float64()
			}

			sum += owenIntegrand(point)
		}

		e := sum/float64(n) - 1.0
		sumSq += e * e
	}

	return math.Sqrt(sumSq / float64(streams))
}

// TestOwenApproximationCostsNothingOnIntegration is measurement four, first
// instrument: what the hash costs on the number the package is gated on.
func TestOwenApproximationCostsNothingOnIntegration(t *testing.T) {
	const (
		dims    = 39
		n       = 4096
		streams = 10

		// The two scrambles are different randomizations, not two runs of one,
		// so their RMS over ten streams differs by stream noise even if the
		// hash were perfect. A factor of 1.5 is the same tolerance
		// TestOwenBeatsDigitalShiftAt39Dims uses for the same reason.
		tolerance = 1.5
	)

	if testing.Short() {
		t.Skip("the reference scramble hashes every bit of every coordinate; -short skips it")
	}

	var (
		hashErr  = owenRMSError(t, hashOwenScrambler, dims, n, streams)
		exactErr = owenRMSError(t, exactOwenScrambler, dims, n, streams)
		mcErr    = owenMCRMSError(dims, n, streams)
	)

	t.Logf("d=%d n=%d streams=%d: hash Owen RMS %.3e (%.1fx MC), exact Owen RMS %.3e (%.1fx MC), hash/exact = %.3f",
		dims, n, streams, hashErr, mcErr/hashErr, exactErr, mcErr/exactErr, hashErr/exactErr)

	if hashErr > exactErr*tolerance {
		t.Fatalf(
			"hash Owen integrates at RMS %.3e against the exact reference's %.3e, %.2fx worse; the "+
				"approximation in the per-node permutation is costing accuracy rather than being free",
			hashErr, exactErr, hashErr/exactErr,
		)
	}
}

// owenWorstCorrelations returns the worst adjacent-pair |r| for each of seeds
// consecutive stream seeds.
func owenWorstCorrelations(t *testing.T, build func(int, uint64) coordScrambler, seeds int) []float64 {
	t.Helper()

	out := make([]float64, seeds)
	for i := range out {
		pts := owenSobolPoints(t, corrDims, corrPoints, corrSkip, build(corrDims, uint64(i+1)))
		out[i], _ = worstAdjacentCorrelation(pts)
	}

	return out
}

func median(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)

	return s[len(s)/2]
}

// TestOwenApproximationCostsNothingOnCorrelation is measurement four, second
// instrument.
//
// Thirty seeds, not five. docs/testing-methodology.md records a five-seed worst
// case moving from 0.40 to 0.12 on a pure re-instantiation of a scrambling
// scheme, so a five-seed comparison between two scrambles would be measuring
// the seeds.
func TestOwenApproximationCostsNothingOnCorrelation(t *testing.T) {
	const (
		seeds     = 30
		tolerance = 1.5
	)

	if testing.Short() {
		t.Skip("thirty seeds through the reference scramble; -short skips it")
	}

	var (
		hash  = owenWorstCorrelations(t, hashOwenScrambler, seeds)
		exact = owenWorstCorrelations(t, exactOwenScrambler, seeds)
	)

	var hashWorst, exactWorst float64
	for i := range hash {
		hashWorst = math.Max(hashWorst, hash[i])
		exactWorst = math.Max(exactWorst, exact[i])
	}

	t.Logf("d=%d n=%d over %d seeds: hash Owen worst |corr| %.4f (median %.4f), exact Owen worst %.4f (median %.4f)",
		corrDims, corrPoints, seeds, hashWorst, median(hash), exactWorst, median(exact))

	if hashWorst > exactWorst*tolerance {
		t.Fatalf(
			"hash Owen's worst adjacent-pair correlation over %d seeds is %.4f against the exact "+
				"reference's %.4f; the hash is leaving structure between coordinates that independent "+
				"per-node draws remove",
			seeds, hashWorst, exactWorst,
		)
	}
}
