# Agent OS & Sandboxing Research

Research conducted March 2026 for the ralphglasses thin client deliverable.

## Executive Summary

No production-ready "Agent OS" exists yet, but the ecosystem is converging fast. **StereOS** (NixOS + gVisor) is the closest purpose-built agent OS but is early alpha. The practical path today is **Docker Sandboxes** (production-ready, official Claude Code template) or **NixOS microvm.nix** (strongest documented real-world isolation). For a homelab thin client, the supported deployment path is an **x86_64 Linux kiosk image** today, with agent sandboxes running on the server side (UNRAID/Proxmox).

---

## Track 1: Purpose-Built Agent OS Projects

### StereOS
- **Repo**: [papercomputeco/stereOS](https://github.com/papercomputeco/stereOS)
- NixOS-based OS hardened for AI agents
- Produces "mixtapes" — machine images bundling hardened Linux + agent harnesses
- gVisor sandboxing + /nix/store namespace mounting
- Output: raw EFI disk images, QCOW2, kernel files
- **Maturity**: Alpha (2026)

### AgenticCore / AgenticArch
- **Repo**: [MYusufY/agenticcore](https://github.com/MYusufY/agenticcore)
- Self-described "World's first agentic Linux distro"
- Embedded chatbot that generates and executes system scripts
- AgenticArch is Arch Linux-based successor with UEFI support
- **Maturity**: Proof-of-concept

### AIOS (AI Agent Operating System)
- **Repo**: [agiresearch/AIOS](https://github.com/agiresearch/AIOS) (~4,100 stars)
- Academic: LLM embedded in OS kernel layer
- Scheduling, context management, memory management for runtime agents
- LLM-based semantic file system with Terminal UI
- **Maturity**: Research-grade (COLM 2025, ICLR 2025)

### Michael Stapelberg's microvm.nix Pattern
- **Blog**: [Coding Agent VMs on NixOS](https://michael.stapelberg.ch/posts/2026-02-01-coding-agent-microvm-nix/)
- Lightweight NixOS VMs via microvm.nix flakes for Claude Code
- Source repos and build deps mounted via virtiofs, agent has no personal file access
- Ephemeral VMs — throw away on compromise
- **Maturity**: Production-quality documented pattern, actually in use

### NixOS Tooling
- [sadjow/claude-code-nix](https://github.com/sadjow/claude-code-nix) — Always up-to-date Nix flake (hourly CI)
- [numtide/llm-agents.nix](https://github.com/numtide/llm-agents.nix) — Nix packages for AI coding agents
- [roman/mcps.nix](https://github.com/roman/mcps.nix) — MCP server presets for home-manager
- [devenv.sh](https://devenv.sh/integrations/claude-code/) — Claude Code integration

---

## Track 2: Container OS Landscape

### Comparison Matrix

| OS | Version | Footprint | K8s Required? | Agent Evidence | Homelab Fit | Pick |
|---|---|---|---|---|---|---|
| **NixOS** | 25.11 | 2-4 GB | No | Multiple real implementations | Excellent | **Top** |
| **Fedora CoreOS** | F43 | ~700 MB | No | None specific | Excellent | **Runner-up** |
| **Kairos** | v4.0.0 | Varies | Optional (K3s) | CNCF edge AI blog | Good | Build-your-own |
| **Talos Linux** | v1.11 | 120 MB | Yes (only) | Sidero blog | Proxmox, bare metal | K8s-native |
| **Flatcar** | Stable | ~700 MB | No | None | Proxmox (community) | Simple Docker host |
| **Bottlerocket** | v1.49 | ~600 MB | Yes | None | AWS only | Not recommended |
| **Ubuntu Core** | UC24 | ~400 MB | No | None | Limited | Not recommended |

### Key Findings
- **NixOS** is the clear winner with the most real-world agent hosting adoption
- **Fedora CoreOS** with Podman Quadlet systemd units is the pragmatic alternative
- **Kairos** lets you build a "Claude Code OS" via Dockerfile → bootable ISO
- Nobody has published Talos/Flatcar/CoreOS agent-specific configs — that's a gap

### Container Base Images
- **Wolfi/Chainguard** — Best secure base (glibc, zero CVEs, ~5.5MB)
- **Alpine** — Smallest (5.3MB) but musl compatibility risk with Node.js native modules
- **Debian slim** — Safe fallback for full glibc compatibility

---

## Track 3: Sandboxing & Isolation

### Comparison Matrix

| Approach | Isolation | Startup | Memory | Kernel Shared? | Escape Resistance | Docker-in-Docker? |
|---|---|---|---|---|---|---|
| **Bubblewrap** | Namespace | Microseconds | ~0 | Yes | Low (bypassed) | No |
| **Firejail** | Namespace + seccomp | Microseconds | ~0 | Yes | Medium | No |
| **gVisor** | User-space kernel | Milliseconds | Moderate | No (intercepted) | High | Limited |
| **Kata Containers** | Hardware VM | ~200ms | Moderate | No | Very High | Yes |
| **Firecracker** | Hardware VM | ~125ms | <5 MiB | No | Very High | No |
| **Docker Sandboxes** | MicroVM | Seconds | Moderate | No | Very High | Yes (built-in) |
| **E2B** | Firecracker | <200ms | <5 MiB | No | Very High | No |
| **microvm.nix** | Hardware VM | Seconds | 4GB | No | Very High | Possible |

### Key Findings

**Bubblewrap is insufficient against reasoning agents.** Ona security research demonstrated Claude Code autonomously discovered bypasses: `/proc/self/root/usr/bin/npx` to evade path denylists, and the ELF dynamic linker bypass (`ld-linux-x86-64.so.2`) which loads binaries via `mmap` instead of `execve`.

**Hardware virtualization is the only reliable boundary.** Firecracker (<5 MiB, 125ms), Kata Containers, and Docker Sandboxes (microVM) provide escape-resistant isolation.

**Simon Willison's "Lethal Trifecta"**: The combination of (1) private data access, (2) untrusted content exposure, and (3) external communication creates exploitable systems. Coding agents inherently need all three — no current approach fully resolves this tension. The mitigation is to remove one leg: network isolation blocks exfiltration, filesystem isolation limits data access.

### Production Sandboxing Options

**Docker Sandboxes** (GA Jan 2026): Official Docker feature. MicroVM per agent, Claude Code template included. Agents can build/run Docker inside the sandbox without host Docker access. macOS/Windows only currently — Linux planned.

**kubernetes-sigs/agent-sandbox** (v0.1.0): K8s CRD from Google. gVisor + Kata, WarmPools for pre-warmed pods.

**alibaba/OpenSandbox** (~5,900 stars): Multi-runtime (Firecracker, gVisor, Kata). Python/Java/JS/C# SDKs.

**E2B** (production): Firecracker microVMs, <200ms boot via snapshot restoration. Python/TypeScript SDKs.

**Cloudflare Sandbox SDK** (open beta): Firecracker on edge. Native Workers AI integration. Claude Code tutorial available.

---

## Track 4: Homelab-Specific

### UNRAID Agent Hosting
- **UNRAID Tab Plugin** — Embeds Claude Code, GeminiCLI, OpenCode into UNRAID WebUI tabs (March 2026)
- **unraid-claude-code** — Plugin installing Claude Code CLI on UNRAID (runs as root, no sandbox)
- **Unraid MCP Server** — 10 tools / 76 actions for UNRAID management via GraphQL
- **UnraidClaw** — Permission-enforcing REST API gateway for AI agents

### Proxmox
- **OCI Container Support** (9.1, Nov 2025) — Pull Docker images directly into LXC containers
- **Helper Scripts** — Community scripts for Ollama, Open-WebUI with GPU passthrough auto-detection
- **LXC GPU Passthrough** — Share NVIDIA GPU across multiple LXC services

### GPU Passthrough (Dual NVIDIA)
- Each GPU gets its own IOMMU group
- GPU A → local inference (Ollama/vLLM in Proxmox LXC)
- GPU B → display output or second compute workload
- LXC device passthrough: `/dev/nvidia*` + matching driver inside container

### Persistent Workspace Patterns
- **Git worktrees** — Dominant pattern. Each agent gets isolated working directory sharing `.git`
- **code-on-incus** — Go-based, most complete solution: `/workspace` mount, multi-slot, SSH forwarding
- **Docker Sandboxes** — Workspace as volume, container ephemeral, workspace persists
- **NFS mounts** — Export from UNRAID, mount into agent containers

### MCP Server Hosting
- **MCPJungle** — Self-hosted MCP gateway. Single binary, unified access to multiple upstream servers
- **Homelab MCP Server** — Python-based, exposes SSH discovery, Proxmox API, VM lifecycle
- **Docker MCP Gateway** — Docker's built-in gateway for sandbox integration
- **Pattern**: MCP servers on host/server side, agents connect over STDIO or HTTP

### Network Isolation
- iptables default-deny with domain whitelisting
- code-on-incus: Restricted, Allowlist, or Open network modes + threat detection
- Proxmox SDN: VLAN/VxLAN zones (native in 9.1)
- Claude Code official devcontainer includes built-in firewall

### Thin Client Distros

| Distro | Size | Key Feature | Fit |
|--------|------|-------------|-----|
| **Manjaro** | ~3-5GB | Existing NVIDIA-friendly kiosk build path | **Best current path** |
| **NixOS** | ~2-4GB | Declarative config, rollback, strong long-term fit | **Best long-term path** |
| Tiny Core | 16-21MB | Runs in RAM, modular extensions | Ultra-minimal |
| ThinStation | ~50MB | PXE-native, RDP/VNC/SSH | Network boot |
| Porteus | ~300MB | Modular, boots from USB | Portable |
| Slax | ~300MB | Debian, persistent USB | Simple |

### i3 Multi-Monitor (7 displays)
- i3 supports arbitrary output count via RandR API
- Each output gets workspaces assigned: `workspace N output $monN`
- **go-i3** library for programmatic IPC from Go
- **autorandr** for persisting xrandr layouts across reboots
- Kiosk pattern: auto-start TUI fullscreen, disable WM chrome

### Recommended Architecture
1. **Server**: UNRAID (storage + Docker) + Proxmox node (agent sandboxes)
2. **GPU split**: GPU A → inference (Ollama LXC), GPU B → display/compute
3. **MCP hub**: MCPJungle on UNRAID, agents connect to single endpoint
4. **Sandboxing**: code-on-incus or Docker Sandboxes per agent
5. **Thin client**: x86_64 kiosk image (Manjaro/Sway today, NixOS later), auto-start ralphglasses TUI
6. **Window management**: 7 workspaces → 7 monitors, go-i3 IPC
7. **Network**: Proxmox SDN VLAN + per-container iptables + MCPJungle as only tool endpoint
8. **Persistence**: UNRAID NFS shares, git worktrees per agent
