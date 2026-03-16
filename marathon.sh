#!/bin/bash
set -euo pipefail

# Ralphglasses 12-Hour Marathon Loop Launcher
# Wraps ralph_loop.sh with marathon defaults and budget guardrails.

# --- Defaults (override with flags or env vars) ---
BUDGET="${BUDGET:-100}"
DURATION_HOURS="${DURATION_HOURS:-12}"
CALLS_PER_HOUR="${CALLS_PER_HOUR:-80}"
TIMEOUT_MINUTES="${TIMEOUT_MINUTES:-20}"
CHECKPOINT_HOURS="${CHECKPOINT_HOURS:-3}"
PROJECT_DIR="${PROJECT_DIR:-$(cd "$(dirname "$0")" && pwd)}"
RALPH_CMD="${RALPH_CMD:-}"
MONITOR=false
VERBOSE=false
LIVE=false
DRY_RUN=false

usage() {
    cat <<EOF
Ralphglasses Marathon Loop Launcher

Usage: $(basename "$0") [OPTIONS]

Options:
  -b, --budget NUM          API budget in USD (default: $BUDGET)
  -d, --duration NUM        Duration in hours (default: $DURATION_HOURS)
  -c, --calls NUM           Max calls per hour (default: $CALLS_PER_HOUR)
  -t, --timeout NUM         Claude timeout in minutes (default: $TIMEOUT_MINUTES)
  -k, --checkpoint NUM      Git tag checkpoint interval in hours (default: $CHECKPOINT_HOURS)
  -p, --project DIR         Project directory (default: script location)
  -m, --monitor             Start with tmux monitoring
  -v, --verbose             Verbose output
  -l, --live                Stream Claude output in real-time
  -n, --dry-run             Print the command without executing
  -h, --help                Show this help

Environment:
  ANTHROPIC_API_KEY         Required. Your Anthropic API key.
  RALPH_CMD                 Path to ralph binary/script (auto-detected).
  BUDGET                    Override budget (same as -b).
  CALLS_PER_HOUR            Override calls/hour (same as -c).

Examples:
  $(basename "$0")                              # Default 12h / \$100 marathon
  $(basename "$0") -b 50 -d 6                   # 6h / \$50 budget
  $(basename "$0") -c 60 -t 30 -m               # Slower, longer calls, tmux
  $(basename "$0") -n                            # Dry run — print command only
EOF
    exit 0
}

# --- Parse arguments ---
while [[ $# -gt 0 ]]; do
    case "$1" in
        -b|--budget)       BUDGET="$2";           shift 2 ;;
        -d|--duration)     DURATION_HOURS="$2";   shift 2 ;;
        -c|--calls)        CALLS_PER_HOUR="$2";   shift 2 ;;
        -t|--timeout)      TIMEOUT_MINUTES="$2";  shift 2 ;;
        -k|--checkpoint)   CHECKPOINT_HOURS="$2";  shift 2 ;;
        -p|--project)      PROJECT_DIR="$2";       shift 2 ;;
        -m|--monitor)      MONITOR=true;           shift ;;
        -v|--verbose)      VERBOSE=true;           shift ;;
        -l|--live)         LIVE=true;              shift ;;
        -n|--dry-run)      DRY_RUN=true;           shift ;;
        -h|--help)         usage ;;
        *)
            echo "Error: Unknown option: $1" >&2
            echo "Run '$(basename "$0") --help' for usage." >&2
            exit 1
            ;;
    esac
done

# --- Validate ---
if [[ -z "${ANTHROPIC_API_KEY:-}" ]]; then
    echo "Error: ANTHROPIC_API_KEY is not set." >&2
    echo "  export ANTHROPIC_API_KEY=sk-ant-..." >&2
    exit 1
fi

if [[ ! -d "$PROJECT_DIR" ]]; then
    echo "Error: Project directory not found: $PROJECT_DIR" >&2
    exit 1
fi

if [[ ! -f "$PROJECT_DIR/.ralph/PROMPT.md" ]]; then
    echo "Error: No .ralph/PROMPT.md in $PROJECT_DIR — is this a ralph project?" >&2
    exit 1
fi

# --- Find ralph ---
if [[ -z "$RALPH_CMD" ]]; then
    if command -v ralph &>/dev/null; then
        RALPH_CMD="ralph"
    elif [[ -x "$HOME/hairglasses-studio/ralph-claude-code/ralph_loop.sh" ]]; then
        RALPH_CMD="$HOME/hairglasses-studio/ralph-claude-code/ralph_loop.sh"
    else
        echo "Error: Cannot find ralph. Set RALPH_CMD or install ralph-claude-code." >&2
        exit 1
    fi
fi

# --- Update .ralphrc with marathon settings ---
RALPHRC="$PROJECT_DIR/.ralphrc"
if [[ -f "$RALPHRC" ]]; then
    # Update values in-place
    sed -i '' "s/^MAX_CALLS_PER_HOUR=.*/MAX_CALLS_PER_HOUR=$CALLS_PER_HOUR/" "$RALPHRC" 2>/dev/null || true
    sed -i '' "s/^CLAUDE_TIMEOUT_MINUTES=.*/CLAUDE_TIMEOUT_MINUTES=$TIMEOUT_MINUTES/" "$RALPHRC" 2>/dev/null || true
    sed -i '' "s/^MARATHON_DURATION_HOURS=.*/MARATHON_DURATION_HOURS=$DURATION_HOURS/" "$RALPHRC" 2>/dev/null || true
    sed -i '' "s/^MARATHON_CHECKPOINT_INTERVAL=.*/MARATHON_CHECKPOINT_INTERVAL=$CHECKPOINT_HOURS/" "$RALPHRC" 2>/dev/null || true
    sed -i '' "s/^RALPH_SESSION_BUDGET=.*/RALPH_SESSION_BUDGET=$BUDGET/" "$RALPHRC" 2>/dev/null || true
    # Add budget if not present
    if ! grep -q "^RALPH_SESSION_BUDGET=" "$RALPHRC"; then
        echo "RALPH_SESSION_BUDGET=$BUDGET" >> "$RALPHRC"
    fi
fi

# --- Build command ---
CMD=("$RALPH_CMD")
CMD+=("--calls" "$CALLS_PER_HOUR")
CMD+=("--timeout" "$TIMEOUT_MINUTES")
CMD+=("--auto-reset-circuit")

if $MONITOR; then
    CMD+=("--monitor")
fi
if $VERBOSE; then
    CMD+=("--verbose")
fi
if $LIVE; then
    CMD+=("--live")
fi

# --- Estimate ---
est_per_hour=$(echo "scale=0; $CALLS_PER_HOUR * 12 / 100" | bc)
est_total=$(echo "scale=0; $est_per_hour * $DURATION_HOURS" | bc)

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Ralphglasses Marathon Loop"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Project:     $(basename "$PROJECT_DIR")"
echo "  Duration:    ${DURATION_HOURS}h"
echo "  Budget:      \$$BUDGET"
echo "  Calls/hour:  $CALLS_PER_HOUR"
echo "  Timeout:     ${TIMEOUT_MINUTES}m per call"
echo "  Checkpoint:  every ${CHECKPOINT_HOURS}h"
echo "  Estimate:    ~\$${est_per_hour}/hr → ~\$${est_total} total"
echo "  Ralph:       $RALPH_CMD"
echo "  Monitor:     $MONITOR"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "  Command: ${CMD[*]}"
echo ""

if $DRY_RUN; then
    echo "[dry-run] Would execute in: $PROJECT_DIR"
    echo "[dry-run] ${CMD[*]}"
    exit 0
fi

# --- Launch ---
echo "Starting in 3 seconds... (Ctrl+C to cancel)"
sleep 3

cd "$PROJECT_DIR"
exec "${CMD[@]}"
