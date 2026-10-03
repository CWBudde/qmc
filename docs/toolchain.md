# Toolchain and CI

The tracked setup and verification commands cover both Go modules. Contributor
instructions are in [CONTRIBUTING.md](../CONTRIBUTING.md); remediation status and
verification evidence are in [PLAN.md](../PLAN.md), TOOL-01 through TOOL-04 and
SHIP-01/02.

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

| Command                             | Purpose                                                                          |
| ----------------------------------- | -------------------------------------------------------------------------------- |
| `just setup-deps`                   | Pinned user-owned developer tools                                                |
| `just check`                        | Fast root and nested-module checks                                               |
| `just ci`                           | Routine checks, race contracts, real Chrome, tool-gate regressions               |
| `just test-statistical`             | Full ordinary statistical and contract suite, 10-minute test budget              |
| `just test-race-statistical`        | Explicit full statistical race audit, 40-minute test budget                      |
| `just check-wasm-demo`              | Nested-module tidy/verify, WASM build/vet, native stub                           |
| `just lint-wasm-demo`               | Production and fixture WASM lint                                                 |
| `just test-browser [site]`          | Real-browser verification with the publishing toolchain                          |
| `just build-wasm-demo [output]`     | Publishing build; defaults to `dist`, safely forwards paths                      |
| `just check-demo-artifact [output]` | Exact inventory, hashes, build identity, and static-reference checks             |
| `just test-demo-artifact`           | Offline publication, cleanup, destination safety, and failure regressions        |
| `just release-verify`               | Shared computational release gates with a 25-minute total budget                 |
| `just release-check VERSION SHA`    | Validate clean, documented source at the explicitly reviewed full SHA            |
| `just release VERSION SHA`          | Check and create an annotated local tag from reviewed, up-to-date main           |
| `just test-release-gates`           | Offline policy, literal-argument, source-drift, workflow and timeout regressions |

Every required configuration is tracked. Optional ignored local tool/editor
state is not part of setup and does not affect these commands. Pages builds use
`just build-wasm-demo dist`, then test that exact artifact before upload.

## Workflows and releases

Every external action is pinned to a full upstream commit SHA with a same-line
major-version comment. Dependabot proposes weekly reviewed updates. The pins
were resolved from official repository tag refs on 2026-10-03; review covered
runtime/input metadata, Go's version-file parser, and the Pages upload dependency
chain. GitHub's [action security guidance](https://docs.github.com/en/actions/reference/security/secure-use)
describes these pin and permission practices.

| Action                  | Reviewed commit                            | Version family |
| ----------------------- | ------------------------------------------ | -------------- |
| actions/checkout        | `11d5960a326750d5838078e36cf38b85af677262` | v4             |
| actions/setup-go        | `40f1582b2485089dde7abd97c1529aa768e1baff` | v5             |
| actions/setup-node      | `49933ea5288caeca8642d1e84afbd3f7d6820020` | v4             |
| actions/cache           | `0057852bfaa89a56745cba8c7296529d2fc39830` | v4             |
| actions/upload-artifact | `ea165f8d65b6e75b540449e92b4886f43607fa02` | v4             |
| extractions/setup-just  | `dd310ad5a97d8e7b41793f8ef055398d51ad4de6` | v2             |
| actions/configure-pages | `983d7736d9b0ae728b81ab479565c72886d7745b` | v5             |
| actions/deploy-pages    | `d6db90164ac5ed86f2b6aed7e0febac5b3c0c03e` | v4             |

Workflow defaults grant `contents: read`. Pages write/OIDC permissions belong
only to the deploy job, which waits for the verified build. Checkouts disable
persisted credentials, all jobs have deadlines, and CI bootstraps Just 1.21.0,
the version exercised locally. The Pages composite uploader's nested floating
action is replaced by verified archive creation and the directly pinned generic
uploader. `package-demo.py` creates only regular-file entries, checks every
archived byte against the site manifest, includes notices, refuses existing
archive destinations, and enforces the Pages size limit. This follows the
[Pages artifact format](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages).

Before a release, commit and review the changes, add exactly one changelog
section for the version with actual change entries, and record the full reviewed
commit SHA. That SHA is the maintainer's review attestation: checks enforce its
identity and record it in the tag. Pull-request approvals remain part of the
maintainer's repository review process.

```bash
reviewed_commit="FULL_40_CHARACTER_SHA_YOU_REVIEWED"
just release-check 0.4.0 "$reviewed_commit"
just release 0.4.0 "$reviewed_commit"
```

Both commands use the same [SemVer 2.0 rules](https://semver.org/spec/v2.0.0.html)
as the workflow: optional leading `v` and legacy `version=` are normalized;
leading-zero core/numeric prerelease identifiers and empty identifiers are
rejected. Build metadata is accepted. The current Go module path permits release
majors 0 and 1; higher majors require semantic import versioning. Version input
and SHA arguments are passed as literal environment/argument data.

`release-check` requires HEAD to match the reviewed SHA, an entirely clean
worktree including untracked files, complete metadata and an exact documented
version section. Any existing version tag must identify that same commit. It
runs `release-verify`, then repeats source/metadata checks.
`release-verify` selects the publishing compiler, disables external Go workspaces
and caller GOFLAGS, and runs shared `just ci` plus the full `test-statistical`
suite. Thus both modules, strict formatting/lint, ordinary race contracts,
WASM build/vet, real-browser artifact/notices and every failure regression are
required. The portable runner bounds the total to 25 minutes and terminates
the process group on failure/timeout; Go race/statistical commands retain their
5-/10-minute limits and browser checks retain their two-minute deadline. The
release workflow has a 30-minute job budget including tool setup. The optional
40-minute full statistical race audit remains a separate scheduled/manual job.

Local tag creation additionally requires branch `main`, an absent version tag,
and fetched remote main matching HEAD before and after verification. The
annotated tag contains `Reviewed-Commit: <full SHA>`. Publication remains an
explicit subsequent Git command. The release workflow checks out its exact event
revision with full history; tagged runs require that annotation, matching
checkout/tag SHA and main ancestry. Manual candidate validation requires the
reviewed SHA input and binds it to the event checkout. `release-verify` is also
available for development branches without declaring a release version.

`test-release-gates` uses private offline Git repositories, fake expensive gates,
and real command/metadata boundaries to test injection, dirty/untracked source,
HEAD/remote changes, wrong or duplicate changelog sections, gate failures,
annotated-tag identity and total-deadline descendant cleanup. Its workflow
checker covers the repository's formatted layout and rejects floating actions,
broader permissions, persisted checkout credentials and missing budgets; required
Prettier checks additionally parse YAML syntax. Archive regressions round-trip
the site through a tar and revalidate it, including every notice and absence of
symlink/hard-link entries. These fixtures complement actual release-verify runs.

## Demo artifact publication

`scripts/build-wasm-demo.sh` resolves the repository location and invokes the
stdlib Python builder. The public recipe selects `tools/go-version`; direct
script invocation uses the caller's Go compiler. Python 3 and a local Linux or
macOS filesystem supporting directory exchange are required for replacing a
nonempty existing build. Fresh-directory publication uses rename.

The payload contains top-level `.html`, `.css`, `.js`, `.mjs`, and `.svg` demo
files. The optional `assets/` tree is recursive and additionally accepts PNG,
JPEG, GIF, WebP, AVIF, ICO, JSON, WOFF/WOFF2, TTF/OTF, TXT, and PDF. Unrecognized
types and symlinks fail the build rather than silently copying source/private
files. The Go compiler produces `qmc.wasm`; `wasm_exec.js` comes from that same
compiler's GOROOT. Compiler identity is checked before and after compilation.
The project MIT license, Joe–Kuo notice, and compiler-matched Go LICENSE/PATENTS
are copied byte-for-byte into `notices/`. Both interactive pages link the static
credits page, which links all four complete notices. No font files are bundled.
Additional third-party assets should include their applicable notices under
`assets/notices/` and credits links.

All payload files occupy one `build-<sha256>/` namespace. The ID hashes the
logical filenames, each file's SHA-256, and the compiler version. Public HTML
aliases insert a relative base URL selecting that namespace and retain stable
page navigation. Thus worker startup, dynamic WASM fetches, CSS assets, and page
scripts use the same build, including on project subpaths. Hosts can revalidate
public HTML and use immutable caching for the versioned directories. A stale
page whose bundle was removed needs Reload; it cannot fetch new-build bytes
under the old URLs. The Chrome test exercises that deployment transition.

`build-manifest.json` records exact file inventory and hashes. The standalone
checker verifies them, the reconstructed entry pages, WASM header, and literal
local references in HTML/SVG, CSS and JavaScript. It checks artifact integrity;
real-browser tests additionally validate runtime behavior. Dynamic resource
references added in future code need corresponding browser checks.
Distribution verification also requires every notice to be nonempty and
reachable through credits from both pages. Ownership verification accepts an
intact preceding managed format so it can be replaced safely even if its notice
set predates this gate; it still checks exact inventories and hashes.

Destinations must be user-owned, nonsymlink directories that are new, empty,
or an intact managed site. Existing unrelated or edited files/directories are
preserved and cause failure. The repository root/ancestors, home, filesystem
root, demo sources, private state and third-party sources are protected. Paths
with spaces and invocation outside the repository are supported. Move older
unmanifested builds aside rather than expecting automatic adoption.

A persistent hidden `.qmc-demo-<destination-hash>.lock` beside the output
serializes cooperating builders, with a 30-second lock deadline. Each Go command
has a 180-second budget. Builds use fresh sibling staging directories and fully
validate them before publication. Linux `renameat2(RENAME_EXCHANGE)` or macOS
`renamex_np(RENAME_SWAP)` replaces a nonempty site atomically; unsupported
filesystems/platforms fail while preserving the old output. This host verified
Linux exchange; macOS behavior has not been executed natively.

Ordinary failed staging directories are cleaned. Caller edits made during
compilation are rejected before publication. The old tree is checked again
after exchange before cleanup; a racing edit causes that tree to be retained
at the reported staging path, while the new complete build remains published.
The manifest is an integrity/ownership convention, not authentication against
an actor who can rewrite the manifest itself. It does not provide a filesystem
transaction against uncooperative concurrent writers.

## Deliberate absences

The current workflow does not require these files:

- `.nojekyll` — the archive/upload/deploy workflow does not run Jekyll, so it would
  change the current build.
- `CODEOWNERS` — reviewers are currently selected through the maintainer's
  review process. Required owner approvals would also need branch protection.
- `SECURITY.md` — the library has no runtime dependencies or network activity.
  The demo downloads its own static/WASM assets, uses system fonts, and performs
  computation locally without analytics, submissions, or third-party requests.
- Issue and PR templates, `CODE_OF_CONDUCT.md`.
- `doc.go` — the package comment in `sequence.go` already provides package
  documentation.
- A `gomod` Dependabot updater — the module has no dependencies, by design. The
  `github-actions` updater proposes reviewed changes to the workflow SHA pins.

These are documented choices, not missing remediation tasks. Reconsider them
when the project's contributor, security-reporting, or hosting needs change.
