# 36 -- Rdcycle Runtime Wiring Addendum

Generated: 2026-04-09
Scope: Documenting the completion of the rdcycle runtime wiring tranche.

## Shipped Changes
- Refactored the generic cycle launch path in `internal/session/supervisor.go` to carry explicit cycle names, objectives, success criteria, and max task counts.
- Updated `SupervisorState` with an `ActiveJob` field to match the automation cycle-launch plumbing.
- `executeCycleAsync` now properly tracks the active job state and resets it upon cycle completion, persisting these changes via `s.persistState()`.

## Next Steps
- Close the batch sprint and merge-path gaps.
- Schedule marathon closeout.
- Cost-aware autonomy refinement.
