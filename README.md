# ralphglasses

Command-and-control TUI for parallel multi-LLM agent fleets.

Orchestrates [Claude Code](https://docs.anthropic.com/en/docs/claude-code/overview), [Gemini CLI](https://ai.google.dev/gemini-api/docs), and [OpenAI Codex CLI](https://platform.openai.com/docs/guides/codex) sessions from a single k9s-style interface. Built with [Charmbracelet](https://github.com/charmbracelet) (BubbleTea + Lip Gloss).

## What It Does

- **Orchestrate** multiple LLM providers (Claude, Gemini, Codex) with unified session management
- **Discover** ralph-enabled repos across your workspace (`--scan-path`)
- **Monitor** live status: loop iteration, circuit breaker state, per-provider costs, model selection
- **Control** ralph loops, headless sessions, and Codex planner/worker loops from TUI or MCP tools
- **Track** costs across all providers in a unified cost ledger
- **Stream** logs in real-time with reactive file watching (fsnotify)
- **Configure** `.ralphrc` settings per repo from an in-TUI editor

## Quick Start

```bash
# Bootstrap local tooling if needed
./scripts/bootstrap-toolchain.sh

# Build
./scripts/dev/go.sh build ./...

# Launch TUI
./scripts/dev/go.sh run . --scan-path ~/hairglasses-studio

# Or install the MCP server for Claude Code
claude mcp add ralphglasses -- ./scripts/dev/run-mcp.sh --scan-path ~/hairglasses-studio
```

## Multi-LLM Provider Support

Launch sessions against any supported provider:

| Provider | CLI | Default Model | Install |
|----------|-----|---------------|---------|
| `claude` (default) | [Claude Code](https://docs.anthropic.com/en/docs/claude-code/overview) | `sonnet` | Pre-installed |
| `gemini` | [Gemini CLI](https://ai.google.dev/gemini-api/docs) | `gemini-3-pro` | `npm i -g @google/gemini-cli` |
| `codex` | [Codex CLI](https://platform.openai.com/docs/guides/codex) | `gpt-5.4-xhigh` | `npm i -g @openai/codex-cli` |

### Usage via MCP

```jsonc
// Launch a Gemini session
{ "tool": "ralphglasses_session_launch", "arguments": {
    "repo": "my-project", "prompt": "Refactor the API layer", "provider": "gemini"
}}

// Create a multi-provider team (Claude leads, delegates to Gemini/Codex workers)
{ "tool": "ralphglasses_team_create", "arguments": {
    "repo": "my-project", "name": "refactor-team", "provider": "claude",
    "tasks": "Rewrite auth middleware\nAdd integration tests\nUpdate API docs"
}}

// List only Gemini sessions
{ "tool": "ralphglasses_session_list", "arguments": { "provider": "gemini" }}
```

### Required Environment Variables

```bash
ANTHROPIC_API_KEY=sk-ant-...    # Claude Code
GOOGLE_API_KEY=AIza...          # Gemini CLI
OPENAI_API_KEY=sk-...           # Codex CLI
```

## Two Deliverables

### 1. `ralphglasses` Go Binary
Cross-platform Unix TUI that manages multi-session, multi-provider LLM loops from any terminal.

### 2. Bootable Linux Thin Client
Ubuntu 24.04-based, boots into i3 + ralphglasses TUI. Supports 7-monitor, dual-NVIDIA-GPU setups.

See [ROADMAP.md](ROADMAP.md) for the full plan.

## MCP Server

47 tools for programmatic control across all providers:

### Core Loop Management
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

### Multi-Provider Session Management
| Tool | Description |
|------|-------------|
| `ralphglasses_session_launch` | Launch a headless session (`provider`: claude/gemini/codex) |
| `ralphglasses_session_list` | List sessions (filter by `provider`, repo, status) |
| `ralphglasses_session_status` | Detailed session info (provider, cost, turns, model) |
| `ralphglasses_session_resume` | Resume a previous session (`claude`/`gemini`; Codex unsupported) |
| `ralphglasses_session_stop` | Stop a running session |
| `ralphglasses_session_budget` | Get/update budget for a session |
| `ralphglasses_loop_start` | Create a Codex `o1-pro` planner + `gpt-5.4-xhigh` worker loop |
| `ralphglasses_loop_status` | Inspect persisted loop status and iteration history |
| `ralphglasses_loop_step` | Run one planner/worker/verifier iteration in a git worktree |
| `ralphglasses_loop_stop` | Stop a loop and block future iterations |

### Agent Teams
| Tool | Description |
|------|-------------|
| `ralphglasses_team_create` | Create team with `provider` for lead session |
| `ralphglasses_team_status` | Get team status and progress |
| `ralphglasses_team_delegate` | Add a task to an existing team |
| `ralphglasses_agent_define` | Create/update agent definitions |
| `ralphglasses_agent_list` | List available agent definitions |

### Roadmap Automation
| Tool | Description |
|------|-------------|
| `ralphglasses_roadmap_parse` | Parse ROADMAP.md into structured JSON |
| `ralphglasses_roadmap_analyze` | Compare roadmap vs codebase |
| `ralphglasses_roadmap_research` | Search GitHub for relevant repos/tools |
| `ralphglasses_roadmap_expand` | Generate proposed roadmap expansions |
| `ralphglasses_roadmap_export` | Export tasks as rdcycle/fix_plan specs |

### Repo Configuration
| Tool | Description |
|------|-------------|
| `ralphglasses_repo_scaffold` | Create/init ralph config files for a repo |
| `ralphglasses_repo_optimize` | Analyze and optimize ralph config |

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
├── internal/mcpserver/    MCP tool handlers (47 tools, stdio)
├── internal/roadmap/      Roadmap parsing, analysis, research, export
├── internal/repofiles/    Ralph config scaffolding and optimization
├── internal/tui/          BubbleTea app, keymap, commands, filter
│   ├── styles/            Lip Gloss theme (k9s-inspired)
│   ├── components/        Table, breadcrumb, status bar, notifications
│   └── views/             Overview, repo detail, log stream, config editor, help
├── distro/                Thin client build system
│   ├── hardware/          Hardware manifests (PCI IDs, modules)
│   ├── scripts/           Build and detection scripts
│   ├── systemd/           Systemd service units
│   └── pxe/               PXE network boot docs
├── docs/                  Research & reference docs
└── scripts/               Shell helpers (marathon.sh)
```

## Developer References

### API & CLI Documentation
- **Claude Code**: [Overview](https://docs.anthropic.com/en/docs/claude-code/overview) | [CLI Reference](https://docs.anthropic.com/en/docs/claude-code/cli-reference) | [SDK](https://docs.anthropic.com/en/docs/claude-code/sdk)
- **Anthropic API**: [API Reference](https://docs.anthropic.com/en/api) | [Tool Use](https://docs.anthropic.com/en/docs/build-with-claude/tool-use)
- **Gemini**: [API Overview](https://ai.google.dev/gemini-api/docs) | [Models](https://ai.google.dev/gemini-api/docs/models) | [Gemini CLI](https://github.com/google-gemini/gemini-cli)
- **OpenAI**: [API Reference](https://platform.openai.com/docs/api-reference) | [Codex CLI](https://github.com/openai/codex) | [Models](https://platform.openai.com/docs/models)

### Frameworks & Libraries
- **MCP (Model Context Protocol)**: [Specification](https://modelcontextprotocol.io/) | [Go SDK (mcp-go)](https://github.com/mark3labs/mcp-go)
- **Charmbracelet**: [Bubble Tea](https://github.com/charmbracelet/bubbletea) | [Lip Gloss](https://github.com/charmbracelet/lipgloss) | [Bubbles](https://github.com/charmbracelet/bubbles)

## Docs

- [ROADMAP.md](ROADMAP.md) — Full development roadmap
- [docs/RESEARCH.md](docs/RESEARCH.md) — Agent OS & sandboxing research
- [docs/MULTI-SESSION.md](docs/MULTI-SESSION.md) — Multi-session tool comparison
- [CLAUDE.md](CLAUDE.md) — Claude Code project instructions
- [GEMINI.md](GEMINI.md) — Gemini CLI project instructions
- [AGENTS.md](AGENTS.md) — Codex CLI project instructions
- [CONTRIBUTING.md](CONTRIBUTING.md) — Multi-provider contribution guide
- [docs/issue-ledger.json](docs/issue-ledger.json) — Current repo issue ledger
