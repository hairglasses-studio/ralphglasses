#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
gate_script="${repo_root}/scripts/dev/check-generated-surfaces.sh"

pass=0
fail_count=0
cleanup_dirs=()

cleanup() {
  for d in "${cleanup_dirs[@]+"${cleanup_dirs[@]}"}"; do
    rm -rf "${d}"
  done
}
trap cleanup EXIT

record_pass() {
  echo "PASS: $1"
  pass=$((pass + 1))
}

record_fail() {
  echo "FAIL: $1"
  fail_count=$((fail_count + 1))
}

assert_contains() {
  local desc="$1"
  local file="$2"
  local needle="$3"
  if grep -Fq "${needle}" "${file}"; then
    record_pass "${desc}"
  else
    record_fail "${desc} (missing ${needle})"
  fi
}

fixture="$(mktemp -d)"
cleanup_dirs+=("${fixture}")
mkdir -p "${fixture}/bin" "${fixture}/scripts/dev" "${fixture}/scripts"

cp "${gate_script}" "${fixture}/scripts/dev/check-generated-surfaces.sh"

log_file="${fixture}/calls.log"

cat > "${fixture}/bin/fake-python" <<EOF
#!/usr/bin/env bash
set -euo pipefail
printf 'python:%s\n' "\$*" >> "${log_file}"
exit 0
EOF

cat > "${fixture}/bin/fake-go" <<EOF
#!/usr/bin/env bash
set -euo pipefail
printf 'go:%s\n' "\$*" >> "${log_file}"
exit 0
EOF

chmod +x "${fixture}/bin/fake-python" "${fixture}/bin/fake-go" "${fixture}/scripts/dev/check-generated-surfaces.sh"

PYTHON_BIN="${fixture}/bin/fake-python" \
GO_CMD="${fixture}/bin/fake-go" \
bash "${fixture}/scripts/dev/check-generated-surfaces.sh"

assert_contains "provider role drift check runs in gate" \
  "${log_file}" \
  "python:${fixture}/scripts/sync-provider-roles.py --check"

assert_contains "generated surface gate runs targeted go test command" \
  "${log_file}" \
  "go:test -count=1 ./tools/genconfig ./tools/gendoc ./tools/genskilldoc ./tools/genmcpdocs ./tools/genskillsurface ./internal/mcpserver"

assert_contains "generated surface gate scopes go test to drift tests" \
  "${log_file}" \
  "TestBuildToolGroups_ExpectedCounts"

echo
echo "Results: ${pass} passed, ${fail_count} failed"
if [[ "${fail_count}" -gt 0 ]]; then
  exit 1
fi
