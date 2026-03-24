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

## Per-Provider Config

- `.gemini/settings.json` — Gemini CLI MCP server registration
- `.codex/config.toml` — Codex CLI project config + MCP server registration
- See [GEMINI.md](../GEMINI.md) for Gemini-specific instructions
- See [AGENTS.md](../AGENTS.md) for Codex-specific instructions
- See [CONTRIBUTING.md](../CONTRIBUTING.md) for multi-provider contribution guide
