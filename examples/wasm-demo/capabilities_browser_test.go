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

	// Every tour step must be a request the page can make without the
	// randomization menu or the dimension clamp quietly substituting another.
	for _, step := range pointLabTour {
		spec, ok := sources[step.source]
		if !ok || spec.construct == nil {
			t.Fatalf("tour step %s names unknown sequence %q", step.key, step.source)
		}

		offered := false

		for _, key := range spec.randomizations {
			offered = offered || key == step.randomization
		}

		if !offered {
			t.Fatalf("tour step %s: %s does not offer randomization %q", step.key, step.source, step.randomization)
		}

		if step.dims < 2 || step.dims > spec.maxDims || step.count < 1 || step.count > maxPoints ||
			step.skip < 0 || step.skip > maxSkip ||
			step.axisX < 0 || step.axisX >= step.dims || step.axisY < 0 || step.axisY >= step.dims ||
			step.axisX == step.axisY {
			t.Fatalf("tour step %s is outside the page's limits: %+v", step.key, step)
		}

		if _, err := newGenerator(step.source, step.dims, step.skip, defaultLeap, step.randomization, defaultSeed); err != nil {
			t.Fatalf("tour step %s is refused: %v", step.key, err)
		}
	}
}
