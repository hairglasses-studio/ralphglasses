# 05 — Test Coverage Audit (ralphglasses)
Generated: 2026-04-04
Source: direct file reads + grep analysis of ralphglasses codebase

---

## Executive Summary

Overall reported coverage is **84.5%** (from `.ralph/coverage.txt`). The test corpus is large
(745 test files, 9,267 `Test*` functions across the project), but that volume masks structural
weaknesses: 7 packages are actively failing, 80 coverage-padding files inflate numbers without
always exercising real invariants, and the highest-blast-radius package (`internal/session`,
2,203 test functions) has 6 failing tests including the critical `knowledge` race condition.

---

## 1. Coverage by Blast Radius

### T1 — Highest Blast Radius

A bug here breaks every session, MCP tool call, or fleet dispatch. These packages are in the
critical path for all agent orchestration.

| Package | Test Files | `Test*` Functions | Reported Coverage | Status |
|---------|-----------|------------------|-------------------|--------|
| `internal/session` | 193 | 2,203 | 83.1% | FAIL — 6 failing tests |
| `internal/mcpserver` | 76 | 1,084 | 74.1% | FAIL — 1 failing test |
| `internal/fleet` | 48 | 625 | 77.8% | PASS |
| `internal/fleet/pool` | 4 | ~60 | 97.9% | PASS |

T1 combined: ~3,972 test functions. Coverage floor: 74.1% (`mcpserver`). Both failing packages
are in this tier, making the current `go test ./...` run non-green.

### T2 — Medium Blast Radius

Bugs here degrade prompt quality, display fidelity, or compositor integration but do not
immediately break session dispatch.

| Package | Test Files | `Test*` Functions | Reported Coverage | Status |
|---------|-----------|------------------|-------------------|--------|
| `internal/enhancer` | 30 | 314 | 93.8% | PASS |
| `internal/enhancer/knowledge` | ~3 | ~40 | 90.5% | FAIL — race condition |
| `internal/tui` (all sub-packages) | 86 | 1,148 | 70.1–90.2% range | PASS |
| `internal/wm` (all sub-packages) | 11 | 156 | 52.4–90.2% range | PASS |

T2 combined: ~1,658 test functions. `wm` coverage is bimodal: `wm/layout` reaches 90.2% but
`wm` root is 52.4%, and `wm/hyprland` is 58.5%.

### T3 — Lower Blast Radius

Everything else: tooling, plugins, process monitoring, store, e2e harness, cmd entrypoints.

| Key Package(s) | Test Files | Coverage | Status |
|----------------|-----------|----------|--------|
| `internal/e2e` | 14 | 91.2% | PASS |
| `internal/process` | 19 | 88.3% | FAIL — 1 test |
| `internal/marathon` | 6 | 88.3% | FAIL — 1 test |
| `internal/knowledge` (session sub) | — | — | FAIL — fatal race |
| `cmd/ralphglasses-mcp` | 10 | 59.1% | FAIL — count mismatch |
| `cmd/prompt-improver` | 6 | N/A | FAIL — API key |
| `internal/flash` | ~2 | 32.3% | PASS |
| `internal/plugin/builtin` | ~3 | 35.5% | PASS |
| `cmd` (root) | 10 | 46.8% | PASS |

---

## 2. Gap Analysis — T1 Package Details

### internal/session (83.1%, 6 failing tests)

Critical subsystems with tests:
- `AutoRecovery.HandleSessionError` — has 7 tests in `autorecovery_lifecycle_test.go`
- `ClearRetryState` — tested in `autorecovery_test.go`
- `RunCycle` — tested in `manager_cycle_test.go` (3 tests)
- `RunSessionOutput` — tested in `runner_test.go` and `runner_coverage_test.go`
- `TruncateStr` — tested in `runner_test.go` and `runner_helpers_test.go`
- `ConfigOptimizer` — tested in `config_optimizer_test.go`
- Cascade routing — `ShouldCascade`, `SelectTier`, `EvaluateCheapResult` have dedicated unit tests
- Supervisor lifecycle — `supervisor_integration_test.go` + `supervisor_integration2_test.go`
- Loop engine — `loop_test.go`, `loop_steps_test.go` cover `StepLoop` path

**Failing tests (still active per baseline):**
- `WeeklyReport` — generator is in `weekly_report.go`; tests exist but assertions fail on
  anomalous data inputs (likely boundary conditions in `Generate()`)
- `TruncateStr` — boundary condition in runner helper
- `RunSessionOutput` — cost extraction assertion fails
- `RunCycle` — context/timeout handling edge case
- `ConfigOptimizer` x2 — optimizer suggestion logic returns unexpected values

**Critical unprotected path (from Wave 3):**
- `AutoRecovery.retryState` — map with no mutex. Tests exercise this path under
  `autorecovery_lifecycle_test.go` but none run with `-race` targeting concurrent
  `onComplete` callbacks on different sessions sharing state. The `smoke` Makefile target
  runs race detection on supervisor/loop tests but not specifically on
  `HandleSessionError` concurrent paths.

### internal/mcpserver (74.1%, 1 failing test)

Critical subsystems with tests:
- Sweep handlers — `handler_sweep_test.go` has 13 tests covering `sweep_generate`,
  `sweep_launch`, `sweep_status`, `sweep_nudge`, `resolveSweepRepos`, budget cap enforcement
- Cascade config — `TestCascadeConfigFromRepo_*` in `handler_coverage_boost2_test.go`
- Supervisor status — `TestHandleSupervisorStatus_*` in `handler_coverage_boost2_test.go`
- Circuit breaker — `handler_circuit_test.go` has 4 tests

**Failing test:**
- `TestHandleCircuitReset_Enhancer` — expects `status=reset` in response but gets `nil`.
  The enhancer circuit breaker state is not populated in the test server setup.
  File: `~/hairglasses-studio/ralphglasses/internal/mcpserver/handler_circuit_test.go`

**Untested handler files (no `_test.go` counterpart) — from Wave 1:**
- `handler_sweep_report.go` (302 lines) — `sweep_report` handler has no test. This is
  the highest-risk gap: complex Markdown/JSON rendering, multi-section output, no assertions.
- `handler_prompt_ab.go` (200 lines) — A/B test result writing, file I/O, no test.
- `handler_provider_benchmark.go` (268 lines) — percentile math, keyword scoring, no test.
- `handler_roadmap_prioritize.go` (178 lines) — weighted scoring algorithm, no test.
- `handler_session_fork.go` (50 lines) — `SessMgr.Fork` call path, no dedicated test.

**Under-tested handlers (shallow test counts):**
- `handler_team_test.go` — 5 tests for 6 tools; `handleTeamDelegate` and `handleAgentCompose`
  lack direct test cases
- `handler_observation_test.go` — 8 tests; `handleObservationSummary` and
  `handleObservationCorrelate` not directly tested

### internal/fleet (77.8%, PASS)

Well-covered areas:
- Worker dispatch, capacity planning, coordinator logic — `handler_fleet_test.go` has 49 tests
- A2A dispatch — `a2a_coverage_test.go`, `a2a_dispatch_coverage_test.go`
- Autoscaler — `autoscaler_coverage_test.go`, `coverage_boost3_test.go`

Coverage gaps:
- `fleet_grafana` handler — only 37 lines, pure transform, no test (Low risk)
- `handler_recommend.go` — thin wrapper around `fleet.NewRecommender`, no dedicated test (Low)
- Fleet analytics worker interaction paths — `analytics_coverage_test.go` exists but
  worker-level tests focus on state machines, not end-to-end cost aggregation

---

## 3. Test Type Distribution

| Test Type | Total Count | Packages |
|-----------|------------|---------|
| Unit tests | ~7,800 | All packages |
| Coverage-padding tests | ~807 (`Test*` in `*coverage*test*` files) | 80 files across 25 packages |
| Integration tests | ~44 (`*integration*test*` files) | session, mcpserver, wm/sway, internal |
| E2E tests (mock harness) | 24 scenarios + ~60 functions | `internal/e2e` |
| Fuzz tests | 15 `Fuzz*` functions | mcpserver, model, session, tui/components |
| Benchmark tests | 20 functions, 6 files | session, fleet, mcpserver |
| Race-targeted tests | 52 functions with "race/concurrent/parallel" in name | session (all) |
| Live fire tests | 2 (skipped without API key) | `internal/e2e/live_test.go` |

The `smoke` Makefile target runs a meaningful race-detected subset: supervisor, cost extraction
goldens, event bus concurrency — but this is a hand-curated list, not the full suite.

The main `test` target (`go test -race ./...`) runs everything with the race detector, which is
correct and consistent. The CI pipeline (`Makefile:test-cover`) uses race detection too.

---

## 4. Coverage Padding Assessment

**Padding file count:** 80 files matching `*coverage*test*` or `*coverage_boost*` patterns,
contributing approximately 807 `Test*` functions.

**Quality breakdown:**

| Quality Level | File Count | Characteristics |
|--------------|-----------|-----------------|
| Real behavioral tests | ~65 (81%) | Verify return values, file contents, state mutations, error conditions with `t.Fatal`/`t.Error` |
| Structural invocations only | ~10 (13%) | Call the function, verify no panic, skip edge cases |
| Call-and-ignore | ~5 (6%) | Call function, assert non-nil but nothing about the return value |

Sampled examples:
- `internal/fleet/coverage_boost3_test.go` — Tests `SetAutoScalerConfig` and `autoScaleCheck`
  with real assertions on config values and return types. Quality: **real tests**.
- `internal/session/session_coverage4_test.go` — Tests `Supervisor.recordGateFindings` with
  file I/O verification and JSON round-trip assertion. Quality: **real tests**.
- `internal/session/coverage_boost_test.go` — Tests `FeedbackAnalyzer.SuggestProvider` with
  boundary conditions (insufficient samples). Quality: **real tests**.
- `internal/mcpserver/handler_coverage_boost_test.go` — Tests `InitSelfImprovement` idempotency
  and `WireAutoOptimizer` nil safety. Quality: **real tests**.

**Verdict:** The coverage padding files are predominantly real tests that happen to be named
with `_coverage` suffixes. They were generated to reach coverage targets but the overwhelming
majority contain meaningful assertions. The naming convention is misleading but not indicative
of test-padding abuse. The ~6% call-and-ignore category (roughly 5 files) is genuine padding.

---

## 5. Failing Packages

Per the baseline from `meta-roadmap/06-test-coverage.md`, 7 ralphglasses packages fail:

| Package | Failure | Still Active? | Assessment |
|---------|---------|---------------|------------|
| `internal/knowledge` | Fatal concurrent map writes in `TieredKnowledge.Query` | **Yes** — no mutex added | `knowledge/graph.go` has `mu sync.RWMutex` on `KnowledgeGraph`, but `TieredKnowledge` query path appears to bypass it |
| `internal/marathon` | `TestRestartPolicy_BackoffCap` — expects `30s`, gets negative duration | **Yes** — test still exists as written | `Backoff()` overflows when factor=10 and restarts=20 (exponential integer overflow before cap) |
| `internal/mcpserver` | `TestHandleCircuitReset_Enhancer` — expected `status=reset`, got nil | **Yes** — enhancer CB not initialized in test harness | Missing `srv.Enhancer.CircuitBreaker` init in `setupTestServer` |
| `internal/process` | `TestCollectChildPIDs_DeadPID` — expected nil, got `[]` | **Yes** — `[]` vs `nil` slice semantic | Minor: return type distinction between nil and empty slice |
| `internal/session` | 6 tests (WeeklyReport, TruncateStr, RunSessionOutput, RunCycle, ConfigOptimizer x2) | **Yes** — all exist in current test files | Mix of boundary conditions and assertion mismatches |
| `cmd/ralphglasses-mcp` | `TestToolGroupNames` — 16 entries, want 15 | **Yes** — `sweep` namespace was added without updating test expectation | Test in `main_test.go` hardcodes 16 names but expects `len == 15`; off-by-one in expected slice |
| `cmd/prompt-improver` | `ANTHROPIC_API_KEY` not set | **Yes** — environment-gated, always fails in CI without key | Tests call live API without skip guard on key absence |

**The `TestToolGroupNames` failure deserves special attention:** both `cmd/ralphglasses-mcp/main_test.go`
and `internal/mcpserver/tools_deferred_test.go` have a `TestToolGroupNames` function. The
`main_test.go` version checks `len(mcpserver.ToolGroupNames) != len(expected)` where `expected`
has 16 entries, but the failure message says "16 entries, want 15" — meaning the hardcoded
`len(expected)` was never updated when `sweep` was added. This is a stale test expectation, not
a real regression.

---

## 6. Skipped Tests

Grouped by package and reason:

### `cmd/` — 5 skips
| Location | Skip Reason |
|----------|-------------|
| `cmd/cmd_test.go` (×3) | Short mode: Makefile integration, build integration |
| `cmd/prompt-improver/coverage_test.go` | `runCacheCheck` calls `os.Exit`, cannot test in-process |
| `cmd/prompt-improver/main_test.go` (×3) | Slow hook/MCP integration tests |
| `cmd/ralphglasses-mcp/main_test.go` (×2) | Test binary not built |

### `internal/discovery/` — 6 skips
| Location | Skip Reason |
|----------|-------------|
| `discovery_test.go`, `scanner_errors_test.go` | Requires non-root user (×2 each) |
| Both files | Symlinks not supported on this filesystem (×2 each) |

### `internal/e2e/` — 5 skips
| Location | Skip Reason |
|----------|-------------|
| `e2e_test.go` (×3) | Short mode gate |
| `live_test.go` | `ANTHROPIC_API_KEY` not set; "live-fire not yet wired to real providers" |
| `platform_test.go` (×2) | Short mode; no scenarios available |

### `internal/enhancer/` — 2 skips
| Location | Skip Reason |
|----------|-------------|
| `edge_test.go` | Large input test in short mode |
| `knowledge/builder_test.go` | `internal/model` directory not found |

### `internal/fleet/` — 3 skips
| Location | Skip Reason |
|----------|-------------|
| `discovery_test.go` (×3) | Tailscale installed/not installed conditional (negative test) |

### `internal/session/` — 15 skips
| Location | Skip Reason |
|----------|-------------|
| `loop_test.go`, `sprint7_ws3_test.go` (×2) | `git` not on PATH |
| `template_test.go` | `UserHomeDir` failed — likely CI without HOME |
| `worktree_integration_test.go` (×6) | `git` not found |
| `worktree_pool_test.go` (×8) | `git` not found |
| `worktree_test.go` (×1) | `git` not found |

The `git not found` skips are significant: at least 15 worktree tests are silently skipped
in environments without `git` on PATH. The e2e harness itself has a `git.LookPath` guard in
`setupRepo`, so these tests are only live when `git` is available.

### `internal/sandbox/` — 14 skips
Network namespace tests require Linux + root. Appropriate for CI isolation.

### `internal/wm/sway/` — 1 skip
Integration test requires Sway compositor running.

### `internal/worktree/` — ~22 skips
All require `git` to be found. Same pattern as session worktree tests.

### `tools/genmcpdocs/` — 1 skip
Builder files not found for real AST integration test.

**Summary:** The largest skip cluster is `git not found` (37+ skips across session and worktree
packages). On a machine where git is available (including this Manjaro host), these would run.
The live e2e tests (2 skips) permanently require real API keys and are the only truly unreachable
test group in normal development.

---

## 7. E2E Harness Quality

### Harness Architecture

`internal/e2e/harness.go` wraps `session.Manager` with mock hooks for `LaunchOptions` and
`waitForSession`. It routes the first launch as the planner and subsequent launches as workers,
setting pre-determined `LastOutput`, `SpentUSD`, `TurnCount`, and optionally `StatusErrored` via
`MockFailure`. This allows `StepLoop` to execute its full path (planner parse → worker fan-out →
verify → noop detection) without spawning real processes.

### Scenario Catalog — 24 Scenarios Total

| Category | Count | Scenarios |
|----------|-------|-----------|
| Core (basic loop types) | 6 | TrivialFix, MultiFileRefactor, TestAddition, DocsUpdate, FeatureAddition, VerifyFailure |
| Multi-provider | 4 | GeminiWorkerBasic, CodexWorkerBasic, MultiProviderTeam, ProviderFailover |
| Stress/edge | 5 | BudgetExhaustion, TimeoutCascade, CircuitBreakerTrip, ConcurrentFileConflict, CheckpointRecovery |
| Cost tracking | 2 | CostTrackingAccuracy, FleetBudgetEnforcement |
| Self-learning | 5 | ReflexionRetry, CascadeEscalation, CascadeCheapSuccess, EpisodicInjection, CurriculumOrdering |
| Cross-subsystem integration | 3 | BanditCascadeIntegration, EpisodicWithEmbedder, FullSubsystemPipeline |

### Strengths

- **Subsystem wiring coverage:** The `ManagerSetup` hook pattern allows injecting real
  `CascadeRouter`, `EpisodicMemory`, `ReflexionStore`, `CurriculumSorter`, and `BanditHooks`
  objects into the manager before each scenario. `FullSubsystemPipeline` exercises all five
  simultaneously, which is the closest to a true integration test without live LLMs.
- **Failure path coverage:** Five scenarios explicitly test failure modes (`VerifyFailure`,
  `BudgetExhaustion`, `TimeoutCascade`, `CircuitBreakerTrip`, `ConcurrentFileConflict`) and
  assert `ExpectedStatus == "failed"`.
- **Constraint system:** Each scenario defines `Constraints{MaxCostUSD, MaxDurationSec,
  MinCompletionRate}` for regression gate validation.
- **Tag-based selection:** `ScenariosByTag()` enables targeted test runs (e.g., only `stress`
  scenarios).
- **Baseline + gate system:** `baseline.go` + `gates.go` form a complete P50/P95 regression
  detection system with `VerdictPass/Warn/Fail/Skip` verdicts. `RunE2EGate()` in `gates.go`
  is the autonomous test gate entry point, callable from the `ralphglasses_loop_gates` MCP tool.

### Weaknesses

- **No multi-worker fan-out test:** All scenarios set `MaxConcurrentWorkers: 1`. The goroutine
  fan-out in `StepLoop` (up to N parallel workers) is not exercised by any e2e scenario. Race
  conditions in the worker collection loop are only exercised by unit tests with explicit
  `t.Parallel()`.
- **No real git operations in most scenarios:** `setupRepo` creates a git repo and does an
  initial commit. But `WorkerBehavior` only writes files — it does not commit them. Scenarios
  that test `VerifyCommands` using `git tag -l` (CheckpointRecovery) work because the tag was
  pre-created in `RepoSetup`, not by the worker. The worktree commit flow is untested in e2e.
- **MockFailure is simplistic:** `MockFailure` sets the session to `StatusErrored` with a fixed
  error string. It does not simulate the actual error detection paths in the runner (e.g.,
  extra-usage exhaustion, startup probe failure, SIGTERM handling). These paths are tested in
  unit tests but not in the e2e harness.
- **Live fire scenarios are stubs:** `live_test.go` has two tests gated on `ANTHROPIC_API_KEY`
  with a `t.Skip("live-fire not yet wired to real providers")` inside — meaning even with an API
  key, the tests skip. The live e2e path is not yet implemented.
- **Provider failover scenario is partially mocked:** `ProviderFailover` simulates the secondary
  provider succeeding, but does not inject a `LaunchWithFailover` path or `FailoverChain` config.
  The mock just sets `WorkerBehavior` to the "successful" result without exercising the actual
  failover dispatch logic in `LaunchWithFailover`.
- **No fleet-level e2e:** All scenarios exercise a single-repo loop. Fleet coordination
  (multi-repo, coordinator dispatch, worker pool dequeue) has no e2e coverage.
- **`selftest.go` is infrastructure-only:** The `SelfTestRunner` uses `exec.CommandContext` to
  invoke the binary with `RALPH_SELF_TEST=1`. There is no test verifying that the binary
  actually handles this flag — it is exercised only via `internal/e2e/selftest_test.go`'s
  dry-run path.

---

## 8. Priority Fix List

Ranked by blast radius × severity of gap.

| # | Priority | Package | Gap | Specific Function/File | Effort |
|---|----------|---------|-----|----------------------|--------|
| 1 | **CRITICAL** | `internal/knowledge` | Fatal race: `TieredKnowledge.Query` concurrent map writes | `enhancer/knowledge/graph.go` — `TieredKnowledge` lacks mutex on `Query` path | Low — add `sync.RWMutex` to struct, wrap reads with `RLock` |
| 2 | **HIGH** | `internal/session` | `AutoRecovery.retryState` map has no mutex (Wave 3 finding) | `autorecovery.go:AutoRecovery` struct | Low — add `sync.Mutex`, wrap all `retryState` access |
| 3 | **HIGH** | `internal/mcpserver` | `TestHandleCircuitReset_Enhancer` fails: enhancer CB not initialized in test harness | `handler_circuit_test.go` + `test_helpers_test.go:setupTestServer` | Low — wire `srv.Enhancer` with a real `EnhancerClient` in test setup |
| 4 | **HIGH** | `internal/mcpserver` | `handler_sweep_report.go` (302 lines) has zero test coverage | `handler_sweep_report.go` — Markdown/JSON multi-section output | Medium — add `handler_sweep_report_test.go` with 5–8 tests covering empty/partial/full sweep data |
| 5 | **HIGH** | `internal/marathon` | `TestRestartPolicy_BackoffCap` fails: exponential overflow before cap applies | `restart.go:Backoff()` — `base * factor^count` overflows `int64` when count=20, factor=10 | Low — apply cap before exponentiation or use `math.Min` with float conversion |
| 6 | **MEDIUM** | `cmd/ralphglasses-mcp` | `TestToolGroupNames` expects 15 entries, actual is 16 (sweep added) | `cmd/ralphglasses-mcp/main_test.go:TestToolGroupNames` — stale expected count | Trivial — update `len(expected)` check to 16 or remove hardcoded count |
| 7 | **MEDIUM** | `internal/mcpserver` | `handler_prompt_ab.go` (200 lines), `handler_provider_benchmark.go` (268 lines) untested | Both files: file I/O + percentile math with no assertions | Medium — add test files for each; mock file writes for prompt_ab, use table tests for provider_benchmark percentile logic |
| 8 | **MEDIUM** | `internal/session` | 6 failing tests in runner/optimizer (WeeklyReport, TruncateStr, RunSessionOutput, RunCycle, ConfigOptimizer ×2) | `runner_test.go`, `weekly_report_test.go`, `config_optimizer_test.go`, `manager_cycle_test.go` | Low-Medium — boundary condition fixes in each; ConfigOptimizer likely needs mock input data update |
| 9 | **MEDIUM** | `internal/e2e` | No multi-worker fan-out scenario | `catalog.go` — all scenarios use `MaxConcurrentWorkers: 1` | Low — add one scenario with `ProfilePatch` setting workers=3 and a planner response with 3 tasks; assert all results collected |
| 10 | **LOW** | `internal/wm` | `wm` root at 52.4%, `wm/hyprland` at 58.5% | `wm.go` compositor dispatch, `hyprland/client.go` IPC parsing | Medium — add unit tests for IPC message parsing and compositor detection; Sway integration test already exists, need Hyprland equivalent |

### Secondary gaps not in top 10 but worth tracking:

- `handler_roadmap_prioritize.go` (178 lines, scoring algorithm) — no test
- `handler_session_fork.go` (50 lines, `SessMgr.Fork`) — no test
- `internal/flash` (32.3%) — Flash attention/memory primitive used in context window management
- `internal/plugin/builtin` (35.5%) — default plugin experience has thin test coverage
- `cmd` entrypoint (46.8%) — CLI flag parsing and subcommand dispatch lightly tested
- `cmd/prompt-improver` — all tests fail without `ANTHROPIC_API_KEY`; needs skip guards

---

## Appendix: Key File Paths

| File | Relevance |
|------|-----------|
| `~/hairglasses-studio/ralphglasses/internal/e2e/harness.go` | Mock harness for StepLoop e2e |
| `~/hairglasses-studio/ralphglasses/internal/e2e/catalog.go` | 6 core scenarios |
| `~/hairglasses-studio/ralphglasses/internal/e2e/catalog_cost.go` | 2 cost scenarios |
| `~/hairglasses-studio/ralphglasses/internal/e2e/catalog_provider.go` | 4 multi-provider scenarios |
| `~/hairglasses-studio/ralphglasses/internal/e2e/catalog_stress.go` | 5 stress scenarios |
| `~/hairglasses-studio/ralphglasses/internal/e2e/catalog_learning.go` | 8 self-learning + integration scenarios |
| `~/hairglasses-studio/ralphglasses/internal/e2e/gates.go` | Regression gate system (P50/P95) |
| `~/hairglasses-studio/ralphglasses/internal/e2e/baseline.go` | Baseline computation |
| `~/hairglasses-studio/ralphglasses/internal/mcpserver/handler_sweep_report.go` | 302-line untested handler (priority #4) |
| `~/hairglasses-studio/ralphglasses/internal/mcpserver/handler_circuit_test.go` | Failing circuit reset test (priority #3) |
| `~/hairglasses-studio/ralphglasses/internal/session/autorecovery.go` | Unprotected retryState map (priority #2) |
| `~/hairglasses-studio/ralphglasses/internal/marathon/restart_test.go` | Failing BackoffCap test (priority #5) |
| `~/hairglasses-studio/ralphglasses/Makefile` | CI pipeline: `test`, `test-cover`, `smoke`, `fuzz` targets |
