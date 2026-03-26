
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
