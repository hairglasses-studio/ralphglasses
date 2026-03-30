#!/usr/bin/env bash
#
# ts-enroll.sh — First-boot Tailscale enrollment for ralphglasses thin client
#
# Derives a deterministic hostname from the primary NIC MAC address, enrolls
# the node into the ralph tailnet with role-appropriate tags, and removes the
# auth key after successful enrollment.
#
# Usage:
#   ts-enroll.sh              # Run as root via systemd (ts-enroll.service)
#   ts-enroll.sh --dry-run    # Print what would be done, write nothing
#
# Environment:
#   RALPH_ROLE   "coordinator" or "worker" (default: worker)
#
# Designed for: ASUS ProArt X870E-CREATOR WIFI
# See: distro/hardware/proart-x870e.md

set -euo pipefail

# --- Constants ---

MARKER="/var/lib/tailscale/ralph-enrolled"
TS_AUTHKEY_FILE="/etc/ralphglasses/ts-authkey"
HOSTNAME_PREFIX="ralph"
LOG_FILE="/var/log/ts-enroll.log"
TAILSCALED_WAIT_SECS=30

# --- Flags ---

DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
    DRY_RUN=true
fi

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

# --- Early exit if already enrolled ---

if [[ -f "$MARKER" ]]; then
    log "Already enrolled (marker exists: $MARKER). Nothing to do."
    exit 0
fi

# --- Check prerequisites ---

if [[ "$DRY_RUN" == false ]] && [[ $EUID -ne 0 ]]; then
    echo "ERROR: Must run as root (or use --dry-run)" >&2
    exit 1
fi

if ! command -v tailscale &>/dev/null; then
    die "tailscale CLI not found. Install: curl -fsSL https://tailscale.com/install.sh | sh"
fi

if ! command -v ip &>/dev/null; then
    die "ip command not found. Install iproute2: apt install iproute2"
fi

# --- Begin enrollment ---

log "=== ts-enroll.sh starting ==="
log "Mode: $(if $DRY_RUN; then echo 'DRY RUN'; else echo 'LIVE'; fi)"
log "Kernel: $(uname -r)"

# --- Derive hostname from MAC address ---

log "--- Hostname Derivation ---"

# Find the first non-loopback, non-virtual interface with a MAC address
PRIMARY_MAC=""
PRIMARY_IFACE=""
while IFS= read -r line; do
    # Match lines like "2: enp6s0: <BROADCAST,..."
    if [[ "$line" =~ ^[0-9]+:\ ([a-zA-Z0-9]+):\ .* ]]; then
        iface="${BASH_REMATCH[1]}"
        # Skip loopback, virtual, and tailscale interfaces
        if [[ "$iface" == "lo" ]] || [[ "$iface" == tailscale* ]] || [[ "$iface" == veth* ]] || [[ "$iface" == docker* ]] || [[ "$iface" == br-* ]]; then
            continue
        fi
        PRIMARY_IFACE="$iface"
    fi
    # Match link/ether lines
    if [[ -n "$PRIMARY_IFACE" ]] && [[ "$line" =~ link/ether\ ([0-9a-f:]+) ]]; then
        PRIMARY_MAC="${BASH_REMATCH[1]}"
        break
    fi
done < <(ip link show 2>/dev/null)

if [[ -z "$PRIMARY_MAC" ]]; then
    die "Could not determine primary NIC MAC address"
fi

# Derive short ID from MAC: strip colons, take last 6 hex chars
MAC_STRIPPED="${PRIMARY_MAC//:/}"
MAC_SUFFIX="${MAC_STRIPPED: -6}"
TS_HOSTNAME="${HOSTNAME_PREFIX}-${MAC_SUFFIX}"

log "Primary interface: $PRIMARY_IFACE"
log "Primary MAC: $PRIMARY_MAC"
log "Derived hostname: $TS_HOSTNAME"

# --- Read auth key ---

log "--- Auth Key ---"

if [[ ! -f "$TS_AUTHKEY_FILE" ]]; then
    die "Auth key file not found: $TS_AUTHKEY_FILE"
fi

TS_AUTHKEY="$(cat "$TS_AUTHKEY_FILE")"
if [[ -z "$TS_AUTHKEY" ]]; then
    die "Auth key file is empty: $TS_AUTHKEY_FILE"
fi

log "Auth key loaded from $TS_AUTHKEY_FILE (${#TS_AUTHKEY} chars)"

# --- Wait for tailscaled readiness ---

log "--- Waiting for tailscaled ---"

elapsed=0
while [[ $elapsed -lt $TAILSCALED_WAIT_SECS ]]; do
    if tailscale status &>/dev/null; then
        log "tailscaled is ready (waited ${elapsed}s)"
        break
    fi
    if [[ "$DRY_RUN" == true ]]; then
        log "DRY RUN: skipping tailscaled wait"
        break
    fi
    sleep 1
    elapsed=$((elapsed + 1))
done

if [[ $elapsed -ge $TAILSCALED_WAIT_SECS ]]; then
    die "tailscaled not ready after ${TAILSCALED_WAIT_SECS}s"
fi

# --- Determine role and tags ---

log "--- Role Configuration ---"

RALPH_ROLE="${RALPH_ROLE:-worker}"
log "Role: $RALPH_ROLE"

case "$RALPH_ROLE" in
    coordinator)
        TS_TAGS="tag:ralph-fleet,tag:ralph-coordinator"
        ;;
    worker)
        TS_TAGS="tag:ralph-fleet,tag:ralph-worker"
        ;;
    *)
        die "Unknown RALPH_ROLE: $RALPH_ROLE (expected 'coordinator' or 'worker')"
        ;;
esac

log "Tags: $TS_TAGS"

# --- Enroll in tailnet ---

log "--- Tailscale Enrollment ---"

if [[ "$DRY_RUN" == true ]]; then
    log "DRY RUN: Would execute: tailscale up --authkey=<redacted> --advertise-tags=${TS_TAGS} --hostname=${TS_HOSTNAME} --ssh --accept-routes"
else
    log "Executing: tailscale up --authkey=<redacted> --advertise-tags=${TS_TAGS} --hostname=${TS_HOSTNAME} --ssh --accept-routes"
    if ! tailscale up \
        --authkey="${TS_AUTHKEY}" \
        --advertise-tags="${TS_TAGS}" \
        --hostname="${TS_HOSTNAME}" \
        --ssh \
        --accept-routes; then
        die "tailscale up failed"
    fi
    log "Tailscale enrollment succeeded"
fi

# --- Verify connectivity ---

log "--- Connectivity Verification ---"

if [[ "$DRY_RUN" == true ]]; then
    log "DRY RUN: Would verify tailscale status and connectivity"
else
    # Verify tailscale reports as running
    if ! ts_status="$(tailscale status --json 2>/dev/null)"; then
        log "WARNING: Could not retrieve tailscale status after enrollment"
    else
        log "Tailscale status retrieved successfully"
    fi

    # Verify the node's IP was assigned
    ts_ip="$(tailscale ip -4 2>/dev/null || true)"
    if [[ -n "$ts_ip" ]]; then
        log "Tailscale IPv4 address: $ts_ip"
    else
        log "WARNING: No Tailscale IPv4 address assigned yet (may take a moment)"
    fi

    # Verify hostname was set correctly
    ts_name="$(tailscale status --self --json 2>/dev/null | grep -o '"HostName":"[^"]*"' | head -1 || true)"
    if [[ -n "$ts_name" ]]; then
        log "Tailscale self: $ts_name"
    fi
fi

# --- Create marker file ---

log "--- Post-Enrollment ---"

if [[ "$DRY_RUN" == true ]]; then
    log "DRY RUN: Would create marker $MARKER"
    log "DRY RUN: Would remove auth key $TS_AUTHKEY_FILE"
else
    mkdir -p "$(dirname "$MARKER")"
    touch "$MARKER"
    log "Created marker: $MARKER"

    # Remove auth key — it is single-use and should not persist on disk
    rm -f "$TS_AUTHKEY_FILE"
    log "Removed auth key: $TS_AUTHKEY_FILE"
fi

# --- Summary ---

log ""
log "=== ts-enroll.sh Summary ==="
log "Hostname:   $TS_HOSTNAME"
log "Role:       $RALPH_ROLE"
log "Tags:       $TS_TAGS"
log "Interface:  $PRIMARY_IFACE ($PRIMARY_MAC)"
log "Marker:     $MARKER"
log ""
log "=== ts-enroll.sh complete ==="
