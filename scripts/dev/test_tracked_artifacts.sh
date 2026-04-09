#!/usr/bin/env bash
# Unit tests for tracked artifact gate behavior.
# Usage: bash scripts/dev/test_tracked_artifacts.sh
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
checker="${script_dir}/check-tracked-artifacts.sh"

pass=0
fail_count=0
cleanup_dirs=()

cleanup() {
  for d in "${cleanup_dirs[@]+"${cleanup_dirs[@]}"}"; do
    rm -rf "${d}"
  done
}
trap cleanup EXIT

BASH_BIN="$(command -v bash)"

make_fixture_repo() {
  local dir
  dir="$(mktemp -d)"
  cleanup_dirs+=("${dir}")
  git -C "${dir}" init -q
  printf '# fixture\n' > "${dir}/README.md"
  git -C "${dir}" add README.md
  printf '%s\n' "${dir}"
}

record_pass() {
  echo "PASS: $1"
  pass=$((pass + 1))
}

record_fail() {
  echo "FAIL: $1"
  fail_count=$((fail_count + 1))
}

assert_exit() {
  local desc="$1"
  local expected_exit="$2"
  shift 2
  local actual_exit=0
  "$@" >/dev/null 2>&1 || actual_exit=$?
  if [[ "${actual_exit}" -eq "${expected_exit}" ]]; then
    record_pass "${desc}"
  else
    record_fail "${desc} (expected exit ${expected_exit}, got ${actual_exit})"
  fi
}

fixture_clean="$(make_fixture_repo)"
assert_exit "clean tracked file set passes" 0 \
  "${BASH_BIN}" "${checker}" "${fixture_clean}"

fixture_ds_store="$(make_fixture_repo)"
printf 'junk\n' > "${fixture_ds_store}/.DS_Store"
git -C "${fixture_ds_store}" add .DS_Store
set +e
output_ds_store="$("${BASH_BIN}" "${checker}" "${fixture_ds_store}" 2>&1)"
status_ds_store=$?
set -e
if [[ "${status_ds_store}" -eq 1 && "${output_ds_store}" == *".DS_Store"* ]]; then
  record_pass "tracked .DS_Store fails with useful path"
else
  record_fail "tracked .DS_Store fails with useful path"
fi

fixture_tmp_dir="$(make_fixture_repo)"
mkdir -p "${fixture_tmp_dir}/tmp"
printf 'artifact\n' > "${fixture_tmp_dir}/tmp/output.txt"
git -C "${fixture_tmp_dir}" add tmp/output.txt
set +e
output_tmp_dir="$("${BASH_BIN}" "${checker}" "${fixture_tmp_dir}" 2>&1)"
status_tmp_dir=$?
set -e
if [[ "${status_tmp_dir}" -eq 1 && "${output_tmp_dir}" == *"tmp/output.txt"* ]]; then
  record_pass "tracked temp directory segment fails with useful path"
else
  record_fail "tracked temp directory segment fails with useful path"
fi

fixture_placeholder="$(make_fixture_repo)"
printf 'placeholder\n' > "${fixture_placeholder}/notes-placeholder.md"
git -C "${fixture_placeholder}" add notes-placeholder.md
set +e
output_placeholder="$("${BASH_BIN}" "${checker}" "${fixture_placeholder}" 2>&1)"
status_placeholder=$?
set -e
if [[ "${status_placeholder}" -eq 1 && "${output_placeholder}" == *"notes-placeholder.md"* ]]; then
  record_pass "placeholder output filename fails with useful path"
else
  record_fail "placeholder output filename fails with useful path"
fi

fixture_allowlist="$(make_fixture_repo)"
mkdir -p "${fixture_allowlist}/tmp"
printf 'artifact\n' > "${fixture_allowlist}/tmp/output.txt"
printf 'tmp/output.txt\n' > "${fixture_allowlist}/.tracked-artifact-allowlist"
git -C "${fixture_allowlist}" add tmp/output.txt .tracked-artifact-allowlist
assert_exit "allowlisted tracked artifact path is tolerated" 0 \
  "${BASH_BIN}" "${checker}" "${fixture_allowlist}"

echo
echo "Results: ${pass} passed, ${fail_count} failed"
if [[ "${fail_count}" -gt 0 ]]; then
  exit 1
fi
