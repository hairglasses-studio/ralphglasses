# Contributing to Ralphglasses

Ralphglasses supports development with **Claude Code**, **Gemini CLI**, and **OpenAI Codex CLI**. Any provider can lead development — pick whichever has available quota.

## Development Setup

### 1. Clone and bootstrap

```bash
git clone https://github.com/hairglasses-studio/ralphglasses
cd ralphglasses
./scripts/bootstrap-toolchain.sh   # installs optional tools, checks prereqs
./scripts/dev/doctor.sh            # verify environment
```

### 2. Configure environment

Create `.env` with your API keys:

```bash
ANTHROPIC_API_KEY=sk-ant-...    # Claude (skip if using OAuth)
GOOGLE_API_KEY=AIza...          # Gemini
OPENAI_API_KEY=sk-...           # Codex
```

> **Claude OAuth users:** Do NOT set `ANTHROPIC_API_KEY` — it conflicts with OAuth.

If using direnv:

```bash
echo "dotenv" > .envrc
direnv allow
```

### 3. Build and verify

```bash
go build ./...          # must succeed before working
make ci                 # vet + test + build (required before every commit)
```

## Prerequisites

- **Go 1.26+**
- **Recommended**: `./scripts/bootstrap-toolchain.sh`
- **direnv** (loads `.env` automatically)
- At least one LLM provider CLI installed:

```bash
# OpenAI Codex CLI (primary)
npm install -g @openai/codex-cli

# Claude Code (optional)
# Already installed if you're reading this in Claude

# Gemini CLI
npm install -g @google/gemini-cli
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
gemini --approval-mode yolo     # Autonomous (no confirmations)
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

## Running Tests

```bash
# Quality gate (REQUIRED before every commit)
make ci                    # vet + test + build

# Individual targets
make test                  # go test -race ./...
make test-verbose          # go test -race -v ./...
make test-cover            # coverage report (opens HTML in browser)
make fuzz                  # fuzz parsers (30s each target)
make build                 # go build ./...
make vet                   # go vet ./...
make lint                  # golangci-lint (if installed)
```

If your host is missing `make`, `gcc`, `shellcheck`, or `bats`, use the repo devcontainer or run `./scripts/dev/ci.sh` directly.

Fuzz targets live in `internal/model/` (status and config parsers). Run them for longer during suspected edge-case fixes:

```bash
go test -fuzz=FuzzParseConfig -fuzztime=60s ./internal/model/
go test -fuzz=FuzzParseStatus -fuzztime=60s ./internal/model/
```

## Adding a New Provider

Follow these 7 steps, all within `internal/session/`:

1. **`types.go`** — Add a `Provider` constant (e.g., `ProviderMyLLM Provider = "myllm"`)
2. **`providers.go`** — Add binary name in `providerBinary()` switch
3. **`providers.go`** — Add `buildMyLLMCmd()` function that constructs the `*exec.Cmd`
4. **`providers.go`** — Add `normalizeMyLLMEvent()` function that maps raw output to `SessionEvent`
5. **`providers.go`** — Add default model in `ProviderDefaults()` switch
6. **`providers.go`** — Add cases in `buildCmdForProvider()` and `normalizeEvent()` switches
7. **`providers_test.go`** — Add tests: command construction, event normalization, binary validation

The `runner.go` file is provider-agnostic; it calls the dispatch functions above.

## Code Style

- **Format:** `gofmt` (enforced by `make vet`)
- **Lint:** `golangci-lint run` — fix all warnings before submitting
- **Tests:** use standard `testing` package only; no external test frameworks
- **No global state** in new code — pass dependencies via constructors
- **Styles are isolated** — `internal/tui/styles/` is the only package that may define Lip Gloss styles; components and views import it, not each other
- **Import cycles forbidden** — `styles` → no other internal package; `components` → `styles` only

## Submitting PRs

### Branch naming

```
feat/short-description        # new feature
fix/issue-or-symptom          # bug fix
chore/what-you-did            # dependency update, tooling, etc.
docs/what-you-documented      # documentation only
```

### Commit messages

Use the imperative mood, present tense. Reference issue numbers where relevant:

```
feat: add Anthropic Haiku provider support

Adds buildHaikuCmd() and normalizeHaikuEvent() to providers.go.
Haiku maps to claude --model haiku-4-5 with the same stream-json
output format as the existing Claude provider.

Closes #42
```

### What to include in a PR

- **Tests** for any new functionality (`make test` must pass with `-race`)
- **Update CONTRIBUTING.md** if you change development setup or tooling
- **Update CLAUDE.md** if you add a new provider, MCP tool, or architectural pattern
- **No generated files** — don't commit `go.sum` hunks you didn't cause
- `make ci` must pass locally before opening the PR

## MCP Server Setup

Use the repo-local `.mcp.json` as the source of truth. Provider-specific repo config surfaces:

- Claude: `.claude/settings.json`
- Gemini: `.gemini/settings.json`
- Codex: `.codex/config.toml`

All three point at `./scripts/dev/run-mcp.sh --scan-path ~/hairglasses-studio`.

When updating repo-level hook automation, keep Claude and Gemini on the shared supported hook surface only: `SessionStart`, `BeforeTool`, `AfterTool`, and `Notification`. Avoid Claude-only lifecycle hooks such as `Stop`, `SessionEnd`, or subagent hooks unless the provider-parity tooling has been extended first.

### Claude Code

Already configured in `.claude/settings.json`.

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
| Resume session | Yes | Yes | Yes (`codex exec resume`, install-dependent) |
| Budget enforcement | Native (`--max-budget-usd`) | External in ralphglasses | External in ralphglasses |
| Agent definitions | Native (`--agent`, `.claude/agents/`) | Native (`.gemini/agents/*.md`) and Legacy (`.gemini/commands/*.toml`) | Repo-native only (`.codex/agents/*.toml`, `AGENTS.md`) |
| Worktree isolation | Yes | Yes (`--worktree`) | No |
| System prompt flag | Yes | No | No |
| Permission mode | Yes (`--permission-mode`) | Yes (`--approval-mode`) | Mapped onto `--sandbox` by ralphglasses |
| Output schema | Yes (`--json-schema`) | No | Yes (`--output-schema`) |
| MCP client | Yes | Yes | Yes |
| MCP server mode | No | No | Yes (`codex mcp-server`) |
| Autonomous mode | Permission mode + allowed tools | `--approval-mode yolo` | `--full-auto` |
| Streaming output | `stream-json` | `stream-json` | `--json` (NDJSON) |
| Project instructions | `CLAUDE.md` | `GEMINI.md` | `AGENTS.md` |
| Skills/plugins | Skills + plugins | Skills + extensions | Skills + plugins + subagents |

Codex loops:
- Planner default: `gpt-5.4`
- Worker/verifier default: `gpt-5.4`
- Session resume is supported when the installed Codex CLI exposes `exec resume`.

## Cost Optimization

| Task Type | Recommended Provider | Why |
|-----------|---------------------|-----|
| Complex architecture | Claude | Best reasoning, agent support |
| Bulk code generation | Gemini | Fast, large context |
| Implementation | Codex (gpt-5.4) | Default primary control-plane model |
| Deep reasoning | Claude or frontier Codex override | Use the expensive lane intentionally |
| Balanced tasks | Codex (gpt-5.4) | Canonical default unless you intentionally choose a cheaper lane |
| Bulk/cheap tasks | Codex (gpt-5.4 or explicit cheaper override) | Defaults stay aligned; cost tuning is an explicit choice |
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
- **MCP server won't start**: Run `./scripts/dev/run-mcp.sh --scan-path ~/hairglasses-studio` directly to see startup errors.
- **direnv not loading**: Run `direnv allow` in the repo root.
