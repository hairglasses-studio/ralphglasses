# Multi-LLM Provider Setup

Sessions can target any of three providers via the `provider` parameter:

| Provider | CLI Binary | Default Model | Stream Format | Resume Support |
|----------|-----------|---------------|---------------|----------------|
| `codex` (default) | `codex` | `gpt-5.4` | `--json` (NDJSON) | Yes (`exec resume`, when supported by the installed CLI) |
| `claude` | `claude` | `sonnet` | `stream-json` | Yes (`--resume`) |
| `gemini` | `gemini` | `gemini-2.5-pro` | `stream-json` | Yes (`--resume`) |

## Prerequisites

```bash
# Claude Code
# https://docs.anthropic.com/en/docs/claude-code/overview

# Gemini CLI
npm install -g @google/gemini-cli
# https://ai.google.dev/gemini-api/docs

# OpenAI Codex CLI
npm install -g @openai/codex-cli
# https://developers.openai.com/codex/noninteractive
```

## Environment Variables

Each provider requires its own API key in the environment:

```bash
# .env (loaded via direnv)
ANTHROPIC_API_KEY=sk-ant-...    # Claude
GOOGLE_API_KEY=AIza...          # Gemini
OPENAI_API_KEY=sk-...           # Codex
```

## Orchestration Pattern

Codex leads by default and delegates subtasks to specialized providers:

```
┌──────────────────────────────────────────────┐
│  Codex (lead)                                 │
│  ├── Gemini worker: bulk generation / cheap   │
│  ├── Codex worker: focused implementation     │
│  └── Claude worker: expensive reasoning lane  │
└──────────────────────────────────────────────┘
```

Use `ralphglasses_team_create` with `provider` to set the lead's provider, then delegate tasks via `ralphglasses_team_delegate`. Each session tracks costs per-provider in the cost ledger.

## Capability Matrix

The canonical capability registry lives in `internal/session/provider_capabilities.go`. Runtime inspection is available through:

- `ralphglasses_provider_capabilities`
- `ralphglasses_provider_compare`
- `ralphglasses_provider_recommend`

| Capability | Claude | Gemini | Codex |
|------------|--------|--------|-------|
| Budget | Native `--max-budget-usd` | Externally enforced by ralphglasses | Externally enforced by ralphglasses |
| Max turns | Native | Unsupported | Unsupported |
| Agent flag | Native `--agent` | Unsupported | Unsupported |
| Allowed tools | Native | Native but deprecated upstream | Unsupported |
| Worktree | Native | Native | Unsupported |
| Permission mode | Native | Native via `--approval-mode` | Emulated through `--sandbox` |
| Output schema | Native `--json-schema` | Unsupported | Native `--output-schema` |
| MCP server mode | Unsupported | Unsupported | Native `codex mcp-server` |

## CLI Flags

| Flag | Description | Example |
|------|-------------|---------|
| `--bare` | Skip hooks (pre/post-session) for faster startup | `ralphglasses_session_launch --bare` |
| `--effort` | Set effort level: `low`, `medium`, `high`, `max` | `--effort low` (cheaper, faster) |
| `--betas` | Enable beta features (comma-separated) | `--betas compact,streaming` |
| `--fallback-model` | Fallback model when primary is overloaded | `--fallback-model gemini-flash` |

The `--effort` flag maps to provider-specific parameters: Claude's `max_tokens` scaling, Gemini's thinking budget, and OpenAI's `reasoning_effort`.

The `--fallback-model` flag enables overload resilience — if the primary model returns a 529/overloaded error, the session automatically retries with the fallback model.

## Cost Rates

### Model Tiers

Ralphglasses uses 4-tier routing to optimize cost vs capability:

| Tier | Model | Input / 1M tokens | Output / 1M tokens | Use Case |
|------|-------|--------------------|---------------------|----------|
| Ultra-cheap | Gemini Flash-Lite | $0.10 | $0.40 | Classification, routing, simple extraction |
| Worker | Gemini Flash | $0.30 | $1.25 | Bulk code generation, tests, documentation |
| Coding | Claude Sonnet | $3.00 | $15.00 | Architecture, complex refactoring |
| Reasoning | Claude Opus | $15.00 | $75.00 | Planning, multi-step reasoning |

OpenAI: GPT-5.4 Codex is $2.50/$15.00 per 1M tokens (input/output).

### Prompt Caching

All three providers can reuse stable prompt prefixes, but ralphglasses now treats Claude more conservatively:

- **Claude**: Automatic cache_control breakpoints on system prompts and tool definitions
- **Gemini**: Explicit `cachedContents` API with TTL-based cache entries
- **OpenAI**: Automatic prefix caching on Responses API

Guardrail:
- Resumed Claude sessions are treated as cache-unsafe for budget estimation until live cache reads are observed. See [CODEX-REFERENCE.md](CODEX-REFERENCE.md).

### Batch API

The `internal/batch/` package provides 50% discount on non-interactive workloads via provider batch endpoints:

```
Claude:  POST /v1/messages/batches   (up to 10,000 requests)
Gemini:  BatchGenerateContent         (server-side batching)
OpenAI:  POST /v1/batches             (JSONL upload, async completion)
```

Batch jobs are submitted via `ralphglasses_fleet_submit` with `batch: true`. Results are polled and merged back into the session's cost ledger. Best for bulk code review, test generation, and documentation tasks where latency is not critical.

## Per-Provider Config

- `.claude/settings.json` — Claude Code repo-local MCP registration
- `.gemini/settings.json` — Gemini CLI MCP server registration
- `.codex/config.toml` — Codex CLI project config + MCP server registration
- `.mcp.json` — shared source of truth for MCP server command and cwd
- See [PROVIDER-PARITY-OBJECTIVES.md](PROVIDER-PARITY-OBJECTIVES.md) for the broader three-provider parity plan
- See [GEMINI.md](../GEMINI.md) for Gemini-specific instructions
- See [CLAUDE.md](../CLAUDE.md) for Claude-specific instructions
- See [AGENTS.md](../AGENTS.md) for Codex-specific instructions
- See [CODEX-REFERENCE.md](CODEX-REFERENCE.md) for pinned Codex docs and cache-risk notes
- See [CONTRIBUTING.md](../CONTRIBUTING.md) for multi-provider contribution guide
