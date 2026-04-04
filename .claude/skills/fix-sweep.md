---
name: fix-sweep
description: Launch a cross-repo fix sweep targeting audit findings, with auto permission mode, monitoring, and push
user-invocable: true
argument-hint: [repos|"active"|"all"] [interval-minutes] [max-budget]
---

You are running a cross-repo fix sweep to address audit findings deposited in `.claude/audit-2026-04-03.md` files across repos. This chains prompt generation, parallel fix sessions, monitoring, and push into a single workflow.

## 1. Parse Arguments

Extract from user input (defaults in parentheses):
- `repos`: JSON array of repo names, `"active"`, or `"all"` (default: `"active"`)
- `interval_minutes`: Status check interval in minutes (default: `5`)
- `max_budget`: Total sweep budget cap (default: `200`)
- `limit`: Max repos (default: `10`)
- `model`: Model to use (default: `opus`)

## 2. Generate the Fix Prompt

```
ralphglasses_sweep_generate task_type=fix target_provider=claude
```

Print the quality score. The fix template instructs sessions to read `.claude/audit-2026-04-03.md`, fix all items in priority order, build/test after each fix, and commit individually.

## 3. Launch the Fix Sweep

```
ralphglasses_sweep_launch prompt=<generated_prompt> repos=<repos> limit=<limit> model=<model> permission_mode=auto enhance_prompt=none max_sweep_budget_usd=<max_budget> max_turns=100 session_persistence=false
```

Note: `permission_mode=auto` allows sessions to edit files, run tests, and commit. `enhance_prompt=none` because the fix template is already optimized.

Capture the `sweep_id`.

## 4. Set Up Monitoring

```
ralphglasses_sweep_schedule sweep_id=<sweep_id> interval_minutes=<interval_minutes> auto_nudge=true max_sweep_budget_usd=<max_budget>
```

## 5. Wait and Monitor

Periodically check status:

```
ralphglasses_sweep_status sweep_id=<sweep_id> verbose=true
```

Print a dashboard table showing per-repo progress.

## 6. Generate Report

After all sessions complete:

```
ralphglasses_sweep_report sweep_id=<sweep_id> format=markdown
```

Print the full report showing commits, costs, and changes per repo.

## 7. Push All Repos

```
ralphglasses_sweep_push sweep_id=<sweep_id> dry_run=true
```

Show what would be pushed. If the user approves:

```
ralphglasses_sweep_push sweep_id=<sweep_id>
```

## 8. Retry Failed Sessions (if any)

If any sessions errored:

```
ralphglasses_sweep_retry sweep_id=<sweep_id>
```

Then re-run monitoring and report for the retried sessions.
