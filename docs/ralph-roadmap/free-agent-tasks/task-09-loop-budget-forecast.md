# Task 09: loop_budget_forecast — Pre-Cycle Budget Prediction

**ROADMAP ID**: 9.2.2  
**Priority**: P1 | **Size**: M  
**Assigned to**: openrouter/free agent

---

## Goal

Implement `loop_budget_forecast` MCP tool that estimates cost for an upcoming loop before it starts.

## Acceptance Criteria

> **Acceptance:** `loop_budget_forecast task_count=5 provider=codex` returns predicted cost with P50/P95 bounds

## Context

`internal/session/costpredictor.go` already has cost prediction logic. This tool exposes it via MCP.

## Schema

```go
type BudgetForecast struct {
    TaskCount    int     `json:"task_count"`
    Provider     string  `json:"provider"`
    P50CostUSD   float64 `json:"p50_cost_usd"`
    P95CostUSD   float64 `json:"p95_cost_usd"`
    P50TurnsEst  float64 `json:"p50_turns_est"`
    CalibrationX float64 `json:"calibration_x"` // predicted/actual ratio
    DataPoints   int     `json:"data_points"`
    Warning      string  `json:"warning,omitempty"`
}
```

## Files to Create/Modify

### New files:
- `internal/session/loop_budget_forecast.go` — `ForecastLoopBudget(profile LoopProfile, taskCount int) BudgetForecast`
- `internal/session/loop_budget_forecast_test.go`

### Modified files:
- `internal/mcpserver/tools_dispatch.go` — register in `loop` group
- `internal/mcpserver/handler_loop.go` — add `handleLoopBudgetForecast`

## Algorithm

1. Load historical loop data from `.ralph/loop_baseline.json`
2. Use `CostPredictor.Predict(provider, taskCount)` from `costpredictor.go`
3. Apply P50/P95 from baseline observations
4. Return calibration ratio from `loop_baseline.json`
5. Warn if data_points < 5

## Verification

```bash
go build ./...
go test ./internal/session/... -run TestForecastLoopBudget -v
go test ./internal/mcpserver/... -run TestHandleLoopBudgetForecast -v
```
