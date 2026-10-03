package qmc

import (
	"math"
	"slices"
	"sync"
	"testing"
)

func TestNestedRootCacheBoundedAndExact(t *testing.T) {
	for _, dims := range []int{1, 39, 97, 98, 1000} {
		for _, seed := range []uint64{1, 31} {
			g, err := NewHalton(dims, WithNestedScrambling(seed))
			if err != nil {
				t.Fatal(err)
			}

			n := g.nest
			if len(n.rootPermutations) > nestedRootCacheEntries {
				t.Fatal("root tables exceeded their fixed entry budget")
			}

			if len(n.rootOffsets) > nestedRootCacheEntries/2+1 {
				t.Fatal("root offsets are not bounded by cached entries")
			}

			if dims == 1000 && n.rootPermutation(dims-1) != nil {
				t.Fatal("cache grew with all requested dimensions")
			}

			for d, base := range g.bases {
				perm := n.rootPermutation(d)
				for _, index := range []int{0, 1, 65, 4160, 1234567, math.MaxInt / 3, math.MaxInt} {
					got := nestedInverse(index, base, n.roots[d], perm)

					want := nestedRadicalInverse(index, base, n.roots[d])
					if got != want {
						t.Fatalf("cache changed value: d=%d seed=%d index=%d got=%g want=%g", d, seed, index, got, want)
					}
				}
			}
		}
	}
}

func TestNestedRootCacheReadsAreImmutable(t *testing.T) {
	g, err := NewHalton(100, WithSkip(64), WithNestedScrambling(31))
	if err != nil {
		t.Fatal(err)
	}

	roots, offsets, permutations := slices.Clone(g.nest.roots), slices.Clone(g.nest.rootOffsets), slices.Clone(g.nest.rootPermutations)

	const points = 128

	reference := Draw(g, points)

	var group sync.WaitGroup
	for worker := range 8 {
		group.Add(1)
		go func() {
			defer group.Done()

			dst := make([]float64, g.Dims())
			for i := worker; i < points; i += 8 {
				g.AtInto(i, dst)

				if !slices.Equal(dst, reference[i]) {
					t.Errorf("concurrent indexed cache result changed at %d", i)
				}
			}
		}()
	}

	group.Wait()

	if !slices.Equal(roots, g.nest.roots) || !slices.Equal(offsets, g.nest.rootOffsets) || !slices.Equal(permutations, g.nest.rootPermutations) {
		t.Fatal("indexed calls modified immutable cache storage")
	}
}
