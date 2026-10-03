#!/usr/bin/env bash
# shellcheck source-path=SCRIPTDIR
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=../tools/versions.sh
source "$ROOT_DIR/tools/versions.sh"
export GOTOOLCHAIN="go$qmc_development_go_version"
export GOWORK=off
export GOFLAGS=""
cd "$ROOT_DIR"

# Routine CI covers both modules, race contracts, WASM build/vet, the exact
# browser artifact/notices, and the tool/module/artifact failure regressions.
exec python3 scripts/run_release_gates.py
