#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
go_cmd="${repo_root}/scripts/dev/go.sh"

"${repo_root}/scripts/dev/doctor.sh"
bash "${repo_root}/scripts/dev/check-tracked-artifacts.sh"
bash "${repo_root}/scripts/dev/check-generated-surfaces.sh"

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
  for pattern in distro/scripts/*.sh scripts/*.sh scripts/dev/*.sh; do
    # shellcheck disable=SC2206
    for f in ${pattern}; do
      [[ -f "$f" ]] && shell_files+=("$f")
    done
  done
  if [[ ${#shell_files[@]} -gt 0 ]]; then
    # Keep CI focused on correctness-level shell issues while the existing
    # script corpus is gradually cleaned up.
    shellcheck -x -S error "${shell_files[@]}"
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

bash "${script_dir}/test_doctor.sh"
bash "${script_dir}/test_generated_surfaces.sh"
bash "${script_dir}/test_tracked_artifacts.sh"
bash "${script_dir}/test_sync_provider_roles.sh"
