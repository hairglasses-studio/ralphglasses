# Skill Feedback: Prompt Capture Pipeline

**Date**: 2026-04-05
**Author**: Claude Code (Opus 4.6 1M)
**Context**: Building global prompt capture into docs-mcp SQLite DB with 4 MCP analytics tools

---

## 1. prompt-improver binary not on PATH

**Impact**: High
**Affected**: `/prompt-improve` skill, `promptgw` module

The `promptgw` module at `docs/internal/promptgw/module.go` looks for `ralphglasses/bin/prompt-improver`. When the binary isn't built, the entire prompt gateway module silently returns nil tools. The `/prompt-improve` skill degrades without any error surfaced to the user.

**Recommendation**: Log a warning at startup when the binary is missing. Add a `prompt_improver_status` diagnostic tool that reports whether the binary exists and its version. Consider adding `go build ./cmd/prompt-improver/` to the pipeline.mk `build` target.

---

## 2. Skill discovery is repo-scoped

**Impact**: Medium
**Affected**: All skills in `docs/.claude/skills/`

Skills defined in the docs repo are only visible when Claude Code sessions are rooted in `~/hairglasses-studio/docs`. Sessions in other repos (mcpkit, dotfiles, ralphglasses) cannot access `/prompt-metrics`, `/prompt-audit`, `/prompt-improve`, etc. without navigating to docs first.

**Recommendation**: Create a `~/.claude/skills/` directory with symlinks to commonly-used skills across repos. Alternatively, add a `skill_catalog` MCP tool to docs-mcp that lists available skills and their locations, so agents can discover them programmatically.

---

## 3. Hook + skill coordination gap

**Impact**: Medium
**Affected**: Prompt quality feedback loop

The `UserPromptSubmit` hook captures prompts silently, but there's no mechanism for skills to read the most recently captured prompts or trigger quality analysis on them. The capture and analysis paths are disconnected.

**Recommendation**: Add a `prompt_recent` tool that returns the last N captured prompts with their metadata. This enables a `/prompt-review` skill that surfaces quality insights from recent prompts without the user having to search manually.

---

## 4. Intent classification duplication

**Impact**: Low (technical debt)
**Affected**: `prompt-scorecard.sh`, `prompt-capture.sh`, `prompt-improver classify`

Three independent intent classifiers exist:
- `scripts/prompt-scorecard.sh` (lines 21-72): awk-based, 15 categories
- `dotfiles/scripts/lib/prompt-capture.sh`: bash case-match, 15 categories (new)
- `ralphglasses/cmd/prompt-improver`: Go-based, uses LLM or local rules

The taxonomies are similar but not identical. Divergence will cause inconsistent metrics between the scorecard, the DB analytics, and the prompt-improver reports.

**Recommendation**: Define a canonical intent taxonomy in `docs/knowledge/INTENT-TAXONOMY.md` with category names, descriptions, and example first-words. All three classifiers should reference this. The prompt-improver's Go classifier should be the reference implementation.

---

## 5. No prompt-level cost attribution

**Impact**: Medium
**Affected**: Cost analytics, prompt ROI analysis

Session cost data exists in the `sessions` and `cost_registry` tables, but the `prompt_captures` table lacks a `sequence_number` field. This makes it impossible to join a specific prompt to its cost (input/output tokens consumed for that turn).

**Recommendation**: Add `sequence_number INTEGER DEFAULT 0` to `prompt_captures`. The hook script can compute this by counting lines in the transcript JSONL. Combined with `session_id`, this enables per-prompt cost estimation by joining to `session_events`.

---

## 6. sqlite3 CLI portability for hook scripts

**Impact**: Low (but could bite in CI/Docker)
**Affected**: `prompt-capture.sh` SQLite insert

The hook uses the `sqlite3` CLI for direct DB insertion. The `readfile('/dev/stdin')` function (tested during development) is a loadable extension not available on all sqlite3 builds. The fallback single-quote escaping (`sed "s/'/''/g"`) works for most prompts but could fail on prompts with unusual Unicode or control characters.

**Recommendation**: Consider building a tiny Go binary (`prompt-db-insert`) that reads the hook JSON from stdin and performs a proper parameterized insert. This eliminates all SQL injection risk and handles any encoding. The binary could be built alongside docs-mcp and placed in `dotfiles/bin/`.
