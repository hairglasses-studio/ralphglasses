#!/usr/bin/env bash
# PostToolUseFailure hook: inject recovery hints for common ralphglasses errors
set -euo pipefail

error="${CLAUDE_TOOL_ERROR:-}"

# Pattern-match common failure modes and suggest recovery
if echo "$error" | grep -qi "circuit.breaker.*open" 2>/dev/null; then
  echo "Recovery: Circuit breaker is OPEN. Use ralphglasses_circuit_reset to reset, or wait for HALF_OPEN transition."
elif echo "$error" | grep -qi "budget.*exceeded\|cost.*limit" 2>/dev/null; then
  echo "Recovery: Session budget exceeded. Check with ralphglasses_session_budget, then ralphglasses_session_budget_set to increase."
elif echo "$error" | grep -qi "session.*not.found\|no.*active.*session" 2>/dev/null; then
  echo "Recovery: Session not found. Use ralphglasses_session_list to see active sessions, or ralphglasses_session_launch to start one."
elif echo "$error" | grep -qi "scan.*path\|repo.*not.found" 2>/dev/null; then
  echo "Recovery: Repo not found in scan path. Run ralphglasses_scan to refresh discovery, then ralphglasses_list to verify."
elif echo "$error" | grep -qi "timeout\|deadline" 2>/dev/null; then
  echo "Recovery: Operation timed out. Check ralphglasses_repo_health for the target repo. The MCP server may need restart."
fi
