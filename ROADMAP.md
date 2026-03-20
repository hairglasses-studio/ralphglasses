# Ralphglasses Roadmap

Command-and-control TUI + bootable thin client for parallel Claude Code agent fleets.

## Core Deliverables

### Deliverable 1: `ralphglasses` Go Binary
Cross-platform Unix TUI (k9s-style) built with Charmbracelet (BubbleTea + Lip Gloss).
Manages multi-session Claude Code / ralph loops from any terminal.

### Deliverable 2: Bootable Linux Thin Client
Featherweight, low-graphics bootable Linux (Ubuntu 24.04-based) that boots into i3 + the ralphglasses TUI.
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

**Completed:**
- [x] Unit tests for all packages — 78.2% coverage (discovery, model, process, mcpserver)
- [x] TUI tests — 55.5% app coverage, view rendering, keymap, command/filter modes
- [x] CI pipeline — `go test`, `go vet`, `golangci-lint`, shellcheck, fuzz, benchmarks, BATS
- [x] Error handling — MCP scan error propagation, log stream errors, config key validation
- [x] Process manager — watcher timeout fix (no longer blocks event loop)
- [x] Config editor — key validation

**Remaining (24 subtasks):**

> **Parallel workstreams:** 1.1 and 1.2 can proceed concurrently. 1.3 and 1.5 can proceed concurrently. 1.4 depends on 1.1 fixtures. 1.6 depends on all others.

### 1.1 — Integration test: full lifecycle
- [ ] 1.1.1 — Create test fixture directory with `.ralph/` dir, mock `status.json`, and dummy `.ralphrc`
- [ ] 1.1.2 — Write mock `ralph_loop.sh` that simulates loop lifecycle (start, write status, exit)
- [ ] 1.1.3 — Implement lifecycle test: scan → start → poll status → stop, assert state transitions
- [ ] 1.1.4 — Add `//go:build integration` tag and CI gate (`go test -tags=integration`)
- **Acceptance:** `go test -tags=integration` passes end-to-end lifecycle

### 1.2 — MCP server hardening
- [ ] 1.2.1 — Audit all shared state in `mcpserver`; add `sync.RWMutex` around `repos` map and scan results
- [ ] 1.2.2 — Migrate all `log.Printf` calls to `slog` with structured fields (tool name, repo path, duration)
- [ ] 1.2.3 — Add request validation: reject empty repo paths, unknown config keys, malformed JSON
- [ ] 1.2.4 — Define MCP error codes (not-found, invalid-input, internal) and return structured errors
- **Acceptance:** no data races under `go test -race`, structured JSON log output

### 1.3 — TUI polish
- [ ] 1.3.1 — Build `ConfirmDialog` component (y/n prompt overlay, reusable across views)
- [ ] 1.3.2 — Wire confirm dialog to destructive actions: stop, stop_all, config delete
- [ ] 1.3.3 — Add SIGINT/SIGTERM shutdown handler: stop all managed processes, flush logs, clean exit
- [ ] 1.3.4 — Audit scroll bounds in log stream and table views; fix off-by-one on terminal resize
- **Acceptance:** destructive actions require y/n, clean exit on signals, no scroll panics on resize

### 1.4 — Process manager improvements
- [ ] 1.4.1 — Define PID file format (JSON: pid, start_time, repo_path) and write on process launch `[BLOCKED BY 1.1.1]`
- [ ] 1.4.2 — Implement orphan scanner: on startup, check PID files against running processes, clean stale entries
- [ ] 1.4.3 — Add restart policy to `.ralphrc` (`RESTART_ON_CRASH=true`, `MAX_RESTARTS=3`, `RESTART_DELAY_SEC=5`)
- [ ] 1.4.4 — Implement health check loop: poll process status every 5s, trigger restart or circuit breaker on repeated failures
- **Acceptance:** survives ralph crash with auto-restart, no orphan processes after TUI exit

### 1.5 — Config editor enhancements
- [ ] 1.5.1 — Add key CRUD operations: insert new key, rename key, delete key from TUI
- [ ] 1.5.2 — Wire fsnotify on `.ralphrc` file; reload config on external change, emit notification
- [ ] 1.5.3 — Add validation rules per key type (numeric ranges, boolean, enum values)
- [ ] 1.5.4 — Implement undo buffer (single-level: revert last edit)
- **Acceptance:** external edits reflected without restart, invalid values rejected with message

### 1.6 — Test coverage targets
- [ ] 1.6.1 — Set per-package coverage targets: discovery 90%, model 90%, process 85%, mcpserver 85%, tui 70%
- [ ] 1.6.2 — Add CI enforcement step: `go test -coverprofile` → parse → fail if below threshold
- [ ] 1.6.3 — Add coverage badge to README via codecov or go-cover-treemap
- [ ] 1.6.4 — Write missing tests to reach 85%+ overall (focus on untested error paths)
- **Acceptance:** `go test -coverprofile` meets thresholds in CI, badge visible in README

## Phase 2: Multi-Session Fleet Management

> **Depends on:** Phase 1 (concurrency guards, process manager improvements)
>
> **Parallel workstreams:** 2.1 (data model) is the foundation — most items depend on it. 2.6 (notifications) and 2.7 (tmux) are independent of each other and can proceed after 2.1. 2.9 (CLI) is independent of TUI work. 2.10 (marathon port) is fully independent.

### 2.1 — Session data model
- [ ] 2.1.1 — Define `Session` struct: ID, repo path, worktree path, PID, budget, model, status, created_at, updated_at
- [ ] 2.1.2 — Add SQLite via `modernc.org/sqlite`: schema migrations, connection pool, WAL mode
- [ ] 2.1.3 — Implement Session CRUD: Create, Get, List, Update, Delete with prepared statements
- [ ] 2.1.4 — Implement lifecycle state machine: `created → running → paused → stopped → archived` with valid transition enforcement
- [ ] 2.1.5 — Add session event log table: state changes, errors, budget events with timestamps
- **Acceptance:** sessions survive TUI restart, queryable via SQL

### 2.2 — Git worktree orchestration `[BLOCKED BY 2.1]`
- [ ] 2.2.1 — Create `internal/worktree/` package: wrapping `git worktree add/list/remove`
- [ ] 2.2.2 — Auto-create worktree on session launch: branch naming convention `ralph/<session-id>`
- [ ] 2.2.3 — Implement merge-back: `git merge --no-ff` with conflict detection and abort-on-conflict option
- [ ] 2.2.4 — Add worktree cleanup on session stop/archive (remove worktree dir, prune)
- [ ] 2.2.5 — Handle edge cases: dirty worktree on stop, orphaned branches, worktree path conflicts
- **Acceptance:** `ralphglasses worktree create <repo>` produces isolated worktree, merge-back detects conflicts

### 2.3 — Budget tracking `[BLOCKED BY 2.1]`
- [ ] 2.3.1 — Per-session spend poller: read `session_spend_usd` from `.ralph/status.json` on watcher tick
- [ ] 2.3.2 — Implement global budget pool: total ceiling, per-session allocation, remaining calculation
- [ ] 2.3.3 — Add threshold alerts at 50%, 75%, 90% — emit BubbleTea message for TUI notification
- [ ] 2.3.4 — Auto-pause session at budget ceiling: send SIGSTOP, update session state
- [ ] 2.3.5 — Port budget tracking patterns from `mcpkit/finops` (cost ledger, rate calculation)
- **Acceptance:** session auto-pauses when budget exhausted, alerts visible in TUI

### 2.4 — Fleet dashboard TUI view `[BLOCKED BY 2.1]`
- [ ] 2.4.1 — Create `ViewFleet` in view stack with aggregate session table
- [ ] 2.4.2 — Columns: session name, repo, status, spend, loop count, model, uptime — sortable
- [ ] 2.4.3 — Live-update via watcher ticks: refresh spend/status/loop count per row
- [ ] 2.4.4 — Inline actions from fleet view: start/stop/pause selected session via keybinds
- [ ] 2.4.5 — Add fleet summary bar: total sessions, running count, total spend, aggregate throughput
- **Acceptance:** fleet view shows all sessions with live-updating spend/status

### 2.5 — Session launcher `[BLOCKED BY 2.1, 2.2, 2.3]`
- [ ] 2.5.1 — Implement `:launch` command: pick repo from discovered list, set session name
- [ ] 2.5.2 — Add budget/model selection to launch flow: dropdown or tab-complete for model, numeric input for budget
- [ ] 2.5.3 — Default budget from `.ralphrc` (`RALPH_SESSION_BUDGET`) or global config fallback
- [ ] 2.5.4 — Session templates: save current launch config as named template, load from template
- [ ] 2.5.5 — Validate launch preconditions: repo exists, no conflicting worktree, budget available in pool
- **Acceptance:** can launch a named session with budget from TUI command mode

### 2.6 — Notification system `[PARALLEL — independent after 2.1]`
- [ ] 2.6.1 — Desktop notification abstraction: `freedesktop.org` D-Bus (Linux), `osascript` (macOS)
- [ ] 2.6.2 — Define event types: session_complete, budget_warning, circuit_breaker_trip, crash, restart
- [ ] 2.6.3 — Add `.ralphrc` config keys: `NOTIFY_DESKTOP=true`, `NOTIFY_SOUND=true`
- [ ] 2.6.4 — Implement notification dedup/throttle: no repeat within 60s for same event type + session
- **Acceptance:** desktop notification fires on circuit breaker trip

### 2.7 — tmux integration `[PARALLEL — independent after 2.1]`
- [ ] 2.7.1 — `internal/tmux/` package: create/list/kill sessions, name windows, attach/detach
- [ ] 2.7.2 — One tmux pane per agent session: auto-create on session launch, name = session ID
- [ ] 2.7.3 — `ralphglasses tmux` subcommand: `list`, `attach <session>`, `detach`
- [ ] 2.7.4 — Headless mode: detect no TTY → auto-use tmux instead of TUI
- [ ] 2.7.5 — Port patterns from claude-tools (WSL-native tmux management, `/mnt/c/` path handling)
- **Acceptance:** `ralphglasses tmux list` shows active sessions, `attach` works

### 2.8 — MCP server expansion `[BLOCKED BY 2.1, 2.2, 2.3]`
- [ ] 2.8.1 — Add `ralphglasses_session_create` tool: accepts repo, budget, model, name
- [ ] 2.8.2 — Add `ralphglasses_session_list` tool: returns all sessions with status
- [ ] 2.8.3 — Add `ralphglasses_worktree_create` tool: create worktree for repo
- [ ] 2.8.4 — Add `ralphglasses_budget_status` tool: per-session and global budget info
- [ ] 2.8.5 — Add `ralphglasses_fleet_summary` tool: aggregate stats for agent-to-agent coordination
- **Acceptance:** MCP tools callable from Claude Code, session lifecycle works end-to-end

### 2.9 — CLI subcommands *(new)*
- [ ] 2.9.1 — `ralphglasses session list|start|stop|status` — non-TUI session management
- [ ] 2.9.2 — `ralphglasses worktree create|list|merge|clean` — worktree operations from CLI
- [ ] 2.9.3 — `ralphglasses budget status|set|reset` — budget management from CLI
- [ ] 2.9.4 — JSON output flag (`--json`) for all subcommands for scripting/piping
- **Acceptance:** all fleet operations available without TUI, JSON output parseable by `jq`

### 2.10 — Marathon.sh Go port *(new)* `[PARALLEL — fully independent]`
- [ ] 2.10.1 — Port `marathon.sh` to `internal/marathon/` package: duration limit, budget limit, checkpoints
- [ ] 2.10.2 — `ralphglasses marathon` subcommand: `--budget`, `--duration`, `--checkpoint-interval`
- [ ] 2.10.3 — Replace shell signal handling with Go `os/signal` (SIGINT/SIGTERM → graceful shutdown)
- [ ] 2.10.4 — Git checkpoint tagging in Go: `git tag marathon-<timestamp>` at configurable interval
- [ ] 2.10.5 — Structured marathon logging via `slog` (replace bash `log()` function)
- **Acceptance:** `ralphglasses marathon` replaces `bash marathon.sh` with identical behavior

## Phase 3: i3 Multi-Monitor Integration

> **Depends on:** Phase 2 (session model, fleet dashboard)
>
> **Parallel workstreams:** 3.1 (i3 IPC) is the foundation. 3.4 (autorandr) is independent. 3.5 (Sway) can proceed in parallel with 3.2. 3.3 depends on 3.1 + 2.1 (SQLite).

### 3.1 — i3 IPC client
- [ ] 3.1.1 — Create `internal/i3/` package wrapping go-i3: connect to i3 socket, subscribe to events
- [ ] 3.1.2 — Workspace CRUD: create named workspace, move to output, rename, close
- [ ] 3.1.3 — Window management: focus, move-to-workspace, set layout (splitv/splith/tabbed/stacked)
- [ ] 3.1.4 — Monitor enumeration: list outputs via i3 IPC (name, resolution, position)
- [ ] 3.1.5 — Event listener: workspace focus, window create/close, output connect/disconnect
- **Acceptance:** programmatic workspace creation and window placement from Go

### 3.2 — Monitor layout manager `[BLOCKED BY 3.1]`
- [ ] 3.2.1 — Define layout presets as JSON: "dev" (agents + logs), "fleet" (all sessions), "focused" (single agent)
- [ ] 3.2.2 — 7-monitor workspace assignment config (`distro/i3/workspaces.json`) — maps output names to workspace numbers
- [ ] 3.2.3 — TUI command `:layout <name>` — apply preset by moving windows/workspaces to designated outputs
- [ ] 3.2.4 — Save current layout as custom preset (`:layout save <name>`)
- [ ] 3.2.5 — Handle missing monitors gracefully: skip unavailable outputs, log warning, fall back to available
- **Acceptance:** `:layout fleet` redistributes windows across monitors

### 3.3 — Multi-instance coordination `[BLOCKED BY 3.1, 2.1]`
- [ ] 3.3.1 — Shared state via SQLite: same DB file, WAL mode, `PRAGMA busy_timeout`
- [ ] 3.3.2 — Instance discovery: Unix domain socket per instance, advertise PID and capabilities
- [ ] 3.3.3 — Leader election: simple file-lock based leader for fleet operations (stop_all, budget enforcement)
- [ ] 3.3.4 — Leader failover: detect leader crash via heartbeat, re-elect
- **Acceptance:** two ralphglasses instances share session state without corruption

### 3.4 — autorandr integration `[PARALLEL — independent]`
- [ ] 3.4.1 — Detect monitor connects/disconnects via i3 output events or udev
- [ ] 3.4.2 — Auto-apply saved autorandr profiles on hotplug
- [ ] 3.4.3 — Generate autorandr profiles from current xrandr state via TUI command (`:autorandr save <name>`)
- [ ] 3.4.4 — Link autorandr profiles to layout presets: hotplug → apply profile → apply layout
- **Acceptance:** monitor hot-plug triggers layout restore

### 3.5 — Sway/Wayland compatibility *(new)* `[PARALLEL — independent of 3.2]`
- [ ] 3.5.1 — Abstract WM interface: `internal/wm/` with i3 and Sway backends (i3 IPC vs sway IPC)
- [ ] 3.5.2 — Sway IPC client: workspace/window/output management via Sway's i3-compatible protocol
- [ ] 3.5.3 — Auto-detect WM at startup: check `$SWAYSOCK` vs `$I3SOCK`, select backend
- [ ] 3.5.4 — Test suite: integration tests for both backends (mock IPC socket)
- **Acceptance:** layout commands work on both i3 (X11) and Sway (Wayland)

## Phase 4: Bootable Thin Client

> **Depends on:** Phase 3 (i3 integration, monitor layout)
>
> **Parallel workstreams:** 4.1 (ISO pipeline) is the foundation. 4.3 (PXE) and 4.6 (OTA) can proceed in parallel. 4.7 (watchdog) is independent. 4.5 (install-to-disk) depends on 4.1.

### 4.1 — Dockerfile → ISO pipeline
**Completed:**
- [x] `distro/Dockerfile` — Ubuntu 24.04, kernel 6.12+ HWE, NVIDIA 550, i3, Go, Claude Code
- [x] `distro/scripts/hw-detect.sh` — GPU detection, GTX 1060 blacklisting, MT7927 BT blacklisting
- [x] `distro/systemd/hw-detect.service` — Oneshot first-boot hardware detection
- [x] `distro/systemd/ralphglasses.service` — TUI autostart after graphical target

**Remaining:**
- [ ] 4.1.1 — `distro/Makefile` target `build`: `docker build` with build args for kernel version and NVIDIA driver
- [ ] 4.1.2 — `distro/Makefile` target `squashfs`: extract rootfs from container, `mksquashfs` with xz compression
- [ ] 4.1.3 — `distro/Makefile` target `iso`: `grub-mkrescue` with EFI + BIOS support
- [ ] 4.1.4 — QEMU smoke test script: boot ISO, verify TUI starts, check GPU detection output
- [ ] 4.1.5 — CI integration: build ISO in GitHub Actions (no GPU, skip NVIDIA tests), upload as artifact
- **Acceptance:** `make iso` produces bootable image, boots in QEMU

### 4.2 — i3 kiosk configuration `[BLOCKED BY 4.1]`
- [ ] 4.2.1 — `distro/i3/config` — workspace-to-output mapping for 7 monitors (RTX 4090 outputs)
- [ ] 4.2.2 — Strip WM chrome: no title bars (`default_border none`), no desktop, no dmenu
- [ ] 4.2.3 — Keybindings: workspace navigation ($mod+1-7), TUI focus, emergency shell ($mod+Shift+Return)
- [ ] 4.2.4 — Auto-start: launch ralphglasses fullscreen on workspace 1 via `exec` directive
- [ ] 4.2.5 — Lock screen: disable screen blanking, DPMS off (24/7 marathon operation)
- **Acceptance:** boots to fullscreen TUI, no visible WM chrome

### 4.3 — PXE/network boot `[PARALLEL — independent after 4.1]`
- [ ] 4.3.1 — iPXE chainload config: DHCP → iPXE → tftp/http boot menu
- [ ] 4.3.2 — LTSP server setup on UNRAID: serve squashfs over NFS/NBD
- [ ] 4.3.3 — Network boot squashfs overlay: persistent `/home` and `/etc/ralphglasses` via NFS
- [ ] 4.3.4 — Fallback: USB boot with local squashfs + overlay partition
- [ ] 4.3.5 — Boot menu: select version (latest, rollback) via iPXE script
- **Acceptance:** PXE boot from UNRAID reaches ralphglasses TUI

### 4.4 — Hardware profiles
- [x] ProArt X870E-CREATOR WIFI — primary target (documented in `distro/hardware/proart-x870e.md`)
- [ ] 4.4.1 — Generalize `hw-detect.sh`: PCI ID table with per-device actions (load module, blacklist, configure)
- [ ] 4.4.2 — Add hardware profile schema: JSON manifest with PCI IDs, required modules, known issues
- [ ] 4.4.3 — Validate profiles against running system: flag mismatches between manifest and detected hardware
- [ ] 4.4.4 — Template for adding new boards: `distro/hardware/TEMPLATE.md` with required fields
- **Acceptance:** hw-detect.sh correctly identifies and configures target hardware via profile lookup

### 4.5 — Install-to-disk `[BLOCKED BY 4.1]`
- [ ] 4.5.1 — `distro/scripts/install-to-disk.sh`: partition scheme (512MB ESP + ext4 rootfs), auto-detect target disk
- [ ] 4.5.2 — GRUB install: UEFI mode, `grub-install` + `update-grub` with kernel cmdline for NVIDIA
- [ ] 4.5.3 — First-boot setup: run hw-detect.sh, generate i3 config, set hostname, configure network
- [ ] 4.5.4 — ZFS root option: `zpool create` with mirror, boot partition on ext4 (ZFS can't be ESP)
- [ ] 4.5.5 — Safety: require `--confirm` flag, show disk info before wiping, never auto-select boot disk
- **Acceptance:** install-to-disk produces bootable system on NVMe

### 4.6 — OTA update mechanism *(new)* `[PARALLEL — independent]`
- [ ] 4.6.1 — Version check: compare local squashfs hash against remote manifest (S3/GitHub Release)
- [ ] 4.6.2 — Download + verify: fetch new squashfs, SHA256 checksum, GPG signature
- [ ] 4.6.3 — Atomic swap: A/B partition scheme or overlay — boot into new version, rollback on failure
- [ ] 4.6.4 — `ralphglasses update` subcommand: check, download, apply, reboot
- **Acceptance:** OTA update replaces running image, rollback works on boot failure

### 4.7 — Health watchdog service *(new)* `[PARALLEL — independent]`
- [ ] 4.7.1 — Systemd watchdog unit: monitor ralphglasses process, restart on crash
- [ ] 4.7.2 — Hardware health checks: GPU temperature, disk space, memory pressure, network connectivity
- [ ] 4.7.3 — Alert escalation: local notification → log → optional webhook on persistent failure
- [ ] 4.7.4 — Heartbeat file: write timestamp to `/run/ralphglasses/heartbeat`, stale = restart
- **Acceptance:** TUI auto-restarts within 10s of crash, hardware alerts visible in TUI

## Phase 5: Agent Sandboxing & Infrastructure

> **Depends on:** Phase 2 (session model needed for container lifecycle)
>
> **Parallel workstreams:** 5.1 (Docker) and 5.2 (Incus) are parallel sandboxing approaches. 5.3 (MCP gateway) is independent. 5.4 (network) depends on 5.1 or 5.2. 5.6 (secrets) is independent.

### 5.1 — Docker sandbox mode
- [ ] 5.1.1 — `internal/sandbox/docker/` package: build/pull image, create container, manage lifecycle
- [ ] 5.1.2 — Container spec: bind-mount workspace, set `--cpus`, `--memory`, `--network` flags from session config
- [ ] 5.1.3 — Lifecycle binding: session start → container start, session stop → container stop + remove
- [ ] 5.1.4 — Log forwarding: capture container stdout/stderr → session log stream
- [ ] 5.1.5 — GPU passthrough: `--gpus` flag for NVIDIA containers (Claude Code doesn't need GPU, but future models might)
- **Acceptance:** session runs inside container, cleanup on stop

### 5.2 — Incus/LXD containers
- [ ] 5.2.1 — `internal/sandbox/incus/` package: Incus client, profile management, instance lifecycle
- [ ] 5.2.2 — Per-container credential isolation: mount secrets as files, no env var leakage
- [ ] 5.2.3 — Workspace persistence: bind-mount project dir, snapshot on session stop
- [ ] 5.2.4 — Threat detection: monitor for suspicious file access, network connections, resource spikes
- [ ] 5.2.5 — Port patterns from code-on-incus: Go-based container management, security profiles
- **Acceptance:** session runs in Incus container with isolated credentials

### 5.3 — MCP gateway `[PARALLEL — independent]`
- [ ] 5.3.1 — Central MCP hub service: accept connections from multiple agents, route to backend tools
- [ ] 5.3.2 — Per-session tool authorization: allowlist of tools per session, deny by default
- [ ] 5.3.3 — Audit logging: every tool call logged with session ID, tool name, args, result, duration
- [ ] 5.3.4 — Rate limiting: per-session and global rate limits on tool calls
- [ ] 5.3.5 — Deploy to UNRAID: systemd service, auto-start, log rotation
- **Acceptance:** agent tool calls routed through gateway with audit trail

### 5.4 — Network isolation `[BLOCKED BY 5.1 or 5.2]`
- [ ] 5.4.1 — VLAN segmentation: assign each sandbox to isolated VLAN via bridge/macvlan
- [ ] 5.4.2 — iptables/nftables allowlists: per-session rules (allow API endpoints, deny everything else)
- [ ] 5.4.3 — DNS sinkholing: local DNS resolver, block unauthorized domains per session policy
- [ ] 5.4.4 — Network policy config in `.ralphrc`: `SANDBOX_ALLOWED_DOMAINS`, `SANDBOX_NETWORK_MODE`
- **Acceptance:** sandboxed session cannot reach unauthorized endpoints

### 5.5 — Budget federation `[BLOCKED BY 2.3]`
- [ ] 5.5.1 — Global budget pool: total ceiling across all sessions, stored in SQLite
- [ ] 5.5.2 — Per-session limits with carry-over: unused budget redistributed to active sessions
- [ ] 5.5.3 — Budget dashboard view: spend rate ($/hr), projection to exhaustion, per-session breakdown
- [ ] 5.5.4 — Anthropic billing API integration: reconcile local tracking with actual billing (when API available)
- [ ] 5.5.5 — Budget alerts: global pool threshold warnings, session overspend detection
- **Acceptance:** global pool enforced across all active sessions

### 5.6 — Secret management *(new)* `[PARALLEL — independent]`
- [ ] 5.6.1 — Secret provider interface: `internal/secrets/` with pluggable backends
- [ ] 5.6.2 — SOPS backend: decrypt `.sops.yaml` encrypted files, inject as env vars into sessions
- [ ] 5.6.3 — Vault backend: HashiCorp Vault KV v2, lease management, auto-renew
- [ ] 5.6.4 — Secret rotation: detect expiry, refresh credentials, restart affected sessions
- [ ] 5.6.5 — Audit: log secret access (not values) per session
- **Acceptance:** API keys loaded from Vault/SOPS, never stored in plaintext config

## Phase 6: Advanced Fleet Intelligence

> **Depends on:** Phase 2 (sessions) + Phase 5 (sandboxing)
>
> **Parallel workstreams:** 6.1 (native loop) and 6.6 (model routing) can proceed in parallel. 6.3 (coordination) depends on 6.1. 6.4 (analytics) and 6.5 (notifications) are independent. 6.7 (replay) depends on 6.4.

### 6.1 — Native ralph loop engine
- [ ] 6.1.1 — Embed `mcpkit/ralph` as Go dependency: import DAG executor, task specs, progress tracking
- [ ] 6.1.2 — Typed task specs: define task schema (inputs, outputs, dependencies) as Go structs
- [ ] 6.1.3 — DAG visualization in TUI: show task graph with status (pending/running/complete/failed)
- [ ] 6.1.4 — Parallel execution: run independent tasks concurrently, respect dependency edges
- [ ] 6.1.5 — Progress telemetry: structured events (task_start, task_complete, task_error) to session event log
- **Acceptance:** ralph loop runs natively in Go, DAG visible in TUI

### 6.2 — R&D cycle orchestrator `[BLOCKED BY 6.1]`
- [ ] 6.2.1 — Port perpetual improvement loop from claudekit rdcycle: benchmark → analyze → generate tasks → execute
- [ ] 6.2.2 — Self-benchmark: measure test coverage, lint score, build time, binary size per iteration
- [ ] 6.2.3 — Regression detection: compare benchmarks across iterations, flag regressions above threshold
- [ ] 6.2.4 — Auto-generate improvement tasks: create ralph task specs from benchmark regressions
- [ ] 6.2.5 — Cycle dashboard: iteration history, benchmark trends, task throughput over time
- **Acceptance:** automated benchmark → task generation cycle runs unattended

### 6.3 — Cross-session coordination `[BLOCKED BY 6.1, 2.1]`
- [ ] 6.3.1 — Shared context store: SQLite table of current tasks per session (file, feature, intent)
- [ ] 6.3.2 — Dedup engine: before task assignment, check if another session is working on same file/feature
- [ ] 6.3.3 — Dependency ordering: agent B subscribes to agent A's output, waits for completion event
- [ ] 6.3.4 — Conflict resolution: detect concurrent edits to same file, pause later session, notify
- [ ] 6.3.5 — Coordination dashboard: TUI view showing task assignments across sessions, conflicts, blockers
- **Acceptance:** two agents targeting same repo don't conflict on same files

### 6.4 — Analytics & observability `[PARALLEL — independent]`
- [ ] 6.4.1 — Historical data model: store session metrics (cost, duration, tasks, model) in SQLite
- [ ] 6.4.2 — TUI analytics view: cost per session, throughput, completion rates, time-series charts
- [ ] 6.4.3 — OpenTelemetry traces: port from `mcpkit/observability`, span per task execution
- [ ] 6.4.4 — Prometheus metrics endpoint: `/metrics` HTTP handler with session gauges and counters
- [ ] 6.4.5 — Grafana dashboard JSON: pre-built dashboard for session metrics (import into Grafana)
- **Acceptance:** Grafana dashboard shows session metrics over time

### 6.5 — External notifications `[PARALLEL — independent]`
- [ ] 6.5.1 — Webhook dispatcher: HTTP POST to configured URLs on events
- [ ] 6.5.2 — Discord integration: format events as Discord embeds, send via webhook URL
- [ ] 6.5.3 — Slack integration: format events as Slack blocks, send via webhook URL
- [ ] 6.5.4 — Notification templates: customizable message format per event type
- [ ] 6.5.5 — Rate limiting and retry: deduplicate within window, retry with backoff on failure
- **Acceptance:** Discord webhook fires on session completion

### 6.6 — Model routing *(new)* `[PARALLEL — independent]`
- [ ] 6.6.1 — Model registry: define available models with capabilities, cost/token, context window
- [ ] 6.6.2 — Task-type classifier: map task types (code, review, test, docs) to preferred models
- [ ] 6.6.3 — Routing rules in `.ralphrc`: `MODEL_ROUTE_CODE=opus`, `MODEL_ROUTE_REVIEW=sonnet`
- [ ] 6.6.4 — Dynamic routing: switch model mid-session based on task type (requires native loop engine)
- [ ] 6.6.5 — Cost optimization: suggest cheaper model when task complexity is below threshold
- **Acceptance:** different task types route to appropriate models, visible in session status

### 6.7 — Replay/audit trail *(new)* `[BLOCKED BY 6.4]`
- [ ] 6.7.1 — Session recording: capture all tool calls, LLM responses, state transitions with timestamps
- [ ] 6.7.2 — Replay viewer: TUI view that steps through session history (forward/backward/seek)
- [ ] 6.7.3 — Export: generate session report as Markdown or JSON (cost, tasks, duration, outcomes)
- [ ] 6.7.4 — Diff view: compare two session replays side-by-side (useful for A/B model testing)
- [ ] 6.7.5 — Retention policy: auto-archive sessions older than N days, configurable in `.ralphrc`
- **Acceptance:** can replay a completed session step-by-step, export as Markdown report

---

## Dependency Chain

```
Phase 1 (Harden) ──→ Phase 2 (Multi-Session) ──→ Phase 3 (i3)
                                                       │
Phase 4 (Thin Client) ←───────────────────────────────┘
                                                       │
Phase 5 (Sandboxing) ←── Phase 2 (sessions needed)    │
                              │                        │
Phase 6 (Fleet Intel) ←── Phase 2 + Phase 5 ──────────┘
```

### Item-Level Dependencies
```
1.1 ──→ 1.4 (fixtures needed for PID file tests)
1.* ──→ 1.6 (coverage targets depend on all other Phase 1 work)

2.1 ──→ 2.2, 2.3, 2.4, 2.5, 2.8 (session model is foundation)
2.1 + 2.2 + 2.3 ──→ 2.5 (launcher needs worktrees + budget)
2.3 ──→ 5.5 (budget federation extends per-session tracking)

3.1 ──→ 3.2, 3.3 (i3 IPC client needed for layout + coordination)
2.1 + 3.1 ──→ 3.3 (multi-instance needs SQLite + i3)

4.1 ──→ 4.2, 4.5 (ISO pipeline needed before kiosk config + install)
5.1 or 5.2 ──→ 5.4 (network isolation needs a sandbox runtime)

6.1 ──→ 6.2, 6.3 (native loop engine needed for orchestrator + coordination)
6.4 ──→ 6.7 (analytics infrastructure needed for replay)
```

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
| **Ubuntu 24.04 HWE** | ~2GB | Current choice. NVIDIA 550 via apt, kernel 6.12+ |
| **DietPi** | ~130MB | Debian, i3 in catalog, thin client proven (legacy option) |
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
