# Ralphglasses — Codex CLI Instructions
> Canonical instructions: AGENTS.md

Command-and-control TUI + bootable thin client for parallel multi-LLM agent fleets.

Supports **Claude Code**, **Gemini CLI**, **OpenAI Codex CLI**, **Google Antigravity** (experimental external-manager handoff), and **Cline CLI** as session providers.

## Start Here

1. Read the architecture tree below before changing module boundaries
2. Check `~/hairglasses-studio/docs/` for existing research before starting new work
3. Run `make ci` before every commit — this is the quality gate
4. **Role and skill surfaces**: `.agents/skills/` is canonical for workflows; `.agents/roles/` is canonical for reusable fleet roles; native role projections live in `.codex/agents/`, `.claude/agents/`, `.gemini/agents/`, and `.clinerules`
5. **Tool discovery**: `ralphglasses_tool_groups` → `ralphglasses_load_tool_group <name>` → use tools. Groups are deferred-loaded to save context tokens.

## Build & Run

```bash
./scripts/bootstrap-toolchain.sh
./scripts/dev/go.sh build ./...
./scripts/dev/go.sh run . --scan-path ~/hairglasses-studio

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
├── internal/mcpserver/    MCP tool handlers (222 tools: 218 grouped + 4 management, stdio)
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
- **`provider_capabilities.go`**: canonical provider capability registry used by launch validation, warnings, and MCP comparison surfaces.
- **`runner.go`**: Provider-agnostic session lifecycle.
- **`types.go`**: `Provider` type (`claude`|`gemini`|`codex`|`antigravity`).
- **`budget.go`**: Per-provider cost tracking via `LedgerEntry` and `CostSummary`.

## Multi-LLM Provider Support

| Provider | CLI Binary | Default Model | Stream Format | Resume Support |
|----------|-----------|---------------|---------------|----------------|
| `codex` (default) | `codex` | `gpt-5.4` | `--json` (NDJSON) | Yes (`exec resume`, when supported by the installed CLI) |
| `claude` | `claude` | `sonnet` | `stream-json` | Yes (`--resume`) |
| `gemini` | `gemini` | `gemini-3.1-pro` | `stream-json` | Yes (`--resume`) |
| `antigravity` | `antigravity` | Antigravity-managed | External interactive handoff | No |
| `cline` | `cline` | Cline-managed (free tier) | `--json` (NDJSON) | Yes (`--taskId`, `--continue`) |

Antigravity is intentionally narrower than the streaming providers. Ralph launches `antigravity chat --mode agent --new-window` in the repo, records a synthetic handoff session, and relies on `AGENTS.md`, `.agents/rules/`, `.agents/workflows/`, `.agents/skills/`, and `.mcp.json` for repo-native integration instead of trying to emulate Claude/Codex-style structured runtime control.

## Codex-Specific Notes

- **Autonomous mode**: Use `codex exec --full-auto` to run without confirmations.
- **Sandbox**: `codex --sandbox workspace-write` allows writes only within the workspace.
- **Output format**: `codex --json` for NDJSON structured output.
- **Resume support**: Use `codex exec resume` when the installed CLI exposes it. ralphglasses probes support at runtime.
- **No system prompt flag**: Project context comes from `AGENTS.md` (this file) — Codex walks the directory tree and reads it automatically (32 KiB max).
- **No budget support**: Codex CLI does not have built-in budget enforcement — ralphglasses tracks costs externally.
- **Canonical role catalog**: Provider-neutral reusable fleet roles live in `.agents/roles/*.json`; shared workflows live in `.agents/skills/`.
- **Native role projection**: Project-scoped Codex subagents live in `.codex/agents/*.toml`.
- **Native structure**: Use `AGENTS.md`, `.agents/roles/`, `.agents/skills/`, `.codex/agents/`, skills, and plugins for Codex-native repo context.
- **MCP server**: Codex can expose itself as an MCP server via `codex mcp-server` for peer-to-peer delegation.
- **Default model**: `gpt-5.4` for primary coding control-plane work.
- **Loop defaults**: `gpt-5.4` for planner, worker, and verifier lanes unless you intentionally override them.
- **Pinned references**: See `docs/CODEX-REFERENCE.md` for current docs, local CLI baseline, and Claude cache guardrails.

## MCP Server

Ralphglasses exposes 222 MCP tools: 218 grouped tools plus 4 management tools across 30 deferred-loaded tool groups. Codex accesses them via `.codex/config.toml` (already configured in this repo).

Key tools for Codex-led development:

```
ralphglasses_session_launch    Launch a session (provider: codex/gemini/claude/antigravity)
ralphglasses_team_create       Create a multi-provider team (Codex as lead)
ralphglasses_team_delegate     Delegate subtasks to Gemini/Claude workers
ralphglasses_session_list      List sessions (filter by provider)
ralphglasses_session_status    Get session info (cost, turns, model)
ralphglasses_fleet_status      Fleet dashboard: repos, sessions, teams, costs
ralphglasses_fleet_analytics   Cost breakdown by provider/repo/time-period
ralphglasses_loop_start        Start a Codex planner/worker loop
ralphglasses_loop_step         Execute one planner/worker/verifier iteration
ralphglasses_loop_status       Inspect persisted loop state
ralphglasses_doctor            Workspace/env readiness parity for CLI doctor
ralphglasses_validate          .ralphrc validation parity for CLI validate
ralphglasses_fleet_runtime     MCP parity for fleet serve coordinator/worker runtime
ralphglasses_marathon          MCP parity for marathon lifecycle and status
ralphglasses_repo_surface_audit Audit AGENTS/CLAUDE/GEMINI/Codex/MCP repo surfaces
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

## Tool Discovery Workflow

MCP tools are organized in 30 deferred-loaded groups. Follow this pattern:

1. `ralphglasses_tool_groups` — list all available groups with tool counts
2. `ralphglasses_load_tool_group <name>` — load a specific group into context
3. Use the loaded tools directly
4. `ralphglasses_tool_search <query>` — find tools by keyword across all groups

Groups: `core`, `session`, `loop`, `prompt`, `fleet`, `repo`, `roadmap`, `team`, `tenant`, `awesome`, `advanced`, `events`, `feedback`, `eval`, `fleet_h`, `observability`, `rdcycle`, `plugin`, `sweep`, `rc`, `autonomy`, `workflow`, `docs`, `recovery`, `promptdj`, `a2a`, `trigger`, `approval`, `context`, `prefetch`.

## Cross-Provider Session Continuity

Sessions persist state via MCP tools enabling handoff between providers:
- `ralphglasses_session_handoff` — export session context for another provider to resume
- `ralphglasses_session_fork` — create a parallel session from a checkpoint
- `.ralph/improvement_journal.jsonl` — append-only log readable by any provider
- `.ralph/status.json` + `.ralph/progress.json` — shared state files watchable by any session

## Role And Skill Surfaces

| Location | Purpose | Canonical? |
|----------|---------|-----------|
| `.agents/skills/` | Provider-neutral workflow definitions | Yes — source of truth for skills and workflows |
| `.agents/workflows/` | Provider-neutral workflow command surfaces | Yes — generated from the canonical workflow catalog |
| `.agents/rules/` | Provider-neutral Antigravity/Gemini rule surfaces | Yes — generated from canonical repo guidance |
| `.agents/roles/` | Provider-neutral reusable fleet role definitions | Yes — source of truth for reusable roles |
| `.claude/skills/` | Claude Code skill projections | Generated mirror |
| `.codex/agents/` | Codex native role projections | Generated from `.agents/roles/*.json` |
| `.claude/agents/` | Claude native role projections | Generated from `.agents/roles/*.json` |
| `.gemini/agents/` | Gemini native role projections | Generated from `.agents/roles/*.json` |
| `.gemini/commands/ralph/*.toml` | Generated workflow command mirrors for Gemini and Antigravity | Generated mirror |
| `.gemini/extensions/ralphglasses-workspace/` | Thin extension bundle for Gemini/Antigravity linking | Generated mirror |

Regenerate skill surfaces: `make skill-surface`
Regenerate role projections: `python3 scripts/sync-provider-roles.py`

## See Also

- [CLAUDE.md](CLAUDE.md) — Claude Code project instructions
- [GEMINI.md](GEMINI.md) — Gemini CLI project instructions
- [CONTRIBUTING.md](CONTRIBUTING.md) — Multi-provider contribution guide
- [docs/PROVIDER-PARITY-OBJECTIVES.md](docs/PROVIDER-PARITY-OBJECTIVES.md) — capability matrix, repo surfaces, and workflow parity targets
- [docs/CODEX-REFERENCE.md](docs/CODEX-REFERENCE.md) — Codex docs + Claude cache protection notes
- [ROADMAP.md](ROADMAP.md) — Full development roadmap

## Shared Research Repository

Cross-project research lives at `~/hairglasses-studio/docs/` (git: hairglasses-studio/docs). When launching research agents, check existing docs first and write reusable research outputs back to the shared repo rather than local docs/.
