---
name: start-supervisor
description: Start the autonomous R&D supervisor with pre-flight checks, budget caps, and termination conditions
user-invocable: true
argument-hint: [repo-path] [budget-usd] [max-cycles] [max-duration]
---

You are starting the autonomous R&D supervisor for ralphglasses. The supervisor monitors repo health, proposes decisions via the audited DecisionLog, and launches R&D cycles automatically. Follow every step below in order.

## 1. Parse Arguments

Extract from user input (defaults in parentheses):
- `repo_path`: Path to the repo (default: current working directory)
- `budget_usd`: Maximum total spend before hard stop (default: `5.00`)
- `max_cycles`: Maximum cycles to launch before stopping (default: `10`)
- `max_duration`: Maximum wall-clock time, e.g. `2h`, `30m` (default: `2h`)

## 2. Pre-flight Checks

Run all three checks. If ANY fails, stop and report — do not start the supervisor.

1. **Build check**: `go build ./...` — must exit 0
2. **Test check**: `go test ./... -count=1 -timeout 120s` — must exit 0
3. **Git state**: `git status --short` — report if dirty (warn, don't block)

## 3. Configure Budget

Set the fleet budget cap to prevent runaway spend:

```
ralphglasses_fleet_budget action=set budget_usd=<budget_usd> repo=<repo_path>
```

## 4. Activate Autonomy Level 2

```
ralphglasses_autonomy_level set=2 repo=<repo_path>
```

This starts the supervisor goroutine with:
- `MaxCycles = <max_cycles>`
- `MaxTotalCostUSD = <budget_usd>`
- `MaxDuration = <max_duration>`
- Default health thresholds (70% completion rate, $5/hr cost cap, 80% verify pass rate, 1h idle trigger)

## 5. Verify Running

Poll supervisor status to confirm it started:

```
ralphglasses_supervisor_status repo=<repo_path>
```

Confirm `running: true`. If not running after 5s, report the error and set autonomy back to 0.

## 6. Report Start

Print a structured status block:

```
Supervisor Started
  Repo:         <repo_path>
  Budget cap:   $<budget_usd>
  Max cycles:   <max_cycles>
  Max duration: <max_duration>
  Tick interval: 60s
  Cooldown:     5m between cycles

Termination: Supervisor will auto-stop when ANY condition is met:
  - Budget exhausted ($<budget_usd>)
  - Max cycles reached (<max_cycles>)
  - Max duration elapsed (<max_duration>)

Use /monitor-supervisor to watch progress.
Use `ralphglasses_autonomy_level set=0` to emergency stop.
```

## Scope Boundaries

The supervisor CAN:
- Launch R&D cycles from ROADMAP.md items
- Chain cycle synthesis into follow-up cycles
- Run `go test` as self-test
- Consolidate journal patterns
- Adjust internal budget allocation
- Publish events to the event bus

The supervisor CANNOT:
- Push to remote repositories
- Modify CI/CD pipelines
- Change its own autonomy level
- Delete files outside `.ralph/`
- Make network requests beyond LLM API calls

## Success Criteria

The start is successful when:
1. Pre-flight checks pass (build + tests green)
2. Autonomy level is set to 2
3. Supervisor status shows `running: true`
4. At least 1 tick completes (visible via tick_count > 0)
