# Ralphglasses Distro Layer Audit

**Date:** 2026-04-04
**Scope:** `distro/` directory + `internal/wm/` Go packages
**Analyst:** Automated research pass (Claude Sonnet 4.6)

---

## 1. Compositor Abstraction Layer

### Architecture

The abstraction layer consists of two scripts:

- `distro/scripts/compositor-detect.sh` — detection logic
- `distro/scripts/compositor-cmd.sh` — unified command dispatcher

Detection priority mirrors `internal/wm/detect.go` exactly:
`SWAYSOCK` > `HYPRLAND_INSTANCE_SIGNATURE` > `I3SOCK` > `XDG_CURRENT_DESKTOP` (case-insensitive) > `pgrep` process scan.

`compositor-cmd.sh` exposes 9 commands with implementations for all three compositors:

| Command | Sway | Hyprland | i3 | Notes |
|---|---|---|---|---|
| `workspace N` | `swaymsg workspace N` | `hyprctl dispatch workspace N` | `i3-msg workspace N` | Complete |
| `exec-on N cmd` | `swaymsg workspace N; exec ...` | `dispatch workspace + exec` | `i3-msg workspace N; exec` | Complete |
| `outputs` | `swaymsg -t get_outputs` | `hyprctl monitors -j` | `i3-msg -t get_outputs` | Complete |
| `clients` | `swaymsg -t get_tree` | `hyprctl clients -j` | `i3-msg -t get_tree` | Complete |
| `reload` | `swaymsg reload` | `hyprctl reload` | `i3-msg reload` | Complete |
| `dpms on\|off` | `swaymsg output * dpms $state` | `hyprctl dispatch dpms` | Returns error (DPMS unsupported on X11/IPC) | i3 gap — expected |
| `version` | `swaymsg -t get_version` | `hyprctl version -j` | `i3-msg -t get_version` | Complete |
| `fullscreen` | `swaymsg fullscreen` | `hyprctl dispatch fullscreen 0` | `i3-msg fullscreen` | Complete |
| `move-to-workspace N` | `swaymsg move container...` | `hyprctl dispatch movetoworkspacesilent` | `i3-msg move container...` | Complete |

### Gaps and Issues

- **DPMS on i3**: `cmd_dpms` intentionally returns an error for i3. This is fine since i3 kiosk configs use `xset -dpms` directly, but callers need to handle this error.
- **hyprland exec-on**: The implementation chains two separate `hyprctl dispatch` calls without atomicity. A race exists: if workspace switching takes time, the exec may fire before the workspace is active.
- **No `get_workspaces` abstraction**: The script exposes `outputs` and `clients` but not workspace listing — callers wanting workspace state must parse compositor-specific output formats.
- **No `move-focused-output` / monitor-focus command**: No abstraction for moving windows between physical outputs, only between workspaces. The Hyprland kiosk watchdog uses the `compositor-cmd.sh clients` output with `jq` to query by class name.
- **Self-tests exist**: Both scripts include `--test` modes with mock binaries. The tests cover Sway, Hyprland, and i3 for all critical commands.
- **kiosk-setup.sh gap**: `compositor-kiosk-setup.sh` only supports `sway` and `hyprland`. i3 kiosk setup still uses the separate `kiosk-setup.sh`. The compositor-kiosk-setup.sh explicitly rejects i3: `"Unsupported compositor: $COMPOSITOR (supported: sway, hyprland)"`.

---

## 2. GPU Detection (hw-detect.sh)

### Detection Approach

`hw-detect.sh` uses `lspci -nn` with PCI vendor:device IDs hardcoded to the ProArt X870E hardware manifest:

| Device | PCI ID | Action |
|---|---|---|
| NVIDIA RTX 4090 | `10de:2684` (Ada Lovelace) | Primary display; Xorg BusID config or Sway/Hyprland monitor config |
| NVIDIA GTX 1060 | `10de:1c03` (Pascal) | Blacklisted via `modprobe.d` with `NVreg_ExcludedGpus` |
| AMD iGPU RDNA2 | `1002:164e` (Ryzen 7950X) | Secondary/overflow display via `amdgpu` |
| Intel I226-V | `8086:125c` | Detection-only; logs if absent |
| MediaTek MT7927 | `14c3:7927` | WiFi detected; BT modules (`btmtk*`) unconditionally blacklisted |
| Marvell AQtion | `1d6a:*` | Detection-only; "known stability issues" logged |

### Dual-GPU Configuration

When both RTX 4090 and AMD iGPU are present:

**X11 path** (default): Xorg config is generated with RTX 4090 as `nvidia` device using converted PCI BusID. The AMD iGPU uses the `amdgpu` module automatically. The GTX 1060 is excluded via `nvidia.NVreg_ExcludedGpus`. The `Dockerfile` hardcodes `PCI:1:0:0` / `PCI:2:0:0` as placeholders — this is the open task 4.1.6.

**Wayland path** (`--wayland-only`): Generates commented-out monitor stanzas for Sway (`/etc/sway/config.d/monitors.conf`) and Hyprland (`/etc/hyprland/monitors.conf`). These are comment-only templates — the admin must uncomment and verify port names. Sets `WLR_DRM_DEVICES` guidance for Sway (RTX 4090 primary, AMD secondary) and `AQ_DRM_DEVICES` for Hyprland.

### PRIME Offload

There is no PRIME offload configuration. All displays are driven directly — the RTX 4090 drives DP-1 through HDMI-A-1, and the AMD iGPU drives the remaining three outputs. This is direct-GPU-to-output, not PRIME render offload. PRIME offload (for rendering on one GPU and scanning out on another) is not needed and not configured.

### Issues

- `nvidia-drm.modeset=1` is checked in `/proc/cmdline` during the Wayland path, but only warns rather than aborting or automatically adding it to `/etc/default/grub`.
- The GTX 1060 blacklist writes to `/etc/modprobe.d/blacklist-gtx1060.conf` but the NVIDIA driver exclusion by PCI address (`NVreg_ExcludedGpus`) may not survive driver updates on Manjaro without a `dkms.d` hook.
- The script is not idempotent: running it twice rewrites Xorg configs, but the `ConditionPathExists=!/var/lib/hw-detect/configured` guard in `hw-detect.service` prevents double execution.

---

## 3. Multi-Monitor Support

### 7-Display Layout

Both Sway and Hyprland configs define 7 named monitor variables (`$mon1`–`$mon7`) with a 1:1 workspace-to-monitor mapping (workspaces 1–7). GPU split:
- RTX 4090: DP-1, DP-2, DP-3, HDMI-A-1 (monitors 1–4)
- AMD iGPU: HDMI-A-2, DP-4, DP-5 (monitors 5–7)

Workspaces 8–10 overflow back to monitors 1–3 in non-kiosk configs.

### Autorandr Integration (i3/X11 only)

The Go `internal/wm/autorandr/` package wraps the `autorandr` CLI with:
- `ListProfiles()` — parse `autorandr` output for profile names with current/detected markers
- `SaveProfile(name)` — `autorandr --save`
- `LoadProfile(name)` — `autorandr --load`
- `DeleteProfile(name)` — `autorandr --remove`
- `DetectAndApply()` — `autorandr --change`
- `WatchChanges(ctx, fn)` — polls every 5 seconds for profile changes, calls callback on change

The i3 kiosk config runs `autorandr --change` on startup. The Sway and Hyprland paths do not use autorandr — they rely on the compositor's native monitor handling and `hw-detect.sh --wayland-only`.

**Gap:** Roadmap tasks 3.4.1–3.4.4 (hotplug-triggered autorandr profile reload, profile-to-layout-preset linking) are all incomplete. The `WatchChanges` polling interval of 5 seconds is reasonable but does not react to `udev` events.

### Dynamic Reconfiguration

Both kiosk configs include a Sway-level (and Hyprland-level) watchdog loop that checks every 10 seconds whether each of the 7 `alacritty` instances (class `rg-ws1`–`rg-ws7`) is running, and relaunches any that have disappeared. The Sway version parses `swaymsg -t get_tree | jq` and the Hyprland version uses `compositor-cmd.sh clients | jq`.

### Layout Presets (Go)

`internal/wm/layout/presets.go` defines three built-in presets:

| Preset | Monitors | Workspaces per Monitor | Description |
|---|---|---|---|
| `single` | 1 | ws-1, ws-2, ws-3, ws-4 | Laptop / fallback |
| `dual` | 2 | ws-1,ws-2 / ws-3,ws-4 | Dev setup |
| `seven` | 7 | 1 each | Thin client target |

The `layout/manager.go` applies these presets, and `layout/commands.go` exposes them to the TUI. Roadmap task 3.2.3 (`:layout <name>` TUI command) is still open.

---

## 4. Go WM IPC Clients

### Sway (`internal/wm/sway/client.go`)

- Protocol: i3-ipc (magic `i3-ipc` + uint32 LE length + uint32 LE type)
- Persistent connection over `SWAYSOCK` Unix socket
- Methods: `RunCommand`, `GetTree`, `GetWorkspaces`, `GetOutputs`
- No event subscription support in the client package (events are handled through the i3 package pattern)
- 226 LOC; has integration test file (`sway/integration_test.go`, 580 LOC)

### Hyprland (`internal/wm/hyprland/`)

- Protocol: per-request Unix socket connections to `.socket.sock`; event socket at `.socket2.sock`
- Socket location: XDG_RUNTIME_DIR/hypr/$SIG (fallback: /tmp/hypr/$SIG); handles both Hyprland >= 0.40 and older
- Methods: `GetMonitors`, `GetWorkspaces`, `GetWindows`, `GetActiveWindow`, `Dispatch`, `MoveToWorkspace`, `GetVersion`
- Full event listener in `events.go` with reconnect backoff (100ms initial, 5s max)
- Events: workspace, activewindow, openwindow, closewindow, monitoradded, monitorremoved, submap, fullscreen, moveworkspace, urgent, configreloaded
- Most complete IPC implementation of the three: structured `Window` type with 19 fields, per-request connection model avoids stale-socket issues

### i3 (`internal/wm/i3/`)

- Protocol: i3-ipc (same as Sway; Sway is wire-compatible)
- Separate packages for client, events, monitors, windows, workspaces
- `client.go`: `sendMessage`, `runCommand` — lower-level than Sway client
- `events.go`: full event listener with subscribe handshake, reconnect backoff, 4 event types (workspace, output, mode, window)
- `monitors.go`: `ListMonitors`, `ActiveMonitors`, `PrimaryMonitor`, `MonitorByName`, `ArrangeForFleet`
- `workspace.go`: `GetWorkspaces`, `FocusWorkspace`, `MoveToWorkspace`
- `window.go` (not read, exists per glob)

### Feature Parity Comparison

| Feature | Sway | Hyprland | i3 |
|---|---|---|---|
| Connect | SWAYSOCK | HYPRLAND_INSTANCE_SIGNATURE | I3SOCK or `i3 --get-socketpath` |
| Run command | Yes | Via `Dispatch` | Yes |
| List monitors | Yes (`GetOutputs`) | Yes (`GetMonitors`) | Yes (`ListMonitors`) |
| List workspaces | Yes | Yes | Yes |
| List windows | Via `GetTree` (parse required) | Yes (`GetWindows`, structured) | Via tree parse |
| Active window | No dedicated method | Yes (`GetActiveWindow`) | No dedicated method |
| Event subscription | No (no event socket in sway client) | Yes (full socket2 listener) | Yes (subscribe handshake) |
| Move window | Via `RunCommand` | `MoveToWorkspace` + address | `MoveToWorkspace` (workspace name) |
| DPMS control | Via `RunCommand` | Via `Dispatch("dpms")` | Not via IPC |
| Monitor hotplug events | No | `monitoradded`/`monitorremoved` | `OutputEvent` |
| Version query | `MsgTypeGetVersion` | `GetVersion` | `MsgTypeGetVersion` |

**Most complete:** Hyprland — structured types, active window query, full event set, per-connection model avoids stale state.

**Gaps:** The Sway client lacks an event subscription mechanism (the i3 events package could be reused since Sway is wire-compatible, but it is not). The `DetectMonitors()` dispatcher in `monitors.go` only handles Sway and Hyprland; it explicitly returns an error for i3 ("use `ParseXrandrOutput` for X11"), meaning TUI monitor detection falls back to xrandr text parsing for i3 users.

---

## 5. Boot Pipeline

### Full Chain: Power-on to TUI

```
UEFI firmware
  └── GRUB (grub.cfg — 5 menu entries, 5s timeout)
        └── kernel vmlinuz (nvidia-drm.modeset=1 in default entry)
              └── initrd.img (live-boot or installed system)
                    └── systemd (graphical.target)
                          ├── hw-detect.service (ConditionPathExists=!/var/lib/hw-detect/configured)
                          │     └── hw-detect.sh → Xorg config, modprobe blacklists
                          ├── ralphglasses-firstboot.service (ConditionPathExists=!/etc/ralphglasses/.firstboot-done)
                          │     └── ralphglasses firstboot → interactive first-boot wizard on tty1
                          ├── ts-enroll.service → ts-enroll.sh → Tailscale enrollment
                          ├── getty@tty1 (autologin ralph → bash_profile)
                          │     └── bash_profile: exec sway / exec Hyprland (Wayland) or exec startx (X11)
                          │           └── Sway/Hyprland/i3 compositor
                          │                 └── kiosk config exec-once / exec
                          │                       └── alacritty × 7 (staggered 0–3s)
                          │                             └── ralphglasses --scan-path /workspace
                          └── ralphglasses.service (After=graphical-session.target)
                                └── ralphglasses --scan-path /workspace (fallback if TUI dies)
```

### GRUB Configuration

`distro/grub/grub.cfg` (template; `distro/boot/grub.cfg` is used by build-iso.sh) defines 5 entries:
1. Ubuntu (NVIDIA RTX 4090) — default, `nvidia-drm.modeset=1`
2. Ubuntu (AMD iGPU) — `amdgpu.dc=1`
3. Headless / Serial Console — dual `console=tty0,ttyS0,115200n8`
4. AMD iGPU fallback — `nomodeset`, all NVIDIA modules blacklisted on cmdline
5. Recovery mode — single user, `nomodeset`

The GRUB config uses a `ROOTFS-UUID-HERE` placeholder that must be substituted by the installer. Serial console (`115200,8n1`) is mirrored to `terminal_input/output serial console` for both BIOS and UEFI debugging.

### ISO Build Process

Two paths via `distro/Makefile`:
1. **Docker path** (primary): `make docker-sway` or `make docker-hyprland` → `make rootfs` (docker export + tar) → `make iso` (mksquashfs xz + xorriso UEFI hybrid)
2. **Debootstrap path**: `make rootfs-debootstrap` → `make chroot` (bind-mount + chroot-setup.sh) → `make iso-debootstrap`

The ISO uses live-boot (Debian `boot=live netboot=nfs nfsroot=...` parameters). The squashfs uses xz compression with x86 BCJ filter, 1M block size.

**Open issue 4.1.3:** `make iso` in the Makefile uses `xorriso` directly, not `grub-mkrescue`. The `build-iso.sh` script uses `grub-mkrescue`. These are parallel implementations with slightly different EFI bootloader selection logic.

### PXE Boot

`distro/pxe/ipxe.cfg` defines an iPXE chainload menu with 4 boot options:
- `live` — NFS root with NVIDIA `nvidia-drm.modeset=1`
- `live-http` — HTTP squashfs with `toram` (copies to RAM before boot)
- `live-igpu` — NFS root with AMD iGPU (`amdgpu.dc=1`, no NVIDIA params)
- `headless` — NFS root with serial console, `systemd.unit=multi-user.target`

The PXE config sets `overlayroot=tmpfs` on all options (writes go to RAM). DHCP retry loop (5 retries, 3s delay). The LTSP server setup script (`pxe/ltsp-setup.sh`) and overlay config (`pxe/overlay.conf`) exist but the LTSP server provisioning is a Phase 4.3 open task.

### cloud-init (Worker Nodes)

`distro/cloud-init/worker.yaml` provisions headless fleet worker VMs with:
- Go install from go.dev
- Tailscale enrollment via `/opt/ralphglasses/env` `TAILSCALE_AUTH_KEY`
- `git clone` + `go build` of ralphglasses
- `ralphglasses-worker.service` with systemd hardening (`NoNewPrivileges`, `ProtectSystem=strict`, `PrivateTmp`)
- `RALPH_MAX_SESSIONS=4` default cap

---

## 6. Docker Builds

Three Dockerfiles produce distinct rootfs images:

| Dockerfile | Base | Compositor | Use Case |
|---|---|---|---|
| `Dockerfile` | Ubuntu 24.04 | Xorg + i3 | Legacy/CI reference, live-boot ISO, X11 environments |
| `Dockerfile.manjaro` | Manjaro Linux | Sway (Wayland) | Primary thin client target — Manjaro + Sway + NVIDIA |
| `Dockerfile.manjaro.hyprland` | Manjaro Linux | Hyprland (Wayland) | Experimental alternative — Manjaro + Hyprland + AUR packages |

All three: build ralphglasses from source, install Claude Code CLI via npm, create user `ralph` with passwordless sudo, configure autologin + compositor autostart via `getty@tty1` override + `~/.bash_profile`.

**Ubuntu (Dockerfile):** Installs NVIDIA driver 550 from apt, sets up dual-GPU Xorg config (hardcoded PCI bus IDs — open issue 4.1.6), installs `autorandr` + `arandr`, configures NetworkManager with Marvell/Intel priority, adds udev rule for TP-Link UB400 CSR8510 BT clone.

**Manjaro Sway:** Uses `pacman`, installs `nvidia`, `sway`, `waybar`, `swayidle`, `swaylock`, `wl-clipboard`. Copies the three compositor abstraction scripts to `/usr/local/bin/`. Adds `nvidia-drm.conf` for `modeset=1`. No `autorandr` (not needed for Wayland).

**Manjaro Hyprland:** Same as Sway base but installs `hyprland` instead of `sway`. Installs `hypridle`, `hyprlock`, `xdg-desktop-portal-hyprland` from AUR via a temporary `aurbuild` user. This is the only Dockerfile that installs AUR packages, making it the most fragile build — AUR repos can break.

All three Dockerfiles use `systemctl enable` during build inside a container without a running systemd. On Manjaro this is harmless (creates symlinks), but the `2>/dev/null || true` suppression hides whether the enable actually worked.

---

## 7. Tailscale Integration

### ACL Policy (`distro/tailscale/policy.json`)

Four tags: `ralph-fleet`, `ralph-coordinator`, `ralph-worker`, `ralph-mcp`.

ACL rules:
1. Fleet-internal: all `ralph-fleet` nodes can reach each other on any port (bidirectional)
2. MCP servers (`ralph-mcp`) can reach fleet nodes (tool dispatch)
3. Admins (`autogroup:admin`) can reach all fleet nodes

SSH rules:
- Coordinator → worker: unrestricted SSH (no re-auth)
- Admin → any fleet node: SSH with 12h re-auth (`action: check`)

Route auto-approvers: `10.0.0.0/8` and `172.16.0.0/12` auto-approved for `ralph-fleet` — enables subnet routing for LAN access without manual approval.

### Enrollment Flow (`distro/scripts/ts-enroll.sh`)

1. Skip if `/var/lib/tailscale/ralph-enrolled` marker exists (idempotent)
2. Derive hostname from primary NIC MAC address: `ralph-<last 6 hex digits>` (e.g., `ralph-a1b2c3`)
3. Read auth key from `/etc/ralphglasses/ts-authkey` (must be provisioned before boot)
4. Wait up to 30s for `tailscaled` to be ready
5. `tailscale up --authkey=... --advertise-tags=tag:ralph-fleet,tag:ralph-worker --hostname=... --ssh --accept-routes`
6. Create marker, delete auth key file (single-use)

Triggered by `distro/systemd/ts-enroll.service` (exists in file list but not read — it runs ts-enroll.sh).

**Security gap:** The Tailscale auth key is stored as a plain file at `/etc/ralphglasses/ts-authkey`. This must be provisioned onto the ISO or disk image before first boot. There is no 1Password CLI integration or ephemeral key handling — the key must be baked in or injected via cloud-init. The script does delete the key after successful enrollment, but it is on-disk before enrollment completes.

---

## 8. Hardware Profiles

Three profiles in `distro/hardware/profiles/`:

### `default.json`
- Generic x86_64, 4-core, any GPU (single), single monitor, 16GB RAM
- No Xorg customization (`xorg.gpu_conf: false`, `xorg.multi_head: false`)
- Kernel minimum: 5.15
- No module blacklists
- Role: fallback when no specific hardware is detected

### `mini-pc.json`
- Intel NUC-style, i3/i5/i7, Intel UHD/Iris iGPU, single monitor, 16GB RAM, NVMe
- Intel i915 driver, WiFi enabled
- No Xorg customization needed (i915 handles single-display fine)
- Kernel minimum: 5.15
- Role: compact single-session deployment or relay agent node

### `proart-x870e.json`
- ASUS ProArt X870E-CREATOR WIFI primary target
- CPU: AMD Ryzen 9 7950X, 32 cores
- GPUs: RTX 4090 (primary, `10de:2684`, nvidia-driver-550), GTX 1060 (skip/blacklist, `10de:1c03`)
- iGPU: AMD RDNA2 Raphael (`1002:164e`, `amdgpu`, fallback display)
- 7 monitors, 128GB DDR5, NVMe
- Network: Intel I226-V 2.5GbE (`8086:125c`) + MediaTek MT7927 WiFi 7 (`14c3:7927`), 2500 Mbps
- Xorg multi-head enabled
- Kernel minimum: 6.12 (required for MT7927 WiFi driver `mt7925e`)
- Blacklists: `btmtk`, `nvidia (GTX 1060 PCI slot only)`

### Profile Usage Gap

The profiles are JSON documents — there is no Go code that reads and applies them at runtime. `hw-detect.sh` has its own hardcoded PCI IDs and is not driven by these JSON files. Roadmap tasks 4.4.1–4.4.4 (generalize `hw-detect.sh` from the JSON profile table, validate profiles against running system) are all open.

---

## 9. Phase 3 and Phase 4 Progress

### Phase 3: i3 Multi-Monitor Integration

**Complete:**
- 3.1.1–3.1.3: i3 IPC client (connect, events, workspace CRUD, window management)
- 3.2.1–3.2.2: Layout presets as JSON; 7-monitor config
- 3.2.5: Graceful missing monitor handling
- 3.3.2–3.3.4: Instance discovery, leader election, leader failover
- 3.5.1–3.5.13: Full Sway/Wayland compatibility including Sway IPC client, monitor integration, Manjaro Dockerfile, kiosk setup
- 3.6.2–3.6.3: Hyprland workspace dispatch and monitor configuration

**Open:**
- 3.1.4: Monitor enumeration via i3 IPC (exists in `i3/monitors.go` as `ListMonitors` — roadmap item may be stale)
- 3.1.5: i3 event listener (exists in `i3/events.go` — may also be stale)
- 3.2.3: TUI command `:layout <name>` to apply a preset
- 3.2.4: Save current layout as custom preset
- 3.3.1: Shared SQLite state for multi-instance coordination
- 3.4.1–3.4.4: autorandr hotplug integration (full section open)
- 3.6.1: Hyprland IPC client (exists in `internal/wm/hyprland/` — likely stale roadmap item)
- 3.6.4: Dynamic Hyprland workspaces

**Assessment:** The i3 and Sway Go client code appears more complete than the roadmap reflects. Several items marked open (`3.1.4`, `3.1.5`, `3.6.1`) correspond to Go packages that exist. The likely actual gaps are TUI integration (`:layout` command) and the autorandr hotplug pipeline.

### Phase 4: Bootable Thin Client

**Complete:**
- `distro/Dockerfile` (Ubuntu/i3)
- `hw-detect.sh` with GPU detection, blacklisting
- `hw-detect.service` (first-boot oneshot)
- `ralphglasses.service` (TUI autostart)
- Makefile targets `build` and `squashfs`
- 4.7.1: Systemd watchdog unit
- 4.7.4: Heartbeat file (watchdog-heartbeat.sh present)
- 4.8.3: Marathon restart logic with cap + backoff
- 4.8.5: Marathon summary report
- Kiosk configs for all three compositors (i3, Sway, Hyprland)
- PXE iPXE config
- install-to-disk.sh (interactive + unattended modes)
- Secure boot scripts (sign-kernel.sh, mok-enroll.sh)
- Tailscale enrollment (ts-enroll.sh, policy.json)
- Hardware profiles (JSON), cloud-init worker config
- Hyprland and Sway Dockerfiles (Dockerfile.manjaro, Dockerfile.manjaro.hyprland)

**Open (blocking or high priority):**
- 4.1.3: `make iso` — the Makefile iso target exists but uses `xorriso` directly; whether it produces a correctly bootable image has not been smoke-tested (`make test-vm` target exists but depends on OVMF being installed)
- 4.1.4: QEMU smoke test automation
- 4.1.5: CI/GitHub Actions ISO build
- 4.1.6: Fix hardcoded Xorg PCI BusID in Dockerfile (uses `PCI:1:0:0` / `PCI:2:0:0`)
- 4.1.7: Network priority alignment (Dockerfile uses Marvell 10GbE primary but docs say Intel I226-V)
- 4.2.1–4.2.6: i3 kiosk config (the distro/i3/kiosk-config exists but roadmap marks all tasks open — likely stale)
- 4.3.1–4.3.5: PXE/LTSP server provisioning (iPXE config exists; server setup does not)
- 4.4.1–4.4.4: Profile-driven hw-detect.sh
- 4.5.1–4.5.5: install-to-disk.sh (script exists and is comprehensive — 3.4.x tasks likely stale)
- 4.6.1–4.6.4: OTA update mechanism (not started)
- 4.7.2–4.7.3: Hardware health checks (GPU temp, disk, memory), alert escalation
- 4.8.1–4.8.2: Marathon disk/memory monitoring
- 4.9.1–4.9.4: Secure boot (scripts exist; integration into ISO build and NVIDIA module signing not done)

**Critical path for first bootable ISO:**
1. Resolve 4.1.3 (ISO build validation via QEMU smoke test)
2. Fix 4.1.6 (Xorg PCI BusID) — otherwise X11 boot will fail on most hardware
3. Validate `hw-detect.service` fires before display manager on target hardware
4. Test Sway kiosk autostart with actual NVIDIA driver on RTX 4090

---

## 10. Security

### Secure Boot

`distro/secureboot/sign-kernel.sh` generates an RSA 4096 key pair (10-year validity, `codeSigning` EKU) using `openssl`, signs the kernel with `sbsign`, verifies with `sbverify`, and replaces the original (backing up to `vmlinuz-*.unsigned`).

`distro/secureboot/mok-enroll.sh` imports the DER certificate into the shim MOK database via `mokutil --import`, creating a pending enrollment that requires a physical reboot with UEFI interaction to complete.

**Status:** Scripts are well-implemented with dry-run modes, logging, and idempotency markers. However, roadmap tasks 4.9.1–4.9.4 are all open. NVIDIA kernel module signing (required for Secure Boot + proprietary NVIDIA driver) is not addressed in any script. Without signed NVIDIA modules, Secure Boot + NVIDIA is non-functional.

### Drive Encryption

No drive encryption is configured. `install-to-disk.sh` creates a plain ext4 root partition with no LUKS layer. There is no mention of encryption in the install script, Dockerfiles, or cloud-init. This is a significant gap for a thin client holding API keys for LLM providers.

### API Key Management on Thin Client

API keys are expected to be in environment variables or a `.ralphrc` file. There is no 1Password CLI integration, no secrets manager, and no encrypted key store. The `cloud-init/worker.yaml` uses a `EnvironmentFile=/opt/ralphglasses/env` with `TAILSCALE_AUTH_KEY=` left blank — the key must be injected before the service starts.

The `ts-enroll.sh` script reads the Tailscale auth key from `/etc/ralphglasses/ts-authkey` as a plain file and deletes it after enrollment. This is a reasonable one-time-use pattern for Tailscale, but LLM API keys (ANTHROPIC_API_KEY, OPENAI_API_KEY) have no equivalent treatment.

### Systemd Service Hardening

`cloud-init/worker.yaml` worker service has meaningful hardening:
- `NoNewPrivileges=yes`
- `ProtectSystem=strict`
- `ReadWritePaths=/opt/ralphglasses`
- `PrivateTmp=yes`
- `ProtectHome=yes`

`rg-status-bar.service` has `MemoryMax=64M` and `CPUQuota=10%` resource limits.

Main `ralphglasses.service` has no hardening directives — it runs as user `ralph` with passwordless sudo and no namespace restrictions. The `watchdog.service` similarly has no hardening beyond `User=ralph`.

**Summary of security posture:** Adequate for a development / homelab deployment. Not suitable for production or untrusted network environments without: drive encryption, API key management via a secrets store, NVIDIA module signing for Secure Boot, and hardened systemd units for the main service.
