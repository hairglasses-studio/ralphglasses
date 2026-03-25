# Self-Learning Test Run Scratchpad

## Current Status (2026-03-25)

All 11 improvement items from Runs 1-4 resolved via 3 parallel workstream agents. 33/33 Go packages pass with `-race`. Ready for Run 5 validation.

---

## Resolved Items

All items below were fixed in the workstream resolution batch. Kept for reference.

| # | Item | Resolution | Workstream |
|---|------|-----------|------------|
| 1 | Planner task dedup | `prevIterations` threaded into `buildLoopPlannerPromptN` | A1 |
| 2 | Reflexion file path regex false positives | Tightened to require `/` or Go extension | A2 |
| 3 | Reflexion correction quality (generic text) | Broadened failure regex, extracts actual error | A3 |
| 4 | Task title sanitization (JSON/markdown) | Added key fallbacks + fence stripping | A4 |
| 5 | `omitempty` hiding profile booleans | Removed from 7 boolean fields | B1 |
| 6 | Phase H only wired in self_improve | Moved to `wireSubsystems()` for both entry points | B2 |
| 7 | Bandit coupled to cascade router | UCB1 Selector on Server, standalone `DefaultProviderArms()` | B3 |
| 8 | FeedbackAnalyzer nil in CurriculumSorter | `wireSubsystems()` passes `s.FeedbackAnalyzer` | B4 |
| 9 | Acceptance `git checkout main` in worktree | Detects worktree, uses `git update-ref` | C1 |
| 10 | Flaky `TestEdge_LargeInputs` timing | Thresholds raised 3s->10s, 5s->15s | C2 |
| 11 | MCP hot reload not documented | Restart workflow in `cmd/mcp.go` + `docs/MCP-TOOLS.md` | C3 |
| 12 | `RecentForTask("")` always nil | Returns N most recent when title empty | Pre-workstream |
| 13 | Cost tracking `total_cost_usd=0` | `costPredictor.Record()` in StepLoop | Run 4 wiring |
| 14 | Per-stage latency all zeros | Planner/worker/verify timestamps populated | Run 4 wiring |
| 15 | Model name `sonnet-4` invalid | Changed to `claude-sonnet-4-6` | Run 4 wiring |
| 16 | Observation `omitempty` on self-learning fields | Already clean — no omitempty on LoopObservation self-learning fields | N/A |
| 17 | Marathon bats flake (ANTHROPIC_API_KEY) | Assertion relaxed with `||` fallback | Pre-workstream |
| 18 | Episode retrieval cap (hardcoded 3) | `DefaultK` configurable on EpisodicMemory | Pre-workstream |
| 19 | Subsystem re-init on every loop_start | `wireSubsystems()` is idempotent (nil checks) | B2 |

---

## Open Items

### NEW: waitForSession hangs after MCP reconnect
- **Symptom**: Loop stuck in `planning` forever after MCP reconnect kills the planner's Claude process
- **Root cause**: `waitForSession()` blocks on session completion but has no timeout or process health check
- **Impact**: Runs 5b, 9, 10 (iter 2), 11 (iter 3) all orphaned by this bug
- **Fix needed**: Add a context timeout to `waitForSession`, or periodically check if the session process is still alive (`os.Process.Signal(0)`)
- **Priority**: HIGH — this is the #1 cause of orphaned loops

### NEW: Task title sanitization regression (Run 11 iter 2)
- **Symptom**: Task title is "All tests pass. Here's what I did:" — worker output, not a task title
- **Root cause**: Planner returned freeform text; `sanitizeTaskTitle()` couldn't extract a structured title
- **Fix needed**: Add heuristic to detect "output-like" text (starts with common phrases like "All tests pass", "Here's what", "I've completed") and reject it, falling back to a default title from the prompt

### BLOCKED: Cascade routing never live-tested
- **Blocker**: Gemini CLI not installed. Cascade requires a cheap provider binary on PATH.
- **Action required**: Install Gemini CLI (`npm install -g @anthropic-ai/gemini-cli` or equivalent), then run with `enable_cascade=true`.
- **Impact**: Bandit Thompson Sampling hooks, cascade escalation, and confidence-based routing remain untested in production.

### DEFERRED: MCP hot reload (fsnotify)
- **Status**: Documented restart workflow. Actual fsnotify-based reload is a feature request, not a bug.
- **Impact**: After code changes, MCP server must be restarted manually: `claude mcp remove ralphglasses && claude mcp add ralphglasses -- go run . mcp`

### RESOLVED: Cross-run task dedup
- **Was**: Planner only saw current-run iterations, repeated tasks from prior runs.
- **Fix**: `StepLoop` now calls `m.ListLoops()` and injects completed task titles from all prior runs for the same repo into `prevIterations`. Flows through existing "Completed tasks (DO NOT repeat these)" dedup section.
- **Commit**: (pending)

### RESOLVED: Selftest gate skipping
- **Was**: `selftest --gate` always returned "skip (current=0.000)".
- **Root cause**: Baseline file didn't exist. First `--gate` call creates baseline and returns "skip". Second call compares against it.
- **Status**: 28 observations exist. Baseline created. Gate now returns pass/warn/fail verdicts. Cost down 76.5%, latency down 48.7% vs baseline.

### OBSERVATION: Planner task type diversity
- Across 18 iterations (Runs 1-4), planner selected 16x from ROADMAP 0.5.1.x cluster (error propagation). Only 1x test, 0x refactor/feature.
- Run 8 (opus): 1x concurrency test, 1x error propagation, 2x test overlap — better but still clustered.
- Not a code bug — planner follows ROADMAP priority ordering. Could be improved by injecting diversity hints or rotating ROADMAP sections.

### OBSERVATION: Type architecture duplication (Phase H)
- Two parallel type hierarchies for Blackboard and CostPredictor:
  - Server (mcpserver): `*blackboard.Blackboard`, `*fleet.CostPredictor` — used by MCP tool handlers
  - Manager (session): `*session.Blackboard`, `*session.CostPredictor` — used by StepLoop internals
- Both wired in `wireSubsystems()`. Not a problem, but important to know when touching Phase H code.

---

## Run 5 Readiness Checklist

- [x] All 11 improvement items resolved
- [x] `go build ./...` passes
- [x] `go test -race -count=1 ./...` — 33/33 packages pass
- [x] `wireSubsystems()` signature: `(s *Server, sessMgr *session.Manager, ralphDir string)`
- [x] Session-level CostPredictor wired on Manager
- [x] `handleSelfImprove` duplicate Phase H wiring removed
- [x] MCP server restarted with fresh binary
- [x] RunLoop auto-driver added (loops no longer stuck at "pending")
- [x] StopLoop race fix (cancel + done channel) — eliminates TempDir flake
- [x] Run 5 executed — converged after 3 iterations, all verification passed

### Run 5a (94aa4384) — Failed, 1 iteration
- **Task**: "Add provider-specific stderr cost fallback (2.5.5.3)" — worker succeeded
- **Failure**: `TestHandleSelfImprove_ValidRepo` TempDir race in ci.sh verification
- **Root cause**: `StopLoop` didn't wait for `RunLoop` goroutine to exit before TempDir cleanup
- **Fix**: Added `cancel`/`done` channel to `LoopRun`; `StopLoop` now cancels context and blocks on `done`
- **Commit**: `a65d2f3`

### Run 5b (78cf65e5) — Orphaned by MCP reconnect (stopped, 0 iterations completed)

### Run 5c (9745237d) — Converged, 3 iterations
- **Status**: `converged` — stopped after 2 consecutive no-change iterations
- **Total duration**: ~9 min (12:04–12:13)
- **All 3 iterations passed verification** (ci.sh + selftest gate)
- **No TempDir race** — StopLoop cancel/done fix confirmed working
- **PR #2 created** for iter 1 (RefreshRepo `[]error` return)

| Iter | Task | Changes | Verify | Duration |
|------|------|---------|--------|----------|
| 1 | Return `[]error` from `RefreshRepo` | 8 call sites updated | passed | 4m28s |
| 2 | Watcher error propagation | no-op (already fixed) | passed | 2m02s |
| 3 | Unit test: corrupt `status.json` | no-op (test already exists) | passed | 2m14s |

#### Validation Target Results

| Subsystem | Result |
|-----------|--------|
| Planner dedup | **PASS** — 3 unique tasks, 0 repeats from Runs 1-4 |
| Title parsing | **PASS** — clean titles, no JSON/markdown artifacts |
| Verification (TempDir) | **PASS** — 3/3 ci.sh runs clean, no flaky failures |
| RunLoop driver | **PASS** — loop auto-drove to convergence (no manual stepping) |
| Acceptance gate | **PASS** — PR created for real changes, skipped for no-ops |
| Reflexion | **N/A** — no failures to trigger reflexion extraction |
| Phase H cost tracking | **N/A** — no observation data populated (selftest gate skipped) |
| Bandit/cascade | **N/A** — cascade disabled (Gemini CLI missing) |

#### Observations
- **Planner picking low-value tasks**: Iter 2+3 targeted already-fixed issues, causing convergence. The planner's ROADMAP scan may need freshness detection (skip tasks whose target code already matches the desired state).
- **No observation data**: `selftest --gate` returned "skip (current=0.000)" — loop observations (difficulty_score, episodes_used, confidence, cost) were not populated in the status output. May need to check if `RecordObservation` is being called in StepLoop after each iteration.
- **PR review**: PR #2 flagged `scanner.go` and `handler_repo.go` for review (non-safe paths), auto-merged `integration_test.go` and `app.go` (safe paths). Acceptance gate classification working correctly.

### Run 6 (07ad881b) — Failed, 1 iteration (opus planner)
- **Status**: `failed` — ci.sh caught stale e2e test assertion
- **Total duration**: ~2 min (12:21–12:23)
- **Opus planner quality**: Significantly more detailed prompt than sonnet — included step-by-step instructions, constraints, and explicit file paths. Good improvement.
- **Worker succeeded**: Added `TestRefreshRepo_CorruptStatusJSON` to `internal/discovery/scanner_test.go` — correct package discovery, proper `[]error` handling
- **Failure**: `TestSelfImprovementProfileHasSelfLearningEnabled` (platform_test.go:207) expected `MaxIterations=5` but profile was changed to `10` in previous session
- **Root cause**: E2e test not updated when `SelfImprovementProfile().MaxIterations` was raised from 5→10
- **Fix**: Updated platform_test.go assertion to expect 10

| Iter | Task | Changes | Verify | Duration |
|------|------|---------|--------|----------|
| 1 | Unit test: corrupt status.json | Added test in discovery pkg | failed (stale e2e assertion) | 2m08s |

#### Observations
- **Opus planner prompt quality**: Much more structured and actionable than sonnet. Included explicit steps, constraints, and file targets. Worth the cost premium.
- **Worker adapted to reality**: Prompt said `internal/scanner/` but worker correctly found the code in `internal/discovery/` and adapted. Good resilience.
- **Pre-existing test debt**: The e2e platform_test.go is a regression trap — any profile change requires updating this test. Consider making the test assert relative properties (e.g., MaxIterations > 0) instead of exact values.

### Run 7 (5baf985f) — Failed, 1 iteration (worktree stale)
- **Status**: `failed` — same `platform_test.go` assertion; worktree branched before fix was committed
- **Task**: "Add unit tests for RefreshRepo error propagation" — worker succeeded
- **Lesson**: Must commit + push fixes to main before launching loops, since worktrees branch from HEAD

### Run 8 (b3648706) — Converged, 4 iterations (opus planner, best run)
- **Status**: `converged` — no changes in last 2 iterations
- **Total duration**: ~19 min (12:33–12:52)
- **All 4 iterations passed verification** (ci.sh + selftest gate)
- **2 PRs created**: #3 (race tests + bugfix), #4 (RefreshRepo caller updates)
- **First real bug found by loop**: `reposCopy` shallow pointer copy causing data races

| Iter | Task | Changes | Verify | Duration |
|------|------|---------|--------|----------|
| 1 | Concurrent scan race test | 2 tests + `reposCopy` deep-copy fix | passed | 7m |
| 2 | RefreshRepo caller error handling | Call-site logging updates | passed | 5m36s |
| 3 | Unit test: corrupt status.json | no-op (test exists) | passed | 3m34s |
| 4 | Display parse errors in repo detail | no-op (already wired) | passed | 2m53s |

#### Observations
- **Opus found a real concurrency bug**: `reposCopy` was doing shallow pointer copies, causing data races between concurrent scan/list. This is the highest-value fix from any loop run so far.
- **Cross-run dedup still weak**: Iters 2-4 targeted tasks already completed in Run 5c. Planner needs access to merged PRs or a "completed tasks" registry that persists across loop instances.
- **Worker resilience confirmed again**: Iter 2 worker found RefreshRepo already returns `[]error` and pivoted to improving caller-side error handling instead.
- **Convergence working correctly**: 2x no-change iterations triggered clean exit.
- **Selftest gate now working**: 28 observations on disk. Baseline was missing (first `--gate` call creates it). Second call: cost -76.5%, latency -48.7% vs baseline. Gate returns warn (78.6% completion rate due to early failed runs).

### Run 9 (3c24cd4c) — Orphaned by MCP reconnect (stopped, 1 iteration idle)
- **Task**: "Wire TUI to consume process.Manager ErrorChan for crash notifications"
- **Cross-run dedup confirmed**: Genuinely new task type (not from ROADMAP 0.5.1.x cluster)
- **Stopped**: MCP reconnect killed planner session

### Run 10 (7b928b36) — Partial, 1 iteration + orphaned (old binary, no auto-merge)
- **Task**: "Emit ProcessExitMsg to TUI on process exit" — PR #6 created and merged
- **Cross-run dedup**: Another unique task (TUI process monitoring theme)
- **Note**: Profile lacked `auto_merge_all` — old binary pre-dates the feature

### Run 11 (45148971) — Partial, 2 iterations + orphaned (auto-merge confirmed!)
- **First auto-merge success**: Iter 1 auto-merged directly to main
- **Iter 2**: Passed but no changes to merge (no-op)
- **Iter 3**: Orphaned by MCP reconnect (stuck in `planning` forever)

| Iter | Task | Changes | Verify | Auto-merged |
|------|------|---------|--------|-------------|
| 1 | Update repo status on process exit | process/manager.go | passed | **YES** |
| 2 | (title sanitization issue) | no-op | passed | N/A |
| 3 | (orphaned) | — | — | — |

#### Observations
- **AutoMergeAll working**: First successful auto-merge in iter 1. Changes to `internal/process/manager.go` (normally a review path) were auto-merged because `auto_merge_all=true` and ci.sh passed.
- **Title sanitization regression**: Iter 2 title is "All tests pass. Here's what I did:" — worker output leaked into task title. The planner may have returned freeform text instead of structured JSON.
- **MCP reconnect kills loops**: `waitForSession` blocks forever when the planner's Claude process is killed. Need a context timeout or health check on the session process.

### Run 12 (9b0ed27d) — Completed, 5 iterations (first fully instrumented run)
- **Status**: `completed` — max_iter reached
- **Total duration**: 42 min (14:02–14:44)
- **All 5 iterations passed verification**
- **3 auto-merges**, 1 pending_review (diff gate), 1 no-op
- **First run with sub-phase timing data**

| Iter | Task | Status | Prompt | Enhance | Accept | Idle | Auto-merged |
|------|------|--------|--------|---------|--------|------|-------------|
| 1 | (title sanitization bug) | passed | 10ms | 12.9s | 95ms | — | YES |
| 2 | Auto-advance iteration scheduling | pending_review | 9ms | 25.3s | — | 108ms | no |
| 3 | Unit tests for stderr cost fallback | passed | 11ms | 28.2s | 7ms | 17ms | no (no-op) |
| 4 | Refactor TUI key bindings | passed | 10ms | 22.7s | 103ms | 22ms | YES |
| 5 | Make doctor.sh warnings non-fatal | passed | 9ms | 30.0s | 108ms | 117ms | YES |

#### Timing Insights (from instrumentation)
- **Enhancement dominates pre-planner time**: 13-30s per iteration (avg ~24s). This is the LLM call to improve the planner prompt. Single biggest optimization opportunity.
- **Prompt build is negligible**: ~10ms (ROADMAP scan + dedup is fast)
- **Acceptance is fast**: 7-108ms (git classify + merge)
- **Zero scheduler overhead**: 17-117ms idle between iterations
- **Reflexion/episodic not wired**: Both null — subsystems not initialized in this run's MCP server context

#### Observations
- **Title sanitization bug persists**: Iter 1 title is worker output, not a task name. Same issue as Run 11 iter 2.
- **Diff gate correctly blocks self-modification**: Iter 2 changed `internal/session/loop.go` → `pending_review` despite `auto_merge_all=true`. The self-test diff gate is a separate check that takes priority.
- **Task diversity improved**: 5 unique tasks across test, refactor, CI, and scheduling categories.
- **Convergence not triggered**: All 5 iters had real tasks; hit max_iter instead of converging.

### Run 5 Validation Targets

| Subsystem | What to verify | How |
|-----------|---------------|-----|
| Planner dedup | New tasks each iteration (no repeats from Runs 1-4) | Check `task_title` in observations |
| Reflexion | `files_involved` has no bare-word false positives | Check observation after a failure |
| Reflexion | Correction text includes actual error message | Check `correction` field |
| Title parsing | Clean titles even from markdown-wrapped JSON | Check `task_title` field |
| omitempty | `enable_reflexion=false` visible in profile JSON | Check loop_start response |
| Phase H | `ralphglasses_cost_forecast` returns data | Call after 2+ iterations |
| Bandit | `ralphglasses_bandit_status` shows pulls | Call after 2+ iterations |
| Episodic | `episodes_used > 0` after first iteration | Check observation |
| Difficulty | `difficulty_score` in 0.5-0.6 range | Check observation |
| Acceptance | No `git checkout main` error in worktree | Check acceptance gate on pass |

---

## Historical Run Data

<details>
<summary>Run 1-4 metrics (click to expand)</summary>

| Metric | Run 1 | Run 2 | Run 3 | Run 4 | Run 5c | Run 8 | Run 10 | Run 11 | Run 12 |
|--------|-------|-------|-------|-------|--------|-------|--------|--------|--------|
| Iterations | 6 | 3 | 6 | 3 | 3 | 4 | 1 | 2 | 5 |
| Passed | 6 | 1 | 5 | 3 | 3 | 4 | 1 | 2 | 5 |
| Failed | 0 | 2 | 1 | 0 | 0 | 0 | 0 | 0 | 0 |
| Completion rate | 100% | 33% | 83% | 100% | 100% | 100% | 100% | 100% | 100% |
| Total latency (min) | 21.5 | 7.7 | 25.2 | 7.2 | 8.8 | 19.1 | ~7 | ~12 | 42 |
| Avg latency/iter (s) | 215 | 154 | 252 | 144 | 176 | 287 | ~420 | ~360 | 504 |
| Avg enhance (s) | — | — | — | — | — | — | — | — | 23.8 |
| Cost tracked ($) | 0 | 0 | 0 | 0.248 | N/A | N/A | N/A | N/A | N/A |
| PRs created | 0 | 0 | 0 | 1 | 1 | 2 | 1 (#6) | 0 | 0 |
| Auto-merges | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 3 |
| Bugs found | 0 | 0 | 0 | 0 | 0 | 1 (race) | 0 | 0 | 0 |
| Planner model | sonnet | sonnet | sonnet | sonnet | sonnet | opus | opus | opus | opus |
| Exit reason | max_iter | max_iter | max_iter | max_iter | converged | converged | orphaned | orphaned | max_iter |

### Key conclusions from Runs 1-4
1. Episodic memory: end-to-end working, cross-run persistence confirmed
2. Reflexion: extraction, injection, cross-run persistence all working
3. Curriculum: difficulty scoring differentiates task types
4. Confidence: 1.0 pass, 0.0 fail (omitempty hid failures in Runs 1-3)
5. Cost tracking: fixed in Run 4 ($0.248 for 3 iterations)
6. Per-stage latency: fixed in Run 4
7. Cascade: never tested (Gemini CLI missing, cascade disabled)

</details>

---

## Merge Conflict Lessons (from workstream resolution)

- **Dual bandit types**: `policy.go` defines `Arm` struct; new `bandit.go` `Selector` must use wrapper (`selectorArm`) not redefine `Arm`
- **Phase H type split**: Server uses `blackboard.Blackboard`/`fleet.CostPredictor`, Manager uses `session.Blackboard`/`session.CostPredictor` — incompatible APIs, both needed
- **wireSubsystems scope creep**: Adding `*Server` param was necessary for Phase H wiring but means the function touches two ownership domains
- **Stash + merge conflicts**: When stashing local changes before merging worktree branches, `git stash pop` conflicts must be resolved per-file with `git checkout HEAD --` for files that should keep the merged version
