# Ralphglasses

Command-and-control TUI + bootable thin client for parallel Claude Code agent fleets.

## Two Deliverables

1. **`ralphglasses` Go binary** — Cross-platform Unix TUI (k9s-style, Charmbracelet). Manages multi-session Claude Code / ralph loops.
2. **Bootable Linux thin client** — DietPi + i3, boots into ralphglasses TUI. 7-monitor, dual-NVIDIA-GPU.

See ROADMAP.md for full plan. See docs/ for research.

## Build & Run

```bash
go build ./...
go run . --scan-path ~/hairglasses-studio
```

## MCP Server

Ralphglasses is also an installable MCP server exposing 9 tools for managing ralph loops programmatically.

### Install

```bash
# Via claude CLI (recommended)
claude mcp add ralphglasses -- go run ./cmd/ralphglasses-mcp

# Or with custom scan path
claude mcp add ralphglasses -e RALPHGLASSES_SCAN_PATH=~/hairglasses-studio -- go run ./cmd/ralphglasses-mcp

# Or via the Cobra subcommand
go run . mcp --scan-path ~/hairglasses-studio
```

A `.mcp.json` is also included in the repo root for automatic local discovery.

### Tools

| Tool | Description |
|------|-------------|
| `ralphglasses_scan` | Scan for ralph-enabled repos |
| `ralphglasses_list` | List all repos with status summary |
| `ralphglasses_status` | Detailed status for a repo (loop, circuit breaker, progress, config) |
| `ralphglasses_start` | Start a ralph loop |
| `ralphglasses_stop` | Stop a ralph loop |
| `ralphglasses_stop_all` | Stop all managed loops |
| `ralphglasses_pause` | Pause/resume a loop |
| `ralphglasses_logs` | Get recent log lines |
| `ralphglasses_config` | Get/set .ralphrc values |
| `ralphglasses_roadmap_parse` | Parse ROADMAP.md into structured JSON |
| `ralphglasses_roadmap_analyze` | Compare roadmap vs codebase (gaps, stale, ready) |
| `ralphglasses_roadmap_research` | Search GitHub for relevant repos/tools |
| `ralphglasses_roadmap_expand` | Generate proposed roadmap expansions |
| `ralphglasses_roadmap_export` | Export tasks as rdcycle/fix_plan/progress specs |
| `ralphglasses_repo_scaffold` | Create/init ralph config files for a repo |
| `ralphglasses_repo_optimize` | Analyze and optimize ralph config files |

## Architecture

- **main.go** → **cmd/root.go**: Cobra CLI with `--scan-path` flag
- **internal/discovery/**: Scans directories for `.ralph/` and `.ralphrc`
- **internal/model/**: Data types and parsers for status.json, progress.json, circuit breaker state, .ralphrc
- **internal/process/**: Process management (launch/stop/pause via os/exec), fsnotify file watcher, log tailing
- **internal/mcpserver/**: MCP tool handlers (16 tools, stdio transport via mcp-go)
- **internal/roadmap/**: Roadmap parsing, analysis, research, expansion, export
- **internal/repofiles/**: Ralph config file scaffolding and optimization
- **cmd/ralphglasses-mcp/**: Standalone MCP server binary entry point
- **internal/tui/**: Bubble Tea app model, keymap, command/filter modes
- **internal/tui/styles/**: Lipgloss theme (k9s-inspired, no other package imports this)
- **internal/tui/components/**: Reusable widgets (sortable table, breadcrumb, status bar, notifications)
- **internal/tui/views/**: View renderers (overview, repo detail, log stream, config editor, help)

## Marathon Supervisor

`marathon.sh` is a supervisor (not a thin wrapper) that runs ralph in the background and enforces guardrails:

```bash
# Requires: ANTHROPIC_API_KEY in environment (direnv loads .env automatically)
bash marathon.sh --dry-run                          # Preview
bash marathon.sh --verbose -p ~/hairglasses-studio/<project>  # Real run
bash marathon.sh -b 50 -d 6 -c 60                  # Custom budget/duration
```

### What it enforces
- **Duration limit**: Hard wallclock kill after N hours (default: 12)
- **Budget limit**: Reads `session_spend_usd` from `.ralph/status.json`, stops at 90% of budget ceiling (default: $100 × 0.90 = $90)
- **Checkpoints**: Git tag + commit every N hours (default: 3)
- **Signal handling**: SIGINT/SIGTERM → graceful SIGTERM to ralph → 30s window → SIGKILL
- **Logging**: All supervisor events → `.ralph/logs/marathon-*.log`

### Flags ralph actually reads from .ralphrc
Only `MAX_CALLS_PER_HOUR` and `CLAUDE_TIMEOUT_MINUTES` are used by ralph_loop.sh. Other marathon-specific keys (MARATHON_DURATION_HOURS, RALPH_SESSION_BUDGET, etc.) are only for documentation/reference — the supervisor enforces them externally.

### Environment setup
Uses direnv (`.envrc` → `dotenv` → `.env`). The `.env` holds `ANTHROPIC_API_KEY`. Both `.env` and `.envrc` are gitignored.

### Incompatibilities
`--monitor` is incompatible with the supervisor (tmux fork breaks PID tracking). Use `--verbose` or `--live` instead.

## Key Patterns

- **Styles are in their own package** (`internal/tui/styles/`) to avoid import cycles. Components and views import styles, not the tui package.
- **View stack**: `CurrentView` + `ViewStack` for breadcrumb navigation (push/pop).
- **Reactive updates**: fsnotify watches `.ralph/` dirs; falls back to 2s polling via `tea.Tick`.
- **Process management**: `os/exec` with process groups (`Setpgid`), SIGTERM/SIGSTOP/SIGCONT.

## File Schemas

- `.ralph/status.json`: LoopStatus (timestamp, loop_count, calls_made_this_hour, status, model, etc.)
- `.ralph/.circuit_breaker_state`: CircuitBreakerState (state: CLOSED/HALF_OPEN/OPEN, counters, reason)
- `.ralph/progress.json`: Progress (iteration, completed_ids, log entries, status)
- `.ralphrc`: Shell-style KEY="value" config (PROJECT_NAME, MAX_CALLS_PER_HOUR, CB thresholds, etc.)

## Distro / Thin Client

The `distro/` directory contains configs for a bootable Linux thin client that starts into the ralphglasses TUI for autonomous Claude Code agent marathons.

### Strategy

- **In-kernel drivers preferred** — no vendored blobs, no Windows drivers in this repo
- **NVIDIA via apt** — `nvidia-driver-550` at build time, not `.run` files
- **Target hardware**: ASUS ProArt X870E-CREATOR WIFI (Ryzen 9 7950X, RTX 4090, 128GB DDR5)
- **Dual-GPU constraint**: RTX 4090 only on Linux. GTX 1060 (Pascal) is blacklisted — driver conflict (one `nvidia.ko` loads at a time)
- **Display**: i3 + RTX 4090 (nvidia), AMD iGPU fallback (amdgpu, zero config)
- **Network**: Wired Intel I226-V 2.5GbE (`igc` module) — reliable for 12h+ marathons

### Key Files

- `distro/hardware/proart-x870e.md` — Full hardware manifest: PCI IDs, kernel modules, known issues, driver cross-reference
- `distro/scripts/hw-detect.sh` — First-boot hardware detection. Configures Xorg for RTX 4090, blacklists GTX 1060 and broken MT7927 Bluetooth. **Testable on WSL**: `distro/scripts/hw-detect.sh --dry-run`
- `distro/systemd/hw-detect.service` — Oneshot systemd unit, runs hw-detect.sh once at first boot (before display-manager)
- `distro/systemd/ralphglasses.service` — TUI autostart after graphical target

### What Doesn't Belong Here

- Windows driver archives (Google Drive)
- NVIDIA `.run` files (GitHub Release artifacts if needed)
- Firmware blobs, DKMS tarballs

### Future Phases (not yet created)

- `distro/Dockerfile` — Ubuntu 24.04 + kernel 6.12+ HWE + nvidia-driver-550 + Go + Claude Code
- `distro/Makefile` — ISO build pipeline (docker build -> squashfs -> ISO)
- `distro/i3/config` — Multi-monitor workspace assignment (depends on monitor strategy)
- `distro/grub/grub.cfg` — UEFI boot menu

### Layout

- **distro/hardware/**: Hardware manifests (PCI IDs, modules, issues)
- **distro/scripts/**: Build and detection scripts
- **distro/systemd/**: Systemd service units
- **distro/dietpi/**: Legacy DietPi config (deprecated)
- **distro/pxe/**: PXE network boot docs
- **distro/autorandr/**: Monitor profiles (populated after setup)

## Related Repos (same org)

- **mcpkit**: Go MCP framework — ralph loop engine, finops, sampling, workflow, gateway
- **hg-mcp**: Go MCP server with modular tool pattern (500+ tools)
- **claudekit**: Go MCP with rdcycle perpetual loop, budget profiles
- **shielddd**: Go + pure SQLite (modernc.org/sqlite) + MCP, audit logs
- **mesmer**: Go MCP server with ralph integration
