# 36 -- Rdcycle Runtime Wiring

Generated: 2026-04-09

## Context

Following the roadmap truth reconciliation, the `rdcycle` implementation was identified as "partially wired" in the autonomous loop paths. The supervisor's generic launch path was using inconsistent cycle objectives and did not share the durable active-job bookkeeping present in the automation-based cycle launches.

## Runtime Unification

The cycle launch paths in `internal/session/supervisor.go` have been refactored to use a consistent, shared internal helper (`executeCycleAsync`). 

**Improvements:**
- All autonomous cycle launches (from the supervisor, the chainer, and the sprint planner) now uniformly update `lastCycleLaunch` and `cyclesLaunched`.
- All cycle launches uniformly publish a `LoopStarted` event with their specific source (`supervisor`, `chainer`, or `sprint_planner`) and objective.
- All launches uniformly track and reset `consecutiveFailures` on cycle success.
- Post-cycle evaluation (such as `gates.Evaluate` and `dl.RecordOutcome`) is now applied uniformly to all autonomous cycles initiated by the supervisor.

## Stable Interfaces

- No new MCP tool names, status schema fields, or provider surface changes were introduced, maintaining backward compatibility.
- Existing cycle and automation status surfaces remain intact, ensuring external tools relying on `supervisor_state.json` or `.ralph/cycles/` do not break.

This narrows the remaining roadmap gaps for `rdcycle` to qualitative improvements like better objective selection and batch-sprint closeout.
