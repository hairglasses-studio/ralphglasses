# ralphglasses distro — Bootable Linux ISO

Build a minimal bootable Linux ISO that boots straight into the ralphglasses TUI on i3, with full NVIDIA dual-GPU support for 7 monitors.

## Quick start

```bash
cd distro/

# Build the ISO (requires Docker + xorriso + squashfs-tools)
make iso

# Test in QEMU
make test-vm

# Write to USB drive
make usb DEVICE=/dev/sdX
```

## Prerequisites

On the build machine:

```bash
# Ubuntu/Debian
sudo apt install docker.io xorriso squashfs-tools mtools ovmf qemu-system-x86

# Arch
sudo pacman -S docker xorriso squashfs-tools mtools edk2-ovmf qemu-system-x86
```

## Build targets

| Target | Description |
|--------|-------------|
| `make docker` | Build Docker image only (for testing/iterating) |
| `make iso` | Build bootable UEFI ISO from Docker image |
| `make test-vm` | Launch ISO in QEMU with UEFI firmware |
| `make usb DEVICE=/dev/sdX` | Write ISO to USB drive (destructive!) |
| `make clean` | Remove build artifacts and Docker image |

## What's in the ISO

- **Ubuntu 24.04** base with systemd
- **i3** window manager with 7-monitor workspace assignments
- **alacritty** terminal
- **NVIDIA driver 550** (proprietary, for dual GPU)
- **autorandr** for multi-monitor persistence
- **Go runtime** (to rebuild ralphglasses)
- **Node.js + Claude Code CLI**
- **ralphglasses** binary (built from source)
- **User `ralph`** with passwordless sudo, autologin to i3
- Boots to graphical target, auto-starts ralphglasses TUI on workspace 1

## NVIDIA dual GPU / 7 monitors

The ISO includes NVIDIA driver 550 and a baseline Xorg config at `/etc/X11/xorg.conf.d/20-nvidia.conf` that defines two GPU devices. After first boot:

1. Run `lspci | grep -i nvidia` to find actual PCI bus IDs
2. Run `nvidia-xconfig --enable-all-gpus` to regenerate the config
3. Run `xrandr --listmonitors` to find output names
4. Edit `~/.config/i3/config` to update `$mon1`-`$mon7` variables
5. Save the layout: `autorandr --save default`

The i3 config maps workspaces 1-7 to monitors and overflow workspaces 8-10 cycle back.

### Driver version

The Dockerfile uses `nvidia-driver-550`. To change:

```bash
docker build --build-arg NVIDIA_DRIVER_VERSION=555 -t ralphglasses-os distro/
```

Check available versions: `apt list nvidia-driver-*`

## Triple-boot: Windows 11 + Ubuntu Desktop + ralphglasses

This ISO is designed for a dedicated partition alongside existing Windows 11 and Ubuntu Desktop installations.

### Partition layout (example for 2TB NVMe)

| Partition | Size | Type | Content |
|-----------|------|------|---------|
| EFI System | 512MB | FAT32 | Shared EFI partition (all three OSes) |
| Windows C: | 500GB | NTFS | Windows 11 |
| Ubuntu root | 500GB | ext4 | Ubuntu Desktop |
| ralphglasses | 200GB | ext4 | This distro |
| Shared data | remaining | ext4/NTFS | `/workspace` mount point |

### Installation steps

1. **Boot the ISO** from USB (F12 / boot menu on your BIOS)
2. **From the live environment**, partition the target disk:
   ```bash
   # Identify free space or shrink an existing partition
   sudo parted /dev/nvme0n1 print

   # Create partition for ralphglasses
   sudo parted /dev/nvme0n1 mkpart primary ext4 1000GB 1200GB

   # Format
   sudo mkfs.ext4 -L ralphglasses /dev/nvme0n1pN
   ```
3. **Copy the live rootfs to disk**:
   ```bash
   sudo mount /dev/nvme0n1pN /mnt
   sudo unsquashfs -d /mnt /live/filesystem.squashfs
   ```
4. **Install GRUB to the shared EFI partition**:
   ```bash
   sudo mount /dev/nvme0n1p1 /mnt/boot/efi  # existing EFI partition
   sudo chroot /mnt
   grub-install --target=x86_64-efi --efi-directory=/boot/efi --bootloader-id=ralphglasses
   update-grub
   exit
   ```
5. **Reboot** — the UEFI boot menu will show ralphglasses alongside Windows and Ubuntu

### Boot manager

After installation, all three OSes appear in the UEFI firmware boot menu. You can also use:
- `efibootmgr` to reorder boot entries
- `sudo update-grub` from Ubuntu to detect all installed OSes
- Hold F12 (or your board's key) at POST to pick an OS

## Customization

### Add packages

Edit `distro/Dockerfile` and add to the `apt-get install` line, then rebuild:

```bash
make clean && make iso
```

### Change i3 config

Edit `distro/i3/config` directly. The Dockerfile copies it into the image at build time.

### Change monitor layout

After booting, configure monitors with `arandr` (GUI) or `xrandr` (CLI), then:

```bash
autorandr --save default
```

The saved profile persists across reboots via `autorandr --change` in the i3 config.

## Architecture

```
distro/
  Dockerfile           # OS definition (Ubuntu 24.04 + i3 + NVIDIA + ralphglasses)
  Makefile             # Build targets (iso, docker, test-vm, usb, clean)
  README.md            # This file
  grub/
    grub.cfg           # UEFI boot menu
  i3/
    config             # i3 window manager config (7 monitors)
  systemd/
    ralphglasses.service  # systemd unit for TUI autostart
  autorandr/           # Saved monitor profiles (populated after first setup)
  dietpi/              # Legacy DietPi config (deprecated, kept for reference)
  pxe/                 # PXE network boot docs (alternative to ISO)
```

## Kairos (optional)

The Dockerfile produces a standard rootfs. For immutable OS features (A/B updates, reset), you can layer Kairos on top:

```bash
# Install AuroraBoot
docker pull quay.io/kairos/auroraboot

# Convert the Docker image to a Kairos-compatible ISO
docker run --rm -v "$PWD":/output \
    quay.io/kairos/auroraboot \
    --set container_image=ralphglasses-os:latest \
    --set "disable_netboot=true" \
    --set "output_type=iso"
```

This gives you Kairos features like:
- Immutable rootfs with overlay
- A/B partition upgrades
- Cloud-config based provisioning
- Factory reset capability

See https://kairos.io/docs/ for details.
