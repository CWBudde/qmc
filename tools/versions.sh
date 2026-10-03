#!/usr/bin/env bash
# Pinned development tools; exported for scripts sourcing this file.
export qmc_treefmt_version=2.5.0
export qmc_gofumpt_version=0.10.0
export qmc_gci_version=0.14.0
export qmc_shfmt_version=3.12.0
export qmc_prettier_version=3.5.3
export qmc_shellcheck_version=0.11.0
export qmc_golangci_version=2.13.1
export qmc_development_go_version=1.26.1

qmc_tools_dir="${QMC_TOOLS_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/qmc-tools}"
export PATH="$qmc_tools_dir/bin:$HOME/go/bin:$PATH"

# Normalize the version output without accepting prefixes such as 3.5.30.
qmc_tool_version() {
  local output
  output=$("$1" --version 2>/dev/null) || return 1
  case "${1##*/}" in
  treefmt) printf '%s\n' "${output#treefmt v}" ;;
  gofumpt)
    output=${output%% *}
    printf '%s\n' "${output#v}"
    ;;
  gci) printf '%s\n' "${output#gci version }" ;;
  shfmt) printf '%s\n' "${output#v}" ;;
  prettier) printf '%s\n' "$output" ;;
  shellcheck) printf '%s\n' "$output" | sed -n 's/^version: //p' ;;
  golangci-lint)
    output=${output#*version }
    printf '%s\n' "${output%% *}"
    ;;
  *) return 1 ;;
  esac
}

qmc_tool_matches() {
  [[ "$(qmc_tool_version "$1")" == "$2" ]]
}
