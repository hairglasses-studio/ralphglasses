#!/usr/bin/env bash
# run-mcp-standalone.sh — Run the MCP server outside of Claude Code.
# Use this for testing session_launch, rc_send, and session_handoff tools
# that spawn CLI subprocesses (which hang when nested inside Claude Code).
#
# Usage:
#   ./scripts/dev/run-mcp-standalone.sh [--scan-path ~/hairglasses-studio]
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"

# Source .env for API keys.
if [ -f "${repo_root}/.env" ]; then
  set -a
  # shellcheck source=/dev/null
  source "${repo_root}/.env"
  set +a
fi

# Strip nesting env vars so child CLIs don't refuse to start.
unset CLAUDECODE CLAUDE_CODE_ENTRYPOINT 2>/dev/null || true

echo "Starting ralphglasses MCP server (standalone)..." >&2
echo "  ANTHROPIC_API_KEY: ${ANTHROPIC_API_KEY:+set}" >&2
echo "  OPENAI_API_KEY: ${OPENAI_API_KEY:+set}" >&2
echo "  GOOGLE_API_KEY: ${GOOGLE_API_KEY:+set}" >&2
echo "  GEMINI_API_KEY: ${GEMINI_API_KEY:+set}" >&2

exec "${repo_root}/scripts/dev/go.sh" run . mcp "$@"
