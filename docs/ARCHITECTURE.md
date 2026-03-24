# Architecture

## Package Layout

- **main.go** → **cmd/root.go**: Cobra CLI with `--scan-path` flag
- **internal/discovery/**: Scans directories for `.ralph/` and `.ralphrc`
- **internal/model/**: Data types and parsers for status.json, progress.json, circuit breaker state, .ralphrc
- **internal/process/**: Process management (launch/stop/pause via os/exec), fsnotify file watcher, log tailing
- **internal/session/**: Multi-provider LLM session management (claude/gemini/codex), agent teams, budget enforcement, provider dispatch, concurrent worker fan-out, autonomy levels, auto-optimization, auto-recovery, context store, HITL metrics, feedback profiling, prompt caching
- **internal/mcpserver/**: MCP tool handlers (84 tools, stdio transport via mcp-go)
  - `tools.go` — Server struct, constructors, Register(), all handler implementations, helpers
  - `handler_prompt.go` — Multi-provider prompt enhancement handlers
  - `handler_fleet.go` — Distributed fleet, HITL, autonomy, and feedback profile handlers
  - `middleware.go` — Composable middleware: InstrumentationMiddleware, EventBusMiddleware, ValidationMiddleware
  - `toolbench.go` — Auto-benchmarking with JSONL logging, P50/P95 latencies, regression detection
- **internal/enhancer/**: Prompt enhancement pipeline (13-stage), scoring, lint, multi-provider LLM improvement
- **internal/roadmap/**: Roadmap parsing, analysis, research, expansion, export
- **internal/repofiles/**: Ralph config file scaffolding and optimization
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

The MCP server supports composable middleware (`internal/mcpserver/middleware.go`):

- **InstrumentationMiddleware**: Records timing, success/fail, input/output size for every tool call
- **EventBusMiddleware**: Emits `tool.call` events to the fleet event bus for real-time monitoring
- **ValidationMiddleware**: Pre-validates required parameters (repo path, session ID) before handler execution

### Tool Benchmarking

`internal/mcpserver/toolbench.go` provides auto-benchmarking applied to all 84 tools:

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
- **MCP**: [Specification](https://modelcontextprotocol.io/) | [Go SDK (mcp-go)](https://github.com/mark3labs/mcp-go)
