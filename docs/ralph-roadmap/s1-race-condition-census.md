# ralphglasses Race Condition Census

**Scope**: `internal/` (all packages, excluding vendor/, .git/)
**Method**: Static analysis via grep + manual code review of goroutine patterns, shared state, and lock discipline
**Date**: 2026-04-04
**Analyst**: Claude (wave 3 research agent, self-improve-review branch)

---

## 1. Summary Table

| # | File | Type | Severity | Fix Effort | Description |
|---|------|------|----------|------------|-------------|
| R-01 | `internal/session/autorecovery.go` | Unprotected map | CRITICAL | S | `retryState` map and `ClearRetryState` accessed from multiple callers with no mutex |
| R-02 | `internal/fleet/retry.go` + `internal/fleet/server_handlers.go` | Unprotected map | CRITICAL | S | `RetryTracker.attempts` map read/written without any lock while HTTP handlers run concurrently |
| R-03 | `internal/session/autooptimize.go` | Global mutable state | HIGH | S | `GateEnabled` and `RunTestGate` are unprotected package-level vars written by supervisor and read under concurrency |
| R-04 | `internal/enhancer/openai_client.go` | Unprotected field | HIGH | S | `OpenAIClient.LastResponseID` written in `Improve()` and read in next call; no mutex if client is shared |
| R-05 | `internal/session/manager_team.go:GetTeam` | Lock ordering | HIGH | M | Acquires `sessionsMu.RLock` then `workersMu.Lock`; other methods use only one; inconsistent ordering is deadlock-prone |
| R-06 | `internal/mcpserver/tools.go` + `tools_dispatch.go` | Unprotected map | HIGH | S | `Server.loadedGroups` map is accessed via concurrent MCP tool calls with only partial `s.mu` coverage |
| R-07 | `internal/session/supervisor.go:tick` | Goroutine leak / no WaitGroup | MEDIUM | M | Two `go func()` launches in `tick()` (RunCycle, RunCycle from planner) have no WaitGroup or context propagation from supervisor's `done` channel |
| R-08 | `internal/knowledge/tiered_knowledge.go` | Read-modify-write in RLock | MEDIUM | S | `hitCount[key]++` executed while holding only `RLock` — concurrent writers will corrupt the map |
| R-09 | `internal/session/manager_stall.go:killWithEscalation` | Goroutine lifecycle | MEDIUM | S | Spawns `cmd.Wait()` goroutine when `done==nil`; if caller also calls `Wait()`, both race on the same cmd |
| R-10 | `internal/session/stall.go` | Channel send in goroutine | MEDIUM | S | StallDetector goroutine may outlive its parent if `Stop()` is never called — the `done` channel pattern is correct but `closeCh` is never closed in the watcher path |
| R-11 | `internal/safety/anomaly_fleet.go:Start` | Cancel field race | MEDIUM | S | `d.cancel = context.WithCancel(ctx)` written in `Start()` without holding `d.mu`; `Stop()` reads `d.cancel` also without lock |
| R-12 | `internal/safety/anomaly.go:Start` | Same cancel field race | MEDIUM | S | Same pattern as R-11 in `AnomalyDetector` |
| R-13 | `internal/enhancer/hybrid.go` | Shared circuit breaker design | MEDIUM | M | `HybridEngine.CB` is a single `CircuitBreaker` shared across all concurrent calls; the CB itself is mutex-protected but all three provider-specific LLM clients (`claude`, `gemini`, `openai`) share one CB instance — one flaky provider trips for all |
| R-14 | `internal/session/runner.go:runSession` | Double `cmd.Wait()` risk | MEDIUM | M | `runSession` calls `s.cmd.Wait()` at line 276; `killWithEscalation` spawns its own `cmd.Wait()` goroutine when `done==nil`; if kill path is invoked while `runSession` is also waiting, both race on `cmd.Wait()` |
| R-15 | `internal/session/manager.go:executeWorkflow` | Local maps not shared | LOW | — | `completed` and `terminal` maps are goroutine-local in `executeWorkflow`; parallel steps go through a channel, so this is safe |
| R-16 | `internal/mcpserver/handler_sweep.go` | Goroutine with closure captures | LOW | S | Background `go func()` captures `ctx` derived from `context.WithCancel`; cancel is registered to `taskID` correctly — low risk but leak window if task registry is not drained |
| R-17 | `internal/session/manager.go` | Unguarded `teams` field (test helpers) | LOW | S | `AddTeamForTesting` and direct `m.teams[...]` in test files bypass `workersMu` in some tests; not a production race |
| R-18 | `internal/tui/view_adapters.go` | Package-level mutable map | LOW | S | `viewDispatch` is a package-level `map[ViewMode]registeredView` written at init time and read concurrently. Safe if `initViewRegistry` is called exactly once before any concurrent read — confirmed single-call pattern in practice |
| R-19 | `internal/session/costnorm.go` | Package-level map with mutex | SAFE | — | `ProviderCostRates` and `claudeBaseRate` are protected by `costRateMu` — correctly implemented |
| R-20 | `internal/knowledge/tiered_knowledge.go` | Mixed atomic + mutex | SAFE | — | `cacheHits`/`cacheMisses` atomics are correct; `cache` + `hitCount` maps are under `mu` — design is safe except for R-08 |

---

## 2. Detailed Findings: CRITICAL and HIGH

### R-01 — `internal/session/autorecovery.go`: `retryState` map unprotected

**Lines**: 54, 103–121, 174–175, 203–205

**Problematic pattern**:
```go
type AutoRecovery struct {
    retryState map[string]*retryInfo  // no mutex
}

func (ar *AutoRecovery) HandleSessionError(ctx context.Context, s *Session) *Session {
    state, ok := ar.retryState[sessionID]  // line 103 — unprotected read
    if !ok {
        state = &retryInfo{}
        ar.retryState[sessionID] = state   // line 106 — unprotected write
    }
    ...
    state.count++          // line 174 — unprotected write
    state.lastRetry = time.Now()  // line 175
}

func (ar *AutoRecovery) ClearRetryState(sessionID string) {
    delete(ar.retryState, sessionID)  // line 204 — unprotected delete
}
```

**Why unsafe**: `HandleSessionError` is called by the supervisor's `tick()` goroutine. `ClearRetryState` may be called by session completion handlers also running concurrently. Go's map type is not safe for concurrent read+write — this is a fatal concurrent map write.

**Recommended fix**: Add `sync.Mutex mu` to `AutoRecovery`. Wrap all `retryState` accesses with `ar.mu.Lock()/Unlock()`.

---

### R-02 — `internal/fleet/retry.go` + `server_handlers.go`: `RetryTracker.attempts` unprotected

**Lines**: retry.go:49–74; server_handlers.go:130, 149

**Problematic pattern**:
```go
// fleet/retry.go
type RetryTracker struct {
    attempts map[string]int  // no mutex
    policy   RetryPolicy
}

func (rt *RetryTracker) RecordFailure(workID string) (bool, time.Duration) {
    rt.attempts[workID]++  // write without lock
    ...
}

// fleet/server_handlers.go — called from HTTP handlers (concurrent)
c.retries.RecordSuccess(item.ID)   // map write, no c.mu held
retryable, delay := c.retries.RecordFailure(item.ID)  // map read+write, no c.mu held
```

**Why unsafe**: The `Coordinator`'s HTTP handlers run concurrently. Both `handleWorkComplete` and `handleWorkPoll` access `c.retries` (the `RetryTracker`) without holding `c.mu`. Multiple concurrent work completions will race on `attempts` map.

**Recommended fix**: Either add `sync.Mutex` to `RetryTracker`, or always access it under `c.mu` in server handlers (currently the budget section acquires `c.mu` separately, but `c.retries` calls straddle those lock sections).

---

### R-03 — `internal/session/autooptimize.go`: Unprotected global `GateEnabled` + `RunTestGate`

**Lines**: autooptimize.go:332, 337; supervisor.go:122

**Problematic pattern**:
```go
// autooptimize.go
var GateEnabled bool      // written in supervisor.Start, read in GateChange
var RunTestGate = defaultTestGate  // written by tests, read during auto-optimization

// supervisor.go:122 — written from supervisor goroutine
GateEnabled = true

// autooptimize.go:354 — read from AutoOptimizer goroutine
if !GateEnabled {
    ...
}
```

**Why unsafe**: `GateEnabled` is written by `supervisor.Start()` (which runs under a mutex) but then read by `GateChange()` without any synchronization. `RunTestGate` is replaced in tests but could also be replaced at runtime without synchronization. Under the race detector (`-race`), this is a data race.

**Recommended fix**: Use `sync/atomic` (`atomic.Bool` for `GateEnabled`) or protect both vars with a dedicated mutex. For `RunTestGate`, an `atomic.Value` suffices.

---

### R-04 — `internal/enhancer/openai_client.go`: `LastResponseID` not mutex-protected

**Lines**: 22, 181

**Problematic pattern**:
```go
type OpenAIClient struct {
    ...
    LastResponseID string  // no mutex — written in Improve(), visible to callers
}

func (c *OpenAIClient) Improve(...) (*ImproveResult, error) {
    ...
    if apiResp.ID != "" {
        c.LastResponseID = apiResp.ID  // write from potentially concurrent Improve() call
    }
    ...
    reqBody := responsesRequest{
        ...
        PreviousResponseID: c.LastResponseID,  // read while another Improve() may be writing
    }
}
```

**Why unsafe**: If the same `OpenAIClient` is used in concurrent goroutines (e.g., batch prompt improvement or concurrent enhancer calls from the MCP server), `LastResponseID` will be read and written simultaneously. The field is exported, meaning callers can also read/write it directly.

**Recommended fix**: Add `mu sync.Mutex` to `OpenAIClient`. Wrap `LastResponseID` accesses. Alternatively, make `LastResponseID` unexported and thread-safe via atomic string.

---

### R-05 — `internal/session/manager_team.go:GetTeam`: Inconsistent lock ordering

**Lines**: manager_team.go:188–191

**Problematic pattern**:
```go
func (m *Manager) GetTeam(name string) (*TeamStatus, bool) {
    m.sessionsMu.RLock()     // FIRST: sessionsMu
    defer m.sessionsMu.RUnlock()
    m.workersMu.Lock()       // SECOND: workersMu
    defer m.workersMu.Unlock()
    ...
}
```

Other code paths acquire `workersMu` alone (e.g., `DelegateTask`, `updateTeamOnSessionEnd`) and `sessionsMu` alone (e.g., `Launch`, `Stop`). If any code path acquires `workersMu` and then tries to acquire `sessionsMu`, a deadlock can occur with `GetTeam`.

**Why unsafe**: Holding two mutexes with inconsistent ordering across goroutines is the classic deadlock recipe. Even without an existing inverse ordering today, adding it later (or through the `correlateTaskStatuses` path which acquires `w.mu` while both are held) creates a 3-mutex ordering issue.

**Recommended fix**: Establish a strict lock hierarchy: `sessionsMu` must always be acquired before `workersMu`. Audit all acquisition sites and enforce via comments. Consider whether `GetTeam` can do a two-phase read: take `workersMu`, copy team data, release, then take `sessionsMu` to look up session.

---

### R-06 — `internal/mcpserver/tools.go` + `tools_dispatch.go`: `loadedGroups` map accessed without `s.mu`

**Lines**: tools.go:61; tools_dispatch.go:47, 70, 82–83, 94, 116, 146, 160

**Problematic pattern**:
```go
// Server.mu is sync.RWMutex (for Repos/lastScanAt), but loadedGroups is:
type Server struct {
    mu           sync.RWMutex  // protects Repos, lastScanAt
    loadedGroups map[string]bool  // accessed WITHOUT mu in handler context
    ...
}

func (s *Server) handleLoadToolGroup(...) {
    if s.loadedGroups[group] {  // read without s.mu
        ...
    }
    s.RegisterToolGroup(s.mcpSrv, group)  // also sets loadedGroups[group]=true inside without s.mu
}

func (s *Server) handleToolGroups(...) {
    ...
    Loaded: s.loadedGroups[g.Name],  // read without s.mu
    ...
}
```

**Why unsafe**: MCP tool calls are dispatched concurrently (one goroutine per call). Two simultaneous `ralphglasses_load_tool_group` calls for different groups will race on `loadedGroups` map reads and writes. `RegisterToolGroup` sets `loadedGroups[group] = true` also without holding `s.mu`.

**Recommended fix**: Either protect `loadedGroups` with `s.mu` in all handlers, or use a `sync.Map` for `loadedGroups`.

---

## 3. Package Risk Heat Map

| Package | Risk Level | Reasoning |
|---------|-----------|-----------|
| `session` | **Risky** | Very large package with complex multi-mutex discipline. `sessionsMu`/`workersMu` ordering inconsistency in `GetTeam`. `autorecovery.go` CRITICAL race. `autooptimize.go` global var races. Background goroutines in supervisor tick lack lifecycle tracking. Session itself is well-protected (per-session `mu`). |
| `fleet` | **Risky** | `RetryTracker.attempts` is a CRITICAL unprotected map. `Coordinator` acquires `c.mu` for budget but not for `c.retries`. HTTP handlers are inherently concurrent. Remaining structures (`workers` map, `budget`) properly guarded. |
| `enhancer` | **Mostly Safe** | `CircuitBreaker` properly mutex-guarded. `HybridEngine` is only risky if shared across goroutines (currently per-call). `OpenAIClient.LastResponseID` is an unprotected field (HIGH, R-04). `PromptCache` has its own mutex. |
| `knowledge` | **Risky** | `TieredKnowledge.hitCount[key]++` inside `RLock` is a read-modify-write race (R-08). Otherwise the structure is properly designed. |
| `mcpserver` | **Risky** | `loadedGroups` map accessed without locking (R-06). MCP server receives concurrent calls. `Server.mu` covers only `Repos`/`lastScanAt`. Task registry needs audit. |
| `safety` | **Mostly Safe** | `AnomalyDetector` and `FleetAnomalyDetector` both have `cancel` field set without mutex in `Start()` (R-11, R-12). All other state properly guarded by `d.mu`. Circuit breaker in `safety` package is exemplary. |
| `bandit` | **Safe** | Has dedicated race tests (`thompson_race_test.go`, `neural_ucb_test.go`). All bandit algorithms guarded with `sync.Mutex`. |
| `blackboard` | **Safe** | Has dedicated race test. Uses `sync.RWMutex` correctly throughout. LRU eviction is properly locked. |
| `distributed` | **Safe** | `DistributedQueue` uses `sync.Mutex` for all map accesses. Background reclaim goroutine uses context properly. |
| `events` | **Safe** | `Bus` uses `sync.RWMutex`. Subscriber maps protected. Transport implementations use channels correctly. |
| `gateway` | **Safe** | `CircuitBreaker`, `RateLimiter`, `DedupCache` all have proper mutex discipline. Dedup has race test. |
| `tui` | **Mostly Safe** | `viewDispatch` package-level map is write-once at init time. `ThemeWatcher` uses mutex. `tui/styles.ThemeMu` is a package-level `sync.RWMutex` used correctly. |
| `workflow` | **Mostly Safe** | `engine.go` fans out steps via goroutines + channel collect pattern (safe). No shared mutable state between workers. |
| `marathon` | **Safe** | `marathon.go` uses channels and proper locking. Cloud scheduler's `jsonUnmarshal` is a package-level var for testability — harmless in production since it's written only during test init. |
| `process` | **Mostly Safe** | `Manager` uses `sync.RWMutex`. `PIDFile` uses atomic ops. `CircuitBreaker` in `process` uses mutex. `killWithEscalation` spawning its own `cmd.Wait()` is a latent double-wait risk (R-09, R-14). |
| `wm` | **Safe** | Hyprland/Sway/i3 event listeners all use proper channel/goroutine patterns with context cancellation. Layout manager uses `sync.RWMutex`. |
| `sandbox` | **Mostly Safe** | `limits.go` atomic counters correct. `logforward.go` uses channels. Firecracker VM uses `sync.Mutex`. |
| `plugin` | **Safe** | Registry uses `sync.RWMutex`. Plugin lifecycle uses channels for graceful shutdown. |
| `batch` | **Safe** | `manager.go` uses `sync.Mutex`. `scheduler.go` uses `sync.Mutex`. All batch state protected. |
| `supervisor (session)` | **Risky** | Supervisor tick fires goroutines for `RunCycle` with no WaitGroup tracking (R-07). `GateEnabled` global var race (R-03). |
| `e2e` | **Safe** | Read-only test infrastructure; no shared mutable production state. |

---

## 4. Recommendations

### Before L2 Autonomy (Must Fix)

These races can corrupt retry state, cause concurrent map panics, or allow the supervisor to make decisions based on stale/corrupt data:

**Priority 1 — Crash-risk races (could cause `fatal error: concurrent map read and map write`):**

1. **R-01** (CRITICAL): Add `sync.Mutex` to `AutoRecovery.retryState`. ~5 lines of change.
   - File: `internal/session/autorecovery.go`
   - Add `mu sync.Mutex` field; wrap all `retryState` accesses.

2. **R-02** (CRITICAL): Add `sync.Mutex` to `RetryTracker` in `fleet/retry.go`. ~5 lines.
   - File: `internal/fleet/retry.go`
   - Add `mu sync.Mutex`; wrap `RecordFailure`, `RecordSuccess`, `Attempts`.

3. **R-08** (MEDIUM): Move `hitCount[key]++` inside write lock in `TieredKnowledge.Query`.
   - File: `internal/knowledge/tiered_knowledge.go:69`
   - Upgrade the `RLock` section to a full `Lock` when a cache hit occurs and hitCount needs updating, OR promote hitCount to `atomic.Int32` values inside the map.

**Priority 2 — Correctness races that affect supervisor decisions:**

4. **R-03** (HIGH): Make `GateEnabled` an `atomic.Bool`; make `RunTestGate` an `atomic.Value`.
   - Files: `internal/session/autooptimize.go`
   - ~3-line change per variable.

5. **R-06** (HIGH): Protect `loadedGroups` map with `s.mu` in `Server`.
   - Files: `internal/mcpserver/tools_dispatch.go`, `tools.go`
   - Add `s.mu.Lock()/Unlock()` around all `loadedGroups` reads/writes.

**Priority 3 — Field races in shared clients:**

6. **R-04** (HIGH): Add `mu sync.Mutex` to `OpenAIClient`; protect `LastResponseID`.
   - File: `internal/enhancer/openai_client.go`
   - ~8-line change.

---

### Before L3 Autonomy (Should Fix)

L3 involves recursive self-modification across multiple repos concurrently. These issues become amplified at that scale:

7. **R-05** (HIGH): Establish and document mutex ordering contract for `Manager`.
   - Files: `internal/session/manager_team.go`, `manager.go`
   - Define: `sessionsMu` > `workersMu` > per-session `mu`. Refactor `GetTeam` to comply.

8. **R-07** (MEDIUM): Track goroutines spawned in `Supervisor.tick()`.
   - File: `internal/session/supervisor.go:229, 242, 367`
   - Add a `sync.WaitGroup` to `Supervisor`; `wg.Add(1)` before each `go func()`, `wg.Done()` at completion. `Stop()` should call `wg.Wait()` after cancelling context.

9. **R-11 + R-12** (MEDIUM): Protect `cancel` field in anomaly detectors.
   - Files: `internal/safety/anomaly.go:139`, `anomaly_fleet.go:211`
   - Assign `d.cancel` while holding `d.mu`, or use an `atomic.Value` wrapper.

10. **R-09 + R-14** (MEDIUM): Audit double `cmd.Wait()` in kill/runner goroutines.
    - Files: `internal/session/manager_stall.go:53`, `runner.go:265`
    - Document that `runSession` always provides `s.doneCh` so `killWithEscalation` never spawns its own `Wait()` goroutine while `runSession` is active.

---

### Patterns Correctly Implemented (Do Not Change)

- `session.Manager` `sessions` map: properly guarded by `sessionsMu`.
- `session.Manager` `teams`/`loops`/`workflowRuns`: properly guarded by `workersMu`.
- `session.Manager` config fields: properly guarded by `configMu`.
- `ProviderCostRates` in `costnorm.go`: properly guarded by `costRateMu`.
- `blackboard.Blackboard`: exemplary `sync.RWMutex` usage, has dedicated race test.
- `bandit.*`: all algorithms have race tests and consistent mutex discipline.
- `enhancer.CircuitBreaker`: mutex-protected, all methods take `cb.mu.Lock()`.
- `safety.CircuitBreaker`: properly uses `sync.Mutex` + `sync/atomic` `Metrics`.
- `events.Bus`: `sync.RWMutex` subscriber map, channel-based delivery — correct.
- `gateway.DedupFilter`: has dedicated race test, channel-based dedup.

---

## 5. Testing Gaps

The following packages have goroutine-heavy code but no dedicated `_race_test.go` files:

- `internal/session/` — has `decision_model_race_test.go` but not for `autorecovery`, `supervisor`, or `stall`
- `internal/fleet/` — `costpredict_race_test.go` exists but does not cover `RetryTracker` or `Coordinator.handleWorkComplete`
- `internal/mcpserver/` — `tools_race_test.go` exists but does not cover `loadedGroups` concurrent load

**Recommended**: Run `go test -race ./internal/session/ ./internal/fleet/ ./internal/mcpserver/ ./internal/knowledge/ -count=5` to surface these races in CI. Adding dedicated race tests for `AutoRecovery`, `RetryTracker`, and `Server.handleLoadToolGroup` would catch regressions.
