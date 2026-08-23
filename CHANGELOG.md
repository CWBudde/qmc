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
- `integration_test.go` measures what the package exists to provide: RMS
  integration error against plain Monte Carlo on the same budget, at 39 and at
  8 dimensions. Until now the suite's strongest claim was a Pearson correlation
  bound, which pseudorandom noise satisfies — `math/rand` scores 0.124 against
  the 0.25 threshold, better than the real generator's 0.141 — so replacing the
  sequence with `rand.Float64()` would have left every test green. It no longer
  does: the substitution integrates 0.9x as well as Monte Carlo against a
  required 5x, while the generator itself achieves 18x.
- Benchmarks for `Next`, `NextInto`, `At`, `AtInto` and construction. The
  `just bench` recipe had no benchmarks to run.
- Runnable `Example` functions for the public API, so the pkg.go.dev page the
  README badges shows usage that the test suite compiles and checks.

### Changed

- Corrected stale comments in `.golangci.yml` that had been copied from a
  sibling DSP repository and described identifiers and a project layout that do
  not exist here.
- An index that overflows now panics instead of returning a point. See Fixed.

### Fixed

- `fill` computed the raw index as `skip + 1 + i` without checking for
  overflow. A wrapped index went negative, met the `index < 0` guard inside
  `radicalInverse`, and returned the all-zeros origin — the one point the
  package documents it never returns — while the scrambled path panicked, so
  the two disagreed. The addition is now checked and refused, matching how
  `scrambledRadicalInverse` already declines an index it cannot reverse.
- `readInt` in the WebAssembly demo converted a JavaScript number to `int`
  after checking only for NaN and infinity. Go leaves float-to-integer
  conversion implementation-defined when the value does not fit, so a value
  such as `1e300` produced a result that `clampInt` could not recognise as out
  of range — defeating the point of clamping in Go rather than trusting the
  page. The value is now saturated while it is still a `float64`.
- Four stale correlation figures in `scramble.go` and `correlation_test.go`.
  The worst adjacent-pair correlation is 0.81 at dimensions 34/35 unscrambled
  and 0.14 scrambled; the comments claimed 0.76 at 37/38, 0.117 and 0.65.
- The README described `WithScrambling` as applying an independent permutation
  to every digit position of every dimension. It draws one permutation per
  dimension and reuses it at every digit position of that dimension, which is
  what `halton.go` documents and what the code does.
- The README put scrambled memory at 5000 dimensions at "a few megabytes". It
  is roughly 475 MB; "a few megabytes" is the 500-dimension figure.

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
