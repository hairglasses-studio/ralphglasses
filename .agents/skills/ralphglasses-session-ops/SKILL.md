---
name: ralphglasses-session-ops
description: Operate sessions, teams, loops, budgets, and fleet-facing session workflows across Claude, Gemini, and Codex.
---

# Ralphglasses Session Ops

Use this skill for active session and loop orchestration.

## Default workflow

1. Inspect current session and budget state:
   - `ralphglasses_session_list`
   - `ralphglasses_session_status`
   - `ralphglasses_budget_status`
   - `ralphglasses_fleet_analytics`
2. Launch or resume execution:
   - `ralphglasses_session_launch`
   - `ralphglasses_session_resume`
   - `ralphglasses_team_create`
   - `ralphglasses_team_delegate`
3. Manage loop execution:
   - `ralphglasses_loop_start`
   - `ralphglasses_loop_step`
   - `ralphglasses_loop_status`
   - `ralphglasses_loop_gates`
4. Close out or compare results:
   - `ralphglasses_session_compare`
   - `ralphglasses_session_export`
   - `ralphglasses_session_stop`

## Best-fit cases

- Provider/session orchestration
- Team creation and delegation
- Planner/worker/verifier loops
- Budget pressure checks
- Session comparison and export

## Guardrails

- Inspect budget and status before launching more work.
- Use teams only when the subtask split is real.
- Prefer loop gates and compare surfaces before calling work complete.
