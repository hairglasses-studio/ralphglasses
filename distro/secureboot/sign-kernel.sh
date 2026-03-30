#!/usr/bin/env bash
#
# sign-kernel.sh — Sign kernel with MOK (Machine Owner Key) for Secure Boot
#
# Generates a MOK key pair if one does not exist, signs the specified kernel
# image with sbsign, and verifies the resulting signature. The signed kernel
# can then boot on UEFI systems with Secure Boot enabled after the MOK is
# enrolled via mok-enroll.sh.
#
# Usage:
#   sign-kernel.sh                    # Sign current running kernel
#   sign-kernel.sh /boot/vmlinuz-6.8  # Sign a specific kernel image
#   sign-kernel.sh --dry-run          # Print what would be done
#
# Environment:
#   RALPH_MOK_DIR   Directory for MOK keys (default: /var/lib/ralphglasses/mok)
#   RALPH_MOK_CN    Common Name for the MOK certificate (default: ralphglasses MOK)
#
# Designed for: Ubuntu 24.04 / RHEL 9+ with UEFI Secure Boot
# See: docs/secure-boot.md

set -euo pipefail

# --- Constants ---

DEFAULT_MOK_DIR="/var/lib/ralphglasses/mok"
MOK_DIR="${RALPH_MOK_DIR:-$DEFAULT_MOK_DIR}"
MOK_KEY="$MOK_DIR/MOK.key"
MOK_CRT="$MOK_DIR/MOK.crt"
MOK_DER="$MOK_DIR/MOK.der"
MOK_CN="${RALPH_MOK_CN:-ralphglasses MOK}"
LOG_FILE="/var/log/ralph-secureboot.log"

# --- Flags ---

DRY_RUN=false
KERNEL_IMAGE=""

for arg in "$@"; do
    case "$arg" in
        --dry-run)
            DRY_RUN=true
            ;;
        *)
            KERNEL_IMAGE="$arg"
            ;;
    esac
done

# --- Helpers ---

log() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] $*"
    echo "$msg"
    if [[ "$DRY_RUN" == false ]]; then
        echo "$msg" >> "$LOG_FILE"
    fi
}

die() {
    log "FATAL: $*"
    exit 1
}

# --- Locate sbsign ---
# Debian/Ubuntu: sbsign is in sbsigntool package at /usr/bin/sbsign
# RHEL/Fedora:   sbsign is in sbsigntools package at /usr/bin/sbsign
#                or pesign-based workflow via /usr/bin/pesign

find_sbsign() {
    if command -v sbsign &>/dev/null; then
        echo "sbsign"
        return
    fi
    # Check common paths on both distro families
    for path in /usr/bin/sbsign /usr/local/bin/sbsign; do
        if [[ -x "$path" ]]; then
            echo "$path"
            return
        fi
    done
    return 1
}

find_sbverify() {
    if command -v sbverify &>/dev/null; then
        echo "sbverify"
        return
    fi
    for path in /usr/bin/sbverify /usr/local/bin/sbverify; do
        if [[ -x "$path" ]]; then
            echo "$path"
            return
        fi
    done
    return 1
}

# --- Check prerequisites ---

if [[ "$DRY_RUN" == false ]] && [[ $EUID -ne 0 ]]; then
    echo "ERROR: Must run as root (or use --dry-run)" >&2
    exit 1
fi

SBSIGN="$(find_sbsign)" || die "sbsign not found. Install: apt install sbsigntool (Debian) or dnf install sbsigntools (RHEL)"
SBVERIFY="$(find_sbverify)" || die "sbverify not found. Install: apt install sbsigntool (Debian) or dnf install sbsigntools (RHEL)"

if ! command -v openssl &>/dev/null; then
    die "openssl not found. Install: apt install openssl (Debian) or dnf install openssl (RHEL)"
fi

# --- Resolve kernel image ---

if [[ -z "$KERNEL_IMAGE" ]]; then
    KERNEL_IMAGE="/boot/vmlinuz-$(uname -r)"
fi

if [[ ! -f "$KERNEL_IMAGE" ]]; then
    die "Kernel image not found: $KERNEL_IMAGE"
fi

# --- Begin signing ---

log "=== sign-kernel.sh starting ==="
log "Mode: $(if $DRY_RUN; then echo 'DRY RUN'; else echo 'LIVE'; fi)"
log "Kernel: $KERNEL_IMAGE"
log "MOK directory: $MOK_DIR"
log "sbsign: $SBSIGN"

# --- Generate MOK key pair if missing ---

log "--- Key Pair Check ---"

if [[ -f "$MOK_KEY" ]] && [[ -f "$MOK_CRT" ]]; then
    log "MOK key pair already exists"
    log "  Private key: $MOK_KEY"
    log "  Certificate: $MOK_CRT"
else
    log "MOK key pair not found, generating..."

    if [[ "$DRY_RUN" == true ]]; then
        log "DRY RUN: Would create directory $MOK_DIR"
        log "DRY RUN: Would generate RSA 4096 key pair with CN='$MOK_CN'"
        log "DRY RUN: Would convert certificate to DER format"
        log "DRY RUN: Would set permissions 0600 on private key"
    else
        mkdir -p "$MOK_DIR"
        chmod 0700 "$MOK_DIR"

        log "Generating RSA 4096 key pair (CN='$MOK_CN', valid 10 years)..."
        openssl req -new -x509 \
            -newkey rsa:4096 \
            -keyout "$MOK_KEY" \
            -out "$MOK_CRT" \
            -nodes \
            -days 3650 \
            -subj "/CN=$MOK_CN/" \
            -addext "extendedKeyUsage=codeSigning" 2>&1 | while IFS= read -r line; do
                log "  openssl: $line"
            done

        # Convert PEM certificate to DER for mokutil enrollment
        openssl x509 -in "$MOK_CRT" -outform DER -out "$MOK_DER"

        # Restrict private key permissions
        chmod 0600 "$MOK_KEY"
        chmod 0644 "$MOK_CRT" "$MOK_DER"

        log "MOK key pair generated:"
        log "  Private key: $MOK_KEY (0600)"
        log "  Certificate: $MOK_CRT (PEM)"
        log "  Certificate: $MOK_DER (DER, for mokutil)"

        # Print certificate fingerprint for verification
        FINGERPRINT="$(openssl x509 -in "$MOK_CRT" -noout -fingerprint -sha256 2>/dev/null)"
        log "  Fingerprint: $FINGERPRINT"
    fi
fi

# --- Sign kernel ---

log "--- Kernel Signing ---"

SIGNED_KERNEL="${KERNEL_IMAGE}.signed"

if [[ "$DRY_RUN" == true ]]; then
    log "DRY RUN: Would sign $KERNEL_IMAGE -> $SIGNED_KERNEL"
    log "DRY RUN: Would execute: $SBSIGN --key $MOK_KEY --cert $MOK_CRT --output $SIGNED_KERNEL $KERNEL_IMAGE"
else
    log "Signing kernel image..."
    log "  Input:  $KERNEL_IMAGE"
    log "  Output: $SIGNED_KERNEL"

    if ! "$SBSIGN" --key "$MOK_KEY" --cert "$MOK_CRT" --output "$SIGNED_KERNEL" "$KERNEL_IMAGE"; then
        die "sbsign failed to sign kernel"
    fi

    log "Kernel signed successfully"
fi

# --- Verify signature ---

log "--- Signature Verification ---"

if [[ "$DRY_RUN" == true ]]; then
    log "DRY RUN: Would verify $SIGNED_KERNEL with $SBVERIFY"
else
    log "Verifying signature on $SIGNED_KERNEL..."

    if "$SBVERIFY" --cert "$MOK_CRT" "$SIGNED_KERNEL" 2>&1; then
        log "Signature verification PASSED"
    else
        die "Signature verification FAILED — the signed kernel may be corrupt"
    fi

    # Replace original with signed version
    BACKUP="${KERNEL_IMAGE}.unsigned"
    cp "$KERNEL_IMAGE" "$BACKUP"
    mv "$SIGNED_KERNEL" "$KERNEL_IMAGE"
    log "Original kernel backed up to: $BACKUP"
    log "Signed kernel installed at: $KERNEL_IMAGE"
fi

# --- Summary ---

log ""
log "=== sign-kernel.sh Summary ==="
log "Kernel:       $KERNEL_IMAGE"
log "MOK key:      $MOK_KEY"
log "MOK cert:     $MOK_CRT"
log "MOK cert DER: $MOK_DER"
log "Backup:       ${KERNEL_IMAGE}.unsigned"
log ""
log "Next step: Enroll the MOK key with mok-enroll.sh"
log "=== sign-kernel.sh complete ==="
