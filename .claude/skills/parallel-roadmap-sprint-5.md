---
name: parallel-roadmap-sprint-5
description: Mega sprint — Phase 1 hardening (38→80%), Phase 0.5 completion, coverage uplift (83.6→88%), 101 zero-coverage functions, Phase 1.5 DX foundations
user-invocable: true
argument-hint: [workstream-filter]
---

You are executing Sprint 5 for the ralphglasses project. This sprint focuses on hardening Phase 1, completing Phase 0.5, and driving coverage toward 88%.

## Project Context

Ralphglasses is a Go TUI + MCP server (125 tools, 14 namespaces) for managing parallel multi-LLM agent fleets. 37 packages, 83.6% test coverage, 37/37 packages green. Uses Charmbracelet BubbleTea v1, mark3labs/mcp-go v0.45.0, modernc.org/sqlite.

**Sprints 1-4 completed:** 12 QW bug fixes, doc sync, self-improvement pipeline, 10 rdcycle tools, score hardening, fleet reaper, cascade default, autonomy persistence, ParamParser, loop validation, gate reports, stall callbacks, SQLite loop_runs/cost_ledger, rdcycle tests (0→88%), FINDING-55 fix, config deprecation, loop store persistence, session history, shell completion, envkit coverage (44→91%).

**Current state:** 28.3% roadmap completion (175/619 tasks). 83.6% overall test coverage. 101 functions at 0% coverage. All 28 handler files have test files.

**Phase status:**
- Phase 0: 100% (16/16)
- Phase 0.5: 71% (32/45) — 13 remaining
- Phase 0.8: 100% (20/20)
- Phase 1: 38% (21/55) — 34 remaining (FOCUS)
- Phase 1.5: 12% (6/52) — 46 remaining
- Phase 2: 11% (8/70)
- Phase 2.5: 89% (24/27) — 3 remaining

**Packages below 80%:**
- `cmd/ralphglasses-mcp`: 59.1%
- `cmd`: 70.7%
- `cmd/prompt-improver`: 70.0%
- `internal/tui`: 75.7%
- `internal/fontkit`: 77.2%

## Pre-flight

Read these files to confirm current state:
- `ROADMAP.md` — Phase 0.5 and Phase 1 unchecked items
- `internal/mcpserver/handler_mergeverify.go` — recent FINDING-55 fix
- `internal/session/loop.go` — RunLoop 44.2% coverage
- `internal/session/loop_helpers.go` — looksLikeJSON/enhanceForProvider at 0%
- `internal/e2e/` — selftest, gates, harness (many 0% functions)
- `internal/fleet/` — discovery, worker, a2a at 0%

## Epic 1: Phase 0.5 Completion (71% → 100%)

### WS-1: Phase 0.5 Remaining Items
**Goal:** Complete the 13 remaining Phase 0.5 tasks.
**Files:** Various — read ROADMAP.md Phase 0.5 section for specifics.

Read ROADMAP.md and identify all `[ ]` items under Phase 0.5. Focus on:
1. **Config validation strictness** (Phase 0.5.11) — All 4 items unchecked. Read `internal/model/config.go` to see what `ValidateConfig()` covers. Add missing validations.
2. **Disk space check** (QW-10.2) — `internal/session/resources.go` may already have `DiskSpaceWarning`. Wire it into loop start.
3. **Restart loop cap** (QW-10.3) — Ensure restart backoff has a max retry limit.
4. **Memory pressure detection** (QW-10.4) — Add runtime.MemStats check before loop iteration.
5. **Log rotation** (QW-10.5) — `internal/session/logrotate.go` may already exist. Verify and wire in.

For each item: verify if already implemented (Sprint 3/4 may have done some), implement if missing, add test.

**Verification:** `go build ./...` && `go test ./internal/session/... ./internal/model/... -count=1`

---

## Epic 2: Phase 1 Hardening (38% → 65%)

### WS-2: Phase 1.1-1.3 Config & Health
**Goal:** Complete config validation, health checks, and self-test gate items from Phase 1.
**Files:** `internal/model/config.go`, `internal/mcpserver/handler_repo_health.go`, `internal/e2e/`

Read ROADMAP.md Phase 1 and identify all `[ ]` items in sections 1.1-1.3. Key tasks likely include:
1. **Config migration tool** — Auto-migrate deprecated keys to new keys
2. **Config export/import** — Serialize/deserialize full config
3. **Health check scoring weights** — Configurable weights per check category
4. **Self-test regression detection** — Compare self-test results across runs

**Verification:** `go test ./internal/model/... ./internal/mcpserver/... ./internal/e2e/... -count=1`

### WS-3: Phase 1.4-1.6 Process & Session Hardening
**Goal:** Complete process management and session lifecycle items.
**Files:** `internal/process/manager.go`, `internal/session/manager.go`, `internal/session/loop.go`

Read ROADMAP.md Phase 1 sections 1.4-1.6. Key tasks likely include:
1. **Graceful shutdown sequence** — Stop sessions in reverse launch order
2. **Session recovery on restart** — Scan `.ralph/sessions/` for leftover PIDs
3. **Orphan zombie detection** — Enhance ReapOrphans for zombie processes
4. **Session timeout escalation** — Configurable per-session timeout profiles

**Verification:** `go test ./internal/process/... ./internal/session/... -count=1`

### WS-4: Phase 1.7-1.9 Observability & Loop Hardening
**Goal:** Complete observability and loop reliability items.
**Files:** `internal/session/loop.go`, `internal/session/loop_steps.go`, `internal/session/loopbench.go`

Read ROADMAP.md Phase 1 sections 1.7-1.9. Key tasks likely include:
1. **Loop step timeout recovery** — Partial result capture on timeout
2. **Observation enrichment** — Add missing fields to LoopObservation
3. **Budget forecasting integration** — Wire budget_forecast into loop step decisions
4. **Stall recovery automation** — Auto-restart on stall with different provider

**Verification:** `go test ./internal/session/... -count=1`

---

## Epic 3: Coverage Uplift (83.6% → 88%)

### WS-5: Zero-Coverage Function Tests (101 functions → <50)
**Goal:** Add tests for the most impactful 0% coverage functions.
**Files:** Test files across multiple packages

Priority targets (grouped by package):

**`internal/session/` (highest impact):**
- `loop.go:RunLoop` (44.2%) — test iteration count, budget cutoff, no-op detection
- `loop_helpers.go:looksLikeJSON` (0%) — test valid/invalid JSON detection
- `loop_helpers.go:enhanceForProvider` (0%) — test provider-specific enhancement
- `loop_acceptance.go:handleSelfImprovementAcceptance*` (0%) — test acceptance logic

**`internal/e2e/` (test infrastructure):**
- `selftest.go:runIteration` (0%) — mock-based test
- `gates.go:RunE2EGate` (0%) — test gate evaluation
- `harness.go:RunAll` (0%) — test harness execution
- `gitdiff.go:GitDiffPaths/GitDiffStats` (0%) — test with temp git repo

**`internal/fleet/` (fleet subsystem):**
- `discovery.go:GetTailscaleStatus/DiscoverCoordinator` (0%) — mock network tests
- `a2a.go:OfferCount/CountByStatus` (0%) — test offer management
- `worker.go:NewWorkerAgent` (0%) — test worker initialization
- `queue.go:ReapStale` (0%) — test stale work item cleanup

**Verification:** `go test ./internal/session/... ./internal/e2e/... ./internal/fleet/... -count=1`

### WS-6: Low-Coverage Package Tests
**Goal:** Raise the 5 packages below 80%.
**Files:** Test files in `cmd/`, `internal/tui/`, `internal/fontkit/`

1. `cmd/ralphglasses-mcp` (59.1%) — Test MCP server startup, shutdown, signal handling
2. `cmd` (70.7%) — Test cobra command error paths, flag validation
3. `cmd/prompt-improver` (70.0%) — Test runImprove, runHook paths
4. `internal/tui` (75.7%) — Test view rendering edge cases, message handling
5. `internal/fontkit` (77.2%) — Test font detection error paths

For each: run coverage, identify gaps, add targeted tests.

**Verification:** `go test ./cmd/... ./internal/tui/... ./internal/fontkit/... -coverprofile=cov.out && go tool cover -func=cov.out | grep -E "total:|cmd|tui|fontkit"`

---

## Epic 4: Phase 1.5 DX Foundations

### WS-7: Developer Experience Quick Wins
**Goal:** Complete high-value Phase 1.5 items that are quick to implement.
**Files:** `cmd/`, `docs/`

Read ROADMAP.md Phase 1.5. Pick the 6-8 most impactful items:
1. **`doctor` enhancements** — Read `cmd/doctor.go`. Add missing checks: MCP server buildability test, `.ralphrc` validation summary, disk space warning
2. **Config templates** — Add `ralphglasses init` or `ralphglasses config --template` for common configurations
3. **Error message improvements** — Grep for generic error messages across handlers. Add context and suggested fixes.
4. **CLI help text** — Review all cobra command help strings for clarity and completeness
5. **Diagnostic bundle** — `ralphglasses debug-bundle` that collects config, logs, versions, health check into a tar.gz

**Verification:** `go build ./...` && `go test ./cmd/... -count=1`

---

## Epic 5: Phase 2.5 Completion (89% → 100%)

### WS-8: Multi-LLM Orchestration Final 3 Items
**Goal:** Complete the last 3 unchecked items in Phase 2.5.
**Files:** Read ROADMAP.md Phase 2.5 for specifics

Read the 3 remaining Phase 2.5 `[ ]` items. These are likely orchestration edge cases or documentation. Implement and test.

**Verification:** `go test ./... -count=1 -timeout 120s`

---

## Execution Plan

```
Phase 1 — Parallel (8 worktree agents):

  WS-1 (Phase 0.5)    WS-2 (P1 config)    WS-3 (P1 process)    WS-4 (P1 loop)
  ─────────────────    ────────────────     ──────────────────    ──────────────

  WS-5 (0% functions)  WS-6 (low-cov pkgs)  WS-7 (P1.5 DX)      WS-8 (P2.5 final)
  ──────────────────   ─────────────────     ───────────────      ──────────────
       ↘                      ↓                    ↓                    ↙
        └─────────────────────┴────────────────────┴───────────────────┘
                                        ↓
Phase 2 — Sequential:             Integration + docs
                                        ↓
                                  Commit & push
```

## Resource Conflict Matrix

| File Area | WS-1 | WS-2 | WS-3 | WS-4 | WS-5 | WS-6 | WS-7 | WS-8 |
|-----------|------|------|------|------|------|------|------|------|
| `model/config.go` | **W** | | | | | | | |
| `session/resources.go` | **W** | | | | | | | |
| `session/logrotate.go` | **W** | | | | | | | |
| `mcpserver/handler_repo_health.go` | | **W** | | | | | | |
| `e2e/*.go` | | **W** | | | | | | |
| `process/manager.go` | | | **W** | | | | | |
| `session/manager.go` | | | **W** | | | | | |
| `session/loop.go` | | | | **W** | | | | |
| `session/loop_steps.go` | | | | **W** | | | | |
| `session/*_test.go` (new) | | | | | **W** | | | |
| `e2e/*_test.go` (new) | | | | | **W** | | | |
| `fleet/*_test.go` (new) | | | | | **W** | | | |
| `cmd/*_test.go` | | | | | | **W** | | |
| `tui/*_test.go` | | | | | | **W** | | |
| `fontkit/*_test.go` | | | | | | **W** | | |
| `cmd/doctor.go` | | | | | | | **W** | |

**Potential conflict: WS-4 and WS-5 both touch session package.** Resolution: WS-4 modifies source files (loop.go, loop_steps.go). WS-5 only creates NEW test files. Different files — no overlap.

**Potential conflict: WS-2 and WS-5 both touch e2e package.** Resolution: WS-2 modifies source files. WS-5 only creates NEW test files. If WS-5 tests test functions WS-2 modifies, merge WS-2 first.

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| ROADMAP items already partially done | High | Low | Read source before implementing; skip if done |
| WS-5 fleet tests need network mocking | Medium | Medium | Use interfaces + mock structs, not real network |
| WS-3 session recovery may break tests | Medium | High | Guard with nil checks; test with mock PIDs |
| WS-4 loop changes may affect other tests | Low | High | Run full suite after merge |
| Phase 1.5 items may be under-specified | Medium | Low | Read ROADMAP for acceptance criteria |

## Acceptance Criteria

| WS | Metric | Target |
|----|--------|--------|
| 1 | Phase 0.5 completion | 100% (45/45) |
| 2 | Phase 1.1-1.3 completion | ≥80% of section items |
| 3 | Phase 1.4-1.6 completion | ≥80% of section items |
| 4 | Phase 1.7-1.9 completion | ≥80% of section items |
| 5 | 0% coverage functions | Reduce from 101 to <50 |
| 6 | Below-80% packages | All packages ≥80% |
| 7 | Phase 1.5 items | 6-8 quick wins complete |
| 8 | Phase 2.5 completion | 100% (27/27) |
| ALL | Overall coverage | ≥88% (from 83.6%) |
| ALL | Test suite | 37/37 green |

## Constraints

- Non-overlapping file ownership per workstream
- Do NOT modify ROADMAP.md during parallel phase
- Do NOT add external dependencies
- Every workstream must pass `go build ./...` && `go vet ./...`
- Use `/bin/cp` (not `cp`) when copying between worktrees (macOS alias)
- If `$ARGUMENTS` filter present, only run specified workstreams
- Backward-compatible changes only
- Read ROADMAP.md FIRST for each WS — many items may already be done from Sprints 1-4

## Post-Sprint

After all workstreams complete:
1. `go test ./... -count=1 -timeout 120s` — full green
2. Coverage: `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | tail -1`
3. Count 0% functions: `go tool cover -func=coverage.out | awk '$NF == "0.0%"' | wc -l`
4. Update CLAUDE.md if tool count changes
5. Update ROADMAP.md: mark completed phases
6. Report: phase completion deltas, coverage delta, 0% function reduction
