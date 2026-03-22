#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
go_cmd="${repo_root}/scripts/dev/go.sh"

"${repo_root}/scripts/dev/doctor.sh"

"${go_cmd}" vet ./...

if command -v cc >/dev/null 2>&1 || command -v gcc >/dev/null 2>&1; then
  "${go_cmd}" test -race ./...
else
  echo "warning: C compiler missing; falling back to non-race go test ./..." >&2
  "${go_cmd}" test ./...
fi

"${go_cmd}" build ./...

if command -v shellcheck >/dev/null 2>&1; then
  shellcheck distro/scripts/*.sh distro/dietpi/Automation_Custom_Script.sh scripts/*.sh scripts/dev/*.sh
else
  echo "warning: shellcheck missing; skipping shell lint" >&2
fi

if command -v bats >/dev/null 2>&1; then
  bats scripts/test/
else
  echo "warning: bats missing; skipping script tests" >&2
fi
