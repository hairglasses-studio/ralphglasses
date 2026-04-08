# Architecture

## Package Layout

- **main.go** → **cmd/root.go**: Cobra CLI with `--scan-path` flag
- **internal/discovery/**: Scans directories for `.ralph/` and `.ralphrc`
- **internal/model/**: Data types and parsers for status.json, progress.json, circuit breaker state, .ralphrc
- **internal/process/**: Process management (launch/stop/pause via os/exec), fsnotify file watcher, log tailing
- **internal/session/**: Multi-provider LLM session management (claude/gemini/codex), agent teams, budget enforcement, provider dispatch, concurrent worker fan-out, autonomy levels, auto-optimization, auto-recovery, context store, HITL metrics, feedback profiling, prompt caching, self-improvement pipeline (reflexion, episodic memory, cascade routing, curriculum sorting, bandit-based provider selection), supervisor (session lifecycle orchestration, health monitor, cycle chainer), sentinel errors, acceptance testing
- **internal/batch/**: Batch API support for Claude, Gemini, and OpenAI — submit non-interactive workloads at 50% discount
- **internal/wsclient/**: WebSocket transport client for OpenAI Responses API (40% faster for multi-turn tool chains)
- **internal/mcpserver/**: MCP tool handlers (218 grouped tools + 4 management tools across 30 deferred-loaded tool groups, stdio transport via mcp-go)
  - `tools.go` — Server struct, constructors, Register()
  - `tools_builders.go` — Tool definition builders for all deferred-load tool groups
  - `tools_dispatch.go` — Dispatch table routing tool names to handlers
  - `handler_cli_parity.go` — CLI parity handlers for doctor, validate, config schema, debug bundle, telemetry, firstboot profile, fleet runtime, marathon, and repo surface audit
  - `handler_prompt.go` — Multi-provider prompt enhancement handlers
  - `handler_fleet.go` — Distributed fleet, HITL, autonomy, and feedback profile handlers
  - `handler_fleet_h.go` — Fleet intelligence: blackboard coordination, A2A offers, cost forecasting
  - `handler_session.go` — Session lifecycle handlers (launch, list, status, stop, etc.)
  - `handler_loop.go` — Loop lifecycle and control handlers
  - `handler_loopbench.go` — Loop benchmarking and baseline handlers
  - `handler_loopwait.go` — Loop await/poll handlers (replaces sleep anti-pattern)
  - `handler_eval.go` — Offline evaluation: counterfactual, A/B test, changepoints
  - `handler_anomaly.go` — Anomaly detection via sliding-window z-score analysis
  - `handler_observation.go` — Observation query and summary handlers
  - `handler_scratchpad.go` — Scratchpad read/append/list/resolve handlers
  - `handler_selftest.go` — Recursive self-test handler
  - `handler_selfimprove.go` — Self-improvement loop handler
  - `handler_rc.go` — Remote control (mobile-friendly) handlers
  - `handler_repo.go` — Repo health, optimize, scaffold, claudemd_check, snapshot
  - `handler_roadmap.go` — Roadmap parse, analyze, research, expand, export
  - `handler_team.go` — Agent team create, delegate, status, agent definitions
  - `handler_awesome.go` — Awesome-list fetch, analyze, diff, report, sync
  - `handler_circuit.go` — Circuit breaker reset handler
  - `handler_costestimate.go` — Pre-launch cost estimation
  - `handler_coverage.go` — Go test coverage reporting
  - `handler_mergeverify.go` — Build+vet+test merge verification
  - `middleware.go` — Composable middleware: InstrumentationMiddleware, EventBusMiddleware, ValidationMiddleware
  - `timeout.go` — TimeoutMiddleware with per-tool overrides and exemptions
  - `toolbench.go` — Auto-benchmarking with JSONL logging, P50/P95 latencies, regression detection
  - `annotations.go` — MCP tool annotation helpers
  - `errors.go` — Coded error helpers and error constants
  - `validate.go` — Path and repo name validation
  - `schemas.go` — Shared JSON schema definitions
  - `helpers.go` — Common handler helper functions
  - `mcplog.go` — MCP-aware structured logging
- **internal/events/**: Typed publish-subscribe event bus with `PublishCtx(ctx, Event)`, filtered subscriptions, history ring buffer, event query, and schema migration
- **internal/enhancer/**: Prompt enhancement pipeline (13-stage), scoring, lint, multi-provider LLM improvement
- **internal/roadmap/**: Roadmap parsing, analysis, research, expansion, export
- **internal/repofiles/**: Ralph config file scaffolding and optimization
- **internal/parity/**: Shared CLI/MCP parity services for doctor, validate, debug bundle, config schema, telemetry, theme export, repo surface audit, and worktree inspection
- **internal/firstboot/**: Firstboot profile load/save/validate helpers shared by CLI and MCP
- **internal/automation/**: Shared serve-runtime automation lifecycle helpers
- **internal/fleet/**: Distributed fleet coordination — HTTP coordinator/worker nodes, priority queue, cost optimizer, Tailscale-based discovery
- **internal/sandbox/**: Docker container isolation for sessions (create, start, exec, stop, cleanup)
- **internal/tracing/**: OpenTelemetry GenAI semantic tracing + Prometheus metrics recorder
- **internal/plugin/**: Plugin system — registry, file-based loader, builtin logger plugin
- **internal/e2e/**: E2E scenario test harness with mock launch/wait hooks
- **cmd/mcp.go**: MCP server subcommand (`go run . mcp`)
- **cmd/doctor.go**: Environment verification (required binaries, API keys, scan-path accessibility)
- **cmd/serve.go**: Fleet node (`--coordinator` for leader, `--coordinator-url` for worker)
- **cmd/validate.go**: Validate all `.ralphrc` configs across scan-path repos
- **internal/tui/**: Bubble Tea app model, keymap, command/filter modes
- **internal/tui/styles/**: Lipgloss theme (k9s-inspired, no other package imports this)
- **internal/tui/components/**: Reusable widgets (sortable table, breadcrumb, status bar, notifications)
- **internal/tui/views/**: View renderers (overview, repo detail, log stream, config editor, help)

## Provider Architecture

The `internal/session/` package uses a provider dispatch pattern:

- **`providers.go`**: `buildCmdForProvider()` dispatches to `buildClaudeCmd()`, `buildGeminiCmd()`, or `buildCodexCmd()`. `normalizeEvent()` dispatches to per-provider event normalizers. `ValidateProvider()` checks CLI binary availability.
- **`runner.go`**: Provider-agnostic session lifecycle. Calls `buildCmdForProvider()` for command construction and `normalizeEvent()` for stream parsing.
- **`types.go`**: `Provider` type (`claude`|`gemini`|`codex`) used in `Session`, `LaunchOptions`, `TeamConfig`.
- **`budget.go`**: `LedgerEntry` and `CostSummary` include `Provider` field for per-provider cost tracking.

### Adding a New Provider

1. Add constant to `Provider` in `types.go`
2. Add binary name in `providerBinary()` in `providers.go`
3. Add `buildXxxCmd()` function in `providers.go`
4. Add `normalizeXxxEvent()` function in `providers.go`
5. Add default model in `ProviderDefaults()` in `providers.go`
6. Add cases in `buildCmdForProvider()` and `normalizeEvent()` switch statements
7. Add tests in `providers_test.go`

## Middleware & Instrumentation

The MCP server supports composable middleware (`internal/mcpserver/middleware.go`, `internal/mcpserver/timeout.go`):

- **InstrumentationMiddleware**: Records timing, success/fail, input/output size for every tool call. Emits structured `slog` logs (`mcp.tool.call`) with tool name, duration, success status, repo correlation, and error details. Pushes counters to Prometheus when a `PrometheusRecorder` is configured.
- **TimeoutMiddleware**: Wraps each handler with a `context.WithTimeout` deadline. Supports per-tool overrides via a `map[string]time.Duration` — a positive duration sets a custom timeout, a zero duration exempts the tool entirely from timeout enforcement.
- **EventBusMiddleware**: Publishes `tool.called` events to the fleet event bus via `bus.PublishCtx(ctx, ...)` for real-time monitoring, respecting context cancellation.
- **ValidationMiddleware**: Pre-validates common parameters (`repo`, `path`) before handler execution. Validates absolute paths against the scan root and relative repo names against naming rules.

### Tool Benchmarking

`internal/mcpserver/toolbench.go` provides auto-benchmarking across the MCP tool surface:

- **JSONL recording**: All tool calls logged with latency, success, error, sizes
- **Percentile summaries**: P50, P95, max latency per tool
- **Regression detection**: Compares current metrics against baseline, flags degradations by severity
- **MCP tool**: `ralphglasses_tool_benchmark` exposes metrics and regression analysis

## Distributed Fleet

The `internal/fleet/` package enables multi-machine workload distribution:

- **`server.go`**: HTTP coordinator — accepts work submissions, tracks workers via heartbeats, enforces fleet-wide budget
- **`worker.go`**: Polls coordinator for work, executes sessions locally, reports results
- **`queue.go`**: Priority queue with cost-aware scheduling and provider affinity
- **`optimizer.go`**: Cost optimizer — routes tasks to cheapest capable provider/worker
- **`discovery.go`**: Tailscale-based fleet discovery (automatic peer detection)
- **`client.go`**: HTTP client for worker-to-coordinator communication

Start with `ralphglasses serve --coordinator` (one node) and `ralphglasses serve --coordinator-url <url>` (worker nodes).

## Tiered Model Routing

The cost optimizer uses 4-tier routing to balance cost and capability:

| Tier | Model | Cost (input/1M) | Routed Tasks |
|------|-------|-----------------|--------------|
| Ultra-cheap | Gemini Flash-Lite | $0.10 | Classification, routing, simple extraction |
| Worker | Gemini Flash | $0.30 | Bulk codegen, tests, docs |
| Coding | Claude Sonnet | $3.00 | Architecture, complex refactoring |
| Reasoning | Claude Opus | $15.00 | Planning, multi-step reasoning |

The `--effort` CLI flag influences tier selection: `low` prefers ultra-cheap/worker tiers, `max` allows reasoning tier. The `ralphglasses_provider_recommend` tool uses feedback profiles to suggest the best tier for a given task.

## Prompt Caching Strategy

All three providers support prompt caching for 80-90% input cost savings:

- **Claude (Anthropic SDK)**: Automatic `cache_control` breakpoints on system prompts and tool definitions. The SDK handles cache key generation and TTL.
- **Gemini**: Explicit `cachedContents` API — the session manager creates cache entries with configurable TTL for system instructions and large context windows. Thinking budget control via `thinkingConfig`.
- **OpenAI**: Automatic prefix caching on the Responses API. Structured output via `OutputSchema` for JSON schema validation.

Cache hit rates are tracked in the cost ledger and surfaced via `ralphglasses_fleet_analytics`.

## Event Bus

The `internal/events/` package provides a typed publish-subscribe event bus for system-wide coordination:

- **`bus.go`**: `Bus` struct with `PublishCtx(ctx, Event)` (context-aware, returns error on cancellation) and backward-compatible `Publish(Event)` wrapper. Supports `Subscribe(id)` for all events and `SubscribeFiltered(id, ...EventType)` for selective subscription. Maintains a configurable history ring buffer with `History(filter, limit)` and optional retention TTL.
- **`query.go`**: `EventQuery` struct for filtering events by type, time range, and other criteria.
- **`migration.go`**: Event schema migration for backward compatibility.
- **30+ event types**: Session lifecycle (`session.started`, `session.ended`), cost (`cost.update`, `budget.exceeded`), loop lifecycle (`loop.started`, `loop.iterated`, `loop.regression`), self-improvement (`auto.optimized`, `provider.selected`, `session.recovered`), tool instrumentation (`tool.called`), worker lifecycle (`worker.deregistered`, `worker.paused`, `worker.resumed`), and more.

## Process Management

The `internal/process/` package manages OS processes with full `context.Context` threading:

- **`manager.go`**: `Manager.Start(ctx, repoPath)` uses `exec.CommandContext` so context cancellation kills the launched process and prevents auto-restarts. `Manager.Stop(ctx, repoPath)` and `Manager.StopAll(ctx)` also accept contexts for deadline-aware shutdown. The reaper goroutine (`reaperLoop(ctx, ...)`) handles process exit, exponential backoff auto-restart, and re-arm of the exit channel listener.
- **`errors.go`**: Sentinel errors — `ErrAlreadyRunning`, `ErrNoLoopScript`, `ErrNotRunning`.
- **`orphans.go`**: Orphan process detection and cleanup.
- **`childpids.go`** / **`childpids_linux.go`**: Platform-specific child PID enumeration (Linux reads `/proc`, other platforms use `pgrep`).
- **`logstream.go`**: Log file tailing with `fsnotify`.
- **`watcher.go`**: File system watcher for `.ralph/` directories.

## Session Sentinel Errors

The `internal/session/errors.go` file defines sentinel errors for consistent error handling across the session package:

`ErrSessionTimeout`, `ErrWorkerStalled`, `ErrInvalidProfile`, `ErrSessionNotFound`, `ErrSessionNotRunning`, `ErrSessionErrored`, `ErrSessionStopped`, `ErrTeamNotFound`, `ErrTeamNameRequired`, `ErrRepoPathRequired`, `ErrNoTasks`, `ErrAlreadyOnProvider`, `ErrWaitTimeout`, `ErrUnexpectedExit`.

Additionally, `internal/session/acceptance.go` defines `ErrRebaseConflict` for merge conflict detection.

## Self-Improvement Pipeline

The `internal/session/` package implements a five-component self-improvement pipeline that learns from loop execution history:

- **Reflexion** (`reflexion.go`): `ReflexionStore` classifies failed iterations and generates corrections. `ExtractReflection()` creates a `Reflection` from a failed `LoopIteration`. `RecentForTask()` retrieves relevant past reflections. `FormatForPrompt()` renders them as markdown for LLM injection — enabling the planner to avoid repeating mistakes.
- **Episodic Memory** (`episodic.go`): `EpisodicMemory` stores successful session trajectories. `RecordSuccess()` captures journal entries with positive signals. `FindSimilar()` uses Jaccard similarity (or optional cosine similarity via `SetEmbedder()`) to find relevant past successes. `FormatExamples()` renders examples for in-context learning. `Prune()` prevents unbounded growth while preserving per-task-type diversity.
- **Cascade Router** (`cascade.go`): `CascadeRouter` implements try-cheap-then-escalate provider routing. `ShouldCascade()` checks whether a task should use the cheap-first strategy. `EvaluateCheapResult()` decides whether to escalate based on session output, turn count, and an optional calibrated `DecisionModel`. `SelectTier()` integrates bandit policy for dynamic tier selection. Persists results as JSONL for offline analysis.
- **Curriculum Sorter** (`curriculum.go`): `CurriculumSorter` scores tasks by estimated difficulty (0.0-1.0) using multi-signal analysis from feedback profiles and episodic memory. `SortTasks()` orders tasks easy-first. `ShouldDecompose()` flags overly complex tasks. `DecompositionPrompt()` generates an LLM prompt to break tasks into sub-tasks.
- **Bandit-Based Provider Selection**: Integrated into `CascadeRouter` via `SetBanditHooks()` — a `selectFn` returns (provider, model) and `updateFn` records rewards (0.0-1.0). The `Manager.SetBanditHooks()` forwards to the cascade router. The `ralphglasses_bandit_status` tool exposes arm statistics.

## MCP Tool Groups

The MCP server currently exposes 222 tools: 218 grouped tools plus 4 management tools. Tool groups are deferred-loaded, and live counts are discoverable through `ralph:///catalog/server`, `ralph:///catalog/tool-groups`, `ralph:///catalog/skills`, `ralph:///runtime/health`, and `ralphglasses_server_health`.

The grouped surface spans 30 tool groups:

`core`, `session`, `loop`, `prompt`, `fleet`, `repo`, `roadmap`, `team`, `tenant`, `awesome`, `advanced`, `events`, `feedback`, `eval`, `fleet_h`, `observability`, `rdcycle`, `plugin`, `sweep`, `rc`, `autonomy`, `workflow`, `docs`, `recovery`, `promptdj`, `a2a`, `trigger`, `approval`, `context`, `prefetch`

The CLI parity additions are implemented as shared services plus MCP handlers rather than CLI-only logic:

- `ralphglasses_doctor`
- `ralphglasses_validate`
- `ralphglasses_config_schema`
- `ralphglasses_debug_bundle`
- `ralphglasses_theme_export`
- `ralphglasses_telemetry_export`
- `ralphglasses_firstboot_profile`
- `ralphglasses_budget_status`
- `ralphglasses_fleet_runtime`
- `ralphglasses_marathon`
- `ralphglasses_repo_surface_audit`
- `ralphglasses_worktree_list`

See [MCP-TOOLS.md](MCP-TOOLS.md) for the live contract summary and [CLI-PARITY.md](CLI-PARITY.md) for the command mapping.

## Provider Cost Normalization & Stderr Fallback

### Provider Event Normalizers

Each provider has a dedicated normalizer that parses streaming NDJSON into the unified `StreamEvent` struct (`internal/session/types.go`):

- **`normalizeClaudeEvent(line []byte) (StreamEvent, error)`** — JSON-unmarshals directly into `StreamEvent`, then does a secondary raw parse to extract nested cost fields
- **`normalizeGeminiEvent(line []byte) (StreamEvent, error)`** — Parses raw JSON map, probes multiple field paths (`type`/`event`/`event_type`, `content`/`message`/`text`/`delta`, etc.), calls `applyEventDefaults()`
- **`normalizeCodexEvent(line []byte) (StreamEvent, error)`** — Same map-probe pattern as Gemini, with Codex-specific paths (`item.type`, `output_text`)

All three are dispatched by `normalizeEvent(provider Provider, line []byte)` in `internal/session/providers.go`, called from `runSessionOutput()` in `internal/session/runner.go`.

### Cost Extraction Cascade

Each normalizer applies a three-tier cost extraction:

1. **Explicit cost field** — `firstNonZeroFloat(raw, "cost_usd", "usage.cost_usd", "usage.total_cost_usd")`
2. **Token-based estimation** — `estimateCostFromTokens(provider, raw)` computes `(inputTokens/1M) × rate.InputPer1M + (outputTokens/1M) × rate.OutputPer1M` using `ProviderCostRates` from `internal/session/costnorm.go`
   - Input token paths tried: `usage.input_tokens`, `usage_metadata.prompt_token_count`, `usage.prompt_tokens`
   - Output token paths tried: `usage.output_tokens`, `usage_metadata.candidates_token_count`, `usage.completion_tokens`
3. **Stderr fallback** — `ParseCostFromStderr(stderr string) float64` (defined in `providers.go:479`)

Reference rates in `internal/session/costs.go`:

```go
CostClaudeSonnetInput  = 3.00   // USD per 1M tokens
CostClaudeSonnetOutput = 15.00
CostGeminiFlashInput   = 0.30
CostGeminiFlashOutput  = 2.50
CostCodexInput         = 2.50
CostCodexOutput        = 15.00
```

### Stderr Cost Fallback (`ParseCostFromStderr`)

**Trigger condition**: `StreamEvent.CostUSD == 0` after both explicit field lookup and token-based estimation return zero. This function is available for callers (e.g. loop orchestrator, MCP handlers) when structured cost data is absent from the provider's stdout JSON stream.

**Parsing logic** (`providers.go:469-496`):

```go
var stderrCostRe = regexp.MustCompile(
    `(?i)(?:total\s+)?(?:session\s+)?cost(?:_usd)?:\s*\$?([\d]+\.[\d]+)`)
```

- Strips ANSI escape codes via `ansiRe` before matching
- Matches patterns like `Cost: $0.0023`, `Total cost: 0.0023`, `cost_usd: $1.23`, `Session cost: $0.05`
- Uses `FindAllStringSubmatch` and takes the **last** match (final/total cost)
- Validates: returns `0` if parse fails or value is negative

**Stderr collection**: `runSession()` in `runner.go` reads stderr into a `strings.Builder` via a background goroutine. After process exit, stderr is used for error reporting (`sanitizeStderr`) and output fallback (`cleanProviderOutput`). `ParseCostFromStderr` operates on this same buffer.

### Cross-Provider Cost Normalization

`NormalizeProviderCost(p Provider, rawCostUSD float64, inputTokens, outputTokens int) NormalizedCost` in `internal/session/costnorm.go` normalizes any provider's cost to Claude-equivalent rates:

- **With token counts**: recomputes cost at `claudeBaseRate` (Claude Sonnet input/output rates)
- **Without token counts**: scales raw cost by `claudeBlended / providerBlended` (50/50 input/output heuristic)
- Returns `NormalizedCost` with `RawCostUSD`, `NormalizedUSD`, `EfficiencyPct` (raw/normalized × 100)

### Data Flow: Raw Output → `Session.SpentUSD`

```
Provider CLI (stdout NDJSON)
  │
  ▼
runSessionOutput()                     # runner.go — line-by-line scanner
  │
  ▼
normalizeEvent(provider, line)         # providers.go — dispatches to per-provider normalizer
  │
  ├─ normalizeClaudeEvent(line)        # JSON unmarshal → secondary raw parse
  ├─ normalizeGeminiEvent(line)        # raw map probe → applyEventDefaults()
  └─ normalizeCodexEvent(line)         # raw map probe → applyEventDefaults()
  │
  │  Cost extracted via:
  │    1. firstNonZeroFloat(raw, "cost_usd", "usage.cost_usd", ...)
  │    2. estimateCostFromTokens(provider, raw)  [if step 1 = 0]
  │
  ▼
StreamEvent.CostUSD                    # types.go:82
  │
  ▼
runSession() event loop                # runner.go:327 — on "result" events:
  │  Claude:  s.SpentUSD = event.CostUSD   (cumulative)
  │  Others:  s.SpentUSD += event.CostUSD  (incremental)
  │  → s.CostHistory append
  │  → tracing.RecordTurnMetric()
  │  → events.CostUpdate published
  │
  ▼
Session.SpentUSD                       # types.go:47
  │
  ▼
Loop orchestrator (loop.go)            # reads SpentUSD from planner + worker sessions
  │  totalCost = planner.SpentUSD + Σ worker.SpentUSD
  │  → CostPredictor.Record(CostObservation{CostUSD: totalCost})
  │
  ▼
NormalizeProviderCost()                # costnorm.go — for cross-provider comparison
  └─ NormalizedCost{RawCostUSD, NormalizedUSD, EfficiencyPct}
```

## TUI Loop Interaction Surface

The TUI exposes four loop-related views, a table-driven key dispatch system, and a status bar health summary. This section traces data flow from the loop engine (see [Provider Architecture](#provider-architecture) for session dispatch and [Data Flow: Raw Output → Session.SpentUSD](#data-flow-raw-output--sessionspentusd) for cost propagation) through the Bubble Tea update cycle to the rendered UI.

### Loop Views

Four `ViewMode` constants control loop navigation (`internal/tui/app.go`):

| ViewMode | Purpose | Entry Key |
|----------|---------|-----------|
| `ViewLoopList` | Tabular list of all active loops | `l` |
| `ViewLoopDetail` | Single loop deep-dive (iterations, output, errors) | `Enter` from loop list |
| `ViewLoopControl` | Multi-loop control panel with expanded selection | `C` |
| `ViewLoopHealth` | Regression gates, sparklines, task distribution | `h` |

#### Loop List (`internal/tui/views/loop_list.go`)

`LoopListColumns` defines the table schema (ID, Repo, Phase, Iters, Status). The `LoopRunsToRows()` function converts `[]*session.LoopRun` into `[]components.Row`:

1. Locks each `LoopRun.mu` for thread-safe field access
2. Extracts `ID` (truncated to 8 chars), `RepoName`, phase (from last iteration status), `IterCount`, and status
3. Renders status with `components.ActivityDot()` and `styles.StatusIcon()`
4. Overrides status to `"paused"` when `l.Paused && status == "running"`

The table is created by `NewLoopListTable()` with empty message `"No active loops"` and `StatusColumn=4`.

#### Loop Control Panel (`internal/tui/views/loop_control.go`)

`LoopControlData` aggregates per-loop metrics: `ID`, `RepoName`, `Status`, `Paused`, `IterCount`, `LastIterStatus`, `LastIterTask`, `LastIterError`, `AvgIterDuration`, and a computed `NextEstimate` (e.g. `"paused"`, `"imminent"`, `"~30s"`). `RenderLoopControlPanel()` renders a title bar with running/paused/other counts, loop rows with next-iteration estimates, and expanded details for the selected loop.

#### Loop Detail (`internal/tui/views/loopdetail.go`)

`RenderLoopDetail()` shows a single `LoopRun`: header (ID, repo, status), iteration metrics, last iteration details (task, error), and truncated planner output / worker result (500 chars each).

#### Loop Health (`internal/tui/views/loophealth.go`)

`LoopHealthData` holds `Observations []session.LoopObservation`, a `*e2e.GateReport`, `*e2e.Summary`, and `*e2e.LoopBaseline`. Rendered sections: gate summary with sparklines and per-metric verdicts (pass/warn/fail with delta% and baseline), recent iterations table (last 15), and task type distribution bar chart.

### Keybindings and Dispatch

Key dispatch uses a **table-driven pattern** in `internal/tui/handlers.go`:

```go
type ViewKeyEntry struct {
    Binding func(km *KeyMap) key.Binding  // key matcher (optional)
    Match   func(msg tea.KeyMsg) bool     // custom matcher (optional)
    Handler KeyHandler                     // func(m *Model, msg tea.KeyMsg) (tea.Model, tea.Cmd)
}
```

`dispatchViewKeys()` iterates entries in order; **first match wins**. Each view has its own entry slice (`loopListKeys`, `loopControlKeys`, `loopDetailKeys`).

#### Loop Key Bindings

**Overview / Repo Detail** (global bindings in `handlers.go`):

| Key | KeyMap Field | Handler | Effect |
|-----|-------------|---------|--------|
| `S` | `StartLoop` | `m.startSelectedLoop()` / `m.startLoop(idx)` | Launch loop for selected repo |
| `X` | `StopAction` | `m.stopSelectedLoop()` / `m.stopLoop(idx)` | Stop selected loop |
| `P` | `PauseLoop` | `m.togglePauseSelected()` / `m.togglePause(idx)` | Toggle pause/resume |

**Loop List** (`loopListKeys`):

| Key | Action |
|-----|--------|
| `j`/`k` or `↓`/`↑` | `m.LoopListTable.MoveDown()`/`MoveUp()` |
| `Enter` | Push `ViewLoopDetail` |

**Loop Control / Detail** (`loopControlKeys`, `loopDetailKeys`):

| Key | KeyMap Field | Command Emitted | Message Type |
|-----|-------------|-----------------|-------------|
| `s` | `LoopCtrlStep` / `LoopDetailStep` | `sessMgr.StepLoop()` | `LoopStepResultMsg` |
| `r` | `LoopCtrlToggle` / `LoopDetailToggle` | `sessMgr.StartLoop()` or `StopLoop()` | `LoopToggleResultMsg` |
| `p` | `LoopCtrlPause` / `LoopDetailPause` | `sessMgr.PauseLoop()` or `ResumeLoop()` | `LoopPauseResultMsg` |

### Message Types

Loop operations produce typed messages handled in `Update()` (`internal/tui/app.go`):

- **`LoopListMsg`** (`[]*session.LoopRun`) — refreshes loop table rows
- **`LoopStepResultMsg`** (`LoopID`, `Err`) — shows notification, re-fetches loop list
- **`LoopToggleResultMsg`** (`LoopID`, `Started`, `Err`) — shows "Started"/"Stopped" notification, re-fetches
- **`LoopPauseResultMsg`** (`LoopID`, `Paused`, `Err`) — shows "Paused"/"Resumed" notification, re-fetches

### `ProcessExitMsg` Flow

`ProcessExitMsg` bridges the process manager to Bubble Tea's message loop, triggering repo status updates when a managed process terminates.

**Definition** (`internal/process/manager.go`):

```go
type ProcessExitMsg struct {
    RepoPath string
    ExitCode int
    Error    error
}
```

**Data flow:**

```
process.Manager reaper goroutine          # manager.go — Start()
  │  waitErr := cmd.Wait()
  │
  ▼
m.exitCh <- ProcessExitMsg{…}            # buffered channel (size 16), non-blocking send
  │
  ▼
WaitForProcessExit(ch) tea.Cmd           # manager.go — blocks on <-ch, returns ProcessExitMsg
  │
  ▼
Model.Update() case process.ProcessExitMsg   # app.go — main update switch
  │
  ├─ applyProcessExit(msg, m.Repos)      # finds repo by msg.RepoPath,
  │    └─ r.Status.Status = model.RepoStatusFromExitCode(msg.ExitCode, msg.Error)
  │
  └─ return m, process.WaitForProcessExit(m.ProcMgr.ExitChan())
       └─ re-arms listener for next exit
```

`WaitForProcessExit()` is a `tea.Cmd` factory that blocks on `Manager.ExitChan()`. The `Update()` handler calls `applyProcessExit()` to update the matching repo's status via `model.RepoStatusFromExitCode()`, then immediately re-arms the listener by returning a new `WaitForProcessExit` command.

### Status Bar Loop Health

The `StatusBar` component (`internal/tui/components/statusbar.go`) surfaces aggregated loop metrics:

```go
type StatusBar struct {
    RunningCount         int              // active loop/process count
    SessionCount         int              // total sessions
    TotalSpendUSD        float64          // cumulative cost
    ProviderCounts       map[string]int   // running sessions per provider
    FleetBudgetPct       float64          // spend / total budget
    AlertCount           int              // active alerts
    HighestAlertSeverity string           // worst alert level
    // … Mode, Filter, Width, SpinnerFrame, TickFrame, etc.
}
```

**Update trigger**: `m.updateTable()` is called on every `tickMsg` (2-second polling interval, `internal/tui/app.go`). It populates:

- `RunningCount` ← `len(m.ProcMgr.RunningPaths())`
- `SessionCount` ← total active sessions from the session manager
- `TotalSpendUSD` ← sum of `Session.SpentUSD` across all sessions (see [Data Flow: Raw Output → Session.SpentUSD](#data-flow-raw-output--sessionspentusd))
- `ProviderCounts` ← per-provider breakdown of running sessions
- `FleetBudgetPct` ← `totalSpend / totalBudget`
- `AlertCount` / `HighestAlertSeverity` ← `m.countAlerts()`

`StatusBar.View()` renders left-aligned: mode badge, repo count, running count (with animated spinner from `TickFrame`), session count, spend, provider counts, fleet budget gauge, and alert indicator. Right-aligned: last refresh timestamp (relative, e.g. `"5s"`, `"2m"`, `"never"`).

## File Schemas

- `.ralph/status.json`: LoopStatus (timestamp, loop_count, calls_made_this_hour, status, model, etc.)
- `.ralph/.circuit_breaker_state`: CircuitBreakerState (state: CLOSED/HALF_OPEN/OPEN, counters, reason)
- `.ralph/progress.json`: Progress (iteration, completed_ids, log entries, status)
- `.ralphrc`: Shell-style KEY="value" config (PROJECT_NAME, MAX_CALLS_PER_HOUR, CB thresholds, etc.)
- `.ralph/improvement_journal.jsonl`: Append-only JSONL, one entry per session (worked/failed/suggest)
- `.ralph/improvement_patterns.json`: Consolidated durable patterns from journal (survives pruning)
- `.ralph/decisions.jsonl`: Autonomous decision log (level, category, rationale, inputs, outcome)
- `.ralph/hitl_events.jsonl`: Human-in-the-loop events (metric type, trigger, session/repo)
- `.ralph/context_store.json`: Active session context entries for file conflict detection

## Developer References

- **Claude Code**: [Overview](https://docs.anthropic.com/en/docs/claude-code/overview) | [CLI Reference](https://docs.anthropic.com/en/docs/claude-code/cli-reference) | [SDK](https://docs.anthropic.com/en/docs/claude-code/sdk)
- **Anthropic API**: [API Reference](https://docs.anthropic.com/en/api) | [Tool Use](https://docs.anthropic.com/en/docs/build-with-claude/tool-use)
- **Gemini**: [API Overview](https://ai.google.dev/gemini-api/docs) | [Gemini CLI](https://github.com/google-gemini/gemini-cli)
- **OpenAI**: [API Reference](https://platform.openai.com/docs/api-reference) | [Codex CLI](https://github.com/openai/codex)
- **MCP**: [Specification](https://modelcontextprotocol.io/) | [Go SDK (mcp-go)](https://github.com/mark3labs/mcp-go). Migration target: `modelcontextprotocol/go-sdk` v1.4.1 (official MCP Go SDK, supersedes `mark3labs/mcp-go`)
