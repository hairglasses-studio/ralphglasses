---
name: Ralph scratchpad locations
description: Historical R&D cycle scratchpads in .ralph/ — reference for studying sprint outcomes and self-improvement patterns
type: reference
---

`.ralph/` contains historical scratchpad files from R&D cycles and improvement runs. These are append-only logs — not actively maintained, but valuable for studying patterns.

**Key scratchpads:**
- `tool_improvement_scratchpad.md` (1942 lines) — Master log of tool, wiring, and workflow improvement opportunities from Sprints 3-7
- `test_run_scratchpad.md` (521 lines) — Self-learning test runs with gate results and merge tracking
- `cycle14_production_readiness_scratchpad.md` (477 lines) — Fleet baseline snapshot + production readiness audit
- `cycle15_tool_exploration_scratchpad.md` (374 lines) — Tool exploration and scratchpad gap analysis
- `research-audit_scratchpad.md` (334 lines) — arXiv research queries for roadmap synthesis
- `fleet_audit_scratchpad.md` (252 lines) — Fleet state discovery and triage across 7 repos
- `sprint7_audit_report.md` — Latest audit: 5 blockers, 16 warnings across 6 dimensions

**Machine-readable data:**
- `cost_observations.json` — Per-iteration cost tracking
- `improvement_patterns.json` — Learned improvement patterns
- `loop_baseline.json` — Performance baselines (P50/P95)

**Skill files (`.claude/skills/`):**
- `parallel-roadmap-sprint.md` through `parallel-roadmap-sprint-7.md` — Sprint 1-7 definitions (scope, workstreams, targets)
