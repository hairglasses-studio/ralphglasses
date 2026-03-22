#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./dev/env.sh
source "${script_dir}/dev/env.sh"

repo_root="$(rg_repo_root)"
go_bin="$(rg_ensure_go "${repo_root}")"
local_bin="$(rg_local_bin_dir "${repo_root}")"
mkdir -p "${local_bin}"
export PATH="$(dirname "${go_bin}"):${local_bin}:${PATH}"
export GOBIN="${local_bin}"

echo "Using Go: $("${go_bin}" version)"

if ! command -v golangci-lint >/dev/null 2>&1 && [[ ! -x "${local_bin}/golangci-lint" ]]; then
  echo "Installing golangci-lint into ${local_bin}"
  "${go_bin}" install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
fi

echo
echo "Bootstrap summary:"
echo "  go: $(command -v "${go_bin}")"
echo "  golangci-lint: $(command -v golangci-lint || printf '%s' "${local_bin}/golangci-lint")"
echo "  make: $(command -v make || echo 'missing (recommended: devcontainer or system package)')"
echo "  cc: $(command -v cc || command -v gcc || echo 'missing (required for go test -race)')"
echo "  shellcheck: $(command -v shellcheck || echo 'missing (recommended: devcontainer or system package)')"
echo "  bats: $(command -v bats || echo 'missing (recommended: devcontainer or system package)')"
