# Toolchain and CI

What the repository's tooling does, where it is known to be weak, and which absences are
deliberate.

## What runs where

`justfile` is the local entry point; `.github/workflows/` is CI. `just check` runs
`check-formatted`, `check-tidy`, `lint` and `test`; `just ci` adds `go mod verify`.

Formatting is `treefmt` (`treefmt.toml`) dispatching gofumpt, gci, shfmt, prettier and
shellcheck by file type. Linting is golangci-lint against `.golangci.yml`, pinned to one
version across `justfile`, both workflows and `.trunk/trunk.yaml` — each site carries a
comment naming the other three, because those four drifting apart once made `just lint` and
CI lint differ by construction.

## The format check can pass without checking anything

This is the weakness worth knowing about first.

- `justfile` runs `treefmt --allow-missing-formatter`, which downgrades "formatter binary not
  found" to a warning, and `setup-deps` swallows a prettier install failure with `|| echo`.
  **If npm fails on the runner, every Markdown, JSON, YAML, JS, CSS and HTML file is skipped
  and the job still goes green.**
- `treefmt.toml` declares `shellcheck` for `*.sh`, but `setup-deps` never installs it, so
  `scripts/build-wasm-demo.sh` has never been shellchecked anywhere. Note that shellcheck
  never writes, so treefmt's change-detection contract does not apply to it either.

## Unpinned tools

`gofumpt`, `gci`, `shfmt` and `prettier` are all unpinned, so an upstream release can break
`check-formatted` with no change to this repository. golangci-lint is pinned; these are not.

## Trunk is dead configuration

`.git/info/exclude` hides `/.trunk`, and no file under it is tracked. It duplicates what
treefmt and golangci-lint already do, and it pins `go@1.21.0` against a module requiring 1.23.
Markdown and YAML linting exist _only_ there, which means CI lints neither.

Either track it and drop treefmt, or delete the directory. Keeping it untracked and half-wired
is the worst of the three states.

## The demo module has no quality gate

`.golangci.yml` excludes `examples/`, `check-tidy` only tidies the root module, and no job vets
the demo. That is roughly 1500 lines of Go and JavaScript shipping to GitHub Pages with nothing
checking it but the compiler. See [the WebAssembly demo](wasm-demo.md) for what that has cost.

## Smaller open items

- `just ci` calls itself the "full CI pipeline" but omits `test-race`, `check-wasm-demo` and
  the version matrix, and no workflow invokes it, so it can rot undetected.
- `wasm-demo-pages.yml` calls `scripts/build-wasm-demo.sh` directly while local users go
  through the justfile, and the justfile does not forward arguments, so the script's `OUT_DIR`
  parameter is unreachable through `just`. The two paths can drift.
- Pages builds with `go-version-file: go.mod`, so the published demo is compiled by the oldest
  supported toolchain rather than a current one.
- `setup-deps` hardcodes `linux_amd64` (broken on macOS and arm64) and pipes a tarball to
  `sudo tar` with no checksum.

## `scripts/build-wasm-demo.sh`

Quoting, `set -euo pipefail`, the nullglob handling and the GOROOT probe are all correct.
Open:

- The output directory is never cleaned, only `mkdir -p`'d, so a renamed or deleted asset
  ships to Pages indefinitely.
- `$1` is unvalidated. It cannot delete anything, but `./scripts/build-wasm-demo.sh ~`
  scatters `index.html`, `app.js`, `style.css` and `wasm_exec.js` into that directory,
  overwriting same-named files without confirmation.
- The asset glob is non-recursive and covers no images, icons, fonts or JSON, so a future
  `assets/` subdirectory silently ships nothing.
- No cache-busting. The pages load `app.js` and `qmc.wasm` by bare name, so a returning
  visitor can pair a new script with a cached `.wasm`. A content hash in the filename, or a
  `?v=<sha>` injected at build time, would fix it.

## Deliberate absences

Not worth adding for this repository, so that nobody adds them by reflex:

- `.nojekyll` — `upload-pages-artifact` plus `deploy-pages` does not run Jekyll, so it would
  be cargo cult here.
- `CODEOWNERS` — does nothing without branch protection.
- `SECURITY.md` — no dependencies, no network, no untrusted parsing.
- Issue and PR templates, `CODE_OF_CONDUCT.md`.
- `doc.go` — the package comment in `halton.go` already does that job.
- A `gomod` Dependabot updater — the module has no dependencies, by design. The
  `github-actions` updater exists because the workflows pin floating majors.

A short `CONTRIBUTING.md` is borderline, and worth three lines only because the tooling above
needs explaining.
