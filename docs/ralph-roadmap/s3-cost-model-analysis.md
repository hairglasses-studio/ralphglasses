# S3 -- Cost Model Analysis
Generated: 2026-04-04

Scope: finops audit of cost tracking, budget enforcement, and cost prediction across
`internal/session/`, `internal/fleet/`, and `internal/mcpserver/` packages.

---

## 1. Cost Tracking Architecture

### Data Model

Cost is tracked at three granularities simultaneously:

**Per-session (live)**
The `Session` struct carries `SpentUSD float64` and `BudgetUSD float64` as mutable fields
protected by `s.mu` (a `sync.Mutex`). These are updated by the runner as the provider CLI
streams output. The runner also maintains `CacheReadTokens` and `CacheWriteTokens` for
provider-level cache savings tracking.

**In-memory ledger (`CostLedger`)**
`session/cost_ledger.go` implements an append-only, in-memory `[]CostLedgerEntry` with a
secondary `byID map[string][]int` index. Each entry records: `SessionID`, `Amount`, `Provider`,
`Timestamp`. There is no eviction — the ledger grows unbounded for the process lifetime. It
supports `Total()`, `TotalForSession()`, `Entries()`, `AllEntries()`, `EntriesSince()`.

**Durable ledger (file-backed)**
`BudgetEnforcer.WriteLedgerEntry()` appends JSONL to `.ralph/logs/cost_ledger.jsonl` per repo.
`BudgetEnforcer.WriteCostSummary()` writes `.ralph/cost_summary.json` (last session only).
These are called at session end, not during execution.

**Historical model (`CostHistory`)**
`session/cost_model.go` manages `.ralph/cost_history.json` (up to 500 records per repo).
Provides `AverageCostPerSession()`, `AverageCostPerTurn()`, and `ProjectBudget()` for
budget projection. Loaded at process start, saved on each `Add()`.

**Fleet analytics (`FleetAnalytics`)**
`fleet/analytics.go` holds rolling `completionSample` and `failureSample` arrays (capped at
`maxSamples`, default not shown but initialized by caller). Provides `Snapshot(window)` and
`CostForecast(horizon)`. Cost data comes from `RecordCompletion(workerID, provider, duration, cost)`.

**Fleet predictor (`fleet/CostPredictor`)**
`fleet/costpredict.go` maintains a sliding window of up to 1,000 `CostSample` records.
Computes burn rate, exhaustion ETA, trend direction, and anomalies.

**Session predictor (`session/CostPredictor`)**
`session/costpredictor.go` tracks `(taskType:provider)` → `costStats{count, totalUSD}` in memory
and persists to `.ralph/cost_observations.json`. Provides `Predict(taskType, provider)`.

### Feed Points

| Source | Written by | When |
|--------|-----------|------|
| `sess.SpentUSD` | Runner (output parser) | Continuously during session |
| `CostLedger` | Caller code (not auto-wired) | Manual or end-of-session |
| `cost_ledger.jsonl` | `BudgetEnforcer.WriteLedgerEntry()` | End-of-session (manual call) |
| `cost_summary.json` | `BudgetEnforcer.WriteCostSummary()` | End-of-session (manual call) |
| `cost_history.json` | `CostHistory.Add()` | On session complete callback |
| `fleet/CostPredictor` | Must be called manually | Not auto-wired from coordinator |
| `session/CostPredictor` | `CostPredictor.Record()` | Manual call at session end |
| `FleetAnalytics` | `RecordCompletion()` | Worker completes work item |
| `pool.State` | `pool.State.Update(snapshots)` | Periodic refresh via manager |

The critical gap: `fleet/CostPredictor` is not automatically fed from `handleWorkComplete`.
The coordinator updates `GlobalBudget.SpentUSD` and `BudgetManager.RecordCost()`, but does not
call `predictor.Record()`. This means the fleet-level predictor starts empty on every restart.

---

## 2. Budget Enforcement Points

### Complete Map

| Location | Mechanism | Hard stop? |
|----------|----------|-----------|
| `fleet/server.go:219` — `SubmitWork` | `GlobalBudget.AvailableBudget()` check before item enters queue | Yes — rejects item |
| `fleet/server_handlers.go:207` — `handleWorkPoll` | `GlobalBudget.AvailableBudget()` read before assigning item | Yes — no assignment |
| `fleet/server_dispatch.go:96` — `assignWork` | `AvailableBudget()` + `BudgetManager.Remaining()` per worker | Yes — skips worker |
| `session/manager_lifecycle.go:64` — `Manager.Launch` | `pool.State.CanSpend()` (fleet-node cap) | Yes — returns error |
| `session/manager_lifecycle.go:54` — `Manager.Launch` | Applies `DefaultBudgetUSD` when `MaxBudgetUSD == 0` | Fills default, no block |
| `session/providers.go:100` — `buildClaudeArgs` | `--max-budget-usd` passed to Claude CLI | Primary enforcement (CLI) |
| `session/providers.go:119` — `buildGeminiArgs` | `--budget` passed to Gemini CLI | Primary enforcement (CLI) |
| `session/providers.go:203` — `buildCodexArgs` | `--max-spend` passed to Codex CLI | Primary enforcement (CLI) |
| `session/budget.go:23` — `BudgetEnforcer.Check` | 90% headroom check on `sess.SpentUSD` | Advisory — returns bool |
| `session/runner.go:559` | `DefaultBudgetThresholds` (50/75/90%) alert emission | Event only, no stop |
| `mcpserver/handler_sweep.go:229` — `handleSweepLaunch` | Pre-launch estimate vs `max_sweep_budget_usd` | Yes — rejects sweep |
| `mcpserver/handler_sweep.go:292` — fan-out loop | `sweepPool.Allocate()` before each session launch | Yes — breaks fan-out |
| `mcpserver/handler_sweep.go:595` — `handleSweepSchedule` | Total cost polling vs `max_sweep_budget_usd` | Reactive — stops running sessions |
| `fleet/autoscaler.go:371` | Budget floor check (10% of total) before scale-up | Suppresses scale-up only |
| `fleet/pool/budget_pool.go:46` — `AllocateSession` | Fleet/pool reservation check | Yes — rejects allocation |
| `session/budget_pool.go:54` — `BudgetPool.Allocate` | Sweep pool ceiling | Yes — returns error |
| `internal/config/validate.go:151` | `DefaultBudgetUSD` range check [0, 10000], warning >$1000 | Config-time only |

### Gaps Where Spend Can Bypass Enforcement

**Gap A: `MaxBudgetUSD == 0` path in session launch.**
`handleSessionLaunch` passes `p.OptionalNumber("budget_usd", 0)` — if the caller omits
`budget_usd`, `MaxBudgetUSD` is zero. The manager then fills in `DefaultBudgetUSD` only if
`m.DefaultBudgetUSD > 0`. If `DefaultBudgetUSD` is also zero (the compiled-in default for
`NewManager()` before any `RALPH_SESSION_BUDGET` env var is set), the session launches with
`MaxBudgetUSD = 0`. The providers receive no `--max-budget-usd` flag. The CLI runs without
any spend cap. The session accumulates cost without any hard stop.

**Gap B: Active sessions are not stopped when `GlobalBudget` is exhausted.**
When `GlobalBudget.AvailableBudget()` reaches zero, the coordinator stops assigning new work.
Sessions already executing continue until they complete or time out. If a session was
allocated `MaxBudgetUSD = 10` but spends `$15`, the reservation was `$10` but the overspend
of `$5` is silently accepted. `ReservedUSD -= item.MaxBudgetUSD; SpentUSD += result.SpentUSD`
does not block overspend; it just records it.

**Gap C: The fleet-node `pool.State.CanSpend()` uses stale total.**
`pool.State.Update()` must be called periodically (by the manager's maintenance goroutine) to
refresh `TotalSpentUSD`. Between updates, `CanSpend()` reads a stale value. New sessions can
be launched in this window even if the cap was already exceeded by concurrent sessions.

**Gap D: Sweep `handleSweepSchedule` cap is reactive.**
As noted in Wave 2 (doc 06), the cost cap check in `handleSweepSchedule` runs every
`interval_minutes` (default 5). Sessions can overspend within a polling interval.

---

## 3. Provider Cost Normalization

### Canonical Rate Source

`internal/config/costs.go` is the single source of truth. Compiled-in rates (USD/1M tokens):

| Provider | Model | Input | Output |
|----------|-------|-------|--------|
| Claude | claude-sonnet-4.6 | $3.00 | $15.00 |
| Claude | claude-opus | $15.00 | $75.00 |
| Gemini | gemini-3.1-flash | $0.30 | $2.50 |
| Gemini | gemini-flash-lite | $0.10 | — |
| Codex | gpt-5.4 | $2.50 | $15.00 |

These constants are re-exported in `session/costs.go` for backward compatibility.
`session/costnorm.go` holds the `ProviderCostRates` package variable (protected by `costRateMu`)
loaded at startup from compiled-in defaults, optionally overridden by `.ralph/cost_rates.json`.

### Normalization Approach

`NormalizeProviderCost(provider, rawCostUSD, inputTokens, outputTokens)` normalizes to the
Claude-sonnet baseline:
- If token counts are known: exact formula using claude rates.
- If token counts are zero: blended rate scaling (`rawCost * (claudeBlended / providerBlended)`).
  The blended rate is `(InputPer1M + OutputPer1M) / 2` — a 50/50 heuristic that ignores
  the actual input/output ratio. For Claude with typical 4:1 input/output ratio, this
  overestimates normalization by roughly 15-20%.

### Rate Accuracy Assessment

The Codex rates (`$2.50 input`, `$15.00 output`) are labeled as `gpt-5.4`. As of April 2026,
actual gpt-4.5-preview pricing is $75/$150 and gpt-4o is $2.50/$10.00. If the model labeled
`gpt-5.4` maps to a gpt-4o tier variant, input cost is plausible but output may be
understated. No mechanism exists to alert when compiled-in rates diverge from actual billing.

**Gemini flash output at $2.50/1M tokens** aligns with the 2025 Gemini 2.0 Flash pricing.
Gemini 2.5 Flash (current) is approximately $3.50/1M output. If sessions are using 2.5 Flash,
output costs are underestimated by ~40%.

**Override path**: `.ralph/cost_rates.json` per-repo allows overrides. There is no org-wide
override file, so each repo must maintain its own rates file or accept the compiled-in defaults.

---

## 4. Fleet vs Session Budget Interaction

### Three Scope Levels

```
[1] fleet/GlobalBudget    -- fleet-coordinator-wide ceiling ($500 default)
         |
[2] fleet/BudgetManager   -- per-worker limit ($10 default per worker)
         |
[3] fleet/pool.State      -- local fleet-node cap (0 = unlimited by default)
         |
[4] session/BudgetPool    -- per-sweep ceiling (max_sweep_budget_usd, $100 default)
         |
[5] session/Session.BudgetUSD -- per-session cap ($5 default in sweep; 0 in direct launch)
```

Levels 1-3 operate in the fleet coordinator context. Level 4 exists only for sweep launches.
Level 5 is always present per-session if a budget was specified.

### Interaction: Can Concurrent Sessions Exceed Fleet Budget?

Yes, in three ways:

**Way 1 — Reservation vs actual spend.**
`GlobalBudget.ReservedUSD` increases by `MaxBudgetUSD` at submission and decreases at
completion. If a session spends more than its `MaxBudgetUSD`, the extra spend goes into
`SpentUSD` without corresponding `ReservedUSD`. This means total spend can exceed the limit:
`LimitUSD - SpentUSD - ReservedUSD` can go negative, and `AvailableBudget()` clamps at 0
but does not stop in-flight sessions.

**Way 2 — Race window in `pool.State`.**
`CanSpend()` in `pool.State` reads the last-computed `TotalSpentUSD`. If two sessions complete
simultaneously between `Update()` calls, both report their costs but only one's cost was
reflected in the `TotalSpentUSD` snapshot checked by the second. Neither is blocked.

**Way 3 — Worker-level budget is advisory during assignment scoring.**
`BudgetManager.CanAcceptWork(workerID, estimatedCost)` returns false if the worker is over
budget. However in `assignWork` / `server_dispatch.go:101`, a worker over budget is skipped,
but this only affects assignment. Already-assigned work continues. A worker can accumulate
more spend than its limit through already-running sessions.

---

## 5. Sweep Budget Enforcement: Full Trace

### Default Parameters

```go
// handler_sweep.go handleSweepLaunch defaults
budgetUSD       := p.OptionalNumber("budget_usd", 5.0)         // per-session cap
maxSweepBudget  := p.OptionalNumber("max_sweep_budget_usd", 100.0)
maxTurns        := p.OptionalNumber("max_turns", 50)
model           := session.ProviderDefaults(session.ProviderCodex)  // Codex default
```

The $0.50 per-session cap in `feedback_sweep_cost_control.md` is a caller convention.
The handler default is `$5.00 per session`. If the caller does not pass `budget_usd=0.50`,
sessions run with a $5.00 cap.

### Enforcement Path

**Step 1 — Pre-launch estimate gate.**
```
estimatedPerSession = CostPredictor.Predict("sweep", "codex") OR static formula
  static: (8000 tokens * 0.6 * maxTurns / 1M) * (codexInRate + codexOutRate)
  with maxTurns=50: ~(8000 * 30 / 1M) * (2.50 + 15.00) = 0.24 * 17.5 = $4.20

if estimatedPerSession * repoCount > maxSweepBudget:
    reject ("estimated sweep cost $X exceeds max_sweep_budget_usd $Y")
```

**Step 2 — Auto-size per-session budget.**
```
if budgetUSD < estimatedPerSession * 1.5:
    budgetUSD = estimatedPerSession * 1.5
```
This means: if the static estimate is $4.20, the effective per-session cap becomes `$6.30`
even if the caller passed `budget_usd=5.0`. This silently inflates the per-session cap.

**Step 3 — Sweep pool allocation gate.**
```
sweepPool = session.NewBudgetPool(maxSweepBudget)  // $100 default
for each repo:
    sweepPool.Allocate(repoName, budgetUSD)  // errors if total exceeds ceiling
    if error: break (remaining repos skipped)
    SessMgr.Launch(ctx, opts)
```
The pool ceiling is `max_sweep_budget_usd`. A 10-repo sweep at $5/session uses $50 of the
$100 pool. The pool `Allocate()` checks total-allocated, not total-spent, so it prevents
over-allocation but not overspend (sessions can run over their `MaxBudgetUSD` if the CLI
doesn't hard-stop at the limit).

**Step 4 — CLI-level hard cap.**
`session/providers.go` passes `--max-budget-usd 5.00` to Claude, `--budget 5.00` to Gemini,
`--max-spend 5.00` to Codex. This is the actual enforcement — the CLI refuses to spend more.
This is the only hard stop that works mid-session.

**Step 5 — Runtime sweep schedule cap (optional, reactive).**
`handleSweepSchedule` with `max_sweep_budget_usd > 0` polls every N minutes and stops all
running sessions when `totalCost >= maxCostCap`. This is not wired automatically; the caller
must separately call `ralphglasses_sweep_schedule` with the cap parameter.

### Where the Cap Actually Bites

The only **hard** enforcement for sweep is:
1. The pre-launch estimate gate (blocks before any session starts).
2. The CLI `--max-budget-usd` flag (hard-stops mid-session at the provider level).

The sweep pool allocation (Step 3) prevents over-allocation at launch time, but once a
session is running, it can spend up to its `MaxBudgetUSD` regardless of how much the
other sessions have already spent.

**Maximum theoretical sweep spend without `handleSweepSchedule`:**
With defaults: 10 repos × $6.30 (auto-sized cap) = $63 max. The pool ceiling ($100) is not
hit until 15-16 repos.

---

## 6. Cost Prediction Model

### Session-Layer Predictor (`session/costpredictor.go`)

A simple per-key average: `totalUSD / count` for `(taskType:provider)` pairs.

- **Algorithm**: running mean. No weighted recency. No variance tracking.
- **Storage**: `[]CostObservation` + `map[key]*costStats`. Persisted to `cost_observations.json`.
- **Cold start**: returns `$1.00` default with no data.
- **Used by**: `handleSweepLaunch` via `s.SessMgr.GetCostPredictor().Predict("sweep", "codex")`.
- **Fed by**: `session.CostPredictor.Record()` must be called explicitly. Not auto-wired.

The session predictor is deliberately simple — it converges to true average over time but has
no memory of trend changes (e.g., if task complexity doubled last week, the estimate is still
a blend of old cheap runs and new expensive ones).

### Fleet-Layer Predictor (`fleet/costpredict.go`)

A sliding-window statistical predictor with more features:

- **Algorithm**: burn rate ($/hr from first→last sample timestamp), z-score anomaly detection
  (preceding 20 samples, threshold 2.5σ), trend direction (first-half vs second-half average,
  ±10% threshold), exhaustion ETA (remaining budget / burn rate).
- **Window**: 1,000 samples, FIFO eviction.
- **Cold start**: returns zero burn rate with < 2 samples. `Forecast()` returns a minimal struct.
- **Fed by**: `CostPredictor.Record(CostSample)` — not auto-wired from `handleWorkComplete`.

**Critical gap**: the fleet predictor is instantiated but not automatically fed. The coordinator
updates `GlobalBudget.SpentUSD` and `BudgetManager.RecordCost()` on work completion, but does
not call `predictor.Record()`. The predictor is effectively unused in the default deployment
unless an operator manually wires it.

### Static Formula Fallback

When `session/CostPredictor.Predict()` returns `$1.00` (cold start), `handleSweepLaunch`
falls back to:
```go
estTurns := float64(maxTurns) * 0.6   // assumes 60% turn utilization
tokPerTurn := 8000.0
estimatedPerSession = (tokPerTurn * estTurns / 1_000_000) * (inRate + outRate)
// With maxTurns=50, codex rates: ~$4.20
```

The 0.6 utilization coefficient and 8,000 tokens/turn are hardcoded heuristics. Tasks that
spawn tool use (file reads, grep, git operations) easily exceed 8,000 tokens/turn. The estimate
is likely conservative by 2-3x for audit tasks.

---

## 7. DecisionModel Calibration

### The 50-Sample Requirement

`NewDecisionModel()` sets `minSamples = 50`. `Train()` returns an error
`"insufficient data: need 50 observations, got N"` until the threshold is met. Until trained,
`Predict()` uses the heuristic fallback:
```go
score = 0.30*VerifyPassed + 0.25*(1-HedgeCount) + 0.20*TurnRatio + 0.15*ErrorFree + 0.10*(1-QuestionCount)
```

The heuristic is deterministic and not calibrated — it can overestimate confidence for sessions
that pass verification without having been genuinely difficult.

### Data Collection

`LoopObservation` records are fed into the decision model via `Train(observations)`.
These observations come from the loop engine (`RunLoop`, `StepLoop`) and are persisted to
the session state dir. `observationToFeatures()` extracts: task type hash, provider ID,
turn ratio (from latency proxy), hedge count (from confidence inversion), verify/error flags,
difficulty score, episodes available.

### Training Path

The cascade router uses `DecisionModel.PredictConfidence(turnCount, expectedTurns, lastOutput, verifyPassed)`
to decide whether to escalate to a more expensive provider. With < 50 samples, the heuristic
runs. With ≥ 50 samples, logistic regression (50 epochs, lr=0.01) is trained and isotonic
calibration is applied (10 equal-frequency bins).

### Calibration Accuracy

With exactly 50 samples, each calibration bin contains 5 observations — insufficient for
reliable isotonic calibration. The model is statistically underfit at the minimum threshold.
The `AdaptThreshold()` method searches [0.30, 0.90] in 0.05 steps, penalizing false negatives
(missed failures) at 2x weight. At 50 samples and 10 bins, the threshold search is noisy.

**The 0.7 confidence threshold** in `DefaultCascadeConfig()` is a hardcoded prior, not
derived from data. Below 50 samples, this is the operative threshold regardless of empirical
accuracy.

### Connection to Cost

The cascade router uses the decision model to decide whether to use a cheaper provider (Gemini
or Codex) vs. escalating to Claude. An overconfident decision model (which the untrained
heuristic can produce) routes too many tasks to cheaper providers — underspend but potentially
lower quality. An underconfident model routes too many tasks to Claude — overspend.

---

## 8. Budget Federation

`session/budget_federation.go` implements `FederatedBudget` for managing a total budget across
multiple named sessions with per-session allocations, redistribution, and exhaustion callbacks.

### Design

- `NewFederatedBudget(totalUSD, opts...)` creates a total USD ceiling.
- `WithReservePercent(pct)` holds a fraction in reserve for redistribution.
- `Allocate(sessionID, amount)` reserves budget; fails if `currentAllocated + amount > allocatable`.
- `Spend(sessionID, amount)` records actual spend; fires `onExhaust` callbacks if session exceeds allocation.
- `Redistribute()` reclaims unspent budget from finished sessions and distributes equally to active ones.
- `OnBudgetExhausted(fn)` registers stop callbacks.

### Integration

`FederatedBudget` is NOT used in the current production path. The sweep handler uses
`session.BudgetPool` (ceiling + per-session allocations) rather than `FederatedBudget`.
`FederatedBudget` has full test coverage but no wiring point in `mcpserver` or `fleet`.
It appears to be a more sophisticated version of `BudgetPool` intended for a future
cross-repo or cross-fleet federation scenario.

The redistribution mechanism is valuable: when one repo's session finishes cheaply, its
surplus can fund another repo that needs more. This is not available in the current sweep
(fixed `budgetUSD` per session, no surplus reclaim).

---

## 9. Cost Leakage Scenarios

### Scenario A: Direct Session Launch With No Budget

**Trigger**: `ralphglasses_session_launch` called without `budget_usd` parameter, and
`RALPH_SESSION_BUDGET` environment variable not set (or `DefaultBudgetUSD == 0`).

**Path**:
```
handleSessionLaunch
  → p.OptionalNumber("budget_usd", 0) = 0
  → Manager.Launch(opts{MaxBudgetUSD: 0})
    → if m.DefaultBudgetUSD > 0: apply (skipped if 0)
    → pool.State.CanSpend(DefaultEstimatedSessionCost=0.50): passes if cap is 0 (unlimited)
    → launch() → providers.buildXArgs() → no --max-budget-usd flag passed
```

The session runs indefinitely. The only stop is:
- `BudgetEnforcer.Check()` which returns `false` when `BudgetUSD <= 0`.
- Max turns (`MaxTurns`) if set.
- Manual `session_stop`.

**Maximum spend**: unbounded. A single Claude session can spend hundreds of dollars on a
long-running task if left unattended.

**Probability**: High. The `budget_usd` parameter is documented as optional. Any caller
who omits it and has not set `RALPH_SESSION_BUDGET` gets an uncapped session.

### Scenario B: Unbounded Queue + No Per-Item Budget Check

**Trigger**: Sweep with 74 repos submitted in rapid succession.

**Path**:
```
handleSweepLaunch (repo 1-74)
  → sweepPool.Allocate() checks ceiling
  → sessions launch serially
  → no check: are current sessions within fleet GlobalBudget?
```

The sweep pool (`session.BudgetPool`) and the fleet `GlobalBudget` are separate state stores.
A sweep can exhaust the sweep pool's $100 ceiling while the fleet GlobalBudget (default $500)
still has headroom. Conversely, a sweep can use up `GlobalBudget` headroom without the
fleet coordinator knowing, because the sweep sessions are launched via `session.Manager.Launch`
(local node), not via `fleet/server.go SubmitWork`.

**Maximum spend**: `max_sweep_budget_usd` default $100 per sweep. With 4 concurrent sweeps
(achievable via 4 MCP clients), $400 can be committed before the $500 fleet GlobalBudget
triggers. The fleet GlobalBudget only applies to work submitted via the fleet coordinator;
direct `session_launch` and `sweep_launch` bypass it entirely.

### Scenario C: Sweep Default $5.00 vs Caller Convention $0.50

**Trigger**: Automated agent calls `sweep_launch` without explicitly setting `budget_usd`.

**Result**: Each session gets a $5.00 cap (not $0.50). For a 10-repo sweep:
- Expected spend at $0.50/session: $5.00
- Actual cap at $5.00/session: up to $50.00
- Factor: 10x more than caller convention.

The auto-sizing logic (Step 2 in section 5) may further inflate this:
- Static estimate ~$4.20 for 50 turns → auto-sized cap = $4.20 × 1.5 = $6.30
- 10 repos × $6.30 = $63.00 maximum

### Scenario D: Concurrent Sessions Exceeding `pool.State` Budget Cap

**Trigger**: `pool.State` cap set to $50. 10 sessions launch concurrently. Each checks
`CanSpend(0.50)` — the default estimated cost of $0.50.

**Path**:
```
Session 1: CanSpend(0.50) → TotalSpentUSD=0 → 0.50 <= 50 → OK
Session 2: CanSpend(0.50) → TotalSpentUSD=0 (stale, not yet updated) → OK
...
Session 10: CanSpend(0.50) → TotalSpentUSD=0 (stale) → OK
```

All 10 sessions launch. Each runs to its per-session cap ($5.00). Total actual spend: $50.
The `pool.State.Update()` runs on the manager maintenance goroutine (interval configurable,
not shown in code but typically seconds to minutes). The stale `TotalSpentUSD` used in the
10 concurrent launches reflects pre-launch state. The $50 fleet cap is effectively bypassed.

### Scenario E: Fleet Coordinator Restart Mid-Sweep

**Trigger**: Coordinator process crashes during a 74-repo sweep.

**Impact**: Queue is lost (in-memory, not persisted automatically). Sessions already running
on workers continue spending. `CompleteWork` calls from workers get HTTP 404. No cost is
recorded for these orphaned sessions. The coordinator restarts with `SpentUSD = 0` — all
prior spend is forgotten. New sweeps can launch as if no budget has been consumed.

**Maximum theoretical spend per hour (no human oversight, L3 autonomy):**
With defaults:
- 32 workers × 4 sessions each = 128 concurrent sessions
- Each session with $5.00 cap, average 30-minute runtime
- 2 rotations per hour × 128 sessions = 256 sessions
- 256 × $5.00 = $1,280/hr absolute maximum (Claude rates with no Codex routing)
- At Codex rates ($5/session): $1,280/hr remains the ceiling (same cap)
- With $0.50 caller convention: $128/hr maximum

With the cascade router routing most tasks to Gemini/Codex at $0.17 average (from meta-roadmap):
- 256 sessions × $0.17 = $43.52/hr typical
- But sessions with no budget cap run until they complete (could be hours for complex tasks)

---

## 10. Recommendations

### R1: Enforce a Mandatory Default Budget — HIGH Priority

**Issue**: `session_launch` with no `budget_usd` produces an uncapped session when
`DefaultBudgetUSD == 0`.

**Fix**: In `Manager.Launch()`, change the fallback logic:
```go
// Current:
if opts.MaxBudgetUSD <= 0 && m.DefaultBudgetUSD > 0 {
    opts.MaxBudgetUSD = m.DefaultBudgetUSD
}
// Proposed:
const HardDefaultBudget = 5.0
if opts.MaxBudgetUSD <= 0 {
    if m.DefaultBudgetUSD > 0 {
        opts.MaxBudgetUSD = m.DefaultBudgetUSD
    } else {
        opts.MaxBudgetUSD = HardDefaultBudget
    }
}
```

Set `HardDefaultBudget = 5.0` (current sweep default) to maintain backward compatibility.
For L2/L3 autonomy, this should be configurable in `Config.DefaultBudgetUSD` with no way to
accidentally leave it at zero.

### R2: Wire `fleet/CostPredictor` to `handleWorkComplete` — HIGH Priority

**Issue**: The fleet predictor starts empty on every restart. Cost forecasts, anomaly
detection, and burn-rate tracking are unavailable.

**Fix**: In `handleWorkComplete` (fleet/server_handlers.go), after updating `SpentUSD`:
```go
if c.predictor != nil {
    c.predictor.Record(CostSample{
        Timestamp: time.Now(),
        CostUSD:   result.SpentUSD,
        Provider:  item.Provider,
        TaskType:  item.TaskType,
    })
}
```

### R3: Harden Sweep Budget from Caller Convention to Handler Default — HIGH Priority

**Issue**: The documented convention is `budget_usd=0.50` but the handler default is `$5.00`.
An autonomous agent (L2/L3) that calls `sweep_launch` without explicit parameters runs at 10x
the intended cost.

**Fix**: Change `handler_sweep.go`:
```go
budgetUSD := p.OptionalNumber("budget_usd", 0.50)  // was 5.0
```

And remove or recalibrate the auto-sizing step (Step 2 in section 5) that inflates this
further. Auto-sizing should be opt-in, not default behavior.

### R4: Add Per-Session Budget Check in `session_retry` and `sweep_nudge` — MEDIUM Priority

**Issue**: `handleSessionRetry` inherits `sess.BudgetUSD` from the original session.
`handleSweepNudge` with `action=restart` relaunches with `budget: sess.BudgetUSD` but does
not check sweep pool remaining capacity. A stuck sweep that auto-nudges can relaunch sessions
after the sweep pool is exhausted.

**Fix**: In `handleSweepNudge`, check sweep pool remaining before each restart. If sweep pool
is exhausted, skip restart and log the reason.

### R5: Implement Proactive Session Cost Events — MEDIUM Priority

**Issue**: `BudgetEnforcer` checks run on the session's own `SpentUSD` but only emit events;
they do not trigger any fleet-level action. The sweep schedule cap is purely reactive.

**Fix**: Expose a `session.BudgetExceededEvent` from the runner when 90% headroom is hit.
Subscribe to it in the sweep schedule goroutine to trigger immediate cap check rather than
waiting for the next polling interval.

### R6: Add `BudgetCapUSD` to the Server's `NewManager()` Path — MEDIUM Priority

**Issue**: `pool.State` is initialized with `BudgetCapUSD = 0` (unlimited) in `NewManager()`.
The fleet node has no hard cap unless the operator explicitly calls `SetBudgetCap()`.

**Fix**: Read `Config.DefaultBudgetUSD` (or a new `Config.FleetBudgetCapUSD`) at server startup
and call `m.FleetPool.SetBudgetCap()`. Default to a conservative value (e.g., $100/hr) for
L2/L3 autonomy deployments.

### R7: Minimum `DecisionModel` Sample Threshold for Production — LOW Priority

**Issue**: The model is trained and calibrated at 50 samples with 5 observations per bin.
This is statistically marginal. At the minimum, calibration bins should have at least 10
observations (100 samples minimum).

**Fix**: Increase `minSamples` to 100, or add a confidence interval on calibration bins and
fall back to the heuristic when per-bin count is below 5.

### R8: Rate File Alert When Compiled-In Rates Diverge — LOW Priority

**Issue**: No mechanism alerts when compiled-in provider rates in `config/costs.go` are stale.
Gemini 2.5 Flash output is underestimated by ~40% vs the compiled constant.

**Fix**: Add a `cost_rates_updated_at` field to `.ralph/cost_rates.json`. Log a warning at
startup if the file is older than 30 days or if compiled-in rates differ from file rates by
more than 20%.

---

## Summary

| Risk | Severity | Scenario | Max Spend |
|------|---------|---------|-----------|
| Uncapped session launch (Gap A) | Critical | Any agent omits `budget_usd` | Unlimited |
| Sweep default $5/session vs $0.50 convention (Gap C) | High | Automated sweep | 10x overrun |
| Concurrent session CanSpend race (Gap D) | High | 10+ parallel sessions | Fleet cap bypassed |
| Coordinator restart loses spend history (Scenario E) | High | Process crash | Full re-spend |
| Sweep schedule cap polling delay (Gap D) | Medium | 5-min poll interval | 1 interval × N sessions |
| Fleet CostPredictor not fed (Gap in feed points) | Medium | Burn rate forecasting | No detection |

**Maximum theoretical spend per hour at L3 autonomy (no human oversight):**
- With mandatory $5 default and fleet coordinator: ~$640/hr (128 concurrent × $5 × 1/session)
- With $0.50 default (R3 applied): ~$64/hr
- With `FleetBudgetCapUSD = 100` (R6 applied): $100/hr hard cap regardless of session count
