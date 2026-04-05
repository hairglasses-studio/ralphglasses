#!/usr/bin/env bash
# test-session-handoff.sh — End-to-end test of session_handoff MCP tool.
# Run in a separate terminal (outside Codex or Claude) to avoid nesting issues.
#
# Usage:
#   ./scripts/dev/test-session-handoff.sh
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"

# Source .env for API keys.
if [ -f "${repo_root}/.env" ]; then
  set -a
  source "${repo_root}/.env"
  set +a
fi

# Strip nesting env vars.
unset CLAUDECODE CLAUDE_CODE_ENTRYPOINT 2>/dev/null || true

cd "${repo_root}"

echo "==> Building ralphglasses..."
go build -o /tmp/rg-test ./...

echo "==> Starting session via direct API call..."
# Launch a quick Codex session.
SESSION_JSON=$(/tmp/rg-test mcp-call ralphglasses_session_launch \
  --repo ralphglasses \
  --prompt "Run: echo hello from handoff test" \
  --provider codex \
  --budget_usd 1 \
  --max_turns 2 2>&1 || true)

echo "Session launch response:"
echo "${SESSION_JSON}" | jq . 2>/dev/null || echo "${SESSION_JSON}"

SESSION_ID=$(echo "${SESSION_JSON}" | jq -r '.session_id // empty' 2>/dev/null)
if [ -z "${SESSION_ID}" ]; then
  echo "ERROR: Failed to extract session_id"
  echo "The MCP tool test requires the server to be running."
  echo ""
  echo "Alternative: use the standalone MCP server in another terminal:"
  echo "  ./scripts/dev/run-mcp-standalone.sh --scan-path ~/hairglasses-studio"
  echo ""
  echo "Then from a Codex or Claude MCP client, call:"
  echo "  session_launch(repo=ralphglasses, prompt='say hello', provider=codex)"
  echo "  session_handoff(source_session_id=<id>, target_provider=codex)"
  exit 1
fi

echo ""
echo "==> Session launched: ${SESSION_ID}"
echo "==> Waiting 10s for session to complete..."
sleep 10

echo "==> Testing handoff to same provider..."
HANDOFF_JSON=$(/tmp/rg-test mcp-call ralphglasses_session_handoff \
  --source_session_id "${SESSION_ID}" \
  --handoff_reason "test handoff" \
  --stop_source true 2>&1 || true)

echo "Handoff response:"
echo "${HANDOFF_JSON}" | jq . 2>/dev/null || echo "${HANDOFF_JSON}"

echo ""
echo "==> Test complete."
