# 18 -- Roadmap-Research Reconciliation

Generated: 2026-04-04
Sources: ROADMAP.md (2,736 lines) reconciled against 16 research reports (01-12, s1-s4),
13-strategic-initiatives.md, 14-autonomy-path.md, 16-research-index.md

---

## 1. Coverage Analysis

### ROADMAP tasks addressed by research findings

The 16 research reports and 4 specialist audits produced 82 indexed findings (F-001
through F-082) plus 9 contradictions (C-01 through C-09) and 10 coverage gaps (G-01
through G-10). Below is the mapping of findings to ROADMAP phases.

**Phase 0.9 (Quick Wins) -- COMPLETE, well-covered.**
All 12 QW items have corresponding findings. QW-1 maps to F-048 (JSON retry rate),
QW-3 maps to F-048/signal:killed, QW-6 maps to F-049 (zero baseline), QW-7 maps to
F-037 (phantom file reference -- snapshot.go missing), QW-8 maps to F-020 (budget
bypass), QW-11 maps to fleet audit findings. Research validates these were correctly
prioritized as quick wins.

**Phase 1 (Harden & Test) -- 96% complete, 6 findings.**
F-006 (5 BLOCKER path traversal vulns) affects Phase 1 MCP handlers but is not tracked
as any Phase 1 task. F-010 (loadedGroups race) affects tools.go in Phase 1 scope but
Phase 1 has no task for it. F-027 (8 untested handler files) is partially addressed by
1.6.4 but no specific task targets handler_sweep_report.go (302 lines, highest risk).
F-029 (ValidationMiddleware gaps) is not tracked. F-033 (7 failing packages) overlaps
1.6.4 but the specific packages are not enumerated. F-066 (mcpserver %w ratio) is not
tracked.

Remaining open tasks 1.2.5.1 (ParamParser) and 1.2.5.4 (handler generator) have no
corresponding research findings -- they are internal DX tasks.

**Phase 1.5 (Developer Experience) -- 83% complete, 6 findings.**
F-025 (deferred loading inactive) maps to 1.5.11 area but is not explicitly called out
as a task. F-026 (advanced namespace too broad, 24 tools) is not tracked. F-035
(LogView unbounded) maps to TUI performance concerns but has no 1.5.10 subtask. F-036
(LoopRun.Iterations O(n)) also lacks a specific task. F-054 (FleetDashboardModel dead
code) has no cleanup task. F-058 (RefreshAllRepos synchronous on tick) has no task.

Open tasks 1.5.10 (Charm v2) and 1.5.11 (official SDK migration) are well-covered by
research: F-025 (deferred loading), findings from 02-mcp-tool-audit and 10-mcp-ecosystem
directly inform the SDK migration scope.

**Phase 3 (i3/WM Integration) -- 69% complete, 1 finding.**
F-003 (Phase 3.5.5 numbering collision) is the only direct finding. Research document
12-thin-client-patterns recommends dropping i3 support (X11 end-of-life) which affects
tasks 3.1.4, 3.1.5, 3.2.3, 3.2.4. These open tasks target i3 IPC, which is being
superseded by Sway/Hyprland.

**Phase 4 (Bootable Thin Client) -- 21% complete, 2 findings.**
F-046 (hw-detect.sh single GPU only) maps to 4.4.1 but the dual RTX 4090 case is not a
subtask. F-063 (Tailscale auth key plaintext) maps to 4.9/12.1 overlap but no Phase 4
task addresses it. Research from 08-distro-audit and 12-thin-client-patterns produced
extensive recommendations (LUKS encryption, 1Password CLI, Cage compositor, drop i3,
greetd/tuigreet) that have no ROADMAP entries.

**Phase 5 (Agent Sandboxing) -- 12% complete, minimal coverage.**
Gap G-02 in the research index notes the sandbox infrastructure was not deeply audited.
F-006 (path traversal) and F-019 (hook exit codes) are security concerns relevant to
Phase 5 but tracked elsewhere.

**Phase 6 (Advanced Fleet Intelligence) -- 84% complete, 1 finding.**
F-048 (JSON format enforcement, 25.7% retry rate) affects 6.1/6.2 loop engine. Open
tasks 6.1.3 (DAG viz), 6.4.3 (OTel), 6.8.4/6.8.5 (TUI A/B), 6.9.1-6.9.4 (NL control)
have no corresponding research findings.

**Phase 8 (Advanced Orchestration) -- 24% complete, 1 finding.**
F-016 (RunLoop error silently discarded in handler_selfimprove.go) directly affects 8.5
self-improvement engine. Open tasks 8.1-8.4, 8.5.6-8.5.8, 8.6 have no research findings.

**Phase 9 (R&D Cycle Automation) -- marked 100%, 2 stale findings.**
F-004 and F-005 were based on roadmap file references that no longer match the code layout.
The tier-1 rdcycle handlers are implemented in `internal/mcpserver/handler_rdcycle.go`
and registered under the `rdcycle` tool group. The real gap is doc/code drift plus
continued behavior coverage, not missing handler implementations.

**Phase 9.5 (Autonomous R&D Supervisor) -- marked 100%, 2 findings.**
F-007 (GateEnabled unprotected var) and F-017/F-018 (supervisor swallows cycle failures)
affect 9.5 directly. The research validates 9.5 is functionally complete but has
safety-critical defects not tracked as 9.5 tasks.

**Phase 10.5 (Horizontal & Vertical Scaling) -- 29% complete, 12 findings.**
This phase has the highest concentration of research findings. F-001, F-002 (CRITICAL
races), F-009 (lock ordering), F-011 (unbounded queue), F-012 (queue not persisted),
F-014 (advisory autoscaler), F-020/F-021/F-022 (budget gaps A/B/D), F-038 (worker
timeout), F-042 (CostPredictor not wired), F-044 (GetLoop write lock). Many of these
have corresponding 10.5 tasks, but the race conditions (F-001, F-002) are not explicitly
listed as 10.5 subtasks -- they are cross-cutting.

**Phase 13 (L3 Autonomy Core) -- 0% complete, 2 findings.**
F-017 (supervisor silence) and F-049 (autonomy persist failure) affect Phase 13 design.
The 14-autonomy-path document provides gate criteria (G3.1-G3.11, F3.1-F3.9) that map
to specific Phase 13 tasks, but the ROADMAP itself does not reference these gate criteria.

**Phases 14-25 -- all 0%, minimal research coverage.**
These future phases have no direct findings. Research document 09-orchestration-landscape
and 11-llm-capabilities provide strategic context (competitive positioning, cost
optimization potential) but no task-level findings.

### ROADMAP tasks with NO corresponding research finding

| Phase | Task | Gap |
|-------|------|-----|
| 1 | 1.2.5.1 ParamParser extraction | No research assessed handler framework DX |
| 1 | 1.2.5.4 Handler generator | Blocked by 1.2.5.1; no research coverage |
| 1.5 | 1.5.10 Charm v2 migration (XL) | Research mentions it but no finding targets it |
| 3 | 3.1.4/3.1.5 i3 monitor enum/events | Research says drop i3; tasks may be obsolete |
| 3 | 3.4.1-3.4.4 autorandr hotplug | No research coverage |
| 3 | 3.6.1/3.6.4 Hyprland IPC/workspaces | No research coverage |
| 4 | 4.1.3-4.1.7 ISO pipeline completion | Research recommends but no finding quantifies gap |
| 4 | 4.2.1-4.2.6 i3 kiosk configuration | Research says drop i3 |
| 4 | 4.3.1-4.3.5 PXE/network boot | No research coverage |
| 5 | 5.2-5.8 (35 tasks) | Gap G-02: sandbox not audited |
| 6 | 6.9.1-6.9.4 NL fleet control | No research coverage |
| 7 | All 25 tasks | No research coverage |
| 8 | 8.1, 8.3, 8.4, 8.6 (25 tasks) | No research coverage |

---

## 2. Priority Disagreements

### Race conditions not prioritized in ROADMAP

The s1 race condition census found 2 CRITICAL and 4 HIGH race conditions. None appear
as explicit ROADMAP tasks with P0/P1 labels. The ROADMAP does have Phase 0.5.9 (race
condition in MCP scan, COMPLETE) but this only addressed the `repos` map race. The
remaining 6 CRITICAL/HIGH races are invisible in the ROADMAP priority structure.

| Finding | Severity | ROADMAP Priority | Research Priority |
|---------|----------|-----------------|-------------------|
| F-001 (R-01): AutoRecovery.retryState | CRITICAL | Not tracked | P0 -- L1 gate blocker (G1.1) |
| F-002 (R-02): RetryTracker.attempts | CRITICAL | Not tracked | P0 -- L2 gate blocker (G2.7) |
| F-007 (R-03): GateEnabled unprotected | HIGH | Not tracked | P0 -- L2 gate blocker (G2.5) |
| F-008 (R-04): OpenAI LastResponseID | HIGH | Not tracked | P1 -- L2 gate blocker (G2.9) |
| F-009 (R-05): GetTeam lock ordering | HIGH | Not tracked | P1 -- L2 gate blocker (G2.8) |
| F-010 (R-06): loadedGroups map | HIGH | Not tracked | P0 -- L2 gate blocker (G2.6) |

**Recommendation:** These should be P0 tasks in a new Phase 0.95 or added to Phase 10.5
with explicit autonomy-gate references.

### Budget enforcement gaps not prioritized

S3 found 4 budget bypass paths (Gaps A-D). Only Gap A is partially addressed by QW-8
(budget params silently ignored). The remaining gaps have no ROADMAP tasks:

| Gap | Description | ROADMAP Status | Research Priority |
|-----|-------------|---------------|-------------------|
| Gap A | MaxBudgetUSD == 0 allows uncapped sessions | QW-8 addressed partially | P0 -- L2 gate (G2.1) |
| Gap B | Active sessions not stopped on GlobalBudget exhaustion | Not tracked | P0 -- L3 gate (G3.2) |
| Gap C | pool.State.CanSpend() reads stale totals | Not tracked | P1 |
| Gap D | Sweep schedule cap is reactive (5-min polling) | Not tracked | P1 -- L2 gate (F2.5) |

### Supervisor error handling not prioritized

S4 found the supervisor silently swallows cycle failures at 3 levels (F-017, F-018).
The ROADMAP has no task addressing this. Research rates this as a hard L1 gate blocker
(G1.2). The ROADMAP should track:
- Supervisor cycle failure escalation (currently Warn, needs Error + event + HITL interrupt)
- RunLoop error propagation (handler_selfimprove.go:71 discards error)
- Hook exit code handling (hooks.go:139 discards cmd.Run() error)

### Path traversal vulnerabilities not prioritized

F-006 identifies 5 BLOCKER-level path traversal vulnerabilities in MCP handlers. The
ROADMAP has no Phase 1 task for these. The 08-ralph-deep-dive Sprint 7 audit discovered
them but they were never added to the ROADMAP. These should be P0 in Phase 1 or a
dedicated security fix phase.

### Sweep default budget disagreement

S3 finds the handler default is $5.00/session (C-09) while user convention is $0.50.
The ROADMAP does not track this as a task. Research rates it as L2 gate blocker (G2.2).

---

## 3. Missing from ROADMAP

### Safety / Concurrency Fixes (new tasks needed)

| ID | Description | Source | Effort | Suggested Phase |
|----|-------------|--------|--------|-----------------|
| NEW-1 | Add sync.Mutex to AutoRecovery.retryState | s1 R-01 | S | 0.95 |
| NEW-2 | Add sync.Mutex to RetryTracker.attempts | s1 R-02 | S | 0.95 |
| NEW-3 | Make GateEnabled atomic.Bool | s1 R-03 | S | 0.95 |
| NEW-4 | Add sync.Mutex to OpenAIClient.LastResponseID | s1 R-04 | S | 0.95 |
| NEW-5 | Fix GetTeam lock ordering (two-phase read) | s1 R-05 | M | 0.95 |
| NEW-6 | Protect loadedGroups with s.mu in MCP dispatch | s1 R-06 | S | 0.95 |
| NEW-7 | Add WaitGroup to supervisor tick goroutines | s1 R-07 | M | 0.95 |
| NEW-8 | Fix hitCount[key]++ RLock -> Lock in TieredKnowledge | s1 R-08 | S | 0.95 |
| NEW-9 | Fix anomaly detector cancel field races (R-11, R-12) | s1 | S | 0.95 |
| NEW-10 | Fix double cmd.Wait() in kill/runner path | s1 R-09/R-14 | M | 0.95 |
| NEW-11 | Fix 5 BLOCKER path traversal vulns in MCP handlers | 08-deep-dive | L | 0.95 |
| NEW-12 | Surface supervisor cycle failure at Error level | s4 fix #1 | M | 0.95 |
| NEW-13 | Propagate RunLoop error (handler_selfimprove.go:71) | s4 fix #2 | S | 0.95 |
| NEW-14 | Handle hook exit codes (hooks.go:139) | s4 fix #6 | S | 0.95 |
| NEW-15 | Fix autonomy level persist failure (retry + backoff) | s4 fix #4 | S | 0.95 |

### Cost Model Hardening (new tasks needed)

| ID | Description | Source | Effort | Suggested Phase |
|----|-------------|--------|--------|-----------------|
| NEW-16 | Enforce mandatory $5 default budget floor | s3 Gap A | S | 10.5 |
| NEW-17 | Change sweep handler default from $5.00 to $0.50 | s3 Gap C | S | 10.5 |
| NEW-18 | Wire fleet CostPredictor.Record() to handleWorkComplete | s3 R2 | S | 10.5 |
| NEW-19 | Update Gemini Flash output rate ($2.50 -> $3.50) | s3 section 3 | S | 10.5 |
| NEW-20 | Update Opus rate ($15/$75 -> $5/$25 for Opus 4.6) | s3 section 3, C-06 | S | 10.5 |
| NEW-21 | Stop active sessions on GlobalBudget exhaustion | s3 Gap B | M | 10.5 |
| NEW-22 | Shorten pool.State refresh interval (reactive -> event) | s3 Gap D | M | 10.5 |
| NEW-23 | Add per-hour spend circuit breaker for L3 | 14-autonomy-path | M | 13 |
| NEW-24 | Add cost rate staleness alerting | s3 section 3 | M | 10.5 |
| NEW-25 | Fix auto-size inflation (cap only when caller omits budget) | s3 section 5 | S | 10.5 |
| NEW-26 | Add org-wide cost rate override file | s3 section 3 | S | 10.5 |

### Dead Code Cleanup (new tasks needed)

| ID | Description | Source | Lines | Suggested Phase |
|----|-------------|--------|-------|-----------------|
| NEW-27 | Delete FleetDashboardModel (fleet_dashboard.go + test) | s2 F-054 | 443 | 1.5 |
| NEW-28 | Remove or implement ViewSearch iota entry | s2 F-056 | ~5 | 1.5 |
| NEW-29 | Remove or implement ModalStack | s2 F-055 | ~30 | 1.5 |
| NEW-30 | Consolidate 4 CircuitBreaker implementations | s2 F-043 | ~400 | 1.5 |

### MCP Modernization (new tasks needed)

| ID | Description | Source | Effort | Suggested Phase |
|----|-------------|--------|--------|-----------------|
| NEW-31 | Activate deferred loading in production binary | 02, F-025 | S | 1.5 |
| NEW-32 | Split advanced namespace (24 tools -> 3-4 sub-namespaces) | 02, F-026 | M | 1.5 |
| NEW-33 | Move 7 misplaced tools to correct namespaces | 02, F-053 | M | 1.5 |
| NEW-34 | Add IdempotentHint to all read-only tools | 02, F-051 | M | 1.5 |
| NEW-35 | Add per-namespace concurrency caps to middleware | 02 | M | 10.5 |
| NEW-36 | Add MaxPromptLength validation to prompt_improve | 02, F-029 | S | 1 |
| NEW-37 | Update handleLoadToolGroup description (missing namespaces) | 02, F-050 | S | 1 |
| NEW-38 | Wire FormatContextBlockMarkdown for Gemini/OpenAI targets | 04, F-040 | S | 23 |

### Test Infrastructure (new tasks needed)

| ID | Description | Source | Effort | Suggested Phase |
|----|-------------|--------|--------|-----------------|
| NEW-39 | Fix 7 actively failing test packages | 05, F-033 | L | 1 |
| NEW-40 | Add test files for 5 highest-risk untested handlers | 05, F-027 | L | 1 |
| NEW-41 | Add runtime calibration test for scoring inflation | 04 | M | 1 |

### Thin Client (new tasks needed)

| ID | Description | Source | Effort | Suggested Phase |
|----|-------------|--------|--------|-----------------|
| NEW-42 | Add LUKS disk encryption to thin client build | 08-distro-audit | L | 4 |
| NEW-43 | Integrate 1Password CLI for secrets management | 08-distro-audit | M | 4 |
| NEW-44 | Extend hw-detect.sh for dual RTX 4090 detection | 08, F-046 | M | 4 |
| NEW-45 | Implement greetd/tuigreet boot-to-TUI pipeline | 12-thin-client | M | 4 |
| NEW-46 | Evaluate Cage compositor for fleet workers | 12-thin-client | S | 4 |

---

## 4. Stale ROADMAP Entries

### Phase 9 file-location drift was misread as missing implementation (C-07)

Phase 9 is marked 100% complete (5/5 tasks). The earlier research correctly found that the
roadmap points at `internal/session/merge.go`, `cycle_plan.go`, `scheduler.go`, `baseline.go`,
and `internal/mcpserver/tools_loop.go`, but that was a documentation problem rather than a
missing-implementation problem.

Current code reality:
- `finding_to_task`, `cycle_baseline`, `cycle_plan`, `cycle_merge`, and `cycle_schedule`
  are implemented in `internal/mcpserver/handler_rdcycle.go`
- the tools are registered in `internal/mcpserver/tools_builders_misc.go` under `rdcycle`
- `tools_loop_test.go` covers loop lifecycle handlers, not missing tier-1 rdcycle contracts

**Action:** Keep Phase 9 closed on the missing-file question, update roadmap/docs to the
consolidated handler location, and continue adding focused behavior tests around the existing
rdcycle handlers.

### Completed tasks referencing missing files (F-037)

5 completed tasks reference files that do not exist on disk:
- **QW-7** references `internal/session/snapshot.go` -- file missing. Snapshot functionality
  may have been consolidated into `checkpoint.go` but the task's acceptance path is wrong.
- **QW-11** references `internal/fleet/coordinator.go` -- file does not exist as a single
  module; coordinator logic is spread across `queue.go`, `worker.go`, `sharding.go`.
- **0.5.7.1** references `internal/version/version.go` -- needs verification.
- **0.5.11.1** references `internal/model/config_schema.go` -- needs verification.
- **1.8.4** references `internal/errors/` package -- needs verification.

**Action:** Verify whether these files exist under different names. If the functionality
exists elsewhere, update the ROADMAP references. If the functionality is missing, reopen
the tasks.

### i3-specific tasks superseded by Sway/Wayland migration

The ROADMAP and research jointly indicate that i3 (X11) is being superseded:
- 12-thin-client-patterns explicitly recommends dropping i3 support
- Phase 3.5 already completed Sway as PRIMARY COMPOSITOR
- The Manjaro migration (Deliverable 2 header still says "Ubuntu 24.04-based")

Stale i3-specific tasks that should be archived:
- 3.1.4 (i3 monitor enumeration via i3 IPC) -- Sway equivalent exists in 3.5.10
- 3.1.5 (i3 event listener) -- Sway events handled by 3.5.9
- 3.2.3/3.2.4 (`:layout` TUI commands) -- still needed but should target Sway, not i3
- 3.4.1-3.4.4 (autorandr integration) -- autorandr is X11-specific; Kanshi for Wayland
- 4.2.1-4.2.6 (i3 kiosk configuration) -- Sway kiosk already done in 3.5.7

**Action:** Archive i3-specific tasks, update 3.2.x to target WM-agnostic interface,
replace autorandr with Kanshi for Wayland hot-plug.

### Deliverable 2 description outdated

ROADMAP line 18: "Featherweight, low-graphics bootable Linux (Ubuntu 24.04-based) that
boots into i3 + the ralphglasses TUI."

Reality: Manjaro Linux with Sway is now the primary compositor. The Dockerfile.manjaro
is the active build. Ubuntu and i3 are legacy fallbacks.

**Action:** Update Deliverable 2 description to reflect Manjaro + Sway primary, with
Hyprland as opt-in alternative.

### Phase name "Advanced Fleet Intelligence" used twice (F-062)

Phase 6 (50 tasks, 84% done) and Phase 15 (32 tasks, 0% done) share the same name.

**Action:** Rename Phase 15 to "Distributed Swarm Intelligence & Scheduling" per the
01-roadmap-matrix recommendation.

### Stale header statistics (C-02, F-039)

ROADMAP line 7: "1,115 tasks, 442 complete (39.6%)"
Live count: 1,143 tasks, 503 complete (44.0%).
Discrepancy: 28 tasks and 61 completions added since the header was last updated.

ROADMAP line 7: "126 MCP tools (124 namespace + 2 meta), 19 TUI views"
Actual: 166 MCP tools across 16 namespaces (C-01).

ROADMAP line 7: "73 packages"
Codebase Statistics section (line 2701) says 37 packages.

**Action:** Update header to: "1,143 tasks, 503 complete (44.0%)" and "166 MCP tools
(164 namespace + 2 meta), 16 namespaces, 19 TUI views". Verify package count.

### Provider Capability Matrix stale pricing

ROADMAP line 1486: Claude Opus 4.6 listed as $15/$75.
Actual (F-073, C-06): Opus 4.6 is $5/$25 (67% price drop).

ROADMAP line 1486: Gemini Flash listed as $0.30/$2.50.
Actual (F-032, C-08): Gemini 2.5 Flash output is ~$3.50.

**Action:** Update Provider Capability Matrix to April 2026 pricing.

---

## 5. Proposed ROADMAP Amendments

### ADD

| ID | Task | Phase | Priority | Rationale |
|----|------|-------|----------|-----------|
| ADD-1 | Phase 0.95: Safety Hardening Sprint (NEW-1 through NEW-15) | 0.95 | P0 | 15 safety fixes prerequisite for any autonomy progression; blocks L1/L2 gates |
| ADD-2 | Fix 7 actively failing test packages | 1 | P0 | F-033: green CI is gate for all autonomous operation |
| ADD-3 | Add test files for 5 highest-risk untested handlers | 1 | P1 | F-027: handler_sweep_report.go (302 lines) has zero test coverage |
| ADD-4 | Fix 5 BLOCKER path traversal vulnerabilities | 1 (or 0.95) | P0 | F-006: security-critical, exploitable via MCP tool inputs |
| ADD-5 | Enforce mandatory $5 default budget floor | 10.5 | P0 | S3 Gap A: uncapped sessions possible without this |
| ADD-6 | Change sweep handler default $5.00 -> $0.50/session | 10.5 | P0 | S3 C-09: 10x cost surprise for autonomous agents |
| ADD-7 | Wire fleet CostPredictor.Record() to handleWorkComplete | 10.5 | P1 | F-042: fleet predictor starts empty every restart |
| ADD-8 | Update compiled-in provider cost rates | 10.5 | P1 | C-06/C-08: Opus 67% overestimate, Gemini 40% underestimate |
| ADD-9 | Stop active sessions on GlobalBudget exhaustion | 10.5 | P0 | S3 Gap B: overspend silently accepted |
| ADD-10 | Add per-hour spend circuit breaker | 13 | P0 | 14-autonomy-path: $1,280/hr max theoretical L3 spend |
| ADD-11 | Activate deferred loading in production binary | 1.5 | P1 | C-05: documented but not activated |
| ADD-12 | Delete FleetDashboardModel dead code (443 lines) | 1.5 | P2 | F-054: confirmed dead code |
| ADD-13 | Split advanced namespace into rc/autonomy/workflow/advanced | 1.5 | P1 | F-026: 24 tools across 7 domains |
| ADD-14 | Add LUKS disk encryption to thin client | 4 | P1 | 08-distro-audit: no disk encryption |
| ADD-15 | Extend hw-detect.sh for dual RTX 4090 | 4 | P1 | F-046: single GPU detection only |
| ADD-16 | Consolidate 4 CircuitBreaker implementations into 1 | 1.5 | P2 | F-043: duplicate code across 4 packages |
| ADD-17 | Per-provider circuit breakers (replace shared CB) | 10.5 | P1 | F-013/F-031: one flaky provider trips CB for all |
| ADD-18 | Add autonomy demotion circuit breaker for L1 | 13 | P0 | 14-autonomy-path: L1 exhausts retries silently |
| ADD-19 | Supervisor health self-check (tick latency, goroutine count) | 13 | P1 | 14-autonomy-path: degraded supervisor continues making decisions |
| ADD-20 | Git auto-revert on test regression for AutoMergeAll | 13 | P1 | 14-autonomy-path: no auto-revert mechanism for L3 |

### DELETE

| ID | Task | Phase | Rationale |
|----|------|-------|-----------|
| DEL-1 | 4.2.1-4.2.6 (i3 kiosk configuration, 6 tasks) | 4 | i3/X11 superseded by Sway; kiosk already done in 3.5.7 |
| DEL-2 | 3.4.1-3.4.4 (autorandr integration, 4 tasks) | 3 | autorandr is X11-only; replace with Kanshi for Wayland |
| DEL-3 | 3.1.4/3.1.5 (i3 IPC monitor/events, 2 tasks) | 3 | i3 IPC not needed; Sway equivalent complete in 3.5.x |
| DEL-4 | FleetDashboardModel (not a task, but dead code) | -- | F-054: 443 lines never instantiated |

### REPRIORITIZE

| ID | Task | Current Priority | New Priority | Rationale |
|----|------|-----------------|--------------|-----------|
| REP-1 | 1.5.11 (official SDK migration) | P1 XL | P1 XL (keep but defer to after 0.95) | Cannot safely migrate SDK before race fixes |
| REP-2 | 6.4.3 (OpenTelemetry traces) | P1 L | P2 L | OTel adds observability but is not safety-critical |
| REP-3 | 6.9.1-6.9.4 (NL fleet control) | P2 | P2 (defer to post-L2) | Nice-to-have; no safety or autonomy gate dependency |
| REP-4 | 10.5.6 (multi-node marathon) | P1 XL | P1 XL (defer to post-L3) | Single machine sufficient for initial 72h L3 target |
| REP-5 | Phase 7 (Kubernetes, 25 tasks) | P2 | P2 (defer to post-L3) | No L3 dependency; Manjaro thin client is target |
| REP-6 | Phase 9 tier-1 roadmap/docs references | P1 doc reconciliation | P1 doc reconciliation | Handler implementations exist; stale file references should not keep re-opening the phase |

---

## 6. Phase Resequencing

### Should safety hardening be consolidated into Phase 0.95?

**Yes.** The research makes a compelling case.

The 15 safety fixes identified in Section 3 (NEW-1 through NEW-15) are currently
scattered across:
- Phase 10.5 (race conditions R-01, R-02)
- Phase 9.5 (supervisor errors, GateEnabled)
- Phase 1 (path traversal, hook exit codes)
- Phase 13 (autonomy persistence)

None of these has an explicit ROADMAP task today. They are prerequisites for every
autonomy gate (L1, L2, L3) as defined in 14-autonomy-path.md.

**Proposed Phase 0.95: Safety Hardening Sprint**

```
Phase 0.95 — Safety Hardening Sprint [NEW]
Prerequisites: None (independent of all other phases)
Parallel workstreams: All items independent

0.95.1 — CRITICAL race fixes (R-01, R-02)               P0  S
0.95.2 — HIGH race fixes (R-03 through R-06)             P0  S-M
0.95.3 — MEDIUM race fixes (R-07 through R-12)           P1  S-M
0.95.4 — Path traversal vulnerability fixes (5 handlers) P0  L
0.95.5 — Supervisor error surfacing (3 levels)            P0  M
0.95.6 — RunLoop error propagation                        P0  S
0.95.7 — Hook exit code handling                          P0  S
0.95.8 — Autonomy persist retry                           P1  S
0.95.9 — Budget enforcement gap closures (A, B, C, D)     P0  M
0.95.10 — Sweep default budget correction ($5 -> $0.50)   P0  S

Acceptance: go test -race ./... -count=5 passes cleanly;
            no budget bypass path exists;
            all supervisor failures surface at Error level

Estimated effort: 2 weeks (matches Initiative 1 from 13-strategic-initiatives.md)
```

**Revised phase order for near-term execution:**

```
0.95 (Safety Hardening) ---> 1 (remaining: ParamParser, failing tests)
                                  |
                                  v
                             1.5 (remaining: Charm v2, SDK, dead code cleanup)
                                  |
                                  v
                             9 (reopen: tier-1 tool implementations)
                                  |
                                  v
                             10.5 (cost model + scaling)
                                  |
                                  v
                             13 (L3 autonomy core)
```

This matches the Initiative execution order from 13-strategic-initiatives.md:
Initiative 1 (Safety) -> Initiative 2 (Test Green) -> Initiative 3 (Cost Control) ->
Initiative 4 (MCP Modernization).

---

## 7. Metric Corrections

### Tool count: 126 -> 166

ROADMAP header (line 7) and CLAUDE.md both say 126 tools, 14 namespaces.
02-mcp-tool-audit counts 166 tools across 16 namespaces (C-01).
The `plugin` and `sweep` namespaces were added; several existing namespaces grew.

**Correction:** Update header to 166 tools, 16 namespaces. Update CLAUDE.md to match.

### Task count: 1,115 -> 1,143

ROADMAP header (line 7) says "1,115 tasks, 442 complete (39.6%)".
01-roadmap-matrix live grep: 1,143 tasks, 503 complete (44.0%) (C-02).
08-ralph-deep-dive: 503 done / 1,140 total (consistent with 01).

**Correction:** Update header to 1,143 tasks, 503 complete (44.0%).

### Phase count: 30 phases

ROADMAP has 36 numbered entries in the phase matrix (01-roadmap-matrix Section 1).
10 complete, 6 in-progress (15%-96%), 20 fully planned (0%).
Phase 0.7 merged into 0.6, so effective count is 35.

### Package count: 73 vs 37

ROADMAP header (line 7) says "73 packages".
Codebase Statistics section (line 2701) says "37 packages".
The 73 figure may include test packages or subdirectories. The 37 figure from the
snapshot section is more recent.

**Correction:** Reconcile and pick one authoritative count. The 37-package figure
appears to count only `internal/` subpackages; 73 may include `cmd/`, test packages,
and tool packages. Use whichever matches `go list ./...` output.

### Coverage: 84.5% reported, 83.4% in KPI table

ROADMAP Metrics table (line 1416) says 83.4%.
Codebase Statistics (line 2708) says 84.5%.
05-test-coverage says 84.5%.

**Correction:** Standardize on the most recent measurement. Note that 80 coverage-padding
files (807 Test* functions) may inflate the true effective coverage.

### Completion percentages needing correction

| Phase | ROADMAP Claim | Research Count | Corrected |
|-------|--------------|----------------|-----------|
| Phase 9 | 100% (5/5) | 100% implementation, stale file references in roadmap/docs (C-07) | 5/5 with doc reconciliation follow-up |
| Phase 3.5 | 80% (24/30) | 80% (01) or 93% (08) -- disagree (C-03) | 80% per 01 (checkbox recount) |
| Phase 10.5 | 29% (14/48) | 29% -- consistent | 29% |

---

## 8. Numbering Collision Fix

### Phase 3.5.5 collision (F-003)

Phase 3.5 has two sections both numbered 3.5.5:

**Instance A (line 809):** `3.5.5 -- Codex-primary command/control parity [NEW]`
- 8 subtasks (3.5.5.1-3.5.5.8), all now marked [x] (completed 2026-04-04)
- P0/P1 tasks about provider-neutrality in the enhancer stack

**Instance B (line 825):** `3.5.5 -- Theme export to terminal`
- 4 subtasks (3.5.5.1-3.5.5.4), all marked [x]
- P2 tasks about exporting themes to Ghostty/Starship/k9s

Both sections share identical sub-IDs (3.5.5.1, 3.5.5.2, etc.) for completely different
tasks. Any tooling addressing tasks by ID will hit ambiguity.

**Recommended resolution:**
1. Keep Instance B (theme export) as `3.5.5` -- it was there first and is complete.
2. Renumber Instance A (Codex parity) to `3.5.6 -- Codex-primary command/control parity`.
3. Renumber subtasks: 3.5.6.1 through 3.5.6.8.
4. Shift any existing 3.5.6 to 3.5.7 (there is no current 3.5.6, so this is clean).
5. Update any cross-references in docs or research reports.

This is the minimal fix. The 01-roadmap-matrix report independently recommends the
same resolution.

---

## 9. Summary of Actions

### Immediate (before next R&D cycle)

1. **Add Phase 0.95** with 10 safety hardening subtasks (15 individual fixes).
2. **Reconcile Phase 9 roadmap/docs references** to `internal/mcpserver/handler_rdcycle.go` and keep future file extractions explicit.
3. **Fix 3.5.5 collision** -- renumber Codex parity section to 3.5.6.
4. **Update header statistics** -- 1,143 tasks, 503 complete, 166 tools, 16 namespaces.
5. **Update Provider Capability Matrix** -- Opus $5/$25, Gemini Flash output $3.50.

### Short-term (next 2 weeks)

6. **Add 20 missing tasks** from Section 3 (safety, cost model, dead code, MCP).
7. **Archive 12 stale i3 tasks** from Phases 3 and 4.
8. **Rename Phase 15** to "Distributed Swarm Intelligence & Scheduling".
9. **Update Deliverable 2 description** to reflect Manjaro + Sway.
10. **Add autonomy gate cross-references** from 14-autonomy-path.md into ROADMAP phases.

### Medium-term (next month)

11. **Verify 5 completed tasks with missing files** (QW-7, QW-11, 0.5.7.1, 0.5.11.1, 1.8.4).
12. **Add 158 phantom file references** as warnings in affected phases.
13. **Integrate Initiative execution order** from 13-strategic-initiatives.md as a
    ROADMAP section for Q2-Q4 2026 planning.
