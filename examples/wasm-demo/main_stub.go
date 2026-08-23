//go:build !js || !wasm

package main

// This stub keeps the package buildable for non-WASM targets. Without it
// `go vet ./...` and every editor's background build fail on the host with
// "function main is undeclared in the main package", because every other file
// here is behind a js && wasm constraint.
func main() {}
