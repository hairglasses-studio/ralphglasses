# Ralphglasses — Gemini CLI Instructions

This repo uses [AGENTS.md](AGENTS.md) as the canonical instruction file.

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

- **`providers.go`**: `buildCmdForProvider()` dispatches to per-provider command builders. `normalizeEvent()` dispatches to per-provider event normalizers. `ValidateProvider()` checks CLI binary availability.
- **`runner.go`**: Provider-agnostic session lifecycle.
- **`types.go`**: `Provider` type (`claude`|`gemini`|`codex`).
- **`budget.go`**: Per-provider cost tracking via `LedgerEntry` and `CostSummary`.

### Adding a New Provider

1. Add constant to `Provider` in `types.go`
2. Add binary name in `providerBinary()` in `providers.go`
3. Add `buildXxxCmd()` function in `providers.go`
4. Add `normalizeXxxEvent()` function in `providers.go`
5. Add default model in `ProviderDefaults()` in `providers.go`
6. Add cases in `buildCmdForProvider()` and `normalizeEvent()` switch statements
7. Add tests in `providers_test.go`

## Multi-LLM Provider Support

| Provider | CLI Binary | Default Model | Stream Format | Resume Support |
|----------|-----------|---------------|---------------|----------------|
| `codex` (default) | `codex` | `gpt-5.4` | `--json` (NDJSON) | Yes (`exec resume`, when supported by the installed CLI) |
| `claude` | `claude` | `sonnet` | `stream-json` | Yes (`--resume`) |
| `gemini` | `gemini` | `gemini-2.5-pro` | `stream-json` | Yes (`--resume`) |

## Gemini-Specific Notes

- **Autonomous mode**: Use `gemini --yolo` to skip all confirmations.
- **Output format**: `gemini --output-format stream-json` for structured streaming output.
- **Resume**: `gemini --resume <session-id>` to continue a previous session.
- **Default model**: `gemini-2.5-pro`. Configured in `.gemini/settings.json`.
- **No budget support**: Gemini CLI does not have built-in budget enforcement — ralphglasses tracks costs externally.
- **No system prompt flag**: Project context comes from `GEMINI.md` (this file), not a CLI flag.
- **Custom commands**: Project-scoped Gemini reusable workflows live in `.gemini/commands/*.toml`.
- **No worktree isolation**: Gemini CLI does not support git worktree isolation — use standard branching.

## MCP Server

Ralphglasses exposes 43 MCP tools. Gemini accesses them via `.gemini/settings.json` (already configured in this repo).

Key tools for Gemini-led development:

```
ralphglasses_session_launch    Launch a headless session (provider: gemini/claude/codex)
ralphglasses_team_create       Create a multi-provider team (Gemini as lead)
ralphglasses_team_delegate     Delegate subtasks to Codex/Claude workers
ralphglasses_session_list      List sessions (filter by provider)
ralphglasses_session_status    Get session info (cost, turns, model)
ralphglasses_fleet_status      Fleet dashboard: repos, sessions, teams, costs
ralphglasses_fleet_analytics   Cost breakdown by provider/repo/time-period
```

### Gemini-Led Team Pattern

Gemini can lead teams, delegating to cheaper/specialized providers:

```
┌──────────────────────────────────────────────┐
│  Gemini (lead)                                │
│  ├── Codex worker: focused refactoring        │
│  ├── Claude worker: complex architecture      │
│  └── Gemini worker: bulk code generation      │
└──────────────────────────────────────────────┘
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
- **[private]**: Go + pure SQLite (modernc.org/sqlite) + MCP, audit logs
- **mesmer**: Go MCP server with ralph integration

## See Also

- [CLAUDE.md](CLAUDE.md) — Claude Code project instructions
- [AGENTS.md](AGENTS.md) — Codex CLI project instructions
- [CONTRIBUTING.md](CONTRIBUTING.md) — Multi-provider contribution guide
- [ROADMAP.md](ROADMAP.md) — Full development roadmap

## Shared Research Repository

Cross-project research lives at `~/hairglasses-studio/docs/` (git: hairglasses-studio/docs). When launching research agents, check existing docs first and write reusable research outputs back to the shared repo rather than local docs/.
