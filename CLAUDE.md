# Ralphglasses

Command-and-control TUI + bootable thin client for parallel multi-LLM agent fleets.

Supports **Claude Code**, **Gemini CLI**, and **OpenAI Codex CLI** as session providers. Claude serves as the primary orchestrator; Gemini and Codex are available as worker providers for cost optimization and task specialization.

## Two Deliverables

1. **`ralphglasses` Go binary** — Cross-platform Unix TUI (k9s-style, Charmbracelet). Manages multi-session, multi-provider LLM loops from any terminal.
2. **Bootable Linux thin client** — Ubuntu 24.04 + i3, boots into ralphglasses TUI. 7-monitor, dual-NVIDIA-GPU.

See ROADMAP.md for full plan. See docs/ for research.

## Build & Run

```bash
go build ./...
go run . --scan-path ~/hairglasses-studio
```

## Multi-LLM Provider Support

Sessions can target any of three providers via the `provider` parameter:

| Provider | CLI Binary | Default Model | Stream Format | Resume Support |
|----------|-----------|---------------|---------------|----------------|
| `claude` (default) | `claude` | `sonnet` | `stream-json` | Yes (`--resume`) |
| `gemini` | `gemini` | `gemini-3-pro` | `stream-json` | Yes (`--resume`) |
| `codex` | `codex` | `gpt-5.4-xhigh` | quiet mode | No |

### Prerequisites

```bash
# Claude Code (primary — already installed)
# https://docs.anthropic.com/en/docs/claude-code/overview

# Gemini CLI
npm install -g @google/gemini-cli
# https://ai.google.dev/gemini-api/docs

# OpenAI Codex CLI
npm install -g @openai/codex-cli
# https://platform.openai.com/docs/guides/codex
```

### Environment Variables

Each provider requires its own API key in the environment:

```bash
# .env (loaded via direnv)
ANTHROPIC_API_KEY=sk-ant-...    # Claude
GOOGLE_API_KEY=AIza...          # Gemini
OPENAI_API_KEY=sk-...           # Codex
```

### Orchestration Pattern

Claude leads, delegates subtasks to cheaper/specialized providers:

```
┌──────────────────────────────────────────────┐
│  Claude (lead)                                │
│  ├── Gemini worker: bulk code generation      │
│  ├── Codex worker: focused refactoring        │
│  └── Claude worker: complex architecture      │
└──────────────────────────────────────────────┘
```

Use `ralphglasses_team_create` with `provider` to set the lead's provider, then delegate tasks via `ralphglasses_team_delegate`. Each session tracks costs per-provider in the cost ledger.

## MCP Server

Ralphglasses is also an installable MCP server exposing 57 tools for managing ralph loops and multi-provider LLM sessions programmatically.

### Install

```bash
# Via claude CLI (recommended)
claude mcp add ralphglasses -- go run . mcp

# Or with custom scan path
claude mcp add ralphglasses -e RALPHGLASSES_SCAN_PATH=~/hairglasses-studio -- go run . mcp

# Or via the Cobra subcommand
go run . mcp --scan-path ~/hairglasses-studio
```

A `.mcp.json` is also included in the repo root for automatic local discovery.

### Tools

| Tool | Description |
|------|-------------|
| `ralphglasses_fleet_status` | Fleet dashboard: repos, sessions, teams, costs, alerts |
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
| `ralphglasses_session_launch` | Launch a headless LLM session (`provider`: claude/gemini/codex) |
| `ralphglasses_session_list` | List sessions (filterable by `provider`, repo, status) |
| `ralphglasses_session_status` | Detailed session info (provider, output, cost, turns, model) |
| `ralphglasses_session_resume` | Resume a previous session (with `provider` param) |
| `ralphglasses_session_stop` | Stop a running session |
| `ralphglasses_session_budget` | Get/update budget for a session |
| `ralphglasses_team_create` | Create agent team with `provider` for lead session |
| `ralphglasses_team_status` | Get team status (tasks, lead session, progress) |
| `ralphglasses_team_delegate` | Add a task to an existing team |
| `ralphglasses_agent_define` | Create/update .claude/agents/*.md agent definitions |
| `ralphglasses_agent_list` | List available agent definitions for a repo |
| `ralphglasses_journal_read` | Read improvement journal entries with synthesized context |
| `ralphglasses_journal_write` | Manually write an improvement note to a repo's journal |
| `ralphglasses_journal_prune` | Compact improvement journal to prevent unbounded growth |
| `ralphglasses_event_list` | Query recent fleet events (by type, repo, time range) |
| `ralphglasses_fleet_analytics` | Cost breakdown by provider/repo/time-period |
| `ralphglasses_session_compare` | Compare two sessions (cost, turns, duration, efficiency) |
| `ralphglasses_session_output` | Get recent output from a running session |
| `ralphglasses_repo_health` | Composite health score (0-100) for a repo |
| `ralphglasses_session_retry` | Re-launch a failed session with same params |
| `ralphglasses_config_bulk` | Get/set .ralphrc values across multiple repos |
| `ralphglasses_agent_compose` | Create composite agent by layering existing agents |
| `ralphglasses_workflow_define` | Define multi-step YAML workflows |
| `ralphglasses_workflow_run` | Execute workflows with dependency ordering |
| `ralphglasses_snapshot` | Save/list fleet state snapshots |
| `ralphglasses_session_stop_all` | Stop all running LLM sessions (emergency cost cutoff) |
| `ralphglasses_awesome_fetch` | Fetch + parse awesome-list README into structured entries |
| `ralphglasses_awesome_analyze` | Deep-analyze repos: fetch READMEs, score value/complexity vs ralph capabilities |
| `ralphglasses_awesome_diff` | Compare current list against previous fetch (new/removed entries) |
| `ralphglasses_awesome_report` | Generate formatted report from analysis results (json/markdown) |
| `ralphglasses_awesome_sync` | Full pipeline: fetch → diff → analyze new → report → save |
| `ralphglasses_prompt_analyze` | Score a prompt across 10 quality dimensions with letter grades and suggestions (`target_provider`: claude/gemini/openai) |
| `ralphglasses_prompt_enhance` | Run the 13-stage prompt enhancement pipeline (`target_provider`: claude/gemini/openai) |
| `ralphglasses_prompt_lint` | Deep-lint a prompt for anti-patterns (unmotivated rules, injection risks, etc.) |
| `ralphglasses_prompt_improve` | LLM-powered prompt improvement via Claude, Gemini, or OpenAI (`provider` param) |
| `ralphglasses_prompt_templates` | List available prompt templates with descriptions and required variables |
| `ralphglasses_prompt_template_fill` | Fill a prompt template with variable values |
| `ralphglasses_claudemd_check` | Health-check a repo's CLAUDE.md for common issues |
| `ralphglasses_prompt_classify` | Classify a prompt's task type (code, troubleshooting, analysis, etc.) |
| `ralphglasses_prompt_should_enhance` | Check whether a prompt would benefit from enhancement |
| `ralphglasses_session_tail` | Tail session output with cursor — returns only new lines since last call |
| `ralphglasses_session_diff` | Git changes made during a session's execution window |
| `ralphglasses_marathon_dashboard` | Compact marathon status: burn rate, stale sessions, team progress, alerts |
| `ralphglasses_session_errors` | Aggregated error view: parse failures, API errors, budget warnings |
| `ralphglasses_rc_status` | Compact fleet overview for mobile remote control (text output) |
| `ralphglasses_rc_send` | Send prompt to repo — auto-stops existing session, launches new |
| `ralphglasses_rc_read` | Read recent output from most active session with cursor |
| `ralphglasses_event_poll` | Cursor-based event polling for efficient mobile updates |
| `ralphglasses_rc_act` | Quick fleet actions: stop, stop_all, pause, resume, retry |

## Prompt Enhancement

The `internal/enhancer/` package (migrated from the archived `prompt-improver` repo) provides multi-provider prompt improvement and scoring:

- **13-stage deterministic pipeline**: specificity, positive reframing, tone downgrade (Claude-only), XML/markdown structure (provider-aware), context reorder, format enforcement, self-check injection, and more
- **10-dimensional quality scoring**: clarity, specificity, structure, examples, tone, etc. with letter grades (A-F) and provider-specific suggestions
- **11+ lint rules**: unmotivated rules, negative framing, aggressive caps, vague quantifiers, injection risks, cache-unfriendly ordering
- **Multi-provider LLM improvement**: Claude, Gemini, and OpenAI API clients with provider-specific meta-prompts, circuit breaker, and caching
- **CLAUDE.md health checks**: length, inline code, overtrigger language, style guide content detection
- **Ralph loop auto-enhancement**: Planner and worker prompts are automatically enhanced for their target provider before session launch

### Provider-Aware Behavior

| Concept | `LLM.Provider` / `provider` | `TargetProvider` / `target_provider` |
|---------|----------------------------|--------------------------------------|
| Controls | Which API to call for improvement | Which model family the enhanced prompt targets |
| Example | Use Claude API to improve a prompt | That will be sent to Gemini |
| Env var | `PROMPT_IMPROVER_PROVIDER` | `PROMPT_IMPROVER_TARGET` |

Pipeline stages that adapt per target provider:
- **tone_downgrade**: Skipped for non-Claude targets (Claude 4.x overtrigger-specific)
- **overtrigger_rewrite**: Skipped for non-Claude targets
- **structure**: XML tags for Claude, markdown headers (`## Role`, `## Instructions`) for Gemini/OpenAI

Scoring dimensions with provider-specific suggestions: Structure, Role Definition, Task Focus, Tone.

### Enhancement in the Ralph Loop

The `Manager.Enhancer` field on the session manager enables automatic prompt enhancement in `StepLoop`. Before launching planner and worker sessions, prompts are enhanced for the target provider (`ProviderCodex` maps to `ProviderOpenAI` for enhancement). Uses `ModeAuto` so LLM failures fall back to the local pipeline without blocking the loop.

The `ralphglasses_session_launch` tool supports an `enhance_prompt` parameter to auto-enhance prompts before launch. The TUI launcher shows a real-time prompt quality score after editing.

## Architecture

- **main.go** → **cmd/root.go**: Cobra CLI with `--scan-path` flag
- **internal/discovery/**: Scans directories for `.ralph/` and `.ralphrc`
- **internal/model/**: Data types and parsers for status.json, progress.json, circuit breaker state, .ralphrc
- **internal/process/**: Process management (launch/stop/pause via os/exec), fsnotify file watcher, log tailing
- **internal/session/**: Multi-provider LLM session management (claude/gemini/codex), agent teams, budget enforcement, provider dispatch
- **internal/mcpserver/**: MCP tool handlers (52 tools, stdio transport via mcp-go)
  - `tools.go` — Server struct, constructors, Register(), all handler implementations, helpers
  - `handler_prompt.go` — Multi-provider prompt enhancement handlers: analyze, enhance, lint, improve (with provider param), templates, classify, should_enhance, claudemd_check
- **internal/roadmap/**: Roadmap parsing, analysis, research, expansion, export
- **internal/repofiles/**: Ralph config file scaffolding and optimization
- **cmd/mcp.go**: MCP server subcommand (`go run . mcp`)
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
Uses direnv (`.envrc` → `dotenv` → `.env`). The `.env` holds API keys for all providers:
- `ANTHROPIC_API_KEY` — Claude Code
- `GOOGLE_API_KEY` — Gemini CLI
- `OPENAI_API_KEY` — Codex CLI

Both `.env` and `.envrc` are gitignored.

### Incompatibilities
`--monitor` is incompatible with the supervisor (tmux fork breaks PID tracking). Use `--verbose` or `--live` instead.

## Remote Control (RC) Tools

Tools prefixed `rc_` are optimized for mobile remote-control via Claude Android/iOS app (`/rc` in terminal → QR code → phone connects). Design principles:

- **Text responses** (`textResult`), not JSON — reads naturally in mobile chat bubbles
- **Minimal params** — most tools need 0-1 required parameters
- **One call per action** — `rc_send` does find-repo + stop-existing + launch in one call
- **Cursor-based polling** — `rc_read` and `event_poll` return cursors for incremental updates
- **Default budget $5** — prevents runaway spending from mobile

Key constraint: stdin closes after initial write (`runner.go:110`), so `rc_send` handles follow-up by stop-and-relaunch (or `resume=true` for session continuity).

See `docs/remote-control-research.md` for full architecture details.

## Provider Architecture

The `internal/session/` package uses a provider dispatch pattern:

- **`providers.go`**: Contains `buildCmdForProvider()` which dispatches to `buildClaudeCmd()`, `buildGeminiCmd()`, or `buildCodexCmd()`. Also contains `normalizeEvent()` which dispatches to per-provider event normalizers. `ValidateProvider()` checks CLI binary availability.
- **`runner.go`**: Provider-agnostic session lifecycle. Calls `buildCmdForProvider()` for command construction and `normalizeEvent()` for stream parsing.
- **`types.go`**: `Provider` type (`claude`|`gemini`|`codex`) used in `Session`, `LaunchOptions`, `TeamConfig`.
- **`budget.go`**: `LedgerEntry` and `CostSummary` include `Provider` field for per-provider cost tracking.

### Adding a New Provider

1. Add constant to `Provider` in `types.go`
2. Add binary name in `providerBinary()` in `providers.go`
3. Add `buildXxxCmd()` function in `providers.go`
4. Add `normalizeXxxEvent()` function in `providers.go`
5. Add default model in `ProviderDefaults()` in `providers.go`
6. Add cases in `buildCmdForProvider()` and `normalizeEvent()` switch statements
7. Add tests in `providers_test.go`

### Developer References

- **Claude Code**: [Overview](https://docs.anthropic.com/en/docs/claude-code/overview) | [CLI Reference](https://docs.anthropic.com/en/docs/claude-code/cli-reference) | [SDK](https://docs.anthropic.com/en/docs/claude-code/sdk)
- **Anthropic API**: [API Reference](https://docs.anthropic.com/en/api) | [Tool Use](https://docs.anthropic.com/en/docs/build-with-claude/tool-use)
- **Gemini**: [API Overview](https://ai.google.dev/gemini-api/docs) | [Gemini CLI](https://github.com/google-gemini/gemini-cli)
- **OpenAI**: [API Reference](https://platform.openai.com/docs/api-reference) | [Codex CLI](https://github.com/openai/codex)
- **MCP**: [Specification](https://modelcontextprotocol.io/) | [Go SDK (mcp-go)](https://github.com/mark3labs/mcp-go)

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
- `.ralph/improvement_patterns.json`: Consolidated durable patterns from journal (survives pruning)

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

## Per-Provider Config

- `.gemini/settings.json` — Gemini CLI MCP server registration
- `.codex/config.toml` — Codex CLI project config + MCP server registration
- See [GEMINI.md](GEMINI.md) for Gemini-specific instructions
- See [AGENTS.md](AGENTS.md) for Codex-specific instructions
- See [CONTRIBUTING.md](CONTRIBUTING.md) for multi-provider contribution guide

## Related Repos (same org)

- **mcpkit**: Go MCP framework — ralph loop engine, finops, sampling, workflow, gateway
- **hg-mcp**: Go MCP server with modular tool pattern (500+ tools)
- **claudekit**: Go MCP with rdcycle perpetual loop, budget profiles
- **shielddd**: Go + pure SQLite (modernc.org/sqlite) + MCP, audit logs
- **mesmer**: Go MCP server with ralph integration
