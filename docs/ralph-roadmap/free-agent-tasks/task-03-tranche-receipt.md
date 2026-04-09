# Task 03: Tranche Receipt Emission

**ROADMAP ID**: ATD-5  
**Priority**: P1 | **Size**: S  
**Assigned to**: openrouter/free agent

---

## Goal

Emit a structured tranche receipt JSON record after every successful push in the autobuild/loop cycle. Receipts are append-only records in `docs/tranche-receipts/`.

## Acceptance Criteria

> **Acceptance:** After each autobuild push, a receipt file `docs/tranche-receipts/<timestamp>-<tranche>.json` is written with metadata: tranche ID, commit hash, files changed, loop iteration, cost, timestamp.

## Context

The `docs/tranche-receipts/` directory already exists (see ROADMAP ATD tracking). We need to wire receipt emission into the loop completion path.

## Schema

```go
type TrancheReceipt struct {
    TrancheID    string    `json:"tranche_id"`    // e.g. "tranche-0042"
    Timestamp    time.Time `json:"timestamp"`
    CommitHash   string    `json:"commit_hash"`
    Branch       string    `json:"branch"`
    FilesChanged []string  `json:"files_changed"`
    LoopIter     int       `json:"loop_iteration"`
    CostUSD      float64   `json:"cost_usd"`
    Provider     string    `json:"provider"`
    RepoPath     string    `json:"repo_path"`
    Notes        string    `json:"notes,omitempty"`
}
```

## Files to Create/Modify

### New files:
- `internal/session/tranche_receipt.go` — `TrancheReceipt`, `WriteTrancheReceipt(repoPath string, receipt TrancheReceipt) error`
- `internal/session/tranche_receipt_test.go`

### Modified files:
- `internal/session/runner.go` — call `WriteTrancheReceipt` after successful session commit detection
- `internal/mcpserver/tools_dispatch.go` — register `tranche_receipt_write` tool in `rdcycle` group

## Output Path

`<repoPath>/docs/tranche-receipts/<YYYYMMDD-HHMMSS>-<tranche_id>.json`

If `docs/tranche-receipts/` doesn't exist, create it.

## Verification

```bash
go build ./...
go test ./internal/session/... -run TestTrancheReceipt -v
```
