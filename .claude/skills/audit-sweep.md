---
name: audit-sweep
description: Generate an optimized audit prompt, launch it across multiple repos with Opus 4.6 1M in plan mode, and monitor progress with auto-nudge
user-invocable: true
argument-hint: [repos|"active"|"all"] [interval-minutes] [auto-nudge]
---

You are running a cross-repo audit sweep using the ralphglasses sweep tools. This chains prompt generation, parallel session launch, and recurring monitoring into a single workflow.

## 1. Parse Arguments

Extract from user input (defaults in parentheses):
- `repos`: JSON array of repo names, `"active"`, or `"all"` (default: `"active"`)
- `interval_minutes`: Status check interval in minutes (default: `5`)
- `auto_nudge`: Whether to auto-restart stalled sessions (default: `true`)
- `task_type`: Type of audit — `audit`, `review`, or `improve` (default: `audit`)
- `limit`: Max repos to target (default: `10`)
- `model`: Model to use (default: `opus`)

## 2. Generate the Prompt

Call the sweep generate tool to create an optimized prompt:

```
ralphglasses_sweep_generate task_type=<task_type> target_provider=claude
```

Capture the returned `prompt` and `quality_score`. If score is below 70, warn the user.

Print:
```
Prompt quality: <score>/100 (grade <grade>)
Stages applied: <stages_run>
Estimated tokens: <estimated_tokens>
```

## 3. Launch the Sweep

Use the generated prompt to launch across target repos:

```
ralphglasses_sweep_launch prompt=<generated_prompt> repos=<repos> limit=<limit> model=<model> permission_mode=plan enhance_prompt=local
```

Capture the `sweep_id` and `task_id` from the response.

Print:
```
Sweep launched: <sweep_id>
Targeting <N> repos
Task ID: <task_id>
```

Wait 10 seconds for sessions to initialize, then do an initial status check:

```
ralphglasses_sweep_status sweep_id=<sweep_id>
```

Print the initial status table.

## 4. Set Up Monitoring

Start the recurring status monitor:

```
ralphglasses_sweep_schedule sweep_id=<sweep_id> interval_minutes=<interval_minutes> auto_nudge=<auto_nudge>
```

Capture the monitoring `task_id`.

Print:
```
Monitor scheduled: checking every <interval_minutes>m
Auto-nudge: <enabled|disabled>
Task ID: <task_id> (cancel with ralphglasses_tasks_cancel)
```

## 5. First Status Report

Call status one more time and print a dashboard:

```
=== Audit Sweep Dashboard ===
Sweep ID:    <sweep_id>
Total repos: <total>
Running:     <running>
Completed:   <completed>
Stalled:     <stalled>
Errored:     <errored>
Completion:  <completion_pct>%
Total cost:  $<total_cost_usd>

Per-repo status:
| Repo | Status | Turns | Cost | Idle |
|------|--------|-------|------|------|
| ...  | ...    | ...   | ...  | ...  |
==============================
```

## 6. Manual Nudge (if needed)

If any sessions appear stalled (idle > 5 min), offer to nudge them:

```
ralphglasses_sweep_nudge sweep_id=<sweep_id> stale_threshold_min=5 action=restart
```

Report how many sessions were restarted.

## 7. Monitoring Commands Reference

Provide the user with commands they can run later:

```
# Check sweep status
ralphglasses_sweep_status sweep_id=<sweep_id> verbose=true

# Nudge stalled sessions
ralphglasses_sweep_nudge sweep_id=<sweep_id>

# Cancel the monitoring schedule
ralphglasses_tasks_cancel id=<monitor_task_id>

# View individual session output
ralphglasses_session_tail id=<session_id>

# View audit report for a repo
cat ~/hairglasses-studio/<repo>/.claude-audit-report.md
```
