package qmc

import (
	"math"
	"reflect"
	"testing"
)

// These callers use small fixed dimensions; constructor error handling is
// covered separately through the public API.
func mustPrimes(n int) []int {
	p, err := primesUpTo(n)
	if err != nil {
		panic(err)
	}

	return p
}

func TestPrimeSieveBoundedExpansion(t *testing.T) {
	got, err := primesFromLimit(10, 3) // expands through 6, 12, 24, and 48
	if err != nil {
		t.Fatal(err)
	}

	want := []int{2, 3, 5, 7, 11, 13, 17, 19, 23, 29}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expanded sieve = %v, want %v", got, want)
	}
}

func TestPrimeSieveGrowthLimit(t *testing.T) {
	for _, limit := range []int{0, -1, math.MaxInt/2 + 1, math.MaxInt} {
		if _, err := growPrimeLimit(limit); err == nil {
			t.Errorf("cannot safely double %d", limit)
		}
	}

	if got, err := growPrimeLimit(math.MaxInt / 2); err != nil || got != math.MaxInt-1 {
		t.Fatalf("last valid doubled bound = %d, %v", got, err)
	}
}

func TestHaltonRejectsOverflowingPrimeLimit(t *testing.T) {
	for _, dims := range []int{math.MaxInt, math.MaxInt/8 + 1, math.MaxInt/15 + 1} {
		if _, err := NewHalton(dims); err == nil {
			t.Fatalf("dimension count %d with unrepresentable sieve bound was accepted", dims)
		}
	}
}

func TestHaltonPrimeSieveExpandsAt637235Dimensions(t *testing.T) {
	if testing.Short() {
		t.Skip("large sieve regression; bounded growth is covered by the routine suite")
	}

	const dims = 637235

	g, err := NewHalton(dims)
	if err != nil {
		t.Fatal(err)
	}
	// The initial exclusive sieve bound is 9558525, eight below the required
	// last prime. 9558533 is checked against the independent prime-counting
	// reference pi(9558533)=637235, not computed by this implementation.
	if got := g.Bases()[dims-1]; got != 9558533 {
		t.Fatalf("prime %d = %d, want 9558533", dims, got)
	}
}
