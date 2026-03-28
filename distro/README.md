# ralphglasses distro — Claude Code Agent Thin Client

Minimal bootable Linux that starts into the ralphglasses TUI for autonomous Claude Code agent marathons.

## Philosophy

- **In-kernel drivers preferred** — the ASUS ProArt X870E-CREATOR WIFI is fully supported by mainline Linux 6.8+
- **NVIDIA via apt** — `nvidia-driver-550` installed at build time, not vendored `.run` files
- **No binary blobs in this repo** — Windows driver archives live on Google Drive, NVIDIA `.run` files go as GitHub Release artifacts if offline install is needed
- **No Windows drivers** — the 12GB Windows archive is irrelevant to Linux builds
- **Wired network only for marathons** — Intel I226-V 2.5GbE (`igc` module) is reliable for 12+ hour sessions
- **Display: i3 + RTX 4090** — NVIDIA proprietary driver for display output only (no CUDA/compute needed)
- **AMD iGPU fallback** — Ryzen 7950X RDNA2 iGPU via `amdgpu`, zero config, no conflict with NVIDIA

## Target Hardware

**ASUS ProArt X870E-CREATOR WIFI** motherboard:
- AMD Ryzen 9 7950X (16C/32T, RDNA2 iGPU)
- NVIDIA RTX 4090 (display) + GTX 1060 (disabled on Linux — driver conflict)
- 128GB DDR5-6000
- Intel I226-V 2.5GbE (primary network)
- MediaTek MT7927 WiFi 7 / Bluetooth (WiFi optional, BT hardware-broken)
- Marvell AQtion 10GbE (optional, known stability issues)

See `distro/hardware/proart-x870e.md` for the full hardware manifest with PCI IDs, kernel modules, and known issues.

## What Claude Code Needs

Claude Code is a CLI tool making Anthropic API calls. The thin client needs:

| Requirement | Solution |
|-------------|----------|
| Network | Intel I226-V wired ethernet (in-kernel `igc`) |
| Display | RTX 4090 via `nvidia-driver-550` for monitors |
| Terminal | i3 + alacritty, ralphglasses TUI fullscreen |
| Storage | Local NVMe for OS, `/workspace` for repos |
| Audio | Not needed (thin client) |
| GPU compute | Not needed (API calls, not local inference) |

## Dual-GPU Constraint

Only one `nvidia.ko` version loads at a time:
- **RTX 4090** (Ada Lovelace) needs driver 550+
- **GTX 1060** (Pascal) needs driver 560.x, which is dropped from 590+

Solution: `hw-detect.sh` blacklists the GTX 1060's PCI slot at first boot. If more monitors are needed, use the AMD iGPU via `amdgpu` (no conflict with NVIDIA).

## First-Boot Hardware Detection

The `distro/scripts/hw-detect.sh` script runs once at first boot via `distro/systemd/hw-detect.service`:

1. Enumerates PCI devices via `lspci -nn`
2. Identifies NVIDIA GPUs by device ID (Ada vs Pascal)
3. Generates `/etc/X11/xorg.conf.d/20-gpu.conf` with RTX 4090 BusID
4. Blacklists GTX 1060 PCI slot if both GPUs present
5. Blacklists `btmtk` module (MT7927 Bluetooth — hardware HCI errors)
6. Enables AMD iGPU as optional secondary display
7. Logs everything to `/var/log/hw-detect.log`

Test on WSL without making changes:

```bash
distro/scripts/hw-detect.sh --dry-run
```

## Directory Structure

```
distro/
  README.md              # This file
  hardware/
    proart-x870e.md      # Full hardware manifest (PCI IDs, modules, issues)
  scripts/
    hw-detect.sh         # First-boot hardware detection (testable with --dry-run)
  systemd/
    hw-detect.service    # Oneshot unit for first-boot detection
    ralphglasses.service # TUI autostart after display-manager
  Dockerfile             # OS build (Ubuntu 24.04 + i3 + NVIDIA) [future]
  Makefile               # Build targets (iso, docker, test-vm) [future]
  grub/                  # UEFI boot menu config [future]
  i3/                    # i3 window manager config [future]
  dietpi/                # Legacy DietPi config (deprecated)
  pxe/                   # PXE network boot docs
  autorandr/             # Monitor profiles (populated after first setup)
```

## Future Phases

**Phase 2 — Dockerfile + Build System:**
- Ubuntu 24.04 base, kernel 6.12+ HWE
- nvidia-driver-550 via apt
- Go + Node.js + Claude Code CLI
- Autologin to i3
- `docker build` -> squashfs -> ISO pipeline

**Phase 3 — Integration:**
- Connects with Go app roadmap (tests, MCP hardening, TUI polish)
- Distro work is independent and can proceed in parallel with app development

## Installation Path Conventions

All distro artifacts follow fixed paths on the target system. Paths are set by the `Dockerfile` (Docker image build) and validated by the `check-paths` Makefile target.

**Quick reference:**
- **Scripts** install to `/usr/local/bin/` (ralphglasses binary, hw-detect.sh, install-to-disk.sh)
- **App configs** install to `/etc/ralphglasses/` (config.yaml, providers.yaml)

### Scripts — `/usr/local/bin/`

| Source | Installed As | Set By |
|--------|-------------|--------|
| `go build .` (repo root) | `/usr/local/bin/ralphglasses` | `Dockerfile` line 112 |
| `distro/scripts/hw-detect.sh` | `/usr/local/bin/hw-detect.sh` | `Dockerfile` line 131 |
| `distro/scripts/install-to-disk.sh` | `/usr/local/bin/install-to-disk.sh` | `Dockerfile` line 137 |

### Systemd Units — `/etc/systemd/system/`

| Source | Installed As | Set By |
|--------|-------------|--------|
| `distro/systemd/ralphglasses.service` | `/etc/systemd/system/ralphglasses.service` | `Dockerfile` line 127 |
| `distro/systemd/hw-detect.service` | `/etc/systemd/system/hw-detect.service` | `Dockerfile` line 133 |
| (inline in Dockerfile) | `/etc/systemd/system/getty@tty1.service.d/autologin.conf` | `Dockerfile` line 141 |

### System Configuration — `/etc/`

| Path | Contents | Set By |
|------|----------|--------|
| `/etc/X11/xorg.conf.d/20-nvidia.conf` | Static dual-GPU Xorg config | `Dockerfile` line 157 |
| `/etc/modprobe.d/ralphglasses-blacklist.conf` | Blacklists `btmtk` and `nouveau` | `Dockerfile` line 199 |
| `/etc/NetworkManager/conf.d/10-proart.conf` | Wired NIC priority (atlantic > igc) | `Dockerfile` line 209 |
| `/etc/udev/rules.d/97-csr8510-bluetooth.rules` | CSR8510 BT dongle HCI mode fix | `Dockerfile` line 223 |
| `/etc/bolt/boltd.conf` | USB4/Thunderbolt auto-authorize | `Dockerfile` line 229 |

### Application Configuration — `/etc/ralphglasses/`

| Path | Contents | Set By |
|------|----------|--------|
| `/etc/ralphglasses/config.yaml` | Main application configuration (scan paths, providers, budgets) | `Dockerfile` line 245 |
| `/etc/ralphglasses/providers.yaml` | Provider credentials and endpoint overrides | `Dockerfile` line 247 |

This directory holds all ralphglasses-specific configuration. The binary reads from `/etc/ralphglasses/` when `$RALPHGLASSES_CONFIG_DIR` is unset. User overrides go in `~/.config/ralphglasses/`.

### Runtime Paths — Generated by `hw-detect.sh`

These paths are created at first boot by `distro/scripts/hw-detect.sh` (invoked via `hw-detect.service`):

| Path | Contents | Set By |
|------|----------|--------|
| `/etc/X11/xorg.conf.d/20-gpu.conf` | Xorg GPU BusID (RTX 4090 or AMD iGPU fallback) | `hw-detect.sh` line 29 |
| `/etc/modprobe.d/blacklist-gtx1060.conf` | Excludes GTX 1060 from nvidia driver | `hw-detect.sh` line 31 |
| `/etc/modprobe.d/blacklist-btmtk.conf` | Blacklists MT7927 Bluetooth modules | `hw-detect.sh` line 32 |
| `/var/log/hw-detect.log` | Detection log output | `hw-detect.sh` line 33 |
| `/var/lib/hw-detect/configured` | Sentinel file preventing re-runs | `hw-detect.service` line 6, 13 |

### User and Data Paths

| Path | Contents | Set By |
|------|----------|--------|
| `/home/ralph/.config/i3/config` | i3 window manager config (copied from `distro/i3/config`) | `Dockerfile` line 116 |
| `/workspace` | Git repos, `RALPHGLASSES_SCAN_PATH` | `Dockerfile` line 242, `ralphglasses.service` |

### Boot — `/boot/grub/`

| Source | Installed As | Set By |
|--------|-------------|--------|
| `distro/grub/grub.cfg` | `build/iso/boot/grub/grub.cfg` (inside ISO) | `Makefile` `iso` target, line 50 |

### Makefile Targets

Run from the `distro/` directory:

| Target | Description |
|--------|-------------|
| `make check-paths` | Validates all `ExecStart=` lines in `.service` files use the `/usr/local/bin/` prefix. |
| `make iso` | Builds a bootable UEFI ISO from the Docker image via `docker` → `rootfs` → squashfs → xorriso. |
| `make test-vm` | Launches the ISO in QEMU with UEFI for testing. |
| `make usb DEVICE=/dev/sdX` | Writes the ISO to a USB drive. |
| `make clean` | Removes `build/` artifacts and the Docker image. |

### Path Consistency Rule

All PRs that add, rename, or remove installed paths must update:
1. This README (the tables above).
2. The `check-paths` Makefile target (to validate script paths).
3. The `Dockerfile` (where paths are actually enforced).

Failure to keep these in sync will be caught in review.

## What Does NOT Belong in This Repo

- Windows driver archives (Google Drive)
- NVIDIA `.run` installer files (GitHub Release artifacts)
- DKMS source tarballs
- Firmware blobs
- The 12GB ProArt driver archive
