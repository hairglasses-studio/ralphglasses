# Phase 13 Research: Decoupling & Cross-Project Learning

## 1. Executive Summary

- **ralphglasses re-implements capabilities that already exist in mcpkit** (ralph loop engine, finops, workflow engine, session management, resilience) — the two codebases share zero Go imports despite mcpkit being explicitly listed as the upstream framework in ROADMAP.md section "Internal Ecosystem Integration."
- **The `internal/session/` package (7,271 lines) is a monolith** combining loop orchestration, budget enforcement, provider dispatch, workflow execution, failover, cost normalization, journaling, agents, templates, health checks, and rate limiting — all in one package with a single `Manager` god-struct that holds six maps and a mutex.
- **Sister repos (hg-mcp, claudekit) have already solved the decoupling problem** via a shim pattern: thin adapter layers (`internal/mcp/tools/helpers.go`, `compat.go`, `registry.go`) that delegate to mcpkit without exposing it to business-logic modules. ralphglasses should adopt this pattern for its Phase 6.1 native loop engine work.
- **Budget/cost tracking is duplicated three ways**: ralphglasses `budget.go` + `costnorm.go` (131+81 lines), mcpkit `finops/` (17 files, production-grade with scoped budgets, time windows, cost estimation), and the shell-based marathon.sh budget check. Consolidating onto mcpkit's finops would eliminate ~200 lines and gain token-level tracking.
- **The event bus (`internal/events/bus.go`, 192 lines) is a minimal reimplementation** of patterns available in mcpkit's registry middleware and observer infrastructure. Wiring it into mcpkit's observability layer would enable OpenTelemetry spans, Prometheus metrics, and audit logging for free.

## 2. Current State Analysis

### 2.1 What Exists

| File | Lines | Tests | Coverage | Status |
|------|-------|-------|----------|--------|
| `internal/session/manager.go` | 846 | 690 | ~70% | Monolith: sessions + teams + workflows + loops |
| `internal/session/loop.go` | 870 | 227 | ~65% | Planner/worker/verifier loop, worktree mgmt |
| `internal/session/providers.go` | 682 | 390 | ~80% | Multi-provider dispatch, event normalization |
| `internal/session/workflow.go` | 265 | 193 | ~75% | YAML workflow DAG execution |
| `internal/session/budget.go` | 131 | 95 | ~70% | Cost ledger, budget enforcement |
| `internal/session/costnorm.go` | 81 | 63 | ~80% | Cross-provider cost normalization |
| `internal/session/journal.go` | 401 | 305 | ~75% | Improvement journal JSONL persistence |
| `internal/session/agents.go` | 321 | 232 | ~70% | Agent definition discovery per provider |
| `internal/session/runner.go` | 380 | 221 | ~65% | Session lifecycle, stream parsing |
| `internal/session/types.go` | 147 | - | N/A | Type definitions only |
| `internal/session/failover.go` | 50 | - | ~90% | Provider failover chain |
| `internal/session/health.go` | 100 | 64 | ~70% | Provider health checking |
| `internal/session/ratelimit.go` | 99 | 80 | ~80% | Token bucket rate limiter |
| `internal/session/costnorm.go` | 81 | 63 | ~80% | Provider cost normalization |
| `internal/session/templates.go` | 62 | 61 | ~90% | Session prompt templates |
| `internal/session/checkpoint.go` | 50 | - | 0% | Git checkpoint tagging |
| `internal/session/gitinfo.go` | 128 | - | 0% | Git metadata extraction |
| `internal/session/question.go` | 37 | - | 0% | Worker question detection |
| `internal/events/bus.go` | 192 | 219 | ~85% | In-process pub/sub with ring buffer |
| `internal/enhancer/enhancer.go` | 836 | 463 | ~70% | 13-stage prompt enhancement pipeline |
| `internal/mcpserver/tools.go` | 3,992 | 2,263 | ~65% | 57 MCP tool handlers in one file |
| `go.mod` | 48 | - | N/A | Zero mcpkit dependency |

**Total internal/session/**: 7,271 lines (4,650 non-test) across 18 files. **Total internal/enhancer/**: 7,363 lines (3,711 non-test) across 36 files.

### 2.2 What Works Well

1. **Provider dispatch pattern** (`providers.go:131-159`): Clean `buildCmdForProvider()` switch with per-provider builders and `normalizeEvent()` normalizers. Adding a new provider is a documented 7-step process (CLAUDE.md lines 268-276). This is well-isolated and testable.

2. **Loop planner prompt construction** (`loop.go:393-452`): Integrates roadmap analysis, journal context, issue ledger, and previous iteration feedback into the planner prompt. Context-aware and well-structured with `buildLoopPlannerPrompt()`.

3. **Prompt enhancement integration** (`loop.go:243-245, 283-285`): The `Enhancer` field on Manager and `enhanceForProvider()` method cleanly inject prompt improvement into StepLoop without coupling the loop logic to the enhancement pipeline.

4. **Event bus cursor-based polling** (`events/bus.go:149-178`): `HistoryAfterCursor()` provides efficient incremental updates for mobile RC tools. Ring buffer with monotonic counter is simple and correct.

5. **Provider failover** (`failover.go:22-50`): Clean health-check-first failover with structured error collection. Small, focused, testable.

6. **Session persistence** (`manager.go:719-846`): JSON-file-based persistence with cross-process discovery (`LoadExternalSessions()`) enables TUI and MCP server to share state. 24-hour TTL cleanup is pragmatic.

### 2.3 What Doesn't Work

1. **Manager god-struct** — `Manager` in `manager.go` holds `sessions`, `teams`, `workflowRuns`, `loops` maps plus `bus`, `stateDir`, `Enhancer`, and two hook functions. It handles session lifecycle, team coordination, workflow execution, loop management, and persistence — violating single responsibility. Cross-refs: ROADMAP 2.1 (session data model), 6.1 (native loop engine).

2. **No mcpkit dependency** — Despite ROADMAP explicitly listing 7 mcpkit packages to port/embed (lines 998-1006), `go.mod` has zero mcpkit imports. The loop engine (`loop.go`, 870 lines), workflow engine (`workflow.go`, 265 lines), budget tracking (`budget.go`, 131 lines), and resilience patterns (circuit breaker references in model/) are all hand-rolled. Cross-refs: ROADMAP 6.1 (embed mcpkit/ralph), 2.3.5 (port from mcpkit/finops).

3. **tools.go monolith** — `internal/mcpserver/tools.go` is 3,992 lines containing all 57 tool handlers, the Server struct, constructors, Register(), and dozens of helpers. hg-mcp's modular pattern (one package per domain, `init()` auto-registration) handles 1,190+ tools cleanly. Cross-refs: ROADMAP 2.8 (MCP server expansion).

4. **Cost tracking duplication** — Three separate cost systems: `budget.go` (BudgetEnforcer, LedgerEntry, CostSummary), `costnorm.go` (NormalizeProviderCost with hardcoded rates), and marathon.sh's shell-based budget check reading `status.json`. mcpkit's `finops/` has token-level tracking, dollar-cost estimation, scoped budgets, time-windowed tracking, and Prometheus export. Cross-refs: ROADMAP 2.3.5, 5.5.


6. **Workflow engine reimplementation** — `workflow.go` (265 lines) implements DAG validation, topological execution, and parallel groups. mcpkit's `workflow/` package provides cyclical graph engine with conditional branching, checkpoints, state machines, node middleware, fork nodes, and compensation/saga rollback. Cross-refs: ROADMAP 8.3.

7. **No observability** — No OpenTelemetry traces, no Prometheus metrics, no structured audit logging. mcpkit's `observability/` provides all three as middleware. The event bus (`events/bus.go`) is a custom lightweight solution with no integration to standard observability infrastructure. Cross-refs: ROADMAP 6.4.

## 3. Gap Analysis

### 3.1 ROADMAP Target vs Current State

| ROADMAP Item | Target | Current State | Gap |
|---|---|---|---|
| 6.1 — Native ralph loop engine | Embed mcpkit/ralph as Go dependency with DAG, specs, progress | Custom loop.go (870 lines) with ad-hoc planner/worker/verifier | Full rewrite needed: mcpkit/ralph has DAG enforcement, YAML specs, templates, checkpointing, cost governor, stuck detection, observability hooks |
| 6.2 — R&D cycle orchestrator | Port claudekit rdcycle perpetual loop | No rdcycle concept exists | claudekit's rdcycle has scan/plan/verify/commit/report/schedule/notes/improve tools + budget profiles + model tiers |
| 2.3.5 — Port budget from mcpkit/finops | Token-level cost tracking, scoped budgets | File-based JSONL ledger with simple headroom check | mcpkit finops has 17 files: Tracker, CostPolicy, ScopedBudget, WindowedTracker, dollar-cost estimation |
| 6.4 — Analytics & observability | OpenTelemetry traces, Prometheus /metrics | Custom events.Bus (192 lines), no OTel/Prometheus | mcpkit/observability ready to use |
| 2.1.2 — SQLite session store | modernc.org/sqlite with WAL mode | JSON file persistence (~50 lines in manager.go) | internal-sqlite-project pattern available for reference |
| 6.9 — Natural language fleet control | mcpkit/sampling for NL commands | No sampling integration | mcpkit/sampling has request builders, middleware, context injection |
| 8.3 — Workflow engine | YAML workflows with conditionals, error handlers | Basic DAG executor (265 lines) | mcpkit/workflow has state machines, checkpoints, fork nodes, compensation |
| 5.3 — MCP gateway | Central hub with per-session auth, audit | Direct MCP server (tools.go) | mcpkit/gateway has dynamic upstream registration, per-upstream resilience |

### 3.2 Missing Capabilities

1. **No shared type library** — ralphglasses, mcpkit, hg-mcp, claudekit, and mesmer all define their own session/task/budget types. A shared `types` package (or Go interface contracts) would enable cross-project interop without tight coupling.

2. **No interface-based decoupling** — The Manager struct directly owns all behavior. No interfaces for `SessionStore`, `BudgetTracker`, `LoopEngine`, `WorkflowRunner`. This makes it impossible to swap implementations (e.g., switch from file-based to SQLite persistence without rewriting Manager).

3. **No plugin/middleware architecture** — Unlike mcpkit's `registry.Middleware` pattern and hg-mcp's `ToolModule` interface, ralphglasses has no extension points. Adding a new tool requires editing the 3,992-line `tools.go` file.

4. **No cross-repo session discovery** — Each repo in the org manages its own ralph loops independently. There is no mechanism for ralphglasses to discover or manage sessions running in other repos via their respective MCP servers.

5. **No model tier awareness** — mcpkit's `rdcycle/models.go` provides `ModelTierConfig` with task-phase-aware model selection. ralphglasses hardcodes default models per provider in `ProviderDefaults()`.

### 3.3 Technical Debt Inventory

| Debt Item | Location | Impact | Effort to Fix |
|---|---|---|---|
| Manager god-struct | `session/manager.go` | High — blocks testing, extension | L — extract interfaces, split into sub-managers |
| tools.go monolith | `mcpserver/tools.go` (3,992 lines) | High — merge conflicts, cognitive load | XL — migrate to module pattern |
| Hardcoded cost rates | `session/costnorm.go:12-16` | Medium — stale pricing | S — load from config or mcpkit/finops |
| JSON file persistence | `session/manager.go:719-846` | Medium — no queries, no transactions | L — migrate to SQLite |
| No context propagation | Multiple files (ROADMAP 1.9) | Medium — can't cancel long ops | M — thread ctx through all paths |
| Duplicated budget logic | `budget.go` + marathon.sh | Low — inconsistent enforcement | S — consolidate |
| No structured logging | `mcpserver/`, `process/` | Low — hard to debug production | M — migrate to slog |

## 4. External Landscape

### 4.1 Competitor/Peer Projects (Sister Repos)

#### mcpkit — Go MCP Framework (35+ packages, 100% MCP spec coverage)

**Architecture pattern**: Layered dependency graph with 4 layers. Lower layers never import upper layers. Every package has `doc.go`, `_test.go`, and example functions. 90%+ coverage across all packages.

**Key packages ralphglasses should consume**:
- `ralph/` (36 files): Autonomous loop runner with DAG enforcement, YAML specs, templates, checkpoints, cost governor, stuck detection, observability hooks, and workflow integration. This directly replaces `internal/session/loop.go`.
- `finops/` (17 files): Token accounting, CostPolicy, ScopedBudget, WindowedTracker, dollar-cost estimation with model pricing, Prometheus export. Replaces `budget.go` + `costnorm.go`.
- `workflow/` (14 files): Cyclical graph engine with conditional branching, checkpoints, state machines, fork nodes, compensation/saga rollback. Replaces `workflow.go`.
- `session/` (8 files): Session/SessionStore interfaces, in-memory store, middleware, TTL/eviction. Provides the interface contract Manager needs.
- `resilience/` (8 files): CircuitBreaker, RateLimiter, CacheEntry generics. Replaces `ratelimit.go`.
- `observability/` (6 files): OpenTelemetry tracing/metrics middleware. Fills the analytics gap.
- `sampling/` (14 files): LLM sampling client with request builders. Needed for Phase 6.9 NL fleet control.

**Integration model**: `go.mod` `require` + `replace ../mcpkit` (same as claudekit and hg-mcp).

#### hg-mcp — Go MCP Server (1,190+ tools, 119 modules)

**Architecture pattern**: Thin shim layer (`internal/mcp/tools/helpers.go`, `compat.go`, `registry.go`) delegates to mcpkit. All 119 tool modules import from the shim, not mcpkit directly. This means mcpkit upgrades require changing only 3 files, not 119.

**Key patterns to adopt**:
- **Module auto-registration via `init()`**: Each tool domain is a separate package with `init()` that calls `tools.GetRegistry().RegisterModule(&Module{})`. No central registration file to maintain.
- **Runtime groups**: Tools auto-assigned to groups based on category. `ListToolsByRuntimeGroup()` and `GetRuntimeGroupStats()` for programmatic access.
- **Rate limit per-service defaults**: Each module declares its own rate limit via `CircuitBreakerGroup` field.
- **Shim insulation**: `helpers.go` wraps `TextResult`, `ErrorResult`, `JSONResult`, param getters. `compat.go` re-exports MCP SDK types. Business logic modules never see mcpkit.

**What ralphglasses would gain**: Decomposing the 3,992-line `tools.go` into ~15 module packages (fleet, session, loop, team, workflow, roadmap, agent, journal, event, awesome, prompt, claudemd, rc, marathon, snapshot) with auto-registration.

#### claudekit — Go MCP with rdcycle + budget profiles

**Architecture pattern**: Direct mcpkit dependency via `replace ../mcpkit`. 10 modules (fontkit, themekit, envkit, statusline, pluginkit, skillkit, mcpserver, rdcycle, cmd).

**Key patterns to adopt**:
- **rdcycle perpetual loop** (`rdcycle/`): scan -> plan -> verify -> commit -> report -> schedule -> notes -> improve cycle. `BudgetProfile` presets (Personal/WorkAPI). `ModelTierConfig` with task-phase-aware model selection. Self-improvement via `rdcycle_improve` that analyzes patterns and suggests budget adjustments.
- **PluginKit** (`pluginkit/`): YAML plugin loading, subprocess handler, ToolModule bridge. Provides the extensibility ralphglasses Phase 3.5.2 needs.
- **SkillKit** (`skillkit/`): Claude Code skill marketplace — discovery, install, remove. Maps to Phase 3.5.4 MCP skill export.

#### internal-sqlite-project — Go + SQLite + MCP + Audit Logs

**Key patterns to adopt**:
- **Pure-Go SQLite** via `modernc.org/sqlite`: No CGO, cross-compiles to all platforms. WAL mode for concurrent reads. This is the pattern ROADMAP 2.1.2 should follow.
- **Audit log pattern**: Append-only audit trail with structured events. Maps to Phase 6.7 replay/audit trail.

#### mesmer — Go MCP Server with Ralph Integration

**Key patterns**: Uses `mcp-go` directly (not mcpkit). Has its own `internal/` structure with research, SLO, prometheus, workflows directories. Demonstrates the pattern of a large Go MCP server with infrastructure tooling.

### 4.2 Patterns Worth Adopting (with specific patterns from sister repos)

1. **Shim Layer Pattern (from hg-mcp)**: Create `internal/mcp/compat.go` and `internal/mcp/helpers.go` that re-export mcpkit types and handler helpers. All tool modules import from the shim. mcpkit upgrades require changing 2-3 files, not touching business logic.

2. **Module Auto-Registration (from hg-mcp)**: Each tool domain in its own package with `init()` registration. Eliminates the 3,992-line `tools.go` monolith. Pattern: `internal/mcpserver/tools/<domain>/module.go` with `Module` interface (`Name()`, `Description()`, `Tools()`).

3. **Interface-Based Manager Decomposition (from mcpkit/session)**: Define `SessionStore`, `LoopRunner`, `WorkflowEngine`, `BudgetTracker` interfaces. Manager becomes a coordinator that holds interface implementations, not concrete logic. Implementations can be swapped (file-based -> SQLite -> mcpkit backends).

4. **Budget Profiles (from claudekit/rdcycle)**: Named budget presets (Personal: $5/cycle, WorkAPI: $50/cycle) with model tier selection per task phase. Replaces the single `BudgetEnforcer.Headroom` field.

5. **Local Replace Directive (from claudekit/hg-mcp)**: `replace github.com/hairglasses-studio/mcpkit => ../mcpkit` in `go.mod`. Zero-cost for local development, enables real dependency once mcpkit gets a release tag.

6. **Middleware Chain (from mcpkit/registry)**: `func(name string, td ToolDefinition, next ToolHandlerFunc) ToolHandlerFunc` signature. Enables composable observability, rate limiting, circuit breaking, audit logging on every tool invocation.

### 4.3 Anti-Patterns to Avoid

1. **Full mcpkit coupling on day one**: Do NOT import mcpkit into every internal package. Use the shim pattern (hg-mcp's approach) so business logic is insulated from framework churn. Import mcpkit only in the shim and adapter layers.

2. **Big-bang migration**: Do NOT rewrite loop.go, workflow.go, and budget.go simultaneously. Instead, extract interfaces first, then swap implementations one at a time behind the interface boundary. The shim pattern enables this incremental approach.

3. **Ignoring the `official_sdk` build tag**: mcpkit supports both `mcp-go` and the official `modelcontextprotocol/go-sdk` via build tags (`//go:build !official_sdk`). ralphglasses currently uses only `mcp-go`. Any mcpkit integration must maintain compatibility with the `mcp-go` path.

4. **Shared SQLite across network**: ROADMAP 3.3.1 suggests sharing SQLite across instances via WAL mode. This works for local multi-process but breaks over NFS/network filesystems. Use SQLite per-instance with a sync layer if cross-machine coordination is needed.

5. **Premature abstraction of provider dispatch**: The current `buildCmdForProvider()` pattern is clean and direct. Do NOT over-abstract it into a plugin system before there is a real fourth provider to support.

### 4.4 Academic & Industry References

1. **Conway's Law and Module Boundaries**: The sister repo structure (mcpkit = framework, hg-mcp = tools, claudekit = CLI experience, ralphglasses = fleet management) reflects Conway's Law. Decoupling should respect these organizational boundaries by importing, not forking, shared code.

2. **Hexagonal Architecture (Ports and Adapters)**: The proposed interface extraction (`SessionStore`, `LoopRunner`, etc.) follows the ports-and-adapters pattern. Core business logic (loop orchestration, budget decisions) defines ports; adapters (file-based, SQLite, mcpkit backends) implement them.

3. **Strangler Fig Pattern**: Rather than rewriting the monolithic Manager, wrap it behind interfaces and gradually replace internal implementations with mcpkit-backed ones. The `SetHooksForTesting()` method on Manager already demonstrates this pattern for test mocks.

4. **MCP Server Best Practices (mark3labs/mcp-go)**: The mcp-go library encourages tool registration via method chaining. mcpkit's `registry.ToolRegistry` builds on this with middleware support. ralphglasses should adopt mcpkit's registry rather than maintaining its own `Register()` in `tools.go`.

## 5. Actionable Recommendations

### 5.1 Immediate Actions

| # | Action | Files | Effort | Impact | ROADMAP Item |
|---|--------|-------|--------|--------|-------------|
| 1 | Define `SessionStore` interface in `internal/session/store.go` with `Create`, `Get`, `List`, `Update`, `Delete` methods. Extract current JSON-file logic into `FileSessionStore`. | `internal/session/store.go` (new), `internal/session/store_file.go` (new), `internal/session/manager.go` (refactor) | M | High — enables SQLite swap, testability | 2.1 |
| 2 | Define `LoopRunner` interface in `internal/session/looprunner.go` with `Start`, `Step`, `Stop`, `Get`, `List` methods. Manager delegates to interface. | `internal/session/looprunner.go` (new), `internal/session/loop.go` (extract), `internal/session/manager.go` (refactor) | M | High — decouples loop engine from session manager | 6.1 |
| 3 | Define `BudgetTracker` interface in `internal/session/budgettracker.go` with `Check`, `Record`, `Summary` methods. Current `BudgetEnforcer` implements it. | `internal/session/budgettracker.go` (new), `internal/session/budget.go` (implement interface) | S | Medium — prepares for mcpkit/finops swap | 2.3.5 |
| 4 | Add `mcpkit` to `go.mod` with `replace ../mcpkit` directive. Import nothing yet — just verify build compatibility. | `go.mod`, `go.sum` | S | Low (prep) — validates that the dependency can be added without breakage | 6.1 |
| 5 | Move hardcoded `ProviderCostRates` from `costnorm.go:12-16` to a YAML/JSON config file loaded at startup. | `internal/session/costnorm.go`, `configs/cost_rates.yaml` (new) | S | Medium — rates can be updated without code changes | 2.5.5 |

### 5.2 Near-Term Actions

| # | Action | Files | Effort | Impact | ROADMAP Item |
|---|--------|-------|--------|--------|-------------|
| 6 | Create `internal/mcpserver/compat.go` shim re-exporting mcpkit registry types and handler helpers. Migrate `tools.go` handlers to use shim helpers instead of direct mcp-go types. | `internal/mcpserver/compat.go` (new), `internal/mcpserver/helpers.go` (new), `internal/mcpserver/tools.go` (refactor) | L | High — insulates from SDK changes, prepares for module split | 2.8 |
| 7 | Split `tools.go` (3,992 lines) into domain-specific handler files: `handler_fleet.go`, `handler_session.go`, `handler_loop.go`, `handler_team.go`, `handler_workflow.go`, `handler_roadmap.go`, `handler_agent.go`, `handler_journal.go`, `handler_event.go`, `handler_awesome.go`, `handler_rc.go`, `handler_marathon.go`, `handler_snapshot.go`. Keep `tools.go` as the registrar calling each handler file's registration function. | `internal/mcpserver/handler_*.go` (13 new files), `internal/mcpserver/tools.go` (reduce to ~200 lines) | L | High — eliminates merge conflicts, improves navigability | 2.8 |
| 8 | Implement `SQLiteSessionStore` behind the `SessionStore` interface using `modernc.org/sqlite`. Schema: sessions table, events table, WAL mode. | `internal/session/store_sqlite.go` (new), `internal/session/migrations.go` (new), `go.mod` (add modernc.org/sqlite) | L | High — enables queryable sessions, survives restart, Phase 3.3 multi-instance | 2.1.2 |
| 9 | Wire mcpkit's `observability.Init()` into `cmd/mcp.go` and `cmd/root.go`. Add OpenTelemetry span creation for session launch, loop step, and budget check. | `cmd/mcp.go`, `cmd/root.go`, `internal/session/manager.go` (add span creation) | M | High — immediate Prometheus metrics and tracing | 6.4 |
| 10 | Replace `ratelimit.go` (99 lines) with mcpkit `resilience.RateLimiter`. The interface is compatible: both provide `Allow() bool` semantics. | `internal/session/ratelimit.go` (delete), `internal/session/manager.go` (use mcpkit) | S | Medium — eliminates duplicated rate limiter | N/A (tech debt) |
| 11 | Create adapter that wraps mcpkit `ralph.Loop` to implement the `LoopRunner` interface. Keep current loop.go as the default until adapter is battle-tested. | `internal/session/loop_mcpkit.go` (new) | M | High — enables feature flag switch between native and mcpkit loop engine | 6.1 |

### 5.3 Strategic Actions

| # | Action | Files | Effort | Impact | ROADMAP Item |
|---|--------|-------|--------|--------|-------------|
| 12 | Port budget tracking to mcpkit `finops.Tracker` + `finops.CostPolicy`. Map session provider+model to mcpkit's `ModelPricing` table. Remove `budget.go`, `costnorm.go`. | `internal/session/budget.go` (delete), `internal/session/costnorm.go` (delete), `internal/session/budget_finops.go` (new adapter) | L | High — gains token-level tracking, scoped budgets, time windows, Prometheus export | 2.3.5, 5.5, 6.10 |
| 13 | Replace `workflow.go` (265 lines) with mcpkit `workflow.Engine`. Map `WorkflowDef` -> mcpkit `workflow.Graph`, `WorkflowStep` -> mcpkit workflow nodes. Keep YAML parsing. | `internal/session/workflow.go` (delete), `internal/session/workflow_mcpkit.go` (new adapter) | L | High — gains conditional branching, checkpoints, state machines, fork nodes, compensation | 8.3 |
| 14 | Extract `internal/enhancer/` into a standalone Go module `github.com/hairglasses-studio/enhancerkit`. Other sister repos (claudekit, hg-mcp) can import it for prompt improvement without depending on all of ralphglasses. | `internal/enhancer/` (move to separate module), update `go.mod` | XL | High — cross-project reuse of prompt enhancement | N/A (ecosystem) |
| 15 | Migrate `tools.go` handler packages to hg-mcp's `ToolModule` auto-registration pattern. Each domain package has `init()` calling `RegisterModule()`. Main entry imports with `_`. | `internal/mcpserver/tools/fleet/`, `internal/mcpserver/tools/session/`, etc. (15+ new packages) | XL | High — enables independent development of tool domains, eliminates 3,992-line file | 2.8, 3.5.2 |
| 16 | Integrate claudekit's rdcycle pattern for self-improving ralph loops. Implement `rdcycle_scan` -> `rdcycle_plan` -> `rdcycle_verify` -> `rdcycle_commit` -> `rdcycle_report` cycle as a loop profile option. | `internal/session/rdcycle.go` (new), MCP tools (5 new) | XL | High — autonomous self-improvement loops | 6.2, 8.5 |
| 17 | Create shared `github.com/hairglasses-studio/ralphtypes` module defining canonical types: `Session`, `LoopSpec`, `BudgetPolicy`, `Provider`, `EventType`. ralphglasses, mcpkit, claudekit import this instead of defining their own. | New repo or module | XL | High — eliminates type duplication across 5 repos | N/A (ecosystem) |

## 6. Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| mcpkit API churn breaks ralphglasses builds | Medium | High | Use shim/compat layer (hg-mcp pattern). Only 2-3 files need updates on mcpkit changes. Pin mcpkit to known-good commit in `replace` directive. |
| SQLite migration loses existing session data | Low | Medium | Write one-time migration script that reads JSON files and imports into SQLite. Keep JSON fallback for 2 releases. |
| Interface extraction creates regression bugs | Medium | Medium | Extract interfaces without changing behavior first. Run full test suite after each extraction. Use `SetHooksForTesting` pattern already in Manager. |
| tools.go split breaks MCP tool registration | Low | High | Validate tool count before/after split. Add `TestAllToolsRegistered` that asserts expected tool count matches actual registered count. |
| Performance regression from mcpkit middleware overhead | Low | Medium | Benchmark hot paths (session launch, event normalization, budget check) before and after mcpkit integration. mcpkit's own benchmarks show <1ms middleware overhead. |
| Enhancer extraction breaks internal imports | Medium | Medium | Use `replace` directive during transition. Keep `internal/enhancer/` as a thin wrapper that re-exports from the extracted module. |
| Cross-repo type library creates coordination tax | High | Medium | Start minimal: only `Provider`, `SessionStatus`, `EventType` enums. Add types only when two or more repos demonstrably need the same type. |
| Parallel development on tools.go during split | High | Medium | Split tools.go first, before any feature work. Use a feature branch, merge quickly. Communicate to all agents. |

## 7. Implementation Priority Ordering

### 7.1 Critical Path

```
Interface extraction (5.1 #1-3)
    |
    v
Add mcpkit to go.mod (5.1 #4)
    |
    v
Create shim layer (5.2 #6)
    |
    v
Split tools.go (5.2 #7)
    |
    +---> SQLite store (5.2 #8) ---> mcpkit finops (5.3 #12) ---> rdcycle (5.3 #16)
    |
    +---> mcpkit loop adapter (5.2 #11) ---> Replace workflow (5.3 #13)
    |
    +---> Wire observability (5.2 #9) ---> Prometheus/OTel dashboards
```

### 7.2 Recommended Sequence

**Sprint 1 (1-2 weeks): Interface Foundation**
1. Define `SessionStore`, `LoopRunner`, `BudgetTracker` interfaces (5.1 #1-3)
2. Extract current implementations behind interfaces
3. Add mcpkit to `go.mod` with `replace` (5.1 #4)
4. Move cost rates to config file (5.1 #5)

**Sprint 2 (2-3 weeks): Shim + Split**
5. Create mcpserver shim layer (5.2 #6)
6. Split tools.go into domain handler files (5.2 #7)
7. Replace ratelimit.go with mcpkit resilience (5.2 #10)

**Sprint 3 (2-3 weeks): Storage + Observability**
8. Implement SQLiteSessionStore (5.2 #8)
9. Wire mcpkit observability (5.2 #9)

**Sprint 4 (3-4 weeks): Engine Swap**
10. Create mcpkit ralph loop adapter (5.2 #11)
11. Port budget to mcpkit finops (5.3 #12)
12. Replace workflow engine (5.3 #13)

**Sprint 5 (4+ weeks): Ecosystem**
13. Extract enhancer module (5.3 #14)
14. Migrate to ToolModule pattern (5.3 #15)
15. Integrate rdcycle (5.3 #16)
16. Create shared types module (5.3 #17)

### 7.3 Parallelization Opportunities

The following work streams can proceed simultaneously:

- **Stream A**: Interface extraction (#1-3) + shim layer (#6) + tools.go split (#7)
- **Stream B**: SQLite store (#8) — independent of Stream A once `SessionStore` interface is defined
- **Stream C**: Observability (#9) — independent of all other streams
- **Stream D**: Cost config (#5) + ratelimit swap (#10) — independent, small items

After Sprint 1 completes:
- **Stream E**: mcpkit loop adapter (#11) + workflow swap (#13) — depends on `LoopRunner` interface from #2
- **Stream F**: mcpkit finops swap (#12) — depends on `BudgetTracker` interface from #3
- **Stream G**: Enhancer extraction (#14) — independent of all other streams

The key constraint is that interface extraction (Sprint 1) must complete before any mcpkit adapter work begins, because the interfaces define the contract that adapters implement.
