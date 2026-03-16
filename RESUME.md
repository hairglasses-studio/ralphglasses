# Resume Instructions for WSL/Ubuntu Session

## Quick Start

```bash
# 1. Clone
git clone <repo-url> ~/hairglasses-studio/ralphglasses
cd ~/hairglasses-studio/ralphglasses

# 2. Install Go 1.22+ if needed
# Ubuntu/WSL: sudo snap install go --classic

# 3. Verify build
go build ./...
go vet ./...

# 4. Get your API key from 1Password
#    Item: "Anthropic API Key (Work - 10K credits)" in Personal vault
#    Or: op read "op://Personal/Anthropic API Key (Work - 10K credits)/credential" --account my.1password.com
export ANTHROPIC_API_KEY="$(op read 'op://Personal/Anthropic API Key (Work - 10K credits)/credential' --account my.1password.com)"

# 5. Install ralph-claude-code (if not present)
git clone <ralph-claude-code-repo-url> ~/hairglasses-studio/ralph-claude-code
# Ensure `ralph` is on PATH or set RALPH_CMD

# 6. Launch the marathon
./marathon.sh -b 100 -d 12 -c 80 -v -m
```

## What's Been Built

Ralphglasses is a command-and-control TUI + MCP server for managing parallel ralph loops.

### Complete (all compile clean):
- **TUI** (Bubble Tea): Overview table, repo detail, log viewer, config editor, help overlay
- **MCP Server** (9 tools): scan, list, status, start, stop, stop_all, pause, logs, config
- **Process Manager**: Launch/stop/pause loops via os/exec with process groups
- **Discovery**: Scans directories for .ralph/ and .ralphrc
- **Model layer**: Parsers for status.json, progress.json, circuit_breaker_state, .ralphrc
- **File watcher**: fsnotify for reactive status updates
- **Marathon script**: `marathon.sh` with adjustable budget/duration/calls

### Not yet built (marathon task list in `.ralph/PROMPT.md`):
1. Tests for all packages
2. MCP server hardening (circuit reset tool, concurrency, logging)
3. TUI polish (confirm dialogs, graceful shutdown, scroll bounds)
4. Process manager improvements (PID detection, auto-restart)
5. Config editor enhancements (add/delete keys, validation)
6. Documentation

## Key Architecture Notes

- **Styles are in `internal/tui/styles/`** (separate package to avoid import cycles)
- **Field is `CurrentView`** not `View` (conflicts with Bubble Tea's `View()` method)
- **mcp-go v0.45.0**: `req.Params.Arguments` is `any`, must type-assert to `map[string]any`
- **Process groups**: Uses `Setpgid: true` and `kill(-pgid, sig)` — Linux compatible

## MCP Server Installation

```bash
# Install for Claude Code
claude mcp add ralphglasses -- go run ./cmd/ralphglasses-mcp

# Or with env override
claude mcp add ralphglasses -e RALPHGLASSES_SCAN_PATH=~/hairglasses-studio -- go run ./cmd/ralphglasses-mcp
```

## File Layout

```
ralphglasses/
├── main.go                          # Entry point → cmd/root.go
├── marathon.sh                      # Marathon launcher (adjustable args)
├── .mcp.json                        # MCP auto-discovery
├── .ralphrc                         # Ralph config (marathon defaults)
├── .ralph/PROMPT.md                 # Marathon task list
├── .ralph/fix_plan.md               # Launch/recovery instructions
├── CLAUDE.md                        # Architecture docs
├── RESUME.md                        # This file
├── cmd/
│   ├── root.go                      # Cobra CLI (--scan-path)
│   ├── mcp.go                       # `ralphglasses mcp` subcommand
│   └── ralphglasses-mcp/main.go     # Standalone MCP binary
└── internal/
    ├── discovery/scanner.go
    ├── model/{repo,status,config}.go
    ├── process/{manager,watcher,logstream}.go
    ├── mcpserver/tools.go
    └── tui/
        ├── app.go, keymap.go, command.go, filter.go
        ├── styles/styles.go
        ├── components/{table,breadcrumb,statusbar,notification}.go
        └── views/{overview,repodetail,logstream,configeditor,help}.go
```
