#!/usr/bin/env bash
# shellcheck source-path=SCRIPTDIR
set -euo pipefail

qmc_root="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=../tools/versions.sh
source "$qmc_root/tools/versions.sh"

case "$(uname -s)" in
Linux) qmc_os=linux ;;
Darwin) qmc_os=darwin ;;
*)
  echo 'error: development setup supports Linux and macOS only' >&2
  exit 1
  ;;
esac
case "$(uname -m)" in
x86_64 | amd64)
  qmc_arch=amd64
  qmc_shell_arch=x86_64
  ;;
aarch64 | arm64)
  qmc_arch=arm64
  qmc_shell_arch=aarch64
  ;;
*)
  echo 'error: development setup supports amd64 and arm64 only' >&2
  exit 1
  ;;
esac

for qmc_required in go node npm curl tar python3; do
  command -v "$qmc_required" >/dev/null || {
    echo "error: install prerequisite $qmc_required first" >&2
    exit 1
  }
done
[[ "$qmc_tools_dir" == /* && "$qmc_tools_dir" != / && "$qmc_tools_dir" != "$HOME" ]] || {
  echo 'error: QMC_TOOLS_DIR must be an absolute dedicated directory' >&2
  exit 1
}
mkdir -p "$qmc_tools_dir"
[[ -O "$qmc_tools_dir" && ! -L "$qmc_tools_dir" ]] || {
  echo 'error: tool directory must be owned by this user and not a symlink' >&2
  exit 1
}
mkdir -p "$qmc_tools_dir/bin"
[[ -O "$qmc_tools_dir/bin" && ! -L "$qmc_tools_dir/bin" ]] || {
  echo 'error: tool bin directory must be owned by this user and not a symlink' >&2
  exit 1
}
qmc_stage=$(mktemp -d)
trap 'rm -rf "$qmc_stage"' EXIT
mkdir "$qmc_stage/bin"

publish_binary() {
  qmc_tool_matches "$qmc_stage/bin/$1" "$2" || {
    echo "error: downloaded $1 has an unexpected version" >&2
    exit 1
  }
  chmod 755 "$qmc_stage/bin/$1"
  mv -f "$qmc_stage/bin/$1" "$qmc_tools_dir/bin/$1"
}

install_archive() {
  local name=$1 version=$2 asset=$3 url=$4 member=$5
  if qmc_tool_matches "$name" "$version"; then return; fi
  curl --fail --location --silent --show-error --retry 2 --connect-timeout 15 --max-time 180 "$url" -o "$qmc_stage/$asset"
  # Verify against repository-owned pins before opening the downloaded archive.
  python3 - "$qmc_root/tools/archive-checksums.sha256" "$qmc_stage/$asset" <<'PY'
import hashlib
import pathlib
import sys

manifest, archive = map(pathlib.Path, sys.argv[1:])
pins = dict(line.split()[::-1] for line in manifest.read_text().splitlines()
            if line.strip() and not line.startswith("#"))
expected = pins.get(archive.name)
actual = hashlib.sha256(archive.read_bytes()).hexdigest()
if expected is None or actual != expected:
    sys.exit(f"error: checksum verification failed for {archive.name}")
PY
  mkdir -p "$qmc_stage/extract-$name"
  tar -xzf "$qmc_stage/$asset" -C "$qmc_stage/extract-$name" "$member"
  cp "$qmc_stage/extract-$name/$member" "$qmc_stage/bin/$name"
  publish_binary "$name" "$version"
}

install_go_tool() {
  local name=$1 version=$2 module=$3
  if qmc_tool_matches "$name" "$version"; then return; fi
  echo "Installing $name $version"
  GOBIN="$qmc_stage/bin" GOTOOLCHAIN="go$qmc_development_go_version" GOSUMDB=sum.golang.org GONOSUMDB='' go install "$module@v$version"
  publish_binary "$name" "$version"
}

qmc_asset="treefmt_${qmc_treefmt_version}_${qmc_os}_${qmc_arch}.tar.gz"
install_archive treefmt "$qmc_treefmt_version" "$qmc_asset" "https://github.com/numtide/treefmt/releases/download/v$qmc_treefmt_version/$qmc_asset" treefmt
qmc_asset="shellcheck-v${qmc_shellcheck_version}.${qmc_os}.${qmc_shell_arch}.tar.gz"
install_archive shellcheck "$qmc_shellcheck_version" "$qmc_asset" "https://github.com/koalaman/shellcheck/releases/download/v$qmc_shellcheck_version/$qmc_asset" "shellcheck-v$qmc_shellcheck_version/shellcheck"
install_go_tool gofumpt "$qmc_gofumpt_version" mvdan.cc/gofumpt
install_go_tool gci "$qmc_gci_version" github.com/daixiang0/gci
install_go_tool shfmt "$qmc_shfmt_version" mvdan.cc/sh/v3/cmd/shfmt
install_go_tool golangci-lint "$qmc_golangci_version" github.com/golangci/golangci-lint/v2/cmd/golangci-lint

if ! qmc_tool_matches prettier "$qmc_prettier_version"; then
  mkdir -p "$qmc_tools_dir/prettier"
  [[ -O "$qmc_tools_dir/prettier" && ! -L "$qmc_tools_dir/prettier" ]] || {
    echo 'error: prettier directory must be user owned and not a symlink' >&2
    exit 1
  }
  cp "$qmc_root/tools/package.json" "$qmc_root/tools/package-lock.json" "$qmc_tools_dir/prettier/"
  npm ci --prefix "$qmc_tools_dir/prettier" --ignore-scripts --no-audit --no-fund
  ln -sf "$qmc_tools_dir/prettier/node_modules/.bin/prettier" "$qmc_tools_dir/bin/prettier"
fi
qmc_tool_matches prettier "$qmc_prettier_version" || {
  echo 'error: prettier version verification failed' >&2
  exit 1
}
printf 'Pinned tools are ready in %s/bin; prepend this directory to PATH for direct use.\n' "$qmc_tools_dir"
