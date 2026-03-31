#!/usr/bin/env bash
#
# waybar-launch.sh — Launch waybar with compositor-appropriate config
#
# Sources compositor-detect.sh to determine which compositor is running,
# then copies the correct waybar config (sway or hyprland variant) into
# ~/.config/waybar/ and execs waybar.
#
# Usage:
#   waybar-launch.sh             # Detect compositor, install config, exec waybar
#   waybar-launch.sh --dry-run   # Print what would be done, don't exec

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# --- Flags ---
DRY_RUN=false
for arg in "$@"; do
    case "$arg" in
        --dry-run) DRY_RUN=true ;;
    esac
done

# --- Detect compositor ---

# shellcheck source=compositor-detect.sh
source "${SCRIPT_DIR}/compositor-detect.sh"

echo "Detected compositor: ${COMPOSITOR}"

# --- Resolve config source directory ---

# Check installed path first, fall back to repo-relative path
resolve_config_dir() {
    local compositor="$1"
    local installed="/opt/ralphglasses/distro/${compositor}/waybar"
    local repo="${SCRIPT_DIR}/../${compositor}/waybar"

    if [[ -d "$installed" ]]; then
        echo "$installed"
    elif [[ -d "$repo" ]]; then
        echo "$repo"
    else
        echo ""
    fi
}

case "$COMPOSITOR" in
    sway)
        CONFIG_DIR="$(resolve_config_dir sway)"
        ;;
    hyprland)
        CONFIG_DIR="$(resolve_config_dir hyprland)"
        ;;
    *)
        echo "WARNING: Unsupported compositor '${COMPOSITOR}', falling back to sway config" >&2
        CONFIG_DIR="$(resolve_config_dir sway)"
        ;;
esac

if [[ -z "$CONFIG_DIR" ]]; then
    echo "ERROR: Could not find waybar config directory for compositor '${COMPOSITOR}'" >&2
    exit 1
fi

echo "Using waybar config from: ${CONFIG_DIR}"

# --- Install config to ~/.config/waybar/ ---

WAYBAR_DIR="${HOME}/.config/waybar"
CONFIG_SRC="${CONFIG_DIR}/config.jsonc"
STYLE_SRC="${CONFIG_DIR}/style.css"

if [[ ! -f "$CONFIG_SRC" ]]; then
    echo "ERROR: Config file not found: ${CONFIG_SRC}" >&2
    exit 1
fi

if [[ ! -f "$STYLE_SRC" ]]; then
    echo "ERROR: Style file not found: ${STYLE_SRC}" >&2
    exit 1
fi

if [[ "$DRY_RUN" == true ]]; then
    echo "--- DRY RUN ---"
    echo "Would create: ${WAYBAR_DIR}/"
    echo "Would copy:   ${CONFIG_SRC} -> ${WAYBAR_DIR}/config"
    echo "Would copy:   ${STYLE_SRC} -> ${WAYBAR_DIR}/style.css"
    echo "Would exec:   waybar"
    echo "--- END ---"
    exit 0
fi

mkdir -p "$WAYBAR_DIR"
cp -f "$CONFIG_SRC" "${WAYBAR_DIR}/config"
cp -f "$STYLE_SRC" "${WAYBAR_DIR}/style.css"

echo "Installed waybar config to ${WAYBAR_DIR}/"

# --- Launch waybar ---

exec waybar
