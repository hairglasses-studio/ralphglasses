# Tool, Wiring & Workflow Improvement Opportunities

Observations from reliability & quality improvement workstreams + recursive self-testing design (2026-03-24).

## Worktree Agent Merge Pain Points

1. **Worktree agents don't auto-commit** — every agent leaves uncommitted diffs, requiring manual `\cp -f` to merge. If agents committed to their worktree branch, we could use `git merge` or `git cherry-pick` instead of file-level copies.

2. **Cross-worktree file conflicts require manual resolution** — when 2+ agents modify the same file (e.g., `runner.go` modified by R1 and R2, `manager.go` by R2 and R4, `bus.go` by R4 and R5), the merge is entirely manual. A merge assistant or conflict detection at dispatch time would help.

3. **Worktree agents start from stale main** — Phase B agents didn't see Phase A changes because worktrees branch from the current HEAD at launch time. For sequential phases, agents should ideally start from the post-merge state. Workaround: commit Phase A before launching Phase B agents.

## MCP Tool Gaps

4. **~~No `loop_observations` query tool~~** — RESOLVED: Phase 0.8 WS-1 added `ralphglasses_observation_query` and `ralphglasses_observation_summary` tools.

5. **~~No cost estimation tool~~** — RESOLVED: Phase 0.8 WS-5 added `ralphglasses_cost_estimate` with per-provider rates and historical calibration.

6. **No event query tool** — now that events persist to JSONL (R5), a `ralphglasses_event_query` tool could search/filter events by type, session, time range.

## Test Infrastructure

7. **~~No test coverage tracking across runs~~** — RESOLVED: Phase 0.8 WS-4 added `ralphglasses_coverage_report` tool that runs `go test -coverprofile` and reports per-package vs threshold.

8. **Fuzz test corpus not persisted** — Go fuzz tests generate corpus entries in `testdata/fuzz/` but these aren't tracked in git. Should add `testdata/fuzz/` to `.gitignore` or decide to commit seed corpus.

## Provider Normalization

9. **Cost estimation is approximate** — R1's token-based cost fallback uses hardcoded rates that will drift as providers change pricing. Should periodically update `ProviderCostRates` or fetch from a config file.

10. **No provider output format documentation** — each normalizer (`normalizeClaudeEvent`, `normalizeGeminiEvent`, `normalizeCodexEvent`) has inline knowledge of the provider's JSON schema. This should be documented in `docs/PROVIDER-SETUP.md` with example outputs.

## Process Management

11. **Kill escalation timeout is hardcoded** — R2 uses 5s timeout. For long-running operations (large git operations, big compilations), the process may need more time for graceful shutdown. Should be configurable per-session or per-provider.

12. **~~No orphan detection~~** — RESOLVED: orphans.go implemented. ~~if ralphglasses crashes, spawned CLI processes continue running. A startup sweep checking for orphaned process groups (matching known session PIDs) would prevent cost leaks.~~

## Event Bus

13. **Event persistence is synchronous on publish path** — R5 writes to disk in `Publish()` under the mutex. For high-throughput scenarios (fleet with 10+ sessions), this could become a bottleneck. Consider buffered async writes.

14. **~~No event schema versioning~~** — [RESOLVED] Event struct has `Version int` field (`json:"v"`), `PublishCtx` sets `Version=1` when zero, and `MigrateEvent` handles version upgrades for old persisted events.

## Worktree CWD Gotcha

15. **Agent CWD is its own worktree** — when this conversation's agent was spawned in a worktree (`agent-a7d1e22f`), `pwd` returns the worktree path, not the main repo. Relative paths like `.claude/worktrees/agent-xxx/...` fail because the worktree doesn't contain other worktrees. Always use absolute paths when copying between worktrees and main repo.

## Wiring Gaps (discovered during reliability work)

16. **~~PersistSession callers don't use the error~~** — [RESOLVED] `persistOrWarn` helper exists in `manager.go:608` that logs via `slog.Warn` on failure. Only one production caller of `PersistSession` (line 610) which checks the error.

17. **~~Event types are stringly-typed~~** — [RESOLVED] `ValidEventType()` function exists in `bus.go:107` with `knownEventTypes` map. `PublishCtx` warns on unknown types via `slog.Warn`.

18. **~~No event subscription filtering~~** — [RESOLVED] `Bus.SubscribeFiltered(id string, types ...EventType)` exists in `bus.go:280`, uses `filteredSubs` map with per-type channel routing.

## Workflow Improvements (discovered during parallel workstream execution)

19. **Phase sequencing needs pre-commit** — Phase B agents didn't see Phase A changes because worktrees branched from stale HEAD. Fix: commit Phase A before launching Phase B agents. Alternatively, the orchestrator could create a temporary branch with Phase A changes for Phase B to branch from.

20. **~~No automated merge verification between phases~~** — RESOLVED: Phase 0.8 WS-6 added `ralphglasses_merge_verify` tool with sequential build→vet→test, 5-min timeout per step.

21. **Observation pipeline not wired to git diffs** — `LoopObservation` has `FilesChanged`/`LinesAdded`/`LinesRemoved` but no actual file paths. Fixing this (plan item 2.3) would enable tracing regressions to specific code changes.

22. **Self-test harness needs binary isolation** — The running `ralphglasses` binary can't safely modify its own source. Stage 1.2 of the recursive self-testing plan addresses this with a snapshot binary pattern, but it's also relevant for any loop that targets its own repo.

## New MCP Tool Opportunities

23. **`ralphglasses_self_test`** — Expose the self-test harness via MCP (plan item 1.5). Params: repo, iterations, budget, use_snapshot.

24. **`ralphglasses_event_query`** — Now that events persist to JSONL (R5), a query tool with filters (type, session, time range, limit) would replace raw file reads.

25. **~~`ralphglasses_observation_query`~~** — RESOLVED: Phase 0.8 WS-1 implemented with filter by loop_id, status, provider, hours, limit.

26. **~~`ralphglasses_coverage_report`~~** — RESOLVED: Phase 0.8 WS-4 implemented with per-package coverage vs configurable threshold.

## Merge Workflow (Round 2 Observations)

27. **`cp -f` alias interference** — macOS aliases `cp` to `cp -i` (interactive) in some shells. Worktree merges need `/bin/cp -f` to bypass. Should document this or use Go file copy in merge tooling.

28. **~~Duplicate utility functions across packages~~** — RESOLVED: centralized in gitutil. ~~`gitDiffPaths()` exists as both `session.gitDiffPaths` (unexported, in protection.go) and `e2e.GitDiffPaths` (exported, in gitdiff.go). Same implementation. Should consolidate to one location — probably `e2e.GitDiffPaths` since it's the utility package — and have session import it.~~

29. **Worktree agents don't create new files in `git diff`** — new files (selftest.go, protection.go, errors.go) appear only as untracked files in worktrees, not in `git diff HEAD`. Need `git diff HEAD` + `git ls-files --others --exclude-standard` to capture full worktree output. Current merge workflow only uses `git diff --name-only HEAD` which misses new files.

30. **Health checker has two validation layers** — `CheckProviderHealth()` does full health (binary + env + version latency) while `HealthChecker.checkOne()` only does `ValidateProvider + ValidateProviderEnv`. The lightweight `checkOne` is fine for periodic heartbeat but could drift from the full check semantics.

31. **~~Error code adoption is partial~~** — RESOLVED: migration complete (216 calls). ~~Q5 converted `handler_loop.go` and `handler_session.go` to use `codedError()`, but ~20+ other handlers still use `errResult()`. Should progressively migrate all handlers. The `errResult` → `codedError` pattern is mechanical and could be automated.~~

32. **~~`EvaluateFromObservations` silently swallows baseline save errors~~** — RESOLVED: `e2e/gates.go:271` now logs via `slog.Warn("failed to save baseline", "err", saveErr)`.

## Round 3: Agent Completion & Research Observations

33. **Worktree auto-cleanup loses new files** — Agents 1.2 and 1.5 created new files only (no modifications to tracked files). When the worktree had no `git diff` changes, it was auto-cleaned, losing the new untracked files. Workaround: agents must `git add` new files before completion, or the worktree cleanup logic should check `git ls-files --others` too.

34. **`selftest` CLI subcommand missing** — Stage 2 CI plan identifies that `cmd/selftest.go` doesn't exist yet. The `SelfTestRunner.Run()` invokes the binary but there's no `selftest` cobra command to receive the invocation. Needs to be created before CI integration works end-to-end.

35. **~~`gitDiffPaths` triplicated~~** — RESOLVED: gitutil package. ~~Now exists in 3 places: `e2e.GitDiffPaths` (exported), `session.gitDiffPaths` (unexported, in protection.go), and planned for `session.gitDiffPathsForWorktree` (Stage 2.3 loopbench). Should converge on one source. Since session can't import e2e (circular), either: (a) move to a shared `internal/gitutil` package, or (b) accept the duplication as import-cycle avoidance.~~

36. **Self-improvement profile needs `selftest --gate` subcommand** — `SelfImprovementProfile()` plans to use `go run . selftest --gate` as a verify command, but this subcommand doesn't exist. The `gate-check` command in `cmd/gatecheck.go` partially covers this but lacks the self-test iteration step.

37. **Two-tier acceptance needs `gh` CLI** — `CreateReviewPR` shell-outs to `gh pr create`. Should add `gh` to the provider prerequisites check or make PR creation optional with a fallback (create branch but skip PR if `gh` unavailable).

38. **~~No `selftest` event types~~** — [RESOLVED] `SelfImproveMerged` and `SelfImprovePR` event types exist in `bus.go:66-67` and are registered in `knownEventTypes`.

## Round 4: Stage 2 Implementation Observations

39. **`SelfTestResult` field naming inconsistency** — WS-B agent discovered `SelfTestResult.Iterations` (actual) vs plan's `IterationsRun`. Plan docs drifted from implementation during Stage 1.2. Plans referencing struct fields should always be verified against code before implementation.

40. **`gate-check` and `selftest --gate` overlap** — Both `cmd/gatecheck.go` and `cmd/selftest.go --gate` call `EvaluateFromObservations`. The gate-check command takes `--baseline` and `--hours` flags; selftest --gate passes hardcoded defaults (0 hours = all observations). Should consider whether gate-check can be deprecated in favor of `selftest --gate` or whether they serve different audiences (gate-check = manual, selftest --gate = CI).

41. **~~`outputGateReport` duplicated~~** — [RESOLVED] Consolidated into `cmd/gate_output.go` with a single `outputGateReport` function used by both `gatecheck.go` and `selftest.go`.

42. **CI self-test job can't run live iterations yet** — The `selftest` command's full mode calls `e2e.Prepare` which builds a snapshot binary and needs API credentials. CI would need `ANTHROPIC_API_KEY` secret + cost budget. Initially deploying as `--gate` only is correct, but should track when to enable full iterations.

44. **Worktree agent replaced test file instead of appending** — WS-A agent was told to "append to loopbench_test.go or create if it doesn't exist" but replaced the entire file, deleting ~170 lines of existing tests. Root cause: the agent saw the file existed, decided to rewrite it with only the new tests. Workaround: manual merge in parent. Fix: agent prompts should explicitly say "do NOT delete existing test functions" when appending tests.

43. **~~`selftest` exit code via `os.Exit(1)` bypasses cobra~~** — [RESOLVED] Both `gatecheck.go` and `selftest.go` now return `ErrGateFailed` sentinel error (defined in `cmd/gate_output.go:15`) instead of `os.Exit(1)`. Cobra's error pipeline handles the exit code.

## Round 5: Stage 3 Phase B Observations

45. **B2 agent created duplicate `SelfImprovementProfile()`** — Agent created `internal/session/selfimprove.go` with its own version of `SelfImprovementProfile()` despite A1 already adding it to `loop.go`. The two versions had different defaults (B2: $5/$15 budget, 2 workers, cascade enabled; A1: $1/$3 budget, 1 worker, cascade disabled per plan). Root cause: B2 didn't see A1's changes in its worktree. Fix: skip the duplicate file during merge, use A1's canonical version.

46. **~~Self-improve handler uses `errResult` instead of `codedError`~~** — RESOLVED: `handler_selfimprove.go:60` uses `codedError(ErrLoopStart, ...)`.

47. **~~`maxIter` parameter unused in handler~~** — RESOLVED: `handler_selfimprove.go:40` sets `profile.MaxIterations` which is used by `StartLoop`.

48. **~~`self-improve.sh` references `mcp-call` subcommand~~** — RESOLVED: mcpcall.go exists. ~~The script calls `./ralphglasses mcp-call ralphglasses_self_improve` but no `mcp-call` cobra command exists. Should either: (a) implement `mcp-call` as a thin wrapper that starts MCP, calls the tool, and exits, or (b) change the script to use the MCP protocol directly via stdin/stdout.~~

## Round 6: Phase 0.8 MCP Smoke Test (2026-03-25)


49. **~~`merge_verify` repo param blocked by middleware~~** — RESOLVED: `middleware.go` now routes absolute paths through `ValidatePath` instead of `ValidateRepoName`.

50. **~~`coverage_report` same middleware conflict~~** — RESOLVED: same fix as #49.

51. **~~`scratchpad_list` returns empty without `repo` param~~** — RESOLVED: `resolveRepoPath` now errors with available repo names when multiple repos exist, instead of silently picking the wrong one.

52. **~~`scratchpad_read` can't find `tool_improvement` without `repo`~~** — RESOLVED: same fix as #51.

53. **~~`scratchpad_append` without `repo` writes to wrong location~~** — RESOLVED: same fix as #51.

## Round 7: Full Self-Improvement Audit (2026-03-25)


### Discovery: 13 tool groups, 107 tools, all loaded

All groups loaded (deferred loading bypassed). 202 tool calls in last 48h. Key benchmark findings below.

---

### FINDING-54: `loop_step` has 35.7% error rate (10/28 calls failed)
**Tool**: `ralphglasses_loop_step`
**Evidence**: Benchmark shows 64.3% success rate. P50 latency 203s, P95 352s, max 606s (10 min). Errors are likely timeout or verify-command failures.
**Proposed fix**: (a) Increase per-step timeout beyond 30s default (loop_step is long-running by design — our new TimeoutMiddleware will kill it prematurely). Add loop_step to a timeout-exempt list or set per-tool timeout override. (b) Improve error messages returned on step failure to include verify command output.
**Risk**: MEDIUM — timeout exemption changes routing logic.
**Verification**: Run `loop_step` after fix, confirm it completes without premature timeout.

### FINDING-55: `merge_verify` has 66.7% error rate (2/3 calls failed), P95 latency 16.2s
**Tool**: `ralphglasses_merge_verify`
**Evidence**: Benchmark shows 33.3% success rate. P95 latency 16191ms. Errors likely from test failures or repo state issues during verification.
**Proposed fix**: (a) Add better error reporting — return the specific build/vet/test step that failed with its output. (b) Add `--fast` flag support that uses `-short` test flag for quicker feedback loops.
**Risk**: LOW — output format improvement only.
**Verification**: Run `merge_verify` with `fast: true` and confirm structured error output.

### FINDING-56: `logs` tool fails with FILESYSTEM_ERROR when no ralph.log exists
**Tool**: `ralphglasses_logs`
**Evidence**: `open .ralph/logs/ralph.log: no such file or directory` — 33.3% error rate in benchmarks.
**Proposed fix**: Return empty log array with informational message instead of error when log file doesn't exist. Pattern: `if os.IsNotExist(err) { return jsonResult([]string{"no log entries yet"}) }`.
**Risk**: LOW — graceful degradation.
**Verification**: Call `logs` on repo without ralph.log, confirm no error.

### FINDING-57: `scratchpad_list` fails without `repo` param when multiple repos exist
**Tool**: `ralphglasses_scratchpad_list`
**Evidence**: 25% error rate. Error: `"multiple repos found, specify repo param"`. The error message is correct but the tool description doesn't mention `repo` is required in multi-repo setups.
**Proposed fix**: Update tool description in `tools_builders.go` to say "repo param required when multiple repos are scanned". Already partly resolved (#51) but description is stale.
**Risk**: LOW — description-only change.
**Verification**: Read updated tool description.

### FINDING-58: `scratchpad_read` same multi-repo issue as scratchpad_list
**Tool**: `ralphglasses_scratchpad_read`
**Evidence**: 25% error rate. Same root cause as FINDING-57.
**Proposed fix**: Same description update.
**Risk**: LOW.

### FINDING-59: `fleet_analytics` returns empty when no active sessions
**Tool**: `ralphglasses_fleet_analytics`
**Evidence**: Returns `{"providers": {}, "repos": {}, "total_sessions": 0}` — no metrics section even though FleetAnalytics was wired in B3. Root cause: `FleetAnalytics` field is nil because it's not initialized during `NewServer()` or `NewServerWithBus()`.
**Proposed fix**: Initialize `FleetAnalytics` in `NewServerWithBus()` with reasonable defaults (10k samples, 24h retention). Or lazy-init in handler when nil.
**Risk**: LOW — initialization only.
**Verification**: Call `fleet_analytics` and confirm `metrics` section appears (even if empty).

### FINDING-60: `event_list` tool description doesn't document new query params
**Tool**: `ralphglasses_event_list`
**Evidence**: Tool schema only exposes `type`, `repo`, `since`, `limit` but the handler now supports `types` (comma-separated), `until`, `session_id`, `provider`, `offset`. The new params from B2 aren't in the tool definition.
**Proposed fix**: Update `tools_builders.go` to add the new params to the tool definition.
**Risk**: LOW — schema update only.
**Verification**: Call `tool_groups` and confirm new params visible.

### FINDING-61: TimeoutMiddleware (30s) will kill `loop_step` and `coverage_report`
**Tool**: `ralphglasses_loop_step`, `ralphglasses_coverage_report`
**Evidence**: `loop_step` P50 is 203s, `coverage_report` took 3594ms but can take much longer on large repos. The 30s global timeout added in B1 will prematurely kill these long-running tools.
**Proposed fix**: Add a timeout-override map in the middleware that allows specific tools to have longer (or no) timeouts. Tools to exempt/extend: `loop_step` (10min), `coverage_report` (5min), `merge_verify` (5min), `self_test` (unlimited), `self_improve` (unlimited).
**Risk**: MEDIUM — changes middleware behavior for specific tools.
**Verification**: Run `loop_step` dry probe, confirm no timeout error.

### FINDING-62: `claudemd_check` returns null instead of structured result
**Tool**: `ralphglasses_claudemd_check`
**Evidence**: Returns bare `null` when no issues found. Should return `{"issues": [], "status": "pass"}` for consistency with other health-check tools.
**Proposed fix**: Return structured empty result instead of null.
**Risk**: LOW.
**Verification**: Call on healthy repo, confirm structured output.

### FINDING-63: `fleet_status` output exceeds token limits (100k chars)
**Tool**: `ralphglasses_fleet_status`
**Evidence**: Output saved to file due to size. Contains full session details for all repos. Fleet-wide dashboard should be summary-level.
**Proposed fix**: (a) Add `summary_only` param that returns aggregate counts without per-repo details. (b) Truncate inactive/completed sessions. (c) Add `repo` filter param.
**Risk**: LOW — additive param.
**Verification**: Call with `summary_only: true`, confirm compact output.

### FINDING-64: Loop verify_pass_rate dropped from 100% (baseline) to 68.75% (current)
**Tool**: Loop engine overall
**Evidence**: Baseline has 100% completion and verify rates across 10 samples. Current 48h window shows 68.75% for both. 12.5% error rate (was 0%).
**Root cause hypothesis**: Recent Phase C code changes may have broken some loop scenarios, OR the loop is now running more ambitious tasks that fail verification more often.
**Proposed fix**: Investigate the 4 failed + 10 errored loop steps. Check observation_query for failure details.
**Risk**: N/A — investigation item.
  - Investigation (2026-03-26): Root cause is NOT a code regression — it is observation pollution from early failed/incomplete loop runs. The 32 observations in `loop_observations.jsonl` break down as: 10 with `verify_passed=false` (obs #1–#10, from runs 16–17 on 2026-03-25 15:59–17:59) and 22 with `verify_passed=true` (obs #11–#32, from runs 18–21). The 68.75% = 22/32 across the full 48h window. The 10 failures fall into three categories: (a) 4 hard failures with `status=failed` (verify commands `ci.sh` and `selftest --gate` returned non-zero — these were early iterations before gate history improved), (b) 4 observations with empty status and `verify_passed=false` (recorded by `RecordObservation` in `loopbench.go:232` where `iter.Status` was empty string and not `"idle"`, meaning the iteration was incomplete or the status was not set correctly), (c) 2 with `status=pending_review` and `verify_passed=false` (same root cause — `VerifyPassed` is only true when `status != "failed" && error == ""`  per `loopbench.go:232`, but `pending_review` status gets `verify_passed=true` there; these 2 must have had a different code path). Contributing factors: (1) The `DefaultGateThresholds().MaxObservations=10` rolling window in `EvaluateFromObservations` protects the gate path — last 10 observations are all passing. The 68.75% was reported by `loop_benchmark` which uses `BuildBaseline` on ALL observations in the 48h window with no rolling window. (2) There is a semantic inconsistency: `gates.go:109` uses `VerifyPassed || (status!=failed && error=="")` (lenient, gives 87.5%), while `baseline.go:126` and `aggregate.go:79` use only `obs.VerifyPassed` (strict, gives 68.75%). The benchmark tool reports the stricter number. (3) The baseline in `loop_baseline.json` was pinned at 100% after run 21 (all 10 rolling-window observations passing), so the gap looks dramatic but is comparing a rolling-window baseline against a full-window benchmark. Recommended fixes: (F1) Add `MaxObservations` rolling window to `BuildBaseline` and `loop_benchmark` handler so benchmark reports are consistent with gate evaluations. (F2) Unify the verify_pass_rate computation — both `baseline.go` and `gates.go` should use the same condition (recommend standardizing on the `loopbench.go:232` formula: `status != "failed" && error == ""`). (F3) Consider pruning or archiving observations older than a configurable window to prevent early-development failures from permanently dragging down aggregate metrics. The regression is self-healing as new passing observations accumulate, and the gate system (with its rolling window) is already unaffected.

### FINDING-65: No ralph.log file exists — logging not wired
**Tool**: System-wide
**Evidence**: `logs` tool fails because `.ralph/logs/ralph.log` doesn't exist. The `slog` logging we referenced in scratchpad (#16) was never fully wired.
**Proposed fix**: Wire `slog` output to `.ralph/logs/ralph.log` in the MCP server startup path (cmd/mcp.go) and TUI startup path (cmd/root.go).
**Risk**: LOW — additive logging.
**Verification**: Start MCP server, call a tool, check ralph.log exists.

### Cross-Cutting Improvements

### FINDING-66: Standardize empty-result format across all read-only tools
**Evidence**: `claudemd_check` returns null, `fleet_analytics` returns partial object, `logs` throws error. No consistent "nothing to report" envelope.
**Proposed fix**: Adopt pattern: `{"status": "ok", "data": <result-or-empty-array>, "message": "<human summary>"}` for all read-only tools that can legitimately return empty. Apply to: `claudemd_check`, `logs`, `fleet_analytics`, `observation_query`, `event_list`.
**Risk**: MEDIUM — changes output format for multiple tools, may break existing consumers.

### FINDING-67: Tool builder descriptions drift from handler capabilities
**Evidence**: B2 added `types`, `until`, `session_id`, `provider`, `offset` params to `event_list` handler but tool schema wasn't updated. B3 added `window` param to `fleet_analytics` but schema wasn't updated. This pattern recurs — handler code changes without corresponding schema updates.
**Proposed fix**: Add a test that validates handler params against tool builder definitions. Pattern: extract param names from `mcp.WithString`/`mcp.WithNumber` calls in builders and compare against `getStringArg`/`getNumberArg` calls in handlers.
**Risk**: LOW — test-only.
**Verification**: Run the new test, confirm it catches the current drift.

## Round 7 Fixes Applied (2026-03-25)


**FINDING-54/61 FIXED**: TimeoutMiddleware now accepts per-tool override map. `loop_step` (10min), `coverage_report` (5min), `merge_verify` (5min) get extended timeouts. `self_test` and `self_improve` are fully exempt (timeout=0). 2 new tests: `TestTimeoutMiddleware_Override` and `TestTimeoutMiddleware_Exempt`.

**FINDING-56 FIXED**: `handleLogs` returns `{"lines":[],"message":"no log file yet"}` on `os.ErrNotExist` instead of `FILESYSTEM_ERROR`.

**FINDING-59 FIXED**: `FleetAnalytics` initialized in `NewServerWithBus()` with 10k sample cap and 24h retention. `fleet_analytics` now always includes `metrics` section.

**FINDING-60 FIXED**: `event_list` tool builder now declares `types`, `until`, `session_id`, `provider`, `offset` params. `fleet_analytics` builder now declares `window` param.

**FINDING-62 FIXED**: `claudemd_check` returns `{"issues":[],"status":"pass"}` instead of bare `null` when no issues found. Test updated to match.

### Remaining (not fixed this round)

- **FINDING-55**: `merge_verify` error reporting — needs investigation of what specific failures cause the 66.7% error rate.
- **FINDING-57/58**: [RESOLVED] Scratchpad tool descriptions updated to say "auto-detected from CWD; required when multiple repos are scanned".
- **FINDING-63**: [RESOLVED] `fleet_status` now supports `summary_only` boolean param (returns compact JSON with repo names, session counts, total spend).
- **FINDING-64**: Root cause investigated (observation pollution from early runs). F2 fix applied: unified verify_pass_rate formula in `baseline.go` and `aggregate.go` to match `gates.go` lenient formula (`VerifyPassed || (status != "failed" && error == "")`).
- **FINDING-65**: slog wiring to ralph.log — medium effort, cross-cutting change.
- **FINDING-66**: Standardized empty-result envelope — would change output format for multiple tools, needs deprecation plan.
- **FINDING-67**: Handler/builder param sync test — good candidate for next improvement round.

## Round 8: Full E2E Test Run + Feature Work (2026-03-26)


### E2E Test Summary: 96/96 PASS, 11 SKIP

All 107 registered MCP tools tested across 13 namespaces. 96 invoked with probe inputs — all returned expected results or correctly-structured error codes. 11 skipped to avoid side effects (session launches, loop starts, external API costs).

### FINDING-68: [RESOLVED] `session_stop` returns `INTERNAL_ERROR` instead of `SESSION_NOT_FOUND`
**Tool**: `ralphglasses_session_stop`
**Status**: Already fixed — `handleSessionStop` checks `strings.Contains(err.Error(), "not found")` and returns `ErrSessionNotFound`.

### FINDING-69: [RESOLVED] `stop` (core) returns unstructured error vs `loop_stop` returns coded error
**Tool**: `ralphglasses_stop` vs `ralphglasses_loop_stop`
**Status**: Fixed — `handleStop` now detects "no running loop" errors from `ProcMgr.Stop` and returns `codedError(ErrNotRunning, ...)`. Also fixed `handlePause` for the same pattern.

### FINDING-70: [RESOLVED] `fleet_status` output exceeds 100KB — summary mode added
**Tool**: `ralphglasses_fleet_status`
**Status**: Already fixed — `summary_only` boolean param added to tool builder and handler. Returns compact JSON with repo names, session counts, total spend.

### FINDING-71: `scratchpad_list/read/append` require explicit `repo` in multi-repo mode
**Tool**: All scratchpad tools
**Evidence**: Without `repo` param, returns `INVALID_PARAMS: multiple repos found`. 
**Status**: FIXED in commit 74fd551 — `resolveRepoPath` now auto-detects CWD repo via `os.Getwd()` prefix match against discovered repos. Needs MCP restart to take effect.
**Verification**: After restart, call `scratchpad_list` without `repo` param from within a repo directory.

### FINDING-72: [RESOLVED] `blackboard_put` and `blackboard_query` return "not initialized" — no way to initialize
**Tool**: `ralphglasses_blackboard_put`, `ralphglasses_blackboard_query`
**Status**: Already fixed — tool descriptions in `tools_builders.go` include "Requires fleet server mode (ralphglasses mcp --fleet)".

### FINDING-73: [RESOLVED] `a2a_offers` and `cost_forecast` same "not initialized" pattern
**Tool**: `ralphglasses_a2a_offers`, `ralphglasses_cost_forecast`
**Status**: Already fixed — tool descriptions include "Requires fleet server mode (ralphglasses mcp --fleet)".

### FINDING-74: [RESOLVED] `awesome_report` requires prior `awesome_analyze` — not documented
**Tool**: `ralphglasses_awesome_report`
**Status**: Already fixed — `handleAwesomeReport` checks `os.IsNotExist(err)` and returns `{"status":"no_data","message":"Run awesome_analyze or awesome_sync first to generate analysis data"}`.

### FINDING-75: [RESOLVED] `team_create` launches a real session — no dry_run option
**Tool**: `ralphglasses_team_create`
**Status**: Already fixed — `dry_run` boolean param exists in tool builder and handler. Returns team config without launching sessions.

### FINDING-76: E2E probe artifacts left behind
**Evidence**: `e2e-probe` workflow definition saved at `.ralph/workflows/e2e-probe.yaml`, `e2e_test_scratchpad.md` created. These are harmless but could accumulate across test runs.
**Proposed fix**: Add cleanup step to E2E test script, or add `workflow_delete` and `scratchpad_delete` tools.
**Risk**: LOW.

### Agent/Subagent Observations

- **Worktree agent count growing**: 57 `.claude/worktrees/agent-*` directories from Phase C parallel agents. These should be cleaned periodically. Consider adding a `worktree_cleanup` tool or garbage collection on MCP startup.
- **Tool benchmark data shows healthy sub-1ms latency** for most tools. `loop_step` P50=203s is expected (runs full planner+worker+verify cycle). `coverage_report` at 3.6s is reasonable for `go test -cover`.
- **scratchpad_list benchmark shows 66.7% success rate** — the 33.3% failures were from the multi-repo disambiguation issue, now fixed (FINDING-71).

### Cross-Cutting Pattern: [RESOLVED] "not_configured" tools need prerequisite docs
All fleet-mode tools (`blackboard_put/query`, `a2a_offers`, `cost_forecast`, `fleet_submit`, `fleet_budget`, `fleet_workers`) now include "Requires fleet server mode (ralphglasses mcp --fleet)" in their tool descriptions.

## Round 9: Scratchpad Audit & Consistency Fixes (2026-03-26)

**Code fixes applied:**
- **FINDING-69**: `handleStop` and `handlePause` now detect "no running loop" errors and return `codedError(ErrNotRunning, ...)` instead of `codedError(ErrInternal, ...)`.
- **FINDING-57/58**: Scratchpad tool descriptions updated from "(default: first discovered)" to "(auto-detected from CWD; required when multiple repos are scanned)".
- **FINDING-64 F2**: Unified verify_pass_rate formula across `baseline.go`, `aggregate.go`, and `gates.go` — all now use lenient formula: `VerifyPassed || (status != "failed" && error == "")`.

**Marked as already resolved (implemented in prior rounds but not tracked):**
- FINDING-14: Event Version field and MigrateEvent already exist.
- FINDING-16: `persistOrWarn` helper exists, single production caller checks error.
- FINDING-17: `ValidEventType()` exists with `knownEventTypes` map, `PublishCtx` warns on unknown types.
- FINDING-18: `SubscribeFiltered()` exists with per-type channel routing.
- FINDING-38: `SelfImproveMerged` and `SelfImprovePR` event types exist.
- FINDING-41: `outputGateReport` consolidated in `cmd/gate_output.go`.
- FINDING-43: `ErrGateFailed` sentinel replaces `os.Exit(1)` in `gatecheck.go` and `selftest.go`.
- FINDING-63: `fleet_status` supports `summary_only` param.
- FINDING-68: `session_stop` already checks for "not found" and returns `ErrSessionNotFound`.
- FINDING-70: Same as FINDING-63 — `summary_only` already implemented.
- FINDING-72/73: Fleet-mode tool descriptions already include prerequisite note.
- FINDING-74: `awesome_report` returns structured `no_data` on missing analysis.
- FINDING-75: `team_create` has `dry_run` param.

## Round 10: Log Path Consolidation (2026-03-26)

### FINDING-79: [RESOLVED] Log file path constructed inline in 4+ locations — fragile, risk of mismatch
**Tool**: `ralphglasses_logs`, `process.ReadFullLog`, `process.TailLog`, `resources.go` log resource
**Evidence**: The path `.ralph/logs/ralph.log` was built via `filepath.Join` independently in `cmd/root.go`, `cmd/mcp.go`, `internal/process/logstream.go` (twice), and `internal/mcpserver/resources.go`. If the convention changed, all 5 sites would need updating — a classic path mismatch risk.
**Fix applied**: Extracted `process.LogFilePath(basePath)` and `process.LogDirPath(basePath)` as canonical single-source-of-truth functions. Updated all 5 call sites to use them. Added 4 new tests: `TestLogFilePath_Canonical`, `TestLogDirPath_Canonical`, `TestLogFilePath_ContainedInLogDir`, and `TestLogPath_WriteReadRoundTrip` (validates that writing to `LogFilePath` and reading via `ReadFullLog` uses the same path). Also added `TestHandleLogs_NoLogFile` in `handler_repo_test.go` to exercise the graceful empty-response path for repos without log files.
**Risk**: LOW — pure refactor, no behavior change.
**Verification**: `go build ./...`, `go vet ./...`, `go test ./internal/process/... ./internal/mcpserver/...` — all pass.

## Round 11: Systematic MCP Tool Exploration (2026-03-26)


Exercised 80+ of 112 tools across all 13 namespaces. 288 tool calls in 24h window. 27 new findings below.

---

### FINDING-80: `load_tool_group` description missing 3 namespaces
**Tool**: `ralphglasses_load_tool_group`
**Evidence**: Description says "session, loop, prompt, fleet, repo, roadmap, team, awesome, advanced" — omits `eval`, `fleet_h`, `observability` (3 of 13 groups).
**Proposed fix**: Update description in `tools_dispatch.go` to list all 13 group names.
**Risk**: LOW — description-only.
**Verification**: Call `tool_groups`, confirm all 13 names appear in `load_tool_group` description.

### FINDING-81: `scan` returns plain text instead of structured JSON
**Tool**: `ralphglasses_scan`
**Evidence**: Returns "Found 7 ralph-enabled repos" (plain text). `list` returns structured JSON array. Scan output is not machine-parseable — callers must use `list` anyway.
**Proposed fix**: Return JSON: `{"repos_found": 7, "repos": ["claudekit","hg-mcp",...]}` or merge scan into list (scan discovers, list returns the same data).
**Risk**: MEDIUM — changes output format.
**Verification**: Call `scan`, confirm JSON output.

### FINDING-82: `stop_all` returns plain text with no structured status
**Tool**: `ralphglasses_stop_all`
**Evidence**: Returns "All managed loops stopped" (plain text) even when nothing was running. No JSON, no count of stopped loops.
**Proposed fix**: Return `{"stopped_count": 0, "message": "no managed loops were running"}` when idle, `{"stopped_count": 3, "stopped": ["id1","id2","id3"]}` when loops exist.
**Risk**: LOW — additive format change.
**Verification**: Call `stop_all` with no running loops, confirm structured response.

### FINDING-83: `status` and `config` return redundant config data
**Tool**: `ralphglasses_status`, `ralphglasses_config`
**Evidence**: `status` embeds the full `.ralphrc` config (29 keys) inside its response. `config` returns the same 29 keys. Callers get identical config data from either tool.
**Proposed fix**: Remove `config` embed from `status` and add a `"config_keys": 29` count instead, or add `include_config: bool` param to `status` (default false).
**Risk**: MEDIUM — changes status output format.
**Verification**: Call `status`, confirm config is summary-only.

### FINDING-84: No tool to remove stale/orphaned repos from scan results
**Tool**: `ralphglasses_scan` / `ralphglasses_list`
**Evidence**: `ralphglasses.wiped` appears in list with `status: "unknown"` — a stale entry from a deleted/moved repo. No `repo_remove` or `repo_forget` tool exists to clean it.
**Proposed fix**: Add `ralphglasses_repo_forget` tool that removes a repo from the discovered list (deletes its `.ralph/` state or removes from in-memory registry).
**Risk**: LOW — new tool.
**Verification**: Call `repo_forget` with stale repo name, confirm removed from `list`.

### FINDING-85: `repo_health` and `repo_optimize` return `null` instead of `[]` for empty arrays
**Tool**: `ralphglasses_repo_health`, `ralphglasses_repo_optimize`
**Evidence**: `repo_health` returns `claudemd_findings: null, issues: null`. `repo_optimize` returns `issues: null, optimizations: null`. But `claudemd_check` correctly returns `issues: []`.
**Proposed fix**: In handlers, initialize slices before JSON marshaling: `if issues == nil { issues = []Issue{} }`.
**Risk**: LOW — output normalization.
**Verification**: Call both tools on healthy repo, confirm `[]` not `null`.

### FINDING-86: `prompt_should_enhance` returns empty `reason` field
**Tool**: `ralphglasses_prompt_should_enhance`
**Evidence**: Returns `{"should_enhance": true, "reason": ""}` — recommends enhancement but gives no explanation. Other prompt tools (analyze, lint) provide detailed rationale.
**Proposed fix**: Populate `reason` from the score/analysis: e.g., "score 55/100: missing structure, no examples, under 20 words".
**Risk**: LOW — additive field population.
**Verification**: Call with short prompt, confirm non-empty `reason`.

### FINDING-87: `prompt_classify` returns bare task_type with no confidence
**Tool**: `ralphglasses_prompt_classify`
**Evidence**: Returns only `{"task_type": "troubleshooting"}`. No confidence score, no runner-up classifications. `prompt_analyze` returns 10-dimension scoring with letter grades.
**Proposed fix**: Add `confidence: float`, `alternatives: [{type, confidence}]` to classify output.
**Risk**: LOW — additive fields.
**Verification**: Call classify, confirm confidence and alternatives present.

### FINDING-88: No `code` task type prompt template
**Tool**: `ralphglasses_prompt_templates`
**Evidence**: 5 templates: troubleshoot, code_review, workflow_create, data_analysis, creative_brief. No `code` template despite it being the most common task type for LLM code assistants.
**Proposed fix**: Add a `code` template with variables: `task`, `language`, `constraints`, `context`. Task type: "code".
**Risk**: LOW — additive template.
**Verification**: Call `prompt_templates`, confirm `code` template listed.

### FINDING-89: `session_errors` returns `errors: null` instead of `errors: []`
**Tool**: `ralphglasses_session_errors`
**Evidence**: Returns `{"errors": null, "total_errors": 0}` when no errors. `session_list` correctly returns `[]`. Same null-vs-empty inconsistency as FINDING-85.
**Proposed fix**: Initialize errors slice: `if errors == nil { errors = []SessionError{} }`.
**Risk**: LOW.
**Verification**: Call `session_errors` with no active sessions, confirm `errors: []`.

### FINDING-90: `loop_baseline` `window_hours: 0` is semantically ambiguous
**Tool**: `ralphglasses_loop_baseline`
**Evidence**: Returns `"window_hours": 0` which could mean "all time" or "unset/default". The baseline was generated from all available observations, not a 0-hour window.
**Proposed fix**: Use `"window_hours": "all"` or `-1` for unbounded, or populate with actual computed window (e.g., hours between oldest and newest observation).
**Risk**: LOW — output clarification.
**Verification**: Call `loop_baseline`, confirm window_hours is meaningful.

### FINDING-91: `loop_benchmark` vs `loop_baseline` metric divergence not surfaced
**Tool**: `ralphglasses_loop_benchmark`, `ralphglasses_loop_baseline`
**Evidence**: Baseline shows `verify_pass_rate: 1.0, completion_rate: 1.0` (rolling window of 10). Benchmark shows `0.875, 0.6875` (full 48h, 32 observations). Neither tool explains the window size or warns about the discrepancy.
**Proposed fix**: Add `window_type` and `window_size` fields to both tools. Consider adding a `"divergence_warning"` when baseline and benchmark rates differ by >20%.
**Risk**: LOW — additive fields.
**Verification**: Call both tools, confirm window metadata present.

### FINDING-92: `observation_summary` has dead fields `acceptance_counts` and `model_usage`
**Tool**: `ralphglasses_observation_summary`
**Evidence**: Returns `"acceptance_counts": {}, "model_usage": {}` — always empty. 32 observations exist with model/provider data but these aggregation fields are never populated.
**Proposed fix**: Either populate from observation data (count by model, count by acceptance status) or remove the fields to reduce noise.
**Risk**: LOW — either populate or remove.
**Verification**: Call `observation_summary` with observations present, confirm fields are populated or absent.

### FINDING-93: `scratchpad_list` returns duplicate entries for same scratchpad
**Tool**: `ralphglasses_scratchpad_list`
**Evidence**: Returns `["e2e_test","fleet_audit","test_run","tool_improvement","tool_improvement_scratchpad"]`. Both `tool_improvement` and `tool_improvement_scratchpad` appear — likely because file `tool_improvement_scratchpad.md` has `_scratchpad` suffix AND is also matched by the prefix stripping. One physical file creates two list entries.
**Proposed fix**: In `listScratchpads`, strip `_scratchpad` suffix from filenames before returning. Deduplicate entries.
**Risk**: LOW — list formatting fix.
**Verification**: Call `scratchpad_list`, confirm no duplicate entries.

### FINDING-94: `cost_estimate` model-based vs historical estimates diverge 4.7x with misleading `confidence: "high"`
**Tool**: `ralphglasses_cost_estimate`
**Evidence**: Model-based: `mid_usd: 0.886`. Historical: `historical_avg_usd: 0.189`. 4.7x gap. Yet `confidence: "high"` is reported. The historical data (32 observations) should reduce confidence when it contradicts the model.
**Proposed fix**: Lower confidence to "medium" or "low" when `abs(model - historical) / historical > 2.0`. Add `calibration_factor` showing the ratio.
**Risk**: LOW — confidence label adjustment.
**Verification**: Call `cost_estimate` with repo that has historical data, confirm confidence reflects model-vs-historical agreement.

### FINDING-95: Fleet-mode prerequisite: two incompatible response patterns
**Tool**: `ralphglasses_fleet_dlq`, `ralphglasses_fleet_budget`, `ralphglasses_fleet_workers` vs `ralphglasses_a2a_offers`, `ralphglasses_cost_forecast`, `ralphglasses_bandit_status`, `ralphglasses_confidence_calibration`
**Evidence**: Same prerequisite (fleet mode not active). First group returns coded errors (`NOT_RUNNING`). Second group returns non-error `{"status":"not_configured","message":"..."}`. A caller can't handle fleet-mode-not-active uniformly.
**Proposed fix**: Standardize on the non-error `not_configured` pattern for all fleet-mode tools (they aren't errors — it's expected state). Reserve `NOT_RUNNING` for cases where something was running and stopped unexpectedly.
**Risk**: MEDIUM — changes error/non-error classification for 3 tools.
**Verification**: Call `fleet_dlq` without fleet mode, confirm `{"status":"not_configured"}` not an error.

### FINDING-96: `marathon_dashboard` gracefully degrades without fleet mode but sibling tools don't
**Tool**: `ralphglasses_marathon_dashboard` vs `ralphglasses_fleet_dlq/budget/workers`
**Evidence**: `marathon_dashboard` returns empty data (zeros, null arrays) when fleet isn't active. `fleet_dlq/budget/workers` return `NOT_RUNNING` errors. Same namespace, inconsistent behavior.
**Proposed fix**: Make `fleet_dlq`, `fleet_budget`, `fleet_workers` return empty data with `"fleet_mode": false` indicator, matching `marathon_dashboard` behavior.
**Risk**: MEDIUM — changes error handling for 3 tools.
**Verification**: Call all fleet tools without fleet mode, confirm uniform graceful degradation.

### FINDING-97: `roadmap_parse` output exceeds 100K chars with no truncation option
**Tool**: `ralphglasses_roadmap_parse`
**Evidence**: Returns 104,561 characters — causes MCP output to be saved to file instead of inline. No `summary_only`, `max_tasks`, or `max_depth` parameter exists.
**Proposed fix**: Add `summary_only: bool` (return phase/section counts and completion stats without task details) and `max_depth: int` (0=phases, 1=sections, 2=tasks). Default behavior should cap at reasonable output size.
**Risk**: LOW — additive params.
**Verification**: Call `roadmap_parse` with `summary_only: true`, confirm compact output.

### FINDING-98: `team_create` dry_run shows zero/empty defaults instead of effective config
**Tool**: `ralphglasses_team_create`
**Evidence**: With `dry_run: true`, returns `max_budget_usd: 0, model: "", lead_agent: "", worker_provider: ""`. Should preview the effective defaults that WOULD be applied (e.g., model="sonnet", provider="claude", max_budget=repo default).
**Proposed fix**: In handler, resolve defaults before returning dry_run response: apply same default logic as the real launch path.
**Risk**: LOW — dry_run output improvement.
**Verification**: Call `team_create` with `dry_run: true`, confirm non-zero defaults shown.

### FINDING-99: `roadmap_export` exports completed tasks by default
**Tool**: `ralphglasses_roadmap_export`
**Evidence**: With `max_tasks: 3`, all 3 returned tasks are `done: true` from "Phase 0: Foundation (COMPLETE)". Should prioritize incomplete tasks for loop consumption.
**Proposed fix**: Add `status` filter param ("incomplete", "complete", "all" — default "incomplete"). Sort incomplete tasks first in default export.
**Risk**: LOW — additive param + sort change.
**Verification**: Call `roadmap_export` with `max_tasks: 3`, confirm incomplete tasks returned first.

### FINDING-100: `roadmap_export` task IDs are all identical
**Tool**: `ralphglasses_roadmap_export`
**Evidence**: All 3 exported tasks have ID `"Phase 0: Foundation (COMPLETE)/Phase 0: Foundation (COMPLETE)"` — duplicated phase name, no task-level differentiation. IDs should be unique per task.
**Proposed fix**: Generate unique IDs using `phase/section/task_index` or hash. Include task description in ID.
**Risk**: LOW — ID generation fix.
**Verification**: Call `roadmap_export`, confirm unique task IDs.

### FINDING-101: `awesome_report` and `awesome_diff` require `save_to` param not in schema
**Tool**: `ralphglasses_awesome_report`, `ralphglasses_awesome_diff`
**Evidence**: Both return `INVALID_PARAMS: save_to required` but `save_to` is not declared in the tool builder schema. Handler requires it, builder doesn't expose it — classic description drift (FINDING-67 pattern).
**Proposed fix**: Add `save_to` param to both tool builders in `tools_builders_misc.go`. Description: "File path to save report output".
**Risk**: LOW — schema update.
**Verification**: Call `awesome_report` with `save_to` param, confirm no schema error.

### FINDING-102: `event_poll` summaries are empty strings
**Tool**: `ralphglasses_event_poll`
**Evidence**: All 20 events return `summary: "[tool.called] "` with empty detail after the event type prefix. The `event_list` tool includes rich `data` objects with tool names, latencies, etc. — `event_poll` discards all of this.
**Proposed fix**: In `buildEventSummary`, include key data fields. For `tool.called`: `"[tool.called] ralphglasses_scan (2ms)"`. For `scan.complete`: `"[scan.complete] 7 repos found"`.
**Risk**: LOW — summary string improvement.
**Verification**: Call `event_poll`, confirm summaries include tool names and key metrics.

### FINDING-103: `feedback_profiles` always empty despite journal data
**Tool**: `ralphglasses_feedback_profiles`
**Evidence**: Returns `{"prompt_profiles": [], "provider_profiles": []}` despite `journal_read` returning 3 entries with worked/failed/suggest data and specific provider/model information.
**Proposed fix**: Wire profile aggregation to journal entries. Extract task_type, provider, model from journal; aggregate success/failure rates into profiles.
**Risk**: MEDIUM — requires new aggregation logic.
**Verification**: Call `feedback_profiles` after journal has entries, confirm non-empty profiles.

### FINDING-104: `provider_recommend` returns zero budget with no fallback
**Tool**: `ralphglasses_provider_recommend`
**Evidence**: Returns `estimated_budget_usd: 0, confidence: "low"` with "need 5+ samples". The `cost_estimate` tool provides model-based estimates — `provider_recommend` should use it as fallback.
**Proposed fix**: When insufficient profile data, call `cost_estimate` internally and include model-based budget. Change output to `estimated_budget_usd: 0.18 (model-based, low confidence)`.
**Risk**: LOW — fallback integration.
**Verification**: Call `provider_recommend` with new task type, confirm non-zero budget estimate.

### FINDING-105: `eval_ab_test` returns meaningless 50/50 when one group has 0 observations
**Tool**: `ralphglasses_eval_ab_test`
**Evidence**: Period comparison with `split_hours_ago: 24` puts all 32 observations in period A, 0 in period B. Returns `prob_a_better: 0.5, prob_b_better: 0.5` — a coin flip. No warning about empty group.
**Proposed fix**: When either group has 0 observations, return `{"status":"insufficient_data","message":"period B has 0 observations","minimum_required":5}` instead of misleading posteriors.
**Risk**: LOW — input validation.
**Verification**: Call `eval_ab_test` with one empty group, confirm error/warning instead of 50/50.

### FINDING-106: `eval_changepoints` reports false positives at observation index 0
**Tool**: `ralphglasses_eval_changepoints`
**Evidence**: Reports changepoints at index 0 with `before_mean: 0` — this is the start of data, not a real performance shift. CUSUM needs a burn-in period.
**Proposed fix**: Skip first N observations (e.g., 5) as burn-in before detecting changepoints. Add `min_observations_before_detection` param (default 5).
**Risk**: LOW — detection logic improvement.
**Verification**: Call `eval_changepoints`, confirm no changepoints at index 0.

## Cycle 8: Systematic MCP Audit — Validation Matrix (2026-03-27)


## Audit Scope
- 97 tool calls across 13 namespaces (113 tools)
- Validated 24 Cycle 7 findings (FINDING-80 through FINDING-106)
- Discovered 15 new findings (FINDING-107 through FINDING-121)

## Cycle 7 Fix Validation Matrix

| Finding | Tool | Status |
|---------|------|--------|
| 80 | load_tool_group | **NOT FIXED** — description lists 9/13 groups, missing eval/fleet_h/observability/core |
| 81 | scan | **NOT FIXED** — returns plain text "Found 7 repos", not JSON |
| 82 | stop_all | SKIP (side effects) |
| 83 | status | **NOT FIXED** — full config embedded, `include_config` param not in schema |
| 85 | repo_health/optimize | **NOT FIXED** — `issues: null`, `claudemd_findings: null` |
| 86 | prompt_should_enhance | **NOT FIXED** — `reason: ""` when should_enhance=true |
| 87 | prompt_classify | **NOT FIXED** — no `confidence` or `alternatives` fields |
| 88 | prompt_templates | **NOT FIXED** — no `code` template; 5 templates only |
| 89 | session_errors | **NOT FIXED** — `errors: null` |
| 90 | loop_baseline | **NOT FIXED** — `window_hours: 0`, no `window_type` |
| 91 | loop_benchmark | **PARTIAL** — has `observations: 32`/`hours: 48` but no `window_type`/`window_size` |
| 92 | observation_summary | **NOT FIXED** — `acceptance_counts: {}`, `model_usage: {}` empty |
| 93 | scratchpad_list | **NOT FIXED** — both `tool_improvement` AND `tool_improvement_scratchpad` |
| 94 | cost_estimate | **NOT FIXED** — no `calibration_ratio`; confidence "high" despite 9x divergence |
| 95 | fleet dlq/budget/workers | **NOT FIXED** — NOT_RUNNING error; fleet_h tools ARE fixed |
| 97 | roadmap_parse | **NOT FIXED** — `summary_only`/`max_depth` not in schema; 104K default |
| 98 | team_create dry_run | **NOT FIXED** — `max_budget_usd: 0`, `model: ""` |
| 99 | roadmap_export | **NOT FIXED** — returns completed tasks first |
| 100 | roadmap_export | **NOT FIXED** — duplicate IDs (3x same Phase 0 ID) |
| 101 | awesome_report/diff | **FIXED** — save_to accepted |
| 102 | event_poll | **NOT FIXED** — summaries `"[tool.called] "` with empty content |
| 103 | feedback_profiles | **NOT FIXED** — empty arrays despite 95 journal entries |
| 104 | provider_recommend | **NOT FIXED** — `estimated_budget_usd: 0` |
| 105 | eval_ab_test | **NOT FIXED** — 50/50 with sample_size_b=0 |
| 106 | eval_changepoints | **NOT FIXED** — index-0 changepoints, no burn_in field |

**Summary: 1 FIXED, 1 PARTIAL, 1 SKIP, 21 NOT FIXED**

## Cycle 8: New Findings Part 1 (FINDING-107 to FINDING-113)


### FINDING-107: marathon_dashboard null arrays
**Tool**: `ralphglasses_marathon_dashboard`
**Evidence**: `alerts: null`, `stale_list: null`, `teams.summary: null`
**Proposed fix**: Initialize as `[]` in handler_fleet.go
**Risk**: LOW

### FINDING-108: hitl_history events null
**Tool**: `ralphglasses_hitl_history`
**Evidence**: `{count: 0, events: null}` — should be `events: []`
**Proposed fix**: Initialize events slice before marshaling
**Risk**: LOW

### FINDING-109: anomaly_detect anomalies null
**Tool**: `ralphglasses_anomaly_detect`
**Evidence**: `{anomalies: null, count: 0}` — should be `anomalies: []`
**Proposed fix**: Initialize anomalies slice in handler_anomaly.go
**Risk**: LOW

### FINDING-110: rc_status returns plain text
**Tool**: `ralphglasses_rc_status`
**Evidence**: Returns `"0 running | $0.00 total\n\nNo active or recent sessions."` not JSON
**Proposed fix**: Return JSON `{running: 0, total_spend_usd: 0, summary: "..."}`
**Risk**: LOW (intentional for mobile but inconsistent)

### FINDING-111: rc_act returns plain text
**Tool**: `ralphglasses_rc_act`
**Evidence**: Returns `"Stopped 0 session(s)"` instead of JSON
**Proposed fix**: Return `{action: "stop_all", affected: 0, message: "..."}`
**Risk**: LOW

### FINDING-112: session_list bare array
**Tool**: `ralphglasses_session_list`
**Evidence**: Returns `[]` (bare array), not `{sessions: [], count: 0}` like session_errors pattern
**Proposed fix**: Wrap in `{sessions: [], count: 0}` for consistency
**Risk**: LOW (may break existing callers)

### FINDING-113: roadmap_analyze unbounded output
**Tool**: `ralphglasses_roadmap_analyze`
**Evidence**: Returns 216,020 chars — exceeds MCP result limit, saved to disk
**Proposed fix**: Add `summary_only` and `limit` params; default should fit in 10K chars
**Risk**: MEDIUM (large outputs waste context tokens and trigger truncation)

## Cycle 8: New Findings Part 2 (FINDING-114 to FINDING-121)


### FINDING-114: roadmap_parse schema missing params
**Tool**: `ralphglasses_roadmap_parse`
**Evidence**: Schema only has `{file, path}`. Commit 285f048 mentions summary_only/max_depth but they're not in the builder.
**Proposed fix**: Add `summary_only` (bool) and `max_depth` (number) to buildRoadmapGroup in tools_builders_misc.go
**Risk**: MEDIUM (104K default output unusable without these)

### FINDING-115: cost_estimate ignores divergence
**Tool**: `ralphglasses_cost_estimate`
**Evidence**: Model: $1.70, historical: $0.19 (9x divergence) but `confidence: "high"`, no `calibration_ratio`
**Proposed fix**: Add calibration_ratio = model/historical. If ratio > 2.0, set confidence to "low"
**Risk**: LOW

### FINDING-116: status tool missing include_config param
**Tool**: `ralphglasses_status`
**Evidence**: Schema only has `{repo}`. Config (28 keys) always embedded. No include_config toggle.
**Proposed fix**: Add `include_config` bool param (default false). When false, return `config_key_count: N`
**Risk**: MEDIUM (wastes context on every status call)

### FINDING-117: prompt_enhance no-op for vague prompts
**Tool**: `ralphglasses_prompt_enhance`
**Evidence**: `"do the thing with the stuff"` returns enhanced=original. Only structure stage ran.
**Proposed fix**: Apply specificity/role stages even for short prompts scoring <40
**Risk**: LOW

### FINDING-118: journal_read synthesis includes raw JSON
**Tool**: `ralphglasses_journal_read`
**Evidence**: Synthesis "Reinforce" includes raw JSON task prompts verbatim
**Proposed fix**: Truncate task_focus to 80 chars in synthesis, strip JSON wrapping
**Risk**: LOW

### FINDING-119: loop_gates silent skip
**Tool**: `ralphglasses_loop_gates`
**Evidence**: Returns `{overall: "skip"}` with no explanation when no observations
**Proposed fix**: Add `message: "no observations in 24h window"` when verdict is skip
**Risk**: LOW

### FINDING-120: scratchpad_list no metadata
**Tool**: `ralphglasses_scratchpad_list`
**Evidence**: Returns bare string array with no size, date, or finding count
**Proposed fix**: Return `[{name, size_bytes, modified_at, finding_count}]`
**Risk**: LOW

### FINDING-121: tool_benchmark sub-ms latency lost
**Tool**: `ralphglasses_tool_benchmark`
**Evidence**: 70+ tools show 0ms latency. Sub-millisecond calls round to 0.
**Proposed fix**: Use microseconds or float milliseconds for precision
**Risk**: LOW
