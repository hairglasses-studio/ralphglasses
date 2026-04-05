# S5: Sweep Parallelization Design

**Status**: Design / Pre-implementation
**Date**: 2026-04-04
**Scope**: `internal/mcpserver/handler_sweep.go` — `handleSweepLaunch`

---

## 1. Current Serial Code Flow

The sweep launch goroutine (lines 253-321 of `handler_sweep.go`) fans out to repos in a plain `for` loop with no concurrency:

```
handleSweepLaunch (line 161)
  └─ go func() {                                           // line 253 — single background goroutine
       for _, r := range targetRepos {                     // line 257 — serial iteration
         repoPrompt := strings.ReplaceAll(...)             // line 258 — placeholder substitution
         opts := session.LaunchOptions{...}                // line 261 — build options
         if enhanceMode != "none" {                        // line 279 — optional LLM round-trip
           enhancer.EnhanceHybrid(ctx, ...)               // line 286 — blocks for entire prompt enhancement
         }
         sweepPool.Allocate(r.Name, budgetUSD)            // line 292 — budget gate (breaks on cap)
         s.SessMgr.Launch(context.Background(), opts)     // line 297 — blocks until session starts
         launched = append(launched, ...)                  // line 303
       }
       s.Tasks.Complete(taskID, result)                    // line 320 — signals task done
     }()
```

### Key observations

| Point | Detail |
|-------|--------|
| **Serial launch** | Each `SessMgr.Launch` call completes before the next repo starts. With 10+ repos this adds wall-clock latency equal to N × launch-overhead. |
| **Serial enhance** | `enhancer.EnhanceHybrid` does an optional LLM round-trip per repo; these are entirely independent and could run in parallel. |
| **Budget break** | `sweepPool.Allocate` returns `ErrBudgetCeiling` and the loop does `break` (line 295). This means repos beyond the cap are silently omitted without being tracked in `errors`. |
| **Context mismatch** | `SessMgr.Launch` receives a fresh `context.Background()` (line 297) instead of the sweep context `ctx`, so cancellation via `ctx.Done()` does not propagate to in-flight launches. |
| **No concurrency control** | There is no semaphore, no rate limiting, and no parallelism floor or ceiling. |

---

## 2. Proposed Parallel Design

### Parameters

- `concurrency int` — default 10, passed through a new `sweep_concurrency` tool parameter (0 or negative → default).
- Semaphore: a buffered channel of size `concurrency`.
- Results channel: collects `sweepResult` structs from goroutines.

### Pseudocode

```go
type sweepResult struct {
    repoName  string
    sess      *session.Session  // nil on error
    err       error
}

// Fan-out with semaphore.
sem := make(chan struct{}, concurrency)   // backpressure: at most N concurrent goroutines
results := make(chan sweepResult, len(targetRepos))
var wg sync.WaitGroup

for _, r := range targetRepos {
    // Budget pre-check: take the allocation before spawning the goroutine.
    // Allocation is atomic in BudgetPool so this is safe under concurrency.
    if err := sweepPool.Allocate(r.Name, budgetUSD); err != nil {
        results <- sweepResult{repoName: r.Name, err: fmt.Errorf("budget cap: %w", err)}
        continue   // do NOT break — remaining repos may still fit if prior repos were cheaper
    }

    wg.Add(1)
    go func(repo *repoRef) {
        defer wg.Done()

        // Acquire semaphore slot (blocks when N goroutines are already active).
        select {
        case sem <- struct{}{}:
        case <-ctx.Done():
            results <- sweepResult{repoName: repo.Name, err: ctx.Err()}
            return
        }
        defer func() { <-sem }()

        repoPrompt := strings.ReplaceAll(prompt, "REPO_PLACEHOLDER", repo.Name)
        opts := session.LaunchOptions{
            Provider:             session.DefaultPrimaryProvider(),
            RepoPath:             repo.Path,
            Prompt:               repoPrompt,
            Model:                model,
            MaxBudgetUSD:         budgetUSD,
            MaxTurns:             maxTurns,
            PermissionMode:       permMode,
            SweepID:              sweepID,
            AllowedTools:         tools,
            NoSessionPersistence: !sessionPersistence,
            SessionID:            fmt.Sprintf("%s-%s", sweepID, repo.Name),
        }
        if effort != "" {
            opts.Effort = effort
        }

        // Optional prompt enhancement (independent per-repo LLM call).
        if enhanceMode != "" && enhanceMode != "none" {
            cfg := enhancer.LoadConfig(repo.Path)
            if enhancer.ShouldEnhance(repoPrompt, cfg) {
                m := enhancer.ValidMode(enhanceMode)
                if m == "" {
                    m = enhancer.ModeLocal
                }
                eResult := enhancer.EnhanceHybrid(ctx, repoPrompt, "", cfg, s.getEngine(), m, enhancer.ProviderOpenAI)
                opts.Prompt = eResult.Enhanced
            }
        }

        // Launch — use sweep ctx so cancellation propagates.
        sess, err := s.SessMgr.Launch(ctx, opts)
        results <- sweepResult{repoName: repo.Name, sess: sess, err: err}
    }(r)
}

// Wait for all goroutines then close results.
go func() {
    wg.Wait()
    close(results)
}()

// Drain results.
var launched []map[string]any
var errors []string
for res := range results {
    if res.err != nil {
        errors = append(errors, fmt.Sprintf("%s: %v", res.repoName, res.err))
        continue
    }
    launched = append(launched, map[string]any{
        "session_id": res.sess.ID,
        "repo":       res.repoName,
        "status":     res.sess.Status,
    })
}

result := map[string]any{ ... }
s.Tasks.Complete(taskID, result)
```

---

## 3. Backpressure Mechanism

The semaphore (`chan struct{}` of size `concurrency`) is the single backpressure point. It prevents the process table from being overwhelmed by N simultaneous subprocess launches when N is large.

```
targetRepos = 50 repos
concurrency = 10

Timeline:
  t=0:   10 goroutines acquire sem and call SessMgr.Launch concurrently
  t=0:   40 goroutines block on `sem <- struct{}{}`
  t=Δ1:  first launch completes, releases one sem slot → goroutine #11 unblocks
  ...
  t=Δn:  all 50 done
```

### Choosing the default (10)

- Each `SessMgr.Launch` spawns a subprocess (Claude / Codex / Gemini CLI). 10 simultaneous subprocesses is well within Linux fd/process limits.
- The existing `MaxSessions: 4` cap on `WorkerAgent` is a per-node worker constraint; this is a per-sweep local-node constraint. They are orthogonal.
- If the sweep runs on the local node alongside the TUI, 10 concurrent launches will saturate I/O briefly but recover quickly (each session then runs asynchronously in the background).

The `sweep_concurrency` tool parameter lets the caller tune this (e.g. `1` for serial fallback, `50` for a large idle machine).

---

## 4. Error Handling Per-Repo

Current behavior: `errors = append(errors, ...)` and `continue` for launch errors, `break` for budget exhaustion. Budget errors after the break are invisible.

Proposed behavior:

| Error type | Action |
|---|---|
| `ErrBudgetCeiling` from `Allocate` | Append to `errors`; `continue` (not `break`) — subsequent cheaper repos may still fit if ceiling has not been fully consumed |
| Launch error from `SessMgr.Launch` | Append `"{repo}: {err}"` to `errors`; free the semaphore slot; do NOT call `sweepPool.Record` |
| `ctx.Err()` (cancelled sweep) | Append `"{repo}: context cancelled"` to `errors`; goroutine exits; semaphore released via defer |
| Enhance error | Non-fatal: fall back to original `repoPrompt`, log at debug level, continue with launch |

The `sweepPool.Allocate` call moves before the goroutine spawn (as shown in pseudocode). This is safe because `BudgetPool` is mutex-protected and allocation is checked against total allocated (not total spent), giving an accurate ceiling gate before any work begins.

Result shape gains two new fields:

```json
{
  "launched": [...],
  "errors": [...],
  "budget_exhausted_repos": [...],  // repos that hit the ceiling
  "concurrency": 10
}
```

---

## 5. Budget Integration Points

The existing budget enforcement has two layers; both are preserved and extended:

### Layer 1 — Pre-launch allocation gate (`sweepPool.Allocate`)

**Location**: before the goroutine spawn (moved from inside the loop body at line 292).

`BudgetPool.Allocate` holds a write lock and checks `totalAllocated + amount > ceiling`. Because allocation happens on the outer goroutine (main fan-out loop) before spawning worker goroutines, there is no race between concurrent goroutines trying to allocate simultaneously — they queue on the outer loop iteration. This is simpler and safer than per-goroutine allocation.

Alternative (per-goroutine allocation): would allow all goroutines to race to allocate. The pool lock handles safety, but the ordering of which repos "win" becomes non-deterministic. The outer-loop model preserves FIFO priority, consistent with `resolveSweepRepos` ordering.

### Layer 2 — Runtime cost cap (`handleSweepSchedule`)

The sweep schedule task already polls `SpentUSD` across sessions and stops all running sessions if `maxCostCap` is reached (lines 596-612). This layer is unaffected by parallelization.

### Layer 3 (recommended addition) — Per-session `CanSpend` gate

`session.Manager.Launch` already applies a `DefaultBudgetUSD` floor and a hard $5 cap (lines 54-60 of `manager_lifecycle.go`). The parallel design passes `ctx` (sweep context) instead of `context.Background()`, which means a cancelled sweep context will abort in-flight launches that haven't fully started.

**No changes required** to `BudgetPool` or `Manager` for the parallel design. The only integration change is passing `ctx` to `SessMgr.Launch`.

---

## 6. Estimated Effort and Risk

### Effort

| Task | Estimate |
|------|----------|
| Add `sweep_concurrency` parameter to `handleSweepLaunch` | 15 min |
| Refactor serial for-loop into goroutine fan-out with semaphore and results channel | 45 min |
| Fix budget break → continue + move Allocate before goroutine spawn | 20 min |
| Fix context propagation (`ctx` instead of `context.Background()`) | 5 min |
| Update `Tasks.Complete` result shape (add `concurrency`, `budget_exhausted_repos`) | 10 min |
| Unit tests: semaphore limits, error aggregation, budget exhaustion mid-sweep | 60 min |
| **Total** | **~2.5 hours** |

### Risk

| Risk | Severity | Mitigation |
|------|----------|------------|
| Simultaneous subprocess spawn storms | Medium | Semaphore default of 10 caps concurrent launches |
| Budget race (two goroutines allocating simultaneously) | Low | `BudgetPool` uses `sync.Mutex`; allocation moved to outer loop |
| Cancelled sweep leaks sessions already launched | Low | Sessions already running are tracked in `SessMgr`; `sweepSessions()` still finds them by `SweepID`; `handleSweepNudge` can stop them |
| `ctx` propagation causes premature abort on slow providers | Low | Sweep context is user-cancellable (`context.WithCancel`); add a per-launch timeout derived from `maxTurns * avgTurnDuration` if needed later |
| Non-deterministic error list ordering | Negligible | Results channel collects in completion order; sort by repo name before returning if determinism is required |

### What does NOT change

- `handleSweepStatus`, `handleSweepNudge`, `handleSweepSchedule` — no changes needed; they operate on session state, not launch mechanics.
- `resolveSweepRepos` — unchanged.
- `BudgetPool` — unchanged.
- `session.Manager.Launch` — unchanged (only `context.Background()` → `ctx` at the call site).
- The MCP tool schema for `sweep_launch` gains one optional integer field (`sweep_concurrency`); existing callers are unaffected.

---

## Implementation Checklist

```
[ ] Add sweep_concurrency param (default 10) to handleSweepLaunch
[ ] Replace serial for-loop with semaphore fan-out
[ ] Move sweepPool.Allocate before goroutine spawn; change break → continue
[ ] Pass ctx to SessMgr.Launch (not context.Background())
[ ] Add budget_exhausted_repos and concurrency to result map
[ ] Write unit tests for:
    [ ] concurrency=1 produces same result as current serial behavior
    [ ] concurrency=N launches at most N simultaneous sessions
    [ ] budget exhaustion mid-sweep records exhausted repos without breaking remaining
    [ ] ctx cancellation drains results cleanly
[ ] Run go test ./internal/mcpserver/... -count=1 -race
```
