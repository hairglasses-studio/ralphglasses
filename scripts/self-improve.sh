#!/usr/bin/env bash
set -euo pipefail

# Self-improvement wrapper for ralphglasses.
# Usage: scripts/self-improve.sh [--budget USD] [--duration HOURS] [--iterations N] [--repo PATH]

BUDGET_USD="${BUDGET_USD:-20}"
DURATION_HOURS="${DURATION_HOURS:-4}"
MAX_ITERATIONS="${MAX_ITERATIONS:-5}"
REPO_PATH="${REPO_PATH:-.}"

while [[ $# -gt 0 ]]; do
  case $1 in
    --budget) BUDGET_USD="$2"; shift 2 ;;
    --duration) DURATION_HOURS="$2"; shift 2 ;;
    --iterations) MAX_ITERATIONS="$2"; shift 2 ;;
    --repo) REPO_PATH="$2"; shift 2 ;;
    *) echo "Unknown arg: $1"; exit 1 ;;
  esac
done

REPO_PATH="$(cd "$REPO_PATH" && pwd)"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== Self-Improvement Pre-flight ==="

# Check gh CLI availability
if ! command -v gh &>/dev/null; then
  echo "WARNING: gh CLI not found — PR creation will be skipped for review-required changes"
fi

# Run CI as pre-flight check
echo "Running CI pre-flight..."
if [ -f "$ROOT_DIR/scripts/dev/ci.sh" ]; then
  "$ROOT_DIR/scripts/dev/ci.sh" || { echo "FATAL: CI pre-flight failed"; exit 1; }
fi

echo ""
echo "=== Starting Self-Improvement Loop ==="
echo "  Repo:       $REPO_PATH"
echo "  Budget:     \$${BUDGET_USD}"
echo "  Duration:   ${DURATION_HOURS}h"
echo "  Iterations: ${MAX_ITERATIONS}"
echo ""

# Build binary if needed
BINARY="$ROOT_DIR/ralphglasses"
if [ ! -f "$BINARY" ] || [ "$ROOT_DIR/go.mod" -nt "$BINARY" ]; then
  echo "Building ralphglasses..."
  (cd "$ROOT_DIR" && go build -o ralphglasses ./...)
fi

# Ensure log directory exists
mkdir -p "$ROOT_DIR/.ralph/logs"

# Start the self-improvement loop via MCP
echo "Starting loop..."
"$BINARY" mcp-call ralphglasses_self_improve \
  -p "repo=$(basename "$REPO_PATH")" \
  -p "budget_usd=$BUDGET_USD" \
  -p "max_iterations=$MAX_ITERATIONS" \
  -p "duration_hours=$DURATION_HOURS" \
  2>&1 | tee "$ROOT_DIR/.ralph/logs/self-improve-$(date +%Y%m%d-%H%M%S).log"

echo ""
echo "=== Self-Improvement Complete ==="
