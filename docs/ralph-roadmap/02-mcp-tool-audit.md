# 02 — MCP Tool Layer Audit (ralphglasses)

Generated: 2026-04-04
Source: `internal/mcpserver/` — all files read directly from source.

---

## 1. Tool Inventory by Namespace

163 tools registered across 16 namespaces, plus 3 meta-tools (`tool_groups`, `load_tool_group`, `skill_export`) always registered in dispatch. Total exposed surface: **166 tools**.

> Note: baseline doc (04-mcp-inventory.md) cited 126 tools across 14 namespaces. Actual count is 166 across 16 namespaces. The discrepancy is explained by: (a) the `plugin` and `sweep` namespaces were added after the baseline was written, and (b) several namespaces grew — `core` gained 3 task-management tools, `session` gained 3 (fork, handoff, replay_diff), `rdcycle` grew from 10 to 18, `observability` from 15 to 17.

| Namespace | Tool Count | Primary Builder File(s) | Handler File(s) | Loading | Line Count (builder) |
|-----------|-----------|------------------------|-----------------|---------|----------------------|
| **core** | 13 | `tools_builders.go` | `tools.go`, `handler_tasks.go` | Eager (always loaded) | 123 |
| **session** | 16 | `tools_builders_session.go` | `handler_session.go`, `handler_session_export.go`, `handler_session_fork.go`, `handler_session_handoff.go`, `handler_session_lifecycle.go` | Deferred | 275 |
| **loop** | 11 | `tools_builders_session.go` | `handler_loop.go`, `handler_loopbench.go`, `handler_loopwait.go`, `handler_selfimprove.go`, `handler_selftest.go` | Deferred | 275 (shared with session) |
| **prompt** | 9 | `tools_builders_session.go` | `handler_prompt.go`, `handler_prompt_ab.go` | Deferred | 275 (shared) |
| **fleet** | 10 | `tools_builders_fleet.go` | `handler_fleet.go`, `handler_fleet_capacity.go`, `handler_fleet_grafana.go`, `tools_fleet.go` | Deferred | 144 |
| **repo** | 5 | `tools_builders_fleet.go` | `handler_repo.go`, `handler_repo_fleet.go`, `handler_repo_health.go`, `handler_repo_scaffold.go` | Deferred | 144 (shared) |
| **roadmap** | 6 | `tools_builders_misc.go` | `handler_roadmap.go`, `handler_roadmap_prioritize.go` | Deferred | 597 |
| **team** | 6 | `tools_builders_misc.go` | `handler_team.go` | Deferred | 597 (shared) |
| **awesome** | 5 | `tools_builders_misc.go` | `handler_awesome.go` | Deferred | 597 (shared) |
| **advanced** | 24 | `tools_builders_misc.go` | `handler_rc.go`, `handler_circuit.go`, `handler_selfimprove.go`, `tools_fleet.go`, `tools_session.go` | Deferred | 597 (shared) |
| **eval** | 6 | `tools_builders_misc.go` | `handler_eval.go`, `handler_anomaly.go` | Deferred | 597 (shared) |
| **fleet_h** | 5 | `tools_builders_fleet.go` | `handler_fleet_h.go`, `handler_recommend.go` | Deferred | 144 (shared) |
| **observability** | 17 | `tools_session.go` | `handler_observation.go`, `handler_scratchpad.go`, `handler_scratchpad_advanced.go`, `handler_loopwait.go`, `handler_coverage.go`, `handler_costestimate.go`, `handler_mergeverify.go`, `handler_worktree.go` | Deferred | 434 |
| **rdcycle** | 18 | `tools_builders_misc.go`, `tools_session.go` | `handler_rdcycle.go`, `handler_cycle_engine.go` | Deferred | 597 (shared) + 434 |
| **plugin** | 4 | `tools_builders.go` | `handler_plugin.go` | Deferred | 123 (shared) |
| **sweep** | 8 | `tools_builders_sweep.go` | `handler_sweep.go`, `handler_sweep_report.go` | Deferred | 69 |

**Deferred loading default:** `DeferredLoading` field on `Server` is `false` by default. The production `cmd/mcp.go` calls `rg.Register(srv)` without setting `DeferredLoading = true`, so **all 166 tools are registered eagerly at startup**. The `tool_groups` / `load_tool_group` meta-tools are present but the deferred model is not activated in the default binary. Only tests explicitly set `DeferredLoading = true`.

---

## 2. Handler Quality Matrix

Ratings on a 1–5 scale. Criteria:
- **Input validation**: does the handler check required params, lengths, formats, and use `codedError`?
- **Error handling**: structured `codedError` used consistently? All error paths handled? No swallowed errors?
- **Test coverage**: does a `_test.go` exist? How many `Test*` functions?
- **Annotation completeness**: from `annotations.go` — are all 4 MCP spec hints set (`ReadOnlyHint`, `DestructiveHint`, `IdempotentHint`, `OpenWorldHint`)?

| Handler File | Lines | Input Validation (1–5) | Error Handling (1–5) | Test Coverage (1–5) | Annotation Completeness (1–5) | Notes |
|---|---|---|---|---|---|---|
| `handler_session.go` | 664 | 4 | 4 | 3 | 3 | Uses `NewParamParserFromRequest` + `RequireString` pattern. Good codedError usage. 14 tests in `handler_session_test.go` + 5 in lifecycle + 3 in export = ~22 total. Annotation: only `ReadOnlyHint` on read tools, missing `IdempotentHint` on `session_budget` |
| `handler_fleet.go` | 613 | 5 | 4 | 4 | 3 | `NewParams` + `RequireString` / `RequireNumber` pattern. Fleet-not-configured returns structured JSON instead of error — good UX, not a bug. 49 tests in `handler_fleet_test.go`. Missing `OpenWorldHint` on `fleet_submit` |
| `handler_loop.go` | 298 | 4 | 4 | 3 | 3 | `ValidateRepoName` called explicitly before lookup. Good scan/findRepo error chain. 14 tests in `handler_loop_test.go` + bench tests. `session_handoff` is defined in this file despite living in `loop` group — slight placement oddity |
| `handler_sweep.go` | 724 | 4 | 4 | 3 | 4 | Budget cap enforcement (`max_sweep_budget_usd`) is validated inline. 14 tests in `handler_sweep_test.go`. `sweep_report` has no dedicated test. Sweep annotations are thorough (4 of 8 tools have 2 hints, rest have 1) |
| `handler_prompt.go` | 380 | 4 | 3 | 3 | 3 | `handlePromptAnalyze` uses raw `getStringArg` (not `NewParams`). `handlePromptImprove` doesn't validate prompt length against `MaxPromptLength` (200KB). 13 tests in `handler_prompt_test.go`. `handlePromptABTest` has no test file |
| `handler_roadmap.go` | 384 | 5 | 4 | 4 | 3 | `ValidatePath` called at every handler entry. All error paths return `codedError`. 26 tests in `handler_roadmap_test.go`. `handler_roadmap_prioritize.go` has no test file |
| `handler_eval.go` | 579 | 4 | 4 | 4 | 3 | `abTestMinGroupSize` and `changepointBurnIn` constants provide clear semantic intent. `filterChangepointBurnIn` is a well-isolated helper. 20 tests in `handler_eval_test.go`. Missing `IdempotentHint` on all eval tools (all are read-only deterministic) |
| `handler_scratchpad.go` | 304 | 4 | 4 | 4 | 3 | `resolveRepoPath` helper centralizes scan/find/error logic across all scratchpad handlers. 14 tests in `handler_scratchpad_test.go` + `scratchpad_advanced_test.go`. Annotation: `scratchpad_append` and `scratchpad_resolve` have `DestructiveHint: false` but no `IdempotentHint` |
| `handler_team.go` | 348 | 4 | 4 | 2 | 3 | `ValidateRepoName` applied. Good dry-run mode. Only 5 tests in `handler_team_test.go` — the lowest coverage of sampled handlers. `handleTeamDelegate` and `handleAgentCompose` lack direct test cases |
| `handler_observation.go` | 217 | 4 | 4 | 2 | 3 | `parseTimeBound` helper is clean. `filterByUntil` is a pure function. 8 tests in `handler_observation_test.go`. No test for `handleObservationSummary` or `handleObservationCorrelate` |

**Summary of scoring:** No handler scores below 3 on any axis. The weakest area is **test coverage** — not from absent test files, but from shallow test counts (5–14 tests covering handlers that implement 3–8 different sub-operations each). Annotation completeness is uniformly 3/5: all tools have at least `ReadOnlyHint` or `DestructiveHint`, but no tool has all 4 hints simultaneously.

---

## 3. Test Coverage Gaps

### Known untested handler files (no `_test.go` counterpart)

| Handler File | Lines | Tools Covered | Risk Level |
|---|---|---|---|
| `handler_fleet_grafana.go` | 37 | `ralphglasses_fleet_grafana` | Low — pure transform (calls `fleet.ExportDashboard`, no state mutations) |
| `handler_prompt_ab.go` | 200 | `ralphglasses_prompt_ab_test` | Medium — complex scoring + file I/O (writes results to `.ralph/prompt_ab/`) |
| `handler_provider_benchmark.go` | 268 | `ralphglasses_provider_benchmark` | Medium — complex math (percentile calc, keyword scoring), no external deps |
| `handler_recommend.go` | 73 | `ralphglasses_cost_recommend` | Low — delegates to `fleet.NewRecommender`, thin wrapper |
| `handler_roadmap_prioritize.go` | 178 | `ralphglasses_roadmap_prioritize` | Medium — scoring algorithm with weights, no test for edge cases |
| `handler_session_fork.go` | 50 | `ralphglasses_session_fork` | Medium — new capability (fork sessions), calls `SessMgr.Fork` |
| `handler_skill_export.go` | 59 | `ralphglasses_skill_export` | Low — tested indirectly via `skill_export_test.go` (5 tests found) |
| `handler_sweep_report.go` | 302 | `ralphglasses_sweep_report` | High — 302 lines, complex Markdown/JSON rendering, no coverage at all |

**Additional under-tested handlers (test file exists but shallow):**
- `handler_team_test.go` — 5 tests for 6 tools (3 handlers have no direct test)
- `handler_observation_test.go` — 8 tests; `handleObservationSummary` and `handleObservationCorrelate` lack tests

**Well-tested handlers (for reference):**
- `handler_fleet_test.go` — 49 tests
- `handler_roadmap_test.go` — 26 tests
- `handler_eval_test.go` — 20 tests

---

## 4. Annotation Completeness

Analyzed from `~/hairglasses-studio/ralphglasses/internal/mcpserver/annotations.go`.

**Total annotated tools:** 166 (perfect sync with registered tool count — no gaps between `ToolAnnotations` map and builder registrations).

| Completeness Level | Count | % |
|---|---|---|
| All 4 hints (`ReadOnly` + `Destructive` + `Idempotent` + `OpenWorld`) | 0 | 0% |
| 2 hints (most common: `ReadOnly` + `OpenWorld`, or `Destructive` + `OpenWorld`) | 26 | 16% |
| 1 hint only | 138 | 83% |
| Title only (no behavioral hints) | 2 | 1% |

**Tools with no behavioral hints (title only):**
- `ralphglasses_fleet_workers` (line 90) — no hint at all; it can mutate worker state via `action` param
- `ralphglasses_worktree_create` (line 185) — creates a git worktree, should have `DestructiveHint: false, OpenWorldHint: true`

**Key annotation gaps:**
1. No tool has `IdempotentHint` paired with `ReadOnlyHint` — but many read-only tools (scan, list, status) are already idempotent and should declare it.
2. `ralphglasses_config` and `ralphglasses_config_bulk` set `IdempotentHint: true` but omit `ReadOnlyHint` — config-get is read-only, config-set is not; the dual get/set pattern makes annotation ambiguous.
3. The `session_stop_all` and `stop_all` tools correctly set `DestructiveHint: true` but omit `OpenWorldHint` — stopping external processes is an open-world side effect.
4. `ralphglasses_prompt_enhance` is annotated `ReadOnlyHint: true` but in `llm` or `auto` mode it calls external APIs — should have `OpenWorldHint: true` conditionally.

---

## 5. Middleware Chain Analysis

**Composition order** (outermost to innermost, from `cmd/mcp.go` lines 97–108):

```
ConcurrencyMiddleware(32)
  → TraceMiddleware()
    → TimeoutMiddleware(30s, with per-tool overrides)
      → InstrumentationMiddleware(toolRec)
        → EventBusMiddleware(bus)
          → ValidationMiddleware(scanPath)
            → handler
```

**What each layer does:**

| Layer | File | Purpose |
|---|---|---|
| `ConcurrencyMiddleware` | `middleware.go:29` | Weighted semaphore (default 32 slots); configurable via `RG_MCP_MAX_CONCURRENT`. Returns `ErrRateLimited` on context cancellation |
| `TraceMiddleware` | `middleware.go:60` | Generates/propagates trace IDs via `context`. Injects `_trace_id` into JSON responses |
| `TimeoutMiddleware` | `timeout.go` | Per-tool timeout overrides: `loop_step`=10m, `coverage_report`=5m, `merge_verify`=5m; `self_test` and `self_improve` are exempt (0 = no timeout) |
| `InstrumentationMiddleware` | `middleware.go:86` | Records latency, success/failure, input/output size to `ToolCallRecorder`. Emits structured `mcp.tool.call` log events |
| `EventBusMiddleware` | `middleware.go:179` | Publishes `tool.called` events to the event bus for every invocation |
| `ValidationMiddleware` | `middleware.go:205` | Validates `repo` and `path` parameters if present: `ValidateRepoName` for bare names, `ValidatePath` for absolute paths |

**Identified gaps:**

1. **No rate-per-tool limiting.** `ConcurrencyMiddleware` is a global semaphore. A single slow tool (e.g., `self_improve`) can consume all 32 slots. There is no per-tool or per-namespace concurrency cap.

2. **No panic recovery at middleware level.** The MCP SDK's `WithRecovery()` option is set at the server level (`cmd/mcp.go:95`), so panics are caught — but the recovery is external to the middleware chain and does not emit structured error events to the event bus.

3. **ValidationMiddleware only validates `repo` and `path` parameters.** Free-text fields like `prompt` (up to 200KB per `MaxPromptLength`) and `yaml` (workflow definitions) are not length-checked in the middleware layer. Each handler is responsible for its own length validation — and not all handlers call `ValidateStringLength`.

4. **`DeferredLoading` is never activated in production.** `Server.DeferredLoading` defaults to `false`. `cmd/mcp.go` calls `rg.Register(srv)` which falls into `RegisterAllTools`. The entire 166-tool surface is loaded eagerly. The documented deferred loading model is only exercised in tests. This means the startup cost savings described in CLAUDE.md are not realized in production.

5. **Tracing is only partially end-to-end.** `TraceMiddleware` now correlates with outgoing prompt-improver LLM spans and OTLP/Langfuse-ready export, but broader session-provider and fleet paths still lack the same child-span coverage.

---

## 6. Dispatch Table Completeness

The dispatch system uses a builder pattern (`ToolGroupRegistry` in `registry.go`) rather than a central dispatch table. Each `buildXGroup()` method returns `ToolEntry{Tool, Handler}` pairs, so handler-tool linkage is done at registration time rather than via a lookup map.

**Cross-check methodology:** extracted all `s.handleXxx` references from builder files and compared against all `func (s *Server) handleXxx` definitions in handler files.

**Result: All handlers referenced in builders are defined. No phantom references.**

However, 11 handlers referenced in builders are defined outside `handler_*.go` files:

| Handler | Defined In | Notes |
|---|---|---|
| `handleEventList` | `tools_fleet.go:16` | Mixed file: tools + handler in same file |
| `handleMarathonDashboard` | `tools_fleet.go:312` | Same pattern |
| `handleToolBenchmark` | `tools_fleet.go:442` | Same pattern |
| `handleWorkflowDefine` | `tools_session.go:74` | Mixed file |
| `handleWorkflowRun` | `tools_session.go:111` | Same pattern |
| `handleWorkflowDelete` | `tools_session.go:156` | Same pattern |
| `handleSnapshot` | `tools_session.go:186` | Same pattern |
| `handleJournalRead` | `tools_session.go:300` | Same pattern |
| `handleJournalWrite` | `tools_session.go:334` | Same pattern |
| `handleJournalPrune` | `tools_session.go:386` | Same pattern |
| `handleEventPoll` | `handler_rc.go:526` | RC handler file contains an event handler |

These are functional — the code compiles and runs — but the convention of `handler_*.go` for handlers is violated. `tools_fleet.go` and `tools_session.go` mix tool definitions (builders) with handler implementations, making grep-based auditing harder.

**3 handlers defined in dispatch/meta layer (not in builders, by design):**
- `handleToolGroups` (`tools_dispatch.go:126`) — meta-tool
- `handleLoadToolGroup` (`tools_dispatch.go:154`) — meta-tool
- `handleSkillExport` (`handler_skill_export.go`) — registered in dispatch, not builder

---

## 7. Namespace Organization Assessment

### Oversized namespaces (>15 tools)

| Namespace | Tool Count | Assessment |
|---|---|---|
| **advanced** | 24 | Too broad. Contains RC tools (4), events (2), HITL (2), autonomy (4), feedback (3), provider selection (2), journals (3), workflows (3), bandit (1), confidence (1), circuit (1). At least 3 coherent sub-namespaces could be extracted: `rc`, `autonomy`, `workflow` |
| **rdcycle** | 18 | Reasonable given R&D cycle is a coherent domain, but contains two distinct sub-systems: the classic rdcycle tools (finding_to_task, cycle_baseline, cycle_plan, cycle_merge, cycle_schedule, loop_replay, budget_forecast, diff_review, finding_reason, observation_correlate) and the new cycle state machine (cycle_create, cycle_advance, cycle_status, cycle_fail, cycle_list, cycle_synthesize, cycle_run, provider_benchmark). The state machine tools deserve their own namespace |
| **observability** | 17 | Reasonable — scratchpad (8), observations (3), loop_wait/poll (2), coverage (1), cost_estimate (1), merge_verify (1), worktree (2). Worktree tools (`worktree_create`, `worktree_cleanup`) are misplaced here — they are development workflow tools, not observability tools |
| **session** | 16 | Borderline. Contains session lifecycle (launch, list, status, resume, stop, stop_all, budget, retry) + output tools (output, tail, diff, compare, errors, export) + session_fork + session_replay_diff + session_handoff. The handoff tool might fit better in `loop` or `team` |

### Misplaced tools

| Tool | Current Namespace | Should Be In | Reason |
|---|---|---|---|
| `ralphglasses_session_handoff` | `loop` (in `tools_builders_session.go`) | `session` | It is a session lifecycle operation, not a loop concept |
| `ralphglasses_worktree_create` | `observability` | New `dev` namespace or `repo` | Worktrees are development workflow tools, not observability |
| `ralphglasses_worktree_cleanup` | `observability` | Same | Same reason |
| `ralphglasses_loop_await` | `observability` | `loop` | It awaits a loop; belongs with loop controls |
| `ralphglasses_loop_poll` | `observability` | `loop` | Same — these are loop polling tools, not observation tools |
| `ralphglasses_event_list` | `advanced` | New `events` namespace or `observability` | Events are an observability concern, not an "advanced" one |
| `ralphglasses_event_poll` | `advanced` | Same | Same reason |

### Undersized namespaces (4–5 tools)

| Namespace | Tool Count | Assessment |
|---|---|---|
| `plugin` | 4 | Appropriate — plugin management is a small, distinct domain |
| `awesome` | 5 | Appropriate — self-contained research workflow |
| `fleet_h` | 5 | Reasonable, though `cost_recommend` and `cost_forecast` could merge with `fleet` |
| `repo` | 5 | Appropriate — repo ops are bounded |

### `ToolGroupNames` vs `defaultRegistry()` consistency

`ToolGroupNames` (in `tools.go:127`) lists 16 namespaces: `core, session, loop, prompt, fleet, repo, roadmap, team, awesome, advanced, eval, fleet_h, observability, rdcycle, plugin, sweep`.

`defaultRegistry()` (in `tools_builders.go:10–29`) registers 16 builders in the same order.

**These are in sync.** No namespace is in one list but not the other.

However, `handleLoadToolGroup` in `tools_dispatch.go:56` hardcodes the description string as `"(core, session, loop, prompt, fleet, repo, roadmap, team, awesome, advanced, eval, fleet_h, observability)"` — missing `rdcycle`, `plugin`, and `sweep`. This description is stale and would mislead users calling the meta-tool.

---

## Key Findings Summary

| Finding | Severity | File(s) | Action |
|---|---|---|---|
| Deferred loading not activated in production binary | Medium | `cmd/mcp.go:120`, `tools.go:58` | Set `rg.DeferredLoading = true` before `rg.Register(srv)` to realize startup cost savings |
| `handleLoadToolGroup` description string missing 3 namespaces | Low | `tools_dispatch.go:55–57` | Update description to include `rdcycle`, `plugin`, `sweep` |
| 8 handler files without dedicated test files | Medium | `handler_sweep_report.go`, `handler_prompt_ab.go`, `handler_provider_benchmark.go`, `handler_roadmap_prioritize.go`, `handler_session_fork.go`, `handler_fleet_grafana.go`, `handler_recommend.go`, `handler_skill_export.go` | `handler_sweep_report.go` (302 lines) is highest risk — add tests first |
| Zero tools have all 4 MCP spec annotations | Low | `annotations.go` | Add `IdempotentHint` to pure read-only tools; add `OpenWorldHint` to tools that call external APIs or spawn processes |
| 11 handler functions defined in non-`handler_*.go` files | Low | `tools_fleet.go`, `tools_session.go`, `handler_rc.go` | Move handlers to dedicated files or add comment convention to distinguish builder-only vs builder+handler files |
| `advanced` namespace too broad (24 tools, 7+ distinct domains) | Medium | `tools_builders_misc.go:154–288` | Extract `rc` (4 tools), `autonomy` (4 tools), `workflow` (3 tools) into their own namespaces |
| `worktree_create/cleanup` misplaced in `observability` | Low | `tools_session.go:583–597` | Move to `repo` namespace or new `dev` namespace |
| `loop_await` and `loop_poll` misplaced in `observability` | Low | `tools_session.go:538–551` | Move to `loop` namespace |
| `prompt_improve` doesn't validate prompt length | Low | `handler_prompt.go` | Add `ValidateStringLength(prompt, MaxPromptLength, "prompt")` |
| `ValidationMiddleware` does not cover free-text fields | Low | `middleware.go:205` | Consider length validation for `prompt`, `yaml`, `content` in middleware |
