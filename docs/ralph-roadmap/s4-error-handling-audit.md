# S4 Error Handling Audit — ralphglasses

**Date**: 2026-04-04
**Auditor**: Claude Sonnet 4.6 (automated static analysis)
**Scope**: `internal/` — all non-test Go source files
**Method**: grep-based pattern analysis + targeted file reads

---

## 1. Handler Contract Compliance

The mcpkit convention requires MCP handlers to return `(*mcp.CallToolResult, nil)` — never `(nil, error)`. ralphglasses uses a local `codedError()` helper instead of mcpkit's `handler.CodedErrorResult`, but the semantics are equivalent.

**No handler-level contract violations were found.**

The search turned up four files with `return nil, ...` patterns in handler files, but all four are helper/private functions, not MCP handlers:

| File | False Alarm Reason |
|------|--------------------|
| `handler_sweep.go:674,680` | `resolveSweepRepos()` returns `([]*repoRef, error)` |
| `handler_rc.go:44,53` | `resolveTarget()` returns `(*session.Session, *mcp.CallToolResult)` |
| `handler_loopwait.go:111,145` | `checkAwaitStatus()` returns `(*mcp.CallToolResult, bool)` |
| `handler_harness_test.go:215` | test helper, not production |

**Handler compliance table** (sampled; all 44 handlers audited):

| Handler File | Return-nil-err Violations | codedError / errResult Uses | Grade |
|---|---|---|---|
| handler_session_lifecycle.go | 0 | 26 | A |
| handler_rc.go | 0 (helper fn) | 28 | A |
| handler_rdcycle.go | 0 | 33 | A |
| handler_sweep.go | 0 (helper fn) | 17 | A |
| handler_repo.go | 0 | 37 | A |
| handler_team.go | 0 | 38 | A |
| handler_fleet.go | 0 | 31 | A |
| handler_eval.go | 0 | 31 | A |
| handler_scratchpad.go | 0 | 30 | A |
| handler_loop.go | 0 | 16 | A |
| handler_prompt.go | 0 | 16 | A |
| handler_observation.go | 0 | 12 | A |
| handler_session.go | 0 | 21 | A |
| handler_provider_benchmark.go | 0 | 0 (no bad params) | B |
| handler_recommend.go | 0 | 0 (no bad params) | B |
| handler_fleet_grafana.go | 0 | 1 | B |

Two handlers (`handler_provider_benchmark.go` and `handler_recommend.go`) have no codedError calls, but both are read-only/analysis tools that degrade gracefully (return empty/not-configured results), so this is acceptable rather than a violation.

---

## 2. Panic Inventory

Only **2 panic calls** exist in production code outside tests:

| File | Line | Code | Reachability | Risk |
|------|------|------|--------------|------|
| `internal/model/costs.go` | 70 | `panic("inputRatio must be in [0, 1]")` | Reachable from any code calling `BlendedCostPer1MTok()` with an out-of-range ratio | Low — caller contract is clear; callers in codebase all pass constants. Recoverable. |
| `internal/safety/circuit_breaker.go` | 142 | `panic(err)` in `MustNew()` | Only reachable from package-init-style call sites | Low — `MustNew` is an explicit "panic on bad config" constructor, self-documenting. |

The `internal/review/criteria.go:186` line contains the string `"panic() in library code..."` as part of a lint rule message — it is not an executable panic.

**Recovery wrappers** that catch these panics:
- `internal/session/loop_steps.go:313-315`: goroutine-level `recover()` converts panics in worker goroutines to `workerResult{err: ...}` — correct.
- `internal/fleet/server_handlers.go:397-405`: `recover()` in a health-check closure, logs and marks `healthy=false` — acceptable defensive use.
- `internal/session/loop_steps.go` recovery does NOT re-panic after logging, which is the right choice for worker isolation.

No unrecovered panics reachable from the hot path (session launch, loop execution, MCP handler dispatch).

---

## 3. Swallowed Errors — Top 20

Ranked by autonomy-safety impact (highest risk first):

| # | File | Line | Pattern | Risk |
|---|------|------|---------|------|
| 1 | `internal/mcpserver/handler_selfimprove.go` | 71 | `_ = s.SessMgr.RunLoop(context.Background(), run.ID)` | **High** — self-improvement loop error is silently discarded. If the loop fails immediately (e.g., repo not found, OOM), the handler returns success. At L2+ autonomy this silences feedback that the agent needs to self-correct. |
| 2 | `internal/hooks/hooks.go` | 139 | `_ = cmd.Run()` | **High** — hook command exit codes are silently discarded. A failing pre/post-session hook is invisible to the orchestrator. |
| 3 | `internal/mcpserver/handler_sweep.go` | 453 | `_ = s.SessMgr.Stop(sessID)` | **Medium-High** — sweep restart stops a session but ignores stop failure. A zombie session may persist consuming budget. |
| 4 | `internal/mcpserver/handler_sweep.go` | 602 | `_ = s.SessMgr.Stop(sid)` | **Medium-High** — same as above in the sweep-schedule nudge path. |
| 5 | `internal/session/manager_lifecycle.go` | 423 | `slog.Warn("store save failed, falling back to JSON")` | **Medium** — SQLite save failure is warn-logged, falls back to JSON. If JSON write also fails (line 443 returns error), the session state is lost on restart. Double-write makes the fallback opaque. |
| 6 | `internal/mcpserver/handler_provider_benchmark.go` | 190,201 | `os.MkdirAll(...)` / `os.WriteFile(...)` | **Medium** — benchmark results silently not persisted. Operator sees no error, assumes data was saved. |
| 7 | `internal/mcpserver/handler_prompt_ab.go` | 177,179 | Same pattern | **Medium** — A/B test results silently lost. |
| 8 | `internal/mcpserver/handler_session_handoff.go` | 165,167 | Same pattern | **Medium** — handoff records not persisted; replay/audit trail broken. |
| 9 | `internal/fleet/worker.go` | 101 | `_ = w.client.Heartbeat(...)` | **Medium** — heartbeat failures are silent. Coordinator marks worker as stale after `StaleThreshold` (90s), but there is no warning logged. Worker continues operating as if healthy. |
| 10 | `internal/fleet/worker.go` | 155,183,197 | `_ = w.client.CompleteWork(...)` | **Medium** — work completion reports silently discarded on network error. Coordinator retains work item indefinitely, causing duplicate execution on next poll. |
| 11 | `internal/fleet/worker.go` | 250 | `_ = w.client.SendEvents(...)` | **Low-Medium** — event forwarding loss; observability gap but not correctness. |
| 12 | `internal/k8s/controller.go` | 217 | `_ = r.client.UpdateSessionStatus(ctx, session)` | **Medium** — K8s status update discarded after pod creation failure. Stale status in API server. |
| 13 | `internal/events/bus.go` | 300 | `_ = b.transport.Publish(ctx, event)` | **Medium** — NATS publish failures silent. Events lost with no retry. |
| 14 | `internal/events/bus.go` | 350 | `_ = b.PublishCtx(context.Background(), event)` | **Medium** — fire-and-forget publish in the synchronous `Publish()` wrapper. No backpressure. |
| 15 | `internal/discovery/cache.go` | 97 | `_ = sc.persistToDisk(cf)` | **Low** — cache not persisted, next start cold. Not a correctness issue. |
| 16 | `internal/mcpserver/handler_costestimate.go` | 179 | `_ = s.scan()` | **Low** — best-effort scan; if it fails, handler falls back to no-repo mode gracefully. |
| 17 | `internal/mcpserver/handler_prompt.go` | 284 | `_ = s.scan()` | **Low** — same as above. |
| 18 | `internal/mcpserver/handler_scratchpad.go` | 37 | `_ = s.scan()` | **Low** — same; handled by fallback logic. |
| 19 | `internal/blackboard/blackboard.go` | 257 | `_, _ = f.Write(data)` | **Low** — blackboard write failure silent; next Put() will overwrite with correct data. |
| 20 | `internal/session/manager_lifecycle.go` | 598 | `slog.Warn("failed to remove session state file")` | **Low** — cleanup failure logged, not propagated; stale files accumulate. |

---

## 4. Error Wrapping

Total `fmt.Errorf` calls in production code: **1,501**
With `%w` (unwrap-compatible): **981** (65%)
Without `%w` (bare string or `%v`): **520** (35%)

Bare `%v` specifically (true chain-breakers): **9** (0.6%)

**Per-package breakdown:**

| Package | Total Errorf | With %w | Ratio |
|---------|-------------|---------|-------|
| session | 383 | 236 | 62% |
| store | 46 | 43 | 93% |
| fleet | 89 | 60 | 67% |
| batch | 70 | 50 | 71% |
| enhancer | 26 | 18 | 69% |
| marathon | 28 | 17 | 61% |
| mcpserver | 40 | 10 | 25% |
| safety | ~4 | 1 | ~25% |

The `mcpserver` package has the worst `%w` ratio at **25%**. This is partly deliberate — many errors there are formatted into `codedError()` strings where `%w` is irrelevant (the error is not passed to callers). However, `validate.go` and `handler_mergeverify.go` use bare string formatting in errors that are returned to callers, losing chain information.

The bare `%v` (true chain-breakers) are mostly in internal helpers and low-risk paths. The most notable is `internal/session/loop_steps.go:315` — the goroutine panic capture — where `%v` is appropriate since the panic value `r` is `any`, not `error`.

The two direct `== context.XXX` comparisons (instead of `errors.Is`) in `hooks/chain.go:152` and `sandbox/limits.go:140` could silently miss wrapped context errors from middleware.

---

## 5. Context Cancellation Gaps

Overall context handling is **good**. The loop engine, session manager, fleet coordinator, and worker all check `ctx.Err()` at iteration boundaries.

**Gaps identified:**

| Location | Issue |
|----------|-------|
| `handler_sweep.go:247` / `handler_sweep.go:517` | `context.WithCancel(context.Background())` detaches sweep from handler context. Intentional (sweep outlives request), but if `Tasks.Create` cancels the context, the goroutine loop does not propagate that signal to already-launched sessions — they run to budget. |
| `handler_loop.go:152` / `handler_selfimprove.go:71` | `go s.SessMgr.RunLoop(context.Background(), run.ID)` — loop runs under `context.Background()`, so MCP request cancellation does not abort the loop. Again intentional (loop outlives request), but the loop has no shutdown hook tied to server shutdown. |
| `handler_rc.go:428` / `handler_session_lifecycle.go:119` / `handler_session_handoff.go:144` | `s.SessMgr.Launch(context.Background(), opts)` — launches with background context, severing cancellation. Intentional for long-running sessions, but means graceful shutdown requires polling `.Stop()` rather than context propagation. |
| `handler_fleet.go:101,143,219` | `s.FleetClient.SubmitWork(context.Background(), ...)` / `FleetState(context.Background())` — fleet client calls use background context. If the fleet server hangs, these calls block indefinitely rather than respecting the MCP request timeout. |
| `internal/session/supervisor.go` tick goroutines | `RunCycle()` goroutines launched at lines 230 and 243 capture `ctx` from the supervisor's `run()` loop. If `tick()` is called when `ctx` is already cancelled (close race), cycles start and immediately see a cancelled context. This is a benign race but could produce confusing "cycle failed" log entries at shutdown. |
| `internal/marathon/cloud_scheduler.go:320` | Checks `ctx.Err()` in the outer scheduler loop but the inner `runTask()` call (which makes cloud API requests) receives the same context without an independent timeout. A hung cloud API blocks the scheduler indefinitely. |

---

## 6. Silent Failure Patterns

Places where errors are **logged with Warn/Info but not propagated**, creating potential silent failures at L2/L3 autonomy:

| Location | Pattern | Autonomy Risk |
|----------|---------|---------------|
| `session/supervisor.go:375` | `slog.Warn("supervisor: RunCycle failed")` then continues | **Critical** — at L2+ the supervisor silently skips failed cycles. No backoff, no escalation, no HITL trigger. Cycle failures accumulate invisibly. |
| `session/supervisor.go:231` | `slog.Warn("supervisor: chained cycle failed")` | **High** — chained R&D cycles silently fail; roadmap progress stalls without alert. |
| `session/supervisor.go:244` | `slog.Warn("supervisor: planned sprint failed")` | **High** — same issue for sprint-planner path. |
| `session/manager_cycle.go:333` | `slog.Warn("RunCycle: task launch failed")` then `continue` | **High** — task launch failures are best-effort within RunCycle. A partially-launched cycle proceeds to wait and observe, producing misleading observations. |
| `session/manager.go:177` | `slog.Warn("failed to rehydrate sessions from store")` | **Medium** — on restart, if SQLite is corrupt, all previous session history is silently lost. Autonomy decisions lose their memory. |
| `marathon/marathon.go:335` | `slog.Warn("marathon: checkpoint save failed")` | **Medium** — marathon state not persisted. On crash, marathon restarts from scratch, losing cycle count and spend tracking. |
| `marathon/marathon.go:130` | `slog.Warn("marathon: resume failed, starting fresh")` | **Medium** — resume failures are silently treated as fresh starts; no operator alert. |
| `session/loop_steps.go:697` | `slog.Warn("store record cost failed")` | **Medium** — cost records not stored; budget tracking becomes inaccurate over time, potentially allowing over-spend. |
| `session/loop_steps.go:438,510,713` | `slog.Warn("failed to write loop journal")` (3 sites) | **Low-Medium** — journal write failures silent; loop replay and debugging broken. |
| `session/loop_acceptance.go:33` | `slog.Warn("acceptance: failed to get diff paths")` then `continue` | **Medium** — if diff path detection fails for a worktree, that worktree's changes are silently skipped in acceptance. Could cause partial merges. |
| `process/manager_lifecycle.go:41` | `slog.Warn("failed to open log file, process output will be lost")` | **Low** — session output lost on disk-full; process still starts. |
| `session/autonomy.go:191` | `slog.Warn("failed to persist autonomy level")` | **Medium** — autonomy level changes not persisted; L2/L3 resets to default on restart. |

---

## 7. Recovery Misuse

Three `recover()` call sites identified:

| File | Line | Assessment |
|------|------|------------|
| `session/loop_steps.go:313-315` | Wraps goroutine worker in recover, converts panic to `workerResult{err: ...}` | **Correct** — isolates individual worker panics, continues other workers. Error surfaces in loop output. |
| `fleet/server_handlers.go:397-405` | Wraps `queue.Counts()` call in recover, logs as `checks["queue"] = "error"` | **Acceptable** — defensive health check. Queue.Counts() shouldn't panic, but the protection prevents health endpoint crashes. Does not mask bugs; the `queue_error` field captures the panic value. |
| `tui/view_adapters_test.go:84,104` | Test-only | N/A |
| `notify/notify_test.go:173` | Test-only | N/A |
| `model/costs_test.go:496` | Test-only, expects panic | N/A |
| `safety/circuit_breaker_test.go:57` | Test-only, expects panic | N/A |

**No production recover() calls mask bugs.** Both production recovery sites convert panics into structured error values that surface through normal result channels.

The `internal/session/loop_steps.go` recovery notably does NOT re-panic, which is appropriate here — re-panicking would crash the entire process, not just the worker goroutine.

---

## 8. Package Grades

| Package | Grade | Rationale |
|---------|-------|-----------|
| `mcpserver` | **B+** | Zero handler contract violations, rich codedError taxonomy. Weak `%w` wrapping (25%) and several `_ = s.scan()` swallows are acceptable (they have fallback logic). `context.Background()` use in session launches is intentional. |
| `session` | **B** | Strong context cancellation throughout loops and workers. `%w` wrapping at 62% is adequate. Main weakness: supervisor silently swallows cycle failures (critical for L2/L3). Autonomy level not persisted on failure. |
| `fleet` | **B-** | Worker heartbeat and `CompleteWork` errors silently discarded. Fleet client calls use `context.Background()` (no request timeout). Queue health recovered defensively. |
| `marathon` | **C+** | Checkpoint save failures silent, resume failures treated as silent fresh-starts. No alerting path for marathon-level failures. Cloud scheduler lacks per-task timeout. |
| `safety` | **A** | `MustNew` panic is documented and justified. Circuit breaker wraps errors cleanly. `%w` used consistently. |
| `store` | **A-** | 93% `%w` wrapping — best in codebase. Clean error propagation throughout SQLite layer. |
| `enhancer` | **B+** | 69% `%w` wrapping. LLM client errors properly propagated. Backoff error wrapping clean. |
| `batch` | **B+** | 71% `%w` wrapping. API error messages include status codes. Some bare `%v` in status-code-only paths. |
| `events` | **C+** | Bus `Publish()` silently discards transport errors. NATS message ack/nak errors silently discarded. Fire-and-forget publish is architecturally intentional but creates observability gaps. |
| `hooks` | **C** | `_ = cmd.Run()` discards hook exit codes entirely. Post-session hooks cannot signal failure back to orchestrator. |
| `process` | **B** | PID file write failure is logged (non-fatal). Log file open failure is logged and process continues. Good context cancellation in reaper. |
| `workflow` | **B+** | Executor checks `ctx.Err()` at step boundaries. Engine propagates errors cleanly. |
| `k8s` | **C+** | `UpdateSessionStatus` after pod creation failure is silently discarded. Stale API state. |

---

## 9. Fix Priority List

Ordered by autonomy-safety impact (most critical first):

| # | Fix | File(s) | Impact |
|---|-----|---------|--------|
| 1 | **Surface supervisor cycle failures** — Log `RunCycle` failures at Error level, emit an event, and after N consecutive failures trigger a HITL interrupt or autonomy level demotion. | `session/supervisor.go:375` | Prevents L2/L3 from silently spinning on broken R&D cycles. |
| 2 | **Propagate RunLoop error in background goroutines** — Send the error to a channel or use `mcplog.Error` so it appears in the tool response or session journal. Currently `_ = s.SessMgr.RunLoop(context.Background(), run.ID)` buries the error completely. | `handler_selfimprove.go:71`, `handler_loop.go:152` | At L2 autonomy, a self-improvement loop that starts and immediately fails is invisible. |
| 3 | **Log or retry fleet worker report failures** — `CompleteWork` network failures cause the coordinator to retain work items indefinitely, leading to duplicate execution. Add retry with exponential backoff or at minimum log at Error level. | `fleet/worker.go:155,183,197` | Prevents duplicate work execution and wasted budget in fleet mode. |
| 4 | **Persist autonomy level changes** — Wrap persist failure in a returned error, or retry with backoff. Autonomy level resetting to L0 on restart silently reduces agent capability and breaks operator intent. | `session/autonomy.go:191` | Autonomy level stability is a safety property. |
| 5 | **Fix silent supervisor task-launch continuation** — After `LaunchCycleTask` fails, log the failure at Error level and mark the cycle task as failed rather than skipping silently. Current `continue` produces partially-executed cycles with misleading observations. | `session/manager_cycle.go:333` | Partial cycle execution produces false-positive observations that corrupt the R&D signal. |
| 6 | **Add hook exit code handling** — Surface non-zero hook exit codes to the caller (at minimum log at Error level). Hooks are used for pre/post session gates; silent failures break the gate contract. | `hooks/hooks.go:139` | Hook-based safety gates are invisible failures. |
| 7 | **Add timeout to fleet client calls** — Replace `context.Background()` with `context.WithTimeout(ctx, 30*time.Second)` for `SubmitWork` and `FleetState`. A hung coordinator blocks the handler indefinitely. | `handler_fleet.go:101,143,219` | Prevents fleet handlers from hanging under network partition. |
| 8 | **Wrap `%w` in mcpserver validate.go** — `validate.go` returns leaf errors without `%w`, preventing `errors.Is` checks on validation errors by callers. | `mcpserver/validate.go` | Low-risk but improves debuggability for automated error classification. |
| 9 | **Use `errors.Is` for context comparisons** — Replace direct `== context.DeadlineExceeded` with `errors.Is(err, context.DeadlineExceeded)` to handle wrapped contexts. | `hooks/chain.go:152`, `sandbox/limits.go:140` | Correctness under middleware wrapping. |
| 10 | **Surface marathon checkpoint failure** — After `SaveCheckpoint` fails, emit an event or set a health flag. Currently marathon can run indefinitely without durable state, losing budget accounting on crash. | `marathon/marathon.go:335` | Budget overrun risk in long-running marathon sessions. |
| 11 | **Make `os.MkdirAll` + `os.WriteFile` errors visible in handlers** — The pattern appears in 3 handlers (benchmark, A/B test, handoff). Log at Warn and include `"persist_failed": true` in the response so callers know the record was not saved. | `handler_provider_benchmark.go:190,201`, `handler_prompt_ab.go:177,179`, `handler_session_handoff.go:165,167` | Audit trail gaps; operator assumes data was persisted. |
| 12 | **Add per-task timeout to marathon cloud scheduler** — Wrap `runTask()` in `context.WithTimeout` independent of the scheduler's outer context. Cloud API hangs block the entire scheduler. | `marathon/cloud_scheduler.go` | Liveness for cloud-hosted agent workers. |
| 13 | **Log sweep Stop failures** — `_ = s.SessMgr.Stop(sessID)` in sweep restart/nudge paths should log failures. A zombie session consuming budget is hard to detect. | `handler_sweep.go:453,602` | Budget waste in large sweeps. |
| 14 | **Add context-aware NATS publish retry** — `events/bus.go` silently discards transport publish errors. Add at minimum a counter metric and a log at Warn level so event loss is observable. | `events/bus.go:300,350` | Observability gap; events are the primary signal for the supervisor. |
| 15 | **Surface `rehydrate` failures** — If `RehydrateFromStore` fails, current code logs at Warn and returns. Consider returning the error to startup so the operator can decide whether to abort. Silently losing session history on startup breaks L2 memory continuity. | `session/manager.go:177` | On bad SQLite state, entire session history is silently dropped at startup. |

---

## Summary Statistics

- **Handler contract violations**: 0 confirmed (all `nil, ...` patterns are in helper functions with non-handler signatures)
- **Production panics**: 2 (both justified, both recoverable)
- **Swallowed errors (top 20)**: 1 critical (RunLoop), 4 high, 10 medium, 5 low
- **Error wrapping ratio**: 65% globally (`%w`); worst: mcpserver 25%; best: store 93%
- **Context cancellation**: Generally good in hot path; gaps in fleet client calls and background goroutines (intentional by design, but fleet calls need timeouts)
- **Recovery misuse**: None — both production `recover()` sites are correct
- **Silent supervisor failures**: The most systemic risk — cycle/sprint failures are warn-logged and swallowed at all levels of the supervisor tick loop, which directly undermines L2/L3 autonomy safety
