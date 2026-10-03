//go:build js && wasm && qmc_browser_fixture

package main

import (
	"math"
	"syscall/js"
	"testing"

	"github.com/cwbudde/qmc"
)

// Executed before the long-lived runtime fixture starts. Independent library
// construction checks the inspector without adding production exports or APIs.
func verifyDigitInspector(t *testing.T) {
	t.Helper()

	for _, randomization := range []string{"none", "scramble", "nested"} {
		for _, request := range []struct{ index, dim, skip, leap int }{
			{0, 0, 0, 1},
			{17, 1, 64, 5},
			{256, 38, 123, 173},
			{maxIndex, maxDims - 1, maxSkip, 997},
			{-1, -1, -1, 0},
			{maxIndex + 1, maxDims, maxSkip + 1, 997},
		} {
			for _, seed := range []int{1, 31} {
				opts := js.ValueOf(map[string]any{
					"source": "halton", "randomization": randomization, "seed": seed,
					"index": request.index, "dim": request.dim, "skip": request.skip, "leap": request.leap,
				})

				result := jsDigits(opts).(map[string]any)
				if result["error"] != nil {
					t.Fatalf("digits rejected valid case %+v/%s: %v", request, randomization, result)
				}

				index, dim := clampInt(request.index, 0, maxIndex), clampInt(request.dim, 0, maxDims-1)
				skip, leap := clampInt(request.skip, 0, maxSkip), clampInt(request.leap, 1, maxLeap)
				options := []qmc.Option{qmc.WithSkip(skip), qmc.WithLeap(leap)}

				switch randomization {
				case "scramble":
					options = append(options, qmc.WithScrambling(uint64(seed)))
				case "nested":
					options = append(options, qmc.WithNestedScrambling(uint64(seed)))
				}

				generator, err := qmc.NewHalton(dim+1, options...)
				if err != nil {
					t.Fatal(err)
				}

				if result["value"] != generator.At(index)[dim] || result["index"] != index || result["dim"] != dim {
					t.Fatalf("coordinate/configuration drift: %+v", result)
				}

				base := result["base"].(int)

				var reconstructed int

				power, weight, plain := 1, 1/float64(base), 0.0
				fixed := 0.0
				perm := generator.Permutation(dim)

				for k, entry := range result["digits"].([]any) {
					digit := entry.(int)
					if digit < 0 || digit >= base {
						t.Fatalf("digit outside alphabet: %d", digit)
					}

					reconstructed += digit * power
					plain += float64(digit) * weight

					if perm != nil {
						mapped := result["permuted"].([]any)[k].(int)
						if mapped != int(perm[digit]) {
							t.Fatalf("permuted digit drift at %d", k)
						}

						fixed += float64(mapped) * weight
					}

					power *= base
					weight /= float64(base)
				}

				if reconstructed != skip+1+index*leap || result["rawIndex"] != reconstructed {
					t.Fatalf("raw-index/digit drift: %+v", result)
				}

				if math.Abs(plain-result["unscrambledValue"].(float64)) > 2e-15 {
					t.Fatalf("plain expansion disagrees with library: %+v", result)
				}

				if perm == nil {
					if result["permutation"] != nil || result["permuted"] != nil {
						t.Fatal("nonfixed randomization reported a fixed permutation")
					}
				} else {
					fixed += float64(perm[0]) * weight / (1 - 1/float64(base))
					if math.Abs(fixed-result["value"].(float64)) > 2e-15 {
						t.Fatalf("fixed expansion/tail disagrees with library: %+v", result)
					}
				}
			}
		}
	}

	for _, opts := range []map[string]any{
		{"source": "unknown"},
		{"source": "sobol"},
		{"source": "random"},
		{"randomization": "unknown"},
		{"randomization": "shift"},
		{"dim": 1, "leap": 2},
		{"leap": maxLeap + 1},
	} {
		if result := jsDigits(js.ValueOf(opts)).(map[string]any); result["error"] == nil {
			t.Fatalf("digits accepted unsupported request: %+v", opts)
		}
	}
}
