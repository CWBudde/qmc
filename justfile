# QMC - Quasi-Monte Carlo sequences - Task Runner

# Default recipe to display available commands
default:
    @just --list

# Build the project
build:
    go build -v ./...

# Full ordinary suite with coverage (includes statistical sweeps)
test:
    go test -count=1 -timeout=10m -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html

# Routine contracts and the small smooth-product PR quality gate
test-fast:
    go test -short -count=1 -timeout=3m ./...

# Routine contracts with race detection; expensive statistical sweeps skip
test-race:
    go test -short -race -count=1 -timeout=5m ./...

# Full ordinary statistical validation, including all contract tests
test-statistical:
    go test -count=1 -timeout=10m ./...

# Optional explicit full-suite race audit, including expensive sweeps
test-race-statistical:
    go test -race -count=1 -timeout=40m ./...

# Run benchmarks
bench:
    go test -run '^$' -bench=. -benchmem ./...

# Build with the publishing toolchain; optional output is forwarded safely
build-wasm-demo $qmc_demo_output="dist":
    #!/usr/bin/env bash
    set -euo pipefail
    source ./tools/versions.sh
    GOTOOLCHAIN="go$qmc_development_go_version" bash ./scripts/build-wasm-demo.sh "$qmc_demo_output"

# Build and serve the WebAssembly demo locally
run-wasm-demo: build-wasm-demo
    @echo "Serving the demo at http://localhost:8090"
    python3 -m http.server -d dist 8090

# Compile/vet the nested module with the caller's compatibility toolchain
check-wasm-demo:
    go -C examples/wasm-demo mod verify
    go -C examples/wasm-demo mod tidy -diff
    GOOS=js GOARCH=wasm go -C examples/wasm-demo build -o /dev/null .
    GOOS=js GOARCH=wasm go -C examples/wasm-demo vet -tags=qmc_browser_fixture ./...
    go -C examples/wasm-demo build -o /dev/null ./...

# Production WASM code and test fixture use the same lint rules as the library
lint-wasm-demo:
    #!/usr/bin/env bash
    set -euo pipefail
    source ./tools/versions.sh
    bash ./scripts/check-tools.sh lint
    cd examples/wasm-demo
    GOOS=js GOARCH=wasm golangci-lint run --config ../../.golangci.yml --build-tags qmc_browser_fixture --timeout 5m ./...

# Bounded real-Chrome checks; optional site argument validates an existing build
test-browser $qmc_browser_site="":
    #!/usr/bin/env bash
    set -euo pipefail
    source ./tools/versions.sh
    GOTOOLCHAIN="go$qmc_development_go_version" bash ./scripts/test-wasm-demo.sh

# Install pinned development tools into a user-owned directory
setup-deps:
    bash ./scripts/setup-deps.sh

# Offline installer contract checks; no host installation is changed
test-tool-setup:
    python3 ./scripts/test-tool-setup.py

# Format source files with required pinned tools and shell diagnostics
fmt:
    bash ./scripts/format.sh fmt

# Alias for `just fmt`
treefmt: fmt

# Run linter
lint:
    #!/usr/bin/env bash
    source ./tools/versions.sh
    bash ./scripts/check-tools.sh lint
    golangci-lint run --config ./.golangci.yml --timeout 5m ./...

# Run linter (with fix)
lint-fix:
    #!/usr/bin/env bash
    source ./tools/versions.sh
    bash ./scripts/check-tools.sh lint
    golangci-lint fmt --config ./.golangci.yml
    golangci-lint run --config ./.golangci.yml --timeout 5m --fix ./...

# Tidy up dependencies
tidy:
    go mod tidy
    go -C examples/wasm-demo mod tidy

# Verify dependencies
verify:
    go mod verify
    go -C examples/wasm-demo mod verify

# Clean build artifacts
clean:
    go clean
    rm -f coverage.out coverage.html
    rm -f *.test *.prof
    rm -rf dist/

# Required formatting gate: missing/wrong tools and ShellCheck failures fail
check-formatted:
    bash ./scripts/format.sh check

# Explicit non-writing shell diagnostic gate
check-shell:
    bash ./scripts/check-shell.sh

# Exercise required formatter failures in isolated temporary worktrees
test-formatting:
    #!/usr/bin/env bash
    set -euo pipefail
    source ./tools/versions.sh
    bash ./scripts/check-tools.sh format
    node ./scripts/test-formatting.mjs

# Fail if go.mod/go.sum are not tidy
check-tidy:
    go mod tidy
    go -C examples/wasm-demo mod tidy -diff
    go -C examples/wasm-demo mod tidy -diff

# Local fast checks for both modules; statistics and real-browser tests are explicit
check: verify check-formatted check-tidy lint lint-wasm-demo check-wasm-demo test-fast

# Routine CI contract, including race and real-browser verification
ci: verify check-formatted check-tidy lint lint-wasm-demo check-wasm-demo test-race test-browser test-tool-setup test-formatting

# Validate a prospective release without creating a tag
release-check version:
    #!/usr/bin/env bash
    set -euo pipefail
    release_version="{{version}}"
    release_version="${release_version#version=}"
    if [[ ! "$release_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
        echo "Invalid semantic version: $release_version" >&2
        exit 1
    fi
    grep -Fq "## [$release_version]" CHANGELOG.md
    test -s LICENSE
    test -s README.md
    test "$(go list -m)" = "github.com/cwbudde/qmc"
    just verify
    just check-formatted
    just check-tidy
    just lint
    go vet ./...
    just test-race
    just test-statistical

# Validate and create an annotated release tag locally
release version:
    #!/usr/bin/env bash
    set -euo pipefail
    release_version="{{version}}"
    release_version="${release_version#version=}"
    release_tag="v$release_version"
    just release-check "$release_version"
    if [[ -n "$(git status --porcelain)" ]]; then
        echo "Release requires a clean worktree" >&2
        exit 1
    fi
    if git rev-parse --verify --quiet "refs/tags/$release_tag" >/dev/null; then
        echo "Tag already exists: $release_tag" >&2
        exit 1
    fi
    git tag -a "$release_tag" -m "Release $release_tag"
    echo "Ready to push: git push origin main $release_tag"
