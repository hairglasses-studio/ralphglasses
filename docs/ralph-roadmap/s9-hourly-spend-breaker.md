# s9 -- Per-Hour Spend Circuit Breaker (L3 Gate G3.2)

Generated: 2026-04-04
Prerequisite for: L3 full autonomy (`14-autonomy-path.md` exit criterion 3)
Addresses: `$1,280/hr theoretical maximum spend` identified in `14-autonomy-path.md` section 1, L3.

---

## 1. Current Budget Tracking Architecture

Four budget systems exist in the codebase today. None of them operate on a rolling hourly
window.

### 1a. BudgetEnvelope (`internal/session/supervisor_budget.go`)

Tracks cumulative spend for a single supervisor run. No time dimension.

- `TotalBudgetUSD`: hard ceiling for the lifetime of the supervisor
- `PerCycleBudgetUSD`: soft cap per cycle launch (defaults to `TotalBudget/10`)
- `RecordSpend(delta)`: adds delta to a running sum protected by `sync.Mutex`
- `CanSpend(estimated)`: returns true if `remaining >= estimated`
- Listens on `events.CostUpdate` via `SubscribeToBus` to compute deltas from cumulative
  per-session `spent_usd` values

Used by: `Supervisor.shouldTerminate()` and `Supervisor.launchCycle()`. Is the correct
hook point for the hourly breaker check on cycle launch.

### 1b. BudgetPool (`internal/session/budget_pool.go`)

Per-session allocation pool with a shared ceiling. No time dimension.

- `Allocate(sessionID, amount)`: reserves budget; rejects if ceiling would be exceeded
- `Record(sessionID, cost)`: records actual spend
- `ShouldPause(sessionID)`: returns true if session or pool ceiling is hit
- Consumed by `BudgetPoller`, which calls a `PauseFunc` when ceiling is reached

Used by: session-level enforcement. Not wired to the supervisor or L3 launch path directly.

### 1c. FleetPool (`internal/fleet/pool.State`, `internal/session/manager.go`)

Fleet-wide budget cap checked at session launch.

- `FleetPool.CanSpend(estimated)`: gates `Manager.Launch()` -- this is the primary
  session launch gate today
- `FleetPool.SetBudgetCap(cap)`: configured from `FLEET_BUDGET_CAP_USD` in `.ralphrc`
- Updated by `Manager.RefreshFleetState()` from `SnapshotSessions()`

Gap identified in `s3` (Gap B): active sessions continue spending after the cap is
exhausted because the cap is only checked at launch time, not proactively enforced while
sessions run.

### 1d. FleetAnomalyDetector (`internal/safety/anomaly_fleet.go`)

The closest existing mechanism to what we need. Detects `MultiRepoSpendSpike` using a
configurable time window (`SpendSpikeWindowMinutes`, default 15). However:

- Default threshold is `$50.00` in a 15-minute window (equivalent to `$200/hr`)
- It fires an `AnomalyDetected` event and optional kill switch, but is not wired as a
  hard gate on `Manager.Launch()`
- `RecordSpend()` must be called explicitly -- it is not automatically fed from the event bus
- `Check()` is called on a periodic `Start()` interval, not synchronously on launch

### 1e. BudgetFederation (`internal/session/budget_federation.go`)

Cross-repo surplus redistribution for sweeps. Not relevant to hourly rate limiting.

### Summary: What Is Missing

No existing system:
1. Tracks spend in a rolling 1-hour window
2. Hard-blocks `Manager.Launch()` when the rolling hourly rate exceeds a threshold
3. Optionally stops running sessions when the threshold is crossed
4. Emits a HITL notification event (`events.AnomalyDetected` or a new event type)
5. Auto-recovers when the 1-hour window's spend drops back below threshold

---

## 2. Rolling Window Implementation

### 2a. Sliding Window vs. Bucketed

**Sliding window**: maintains a timestamp-ordered slice of spend events and trims entries
older than 1 hour on every check. The `FleetAnomalyDetector.recentSpend` field already
uses this pattern exactly (`[]spendEvent{CostUSD, Timestamp}`). Simple, precise, and
correct for sub-second spend events. Memory: O(N) where N = number of spend events in the
last hour. At L3 with 128 sessions spending over 1 hour, N is bounded by the number of
`CostUpdate` events -- roughly one per session completion, so O(200) at most.

**Bucketed (1-minute buckets, 60 slots)**: pre-allocates 60 `float64` slots and uses a
circular index. Simpler eviction (just zero the slot being overwritten), but less precise
(a spend event at minute boundary can appear in either the current or next bucket,
introducing up to 1-minute drift in the window boundary). Also complicates partial-window
fill at startup.

**Decision: sliding window.** The codebase already uses this pattern in
`FleetAnomalyDetector`. Precision matters when the threshold is $50/hr -- a 1-minute
bucket error is $0.83 at that rate, which is acceptable, but a sliding window requires
no additional complexity.

### 2b. SpendRateMonitor Struct

New type in `internal/safety/` alongside `circuit_breaker.go`:

```go
// SpendRateMonitor tracks fleet spend in a rolling window and trips when the
// hourly rate exceeds a configured threshold. It is the primary L3 safety gate.
type SpendRateMonitor struct {
    mu        sync.Mutex
    cfg       SpendRateConfig
    events    []spendPoint   // sorted ascending by timestamp
    tripped   bool
    trippedAt time.Time
    trippedAt float64        // rate at trip time, for logging
    bus       *events.Bus
    ks        *KillSwitch    // optional; engaged when StopAllOnBreach is true
    now       func() time.Time
}

type SpendRateConfig struct {
    WindowDuration  time.Duration  // rolling window size; default 1h
    ThresholdUSD    float64        // spend in window that triggers breach; default 50.0
    StopAllOnBreach bool           // if true, engage kill switch on breach; default false
    AutoReset       bool           // if true, reset when window spend drops below threshold; default true
}

type spendPoint struct {
    ts    time.Time
    costUSD float64
}
```

**Thread safety**: single `sync.Mutex` (not `RWMutex`) because the critical path
(launch gate check) always reads and writes in the same lock: it reads the window sum,
and may update the `tripped` state atomically in one operation, preventing a TOCTOU race.

### 2c. Core Operations

```go
// RecordSpend adds a spend event to the rolling window.
// Called from BudgetEnvelope.handleCostEvent (already on bus listener goroutine).
func (m *SpendRateMonitor) RecordSpend(costUSD float64)

// WindowSpend returns the total spend in the current rolling window.
func (m *SpendRateMonitor) WindowSpend() float64

// Check evaluates the current window spend and trips or resets the breaker.
// Returns (tripped bool, windowSpendUSD float64, hourlyRateUSD float64).
// This is the synchronous gate called at launch time.
func (m *SpendRateMonitor) Check() (bool, float64, float64)

// Tripped returns the current trip state without mutating anything.
func (m *SpendRateMonitor) Tripped() bool

// Reset manually clears the trip state. For operator recovery or tests.
func (m *SpendRateMonitor) Reset()
```

**Auto-reset logic**: When `AutoReset` is true (default), `Check()` clears `tripped` if
the window spend has fallen back below threshold. This means a brief cost spike (e.g.,
one expensive session completing) that crosses the threshold momentarily will self-heal
as older events age out of the window. The window is 1 hour -- once the expensive events
are 60+ minutes old, the rate drops and launches resume automatically.

If `AutoReset` is false, the trip is permanent until `Reset()` is called manually
(operator acknowledgement required). This is the safer choice for unattended L3: a human
must explicitly clear the trip after reviewing the spend event.

**Recommendation for L3 default**: `AutoReset: false`. At L3, a breach means the
$1,280/hr theoretical maximum is in play. Require human acknowledgement before resuming.

---

## 3. Integration Points

### 3a. Session Launch Gate (`Manager.Launch`)

In `internal/session/manager_lifecycle.go`, `Manager.Launch()` already has a fleet
budget gate at line 63-73. The hourly breaker check slots in immediately after:

```go
// After fleet pool check, before optimizer:
if m.spendMonitor != nil {
    if tripped, windowSpend, rate := m.spendMonitor.Check(); tripped {
        slog.Error("hourly spend breaker tripped: rejecting session launch",
            "window_spend_usd", windowSpend,
            "hourly_rate_usd", rate,
            "threshold_usd", m.spendMonitor.cfg.ThresholdUSD,
        )
        return nil, fmt.Errorf("hourly spend limit exceeded: $%.2f/hr (threshold: $%.2f/hr)",
            rate, m.spendMonitor.cfg.ThresholdUSD)
    }
}
```

`m.spendMonitor` is a new field on `Manager` (type `*safety.SpendRateMonitor`).
Nil-guarded so existing code paths with no monitor configured are unaffected.

### 3b. Supervisor Tick Gate (`Supervisor.launchCycle`)

In `supervisor.go`, `launchCycle()` already checks `budget.CanSpend()` before launching
a cycle. Add the same breaker check here:

```go
if mgr != nil && mgr.SpendMonitor() != nil {
    if tripped, windowSpend, rate := mgr.SpendMonitor().Check(); tripped {
        slog.Error("supervisor: hourly spend breaker tripped; skipping cycle launch",
            "window_spend_usd", windowSpend, "rate_usd_per_hr", rate)
        s.publishEvent(events.AnomalyDetected, map[string]any{
            "anomaly_type": "hourly_spend_breach",
            "severity":     "critical",
            "window_spend": windowSpend,
            "rate":         rate,
        })
        return
    }
}
```

This means the supervisor tick is naturally polled every 60 seconds (the `TickInterval`),
so the breach is detected within one tick of the threshold being crossed. No additional
polling goroutine is required.

### 3c. BudgetEnvelope Integration

`BudgetEnvelope.handleCostEvent()` already processes `events.CostUpdate` events and calls
`RecordSpend(delta)`. Extend it to also call `spendMonitor.RecordSpend(delta)` when a
monitor is configured:

```go
func (be *BudgetEnvelope) handleCostEvent(evt events.Event, sessionSpent map[string]float64) {
    // ... existing delta calculation ...
    if delta > 0 {
        be.RecordSpend(delta)
        if be.spendMonitor != nil {
            be.spendMonitor.RecordSpend(delta)
        }
    }
}
```

Alternatively, the `SpendRateMonitor` can subscribe directly to `events.CostUpdate` on
the bus (same as `BudgetEnvelope.SubscribeToBus`). This avoids coupling the monitor to
`BudgetEnvelope`. Preferred approach: independent subscription, so the monitor works
without a supervisor-managed `BudgetEnvelope`.

### 3d. FleetAnomalyDetector Coordination

The existing `FleetAnomalyDetector.checkSpendSpike()` fires a `MultiRepoSpendSpike`
anomaly at the configured threshold. The new `SpendRateMonitor` is a harder gate -- it
actively blocks launches, not just alerts.

These two can coexist:
- `FleetAnomalyDetector`: warning at a lower threshold (e.g., 70% of the breaker threshold)
- `SpendRateMonitor`: hard block at the configured threshold

The `FleetAnomalyDetector` should have its default `SpendSpikeThresholdUSD` set to 35.0
(70% of the default $50/hr) so it fires an early warning before the breaker trips.

### 3e. HITL Event

When the breaker trips, publish an `events.AnomalyDetected` event with:

```go
events.Event{
    Type: events.AnomalyDetected,
    Data: map[string]any{
        "anomaly_type":       "hourly_spend_breach",
        "severity":           "critical",
        "window_spend_usd":   windowSpend,
        "hourly_rate_usd":    rate,
        "threshold_usd":      cfg.ThresholdUSD,
        "recommended_action": "review session costs; call ralphglasses_circuit_reset or wait for window to expire",
    },
}
```

The TUI already renders `AnomalyDetected` events. This surfaces immediately in the
operator's session list view. An existing MCP tool `ralphglasses_circuit_reset` can be
wired to call `spendMonitor.Reset()` for operator-initiated recovery.

---

## 4. Configuration

### 4a. Config Keys (`.ralphrc`)

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `HOURLY_SPEND_THRESHOLD_USD` | float | `50.0` | Rolling 1-hour spend cap before breaker trips |
| `HOURLY_SPEND_WINDOW_MINUTES` | int | `60` | Window size in minutes (60 = 1 hour, no reason to change) |
| `HOURLY_SPEND_STOP_ALL` | bool | `false` | Engage kill switch (stop all running sessions) on breach |
| `HOURLY_SPEND_AUTO_RESET` | bool | `false` | Auto-reset when window spend drops below threshold |

`HOURLY_SPEND_STOP_ALL` defaults to `false` because stopping in-flight sessions is
destructive. At L3 with 128 sessions, a sudden kill of all sessions wastes all accumulated
context and work. The safer default is to block new launches while letting current sessions
drain. Set to `true` for production unattended deployments where cost control is paramount.

### 4b. Manager.ApplyConfig Extension

Add to `Manager.ApplyConfig()` in `manager.go` alongside the existing `FLEET_BUDGET_CAP_USD`
block:

```go
if raw := cfg.Get("HOURLY_SPEND_THRESHOLD_USD", ""); raw != "" {
    if f, err := strconv.ParseFloat(raw, 64); err == nil && f > 0 {
        cfg := safety.SpendRateConfig{
            WindowDuration: 60 * time.Minute,
            ThresholdUSD:   f,
        }
        if raw2 := cfg.Get("HOURLY_SPEND_STOP_ALL", ""); raw2 == "true" {
            cfg.StopAllOnBreach = true
        }
        if raw3 := cfg.Get("HOURLY_SPEND_AUTO_RESET", ""); raw3 == "true" {
            cfg.AutoReset = true
        }
        m.spendMonitor = safety.NewSpendRateMonitor(cfg, m.bus, nil)
        slog.Info("hourly spend breaker configured", "threshold_usd", f)
    }
}
```

### 4c. Programmatic API

For tests and embedding environments:

```go
// NewManagerWithSpendBreaker creates a manager with the hourly breaker pre-configured.
// Useful in integration tests and the TUI startup path for L3 mode.
func NewManagerWithSpendBreaker(bus *events.Bus, thresholdUSD float64) *Manager {
    m := NewManagerWithBus(bus)
    m.spendMonitor = safety.NewSpendRateMonitor(safety.SpendRateConfig{
        WindowDuration: 60 * time.Minute,
        ThresholdUSD:   thresholdUSD,
        AutoReset:      false,
        StopAllOnBreach: false,
    }, bus, nil)
    return m
}
```

---

## 5. Recovery Behavior

### 5a. Auto-Reset (AutoReset: true)

The monitor clears `tripped` when `Check()` finds `windowSpend < ThresholdUSD`. Because
the window is 1 hour, old spend events fall off automatically. At a sustained $50/hr burn
rate, the window is always at threshold. At a burst rate ($200 in 10 minutes, then
silence), the window clears after ~50 minutes of silence.

Implication: auto-reset allows burst-and-pause patterns to bypass the intent of the cap.
A fleet that spends $50 and then idles for an hour can spend another $50, repeating
indefinitely. This is acceptable if the threshold is meant as a rate cap ($50/hr sustained)
but not if it is meant as an absolute daily budget ($1,200/day).

**For L3 validation**: use `AutoReset: false`. The breaker stays tripped until explicitly
reset via MCP tool or process restart.

### 5b. Manual Reset

`ralphglasses_circuit_reset` MCP tool calls `m.SpendMonitor().Reset()`. This should
require the operator to acknowledge the spend event in the HITL history
(`ralphglasses_hitl_history`) before the reset is permitted.

Proposed gate in the MCP handler:

```
1. Check that the breach event is in HITL history
2. Require operator to call ralphglasses_hitl_score with positive score to acknowledge
3. Only then call spendMonitor.Reset()
```

### 5c. Kill Switch Integration (StopAllOnBreach: true)

When `StopAllOnBreach` is true, the monitor calls `ks.Engage(reason)` on trip. The kill
switch publishes `events.EmergencyStop`. The TUI and supervisor both listen for
`EmergencyStop` and halt all session launches and supervisor ticks. Running sessions
receive SIGTERM via the process group signal path in `Manager.StopAll()`.

The existing `KillSwitch.Disengage()` can be wired to `SpendRateMonitor.Reset()` so the
kill switch is lifted when the operator manually resets the breaker.

---

## 6. Estimated Effort

| Task | Size | Notes |
|------|------|-------|
| `safety.SpendRateMonitor` struct + sliding window implementation | S | ~80 lines, mirrors `FleetAnomalyDetector.recentSpend` pattern exactly |
| `SpendRateMonitor.SubscribeToBus(ctx, bus)` goroutine | S | ~30 lines, mirrors `BudgetEnvelope.SubscribeToBus` |
| `Manager.spendMonitor` field + nil-guarded launch gate in `manager_lifecycle.go` | S | ~15 lines |
| `Supervisor.launchCycle` gate + `AnomalyDetected` event on breach | S | ~20 lines |
| `Manager.ApplyConfig` extension for 4 new config keys | S | ~30 lines |
| `NewManagerWithSpendBreaker` convenience constructor | XS | ~10 lines |
| `ralphglasses_circuit_reset` MCP tool wiring to `spendMonitor.Reset()` | S | handler exists; add reset call + HITL acknowledgement gate |
| Unit tests: sliding window, auto-reset, trip/reset cycle, concurrent `RecordSpend` + `Check` | M | ~150 lines, use `now` func override for time control |
| Integration test: launch gate blocks at threshold, unblocks after window expires | M | ~80 lines, uses fake clock |
| `FleetAnomalyDetector` default threshold adjustment to 70% of breaker threshold | XS | config default change |
| `events.bus.go` knownEventTypes: no new event type needed; reuse `AnomalyDetected` | -- | no code change |

**Total estimated effort: M (1-2 days)**

All constituent patterns already exist in the codebase. This is assembly, not invention.

---

## 7. Risks and Open Questions

### 7a. Spend Data Latency

`CostUpdate` events are emitted by sessions when they write to the ledger, which happens
asynchronously on session completion or periodic flush. At L3 with 128 concurrent sessions,
there may be a lag between actual API billing and the event reaching the monitor. The
monitor could undercount spend in-flight. Mitigation: the fleet pool `SnapshotSessions()`
already reads live `SpentUSD` from session state -- the monitor's `RecordSpend` can be
supplemented by a periodic scan of session snapshots to catch in-flight spend.

### 7b. Monitor Wired Outside Supervisor Lifecycle

The `SpendRateMonitor` should subscribe to the bus independently of the supervisor
starting or stopping. If attached to `BudgetEnvelope.SubscribeToBus`, it only works when
the supervisor is running (L2+). For L3, the monitor should be started in
`Manager.Init()` alongside `InitPIDFiles()` and orphan sweeping, so it runs regardless
of autonomy level.

### 7c. Multiple Managers

In the current architecture, the TUI and the MCP server share one `Manager` instance.
There is only one `SpendRateMonitor` per Manager instance. If multiple managers are
created (e.g., tests), each has its own monitor. This is correct behavior.

### 7d. Relationship to G3.2

`14-autonomy-path.md` gate G3.2 says: "Fleet budget ceiling enforced: `FleetBudgetCapUSD`
in `NewManager()` path". The `SpendRateMonitor` is complementary to `FleetBudgetCapUSD`,
not a replacement:

- `FleetBudgetCapUSD` (existing): absolute total spend ceiling across the fleet lifetime
- `SpendRateMonitor` (new): rolling hourly rate ceiling

Both are required for L3. G3.2 as written covers the absolute cap; the hourly rate cap
is the specific new requirement from the research sweep ($1,280/hr max, $50/hr breaker).

---

## 8. Summary

The per-hour spend circuit breaker is a new `safety.SpendRateMonitor` type using a
sliding window identical to the existing `FleetAnomalyDetector` pattern. It integrates
at two points: (1) `Manager.Launch()` as a synchronous gate, and (2)
`Supervisor.launchCycle()` as a pre-flight check. Feed is from `events.CostUpdate` via
independent bus subscription. HITL notification reuses `events.AnomalyDetected`. Default
configuration: $50/hr threshold, 1-hour window, block-new-only (no kill switch), manual
reset required. Operator recovery via `ralphglasses_circuit_reset` MCP tool with HITL
acknowledgement gate. Estimated effort: 1-2 days.
