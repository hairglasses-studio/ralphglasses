# Contributing to Ralphglasses

Ralphglasses supports development with **Claude Code**, **Gemini CLI**, and **OpenAI Codex CLI**. Any provider can lead development — pick whichever has available quota.

## Prerequisites

- **Go 1.26+**
- **direnv** (loads `.env` automatically)
- At least one LLM provider CLI installed:

```bash
# Claude Code (primary)
# Already installed if you're reading this in Claude

# Gemini CLI
npm install -g @google/gemini-cli

# OpenAI Codex CLI
npm install -g @openai/codex-cli
```

- API keys in `.env`:

```bash
ANTHROPIC_API_KEY=sk-ant-...    # Claude
GOOGLE_API_KEY=AIza...          # Gemini
OPENAI_API_KEY=sk-...           # Codex
```

## Quick Start per Provider

### Claude Code

```bash
claude                          # Interactive
claude -p "implement feature X" --allowedTools "Bash,Read,Write,Edit,Glob,Grep"
claude -p "implement feature X" --output-format stream-json  # Headless
```

Project instructions: [CLAUDE.md](CLAUDE.md)

### Gemini CLI

```bash
gemini                          # Interactive
gemini --yolo                   # Autonomous (no confirmations)
gemini -p "implement feature X" --output-format stream-json  # Headless
```

Project instructions: [GEMINI.md](GEMINI.md)

### Codex CLI

```bash
codex                           # Interactive
codex exec --full-auto "implement feature X"  # Autonomous
codex --json                    # NDJSON output
```

Project instructions: [AGENTS.md](AGENTS.md)

## Build & Test

```bash
# Quality gate (REQUIRED before every commit)
make ci                    # vet + test + build

# Individual targets
make test                  # go test -race ./...
make test-verbose          # go test -race -v ./...
make test-cover            # coverage report
make fuzz                  # fuzz parsers (30s each)
make build                 # go build ./...
make vet                   # go vet ./...
make lint                  # golangci-lint (if installed)
```

`make ci` works regardless of which LLM provider runs it — it's pure Go toolchain.

## MCP Server Setup

Each provider has its own MCP configuration:

### Claude Code

```bash
claude mcp add ralphglasses -- go run . mcp --scan-path ~/hairglasses-studio
```

Or use the `.mcp.json` in the repo root (auto-discovered).

### Gemini CLI

Already configured in `.gemini/settings.json`. Verify:

```bash
gemini mcp list
```

### Codex CLI

Already configured in `.codex/config.toml`. Verify:

```bash
codex mcp list
```

## Multi-Agent Without Claude

When Claude Code is at capacity, use Gemini as lead with Codex workers:

```bash
# Gemini-led team via MCP
gemini --yolo -p "Use ralphglasses_team_create to create a team with provider gemini, then delegate subtasks to codex workers"
```

Or use Codex directly for focused tasks:

```bash
codex exec --full-auto "Read AGENTS.md, then fix the failing test in internal/session/"
```

## Provider Feature Matrix

| Feature | Claude Code | Gemini CLI | Codex CLI |
|---------|------------|------------|-----------|
| Resume session | Yes | Yes | No |
| Budget enforcement | Yes (external) | No | No |
| Agent definitions | Yes (`.claude/agents/`) | No | No |
| Worktree isolation | Yes | No | No |
| System prompt flag | Yes (`-s`) | No | No |
| MCP client | Yes | Yes | Yes |
| MCP server mode | No | No | Yes (`codex mcp-server`) |
| Autonomous mode | `--allowedTools` | `--yolo` | `--full-auto` |
| Streaming output | `stream-json` | `stream-json` | `--json` (NDJSON) |
| Project instructions | `CLAUDE.md` | `GEMINI.md` | `AGENTS.md` |

## Cost Optimization

| Task Type | Recommended Provider | Why |
|-----------|---------------------|-----|
| Complex architecture | Claude | Best reasoning, agent support |
| Bulk code generation | Gemini | Fast, large context |
| Implementation | Codex (gpt-5.4-xhigh) | Best non-thinking, default |
| Deep reasoning | Codex (o1-pro) | Extended thinking for architecture |
| Balanced tasks | Codex (gpt-4.1) | Good quality, moderate cost |
| Bulk/cheap tasks | Codex (o4-mini) | Fast, lowest cost |
| Test writing | Codex or Gemini | High volume, lower complexity |
| Code review | Claude or Gemini | Nuanced feedback |

## Troubleshooting

### Gemini CLI

- **"API key not valid"**: Check `GOOGLE_API_KEY` in `.env`, run `direnv allow`
- **MCP not connecting**: Verify `.gemini/settings.json` has correct `cwd` path
- **No project context**: Ensure `GEMINI.md` exists in project root

### Codex CLI

- **"Unauthorized"**: Check `OPENAI_API_KEY` in `.env`, run `direnv allow`
- **MCP not connecting**: Verify `.codex/config.toml` exists with `[mcp_servers.ralphglasses]`
- **Sandbox errors**: Run with `--sandbox workspace-write` or check `~/.codex/config.toml`
- **No project context**: Ensure `AGENTS.md` exists in project root (Codex reads it automatically)

### Claude Code

- **OAuth vs API key**: Claude Code uses OAuth by default. Do NOT set `ANTHROPIC_API_KEY` if using OAuth — it causes conflicts.
- **Nested session detection**: Unset `CLAUDECODE` env var before launching headless sessions.

### All Providers

- **`make ci` fails**: Run `make test-verbose` to see detailed failures.
- **MCP server won't start**: Run `go run . mcp` directly to see startup errors.
- **direnv not loading**: Run `direnv allow` in the repo root.
