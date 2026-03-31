#!/usr/bin/env bash
#
# sway-kiosk-setup.sh — Install Sway kiosk mode for ralphglasses thin client
#
# Sets up auto-login → sway → ralphglasses TUI fullscreen on all
# 7 monitors. Handles both first-boot and reconfigure scenarios.
#
# Usage:
#   sudo ./sway-kiosk-setup.sh                # First-boot install
#   sudo ./sway-kiosk-setup.sh --reconfigure  # Re-apply config (preserves user data)
#   sudo ./sway-kiosk-setup.sh --disable      # Switch back to normal Sway config
#   sudo ./sway-kiosk-setup.sh --status       # Show current kiosk state
#   sudo ./sway-kiosk-setup.sh --dry-run      # Print actions without writing
#
# Requires: root, sway installed, ralphglasses binary at /usr/local/bin/
# See: distro/sway/kiosk-config, distro/hardware/proart-x870e.md

set -euo pipefail

# ── Constants ────────────────────────────────────────────────────────
RALPH_USER="ralph"
RALPH_HOME="/home/${RALPH_USER}"
SWAY_CONFIG_DIR="${RALPH_HOME}/.config/sway"
KIOSK_SOURCE="/opt/ralphglasses/distro/sway/kiosk-config"
NORMAL_SOURCE="/opt/ralphglasses/distro/sway/config"
SWAY_TARGET="${SWAY_CONFIG_DIR}/config"
BASH_PROFILE="${RALPH_HOME}/.bash_profile"
GETTY_OVERRIDE_DIR="/etc/systemd/system/getty@tty1.service.d"
GETTY_OVERRIDE="${GETTY_OVERRIDE_DIR}/autologin.conf"
KIOSK_SERVICE="/etc/systemd/system/ralphglasses-kiosk.service"
KIOSK_STATE_FILE="/etc/ralphglasses/kiosk.state"
BACKUP_DIR="${RALPH_HOME}/.config/sway/backup"
NVIDIA_ENV_DIR="${RALPH_HOME}/.config/environment.d"
NVIDIA_ENV_SOURCE="/opt/ralphglasses/distro/sway/environment.d/nvidia-wayland.conf"
DRY_RUN=false
MODE="install"

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
        --reconfigure) MODE="reconfigure"; shift ;;
        --disable)     MODE="disable";     shift ;;
        --status)      MODE="status";      shift ;;
        --dry-run)     DRY_RUN=true;       shift ;;
        --help|-h)
            echo "Usage: sudo $0 [--reconfigure|--disable|--status|--dry-run]"
            exit 0
            ;;
        *)
            err "Unknown option: $1"
            exit 1
            ;;
    esac
done

# ── Preflight checks ────────────────────────────────────────────────
preflight() {
    if [[ $EUID -ne 0 ]] && [[ "$DRY_RUN" = false ]]; then
        err "Must run as root (or use --dry-run)"
        exit 1
    fi

    if ! id "$RALPH_USER" &>/dev/null && [[ "$DRY_RUN" = false ]]; then
        err "User '${RALPH_USER}' does not exist. Create it first."
        exit 1
    fi

    if ! command -v sway &>/dev/null && [[ "$DRY_RUN" = false ]]; then
        err "Sway is not installed. Install with: pacman -S sway"
        exit 1
    fi
}

# ── Dry-run wrapper ──────────────────────────────────────────────────
run() {
    if [[ "$DRY_RUN" = true ]]; then
        info "[dry-run] $*"
    else
        "$@"
    fi
}

write_file() {
    local dest="$1"
    local content="$2"
    local mode="${3:-644}"
    if [[ "$DRY_RUN" = true ]]; then
        info "[dry-run] write ${dest} (mode ${mode})"
        return
    fi
    mkdir -p "$(dirname "$dest")"
    echo "$content" > "$dest"
    chmod "$mode" "$dest"
}

# ── Status ───────────────────────────────────────────────────────────
show_status() {
    echo "=== Ralphglasses Kiosk Status (Sway) ==="
    echo ""

    # Check state file
    if [[ -f "$KIOSK_STATE_FILE" ]]; then
        echo "State file: $(cat "$KIOSK_STATE_FILE")"
    else
        echo "State file: not found (kiosk not installed)"
    fi

    # Check which sway config is active
    if [[ -f "$SWAY_TARGET" ]]; then
        if head -1 "$SWAY_TARGET" | grep -q "kiosk"; then
            echo "Sway config: KIOSK mode"
        else
            echo "Sway config: NORMAL mode"
        fi
    else
        echo "Sway config: not found"
    fi

    # Check auto-login
    if [[ -f "$GETTY_OVERRIDE" ]]; then
        echo "Auto-login:  ENABLED (getty@tty1)"
    else
        echo "Auto-login:  DISABLED"
    fi

    # Check NVIDIA Wayland env
    if [[ -f "${NVIDIA_ENV_DIR}/nvidia-wayland.conf" ]]; then
        echo "NVIDIA env:  present"
    else
        echo "NVIDIA env:  not found"
    fi

    # Check systemd service
    if [[ -f "$KIOSK_SERVICE" ]]; then
        local state
        state=$(systemctl is-enabled ralphglasses-kiosk.service 2>/dev/null || echo "unknown")
        echo "Service:     ${state}"
    else
        echo "Service:     not installed"
    fi

    # Check binary
    if [[ -x /usr/local/bin/ralphglasses ]]; then
        echo "Binary:      /usr/local/bin/ralphglasses (present)"
    else
        echo "Binary:      NOT FOUND — build and install first"
    fi

    echo ""
}

# ── Backup existing config ───────────────────────────────────────────
backup_config() {
    if [[ -f "$SWAY_TARGET" ]] && [[ "$DRY_RUN" = false ]]; then
        mkdir -p "$BACKUP_DIR"
        local ts
        ts=$(date +%Y%m%d-%H%M%S)
        cp "$SWAY_TARGET" "${BACKUP_DIR}/config.${ts}"
        log "Backed up current config to ${BACKUP_DIR}/config.${ts}"
    fi
}

# ── Install kiosk Sway config ────────────────────────────────────────
install_sway_config() {
    log "Installing kiosk Sway config..."
    backup_config

    if [[ "$DRY_RUN" = false ]]; then
        mkdir -p "$SWAY_CONFIG_DIR"
        if [[ -f "$KIOSK_SOURCE" ]]; then
            cp "$KIOSK_SOURCE" "$SWAY_TARGET"
        else
            # Fallback: copy from repo relative path (for development)
            local script_dir
            script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
            local repo_kiosk="${script_dir}/../sway/kiosk-config"
            if [[ -f "$repo_kiosk" ]]; then
                cp "$repo_kiosk" "$SWAY_TARGET"
            else
                err "Kiosk config not found at ${KIOSK_SOURCE} or ${repo_kiosk}"
                exit 1
            fi
        fi
        chown -R "${RALPH_USER}:${RALPH_USER}" "$SWAY_CONFIG_DIR"
    else
        info "[dry-run] cp ${KIOSK_SOURCE} -> ${SWAY_TARGET}"
    fi
}

# ── Install NVIDIA Wayland environment ────────────────────────────────
install_nvidia_env() {
    log "Installing NVIDIA Wayland environment variables..."

    if [[ "$DRY_RUN" = false ]]; then
        mkdir -p "$NVIDIA_ENV_DIR"
        if [[ -f "$NVIDIA_ENV_SOURCE" ]]; then
            cp "$NVIDIA_ENV_SOURCE" "${NVIDIA_ENV_DIR}/nvidia-wayland.conf"
        else
            local script_dir
            script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
            local repo_env="${script_dir}/../sway/environment.d/nvidia-wayland.conf"
            if [[ -f "$repo_env" ]]; then
                cp "$repo_env" "${NVIDIA_ENV_DIR}/nvidia-wayland.conf"
            else
                warn "nvidia-wayland.conf not found — NVIDIA may not work correctly"
                return
            fi
        fi
        chown -R "${RALPH_USER}:${RALPH_USER}" "$NVIDIA_ENV_DIR"
    else
        info "[dry-run] install nvidia-wayland.conf -> ${NVIDIA_ENV_DIR}/"
    fi
}

# ── Configure auto-login on tty1 ────────────────────────────────────
setup_autologin() {
    log "Configuring auto-login for ${RALPH_USER} on tty1..."

    local content
    content="[Service]
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

# ── Configure bash_profile to auto-start Sway on tty1 ────────────────
setup_bash_profile() {
    log "Configuring bash_profile for auto-start Sway..."

    local sway_block='# ralphglasses kiosk — auto-start Sway on tty1
if [ -z "$WAYLAND_DISPLAY" ] && [ "$(tty)" = "/dev/tty1" ]; then
    exec sway
fi'

    if [[ "$DRY_RUN" = false ]]; then
        # Remove any existing kiosk/startx/sway blocks
        if [[ -f "$BASH_PROFILE" ]]; then
            sed -i '/# ralphglasses kiosk/,/^fi$/d' "$BASH_PROFILE"
            # Also strip old i3/startx blocks from previous setup
            sed -i '/^if \[ -z "\$DISPLAY" \]/,/^fi$/d' "$BASH_PROFILE"
            sed -i '/^if \[ -z "\$WAYLAND_DISPLAY" \]/,/^fi$/d' "$BASH_PROFILE"
        else
            touch "$BASH_PROFILE"
        fi
        echo "" >> "$BASH_PROFILE"
        echo "$sway_block" >> "$BASH_PROFILE"
        chown "${RALPH_USER}:${RALPH_USER}" "$BASH_PROFILE"
    else
        info "[dry-run] append sway block to ${BASH_PROFILE}"
    fi
}

# ── Create systemd watchdog service ──────────────────────────────────
setup_systemd_service() {
    log "Installing ralphglasses-kiosk systemd service..."

    local service_content="[Unit]
Description=Ralphglasses Kiosk Watchdog
After=graphical-session.target
Wants=graphical-session.target
Documentation=https://github.com/hairglasses-studio/ralphglasses

[Service]
Type=simple
User=${RALPH_USER}
Environment=WAYLAND_DISPLAY=wayland-1
Environment=XDG_RUNTIME_DIR=/run/user/1000
Environment=SWAYSOCK=/run/user/1000/sway-ipc.sock
Environment=RALPHGLASSES_SCAN_PATH=/workspace
ExecStart=/bin/bash -c 'while true; do \\
    if ! pgrep -u ${RALPH_USER} -f \"ralphglasses --scan-path\" >/dev/null; then \\
        logger -t ralphglasses-kiosk \"No ralphglasses process found, Sway watchdog should restart\"; \\
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
        systemctl enable ralphglasses-kiosk.service
    else
        info "[dry-run] write ${KIOSK_SERVICE}"
        info "[dry-run] systemctl enable ralphglasses-kiosk.service"
    fi
}

# ── Write state file ────────────────────────────────────────────────
write_state() {
    local state="$1"
    if [[ "$DRY_RUN" = false ]]; then
        mkdir -p "$(dirname "$KIOSK_STATE_FILE")"
        cat > "$KIOSK_STATE_FILE" <<EOF
mode=${state}
compositor=sway
timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
user=${RALPH_USER}
config=${SWAY_TARGET}
EOF
    else
        info "[dry-run] write state: ${state} -> ${KIOSK_STATE_FILE}"
    fi
}

# ── Disable kiosk mode ──────────────────────────────────────────────
disable_kiosk() {
    log "Disabling kiosk mode..."
    backup_config

    # Restore normal Sway config
    if [[ "$DRY_RUN" = false ]]; then
        if [[ -f "$NORMAL_SOURCE" ]]; then
            cp "$NORMAL_SOURCE" "$SWAY_TARGET"
        else
            local script_dir
            script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
            local repo_normal="${script_dir}/../sway/config"
            if [[ -f "$repo_normal" ]]; then
                cp "$repo_normal" "$SWAY_TARGET"
            else
                warn "Normal config not found; removing kiosk config only"
                rm -f "$SWAY_TARGET"
            fi
        fi
        chown -R "${RALPH_USER}:${RALPH_USER}" "$SWAY_CONFIG_DIR"
    else
        info "[dry-run] cp ${NORMAL_SOURCE} -> ${SWAY_TARGET}"
    fi

    # Disable kiosk service
    if [[ -f "$KIOSK_SERVICE" ]]; then
        run systemctl disable ralphglasses-kiosk.service 2>/dev/null || true
        run systemctl stop ralphglasses-kiosk.service 2>/dev/null || true
    fi

    write_state "disabled"
    log "Kiosk mode disabled. Run 'swaymsg reload' or reboot to apply."
}

# ── Main install flow ────────────────────────────────────────────────
install_kiosk() {
    local label
    if [[ "$MODE" = "reconfigure" ]]; then
        label="Reconfiguring"
    else
        label="Installing"
    fi

    log "${label} ralphglasses kiosk mode (Sway/Wayland)..."
    echo ""

    # First-boot: full setup including autologin and env
    # Reconfigure: only update Sway config and service
    install_sway_config
    install_nvidia_env

    if [[ "$MODE" = "install" ]]; then
        setup_autologin
        setup_bash_profile
    else
        log "Skipping autologin/bash_profile setup (reconfigure mode)"
    fi

    setup_systemd_service
    write_state "enabled"

    echo ""
    log "Kiosk mode ${label,,} complete."
    log ""
    log "  Emergency exit:    Ctrl+Alt+Backspace"
    log "  Switch to normal:  Mod+Escape"
    log "  Reload config:     Mod+Shift+c"
    log ""

    if [[ "$MODE" = "install" ]]; then
        log "Reboot to enter kiosk mode, or run: swaymsg reload"
    else
        log "Run: swaymsg reload"
    fi
}

# ── Entrypoint ───────────────────────────────────────────────────────
preflight

case "$MODE" in
    install|reconfigure)
        install_kiosk
        ;;
    disable)
        disable_kiosk
        ;;
    status)
        show_status
        ;;
esac
