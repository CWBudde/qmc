#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"
if (($# != 3)) || [[ $1 != "check" && $1 != "tag" ]]; then
  echo 'usage: release.sh check|tag VERSION REVIEWED_COMMIT' >&2
  exit 2
fi
mode="$1"
version="$(python3 scripts/release-policy.py "$2" "$3")"
reviewed_commit="$(printf '%s' "$3" | tr 'A-F' 'a-f')"

if [[ $mode == "tag" ]]; then
  [[ "$(git branch --show-current)" == "main" ]] || {
    echo 'Tag creation requires main' >&2
    exit 1
  }
  if git rev-parse --verify --quiet "refs/tags/v$version" >/dev/null; then
    echo 'Release tag already exists' >&2
    exit 1
  fi
  git fetch --no-tags origin main
  [[ "$(git rev-parse FETCH_HEAD)" == "$(git rev-parse HEAD)" ]] || {
    echo 'main differs from the remote reviewed source' >&2
    exit 1
  }
fi

bash scripts/verify-release.sh
# Recheck identity, cleanliness, and metadata after every required gate.
python3 scripts/release-policy.py "$version" "$reviewed_commit" >/dev/null

if [[ $mode == "tag" ]]; then
  [[ "$(git branch --show-current)" == "main" ]] || {
    echo 'Branch changed during release checks' >&2
    exit 1
  }
  git fetch --no-tags origin main
  [[ "$(git rev-parse FETCH_HEAD)" == "$(git rev-parse HEAD)" ]] || {
    echo 'Remote main changed during release checks' >&2
    exit 1
  }
  git tag -a "v$version" "$reviewed_commit" -m "Release v$version" -m "Reviewed-Commit: $reviewed_commit"
  echo "Created reviewed tag v$version; publication is a separate command."
fi
