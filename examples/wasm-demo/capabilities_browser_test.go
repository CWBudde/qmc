//go:build js && wasm && qmc_browser_fixture

package main

import "testing"

// Product menus describe a deliberately limited subset of library behavior.
// Keep the constructor as the authority without duplicating a table ceiling.
func verifyDemoCapabilities(t *testing.T) {
	t.Helper()

	for _, source := range sourceOrder {
		spec := sources[source]
		if spec.construct == nil {
			continue
		}

		for _, dims := range []int{1, spec.maxDims} {
			for _, randomization := range spec.randomizations {
				g, err := newGenerator(source, dims, 0, 1, randomization, 31)
				if err != nil {
					t.Fatalf("menu offers unsupported %s/%s at %d dimensions: %v", source, randomization, dims, err)
				}

				if g.Dims() != dims {
					t.Fatalf("menu dimension contract changed for %s", source)
				}
			}
		}
	}
}
