# 01 -- ralphglasses Roadmap Matrix

Generated: 2026-04-04

Source: `ralphglasses/ROADMAP.md` (2,736 lines, 1,143 checkbox tasks — 503 done, 640 open)

> **Note on counts:** The ROADMAP.md header states "1,115 tasks, 442 complete." Live `grep` of checkbox items finds **503 `[x]`** and **640 `[ ]`** for a total of 1,143. The header predates several recent phases. All analysis here uses the live grep count. The `08-ralph-deep-dive.md` entry (503 done / 1,140 total) matches the checkbox count and is used as the baseline; this document updates it and adds five new analysis dimensions.

---

## 1. Phase-by-Phase Status Matrix

This table supersedes the phase table in `08-ralph-deep-dive.md`. Task counts were re-derived from live checkbox counts in ROADMAP.md; percentages are recalculated accordingly. Dependency and blocker columns are original additions.

| # | Phase | Name | Total | Done | % | Key Blockers | Direct Dependencies |
|---|-------|------|-------|------|---|--------------|---------------------|
| 0 | 0 | Foundation | 16 | 16 | 100% | None | None |
| 1 | 0.5 | Critical Fixes | 45 | 45 | 100% | None | Phase 0 |
| 2 | 0.6 | Code Quality & Observability | 39 | 39 | 100% | None | Phase 0.5 |
| 3 | 0.7 | Codebase Hardening | merged→0.6 | — | — | N/A | Phase 0.6 |
| 4 | 0.8 | MCP Observability & Scratchpad | 20 | 20 | 100% | None | Phase 0.9 |
| 5 | 0.9 | Quick Wins | 12 | 12 | 100% | None | Independent |
| 6 | 1 | Harden & Test | 55 | 53 | 96% | 1.2.5.1 ParamParser (P1 L); 1.2.5.4 blocked by 1.2.5.1 | Phase 0.5 |
| 7 | 1.5 | Developer Experience | 52 | 43 | 83% | 1.5.10 Charm v2 (P1 XL); 1.5.11 official SDK (P1 XL); 3.5.5.5–3.5.5.8 Codex parity (P0–P1) | Phase 0.5.7 (version ldflags) |
| 8 | 2 | Multi-Session Fleet | 70 | 70 | 100% | None | Phase 1 |
| 9 | 2.5 | Multi-LLM Orchestration | 27 | 27 | 100% | None | Phase 2.1 |
| 10 | 2.75 | Architecture & Capability Extensions | 33 | 33 | 100% | None | Phase 2.5 |
| 11 | 3 | i3 Multi-Monitor Integration | 35 | 24 | 69% | 3.1.4 monitor enum; 3.1.5 event listener; 3.2.3–3.2.4 layout cmds; 3.3.1 SQLite; 3.4 autorandr (4 tasks); 3.6.1 Hyprland IPC; 3.6.4 Hyprland workspaces | Phase 2 |
| 12 | 3.5 | Theme & Plugin Ecosystem | 30 | 24 | 80% | 3.5.4.5 Codex plugin bundle; 3.5.5.5–3.5.5.8 Codex parity (NEW section, 4 open) | Phase 3 |
| 13 | 4 | Bootable Thin Client | 53 | 11 | 21% | 4.1.3 iso target (blocks 4.2/4.5/4.10); 4.1.4–4.1.7; all of 4.2, 4.3, 4.4.1–4.4.4, 4.5, 4.6, 4.9 | Phase 3 (i3/WM integration) |
| 14 | 5 | Agent Sandboxing & Infrastructure | 40 | 5 | 12% | 5.2–5.8 all open; 5.1 done (Docker); network isolation, secrets, Firecracker, gVisor | Phase 2 (session model) |
| 15 | 6 | Advanced Fleet Intelligence | 50 | 42 | 84% | 6.1.3 DAG viz (P2 L); 6.4.3 OTel; 6.8.4 TUI A/B; 6.8.5 auto-promote; 6.9.1–6.9.4 NL control | Phase 2, Phase 5 |
| 16 | 7 | Kubernetes & Cloud Fleet | 25 | 0 | 0% | Entire phase open; requires K8s operator (7.1) before rest | Phase 5, Phase 6 |
| 17 | 8 | Advanced Orchestration & AI-Native | 33 | 8 | 24% | 8.1.1–8.1.5 multi-agent; 8.2.4–8.2.5 prompt A/B + editor; 8.3.1–8.3.5 workflow engine; 8.4.1–8.4.5 code review; 8.5.6–8.5.8; 8.6.1–8.6.5 knowledge graph | Phase 6.1 (native loop), Phase 6.2 (R&D cycle) |
| 18 | 9 | R&D Cycle Automation | 5 | 5 | 100% | None | Phase 0.8 |
| 19 | 9.5 | Autonomous R&D Supervisor | 5 | 5 | 100% | None | Phase 9 |
| 20 | 10 | Claude Code Native Integration | 20 | 3 | 15% | 10.1–10.4 mostly open; 10.5.1–10.5.2 done, 10.5.3.4–10.5.3.5 open | Phase 9.5, Phase 2.75 |
| 21 | 10.5 | Horizontal & Vertical Scaling | 48 | 14 | 29% | sync.Map lock; NATS; multi-node; per-namespace rate limiting; SQLite WAL migration; worktree alternates | In progress, no hard external blocker |
| 22 | 11 | A2A Protocol Integration | 21 | 0 | 0% | Official Go SDK adoption (`a2aproject/a2a-go/v2`); path mismatch at `/.well-known/agent.json` | Phase 10.5 (scaling foundation) |
| 23 | 12 | Tailscale Fleet Networking | 26 | 0 | 0% | `tailscale.com/tsnet` embedding; `ts-enroll.sh` and service exist on disk | Phase 11 (A2A) |
| 24 | 13 | Level 3 Autonomy Core | 40 | 0 | 0% | All 40 tasks open; `self_heal.go`, `decision_engine.go`, `config_hotreload.go`, `unattended.go`, `param_tuner.go` do not yet exist | Phase 9.5, Phase 10.5 |
| 25 | 14 | Agent Memory & Meta-Learning | 31 | 0 | 0% | `internal/memory/` package does not exist | Phase 13 |
| 26 | 15 | Advanced Fleet Intelligence (v2) | 32 | 0 | 0% | `internal/fleet/swarm.go`, `internal/fleet/moe_router.go` do not exist | Phase 14 |
| 27 | 16 | Edge & Embedded Agents | 23 | 0 | 0% | Ollama/vLLM provider files do not exist; `internal/session/offline.go` missing | Independent of Phases 13–15 |
| 28 | 17 | AI Safety & Governance | 37 | 0 | 0% | `internal/safety/` has only anomaly + circuit breaker; guardrails, constitution, reward model, redteam all missing | Phase 13 |
| 29 | 18 | World Models & Predictive Systems | 28 | 0 | 0% | `internal/predict/` package does not exist | Phase 16 |
| 30 | 19 | Cross-Repository Orchestration | 29 | 0 | 0% | `internal/multirepo/` package does not exist | Independent (uses existing fleet package) |
| 31 | 20 | Agent Marketplace & Ecosystem | 43 | 0 | 0% | `internal/plugin/wasm*.go` missing; WASM runtime not yet embedded | Existing `internal/plugin/` provides foundation |
| 32 | 21 | Observability & Evaluation | 30 | 0 | 0% | OTel spans, Langfuse, SWE-bench, tau-bench not started | Phase 20 ecosystem integration |
| 33 | 22 | DevOps & Infrastructure Automation | 45 | 0 | 0% | `internal/devops/` package does not exist | Independent (CI/CD, security scan are standalone) |
| 34 | 23 | Advanced Prompt Engineering | 23 | 0 | 0% | `internal/rag/` package missing; compression, distillation, caching missing | Independent (builds on existing `internal/enhancer/`) |
| 35 | 24 | MoE-Inspired Provider Routing | 10 | 0 | 0% | `internal/fleet/moe_router.go` does not exist | Research-grounded; partially served by existing `cascade.go` + `bandit/` |
| 36 | 25 | Federated Fleet Learning | 9 | 0 | 0% | `internal/fedlearn/` package does not exist; requires Tailscale networking | Phase 12 (Tailscale), Phase 14 (memory) |

**Summary:** 10 phases complete (100%), 6 in-progress (15–96%), 20 fully planned (0%). Live task total: **503 done / 1,143 total (44.0%)**.

### What 08-ralph-deep-dive.md missed

1. Phase 3.5.5 now has **two conflicting sections** with the same number (see Section 3 below).
2. Phase 10.5 was understated at 14 done — the header "10.5" tasks use bullets without granular sub-IDs in several cases, making exact counts harder. The 14 figure from the prior analysis is used here unchanged.
3. Phases 22–25 task counts were not individually listed in the prior doc; they are computed here from the checkbox format.
4. `internal/fleet/coordinator.go` referenced in many 10.5 items **does not exist** — the coordinator logic appears split across `internal/fleet/worker.go`, `internal/fleet/queue.go`, and `internal/fleet/sharding.go`.

---

## 2. Orphaned Tasks

Tasks that reference files or packages that do not exist in the current codebase. Each finding verified via Glob or Bash.

### 2.1 Fully Missing Packages

| Phase | Task | Referenced File | Actual Status |
|-------|------|-----------------|---------------|
| 13.1 | SH-1…SH-10 | `internal/session/self_heal.go`, `watchdog.go`, `crash_recovery.go` | **MISSING** — none of these files exist |
| 13.2 | CA-1…CA-7 | `internal/session/config_hotreload.go`, `config_validator.go` | **MISSING** |
| 13.3 | AD-1…AD-8 | `internal/session/decision_engine.go`, `decision_policy.go`, `decision_journal.go` | **MISSING** (note: `decision_model.go` exists but is the data type, not the engine) |
| 13.4 | SO-1…SO-7 | `internal/session/self_optimize.go`, `param_tuner.go` | **MISSING** |
| 13.5 | UO-1…UO-8 | `internal/session/unattended.go`, `maintenance.go`, `sla.go` | **MISSING** |
| 14.x | All | `internal/memory/` (entire package) | **MISSING** — directory does not exist |
| 15.1–15.4 | All | `internal/fleet/swarm.go`, `stigmergy.go`, `consensus.go`, `moe_router.go`, `bandit_router.go`, `pareto.go`, `work_stealing.go`, `scheduler.go`, `dag.go`, `gantt.go` | **MISSING** |
| 16.1–16.3 | All | `internal/session/provider_ollama.go`, `provider_vllm.go`, `model_discovery.go`, `offline.go`, `request_queue.go`; `internal/fleet/edge.go`, `edge_monitor.go`, `mesh.go` | **MISSING** |
| 17.1–17.5 | All | `internal/safety/guardrails.go`, `allowlist.go`, `sanitizer.go`, `constitution.go`, `self_critique.go`, `reward_model.go`, `process_reward.go`, `redteam.go`, `chaos.go`, `injection_test.go`, `audit.go`, `lineage.go`, `model_card.go` | **MISSING** (only `anomaly.go`, `circuit_breaker.go`, `killswitch.go`, `anomaly_fleet.go` exist) |
| 18.x | All | `internal/predict/` (entire package) | **MISSING** |
| 19.x | All | `internal/multirepo/` (entire package) | **MISSING** |
| 20.1–20.6 | Plugin SDK, WASM | `internal/plugin/sdk.go`, `sandbox.go`, `wasm_runtime.go`, `wasm_capabilities.go`, `wasm_host.go`; `internal/plugin/wit/` | **MISSING** (marketplace.go and registry.go exist; wasm infrastructure absent) |
| 20.4 | Ecosystem | `internal/integrations/github_app.go`, `slack.go`, `grafana.go` | **MISSING** — no `internal/integrations/` package |
| 21.1–21.4 | All | `internal/telemetry/otel.go`, `llm_spans.go`, `cost_attribution.go`; `internal/eval/benchmark.go`, `harness.go`, `comparison.go`; `internal/eval/pass_k.go`, `task_class.go`, `test_grader.go`, `leaderboard.go` | **MISSING** (existing `internal/eval/` has bayesian A/B, anomaly, changepoint, counterfactual — not the benchmark harness these tasks need) |
| 22.x | All | `internal/devops/` (entire package) | **MISSING** |
| 22.7 | Profiling | `internal/profiler/pprof.go`, `pyroscope.go`, `pgo.go`; `internal/tui/views/profile_view.go` | **MISSING** |
| 23.1–23.3 | All | `internal/rag/hybrid.go`, `colbert.go`, `raptor.go`, `graphrag.go`; `internal/enhancer/compression.go`, `distillation.go`, `prompt_versioning.go`, `retrieval_gate.go`; `internal/session/semantic_cache.go`, `cache_hierarchy.go` | **MISSING** |
| 24.x | All | `internal/fleet/moe_router.go`, `task_classifier.go`, `capacity.go` (capacity.go exists — see below), `ensemble.go`, `decomposer.go`, `router_telemetry.go` | Most **MISSING**; `internal/fleet/capacity.go` actually exists — the file is present. `moe_router.go` is absent. |
| 25.x | All | `internal/fedlearn/` (entire package) | **MISSING** |

### 2.2 Partially Mismatched References

| Phase | Task | Reference | Reality |
|-------|------|-----------|---------|
| 3.1.1–3.1.3 | i3 IPC client | `internal/i3/` | **MISSING** — i3 items done in Phase 3.1 are actually in `internal/wm/` (detect, monitors, hyprland) with no dedicated `internal/i3/` directory |
| 3.2.2 | 7-monitor config | `distro/i3/workspaces.json` | **MISSING** — `distro/i3/config` exists but `workspaces.json` does not |
| 6.1.1–6.1.5 | Native loop engine | `internal/session/loop.go`, `loop_worker.go` | Exists — matches |
| 9.1.1–9.1.5 | Phase 9 tier-1 rdcycle tools | `internal/mcpserver/tools_loop.go`, `internal/session/{merge,cycle_plan,scheduler,baseline}.go` | **STALE REFERENCE** — current handlers are consolidated in `internal/mcpserver/handler_rdcycle.go` and registered in `tools_builders_misc.go` under `rdcycle` |
| 8.5.5 | Meta-agent / Supervisor | `internal/session/supervisor.go` | **EXISTS** — confirmed present |
| 10.5.1 | Lock splitting | `internal/session/manager.go` | Exists; `sync.Map` subtask still open |
| 10.5.4 | Worker pool | `internal/fleet/coordinator.go` | **MISSING** — referenced file does not exist; autoscaler logic lives in `internal/fleet/autoscaler.go` |
| 10.5.9 | SQLite WAL | `internal/store/sqlite.go` | **EXISTS** — `internal/store/sqlite.go` is present |
| 11.1 | A2A SDK adoption | `internal/fleet/a2a_card.go`, `a2a.go` | **EXISTS** — partial implementation confirmed |
| 11.2 | FleetExecutor | `internal/fleet/a2a_executor.go` | **EXISTS** — file present |
| 11.3 | A2A client dispatch | `internal/fleet/a2a_dispatch.go` | **EXISTS** — file present |
| QW-7 | Snapshot path | `internal/session/snapshot.go` | **MISSING** — marked `[x]` but file does not exist. Likely consolidated elsewhere. |

### 2.3 Completed Tasks with Missing Files (Stale `[x]`)

These tasks are checked `[x]` but the referenced files are absent, suggesting the implementation may have been moved or the acceptance criteria were met differently:

- **QW-7** (`internal/session/snapshot.go`) — marked done; file missing. Snapshot functionality may be in `internal/session/checkpoint.go` (which exists), but the file path in the task is wrong.
- **9.1.1–9.1.5** — the roadmap referenced a planned split across `tools_loop.go`, `merge.go`, `cycle_plan.go`, `scheduler.go`, and `baseline.go`, but the actual implementations were consolidated in `internal/mcpserver/handler_rdcycle.go`.
- **3.1.1–3.1.3** — marked `[x]` in the i3 IPC section but `internal/i3/` does not exist. The WM abstraction lives in `internal/wm/`.

---

## 3. Duplicated Tasks

### 3.1 Section 3.5.5 Collision (CRITICAL)

**The most significant duplication in the entire roadmap.** Phase 3.5 has two sections both numbered `3.5.5`:

**Instance A — line 809** (`### 3.5.5 — Codex-primary command/control parity [NEW]`):
- 8 subtasks (3.5.5.1–3.5.5.8), of which 4 are done and 4 are open
- These are high-priority P0/P1 tasks about provider-neutrality in the enhancer stack

**Instance B — line 825** (`### 3.5.5 — Theme export to terminal`):
- 4 subtasks (3.5.5.1–3.5.5.4), all marked `[x]` (done)
- These are P2 tasks about exporting themes to Ghostty/Starship/k9s

The two sections share identical sub-IDs (3.5.5.1, 3.5.5.2, etc.) for completely different tasks. This creates:
- Ambiguity in any tooling that addresses tasks by ID
- The older "theme export" section is complete; the newer "Codex parity" section is 50% done
- **Recommendation:** Renumber the Codex parity section to `3.5.6` (or higher), preserving the theme export section as `3.5.5`.

### 3.2 "Advanced Fleet Intelligence" Duplicated Phase Name

Phase 6 is named "Advanced Fleet Intelligence" (50 tasks, 84% complete).
Phase 15 is also named "Advanced Fleet Intelligence" (32 tasks, 0% complete).

These cover genuinely different scope (Phase 6: routing, analytics, coordination; Phase 15: swarm intelligence, Thompson sampling, distributed scheduling), but the identical name will cause confusion in communication and tooling.

- **Recommendation:** Rename Phase 15 to "Distributed Swarm Intelligence & Scheduling" to distinguish it.

### 3.3 Budget Tracking Overlap (Phase 2.3 vs Phase 5.5)

Phase 2.3 implements per-session budget tracking and auto-pause. Phase 5.5 implements "Budget federation" with a global pool, per-session limits, and a budget dashboard. The 5 items in 5.5 overlap conceptually with 2.3.4 (auto-pause) and 2.3.2 (global pool).

- Phase 2.3 is complete and ships the per-session tracking.
- Phase 5.5 extends it with federation across sandboxed sessions.
- **Recommendation:** Keep both; add a cross-reference noting 5.5 extends 2.3, not replaces it.

### 3.4 Notification System Overlap (Phase 2.6 vs Phase 6.5)

Phase 2.6 (`COMPLETE`) implements desktop notifications (D-Bus/osascript). Phase 6.5 (`COMPLETE`) implements external notifications (Discord, Slack, webhooks). These are distinct enough to keep separate, but both are now complete so the duplication is harmless.

### 3.5 Workflow Engine Overlap (Phase 2.75.3 vs Phase 8.3)

Phase 2.75.3 adds `ralphglasses_workflow_define` and `ralphglasses_workflow_run` MCP tools (complete). Phase 8.3 defines a full YAML workflow engine with executor and marketplace (0% done). Phase 8.3 is explicitly marked `[BLOCKED BY 6.1]` and frames itself as a deeper implementation on top of the Phase 2.75 MCP tools — this relationship is sound but not stated explicitly in either section.

- **Recommendation:** Add an explicit note to 8.3.1 stating it extends the Phase 2.75 workflow MCP tools.

### 3.6 Plugin System Overlap (Phase 2.13 vs Phase 3.5.2 vs Phase 20.1)

Three separate plugin system phases exist:
- Phase 2.13 — hashicorp/go-plugin gRPC interface (COMPLETE, `internal/plugin/`)
- Phase 3.5.2 — YAML-based `plugins.yml` keybind plugin system (COMPLETE)
- Phase 20.1 — Full plugin SDK with WASM sandboxing and marketplace (0% done)

The three address different abstraction levels and are not truly duplicated, but Phase 20.1 should explicitly state it supersedes the gRPC approach from 2.13 in its architecture overview.

---

## 4. Dependency Chain Analysis

### 4.1 Canonical Phase Dependency Map

```
Phase 0 ──────────────────────────────────────> Phase 0.5
                                                     |
                          Phase 0.9 (independent)    |
                                  |                  v
                             Phase 0.8 ---------> Phase 1 ---------> Phase 1.5
                                                     |                   |
                                                     v                   |
                                                 Phase 2 <--------------+
                                                     |
                                    +────────────────+────────────────+
                                    |                                 |
                                    v                                 v
                               Phase 2.5                         Phase 2.75
                                    |                                 |
                                    +────────────────+────────────────+
                                                     |
                                         +-----------+-----------+
                                         |                       |
                                         v                       v
                                     Phase 3               Phase 5
                                         |                       |
                              +----------+----------+            |
                              |                     |            |
                              v                     v            |
                          Phase 3.5             Phase 4          |
                                                                 v
                                                             Phase 6
                                                                 |
                                              +------------------+------------------+
                                              |                  |                  |
                                              v                  v                  v
                                          Phase 7            Phase 8           Phase 9.5
                                                                 |                  |
                                                                 v                  v
                                                          Phase 10 <--------> Phase 10.5
                                                                                    |
                                                                    +───────────────+───────────────+
                                                                    |               |               |
                                                                    v               v               v
                                                                Phase 11        Phase 13        Phase 12
                                                                    |               |               |
                                                                    +───────────────+               |
                                                                                    |               v
                                                                                    v           Phase 25
                                                                                Phase 14
                                                                                    |
                                                                    +───────────────+───────────────+
                                                                    |               |               |
                                                                    v               v               v
                                                                Phase 15        Phase 17        Phase 16
                                                                                                    |
                                                                                                    v
                                                                                                Phase 18

Phases 19, 22, 23: Independent of the main chain
Phase 20: Requires plugin foundation only (partial)
Phase 21: Requires Phase 20 ecosystem integration
Phase 24: Research-grounded, can start from Phase 6.6 (model routing)
```

### 4.2 Critical Path to Level 3 Autonomy

The stated goal is L3: 72-hour unattended operation. The minimum path from current state:

```
10.5 complete (sync.Map, NATS, SQLite WAL, multi-node)   [29% done, ~34 tasks remaining]
    |
    v
13.1 Self-Healing Runtime (SH-1..SH-10, XL)              [0%, 10 tasks — 0 exist on disk]
    |
    v
13.3 Autonomous Decision Engine (AD-1..AD-8, XL)         [0%, 8 tasks]
    |
    v
13.5 Unattended Operation Mode (UO-1..UO-8, L)           [0%, 8 tasks]
    |
    v
17.1 Safety Boundaries & Guardrails (SB-1..SB-10, XL)   [0%, 10 tasks]
    |
    v
14.1 Persistent Agent Memory (PM-1..PM-10, XL)           [0%, 10 tasks]
    |
    v
15.1 Intelligent Task Router (TR-1..TR-10, XL)           [0%, 10 tasks]
    |
    v
L3 OPERATIONAL
```

**Critical path task count: ~100 tasks, all L or XL sized, all currently at 0%.**

The bottleneck is Phase 13.1 (Self-Healing Runtime). Without heartbeat-based session health monitoring, circuit breakers per provider, and crash recovery, unattended operation is unsafe. Phase 13.1 has no files on disk — it must be built from scratch.

The roadmap's own updated dependency chain (line 2676) confirms:
> "13.1 (self-heal) → 13.3 (decision engine) → 13.5 (unattended) → 17.1 (safety) → 14.1 (memory) → 15.1 (router)"

### 4.3 Parallelizable Work (Independent of L3 chain)

These phases can start now without waiting for the L3 path:

| Phase | Why Independent | Estimated Tasks |
|-------|-----------------|-----------------|
| 19 | Only needs existing fleet package | 29 open tasks |
| 22 | CI/CD, security scanning, DevOps are standalone | 45 open tasks |
| 23 | Builds on existing `internal/enhancer/` | 23 open tasks |
| 16.1 | Ollama/vLLM are provider additions | ~10 tasks |
| 11 | A2A partial impl exists; SDK adoption independent | 21 tasks |
| 24 | Extends existing `cascade.go` + bandit | 10 tasks |

Total parallelizable backlog: **~138 tasks** that can proceed without L3.

---

## 5. Task Size Distribution

Task sizes are labeled `S`, `M`, `L`, `XL` in the roadmap. Phases 13–25 use implicit sizing from section descriptions. The distribution below covers all phases with explicit labels.

### 5.1 Phases with Explicit Size Labels

| Phase | S | M | L | XL | Total Labeled | XL% | Assessment |
|-------|---|---|---|----|---------------|-----|------------|
| 0.9 QW | 9 | 3 | 0 | 0 | 12 | 0% | Healthy |
| 0.5 | 14 | 19 | 2 | 0 | 35 | 0% | Healthy |
| 0.6 | 4 | 14 | 2 | 0 | 20 | 0% | Healthy |
| 1 | 8 | 18 | 8 | 0 | 34 | 0% | Manageable |
| 1.5 | 14 | 20 | 5 | 3 | 42 | 7% | Note: 1.5.10 + 1.5.11 are XL blockers |
| 2 | 9 | 30 | 8 | 0 | 47 | 0% | Healthy |
| 2.5 | 4 | 8 | 4 | 0 | 16 | 0% | Healthy |
| 3 | 6 | 16 | 6 | 0 | 28 | 0% | Healthy |
| 3.5 | 6 | 10 | 2 | 0 | 18 | 0% | Healthy |
| 4 | 7 | 16 | 7 | 0 | 30 | 0% | Manageable |
| 5 | 4 | 14 | 10 | 0 | 28 | 0% | L-heavy (sandboxing is complex) |
| 6 | 5 | 12 | 10 | 0 | 27 | 0% | L-heavy |
| 7 | 2 | 10 | 8 | 1 | 21 | 5% | Manageable |
| 8 | 0 | 8 | 12 | 2 | 22 | 9% | L+XL dominant — risky scope |
| 9 | 2 | 4 | 3 | 1 | 10 | 10% | Manageable |
| 10 | 4 | 8 | 2 | 0 | 14 | 0% | Healthy |
| 10.5 | 0 | 2 | 7 | 2 | 11 | 18% | **L/XL dominant** |

### 5.2 Phases with Implicit Sizing (13–25)

These phases use named items (SH-1, AD-3, etc.) without explicit size labels. However, all sections with `XL` in their header can be estimated:

| Phase | XL Sections | Assessment |
|-------|-------------|------------|
| 13 | 13.1, 13.3 (both XL in header); 13.5 (L) | **3 XL/L P0 items blocking L3** |
| 14 | 14.1 (XL in header) | 1 XL P0 prerequisite |
| 15 | 15.1 (XL in header) | 1 XL P0 prerequisite |
| 16 | 16.1 (XL in header) | Independent XL |
| 17 | 17.1 (XL in header) | XL safety gate required before L3 |
| 18 | 18.1 (XL), 18.4 (XL) | Research-phase XL |
| 19 | 19.1 (XL) | Independent, could start now |
| 20 | 20.1 (XL), 20.6 (XL) | WASM XL work |
| 21 | 21.4 (XL) | Evaluation framework XL |
| 22 | No XL headers | Most manageable late phase |
| 23 | 23.3 (XL) | RAG XL |
| 24 | 24.1 (XL) | MoE routing XL |
| 25 | 25.1 (XL) | Federated learning XL |

### 5.3 Phases with Unrealistic Scope

**Phase 13 (Level 3 Autonomy Core)** — 40 tasks, 0% done, three XL sections, all files missing from disk. The L3 target (72-hour unattended operation) requires SH-10 + AD-8 + UO-8 + SO-7 to all work correctly together before any is proven. This is the highest-risk scope in the roadmap.

**Phase 10.5 (Horizontal & Vertical Scaling)** — 18% XL fraction, all blocking L3. The multi-node marathon distribution (10.5.6) and autonomy scaling (10.5.11) are both `XL` items with external infrastructure dependencies (NATS JetStream, multi-machine coordination).

**Phase 17 (AI Safety & Governance)** — 37 tasks with Phase 17.1 (`XL`) as the gate. The constitutional AI and process reward model sections (17.2, 17.3) require fine-tuning infrastructure that is not currently part of the stack at all. Phases 17.3–17.4 (PRMs, adversarial testing) are research-grade work that should realistically be deferred after L3 is stable.

**Phases 24–25 (MoE Routing, Federated Learning)** — Both are research-grounded phases tagged with XL items. Phase 25 (Federated Learning with DP-SGD and Shamir secret sharing) is the most academically ambitious item in the entire roadmap and has no realistic path to production before all other phases are stable.

---

## 6. Stale Completed Tasks

Tasks marked `[x]` whose acceptance criteria are suspect, referencing files that have moved or no longer exist as described.

### 6.1 QW-7 — Snapshot path fix

**Task:** Fix snapshot path saving to claudekit path (FINDING-148/268)
**File referenced:** `internal/session/snapshot.go`
**Verification:** `internal/session/snapshot.go` does not exist. `internal/session/checkpoint.go` does exist and likely contains this functionality.
**Status:** Acceptance criterion ("Snapshots save to `.ralph/snapshots/`") may be met, but the file listed in the task is gone. The task description is stale.

### 6.2 Phase 3.1.1–3.1.3 — i3 IPC client

**Tasks:** Create `internal/i3/` package (3.1.1), Workspace CRUD (3.1.2), Window management (3.1.3)
**File referenced:** `internal/i3/`
**Verification:** No `internal/i3/` directory exists. `internal/wm/` exists with `detect.go`, `monitors.go`, `hyprland.go`, and `internal/wm/sway/client.go`. The i3 functionality may be subsumed under the WM abstraction layer.
**Status:** Marked `[x]` but the implementation home is different from what was specified. The WM abstraction (`internal/wm/`) may satisfy the acceptance criterion ("programmatic workspace creation from Go") but the path reference is wrong.

### 6.3 Phase 9 file-reference drift, not missing handlers

**Phase 9 status:** The earlier research correctly noticed that the roadmap referenced files that do not exist, but the conclusion was wrong. Tier-1 Phase 9 tools are implemented today as consolidated handlers in `internal/mcpserver/handler_rdcycle.go`, with tool registration in `tools_builders_misc.go`.

The actual drift is documentation shape: the roadmap still points at a planned file split (`tools_loop.go`, `merge.go`, `cycle_plan.go`, `scheduler.go`, `baseline.go`) that never landed. The implementation stayed consolidated instead. Phase 9 should not be reopened on missing-file grounds; the real follow-up is keeping roadmap/docs aligned and continuing behavior coverage for these handlers.

`tools_loop_test.go` also does not prove missing handlers. It exercises the legacy loop lifecycle (`handleLoopStart`, `handleLoopStep`, `handleLoopStatus`, `handleLoopStop`), not the tier-1 rdcycle handler surface.

**Risk:** Future automation will keep re-opening already-implemented work unless roadmap/docs and generated research artifacts point at the consolidated handler location.

### 6.4 Phase 1.5.2–1.5.9 — Release automation and docs

**Tasks:** GoReleaser config (1.5.2.1), GitHub Actions release (1.5.2.2), flake.nix (1.5.7.1–1.5.7.3), devcontainer (1.5.8.x), docs site (1.5.9.x) — all `[x]`
**Verification:**
- `.goreleaser.yaml` — **EXISTS**
- `flake.nix` — **EXISTS**
- `.devcontainer/devcontainer.json` — **EXISTS**
**Status:** These appear legitimately done. The acceptance criteria for docs site (1.5.9.1: "docs site live at `hairglasses-studio.github.io/ralphglasses`") cannot be verified locally, but the task is likely still aspirational — marked done for the config files rather than the live deploy.

### 6.5 Phase 2.75.3 — Workflow tools

**Tasks:** `ralphglasses_workflow_define` and `ralphglasses_workflow_run` MCP tools (marked `[x]`)
**Verification:** `tools_builders_misc.go` and `tools_builders.go` exist. No dedicated `tools_workflow.go` file. The workflow tools exist in the builder dispatch system.
**Status:** Likely valid — the tools are registered. However, Phase 8.3 (workflow engine) is still 0% done, suggesting the tools exist as stubs without a real executor backend.

### 6.6 Phase 6.2 — R&D cycle orchestrator

**Tasks:** 6.2.1–6.2.5 all `[x]`. Specifically 6.2.3 "Regression detection" and 6.2.4 "Auto-generate improvement tasks."
**Verification:** `internal/session/loopbench.go`, `loopbench_analyze.go`, `loopbench_report.go` exist.
**Concern:** The KPI table shows zero-change iteration rate at 95.6% — meaning 22 of 23 passed iterations changed zero files. If 6.2.4 ("auto-generate improvement tasks from benchmark regressions") were working correctly, this rate should be much lower.
**Status:** The code exists but the acceptance criteria ("automated benchmark -> task generation cycle runs unattended") appears to not be producing meaningful work, as evidenced by the KPI data.

---

## Cross-Cutting Findings

### Finding A: The Coordinator File Phantom

The ROADMAP.md references `internal/fleet/coordinator.go` in tasks 10.5.1, 10.5.4, 10.5.6 (the bottleneck analysis table). This file **does not exist**. The coordinator logic is split across:
- `internal/fleet/worker.go` (worker lifecycle)
- `internal/fleet/queue.go` (task queuing)
- `internal/fleet/autoscaler.go` (scaling)
- `internal/fleet/sharding.go` (repo sharding)

Any task that specifies `coordinator.go` as the implementation file will need a path correction before work begins.

### Finding B: `tools_loop_test.go` does not imply missing tier-1 handlers

`internal/mcpserver/tools_loop_test.go` exists but covers the loop lifecycle handlers, not the tier-1 rdcycle tools. The Tier 1 Phase 9 handlers (`finding_to_task`, `cycle_merge`, `cycle_plan`, `cycle_schedule`, `cycle_baseline`) are implemented in `internal/mcpserver/handler_rdcycle.go`; the stale part was the roadmap's planned file split, not a missing implementation.

### Finding C: 3.5.5 Numbering Collision Creates Roadmap Debt

The double-numbered `3.5.5` section (Theme Export vs. Codex Parity) means any automated roadmap tooling that addresses tasks by ID — including the `ralphglasses_roadmap_*` MCP tools — will have undefined behavior on Phase 3.5 tasks. The Codex-parity tasks (P0/P1) are higher priority and should be renumbered to 3.5.6–3.5.9 immediately to avoid confusion.

### Finding D: `internal/safety/` is Underbuilt Relative to Phase 17 Scope

Phase 17 requires 37 safety tasks across 5 subsections. The current `internal/safety/` package has 4 files: anomaly detection, circuit breaker, kill switch, and fleet anomaly. The guardrails, constitutional AI, process reward models, and adversarial testing subsystems (17.1–17.4) are entirely absent. Phase 17 is a prerequisite for L3 autonomy and cannot be skipped.

### Finding E: Discrepancy Between Header Task Count and Live Count

The ROADMAP.md header reads "1,115 tasks, 442 complete." Live grep counts 1,143 checkboxes (503 done). The header is 28 tasks understated and overstates completion by 61 tasks. This matters for KPI tracking — the "finding resolution rate" KPI (9.8%) is computed against the header count. With correct totals, completion is 44.0%, not 39.6%.

---

## Recommended Immediate Actions

1. **Renumber Phase 3.5.5 (Codex parity)** to 3.5.6 through 3.5.9 — eliminates numbering collision.
2. **Align Phase 9 roadmap/docs to `internal/mcpserver/handler_rdcycle.go`** — keep future handler extraction explicit instead of treating the planned split as missing implementation.
3. **Fix `coordinator.go` path references** in Phase 10.5 tasks — update to reference `autoscaler.go`, `queue.go`, `sharding.go` as appropriate.
4. **Update ROADMAP.md header** task count from "1,115 / 442" to "1,143 / 503."
5. **Create `internal/session/self_heal.go`** stub — Phase 13.1 is the critical path bottleneck and has zero on-disk presence.
6. **Address the 5 Sprint 7 BLOCKER path traversal vulnerabilities** before any public exposure of the MCP server. These are S/M fixes using existing `ValidatePath` functions.
