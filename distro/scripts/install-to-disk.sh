#!/usr/bin/env bash
#
# install-to-disk.sh — Install ralphglasses thin client to a target disk
#
# Interactive installer that partitions a disk (GPT: EFI + swap + root),
# copies the live rootfs, installs GRUB, and configures the system for
# first boot on the ASUS ProArt X870E-CREATOR WIFI target hardware.
#
# Usage:
#   sudo install-to-disk.sh                     # Interactive mode
#   sudo install-to-disk.sh --unattended FILE   # Unattended mode with config
#   sudo install-to-disk.sh --help
#
# The --unattended config file is a shell-sourceable file with variables:
#   TARGET_DISK=/dev/nvme0n1
#   HOSTNAME=ralphglasses
#   USERNAME=ralph
#   PASSWORD=changeme
#   TIMEZONE=America/New_York
#   SWAP_SIZE=16G
#   NETWORK_DHCP=true
#   NETWORK_IFACE=enp6s0
#   SKIP_CONFIRM=true
#
# Requires: root, UEFI system, live environment with squashfs rootfs
# See: distro/hardware/proart-x870e.md

set -euo pipefail

# ── Constants ────────────────────────────────────────────────────────────

readonly SCRIPT_NAME="$(basename "$0")"
readonly VERSION="0.1.0"
readonly LOG_FILE="/var/log/install-to-disk.log"
readonly MOUNT_ROOT="/mnt/ralphglasses"
readonly LIVE_ROOTFS="/run/live/rootfsbase"
readonly SQUASHFS_PATH="/run/live/medium/live/filesystem.squashfs"
readonly EFI_SIZE="512M"
readonly DEFAULT_SWAP_SIZE="16G"
readonly DEFAULT_HOSTNAME="ralphglasses"
readonly DEFAULT_USERNAME="ralph"
readonly DEFAULT_TIMEZONE="America/New_York"

# Partition layout (GPT):
#   1: EFI System Partition (FAT32, 512M)
#   2: Swap (16G default, tunable)
#   3: Root (ext4, remainder)

# ── State ────────────────────────────────────────────────────────────────

TARGET_DISK=""
HOSTNAME_SET="${DEFAULT_HOSTNAME}"
USERNAME_SET="${DEFAULT_USERNAME}"
PASSWORD_SET=""
TIMEZONE_SET="${DEFAULT_TIMEZONE}"
SWAP_SIZE="${DEFAULT_SWAP_SIZE}"
NETWORK_DHCP="true"
NETWORK_IFACE=""
UNATTENDED="false"
SKIP_CONFIRM="false"

# Partition device paths (set after disk selection)
PART_EFI=""
PART_SWAP=""
PART_ROOT=""

# ── Helpers ──────────────────────────────────────────────────────────────

log() {
    local ts
    ts="$(date '+%Y-%m-%d %H:%M:%S')"
    printf "[%s] %s\n" "$ts" "$*" | tee -a "$LOG_FILE"
}

die() {
    log "FATAL: $*"
    exit 1
}

warn() {
    log "WARNING: $*"
}

banner() {
    echo ""
    echo "================================================================"
    echo "  $*"
    echo "================================================================"
    echo ""
}

# Prompt for confirmation. Returns 0 if confirmed, 1 otherwise.
confirm() {
    local prompt="${1:-Continue?}"
    if [[ "$SKIP_CONFIRM" == "true" ]]; then
        log "Auto-confirmed (--unattended): $prompt"
        return 0
    fi
    local reply
    read -r -p "$prompt [yes/NO]: " reply
    case "$reply" in
        yes|YES) return 0 ;;
        *) return 1 ;;
    esac
}

# Prompt for input with a default value.
prompt_input() {
    local prompt="$1"
    local default="$2"
    local varname="$3"
    if [[ "$UNATTENDED" == "true" ]]; then
        return
    fi
    local reply
    read -r -p "$prompt [$default]: " reply
    if [[ -n "$reply" ]]; then
        eval "$varname=\"$reply\""
    fi
}

# Return partition device path for a given disk and partition number.
# Handles both /dev/sdX and /dev/nvmeXnYpZ naming conventions.
partition_path() {
    local disk="$1"
    local num="$2"
    if [[ "$disk" =~ nvme|mmcblk|loop ]]; then
        echo "${disk}p${num}"
    else
        echo "${disk}${num}"
    fi
}

usage() {
    cat <<EOF
$SCRIPT_NAME v$VERSION — Install ralphglasses to disk

Usage:
  sudo $SCRIPT_NAME                       Interactive mode
  sudo $SCRIPT_NAME --unattended FILE     Unattended mode with config file
  sudo $SCRIPT_NAME --help                Show this help

Interactive mode will:
  1. List available disks and let you select one
  2. Ask for hostname, username, password, timezone
  3. Require explicit confirmation before any destructive operation
  4. Partition the disk (EFI + swap + root)
  5. Format and mount partitions
  6. Copy the live rootfs to disk
  7. Install GRUB bootloader
  8. Configure fstab, hostname, network, initial user

Unattended config file variables:
  TARGET_DISK     Target disk device (e.g., /dev/nvme0n1)
  HOSTNAME        System hostname (default: $DEFAULT_HOSTNAME)
  USERNAME        Initial user (default: $DEFAULT_USERNAME)
  PASSWORD        User password (required in unattended mode)
  TIMEZONE        Timezone (default: $DEFAULT_TIMEZONE)
  SWAP_SIZE       Swap partition size (default: $DEFAULT_SWAP_SIZE)
  NETWORK_DHCP    Use DHCP (default: true)
  NETWORK_IFACE   Network interface for static config
  SKIP_CONFIRM    Skip confirmation prompts (default: false)
EOF
    exit 0
}

# ── Preflight checks ────────────────────────────────────────────────────

preflight() {
    log "Running preflight checks..."

    # Must be root
    if [[ $EUID -ne 0 ]]; then
        die "This script must be run as root (use sudo)"
    fi

    # Must be UEFI
    if [[ ! -d /sys/firmware/efi ]]; then
        die "UEFI firmware not detected. This installer requires UEFI boot mode."
    fi

    # Required tools
    local tools=(parted mkfs.ext4 mkfs.fat mkswap mount umount rsync
                 grub-install update-grub chroot lsblk blkid)
    for tool in "${tools[@]}"; do
        if ! command -v "$tool" &>/dev/null; then
            die "Required tool not found: $tool"
        fi
    done

    # Verify rootfs source exists (either live mount or squashfs)
    if [[ -d "$LIVE_ROOTFS" ]]; then
        log "Live rootfs found at $LIVE_ROOTFS"
    elif [[ -f "$SQUASHFS_PATH" ]]; then
        log "Squashfs image found at $SQUASHFS_PATH"
    elif [[ -d /run/live/rootfs ]]; then
        log "Alternative live rootfs found at /run/live/rootfs"
    else
        warn "No live rootfs found at expected paths."
        warn "Will attempt to copy from current running system (/)."
        warn "This is only appropriate when running from the live ISO."
        if ! confirm "Continue without verified live rootfs?"; then
            die "Aborted: no rootfs source"
        fi
    fi

    log "Preflight checks passed"
}

# ── Disk selection ───────────────────────────────────────────────────────

select_disk() {
    if [[ -n "$TARGET_DISK" ]]; then
        # Already set (unattended mode)
        if [[ ! -b "$TARGET_DISK" ]]; then
            die "Target disk $TARGET_DISK does not exist or is not a block device"
        fi
        log "Target disk (from config): $TARGET_DISK"
        return
    fi

    banner "Disk Selection"

    echo "Available disks:"
    echo ""
    lsblk -d -o NAME,SIZE,MODEL,TYPE,TRAN | grep -E "disk|NAME"
    echo ""

    # Filter to only real disks (exclude loop, rom)
    local disks
    disks=$(lsblk -dnpo NAME,TYPE | awk '$2 == "disk" { print $1 }')

    if [[ -z "$disks" ]]; then
        die "No disks found"
    fi

    echo "Enter the full device path (e.g., /dev/sda or /dev/nvme0n1):"
    read -r -p "> " TARGET_DISK

    if [[ -z "$TARGET_DISK" ]]; then
        die "No disk selected"
    fi

    if [[ ! -b "$TARGET_DISK" ]]; then
        die "$TARGET_DISK is not a valid block device"
    fi

    # Safety: refuse to install on the boot device
    local boot_disk
    boot_disk=$(findmnt -no SOURCE / 2>/dev/null | sed 's/[0-9]*$//' | sed 's/p[0-9]*$//' || true)
    if [[ "$TARGET_DISK" == "$boot_disk" ]]; then
        die "Refusing to install on the currently booted disk ($boot_disk)"
    fi

    echo ""
    echo "Selected disk: $TARGET_DISK"
    lsblk -o NAME,SIZE,FSTYPE,MOUNTPOINT "$TARGET_DISK"
    echo ""
}

# ── Gather configuration ────────────────────────────────────────────────

gather_config() {
    if [[ "$UNATTENDED" == "true" ]]; then
        # Validate required unattended fields
        if [[ -z "$PASSWORD_SET" ]]; then
            die "PASSWORD is required in unattended mode"
        fi
        return
    fi

    banner "System Configuration"

    prompt_input "Hostname" "$HOSTNAME_SET" HOSTNAME_SET
    prompt_input "Username" "$USERNAME_SET" USERNAME_SET
    prompt_input "Timezone" "$TIMEZONE_SET" TIMEZONE_SET
    prompt_input "Swap size" "$SWAP_SIZE" SWAP_SIZE

    # Password (hidden input)
    while true; do
        read -r -s -p "Password for $USERNAME_SET: " PASSWORD_SET
        echo ""
        local pw_confirm
        read -r -s -p "Confirm password: " pw_confirm
        echo ""
        if [[ "$PASSWORD_SET" == "$pw_confirm" ]]; then
            break
        fi
        echo "Passwords do not match. Try again."
    done

    echo ""
    echo "Network configuration:"
    echo "  1) DHCP (automatic)"
    echo "  2) Skip network configuration"
    local net_choice
    read -r -p "Choice [1]: " net_choice
    case "${net_choice:-1}" in
        1) NETWORK_DHCP="true" ;;
        2) NETWORK_DHCP="false" ;;
        *) NETWORK_DHCP="true" ;;
    esac
}

# ── Final confirmation ───────────────────────────────────────────────────

show_summary_and_confirm() {
    banner "Installation Summary"

    cat <<EOF
  Target disk:    $TARGET_DISK
  Hostname:       $HOSTNAME_SET
  Username:       $USERNAME_SET
  Timezone:       $TIMEZONE_SET
  Swap size:      $SWAP_SIZE
  Network:        $(if [[ "$NETWORK_DHCP" == "true" ]]; then echo "DHCP"; else echo "Manual/None"; fi)

  Partition layout:
    ${TARGET_DISK}p1 / ${TARGET_DISK}1  —  EFI System Partition  ($EFI_SIZE, FAT32)
    ${TARGET_DISK}p2 / ${TARGET_DISK}2  —  Swap                  ($SWAP_SIZE)
    ${TARGET_DISK}p3 / ${TARGET_DISK}3  —  Root                  (remainder, ext4)

EOF

    echo "!!! WARNING !!!"
    echo "This will ERASE ALL DATA on $TARGET_DISK"
    echo ""

    if ! confirm "Type 'yes' to proceed with installation"; then
        die "Installation aborted by user"
    fi
}

# ── Partition disk ───────────────────────────────────────────────────────

partition_disk() {
    banner "Partitioning $TARGET_DISK"

    log "Unmounting any existing partitions on $TARGET_DISK..."
    umount "${TARGET_DISK}"* 2>/dev/null || true
    swapoff "${TARGET_DISK}"* 2>/dev/null || true

    log "Wiping existing partition table..."
    wipefs -af "$TARGET_DISK" >> "$LOG_FILE" 2>&1
    sgdisk --zap-all "$TARGET_DISK" >> "$LOG_FILE" 2>&1

    log "Creating GPT partition table..."
    parted -s "$TARGET_DISK" mklabel gpt

    log "Creating EFI partition ($EFI_SIZE)..."
    parted -s "$TARGET_DISK" mkpart "EFI" fat32 1MiB "$EFI_SIZE"
    parted -s "$TARGET_DISK" set 1 esp on

    log "Creating swap partition ($SWAP_SIZE)..."
    parted -s "$TARGET_DISK" mkpart "swap" linux-swap "$EFI_SIZE" "$((${SWAP_SIZE%G} * 1024 + 512))MiB"

    log "Creating root partition (remainder)..."
    parted -s "$TARGET_DISK" mkpart "root" ext4 "$((${SWAP_SIZE%G} * 1024 + 512))MiB" 100%

    # Wait for kernel to re-read partition table
    partprobe "$TARGET_DISK"
    sleep 2

    # Set partition device paths
    PART_EFI="$(partition_path "$TARGET_DISK" 1)"
    PART_SWAP="$(partition_path "$TARGET_DISK" 2)"
    PART_ROOT="$(partition_path "$TARGET_DISK" 3)"

    # Verify partitions exist
    for part in "$PART_EFI" "$PART_SWAP" "$PART_ROOT"; do
        if [[ ! -b "$part" ]]; then
            die "Expected partition $part not found after partitioning"
        fi
    done

    log "Partitioning complete: EFI=$PART_EFI  Swap=$PART_SWAP  Root=$PART_ROOT"
}

# ── Format partitions ───────────────────────────────────────────────────

format_partitions() {
    banner "Formatting Partitions"

    log "Formatting EFI partition ($PART_EFI) as FAT32..."
    mkfs.fat -F 32 -n "EFI" "$PART_EFI" >> "$LOG_FILE" 2>&1

    log "Formatting swap partition ($PART_SWAP)..."
    mkswap -L "swap" "$PART_SWAP" >> "$LOG_FILE" 2>&1

    log "Formatting root partition ($PART_ROOT) as ext4..."
    mkfs.ext4 -L "ralphglasses" -F "$PART_ROOT" >> "$LOG_FILE" 2>&1

    log "Formatting complete"
}

# ── Mount partitions ─────────────────────────────────────────────────────

mount_partitions() {
    log "Mounting partitions..."

    mkdir -p "$MOUNT_ROOT"
    mount "$PART_ROOT" "$MOUNT_ROOT"

    mkdir -p "$MOUNT_ROOT/boot/efi"
    mount "$PART_EFI" "$MOUNT_ROOT/boot/efi"

    log "Mounted root at $MOUNT_ROOT, EFI at $MOUNT_ROOT/boot/efi"
}

# ── Copy rootfs ──────────────────────────────────────────────────────────

copy_rootfs() {
    banner "Copying Root Filesystem"

    local source_dir=""

    if [[ -d "$LIVE_ROOTFS" ]]; then
        source_dir="$LIVE_ROOTFS"
    elif [[ -f "$SQUASHFS_PATH" ]]; then
        log "Mounting squashfs image..."
        local squash_mnt="/tmp/squashfs_mount"
        mkdir -p "$squash_mnt"
        mount -o loop,ro "$SQUASHFS_PATH" "$squash_mnt"
        source_dir="$squash_mnt"
    elif [[ -d /run/live/rootfs ]]; then
        # Some live systems use this path
        source_dir="/run/live/rootfs"
    else
        # Fallback: copy from running system (live ISO only)
        source_dir="/"
    fi

    log "Copying rootfs from $source_dir to $MOUNT_ROOT..."
    log "This may take several minutes..."

    rsync -aHAXx --info=progress2 \
        --exclude='/dev/*' \
        --exclude='/proc/*' \
        --exclude='/sys/*' \
        --exclude='/tmp/*' \
        --exclude='/run/*' \
        --exclude='/mnt/*' \
        --exclude='/media/*' \
        --exclude='/lost+found' \
        --exclude='/swapfile' \
        "$source_dir/" "$MOUNT_ROOT/"

    # Unmount squashfs if we mounted it
    if [[ -f "$SQUASHFS_PATH" ]] && mountpoint -q /tmp/squashfs_mount 2>/dev/null; then
        umount /tmp/squashfs_mount
    fi

    # Create required mount points
    mkdir -p "$MOUNT_ROOT"/{dev,proc,sys,tmp,run,mnt,media}

    log "Rootfs copy complete"
}

# ── Install GRUB ─────────────────────────────────────────────────────────

install_grub() {
    banner "Installing GRUB Bootloader"

    # Bind-mount system filesystems for chroot
    mount --bind /dev "$MOUNT_ROOT/dev"
    mount --bind /dev/pts "$MOUNT_ROOT/dev/pts"
    mount -t proc proc "$MOUNT_ROOT/proc"
    mount -t sysfs sysfs "$MOUNT_ROOT/sys"
    mount --bind /sys/firmware/efi/efivars "$MOUNT_ROOT/sys/firmware/efi/efivars" 2>/dev/null || true

    # Copy DNS config for chroot
    cp /etc/resolv.conf "$MOUNT_ROOT/etc/resolv.conf" 2>/dev/null || true

    log "Installing GRUB for x86_64-efi..."
    chroot "$MOUNT_ROOT" grub-install \
        --target=x86_64-efi \
        --efi-directory=/boot/efi \
        --bootloader-id=ralphglasses \
        --recheck \
        >> "$LOG_FILE" 2>&1

    # Write GRUB defaults for installed system
    cat > "$MOUNT_ROOT/etc/default/grub" <<'GRUBDEFAULT'
# ralphglasses GRUB defaults (installed system)
GRUB_DEFAULT=0
GRUB_TIMEOUT=5
GRUB_DISTRIBUTOR="ralphglasses"
GRUB_CMDLINE_LINUX_DEFAULT="quiet splash nvidia-drm.modeset=1"
GRUB_CMDLINE_LINUX=""
GRUB_TERMINAL="console"
GRUBDEFAULT

    log "Updating GRUB configuration..."
    chroot "$MOUNT_ROOT" update-grub >> "$LOG_FILE" 2>&1

    log "GRUB installation complete"
}

# ── Configure fstab ──────────────────────────────────────────────────────

setup_fstab() {
    log "Generating /etc/fstab..."

    local root_uuid efi_uuid swap_uuid
    root_uuid="$(blkid -s UUID -o value "$PART_ROOT")"
    efi_uuid="$(blkid -s UUID -o value "$PART_EFI")"
    swap_uuid="$(blkid -s UUID -o value "$PART_SWAP")"

    cat > "$MOUNT_ROOT/etc/fstab" <<FSTAB
# /etc/fstab — ralphglasses thin client
# Generated by install-to-disk.sh v$VERSION
#
# <filesystem>                          <mount>     <type>  <options>               <dump> <pass>
UUID=$root_uuid  /           ext4    errors=remount-ro       0      1
UUID=$efi_uuid   /boot/efi   vfat    umask=0077              0      1
UUID=$swap_uuid  none        swap    sw                      0      0
# tmpfs for /tmp (reduce disk writes)
tmpfs                                   /tmp        tmpfs   defaults,noatime,size=4G 0      0
FSTAB

    log "fstab written with UUIDs: root=$root_uuid efi=$efi_uuid swap=$swap_uuid"
}

# ── Configure hostname ───────────────────────────────────────────────────

setup_hostname() {
    log "Setting hostname to $HOSTNAME_SET..."

    echo "$HOSTNAME_SET" > "$MOUNT_ROOT/etc/hostname"

    cat > "$MOUNT_ROOT/etc/hosts" <<HOSTS
127.0.0.1   localhost
127.0.1.1   $HOSTNAME_SET

# IPv6
::1         localhost ip6-localhost ip6-loopback
ff02::1     ip6-allnodes
ff02::2     ip6-allrouters
HOSTS

    log "Hostname configured"
}

# ── Configure network ───────────────────────────────────────────────────

setup_network() {
    log "Configuring network..."

    mkdir -p "$MOUNT_ROOT/etc/netplan"

    if [[ "$NETWORK_DHCP" == "true" ]]; then
        # Auto-detect primary wired interface or use provided one
        local iface="${NETWORK_IFACE:-}"
        if [[ -z "$iface" ]]; then
            # Default to the Intel I226-V 2.5GbE (expected on ProArt X870E)
            iface="enp6s0"
        fi

        cat > "$MOUNT_ROOT/etc/netplan/01-ralphglasses.yaml" <<NETPLAN
# ralphglasses network — DHCP on wired interface
# Intel I226-V 2.5GbE recommended for marathon sessions
network:
  version: 2
  renderer: networkd
  ethernets:
    $iface:
      dhcp4: true
      dhcp6: true
    # Match any other wired interface as fallback
    fallback:
      match:
        driver: igc e1000e r8169
      dhcp4: true
NETPLAN

        log "Network configured: DHCP on $iface (with fallback)"
    else
        log "Network configuration skipped (manual)"
    fi
}

# ── Create initial user ─────────────────────────────────────────────────

setup_user() {
    banner "Creating User: $USERNAME_SET"

    # Create user with home directory and bash shell
    chroot "$MOUNT_ROOT" useradd -m -s /bin/bash -G sudo,video,render,input "$USERNAME_SET" 2>/dev/null || \
        log "User $USERNAME_SET already exists, updating..."

    # Set password
    echo "${USERNAME_SET}:${PASSWORD_SET}" | chroot "$MOUNT_ROOT" chpasswd

    # Enable passwordless sudo for the user (thin client convenience)
    mkdir -p "$MOUNT_ROOT/etc/sudoers.d"
    echo "$USERNAME_SET ALL=(ALL) NOPASSWD: ALL" > "$MOUNT_ROOT/etc/sudoers.d/90-$USERNAME_SET"
    chmod 440 "$MOUNT_ROOT/etc/sudoers.d/90-$USERNAME_SET"

    # Set timezone
    chroot "$MOUNT_ROOT" ln -sf "/usr/share/zoneinfo/$TIMEZONE_SET" /etc/localtime
    echo "$TIMEZONE_SET" > "$MOUNT_ROOT/etc/timezone"

    # Set locale
    chroot "$MOUNT_ROOT" locale-gen en_US.UTF-8 >> "$LOG_FILE" 2>&1 || true
    echo 'LANG=en_US.UTF-8' > "$MOUNT_ROOT/etc/default/locale"

    log "User $USERNAME_SET created with sudo access"
}

# ── Post-install: enable services ────────────────────────────────────────

post_install() {
    log "Running post-install tasks..."

    # Enable essential services
    chroot "$MOUNT_ROOT" systemctl enable systemd-networkd.service 2>/dev/null || true
    chroot "$MOUNT_ROOT" systemctl enable systemd-resolved.service 2>/dev/null || true

    # Enable ralphglasses services if present
    for svc in ralphglasses.service hw-detect.service rg-status-bar.timer watchdog.service; do
        if [[ -f "$MOUNT_ROOT/etc/systemd/system/$svc" ]]; then
            chroot "$MOUNT_ROOT" systemctl enable "$svc" 2>/dev/null || true
            log "Enabled service: $svc"
        fi
    done

    # Remove live-boot artifacts if present
    rm -f "$MOUNT_ROOT/etc/live" 2>/dev/null || true

    # Set machine ID (will be regenerated on first boot if empty)
    : > "$MOUNT_ROOT/etc/machine-id"

    log "Post-install tasks complete"
}

# ── Cleanup ──────────────────────────────────────────────────────────────

cleanup() {
    log "Cleaning up mounts..."

    # Unmount chroot bind-mounts (reverse order)
    umount "$MOUNT_ROOT/sys/firmware/efi/efivars" 2>/dev/null || true
    umount "$MOUNT_ROOT/sys" 2>/dev/null || true
    umount "$MOUNT_ROOT/proc" 2>/dev/null || true
    umount "$MOUNT_ROOT/dev/pts" 2>/dev/null || true
    umount "$MOUNT_ROOT/dev" 2>/dev/null || true

    # Unmount install target
    umount "$MOUNT_ROOT/boot/efi" 2>/dev/null || true
    umount "$MOUNT_ROOT" 2>/dev/null || true

    log "Cleanup complete"
}

# ── Main ─────────────────────────────────────────────────────────────────

main() {
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --help|-h)
                usage
                ;;
            --unattended)
                UNATTENDED="true"
                if [[ -z "${2:-}" ]]; then
                    die "--unattended requires a config file path"
                fi
                local config_file="$2"
                if [[ ! -f "$config_file" ]]; then
                    die "Config file not found: $config_file"
                fi
                log "Loading unattended config from $config_file"
                # Source the config file (sets TARGET_DISK, HOSTNAME, etc.)
                # shellcheck source=/dev/null
                source "$config_file"
                # Map config variable names to internal state
                HOSTNAME_SET="${HOSTNAME:-$DEFAULT_HOSTNAME}"
                USERNAME_SET="${USERNAME:-$DEFAULT_USERNAME}"
                PASSWORD_SET="${PASSWORD:-}"
                TIMEZONE_SET="${TIMEZONE:-$DEFAULT_TIMEZONE}"
                SWAP_SIZE="${SWAP_SIZE:-$DEFAULT_SWAP_SIZE}"
                NETWORK_DHCP="${NETWORK_DHCP:-true}"
                NETWORK_IFACE="${NETWORK_IFACE:-}"
                SKIP_CONFIRM="${SKIP_CONFIRM:-false}"
                TARGET_DISK="${TARGET_DISK:-}"
                shift 2
                ;;
            *)
                die "Unknown argument: $1  (use --help for usage)"
                ;;
        esac
    done

    banner "ralphglasses Disk Installer v$VERSION"

    # Set up cleanup trap
    trap cleanup EXIT

    # Initialize log
    mkdir -p "$(dirname "$LOG_FILE")"
    echo "=== install-to-disk.sh v$VERSION — $(date) ===" >> "$LOG_FILE"

    # Run installation steps
    preflight
    select_disk
    gather_config
    show_summary_and_confirm
    partition_disk
    format_partitions
    mount_partitions
    copy_rootfs
    setup_fstab
    setup_hostname
    setup_network
    install_grub
    setup_user
    post_install

    banner "Installation Complete"

    log "ralphglasses has been installed to $TARGET_DISK"
    echo ""
    echo "  Next steps:"
    echo "    1. Remove the installation media (USB/ISO)"
    echo "    2. Reboot into the UEFI firmware and set $TARGET_DISK as first boot device"
    echo "    3. Boot into ralphglasses"
    echo "    4. Log in as '$USERNAME_SET'"
    echo "    5. hw-detect.sh will run on first boot to configure GPUs"
    echo ""
    echo "  Log file: $LOG_FILE"
    echo ""
}

main "$@"
