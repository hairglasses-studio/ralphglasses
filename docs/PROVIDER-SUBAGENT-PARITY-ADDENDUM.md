# Provider Subagent Parity Addendum

Status: 2026-04-08.

This file supersedes the old assumption that Gemini is limited to `.gemini/settings.json` plus command shortcuts.

## Current Baseline

- Codex native reusable subagents: `.codex/agents/*.toml`
- Claude native reusable agents: `.claude/agents/*.md`
- Gemini native reusable local subagents: `.gemini/agents/*.md`
- Gemini remote subagents: A2A remote-agent definitions
- Gemini reusable workflow assets: native skills and extension-bundled skills

## Practical Consequences

- `ralphglasses` parity should normalize fleet intent, not fake identical CLI flags.
- `agent` flag parity is still intentionally asymmetric: Claude has a native flag, Codex and Gemini rely on repo-native agent surfaces instead.
- `subagents` parity is now native across all three primary providers.
- `.gemini/commands/` remains valid as a shortcut surface, but it should not be the canonical reusable-role source.

## Recommended Source Of Truth

- Canonical workflows: `.agents/skills/`
- Canonical reusable roles: `.agents/roles/*.json`
- Native projections:
  - `.codex/agents/*.toml`
  - `.claude/agents/*.md`
  - `.gemini/agents/*.md`
