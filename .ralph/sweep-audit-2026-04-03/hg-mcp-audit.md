# hg-mcp Audit Report

## Summary

hg-mcp is a well-architected, production-grade MCP server with 1,060+ tools across 119 modules. Core infrastructure (registry, middleware, concurrency) is solid — thread-safe singletons, proper mutex usage, comprehensive CI with race detection. The highest-priority improvement is **migrating the ~30 handlers in `inventory/module.go` and similar large modules that still use manual `GetStringParam` + `== ""` + `CodedErrorResult` into `RequireStringParam`**, which already exists but is underused (412 usages vs hundreds of manual blocks remaining). The CLAUDE.md is largely accurate but has a stale Go version claim and inflated DRY opportunity numbers.

## Findings

### [1] Non-deferred `resp.Body.Close()` in 5 client functions (Severity: medium)
- **File(s)**: `internal/clients/bandcamp.go:158`, `internal/clients/dashboard.go:396,451`, `internal/clients/fingerprint.go:433`, `internal/clients/mixcloud.go:178`
- **Issue**: These call `resp.Body.Close()` without `defer`. If future code reads `resp.Body` after the close or if a panic occurs between the HTTP call and the close, the body leaks or reads fail. The rest of the codebase consistently uses `defer resp.Body.Close()`.
- **Fix**: Replace `resp.Body.Close()` with `defer resp.Body.Close()` immediately after the error check on `httpClient.Do()`. For the dashboard.go cases where the function returns right after, wrap in defer for consistency.
- **Effort**: small

### [2] `calendar/module.go` uses raw `json.MarshalIndent` + `TextResult` instead of `JSONResult` (Severity: low)
- **File(s)**: `internal/mcp/tools/calendar/module.go:429,457,513,530,552,580,625,652,701,728` (12 instances), also `ledfx/module.go`, `prolink/bridge_tools.go`, `showcontrol/profiles.go`, `snapshots/auto.go` (6 more)
- **Issue**: `tools.JSONResult(data)` does exactly `json.MarshalIndent` + `TextResult`. Using the raw pattern bypasses future improvements to `JSONResult` (e.g., truncation, schema validation) and silently drops marshal errors with `_ =`.
- **Fix**: Replace `data, _ := json.MarshalIndent(result, "", "  "); return tools.TextResult(string(data)), nil` with `return tools.JSONResult(result), nil` across all 18 instances.
- **Effort**: small

### [3] `inventory/module.go` is 3,673 lines with 44 handlers in one file (Severity: medium)
- **File(s)**: `internal/mcp/tools/inventory/module.go:1-3673`
- **Issue**: Single file contains module registration, 51 tool definitions, and 44 handler functions. Navigation is difficult, and merge conflicts are likely when multiple changes touch this file. `resolume/module.go` (2,231 lines, 39 handlers) has the same problem.
- **Fix**: Split handlers into domain-grouped files: `crud_handlers.go`, `listing_handlers.go`, `valuation_handlers.go`, `photo_handlers.go` (inventory already has `photo_ingest.go` as precedent). Keep `module.go` for `Tools()` and `Module` interface only.
- **Effort**: medium

### [4] Swallowed errors in `TaskManager.RunAsync` goroutine (Severity: medium)
- **File(s)**: `internal/mcp/tasks/tasks.go:319,322,327,329`
- **Issue**: `StartTask`, `UpdateProgress`, `FailTask`, and `CompleteTask` return errors that are discarded with `_ =`. If disk persistence fails, the task state becomes inconsistent between memory and disk with no diagnostic signal.
- **Fix**: Log errors with `slog.Warn("task state update failed", "task_id", taskID, "error", err)` instead of discarding. These are background operations so returning errors isn't possible, but logging preserves observability.
- **Effort**: small

### [5] 209 scattered `os.Getenv` calls in client constructors (Severity: medium)
- **File(s)**: `internal/clients/*.go` (distributed across 100+ files)
- **Issue**: Each client constructor independently calls `os.Getenv` for its configuration. No validation, no centralized documentation of required vs optional vars, no startup-time detection of missing critical config. A typo in a var name silently returns empty string.
- **Fix**: Create `internal/config/env.go` with `GetRequired(key)` (logs + returns error) and `GetOptional(key, default)` helpers. Migrate client constructors incrementally, starting with the most critical integrations (inventory, discord, resolume).
- **Effort**: large (migration), small (helper creation)

### [6] `conditionMap` in `handleFBContent` is a hardcoded inline map (Severity: low)
- **File(s)**: `internal/mcp/tools/inventory/module.go:1679-1691`
- **Issue**: Condition mapping (`"new" -> "New"`, `"poor" -> "Fair"`, `"Renewed" -> "Like New"`) is defined inline in a handler. This mapping is business logic that may change and is not testable independently. Also maps `"poor"` to `"Fair"` which seems intentionally misleading for FB Marketplace.
- **Fix**: Extract to a package-level `var fbConditionMap` or a function `mapToFBCondition(condition string) string` with its own unit test. Review whether `"poor" -> "Fair"` is intended business logic.
- **Effort**: small

### [7] `loadFromDisk` silently ignores corrupt JSON (Severity: low)
- **File(s)**: `internal/mcp/tasks/tasks.go:355-357`
- **Issue**: If `tasks.json` contains corrupt JSON, `json.Unmarshal` fails and the function returns silently — all persisted tasks are lost without any log message. The user has no way to know their task history was dropped.
- **Fix**: Add `slog.Warn("failed to load persisted tasks", "path", path, "error", err)` before the return on line 356.
- **Effort**: small

### [8] `RequireStringParam` exists but ~30 inventory handlers still use manual validation (Severity: low)
- **File(s)**: `internal/mcp/tools/inventory/module.go` (30 instances of `GetStringParam` + `== ""` + `CodedErrorResult`)
- **Issue**: `RequireStringParam` was added as a DRY helper (412 usages across the codebase) but the inventory module predates it and still uses the verbose 3-line pattern. This is the exact "~486 repeated parameter validation blocks" noted in CLAUDE.md's DRY section — but the helper already exists, making the documented count misleading.
- **Fix**: Replace manual `GetStringParam`+check+`CodedErrorResult` blocks with `RequireStringParam` in inventory and the ~29 other modules that still use the old pattern.
- **Effort**: medium (mechanical, low risk)

### [9] `resolume.go` client is 2,075 lines with complex OSC protocol handling (Severity: low)
- **File(s)**: `internal/clients/resolume.go:1-2075`
- **Issue**: Single client file handling HTTP API calls, OSC binary protocol construction, state synchronization, and effect management. High cognitive load for maintenance.
- **Fix**: Split into `resolume_http.go` (REST API), `resolume_osc.go` (OSC protocol), `resolume_effects.go` (effect type handling). Keep `resolume.go` for the client struct and constructor.
- **Effort**: medium

### [10] `rclone.go` client is 2,449 lines — largest client file (Severity: low)
- **File(s)**: `internal/clients/rclone.go:1-2449`
- **Issue**: Monolithic file covering all rclone operations (copy, sync, mount, serve, bisync, etc.). Similar to finding [9].
- **Fix**: Split by operation domain: `rclone_transfer.go`, `rclone_mount.go`, `rclone_serve.go`, `rclone_config.go`.
- **Effort**: medium

### [11] Handler naming inconsistency across ~30 modules (Severity: low)
- **File(s)**: `internal/mcp/tools/ardour/module.go` (`handleStatus`), `internal/mcp/tools/chataigne/module.go` (`handleStatus`), `internal/mcp/tools/discord/module.go` (`handleStatus`), ~27 more modules
- **Issue**: ~30 modules use bare `handleStatus`/`handleHealth` names while ~89 modules use the prefixed `handleAbletonStatus`/`handleBandcampStatus` convention. Package scoping makes both correct, but the inconsistency slows grep-based navigation across modules.
- **Fix**: Standardize on `handle<Module><Action>` convention. Low priority — cosmetic, but affects developer velocity when grepping across modules.
- **Effort**: medium (mechanical)

## CLAUDE.md Accuracy

| Section | Status | Issue |
|---------|--------|-------|
| "Go 1.25+ required" (line 104) | **Outdated** | `go.mod` declares `go 1.26.1`. Update to "Go 1.26+" |
| "1,190+ tools across 119 modules" (line 3) | **Slightly inflated** | Test MinTools assertions sum to ~1,060. Update to "1,060+ tools" or re-count |
| "368 manual json.MarshalIndent calls" (DRY section) | **Outdated** | Only 18 remain in tools, 31 in clients. Most were already migrated to `JSONResult` |
| "~486 repeated parameter validation blocks" (DRY section) | **Misleading** | `RequireStringParam` helper exists with 412 usages. The remaining manual blocks are ~30-50, not 486 |
| "208 scattered os.Getenv calls" (DRY section) | **Understated** | Actual count is 209 in clients alone, ~294 total across codebase |
| "Identical test boilerplate across ~91 modules" (DRY section) | **Partially outdated** | `testutil.AssertModuleInfo()` helper already exists and all 119 modules use it. The boilerplate is the 3-line `TestModuleInfo` wrapper function — still duplicated but already minimal |
| Project structure diagram | **Accurate** | Matches actual layout |
| Handler helpers documentation | **Accurate** | All documented functions exist with correct signatures |
| Runtime groups (10 groups) | **Accurate** | Verified in `registry.go` categoryToRuntimeGroup map |
| mcpkit dependency table | **Accurate** | Shim layer files match documented architecture |

## Recommended Next Actions

1. **Fix non-deferred `resp.Body.Close()` in 5 client files** — 10-minute fix, prevents resource leaks, high safety impact
2. **Replace 18 raw `json.MarshalIndent` calls with `tools.JSONResult()`** — 20-minute mechanical fix, eliminates silent marshal error drops
3. **Update CLAUDE.md DRY section and Go version** — 10-minute edit, prevents future developers from chasing already-solved problems
