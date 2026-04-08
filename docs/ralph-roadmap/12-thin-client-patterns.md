# Thin Client Patterns for AI Workstation

**Date:** 2026-04-04
**Scope:** Bootable thin client architecture for ralphglasses agent fleet
**Analyst:** Research pass (Claude Opus 4.6)
**Baseline:** `ralphglasses/distro/` at Phase 4 (~21% completion)

---

## Table of Contents

1. [Distro Base Recommendation](#1-distro-base-recommendation)
2. [Compositor Strategy](#2-compositor-strategy)
3. [Multi-GPU Setup](#3-multi-gpu-setup)
4. [Boot-to-TUI Pipeline](#4-boot-to-tui-pipeline)
5. [Secrets Management](#5-secrets-management)
6. [OTA Updates](#6-ota-updates)
7. [Fleet Deployment](#7-fleet-deployment)
8. [Thin Client Deployment Scope](#8-thin-client-deployment-scope)
9. [Fastest Path to Bootable Prototype](#9-fastest-path-to-bootable-prototype)
10. [Recommendations](#10-recommendations)

---

## 1. Distro Base Recommendation

The current implementation uses Manjaro Linux (`Dockerfile.manjaro`) with pacman for package management. This section evaluates whether to stay or switch.

### Candidates

| Distro | Base | Package Mgr | Immutable | Rollback | NVIDIA Support | Kiosk Ecosystem |
|--------|------|-------------|-----------|----------|----------------|-----------------|
| **Manjaro** | Arch | pacman | No | timeshift/snapper | Good (AUR, mhwd) | Manual |
| **NixOS** | Independent | nix | Yes (generations) | Native (built-in) | Good (nixpkgs) | nixiosk, cage module |
| **Fedora Kinoite** | Fedora | rpm-ostree | Yes (OSTree) | Native (ostree rollback) | Acceptable (RPM Fusion) | Flatpak-centric |
| **Arch Linux** | Independent | pacman | No | Manual | Good (official repos) | Manual |
| **Custom Buildroot** | Cross-compile | N/A | Yes (image-based) | A/B partition | Manual | Full control |

### Analysis

**Manjaro (current)**
- Pros: Familiar, rolling release with tested snapshots, good NVIDIA support via `mhwd` hardware detection, large AUR for edge packages. The current `Dockerfile.manjaro` already works.
- Cons: Not immutable. No atomic rollback. Rolling release means silent breakage risk on unattended thin clients. No built-in reproducibility guarantee -- two ISOs built a week apart may differ.

**NixOS (recommended for new builds)**
- Pros: Entire system state declared in `configuration.nix`. Atomic generations with instant rollback. The nixiosk project provides a ready-made kiosk framework using Cage compositor. Flakes pin every dependency for reproducible builds. NixOps 4 reports 90% reduction in configuration drift in enterprise deployments. A single `.nix` file can define the complete thin client: kernel, NVIDIA drivers, Sway config, ralphglasses binary, systemd units, and secrets.
- Cons: Steep learning curve (Nix language). Larger disk footprint due to `/nix/store`. NVIDIA driver packaging requires `nixpkgs.config.allowUnfree = true` and careful pinning. Not Arch-based, so AUR packages need manual porting. The team must learn a new paradigm.
- Sources: [NixOS.org](https://nixos.org/), [NixOS-Powered AI Infrastructure](https://medium.com/@mehtacharu0215/nixos-powered-ai-infrastructure-reproducible-immutable-deployable-anywhere-d3e225fc9b5a), [NixOS as Most Powerful Distro 2026](https://allthingsopen.org/articles/nixos-most-powerful-linux-distro-2026), [Nixiosk GitHub](https://github.com/matthewbauer/nixiosk), [NixOS Kiosk with Cage](https://github.com/matthewbauer/nixos-kiosk)

**Fedora Kinoite**
- Pros: OSTree-based atomic updates with rollback. Strong Secure Boot support. Red Hat ecosystem stability. Toolbox/Distrobox for development containers.
- Cons: RPM Fusion NVIDIA packaging lags behind upstream. rpm-ostree layering is slower than pacman. KDE-focused (Sway needs manual overlay). Six-month release cadence may lag behind kernel features needed for new NVIDIA drivers.
- Sources: [Fedora Kinoite](https://fedoraproject.org/atomic-desktops/kinoite/), [Fedora Atomic Desktops](https://fedoramagazine.org/introducing-fedora-atomic-desktops/), [Fedora Silverblue Review](https://www.xda-developers.com/replaced-my-linux-desktop-fedora-silverblue-feels-futuristic/)

**Arch Linux (raw)**
- Pros: Identical package base to Manjaro without the Manjaro overlay. `archinstall` now supports Sway, Hyprland, labwc, and niri as first-class profiles. Maximum control. Excellent documentation.
- Cons: Same mutability problems as Manjaro. No guardrails against partial upgrades. Requires custom tooling for reproducible image builds.
- Sources: [Arch Linux Installer Supports Labwc, Niri, River](https://9to5linux.com/arch-linux-installer-now-supports-labwc-niri-and-river-wayland-compositors), [Arch vs NixOS 2026](https://www.slant.co/versus/2690/2700/~arch-linux_vs_nixos)

**Custom Buildroot**
- Pros: Minimal footprint. Full control over every binary. Ideal for embedded appliances.
- Cons: Massive engineering investment. No package manager for runtime updates. Every component must be cross-compiled and tested. Not practical for a team of one.

### Verdict

**Short term (Phase 4 completion):** Stay with Manjaro. The existing `Dockerfile.manjaro` and `build-iso.sh` pipeline work. Switching distros now would reset Phase 4 to 0%.

**Medium term (Phase 5, fleet deployment):** Migrate to NixOS. The declarative model eliminates the "works on my machine" problem that will become acute when deploying 10+ thin clients. A single `flake.nix` can define every thin client variant. The nixiosk + Cage pattern directly matches the ralphglasses kiosk use case.

**Transition path:** Write a `flake.nix` that produces the same system as the current `Dockerfile.manjaro`. Validate it boots in QEMU. Then cut over.

---

## 2. Compositor Strategy

The current codebase supports three compositors via an abstraction layer (`compositor-detect.sh`, `compositor-cmd.sh`, `internal/wm/`): Sway, Hyprland, and i3.

### Maintenance Cost of Three Compositors

The existing audit (see `08-distro-audit.md`) documents 9 abstracted commands across all three compositors. The cost surfaces as:

- **Three config sets**: `distro/sway/`, `distro/hyprland/`, `distro/i3/` each require separate kiosk configs, waybar/i3blocks configs, and environment.d files.
- **Three kiosk setup scripts**: `sway-kiosk-setup.sh`, `compositor-kiosk-setup.sh` (Sway + Hyprland), and `kiosk-setup.sh` (i3 only).
- **Testing matrix**: Every distro change must be validated on 3 compositors x 2 GPU configs.
- **DPMS gap**: i3 cannot do DPMS via IPC; callers must special-case it.
- **Hyprland race**: The `exec-on` command in Hyprland chains two non-atomic `hyprctl dispatch` calls.

### Compositor Comparison for This Use Case

| Feature | Sway | Hyprland | i3 |
|---------|------|----------|-----|
| Protocol | Wayland (wlroots) | Wayland (aquamarine) | X11 |
| Multi-monitor | Native, stable | Native, active HDR/fractional work | Via xrandr |
| NVIDIA | Improving (GBM, explicit sync) | Trickier, NVIDIA-specific flags needed | Mature (proprietary driver) |
| Kiosk mode | Via config (`fullscreen enable`) | Via `windowrulev2` | Via config |
| Hotplug | kanshi integration | Built-in, dynamic | autorandr (X11) |
| CPU/Memory | Low | 15-25% lower than Sway under load | Low |
| Stability | High, conservative releases | Fast-moving, occasional regressions | Very high |
| IPC | i3-compatible JSON | hyprctl JSON | i3 JSON |

Sources: [Hyprland vs Sway 2025](https://gigasblade.blogspot.com/2025/10/hyprland-vs-swaywm-2025-dazzling.html), [Hyprland vs Sway Landscape 2025](https://www.oreateai.com/blog/hyprland-vs-sway-navigating-the-wayland-tiling-window-manager-landscape-in-2025/6730d963b567b07eb8f667c5e92f8b8a), [Tiling WMs for Productivity](https://dasroot.net/posts/2026/01/tiling-window-managers-i3-sway-hyprland-productivity/), [Sway ArchWiki](https://wiki.archlinux.org/title/Sway)

### Other Compositors Worth Noting

- **Cage**: Purpose-built Wayland kiosk compositor. Runs a single maximized application. Zero configuration. Built on wlroots 0.18. Ideal for the ralphglasses use case where only one TUI app runs full-screen. Multi-monitor support is limited to mirroring by design.
  Source: [Cage GitHub](https://github.com/cage-kiosk/cage), [Cage 0.2 Release](https://www.phoronix.com/news/Cage-0.2-Released)

- **niri**: Scrollable-tiling Wayland compositor written in Rust. Reached v25.05 with overview features. Interesting UX model but no kiosk mode and unclear NVIDIA multi-GPU support.
  Source: [niri GitHub](https://github.com/niri-wm/niri), [niri v25.01](https://www.phoronix.com/news/Niri-25.01-Tiling-Wayland-Comp)

- **labwc**: Openbox-like Wayland compositor. Now a first-class `archinstall` profile. Lightweight but lacks tiling, which matters less for a single-app kiosk.
  Source: [Arch Installer labwc Support](https://9to5linux.com/arch-linux-installer-now-supports-labwc-niri-and-river-wayland-compositors)

### Verdict

**Drop i3 support.** X11 is end-of-life for new deployments. The i3 path exists as a "legacy fallback" but adds testing burden with no users. The `distro/i3/`, `distro/xorg/`, and `kiosk-setup.sh` can be archived.

**Primary: Sway.** It is the most stable wlroots compositor for NVIDIA multi-monitor. The existing Sway config, waybar, and kiosk setup are the most complete. kanshi handles hotplug. The i3-compatible IPC means existing tooling works.

**Keep Hyprland as opt-in alternative.** The abstraction layer already supports it. The Hyprland kiosk.conf and waybar configs exist. But do not invest in parity -- let it be a "best effort" second compositor.

**Evaluate Cage for fleet workers.** Worker thin clients that only run ralphglasses (no waybar, no multi-workspace) could use Cage instead of Sway, reducing the attack surface and configuration to zero. Cage + ralphglasses TUI is the minimal kiosk.

---

## 3. Multi-GPU Setup

The current hardware target is: 2x NVIDIA RTX 4090 (Ada Lovelace) + AMD Ryzen 7950X iGPU (RDNA2), driving 7 monitors on an ASUS ProArt X870E-CREATOR WIFI.

### Current State

`hw-detect.sh` currently handles a single RTX 4090 + AMD iGPU. It does not handle dual RTX 4090 cards. The PCI device ID `2684` is hardcoded for a single card.

### Dual NVIDIA 4090 Under Sway

As of early 2025, dual RTX 4090 cards can drive 5+ monitors under KDE Wayland with `nvidia-open-dkms-570.86.16-2`. However, Sway/wlroots has a fundamental limitation:

**Sway renders on a single GPU.** The `WLR_DRM_DEVICES` environment variable specifies which DRM devices to use, but wlroots performs all rendering on the first device. Outputs connected to the second card use PRIME copy (DMA-BUF import), which has historically been broken between two NVIDIA cards.

Sources: [wlroots Multi-GPU Issue #934](https://github.com/swaywm/wlroots/issues/934), [Arch Forums: Multiple NVIDIA GPUs with 5 Screens in Wayland](https://bbs.archlinux.org/viewtopic.php?id=303372), [NVIDIA open-gpu-kernel-modules Issue #318](https://github.com/NVIDIA/open-gpu-kernel-modules/issues/318)

### Required Configuration

```bash
# Kernel parameters (GRUB_CMDLINE_LINUX)
nvidia-drm.modeset=1 nvidia-drm.fbdev=1

# /etc/modprobe.d/nvidia-drm.conf
options nvidia-drm modeset=1

# Environment for Sway (environment.d/nvidia-wayland.conf)
WLR_DRM_DEVICES=/dev/dri/card0:/dev/dri/card1
GBM_BACKEND=nvidia-drm
__GLX_VENDOR_LIBRARY_NAME=nvidia
WLR_NO_HARDWARE_CURSORS=1
LIBVA_DRIVER_NAME=nvidia
```

### Practical Multi-Monitor Assignment

With 2x RTX 4090, each card provides 4 display outputs (3x DP 1.4a + 1x HDMI 2.1). Assign 4 monitors to card0, 3 monitors to card1 (plus the AMD iGPU as emergency fallback).

The `hw-detect.sh` script needs these changes:
1. Detect multiple RTX 4090 cards (iterate `lspci` results, not just take the first match)
2. Enumerate `/dev/dri/card*` nodes and map them to PCI bus IDs
3. Generate `WLR_DRM_DEVICES` with both cards in the correct order
4. Write per-card monitor assignments to `monitors.conf`

### Workaround: Single-Card Mode

If PRIME copy between two NVIDIA cards remains broken in wlroots, a viable workaround is to run Sway on only one RTX 4090 (4 monitors) and use the second card exclusively for CUDA/compute workloads (LLM inference, etc.). Set:

```bash
WLR_DRM_DEVICES=/dev/dri/card0  # Display card only
CUDA_VISIBLE_DEVICES=1          # Compute card only
```

This avoids the DMA-BUF interop problem entirely and gives 4 monitors for the TUI + 1 dedicated GPU for local inference.

Sources: [RTX 4090 Cursor Lag Solution](https://github.com/basecamp/omarchy/discussions/4702), [Wayland Multi-Monitor 2026](https://copyprogramming.com/howto/multi-monitor-issues-with-xorg-nvidia-wayland), [Hyprland Wiki WLR_DRM_DEVICES](https://github.com/hyprwm/hyprland-wiki/issues/694)

---

## 4. Boot-to-TUI Pipeline

The goal: power on -> BIOS/UEFI -> bootloader -> kernel -> directly into ralphglasses TUI. No desktop environment, no login prompt for the default kiosk user.

### Current Pipeline

The existing `Dockerfile.manjaro` sets up:
```
BIOS -> GRUB -> kernel -> systemd -> getty@tty1 (autologin ralph)
  -> .bash_profile (exec sway) -> Sway -> ralphglasses.service
```

This chain has 6 steps and 3 potential failure points (getty, bash_profile exec, Sway startup).

### Recommended Pipeline Options

**Option A: greetd + tuigreet (recommended for Sway path)**

```
BIOS -> GRUB -> kernel -> systemd -> greetd -> tuigreet --cmd sway
  -> Sway -> ralphglasses (via sway exec)
```

greetd is a minimal login manager daemon with PAM integration. tuigreet provides a TUI login screen that can auto-login and launch Sway directly. This is cleaner than the getty + bash_profile hack because greetd handles session management, PAM, and environment setup in one place.

Configuration (`/etc/greetd/config.toml`):
```toml
[terminal]
vt = 1

[default_session]
command = "tuigreet --cmd sway --remember --user ralph"
user = "greeter"
```

Sources: [greetd ArchWiki](https://wiki.archlinux.org/title/Greetd), [tuigreet GitHub](https://github.com/apognu/tuigreet), [greetd SourceHut](https://sr.ht/~kennylevinsen/greetd/)

**Option B: Cage + ralphglasses (recommended for worker thin clients)**

```
BIOS -> GRUB -> kernel -> systemd -> cage ralphglasses
```

Cage is a Wayland kiosk compositor that runs a single maximized application. systemd can launch Cage directly via a service unit, bypassing any login manager entirely:

```ini
[Service]
Type=simple
User=ralph
ExecStart=/usr/bin/cage -- /usr/local/bin/ralphglasses --scan-path /workspace
Environment=WLR_DRM_DEVICES=/dev/dri/card0
```

This is the shortest possible chain: 5 steps, single binary compositor, no login prompt, no shell. Cage handles Wayland initialization and runs ralphglasses full-screen.

Limitation: Cage mirrors all outputs (all monitors show the same thing). For a 7-monitor dashboard with different content per monitor, Sway is required. But for worker nodes running headless or single-monitor, Cage is ideal.

Sources: [Cage Website](https://www.hjdskes.nl/projects/cage/), [NixOS Kiosk with Cage](https://github.com/matthewbauer/nixos-kiosk)

**Option C: systemd autologin + Sway (current approach, simplified)**

Keep the current getty autologin but replace the `.bash_profile` exec with a proper systemd user service:

```ini
# /etc/systemd/system/sway-session.service
[Unit]
Description=Sway Wayland Compositor
After=systemd-user-sessions.service

[Service]
Type=simple
User=ralph
PAMName=login
TTYPath=/dev/tty1
Environment=XDG_SESSION_TYPE=wayland
ExecStart=/usr/bin/sway
Restart=on-failure

[Install]
WantedBy=graphical.target
```

This eliminates the fragile `.bash_profile` and `getty@tty1` override hack.

### Verdict

Use **Option A (greetd + tuigreet)** for the primary workstation (7 monitors, Sway). Use **Option B (Cage)** for fleet worker thin clients. Both are cleaner than the current getty + bash_profile chain.

---

## 5. Secrets Management

The thin client needs to store LLM API keys (Anthropic, Google, OpenAI), GitHub tokens, and potentially 1Password service account tokens. Current approach: none implemented. The `CLAUDE.md` specifies environment variables but the distro has no secrets provisioning.

### Options

| Approach | Encryption at Rest | TPM Binding | Offline | Complexity |
|----------|-------------------|-------------|---------|------------|
| **systemd-creds** | Yes (host key + TPM2) | Yes | Yes | Low |
| **1Password CLI (op)** | Yes (vault) | No | No (needs network) | Medium |
| **HashiCorp Vault Agent** | Yes (transit) | Optional | No | High |
| **LUKS + TPM auto-unlock** | Yes (full disk) | Yes | Yes | Medium |
| **Environment files** | No | No | Yes | Trivial (insecure) |

### systemd-creds (recommended)

systemd-creds encrypts credentials using a combination of a host-specific key and TPM2 PCR values. Secrets are decrypted only at service runtime and automatically cleaned up when the service stops.

```bash
# Encrypt a credential bound to this machine's TPM
systemd-creds encrypt --with-key=tpm2 \
  --name=anthropic-api-key \
  /tmp/plaintext-key \
  /etc/credstore.encrypted/anthropic-api-key

# Reference in the service unit
[Service]
LoadCredentialEncrypted=anthropic-api-key:/etc/credstore.encrypted/anthropic-api-key
ExecStart=/usr/local/bin/ralphglasses --api-key-file ${CREDENTIALS_DIRECTORY}/anthropic-api-key
```

This means the API keys are bound to the specific TPM chip in the machine. If the SSD is removed and placed in another machine, the credentials cannot be decrypted. No network connectivity required.

Sources: [systemd-creds ArchWiki](https://wiki.archlinux.org/title/Systemd-creds), [The Magic of systemd-creds](https://smallstep.com/blog/systemd-creds-hardware-protected-secrets/), [systemd.io Credentials](https://systemd.io/CREDENTIALS/), [systemd-creds Secret Injection RHEL](https://oneuptime.com/blog/post/2026-03-04-systemd-credentials-secret-injection-rhel-9/view)

### 1Password CLI (complementary)

For initial provisioning and rotation, use 1Password service accounts. A headless thin client can use `OP_SERVICE_ACCOUNT_TOKEN` to read secrets from a vault without the desktop app.

```bash
# First-boot provisioning script
export OP_SERVICE_ACCOUNT_TOKEN="..."  # Set during PXE/cloud-init
op read "op://RalphSecrets/Anthropic/api-key" > /tmp/anthropic-key
systemd-creds encrypt --with-key=tpm2 --name=anthropic-api-key /tmp/anthropic-key /etc/credstore.encrypted/anthropic-api-key
shred -u /tmp/anthropic-key
```

This combines 1Password for centralized secret management with systemd-creds for TPM-bound local storage. The plaintext key exists only transiently during first-boot provisioning.

Sources: [1Password CLI Get Started](https://developer.1password.com/docs/cli/get-started/), [1Password Secrets in Scripts](https://developer.1password.com/docs/cli/secrets-scripts/), [1Password systemd Integration](https://1password.community/discussion/128572/how-to-inject-a-secret-into-the-environment-via-a-systemd-service-definition)

### Secure Boot Chain

For the secrets to be trustworthy, the boot chain must be verified:

1. **UEFI Secure Boot**: Verifies the GRUB bootloader is signed. The existing `secureboot/mok-enroll.sh` handles MOK enrollment.
2. **NVIDIA Module Signing**: DKMS must sign NVIDIA kernel modules with the MOK key. Configure `/etc/dkms/nvidia.conf` with the MOK signing key paths for automatic signing during kernel updates.
3. **Measured Boot (TPM PCRs)**: The TPM records each boot stage in PCR registers. systemd-creds can bind credentials to specific PCR values, so secrets are only released if the boot chain is unmodified.
4. **LUKS with TPM**: Full disk encryption unlocked automatically by the TPM, ensuring data at rest is protected even if the physical disk is removed.

Sources: [UEFI Secure Boot ArchWiki](https://wiki.archlinux.org/title/Unified_Extensible_Firmware_Interface/Secure_Boot), [TPM 2.0 and Secure Boot on Servers 2026](https://servermall.com/blog/tpm-2-0-and-secure-boot-on-the-server/), [NSA UEFI Secure Boot Guidance](https://media.defense.gov/2025/Dec/11/2003841096/-1/-1/0/CSI_UEFI_SECURE_BOOT.PDF), [NVIDIA DKMS Module Signing](https://gist.github.com/lijikun/22be09ec9b178e745758a29c7a147cc9), [NVIDIA Driver r595 Installation Guide](https://docs.nvidia.com/datacenter/tesla/pdf/Driver_Installation_Guide.pdf), [TPM2 LUKS systemd-cryptenroll](https://gierdo.astounding.technology/blog/2025/07/05/tpm2-luks-systemd)

### Verdict

Use **systemd-creds + TPM2** for local secret storage. Use **1Password service accounts** for centralized provisioning and rotation. Implement **LUKS + TPM auto-unlock** for full disk encryption. This provides defense in depth without requiring network connectivity at runtime.

---

## 6. OTA Updates

The existing `ota.sh` implements a basic update mechanism: check a server for a new version, download a tarball, verify SHA256, back up the current binary/kernel/overlay, install the new one, and optionally reboot. It supports rollback by restoring from the backup directory.

### Current Gaps

- No A/B partitioning (rollback restores files, not partition state)
- No signature verification (only SHA256 checksum)
- No delta updates (downloads full artifact every time)
- No update server implementation (OTA_ENDPOINT is undefined)
- No integration with the distro package manager

### OTA Pattern Comparison

| Pattern | Atomicity | Storage Overhead | Delta Support | Rollback | Complexity |
|---------|-----------|-----------------|---------------|----------|------------|
| **OSTree** | Atomic | Low (hardlinks) | Yes (static deltas) | Instant (generation switch) | Medium |
| **A/B Partition (RAUC)** | Atomic | 2x root | Yes (casync, adaptive) | Instant (partition swap) | Medium |
| **SWUpdate** | Flexible | Configurable | Yes (handler-based) | Via A/B or recovery | High |
| **Current ota.sh** | File-level | Backup copies | No | Manual file restore | Low |
| **Package manager** | Per-package | Low | Yes (pacman delta) | Manual downgrade | Low |

Sources: [Rugix OTA Engines Compared](https://rugix.org/blog/2026-02-28-ota-update-engines-compared/), [RAUC.io](https://rauc.io/), [SWUpdate vs Mender vs RAUC](https://32blog.com/en/yocto/yocto-ota-update-comparison), [FOSDEM 2025 A/B Update Solutions](https://archive.fosdem.org/2025/events/attachments/fosdem-2025-6299-exploring-open-source-dual-a-b-update-solutions-for-embedded-linux/slides/237879/leon-anav_pyytRpX.pdf), [Torizon OTA Guide](https://www.torizon.io/blog/ota-best-linux-os-image-update-model)

### Recommended Approach

**If staying on Manjaro:** Use RAUC with A/B partitioning. RAUC is lightweight, supports HTTP streaming and adaptive (delta) updates, and works with any Linux distro. The thin client SSD is partitioned into two root partitions; updates write to the inactive partition and a bootloader flag swap activates it on reboot. If the new partition fails to boot, the watchdog reboots into the previous partition.

**If migrating to NixOS:** OSTree becomes unnecessary because NixOS generations provide the same semantics natively. `nixos-rebuild switch --flake .#thin-client` creates a new generation. `nixos-rebuild switch --rollback` reverts. The update pipeline becomes: push new `flake.lock` to git -> thin client pulls and rebuilds. No custom OTA server needed.

### Partition Layout for RAUC (Manjaro)

```
/dev/sda1  512MB  EFI System Partition (ESP)
/dev/sda2  30GB   Root A (active)
/dev/sda3  30GB   Root B (standby)
/dev/sda4  *      Data (persistent: /workspace, /etc/credstore.encrypted, logs)
```

### Verdict

For Phase 4 (Manjaro), enhance `ota.sh` with GPG signature verification and implement RAUC A/B partitioning. For Phase 5+ (NixOS), rely on native Nix generations and drop custom OTA tooling entirely.

---

## 7. Fleet Deployment

The existing `distro/pxe/` directory contains an iPXE boot script and an LTSP setup script. The iPXE config supports NFS root, squashfs-over-HTTP, AMD iGPU, and headless serial modes. The `distro/cloud-init/worker.yaml` suggests cloud-init provisioning is partially planned.

### PXE Boot Architecture

The current iPXE script chain is sound:

```
DHCP -> iPXE chainload -> HTTP kernel+initrd -> NFS/squashfs root -> systemd -> ralphglasses
```

### Fleet Provisioning Tools

| Tool | Model | GPU Support | Scale | Maturity |
|------|-------|-------------|-------|----------|
| **Pixiecore** | Go binary, API-driven PXE | Agnostic | 10-100 | Moderate |
| **MAAS** | Full lifecycle (IPMI/Redfish/BMC) | Agnostic | 100-10000 | High |
| **Tinkerbell** | CNCF, workflow engine | Agnostic | 10-1000 | Growing |
| **Custom iPXE + HTTP** | Current approach | Agnostic | 1-50 | DIY |

Sources: [Pixiecore README](https://github.com/danderson/netboot/blob/main/pixiecore/README.booting.md), [MAAS Ubuntu](https://canonical.com/maas), [Tinkerbell.org](https://tinkerbell.org/), [Tinkerbell GitHub](https://github.com/tinkerbell/tinkerbell), [DigitalOcean PXE Guide](https://www.digitalocean.com/community/tutorials/bare-metal-provisioning-with-pxe-and-ipxe), [Network Booting Overview](https://rootknight.com/2025/10/25/network-booting-pxe-boot-ipxe-boot-netboot-xyz/)

### Recommended Pipeline for 10+ Worker Thin Clients

**Phase 1: Pixiecore (immediate, 1-10 nodes)**

Pixiecore is a single Go binary that cooperates with your existing DHCP server. It serves iPXE boot scripts and can be driven by an API for per-machine customization. This fits the ralphglasses ecosystem perfectly -- a Go binary that could eventually be integrated into the ralphglasses MCP server.

```bash
# Serve the ralphglasses boot image to any PXE-booting machine
pixiecore boot vmlinuz initrd.img \
  --cmdline "boot=live fetch=http://10.0.0.1:8080/filesystem.squashfs ip=dhcp nvidia-drm.modeset=1"
```

**Phase 2: Tinkerbell (10-50 nodes, hardware lifecycle)**

Tinkerbell adds BMC/IPMI integration, workflow templating, and a metadata service. It can power-cycle machines, flash firmware, and run multi-step provisioning workflows. The CNCF backing gives it longevity.

**Phase 3: MAAS (50+ nodes, if needed)**

Only necessary if the fleet grows beyond what Tinkerbell handles comfortably, or if Canonical/Ubuntu integration is desired.

### Provisioning Workflow

```
1. New thin client powers on, PXE boots
2. Pixiecore/Tinkerbell serves iPXE -> kernel -> squashfs
3. First-boot script runs:
   a. hw-detect.sh --wayland-only (GPU detection)
   b. systemd-creds encrypt (seal API keys from 1Password via OP_SERVICE_ACCOUNT_TOKEN)
   c. kanshi profile generation (monitor layout)
   d. Tailscale enrollment (ts-enroll.sh already exists)
   e. Report readiness to fleet coordinator
4. Thin client reboots into local install (or continues diskless)
5. ralphglasses TUI starts, joins fleet
```

---

## 8. Thin Client Deployment Scope

### Feasibility Assessment

Ralphglasses now targets x86_64 thin clients in production, with Apple Silicon
developer machines supported separately via `darwin/arm64` builds.

**Primary deployment path:**
```bash
GOOS=linux GOARCH=amd64 go build -o ralphglasses-amd64 .
```

The ralphglasses binary is pure Go with Charmbracelet TUI (no CGO dependencies that would complicate cross-compilation).

**Supported hardware focus (2025-2026):**

| Board | CPU | RAM | Price | Notes |
|-------|-----|-----|-------|-------|
| ASUS ProArt workstation | Ryzen 9 + x86_64 Linux | 128GB | Existing target | Primary control-plane host |
| Intel/AMD mini PC | x86_64 | 16-64GB | Variable | Thin client / kiosk deployment |
| Apple Silicon Mac | M-series | 16-64GB | Variable | Developer workstation, not Linux thin client |

### Supported Deployment Targets

- **x86_64 thin clients**: Primary Linux deployment target for kiosks, PXE workers, and multi-monitor control planes.
- **x86_64 coordinators**: Valid for PXE/Pixiecore, observability, relay, and orchestration services.
- **Apple Silicon developer workstations**: Supported for local development and testing, but not as Linux thin clients.

### Unsupported Linux ARM Targets

- **No release artifacts or CI**: Linux ARM builds are intentionally out of scope.
- **No thin-client packaging**: The supported kiosk image and install path target x86_64 Linux only.
- **No multi-monitor workstation target**: Linux ARM boards do not match the intended fleet-control hardware profile.

### Verdict

Linux deployment stays x86_64. Apple Silicon remains supported for development, but Linux ARM boards are no longer part of the thin-client or fleet-runtime plan.

---

## 9. Fastest Path to Bootable Prototype

Given 21% Phase 4 completion, here is the minimum viable path to a bootable ISO that launches ralphglasses.

### What Already Works

- `Dockerfile.manjaro`: Full rootfs definition with Sway, NVIDIA drivers, ralphglasses binary, systemd service, autologin
- `build-iso.sh`: Takes a rootfs and produces a hybrid BIOS+UEFI ISO via grub-mkrescue
- `grub/grub.cfg`: GRUB menu with NVIDIA kernel parameters
- `hw-detect.sh`: GPU detection for single RTX 4090 + AMD iGPU
- `ralphglasses.service`: systemd unit that launches ralphglasses after graphical-session.target
- `sway/config` + `sway/kiosk-config`: Sway kiosk configuration
- `scripts/qemu-smoke.sh`: QEMU boot test script

### What Is Missing for MVP

1. **No actual ISO has been built and tested.** The Dockerfile and build-iso.sh exist but have not been validated end-to-end. The Docker-to-rootfs-to-ISO pipeline has not been run.

2. **Two competing build paths.** The `Dockerfile.manjaro` (Docker export -> squashfs) and `Dockerfile` (Ubuntu/debootstrap path) create confusion. Pick one.

3. **No squashfs image hosted for PXE.** The iPXE config references `${boot-url}/filesystem.squashfs` but no squashfs artifact is built or served.

4. **Sway config references unresolved monitor names.** The `hw-detect.sh --wayland-only` generates commented-out monitor configs. No active monitor layout is applied.

5. **No secrets provisioning.** API keys have no delivery mechanism.

### Concrete MVP Steps (estimated 2-3 focused sessions)

```
Step 1: Build the rootfs (1 session)
  - docker build -f distro/Dockerfile.manjaro -t ralphglasses-os .
  - docker create ralphglasses-os
  - docker export <container> | tar -C build/rootfs -xf -
  - Verify: chroot build/rootfs /bin/bash, check ralphglasses --version

Step 2: Build the ISO (same session)
  - ./distro/scripts/build-iso.sh build/rootfs
  - Verify: ls -lh ralphglasses-*.iso

Step 3: QEMU smoke test (same session)
  - qemu-system-x86_64 -enable-kvm -m 4096 \
      -bios /usr/share/OVMF/OVMF_CODE.fd \
      -cdrom ralphglasses-*.iso -boot d \
      -device virtio-gpu
  - Verify: system boots, autologin works, Sway starts, ralphglasses TUI appears

Step 4: Real hardware test (1 session)
  - sudo dd if=ralphglasses-*.iso of=/dev/sdX bs=4M status=progress
  - Boot ProArt X870E from USB
  - Verify: hw-detect.sh runs, NVIDIA driver loads, monitors detected

Step 5: Secrets and connectivity (1 session)
  - Implement systemd-creds provisioning for API keys
  - Test: ralphglasses session launch with encrypted credentials
  - Configure Tailscale enrollment (ts-enroll.sh)
```

### Risk: Docker-to-ISO Approach

The `Dockerfile.manjaro` approach builds the rootfs in Docker, then exports it as a tarball for squashfs. This works but has a known issue: systemd services enabled via `RUN systemctl enable ...` inside Docker do not always persist correctly because Docker does not run a real init system. The `2>/dev/null || true` guards in the Dockerfile acknowledge this.

Mitigation: After Docker export, run `chroot-setup.sh` (which already exists) to re-enable services in the extracted rootfs before building the ISO.

---

## 10. Recommendations

Prioritized list of thin client actions, ordered by impact and effort.

### P0 -- Do Now (Phase 4 completion)

| # | Action | Effort | Impact |
|---|--------|--------|--------|
| 1 | **Build and test the ISO end-to-end** | 1 session | Validates the entire pipeline; unblocks everything else |
| 2 | **Delete the Ubuntu Dockerfile** | 10 min | Eliminates the competing build path confusion |
| 3 | **Drop i3 support** | 1 hour | Archive `distro/i3/`, `distro/xorg/`, `kiosk-setup.sh`; remove from compositor-detect.sh |
| 4 | **Fix hw-detect.sh for dual RTX 4090** | 2 hours | Support iterating multiple NVIDIA cards, generate correct WLR_DRM_DEVICES |
| 5 | **Replace getty+bash_profile with greetd** | 2 hours | Cleaner boot chain, proper PAM/session handling |

### P1 -- Next Sprint (Secrets and Stability)

| # | Action | Effort | Impact |
|---|--------|--------|--------|
| 6 | **Implement systemd-creds + TPM for API keys** | 1 day | Solves the secrets gap; required for any real deployment |
| 7 | **Add LUKS + TPM auto-unlock** | 1 day | Full disk encryption for kiosk devices |
| 8 | **Integrate kanshi for monitor hotplug** | 2 hours | Write profiles for 7-monitor layout; handle dock/undock |
| 9 | **Add GPG signing to ota.sh** | 2 hours | Verify update authenticity, not just integrity |
| 10 | **NVIDIA MOK signing via DKMS** | 2 hours | Required for Secure Boot with proprietary NVIDIA modules |

### P2 -- Phase 5 (Fleet and Scale)

| # | Action | Effort | Impact |
|---|--------|--------|--------|
| 11 | **Deploy Pixiecore for PXE fleet boot** | 1 day | Network-boot worker thin clients from a Go binary |
| 12 | **Implement RAUC A/B partitioning** | 2 days | Atomic OTA updates with instant rollback |
| 13 | **Evaluate Cage for worker kiosks** | 4 hours | Simplest possible compositor for single-app workers |
| 14 | **Write flake.nix for NixOS thin client** | 3 days | Declarative system definition; start the NixOS migration |
| 15 | **Secondary x86_64 fleet coordinator** | 1 day | Small-form-factor x86_64 host running Pixiecore + fleet orchestration |

### P3 -- Long Term (Production Hardening)

| # | Action | Effort | Impact |
|---|--------|--------|--------|
| 16 | **Complete NixOS migration** | 1-2 weeks | Full declarative system; reproducible builds; native rollback |
| 17 | **Measured boot with TPM PCR attestation** | 2 days | Secrets bound to verified boot state |
| 18 | **CI pipeline for ISO builds** | 1 day | Automated ISO build + QEMU smoke test on every merge |
| 19 | **Wolfi/Alpine sandbox containers** | 1 day | Minimal containers for agent sandboxing within the thin client |
| 20 | **Tinkerbell for hardware lifecycle** | 3 days | BMC integration, firmware updates, multi-step provisioning |

---

## Source Index

### Distro and Immutability
- [NixOS.org](https://nixos.org/)
- [NixOS-Powered AI Infrastructure](https://medium.com/@mehtacharu0215/nixos-powered-ai-infrastructure-reproducible-immutable-deployable-anywhere-d3e225fc9b5a)
- [NixOS Most Powerful Distro 2026](https://allthingsopen.org/articles/nixos-most-powerful-linux-distro-2026)
- [NixOS Declarative Revolution](https://www.webpronews.com/nixos-declarative-linux-revolution-with-reproducible-systems-and-rollbacks/)
- [Nixiosk GitHub](https://github.com/matthewbauer/nixiosk)
- [NixOS Kiosk with Cage](https://github.com/matthewbauer/nixos-kiosk)
- [How I Used NixOS for Immutable Home Lab](https://www.xda-developers.com/how-nixos-make-home-lab-immutable/)
- [Fedora Kinoite](https://fedoraproject.org/atomic-desktops/kinoite/)
- [Fedora Atomic Desktops](https://fedoramagazine.org/introducing-fedora-atomic-desktops/)
- [Vanilla OS](https://vanillaos.org/)
- [blendOS](https://blendos.co/)
- [Immutable Linux Distributions](https://itsfoss.com/immutable-linux-distros/)
- [Arch vs NixOS 2026](https://www.slant.co/versus/2690/2700/~arch-linux_vs_nixos)
- [Manjaro vs NixOS](https://www.slant.co/versus/2700/2706/~nixos_vs_manjaro-linux)

### Compositors
- [Sway ArchWiki](https://wiki.archlinux.org/title/Sway)
- [Sway Multi-Monitor Guide](https://std.rocks/sway-wayland-multi-monitor-setup.html)
- [Hyprland vs Sway 2025](https://gigasblade.blogspot.com/2025/10/hyprland-vs-swaywm-2025-dazzling.html)
- [Hyprland vs Sway Landscape 2025](https://www.oreateai.com/blog/hyprland-vs-sway-navigating-the-wayland-tiling-window-manager-landscape-in-2025/6730d963b567b07eb8f667c5e92f8b8a)
- [Cage GitHub](https://github.com/cage-kiosk/cage)
- [Cage 0.2 Released](https://www.phoronix.com/news/Cage-0.2-Released)
- [Cage Website](https://www.hjdskes.nl/projects/cage/)
- [niri GitHub](https://github.com/niri-wm/niri)
- [niri 25.01 Release](https://www.phoronix.com/news/Niri-25.01-Tiling-Wayland-Comp)
- [Arch Installer Supports labwc, niri, river](https://9to5linux.com/arch-linux-installer-now-supports-labwc-niri-and-river-wayland-compositors)
- [Wayland Compositors Ranked 2025](https://www.slant.co/topics/11023/~wayland-compositors)

### Multi-GPU and NVIDIA
- [wlroots Multi-GPU Issue #934](https://github.com/swaywm/wlroots/issues/934)
- [Multiple NVIDIA GPUs with 5 Screens (Arch Forums)](https://bbs.archlinux.org/viewtopic.php?id=303372)
- [NVIDIA open-gpu-kernel-modules Issue #318](https://github.com/NVIDIA/open-gpu-kernel-modules/issues/318)
- [RTX 4090 Cursor Lag Solution (Omarchy)](https://github.com/basecamp/omarchy/discussions/4702)
- [Wayland Multi-Monitor 2026](https://copyprogramming.com/howto/multi-monitor-issues-with-xorg-nvidia-wayland)
- [Hyprland WLR_DRM_DEVICES Issue](https://github.com/hyprwm/hyprland-wiki/issues/694)
- [NVIDIA Wayland External Monitors](https://forums.developer.nvidia.com/t/nvidia-please-get-it-together-with-external-monitors-on-wayland/301684)

### Boot and Login
- [greetd ArchWiki](https://wiki.archlinux.org/title/Greetd)
- [tuigreet GitHub](https://github.com/apognu/tuigreet)
- [greetd SourceHut](https://sr.ht/~kennylevinsen/greetd/)
- [greetd GitHub Mirror](https://github.com/kennylevinsen/greetd)

### Secrets and Security
- [systemd-creds ArchWiki](https://wiki.archlinux.org/title/Systemd-creds)
- [The Magic of systemd-creds](https://smallstep.com/blog/systemd-creds-hardware-protected-secrets/)
- [systemd.io Credentials](https://systemd.io/CREDENTIALS/)
- [systemd-creds RHEL Guide](https://oneuptime.com/blog/post/2026-03-04-systemd-credentials-secret-injection-rhel-9/view)
- [LUKS + TPM2 + systemd-cryptenroll](https://gierdo.astounding.technology/blog/2025/07/05/tpm2-luks-systemd)
- [UEFI Secure Boot ArchWiki](https://wiki.archlinux.org/title/Unified_Extensible_Firmware_Interface/Secure_Boot)
- [NSA UEFI Secure Boot Guidance (Dec 2025)](https://media.defense.gov/2025/Dec/11/2003841096/-1/-1/0/CSI_UEFI_SECURE_BOOT.PDF)
- [TPM 2.0 and Secure Boot 2026](https://servermall.com/blog/tpm-2-0-and-secure-boot-on-the-server/)
- [NVIDIA DKMS Module Signing](https://gist.github.com/lijikun/22be09ec9b178e745758a29c7a147cc9)
- [NVIDIA Driver r595 Installation Guide](https://docs.nvidia.com/datacenter/tesla/pdf/Driver_Installation_Guide.pdf)
- [1Password CLI](https://developer.1password.com/docs/cli/get-started/)
- [1Password Secrets in Scripts](https://developer.1password.com/docs/cli/secrets-scripts/)
- [opnix: 1Password for NixOS](https://github.com/mrjones2014/opnix)

### Display Management
- [kanshi GitHub](https://github.com/emersion/kanshi)
- [kanshi ArchWiki](https://wiki.archlinux.org/title/Kanshi)
- [wdisplays GitHub](https://github.com/artizirk/wdisplays)
- [nwg-displays GitHub](https://github.com/nwg-piotr/nwg-displays)

### OTA Updates
- [Rugix OTA Engines Compared (Feb 2026)](https://rugix.org/blog/2026-02-28-ota-update-engines-compared/)
- [RAUC.io](https://rauc.io/)
- [SWUpdate vs Mender vs RAUC](https://32blog.com/en/yocto/yocto-ota-update-comparison)
- [FOSDEM 2025 A/B Update Solutions](https://archive.fosdem.org/2025/events/attachments/fosdem-2025-6299-exploring-open-source-dual-a-b-update-solutions-for-embedded-linux/slides/237879/leon-anav_pyytRpX.pdf)
- [Torizon OTA Guide](https://www.torizon.io/blog/ota-best-linux-os-image-update-model)

### Fleet Deployment
- [Pixiecore README](https://github.com/danderson/netboot/blob/main/pixiecore/README.booting.md)
- [MAAS Canonical](https://canonical.com/maas)
- [MAAS Provisioning Guide](https://oneuptime.com/blog/post/2026-03-02-how-to-use-maas-metal-as-a-service-for-ubuntu-provisioning/view)
- [Tinkerbell.org](https://tinkerbell.org/)
- [Tinkerbell GitHub](https://github.com/tinkerbell/tinkerbell)
- [DigitalOcean PXE/iPXE Guide](https://www.digitalocean.com/community/tutorials/bare-metal-provisioning-with-pxe-and-ipxe)
- [Digitec Galaxus Netbooting Thin Clients](https://github.com/DigitecGalaxus/netbooting-thinclients)
- [PXE ArchWiki](https://wiki.archlinux.org/title/Preboot_Execution_Environment)

### Containers
- [Alpine Linux](https://alpinelinux.org/downloads/)
- [Alpine for Containers](https://dasroot.net/posts/2025/12/alpine-linux-containerization-docker-kubernetes/)
- [Wolfi (Chainguard)](https://www.chainguard.dev/unchained/reimagining-the-linux-distro-with-wolfi)
- [Red Hat Hummingbird](https://thenewstack.io/hummingbird-red-hats-answer-to-alpine-ubuntu-chiseled-wolfi/)
