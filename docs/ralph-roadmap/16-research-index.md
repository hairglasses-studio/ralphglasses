# 16 -- Research Sweep Master Index

Generated: 2026-04-04
Sources: 16 agent reports (01-12, s1-s4) + meta-roadmap/08-ralph-deep-dive.md

This document is the master lookup table for all findings from the ralphglasses research sweep.

---

## 1. Finding Index

### CRITICAL

| ID | Source | Summary | Affected Packages/Phases | Cross-Refs |
|----|--------|---------|--------------------------|------------|
| F-001 | s1-race-condition-census (R-01) | `AutoRecovery.retryState` map accessed from concurrent goroutines with no mutex -- fatal concurrent map write | `internal/session/autorecovery.go` / Phase 10.5 | F-002, F-028 |
| F-002 | s1-race-condition-census (R-02) | `RetryTracker.attempts` map in fleet package has no lock; HTTP handlers race on it | `internal/fleet/retry.go`, `server_handlers.go` / Phase 10.5 | F-001, F-015 |
| F-003 | 01-roadmap-matrix (Section 3.1) | Phase 3.5.5 numbering collision -- two sections share identical sub-IDs for completely different tasks (Codex parity vs theme export) | ROADMAP.md / Phase 3.5 | F-039 |
| F-004 | 01-roadmap-matrix (Finding B) | `tools_loop_test.go` exists without `tools_loop.go` -- Phase 9 tier-1 tools have test expectations but no implementations | `internal/mcpserver/` / Phase 9 | F-005, F-037 |
| F-005 | 01-roadmap-matrix (Section 6.3) | Phase 9 marked 100% complete but tier-1 tool files (`merge.go`, `cycle_plan.go`, `scheduler.go`, `baseline.go`) do not exist on disk | `internal/session/` / Phase 9 | F-004, F-037 |
| F-006 | 08-ralph-deep-dive (Sprint 7) | 5 BLOCKER path traversal vulnerabilities in MCP handlers -- `scratchpadName`, `name`, `worktree_paths`, `hooks.yaml command`, `verify_commands` bypass `ValidatePath` | `internal/mcpserver/` / Phase 1 | F-029 |

### HIGH

| ID | Source | Summary | Affected Packages/Phases | Cross-Refs |
|----|--------|---------|--------------------------|------------|
| F-007 | s1-race-condition-census (R-03) | `GateEnabled` and `RunTestGate` are unprotected package-level vars written by supervisor, read concurrently | `internal/session/autooptimize.go` / Phase 9.5 | F-001 |
| F-008 | s1-race-condition-census (R-04) | `OpenAIClient.LastResponseID` written and read without mutex in concurrent `Improve()` calls | `internal/enhancer/openai_client.go` / Phase 23 | F-013 |
| F-009 | s1-race-condition-census (R-05) | `GetTeam` acquires `sessionsMu` then `workersMu` -- inconsistent lock ordering is deadlock-prone | `internal/session/manager_team.go` / Phase 10.5 | F-001 |
| F-010 | s1-race-condition-census (R-06) | `Server.loadedGroups` map accessed via concurrent MCP tool calls without `s.mu` protection | `internal/mcpserver/tools.go`, `tools_dispatch.go` / Phase 1 | F-006 |
| F-011 | 06-fleet-sweep (Risk 1) | Unbounded in-memory `WorkQueue` -- no capacity limit; can grow to OOM under heavy sweep load | `internal/fleet/queue.go` / Phase 10.5 | F-015 |
| F-012 | 06-fleet-sweep (Risk 2) | Fleet coordinator queue is in-memory only; restart loses all pending/assigned work and orphans in-flight results | `internal/fleet/` / Phase 10.5 | F-011 |
| F-013 | 04-enhancer-pipeline (Section 5) | Circuit breaker shared across all 3 LLM providers -- one flaky provider trips the CB for all | `internal/enhancer/circuit.go`, `hybrid.go` / Phase 23 | F-008, F-043 |
| F-014 | 06-fleet-sweep (Risk 4) | Autoscaler scale-up is advisory only -- publishes event but no actuator spawns new workers | `internal/fleet/autoscaler.go` / Phase 10.5 | F-011 |
| F-015 | 06-fleet-sweep (Risk 3) | `RetryTracker` has no mutex -- same as R-02 but found independently in fleet audit | `internal/fleet/retry.go` / Phase 10.5 | F-002 |
| F-016 | s4-error-handling (Swallowed #1) | `_ = s.SessMgr.RunLoop(context.Background(), run.ID)` -- self-improvement loop error silently discarded | `internal/mcpserver/handler_selfimprove.go` / Phase 8 | F-017 |
| F-017 | s4-error-handling (Silent #1) | Supervisor silently swallows `RunCycle` failures at Warn level -- at L2/L3, cycles fail invisibly | `internal/session/supervisor.go:375` / Phase 9.5, 13 | F-016, F-018 |
| F-018 | s4-error-handling (Silent #2-3) | Chained cycle and sprint-planner failures also warn-logged and swallowed in supervisor tick | `internal/session/supervisor.go:231,244` / Phase 9.5, 13 | F-017 |
| F-019 | s4-error-handling (Swallowed #2) | `_ = cmd.Run()` in hooks -- hook command exit codes entirely discarded; safety gates invisible | `internal/hooks/hooks.go:139` / Phase 1 | F-006 |
| F-020 | s3-cost-model (Gap A) | `MaxBudgetUSD == 0` path allows sessions to launch with no spend cap when `DefaultBudgetUSD` is also zero | `internal/session/manager_lifecycle.go` / Phase 10.5 | F-021 |
| F-021 | s3-cost-model (Gap B) | Active sessions are not stopped when `GlobalBudget` is exhausted; overspend silently accepted | `internal/fleet/` / Phase 10.5 | F-020, F-022 |
| F-022 | s3-cost-model (Gap D) | Sweep `handleSweepSchedule` cost cap is reactive (5-min polling); overspend within polling interval | `internal/mcpserver/handler_sweep.go` / Phase 10.5 | F-021 |
| F-023 | 05-test-coverage | `internal/knowledge` fatal concurrent map writes in `TieredKnowledge.Query` -- active race condition | `internal/enhancer/knowledge/` / Phase 0.6 | F-001 |
| F-024 | 03-session-architecture (Risk 1) | `AutoRecovery.retryState` lacks mutex -- confirmed independently from session architecture review | `internal/session/autorecovery.go` / Phase 10.5 | F-001 |

### MEDIUM

| ID | Source | Summary | Affected Packages/Phases | Cross-Refs |
|----|--------|---------|--------------------------|------------|
| F-025 | 02-mcp-tool-audit (Section 5) | Deferred loading not activated in production binary -- all 166 tools loaded eagerly despite documented deferred model | `cmd/mcp.go`, `internal/mcpserver/tools.go` / Phase 1.5 | F-039 |
| F-026 | 02-mcp-tool-audit (Section 7) | `advanced` namespace too broad (24 tools, 7+ domains) -- should be split into rc, autonomy, workflow | `internal/mcpserver/tools_builders_misc.go` / Phase 1.5 | -- |
| F-027 | 02-mcp-tool-audit (Section 3) | 8 handler files without dedicated test files; `handler_sweep_report.go` (302 lines) is highest risk | `internal/mcpserver/` / Phase 1 | F-033 |
| F-028 | s1-race-condition-census (R-07) | Supervisor `tick()` launches goroutines for `RunCycle` with no WaitGroup or context propagation from `done` channel | `internal/session/supervisor.go` / Phase 9.5 | F-017 |
| F-029 | 02-mcp-tool-audit (Section 5.3) | `ValidationMiddleware` only validates `repo` and `path` params; free-text fields (`prompt` up to 200KB) unchecked | `internal/mcpserver/middleware.go` / Phase 1 | F-006 |
| F-030 | s1-race-condition-census (R-08) | `hitCount[key]++` in `TieredKnowledge` executed under `RLock` -- concurrent writers corrupt the map | `internal/knowledge/tiered_knowledge.go` / Phase 0.6 | F-023 |
| F-031 | s1-race-condition-census (R-13) | Shared `CircuitBreaker` across all 3 enhancer providers; one provider failure blocks all | `internal/enhancer/hybrid.go` / Phase 23 | F-013 |
| F-032 | s3-cost-model (Section 3) | Gemini Flash output rate understated by ~40% ($2.50 compiled-in vs ~$3.50 actual for 2.5 Flash) | `internal/config/costs.go` / Phase 24 | F-041 |
| F-033 | 05-test-coverage | 7 packages actively failing tests; `internal/session` has 6 failing tests including critical knowledge race | `internal/session`, `internal/mcpserver`, `internal/knowledge` / Phase 1 | F-023, F-027 |
| F-034 | 05-test-coverage | 80 coverage-padding files (807 Test* functions); ~6% are genuine call-and-ignore padding | All packages / Phase 1 | -- |
| F-035 | 07-tui-audit (Critical #1) | LogView unbounded line growth -- long-running sessions with verbose output exhaust memory | `internal/tui/views/` / Phase 1.5 | F-036 |
| F-036 | 07-tui-audit (Critical #2) | LoopRun.Iterations O(n) computed per 2s tick via `SnapshotLoopControl` -- performance degrades with 500+ iterations | `internal/tui/` / Phase 1.5 | F-035 |
| F-037 | s2-dead-code-audit (Section 1a) | 5 completed tasks reference files that do not exist (QW-7, QW-11, 0.5.7.1, 0.5.11.1, 1.8.4) | ROADMAP.md / Phase 0-1 | F-004, F-005 |
| F-038 | 06-fleet-sweep (Risk 5) | Worker `executeWork` polls session status every 2s with no upper-bound timeout; stuck sessions consume worker slots forever | `internal/fleet/worker.go` / Phase 10.5 | F-014 |
| F-039 | 01-roadmap-matrix (Finding E) | ROADMAP.md header says "1,115 tasks, 442 complete" but live count is 1,143 / 503 -- metrics are stale | ROADMAP.md / All phases | F-003 |
| F-040 | 04-enhancer-pipeline (Section 2) | Default target provider resolves to OpenAI (markdown structure), not Claude (XML) -- potentially surprising default | `internal/enhancer/config.go` / Phase 23 | -- |
| F-041 | s3-cost-model (Section 3) | Opus rate in compiled-in costs is $15/$75 (pre-4.6 pricing) while actual Opus 4.6 is $5/$25 -- 67% overestimate | `internal/config/costs.go` / Phase 24 | F-032 |
| F-042 | s3-cost-model (Section 6) | Fleet `CostPredictor` instantiated but not auto-wired from `handleWorkComplete`; starts empty on every restart | `internal/fleet/costpredict.go` / Phase 10.5 | F-020 |
| F-043 | s2-dead-code-audit (Section 5) | 4 independent `CircuitBreaker` implementations across enhancer, gateway, process, safety packages | Multiple packages / Phase 1 | F-013, F-031 |
| F-044 | 03-session-architecture (Section 7.6) | `GetLoop`/`ListLoops` use write lock (`Lock()`) instead of read lock (`RLock()`) -- serializes all concurrent readers | `internal/session/loop.go` / Phase 10.5 | -- |
| F-045 | s4-error-handling (Swallowed #9-10) | Fleet worker `CompleteWork` and `Heartbeat` errors silently discarded; coordinator retains items indefinitely | `internal/fleet/worker.go` / Phase 10.5 | F-012 |
| F-046 | 08-distro-audit (Section 2) | hw-detect.sh PCI IDs hardcoded for single RTX 4090; cannot detect dual GPU or enumerate multiple cards | `distro/scripts/hw-detect.sh` / Phase 4 | -- |
| F-047 | s1-race-condition-census (R-11, R-12) | `cancel` field in both `AnomalyDetector` and `FleetAnomalyDetector` written/read without lock | `internal/safety/anomaly.go`, `anomaly_fleet.go` / Phase 17 | -- |
| F-048 | 03-session-architecture (Section 7) | JSON format enforcement is the system's top failure mode -- 25.7% retry rate vs target <5% | `internal/session/loop_steps.go` / Phase 6 | -- |
| F-049 | s4-error-handling (Silent #4) | Autonomy level persist failure is Warn-logged; L2/L3 resets to default on restart | `internal/session/autonomy.go` / Phase 13 | F-017 |

### LOW

| ID | Source | Summary | Affected Packages/Phases | Cross-Refs |
|----|--------|---------|--------------------------|------------|
| F-050 | 02-mcp-tool-audit (Section 7) | `handleLoadToolGroup` description string missing 3 namespaces (rdcycle, plugin, sweep) | `internal/mcpserver/tools_dispatch.go` / Phase 1 | F-025 |
| F-051 | 02-mcp-tool-audit (Section 4) | Zero tools have all 4 MCP spec annotations simultaneously; 83% have only 1 hint | `internal/mcpserver/annotations.go` / Phase 1 | -- |
| F-052 | 02-mcp-tool-audit (Section 6) | 11 handler functions defined in non-`handler_*.go` files (tools_fleet.go, tools_session.go) | `internal/mcpserver/` / Phase 1 | F-043 |
| F-053 | 02-mcp-tool-audit (Section 7) | `worktree_create/cleanup` misplaced in `observability` namespace; `loop_await/poll` also misplaced | `internal/mcpserver/` / Phase 1.5 | F-026 |
| F-054 | s2-dead-code-audit (Section 2) | `FleetDashboardModel` in `fleet_dashboard.go` is dead code (207 lines + 236 line test) -- never instantiated | `internal/tui/views/fleet_dashboard.go` / Phase 1.5 | -- |
| F-055 | s2-dead-code-audit (Section 2) | `ModalStack` defined but never instantiated; root model uses concrete `ModalState` struct instead | `internal/tui/components/modal.go` / Phase 1.5 | -- |
| F-056 | s2-dead-code-audit (Section 2) | `ViewSearch` iota value defined but has no handler in `handleKey` switch and no `viewDispatch` entry | `internal/tui/app_init.go` / Phase 1.5 | -- |
| F-057 | 07-tui-audit (Section 8) | No mouse scroll support -- wheel events not dispatched; viewports scroll only via keyboard | `internal/tui/handlers_mouse.go` / Phase 1.5 | -- |
| F-058 | 07-tui-audit (Section 2) | `RefreshAllRepos` is synchronous on the 2s UI tick -- 69+ repos cause latency spikes | `internal/tui/` / Phase 1.5 | F-035 |
| F-059 | 04-enhancer-pipeline (Section 4) | Rule 14 (`cache-no-structure`) conflicts with default pipeline: flags markdown-structured prompts targeting OpenAI | `internal/enhancer/context.go` / Phase 23 | F-040 |
| F-060 | 04-enhancer-pipeline (Section 5) | Gemini `CreateCachedContent()` never called automatically; prompt caching is opt-in at operator level | `internal/enhancer/gemini_client.go` / Phase 23 | -- |
| F-061 | 04-enhancer-pipeline (Section 7) | `PromptCache` eviction is O(n) map scan vs `EnhancerCache` O(1) LRU -- latent performance gap | `internal/enhancer/cache.go` / Phase 23 | -- |
| F-062 | 01-roadmap-matrix (Section 3.2) | Phase 6 and Phase 15 both named "Advanced Fleet Intelligence" -- confusing duplicate names | ROADMAP.md / Phase 6, 15 | -- |
| F-063 | 08-distro-audit (Section 7) | Tailscale auth key stored as plain file at `/etc/ralphglasses/ts-authkey` before enrollment | `distro/scripts/ts-enroll.sh` / Phase 4, 12 | -- |
| F-064 | s3-cost-model (Section 5) | Sweep auto-sizes per-session budget to `estimatedPerSession * 1.5` even if caller passed lower cap | `internal/mcpserver/handler_sweep.go` / Phase 10.5 | F-022 |
| F-065 | s2-dead-code-audit (Section 1) | 158 phantom file references in ROADMAP.md (5 in completed tasks, 153 in open future tasks) | ROADMAP.md / Phases 0-25 | F-037 |
| F-066 | s4-error-handling (Section 4) | `mcpserver` package has worst `%w` error wrapping ratio at 25% | `internal/mcpserver/` / Phase 1 | -- |

### INFO

| ID | Source | Summary | Affected Packages/Phases | Cross-Refs |
|----|--------|---------|--------------------------|------------|
| F-067 | 05-test-coverage | Overall reported coverage is 84.5%; 745 test files, 9,267 Test* functions | All packages | -- |
| F-068 | s2-dead-code-audit | Zero genuine stale TODOs in production code; all 7 occurrences are intentional patterns | All packages | -- |
| F-069 | s2-dead-code-audit | 187 orphaned test files (~39,436 lines) predominantly coverage-boost tests with meaningful assertions | All packages | F-034 |
| F-070 | 09-orchestration-landscape | ralphglasses occupies unique position: only Go-native multi-provider orchestrator with MCP, TUI, fleet, cascade routing | Architecture assessment | -- |
| F-071 | 10-mcp-ecosystem | MCP spec at v2025-11-25 with Tasks primitive, Extensions framework, OAuth Client ID Metadata | Protocol tracking | -- |
| F-072 | 11-llm-capabilities | Gemini 3.1 Pro best cost/performance on SWE-bench Verified (80.6% at $2.00 vs Opus 80.8% at $5.00) | Cost optimization / Phase 24 | F-032 |
| F-073 | 11-llm-capabilities | Opus 4.6 price dropped 67% ($15/$75 to $5/$25); only frontier model with flat pricing across 1M context | Cost optimization / Phase 24 | F-041 |
| F-074 | 12-thin-client-patterns | NixOS recommended for medium-term fleet deployment; Manjaro retained short-term for Phase 4 completion | `distro/` / Phase 4-5 | -- |
| F-075 | 12-thin-client-patterns | Cage compositor recommended for fleet worker thin clients (single maximized application) | `distro/` / Phase 4 | -- |
| F-076 | 12-thin-client-patterns | systemd-creds + TPM2 recommended for secrets management on thin clients | `distro/` / Phase 4 | F-063 |
| F-077 | s2-dead-code-audit | Maximum cleanable code: ~42,000 lines across ~200 files; safe immediate: ~500 lines across 3 files | All packages | -- |
| F-078 | s4-error-handling | Zero handler contract violations; all 44 MCP handlers return `(*CallToolResult, nil)` correctly | `internal/mcpserver/` | -- |
| F-079 | s4-error-handling | Only 2 production panics -- both justified (`BlendedCostPer1MTok` range check, `MustNew` constructor) | `internal/model/`, `internal/safety/` | -- |
| F-080 | 03-session-architecture | Lock hierarchy is sound -- three-mutex Manager model consistently applied with no cycles observed | `internal/session/` | -- |
| F-081 | 03-session-architecture | Bootstrap clamps autonomy to L1 -- `.ralphrc` cannot activate L2/L3; correct safety design | `internal/session/` / Phase 13 | -- |
| F-082 | 01-roadmap-matrix | Critical path to L3 autonomy: ~100 tasks across 6 unstarted phases, all L or XL sized | ROADMAP.md / Phase 13 | -- |

---

## 2. Key Metrics Summary

| Metric | Value | Source |
|--------|-------|--------|
| **Roadmap total tasks** | 1,143 (live grep count) | 01-roadmap-matrix |
| **Roadmap done tasks** | 503 | 01-roadmap-matrix |
| **Roadmap completion %** | 44.0% | 01-roadmap-matrix |
| **Phases complete (100%)** | 10 | 01-roadmap-matrix |
| **Phases in progress** | 10 (6 per 01, 10 per 08) | 01-roadmap-matrix, 08-deep-dive |
| **Phases fully planned (0%)** | 10-11 | 01-roadmap-matrix |
| **MCP tool count (actual)** | 166 across 16 namespaces | 02-mcp-tool-audit |
| **MCP annotation completeness (all 4 hints)** | 0 tools (0%) | 02-mcp-tool-audit |
| **MCP annotation completeness (2 hints)** | 26 tools (16%) | 02-mcp-tool-audit |
| **MCP annotation completeness (1 hint)** | 138 tools (83%) | 02-mcp-tool-audit |
| **Test file count** | 745 | 05-test-coverage |
| **Test function count (Test*)** | 9,267 | 05-test-coverage |
| **Overall reported coverage** | 84.5% | 05-test-coverage |
| **Failing packages** | 7 | 05-test-coverage |
| **Coverage-padding files** | 80 (807 Test* functions) | 05-test-coverage |
| **Fuzz test count** | 15 Fuzz* functions | 05-test-coverage |
| **Benchmark test count** | 20 functions, 6 files | 05-test-coverage |
| **Race-targeted test count** | 52 functions | 05-test-coverage |
| **Race conditions found (CRITICAL)** | 2 (R-01, R-02) | s1-race-condition-census |
| **Race conditions found (HIGH)** | 4 (R-03 to R-06) | s1-race-condition-census |
| **Race conditions found (MEDIUM)** | 8 (R-07 to R-14) | s1-race-condition-census |
| **Race conditions found (LOW)** | 4 (R-15 to R-18) | s1-race-condition-census |
| **Race conditions found (SAFE)** | 2 (R-19, R-20) | s1-race-condition-census |
| **Dead code (FleetDashboardModel)** | 443 lines | s2-dead-code-audit |
| **Phantom ROADMAP references (total)** | 158 | s2-dead-code-audit |
| **Phantom references in completed tasks** | 5 | s2-dead-code-audit |
| **Orphaned test files** | 187 (~39,436 lines) | s2-dead-code-audit |
| **Duplicate CircuitBreaker impls** | 4 | s2-dead-code-audit |
| **Stale TODOs in production code** | 0 | s2-dead-code-audit |
| **Budget enforcement layers** | 5 (Global, Worker, Pool, Sweep, Session) | s3-cost-model |
| **Budget gaps identified** | 4 (Gaps A-D) | s3-cost-model |
| **Max theoretical L3 spend (unlimited)** | Unbounded -- sessions can launch with $0 cap | s3-cost-model (Gap A) |
| **Average task cost** | $0.17 | 08-deep-dive |
| **Total observed spend** | $6.21 across 36 tasks | 08-deep-dive |
| **Provider distribution** | 100% Claude | 08-deep-dive |
| **Cost optimization potential** | $0.17 avg to $0.03-0.05 (70-85% reduction) | 11-llm-capabilities |
| **Swallowed errors (top 20)** | 1 critical, 4 high, 10 medium, 5 low | s4-error-handling |
| **Error wrapping ratio (`%w`)** | 65% global; worst: mcpserver 25%; best: store 93% | s4-error-handling |
| **Production panics** | 2 (both justified) | s4-error-handling |
| **Handler contract violations** | 0 | s4-error-handling |
| **L3 critical path tasks remaining** | ~100 (all 0% done, all L/XL) | 01-roadmap-matrix |
| **Parallelizable backlog (no L3 dependency)** | ~138 tasks | 01-roadmap-matrix |
| **Enhancer pipeline stages** | 13 (Stage 0-12) | 04-enhancer-pipeline |
| **Lint rules** | 14 (11 per-line + 3 cache) | 04-enhancer-pipeline |
| **Scoring dimensions** | 10 (weights sum to 1.00) | 04-enhancer-pipeline |
| **Supervisor observed tick interval** | ~27s (vs 60s default) | 08-deep-dive |
| **JSON format retry rate** | 25.7% (25 occurrences across 15 cycles) | 08-deep-dive |
| **Max fleet concurrency** | 128 sessions (32 workers x 4 sessions) | 06-fleet-sweep |
| **A2A implementation completeness** | Core protocol working; streaming/auth/input-resume are stubs | 06-fleet-sweep |

---

## 3. Contradiction Register

| # | Topic | Report A | Report B | Contradiction | Resolution |
|---|-------|----------|----------|---------------|------------|
| C-01 | MCP tool count | CLAUDE.md says 126 tools / 14 namespaces | 02-mcp-tool-audit counts 166 tools / 16 namespaces | 40-tool / 2-namespace discrepancy | CLAUDE.md is stale. `plugin` and `sweep` namespaces were added; several existing namespaces grew. True count is 166. |
| C-02 | Roadmap task count | ROADMAP.md header says "1,115 tasks, 442 complete" | 01-roadmap-matrix live grep counts 1,143 tasks / 503 complete | 28-task and 61-completion discrepancy | Header predates recent phase additions. Live count is authoritative. |
| C-03 | Phase 3.5 completion | 08-deep-dive says 28/30 (93%) | 01-roadmap-matrix says 24/30 (80%) | 4-task discrepancy | Likely different counting methods for the duplicate 3.5.5 section. The 01 report recounted from checkboxes and is more reliable. |
| C-04 | In-progress phases count | 08-deep-dive says 10 in progress | 01-roadmap-matrix says 6 in progress (15-96%) | Different threshold for "in progress" | 08 counts any phase with >0% and <100% as in-progress; 01 excludes phases at exactly 0% which some docs round differently. |
| C-05 | Deferred loading status | CLAUDE.md says tools use deferred loading | 02-mcp-tool-audit confirms deferred loading is NOT activated in production | Documentation disagrees with code | Production `cmd/mcp.go` does not set `DeferredLoading = true`. The documented behavior only applies in tests. |
| C-06 | Opus pricing in code vs reality | `config/costs.go` uses $15/$75 for Opus | 11-llm-capabilities reports Opus 4.6 at $5/$25 | Compiled-in rate 3x higher than actual | Code has not been updated for the Opus 4.6 price drop. Cost normalization overestimates by 67%. |
| C-07 | Phase 9 completion | ROADMAP.md marks Phase 9 as 100% complete | 01-roadmap-matrix and s2-dead-code-audit show tier-1 implementation files missing | Phase declared complete but code incomplete | The "5 tasks" counted as complete correspond only to tier-3 sub-items. Tier-1 tools lack implementations. |
| C-08 | Gemini Flash output rate | `config/costs.go` uses $2.50/1M | 11-llm-capabilities says actual Gemini 2.5 Flash is ~$3.50/1M | 40% underestimate | Code needs rate update. |
| C-09 | Default sweep budget | User convention says $0.50/session | Handler default is $5.00/session | 10x discrepancy | The $0.50 cap is a caller convention only; the handler does not enforce it without explicit parameter. |

---

## 4. Gap Register

Topics that no agent covered adequately despite being in the original task scope.

| # | Gap | Expected Coverage | Actual Coverage | Impact |
|---|-----|-------------------|-----------------|--------|
| G-01 | Plugin system architecture | Full audit of `internal/plugin/` | Only mentioned as dead-code items; no deep analysis of gRPC interface, marketplace, or plugin lifecycle | Low -- plugin is Phase 20 (0% done) |
| G-02 | Sandbox infrastructure | Full audit of `internal/sandbox/` | Mentioned in passing (Phase 5 status); no analysis of Docker, Firecracker, gVisor, or Incus implementations | Medium -- Phase 5 is 12% done |
| G-03 | Store/SQLite migration path | Detailed store layer analysis | 03-session-architecture covers MemoryStore and notes dual-write; no analysis of `internal/store/sqlite.go` implementation details | Medium -- store migration is Phase 10.5 |
| G-04 | Kubernetes controller | Audit of `internal/k8s/` | Only one swallowed error noted (s4); no architectural analysis of pod lifecycle, CRD, or controller logic | Low -- Phase 7 is 0% done |
| G-05 | Marathon subsystem depth | Deep audit of marathon supervisor | 05-test-coverage notes 1 failing test; s4 notes checkpoint issues; no deep architectural analysis of marathon loop, restart policies, or cloud scheduler | Medium -- marathon is used in L2+ autonomy |
| G-06 | Event bus architecture | Analysis of `internal/events/` | s4 notes publish errors silently discarded; no analysis of NATS integration, event bus capacity, or subscriber patterns | Medium -- events drive supervisor decisions |
| G-07 | Bandit/UCB cascade details | Analysis of `internal/bandit/` | 03-session-architecture mentions bandit hooks in cascade; no deep analysis of Thompson sampling, UCB, or exploration/exploitation tuning | Low -- Phase 24 is 0% done |
| G-08 | First-boot wizard | Analysis of `firstboot.go` TUI view | 08-distro-audit mentions firstboot service; 07-tui-audit notes `FirstBootView` is not wired; no analysis of wizard steps | Low -- Phase 4 item |
| G-09 | Workflow engine depth | Analysis of workflow define/run/delete | 06-fleet-sweep mentions workflow handlers; no deep analysis of YAML schema, executor, step isolation | Low -- Phase 8.3 is 0% |
| G-10 | Cross-repo tool interaction | How tools in different namespaces compose | No report analyzed tool composition patterns or cross-namespace dependencies | Medium -- affects tool design |

---

## 5. Cross-Reference Matrix

Findings mapped to roadmap phases they affect. Columns show phases with active findings; empty phases omitted.

| Finding | Ph 0.6 | Ph 1 | Ph 1.5 | Ph 3.5 | Ph 4 | Ph 6 | Ph 8 | Ph 9 | Ph 9.5 | Ph 10.5 | Ph 13 | Ph 17 | Ph 23 | Ph 24 | ROADMAP |
|---------|--------|------|--------|--------|------|------|------|------|--------|---------|-------|-------|-------|-------|---------|
| F-001 | | | | | | | | | | X | | | | | |
| F-002 | | | | | | | | | | X | | | | | |
| F-003 | | | | X | | | | | | | | | | | X |
| F-004 | | | | | | | | X | | | | | | | X |
| F-005 | | | | | | | | X | | | | | | | X |
| F-006 | | X | | | | | | | | | | | | | |
| F-007 | | | | | | | | | X | | | | | | |
| F-008 | | | | | | | | | | | | | X | | |
| F-009 | | | | | | | | | | X | | | | | |
| F-010 | | X | | | | | | | | | | | | | |
| F-011 | | | | | | | | | | X | | | | | |
| F-012 | | | | | | | | | | X | | | | | |
| F-013 | | | | | | | | | | | | | X | | |
| F-014 | | | | | | | | | | X | | | | | |
| F-016 | | | | | | | X | | | | | | | | |
| F-017 | | | | | | | | | X | | X | | | | |
| F-020 | | | | | | | | | | X | | | | | |
| F-021 | | | | | | | | | | X | | | | | |
| F-022 | | | | | | | | | | X | | | | | |
| F-023 | X | | | | | | | | | | | | | | |
| F-025 | | | X | | | | | | | | | | | | |
| F-026 | | | X | | | | | | | | | | | | |
| F-027 | | X | | | | | | | | | | | | | |
| F-029 | | X | | | | | | | | | | | | | |
| F-032 | | | | | | | | | | | | | | X | |
| F-033 | | X | | | | | | | | | | | | | |
| F-035 | | | X | | | | | | | | | | | | |
| F-037 | | X | | | | | | | | | | | | | X |
| F-039 | | | | | | | | | | | | | | | X |
| F-041 | | | | | | | | | | | | | | X | |
| F-042 | | | | | | | | | | X | | | | | |
| F-046 | | | | | X | | | | | | | | | | |
| F-047 | | | | | | | | | | | | X | | | |
| F-048 | | | | | | X | | | | | | | | | |
| F-049 | | | | | | | | | | | X | | | | |
| F-054 | | | X | | | | | | | | | | | | |
| F-058 | | | X | | | | | | | | | | | | |
| F-063 | | | | | X | | | | | | | | | | |
| **Count** | **1** | **6** | **6** | **1** | **2** | **1** | **1** | **2** | **2** | **12** | **2** | **1** | **3** | **2** | **5** |

Phase 10.5 (Horizontal & Vertical Scaling) has the highest concentration of findings (12), followed by Phase 1 and Phase 1.5 (6 each).

---

## 6. Research URLs

All external URLs from Wave 3 research agents (09-orchestration-landscape, 10-mcp-ecosystem, 11-llm-capabilities, 12-thin-client-patterns), categorized by topic.

### MCP Protocol and SDKs

- MCP Specification (2025-11-25): https://modelcontextprotocol.io/specification/2025-11-25
- MCP First Anniversary: https://blog.modelcontextprotocol.io/posts/2025-11-25-first-mcp-anniversary/
- MCP Changelog: https://modelcontextprotocol.io/specification/2025-11-25/changelog
- MCP 2026 Roadmap: http://blog.modelcontextprotocol.io/posts/2026-mcp-roadmap/
- MCP Official Roadmap: https://modelcontextprotocol.io/development/roadmap
- MCP Transports Spec: https://modelcontextprotocol.io/specification/2025-03-26/basic/transports
- MCP Auth Tutorial: https://modelcontextprotocol.io/docs/tutorials/security/authorization
- MCP 2025-06-18 Spec Update: https://forgecode.dev/blog/mcp-spec-updates/
- MCP Go SDK (official): https://github.com/modelcontextprotocol/go-sdk
- MCP Go SDK v1.0.0: https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.0.0
- MCP Go SDK pkg.go.dev: https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp
- mcp-go (community): https://github.com/mark3labs/mcp-go
- MCP Go SDK Design Discussion: https://github.com/orgs/modelcontextprotocol/discussions/364
- Agentic AI Foundation: https://www.anthropic.com/news/donating-the-model-context-protocol-and-establishing-of-the-agentic-ai-foundation
- Linux Foundation AAIF: https://www.linuxfoundation.org/press/linux-foundation-announces-the-formation-of-the-agentic-ai-foundation
- Why MCP Deprecated SSE: https://blog.fka.dev/blog/2025-06-06-why-mcp-deprecated-sse-and-go-with-streamable-http/
- Socket.dev MCP Update: https://socket.dev/blog/mcp-spec-updated-to-add-structured-tool-output-and-improved-oauth-2-1-compliance
- WorkOS MCP Spec Update: https://workos.com/blog/mcp-2025-11-25-spec-update
- Auth0 MCP Auth: https://auth0.com/blog/mcp-specs-update-all-about-auth/
- Den.dev MCP Auth: https://den.dev/blog/mcp-november-authorization-spec/
- Aaron Parecki MCP Client Registration: https://aaronparecki.com/2025/11/25/1/mcp-authorization-spec-update
- Stack Overflow MCP Auth: https://stackoverflow.blog/2026/01/21/is-that-allowed-authentication-and-authorization-in-model-context-protocol/
- MCP vs A2A Comparison: https://apigene.ai/blog/mcp-vs-a2a-when-to-use-each-protocol
- AI Agent Protocol Ecosystem Map 2026: https://www.digitalapplied.com/blog/ai-agent-protocol-ecosystem-map-2026-mcp-a2a-acp-ucp

### MCP Gateways

- 5 Best MCP Gateways: https://www.getmaxim.ai/articles/5-best-mcp-gateways-for-developers-in-2026-2/
- Composio MCP Gateways Guide: https://composio.dev/content/mcp-gateways-guide
- Microsoft MCP Gateway: https://github.com/microsoft/mcp-gateway
- Envoy AI Gateway MCP: https://aigateway.envoyproxy.io/blog/mcp-implementation/
- Traefik Hub MCP: https://doc.traefik.io/traefik-hub/mcp-gateway/mcp
- MCP Gateway Registry: https://github.com/agentic-community/mcp-gateway-registry
- MCP OAuth Implementation: https://www.mcpserverspot.com/learn/architecture/mcp-oauth-implementation-guide
- SAP MCP Enterprise Pain Points: https://community.sap.com/t5/artificial-intelligence-blogs-posts/8-critical-pain-points-of-mcp-in-an-enterprise/ba-p/14303370
- New Stack MCP Production: https://thenewstack.io/model-context-protocol-roadmap-2026/
- WorkOS Enterprise MCP: https://workos.com/blog/2026-mcp-roadmap-enterprise-readiness
- Cloudflare MCP Transport: https://developers.cloudflare.com/agents/model-context-protocol/transport/
- Sunpeak SSE to Streamable HTTP: https://sunpeak.ai/blogs/claude-connector-sse-to-streamable-http/
- Auth0 Streamable HTTP Security: https://auth0.com/blog/mcp-streamable-http/

### Agent Frameworks and SDKs

- Claude Agent SDK Overview: https://platform.claude.com/docs/en/agent-sdk/overview
- Building Agents with Claude Agent SDK: https://www.anthropic.com/engineering/building-agents-with-the-claude-agent-sdk
- Claude Code Agent Teams: https://code.claude.com/docs/en/agent-teams
- TeammateTool Discovery: https://paddo.dev/blog/claude-code-hidden-swarm/
- Claude Agent Teams Guide: https://www.nxcode.io/resources/news/claude-agent-teams-parallel-ai-development-guide-2026
- Google A2A Announcement: https://developers.googleblog.com/en/a2a-a-new-era-of-agent-interoperability/
- A2A Go SDK: https://github.com/a2aproject/a2a-go
- Google ADK Go 1.0: https://developers.googleblog.com/adk-go-10-arrives/
- ADK Go Announcement: https://developers.googleblog.com/announcing-the-agent-development-kit-for-go-build-powerful-ai-agents-with-your-favorite-languages/
- OpenAI Agents SDK: https://openai.github.io/openai-agents-python/
- OpenAI Handoffs: https://openai.github.io/openai-agents-python/handoffs/
- OpenAI Guardrails: https://openai.github.io/openai-agents-python/guardrails/
- OpenAI Codex CLI: https://github.com/openai/codex
- LangGraph: https://github.com/langchain-ai/langgraph
- LangGraph 2026: https://dev.to/ottoaria/langgraph-in-2026-build-multi-agent-ai-systems-that-actually-work-3h5
- LangChain Multi-Agent: https://docs.langchain.com/oss/python/langchain/multi-agent
- deepagents: https://github.com/langchain-ai/deepagents
- CrewAI: https://github.com/crewAIInc/crewAI
- CrewAI Stars Analysis: https://theagenttimes.com/articles/44335-stars-and-counting-crewais-github-surge-maps-the-rise-of-the-multi-agent-e
- CrewAI Deep Dive: https://qubittool.com/blog/crewai-multi-agent-workflow-guide
- AutoGen: https://github.com/microsoft/autogen
- AG2: https://github.com/ag2ai/ag2
- Microsoft Agent Framework: https://learn.microsoft.com/en-us/agent-framework/overview/
- Microsoft Agent Framework Announcement: https://devblogs.microsoft.com/foundry/introducing-microsoft-agent-framework-the-open-source-engine-for-agentic-ai-apps/
- Agent Framework Migration: https://devblogs.microsoft.com/agent-framework/migrate-your-semantic-kernel-and-autogen-projects-to-microsoft-agent-framework-release-candidate/
- AutoGen Split Analysis: https://dev.to/maximsaplin/microsoft-autogen-has-split-in-2-wait-3-no-4-parts-2p58
- smolagents: https://github.com/huggingface/smolagents
- smolagents Analysis: https://www.decisioncrafters.com/smolagents-build-powerful-ai-agents-in-1-000-lines-of-code-with-26-3k-github-stars/
- MCP Adapter for smolagents: https://grll.github.io/mcpadapt/guide/smolagents/
- PydanticAI: https://ai.pydantic.dev/
- PydanticAI GitHub: https://github.com/pydantic/pydantic-ai
- PydanticAI MCP: https://ai.pydantic.dev/mcp/overview/
- PydanticAI Analysis: https://www.decisioncrafters.com/pydanticai-type-safe-ai-agent-framework-with-16k-github-stars/
- agency-swarm: https://github.com/VRSEN/agency-swarm
- CAMEL-AI: https://github.com/camel-ai/camel
- CAMEL NeurIPS Paper: https://openreview.net/forum?id=3IyL2XWDkG

### LLM Provider Pricing and Capabilities

- Anthropic Pricing: https://platform.claude.com/docs/en/about-claude/pricing
- Google Gemini Pricing: https://ai.google.dev/gemini-api/docs/pricing
- OpenAI Pricing: https://developers.openai.com/api/docs/pricing
- Claude 1M Context GA: https://claude.com/blog/1m-context-ga
- Claude Extended Thinking: https://platform.claude.com/docs/en/build-with-claude/extended-thinking
- Claude Prompt Caching: https://platform.claude.com/docs/en/build-with-claude/prompt-caching
- Claude Code Subagents: https://code.claude.com/docs/en/sub-agents
- Claude 4.6 What's New: https://platform.claude.com/docs/en/about-claude/models/whats-new-claude-4-6
- Claude Code Q1 2026: https://www.mindstudio.ai/blog/claude-code-q1-2026-update-roundup
- Claude Caching Guide: https://blog.wentuo.ai/en/claude-code-prompt-caching-ttl-pricing-guide-en.html
- Gemini Caching: https://ai.google.dev/gemini-api/docs/caching
- Gemini Implicit Caching: https://developers.googleblog.com/en/gemini-2-5-models-now-support-implicit-caching/
- Gemini CLI MCP: https://geminicli.com/docs/tools/mcp-server/
- OpenAI Prompt Caching: https://developers.openai.com/api/docs/guides/prompt-caching
- OpenAI Prompt Caching Announcement: https://openai.com/index/api-prompt-caching/
- Codex CLI Features: https://developers.openai.com/codex/cli/features
- Codex Noninteractive: https://developers.openai.com/codex/noninteractive
- Confidence-Based Escalation Paper: https://arxiv.org/abs/2410.10347
- Claude Worktrees Productivity: https://botmonster.com/posts/claude-md-productivity-stack-custom-commands-git-worktrees-agent-rules/

### Benchmarks

- SWE-bench Verified: https://www.vals.ai/benchmarks/swebench
- marc0.dev Leaderboard: https://www.marc0.dev/en/leaderboard
- LLM Coding Benchmark Comparison 2026: https://smartscope.blog/en/generative-ai/chatgpt/llm-coding-benchmark-comparison-2026/

### Thin Client and Distro

- NixOS.org: https://nixos.org/
- NixOS AI Infrastructure: https://medium.com/@mehtacharu0215/nixos-powered-ai-infrastructure-reproducible-immutable-deployable-anywhere-d3e225fc9b5a
- NixOS Most Powerful 2026: https://allthingsopen.org/articles/nixos-most-powerful-linux-distro-2026
- Nixiosk: https://github.com/matthewbauer/nixiosk
- NixOS Kiosk with Cage: https://github.com/matthewbauer/nixos-kiosk
- Fedora Kinoite: https://fedoraproject.org/atomic-desktops/kinoite/
- Fedora Atomic: https://fedoramagazine.org/introducing-fedora-atomic-desktops/
- Fedora Silverblue Review: https://www.xda-developers.com/replaced-my-linux-desktop-fedora-silverblue-feels-futuristic/
- Arch Installer Compositors: https://9to5linux.com/arch-linux-installer-now-supports-labwc-niri-and-river-wayland-compositors
- Arch vs NixOS 2026: https://www.slant.co/versus/2690/2700/~arch-linux_vs_nixos
- Manjaro ARM Wiki: https://wiki.manjaro.org/index.php/Manjaro-ARM
- Hyprland vs Sway 2025: https://gigasblade.blogspot.com/2025/10/hyprland-vs-swaywm-2025-dazzling.html
- Hyprland vs Sway Landscape: https://www.oreateai.com/blog/hyprland-vs-sway-navigating-the-wayland-tiling-window-manager-landscape-in-2025/
- Tiling WMs Productivity: https://dasroot.net/posts/2026/01/tiling-window-managers-i3-sway-hyprland-productivity/
- Sway ArchWiki: https://wiki.archlinux.org/title/Sway
- Cage GitHub: https://github.com/cage-kiosk/cage
- Cage 0.2: https://www.phoronix.com/news/Cage-0.2-Released
- niri: https://github.com/niri-wm/niri
- niri v25.01: https://www.phoronix.com/news/Niri-25.01-Tiling-Wayland-Comp
- wlroots Multi-GPU Issue: https://github.com/swaywm/wlroots/issues/934
- Multiple NVIDIA GPUs Wayland: https://bbs.archlinux.org/viewtopic.php?id=303372
- NVIDIA open-gpu Issue: https://github.com/NVIDIA/open-gpu-kernel-modules/issues/318
- RTX 4090 Cursor Lag: https://github.com/basecamp/omarchy/discussions/4702
- Hyprland WLR_DRM_DEVICES: https://github.com/hyprwm/hyprland-wiki/issues/694
- Wayland Multi-Monitor 2026: https://copyprogramming.com/howto/multi-monitor-issues-with-xorg-nvidia-wayland

### Secrets and Boot Security

- systemd-creds ArchWiki: https://wiki.archlinux.org/title/Systemd-creds
- systemd-creds Magic: https://smallstep.com/blog/systemd-creds-hardware-protected-secrets/
- systemd.io Credentials: https://systemd.io/CREDENTIALS/
- systemd-creds RHEL: https://oneuptime.com/blog/post/2026-03-04-systemd-credentials-secret-injection-rhel-9/view
- 1Password CLI: https://developer.1password.com/docs/cli/get-started/
- 1Password Secrets in Scripts: https://developer.1password.com/docs/cli/secrets-scripts/
- 1Password systemd: https://1password.community/discussion/128572/how-to-inject-a-secret-into-the-environment-via-a-systemd-service-definition
- UEFI Secure Boot ArchWiki: https://wiki.archlinux.org/title/Unified_Extensible_Firmware_Interface/Secure_Boot
- TPM 2.0 and Secure Boot: https://servermall.com/blog/tpm-2-0-and-secure-boot-on-the-server/
- NSA UEFI Guidance: https://media.defense.gov/2025/Dec/11/2003841096/-1/-1/0/CSI_UEFI_SECURE_BOOT.PDF
- NVIDIA DKMS Signing: https://gist.github.com/lijikun/22be09ec9b178e745758a29c7a147cc9
- NVIDIA r595 Guide: https://docs.nvidia.com/datacenter/tesla/pdf/Driver_Installation_Guide.pdf
- TPM2 LUKS systemd-cryptenroll: https://gierdo.astounding.technology/blog/2025/07/05/tpm2-luks-systemd

### OTA Updates and Fleet Deployment

- Rugix OTA Compared: https://rugix.org/blog/2026-02-28-ota-update-engines-compared/
- RAUC: https://rauc.io/
- SWUpdate vs Mender vs RAUC: https://32blog.com/en/yocto/yocto-ota-update-comparison
- FOSDEM 2025 A/B Updates: https://archive.fosdem.org/2025/events/attachments/fosdem-2025-6299-exploring-open-source-dual-a-b-update-solutions-for-embedded-linux/slides/237879/leon-anav_pyytRpX.pdf
- Torizon OTA Guide: https://www.torizon.io/blog/ota-best-linux-os-image-update-model
- greetd ArchWiki: https://wiki.archlinux.org/title/Greetd
- tuigreet: https://github.com/apognu/tuigreet
- greetd SourceHut: https://sr.ht/~kennylevinsen/greetd/
- Cage Website: https://www.hjdskes.nl/projects/cage/

---

## 7. Missing Data

Questions that remain unanswered after this sweep.

### Architecture

1. **SQLite store implementation details** -- The `internal/store/sqlite.go` exists but no agent analyzed its schema, migration path, or WAL mode configuration in detail.
2. **NATS JetStream integration** -- Phase 10.5 references NATS for event bus transport; no agent analyzed the `internal/events/` NATS subscriber/publisher implementation.
3. **Worktree pool lifecycle** -- `WorktreePool` is mentioned in loop engine flow but no agent analyzed pool sizing, cleanup, or stale worktree handling.
4. **Session rehydration correctness** -- When `RehydrateFromStore` runs on restart, what happens to sessions that were mid-turn? No analysis of partial-state recovery.

### Testing

5. **Actual `go test -race ./...` results** -- No agent ran the race detector; findings are from static analysis only. Actual race detector output would confirm or refute R-01 through R-20.
6. **Coverage delta from padding removal** -- If the ~5% call-and-ignore padding tests were removed, what would the real coverage number be?
7. **E2E scenario pass rate** -- No agent ran the e2e scenarios. The 24 scenarios are documented but not validated against current code.

### Cost and Operations

8. **Actual Gemini/Codex cost data** -- All observed cost data is 100% Claude. No real-world cost observations exist for multi-provider cascade routing.
9. **DecisionModel training data volume** -- How many `LoopObservation` records actually exist? Is the 50-sample threshold close to being met?
10. **Fleet coordinator stress test** -- No load testing data. The theoretical max of 128 concurrent sessions has never been validated.

### Distro and Thin Client

11. **Bootable ISO test results** -- No agent tested whether `make iso` produces a bootable image in QEMU. Phase 4.1.3 and 4.1.4 remain unvalidated.
12. **Dual RTX 4090 Sway compatibility** -- 12-thin-client-patterns flags the DMA-BUF interop problem but no validation on actual hardware.
13. **hw-detect.sh on target hardware** -- The script has only been analyzed statically. No run on ProArt X870E hardware.

### Security

14. **Full path traversal exploit verification** -- The 5 Sprint 7 BLOCKERs are identified but no agent verified exploitability with concrete payloads.
15. **Tailscale ACL policy audit** -- The policy JSON was read but not analyzed for over-permissive rules or tag escalation paths.

### Roadmap

16. **Phase 13 feasibility with current architecture** -- The L3 autonomy target (72-hour unattended) requires self-healing, but no analysis of whether the current supervisor architecture can support it without fundamental redesign.
17. **Phase 7 (Kubernetes) CRD design** -- The `internal/k8s/` package exists but no agent analyzed whether the CRD design is compatible with the A2A protocol integration in Phase 11.
18. **WASM plugin runtime selection** -- Phase 20 requires embedded WASM; no analysis of which Go WASM runtime (wazero, wasmtime, wasmer) fits the mcpkit architecture.
