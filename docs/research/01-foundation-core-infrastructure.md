# Phase Research: Foundation & Core Infrastructure
> Research Agent: research-01-foundation
> Generated: 2026-03-22
> Scope: Go module structure, discovery engine, model layer, process management
> Relevant ROADMAP Phases: 0, 0.5

## 1. Executive Summary

- **Phase 0 is structurally complete** with a functional discovery engine, model layer, process manager, file watcher, and log streamer. The codebase is well-tested (78.2% coverage claimed) with fuzz tests, benchmarks, and thorough unit test coverage across all foundation packages.
- **Phase 0.5 is partially addressed in code but not marked done in ROADMAP.** `RefreshRepo` already returns `[]error` (0.5.1.1 done), watcher already emits `WatcherErrorMsg` and falls back to polling on timeout (0.5.2 partially done), process reaper already captures exit codes and sends `ProcessErrorMsg` (0.5.3 partially done), and version ldflags are wired through `cmd/root.go` (0.5.7 partially done). At least 7 of 11 ROADMAP 0.5.x sections have partial implementation that needs verification and checkbox updates.
- **The watcher's 2-second idle timeout is aggressive** -- it creates and destroys a new `fsnotify.Watcher` on every cycle, which is wasteful and may miss events during teardown. This is the highest-impact reliability gap in the foundation layer.
- **Process group management is correctly implemented** with `Setpgid: true` and negative-PGID signaling via `sendSignal()`, but lacks a fallback kill escalation (SIGTERM -> wait -> SIGKILL) and orphan audit, as ROADMAP 0.5.4 identifies.
- **Config validation is absent** -- `.ralphrc` parsing silently accepts any key/value pair with no schema, type checking, or typo detection, exactly as ROADMAP 0.5.11 describes.

## 2. Current State Analysis

### 2.1 What Exists

| File | Lines | Test File | Test Lines | Status |
|------|-------|-----------|------------|--------|
| `internal/discovery/scanner.go` | 56 | `scanner_test.go` | 232 | Complete, well-tested |
| `internal/model/repo.go` | 116 | `repo_test.go` | 204 | Complete, 6 display methods tested |
| `internal/model/status.go` | 79 | `status_test.go` | 307 | Complete, fuzz tests present |
| `internal/model/config.go` | 81 | `config_test.go` | 270 | Complete, round-trip save test |
| `internal/model/benchmark.go` | 188 | `benchmark_test.go` | 163 | Complete, summary generation |
| `internal/model/breakglass.go` | 127 | `breakglass_test.go` | exists | Circuit breaker config model |
| `internal/model/identity.go` | 152 | `identity_test.go` | exists | Agent identity for parallel runs |
| `internal/process/manager.go` | 356 | `manager_test.go` | 499 | Complete, lifecycle + PID + recovery |
| `internal/process/watcher.go` | 77 | `watcher_test.go` | 303 | Complete, 9 test cases |
| `internal/process/logstream.go` | 64 | `logstream_test.go` | 164 | Complete, tail + full read |
| `cmd/root.go` | 129 | `cmd_test.go` | 175 | Complete, flag + version tests |
| `main.go` | 7 | -- | -- | Minimal entry point |
| `internal/events/bus.go` | 193 | -- | -- | Event bus with ring buffer |

**Total source lines (foundation scope):** 1,625 production, 2,317 test

### 2.2 What Works Well

1. **Discovery engine** (`internal/discovery/scanner.go:13-46`): Clean single-level scan with sorted output, correct separation of `dirExists`/`fileExists` helpers, and automatic `RefreshRepo` call during discovery. 8 test cases cover all edge cases including empty dirs, missing roots, and RC-only repos.

2. **Model layer type definitions** (`internal/model/repo.go:9-70`): Rich struct hierarchy -- `Repo` aggregates `LoopStatus`, `CircuitBreakerState`, `Progress`, and `RalphConfig`. Display methods (`StatusDisplay`, `CircuitDisplay`, `CallsDisplay`, `UpdatedDisplay`) follow a null-safe pattern returning `"-"` for nil pointers.

3. **RefreshRepo error handling** (`internal/model/status.go:53-78`): Already returns `[]error` and stores them on `r.RefreshErrors`. Missing files (`os.ErrNotExist`) are correctly excluded from error results. This addresses ROADMAP 0.5.1.1 -- the code signature already matches the target.

4. **Process manager lifecycle** (`internal/process/manager.go:148-224`): Correct use of `Setpgid: true` at line 163, process-group signaling via `sendSignal()` at lines 227-233, background reaper goroutine with `cmd.Wait()` that records exit status, cleans PID files, publishes events, and sends `ProcessErrorMsg` on non-zero exit.

5. **PID file recovery** (`internal/process/manager.go:119-145`): `Recover()` re-adopts live processes from PID files and cleans stale ones. `CleanStalePIDFiles()` is a standalone function for startup hygiene.

6. **Event bus** (`internal/events/bus.go:50-178`): Ring buffer with configurable max history, cursor-based polling via `HistoryAfterCursor`, non-blocking fan-out to subscribers. Well-designed for both TUI and MCP consumers.

7. **Test quality**: Fuzz tests for all three JSON parsers (`status_fuzz_test.go`), benchmarks for `LoadConfig`, `LoadStatus`, and `RefreshRepo` (`bench_test.go`), and comprehensive lifecycle tests for the process manager including zombie prevention and error channel verification.

### 2.3 What Doesn't Work

1. **Watcher creates/destroys on every cycle** (`internal/process/watcher.go:26-77`): Each `WatchStatusFiles` call creates a new `fsnotify.Watcher`, adds paths, blocks for one event or 2s timeout, then closes. This means a new inotify instance every 2 seconds. On Linux with many repos, this risks hitting `fs.inotify.max_user_instances`. Cross-ref: ROADMAP 0.5.2.

2. **No exponential backoff on watcher failure** (`internal/process/watcher.go:72-74`): On idle timeout, the error message says "falling back to polling" but the actual fallback is just the TUI re-issuing the same `WatchStatusFiles` command, creating the same watcher again. Cross-ref: ROADMAP 0.5.2.4.

3. **No escalation kill sequence** (`internal/process/manager.go:236-254`): `Stop()` sends a single SIGTERM via process group and returns immediately. No wait-and-escalate to SIGKILL. Recovered processes are cleaned from the map but the actual process may survive SIGTERM. Cross-ref: ROADMAP 0.5.4.3.

4. **No orphan detection** (`internal/process/manager.go`): After `StopAll()`, no verification that child processes actually terminated. No `ralph_loop` pattern scan. Cross-ref: ROADMAP 0.5.4.4.

5. **Config accepts any key without validation** (`internal/model/config.go:34-46`): Parser accepts arbitrary keys -- no schema, no type checking, no warning on unknown keys. The `validKey` regex at line 12 is only enforced on `Save()`, not `Load()`. Cross-ref: ROADMAP 0.5.11.

6. **Hardcoded version in MCP server** (`cmd/mcp.go:30`): The MCP server constructs its own version string rather than using the `version` variable from `cmd/root.go`. Cross-ref: ROADMAP 0.5.7.2.

7. **Config Save() writes keys in random map iteration order** (`internal/model/config.go:75-79`): `for k, v := range c.Values` iterates in non-deterministic order, causing `.ralphrc` file diffs to be noisy.

8. **No context.Context threading** (`internal/discovery/scanner.go:13`): `Scan()` takes `root string` only. A long scan of a slow filesystem cannot be cancelled. Cross-ref: ROADMAP 1.9.1.

9. **Log path mismatch**: `logstream.go:21` reads from `.ralph/logs/ralph.log` but ROADMAP Phase 0 bullet says "Log streamer -- tail `.ralph/live.log`". The code is correct (uses `ralph.log`), but the ROADMAP description is stale.

## 3. Gap Analysis

### 3.1 ROADMAP Target vs Current State

| ROADMAP Item | Description | Status | Evidence |
|-------------|-------------|--------|----------|
| 0.5.1.1 | `RefreshRepo` returns `[]error` | **Done** | `status.go:53` signature already returns `[]error` |
| 0.5.1.2 | Propagate errors to TUI as `RefreshErrorMsg` | **Not done** | No `RefreshErrorMsg` type found in codebase |
| 0.5.1.3 | Display parse errors in repo detail view | **Not done** | TUI does not render `RefreshErrors` |
| 0.5.1.4 | Unit test: corrupt status returns error | **Done** | `status_test.go:256-284` `TestRefreshRepo_CorruptStatus_ReturnsError` |
| 0.5.2.1 | Replace `return nil` on watcher error | **Done** | Watcher returns `WatcherErrorMsg` on all error paths |
| 0.5.2.2 | Emit `WatcherErrorMsg` to TUI | **Done** | `watcher.go:29,69,71,74` all return `WatcherErrorMsg` |
| 0.5.2.3 | Auto-fallback to polling on error | **Partial** | TUI handles `WatcherErrorMsg` by re-issuing tick, but no user notification |
| 0.5.2.4 | Exponential backoff on repeated failures | **Not done** | No backoff logic anywhere in watcher |
| 0.5.3.1 | Capture `cmd.Wait()` error | **Done** | `manager.go:188-189` captures wait error |
| 0.5.3.2 | Parse exit code | **Done** | `manager.go:192-194` reads `ProcessState.ExitCode()` |
| 0.5.3.3 | Emit `ProcessExitMsg` to TUI | **Partial** | `ProcessErrorMsg` emitted on `exitCode > 0` at line 206, but no msg for clean exit |
| 0.5.3.4 | Update repo status to crashed/stopped | **Not done** | No automatic status update on exit |
| 0.5.3.5 | Unit test: crash notification | **Done** | `manager_test.go:447-468` `TestManager_FailingProcess_ErrorChan` |
| 0.5.4.1 | Log warning on `Getpgid` failure | **Not done** | `sendSignal` at line 229 silently falls back |
| 0.5.4.2 | Track child PIDs | **Not done** | Only parent PID tracked |
| 0.5.4.3 | Fallback kill sequence | **Not done** | Single SIGTERM only |
| 0.5.4.4 | Post-stop orphan audit | **Not done** | No audit mechanism |
| 0.5.7.1 | Version variable in `internal/version/` | **Not done** | Version lives in `cmd/root.go:24` as package-level var |
| 0.5.7.2 | Replace hardcoded MCP version | **Not done** | `cmd/mcp.go` has its own version string |
| 0.5.7.3 | ldflags in build | **Done** | `cmd_test.go:111-142` verifies ldflags injection works |
| 0.5.7.4 | `ralphglasses version` subcommand | **Not done** | Only `--version` flag exists |
| 0.5.9.1 | Mutex on MCP repos map | **Not assessed** | Requires `mcpserver/tools.go` analysis (out of scope) |
| 0.5.11.1 | Config schema definition | **Not done** | No `config_schema.go` exists |
| 0.5.11.2 | Unknown key warnings | **Not done** | Parser accepts anything |
| 0.5.11.3 | Numeric range validation | **Not done** | No validation on load |
| 0.5.11.4 | Boolean value validation | **Not done** | No validation on load |

**Summary:** Of 49 subtasks across 11 ROADMAP 0.5.x sections, approximately 12 are done, 4 are partial, and 33 remain.

### 3.2 Missing Capabilities

1. **No config schema or validation layer** -- the model layer parses `.ralphrc` into an untyped `map[string]string` with no schema, defaults catalog, or type coercion.
2. **No structured logging** -- foundation packages use no logging at all (discovery, model) or embed log messages in error strings (process). No `slog` integration.
3. **No context propagation** -- all I/O operations (`Scan`, `LoadStatus`, `LoadConfig`, etc.) block without cancellation support.
4. **No graceful shutdown coordination** -- `StopAll()` fires SIGTERM to all processes but does not wait for them to actually exit before returning.
5. **Watcher is stateless** -- each invocation is a fresh watcher instance with no continuity, no debouncing, and no batching of events.

### 3.3 Technical Debt Inventory

| Debt Item | Location | Severity | Notes |
|-----------|----------|----------|-------|
| Non-deterministic config write order | `config.go:75-79` | Low | Use `sort.Strings(keys)` before iteration |
| Watcher create/destroy cycle | `watcher.go:26-77` | High | fd churn, event gaps during teardown |
| Global mutable `lastExits` | `manager.go:44-47` | Medium | Package-level state, hard to test in isolation |
| `version` var in cmd package | `cmd/root.go:24` | Low | Should be in `internal/version/` per ROADMAP 0.5.7.1 |
| ROADMAP stale description | ROADMAP.md line 25 | Low | Says `.ralph/live.log` but code reads `.ralph/logs/ralph.log` |
| `config.go` validKey only on Save | `config.go:62-64` | Medium | Invalid keys accepted on Load but rejected on Save |
| No error wrapping with `%w` | `config.go:64`, `status.go:19` | Medium | `json.Unmarshal` errors not wrapped, breaks `errors.Is` |
| `logstream.go` offset is bare pointer | `logstream.go:19` | Low | Caller manages `*int64` state externally |

## 4. External Landscape

### 4.1 Competitor/Peer Projects

| Project | URL | Relevance | Key Pattern |
|---------|-----|-----------|-------------|
| **k9s** | [github.com/derailed/k9s](https://github.com/derailed/k9s) | High -- same domain (fleet TUI) | `internal/dao/` (data access objects), `internal/model/` (resource watchers with informer pattern), `internal/render/` (display formatters), `internal/config/` (typed config with aliases). Strict separation of data fetching (dao) from rendering (render) from state (model). |
| **lazydocker** | [github.com/jesseduffield/lazydocker](https://github.com/jesseduffield/lazydocker) | Medium -- Go TUI for process management | `pkg/commands/` (command execution layer), `pkg/config/` (typed config with PascalCase fields), `pkg/gui/` (presentation). Uses `lazycore` shared library for cross-project patterns. Config is strongly typed (PascalCase Go structs), not untyped maps. |
| **lazygit** | [github.com/jesseduffield/lazygit](https://github.com/jesseduffield/lazygit) | Medium -- mature Go TUI patterns | Similar to lazydocker. Uses integration tests with recorded terminal sessions. Demonstrates how to test TUI applications end-to-end. |
| **fsnotify** | [github.com/fsnotify/fsnotify](https://github.com/fsnotify/fsnotify) | Direct dependency | Does NOT provide a polling fallback (long-standing open issue #9). Callers must implement their own polling. kqueue platforms require one fd per watched file. Recommends ignoring `Chmod` events. |
| **overseer.go** | [github.com/sasa-b/overseer.go](https://github.com/sasa-b/overseer.go) | Medium -- Go process supervisor | Demonstrates process group management, PID tracking, and graceful shutdown with escalation (SIGTERM -> wait -> SIGKILL). |

### 4.2 Patterns Worth Adopting

1. **k9s DAO pattern**: Separate data access (`dao/`) from model (`model/`) from rendering (`render/`). Ralphglasses currently blends data loading into the model package (`LoadStatus` in `status.go`). Extracting a `dao/` or keeping the current `discovery/` package as the sole data access point would improve testability. The current structure is acceptable at this scale, but as the model grows (benchmark, breakglass, identity already added), the loading functions should be extracted.

2. **lazydocker typed config**: Replace `map[string]string` with a typed Go struct for `.ralphrc` configuration. This eliminates the need for runtime key validation, provides compile-time safety, and enables `go:generate`-based documentation. The current approach requires manual key name matching and type casting everywhere config values are consumed.

3. **Long-lived watcher with debouncing**: Instead of creating a new fsnotify.Watcher per cycle, create one at TUI startup, keep it running for the lifetime of the program, and use a debounce timer (50-100ms) to batch rapid file changes into a single refresh. This is the standard pattern used by k9s informers and Hugo's file watcher.

4. **Escalating kill sequence**: From the Go process management literature: SIGTERM to process group -> wait 5s -> SIGKILL to process group -> verify exit via `os.FindProcess` + signal 0. This prevents orphans from shell scripts that trap SIGTERM and continue.

5. **Config write ordering**: Sort keys alphabetically before writing `.ralphrc` to produce deterministic file output. lazydocker and other tools with config persistence use sorted or schema-ordered writes.

### 4.3 Anti-Patterns to Avoid

1. **Watcher per-event teardown**: The current pattern of creating a new `fsnotify.Watcher` on every 2-second cycle is not seen in any mature Go TUI project. k9s maintains persistent informers; lazydocker maintains persistent watchers. The teardown-recreate cycle risks missing events that occur during the gap.

2. **Package-level mutable state**: The `lastExits` global in `manager.go:44-47` is a package-level mutex-guarded map. This makes the `Manager` type hard to test in isolation and prevents running multiple managers in the same process. Move this state into the `Manager` struct.

3. **Untyped config maps**: Using `map[string]string` for configuration that has known keys, types, and valid ranges requires defensive coding at every call site. The k9s and lazydocker projects both use typed structs.

4. **Silent error swallowing in display methods**: While `repo.go` display methods return `"-"` for nil data (correct), there is no way for the TUI to distinguish "no data loaded" from "data load failed" without checking `RefreshErrors`. Consider adding a distinct display indicator for error state.

## 5. Actionable Recommendations

### 5.1 Immediate Actions (can start now, unblocked)

| # | Action | Files | Effort | Impact | ROADMAP Item |
|---|--------|-------|--------|--------|-------------|
| 1 | **Replace watcher create/destroy with long-lived watcher**: Refactor `WatchStatusFiles` to accept a persistent `*fsnotify.Watcher` created at TUI init. Add debounce timer (100ms). Remove the 2s idle timeout in favor of the TUI's existing `tea.Tick` polling fallback. | `internal/process/watcher.go:26-77`, `internal/tui/model.go` (watcher init) | M | High | 0.5.2 |
| 2 | **Add escalating kill sequence to Stop()**: After SIGTERM, spawn goroutine: wait 5s, check `isProcessAlive(pid)`, if still alive send SIGKILL to process group. Log warning on escalation. | `internal/process/manager.go:236-254` | S | High | 0.5.4.3 |
| 3 | **Sort keys in config Save()**: Before the write loop in `Save()`, collect keys into a sorted slice. Produces deterministic `.ralphrc` output. | `internal/model/config.go:73-80` | S | Low | -- |
| 4 | **Add config schema file**: Create `internal/model/config_schema.go` with a `ConfigSchema` map defining valid keys, types, defaults, and ranges. Wire into `LoadConfig` to warn on unknown keys. | `internal/model/config_schema.go` (new), `internal/model/config.go:34-46` | M | Med | 0.5.11.1, 0.5.11.2 |
| 5 | **Move lastExits into Manager struct**: Replace the package-level `lastExits` with a field on `Manager`. Update `LastExitStatus` method accordingly. | `internal/process/manager.go:44-52, 195-197, 321-329` | S | Med | -- |
| 6 | **Add log warning on Getpgid fallback**: In `sendSignal()`, log a warning when `syscall.Getpgid` fails and falls back to PID-only signaling. Use `util.Debug.Debugf` for now (slog migration comes in Phase 1.7). | `internal/process/manager.go:228-233` | S | Low | 0.5.4.1 |
| 7 | **Wrap JSON unmarshal errors with %w**: In `LoadStatus`, `LoadCircuitBreaker`, `LoadProgress`, wrap `json.Unmarshal` errors so callers can use `errors.Is` and `errors.As`. | `internal/model/status.go:18-20, 31-33, 44-46` | S | Med | 0.5.1 (prep for 1.8) |

### 5.2 Near-Term Actions (depend on 5.1 or require broader changes)

| # | Action | Files | Effort | Impact | ROADMAP Item |
|---|--------|-------|--------|--------|-------------|
| 8 | **Define RefreshErrorMsg and propagate to TUI**: Create a `RefreshErrorMsg` Bubble Tea message type. Emit it from the TUI's refresh cycle when `RefreshRepo` returns errors. Display in repo detail view status bar as a yellow warning. | `internal/tui/model.go`, `internal/tui/views/detail.go` | M | Med | 0.5.1.2, 0.5.1.3 |
| 9 | **Add exponential backoff to watcher retry**: Track consecutive watcher failures in TUI model. On failure, delay re-watch by min(2^n seconds, 30s). Reset counter on successful event. | `internal/process/watcher.go`, `internal/tui/model.go` | S | Med | 0.5.2.4 |
| 10 | **Create internal/version package**: Move `version`, `commit`, `buildDate` to `internal/version/version.go`. Update `cmd/root.go` and `cmd/mcp.go` to import from there. Add `ralphglasses version` subcommand. | `internal/version/version.go` (new), `cmd/root.go:18-26`, `cmd/mcp.go` | S | Low | 0.5.7.1, 0.5.7.2, 0.5.7.4 |
| 11 | **Add config numeric/boolean validation**: In `LoadConfig`, validate known keys against schema. Reject non-numeric values for numeric keys, non-boolean for boolean keys. Return validation errors in `[]error`. | `internal/model/config.go:34-46`, `internal/model/config_schema.go` | M | Med | 0.5.11.3, 0.5.11.4 |
| 12 | **Post-stop orphan audit**: After `Stop()` wait-and-escalate, scan `/proc` or use `pgrep` for processes matching `ralph_loop` pattern rooted at the repo directory. Log any orphans found. | `internal/process/manager.go` (new function) | M | Med | 0.5.4.4 |

### 5.3 Strategic Actions (Phase 1+ scope)

| # | Action | Files | Effort | Impact | ROADMAP Item |
|---|--------|-------|--------|--------|-------------|
| 13 | **Thread context.Context through discovery and model loaders**: Add `ctx context.Context` as first parameter to `Scan()`, `LoadStatus()`, `LoadConfig()`, etc. Check `ctx.Err()` between operations. | `internal/discovery/scanner.go:13`, `internal/model/status.go:12,25,39` | L | Med | 1.9.1, 1.9.2 |
| 14 | **Replace model Load functions with typed config struct**: Define `RalphRC` struct with typed fields (`Model string`, `MaxCallsPerHour int`, etc.). Parse `.ralphrc` into struct with defaults. Eliminate `map[string]string`. | `internal/model/config.go` (rewrite) | L | High | 0.5.11, 1.5.4 |
| 15 | **Migrate to slog**: Replace `util.Debug.Debugf` and any `log.Printf` in foundation packages with `slog.Info`/`slog.Error`/`slog.Debug`. Add structured fields (repo path, file, duration). | `internal/process/manager.go`, `internal/discovery/scanner.go` | M | Med | 1.7.1, 1.7.2 |

## 6. Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|-----------|
| **Watcher fd exhaustion on Linux** -- creating new inotify instances every 2s in a 50-repo fleet may hit `max_user_instances` (default 128) | Medium | High (silent monitoring failure) | Recommendation #1: long-lived watcher. Monitor `/proc/sys/fs/inotify/max_user_instances`. |
| **Orphan `ralph_loop.sh` processes after TUI crash** -- if the TUI is killed with SIGKILL, reaper goroutines die, leaving child processes running without supervision | Medium | High (runaway API spend) | Recommendation #2: kill escalation + #12 orphan audit on startup. PID file recovery (`Recover()`) already partially addresses this. |
| **Config typos cause silent misbehavior** -- e.g., `MAX_CALL_PER_HOUR` (typo) is accepted but ignored, user thinks rate limiting is active | High | Medium (unexpected behavior) | Recommendation #4: config schema with unknown-key warnings. |
| **Race condition between watcher teardown and file write** -- during the gap between watcher close and next watcher create, a file change may be missed entirely | Medium | Low (next polling tick catches it) | Recommendation #1: long-lived watcher eliminates the gap. |
| **Non-deterministic config writes cause false git diffs** -- `Save()` writes keys in random order, making `.ralphrc` changes noisy in version control | High | Low (cosmetic) | Recommendation #3: sorted key output. |
| **Recovered processes cannot be paused/resumed** -- `TogglePause` calls `syscall.Kill(mp.PID, ...)` which works, but `StopAll()` at line 302-307 does not wait for recovered processes to exit | Low | Medium | Add wait-for-exit logic in `StopAll()` for recovered processes. |

## 7. Implementation Priority Ordering

### 7.1 Critical Path

The critical path for Phase 0.5 completion is:

1. Watcher reliability (0.5.2) -- directly affects monitoring correctness
2. Kill escalation (0.5.4) -- directly affects process safety and cost containment
3. Config validation (0.5.11) -- directly affects user experience and error prevention
4. Version consolidation (0.5.7) -- low effort, cleans up multiple locations
5. Error propagation to TUI (0.5.1.2, 0.5.1.3) -- makes existing error data visible

### 7.2 Recommended Sequence

```
Week 1 (parallel):
  [Dev A] Recommendations #1 (watcher) + #9 (backoff)     -> closes 0.5.2
  [Dev B] Recommendations #2 (kill escalation) + #6 (log)  -> closes 0.5.4.1, 0.5.4.3

Week 2 (parallel):
  [Dev A] Recommendations #4 (schema) + #11 (validation)   -> closes 0.5.11
  [Dev B] Recommendations #5 (lastExits) + #7 (error wrap)  -> tech debt cleanup

Week 3 (parallel):
  [Dev A] Recommendation #10 (version pkg)                  -> closes 0.5.7
  [Dev B] Recommendations #8 (RefreshErrorMsg) + #3 (sort)  -> closes 0.5.1.2, 0.5.1.3, cosmetic fix

Week 4:
  [Dev A] Recommendation #12 (orphan audit)                 -> closes 0.5.4.4
  [Dev B] ROADMAP checkbox audit                             -> update all done/partial items
```

### 7.3 Parallelization Opportunities

All ROADMAP 0.5.x items are explicitly marked as independent and parallelizable. Within the recommendations above:

- **Fully parallel**: #1 (watcher) and #2 (kill escalation) touch different files
- **Fully parallel**: #3 (config sort) and #5 (lastExits) and #6 (Getpgid log) touch different files
- **Fully parallel**: #4 (config schema) and #10 (version pkg) are additive new files
- **Sequential**: #8 (RefreshErrorMsg) depends on understanding TUI model structure
- **Sequential**: #11 (config validation) depends on #4 (config schema)
- **Sequential**: #12 (orphan audit) should follow #2 (kill escalation) to avoid redundant work
