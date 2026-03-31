#!/usr/bin/env bash
#
# compositor-detect.sh — Detect running Wayland compositor
#
# Mirrors internal/wm/detect.go at the shell level. Sources this file
# to get $COMPOSITOR set to one of: sway, hyprland, i3, unknown.
#
# Usage:
#   source compositor-detect.sh          # Sets $COMPOSITOR
#   compositor-detect.sh                 # Prints compositor name
#   compositor-detect.sh --test          # Run self-tests with mocked env

set -euo pipefail

# Detect compositor from environment variables and running processes.
# Priority matches Go wm.Detect(): SWAYSOCK > HYPRLAND_INSTANCE_SIGNATURE > I3SOCK > XDG fallback.
detect_compositor() {
    if [ -n "${SWAYSOCK:-}" ]; then
        echo "sway"
    elif [ -n "${HYPRLAND_INSTANCE_SIGNATURE:-}" ]; then
        echo "hyprland"
    elif [ -n "${I3SOCK:-}" ]; then
        echo "i3"
    else
        # Fallback: check XDG_CURRENT_DESKTOP
        local desktop="${XDG_CURRENT_DESKTOP:-}"
        case "${desktop,,}" in
            sway)      echo "sway" ;;
            hyprland)  echo "hyprland" ;;
            i3)        echo "i3" ;;
            *)
                # Last resort: check running processes
                if pgrep -x sway >/dev/null 2>&1; then
                    echo "sway"
                elif pgrep -x Hyprland >/dev/null 2>&1; then
                    echo "hyprland"
                elif pgrep -x i3 >/dev/null 2>&1; then
                    echo "i3"
                else
                    echo "unknown"
                fi
                ;;
        esac
    fi
}

# Self-test mode: verify detection logic with mocked env vars.
run_tests() {
    local pass=0 fail=0

    _assert() {
        local name="$1" expected="$2" actual="$3"
        if [ "$expected" = "$actual" ]; then
            echo "  PASS: $name"
            ((pass++)) || true
        else
            echo "  FAIL: $name (expected=$expected, got=$actual)"
            ((fail++)) || true
        fi
    }

    echo "compositor-detect.sh self-tests"
    echo "================================"

    # Test 1: SWAYSOCK takes priority
    result=$(SWAYSOCK="/run/user/1000/sway-ipc.sock" HYPRLAND_INSTANCE_SIGNATURE="" I3SOCK="" XDG_CURRENT_DESKTOP="" detect_compositor)
    _assert "SWAYSOCK priority" "sway" "$result"

    # Test 2: Hyprland via instance signature
    result=$(SWAYSOCK="" HYPRLAND_INSTANCE_SIGNATURE="abc123" I3SOCK="" XDG_CURRENT_DESKTOP="" detect_compositor)
    _assert "HYPRLAND_INSTANCE_SIGNATURE" "hyprland" "$result"

    # Test 3: i3 via I3SOCK
    result=$(SWAYSOCK="" HYPRLAND_INSTANCE_SIGNATURE="" I3SOCK="/run/user/1000/i3/ipc-socket.1234" XDG_CURRENT_DESKTOP="" detect_compositor)
    _assert "I3SOCK" "i3" "$result"

    # Test 4: XDG_CURRENT_DESKTOP fallback (sway)
    result=$(SWAYSOCK="" HYPRLAND_INSTANCE_SIGNATURE="" I3SOCK="" XDG_CURRENT_DESKTOP="sway" detect_compositor)
    _assert "XDG_CURRENT_DESKTOP=sway" "sway" "$result"

    # Test 5: XDG_CURRENT_DESKTOP fallback (Hyprland, case insensitive)
    result=$(SWAYSOCK="" HYPRLAND_INSTANCE_SIGNATURE="" I3SOCK="" XDG_CURRENT_DESKTOP="Hyprland" detect_compositor)
    _assert "XDG_CURRENT_DESKTOP=Hyprland" "hyprland" "$result"

    # Test 6: SWAYSOCK beats XDG_CURRENT_DESKTOP
    result=$(SWAYSOCK="/tmp/sway.sock" HYPRLAND_INSTANCE_SIGNATURE="" I3SOCK="" XDG_CURRENT_DESKTOP="Hyprland" detect_compositor)
    _assert "SWAYSOCK beats XDG" "sway" "$result"

    echo ""
    echo "Results: $pass passed, $fail failed"
    [ "$fail" -eq 0 ]
}

# When sourced, export COMPOSITOR. When executed directly, print or test.
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
    case "${1:-}" in
        --test) run_tests ;;
        *)      detect_compositor ;;
    esac
else
    COMPOSITOR="$(detect_compositor)"
    export COMPOSITOR
fi
