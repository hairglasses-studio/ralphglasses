# Phase 8 Research: Session Management & Persistence

Covers ROADMAP items **2.1** (Session data model), **2.2** (Git worktree orchestration), and **2.10** (Marathon.sh Go port).

---

## 1. Executive Summary

- **The in-memory session model is feature-rich but fragile**: sessions are lost on TUI restart because the only persistence mechanism is JSON files in `~/.ralphglasses/sessions/`, with no transactional guarantees, no migration support, and a 24h auto-cleanup that silently deletes history. Adopting SQLite (via `modernc.org/sqlite`) per ROADMAP 2.1.2 is the highest-impact change.
- **Git worktree orchestration already exists in `loop.go` but is not exposed as a standalone capability**: `createLoopWorktree()` (loop.go:525-544) handles worktree creation for the planner/worker loop, but there is no `internal/worktree/` package, no merge-back, no cleanup, and no CLI/MCP surface -- all required by ROADMAP 2.2.
- **`marathon.sh` (418 lines of bash) is a fully functional but unmaintainable supervisor**: it duplicates budget checking, checkpoint logic, signal handling, and cost ledger writing that already exist (partially) in Go. Porting to `internal/marathon/` (ROADMAP 2.10) eliminates shell fragility, enables cross-platform parity, and unifies the cost pipeline.
- **The Session struct is already well-defined (types.go:33-66)** with 25+ fields, but lacks a formal state machine -- status transitions are ad-hoc assignments scattered across manager.go and runner.go. ROADMAP 2.1.4 requires valid transition enforcement.
- **Test coverage is 64.5% with 0 failures** across 29 source files (7,689 lines). Priority gaps are checkpoint.go (0 tests), failover.go (0 tests), and the persistence path in LoadExternalSessions.

---

## 2. Current State Analysis

### 2.1 What Exists

| File | Lines | Tests | Coverage | Status |
|------|-------|-------|----------|--------|
| `internal/session/manager.go` | 846 | `manager_test.go` (690 lines, 16 test funcs) | Good | Core session CRUD, teams, workflows, persistence |
| `internal/session/runner.go` | 380 | `runner_test.go` (221 lines, 4 test funcs) | Moderate | Session lifecycle, streaming JSON parsing |
| `internal/session/types.go` | 147 | N/A (type defs) | N/A | Session, LaunchOptions, Provider, TeamConfig, StreamEvent |
| `internal/session/loop.go` | 870 | `loop_test.go` (227 lines, 4 test funcs) | Moderate | Planner/worker/verifier loop, worktree creation |
| `internal/session/checkpoint.go` | 50 | None | **0%** | Git checkpoint (add, commit, tag) |
| `internal/session/budget.go` | 131 | `budget_test.go` (95 lines) | Good | BudgetEnforcer, LedgerEntry, CostSummary |
| `internal/session/journal.go` | 401 | `journal_test.go` (305 lines) | Good | Improvement journal JSONL, pattern consolidation |
| `internal/session/workflow.go` | 265 | `workflow_test.go` (193 lines) | Good | YAML workflow defs, validation, DAG execution |
| `internal/session/providers.go` | 682 | `providers_test.go` (390 lines) | Good | Multi-provider command builder, event normalizer |
| `internal/session/failover.go` | 50 | None | **0%** | FailoverChain, LaunchWithFailover |
| `internal/session/health.go` | 100 | `health_test.go` (64 lines) | Moderate | Provider health checks |
| `internal/session/ratelimit.go` | 99 | `ratelimit_test.go` (80 lines) | Good | Per-provider sliding window rate limiter |
| `internal/session/costnorm.go` | 81 | `costnorm_test.go` (63 lines) | Good | Cross-provider cost normalization |
| `internal/session/agents.go` | 321 | `agents_test.go` (232 lines) | Good | Agent definition discovery and parsing |
| `internal/session/gitinfo.go` | 128 | None | **0%** | GitLogSince, GitDiffWindow |
| `internal/session/question.go` | 37 | Inline in other tests | Low | Question detection for headless mode |
| `internal/session/templates.go` | 62 | `templates_test.go` (61 lines) | Good | Provider-specific prompt templates |
| `marathon.sh` | 418 | None | **0%** | Bash supervisor: budget, duration, checkpoints, restarts |

**Totals**: 29 Go files, 7,689 lines (source + tests), 64.5% statement coverage, all tests passing.

### 2.2 What Works Well

1. **In-memory session management is solid**: The `Manager` struct (`manager.go:23-34`) provides thread-safe session CRUD with proper mutex discipline. Concurrent tests (`manager_test.go:460-526`) pass under `-race` reliably.

2. **Multi-provider architecture is extensible**: The `buildCmdForProvider()` dispatch (`providers.go:132-159`) and `normalizeEvent()` (`providers.go:263-275`) make adding new providers a matter of following a documented 7-step process (CLAUDE.md).

3. **Streaming output parsing is robust**: `runSessionOutput()` (`runner.go:226-343`) handles context cancellation, parse errors (incrementing `StreamParseErrors`), and EOF gracefully. The `scanCh` channel pattern prevents goroutine leaks.

4. **Event bus integration is complete**: `events.Bus` wiring (`manager.go:48-57`, `runner.go:182-196`) publishes lifecycle events (SessionStarted, SessionEnded, CostUpdate, BudgetExceeded, JournalWritten, PromptEnhanced) enabling reactive TUI updates.

5. **Loop worktree creation works**: `createLoopWorktree()` (`loop.go:525-544`) correctly uses `git worktree add -B` with proper branch naming (`ralph/<loop-id>/<iteration>`).

6. **Budget enforcement has dual layers**: Primary via CLI flag `--max-budget-usd` plus secondary via `BudgetEnforcer.Check()` (`budget.go:23-38`) with configurable headroom (90% default).

### 2.3 What Doesn't Work

1. **Sessions do not survive TUI restart** [ROADMAP 2.1]: `PersistSession()` (`manager.go:719-734`) writes JSON to disk but `LoadExternalSessions()` (`manager.go:786-846`) only runs ad-hoc and silently drops sessions older than 24h. There is no startup-load path. No transactional writes (crashes can produce corrupt JSON).

2. **No state machine enforcement** [ROADMAP 2.1.4]: Status transitions are direct field assignments: `s.Status = StatusStopped` (`manager.go:174`), `s.Status = StatusRunning` (`runner.go:101-102`), `s.Status = StatusErrored` (`runner.go:151`). Invalid transitions (e.g., `completed -> running`) are not prevented.

3. **No session event log** [ROADMAP 2.1.5]: State changes are published to the event bus for real-time use but never persisted. Historical lifecycle queries are impossible.

4. **No standalone worktree package** [ROADMAP 2.2]: Worktree logic is embedded in `loop.go:525-544` (creation) with no merge-back (`2.2.3`), no cleanup (`2.2.4`), and no edge case handling (`2.2.5`). The `Worktree` field in `LaunchOptions` (`types.go:103`) maps to Claude's `-w` flag but is not orchestrated by ralphglasses itself.

5. **No merge-back capability** [ROADMAP 2.2.3]: Workers commit to worktree branches but there is no mechanism to merge changes back to the main branch, detect conflicts, or abort.

6. **Marathon supervisor is pure bash** [ROADMAP 2.10]: `marathon.sh` reimplements budget reading (`read_spend()`, line 277), cost ledger writing (`write_cost_ledger()`, line 297), checkpoint tagging (`create_checkpoint()`, line 315), and signal handling (`cleanup()`, line 236) -- all of which already exist partially in Go. The bash implementation is fragile: `bc` dependency, `jq` dependency, BSD/GNU `sed` incompatibility workaround (line 152).

7. **Checkpoint has no tests** [ROADMAP 2.10.4]: `CreateCheckpoint()` (`checkpoint.go:12-50`) uses `git add -A` blindly (stages everything including potentially sensitive files) and has 0% test coverage.

8. **No persistence for teams or workflows**: `TeamStatus` and `WorkflowRun` are in-memory only. Team state is lost on restart.

---

## 3. Gap Analysis

### 3.1 ROADMAP Target vs Current State

| ROADMAP Item | Target | Current State | Gap |
|---|---|---|---|
| 2.1.1 | Define Session struct | Session struct exists with 25+ fields (`types.go:33-66`) | Partially met -- missing worktree_path, PID tracking, updated_at semantics |
| 2.1.2 | SQLite via modernc.org/sqlite | Pure JSON file persistence (`manager.go:719-734`) | **Full gap** -- no SQLite, no schema, no migrations |
| 2.1.3 | Session CRUD with prepared statements | In-memory maps + JSON files | **Full gap** -- no SQL, no prepared statements |
| 2.1.4 | Lifecycle state machine | Ad-hoc status assignments | **Full gap** -- no transition validation |
| 2.1.5 | Session event log table | Events published but not persisted | **Full gap** -- no event log persistence |
| 2.2.1 | internal/worktree/ package | Logic in loop.go, no package | **Full gap** -- package does not exist |
| 2.2.2 | Auto-create worktree on launch | `createLoopWorktree()` in loop.go | **Partial** -- works for loops only, not standalone launches |
| 2.2.3 | Merge-back with conflict detection | None | **Full gap** |
| 2.2.4 | Worktree cleanup on stop/archive | None | **Full gap** |
| 2.2.5 | Edge case handling | None | **Full gap** |
| 2.10.1 | Port marathon.sh to internal/marathon/ | 418 lines of bash | **Full gap** -- package does not exist |
| 2.10.2 | ralphglasses marathon subcommand | None | **Full gap** |
| 2.10.3 | Go os/signal for SIGINT/SIGTERM | Bash trap handlers | **Full gap** in Go, working in bash |
| 2.10.4 | Git checkpoint tagging in Go | `checkpoint.go` exists (50 lines) | **Partial** -- exists but untested, no marathon integration |
| 2.10.5 | Structured logging via slog | Bash `log()` function | **Full gap** -- no slog in session package |

### 3.2 Missing Capabilities

1. **Database layer**: No ORM, no migration framework, no connection pooling. The `modernc.org/sqlite` library is not in `go.mod`.
2. **State machine**: No FSM library or custom implementation. Status constants exist (`types.go:24-30`) but transitions are unconstrained.
3. **Worktree lifecycle management**: No worktree list, remove, prune, or status commands.
4. **Merge strategy**: No git merge orchestration, conflict resolution, or abort mechanics.
5. **Marathon Go binary**: No `cmd/marathon.go` or `internal/marathon/` package.
6. **Structured logging**: The session package uses no logging at all -- errors are silently swallowed (`manager.go:728-729`: `if err != nil { return }`).

### 3.3 Technical Debt Inventory

| Debt Item | Location | Severity | ROADMAP Item |
|---|---|---|---|
| Errors silently swallowed in PersistSession | `manager.go:728-729` | High | 2.1.2 |
| `git add -A` in checkpoint stages everything | `checkpoint.go:27-29` | Medium | 2.10.4 |
| 24h auto-cleanup with no user opt-out | `manager.go:832-845` | Medium | 2.1.2 |
| JSON file persistence has no atomic write (crash = corrupt) | `manager.go:733` (`os.WriteFile`) | High | 2.1.2 |
| Team and Workflow state not persisted | `manager.go` (in-memory maps only) | Medium | 2.1.3 |
| No session PID tracking after restart | `types.go:60` (cmd/cancel are `json:"-"`) | Medium | 2.1.1 |
| `LoadExternalSessions` re-persists in-process sessions via goroutine | `manager.go:809` | Low | 2.1.2 |
| `bc` and `jq` hard dependencies in marathon.sh | `marathon.sh:104-109` | Medium | 2.10.1 |
| Race in marathon.sh between RALPH_PID check and kill | `marathon.sh:354-357` | Low | 2.10.3 |

---

## 4. External Landscape

### 4.1 Competitor/Peer Projects

| Project | Relevance | Session Persistence | Worktree Pattern | Supervisor Pattern |
|---|---|---|---|---|
| **Claude Code SDK** (`@anthropic/claude-code`) | Direct upstream | SQLite for conversation history, file-based session state | Built-in `-w` flag creates worktrees automatically | N/A (single session) |
| **aider** (paul-gauthier/aider) | Peer AI coding tool | SQLite for chat history, JSON for settings | No worktree support | No supervisor; single-session |
| **Devon** (devin-ai/devon) | Multi-agent coding platform | PostgreSQL for task state, Redis for session cache | Git branch per task, auto-merge with conflict detection | Kubernetes-based supervisor with health checks |
| **SWE-agent** (princeton-nlp/SWE-agent) | Academic AI coding agent | JSON state files per episode | Docker-based isolation (no worktrees) | Bash runner with timeout |
| **mcpkit** (same org) | Sibling project | SQLite (WAL mode) for finops ledger | N/A | rdcycle perpetual loop in Go |
| **claudekit** (same org) | Sibling project | SQLite for budget profiles | N/A | rdcycle with budget profiles |

### 4.2 Patterns Worth Adopting

1. **SQLite WAL mode** (from mcpkit/claudekit): Write-Ahead Logging enables concurrent reads during writes, critical for TUI polling session state while the runner updates it. Reference: `modernc.org/sqlite` with `_journal_mode=WAL` pragma. Both sibling projects (mcpkit, claudekit) already use this pattern successfully.

2. **Finite State Machine with transition table**: Define allowed transitions as a map `map[Status][]Status` and validate before any status change. This is lighter than a full FSM library and matches the existing pattern in the codebase. The allowed transitions for session lifecycle should be:
   - `launching -> running, errored, stopped`
   - `running -> completed, errored, stopped`
   - `completed -> (terminal)`
   - `errored -> running (retry)`
   - `stopped -> running (resume)`

3. **Atomic file writes via temp + rename**: Devon and Claude Code SDK both use `os.CreateTemp()` followed by `os.Rename()` for crash-safe persistence. This pattern should replace the current `os.WriteFile()` in `PersistSession()`.

4. **Worktree-per-task with auto-cleanup**: Devon's pattern of creating one git branch per task, running the agent in its worktree, and auto-cleaning on completion maps directly to ROADMAP 2.2.4. The `git worktree remove` + `git branch -D` sequence should be gated by task completion status.

5. **Go supervisor with signal.NotifyContext**: Replace bash signal traps with `signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)` for clean goroutine cancellation. The `os/signal` approach integrates naturally with the existing `context.Context` cancellation chain.

### 4.3 Anti-Patterns to Avoid

1. **PostgreSQL/Redis for local-only state**: Devon's architecture requires network services. For a local TUI tool, SQLite is the right choice -- zero config, single file, embedded. Do not introduce external database dependencies.

2. **ORM abstraction layer**: For the ~5 tables needed (sessions, events, teams, workflows, loops), raw SQL with prepared statements is simpler, faster, and more debuggable than an ORM like GORM. The session package already uses `encoding/json` for serialization; SQL schemas can reuse the same field names.

3. **Worktree-per-session by default**: Creating a worktree for every session launch adds latency and disk usage. Keep it opt-in (via `LaunchOptions.Worktree`) for standalone launches. Only the loop system should auto-create worktrees.

4. **Blocking migration on startup**: Schema migrations should run fast (< 100ms for empty DB) and not block TUI rendering. Use `IF NOT EXISTS` and version stamps, not sequential migration files.

5. **Over-engineering the marathon supervisor**: The Go port should mirror `marathon.sh` behavior, not add features. The supervisor is intentionally simple: poll, check budget, check duration, checkpoint, restart. Keep it under 500 lines.

### 4.4 Academic & Industry References

1. **"SQLite as an Application File Format"** (sqlite.org/appfileformat.html): Authoritative argument for SQLite over custom file formats. Directly supports ROADMAP 2.1.2 rationale.

2. **"Supervision Trees" (Erlang/OTP)**: The marathon supervisor follows the OTP "one-for-one" restart strategy with exponential backoff. Go's `context` + `errgroup` provides equivalent semantics without the Erlang runtime.

3. **"Git Worktrees for Parallel Development"** (git-scm.com/docs/git-worktree): Official documentation covering the `add -B` pattern already used in `loop.go:540`, plus `remove`, `prune`, and `list` commands needed for ROADMAP 2.2.

4. **"Finite State Machines in Go"** (looplab/fsm): Popular Go FSM library. For this project, a hand-coded transition table is preferable (fewer dependencies, simpler debugging), but the API design is a useful reference.

---

## 5. Actionable Recommendations

### 5.1 Immediate Actions

| # | Action | Target Files | Effort | Impact | ROADMAP |
|---|---|---|---|---|---|
| 1 | Add SQLite dependency and create `internal/db/` package with schema (sessions, events, teams, workflows, loops tables) | `go.mod`, `internal/db/db.go`, `internal/db/schema.go`, `internal/db/migrate.go` | L | Critical | 2.1.2 |
| 2 | Implement session state machine: `TransitionTo(newStatus)` method with validation map | `internal/session/types.go`, `internal/session/statemachine.go` | S | High | 2.1.4 |
| 3 | Replace `PersistSession()` JSON writes with SQLite inserts/updates | `internal/session/manager.go:719-734` | M | Critical | 2.1.2, 2.1.3 |
| 4 | Add `LoadSessionsFromDB()` to replace `LoadExternalSessions()` file scanning | `internal/session/manager.go:786-846` | M | High | 2.1.3 |
| 5 | Add tests for `checkpoint.go` | `internal/session/checkpoint_test.go` | S | Medium | 2.10.4 |
| 6 | Add tests for `failover.go` | `internal/session/failover_test.go` | S | Medium | N/A (quality) |

### 5.2 Near-Term Actions

| # | Action | Target Files | Effort | Impact | ROADMAP |
|---|---|---|---|---|---|
| 7 | Create `internal/worktree/` package: `Create()`, `List()`, `Remove()`, `Prune()` | `internal/worktree/worktree.go`, `internal/worktree/worktree_test.go` | M | High | 2.2.1 |
| 8 | Implement merge-back: `MergeBack(worktreePath, targetBranch)` with conflict detection | `internal/worktree/merge.go`, `internal/worktree/merge_test.go` | M | High | 2.2.3 |
| 9 | Wire worktree auto-create into `Manager.Launch()` when `opts.Worktree != ""` | `internal/session/manager.go:96-132` | S | Medium | 2.2.2 |
| 10 | Add worktree cleanup to `Manager.Stop()` and session completion callback | `internal/session/manager.go:158-197`, `internal/session/runner.go:200-207` | S | Medium | 2.2.4 |
| 11 | Implement session event log: persist state transitions to SQLite events table | `internal/db/events.go`, `internal/session/manager.go` (inject on transition) | M | High | 2.1.5 |
| 12 | Create `internal/marathon/` package: port budget/duration/checkpoint loop | `internal/marathon/marathon.go`, `internal/marathon/marathon_test.go` | L | High | 2.10.1 |

### 5.3 Strategic Actions

| # | Action | Target Files | Effort | Impact | ROADMAP |
|---|---|---|---|---|---|
| 13 | Add `ralphglasses marathon` Cobra subcommand | `cmd/marathon.go` | M | High | 2.10.2 |
| 14 | Port signal handling to Go `os/signal` with context cancellation | `internal/marathon/signal.go` | S | Medium | 2.10.3 |
| 15 | Git checkpoint in Go with configurable file patterns (avoid `git add -A`) | `internal/session/checkpoint.go` refactor | S | Medium | 2.10.4 |
| 16 | Structured marathon logging via `slog` | `internal/marathon/marathon.go` | S | Medium | 2.10.5 |
| 17 | Handle worktree edge cases: dirty worktree on stop, orphaned branches, path conflicts | `internal/worktree/cleanup.go`, `internal/worktree/cleanup_test.go` | M | Medium | 2.2.5 |
| 18 | Persist teams and workflows to SQLite (currently in-memory only) | `internal/db/teams.go`, `internal/db/workflows.go` | M | Medium | 2.1.3 |
| 19 | Add CLI subcommands `ralphglasses worktree create|list|merge|clean` | `cmd/worktree.go` | M | Medium | 2.2 (implicit via 2.9.2) |
| 20 | Migrate JSON file persistence to SQLite migration path (keep JSON as export format) | `internal/session/manager.go`, `internal/db/migrate.go` | M | Medium | 2.1.2 |

---

## 6. Risk Assessment

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| SQLite CGo-free performance issues (`modernc.org/sqlite` is pure Go, ~2-5x slower than CGo `mattn/go-sqlite3`) | Medium | Low | WAL mode + prepared statements keep latency under 10ms for expected session counts (<1000). Benchmark before committing. |
| Worktree merge conflicts block automated loops | High | High | Implement `MergeBack()` with `--no-commit` dry-run first; auto-abort on conflict and leave worktree intact for manual resolution. Publish `MergeConflict` event for TUI notification. |
| Marathon Go port introduces behavioral regressions vs bash | Medium | Medium | Write integration tests that run both bash and Go supervisors against a mock ralph process and compare budget/checkpoint behavior. Keep `marathon.sh` as fallback until Go port passes all tests. |
| SQLite file locking conflicts between TUI and MCP server (separate processes) | Medium | High | Use WAL mode (allows concurrent readers) and `_busy_timeout=5000` pragma. Both processes can read; only one writes at a time. Current JSON file system has the same issue but fails silently. |
| `git worktree add` fails on repos with detached HEAD or shallow clones | Low | Medium | Pre-validate repo state in `worktree.Create()`: check for `.git` directory (not file, which indicates existing worktree), verify HEAD is attached, reject shallow clones. |
| Breaking change to Session JSON format when adding SQLite | Medium | Low | Implement import path: `LoadExternalSessions()` reads legacy JSON files and inserts into SQLite on first run, then deletes JSON files. Tag this as a migration in the version string. |
| Checkpoint `git add -A` stages sensitive files (.env, credentials) | Medium | High | Replace with explicit file pattern: `git add -u` (tracked files only) or configurable `.ralphignore` pattern list. Never stage `.env`, `.ralph/logs/`, or files matching `.gitignore`. |

---

## 7. Implementation Priority Ordering

### 7.1 Critical Path

The dependency chain for ROADMAP 2.1/2.2/2.10 is:

```
2.1.2 (SQLite) ──> 2.1.3 (CRUD) ──> 2.1.4 (State machine) ──> 2.1.5 (Event log)
                         │
                         ├──> 2.2.1 (Worktree pkg) ──> 2.2.2 (Auto-create) ──> 2.2.3 (Merge)
                         │                                                          │
                         │                                                          └──> 2.2.4 (Cleanup) ──> 2.2.5 (Edge cases)
                         │
                         └──> Persist teams/workflows

2.10.1 (Marathon pkg) ──> 2.10.2 (CLI) ──> 2.10.3 (Signals) ──> 2.10.4 (Checkpoints) ──> 2.10.5 (slog)
```

**2.10 is fully independent** of 2.1 and 2.2 and can proceed in parallel.

### 7.2 Recommended Sequence

**Sprint 1 (Foundation)**:
1. `internal/db/` package with SQLite schema and WAL mode (**2.1.2**) -- Effort: L
2. State machine `TransitionTo()` (**2.1.4**) -- Effort: S
3. Tests for `checkpoint.go` and `failover.go` -- Effort: S

**Sprint 2 (Persistence)**:
4. Session CRUD via SQLite, replacing JSON persistence (**2.1.3**) -- Effort: M
5. Session event log table (**2.1.5**) -- Effort: M
6. Persist teams and workflows to SQLite -- Effort: M

**Sprint 3 (Worktrees)**:
7. `internal/worktree/` package with Create/List/Remove (**2.2.1**) -- Effort: M
8. Auto-create worktree on session launch (**2.2.2**) -- Effort: S
9. Merge-back with conflict detection (**2.2.3**) -- Effort: M
10. Cleanup on stop/archive (**2.2.4**) + edge cases (**2.2.5**) -- Effort: M

**Sprint 4 (Marathon Port)**:
11. `internal/marathon/` package (**2.10.1**) -- Effort: L
12. `ralphglasses marathon` subcommand (**2.10.2**) -- Effort: M
13. Go signal handling (**2.10.3**) + checkpoint refactor (**2.10.4**) -- Effort: S
14. Structured slog logging (**2.10.5**) -- Effort: S

### 7.3 Parallelization Opportunities

Three independent workstreams can proceed simultaneously:

| Workstream | Items | Dependencies | Estimated Effort |
|---|---|---|---|
| **A: Data Model & Persistence** | 2.1.2, 2.1.3, 2.1.4, 2.1.5 | Sequential within stream | XL (combined) |
| **B: Git Worktrees** | 2.2.1, 2.2.2, 2.2.3, 2.2.4, 2.2.5 | Needs 2.1.3 for persistence | L (combined), starts Sprint 2 |
| **C: Marathon Go Port** | 2.10.1, 2.10.2, 2.10.3, 2.10.4, 2.10.5 | Fully independent | L (combined), starts Sprint 1 |

Workstream C (marathon) has zero dependencies on A or B and should begin immediately in parallel. Workstream B requires the Session CRUD from A (Sprint 2) to persist worktree associations, but the `internal/worktree/` package design can begin during Sprint 1.

Within each workstream, items are sequential -- each builds on the prior. Across workstreams, only B depends on A (specifically, 2.2.2 needs 2.1.3 to associate worktrees with persisted sessions).
