# Stage 3: Continuous Self-Improvement

## Overview

Four pieces: self-improvement LoopProfile, two-tier acceptance gate, marathon wrapper, and TUI dashboard.

## 3.1 Self-Improvement Profile

### `SelfImprovementProfile()` in `internal/session/loop.go`
- Claude as planner+worker (sonnet-4)
- `MaxConcurrentWorkers: 1` — serial to avoid merge conflicts
- `RetryLimit: 2` — circuit-breaks fast, reflexion can recover once
- Verify: `ci.sh` + `selftest --gate`
- Budget: planner $1, worker $3 ($4 total per iteration)
- All self-learning enabled (reflexion, episodic, uncertainty, curriculum)
- `EnableCascade: false` — no provider cascading for self-modification

### New field on `LoopProfile`
```go
SelfImprovement bool `json:"self_improvement,omitempty"`
```

### MCP handler wiring
- `handler_loop.go`: `getBoolArg(req, "self_improvement")` → use `SelfImprovementProfile()`
- `tools.go`: add `self_improvement` bool param to `loop_start`

## 3.2 Two-Tier Acceptance

### Path Classification
| Classification | Paths | Action |
|---|---|---|
| Safe | `*_test.go`, `docs/`, `scripts/`, `internal/tui/`, `distro/` | Auto-commit + merge |
| Review | `internal/session/`, `internal/mcpserver/`, `internal/e2e/`, `cmd/`, `go.mod`, `CLAUDE.md` | Create PR via `gh` |

### New file: `internal/session/acceptance.go`
```go
type AcceptanceResult struct {
    SafePaths, ReviewPaths []string
    AutoMerged, PRCreated  bool
    PRURL                  string
    Error                  string
}

func (m *Manager) handleSelfImprovementAcceptance(ctx, run, iterIdx, worktrees) error
func AutoCommitAndMerge(dir, branch, message string) error
func CreateReviewPR(dir, branch, title string, reviewPaths []string) (string, error)
```

### New event types
- `SelfImproveMerged` — auto-merged safe changes
- `SelfImprovePR` — created PR for review-required changes

### StepLoop wiring
After verification passes, if `profile.SelfImprovement`:
1. Get diff paths per worktree
2. Classify via `ClassifyDiffPaths`
3. Route: all safe → auto-merge, any review → create PR
4. Record `AcceptanceResult` on `LoopIteration`

## 3.3 Marathon Integration

### `scripts/self-improve.sh`
- Pre-flight: run `ci.sh` + verify `gh` CLI
- Defaults: $20 budget, 4h duration, 1h checkpoints, 5 iterations
- Wraps `marathon.sh` with self-improvement flags

### MCP: `ralphglasses_self_improve` tool
- Combines `loop_start` with self-improvement defaults
- Params: `max_iterations`, `budget_usd`, `duration_hours`

## 3.4 TUI Dashboard

### New file: `internal/tui/views/selftest.go`
- `ViewSelfTest` mode in app.go, key binding `I`
- 4-panel layout: iteration history, gate trends, acceptance log, cost + HITL

### Data types
```go
type SelfTestData struct {
    RepoName, IsRunning, CurrentIter, TotalIters
    Iterations []SelfTestIteration
    GateHistory []GateSnapshot
    HITLAlerts []HITLAlert
    CostSoFar, BudgetRemaining float64
    AcceptanceLog []AcceptanceEntry
}
```

### Event subscriptions
- `SelfImproveMerged` → update acceptance log
- `SelfImprovePR` → show PR URL in notification
- `LoopIterated` → refresh iteration data

## Dependency Graph
```
3.1 SelfImprovementProfile
  ├── 3.2 Two-Tier Acceptance (needs SelfImprovement flag)
  │     ├── 3.3 Marathon Integration (needs acceptance gate)
  │     └── 3.4 TUI Dashboard (needs AcceptanceResult)
  └── 3.3 Marathon Integration (needs profile function)
```

## Implementation Order
| Step | Item | Effort | Can Parallelize With |
|------|------|--------|---------------------|
| 1 | 3.1 Profile | Small | Step 4 |
| 2 | 3.2 Protection expansion | Medium | — |
| 3 | 3.2 Acceptance + StepLoop wiring | Medium | — |
| 4 | 3.3 Marathon wrapper + MCP flags | Small | Step 1 |
| 5 | 3.4 TUI Dashboard | Medium-Large | — |

## Safety Matrix
| Mechanism | Protects Against |
|-----------|-----------------|
| `MaxConcurrentWorkers: 1` | Self-modification merge conflicts |
| `RetryLimit: 2` | Infinite failure retry |
| Core path classification | Auto-merging dangerous code |
| `ci.sh` pre-flight | Starting from broken state |
| `gh` CLI requirement | Silent PR creation failures |
| HITL tracking | Unobserved autonomous merges |
| Budget ceiling ($4/iter, $20/run) | Cost runaway |
| Git worktree isolation | Direct main branch modification |
