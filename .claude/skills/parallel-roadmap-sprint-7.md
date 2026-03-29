---
name: parallel-roadmap-sprint-7
description: Full-audit mega sprint — ROADMAP sync (mark 50+ done items), P0/P1 finding fixes, Phase 0.6/0.9 completion, coverage 84.6→90%, 54→<15 zero-coverage functions, Phase 1 hardening
user-invocable: true
argument-hint: [workstream-filter]
---

You are executing Sprint 7 for the ralphglasses project. This sprint combines a full codebase audit with remaining sprint coverage. Every workstream distinguishes **IMPLEMENTATION** tasks from **AUDIT** tasks and flags intersections.

## Project Context

Ralphglasses is a Go TUI + MCP server (125 tools, 14 namespaces) for managing parallel multi-LLM agent fleets. 37 packages, 84.6% test coverage, 37/37 packages green. Uses Charmbracelet BubbleTea v1, mark3labs/mcp-go v0.45.0, modernc.org/sqlite.

**Sprints 1-6 completed:** QW-1 through QW-11 implemented (all unchecked in ROADMAP — marking needed), loop engine fixes, MCP handler audit, enhancer recalibration, provider hardening, observation enrichment, worktree/dedup fixes, TUI hardening, 81 zero-coverage tests. 56 files, +3768 lines in Sprint 6.

**Current state:**
- ROADMAP: 191/471 checked (40.6%) — but ~50 items are implemented and unchecked
- Coverage: 84.6% overall, 54 functions at 0%
- Scratchpad: ~249 open findings (3 P0, 14+ P1, fix rate 9.8%)
- Phases at 100%: 0, 0.8, 2.5, 2.75
- Phase 0.5: 97% (44/45) — QW items all implemented but unchecked
- Phase 0.9: 0% (0/12) — same QW items, all implemented but unchecked
- Phase 0.6: 0% (0/39) — many partially implemented in Sprint 5/6
- Phase 1: 58% (32/55) — 23 remaining

## Pre-flight

Read these files FIRST to confirm current state. Do NOT skip this step:
- `ROADMAP.md` — Identify all `[ ]` items that are actually implemented
- `.ralph/tool_improvement_scratchpad.md` — P0/P1 findings for your workstream
- Run `go tool cover -func=coverage.out | awk '$NF == "0.0%"'` to confirm current zero-coverage list

---

## Epic 1: ROADMAP Truth Sync + Phase Completion (AUDIT + IMPLEMENTATION)

### WS-1: ROADMAP Checkbox Sync + Phase 0.6 Partial Completion
**Goal:** Mark all implemented-but-unchecked items. Complete Phase 0.5 to 100%, Phase 0.9 to 100%, Phase 0.6 to >60%.
**Files:** `ROADMAP.md` (ONLY file this WS modifies)

**Step 1 — AUDIT: Verify each QW item is implemented (cross-reference code):**

| QW | File | What to verify | Expected |
|----|------|----------------|----------|
| QW-1 | `session/loop_steps.go:372` | `looksLikeJSON()` + `json.Valid()` pre-validation | Present |
| QW-2 | `session/cascade.go:27` | `Enabled: true` in DefaultCascadeConfig | Present |
| QW-3 | `session/loop.go:166` | `MaxWorkerTurns` default 20 enforcement | Present |
| QW-4 | `enhancer/scoring.go:564` | `scoreTone()` baseline 50 | Present |
| QW-5 | `enhancer/pipeline.go:232` | Skip reason in improvements array | Present |
| QW-6 | `session/loop_gates.go:17` | `InitBaselineFromFirstObservation()` | Present |
| QW-7 | `mcpserver/tools_session.go:21` | `resolveSnapshotRepo` longest-path | Present |
| QW-8 | `mcpserver/handler_session_lifecycle.go:63` | `budget_usd`/`max_turns` propagation | Present |
| QW-9 | `session/autooptimize.go:55` | `PersistAutonomyLevel()` | Present |
| QW-10 | `mcpserver/handler_roadmap.go:103` | `relevanceScore()` Jaccard similarity | Present |
| QW-11 | `fleet/queue.go:34` | `PushValidated()` path validation | Present |

**Step 2 — IMPLEMENTATION: Check off verified items in ROADMAP.md:**
- Mark QW-1 through QW-11 as `[x]` in Phase 0.9 section
- Mark corresponding Phase 0.5 items as `[x]`
- Scan Phase 0.6 for items already implemented in Sprint 5/6:
  - 0.6.1.1 discovery tests → check `internal/discovery/discovery_test.go` exists ✓
  - 0.6.1.4 enhancer edge tests → check `internal/enhancer/pipeline_edge_test.go` exists ✓
  - 0.6.2.4 SummarizeObservations → check `session/loop_helpers.go:EnrichObservationSummary` ✓
  - 0.6.3.2 model name validation → check `session/loop_validate.go:ValidateModelName` ✓
  - 0.6.4.1/4.2 FormatGateReport → check `e2e/format.go` ✓
  - 0.6.4.3 CompareGateReports → check `e2e/gates.go:CompareGateVerdicts` ✓
  - 0.6.5.1-5.4 StallDetector → check `session/stall.go` ✓
  - 0.6.6.1-6.3 worktree cleanup → check `session/worktree.go` lock + dirty detection ✓
  - 0.6.7.1-7.4 dedup Jaccard → check `session/dedup.go:IsSimilarTaskWithReason` ✓
- Scan Phase 1 for already-done items and mark them

**Step 3 — AUDIT: Count final phase completion percentages after marking.**

**Verification:** Diff ROADMAP.md to confirm only checkbox changes (`[ ]` → `[x]`), no content modifications.

---

## Epic 2: P0/P1 Finding Fixes (IMPLEMENTATION)

### WS-2: Critical Path Finding Fixes
**Goal:** Fix the 3 P0 and 8 highest-impact P1 findings.
**Files:** Various handler/session files (see per-finding assignments below)

**P0 Fixes (multi-cycle regressions — BLOCKERS):**

**FINDING-268: snapshot saves to wrong repo path**
- File: `internal/mcpserver/tools_session.go` — `resolveSnapshotRepo()`
- AUDIT: Sprint 6 added longest-path matching. Verify it handles the claudekit/ralphglasses ambiguity case. Test with CWD inside ralphglasses when claudekit is also scanned.
- IMPLEMENTATION: If still broken, add path-separator boundary check. Add test for nested repo disambiguation.

**FINDING-160: sessions killed in <1s (signal:killed)**
- File: `internal/session/runner.go` — `runSession()`
- AUDIT: Check startup probe timeout. Check if MaxWorkerTurns=20 (QW-3) helps.
- IMPLEMENTATION: Add configurable `MinSessionDuration` (default 30s) — sessions younger than this are not killed by reaper. Add structured error output on early termination.

**FINDING-169: logs NO_LOG_FILE error when loop exists**
- File: `internal/mcpserver/handler_repo.go` — `handleLogs()`
- AUDIT: Sprint 6 added graceful empty response. Verify the fix works when ralph.log doesn't exist but `.ralph/logs/` directory does.
- IMPLEMENTATION: If still broken, check both `ralph.log` and `logs/*.log` fallback paths.

**P1 Fixes (high impact):**

**FINDING-237: fleet_analytics all-zero metrics**
- File: `internal/mcpserver/tools_fleet.go`
- AUDIT: Sprint 6 made nil-safe. Check if the analytics data source (observation_summary) actually feeds into fleet analytics.
- IMPLEMENTATION: Wire observation data into fleet analytics aggregation.

**FINDING-240: prompt_analyze score inflation (97/A with C/D/F dimensions)**
- AUDIT: Sprint 6 recalibrated scoring.go baselines. Run `go test ./internal/enhancer/ -run TestScoringCorpus -v` to verify terrible prompts now score <50.
- IMPLEMENTATION: If overall score still inflated, add weighted average where overall = weighted mean of dimensions (not max).

**FINDING-186: loop defaults to unavailable models**
- AUDIT: Sprint 6 changed `o1-pro` → `gpt-4o`. Verify `BudgetOptimizedSelfImprovementProfile()` also uses valid models.
- File: `internal/session/loop_types.go`
- IMPLEMENTATION: Fix `BudgetOptimizedSelfImprovementProfile()` if it still references `o1-pro` or `gpt-5.4-xhigh`.

**FINDING-176/187: null arrays instead of empty arrays**
- AUDIT: Run `grep -rn 'null' internal/mcpserver/ | grep -i 'array\|slice\|list'` to find remaining null-array sources.
- File: `internal/mcpserver/handler_fleet.go`, `internal/mcpserver/tools_fleet.go`
- IMPLEMENTATION: Initialize all response slices with `make([]T, 0)` in handlers that return lists.

**FINDING-173: fleet_status 128K char output overflow**
- File: `internal/mcpserver/handler_fleet.go` or `internal/mcpserver/tools_fleet.go`
- IMPLEMENTATION: Add `limit` parameter (default 50 repos). Truncate session lists. Add `total_count` field for pagination awareness.

**FINDING-199: roadmap_expand 179K char output**
- File: `internal/mcpserver/handler_roadmap.go`
- IMPLEMENTATION: Enforce `limit` parameter on output. Truncate task descriptions to 500 chars. Add summary mode.

**Verification:** `go build ./...` && `go test ./internal/mcpserver/... ./internal/session/... -count=1`

---

## Epic 3: Coverage Uplift 84.6% → 90% (IMPLEMENTATION)

### WS-3: Session Package Zero-Coverage Functions (highest impact)
**Goal:** Test the 12 zero-coverage functions in `internal/session/`.
**Files:** NEW test files in `internal/session/`

**Target functions (from coverage.out):**

| Function | File | Line | Strategy |
|----------|------|------|----------|
| `aggregateLoopSpend` | loop.go | 332 | Test with mock observations, verify sum |
| `handleSelfImprovementAcceptance` | loop_acceptance.go | 16 | Test acceptance logic with mock iteration |
| `handleSelfImprovementAcceptanceTraced` | loop_acceptance.go | 21 | Test traced variant |
| `enhanceForProvider` | loop_helpers.go | 225 | Test provider-specific enhancement (Claude vs Gemini) |
| `BudgetOptimizedSelfImprovementProfile` | loop_types.go | 180 | Test profile defaults |
| `Init` | manager.go | 116 | Test manager initialization with mock store |
| `Resume` | manager_lifecycle.go | 98 | Test session resume with mock state |
| `AddTeamForTesting` | manager_subsystem.go | 198 | Test team addition |
| `gitDiffPaths` | protection.go | 22 | Test with temp git repo |
| `buildCmdForProvider` | providers.go | 163 | Test command construction per provider |
| `startupProbe` | runner.go | 168 | Test probe with mock process |
| `Rules` | reflexion.go | 207 | Test rule extraction from entries |

For each: write a focused test using existing test helpers and mocks. Use `t.TempDir()` for any git repo needs.

**Verification:** `go test ./internal/session/... -count=1 -coverprofile=session_cov.out && go tool cover -func=session_cov.out | awk '$NF == "0.0%"' | wc -l` — target: 0 zero-coverage functions in session package.

### WS-4: MCP Handler + Fleet + E2E Zero-Coverage (second highest impact)
**Goal:** Test 15 zero-coverage functions across mcpserver, fleet, e2e.
**Files:** NEW test files in respective packages

**Target functions:**

| Function | File | Line | Strategy |
|----------|------|------|----------|
| `InitFleetTools` | mcpserver/handler_fleet.go | 16 | Test fleet tool registration |
| `pruneLoopRunsFiltered` | mcpserver/handler_loop.go | 262 | Test with temp loop run files |
| `buildSummary` | mcpserver/handler_mergeverify.go | 41 | Test summary construction |
| `parseCoverageTotal` | mcpserver/handler_mergeverify.go | 142 | Test coverage line parsing |
| `mapSessionProvider` | mcpserver/handler_prompt.go | 369 | Test provider string mapping |
| `handleToolBenchmark` | mcpserver/tools_fleet.go | 442 | Test with mock benchmark data |
| `handleAwesomeFetch` | mcpserver/handler_awesome.go | 16 | Test with mock HTTP |
| `handleAwesomeAnalyze` | mcpserver/handler_awesome.go | 27 | Test with mock data |
| `RefreshBaseline` | e2e/baseline.go | 147 | Test with temp repo |
| `RunE2EGate` | e2e/gates.go | 293 | Test gate evaluation |
| `runIteration` | e2e/selftest.go | 191 | Test with mock session |
| `NewRemoteA2AAdapter` | fleet/a2a_card.go | 141 | Test adapter construction |
| `GetHostname` | fleet/discovery.go | 104 | Test hostname retrieval |
| `handleEventStream` | fleet/server_handlers.go | 270 | Test SSE handler with httptest |
| `Run/heartbeatLoop/pollLoop/executeWork` | fleet/worker.go | 48-136 | Test with mock bus/manager, context cancel |

**Verification:** `go test ./internal/mcpserver/... ./internal/fleet/... ./internal/e2e/... -count=1`

### WS-5: Remaining Zero-Coverage + Low-Coverage Package Uplift
**Goal:** Test remaining 27 zero-coverage functions across cmd, bandit, batch, plugin, process, tracing, tui, wsclient.
**Files:** NEW or modified test files in respective packages

**Target functions:**

| Function | File | Strategy |
|----------|------|----------|
| `main` (3 instances) | main.go, cmd/*/main.go | Skip — untestable entry points |
| `Execute` | cmd/root.go:199 | Test with mock cobra setup |
| `runImprove` | cmd/prompt-improver/main.go:305 | Test with mock API client |
| `runHook` | cmd/prompt-improver/main.go:488 | Test hook execution path |
| `runMCP` | cmd/prompt-improver/mcp.go:191 | Test MCP server startup/shutdown |
| `SelectRandom` | bandit/bandit.go:114 | Test random arm selection |
| `WithHTTPClient` | batch/batch.go:119 | Test option application |
| `Cancel` | batch/gemini.go:228 | Test batch cancellation |
| `LoadDirManifests` | plugin/loader.go:65 | Test with temp plugin dir |
| `migrateToJSON` | process/pidfile.go:378 | Test legacy format migration |
| `collectSessionChildPIDs` | session/childpids.go:13 | Test PID collection |
| `CreateReviewPR` | session/acceptance.go:288 | Test with mock git |
| `EndSessionSpan` | tracing/tracing.go:145 | Test span operations |
| `RecordTurnMetric` | tracing/tracing.go:148 | Test metric recording |
| `RecordError` | tracing/tracing.go:152 | Test error recording |
| `RecordCostMetric` | tracing/tracing.go:155 | Test cost recording |
| `ScrollDown` | tui/views/logstream.go:98 | Test scroll behavior |
| `PageDown` | tui/views/logstream.go:122 | Test page behavior |
| `renderTaskDistribution` | tui/views/loophealth.go:173 | Test rendering |
| `percentile` | tui/views/repodetail.go:243 | Test percentile calculation |
| `WithHTTPClient` | wsclient/wsclient.go:101 | Test option application |

**Subtract 3 untestable `main()` functions: 24 testable functions → target all 24.**

**Verification:** `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | awk '$NF == "0.0%"' | wc -l` — target: ≤3 (only main() functions)

---

## Epic 4: Full Codebase Audit (AUDIT)

### WS-6: Security + API Contract Audit
**Goal:** Systematic audit across 6 dimensions. Output as categorized findings.
**Files:** Read-only audit — produce findings report, NO code changes.

**Dimension 1 — Input Validation (OWASP focus):**
- Grep for unsanitized user input flowing into `exec.Command`, `os.Open`, SQL queries
- Check all MCP tool handlers for path traversal via `repo` parameter
- Check `internal/mcpserver/handler_*.go` for missing parameter validation
- Categorize: BLOCKER (exploitable), WARNING (defense-in-depth), INFO (theoretical)

**Dimension 2 — API Contract Correctness:**
- Compare MCP tool schemas (registered in `tools_*.go`) against actual handler parameter usage
- Find parameters registered in schema but never read by handler
- Find parameters read by handler but not in schema (undocumented)
- Check return types: null vs empty array, missing fields, inconsistent error formats

**Dimension 3 — Dead Code Detection:**
- `go vet ./...` for unused variables
- Grep for exported functions with zero callers (excluding test files and MCP registrations)
- Check for dead feature flags or unreachable code paths

**Dimension 4 — Dependency Hygiene:**
- `go mod tidy` — check for unused deps
- Check for pinned vs floating versions
- Flag any deps with known CVEs (check go.sum against govulncheck if available)

**Dimension 5 — Error Handling Consistency:**
- Grep for `fmt.Errorf` without `%w` (unwrapped errors)
- Find error returns that discard context (bare `return err` without wrapping)
- Check for swallowed errors (`_ = someFunc()` on fallible operations)

**Dimension 6 — Performance Hotspots:**
- Identify unbounded slice allocations in hot paths (loop step, observation query)
- Check for missing context.Context propagation in blocking operations
- Find goroutine leaks (goroutines started without cleanup mechanism)

**Output format:** Write findings to `.ralph/sprint7_audit_report.md` with:
```
## [Dimension Name]

### BLOCKER
- [file:line] Description. Impact. Suggested fix.

### WARNING
- [file:line] Description.

### INFO
- [file:line] Description.
```

**Verification:** Report file exists and contains categorized findings for all 6 dimensions.

### WS-7: Test Quality + Flakiness Audit
**Goal:** Audit test suite health. Fix flaky tests.
**Files:** Read + fix test files

**Audit tasks:**
1. Run `go test ./... -count=5 -timeout 180s 2>&1 | grep FAIL` — identify flaky tests
2. Grep test files for `time.Sleep` — potential flakiness source
3. Grep for tests using real filesystem without `t.TempDir()` — test pollution risk
4. Check for tests modifying package-level globals without mutex — race conditions
5. Grep for `t.Parallel()` in tests that share state
6. Verify all test helpers use `t.Helper()`

**Fix tasks (IMPLEMENTATION where audit intersects):**
- Fix any flaky tests found in run-5
- Add missing `t.Helper()` to test helpers
- Replace raw `os.MkdirTemp` with `t.TempDir()` where found
- Guard global mutation with sync.Mutex (like Sprint 6's root_test.go fix)

**Verification:** `go test ./... -count=3 -timeout 180s -race` — all green, no races.

---

## Epic 5: Phase 0.6 Implementation Gap Closure

### WS-8: Phase 0.6 Remaining Implementation
**Goal:** Implement the Phase 0.6 items NOT already done.
**Files:** Various (see task list)

**Read ROADMAP.md Phase 0.6 after WS-1 marks done items. Focus on what's left:**

**Likely remaining (not done in Sprint 5/6):**
1. **0.6.1.2** — model/ corrupt file tests (truncated JSON, invalid UTF-8)
2. **0.6.1.3** — process/ edge case tests (double-stop, stop-before-start)
3. **0.6.2.1-2.3** — LoopObservation enrichment fields (GitDiffStat, PlannerModelUsed, WorkerModelUsed, AcceptancePath) — struct fields may exist but not populated
4. **0.6.3.1** — `ValidateLoopConfig()` comprehensive validation (beyond model names)
5. **0.6.3.3** — Enhancement flag validation (warn if non-Claude worker with enhancement)
6. **0.6.3.4** — Wire config validation into loop start
7. **0.6.4.4** — Wire FormatGateReport into loop_gates MCP tool response
8. **0.6.5.3** — StallCount field in LoopObservation
9. **0.6.6.4** — `ralphglasses_worktree_cleanup` MCP tool (add to tools_observability.go)
10. **0.6.7.2** — Track completed tasks in observation history, reject re-proposals
11. **0.7 items** — These are rollups of 0.6 subitems; mark as done when subitems are done

For each: verify current state (may already be done), implement if missing, add test.

**Verification:** `go build ./...` && `go test ./... -count=1 -timeout 120s`

---

## Execution Plan

```
Phase 1 — Parallel (8 worktree agents):

  WS-1 (ROADMAP sync)   WS-2 (P0/P1 fixes)    WS-3 (session tests)   WS-4 (handler tests)
  ──────────────────     ──────────────────      ──────────────────     ────────────────────

  WS-5 (remaining tests)  WS-6 (security audit)  WS-7 (test quality)   WS-8 (Phase 0.6)
  ─────────────────────   ────────────────────    ────────────────────  ─────────────────
       ↘                          ↓                      ↓                    ↙
        └─────────────────────────┴──────────────────────┴───────────────────┘
                                          ↓
Phase 2 — Sequential:               Integration + merge
                                          ↓
                                 Final verification
                                          ↓
                                    Commit & report
```

## Resource Conflict Matrix

| File Area | WS-1 | WS-2 | WS-3 | WS-4 | WS-5 | WS-6 | WS-7 | WS-8 |
|-----------|------|------|------|------|------|------|------|------|
| `ROADMAP.md` | **W** | | | | | | | |
| `mcpserver/tools_session.go` | | **W** | | | | R | | |
| `mcpserver/handler_fleet.go` | | **W** | | | | R | | |
| `mcpserver/handler_roadmap.go` | | **W** | | | | R | | |
| `session/runner.go` | | **W** | | | | R | | |
| `session/loop_types.go` | | **W** | | | | | | |
| `session/*_test.go` (NEW) | | | **W** | | | | R | |
| `mcpserver/*_test.go` (NEW) | | | | **W** | | | R | |
| `fleet/*_test.go` (NEW) | | | | **W** | | | R | |
| `e2e/*_test.go` (NEW) | | | | **W** | | | R | |
| `cmd/*_test.go` (NEW) | | | | | **W** | | R | |
| `tui/views/*_test.go` (NEW) | | | | | **W** | | R | |
| `.ralph/sprint7_audit_report.md` (NEW) | | | | | | **W** | | |
| `session/loop_observation.go` | | | | | | | | **W** |
| `session/loop_validate.go` | | | | | | | | **W** |
| `mcpserver/tools_observability.go` | | | | | | | | **W** |

**Key conflicts:**
- WS-2 and WS-8 both touch session package source files but different functions. Merge WS-2 first.
- WS-3/4/5 create NEW test files only — no overlap with source-modifying WS.
- WS-6 is READ-ONLY audit — no write conflicts.
- WS-7 may fix existing test files that WS-3/4/5 also create. Merge WS-3/4/5 before WS-7.

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| WS-1 marks items as done that aren't | Medium | High | Verify each item by reading source code |
| WS-2 P0 fixes break existing behavior | Medium | High | Backward-compatible changes only; add tests |
| WS-3/4/5 coverage tests are shallow | Medium | Low | Each test must exercise actual logic, not just call the function |
| WS-6 audit finds BLOCKER in hot path | Low | High | Flag for immediate fix; don't ship with known blockers |
| WS-7 flaky test fix changes test semantics | Low | Medium | Preserve assertions; only change timing/isolation |
| WS-8 observation enrichment fields break JSON compat | Medium | Medium | Add fields with `omitempty`; never remove fields |
| Worktree transcript extraction needed again | Medium | Medium | Check for NEW (untracked) files in worktrees BEFORE cleanup |

## Acceptance Criteria

| WS | Type | Metric | Target |
|----|------|--------|--------|
| 1 | AUDIT+IMPL | Phase 0.5 completion | 100% (45/45) |
| 1 | AUDIT+IMPL | Phase 0.9 completion | 100% (12/12) |
| 1 | AUDIT+IMPL | Phase 0.6 checked items | >60% (>23/39) |
| 2 | IMPL | P0 findings fixed | 3/3 |
| 2 | IMPL | P1 findings fixed | ≥6 of 14 |
| 3 | IMPL | session/ zero-coverage functions | 0 |
| 4 | IMPL | mcpserver+fleet+e2e zero-coverage | ≤3 |
| 5 | IMPL | Total zero-coverage functions | ≤3 (main() only) |
| 6 | AUDIT | Audit dimensions covered | 6/6 |
| 6 | AUDIT | BLOCKER findings documented | All |
| 7 | AUDIT | Flaky tests identified + fixed | All found |
| 8 | IMPL | Phase 0.6 remaining items done | ≥8 of remaining |
| ALL | IMPL | Overall coverage | ≥90% (from 84.6%) |
| ALL | IMPL | Test suite | 37/37 green, -race clean |

## Constraints

- Non-overlapping file ownership per workstream
- WS-6 is READ-ONLY — produces report, no source changes
- WS-1 modifies ONLY ROADMAP.md — no source changes
- Do NOT add external dependencies
- Every workstream must pass `go build ./...` && `go vet ./...`
- Use `/bin/cp` (not `cp`) when copying between worktrees (macOS alias)
- **CRITICAL:** Before removing worktrees, check for untracked files with `git -C <worktree> status --short` and copy NEW files first
- If `$ARGUMENTS` filter present, only run specified workstreams
- Backward-compatible changes only — add fields with `omitempty`, never remove
- Coverage tests must exercise real logic, not just achieve line coverage
- Read ROADMAP.md FIRST for each WS — many items are already done from Sprints 1-6

## Post-Sprint

After all workstreams complete:
1. `go build ./...` && `go vet ./...`
2. `go test ./... -count=1 -timeout 120s -race` — full green, no races
3. Coverage: `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | tail -1`
4. Zero-coverage: `go tool cover -func=coverage.out | awk '$NF == "0.0%"' | wc -l`
5. ROADMAP check: count `[x]` vs total, report per-phase deltas
6. Review `.ralph/sprint7_audit_report.md` — flag any BLOCKERs for immediate action
7. Update CLAUDE.md if tool count changes
8. Single commit with Sprint 7 deliverables
9. Report: phase completion deltas, findings fixed, coverage delta, audit summary
