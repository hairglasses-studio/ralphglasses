---
name: parallel-roadmap-sprint-4
description: Mega sprint — rdcycle tests, scratchpad fixes, Phase 1 hardening, Phase 1.5 DX, Phase 2 multi-session foundations, coverage uplift
user-invocable: true
argument-hint: [workstream-filter]
---

You are executing Sprint 4 for the ralphglasses project. This sprint focuses on test gaps, blocking bugs from scratchpad findings, and unlocking the multi-session architecture.

## Project Context

Ralphglasses is a Go TUI + MCP server (125 tools, 14 namespaces) for managing parallel multi-LLM agent fleets. 37 packages, ~81% test coverage, 37/37 packages green. Uses Charmbracelet BubbleTea v1, mark3labs/mcp-go v0.45.0, modernc.org/sqlite.

**Sprints 1-3 completed:** 12 QW bug fixes, doc sync, self-improvement pipeline, 10 rdcycle tools, score hardening, fleet reaper, cascade default, autonomy persistence, ParamParser, loop validation, gate reports, stall callbacks, SQLite loop_runs/cost_ledger. Total: 39 files, 4008 insertions in Sprint 3 alone.

**Current state:** 28.3% roadmap completion (175/619 tasks). Critical path: Phase 0.5/0.9 completion → Phase 1 hardening → Phase 2 multi-session.

## Pre-flight

Read these files to confirm current state:
- `ROADMAP.md` — phases, task counts, completion markers
- `.ralph/tool_improvement_scratchpad.md` — 50 unresolved findings (FINDING-54 through FINDING-61 are critical)
- `internal/mcpserver/handler_rdcycle.go` — 1,301 lines, NO test file (critical gap)
- `internal/session/store.go` — Store interface with new loop_runs/cost methods
- `internal/mcpserver/params.go` — ParamParser (Sprint 3)

## Epic 1: RDCycle Test Coverage (CRITICAL — 0% → 80%)

### WS-1: handler_rdcycle_test.go
**Goal:** Test the 10 rdcycle handlers that have zero direct test coverage. This is a 1,301-line file with complex logic.
**Files:** NEW `internal/mcpserver/handler_rdcycle_test.go`

For each of the 10 handlers, write tests covering:
1. **Happy path** — valid inputs, expected output structure
2. **Missing required params** — verify INVALID_PARAMS error
3. **Edge cases** — empty repo, nonexistent finding, zero iterations

Handlers to test (read handler_rdcycle.go first for signatures):
- `handleFindingToTask` — needs scratchpad file fixture
- `handleCycleBaseline` — needs git repo fixture (use t.TempDir + git init)
- `handleCyclePlan` — needs scratchpad + patterns fixtures
- `handleCycleMerge` — needs worktree directory fixtures
- `handleCycleSchedule` — needs valid/invalid cron expressions
- `handleLoopReplay` — needs loop run file fixture
- `handleBudgetForecast` — needs cost_observations.json fixture
- `handleDiffReview` — needs git repo with committed diff
- `handleFindingReason` — needs scratchpad fixture
- `handleObservationCorrelate` — needs observations + git log fixtures

Use `testServer()` or the pattern from existing handler test files (e.g., `handler_loop_test.go`).

**Verification:** `go test ./internal/mcpserver/... -run "RDCycle|Rdcycle|FindingToTask|CycleBaseline|CyclePlan|CycleMerge|CycleSchedule|LoopReplay|BudgetForecast|DiffReview|FindingReason|ObservationCorrelate" -count=1 -v`

---

## Epic 2: Scratchpad Finding Fixes (P0/P1 blockers)

### WS-2: Tool Error Fixes (FINDING-54 through FINDING-61)
**Goal:** Fix the 8 most critical tool errors found in self-improvement audit.
**Files:** Various handler files in `internal/mcpserver/`

**FINDING-54: `loop_step` 35.7% error rate**
- File: `internal/mcpserver/handler_loop.go` — find `handleLoopStep`
- Issue: timeout too aggressive for complex iterations
- Fix: increase per-step timeout from 60s to 120s, add structured error output with partial results

**FINDING-55: `merge_verify` 66.7% error rate**
- File: `internal/mcpserver/handler_mergeverify.go`
- Issue: error output not structured, hard to diagnose
- Fix: return structured JSON with per-step results (build/vet/test each as separate status)

**FINDING-56: `logs` tool crashes on missing ralph.log**
- File: `internal/mcpserver/handler_core.go` or wherever `handleLogs` is
- Fix: return `{"entries": [], "message": "no log file found"}` instead of error

**FINDING-57/58: `scratchpad_list/read` multi-repo confusion**
- File: `internal/mcpserver/handler_scratchpad.go`
- Fix: when `repo` param is empty, search all known repos; add clear error when scratchpad not found

**FINDING-59: `fleet_analytics` not initialized**
- File: `internal/mcpserver/handler_fleet.go`
- Fix: handle nil analytics gracefully, return empty analytics with warning

**FINDING-60: `event_list` undocumented params**
- File: `internal/mcpserver/handler_events.go` or advanced handler
- Fix: update tool description/schema to document all params

**FINDING-61: TimeoutMiddleware kills long-running tools**
- File: `internal/mcpserver/middleware.go`
- Fix: add per-tool timeout override map; exempt long-running tools (cycle_baseline, diff_review, self_improve)

**Verification:** `go build ./...` && `go test ./internal/mcpserver/... -count=1`

---

## Epic 3: Phase 1 Hardening Completion

### WS-3: Config Validation + Health Checks (Phase 1.1-1.3)
**Goal:** Complete the config/health hardening items from Phase 1.
**Files:** `internal/model/config.go`, `internal/session/manager.go`, `internal/mcpserver/handler_repo_health.go`

Tasks:
1. **Deprecated key warnings** — Add `DeprecatedKeys` map to `config.go`. When `ValidateConfig` encounters a deprecated key, add warning with migration hint.
2. **Config diff tool** — Add `ConfigDiff(old, new *RalphConfig) []ConfigChange` to compare configs and report additions/removals/changes.
3. **Health check improvements** — In `handleRepoHealth`, add checks for:
   - `.ralphrc` parse errors
   - Missing required directories (`.ralph/`, `.ralph/logs/`)
   - Stale lock files older than 1 hour
4. **Self-test gate mode** — Read `cmd/selftest.go` if it exists. If not, create it with `--gate` flag that runs build+vet+test and exits 0/1.

**Verification:** `go build ./...` && `go test ./internal/model/... ./internal/session/... ./internal/mcpserver/... -count=1`

### WS-4: Process Management Hardening (Phase 1.4-1.6)
**Goal:** Harden process lifecycle for production reliability.
**Files:** `internal/process/manager.go`, `internal/session/loop.go`

Tasks:
1. **Kill escalation timeout** — Currently hardcoded. Make configurable via `.ralphrc` key `KILL_ESCALATION_SECS` (default 10). Read from config in `Manager.Stop()`.
2. **Graceful shutdown sequence** — Ensure `Manager.Shutdown()` stops all sessions in reverse launch order, waits for each.
3. **Orphan process detection improvement** — Enhance `ReapOrphans()` to also check for zombie processes (state Z).
4. **Session recovery on restart** — In `Manager.Init()`, scan for leftover session PIDs from `.ralph/sessions/`. If process alive, re-adopt. If dead, mark as crashed.

**Verification:** `go test ./internal/process/... ./internal/session/... -count=1`

---

## Epic 4: Phase 1.5 Developer Experience

### WS-5: CLI Improvements + Documentation
**Goal:** Quality-of-life CLI improvements.
**Files:** `cmd/` package, docs/

Tasks:
1. **`selftest` subcommand** — Create `cmd/selftest.go`:
   - `ralphglasses selftest` — run build, vet, test suite
   - `ralphglasses selftest --gate` — same but exit code for CI
   - `ralphglasses selftest --coverage` — also emit coverage
   This addresses FINDING-34 and FINDING-36.

2. **`doctor` improvements** — Read `cmd/doctor.go`. Add checks for:
   - Go version compatibility (≥1.22)
   - Required env vars (ANTHROPIC_API_KEY, GEMINI_API_KEY presence)
   - MCP server binary buildability
   - `.ralphrc` parse status

3. **Shell completion** — If not already present, add `cmd/completion.go` with bash/zsh/fish completion via cobra.

4. **Envkit coverage** — `internal/envkit` is at 43.9% coverage. Read the package, add error-path tests to reach ≥75%.

**Verification:** `go build ./...` && `go test ./cmd/... ./internal/envkit/... -count=1`

---

## Epic 5: Phase 2.1 Multi-Session Foundations

### WS-6: Session Store Wiring + Manager Integration
**Goal:** Wire the SQLite store (from Sprint 3) into the Manager and session lifecycle.
**Files:** `internal/session/manager.go`, `internal/session/loop.go`

Tasks:
1. **Manager store wiring** — In `Manager.Init()`, initialize SQLiteStore if not already set:
   ```go
   if m.store == nil {
       store, err := NewSQLiteStore(filepath.Join(m.ralphDir, "ralph.db"))
       if err != nil {
           log.Warn("falling back to memory store", "err", err)
           store = NewMemoryStore()
       }
       m.store = store
   }
   ```

2. **Persist sessions on state change** — In `Manager.LaunchSession()`, `Manager.StopSession()`, and session status updates, call `m.store.SaveSession()`.

3. **Persist loop runs** — In `StartLoop()`, `StopLoop()`, and iteration completion, call `m.store.SaveLoopRun()`.

4. **Cost recording** — In `recordCost()` or `updateSpend()`, also call `m.store.RecordCost()`.

5. **Query sessions from store** — Wire `Manager.ListSessions()` to use `m.store.ListSessions()` as primary source, falling back to in-memory map.

**Verification:** `go test ./internal/session/... -count=1 -v`

### WS-7: Multi-Session Query API (Phase 2.2)
**Goal:** Add cross-session query capabilities via MCP tools.
**Files:** `internal/mcpserver/handler_session.go`

Tasks:
1. **Session history** — Enhance `handleSessionList` to accept `include_ended=true` param that queries SQLite for historical sessions (not just live ones).
2. **Cost summary** — Enhance `handleSessionBudget` or add new handler to return aggregated cost by provider, model, time window.
3. **Session search** — Add `handleSessionSearch` with free-text search across session prompts, outputs, and error messages.

**Verification:** `go test ./internal/mcpserver/... -run "Session" -count=1`

---

## Epic 6: Coverage Uplift

### WS-8: Package Coverage Targets
**Goal:** Raise coverage for packages below 75%.
**Files:** Test files in `internal/envkit/`, `internal/tui/`, `cmd/`

Priority order:
1. `internal/envkit` — 43.9% → ≥75% (font/theme detection error paths)
2. `cmd/ralphglasses-mcp` — 59.1% → ≥75% (MCP CLI startup/shutdown)
3. `cmd` — 70.7% → ≥80% (cobra command error paths)
4. `internal/tui` — 75.7% → ≥80% (view rendering edge cases)

For each: run coverage, identify gaps, add targeted tests.

**Verification:** `go test ./internal/envkit/... ./cmd/... ./internal/tui/... -coverprofile=cov.out && go tool cover -func=cov.out | grep -E "total:|envkit|cmd|tui"`

---

## Execution Plan

```
Phase 1 — Parallel (8 worktree agents):

  WS-1 (rdcycle tests)   WS-2 (finding fixes)   WS-3 (config/health)   WS-4 (process harden)
  ────────────────────    ──────────────────────   ────────────────────    ────────────────────

  WS-5 (CLI/DX)          WS-6 (store wiring)     WS-7 (session query)   WS-8 (coverage)
  ──────────────          ─────────────────────    ──────────────────     ──────────────
       ↘                         ↓                        ↓                    ↙
        └────────────────────────┴────────────────────────┴───────────────────┘
                                          ↓
Phase 2 — Sequential:               Integration + docs
                                          ↓
                                    Commit & push
```

## Resource Conflict Matrix

| File | WS-1 | WS-2 | WS-3 | WS-4 | WS-5 | WS-6 | WS-7 | WS-8 |
|------|------|------|------|------|------|------|------|------|
| `mcpserver/handler_rdcycle_test.go` (NEW) | **W** | | | | | | | |
| `mcpserver/handler_loop.go` | | **W** | | | | | | |
| `mcpserver/handler_mergeverify.go` | | **W** | | | | | | |
| `mcpserver/handler_scratchpad.go` | | **W** | | | | | | |
| `mcpserver/handler_fleet.go` | | **W** | | | | | | |
| `mcpserver/middleware.go` | | **W** | | | | | | |
| `model/config.go` | | | **W** | | | | | |
| `mcpserver/handler_repo_health.go` | | | **W** | | | | | |
| `process/manager.go` | | | | **W** | | | | |
| `session/loop.go` | | | | | | **W** | | |
| `session/manager.go` | | | | | | **W** | | |
| `mcpserver/handler_session.go` | | | | | | | **W** | |
| `cmd/selftest.go` (NEW) | | | | | **W** | | | |
| `cmd/doctor.go` | | | | | **W** | | | |
| `envkit/*_test.go` | | | | | | | | **W** |
| `tui/*_test.go` | | | | | | | | **W** |

**Conflict: WS-4 and WS-6 both reference session/loop.go.** Resolution: WS-4 touches process/manager.go only; WS-6 touches session/loop.go for store persistence. Different functions — no overlap if WS-4 stays in process package.

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| WS-1 rdcycle tests need complex fixtures | High | Medium | Use t.TempDir + minimal git repos |
| WS-2 finding fixes may change tool behavior | Medium | High | Keep backward-compatible; add params, don't remove |
| WS-6 store wiring may break existing tests | Medium | High | Guard with nil checks; fallback to memory store |
| WS-7 session search needs SQLite FTS | Low | Low | Use LIKE queries initially; FTS5 in later sprint |
| FINDING-61 timeout fix may slow tools | Low | Medium | Per-tool override map, not global change |

## Acceptance Criteria

| WS | Metric | Target |
|----|--------|--------|
| 1 | handler_rdcycle.go test coverage | ≥80% (from 0%) |
| 2 | Scratchpad findings resolved | 6 of 8 (FINDING-54-61) |
| 3 | Config validation | Deprecated key warnings work |
| 4 | Process hardening | Kill timeout configurable, recovery works |
| 5 | selftest command | `ralphglasses selftest --gate` exits 0 |
| 6 | Session persistence | Sessions survive Manager restart |
| 7 | Historical session query | `include_ended=true` returns SQLite data |
| 8 | Coverage | envkit ≥75%, cmd ≥75%, overall ≥83% |

## Constraints

- Non-overlapping file ownership per workstream
- Do NOT modify ROADMAP.md during parallel phase
- Do NOT add external dependencies
- Every workstream must pass `go build ./...` && `go vet ./...`
- Use `/bin/cp` (not `cp`) when copying between worktrees (macOS alias)
- If `$ARGUMENTS` filter present, only run specified workstreams
- Backward-compatible tool changes only — add params, don't remove

## Post-Sprint

After all workstreams complete:
1. `go test ./... -count=1 -timeout 120s` — full green
2. Coverage: `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | tail -1`
3. Update CLAUDE.md if tool count changes
4. Update docs/MCP-TOOLS.md if tool schemas change
5. Report: phase completion deltas, findings resolved, coverage delta
