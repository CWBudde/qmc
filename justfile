# QMC - Quasi-Monte Carlo sequences - Task Runner

# Default recipe to display available commands
default:
    @just --list

# Build the project
build:
    go build -v ./...

# Run tests with coverage
test:
    go test -v -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html

# Run tests with race detection
test-race:
    go test -v -race ./...

# Run benchmarks
bench:
    go test -bench=. -benchmem ./...

# Build the WebAssembly demo into ./dist
build-wasm-demo:
    ./scripts/build-wasm-demo.sh

# Build and serve the WebAssembly demo locally
run-wasm-demo: build-wasm-demo
    @echo "Serving the demo at http://localhost:8090"
    python3 -m http.server -d dist 8090

# Build the demo for js/wasm without emitting a binary (a fast compile check)
check-wasm-demo:
    cd examples/wasm-demo && GOOS=js GOARCH=wasm go build -o /dev/null . && go build -o /dev/null ./...

# Install the formatters and linters used by `just fmt` / `just lint`
setup-deps:
    #!/usr/bin/env bash
    set -euo pipefail
    export PATH=$HOME/go/bin:$PATH
    echo "Installing development dependencies..."

    # treefmt (formatter multiplexer)
    command -v treefmt >/dev/null 2>&1 || { echo "Installing treefmt..."; curl -fsSL https://github.com/numtide/treefmt/releases/download/v2.5.0/treefmt_2.5.0_linux_amd64.tar.gz | sudo tar -C /usr/local/bin -xz treefmt; }

    # golangci-lint v2 (linter + formatter runner)
    command -v golangci-lint >/dev/null 2>&1 || { echo "Installing golangci-lint..."; go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest; }

    # Go formatters
    command -v gofumpt >/dev/null 2>&1 || { echo "Installing gofumpt..."; go install mvdan.cc/gofumpt@latest; }
    command -v gci >/dev/null 2>&1 || { echo "Installing gci..."; go install github.com/daixiang0/gci@latest; }

    # Shell formatter
    command -v shfmt >/dev/null 2>&1 || { echo "Installing shfmt..."; go install mvdan.cc/sh/v3/cmd/shfmt@latest; }

    # Markdown/JSON/YAML plus the demo's JS/CSS/HTML formatter
    command -v prettier >/dev/null 2>&1 || { echo "Installing prettier..."; npm install -g prettier || echo "Prettier installation failed - npm not found."; }

# Format all files with treefmt
fmt:
    #!/usr/bin/env bash
    export PATH=$HOME/go/bin:$PATH
    treefmt --allow-missing-formatter

# Alias for `just fmt`
treefmt: fmt

# Run linter
lint:
    #!/usr/bin/env bash
    export PATH=$HOME/go/bin:$PATH
    golangci-lint run --config ./.golangci.yml --timeout 5m ./...

# Run linter (with fix)
lint-fix:
    #!/usr/bin/env bash
    export PATH=$HOME/go/bin:$PATH
    golangci-lint fmt --config ./.golangci.yml
    golangci-lint run --config ./.golangci.yml --timeout 5m --fix ./...

# Tidy up dependencies
tidy:
    go mod tidy

# Verify dependencies
verify:
    go mod verify

# Clean build artifacts
clean:
    go clean
    rm -f coverage.out coverage.html
    rm -f *.test *.prof
    rm -rf dist/

# Fail if any file is not formatted
check-formatted:
    #!/usr/bin/env bash
    export PATH=$HOME/go/bin:$PATH
    treefmt --allow-missing-formatter --fail-on-change

# Fail if go.mod/go.sum are not tidy
check-tidy:
    go mod tidy -diff

# Run all checks (format, lint, test)
check: check-formatted check-tidy lint test

# Full CI pipeline
ci: verify check

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
    go test -timeout 20m ./...

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
