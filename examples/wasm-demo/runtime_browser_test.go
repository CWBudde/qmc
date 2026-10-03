//go:build js && wasm && qmc_browser_fixture

package main

import (
	"os"
	"syscall/js"
	"testing"
)

// This deliberately long-lived test binary is loaded only by the browser
// regression runner. Neither fixture export exists in production builds.
func TestBrowserRuntimeFixture(t *testing.T) {
	verifyDigitInspector(t)

	exports["testPanic"] = func(js.Value) any { panic("browser fixture request panic") }
	exports["testExit"] = func(js.Value) any {
		os.Exit(0)
		return nil
	}

	main()
}
