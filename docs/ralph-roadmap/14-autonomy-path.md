# 14 -- Autonomy Path: L0 through L3

Generated: 2026-04-04

Critical-path analysis for graduated autonomy in ralphglasses. This document defines
what must be true at each level transition, what safety interlocks exist (and what is
missing), and the realistic timeline for reaching 72-hour unattended operation.

---

## 1. Current State Assessment

### L0 (Observe) -- Functional

L0 is the default and works correctly today. All autonomous decisions are logged as
"would have done" entries in `decisions.jsonl`. The `DecisionLog.Propose()` flow
(autonomy.go:213-229) correctly gates on `dl.level >= d.RequiredLevel` and records
blocked proposals with full audit trail. `BootstrapAutonomy` clamps `.ralphrc` values
above L1 back to L1, preventing accidental L2/L3 activation (autonomy.go:65).

**What works:**
- Decision audit log with JSONL persistence (decisions.jsonl)
- Bootstrap clamping (L0/L1 only from config)
- Health monitoring (5 metrics with configurable thresholds)
- HITL tracking (manual vs autonomous action counts, trend scoring)
- Session lifecycle state machine (6 states, clean terminal detection)

**What is incomplete:**
- Autonomy level persistence fails silently (s4 fix #4: `slog.Warn` on persist failure
  in autonomy.go:191, level resets to L0 on restart)
- Decision outcome loop is not closed for most decisions (only `HandleSessionCompleteWithOutcome`
  closes outcomes; supervisor cycle decisions and fleet decisions do not)

### L1 (AutoRecover) -- Partially Functional, Not Safe

L1 enables `DecisionRestart` (auto-restart on transient error) and `DecisionFailover`
(provider failover). The `AutoRecovery` struct (autorecovery.go) implements retry with
exponential backoff and transient error pattern matching.

**What works:**
- Transient error detection (13 patterns: connection reset, timeout, 429, 503, signal:killed)
- Exponential backoff with configurable cooldown (default 30s base, 2x factor)
- Max retry limit (default 3)
- Decision log integration (restart proposals go through `DecisionLog.Propose`)
- Remaining budget calculation for relaunched sessions

**What is broken:**
- **R-01 (CRITICAL)**: `AutoRecovery.retryState` map has no mutex (s1). `HandleSessionError`
  and `ClearRetryState` are called from concurrent goroutines (supervisor tick vs session
  completion callbacks). This is a fatal concurrent map write that will crash under `-race`
  and potentially in production.
- **Supervisor swallows RunCycle errors** (s4 fix #1): supervisor.go:375 logs `RunCycle`
  failures at Warn level and continues. A chain of failing L1 recovery attempts generates
  no escalation signal, no HITL interrupt, no autonomy level demotion. The system silently
  retries broken cycles forever.
- **RunLoop error buried** (s4 fix #2): `handler_selfimprove.go:71` discards the RunLoop
  error entirely (`_ = s.SessMgr.RunLoop(...)`). A self-improvement loop that starts and
  immediately fails is invisible to the operator and to the autonomy system.
- **Hook exit codes discarded** (s4 fix #6): `hooks/hooks.go:139` uses `_ = cmd.Run()`.
  Pre/post-session hooks are intended as safety gates, but silent failure makes them
  decorative. At L1 where hooks control auto-recovery behavior, this breaks the gate
  contract.

### L2 (AutoOptimize) -- Not Ready

L2 enables auto-adjustment of budgets, providers, and rate limits via `AutoOptimizer`.
The supervisor starts at L2+ and runs a 60-second tick loop that evaluates health metrics
and chains R&D cycles.

**What works:**
- `AutoOptimizer.OptimizedLaunchOptions` adjusts provider and budget from FeedbackAnalyzer
  profiles, gated through `DecisionLog.Propose`
- `GateChange` validates L2 changes against the test suite before applying
- `ImprovementNote` generation and auto-application pipeline with rollback on gate failure
- Supervisor tick architecture: health monitor, cycle chainer, sprint planner, stall handler
- Budget envelope with per-cycle cap and termination conditions (MaxCycles, MaxDuration, cost)
- Cooldown enforcement (5-minute minimum between cycle launches)

**What is broken or incomplete:**
- **3 budget gaps** (s3 gaps A/B/C): (A) Sessions can launch with no budget cap when
  `DefaultBudgetUSD == 0` and caller omits `budget_usd`. (B) Active sessions continue
  spending after GlobalBudget is exhausted. (C) `pool.State.CanSpend()` reads stale totals.
- **Gemini Flash output rate 40% off** (s3 section 3): Compiled-in rate is $2.50/1M output;
  actual 2.5 Flash rate is ~$3.50. Cost normalization systematically underestimates Gemini
  sessions, corrupting cascade routing decisions.
- **DecisionModel untrained** (03 section 4, s3 section 7): Requires 50+ multi-provider
  observations. Current data is 100% Claude. Heuristic fallback operates at hardcoded
  confidence threshold 0.7, not calibrated to actual performance.
- **Supervisor has not run on Manjaro** (03 finding 5): Supervisor state file shows 95
  ticks from a macOS run. The supervisor has never executed on the target deployment machine.
  Unknown integration issues with Manjaro-specific paths, process management, GPU detection.
- **R-03 (HIGH)**: `GateEnabled` and `RunTestGate` are unprotected package-level vars
  (s1). `GateEnabled` is written by `supervisor.Start()` and read concurrently by
  `GateChange()`. Race detector will flag this.
- **R-05 (HIGH)**: `GetTeam` acquires `sessionsMu.RLock` then `workersMu.Lock`,
  inconsistent with other code paths. Deadlock-prone under concurrent team operations.
- **R-06 (HIGH)**: `Server.loadedGroups` map accessed without mutex in concurrent MCP
  tool calls. Two simultaneous `load_tool_group` calls race on the map.
- **Sweep default $5 vs $0.50 convention** (s3 scenario C): An autonomous L2 agent
  calling `sweep_launch` without explicit `budget_usd` runs at 10x the intended cost.

### L3 (FullAutonomy) -- Not Ready

L3 enables `DecisionLaunch` (roadmap-driven cycle launch), `DecisionScale` (team
scaling), and HyperagentEngine modifications with confidence >= 0.8 and rate limit of
3/hour.

**What works:**
- `AutoMergeAll` mode for fully unattended self-improvement (loop.go profile)
- Cycle chaining with depth cap of 10 and lineage tracking
- Sprint planner for roadmap-driven task generation
- Acceptance gates for post-cycle validation

**What is broken or incomplete:**
- All L1 and L2 issues above, amplified by scale
- **6 total race conditions** (s1): 2 CRITICAL + 4 HIGH. At L3 concurrency (up to 128
  sessions), these become crash risks, not theoretical concerns
- **$1,280/hr theoretical maximum spend** (s3 scenario E): 32 workers x 4 sessions x $5
  cap x 2 rotations/hour. No fleet-level hard ceiling prevents this.
- **Sweep is serial** (06 risk 7): 74-repo sweep takes 6+ minutes to launch all sessions.
  At L3 scale this serialization bottleneck means the first sessions finish before the
  last ones start.
- **Autoscaler is advisory-only** (06 risk 4): Scale-up publishes an event; nothing
  consumes it to spawn workers. L3 fleet scaling has no actuator.
- **Queue not persisted** (06 risk 2): Coordinator restart loses all pending work. At L3,
  a coordinator crash during a fleet sweep orphans results and loses budget tracking.
- **Supervisor tick goroutines untracked** (s1 R-07): `RunCycle` goroutines launched in
  `tick()` have no WaitGroup. `Stop()` cannot wait for in-flight cycles to complete.
  At L3, stopping the supervisor may leave orphaned cycles with no cleanup.

---

## 2. Gate Criteria: L0 to L1 (Auto-Recovery)

**Objective**: Reliable auto-restart for transient errors without human intervention.

### Must Fix (Hard Gate)

| ID | Issue | Source | Effort |
|----|-------|--------|--------|
| G1.1 | Add `sync.Mutex` to `AutoRecovery.retryState` | s1 R-01 (CRITICAL) | S (5 lines) |
| G1.2 | Surface supervisor cycle failures at Error level; emit event; demote autonomy after N consecutive failures | s4 fix #1 | M |
| G1.3 | Propagate RunLoop error in background goroutines (channel or structured log) | s4 fix #2 | S |
| G1.4 | Fix autonomy level persistence failure (retry with backoff or return error to caller) | s4 fix #4 | S |
| G1.5 | Add hook exit code handling (log at Error, surface to orchestrator) | s4 fix #6 | S |

### Must Verify (Soft Gate)

| ID | Verification | Source |
|----|-------------|--------|
| V1.1 | `go test -race ./internal/session/ -count=5` passes with R-01 fix applied | s1 |
| V1.2 | Auto-recovery integration test: inject transient error, verify retry, verify max-retry exhaustion | autorecovery.go |
| V1.3 | Decision log correctly records "would have done" entries at L0, executed entries at L1 | autonomy.go |
| V1.4 | Provider failover integration test: primary fails, secondary succeeds | autorecovery.go:171 |
| V1.5 | Autonomy level survives process restart (save + load roundtrip) | autooptimize.go:21-54 |

### Exit Criteria

L1 is gated open when:
1. All G1.x fixes are merged and tests pass
2. All V1.x verifications pass on Manjaro
3. A 4-hour unattended L1 session completes with at least one auto-recovery event
   and no crashes, no silent failures, no orphaned sessions

---

## 3. Gate Criteria: L1 to L2 (Auto-Optimize)

**Objective**: The system adjusts its own providers, budgets, and configuration based
on feedback profiles, validated by test gates, without human tuning.

### Must Fix (Hard Gate)

| ID | Issue | Source | Effort |
|----|-------|--------|--------|
| G2.1 | Enforce mandatory default budget ($5 hard floor when `DefaultBudgetUSD == 0`) | s3 R1, Gap A | S |
| G2.2 | Change sweep handler default from `$5.00` to `$0.50` per session | s3 R3, Gap C | S |
| G2.3 | Wire `fleet/CostPredictor.Record()` to `handleWorkComplete` | s3 R2 | S |
| G2.4 | Update Gemini 2.5 Flash output rate from $2.50 to $3.50 in `config/costs.go` | s3 section 3 | S |
| G2.5 | Make `GateEnabled` an `atomic.Bool` | s1 R-03 (HIGH) | S (3 lines) |
| G2.6 | Protect `loadedGroups` map with `s.mu` in MCP server | s1 R-06 (HIGH) | S |
| G2.7 | Add `sync.Mutex` to `RetryTracker.attempts` in fleet | s1 R-02 (CRITICAL) | S (5 lines) |
| G2.8 | Fix `GetTeam` lock ordering (two-phase read or single-lock) | s1 R-05 (HIGH) | M |
| G2.9 | Add `mu sync.Mutex` to `OpenAIClient` for `LastResponseID` | s1 R-04 (HIGH) | S |

### Must Complete (Functional Gate)

| ID | Requirement | Source |
|----|------------|--------|
| F2.1 | Run supervisor on Manjaro for 24+ hours with no crashes | 03 finding 5 |
| F2.2 | Collect 50+ multi-provider loop observations for DecisionModel training | 03 section 4, s3 section 7 |
| F2.3 | Validate cascade routing accuracy: cheap-provider completion rate >= 70% on routed tasks | 03 section 3 |
| F2.4 | BudgetEnforcer.Check 90% headroom alert triggers a fleet-level event (not just a log) | s3 R5 |
| F2.5 | Proactive sweep cost check (event-driven, not 5-minute polling) | s3 Gap D, 06 risk 6 |
| F2.6 | `go test -race ./internal/... -count=3` passes with all G2.x fixes | s1 |

### Training Requirements

The DecisionModel (decision_model.go) needs calibration data before L2 can make
informed cascade routing decisions:

- **50 observations minimum** (configured at `minSamples = 50`)
- **Multi-provider**: current data is 100% Claude. Need observations from Gemini and
  Codex sessions to calibrate the ProviderID feature
- **Calibration bin quality**: At 50 samples with 10 bins, each bin has 5 observations.
  Isotonic calibration is unreliable at this density. Recommend increasing `minSamples`
  to 100 for production L2 (s3 R7)
- **Threshold adaptation**: The 0.7 hardcoded confidence threshold should be replaced by
  `AdaptThreshold()` once trained, which searches [0.30, 0.90] with 2x penalty on
  false negatives

**Collection plan**: Run 25 Gemini Flash sessions, 15 Codex sessions, and 10 Claude
sessions on real tasks (sweep across 5-10 repos with VerifyCommands) to build a
balanced training set. Each session generates 1+ LoopObservation records.

### Exit Criteria

L2 is gated open when:
1. All G2.x fixes and F2.x requirements are met
2. DecisionModel is trained on 50+ multi-provider observations
3. Cascade routing has been validated with real tasks (70%+ cheap-provider completion)
4. Supervisor has run 24+ hours on Manjaro with no silent failures
5. Fleet CostPredictor correctly forecasts burn rate (within 30% of actual)

---

## 4. Gate Criteria: L2 to L3 (Full Autonomy)

**Objective**: The system operates for 72+ hours without human intervention, launching
from roadmap, scaling teams, and merging its own PRs via AutoMergeAll.

### Must Fix (Hard Gate)

| ID | Issue | Source | Effort |
|----|-------|--------|--------|
| G3.1 | All 6 races from s1 fixed (R-01 through R-06) | s1 | M (aggregate) |
| G3.2 | Fleet budget ceiling enforced: `FleetBudgetCapUSD` in `NewManager()` path | s3 R6 | M |
| G3.3 | Supervisor tick goroutines tracked with WaitGroup | s1 R-07 (MEDIUM) | M |
| G3.4 | Sweep launch parallelized with bounded goroutine pool (semaphore, size 10) | 06 risk 7 | M |
| G3.5 | Queue persistence (auto-save every 30s, restore on startup) | 06 risk 2 | M |
| G3.6 | Worker executeWork timeout (2x DefaultStallThreshold, ~15 min) | 06 risk 5 | S |
| G3.7 | Anomaly detector cancel field races fixed | s1 R-11, R-12 | S |
| G3.8 | Double `cmd.Wait()` in kill/runner path documented and prevented | s1 R-09, R-14 | M |
| G3.9 | Fleet worker `CompleteWork` retry with exponential backoff | s4 fix #3 | M |
| G3.10 | Marathon checkpoint persistence failure surfaced as event | s4 fix #10 | S |
| G3.11 | Session rehydration failure at startup returned as error, not swallowed | s4 fix #15 | S |

### Must Complete (Functional Gate)

| ID | Requirement | Source |
|----|------------|--------|
| F3.1 | Autoscaler has a local actuator (spawn worker instances on same machine) | 06 risk 4 |
| F3.2 | Proactive per-session cost events trigger fleet-level cap enforcement | s3 R5 |
| F3.3 | `FederatedBudget` wired to sweep path for surplus redistribution | s3 section 8 |
| F3.4 | Coordinator graceful restart preserves queue and spend tracking | 06 risk 2, s3 scenario E |
| F3.5 | Context-aware NATS publish retry or at minimum counter metrics | s4 fix #14 |
| F3.6 | DecisionModel trained on 100+ observations with per-bin count >= 10 | s3 R7 |
| F3.7 | Sweep concurrent (not serial) with backpressure (queue depth limit) | 06 risks 1, 7 |
| F3.8 | 48-hour unattended L2 run completes with no human intervention required | Integration test |
| F3.9 | `hitCount[key]++` race in TieredKnowledge fixed (promote to atomic or full Lock) | s1 R-08 |

### Exit Criteria

L3 is gated open when:
1. All G3.x fixes and F3.x requirements are met
2. `go test -race ./... -count=5` passes across all packages
3. Fleet budget ceiling limits spend to a configured $/hr rate
4. A 48-hour unattended L2 run completes successfully (prerequisite experience)
5. Autoscaler can both scale up (spawn) and scale down (drain) workers
6. Queue survives coordinator restart with zero work items lost
7. All supervisor failure paths produce Error-level logs and events (no silent Warn-and-continue)

---

## 5. Safety Interlocks at Each Level

### L0 -- Observe

| Interlock | Status | Description |
|-----------|--------|-------------|
| Bootstrap clamp | Working | `.ralphrc` cannot set level > 1 |
| Decision audit | Working | All proposals logged as "would have done" |
| No side effects | Working | `Propose()` returns false; no actions taken |

**Missing:** Nothing critical. L0 is passive by design.

### L1 -- Auto-Recovery

| Interlock | Status | Description |
|-----------|--------|-------------|
| Max retry limit | Working | Default 3 retries, configurable via `AutoRecoveryConfig` |
| Exponential backoff | Working | 30s -> 60s -> 120s between retries |
| Transient-only retry | Working | Only 13 known transient patterns trigger retry |
| Remaining budget cap | Working | Relaunch uses `budget - spent`, not full original budget |
| Category blocklist | Working | `DecisionLog.Block(DecisionRestart)` disables recovery |

**Missing:**
- **Autonomy demotion circuit breaker**: If auto-recovery fails N times consecutively,
  the system should demote from L1 to L0 and alert the operator. Currently it exhausts
  retries silently and stops retrying, but stays at L1.
- **Retry state mutex** (R-01): Without the fix, concurrent retries corrupt the map.

### L2 -- Auto-Optimize

| Interlock | Status | Description |
|-----------|--------|-------------|
| Test gate (GateChange) | Working | Changes validated against `go test ./...` before applying |
| Budget envelope | Working | Per-cycle cap and total spend ceiling in supervisor |
| Cooldown | Working | 5-minute minimum between cycle launches |
| Chain depth cap | Working | Hard limit of 10 chained cycles |
| Concurrency cap | Working | MaxConcurrent = 1 supervisor-launched cycle per repo |
| Decision log | Working | Every tick writes a decision record with rationale and metrics |
| Termination conditions | Working | MaxCycles, MaxDuration, MaxTotalCostUSD all checked per tick |

**Missing:**
- **Fleet-level hard budget ceiling**: `GlobalBudget.LimitUSD` prevents new assignments
  but does not stop running sessions. There is no "kill all sessions" circuit breaker
  when spend exceeds a threshold.
- **Cascade routing confidence floor**: The heuristic threshold (0.7) is not backed by
  data. An untrained model routing 70%+ of tasks to cheap providers may produce
  unacceptable quality. Need a minimum sample gate before cascade routing activates.
- **Supervisor health self-check**: The supervisor monitors repo health but does not
  monitor its own health (tick latency, error rate, goroutine count). A degraded
  supervisor continues making decisions.
- **Cost rate staleness alert**: Compiled-in provider rates can diverge from actual billing.
  The Gemini Flash output rate is already 40% off. No mechanism alerts the operator.

### L3 -- Full Autonomy

| Interlock | Status | Description |
|-----------|--------|-------------|
| AutoMergeAll guard | Partial | Gated on VerifyCommands passing, but no human review |
| Chain depth 10 | Working | Prevents infinite recursive cycles |
| L3 rate limit | Designed | HyperagentEngine modifications limited to 3/hour at confidence >= 0.8 |
| L3 requires API call | Working | Cannot be set from `.ralphrc`; requires `SetAutonomyLevel` |

**Missing (critical for 72-hour unattended):**
- **Per-hour spend circuit breaker**: Must hard-kill all sessions if spend rate exceeds
  a configurable $/hr threshold (e.g., $100/hr). Currently no such mechanism exists.
  The $1,280/hr theoretical maximum is unacceptable for unattended operation.
- **Watchdog process**: The supervisor runs inside the same process. If the process
  crashes, all safety interlocks are gone. A separate watchdog (systemd unit or cron)
  must monitor the supervisor and alert/restart on failure.
- **Network isolation for L3**: `AutoMergeAll` merges PRs without human review. At L3,
  the system must run in an isolated environment (container, VM, dedicated machine)
  where a catastrophic merge can be reverted without affecting production.
- **Git revert capability**: If an AutoMerge'd PR breaks the test suite on the next
  iteration, the system must be able to revert the merge automatically. Currently there
  is no auto-revert mechanism.
- **Rollback on autonomy escalation failure**: If L3 is set and the first action fails,
  there is no automatic demotion. The system should fall back to L2 (or L1) if L3-specific
  operations fail consecutively.

---

## 6. Minimum Viable L3: 72-Hour Unattended Operation

The smallest subset of work for safe 72-hour unattended operation, assuming a single
machine (Manjaro, dual RTX 4090) targeting 3-5 repos.

### Must Have (non-negotiable)

| Subsystem | Requirement | Why |
|-----------|------------|-----|
| **Budget** | Per-hour spend circuit breaker at $50/hr | Prevents $1,280/hr worst case |
| **Budget** | Mandatory $5 default on all sessions (Gap A closed) | No uncapped sessions |
| **Budget** | Sweep default $0.50/session (Gap C closed) | No 10x cost surprise |
| **Races** | R-01 and R-02 fixed (CRITICAL map races) | Process crash prevention |
| **Races** | R-03 through R-06 fixed (HIGH races) | Correct autonomous decisions |
| **Supervisor** | Cycle failure escalation (not just Warn) | Prevents silent spin |
| **Supervisor** | RunLoop error propagation | Self-improvement failures visible |
| **Supervisor** | 48-hour successful L2 run as prerequisite | Proven stability |
| **Watchdog** | systemd unit monitoring supervisor process | Restart on crash |
| **Compaction** | Claude compaction enabled for marathon sessions | Context window not exhausted |
| **Queue** | Persistence every 30s | Survives coordinator restart |

### Should Have (important but deferrable to L3.1)

| Subsystem | Requirement | Why |
|-----------|------------|-----|
| **Autoscaler** | Local worker spawner | Dynamic capacity |
| **Sweep** | Parallel launch with semaphore | Faster 74-repo sweeps |
| **A2A** | Agent card publishing | External discoverability |
| **FederatedBudget** | Surplus redistribution in sweeps | Better budget utilization |
| **Git** | Auto-revert on test regression | Self-healing on bad merges |

### Can Defer (L3.2+)

| Subsystem | Requirement | Why |
|-----------|------------|-----|
| **Topology** | Multi-machine fleet | Single machine is sufficient for 3-5 repos |
| **A2A** | Full task lifecycle integration | Single-org usage does not need A2A |
| **K8s** | Kubernetes fleet deployment | Manjaro thin client is the target platform |
| **Batch API** | Default batch mode for sweeps | Optimization, not safety |

### Hardened Configuration for 72-Hour Run

```
autonomy_level:       3
max_duration:         72h
max_total_cost_usd:   500.00
per_hour_cap_usd:     50.00     # circuit breaker (new)
per_session_budget:   5.00      # hard default
sweep_budget_usd:     0.50      # per-session in sweeps
max_concurrent:       1          # supervisor cycles
max_workers:          4          # fleet workers (single machine)
max_sessions_per_worker: 2      # conservative for dual GPU
chain_depth_cap:      5          # reduced from 10 for first 72h run
cooldown:             10m        # increased from 5m for safety
tick_interval:        60s
compaction:           true
auto_merge_all:       false      # first 72h: create PRs but require manual merge
```

Note: `auto_merge_all: false` for the first 72-hour run. Fully unattended merging
(AutoMergeAll) is a separate gate after proving the system does not produce regressions
over 72 hours of PR creation.

---

## 7. Risk Matrix

### L1 Risks

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| Concurrent map crash (R-01) | High (under race detector; possible in production if two sessions share retry path) | Critical -- process crash, all sessions lost | Fix R-01 before L1 activation |
| Infinite silent retry | Medium (transient error that recurs: e.g., persistent rate limit) | Medium -- budget consumed on doomed retries | Max retry limit (3) exists; add autonomy demotion after exhaustion |
| Recovery launches uncapped session | Low (remaining budget calculation is correct) | High -- unbounded spend | Verify remaining budget is passed to `--max-budget-usd` flag in all providers |
| Autonomy level resets on restart | High (persist failure is swallowed) | Medium -- drops to L0, loses auto-recovery capability | Fix persist failure handling |

### L2 Risks

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| Uncapped session launch (Gap A) | High (any caller omitting `budget_usd`) | Critical -- unbounded spend | G2.1: mandatory $5 default |
| Stale cost rates underestimate Gemini | Certain (Gemini Flash output 40% off) | High -- cascade routes too aggressively to Gemini, actual spend 40% higher than predicted | G2.4: update rates; add staleness alert |
| Untrained DecisionModel routes badly | High (no multi-provider data yet) | Medium -- quality regression on cheap-provider tasks | Heuristic fallback is conservative (0.7 threshold); collect data before enabling cascade |
| Sweep 10x cost overrun | High (autonomous agent uses default $5, not $0.50) | High -- $50 vs $5 per sweep | G2.2: change handler default |
| Supervisor tick race on GateEnabled | Medium (only if L2 start and GateChange overlap) | Low -- wrong gate decision | G2.5: atomic.Bool |
| GetTeam deadlock | Low (requires specific concurrent call pattern) | Critical -- process hangs | G2.8: fix lock ordering |

### L3 Risks

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| $1,280/hr worst-case spend | Low (requires 128 concurrent sessions at $5 each) | Critical -- financial damage | Per-hour circuit breaker; FleetBudgetCapUSD |
| Queue loss on coordinator restart | Medium (any process crash) | High -- orphaned sessions, lost budget tracking, duplicate work | G3.5: queue persistence |
| Supervisor goroutine leak on stop | Medium (RunCycle goroutines untracked) | Medium -- orphaned cycles continue running | G3.3: WaitGroup tracking |
| AutoMerge introduces regression | Medium (VerifyCommands may not catch all regressions) | High -- production code broken | Disable AutoMergeAll for first 72h; add auto-revert |
| Fleet worker capacity exhaustion | Medium (stuck sessions consume slots) | Medium -- queue grows, no work executed | G3.6: executeWork timeout |
| Coordinator single point of failure | High (single process, no HA) | High -- fleet inoperable | Watchdog + queue persistence; HA is L3.2+ |
| Event bus publish failures silent | Medium (NATS connection issues) | Medium -- supervisor loses observability signals | F3.5: retry or counter metrics |
| Self-improvement loop corrupts codebase | Low (test gates exist) | Critical -- repo damaged | Container isolation for L3; git worktrees for isolation; auto-revert |

---

## 8. Timeline

Based on current velocity (1 developer + multi-agent fleet, ~20 tasks/week throughput)
and the dependency chain of fixes required at each level.

### Phase 1: L1 Ready (2 weeks)

**Week 1: Race fixes and error handling**
- Day 1-2: Fix R-01 (retryState mutex, 5 lines) + R-02 (RetryTracker mutex, 5 lines)
- Day 3: Fix autonomy level persistence (s4 fix #4), hook exit codes (s4 fix #6)
- Day 4: Surface supervisor cycle failures at Error level (s4 fix #1)
- Day 5: Propagate RunLoop errors in background goroutines (s4 fix #2)

**Week 2: Integration testing**
- Day 1-2: Write auto-recovery integration tests (transient error injection, max retry)
- Day 3: Write provider failover integration test
- Day 4: Run `go test -race ./internal/session/ -count=5` on Manjaro, fix any new races
- Day 5: 4-hour unattended L1 validation run

**Deliverable**: L1 gate open. Auto-recovery works reliably.

### Phase 2: L2 Ready (4-6 weeks after L1)

**Week 3-4: Budget hardening and rate fixes**
- Mandatory default budget (G2.1), sweep default $0.50 (G2.2)
- Wire CostPredictor (G2.3), update Gemini rates (G2.4)
- Fix remaining HIGH races: R-03 (atomic.Bool), R-04 (OpenAI mutex), R-05 (lock ordering), R-06 (loadedGroups)
- Proactive sweep cost events (F2.5)

**Week 5-6: DecisionModel training and supervisor validation**
- Run 50+ multi-provider sessions (25 Gemini, 15 Codex, 10 Claude) with VerifyCommands
- Train DecisionModel, validate cascade routing accuracy
- First 24-hour supervisor run on Manjaro (F2.1)
- Validate fleet CostPredictor forecasting accuracy

**Week 7-8 (buffer): Iteration on cascade routing**
- Tune confidence threshold via AdaptThreshold
- Increase minSamples to 100 if calibration bins are noisy
- Second 24-hour supervisor run to confirm stability

**Deliverable**: L2 gate open. Supervisor runs stably on Manjaro, cascade routing
validated, budget enforcement hardened.

### Phase 3: L3 Ready (6-8 weeks after L2)

**Week 9-10: Fleet hardening**
- Queue persistence (G3.5), worker timeout (G3.6)
- Fleet budget ceiling (G3.2), per-hour circuit breaker
- Sweep parallelization (G3.4)
- CompleteWork retry (G3.9)

**Week 11-12: Supervisor hardening**
- WaitGroup for tick goroutines (G3.3)
- Anomaly detector races (G3.7), cmd.Wait races (G3.8)
- Rehydration error surfacing (G3.11)
- Event bus reliability (F3.5)

**Week 13-14: Local autoscaler and 48-hour L2 validation**
- Local worker spawner for autoscaler (F3.1)
- FederatedBudget wiring (F3.3)
- Coordinator graceful restart (F3.4)
- 48-hour unattended L2 run (F3.8)

**Week 15-16: First 72-hour L3 run**
- Systemd watchdog unit for supervisor process
- Configure hardened 72-hour profile (section 6 above)
- Run with `auto_merge_all: false` (PRs created but not merged)
- Monitor spend, review PRs, verify no regressions

**Deliverable**: L3 gate open (with AutoMerge disabled). System operates 72 hours
unattended, creating PRs for human review.

### Phase 4: L3 with AutoMerge (2-4 weeks after Phase 3)

- Review all PRs from first 72-hour run
- Implement auto-revert on test regression
- Enable AutoMergeAll on a single repo (low-risk, high-test-coverage)
- 72-hour run with full autonomy including auto-merge
- Expand to additional repos after validation

**Deliverable**: Full L3 with AutoMergeAll. Truly unattended self-improvement.

### Summary Timeline

```
Week 0        Week 2         Week 8         Week 16        Week 20
  |              |              |               |              |
  v              v              v               v              v
  L0 -----> L1 ready ---> L2 ready -----> L3 ready ----> L3+AutoMerge
  (today)   (race fixes,   (budget,        (fleet,         (auto-revert,
             error          cascade,        watchdog,       full autonomy)
             handling)      supervisor)     72h run)
```

**Total elapsed time from today to L3 with AutoMerge: approximately 20 weeks (5 months).**

This timeline assumes focused effort on autonomy-path work. Competing priorities
(open-source release waves, MCP SDK migration, TUI polish) will extend the timeline.
The critical path is: race fixes (week 1-2) -> budget hardening (week 3-4) ->
DecisionModel training data (week 5-8) -> fleet hardening (week 9-14) ->
validation runs (week 15-20).

---

## Appendix: Cross-Reference to Research Documents

| Finding | Source Document | Section |
|---------|----------------|---------|
| AutoRecovery.retryState race (CRITICAL) | s1-race-condition-census.md | R-01 |
| RetryTracker.attempts race (CRITICAL) | s1-race-condition-census.md | R-02 |
| GateEnabled/RunTestGate race (HIGH) | s1-race-condition-census.md | R-03 |
| OpenAIClient.LastResponseID race (HIGH) | s1-race-condition-census.md | R-04 |
| GetTeam lock ordering (HIGH) | s1-race-condition-census.md | R-05 |
| loadedGroups map race (HIGH) | s1-race-condition-census.md | R-06 |
| Supervisor tick goroutine leak (MEDIUM) | s1-race-condition-census.md | R-07 |
| TieredKnowledge hitCount race (MEDIUM) | s1-race-condition-census.md | R-08 |
| Uncapped session launch (Gap A) | s3-cost-model-analysis.md | Section 2, Gap A |
| Active sessions bypass GlobalBudget (Gap B) | s3-cost-model-analysis.md | Section 2, Gap B |
| pool.State stale total (Gap C) | s3-cost-model-analysis.md | Section 2, Gap C |
| Sweep $5 vs $0.50 convention (Gap C) | s3-cost-model-analysis.md | Scenario C |
| $1,280/hr maximum theoretical spend | s3-cost-model-analysis.md | Scenario E |
| Gemini Flash rate 40% off | s3-cost-model-analysis.md | Section 3 |
| DecisionModel 50-sample requirement | s3-cost-model-analysis.md | Section 7 |
| Supervisor swallows cycle failures | s4-error-handling-audit.md | Fix #1 |
| RunLoop error buried | s4-error-handling-audit.md | Fix #2 |
| Fleet worker CompleteWork silent | s4-error-handling-audit.md | Fix #3 |
| Autonomy persist failure silent | s4-error-handling-audit.md | Fix #4 |
| Hook exit codes discarded | s4-error-handling-audit.md | Fix #6 |
| Rehydration failure swallowed | s4-error-handling-audit.md | Fix #15 |
| Queue not persisted | 06-fleet-sweep.md | Risk 2 |
| Autoscaler advisory only | 06-fleet-sweep.md | Risk 4 |
| Worker no session timeout | 06-fleet-sweep.md | Risk 5 |
| Sweep cost cap reactive | 06-fleet-sweep.md | Risk 6 |
| Sweep fan-out serial | 06-fleet-sweep.md | Risk 7 |
| Supervisor never run on Manjaro | 03-session-architecture.md | Finding 5 |
| DecisionModel needs multi-provider data | 03-session-architecture.md | Finding 4 |
| Cascade heuristic vs calibrated model | 03-session-architecture.md | Section 3 |
| Competitive safety patterns | 09-orchestration-landscape.md | Guardrails (OpenAI) |
| Opus 4.6 price reduction enables L3 cost viability | 11-llm-capabilities.md | Section 1 |
| Compaction for marathon sessions | 11-llm-capabilities.md | Section 7, item 4 |
