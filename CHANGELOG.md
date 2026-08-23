# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- The Sobol sequence, `NewSobol`, with Joe and Kuo direction numbers for 1024
  dimensions embedded from `third_party/joe-kuo` under their BSD-3 notice, and
  `WithDirectionNumbers` for a caller's own table beyond that. Direction
  numbers cannot be derived — the initial values come from those authors'
  numerical search — so embedding a table or shipping a markedly worse sequence
  were the only honest options. `At` evaluates directly and is safe for
  concurrent use; `Next` uses the Gray-code recurrence and runs at 65 ns/op
  against 471 for Halton's `NextInto` at 39 dimensions. Requiring both forces
  Gray-code ordering on the sequence itself, so dimension 1 goes 0.5, 0.75,
  0.25 — matching Joe and Kuo's own `sobol.cc` and scipy rather than the
  ordering a reader might expect, and documented on the type.
- `Sequence`, the interface both generators satisfy. It carries `Dims`, the
  stateful cursor and the stateless index-addressed form, and deliberately not
  `Bases` or `Permutation`: those are Halton-specific, and the honest answer to
  a question that does not apply to Sobol is that the caller is holding the
  wrong type, not a zero value. Settled before the second sequence landed,
  because adding an interface afterwards to a package with two concrete types
  is a breaking change.
- `WithDigitalShift` and `WithOwenScrambling` randomize Sobol;
  `WithNestedScrambling` randomizes Halton. Measured against the same Monte
  Carlo baseline on a smooth 39-dimensional product integrand at n=4096 over
  ten streams: MC 4.3e-03, random-digit Halton 2.4e-04 (17.7x), shifted Sobol
  1.5e-04 (29.5x), Owen-scrambled Sobol 1.4e-04 (32.0x), nested Halton 1.4e-04
  (31.9x). Ten streams is few enough to reorder the middle of that list; over
  forty, random-digit Halton reads 24.4x and nested Halton 41.1x.
- `WithNestedScrambling` draws a genuine uniform permutation of the digit
  alphabet at every node of the scramble tree. It began as an affine
  construction — `x → ax+b mod p`, free of shuffles — which integrated better
  (49.9x over forty streams against 41.1x) but left a correlation tail the full
  permutation does not: worst adjacent-pair |r| over thirty seeds 0.37 against
  0.14, where random-digit scrambling scores 0.16. The affine map turns a
  large-base coordinate's only varying digit into a ramp of another slope
  rather than scattering it, and two dimensions drawing commensurate slopes
  ramp together again. The uniform draw costs about 40x per point against
  random-digit scrambling; it suits integration, not a parameter sweep.
- Owen scrambling is nearly free on `AtInto` (370 ns/op against 360 for a
  digital shift) but roughly 3x on `NextInto` (197 against 65), because the
  Gray-code recurrence is precisely what cannot carry a non-linear scramble.
- A randomization that does not apply to a generator is now refused by name at
  construction rather than ignored. Ignoring it would hand back a deterministic
  sequence to code that is about to average over seeds, which would report an
  error estimate of exactly zero.
- `Sobol`'s documentation now says where its balance property holds: the
  (t,m,s)-net property covers a 2^m-aligned block of raw indices, not any 2^m
  consecutive points. At 40 dimensions and m=8 the default skip of 0 leaves all
  40 dimensions unbalanced, which is what a caller running their own
  stratification check sees before concluding the sequence is broken. The
  package's own tests used `WithSkip(2^m - 1)` for this reason and said so
  nowhere. Documented alongside it: Joe and Kuo's D(6) criterion optimises
  two-dimensional projections without making them all nets — of the 780 pairs
  among the first 40 dimensions, 18 are balanced at every split at m=8 and 4 at
  m=10 — so dimensions 12 and 23 leave 224 of 256 cells empty where 0 and 1
  fill every one.
- `WithDirectionNumbers` documents the Joe-Kuo file format and every invariant
  the validator enforces, and says what it cannot prove: that the numbers came
  from the authors' search. The path above the embedded 1024-dimension ceiling
  is now tested rather than assumed, with a synthesised 1200-dimension table
  whose polynomials are found by running the primitivity check rather than
  asserted.
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
- `owen_uniformity_test.go` measures what the hash in `owenScramble` costs
  against a textbook Owen scramble that stores one independent fair bit per
  node. Node by node there is nothing to find: over 40000 seeds and all 8191
  nodes to depth 12 the worst node sits 3.85 sigma from fair, where the largest
  of 8191 fair coins is expected near 3.9. Jointly across a level there is —
  the hash's flip-count variance runs from 0.09 to 3.06 of the binomial value
  the true construction reproduces to within 4% — and a level-wide effect
  spreads as ~1/2^k per pair, which is why no per-node statistic sees it. It
  costs at most a tenth of the RMS integration error, so the constants are
  unchanged; this is simply the first instrument in the package able to
  evaluate a change to them.
- Benchmarks for `Next`, `NextInto`, `At`, `AtInto` and construction. The
  `just bench` recipe had no benchmarks to run.
- Runnable `Example` functions for the public API, so the pkg.go.dev page the
  README badges shows usage that the test suite compiles and checks.

### Changed

- The package doc comment moved from `halton.go` to `sequence.go` and now
  covers both generators, with guidance on which to reach for.
- The correlation figures in the README are now quoted as a median and a tail
  over thirty seeds rather than a worst case over five. The statistic is
  high-variance: a change that was a pure re-instantiation of the scrambling,
  not a change of scheme, moved a five-seed worst case from 0.40 to 0.12.
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
