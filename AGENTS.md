# Ralphglasses — Codex CLI Instructions

Command-and-control TUI + bootable thin client for parallel multi-LLM agent fleets.

Supports **Claude Code**, **Gemini CLI**, and **OpenAI Codex CLI** as session providers.

## Build & Run

```bash
go build ./...
go run . --scan-path ~/hairglasses-studio

# Quality gate (REQUIRED before every commit)
make ci
```

## Architecture

```
main.go → cmd/root.go (Cobra CLI)
├── internal/discovery/    Scan for .ralph/ repos
├── internal/model/        Status, progress, config parsers
├── internal/process/      Process management, file watcher, log tailing
├── internal/session/      Multi-provider LLM session management
│   ├── providers.go       Per-provider cmd builders + event normalizers
│   ├── runner.go          Session lifecycle (launch, stream, terminate)
│   ├── manager.go         Session/team registry
│   ├── budget.go          Per-provider cost tracking + enforcement
│   └── types.go           Provider enum, Session, LaunchOptions, TeamConfig
├── internal/mcpserver/    MCP tool handlers (43 tools, stdio)
├── internal/roadmap/      Roadmap parsing, analysis, research, export
├── internal/repofiles/    Ralph config scaffolding and optimization
├── internal/tui/          BubbleTea app, keymap, commands, filter
│   ├── styles/            Lip Gloss theme (k9s-inspired, isolated package)
│   ├── components/        Table, breadcrumb, status bar, notifications
│   └── views/             Overview, repo detail, log stream, config editor, help
├── distro/                Thin client build system
├── docs/                  Research & reference docs
└── scripts/               Shell helpers (marathon.sh)
```

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
- `.ralph/improvement_journal.jsonl`: Append-only JSONL, one entry per session (worked/failed/suggest)

## Provider Architecture

The `internal/session/` package uses a provider dispatch pattern:

- **`providers.go`**: `buildCmdForProvider()` dispatches to per-provider command builders. `normalizeEvent()` dispatches to per-provider event normalizers.
- **`runner.go`**: Provider-agnostic session lifecycle.
- **`types.go`**: `Provider` type (`claude`|`gemini`|`codex`).
- **`budget.go`**: Per-provider cost tracking via `LedgerEntry` and `CostSummary`.

## Multi-LLM Provider Support

| Provider | CLI Binary | Default Model | Stream Format | Resume Support |
|----------|-----------|---------------|---------------|----------------|
| `claude` (default) | `claude` | `sonnet` | `stream-json` | Yes (`--resume`) |
| `gemini` | `gemini` | `gemini-3-pro` | `stream-json` | Yes (`--resume`) |
| `codex` | `codex` | `gpt-5.4-xhigh` | quiet mode | No |

## Codex-Specific Notes

- **Autonomous mode**: Use `codex exec --full-auto` to run without confirmations.
- **Sandbox**: `codex --sandbox workspace-write` allows writes only within the workspace.
- **Output format**: `codex --json` for NDJSON structured output.
- **No resume support**: Codex CLI does not support session resume — use `ralphglasses_session_retry` to re-launch.
- **No system prompt flag**: Project context comes from `AGENTS.md` (this file) — Codex walks the directory tree and reads it automatically (32 KiB max).
- **No budget support**: Codex CLI does not have built-in budget enforcement — ralphglasses tracks costs externally.
- **No agent definitions**: Codex CLI does not support agent definition files.
- **MCP server**: Codex can expose itself as an MCP server via `codex mcp-server` for peer-to-peer delegation.
- **Default model**: `gpt-5.4-xhigh` — best non-thinking model for code generation, refactoring, and implementation tasks.
- **Thinking mode**: For complex architectural reasoning, use `codex --model o1-pro` which enables extended thinking. Higher cost but superior for design decisions and multi-step planning.
- **Model selection**: Available models: `gpt-5.4-xhigh` (default, best non-thinking), `o1-pro` (thinking/reasoning), `gpt-4.1` (balanced), `o4-mini` (fast/cheap). Override per-session with `codex --model <model>`.

## MCP Server

Ralphglasses exposes 43 MCP tools. Codex accesses them via `.codex/config.toml` (already configured in this repo).

Key tools for Codex-led development:

```
ralphglasses_session_launch    Launch a headless session (provider: codex/gemini/claude)
ralphglasses_team_create       Create a multi-provider team (Codex as lead)
ralphglasses_team_delegate     Delegate subtasks to Gemini/Claude workers
ralphglasses_session_list      List sessions (filter by provider)
ralphglasses_session_status    Get session info (cost, turns, model)
ralphglasses_fleet_status      Fleet dashboard: repos, sessions, teams, costs
ralphglasses_fleet_analytics   Cost breakdown by provider/repo/time-period
```

## Environment Variables

```bash
# .env (loaded via direnv)
ANTHROPIC_API_KEY=sk-ant-...    # Claude
GOOGLE_API_KEY=AIza...          # Gemini
OPENAI_API_KEY=sk-...           # Codex
```

## Distro / Thin Client

The `distro/` directory contains configs for a bootable Linux thin client (Ubuntu 24.04 + i3) that starts into the ralphglasses TUI. Target hardware: ASUS ProArt X870E-CREATOR WIFI (Ryzen 9 7950X, RTX 4090, 128GB DDR5, 7-monitor).

## Related Repos

- **mcpkit**: Go MCP framework — ralph loop engine, finops, sampling, workflow, gateway
- **hg-mcp**: Go MCP server with modular tool pattern (500+ tools)
- **claudekit**: Go MCP with rdcycle perpetual loop, budget profiles
- **shielddd**: Go + pure SQLite (modernc.org/sqlite) + MCP, audit logs
- **mesmer**: Go MCP server with ralph integration

## See Also

- [CLAUDE.md](CLAUDE.md) — Claude Code project instructions
- [GEMINI.md](GEMINI.md) — Gemini CLI project instructions
- [CONTRIBUTING.md](CONTRIBUTING.md) — Multi-provider contribution guide
- [ROADMAP.md](ROADMAP.md) — Full development roadmap
