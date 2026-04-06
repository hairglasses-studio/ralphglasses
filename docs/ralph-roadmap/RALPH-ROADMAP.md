# RALPH-ROADMAP: Strategic Development Plan

*Generated 2026-04-04 by 24-agent cascading research sweep*

---

## Executive Summary

Ralphglasses is a Go-native multi-provider (Claude, Gemini, Codex) agent orchestration TUI and fleet manager with 166 MCP tools across 16 namespaces, currently at 44% roadmap completion (503/1,143 tasks) [Agent 1]. Six CRITICAL findings -- including two fatal concurrent map write races (R-01, R-02) and a $1,280/hr worst-case uncontrolled spend scenario -- block safe progression beyond L1 autonomy [S1, S3]. The platform occupies a unique competitive position as the only Go-native, MCP-first, multi-provider orchestrator with TUI, fleet distribution, and cost-aware cascade routing [Agent 9]. A 10-initiative strategic plan spanning Q2 2026 through Q1 2027 charts the path from current state through L3 full autonomy (72-hour unattended operation) in approximately 20 weeks of focused development [Agent 13, Agent 14].

---

## Part 1: Codebase Health Dashboard

### 1.1 Package Statistics

| Package | Role | Test Files | Test Functions | Coverage | Status | Blast Radius |
|---------|------|-----------|---------------|----------|--------|--------------|
| `internal/session` | Session lifecycle, loop engine, supervisor | 193 | 2,203 | 83.1% | FAIL (6 tests) | T1-Critical |
| `internal/mcpserver` | MCP tool handlers, dispatch, middleware | 76 | 1,084 | 74.1% | FAIL (1 test) | T1-Critical |
| `internal/fleet` | Worker dispatch, coordination, autoscaler | 48 | 625 | 77.8% | PASS | T1-Critical |
| `internal/fleet/pool` | Budget pool, worker pool | 4 | ~60 | 97.9% | PASS | T1-Critical |
| `internal/enhancer` | 13-stage prompt pipeline, scoring, lint | 30 | 314 | 93.8% | PASS | T2-Medium |
| `internal/enhancer/knowledge` | Tiered knowledge graph | ~3 | ~40 | 90.5% | FAIL (race) | T2-Medium |
| `internal/tui` (all) | BubbleTea TUI, 21 views, 38 view files | 86 | 1,148 | 70-90% | PASS | T2-Medium |
| `internal/wm` (all) | Compositor IPC (Sway, Hyprland, i3) | 11 | 156 | 52-90% | PASS | T2-Medium |
| `internal/e2e` | Mock harness, 24 scenarios | 14 | ~84 | 91.2% | PASS | T3-Lower |
| `internal/process` | Process management | 19 | — | 88.3% | FAIL (1 test) | T3-Lower |
| `internal/marathon` | Marathon supervisor, restart policy | 6 | — | 88.3% | FAIL (1 test) | T3-Lower |
| `cmd/ralphglasses-mcp` | MCP binary entrypoint | 10 | — | 59.1% | FAIL (count) | T3-Lower |
| `cmd/prompt-improver` | Prompt improvement CLI | 6 | — | N/A | FAIL (API key) | T3-Lower |

**Totals**: 745 test files, 9,267 `Test*` functions, 84.5% reported coverage, **7 actively failing packages** [Agent 5].

### 1.2 MCP Tool Health Matrix

| Namespace | Tools | Loading | Handler Quality (1-5) | Test Coverage (1-5) | Annotation (1-5) | Key Issues |
|-----------|-------|---------|----------------------|--------------------|--------------------|------------|
| **core** | 13 | Eager | 4.5 | 4 | 3 | -- |
| **session** | 16 | Deferred* | 4 | 3 | 3 | `session_handoff` misplaced in `loop` [Agent 2] |
| **loop** | 11 | Deferred* | 4 | 3 | 3 | -- |
| **prompt** | 9 | Deferred* | 4 | 3 | 3 | No prompt length validation [Agent 2] |
| **fleet** | 10 | Deferred* | 5 | 4 | 3 | -- |
| **repo** | 5 | Deferred* | 5 | 4 | 3 | -- |
| **roadmap** | 6 | Deferred* | 5 | 4 | 3 | `roadmap_prioritize` untested [Agent 2] |
| **team** | 6 | Deferred* | 4 | 2 | 3 | Only 5 tests for 6 tools [Agent 2] |
| **awesome** | 5 | Deferred* | 4 | 3 | 3 | -- |
| **advanced** | 24 | Deferred* | 4 | 3 | 3 | Too broad -- 7+ domains [Agent 2] |
| **eval** | 6 | Deferred* | 4 | 4 | 3 | Missing `IdempotentHint` [Agent 2] |
| **fleet_h** | 5 | Deferred* | 4 | 3 | 3 | -- |
| **observability** | 17 | Deferred* | 4 | 3 | 3 | Worktree tools misplaced [Agent 2] |
| **rdcycle** | 18 | Deferred* | 4 | 3 | 3 | State machine tools deserve split [Agent 2] |
| **plugin** | 4 | Deferred* | 4 | 3 | 3 | -- |
| **sweep** | 8 | Deferred* | 4 | 3 | 4 | `sweep_report` untested (302 lines) [Agent 2] |

*Deferred loading is documented but **not activated in production** -- all 166 tools load eagerly [Agent 2].

**Annotation completeness**: 0 tools have all 4 MCP spec hints; 83% have only 1 hint; 2 tools have zero behavioral hints [Agent 2].

### 1.3 MCP Middleware Chain

The middleware stack processes every tool call in this order (outermost to innermost) [Agent 2]:

```
ConcurrencyMiddleware(32 slots)
  -> TraceMiddleware(trace ID generation)
    -> TimeoutMiddleware(30s default; loop_step=10m, coverage_report=5m, self_test/self_improve=exempt)
      -> InstrumentationMiddleware(latency, success/failure recording)
        -> EventBusMiddleware(tool.called events)
          -> ValidationMiddleware(repo/path param validation)
            -> handler
```

**Identified gaps in middleware** [Agent 2]:
1. **No per-tool rate limiting** -- global 32-slot semaphore only. One slow tool can consume all slots.
2. **No panic recovery at middleware level** -- SDK-level `WithRecovery()` catches panics but does not emit structured error events to the event bus.
3. **Free-text fields unchecked** -- `ValidationMiddleware` only validates `repo` and `path`. Prompt (up to 200KB), YAML workflow definitions, and content fields are not length-checked in middleware.
4. **Tracing not end-to-end** -- `TraceMiddleware` generates trace IDs but there is no correlation with outgoing LLM API calls. OTel only used if `OTEL_EXPORTER_OTLP_ENDPOINT` is set.

### 1.4 Test Coverage by Blast Radius

| Tier | Description | Test Functions | Coverage Floor | Status |
|------|-------------|---------------|---------------|--------|
| T1 (Critical) | Session, MCP, Fleet | ~3,972 | 74.1% | 2 packages FAILING |
| T2 (Medium) | Enhancer, TUI, WM | ~1,658 | 52.4% | 1 package FAILING (knowledge race) |
| T3 (Lower) | E2E, process, marathon, cmd | ~500+ | 32.3% | 4 packages FAILING |

### 1.5 Failing Test Packages Detail

| Package | Failure | Root Cause | Fix Effort | Source |
|---------|---------|-----------|-----------|--------|
| `internal/knowledge` | Fatal concurrent map writes in `TieredKnowledge.Query` | No mutex on `hitCount` map under `RLock` | S -- add `sync.RWMutex` | Agent 5, S1 R-08 |
| `internal/marathon` | `TestRestartPolicy_BackoffCap` expects 30s, gets negative | Exponential overflow (`base * factor^count` overflows int64 at count=20, factor=10) | S -- cap before exponentiation | Agent 5 |
| `internal/mcpserver` | `TestHandleCircuitReset_Enhancer` expects `status=reset`, gets nil | Enhancer circuit breaker not initialized in test harness | S -- wire `srv.Enhancer.CircuitBreaker` in setup | Agent 5 |
| `internal/process` | `TestCollectChildPIDs_DeadPID` expects nil, gets `[]` | `nil` vs empty slice semantic mismatch | Trivial | Agent 5 |
| `internal/session` | 6 tests (WeeklyReport, TruncateStr, RunSessionOutput, RunCycle, ConfigOptimizer x2) | Mix of boundary conditions and assertion mismatches | S-M per test | Agent 5 |
| `cmd/ralphglasses-mcp` | `TestToolGroupNames` expects 15 entries, actual 16 | `sweep` namespace added without updating test expectation | Trivial | Agent 5 |
| `cmd/prompt-improver` | `ANTHROPIC_API_KEY` not set | Tests call live API without skip guard | S -- add key check | Agent 5 |

### 1.6 E2E Harness Assessment

24 scenarios across 6 categories: core (6), multi-provider (4), stress/edge (5), cost tracking (2), self-learning (5), cross-subsystem (3) [Agent 5].

**Strengths**: Full subsystem wiring coverage (`CascadeRouter`, `EpisodicMemory`, `ReflexionStore`, `CurriculumSorter`, `BanditHooks` injected per scenario). 5 explicit failure path scenarios. Tag-based selection. Baseline + gate regression detection system [Agent 5].

**Weaknesses**:
- **No multi-worker fan-out test** -- all scenarios use `MaxConcurrentWorkers: 1`. Goroutine race conditions in worker collection loop untested in E2E [Agent 5].
- **No real git operations** -- workers write files but do not commit. Worktree commit flow untested [Agent 5].
- **Live fire scenarios are stubs** -- `live_test.go` has `t.Skip("live-fire not yet wired to real providers")` even with API key present [Agent 5].
- **No fleet-level E2E** -- all scenarios exercise single-repo loop. Fleet coordination has no E2E coverage [Agent 5].
- **Provider failover partially mocked** -- `ProviderFailover` scenario does not inject `LaunchWithFailover` path or `FailoverChain` config [Agent 5].

### 1.7 Test Type Distribution

| Type | Count | Packages |
|------|-------|---------|
| Unit tests | ~7,800 | All |
| Coverage-padding tests | ~807 in 80 files | 25 packages (81% are real behavioral tests) |
| Integration tests | ~44 | session, mcpserver, wm/sway |
| E2E (mock harness) | 24 scenarios, ~60 functions | `internal/e2e` |
| Fuzz tests | 15 `Fuzz*` functions | mcpserver, model, session, tui/components |
| Benchmarks | 20 functions, 6 files | session, fleet, mcpserver |
| Race-targeted | 52 functions | session |
| Live fire | 2 (always skipped) | `internal/e2e/live_test.go` |

[Agent 5]

---

## Part 2: Critical Findings

### 2.1 CRITICAL Findings (6)

| ID | Finding | Source | Fix Effort | Description |
|----|---------|--------|-----------|-------------|
| F-001 | `AutoRecovery.retryState` race | S1 R-01 | S (5 lines) | Unprotected `map[string]*retryInfo` accessed from concurrent `HandleSessionError` and `ClearRetryState` callbacks. Fatal `concurrent map write` panic. |
| F-002 | `RetryTracker.attempts` race | S1 R-02 | S (5 lines) | Fleet `RetryTracker` map with no mutex; HTTP handlers `RecordFailure`/`RecordSuccess` race concurrently. |
| F-003 | Phase 3.5.5 numbering collision | Agent 1 | Trivial | Two sections share identical sub-IDs (Codex parity vs theme export) -- any tool addressing tasks by ID has undefined behavior. |
| F-004 | `tools_loop_test.go` without implementation | Agent 1 | M | Phase 9 tier-1 tools have test expectations but no Go implementations -- test file exists, source does not. |
| F-005 | Phase 9 marked complete but files missing | Agent 1 | M | `merge.go`, `cycle_plan.go`, `scheduler.go`, `baseline.go` do not exist despite Phase 9 showing 100%. |
| F-006 | 5 path traversal vulnerabilities | Agent 8 | S-M | `scratchpadName`, `name`, `worktree_paths`, `hooks.yaml command`, `verify_commands` bypass `ValidatePath` in MCP handlers. |

### 2.2 HIGH Findings (18)

| ID | Finding | Source | Package |
|----|---------|--------|---------|
| F-007 | `GateEnabled` unprotected global var | S1 R-03 | `session/autooptimize.go` |
| F-008 | `OpenAIClient.LastResponseID` unprotected | S1 R-04 | `enhancer/openai_client.go` |
| F-009 | `GetTeam` inconsistent lock ordering | S1 R-05 | `session/manager_team.go` |
| F-010 | `loadedGroups` map unprotected in MCP dispatch | S1 R-06 | `mcpserver/tools.go` |
| F-011 | Unbounded in-memory work queue | Agent 6 | `fleet/queue.go` |
| F-012 | Queue lost on coordinator restart | Agent 6 | `fleet/` |
| F-013 | Shared circuit breaker across all 3 LLM providers | Agent 4 | `enhancer/circuit.go` |
| F-014 | Autoscaler scale-up is advisory only (no actuator) | Agent 6 | `fleet/autoscaler.go` |
| F-015 | `RetryTracker` no mutex (independent finding) | Agent 6 | `fleet/retry.go` |
| F-016 | Self-improvement `RunLoop` error silently discarded | S4 | `mcpserver/handler_selfimprove.go` |
| F-017 | Supervisor swallows `RunCycle` failures | S4 | `session/supervisor.go` |
| F-018 | Chained cycle and sprint-planner failures swallowed | S4 | `session/supervisor.go` |
| F-019 | Hook exit codes entirely discarded | S4 | `hooks/hooks.go` |
| F-020 | `MaxBudgetUSD == 0` allows uncapped sessions | S3 | `session/manager_lifecycle.go` |
| F-021 | Active sessions not stopped when `GlobalBudget` exhausted | S3 | `fleet/` |
| F-022 | Sweep cost cap is reactive (5-min polling) | S3 | `mcpserver/handler_sweep.go` |
| F-023 | `TieredKnowledge.Query` fatal concurrent map writes | Agent 5 | `enhancer/knowledge/` |
| F-024 | `AutoRecovery.retryState` confirmed independently | Agent 3 | `session/autorecovery.go` |

### 2.3 MEDIUM Findings (Top 25)

| ID | Finding | Source | Package |
|----|---------|--------|---------|
| F-025 | Deferred loading not activated in production binary | Agent 2 | `cmd/mcp.go` |
| F-026 | `advanced` namespace too broad (24 tools, 7+ domains) | Agent 2 | `mcpserver/tools_builders_misc.go` |
| F-027 | 8 handler files without test counterparts | Agent 2, Agent 5 | `mcpserver/` |
| F-028 | Supervisor tick launches goroutines with no WaitGroup | S1 R-07 | `session/supervisor.go` |
| F-029 | `ValidationMiddleware` only covers `repo`/`path` params | Agent 2 | `mcpserver/middleware.go` |
| F-030 | `hitCount[key]++` in TieredKnowledge under `RLock` | S1 R-08 | `knowledge/tiered_knowledge.go` |
| F-031 | Shared CircuitBreaker across 3 enhancer providers | S1 R-13 | `enhancer/hybrid.go` |
| F-032 | Gemini Flash output rate understated by ~40% | S3 | `config/costs.go` |
| F-033 | 7 packages actively failing tests | Agent 5 | Multiple |
| F-034 | 80 coverage-padding files (807 functions, ~6% genuine padding) | Agent 5 | All |
| F-035 | LogView unbounded line growth | Agent 7 | `tui/views/` |
| F-036 | LoopRun.Iterations O(n) per 2s tick | Agent 7 | `tui/` |
| F-037 | 5 completed tasks reference nonexistent files | S2 | ROADMAP.md |
| F-038 | Worker `executeWork` no timeout on status polling | Agent 6 | `fleet/worker.go` |
| F-039 | ROADMAP.md header task count stale (1,115 vs 1,143) | Agent 1 | ROADMAP.md |
| F-040 | Default target provider resolves to OpenAI, not Claude | Agent 4 | `enhancer/config.go` |
| F-041 | Opus rate in code $15/$75 vs actual $5/$25 (67% overestimate) | S3 | `config/costs.go` |
| F-042 | Fleet `CostPredictor` not auto-wired from `handleWorkComplete` | S3 | `fleet/costpredict.go` |
| F-043 | 4 independent `CircuitBreaker` implementations | S2 | Multiple packages |
| F-044 | `GetLoop`/`ListLoops` use write lock instead of read lock | Agent 3 | `session/loop.go` |
| F-045 | Fleet worker `CompleteWork`/`Heartbeat` errors silently discarded | S4 | `fleet/worker.go` |
| F-046 | `hw-detect.sh` cannot detect dual GPU or enumerate multiple cards | Agent 8 | `distro/scripts/` |
| F-047 | `cancel` field in both anomaly detectors written/read without lock | S1 R-11,R-12 | `safety/anomaly*.go` |
| F-048 | JSON format enforcement 25.7% retry rate (target <5%) | Agent 3 | `session/loop_steps.go` |
| F-049 | Autonomy level persist failure Warn-logged; resets to L0 on restart | S4 | `session/autonomy.go` |

### 2.4 Race Condition Heat Map

| Package | CRITICAL | HIGH | MEDIUM | LOW | SAFE | Total |
|---------|----------|------|--------|-----|------|-------|
| `internal/session` | 1 (R-01) | 1 (R-05) | 4 (R-07,R-09,R-10,R-14) | 1 (R-17) | -- | 7 |
| `internal/fleet` | 1 (R-02) | -- | -- | 1 (R-16) | -- | 2 |
| `internal/mcpserver` | -- | 1 (R-06) | -- | 1 (R-18) | -- | 2 |
| `internal/enhancer` | -- | 1 (R-04) | 2 (R-13,R-31) | -- | -- | 3 |
| `internal/knowledge` | -- | -- | 1 (R-08) | -- | 1 (R-20) | 2 |
| `internal/safety` | -- | -- | 2 (R-11,R-12) | -- | -- | 2 |
| `internal/session/costnorm` | -- | -- | -- | -- | 1 (R-19) | 1 |

**Total: 2 CRITICAL + 4 HIGH + 8 MEDIUM + 4 LOW + 2 SAFE = 20 findings** [S1].

### 2.5 Budget Enforcement Gaps

| Gap | Description | Risk | Source |
|-----|-------------|------|--------|
| **Gap A** | `MaxBudgetUSD == 0` launches uncapped sessions when `DefaultBudgetUSD` is also zero | Critical -- unbounded spend | S3 |
| **Gap B** | Active sessions continue after `GlobalBudget` exhausted; overspend silently accepted | High -- budget exceeded | S3 |
| **Gap C** | `pool.State.CanSpend()` uses stale total between refresh intervals | Medium -- temporary over-budget | S3 |
| **Gap D** | Sweep cost cap polled every 5 minutes; overspend within interval | Medium -- reactive, not proactive | S3, Agent 6 |
| **$1,280/hr** | 32 workers x 4 sessions x $10 default x 2 rotations/hr with no fleet ceiling | Critical -- theoretical maximum | S3 |
| **Sweep 10x** | Handler default $5.00/session vs user convention $0.50/session | High -- cost surprise | S3 |

### 2.6 Error Handling Gaps

| Category | Count | Key Examples | Source |
|----------|-------|-------------|--------|
| Swallowed critical errors | 4 | RunLoop discarded, hook exit codes ignored, supervisor cycle failures silent, sweep stop failures ignored | S4 |
| Swallowed medium errors | 10 | Fleet worker heartbeat/completion, store save fallback, file persistence, event bus publish | S4 |
| Silent failure patterns | 6 | Autonomy persist failure, marathon checkpoint, NATS publish, session rehydration | S4 |
| Error wrapping ratio | 65% global | Worst: `mcpserver` at 25%; best: `store` at 93% | S4 |
| Production panics | 2 | Both justified (`BlendedCostPer1MTok` range, `MustNew` constructor) | S4 |
| Handler contract violations | **0** | All 44 MCP handlers correctly return `(*CallToolResult, nil)` | S4 |

### 2.7 LOW Findings (17)

| ID | Finding | Source | Package |
|----|---------|--------|---------|
| F-050 | `handleLoadToolGroup` description missing 3 namespaces | Agent 2 | `mcpserver/tools_dispatch.go` |
| F-051 | 0/166 tools have all 4 MCP spec annotations | Agent 2 | `mcpserver/annotations.go` |
| F-052 | 11 handlers defined in non-`handler_*.go` files | Agent 2 | `mcpserver/` |
| F-053 | Worktree/loop tools misplaced in `observability` namespace | Agent 2 | `mcpserver/` |
| F-054 | `FleetDashboardModel` dead code (207+236 lines) | S2 | `tui/views/fleet_dashboard.go` |
| F-055 | `ModalStack` defined but never instantiated | S2 | `tui/components/modal.go` |
| F-056 | `ViewSearch` iota defined but no handler exists | S2, Agent 7 | `tui/app_init.go` |
| F-057 | No mouse scroll support in viewports | Agent 7 | `tui/handlers_mouse.go` |
| F-058 | `RefreshAllRepos` synchronous on 2s UI tick (69+ repos) | Agent 7 | `tui/` |
| F-059 | Lint rule 14 conflicts with default pipeline (markdown for OpenAI flagged as no XML) | Agent 4 | `enhancer/context.go` |
| F-060 | Gemini `CreateCachedContent()` never called automatically | Agent 4 | `enhancer/gemini_client.go` |
| F-061 | `PromptCache` O(n) eviction vs `EnhancerCache` O(1) LRU | Agent 4 | `enhancer/cache.go` |
| F-062 | Phase 6 and Phase 15 both named "Advanced Fleet Intelligence" | Agent 1 | ROADMAP.md |
| F-063 | Tailscale auth key stored as plain file before enrollment | Agent 8 | `distro/scripts/ts-enroll.sh` |
| F-064 | Sweep auto-sizes budget to 1.5x estimate even if caller passed lower | S3 | `mcpserver/handler_sweep.go` |
| F-065 | 158 phantom file references in ROADMAP.md | S2 | ROADMAP.md |
| F-066 | `mcpserver` has worst `%w` error wrapping ratio at 25% | S4 | `mcpserver/` |

### 2.8 INFO Findings (Selected)

| ID | Finding | Source |
|----|---------|--------|
| F-067 | Overall coverage 84.5%; 745 test files, 9,267 functions | Agent 5 |
| F-068 | Zero genuine stale TODOs in production code | S2 |
| F-070 | Unique competitive position: only Go-native multi-provider MCP orchestrator | Agent 9 |
| F-071 | MCP spec at v2025-11-25 with Tasks, Extensions, OAuth | Agent 10 |
| F-072 | Gemini 3.1 Pro: SWE-bench 80.6% at $2.00 vs Opus 80.8% at $5.00 | Agent 11 |
| F-073 | Opus 4.6 price dropped 67%; only flat-pricing frontier model across 1M context | Agent 11 |
| F-074 | NixOS recommended for fleet deployment; Manjaro retained short-term | Agent 12 |
| F-078 | Zero handler contract violations across all 44 MCP handlers | S4 |
| F-080 | Three-mutex Manager lock hierarchy is sound; no cycles observed | Agent 3 |
| F-081 | Bootstrap correctly clamps autonomy to L1 from `.ralphrc` | Agent 3 |
| F-082 | L3 critical path: ~100 tasks across 6 unstarted phases, all L/XL | Agent 1 |

### 2.9 Architecture Deep Dive: Session Lifecycle

The session manager uses a **three-mutex hierarchy** (`sessionsMu`, `workersMu`, `configMu`) with a `statusCache sync.Map` hot path for TUI polling. No direct deadlock risk observed -- the manager never holds two of its own mutexes simultaneously [Agent 3].

**State machine**: 6 states (`launching`, `running`, `completed`, `stopped`, `errored`, `interrupted`). Transition triggers are well-defined. `IsTerminal()` is the canonical completion test [Agent 3].

**Cascade routing**: Default config uses Gemini as cheap provider and Codex as expensive provider with 0.7 confidence threshold. The confidence heuristic combines 5 factors: `verifyPassed (0.30)`, `hedgeCount (0.25)`, `turnRatio (0.20)`, `errorFree (0.15)`, `questionCount (0.10)`. The calibrated `DecisionModel` (logistic regression on 10 features) requires 50+ samples to activate -- currently untrained due to 100% Claude observation distribution [Agent 3].

**Loop engine architecture**: `RunLoop` calls `StepLoop` per iteration with convergence gates (retry limit, max iterations, deadline, consecutive no-ops). Workers fan out in goroutines with `recover()` guards and a 15-minute collection timeout. Key subsystems: planner JSON parsing (25.7% retry rate), near-duplicate detection, curriculum sorting, episodic memory injection, cascade routing per worker [Agent 3].

**Supervisor control loop**: 60-second tick (observed at ~27s in prior run). Each tick: `shouldTerminate()`, `stallHandler.CheckAndHandle()`, `monitor.Evaluate()`, `chainer.CheckAndChain()`, `planner.PlanNextSprint()`, `persistState()`. Feedback loop runs every 10 ticks. Cooldown between cycle launches: 5 minutes [Agent 3].

**Store dual-write**: JSON file persistence and SQLite store coexist. Cross-process session discovery (TUI to MCP server) depends on JSON files. Migration to SQLite-only is planned but blocked by the shared-filesystem discovery pattern [Agent 3].

### 2.10 Architecture Deep Dive: Enhancer Pipeline

13 deterministic pipeline stages executing sequentially on mutable text [Agent 4]:

| Stage | Name | Provider-Aware? | Key Behavior |
|-------|------|----------------|-------------|
| 0 | Config rules | No | Substring-match augmentation from `.prompt-improver.yaml` |
| 1 | Specificity | No | 22-entry vague phrase replacement table |
| 2 | Positive reframe | No | 17-entry negative-to-positive rewriting (safety bypass for credentials) |
| 3 | Tone downgrade | **Claude-only** | ALL-CAPS normalization (43-entry acronym whitelist) |
| 4 | Overtrigger rewrite | **Claude-only** | `CRITICAL: You MUST` prefix stripping |
| 5 | Example wrapping | No | Detects bare examples, wraps in `<examples>` XML |
| 6 | Structure | **Provider-adapted** | XML tags (Claude) or markdown headers (Gemini/OpenAI) |
| 7 | Context reorder | No | Moves large context blocks before query in 20K+ char prompts |
| 8 | Format enforcement | No | Detects JSON/YAML/CSV/code requests, injects `<output_format>` |
| 9 | Quote grounding | No | Injects quote-grounding instruction for analysis prompts >= 5K tokens |
| 10 | Self-check | No | Appends `<verification>` checklist (code/analysis/troubleshooting) |
| 11 | Overengineering guard | No | Appends anti-overengineering instruction for code tasks |
| 12 | Preamble suppression | No | Appends direct-response instruction for code/workflow tasks |

**Scoring**: 10 dimensions (clarity 0.15, specificity 0.12, context 0.10, structure 0.15, examples 0.10, placement 0.08, role 0.08, focus 0.07, format 0.08, tone 0.07). Score capped to [5, 95]. FINDING-240 baselines lowered to prevent inflation [Agent 4].

**Key gaps**:
- Default target provider resolves to OpenAI (markdown), not Claude (XML) [Agent 4]
- Shared circuit breaker across all 3 providers -- one flaky provider trips for all [Agent 4, S1]
- Gemini `CreateCachedContent()` never called automatically [Agent 4]
- `FormatContextBlockMarkdown()` defined but unreachable from pipeline [Agent 4]
- `PromptCache` eviction is O(n) vs `EnhancerCache` O(1) LRU [Agent 4]

### 2.11 Architecture Deep Dive: MCP Ecosystem Integration

**Spec compliance gap**: ralphglasses targets spec ~2025-03-26. The 2025-11-25 spec adds features worth adopting [Agent 10]:

1. **Tasks primitive** -- async call-now-fetch-later model. Maps naturally to sweep/fleet operations where tool calls initiate long-running work and callers poll for results.
2. **Extensions framework** -- custom protocol extensions for fleet-specific capabilities (cost tracking, autonomy levels).
3. **Structured output** -- tools declare `outputSchema` (JSON Schema) and return `structuredContent`. Would improve the 25.7% JSON retry rate by allowing MCP-layer output validation.

**Go SDK migration path**: Official `modelcontextprotocol/go-sdk` v1.4+ provides semver stability that mcp-go v0.46.0 (pre-1.0) does not. mcpkit provides ToolModule pattern, DynamicRegistry, middleware chain, and gateway on top of mcp-go. Migration updates the transport layer in mcpkit, not individual tool handlers [Agent 10].

**Gateway landscape**: Bifrost (Go, open-source), Microsoft MCP Gateway (K8s-native), Envoy AI Gateway, Traefik Hub all aggregate upstream servers. mcpkit's `DynamicRegistry` serves as a built-in gateway. The 16-namespace deferred loading model effectively mirrors the gateway aggregation pattern [Agent 10].

### 2.12 Architecture Deep Dive: Fleet Operations

**Job processing pipeline**: `SubmitWork` -> `WorkQueue.Push` (in-memory map) -> `WorkerAgent.pollLoop` (5s interval) -> `assignWork` (locality + constraint scoring) -> `executeWork` (2s status polling) -> `CompleteWork` -> `RetryTracker`/`GlobalBudget` update [Agent 6].

**Three budget layers**: (1) GlobalBudget fleet ceiling ($500 default), (2) BudgetManager per-worker tracking ($10 default), (3) BudgetPool per-sweep allocation [Agent 6].

**Autoscaler**: Full algorithm implemented. Scale-up triggers when `queueDepth > 2x active` (budget floor: 10% remaining). Scale-down when `idle/active > 0.50` and queue empty. **Scale-up is advisory only** -- publishes event but spawns nothing. Scale-down is actuated via worker draining [Agent 6].

**A2A protocol**: Core task lifecycle working (6 states). Agent card serving, task offers, negotiation, skill dispatch all functional. **Stubs**: SSE streaming, authentication enforcement, multi-turn input-required resume. **Gap**: `A2AAdapter` and `Coordinator` queue are parallel systems, not integrated [Agent 6].

**Topology**: Consistent hash ring (Ketama MD5, 128 virtual nodes), multi-strategy routing optimizer (simulation-based). **Not connected** to actual `assignWork` path -- optimizer is advisory. Single coordinator SPOF with no HA [Agent 6].

### 2.13 Architecture Deep Dive: TUI Layer

**Navigation**: `CurrentView` + `ViewStack` push/pop model with breadcrumb. 21 named ViewModes, 38 view files. Tab switching (1-4) clears entire stack [Agent 7].

**Tick architecture**: 2-second `tea.Tick` performs synchronous I/O for every repo (69+ file reads), rebuilds all tables regardless of current view, aggregates cost history. At 100+ repos, tick duration grows linearly and blocks the render loop [Agent 7].

**Memory risks**:
- `LogView.Lines` unbounded -- hours of verbose output exhausts memory [Agent 7]
- `LoopRun.Iterations` grows indefinitely; `SnapshotLoopControl` iterates all per tick at O(n) [Agent 7]
- `CostHistory` aggregation creates large intermediate slice every 2 seconds [Agent 7]

**Dead code**: `FleetDashboardModel` (207 lines + 236-line test) never instantiated. `ModalStack` defined but unused. `ViewSearch` iota has no handler [S2, Agent 7].

**Reactive path**: fsnotify watches `.ralph/` dirs with one-shot re-arm pattern. Failure handling: 5 consecutive failures disables watcher, falls back to 2s polling [Agent 7].

### 2.14 Distro Layer Status

**Compositor abstraction**: 9 commands across Sway/Hyprland/i3 with detection priority matching `internal/wm/detect.go`. Self-tests exist for all three compositors [Agent 8].

**GPU detection**: `hw-detect.sh` hardcodes PCI IDs for ProArt X870E hardware. Handles single RTX 4090 + AMD iGPU. **Does not** handle dual RTX 4090. GTX 1060 blacklisted via `NVreg_ExcludedGpus` [Agent 8].

**Boot chain**: UEFI -> GRUB -> kernel -> systemd -> hw-detect.service -> getty@tty1 (autologin) -> Sway/Hyprland/i3 -> kiosk config -> 7x alacritty -> ralphglasses [Agent 8].

**Critical gaps**: No disk encryption (LUKS). No secrets management (API keys in env vars). Autorandr tasks 3.4.1-3.4.4 incomplete. `hw-detect.sh` not idempotent. `nvidia-drm.modeset=1` checked but only warned (not auto-added) [Agent 8].

### 2.15 Dead Code Summary

| Category | Item | Lines | Action | Source |
|----------|------|-------|--------|--------|
| Unused struct | `FleetDashboardModel` + test | 443 | Delete | S2 |
| Unused struct | `ModalStack` | ~75 | Flag for removal | S2 |
| Unreachable iota | `ViewSearch` | 1 | Remove or implement | S2 |
| Convention violation | 11 handlers in non-`handler_*.go` files | 964 | Move to handler files | S2, Agent 2 |
| Duplicate impls | 4 `CircuitBreaker` copies | varies | Consolidate to `safety/` | S2 |
| Duplicate impls | 2 `BudgetPool` copies (fleet + session) | varies | Evaluate merge | S2 |
| Phantom ROADMAP refs | 5 in completed tasks, 153 in future tasks | N/A | Fix 5 completed, accept 153 | S2 |
| Stale comments | 0 genuine (7 intentional regex/lint patterns) | N/A | None needed | S2 |
| Orphaned test files | 187 files (~39,436 lines) | ~39,436 | Keep -- predominantly real tests | S2 |

---

## Part 3: Strategic Initiatives

### 3.1 Initiative Summary

| # | Initiative | Tasks | Weeks | Prerequisites | Risk | Gate |
|---|-----------|-------|-------|--------------|------|------|
| 1 | Concurrency & Safety Hardening | ~35 | 2 | None | HIGH | Race-free, budget-enforced |
| 2 | Test Green & Coverage Integrity | ~40 | 2 | I1 | MEDIUM | Green CI, Phase 1 complete |
| 3 | Cost Control & Multi-Provider Cascade | ~50 | 3 | I1 | MEDIUM | $0.05/task avg, 3-provider active |
| 4 | MCP Tool Layer Modernization | ~45 | 3 | I2 | HIGH | Official SDK, deferred loading |
| 5 | TUI Performance & Fleet Dashboard | ~55 | 3 | I2 | MEDIUM | Sub-100ms at 100+ sessions |
| 6 | Bootable Thin Client | ~50 | 3 | I5 | HIGH | Bootable ISO with Sway kiosk |
| 7 | Fleet Scaling & A2A Protocol | ~75 | 4 | I1, I3 | HIGH | Multi-node fleet, A2A live |
| 8 | Sandboxing & Security | ~55 | 3 | I1, I7 | HIGH | Sandboxed multi-tenant |
| 9 | Autonomous R&D (L2) | ~65 | 4 | I1, I2, I3 | HIGH | Multi-hour unattended R&D |
| 10 | Prompt Enhancement & Observability | ~55 | 3 | I3 | LOW | OTel e2e, prompt caching |
| | **Total** | **~525** | **~30** | | | |

Remaining ~115 tasks (Phases 14-25: memory, swarm, edge, world models, marketplace, federated learning) are deferred to post-L3 [Agent 13].

### 3.2 Initiative Details

**I1 -- Concurrency & Safety Hardening** (Foundation for everything):
Fix all 6 CRITICAL+HIGH race conditions [S1 R-01 through R-06]. Close 3 budget enforcement gaps [S3]. Surface supervisor cycle failures and propagate RunLoop errors [S4]. Fix 7 failing test packages [Agent 5]. Delete confirmed dead code (443 lines) [S2]. This is the prerequisite for L1 activation and all downstream initiatives.

**I2 -- Test Green & Coverage Integrity**:
Achieve green `go test -race ./...` across all packages. Complete Phase 1 (2 tasks remaining: ParamParser extraction, handler generator) [Agent 1]. Add tests for 5 highest-risk untested handlers (`handler_sweep_report.go` at 302 lines is priority #1) [Agent 2, Agent 5]. Activate deferred loading. Complete all 166 tool annotations to 4/4 MCP spec hints [Agent 2].

**I3 -- Cost Control & Multi-Provider Cascade**:
Update compiled-in rates to April 2026 pricing (Opus $5/$25 vs $15/$75 coded, Gemini Flash $3.50 vs $2.50 coded) [S3, Agent 11]. Implement Tier 0 classifier (GPT-4.1-nano at $0.05/1M). Activate Gemini Flash as Tier 1 worker. Target: $0.17/task average down to $0.03-0.05 [Agent 11]. Wire fleet CostPredictor auto-feed [S3].

**I4 -- MCP Tool Layer Modernization**:
Migrate from mcp-go v0.46.0 to official `modelcontextprotocol/go-sdk` v1.4+ (XL task touching all registration paths) [Agent 10]. Split `advanced` namespace (24 tools spanning 7+ domains) into `rc` (4), `autonomy` (4), `workflow` (3), and residual [Agent 2]. Move 7 misplaced tools. Add per-namespace concurrency caps [Agent 2].

**I5 -- TUI Performance & Fleet Dashboard**:
Replace 2-second polling tick with event bus subscription [Agent 7]. Add max-lines cap to `LogView.Lines` (ring buffer) [Agent 7]. Skip table rebuilds when view is not active. Virtual scrolling for 100+ sessions. Charm Bubble Tea v2 migration (XL) [Agent 13]. Implement or remove `ViewSearch` [Agent 7, S2].

**I6 -- Bootable Thin Client**:
Complete ISO build pipeline (task 4.1.3 blocks 4.2/4.5/4.10) [Agent 1]. Add LUKS disk encryption [Agent 8]. Integrate 1Password CLI for secrets [Agent 8]. Drop i3 support, archive `distro/i3/` [Agent 12]. Extend `hw-detect.sh` for dual RTX 4090. Implement greetd boot-to-TUI pipeline [Agent 12].

**I7 -- Fleet Scaling & A2A Protocol**:
Complete Phase 10.5: NATS transport, multi-node coordination, SQLite WAL migration [Agent 1]. Bound work queue (capacity limit + persistent backing) [Agent 6]. Parallelize sweep fan-out [Agent 6]. Adopt official A2A Go SDK, implement SSE streaming, A2A-level auth [Agent 9]. Begin Tailscale fleet networking [Agent 13].

**I8 -- Sandboxing & Security**:
Fix 5 BLOCKER path traversal vulnerabilities [Agent 8]. Complete Phase 5 network isolation. Implement Firecracker/gVisor sandboxing for untrusted sessions [Agent 1]. Budget federation across sandboxed sessions. Begin Phase 17.1 safety boundaries [Agent 13].

**I9 -- Autonomous R&D (L2) & Self-Improvement**:
Fix planner JSON reliability to <5% retry rate (currently 25.7%) [Agent 8]. Surface supervisor failures as errors with HITL interrupts [S4]. Implement per-provider circuit breakers [Agent 4, S1]. Ship self-test CI gate and self-improvement acceptance pipeline [Agent 13]. Begin Phase 13.1 self-healing runtime [Agent 14].

**I10 -- Prompt Enhancement & Observability**:
Fix default target provider to match LLM provider [Agent 4]. Wire `FormatContextBlockMarkdown()` into pipeline [Agent 4]. Complete OTel integration (correlate MCP traces with LLM API spans) [Agent 2]. Activate prompt caching for all 3 providers [Agent 4, Agent 11]. Begin evaluation harness (SWE-bench, tau-bench) [Agent 13].

### 3.3 Dependency DAG

```
I1 Concurrency & Safety Hardening
 |
 +---> I2 Test Green & Coverage Integrity
 |      |
 |      +---> I4 MCP Tool Layer Modernization
 |      |
 |      +---> I5 TUI Performance & Fleet Dashboard
 |      |      |
 |      |      +---> I6 Bootable Thin Client
 |      |
 |      +---> I9 Autonomous R&D (L2) & Self-Improvement
 |
 +---> I3 Cost Control & Multi-Provider Cascade
 |      |
 |      +---> I9 (also depends on I1, I2)
 |      |
 |      +---> I10 Prompt Enhancement & Observability
 |      |
 |      +---> I7 Fleet Scaling & A2A Protocol
 |             |
 |             +---> I8 Sandboxing & Security
 |
 +---> I7 (also depends on I1)
 +---> I8 (also depends on I1)
```

Edge list: `I1 --> I2, I3, I7, I8, I9` | `I2 --> I4, I5, I9` | `I3 --> I7, I9, I10` | `I5 --> I6` | `I7 --> I8` [Agent 13].

### 3.4 Quarter-Level Timeline

| Quarter | Initiatives | Key Milestones |
|---------|------------|----------------|
| **Q2 2026** (Apr-Jun) | I1, I2, I3, I4 start, I5 start | All CRITICAL/HIGH races fixed. Green CI. Multi-provider cascade active. Official MCP SDK adopted. L1 autonomy stable. |
| **Q3 2026** (Jul-Sep) | I5 complete, I9, I6, I10 | Self-improvement pipeline shipping. Bootable ISO with Sway kiosk. OTel tracing end-to-end. L2 autonomy validated. |
| **Q4 2026** (Oct-Dec) | I7, I8 | Multi-node fleet over Tailscale. A2A protocol live. Sandboxing complete. L3 readiness assessment. |
| **Q1 2027** (Jan-Mar) | Phase 13, 14, 17 | Self-healing runtime. Persistent agent memory. Safety guardrails. 72-hour unattended L3 operation. |

[Agent 13]

---

## Part 4: Autonomy Progression Plan

### 4.1 Current State per Level

| Level | Name | Status | Key Finding |
|-------|------|--------|-------------|
| **L0** (Observe) | Default | Functional | Decision audit log works. Bootstrap clamps to L1 max from `.ralphrc`. No side effects. [Agent 14] |
| **L1** (Auto-Recover) | Partially functional | **Not safe** | R-01 CRITICAL race in `retryState`. Supervisor swallows cycle failures. Hook exit codes discarded. [Agent 14, S1, S4] |
| **L2** (Auto-Optimize) | Not ready | **Blocked** | 3 budget gaps. Gemini rates 40% off. DecisionModel untrained (0 multi-provider data). Supervisor never run on Manjaro. [Agent 14, S3] |
| **L3** (Full Autonomy) | Not ready | **Blocked** | All L1/L2 issues amplified. $1,280/hr worst-case. Queue not persisted. Autoscaler advisory-only. [Agent 14, S3, Agent 6] |

### 4.2 Gate Criteria

#### L0 to L1 (Auto-Recovery) -- Hard Gates

| ID | Fix | Source | Effort |
|----|-----|--------|--------|
| G1.1 | Add `sync.Mutex` to `AutoRecovery.retryState` | S1 R-01 | S |
| G1.2 | Surface supervisor cycle failures at Error level; demote autonomy after N consecutive failures | S4 | M |
| G1.3 | Propagate RunLoop error in background goroutines | S4 | S |
| G1.4 | Fix autonomy level persistence failure | S4 | S |
| G1.5 | Add hook exit code handling | S4 | S |

**Exit**: 4-hour unattended L1 session with at least one auto-recovery event, no crashes, no orphaned sessions [Agent 14].

#### L1 to L2 (Auto-Optimize) -- Hard Gates

| ID | Fix | Source | Effort |
|----|-----|--------|--------|
| G2.1 | Enforce mandatory $5 default budget | S3 Gap A | S |
| G2.2 | Change sweep handler default from $5.00 to $0.50 | S3 | S |
| G2.3 | Wire `CostPredictor.Record()` to `handleWorkComplete` | S3 | S |
| G2.4 | Update Gemini 2.5 Flash output rate ($2.50 to $3.50) | S3 | S |
| G2.5 | Make `GateEnabled` an `atomic.Bool` | S1 R-03 | S |
| G2.6 | Protect `loadedGroups` map with mutex | S1 R-06 | S |
| G2.7 | Add `sync.Mutex` to `RetryTracker.attempts` | S1 R-02 | S |
| G2.8 | Fix `GetTeam` lock ordering | S1 R-05 | M |
| G2.9 | Add mutex to `OpenAIClient.LastResponseID` | S1 R-04 | S |

**Functional requirements**: 50+ multi-provider observations for DecisionModel. 24+ hour supervisor run on Manjaro. Cascade routing 70%+ cheap-provider completion rate [Agent 14].

#### L2 to L3 (Full Autonomy) -- Hard Gates

| ID | Fix | Source | Effort |
|----|-----|--------|--------|
| G3.1 | All 6 CRITICAL+HIGH races fixed | S1 | M (aggregate) |
| G3.2 | Fleet budget ceiling enforced (`FleetBudgetCapUSD`) | S3 | M |
| G3.3 | Supervisor tick goroutines tracked with WaitGroup | S1 R-07 | M |
| G3.4 | Sweep launch parallelized (semaphore, size 10) | Agent 6 | M |
| G3.5 | Queue persistence (auto-save every 30s) | Agent 6 | M |
| G3.6 | Worker `executeWork` timeout | Agent 6 | S |
| G3.7-G3.11 | Anomaly detector, cmd.Wait, fleet retry, marathon checkpoint, rehydration fixes | S1, S4 | S-M each |

**Functional requirements**: Local autoscaler actuator. Proactive cost events. 48-hour unattended L2 run. `go test -race ./... -count=5` green [Agent 14].

### 4.3 Safety Interlocks Required at Each Level

| Level | Existing Interlocks | Missing Interlocks |
|-------|--------------------|--------------------|
| **L0** | Bootstrap clamp, decision audit, no side effects | None critical |
| **L1** | Max retry limit (3), exponential backoff, transient-only retry, remaining budget cap, category blocklist | Autonomy demotion circuit breaker. Retry state mutex (R-01). |
| **L2** | Test gate (`GateChange`), budget envelope, cooldown (5m), chain depth cap (10), concurrency cap, termination conditions | Fleet-level hard budget ceiling. Cascade confidence floor with minimum sample gate. Supervisor self-health check. Cost rate staleness alert. |
| **L3** | AutoMergeAll guard (partial), chain depth 10, rate limit (3/hr at confidence >= 0.8), requires API call (not `.ralphrc`) | Per-hour spend circuit breaker ($50/hr). Watchdog process (systemd). Network isolation. Git auto-revert. Rollback on autonomy escalation failure. |

[Agent 14]

### 4.4 Minimum Viable L3 Configuration

```yaml
autonomy_level:           3
max_duration:             72h
max_total_cost_usd:       500.00
per_hour_cap_usd:         50.00      # circuit breaker (new)
per_session_budget:       5.00       # hard default
sweep_budget_usd:         0.50       # per-session in sweeps
max_concurrent:           1          # supervisor cycles
max_workers:              4          # fleet workers (single machine)
max_sessions_per_worker:  2          # conservative for dual GPU
chain_depth_cap:          5          # reduced from 10 for first 72h run
cooldown:                 10m        # increased from 5m for safety
tick_interval:            60s
compaction:               true
auto_merge_all:           false      # first 72h: create PRs, don't merge
```

**Note**: `auto_merge_all: false` for the initial 72-hour run. Full AutoMergeAll is a separate gate requiring auto-revert capability [Agent 14].

### 4.5 Risk Matrix for L3

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| $1,280/hr worst-case spend | Low (requires 128 concurrent sessions) | Critical -- financial damage | Per-hour circuit breaker; `FleetBudgetCapUSD` [S3] |
| Queue loss on coordinator restart | Medium (any process crash) | High -- orphaned sessions, duplicate work | Queue persistence (G3.5) [Agent 6] |
| AutoMerge introduces regression | Medium (VerifyCommands may miss regressions) | High -- production code broken | Disable AutoMergeAll for first 72h; auto-revert [Agent 14] |
| Coordinator single point of failure | High (single process, no HA) | High -- fleet inoperable | Watchdog + queue persistence; HA deferred to L3.2+ [Agent 14] |
| Self-improvement loop corrupts codebase | Low (test gates exist) | Critical -- repo damaged | Container isolation; git worktrees; auto-revert [Agent 14] |
| Concurrent map crash (R-01, R-02) | High (under race detector) | Critical -- process crash, all sessions lost | Must fix before L1 activation [S1] |
| Sweep 10x cost overrun | High (autonomous agent uses default $5) | High -- $50 vs $5 per sweep | Change handler default to $0.50 (G2.2) [S3] |
| Untrained DecisionModel routes badly | High (no multi-provider data) | Medium -- quality regression | Heuristic fallback at 0.7 threshold; collect data [Agent 14] |

[Agent 14]

### 4.6 Timeline to L3

```
Week 0        Week 2         Week 8         Week 16        Week 20
  |              |              |               |              |
  v              v              v               v              v
  L0 -----> L1 ready ---> L2 ready -----> L3 ready ----> L3+AutoMerge
  (today)   (race fixes,   (budget,        (fleet,         (auto-revert,
             error          cascade,        watchdog,       full autonomy)
             handling)      supervisor)     72h run)
```

**Total elapsed time: approximately 20 weeks (5 months)** [Agent 14].

---

## Part 5: External Landscape

### 5.1 Multi-Agent Orchestration Comparison

| Framework | Stars | Go Support | MCP | Multi-Provider | Cost Routing | Fleet | TUI |
|-----------|-------|-----------|-----|---------------|-------------|-------|-----|
| **ralphglasses** | -- | Native | Native (166 tools) | Claude+Gemini+Codex | 4-tier cascade | Multi-node (proto) | BubbleTea |
| Claude Agent SDK | -- | Community ports | Native | Claude only | No | No | No |
| Google A2A + ADK | -- | Official SDK | Complementary | Multi | No | No | No |
| OpenAI Agents SDK | -- | Community port | Via tool_use | 100+ LLMs | No | No | No |
| LangGraph | 28.3K | None | Adapters | Multi | No | Cloud option | No |
| CrewAI | 45.9K | None | Adapters | Multi | No | No | No |
| AutoGen/MS Agent Framework | 50.4K | None | No | Multi | No | No | No |

**Unique position**: ralphglasses is the only Go-native, multi-provider, MCP-first orchestrator with process management, cost-aware cascade routing, fleet distribution, and a TUI [Agent 9].

**Key framework details**:

- **Claude Agent SDK** (Python/TS): TeammateTool enables peer-to-peer agent coordination. V2 Session API with multi-turn. Community Go ports exist but are thin CLI wrappers. Single-provider limitation is fundamental [Agent 9].
- **Google A2A** (Protocol): JSON-RPC over HTTP with Agent Card discovery. 50+ technology partners. Official Go SDK (`a2a-go/v2`) supports gRPC, JSON-RPC, REST. ADK Go reached 1.0 with OTel integration. Protocol-only -- no runtime, costs, or loops [Agent 9].
- **OpenAI Agents SDK** (Python/JS): Handoffs for delegation, guardrails for parallel validation (tripwires block before token consumption). Provider-agnostic via Chat Completions. No fleet or cost tracking [Agent 9].
- **LangGraph** (~28.3K stars): Graph-based state machines with cycles. LangGraph Cloud for hosted execution. `deepagents` adds planning + subagent spawning. **No Go support** -- fundamental blocker [Agent 9].
- **CrewAI** (~45.9K stars): Role-based agents (role/goal/backstory). Hierarchical delegation with auto-manager. Largest star count. Python only, no process management [Agent 9].
- **AutoGen/MS Agent Framework** (~50.4K stars): Fragmented into AutoGen v0.2, AG2 (fork), and Microsoft Agent Framework (RC Feb 2026). GroupChat coordination pattern. Python/.NET only [Agent 9].

**Recommended adoptions**:
- **Adopt**: Google A2A protocol (official Go SDK at `a2aproject/a2a-go/v2`), official MCP Go SDK (`modelcontextprotocol/go-sdk` v1.4+)
- **Adapt**: OpenAI guardrails pattern (parallel validation), Claude Agent SDK TeammateTool peer-to-peer patterns
- **Ignore**: LangGraph (no Go), CrewAI (no Go), AutoGen (no Go, fragmented ecosystem)

[Agent 9]

### 5.2 MCP Ecosystem Status

| Aspect | Status |
|--------|--------|
| **Spec version** | 2025-11-25 (4th release). Tasks primitive, Extensions framework, OAuth Client ID Metadata, JSON Schema 2020-12 default [Agent 10] |
| **Governance** | Agentic AI Foundation (Linux Foundation). Co-founded by Anthropic, OpenAI, Google, Microsoft, AWS, Block [Agent 10] |
| **Adoption** | 97M+ monthly SDK downloads. 10,000+ active servers. First-class in ChatGPT, Claude, Cursor, Gemini, VS Code [Agent 10] |
| **Go SDK** | Official: `modelcontextprotocol/go-sdk` v1.4+ (semver stable). Community: `mark3labs/mcp-go` v0.46.0 (ralphglasses current). mcpkit: internal framework [Agent 10] |
| **Transport** | stdio (recommended for local), Streamable HTTP (replaces SSE), HTTP+SSE (deprecated) [Agent 10] |
| **2026 roadmap** | Transport evolution, Tasks primitive maturity, enterprise readiness, governance model [Agent 10] |

**Recommended action**: Migrate from mcp-go v0.46.0 to official `modelcontextprotocol/go-sdk` for semver stability. No HTTP transport needed now -- plan for it when multi-host fleet ships [Agent 10].

### 5.3 Provider Capability Matrix and Cost Optimization

| Provider | Model | Tier | Input $/1M | Output $/1M | Cached Input | Context | Max Output |
|----------|-------|------|-----------|------------|-------------|---------|------------|
| Anthropic | Opus 4.6 | Reasoning | $5.00 | $25.00 | $0.50 (90% off) | 1M | 128K |
| Anthropic | Sonnet 4.6 | Coding | $3.00 | $15.00 | $0.30 (90% off) | 1M | 64K |
| Google | Gemini 3.1 Pro | Reasoning | $2.00 | $12.00 | $0.20 (90% off) | 1M | 65K |
| Google | Gemini 3 Flash | Coding | $0.50 | $3.00 | $0.05 (90% off) | 1M | 65K |
| Google | Gemini 2.5 Flash | Worker | $0.30 | $2.50 | $0.03 (90% off) | 1M | 65K |
| Google | Gemini 2.5 Flash-Lite | Ultra-cheap | $0.10 | $0.40 | $0.01 (90% off) | 1M | 65K |
| OpenAI | GPT-5.4 | Reasoning | $2.50 | $15.00 | $0.25 (90% off) | 1M | 33K |
| OpenAI | codex-mini-latest | Coding | $1.50 | $6.00 | $0.375 (75% off) | 200K | 16K |
| OpenAI | GPT-4.1-nano | Classifier | $0.05 | $0.20 | $0.025 (50% off) | 1M | 16K |

**Opus 4.6**: Only frontier model with flat pricing across full 1M context (no long-context surcharge). Price dropped 67% from $15/$75 to $5/$25 [Agent 11].

**Gemini 3.1 Pro**: SWE-bench Verified 80.6% vs Opus 80.8% at 60% lower cost -- viable Opus alternative for planning tasks [Agent 11].

**Recommended cascade**:

```
Tier 0: GPT-4.1-nano ($0.05/$0.20)     -- classification, routing
Tier 1: Gemini 2.5 Flash ($0.30/$2.50)  -- bulk work, docs, tests
Tier 2: Sonnet 4.6 ($3.00/$15.00)       -- complex coding, architecture
Tier 3: Opus 4.6 ($5.00/$25.00)         -- multi-step reasoning, planning
```

**Cost reduction projection**: $0.17 avg task cost to $0.03-0.05 (70-85% reduction) via combined prompt caching + 4-tier cascade + batch API [Agent 11].

**Stale compiled-in rates**: Opus at $15/$75 (3x actual), Gemini Flash output at $2.50 (~40% below actual $3.50) [S3].

**Prompt caching savings by provider**:

| Provider | Mechanism | TTL | Cache Read Discount | Auto-Activated? | Source |
|----------|-----------|-----|--------------------|-----------------| -------|
| Claude | `cache_control` breakpoints | 5min (1.25x write) / 1hr (2x write) | 90% | Yes (default on) | Agent 4, Agent 11 |
| Gemini | `CreateCachedContent()` REST API | 1hr | 90% | **No** -- operator must call manually | Agent 4 |
| OpenAI | Automatic prefix caching (Responses API) | Automatic | 50% | Yes (no config needed) | Agent 4, Agent 11 |

For a typical 10-turn session with 15K-token stable prefix: ~$0.085 cached vs $0.45 uncached = **81% input savings** on Claude [Agent 11].

**CLI feature comparison**:

| Feature | Claude Code | Gemini CLI | Codex CLI |
|---------|------------|------------|-----------|
| MCP Support | Native | Native (stdio + SSE) | Native (stdio) |
| Subagents | Built-in (Explore, Plan, General) | No | No |
| Worktrees | `--worktree` flag | No | Workspace sandbox |
| Hooks | PreToolUse, PostToolUse, etc. | No | No |
| Session Resume | `--resume` | `--resume` | `exec resume` |
| Sandbox Modes | Permission-based | N/A | suggest / auto-edit / full-auto |
| Max Context | 1M tokens | 2M tokens | 1M (model-dependent) |
| Compaction | Server-side auto-summarization | N/A | N/A |

[Agent 11]

### 5.4 Thin Client Recommendations

| Decision | Recommendation | Source |
|----------|---------------|--------|
| **Distro base (short-term)** | Stay Manjaro -- existing `Dockerfile.manjaro` works for Phase 4 completion | Agent 12 |
| **Distro base (medium-term)** | Migrate to NixOS -- declarative model eliminates drift across 10+ thin clients | Agent 12 |
| **Primary compositor** | Sway -- most stable wlroots compositor for NVIDIA multi-monitor | Agent 12 |
| **Secondary compositor** | Hyprland -- opt-in alternative, best-effort parity | Agent 12 |
| **Drop** | i3 -- X11 is end-of-life; archive `distro/i3/`, `distro/xorg/` | Agent 12 |
| **Fleet workers** | Evaluate Cage -- zero-config kiosk compositor for single maximized app | Agent 12 |
| **Boot pipeline** | greetd + tuigreet -- 4-step chain (kernel -> systemd -> greetd -> Sway) vs current 6-step | Agent 12 |
| **Secrets** | systemd-creds + TPM2 for API keys on thin clients | Agent 12 |
| **Dual GPU** | Sway renders on single GPU; use one RTX 4090 for display + one for compute | Agent 12 |

---

## Part 6: Fleet Operations

### 6.1 Fleet Readiness Tiers

| Tier | Count | % | Description | Repos |
|------|-------|---|-------------|-------|
| **T1 (Fleet-Ready)** | 19 | 25% | CLAUDE.md + go.mod + Makefile -- sweepable now | claudekit, dotfiles-mcp, hg-mcp, mcpkit, ralphglasses, systemd-mcp, tmux-mcp, process-mcp, [private-ops], [private-ops-2], mesmer, webb, webbb, [private-audit], hyprland-mcp, input-mcp, shader-mcp, prompt-improver, [private]-old |
| **T2 (Partially Ready)** | 5 | 6% | Has build infra but missing agent context | gh-dash, pinecone-canopy, runmylife, terraform-docs, whiteclaw |
| **T3 (Needs Setup)** | 44 | 57% | Missing core fleet infrastructure | cr8-cli, dotfiles, archlet, [private], and 40 others |
| **T4 (Not Applicable)** | 8 | 10% | Forks, dormant, non-git | cmatrix, lnav, makima, etc. |

**Inner ring** (7 repos with `.ralphrc` + `.ralph/`): claudekit, hg-mcp, [private-ops], mcpkit, mesmer, ralphglasses, whiteclaw [Agent 15].

**Dependency health**: 9 repos depend on mcpkit. 5 active repos use `../mcpkit` relative replace directives (consistent dev-mode pattern). 4 repos use absolute paths that break portability (2 deprecated, 2 legacy macOS paths). claudekit is the only active mcpkit consumer that builds from fresh clone. All 20 org-owned Go repos are on Go 1.26.1 [Agent 15].

**Sweep cost economics**: At 19 T1-ready repos with $0.50/session budget, a full sweep costs approximately $9.50. With 4-tier cascade routing achieving 70% Gemini Flash routing, estimated sweep cost drops to $3.23. A 74-repo sweep (future T1+T2+upgraded T3) at optimized rates: ~$12-15 per run [Agent 15, Agent 11].

### 6.2 Recommended Onboarding Order

Based on fleet readiness, test coverage, and strategic importance:

| Priority | Repos | Rationale |
|----------|-------|-----------|
| **Wave 1** (now) | mcpkit, claudekit, dotfiles-mcp, systemd-mcp, tmux-mcp, process-mcp | Core frameworks + public MCP servers. All T1-ready. |
| **Wave 2** | hg-mcp, mesmer, [private-audit], [private-ops] | Active T1 repos with .ralph/ integration. |
| **Wave 3** | ralphglasses (self), prompt-improver | Self-improvement and prompt tooling. |
| **Wave 4** | cr8-cli, dotfiles, runmylife | Non-Go or partially-ready repos needing CLAUDE.md or Makefile. |

### 6.3 Fleet Scaling Requirements for L3

| Requirement | Current State | L3 Need | Source |
|-------------|--------------|---------|--------|
| Queue persistence | In-memory only | Auto-save every 30s, restore on startup | Agent 6 |
| Queue bounding | Unbounded map | MaxDepth with rejection on overflow | Agent 6 |
| Sweep parallelism | Serial for-loop | Bounded goroutine pool (size 10) | Agent 6 |
| Autoscaler | Advisory only (event, no actuator) | Local worker spawner for single-machine | Agent 6 |
| Worker timeout | No upper bound on status polling | `2 * DefaultStallThreshold` (~15 min) | Agent 6 |
| Budget ceiling | No fleet-level hard cap | `FleetBudgetCapUSD` + per-hour circuit breaker | S3 |
| Cost predictor | Not auto-fed from coordinator | Wire `Record()` to `handleWorkComplete` | S3 |
| A2A integration | Adapter and coordinator not integrated | Wire `CompleteWork` to `adapter.CompleteOffer` | Agent 6 |
| Multi-node | Protocol-ready, single coordinator SPOF | Queue persistence + Tailscale networking | Agent 6, Agent 15 |
| Max fleet concurrency | 128 sessions (32 workers x 4 sessions) | Sufficient for single-machine L3 | Agent 6 |

---

## Part 7: Recommended Actions

### 7.1 Immediate Actions (Next 2 Weeks) -- CRITICAL Fixes

| # | Action | Source | Effort | Impact |
|---|--------|--------|--------|--------|
| 1 | Add `sync.Mutex` to `AutoRecovery.retryState` | S1 R-01 | S (5 lines) | Eliminates fatal crash risk at L1+ |
| 2 | Add `sync.Mutex` to `RetryTracker.attempts` | S1 R-02 | S (5 lines) | Eliminates fleet crash risk |
| 3 | Fix 5 path traversal vulnerabilities in MCP handlers | Agent 8 | S-M | Blocks any public MCP exposure |
| 4 | Surface supervisor cycle failures at Error level | S4 | M | Prevents silent L1/L2 spin |
| 5 | Propagate RunLoop errors in background goroutines | S4 | S | Makes self-improvement failures visible |
| 6 | Add hook exit code handling | S4 | S | Safety gates become functional |
| 7 | Make `GateEnabled` an `atomic.Bool` | S1 R-03 | S (3 lines) | Fixes HIGH race |
| 8 | Protect `loadedGroups` map with mutex | S1 R-06 | S | Fixes HIGH MCP dispatch race |
| 9 | Enforce mandatory $5 default budget floor | S3 Gap A | S | Eliminates uncapped sessions |
| 10 | Fix autonomy level persistence failure | S4 | S | L2/L3 survives restart |

### 7.2 Medium-Term Actions (Next Quarter) -- L2 Enablement

| # | Action | Source | Effort | Impact |
|---|--------|--------|--------|--------|
| 1 | Fix all 7 failing test packages to achieve green CI | Agent 5 | M | Establishes autonomous operation gate |
| 2 | Update compiled-in provider cost rates to April 2026 pricing | S3, Agent 11 | S | Accurate cascade routing decisions |
| 3 | Activate deferred loading in production binary | Agent 2 | S | Halves MCP startup latency |
| 4 | Wire `fleet/CostPredictor.Record()` to `handleWorkComplete` | S3 | S | Fleet-level cost forecasting |
| 5 | Activate 4-tier cascade routing with Gemini Flash as Tier 1 | Agent 11 | M | 70-85% cost reduction |
| 6 | Migrate to official `modelcontextprotocol/go-sdk` | Agent 10 | XL | Semver stability, spec compliance |
| 7 | Split `advanced` namespace (24 tools) into rc/autonomy/workflow/residual | Agent 2 | M | Cleaner tool organization |
| 8 | Add max-lines cap to `LogView.Lines` (ring buffer, 10K default) | Agent 7 | S | Prevents memory exhaustion |
| 9 | Collect 50+ multi-provider observations for DecisionModel training | Agent 14 | M | Enables calibrated cascade routing |
| 10 | Run 24-hour supervisor session on Manjaro | Agent 14, Agent 3 | M | Validates L2 readiness |

### 7.3 Long-Term Investments (Next 6 Months) -- L3 and Fleet Scaling

| # | Action | Source | Effort | Impact |
|---|--------|--------|--------|--------|
| 1 | Implement per-hour spend circuit breaker ($50/hr) | Agent 14, S3 | M | Prevents $1,280/hr worst case |
| 2 | Persist work queue (auto-save 30s, restore on startup) | Agent 6 | M | Queue survives coordinator restart |
| 3 | Parallelize sweep fan-out (semaphore, size 10) | Agent 6 | M | 74-repo sweep in seconds, not minutes |
| 4 | Add local autoscaler actuator (spawn worker instances) | Agent 6 | M | Dynamic capacity for L3 |
| 5 | Complete ISO build pipeline for Manjaro thin client | Agent 12 | L | Bootable agent workstations |
| 6 | Implement Phase 13.1 self-healing runtime (heartbeat, crash recovery) | Agent 1, Agent 14 | XL | L3 foundation |
| 7 | Adopt A2A protocol via official Go SDK | Agent 9, Agent 10 | L | Cross-agent interop |
| 8 | Implement per-provider circuit breakers (replace shared CB) | Agent 4, S1 R-13 | M | Provider isolation |
| 9 | Add systemd watchdog for supervisor process | Agent 14 | M | Restart-on-crash for 72h runs |
| 10 | Validate 72-hour L3 unattended operation (AutoMerge disabled) | Agent 14 | L | Full autonomy gate |

---

## Appendix A: Finding Index

Full finding index with 82 cataloged findings is available in [16-research-index.md](16-research-index.md).

**Distribution**:

| Severity | Count | Range |
|----------|-------|-------|
| CRITICAL | 6 | F-001 through F-006 |
| HIGH | 18 | F-007 through F-024 |
| MEDIUM | 25 | F-025 through F-049 |
| LOW | 17 | F-050 through F-066 |
| INFO | 16 | F-067 through F-082 |

### Contradiction Register

9 contradictions resolved between research reports [Agent 16]:

| # | Topic | Report A | Report B | Resolution |
|---|-------|----------|----------|------------|
| C-01 | MCP tool count | CLAUDE.md: 126/14 | Agent 2: 166/16 | CLAUDE.md stale. `plugin` and `sweep` namespaces added; several grew. True: 166/16. |
| C-02 | Roadmap task count | ROADMAP.md header: 1,115/442 | Agent 1: 1,143/503 | Header predates recent phases. Live count authoritative. |
| C-03 | Phase 3.5 completion | Agent 8: 28/30 (93%) | Agent 1: 24/30 (80%) | Different counting of duplicate 3.5.5 section. Agent 1 more reliable. |
| C-04 | In-progress phases | Agent 8: 10 | Agent 1: 6 | Different thresholds for "in progress" -- both valid by their definitions. |
| C-05 | Deferred loading | CLAUDE.md: active | Agent 2: inactive in production | Production `cmd/mcp.go` does not set `DeferredLoading = true`. Tests only. |
| C-06 | Opus pricing | `config/costs.go`: $15/$75 | Agent 11: $5/$25 | Code not updated for Opus 4.6 price drop. 67% overestimate. |
| C-07 | Phase 9 completion | ROADMAP.md: 100% | Agent 1, S2: files missing | "5 tasks" are tier-3 only. Tier-1 tool implementations absent. |
| C-08 | Gemini Flash rate | `config/costs.go`: $2.50/1M output | Agent 11: ~$3.50/1M | 40% underestimate in compiled-in rate. |
| C-09 | Sweep budget | User convention: $0.50/session | Handler default: $5.00/session | $0.50 is caller convention only; handler does not enforce without explicit param. |

### Research Gaps

| # | Gap | Expected | Actual | Impact |
|---|-----|----------|--------|--------|
| G-01 | Plugin system | Full audit of `internal/plugin/` | Mentioned as dead-code only | Low -- Phase 20 is 0% done |
| G-02 | Sandbox infrastructure | Full audit of `internal/sandbox/` | Mentioned in passing | Medium -- Phase 5 is 12% done |
| G-03 | Store layer | Full SQLite audit | Covered architecturally, not schema-level | Medium -- migration planned |
| G-04 | K8s controller | Analysis of `internal/k8s/` | Only touched in S4 error audit | Low -- Phase 7 is 0% done |
| G-05 | Batch API integration | Cost optimization via batch | Mentioned; no code audit | Low -- optimization tier |

[Agent 16]

---

## Appendix B: Metric Summary

| Metric | Value | Source |
|--------|-------|--------|
| Roadmap total tasks | 1,143 | Agent 1 |
| Roadmap done tasks | 503 (44.0%) | Agent 1 |
| Phases complete (100%) | 10 | Agent 1 |
| Phases in progress | 6-10 | Agent 1 |
| Phases fully planned (0%) | 10-11 | Agent 1 |
| MCP tool count | 166 across 16 namespaces | Agent 2 |
| Test file count | 745 | Agent 5 |
| Test function count | 9,267 | Agent 5 |
| Overall coverage | 84.5% | Agent 5 |
| Failing packages | 7 | Agent 5 |
| Race conditions (CRITICAL+HIGH) | 6 (2+4) | S1 |
| Race conditions (total) | 20 | S1 |
| Dead code (confirmed) | 443 lines | S2 |
| Phantom ROADMAP references | 158 (5 in completed tasks) | S2 |
| Duplicate CircuitBreaker impls | 4 | S2 |
| Budget enforcement gaps | 4 (Gaps A-D) | S3 |
| Max theoretical L3 spend | $1,280/hr (uncapped) | S3 |
| Average task cost | $0.17 | Agent 8 |
| Cost optimization potential | $0.03-0.05/task (70-85% reduction) | Agent 11 |
| Swallowed errors (top 20) | 4 high, 10 medium, 6 low | S4 |
| Error wrapping ratio | 65% global | S4 |
| Handler contract violations | 0 | S4 |
| Production panics | 2 (both justified) | S4 |
| Enhancer pipeline stages | 13 | Agent 4 |
| Lint rules | 14 | Agent 4 |
| Scoring dimensions | 10 (weights sum to 1.00) | Agent 4 |
| E2E test scenarios | 24 | Agent 5 |
| Fleet-ready repos | 19/77 (25%) | Agent 15 |
| Max fleet concurrency | 128 sessions | Agent 6 |
| L3 critical path tasks | ~100 (all 0% done) | Agent 1 |
| Time to L3+AutoMerge | ~20 weeks | Agent 14 |
| Provider distribution | 100% Claude (no multi-provider data) | Agent 8 |
| JSON format retry rate | 25.7% (target <5%) | Agent 8 |

---

## Appendix C: Phase Status Matrix

Condensed from [01-roadmap-matrix.md](01-roadmap-matrix.md). 36 phases total.

| Phase | Name | Total | Done | % | Status |
|-------|------|-------|------|---|--------|
| 0 | Foundation | 16 | 16 | 100% | Complete |
| 0.5 | Critical Fixes | 45 | 45 | 100% | Complete |
| 0.6 | Code Quality & Observability | 39 | 39 | 100% | Complete |
| 0.8 | MCP Observability & Scratchpad | 20 | 20 | 100% | Complete |
| 0.9 | Quick Wins | 12 | 12 | 100% | Complete |
| 1 | Harden & Test | 55 | 53 | 96% | In Progress |
| 1.5 | Developer Experience | 52 | 43 | 83% | In Progress |
| 2 | Multi-Session Fleet | 70 | 70 | 100% | Complete |
| 2.5 | Multi-LLM Orchestration | 27 | 27 | 100% | Complete |
| 2.75 | Architecture Extensions | 33 | 33 | 100% | Complete |
| 3 | i3 Multi-Monitor Integration | 35 | 24 | 69% | In Progress |
| 3.5 | Theme & Plugin Ecosystem | 30 | 24 | 80% | In Progress |
| 4 | Bootable Thin Client | 53 | 11 | 21% | In Progress |
| 5 | Agent Sandboxing | 40 | 5 | 12% | In Progress |
| 6 | Advanced Fleet Intelligence | 50 | 42 | 84% | In Progress |
| 7 | Kubernetes & Cloud Fleet | 25 | 0 | 0% | Planned |
| 8 | Advanced Orchestration & AI-Native | 33 | 8 | 24% | In Progress |
| 9 | R&D Cycle Automation | 5 | 5 | 100% | Complete* |
| 9.5 | Autonomous R&D Supervisor | 5 | 5 | 100% | Complete |
| 10 | Claude Code Native Integration | 20 | 3 | 15% | In Progress |
| 10.5 | Horizontal & Vertical Scaling | 48 | 14 | 29% | In Progress |
| 11 | A2A Protocol Integration | 21 | 0 | 0% | Planned |
| 12 | Tailscale Fleet Networking | 26 | 0 | 0% | Planned |
| 13 | Level 3 Autonomy Core | 40 | 0 | 0% | Planned |
| 14 | Agent Memory & Meta-Learning | 31 | 0 | 0% | Planned |
| 15 | Advanced Fleet Intelligence v2 | 32 | 0 | 0% | Planned |
| 16 | Edge & Embedded Agents | 23 | 0 | 0% | Planned |
| 17 | AI Safety & Governance | 37 | 0 | 0% | Planned |
| 18 | World Models & Predictive | 28 | 0 | 0% | Planned |
| 19 | Cross-Repository Orchestration | 29 | 0 | 0% | Planned |
| 20 | Agent Marketplace & Ecosystem | 43 | 0 | 0% | Planned |
| 21 | Observability & Evaluation | 30 | 0 | 0% | Planned |
| 22 | DevOps & Infrastructure | 45 | 0 | 0% | Planned |
| 23 | Advanced Prompt Engineering | 23 | 0 | 0% | Planned |
| 24 | MoE-Inspired Provider Routing | 10 | 0 | 0% | Planned |
| 25 | Federated Fleet Learning | 9 | 0 | 0% | Planned |

*Phase 9 marked 100% but tier-1 implementation files are missing [Agent 1, S2].

**Summary**: 10 phases complete, 10 in progress (12-96%), 16 planned at 0%.

### Roadmap Structural Issues

**Numbering collision (CRITICAL)**: Phase 3.5.5 has two sections with identical sub-IDs -- "Codex-primary command/control parity" (4 open P0/P1 tasks) and "Theme export to terminal" (4 done P2 tasks). Any tooling addressing tasks by ID has undefined behavior. **Recommendation**: Renumber Codex parity to 3.5.6 [Agent 1].

**Duplicate phase name**: Phase 6 and Phase 15 are both named "Advanced Fleet Intelligence" covering different scope. **Recommendation**: Rename Phase 15 to "Distributed Swarm Intelligence & Scheduling" [Agent 1].

**Stale header**: ROADMAP.md header says "1,115 tasks, 442 complete" but live checkbox count is 1,143 / 503. Metrics tracking uses the wrong baseline [Agent 1].

**Phantom file references**: 158 total phantom references in ROADMAP.md. 5 are in completed tasks (highest priority to reconcile): QW-7 (`snapshot.go`), QW-11 (`coordinator.go`), 0.5.7.1 (`version.go`), 0.5.11.1 (`config_schema.go`), 1.8.4 (`internal/errors/` package) [S2].

**Orphaned tasks with missing code**: Phase 9 tier-1 tools (`finding_to_task`, `cycle_merge`, `cycle_plan`, `cycle_schedule`, `cycle_baseline`) have `tools_loop_test.go` but no `tools_loop.go`. If the supervisor calls these tools autonomously, they may not have working handlers [Agent 1, S2].

### Phases with Unrealistic Scope

**Phase 13 (L3 Autonomy Core)**: 40 tasks, 0% done, 3 XL sections, all implementation files missing. The L3 target (72-hour unattended operation) requires `self_heal.go` + `decision_engine.go` + `unattended.go` + `param_tuner.go` to all work correctly together. Highest-risk scope in the roadmap [Agent 1].

**Phase 10.5 (Scaling)**: 18% XL fraction, all blocking L3. Multi-node marathon (10.5.6) and autonomy scaling (10.5.11) are both XL with external infrastructure dependencies (NATS, multi-machine coordination) [Agent 1].

**Phases 17, 24-25**: Phase 17 (Safety) has `XL` gate; Phases 17.3-17.4 (PRMs, adversarial testing) require fine-tuning infrastructure not in the stack. Phase 25 (Federated Learning with DP-SGD, Shamir secret sharing) is the most academically ambitious item with no realistic production path before L3 is stable [Agent 1].

### Critical Path to L3

```
Phase 10.5 (29% done, ~34 tasks remaining)
  --> Phase 13.1 Self-Healing Runtime (0%, 10 tasks)
    --> Phase 13.3 Autonomous Decision Engine (0%, 8 tasks)
      --> Phase 13.5 Unattended Operation (0%, 8 tasks)
        --> Phase 17.1 Safety Guardrails (0%, 10 tasks)
          --> Phase 14.1 Persistent Memory (0%, 10 tasks)
            --> Phase 15.1 Intelligent Router (0%, 10 tasks)
```

**Critical path: ~100 tasks, all L/XL sized, all at 0%** [Agent 1].

### Parallelizable Backlog (Independent of L3)

| Phase | Tasks | Why Independent |
|-------|-------|----------------|
| 19 (Cross-Repo) | 29 | Only needs existing fleet package |
| 22 (DevOps) | 45 | CI/CD, security scanning are standalone |
| 23 (Prompt Engineering) | 23 | Builds on existing enhancer |
| 16.1 (Edge/Ollama) | ~10 | Provider additions |
| 11 (A2A) | 21 | Partial impl exists; SDK independent |
| 24 (MoE Routing) | 10 | Extends existing cascade + bandit |
| **Total** | **~138** | |

[Agent 1]

---

## Appendix D: Research Index

Full research sweep covered 20 reports by 24 agents. Source documents with key URLs are indexed in [16-research-index.md](16-research-index.md).

### Report Inventory

| Report | Agent | Focus | Key Deliverable |
|--------|-------|-------|----------------|
| 01-roadmap-matrix.md | Agent 1 | ROADMAP.md analysis | 36 phases, 503/1143 tasks, dependency DAG |
| 02-mcp-tool-audit.md | Agent 2 | MCP tool layer | 166 tools, annotation gaps, namespace issues |
| 03-session-architecture.md | Agent 3 | Session internals | State machine, 3-lock hierarchy, 7 race risks |
| 04-enhancer-pipeline.md | Agent 4 | Prompt enhancement | 13 stages, 14 lint rules, shared CB gap |
| 05-test-coverage.md | Agent 5 | Test health | 9,267 tests, 7 failing packages, priority fix list |
| 06-fleet-sweep.md | Agent 6 | Fleet subsystems | Unbounded queue, advisory autoscaler, serial sweep |
| 07-tui-audit.md | Agent 7 | TUI architecture | 21 views, unbounded LogView, O(n) tick |
| 08-distro-audit.md | Agent 8 | Thin client distro | No encryption, no secrets, decoupled GPU profiles |
| 09-orchestration-landscape.md | Agent 9 | Competitive analysis | Unique Go-native position, A2A/MCP SDK adoption |
| 10-mcp-ecosystem.md | Agent 10 | MCP protocol | Official Go SDK v1.4+, 10K+ servers, AAIF governance |
| 11-llm-capabilities.md | Agent 11 | Provider pricing | $0.17 to $0.03 possible, Opus price drop 67% |
| 12-thin-client-patterns.md | Agent 12 | Boot architecture | Manjaro short-term, NixOS medium-term, drop i3 |
| 13-strategic-initiatives.md | Agent 13 | Strategic plan | 10 initiatives, Q2 2026 to Q1 2027 |
| 14-autonomy-path.md | Agent 14 | Autonomy progression | L1 2wk, L2 8wk, L3 16-20wk, safety gates |
| 15-fleet-readiness.md | Agent 15 | Cross-repo readiness | 19/77 repos fleet-ready, $3.23 sweep cost |
| 16-research-index.md | Agent 16 | Master index | 82 findings, 9 contradictions, metrics |
| s1-race-condition-census.md | S1 | Race conditions | 2 CRITICAL + 4 HIGH + 8 MEDIUM races |
| s2-dead-code-audit.md | S2 | Dead code | 443 lines dead, 4 CB copies, 158 phantoms |
| s3-cost-model-analysis.md | S3 | Cost model | 4 budget gaps, $1,280/hr max, stale rates |
| s4-error-handling-audit.md | S4 | Error handling | Supervisor swallows errors, 0 handler violations |

### Report Statistics

| Report | Word Count (est.) | Findings | Key Tables |
|--------|------------------|----------|------------|
| 01-roadmap-matrix | ~4,500 | 7 | 36-phase status matrix, dependency DAG, task size distribution |
| 02-mcp-tool-audit | ~3,800 | 11 | 16-namespace inventory, handler quality matrix, annotation gaps |
| 03-session-architecture | ~5,000 | 8 | State machine, lock hierarchy, cascade routing, supervisor tick |
| 04-enhancer-pipeline | ~5,500 | 15 | 13-stage pipeline, 14 lint rules, 10 scoring dimensions, 3 LLM clients |
| 05-test-coverage | ~3,500 | 10 | Coverage by blast radius, 7 failing packages, priority fix list |
| 06-fleet-sweep | ~4,200 | 10 | Job pipeline, 3 budget layers, autoscaler, A2A status, topology |
| 07-tui-audit | ~3,000 | 8 | View stack, tick bottleneck, memory model, component inventory |
| 08-distro-audit | ~3,500 | 6 | Compositor abstraction, GPU detection, boot pipeline, WM comparison |
| 09-orchestration-landscape | ~4,000 | 1 | 11-framework comparison, adopt/adapt/ignore matrix |
| 10-mcp-ecosystem | ~3,000 | 1 | Spec timeline, Go SDK comparison, gateway landscape |
| 11-llm-capabilities | ~3,500 | 3 | Provider pricing matrix, cascade config, cost projections |
| 12-thin-client-patterns | ~3,800 | 3 | 5-distro comparison, compositor strategy, multi-GPU, boot pipeline |
| 13-strategic-initiatives | ~4,500 | 0 | 10 initiatives, dependency DAG, quarterly timeline |
| 14-autonomy-path | ~5,000 | 0 | Gate criteria L0-L3, safety interlocks, risk matrix, 20-week timeline |
| 15-fleet-readiness | ~3,200 | 0 | 77-repo tier inventory, dependency health, sweep readiness |
| 16-research-index | ~3,500 | 82 (index) | Master finding table, contradiction register, gap register |
| s1-race-condition-census | ~3,000 | 20 | Race severity table, detailed CRITICAL/HIGH analysis |
| s2-dead-code-audit | ~2,800 | 6 | Phantom references, dead code inventory, orphan test analysis |
| s3-cost-model-analysis | ~3,200 | 7 | Cost tracking architecture, budget enforcement map, rate accuracy |
| s4-error-handling-audit | ~2,800 | 20 | Handler compliance, panic inventory, top-20 swallowed errors |

### Key External References

| Topic | URL | Source |
|-------|-----|--------|
| MCP Spec 2025-11-25 | modelcontextprotocol.io/specification/2025-11-25 | Agent 10 |
| Official MCP Go SDK | github.com/modelcontextprotocol/go-sdk | Agent 10 |
| A2A Go SDK | github.com/a2aproject/a2a-go | Agent 9 |
| Anthropic Pricing | platform.claude.com/docs/en/about-claude/pricing | Agent 11 |
| Google Pricing | ai.google.dev/gemini-api/docs/pricing | Agent 11 |
| OpenAI Pricing | developers.openai.com/api/docs/pricing | Agent 11 |
| AAIF Formation | linuxfoundation.org/press/linux-foundation-announces-the-formation-of-the-agentic-ai-foundation | Agent 10 |
| Claude Agent SDK | platform.claude.com/docs/en/agent-sdk/overview | Agent 9 |
| Confidence-Based Cascading | arxiv.org/abs/2410.10347 | Agent 11 |
| NixOS Kiosk | github.com/matthewbauer/nixiosk | Agent 12 |
| Cage Compositor | github.com/cage-kiosk/cage | Agent 12 |
