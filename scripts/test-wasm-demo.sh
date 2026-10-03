#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
TASK_DIR="$(mktemp -d)"
trap 'rm -rf "$TASK_DIR"' EXIT

# An optional existing build lets Pages validate the actual upload directory.
SITE_DIR="${qmc_browser_site:-}"
if [ -z "$SITE_DIR" ]; then
  SITE_DIR="$TASK_DIR/site"
  "$ROOT_DIR/scripts/build-wasm-demo.sh" "$SITE_DIR"
else
  SITE_DIR="$(cd "$SITE_DIR" && pwd)"
fi

GOOS=js GOARCH=wasm go test -C "$ROOT_DIR/examples/wasm-demo" -c \
  -tags=qmc_browser_fixture -o "$TASK_DIR/runtime-fixture.wasm" .
node "$ROOT_DIR/scripts/test-demo-browser.mjs" "$SITE_DIR" "$TASK_DIR/runtime-fixture.wasm"
