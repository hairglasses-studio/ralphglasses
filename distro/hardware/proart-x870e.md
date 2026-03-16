# ASUS ProArt X870E-CREATOR WIFI — Linux Hardware Manifest

Target hardware for the ralphglasses thin client. All data sourced from PCI enumeration, kernel module analysis, and ASUS support documentation.

## System Summary

| Component | Value |
|-----------|-------|
| Motherboard | ASUS ProArt X870E-CREATOR WIFI |
| Chipset | AMD X870E (Granite Ridge) |
| CPU | AMD Ryzen 9 7950X (16C/32T, 5.7GHz boost) |
| iGPU | AMD RDNA2 (Raphael/Granite Ridge integrated) |
| RAM | 128GB DDR5-6000 (4x32GB) |
| GPU 1 | NVIDIA GeForce RTX 4090 (Ada Lovelace) |
| GPU 2 | NVIDIA GeForce GTX 1060 6GB (Pascal) |
| Minimum kernel | 6.8+ (MT7927 WiFi), 6.12+ recommended |
| BIOS | 2102+ required (1003 kills USB4/Thunderbolt) |

## Onboard Device Inventory

### Network

| Device | PCI ID | Kernel Module | Status | Notes |
|--------|--------|---------------|--------|-------|
| Intel I226-V 2.5GbE | `8086:125c` | `igc` | **Required** | Primary network. In-kernel since 5.15. Wired, reliable for 12h+ marathons |
| MediaTek MT7927 WiFi 7 | `14c3:7927` | `mt7925e` | Optional | In-kernel since 6.8. Use wired for marathons |
| MediaTek MT7927 Bluetooth | (USB/PCIe) | `btmtk` | **SKIP** | Hardware-broken. Persistent HCI timeout errors. Blacklist `btmtk` module |
| Marvell AQtion 10GbE | `1d6a:d108` | `atlantic` | Optional | Known link drops under sustained load. Not recommended for marathons |

### Display / GPU

| Device | PCI ID | Kernel Module | Status | Notes |
|--------|--------|---------------|--------|-------|
| NVIDIA RTX 4090 | `10de:2684` | `nvidia` (550+) | **Required** | Display output only. Install via `apt install nvidia-driver-550` |
| NVIDIA GTX 1060 6GB | `10de:1c03` | `nvidia` (560.x) | **SKIP** | Driver conflict with RTX 4090. Blacklist PCI slot |
| AMD Radeon iGPU (RDNA2) | `1002:164e` | `amdgpu` | Fallback | Ryzen 7950X integrated. Zero config, in-kernel. Use as secondary display if needed |

### Storage

| Device | PCI ID | Kernel Module | Status | Notes |
|--------|--------|---------------|--------|-------|
| AMD NVMe controller | `1022:xxxx` | `nvme` | In-kernel | Primary boot/data storage |
| AMD SATA (AHCI) | `1022:xxxx` | `ahci` | In-kernel | For SATA SSDs if connected |

### USB / Thunderbolt

| Device | PCI ID | Kernel Module | Status | Notes |
|--------|--------|---------------|--------|-------|
| AMD USB 3.2 (xHCI) | `1022:xxxx` | `xhci_hcd` | In-kernel | All USB ports |
| AMD USB4/Thunderbolt | `1022:xxxx` | `thunderbolt` | In-kernel | Requires BIOS 2102+. BIOS 1003 disables this |

### Audio

| Device | PCI ID | Kernel Module | Status | Notes |
|--------|--------|---------------|--------|-------|
| Realtek ALC4082 | `10ec:xxxx` | `snd_hda_intel` | Not needed | Thin client doesn't need audio. In-kernel if wanted |
| NVIDIA HDMI Audio (4090) | `10de:22ba` | `snd_hda_intel` | Not needed | HDMI audio passthrough. In-kernel |

### Chipset / Platform

| Device | PCI ID | Kernel Module | Status | Notes |
|--------|--------|---------------|--------|-------|
| AMD X870E PCIe Root | `1022:xxxx` | In-kernel | Auto | Platform bus |
| AMD IOMMU | `1022:xxxx` | `amd_iommu` | In-kernel | Required for GPU passthrough if ever needed |
| AMD SMBus | `1022:xxxx` | `piix4_smbus` | In-kernel | Sensor access |
| Nuvoton NCT6799 SuperIO | ISA | `nct6775` | In-kernel | Fan/temp monitoring via `lm-sensors` |

## NVIDIA Dual-GPU Constraint

This is the most important hardware constraint for the thin client.

### The Problem

The Linux NVIDIA proprietary driver loads a single `nvidia.ko` kernel module. Only one driver branch can be active:

| GPU | Architecture | Minimum Driver | Maximum Driver |
|-----|-------------|----------------|----------------|
| RTX 4090 | Ada Lovelace | 525.60 | Current (550+) |
| GTX 1060 | Pascal | 390.x | 560.x (legacy) |

Driver 550 supports RTX 4090 but also supports Pascal. However, starting with driver 590+, Pascal support is being phased out. The safe path is: **use driver 550 for the RTX 4090 and blacklist the GTX 1060 entirely**.

### Solution

1. Install `nvidia-driver-550` via apt (supports both Ada and Pascal, but we only want Ada)
2. Blacklist the GTX 1060's PCI slot via modprobe config:
   ```
   # /etc/modprobe.d/blacklist-gtx1060.conf
   # Prevent nvidia driver from binding to GTX 1060 (Pascal)
   # PCI slot identified by hw-detect.sh at first boot
   options nvidia NVreg_ExcludedGpus=GPU-<uuid>
   ```
3. Or use Xorg BusID to only configure the RTX 4090:
   ```
   # /etc/X11/xorg.conf.d/20-gpu.conf
   Section "Device"
       Identifier "nvidia-rtx4090"
       Driver     "nvidia"
       BusID      "PCI:X:Y:Z"  # RTX 4090 only
   EndSection
   ```
4. If more monitors needed beyond RTX 4090 outputs, use AMD iGPU via `amdgpu` (no driver conflict)

### What hw-detect.sh Does

The `distro/scripts/hw-detect.sh` script automates this:
- Scans `lspci -nn` for NVIDIA device IDs
- `10de:2684` = RTX 4090 (Ada) — configure as primary
- `10de:1c03` = GTX 1060 (Pascal) — blacklist
- Writes Xorg config with correct BusID
- Writes modprobe blacklist for the 1060's slot

## Windows Driver → Linux Module Cross-Reference

For anyone coming from the Windows driver archive, here's what maps to what:

| Windows Driver | Linux Equivalent | Installation |
|----------------|-----------------|-------------|
| NVIDIA GeForce 560.94 WHQL | `nvidia-driver-550` | `apt install nvidia-driver-550` |
| Intel I226-V LAN 1.0.3.28 | `igc` module | In-kernel (5.15+) |
| MediaTek Wi-Fi 7 MT7927 | `mt7925e` module | In-kernel (6.8+) |
| MediaTek Bluetooth MT7927 | `btmtk` module | **Blacklist** (hardware broken) |
| Marvell AQtion 10G | `atlantic` module | In-kernel |
| Realtek Audio HDA | `snd_hda_intel` | In-kernel |
| AMD Chipset Driver | Not needed | All in-kernel (6.1+) |
| AMD GPIO/SPI/I2C | Not needed | All in-kernel |
| ASUS Armory Crate | No equivalent | ASUS-only Windows software |
| ASUS AI Suite | No equivalent | Fan control via `lm-sensors` + `fancontrol` instead |

## Known Issues

### MT7927 Bluetooth HCI Errors (Hardware)

The MediaTek MT7927 Bluetooth controller has persistent HCI timeout errors on this motherboard. This appears to be a hardware/firmware issue, not a driver bug. Symptoms:
- `hci0: command timeout` in dmesg
- Bluetooth devices fail to pair or disconnect randomly
- System log spam every few seconds

**Mitigation:** Blacklist `btmtk` module:
```bash
echo "blacklist btmtk" | sudo tee /etc/modprobe.d/blacklist-btmtk.conf
```

### Marvell 10GbE Link Drops

The Marvell AQtion 10GbE adapter (`atlantic` driver) experiences link drops under sustained high-throughput loads. Not recommended for marathon sessions.

**Mitigation:** Use Intel I226-V 2.5GbE for all network activity. Disable Marvell interface:
```bash
echo "blacklist atlantic" | sudo tee /etc/modprobe.d/blacklist-atlantic.conf
```

### BIOS Version Sensitivity

| BIOS | Status | Notes |
|------|--------|-------|
| 1003 | **Avoid** | Breaks USB4/Thunderbolt controller entirely |
| 2102 | Minimum | Restores USB4, stable baseline |
| Latest | Recommended | Check ASUS support page for X870E-CREATOR |

### GTX 1060 on Linux

If you absolutely need the GTX 1060 on Linux (e.g., for a different machine without the 4090):
- Use `nvidia-driver-550` (still supports Pascal)
- Do NOT install alongside 4090 — only one nvidia.ko loads
- Future NVIDIA driver branches (590+) will drop Pascal

## Kernel Version Guide

| Kernel | What Works |
|--------|-----------|
| 5.15+ | `igc` (Intel 2.5GbE), basic USB, NVMe, AHCI |
| 6.1+ | Full AMD chipset support, `amdgpu` iGPU |
| 6.8+ | `mt7925e` (MT7927 WiFi 7) |
| 6.12+ | All peripherals in-tree, recommended for this board |

Ubuntu 24.04 ships kernel 6.8 by default. Install HWE kernel for 6.12+:
```bash
sudo apt install linux-generic-hwe-24.04
```

## Validation

After booting Linux on this hardware, verify with:

```bash
# Check all PCI devices detected
lspci -nn | grep -E "(NVIDIA|Intel|MediaTek|Marvell|AMD)"

# Check NVIDIA driver loaded for 4090 only
nvidia-smi

# Check network interfaces
ip link show

# Check kernel modules loaded
lsmod | grep -E "(igc|nvidia|amdgpu|mt7925|atlantic)"

# Check for BT errors (should be clean if btmtk blacklisted)
dmesg | grep -i bluetooth

# Check sensors
sudo sensors-detect  # then: sensors
```
