# Tool, Wiring & Workflow Improvement Opportunities

Observations from reliability & quality improvement workstreams + recursive self-testing design (2026-03-24).

## Worktree Agent Merge Pain Points

1. **Worktree agents don't auto-commit** — every agent leaves uncommitted diffs, requiring manual `\cp -f` to merge. If agents committed to their worktree branch, we could use `git merge` or `git cherry-pick` instead of file-level copies.

2. **Cross-worktree file conflicts require manual resolution** — when 2+ agents modify the same file (e.g., `runner.go` modified by R1 and R2, `manager.go` by R2 and R4, `bus.go` by R4 and R5), the merge is entirely manual. A merge assistant or conflict detection at dispatch time would help.

3. **Worktree agents start from stale main** — Phase B agents didn't see Phase A changes because worktrees branch from the current HEAD at launch time. For sequential phases, agents should ideally start from the post-merge state. Workaround: commit Phase A before launching Phase B agents.

## MCP Tool Gaps

4. **No `loop_observations` query tool** — after a loop run, querying `.ralph/logs/loop_observations.jsonl` requires raw file reads. A dedicated MCP tool (e.g., `ralphglasses_loop_observations`) could return filtered/aggregated observation data.

5. **No cost estimation tool** — before launching a session, there's no way to estimate cost. A `ralphglasses_cost_estimate` tool that takes prompt length + provider + model could return expected cost range.

6. **No event query tool** — now that events persist to JSONL (R5), a `ralphglasses_event_query` tool could search/filter events by type, session, time range.

## Test Infrastructure

7. **No test coverage tracking across runs** — `check-coverage.sh` enforces thresholds but doesn't track trends. A `ralphglasses_test_coverage` MCP tool or dashboard entry showing coverage over time would catch slow regression.

8. **Fuzz test corpus not persisted** — Go fuzz tests generate corpus entries in `testdata/fuzz/` but these aren't tracked in git. Should add `testdata/fuzz/` to `.gitignore` or decide to commit seed corpus.

## Provider Normalization

9. **Cost estimation is approximate** — R1's token-based cost fallback uses hardcoded rates that will drift as providers change pricing. Should periodically update `ProviderCostRates` or fetch from a config file.

10. **No provider output format documentation** — each normalizer (`normalizeClaudeEvent`, `normalizeGeminiEvent`, `normalizeCodexEvent`) has inline knowledge of the provider's JSON schema. This should be documented in `docs/PROVIDER-SETUP.md` with example outputs.

## Process Management

11. **Kill escalation timeout is hardcoded** — R2 uses 5s timeout. For long-running operations (large git operations, big compilations), the process may need more time for graceful shutdown. Should be configurable per-session or per-provider.

12. **~~No orphan detection~~** — RESOLVED: orphans.go implemented. ~~if ralphglasses crashes, spawned CLI processes continue running. A startup sweep checking for orphaned process groups (matching known session PIDs) would prevent cost leaks.~~

## Event Bus

13. **Event persistence is synchronous on publish path** — R5 writes to disk in `Publish()` under the mutex. For high-throughput scenarios (fleet with 10+ sessions), this could become a bottleneck. Consider buffered async writes.

14. **No event schema versioning** — JSONL events have no version field. When the Event struct changes, old persisted events may fail to deserialize. Add a `"v": 1` field.

## Worktree CWD Gotcha

15. **Agent CWD is its own worktree** — when this conversation's agent was spawned in a worktree (`agent-a7d1e22f`), `pwd` returns the worktree path, not the main repo. Relative paths like `.claude/worktrees/agent-xxx/...` fail because the worktree doesn't contain other worktrees. Always use absolute paths when copying between worktrees and main repo.

## Wiring Gaps (discovered during reliability work)

16. **PersistSession callers don't use the error** — R4 changed `PersistSession` to return `error`, but most callers use `_ = m.PersistSession(s)`. The error should at minimum be logged once `slog` is in place (Q7). Pattern: wrap in a helper like `m.persistOrWarn(s)` that logs + publishes event.

17. **Event types are stringly-typed** — `events.EventType` is a `string` type with constants, but nothing prevents typos or unknown types from being published. Consider adding a `ValidEventType()` check or using an enum/iota pattern.

18. **No event subscription filtering** — `Bus.Subscribe()` returns all events. Subscribers (TUI, marathon, fleet) each manually filter in their handler. Add `Bus.SubscribeFiltered(types ...EventType)` to reduce noise.

## Workflow Improvements (discovered during parallel workstream execution)

19. **Phase sequencing needs pre-commit** — Phase B agents didn't see Phase A changes because worktrees branched from stale HEAD. Fix: commit Phase A before launching Phase B agents. Alternatively, the orchestrator could create a temporary branch with Phase A changes for Phase B to branch from.

20. **No automated merge verification between phases** — After merging 3+ worktree agents, must manually run `go vet` + full test suite. Should have a `merge-verify.sh` script that: copies worktree diffs, runs vet+test, reports conflicts.

21. **Observation pipeline not wired to git diffs** — `LoopObservation` has `FilesChanged`/`LinesAdded`/`LinesRemoved` but no actual file paths. Fixing this (plan item 2.3) would enable tracing regressions to specific code changes.

22. **Self-test harness needs binary isolation** — The running `ralphglasses` binary can't safely modify its own source. Stage 1.2 of the recursive self-testing plan addresses this with a snapshot binary pattern, but it's also relevant for any loop that targets its own repo.

## New MCP Tool Opportunities

23. **`ralphglasses_self_test`** — Expose the self-test harness via MCP (plan item 1.5). Params: repo, iterations, budget, use_snapshot.

24. **`ralphglasses_event_query`** — Now that events persist to JSONL (R5), a query tool with filters (type, session, time range, limit) would replace raw file reads.

25. **`ralphglasses_observation_query`** — Filter/aggregate `.ralph/logs/loop_observations.jsonl` by loop_id, date range, provider. Currently requires `jq` on the command line.

26. **`ralphglasses_coverage_report`** — Run `go test -coverprofile` and return per-package coverage vs thresholds. Integrates with `check-coverage.sh`.

## Merge Workflow (Round 2 Observations)

27. **`cp -f` alias interference** — macOS aliases `cp` to `cp -i` (interactive) in some shells. Worktree merges need `/bin/cp -f` to bypass. Should document this or use Go file copy in merge tooling.

28. **~~Duplicate utility functions across packages~~** — RESOLVED: centralized in gitutil. ~~`gitDiffPaths()` exists as both `session.gitDiffPaths` (unexported, in protection.go) and `e2e.GitDiffPaths` (exported, in gitdiff.go). Same implementation. Should consolidate to one location — probably `e2e.GitDiffPaths` since it's the utility package — and have session import it.~~

29. **Worktree agents don't create new files in `git diff`** — new files (selftest.go, protection.go, errors.go) appear only as untracked files in worktrees, not in `git diff HEAD`. Need `git diff HEAD` + `git ls-files --others --exclude-standard` to capture full worktree output. Current merge workflow only uses `git diff --name-only HEAD` which misses new files.

30. **Health checker has two validation layers** — `CheckProviderHealth()` does full health (binary + env + version latency) while `HealthChecker.checkOne()` only does `ValidateProvider + ValidateProviderEnv`. The lightweight `checkOne` is fine for periodic heartbeat but could drift from the full check semantics.

31. **~~Error code adoption is partial~~** — RESOLVED: migration complete (216 calls). ~~Q5 converted `handler_loop.go` and `handler_session.go` to use `codedError()`, but ~20+ other handlers still use `errResult()`. Should progressively migrate all handlers. The `errResult` → `codedError` pattern is mechanical and could be automated.~~

32. **`EvaluateFromObservations` silently swallows baseline save errors** — Line `_ = saveErr` after rebuilding baseline. Should at minimum log when baseline persistence fails, since a missing baseline causes VerdictSkip on next run.

## Round 3: Agent Completion & Research Observations

33. **Worktree auto-cleanup loses new files** — Agents 1.2 and 1.5 created new files only (no modifications to tracked files). When the worktree had no `git diff` changes, it was auto-cleaned, losing the new untracked files. Workaround: agents must `git add` new files before completion, or the worktree cleanup logic should check `git ls-files --others` too.

34. **`selftest` CLI subcommand missing** — Stage 2 CI plan identifies that `cmd/selftest.go` doesn't exist yet. The `SelfTestRunner.Run()` invokes the binary but there's no `selftest` cobra command to receive the invocation. Needs to be created before CI integration works end-to-end.

35. **~~`gitDiffPaths` triplicated~~** — RESOLVED: gitutil package. ~~Now exists in 3 places: `e2e.GitDiffPaths` (exported), `session.gitDiffPaths` (unexported, in protection.go), and planned for `session.gitDiffPathsForWorktree` (Stage 2.3 loopbench). Should converge on one source. Since session can't import e2e (circular), either: (a) move to a shared `internal/gitutil` package, or (b) accept the duplication as import-cycle avoidance.~~

36. **Self-improvement profile needs `selftest --gate` subcommand** — `SelfImprovementProfile()` plans to use `go run . selftest --gate` as a verify command, but this subcommand doesn't exist. The `gate-check` command in `cmd/gatecheck.go` partially covers this but lacks the self-test iteration step.

37. **Two-tier acceptance needs `gh` CLI** — `CreateReviewPR` shell-outs to `gh pr create`. Should add `gh` to the provider prerequisites check or make PR creation optional with a fallback (create branch but skip PR if `gh` unavailable).

38. **No `selftest` event types** — Stage 3 plan proposes `SelfImproveMerged` and `SelfImprovePR` event types. These should be added to `events/bus.go` before building the acceptance gate or TUI dashboard.

## Round 4: Stage 2 Implementation Observations

39. **`SelfTestResult` field naming inconsistency** — WS-B agent discovered `SelfTestResult.Iterations` (actual) vs plan's `IterationsRun`. Plan docs drifted from implementation during Stage 1.2. Plans referencing struct fields should always be verified against code before implementation.

40. **`gate-check` and `selftest --gate` overlap** — Both `cmd/gatecheck.go` and `cmd/selftest.go --gate` call `EvaluateFromObservations`. The gate-check command takes `--baseline` and `--hours` flags; selftest --gate passes hardcoded defaults (0 hours = all observations). Should consider whether gate-check can be deprecated in favor of `selftest --gate` or whether they serve different audiences (gate-check = manual, selftest --gate = CI).

41. **`outputGateReport` duplicated** — `cmd/gatecheck.go:outputGateReport` and `cmd/selftest.go:outputSelftestGateReport` have nearly identical lipgloss rendering logic. Should extract to a shared `cmd/gate_output.go` helper. Low priority — cosmetic duplication.

42. **CI self-test job can't run live iterations yet** — The `selftest` command's full mode calls `e2e.Prepare` which builds a snapshot binary and needs API credentials. CI would need `ANTHROPIC_API_KEY` secret + cost budget. Initially deploying as `--gate` only is correct, but should track when to enable full iterations.

44. **Worktree agent replaced test file instead of appending** — WS-A agent was told to "append to loopbench_test.go or create if it doesn't exist" but replaced the entire file, deleting ~170 lines of existing tests. Root cause: the agent saw the file existed, decided to rewrite it with only the new tests. Workaround: manual merge in parent. Fix: agent prompts should explicitly say "do NOT delete existing test functions" when appending tests.

43. **`selftest` exit code via `os.Exit(1)` bypasses cobra** — Both `gatecheck.go` and `selftest.go` call `os.Exit(1)` inside `RunE`, which skips defer cleanup and cobra's error handling. Should return a sentinel error and handle exit codes in `main()` or a `PersistentPostRunE`. Low priority — works fine in practice.

## Round 5: Stage 3 Phase B Observations

45. **B2 agent created duplicate `SelfImprovementProfile()`** — Agent created `internal/session/selfimprove.go` with its own version of `SelfImprovementProfile()` despite A1 already adding it to `loop.go`. The two versions had different defaults (B2: $5/$15 budget, 2 workers, cascade enabled; A1: $1/$3 budget, 1 worker, cascade disabled per plan). Root cause: B2 didn't see A1's changes in its worktree. Fix: skip the duplicate file during merge, use A1's canonical version.

46. **Self-improve handler uses `errResult` instead of `codedError`** — `handler_selfimprove.go:72` uses `errResult()` for the loop start error, while the surrounding code uses `codedError()`. Should use `codedError(ErrLoopStart, ...)` for consistency with the error code migration (scratchpad #31).

47. **`maxIter` parameter unused in handler** — `handler_selfimprove.go` parses `max_iterations` param but doesn't pass it anywhere — `StartLoop` doesn't take a max iterations arg. The loop runs until stopped or budget exhausted. Either: (a) add `MaxIterations` to `LoopProfile`, or (b) remove the param and document that budget is the real limiter.

48. **~~`self-improve.sh` references `mcp-call` subcommand~~** — RESOLVED: mcpcall.go exists. ~~The script calls `./ralphglasses mcp-call ralphglasses_self_improve` but no `mcp-call` cobra command exists. Should either: (a) implement `mcp-call` as a thin wrapper that starts MCP, calls the tool, and exits, or (b) change the script to use the MCP protocol directly via stdin/stdout.~~
