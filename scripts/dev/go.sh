#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./env.sh
source "${script_dir}/env.sh"

repo_root="$(rg_repo_root)"
go_bin="$(rg_ensure_go "${repo_root}")"
export PATH="$(dirname "${go_bin}")":"$(rg_local_bin_dir "${repo_root}"):${PATH}"

exec "${go_bin}" "$@"
