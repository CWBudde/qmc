# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `(*Halton).Bases()` returns the prime base of each dimension, in order, as a
  defensive copy. The d-th base is the d-th prime, and it is what explains a
  dimension's behaviour: dimension 38 uses base 167.
- `(*Halton).Permutation(dim)` returns the random-digit scrambling permutation
  of `{0..base-1}` applied to that dimension, again as a copy, or `nil` for an
  unscrambled generator or an out-of-range dimension.
- A WebAssembly web demo in `examples/wasm-demo/`, published to GitHub Pages.
  Everything it shows is computed by this library compiled to `js/wasm`; there
  is no JavaScript reimplementation of the sequence.
- CI split into `test.yml`, `wasm-demo-pages.yml` and `release.yml`, so a demo
  deployment and a release no longer ride on the test workflow.
- A `justfile` and a `treefmt.toml` describing the toolchain, so formatting and
  the common tasks are the same locally and in CI.

### Changed

- Corrected stale comments in `.golangci.yml` that had been copied from a
  sibling DSP repository and described identifiers and a project layout that do
  not exist here.

## [0.1.2] - 2026-08-23

### Fixed

- `sieve` computed `i*i` in `int`. Where `int` is 32 bits that product can wrap
  to a positive value, pass the old `j > 0` guard and mark a slot it has no
  business marking, dropping a real prime — so a dimension got a different base
  on a 32-bit build than on a 64-bit one. A generator whose sequence depends on
  `GOARCH` is not reproducible, which is the one property this package exists to
  provide. The product is now computed in `uint64`.
- The test matrix gained a `386` leg. `TestSieveIsArchitectureIndependent` had
  been pinned to this bug since it was written, but it passes on amd64 whether
  or not the guard is present, so nothing had ever run it where it could fail.

## [0.1.1] - 2026-08-23

### Note

The commit shipped under this tag claims to fix the 32-bit sieve overflow
described under 0.1.2. It does not: the edit did not reach `primes.go`, and
nothing caught the discrepancy because the regression test could not fail on
the only architecture CI ran. **v0.1.1 contains the bug in full.** Callers on a
32-bit target should use 0.1.2 or later; on a 64-bit target the two are
equivalent. This entry exists because a published tag cannot be rewritten and a
changelog is the only place the correction survives.

## [0.1.0] - 2026-08-23

### Added

- Initial release: the Halton sequence with optional random-digit scrambling,
  `WithSkip` and `WithScrambling`, and stateless `At`/`AtInto` alongside the
  stateful `Next`/`NextInto`/`Reset`.
