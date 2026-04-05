# Recommendations: Tools, Skills, and Future Enhancements

Generated 2026-04-04 from 32 research agents + 27 implementation agents across 7 sprints.

## New MCP Tools to Create

### 1. `ralphglasses_spend_rate` (Priority: HIGH)
- Report current hourly spend rate from SpendRateMonitor
- Show: current rate, threshold, tripped status, bucket histogram
- Namespace: `core` (always loaded)

### 2. `ralphglasses_cost_rates_check` (Priority: MEDIUM)
- Compare compiled-in rates to documented rates
- Flag staleness based on CostRatesVerified date
- Namespace: `fleet` or `advanced`

### 3. `ralphglasses_namespace_info` (Priority: LOW)
- Show tool counts per namespace, loaded/deferred status, misplaced tool warnings
- Complement to existing `tool_groups` meta-tool

## New Claude Code Skills to Create

### 1. `/ralph-sweep` skill
- Launch a parallelized sweep across T1 repos with budget controls
- Default: `sweep_concurrency=10`, `budget_usd=0.50`, `--no-session-persistence`
- Auto-report results when complete

### 2. `/ralph-health` skill
- Run comprehensive health check: `go build`, `go vet`, `go test -race`, check cost rate staleness, verify spend monitor threshold, check supervisor status
- Single command for pre-L2/L3 gate validation

### 3. `/ralph-cascade` skill
- Show current cascade routing tiers with costs
- Verify tier order matches actual pricing
- Report DecisionModel calibration status (sample count, confidence)

## Future Enhancement Priorities (Next Sessions)

### Tier 1: L2 Gate Prerequisites (8 weeks)
| Item | Design Doc | Effort | Status |
|------|-----------|--------|--------|
| TUI event bus integration (Step 3 of tick opt) | s8 | M | Design ready |
| CircuitBreaker consolidation (4→1 package) | s12 | L | Design ready |
| Official MCP Go SDK migration | — | XL | Research done (Agent 10) |
| 24-hour supervisor validation on Manjaro | — | M | Requires manual |
| 50+ multi-provider observations for DecisionModel | — | M | Requires runtime |

### Tier 2: L3 Gate Prerequisites (20 weeks)
| Item | Design Doc | Effort | Status |
|------|-----------|--------|--------|
| Autoscaler local actuator (spawn workers) | — | M | Research done (Agent 6) |
| A2A protocol adoption (Go SDK) | — | L | Research done (Agent 9) |
| Self-healing runtime (Phase 13.1) | — | XL | Roadmap planned |
| systemd watchdog for supervisor | — | S | Straightforward |
| 72-hour unattended L3 validation | — | L | Requires all above |

### Tier 3: Platform Maturity
| Item | Design Doc | Effort | Status |
|------|-----------|--------|--------|
| Bootable thin client ISO (Manjaro+Sway) | s5-s8 from distro audit | XL | Phase 4, 21% done |
| Drop i3 support, focus on Sway | — | M | Recommended by Agent 12 |
| Charm v2 TUI migration | — | XL | Phase 3.5 |
| Fleet dashboard virtual scrolling | — | M | For 100+ sessions |
| Tailscale fleet networking | — | L | Phase 12 |

## Architecture Recommendations

### 1. Consolidate CircuitBreaker (s12)
Four independent implementations → one `internal/circuit` leaf package. The `safety.CircuitBreaker` is the best base (has metrics, clock injection, Execute wrapper). Process CB wraps it with sliding window + persistence.

### 2. Complete Event Bus Migration (s8 Step 3)
The event bus is already wired and publishing session/loop/cost events. Step 3 subscribes the TUI for reactive updates — eliminates the remaining 5s polling for session state changes, achieving sub-millisecond update latency.

### 3. MCP SDK Migration Path
mcpkit's shim architecture isolates the mcp-go dependency. Migration to official `modelcontextprotocol/go-sdk` v1.4+ is an mcpkit-internal change with zero impact on ralphglasses handler code. Should be done before the Go SDK reaches v2.0.

### 4. Cost Rate Automation
Current approach (compile-time constants) doesn't scale. Recommended path:
1. Short-term: CostRatesVerified staleness check (DONE this session)
2. Medium-term: JSON override file (`cost_rates.json`) already supported
3. Long-term: Fetch rates from provider APIs at startup, cache locally

## Research Documents Index

| File | Topic | Key Finding |
|------|-------|-------------|
| 01-roadmap-matrix.md | Phase status | 26 phases, 503/1143 tasks (44%) |
| 02-mcp-tool-audit.md | MCP tools | 166 tools, was 0/166 annotated |
| 03-session-architecture.md | Session layer | 7 race conditions identified |
| 04-enhancer-pipeline.md | Prompt enhancement | 13 stages, shared CB problem |
| 05-test-coverage.md | Tests | 9267 tests, 7 failing packages |
| 06-fleet-sweep.md | Fleet | Unbounded queue, serial sweep |
| 07-tui-audit.md | TUI | LogView unbounded, O(n) dashboard |
| 08-distro-audit.md | Thin client | No encryption, hardcoded GPU |
| 09-orchestration-landscape.md | Competition | Unique Go-native MCP position |
| 10-mcp-ecosystem.md | MCP spec | Official Go SDK exists (v1.4+) |
| 11-llm-capabilities.md | Providers | $0.17→$0.03 cost reduction |
| 12-thin-client-patterns.md | Boot | Stay Manjaro, drop i3, greetd |
| 13-strategic-initiatives.md | Strategy | 10 initiatives, Q2-Q1 2027 |
| 14-autonomy-path.md | Autonomy | L1=2wk, L2=8wk, L3=20wk |
| 15-fleet-readiness.md | Fleet | 19/77 repos ready |
| 16-research-index.md | Index | 82 findings, 120+ URLs |
| 18-reconciliation.md | Reconciliation | 46 new tasks proposed |
| 19-stress-test.md | Safety | L3 grade D+, safety unwired |
| 20-summary.json | Machine data | Structured JSON for automation |
| s1-s4 | Deep dives | Races, dead code, cost, errors |
| s5-s7 | Designs | Sweep, LogView, namespaces |
| s8-s10 | Designs | TUI tick, spend breaker, cost audit |
| s11-s12 | Research | Test failures, CB consolidation |
