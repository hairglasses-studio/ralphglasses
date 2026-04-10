# Multi-LLM Provider Setup

Sessions can target Claude, Gemini, Codex, or the experimental Antigravity external-manager handoff via the `provider` parameter:

| Provider | CLI Binary | Default Model | Stream Format | Resume Support |
|----------|-----------|---------------|---------------|----------------|
| `codex` (default) | `codex` | `gpt-5.4` | `--json` (NDJSON) | Yes (`exec resume`, when supported by the installed CLI) |
| `claude` | `claude` | `sonnet` | `stream-json` | Yes (`--resume`) |
| `gemini` | `gemini` | `gemini-3.1-pro` | `stream-json` | Yes (`--resume`) |
| `antigravity` | `antigravity` | Antigravity-managed | External interactive handoff | No |

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

# Google Antigravity
# install via the official Google distribution for your platform
# https://antigravity.google/docs/mcp
```

## Environment Variables

Each provider requires its own API key in the environment:

```bash
# .env (loaded via direnv)
ANTHROPIC_API_KEY=sk-ant-...    # Claude
GOOGLE_API_KEY=AIza...          # Gemini
OPENAI_API_KEY=sk-...           # Codex
```

Antigravity itself does not require a separate Ralph-owned API key variable. It inherits provider credentials from the local environment and its own app configuration.

For non-session prompt-enhancement clients only, Ralph can also target a local Ollama-compatible endpoint with:

```bash
OLLAMA_BASE_URL=http://127.0.0.1:11434
OLLAMA_API_KEY=ollama
OLLAMA_CHAT_MODEL=code-primary
OLLAMA_FAST_MODEL=code-fast
OLLAMA_CODE_MODEL=code-primary
OLLAMA_HIGH_CONTEXT_CODE_MODEL=code-long
OLLAMA_CLOUD_CODE_MODEL=glm-5.1:cloud
OLLAMA_CLOUD_VERIFIED_CODE_MODEL=glm-5:cloud
OLLAMA_EMBED_MODEL=nomic-embed-text:v1.5
OLLAMA_KEEP_ALIVE=15m
```

That local path also enables Ralph's optional `ollama` session provider. Claude, Gemini, and Codex remain the normal cloud runtimes, and `ollama` is the explicit local lane backed by the shared `code-*` aliases from `dotfiles`.

The shared workstation service uses a single-user latency profile: Flash
Attention enabled, `q8_0` K/V cache, one loaded model, and one parallel request
lane. Validate it with:

```bash
~/hairglasses-studio/dotfiles/scripts/hg-ollama-smoke.sh
~/hairglasses-studio/dotfiles/scripts/hg-ollama-full-test.sh
```

Repo-owned prompt-improver eval coverage lives under `promptfoo/` and can be
run from the repo root with:

```bash
~/hairglasses-studio/dotfiles/scripts/hg-promptfoo.sh . eval -c promptfoo/promptfooconfig.yaml
```

That suite is intentionally fast and deterministic. It validates the local
prompt-enhancement lane plus one bounded Ollama-backed improvement case,
defaulting that LLM step to the workstation-standard `code-fast` alias unless
`OLLAMA_FAST_MODEL` overrides it. If a slower host needs more time, set
`PROMPTFOO_PROVIDER_TIMEOUT_MS` for that run.

If you want prompt-improver runs to export spans, `ralphglasses` now accepts
either standard OTLP env vars or Langfuse-native env vars:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
OTEL_EXPORTER_OTLP_HEADERS=authorization=Bearer demo-token

# or
LANGFUSE_HOST=https://cloud.langfuse.com
LANGFUSE_PUBLIC_KEY=pk-lf-...
LANGFUSE_SECRET_KEY=sk-lf-...
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

| Capability | Claude | Gemini | Codex | Antigravity |
|------------|--------|--------|-------|-------------|
| Budget | Native `--max-budget-usd` | Externally enforced by ralphglasses | Externally enforced by ralphglasses | Externally enforced by ralphglasses |
| Max turns | Native | Unsupported | Unsupported | Unsupported |
| Agent flag | Native `--agent` | Unsupported | Unsupported | Unsupported |
| Allowed tools | Native | Native but deprecated upstream | Unsupported | Unsupported |
| Worktree | Native | Native | Unsupported | Unsupported |
| Permission mode | Native | Native via `--approval-mode` | Emulated through `--sandbox` | Unsupported |
| Output schema | Native `--json-schema` | Unsupported | Native `--output-schema` | Unsupported |
| MCP server mode | Unsupported | Unsupported | Native `codex mcp-server` | Unsupported |

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
- `.agents/rules/` and `.agents/workflows/` — generated Antigravity/Gemini repo-native rule and slash-command surfaces
- `.gemini/extensions/ralphglasses-workspace/` — generated thin extension bundle for Gemini and Antigravity
- See [PROVIDER-PARITY-OBJECTIVES.md](PROVIDER-PARITY-OBJECTIVES.md) for the broader provider parity plan and Antigravity constraints
- See [GEMINI.md](../GEMINI.md) for Gemini-specific instructions
- See [CLAUDE.md](../CLAUDE.md) for Claude-specific instructions
- See [AGENTS.md](../AGENTS.md) for Codex-specific instructions
- See [CODEX-REFERENCE.md](CODEX-REFERENCE.md) for pinned Codex docs and cache-risk notes
- See [CONTRIBUTING.md](../CONTRIBUTING.md) for multi-provider contribution guide
