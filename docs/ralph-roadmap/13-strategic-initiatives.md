# 13 -- Strategic Initiatives for ralphglasses

Generated: 2026-04-04
Source: Synthesis of 16 research reports (01--12, s1--s4) + meta-roadmap deep dive (08-ralph-deep-dive.md)

Baseline: 503/1,143 tasks complete (44.0%). 640 remaining across 30 phases. Velocity: ~20 tasks/week (from meta-roadmap assessment). 10 phases complete, 10 in-progress, 10 planned at 0%.

---

## Initiative 1: Concurrency & Safety Hardening

**Fix race conditions, budget enforcement gaps, and silent error paths before any autonomy progression.**

| Attribute | Value |
|-----------|-------|
| Phases spanned | 1 (tail), 10.5 (partial), cross-cutting |
| Estimated task count | ~35 (11 race fixes, 15 error handling fixes, 5 budget gap closures, 4 dead code cleanup) |
| Prerequisites | None -- this is the foundation everything else depends on |
| Effort estimate | 2 weeks |
| Risk level | **HIGH** -- primary risk: R-01 and R-02 are crash-severity concurrent map writes that can panic in production |
| Value proposition | Eliminates all crash-risk data races, closes budget bypass paths, and surfaces silent supervisor failures -- the minimum safety floor for L1+ autonomy |

### Key findings driving this initiative

- **S1 found 2 CRITICAL races** (R-01: `AutoRecovery.retryState` unprotected map; R-02: `RetryTracker.attempts` unprotected map), both reachable from concurrent goroutines and capable of fatal `concurrent map read and map write` panics.
- **S1 found 4 HIGH races** (R-03: global `GateEnabled` var; R-04: `OpenAIClient.LastResponseID`; R-05: `GetTeam` lock ordering inconsistency; R-06: `loadedGroups` map in MCP dispatch) and 5 MEDIUM races.
- **S3 found 3 budget enforcement gaps**: Gap A (`MaxBudgetUSD == 0` launches uncapped sessions), Gap B (active sessions not stopped when `GlobalBudget` exhausted), Gap C (stale `pool.State` allows over-budget launches).
- **S3 found $1,280/hr max theoretical L3 spend** if 128 concurrent sessions run at $10 default budget with no global enforcement.
- **S4 found supervisor swallows cycle failures** (supervisor.go:375 -- `slog.Warn` then continue), RunLoop errors silently discarded in self-improvement path (handler_selfimprove.go:71), and hook exit codes ignored (hooks.go:139).
- **S2 found 4 duplicate CircuitBreaker implementations** across enhancer, gateway, process, and safety packages.

### Scope

1. Fix all 6 CRITICAL + HIGH race conditions from S1 (R-01 through R-06).
2. Fix 5 MEDIUM races (R-07 through R-12) to pass `go test -race ./...` cleanly.
3. Close the 3 budget enforcement gaps from S3 (enforce default budget floor, stop active sessions on global exhaustion, shorten pool.State refresh interval).
4. Surface supervisor cycle failures (S4 fix #1), propagate RunLoop errors (S4 fix #2), and add fleet worker retry (S4 fix #3).
5. Delete confirmed dead code: `fleet_dashboard.go` + test (443 lines from S2).
6. Fix 7 failing test packages (05 report: knowledge race, marathon backoff overflow, mcpserver circuit init, process slice semantics, session boundary conditions, stale tool count, API key gating).

---

## Initiative 2: Test Green & Coverage Integrity

**Achieve a green `go test -race ./...` across all packages and close critical coverage gaps.**

| Attribute | Value |
|-----------|-------|
| Phases spanned | 1 (tail: 1.2.5.1 ParamParser), 0.6 follow-up |
| Estimated task count | ~40 (7 failing package fixes, 8 untested handler files, Phase 1 completion, test infrastructure) |
| Prerequisites | Initiative 1 (race fixes are prerequisite for `-race` passing) |
| Effort estimate | 2 weeks |
| Risk level | **MEDIUM** -- primary risk: coverage padding files (807 tests in 80 files) may mask real gaps |
| Value proposition | Establishes the "green CI" gate required for all autonomous operation; completes Phase 1 |

### Key findings driving this initiative

- **05 found 7 actively failing packages**: `internal/knowledge` (fatal concurrent map writes), `internal/marathon` (backoff overflow), `internal/mcpserver` (circuit breaker init), `internal/process` (nil vs empty slice), `internal/session` (6 failing tests), `cmd/ralphglasses-mcp` (stale tool count), `cmd/prompt-improver` (missing API key guard).
- **05 found 9,267 Test* functions across 745 files** but 84.5% reported coverage masks structural weaknesses: 8 handler files have no test counterpart, including `handler_sweep_report.go` (302 lines, highest risk).
- **02 found 0/166 tools have all 4 MCP annotation hints** -- `IdempotentHint` is universally absent on read-only tools.
- **01 found Phase 1 at 96%** with only 2 tasks remaining: 1.2.5.1 (ParamParser extraction, P1 L) and 1.2.5.4 (handler generator, blocked by 1.2.5.1).

### Scope

1. Fix all 7 failing packages to achieve green `go test -race ./...`.
2. Complete Phase 1 (1.2.5.1 ParamParser, 1.2.5.4 handler generator).
3. Add test files for the 5 highest-risk untested handlers: `handler_sweep_report.go`, `handler_prompt_ab.go`, `handler_provider_benchmark.go`, `handler_roadmap_prioritize.go`, `handler_session_fork.go`.
4. Add `IdempotentHint` to all read-only tools; add `OpenWorldHint` to tools that call external APIs.
5. Fix stale `handleLoadToolGroup` description (missing rdcycle, plugin, sweep namespaces from 02).
6. Activate deferred loading in production binary (`DeferredLoading = true` before `rg.Register`).

---

## Initiative 3: Cost Control & Multi-Provider Cascade

**Close budget bypass paths, activate multi-provider cascade routing, and reduce average task cost from $0.17 to $0.05.**

| Attribute | Value |
|-----------|-------|
| Phases spanned | 6 (tail: 6.6 model routing), 10 (partial), 24 (MoE routing) |
| Estimated task count | ~50 (cost rate updates, cascade activation, predictor wiring, 4-tier routing, batch integration) |
| Prerequisites | Initiative 1 (budget gaps must be closed first) |
| Effort estimate | 3 weeks |
| Risk level | **MEDIUM** -- primary risk: zero production cost data for Gemini/Codex paths; cascade is untested in multi-provider scenarios |
| Value proposition | 70-85% cost reduction ($0.17 to $0.03-0.05/task), multi-provider resilience, and the cost observability required for fleet-scale operations |

### Key findings driving this initiative

- **S3 found compiled-in Codex rates may be stale** (`gpt-5.4` at $2.50/$15.00 -- actual model mapping unclear) and **Gemini flash output underestimated by ~40%** ($2.50 compiled vs ~$3.50 actual for 2.5 Flash).
- **S3 found `fleet/CostPredictor` is not wired to `handleWorkComplete`** -- the fleet predictor starts empty every restart.
- **S3 found the auto-size formula silently inflates per-session budget** (Step 2: `budgetUSD = estimatedPerSession * 1.5` can raise $5.00 to $6.30 without caller awareness).
- **11 found $0.17 to $0.03/task is achievable** via combined prompt caching + 4-tier cascade + batch API, with Gemini 2.5 Flash-Lite at $0.10/1M input as the ultra-cheap tier.
- **08-deep-dive found 100% Claude provider distribution** across 36 observed tasks -- cascade router is implemented but never exercised.
- **04 found default target provider is OpenAI** (config.go:236 fallback) -- surprising for Claude-first users.
- **S3 found 3 ways concurrent sessions can exceed fleet budget** (reservation vs actual, stale pool.State, advisory worker limits).

### Scope

1. Update compiled-in provider cost rates to April 2026 pricing (11 report table).
2. Wire `fleet/CostPredictor.Record()` into `handleWorkComplete`.
3. Implement Tier 0 classifier using GPT-4.1-nano ($0.05/1M) for task routing.
4. Activate cascade routing with Gemini 2.5 Flash as Tier 1, Sonnet 4.6 as Tier 2, Opus 4.6 as Tier 3.
5. Fix auto-size inflation by capping at `max(budgetUSD, estimatedPerSession * 1.5)` only when caller did not explicitly set budget.
6. Add org-wide cost rate override file (currently per-repo only).
7. Add cost rate drift alerting (compare compiled-in rates to live billing).

---

## Initiative 4: MCP Tool Layer Modernization

**Activate deferred loading, adopt the official Go SDK, restructure oversized namespaces, and complete tool annotations.**

| Attribute | Value |
|-----------|-------|
| Phases spanned | 1.5 (1.5.11: official SDK migration, XL), 10.5 (tool surface) |
| Estimated task count | ~45 (SDK migration, namespace restructure, annotation completion, deferred loading, middleware gaps) |
| Prerequisites | Initiative 2 (deferred loading activation, annotation work) |
| Effort estimate | 3 weeks |
| Risk level | **HIGH** -- primary risk: mcp-go to official go-sdk migration is an XL task touching every tool registration path |
| Value proposition | Halves MCP startup latency via deferred loading, aligns with the AAIF-governed official SDK (semver stability), and reduces the 166-tool context window burden for connected agents |

### Key findings driving this initiative

- **02 found deferred loading is inactive in production** -- all 166 tools load eagerly despite the documented deferred model. Only tests set `DeferredLoading = true`.
- **02 found the `advanced` namespace has 24 tools spanning 7+ distinct domains** (RC, events, HITL, autonomy, feedback, journals, workflows) -- should be split into at least 3 sub-namespaces.
- **02 found 7 misplaced tools** (worktree in observability, loop_await/poll in observability, event_list/poll in advanced, session_handoff in loop).
- **10 found the official `modelcontextprotocol/go-sdk` v1.4+ exists** with semver stability guarantee, maintained by MCP org + Google. mcpkit is pinned to mcp-go v0.46.0 (pre-1.0, breaking changes possible).
- **10 found the 2025-11-25 spec adds Tasks primitive, Extensions, and structured output** -- features ralphglasses should adopt.
- **02 found no per-tool rate limiting** -- a single slow tool can consume all 32 concurrency slots.
- **09 found the competitive landscape has no Go-native MCP-first orchestrator** -- this is ralphglasses' unique moat, worth protecting with official SDK alignment.

### Scope

1. Migrate from mcp-go v0.46.0 to official `modelcontextprotocol/go-sdk` (1.5.11).
2. Activate deferred loading in the production binary.
3. Split `advanced` namespace (24 tools) into `rc` (4), `autonomy` (4), `workflow` (3), and residual `advanced` (13).
4. Move 7 misplaced tools to correct namespaces.
5. Complete all 166 tool annotations to 4/4 MCP spec hints.
6. Add per-namespace concurrency caps to `ConcurrencyMiddleware`.
7. Add `MaxPromptLength` validation to `prompt_improve` handler.

---

## Initiative 5: TUI Performance & Fleet Dashboard

**Eliminate unbounded memory growth, replace polling with event-driven updates, and ship the fleet operations dashboard.**

| Attribute | Value |
|-----------|-------|
| Phases spanned | 1.5 (1.5.10: Charm v2, XL), 3 (tail), 6 (tail: 6.8.4 TUI A/B, 6.9 NL control) |
| Estimated task count | ~55 (Charm v2 migration, tick optimization, LogView bounds, fleet dashboard, ViewSearch) |
| Prerequisites | Initiative 2 (test green gate) |
| Effort estimate | 3 weeks |
| Risk level | **MEDIUM** -- primary risk: Charm Bubble Tea v2 migration is XL and touches every view/component |
| Value proposition | Sub-100ms render at 100+ sessions, bounded memory for long-running operations, and the operational dashboard needed for fleet-scale monitoring |

### Key findings driving this initiative

- **07 found the 2-second tick performs synchronous I/O for every repo** (69+ file reads per tick in the hairglasses-studio ecosystem). At 100+ repos, tick duration grows linearly and blocks the render loop.
- **07 found `LogView.Lines` is unbounded** -- a session running for hours with verbose output will exhaust memory. No max-lines cap exists.
- **07 found `LoopRun.Iterations` accumulates indefinitely** -- `SnapshotLoopControl` iterates all iterations to compute average duration on every 2s tick. At 500+ iterations this is O(n) per tick.
- **07 found `updateTable()`, `updateSessionTable()`, and `updateTeamTable()` run on every tick regardless of current view** -- rebuilding all rows even when the user is looking at a log view.
- **07 found `ViewSearch` is defined in the `ViewMode` iota but has no handler** -- the view dispatch map has no entry for it.
- **S2 confirmed `FleetDashboardModel` in `fleet_dashboard.go` is dead code** (207 lines + 236-line test) -- `FleetView` in `fleet.go` is the live implementation.
- **07 found `CostHistory` aggregation creates a large intermediate slice every 2 seconds** from all sessions before trimming to 20 samples.

### Scope

1. Migrate to Charm Bubble Tea v2 (1.5.10).
2. Replace 2-second polling tick with event bus subscription for session state changes.
3. Add max-lines cap to `LogView.Lines` (ring buffer, configurable default 10,000).
4. Skip `updateTable`/`updateSessionTable`/`updateTeamTable` when current view is not the overview.
5. Add virtual scrolling for fleet dashboard at 100+ sessions.
6. Implement `ViewSearch` or remove the iota entry.
7. Complete Phase 3 remaining tasks (3.1.4 monitor enum, 3.2.3 `:layout` command, 3.4 autorandr hotplug).

---

## Initiative 6: Bootable Thin Client

**Ship the Manjaro-based bootable thin client with Sway kiosk, dual-GPU support, and secrets management.**

| Attribute | Value |
|-----------|-------|
| Phases spanned | 4 (21% done, 42 remaining), 3 (partial: multi-monitor) |
| Estimated task count | ~50 (ISO pipeline, kiosk hardening, dual GPU, secrets, Sway-primary, drop i3) |
| Prerequisites | Initiative 5 (TUI must be performant for kiosk mode) |
| Effort estimate | 3 weeks |
| Risk level | **HIGH** -- primary risk: dual NVIDIA RTX 4090 under Sway/wlroots has known PRIME copy issues between two NVIDIA cards |
| Value proposition | Boots directly into the ralphglasses TUI, enabling dedicated agent workstations that require zero manual setup |

### Key findings driving this initiative

- **08 found no disk encryption** in the thin client build -- `/etc/crypttab` and LUKS setup are absent.
- **08 found no secrets management** -- API keys are expected in environment variables with no 1Password CLI, SOPS, or Vault integration.
- **08 found autorandr integration is unimplemented** -- tasks 3.4.1-3.4.4 (hotplug-triggered profile reload) are all incomplete.
- **12 recommended staying with Manjaro for Phase 4** (existing Dockerfile.manjaro works) and migrating to NixOS for fleet deployment (Phase 5+).
- **12 recommended dropping i3 support** -- X11 is end-of-life for new deployments, and i3 adds testing burden with no users.
- **12 found Cage compositor** as an ideal minimal kiosk option for fleet workers (zero-config, single maximized app).
- **08 found `hw-detect.sh` only handles single RTX 4090** -- dual 4090 PCI ID detection is not implemented.
- **12 found `WLR_DRM_DEVICES` limitation** -- Sway/wlroots renders on a single GPU; outputs on the second card use PRIME copy.

### Scope

1. Complete ISO build pipeline (4.1.3 -- blocks 4.2/4.5/4.10).
2. Add LUKS disk encryption to thin client build.
3. Integrate 1Password CLI (`op read`) for secrets management.
4. Drop i3 support: archive `distro/i3/`, `distro/xorg/`, and `kiosk-setup.sh`.
5. Make Sway the primary compositor with Hyprland as opt-in alternative.
6. Extend `hw-detect.sh` for dual RTX 4090 detection.
7. Implement greetd/tuigreet boot-to-TUI pipeline (from 12 recommendations).
8. Evaluate Cage for fleet worker thin clients.

---

## Initiative 7: Fleet Scaling & A2A Protocol

**Complete horizontal scaling infrastructure (NATS, multi-node, SQLite WAL) and ship A2A protocol integration for cross-agent delegation.**

| Attribute | Value |
|-----------|-------|
| Phases spanned | 10.5 (29% done, 34 remaining), 11 (A2A, 0%), 12 (Tailscale, partial) |
| Estimated task count | ~75 (NATS transport, multi-node coordination, SQLite WAL migration, A2A SDK adoption, Tailscale embedding) |
| Prerequisites | Initiative 1 (race fixes), Initiative 3 (cost control at fleet scale) |
| Effort estimate | 4 weeks |
| Risk level | **HIGH** -- primary risk: multi-node coordination is architecturally complex; NATS adds an external dependency |
| Value proposition | Enables multi-machine agent fleets coordinated over Tailscale, with A2A interop for external agent ecosystems |

### Key findings driving this initiative

- **06 found the work queue is unbounded and in-memory** -- all queue state is lost on restart. `SaveTo/LoadFrom` provide optional JSON serialization but the coordinator does not call them automatically.
- **06 found sweep fan-out is serial** -- sessions launch one at a time in a for-loop, not concurrently. A sweep across 69 repos takes minutes just for launch overhead.
- **06 found autoscaler scale-up is advisory only** -- no new worker processes are spawned. External orchestration must watch for `fleet.autoscale` events.
- **03 found dual-write persistence is transitional** -- JSON files and SQLite store coexist. Cross-process discovery (TUI to MCP server) depends on JSON files with no SQLite replacement.
- **06 found A2A push notifications are stub** -- `streaming: true` is advertised but no SSE stream exists. Authentication is stub-only (Tailscale network auth but no A2A-level token validation).
- **10 found the official A2A Go SDK exists** (`a2aproject/a2a-go/v2`) with gRPC, JSON-RPC, and REST support.
- **09 confirmed ralphglasses occupies a unique competitive position** -- no other Go-native, multi-provider, MCP-first orchestrator exists with fleet distribution.

### Scope

1. Complete Phase 10.5: NATS transport, multi-node coordination, per-namespace rate limiting, SQLite WAL migration.
2. Bound the work queue (capacity limit, persistent backing store).
3. Parallelize sweep fan-out (concurrent session launches with configurable parallelism).
4. Complete A2A protocol integration (Phase 11): adopt official Go SDK, implement SSE streaming, add A2A-level authentication.
5. Begin Tailscale fleet networking (Phase 12): `tsnet` embedding, `ts-enroll.sh`, peer discovery.
6. Wire autoscaler to actually spawn worker processes on single-machine deployments.

---

## Initiative 8: Sandboxing & Security

**Implement agent sandboxing, close path traversal vulnerabilities, and establish the security foundation for multi-tenant and unattended operation.**

| Attribute | Value |
|-----------|-------|
| Phases spanned | 5 (12% done, 35 remaining), 17 (partial: safety boundaries) |
| Estimated task count | ~55 (network isolation, Firecracker/gVisor, path traversal fixes, budget federation, safety guardrails) |
| Prerequisites | Initiative 1 (budget enforcement), Initiative 7 (fleet infrastructure) |
| Effort estimate | 3 weeks |
| Risk level | **HIGH** -- primary risk: sandbox escape in shared-machine deployments; Firecracker/gVisor complexity |
| Value proposition | Enables multi-tenant fleet operation where agent sessions cannot access each other's data or exceed their resource budgets |

### Key findings driving this initiative

- **08-deep-dive found 5 BLOCKER-level path traversal vulnerabilities** in MCP handlers: `scratchpadName`, `name`, `worktree_paths`, `hooks.yaml command`, and `verify_commands` flow into `filepath.Join` or `exec.CommandContext` without validation.
- **08 found no encryption or secrets management** in the thin client build.
- **S4 found hook exit codes silently discarded** (`hooks.go:139` -- `_ = cmd.Run()`), meaning pre/post session safety gates are invisible failures.
- **06 found budget federation (Phase 5.5)** extends per-session tracking with a global pool and budget dashboard across sandboxed sessions -- overlaps with but extends Phase 2.3.
- **01 found Phase 5 at 12%** (5/40 done) with Docker isolation complete but network isolation, secrets, Firecracker, and gVisor all pending.

### Scope

1. Fix 5 BLOCKER path traversal vulnerabilities (Sprint 7 audit findings).
2. Complete Phase 5 network isolation (5.2-5.3).
3. Implement Firecracker or gVisor sandboxing for untrusted agent sessions (5.4-5.5).
4. Add budget federation across sandboxed sessions (5.5).
5. Begin Phase 17.1 safety boundaries: allowlists, resource limits, per-provider circuit breakers.
6. Surface hook exit codes (S4 fix #6).

---

## Initiative 9: Autonomous R&D (L2) & Self-Improvement

**Complete the self-improvement pipeline, harden the supervisor, and achieve reliable L2 autonomy (auto-optimize).**

| Attribute | Value |
|-----------|-------|
| Phases spanned | 8 (24% done: self-improvement), 9.5 (done), 10 (15% done), 13 (L3 foundation) |
| Estimated task count | ~65 (supervisor hardening, planner reliability, self-test CI gate, self-improvement acceptance, L3 decision engine stub) |
| Prerequisites | Initiative 1 (safety hardening), Initiative 2 (green CI), Initiative 3 (cost control) |
| Effort estimate | 4 weeks |
| Risk level | **HIGH** -- primary risk: planner JSON retry rate at 25.7% (target <5%) makes autonomous operation unreliable |
| Value proposition | Reliable L2 autonomy: the system can auto-optimize its own code, run R&D cycles unattended for hours, and self-test its changes |

### Key findings driving this initiative

- **08-deep-dive found JSON format enforcement is the top failure mode** -- 25 occurrences across 15 cycles (25.7% retry rate, target <5%). The planner prompt has been refined but the gap is 5x.
- **S4 found supervisor silently swallows cycle failures** at all 3 levels (RunCycle, chained cycle, planned sprint) -- L2 autonomy without error surfacing means the agent spins silently.
- **03 found the cascade confidence threshold is 0.7 (heuristic)** because the `DecisionModel` needs 50+ multi-provider observations to train, and all 36 observations are Claude-only. The calibrated path is untested.
- **08-deep-dive found L3 requires ~100 tasks across Phases 10.5, 13, 17, 14, 15** -- all currently at 0%.
- **04 found the shared `CircuitBreaker` in `HybridEngine`** trips for all providers when one is flaky (S1 R-13).
- **03 found the supervisor ran for 43 minutes on macOS** (state file has `$HOME/` path) -- it has never been validated on the Manjaro workstation.

### Scope

1. Fix planner JSON reliability to <5% retry rate (structured output format, fallback parser).
2. Surface supervisor cycle/sprint failures as errors with HITL interrupts after N consecutive failures.
3. Implement per-provider circuit breakers (replace shared CB in HybridEngine).
4. Ship Stage 2 CI self-test integration (ready now, no dependencies).
5. Ship Stage 3 self-improvement with two-tier acceptance (safe paths auto-merge, core paths create PRs).
6. Complete Phase 10 Claude Code native integration tasks (10.1-10.4).
7. Begin Phase 13.1 self-healing runtime (heartbeat, auto-restart, crash recovery) as L3 foundation.

---

## Initiative 10: Prompt Enhancement & Observability

**Mature the enhancer pipeline, add OpenTelemetry tracing, and build the evaluation harness for systematic quality measurement.**

| Attribute | Value |
|-----------|-------|
| Phases spanned | 23 (prompt engineering), 21 (observability/evaluation), 8 (partial: 8.6 knowledge graph) |
| Estimated task count | ~55 (OTel integration, prompt caching all providers, evaluation harness, knowledge graph, RAG) |
| Prerequisites | Initiative 3 (multi-provider cascade must be active for cross-provider evaluation) |
| Effort estimate | 3 weeks |
| Risk level | **LOW** -- these are additive capabilities with no safety implications |
| Value proposition | End-to-end observability from MCP tool call through LLM API call and back; systematic prompt quality measurement; 40-60% input token cost savings via prompt caching |

### Key findings driving this initiative

- **04 found the default target provider is OpenAI** (`config.go:236`) when neither `LLM.Provider` nor `TargetProvider` is set -- surprising for Claude-first users.
- **04 found `FormatContextBlockMarkdown()` is defined but never called** from the pipeline -- the knowledge injector always uses XML format regardless of target provider.
- **04 found FINDING-240 score inflation** was addressed through baseline constant adjustments, but no runtime calibration test enforces the <1.2x target.
- **02 found tracing is not end-to-end** -- `TraceMiddleware` generates trace IDs but there is no correlation with outgoing LLM API calls. OTel is initialized but only used if `OTEL_EXPORTER_OTLP_ENDPOINT` is set.
- **11 found prompt caching saves 40-60% on input tokens** across all 3 providers (Claude cache_control, Gemini cachedContents, OpenAI prefix caching) -- partially implemented.
- **04 found 11+ lint rules** with varying false-positive rates; `vague-quantifier` rule flags "good" and "nice" as vague (high FP rate).

### Scope

1. Fix default target provider to match LLM provider (Claude when using Claude API).
2. Wire `FormatContextBlockMarkdown()` into the pipeline for Gemini/OpenAI targets.
3. Add runtime calibration test for scoring (<1.2x inflation check).
4. Complete OTel integration: correlate MCP trace IDs with outgoing LLM API call spans.
5. Activate prompt caching for all 3 providers in the session launch path.
6. Begin evaluation harness (Phase 21.2): SWE-bench, tau-bench, pass@k measurement.
7. Tune lint rule false-positive rates (lower threshold for `vague-quantifier`, `decomposition-needed`).

---

## Recommended Execution Order

1. **Initiative 1: Concurrency & Safety Hardening** (weeks 1-2)
2. **Initiative 2: Test Green & Coverage Integrity** (weeks 2-3, overlaps week 2 with I1)
3. **Initiative 3: Cost Control & Multi-Provider Cascade** (weeks 3-5)
4. **Initiative 4: MCP Tool Layer Modernization** (weeks 4-6, overlaps with I3)
5. **Initiative 5: TUI Performance & Fleet Dashboard** (weeks 5-7)
6. **Initiative 9: Autonomous R&D (L2) & Self-Improvement** (weeks 6-9, overlaps with I5)
7. **Initiative 6: Bootable Thin Client** (weeks 8-10)
8. **Initiative 10: Prompt Enhancement & Observability** (weeks 8-10, parallel with I6)
9. **Initiative 7: Fleet Scaling & A2A Protocol** (weeks 10-13)
10. **Initiative 8: Sandboxing & Security** (weeks 12-14, overlaps with I7)

---

## Dependency DAG

```
I1 Concurrency & Safety Hardening
 |
 +---> I2 Test Green & Coverage Integrity
 |      |
 |      +---> I4 MCP Tool Layer Modernization
 |      |
 |      +---> I5 TUI Performance & Fleet Dashboard
 |             |
 |             +---> I6 Bootable Thin Client
 |
 +---> I3 Cost Control & Multi-Provider Cascade
        |
        +---> I9 Autonomous R&D (L2) & Self-Improvement  <--- also depends on I1, I2
        |
        +---> I10 Prompt Enhancement & Observability
        |
        +---> I7 Fleet Scaling & A2A Protocol  <--- also depends on I1
               |
               +---> I8 Sandboxing & Security  <--- also depends on I1, I7
```

Text representation of all edges:

```
I1 --> I2, I3, I7, I8, I9
I2 --> I4, I5, I9
I3 --> I7, I9, I10
I4 --> (none downstream)
I5 --> I6
I7 --> I8
I9 --> (none downstream, but is the L2 autonomy gate)
```

---

## Quarter-Level Timeline

### Q2 2026 (April -- June)

| Month | Initiatives | Milestone |
|-------|------------|-----------|
| April | I1 (Concurrency & Safety Hardening), I2 start | All CRITICAL/HIGH races fixed. 7 failing packages green. |
| May | I2 complete, I3 (Cost Control), I4 start | Phase 1 complete. Multi-provider cascade active. `go test -race ./...` green. |
| June | I4 complete, I5 (TUI Performance), I9 start | Official MCP SDK adopted. Deferred loading active. Charm v2 migrated. |

**Q2 exit criteria:** Green CI, multi-provider cascade routing with cost data, official MCP SDK, L1 autonomy stable.

### Q3 2026 (July -- September)

| Month | Initiatives | Milestone |
|-------|------------|-----------|
| July | I9 (Autonomous R&D L2), I5 complete | Self-improvement pipeline shipping. TUI handles 100+ sessions. |
| August | I6 (Bootable Thin Client), I10 (Observability) | Bootable ISO with Sway kiosk. OTel tracing end-to-end. |
| September | I7 start (Fleet Scaling & A2A) | L2 autonomy validated (multi-hour unattended R&D cycles). |

**Q3 exit criteria:** L2 autonomy operational, bootable thin client shipping, prompt caching active across all providers.

### Q4 2026 (October -- December)

| Month | Initiatives | Milestone |
|-------|------------|-----------|
| October | I7 complete (Fleet Scaling & A2A) | Multi-node fleet over Tailscale. A2A protocol live. |
| November | I8 (Sandboxing & Security) | Firecracker/gVisor sandboxing. Path traversal vulns closed. |
| December | I8 complete, L3 foundation assessment | Security hardened for multi-tenant. Decision: proceed to L3 or consolidate. |

**Q4 exit criteria:** Multi-machine fleet operational, A2A interop live, sandboxing complete, L3 readiness assessment.

### Q1 2027 (January -- March)

| Month | Focus | Milestone |
|-------|-------|-----------|
| January | Phase 13 (L3 Autonomy Core) | Self-healing runtime, decision engine. |
| February | Phase 14 (Agent Memory), Phase 17 (Safety) | Persistent memory, safety guardrails. |
| March | L3 integration testing | 72-hour unattended operation validated. |

**Q1 2027 exit criteria:** L3 autonomy operational with safety boundaries and persistent memory.

---

## Summary

These 10 initiatives group the 640 remaining roadmap tasks into a sequenced execution plan:

| # | Initiative | Tasks | Weeks | Gate |
|---|-----------|-------|-------|------|
| 1 | Concurrency & Safety Hardening | ~35 | 2 | Race-free, budget-enforced |
| 2 | Test Green & Coverage Integrity | ~40 | 2 | Green CI, Phase 1 complete |
| 3 | Cost Control & Multi-Provider Cascade | ~50 | 3 | $0.05/task avg, 3-provider active |
| 4 | MCP Tool Layer Modernization | ~45 | 3 | Official SDK, deferred loading |
| 5 | TUI Performance & Fleet Dashboard | ~55 | 3 | Sub-100ms at 100+ sessions |
| 6 | Bootable Thin Client | ~50 | 3 | Bootable ISO with Sway kiosk |
| 7 | Fleet Scaling & A2A Protocol | ~75 | 4 | Multi-node fleet, A2A live |
| 8 | Sandboxing & Security | ~55 | 3 | Sandboxed multi-tenant |
| 9 | Autonomous R&D (L2) | ~65 | 4 | Multi-hour unattended R&D |
| 10 | Prompt Enhancement & Observability | ~55 | 3 | OTel e2e, prompt caching |
| | **Total** | **~525** | **~30** | |

The remaining ~115 tasks (640 minus 525) fall in Phases 14-25 (agent memory, swarm intelligence, edge agents, world models, cross-repo orchestration, marketplace, federated learning). These are deferred to post-L3 and are not included in any initiative. They represent the research/vision layer that should only be prioritized after L3 autonomy is operational.
