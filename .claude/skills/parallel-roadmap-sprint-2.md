---
name: parallel-roadmap-sprint-2
description: Execute remaining 12 QW items + 5 rdcycle implementations across 5 parallel workstreams with dependency analysis and risk management
user-invocable: true
argument-hint: [workstream-filter]
---

You are executing the second parallel roadmap sprint for the ralphglasses project. Sprint 1 (commit 41c008d) delivered foundations: JSON pre-check, cascade init, zero-baseline guard, score baseline fix, pattern threshold fix, rdcycle stubs, and doc sync. This sprint implements the **12 remaining QW items** and **5 rdcycle real implementations**.

## Pre-flight

Read these files to confirm current state before launching agents:
- `ROADMAP.md` — Phase 0.9 QW items (all 12 marked incomplete)
- `internal/mcpserver/handler_rdcycle.go` — 5 stub handlers from sprint 1
- `.ralph/tool_improvement_scratchpad.md` — Finding details for each QW item
- `internal/session/manager.go` — Cascade init already wired (lines 168-173)
- `internal/e2e/gates.go` — Zero-baseline guard already in place (lines 177-182)
- `internal/enhancer/scoring.go` — Baselines already lowered 50→30

## Dependency Map

```
Independent (no cross-workstream dependencies):
  WS-A ─── Session Lifecycle (QW-1, QW-3, QW-8)
  WS-B ─── Loop & Scoring (QW-6, QW-10)
  WS-C ─── Config & Persistence (QW-2, QW-9)
  WS-D ─── Prompt Enhancer (QW-4, QW-5)
  WS-E ─── System Hardening (QW-7, QW-11, QW-12)

Sequential (after all above):
  WS-F ─── RDCycle Real Implementations (5 tools)
  WS-G ─── Integration Tests + Doc Finalization
```

**Cross-workstream data flows:**
- QW-1 (JSON enforcement) and QW-3 (turn cap) both touch session loop — assigned to same workstream to avoid conflict
- QW-8 (budget wiring) touches `handler_session.go` — no other WS touches this file
- QW-12 (rules extraction) touches `reflexion.go` — no other WS touches this file
- WS-F depends on WS-A/B completing (rdcycle tools read session/loop state)

## Shared Contracts

These interfaces and types are shared across workstreams. Do NOT modify their signatures:

```go
// internal/session/types.go — Session, LaunchOptions, Manager
// internal/mcpserver/types.go — Server, ToolGroup, ToolDef
// internal/e2e/gates.go — Verdict type (VerdictPass/VerdictFail/VerdictSkip/VerdictWarn)
// internal/enhancer/types.go — ScoreResult, LintResult, EnhanceResult
```

**New fields may be ADDED to structs. Existing fields must NOT be renamed or removed.**

## Workstream Definitions

### WS-A: Session Lifecycle (QW-1, QW-3, QW-8)
**Goal:** Fix JSON retry storms, cap runaway workers, enforce budget params.
**Files:** `internal/session/loop_worker.go`, `internal/session/loop.go`, `internal/session/session.go`, `internal/mcpserver/handler_session.go`

**QW-1: JSON format enforcement (<5% retry rate)**
- File: `internal/session/loop_worker.go`
- Sprint 1 added `looksLikeJSON()` pre-check in `loop_steps.go:216`. Now add:
  1. Schema validation wrapper: before parsing planner output, validate JSON structure
  2. Retry with format reminder: on first JSON failure, append "Respond with valid JSON only" to next prompt
  3. Hard fail after 2 retries (not infinite loop)
- Acceptance: JSON retry rate drops from 25.7% to <5%

**QW-3: Cap worker turns at 20**
- File: `internal/session/loop.go` (add `MaxWorkerTurns` field, default 20)
- File: `internal/session/session.go` (enforce in step loop, return error when exceeded)
- Acceptance: No session exceeds 20 worker turns; `signal:killed` drops to 0

**QW-8: Budget params not ignored**
- File: `internal/mcpserver/handler_session.go` — wire `budget_usd` and `max_turns` from request args into `LaunchOptions`
- File: `internal/mcpserver/tools_session.go` — ensure schema declares these params
- Acceptance: `session_launch` with `budget_usd=0.50` actually constrains spending

**Verification:** `go build ./...` && `go vet ./...` && `go test ./internal/session/... ./internal/mcpserver/... -count=1`

---

### WS-B: Loop & Scoring (QW-6, QW-10)
**Goal:** Fix loop gates baseline persistence and flat relevance scoring.
**Files:** `internal/e2e/gates.go`, `internal/mcpserver/tools_roadmap.go`

**QW-6: Loop gates baseline persistence**
- File: `internal/e2e/gates.go`
- Sprint 1 added zero-baseline skip guard. Now add:
  1. `SaveBaseline()` function that writes baseline to `.ralph/baselines/<repo>.json`
  2. `LoadBaseline()` that reads persisted baseline on startup
  3. On first observation, auto-save as baseline instead of using zeros
- Acceptance: Loop gates produce valid verdicts on first run (not skip/zero)

**QW-10: Relevance scoring (flat @ 0.5)**
- File: `internal/mcpserver/tools_roadmap.go` — find the relevance scoring logic
- Implement keyword overlap scoring: extract keywords from query, compute Jaccard similarity against roadmap item text
- Score range should have stddev > 0.15 (not all 0.5)
- Acceptance: `roadmap_analyze` returns varied relevance scores

**Verification:** `go build ./...` && `go test ./internal/e2e/... ./internal/mcpserver/... -run "Gate|Roadmap" -count=1`

---

### WS-C: Config & Persistence (QW-2, QW-9)
**Goal:** Enable cascade routing by default, persist autonomy level across restarts.
**Files:** `internal/session/manager.go`, `internal/session/autooptimize.go`, config defaults

**QW-2: Enable cascade routing by default**
- Sprint 1 wired `DefaultCascadeFromConfig()` in `manager.go:168-173` when CASCADE_ENABLED=true
- Now: ensure the default config value for CASCADE_ENABLED is `true` (check `internal/session/config.go` or wherever defaults are set)
- Add cascade routing section to CLAUDE.md under Key Patterns
- Acceptance: New sessions use cascade routing without explicit config

**QW-9: Persist autonomy level**
- File: `internal/session/autooptimize.go`
- Add `SaveAutonomyLevel(level int)` → writes to `.ralph/autonomy.json`
- Add `LoadAutonomyLevel() int` → reads from `.ralph/autonomy.json`, defaults to current default
- Wire into `SetAutonomyLevel()` to persist on change, load on `NewManager()`
- Acceptance: Autonomy level survives process restart

**Verification:** `go build ./...` && `go test ./internal/session/... -run "Config|Autonomy|Cascade" -count=1`

---

### WS-D: Prompt Enhancer (QW-4, QW-5)
**Goal:** Fix score inflation clustering and add pipeline stage transparency.
**Files:** `internal/enhancer/scoring.go`, `internal/enhancer/analyze.go`, `internal/enhancer/pipeline.go`

**QW-4: Fix score inflation (8-9 cluster)**
- Sprint 1 lowered baselines 50→30. Now:
  1. File: `scoring.go` — add negative signal detection (penalty for vague language, excessive hedging, no examples)
  2. File: `analyze.go` — normalize final score distribution to 3-9 range
  3. Add calibration test with known-bad prompts that should score < 5
- Acceptance: Score distribution spans 3-9, not 8-9

**QW-5: Pipeline stage transparency**
- File: `pipeline.go`
- Add `SkippedStages []SkipReason` field to `EnhanceResult` (or equivalent result struct)
- Each stage that short-circuits records its name + skip reason
- `prompt_enhance` tool output includes skipped stages in response
- Acceptance: Users can see which pipeline stages fired and which were skipped

**Verification:** `go build ./...` && `go test ./internal/enhancer/... -count=1`

---

### WS-E: System Hardening (QW-7, QW-11, QW-12)
**Goal:** Fix snapshot paths, clean phantom fleet tasks, populate improvement rules.
**Files:** `internal/session/snapshot.go`, `internal/fleet/coordinator.go`, `internal/session/reflexion.go`

**QW-7: Snapshot path fix (claudekit → ralphglasses)**
- File: `internal/session/snapshot.go`
- Find hardcoded or inherited claudekit path references
- Change snapshot save location to `.ralph/snapshots/`
- Acceptance: `ralphglasses_snapshot` writes to correct path

**QW-11: Clean phantom fleet tasks**
- File: `internal/fleet/coordinator.go`
- Add `ReapStaleTasks()` method: mark tasks stale if repo path doesn't exist or task age > 1h with no progress
- Add `ValidateRepoPath()` on fleet_submit: reject submissions for non-existent repos
- Call reaper on `fleet_status` if stale count > 50%
- Acceptance: Phantom task rate drops from 73% to <5%

**QW-12: improvement_patterns.json rules extraction**
- Sprint 1 lowered consolidation threshold 3→2 and added auto-generate rules from patterns
- File: `internal/session/reflexion.go` — verify `ExtractReflection` populates structured verbal reflections
- Ensure `ConsolidatePatterns` in `journal.go` actually writes non-null rules to `improvement_patterns.json`
- Run a manual consolidation pass to validate
- Acceptance: `improvement_patterns.json` has ≥3 rules (not null)

**Verification:** `go build ./...` && `go test ./internal/session/... ./internal/fleet/... -run "Snapshot|Fleet|Reap|Reflexion|Journal" -count=1`

---

### WS-F: RDCycle Real Implementations (sequential, after WS-A through WS-E)
**Goal:** Replace 5 rdcycle handler stubs with real implementations.
**File:** `internal/mcpserver/handler_rdcycle.go`
**Depends on:** WS-A, WS-B (session/loop fixes must be stable first)

**1. finding_to_task** (lines 12-34)
- Read scratchpad file from `.ralph/` by name
- Parse findings (grep for `FINDING-` prefix or `#` numbered items)
- Match `finding_id` parameter to finding text
- Generate task spec: title, description, difficulty (1-5 heuristic from word count/complexity), provider_hint ("claude" for complex, "gemini" for simple), estimated_cost

**2. cycle_baseline** (lines 36-68)
- Run `go test -count=1 -coverprofile` in specified repo
- Parse coverage percentage from output
- Count test files and test functions
- Record baseline to `.ralph/cycle_baselines/<timestamp>.json`
- Return baseline_id + metrics snapshot

**3. cycle_plan** (lines 71-89)
- Read all scratchpad files from `.ralph/`
- Extract unresolved findings (no `[DONE]` or `[RESOLVED]` prefix)
- Score by: recurrence count (from improvement_patterns.json) + severity keyword detection
- Sort descending, limit to `max_tasks` parameter
- Filter by `budget` parameter (estimate cost per task)

**4. cycle_merge** (lines 92-113)
- Accept `worktree_paths` array
- For each path, run `git diff` to collect changes
- Attempt sequential merge using `git apply` or file copy
- Detect conflicts (same file modified in multiple worktrees)
- Return merge result with conflicts list and resolution hints

**5. cycle_schedule** (lines 116-135)
- Parse cron expression (use simple 5-field parser or Go cron library)
- Write schedule config to `.ralph/schedules/<id>.json`
- Return schedule_id and next 3 run times

**Verification:** `go build ./...` && `go test ./internal/mcpserver/... -run "RDCycle|Rdcycle|rdcycle" -count=1`

---

### WS-G: Integration Tests + Doc Finalization (sequential, after WS-F)
**Goal:** Full test pass, coverage report, documentation update.
**Depends on:** All previous workstreams.

Tasks:
1. Run full suite: `go test ./... -count=1 -timeout 120s`
2. Run coverage: `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | tail -1`
3. Write regression tests for each QW fix (at minimum one test per QW item)
4. Update CLAUDE.md: tool count (120 tools, 14 namespaces), add rdcycle namespace description
5. Update docs/MCP-TOOLS.md: rdcycle tool descriptions (no longer stubs)
6. Update `.ralph/tool_improvement_scratchpad.md` with resolved findings

## Execution Plan

```
Phase 1 — Parallel (worktree agents, non-overlapping files):

  WS-A (session)    WS-B (loop/scoring)    WS-C (config)    WS-D (enhancer)    WS-E (hardening)
  ──────────────     ──────────────────     ─────────────    ───────────────     ────────────────
       ↘                    ↓                    ↓                ↓                   ↙
        └───────────────────┴────────────────────┴────────────────┴──────────────────┘
                                              ↓
Phase 2 — Sequential (main branch):        WS-F (rdcycle real implementations)
                                              ↓
Phase 3 — Sequential (main branch):        WS-G (tests + docs)
                                              ↓
                                         Commit & verify
```

Launch WS-A through WS-E as parallel agents in isolated worktrees. After all 5 complete and merge, run WS-F then WS-G sequentially on main.

## Resource Conflict Matrix

| File | WS-A | WS-B | WS-C | WS-D | WS-E | WS-F |
|------|------|------|------|------|------|------|
| `session/loop_worker.go` | **W** | | | | | |
| `session/loop.go` | **W** | | | | | |
| `session/session.go` | **W** | | | | | |
| `session/manager.go` | | | **W** | | | |
| `session/autooptimize.go` | | | **W** | | | |
| `session/snapshot.go` | | | | | **W** | |
| `session/reflexion.go` | | | | | **W** | |
| `e2e/gates.go` | | **W** | | | | |
| `mcpserver/handler_session.go` | **W** | | | | | |
| `mcpserver/tools_roadmap.go` | | **W** | | | | |
| `mcpserver/handler_rdcycle.go` | | | | | | **W** |
| `enhancer/scoring.go` | | | | **W** | | |
| `enhancer/pipeline.go` | | | | **W** | | |
| `enhancer/analyze.go` | | | | **W** | | |
| `fleet/coordinator.go` | | | | | **W** | |

**No file is written by more than one parallel workstream.** WS-F runs after all parallel work merges.

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| QW-1 JSON enforcement breaks valid non-JSON responses | Medium | High | Guard with `looksLikeJSON()` pre-check (already in place from sprint 1); only enforce on planner output, not all responses |
| QW-3 turn cap too aggressive for complex tasks | Low | Medium | Make cap configurable via `MAX_WORKER_TURNS` env var; default 20 is generous |
| QW-11 reaper kills in-progress tasks | Medium | High | Only reap tasks with no heartbeat update in >1h AND repo path missing; never reap tasks with active PID |
| WS-F rdcycle `cycle_baseline` runs `go test` in arbitrary repos | Low | High | Validate repo path exists and contains go.mod before running; timeout at 60s |
| WS-F rdcycle `cycle_merge` corrupts working tree | Medium | High | Always operate on worktree copies, never main; validate git status clean before merge |
| Score recalibration (QW-4) breaks existing score-based gates | Low | Medium | Run `loop_gates` tests after scoring changes; add calibration corpus test |

## Integration Sequence

After all parallel workstreams merge:
1. `go build ./...` — verify compilation
2. `go vet ./...` — static analysis
3. `go test ./internal/session/... -count=1` — session package (QW-1,2,3,7,8,9,12)
4. `go test ./internal/e2e/... -count=1` — gates package (QW-6)
5. `go test ./internal/enhancer/... -count=1` — enhancer package (QW-4,5)
6. `go test ./internal/fleet/... -count=1` — fleet package (QW-11)
7. `go test ./internal/mcpserver/... -count=1` — MCP tools (QW-8,10 + rdcycle)
8. `go test ./... -count=1 -timeout 120s` — full suite

## Acceptance Criteria (per QW item)

| QW | Metric | Target | How to Verify |
|----|--------|--------|---------------|
| QW-1 | JSON retry rate | <5% | Run 10 loop iterations, count retries |
| QW-2 | Cascade routing default | Enabled | `NewManager()` → check `CascadeRouter != nil` |
| QW-3 | Max worker turns | 20 | Session with >20 turns returns error |
| QW-4 | Score range | 3-9 spread | Score 5 known-good + 5 known-bad prompts |
| QW-5 | Skipped stages visible | Yes | `prompt_enhance` result includes `skipped_stages` |
| QW-6 | First-run baseline | Persisted | Delete baselines, run gates, check file created |
| QW-7 | Snapshot path | `.ralph/snapshots/` | `snapshot` tool creates file in correct dir |
| QW-8 | Budget enforcement | Active | `session_launch budget_usd=0.50` respects limit |
| QW-9 | Autonomy persistence | Survives restart | Set level, restart, verify level unchanged |
| QW-10 | Relevance scoring | stddev > 0.15 | `roadmap_analyze` on 10 items, compute variance |
| QW-11 | Phantom tasks | <5% | `fleet_status` shows no stale tasks |
| QW-12 | Rules populated | ≥3 rules | `improvement_patterns.json` `rules` is non-null |

## Constraints

- Each parallel workstream operates on non-overlapping files (see resource matrix)
- Do NOT modify `ROADMAP.md`
- Do NOT add features beyond what's specified
- Every workstream must end with `go build ./...` and `go vet ./...` passing
- If `$ARGUMENTS` contains a workstream filter (e.g., "ws-a" or "ws-a,ws-d"), only run those workstreams
- Use `/bin/cp` (not `cp`) when copying files between worktrees to avoid macOS alias interference

## Post-Sprint

After all workstreams complete:
1. Run `go test ./... -count=1 -timeout 120s`
2. Update `.ralph/tool_improvement_scratchpad.md` — mark resolved findings
3. Report: QW resolution status, rdcycle implementation status, coverage delta, remaining work for sprint 3
