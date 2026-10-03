# Toolchain and CI

What the repository's tooling does, where it is known to be weak, and which absences are
deliberate.

## What runs where

`justfile` is the local entry point; `.github/workflows/` is CI. `just check` runs
`check-formatted`, `check-tidy`, `lint` and `test`; `just ci` adds `go mod verify`.

Formatting is `treefmt` (`treefmt.toml`) dispatching gofumpt, gci, shfmt, prettier and
shellcheck by file type. Development versions are tracked in `tools/versions.sh`.
Linting uses golangci-lint against `.golangci.yml`; workflow linter versions must
match that source. Strict format enforcement remains tracked as TOOL-01 in
[PLAN.md](../PLAN.md).

## The format check can pass without checking anything

This is the weakness worth knowing about first.

- `justfile` runs `treefmt --allow-missing-formatter`, which downgrades "formatter binary not
  found" to a warning, and `setup-deps` swallows a prettier install failure with `|| echo`.
  **If npm fails on the runner, every Markdown, JSON, YAML, JS, CSS and HTML file is skipped
  and the job still goes green.**
- `treefmt.toml` declares `shellcheck` for `*.sh`, but `setup-deps` never installs it, so
  `scripts/build-wasm-demo.sh` has never been shellchecked anywhere. Note that shellcheck
  never writes, so treefmt's change-detection contract does not apply to it either.

## Pinned development-tool installation

Run `just setup-deps`. Prerequisites are Bash, Go 1.23 or newer, Node/npm,
Python 3, curl and tar; Node 18 or newer also supports the browser runner.
The installer selects official archives for Linux/macOS on amd64/arm64,
checks SHA-256 against `tools/archive-checksums.sha256` before extraction,
and verifies the resulting tool version. Other platforms fail explicitly.
Go tools use exact module versions and the public Go checksum database;
Prettier uses an exact version plus npm's integrity-locked `tools/package-lock.json`,
with lifecycle scripts disabled. Installation errors stop setup.

`tools/versions.sh` is the version source for local setup/checks: treefmt 2.5.0,
gofumpt 0.10.0, gci 0.14.0, shfmt 3.12.0, Prettier 3.5.3, ShellCheck 0.11.0,
and golangci-lint 2.13.1. Source-built tools use Go 1.26.1 through `GOTOOLCHAIN`;
the Go command downloads that exact toolchain when necessary. This development
toolchain is separate from the library's Go 1.23 compatibility requirement.
Archive pins come from the official
[treefmt release](https://github.com/numtide/treefmt/releases/tag/v2.5.0) and
[ShellCheck release](https://github.com/koalaman/shellcheck/releases/tag/v0.11.0).
Update the version source, archive checksums, npm lock, and workflow lint pin
together when intentionally upgrading tools.

A matching version already on PATH is reused; a missing or mismatched version
is installed under `${XDG_DATA_HOME:-$HOME/.local/share}/qmc-tools/bin` without
sudo. Set `QMC_TOOLS_DIR` to an absolute, dedicated user-owned directory to
choose another location. Symlinked installation directories and relative paths
are rejected. Just recipes source the tracked tool environment automatically.
For direct tool commands, prepend that installation directory's `bin` to PATH.
An unrelated tool already on PATH is not overwritten.

`just test-tool-setup` runs offline fixtures with real archive extraction and
hash checking, mocked platform/download/build endpoints, and temporary installation
directories. It checks all four platform selections, version replacement and reuse,
checksum refusal before extraction, wrong compiled versions, npm failures,
unsupported platforms, and invalid destinations. CI runs it after setup. Linux
amd64 installation is also exercised with real upstream downloads; the fixtures
verify other platform routing without claiming native execution on macOS/arm64.

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
- `SECURITY.md` — the library has no runtime dependencies or network activity.
  The demo downloads its own static/WASM assets, uses system fonts, and performs
  computation locally without analytics, submissions, or third-party requests.
- Issue and PR templates, `CODE_OF_CONDUCT.md`.
- `doc.go` — the package comment in `halton.go` already does that job.
- A `gomod` Dependabot updater — the module has no dependencies, by design. The
  `github-actions` updater exists because the workflows pin floating majors.

A short `CONTRIBUTING.md` is borderline, and worth three lines only because the tooling above
needs explaining.
