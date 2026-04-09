# Task 08: observation_correlate — Cross-Reference Observations with Git Commits

**ROADMAP ID**: 9.2.5  
**Priority**: P1 | **Size**: M  
**Assigned to**: openrouter/free agent

---

## Goal

Implement the `observation_correlate` MCP tool that cross-references loop observations with git commits to understand which changes produced which outcomes.

## Acceptance Criteria

> **Acceptance:** `observation_correlate repo_path=<p> window=7d` returns correlations between observations and commits in the time window

## Schema

```go
type ObservationCorrelation struct {
    ObservationID string    `json:"observation_id"`
    CommitHash    string    `json:"commit_hash"`
    CommitMessage string    `json:"commit_message"`
    Timestamp     time.Time `json:"timestamp"`
    FilesChanged  []string  `json:"files_changed"`
    Outcome       string    `json:"outcome"` // "improved", "regressed", "neutral"
    ConfidencePC  float64   `json:"confidence_pct"`
}
```

## Files to Create/Modify

### New files:
- `internal/session/observation_correlate.go` — `CorrelateObservations(repoPath string, window time.Duration) ([]ObservationCorrelation, error)`
- `internal/session/observation_correlate_test.go`

### Modified files:
- `internal/mcpserver/tools_dispatch.go` — register in `rdcycle` group
- `internal/mcpserver/handler_rdcycle.go` — handler

## Algorithm

1. Read `.ralph/observations.jsonl` for the time window
2. Run `git log --format="%H %ai %s" --since=<window>` in the repo
3. For each observation, find the closest git commit (within ±1h)
4. Classify outcome based on observation success/failure fields
5. Return sorted by timestamp

## Verification

```bash
go build ./...
go test ./internal/session/... -run TestObservationCorrelate -v
```
