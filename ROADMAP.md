# Ralphglasses Roadmap

Command-and-control TUI + bootable thin client for parallel Claude Code agent fleets.

## Core Deliverables

### Deliverable 1: `ralphglasses` Go Binary
Cross-platform Unix TUI (k9s-style) built with Charmbracelet (BubbleTea + Lip Gloss).
Manages multi-session Claude Code / ralph loops from any terminal.

### Deliverable 2: Bootable Linux Thin Client
Featherweight, low-graphics bootable Linux (Ubuntu 24.04-based) that boots into i3 + the ralphglasses TUI.
Supports multi-monitor (7-display, dual-NVIDIA-GPU) and autoboot/cron operation.

---

## Phase 0: Foundation (COMPLETE)

- [x] Go module (`github.com/hairglasses-studio/ralphglasses`)
- [x] Cobra CLI with `--scan-path` flag
- [x] Discovery engine — scan for `.ralph/` and `.ralphrc` repos
- [x] Model layer — parsers for status.json, progress.json, circuit breaker, .ralphrc
- [x] Process manager — launch/stop/pause ralph loops via os/exec with process groups
- [x] File watcher — fsnotify with 2s polling fallback
- [x] Log streamer — tail `.ralph/live.log`
- [x] MCP server — 9 tools (scan, list, status, start, stop, stop_all, pause, logs, config)
- [x] Standalone MCP binary (`cmd/ralphglasses-mcp/`)
- [x] TUI shell — BubbleTea app with k9s-style keymap
- [x] TUI views — overview table, repo detail, log stream, config editor, help
- [x] TUI components — sortable table, breadcrumb, status bar, notifications
- [x] Styles package — Lip Gloss theme (isolated to avoid import cycles)
- [x] Marathon launcher script (`marathon.sh`)

## Phase 0.5: Critical Fixes

Pre-requisite fixes for existing bugs and silent failures. No new features. All items are independent and can be worked in parallel.

> **Parallel workstreams:** All 0.5.x items are independent. No blockers between them.

### 0.5.1 — Silent error suppression in RefreshRepo
- [ ] 0.5.1.1 — Return `[]error` from `RefreshRepo()` in `internal/model/status.go:49-54` instead of discarding with `_ =`
- [ ] 0.5.1.2 — Propagate errors to TUI layer: emit `RefreshErrorMsg` with repo path and error details
- [ ] 0.5.1.3 — Display parse errors in repo detail view status bar (non-blocking, yellow warning)
- [ ] 0.5.1.4 — Add unit test: corrupt status.json → RefreshRepo returns error, not silent zero-value
- **Acceptance:** parse errors in `.ralph/` files visible to user, not silently dropped

### 0.5.2 — Watcher error handling
- [ ] 0.5.2.1 — Replace `return nil` on watcher error (`process/watcher.go:47-48`) with error propagation
- [ ] 0.5.2.2 — Emit `WatcherErrorMsg` to TUI when fsnotify errors occur
- [ ] 0.5.2.3 — Auto-fallback: on watcher error, switch to polling mode and notify user
- [ ] 0.5.2.4 — Add exponential backoff on repeated watcher failures (max 30s)
- **Acceptance:** watcher failures visible in TUI, automatic fallback to polling

### 0.5.3 — Process reaper exit status
- [ ] 0.5.3.1 — Capture `cmd.Wait()` error in `process/manager.go:59` goroutine
- [ ] 0.5.3.2 — Parse exit code: distinguish crash (non-zero) from clean exit (0)
- [ ] 0.5.3.3 — Emit `ProcessExitMsg{RepoPath, ExitCode, Error}` to TUI
- [ ] 0.5.3.4 — Update repo status to "crashed" or "stopped" based on exit code
- [ ] 0.5.3.5 — Add unit test: simulate ralph crash, assert TUI receives crash notification
- **Acceptance:** TUI correctly reports ralph crash vs clean stop

### 0.5.4 — Getpgid fallback safety
- [ ] 0.5.4.1 — Log warning when `Getpgid` fails in `manager.go:78-82` (currently silent fallback to PID-only signal)
- [ ] 0.5.4.2 — Track child PIDs: on process launch, record PID + all child PIDs if available
- [ ] 0.5.4.3 — Fallback kill sequence: SIGTERM to PID → wait 5s → SIGTERM to known children → wait 5s → SIGKILL
- [ ] 0.5.4.4 — Post-stop audit: check for orphaned processes matching `ralph_loop` pattern
- **Acceptance:** `Stop()` reliably kills all child processes, no orphans

### 0.5.5 — Distro path mismatch
- [ ] 0.5.5.1 — Align `hw-detect.service` ExecStart path with Dockerfile install location (`/usr/local/bin/`)
- [ ] 0.5.5.2 — Add path consistency check to `distro/Makefile`: validate service files reference correct paths
- [ ] 0.5.5.3 — Document path conventions in `distro/README.md`: scripts → `/usr/local/bin/`, configs → `/etc/ralphglasses/`
- **Acceptance:** `hw-detect.service` starts successfully on first boot

### 0.5.6 — Grub AMD iGPU fallback
- [ ] 0.5.6.1 — Add GRUB menuentry for AMD iGPU boot: `nomodeset` removed, `amdgpu.dc=1` enabled
- [ ] 0.5.6.2 — Add GRUB menuentry for headless/serial console boot
- [ ] 0.5.6.3 — Set GRUB timeout to 5s (allow human intervention on boot failure)
- [ ] 0.5.6.4 — Add `grub.cfg` validation to CI: parse all menuentry blocks, verify kernel image paths exist
- **Acceptance:** system boots on AMD iGPU when NVIDIA unavailable

### 0.5.7 — Hardcoded version string
- [ ] 0.5.7.1 — Define `var Version = "dev"` in `internal/version/version.go`
- [ ] 0.5.7.2 — Replace hardcoded `"0.1.0"` in `cmd/mcp.go:19` and `cmd/ralphglasses-mcp/main.go:22`
- [ ] 0.5.7.3 — Add `-ldflags "-X internal/version.Version=$(git describe)"` to build commands
- [ ] 0.5.7.4 — Add `ralphglasses version` subcommand: print version, go version, build date, commit SHA
- [ ] 0.5.7.5 — Display version in TUI help view and MCP server info
- **Acceptance:** `ralphglasses version` outputs correct git-derived version

### 0.5.8 — CI BATS guard
- [ ] 0.5.8.1 — Guard BATS step in CI: check `scripts/test/` exists and contains `.bats` files before running
- [ ] 0.5.8.2 — Add BATS install step to CI (install `bats-core` if not present)
- [ ] 0.5.8.3 — Run `marathon.bats` in CI with mock ANTHROPIC_API_KEY
- [ ] 0.5.8.4 — Add CI matrix: test on ubuntu-latest and macos-latest
- **Acceptance:** CI passes regardless of test directory presence

### 0.5.9 — Race condition in MCP scan
- [ ] 0.5.9.1 — Add `sync.RWMutex` to protect `repos` map in `internal/mcpserver/` during concurrent scans
- [ ] 0.5.9.2 — Add `go test -race` to CI pipeline for all packages
- [ ] 0.5.9.3 — Write concurrent scan test: 10 goroutines scanning simultaneously
- **Acceptance:** `go test -race ./...` passes clean

### 0.5.10 — Marathon.sh edge cases
- [ ] 0.5.10.1 — Add `bc` availability check at script start (budget calculation depends on it)
- [ ] 0.5.10.2 — Add disk space check before marathon start (warn if < 5GB free)
- [ ] 0.5.10.3 — Fix infinite restart loop: cap MAX_RESTARTS, add cooldown between restarts
- [ ] 0.5.10.4 — Add memory pressure monitoring: check `/proc/meminfo` AvailMem, warn at < 2GB
- [ ] 0.5.10.5 — Add log rotation: rotate marathon logs at 100MB, keep last 3
- **Acceptance:** marathon.sh handles resource exhaustion gracefully

### 0.5.11 — Config validation strictness
- [ ] 0.5.11.1 — Define canonical key list with types: `internal/model/config_schema.go`
- [ ] 0.5.11.2 — Warn on unknown keys in `.ralphrc` (typo detection)
- [ ] 0.5.11.3 — Validate numeric ranges: `MAX_CALLS_PER_HOUR` must be 1-1000, `CB_COOLDOWN_MINUTES` must be 1-60
- [ ] 0.5.11.4 — Validate boolean values: only "true"/"false", reject "yes"/"1"/"on"
- **Acceptance:** invalid `.ralphrc` values produce clear error messages

## Phase 1: Harden & Test

**Completed:**
- [x] Unit tests for all packages — 78.2% coverage (discovery, model, process, mcpserver)
- [x] TUI tests — 55.5% app coverage, view rendering, keymap, command/filter modes
- [x] CI pipeline — `go test`, `go vet`, `golangci-lint`, shellcheck, fuzz, benchmarks, BATS
- [x] Error handling — MCP scan error propagation, log stream errors, config key validation
- [x] Process manager — watcher timeout fix (no longer blocks event loop)
- [x] Config editor — key validation

**Remaining (38 subtasks):**

> **Parallel workstreams:** 1.1 and 1.2 can proceed concurrently. 1.3 and 1.5 can proceed concurrently. 1.4 depends on 1.1 fixtures. 1.6 depends on all others. 1.7-1.10 can proceed in parallel with everything except 1.6.

### 1.1 — Integration test: full lifecycle
- [ ] 1.1.1 — Create test fixture directory with `.ralph/` dir, mock `status.json`, and dummy `.ralphrc`
- [ ] 1.1.2 — Write mock `ralph_loop.sh` that simulates loop lifecycle (start, write status, exit)
- [ ] 1.1.3 — Implement lifecycle test: scan → start → poll status → stop, assert state transitions
- [ ] 1.1.4 — Add `//go:build integration` tag and CI gate (`go test -tags=integration`)
- **Acceptance:** `go test -tags=integration` passes end-to-end lifecycle

### 1.2 — MCP server hardening
- [ ] 1.2.1 — Audit all shared state in `mcpserver`; add `sync.RWMutex` around `repos` map and scan results
- [ ] 1.2.2 — Migrate all `log.Printf` calls to `slog` with structured fields (tool name, repo path, duration)
- [ ] 1.2.3 — Add request validation: reject empty repo paths, unknown config keys, malformed JSON
- [ ] 1.2.4 — Define MCP error codes (not-found, invalid-input, internal) and return structured errors
- **Acceptance:** no data races under `go test -race`, structured JSON log output

### 1.3 — TUI polish
- [ ] 1.3.1 — Build `ConfirmDialog` component (y/n prompt overlay, reusable across views)
- [ ] 1.3.2 — Wire confirm dialog to destructive actions: stop, stop_all, config delete
- [ ] 1.3.3 — Add SIGINT/SIGTERM shutdown handler: stop all managed processes, flush logs, clean exit
- [ ] 1.3.4 — Audit scroll bounds in log stream and table views; fix off-by-one on terminal resize
- **Acceptance:** destructive actions require y/n, clean exit on signals, no scroll panics on resize

### 1.4 — Process manager improvements
- [ ] 1.4.1 — Define PID file format (JSON: pid, start_time, repo_path) and write on process launch `[BLOCKED BY 1.1.1]`
- [ ] 1.4.2 — Implement orphan scanner: on startup, check PID files against running processes, clean stale entries
- [ ] 1.4.3 — Add restart policy to `.ralphrc` (`RESTART_ON_CRASH=true`, `MAX_RESTARTS=3`, `RESTART_DELAY_SEC=5`)
- [ ] 1.4.4 — Implement health check loop: poll process status every 5s, trigger restart or circuit breaker on repeated failures
- **Acceptance:** survives ralph crash with auto-restart, no orphan processes after TUI exit

### 1.5 — Config editor enhancements
- [ ] 1.5.1 — Add key CRUD operations: insert new key, rename key, delete key from TUI
- [ ] 1.5.2 — Wire fsnotify on `.ralphrc` file; reload config on external change, emit notification
- [ ] 1.5.3 — Add validation rules per key type (numeric ranges, boolean, enum values)
- [ ] 1.5.4 — Implement undo buffer (single-level: revert last edit)
- **Acceptance:** external edits reflected without restart, invalid values rejected with message

### 1.6 — Test coverage targets
- [ ] 1.6.1 — Set per-package coverage targets: discovery 90%, model 90%, process 85%, mcpserver 85%, tui 70%
- [ ] 1.6.2 — Add CI enforcement step: `go test -coverprofile` → parse → fail if below threshold
- [ ] 1.6.3 — Add coverage badge to README via codecov or go-cover-treemap
- [ ] 1.6.4 — Write missing tests to reach 85%+ overall (focus on untested error paths)
- **Acceptance:** `go test -coverprofile` meets thresholds in CI, badge visible in README

### 1.7 — Structured logging `[NEW]`
- [ ] 1.7.1 — Replace all `log.Printf` calls in `internal/mcpserver/` with `slog.Info`/`slog.Error`
- [ ] 1.7.2 — Replace all `log.Printf` calls in `internal/process/` with structured `slog`
- [ ] 1.7.3 — Add `--log-level` flag to CLI: debug, info, warn, error (default: info)
- [ ] 1.7.4 — Add `--log-format` flag: text (default for TTY), json (default for non-TTY)
- [ ] 1.7.5 — Add request-scoped fields: inject `slog.Group("request", ...)` with tool name, repo path, duration
- **Acceptance:** all log output is structured `slog`, configurable level and format

### 1.8 — Custom error types `[NEW]` `[BLOCKED BY 0.5.1]`
- [ ] 1.8.1 — Define sentinel errors in `internal/model/`: `ErrStatusNotFound`, `ErrConfigParseFailed`, `ErrCircuitOpen`
- [ ] 1.8.2 — Define sentinel errors in `internal/process/`: `ErrProcessNotRunning`, `ErrAlreadyRunning`, `ErrWatcherFailed`
- [ ] 1.8.3 — Wrap all `fmt.Errorf` with `%w` verb for proper `errors.Is()` / `errors.As()` support
- [ ] 1.8.4 — Create `internal/errors/` package with error classification: transient, permanent, user-facing
- [ ] 1.8.5 — Add error type assertions in MCP handlers: map error types to MCP error codes
- **Acceptance:** callers can use `errors.Is()` and `errors.As()` on all returned errors

### 1.9 — Context propagation `[NEW]`
- [ ] 1.9.1 — Thread `context.Context` through `discovery.Scan()` — support cancellation of long scans
- [ ] 1.9.2 — Thread `context.Context` through `model.Load*()` functions — timeout on stuck file reads
- [ ] 1.9.3 — Use incoming `ctx` in MCP tool handlers (currently received but ignored)
- [ ] 1.9.4 — Add `--scan-timeout` flag: max time for initial directory scan (default: 30s)
- [ ] 1.9.5 — Wire context cancellation to TUI shutdown: cancel in-flight operations on exit
- **Acceptance:** all long-running operations respect context cancellation

### 1.10 — TUI bounds safety `[NEW]`
- [ ] 1.10.1 — Fix SortCol out-of-bounds: clamp `SortCol` to valid range when columns change
- [ ] 1.10.2 — Add search UI to LogView: `/` to enter search, `n`/`N` for next/prev match
- [ ] 1.10.3 — Audit all slice access in TUI components for empty-slice panics
- [ ] 1.10.4 — Add fuzz tests for table rendering with random column counts and data
- [ ] 1.10.5 — Handle zero-height terminal gracefully (don't panic, show "terminal too small")
- **Acceptance:** no panics on edge-case terminal sizes or empty data

## Phase 1.5: Developer Experience

Tooling, release automation, and contributor workflow. All items independent of Phase 1.

> **Parallel workstreams:** All 1.5.x items are independent except 1.5.2 depends on 0.5.7 (version ldflags).

### 1.5.1 — Shell completions
- [ ] 1.5.1.1 — Add `ralphglasses completion bash|zsh|fish` via cobra built-in `GenBashCompletionV2`
- [ ] 1.5.1.2 — Add dynamic completions for `--scan-path` (directory completion)
- [ ] 1.5.1.3 — Add dynamic completions for repo names in `status`, `start`, `stop` subcommands
- [ ] 1.5.1.4 — Add install instructions for each shell to `docs/completions.md`
- [ ] 1.5.1.5 — Package completions in release artifacts (`.deb` installs to `/usr/share/bash-completion/`)
- **Acceptance:** `ralphglasses <tab>` completes subcommands and repo names

### 1.5.2 — Release automation `[BLOCKED BY 0.5.7]`
- [ ] 1.5.2.1 — Add `.goreleaser.yaml`: multi-arch builds (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64)
- [ ] 1.5.2.2 — GitHub Actions release workflow: tag push → goreleaser → GitHub Release with binaries
- [ ] 1.5.2.3 — Add changelog generation: `goreleaser` changelog from conventional commits
- [ ] 1.5.2.4 — Add Docker image build: `ghcr.io/hairglasses-studio/ralphglasses` multi-arch manifest
- [ ] 1.5.2.5 — Add Homebrew tap: `hairglasses-studio/homebrew-tap` with goreleaser auto-update
- [ ] 1.5.2.6 — Add AUR package: `PKGBUILD` for Arch Linux users
- **Acceptance:** `git tag v0.2.0 && git push --tags` produces release with binaries, Docker image, Homebrew formula

### 1.5.3 — Pre-commit hooks
- [ ] 1.5.3.1 — Add `.pre-commit-config.yaml`: golangci-lint, gofumpt, shellcheck, markdownlint
- [ ] 1.5.3.2 — Add `Makefile` with targets: `lint`, `fmt`, `test`, `build`, `install`
- [ ] 1.5.3.3 — Add EditorConfig (`.editorconfig`) for consistent formatting across editors
- [ ] 1.5.3.4 — Add `CONTRIBUTING.md` with setup instructions and PR guidelines
- **Acceptance:** `pre-commit run --all-files` passes clean

### 1.5.4 — Config schema documentation
- [ ] 1.5.4.1 — Write `docs/ralphrc-reference.md`: all keys, types, defaults, descriptions, examples
- [ ] 1.5.4.2 — Add `ralphglasses config list-keys` subcommand: print all known keys with defaults
- [ ] 1.5.4.3 — Add `ralphglasses config validate` subcommand: check `.ralphrc` against schema
- [ ] 1.5.4.4 — Add `ralphglasses config init` subcommand: generate `.ralphrc` with all keys and defaults
- [ ] 1.5.4.5 — Auto-generate config docs from schema (Go code → Markdown via `go:generate`)
- **Acceptance:** `ralphglasses config list-keys` outputs all valid configuration keys

### 1.5.5 — Man page generation
- [ ] 1.5.5.1 — Add `//go:generate` directive: `cobra/doc.GenManTree` for all subcommands
- [ ] 1.5.5.2 — Include man pages in release artifacts (`.tar.gz` includes `man/`)
- [ ] 1.5.5.3 — Add `make install-man` target: copy to `/usr/local/share/man/man1/`
- **Acceptance:** `man ralphglasses` works after install

### 1.5.6 — Multi-arch builds
- [ ] 1.5.6.1 — Add arm64 cross-compilation to CI matrix (linux/arm64 for Raspberry Pi)
- [ ] 1.5.6.2 — Test arm64 binary in QEMU user-mode emulation in CI
- [ ] 1.5.6.3 — Add `GOARCH=arm64` smoke test: build + run `--help` + exit
- [ ] 1.5.6.4 — Document Raspberry Pi thin client setup in `docs/raspberry-pi.md`
- **Acceptance:** arm64 binary runs on Raspberry Pi 4/5

### 1.5.7 — Nix flake (optional)
- [ ] 1.5.7.1 — Add `flake.nix` with `buildGoModule` + dev shell (Go, golangci-lint, shellcheck)
- [ ] 1.5.7.2 — Add NixOS module: systemd service, option types for config
- [ ] 1.5.7.3 — Add `flake.lock` and CI check: `nix build` + `nix flake check`
- **Acceptance:** `nix run github:hairglasses-studio/ralphglasses` works

### 1.5.8 — Development containers
- [ ] 1.5.8.1 — Add `.devcontainer/devcontainer.json`: Go + tools, port forwarding, recommended extensions
- [ ] 1.5.8.2 — Add `.devcontainer/Dockerfile`: Go 1.22+, golangci-lint, BATS, shellcheck
- [ ] 1.5.8.3 — GitHub Codespaces support: verify `go build ./...` and `go test ./...` work
- **Acceptance:** `devcontainer up` provides working dev environment

### 1.5.9 — Documentation site
- [ ] 1.5.9.1 — Add `docs/` site with mdBook or mkdocs: getting started, architecture, API reference
- [ ] 1.5.9.2 — Add GitHub Actions: build docs on push, deploy to GitHub Pages
- [ ] 1.5.9.3 — Add architecture diagrams: mermaid flowcharts for data flow, component relationships
- [ ] 1.5.9.4 — Add MCP tool API reference: auto-generated from tool handler docstrings
- **Acceptance:** docs site live at `hairglasses-studio.github.io/ralphglasses`

### 1.5.10 — Benchmarking infrastructure
- [ ] 1.5.10.1 — Add Go benchmarks for hot paths: `RefreshRepo`, `Scan`, `LoadStatus`, table rendering
- [ ] 1.5.10.2 — Add `benchstat` comparison in CI: detect performance regressions between commits
- [ ] 1.5.10.3 — Add benchmark dashboard: track p50/p99 latencies over time
- [ ] 1.5.10.4 — Add memory allocation benchmarks: `b.ReportAllocs()` on all benchmark functions
- **Acceptance:** CI fails on >10% performance regression

## Phase 2: Multi-Session Fleet Management

> **Depends on:** Phase 1 (concurrency guards, process manager improvements)
>
> **Parallel workstreams:** 2.1 (data model) is the foundation — most items depend on it. 2.6 (notifications) and 2.7 (tmux) are independent of each other and can proceed after 2.1. 2.9 (CLI) is independent of TUI work. 2.10 (marathon port) is fully independent. 2.11-2.14 are independent.

### 2.1 — Session data model
- [ ] 2.1.1 — Define `Session` struct: ID, repo path, worktree path, PID, budget, model, status, created_at, updated_at
- [ ] 2.1.2 — Add SQLite via `modernc.org/sqlite`: schema migrations, connection pool, WAL mode
- [ ] 2.1.3 — Implement Session CRUD: Create, Get, List, Update, Delete with prepared statements
- [ ] 2.1.4 — Implement lifecycle state machine: `created → running → paused → stopped → archived` with valid transition enforcement
- [ ] 2.1.5 — Add session event log table: state changes, errors, budget events with timestamps
- **Acceptance:** sessions survive TUI restart, queryable via SQL

### 2.2 — Git worktree orchestration `[BLOCKED BY 2.1]`
- [ ] 2.2.1 — Create `internal/worktree/` package: wrapping `git worktree add/list/remove`
- [ ] 2.2.2 — Auto-create worktree on session launch: branch naming convention `ralph/<session-id>`
- [ ] 2.2.3 — Implement merge-back: `git merge --no-ff` with conflict detection and abort-on-conflict option
- [ ] 2.2.4 — Add worktree cleanup on session stop/archive (remove worktree dir, prune)
- [ ] 2.2.5 — Handle edge cases: dirty worktree on stop, orphaned branches, worktree path conflicts
- **Acceptance:** `ralphglasses worktree create <repo>` produces isolated worktree, merge-back detects conflicts

### 2.3 — Budget tracking `[BLOCKED BY 2.1]`
- [ ] 2.3.1 — Per-session spend poller: read `session_spend_usd` from `.ralph/status.json` on watcher tick
- [ ] 2.3.2 — Implement global budget pool: total ceiling, per-session allocation, remaining calculation
- [ ] 2.3.3 — Add threshold alerts at 50%, 75%, 90% — emit BubbleTea message for TUI notification
- [ ] 2.3.4 — Auto-pause session at budget ceiling: send SIGSTOP, update session state
- [ ] 2.3.5 — Port budget tracking patterns from `mcpkit/finops` (cost ledger, rate calculation)
- **Acceptance:** session auto-pauses when budget exhausted, alerts visible in TUI

### 2.4 — Fleet dashboard TUI view `[BLOCKED BY 2.1]`
- [ ] 2.4.1 — Create `ViewFleet` in view stack with aggregate session table
- [ ] 2.4.2 — Columns: session name, repo, status, spend, loop count, model, uptime — sortable
- [ ] 2.4.3 — Live-update via watcher ticks: refresh spend/status/loop count per row
- [ ] 2.4.4 — Inline actions from fleet view: start/stop/pause selected session via keybinds
- [ ] 2.4.5 — Add fleet summary bar: total sessions, running count, total spend, aggregate throughput
- **Acceptance:** fleet view shows all sessions with live-updating spend/status

### 2.5 — Session launcher `[BLOCKED BY 2.1, 2.2, 2.3]`
- [ ] 2.5.1 — Implement `:launch` command: pick repo from discovered list, set session name
- [ ] 2.5.2 — Add budget/model selection to launch flow: dropdown or tab-complete for model, numeric input for budget
- [ ] 2.5.3 — Default budget from `.ralphrc` (`RALPH_SESSION_BUDGET`) or global config fallback
- [ ] 2.5.4 — Session templates: save current launch config as named template, load from template
- [ ] 2.5.5 — Validate launch preconditions: repo exists, no conflicting worktree, budget available in pool
- **Acceptance:** can launch a named session with budget from TUI command mode

### 2.6 — Notification system `[PARALLEL — independent after 2.1]`
- [ ] 2.6.1 — Desktop notification abstraction: `freedesktop.org` D-Bus (Linux), `osascript` (macOS)
- [ ] 2.6.2 — Define event types: session_complete, budget_warning, circuit_breaker_trip, crash, restart
- [ ] 2.6.3 — Add `.ralphrc` config keys: `NOTIFY_DESKTOP=true`, `NOTIFY_SOUND=true`
- [ ] 2.6.4 — Implement notification dedup/throttle: no repeat within 60s for same event type + session
- **Acceptance:** desktop notification fires on circuit breaker trip

### 2.7 — tmux integration `[PARALLEL — independent after 2.1]`
- [ ] 2.7.1 — `internal/tmux/` package: create/list/kill sessions, name windows, attach/detach
- [ ] 2.7.2 — One tmux pane per agent session: auto-create on session launch, name = session ID
- [ ] 2.7.3 — `ralphglasses tmux` subcommand: `list`, `attach <session>`, `detach`
- [ ] 2.7.4 — Headless mode: detect no TTY → auto-use tmux instead of TUI
- [ ] 2.7.5 — Port patterns from claude-tools (WSL-native tmux management, `/mnt/c/` path handling)
- **Acceptance:** `ralphglasses tmux list` shows active sessions, `attach` works

### 2.8 — MCP server expansion `[BLOCKED BY 2.1, 2.2, 2.3]`
- [ ] 2.8.1 — Add `ralphglasses_session_create` tool: accepts repo, budget, model, name
- [ ] 2.8.2 — Add `ralphglasses_session_list` tool: returns all sessions with status
- [ ] 2.8.3 — Add `ralphglasses_worktree_create` tool: create worktree for repo
- [ ] 2.8.4 — Add `ralphglasses_budget_status` tool: per-session and global budget info
- [ ] 2.8.5 — Add `ralphglasses_fleet_summary` tool: aggregate stats for agent-to-agent coordination
- **Acceptance:** MCP tools callable from Claude Code, session lifecycle works end-to-end

### 2.9 — CLI subcommands *(new)*
- [ ] 2.9.1 — `ralphglasses session list|start|stop|status` — non-TUI session management
- [ ] 2.9.2 — `ralphglasses worktree create|list|merge|clean` — worktree operations from CLI
- [ ] 2.9.3 — `ralphglasses budget status|set|reset` — budget management from CLI
- [ ] 2.9.4 — JSON output flag (`--json`) for all subcommands for scripting/piping
- **Acceptance:** all fleet operations available without TUI, JSON output parseable by `jq`

### 2.10 — Marathon.sh Go port *(new)* `[PARALLEL — fully independent]`
- [ ] 2.10.1 — Port `marathon.sh` to `internal/marathon/` package: duration limit, budget limit, checkpoints
- [ ] 2.10.2 — `ralphglasses marathon` subcommand: `--budget`, `--duration`, `--checkpoint-interval`
- [ ] 2.10.3 — Replace shell signal handling with Go `os/signal` (SIGINT/SIGTERM → graceful shutdown)
- [ ] 2.10.4 — Git checkpoint tagging in Go: `git tag marathon-<timestamp>` at configurable interval
- [ ] 2.10.5 — Structured marathon logging via `slog` (replace bash `log()` function)
- **Acceptance:** `ralphglasses marathon` replaces `bash marathon.sh` with identical behavior

### 2.11 — Health check endpoint `[NEW — PARALLEL]`
- [ ] 2.11.1 — Add optional `--http-addr` flag (default: disabled, e.g. `:9090`)
- [ ] 2.11.2 — Implement `/healthz` endpoint: returns 200 if process alive, 503 if shutting down
- [ ] 2.11.3 — Implement `/readyz` endpoint: returns 200 if scan complete and sessions loaded
- [ ] 2.11.4 — Implement `/metrics` stub: placeholder for Prometheus endpoint (wired in Phase 6)
- [ ] 2.11.5 — Add systemd watchdog integration: `sd_notify` READY and WATCHDOG signals
- **Acceptance:** `curl localhost:9090/healthz` returns 200 when TUI is running

### 2.12 — Telemetry opt-in `[NEW — PARALLEL]`
- [ ] 2.12.1 — Define telemetry event schema: session_start, session_stop, crash, budget_hit, circuit_trip
- [ ] 2.12.2 — Local JSONL file writer: append events to `~/.ralphglasses/telemetry.jsonl`
- [ ] 2.12.3 — Add `--telemetry` flag and `TELEMETRY_ENABLED` config key (default: off)
- [ ] 2.12.4 — Optional remote POST: send anonymized events to configurable endpoint
- [ ] 2.12.5 — Add `ralphglasses telemetry export` subcommand: export telemetry as CSV/JSON
- **Acceptance:** telemetry events written to local file when opt-in enabled

### 2.13 — Plugin system `[NEW — PARALLEL]`
- [ ] 2.13.1 — Define plugin interface: `Plugin{ Name(), Init(ctx), OnEvent(event), Shutdown() }`
- [ ] 2.13.2 — Plugin discovery: scan `~/.ralphglasses/plugins/` for Go plugin `.so` files
- [ ] 2.13.3 — Built-in plugin: `notify-desktop` (extract from 2.6 as reference implementation)
- [ ] 2.13.4 — Plugin lifecycle: load on startup, unload on shutdown, hot-reload on SIGHUP
- [ ] 2.13.5 — Plugin config: per-plugin config section in `.ralphrc` (e.g. `PLUGIN_NOTIFY_DESKTOP_SOUND=true`)
- **Acceptance:** external `.so` plugin loaded and receives session events

### 2.14 — SSH remote management `[NEW — PARALLEL]`
- [ ] 2.14.1 — `ralphglasses remote add <name> <host>` — register remote thin client
- [ ] 2.14.2 — `ralphglasses remote status` — SSH into registered hosts, collect session status
- [ ] 2.14.3 — `ralphglasses remote start <host> <repo>` — start ralph loop on remote host
- [ ] 2.14.4 — Aggregate remote sessions into fleet view (poll via SSH tunnel)
- [ ] 2.14.5 — SSH key management: `~/.ralphglasses/ssh/` with per-host key configuration
- **Acceptance:** fleet view shows sessions from multiple physical machines

## Phase 2.5: Multi-LLM Agent Orchestration

> **Depends on:** Phase 2.1 (session model)
>
> **Parallel workstreams:** 2.5.1 (provider fixes) is foundation. 2.5.2-2.5.5 depend on it. 2.5.6 is independent.

### 2.5.1 — Fix provider CLI command builders
- [x] 2.5.1.1 — Fix buildCodexCmd: `codex exec PROMPT --json --full-auto` (not `--quiet`)
- [x] 2.5.1.2 — Fix buildGeminiCmd: add `-p` flag and `--yolo` for headless mode
- [x] 2.5.1.3 — Fix Codex prompt delivery (positional arg, not stdin)
- [x] 2.5.1.4 — Fix npm package name in docs (`@google/gemini-cli`)
- [x] 2.5.1.5 — Update provider test suite for correct CLI flags
- **Acceptance:** codex and gemini sessions launchable via MCP tools

### 2.5.2 — Per-provider agent discovery `[BLOCKED BY 2.5.1]`
- [x] 2.5.2.1 — Discover `.gemini/agents/*.md` for Gemini provider
- [x] 2.5.2.2 — Parse `AGENTS.md` sections for Codex provider
- [x] 2.5.2.3 — Add `Provider` field to `AgentDef` type
- [x] 2.5.2.4 — Wire provider param into `agent_list` and `agent_define` MCP tools
- **Acceptance:** `agent_list` returns provider-specific agent definitions

### 2.5.3 — Cross-provider team delegation `[BLOCKED BY 2.5.1]`
- [x] 2.5.3.1 — Add per-task provider override in `TeamTask`
- [x] 2.5.3.2 — Generate provider-aware delegation prompts for lead sessions
- [x] 2.5.3.3 — Update `team_create` with `worker_provider` default param
- [x] 2.5.3.4 — Update `team_delegate` with optional `provider` param
- **Acceptance:** Claude lead delegates tasks to Gemini/Codex workers

### 2.5.4 — Provider-specific resume support `[BLOCKED BY 2.5.1]`
- [x] 2.5.4.1 — Implement Codex resume: `codex exec resume SESSION_ID`
- [x] 2.5.4.2 — Verify Gemini `--resume` flag works with `stream-json`
- [x] 2.5.4.3 — Add resume tests per provider
- **Acceptance:** `session_resume` works for all three providers

### 2.5.5 — Unified cost normalization `[BLOCKED BY 2.5.1]`
- [x] 2.5.5.1 — Verify Codex `--json` cost output fields, update normalizer
- [x] 2.5.5.2 — Verify Gemini `stream-json` cost output fields, update normalizer
- [ ] 2.5.5.3 — Add provider-specific cost fallback (parse stderr for cost if not in JSON)
- **Acceptance:** `cost_usd` tracked accurately for all providers

### 2.5.6 — Batch API integration `[PARALLEL — independent]`
- [ ] 2.5.6.1 — Research: map batch API endpoints for Claude, Gemini, OpenAI (~50% cost)
- [ ] 2.5.6.2 — Add `BatchOptions` to `LaunchOptions` (batch mode flag, callback URL)
- [ ] 2.5.6.3 — Implement batch submission for Claude (Messages Batches API)
- [ ] 2.5.6.4 — Implement batch submission for Gemini (Batch Prediction API)
- [ ] 2.5.6.5 — Implement batch polling/webhook for result collection
- **Acceptance:** batch tasks submitted and results collected for at least one provider

## Phase 2.75: Architecture & Capability Extensions (COMPLETE)

Built across multiple implementation sessions. Extends the TUI, MCP server, and internal architecture with event-driven patterns, new tools, and interactive components.

### 2.75.1 — TUI Polish & Distribution
- [x] 4-tab layout with bubbles tab bar (Repos, Sessions, Teams, Fleet)
- [x] Sparkline charts via ntcharts for cost trends
- [x] 4 themes: k9s (default), dracula, nord, solarized (`internal/tui/styles/theme.go`)
- [x] Desktop notifications — macOS `osascript`, Linux `notify-send` (`internal/notify/`)
- [x] GoReleaser config (`.goreleaser.yaml`)
- [x] Diff view for repo git changes (`internal/tui/views/diffview.go`)

### 2.75.2 — Event Bus & Hook System
- [x] Internal pub/sub event bus (`internal/events/bus.go`) with ring buffer history (1000 events)
- [x] Event types: session lifecycle, cost updates, budget exceeded, loop started/stopped, scan complete, config changed
- [x] Bus wired into session manager, process manager, MCP server
- [x] Hook executor (`internal/hooks/hooks.go`) with sync/async hook dispatch
- [x] Hook config via `.ralph/hooks.yaml` per repo
- [x] `ralphglasses_event_list` MCP tool for querying recent events

### 2.75.3 — MCP Tool Extensions (38 total, +11 new)
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

### 2.75.4 — TUI Capability Extensions
- [x] Confirm dialog component — modal overlay for destructive actions (`internal/tui/components/confirm.go`)
- [x] Multi-select in tables — space to toggle, batch stop (`internal/tui/components/table.go`)
- [x] Actions menu — context-dependent quick actions via `a` key (`internal/tui/components/actionmenu.go`)
- [x] Session launcher — inline form to launch sessions via `L` key (`internal/tui/components/launcher.go`)
- [x] Session output streaming — real-time output view via `o` key
- [x] Timeline view — horizontal bar chart of session lifetimes via `t` key (`internal/tui/views/timeline.go`)
- [x] Enhanced fleet dashboard — provider bar charts, cost-per-turn, budget gauges, top 5 expensive sessions

### 2.75.5 — Code Organization
- [x] Extracted key handlers to `internal/tui/handlers.go` (~770 lines)
- [x] Extracted fleet data builder to `internal/tui/fleet_builder.go` (~200 lines)
- [x] `app.go` focused on Model/Init/Update/View (~500 lines)

## Phase 3: i3 Multi-Monitor Integration

> **Depends on:** Phase 2 (session model, fleet dashboard)
>
> **Parallel workstreams:** 3.1 (i3 IPC) is the foundation. 3.4 (autorandr) is independent. 3.5 (Sway) can proceed in parallel with 3.2. 3.3 depends on 3.1 + 2.1 (SQLite). 3.6 (Hyprland) is independent.

### 3.1 — i3 IPC client
- [ ] 3.1.1 — Create `internal/i3/` package wrapping go-i3: connect to i3 socket, subscribe to events
- [ ] 3.1.2 — Workspace CRUD: create named workspace, move to output, rename, close
- [ ] 3.1.3 — Window management: focus, move-to-workspace, set layout (splitv/splith/tabbed/stacked)
- [ ] 3.1.4 — Monitor enumeration: list outputs via i3 IPC (name, resolution, position)
- [ ] 3.1.5 — Event listener: workspace focus, window create/close, output connect/disconnect
- **Acceptance:** programmatic workspace creation and window placement from Go

### 3.2 — Monitor layout manager `[BLOCKED BY 3.1]`
- [ ] 3.2.1 — Define layout presets as JSON: "dev" (agents + logs), "fleet" (all sessions), "focused" (single agent)
- [ ] 3.2.2 — 7-monitor workspace assignment config (`distro/i3/workspaces.json`) — maps output names to workspace numbers
- [ ] 3.2.3 — TUI command `:layout <name>` — apply preset by moving windows/workspaces to designated outputs
- [ ] 3.2.4 — Save current layout as custom preset (`:layout save <name>`)
- [ ] 3.2.5 — Handle missing monitors gracefully: skip unavailable outputs, log warning, fall back to available
- **Acceptance:** `:layout fleet` redistributes windows across monitors

### 3.3 — Multi-instance coordination `[BLOCKED BY 3.1, 2.1]`
- [ ] 3.3.1 — Shared state via SQLite: same DB file, WAL mode, `PRAGMA busy_timeout`
- [ ] 3.3.2 — Instance discovery: Unix domain socket per instance, advertise PID and capabilities
- [ ] 3.3.3 — Leader election: simple file-lock based leader for fleet operations (stop_all, budget enforcement)
- [ ] 3.3.4 — Leader failover: detect leader crash via heartbeat, re-elect
- **Acceptance:** two ralphglasses instances share session state without corruption

### 3.4 — autorandr integration `[PARALLEL — independent]`
- [ ] 3.4.1 — Detect monitor connects/disconnects via i3 output events or udev
- [ ] 3.4.2 — Auto-apply saved autorandr profiles on hotplug
- [ ] 3.4.3 — Generate autorandr profiles from current xrandr state via TUI command (`:autorandr save <name>`)
- [ ] 3.4.4 — Link autorandr profiles to layout presets: hotplug → apply profile → apply layout
- **Acceptance:** monitor hot-plug triggers layout restore

### 3.5 — Sway/Wayland compatibility *(new)* `[PARALLEL — independent of 3.2]`
- [ ] 3.5.1 — Abstract WM interface: `internal/wm/` with i3 and Sway backends (i3 IPC vs sway IPC)
- [ ] 3.5.2 — Sway IPC client: workspace/window/output management via Sway's i3-compatible protocol
- [ ] 3.5.3 — Auto-detect WM at startup: check `$SWAYSOCK` vs `$I3SOCK`, select backend
- [ ] 3.5.4 — Test suite: integration tests for both backends (mock IPC socket)
- **Acceptance:** layout commands work on both i3 (X11) and Sway (Wayland)

### 3.6 — Hyprland support *(new)* `[PARALLEL — independent]`
- [ ] 3.6.1 — Hyprland IPC client: `internal/wm/hyprland/` using Hyprland socket protocol
- [ ] 3.6.2 — Workspace dispatch: `hyprctl dispatch workspace` for window placement
- [ ] 3.6.3 — Monitor configuration: `hyprctl monitors` for output enumeration
- [ ] 3.6.4 — Dynamic workspaces: leverage Hyprland's per-monitor workspace model
- **Acceptance:** layout commands work on Hyprland

## Phase 4: Bootable Thin Client

> **Depends on:** Phase 3 (i3 integration, monitor layout)
>
> **Parallel workstreams:** 4.1 (ISO pipeline) is the foundation. 4.3 (PXE) and 4.6 (OTA) can proceed in parallel. 4.7 (watchdog) is independent. 4.5 (install-to-disk) depends on 4.1. 4.8 (marathon hardening) is independent.

### 4.1 — Dockerfile → ISO pipeline
**Completed:**
- [x] `distro/Dockerfile` — Ubuntu 24.04, kernel 6.12+ HWE, NVIDIA 550, i3, Go, Claude Code
- [x] `distro/scripts/hw-detect.sh` — GPU detection, GTX 1060 blacklisting, MT7927 BT blacklisting
- [x] `distro/systemd/hw-detect.service` — Oneshot first-boot hardware detection
- [x] `distro/systemd/ralphglasses.service` — TUI autostart after graphical target

**Remaining:**
- [ ] 4.1.1 — `distro/Makefile` target `build`: `docker build` with build args for kernel version and NVIDIA driver
- [ ] 4.1.2 — `distro/Makefile` target `squashfs`: extract rootfs from container, `mksquashfs` with xz compression
- [ ] 4.1.3 — `distro/Makefile` target `iso`: `grub-mkrescue` with EFI + BIOS support
- [ ] 4.1.4 — QEMU smoke test script: boot ISO, verify TUI starts, check GPU detection output
- [ ] 4.1.5 — CI integration: build ISO in GitHub Actions (no GPU, skip NVIDIA tests), upload as artifact
- [ ] 4.1.6 — Fix Xorg config: remove hardcoded PCI `BusID "PCI:1:0:0"` from Dockerfile, use hw-detect.sh output `[NEW]`
- [ ] 4.1.7 — Fix networking priority: align Dockerfile with docs (Intel I226-V primary, not reversed) `[NEW]`
- **Acceptance:** `make iso` produces bootable image, boots in QEMU

### 4.2 — i3 kiosk configuration `[BLOCKED BY 4.1]`
- [ ] 4.2.1 — `distro/i3/config` — workspace-to-output mapping for 7 monitors (RTX 4090 outputs)
- [ ] 4.2.2 — Strip WM chrome: no title bars (`default_border none`), no desktop, no dmenu
- [ ] 4.2.3 — Keybindings: workspace navigation ($mod+1-7), TUI focus, emergency shell ($mod+Shift+Return)
- [ ] 4.2.4 — Auto-start: launch ralphglasses fullscreen on workspace 1 via `exec` directive
- [ ] 4.2.5 — Lock screen: disable screen blanking, DPMS off (24/7 marathon operation)
- [ ] 4.2.6 — Template monitor names: replace hardcoded DP-1/DP-2 in i3 config with hw-detect.sh-generated values `[NEW]`
- **Acceptance:** boots to fullscreen TUI, no visible WM chrome

### 4.3 — PXE/network boot `[PARALLEL — independent after 4.1]`
- [ ] 4.3.1 — iPXE chainload config: DHCP → iPXE → tftp/http boot menu
- [ ] 4.3.2 — LTSP server setup on UNRAID: serve squashfs over NFS/NBD
- [ ] 4.3.3 — Network boot squashfs overlay: persistent `/home` and `/etc/ralphglasses` via NFS
- [ ] 4.3.4 — Fallback: USB boot with local squashfs + overlay partition
- [ ] 4.3.5 — Boot menu: select version (latest, rollback) via iPXE script
- **Acceptance:** PXE boot from UNRAID reaches ralphglasses TUI

### 4.4 — Hardware profiles
- [x] ProArt X870E-CREATOR WIFI — primary target (documented in `distro/hardware/proart-x870e.md`)
- [ ] 4.4.1 — Generalize `hw-detect.sh`: PCI ID table with per-device actions (load module, blacklist, configure)
- [ ] 4.4.2 — Add hardware profile schema: JSON manifest with PCI IDs, required modules, known issues
- [ ] 4.4.3 — Validate profiles against running system: flag mismatches between manifest and detected hardware
- [ ] 4.4.4 — Template for adding new boards: `distro/hardware/TEMPLATE.md` with required fields
- **Acceptance:** hw-detect.sh correctly identifies and configures target hardware via profile lookup

### 4.5 — Install-to-disk `[BLOCKED BY 4.1]`
- [ ] 4.5.1 — `distro/scripts/install-to-disk.sh`: partition scheme (512MB ESP + ext4 rootfs), auto-detect target disk
- [ ] 4.5.2 — GRUB install: UEFI mode, `grub-install` + `update-grub` with kernel cmdline for NVIDIA
- [ ] 4.5.3 — First-boot setup: run hw-detect.sh, generate i3 config, set hostname, configure network
- [ ] 4.5.4 — ZFS root option: `zpool create` with mirror, boot partition on ext4 (ZFS can't be ESP)
- [ ] 4.5.5 — Safety: require `--confirm` flag, show disk info before wiping, never auto-select boot disk
- **Acceptance:** install-to-disk produces bootable system on NVMe

### 4.6 — OTA update mechanism *(new)* `[PARALLEL — independent]`
- [ ] 4.6.1 — Version check: compare local squashfs hash against remote manifest (S3/GitHub Release)
- [ ] 4.6.2 — Download + verify: fetch new squashfs, SHA256 checksum, GPG signature
- [ ] 4.6.3 — Atomic swap: A/B partition scheme or overlay — boot into new version, rollback on failure
- [ ] 4.6.4 — `ralphglasses update` subcommand: check, download, apply, reboot
- **Acceptance:** OTA update replaces running image, rollback works on boot failure

### 4.7 — Health watchdog service *(new)* `[PARALLEL — independent]`
- [ ] 4.7.1 — Systemd watchdog unit: monitor ralphglasses process, restart on crash
- [ ] 4.7.2 — Hardware health checks: GPU temperature, disk space, memory pressure, network connectivity
- [ ] 4.7.3 — Alert escalation: local notification → log → optional webhook on persistent failure
- [ ] 4.7.4 — Heartbeat file: write timestamp to `/run/ralphglasses/heartbeat`, stale = restart
- **Acceptance:** TUI auto-restarts within 10s of crash, hardware alerts visible in TUI

### 4.8 — Marathon.sh hardening `[NEW — PARALLEL]`
- [ ] 4.8.1 — Add disk space monitoring: check every checkpoint, abort if < 1GB free
- [ ] 4.8.2 — Add memory pressure monitoring: parse `/proc/meminfo`, pause sessions if AvailMem < 2GB
- [ ] 4.8.3 — Fix restart logic: cap `MAX_RESTARTS` (default 5), exponential backoff (30s, 60s, 120s, 300s)
- [ ] 4.8.4 — Add `bc` availability check at script start (budget math depends on it)
- [ ] 4.8.5 — Add marathon summary report: on completion, write `marathon-summary.json` with stats
- **Acceptance:** marathon.sh survives disk fill and memory pressure

### 4.9 — Secure boot support `[NEW — PARALLEL]`
- [ ] 4.9.1 — Sign kernel and bootloader with custom MOK (Machine Owner Key)
- [ ] 4.9.2 — Sign NVIDIA kernel modules with same MOK
- [ ] 4.9.3 — Add MOK enrollment to first-boot flow (interactive prompt)
- [ ] 4.9.4 — Document Secure Boot setup in `docs/secure-boot.md`
- **Acceptance:** system boots with Secure Boot enabled + NVIDIA driver loaded

### 4.10 — USB provisioning tool `[NEW]` `[BLOCKED BY 4.1]`
- [ ] 4.10.1 — `ralphglasses flash <iso> <device>` — write ISO to USB drive with progress bar
- [ ] 4.10.2 — Persistent overlay partition: create ext4 overlay for config/keys on USB
- [ ] 4.10.3 — Pre-seed config: embed `.ralphrc` and API keys into overlay at flash time
- [ ] 4.10.4 — Verify write: read-back and compare checksums after flash
- **Acceptance:** `ralphglasses flash` produces bootable USB with pre-loaded config

## Phase 5: Agent Sandboxing & Infrastructure

> **Depends on:** Phase 2 (session model needed for container lifecycle)
>
> **Parallel workstreams:** 5.1 (Docker) and 5.2 (Incus) are parallel sandboxing approaches. 5.3 (MCP gateway) is independent. 5.4 (network) depends on 5.1 or 5.2. 5.6 (secrets) is independent. 5.7-5.8 are independent.

### 5.1 — Docker sandbox mode
- [ ] 5.1.1 — `internal/sandbox/docker/` package: build/pull image, create container, manage lifecycle
- [ ] 5.1.2 — Container spec: bind-mount workspace, set `--cpus`, `--memory`, `--network` flags from session config
- [ ] 5.1.3 — Lifecycle binding: session start → container start, session stop → container stop + remove
- [ ] 5.1.4 — Log forwarding: capture container stdout/stderr → session log stream
- [ ] 5.1.5 — GPU passthrough: `--gpus` flag for NVIDIA containers (Claude Code doesn't need GPU, but future models might)
- **Acceptance:** session runs inside container, cleanup on stop

### 5.2 — Incus/LXD containers
- [ ] 5.2.1 — `internal/sandbox/incus/` package: Incus client, profile management, instance lifecycle
- [ ] 5.2.2 — Per-container credential isolation: mount secrets as files, no env var leakage
- [ ] 5.2.3 — Workspace persistence: bind-mount project dir, snapshot on session stop
- [ ] 5.2.4 — Threat detection: monitor for suspicious file access, network connections, resource spikes
- [ ] 5.2.5 — Port patterns from code-on-incus: Go-based container management, security profiles
- **Acceptance:** session runs in Incus container with isolated credentials

### 5.3 — MCP gateway `[PARALLEL — independent]`
- [ ] 5.3.1 — Central MCP hub service: accept connections from multiple agents, route to backend tools
- [ ] 5.3.2 — Per-session tool authorization: allowlist of tools per session, deny by default
- [ ] 5.3.3 — Audit logging: every tool call logged with session ID, tool name, args, result, duration
- [ ] 5.3.4 — Rate limiting: per-session and global rate limits on tool calls
- [ ] 5.3.5 — Deploy to UNRAID: systemd service, auto-start, log rotation
- **Acceptance:** agent tool calls routed through gateway with audit trail

### 5.4 — Network isolation `[BLOCKED BY 5.1 or 5.2]`
- [ ] 5.4.1 — VLAN segmentation: assign each sandbox to isolated VLAN via bridge/macvlan
- [ ] 5.4.2 — iptables/nftables allowlists: per-session rules (allow API endpoints, deny everything else)
- [ ] 5.4.3 — DNS sinkholing: local DNS resolver, block unauthorized domains per session policy
- [ ] 5.4.4 — Network policy config in `.ralphrc`: `SANDBOX_ALLOWED_DOMAINS`, `SANDBOX_NETWORK_MODE`
- **Acceptance:** sandboxed session cannot reach unauthorized endpoints

### 5.5 — Budget federation `[BLOCKED BY 2.3]`
- [ ] 5.5.1 — Global budget pool: total ceiling across all sessions, stored in SQLite
- [ ] 5.5.2 — Per-session limits with carry-over: unused budget redistributed to active sessions
- [ ] 5.5.3 — Budget dashboard view: spend rate ($/hr), projection to exhaustion, per-session breakdown
- [ ] 5.5.4 — Anthropic billing API integration: reconcile local tracking with actual billing (when API available)
- [ ] 5.5.5 — Budget alerts: global pool threshold warnings, session overspend detection
- **Acceptance:** global pool enforced across all active sessions

### 5.6 — Secret management *(new)* `[PARALLEL — independent]`
- [ ] 5.6.1 — Secret provider interface: `internal/secrets/` with pluggable backends
- [ ] 5.6.2 — SOPS backend: decrypt `.sops.yaml` encrypted files, inject as env vars into sessions
- [ ] 5.6.3 — Vault backend: HashiCorp Vault KV v2, lease management, auto-renew
- [ ] 5.6.4 — Secret rotation: detect expiry, refresh credentials, restart affected sessions
- [ ] 5.6.5 — Audit: log secret access (not values) per session
- **Acceptance:** API keys loaded from Vault/SOPS, never stored in plaintext config

### 5.7 — Firecracker microVM sandbox `[NEW — PARALLEL]`
- [ ] 5.7.1 — `internal/sandbox/firecracker/` package: VM lifecycle via Firecracker SDK
- [ ] 5.7.2 — Boot kernel + rootfs: minimal initrd with Go binary + Claude Code
- [ ] 5.7.3 — virtio-fs workspace mount: share project directory with microVM
- [ ] 5.7.4 — Resource limits: vCPU count, memory MB, network bandwidth via rate limiter
- [ ] 5.7.5 — Snapshot/restore: save VM state for instant resume on session unpause
- **Acceptance:** session runs in Firecracker microVM with <500ms boot time

### 5.8 — gVisor runtime `[NEW — PARALLEL]`
- [ ] 5.8.1 — Configure `runsc` as OCI runtime alternative to `runc`
- [ ] 5.8.2 — gVisor sandbox profile: syscall filtering tailored for Claude Code workloads
- [ ] 5.8.3 — Performance benchmarking: measure overhead vs Docker `runc` for typical ralph operations
- [ ] 5.8.4 — Fallback logic: detect gVisor availability, fall back to runc if unavailable
- **Acceptance:** sessions optionally run under gVisor with configurable sandbox profile

## Phase 6: Advanced Fleet Intelligence

> **Depends on:** Phase 2 (sessions) + Phase 5 (sandboxing). Phase 2.75 event bus provides the foundation for 6.4 (analytics), 6.5 (notifications), and 6.7 (replay).
>
> **Parallel workstreams:** 6.1 (native loop) and 6.6 (model routing) can proceed in parallel. 6.3 (coordination) depends on 6.1. 6.4 (analytics) and 6.5 (notifications) are independent. 6.7 (replay) depends on 6.4. 6.8-6.10 are independent.

### 6.1 — Native ralph loop engine
- [ ] 6.1.1 — Embed `mcpkit/ralph` as Go dependency: import DAG executor, task specs, progress tracking
- [ ] 6.1.2 — Typed task specs: define task schema (inputs, outputs, dependencies) as Go structs
- [ ] 6.1.3 — DAG visualization in TUI: show task graph with status (pending/running/complete/failed)
- [ ] 6.1.4 — Parallel execution: run independent tasks concurrently, respect dependency edges
- [ ] 6.1.5 — Progress telemetry: structured events (task_start, task_complete, task_error) to session event log
- **Acceptance:** ralph loop runs natively in Go, DAG visible in TUI

### 6.2 — R&D cycle orchestrator `[BLOCKED BY 6.1]`
- [ ] 6.2.1 — Port perpetual improvement loop from claudekit rdcycle: benchmark → analyze → generate tasks → execute
- [ ] 6.2.2 — Self-benchmark: measure test coverage, lint score, build time, binary size per iteration
- [ ] 6.2.3 — Regression detection: compare benchmarks across iterations, flag regressions above threshold
- [ ] 6.2.4 — Auto-generate improvement tasks: create ralph task specs from benchmark regressions
- [ ] 6.2.5 — Cycle dashboard: iteration history, benchmark trends, task throughput over time
- **Acceptance:** automated benchmark → task generation cycle runs unattended

### 6.3 — Cross-session coordination `[BLOCKED BY 6.1, 2.1]`
- [ ] 6.3.1 — Shared context store: SQLite table of current tasks per session (file, feature, intent)
- [ ] 6.3.2 — Dedup engine: before task assignment, check if another session is working on same file/feature
- [ ] 6.3.3 — Dependency ordering: agent B subscribes to agent A's output, waits for completion event
- [ ] 6.3.4 — Conflict resolution: detect concurrent edits to same file, pause later session, notify
- [ ] 6.3.5 — Coordination dashboard: TUI view showing task assignments across sessions, conflicts, blockers
- **Acceptance:** two agents targeting same repo don't conflict on same files

### 6.4 — Analytics & observability `[PARALLEL — independent]`
- [ ] 6.4.1 — Historical data model: store session metrics (cost, duration, tasks, model) in SQLite
- [ ] 6.4.2 — TUI analytics view: cost per session, throughput, completion rates, time-series charts
- [ ] 6.4.3 — OpenTelemetry traces: port from `mcpkit/observability`, span per task execution
- [ ] 6.4.4 — Prometheus metrics endpoint: `/metrics` HTTP handler with session gauges and counters
- [ ] 6.4.5 — Grafana dashboard JSON: pre-built dashboard for session metrics (import into Grafana)
- **Acceptance:** Grafana dashboard shows session metrics over time

### 6.5 — External notifications `[PARALLEL — independent]`
- [ ] 6.5.1 — Webhook dispatcher: HTTP POST to configured URLs on events
- [ ] 6.5.2 — Discord integration: format events as Discord embeds, send via webhook URL
- [ ] 6.5.3 — Slack integration: format events as Slack blocks, send via webhook URL
- [ ] 6.5.4 — Notification templates: customizable message format per event type
- [ ] 6.5.5 — Rate limiting and retry: deduplicate within window, retry with backoff on failure
- **Acceptance:** Discord webhook fires on session completion

### 6.6 — Model routing *(new)* `[PARALLEL — independent]`
- [ ] 6.6.1 — Model registry: define available models with capabilities, cost/token, context window
- [ ] 6.6.2 — Task-type classifier: map task types (code, review, test, docs) to preferred models
- [ ] 6.6.3 — Routing rules in `.ralphrc`: `MODEL_ROUTE_CODE=opus`, `MODEL_ROUTE_REVIEW=sonnet`
- [ ] 6.6.4 — Dynamic routing: switch model mid-session based on task type (requires native loop engine)
- [ ] 6.6.5 — Cost optimization: suggest cheaper model when task complexity is below threshold
- **Acceptance:** different task types route to appropriate models, visible in session status

### 6.7 — Replay/audit trail *(new)* `[BLOCKED BY 6.4]`
- [ ] 6.7.1 — Session recording: capture all tool calls, LLM responses, state transitions with timestamps
- [ ] 6.7.2 — Replay viewer: TUI view that steps through session history (forward/backward/seek)
- [ ] 6.7.3 — Export: generate session report as Markdown or JSON (cost, tasks, duration, outcomes)
- [ ] 6.7.4 — Diff view: compare two session replays side-by-side (useful for A/B model testing)
- [ ] 6.7.5 — Retention policy: auto-archive sessions older than N days, configurable in `.ralphrc`
- **Acceptance:** can replay a completed session step-by-step, export as Markdown report

### 6.8 — Multi-model A/B testing `[NEW — PARALLEL]`
- [ ] 6.8.1 — A/B test definition: specify two models + same task, run in parallel worktrees
- [ ] 6.8.2 — Metric collection: capture cost, duration, test pass rate, lint score for each model
- [ ] 6.8.3 — Comparison report: side-by-side results with statistical significance testing
- [ ] 6.8.4 — TUI A/B view: live comparison of two concurrent sessions
- [ ] 6.8.5 — Auto-promote: after N iterations, update default model based on results
- **Acceptance:** `ralphglasses ab-test --model-a opus --model-b sonnet --task "fix lint"` produces comparison

### 6.9 — Natural language fleet control `[NEW — PARALLEL]`
- [ ] 6.9.1 — MCP sampling integration: use `mcpkit/sampling` to parse natural language commands
- [ ] 6.9.2 — Command parser: "start 3 sessions on ralphglasses with $50 budget each" → fleet operations
- [ ] 6.9.3 — Intent classifier: distinguish fleet commands from individual session commands
- [ ] 6.9.4 — Confirmation flow: parse → display plan → confirm → execute
- [ ] 6.9.5 — History: persist and replay natural language commands
- **Acceptance:** natural language commands execute fleet operations via MCP sampling

### 6.10 — Cost forecasting `[NEW — PARALLEL]`
- [ ] 6.10.1 — Historical cost model: regression on (task_type, model, complexity) → predicted_cost
- [ ] 6.10.2 — Budget projection: given remaining budget + historical rates, predict session end time
- [ ] 6.10.3 — TUI forecast widget: show "estimated X hours remaining at current spend rate"
- [ ] 6.10.4 — Alert on anomaly: flag sessions spending >2x their predicted rate
- [ ] 6.10.5 — Recommendation engine: suggest budget adjustments based on historical patterns
- **Acceptance:** forecast accuracy within 20% of actual spend after 10+ sessions

## Phase 7: Kubernetes & Cloud Fleet

> **Depends on:** Phase 5 (sandbox model) + Phase 6 (fleet intelligence)
>
> **Parallel workstreams:** 7.1 (K8s operator) is the foundation. 7.2 (autoscaling) depends on 7.1. 7.3 (multi-cloud) is independent. 7.4 (cost management) depends on 7.1. 7.5 (GitOps) is independent.

### 7.1 — Kubernetes operator `[NEW]`
- [ ] 7.1.1 — CRD definition: `RalphSession` custom resource with spec (repo, model, budget, sandbox)
- [ ] 7.1.2 — Controller: reconcile loop watching `RalphSession` resources, manage pods
- [ ] 7.1.3 — Pod template: Claude Code container with workspace PVC, secret mounts, resource limits
- [ ] 7.1.4 — Status subresource: report session state, spend, progress back to K8s
- [ ] 7.1.5 — RBAC: service account with minimal permissions, namespace isolation
- **Acceptance:** `kubectl apply -f session.yaml` creates and manages a ralph session

### 7.2 — Autoscaling `[BLOCKED BY 7.1]`
- [ ] 7.2.1 — HPA integration: scale session pods based on queue depth (pending tasks)
- [ ] 7.2.2 — Node autoscaler hints: GPU node affinity, spot instance tolerance
- [ ] 7.2.3 — Budget-aware scaling: don't scale beyond remaining budget headroom
- [ ] 7.2.4 — Scale-to-zero: terminate idle sessions after configurable timeout
- [ ] 7.2.5 — Warm pool: maintain N pre-warmed pods for instant session start
- **Acceptance:** session count auto-adjusts based on workload within budget

### 7.3 — Multi-cloud support `[PARALLEL — independent]`
- [ ] 7.3.1 — AWS provider: EC2 instances with GPU, S3 for workspace storage
- [ ] 7.3.2 — GCP provider: GCE instances with L4 GPU, GCS for storage
- [ ] 7.3.3 — Provider interface: `internal/cloud/` with pluggable backends
- [ ] 7.3.4 — Cross-cloud fleet view: unified session list across providers
- [ ] 7.3.5 — Cost comparison: show per-provider pricing for equivalent resources
- **Acceptance:** sessions can launch on AWS or GCP from same TUI

### 7.4 — Cloud cost management `[BLOCKED BY 7.1]`
- [ ] 7.4.1 — Real-time cloud spend tracking: poll cloud billing APIs (AWS Cost Explorer, GCP Billing)
- [ ] 7.4.2 — Combined budget: API spend + cloud compute in unified budget pool
- [ ] 7.4.3 — Spot instance strategy: prefer spot for non-critical sessions, on-demand for time-sensitive
- [ ] 7.4.4 — Idle resource detection: flag running instances with no active sessions
- [ ] 7.4.5 — Weekly cost report: email/webhook summary of cloud + API spend
- **Acceptance:** total cost (API + cloud) visible in single budget view

### 7.5 — GitOps deployment `[PARALLEL — independent]`
- [ ] 7.5.1 — Helm chart: `charts/ralphglasses/` with configurable values
- [ ] 7.5.2 — ArgoCD application: auto-deploy from git, environment overlays
- [ ] 7.5.3 — Kustomize overlays: dev, staging, production configurations
- [ ] 7.5.4 — Sealed secrets: encrypt API keys for git-committed manifests
- [ ] 7.5.5 — Canary deployment: gradual rollout with health check gates
- **Acceptance:** `git push` to deploy branch triggers automated deployment

## Phase 8: Advanced Orchestration & AI-Native Features

> **Depends on:** Phase 6 (fleet intelligence, native loop engine)
>
> **Parallel workstreams:** All sections are independent unless noted.

### 8.1 — Multi-agent collaboration patterns `[NEW]`
- [ ] 8.1.1 — Architect/worker pattern: one session plans tasks, others execute
- [ ] 8.1.2 — Review chain: agent A codes → agent B reviews → agent A fixes feedback
- [ ] 8.1.3 — Specialist routing: route database tasks to DB-expert session, UI tasks to frontend session
- [ ] 8.1.4 — Shared memory: cross-session knowledge base (SQLite) for discovered patterns, conventions
- [ ] 8.1.5 — Communication protocol: structured messages between sessions via SQLite queue
- **Acceptance:** architect/worker pattern produces PRs with automated code review

### 8.2 — Prompt management `[NEW — PARALLEL]`
- [ ] 8.2.1 — Prompt library: `~/.ralphglasses/prompts/` with named prompt templates
- [ ] 8.2.2 — Variable interpolation: `{{repo_name}}`, `{{task_description}}`, `{{context}}` in templates
- [ ] 8.2.3 — Prompt versioning: track prompt changes, roll back to previous versions
- [ ] 8.2.4 — A/B testing: run same task with different prompts, compare outcomes
- [ ] 8.2.5 — TUI prompt editor: view, edit, and test prompts from within the TUI
- **Acceptance:** prompt templates configurable per task type, version-controlled

### 8.3 — Workflow engine `[NEW]` `[BLOCKED BY 6.1]`
- [ ] 8.3.1 — YAML workflow definitions: steps, conditions, parallel branches, error handlers
- [ ] 8.3.2 — Built-in workflows: "fix-all-lint", "increase-coverage", "migrate-dependency"
- [ ] 8.3.3 — Workflow executor: parse YAML → build DAG → assign to sessions → track progress
- [ ] 8.3.4 — Conditional logic: if test fails → create fix task, if coverage < threshold → add tests
- [ ] 8.3.5 — Workflow marketplace: share and discover workflows via git repository
- **Acceptance:** YAML workflow runs multi-step, multi-session task to completion

### 8.4 — Code review automation `[NEW — PARALLEL]`
- [ ] 8.4.1 — PR review agent: auto-review PRs created by other sessions
- [ ] 8.4.2 — Review criteria: configurable rules (test coverage, lint clean, no large files, no secrets)
- [ ] 8.4.3 — GitHub integration: post review comments directly on PR via GitHub API
- [ ] 8.4.4 — Auto-approve: auto-merge PRs that pass all review criteria
- [ ] 8.4.5 — Review dashboard: TUI view of pending/approved/rejected PRs
- **Acceptance:** agent-created PRs automatically reviewed and approved/blocked

### 8.5 — Self-improvement engine `[NEW]` `[BLOCKED BY 6.2]`
- [ ] 8.5.1 — Meta-agent: session that monitors other sessions' effectiveness
- [ ] 8.5.2 — Pattern mining: identify common failure modes, slow tasks, wasted budget
- [ ] 8.5.3 — Config optimization: suggest `.ralphrc` changes based on observed patterns
- [ ] 8.5.4 — Prompt evolution: mutate and test prompts, keep highest-performing variants
- [ ] 8.5.5 — Report generation: weekly summary of fleet performance, trends, recommendations
- **Acceptance:** meta-agent produces actionable configuration improvements

### 8.6 — Codebase knowledge graph `[NEW — PARALLEL]`
- [ ] 8.6.1 — Parse codebase: extract packages, types, functions, dependencies into graph
- [ ] 8.6.2 — Store in SQLite: nodes (entities) and edges (relationships) with metadata
- [ ] 8.6.3 — Query API: "find all callers of function X", "show dependency chain for package Y"
- [ ] 8.6.4 — TUI graph view: interactive dependency visualization (text-mode)
- [ ] 8.6.5 — Context injection: provide relevant graph context to agents before task execution
- **Acceptance:** knowledge graph queries return accurate code relationships

---

## Dependency Chain

```
Phase 0.5 (Critical Fixes) ──→ Phase 1 (Harden) ──→ Phase 1.5 (DX)
                                      │                     │
                                      ↓                     ↓
                               Phase 2 (Multi-Session) ←────┘
                                      │
                               Phase 2.5 (Multi-LLM)
                                      │
                               Phase 2.75 (Event Bus + MCP + TUI) ✅
                                      │
                               ┌──────┴──────┐
                               ↓              ↓
                          Phase 3 (i3)   Phase 5 (Sandbox)
                               │              │
                               ↓              ↓
                          Phase 4 (Thin)  Phase 6 (Intel) ←── 2.75 event bus
                               │              │
                               └──────┬───────┘
                                      ↓
                               Phase 7 (K8s/Cloud)
                                      │
                                      ↓
                               Phase 8 (AI-Native)
```

### Item-Level Dependencies
```
0.5.1 (error fix) ──→ 1.8 (custom error types build on this)
0.5.2 (watcher fix) ──→ 1.7 (structured logging for watcher errors)
0.5.7 (version) ──→ 1.5.2 (release automation needs ldflags)

1.1 ──→ 1.4 (fixtures needed for PID file tests)
1.* ──→ 1.6 (coverage targets depend on all other Phase 1 work)

2.1 ──→ 2.2, 2.3, 2.4, 2.5, 2.8 (session model is foundation)
2.1 + 2.2 + 2.3 ──→ 2.5 (launcher needs worktrees + budget)
2.3 ──→ 5.5 (budget federation extends per-session tracking)
2.11 (health endpoint) ──→ 6.4 (prometheus reuses HTTP server)

3.1 ──→ 3.2, 3.3 (i3 IPC client needed for layout + coordination)
2.1 + 3.1 ──→ 3.3 (multi-instance needs SQLite + i3)

4.1 ──→ 4.2, 4.5, 4.10 (ISO pipeline needed before kiosk + install + USB)
5.1 or 5.2 ──→ 5.4 (network isolation needs a sandbox runtime)

6.1 ──→ 6.2, 6.3, 8.3 (native loop engine needed for orchestrator + coordination + workflows)
6.2 ──→ 8.5 (self-improvement needs R&D cycle)
6.4 ──→ 6.7 (analytics infrastructure needed for replay)

7.1 ──→ 7.2, 7.4 (K8s operator needed for autoscaling + cost mgmt)

2.75.2 (event bus) ──→ 6.4 (analytics builds on event history)
2.75.2 (event bus) ──→ 6.5 (external notifications consume events)
2.75.3 (workflow tools) ──→ 8.3 (workflow engine extends MCP workflows)
```

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
| **Kairos** | Build-your-own | Dockerfile → bootable ISO |
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

## Phase 3.5: Theme & Plugin Ecosystem

> Inspired by k9s skins + plugins system, Ghostty shader architecture,
> Starship module design, and Claude Code skills framework.

### 3.5.1 — Theme ecosystem (like k9s skins + Ghostty themes)
- [ ] 3.5.1.1 — Switch theme colors from ANSI-256 integers to hex strings (lipgloss supports both; hex enables Snazzy/Catppuccin-accurate matching with terminal palette)
- [ ] 3.5.1.2 — Add `snazzy` theme: `#57c7ff` primary, `#ff6ac1` accent, `#5af78e` green, `#f3f99d` yellow, `#ff5c57` red, `#686868` gray, `#1a1a1a` dark, `#f1f1f0` bright — matches Ghostty/k9s/Starship palette ✅ (added to DefaultThemes)
- [ ] 3.5.1.3 — Add `catppuccin-macchiato` and `catppuccin-mocha` themes (popular in k9s/Ghostty/bat/delta ecosystem)
- [ ] 3.5.1.4 — Add `tokyo-night` theme (popular across k9s community)
- [ ] 3.5.1.5 — Support `~/.config/ralphglasses/themes/` external theme directory (YAML files, same schema as LoadTheme)
- [ ] 3.5.1.6 — Add `--theme` CLI flag and `RALPH_THEME` .ralphrc key for theme selection
- [ ] 3.5.1.7 — Add `:theme <name>` TUI command for live theme switching (like k9s skin hotswap)
- **Acceptance:** `ralphglasses --theme snazzy` renders TUI with hex-accurate Snazzy palette; user themes from ~/.config/ralphglasses/themes/ load correctly

### 3.5.2 — Plugin system v2 (like k9s plugins.yml)
Evolve the Phase 2.13 Go `.so` plugin approach into a more accessible YAML-defined command plugin system (like k9s):
- [ ] 3.5.2.1 — Define `~/.config/ralphglasses/plugins.yml` schema: shortcut, description, scopes (repos/sessions/teams/fleet), command, args with variable substitution ($NAME, $REPO, $SESSION_ID, $PROVIDER, $STATUS)
- [ ] 3.5.2.2 — Plugin loader: parse YAML at startup, register keybinds per scope
- [ ] 3.5.2.3 — Variable resolver: substitute runtime context ($NAME, $REPO, etc.) in command args
- [ ] 3.5.2.4 — Built-in plugins: `stern-logs` (tail pod logs via stern for active session's K8s namespace), `gh-pr` (open GitHub PR for session's worktree), `session-cost-report` (pipe session cost data to jq)
- [ ] 3.5.2.5 — Plugin shortcut display in help view (like k9s shows plugin hotkeys)
- [ ] 3.5.2.6 — MCP tool for plugin management: `ralphglasses_plugin_list`, `ralphglasses_plugin_toggle`
- **Acceptance:** user-defined YAML plugins execute commands with variable substitution from TUI

### 3.5.3 — Resource aliases (like k9s aliases.yml)
- [ ] 3.5.3.1 — Define `~/.config/ralphglasses/aliases.yml` schema for TUI command shortcuts
- [ ] 3.5.3.2 — Built-in aliases: `:rp` → repos tab, `:ss` → sessions tab, `:tm` → teams tab, `:fl` → fleet tab
- [ ] 3.5.3.3 — User-defined command aliases: `:deploy <repo>` → custom workflow sequence
- **Acceptance:** `:alias-name` in command mode executes mapped command

### 3.5.4 — MCP skill export (like Claude Code skills)
Export ralphglasses capabilities as Claude Code skills for autonomous agent consumption:
- [ ] 3.5.4.1 — Generate `.claude/skills/ralphglasses/SKILL.md` from MCP tool descriptions
- [ ] 3.5.4.2 — Include YAML frontmatter: `name: ralphglasses`, `description: "Fleet management for multi-LLM agent sessions"`, `allowed-tools: "Bash(ralphglasses *), mcp__ralphglasses__*"`
- [ ] 3.5.4.3 — Auto-update skill on `ralphglasses mcp` server start (regenerate if tool list changed)
- **Acceptance:** Claude Code auto-discovers ralphglasses skill when MCP server is connected

### 3.5.5 — Theme export to terminal (like claudekit themekit)
- [ ] 3.5.5.1 — `ralphglasses theme export ghostty` → generate Ghostty palette config from active theme
- [ ] 3.5.5.2 — `ralphglasses theme export starship` → generate Starship color overrides
- [ ] 3.5.5.3 — `ralphglasses theme export k9s` → generate k9s skin.yml
- [ ] 3.5.5.4 — `ralphglasses theme sync` → export active theme to all supported tools simultaneously
- **Acceptance:** `ralphglasses theme sync` updates Ghostty, Starship, and k9s to match TUI theme
