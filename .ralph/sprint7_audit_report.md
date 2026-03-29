# Sprint 7 WS-6 — 6-Dimension Audit Report

**Date:** 2026-03-28
**Branch:** main (worktree agent-ae698afa)
**Auditor:** Claude Sonnet 4.6 (read-only)

---

## Dimension 1 — Input Validation (OWASP Focus)

### BLOCKER

- **internal/mcpserver/handler_rdcycle.go:36** — `scratchpadName` flows directly into a `filepath.Join` without validation. A caller can supply `../../etc/passwd` or similar and the path will escape `.ralph/`. The `ValidateRepoName` / `ValidatePath` functions exist and are used elsewhere — they are simply not called here. Affected handlers: `handleFindingToTask` (line 36), `handleCyclePlan` (line 227, via glob), `handleFindingReason` (line 1007-1011). Fix: call `ValidatePath(scratchpadName, repoPath)` or `ValidateRepoName(scratchpadName)` before constructing the path.

- **internal/mcpserver/handler_scratchpad.go:113,150,218,268** — The `name` parameter read by `handleScratchpadRead`, `handleScratchpadAppend`, `handleScratchpadDelete`, and `handleScratchpadResolve` is never validated. It is joined directly into `filepath.Join(repoPath, ".ralph", name+"_scratchpad.md")`. A `name` value of `../../secret` would read/write outside the `.ralph` directory. Fix: apply `ValidateRepoName(name)` or `ValidatePath` before file access.

- **internal/mcpserver/handler_rdcycle.go:354-382 (handleCycleMerge)** — The `worktree_paths` CSV is parsed and each path is fed to `os.Stat` and `exec.CommandContext(ctx, "git", "-C", wt, ...)` without any containment check. A caller can supply `/etc` or `../../../` as a worktree path, causing git operations in arbitrary filesystem locations. `ValidatePath` with `s.ScanPath` as the root is not called. Fix: call `ValidatePath(wt, s.ScanPath)` for each element.

- **internal/hooks/hooks.go:130** — Hook `Command` values from `.ralph/hooks.yaml` are passed verbatim to `exec.CommandContext(ctx, "sh", "-c", h.Command)`. If a user's hook config is malicious or the YAML is tampered with, arbitrary shell commands execute as the server process. This is by design but the hook config is loaded from the repo path without any content sanitization. **Impact:** full shell code execution. Mitigation would require a command allowlist or sandboxing; at minimum document that hooks.yaml must be trusted.

- **internal/session/loop_worker.go:280** — `verify_commands` from the loop profile (ultimately from the `ralphglasses_loop_start` MCP tool's `verify_commands` parameter) are passed verbatim to `exec.CommandContext(ctx, "bash", "-lc", command)`. There is no sanitization or allowlisting of these commands. An MCP client can inject arbitrary shell commands. Fix: validate commands against an allowlist pattern, or at minimum document as privileged input.

### WARNING

- **internal/mcpserver/handler_scratchpad_advanced.go:49,257,507** — Same pattern as the scratchpad handler: `name` used directly in `filepath.Join` without `ValidateRepoName` or `ValidatePath`. Affects `handleScratchpadContext`, `handleScratchpadValidate`, `handleScratchpadAppend` (advanced variant), and `handleScratchpadReason`.

- **internal/mcpserver/handler_rdcycle.go:658** — `loopID` from `handleLoopReplay` is joined into `filepath.Join(repoPath, ".ralph", "loop_runs", loopID+".json")` without validation. A `loopID` of `../../shadow` escapes the `.ralph/loop_runs/` directory. Fix: validate `loopID` is alphanumeric/dash only.

- **internal/tui/views/diffview.go:31,51** — `fromRef` is concatenated into git diff arguments (`fromRef+"..HEAD"`) without validation. Git itself would reject malformed refs, but a maliciously crafted ref like `--evil-option` could inject git flags. Fix: validate `fromRef` matches `[a-zA-Z0-9._/~^-]+` before use.

- **internal/mcpserver/handler_rdcycle.go:196** — `baselineID` is constructed as `fmt.Sprintf("baseline-%s-%d", repo, time.Now().Unix())`, where `repo` is already validated via `ValidateRepoName` in `resolveRepoPath`. This is safe but worth noting the dependency.

### INFO

- **internal/envkit/mise.go:111** — `exec.CommandContext(ctx, "bash", "-c", "curl -fsSL https://mise.jdx.dev/install.sh | bash")` runs a remote script. This is a bootstrapping concern, not a runtime injection issue, but represents a supply-chain risk.

- **internal/mcpserver/handler_repo.go:88** — `ValidateRepoName` is called correctly before lookup. Good pattern to replicate.

---

## Dimension 2 — API Contract Correctness

### WARNING

- **ralphglasses_finding_to_task schema (tools_builders_misc.go:323-327) vs handler (handler_rdcycle.go:30)** — Schema registers `finding_id` and `scratchpad_name` only. Handler additionally reads `repo` via `getStringArg(req, "repo")`. The `repo` parameter is silently ignored if not provided (falls back via `resolveRepoPath`), but it is not documented in the schema so callers cannot pass it. If multiple repos are managed and the auto-detect fallback fails, the handler returns an error that is confusing given the missing schema field. Fix: add `mcp.WithString("repo", ...)` to the schema.

- **ralphglasses_cycle_plan schema (tools_builders_misc.go:333-338) vs handler (handler_rdcycle.go:217)** — Schema has `previous_cycle_id`, `max_tasks`, `budget`. Handler also reads `repo` (line 217). Same issue as above: `repo` is not in the schema. Fix: add `mcp.WithString("repo", ...)`.

- **ralphglasses_cycle_schedule schema (tools_builders_misc.go:344-348) vs handler (handler_rdcycle.go:468)** — Schema has `cron_expr` and `cycle_config`. Handler additionally reads `repo` (line 468). Fix: add `mcp.WithString("repo", ...)`.

- **ralphglasses_merge_verify schema (tools_builders_misc.go:494-501) vs handler (handler_mergeverify.go:173-187)** — Schema says `repo` is "Repo path (absolute)". Handler calls `filepath.Abs(repo)` — so relative paths are actually accepted (resolved against CWD). Schema description is misleading; relative paths work but aren't documented. Minor contract gap.

- **ralphglasses_cycle_baseline handler (handler_rdcycle.go:100)** — Handler ignores the incoming `context.Context` (bound to `_`) and creates its own `context.WithTimeout(context.Background(), 60*time.Second)`. This means client-side cancellation of the MCP call is not propagated to the git/go subprocesses. Same pattern in `handleCycleMerge` (line 369) and `handleObservationCorrelate` (line 1210). Fix: use the handler's `ctx` as the parent.

### INFO

- **ralphglasses_loop_start schema** — `verify_commands` parameter is documented as newline-separated, but the handler uses `splitLines`. This is consistent but may surprise callers expecting CSV.

- **go.mod**: `gonum.org/v1/gonum`, `modernc.org/sqlite`, and `nhooyr.io/websocket` are marked `// indirect` but are directly imported by `internal/bandit/thompson.go`, `internal/eval/bayesian.go`, `internal/wsclient/wsclient.go`, and `internal/session/store_sqlite.go`. `go mod tidy` would promote these to direct dependencies. No runtime impact, but the module file is misleading.

---

## Dimension 3 — Dead Code Detection

### INFO

- **`go vet ./...`** produced no output — zero vet issues found. Clean.

- **internal/fleet/worker.go:295** — `_ = data` is a dead assignment. A `json.Marshal` error branch assigns result to `data` but never uses it (only logs and returns). Low priority.

- **internal/mcpserver/handler_mergeverify.go:160** — `_ = data` inside `parseCoverageTotal` — the raw profile data is read but then discarded in the fallback branch (go tool cover is always preferred). The variable is kept only so the `os.ReadFile` call isn't "unused". Consider removing the `os.ReadFile` call entirely since it's never used.

- **internal/awesome/sync.go:89** — `analysis, _ = LoadAnalysis(opts.SaveTo)` silently ignores load errors. If the file is corrupt, execution continues with a zero-value struct. The error is not logged. This is intentional (best-effort load) but undocumented.

---

## Dimension 4 — Dependency Hygiene

### WARNING

- **`go mod tidy` diff detected** — Three dependencies are mis-classified as `// indirect` but are directly imported: `gonum.org/v1/gonum`, `modernc.org/sqlite`, `nhooyr.io/websocket`. Running `go mod tidy` would move them to the `require` block as direct dependencies and remove the `// indirect` marker. Also, `google/go-cmp` has an outdated sum entry. The module is buildable as-is but the go.mod is in a non-tidy state.

### INFO

- **All dependencies are pinned to specific versions** — No floating `latest` or `^` constraints. Good hygiene.

- **`go 1.26.1` in go.mod** — This is a future Go version (as of 2026-03-28 the latest stable is ~1.22/1.23). If the toolchain version doesn't match this directive, builds could fail or emit warnings. Verify the actual toolchain in use.

- **`modernc.org/sqlite v1.48.0`** — Pure-Go SQLite driver. No CGO concerns. Good choice for the thin-client target.

---

## Dimension 5 — Error Handling Consistency

### WARNING

- **internal/blackboard/blackboard.go:233** — `_, _ = f.Write(data)` in `appendToFile`. Write failures during blackboard persistence are silently swallowed with no logging. If the filesystem is full, entries are lost without any signal. Fix: log the error at `slog.Warn` level.

- **internal/blackboard/blackboard.go:182,219** — `_ = os.MkdirAll(bb.stateDir, 0755)` — MkdirAll failures are silently ignored. If the directory cannot be created (permissions, read-only fs), subsequent writes will fail and also be silently swallowed. Fix: log the error.

- **internal/fleet/worker.go:101,155,183,197,250** — `Heartbeat`, `CompleteWork`, and `SendEvents` results are all `_ = ...`. Failed fleet coordination calls are not logged or retried. If the coordinator is unreachable, tasks complete locally but the fleet has no record. Consider at least a `slog.Warn` on failure.

- **internal/fleet/server_dispatch.go:142** — `_ = json.NewEncoder(w).Encode(v)` — JSON encoding failures for HTTP responses are silently swallowed. A partial or empty HTTP response is sent without indication of error.

- **internal/mcpserver/handler_rdcycle.go:271** — `_ = json.Unmarshal(pdata, &patterns)` — Unmarshal failure of `improvement_patterns.json` is silently ignored. If the file is corrupt, scoring continues with no patterns, which produces incorrect task prioritization. Fix: log at `slog.Warn`.

### INFO

- **71 bare `return err` sites across internal/** — Most are in internal helpers where context is clear from the call site. The most impactful are in `internal/model/` and `internal/fleet/` where errors cross package boundaries and lose context. Examples:
  - `internal/fleet/queue.go:195,207,212` — SQLite queue errors returned bare; callers have no context about which operation failed.
  - `internal/model/benchmark.go:51,55,61,64` — Multiple bare returns lose the operation name.

- **Multiple `fmt.Errorf` without `%w`** — Errors in `internal/batch/`, `internal/enhancer/gemini_client.go`, and `internal/config/config_schema.go` create new errors rather than wrapping originals. This prevents `errors.Is`/`errors.As` matching at call sites. Examples:
  - `internal/batch/claude.go:172` — `fmt.Errorf("batch: api error (status %d): %s", ...)` drops the underlying HTTP error.
  - `internal/enhancer/gemini_client.go:169,178` — Same pattern.

---

## Dimension 6 — Performance Hotspots

### WARNING

- **internal/mcpserver/handler_selfimprove.go:71-73** — Background goroutine `go func() { _ = s.SessMgr.RunLoop(context.Background(), run.ID) }()` has no cancellation handle and no WaitGroup tracking. If the server shuts down, this goroutine continues running with a detached `context.Background()` context until the loop finishes naturally. There is no way to cancel it from outside. Same pattern at `handler_loop.go:141`. Fix: use a server-lifecycle context or store the cancel function.

- **internal/session/loop_steps.go:288** — Worker goroutines are fanned out with `go func(workerIdx int, t LoopTask)` and collected via `resultCh`. The collect loop uses `workerCollectTimeout := time.After(15 * time.Minute)`. If a goroutine panics and the recover sends to `resultCh`, but the `collected < len(tasks)` count is off due to a race, the loop can hang for 15 minutes. The recover handling looks correct but deserves scrutiny under stress.

- **internal/mcpserver/handler_rdcycle.go:126** — `handleCycleBaseline` ignores the incoming `ctx` and creates `context.WithTimeout(context.Background(), 60*time.Second)`. This means MCP client cancellation is not propagated; if the client drops the connection, the go build/test commands continue running for up to 60 seconds. Same issue in `handleCycleMerge` (30s per worktree), `handleObservationCorrelate` (30s), and `parseCoverageTotal` (30s).

- **internal/tui/views/diffview.go:53** — `diffCmd.Output()` is called with `exec.Command` (no context). For large repos with extensive history between `fromRef` and HEAD, this call blocks indefinitely. Fix: use `exec.CommandContext` with a reasonable timeout.

### INFO

- **internal/session/loop_steps.go:311-313** — Three parallel `make([]string, len(tasks))` allocations with fixed capacity are appropriate and correct. No issue.

- **internal/awesome/analyze.go:50** — `go func(i int, entry AwesomeEntry)` fan-out over all awesome entries. No explicit goroutine cap. For repos with thousands of entries this could spawn thousands of goroutines simultaneously. Consider using a semaphore or worker pool.

- **internal/mcpserver/handler_rdcycle.go:1146** — `filepath.Glob` over `observations/*.jsonl` with no file count limit. If thousands of observation files accumulate, each is read into memory (line 1167) and all are held simultaneously. Consider pagination or streaming.

---

## Summary Table

| Dimension | BLOCKERs | WARNINGs | INFOs |
|-----------|----------|----------|-------|
| Input Validation | 5 | 3 | 2 |
| API Contract | 0 | 4 | 2 |
| Dead Code | 0 | 0 | 3 |
| Dependency Hygiene | 0 | 1 | 2 |
| Error Handling | 0 | 5 | 2 |
| Performance | 0 | 3 | 3 |
| **Total** | **5** | **16** | **14** |

## Top Priority Fixes

1. **[BLOCKER]** Add `ValidatePath` or `ValidateRepoName` to all scratchpad `name` parameters before `filepath.Join` (affects ~8 handlers in `handler_scratchpad.go`, `handler_scratchpad_advanced.go`, and `handler_rdcycle.go`).
2. **[BLOCKER]** Add `ValidatePath(wt, s.ScanPath)` per-element in `handleCycleMerge` before passing worktree paths to `os.Stat` and `git -C`.
3. **[BLOCKER]** Add `ValidatePath` to `loopID` in `handleLoopReplay` before file path construction.
4. **[BLOCKER]** Document `hooks.yaml` `command` and `verify_commands` as privileged/trusted inputs, or implement a command allowlist.
5. **[WARNING]** Add missing `repo` parameter to schemas for `ralphglasses_finding_to_task`, `ralphglasses_cycle_plan`, `ralphglasses_cycle_schedule`.
6. **[WARNING]** Replace `context.Background()` with handler `ctx` in `handleCycleBaseline`, `handleCycleMerge`, `handleObservationCorrelate`, and `parseCoverageTotal`.
7. **[WARNING]** Log (don't swallow) errors in `blackboard.appendToFile`, fleet `Heartbeat`/`CompleteWork`, and `json.NewEncoder.Encode`.
8. **[WARNING]** Run `go mod tidy` to correct direct/indirect classification of `gonum`, `sqlite`, and `websocket` dependencies.
