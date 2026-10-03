# Contributing

Use Go 1.23 or newer for library development. The root module has no runtime
dependencies; `examples/wasm-demo` is a separate module replacing the root
locally. Routine compatibility CI runs amd64 and executable 386 with Go 1.23,
1.24, 1.25, and 1.26.1.

## Setup and everyday checks

Install Just (CI uses 1.21.0), Bash, Go, Python 3, Node/npm (Node 18 or newer),
curl, and tar. Run from the repository root:

```bash
just setup-deps
just fmt
just check
```

Setup installs exact versions from `tools/versions.sh` into a user-owned
directory. `QMC_TOOLS_DIR` selects an absolute dedicated alternative.
Source-built tools and publishing use `tools/go-version` (Go 1.26.1); Go may
download that toolchain. This does not raise the library's Go 1.23 minimum.
All required configuration is tracked; ignored editor or Trunk state is
optional. `just check` verifies formatting, both modules, tidy diffs, lint,
WASM build/vet, the native demo stub, and routine library contracts. A failed
format check can leave corrections in the worktree; inspect the diff.

## Test the behavior you changed

```bash
just test-fast                # routine contracts; 3-minute test budget
just test-race                # same contracts under race detector; 5 minutes
just test-statistical         # full ordinary/statistical suite; 10 minutes
just test                    # full suite plus coverage artifacts
just test-race-statistical    # optional full statistical race audit; 40 minutes
```

For integer-boundary changes, also run the routine suite as executable 386:

```bash
CGO_ENABLED=0 GOARCH=386 just test-fast
GOTOOLCHAIN=go1.23.0 just test-fast
GOTOOLCHAIN=go1.23.0 just check-wasm-demo
```

Keep deterministic outputs stable unless fixing a documented correctness
problem. Add focused regressions for observable failures and use independent
mathematical references when practical. Sampling-quality changes need the full
statistical suite. Report finite seeded measurements with their configuration
and uncertainty assumptions; do not infer universal convergence or unbiasedness.
See [testing methodology](docs/testing-methodology.md).

## Browser and distribution changes

Real-browser checks require Chrome/Chromium. The runner defaults to
`google-chrome`; set `CHROME_BIN` to another browser executable path if necessary.
Local server binding must be permitted.

```bash
just test-browser
just build-wasm-demo dist
just check-demo-artifact dist
just test-browser dist
just test-demo-artifact
just run-wasm-demo
```

The browser recipe checks both production pages and a test-only Go runtime
fixture. It requires Python 3 and Node, uses the publishing compiler, and owns
temporary builds, server and browser lifetimes. Existing-artifact checks verify
the exact directory that would be uploaded. `just run-wasm-demo` serves `dist`
on port 8090 until stopped. Build destinations must be new, empty, or an intact
managed artifact; move unrelated or edited output aside rather than deleting it.
Full notices and cache-consistent bundles are part of the distribution contract.
Manual keyboard inspection is documented in the
[demo README](examples/wasm-demo/README.md).

## Benchmarks and design decisions

```bash
just bench
taskset -c 2 just measure-performance /tmp/qmc-performance-new-run
```

The controlled recipe requires a new output directory, records source/environment
metadata, pins the compiler, uses one Go execution thread, and runs repeated
throughput/construction/integration experiments plus a separate profile.
Choose a permitted CPU when using Linux `taskset`; omit it on other platforms
and record scheduling limitations. Run benchmarks separately from other tests
and benchmarks. Review allocations, accuracy, and compatibility alongside speed.
[Performance](docs/performance.md) contains the current canonical evidence.

## CI and release validation

```bash
just ci
just release-verify
just test-release-gates
```

`ci` combines routine race checks, both modules, formatting/lint, real Chrome,
and offline tooling/artifact/release regressions. `release-verify` additionally
runs full ordinary statistics under a 25-minute total deadline, without creating
a tag or publishing. The optional full statistical race audit remains separate.

For an actual release, commit and review the source, add one exact version
section with real entries to `CHANGELOG.md`, and record the full reviewed SHA:

```bash
just release-check 0.4.0 FULL_REVIEWED_40_CHARACTER_SHA
just release 0.4.0 FULL_REVIEWED_40_CHARACTER_SHA
```

The example version is a placeholder, not the next assigned release.
Both commands require clean source matching that SHA and repeat checks after
verification. Tag creation additionally requires current `main` matching fetched
remote main and an absent version tag; it creates an annotated local tag with
the review attestation. Pushing/publishing is a separate maintainer action.
See [toolchain and CI](docs/toolchain.md) for precise policy and budgets.

Record review remediation in [PLAN.md](PLAN.md), including acceptance evidence,
and update the affected topic docs and changelog with behavior changes.
