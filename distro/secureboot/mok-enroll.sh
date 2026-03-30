#!/usr/bin/env bash
#
# mok-enroll.sh — Enroll MOK (Machine Owner Key) for UEFI Secure Boot
#
# Imports the ralphglasses MOK certificate into the shim MOK database using
# mokutil. On first boot after enrollment, the shim bootloader will prompt
# for the enrollment password to complete the process.
#
# Usage:
#   mok-enroll.sh                  # Enroll MOK and check status
#   mok-enroll.sh --status         # Only check enrollment status
#   mok-enroll.sh --dry-run        # Print what would be done
#
# Environment:
#   RALPH_MOK_DIR   Directory for MOK keys (default: /var/lib/ralphglasses/mok)
#
# Designed for: Ubuntu 24.04 / RHEL 9+ with UEFI Secure Boot
# See: docs/secure-boot.md

set -euo pipefail

# --- Constants ---

DEFAULT_MOK_DIR="/var/lib/ralphglasses/mok"
MOK_DIR="${RALPH_MOK_DIR:-$DEFAULT_MOK_DIR}"
MOK_DER="$MOK_DIR/MOK.der"
MOK_CRT="$MOK_DIR/MOK.crt"
LOG_FILE="/var/log/ralph-secureboot.log"
MARKER="/var/lib/ralphglasses/mok-enrolled"

# --- Flags ---

DRY_RUN=false
STATUS_ONLY=false

for arg in "$@"; do
    case "$arg" in
        --dry-run)
            DRY_RUN=true
            ;;
        --status)
            STATUS_ONLY=true
            ;;
        *)
            echo "Unknown argument: $arg" >&2
            echo "Usage: mok-enroll.sh [--status] [--dry-run]" >&2
            exit 1
            ;;
    esac
done

# --- Helpers ---

log() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] $*"
    echo "$msg"
    if [[ "$DRY_RUN" == false ]]; then
        echo "$msg" >> "$LOG_FILE" 2>/dev/null || true
    fi
}

die() {
    log "FATAL: $*"
    exit 1
}

# --- Locate mokutil ---
# Debian/Ubuntu: /usr/bin/mokutil from mokutil package
# RHEL/Fedora:   /usr/bin/mokutil from mokutil package

find_mokutil() {
    if command -v mokutil &>/dev/null; then
        echo "mokutil"
        return
    fi
    for path in /usr/bin/mokutil /usr/local/bin/mokutil; do
        if [[ -x "$path" ]]; then
            echo "$path"
            return
        fi
    done
    return 1
}

# --- Check prerequisites ---

if [[ "$DRY_RUN" == false ]] && [[ "$STATUS_ONLY" == false ]] && [[ $EUID -ne 0 ]]; then
    echo "ERROR: Must run as root (or use --dry-run / --status)" >&2
    exit 1
fi

MOKUTIL="$(find_mokutil)" || die "mokutil not found. Install: apt install mokutil (Debian) or dnf install mokutil (RHEL)"

# --- Check Secure Boot state ---

log "=== mok-enroll.sh starting ==="
log "Mode: $(if $DRY_RUN; then echo 'DRY RUN'; elif $STATUS_ONLY; then echo 'STATUS CHECK'; else echo 'LIVE'; fi)"
log "MOK directory: $MOK_DIR"
log "mokutil: $MOKUTIL"

log "--- Secure Boot Status ---"

SB_STATE="unknown"
if "$MOKUTIL" --sb-state 2>/dev/null | grep -qi "secureboot enabled"; then
    SB_STATE="enabled"
    log "Secure Boot: ENABLED"
elif "$MOKUTIL" --sb-state 2>/dev/null | grep -qi "secureboot disabled"; then
    SB_STATE="disabled"
    log "Secure Boot: DISABLED"
    log "WARNING: Secure Boot is disabled. MOK enrollment will succeed but has no effect until Secure Boot is enabled in UEFI settings."
else
    log "Secure Boot: UNKNOWN (could not determine state)"
fi

# --- Check if MOK is already enrolled ---

log "--- Enrollment Status ---"

MOK_ENROLLED=false

if [[ -f "$MOK_CRT" ]]; then
    # Extract the certificate fingerprint
    MOK_FINGERPRINT="$(openssl x509 -in "$MOK_CRT" -noout -fingerprint -sha256 2>/dev/null | sed 's/.*=//')"
    log "MOK certificate fingerprint: $MOK_FINGERPRINT"

    # Check if this specific key is already enrolled
    if "$MOKUTIL" --list-enrolled 2>/dev/null | grep -qi "${MOK_FINGERPRINT//:/}"; then
        MOK_ENROLLED=true
        log "MOK key is ALREADY ENROLLED in the shim database"
    else
        log "MOK key is NOT enrolled"
    fi

    # Check if there is a pending enrollment request
    if "$MOKUTIL" --list-new 2>/dev/null | grep -qi "SHA256"; then
        log "PENDING enrollment request detected — reboot to complete enrollment"
    fi
else
    log "MOK certificate not found at $MOK_CRT"
    log "Run sign-kernel.sh first to generate the MOK key pair"
fi

# --- Status-only mode exits here ---

if [[ "$STATUS_ONLY" == true ]]; then
    log ""
    log "=== Enrollment Summary ==="
    log "Secure Boot: $SB_STATE"
    log "MOK enrolled: $MOK_ENROLLED"
    if [[ -f "$MARKER" ]]; then
        log "Marker file: $MARKER (present)"
    else
        log "Marker file: $MARKER (absent)"
    fi
    exit 0
fi

# --- Early exit if already enrolled ---

if [[ "$MOK_ENROLLED" == true ]]; then
    log "MOK is already enrolled. Nothing to do."
    if [[ "$DRY_RUN" == false ]]; then
        mkdir -p "$(dirname "$MARKER")"
        touch "$MARKER"
    fi
    exit 0
fi

# --- Verify DER certificate exists ---

log "--- MOK Import ---"

if [[ ! -f "$MOK_DER" ]]; then
    # Try to convert from PEM if only PEM exists
    if [[ -f "$MOK_CRT" ]]; then
        log "DER certificate not found, converting from PEM..."
        if [[ "$DRY_RUN" == false ]]; then
            openssl x509 -in "$MOK_CRT" -outform DER -out "$MOK_DER"
            log "Converted $MOK_CRT -> $MOK_DER"
        else
            log "DRY RUN: Would convert $MOK_CRT -> $MOK_DER"
        fi
    else
        die "No MOK certificate found. Run sign-kernel.sh first to generate the key pair."
    fi
fi

# --- Import MOK ---

if [[ "$DRY_RUN" == true ]]; then
    log "DRY RUN: Would execute: $MOKUTIL --import $MOK_DER"
    log "DRY RUN: You would be prompted for an enrollment password"
    log "DRY RUN: On next reboot, shim would prompt to complete enrollment"
else
    log "Importing MOK certificate..."
    log "You will be prompted to set a one-time enrollment password."
    log "Remember this password — you will need it on the next reboot."
    log ""

    if ! "$MOKUTIL" --import "$MOK_DER"; then
        die "mokutil --import failed"
    fi

    log ""
    log "MOK import request submitted successfully"
    log ""
    log "IMPORTANT: Complete enrollment by rebooting the system."
    log "  1. Reboot the machine"
    log "  2. The shim bootloader will display 'Perform MOK management'"
    log "  3. Select 'Enroll MOK'"
    log "  4. Select 'Continue'"
    log "  5. Enter the enrollment password you just set"
    log "  6. Select 'Reboot'"

    # Create marker indicating import was requested (not yet enrolled)
    mkdir -p "$(dirname "$MARKER")"
    echo "pending-reboot" > "$MARKER"
    log "Marker created: $MARKER (pending-reboot)"
fi

# --- Verify pending enrollment ---

log "--- Verification ---"

if [[ "$DRY_RUN" == false ]]; then
    if "$MOKUTIL" --list-new 2>/dev/null | grep -qi "SHA256"; then
        log "Confirmed: enrollment request is PENDING"
    else
        log "WARNING: Could not confirm pending enrollment request"
    fi
fi

# --- Summary ---

log ""
log "=== mok-enroll.sh Summary ==="
log "Secure Boot:  $SB_STATE"
log "MOK cert:     $MOK_DER"
log "Status:       import requested, pending reboot"
log ""
log "Next steps:"
log "  1. Reboot to complete MOK enrollment"
log "  2. Run: mok-enroll.sh --status  (to verify after reboot)"
log "=== mok-enroll.sh complete ==="
