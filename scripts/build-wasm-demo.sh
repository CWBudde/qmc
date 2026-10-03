#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
if (($# > 1)); then
  echo 'usage: build-wasm-demo.sh [output-directory]' >&2
  exit 2
fi
exec python3 "$ROOT_DIR/scripts/build-wasm-demo.py" "${1:-$ROOT_DIR/dist}"
