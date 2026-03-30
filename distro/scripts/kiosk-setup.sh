#!/usr/bin/env bash
#
# kiosk-setup.sh — Install i3 kiosk mode for ralphglasses thin client
#
# Sets up auto-login → startx → i3 → ralphglasses TUI fullscreen on all
# 7 monitors. Handles both first-boot and reconfigure scenarios.
#
# Usage:
#   sudo ./kiosk-setup.sh                # First-boot install
#   sudo ./kiosk-setup.sh --reconfigure  # Re-apply config (preserves user data)
#   sudo ./kiosk-setup.sh --disable      # Switch back to normal i3 config
#   sudo ./kiosk-setup.sh --status       # Show current kiosk state
#   sudo ./kiosk-setup.sh --dry-run      # Print actions without writing
#
# Requires: root, i3 installed, ralphglasses binary at /usr/local/bin/
# See: distro/i3/kiosk-config, distro/hardware/proart-x870e.md

set -euo pipefail

# ── Constants ────────────────────────────────────────────────────────
RALPH_USER="ralph"
RALPH_HOME="/home/${RALPH_USER}"
I3_CONFIG_DIR="${RALPH_HOME}/.config/i3"
KIOSK_SOURCE="/opt/ralphglasses/distro/i3/kiosk-config"
NORMAL_SOURCE="/opt/ralphglasses/distro/i3/config"
I3_TARGET="${I3_CONFIG_DIR}/config"
XINITRC="${RALPH_HOME}/.xinitrc"
BASH_PROFILE="${RALPH_HOME}/.bash_profile"
GETTY_OVERRIDE_DIR="/etc/systemd/system/getty@tty1.service.d"
GETTY_OVERRIDE="${GETTY_OVERRIDE_DIR}/autologin.conf"
KIOSK_SERVICE="/etc/systemd/system/ralphglasses-kiosk.service"
KIOSK_STATE_FILE="/etc/ralphglasses/kiosk.state"
BACKUP_DIR="${RALPH_HOME}/.config/i3/backup"
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
        err "User '${RALPH_USER}' does not exist. Run chroot-setup.sh first."
        exit 1
    fi

    if ! command -v i3 &>/dev/null && [[ "$DRY_RUN" = false ]]; then
        err "i3 is not installed. Install with: apt install i3"
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
    echo "=== Ralphglasses Kiosk Status ==="
    echo ""

    # Check state file
    if [[ -f "$KIOSK_STATE_FILE" ]]; then
        echo "State file: $(cat "$KIOSK_STATE_FILE")"
    else
        echo "State file: not found (kiosk not installed)"
    fi

    # Check which i3 config is active
    if [[ -f "$I3_TARGET" ]]; then
        if head -1 "$I3_TARGET" | grep -q "kiosk"; then
            echo "i3 config:  KIOSK mode"
        else
            echo "i3 config:  NORMAL mode"
        fi
    else
        echo "i3 config:  not found"
    fi

    # Check auto-login
    if [[ -f "$GETTY_OVERRIDE" ]]; then
        echo "Auto-login: ENABLED (getty@tty1)"
    else
        echo "Auto-login: DISABLED"
    fi

    # Check xinitrc
    if [[ -f "$XINITRC" ]]; then
        echo "xinitrc:    present"
    else
        echo "xinitrc:    not found"
    fi

    # Check systemd service
    if [[ -f "$KIOSK_SERVICE" ]]; then
        local state
        state=$(systemctl is-enabled ralphglasses-kiosk.service 2>/dev/null || echo "unknown")
        echo "Service:    ${state}"
    else
        echo "Service:    not installed"
    fi

    # Check binary
    if [[ -x /usr/local/bin/ralphglasses ]]; then
        echo "Binary:     /usr/local/bin/ralphglasses (present)"
    else
        echo "Binary:     NOT FOUND — build and install first"
    fi

    echo ""
}

# ── Backup existing config ───────────────────────────────────────────
backup_config() {
    if [[ -f "$I3_TARGET" ]] && [[ "$DRY_RUN" = false ]]; then
        mkdir -p "$BACKUP_DIR"
        local ts
        ts=$(date +%Y%m%d-%H%M%S)
        cp "$I3_TARGET" "${BACKUP_DIR}/config.${ts}"
        log "Backed up current config to ${BACKUP_DIR}/config.${ts}"
    fi
}

# ── Install kiosk i3 config ─────────────────────────────────────────
install_i3_config() {
    log "Installing kiosk i3 config..."
    backup_config

    if [[ "$DRY_RUN" = false ]]; then
        mkdir -p "$I3_CONFIG_DIR"
        if [[ -f "$KIOSK_SOURCE" ]]; then
            cp "$KIOSK_SOURCE" "$I3_TARGET"
        else
            # Fallback: copy from repo relative path (for development)
            local script_dir
            script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
            local repo_kiosk="${script_dir}/../i3/kiosk-config"
            if [[ -f "$repo_kiosk" ]]; then
                cp "$repo_kiosk" "$I3_TARGET"
            else
                err "Kiosk config not found at ${KIOSK_SOURCE} or ${repo_kiosk}"
                exit 1
            fi
        fi
        chown -R "${RALPH_USER}:${RALPH_USER}" "$I3_CONFIG_DIR"
    else
        info "[dry-run] cp ${KIOSK_SOURCE} -> ${I3_TARGET}"
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

# ── Configure xinit to start i3 ─────────────────────────────────────
setup_xinit() {
    log "Configuring xinit to start i3..."

    write_file "$XINITRC" '#!/bin/sh
# ralphglasses kiosk — auto-generated by kiosk-setup.sh
# Do not edit manually; re-run kiosk-setup.sh --reconfigure instead.

# Disable screen blanking
xset s off
xset -dpms
xset s noblank

# Set keyboard repeat rate (fast for TUI navigation)
xset r rate 200 40

# Start i3
exec i3' "755"

    if [[ "$DRY_RUN" = false ]]; then
        chown "${RALPH_USER}:${RALPH_USER}" "$XINITRC"
    fi
}

# ── Configure bash_profile to auto-start X on tty1 ──────────────────
setup_bash_profile() {
    log "Configuring bash_profile for auto-startx..."

    local startx_block='# ralphglasses kiosk — auto-startx on tty1
if [ -z "$DISPLAY" ] && [ "$(tty)" = "/dev/tty1" ]; then
    exec startx -- -keeptty > /tmp/xorg.log 2>&1
fi'

    if [[ "$DRY_RUN" = false ]]; then
        # Remove any existing startx block, then append
        if [[ -f "$BASH_PROFILE" ]]; then
            # Strip old kiosk/startx blocks
            sed -i '/# ralphglasses kiosk/,/^fi$/d' "$BASH_PROFILE"
            # Also strip the old chroot-setup.sh block
            sed -i '/^if \[ -z "\$DISPLAY" \]/,/^fi$/d' "$BASH_PROFILE"
        else
            touch "$BASH_PROFILE"
        fi
        echo "" >> "$BASH_PROFILE"
        echo "$startx_block" >> "$BASH_PROFILE"
        chown "${RALPH_USER}:${RALPH_USER}" "$BASH_PROFILE"
    else
        info "[dry-run] append startx block to ${BASH_PROFILE}"
    fi
}

# ── Create systemd watchdog service ──────────────────────────────────
setup_systemd_service() {
    log "Installing ralphglasses-kiosk systemd service..."

    local service_content="[Unit]
Description=Ralphglasses Kiosk Watchdog
After=graphical.target
Wants=graphical.target
Documentation=https://github.com/hairglasses-studio/ralphglasses

[Service]
Type=simple
User=${RALPH_USER}
Environment=DISPLAY=:0
Environment=XAUTHORITY=${RALPH_HOME}/.Xauthority
Environment=RALPHGLASSES_SCAN_PATH=/workspace
ExecStartPre=/usr/bin/test -S /tmp/.X11-unix/X0
ExecStart=/bin/bash -c 'while true; do \\
    if ! pgrep -u ${RALPH_USER} -f \"ralphglasses --scan-path\" >/dev/null; then \\
        logger -t ralphglasses-kiosk \"No ralphglasses process found, i3 watchdog should restart\"; \\
    fi; \\
    sleep 30; \\
done'
Restart=always
RestartSec=10

[Install]
WantedBy=graphical.target"

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
timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
user=${RALPH_USER}
config=${I3_TARGET}
EOF
    else
        info "[dry-run] write state: ${state} -> ${KIOSK_STATE_FILE}"
    fi
}

# ── Disable kiosk mode ──────────────────────────────────────────────
disable_kiosk() {
    log "Disabling kiosk mode..."
    backup_config

    # Restore normal i3 config
    if [[ "$DRY_RUN" = false ]]; then
        if [[ -f "$NORMAL_SOURCE" ]]; then
            cp "$NORMAL_SOURCE" "$I3_TARGET"
        else
            local script_dir
            script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
            local repo_normal="${script_dir}/../i3/config"
            if [[ -f "$repo_normal" ]]; then
                cp "$repo_normal" "$I3_TARGET"
            else
                warn "Normal config not found; removing kiosk config only"
                rm -f "$I3_TARGET"
            fi
        fi
        chown -R "${RALPH_USER}:${RALPH_USER}" "$I3_CONFIG_DIR"
    else
        info "[dry-run] cp ${NORMAL_SOURCE} -> ${I3_TARGET}"
    fi

    # Disable kiosk service
    if [[ -f "$KIOSK_SERVICE" ]]; then
        run systemctl disable ralphglasses-kiosk.service 2>/dev/null || true
        run systemctl stop ralphglasses-kiosk.service 2>/dev/null || true
    fi

    write_state "disabled"
    log "Kiosk mode disabled. Reload i3 (Mod+Shift+r) or reboot to apply."
}

# ── Main install flow ────────────────────────────────────────────────
install_kiosk() {
    local label
    if [[ "$MODE" = "reconfigure" ]]; then
        label="Reconfiguring"
    else
        label="Installing"
    fi

    log "${label} ralphglasses kiosk mode..."
    echo ""

    # First-boot: full setup including autologin and xinit
    # Reconfigure: only update i3 config and service
    install_i3_config

    if [[ "$MODE" = "install" ]]; then
        setup_autologin
        setup_xinit
        setup_bash_profile
    else
        log "Skipping autologin/xinit setup (reconfigure mode)"
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
        log "Reboot to enter kiosk mode, or run: i3-msg reload"
    else
        log "Run: i3-msg reload"
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
