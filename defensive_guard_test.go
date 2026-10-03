package qmc

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestRadicalInverseDefensiveBaseAndIndexGuards(t *testing.T) {
	for _, tc := range [][2]int{{1, 0}, {1, 1}, {1, -2}, {-1, 2}} {
		index, base := tc[0], tc[1]
		for name, value := range map[string]float64{
			"plain":  radicalInverse(index, base),
			"digit":  scrambledRadicalInverse(index, base, nil),
			"nested": nestedRadicalInverse(index, base, 17),
		} {
			if value != 0 {
				t.Errorf("%s(%d, %d) = %g, want 0", name, index, base, value)
			}
		}
	}
}

func TestDigitReversalRefusesBoundaryExcludedByFuzzMapping(t *testing.T) {
	// The valid reverse permutation maps eight low zero digits to 166, so
	// the accumulated reversal is 167^8-1 before the ninth multiplication.
	// That multiplication exceeds uint64, although raw index 167^8 fits int64.
	const base = 167

	power := uint64(1)
	for range 8 {
		power *= base
	}

	if power > uint64(math.MaxInt) {
		t.Skip("the raw index causing uint64 reversal overflow does not fit int32")
	}

	perm := make([]int32, base)
	for i := range perm {
		perm[i] = int32(base - 1 - i)
	}

	defer func() {
		err := recover()
		if err == nil || !strings.Contains(fmt.Sprint(err), "reverse without overflow") {
			t.Errorf("digit reversal overflow: recovered %v, want the specific refusal", err)
		}
	}()

	scrambledRadicalInverse(int(power), base, perm)
}

func TestCorrelationDoesNotHideNonfiniteCoordinates(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		got, _ := worstAdjacentCorrelation([][]float64{{value, 0.5}, {0.25, 0.75}})
		if finiteDiscrepancyTerm(got) {
			t.Fatalf("nonfinite input %g disappeared into successful correlation %g", value, got)
		}
	}
}
