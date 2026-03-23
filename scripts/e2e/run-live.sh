#!/usr/bin/env bash
set -euo pipefail

# Run live-fire e2e scenarios against real LLM providers.
# Requires ANTHROPIC_API_KEY in environment.

PROVIDER="${PROVIDER:-claude}"
BUDGET="${BUDGET:-10}"
TIMEOUT="${TIMEOUT:-30m}"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
go_cmd="${repo_root}/scripts/dev/go.sh"

echo "=== E2E Live Fire ==="
echo "Provider: ${PROVIDER}"
echo "Budget:   \$${BUDGET}"
echo "Timeout:  ${TIMEOUT}"
echo ""

"${go_cmd}" test \
    -run TestE2ELiveFire \
    ./internal/e2e/ \
    -v -count=1 \
    -timeout "${TIMEOUT}" \
    -tags e2e_live \
    -args -provider="${PROVIDER}" -budget="${BUDGET}"
