# Architecture

## Package Layout

- **main.go** → **cmd/root.go**: Cobra CLI with `--scan-path` flag
- **internal/discovery/**: Scans directories for `.ralph/` and `.ralphrc`
- **internal/model/**: Data types and parsers for status.json, progress.json, circuit breaker state, .ralphrc
- **internal/process/**: Process management (launch/stop/pause via os/exec), fsnotify file watcher, log tailing
- **internal/session/**: Multi-provider LLM session management (claude/gemini/codex), agent teams, budget enforcement, provider dispatch, concurrent worker fan-out, autonomy levels, auto-optimization, auto-recovery, context store, HITL metrics, feedback profiling, prompt caching
- **internal/batch/**: Batch API support for Claude, Gemini, and OpenAI — submit non-interactive workloads at 50% discount
- **internal/wsclient/**: WebSocket transport client for OpenAI Responses API (40% faster for multi-turn tool chains)
- **internal/mcpserver/**: MCP tool handlers (86 tools in 10 namespaces, deferred loading, stdio transport via mcp-go)
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

`internal/mcpserver/toolbench.go` provides auto-benchmarking applied to all 86 tools:

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

## Provider Event Normalization & Cost Extraction

The `internal/session/` package normalizes heterogeneous provider output into a single `StreamEvent` type and extracts cost through a three-tier cascade.

### Event Normalizer Dispatch

`normalizeEvent()` in `providers.go` is the entry point called by `runner.go`'s stream reader on each output line:

```
normalizeEvent(provider, line []byte) → StreamEvent
  ├─ ProviderGemini  → normalizeGeminiEvent(line)
  ├─ ProviderCodex   → normalizeCodexEvent(line)
  └─ default/Claude  → normalizeClaudeEvent(line)
```

Each normalizer unmarshals the raw JSON into a `map[string]any`, extracts fields using dotted path helpers (`valueAtPath`, `firstNonZeroFloat`, etc.), and returns a unified `StreamEvent`. All three set `event.Raw` to the original bytes for downstream consumers.

| Normalizer | Provider format | Notable behavior |
|---|---|---|
| `normalizeClaudeEvent` | Claude stream-json | Double-unmarshal: flat struct + raw map for nested fields; normalises `subagent` → `agent` type |
| `normalizeGeminiEvent` | Gemini NDJSON | Path-based extraction; falls back to `fallbackTextEvent` on parse error |
| `normalizeCodexEvent` | Codex quiet-mode JSON | Path-based extraction; similar fallback path |

`applyEventDefaults()` is called by the Gemini and Codex normalizers to canonicalise type names (`message`/`delta`/`output` → `assistant`, `error` → `result`) and fill derived fields (`Text`, `Content`, `Result`).

### Three-Tier Cost Extraction Cascade

Within each normalizer, cost is extracted in priority order. The first tier to produce a non-zero value wins:

**Tier 1 — Explicit cost field** (`firstNonZeroFloat`):
```
cost_usd  →  usage.cost_usd  →  usage.total_cost_usd
```
Resolves provider quirks where the cost field is top-level for some events and nested under `usage` for others.

**Tier 2 — Token estimation** (`estimateCostFromTokens`):
Activated only when Tier 1 returns zero. Walks multiple provider-specific token paths:
- Input: `usage.input_tokens` → `usage_metadata.prompt_token_count` → `usage.prompt_tokens`
- Output: `usage.output_tokens` → `usage_metadata.candidates_token_count` → `usage.completion_tokens`

Cost is then computed using `ProviderCostRates` (USD per 1M tokens):
```
cost = (inputTokens / 1_000_000) × rates.InputPer1M
     + (outputTokens / 1_000_000) × rates.OutputPer1M
```

**Tier 3 — Stderr fallback** (`ParseCostFromStderr`):
An exported utility for callers that collect stderr separately (e.g. MCP tool handlers). Not called inline by the normalizers, but available as a last resort when structured stdout data is absent.

### Stderr Cost Fallback (`ParseCostFromStderr`)

`ParseCostFromStderr` parses human-readable cost lines emitted by LLM CLIs to their terminal:

```go
var stderrCostRe = regexp.MustCompile(
    `(?i)(?:total\s+)?(?:session\s+)?cost(?:_usd)?:\s*\$?([\d]+\.[\d]+)`)
```

Matched patterns (case-insensitive):
- `Cost: $0.0023`
- `Total cost: 0.0023`
- `cost_usd: $1.23`
- `Session cost: $0.05`

Processing steps:
1. **ANSI strip** — removes terminal colour codes via `\x1b\[[0-9;]*[a-zA-Z]` before matching
2. **Find all matches** — collects every match in the buffer
3. **Last-match selection** — `matches[len(matches)-1]` picks the final printed cost, which is the cumulative total (CLIs often print intermediate costs as well)
4. **Validation** — returns `0` if parse fails or `cost < 0`; negative costs are treated as absent

### Cross-Provider Normalization (`NormalizeProviderCost`)

`NormalizeProviderCost` in `costnorm.go` scales a raw provider cost to the Claude Sonnet baseline, enabling apples-to-apples comparisons in the fleet optimizer and auto-optimizer:

```go
func NormalizeProviderCost(p Provider, rawCostUSD float64,
    inputTokens, outputTokens int) NormalizedCost
```

Two normalization paths:

| Token counts known | Method |
|---|---|
| Yes | `NormalizedUSD = (inputTokens/1M × claudeRate.Input) + (outputTokens/1M × claudeRate.Output)` |
| No | Blended-rate scaling: `NormalizedUSD = rawCostUSD × (claudeBlended / providerBlended)` where blended = (InputPer1M + OutputPer1M) / 2 |

`EfficiencyPct = (RawCostUSD / NormalizedUSD) × 100`. Values below 100 indicate the provider is cheaper than Claude at equivalent work.

`NormalizeSessionCost(s *Session)` is a convenience wrapper that reads `s.SpentUSD` under the session mutex and delegates to `NormalizeProviderCost` with zero token counts.

Consumers: `internal/session/autooptimize.go` (provider scoring) and `internal/fleet/optimizer.go` (fleet task routing).

### Data Flow Diagram

```
Provider CLI stdout (NDJSON stream)
         │
         ▼
  normalizeEvent(provider, line)
         │
         ├─── normalizeClaudeEvent ──┐
         ├─── normalizeGeminiEvent ──┤
         └─── normalizeCodexEvent ───┘
                                     │
                     Three-tier cost cascade:
                     1. Explicit cost_usd field
                     2. Token estimation (ProviderCostRates)
                     3. ParseCostFromStderr (caller-driven fallback)
                                     │
                                     ▼
                            StreamEvent.CostUSD
                                     │
                    runner.go accumulates into Session:
                    ┌─ Claude:  SpentUSD  = event.CostUSD  (cumulative)
                    └─ Others:  SpentUSD += event.CostUSD  (additive)
                                     │
                                     ▼
                            Session.SpentUSD
                                     │
              ┌──────────────────────┴──────────────────────┐
              │                                             │
    CostUpdate event bus                     NormalizeSessionCost()
    (fleet monitoring,                                      │
     budget enforcement)               NormalizeProviderCost(provider,
                                         SpentUSD, 0, 0)
                                                            │
                                                            ▼
                                                    NormalizedCost
                                                  (NormalizedUSD,
                                                   EfficiencyPct)
                                                            │
                                          ┌─────────────────┴──────────────┐
                                          │                                │
                                 autooptimize.go                  fleet/optimizer.go
                                 (provider scoring)               (task routing)
```

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
