# 03 -- Session Architecture
Generated: 2026-04-04

## 1. Session Lifecycle State Machine

### States (from types.go)

| State | Value | Terminal? | Description |
|-------|-------|-----------|-------------|
| `StatusLaunching` | `"launching"` | No | Process started, startup probe running |
| `StatusRunning` | `"running"` | No | Process alive, streaming output |
| `StatusCompleted` | `"completed"` | Yes | Process exited 0, parsed result event |
| `StatusStopped` | `"stopped"` | Yes | User or StopAll called; SIGTERM/SIGKILL sent |
| `StatusErrored` | `"errored"` | Yes | Process exited non-0, or extra-usage exhausted |
| `StatusInterrupted` | `"interrupted"` | Yes | Rehydrated from store; process no longer exists |

`IsTerminal()` returns true for all four terminal states and is the canonical test for "is this session done?".

### State Diagram

```
                  ┌──────────────┐
                  │  (not yet)   │
                  └──────┬───────┘
                         │ Manager.Launch → runner.launch()
                         ▼
                  ┌──────────────┐
            ┌────>│  LAUNCHING   │<──────────────────────────────┐
            │     └──────┬───────┘                               │
            │            │ cmd.Start() succeeds                  │
            │            │ startup probe passes                  │
            │            ▼                                       │
            │     ┌──────────────┐                               │
            │     │   RUNNING    │──────── user Stop() ─────────>│
            │     └──────┬───────┘                     ┌─────────┴──────────┐
            │            │                             │      STOPPED        │
            │            │ process exit 0              └─────────────────────┘
            │            ▼
            │     ┌──────────────┐
            │     │  COMPLETED   │  (terminal)
            │     └──────────────┘
            │
            │     process exit != 0, or extra-usage
            │            ▼
            │     ┌──────────────┐
            │     │   ERRORED    │  (terminal)
            │     └──────────────┘
            │
            │     RehydrateFromStore on restart
            │            ▼
            │     ┌──────────────┐
            └─────│ INTERRUPTED  │  (terminal)
                  └──────────────┘
```

### Transition Triggers

- `LAUNCHING → RUNNING`: `cmd.Start()` succeeds; `s.mu.Lock()` used to set `Status = StatusRunning` and capture PID.
- `RUNNING → COMPLETED`: `runSession` detects `cmd.Wait()` returns `nil`; sets status under `s.mu`.
- `RUNNING → ERRORED`: `cmd.Wait()` returns non-nil error; also triggered by `isExtraUsageExhausted()` detection post-exit.
- `RUNNING → STOPPED`: `Manager.Stop()` sets `Status = StatusStopped` under `s.mu`, then sends SIGTERM/SIGKILL.
- Any non-terminal → `INTERRUPTED`: `RehydrateFromStore()` on process restart; previous runner goroutine is dead.

Startup probe (5-second window): if the process exits in the 5-second probe window, `launch()` returns an error and the caller never sees a LAUNCHING session — the session object is discarded.

---

## 2. Manager Concurrency Model

### Three-Lock Hierarchy

Manager carries three distinct `sync.RWMutex` fields, each guarding a different scope:

```
Manager.sessionsMu  -- guards sessions map[string]*Session
Manager.workersMu   -- guards teams, workflowRuns, loops maps + totalPrunedThisSession
Manager.configMu    -- guards stateDir, optimizer, cascade, supervisor, store, worktreePool
```

Additionally, `Manager.statusCache` is a `sync.Map` (Phase 10.5.1) for hot-read path lookups
that bypass `sessionsMu`.

### Lock Acquisition Order

The code consistently reads `configMu` (or `sessionsMu`) before accessing session-level locks.
The canonical read pattern is:

1. `m.configMu.RLock()` — read config field (optimizer, cascade, etc.)
2. `m.configMu.RUnlock()`
3. Operate on retrieved reference (no Manager lock held while calling into session or supervisor)

The session map read pattern:

1. `m.sessionsMu.RLock()` — read sessions map
2. `m.sessionsMu.RUnlock()`
3. `s.Lock()` / `s.Unlock()` — lock individual session

This correctly avoids holding the map lock while acquiring session-level locks.

### statusCache Hot Path

`updateStatusCache(id, status)` writes to the `sync.Map` whenever a session status changes.
`evictStatusCache(id)` deletes entries during cleanup in `LoadExternalSessions`.
This allows status reads without acquiring `sessionsMu`, reducing contention for high-frequency
TUI polling. There is no ordering guarantee between the `sync.Map` update and the `sessions` map
update, but since `statusCache` is always written immediately after `sessions`, the brief
inconsistency window is acceptable for a display-only fast path.

### Team Management Locking

Teams are stored under `workersMu` alongside workflows and loops. The manager acquires
`workersMu.Lock()` to insert/update/delete teams. However, `updateTeamOnSessionEnd` (called
from `s.onComplete`) acquires `m.workersMu.Lock()` from within the runner goroutine — this
is safe because the runner goroutine does not hold any Manager locks when invoking `onComplete`.

### Deadlock Risk Assessment

No direct deadlock risk observed. The Manager never holds two of its own mutexes simultaneously.
`configMu`, `sessionsMu`, and `workersMu` are never held concurrently within a single call frame.
The only notable nesting is Manager lock → session lock (always in that order), which is
consistent throughout the codebase.

One subtle pattern: `GetLoop` calls `LoadExternalLoops()` and then acquires `workersMu.Lock()`
(not RLock). If the go routine holding workersMu calls GetLoop recursively, this would deadlock.
Inspection shows `LoadExternalLoops` does not call `GetLoop`, so this path is safe.

---

## 3. Cascade Routing Flow

### Config and Defaults

`DefaultCascadeConfig()` returns:

```
CheapProvider:       ProviderGemini
ExpensiveProvider:   DefaultPrimaryProvider()  (Codex as of current config)
ConfidenceThreshold: 0.7
MaxCheapBudgetUSD:   $2.00
MaxCheapTurns:       15
```

The router is constructed in `NewManager*` constructors and attached to `m.cascade`.

### Provider Selection Logic (ShouldCascade / ResolveProvider)

```
ShouldCascade(taskType, prompt):
  1. Check TaskTypeOverrides — if override exists, skip cascade (use override directly)
  2. Check latency gate — if cheap provider's P95 latency > LatencyThresholdMs, skip
  3. Check FeedbackAnalyzer profile — if cheap provider has >90% completion on 5+ samples, skip
  4. Otherwise: cascade

SelectTier(taskType, complexity):
  1. If bandit hooks configured and enough history: consult bandit selector
  2. Otherwise: find tier matching task complexity from DefaultModelTiers()
```

### Confidence Thresholds and Escalation

After the cheap session completes, `EvaluateCheapResult` decides whether to escalate:

1. If `s.Error != ""` → escalate with `reason="error"`
2. If any `LoopVerification.ExitCode != 0` → escalate with `reason="verify_failed"`
3. Compute confidence (heuristic or calibrated DecisionModel):
   - Heuristic: `0.30*verifyPassed + 0.25*(1-hedgeCount) + 0.20*turnRatio + 0.15*errorFree + 0.10*(1-questionCount)`
   - Calibrated (when DecisionModel.IsTrained()): logistic regression on 10 features
4. If `confidence < ConfidenceThreshold (0.7)` → escalate with `reason="low_confidence"`
5. Otherwise → accept cheap result, skip expensive launch

### All-Providers-Fail Behavior

`LaunchWithFailover` iterates the FailoverChain. For each provider:
- Quick health pre-check (`CheckProviderHealth`)
- If healthy: attempt launch
- If launch fails: record error, continue to next provider

If all providers fail, returns `fmt.Errorf("all providers failed: <errors joined with '; '>")`.
The cascade router does not itself retry; retry logic lives in `RunLoop`'s auto-recovery layer.

---

## 4. Loop Engine Architecture

### Call Chain

```
RunLoop (loop.go)
  └── StepLoop (loop_steps.go)          -- one planner/worker/verify iteration
        ├── buildLoopPlannerPromptN      -- (loop_planner.go) builds prompt from ROADMAP, journal, prev iters
        │     └── buildLoopPlannerPrompt
        ├── launchWorkflowSession        -- planner session
        ├── waitForSession               -- blocks until planner done
        ├── plannerTasksFromSession      -- (loop_planner.go) parse JSON task(s) from output
        ├── [JSON retry loop, max 2]     -- retry if planner emits freeform text
        ├── [dedup filters]              -- near-duplicate and content-overlap rejection
        ├── [curriculum sort]            -- easy-first ordering if enabled
        ├── [goroutine fan-out]          -- one goroutine per task
        │     └── runWorkerTask          -- (loop_worker.go)
        │           ├── WorktreePool.Acquire or createLoopWorktree
        │           ├── [episodic + reflexion injection]
        │           ├── [cascade routing: cheap → evaluate → expensive]
        │           ├── launchWorkflowSession  -- worker session
        │           └── waitForSession
        ├── [collect results, 15-min timeout]
        ├── [noop detection]
        ├── runLoopVerification          -- bash commands in worktree
        ├── [auto-CI-fix, max retries]
        └── [acceptance gate + self-improvement routing]
```

### Gate Evaluation

Convergence gates in `StepLoop` (evaluated before launching planner):

- `consecutiveLoopFailures > run.Profile.RetryLimit` → fail with ErrRetryLimitExceeded
- `run.Profile.MaxIterations > 0 && len(run.Iterations) >= MaxIterations` → status=completed
- `run.Deadline != nil && time.Now().After(*run.Deadline)` → status=completed
- `detectConvergence(run.Iterations)` → status=converged if 3+ recent identical no-ops or same error

Post-worker gates (in `RunLoop`, wrapping `StepLoop`):

- `checkLoopBudget` — aggregate planner+worker spend vs `(budget * Headroom)` — soft cap
- `HardBudgetCapUSD` — absolute spend ceiling (checked before each step)
- `MaxWorkerTurns` (default 20) — total iteration count ceiling
- `NoopPlateauLimit` — N consecutive no-op iterations triggers status=converged
- `depthEst.ShouldEarlyStop` — adaptive early stop on diminishing returns

### Pruning / No-Op Detection

`NoOpDetector` (loop_noop.go) tracks consecutive no-change iterations per loop ID. When
`noopFilesChanged == 0 && noopLinesAdded == 0` on 2+ consecutive iterations (default),
the detector signals skip, the iteration is marked `"idle"`, and `RunLoop` increments its
`consecutiveNoops` counter toward `NoopPlateauLimit`.

---

## 5. Supervisor Control Loop

### Tick Architecture

```
Supervisor.Start(ctx)
  └── goroutine: run(ctx)
        └── ticker := time.NewTicker(TickInterval)  // default 60s
              for each tick:
                └── tick(ctx)
                      ├── shouldTerminate()          // check MaxCycles, MaxDuration, budget
                      ├── stallHandler.CheckAndHandle // kill stalled sessions
                      ├── monitor.Evaluate()          // health signals
                      │     └── executeDecision()    // per signal
                      │           └── Propose() → launchCycle / runSelfTest / runConsolidation
                      ├── chainer.CheckAndChain()    // cycle chaining
                      ├── planner.PlanNextSprint()   // sprint planning fallback
                      ├── tickCount++
                      └── persistState()             // .ralph/supervisor_state.json
```

### Health Assessment

`HealthMonitor.Evaluate(repoPath)` reads metrics from disk files in `.ralph/`:
- `cost_observations.json` → completion rate signal
- `coverage.txt` → coverage regression signal
- Emits `HealthSignal` structs with `Category`, `Metric`, `Value`, `Threshold`, `SuggestedAction`

### Cycle Chaining and Depth Cap

`CycleChainer.CheckAndChain` inspects the last completed cycle's outcomes. If criteria are met,
it returns a next cycle. The supervisor launches it in a goroutine via `mgr.RunCycle(ctx, ..., depth=3)`.
Depth cap: `RunCycle` is called with a fixed depth of 3 in all supervisor paths, preventing
unbounded recursive chaining.

### Cooldown Period

`CooldownBetween` (default 5 minutes). `launchCycle` checks `time.Since(s.lastCycleLaunch) < cooldown`
before launching. The first cycle (when `lastCycleLaunch.IsZero()`) is exempt from cooldown.

### Unhealthy Session Response

When `stallHandler.CheckAndHandle` detects a stalled session (exceeding `profile.StallTimeout`),
it kills the session and logs the event. The supervisor does not relaunch stalled sessions directly;
relaunching is delegated to the auto-recovery layer (`AutoRecovery.HandleSessionError`) which fires
from `onComplete` when the session ends with `StatusErrored`.

### Feedback Loop (every 10 ticks)

`RunFeedbackLoop()` is called at the end of each tick. It runs pattern consolidation and note
generation as a cross-subsystem feedback pass.

---

## 6. Autonomy Level Gating

### Levels

| Level | Name | Value | Bootstrap-eligible? |
|-------|------|-------|---------------------|
| 0 | `observe` | `LevelObserve` | Yes (default) |
| 1 | `auto-recover` | `LevelAutoRecover` | Yes (max for bootstrap) |
| 2 | `auto-optimize` | `LevelAutoOptimize` | No (requires explicit SetAutonomyLevel) |
| 3 | `full-autonomy` | `LevelFullAutonomy` | No |

Bootstrap (`BootstrapAutonomy`) clamps levels > 1 to 1, preventing L2/L3 from activating
via `.ralphrc` alone — they require programmatic `SetAutonomyLevel` calls.

### DecisionLog.Propose Flow

Every autonomous action passes through `DecisionLog.Propose(decision)`:

```
Propose(d AutonomousDecision):
  1. Check d.RequiredLevel <= dl.level
  2. Check dl.blocklist[d.Category] is false
  3. If both pass → d.Executed = true, append to in-memory log, append to decisions.jsonl
  4. Return allowed bool
```

If `Propose` returns false, the decision is recorded as "would have done" — full audit trail
with no side effects.

### Actions by Level

| Level | Allowed Actions |
|-------|----------------|
| L0 | No execution; all proposals logged as "would have done" |
| L1 | `DecisionRestart` (session auto-restart); `DecisionFailover` (provider failover) |
| L2 | L1 + `DecisionBudgetAdjust`, `DecisionProviderSelect`, `DecisionConfigChange`, `DecisionSelfTest`, `DecisionReflexion`, `DecisionCascadeRoute`, `DecisionCurriculum`, `DecisionEpisodicReplay` |
| L3 | L2 + `DecisionLaunch` (roadmap-driven cycle launch), `DecisionScale` + HyperagentEngine modifications (confidence >= 0.8, rate-limited to 3/hour) |

### Supervisor Activation Threshold

`SetAutonomyLevel(level >= LevelAutoOptimize, repoPath)` starts the supervisor goroutine.
`SetAutonomyLevel(level < LevelAutoOptimize)` calls `stopSupervisor()`. The supervisor requires
L2 minimum and is where `DecisionLaunch` signals actually run cycles.

---

## 7. Race Condition Risks

### Tier 3 Grep Findings

The package has 70+ distinct mutexes across 60+ files. The conventions are largely followed:
`sync.RWMutex` with `RLock` for reads and `Lock` for writes. Below are the notable risks.

#### Risk 1: AutoRecovery.retryState — MEDIUM

```go
// autorecovery.go
type AutoRecovery struct {
    retryState  map[string]*retryInfo // session ID → retry state
    // no mutex protecting retryState
}
```

`HandleSessionError` reads and writes `retryState` without a lock. It is called from
`optimizer.HandleSessionComplete`, which is invoked from `s.onComplete` — a goroutine
spawned per session in `runSession`. If two sessions for the same session ID (e.g.,
a retry relaunched under the same original ID for tracking) call `HandleSessionError`
concurrently, the map access is a data race. In practice the original session ID is
unique, but `ClearRetryState` is called from a different goroutine path (successful
completion callback) and could race with `HandleSessionError` on a failing session.

**Rating: MEDIUM** — likely benign in typical single-session-per-ID usage, but will
fail under `-race` detection if two sessions share a retry tracking path.

#### Risk 2: Loop.iterationProgressDelta — low

```go
// loop.go
func iterationProgressDelta(run *LoopRun, iterCount int) float64 {
    run.mu.Lock()
    defer run.mu.Unlock()
    ...
}
```

This function is called from `RunLoop` (the main loop goroutine) immediately after reading
`iterCount` outside the lock. The `iterCount` value could be stale by the time the lock is
acquired, but since `RunLoop` is the only goroutine that appends to `run.Iterations` after
startup, this is safe in practice.

**Rating: LOW**

#### Risk 3: Cascade.ShouldCascade with FeedbackAnalyzer — LOW

`ShouldCascade` calls `cr.feedback.GetProviderProfile()` without holding `cr.mu`. The
`FeedbackAnalyzer` has its own `sync.Mutex`, so the call is safe. However, the combined
"slow check then profile check" sequence is not atomic — the latency state and feedback
profile could diverge between the two checks. This is a TOCTOU issue but with no
safety-critical consequences (worst case: occasionally incorrect routing decision).

**Rating: LOW**

#### Risk 4: goroutine fan-out in StepLoop without context propagation — LOW

```go
// loop_steps.go
for i, task := range tasks {
    go func(workerIdx int, t LoopTask) {
        defer func() {
            if r := recover(); r != nil {
                resultCh <- workerResult{...}
            }
        }()
        resultCh <- m.runWorkerTask(workerParams{ctx: ctx, ...})
    }(i, task)
}
```

The `recover()` panic guard is present. Worker goroutines respect `ctx.Done()` via
`p.ctx.Err()` checks before expensive operations. However, the 15-minute
`workerCollectTimeout` is a wall-clock timer, not a context-derived deadline. If the
parent context is cancelled (e.g., `StopLoop`), the workers will eventually detect
`ctx.Done()` but the collector select may block until `workerCollectTimeout` fires
if workers are stuck in a blocking call that doesn't check context. This is mitigated
by `waitForSession` which itself polls context.

**Rating: LOW**

#### Risk 5: supervisor goroutine leak on Stop timeout — LOW

```go
// supervisor.go Stop():
select {
case <-done:
case <-time.After(5 * time.Second):
    slog.Warn("supervisor: stop timed out")
}
s.mu.Lock()
s.running = false
s.mu.Unlock()
```

If the supervisor tick is blocked (e.g., waiting on a cycle that itself is waiting), `Stop`
returns after 5 seconds but the `run` goroutine may still be executing. `s.running = false`
is set, but the goroutine continues until `ctx.Done()` fires. Since `cancel()` is called
before the timeout wait, the goroutine will exit at its next `select` point. The window
between timeout and actual exit is bounded.

**Rating: LOW**

#### Risk 6: GetLoop uses workersMu.Lock() not RLock — LOW

```go
// loop.go
func (m *Manager) GetLoop(id string) (*LoopRun, bool) {
    m.LoadExternalLoops()
    m.workersMu.Lock()  // write lock for a read operation
    defer m.workersMu.Unlock()
    run, ok := m.loops[id]
    return run, ok
}
```

`GetLoop` acquires a write lock (`Lock()`) instead of a read lock (`RLock()`). This is
unnecessarily exclusive and causes all concurrent `GetLoop` calls (from different goroutines
during parallel worker collection) to serialize. It also prevents `ListLoops` from running
concurrently with `GetLoop`. No correctness issue, but this is a performance regression
and deviates from the `sync.RWMutex` convention.

`ListLoops` has the same pattern.

**Rating: LOW** (correctness), **MEDIUM** (performance under concurrent loop access)

#### Risk 7: WriteJournalEntry goroutine — LOW

```go
// runner.go
go func() {
    writeCtx, writeCancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer writeCancel()
    select {
    case <-writeCtx.Done():
        return
    default:
    }
    if err := WriteJournalEntry(s); err == nil && s.bus != nil {
        s.bus.Publish(...)
    }
}()
```

`WriteJournalEntry(s)` is called from a detached goroutine after `s.mu` is released. If
`WriteJournalEntry` internally acquires `s.mu`, there is no deadlock risk. If it reads
session fields without the lock, there could be a data race — this depends on
`WriteJournalEntry`'s implementation (not in the tier-1/2 files reviewed). The context
timeout (10s) bounds the goroutine lifetime.

**Rating: LOW** — bounded, but warrants verifying `WriteJournalEntry` uses `s.mu`.

---

## 8. Store Abstraction

### Store Interface

Defined in `store.go`, the interface covers:

```
SaveSession / GetSession / ListSessions / DeleteSession / UpdateSessionStatus
AggregateSpend
SaveLoopRun / GetLoopRun / ListLoopRuns / UpdateLoopRunStatus
RecordCost / AggregateCostByProvider
Close
```

### MemoryStore

`store_memory.go` — in-memory implementation using `sync.RWMutex`. Fully thread-safe.
Uses `RLock` for read operations and `Lock` for writes. Correct convention.

The `AggregateSpend` method reads `s.SpentUSD` directly from `*Session` pointer without
acquiring the session's own `s.mu`. This is a minor inconsistency — `SpentUSD` could
be concurrently mutated by the runner goroutine. For the MemoryStore, the map and session
pointer are the same objects the Manager holds, so this is a design coupling.

### SQLite Store (SharedState / implicit SQLite store)

`shared_state.go` implements the key-value and distributed lock surface backed by SQLite WAL
mode with `MaxOpenConns(1)` to serialize writes. This is used for the multi-session
coordination blackboard, not the primary session persistence store.

The primary SQLite session store (referenced by `NewManagerWithStore`) is a separate implementation
not in the tier-1/2 files. Its existence is confirmed by `RehydrateFromStore()` which calls
`m.store.ListSessions()` and `m.store.SaveSession()`. Based on the Store interface, the SQLite
store would need `modernc.org/sqlite` (no CGo) matching the pattern from `shared_state.go`.

### What Persists Across Restarts

With SQLite store configured:
- Session records (all exported fields including status, spend, turn count, error, exit reason)
- Loop run records and their iteration history (via `SaveLoopRun`)
- Cost ledger entries (via `RecordCost`)
- Supervisor state (via `.ralph/supervisor_state.json` flat file, not Store)
- Autonomy level (via `.ralph/autonomy.json` flat file, not Store)
- Decision log (via `.ralph/decisions.jsonl` flat file, not Store)
- Improvement patterns (via `.ralph/improvement_patterns.json`)

Without SQLite store (MemoryStore or no store):
- All session and loop run state is lost on restart
- On restart, sessions that were running are marked `StatusInterrupted` by `RehydrateFromStore`
  (which is a no-op without a store, so they simply disappear)

### Migration Path

The current architecture runs dual persistence: Store (when configured) and legacy JSON
files in `~/.ralphglasses/sessions/<id>.json`. `PersistSession` writes to both. The JSON
files serve as the fallback discovery path for `LoadExternalSessions` (multi-process scenario:
TUI discovers sessions launched by MCP server).

Migration to SQLite-only is planned (Phase 10.5 tasks) but not yet complete. The migration
path is clear: once `ListSessions` returns all sessions from SQLite, `LoadExternalSessions`
can be removed and the JSON file writes in `PersistSession` can be gated behind a feature flag.
The blocker is that `LoadExternalSessions` is the cross-process discovery mechanism (TUI ↔ MCP
server share a filesystem, not a process), which would need to be replaced by a shared SQLite
WAL file or a local IPC socket.

---

## Summary of Key Findings

1. **Lock hierarchy is sound**: Three-mutex Manager model (`sessionsMu`, `workersMu`, `configMu`)
   is consistently applied. No cycles observed. The `statusCache sync.Map` hot path is correctly
   maintained alongside the primary map.

2. **GetLoop/ListLoops use write locks unnecessarily**: These methods should use `RLock` for the
   map read. Under concurrent loop access this serializes all readers. Low correctness risk, medium
   performance impact. Trivial fix.

3. **AutoRecovery.retryState lacks a mutex**: Unprotected map accessed from concurrent `onComplete`
   callbacks. Fix by adding `sync.Mutex` to `AutoRecovery` struct.

4. **Cascade confidence threshold is 0.7 (heuristic) or calibrated (requires 50+ samples)**:
   With only Claude data (100% of observed sessions), the DecisionModel will not be trained
   (needs 50+ observations with multi-provider data). All cascade decisions currently use the
   heuristic path.

5. **Supervisor tick interval is 60s in code, but observed at ~27s**: The state file shows 95
   ticks in 43 minutes (~27s/tick). `TickInterval` defaults to 60s but was likely overridden
   during the macOS run. The macOS path anomaly in the state file confirms the supervisor has
   not run on this Manjaro machine.

6. **Store dual-write is transitional**: JSON file persistence and SQLite store coexist.
   Cross-process session discovery (TUI ↔ MCP) depends on JSON files and has no SQLite-backed
   replacement yet.

7. **Bootstrap clamps autonomy to L1**: `.ralphrc` can enable auto-recovery but cannot enable
   L2 (auto-optimize) or L3 (full-autonomy) — those require explicit API calls. This is a
   correct safety design.

8. **JSON format enforcement is the system's top failure mode**: 25 occurrences across 15 cycles.
   The JSON retry loop in `StepLoop` (max 2 retries) addresses this, but the system still fails
   hard if both retries produce freeform text. The planner prompt has been refined, but the
   observed 25.7% retry rate vs the target <5% indicates the prompt engineering gap is not closed.
