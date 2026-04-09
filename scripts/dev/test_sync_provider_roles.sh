#!/usr/bin/env bash
# Fixture tests for provider role projection generation and drift detection.
# Usage: bash scripts/dev/test_sync_provider_roles.sh
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
generator="${repo_root}/scripts/sync-provider-roles.py"

pass=0
fail_count=0
cleanup_dirs=()

cleanup() {
  for d in "${cleanup_dirs[@]+"${cleanup_dirs[@]}"}"; do
    rm -rf "${d}"
  done
}
trap cleanup EXIT

PYTHON_BIN="$(command -v python3)"

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

assert_file_contains() {
  local desc="$1"
  local file="$2"
  local needle="$3"
  if grep -Fq "${needle}" "${file}"; then
    record_pass "${desc}"
  else
    record_fail "${desc} (missing ${needle} in ${file})"
  fi
}

make_fixture_repo() {
  local dir
  dir="$(mktemp -d)"
  cleanup_dirs+=("${dir}")
  mkdir -p "${dir}/scripts" "${dir}/.agents/roles"
  cp "${generator}" "${dir}/scripts/sync-provider-roles.py"
  cat > "${dir}/.agents/roles/docs-researcher.json" <<'JSON'
{
  "name": "docs-researcher",
  "summary": "Base summary",
  "prompt": "Base prompt.",
  "provider_overrides": {
    "codex": {
      "surface": ".codex/agents/docs_researcher.toml",
      "description": "Codex description",
      "prompt": "Codex prompt.",
      "model": "gpt-5.4-mini",
      "model_reasoning_effort": "medium",
      "sandbox_mode": "read-only"
    },
    "gemini": {
      "surface": ".gemini/agents/docs_researcher.md",
      "description": "Gemini description",
      "prompt": "Gemini prompt.",
      "model": "gemini-2.5-pro",
      "tools": ["read_file", "search_repo"],
      "max_turns": 5
    }
  }
}
JSON
  printf '%s\n' "${dir}"
}

fixture="$(make_fixture_repo)"

assert_exit "generator writes provider projections for fixture repo" 0 \
  "${PYTHON_BIN}" "${fixture}/scripts/sync-provider-roles.py"

assert_file_contains "codex projection uses override surface" \
  "${fixture}/.codex/agents/docs_researcher.toml" \
  'name = "docs_researcher"'
assert_file_contains "codex projection preserves override description" \
  "${fixture}/.codex/agents/docs_researcher.toml" \
  'description = "Codex description"'
assert_file_contains "codex projection renders reasoning override" \
  "${fixture}/.codex/agents/docs_researcher.toml" \
  'model_reasoning_effort = "medium"'
assert_file_contains "codex projection renders sandbox override" \
  "${fixture}/.codex/agents/docs_researcher.toml" \
  'sandbox_mode = "read-only"'
assert_file_contains "codex projection renders override prompt" \
  "${fixture}/.codex/agents/docs_researcher.toml" \
  'Codex prompt.'

assert_file_contains "claude projection uses default surface" \
  "${fixture}/.claude/agents/docs-researcher.md" \
  'provider: claude -->'
assert_file_contains "claude projection falls back to base summary" \
  "${fixture}/.claude/agents/docs-researcher.md" \
  'description: Base summary'
assert_file_contains "claude projection falls back to base prompt" \
  "${fixture}/.claude/agents/docs-researcher.md" \
  'Base prompt.'

assert_file_contains "gemini projection uses override surface" \
  "${fixture}/.gemini/agents/docs_researcher.md" \
  'provider: gemini -->'
assert_file_contains "gemini projection renders override model" \
  "${fixture}/.gemini/agents/docs_researcher.md" \
  'model: gemini-2.5-pro'
assert_file_contains "gemini projection renders override tools" \
  "${fixture}/.gemini/agents/docs_researcher.md" \
  'tools: [read_file, search_repo]'
assert_file_contains "gemini projection renders override max turns" \
  "${fixture}/.gemini/agents/docs_researcher.md" \
  'maxTurns: 5'
assert_file_contains "gemini projection records source manifest" \
  "${fixture}/.gemini/agents/docs_researcher.md" \
  'source_manifest: .agents/roles/docs-researcher.json -->'

assert_exit "generator check passes after sync" 0 \
  "${PYTHON_BIN}" "${fixture}/scripts/sync-provider-roles.py" --check

printf '\n# drift\n' >> "${fixture}/.gemini/agents/docs_researcher.md"
set +e
check_output="$("${PYTHON_BIN}" "${fixture}/scripts/sync-provider-roles.py" --check 2>&1)"
check_status=$?
set -e
if [[ "${check_status}" -eq 1 \
  && "${check_output}" == *"provider role projections are out of date"* \
  && "${check_output}" == *"drift .gemini/agents/docs_researcher.md"* ]]; then
  record_pass "generator check reports drift with a useful path"
else
  record_fail "generator check reports drift with a useful path"
fi

echo
echo "Results: ${pass} passed, ${fail_count} failed"
if [[ "${fail_count}" -gt 0 ]]; then
  exit 1
fi
