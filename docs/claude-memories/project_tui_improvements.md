---
name: TUI overhaul plan — 4-phase architecture upgrade
description: Comprehensive TUI refactor plan from Sprint 7 audit: async I/O, model decomposition, modal stack, responsive columns, view interface, multi-agent views
type: project
---

4-phase TUI architecture upgrade. Phase 1 COMPLETE (2026-03-28). Phase 2 next.

**Why:** TUI grew to 18 views and 70+ Model fields but architecture was designed for 5-6 views. Monolithic Model, blocking I/O, hardcoded dimensions, no modal stack, scattered state.

**How to apply:** Follow the phased plan in `.claude/plans/purring-coalescing-meadow.md`.

## Phase 1: Foundation Fixes (COMPLETE 2026-03-28)
- 1.1 Async I/O — wrap ReadFullLog in tea.Cmd, LogLoadedMsg handler (DONE)
- 1.2 Model decomposition — extract Nav, Sel, Modals, Stream, Cache, Fleet sub-structs (DONE)
- 1.3 Modal interface + ModalStack with push/pop (DONE)
- 1.4 Responsive table columns with flex weights (DONE)
- 1.5 SetViewContext allowlist rewrite (DONE)
- teatest integration tests — 4 golden file snapshots + 3 interactive flow tests (DONE)

## Phase 2: View System Upgrade (IN PROGRESS — started 2026-03-28)
- 2.1 View interface abstraction — DONE (Render/SetDimensions, `views/view.go`)
- 2.2 Viewport-based rendering — 3/18 views migrated (Help, RepoDetail, LoopHealth)
- Mouse support foundation — not started
- Global search/filter — not started

## Phase 3: Multi-Agent Control Views
- Team orchestration view (tree + delegation feed)
- Task queue visibility
- Agent composition UI
- R&D cycle dashboard

## Phase 4: Polish & Performance
- Virtual scrolling, selective tick refresh, theme switching, debug overlay

## Key Architecture Decisions
- Modal interface: `IsActive()`, `Deactivate()`, `HandleKey()`, `View()` with ModalStack
- Sub-structs use short names: `m.Nav`, `m.Sel`, `m.Modals`, `m.Stream`, `m.Cache`, `m.Fleet`
- SetViewContext inverted to allowlist (each view declares enabled bindings)
- Column flex weights: MinWidth, MaxWidth, Flex float64 on Column struct
- teatest golden files in `internal/tui/testdata/`, update with `go test ./internal/tui -run TestTeatest -args -update`
