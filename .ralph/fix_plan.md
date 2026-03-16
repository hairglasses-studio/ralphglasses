# Ralph Fix Plan

## Phase 1: Harden & Test (from ROADMAP.md)

### High Priority
- [x] PID file management for process manager (orphan detection, recovery, cleanup)
- [ ] CI pipeline hardening: add golangci-lint step, coverage threshold enforcement
- [ ] Process manager: auto-restart on crash (with backoff)
- [ ] MCP server: add `ralphglasses_circuit_reset` tool to reset circuit breaker state
- [ ] Structured errors: wrap errors with context throughout, user-facing messages in TUI

### Medium Priority
- [ ] Config editor: add/delete keys, validation for known keys, reload on external change
- [ ] TUI polish: confirm dialogs for destructive actions (stop, stopall)
- [ ] TUI: graceful shutdown (stop all managed processes before exit)
- [ ] MCP server: concurrency guards (prevent concurrent start/stop on same repo)
- [ ] Integration test: full scan → start → status → stop lifecycle test

### Low Priority
- [ ] Process manager: SIGKILL escalation after SIGTERM timeout (30s)
- [ ] TUI: scroll bounds checking for all views
- [ ] Build matrix: linux/amd64, darwin/arm64 in CI

## Completed
- [x] Phase 0: Foundation (all items — see ROADMAP.md)
- [x] Unit tests for all packages (discovery, model, process, mcpserver, tui)
- [x] TUI tests: view rendering, keymap, command mode, filter
- [x] Fuzz tests for parsers and MCP arg extractors
- [x] Benchmark tests for model loading
- [x] BATS tests for marathon.sh
- [x] CI pipeline: go test, go vet, fuzz, benchmarks, BATS
- [x] PID file write on Start, cleanup on Stop/StopAll/process exit
- [x] Orphan process recovery via PID files on startup (Recover method)
- [x] Stale PID file cleanup (CleanStalePIDFiles)
- [x] Race condition fix: use syscall.Kill with stored PID instead of mp.Cmd.Process.Signal
- [x] PID exposed in MCP status detail response
- [x] TUI auto-recovers orphaned loops on scan

## Notes
- Phase 0 is 100% complete with comprehensive test coverage
- PID files are written to `.ralph/ralphglasses.pid` in each repo
- Race detector passes on full test suite
- Focus on Phase 1 hardening before moving to Phase 2 fleet management
