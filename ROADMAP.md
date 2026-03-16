# Ralphglasses Roadmap

Command-and-control TUI + bootable thin client for parallel Claude Code agent fleets.

## Core Deliverables

### Deliverable 1: `ralphglasses` Go Binary
Cross-platform Unix TUI (k9s-style) built with Charmbracelet (BubbleTea + Lip Gloss).
Manages multi-session Claude Code / ralph loops from any terminal.

### Deliverable 2: Bootable Linux Thin Client
Featherweight, low-graphics bootable Linux (DietPi-based) that boots into i3 + the ralphglasses TUI.
Supports multi-monitor (7-display, dual-NVIDIA-GPU) and autoboot/cron operation.

---

## Phase 0: Foundation (COMPLETE)

- [x] Go module (`github.com/hairglasses-studio/ralphglasses`)
- [x] Cobra CLI with `--scan-path` flag
- [x] Discovery engine — scan for `.ralph/` and `.ralphrc` repos
- [x] Model layer — parsers for status.json, progress.json, circuit breaker, .ralphrc
- [x] Process manager — launch/stop/pause ralph loops via os/exec with process groups
- [x] File watcher — fsnotify with 2s polling fallback
- [x] Log streamer — tail `.ralph/live.log`
- [x] MCP server — 9 tools (scan, list, status, start, stop, stop_all, pause, logs, config)
- [x] Standalone MCP binary (`cmd/ralphglasses-mcp/`)
- [x] TUI shell — BubbleTea app with k9s-style keymap
- [x] TUI views — overview table, repo detail, log stream, config editor, help
- [x] TUI components — sortable table, breadcrumb, status bar, notifications
- [x] Styles package — Lip Gloss theme (isolated to avoid import cycles)
- [x] Marathon launcher script (`marathon.sh`)

## Phase 1: Harden & Test

- [ ] Unit tests for all packages (discovery, model, process, mcpserver)
- [ ] Integration test: scan → start → status → stop lifecycle
- [ ] TUI tests: view rendering, keymap, command mode
- [ ] MCP server hardening: circuit reset tool, concurrency guards, structured logging
- [ ] TUI polish: confirm dialogs for destructive actions, graceful shutdown, scroll bounds
- [ ] Process manager: PID file detection, orphan cleanup, auto-restart on crash
- [ ] Config editor: add/delete keys, validation, reload on external change
- [ ] Error handling: structured errors, user-facing messages in TUI
- [ ] CI pipeline: `go test`, `go vet`, `golangci-lint`, build matrix (linux/amd64, darwin/arm64)

## Phase 2: Multi-Session Fleet Management

- [ ] **Git worktree orchestration** — create/list/merge worktrees per agent session
  - Port patterns from [craigsc/cmux](https://github.com/craigsc/cmux) (pure bash, worktree-per-agent)
  - Port patterns from [cc-workflow](https://github.com/jrimmer/cc-workflow) (tmux + worktrees)
- [ ] **Session manager** — named sessions with independent Claude Code processes
  - Budget tracking per session (port from mcpkit/finops)
  - Session state persistence (SQLite via modernc.org/sqlite, pattern from shielddd)
- [ ] **Fleet dashboard view** — aggregate costs, active sessions, circuit breaker states
- [ ] **TUI session launcher** — interactive wizard: pick repo, set budget, choose model, launch
- [ ] **Notification system** — desktop notifications when agent needs attention or completes
- [ ] **tmux integration** — optional tmux session management for headless operation
  - Port patterns from [claude-tools](https://github.com/oreoriorosu/claude-tools) (WSL-native tmux management)

## Phase 3: i3 Multi-Monitor Integration

- [ ] **i3 IPC** via [go-i3](https://github.com/i3/go-i3) library
  - Programmatic workspace control
  - Assign TUI views to specific monitors
  - Split/layout management
- [ ] **Monitor layout manager** — configure which view goes on which monitor
  - Presets: "dev" (agents + logs), "fleet" (all sessions), "focused" (1 agent fullscreen)
  - 7-monitor workspace assignment
- [ ] **Multi-instance mode** — ralphglasses instances on different monitors sharing state
- [ ] **autorandr integration** — detect and apply monitor layout profiles

## Phase 4: Bootable Thin Client

- [ ] **DietPi base image** (`distro/dietpi/`)
  - `dietpi.txt` automation config (headless install)
  - Software list: i3, alacritty, autorandr, ralphglasses binary
  - Auto-login to i3, auto-start ralphglasses TUI
- [ ] **i3 configuration** (`distro/i3/`)
  - 7-monitor workspace assignment (dual NVIDIA GPU)
  - Kiosk mode: TUI fullscreen, minimal WM chrome
  - Keybindings for workspace navigation + TUI control
- [ ] **PXE boot** (`distro/pxe/`)
  - LTSP/ThinStation config to serve thin client from UNRAID
  - Network boot = no local storage, centralized management
- [ ] **autorandr profiles** (`distro/autorandr/`)
  - Persist 7-monitor xrandr layout across reboots
  - Hot-plug detection
- [ ] **systemd units** (`distro/systemd/`)
  - Auto-login service
  - ralphglasses auto-start
  - Health watchdog
- [ ] **GPU configuration**
  - NVIDIA driver setup for dual-GPU
  - GPU A → display output (7 monitors)
  - GPU B → available for compute/passthrough
- [ ] **Build system** — script to produce bootable USB/ISO from DietPi + customizations

## Phase 5: Agent Sandboxing & Infrastructure

- [ ] **Docker Sandbox integration** — launch Claude Code in Docker Sandbox (official template)
- [ ] **code-on-incus patterns** — credential isolation, workspace persistence, threat detection
  - Port from [code-on-incus](https://github.com/mensfeld/code-on-incus) (Go-based)
- [ ] **MCPJungle gateway** — central MCP hub on UNRAID for all agent tool access
- [ ] **Network isolation** — VLAN segmentation, iptables allowlists per sandbox
- [ ] **NixOS microVM option** — Stapelberg-style microvm.nix for strongest isolation
- [ ] **Budget federation** — global budget pool across all sessions with per-session limits

## Phase 6: Advanced Fleet Intelligence

- [ ] **Ralph loop engine integration** — embed mcpkit/ralph directly for native DAG execution
- [ ] **R&D cycle orchestrator** — perpetual loop with self-improvement cadence (from claudekit)
- [ ] **Cross-session coordination** — agents aware of each other's work, avoid conflicts
- [ ] **Analytics dashboard** — historical cost, throughput, completion rates
- [ ] **Webhook/Discord notifications** — alert on budget exhaustion, circuit breaker trips, completions

---

## External Projects of Interest

### Multi-Session Claude Code Managers
| Project | Type | Platform | Key Feature |
|---------|------|----------|-------------|
| [craigsc/cmux](https://github.com/craigsc/cmux) | Bash | Linux/macOS/WSL | Git worktree per agent, pure bash |
| [cc-workflow](https://github.com/jrimmer/cc-workflow) | Bash | Linux/macOS | tmux + worktrees, Ubuntu provision script |
| [claude-tools](https://github.com/oreoriorosu/claude-tools) | Bash | WSL-native | tmux session management, `/mnt/c/` defaults |
| [zenportal](https://github.com/kgang/zenportal) | Python TUI | Linux | Multi-AI-CLI (Claude, Codex, Gemini) |
| [cc-hub](https://github.com/m0a/cc-hub) | Web UI | Linux | Linux x64 binary, multi-pane terminals |
| [Hive](https://github.com/latagore/hive) | Web UI | Linux | Mobile-first fleet dashboard, task queue |
| [code-on-incus](https://github.com/mensfeld/code-on-incus) | Go CLI | Linux | Incus containers, threat detection |
| [agent-runner](https://github.com/zsyu9779/agent-runner) | Go CLI | Linux/macOS | Stateful sessions, auto-commits |
| [claude-multi.nvim](https://github.com/mb6611/claude-multi.nvim) | Neovim | Any | Multi-session in Neovim |

### Agent OS & Sandboxing
| Project | Type | Maturity | Notes |
|---------|------|----------|-------|
| [StereOS](https://github.com/papercomputeco/stereOS) | NixOS agent OS | Alpha | gVisor sandboxing, produces VM images |
| Docker Sandboxes | Official | Production | Claude Code template, microVM isolation |
| [microvm.nix](https://michael.stapelberg.ch/posts/2026-02-01-coding-agent-microvm-nix/) | NixOS pattern | Documented | Stapelberg's microVM guide |
| [kubernetes-sigs/agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox) | K8s CRD | v0.1.0 | gVisor + Kata, WarmPools |
| [alibaba/OpenSandbox](https://github.com/alibaba/OpenSandbox) | Multi-runtime | Production | Firecracker, gVisor, Kata |
| [E2B](https://e2b.dev/) | Firecracker SaaS | Production | <200ms sandbox boot |
| [Daytona](https://github.com/daytonaio/daytona) | Docker SaaS | Production | <90ms startup, state management |

### Container OS (for hosting agent workloads)
| OS | Recommendation | Notes |
|----|---------------|-------|
| **NixOS** | Top pick | microvm.nix, llm-agents.nix, claude-code-nix |
| **Fedora CoreOS** | Runner-up | Podman Quadlet systemd, no K8s required |
| **Kairos** | Build-your-own | Dockerfile → bootable ISO |
| **Talos Linux** | K8s-only | API-only, NVIDIA extensions |

### Thin Client Base
| Distro | Size | Notes |
|--------|------|-------|
| **DietPi** | ~130MB | Recommended. Debian, i3 in catalog, thin client proven |
| Tiny Core Linux | 16-21MB | Ultra-minimal, runs in RAM |
| ThinStation | ~50MB | PXE-native, RDP/VNC/SSH |

---

## Internal Ecosystem Integration

### From mcpkit (Go packages to port/embed)
- `mcpkit/ralph/` — Ralph Loop engine (DAG, specs, progress, cost tracking)
- `mcpkit/finops/` — FinOps cost tracking, budget management
- `mcpkit/sampling/` — LLM sampling client
- `mcpkit/registry/` — Tool registry, typed handlers
- `mcpkit/resilience/` — Circuit breakers, retries
- `mcpkit/observability/` — OpenTelemetry + Prometheus
- `mcpkit/orchestrator/` — Multi-agent orchestration

### From shell scripts (port to Go)
- `ralphglasses/marathon.sh` — 12h marathon launcher
- `hg-mcp/.ralph/start_session.sh` — Session launcher with budget reset
- `mesmer/.ralph/start-12hr.sh` — Pre-flight checks, budget projection
- `claudekit/scripts/perpetual-loop.sh` — Perpetual R&D cycle

### From Go MCP servers (reuse patterns)
- `hg-mcp/` — Modular tool registration pattern
- `shielddd/` — Pure-Go SQLite (modernc.org/sqlite), audit logs
- `claudekit/` — rdcycle perpetual loop, budget profiles
