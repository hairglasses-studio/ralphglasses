# Service Paths

Systemd units for the ralphglasses thin client (Ubuntu 24.04, i3).

Source directory: `distro/systemd/`
Install target: `/etc/systemd/system/`

## Units

| Unit | Type | Install Path | Purpose |
|------|------|-------------|---------|
| `hw-detect.service` | oneshot | `/etc/systemd/system/hw-detect.service` | First-boot hardware detection: GPU enumeration, Xorg config, module blacklists |
| `ralphglasses.service` | simple | `/etc/systemd/system/ralphglasses.service` | Main TUI agent fleet manager, runs as user `ralph` on display `:0` |

## Unit Details

### hw-detect.service

Runs `/usr/local/bin/hw-detect.sh` (from `distro/scripts/hw-detect.sh`) exactly once on first boot.

- **Ordering**: After `local-fs.target`, before `display-manager.service`
- **Guard**: `ConditionPathExists=!/var/lib/hw-detect/configured` -- skips on subsequent boots
- **Side effects**: Creates `/var/lib/hw-detect/configured` sentinel after success
- **Writes**:
  - `/etc/X11/xorg.conf.d/20-gpu.conf` -- Xorg BusID for RTX 4090
  - `/etc/modprobe.d/blacklist-gtx1060.conf` -- excludes GTX 1060 from nvidia driver
  - `/etc/modprobe.d/blacklist-btmtk.conf` -- disables broken MT7927 Bluetooth
- **Log**: `/var/log/hw-detect.log`

### ralphglasses.service

Runs the `ralphglasses` TUI binary after the graphical session is up.

- **Ordering**: After `graphical.target` (which implies display-manager is running)
- **User**: `ralph` (non-root)
- **Environment**: `DISPLAY=:0`, `RALPHGLASSES_SCAN_PATH=/workspace`
- **Restart**: On failure, 5-second delay

## Boot Sequence

```
local-fs.target
    |
    v
hw-detect.service  (first boot only)
    |
    v
display-manager.service  (i3 + X11)
    |
    v
graphical.target
    |
    v
ralphglasses.service
```

## Dependencies

- `hw-detect.service` must complete before the display manager starts, so that Xorg picks up the generated GPU config in `/etc/X11/xorg.conf.d/20-gpu.conf`.
- `ralphglasses.service` depends on `graphical.target` because it needs a running X display (`DISPLAY=:0`) for TUI rendering in a terminal emulator.

## Hardware Detection Cross-Reference

The `hw-detect.sh` script (invoked by `hw-detect.service`) performs the following detection steps relevant to system services:

| Detection Step | Config Written | Affects |
|---------------|---------------|---------|
| RTX 4090 PCI scan (`10de:2684`) | `/etc/X11/xorg.conf.d/20-gpu.conf` | `display-manager.service` (Xorg) |
| GTX 1060 PCI scan (`10de:1c03`) | `/etc/modprobe.d/blacklist-gtx1060.conf` | nvidia module loading at boot |
| AMD iGPU scan (`1002:164e`) | Xorg fallback config (if no NVIDIA found) | `display-manager.service` (Xorg) |
| MT7927 Bluetooth | `/etc/modprobe.d/blacklist-btmtk.conf` | btmtk module loading at boot |
| Intel I226-V (`8086:125c`) | (logged only) | Network availability |

## GRUB Integration

The GRUB config (`distro/boot/grub.cfg`) provides an "AMD iGPU fallback" boot entry that blacklists all NVIDIA modules via kernel command line. This is the manual escape hatch when NVIDIA GPUs fail to initialize -- it bypasses `hw-detect.sh` entirely by preventing nvidia modules from loading.
