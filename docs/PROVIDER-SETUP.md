# Multi-LLM Provider Setup

Sessions can target any of three providers via the `provider` parameter:

| Provider | CLI Binary | Default Model | Stream Format | Resume Support |
|----------|-----------|---------------|---------------|----------------|
| `claude` (default) | `claude` | `sonnet` | `stream-json` | Yes (`--resume`) |
| `gemini` | `gemini` | `gemini-3-pro` | `stream-json` | Yes (`--resume`) |
| `codex` | `codex` | `gpt-5.4-xhigh` | quiet mode | No |

## Prerequisites

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

## Environment Variables

Each provider requires its own API key in the environment:

```bash
# .env (loaded via direnv)
ANTHROPIC_API_KEY=sk-ant-...    # Claude
GOOGLE_API_KEY=AIza...          # Gemini
OPENAI_API_KEY=sk-...           # Codex
```

## Orchestration Pattern

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

All three providers support prompt caching for 80-90% input cost savings on repeated prefixes:

- **Claude**: Automatic cache_control breakpoints on system prompts and tool definitions
- **Gemini**: Explicit `cachedContents` API with TTL-based cache entries
- **OpenAI**: Automatic prefix caching on Responses API

### Batch API

The `internal/batch/` package provides 50% discount on non-interactive workloads via provider batch endpoints:

```
Claude:  POST /v1/messages/batches   (up to 10,000 requests)
Gemini:  BatchGenerateContent         (server-side batching)
OpenAI:  POST /v1/batches             (JSONL upload, async completion)
```

Batch jobs are submitted via `ralphglasses_fleet_submit` with `batch: true`. Results are polled and merged back into the session's cost ledger. Best for bulk code review, test generation, and documentation tasks where latency is not critical.

## Per-Provider Config

- `.gemini/settings.json` — Gemini CLI MCP server registration
- `.codex/config.toml` — Codex CLI project config + MCP server registration
- See [GEMINI.md](../GEMINI.md) for Gemini-specific instructions
- See [AGENTS.md](../AGENTS.md) for Codex-specific instructions
- See [CONTRIBUTING.md](../CONTRIBUTING.md) for multi-provider contribution guide
