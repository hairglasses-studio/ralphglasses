# Ralphglasses — Codex CLI Instructions — Gemini CLI Instructions

This repo uses [AGENTS.md](AGENTS.md) as the canonical instruction file.

- Read [AGENTS.md](AGENTS.md) first for build, test, architecture, and repo conventions.
- Repo-local Gemini config lives in `.gemini/settings.json`; command/`cwd` must stay aligned with `.mcp.json`.
- Repo-local Gemini command surfaces live in `.gemini/commands/*.toml`; optional extension metadata lives in `.gemini/extensions/`.
- Gemini-native controls in this repo are `--approval-mode`, `--worktree`, `--resume`, and `--allowed-tools` (deprecated upstream but still supported by the installed CLI).
- Gemini review automation lives in `.github/workflows/gemini-review.yml` and `.github/workflows/gemini-security.yml`.
- Cross-provider capability rules live in [docs/PROVIDER-PARITY-OBJECTIVES.md](docs/PROVIDER-PARITY-OBJECTIVES.md).

## Summary

> Canonical instructions: AGENTS.md
