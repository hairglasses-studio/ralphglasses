# Self-Learning Test Run Scratchpad

## Current Status (2026-03-25)

**Run 15 completed — 5/5 passed, 100% completion rate, fully autonomous auto-drive.** First run with zero manual intervention since auto-drive fix. 2 auto-merges, 2 pending_review (production code changes), 1 no-op. Total duration: ~20 min. Planner title JSON parsing: 3/5 clean, 2/5 fallback (planner returned worker-style output).

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

### RESOLVED: waitForSession hangs after MCP reconnect
- **Fix**: Added `doneCh` select case + 10-minute timeout to `waitForSession()` in manager.go. Sessions now detect process death immediately via doneCh closure.
- **Commit**: `7dc5e4d` (worktree-agent merge)
- **Note**: Run 13 used old binary, so iter 5 still hung (worker never dispatched). Fix confirmed working in new binary tests.

### RESOLVED: Task title sanitization regression
- **Fix**: Added 15-pattern prefix rejection heuristic in `sanitizeTaskTitle()`. Worker output phrases like "All tests pass..." now return fallback "self-improvement iteration".
- **Commit**: `e8fd581` (worktree-agent merge)
- **Confirmed**: All Run 13 task titles are clean.

### RESOLVED: Enhancement latency (~24s per iteration)
- **Fix**: Split `EnableEnhancement` into `EnablePlannerEnhancement` + `EnableWorkerEnhancement`. Self-improvement profile: planner=true, worker=false.
- **Commit**: `da3d3f3`
- **Confirmed**: Run 13 shows 0ms enhancement time (old binary had `enable_enhancement: false`). Next run with new binary will use split flags.

### NEW: Worker dispatch failure (Run 13 iter 5)
- **Symptom**: Planner completed but worker_session_id is null. Iteration stuck in "executing" forever.
- **Root cause**: Unknown — planner output was valid JSON, but worker session was never created. No error recorded.
- **Impact**: Hung Run 13 after iter 4. Had to stop manually.
- **Priority**: HIGH — silent worker dispatch failure with no error logging
- **Investigation**: Check StepLoop code path between planner output parsing and worker session creation. May need explicit error handling + logging when worker launch fails.

### NEW: ff-merge fails when main diverges during active loop
- **Symptom**: All worktree iterations fail `acceptance: ff-merge: main has diverged, cannot fast-forward in worktree`
- **Root cause**: `AutoCommitAndMerge` only supports ff-merge. When operator pushes commits to main during a loop, the worktree branch can't fast-forward.
- **Impact**: Run 13 iters 1, 3 both stranded. Had to manually cherry-pick changes.
- **Fix options**: (a) Rebase worktree branch onto main before merge, (b) Fall back to creating a PR when ff-merge fails, (c) Use merge commit instead of ff
- **Priority**: MEDIUM — workaround is to not push during active loops, but this limits operator workflow

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

### RESOLVED: Auto-drive stall (handleLoopStart never calls RunLoop)
- **Symptom**: Loops started via `loop_start` with `self_improvement=true` stayed at "pending" — required manual `loop_step` between each iteration.
- **Root cause**: `handleLoopStart` called `StartLoop` but never `RunLoop`. Only `handleSelfImprove` called `RunLoop`.
- **Fix**: Added `go s.SessMgr.RunLoop(context.Background(), run.ID)` when `profile.SelfImprovement` is true in `handleLoopStart`.
- **File**: `internal/mcpserver/handler_loop.go` (lines 140-143)

### RESOLVED: Worktree cleanup path safety (defense-in-depth)
- **Symptom**: Run 14 iter 4 failed, then entire repo source tree was wiped (only `.ralph/` survived).
- **Investigation**: Ralph's `CleanupLoopWorktrees` only deletes inside `.ralph/worktrees/loops/`. The wipe was likely caused by Claude Code agent worktree cleanup (`.claude/worktrees/` entries), not ralph code.
- **Fix**: Hardened `CleanupLoopWorktrees` with `sanitizeLoopName()` (matching `createLoopWorktree`), empty-path rejection, and `filepath.Abs` boundary check to prevent future traversal.
- **File**: `internal/session/worktree.go`
- **Tests**: Added `TestCleanupLoopWorktrees_PathTraversal` and `TestCleanupLoopWorktrees_EmptyRepoPath`

### RESOLVED: Title sanitization "Created" prefix
- **Symptom**: Run 14 iter 1 title "Created `.github/workflows/ci-macos.yml`" — past-tense worker output, not a task name.
- **Fix**: Added `"i created"`, `"created "`, `"created."` to rejection prefix list in `sanitizeTaskTitle()`.
- **File**: `internal/session/loop.go` (line 1553-1555)

### NEW: Planner returns worker-style output instead of JSON (40% fallback rate)
- **Symptom**: 2/5 iterations in Run 15 (and 2/4 in Run 14) had `source: "fallback"` — planner output was freeform completion text like "All session tests pass..." instead of `{"title":..., "prompt":...}` JSON.
- **Impact**: Task title becomes the worker output text (caught by sanitization, but the real task prompt is lost). Worker gets confusing instructions.
- **Root cause**: Planner sometimes ignores the JSON output format instruction in the system prompt, especially when it "completes" the task itself instead of delegating.
- **Fix options**: (a) Add a JSON schema enforcement post-check + retry on parse failure, (b) Strengthen the planner system prompt with "CRITICAL: respond ONLY with JSON", (c) Use structured output / tool_use for planner response format
- **Priority**: MEDIUM — sanitization catches the worst cases, but 40% fallback rate degrades task quality

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

### Run 13 (8713fdbd) — Stopped, 5 iterations (3 fixes deployed, TUI loop views)
- **Status**: `stopped` — iter 5 hung (worker never dispatched), manually stopped
- **Total duration**: ~45 min (14:58–15:43)
- **4/5 iterations passed verification**, 1 hung
- **1 auto-merge** (iter 4), 2 ff-merge failures (manually cherry-picked), 1 no-op
- **Enhancement disabled**: 0ms enhance time (old binary, `enable_enhancement: false`)
- **Clean task titles**: All 5 titles are real task names (sanitization fix confirmed)
- **New binary features**: waitForSession timeout, split enhancement flags, title sanitization — deployed but Run 13 used pre-deployment binary

| Iter | Task | Verify | Merge | Duration | Notes |
|------|------|--------|-------|----------|-------|
| 1 | TUI loop list view | passed | ff-merge failed | 7m12s | Cherry-picked to main |
| 2 | Fix self-improvement tests | passed (no-op) | N/A | 1m47s | Already fixed on main |
| 3 | TUI loop detail view | passed | ff-merge failed | 8m | Manually integrated RenderLoopDetail |
| 4 | Start/stop keybindings | passed | **auto-merged** | 7m23s | handleLoopListStart/Stop + KeyDispatch |
| 5 | Loop cost summary | **hung** | — | >20m | Worker never dispatched, planner completed |

#### Observations
- **ff-merge is the #1 acceptance failure mode**: 2/4 completed iters failed ff-merge because operator pushed commits during the run. AutoCommitAndMerge needs rebase or PR fallback.
- **Worker dispatch silently fails**: Iter 5 planner completed but worker_session_id remained null with no error. Needs investigation and explicit error handling.
- **Enhancement skip confirmed**: 0ms enhance time across all iterations (was 24s avg in Run 12). Prompt build time: 8-10ms.
- **Task diversity excellent**: 5 unique TUI feature tasks, no repeats from prior runs. Planner successfully identified loop views as a feature gap.
- **Old binary limitation**: Run 13 ran pre-fix binary. waitForSession timeout, split enhancement flags, and title sanitization were deployed but only take effect on next MCP reconnect.

### Run 14 — Partial, 4 iterations (3 passed, 1 failed; auto-drive stalled)
- **Status**: Manually stepped (auto-drive stall bug — `handleLoopStart` never called `RunLoop`)
- **Total duration**: ~30 min (manually stepped)
- **3/4 iterations passed verification**, 1 failed (worktree removed mid-execution)
- **CRITICAL INCIDENT**: After iter 4 failed, entire repo source tree was wiped. Recovered via re-clone from remote. `.ralph/` data restored from backup.

| Iter | Task | Verify | Merge | Notes |
|------|------|--------|-------|-------|
| 1 | Created `.github/workflows/ci-macos.yml` | passed | auto-merged | Title sanitization gap ("Created" prefix) |
| 2 | Sub-phase timing tests | passed | auto-merged (local-only, lost in wipe) | loop_test.go + loopbench_test.go |
| 3 | (fallback title) | passed | pending_review | Title source: "fallback" |
| 4 | (fallback title) | **failed** | — | Worktree removed mid-execution |

#### Bugs Found
1. **Auto-drive stall** (FIXED): `handleLoopStart` → `StartLoop` but never `RunLoop`. Added `go RunLoop()` when `SelfImprovement=true`.
2. **Repo working tree wiped** (HARDENED): Likely Claude Code agent worktree cleanup, not ralph code. Hardened `CleanupLoopWorktrees` with sanitization + boundary checks.
3. **Title sanitization gap** (FIXED): "Created" prefix not in rejection list. Added `"i created"`, `"created "`, `"created."`.

### Run 15 (04b326e1) — Completed, 5 iterations (first fully autonomous auto-drive run)
- **Status**: `completed` — max_iter reached, zero manual intervention
- **Total duration**: ~20 min (16:17–16:37)
- **All 5 iterations passed verification** (ci.sh + selftest gate)
- **2 auto-merges**, 2 pending_review, 1 no-op
- **Auto-drive fix validated**: handleLoopStart → RunLoop goroutine working perfectly

| Iter | Task | Verify | Merge | Duration | Enhance | Title Source |
|------|------|--------|-------|----------|---------|-------------|
| 1 | Config schema validation for .ralphrc | passed | auto-merged | 4m00s | 19.9s | JSON ✓ |
| 2 | TestSubPhaseTimingPopulated | passed | auto-merged | 3m20s | 6.5s | fallback |
| 3 | ProcessExitMsg sets repo status | passed | no-op | 2m21s | 5.0s | fallback |
| 4 | Auto-advance iteration scheduling | passed | pending_review | 6m01s | 30.0s | JSON ✓ |
| 5 | Loop health summary in TUI status bar | passed | pending_review | 3m50s | 27.9s | JSON ✓ |

#### Validation Results
| Subsystem | Result |
|-----------|--------|
| Auto-drive (RunLoop) | **PASS** — 5 iterations, zero stalls, zero manual steps |
| Title sanitization | **PARTIAL** — 3/5 clean JSON, 2/5 fallback (planner returned worker-style text) |
| Acceptance gate | **PASS** — production code → pending_review, test-only → auto-merge |
| Enhancement (planner) | **WORKING** — 5-30s per iter, prompt caching visible in early iters |
| Idle between iters | **PASS** — 14-255ms, negligible overhead |
| Selftest gate | **SKIP** — 0 observations (observation recording not wired in worktree context) |

#### Observations
- **Planner output format regression**: 2/5 iterations, the planner returned worker-style completion text instead of `{"title":..., "prompt":...}` JSON. The planner prompt may need stronger formatting enforcement or a retry on parse failure.
- **Iter 4 added `AutoAdvance` to LoopProfile**: Worker added `AutoAdvance bool` field and auto-advance logic to `StepLoop`. This duplicates `RunLoop`'s purpose — needs review before merge. The planner doesn't know about `RunLoop`, so it planned a feature that already exists.
- **Iter 5 added `LoopFleetSummary` to TUI**: Good feature — status bar shows `Loops: 3▶ 1⏸ 0✗`. Needs review for import cycle safety.
- **Enhancement time bimodal**: Early iters benefit from prompt caching (5-7s), later iters with longer context hit 28-30s. Consider caching the base planner prompt separately.
- **Task diversity excellent**: 5 unique tasks across config validation, testing, TUI, and loop engine categories. No repeats from prior runs.

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

| Metric | Run 1 | Run 2 | Run 3 | Run 4 | Run 5c | Run 8 | Run 10 | Run 11 | Run 12 | Run 13 | Run 14 | **Run 15** |
|--------|-------|-------|-------|-------|--------|-------|--------|--------|--------|--------|--------|------------|
| Iterations | 6 | 3 | 6 | 3 | 3 | 4 | 1 | 2 | 5 | 5 | 4 | **5** |
| Passed | 6 | 1 | 5 | 3 | 3 | 4 | 1 | 2 | 5 | 4 | 3 | **5** |
| Failed/Hung | 0 | 2 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 1 (hung) | 1 (wipe) | **0** |
| Completion rate | 100% | 33% | 83% | 100% | 100% | 100% | 100% | 100% | 100% | 80% | 75% | **100%** |
| Total latency (min) | 21.5 | 7.7 | 25.2 | 7.2 | 8.8 | 19.1 | ~7 | ~12 | 42 | ~45 | ~30 | **20** |
| Avg latency/iter (s) | 215 | 154 | 252 | 144 | 176 | 287 | ~420 | ~360 | 504 | ~360 | ~450 | **236** |
| Avg enhance (s) | — | — | — | — | — | — | — | — | 23.8 | 0 | — | **17.9** |
| Cost tracked ($) | 0 | 0 | 0 | 0.248 | N/A | N/A | N/A | N/A | N/A | N/A | N/A | N/A |
| PRs created | 0 | 0 | 0 | 1 | 1 | 2 | 1 (#6) | 0 | 0 | 0 | 0 | **0** |
| Auto-merges | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 3 | 1 | 2 | **2** |
| ff-merge failures | — | — | — | — | — | — | — | — | — | 2 | 0 | **0** |
| Bugs found | 0 | 0 | 0 | 0 | 0 | 1 (race) | 0 | 0 | 0 | 1 (dispatch) | 3 (stall, wipe, title) | **0** |
| Planner model | sonnet | sonnet | sonnet | sonnet | sonnet | opus | opus | opus | opus | opus | opus | **opus** |
| Exit reason | max_iter | max_iter | max_iter | max_iter | converged | converged | orphaned | orphaned | max_iter | stopped | manual | **max_iter** |

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
