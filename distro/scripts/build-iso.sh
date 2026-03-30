#!/bin/bash
# build-iso.sh — Build a bootable UEFI ISO for the ralphglasses thin client
#
# Takes a prepared rootfs directory (from Docker export or debootstrap) and
# produces a hybrid BIOS+UEFI ISO using grub-mkrescue.
#
# Prerequisites:
#   grub-mkrescue, grub-pc-bin, grub-efi-amd64-bin, mksquashfs, xorriso
#
# Usage:
#   ./build-iso.sh <rootfs-dir>
#   ./build-iso.sh --output /tmp/ralphglasses.iso <rootfs-dir>
#
# See also:
#   distro/Makefile              — full build pipeline (make iso)
#   distro/grub/grub.cfg         — GRUB menu entries
#   distro/scripts/chroot-setup.sh — rootfs preparation

set -euo pipefail

# ── Defaults ────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DISTRO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERSION="${VERSION:-0.1.0}"
OUTPUT_ISO=""
ROOTFS_DIR=""
WORK_DIR=""
RALPHGLASSES_BIN=""

# ── Usage ───────────────────────────────────────────────────────────────
usage() {
    cat <<EOF
Usage: $(basename "$0") [OPTIONS] <rootfs-dir>

Build a bootable UEFI ISO from a prepared rootfs directory.

Arguments:
  rootfs-dir              Path to the root filesystem (from Docker export or debootstrap)

Options:
  --output, -o PATH       Output ISO path (default: ralphglasses-VERSION-amd64.iso)
  --binary, -b PATH       Path to ralphglasses binary (default: auto-detect from rootfs or build)
  --version, -v VERSION   Version string for ISO filename (default: ${VERSION})
  --help, -h              Show this help message

Examples:
  $(basename "$0") build/rootfs
  $(basename "$0") --output /tmp/rg.iso --binary ./ralphglasses build/chroot
EOF
    exit "${1:-0}"
}

# ── Parse arguments ─────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        --output|-o)
            OUTPUT_ISO="$2"
            shift 2
            ;;
        --binary|-b)
            RALPHGLASSES_BIN="$2"
            shift 2
            ;;
        --version|-v)
            VERSION="$2"
            shift 2
            ;;
        --help|-h)
            usage 0
            ;;
        -*)
            echo "ERROR: Unknown option: $1" >&2
            usage 1
            ;;
        *)
            ROOTFS_DIR="$1"
            shift
            ;;
    esac
done

if [[ -z "${ROOTFS_DIR}" ]]; then
    echo "ERROR: rootfs directory is required" >&2
    usage 1
fi

if [[ ! -d "${ROOTFS_DIR}" ]]; then
    echo "ERROR: rootfs directory does not exist: ${ROOTFS_DIR}" >&2
    exit 1
fi

# Resolve to absolute path
ROOTFS_DIR="$(cd "${ROOTFS_DIR}" && pwd)"

# Default output path
if [[ -z "${OUTPUT_ISO}" ]]; then
    OUTPUT_ISO="ralphglasses-${VERSION}-amd64.iso"
fi

# ── Check dependencies ──────────────────────────────────────────────────
check_deps() {
    local missing=()
    for cmd in grub-mkrescue mksquashfs xorriso; do
        if ! command -v "$cmd" &>/dev/null; then
            missing+=("$cmd")
        fi
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        echo "ERROR: Missing required tools: ${missing[*]}" >&2
        echo "Install with: sudo apt-get install -y grub-pc-bin grub-efi-amd64-bin squashfs-tools xorriso mtools" >&2
        exit 1
    fi
}

# ── Locate kernel and initrd ────────────────────────────────────────────
find_kernel() {
    local vmlinuz initrd

    # Look for kernel in rootfs /boot
    vmlinuz=$(ls -1 "${ROOTFS_DIR}"/boot/vmlinuz-* 2>/dev/null | sort -V | tail -1)
    initrd=$(ls -1 "${ROOTFS_DIR}"/boot/initrd.img-* 2>/dev/null | sort -V | tail -1)

    if [[ -z "${vmlinuz}" ]]; then
        echo "ERROR: No vmlinuz found in ${ROOTFS_DIR}/boot/" >&2
        echo "Ensure the rootfs has a kernel installed (linux-image-generic)." >&2
        exit 1
    fi

    if [[ -z "${initrd}" ]]; then
        echo "ERROR: No initrd.img found in ${ROOTFS_DIR}/boot/" >&2
        echo "Ensure the rootfs has an initramfs (live-boot-initramfs-tools)." >&2
        exit 1
    fi

    echo "${vmlinuz}:${initrd}"
}

# ── Locate ralphglasses binary ──────────────────────────────────────────
find_binary() {
    if [[ -n "${RALPHGLASSES_BIN}" ]]; then
        if [[ ! -f "${RALPHGLASSES_BIN}" ]]; then
            echo "ERROR: Specified binary not found: ${RALPHGLASSES_BIN}" >&2
            exit 1
        fi
        echo "${RALPHGLASSES_BIN}"
        return
    fi

    # Check rootfs first
    for path in \
        "${ROOTFS_DIR}/usr/local/bin/ralphglasses" \
        "${ROOTFS_DIR}/usr/bin/ralphglasses" \
        "${ROOTFS_DIR}/opt/ralphglasses/ralphglasses"; do
        if [[ -f "${path}" ]]; then
            echo "${path}"
            return
        fi
    done

    # Check project build output
    local project_root
    project_root="$(cd "${DISTRO_DIR}/.." && pwd)"
    if [[ -f "${project_root}/ralphglasses" ]]; then
        echo "${project_root}/ralphglasses"
        return
    fi

    echo "WARN: ralphglasses binary not found; it should already be in the rootfs." >&2
    echo ""
}

# ── Build ISO ───────────────────────────────────────────────────────────
build_iso() {
    check_deps

    echo "==> Building ralphglasses ISO (v${VERSION})"
    echo "    Rootfs:  ${ROOTFS_DIR}"
    echo "    Output:  ${OUTPUT_ISO}"

    # Create temporary working directory
    WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/ralphglasses-iso.XXXXXX")"
    trap cleanup EXIT

    local iso_root="${WORK_DIR}/iso"

    # Set up ISO directory structure for grub-mkrescue
    mkdir -p "${iso_root}/boot/grub"
    mkdir -p "${iso_root}/EFI/BOOT"
    mkdir -p "${iso_root}/live"

    # ── Kernel + initrd ──────────────────────────────────────────────
    echo "==> Copying kernel and initrd..."
    local kernel_info
    kernel_info="$(find_kernel)"
    local vmlinuz="${kernel_info%%:*}"
    local initrd="${kernel_info##*:}"

    cp "${vmlinuz}" "${iso_root}/live/vmlinuz"
    cp "${initrd}" "${iso_root}/live/initrd.img"
    echo "    Kernel: $(basename "${vmlinuz}")"
    echo "    Initrd: $(basename "${initrd}")"

    # ── ralphglasses binary ──────────────────────────────────────────
    local rg_bin
    rg_bin="$(find_binary)"
    if [[ -n "${rg_bin}" && ! "${rg_bin}" == "${ROOTFS_DIR}/"* ]]; then
        # Binary is outside rootfs; inject it before squashing
        echo "==> Injecting ralphglasses binary into rootfs..."
        sudo cp "${rg_bin}" "${ROOTFS_DIR}/usr/local/bin/ralphglasses"
        sudo chmod 755 "${ROOTFS_DIR}/usr/local/bin/ralphglasses"
    fi

    # ── Squashfs ─────────────────────────────────────────────────────
    echo "==> Creating squashfs (this may take a while)..."
    sudo mksquashfs "${ROOTFS_DIR}" "${iso_root}/live/filesystem.squashfs" \
        -comp xz -Xbcj x86 -b 1M -no-duplicates -noappend \
        -e boot/vmlinuz-\* boot/initrd.img-\*

    local squash_size
    squash_size="$(du -h "${iso_root}/live/filesystem.squashfs" | cut -f1)"
    echo "    Squashfs size: ${squash_size}"

    # ── GRUB config ──────────────────────────────────────────────────
    echo "==> Installing GRUB configuration..."
    if [[ -f "${DISTRO_DIR}/grub/grub.cfg" ]]; then
        cp "${DISTRO_DIR}/grub/grub.cfg" "${iso_root}/boot/grub/grub.cfg"
    else
        echo "ERROR: GRUB config not found at ${DISTRO_DIR}/grub/grub.cfg" >&2
        exit 1
    fi

    # ── EFI bootloader ───────────────────────────────────────────────
    echo "==> Setting up EFI bootloader..."
    if [[ -f "${ROOTFS_DIR}/usr/lib/shim/shimx64.efi.signed" ]]; then
        cp "${ROOTFS_DIR}/usr/lib/shim/shimx64.efi.signed" "${iso_root}/EFI/BOOT/BOOTX64.EFI"
        cp "${ROOTFS_DIR}/usr/lib/grub/x86_64-efi-signed/grubx64.efi.signed" \
            "${iso_root}/EFI/BOOT/grubx64.efi"
        echo "    Using signed shim + GRUB EFI"
    elif [[ -f "${ROOTFS_DIR}/usr/lib/grub/x86_64-efi/monolithic/grubx64.efi" ]]; then
        cp "${ROOTFS_DIR}/usr/lib/grub/x86_64-efi/monolithic/grubx64.efi" \
            "${iso_root}/EFI/BOOT/BOOTX64.EFI"
        echo "    Using unsigned GRUB EFI"
    else
        echo "    No prebuilt EFI found; grub-mkrescue will generate one"
    fi

    # ── Build ISO with grub-mkrescue ─────────────────────────────────
    echo "==> Running grub-mkrescue..."
    grub-mkrescue \
        -o "${OUTPUT_ISO}" \
        --product-name="ralphglasses" \
        --product-version="${VERSION}" \
        -- \
        -volid "RALPHGLASSES" \
        -iso-level 3 \
        -full-iso9660-filenames \
        "${iso_root}"

    # ── Summary ──────────────────────────────────────────────────────
    echo ""
    echo "==> ISO built successfully: ${OUTPUT_ISO}"
    ls -lh "${OUTPUT_ISO}"
    echo "    SHA256: $(sha256sum "${OUTPUT_ISO}" | cut -d' ' -f1)"
    echo ""
    echo "Test with QEMU:"
    echo "    qemu-system-x86_64 -enable-kvm -m 4096 -bios /usr/share/OVMF/OVMF_CODE.fd -cdrom ${OUTPUT_ISO} -boot d"
    echo ""
    echo "Write to USB:"
    echo "    sudo dd if=${OUTPUT_ISO} of=/dev/sdX bs=4M status=progress oflag=sync"
}

# ── Cleanup ─────────────────────────────────────────────────────────────
cleanup() {
    if [[ -n "${WORK_DIR}" && -d "${WORK_DIR}" ]]; then
        rm -rf "${WORK_DIR}"
    fi
}

# ── Main ────────────────────────────────────────────────────────────────
build_iso
