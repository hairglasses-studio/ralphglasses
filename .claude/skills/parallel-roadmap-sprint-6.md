---
name: parallel-roadmap-sprint-6
description: Mega sprint — codebase audit fix (157→<100 findings) + coverage uplift (84.4→88%), Phase 0.6/0.9 completion, 74 zero-coverage functions → <40, race fix, PID wiring
user-invocable: true
argument-hint: [workstream-filter]
---

You are executing Sprint 6 for the ralphglasses project. This sprint combines two goals: (1) completing remaining sprint coverage items and (2) performing a simultaneous full codebase audit to fix open scratchpad findings.

## Project Context

Ralphglasses is a Go TUI + MCP server (125 tools, 14 namespaces) for managing parallel multi-LLM agent fleets. 37 packages, 84.4% test coverage, 37/37 packages green. Uses Charmbracelet BubbleTea v1, mark3labs/mcp-go v0.45.0, modernc.org/sqlite.

**Sprints 1-5 completed:** 12 QW bug fixes, doc sync, self-improvement pipeline, 10 rdcycle tools, score hardening, fleet reaper, cascade default, autonomy persistence, ParamParser, loop validation, gate reports, stall callbacks, SQLite loop_runs/cost_ledger, rdcycle tests (0→88%), FINDING-55 fix, config deprecation, loop store persistence, session history, shell completion, envkit coverage (44→91%), Phase 0.5 completion (98%), Phase 1 hardening (58%), Phase 2.5 completion (100%), PID file infrastructure, batch collector, sentinel errors, doctor/init/debug-bundle commands.

**Current state:** 32.5% roadmap completion (201/619 tasks). 84.4% overall test coverage. 74 functions at 0% coverage. 157 open scratchpad findings.

**Phase status:**
- Phase 0.5: 98% (44/45) — 1 remaining
- Phase 0.6: 0% (0/47) — NEW FOCUS
- Phase 0.9: 0% (0/12) — Quick Wins
- Phase 1: 58% (32/55) — 23 remaining
- Phase 1.5: 12% (6/52)
- Phase 2: 11% (8/70)

**Packages below target:**
- `cmd/ralphglasses-mcp`: 59.1%
- `cmd/prompt-improver`: 70.0%
- `cmd`: 74.5%
- `internal/tui`: 80.2%
- `internal/mcpserver`: 81.6%
- `internal/session`: 83.4%

## Pre-flight

Read these files to confirm current state:
- `ROADMAP.md` — Phase 0.6, 0.9, and Phase 1 unchecked items
- `.ralph/tool_improvement_scratchpad.md` — 157 open findings, prioritize P0/P1
- `internal/session/loop_types.go` — DefaultLoopProfile model names (FINDING-186)
- `internal/mcpserver/handler_session_lifecycle.go` — budget_usd wiring (FINDING-258/261)
- `internal/enhancer/scoring.go` — Score inflation (FINDING-240)
- `internal/session/batch.go` — Webhook retry missing (FINDING-66)
- `cmd/root_test.go` — Race condition (FINDING-62)

## Epic 1: P0 Audit Fixes — Loop & Handler Bugs

### WS-1: Loop Engine Fixes + QW-1/3/6/8
**Goal:** Fix critical loop bugs and complete Quick Win items related to loop behavior.
**Files:** `internal/session/loop_types.go`, `internal/session/loop.go`, `internal/session/loop_steps.go`, `internal/session/loop_gates.go`, `internal/session/loop_validate.go`

1. **FINDING-186 (P0):** In `DefaultLoopProfile()`, replace unavailable model names (o1-pro, gpt-5.4-xhigh) with currently available models. Add `ValidateModelName(provider, model string) error` in `loop_validate.go` with known model lists per provider.
2. **FINDING-267 (P1):** In `StartLoop()`, add auto-prune: if `PruneLoopRuns()` finds >200 stale runs, prune before starting. Log pruned count.
3. **FINDING-187 (P1):** Audit all `[]LoopIteration` and `[]string` fields in loop types. Ensure JSON marshaling produces `[]` not `null` — initialize with `make([]T, 0)`.
4. **QW-1 (P0):** In `loop_steps.go` worker output parsing, add JSON schema pre-validation. If parse fails, inject retry prompt with format reminder.
5. **QW-3 (P0):** Ensure `MaxWorkerTurns` defaults to 20 in `normalizeLoopProfile()`. Already enforced in `RunLoop` — verify.
6. **QW-6 (P0):** In `loop_gates.go`, fix zero-baseline: initialize from first observation, not zero values.
7. **QW-8 (P0):** In `StartLoop`, verify `budget_usd` param propagates to `BudgetEnforcer`. Trace the path from `LoopProfile.WorkerBudgetUSD` through to `checkLoopBudget`.

**Verification:** `go test ./internal/session/... -count=1 -race -timeout 120s`

---

### WS-2: MCP Handler Audit Fixes + QW-7
**Goal:** Fix handler-level bugs in snapshot, logs, session_launch, and fleet analytics.
**Files:** `internal/mcpserver/tools_session.go`, `internal/mcpserver/handler_repo.go`, `internal/mcpserver/handler_session_lifecycle.go`, `internal/mcpserver/tools_fleet.go`

1. **FINDING-268/QW-7 (P0):** In snapshot repo resolution, fix CWD fallback that matches parent repos. Use `filepath.Base(r.Path)` matching or require explicit `repo` param.
2. **FINDING-169 (P0):** In `handleLogs`, when log file missing, return `{"entries": [], "message": "no log file found"}` instead of error.
3. **FINDING-258/261 (P1):** In `handleSessionLaunch`, verify `budget_usd` and `max_turns` propagate through `Manager.LaunchSession` to `BudgetEnforcer`. Add integration test.
4. **FINDING-237 (P1):** In `handleFleetAnalytics`, when fleet not initialized, return `{"warning": "fleet not initialized", "analytics": {}}` instead of all-zeros.

**Verification:** `go build ./...` && `go test ./internal/mcpserver/... -count=1 -race`

---

## Epic 2: Scoring, Provider & Batch Fixes

### WS-3: Enhancer Score Recalibration + QW-4/5
**Goal:** Fix prompt_analyze score inflation and stage skipping transparency.
**Files:** `internal/enhancer/scoring.go`, `internal/enhancer/scoring_test.go`, `internal/enhancer/pipeline.go`, `internal/enhancer/pipeline_format.go`, `internal/enhancer/pipeline_edge_test.go`

1. **FINDING-240/QW-4 (P1):** Recalibrate `Score()`: lower base scores from ~85 to ~60, add positive signals that earn points, add negative signals for lack of examples/context/vague language. Test with corpus of known-bad (expect 30-50) and known-good (expect 75-90) prompts.
2. **QW-5 (P1):** Add `SkippedStages []SkippedStage` to `EnhanceResult`. In each pipeline stage, record skip reason when stage is conditionally skipped.
3. **Phase 0.6.1.4:** Add edge tests: empty input, >100K chars, unicode-heavy prompts, prompts with XML tags.

**Verification:** `go test ./internal/enhancer/... -count=1 -race`

---

### WS-4: Provider/Batch/Persistence Fixes + QW-9/12
**Goal:** Fix webhook retry, null-array marshaling, autonomy persistence, and rules generation.
**Files:** `internal/session/batch.go`, `internal/session/batch_test.go`, `internal/session/providers_normalize.go`, `internal/session/providers_test.go`, `internal/session/reflexion.go`, `internal/session/autooptimize.go`, `internal/session/autooptimize_test.go`

1. **FINDING-66 (P2):** In `fireWebhook()`, add 3-attempt exponential backoff retry (1s, 2s, 4s). Add `WebhookDeliveryAttempts` to payload.
2. **FINDING-196 (P1):** Audit all struct fields using `[]string` or `[]T`. Initialize with `make([]T, 0)` to ensure `[]` not `null` in JSON.
3. **QW-9 (P1):** Add `PersistAutonomyLevel(level int, path string)` writing to `.ralph/autonomy.json`. Call on every `SetAutonomyLevel`.
4. **QW-12 (P2):** In `reflexion.go`, implement basic rule extraction from positive/negative patterns after 5+ cycles.

**Verification:** `go test ./internal/session/... -run "Batch|Normalize|Reflexion|AutoOptimize" -count=1 -race`

---

## Epic 3: Phase 0.6 Code Quality Completion

### WS-5: Observation Enrichment + Stall Detection + Gate Formatting
**Goal:** Complete Phase 0.6 items: 0.6.2 (observation enrichment), 0.6.4 (gate report formatting), 0.6.5 (stall detection).
**Files:** `internal/session/loop_helpers.go`, `internal/session/loop_helpers_test.go`, `internal/session/stall.go`, `internal/session/stall_test.go`, `internal/e2e/format.go`, `internal/e2e/format_test.go`, `internal/e2e/gates.go`, `internal/e2e/gates_test.go`

1. **Phase 0.6.2:** Add `GitDiffStat`, `PlannerModelUsed`, `WorkerModelUsed`, `AcceptancePath` fields to `LoopIteration`. Add `SummarizeObservations()` helper. Wire population in loop_helpers.
2. **Phase 0.6.4:** Add `FormatGateReport(*GateReport) string` and `FormatGateReportMarkdown(*GateReport) string` in `format.go`. Add `CompareGateReports(prev, current) []GateTrend` in `gates.go`.
3. **Phase 0.6.5:** Wire `StallTimeout` from LoopProfile into worker context deadline. If no output for `StallTimeout`, cancel worker and increment `StallCount`.
4. **Coverage:** Tests for `looksLikeJSON` and `enhanceForProvider` in loop_helpers.go (currently 0%).

**Verification:** `go test ./internal/session/... ./internal/e2e/... -count=1 -race`

---

### WS-6: Worktree Cleanup + Dedup + Race Fix + PID Wiring
**Goal:** Complete Phase 0.6 items: 0.6.1.1, 0.6.6, 0.6.7. Fix FINDING-62 race and FINDING-65 PID wiring.
**Files:** `internal/session/worktree.go`, `internal/session/worktree_test.go`, `internal/session/dedup.go`, `internal/session/dedup_test.go`, `internal/session/manager_lifecycle.go`, `internal/session/manager_lifecycle_test.go`, `cmd/root_test.go`, `internal/discovery/discovery_test.go`

1. **FINDING-62 (P2):** In `cmd/root_test.go`, fix race: `TestGateCheckCmd_ObservationsLoadError` mutates package globals without sync. Isolate state per test with `t.Setenv` or `sync.Mutex`.
2. **FINDING-65 (P3):** In `manager_lifecycle.go`, wire PID files: write JSON PID on launch, scan on init (re-adopt alive, clean dead), remove on stop.
3. **Phase 0.6.6:** Enhance `CleanupStaleWorktrees`: add `.lock` file detection, uncommitted-changes guard before removal.
4. **Phase 0.6.7:** Add Jaccard similarity to `isDuplicateTask` (threshold 0.8). Add `DedupReason` field.
5. **Phase 0.6.1.1:** Add tests for `internal/discovery/` error paths (unreadable dirs, symlink cycles).

**Verification:** `go test -race ./cmd/... ./internal/session/... ./internal/discovery/... -count=1`

---

## Epic 4: Coverage Uplift + Phase 1 Remaining

### WS-7: Zero-Coverage Function Tests
**Goal:** Reduce 74 zero-coverage functions to <40. Test files only — no source modifications.
**Files:** NEW test files in `internal/fleet/`, `internal/e2e/`, `internal/tracing/`, `internal/tui/`, `cmd/`

Priority targets:
- `internal/fleet/worker.go`: `Run`, `heartbeatLoop`, `pollLoop`, `executeWork` — mock-based lifecycle tests
- `internal/fleet/discovery.go`: `GetTailscaleStatus`, `DiscoverCoordinator` — mock network tests
- `internal/e2e/selftest.go`: `runIteration` — mock session provider
- `internal/e2e/harness.go`: `RunAll` — harness execution test
- `internal/tracing/prometheus.go`: `RecordError` — metric registration tests
- `internal/tui/views/eventlog.go`: `Init`, `ScrollDown` — view tests
- `cmd/root.go`: `Execute`, `ScanTimeoutDuration` — cobra command tests
- `cmd/ralphglasses-mcp/main.go`: startup/shutdown signal tests
- `cmd/prompt-improver/main.go`: `runImprove`, `runHook` error paths

**Verification:** `go test ./internal/fleet/... ./internal/e2e/... ./internal/tracing/... ./internal/tui/... ./cmd/... -coverprofile=cov.out -count=1 && go tool cover -func=cov.out | awk '$NF == "0.0%"' | wc -l`

---

### WS-8: QW-2/10/11 + Phase 1 TUI Hardening
**Goal:** Complete remaining Quick Wins and critical Phase 1 TUI safety items.
**Files:** `internal/session/cascade.go`, `internal/session/cascade_test.go`, `internal/fleet/coordinator.go`, `internal/fleet/coordinator_test.go`, `internal/mcpserver/tools_roadmap.go`, `internal/tui/app.go`, `internal/tui/app_update.go`

1. **QW-2 (P0):** Enable cascade routing by default in `DefaultCascadeConfig()`. Add test.
2. **QW-10 (P1):** In `tools_roadmap.go`, fix flat 0.5 relevance scoring. Replace with keyword overlap (Jaccard of title tokens vs query).
3. **QW-11 (P1):** In `fleet/coordinator.go`, add `ReapStaleTasks(maxAge)` removing pending items older than maxAge. Validate repo paths on submission.
4. **Phase 1.3.3 (P0):** Add SIGINT/SIGTERM shutdown handler via BubbleTea's `WithContext`.
5. **Phase 1.10.1/1.10.3 (P0):** Audit SortCol and all slice access in TUI. Clamp to valid range, add nil/empty guards.
6. **Phase 1.10.5 (P1):** Handle zero-height terminal: render "terminal too small" if `Height < 3`.

**Verification:** `go build ./...` && `go test ./internal/session/... ./internal/fleet/... ./internal/mcpserver/... ./internal/tui/... -count=1 -race`

---

## Execution Plan

```
Phase 1 — Parallel (8 worktree agents):

  WS-1 (loop engine)     WS-2 (MCP handlers)   WS-3 (enhancer)      WS-4 (provider/batch)
  ─────────────────────   ────────────────────   ──────────────       ─────────────────────

  WS-5 (P0.6 obs/stall)  WS-6 (P0.6 wt/dedup)  WS-7 (coverage)     WS-8 (QW+P1 harden)
  ────────────────────    ─────────────────────   ───────────────     ─────────────────────
       ↘                         ↓                     ↓                    ↙
        └────────────────────────┴─────────────────────┴───────────────────┘
                                        ↓
Phase 2 — Sequential:             Integration + docs
                                        ↓
                                  Commit & push
```

## Resource Conflict Matrix

| File Area | WS-1 | WS-2 | WS-3 | WS-4 | WS-5 | WS-6 | WS-7 | WS-8 |
|-----------|------|------|------|------|------|------|------|------|
| `session/loop_types.go` | **W** | | | | | | | |
| `session/loop.go` | **W** | | | | | | | |
| `session/loop_steps.go` | **W** | | | | | | | |
| `session/loop_gates.go` | **W** | | | | | | | |
| `session/loop_validate.go` | **W** | | | | | | | |
| `mcpserver/tools_session.go` | | **W** | | | | | | |
| `mcpserver/handler_repo.go` | | **W** | | | | | | |
| `mcpserver/handler_session_lifecycle.go` | | **W** | | | | | | |
| `mcpserver/tools_fleet.go` | | **W** | | | | | | |
| `enhancer/scoring.go` | | | **W** | | | | | |
| `enhancer/pipeline.go` | | | **W** | | | | | |
| `enhancer/pipeline_format.go` | | | **W** | | | | | |
| `session/batch.go` | | | | **W** | | | | |
| `session/providers_normalize.go` | | | | **W** | | | | |
| `session/reflexion.go` | | | | **W** | | | | |
| `session/autooptimize.go` | | | | **W** | | | | |
| `session/loop_helpers.go` | | | | | **W** | | | |
| `session/stall.go` | | | | | **W** | | | |
| `e2e/format.go` (new) | | | | | **W** | | | |
| `e2e/gates.go` | | | | | **W** | | | |
| `session/worktree.go` | | | | | | **W** | | |
| `session/dedup.go` | | | | | | **W** | | |
| `session/manager_lifecycle.go` | | | | | | **W** | | |
| `cmd/root_test.go` | | | | | | **W** | | |
| `fleet/*_test.go` (new) | | | | | | | **W** | |
| `tracing/*_test.go` (new) | | | | | | | **W** | |
| `tui/*_test.go` (new) | | | | | | | **W** | |
| `cmd/*_test.go` (new) | | | | | | | **W** | |
| `session/cascade.go` | | | | | | | | **W** |
| `fleet/coordinator.go` | | | | | | | | **W** |
| `mcpserver/tools_roadmap.go` | | | | | | | | **W** |
| `tui/app.go` | | | | | | | | **W** |
| `tui/app_update.go` | | | | | | | | **W** |

**WS-1/WS-5:** Both touch session pkg. WS-1 owns loop*.go files. WS-5 owns loop_helpers.go, stall.go. No overlap.
**WS-4/WS-6:** Both touch session pkg. WS-4 owns batch/providers/reflexion/autooptimize. WS-6 owns worktree/dedup/manager_lifecycle. No overlap.
**WS-5/WS-7:** Both touch e2e pkg. WS-5 modifies source (format.go, gates.go). WS-7 creates NEW test files only. Merge WS-5 first if overlap.
**WS-7/WS-8:** Both touch tui pkg. WS-7 creates NEW test files. WS-8 modifies source (app.go, app_update.go). No overlap.

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Score recalibration breaks existing enhancer tests | High | Medium | Adjust expected ranges, not exact values; use fuzzy assertions |
| Model name changes break DefaultLoopProfile callers | Medium | High | Keep backward compat: unknown model falls back to Claude Sonnet |
| Race fix in root_test.go more complex than expected | Medium | Medium | Use t.Setenv and isolated cobra commands per test |
| SIGINT handler conflicts with BubbleTea signal handling | Medium | High | Use BubbleTea's WithContext, don't override BT's handler |
| Fleet discovery tests need network mocking | High | Low | Use interfaces + mock structs, not real Tailscale |
| Webhook retry causes test flakiness | Medium | Low | Use httptest.NewServer with configurable failure counts |
| Phase 0.6 items partially implemented in Sprint 5 | Medium | Low | Read source before implementing; skip if done |

## Acceptance Criteria

| WS | Metric | Target |
|----|--------|--------|
| 1 | Loop findings | FINDING-186, 187, 267 fixed; QW-1, 3, 6, 8 done |
| 2 | Handler findings | FINDING-268, 169, 258/261, 237 fixed; QW-7 done |
| 3 | Score distribution | Spans 30-90 on test corpus (was 85-97) |
| 4 | Webhook retry | 3 attempts with backoff; null-array fix verified |
| 5 | Phase 0.6 items | 0.6.2, 0.6.4, 0.6.5 complete (12 subtasks) |
| 6 | Phase 0.6 items | 0.6.1.1, 0.6.6, 0.6.7 complete; race fixed; PID wired |
| 7 | Zero-coverage | Reduce from 74 to <40 |
| 8 | QW + Phase 1 | QW-2, 10, 11 done; 1.3.3, 1.10.1, 1.10.3, 1.10.5 done |
| ALL | Coverage | ≥88% (from 84.4%) |
| ALL | Tests | 37/37 green, `-race` clean |
| ALL | Open findings | <100 (from 157) |

## Constraints

- Non-overlapping file ownership per workstream (see matrix)
- Do NOT modify ROADMAP.md during parallel phase
- Do NOT add external dependencies
- Every workstream must pass `go build ./...` && `go vet ./...`
- Use `/bin/cp` (not `cp`) when copying between worktrees (macOS alias)
- If `$ARGUMENTS` filter present, only run specified workstreams
- Backward-compatible changes only: add params/fields, don't remove
- Read source before implementing — many items may already be partially done
- JSON marshaling changes must not break existing API consumers

## Post-Sprint

After all workstreams complete:
1. `go test ./... -count=1 -timeout 120s -race` — full green
2. Coverage: `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | tail -1`
3. Count 0% functions: `go tool cover -func=coverage.out | awk '$NF == "0.0%"' | wc -l`
4. Scratchpad: count open findings, verify fixed ones resolved
5. Update CLAUDE.md if tool count changes
6. Update ROADMAP.md: mark completed Phase 0.6, 0.9, Phase 1 items
7. Report: phase completion deltas, findings resolved count, coverage delta, 0% function reduction
