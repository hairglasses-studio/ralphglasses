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

All distro artifacts follow fixed paths on the target system. PRs that touch install paths must update both this section and the `check-paths` Makefile target together.

### Scripts — `/usr/local/bin/`

All executable scripts and binaries deploy to `/usr/local/bin/`:

| Source | Installed As | Description |
|--------|-------------|-------------|
| `go build .` (repo root) | `/usr/local/bin/ralphglasses` | TUI agent fleet manager binary |
| `distro/scripts/hw-detect.sh` | `/usr/local/bin/hw-detect.sh` | First-boot hardware detection (GPU enumeration, module blacklists) |
| `distro/scripts/install-to-disk.sh` | `/usr/local/bin/install-to-disk.sh` | Writes the ISO to a target disk for bare-metal install |

### Configuration — `/etc/ralphglasses/`

Static configuration files deploy to `/etc/ralphglasses/`:

| Source | Installed As | Description |
|--------|-------------|-------------|
| `distro/i3/config` | `/etc/ralphglasses/i3.conf` | i3 window manager config for the thin client session |
| `distro/grub/grub.cfg` | `/etc/ralphglasses/grub.cfg` | Reference copy of the GRUB menu config (live copy is in `/boot/grub/`) |

### Systemd Units — `/etc/systemd/system/`

Service files deploy to `/etc/systemd/system/`. Enable and start with:

```bash
sudo systemctl enable --now hw-detect.service
sudo systemctl enable --now ralphglasses.service
```

| Source | Installed As | Description |
|--------|-------------|-------------|
| `distro/systemd/hw-detect.service` | `/etc/systemd/system/hw-detect.service` | Oneshot: first-boot GPU and module detection |
| `distro/systemd/ralphglasses.service` | `/etc/systemd/system/ralphglasses.service` | TUI autostart after graphical target is reached |

See `distro/docs/service-paths.md` for boot ordering and unit dependencies.

### Makefile Targets

Run from the `distro/` directory:

| Target | Description |
|--------|-------------|
| `make install` | Copies scripts to `/usr/local/bin/`, configs to `/etc/ralphglasses/`, and units to `/etc/systemd/system/`. Runs `systemctl daemon-reload`. |
| `make uninstall` | Removes installed scripts, configs, and units. Runs `systemctl daemon-reload`. |
| `make check-paths` | Validates all `ExecStart=` lines in `.service` files use the `/usr/local/bin/` prefix. |
| `make iso` | Builds a bootable UEFI ISO from the Docker image. |
| `make test-vm` | Launches the ISO in QEMU for testing. |
| `make usb DEVICE=/dev/sdX` | Writes the ISO to a USB drive. |
| `make clean` | Removes build artifacts and the Docker image. |

### Path Consistency Rule

All PRs that add, rename, or remove installed paths must update:
1. This README (the tables above).
2. The `check-paths` Makefile target (to validate the new paths).
3. The `install` and `uninstall` Makefile targets.

Failure to keep these in sync will be caught in review.

## What Does NOT Belong in This Repo

- Windows driver archives (Google Drive)
- NVIDIA `.run` installer files (GitHub Release artifacts)
- DKMS source tarballs
- Firmware blobs
- The 12GB ProArt driver archive
