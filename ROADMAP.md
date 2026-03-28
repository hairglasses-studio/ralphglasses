# Ralphglasses Roadmap

Command-and-control TUI + bootable thin client for parallel multi-LLM agent fleets.

**Last updated:** 2026-03-27
**Codebase:** 37 packages, 115 MCP tools (13 namespaces + 2 meta), 18 TUI views
**Status:** 619 tasks, 175 complete (28.3%), 444 remaining
**Key deps:** Go 1.26.1, mcp-go v0.45.0, bubbletea v1.3.10, anthropic-sdk-go v1.27.1

## Core Deliverables

### Deliverable 1: `ralphglasses` Go Binary
Cross-platform Unix TUI (k9s-style) built with Charmbracelet (BubbleTea + Lip Gloss).
Manages multi-session, multi-provider LLM loops from any terminal.

### Deliverable 2: Bootable Linux Thin Client
Featherweight, low-graphics bootable Linux (Ubuntu 24.04-based) that boots into i3 + the ralphglasses TUI.
Supports multi-monitor (7-display, dual-NVIDIA-GPU) and autoboot/cron operation.

---

## Quick Wins (Phase 0.9) `[NEW]`

Immediately-actionable items derived from R&D cycle findings. Each is <30 minutes, no dependencies, high impact.

> **Parallel workstreams:** All items independent. Target: complete in 1-2 loop iterations.

- [ ] **QW-1** — Fix JSON response format enforcement (25.7% retry rate, "not valid json" pattern seen 26 times across 15 cycles) `P0` `S`
  - File: `internal/session/loop_worker.go` — add JSON schema validation + retry with format reminder
  - **Acceptance:** JSON parse retry rate < 5%

- [ ] **QW-2** — Enable cascade routing by default (code exists in `internal/session/cascade.go`, fleet audit shows NOT configured) `P0` `S`
  - File: `.ralphrc` default + `internal/session/manager.go` — set `CASCADE_ENABLED=true` in defaults
  - **Acceptance:** New sessions use cascade routing without explicit config

- [ ] **QW-3** — Cap worker turns at 20 to prevent runaway sessions (FINDING-160: signal:killed, 3rd cycle recurrence) `P0` `S`
  - File: `internal/session/loop.go` — add `MaxWorkerTurns` with default 20
  - **Acceptance:** Sessions terminate cleanly at turn limit instead of being killed

- [ ] **QW-4** — Fix prompt_analyze score inflation (FINDING-240: scores cluster at 8-9/10 regardless of quality) `P1` `S`
  - File: `internal/enhancer/analyze.go` — recalibrate scoring rubric, add negative signal detection
  - **Acceptance:** Score distribution spans 3-9 range on test corpus

- [ ] **QW-5** — Fix prompt_enhance stage skipping transparency (FINDING-243: stages silently skipped) `P1` `S`
  - File: `internal/enhancer/pipeline.go` — add `SkippedStages` field to result, log skip reasons
  - **Acceptance:** Enhanced result includes list of skipped stages with reasons

- [ ] **QW-6** — Fix loop_gates zero-baseline bug (FINDING-226/238: baseline zero-init, 2nd cycle recurrence) `P0` `S`
  - File: `internal/session/loop_gates.go` — ensure baseline save errors propagate, initialize from first observation
  - **Acceptance:** `loop_gates` returns meaningful deltas on first run after baseline

- [ ] **QW-7** — Fix snapshot path saving to claudekit path (FINDING-148/268: 4th cycle recurrence) `P1` `S`
  - File: `internal/session/snapshot.go` — update path resolution to use ralphglasses project root
  - **Acceptance:** Snapshots save to `.ralph/snapshots/` not `claudekit/` path

- [ ] **QW-8** — Fix budget params silently ignored in session_launch (FINDING-258/261) `P0` `S`
  - File: `internal/mcpserver/tools_session.go` — wire budget_usd and max_turns params through to LaunchOptions
  - **Acceptance:** `session_launch budget_usd=5.0` actually enforces $5 budget

- [ ] **QW-9** — Persist autonomy level changes across restarts (FINDING-257) `P1` `S`
  - File: `internal/session/autooptimize.go` — write autonomy level to `.ralph/autonomy.json`
  - **Acceptance:** `autonomy_level` survives process restart

- [ ] **QW-10** — Fix relevance scoring flat at 0.5 for all results (research-audit FINDING) `P1` `M`
  - File: `internal/mcpserver/tools_roadmap.go` — implement actual TF-IDF or keyword overlap scoring
  - **Acceptance:** Relevance scores vary meaningfully (stddev > 0.15)

- [ ] **QW-11** — Clean phantom fleet work (73% stale, 109 phantom "001" repo entries) `P1` `S`
  - File: `internal/fleet/coordinator.go` — add stale task reaper, validate repo paths on submission
  - **Acceptance:** `fleet_status` shows 0 phantom entries after cleanup

- [ ] **QW-12** — Fix improvement_patterns.json rules always null `P2` `S`
  - File: `internal/session/reflexion.go` — implement rule extraction from positive/negative patterns
  - **Acceptance:** After 5+ cycles, `rules` array contains at least 1 learned rule

---

## Phase 0: Foundation (COMPLETE)

- [x] Go module (`github.com/hairglasses-studio/ralphglasses`)
- [x] Cobra CLI with `--scan-path` flag
- [x] Discovery engine — scan for `.ralph/` and `.ralphrc` repos
- [x] Model layer — parsers for status.json, progress.json, circuit breaker, .ralphrc
- [x] Process manager — launch/stop/pause ralph loops via os/exec with process groups
- [x] File watcher — fsnotify with 2s polling fallback
- [x] Log streamer — tail `.ralph/live.log`
- [x] MCP server — 10 core tools + 105 deferred (115 total across 13 namespaces + 2 meta-tools)
- [x] Standalone MCP binary (`cmd/ralphglasses-mcp/`)
- [x] TUI shell — BubbleTea app with k9s-style keymap
- [x] TUI views — overview table, repo detail, log stream, config editor, help
- [x] TUI components — sortable table, breadcrumb, status bar, notifications
- [x] Styles package — Lip Gloss theme (isolated to avoid import cycles)
- [x] Marathon launcher script (`marathon.sh`)
- [x] Discovery package — `internal/discovery/` scans for `.ralph/` and `.ralphrc` repos with context support `[reconciled 2026-03-26]`
- [x] Repo files package — `internal/repofiles/` validates and optimizes repo configuration files `[reconciled 2026-03-26]`

## Phase 0.5: Critical Fixes

Pre-requisite fixes for existing bugs and silent failures. No new features. All items are independent and can be worked in parallel.

> **Parallel workstreams:** All 0.5.x items are independent. No blockers between them.

### 0.5.1 — Silent error suppression in RefreshRepo (COMPLETE)
- [x] 0.5.1.1 — Return `[]error` from `RefreshRepo()` in `internal/model/status.go:49-54` instead of discarding with `_ =`
- [x] 0.5.1.2 — Propagate errors to TUI layer: emit `RefreshErrorMsg` with repo path and error details
- [x] 0.5.1.3 — Display parse errors in repo detail view status bar (non-blocking, yellow warning)
- [x] 0.5.1.4 — Add unit test: corrupt status.json -> RefreshRepo returns error, not silent zero-value
- **Acceptance:** parse errors in `.ralph/` files visible to user, not silently dropped

### 0.5.2 — Watcher error handling (COMPLETE)
- [x] 0.5.2.1 — Replace `return nil` on watcher error (`process/watcher.go:47-48`) with error propagation
- [x] 0.5.2.2 — Emit `WatcherErrorMsg` to TUI when fsnotify errors occur
- [x] 0.5.2.3 — Auto-fallback: on watcher error, switch to polling mode and notify user
- [x] 0.5.2.4 — Add exponential backoff on repeated watcher failures (max 30s)
- **Acceptance:** watcher failures visible in TUI, automatic fallback to polling

### 0.5.3 — Process reaper exit status (COMPLETE)
- [x] 0.5.3.1 — Capture `cmd.Wait()` error in `process/manager.go:59` goroutine
- [x] 0.5.3.2 — Parse exit code: distinguish crash (non-zero) from clean exit (0)
- [x] 0.5.3.3 — Emit `ProcessExitMsg{RepoPath, ExitCode, Error}` to TUI
- [x] 0.5.3.4 — Update repo status to "crashed" or "stopped" based on exit code
- [x] 0.5.3.5 — Add unit test: simulate ralph crash, assert TUI receives crash notification
- **Acceptance:** TUI correctly reports ralph crash vs clean stop

### 0.5.4 — Getpgid fallback safety (COMPLETE)
- [x] 0.5.4.1 — Log warning when `Getpgid` fails in `manager.go:78-82` (currently silent fallback to PID-only signal)
- [x] 0.5.4.2 — Track child PIDs: on process launch, record PID + all child PIDs if available
- [x] 0.5.4.3 — Fallback kill sequence: SIGTERM to PID -> wait 5s -> SIGTERM to known children -> wait 5s -> SIGKILL
- [x] 0.5.4.4 — Post-stop audit: check for orphaned processes matching `ralph_loop` pattern
- **Acceptance:** `Stop()` reliably kills all child processes, no orphans

### 0.5.5 — Distro path mismatch
- [x] 0.5.5.1 — Align `hw-detect.service` ExecStart path with Dockerfile install location (`/usr/local/bin/`)
- [x] 0.5.5.2 — Add path consistency check to `distro/Makefile`: validate service files reference correct paths
- [ ] 0.5.5.3 — Document path conventions in `distro/README.md`: scripts -> `/usr/local/bin/`, configs -> `/etc/ralphglasses/` `P2` `S`
- **Acceptance:** `hw-detect.service` starts successfully on first boot

### 0.5.6 — Grub AMD iGPU fallback
- [ ] 0.5.6.1 — Add GRUB menuentry for AMD iGPU boot: `nomodeset` removed, `amdgpu.dc=1` enabled `P2` `M`
- [ ] 0.5.6.2 — Add GRUB menuentry for headless/serial console boot `P2` `S`
- [ ] 0.5.6.3 — Set GRUB timeout to 5s (allow human intervention on boot failure) `P2` `S`
- [ ] 0.5.6.4 — Add `grub.cfg` validation to CI: parse all menuentry blocks, verify kernel image paths exist `P2` `M`
- **Acceptance:** system boots on AMD iGPU when NVIDIA unavailable

### 0.5.7 — Hardcoded version string (COMPLETE)
- [x] 0.5.7.1 — Define `var Version = "dev"` in `internal/version/version.go`
- [x] 0.5.7.2 — Replace hardcoded `"0.1.0"` in `cmd/mcp.go:19` and `cmd/ralphglasses-mcp/main.go:22`
- [x] 0.5.7.3 — Add `-ldflags "-X internal/version.Version=$(git describe)"` to build commands
- [x] 0.5.7.4 — Add `ralphglasses version` subcommand: print version, go version, build date, commit SHA
- [x] 0.5.7.5 — Display version in TUI help view and MCP server info
- **Acceptance:** `ralphglasses version` outputs correct git-derived version

### 0.5.8 — CI BATS guard (COMPLETE)
- [x] 0.5.8.1 — Guard BATS step in CI: check `scripts/test/` exists and contains `.bats` files before running
- [x] 0.5.8.2 — Add BATS install step to CI (install `bats-core` if not present)
- [x] 0.5.8.3 — Run `marathon.bats` in CI with mock ANTHROPIC_API_KEY
- [x] 0.5.8.4 — Add CI matrix: test on ubuntu-latest and macos-latest
- **Acceptance:** CI passes regardless of test directory presence

### 0.5.9 — Race condition in MCP scan (COMPLETE)
- [x] 0.5.9.1 — Add `sync.RWMutex` to protect `repos` map in `internal/mcpserver/` during concurrent scans
- [x] 0.5.9.2 — Add `go test -race` to CI pipeline for all packages
- [x] 0.5.9.3 — Write concurrent scan test: 10 goroutines scanning simultaneously
- **Acceptance:** `go test -race ./...` passes clean

### 0.5.10 — Marathon.sh edge cases
- [x] 0.5.10.1 — Add `bc` availability check at script start (budget calculation depends on it)
- [ ] 0.5.10.2 — Add disk space check before marathon start (warn if < 5GB free) `P1` `S`
- [ ] 0.5.10.3 — Fix infinite restart loop: cap MAX_RESTARTS, add cooldown between restarts `P1` `M`
- [ ] 0.5.10.4 — Add memory pressure monitoring: check `/proc/meminfo` AvailMem, warn at < 2GB `P2` `S`
- [ ] 0.5.10.5 — Add log rotation: rotate marathon logs at 100MB, keep last 3 `P2` `S`
- **Acceptance:** marathon.sh handles resource exhaustion gracefully

### 0.5.11 — Config validation strictness
- [ ] 0.5.11.1 — Define canonical key list with types: `internal/model/config_schema.go` `P1` `M`
- [ ] 0.5.11.2 — Warn on unknown keys in `.ralphrc` (typo detection) `P1` `S`
- [ ] 0.5.11.3 — Validate numeric ranges: `MAX_CALLS_PER_HOUR` must be 1-1000, `CB_COOLDOWN_MINUTES` must be 1-60 `P1` `S`
- [ ] 0.5.11.4 — Validate boolean values: only "true"/"false", reject "yes"/"1"/"on" `P2` `S`
- **Acceptance:** invalid `.ralphrc` values produce clear error messages

## Phase 0.6: Code Quality & Observability

Post-gate-pass improvements. All items are independent, parallel, and sized for single-iteration self-improvement loop completion.

> **Parallel workstreams:** All 0.6.x items are independent. No blockers between them.

### 0.6.1 — Test coverage for uncovered error paths
- [ ] 0.6.1.1 — Add tests for `internal/discovery/` error paths: unreadable dirs, symlink cycles, permission denied `P1` `M`
- [ ] 0.6.1.2 — Add tests for `internal/model/` corrupt file handling: truncated JSON, invalid UTF-8, zero-byte files `P1` `M`
- [ ] 0.6.1.3 — Add tests for `internal/process/` edge cases: double-stop, stop-before-start, signal delivery to exited process `P1` `M`
- [ ] 0.6.1.4 — Add tests for `internal/enhancer/` pipeline stages: empty input, extremely long input, unicode-heavy prompts `P1` `M`
- **Acceptance:** each new test exercises an error path that previously had no coverage

### 0.6.2 — Observation enrichment
- [ ] 0.6.2.1 — Add `GitDiffStat` field to `LoopObservation`: files changed, insertions, deletions from worker output `P1` `M`
- [ ] 0.6.2.2 — Add `PlannerModelUsed` and `WorkerModelUsed` fields to `LoopObservation` for provider tracking `P1` `S`
- [ ] 0.6.2.3 — Add `AcceptancePath` field to `LoopObservation`: "auto_merge", "pr", "rejected" for merge outcome tracking `P1` `M`
- [ ] 0.6.2.4 — Add observation summary helper: `SummarizeObservations([]LoopObservation) ObservationSummary` with aggregate stats `P1` `M`
- **Acceptance:** new fields populated in observations, summary helper has tests

### 0.6.3 — Loop configuration validation
- [ ] 0.6.3.1 — Add `ValidateLoopConfig(LoopConfig) []error` — validate all loop config fields before loop start `P0` `M`
- [ ] 0.6.3.2 — Validate model names against known provider models (claude-opus-4-6, claude-sonnet-4-6, gemini-2.5-pro, etc.) `P1` `S`
- [ ] 0.6.3.3 — Validate enhancement flags: warn if `enable_worker_enhancement=true` with non-Claude worker (no effect) `P1` `S`
- [ ] 0.6.3.4 — Add config validation call at loop start, return clear error before spawning any sessions `P0` `S`
- **Acceptance:** invalid loop configs rejected with descriptive errors before work begins

### 0.6.4 — Gate report formatting
- [ ] 0.6.4.1 — Add `FormatGateReport(*GateReport) string` — human-readable gate summary with pass/warn/fail coloring hints `P1` `M`
- [ ] 0.6.4.2 — Add `FormatGateReportMarkdown(*GateReport) string` — markdown table for scratchpad/PR descriptions `P1` `S`
- [ ] 0.6.4.3 — Add gate trend helper: `CompareGateReports(prev, current *GateReport) []GateTrend` showing improvement/regression per metric `P1` `M`
- [ ] 0.6.4.4 — Wire `FormatGateReport` into loop status output and MCP `loop_gates` tool response `P1` `S`
- **Acceptance:** gate reports render as readable tables, trend comparison shows metric direction

### 0.6.5 — Session timeout and stall detection
- [ ] 0.6.5.1 — Add `StallTimeout` field to `LoopConfig` (default: 10 minutes) — max time for a single iteration with no output `P0` `M`
- [ ] 0.6.5.2 — Implement stall detector in `StepLoop`: monitor worker session output timestamp, kill and retry on timeout `P0` `L`
- [ ] 0.6.5.3 — Add `StallCount` field to `LoopObservation` for tracking stall frequency `P1` `S`
- [ ] 0.6.5.4 — Add stall detection tests: mock session that produces no output, assert timeout triggers `P1` `M`
- **Acceptance:** stalled iterations detected and retried, stall count tracked in observations

### 0.6.6 — Worktree cleanup robustness
- [ ] 0.6.6.1 — Add `CleanupStaleWorktrees(repoRoot string, maxAge time.Duration) (int, error)` — remove worktrees older than maxAge `P1` `M`
- [ ] 0.6.6.2 — Add worktree lock file detection: skip cleanup if `.lock` file present (active worktree) `P1` `S`
- [ ] 0.6.6.3 — Call `CleanupStaleWorktrees` at loop start with 24h maxAge `P1` `S`
- [ ] 0.6.6.4 — Add `ralphglasses_worktree_cleanup` MCP tool for manual cleanup `P2` `M`
- **Acceptance:** stale worktrees cleaned up automatically, active worktrees preserved

### 0.6.7 — Planner task deduplication improvement
- [ ] 0.6.7.1 — Add Levenshtein/Jaccard similarity check to `isDuplicateTask`: catch near-duplicate titles (threshold 0.8) `P1` `M`
- [ ] 0.6.7.2 — Track completed task titles in observation history, reject re-proposals of already-completed work `P1` `M`
- [ ] 0.6.7.3 — Add `DedupReason` field to skipped tasks for debugging planner behavior `P2` `S`
- [ ] 0.6.7.4 — Add dedup tests: exact match, near-match, and distinct task pairs `P1` `M`
- **Acceptance:** planner doesn't re-propose completed or near-duplicate tasks

### Phase 0.7 — Codebase Hardening & Observability

- [ ] 0.7.1 — Observation enrichment: add GitDiffStat, model fields, AcceptancePath, StallCount to LoopObservation; add SummarizeObservations helper `P1` `L`
- [ ] 0.7.2 — Loop config validation: ValidateLoopConfig with model name, enhancement flag, and StallTimeout checks `P0` `M`
- [ ] 0.7.3 — Stall detection: StallDetector monitors worker output timestamps, kills + retries on timeout `P0` `L`
- [ ] 0.7.4 — Gate report formatting: FormatGateReport, FormatGateReportMarkdown, CompareGateReports `P1` `M`
- [ ] 0.7.5 — Gate report dedup + baseline fix: consolidate outputGateReport, fix swallowed baseline save errors `P0` `M`
- [ ] 0.7.6 — Event bus improvements: SubscribeFiltered, event type validation, schema versioning, async persistence `P1` `L`
- [ ] 0.7.7 — Provider cost rate config: externalize hardcoded rates to .ralph/cost_rates.json `P1` `S`
- [ ] 0.7.8 — Worktree cleanup robustness: age-based cleanup with lock file + uncommitted change detection `P1` `M`
- [ ] 0.7.9 — CLI os.Exit fix: replace os.Exit in RunE with sentinel errors for proper cobra handling `P1` `S`
- [ ] 0.7.10 — Planner task dedup: Jaccard similarity matching for near-duplicate task detection `P1` `M`
- [ ] 0.7.11 — Marathon resource monitoring: disk space, memory checks, log rotation `P2` `M`

## Phase 0.8: MCP Observability & Scratchpad Automation (COMPLETE)

New `observability` tool group (13th namespace, 11 tools). Replaces sleep anti-patterns,
adds programmatic scratchpad note-taking, surfaces observation/cost/coverage data via MCP.
Implements MCP spec features: structured output schemas, logging notifications.

> **Parallel workstreams:** 0.8.1-0.8.8 are independent (new files only). 0.8.9 wires registration.

### 0.8.1 — Observation query tools (COMPLETE)
- [x] `ralphglasses_observation_query`: filter/paginate loop observations by repo, hours, loop_id, status, provider
- [x] `ralphglasses_observation_summary`: aggregate stats via SummarizeObservations

### 0.8.2 — Scratchpad MCP tools (COMPLETE)
- [x] `ralphglasses_scratchpad_read`: read `.ralph/{name}_scratchpad.md`
- [x] `ralphglasses_scratchpad_append`: append markdown note with optional section header
- [x] `ralphglasses_scratchpad_list`: glob `.ralph/*_scratchpad.md`
- [x] `ralphglasses_scratchpad_resolve`: mark numbered item as resolved

### 0.8.3 — Loop wait/poll tools (COMPLETE)
- [x] `ralphglasses_loop_await`: blocking wait with timeout, replaces `sleep && echo done` anti-pattern
- [x] `ralphglasses_loop_poll`: non-blocking single status check

### 0.8.4 — Coverage report tool (COMPLETE)
- [x] `ralphglasses_coverage_report`: run `go test -coverprofile`, report per-package vs threshold

### 0.8.5 — Cost estimation tool (COMPLETE)
- [x] `ralphglasses_cost_estimate`: pre-launch cost estimate with historical calibration (60/40 blend)

### 0.8.6 — Merge verification tool (COMPLETE)
- [x] `ralphglasses_merge_verify`: sequential build->vet->test with 5-min timeout per step

### 0.8.7 — MCP logging integration (COMPLETE)
- [x] `MCPLogger` wrapping `*server.MCPServer` for `notifications/message` emission
- [x] `MCPLoggingMiddleware` returning `server.ToolHandlerMiddleware`
- [x] Falls back to slog when no MCP client connected

### 0.8.8 — Structured output schemas (COMPLETE)
- [x] `OutputSchemas` map for 6 high-value tools (observation_query, observation_summary, loop_benchmark, fleet_status, cost_estimate, coverage_report)
- [x] `SchemaForTool()` helper for integration

### 0.8.9 — Registration wiring & bookkeeping (COMPLETE)
- [x] `buildObservabilityGroup()` in tools_builders.go with 11 tool entries
- [x] `"observability"` added to ToolGroupNames
- [x] Test expectations updated (96->107 tools, 12->13 namespaces)
- [x] Scratchpad items #4, #5, #7, #20, #25, #26 marked RESOLVED

---

## Phase 1: Harden & Test

**Completed:**
- [x] Unit tests for all packages — 78.2% coverage (discovery, model, process, mcpserver)
- [x] TUI tests — 55.5% app coverage, view rendering, keymap, command/filter modes
- [x] CI pipeline — `go test`, `go vet`, `golangci-lint`, shellcheck, fuzz, benchmarks, BATS
- [x] Error handling — MCP scan error propagation, log stream errors, config key validation
- [x] Process manager — watcher timeout fix (no longer blocks event loop)
- [x] Config editor — key validation
- [x] End-to-end evaluation framework — `internal/e2e/` with baseline tracking, aggregate metrics, scenario stats, counterfactual analysis `[reconciled 2026-03-26]`

**Remaining (38 subtasks):**

> **Parallel workstreams:** 1.1 and 1.2 can proceed concurrently. 1.3 and 1.5 can proceed concurrently. 1.4 depends on 1.1 fixtures. 1.6 depends on all others. 1.7-1.10 can proceed in parallel with everything except 1.6.

### 1.1 — Integration test: full lifecycle (COMPLETE)
- [x] 1.1.1 — Create test fixture directory with `.ralph/` dir, mock `status.json`, and dummy `.ralphrc` — `internal/integration/helpers_test.go` `[reconciled 2026-03-26]`
- [x] 1.1.2 — Write mock `ralph_loop.sh` that simulates loop lifecycle (start, write status, exit) `[reconciled 2026-03-26]`
- [x] 1.1.3 — Implement lifecycle test: scan -> start -> poll status -> stop, assert state transitions — `internal/integration/lifecycle_test.go` `[reconciled 2026-03-26]`
- [x] 1.1.4 — Add `//go:build integration` tag and CI gate (`go test -tags=integration`) `[reconciled 2026-03-26]`
- **Acceptance:** `go test -tags=integration` passes end-to-end lifecycle

### 1.2 — MCP server hardening (COMPLETE)
- [x] 1.2.1 — Audit all shared state in `mcpserver`; add `sync.RWMutex` around `repos` map and scan results — `tools.go` line 41 `[reconciled 2026-03-26]`
- [x] 1.2.2 — Migrate all `log.Printf` calls to `slog` with structured fields (tool name, repo path, duration) — zero `log.Printf` remain in mcpserver `[reconciled 2026-03-26]`
- [x] 1.2.3 — Add request validation: reject empty repo paths, unknown config keys, malformed JSON — `internal/mcpserver/validate.go` `[reconciled 2026-03-26]`
- [x] 1.2.4 — Define MCP error codes (not-found, invalid-input, internal) and return structured errors — `internal/mcpserver/errors.go` with 17 error codes `[reconciled 2026-03-26]`
- **Acceptance:** no data races under `go test -race`, structured JSON log output

### 1.2.5 — MCP Handler Framework
- [ ] 1.2.5.1 — Extract ParamParser helper: type-safe parameter extraction with validation, replacing manual `getStringArg`/`getNumberArg` calls across 81 handlers `P1` `L`
- [ ] 1.2.5.2 — Standardize error codes across all handlers: migrate from `errResult()` to `errCode()` with consistent error_code values (invalid_params, not_found, internal_error) `P1` `L`
- [ ] 1.2.5.3 — Handler test harness: mock Server with mock providers for table-driven tests, reducing per-handler test boilerplate `P1` `M`
- [ ] 1.2.5.4 — Handler generator: codegen tool for new MCP tool scaffolding (registration + handler + test stub) `[BLOCKED BY 1.2.5.1, 1.2.5.2]` `P2` `M`
- **Acceptance:** new handler scaffolding requires <10 LOC, all 81 handlers use ParamParser, zero raw `errResult()` calls remain

### 1.3 — TUI polish
- [x] 1.3.1 — Build `ConfirmDialog` component (y/n prompt overlay, reusable across views) — `internal/tui/components/confirm.go` `[reconciled 2026-03-26]`
- [x] 1.3.2 — Wire confirm dialog to destructive actions: stop, stop_all, config delete — wired in handlers_detail.go, handlers_loops.go, handlers_common.go `[reconciled 2026-03-26]`
- [ ] 1.3.3 — Add SIGINT/SIGTERM shutdown handler: stop all managed processes, flush logs, clean exit `P0` `M`
- [ ] 1.3.4 — Audit scroll bounds in log stream and table views; fix off-by-one on terminal resize `P1` `M`
- **Acceptance:** destructive actions require y/n, clean exit on signals, no scroll panics on resize

### 1.4 — Process manager improvements
- [ ] 1.4.1 — Define PID file format (JSON: pid, start_time, repo_path) and write on process launch `[BLOCKED BY 1.1.1]` `P1` `M`
- [ ] 1.4.2 — Implement orphan scanner: on startup, check PID files against running processes, clean stale entries `P1` `M`
- [ ] 1.4.3 — Add restart policy to `.ralphrc` (`RESTART_ON_CRASH=true`, `MAX_RESTARTS=3`, `RESTART_DELAY_SEC=5`) `P1` `M`
- [ ] 1.4.4 — Implement health check loop: poll process status every 5s, trigger restart or circuit breaker on repeated failures `P1` `L`
- **Acceptance:** survives ralph crash with auto-restart, no orphan processes after TUI exit

### 1.5 — Config editor enhancements
- [ ] 1.5.1 — Add key CRUD operations: insert new key, rename key, delete key from TUI `P2` `M`
- [ ] 1.5.2 — Wire fsnotify on `.ralphrc` file; reload config on external change, emit notification `P1` `M`
- [ ] 1.5.3 — Add validation rules per key type (numeric ranges, boolean, enum values) `P1` `M`
- [ ] 1.5.4 — Implement undo buffer (single-level: revert last edit) `P2` `S`
- **Acceptance:** external edits reflected without restart, invalid values rejected with message

### 1.6 — Test coverage targets
- [ ] 1.6.1 — Set per-package coverage targets: discovery 90%, model 90%, process 85%, mcpserver 85%, tui 70% `P1` `S`
- [ ] 1.6.2 — Add CI enforcement step: `go test -coverprofile` -> parse -> fail if below threshold `P1` `M`
- [ ] 1.6.3 — Add coverage badge to README via codecov or go-cover-treemap `P2` `S`
- [ ] 1.6.4 — Write missing tests to reach 85%+ overall (focus on untested error paths) `P1` `L`
- **Acceptance:** `go test -coverprofile` meets thresholds in CI, badge visible in README

### 1.7 — Structured logging
- [x] 1.7.1 — Replace all `log.Printf` calls in `internal/mcpserver/` with `slog.Info`/`slog.Error` — zero `log.Printf` remain `[reconciled 2026-03-26]`
- [x] 1.7.2 — Replace all `log.Printf` calls in `internal/process/` with structured `slog` — uses `slog` in manager, lifecycle, orphans `[reconciled 2026-03-26]`
- [ ] 1.7.3 — Add `--log-level` flag to CLI: debug, info, warn, error (default: info) `P1` `S`
- [ ] 1.7.4 — Add `--log-format` flag: text (default for TTY), json (default for non-TTY) `P1` `S`
- [ ] 1.7.5 — Add request-scoped fields: inject `slog.Group("request", ...)` with tool name, repo path, duration `P1` `M`
- **Acceptance:** all log output is structured `slog`, configurable level and format

### 1.8 — Custom error types `[BLOCKED BY 0.5.1]`
- [ ] 1.8.1 — Define sentinel errors in `internal/model/`: `ErrStatusNotFound`, `ErrConfigParseFailed`, `ErrCircuitOpen` `P1` `S`
- [x] 1.8.2 — Define sentinel errors in `internal/process/`: `ErrAlreadyRunning`, `ErrNotRunning`, `ErrNoLoopScript` — `internal/process/errors.go` `[reconciled 2026-03-26]`
- [ ] 1.8.3 — Wrap all `fmt.Errorf` with `%w` verb for proper `errors.Is()` / `errors.As()` support `P1` `M`
- [ ] 1.8.4 — Create `internal/errors/` package with error classification: transient, permanent, user-facing `P1` `M`
- [ ] 1.8.5 — Add error type assertions in MCP handlers: map error types to MCP error codes `P1` `M`
- **Acceptance:** callers can use `errors.Is()` and `errors.As()` on all returned errors

### 1.9 — Context propagation
- [x] 1.9.1 — Thread `context.Context` through `discovery.Scan()` — support cancellation of long scans `[reconciled 2026-03-26]`
- [ ] 1.9.2 — Thread `context.Context` through `model.Load*()` functions — timeout on stuck file reads `P1` `M`
- [ ] 1.9.3 — Use incoming `ctx` in MCP tool handlers (currently received but ignored) `P0` `M`
- [ ] 1.9.4 — Add `--scan-timeout` flag: max time for initial directory scan (default: 30s) `P1` `S`
- [ ] 1.9.5 — Wire context cancellation to TUI shutdown: cancel in-flight operations on exit `P1` `M`
- **Acceptance:** all long-running operations respect context cancellation

### 1.10 — TUI bounds safety
- [ ] 1.10.1 — Fix SortCol out-of-bounds: clamp `SortCol` to valid range when columns change `P0` `S`
- [ ] 1.10.2 — Add search UI to LogView: `/` to enter search, `n`/`N` for next/prev match `P2` `M`
- [ ] 1.10.3 — Audit all slice access in TUI components for empty-slice panics `P0` `M`
- [ ] 1.10.4 — Add fuzz tests for table rendering with random column counts and data `P2` `M`
- [ ] 1.10.5 — Handle zero-height terminal gracefully (don't panic, show "terminal too small") `P1` `S`
- **Acceptance:** no panics on edge-case terminal sizes or empty data

## Phase 1.5: Developer Experience

Tooling, release automation, and contributor workflow. All items independent of Phase 1.

> **Parallel workstreams:** All 1.5.x items are independent except 1.5.2 depends on 0.5.7 (version ldflags).

- [x] Plugin system — `internal/plugin/` with hashicorp/go-plugin gRPC interface, provider plugins, lifecycle management `[reconciled 2026-03-26]`
- [x] Batch API framework — `internal/batch/` with multi-provider batch submission (Claude, Gemini, OpenAI) `[reconciled 2026-03-26]`

### 1.5.1 — Shell completions
- [ ] 1.5.1.1 — Add `ralphglasses completion bash|zsh|fish` via cobra built-in `GenBashCompletionV2` `P1` `S`
- [ ] 1.5.1.2 — Add dynamic completions for `--scan-path` (directory completion) `P2` `S`
- [ ] 1.5.1.3 — Add dynamic completions for repo names in `status`, `start`, `stop` subcommands `P2` `M`
- [ ] 1.5.1.4 — Add install instructions for each shell to `docs/completions.md` `P2` `S`
- [ ] 1.5.1.5 — Package completions in release artifacts (`.deb` installs to `/usr/share/bash-completion/`) `P2` `M`
- **Acceptance:** `ralphglasses <tab>` completes subcommands and repo names

### 1.5.2 — Release automation `[BLOCKED BY 0.5.7]`
- [x] 1.5.2.1 — Add `.goreleaser.yaml`: multi-arch builds (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64) `[reconciled 2026-03-26]`
- [x] 1.5.2.2 — GitHub Actions release workflow: tag push -> goreleaser -> GitHub Release with binaries — `.github/workflows/release.yml` `[reconciled 2026-03-26]`
- [ ] 1.5.2.3 — Add changelog generation: `goreleaser` changelog from conventional commits `P2` `S`
- [ ] 1.5.2.4 — Add Docker image build: `ghcr.io/hairglasses-studio/ralphglasses` multi-arch manifest `P2` `M`
- [ ] 1.5.2.5 — Add Homebrew tap: `hairglasses-studio/homebrew-tap` with goreleaser auto-update `P2` `M`
- [ ] 1.5.2.6 — Add AUR package: `PKGBUILD` for Arch Linux users `P2` `S`
- **Acceptance:** `git tag v0.2.0 && git push --tags` produces release with binaries, Docker image, Homebrew formula

### 1.5.3 — Pre-commit hooks
- [ ] 1.5.3.1 — Add `.pre-commit-config.yaml`: golangci-lint, gofumpt, shellcheck, markdownlint `P2` `S`
- [x] 1.5.3.2 — Add `Makefile` with targets: `lint`, `test`, `build`, `install`, `bench`, `fuzz`, and more `[reconciled 2026-03-26]`
- [ ] 1.5.3.3 — Add EditorConfig (`.editorconfig`) for consistent formatting across editors `P2` `S`
- [x] 1.5.3.4 — Add `CONTRIBUTING.md` with setup instructions and PR guidelines (281 lines) `[reconciled 2026-03-26]`
- **Acceptance:** `pre-commit run --all-files` passes clean

### 1.5.4 — Config schema documentation
- [ ] 1.5.4.1 — Write `docs/ralphrc-reference.md`: all keys, types, defaults, descriptions, examples `P2` `M`
- [ ] 1.5.4.2 — Add `ralphglasses config list-keys` subcommand: print all known keys with defaults `P2` `S`
- [ ] 1.5.4.3 — Add `ralphglasses config validate` subcommand: check `.ralphrc` against schema `P1` `S`
- [ ] 1.5.4.4 — Add `ralphglasses config init` subcommand: generate `.ralphrc` with all keys and defaults `P2` `S`
- [ ] 1.5.4.5 — Auto-generate config docs from schema (Go code -> Markdown via `go:generate`) `P2` `M`
- **Acceptance:** `ralphglasses config list-keys` outputs all valid configuration keys

### 1.5.5 — Man page generation
- [ ] 1.5.5.1 — Add `//go:generate` directive: `cobra/doc.GenManTree` for all subcommands `P2` `S`
- [ ] 1.5.5.2 — Include man pages in release artifacts (`.tar.gz` includes `man/`) `P2` `S`
- [ ] 1.5.5.3 — Add `make install-man` target: copy to `/usr/local/share/man/man1/` `P2` `S`
- **Acceptance:** `man ralphglasses` works after install

### 1.5.6 — Multi-arch builds
- [ ] 1.5.6.1 — Add arm64 cross-compilation to CI matrix (linux/arm64 for Raspberry Pi) `P2` `M`
- [ ] 1.5.6.2 — Test arm64 binary in QEMU user-mode emulation in CI `P2` `M`
- [ ] 1.5.6.3 — Add `GOARCH=arm64` smoke test: build + run `--help` + exit `P2` `S`
- [ ] 1.5.6.4 — Document Raspberry Pi thin client setup in `docs/raspberry-pi.md` `P2` `S`
- **Acceptance:** arm64 binary runs on Raspberry Pi 4/5

### 1.5.7 — Nix flake (optional)
- [ ] 1.5.7.1 — Add `flake.nix` with `buildGoModule` + dev shell (Go, golangci-lint, shellcheck) `P2` `M`
- [ ] 1.5.7.2 — Add NixOS module: systemd service, option types for config `P2` `L`
- [ ] 1.5.7.3 — Add `flake.lock` and CI check: `nix build` + `nix flake check` `P2` `S`
- **Acceptance:** `nix run github:hairglasses-studio/ralphglasses` works

### 1.5.8 — Development containers
- [ ] 1.5.8.1 — Add `.devcontainer/devcontainer.json`: Go + tools, port forwarding, recommended extensions `P2` `S`
- [ ] 1.5.8.2 — Add `.devcontainer/Dockerfile`: Go 1.26+, golangci-lint, BATS, shellcheck `P2` `S`
- [ ] 1.5.8.3 — GitHub Codespaces support: verify `go build ./...` and `go test ./...` work `P2` `M`
- **Acceptance:** `devcontainer up` provides working dev environment

### 1.5.9 — Documentation site
- [ ] 1.5.9.1 — Add `docs/` site with mdBook or mkdocs: getting started, architecture, API reference `P2` `L`
- [ ] 1.5.9.2 — Add GitHub Actions: build docs on push, deploy to GitHub Pages `P2` `M`
- [ ] 1.5.9.3 — Add architecture diagrams: mermaid flowcharts for data flow, component relationships `P2` `M`
- [ ] 1.5.9.4 — Add MCP tool API reference: auto-generated from tool handler docstrings `P2` `L`
- **Acceptance:** docs site live at `hairglasses-studio.github.io/ralphglasses`

### 1.5.10 — Charmbracelet v2 migration
- [ ] 1.5.10.1 — Migrate to Bubble Tea v2 (`charm.land/bubbletea/v2`): synchronized rendering (eliminates tearing), clipboard (OSC52), GraphicsMode, declarative Views API `P1` `XL`
- [ ] 1.5.10.2 — Migrate to Lip Gloss v2 (`charm.land/lipgloss/v2`): deterministic styles (explicit `isDark` bool), explicit I/O control, SSH/Wish compat `P1` `L`
- [ ] 1.5.10.3 — Update bubbles components for v2 API changes (table, viewport, list, textinput) `P1` `L`
- [ ] 1.5.10.4 — Adopt Lip Gloss v2 `table`, `tree`, `list` packages for fleet dashboard `P2` `M`
- [ ] 1.5.10.5 — Evaluate ntcharts streaming charts for real-time fleet health graphs `P2` `M`
- **Acceptance:** All 18 TUI views render without tearing; clipboard copy works; `go build ./...` clean

> **Breaking changes:** Bubble Tea v2 uses ncurses-based renderer. Lip Gloss v2 removes auto-detection side effects. Both import paths change. Must migrate together. See [Charm v2 blog](https://charm.land/blog/v2/).

### 1.5.11 — mcp-go → official SDK migration
- [ ] 1.5.11.1 — Evaluate `modelcontextprotocol/go-sdk` v1.4.1 feature parity with mcp-go v0.45.0 `P1` `M`
- [ ] 1.5.11.2 — Migrate `internal/mcpserver/` tool registration from mcp-go to official SDK `P1` `XL`
- [ ] 1.5.11.3 — Migrate transport layer (stdio + add streamable HTTP support) `P1` `L`
- [ ] 1.5.11.4 — Add OAuth support for remote MCP server mode `P2` `L`
- **Acceptance:** All 115 tools register and pass integration tests with official SDK

### 1.5.12 — Benchmarking infrastructure
- [ ] 1.5.12.1 — Add Go benchmarks for hot paths: `RefreshRepo`, `Scan`, `LoadStatus`, table rendering `P1` `M`
- [ ] 1.5.12.2 — Add `benchstat` comparison in CI: detect performance regressions between commits `P1` `M`
- [ ] 1.5.12.3 — Add benchmark dashboard: track p50/p99 latencies over time `P2` `L`
- [ ] 1.5.12.4 — Add memory allocation benchmarks: `b.ReportAllocs()` on all benchmark functions `P1` `S`
- **Acceptance:** CI fails on >10% performance regression

## Phase 2: Multi-Session Fleet Management

> **Depends on:** Phase 1 (concurrency guards, process manager improvements)
>
> **Parallel workstreams:** 2.1 (data model) is the foundation — most items depend on it. 2.6 (notifications) and 2.7 (tmux) are independent of each other and can proceed after 2.1. 2.9 (CLI) is independent of TUI work. 2.10 (marathon port) is fully independent. 2.11-2.14 are independent.

- [x] Fleet management package — `internal/fleet/` with A2A agent cards, task offers, worker pool, DLQ, budget enforcement (38 files) `[reconciled 2026-03-26]`
- [x] Eval framework — `internal/eval/` with Bayesian A/B testing, anomaly detection, changepoint analysis, counterfactual evaluation `[reconciled 2026-03-26]`

### 2.1 — Session data model
- [ ] 2.1.1 — Define `Session` struct: ID, repo path, worktree path, PID, budget, model, status, created_at, updated_at `P0` `M`
- [ ] 2.1.2 — Add SQLite via `modernc.org/sqlite`: schema migrations, connection pool, WAL mode `P0` `L`
- [ ] 2.1.3 — Implement Session CRUD: Create, Get, List, Update, Delete with prepared statements `P0` `M`
- [ ] 2.1.4 — Implement lifecycle state machine: `created -> running -> paused -> stopped -> archived` with valid transition enforcement `P0` `M`
- [ ] 2.1.5 — Add session event log table: state changes, errors, budget events with timestamps `P1` `M`
- **Acceptance:** sessions survive TUI restart, queryable via SQL

### 2.2 — Git worktree orchestration `[BLOCKED BY 2.1]`
- [ ] 2.2.1 — Create `internal/worktree/` package: wrapping `git worktree add/list/remove` `P0` `M`
- [ ] 2.2.2 — Auto-create worktree on session launch: branch naming convention `ralph/<session-id>` `P0` `M`
- [ ] 2.2.3 — Implement merge-back: `git merge --no-ff` with conflict detection and abort-on-conflict option `P0` `L`
- [ ] 2.2.4 — Add worktree cleanup on session stop/archive (remove worktree dir, prune) `P1` `S`
- [ ] 2.2.5 — Handle edge cases: dirty worktree on stop, orphaned branches, worktree path conflicts `P1` `M`
- **Acceptance:** `ralphglasses worktree create <repo>` produces isolated worktree, merge-back detects conflicts

### 2.3 — Budget tracking `[BLOCKED BY 2.1]`
- [ ] 2.3.1 — Per-session spend poller: read `session_spend_usd` from `.ralph/status.json` on watcher tick `P0` `M`
- [ ] 2.3.2 — Implement global budget pool: total ceiling, per-session allocation, remaining calculation `P0` `M`
- [ ] 2.3.3 — Add threshold alerts at 50%, 75%, 90% — emit BubbleTea message for TUI notification `P1` `S`
- [ ] 2.3.4 — Auto-pause session at budget ceiling: send SIGSTOP, update session state `P0` `M`
- [ ] 2.3.5 — Port budget tracking patterns from `mcpkit/finops` (cost ledger, rate calculation) `P1` `M`
- **Acceptance:** session auto-pauses when budget exhausted, alerts visible in TUI

### 2.4 — Fleet dashboard TUI view `[BLOCKED BY 2.1]`
- [ ] 2.4.1 — Create `ViewFleet` in view stack with aggregate session table `P1` `M`
- [ ] 2.4.2 — Columns: session name, repo, status, spend, loop count, model, uptime — sortable `P1` `M`
- [ ] 2.4.3 — Live-update via watcher ticks: refresh spend/status/loop count per row `P1` `M`
- [ ] 2.4.4 — Inline actions from fleet view: start/stop/pause selected session via keybinds `P1` `M`
- [ ] 2.4.5 — Add fleet summary bar: total sessions, running count, total spend, aggregate throughput `P1` `S`
- **Acceptance:** fleet view shows all sessions with live-updating spend/status

### 2.5 — Session launcher `[BLOCKED BY 2.1, 2.2, 2.3]`
- [ ] 2.5.1 — Implement `:launch` command: pick repo from discovered list, set session name `P1` `M`
- [ ] 2.5.2 — Add budget/model selection to launch flow: dropdown or tab-complete for model, numeric input for budget `P1` `M`
- [ ] 2.5.3 — Default budget from `.ralphrc` (`RALPH_SESSION_BUDGET`) or global config fallback `P1` `S`
- [ ] 2.5.4 — Session templates: save current launch config as named template, load from template `P2` `M`
- [ ] 2.5.5 — Validate launch preconditions: repo exists, no conflicting worktree, budget available in pool `P1` `M`
- **Acceptance:** can launch a named session with budget from TUI command mode

### 2.6 — Notification system `[PARALLEL — independent after 2.1]`
- [ ] 2.6.1 — Desktop notification abstraction: `freedesktop.org` D-Bus (Linux), `osascript` (macOS) `P2` `M`
- [ ] 2.6.2 — Define event types: session_complete, budget_warning, circuit_breaker_trip, crash, restart `P2` `S`
- [ ] 2.6.3 — Add `.ralphrc` config keys: `NOTIFY_DESKTOP=true`, `NOTIFY_SOUND=true` `P2` `S`
- [ ] 2.6.4 — Implement notification dedup/throttle: no repeat within 60s for same event type + session `P2` `M`
- **Acceptance:** desktop notification fires on circuit breaker trip

### 2.7 — tmux integration `[PARALLEL — independent after 2.1]`
- [ ] 2.7.1 — `internal/tmux/` package: create/list/kill sessions, name windows, attach/detach `P2` `M`
- [ ] 2.7.2 — One tmux pane per agent session: auto-create on session launch, name = session ID `P2` `M`
- [ ] 2.7.3 — `ralphglasses tmux` subcommand: `list`, `attach <session>`, `detach` `P2` `S`
- [ ] 2.7.4 — Headless mode: detect no TTY -> auto-use tmux instead of TUI `P1` `M`
- [ ] 2.7.5 — Port patterns from claude-tools (WSL-native tmux management, `/mnt/c/` path handling) `P2` `S`
- **Acceptance:** `ralphglasses tmux list` shows active sessions, `attach` works

### 2.8 — MCP server expansion `[BLOCKED BY 2.1, 2.2, 2.3]`
- [x] 2.8.1 — Add `ralphglasses_session_launch` tool: accepts repo, budget, model, name — implemented as `session_launch` `[reconciled 2026-03-26]`
- [x] 2.8.2 — Add `ralphglasses_session_list` tool: returns all sessions with status `[reconciled 2026-03-26]`
- [ ] 2.8.3 — Add `ralphglasses_worktree_create` tool: create worktree for repo `P1` `M`
- [x] 2.8.4 — Add `ralphglasses_session_budget` tool: per-session budget info `[reconciled 2026-03-26]`
- [x] 2.8.5 — Add `ralphglasses_fleet_status` tool: aggregate stats for agent-to-agent coordination `[reconciled 2026-03-26]`
- **Acceptance:** MCP tools callable from Claude Code, session lifecycle works end-to-end

### 2.9 — CLI subcommands
- [ ] 2.9.1 — `ralphglasses session list|start|stop|status` — non-TUI session management `P1` `M`
- [ ] 2.9.2 — `ralphglasses worktree create|list|merge|clean` — worktree operations from CLI `P1` `M`
- [ ] 2.9.3 — `ralphglasses budget status|set|reset` — budget management from CLI `P2` `S`
- [ ] 2.9.4 — JSON output flag (`--json`) for all subcommands for scripting/piping `P1` `S`
- **Acceptance:** all fleet operations available without TUI, JSON output parseable by `jq`

### 2.10 — Marathon.sh Go port `[PARALLEL — fully independent]`
- [ ] 2.10.1 — Port `marathon.sh` to `internal/marathon/` package: duration limit, budget limit, checkpoints `P1` `L`
- [ ] 2.10.2 — `ralphglasses marathon` subcommand: `--budget`, `--duration`, `--checkpoint-interval` `P1` `M`
- [ ] 2.10.3 — Replace shell signal handling with Go `os/signal` (SIGINT/SIGTERM -> graceful shutdown) `P1` `M`
- [ ] 2.10.4 — Git checkpoint tagging in Go: `git tag marathon-<timestamp>` at configurable interval `P1` `S`
- [ ] 2.10.5 — Structured marathon logging via `slog` (replace bash `log()` function) `P1` `S`
- **Acceptance:** `ralphglasses marathon` replaces `bash marathon.sh` with identical behavior

### 2.11 — Health check endpoint `[PARALLEL]`
- [ ] 2.11.1 — Add optional `--http-addr` flag (default: disabled, e.g. `:9090`) `P2` `S`
- [ ] 2.11.2 — Implement `/healthz` endpoint: returns 200 if process alive, 503 if shutting down `P2` `S`
- [ ] 2.11.3 — Implement `/readyz` endpoint: returns 200 if scan complete and sessions loaded `P2` `S`
- [ ] 2.11.4 — Implement `/metrics` stub: placeholder for Prometheus endpoint (wired in Phase 6) `P2` `S`
- [ ] 2.11.5 — Add systemd watchdog integration: `sd_notify` READY and WATCHDOG signals `P2` `M`
- **Acceptance:** `curl localhost:9090/healthz` returns 200 when TUI is running

### 2.12 — Telemetry opt-in `[PARALLEL]`
- [ ] 2.12.1 — Define telemetry event schema: session_start, session_stop, crash, budget_hit, circuit_trip `P2` `S`
- [ ] 2.12.2 — Local JSONL file writer: append events to `~/.ralphglasses/telemetry.jsonl` `P2` `M`
- [ ] 2.12.3 — Add `--telemetry` flag and `TELEMETRY_ENABLED` config key (default: off) `P2` `S`
- [ ] 2.12.4 — Optional remote POST: send anonymized events to configurable endpoint `P2` `M`
- [ ] 2.12.5 — Add `ralphglasses telemetry export` subcommand: export telemetry as CSV/JSON `P2` `S`
- **Acceptance:** telemetry events written to local file when opt-in enabled

### 2.13 — Plugin system `[PARALLEL]`
- [x] 2.13.1 — Define plugin interface: `Plugin{ Name(), Init(ctx), OnEvent(event), Shutdown() }` — implemented in `internal/plugin/interfaces.go` `[reconciled 2026-03-26]`
- [x] 2.13.2 — Plugin discovery via hashicorp/go-plugin gRPC protocol (`internal/plugin/grpc.go`) `[reconciled 2026-03-26]`
- [ ] 2.13.3 — Built-in plugin: `notify-desktop` (extract from 2.6 as reference implementation) `P2` `M`
- [ ] 2.13.4 — Plugin lifecycle: load on startup, unload on shutdown, hot-reload on SIGHUP `P2` `M`
- [ ] 2.13.5 — Plugin config: per-plugin config section in `.ralphrc` (e.g. `PLUGIN_NOTIFY_DESKTOP_SOUND=true`) `P2` `S`
- **Acceptance:** external plugin loaded and receives session events

### 2.14 — SSH remote management `[PARALLEL]`
- [ ] 2.14.1 — `ralphglasses remote add <name> <host>` — register remote thin client `P2` `M`
- [ ] 2.14.2 — `ralphglasses remote status` — SSH into registered hosts, collect session status `P2` `M`
- [ ] 2.14.3 — `ralphglasses remote start <host> <repo>` — start ralph loop on remote host `P2` `M`
- [ ] 2.14.4 — Aggregate remote sessions into fleet view (poll via SSH tunnel) `P2` `L`
- [ ] 2.14.5 — SSH key management: `~/.ralphglasses/ssh/` with per-host key configuration `P2` `M`
- **Acceptance:** fleet view shows sessions from multiple physical machines

## Phase 2.5: Multi-LLM Agent Orchestration

> **Depends on:** Phase 2.1 (session model)
>
> **Parallel workstreams:** 2.5.1 (provider fixes) is foundation. 2.5.2-2.5.5 depend on it. 2.5.6 is independent.

- [x] Awesome-list analyzer — `internal/awesome/` fetches, indexes, diffs, and reports on awesome-list repos for competitive intelligence (15 files) `[reconciled 2026-03-26]`
- [x] Multi-armed bandit provider selection — `internal/bandit/` with UCB1 selector, policy framework, arm tracking for dynamic provider routing `[reconciled 2026-03-26]`
- [x] Blackboard shared state — `internal/blackboard/` with CAS-based key-value store for cross-agent coordination `[reconciled 2026-03-26]`

### 2.5.1 — Fix provider CLI command builders (COMPLETE)
- [x] 2.5.1.1 — Fix buildCodexCmd: `codex exec PROMPT --json --full-auto` (not `--quiet`)
- [x] 2.5.1.2 — Fix buildGeminiCmd: add `-p` flag and `--yolo` for headless mode
- [x] 2.5.1.3 — Fix Codex prompt delivery (positional arg, not stdin)
- [x] 2.5.1.4 — Fix npm package name in docs (`@google/gemini-cli`)
- [x] 2.5.1.5 — Update provider test suite for correct CLI flags
- **Acceptance:** codex and gemini sessions launchable via MCP tools

### 2.5.2 — Per-provider agent discovery (COMPLETE)
- [x] 2.5.2.1 — Discover `.gemini/agents/*.md` for Gemini provider
- [x] 2.5.2.2 — Parse `AGENTS.md` sections for Codex provider
- [x] 2.5.2.3 — Add `Provider` field to `AgentDef` type
- [x] 2.5.2.4 — Wire provider param into `agent_list` and `agent_define` MCP tools
- **Acceptance:** `agent_list` returns provider-specific agent definitions

### 2.5.3 — Cross-provider team delegation (COMPLETE)
- [x] 2.5.3.1 — Add per-task provider override in `TeamTask`
- [x] 2.5.3.2 — Generate provider-aware delegation prompts for lead sessions
- [x] 2.5.3.3 — Update `team_create` with `worker_provider` default param
- [x] 2.5.3.4 — Update `team_delegate` with optional `provider` param
- **Acceptance:** Claude lead delegates tasks to Gemini/Codex workers

### 2.5.4 — Provider-specific resume support (COMPLETE)
- [x] 2.5.4.1 — Document Codex resume as unsupported and route retries through `session_retry`
- [x] 2.5.4.2 — Verify Gemini `--resume` flag works with `stream-json`
- [x] 2.5.4.3 — Add resume tests per provider, including explicit Codex rejection
- **Acceptance:** `session_resume` works for Claude/Gemini and returns an explicit validation error for Codex

### 2.5.5 — Unified cost normalization `[BLOCKED BY 2.5.1]`
- [x] 2.5.5.1 — Verify Codex `--json` cost output fields, update normalizer
- [x] 2.5.5.2 — Verify Gemini `stream-json` cost output fields, update normalizer
- [ ] 2.5.5.3 — Add provider-specific cost fallback (parse stderr for cost if not in JSON) `P1` `M`
- **Acceptance:** `cost_usd` tracked accurately for all providers

### 2.5.6 — Batch API integration `[PARALLEL — independent]`
- [x] 2.5.6.1 — Research: map batch API endpoints for Claude, Gemini, OpenAI (~50% cost) `[reconciled 2026-03-26]`
- [ ] 2.5.6.2 — Add `BatchOptions` to `LaunchOptions` (batch mode flag, callback URL) `P1` `M`
- [x] 2.5.6.3 — Implement batch submission for Claude (Messages Batches API) — `internal/batch/claude.go` `[reconciled 2026-03-26]`
- [x] 2.5.6.4 — Implement batch submission for Gemini (Batch Prediction API) — `internal/batch/gemini.go` `[reconciled 2026-03-26]`
- [ ] 2.5.6.5 — Implement batch polling/webhook for result collection `P1` `L`
- **Acceptance:** batch tasks submitted and results collected for at least one provider

## Phase 2.75: Architecture & Capability Extensions (COMPLETE)

Built across multiple implementation sessions. Extends the TUI, MCP server, and internal architecture with event-driven patterns, new tools, and interactive components.

### 2.75.1 — TUI Polish & Distribution (COMPLETE)
- [x] 4-tab layout with bubbles tab bar (Repos, Sessions, Teams, Fleet)
- [x] Sparkline charts via ntcharts for cost trends
- [x] 4 themes: k9s (default), dracula, nord, solarized (`internal/tui/styles/theme.go`)
- [x] Desktop notifications — macOS `osascript`, Linux `notify-send` (`internal/notify/`)
- [x] GoReleaser config (`.goreleaser.yaml`)
- [x] Diff view for repo git changes (`internal/tui/views/diffview.go`)

### 2.75.2 — Event Bus & Hook System (COMPLETE)
- [x] Internal pub/sub event bus (`internal/events/bus.go`) with ring buffer history (1000 events)
- [x] Event types: session lifecycle, cost updates, budget exceeded, loop started/stopped, scan complete, config changed
- [x] Bus wired into session manager, process manager, MCP server
- [x] Hook executor (`internal/hooks/hooks.go`) with sync/async hook dispatch
- [x] Hook config via `.ralph/hooks.yaml` per repo
- [x] `ralphglasses_event_list` MCP tool for querying recent events

### 2.75.3 — MCP Tool Extensions (COMPLETE, 38 total, +11 new)
- [x] `ralphglasses_event_list` — Query recent fleet events
- [x] `ralphglasses_fleet_analytics` — Cost breakdown by provider/repo/time-period
- [x] `ralphglasses_session_compare` — Compare two sessions (cost, turns, duration)
- [x] `ralphglasses_session_output` — Get recent output from running session
- [x] `ralphglasses_repo_health` — Composite health score (0-100)
- [x] `ralphglasses_session_retry` — Re-launch failed session with same params
- [x] `ralphglasses_config_bulk` — Get/set `.ralphrc` values across multiple repos
- [x] `ralphglasses_agent_compose` — Create composite agent by layering existing agents
- [x] `ralphglasses_workflow_define` — Define multi-step YAML workflows
- [x] `ralphglasses_workflow_run` — Execute workflows with dependency ordering
- [x] `ralphglasses_snapshot` — Save/list fleet state snapshots

### 2.75.4 — TUI Capability Extensions (COMPLETE)
- [x] Confirm dialog component — modal overlay for destructive actions
- [x] Multi-select in tables — space to toggle, batch stop
- [x] Actions menu — context-dependent quick actions via `a` key
- [x] Session launcher — inline form to launch sessions via `L` key
- [x] Session output streaming — real-time output view via `o` key
- [x] Timeline view — horizontal bar chart of session lifetimes via `t` key
- [x] Enhanced fleet dashboard — provider bar charts, cost-per-turn, budget gauges, top 5 expensive sessions

### 2.75.5 — Code Organization (COMPLETE)
- [x] Extracted key handlers to `internal/tui/handlers_*.go` (~1163 lines across 4 files) `[reconciled 2026-03-26]`
- [x] Extracted fleet data builder to `internal/tui/fleet_builder.go` (~200 lines)
- [x] `app.go` focused on Model/Init/Update/View (~237 lines) `[reconciled 2026-03-26]`

---

## Phase 3: i3 Multi-Monitor Integration

> **Depends on:** Phase 2 (session model, fleet dashboard)
>
> **Parallel workstreams:** 3.1 (i3 IPC) is the foundation. 3.4 (autorandr) is independent. 3.5 (Sway) can proceed in parallel with 3.2. 3.3 depends on 3.1 + 2.1 (SQLite). 3.6 (Hyprland) is independent.

### 3.1 — i3 IPC client
- [ ] 3.1.1 — Create `internal/i3/` package wrapping go-i3: connect to i3 socket, subscribe to events `P1` `M`
- [ ] 3.1.2 — Workspace CRUD: create named workspace, move to output, rename, close `P1` `M`
- [ ] 3.1.3 — Window management: focus, move-to-workspace, set layout (splitv/splith/tabbed/stacked) `P1` `M`
- [ ] 3.1.4 — Monitor enumeration: list outputs via i3 IPC (name, resolution, position) `P1` `S`
- [ ] 3.1.5 — Event listener: workspace focus, window create/close, output connect/disconnect `P1` `M`
- **Acceptance:** programmatic workspace creation and window placement from Go

### 3.2 — Monitor layout manager `[BLOCKED BY 3.1]`
- [ ] 3.2.1 — Define layout presets as JSON: "dev" (agents + logs), "fleet" (all sessions), "focused" (single agent) `P1` `M`
- [ ] 3.2.2 — 7-monitor workspace assignment config (`distro/i3/workspaces.json`) `P1` `S`
- [ ] 3.2.3 — TUI command `:layout <name>` — apply preset `P1` `M`
- [ ] 3.2.4 — Save current layout as custom preset (`:layout save <name>`) `P2` `M`
- [ ] 3.2.5 — Handle missing monitors gracefully: skip unavailable outputs, log warning, fall back `P1` `S`
- **Acceptance:** `:layout fleet` redistributes windows across monitors

### 3.3 — Multi-instance coordination `[BLOCKED BY 3.1, 2.1]`
- [ ] 3.3.1 — Shared state via SQLite: same DB file, WAL mode, `PRAGMA busy_timeout` `P1` `L`
- [ ] 3.3.2 — Instance discovery: Unix domain socket per instance, advertise PID and capabilities `P1` `M`
- [ ] 3.3.3 — Leader election: simple file-lock based leader for fleet operations `P1` `M`
- [ ] 3.3.4 — Leader failover: detect leader crash via heartbeat, re-elect `P2` `M`
- **Acceptance:** two ralphglasses instances share session state without corruption

### 3.4 — autorandr integration `[PARALLEL — independent]`
- [ ] 3.4.1 — Detect monitor connects/disconnects via i3 output events or udev `P2` `M`
- [ ] 3.4.2 — Auto-apply saved autorandr profiles on hotplug `P2` `M`
- [ ] 3.4.3 — Generate autorandr profiles from current xrandr state `P2` `M`
- [ ] 3.4.4 — Link autorandr profiles to layout presets `P2` `M`
- **Acceptance:** monitor hot-plug triggers layout restore

### 3.5 — Sway/Wayland compatibility `[PARALLEL]`
- [ ] 3.5.1 — Abstract WM interface: `internal/wm/` with i3 and Sway backends `P2` `L`
- [ ] 3.5.2 — Sway IPC client `P2` `M`
- [ ] 3.5.3 — Auto-detect WM at startup: check `$SWAYSOCK` vs `$I3SOCK` `P2` `S`
- [ ] 3.5.4 — Test suite: integration tests for both backends `P2` `M`
- **Acceptance:** layout commands work on both i3 and Sway

### 3.6 — Hyprland support `[PARALLEL]`
- [ ] 3.6.1 — Hyprland IPC client: `internal/wm/hyprland/` `P2` `M`
- [ ] 3.6.2 — Workspace dispatch: `hyprctl dispatch workspace` `P2` `S`
- [ ] 3.6.3 — Monitor configuration: `hyprctl monitors` `P2` `S`
- [ ] 3.6.4 — Dynamic workspaces: leverage Hyprland's per-monitor model `P2` `M`
- **Acceptance:** layout commands work on Hyprland

## Phase 3.5: Theme & Plugin Ecosystem

> Inspired by k9s skins + plugins system, Ghostty shader architecture,
> Starship module design, and Claude Code skills framework.

### 3.5.1 — Theme ecosystem (like k9s skins + Ghostty themes)
- [ ] 3.5.1.1 — Switch theme colors from ANSI-256 integers to hex strings `P1` `M`
- [ ] 3.5.1.2 — Add `snazzy` theme `P1` `S`
- [ ] 3.5.1.3 — Add `catppuccin-macchiato` and `catppuccin-mocha` themes `P1` `S`
- [ ] 3.5.1.4 — Add `tokyo-night` theme `P2` `S`
- [ ] 3.5.1.5 — Support `~/.config/ralphglasses/themes/` external theme directory `P1` `M`
- [ ] 3.5.1.6 — Add `--theme` CLI flag and `RALPH_THEME` .ralphrc key `P1` `S`
- [ ] 3.5.1.7 — Add `:theme <name>` TUI command for live theme switching `P1` `M`
- **Acceptance:** `ralphglasses --theme snazzy` renders with hex-accurate palette; user themes load correctly

### 3.5.2 — Plugin system v2 (like k9s plugins.yml)
- [ ] 3.5.2.1 — Define `~/.config/ralphglasses/plugins.yml` schema `P1` `M`
- [ ] 3.5.2.2 — Plugin loader: parse YAML at startup, register keybinds per scope `P1` `M`
- [ ] 3.5.2.3 — Variable resolver: substitute runtime context in command args `P1` `M`
- [ ] 3.5.2.4 — Built-in plugins: `stern-logs`, `gh-pr`, `session-cost-report` `P2` `L`
- [ ] 3.5.2.5 — Plugin shortcut display in help view `P2` `S`
- [ ] 3.5.2.6 — MCP tool for plugin management `P2` `M`
- **Acceptance:** user-defined YAML plugins execute commands with variable substitution from TUI

### 3.5.3 — Resource aliases (like k9s aliases.yml)
- [ ] 3.5.3.1 — Define `~/.config/ralphglasses/aliases.yml` schema `P2` `S`
- [ ] 3.5.3.2 — Built-in aliases: `:rp` -> repos, `:ss` -> sessions, `:tm` -> teams, `:fl` -> fleet `P2` `S`
- [ ] 3.5.3.3 — User-defined command aliases `P2` `M`
- **Acceptance:** `:alias-name` in command mode executes mapped command

### 3.5.4 — MCP skill export (like Claude Code skills)
- [ ] 3.5.4.1 — Generate `.claude/skills/ralphglasses/SKILL.md` from MCP tool descriptions `P1` `M`
- [ ] 3.5.4.2 — Include YAML frontmatter with allowed-tools `P1` `S`
- [ ] 3.5.4.3 — Auto-update skill on `ralphglasses mcp` server start `P1` `S`
- **Acceptance:** Claude Code auto-discovers ralphglasses skill when MCP server is connected

### 3.5.5 — Theme export to terminal (like claudekit themekit)
Partially complete: `internal/themekit/` ported from claudekit `[reconciled 2026-03-27]`
- [ ] 3.5.5.1 — `ralphglasses theme export ghostty` -> generate Ghostty palette config `P2` `S`
- [ ] 3.5.5.2 — `ralphglasses theme export starship` -> generate Starship color overrides `P2` `S`
- [ ] 3.5.5.3 — `ralphglasses theme export k9s` -> generate k9s skin.yml `P2` `S`
- [ ] 3.5.5.4 — `ralphglasses theme sync` -> export to all supported tools simultaneously `P2` `M`
- **Acceptance:** `ralphglasses theme sync` updates Ghostty, Starship, and k9s to match TUI theme

---

## Phase 4: Bootable Thin Client

> **Depends on:** Phase 3 (i3 integration, monitor layout)
>
> **Parallel workstreams:** 4.1 (ISO pipeline) is the foundation. 4.3 (PXE) and 4.6 (OTA) can proceed in parallel. 4.7 (watchdog) is independent. 4.5 (install-to-disk) depends on 4.1. 4.8 (marathon hardening) is independent.

### 4.1 — Dockerfile -> ISO pipeline
**Completed:**
- [x] `distro/Dockerfile` — Ubuntu 24.04, kernel 6.12+ HWE, NVIDIA 550, i3, Go, Claude Code
- [x] `distro/scripts/hw-detect.sh` — GPU detection, GTX 1060 blacklisting, MT7927 BT blacklisting
- [x] `distro/systemd/hw-detect.service` — Oneshot first-boot hardware detection
- [x] `distro/systemd/ralphglasses.service` — TUI autostart after graphical target

**Remaining:**
- [ ] 4.1.1 — `distro/Makefile` target `build`: `docker build` with build args `P1` `M`
- [ ] 4.1.2 — `distro/Makefile` target `squashfs`: extract rootfs, `mksquashfs` with xz `P1` `M`
- [ ] 4.1.3 — `distro/Makefile` target `iso`: `grub-mkrescue` with EFI + BIOS `P1` `M`
- [ ] 4.1.4 — QEMU smoke test script: boot ISO, verify TUI starts `P1` `M`
- [ ] 4.1.5 — CI integration: build ISO in GitHub Actions, upload as artifact `P2` `L`
- [ ] 4.1.6 — Fix Xorg config: remove hardcoded PCI `BusID`, use hw-detect.sh output `P1` `S`
- [ ] 4.1.7 — Fix networking priority: align Dockerfile with docs (Intel I226-V primary) `P1` `S`
- **Acceptance:** `make iso` produces bootable image, boots in QEMU

### 4.2 — i3 kiosk configuration `[BLOCKED BY 4.1]`
- [ ] 4.2.1 — `distro/i3/config` — workspace-to-output mapping for 7 monitors `P1` `M`
- [ ] 4.2.2 — Strip WM chrome: `default_border none`, no desktop, no dmenu `P1` `S`
- [ ] 4.2.3 — Keybindings: workspace navigation, TUI focus, emergency shell `P1` `S`
- [ ] 4.2.4 — Auto-start: launch ralphglasses fullscreen on workspace 1 `P1` `S`
- [ ] 4.2.5 — Lock screen: disable screen blanking, DPMS off (24/7 operation) `P1` `S`
- [ ] 4.2.6 — Template monitor names: replace hardcoded DP-1/DP-2 with hw-detect.sh values `P1` `S`
- **Acceptance:** boots to fullscreen TUI, no visible WM chrome

### 4.3 — PXE/network boot `[PARALLEL]`
- [ ] 4.3.1 — iPXE chainload config `P2` `M`
- [ ] 4.3.2 — LTSP server setup on UNRAID `P2` `L`
- [ ] 4.3.3 — Network boot squashfs overlay `P2` `M`
- [ ] 4.3.4 — Fallback: USB boot `P2` `M`
- [ ] 4.3.5 — Boot menu: select version `P2` `S`
- **Acceptance:** PXE boot from UNRAID reaches ralphglasses TUI

### 4.4 — Hardware profiles
- [x] ProArt X870E-CREATOR WIFI — primary target (documented in `distro/hardware/proart-x870e.md`)
- [ ] 4.4.1 — Generalize `hw-detect.sh`: PCI ID table with per-device actions `P1` `M`
- [ ] 4.4.2 — Hardware profile schema: JSON manifest with PCI IDs, required modules `P2` `M`
- [ ] 4.4.3 — Validate profiles against running system `P2` `M`
- [ ] 4.4.4 — Template for adding new boards `P2` `S`
- **Acceptance:** hw-detect.sh correctly identifies and configures target hardware

### 4.5 — Install-to-disk `[BLOCKED BY 4.1]`
- [ ] 4.5.1 — `install-to-disk.sh`: partition scheme (512MB ESP + ext4 rootfs) `P2` `L`
- [ ] 4.5.2 — GRUB install: UEFI mode `P2` `M`
- [ ] 4.5.3 — First-boot setup `P2` `M`
- [ ] 4.5.4 — ZFS root option `P2` `L`
- [ ] 4.5.5 — Safety: require `--confirm` flag `P2` `S`
- **Acceptance:** install-to-disk produces bootable system on NVMe

### 4.6 — OTA update mechanism `[PARALLEL]`
- [ ] 4.6.1 — Version check: compare local squashfs hash against remote manifest `P2` `M`
- [ ] 4.6.2 — Download + verify: fetch, SHA256, GPG signature `P2` `M`
- [ ] 4.6.3 — Atomic swap: A/B partition scheme `P2` `L`
- [ ] 4.6.4 — `ralphglasses update` subcommand `P2` `M`
- **Acceptance:** OTA update replaces running image, rollback works on boot failure

### 4.7 — Health watchdog service `[PARALLEL]`
- [ ] 4.7.1 — Systemd watchdog unit `P1` `S`
- [ ] 4.7.2 — Hardware health checks: GPU temp, disk space, memory `P1` `M`
- [ ] 4.7.3 — Alert escalation `P2` `M`
- [ ] 4.7.4 — Heartbeat file `P1` `S`
- **Acceptance:** TUI auto-restarts within 10s of crash

### 4.8 — Marathon.sh hardening `[PARALLEL]`
- [ ] 4.8.1 — Disk space monitoring `P1` `S`
- [ ] 4.8.2 — Memory pressure monitoring `P1` `S`
- [ ] 4.8.3 — Fix restart logic: cap MAX_RESTARTS, exponential backoff `P0` `M`
- [ ] 4.8.4 — Add `bc` availability check `P2` `S`
- [ ] 4.8.5 — Marathon summary report on completion `P2` `M`
- **Acceptance:** marathon.sh survives disk fill and memory pressure

### 4.9 — Secure boot support `[PARALLEL]`
- [ ] 4.9.1 — Sign kernel and bootloader with custom MOK `P2` `L`
- [ ] 4.9.2 — Sign NVIDIA kernel modules `P2` `M`
- [ ] 4.9.3 — MOK enrollment first-boot flow `P2` `M`
- [ ] 4.9.4 — Document Secure Boot setup `P2` `S`
- **Acceptance:** system boots with Secure Boot enabled + NVIDIA driver loaded

### 4.10 — USB provisioning tool `[BLOCKED BY 4.1]`
- [ ] 4.10.1 — `ralphglasses flash <iso> <device>` — write ISO with progress `P2` `M`
- [ ] 4.10.2 — Persistent overlay partition `P2` `M`
- [ ] 4.10.3 — Pre-seed config `P2` `M`
- [ ] 4.10.4 — Verify write: read-back checksums `P2` `S`
- **Acceptance:** `ralphglasses flash` produces bootable USB with pre-loaded config

---

## Phase 5: Agent Sandboxing & Infrastructure

> **Depends on:** Phase 2 (session model needed for container lifecycle)
>
> **Parallel workstreams:** 5.1 (Docker) and 5.2 (Incus) are parallel sandboxing approaches. 5.3 (MCP gateway) is independent. 5.4 (network) depends on 5.1 or 5.2. 5.6 (secrets) is independent. 5.7-5.8 are independent.

### 5.1 — Docker sandbox mode
- [x] 5.1.1 — `internal/sandbox/` package: create container, manage lifecycle — `docker.go` `[reconciled 2026-03-26]`
- [x] 5.1.2 — Container spec: bind-mount workspace, set `--cpus`, `--memory`, `--network` `[reconciled 2026-03-26]`
- [x] 5.1.3 — Lifecycle binding: session start -> container start, session stop -> container stop + remove `[reconciled 2026-03-26]`
- [ ] 5.1.4 — Log forwarding: capture container stdout/stderr -> session log stream `P1` `M`
- [ ] 5.1.5 — GPU passthrough: `--gpus` flag for NVIDIA containers `P2` `M`
- **Acceptance:** session runs inside container, cleanup on stop

### 5.2 — Incus/LXD containers
- [ ] 5.2.1 — `internal/sandbox/incus/` package: Incus client, profile management `P2` `L`
- [ ] 5.2.2 — Per-container credential isolation `P2` `M`
- [ ] 5.2.3 — Workspace persistence: bind-mount + snapshot `P2` `M`
- [ ] 5.2.4 — Threat detection: suspicious file access, network, resource spikes `P2` `L`
- [ ] 5.2.5 — Port patterns from code-on-incus `P2` `M`
- **Acceptance:** session runs in Incus container with isolated credentials

### 5.3 — MCP gateway `[PARALLEL]`
- [ ] 5.3.1 — Central MCP hub service `P1` `L`
- [ ] 5.3.2 — Per-session tool authorization `P1` `M`
- [ ] 5.3.3 — Audit logging `P1` `M`
- [ ] 5.3.4 — Rate limiting `P1` `M`
- [ ] 5.3.5 — Deploy to UNRAID `P2` `M`
- **Acceptance:** agent tool calls routed through gateway with audit trail

### 5.4 — Network isolation `[BLOCKED BY 5.1 or 5.2]`
- [ ] 5.4.1 — VLAN segmentation `P2` `L`
- [ ] 5.4.2 — iptables/nftables allowlists `P2` `M`
- [ ] 5.4.3 — DNS sinkholing `P2` `M`
- [ ] 5.4.4 — Network policy config in `.ralphrc` `P2` `S`
- **Acceptance:** sandboxed session cannot reach unauthorized endpoints

### 5.5 — Budget federation `[BLOCKED BY 2.3]`
- [ ] 5.5.1 — Global budget pool `P1` `M`
- [ ] 5.5.2 — Per-session limits with carry-over `P1` `M`
- [ ] 5.5.3 — Budget dashboard view `P1` `M`
- [ ] 5.5.4 — Anthropic billing API integration `P2` `L`
- [ ] 5.5.5 — Budget alerts `P1` `S`
- **Acceptance:** global pool enforced across all active sessions

### 5.6 — Secret management `[PARALLEL]`
- [ ] 5.6.1 — Secret provider interface: `internal/secrets/` `P2` `M`
- [ ] 5.6.2 — SOPS backend `P2` `M`
- [ ] 5.6.3 — Vault backend `P2` `L`
- [ ] 5.6.4 — Secret rotation `P2` `M`
- [ ] 5.6.5 — Audit: log secret access per session `P2` `S`
- **Acceptance:** API keys loaded from Vault/SOPS, never stored in plaintext config

### 5.7 — Firecracker microVM sandbox `[PARALLEL]`

> **Research:** E2B achieves ~150ms cold start with Firecracker. Daytona achieves ~90ms with Docker. Industry consensus: Firecracker for untrusted code, gVisor for trusted agents needing syscall-level isolation.

- [ ] 5.7.1 — `internal/sandbox/firecracker/` package `P2` `L`
- [ ] 5.7.2 — Boot kernel + rootfs (target: <200ms cold start, <5MiB RAM per sandbox) `P2` `L`
- [ ] 5.7.3 — virtio-fs workspace mount `P2` `M`
- [ ] 5.7.4 — Resource limits (CPU, memory, network, disk I/O) `P2` `M`
- [ ] 5.7.5 — Snapshot/restore (24h sandbox lifetime for marathon sessions) `P2` `L`
- [ ] 5.7.6 — E2B-compatible sandbox API: `Create()`, `Execute()`, `Filesystem()`, `Terminate()` `P2` `M`
- **Acceptance:** session runs in Firecracker microVM with <200ms boot time

### 5.8 — gVisor runtime `[PARALLEL]`

> **Research:** gVisor provides syscall-level isolation with 10-30% I/O overhead (acceptable for CPU/network-bound agents). Google's kubernetes-sigs/agent-sandbox uses gVisor + Kata. Sweet spot for thin client's trusted-but-isolated agents.

- [ ] 5.8.1 — Configure `runsc` as OCI runtime alternative `P2` `M`
- [ ] 5.8.2 — gVisor sandbox profile (seccomp + AppArmor for defense-in-depth) `P2` `M`
- [ ] 5.8.3 — Performance benchmarking vs Docker `runc` (target: <30% overhead) `P2` `M`
- [ ] 5.8.4 — Fallback logic: detect gVisor, fall back to runc `P2` `S`
- [ ] 5.8.5 — Add gVisor runtime to `distro/` thin client config `P1` `S`
- **Acceptance:** sessions optionally run under gVisor; thin client defaults to gVisor

---

## Phase 6: Advanced Fleet Intelligence

> **Depends on:** Phase 2 (sessions) + Phase 5 (sandboxing). Phase 2.75 event bus provides foundation.
>
> **Parallel workstreams:** 6.1 (native loop) and 6.6 (model routing) can proceed in parallel. 6.3 (coordination) depends on 6.1. 6.4 (analytics) and 6.5 (notifications) are independent. 6.7 (replay) depends on 6.4. 6.8-6.10 are independent.

### 6.1 — Native ralph loop engine
Partially complete: `internal/session/loop.go`, `loop_worker.go`, `loop_helpers.go`, `loop_noop.go`, `loop_validate.go` implement `StepLoop` `[reconciled 2026-03-27]`
- [x] 6.1.1 — StepLoop implementation with iteration management, observation tracking `[reconciled 2026-03-27]`
- [x] 6.1.4 — Parallel execution: run independent tasks concurrently `[reconciled 2026-03-27]`
- [x] 6.1.5 — Progress telemetry: structured events to session event log `[reconciled 2026-03-27]`
- [ ] 6.1.2 — Typed task specs: define task schema (inputs, outputs, dependencies) as Go structs `P1` `M`
- [ ] 6.1.3 — DAG visualization in TUI: show task graph with status `P2` `L`
- **Acceptance:** ralph loop runs natively in Go, DAG visible in TUI

### 6.2 — R&D cycle orchestrator `[BLOCKED BY 6.1]`
- [ ] 6.2.1 — Port perpetual improvement loop from claudekit rdcycle `P1` `L`
- [ ] 6.2.2 — Self-benchmark: coverage, lint score, build time, binary size per iteration `P1` `M`
- [ ] 6.2.3 — Regression detection: compare benchmarks, flag regressions `P0` `M`
- [ ] 6.2.4 — Auto-generate improvement tasks from benchmark regressions `P1` `L`
- [ ] 6.2.5 — Cycle dashboard: iteration history, benchmark trends `P2` `M`
- **Acceptance:** automated benchmark -> task generation cycle runs unattended

### 6.3 — Cross-session coordination `[BLOCKED BY 6.1, 2.1]`
- [ ] 6.3.1 — Shared context store `P1` `M`
- [ ] 6.3.2 — Dedup engine `P1` `M`
- [ ] 6.3.3 — Dependency ordering `P1` `L`
- [ ] 6.3.4 — Conflict resolution `P1` `L`
- [ ] 6.3.5 — Coordination dashboard `P2` `M`
- **Acceptance:** two agents targeting same repo don't conflict on same files

### 6.4 — Analytics & observability `[PARALLEL]`
- [ ] 6.4.1 — Historical data model: SQLite `P1` `M`
- [ ] 6.4.2 — TUI analytics view `P1` `L`
- [ ] 6.4.3 — OpenTelemetry traces `P1` `L`
- [ ] 6.4.4 — Prometheus metrics endpoint `P1` `M`
- [ ] 6.4.5 — Grafana dashboard JSON `P2` `M`
- **Acceptance:** Grafana dashboard shows session metrics over time

### 6.5 — External notifications `[PARALLEL]`
- [ ] 6.5.1 — Webhook dispatcher `P2` `M`
- [ ] 6.5.2 — Discord integration `P2` `M`
- [ ] 6.5.3 — Slack integration `P2` `M`
- [ ] 6.5.4 — Notification templates `P2` `S`
- [ ] 6.5.5 — Rate limiting and retry `P2` `M`
- **Acceptance:** Discord webhook fires on session completion

### 6.6 — Model routing
- [ ] 6.6.1 — Model registry: available models with capabilities, cost/token, context window `P1` `M`
- [ ] 6.6.2 — Task-type classifier: map task types to preferred models `P1` `M`
- [ ] 6.6.3 — Routing rules in `.ralphrc` `P1` `S`
- [ ] 6.6.4 — Dynamic routing: switch model mid-session based on task type `P1` `L`
- [ ] 6.6.5 — Cost optimization: suggest cheaper model when task below threshold `P1` `M`
- **Acceptance:** different task types route to appropriate models

### 6.7 — Replay/audit trail `[BLOCKED BY 6.4]`
- [ ] 6.7.1 — Session recording `P2` `L`
- [ ] 6.7.2 — Replay viewer `P2` `L`
- [ ] 6.7.3 — Export as Markdown/JSON `P2` `M`
- [ ] 6.7.4 — Diff view: compare two session replays `P2` `L`
- [ ] 6.7.5 — Retention policy `P2` `S`
- **Acceptance:** can replay a completed session step-by-step

### 6.8 — Multi-model A/B testing `[PARALLEL]`
- [ ] 6.8.1 — A/B test definition `P2` `M`
- [ ] 6.8.2 — Metric collection `P2` `M`
- [ ] 6.8.3 — Comparison report with statistical significance `P2` `L`
- [ ] 6.8.4 — TUI A/B view `P2` `L`
- [ ] 6.8.5 — Auto-promote default model based on results `P2` `M`
- **Acceptance:** `ralphglasses ab-test --model-a opus --model-b sonnet` produces comparison

### 6.9 — Natural language fleet control `[PARALLEL]`
- [ ] 6.9.1 — MCP sampling integration `P2` `L`
- [ ] 6.9.2 — Command parser `P2` `L`
- [ ] 6.9.3 — Intent classifier `P2` `M`
- [ ] 6.9.4 — Confirmation flow `P2` `M`
- [ ] 6.9.5 — History: persist and replay commands `P2` `S`
- **Acceptance:** natural language commands execute fleet operations

### 6.10 — Cost forecasting `[PARALLEL]`
- [ ] 6.10.1 — Historical cost model `P1` `M`
- [ ] 6.10.2 — Budget projection `P1` `M`
- [ ] 6.10.3 — TUI forecast widget `P2` `M`
- [ ] 6.10.4 — Alert on anomaly: flag >2x predicted rate `P1` `S`
- [ ] 6.10.5 — Recommendation engine `P2` `M`
- **Acceptance:** forecast accuracy within 20% after 10+ sessions

---

## Phase 7: Kubernetes & Cloud Fleet

> **Depends on:** Phase 5 (sandbox model) + Phase 6 (fleet intelligence)
>
> **Parallel workstreams:** 7.1 (K8s operator) is the foundation. 7.2 (autoscaling) depends on 7.1. 7.3 (multi-cloud) is independent. 7.4 (cost management) depends on 7.1. 7.5 (GitOps) is independent.

### 7.1 — Kubernetes operator
- [ ] 7.1.1 — CRD definition: `RalphSession` custom resource `P2` `L`
- [ ] 7.1.2 — Controller: reconcile loop `P2` `XL`
- [ ] 7.1.3 — Pod template `P2` `M`
- [ ] 7.1.4 — Status subresource `P2` `M`
- [ ] 7.1.5 — RBAC: minimal permissions `P2` `S`
- **Acceptance:** `kubectl apply -f session.yaml` creates and manages a ralph session

### 7.2 — Autoscaling `[BLOCKED BY 7.1]`
- [ ] 7.2.1 — HPA integration `P2` `M`
- [ ] 7.2.2 — Node autoscaler hints `P2` `M`
- [ ] 7.2.3 — Budget-aware scaling `P2` `M`
- [ ] 7.2.4 — Scale-to-zero `P2` `M`
- [ ] 7.2.5 — Warm pool `P2` `L`
- **Acceptance:** session count auto-adjusts within budget

### 7.3 — Multi-cloud support `[PARALLEL]`
- [ ] 7.3.1 — AWS provider `P2` `L`
- [ ] 7.3.2 — GCP provider `P2` `L`
- [ ] 7.3.3 — Provider interface: `internal/cloud/` `P2` `M`
- [ ] 7.3.4 — Cross-cloud fleet view `P2` `M`
- [ ] 7.3.5 — Cost comparison `P2` `M`
- **Acceptance:** sessions can launch on AWS or GCP from same TUI

### 7.4 — Cloud cost management `[BLOCKED BY 7.1]`
- [ ] 7.4.1 — Real-time cloud spend tracking `P2` `L`
- [ ] 7.4.2 — Combined budget: API + compute `P2` `M`
- [ ] 7.4.3 — Spot instance strategy `P2` `M`
- [ ] 7.4.4 — Idle resource detection `P2` `S`
- [ ] 7.4.5 — Weekly cost report `P2` `M`
- **Acceptance:** total cost (API + cloud) visible in single budget view

### 7.5 — GitOps deployment `[PARALLEL]`
- [ ] 7.5.1 — Helm chart `P2` `L`
- [ ] 7.5.2 — ArgoCD application `P2` `M`
- [ ] 7.5.3 — Kustomize overlays `P2` `M`
- [ ] 7.5.4 — Sealed secrets `P2` `M`
- [ ] 7.5.5 — Canary deployment `P2` `L`
- **Acceptance:** `git push` to deploy branch triggers automated deployment

---

## Phase 8: Advanced Orchestration & AI-Native Features

> **Depends on:** Phase 6 (fleet intelligence, native loop engine)
>
> **Parallel workstreams:** All sections are independent unless noted.

### 8.1 — Multi-agent collaboration patterns
- [ ] 8.1.1 — Architect/worker pattern `P1` `L`
- [ ] 8.1.2 — Review chain: code -> review -> fix `P1` `L`
- [ ] 8.1.3 — Specialist routing `P1` `M`
- [ ] 8.1.4 — Shared memory: cross-session knowledge base `P1` `L`
- [ ] 8.1.5 — Communication protocol: structured messages via SQLite queue `P1` `M`
- **Acceptance:** architect/worker pattern produces PRs with automated code review

### 8.2 — Prompt management `[PARALLEL]`
- [ ] 8.2.1 — Prompt library: `~/.ralphglasses/prompts/` `P2` `M`
- [ ] 8.2.2 — Variable interpolation `P2` `M`
- [ ] 8.2.3 — Prompt versioning `P2` `M`
- [ ] 8.2.4 — A/B testing `P2` `L`
- [ ] 8.2.5 — TUI prompt editor `P2` `L`
- **Acceptance:** prompt templates configurable per task type

### 8.3 — Workflow engine `[BLOCKED BY 6.1]`
- [ ] 8.3.1 — YAML workflow definitions `P1` `L`
- [ ] 8.3.2 — Built-in workflows: "fix-all-lint", "increase-coverage", "migrate-dependency" `P1` `M`
- [ ] 8.3.3 — Workflow executor: parse YAML -> build DAG -> assign `P1` `XL`
- [ ] 8.3.4 — Conditional logic `P1` `M`
- [ ] 8.3.5 — Workflow marketplace `P2` `L`
- **Acceptance:** YAML workflow runs multi-step, multi-session task to completion

### 8.4 — Code review automation `[PARALLEL]`
- [ ] 8.4.1 — PR review agent `P2` `L`
- [ ] 8.4.2 — Review criteria `P2` `M`
- [ ] 8.4.3 — GitHub integration `P2` `M`
- [ ] 8.4.4 — Auto-approve `P2` `M`
- [ ] 8.4.5 — Review dashboard `P2` `M`
- **Acceptance:** agent-created PRs automatically reviewed

### 8.5 — Self-improvement engine `[BLOCKED BY 6.2]`
Partially complete: `internal/session/reflexion.go`, `episodic.go`, `cascade.go`, `curriculum.go`, `autooptimize.go` implement core subsystems `[reconciled 2026-03-27]`
- [x] 8.5.1 — Reflexion store: verbal reinforcement learning for self-improvement `[reconciled 2026-03-27]`
- [x] 8.5.2 — Episodic memory: Jaccard/cosine similarity for experience retrieval `[reconciled 2026-03-27]`
- [x] 8.5.3 — Cascade router: try-cheap-then-escalate routing strategy `[reconciled 2026-03-27]`
- [x] 8.5.4 — Curriculum sorter: difficulty scoring for task ordering `[reconciled 2026-03-27]`
- [ ] 8.5.5 — Meta-agent: session that monitors other sessions' effectiveness `P1` `XL`
- [ ] 8.5.6 — Config optimization: suggest `.ralphrc` changes based on observed patterns `P1` `L`
- [ ] 8.5.7 — Prompt evolution: mutate and test prompts, keep highest-performing variants `P2` `L`
- [ ] 8.5.8 — Report generation: weekly summary of fleet performance, trends, recommendations `P2` `M`
- **Acceptance:** meta-agent produces actionable configuration improvements

### 8.6 — Codebase knowledge graph `[PARALLEL]`
- [ ] 8.6.1 — Parse codebase: packages, types, functions, dependencies `P2` `L`
- [ ] 8.6.2 — Store in SQLite `P2` `M`
- [ ] 8.6.3 — Query API `P2` `M`
- [ ] 8.6.4 — TUI graph view `P2` `XL`
- [ ] 8.6.5 — Context injection: provide graph context to agents `P2` `L`
- **Acceptance:** knowledge graph queries return accurate code relationships

---

## Phase 9: R&D Cycle Automation `[NEW]`

New phase addressing the critical pipeline gap between findings and actionable work. Derived from analysis of 15 R&D cycles (174+ findings, 9.8% resolution rate). Goal: close the Finding -> Roadmap -> Task -> Execute -> Verify loop.

> **Depends on:** Phase 0.8 (scratchpad tools, observation query)
>
> **Parallel workstreams:** Tier 1 tools are critical path. Tier 2 depends on Tier 1 foundations. Tier 3 is independent.

### Architecture

```
                    +---------+
                    | OBSERVE |  <-- loop_poll, observation_query
                    +----+----+
                         |
                         v
+--------+     +----+--------+----+     +----------+
| PLAN   | <-- | ANALYZE/LEARN   | --> | SCHEDULE |
+---+----+     +-----------------+     +----+-----+
    |          scratchpad_reason,           |
    |          finding_to_task           cron/cycle_schedule
    v                                      |
+---+----+                                 v
| EXECUTE|  <-- loop_start, fleet_submit   |
+---+----+                                 |
    |                                      |
    v                                      |
+---+------+     +----------+              |
| VERIFY   | --> | BASELINE | <------------+
+----------+     +----------+
  merge_verify    cycle_baseline
```

### 9.1 — Tier 1: Critical Path (P0)

These tools close the primary pipeline breaks.

#### 9.1.1 — `finding_to_task` `P0` `M`
Convert scratchpad findings into loop-executable tasks.
- Inputs: `finding_id`, `scratchpad_name`
- Outputs: task spec with difficulty estimate, provider hint, estimated cost, file paths
- Logic: parse finding text, identify affected files via grep, classify severity, estimate effort
- Namespace: `loop`
- File: `internal/mcpserver/tools_loop.go`
- **Acceptance:** `finding_to_task FINDING-240 cycle15_tool_exploration` produces valid loop task spec

#### 9.1.2 — `cycle_merge` `P0` `L`
Auto-merge parallel worktree results from concurrent loop iterations.
- Inputs: `worktree_paths[]`, `conflict_strategy` (ours/theirs/manual)
- Outputs: merge result, conflicts list, merged branch name
- Logic: sequential merge with conflict detection, rollback on failure
- Namespace: `loop`
- File: `internal/session/merge.go`
- **Acceptance:** merges 3 parallel worktrees, reports conflicts without data loss

#### 9.1.3 — `cycle_plan` `P0` `L`
Generate next R&D cycle plan from previous observations.
- Inputs: `previous_cycle_id`, `max_tasks`, `budget_usd`
- Outputs: prioritized task list with difficulty, provider assignments, dependencies
- Logic: read observations -> identify regressions -> check unresolved findings -> generate plan
- Namespace: `loop`
- File: `internal/session/cycle_plan.go`
- **Acceptance:** generates coherent cycle plan from observation history

#### 9.1.4 — `cycle_schedule` `P1` `M`
Schedule cycles with cron triggers for unattended operation.
- Inputs: `cron_expr`, `cycle_config` (budget, max_tasks, target_repo)
- Outputs: `schedule_id`, next execution time
- Logic: register cron job, persist to `.ralph/schedules.json`, trigger via internal ticker
- Namespace: `loop`
- File: `internal/session/scheduler.go`
- **Acceptance:** `cycle_schedule "0 2 * * *" ...` runs daily cycle at 2 AM

#### 9.1.5 — `cycle_baseline` `P0` `S`
Snapshot metrics at cycle start for before/after comparison.
- Inputs: `repo_path`, `metrics[]` (coverage, lint_score, build_time, binary_size, test_count)
- Outputs: `baseline_id`, metric values, timestamp
- Logic: run `go test -cover`, `go vet`, `go build`, record results
- Namespace: `loop`
- File: `internal/session/baseline.go`
- **Acceptance:** `cycle_baseline . coverage,build_time` records reproducible snapshot

### 9.2 — Tier 2: Loop Quality (P1)

#### 9.2.1 — `loop_replay` `P1` `M`
Replay failed iterations with modified parameters.
- Inputs: `loop_id`, `iteration_index`, `overrides` (model, budget, prompt)
- Outputs: replay result, diff from original
- Namespace: `loop`
- **Acceptance:** replay produces different outcome when model/prompt changed

#### 9.2.2 — `loop_budget_forecast` `P1` `S`
Predict iteration costs before execution.
- Inputs: `task_spec`, `model`, `historical_window_hours`
- Outputs: estimated cost, confidence interval, historical comparables
- Namespace: `loop`
- **Acceptance:** forecast within 1.5x of actual cost on 80% of predictions

#### 9.2.3 — `loop_diff_review` `P1` `L`
Auto-review worker diffs against planner intent.
- Inputs: `loop_id`, `iteration_index`
- Outputs: review result (pass/warn/fail), alignment score, specific concerns
- Namespace: `loop`
- **Acceptance:** catches unrelated changes, missing test coverage, scope creep

#### 9.2.4 — `scratchpad_reason` enhancement `P1` `S`
Add LLM-powered reasoning over findings (enhance existing tool).
- Add: cross-finding correlation, root cause analysis, suggested fixes
- Namespace: `observability` (existing)
- **Acceptance:** produces root cause analysis linking related findings

#### 9.2.5 — `observation_correlate` `P1` `M`
Cross-reference observations with git commits.
- Inputs: `loop_id`, `time_range`
- Outputs: observation-commit pairs, files changed per observation, regression markers
- Namespace: `observability`
- **Acceptance:** links observations to specific git commits that caused them

### 9.3 — Tier 3: Fleet Intelligence (P2)

#### 9.3.1 — `fleet_capacity_plan` `P2` `S`
Recommend worker count from queue depth and budget.
- Inputs: `queue_depth`, `available_budget`, `target_completion_hours`
- Outputs: recommended workers, estimated cost, estimated completion time
- Namespace: `fleet`
- **Acceptance:** recommendation matches optimal within 20%

#### 9.3.2 — `provider_benchmark` `P2` `L`
Standardized task suite across providers for comparison.
- Inputs: `providers[]`, `benchmark_suite` (code_fix, test_write, refactor, docs)
- Outputs: per-provider results (cost, duration, quality score)
- Namespace: `eval`
- **Acceptance:** produces reproducible cross-provider comparison

#### 9.3.3 — `session_handoff` `P2` `XL`
Transfer context between sessions for long-running tasks.
- Inputs: `source_session_id`, `target_session_id`, `context_items[]`
- Outputs: handoff summary, transferred items count
- Namespace: `session`
- **Acceptance:** target session has access to source session's key findings

#### 9.3.4 — `prompt_ab_test` `P2` `M`
A/B test prompt variants with statistical analysis.
- Inputs: `prompt_a`, `prompt_b`, `task_spec`, `iterations`
- Outputs: per-variant metrics, p-value, winner recommendation
- Namespace: `eval`
- **Acceptance:** detects 20%+ quality difference with p < 0.05

#### 9.3.5 — `roadmap_prioritize` `P2` `M`
Score roadmap items by impact/effort/dependency.
- Inputs: `items[]` (from roadmap_parse), `weights` (impact, effort, dependency_depth)
- Outputs: prioritized list with scores, suggested next batch
- Namespace: `roadmap`
- **Acceptance:** top 10 items match human judgment on 7/10 picks

---

## Metrics & KPIs `[NEW]`

Track project health and R&D cycle effectiveness. Data sources: loop_baseline.json (10 observations), 101 journal entries, 7 reflections, 29 episodes.

| Metric | Target | Current | Method | Key Roadmap Items |
|--------|--------|---------|--------|-------------------|
| Finding resolution rate | 40% | 9.8% (17/174+) | findings resolved / total findings | Phase 9 (finding_to_task, cycle_plan) |
| Loop completion rate | 85% | 68.75% | completed / started iterations | QW-3, QW-6, 0.6.5 (stall detection) |
| Loop P50 cost | <$0.05 | $0.0553 | loop_baseline.json | 9.2.2 (budget forecast) |
| Loop P95 cost | <$0.25 | $0.2841 | loop_baseline.json | Cost calibration, cascade routing |
| Loop P50 latency | <120s | 137.5s | loop_baseline.json | Worker turn cap, stall detection |
| Loop P95 latency | <300s | 536s | loop_baseline.json | QW-3 (turn cap), timeout tuning |
| Loop verify pass rate | 100% | 100% | loop_baseline.json | Maintain via regression gates |
| Test coverage | 85% | 83.4% | `go test -coverprofile` | 1.6 (coverage targets) |
| Cost calibration accuracy | <1.2x | 1.19x | predicted / actual cost | 9.2.2 (budget forecast) |
| JSON format retry rate | <5% | 25.7% | retries / total calls | QW-1 |
| Zero-change iteration rate | <10% | 95.6% (22/23) | 0-file-change / total passed | QW-3, 0.6.2 (observation enrichment) |
| Fleet phantom task rate | <5% | 73% (381/523 pending) | phantom / total tasks | QW-11 |
| MCP tool coverage (exercised) | 95% | 71% (82/115) | tools tested / total tools | Phase 9 (provider_benchmark) |
| Cascade routing adoption | 100% | 0% | sessions using cascade / total | QW-2 |
| R&D cycle frequency | 1/week | ~2/week | cycles completed per week | 9.1.4 (cycle_schedule) |
| Episodic memory entries | 100+ | 29 | episodes.jsonl count | Self-improvement maturity |
| Learned rules | 5+ | 0 (null) | improvement_patterns.json | QW-12 |

---

## MCP Spec Adoption Tracker `[NEW]`

Current spec: **2025-11-25** (latest stable). Official Go SDK: `modelcontextprotocol/go-sdk` v1.4.1 (supersedes mcp-go for new projects).

| Feature | Spec Version | Status | Phase | Notes |
|---------|-------------|--------|-------|-------|
| Structured Output (`outputSchema`) | 2025-06-18 | Implemented | 0.8 | Schemas for 6 tools |
| MCP Logging (`notifications/message`) | 2024-11-05 | Implemented | 0.8 | LoggingMiddleware |
| Tool Annotations (read-only, destructive) | 2025-06-18 | Implemented | 0.8 | `addToolWithMetadata()` in tools_dispatch.go |
| Tasks (async with polling) | 2025-11-25 | Not started | 9.1 | Durable state machines, `tasks/get` polling, `input_required` state |
| Elicitation (server prompts) | 2025-11-25 | Not started | 9.2 | Server→client clarification mid-execution, bridges to HITL |
| Streamable HTTP transport | 2025-11-25 | Not started | 5.3 | Replaces deprecated SSE; single `/mcp` endpoint, `Mcp-Session-Id`, resume via `Last-Event-ID` |
| Progress notifications | 2024-11-05 | Not started | 0.7 | JSON-RPC `notifications/progress` with progressToken |
| Resource subscriptions | 2025-06-18 | Not started | 6.4 | Push notifications on resource changes |
| OAuth/Auth | 2025-11-25 | Not started | 5.3 | OAuth 2.0 for remote MCP servers (required for Streamable HTTP) |
| Roots | 2025-06-18 | Not started | - | Workspace root discovery |
| Sampling | 2024-11-05 | Not started | 6.9 | NL fleet control |

> **Migration note:** `mark3labs/mcp-go` v0.45.0 is current but v0.x (unstable API). GitHub MCP Server has already migrated to the official SDK. Plan migration path in Phase 1.5.

---

## Claude Code Integration Matrix `[NEW]`

Claude Code supports **24 hook events**, SKILL.md framework, Agent Teams (research preview), and Agent SDK (Python/TS, no Go SDK).

| Feature | Component | Status | Notes |
|---------|-----------|--------|-------|
| MCP Server (stdio) | `internal/mcpserver/` | Implemented | 115 tools (113 namespace + 2 meta), 13 namespaces |
| Deferred tool loading | `internal/mcpserver/tools_dispatch.go` | Implemented | Only core loaded at startup; 85% token reduction |
| Hooks (internal) | `.ralph/hooks.yaml` | Implemented | Internal hook system, not CC hooks |
| CC hooks integration | - | Not started | 24 events: PreToolUse, PostToolUse, Stop, SessionStart, TeammateIdle, TaskCreated/Completed, WorktreeCreate, etc. |
| Skills export (.claude/skills/) | - | Not started | Phase 3.5.4; SKILL.md supports `context: fork`, `agent:`, MCP deps |
| Agent SDK bridge | - | Not started | No native Go SDK; requires sidecar/bridge pattern (Python/TS only) |
| Agent Teams | - | Not started | Research preview; teammates share findings, direct messaging; `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` |
| Prompt caching (`cache_control`) | `internal/enhancer/` | Partial | Enhancer has 3-provider caching; MCP handlers not cached yet |
| Adaptive thinking | - | Not started | GA Feb 2026; `thinking: {type: "adaptive"}` + `effort` param (replaces `budget_tokens`) |
| Beta headers | - | Partial | Key headers: `interleaved-thinking-2025-05-14`, `fast-mode-2026-02-01`, `compact-2026-01-12`, `mcp-client-2025-11-20`, `structured-outputs-2025-11-13`, `files-api-2025-04-14` |
| Token-efficient tool use | - | Built-in | All Claude 4 models have this natively; beta header only needed for Claude 3.7 |
| Structured outputs | - | Not started | GA Jan 2026; `output_config.format.json_schema` for guaranteed schema conformance |
| Messages Batches API | `internal/batch/` | Implemented | 50% discount, 3-provider support (Claude, Gemini, OpenAI) |
| Fast Mode | - | Not started | 2.5x faster Opus, 6x cost ($30/$150/MTok); `fast-mode-2026-02-01` |
| Compaction API | - | Not started | Server-side context summarization; `compact-2026-01-12` |
| MCP Connector (remote) | - | Not started | Messages API → remote MCP servers; `mcp-client-2025-11-20` |

---

## Provider Capability Matrix `[NEW]`

Cost comparison (March 2026, per 1M tokens, input/output):

| Tier | Claude | Gemini | OpenAI |
|------|--------|--------|--------|
| Ultra-cheap | — | Flash-Lite $0.10/$0.40 | GPT-5.4-nano $0.20/$1.25 |
| Worker | — | Flash $0.30/$2.50 | GPT-5.4-mini $0.75/$4.50 |
| Coding | Sonnet 4.6 $3/$15 | — | GPT-5.3-Codex $1.75/$14 |
| Frontier | Opus 4.6 $15/$75 | 3.1 Pro $2/$12 | GPT-5.4-pro $30/$180 |

| Capability | Claude (Anthropic) | Gemini (Google) | Codex (OpenAI) |
|------------|-------------------|-----------------|----------------|
| Models | opus-4-6, sonnet-4-6, haiku-4-5 | 3.1-pro, 2.5-flash, flash-lite | codex-mini, GPT-5.4 |
| Max context | 200K (1M via beta) | 1M tokens | 200K tokens |
| Tool use | Yes (parallel, built-in efficient) | Yes (VALIDATED mode preview) | Yes (strict schema) |
| Streaming | Yes | Yes (stream-json) | Yes (Responses API) |
| Prompt caching | cache_control (90% reads, 1.25x writes) | cachedContents ($1-4.50/hr storage) | Auto prefix (75-90%) |
| Batch API | Messages Batches (50% off) | BatchGenerateContent | POST /v1/batches (JSONL) |
| Resume | --resume | --resume | Not supported |
| Headless mode | --print, -p | --yolo | --full-auto |
| Agent file | CLAUDE.md | .gemini/agents/*.md | AGENTS.md |
| ralphglasses support | Full | Launch + output | Launch only |

> **Routing research:** FrugalGPT/RouteLLM papers show 2-4x cost reduction with learned routers. Current cascade in `cascade.go` uses static thresholds — upgrade to confidence-based escalation (Phase 6.2).

---

## Tech Debt Register `[NEW]`

Derived from R&D cycle findings and scratchpad analysis.

| Issue | Impact | Effort | Components | Phase | Finding IDs |
|-------|--------|--------|------------|-------|-------------|
| JSON format enforcement failing | High — 25.7% retries waste budget | S | loop_worker.go | QW-1 | pattern_count: 26 |
| Zero-change iterations | High — 95.6% wasted compute | M | loop.go, loop_worker.go | QW-3, 0.6.2 | fleet_audit |
| Phantom fleet tasks | High — 73% stale work | S | coordinator.go | QW-11 | fleet_audit |
| Snapshot path claudekit | Medium — broken snapshots | S | snapshot.go | QW-7 | FINDING-148/268 |
| Loop gates zero baseline | Medium — misleading metrics | S | loop_gates.go | QW-6 | FINDING-226/238 |
| Budget params ignored | High — budget not enforced | S | tools_session.go | QW-8 | FINDING-258/261 |
| Provider recommend Claude-only | Medium — no multi-provider | M | tools_provider.go | 2.5 | FINDING-220/262 |
| Relevance scoring flat 0.5 | Medium — research unusable | M | tools_roadmap.go | QW-10 | research_audit |
| improvement_patterns rules null | Low — no learning | S | reflexion.go | QW-12 | pattern_analysis |
| Autonomy not persisted | Medium — state lost on restart | S | autooptimize.go | QW-9 | FINDING-257 |
| Session signal:killed | High — unclean shutdown | M | loop.go, manager.go | QW-3, 0.6.5 | FINDING-160 |
| cmd/ralphglasses-mcp 66.7% coverage | Low — undertested | L | cmd/ralphglasses-mcp/ | 1.6 | cycle14 |

---

## Dependency Chain

```
Phase 0.5 (Critical Fixes) --> Phase 1 (Harden) --> Phase 1.5 (DX)
                                      |                     |
                                      v                     v
Phase 0.9 (Quick Wins)        Phase 2 (Multi-Session) <----+
      |                              |
      v                              v
Phase 9 (R&D Cycle) <--+     Phase 2.5 (Multi-LLM)
      ^                |            |
      |                |     Phase 2.75 (Event Bus + MCP + TUI) [DONE]
Phase 0.8 [DONE] ------+            |
                               +----+----+
                               v         v
                          Phase 3 (i3)   Phase 5 (Sandbox)
                               |              |
                               v              v
                          Phase 3.5      Phase 6 (Intel) <-- 2.75 event bus
                               |              |
                          Phase 4 (Thin)      |
                               |              |
                               +------+-------+
                                      v
                               Phase 7 (K8s/Cloud)
                                      |
                                      v
                               Phase 8 (AI-Native)
```

### Item-Level Dependencies
```
0.5.1 (error fix) --> 1.8 (custom error types build on this)
0.5.2 (watcher fix) --> 1.7 (structured logging for watcher errors)
0.5.7 (version) --> 1.5.2 (release automation needs ldflags)

1.1 --> 1.4 (fixtures needed for PID file tests)
1.* --> 1.6 (coverage targets depend on all other Phase 1 work)

2.1 --> 2.2, 2.3, 2.4, 2.5, 2.8 (session model is foundation)
2.1 + 2.2 + 2.3 --> 2.5 (launcher needs worktrees + budget)
2.3 --> 5.5 (budget federation extends per-session tracking)
2.11 (health endpoint) --> 6.4 (prometheus reuses HTTP server)

3.1 --> 3.2, 3.3 (i3 IPC client needed for layout + coordination)
2.1 + 3.1 --> 3.3 (multi-instance needs SQLite + i3)

4.1 --> 4.2, 4.5, 4.10 (ISO pipeline needed before kiosk + install + USB)
5.1 or 5.2 --> 5.4 (network isolation needs a sandbox runtime)

6.1 --> 6.2, 6.3, 8.3 (native loop engine needed for orchestrator + coordination + workflows)
6.2 --> 8.5 (self-improvement needs R&D cycle)
6.4 --> 6.7 (analytics infrastructure needed for replay)

7.1 --> 7.2, 7.4 (K8s operator needed for autoscaling + cost mgmt)

0.8 (observability tools) --> 9.1 (R&D cycle tools build on scratchpad + observation query)
9.1 (finding_to_task, cycle_plan) --> 9.2 (loop quality tools)

2.75.2 (event bus) --> 6.4 (analytics builds on event history)
2.75.2 (event bus) --> 6.5 (external notifications consume events)
2.75.3 (workflow tools) --> 8.3 (workflow engine extends MCP workflows)
```

---

## R&D Cycle Architecture `[NEW]`

The self-improvement pipeline operates as a closed loop:

```
 Observations --> Journal --> Patterns --> Scratchpad --> Roadmap --> Curriculum
      ^                                                                  |
      |                                                                  v
      +--- Verify <--- Execute <--- Plan <--- finding_to_task <--- Prioritize
```

### Data flow per cycle

1. **Baseline** (`cycle_baseline`): Snapshot coverage, lint, build time, test count
2. **Plan** (`cycle_plan`): Read observations, identify regressions, generate task list
3. **Execute** (`loop_start`): Run tasks via StepLoop with cascade routing
4. **Observe** (`observation_query`): Record iteration outcomes, costs, diffs
5. **Learn** (`scratchpad_reason`): Analyze observations, extract findings
6. **Verify** (`merge_verify`, `loop_gates`): Confirm improvements, detect regressions
7. **Persist** (`scratchpad_append`): Write findings, update patterns
8. **Merge** (`cycle_merge`): Combine parallel worktree results

### Subsystem mapping

| Subsystem | Package | Key Files | Status |
|-----------|---------|-----------|--------|
| StepLoop | `internal/session/` | loop.go, loop_worker.go | Implemented |
| Cascade Router | `internal/session/` | cascade.go | Implemented, not enabled by default |
| Curriculum Sorter | `internal/session/` | curriculum.go | Implemented |
| Reflexion Store | `internal/session/` | reflexion.go | Implemented, rules null |
| Episodic Memory | `internal/session/` | episodic.go | Implemented |
| Auto-Optimize | `internal/session/` | autooptimize.go | Implemented, not persisted |
| Auto-Recovery | `internal/session/` | autorecovery.go | Implemented |
| Bandit (UCB1) | `internal/bandit/` | selector.go | Implemented, not configured |
| Blackboard | `internal/blackboard/` | blackboard.go | Implemented |
| Cost Predictor | `internal/session/` | costpredictor.go | Implemented |
| No-op Detector | `internal/session/` | convergence.go | Implemented |
| Batch Processing | `internal/batch/` | batch.go | Implemented (3-provider, 50% discount) |

### Research-informed enhancements

| Paper/Pattern | Applicable Subsystem | Enhancement | Priority |
|---------------|---------------------|-------------|----------|
| Reflexion (NeurIPS 2023) | Reflexion Store | Store structured verbal reflections, not just failure logs; inject as context on retry | P1 |
| LATS (ICML 2024) | StepLoop | For high-value tasks, spawn parallel candidate approaches (tree branches), prune by value estimate | P2 |
| DSPy/OPRO | Prompt Enhancement | Add optimizer loop: generate N candidate prompts, score with quality scorer, keep best | P1 |
| FrugalGPT/RouteLLM | Cascade Router | Upgrade from static thresholds to confidence-based escalation; learned classifier for task→provider routing | P1 |
| CrewAI memory taxonomy | Blackboard | Upgrade flat KV to structured memory: short-term, long-term, entity, contextual | P2 |
| Multi-agent scaling (Chen 2024) | Fleet | Diminishing returns above 5-7 agents; cap default fleet size, add communication overhead tracking | P1 |

### Competitive landscape positioning

| Competitor | Model | ralphglasses differentiator |
|------------|-------|-----------------------------|
| Cursor | IDE-integrated, single-session, background agents | Fleet orchestration, multi-provider, TUI/headless |
| Windsurf (→OpenAI) | IDE, "awareness" context, single-session | Multi-session, cost optimization, self-improvement loop |
| Aider | CLI single-session, architect/editor two-model | Fleet-scale, TUI dashboard, automated R&D cycles |
| CrewAI/AutoGen/LangGraph | Python frameworks, no end-user TUI | Go binary, production TUI, bootable thin client |
| E2B/Daytona | Sandbox-as-a-service | Self-hosted, integrated orchestration, no vendor lock-in |

---

## External Projects of Interest

### Multi-Session Claude Code Managers
| Project | Type | Platform | Key Feature |
|---------|------|----------|-------------|
| [craigsc/cmux](https://github.com/craigsc/cmux) | Bash | Linux/macOS/WSL | Git worktree per agent, pure bash |
| [cc-workflow](https://github.com/jrimmer/cc-workflow) | Bash | Linux/macOS | tmux + worktrees, Ubuntu provision script |
| [claude-tools](https://github.com/oreoriorosu/claude-tools) | Bash | WSL-native | tmux session management, `/mnt/c/` defaults |
| [zenportal](https://github.com/kgang/zenportal) | Python TUI | Linux | Multi-AI-CLI (Claude, Codex, Gemini) |
| [cc-hub](https://github.com/m0a/cc-hub) | Web UI | Linux | Linux x64 binary, multi-pane terminals |
| [Hive](https://github.com/latagore/hive) | Web UI | Linux | Mobile-first fleet dashboard, task queue |
| [code-on-incus](https://github.com/mensfeld/code-on-incus) | Go CLI | Linux | Incus containers, threat detection |
| [agent-runner](https://github.com/zsyu9779/agent-runner) | Go CLI | Linux/macOS | Stateful sessions, auto-commits |
| [claude-multi.nvim](https://github.com/mb6611/claude-multi.nvim) | Neovim | Any | Multi-session in Neovim |

### Agent OS & Sandboxing
| Project | Type | Maturity | Notes |
|---------|------|----------|-------|
| [StereOS](https://github.com/papercomputeco/stereOS) | NixOS agent OS | Alpha | gVisor sandboxing, produces VM images |
| Docker Sandboxes | Official | Production | Claude Code template, microVM isolation |
| [microvm.nix](https://michael.stapelberg.ch/posts/2026-02-01-coding-agent-microvm-nix/) | NixOS pattern | Documented | Stapelberg's microVM guide |
| [kubernetes-sigs/agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox) | K8s CRD | v0.1.0 | gVisor + Kata, WarmPools |
| [alibaba/OpenSandbox](https://github.com/alibaba/OpenSandbox) | Multi-runtime | Production | Firecracker, gVisor, Kata |
| [E2B](https://e2b.dev/) | Firecracker SaaS | Production | <200ms sandbox boot |
| [Daytona](https://github.com/daytonaio/daytona) | Docker SaaS | Production | <90ms startup, state management |

### Container OS (for hosting agent workloads)
| OS | Recommendation | Notes |
|----|---------------|-------|
| **NixOS** | Top pick | microvm.nix, llm-agents.nix, claude-code-nix |
| **Fedora CoreOS** | Runner-up | Podman Quadlet systemd, no K8s required |
| **Kairos** | Build-your-own | Dockerfile -> bootable ISO |
| **Talos Linux** | K8s-only | API-only, NVIDIA extensions |

### Thin Client Base
| Distro | Size | Notes |
|--------|------|-------|
| **Ubuntu 24.04 HWE** | ~2GB | Current choice. NVIDIA 550 via apt, kernel 6.12+ |
| **DietPi** | ~130MB | Debian, i3 in catalog, thin client proven (legacy option) |
| Tiny Core Linux | 16-21MB | Ultra-minimal, runs in RAM |
| ThinStation | ~50MB | PXE-native, RDP/VNC/SSH |

---

## Internal Ecosystem Integration

### From mcpkit (Go packages to port/embed)
- `mcpkit/ralph/` — Ralph Loop engine (DAG, specs, progress, cost tracking)
- `mcpkit/finops/` — FinOps cost tracking, budget management
- `mcpkit/sampling/` — LLM sampling client
- `mcpkit/registry/` — Tool registry, typed handlers
- `mcpkit/resilience/` — Circuit breakers, retries
- `mcpkit/observability/` — OpenTelemetry + Prometheus
- `mcpkit/orchestrator/` — Multi-agent orchestration

### From shell scripts (port to Go)
- `ralphglasses/marathon.sh` — 12h marathon launcher
- `hg-mcp/.ralph/start_session.sh` — Session launcher with budget reset
- `mesmer/.ralph/start-12hr.sh` — Pre-flight checks, budget projection
- `claudekit/scripts/perpetual-loop.sh` — Perpetual R&D cycle

### From Go MCP servers (reuse patterns)
- `hg-mcp/` — Modular tool registration pattern
- `shielddd/` — Pure-Go SQLite (modernc.org/sqlite), audit logs
- `claudekit/` — rdcycle perpetual loop, budget profiles

---

## Priority Legend

| Label | Meaning |
|-------|---------|
| `P0` | Critical — blocks other work or causes data loss/waste |
| `P1` | Important — significant user/developer value |
| `P2` | Nice to have — polish, optimization, future-proofing |
| `S` | Small — <1 hour, single file |
| `M` | Medium — 1-4 hours, 2-5 files |
| `L` | Large — 4-16 hours, cross-package |
| `XL` | Extra large — multi-day, architectural change |
