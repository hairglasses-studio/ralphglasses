#!/usr/bin/env bash
#
# compositor-kiosk-setup.sh — Install kiosk mode for any supported compositor
#
# Generalizes sway-kiosk-setup.sh to work with Sway and Hyprland.
# Uses compositor-detect.sh for auto-detection, or accepts --compositor flag.
#
# Usage:
#   sudo ./compositor-kiosk-setup.sh                          # Auto-detect, install
#   sudo ./compositor-kiosk-setup.sh --compositor hyprland    # Force Hyprland
#   sudo ./compositor-kiosk-setup.sh --reconfigure            # Re-apply config
#   sudo ./compositor-kiosk-setup.sh --disable                # Switch to normal config
#   sudo ./compositor-kiosk-setup.sh --status                 # Show current state
#   sudo ./compositor-kiosk-setup.sh --dry-run                # Print actions only

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ── Constants ────────────────────────────────────────────────────────
RALPH_USER="ralph"
RALPH_HOME="/home/${RALPH_USER}"
BASH_PROFILE="${RALPH_HOME}/.bash_profile"
GETTY_OVERRIDE_DIR="/etc/systemd/system/getty@tty1.service.d"
GETTY_OVERRIDE="${GETTY_OVERRIDE_DIR}/autologin.conf"
KIOSK_STATE_FILE="/etc/ralphglasses/kiosk.state"
DRY_RUN=false
MODE="install"
COMPOSITOR_OVERRIDE=""

# ── Colors ───────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log()  { echo -e "${GREEN}[kiosk]${NC} $*"; }
warn() { echo -e "${YELLOW}[kiosk]${NC} $*" >&2; }
err()  { echo -e "${RED}[kiosk]${NC} $*" >&2; }
info() { echo -e "${BLUE}[kiosk]${NC} $*"; }

# ── Argument parsing ─────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        --compositor)    COMPOSITOR_OVERRIDE="$2"; shift 2 ;;
        --reconfigure)   MODE="reconfigure"; shift ;;
        --disable)       MODE="disable";     shift ;;
        --status)        MODE="status";      shift ;;
        --dry-run)       DRY_RUN=true;       shift ;;
        --help|-h)
            echo "Usage: sudo $0 [--compositor sway|hyprland] [--reconfigure|--disable|--status|--dry-run]"
            exit 0
            ;;
        *)
            err "Unknown option: $1"
            exit 1
            ;;
    esac
done

# ── Detect compositor ────────────────────────────────────────────────
if [ -n "$COMPOSITOR_OVERRIDE" ]; then
    COMPOSITOR="$COMPOSITOR_OVERRIDE"
else
    # shellcheck source=compositor-detect.sh
    source "${SCRIPT_DIR}/compositor-detect.sh"
    # If running as root during install (no compositor running yet), default to sway
    if [ "$COMPOSITOR" = "unknown" ]; then
        COMPOSITOR="sway"
        warn "No compositor detected (expected during install). Defaulting to: sway"
    fi
fi

# Validate compositor choice
case "$COMPOSITOR" in
    sway|hyprland) ;;
    *)
        err "Unsupported compositor: $COMPOSITOR (supported: sway, hyprland)"
        exit 1
        ;;
esac

log "Compositor: $COMPOSITOR"

# ── Compositor-specific paths ────────────────────────────────────────

case "$COMPOSITOR" in
    sway)
        CONFIG_DIR="${RALPH_HOME}/.config/sway"
        CONFIG_TARGET="${CONFIG_DIR}/config"
        KIOSK_SOURCE="/opt/ralphglasses/distro/sway/kiosk-config"
        NORMAL_SOURCE="/opt/ralphglasses/distro/sway/config"
        KIOSK_SOURCE_REPO="${SCRIPT_DIR}/../sway/kiosk-config"
        NORMAL_SOURCE_REPO="${SCRIPT_DIR}/../sway/config"
        ENV_SOURCE="/opt/ralphglasses/distro/sway/environment.d/nvidia-wayland.conf"
        ENV_SOURCE_REPO="${SCRIPT_DIR}/../sway/environment.d/nvidia-wayland.conf"
        COMPOSITOR_EXEC="sway"
        KIOSK_SERVICE_NAME="ralphglasses-kiosk-sway"
        ;;
    hyprland)
        CONFIG_DIR="${RALPH_HOME}/.config/hypr"
        CONFIG_TARGET="${CONFIG_DIR}/hyprland.conf"
        KIOSK_SOURCE="/opt/ralphglasses/distro/hyprland/kiosk.conf"
        NORMAL_SOURCE="/opt/ralphglasses/distro/hyprland/hyprland.conf"
        KIOSK_SOURCE_REPO="${SCRIPT_DIR}/../hyprland/kiosk.conf"
        NORMAL_SOURCE_REPO="${SCRIPT_DIR}/../hyprland/hyprland.conf"
        ENV_SOURCE="/opt/ralphglasses/distro/hyprland/environment.d/nvidia-wayland.conf"
        ENV_SOURCE_REPO="${SCRIPT_DIR}/../hyprland/environment.d/nvidia-wayland.conf"
        COMPOSITOR_EXEC="Hyprland"
        KIOSK_SERVICE_NAME="ralphglasses-kiosk-hyprland"
        ;;
esac

BACKUP_DIR="${CONFIG_DIR}/backup"
NVIDIA_ENV_DIR="${RALPH_HOME}/.config/environment.d"
KIOSK_SERVICE="/etc/systemd/system/${KIOSK_SERVICE_NAME}.service"

# ── Helpers ──────────────────────────────────────────────────────────

run() {
    if [[ "$DRY_RUN" = true ]]; then
        info "[dry-run] $*"
    else
        "$@"
    fi
}

write_file() {
    local dest="$1" content="$2" mode="${3:-644}"
    if [[ "$DRY_RUN" = true ]]; then
        info "[dry-run] write ${dest} (mode ${mode})"
        return
    fi
    mkdir -p "$(dirname "$dest")"
    echo "$content" > "$dest"
    chmod "$mode" "$dest"
}

resolve_source() {
    local installed="$1" repo="$2"
    if [[ -f "$installed" ]]; then
        echo "$installed"
    elif [[ -f "$repo" ]]; then
        echo "$repo"
    else
        echo ""
    fi
}

# ── Preflight checks ────────────────────────────────────────────────

preflight() {
    if [[ $EUID -ne 0 ]] && [[ "$DRY_RUN" = false ]]; then
        err "Must run as root (or use --dry-run)"
        exit 1
    fi
    if ! id "$RALPH_USER" &>/dev/null && [[ "$DRY_RUN" = false ]]; then
        err "User '${RALPH_USER}' does not exist."
        exit 1
    fi
    if [[ "$COMPOSITOR" = "sway" ]] && ! command -v sway &>/dev/null && [[ "$DRY_RUN" = false ]]; then
        err "Sway not installed. Install: pacman -S sway"
        exit 1
    fi
    if [[ "$COMPOSITOR" = "hyprland" ]] && ! command -v Hyprland &>/dev/null && [[ "$DRY_RUN" = false ]]; then
        err "Hyprland not installed. Install: pacman -S hyprland"
        exit 1
    fi
}

# ── Status ───────────────────────────────────────────────────────────

show_status() {
    echo "=== Ralphglasses Kiosk Status ==="
    echo ""

    if [[ -f "$KIOSK_STATE_FILE" ]]; then
        echo "State file:"
        cat "$KIOSK_STATE_FILE"
    else
        echo "State file: not found (kiosk not installed)"
    fi
    echo ""

    # Check compositor config
    if [[ -f "$CONFIG_TARGET" ]]; then
        if head -1 "$CONFIG_TARGET" | grep -qi "kiosk"; then
            echo "Config ($COMPOSITOR): KIOSK mode"
        else
            echo "Config ($COMPOSITOR): NORMAL mode"
        fi
    else
        echo "Config ($COMPOSITOR): not found"
    fi

    # Auto-login
    if [[ -f "$GETTY_OVERRIDE" ]]; then
        echo "Auto-login:  ENABLED (getty@tty1)"
    else
        echo "Auto-login:  DISABLED"
    fi

    # NVIDIA env
    if [[ -f "${NVIDIA_ENV_DIR}/nvidia-wayland.conf" ]]; then
        echo "NVIDIA env:  present"
    else
        echo "NVIDIA env:  not found"
    fi

    # Service
    if [[ -f "$KIOSK_SERVICE" ]]; then
        local state
        state=$(systemctl is-enabled "${KIOSK_SERVICE_NAME}.service" 2>/dev/null || echo "unknown")
        echo "Service:     ${state}"
    else
        echo "Service:     not installed"
    fi

    # Binary
    if [[ -x /usr/local/bin/ralphglasses ]]; then
        echo "Binary:      present"
    else
        echo "Binary:      NOT FOUND"
    fi

    echo ""
}

# ── Install ──────────────────────────────────────────────────────────

backup_config() {
    if [[ -f "$CONFIG_TARGET" ]] && [[ "$DRY_RUN" = false ]]; then
        mkdir -p "$BACKUP_DIR"
        local ts
        ts=$(date +%Y%m%d-%H%M%S)
        cp "$CONFIG_TARGET" "${BACKUP_DIR}/config.${ts}"
        log "Backed up to ${BACKUP_DIR}/config.${ts}"
    fi
}

install_config() {
    log "Installing kiosk config ($COMPOSITOR)..."
    backup_config

    local src
    src=$(resolve_source "$KIOSK_SOURCE" "$KIOSK_SOURCE_REPO")
    if [[ -z "$src" ]]; then
        err "Kiosk config not found for $COMPOSITOR"
        exit 1
    fi

    if [[ "$DRY_RUN" = false ]]; then
        mkdir -p "$CONFIG_DIR"
        cp "$src" "$CONFIG_TARGET"
        chown -R "${RALPH_USER}:${RALPH_USER}" "$CONFIG_DIR"
    else
        info "[dry-run] cp $src -> $CONFIG_TARGET"
    fi

    # Also install hypridle/hyprlock configs for Hyprland
    if [[ "$COMPOSITOR" = "hyprland" ]]; then
        for cfg in hypridle.conf hyprlock.conf; do
            local cfg_src
            cfg_src=$(resolve_source "/opt/ralphglasses/distro/hyprland/$cfg" "${SCRIPT_DIR}/../hyprland/$cfg")
            if [[ -n "$cfg_src" ]]; then
                if [[ "$DRY_RUN" = false ]]; then
                    cp "$cfg_src" "${CONFIG_DIR}/$cfg"
                else
                    info "[dry-run] cp $cfg_src -> ${CONFIG_DIR}/$cfg"
                fi
            fi
        done
    fi
}

install_nvidia_env() {
    log "Installing NVIDIA Wayland environment ($COMPOSITOR)..."

    local src
    src=$(resolve_source "$ENV_SOURCE" "$ENV_SOURCE_REPO")
    if [[ -z "$src" ]]; then
        warn "nvidia-wayland.conf not found for $COMPOSITOR"
        return
    fi

    if [[ "$DRY_RUN" = false ]]; then
        mkdir -p "$NVIDIA_ENV_DIR"
        cp "$src" "${NVIDIA_ENV_DIR}/nvidia-wayland.conf"
        chown -R "${RALPH_USER}:${RALPH_USER}" "$NVIDIA_ENV_DIR"
    else
        info "[dry-run] install nvidia-wayland.conf"
    fi
}

setup_autologin() {
    log "Configuring auto-login for ${RALPH_USER} on tty1..."

    local content="[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin ${RALPH_USER} --noclear %I \$TERM
Type=idle"

    if [[ "$DRY_RUN" = false ]]; then
        mkdir -p "$GETTY_OVERRIDE_DIR"
        echo "$content" > "$GETTY_OVERRIDE"
        chmod 644 "$GETTY_OVERRIDE"
        run systemctl daemon-reload
    else
        info "[dry-run] write ${GETTY_OVERRIDE}"
    fi
}

setup_bash_profile() {
    log "Configuring bash_profile to auto-start ${COMPOSITOR_EXEC} on tty1..."

    local start_block="# ralphglasses kiosk — auto-start ${COMPOSITOR_EXEC} on tty1
if [ -z \"\$WAYLAND_DISPLAY\" ] && [ \"\$(tty)\" = \"/dev/tty1\" ]; then
    exec ${COMPOSITOR_EXEC}
fi"

    if [[ "$DRY_RUN" = false ]]; then
        # Remove existing kiosk blocks
        if [[ -f "$BASH_PROFILE" ]]; then
            sed -i '/# ralphglasses kiosk/,/^fi$/d' "$BASH_PROFILE"
            sed -i '/^if \[ -z "\$WAYLAND_DISPLAY" \]/,/^fi$/d' "$BASH_PROFILE"
        else
            touch "$BASH_PROFILE"
        fi
        echo "" >> "$BASH_PROFILE"
        echo "$start_block" >> "$BASH_PROFILE"
        chown "${RALPH_USER}:${RALPH_USER}" "$BASH_PROFILE"
    else
        info "[dry-run] append ${COMPOSITOR_EXEC} block to ${BASH_PROFILE}"
    fi
}

setup_systemd_service() {
    log "Installing ${KIOSK_SERVICE_NAME} systemd service..."

    # Use compositor-cmd.sh for compositor-agnostic process checking
    local service_content="[Unit]
Description=Ralphglasses Kiosk Watchdog (${COMPOSITOR})
After=graphical-session.target
Wants=graphical-session.target

[Service]
Type=simple
User=${RALPH_USER}
Environment=WAYLAND_DISPLAY=wayland-1
Environment=XDG_RUNTIME_DIR=/run/user/1000
Environment=RALPHGLASSES_SCAN_PATH=/workspace
ExecStart=/bin/bash -c 'while true; do \\
    if ! pgrep -u ${RALPH_USER} -f \"ralphglasses --scan-path\" >/dev/null; then \\
        logger -t ${KIOSK_SERVICE_NAME} \"No ralphglasses process found\"; \\
    fi; \\
    sleep 30; \\
done'
Restart=always
RestartSec=10

[Install]
WantedBy=graphical-session.target"

    if [[ "$DRY_RUN" = false ]]; then
        echo "$service_content" > "$KIOSK_SERVICE"
        chmod 644 "$KIOSK_SERVICE"
        systemctl daemon-reload
        systemctl enable "${KIOSK_SERVICE_NAME}.service"
    else
        info "[dry-run] write ${KIOSK_SERVICE}"
    fi
}

write_state() {
    local state="$1"
    if [[ "$DRY_RUN" = false ]]; then
        mkdir -p "$(dirname "$KIOSK_STATE_FILE")"
        cat > "$KIOSK_STATE_FILE" <<EOF
mode=${state}
compositor=${COMPOSITOR}
timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
user=${RALPH_USER}
config=${CONFIG_TARGET}
EOF
    else
        info "[dry-run] write state: ${state}, compositor=${COMPOSITOR}"
    fi
}

# ── Disable ──────────────────────────────────────────────────────────

disable_kiosk() {
    log "Disabling kiosk mode ($COMPOSITOR)..."
    backup_config

    local src
    src=$(resolve_source "$NORMAL_SOURCE" "$NORMAL_SOURCE_REPO")
    if [[ -n "$src" ]]; then
        if [[ "$DRY_RUN" = false ]]; then
            cp "$src" "$CONFIG_TARGET"
            chown -R "${RALPH_USER}:${RALPH_USER}" "$CONFIG_DIR"
        else
            info "[dry-run] cp $src -> $CONFIG_TARGET"
        fi
    else
        warn "Normal config not found; removing kiosk config"
        [[ "$DRY_RUN" = false ]] && rm -f "$CONFIG_TARGET"
    fi

    if [[ -f "$KIOSK_SERVICE" ]]; then
        run systemctl disable "${KIOSK_SERVICE_NAME}.service" 2>/dev/null || true
        run systemctl stop "${KIOSK_SERVICE_NAME}.service" 2>/dev/null || true
    fi

    write_state "disabled"

    case "$COMPOSITOR" in
        sway)     log "Run 'swaymsg reload' or reboot to apply." ;;
        hyprland) log "Run 'hyprctl reload' or reboot to apply." ;;
    esac
}

# ── Main install flow ────────────────────────────────────────────────

install_kiosk() {
    local label
    [[ "$MODE" = "reconfigure" ]] && label="Reconfiguring" || label="Installing"

    log "${label} ralphglasses kiosk mode (${COMPOSITOR})..."
    echo ""

    install_config
    install_nvidia_env

    if [[ "$MODE" = "install" ]]; then
        setup_autologin
        setup_bash_profile
    else
        log "Skipping autologin/bash_profile (reconfigure mode)"
    fi

    setup_systemd_service
    write_state "enabled"

    echo ""
    log "Kiosk mode ${label,,} complete."
    log ""
    log "  Emergency exit:    Ctrl+Alt+Backspace"
    log "  Switch to normal:  Mod+Escape"
    log ""
    [[ "$MODE" = "install" ]] && log "Reboot to enter kiosk mode." || log "Reload config to apply."
}

# ── Entrypoint ───────────────────────────────────────────────────────
preflight

case "$MODE" in
    install|reconfigure) install_kiosk ;;
    disable)             disable_kiosk ;;
    status)              show_status ;;
esac
