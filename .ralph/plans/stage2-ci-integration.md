# Stage 2: CI Integration for Recursive Self-Testing

## Overview

Three interconnected work items: CI self-test job, baseline artifact storage, and observation-to-diff correlation.

## 2.1 CI Self-Test Job

### Files to Create/Modify
- `.github/workflows/ci.yml` — Add `self-test-gate` job (depends on `go-tests`)
- `cmd/selftest.go` (new) — Wire `selftest` subcommand into cobra root

### Trigger Strategy
- Push to main + workflow_dispatch (not PRs initially to control API spend)
- Depends on `go-tests` passing first
- 2 iterations, $2 budget, 15min timeout, `RALPH_SELF_TEST=1`

### Gate Verdict to Exit Code
- `VerdictPass` → exit 0
- `VerdictWarn` → exit 0 (non-blocking)
- `VerdictSkip` → exit 0 (first run, no baseline)
- `VerdictFail` → exit 1 (blocks the build)

### `cmd/selftest.go` Design
- Cobra subcommand with flags: `--iterations` (default 2), `--budget` (default 2.00), `--repo-path` (default `.`), `--json`, `--gate`
- Uses `e2e.Prepare()` + `runner.Run()` from Stage 1.2
- After run: save observations, rebuild baseline, output JSON result

## 2.2 Baseline Artifact Storage

### Mechanism
- `actions/download-artifact@v4` with `continue-on-error: true` for first-run
- After self-test: rebuild baseline from observations, save to `.ralph/loop_baseline.json`
- `actions/upload-artifact@v4` with `overwrite: true`, 30-day retention

### Artifact Retention Policy
| Artifact | Retention | Overwrite |
|----------|-----------|-----------|
| `selftest-baseline` | 30 days | Yes (rolling) |
| `selftest-results` | 14 days | No (accumulate) |

### Baseline Merge Strategy
1. Load existing baseline (if present from download)
2. Load all observations (old + new)
3. `BuildBaseline(allObservations, 0)` for fresh P50/P95
4. Save merged baseline

## 2.3 Observation-to-Diff Correlation

### New Fields on `LoopObservation` (loopbench.go)
```go
DiffPaths   []string `json:"diff_paths,omitempty"`
DiffSummary string   `json:"diff_summary,omitempty"`
```

### Where to Capture
In `emitLoopObservation` after existing `gitDiffStats` loop:
- Iterate worktrees, collect paths via `gitDiffPathsForWorktree(wt)`
- Deduplicate paths, build summary string
- Session-internal helper (avoids circular import with e2e)

### Helper: `buildDiffSummary`
Format: `"N files: path1, path2, +M more"` (max 3 shown)

## Dependencies
```
2.3 (Observation-to-Diff) — no dependencies, ship first
2.1 (CI Self-Test Job) ← depends on cmd/selftest.go
2.2 (Baseline Artifact) ← structurally part of 2.1 YAML
```

## Rollout Order
1. Ship 2.3 (pure additive, no CI impact)
2. Ship 2.1 + 2.2 together (CI job + artifacts)
3. Enable PR gating after 5-10 stable main-branch runs
