# Task 01: cycle_baseline — R&D Cycle Baseline Snapshot Tool

**ROADMAP ID**: 9.1.5  
**Priority**: P0 | **Size**: S  
**Assigned to**: openrouter/free agent

---

## Goal

Implement the `cycle_baseline` MCP tool. This tool takes a snapshot of current repo metrics at the start of each R&D cycle so improvements can be measured against baseline.

## Acceptance Criteria (from ROADMAP)

> **Acceptance:** Snapshot of coverage, lint count, build time, test count written to `.ralph/cycle_baseline.json`

Specifically:
1. Run `go test -coverprofile=/tmp/cov.out ./...` and extract overall coverage %
2. Run `go build ./...` and measure wall-clock time
3. Count total tests (`go test -list '.*' ./... | wc -l`)
4. Run `golangci-lint run --no-config 2>&1 | wc -l` for lint count
5. Write JSON snapshot to `.ralph/cycle_baseline.json`
6. Register the MCP tool in `internal/mcpserver/`

## Files to Create/Modify

### New files:
- `internal/session/cycle_baseline.go` — `CycleBaseline`, `RunCycleBaseline(repoPath string) (*CycleBaseline, error)`
- `internal/session/cycle_baseline_test.go` — unit tests

### Modified files:
- `internal/mcpserver/tools_dispatch.go` — register `cycle_baseline` tool in the `rdcycle` tool group
- `internal/mcpserver/handler_rdcycle.go` (create if not exists) — `handleCycleBaseline` handler

## Schema

```go
type CycleBaseline struct {
    Timestamp    time.Time `json:"timestamp"`
    RepoPath     string    `json:"repo_path"`
    CoveragePC   float64   `json:"coverage_pct"`      // e.g. 83.4
    TestCount    int       `json:"test_count"`
    LintCount    int       `json:"lint_count"`
    BuildTimeSec float64   `json:"build_time_sec"`
    GoVersion    string    `json:"go_version"`
}
```

Output path: `<repoPath>/.ralph/cycle_baseline.json`

## Verification

```bash
go build ./...
go test ./internal/session/... -run TestCycleBaseline -v
go test ./internal/mcpserver/... -run TestHandleCycleBaseline -v
```

## Notes
- The tool should work even if `golangci-lint` is not installed (set lint_count to -1)
- Build time should use `os/exec` with time measurement, not the MCP call time
- Existing baseline file should be overwritten
