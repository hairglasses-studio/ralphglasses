# Ralphglasses — Codex CLI Instructions — Claude Code Instructions

This repo uses [AGENTS.md](AGENTS.md) as the canonical instruction file. Read it before making changes.

## Claude Notes

- Use [AGENTS.md](AGENTS.md) for build, test, architecture, and repo-specific conventions.
- Repo-local MCP registration lives in `.claude/settings.json` and must keep the same wrapper command and `cwd` as `.mcp.json`.
- Claude-native controls in this repo are `--max-budget-usd`, `--max-turns`, `--agent`, `--allowedTools`, `--permission-mode`, and `--json-schema`.
- Claude review automation lives in `.github/workflows/claude-review.yml` and `.github/workflows/claude-security.yml`.
- Cross-provider capability rules live in [docs/PROVIDER-PARITY-OBJECTIVES.md](docs/PROVIDER-PARITY-OBJECTIVES.md).

## Summary

> Canonical instructions: AGENTS.md
