---
name: Perpetual R&D cycle — current phase definition
description: Active R&D cycle objectives, success criteria, scope, and key experiments for the concentric cycle framework + TUI Phase 2-3
type: project
---

Defined 2026-03-28. Updated 2026-03-28 (third R&D cycle sprint).

**Objective:** Run perpetual R&D cycles — state machine drives autonomous propose→execute→observe→synthesize loops.

**Why:** Transition from sequential sprints (1→7) to continuous concentric R&D where research feeds development feeds research.

**How to apply:** Cycle engine is feature-complete with RunCycle executor and dashboard view. Next: run first autonomous end-to-end cycle, then Phase 2.3 (mouse support) and Phase 3 views.

## Success Criteria (All DONE)
1. R&D cycle state machine — DONE
2. Cycle executor (RunCycle) — DONE
3. MCP tool wiring (17 rdcycle tools) — DONE
4. R&D cycle dashboard view — DONE (Phase 3.4)
5. Signal:killed fix — DONE (classification, 15s grace, auto-recovery, memory pressure check)
6. Deterministic golden files — DONE (NowFunc, empty ScanPath, StatusBar sync)
7. TUI View interface + 12 views migrated — DONE (11 ViewportView + 1 RDCycleView)
8. All tests green + stable — DONE (37/37, golden files 10/10 at count=10)

## Accomplished (2026-03-28, cycle 3)
- R&D Cycle Dashboard: 5-panel view (active cycle, tasks, findings, synthesis, history)
- Signal classification: ClassifyExitSignal (OOM/timeout/user/budget), FormatKillError
- Kill grace period 5s→15s, auto-recovery for killed sessions
- Memory pressure check before loop iterations with forced GC
- Fixed flaky golden files: StatusBar.LastRefresh in View(), skip scanRepos when empty, NowFunc

## Remaining Work
- Run first autonomous cycle via `cycle_run` tool
- Phase 2.3: Mouse support (click-to-select tables, tab bar, modals)
- Phase 2.4: Global search (Ctrl+/)
- Phase 3.1: Team Orchestration view
- Phase 3.2: Task Queue view
- Phase 3.3: Agent Composition UI
- Phase 4: Virtual scrolling, selective tick, themes, debug overlay

## Current State
- Commit: `e7699c4` on main, pushed
- 37 packages, 17 rdcycle MCP tools, 19 TUI views (12 migrated), ~86% coverage
- Cycle engine fully operational: types + persistence + pipeline + Manager bridge + MCP handlers + RunCycle executor + dashboard
- Golden files fully deterministic
