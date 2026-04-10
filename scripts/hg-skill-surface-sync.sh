#!/usr/bin/env bash
set -euo pipefail

REPO_PATH=""
MODE="sync"

usage() {
  cat <<'EOF'
Usage: scripts/hg-skill-surface-sync.sh <repo_path> [--dry-run|--check]

Refresh or verify ralphglasses-generated skill compatibility surfaces using the
repo-local genskillsurface pipeline.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)
      MODE="check"
      shift
      ;;
    --check)
      MODE="check"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -*)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
    *)
      [[ -z "$REPO_PATH" ]] || {
        echo "Only one repo path may be provided" >&2
        exit 1
      }
      REPO_PATH="$1"
      shift
      ;;
  esac
done

[[ -n "$REPO_PATH" ]] || {
  usage >&2
  exit 1
}

REPO_PATH="$(cd "$REPO_PATH" && pwd)"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

[[ "$REPO_PATH" == "$REPO_ROOT" ]] || {
  echo "Repo path must be the ralphglasses repo root: $REPO_ROOT" >&2
  exit 1
}

cd "$REPO_ROOT"
if [[ "$MODE" == "check" ]]; then
  go run ./tools/genskillsurface --check
  exit 0
fi

go run ./tools/genskillsurface
