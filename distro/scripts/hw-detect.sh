#!/usr/bin/env bash
#
# hw-detect.sh — First-boot hardware detection for ralphglasses thin client
#
# Detects NVIDIA GPUs, configures Xorg for RTX 4090 only, blacklists
# conflicting hardware (GTX 1060, MT7927 Bluetooth), and enables AMD iGPU
# as a fallback display.
#
# Usage:
#   hw-detect.sh                       # Run as root, writes Xorg configs
#   hw-detect.sh --wayland-only        # Run as root, writes Sway configs (skip Xorg)
#   hw-detect.sh --dry-run             # Print what would be done, write nothing
#   hw-detect.sh --wayland-only --dry-run  # Dry run for Wayland path
#
# Designed for: ASUS ProArt X870E-CREATOR WIFI
# See: distro/hardware/proart-x870e.md

set -euo pipefail

# --- Constants ---

# NVIDIA PCI device IDs
NVIDIA_RTX4090_DEVID="2684"   # Ada Lovelace
NVIDIA_GTX1060_DEVID="1c03"   # Pascal 6GB

# AMD iGPU device ID (Ryzen 7950X RDNA2)
AMD_IGPU_DEVID="164e"

# Output paths
XORG_CONF_DIR="/etc/X11/xorg.conf.d"
XORG_GPU_CONF="${XORG_CONF_DIR}/20-gpu.conf"
MODPROBE_DIR="/etc/modprobe.d"
BLACKLIST_BT="${MODPROBE_DIR}/blacklist-btmtk.conf"
BLACKLIST_GTX1060="${MODPROBE_DIR}/blacklist-gtx1060.conf"
LOG_FILE="/var/log/hw-detect.log"

# --- Flags ---
DRY_RUN=false
WAYLAND_ONLY=false
for arg in "$@"; do
    case "$arg" in
        --dry-run)      DRY_RUN=true ;;
        --wayland-only) WAYLAND_ONLY=true ;;
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

write_file() {
    local path="$1"
    local content="$2"
    if [[ "$DRY_RUN" == true ]]; then
        echo "--- DRY RUN: Would write $path ---"
        echo "$content"
        echo "--- END ---"
        echo
    else
        mkdir -p "$(dirname "$path")"
        echo "$content" > "$path"
        log "Wrote $path"
    fi
}

# --- Check prerequisites ---

if [[ "$DRY_RUN" == false ]] && [[ $EUID -ne 0 ]]; then
    echo "ERROR: Must run as root (or use --dry-run)" >&2
    exit 1
fi

if ! command -v lspci &>/dev/null; then
    echo "ERROR: lspci not found. Install pciutils: apt install pciutils" >&2
    exit 1
fi

# --- Begin detection ---

log "=== hw-detect.sh starting ==="
log "Mode: $(if $DRY_RUN; then echo 'DRY RUN'; else echo 'LIVE'; fi)"
log "Hostname: $(hostname)"
log "Kernel: $(uname -r)"

# --- Enumerate PCI devices ---

log "--- PCI Device Scan ---"

LSPCI_OUTPUT=$(lspci -nn 2>/dev/null || true)

if [[ -z "$LSPCI_OUTPUT" ]]; then
    log "WARNING: lspci returned no output (normal on WSL without PCI passthrough)"
    log "On real hardware, this script will detect PCI devices"
    if [[ "$DRY_RUN" == true ]]; then
        echo "NOTE: No PCI devices found. On WSL, this is expected."
        echo "On real hardware, lspci will enumerate all devices."
    fi
fi

# Log full PCI output
log "Full lspci output:"
while IFS= read -r line; do
    log "  $line"
done <<< "$LSPCI_OUTPUT"

# --- Detect NVIDIA GPUs ---

log "--- NVIDIA GPU Detection ---"

RTX4090_BUS=""
GTX1060_BUS=""

# Find RTX 4090 by device ID
while IFS= read -r line; do
    if [[ -n "$line" ]]; then
        # Extract PCI bus ID (e.g., "01:00.0")
        bus_id=$(echo "$line" | awk '{print $1}')
        RTX4090_BUS="$bus_id"
        log "Found RTX 4090 at PCI $bus_id"
    fi
done < <(echo "$LSPCI_OUTPUT" | grep -i "10de:${NVIDIA_RTX4090_DEVID}" 2>/dev/null || true)

# Find GTX 1060 by device ID
while IFS= read -r line; do
    if [[ -n "$line" ]]; then
        bus_id=$(echo "$line" | awk '{print $1}')
        GTX1060_BUS="$bus_id"
        log "Found GTX 1060 at PCI $bus_id"
    fi
done < <(echo "$LSPCI_OUTPUT" | grep -i "10de:${NVIDIA_GTX1060_DEVID}" 2>/dev/null || true)

# Also check for any other NVIDIA GPUs
while IFS= read -r line; do
    if [[ -n "$line" ]]; then
        log "NVIDIA device: $line"
    fi
done < <(echo "$LSPCI_OUTPUT" | grep -i "10de:" 2>/dev/null || true)

# --- Detect AMD iGPU ---

log "--- AMD iGPU Detection ---"

AMD_IGPU_BUS=""
while IFS= read -r line; do
    if [[ -n "$line" ]]; then
        bus_id=$(echo "$line" | awk '{print $1}')
        AMD_IGPU_BUS="$bus_id"
        log "Found AMD iGPU (RDNA2) at PCI $bus_id"
    fi
done < <(echo "$LSPCI_OUTPUT" | grep -i "1002:${AMD_IGPU_DEVID}" 2>/dev/null || true)

# --- Detect Network Interfaces ---

log "--- Network Detection ---"

# Intel I226-V
if echo "$LSPCI_OUTPUT" | grep -qi "8086:125c"; then
    log "Found Intel I226-V 2.5GbE (igc driver)"
else
    log "WARNING: Intel I226-V not detected — check BIOS or kernel 5.15+"
fi

# MediaTek MT7927 WiFi
if echo "$LSPCI_OUTPUT" | grep -qi "14c3:7927"; then
    log "Found MediaTek MT7927 WiFi 7 (mt7925e driver)"
else
    log "MediaTek MT7927 WiFi not detected (may need kernel 6.8+)"
fi

# Marvell 10GbE
if echo "$LSPCI_OUTPUT" | grep -qi "1d6a:"; then
    log "Found Marvell AQtion 10GbE (atlantic driver) — known stability issues"
fi

# --- Generate Sway/Wayland Config (if --wayland-only) ---

if [[ "$WAYLAND_ONLY" == true ]]; then
    log "--- Sway/Wayland Configuration ---"

    # Verify nvidia-drm.modeset=1 in kernel cmdline
    if [[ -f /proc/cmdline ]]; then
        if ! grep -q "nvidia-drm.modeset=1" /proc/cmdline 2>/dev/null; then
            warn "WARNING: nvidia-drm.modeset=1 not found in kernel cmdline"
            warn "Add to GRUB_CMDLINE_LINUX in /etc/default/grub and run update-grub"
        else
            log "nvidia-drm.modeset=1 confirmed in kernel cmdline"
        fi
    fi

    # Write Sway monitor config (auto-detected outputs)
    SWAY_CONF_DIR="/etc/sway/config.d"
    SWAY_MONITORS="${SWAY_CONF_DIR}/monitors.conf"

    SWAY_CONTENT="# Generated by hw-detect.sh --wayland-only
# Sway output configuration for detected GPUs
# Customize positions to match your physical monitor layout

"
    if [[ -n "$RTX4090_BUS" ]]; then
        SWAY_CONTENT+="# RTX 4090 outputs (PCI $RTX4090_BUS)
# output DP-1 resolution 3840x2160 position 0,0
# output DP-2 resolution 3840x2160 position 3840,0
# output DP-3 resolution 3840x2160 position 7680,0
# output HDMI-A-1 resolution 3840x2160 position 11520,0
"
    fi

    if [[ -n "$AMD_IGPU_BUS" ]]; then
        SWAY_CONTENT+="
# AMD iGPU outputs (PCI $AMD_IGPU_BUS) — secondary/overflow
# output HDMI-A-2 resolution 1920x1080 position 15360,0
# output DP-4 resolution 1920x1080 position 17280,0
# output DP-5 resolution 1920x1080 position 19200,0
"
    fi

    # Set WLR_DRM_DEVICES for GPU ordering (NVIDIA first)
    if [[ -n "$RTX4090_BUS" ]] && [[ -n "$AMD_IGPU_BUS" ]]; then
        SWAY_CONTENT+="
# Dual GPU: NVIDIA primary, AMD secondary
# Set WLR_DRM_DEVICES in environment.d or sway config:
# exec export WLR_DRM_DEVICES=/dev/dri/card0:/dev/dri/card1
"
    fi

    write_file "$SWAY_MONITORS" "$SWAY_CONTENT"
    log "Wrote Sway monitor config to $SWAY_MONITORS"
fi

# --- Write monitors.env (used by kiosk config templating, both X11 and Wayland) ---

MONITORS_ENV_DIR="/etc/ralphglasses"
MONITORS_ENV="${MONITORS_ENV_DIR}/monitors.env"

MONITORS_ENV_CONTENT="# Generated by hw-detect.sh
# Source this file to get monitor output names
RTX4090_BUS=${RTX4090_BUS:-}
AMD_IGPU_BUS=${AMD_IGPU_BUS:-}
GTX1060_BUS=${GTX1060_BUS:-}
WAYLAND_ONLY=${WAYLAND_ONLY}
"
write_file "$MONITORS_ENV" "$MONITORS_ENV_CONTENT"

# --- Generate Xorg GPU Config (skip if --wayland-only) ---

if [[ "$WAYLAND_ONLY" == true ]]; then
    log "--- Skipping Xorg Configuration (--wayland-only) ---"
else

log "--- Xorg Configuration ---"

if [[ -n "$RTX4090_BUS" ]]; then
    # Convert PCI bus ID format: "01:00.0" -> "PCI:1:0:0" (hex to decimal)
    IFS=':.' read -r domain bus dev func <<< "$RTX4090_BUS"
    xorg_bus_id="PCI:$((16#$bus)):$((16#$dev)):$func"

    XORG_CONTENT="# Generated by hw-detect.sh — RTX 4090 as primary display
# GTX 1060 is blacklisted; AMD iGPU available as secondary via amdgpu

Section \"Device\"
    Identifier  \"nvidia-rtx4090\"
    Driver      \"nvidia\"
    BusID       \"${xorg_bus_id}\"
    Option      \"AllowEmptyInitialConfiguration\" \"true\"
EndSection"

    log "RTX 4090 Xorg BusID: ${xorg_bus_id}"
    write_file "$XORG_GPU_CONF" "$XORG_CONTENT"
else
    log "No RTX 4090 detected — skipping Xorg nvidia config"

    # If AMD iGPU present, configure as fallback
    if [[ -n "$AMD_IGPU_BUS" ]]; then
        IFS=':.' read -r domain bus dev func <<< "$AMD_IGPU_BUS"
        amd_bus_id="PCI:$((16#$bus)):$((16#$dev)):$func"

        XORG_CONTENT="# Generated by hw-detect.sh — AMD iGPU as primary (no NVIDIA detected)

Section \"Device\"
    Identifier  \"amd-igpu\"
    Driver      \"amdgpu\"
    BusID       \"${amd_bus_id}\"
EndSection"

        log "AMD iGPU Xorg BusID: ${amd_bus_id}"
        write_file "$XORG_GPU_CONF" "$XORG_CONTENT"
    else
        log "WARNING: No supported GPU detected for Xorg config"
    fi
fi

# --- Blacklist GTX 1060 ---

if [[ -n "$GTX1060_BUS" ]]; then
    log "--- GTX 1060 Blacklist ---"

    BLACKLIST_CONTENT="# Generated by hw-detect.sh
# Prevent nvidia driver from binding to GTX 1060 (Pascal) at PCI ${GTX1060_BUS}
# RTX 4090 (Ada) uses nvidia-driver-550; GTX 1060 needs legacy 560.x which conflicts
#
# This uses nouveau blacklist + nvidia GPU exclusion by PCI bus ID
softdep nouveau pre: blacklist-gtx1060
blacklist nouveau

# Tell nvidia to skip this GPU by PCI address
# The nvidia driver will only bind to the RTX 4090
options nvidia NVreg_ExcludedGpus=0000:${GTX1060_BUS}"

    write_file "$BLACKLIST_GTX1060" "$BLACKLIST_CONTENT"
    log "GTX 1060 at PCI $GTX1060_BUS will be excluded from nvidia driver"
fi

# --- Blacklist MT7927 Bluetooth ---

log "--- Bluetooth Blacklist ---"

BT_CONTENT="# Generated by hw-detect.sh
# MediaTek MT7927 Bluetooth has hardware-level HCI timeout errors
# on ASUS ProArt X870E-CREATOR WIFI. Blacklist to prevent log spam.
blacklist btmtk
blacklist btmtk_usb
blacklist btmtk_pci"

write_file "$BLACKLIST_BT" "$BT_CONTENT"
log "MT7927 Bluetooth modules blacklisted (btmtk, btmtk_usb, btmtk_pci)"

# --- AMD iGPU Secondary Display ---

if [[ -n "$AMD_IGPU_BUS" ]] && [[ -n "$RTX4090_BUS" ]]; then
    log "--- AMD iGPU Secondary Display ---"
    log "AMD iGPU available at PCI $AMD_IGPU_BUS as secondary display via amdgpu"
    log "amdgpu module loads automatically — no config needed"
    if [[ "$WAYLAND_ONLY" == true ]]; then
        log "To use: Sway auto-detects outputs via WLR_DRM_DEVICES"
    else
        log "To use: connect monitor to motherboard HDMI/DP, then xrandr --auto"
    fi
fi

fi  # end of: if WAYLAND_ONLY == true ... else (Xorg path)

# --- Summary ---

log ""
log "=== hw-detect.sh Summary ==="
log "RTX 4090:       $(if [[ -n "$RTX4090_BUS" ]]; then echo "FOUND at PCI $RTX4090_BUS (primary display)"; else echo "NOT FOUND"; fi)"
log "GTX 1060:       $(if [[ -n "$GTX1060_BUS" ]]; then echo "FOUND at PCI $GTX1060_BUS (BLACKLISTED)"; else echo "not present"; fi)"
log "AMD iGPU:       $(if [[ -n "$AMD_IGPU_BUS" ]]; then echo "FOUND at PCI $AMD_IGPU_BUS (fallback/secondary)"; else echo "not present"; fi)"
log "Intel I226-V:   $(if echo "$LSPCI_OUTPUT" | grep -qi '8086:125c'; then echo 'FOUND (primary network)'; else echo 'NOT FOUND'; fi)"
log "MT7927 WiFi:    $(if echo "$LSPCI_OUTPUT" | grep -qi '14c3:7927'; then echo 'FOUND (optional)'; else echo 'not detected'; fi)"
log "MT7927 BT:      BLACKLISTED (hardware broken)"
log ""
log "=== hw-detect.sh complete ==="
