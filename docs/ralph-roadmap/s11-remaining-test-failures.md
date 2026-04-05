# S11: Remaining Test Failures — Root Cause Analysis

Date: 2026-04-04  
Branch: self-improve-review-20260404-165828

## Summary

Five test failures across three packages. Two are real bugs in source code, two are stale test expectations, and one is an environment-dependent failure.

---

## 1. `TestConfigOptimizer_RecordOutcome_WindowTrim`

**File:** `internal/session/config_optimizer_test.go:73`  
**Error:** `expected window <= 5, got 10`

**Test expectation:** Sets `cfg.WindowSize = 5`, records 10 outcomes, expects `len(arm.recentOutcomes) <= 5`.

**Root cause — Real bug.** `NewConfigOptimizer` does not forward `cfg.WindowSize` to the arm on creation. In `RecordOutcome`, the arm is always initialized with a hardcoded `windowSize: 20`:

```go
// config_optimizer.go:122-128
arm = &ConfigArm{
    ID:         key,
    Provider:   provider,
    TaskType:   taskType,
    windowSize: 20,  // BUG: ignores cfg.WindowSize
}
```

The `NewConfigOptimizer` stores `exploration` and `minTrials` from config but never stores `windowSize` for use when creating arms.

**Classification:** real-bug

**Fix:** Store `windowSize` in `ConfigOptimizer` and use it when creating new arms:

```go
// In ConfigOptimizer struct, add:
windowSize int

// In NewConfigOptimizer, add:
windowSize: cfg.WindowSize,

// In RecordOutcome, replace hardcoded windowSize:
arm = &ConfigArm{
    ID:         key,
    Provider:   provider,
    TaskType:   taskType,
    windowSize: co.windowSize,
}
```

---

## 2. `TestConfigOptimizer_PendingSuggestions`

**File:** `internal/session/config_optimizer_test.go:202`  
**Error:** `expected pending suggestions`

**Test expectation:** Records 5 outcomes each for claude (all success) and gemini (all failure) for task `"fix"`, calls `SuggestChanges()`, then expects `PendingSuggestions()` to return non-empty.

**Root cause — Real bug (intermittent, map iteration order).** In `SuggestChanges`, the algorithm finds both `bestArm` (highest score) and `mostUsedArm` (highest trial count). Both arms have exactly 5 trials, so `mostUsedArm` is whichever arm is iterated first from the `byTask[taskType]` slice (which in turn comes from iterating over the `co.arms` map — non-deterministic in Go). When claude happens to be iterated first, it becomes `mostUsedArm` with `mostTrials = 5`. Then gemini is checked: `arm.Trials (5) > mostTrials (5)` is false, so gemini never updates `mostUsedArm`. Claude ends up as both `bestArm` and `mostUsedArm`, so `bestArm.ID == mostUsedArm.ID` and no suggestion is generated.

The existing passing test `TestConfigOptimizer_SuggestChanges` avoids this by recording 10 outcomes each (10 vs 10) — same race. It passes intermittently for the same reason.

**Classification:** real-bug

**Fix:** Break the tie in `mostUsedArm` selection to always prefer the arm with lower score when trial counts are equal, or track "currently configured" arm separately rather than inferring it from trial count:

```go
// Replace the mostUsedArm update condition:
if arm.Trials > mostTrials || (arm.Trials == mostTrials && mostUsedArm != nil && co.armScore(arm) < co.armScore(mostUsedArm)) {
    mostTrials = arm.Trials
    mostUsedArm = arm
}
```

Alternatively, separate the "current provider" concept from the "most-used arm" — the most-used arm and the best arm should come from different selection criteria so they cannot converge to the same pointer.

---

## 3. `TestRunSessionOutput_CostSource`

**File:** `internal/session/runner_coverage_test.go:638`  
**Error:** `expected cost_source=api_key, got structured`

**Test expectation:** Sends JSON line `{"type":"result","result":"ok","cost_usd":0.50,"cost_source":"api_key"}` through `runSessionOutput`, expects `s.CostSource == "api_key"`.

**Root cause — Stale test expectation.** The `StreamEvent` struct has `CostSource` tagged as `json:"-"`:

```go
// internal/session/types.go:115
CostSource string `json:"-"` // "structured" or "estimated" — set by normalizer
```

The `json:"-"` tag explicitly excludes the field from JSON unmarshaling, so the `cost_source` value in the input JSON is ignored. The `normalizeClaudeEvent` function then sets `CostSource = "structured"` because `CostUSD > 0` and `CostSource == ""` (line 58-60 of `providers_normalize.go`). The normalizer intentionally overrides any raw value.

The test was written expecting raw `cost_source` values to be passed through, but the design deliberately normalizes this field. There are only three valid values: `"structured"`, `"estimated"`, and `"stderr"`.

**Classification:** quick-fix (update expectation)

**Fix:** Update the test to expect `"structured"` (the correct normalized value) and document that `cost_source` in the wire format is normalized — callers cannot inject custom values:

```go
// runner_coverage_test.go:637
if s.CostSource != "structured" {
    t.Fatalf("expected cost_source=structured (normalized), got %s", s.CostSource)
}
```

---

## 4. `TestCollectChildPIDs_DeadPID`

**File:** `internal/process/childpids_linux_test.go:52`  
**Error:** `expected nil for non-existent PID, got []`

**Test expectation:** Calls `CollectChildPIDs(1_000_000_000)` (a nonexistent PID) and expects `nil` return.

**Root cause — Stale test expectation.** `CollectChildPIDs` explicitly returns empty non-nil slices (`[]int{}`) on all error/empty paths:

```go
// childpids.go:14-20
func CollectChildPIDs(pid int) []int {
    pids := CollectChildPIDsByPgid(pid)
    if len(pids) > 0 {
        return pids
    }
    return CollectChildPIDsFromProc(pid)  // returns []int{} on no match
}
```

Both `CollectChildPIDsByPgid` (returns `[]int{}` when `Getpgid` fails) and `CollectChildPIDsFromProc` (returns `[]int{}` when no children found) return empty non-nil slices. The test assumes `nil` is returned for a dead PID, but the contract is "empty slice" not "nil". This is a Go convention mismatch — the code follows "never return nil slice" idiom but the test uses `if pids != nil` as the check.

**Classification:** quick-fix (update expectation)

**Fix:** Update the test to check `len(pids) != 0` instead of `pids != nil`:

```go
// childpids_linux_test.go:51-53
if len(pids) != 0 {
    t.Errorf("expected empty result for non-existent PID, got %v", pids)
}
```

---

## 5. `TestDispatch_Improve_WithQuietFlag`

**File:** `cmd/prompt-improver/coverage_test.go:54`  
**Error:** Test binary terminates (os.Exit) mid-run with 401 from OpenAI API.

**Test expectation (per comment):** "will os.Exit due to no API key, but exercises the flag parsing branch." Expected: test runs and exits cleanly because `runImprove` calls `os.Exit(1)` when no API key is found.

**Root cause — Env-dependent, with secondary real-bug.** Two issues compound:

1. **Environment issue:** `OPENAI_API_KEY` is set in the shell environment (with a stale/expired key). `getOrCreateEngine` returns a non-nil engine, so `runImprove` does not exit early. It proceeds to call the OpenAI API, which returns HTTP 401. The test then calls `os.Exit(1)` via the error path (line 346), terminating the entire test binary before subsequent tests can run.

2. **Underlying real bug:** Even when `OPENAI_API_KEY` is unset (confirmed by running with `OPENAI_API_KEY=""`), the `runImprove` → `os.Exit(1)` path kills the entire `go test` binary. Calling `os.Exit` inside an in-process test function is always incorrect — it terminates all tests, not just the one under test. The comment admits this ("os.Exit happens in runImprove") but does not skip or guard the test.

**Classification:** env-dependent (primary), real-bug in test design (secondary)

**Recommended fix:** Wrap the call in a subprocess so `os.Exit` doesn't kill the test runner, matching the pattern used by `TestCLI_Improve_NoAPIKey` in `main_test.go`:

```go
func TestDispatch_Improve_WithQuietFlag(t *testing.T) {
    // runImprove calls os.Exit — must run in a subprocess to avoid killing test binary.
    cmd := exec.Command(os.Args[0], "-test.run=TestDispatch_Improve_WithQuietFlag_subprocess")
    cmd.Env = append(os.Environ(), "OPENAI_API_KEY=", "ANTHROPIC_API_KEY=", "GOOGLE_API_KEY=")
    err := cmd.Run()
    if err == nil {
        t.Error("expected non-zero exit when no API key set")
    }
}
```

Or simply skip the test since the coverage it provides (flag parsing) is already covered by `TestCLI_Improve_NoAPIKey`:

```go
func TestDispatch_Improve_WithQuietFlag(t *testing.T) {
    t.Skip("runImprove calls os.Exit — covered by TestCLI_Improve_NoAPIKey subprocess test")
}
```

---

## Defect Summary Table

| Test | File | Line | Classification | Fix Effort |
|------|------|------|----------------|------------|
| `TestConfigOptimizer_RecordOutcome_WindowTrim` | `internal/session/config_optimizer_test.go` | 73 | real-bug | Small — store windowSize in ConfigOptimizer, use it in RecordOutcome |
| `TestConfigOptimizer_PendingSuggestions` | `internal/session/config_optimizer_test.go` | 202 | real-bug | Small — fix tie-breaking in mostUsedArm selection |
| `TestRunSessionOutput_CostSource` | `internal/session/runner_coverage_test.go` | 638 | quick-fix | Trivial — change expected value from `"api_key"` to `"structured"` |
| `TestCollectChildPIDs_DeadPID` | `internal/process/childpids_linux_test.go` | 52 | quick-fix | Trivial — change `pids != nil` to `len(pids) != 0` |
| `TestDispatch_Improve_WithQuietFlag` | `cmd/prompt-improver/coverage_test.go` | 54 | env-dependent | Small — skip or convert to subprocess test; unset API keys in env |
