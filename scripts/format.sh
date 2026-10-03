#!/usr/bin/env bash
# shellcheck source-path=SCRIPTDIR
set -euo pipefail
qmc_root="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=../tools/versions.sh
source "$qmc_root/tools/versions.sh"
qmc_mode="${1:-check}"
qmc_format_root="${2:-$qmc_root}"
case "$qmc_mode" in
check) qmc_options=(--fail-on-change) ;;
fmt) qmc_options=() ;;
*)
  echo 'usage: format.sh [check|fmt] [git-worktree-root]' >&2
  exit 2
  ;;
esac
bash "$qmc_root/scripts/check-tools.sh" format
# Formatting must not hide diagnostics from a checker that never rewrites files.
bash "$qmc_root/scripts/check-shell.sh" "$qmc_format_root"
# Required checks cannot be weakened by a local treefmt environment override.
for qmc_variable in ${!TREEFMT_@}; do unset "$qmc_variable"; done
treefmt --config-file "$qmc_root/treefmt.toml" --tree-root "$qmc_format_root" \
  --walk git --no-cache --allow-missing-formatter=false "${qmc_options[@]}"
