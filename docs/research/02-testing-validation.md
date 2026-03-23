# Research Phase 2: Testing & Validation

**Date:** 2026-03-22
**Agent:** research-02-testing
**Scope:** Test strategy, coverage targets, CI pipeline, fuzz testing, race detection
**ROADMAP Phases:** 0.5, 1 (1.1-1.10), 1.5.10

---

## 1. Executive Summary

Ralphglasses has a substantial test suite with 80+ test files spanning all major packages. Current measured coverage ranges from 100% (discovery) to 16.3% (cmd), with an overall weighted average estimated at ~65%. The ROADMAP targets 85%+ overall with per-package floors (discovery 90%, model 90%, process 85%, mcpserver 85%, tui 70%). The largest coverage gaps are in `tui` (23%), `cmd` (16.3%), and `awesome` (40.6%). Twenty-nine source files have no corresponding test file, with the most dangerous untested paths in `internal/session/failover.go`, `internal/mcpserver/handler_prompt.go`, and `internal/tui/handlers.go`. No CI pipeline exists yet (no `.github/workflows/` directory), making this the single highest-priority infrastructure gap. The project already has three fuzz test files and one integration test with a build tag, but race detection is not yet enforced in any automated way. This document provides 12 concrete, prioritized recommendations to reach the ROADMAP targets.

---

## 2. Current State Analysis

### 2.1 Coverage by Package (Measured)

| Package | Coverage | ROADMAP Target | Delta | Status |
|---------|----------|----------------|-------|--------|
| `internal/discovery` | **100.0%** | 90% | +10% | Exceeds target |
| `internal/events` | **95.1%** | -- | -- | Strong |
| `internal/process` | **92.0%** | 85% | +7% | Exceeds target |
| `internal/model` | **89.0%** | 90% | -1% | Near target |
| `internal/enhancer` | **88.0%** | -- | -- | Strong |
| `internal/hooks` | **82.7%** | -- | -- | Acceptable |
| `internal/repofiles` | **70.8%** | -- | -- | Below ideal |
| `internal/session` | **64.5%** | -- | -- | Gap |
| `internal/roadmap` | **63.1%** | -- | -- | Gap |
| `internal/util` | **62.5%** | -- | -- | Gap |
| `internal/mcpserver` | **FAILING** | 85% | -- | Tests fail (8 failures) |
| `internal/awesome` | **40.6%** | -- | -- | Large gap |
| `internal/tui` | **23.0%** | 70% | -47% | Critical gap |
| `cmd` | **16.3%** | -- | -- | Critical gap |

**Note:** `internal/mcpserver` tests are currently failing (8 test failures in `TestHandleRoadmapParse`, `TestHandleRoadmapAnalyze`, `TestHandleRoadmapExport_*`, `TestHandleRepoScaffold`, `TestHandleRepoOptimize`, `TestValidatePath`). Coverage cannot be measured until these are fixed.

### 2.2 Test Infrastructure Inventory

**Test strategies already in use:**
- **Table-driven tests:** Used extensively in `validate_test.go`, `config_test.go`, `runner_test.go`, `status_test.go`. This is the dominant pattern.
- **Fuzz tests:** 3 fuzz test files covering config parsing (`FuzzLoadConfig`, `FuzzConfigKey`), status/circuit-breaker/progress JSON parsing (`FuzzLoadStatus`, `FuzzLoadCircuitBreaker`, `FuzzLoadProgress`), and MCP argument extraction (`FuzzGetStringArg`, `FuzzGetNumberArg`).
- **Benchmarks:** `bench_test.go` in model package with `BenchmarkLoadConfig`, `BenchmarkLoadStatus`, `BenchmarkRefreshRepo`. Also `bench_test.go` in enhancer package.
- **Integration test:** `internal/integration_test.go` with `//go:build integration` tag covering scan-refresh-config lifecycle.
- **Helper functions:** `setupRalphDir`, `writeJSON`, `makeRepo`, `writeTestScript`, `setupTestServer` -- well-factored test helpers.
- **Process lifecycle tests:** Real `os/exec` tests with `sleep` scripts, PID file management, reaper goroutines, exit status channels.

**Missing test infrastructure:**
- No CI pipeline (no `.github/workflows/` directory)
- No `teatest` / golden snapshot tests for TUI
- No `go test -race` enforcement
- No coverage threshold enforcement
- No `benchstat` regression detection
- No BATS tests for `marathon.sh` in the repo

### 2.3 Untested Source Files

29 source files have no corresponding `_test.go`:

**Critical (high-risk untested paths):**
- `internal/session/failover.go` -- provider failover logic, untested error paths could cause silent session loss
- `internal/mcpserver/handler_prompt.go` -- all prompt enhancement MCP handlers (9 tools), no test coverage
- `internal/tui/handlers.go` -- TUI event dispatch logic, untested could cause panics in production
- `internal/session/checkpoint.go` -- checkpoint save/restore, data loss risk
- `internal/session/gitinfo.go` -- git metadata extraction, could fail silently

**Moderate (functional gaps):**
- `internal/awesome/sync.go` -- full sync pipeline, orchestrates fetch/diff/analyze
- `internal/awesome/auth.go` -- GitHub auth token handling
- `internal/repofiles/protection.go` -- file protection checks
- `internal/roadmap/expand.go` -- roadmap expansion generation
- `internal/session/question.go` -- interactive question handling
- `internal/enhancer/classifier.go` -- prompt type classification
- `internal/enhancer/metaprompt.go` -- meta-prompt generation

**Lower risk (presentational/types):**
- `internal/tui/views/diffview.go`, `sessiondetail.go`, `sessions.go`, `teamdetail.go`, `teams.go`, `timeline.go`
- `internal/tui/components/gauge.go`, `tabbar.go`, `width.go`
- `internal/tui/fleet_builder.go`, `keymap.go`
- `internal/tui/styles/icons.go`, `theme.go`
- `internal/session/types.go`
- `internal/util/debug.go`
- `internal/notify/notify.go`

---

## 3. Gap Analysis

### 3.1 Phase 0.5 Untested Error Paths (Most Dangerous)

| ROADMAP Item | Risk | Current State | What's Missing |
|---|---|---|---|
| 0.5.1 Silent error suppression in RefreshRepo | **HIGH** | `RefreshRepo` now returns `[]error` and tests verify corrupt files produce errors | Error propagation to TUI layer (0.5.1.2, 0.5.1.3) is untested |
| 0.5.2 Watcher error handling | **HIGH** | `WatcherErrorMsg` exists and is tested, timeout fallback works | Auto-fallback to polling (0.5.2.3) and exponential backoff (0.5.2.4) not tested |
| 0.5.3 Process reaper exit status | **MEDIUM** | `TestManager_FailingProcess_ErrorChan` tests non-zero exit. `LastExitStatus` tested | Crash vs clean exit distinction (0.5.3.2) needs TUI integration test |
| 0.5.9 Race condition in MCP scan | **HIGH** | No `sync.RWMutex` in mcpserver, no race testing in CI | No concurrent scan test, no `-race` enforcement |
| 0.5.11 Config validation strictness | **MEDIUM** | Key format validation exists (`TestSave_InvalidKey`), fuzz tests exist | No schema-based validation, no range checks for numeric values |

### 3.2 Phase 1 Testing Gaps

| ROADMAP Item | Current State | Gap |
|---|---|---|
| 1.1 Integration test lifecycle | `integration_test.go` exists with scan/refresh/config cycle | Missing: mock `ralph_loop.sh` lifecycle (start/poll/stop), no CI gate for `-tags=integration` |
| 1.2 MCP server hardening | 8 test failures in tools_test.go, validation tests pass | Tests broken -- must fix before coverage can be measured or improved. Missing: concurrent access tests, structured error codes |
| 1.6 Test coverage targets | No CI enforcement, no coverage badge | Need: `go test -coverprofile` parsing in CI, per-package thresholds, badge generation |
| 1.10 TUI bounds safety | Some bounds tests exist in component tests | Missing: fuzz tests for table rendering, zero-height terminal test, empty-slice audit |

### 3.3 Phase 1.5.10 Benchmarking Gaps

| ROADMAP Item | Current State | Gap |
|---|---|---|
| 1.5.10.1 Go benchmarks | `BenchmarkLoadConfig`, `BenchmarkLoadStatus`, `BenchmarkRefreshRepo` exist in `model/bench_test.go`. Enhancer benchmarks exist | Missing: `Scan` benchmark, table rendering benchmark |
| 1.5.10.2 benchstat in CI | Not implemented | Need: store baseline, run `benchstat old.txt new.txt`, fail on regression |
| 1.5.10.3 Benchmark dashboard | Not implemented | Stretch goal -- low priority vs coverage targets |
| 1.5.10.4 `b.ReportAllocs()` | Not present in existing benchmarks | Quick win -- add to all 3 existing benchmarks |

---

## 4. External Landscape

### 4.1 charmbracelet/x/teatest -- TUI Testing

**Project:** [charmbracelet/x/exp/teatest](https://pkg.go.dev/github.com/charmbracelet/x/exp/teatest)

The official Charmbracelet testing library provides:
- **`teatest.NewProgram()`**: Creates a test program from a `tea.Model`, runs it in-process without a real terminal.
- **Golden file snapshots**: Captures rendered output and compares against `.golden` files. Tests fail when output changes unexpectedly, and developers run with `-update` flag to accept intentional changes.
- **Message injection**: Send `tea.Msg` values directly to the program for deterministic testing without simulating keyboard input.
- **`teatest.WaitFor()`**: Block until output matches a condition, enabling async-safe assertions.

**Relevance to ralphglasses:** The current `app_test.go` manually constructs a `Model`, calls `Update()`, and inspects state -- a valid but limited approach that tests the Elm Architecture model layer but not actual rendered output or multi-message sequences. Adopting `teatest` would enable golden snapshot tests for all views (overview, fleet, repo detail, log stream, help) and catch rendering regressions automatically. The experimental status of `teatest` (published Feb 2026) is acceptable for a project at ralphglasses' maturity level.

**Recommendation:** Use `teatest` for view rendering validation and golden snapshots. Keep the existing direct `Update()` tests for logic (they are faster and more focused).

### 4.2 mark3labs/mcp-go -- MCP Server Testing

**Project:** [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go)

The mcp-go SDK provides a `mcptest` package with:
- **In-memory server/client:** `mcptest.NewServer()` creates a server that can be tested without stdio pipes. Tools are registered, and a test client calls them directly.
- **`NewUnstartedServer()`**: Allows adding tools/prompts/resources before starting, useful for isolated test fixtures.
- **Transport-agnostic testing:** The same server can be tested over stdio, SSE, or in-memory, eliminating flaky transport-layer issues from unit tests.

**Relevance to ralphglasses:** The current `setupTestServer()` in `tools_test.go` creates a `Server` struct and calls handler methods directly by constructing `mcp.CallToolRequest` values -- essentially an in-memory approach but hand-rolled. Migrating to `mcptest` would standardize the pattern and reduce test boilerplate. More importantly, it would enable testing the full MCP request/response lifecycle including tool registration, argument validation, and error code propagation.

**Recommendation:** Evaluate migrating `tools_test.go` to `mcptest.NewServer()` once the 8 failing tests are fixed. Priority is lower than fixing the failures themselves.

### 4.3 Go Coverage and Race Detection Tooling

**go-cover-treemap:** [nikolaydubina/go-cover-treemap](https://github.com/nikolaydubina/go-cover-treemap) generates SVG treemap visualizations from `go test -coverprofile` output. Available as a web tool at go-cover-treemap.io and as a CLI. Useful for identifying coverage cold spots visually.

**Codecov:** [codecov/example-go](https://github.com/codecov/example-go) provides GitHub Actions integration that uploads coverage profiles, tracks trends, and posts PR comments with coverage diffs. Supports per-package thresholds via `codecov.yml`.

**Go race detector:** Built on ThreadSanitizer (TSan). Adds 5-10x memory and 2-20x execution time overhead. Only detects races in code paths actually exercised at runtime. Uber's engineering blog documents their experience running `-race` across their entire Go monorepo in CI, catching hundreds of races before production. Key insight: the race detector is most effective when combined with integration tests and concurrent load tests that exercise real contention patterns, not just unit tests.

**Recommendation:** Add `-race` to all `go test` CI invocations. The overhead is acceptable for CI (not local dev). Use Codecov for coverage tracking with per-package thresholds matching ROADMAP 1.6.1. Use go-cover-treemap for one-time visualization to find cold spots.

---

## 5. Actionable Recommendations

### 5.1 Create CI Pipeline (Priority: P0)

**Target file:** `.github/workflows/ci.yml`
**Effort:** M
**Impact:** Critical -- foundation for all other testing improvements
**ROADMAP items:** 0.5.8.1-0.5.8.4, 1.6.2

No CI pipeline exists. This is the single most impactful gap. Create a GitHub Actions workflow with:

```yaml
# Minimum viable CI pipeline
jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
    steps:
      - go test -race -coverprofile=coverage.txt ./...
      - go test -tags=integration -race ./internal/...
      - go vet ./...
      - golangci-lint run
      - Upload coverage to Codecov
  fuzz:
    steps:
      - go test -fuzz=. -fuzztime=30s ./internal/model/
      - go test -fuzz=. -fuzztime=30s ./internal/mcpserver/
  bench:
    steps:
      - go test -bench=. -benchmem ./internal/model/ > bench.txt
      - benchstat comparison (if baseline exists)
```

Add per-package coverage thresholds enforced in CI per ROADMAP 1.6.1-1.6.2.

### 5.2 Fix Failing MCP Server Tests (Priority: P0)

**Target file:** `internal/mcpserver/tools_test.go`
**Effort:** S
**Impact:** High -- 8 failing tests block coverage measurement for a core package
**ROADMAP items:** 1.2.1-1.2.4

The following tests are currently broken: `TestHandleRoadmapParse`, `TestHandleRoadmapAnalyze`, `TestHandleRoadmapExport_RDCycle`, `TestHandleRoadmapExport_FixPlan`, `TestHandleRoadmapExpand`, `TestHandleRepoScaffold`, `TestHandleRepoOptimize`, `TestValidatePath`. These must be fixed before any coverage improvements can be measured. Root cause investigation needed -- likely a breaking API change in the handler signatures or test fixture setup.

### 5.3 Add Tests for handler_prompt.go (Priority: P1)

**Target file:** `internal/mcpserver/handler_prompt_test.go` (new)
**Effort:** M
**Impact:** High -- 9 MCP tools with zero test coverage (prompt analyze, enhance, lint, improve, templates, classify, should_enhance, claudemd_check, template_fill)
**ROADMAP items:** 1.2.3, 1.6.4

This file contains handlers for all prompt enhancement MCP tools. None are tested. Create table-driven tests for each handler covering:
- Valid inputs with expected output structure
- Missing required arguments (empty prompt, missing provider)
- Edge cases (very long prompts, prompts with special characters)
- Provider-specific behavior (Claude vs Gemini vs OpenAI target)

### 5.4 Add teatest Golden Snapshots for TUI Views (Priority: P1)

**Target file:** `internal/tui/app_golden_test.go` (new), `internal/tui/views/*_golden_test.go` (new)
**Effort:** L
**Impact:** High -- TUI is at 23% coverage vs 70% target, a 47-point gap
**ROADMAP items:** 1.6.4, 1.10.1-1.10.5

Current TUI tests validate state transitions but not rendered output. Add `teatest`-based golden snapshot tests for:
- Overview table with 0, 1, and 10 repos
- Repo detail view with all status combinations (running, idle, crashed, circuit-open)
- Fleet view with multi-session data
- Help view (static content, easy golden test)
- Log stream view with 100+ lines (scroll bounds)
- Config editor with key editing
- Zero-height terminal handling (ROADMAP 1.10.5)

Install: `go get github.com/charmbracelet/x/exp/teatest@latest`

### 5.5 Add Race Detection Tests for MCP Server (Priority: P1)

**Target file:** `internal/mcpserver/tools_race_test.go` (new)
**Effort:** S
**Impact:** High -- ROADMAP 0.5.9 specifically calls out race conditions in MCP scan
**ROADMAP items:** 0.5.9.1-0.5.9.3

Write a concurrent scan test: 10 goroutines calling `handleScan` simultaneously on the same server instance. Also test concurrent `handleList` + `handleStatus` while a scan is in progress. Run with `go test -race`. This will expose whether `repos` map access needs `sync.RWMutex` protection (it does, per ROADMAP 0.5.9.1).

```go
func TestConcurrentScan_Race(t *testing.T) {
    srv, _ := setupTestServer(t)
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            srv.handleScan(context.Background(), mcp.CallToolRequest{})
        }()
    }
    wg.Wait()
}
```

### 5.6 Add Tests for session/failover.go (Priority: P1)

**Target file:** `internal/session/failover_test.go` (new)
**Effort:** M
**Impact:** High -- provider failover is critical path; silent failure means session loss
**ROADMAP items:** 1.6.4

Test the provider failover chain: primary fails -> secondary attempted -> tertiary attempted -> all fail returns error. Test that failover preserves session state (prompt, budget, repo path). Test timeout handling during failover.

### 5.7 Expand Fuzz Testing to Cover JSON Event Parsing (Priority: P2)

**Target file:** `internal/session/providers_fuzz_test.go` (new)
**Effort:** M
**Impact:** Medium -- JSON stream parsing is exposed to arbitrary LLM CLI output
**ROADMAP items:** 1.10.4

The existing fuzz tests cover config parsing and MCP argument extraction. The highest-value expansion is fuzzing the `normalizeEvent()` function in `providers.go`, which parses JSON events from Claude/Gemini/Codex CLI stdout. Malformed JSON from these CLIs could cause panics or silent data loss.

```go
func FuzzNormalizeClaudeEvent(f *testing.F) {
    f.Add(`{"type":"assistant","content":"hello"}`)
    f.Add(`{}`)
    f.Add(`not json`)
    f.Add(`{"type":"`)  // truncated
    f.Fuzz(func(t *testing.T, data string) {
        normalizeClaudeEvent(data)  // must not panic
    })
}
```

### 5.8 Add Coverage Threshold Enforcement (Priority: P2)

**Target file:** `.github/workflows/ci.yml`, `codecov.yml`
**Effort:** S
**Impact:** Medium -- prevents coverage regressions once targets are reached
**ROADMAP items:** 1.6.1-1.6.3

Create `codecov.yml` with per-package thresholds:

```yaml
coverage:
  status:
    project:
      default:
        target: 85%
    patch:
      default:
        target: 90%
  flags:
    discovery: { target: 90% }
    model: { target: 90% }
    process: { target: 85% }
    mcpserver: { target: 85% }
    tui: { target: 70% }
```

Alternative (no external service): parse `go test -coverprofile` output in a shell script and fail CI if any package is below threshold.

### 5.9 Add benchstat Regression Detection (Priority: P2)

**Target file:** `.github/workflows/ci.yml` (bench job), `internal/model/bench_test.go`
**Effort:** S
**Impact:** Medium -- catches performance regressions in hot paths
**ROADMAP items:** 1.5.10.1-1.5.10.4

Add `b.ReportAllocs()` to all 3 existing benchmarks (quick win). Add `BenchmarkScan` in discovery package. Store benchmark baseline as a CI artifact and use `benchstat` to compare.

### 5.10 Add Tests for TUI Handlers and Fleet Builder (Priority: P2)

**Target file:** `internal/tui/handlers_test.go` (new), `internal/tui/fleet_builder_test.go` (new)
**Effort:** M
**Impact:** Medium -- `handlers.go` is the TUI event dispatch hub; untested panics are user-visible
**ROADMAP items:** 1.6.4, 1.10.3

Test all message types handled in the TUI dispatch: `FileChangedMsg`, `ProcessExitMsg`, `SessionUpdateMsg`, `BudgetAlertMsg`. Verify no panics on nil repos, empty session lists, or zero-width terminals.

### 5.11 Add Tests for session/checkpoint.go (Priority: P2)

**Target file:** `internal/session/checkpoint_test.go` (new)
**Effort:** S
**Impact:** Medium -- checkpoint save/restore is a data integrity path
**ROADMAP items:** 1.6.4

Test checkpoint creation (writes expected files), restoration (loads state correctly), and corruption handling (graceful error on malformed checkpoint).

### 5.12 Add Integration Test for Full Session Lifecycle (Priority: P2)

**Target file:** `internal/integration_test.go` (extend)
**Effort:** L
**Impact:** High -- validates the entire scan-launch-poll-stop pipeline
**ROADMAP items:** 1.1.1-1.1.4

The existing integration test covers scan/refresh/config. Extend with:
- Mock `ralph_loop.sh` that writes status updates on a timer
- Session launch via `Manager.Launch()` with mock provider
- Poll session status until completion
- Verify cost ledger and journal entries
- Clean shutdown with `StopAll()`

---

## 6. Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| MCP server race condition causes data corruption in production | **High** | **High** | Implement 5.5 (race tests) + ROADMAP 0.5.9.1 (mutex) before any multi-user deployment |
| TUI panics on edge-case terminal sizes | **Medium** | **Medium** | Implement 5.4 (golden snapshots) + ROADMAP 1.10.5 (zero-height guard) |
| Failing tests mask new regressions | **High** | **High** | Implement 5.2 (fix failing tests) immediately -- broken tests are worse than no tests |
| Coverage regressions during rapid feature development | **Medium** | **Medium** | Implement 5.8 (threshold enforcement) in CI |
| Provider failover silently drops sessions | **Medium** | **High** | Implement 5.6 (failover tests) before multi-provider production use |
| Fuzz corpus too small to find real parser bugs | **Low** | **Medium** | Extend fuzz time in CI to 60s+ per target; add corpus entries from real-world malformed data |
| `teatest` API instability (experimental package) | **Low** | **Low** | Pin version, wrap in local helper functions to isolate from API changes |
| CI pipeline too slow for developer iteration | **Medium** | **Low** | Split into fast (test+vet, ~2min) and slow (fuzz+bench+integration, ~10min) jobs |

---

## 7. Implementation Priority Ordering

### 7.1 Immediate (This Sprint)

| # | Action | Effort | ROADMAP | Impact |
|---|--------|--------|---------|--------|
| 1 | Fix 8 failing mcpserver tests | S | 1.2 | Unblocks coverage measurement |
| 2 | Create `.github/workflows/ci.yml` | M | 0.5.8, 1.6.2 | Foundation for all CI enforcement |
| 3 | Add `-race` flag to all `go test` invocations | S | 0.5.9.2 | Catches races immediately |
| 4 | Add concurrent scan race test | S | 0.5.9.3 | Validates mutex need |

### 7.2 Next Sprint

| # | Action | Effort | ROADMAP | Impact |
|---|--------|--------|---------|--------|
| 5 | Add `handler_prompt_test.go` | M | 1.2.3, 1.6.4 | 9 untested MCP tools |
| 6 | Add `failover_test.go` | M | 1.6.4 | Critical session safety |
| 7 | Add `teatest` golden snapshots for 4 core views | L | 1.6.4, 1.10 | 23% -> ~50% TUI coverage |
| 8 | Add `b.ReportAllocs()` to existing benchmarks | S | 1.5.10.4 | Quick win, 5-minute change |
| 9 | Add coverage threshold enforcement | S | 1.6.1-1.6.3 | Prevents regressions |

### 7.3 Backlog

| # | Action | Effort | ROADMAP | Impact |
|---|--------|--------|---------|--------|
| 10 | Fuzz test `normalizeEvent()` for all 3 providers | M | 1.10.4 | Parser safety |
| 11 | Add `handlers_test.go` and `fleet_builder_test.go` | M | 1.6.4, 1.10.3 | TUI dispatch coverage |
| 12 | Add `checkpoint_test.go` | S | 1.6.4 | Data integrity |
| 13 | Extend integration test with mock session lifecycle | L | 1.1.1-1.1.4 | End-to-end validation |
| 14 | Add `benchstat` CI comparison | S | 1.5.10.2 | Perf regression detection |
| 15 | Evaluate `mcptest` package migration | M | 1.2 | Test infrastructure modernization |
| 16 | Add golden snapshots for remaining 6 TUI views | M | 1.6.4 | TUI coverage to 70%+ |

---

## References

- [charmbracelet/x/exp/teatest](https://pkg.go.dev/github.com/charmbracelet/x/exp/teatest) -- Bubble Tea testing library
- [Writing Bubble Tea Tests](https://charm.land/blog/teatest/) -- Official Charm blog post on teatest
- [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) -- Go MCP SDK with `mcptest` package
- [mcptest package docs](https://pkg.go.dev/github.com/mathiasXie/mcp-go/mcptest) -- In-memory MCP server testing
- [Unit Testing MCP Servers](https://mcpcat.io/guides/writing-unit-tests-mcp-servers/) -- MCP testing guide
- [nikolaydubina/go-cover-treemap](https://github.com/nikolaydubina/go-cover-treemap) -- Coverage visualization
- [codecov/example-go](https://github.com/codecov/example-go) -- Codecov Go integration
- [Go Race Detector](https://go.dev/doc/articles/race_detector) -- Official race detector docs
- [Uber Dynamic Data Race Detection](https://www.uber.com/blog/dynamic-data-race-detection-in-go-code/) -- Production race detection at scale
- [Go Fuzz Testing](https://go.dev/doc/articles/fuzz/) -- Official fuzzing documentation
- [encoding/json fuzz_test.go](https://go.dev/src/encoding/json/fuzz_test.go) -- stdlib JSON fuzz test patterns
- [Fixing a fuzzed-up function](https://bitfieldconsulting.com/posts/bugs-fuzzing) -- Real bugs found via Go fuzzing
