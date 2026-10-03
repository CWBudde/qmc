#!/usr/bin/env bash
# shellcheck source-path=SCRIPTDIR
set -euo pipefail
qmc_root="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=../tools/versions.sh
source "$qmc_root/tools/versions.sh"
bash "$qmc_root/scripts/check-tools.sh" shell
qmc_shell_root="${1:-$qmc_root}"
cd "$qmc_shell_root"
qmc_shell_files=()
qmc_shell_list=$(mktemp)
trap 'rm -f "$qmc_shell_list"' EXIT
# Match the local agent/editor state that treefmt.toml excludes from formatting.
git ls-files -z --cached --others --exclude-standard -- '*.sh' \
  ':(exclude).agents/' ':(exclude).codex/' ':(exclude).aws/' >"$qmc_shell_list"
while IFS= read -r -d '' qmc_file; do
  qmc_shell_files+=("$qmc_file")
done <"$qmc_shell_list"
if ((${#qmc_shell_files[@]})); then
  shellcheck -x "${qmc_shell_files[@]}"
fi
