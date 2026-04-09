# Task 10: QW-8 — Fix Budget Params Ignored in tools_session

**ROADMAP ID**: QW-8 (Tech Debt)  
**Priority**: P0 | **Size**: S  
**Assigned to**: openrouter/free agent

---

## Goal

Fix the bug where budget parameters passed to `ralphglasses_session_launch` are silently ignored.

## Acceptance Criteria (from ROADMAP Tech Debt)

> **Impact**: High — budget not enforced  
> **Component**: `tools_session.go`  
> **Finding IDs**: FINDING-258/261

When a caller passes `max_budget_usd=0.50` to `ralphglasses_session_launch`, that value should be forwarded to `LaunchOptions.MaxBudgetUSD` and respected by the session runner.

## Root Cause

In `internal/mcpserver/tools_session.go` (or wherever `ralphglasses_session_launch` is handled), the budget/turns params from the MCP request are not forwarded to `LaunchOptions`.

## Fix

1. Locate `handleSessionLaunch` (or equivalent) in `internal/mcpserver/`
2. Find where `LaunchOptions` is constructed
3. Ensure these MCP params are wired through:
   - `max_budget_usd` → `LaunchOptions.MaxBudgetUSD`
   - `max_turns` → `LaunchOptions.MaxTurns`
   - `max_time_minutes` → `LaunchOptions.MaxTimeMinutes` (if field exists)

## Files to Modify

- `internal/mcpserver/tools_session.go` — wire budget params into LaunchOptions
- `internal/mcpserver/handler_session.go` (if separate) — same fix

## Test to Add

In `internal/mcpserver/handler_session_test.go`:
```go
func TestSessionLaunch_BudgetParamForwarded(t *testing.T) {
    // verify that max_budget_usd=0.50 in request becomes LaunchOptions.MaxBudgetUSD=0.50
}
```

## Verification

```bash
go build ./...
go test ./internal/mcpserver/... -run TestSessionLaunch_Budget -v
```

## Notes

- Do NOT change the behavior when budget=0 (means "no limit")
- All 3 params (budget, turns, time) should be fixed in one PR
