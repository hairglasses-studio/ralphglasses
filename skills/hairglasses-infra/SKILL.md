---
name: hairglasses-infra
description: Infrastructure and platform reference for the hairglasses-studio homelab and cloud setup. Use when discussing UNRAID, OPNsense firewall, Terraform, rclone mounts, 1Password CLI, network configuration, Docker, Kubernetes, or server management.
---

# hairglasses-studio Infrastructure

## Hardware

- **Workstation**: Ryzen 9 7950X (16C/32T), 128GB DDR5, RTX 3080, liquid-cooled
- **Monitor**: Samsung LC49G95T ultrawide (3840x1080@240Hz)
- **Storage**: Phison E18 NVMe, Btrfs with zstd compression
- **Network**: MediaTek MT7922 WiFi 6E + Aquantia 10GbE
- **Keyboard**: Drop CTRL (QMK, USB) — always US QWERTY
- **Mouse**: Logitech MX Master 4 (Bluetooth, logiops 0.3.5, DPI 4000)

## Cloud Storage

| Mount | Service | Path | Systemd Unit |
|-------|---------|------|-------------|
| Google Drive | rclone | `~/gdrive` | `rclone-gdrive.service` (8 transfers, 128M chunks) |
| MEGA.nz | rclone | `~/mega` | `rclone-mega.service` (4 transfers, conservative) |

## 1Password CLI

```bash
op item get "secret-name" --vault "vault" --fields password
# Account shorthand: my
```

## UNRAID Server

Managed via `unraid-monolith` (Go MCP server). Services: Docker containers, storage pools, VMs, backup management.

## OPNsense Firewall

Managed via `opnsense-monolith` (Go MCP server). Features: VLANs, firewall rules, VPN, HAProxy, intrusion detection.

## Terraform

`aftrs-terraform` — IaC for Aftrs Studio operations (AWS).

## MCP Platform Servers

| Server | Tools | Scope |
|--------|-------|-------|
| hg-mcp | 1,190 | DJ/VJ/creative studio (10 runtime groups) |
| mesmer | 1,790 | MSP/IT consulting (70 modules) |
| webb | 1,371 | Enterprise ops (Kubernetes, AWS, DBs) |
| shielddd | 115 | Employment law evidence |

## MCP Transport Modes

| Mode | Use Case | Status |
|------|----------|--------|
| stdio | Local Claude Code | Default, lowest latency |
| Streamable HTTP | Remote/networked | MCP 2025-03-26 spec |
| SSE | Legacy HTTP streaming | Deprecated April 2026 |

Control via `MCP_MODE` environment variable.

## Docker

Multi-platform builds: `docker buildx --platform linux/amd64,linux/arm64`

## Network

- 10GbE for UNRAID NAS access
- WiFi 6E for general connectivity
- OPNsense manages VLANs and routing

## OS

Manjaro Linux (Arch-based, rolling release). Packages via `pacman` / `yay`. Kernel: `6.12.x` with NVIDIA proprietary drivers. GPU compute on RTX 3080 via nouveau, display on AMD iGPU.
