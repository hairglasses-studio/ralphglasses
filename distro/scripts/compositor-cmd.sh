#!/usr/bin/env bash
#
# compositor-cmd.sh — Unified compositor command dispatcher
#
# Wraps Sway (swaymsg) and Hyprland (hyprctl) commands behind a single
# interface. Auto-detects compositor via compositor-detect.sh.
#
# Usage:
#   compositor-cmd.sh workspace 3           # Switch to workspace 3
#   compositor-cmd.sh exec-on 3 alacritty   # Launch alacritty on workspace 3
#   compositor-cmd.sh outputs               # JSON list of monitors/outputs
#   compositor-cmd.sh clients               # JSON list of windows/clients
#   compositor-cmd.sh reload                # Reload compositor config
#   compositor-cmd.sh dpms on|off           # Toggle display power
#   compositor-cmd.sh version               # Compositor version info
#   compositor-cmd.sh fullscreen            # Toggle fullscreen on focused window
#   compositor-cmd.sh move-to-workspace 5   # Move focused window to workspace 5
#   compositor-cmd.sh --test                # Run self-tests with mocked commands

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source compositor detection
# shellcheck source=compositor-detect.sh
if [ -f "${SCRIPT_DIR}/compositor-detect.sh" ]; then
    source "${SCRIPT_DIR}/compositor-detect.sh"
else
    echo "ERROR: compositor-detect.sh not found at ${SCRIPT_DIR}/" >&2
    exit 1
fi

# ── Command implementations ──────────────────────────────────────────

cmd_workspace() {
    local ws="$1"
    case "$COMPOSITOR" in
        sway)     swaymsg "workspace $ws" ;;
        hyprland) hyprctl dispatch workspace "$ws" ;;
        i3)       i3-msg "workspace $ws" ;;
        *)        echo "ERROR: unsupported compositor: $COMPOSITOR" >&2; return 1 ;;
    esac
}

cmd_exec_on() {
    local ws="$1"; shift
    local cmd="$*"
    case "$COMPOSITOR" in
        sway)     swaymsg "workspace $ws; exec $cmd" ;;
        hyprland) hyprctl dispatch workspace "$ws" && hyprctl dispatch exec "$cmd" ;;
        i3)       i3-msg "workspace $ws; exec $cmd" ;;
        *)        echo "ERROR: unsupported compositor: $COMPOSITOR" >&2; return 1 ;;
    esac
}

cmd_outputs() {
    case "$COMPOSITOR" in
        sway)     swaymsg -t get_outputs ;;
        hyprland) hyprctl monitors -j ;;
        i3)       i3-msg -t get_outputs ;;
        *)        echo "ERROR: unsupported compositor: $COMPOSITOR" >&2; return 1 ;;
    esac
}

cmd_clients() {
    case "$COMPOSITOR" in
        sway)     swaymsg -t get_tree ;;
        hyprland) hyprctl clients -j ;;
        i3)       i3-msg -t get_tree ;;
        *)        echo "ERROR: unsupported compositor: $COMPOSITOR" >&2; return 1 ;;
    esac
}

cmd_reload() {
    case "$COMPOSITOR" in
        sway)     swaymsg reload ;;
        hyprland) hyprctl reload ;;
        i3)       i3-msg reload ;;
        *)        echo "ERROR: unsupported compositor: $COMPOSITOR" >&2; return 1 ;;
    esac
}

cmd_dpms() {
    local state="$1"
    case "$COMPOSITOR" in
        sway)     swaymsg "output * dpms $state" ;;
        hyprland) hyprctl dispatch dpms "$state" ;;
        i3)       echo "DPMS not supported on i3/X11 via IPC" >&2; return 1 ;;
        *)        echo "ERROR: unsupported compositor: $COMPOSITOR" >&2; return 1 ;;
    esac
}

cmd_version() {
    case "$COMPOSITOR" in
        sway)     swaymsg -t get_version ;;
        hyprland) hyprctl version -j ;;
        i3)       i3-msg -t get_version ;;
        *)        echo "ERROR: unsupported compositor: $COMPOSITOR" >&2; return 1 ;;
    esac
}

cmd_fullscreen() {
    case "$COMPOSITOR" in
        sway)     swaymsg fullscreen ;;
        hyprland) hyprctl dispatch fullscreen 0 ;;
        i3)       i3-msg fullscreen ;;
        *)        echo "ERROR: unsupported compositor: $COMPOSITOR" >&2; return 1 ;;
    esac
}

cmd_move_to_workspace() {
    local ws="$1"
    case "$COMPOSITOR" in
        sway)     swaymsg "move container to workspace $ws" ;;
        hyprland) hyprctl dispatch movetoworkspacesilent "$ws" ;;
        i3)       i3-msg "move container to workspace $ws" ;;
        *)        echo "ERROR: unsupported compositor: $COMPOSITOR" >&2; return 1 ;;
    esac
}

# ── Self-tests ────────────────────────────────────────────────────────

run_tests() {
    local pass=0 fail=0 tmpdir

    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT

    # Create mock commands that log what they were called with
    cat > "$tmpdir/swaymsg" <<'MOCK'
#!/bin/bash
echo "swaymsg $*" >> "${MOCK_LOG}"
echo '{"success": true}'
MOCK
    cat > "$tmpdir/hyprctl" <<'MOCK'
#!/bin/bash
echo "hyprctl $*" >> "${MOCK_LOG}"
echo "ok"
MOCK
    cat > "$tmpdir/i3-msg" <<'MOCK'
#!/bin/bash
echo "i3-msg $*" >> "${MOCK_LOG}"
echo '{"success": true}'
MOCK
    chmod +x "$tmpdir/swaymsg" "$tmpdir/hyprctl" "$tmpdir/i3-msg"

    export PATH="$tmpdir:$PATH"
    export MOCK_LOG="$tmpdir/calls.log"

    _assert() {
        local name="$1" expected="$2"
        if grep -qF "$expected" "$MOCK_LOG" 2>/dev/null; then
            echo "  PASS: $name"
            ((pass++))
        else
            echo "  FAIL: $name (expected log to contain: $expected)"
            echo "    actual: $(cat "$MOCK_LOG" 2>/dev/null || echo '<empty>')"
            ((fail++))
        fi
        : > "$MOCK_LOG"  # reset
    }

    echo "compositor-cmd.sh self-tests"
    echo "================================"

    # Sway tests
    COMPOSITOR=sway cmd_workspace 3
    _assert "sway workspace" "swaymsg workspace 3"

    COMPOSITOR=sway cmd_exec_on 2 alacritty --class rg-term
    _assert "sway exec-on" "swaymsg workspace 2; exec alacritty --class rg-term"

    COMPOSITOR=sway cmd_reload
    _assert "sway reload" "swaymsg reload"

    COMPOSITOR=sway cmd_dpms on
    _assert "sway dpms" "swaymsg output * dpms on"

    # Hyprland tests
    COMPOSITOR=hyprland cmd_workspace 5
    _assert "hyprland workspace" "hyprctl dispatch workspace 5"

    COMPOSITOR=hyprland cmd_reload
    _assert "hyprland reload" "hyprctl reload"

    COMPOSITOR=hyprland cmd_dpms off
    _assert "hyprland dpms" "hyprctl dispatch dpms off"

    COMPOSITOR=hyprland cmd_fullscreen
    _assert "hyprland fullscreen" "hyprctl dispatch fullscreen 0"

    # i3 tests
    COMPOSITOR=i3 cmd_workspace 1
    _assert "i3 workspace" "i3-msg workspace 1"

    COMPOSITOR=i3 cmd_reload
    _assert "i3 reload" "i3-msg reload"

    echo ""
    echo "Results: $pass passed, $fail failed"
    [ "$fail" -eq 0 ]
}

# ── Usage ─────────────────────────────────────────────────────────────

usage() {
    cat <<EOF
Usage: $(basename "$0") <command> [args...]

Commands:
  workspace <N>              Switch to workspace N
  exec-on <N> <cmd...>      Launch command on workspace N
  outputs                    JSON list of monitors/outputs
  clients                    JSON list of windows/clients
  reload                     Reload compositor config
  dpms <on|off>              Toggle display power management
  version                    Compositor version info (JSON)
  fullscreen                 Toggle fullscreen on focused window
  move-to-workspace <N>      Move focused window to workspace N

Detected compositor: ${COMPOSITOR:-unknown}
EOF
}

# ── Main ──────────────────────────────────────────────────────────────

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
    if [ $# -eq 0 ]; then
        usage
        exit 1
    fi

    subcmd="$1"; shift
    case "$subcmd" in
        workspace)          cmd_workspace "$@" ;;
        exec-on)            cmd_exec_on "$@" ;;
        outputs)            cmd_outputs ;;
        clients)            cmd_clients ;;
        reload)             cmd_reload ;;
        dpms)               cmd_dpms "$@" ;;
        version)            cmd_version ;;
        fullscreen)         cmd_fullscreen ;;
        move-to-workspace)  cmd_move_to_workspace "$@" ;;
        --test)             run_tests ;;
        --help|-h)          usage ;;
        *)
            echo "ERROR: Unknown command: $subcmd" >&2
            usage
            exit 1
            ;;
    esac
fi
