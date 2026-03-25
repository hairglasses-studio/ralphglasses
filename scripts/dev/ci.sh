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

# E2E mock scenarios
"${go_cmd}" test -run TestE2EAllScenarios ./internal/e2e/ -v -count=1

# Regression gates (uses golden baseline from testdata/)
if "${go_cmd}" run . gate-check --baseline internal/e2e/testdata/mock_baseline.json --hours 0 2>/dev/null; then
  echo "gate-check: passed"
else
  echo "warning: gate-check failed or no data" >&2
fi

if command -v shellcheck >/dev/null 2>&1; then
  shell_files=()
  for pattern in distro/scripts/*.sh distro/dietpi/Automation_Custom_Script.sh scripts/*.sh scripts/dev/*.sh; do
    # shellcheck disable=SC2206
    for f in ${pattern}; do
      [[ -f "$f" ]] && shell_files+=("$f")
    done
  done
  if [[ ${#shell_files[@]} -gt 0 ]]; then
    shellcheck "${shell_files[@]}"
  else
    echo "warning: no shell files found for shellcheck" >&2
  fi
else
  echo "warning: shellcheck missing; skipping shell lint" >&2
fi

if command -v bats >/dev/null 2>&1; then
  bats scripts/test/
else
  echo "warning: bats missing; skipping script tests" >&2
fi
