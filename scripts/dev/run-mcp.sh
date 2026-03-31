#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"

# Source .env if present (API keys for session providers).
if [ -f "${repo_root}/.env" ]; then
  set -a
  # shellcheck source=/dev/null
  source "${repo_root}/.env"
  set +a
fi

exec "${repo_root}/scripts/dev/go.sh" run . mcp "$@"
