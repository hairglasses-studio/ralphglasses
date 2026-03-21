# Cross-Machine Test Suite for Ralphglasses

**Paste this entire prompt into a Claude Code session on the target machine after cloning the repo.**

---

You are running a comprehensive use-case test suite for the "ralphglasses" Go project. This is a command-and-control TUI + MCP server for managing parallel multi-LLM agent fleets.

**First:** Read `CLAUDE.md` for project context.

**Environment notes:**
- You may NOT have `ANTHROPIC_API_KEY`, `GOOGLE_API_KEY`, or `OPENAI_API_KEY` set. Tests that launch real LLM sessions should verify graceful error handling, not successful launches.
- The project uses Go 1.22+. Confirm with `go version`.
- Run all tests from the repo root directory.
- For MCP stdio tests, use `printf '...\n...\n' | RALPHGLASSES_SCAN_PATH="$TEST_ROOT" go run ./cmd/ralphglasses-mcp` pattern.

Execute each test IN ORDER. For each test: run the command, verify the expected outcome, report PASS or FAIL with details. If FAIL, capture full error output.

**Setup:** Create a mock repo fixture used by Categories C and I:

```bash
TEST_ROOT=$(mktemp -d)
REPO_DIR="$TEST_ROOT/test-repo"
mkdir -p "$REPO_DIR/.ralph/logs"
echo '{"loop_count":10,"status":"running","calls_made_this_hour":5,"max_calls_per_hour":100}' > "$REPO_DIR/.ralph/status.json"
echo '{"state":"CLOSED","total_opens":0}' > "$REPO_DIR/.ralph/.circuit_breaker_state"
echo '{"iteration":3,"status":"in_progress","completed_ids":["task-1"]}' > "$REPO_DIR/.ralph/progress.json"
echo 'MODEL=sonnet' > "$REPO_DIR/.ralphrc"
echo 'log line 1' > "$REPO_DIR/.ralph/logs/ralph.log"
printf 'module test\n\ngo 1.22\n' > "$REPO_DIR/go.mod"
printf '# Test Roadmap\n## Phase 1\n- [ ] Task 1\n- [x] Task 2\n' > "$REPO_DIR/ROADMAP.md"
```

---

## Category A: Build & Compile (7 tests)

**A1.** `go build ./...` — Expected: exit 0, no output.

**A2.** `go vet ./...` — Expected: exit 0, no output.

**A3.** `go test ./...` — Expected: exit 0, all packages pass with "ok" lines.

**A4.** `go test -race ./...` — Expected: exit 0, no data races.

**A5.** `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null .` — Expected: exit 0 (cross-compiles).

**A6.** `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /dev/null .` — Expected: exit 0.

**A7.** `go mod tidy && git diff --exit-code go.mod go.sum` — Expected: exit 0, no diff.

---

## Category B: CLI & Help (6 tests)

**B1.** `go run . --help` — Expected: output contains "Command-and-control TUI" and `--scan-path`.

**B2.** `go run . --version` — Expected: prints version string.

**B3.** `go run . mcp --help` — Expected: output contains "MCP server" or "stdio".

**B4.** `go run . completion bash | head -5` — Expected: generates bash completion script.

**B5.** `go run . completion zsh | head -5` — Expected: generates zsh completion script.

**B6.** `go run . completion powershell 2>&1; echo "exit=$?"` — Expected: error containing "unsupported shell", non-zero exit.

---

## Category C: MCP Server via stdio (20 tests)

Use the `$TEST_ROOT` fixture from Setup. For each test, pipe JSON-RPC messages to the MCP server. The pattern is:

```bash
printf 'INIT_LINE\nCALL_LINE\n' | RALPHGLASSES_SCAN_PATH="$TEST_ROOT" go run ./cmd/ralphglasses-mcp 2>/dev/null
```

Where `INIT_LINE` is always:
```
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
```

**C1. Initialize handshake** — Send only INIT_LINE. Expected: response contains `"serverInfo"` with name `"ralphglasses"`.

**C2. List tools** — Call `tools/list`. Expected: response contains `"tools"` array. Count tool names — should be 38.

**C3. ralphglasses_scan** — Call with `{"name":"ralphglasses_scan","arguments":{}}`. Expected: contains "1" and "repo" (found 1 repo).

**C4. ralphglasses_list** — Call. Expected: contains "test-repo".

**C5. ralphglasses_status** — Call with `{"repo":"test-repo"}`. Expected: contains loop_count or status info.

**C6. ralphglasses_status missing arg** — Call with `{}`. Expected: `"isError":true`.

**C7. ralphglasses_logs** — Call with `{"repo":"test-repo"}`. Expected: contains "log line 1".

**C8. ralphglasses_config list** — Call with `{"repo":"test-repo"}`. Expected: contains "MODEL".

**C9. ralphglasses_config get** — Call with `{"repo":"test-repo","key":"MODEL"}`. Expected: contains "sonnet".

**C10. ralphglasses_fleet_status** — Call with `{}`. Expected: contains fleet data (repos count, sessions).

**C11. ralphglasses_roadmap_parse** — Call with `{"path":"$REPO_DIR"}` (substitute actual path). Expected: contains "Phase 1".

**C12. ralphglasses_stop_all** — Call with `{}`. Expected: contains "stopped" or "0" (no loops running).

**C13. ralphglasses_session_list** — Call with `{}`. Expected: returns empty list or "0 sessions", no error.

**C14. ralphglasses_agent_list** — Call with `{"repo":"test-repo"}`. Expected: empty list, no error.

**C15. ralphglasses_event_list** — Call with `{}`. Expected: empty event list, no error.

**C16. ralphglasses_repo_scaffold** — Create `$TEST_ROOT/new-repo` with `go.mod`, call with `{"path":"$TEST_ROOT/new-repo"}`. Expected: creates `.ralphrc` and `.ralph/PROMPT.md`.

**C17. ralphglasses_repo_optimize** — Call with `{"path":"$REPO_DIR"}`. Expected: returns optimization report.

**C18. ralphglasses_repo_health** — Call with `{"repo":"test-repo"}`. Expected: returns health report with score.

**C19. ralphglasses_snapshot save** — Call with `{"action":"save"}`. Expected: no error.

**C20. ralphglasses_session_launch without API key** — Call with `{"repo":"test-repo","prompt":"hello"}`. Expected: errors (no API key) or launches and fails quickly. Must not hang.

---

## Category D: Session Management Unit Tests (8 tests)

**D1.** `go test -v -run TestValidateProviderUnknown ./internal/session/` — PASS

**D2.** `go test -v -run TestProviderDefaults ./internal/session/` — PASS

**D3.** `go test -v -run TestBuildCmd ./internal/session/` — PASS

**D4.** `go test -v -run TestBuildGeminiCmd ./internal/session/` — PASS

**D5.** `go test -v -run TestBuildCodexCmd ./internal/session/` — PASS

**D6.** `go test -v -run TestBudgetEnforcer ./internal/session/` — PASS (all 3 subtests)

**D7.** `go test -v -run TestWriteAndListAgents ./internal/session/` — PASS

**D8.** `go test -v -run TestManager ./internal/session/` — PASS (all 7 manager subtests: NewManager, ListEmpty, GetNotFound, StopNotFound, IsRunningEmpty, GetTeamNotFound, ListTeamsEmpty, StopAlreadyStopped, FindByRepo)

---

## Category E: Event Bus (7 tests)

**E1.** `go test -v -run TestPublishSubscribe ./internal/events/` — PASS

**E2.** `go test -v -run TestUnsubscribe ./internal/events/` — PASS

**E3.** `go test -v -run TestHistory ./internal/events/` — PASS

**E4.** `go test -v -run TestHistorySince ./internal/events/` — PASS

**E5.** `go test -v -run TestHistoryRingBuffer ./internal/events/` — PASS

**E6.** `go test -v -run TestMultipleSubscribers ./internal/events/` — PASS

**E7.** `go test -v -run TestOverflow ./internal/events/` — PASS

---

## Category F: Hook System (4 tests)

**F1.** `go test -v -run TestLoadConfigNotExist ./internal/hooks/` — PASS

**F2.** `go test -v -run TestLoadConfigValid ./internal/hooks/` — PASS

**F3.** `go test -v -run TestDispatchHook ./internal/hooks/` — PASS

**F4.** `go test -v -run TestStartStop ./internal/hooks/` — PASS

---

## Category G: TUI Components (18 tests)

**G1.** `go test -v -run TestNewTable ./internal/tui/components/` — PASS

**G2.** `go test -v -run TestSelectedRow$ ./internal/tui/components/` — PASS

**G3.** `go test -v -run TestSelectedRowEmpty ./internal/tui/components/` — PASS

**G4.** `go test -v -run TestMoveDownUp ./internal/tui/components/` — PASS

**G5.** `go test -v -run TestFilter$ ./internal/tui/components/` — PASS

**G6.** `go test -v -run TestFilterCaseInsensitive ./internal/tui/components/` — PASS

**G7.** `go test -v -run TestCycleSort ./internal/tui/components/` — PASS

**G8.** `go test -v -run "TestView$" ./internal/tui/components/` — PASS

**G9.** `go test -v -run TestViewEmpty ./internal/tui/components/` — PASS

**G10.** `go test -v -run TestConfirmDialog ./internal/tui/components/` — PASS (all 4 subtests)

**G11.** `go test -v -run TestActionMenu ./internal/tui/components/` — PASS (all 5 subtests)

**G12.** `go test -v -run TestNewSessionLauncher ./internal/tui/components/` — PASS

**G13.** `go test -v -run TestCycleProvider ./internal/tui/components/` — PASS

**G14.** `go test -v -run TestLauncher ./internal/tui/components/` — PASS (Navigate, Edit, Submit, Escape, View)

**G15.** `go test -v -run TestBreadcrumb ./internal/tui/components/` — PASS

**G16.** `go test -v -run TestNotification ./internal/tui/components/` — PASS

**G17.** `go test -v -run TestStatusBar ./internal/tui/components/` — PASS

**G18.** `go test -v -run TestFormatAgo ./internal/tui/components/` — PASS

---

## Category H: View Rendering (8 tests)

**H1.** `go test -v -run TestNewConfigEditor ./internal/tui/views/` — PASS

**H2.** `go test -v -run TestConfigEditor ./internal/tui/views/` — PASS (all subtests)

**H3.** `go test -v -run TestRenderHelp ./internal/tui/views/` — PASS (all 3 subtests)

**H4.** `go test -v -run TestNewLogView ./internal/tui/views/` — PASS

**H5.** `go test -v -run TestAppendLines ./internal/tui/views/` — PASS

**H6.** `go test -v -run TestScroll ./internal/tui/views/` — PASS (all scroll tests)

**H7.** `go test -v -run TestToggleFollow ./internal/tui/views/` — PASS

**H8.** `go test -v ./internal/tui/styles/` — PASS (TestStatusStyle, TestCBStyle)

---

## Category I: TUI App Integration (10 tests)

**I1.** `go test -v -run TestNewModel ./internal/tui/` — PASS

**I2.** `go test -v -run TestInit ./internal/tui/` — PASS

**I3.** `go test -v -run TestViewStackPushPop ./internal/tui/` — PASS

**I4.** `go test -v -run TestWindowSizeMsg ./internal/tui/` — PASS

**I5.** `go test -v -run TestHandleKeyQuit ./internal/tui/` — PASS

**I6.** `go test -v -run TestHandleKeyEscAtRoot ./internal/tui/` — PASS

**I7.** `go test -v -run TestHandleKeyEscPopsView ./internal/tui/` — PASS

**I8.** `go test -v -run TestCommandModeInput ./internal/tui/` — PASS

**I9.** `go test -v -run TestFindRepoByName ./internal/tui/` — PASS

**I10.** `go test -v -run TestView$ ./internal/tui/` — PASS (renders without panic)

---

## Category J: MCP Server Unit Tests (12 tests)

**J1.** `go test -v -run TestNewServer ./internal/mcpserver/` — PASS

**J2.** `go test -v -run TestHandleScan ./internal/mcpserver/` — PASS

**J3.** `go test -v -run TestHandleList$ ./internal/mcpserver/` — PASS

**J4.** `go test -v -run TestHandleStatus$ ./internal/mcpserver/` — PASS

**J5.** `go test -v -run TestHandleStatus_MissingRepoArg ./internal/mcpserver/` — PASS

**J6.** `go test -v -run TestHandleStatus_ScanError ./internal/mcpserver/` — PASS

**J7.** `go test -v -run TestHandleLogs$ ./internal/mcpserver/` — PASS

**J8.** `go test -v -run TestHandleConfig_ListAll ./internal/mcpserver/` — PASS

**J9.** `go test -v -run TestGetStringArg ./internal/mcpserver/` — PASS

**J10.** `go test -v -run TestGetNumberArg ./internal/mcpserver/` — PASS

**J11.** `go test -v -run TestHandleRoadmapParse$ ./internal/mcpserver/` — PASS

**J12.** `go test -v -run TestHandleRepoScaffold$ ./internal/mcpserver/` — PASS

---

## Category K: Package-Level Full Runs (6 tests)

**K1.** `go test -v ./internal/discovery/` — PASS (100% coverage)

**K2.** `go test -v ./internal/model/` — PASS (95.6% coverage)

**K3.** `go test -v ./internal/process/` — PASS (86% coverage)

**K4.** `go test -v ./internal/repofiles/` — PASS

**K5.** `go test -v ./internal/roadmap/` — PASS

**K6.** `go test -v ./internal/util/` — PASS (100% coverage)

---

## Category L: Edge Cases & Fuzz (4 tests)

**L1.** `go test -v -run TestNormalizeEventEmptyLine ./internal/session/` — PASS

**L2.** `go test -v -run TestNormalizeEventInvalidJSON ./internal/session/` — PASS

**L3.** `go test -fuzz=FuzzLoadConfig -fuzztime=10s ./internal/model/` — No crashes in 10s.

**L4.** `go test -fuzz=FuzzGetStringArg -fuzztime=10s ./internal/mcpserver/` — No crashes in 10s.

---

## Cleanup

```bash
rm -rf "$TEST_ROOT"
```

---

## Summary Template

Report results in this format:

```
=== RALPHGLASSES TEST RESULTS ===
Machine: [OS / arch / Go version]
Date: [date]

Category A (Build & Compile):     _/7 passed
Category B (CLI & Help):          _/6 passed
Category C (MCP Server stdio):    _/20 passed
Category D (Session Unit):        _/8 passed
Category E (Event Bus):           _/7 passed
Category F (Hooks):               _/4 passed
Category G (TUI Components):      _/18 passed
Category H (View Rendering):      _/8 passed
Category I (TUI App Integration): _/10 passed
Category J (MCP Server Unit):     _/12 passed
Category K (Package Full Runs):   _/6 passed
Category L (Edge Cases & Fuzz):   _/4 passed
TOTAL:                            _/110 passed

FAILURES (if any):
- [Test ID]: [Brief description]
```
