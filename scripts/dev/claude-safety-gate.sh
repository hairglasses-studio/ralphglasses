#!/usr/bin/env bash
# PreToolUse hook: require confirmation for destructive fleet operations
# Exit 2 = block with feedback; Exit 0 = allow
set -euo pipefail

tool="${CLAUDE_TOOL_NAME:-}"
input="${CLAUDE_TOOL_INPUT:-}"

# Block fleet-wide destructive operations without explicit confirm param
case "$tool" in
  mcp__ralphglasses__ralphglasses_session_stop_all|\
  mcp__ralphglasses__ralphglasses_stop_all|\
  mcp__ralphglasses__ralphglasses_fleet_reset|\
  mcp__ralphglasses__ralphglasses_loop_stop_all)
    if ! echo "$input" | grep -q '"confirm".*true' 2>/dev/null; then
      echo "BLOCKED: $tool requires confirm=true parameter. This is a fleet-wide destructive operation." >&2
      exit 2
    fi
    ;;
esac

exit 0
