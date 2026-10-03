#!/usr/bin/env bash
# shellcheck source-path=SCRIPTDIR
set -euo pipefail
qmc_root="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=../tools/versions.sh
source "$qmc_root/tools/versions.sh"

require_tool() {
  if ! qmc_tool_matches "$1" "$2"; then
    printf 'error: %s %s is required; found %s. Run just setup-deps.\n' "$1" "$2" "$(qmc_tool_version "$1" || printf missing)" >&2
    exit 1
  fi
}

require_format_tools() {
  require_tool treefmt "$qmc_treefmt_version"
  require_tool gofumpt "$qmc_gofumpt_version"
  require_tool gci "$qmc_gci_version"
  require_tool shfmt "$qmc_shfmt_version"
  require_tool prettier "$qmc_prettier_version"
  require_tool shellcheck "$qmc_shellcheck_version"
}

case "${1:-all}" in
format) require_format_tools ;;
shell) require_tool shellcheck "$qmc_shellcheck_version" ;;
lint) require_tool golangci-lint "$qmc_golangci_version" ;;
all)
  require_format_tools
  require_tool golangci-lint "$qmc_golangci_version"
  ;;
*)
  echo 'usage: check-tools.sh [format|shell|lint|all]' >&2
  exit 2
  ;;
esac
