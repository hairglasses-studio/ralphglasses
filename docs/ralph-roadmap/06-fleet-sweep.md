# 06 -- Fleet and Sweep Subsystems
Generated: 2026-04-04

## 1. Job Processing Pipeline

### Full Flow: submit → queue → dispatch → worker → result

```
[Caller: MCP handler or Coordinator.SubmitWork]
  │
  ├── Budget pre-check: item.MaxBudgetUSD <= GlobalBudget.AvailableBudget()
  ├── WorkQueue.Push(item)              -- in-memory map[string]*WorkItem
  └── GlobalBudget.ReservedUSD += item.MaxBudgetUSD
                │
                ▼
[WorkerAgent.pollLoop -- every 5s]
  └── Client.PollWork(ctx, workerID)
        └── POST /api/v1/work/poll
              └── Coordinator.assignWork(workerID, worker)
                    ├── buildCandidates()         -- Router.SelectWorker for healthy workers
                    └── WorkQueue.AssignBest(scorer, workerID)
                          │  scoring fn: routerBoost + repoLocalityBoost + constraintBoost
                          └── item.Status = WorkAssigned, AssignedTo = workerID
                │
                ▼
[WorkerAgent.executeWork(ctx, item)]
  ├── session.Manager.Launch(ctx, LaunchOptions{...item fields})
  └── Ticker 2s polling loop:
        ├── sess.Status == StatusCompleted →
        │     Client.CompleteWork(WorkCompleted, WorkResult)
        └── sess.Status == StatusErrored/Stopped →
              Client.CompleteWork(WorkFailed, error message)
                │
                ▼
[Coordinator.handleWorkComplete]
  ├── RetryTracker.RecordFailure / RecordSuccess
  ├── GlobalBudget.ReservedUSD -= item.MaxBudgetUSD
  ├── GlobalBudget.SpentUSD += result.SpentUSD
  └── BudgetManager.RecordCost(workerID, result.SpentUSD)
```

### Queue Data Structure

`WorkQueue` is an in-memory `map[string]*WorkItem` protected by a single `sync.Mutex`. There is no separate backing store — all queue state is lost on process restart. `SaveTo/LoadFrom` provide optional JSON file serialization, but the coordinator does not call these automatically; persistence is manual.

The queue is **not bounded**: `Push` appends without capacity enforcement. A burst of sweep submissions can grow the queue to any size until OOM.

**Priority**: `AssignBest` iterates the full pending set and picks the highest-score item. The `Pending()` helper sorts by `item.Priority` descending, but the scoring function used in `assignWork` applies additional locality and constraint bonuses on top of the base priority field.

**FIFO vs Priority**: Effective ordering is priority-first (via int `Priority` field) but not FIFO within the same priority level because map iteration order is random in Go. Items submitted at the same priority are non-deterministically ordered.

---

## 2. DLQ (Dead Letter Queue)

The `WorkQueue` struct contains a separate `dlq map[string]*WorkItem` map alongside the main `items` map. Both share the same `sync.Mutex`.

### Items Enter DLQ Via Three Paths

| Path | Trigger | Error Set |
|------|---------|-----------|
| `ReapStale(maxAge)` | Pending item older than maxAge (default 1h) OR repo path no longer exists | `"reaped: stale task"` |
| `ReapPhantomRepos()` | Pending item with RepoName == "001" or RepoPath base == "001" | `"reaped: phantom repo placeholder"` |
| `MoveToDLQ(itemID)` | Explicit call after `RetryTracker` says no more retries | Item retains its last error string |

The maintenance loop runs every 30s and calls `ReapStale(time.Hour)` and `ReapPhantomRepos()`. Manual `MoveToDLQ` is called from `handleWorkComplete` when `RetryTracker.RecordFailure` returns `retry=false`.

### Retry Policy

`DefaultRetryPolicy()`:
- MaxRetries: 3
- BaseDelay: 1s
- MaxDelay: 30s
- Multiplier: 2.0 (exponential backoff)
- JitterFraction: 0.1 (±10% jitter)

Retry sequence: attempt 0 → 1s, attempt 1 → 2s, attempt 2 → 4s, then DLQ on attempt 3.

`WorkItem.MaxRetries` field (default 2 from `SubmitWork`) is a separate per-item override. Both the `RetryPolicy.MaxRetries` and `WorkItem.MaxRetries` must be checked: the tracker uses `policy.MaxRetries` while item-level cap is an orthogonal guard.

### DLQ Replay

`RetryFromDLQ(itemID)` resets status to WorkPending, clears RetryCount/Error/timestamps, and moves the item back to the main queue. This is exposed via `Coordinator.RetryFromDLQ` and the MCP `fleet_dlq` tool. The DLQ is also purgeable via `PurgeDLQ`. There is no automatic replay or scheduling — replay requires explicit operator action.

---

## 3. Budget Enforcement

### Three Budget Layers

```
Layer 1: GlobalBudget (fleet-wide ceiling)
  ├── LimitUSD:    500.0 (default, adjustable via SetBudgetLimit)
  ├── SpentUSD:    running total of completed work
  └── ReservedUSD: sum of MaxBudgetUSD for pending/active items

Layer 2: BudgetManager (per-worker tracking)
  ├── defaultLimit: $10.00 per worker
  └── budgets map: workerID → WorkerBudget{Limit, Spent}

Layer 3: BudgetPool (sweep-level, per sweep_launch call)
  ├── globalLimit:  max_sweep_budget_usd (default $100.00)
  └── allocations:  repoName → allocatedUSD
```

### Interaction

1. On `SubmitWork`: checked against `GlobalBudget.AvailableBudget()` = `LimitUSD - SpentUSD - ReservedUSD`. Items with `MaxBudgetUSD > available` are rejected with an error.
2. On work assignment in `assignWork`: `GlobalBudget.AvailableBudget()` is read again; if depleted, no item is assigned.
3. On completion: `ReservedUSD` decreases by `item.MaxBudgetUSD`, `SpentUSD` increases by `result.SpentUSD`. If `result.SpentUSD > item.MaxBudgetUSD`, the reservation was insufficient and global spend exceeds reservation.
4. Worker-level: `BudgetManager.CanAcceptWork(workerID, estimatedCost)` is consulted during candidate scoring. Workers over their per-worker limit receive a lower score but are not hard-blocked.
5. Sweep-level: `BudgetPool.Allocate(repoName, budgetUSD)` returns an error when the sweep total would exceed `max_sweep_budget_usd`. This stops the fan-out loop and records the remaining repos as errors.

### What Happens When Budget Exhausted Mid-Job

- **Global budget hit**: `assignWork` stops assigning new work. Active sessions already executing continue to completion (no SIGTERM). Spend can temporarily exceed the limit for already-running work if sessions spend more than their `MaxBudgetUSD`.
- **Session-level cap**: `MaxBudgetUSD` is passed to `session.LaunchOptions`, which triggers extra-usage detection in the runner. The session is marked `StatusErrored` when its per-session cap is exhausted.
- **Sweep budget cap**: The `handleSweepSchedule` cost-cap check polls every `interval_minutes` and calls `Stop` on all running sessions when `totalCost >= maxCostCap`. This is a reactive check, not a proactive gate, so spend may slightly exceed the cap within a polling interval.

---

## 4. Worker Turn Limits (MaxWorkerTurns)

`MaxWorkerTurns` is a loop-engine concept, not a fleet-level concept. It lives in the session loop layer (`session.LoopConfig.MaxWorkerTurns`, capped at 20 in Phase 0.9). In the fleet layer, the analogous field is `WorkItem.MaxTurns`, which is forwarded to `session.LaunchOptions.MaxTurns`.

The fleet worker's `executeWork` function passes `item.MaxTurns` directly to the session manager. Turn limit enforcement happens inside the session runner:

- When `MaxTurns` is reached, the session exits with a specific exit reason (e.g., `"max_turns_reached"`).
- The session status becomes `StatusCompleted` (not `StatusErrored`) — the session is not killed, it self-terminates.
- `executeWork` polls session status every 2 seconds and calls `CompleteWork` with `WorkCompleted` when it sees `StatusCompleted`, regardless of exit reason.

**Hard kill**: Sessions that exceed `MaxTurns` are not hard-killed. The LLM CLI subprocess receives the turn count as a flag and exits cleanly when the limit is hit. There is no SIGKILL fallback for turn exhaustion in the fleet layer.

**MaxWorkerTurns (loop engine, default 20)**: In the loop engine (`RunLoop`), `MaxWorkerTurns` gates the number of `StepLoop` iterations. When the limit is reached, `RunLoop` sets the loop run status to `"completed"` and returns. This is a separate counter from per-session `MaxTurns`.

---

## 5. A2A Protocol Integration

### What Works

The A2A implementation is **substantially complete** for the core protocol surface:

| Component | Status | Notes |
|-----------|--------|-------|
| Agent card serving (`.well-known/agent-card.json`) | Working | `BuildAgentCard` + `handleAgentCard` |
| Task state machine | Working | All 6 states: queued, working, input-required, completed, failed, canceled |
| Task offer lifecycle | Working | `A2AAdapter`: Offer → Accept → StartWorking → Complete/Fail/Cancel |
| Artifact attachment | Working | `AddArtifact` with streaming index support |
| Negotiation (counter-proposals) | Working | `Negotiate` updates `DelegationConstraints` on open offers |
| Skill-based dispatch | Working | `A2ADispatcher`: card discovery, TTL cache (5min), strategy (first/round-robin/best-fit) |
| HTTP task submission (`/api/v1/a2a/task/send`) | Working | `handleA2ATaskSend` creates a fleet work item |
| HTTP task status (`GET /api/v1/a2a/task/{taskID}`) | Working | Returns current offer state |
| Task cancellation | Working | `handleA2ATaskCancel` |
| Push notifications (SSE/streaming) | Stub | `AgentCapabilities.Streaming: true` is advertised but no SSE stream is implemented |
| Authentication (security schemes) | Stub | `SecuritySchemes` field is populated in card but not enforced |
| Multi-turn input-required → resume | Partial | State transition is tracked, but no HTTP endpoint to submit additional input exists |

### What Is Stub/Placeholder

1. **Streaming/push notifications**: The agent card advertises `streaming: true` and `pushNotifications: false`. There is no SSE or WebSocket implementation. The SSE endpoint in `server.go` (`GET /api/v1/events`) emits fleet events, not A2A task events.
2. **Authentication**: The `SecuritySchemes` map in the agent card is populated (Tailscale bearer), but `handleA2ATaskSend` does not validate any A2A-level auth token — it relies on Tailscale network-level auth only.
3. **Input injection for input-required**: `RequestInput` transitions state and records a message, but no HTTP endpoint exists for an external agent to submit the required input and resume execution.
4. **Coordinator integration**: `A2AAdapter` has an optional `coordinator *Coordinator` field (`NewA2AAdapterWithCoordinator`), but `handleA2ATaskSend` creates a `WorkItem` directly via `coordinator.SubmitWork` without going through the `A2AAdapter` offer lifecycle. The two systems are parallel, not integrated.

### Mapping to Google A2A Spec

The implementation aligns with Google's A2A v1.0 protocol:
- Well-known card path changed from `/agent.json` (legacy) to `/.well-known/agent-card.json` (spec-compliant).
- `TaskState` constants match the spec wire values.
- `Message` + `Part` structure matches the spec's TextPart/DataPart/FilePart model.
- `A2ATaskSendRequest` and `A2ATaskResponse` types follow the spec's task/send endpoint contract.

---

## 6. Autoscaler Behavior

### Algorithm

`AutoScaler.Evaluate(AutoScalerSnapshot)` runs on every maintenance tick (every 30s). The decision logic:

```
if cooldown active (within 60s of last action):
  → ScaleNone (reason: "cooldown active")

Scale-up check:
  if active > 0 AND queueDepth > 2.0 * active:
    if (budgetRemaining / budgetTotal) < 0.10:
      → ScaleNone (scale-up suppressed: budget below floor)
    target = max(queueDepth / 2, active + 1) capped at MaxWorkers (32)
    → ScaleUp, delta = target - active

  if active == 0 AND queueDepth > 0:
    → ScaleUp, delta = MinWorkers (2)

Scale-down check:
  if active > MinWorkers AND (idle / active) > 0.50 AND queueDepth == 0:
    target = active - idle, clamped to MinWorkers
    → ScaleDown, delta = -(active - target)

default:
  → ScaleNone (reason: "fleet is balanced")
```

### Scale-Up Action: Advisory Only

Scale-up is **advisory, not actuating**. When `ScaleUp` is decided, the coordinator publishes a `fleet.autoscale` event to the event bus. No new worker processes are spawned. An external orchestration layer (provisioner, K8s HPA, or manual operator action) must watch for this event and spin up new worker nodes. This is a significant gap for single-machine deployments.

### Scale-Down Action: Draining Idle Workers

Scale-down is actuated: `applyScaleDown` marks idle workers as `WorkerDraining`, preventing new work assignment. Workers are not terminated — they continue running until they are deregistered or a heartbeat times out. There is no forced termination signal.

### Max Concurrency

- `MaxWorkers`: 32 (configurable via `AutoScalerConfig.MaxWorkers`)
- Per-worker `MaxSessions`: 4 (hardcoded in `WorkerAgent.Run` RegisterPayload)
- Theoretical max fleet concurrency: 32 × 4 = 128 concurrent sessions

---

## 7. Sweep Orchestration

### Dispatch

`handleSweepLaunch` fans out sessions in a **serial for-loop** inside a goroutine, not concurrently. Sessions for repos [0..N-1] are launched one at a time in order. A failed launch for repo K does not stop the loop; it records an error and continues to K+1.

```
goroutine:
  for _, r := range targetRepos:
    sweepPool.Allocate(r.Name, budgetUSD)   -- if cap reached, break (remaining repos skipped)
    s.SessMgr.Launch(ctx, opts)             -- blocking call, may take seconds
    launched = append(...)
```

The goroutine returns immediately to the MCP caller via a task ID. The caller polls `ralphglasses_tasks_get` or `sweep_status` to observe progress.

### Status Aggregation

`handleSweepStatus` calls `s.sweepSessions(sweepID)`, which does a linear scan of all sessions matching `sess.SweepID == sweepID`. For each session it computes:
- Status counts (running/completed/errored)
- Total cost
- Stall detection (via `s.SessMgr.DetectStalls`)
- Per-session `idle_sec` from `sess.LastActivity`

The result is aggregated in memory — no database or file query. All sessions must still be in-memory for accurate status.

### Budget Enforcement in Sweep

Two layers:

1. **Pre-launch cost estimate**: `handleSweepLaunch` estimates `estimatedPerSession` from `CostPredictor.Predict()` or a formula (`estTurns * tokPerTurn * (inRate + outRate)`). If `totalEstimated > max_sweep_budget_usd`, the launch is rejected before any session starts.

2. **Per-repo allocation**: `session.NewBudgetPool(maxSweepBudget)` (from the sweep-layer `pool` package, which is actually the `session.BudgetPool` — note the sweep handler uses `session.NewBudgetPool`, not `fleet/pool.NewBudgetPool`). `sweepPool.Allocate(r.Name, budgetUSD)` is called before each launch. When the pool is exhausted, the loop breaks and remaining repos are skipped.

3. **Runtime cap**: `handleSweepSchedule` with `max_sweep_budget_usd` checks total spend on every polling interval and calls `Stop` on all running sessions when exceeded.

**The $0.50 cap and --no-session-persistence**: These are caller conventions documented in feedback (memory file: `feedback_sweep_cost_control.md`), not enforced in the handler. The caller must pass `budget_usd=0.50` and `session_persistence=false`. The handler defaults are `budget_usd=5.0` and `session_persistence=false`. So `--no-session-persistence` is the default in the handler; the budget cap requires explicit override.

---

## 8. Cost Prediction

### CostPredictor (fleet layer)

`CostPredictor` in `fleet/costpredict.go` maintains a sliding window of up to 1,000 `CostSample` records. It computes:

- **Burn rate**: total cost / time span of window ($/hour)
- **Trend direction**: compare first-half avg vs second-half avg (±10% threshold = stable/increasing/decreasing)
- **Exhaustion ETA**: remaining budget / burn rate
- **Anomalies**: per-sample z-score against preceding 20 samples, flagged if z > 2.5

**Data feed**: `CostPredictor.Record(CostSample)` must be called by callers. Inspection of the fleet package shows the predictor is instantiated but the integration with `handleWorkComplete` is not wired — the coordinator does not automatically call `predictor.Record` after each completed work item. The predictor appears to be populated manually or from the MCP handler layer.

### Session-Layer Predictor

`s.SessMgr.GetCostPredictor()` is used in `handleSweepLaunch` to estimate per-session cost. This is a separate predictor on the session manager, trained from session-level cost observations.

### Calibration Accuracy

Accuracy depends entirely on sample population. With a fresh installation (no history), the fallback formula is used:

```
estimatedPerSession = (tokPerTurn * estTurns / 1_000_000) * (inRate + outRate)
  = (8000 * 0.6 * maxTurns / 1_000_000) * (codexInRate + codexOutRate)
```

With default `maxTurns=50`: `(8000 * 30 / 1e6) * rates = 0.24 * rates`. Using Codex rates from `config.DefaultProviderCosts()`, this gives a rough estimate that likely understates actual cost for complex tasks (sessions that hit more turns).

There is no feedback loop from actual spend back into the formula calibration — the formula is static. The `CostPredictor` window-based approach will improve accuracy once sufficient samples are collected, but there is no minimum sample threshold before it is used (it returns 0 with < 2 samples, at which point the static formula applies).

---

## 9. Topology and Sharding

### Multi-Node Architecture

The fleet is **distributed-ready at the protocol level but single-machine in practice**:

| Concern | Status |
|---------|--------|
| HTTP coordinator protocol | Implemented; worker polls over HTTP |
| Tailscale network auth | Implemented; `TailscaleAuthMiddleware` with CGNAT detection |
| Consistent hash ring (ShardManager) | Implemented; Ketama MD5 with 128 virtual nodes |
| Worker node migration on join/leave | Implemented; `rebalanceLocked` + `MigrationCallback` |
| Scale-up provisioning | **Not implemented**; scale-up is advisory only |
| Coordinator HA / failover | **Not implemented**; single coordinator node |
| Queue persistence across coordinator restart | **Not implemented**; queue is in-memory only |
| Cross-node session discovery | **Not implemented**; session state is local to each worker |

The `ShardManager` and `ShardMap` with `HashShardStrategy` / `ExplicitShardStrategy` provide the data structures for multi-node session affinity. The `TopologyOptimizer` compares round-robin, affinity, and load-balanced strategies using simulation. These are correct and well-tested, but the coordinator wires `LeastLoadedRouter` (not the topology optimizer) for actual work assignment.

The `topology.go` optimizer is advisory — it produces `SimulationResult` and `Assignment` recommendations but is not connected to the coordinator's `assignWork` path.

### Tailscale Network Topology

Discovery of Tailscale IP uses `GetTailscaleStatus()` (polling Tailscale local API or CLI). Workers advertise their Tailscale IP at registration time. The coordinator stores `TailscaleIP` in `WorkerInfo` but does not use it for routing — routing goes through the coordinator's HTTP server URL, not direct worker-to-worker. This means all work submission must go through the single coordinator node.

---

## 10. Risk Areas

### Risk 1: Unbounded In-Memory Queue — HIGH

`WorkQueue` uses an unbounded `map[string]*WorkItem`. A sweep of 74 repos with large prompts submits 74+ items with no capacity limit. The maintenance loop reaps stale items after 1 hour, but items assigned to workers (status WorkAssigned) are not reaped. If workers disconnect after assignment, items are reclaimed to pending after `ClaimTimeout` (5 minutes), but this still allows unbounded accumulation during normal operation.

**Impact**: Memory exhaustion under heavy sweep load. No backpressure to sweep launcher.
**Fix**: Add a `MaxDepth` parameter to `WorkQueue` with rejection on overflow.

### Risk 2: Queue Not Persisted Across Restart — HIGH

The coordinator's `WorkQueue` is in-memory with no automatic persistence. A coordinator restart loses all pending and assigned work. Sessions already running on workers continue executing, but their work items are gone — `CompleteWork` calls from workers will receive HTTP 404 ("work item not found"). The fleet enters an inconsistent state where workers complete work the coordinator has no record of.

**Impact**: Any coordinator restart during active sweeps loses all pending work and orphans in-flight results.
**Fix**: Wire `SaveTo/LoadFrom` in the maintenance loop (30s interval) and on `Stop`.

### Risk 3: RetryTracker Has No Mutex — HIGH

`RetryTracker.attempts` is a plain `map[string]int` with no mutex:

```go
type RetryTracker struct {
    attempts map[string]int
    policy   RetryPolicy
}
```

`RecordFailure` and `RecordSuccess` are called from HTTP request goroutines (one per `handleWorkComplete` request). These goroutines run concurrently and can race on the `attempts` map.

**Impact**: Data race detected by `go test -race`. Map concurrent write panic in production under high load.
**Fix**: Add `sync.Mutex` to `RetryTracker`.

### Risk 4: Scale-Up Is Advisory with No Actuator — HIGH

When the autoscaler decides `ScaleUp`, it publishes an event to the in-process event bus. Nothing consumes this event to spawn new workers. In a single-machine deployment, there is no way to add worker capacity automatically. The event is logged and dropped.

**Impact**: Under sustained load, the queue depth exceeds `QueueDepthMultiplier * active` indefinitely. The autoscaler keeps recommending scale-up every cooldown period (60s), but capacity never increases.
**Fix**: Add a local worker spawner that can `exec` additional ralphglasses worker instances on the same machine, or document clearly that scale-up requires external orchestration.

### Risk 5: Worker executeWork Has No Session Timeout — MEDIUM

`executeWork` polls session status every 2 seconds with no upper bound on how long it will wait:

```go
ticker := time.NewTicker(2 * time.Second)
for {
    select {
    case <-ctx.Done(): return
    case <-ticker.C: ... check status ...
    }
}
```

If a session enters a state that never becomes terminal (e.g., stuck in `StatusLaunching` due to a hung subprocess), `executeWork` will block that goroutine indefinitely. The worker's `MaxSessions=4` slots are consumed by these stuck goroutines.

**Impact**: Worker capacity exhaustion. Up to 4 goroutines stuck forever per worker node.
**Fix**: Add a `context.WithTimeout` wrapper around the polling loop. A reasonable timeout is `2 * session.DefaultStallThreshold` (currently ~15 minutes).

### Risk 6: Sweep Cost Cap is Reactive, Not Proactive — MEDIUM

`handleSweepSchedule`'s cost cap check runs every `interval_minutes` (default 5). A session can spend beyond `max_sweep_budget_usd / n_repos` during one polling interval before the cap triggers. With expensive sessions and a 5-minute interval, overspend can be significant.

**Impact**: Sweep budgets can be exceeded by up to `(n_sessions_running * budget_per_session)` before the cap fires.
**Fix**: Subscribe to session cost events and trigger cap enforcement immediately when threshold is crossed, rather than on a polling interval.

### Risk 7: Sweep Fan-Out Is Serial — LOW

`handleSweepLaunch` launches sessions in a for-loop without goroutine fan-out. With 74 repos, each `s.SessMgr.Launch` call takes time (process spawn + startup probe up to 5 seconds). Total launch time could exceed 6 minutes before all sessions are running.

**Impact**: Slow sweep startup. Sessions launched first may complete before last sessions even start, distorting cost/completion estimates.
**Fix**: Parallelize the launch loop with a bounded goroutine pool (e.g., semaphore of size 10).

### Risk 8: A2A Adapter and Coordinator Are Parallel, Not Integrated — LOW

`handleA2ATaskSend` creates work items via `coordinator.SubmitWork` directly, bypassing the `A2AAdapter` lifecycle entirely. The `A2AAdapter` offer map and the coordinator work queue are two separate state stores for "the same" A2A task.

**Impact**: A2A task status queries (`GET /api/v1/a2a/task/{taskID}`) reflect offer state in `A2AAdapter`, which is not updated as the work item progresses through the coordinator queue. A task submitted via A2A appears stuck in "submitted" state even after the coordinator assigns and completes the work.
**Fix**: Wire `handleWorkComplete` to call `adapter.CompleteOffer` or `adapter.FailOffer` when the corresponding work item finishes.

### Risk 9: Grafana Metrics Are Declared but Not Emitted — LOW

`grafana.go` generates a valid Grafana dashboard JSON referencing Prometheus metrics like `ralphglasses_session_completions_total`, `ralphglasses_cost_usd_total`, `ralphglasses_budget_spent_usd`. These metric names are referenced in PromQL queries but there is no Prometheus client instrumentation in the fleet package that actually emits these counters. The `tracing` package provides a `PrometheusRecorder`, but its metric names are not confirmed to match the dashboard's expected names.

**Impact**: Dashboard panels show "No data" unless metric names are aligned with what the tracing layer actually exports.
**Fix**: Audit the `tracing/observability` packages and ensure metric names match, or update the dashboard template to use the actual emitted names.

### Risk 10: Coordination Uses /tmp Without Cleanup — LOW

`fleet/coordination.go` writes claim files to `/tmp/ralphglasses-coordination/claims/`. Claim files are removed by `ReleaseClaim`, but this requires explicit caller cleanup. A crashed process leaves orphaned claim files that block re-claiming resources indefinitely (no TTL on claims).

**Impact**: Stale claim files survive process crashes and prevent resource acquisition on restart.
**Fix**: Add a `Timestamp` to `Claim` (already present) and enforce a TTL in `IsResourceClaimed` (e.g., reject claims older than 10 minutes).

---

## Summary Table

| Topic | Current State | Key Gap |
|-------|--------------|---------|
| Queue structure | In-memory map, priority-scored, JSON-persistable manually | Unbounded, not auto-persisted |
| DLQ | Inline in WorkQueue, 3-retry default with exponential backoff | Retry replay requires manual operator action |
| Budget | 3-layer (global/worker/sweep), reservation accounting | Sweep cap is reactive (polling), not proactive |
| MaxWorkerTurns | Loop engine: 20 iterations; fleet: MaxTurns forwarded to session | Graceful exit (not SIGKILL) when limit hit |
| A2A | Core protocol complete; streaming and auth enforcement are stubs | Adapter and coordinator queue are not integrated |
| Autoscaler | Full algorithm implemented; scale-down actuated; scale-up advisory | No actuator for scale-up; purely event/advisory |
| Sweep dispatch | Serial fan-out; pool pre-allocation; cost estimate gates | Serial launch adds ~5s latency per repo |
| Cost prediction | Sliding window CostPredictor + static formula fallback | Predictor not auto-fed from fleet completions |
| Topology | Consistent hash ring, topology optimizer, multi-strategy routing | Optimizer not connected to assignWork path |
| Distributed readiness | Protocol-complete; Tailscale auth; ShardManager | Single coordinator SPOF; no HA; queue not durable |
