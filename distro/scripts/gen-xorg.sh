#!/usr/bin/env bash
#
# gen-xorg.sh — Auto-detect NVIDIA GPUs and generate Xorg config for
# ralphglasses thin client (7-monitor, single or dual GPU).
#
# Reads the template at distro/xorg/dynamic-gpu.conf, detects GPUs via
# lspci, substitutes BusIDs, and emits a ready-to-use xorg.conf.
#
# Usage:
#   gen-xorg.sh                     # Detect GPUs, write /etc/X11/xorg.conf
#   gen-xorg.sh --dry-run           # Print config to stdout, write nothing
#   gen-xorg.sh --output /tmp/x.conf  # Write to custom path
#   gen-xorg.sh --template /path/to/template.conf
#
# Requires: lspci (pciutils), bash 4+
# Designed for: ASUS ProArt X870E-CREATOR WIFI
# See: distro/hardware/proart-x870e.md

set -euo pipefail

# --- Constants ---

NVIDIA_VENDOR="10de"
# RTX 4090 Ada Lovelace device IDs (reference + partner cards)
NVIDIA_RTX4090_IDS=("2684" "2685")
# Cards to exclude (Pascal GTX 1060, known driver conflict)
NVIDIA_EXCLUDE_IDS=("1c03" "1c02")

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_TEMPLATE="${SCRIPT_DIR}/../xorg/dynamic-gpu.conf"
DEFAULT_OUTPUT="/etc/X11/xorg.conf"
LOG_TAG="gen-xorg"

# --- Argument parsing ---

DRY_RUN=false
TEMPLATE_PATH="$DEFAULT_TEMPLATE"
OUTPUT_PATH="$DEFAULT_OUTPUT"
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --output)
            OUTPUT_PATH="$2"
            shift 2
            ;;
        --template)
            TEMPLATE_PATH="$2"
            shift 2
            ;;
        --verbose|-v)
            VERBOSE=true
            shift
            ;;
        --help|-h)
            echo "Usage: gen-xorg.sh [--dry-run] [--output PATH] [--template PATH] [--verbose]"
            echo ""
            echo "Auto-detect NVIDIA GPUs and generate Xorg config for 7-monitor setup."
            echo ""
            echo "Options:"
            echo "  --dry-run         Print generated config to stdout, write nothing"
            echo "  --output PATH     Output file (default: /etc/X11/xorg.conf)"
            echo "  --template PATH   Template file (default: distro/xorg/dynamic-gpu.conf)"
            echo "  --verbose, -v     Print detection details to stderr"
            echo "  --help, -h        Show this help"
            exit 0
            ;;
        *)
            echo "ERROR: Unknown argument: $1" >&2
            echo "Run with --help for usage." >&2
            exit 1
            ;;
    esac
done

# --- Helpers ---

log() {
    if [[ "$VERBOSE" == true ]]; then
        echo "[${LOG_TAG}] $*" >&2
    fi
}

die() {
    echo "ERROR: $*" >&2
    exit 1
}

# Convert PCI bus address "XX:YY.Z" to Xorg BusID "PCI:D:D:D" (hex to decimal)
pci_to_busid() {
    local addr="$1"
    local bus dev func
    IFS=':.' read -r bus dev func <<< "$addr"
    printf "PCI:%d:%d:%d" "0x${bus}" "0x${dev}" "${func}"
}

# Check if a device ID is in the exclude list
is_excluded() {
    local devid="$1"
    local exclude
    for exclude in "${NVIDIA_EXCLUDE_IDS[@]}"; do
        if [[ "${devid,,}" == "${exclude,,}" ]]; then
            return 0
        fi
    done
    return 1
}

# Check if a device ID is an RTX 4090 variant
is_rtx4090() {
    local devid="$1"
    local id
    for id in "${NVIDIA_RTX4090_IDS[@]}"; do
        if [[ "${devid,,}" == "${id,,}" ]]; then
            return 0
        fi
    done
    return 1
}

# --- Prerequisites ---

if [[ "$DRY_RUN" == false ]] && [[ $EUID -ne 0 ]]; then
    die "Must run as root (or use --dry-run)"
fi

if ! command -v lspci &>/dev/null; then
    die "lspci not found. Install pciutils: apt install pciutils"
fi

if [[ ! -f "$TEMPLATE_PATH" ]]; then
    die "Template not found: $TEMPLATE_PATH"
fi

# --- GPU Detection ---

log "Scanning PCI bus for NVIDIA GPUs..."

LSPCI_OUTPUT=$(lspci -nn 2>/dev/null || true)

if [[ -z "$LSPCI_OUTPUT" ]]; then
    die "lspci returned no output. No PCI bus available (WSL without passthrough?)"
fi

# Collect eligible NVIDIA VGA/3D GPUs
declare -a GPU_ADDRS=()
declare -a GPU_DEVIDS=()
declare -a GPU_NAMES=()

while IFS= read -r line; do
    [[ -z "$line" ]] && continue

    # Extract PCI address (first field, e.g., "01:00.0")
    addr=$(echo "$line" | awk '{print $1}')

    # Extract NVIDIA device ID from [10de:XXXX]
    devid=$(echo "$line" | grep -oP "10de:\K[0-9a-fA-F]{4}" | head -1)
    [[ -z "$devid" ]] && continue

    # Extract human-readable name (between first ] and [10de:)
    name=$(echo "$line" | sed 's/^[^ ]* //' | sed 's/ \[10de:.*//')

    if is_excluded "$devid"; then
        log "Excluding GPU at PCI $addr: $name [10de:$devid] (blacklisted device ID)"
        continue
    fi

    log "Found eligible GPU at PCI $addr: $name [10de:$devid]"
    GPU_ADDRS+=("$addr")
    GPU_DEVIDS+=("$devid")
    GPU_NAMES+=("$name")

done < <(echo "$LSPCI_OUTPUT" | grep -i "${NVIDIA_VENDOR}:" | grep -iE "VGA|3D controller" || true)

GPU_COUNT=${#GPU_ADDRS[@]}

if [[ $GPU_COUNT -eq 0 ]]; then
    die "No eligible NVIDIA GPUs detected. Check lspci output and driver installation."
fi

log "Detected $GPU_COUNT eligible NVIDIA GPU(s)"

# --- Sort: prefer RTX 4090 as GPU0 ---

# If we have multiple GPUs, ensure RTX 4090 variants come first
if [[ $GPU_COUNT -ge 2 ]]; then
    for i in $(seq 0 $((GPU_COUNT - 1))); do
        if is_rtx4090 "${GPU_DEVIDS[$i]}" && [[ $i -ne 0 ]]; then
            # Swap with position 0
            tmp_addr="${GPU_ADDRS[0]}"
            tmp_devid="${GPU_DEVIDS[0]}"
            tmp_name="${GPU_NAMES[0]}"
            GPU_ADDRS[0]="${GPU_ADDRS[$i]}"
            GPU_DEVIDS[0]="${GPU_DEVIDS[$i]}"
            GPU_NAMES[0]="${GPU_NAMES[$i]}"
            GPU_ADDRS[$i]="$tmp_addr"
            GPU_DEVIDS[$i]="$tmp_devid"
            GPU_NAMES[$i]="$tmp_name"
            break
        fi
    done
fi

GPU0_BUSID=$(pci_to_busid "${GPU_ADDRS[0]}")
log "GPU0: ${GPU_NAMES[0]} at ${GPU_ADDRS[0]} -> $GPU0_BUSID"

if [[ $GPU_COUNT -ge 2 ]]; then
    GPU1_BUSID=$(pci_to_busid "${GPU_ADDRS[1]}")
    log "GPU1: ${GPU_NAMES[1]} at ${GPU_ADDRS[1]} -> $GPU1_BUSID"
fi

# --- Determine mode ---

if [[ $GPU_COUNT -ge 2 ]]; then
    MODE="dual"
    log "Mode: dual-GPU (7 monitors across 2 GPUs)"
else
    MODE="single"
    log "Mode: single-GPU (7 monitors on 1 GPU)"
fi

# --- Generate config from template ---

log "Reading template: $TEMPLATE_PATH"
CONFIG=$(cat "$TEMPLATE_PATH")

# Substitute GPU0 BusID
CONFIG="${CONFIG//@@GPU0_BUSID@@/$GPU0_BUSID}"

if [[ "$MODE" == "dual" ]]; then
    # Substitute GPU1 BusID and use dual layout
    CONFIG="${CONFIG//@@GPU1_BUSID@@/$GPU1_BUSID}"
    CONFIG=$(echo "$CONFIG" | sed 's/"DefaultServerLayout" "layout-dual"/"DefaultServerLayout" "layout-dual"/')
else
    # Single-GPU mode: remove GPU1 device/screen sections and remap screens 4-6 to GPU0
    log "Rewriting template for single-GPU mode..."

    # Build single-GPU config from scratch instead of patching template
    CONFIG="# Generated by gen-xorg.sh — single-GPU mode
# GPU: ${GPU_NAMES[0]} at ${GPU_ADDRS[0]}
# Date: $(date -u '+%Y-%m-%dT%H:%M:%SZ')
# 7 monitors on a single NVIDIA GPU

Section \"ServerLayout\"
    Identifier  \"layout-single\"
    Screen   0  \"screen0\" 0    0
    Screen   1  \"screen1\" RightOf \"screen0\"
    Screen   2  \"screen2\" RightOf \"screen1\"
    Screen   3  \"screen3\" RightOf \"screen2\"
    Screen   4  \"screen4\" RightOf \"screen3\"
    Screen   5  \"screen5\" RightOf \"screen4\"
    Screen   6  \"screen6\" RightOf \"screen5\"
    Option      \"Xinerama\" \"on\"
EndSection

Section \"ServerFlags\"
    Option \"DefaultServerLayout\" \"layout-single\"
    Option \"BlankTime\"  \"0\"
    Option \"StandbyTime\" \"0\"
    Option \"SuspendTime\" \"0\"
    Option \"OffTime\"     \"0\"
    Option \"DPMS\"        \"false\"
EndSection
"

    # Generate 7 Device sections, all pointing to GPU0
    for i in $(seq 0 6); do
        if [[ $i -eq 0 ]]; then
            ident="gpu0"
            extra=$'\n    Option      "Coolbits" "28"\n    Option      "TripleBuffer" "true"'
        else
            ident="gpu0-screen${i}"
            extra=""
        fi
        CONFIG+="
Section \"Device\"
    Identifier  \"${ident}\"
    Driver      \"nvidia\"
    BusID       \"${GPU0_BUSID}\"
    Option      \"AllowEmptyInitialConfiguration\" \"true\"${extra}
    Screen      ${i}
EndSection
"
    done

    # Generate 7 Monitor sections
    for i in $(seq 0 6); do
        primary=""
        if [[ $i -eq 0 ]]; then
            primary=$'\n    Option      "Primary" "true"'
        fi
        CONFIG+="
Section \"Monitor\"
    Identifier  \"monitor${i}\"
    Option      \"DPMS\" \"false\"${primary}
EndSection
"
    done

    # Generate 7 Screen sections
    for i in $(seq 0 6); do
        if [[ $i -eq 0 ]]; then
            dev="gpu0"
        else
            dev="gpu0-screen${i}"
        fi
        CONFIG+="
Section \"Screen\"
    Identifier  \"screen${i}\"
    Device      \"${dev}\"
    Monitor     \"monitor${i}\"
    DefaultDepth 24
    SubSection \"Display\"
        Depth   24
    EndSubSection
EndSection
"
    done
fi

# --- Add generation metadata as a header ---

HEADER="# Auto-generated by gen-xorg.sh — DO NOT EDIT
# Mode: ${MODE}-GPU
# GPU0: ${GPU_NAMES[0]} at ${GPU_ADDRS[0]} (${GPU0_BUSID})"

if [[ "$MODE" == "dual" ]]; then
    HEADER+="
# GPU1: ${GPU_NAMES[1]} at ${GPU_ADDRS[1]} (${GPU1_BUSID})"
fi

HEADER+="
# Date: $(date -u '+%Y-%m-%dT%H:%M:%SZ')
# Template: ${TEMPLATE_PATH}
#"

# For dual mode, prepend header to template-based config
if [[ "$MODE" == "dual" ]]; then
    CONFIG="${HEADER}
${CONFIG}"
fi

# --- Validate with Xorg dry-run (best-effort) ---

VALIDATION_OK=true
if command -v Xorg &>/dev/null; then
    log "Validating generated config with Xorg -configure dry-run..."

    TMPCONF=$(mktemp /tmp/xorg-validate-XXXXXX.conf)
    echo "$CONFIG" > "$TMPCONF"

    # Xorg -config test: parse-only validation
    # -configdir /dev/null prevents loading drop-in snippets
    if Xorg -config "$TMPCONF" -configdir /dev/null -validate 2>/dev/null; then
        log "Xorg config validation: PASSED"
    else
        # -validate may not be available; try a syntax check via config parse
        # Xorg returns 0 on parse success even without displays
        if Xorg -config "$TMPCONF" -configure 2>/dev/null; then
            log "Xorg config syntax check: PASSED (via -configure)"
        else
            log "WARNING: Xorg config validation could not confirm syntax"
            log "The config will still be written — verify manually with:"
            log "  Xorg -config $OUTPUT_PATH -configdir /dev/null :99"
            VALIDATION_OK=false
        fi
    fi

    rm -f "$TMPCONF"
else
    log "Xorg binary not found — skipping config validation"
    log "Install xserver-xorg-core to enable validation"
fi

# --- Output ---

if [[ "$DRY_RUN" == true ]]; then
    echo "$CONFIG"
    echo "" >&2
    echo "=== gen-xorg.sh dry-run summary ===" >&2
    echo "Mode:      ${MODE}-GPU" >&2
    echo "GPU0:      ${GPU_NAMES[0]} at ${GPU_ADDRS[0]} (${GPU0_BUSID})" >&2
    if [[ "$MODE" == "dual" ]]; then
        echo "GPU1:      ${GPU_NAMES[1]} at ${GPU_ADDRS[1]} (${GPU1_BUSID})" >&2
    fi
    echo "Monitors:  7" >&2
    echo "Output:    (stdout, not written)" >&2
    if [[ "$VALIDATION_OK" == false ]]; then
        echo "Validate:  WARNING — could not confirm Xorg syntax" >&2
    fi
else
    mkdir -p "$(dirname "$OUTPUT_PATH")"
    echo "$CONFIG" > "$OUTPUT_PATH"
    chmod 644 "$OUTPUT_PATH"
    echo "[${LOG_TAG}] Wrote ${OUTPUT_PATH} (${MODE}-GPU, 7 monitors)"
    echo "[${LOG_TAG}] GPU0: ${GPU_NAMES[0]} at ${GPU_ADDRS[0]} (${GPU0_BUSID})"
    if [[ "$MODE" == "dual" ]]; then
        echo "[${LOG_TAG}] GPU1: ${GPU_NAMES[1]} at ${GPU_ADDRS[1]} (${GPU1_BUSID})"
    fi
    if [[ "$VALIDATION_OK" == false ]]; then
        echo "[${LOG_TAG}] WARNING: Xorg validation inconclusive — verify config manually"
    fi
fi
