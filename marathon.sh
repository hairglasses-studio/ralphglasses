#!/bin/bash
set -euo pipefail

# Ralphglasses 12-Hour Marathon Loop Launcher
# Supervises a ralph loop with budget guardrails, duration limits, and checkpoints.

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

# Cost estimation: conservative avg cost per Claude call in USD.
# Sonnet ~$0.10-0.50/call, Opus ~$0.50-2.00/call. Default assumes Sonnet.
COST_PER_CALL="${COST_PER_CALL:-0.15}"
# Budget headroom: stop at this fraction to avoid overshoot (e.g. 0.90 = stop at 90%)
BUDGET_HEADROOM="${BUDGET_HEADROOM:-0.90}"

MARATHON_LOG=""
RALPH_PID=""
START_EPOCH=""
CHECKPOINT_COUNT=0
RESTART_COUNT=0
CONSECUTIVE_RESTARTS=0
MAX_CONSECUTIVE_RESTARTS="${MAX_CONSECUTIVE_RESTARTS:-10}"
MAX_RESTARTS="${MAX_RESTARTS:-5}"
RESTART_BACKOFF=30
COOLDOWN_SECONDS=60

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
  RALPH_CMD                 Path to ralph binary/script (auto-detected).
  BUDGET                    Override budget (same as -b).
  CALLS_PER_HOUR            Override calls/hour (same as -c).
  COST_PER_CALL             Estimated USD per API call for fallback tracking (default: 0.15).
  BUDGET_HEADROOM           Stop at this fraction of budget to avoid overshoot (default: 0.90).
  MARATHON_MIN_DISK_GB      Warn when free disk drops below this (default: 5).
  MARATHON_MIN_MEMORY_GB    Warn when free memory drops below this (default: 2).
  MARATHON_MAX_LOG_MB       Rotate marathon log when it exceeds this size (default: 100).
  MARATHON_LOG_KEEP         Number of rotated log files to keep (default: 3).

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

# --- Logging ---
log() {
    local level="$1"; shift
    local ts
    ts="$(date '+%Y-%m-%d %H:%M:%S')"
    local msg="[$ts] [$level] $*"
    echo "$msg"
    if [[ -n "$MARATHON_LOG" ]]; then
        echo "$msg" >> "$MARATHON_LOG"
    fi
}

# --- Pre-launch disk space check ---
MIN_DISK_MB=500
AVAIL_MB=$(df -m . | awk 'NR==2{print $4}')
if [ "$AVAIL_MB" -lt "$MIN_DISK_MB" ]; then
    echo "ERROR: Only ${AVAIL_MB}MB free, need ${MIN_DISK_MB}MB minimum" >&2
    exit 1
fi

# --- Validate dependencies ---
for dep in jq bc; do
    if ! command -v "$dep" &>/dev/null; then
        echo "Error: Required command '$dep' not found. Install it first." >&2
        exit 1
    fi
done

# --- Validate ---
# Note: Claude Code uses its own OAuth session, not ANTHROPIC_API_KEY.
# If ANTHROPIC_API_KEY is set, it overrides Claude's auth and may cause "Invalid API key" errors.
# Unset it to let Claude use its own authentication.
if [[ -n "${ANTHROPIC_API_KEY:-}" ]]; then
    echo "WARNING: Unsetting ANTHROPIC_API_KEY — Claude Code uses its own OAuth session." >&2
    unset ANTHROPIC_API_KEY
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
    # Default to the ralphglasses binary itself with the loop subcommand
    if command -v ralphglasses &>/dev/null; then
        RALPH_CMD="ralphglasses"
    elif [[ -f "$PROJECT_DIR/ralph_loop.sh" ]]; then
        RALPH_CMD="bash $PROJECT_DIR/ralph_loop.sh"
    else
        echo "Error: Cannot find ralphglasses binary or ralph_loop.sh in project dir." >&2
        echo "       Build with: go build -o ralphglasses . && sudo mv ralphglasses /usr/local/bin/" >&2
        exit 1
    fi
fi

# --- Update .ralphrc with marathon settings ---
# Uses temp file pattern for portability across GNU/BSD sed.
update_ralphrc_key() {
    local file="$1" key="$2" value="$3"
    if [[ ! -f "$file" ]]; then
        return
    fi
    if grep -q "^${key}=" "$file"; then
        sed "s|^${key}=.*|${key}=${value}|" "$file" > "${file}.tmp" && mv "${file}.tmp" "$file"
    else
        echo "${key}=${value}" >> "$file"
    fi
}

RALPHRC="$PROJECT_DIR/.ralphrc"

# --- Build command ---
CMD=("$RALPH_CMD")
CMD+=("--calls" "$CALLS_PER_HOUR")
CMD+=("--timeout" "$TIMEOUT_MINUTES")
CMD+=("--auto-reset-circuit")

if $MONITOR; then
    echo "WARNING: --monitor is incompatible with marathon supervisor (tmux fork breaks PID tracking)." >&2
    echo "         Use --verbose or --live instead, or run ralph --monitor directly." >&2
    exit 1
fi
if $VERBOSE; then
    CMD+=("--verbose")
fi
if $LIVE; then
    CMD+=("--live")
fi

# --- Compute estimates ---
# Budget ceiling with headroom
budget_ceiling=$(echo "$BUDGET * $BUDGET_HEADROOM" | bc)
est_per_hour=$(echo "scale=2; $CALLS_PER_HOUR * $COST_PER_CALL" | bc)
est_total=$(echo "scale=2; $est_per_hour * $DURATION_HOURS" | bc)
duration_seconds=$(echo "$DURATION_HOURS * 3600 / 1" | bc)
checkpoint_seconds=$(echo "$CHECKPOINT_HOURS * 3600 / 1" | bc)

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Ralphglasses Marathon Loop"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Project:     $(basename "$PROJECT_DIR")"
echo "  Duration:    ${DURATION_HOURS}h (hard limit)"
echo "  Budget:      \$$BUDGET (stop at \$$budget_ceiling)"
echo "  Calls/hour:  $CALLS_PER_HOUR"
echo "  Timeout:     ${TIMEOUT_MINUTES}m per call"
echo "  Checkpoint:  every ${CHECKPOINT_HOURS}h"
echo "  Cost/call:   ~\$$COST_PER_CALL"
echo "  Estimate:    ~\$${est_per_hour}/hr → ~\$${est_total} total"
echo "  Ralph:       $RALPH_CMD"
echo "  Monitor:     $MONITOR"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "  Command: ${CMD[*]}"
echo ""

# Warn if estimate exceeds budget
if (( $(echo "$est_total > $BUDGET" | bc -l) )); then
    echo "  WARNING: Estimated cost (\$$est_total) exceeds budget (\$$BUDGET)."
    echo "           The supervisor will enforce the budget limit at runtime."
    echo ""
fi

if $DRY_RUN; then
    echo "[dry-run] Would execute in: $PROJECT_DIR"
    echo "[dry-run] ${CMD[*]}"
    exit 0
fi

# --- Update .ralphrc with marathon settings (after dry-run gate) ---
if [[ -f "$RALPHRC" ]]; then
    update_ralphrc_key "$RALPHRC" "MAX_CALLS_PER_HOUR" "$CALLS_PER_HOUR"
    update_ralphrc_key "$RALPHRC" "CLAUDE_TIMEOUT_MINUTES" "$TIMEOUT_MINUTES"
fi

# --- Setup ---
# Unset CLAUDECODE to avoid "nested session" detection when launched from Claude Code
unset CLAUDECODE 2>/dev/null || true

cd "$PROJECT_DIR"
mkdir -p .ralph/logs
MARATHON_LOG=".ralph/logs/marathon-$(date '+%Y%m%d-%H%M%S').log"
START_EPOCH=$(date +%s)

log "INFO" "Marathon starting: ${DURATION_HOURS}h, \$$BUDGET budget, ${CALLS_PER_HOUR} calls/hr"
log "INFO" "Command: ${CMD[*]}"

# --- Signal handling ---
cleanup() {
    local exit_code="${1:-0}"
    log "INFO" "Marathon cleanup (exit_code=$exit_code)"
    if [[ -n "$RALPH_PID" ]] && kill -0 "$RALPH_PID" 2>/dev/null; then
        log "INFO" "Sending SIGTERM to ralph (PID $RALPH_PID)"
        kill -TERM "$RALPH_PID" 2>/dev/null || true
        # Wait up to 30s for graceful shutdown
        local waited=0
        while kill -0 "$RALPH_PID" 2>/dev/null && (( waited < 30 )); do
            sleep 1
            ((waited++))
        done
        if kill -0 "$RALPH_PID" 2>/dev/null; then
            log "WARN" "Ralph did not exit after 30s, sending SIGKILL"
            kill -KILL "$RALPH_PID" 2>/dev/null || true
        fi
    fi
    local elapsed=$(( $(date +%s) - START_EPOCH ))
    local hours=$(( elapsed / 3600 ))
    local mins=$(( (elapsed % 3600) / 60 ))
    local spend
    spend=$(read_spend)
    log "INFO" "Marathon finished: ran ${hours}h${mins}m, spent ~\$${spend}"
    exit "$exit_code"
}

trap 'log "INFO" "Caught SIGINT"; cleanup 130' INT
trap 'log "INFO" "Caught SIGTERM"; cleanup 143' TERM

# --- Status file readers (using jq) ---
read_status_field() {
    local field="$1" default="${2:-0}"
    local status_file="$PROJECT_DIR/.ralph/status.json"
    if [[ -f "$status_file" ]]; then
        jq -r ".${field} // ${default}" "$status_file" 2>/dev/null || echo "$default"
    else
        echo "$default"
    fi
}

# Reads session_spend_usd from .ralph/status.json, falls back to call-count estimate
read_spend() {
    local spend
    spend=$(read_status_field "session_spend_usd" "0")
    if [[ "$spend" != "0" ]] && [[ "$spend" != "null" ]] && [[ -n "$spend" ]]; then
        echo "$spend"
        return
    fi
    # Fallback: estimate from loop_count × cost_per_call
    local loops
    loops=$(read_status_field "loop_count" "0")
    echo "scale=2; $loops * $COST_PER_CALL" | bc 2>/dev/null || echo "0"
}

read_loop_count() {
    read_status_field "loop_count" "0"
}

# --- Cost ledger (inspired by hg-mcp pattern) ---
# Appends per-poll cost snapshot to .ralph/logs/cost_ledger.jsonl
LAST_LEDGER_LOOPS=0
write_cost_ledger() {
    local loops="$1" spend="$2" elapsed="$3"
    local ledger="$PROJECT_DIR/.ralph/logs/cost_ledger.jsonl"
    # Only write when loop count changes (new work done)
    if [[ "$loops" == "$LAST_LEDGER_LOOPS" ]]; then
        return
    fi
    LAST_LEDGER_LOOPS="$loops"
    local ts
    ts="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
    local status
    status=$(read_status_field "status" "unknown")
    local model
    model=$(read_status_field "model" "sonnet")
    echo "{\"ts\":\"${ts}\",\"loop\":${loops},\"spend_usd\":${spend},\"elapsed_s\":${elapsed},\"model\":\"${model}\",\"status\":\"${status}\"}" >> "$ledger"
}

# --- Checkpoint logic ---
create_checkpoint() {
    CHECKPOINT_COUNT=$((CHECKPOINT_COUNT + 1))
    local tag="marathon-checkpoint-${CHECKPOINT_COUNT}-$(date '+%Y%m%d-%H%M%S')"
    local elapsed=$(( $(date +%s) - START_EPOCH ))
    local hours=$(( elapsed / 3600 ))
    local spend
    spend=$(read_spend)
    local loops
    loops=$(read_loop_count)

    if git rev-parse --is-inside-work-tree &>/dev/null; then
        # Stage and commit any ralph working changes
        if [[ -n "$(git status --porcelain 2>/dev/null)" ]]; then
            git add -A && git commit -m "marathon checkpoint #${CHECKPOINT_COUNT} (${hours}h, \$${spend}, ${loops} loops)" --no-verify 2>/dev/null || true
        fi
        git tag "$tag" 2>/dev/null || true
        log "CHECKPOINT" "#${CHECKPOINT_COUNT}: tag=$tag, elapsed=${hours}h, spend=\$${spend}, loops=${loops}"
    else
        log "CHECKPOINT" "#${CHECKPOINT_COUNT}: elapsed=${hours}h, spend=\$${spend}, loops=${loops} (not a git repo, skipped tag)"
    fi
}

# --- Resource checks ---
check_disk_space() {
    local min_gb="${MARATHON_MIN_DISK_GB:-5}"
    local avail_kb
    avail_kb=$(df -k "$PROJECT_DIR" 2>/dev/null | awk 'NR==2{print $4}')
    if [ -n "$avail_kb" ] && [ "$avail_kb" -lt $((min_gb * 1024 * 1024)) ]; then
        log "WARN" "Low disk space: $((avail_kb / 1024 / 1024))GB available (minimum: ${min_gb}GB)"
        return 1
    fi
    return 0
}

check_memory() {
    local min_gb="${MARATHON_MIN_MEMORY_GB:-2}"
    local avail_kb=0
    if [ "$(uname)" = "Linux" ]; then
        avail_kb=$(awk '/MemAvailable/{print $2}' /proc/meminfo 2>/dev/null)
    elif [ "$(uname)" = "Darwin" ]; then
        # macOS: use vm_stat page-free count * page size
        local page_size pages_free
        page_size=$(sysctl -n hw.pagesize 2>/dev/null || echo 4096)
        pages_free=$(vm_stat 2>/dev/null | awk '/Pages free/{gsub(/\./,""); print $3}')
        if [ -n "$pages_free" ]; then
            avail_kb=$((pages_free * page_size / 1024))
        fi
    fi
    if [ -n "$avail_kb" ] && [ "$avail_kb" -gt 0 ] && [ "$avail_kb" -lt $((min_gb * 1024 * 1024)) ]; then
        log "WARN" "Low memory: $((avail_kb / 1024 / 1024))GB available (minimum: ${min_gb}GB)"
        return 1
    fi
    return 0
}

# --- Log rotation ---
MAX_LOG_SIZE=$((10 * 1024 * 1024))  # 10MB
rotate_logs() {
    local log_file="$1"
    local max_size="${MARATHON_MAX_LOG_MB:-100}"
    local keep="${MARATHON_LOG_KEEP:-3}"

    [ ! -f "$log_file" ] && return

    # Fast check using stat (cross-platform: macOS -f%z, Linux -c%s)
    local file_size
    file_size=$(stat -f%z "$log_file" 2>/dev/null || stat -c%s "$log_file" 2>/dev/null || echo "0")
    if [ "$file_size" -gt "$MAX_LOG_SIZE" ]; then
        # Gzip compress the current log with timestamp
        gzip -c "$log_file" > "${log_file}.$(date +%Y%m%d%H%M%S).gz"
        : > "$log_file"
        log "INFO" "Log rotated (gzip): ${log_file}"

        # Prune old compressed logs beyond $keep
        local gz_count
        gz_count=$(ls -1 "${log_file}".*.gz 2>/dev/null | wc -l)
        if [ "$gz_count" -gt "$keep" ]; then
            ls -1t "${log_file}".*.gz 2>/dev/null | tail -n +$((keep + 1)) | while read -r old; do
                rm -f "$old"
            done
            log "INFO" "Pruned old log archives (keeping ${keep})"
        fi
        return
    fi

    # Legacy rotation fallback using du (for larger threshold from env var)
    local size_mb
    size_mb=$(du -m "$log_file" 2>/dev/null | cut -f1)
    if [ "${size_mb:-0}" -ge "$max_size" ]; then
        gzip -c "$log_file" > "${log_file}.$(date +%Y%m%d%H%M%S).gz"
        : > "$log_file"
        log "INFO" "Log rotated (size threshold): ${log_file}"
    fi
}

# --- Launch ralph ---
echo "Starting in 3 seconds... (Ctrl+C to cancel)"
sleep 3

log "INFO" "Launching ralph..."
"${CMD[@]}" &
RALPH_PID=$!
log "INFO" "Ralph started with PID $RALPH_PID"

# --- Supervisor loop ---
POLL_INTERVAL=30
last_checkpoint_epoch=$START_EPOCH

while true; do
    sleep "$POLL_INTERVAL" || true

    # --- Log rotation check each iteration ---
    rotate_logs "$MARATHON_LOG"

    # Check if ralph is still running
    if ! kill -0 "$RALPH_PID" 2>/dev/null; then
        ralph_exit=0
        wait "$RALPH_PID" 2>/dev/null || ralph_exit=$?
        log "INFO" "Ralph exited with code $ralph_exit"
        RALPH_PID=""

        # Auto-restart with exponential backoff (inspired by hg-mcp/mesmer patterns)
        if (( ralph_exit != 0 && RESTART_COUNT < MAX_RESTARTS )); then
            RESTART_COUNT=$((RESTART_COUNT + 1))
            CONSECUTIVE_RESTARTS=$((CONSECUTIVE_RESTARTS + 1))
            log "RESTART" "[$(date '+%Y-%m-%d %H:%M:%S')] Ralph failed (exit $ralph_exit). Restart #${RESTART_COUNT}/${MAX_RESTARTS}, consecutive=${CONSECUTIVE_RESTARTS}/${MAX_CONSECUTIVE_RESTARTS}"

            # Cooldown: if consecutive restarts hit the cap, sleep longer and reset
            if (( CONSECUTIVE_RESTARTS >= MAX_CONSECUTIVE_RESTARTS )); then
                log "RESTART" "Hit ${MAX_CONSECUTIVE_RESTARTS} consecutive restarts. Cooling down for ${COOLDOWN_SECONDS}s..."
                sleep "$COOLDOWN_SECONDS" || true
                CONSECUTIVE_RESTARTS=0
                log "RESTART" "Cooldown complete. Consecutive restart counter reset."
            else
                backoff=$((RESTART_BACKOFF * RESTART_COUNT))
                log "RESTART" "Backing off for ${backoff}s before restart..."
                sleep "$backoff" || true
            fi

            log "RESTART" "Relaunching ralph..."
            "${CMD[@]}" &
            RALPH_PID=$!
            log "RESTART" "Ralph restarted with PID $RALPH_PID"
        elif (( ralph_exit == 0 )); then
            log "INFO" "Ralph exited cleanly."
            cleanup 0
        else
            log "ERROR" "Ralph failed $MAX_RESTARTS times. Giving up."
            cleanup "$ralph_exit"
        fi
    fi

    now=$(date +%s)
    elapsed=$(( now - START_EPOCH ))
    elapsed_hours=$(echo "scale=1; $elapsed / 3600" | bc)
    spend=$(read_spend)
    loops=$(read_loop_count)

    # --- Cost ledger ---
    write_cost_ledger "$loops" "$spend" "$elapsed"

    # --- Duration check ---
    if (( elapsed >= duration_seconds )); then
        log "BUDGET" "Duration limit reached (${elapsed_hours}h >= ${DURATION_HOURS}h). Stopping."
        cleanup 0
    fi

    # --- Budget check ---
    if (( $(echo "$spend >= $budget_ceiling" | bc -l 2>/dev/null || echo 1) )); then
        log "BUDGET" "Budget limit reached (\$${spend} >= \$${budget_ceiling}). Stopping."
        cleanup 0
    fi

    # --- Regression gate check ---
    if command -v ralphglasses &>/dev/null; then
        gate_result=$(ralphglasses gate-check --json --hours 1 2>/dev/null || echo '{"overall":"skip"}')
        gate_verdict=$(echo "$gate_result" | jq -r '.overall' 2>/dev/null || echo "skip")
        if [ "$gate_verdict" = "fail" ]; then
            log "REGRESSION" "Regression gate FAILED. Stopping marathon."
            cleanup 2
        elif [ "$gate_verdict" = "warn" ]; then
            log "WARN" "Regression gate warning detected."
        fi
    fi

    # --- Resource checks (at checkpoint interval) ---
    if (( now - last_checkpoint_epoch >= checkpoint_seconds )); then
        check_disk_space || true
        check_memory || true
        rotate_logs "$MARATHON_LOG"

        # --- Checkpoint ---
        create_checkpoint
        last_checkpoint_epoch=$now
    fi

    # Reset restart counters on successful progress
    if (( RESTART_COUNT > 0 )) && [[ "$loops" != "0" ]]; then
        log "INFO" "Ralph recovered after restart. Resetting restart counters."
        RESTART_COUNT=0
        CONSECUTIVE_RESTARTS=0
    fi

    # Periodic status (every 5 minutes = every 10 polls)
    if (( (elapsed / POLL_INTERVAL) % 10 == 0 )); then
        remaining_hours=$(echo "scale=1; ($duration_seconds - $elapsed) / 3600" | bc)
        remaining_budget=$(echo "scale=2; $budget_ceiling - $spend" | bc)
        log "STATUS" "elapsed=${elapsed_hours}h, remaining=${remaining_hours}h, spend=\$${spend}/\$${budget_ceiling}, loops=${loops}"
    fi
done
