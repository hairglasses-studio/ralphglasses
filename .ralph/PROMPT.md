# Ralphglasses 12-Hour Marathon Loop

## Objective
Continue building and polishing ralphglasses across all phases. Work autonomously through the task list below, committing after each meaningful unit of work.

## Budget & Constraints
- **API Budget**: $100 USD
- **Duration**: 12 hours
- **Model**: sonnet (cost-efficient for iteration-heavy work)
- **Max Calls/Hour**: 80 (960 total budget ceiling)
- **Circuit Breaker**: CB_NO_PROGRESS_THRESHOLD=4, CB_SAME_ERROR_THRESHOLD=5

## Task List (Priority Order)

### 1. Tests
- [ ] Unit tests for `internal/model/` (status.json parsing, config parsing, repo display methods)
- [ ] Unit tests for `internal/discovery/` (scanner with temp directories)
- [ ] Unit tests for `internal/tui/command.go` and `internal/tui/filter.go`
- [ ] Unit tests for `internal/mcpserver/` (tool handlers with mock repos)
- [ ] Integration test: build binary, scan a test directory, verify output

### 2. MCP Server Hardening
- [ ] Add `ralphglasses_circuit_reset` tool to reset a repo's circuit breaker
- [ ] Add `ralphglasses_overview` resource that returns a formatted dashboard string
- [ ] Handle concurrent access in MCP server (scan while listing)
- [ ] Add request logging to stderr for MCP debugging
- [ ] Test MCP server with `echo '{"jsonrpc":"2.0",...}' | go run ./cmd/ralphglasses-mcp`

### 3. TUI Polish
- [ ] Wire `contentHeight` to table/log views for proper scroll bounds
- [ ] Add color-coded status indicators in overview (green dot running, red dot failed)
- [ ] Confirm dialog for destructive actions (stop loop, stop all)
- [ ] Graceful shutdown: trap SIGINT/SIGTERM → stop all managed processes → exit
- [ ] Handle edge case: repo deleted while viewing detail

### 4. Process Manager Improvements
- [ ] Detect externally-running ralph loops (check for PID files in .ralph/)
- [ ] Log process start/stop events to .ralph/logs/
- [ ] Support passing --calls and --timeout flags when starting loops
- [ ] Watchdog: auto-restart loops that crash unexpectedly (opt-in)

### 5. Config Editor Enhancements
- [ ] Add new key support (not just edit existing)
- [ ] Delete key support
- [ ] Validation for known keys (numeric values, boolean values)
- [ ] Show key descriptions from a known-keys registry

### 6. Documentation
- [ ] Update README.md with screenshots/usage
- [ ] Add --help text improvements to Cobra commands

## Working Style
- Run `go build ./...` and `go vet ./...` after every change
- Run `go test ./...` after adding/modifying tests
- Commit after completing each numbered section
- If stuck on a task for >3 iterations, skip and move to next
- Prefer small, correct changes over ambitious ones
