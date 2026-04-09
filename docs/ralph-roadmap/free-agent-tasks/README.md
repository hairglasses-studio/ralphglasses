# Free Agent Task Assignments

Tasks selected from the ROADMAP for OpenRouter/free-tier agents (Cline with free models).

Each task file contains:
- **Task ID** and ROADMAP reference
- **Description** and acceptance criteria
- **Affected files** to create/modify  
- **Verification command** agents must pass before committing
- **Complexity**: S (small), M (medium)

## Task Queue

| # | File | ROADMAP ID | Title | Priority | Size |
|---|------|-----------|-------|----------|------|
| 1 | `task-01-cycle-baseline.md` | 9.1.5 | cycle_baseline tool | P0 | S |
| 2 | `task-02-finding-to-task.md` | 9.1.1 | finding_to_task converter | P0 | M |
| 3 | `task-03-tranche-receipt.md` | ATD-5 | Tranche receipt emission | P1 | S |
| 4 | `task-04-cycle-schedule.md` | 9.1.4 | cycle_schedule cron trigger | P1 | M |
| 5 | `task-05-wm-dedup.md` | WM-4 | Existing-equivalent detection | P1 | S |
| 6 | `task-06-builtin-workflows.md` | 8.3.2 | Built-in workflow definitions | P1 | M |
| 7 | `task-07-genhandler-extend.md` | 1.2.5.4 | Extend handler codegen tool | P2 | M |
| 8 | `task-08-observation-correlate.md` | 9.2.5 | observation_correlate tool | P1 | M |
| 9 | `task-09-loop-budget-forecast.md` | 9.2.2 | loop_budget_forecast tool | P1 | M |
| 10 | `task-10-relaxed-provider.md` | QW-8 | Fix budget params ignored in tools_session | P0 | S |

## How Agents Should Work

1. Read the task file fully before starting
2. Make changes to the specified files
3. Run `go build ./...` to verify compilation
4. Run the specified test command in the task file
5. Run `make ci` as final gate before committing
