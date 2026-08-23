package qmc

// This file owns the Joe-Kuo direction numbers: where they come from, how they
// are parsed, and — the part that earns the file its length — how they are
// checked before anything is generated from them.
//
// Provenance. The table is the first 1024 dimensions of
// https://web.maths.unsw.edu.au/~fkuo/sobol/new-joe-kuo-6.21201, the authors'
// recommended set (search criterion D(6), last updated 16 September 2010),
// fetched with curl's --insecure flag because this project's CA store cannot
// build a chain to that host. That is worth stating plainly: the bytes arrived
// over a connection nobody authenticated, so trusting them because of where
// they appeared to come from is not available. They are trusted instead
// because they are checked against properties a corrupted or truncated
// download cannot satisfy by accident — see validateDirectionRows below, and
// TestEmbeddedTableSatisfiesItsInvariants, which runs those checks over the
// committed asset on every test run.
//
// third_party/joe-kuo/LICENSE.txt is the licence from
// https://web.maths.unsw.edu.au/~fkuo/sobol/licence, verbatim: BSD-3-Clause,
// copyright 2008 Frances Y. Kuo and Stephen Joe.

import (
	"bufio"
	_ "embed"
	"fmt"
	"io"
	"math/bits"
	"strconv"
	"strings"
)

// sobolBits is the number of direction numbers held per dimension, and so the
// number of bits of index the generator can consume. Thirty-two is not a
// tuning knob: the direction numbers are uint32 and a coordinate is X/2^32, so
// widening this would change every point the package has ever produced.
const sobolBits = 32

// maxSobolDims is the number of dimensions in the embedded table.
const maxSobolDims = 1024

// embeddedDirectionNumbers is the committed asset, byte-for-byte the first
// 1024 lines of the upstream file — one header line plus rows for dimensions
// 2 through 1024. Its SHA-256 is
//
//	52eeb57738dd69f5e1f593f2085c4efb3fb410c3af241d5396d7a2bef60b9257
//
// and the upstream file it was cut from hashes to
//
//	68eedd2a4e3b659b9695e7aff0f8ac68718bcf620730fc3d3a8c65df2a067441
//
// so a future re-fetch is comparable without guesswork: `head -1024` of the
// re-fetched file must reproduce the first hash exactly.
//
// It is stored as upstream's text rather than packed binary, and that costs
// something — 62 KB against roughly 26 KB for two-byte fields — so the reason
// has to be better than habit. A packed form would need its own writer, and
// that writer would be the one piece of the table's handling that no test
// could reach, because the only input it would ever run on is the one the
// committed bytes already came from. Keeping upstream's format means
// parseDirectionNumbers is the single parser: the embedded table and a
// caller's file supplied through WithDirectionNumbers go through the same code
// and the same validator, so there is no second path to drift, and every test
// written against one is a test of the other. Reviewing a change to the table
// is also a diff of decimal numbers rather than of a blob. 62 KB is well
// inside the budget; the saving buys nothing worth an untested code path.
//
//go:embed third_party/joe-kuo/new-joe-kuo-6.1024
var embeddedDirectionNumbers string

// directionRow is one dimension's entry: the degree of its primitive
// polynomial, the polynomial itself, and its initial direction numbers.
//
// Dimension 1 has no row. Its polynomial is the empty one and all of its m_i
// are 1, which is a special case in every Sobol implementation and is handled
// where the direction numbers are expanded rather than by inventing a row that
// the file format cannot express.
type directionRow struct {
	// dim is the dimension the file says this row is for. It is kept only so
	// that validateDirectionRows can compare it against the row's position in
	// the slice, which is what actually selects the dimension the row will be
	// used for. The two disagreeing is the whole point: see the contiguity
	// paragraph on validateDirectionRows.
	dim int

	// degree is s, the degree of the primitive polynomial, and also len(m).
	degree int

	// poly is the polynomial with bit e holding the coefficient of x^e, so it
	// always has bit 0 and bit degree set.
	//
	// The file stores only the interior coefficients, as the integer a whose
	// bit (s-1-k) is the coefficient of x^(s-k) for k = 1..s-1. Substituting
	// e = s-k turns that into: bit (e-1) of a is the coefficient of x^e. So
	// the whole file-format convention is poly = 1<<s | a<<1 | 1, applied once
	// here, and nothing downstream has to remember which end a is indexed
	// from. That off-by-one is the classic way to get a Sobol table subtly
	// wrong — the resulting points still look like points.
	poly uint64

	// m holds m_1..m_degree, indexed from zero: m[i] is m_(i+1).
	m []uint32
}

// parseDirectionNumbers reads the Joe-Kuo text format and returns one row per
// dimension, starting at dimension 2.
//
// The format is a header line followed by rows `d s a m_1 ... m_s`. A leading
// line whose first field is not an integer is taken as the header and skipped;
// upstream ships one, a caller's hand-made file may not, and refusing a file
// for the absence of a line nobody reads would be pedantry.
//
// Everything it returns has been through validateDirectionRows, so a caller
// holding the result holds a table that has already been proved consistent.
func parseDirectionNumbers(r io.Reader) ([]directionRow, error) {
	rows := make([]directionRow, 0, maxSobolDims)
	scanner := bufio.NewScanner(r)

	for lineNo := 1; scanner.Scan(); lineNo++ {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}

		if lineNo == 1 {
			if _, err := strconv.Atoi(fields[0]); err != nil {
				continue
			}
		}

		row, err := parseDirectionRow(fields)
		if err != nil {
			return nil, fmt.Errorf("qmc: direction numbers, line %d: %w", lineNo, err)
		}

		rows = append(rows, row)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("qmc: reading direction numbers: %w", err)
	}

	if err := validateDirectionRows(rows); err != nil {
		return nil, err
	}

	return rows, nil
}

// parseDirectionRow converts one whitespace-split row. It checks only what it
// needs in order to build a directionRow at all — field counts and value
// ranges that would make the fields meaningless. The properties that say
// whether the numbers are the right numbers live in validateDirectionRows,
// because those are the ones a caller-supplied file and the embedded asset
// must be held to identically.
func parseDirectionRow(fields []string) (directionRow, error) {
	if len(fields) < 4 {
		return directionRow{}, fmt.Errorf("want at least 4 fields (d s a m_1), got %d", len(fields))
	}

	degree, err := strconv.Atoi(fields[1])
	if err != nil {
		return directionRow{}, fmt.Errorf("degree s: %w", err)
	}

	// A degree above sobolBits cannot be represented: the initial direction
	// numbers for i > 32 would be scaled by a negative shift, and there is no
	// slot to put them in. Upstream's largest degree inside 1024 dimensions is
	// 13, so this only ever fires on a file that is not a Joe-Kuo table.
	if degree < 1 || degree > sobolBits {
		return directionRow{}, fmt.Errorf("degree s = %d, want 1..%d", degree, sobolBits)
	}

	if len(fields) != 3+degree {
		return directionRow{}, fmt.Errorf(
			"degree s = %d needs %d direction numbers, got %d", degree, degree, len(fields)-3,
		)
	}

	a, err := strconv.ParseUint(fields[2], 10, 64)
	if err != nil {
		return directionRow{}, fmt.Errorf("polynomial coefficients a: %w", err)
	}

	// a encodes s-1 interior coefficients. A value with bits above that is not
	// a truncation of a valid row, it is a different file format, and shifting
	// it into poly would silently drop the excess.
	if degree > 1 && a >= 1<<uint(degree-1) {
		return directionRow{}, fmt.Errorf(
			"polynomial coefficients a = %d do not fit the %d interior bits of a degree-%d polynomial",
			a, degree-1, degree,
		)
	}

	row := directionRow{
		degree: degree,
		poly:   1<<uint(degree) | a<<1 | 1,
		m:      make([]uint32, degree),
	}

	for i := range row.m {
		v, err := strconv.ParseUint(fields[3+i], 10, 32)
		if err != nil {
			return directionRow{}, fmt.Errorf("direction number m_%d: %w", i+1, err)
		}

		row.m[i] = uint32(v)
	}

	// d is carried but not acted on here: contiguity is a property of the
	// sequence of rows, not of any one row, so validateDirectionRows checks it
	// against each row's position instead. Parsing it also keeps a
	// shifted-by-one-column file from being read as a valid table with
	// everything in the wrong field.
	row.dim, err = strconv.Atoi(fields[0])
	if err != nil {
		return directionRow{}, fmt.Errorf("dimension d: %w", err)
	}

	return row, nil
}

// validateDirectionRows proves that rows are a usable Joe-Kuo table.
//
// This is the function that stands in for the authentication the download did
// not have. Each check below rejects a whole class of damage that would
// otherwise reach the generator and come back out as points — and points are
// the one form of output where being wrong is invisible, because a table of
// corrupted direction numbers still produces numbers in [0,1) that scatter
// across the cube. There is nothing to notice.
//
//   - Every m_i must be odd. The direction number V_i is m_i << (32-i), and
//     the (t,m,s)-net structure needs the V_i to be linearly independent over
//     GF(2); an even m_i leaves V_i's leading bit clear and collapses that
//     independence. Truncation and byte corruption both produce even values
//     roughly half the time, so this is the cheapest check that catches them.
//
//   - Every m_i must be below 2^i, because m_i << (32-i) must not overflow
//     past the top of the word. An out-of-range value would be shifted out of
//     the uint32 and the dimension would quietly lose its leading digits.
//
//   - The polynomial must be primitive over GF(2). This is the one that cannot
//     be passed by accident. Primitive polynomials of degree s are a small
//     fraction of the 2^(s-1) candidates — for s = 13 it is 630 out of 4096 —
//     so a corrupted a field fails it with high probability, and a row that
//     survives it is doing arithmetic that only a real table does. It is also
//     the property the construction actually depends on: expandDirections runs
//     a linear recurrence over GF(2) whose characteristic polynomial is this
//     one, and the recurrence only has full period 2^s-1 when the polynomial
//     is primitive.
//
// Contiguity from d = 2 is checked because a row's position in the slice is
// what selects the dimension it will be used for. A file missing one line
// shifts every dimension after it onto the wrong polynomial, which is a defect
// with no symptom at all: every dimension still has a valid primitive
// polynomial and valid direction numbers, just not its own, and the
// two-dimensional projections the Joe-Kuo search optimised are gone.
func validateDirectionRows(rows []directionRow) error {
	if len(rows) == 0 {
		return fmt.Errorf("qmc: direction numbers contain no dimensions")
	}

	for i, row := range rows {
		dim := i + 2

		if row.dim != dim {
			return fmt.Errorf(
				"qmc: direction numbers: row %d is labelled dimension %d but sits where dimension %d must be; "+
					"d has to run contiguously from 2 or every later dimension uses another dimension's polynomial",
				i+1, row.dim, dim,
			)
		}

		if len(row.m) != row.degree {
			return fmt.Errorf(
				"qmc: direction numbers, dimension %d: degree %d but %d direction numbers",
				dim, row.degree, len(row.m),
			)
		}

		for k, m := range row.m {
			if m%2 == 0 {
				return fmt.Errorf(
					"qmc: direction numbers, dimension %d: m_%d = %d is even, every m_i must be odd",
					dim, k+1, m,
				)
			}

			if uint64(m) >= 1<<uint(k+1) {
				return fmt.Errorf(
					"qmc: direction numbers, dimension %d: m_%d = %d must be below 2^%d",
					dim, k+1, m, k+1,
				)
			}
		}

		if !isPrimitiveOverGF2(row.poly, row.degree) {
			return fmt.Errorf(
				"qmc: direction numbers, dimension %d: polynomial %#b of degree %d is not primitive over GF(2)",
				dim, row.poly, row.degree,
			)
		}
	}

	return nil
}

// isPrimitiveOverGF2 reports whether poly — degree s, bit e holding the
// coefficient of x^e — is primitive: whether x generates the whole
// multiplicative group of GF(2)[x]/(poly).
//
// The order of x is computed rather than the polynomial being factored,
// because the order test is a complete proof on its own and irreducibility is
// not. If x has order exactly 2^s-1 then it generates 2^s-1 distinct units,
// the ring has 2^s elements, so every nonzero element is a unit; that makes
// the ring a field, poly irreducible, and x a generator. Testing only for
// irreducibility would accept polynomials the construction cannot use —
// x^4+x^3+x^2+x+1 is irreducible over GF(2) but x has order 5, not 15, so the
// direction-number recurrence built on it would repeat after 5 steps instead
// of 15.
//
// The order is established by the standard two-part test rather than by
// stepping x up to 2^s-1 times. Stepping is fine for the embedded table, whose
// largest degree is 13, but WithDirectionNumbers accepts upstream's full file,
// where s reaches 18: at 21200 rows that is billions of multiplications on a
// path a caller sits and waits on. The test below costs a few dozen per row.
func isPrimitiveOverGF2(poly uint64, s int) bool {
	order := uint64(1)<<uint(s) - 1

	// x^(2^s-1) = 1 says the order divides 2^s-1; the loop then rules out
	// every proper divisor at once, since any proper divisor of 2^s-1 divides
	// (2^s-1)/q for at least one prime q of 2^s-1. Neither half is sufficient
	// alone: without the first, an x that is not a unit at all would pass the
	// loop vacuously.
	if gf2PowX(order, poly, s) != 1 {
		return false
	}

	for _, q := range distinctPrimeFactors(order) {
		if gf2PowX(order/q, poly, s) == 1 {
			return false
		}
	}

	return true
}

// gf2PowX returns x^e in GF(2)[x]/(poly), as a bitmask in the same convention
// as poly.
func gf2PowX(e uint64, poly uint64, s int) uint64 {
	// The base is x reduced, not the literal bit pattern 0b10. They differ
	// only at s = 1, where the ring is GF(2)[x]/(x+1) and x reduces to 1 — the
	// degenerate dimension-2 row of every Joe-Kuo file, so the case that looks
	// least worth handling is the first one the table exercises.
	base := gf2MulByX(1, poly, s)
	result := uint64(1)

	for ; e > 0; e >>= 1 {
		if e&1 != 0 {
			result = gf2Mul(result, base, poly, s)
		}

		base = gf2Mul(base, base, poly, s)
	}

	return result
}

// gf2Mul multiplies two reduced elements of GF(2)[x]/(poly).
func gf2Mul(a, b, poly uint64, s int) uint64 {
	var result uint64

	for ; b > 0; b >>= 1 {
		if b&1 != 0 {
			result ^= a
		}

		a = gf2MulByX(a, poly, s)
	}

	return result
}

// gf2MulByX multiplies a reduced element by x and reduces again. poly has bit
// s set by construction, so XORing it is exactly what clears the bit that the
// shift pushed out of range.
func gf2MulByX(a, poly uint64, s int) uint64 {
	a <<= 1
	if a>>uint(s)&1 != 0 {
		a ^= poly
	}

	return a
}

// distinctPrimeFactors returns the distinct primes dividing n, by trial
// division. n here is 2^s-1 with s at most 32, so the loop runs to at most
// 65535, which is not measurable against the file parse that precedes it.
// Nothing is cached: a table is parsed once per generator, not once per point.
func distinctPrimeFactors(n uint64) []uint64 {
	var factors []uint64

	for p := uint64(2); p*p <= n; p++ {
		if n%p != 0 {
			continue
		}

		factors = append(factors, p)

		for n%p == 0 {
			n /= p
		}
	}

	if n > 1 {
		factors = append(factors, n)
	}

	return factors
}

// expandDirections turns one dimension's initial direction numbers into all
// sobolBits of them, scaled so that V_i occupies the top i bits of a uint32.
//
// A nil row is the implicit first dimension, which has no line in the file and
// whose V_i are simply 1<<(32-i). It is spelled out here rather than faked as
// a degree-0 row because the recurrence below has no meaningful form at degree
// 0, and a fake row would have to be excluded from the validator that every
// other row goes through — an exception in the one place the package cannot
// afford them.
//
// The recurrence is Joe and Kuo's, in the scaled form: for i > s,
//
//	V_i = V_(i-s) XOR (V_(i-s) >> s) XOR (sum over k of a_k * V_(i-k))
//
// It is written against poly's bits rather than against the file's a, so the
// indexing convention is stated once, in directionRow.poly, and read once,
// here.
func expandDirections(row *directionRow, dst []uint32) {
	if row == nil {
		for i := range dst {
			dst[i] = 1 << uint(sobolBits-1-i)
		}

		return
	}

	s := row.degree
	for i := 0; i < len(dst) && i < s; i++ {
		dst[i] = row.m[i] << uint(sobolBits-1-i)
	}

	for i := s; i < len(dst); i++ {
		v := dst[i-s] ^ dst[i-s]>>uint(s)

		for k := 1; k < s; k++ {
			if row.poly>>uint(s-k)&1 != 0 {
				v ^= dst[i-k]
			}
		}

		dst[i] = v
	}
}

// lowestZeroBit returns the position of the lowest zero bit of n, counting
// from zero.
//
// It is the Gray-code recurrence's entire state transition: gray(n+1) differs
// from gray(n) in exactly this one bit. n is uint32 and the caller has already
// refused n = 2^32-1, so there is always a zero bit to find and the
// TrailingZeros32-of-zero case — which would answer 32 and index one past the
// direction numbers — cannot arise here.
func lowestZeroBit(n uint32) int {
	return bits.TrailingZeros32(^n)
}
