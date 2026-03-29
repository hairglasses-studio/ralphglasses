---
name: monitor-supervisor
description: Monitor the autonomous R&D supervisor with stall/runaway detection and structured escalation
user-invocable: true
argument-hint: [repo-path] [poll-interval] [max-polls]
---

You are monitoring the autonomous R&D supervisor for ralphglasses. You will poll status at regular intervals, detect anomalies, and escalate when thresholds are breached. Follow the protocol below.

## 1. Parse Arguments

Extract from user input (defaults in parentheses):
- `repo_path`: Path to the repo (default: current working directory)
- `poll_interval`: Time between checks, e.g. `5m`, `2m` (default: `5m`)
- `max_polls`: Stop monitoring after N polls (default: `20`)

## 2. Initial Status Check

Gather baseline state by calling these tools:

```
ralphglasses_supervisor_status repo=<repo_path>
ralphglasses_autonomy_decisions repo=<repo_path>
ralphglasses_observation_summary repo=<repo_path>
```

If supervisor is not running, report that and exit — nothing to monitor.

Record baseline: `tick_count`, `last_cycle_launch`, `started_at`, decision count, total cost.

## 3. Polling Loop

For each poll iteration (1 to max_polls):

### 3a. Gather Metrics

Call the same 3 tools as step 2. Extract:

| Metric | Source |
|--------|--------|
| `tick_count` | supervisor_status |
| `last_cycle_launch` | supervisor_status |
| `running` | supervisor_status |
| `decision_count` | autonomy_decisions |
| `total_cost_usd` | observation_summary |
| `completion_rate` | observation_summary |
| `cost_rate_per_hour` | observation_summary |

### 3b. Detect Anomalies

Check each condition and assign alert level:

**YELLOW alerts** (warn, continue monitoring):
- `tick_count` unchanged for 2+ consecutive polls → STALL
- No cycle launch in 2+ hours despite supervisor running → IDLE
- `completion_rate` below 50% → DEGRADED
- `cost_rate_per_hour` between 1x and 2x threshold ($5-$10/hr) → COST_ELEVATED

**RED alerts** (recommend shutdown):
- `tick_count` unchanged for 3+ consecutive polls → STALL_CRITICAL
- `cost_rate_per_hour` above 2x threshold (>$10/hr) → COST_RUNAWAY
- 3+ consecutive failed cycles (completion_rate = 0 over recent window) → FAILURE_CASCADE
- Total cost exceeds 80% of budget cap → BUDGET_NEAR_LIMIT
- Supervisor stopped unexpectedly (was running, now not) → UNEXPECTED_STOP

### 3c. Print Status Table

```
=== Supervisor Monitor [poll N/max_polls] ===
Status:      RUNNING | STOPPED | TERMINATED
Tick count:  <n> (+<delta> since last poll)
Last cycle:  <timestamp> (<duration> ago)
Cost:        $<total> ($<rate>/hr)
Completion:  <rate>%
Decisions:   <count> total
Alert level: GREEN | YELLOW | RED
Alerts:      <list of active alerts, or "none">
================================================
```

### 3d. Escalate if Needed

**On YELLOW**: Print the alert details inline. Continue monitoring.

**On RED**: Print alert details prominently, then recommend:
```
RED ALERT: <alert_name>
Recommended action: Emergency stop
  ralphglasses_autonomy_level set=0 repo=<repo_path>
```

Ask the user whether to execute the emergency stop or continue monitoring.

### 3e. Check Exit Conditions

Stop the monitoring loop if ANY of:
- Supervisor is no longer running (normal termination)
- `max_polls` reached
- User requests stop
- RED alert and user confirms shutdown

## 4. Final Report

After the loop exits, print a summary:

```
=== Monitor Session Complete ===
Duration:     <total monitoring time>
Polls:        <count>
Cycles seen:  <cycles launched during monitoring>
Total cost:   $<total>
Final status: <RUNNING | STOPPED | TERMINATED>
Alerts fired: <count yellow> yellow, <count red> red
Exit reason:  <why monitoring stopped>
================================
```

## 5. Post-Monitor Verification

If supervisor terminated normally (max cycles/duration/budget reached):
1. Run `go test ./... -count=1 -timeout 120s` to verify no regressions
2. Check `ralphglasses_cycle_list repo=<repo_path>` for completed cycles with synthesis
3. Report how many cycles completed successfully vs failed

If supervisor was emergency-stopped:
1. Run the same test suite
2. Check `git diff --stat` for any uncommitted changes from interrupted cycles
3. Recommend cleanup steps if needed

## Alert Thresholds Reference

| Alert | Condition | Level |
|-------|-----------|-------|
| STALL | tick_count unchanged 2 polls | YELLOW |
| STALL_CRITICAL | tick_count unchanged 3 polls | RED |
| IDLE | No cycle launch in 2h | YELLOW |
| DEGRADED | completion_rate < 50% | YELLOW |
| COST_ELEVATED | cost_rate $5-$10/hr | YELLOW |
| COST_RUNAWAY | cost_rate > $10/hr | RED |
| FAILURE_CASCADE | 3+ consecutive failures | RED |
| BUDGET_NEAR_LIMIT | cost > 80% of cap | RED |
| UNEXPECTED_STOP | running→stopped unexpectedly | RED |
