# ralphglasses Dead Code & Phantom Reference Audit

**Date:** 2026-04-04
**Scope:** `~/hairglasses-studio/ralphglasses` (read-only)
**Method:** grep + filesystem verification of all ROADMAP.md file references, orphan test detection, struct/interface usage analysis

---

## 1. Phantom References in ROADMAP.md

### 1a. Files Referenced in Completed Tasks That Do Not Exist

These are the most dangerous phantom references: tasks marked `[x]` (done) that point to files that were never created or were renamed without updating the ROADMAP.

| Task ID | Referenced File | Status | Notes |
|---------|----------------|--------|-------|
| QW-7 | `internal/session/snapshot.go` | MISSING | Task marked complete; snapshot logic lives in `internal/mcpserver/tools_session.go:186`. File was never extracted. |
| QW-11 | `internal/fleet/coordinator.go` | MISSING | Task marked complete; fleet server is `internal/fleet/server.go`. The "coordinator" concept was subsumed into `Coordinator` struct in `server.go`. |
| 0.5.7.1 | `internal/version/version.go` | MISSING (dir) | Task marked complete ("Define `var Version = \"dev\"`"). Version is defined in `cmd/prompt-improver/main.go` and the cobra root — never moved to a dedicated package. |
| 0.5.11.1 | `internal/model/config_schema.go` | MISSING | Task marked complete. Schema validation was not extracted to this file; canonical key list is likely inline. |
| 1.8.4 | `internal/errors/` (package) | MISSING (dir) | Task marked complete ("Create `internal/errors/` package with error classification"). Error codes are in `internal/mcpserver/errors.go` but the standalone package was never created. |

### 1b. Future Task References to Phantom Packages

These are open tasks (`[ ]`) — expected to not exist yet — but included for completeness. The entire package directory is missing.

| Phase | Package Dir | Files Referenced | Count |
|-------|-------------|-----------------|-------|
| Phase 14 | `internal/memory/` | store, retrieval, consolidation, embeddings, replay, trajectory, strategy, curriculum, competence, metacognition, confidence | 11 |
| Phase 15 | (fleet sub-files) | coordinator, bandit_router, batch, consensus, dag, decomposer, edge, edge_monitor, ensemble, gantt, mesh, moe_router, pareto, router_telemetry, scheduler, stigmergy, swarm, task_classifier, work_stealing | 19 |
| Phase 17 | `internal/integrations/` | github_app, slack, grafana | 3 |
| Phase 18 | `internal/predict/` | build, test_impact, flaky, duration, calibration, evolution, debt, hotspot, simulator, fleet_sim, cost_sim | 11 |
| Phase 19 | `internal/multirepo/` | registry, depgraph, coordinated_pr, intelligence, migration, deps, license, supply_chain, release, changelog, canary | 11 |
| Phase 20 | `internal/devops/` | ci_gen, ci_analyzer, ci_optimize, infra, docker, k8s_gen, profiler, perf_regression, benchmark_tracker, sast, vuln_scan, sbom, docs_gen, arch_diagram | 14 |
| Phase 20 | `internal/profiler/` | pprof, pyroscope, pgo | 3 |
| Phase 22 | `internal/rag/` | hybrid, colbert, raptor, graphrag | 4 |
| Phase 22 | `internal/enhancer/` (partial) | compression, distillation, prompt_versioning, retrieval_gate | 4 |
| Phase 25 | `internal/fedlearn/` | aggregator, dp, topology, secure_agg, lora, personalization, prompt_tuning, distill, fed_bandit | 9 |

### 1c. Missing Files in Existing Packages (Mixed Completed/Open Tasks)

| Task Status | File | Phase / Task | Notes |
|-------------|------|-------------|-------|
| OPEN | `internal/mcpserver/tools_loop.go` | Future sprint | Server struct is in `tools.go`; loop tool handlers are in `handler_loop.go`. |
| OPEN | `internal/mcpserver/server.go` | Phase perf sprint | `Server` struct defined in `tools.go:42`, not a dedicated `server.go`. |
| OPEN | `internal/bandit/selector.go` | Future cascade phase | Bandit logic lives in `bandit.go`, `ucb.go`, `thompson.go`, etc. No file named `selector.go`. |
| OPEN | `internal/fleet/a2a_federation.go` | Future | Exists as concept in `a2a.go` / `a2a_dispatch.go`. |
| OPEN | `internal/session/snapshot.go` | QW-7 (completed differently) | See 1a above. |
| OPEN | `internal/session/merge.go` | Future sprint | Cycle merge logic TBD. |
| OPEN | `internal/session/cycle_plan.go` | Future sprint | |
| OPEN | `internal/session/scheduler.go` | Future sprint | |
| OPEN | `internal/session/baseline.go` | Future sprint | |
| OPEN | `internal/eval/` (many files) | Future eval phase | Package dir exists but 9 specific files are missing: benchmark, comparison, dashboard, harness, leaderboard, pass_k, quality, slo, task_class, test_grader. |
| OPEN | `internal/plugin/` (many files) | Future plugin phases | sandbox, sdk, template_marketplace, templates, tool_compose, tool_registry, wasm_capabilities, wasm_host, wasm_runtime are missing. |
| OPEN | `internal/sandbox/incus/` | Phase 5.2 | Package exists under `internal/sandbox/` but `incus/` subdir is missing. |
| OPEN | `internal/tui/views/profile_view.go` | Future profiling phase | |
| OPEN | `internal/telemetry/` (partial) | Future | Package exists (remote.go, spantree.go, telemetry.go) but `cost_attribution.go`, `llm_spans.go`, `otel.go` are missing. |
| OPEN | `internal/safety/` (many files) | Future safety phase | Package exists (anomaly, circuit_breaker, killswitch) but constitution, guardrails, allowlist, audit, chaos, injection_test, lineage, model_card, process_reward, redteam, reward_model, sanitizer, self_critique are all missing. |

**Total phantom file references in ROADMAP.md: 158** (verified by filesystem check)
**Of which are in completed tasks: 5** (highest priority to reconcile)
**Of which are in open future tasks: 153** (expected; phases 14-25 are not yet built)

---

## 2. Dead Code Inventory

| File | Type | Name | Lines | Can Delete? | Notes |
|------|------|------|-------|-------------|-------|
| `internal/tui/views/fleet_dashboard.go` | Unused struct + methods | `FleetDashboardModel` | 207 | Yes | Never instantiated outside file. `FleetView` in `fleet.go` is the live implementation used in `app.go:52`. Dead duplicate. |
| `internal/tui/views/fleet_dashboard_test.go` | Orphan test | Tests for `FleetDashboardModel` | 236 | Yes (with source) | Tests exercise dead code only. |
| `internal/tui/components/modal.go` | Unused struct | `ModalStack` | ~75 | No (yet) | Defined and tested but never instantiated in production code (no import outside own package and test). Low-risk to keep — small file. Flag for removal if search view is never built. |
| `internal/tui/app_init.go:47` | Unreachable iota value | `ViewSearch` | 1 | Caution | Defined in `ViewMode` iota but `viewDispatch` map and `viewBindings` map have no entry for it. Switching to `ViewSearch` via `pushView` would hit the default/no-op branch. Can be removed if search view is not planned soon. |
| `internal/mcpserver/tools_fleet.go` | Convention violation | `handleEventList`, `handleFleetAnalytics`, `handleMarathonDashboard`, `handleToolBenchmark` | 530 | No (move) | Handlers defined outside `handler_*.go` convention. Should be split to `handler_fleet.go` / `handler_marathon.go`. Not dead, just misplaced. |
| `internal/mcpserver/tools_session.go` | Convention violation | `handleWorkflowDefine`, `handleWorkflowRun`, `handleWorkflowDelete`, `handleSnapshot`, `handleJournalRead`, `handleJournalWrite`, `handleJournalPrune` | 434 | No (move) | Same naming convention issue — these are live handlers in the wrong file type. Should move to `handler_session.go` / `handler_journal.go`. |

### Duplicate Type Definitions (Copy-Paste Risk)

These struct names appear in multiple packages with different implementations. Not dead code, but potential maintenance burden.

| Type Name | Packages Defining It | Risk |
|-----------|---------------------|------|
| `CircuitBreaker` | `enhancer/circuit.go`, `gateway/circuit.go`, `process/circuit_breaker.go`, `safety/circuit_breaker.go` | Medium — 4 independent implementations. Consolidation into `safety/` or shared lib would reduce drift. |
| `BudgetPool` | `fleet/pool/budget_pool.go`, `session/budget_pool.go` | Medium — same concept in fleet and session layers. |
| `Blackboard` | `blackboard/blackboard.go`, `session/blackboard.go` | Medium — dedicated package exists; session copy may be a migration artifact. |
| `Coordinator` | `fleet/server.go`, `session/coordination.go` | Low — different semantics (fleet vs session coordination), but confusing naming. |

---

## 3. Stale Comments (TODO/FIXME/HACK/XXX)

The codebase is remarkably clean on stale comments — only 7 non-test instances found across the entire `internal/` tree.

| Package | Count | Examples |
|---------|-------|---------|
| `mcpserver` | 4 | In `handler_rdcycle.go:941-952`: grep logic that searches for `TODO/FIXME/HACK` in diffs as part of the diff review check. These are **intentional** — they are regex patterns, not actual stale comments. In `tools_builders_misc.go:387`: tool description mentions TODOs. |
| `review` | 2 | In `criteria.go:151-152`: lint rule definition for TODO-without-issue pattern. Also intentional — this is the rule engine. |
| `e2e` | 1 | In `catalog_learning.go:328`: `"// TODO: add helpers"` embedded in a string literal (test fixture content). Intentional. |

**Verdict:** Zero genuine stale TODOs in production code. The 7 occurrences are all intentional — two are regex patterns in code review tools, two are lint rule definitions, and one is inside a string literal for test fixture generation.

---

## 4. Orphaned Test Files

Test files whose base name has no corresponding source file in the same directory. Total: **187 orphaned test files**.

These are almost universally "coverage boost" and "sprint" tests that test exported functions across the package without having a dedicated source file — a common pattern used to increase coverage without naming tests after a specific implementation file.

### Count by Package (Top 12)

| Package | Orphan Test Count | Pattern |
|---------|-------------------|---------|
| `session` | 56 | `coverage_boost_test.go`, `sprint_planner_coverage_test.go`, `loop_planner_fuzz_test.go`, etc. |
| `mcpserver` | 20 | `tools_core_test.go`, `tools_race_test.go`, `handler_coverage_boost_test.go`, etc. |
| `fleet` | 17 | `dlq_test.go`, `coverage_boost3_test.go`, `sprint7_fleet_test.go`, etc. |
| `tui/views` | 11 | `fleet_helpers_test.go`, `percentile_test.go`, `coverage_boost_test.go`, etc. |
| `enhancer` | 9 | `bench_test.go`, `edge_test.go`, `golden_test.go`, etc. |
| `process` | 8 | `killsequence_test.go`, `errorpaths_test.go`, `manager_edge_test.go`, etc. |
| `model` | 7 | `corrupt_test.go`, `config_fuzz_test.go`, `bench_test.go`, etc. |
| `e2e` | 6 | `e2e_test.go`, `live_test.go`, `platform_test.go`, etc. |
| `discovery` | 4 | `discovery_test.go`, `bench_test.go`, `scanner_errorpaths_test.go`, etc. |
| `tracing` | 3 | `context_test.go`, `tracing_coverage_test.go`, `tracing_extra_test.go` |
| `tui` | 3 | `aliases_path_coverage_test.go`, `app_teatest_test.go`, `command_history_coverage_test.go` |
| `tui/components` | 3 | `modal_coverage_test.go`, `mouse_test.go`, `table_fuzz_test.go` |

**Notable true orphans** (where the source file clearly does not exist anywhere in the package):

- `internal/bandit/selectrandom_test.go` — tests a `SelectRandom` function; no `selectrandom.go` or equivalent.
- `internal/fleet/dlq_test.go` — tests a DLQ (dead-letter-queue); no `dlq.go` in fleet package (though DLQ may be embedded in `queue.go`).
- `internal/cloud/cloud_test.go` — the `cloud` package has a test but no production source file at all.
- `internal/tui/views/fleet_helpers_test.go` — tests fleet helpers that may be inlined in `fleet.go`.
- `internal/session/wiring_test.go`, `recording_wiring_test.go` — no corresponding wiring source files.

**Total lines in orphaned test files: ~39,436**

Most of these are intentional coverage-boosting tests and should not be deleted without running `go test ./...` to confirm they still compile and pass.

---

## 5. Duplicate Code

### High-Priority Duplicates

**1. `CircuitBreaker` — 4 independent implementations**
- `internal/enhancer/circuit.go` — HTTP-call circuit breaker for LLM enhancer
- `internal/gateway/circuit.go` — gateway-level circuit breaker
- `internal/process/circuit_breaker.go` — process manager circuit breaker
- `internal/safety/circuit_breaker.go` — general-purpose safety circuit breaker

Each implementation is functionally similar (state machine: closed/open/half-open, threshold-based tripping) but has different fields and thresholds. The `safety/` package's `CircuitBreaker` is the most general-purpose. Consolidating would eliminate ~200-300 duplicate lines.

**2. `Blackboard` — package vs session copy**
- `internal/blackboard/blackboard.go` — standalone CAS-based key-value store (dedicated package)
- `internal/session/blackboard.go` — second implementation inside session package

The session copy appears to be a migration artifact from before `internal/blackboard/` was created. They implement similar CAS semantics. The session one should be removed in favor of importing from `internal/blackboard`.

**3. `BudgetPool` — fleet vs session**
- `internal/fleet/pool/budget_pool.go`
- `internal/session/budget_pool.go`

Both implement a shared token budget pool for rate limiting. The fleet version may have distributed concerns; the session version is local. Worth reviewing whether one can import the other.

**4. `tools_fleet.go` / `tools_session.go` Handler Convention Violation**

11 handlers defined outside the `handler_*.go` naming convention:
- `tools_fleet.go`: `handleEventList`, `handleFleetAnalytics`, `aggregateObservationMetrics`, `obsPercentile`, `handleMarathonDashboard`, `handleToolBenchmark`
- `tools_session.go`: `resolveSnapshotRepo`, `handleWorkflowDefine`, `handleWorkflowRun`, `handleWorkflowDelete`, `handleSnapshot`, `handleJournalRead`, `handleJournalWrite`, `handleJournalPrune`

These are live handlers mixed into tool-registration files. They should be extracted to dedicated `handler_*.go` files for consistency with the rest of the mcpserver package (which has `handler_fleet.go`, `handler_session.go`, etc.).

---

## 6. Cleanup Effort Estimate

| Category | Files | Lines | Risk |
|----------|-------|-------|------|
| `fleet_dashboard.go` + its test | 2 | ~443 | Low — confirmed dead, not referenced anywhere outside itself |
| ROADMAP phantom reference reconciliation (completed tasks) | 0 files to delete, 1 ROADMAP update | ~10 | Zero — editing doc only |
| Extract handlers from `tools_fleet.go` / `tools_session.go` | 2 source → 4 files | ~964 | Low — refactor only, no logic change |
| Remove `ViewSearch` from `ViewMode` iota | 1 line | 1 | Low if search view not in near-term plan |
| `session/blackboard.go` dedup (if confirmed redundant) | 1 | ~150 | Medium — need to verify all call sites use the right package |
| Consolidate `CircuitBreaker` implementations | 4 → 1 | ~300-400 | High — each has different API; needs careful interface extraction |
| Orphaned test files (safe to remove only if source confirmed absent) | Up to 187 | ~39,436 | Low-medium — most compile fine as cross-package tests; run `go test ./...` before deleting any |

**Maximum cleanable (aggressive):** ~42,000 lines across ~200 files  
**Safe immediate cleanup:** ~500 lines across ~3 files (fleet_dashboard + ROADMAP reconciliation)

---

## 7. Prioritized Cleanup List

### Priority 1 — Immediate, Zero Risk

1. **Delete `internal/tui/views/fleet_dashboard.go` and `fleet_dashboard_test.go`** (443 lines)
   - `FleetDashboardModel` is confirmed unreachable from any production code
   - `FleetView` in `fleet.go` is the live implementation used by `app.go`
   - This is clean, isolated deletion with no import graph impact

2. **Reconcile 5 completed-task phantom references in ROADMAP.md**
   - QW-7: Update note to reflect snapshot lives in `tools_session.go:186`, not a dedicated `snapshot.go`
   - QW-11: Update note to reflect fleet coordinator is `fleet/server.go:Coordinator`, not `coordinator.go`
   - 0.5.7.1: Note version lives in cobra root / `cmd/` not `internal/version/version.go`
   - 0.5.11.1: Note config schema validation is inline in `model/config.go`, not extracted
   - 1.8.4: Note error codes are in `mcpserver/errors.go`, the `internal/errors/` package was never created

### Priority 2 — Low Risk, High Hygiene

3. **Move handlers out of `tools_fleet.go` and `tools_session.go`** (~964 lines to reorganize)
   - Create/extend `handler_marathon.go` for marathon dashboard + tool benchmark
   - Extend `handler_fleet.go` for event list and fleet analytics
   - Extend `handler_session.go` for workflow, snapshot, journal handlers
   - This restores the `handler_*.go` convention across all 11 violating functions

4. **Remove `ViewSearch` from `ViewMode` iota** (1 line, `app_init.go:47`)
   - No handler registered, no key binding triggers it
   - Causes confusion about whether search view exists

### Priority 3 — Medium Effort, Good Payoff

5. **Audit `session/blackboard.go` vs `blackboard/blackboard.go`**
   - If session package is importing from `blackboard/`, the `session/blackboard.go` is dead
   - If session is using its local copy, migrate to the canonical package and delete the copy
   - Saves ~150 lines and prevents semantic drift

6. **`internal/cloud/` — verify or delete**
   - `cloud_test.go` exists with no production source at all
   - Either the package was deleted but the test survived, or the test was added prematurely
   - `go build ./...` will catch this if it's a real problem

### Priority 4 — Long-Term Architectural

7. **Consolidate `CircuitBreaker` implementations** (4 → 1)
   - Extract a common interface in `safety/circuit_breaker.go`
   - Have `enhancer/`, `gateway/`, `process/` import from `safety/` or a new shared lib
   - ~300 lines removed across packages, more importantly: single source of truth for threshold tuning

8. **Address orphaned `*_coverage_test.go` files systematically**
   - These are auto-generated by sweep agents to boost coverage
   - Many test internal functions via the same package name (valid Go)
   - Run `go test ./... -count=1` to confirm all 187 still compile before declaring any orphaned
   - Over time, consolidate into proper `_test.go` files named after their actual subject

---

## Appendix: Key File Locations

- Dead code: `/home/hg/hairglasses-studio/ralphglasses/internal/tui/views/fleet_dashboard.go`
- Convention violations: `/home/hg/hairglasses-studio/ralphglasses/internal/mcpserver/tools_fleet.go`, `tools_session.go`
- Unreachable ViewMode: `/home/hg/hairglasses-studio/ralphglasses/internal/tui/app_init.go:47`
- Unused ModalStack: `/home/hg/hairglasses-studio/ralphglasses/internal/tui/components/modal.go`
- Duplicate Blackboard: `/home/hg/hairglasses-studio/ralphglasses/internal/session/blackboard.go` (vs `internal/blackboard/blackboard.go`)
- Duplicate BudgetPool: `/home/hg/hairglasses-studio/ralphglasses/internal/session/budget_pool.go` (vs `internal/fleet/pool/budget_pool.go`)
