# Ralphglasses — Codex CLI Instructions — Gemini CLI Instructions

This repo uses [AGENTS.md](AGENTS.md) as the canonical instruction file.

- Read [AGENTS.md](AGENTS.md) first for build, test, architecture, and repo conventions.
- Treat [GEMINI.md](GEMINI.md) as a compatibility mirror, not the primary source of truth.

## Summary

> Canonical instructions: AGENTS.md

## Gemini Notes

- Native Gemini subagents live in `.gemini/agents/*.md`.
- `.gemini/commands/` is reserved for shortcut and compatibility prompts, not reusable fleet role definitions.
- Shared reusable fleet roles live in `.agents/roles/*.json` and shared workflows live in `.agents/skills/`.
- Gemini parity in this repo is native-first: local subagents, remote A2A agents, skills, and extensions are all first-class fleet surfaces.
- Regenerate native Gemini role projections with `python3 scripts/sync-provider-roles.py`.
