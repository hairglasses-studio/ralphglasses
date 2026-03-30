#!/usr/bin/env bash
set -euo pipefail
# ralphglasses watchdog — restarts TUI if it crashes, reports health to systemd

RALPH_PID_FILE="/run/ralphglasses/tui.pid"
RALPH_BIN="/usr/local/bin/ralphglasses"
HEALTH_LOG="/var/log/ralphglasses/watchdog.log"
MAX_RESTARTS=10
RESTART_COUNT=0

log() { echo "$(date -Iseconds) $1" >> "$HEALTH_LOG"; }

mkdir -p "$(dirname "$RALPH_PID_FILE")" "$(dirname "$HEALTH_LOG")"

while true; do
    # Notify systemd we're alive
    systemd-notify WATCHDOG=1 2>/dev/null || true

    if [[ -f "$RALPH_PID_FILE" ]]; then
        PID=$(cat "$RALPH_PID_FILE")
        if kill -0 "$PID" 2>/dev/null; then
            sleep 10
            continue
        fi
    fi

    # Process not running — restart
    RESTART_COUNT=$((RESTART_COUNT + 1))
    if [[ $RESTART_COUNT -gt $MAX_RESTARTS ]]; then
        log "ERROR: Max restarts ($MAX_RESTARTS) exceeded, giving up"
        exit 1
    fi

    log "Starting ralphglasses (restart #$RESTART_COUNT)"
    $RALPH_BIN --kiosk &
    echo $! > "$RALPH_PID_FILE"
    sleep 5
done
