# ralphglasses

Command-and-control TUI for parallel Claude Code agent fleets.

Built with [Charmbracelet](https://github.com/charmbracelet) (BubbleTea + Lip Gloss). Inspired by [k9s](https://k9scli.io/).

## What It Does

- **Discover** ralph-enabled repos across your workspace (`--scan-path`)
- **Monitor** live status: loop iteration, circuit breaker state, costs, model selection
- **Control** ralph loops: start, stop, pause, resume — from TUI or MCP tools
- **Stream** logs in real-time with reactive file watching (fsnotify)
- **Configure** `.ralphrc` settings per repo from an in-TUI editor

## Quick Start

```bash
# Build
go build ./...

# Launch TUI
go run . --scan-path ~/hairglasses-studio

# Or install the MCP server for Claude Code
claude mcp add ralphglasses -- go run ./cmd/ralphglasses-mcp
```

## Two Deliverables

### 1. `ralphglasses` Go Binary
Cross-platform Unix TUI that manages multi-session Claude Code / ralph loops from any terminal.

### 2. Bootable Linux Thin Client
DietPi-based, boots into i3 + ralphglasses TUI. Supports 7-monitor, dual-NVIDIA-GPU setups.

See [ROADMAP.md](ROADMAP.md) for the full plan.

## MCP Server

9 tools for programmatic control:

| Tool | Description |
|------|-------------|
| `ralphglasses_scan` | Scan for ralph-enabled repos |
| `ralphglasses_list` | List all repos with status |
| `ralphglasses_status` | Detailed status for a repo |
| `ralphglasses_start` | Start a ralph loop |
| `ralphglasses_stop` | Stop a ralph loop |
| `ralphglasses_stop_all` | Stop all managed loops |
| `ralphglasses_pause` | Pause/resume a loop |
| `ralphglasses_logs` | Get recent log lines |
| `ralphglasses_config` | Get/set .ralphrc values |

## Architecture

```
main.go → cmd/root.go (Cobra CLI)
├── internal/discovery/    Scan for .ralph/ repos
├── internal/model/        Status, progress, config parsers
├── internal/process/      Process management, file watcher, log tailing
├── internal/mcpserver/    MCP tool handlers (9 tools, stdio)
├── internal/tui/          BubbleTea app, keymap, commands, filter
│   ├── styles/            Lip Gloss theme (k9s-inspired)
│   ├── components/        Table, breadcrumb, status bar, notifications
│   └── views/             Overview, repo detail, log stream, config editor, help
├── distro/                Thin client build system
│   ├── dietpi/            DietPi automation config
│   ├── i3/                7-monitor i3 configs
│   ├── pxe/               PXE boot server configs
│   └── systemd/           Auto-login + TUI autostart
├── docs/                  Research & reference docs
└── scripts/               Shell helpers (marathon.sh)
```

## Docs

- [ROADMAP.md](ROADMAP.md) — Full development roadmap
- [docs/RESEARCH.md](docs/RESEARCH.md) — Agent OS & sandboxing research
- [docs/MULTI-SESSION.md](docs/MULTI-SESSION.md) — Multi-session Claude Code tool comparison
- [CLAUDE.md](CLAUDE.md) — Architecture conventions for Claude Code agents
