
## Self-Improvement Round 8 — Legacy Error Helper Migration

**Date:** 2026-03-26
**Commit:** ab19a22

### Summary
Migrated all ~75 legacy error helper calls (`invalidParams()`, `notFound()`, `internalErr()`) to `codedError()` with typed `ErrorCode` constants across 17 files. Removed the dead `errCode()` function and its 3 wrapper helpers from `tools.go`.

### Files Modified (19 total)
- `handler_repo.go` — 20 calls migrated (was partially done in earlier rounds)
- `handler_rc.go` — 20 calls migrated
- `handler_roadmap.go`, `handler_team.go`, `handler_coverage.go`, `handler_costestimate.go`, `handler_scratchpad.go`, `handler_selftest.go`, `handler_mergeverify.go`, `handler_session.go`, `middleware.go`, `tools.go`, `tools_dispatch.go` — remaining ~35 calls
- 4 test files updated to match uppercase error code format (`INVALID_PARAMS`, `SESSION_NOT_FOUND`)

### Key Fixes
1. **FINDING-68 resolved**: `handleSessionStop` now checks `err.Error()` for "not found" and returns `ErrSessionNotFound` instead of generic `ErrInternal`
2. **Error code specificity improved**: `stop failed` errors in `handler_repo.go` now use `ErrNotRunning`; scan failures use `ErrScanFailed`; repo lookups use `ErrRepoNotFound`; session lookups use `ErrSessionNotFound`
3. **Dead code removed**: `errCode()`, `invalidParams()`, `notFound()`, `internalErr()` all deleted — zero callers remain

### Regressions from Benchmark (carried from Round 7)
- `loop_stop` P95 +14300%, `fleet_status` P95 +3200%, `loop_start` P95 +240%, `loop_status` P95 +300% — likely caused by timeout middleware overhead, not error helpers. Separate investigation needed.
- `loop_stop` success_rate -42.9% — may be related to process lifecycle; not addressed in this round.

### Next Steps
- Investigate benchmark regressions (timeout middleware vs baseline)
- Consider adding `ErrBudgetExceeded`, `ErrTimeout` error codes for budget/timeout-specific failures
- Run full E2E tool probe to validate error code format consistency

## Cycle 2 Results (2026-03-26)

**Status**: Complete — 34/34 packages pass, 0 race conditions, pushed to main.

### Coverage Gains
| Package | Before | After |
|---------|--------|-------|
| sandbox | 8.7% | 95.1% |
| roadmap | 63.1% | 97.5% |
| tui | 29.5% | 52.5% |
| fleet | 71.9% | 82.4% |
| mcpserver | 65.4% | 71.7% |
| session | 70.1% | 80.8% |

### File Splits
- `mcpserver/tools.go` (972 LOC) → `tools.go` + `tools_session.go` + `tools_fleet.go`
- `fleet/server.go` (938 LOC) → `server.go` + `server_handlers.go` + `server_queue.go`
- `session/providers.go` (785 LOC) → `providers.go` + `providers_normalize.go`
- `session/loop.go` (732 LOC) → `loop.go` + `loop_steps.go`

### Bugs Fixed
- Git command timeout hangs in session + mcpserver tests (bare `exec.Command` without context timeout + GPG signing = infinite hang)
- Data race in `util/debug_test.go` (parallel tests mutating global `Debug.Enabled` and `os.Stderr`)
- WS4/WS5 merge conflict in `handler_fleet_test.go` (both agents added different tests to same file)

### Remaining Gaps
- tui/styles: 13.3% (low priority)
- mcpserver: 71.7% (target 80% — needs more handler tests)
- tui core: 52.5% (target 60% — close)
- session/manager.go: 758 LOC (deferred split)

## Cycle 3 Results (2026-03-26)

**Status**: Complete — 34/34 packages pass, 0 race conditions, pushed to main.

### Coverage Gains
| Package | Before | After | Target |
|---------|--------|-------|--------|
| tui/styles | 13.3% | 100.0% | 60% ✅ |
| notify | 71.4% | 93.3% | 80% ✅ |
| tui/components | 70.1% | 86.6% | 82% ✅ |
| batch | 73.9% | 83.7% | 80% ✅ |
| e2e | 72.6% | 82.9% | 78% ✅ |
| tui core | 52.5% | 66.5% | 62% ✅ |
| mcpserver | 71.7% | 74.3% | 80% ❌ |
| enhancer | 87.6% | 87.6% | — |
| session | 80.8% | 80.8% | — |

### File Splits
| File | Before | After |
|------|--------|-------|
| mcpserver/tools_builders.go | 818 | 83 (+3 new files) |
| enhancer/enhancer.go | 836 | 318 (+2 new files) |
| tui/keymap.go | 822 | 197 (+2 new files) |
| e2e/catalog.go | 1019 | 307 (+4 new files) |
| session/manager.go | 758 | 501 (+2 new files) |

### New Test Files (24 total)
- mcpserver: tools_builders_test, errors_test, tools_dispatch_test, tools_registry_test, tools_fleet_event_test, handler_repo_fleet_test, handler_repo_health_test
- tui: handlers_overview_test, handlers_detail_test, handlers_loops_test
- tui/styles: theme_test, icons_test (+ styles_test extended)
- tui/components: gauge_test, tabbar_test, width_test (+ table_test, actionmenu_test extended)
- batch: claude_test, openai_test
- e2e: aggregate_test, baseline_test
- notify: notify_test extended

### Remaining Gaps
- mcpserver: 74.3% (target 80%) — handler_rc.go, handler_session.go, schemas.go need more test depth
- session/manager.go still 501 LOC (target <400)
