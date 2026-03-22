# Top-10 Integration Candidates from Awesome-Claude-Code

Prioritized by direct value to ralph with minimal dev cycles.

---

## 1. `ryoppippi/ccusage` — Cost Tracking (drop-in)

**What**: Parses Claude Code JSONL session logs for token counts and cost data.
**Integration**: Port JSONL parsing into `session/budget.go`. Replace estimated costs with actual Claude-reported token usage.
**Effort**: ~2 hours. Read their parser, adapt to our `LedgerEntry` type.

## 2. `avifenesh/agnix` — Config Linter (drop-in)

**What**: 342-rule linter for CLAUDE.md, hooks, MCP configs.
**Integration**: Run as pre-session hook or integrate scoring into `repo_health`. Surface findings in fleet dashboard.
**Effort**: ~1 hour. Install binary, wire into `handleRepoHealth`.

## 3. `vaporif/parry` — Prompt Security (drop-in)

**What**: DeBERTa v3 prompt injection scanner with 7-layer security pipeline.
**Integration**: Pre-session hook that scans prompts before `session_launch`. Blocks injection attempts in team_delegate tasks.
**Effort**: ~2 hours. Hook integration + threshold config in `.ralphrc`.

## 4. `GowayLee/cchooks` — Hook SDK (drop-in)

**What**: Typed hook SDK with budget enforcement, stop, and lifecycle hooks.
**Integration**: Reference for building Go-native hook system. Their budget hook pattern maps to `session/budget.go` guardrails.
**Effort**: ~3 hours. Study patterns, implement Go equivalents.

## 5. `smtg-ai/claude-squad` — Worktree + TUI Patterns (moderate)

**What**: Closest architectural cousin. Go/Bubbletea TUI with worktree isolation and profile system.
**Integration**: Study their worktree manager for `session_launch --worktree`. Their profile system = our agent definitions. May share Bubbletea components.
**Effort**: ~4 hours. Code review + selective port.

## 6. `obra/superpowers` — Skill Auto-Triggering (drop-in)

**What**: Auto-triggers skills based on context, mandatory TDD, worktree isolation.
**Integration**: Skill triggering pattern for `workflow_run` steps. TDD gate = loop verification step.
**Effort**: ~2 hours. Port triggering logic to workflow engine.

## 7. `trailofbits/skills` — Security Skills + Multi-Provider (drop-in)

**What**: Security-focused skills. `second-opinion` skill invokes other LLM CLIs for cross-validation.
**Integration**: Validates our multi-provider concept. Install skills directly, reference `second-opinion` for team_delegate cross-provider verification.
**Effort**: ~1 hour. Copy skills to `.claude/skills/`, study cross-provider invocation.

## 8. `phiat/claude-esp` — Session Monitoring TUI (moderate)

**What**: Go/Bubbletea/Lipgloss JSONL session monitoring with fsnotify and tree view.
**Integration**: Port session monitoring view to our TUI. Their fsnotify+JSONL pattern matches our reactive update architecture.
**Effort**: ~4 hours. Component port + integration.

## 9. `Piebald-AI/tweakcc` — MCP Startup Optimization (drop-in)

**What**: ~50% faster MCP startup, auto-accept plan mode for unattended operation.
**Integration**: Apply optimization to `session_launch`. Auto-accept plan mode critical for marathon loops.
**Effort**: ~1 hour. Config changes + session launch flags.

## 10. `hagan/claudia-statusline` — Cost Tracking in Go (drop-in)

**What**: Go-native statusline with SQLite cost tracking and burn rate math.
**Integration**: Burn rate calculation directly applicable to `fleet_analytics`. SQLite schema reference for persistent cost storage.
**Effort**: ~2 hours. Port burn rate math to `session/budget.go`.

---

## Summary

| # | Repo | Type | Hours | Impact |
|---|------|------|-------|--------|
| 1 | ccusage | drop-in | 2 | Real cost data in budget system |
| 2 | agnix | drop-in | 1 | Config quality in repo_health |
| 3 | parry | drop-in | 2 | Prompt security for unattended loops |
| 4 | cchooks | drop-in | 3 | Budget enforcement hooks |
| 5 | claude-squad | moderate | 4 | Worktree isolation + TUI patterns |
| 6 | superpowers | drop-in | 2 | Skill triggering + TDD gate |
| 7 | trailofbits/skills | drop-in | 1 | Security skills + cross-provider |
| 8 | claude-esp | moderate | 4 | Session monitoring TUI |
| 9 | tweakcc | drop-in | 1 | 50% faster MCP startup |
| 10 | claudia-statusline | drop-in | 2 | Burn rate math for analytics |

**Total estimated effort**: ~22 hours for all 10.
**Recommended first sprint**: #1, #2, #3, #9 (6 hours, highest ROI).
