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
