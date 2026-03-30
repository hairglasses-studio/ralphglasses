#!/usr/bin/env bash
#
# hw-profile.sh — Detect hardware and select matching ralphglasses profile
#
# Probes the local machine using lspci, lscpu, and free, then matches
# against JSON profiles in distro/hardware/profiles/. Outputs the matched
# profile name to stdout.
#
# Usage:
#   hw-profile.sh                          # Auto-detect and print profile name
#   hw-profile.sh --list                   # List available profiles
#   hw-profile.sh --apply <profile-name>   # Print profile JSON to stdout
#   hw-profile.sh --dry-run                # Detect without writing anything
#   hw-profile.sh --verbose                # Show detection details
#
# Exit codes:
#   0 — Profile matched
#   1 — Error (missing tools, no profiles)
#   2 — No specific match, fell back to default

set -euo pipefail

# --- Resolve script and profile directories ---

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROFILE_DIR="$(cd "${SCRIPT_DIR}/../hardware/profiles" 2>/dev/null && pwd)" || {
    echo "ERROR: Cannot find profiles directory at ${SCRIPT_DIR}/../hardware/profiles" >&2
    exit 1
}

# --- Flags ---

DRY_RUN=false
VERBOSE=false
ACTION="detect"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run)  DRY_RUN=true; shift ;;
        --verbose)  VERBOSE=true; shift ;;
        --list)     ACTION="list"; shift ;;
        --apply)    ACTION="apply"; APPLY_NAME="${2:-}"; shift 2 || { echo "ERROR: --apply requires a profile name" >&2; exit 1; } ;;
        -h|--help)
            sed -n '2,/^$/{ s/^# //; s/^#$//; p }' "$0"
            exit 0
            ;;
        *)
            echo "ERROR: Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

# --- Helpers ---

log() {
    if [[ "$VERBOSE" == true ]]; then
        echo "[hw-profile] $*" >&2
    fi
}

# --- List action ---

if [[ "$ACTION" == "list" ]]; then
    for f in "${PROFILE_DIR}"/*.json; do
        [[ -f "$f" ]] || continue
        basename "$f" .json
    done
    exit 0
fi

# --- Apply action ---

if [[ "$ACTION" == "apply" ]]; then
    PROFILE_FILE="${PROFILE_DIR}/${APPLY_NAME}.json"
    if [[ ! -f "$PROFILE_FILE" ]]; then
        echo "ERROR: Profile not found: ${APPLY_NAME}" >&2
        echo "Available profiles:" >&2
        for f in "${PROFILE_DIR}"/*.json; do
            [[ -f "$f" ]] && echo "  $(basename "$f" .json)" >&2
        done
        exit 1
    fi
    cat "$PROFILE_FILE"
    exit 0
fi

# --- Check prerequisites ---

for cmd in lspci lscpu free; do
    if ! command -v "$cmd" &>/dev/null; then
        echo "ERROR: $cmd not found. Install required packages:" >&2
        echo "  apt install pciutils util-linux procps" >&2
        exit 1
    fi
done

# --- Gather hardware facts ---

log "Gathering hardware facts..."

# GPU count and models (NVIDIA + AMD discrete)
GPU_LINES="$(lspci -nn 2>/dev/null | grep -iE 'VGA|3D controller|Display controller' || true)"
GPU_COUNT="$(echo "$GPU_LINES" | grep -c . 2>/dev/null || echo 0)"
HAS_RTX4090=false
HAS_GTX1060=false
HAS_INTEL_GPU=false

if echo "$GPU_LINES" | grep -qi "RTX 4090"; then
    HAS_RTX4090=true
fi
if echo "$GPU_LINES" | grep -qi "GTX 1060"; then
    HAS_GTX1060=true
fi
if echo "$GPU_LINES" | grep -qiE "Intel.*(UHD|Iris|HD Graphics)"; then
    HAS_INTEL_GPU=true
fi

log "GPU count: ${GPU_COUNT}"
log "GPUs found: ${GPU_LINES}"

# CPU cores (logical)
CPU_CORES="$(lscpu 2>/dev/null | awk -F: '/^CPU\(s\):/ { gsub(/[[:space:]]/, "", $2); print $2 }')"
CPU_CORES="${CPU_CORES:-0}"
CPU_MODEL="$(lscpu 2>/dev/null | awk -F: '/Model name/ { gsub(/^[[:space:]]+/, "", $2); print $2 }')"
CPU_MODEL="${CPU_MODEL:-unknown}"

log "CPU cores: ${CPU_CORES}"
log "CPU model: ${CPU_MODEL}"

# RAM in MB
RAM_MB="$(free -m 2>/dev/null | awk '/^Mem:/ { print $2 }')"
RAM_MB="${RAM_MB:-0}"

log "RAM MB: ${RAM_MB}"

# Storage type (prefer nvme > ssd > hdd)
STORAGE_TYPE="ssd"
if lspci -nn 2>/dev/null | grep -qi "NVM Express\|Non-Volatile memory"; then
    STORAGE_TYPE="nvme"
elif [[ -d /sys/block ]] && ls /sys/block/sd* &>/dev/null; then
    # Check if any disk is rotational
    for disk in /sys/block/sd*; do
        if [[ -f "${disk}/queue/rotational" ]] && [[ "$(cat "${disk}/queue/rotational")" == "1" ]]; then
            STORAGE_TYPE="hdd"
            break
        fi
    done
fi

log "Storage type: ${STORAGE_TYPE}"

# Network — check for known NICs
HAS_INTEL_I226=false
HAS_MT7927=false
if lspci -nn 2>/dev/null | grep -q "8086:125c"; then
    HAS_INTEL_I226=true
fi
if lspci -nn 2>/dev/null | grep -q "14c3:7927"; then
    HAS_MT7927=true
fi

log "Intel I226-V: ${HAS_INTEL_I226}"
log "MT7927 WiFi: ${HAS_MT7927}"

# --- Match profiles ---

log "Matching against profiles..."

MATCHED_PROFILE=""
EXIT_CODE=0

# ProArt X870E: RTX 4090 + high RAM + many cores
if [[ "$HAS_RTX4090" == true ]] && [[ "$RAM_MB" -ge 65536 ]] && [[ "$CPU_CORES" -ge 16 ]]; then
    MATCHED_PROFILE="proart-x870e"
    log "Matched: proart-x870e (RTX 4090 + >=64GB RAM + >=16 cores)"

# Mini PC: Intel integrated GPU + low RAM + few cores
elif [[ "$HAS_INTEL_GPU" == true ]] && [[ "$GPU_COUNT" -le 1 ]] && [[ "$RAM_MB" -le 32768 ]] && [[ "$CPU_CORES" -le 8 ]]; then
    MATCHED_PROFILE="mini-pc"
    log "Matched: mini-pc (Intel iGPU + <=32GB RAM + <=8 cores)"

# Default fallback
else
    MATCHED_PROFILE="default"
    EXIT_CODE=2
    log "No specific match, falling back to default"
fi

# Verify the profile file exists
PROFILE_FILE="${PROFILE_DIR}/${MATCHED_PROFILE}.json"
if [[ ! -f "$PROFILE_FILE" ]]; then
    echo "ERROR: Matched profile file missing: ${PROFILE_FILE}" >&2
    exit 1
fi

# --- Output ---

if [[ "$DRY_RUN" == true ]]; then
    echo "--- DRY RUN ---"
    echo "Detected hardware:"
    echo "  CPU:     ${CPU_MODEL} (${CPU_CORES} cores)"
    echo "  RAM:     ${RAM_MB} MB"
    echo "  GPUs:    ${GPU_COUNT}"
    echo "  Storage: ${STORAGE_TYPE}"
    echo "  RTX4090: ${HAS_RTX4090}"
    echo "  Intel:   ${HAS_INTEL_GPU}"
    echo ""
    echo "Matched profile: ${MATCHED_PROFILE}"
    echo "Profile path:    ${PROFILE_FILE}"
else
    echo "${MATCHED_PROFILE}"
fi

exit ${EXIT_CODE}
