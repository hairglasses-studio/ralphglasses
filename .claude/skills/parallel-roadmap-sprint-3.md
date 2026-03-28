---
name: parallel-roadmap-sprint-3
description: Mega sprint — Phase 0.6 observability, Phase 0.7 hardening, Phase 1.2.5 MCP framework, Phase 9 Tier 2 rdcycle, Phase 2.1 SQLite data model
user-invocable: true
argument-hint: [workstream-filter]
---

You are executing Sprint 3 for the ralphglasses project — the largest sprint yet. This sprint targets 5 major epics across 8 parallel workstreams, advancing the project from 28% to ~45% completion.

## Project Context

Ralphglasses is a Go TUI + MCP server (120 tools, 14 namespaces) for managing parallel multi-LLM agent fleets. 37 packages, 82.6% test coverage, 36/36 packages green. Uses Charmbracelet BubbleTea v1, mark3labs/mcp-go v0.45.0, modernc.org/sqlite.

**Sprints 1–2 completed:** 12 QW bug fixes, doc sync, self-improvement pipeline, 5 rdcycle tool implementations, score hardening, fleet reaper, cascade default, autonomy persistence.

## Pre-flight

Read these files to confirm current state:
- `ROADMAP.md` — 1661 lines, 619 tasks, 29 phases
- `internal/mcpserver/handler_rdcycle.go` — 637 lines, 5 tools implemented
- `internal/mcpserver/tools.go` — Server struct, ToolGroupNames, registration
- `docs/ARCHITECTURE.md` — Package layout reference
- `.ralph/tool_improvement_scratchpad.md` — Unresolved findings

## Epic 1: Phase 0.6 Code Quality & Observability (28 tasks)

### WS-1: Error Path Coverage (0.6.1)
**Goal:** Add tests for error paths in 4 key packages.
**Files:** Test files only — `*_test.go` in these packages:
- `internal/discovery/` — scan failure, permission errors, symlink loops
- `internal/model/` — invalid config parse, missing fields, malformed JSON
- `internal/process/` — exec failure, SIGTERM timeout, zombie process
- `internal/enhancer/` — API timeout, rate limit, malformed response

For each package:
1. Read existing tests to understand patterns
2. Add error-path tests: nil inputs, missing files, permission denied, timeout
3. Target: each package error coverage ≥80%

**Verification:** `go test ./internal/discovery/... ./internal/model/... ./internal/process/... ./internal/enhancer/... -count=1 -coverprofile=cover.out && go tool cover -func=cover.out | grep -E "total:"`

### WS-2: Observation Enrichment (0.6.2)
**Goal:** Add 4 new fields to loop observations for better diagnostics.
**Files:** `internal/session/loop_steps.go`, `internal/session/types.go` (observation struct)

New fields to add to the observation struct:
1. `GitDiffStat string` — `git diff --stat HEAD` output after worker completes
2. `PlannerModelUsed string` — actual model used by planner (from session metadata)
3. `AcceptancePath string` — which acceptance check triggered (build/test/vet/custom)
4. `WorkerEnhancementSource string` — how prompt was enhanced (none/local/api)

For each field:
1. Find the observation struct (grep for `type.*Observation struct` in session package)
2. Add the field with JSON tag
3. Populate it in the relevant step of `StepLoop`
4. Write test verifying the field is populated

**Verification:** `go test ./internal/session/... -run "Observation" -count=1 -v`

### WS-3: Loop Config Validation (0.6.3)
**Goal:** Validate loop profile configuration before execution.
**Files:** `internal/session/loop.go`, `internal/session/loop_types.go`

Add `ValidateLoopProfile(p LoopProfile) error` that checks:
1. `PlannerProvider` is a valid provider (claude/gemini/openai)
2. `WorkerProvider` is a valid provider
3. `MaxIterations` > 0 or `MaxDurationSecs` > 0 (at least one limit)
4. `MaxConcurrentWorkers` 1-10 range
5. `PlannerBudgetUSD` > 0 and `WorkerBudgetUSD` > 0
6. `RetryLimit` 0-10 range
7. `StallTimeout` non-negative

Call from `StartLoop()` before creating the run. Return descriptive errors.

**Verification:** `go test ./internal/session/... -run "ValidateLoop" -count=1 -v`

### WS-4: Gate Report Formatting (0.6.4) + Worktree Cleanup (0.6.6) + Planner Dedup (0.6.7)
**Goal:** Three smaller items bundled.
**Files:** `internal/e2e/gates.go`, `internal/session/worktree.go`, `internal/session/loop_steps.go`

**Gate report formatting:**
- Add `FormatReport(r *GateReport) string` — human-readable markdown table
- Add `FormatReportJSON(r *GateReport) ([]byte, error)` — JSON with all fields
- Include overall verdict, per-metric verdicts, delta percentages

**Worktree cleanup robustness:**
- Find `CleanupStaleWorktrees` (already exists in loop.go)
- Add lock detection: skip worktrees with `.git/index.lock` present
- Add age-based cleanup: only remove worktrees older than threshold
- Log which worktrees were cleaned vs skipped

**Planner task deduplication:**
- In `plannerTasksFromSession()` or task parsing, add dedup:
  - Compute title similarity (lowercase Jaccard on words)
  - If similarity > 0.8 with any previously completed task, skip
  - Track completed tasks in `LoopRun.CompletedTasks []string`

**Verification:** `go test ./internal/e2e/... ./internal/session/... -count=1`

---

## Epic 2: Phase 0.7 Hardening (11 tasks)

### WS-5: Session Stall Detection + Config Validation (0.7)
**Goal:** Harden session lifecycle and configuration.
**Files:** `internal/session/stall.go`, `internal/model/config.go`, `internal/config/`

**Stall detection enhancement (0.6.5):**
- The `StallDetector` already exists. Enhance:
  - Add `OnStall(callback func(sessionID string))` hook for custom actions
  - Wire into `Manager.waitForSession()` — if stall detected, emit event + optionally kill
  - Add stall count to session metadata for fleet dashboard

**Config validation strictness (0.5.11):**
- Read `internal/model/config.go` — find `KnownKeys` registry
- Add `ValidateConfig(cfg map[string]string) []ConfigWarning` that:
  - Warns on unknown keys (not in KnownKeys)
  - Warns on out-of-range values (e.g., negative timeouts)
  - Warns on deprecated keys
- Return `[]ConfigWarning{Key, Message, Severity}`
- Call from `ApplyConfig()`, log warnings

**Marathon resource checks (0.5.10):**
- Read `distro/scripts/marathon.sh` or `internal/session/` for marathon integration
- Add disk space check before loop start: `if free < 1GB, warn`
- Add memory pressure check: read `/proc/meminfo` or `vm_stat` (macOS)
- Add log rotation: truncate `.ralph/logs/` files > 100MB

**Verification:** `go test ./internal/session/... ./internal/model/... ./internal/config/... -count=1`

---

## Epic 3: Phase 1.2.5 MCP Handler Framework (4 tasks)

### WS-6: Handler Abstraction + Middleware
**Goal:** Reduce handler boilerplate by 40%. Currently 81 `getStringArg`/`getNumberArg` calls are scattered across 57 handler files.
**Files:** `internal/mcpserver/` — new files + refactor existing

**Step 1: Define ParamParser helper**
Create `internal/mcpserver/params.go`:
```go
type Params struct {
    req mcp.CallToolRequest
}

func NewParams(req mcp.CallToolRequest) *Params
func (p *Params) RequireString(key string) (string, error)  // returns codedError if missing
func (p *Params) OptionalString(key, defaultVal string) string
func (p *Params) RequireNumber(key string) (float64, error)
func (p *Params) OptionalNumber(key string, defaultVal float64) float64
func (p *Params) RequireBool(key string) (bool, error)
func (p *Params) OptionalBool(key string, defaultVal bool) bool
```

**Step 2: Standardize error codes**
Create `internal/mcpserver/errcodes.go`:
- Ensure all error codes are defined as typed constants
- Add `ErrTimeout`, `ErrPermissionDenied`, `ErrRateLimited` if missing
- Add `ErrorResponse(code ErrorCode, msg string) *mcp.CallToolResult` wrapper

**Step 3: Refactor 5 handlers as proof-of-concept**
Pick 5 handlers with the most `getStringArg` calls. Refactor to use `Params`:
- `handleSessionLaunch` (handler_session_lifecycle.go)
- `handleFleetSubmit` (handler_fleet.go)
- `handleLoopStart` (handler_loop.go)
- `handleScratchpadAppend` (handler_scratchpad.go)
- `handleRoadmapAnalyze` (handler_roadmap.go)

**Step 4: Add middleware chain**
If `internal/mcpserver/middleware.go` already exists, enhance it. Otherwise create:
- `type HandlerMiddleware func(next ToolHandler) ToolHandler`
- Built-in: `LoggingMiddleware` (log tool name, duration, error), `TimeoutMiddleware` (context deadline)
- Apply to all handlers via dispatch chain in `tools_dispatch.go`

**Verification:** `go test ./internal/mcpserver/... -count=1 -timeout 60s`

---

## Epic 4: Phase 9 R&D Cycle Tier 2 (5 new tools)

### WS-7: RDCycle Tier 2 Tools
**Goal:** Implement 5 Tier 2 rdcycle tools that complete the R&D automation pipeline.
**Files:** `internal/mcpserver/handler_rdcycle.go`, `internal/mcpserver/tools_builders_misc.go`

**1. loop_replay** — Replay a failed loop iteration with modified parameters
- Input: `loop_id` (string), `iteration` (number), `overrides` (JSON string — model, provider, budget)
- Logic: Load loop run, find iteration, clone its config with overrides, launch new session
- Output: new_session_id, original_error, overrides_applied

**2. budget_forecast** — Predict cost of N more iterations
- Input: `loop_id` (string), `iterations` (number, default 10)
- Logic: Read cost_observations.json, compute cost per iteration (P50/P95), project forward
- Output: estimated_cost_p50, estimated_cost_p95, confidence_pct, cost_per_iteration_history

**3. diff_review** — Auto-review a git diff for quality issues
- Input: `repo` (string), `ref` (string, default "HEAD"), `checks` (CSV: scope_creep,missing_tests,style)
- Logic: Run `git diff <ref>~1..<ref>`, parse hunks, check for: files outside task scope, no test changes when code changed, TODO left behind
- Output: issues[], severity, file, line, recommendation

**4. scratchpad_reason** — Analyze scratchpad findings for root causes
- Input: `name` (string), `repo` (string, optional)
- Logic: Read scratchpad, group findings by category, identify recurring themes, extract root causes from patterns
- Output: root_causes[], affected_findings[], recommended_actions[]

**5. observation_correlate** — Link observations to git commits
- Input: `repo` (string), `hours` (number, default 24)
- Logic: Read observations from `.ralph/observations/`, run `git log --since=<hours>h`, match timestamps, correlate session_id to commit author/message
- Output: correlations[{observation_id, commit_sha, commit_msg, session_id, delta_lines}]

For each tool:
1. Add handler in `handler_rdcycle.go`
2. Add tool definition in `buildRdcycleGroup()` in `tools_builders_misc.go`
3. Add annotation in `annotations.go`
4. Update `tools_deferred_test.go` expected count (rdcycle: 5→10)

**Verification:** `go build ./...` && `go test ./internal/mcpserver/... -count=1`

---

## Epic 5: Phase 2.1 Session Data Model (SQLite)

### WS-8: SQLite Persistence Layer
**Goal:** Replace in-memory session store with SQLite for persistence, querying, and fleet analytics.
**Files:** New `internal/session/sqlite.go`, modify `internal/session/manager.go`

**Step 1: Schema**
The `Store` interface likely already exists (grep for `type Store interface` in session package). If not, define:
```go
type Store interface {
    SaveSession(s *Session) error
    GetSession(id string) (*Session, error)
    ListSessions(filter SessionFilter) ([]*Session, error)
    DeleteSession(id string) error
    SaveLoopRun(r *LoopRun) error
    GetLoopRun(id string) (*LoopRun, error)
    ListLoopRuns(filter LoopFilter) ([]*LoopRun, error)
}
```

**Step 2: SQLite implementation**
Use `modernc.org/sqlite` (already in go.mod — pure Go, no CGO). Create tables:
```sql
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    repo_path TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    model TEXT,
    prompt TEXT,
    budget_usd REAL DEFAULT 0,
    spent_usd REAL DEFAULT 0,
    turn_count INTEGER DEFAULT 0,
    launched_at DATETIME,
    ended_at DATETIME,
    last_error TEXT,
    metadata TEXT -- JSON blob for extensibility
);

CREATE TABLE loop_runs (
    id TEXT PRIMARY KEY,
    repo_path TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    profile TEXT NOT NULL, -- JSON blob
    iteration_count INTEGER DEFAULT 0,
    created_at DATETIME,
    updated_at DATETIME,
    deadline DATETIME,
    last_error TEXT
);

CREATE TABLE cost_ledger (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT REFERENCES sessions(id),
    provider TEXT NOT NULL,
    spend_usd REAL NOT NULL,
    turn_count INTEGER,
    elapsed_sec REAL,
    model TEXT,
    recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

**Step 3: Wire into Manager**
- `Manager.store` field already exists (type `Store`)
- `NewManager()` should accept a `Store` option
- Add `NewSQLiteStore(dbPath string) (*SQLiteStore, error)`
- Default DB path: `~/.ralphglasses/ralph.db`
- Fallback: if SQLite init fails, use `MemoryStore` (current behavior)

**Step 4: Migration**
- On first open, run `CREATE TABLE IF NOT EXISTS` for all tables
- Add schema version table for future migrations

**Verification:** `go test ./internal/session/... -run "SQLite|Store" -count=1 -v`

---

## Execution Plan

```
Phase 1 — Parallel (8 worktree agents, non-overlapping files):

  WS-1 (error paths)    WS-2 (observation)    WS-3 (loop config)    WS-4 (gates/worktree/dedup)
  ─────────────────      ─────────────────      ─────────────────     ──────────────────────────

  WS-5 (stall/config)   WS-6 (MCP framework)   WS-7 (rdcycle T2)    WS-8 (SQLite)
  ─────────────────      ─────────────────       ─────────────────     ──────────────
       ↘                        ↓                      ↓                    ↙
        └───────────────────────┴──────────────────────┴───────────────────┘
                                         ↓
Phase 2 — Sequential:              Integration + docs
                                         ↓
                                   Commit & verify
```

## Resource Conflict Matrix

| File | WS-1 | WS-2 | WS-3 | WS-4 | WS-5 | WS-6 | WS-7 | WS-8 |
|------|------|------|------|------|------|------|------|------|
| `session/loop_steps.go` | | **W** | | R | | | | |
| `session/loop.go` | | | **W** | | | | | |
| `session/loop_types.go` | | **W** | **W** | | | | | |
| `session/stall.go` | | | | | **W** | | | |
| `session/manager.go` | | | | | | | | **W** |
| `session/worktree.go` | | | | **W** | | | | |
| `e2e/gates.go` | | | | **W** | | | | |
| `model/config.go` | | | | | **W** | | | |
| `mcpserver/handler_rdcycle.go` | | | | | | | **W** | |
| `mcpserver/tools_builders_misc.go` | | | | | | | **W** | |
| `mcpserver/params.go` (NEW) | | | | | | **W** | | |
| `mcpserver/middleware.go` | | | | | | **W** | | |
| `mcpserver/handler_session_lifecycle.go` | | | | | | **W** | | |
| `session/sqlite.go` (NEW) | | | | | | | | **W** |
| `discovery/*_test.go` | **W** | | | | | | | |
| `model/*_test.go` | **W** | | | | | | | |
| `process/*_test.go` | **W** | | | | | | | |
| `enhancer/*_test.go` | **W** | | | | | | | |

**Conflict: WS-2 and WS-3 both write `loop_types.go`.** Resolution: WS-2 adds observation fields to the observation struct. WS-3 adds validation to LoopProfile. These are different structs — no conflict unless the file is too small. Merge manually if needed.

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| WS-6 handler refactor breaks tools | Medium | High | Refactor only 5 handlers as POC; keep getStringArg working |
| WS-8 SQLite init fails on some systems | Low | Medium | Fallback to MemoryStore; modernc.org/sqlite is pure Go, no CGO |
| WS-7 rdcycle Tier 2 tools are too complex | Medium | Medium | Keep implementations simple; no external API calls |
| WS-2/WS-3 loop_types.go merge conflict | Medium | Low | Different structs; manual merge if needed |
| Phase 0.6 tests reveal new bugs | Medium | Low | Fix as found; don't scope-creep into Sprint 4 items |
| Config validation breaks existing .ralphrc files | Low | Medium | Warnings only, never reject configs |

## Integration Sequence

After all parallel workstreams merge:
1. `go build ./...` — verify compilation
2. `go vet ./...` — static analysis
3. `go test ./... -count=1 -timeout 120s` — full suite
4. `go test ./... -coverprofile=cover.out && go tool cover -func=cover.out | tail -1` — coverage check
5. Update CLAUDE.md: tool count (120→125), add Phase 0.6/0.7 completion notes
6. Update docs/MCP-TOOLS.md: add 5 new rdcycle Tier 2 tool entries
7. Update ROADMAP.md: mark completed phases

## Acceptance Criteria

| Epic | Metric | Target |
|------|--------|--------|
| Phase 0.6 error paths | Package coverage | ≥80% for discovery, model, process, enhancer |
| Phase 0.6 observations | New fields populated | 4 fields non-empty in test |
| Phase 0.6 loop config | Invalid profiles rejected | ValidateLoopProfile catches 7 error types |
| Phase 0.6 gate reports | Formatted output | Markdown + JSON formatters, tests pass |
| Phase 0.7 stall detection | Stall callback fires | OnStall hook invoked within 2x timeout |
| Phase 0.7 config validation | Unknown keys warned | ValidateConfig returns warnings for bad keys |
| Phase 1.2.5 params | ParamParser works | 5 handlers refactored, tests pass |
| Phase 1.2.5 middleware | Logging middleware | All tool calls logged with duration |
| Phase 9 Tier 2 | 5 tools functional | Each tool returns real data, schema valid |
| Phase 2.1 SQLite | Sessions persisted | Save/Get/List round-trip, cost ledger works |

## Constraints

- Each parallel workstream operates on non-overlapping files (see matrix)
- Do NOT modify `ROADMAP.md` during workstreams — only in integration phase
- Do NOT add external dependencies — use standard library + existing deps
- Every workstream must end with `go build ./...` and `go vet ./...` passing
- Use `/bin/cp` (not `cp`) when copying files between worktrees (macOS alias)
- If `$ARGUMENTS` contains a workstream filter (e.g., "ws-1" or "ws-1,ws-7"), only run those

## Post-Sprint

After all workstreams complete:
1. `go test ./... -count=1 -timeout 120s` — full green
2. Coverage report: target ≥85% overall (currently 82.6%)
3. Update `.ralph/tool_improvement_scratchpad.md` — mark resolved findings
4. Report: phase completion deltas, new tool count, coverage delta, remaining for sprint 4
