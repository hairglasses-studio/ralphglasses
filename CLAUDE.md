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

## Architecture

- **main.go** → **cmd/root.go**: Cobra CLI with `--scan-path` flag
- **internal/discovery/**: Scans directories for `.ralph/` and `.ralphrc`
- **internal/model/**: Data types and parsers for status.json, progress.json, circuit breaker state, .ralphrc
- **internal/process/**: Process management (launch/stop/pause via os/exec), fsnotify file watcher, log tailing
- **internal/mcpserver/**: MCP tool handlers (9 tools, stdio transport via mcp-go)
- **cmd/ralphglasses-mcp/**: Standalone MCP server binary entry point
- **internal/tui/**: Bubble Tea app model, keymap, command/filter modes
- **internal/tui/styles/**: Lipgloss theme (k9s-inspired, no other package imports this)
- **internal/tui/components/**: Reusable widgets (sortable table, breadcrumb, status bar, notifications)
- **internal/tui/views/**: View renderers (overview, repo detail, log stream, config editor, help)

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

## Thin Client Layout

- **distro/dietpi/**: DietPi automation config + post-install script
- **distro/i3/**: i3 config for 7-monitor workspace assignment
- **distro/pxe/**: PXE boot server configs (LTSP/ThinStation)
- **distro/systemd/**: Auto-login + TUI autostart services
- **distro/autorandr/**: Multi-monitor xrandr profiles

## Related Repos (same org)

- **mcpkit**: Go MCP framework — ralph loop engine, finops, sampling, workflow, gateway
- **hg-mcp**: Go MCP server with modular tool pattern (500+ tools)
- **claudekit**: Go MCP with rdcycle perpetual loop, budget profiles
- **shielddd**: Go + pure SQLite (modernc.org/sqlite) + MCP, audit logs
- **mesmer**: Go MCP server with ralph integration
