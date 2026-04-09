# Task 04: cycle_schedule — Cron-Triggered R&D Cycles

**ROADMAP ID**: 9.1.4  
**Priority**: P1 | **Size**: M  
**Assigned to**: openrouter/free agent

---

## Goal

Implement the `cycle_schedule` MCP tool to enable cron-triggered R&D cycles. The schedule state is persisted to `.ralph/cycle_schedule.json`.

## Acceptance Criteria (from ROADMAP)

> **Acceptance:** `cycle_schedule set=daily` schedules a cycle; `cycle_schedule status` shows next run time

## Context

R&D cycles currently run manually. This tool adds a lightweight cron-style scheduler that persists schedule state. The actual cycle execution uses the existing `loop_start` and `loop_step` tools.

## Schema

```go
type CycleSchedule struct {
    Enabled      bool      `json:"enabled"`
    Frequency    string    `json:"frequency"`    // "hourly", "daily", "weekly", "manual"
    LastRun      time.Time `json:"last_run"`
    NextRun      time.Time `json:"next_run"`
    MaxCycles    int       `json:"max_cycles"`   // 0 = unlimited
    RunCount     int       `json:"run_count"`
    RepoPath     string    `json:"repo_path"`
}
```

## MCP Tool Interface

```
cycle_schedule set=<frequency> [max_cycles=N]   — set schedule
cycle_schedule status                            — show current schedule and next run
cycle_schedule disable                           — disable schedule
cycle_schedule check                             — check if a run is due (returns bool)
```

## Files to Create/Modify

### New files:
- `internal/session/cycle_schedule.go` — scheduler logic
- `internal/session/cycle_schedule_test.go` — unit tests

### Modified files:
- `internal/mcpserver/tools_dispatch.go` — register `cycle_schedule` in `rdcycle` group
- `internal/mcpserver/handler_rdcycle.go` — `handleCycleSchedule` handler

## Verification

```bash
go build ./...
go test ./internal/session/... -run TestCycleSchedule -v
```

## Notes

- `check` action returns `{"due": true/false, "next_run": "...", "last_run": "..."}`
- Frequency calculation: hourly=1h, daily=24h, weekly=168h from last_run
- No background goroutine needed — supervisor polls `check` via 60s tick
