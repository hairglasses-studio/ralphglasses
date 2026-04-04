# mcpkit Audit Report

## Summary

mcpkit is a well-architected, thoroughly tested Go toolkit with 33+ packages at 90%+ coverage and clean dependency layering. The codebase is production-grade with strong patterns (middleware chains, typed handlers, thread-safe registries). The single highest-priority improvement is **fixing silent audit event loss in `security/audit.go`** — a security-critical package that silently drops events on queue overflow and file I/O errors, undermining its core purpose.

## Findings

### [1] Audit logger silently drops security events (Severity: high)
- **File(s)**: `security/audit.go:109-113, 265-293`
- **Issue**: Three silent failure modes in a security-critical component:
  1. **Line 110-113**: When `writeQueue` is full, events are silently dropped via `default` case in select — no counter, no log, no signal to caller.
  2. **Line 267-269**: If `os.MkdirAll` fails, `backgroundWriter` returns silently — all future file writes are lost with no indication.
  3. **Line 117**: Exporter errors discarded with `_ = exp.Export(event)` — external audit sinks fail invisibly.
- **Fix**: (a) Add an `atomic.Int64` dropped-event counter and surface it in `GetStats()`. (b) Log `backgroundWriter` startup failures to stderr as a last-resort sink. (c) Collect exporter errors and expose via a callback or stats field.
- **Effort**: small

### [2] Audit event ID collision risk (Severity: medium)
- **File(s)**: `security/audit.go:99`
- **Issue**: `event.ID = fmt.Sprintf("%d-%d", event.Timestamp.UnixNano(), time.Now().Nanosecond())` calls `time.Now()` twice — the second call's `Nanosecond()` component is redundant (it's the sub-second part of a *different* timestamp). Two events logged within the same nanosecond get identical IDs.
- **Fix**: Use a monotonic counter: `atomic.AddUint64(&l.seq, 1)` appended to the timestamp, or use `uuid` / `crypto/rand`.
- **Effort**: small

### [3] `ralph/loop.go` Run() is 450+ lines with cyclomatic complexity ~25 (Severity: medium)
- **File(s)**: `ralph/loop.go:20-453`
- **Issue**: Single function handles spec loading, iteration control, LLM sampling with retries, decision parsing, DAG enforcement, tool execution, auto-verify, history management, cost tracking, stuck detection, and circuit breaker recording. This is the hardest function in the codebase to review, test, or modify safely.
- **Fix**: Extract into focused methods: `executeIteration()`, `callSampler()`, `executeToolCalls()`, `recordAndCheck()`. Each becomes independently testable. The main loop becomes a ~50-line orchestrator.
- **Effort**: large

### [4] Panic recovery returns both error result AND Go error (Severity: medium)
- **File(s)**: `registry/registry.go:349-355`
- **Issue**: On panic recovery, `wrapHandler` sets both `result = MakeErrorResult(...)` and `err = fmt.Errorf(...)`. The project's own tool design rules (`.claude/rules/tool-design.md`) mandate "Always return `(*CallToolResult, nil)` — never `(nil, err)`". Returning both can confuse callers that branch on `err != nil` vs checking `result.IsError`.
- **Fix**: Set `err = nil` after assigning the error result, or keep err for logging but document that panic recovery is the one exception.
- **Effort**: small

### [5] CLAUDE.md lists `feedback` and `a2a` as existing packages (Severity: medium)
- **File(s)**: `CLAUDE.md` (Package Map table, Dependency Layers section)
- **Issue**: `feedback/` directory does not exist (Phase 42 planned). `a2a/` directory does not exist (Phase 39, deferred). Both are listed in the Package Map and Layer 2/3 dependency tables as if they're implemented, misleading agents and contributors.
- **Fix**: Move both to a separate "Planned Packages" section, or remove from Package Map and Layers until implemented.
- **Effort**: small

### [6] `transport` package at 44% coverage, contradicting 90%+ claim (Severity: medium)
- **File(s)**: `transport/*.go`, `ROADMAP.md` Phase 30 summary
- **Issue**: ROADMAP.md line 9 claims "All 35 non-example packages now at 90%+ coverage". `transport` is at 44.0% — `http.go` has 0% coverage, `websocket.go` and `middleware.go` are partial. Either transport was excluded from the Phase 30 count or the claim is stale.
- **Fix**: Either add tests to bring transport to 90%+ (it's a thin adapter layer, should be straightforward), or update ROADMAP.md to note the exception. Transport was added in Phase 33 (after Phase 30), so the claim was true when written but is now misleading.
- **Effort**: medium

### [7] `security/audit.go` Close() races with backgroundWriter (Severity: medium)
- **File(s)**: `security/audit.go:258-263, 265-278`
- **Issue**: `Close()` calls `close(l.stopCh)` then immediately closes exporters. But `backgroundWriter` may still be processing a queued event when exporters are closed. There's no synchronization to wait for the goroutine to drain. Also, calling `Close()` twice panics (double close on `stopCh`).
- **Fix**: Add a `sync.Once` for Close, and use a `sync.WaitGroup` or a done channel to wait for `backgroundWriter` to exit before closing exporters.
- **Effort**: small

### [8] `rdcycle/filestore.go` silently skips unreadable artifacts (Severity: low)
- **File(s)**: `rdcycle/filestore.go:68-89` (inferred from agent report)
- **Issue**: `ReadDir` and `ReadFile` errors cause `continue` with no logging. If the artifact directory becomes partially corrupted or permissions change, the store returns partial data with no indication of incompleteness.
- **Fix**: Collect errors and either return them as a second return value or log at warning level.
- **Effort**: small

### [9] `session/MemStore` eviction goroutine has no context cancellation (Severity: low)
- **File(s)**: `session/session.go:177-194`
- **Issue**: `evictLoop` is stopped only via `ms.done` channel. If the caller forgets to call `Close()`, the goroutine leaks. There's no `context.Context` parameter on `NewMemStore` to tie the goroutine lifetime to a parent scope. This is a common Go lifecycle issue.
- **Fix**: Accept an optional `context.Context` in `MemStoreOpts` and select on `ctx.Done()` alongside the done channel. Or document that `Close()` is mandatory and consider a finalizer warning.
- **Effort**: small

### [10] Inconsistent error wrapping across packages (Severity: low)
- **File(s)**: Various — `gateway/upstream.go`, `handoff/handoff.go`, `auth/discovery.go`
- **Issue**: Some error returns use `fmt.Errorf("context: %w", err)` (proper wrapping), others return bare `err` without context. Inconsistency makes production debugging harder because error chains lose the call path. Notably `gateway/upstream.go` returns bare errors from connect/syncTools.
- **Fix**: Adopt a convention: all errors crossing package boundaries get `fmt.Errorf("pkg.func: %w", err)` wrapping. A lint rule (`wrapcheck` or custom) can enforce this.
- **Effort**: medium

### [11] `integrity.go` Fingerprint silently degrades when schema marshal fails (Severity: low)
- **File(s)**: `registry/integrity.go:33-36`
- **Issue**: If `json.Marshal(td.Tool.InputSchema)` fails, the schema bytes are silently omitted from the fingerprint. Two tools with identical name/description but different (unmarshalable) schemas would get the same fingerprint, defeating tamper detection for that edge case.
- **Fix**: Return an error from `Fingerprint()`, or fall back to `fmt.Sprintf("%v", td.Tool.InputSchema)` to ensure schema differences are always captured.
- **Effort**: small

### [12] ROADMAP.md package count stale (Severity: low)
- **File(s)**: `ROADMAP.md`
- **Issue**: Claims "33 packages" with doc.go files (Phase 29) and "35 non-example packages" at 90%+ (Phase 30). Actual count is ~39 packages including `session`, `transport`, `feedback` (planned), `executil`, `configutil`, `pathutil`, and utility packages added after those phases.
- **Fix**: Update counts or change to "all packages at time of writing" with a date qualifier.
- **Effort**: small

## CLAUDE.md Accuracy

| Section | Status | Issue |
|---------|--------|-------|
| Package Map | **Outdated** | Lists `feedback` (not implemented) and `a2a` (not implemented) as existing packages |
| Dependency Layers | **Outdated** | Layer 2 includes `feedback`, Layer 3 includes `a2a` — neither exist. `transport` missing from Layer 1 in ROADMAP.md but present in CLAUDE.md (CLAUDE.md is correct) |
| Phase 30 claim | **Stale** | "All 35 non-example packages at 90%+" — `transport` is at 44% |
| Phase 29 claim | **Stale** | "All 33 packages have doc.go" — count has grown since |
| Commands section | **Accurate** | `make check`, `make build-official`, `make check-dual` all verified working |
| Coding Conventions | **Accurate** | Middleware signature, error codes, param extraction, result builders all match code |
| Testing section | **Accurate** | mcptest patterns, assertion helpers, isolation all match reality |
| Package descriptions | **Accurate** | All implemented package one-liners match their actual functionality |

**Suggested CLAUDE.md edits:**
1. Remove `feedback` from Package Map and Layer 2 (or move to "Planned" section)
2. Remove `a2a` from Layer 3 (or add "(planned)" qualifier)
3. Add `executil`, `configutil`, `pathutil` to Package Map if they're considered first-class packages
4. Update phase status notes to reflect transport coverage gap

## Recommended Next Actions

1. **Fix audit logger silent failures** (`security/audit.go`) — add dropped-event counter, startup error logging, Close() synchronization. High impact on security observability, ~30 min effort.
2. **Update CLAUDE.md package accuracy** — remove `feedback`/`a2a` from implemented tables, fix package counts. Prevents agent confusion, ~15 min effort.
3. **Bring `transport` to 90% coverage** — thin adapter layer, mostly needs HTTP/WebSocket handler tests. Restores the "all packages 90%+" invariant, ~2 hours effort.
