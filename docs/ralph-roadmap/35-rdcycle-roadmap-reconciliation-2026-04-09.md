# 35 -- Rdcycle Roadmap Reconciliation

Generated: 2026-04-09

## Context

The roadmap previously listed the `rdcycle` (R&D cycle orchestrator) MCP handlers and tools as missing. However, they were already shipped and merged to `main` prior to this addendum. The autonomous paths are partially wired but lack the final batch-sprint closeouts and qualitative tuning.

## Roadmap Updates

The `ROADMAP.md` file has been updated to explicitly reflect the current `rdcycle` reality. 

**Shipped Now:**
- `rdcycle` MCP handlers, builders, and tests
- Cycle runtime primitives and the underlying cycle state machine
- TUI dashboard for iteration history and phase visibility

**Partially Wired:**
- Supervisor trigger hooks for cycles
- Sprint planner execution loops
- Subscription automation cycle launches

**Remaining Gaps:**
- Better objective selection quality
- Durable status/writeback polish
- Merge-path and batch-sprint closeout
- Scheduled marathon closeout
- Cost-aware autonomy refinement

This updated context allows autonomous loops to target specific remaining gaps (such as runtime wiring or objective tuning) without re-implementing existing primitive infrastructure.
