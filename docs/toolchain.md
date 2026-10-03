# Toolchain and CI

What the repository's tooling does, where it is known to be weak, and which absences are
deliberate.

## What runs where

`justfile` is the local entry point; workflows invoke the same recipes.
`just check` runs both modules' verification, strict formatting, tidy diffs, lint,
WASM compile/vet, and fast library tests. `just ci` uses routine race tests and
adds real-browser verification plus installer/formatter failure regressions.
It excludes the full statistical suite, which has separate bounded commands.

Formatting is `treefmt` (`treefmt.toml`) dispatching gofumpt, gci, shfmt, prettier and
shellcheck by file type. Development versions are tracked in `tools/versions.sh`.
Linting uses golangci-lint against `.golangci.yml`; workflow linter versions must
match that source.

## Required formatting and shell diagnostics

`just check-formatted` requires the exact formatter versions from
`tools/versions.sh`, runs ShellCheck as an explicit diagnostic gate, then
runs every configured treefmt formatter without cache or missing-tool leniency.
Missing, unusable, and wrong-version tools fail with setup instructions. Local
`TREEFMT_*` overrides are cleared so excludes or formatter selections cannot
silently weaken the required check. `just fmt` uses the same tools and shell
gate before writing formatting changes; `just check-shell` runs only shell
diagnostics. Like treefmt's normal check mode, a failed formatting check can
leave corrections in the worktree for inspection.

Go uses gofumpt and gci; Markdown, JSON, YAML, JavaScript (including `.mjs`),
CSS and HTML use pinned Prettier; shell uses shfmt plus diagnostic-only ShellCheck.
Both tracked files and non-ignored new source files are included through Git's
walker. The generated changelog, module files owned by Go, built assets, dependency
folders and private local state have explicit exclusions in `treefmt.toml`.
TOML and Python are not configured formatter targets; adding them requires a
pinned tool and failure regression rather than a missing-tool exception.

`just test-formatting` runs actual tools in isolated temporary Git worktrees.
It checks invalid formatting/parser input in nine extensions, a ShellCheck-only
undefined-variable diagnostic, six unusable executables, a near-matching wrong
version, and treefmt overrides attempting to skip files. Correct fixtures must
pass before and after failures. CI runs this gate after pinned setup; no fixture
mutates the developer's real executables or repository files.

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
and golangci-lint 2.13.1. Source-built tools use `tools/go-version` (Go 1.26.1) through `GOTOOLCHAIN`;
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

## Modules, compatibility, and publishing

`check-tidy`, `tidy` and `verify` cover both the root and `examples/wasm-demo`
modules. `check-wasm-demo` verifies/tidies the demo, compiles production js/wasm,
vets js/wasm with the real runtime fixture tag, and also compiles the native stub.
`lint-wasm-demo` runs the same lint rules on its production code and fixture.
There is no blanket examples exclusion; the only dependency-path exclusion is
`node_modules`. The stub is a convenience build target, not WASM behavior evidence.
`test-browser` checks production/runtime fixtures in actual Chrome.

The PR matrix executes routine root tests on amd64 and 386 for Go 1.23, 1.24,
1.25 and 1.26.1 and compiles/vets the demo for each version. The 386 leg uses
`CGO_ENABLED=0` and runs binaries, so it tests real 32-bit arithmetic. Race tests
run on amd64; js/wasm and wasip1/wasm have a 64-bit int and do not replace 386.

`tools/go-version` pins Go 1.26.1 for source-built developer tools, published
WASM, real-browser checks, statistical jobs, and release validation. Setup/PR
format/lint/browser/Pages jobs use that same file. Compatibility recipes keep
the caller's Go version: for example `GOTOOLCHAIN=go1.23.0 just check-wasm-demo`
and `GOTOOLCHAIN=go1.23.0 CGO_ENABLED=0 GOARCH=386 just test-fast`.
Publishing deliberately uses a separate toolchain rather than changing the
library's Go 1.23 requirement.

| Command                         | Purpose                                                                        |
| ------------------------------- | ------------------------------------------------------------------------------ |
| `just setup-deps`               | Pinned user-owned developer tools                                              |
| `just check`                    | Fast root and nested-module checks                                             |
| `just ci`                       | Routine checks, race contracts, real Chrome, tool-gate regressions             |
| `just test-statistical`         | Full ordinary statistical and contract suite, 10-minute test budget            |
| `just test-race-statistical`    | Explicit full statistical race audit, 40-minute test budget                    |
| `just check-wasm-demo`          | Nested-module tidy/verify, WASM build/vet, native stub                         |
| `just lint-wasm-demo`           | Production and fixture WASM lint                                               |
| `just test-browser [site]`      | Real-browser verification with the publishing toolchain                        |
| `just build-wasm-demo [output]` | Publishing build; defaults to `dist`, safely forwards paths                    |
| `just release-check VERSION`    | Prospective release checks; full artifact/release alignment tracked as TOOL-04 |

Every required configuration is tracked. Optional ignored local tool/editor
state is not part of setup and does not affect these commands. Pages builds use
`just build-wasm-demo dist`, then test that exact artifact before upload.
Release policy, action SHA pins, clean artifact publication, and notices remain
TOOL-04/SHIP-01/SHIP-02 in [PLAN.md](../PLAN.md).

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
