#!/usr/bin/env bash
# Unit tests for doctor.sh exit-code behavior.
# Usage: bash scripts/dev/test_doctor.sh
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
doctor="${script_dir}/doctor.sh"

pass=0
fail_count=0
cleanup_dirs=()

cleanup() {
  for d in "${cleanup_dirs[@]+"${cleanup_dirs[@]}"}"; do
    rm -rf "${d}"
  done
}
trap cleanup EXIT

# Absolute path to bash so env PATH=... doesn't prevent finding it.
BASH_BIN="$(command -v bash)"

# make_stub_path <tool> [<tool> ...] → prints path to a temp dir containing:
#   - symlinks to real awk, grep, uname (needed by env.sh internals)
#   - simple stub executables for each requested <tool>
make_stub_path() {
  local dir
  dir="$(mktemp -d)"
  cleanup_dirs+=("${dir}")
  # Essential system tools required by doctor.sh/env.sh internals.
  # Resolve via /usr/bin:/bin to get real binary paths, not shell aliases.
  for tool in awk grep uname dirname; do
    local real_path
    real_path="$(PATH="/usr/bin:/bin" command -v "${tool}" 2>/dev/null)" || true
    if [[ -n "${real_path}" && -x "${real_path}" ]]; then
      ln -sf "${real_path}" "${dir}/${tool}"
    fi
  done
  # Stub executables for each requested tool
  for tool in "$@"; do
    printf '#!/bin/sh\necho "stub %s"\n' "${tool}" > "${dir}/${tool}"
    chmod +x "${dir}/${tool}"
  done
  printf '%s\n' "${dir}"
}

assert_exit() {
  local desc="$1"
  local expected_exit="$2"
  shift 2
  local actual_exit=0
  "$@" >/dev/null 2>&1 || actual_exit=$?
  if [[ "${actual_exit}" -eq "${expected_exit}" ]]; then
    echo "PASS: ${desc}"
    pass=$((pass + 1))
  else
    echo "FAIL: ${desc} (expected exit ${expected_exit}, got ${actual_exit})"
    fail_count=$((fail_count + 1))
  fi
}

# Test 1: required tools present (go + make + git), optional tools absent (no shellcheck/bats) → exit 0
stub_required="$(make_stub_path go make git)"
assert_exit "exits 0 when required tools present, optional tools absent" 0 \
  env PATH="${stub_required}" "${BASH_BIN}" "${doctor}"

# Test 2: make absent, go + git present → exit non-zero
stub_no_make="$(make_stub_path go git)"
assert_exit "exits non-zero when required tool make is absent" 1 \
  env PATH="${stub_no_make}" "${BASH_BIN}" "${doctor}"

# Test 3: system go absent but repo-local bootstrap go still available → exit 0
stub_no_go="$(make_stub_path make git)"
assert_exit "exits 0 when bootstrap-managed go is available without system go" 0 \
  env PATH="${stub_no_go}" "${BASH_BIN}" "${doctor}"

# Test 4: git absent, go + make present → exit non-zero
stub_no_git="$(make_stub_path go make)"
assert_exit "exits non-zero when required tool git is absent" 1 \
  env PATH="${stub_no_git}" "${BASH_BIN}" "${doctor}"

echo ""
echo "Results: ${pass} passed, ${fail_count} failed"
if [[ "${fail_count}" -gt 0 ]]; then
  exit 1
fi
