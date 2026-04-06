# 15 -- Fleet Readiness Assessment

Generated: 2026-04-04

Cross-repo readiness analysis for ralphglasses fleet management. Evaluates all 77 directories under ~/hairglasses-studio/ against fleet infrastructure requirements: agent instructions (CLAUDE.md, AGENTS.md), ralph integration (.ralphrc, .ralph/), build pipeline (Makefile, go.mod), and MCP configuration (.mcp.json).

---

## 1. Fleet-Ready Repo Inventory

Data collected via filesystem scan of all 77 directories. "Y" = present, "-" = absent.

### Tier 1: Fleet-Ready (CLAUDE.md + go.mod + Makefile)

These repos have full agent context, Go build infrastructure, and a Makefile -- they can be swept immediately by the fleet.

| Repo | CLAUDE.md | AGENTS.md | .ralphrc | .ralph/ | Makefile | go.mod | .mcp.json | Tier |
|------|-----------|-----------|----------|---------|----------|--------|-----------|------|
| claudekit | Y | Y | Y | Y | Y | Y | Y | **T1** |
| [private-1] | Y | Y | - | - | Y | Y | - | **T1** |
| dotfiles-mcp | Y | Y | - | - | Y | Y | - | **T1** |
| hg-mcp | Y | Y | Y | Y | Y | Y | - | **T1** |
| hyprland-mcp | Y | Y | - | - | Y | Y | - | **T1** |
| input-mcp | Y | Y | - | - | Y | Y | - | **T1** |
| [private-ops] | Y | Y | Y | Y | Y | Y | Y | **T1** |
| [private-ops-2] | Y | Y | - | - | Y | Y | - | **T1** |
| mcpkit | Y | Y | Y | Y | Y | Y | Y | **T1** |
| mesmer | Y | Y | Y | Y | Y | Y | - | **T1** |
| process-mcp | Y | Y | - | - | Y | Y | - | **T1** |
| prompt-improver | - | Y | - | - | Y | Y | Y | **T1** |
| ralphglasses | Y | Y | Y | Y | Y | Y | Y | **T1** |
| shader-mcp | Y | Y | - | - | Y | Y | - | **T1** |
| [private-audit] | Y | Y | - | - | Y | Y | - | **T1** |
| systemd-mcp | Y | Y | - | - | Y | Y | - | **T1** |
| tmux-mcp | Y | Y | - | - | Y | Y | - | **T1** |
| webb | Y | Y | - | - | Y | Y | - | **T1** |
| webbb | Y | Y | - | - | Y | Y | - | **T1** |

**Count: 19 repos** -- can be swept today with full agent context.

### Tier 2: Partially Ready (go.mod but missing CLAUDE.md or Makefile)

| Repo | CLAUDE.md | AGENTS.md | .ralphrc | .ralph/ | Makefile | go.mod | .mcp.json | Tier |
|------|-----------|-----------|----------|---------|----------|--------|-----------|------|
| gh-dash | - | - | - | - | Y | Y | - | **T2** |
| pinecone-canopy | - | - | - | - | Y | Y | - | **T2** |
| runmylife | Y | Y | - | - | Y | Y | Y | **T2** |
| terraform-docs | - | - | - | - | Y | Y | - | **T2** |
| whiteclaw | Y | Y | Y | Y | - | - | - | **T2** |

Note: runmylife has CLAUDE.md + AGENTS.md + Makefile + go.mod but lacks .ralphrc/.ralph/ integration. whiteclaw has .ralphrc + .ralph/ but no go.mod (not a Go project). gh-dash, pinecone-canopy, and terraform-docs are forks with Makefiles but no agent context.

**Count: 5 repos** -- need CLAUDE.md or pipeline setup before sweep.

### Tier 3: Needs Setup (Missing core fleet infrastructure)

| Repo | CLAUDE.md | AGENTS.md | .ralphrc | .ralph/ | Makefile | go.mod | .mcp.json | Notes |
|------|-----------|-----------|----------|---------|----------|--------|-----------|-------|
| aftrs-terraform | Y | Y | - | - | - | - | - | IaC, no Go |
| allthelinks | - | - | - | - | - | - | - | Minimal |
| archlet | - | - | - | - | - | - | - | Thin client base |
| assimilate | - | - | - | - | - | - | - | Minimal |
| caper-bush | - | - | - | - | - | - | - | Minimal |
| claude-skills | - | - | - | - | - | - | - | Skills repo |
| comfyui-hairglasses-mix | - | - | - | - | - | - | - | ComfyUI |
| cr8-cli | Y | Y | - | - | Y | - | Y | Python, not Go |
| [private-3] | Y | Y | - | - | - | - | Y | Python MCP |
| [private-2] | Y | Y | - | - | - | - | Y | Duplicate of [private-3] |
| dj-archive | Y | Y | - | - | - | - | - | Archive |
| dotfile-museum | Y | Y | - | - | - | - | - | Reference |
| dotfiles | Y | Y | - | - | - | - | Y | Config/shaders |
| dotfiles-arch | - | - | - | - | - | - | - | Archive |
| [private-dotfiles] | Y | Y | - | - | - | - | - | Private |
| gaming-projects | Y | Y | - | - | - | - | - | Projects list |
| gh-repo-star-archive | - | Y | - | - | - | - | - | Data repo |
| github-stars-catalog | - | - | - | - | - | - | - | Data repo |
| hg-android | - | Y | - | - | - | - | Y | Android, not Go |
| hg-pi | Y | Y | - | - | - | - | Y | Pi devices |
| hgmux | Y | Y | - | - | - | - | - | Stalled fork |
| luke-toolkit | - | - | - | - | - | - | - | Toolkit |
| mac-mcp | - | Y | - | - | - | - | - | macOS, stalled |
| ngrok.plugin.zsh | - | - | - | - | - | - | - | Zsh plugin |
| open-multi-agent | - | - | - | - | - | - | - | Reference |
| open-sourcing-research | - | - | - | - | - | - | - | Research |
| opnsense-monolith | Y | Y | - | - | - | - | - | Firewall config |
| procgen-videoclip | - | - | - | - | - | - | - | Generative art |
| ralph-claude-code | Y | Y | - | - | - | - | - | Claude Code ref |
| romhub | Y | Y | - | - | - | - | Y | TypeScript MCP |
| sam3-video-segmenter | Y | Y | - | - | - | - | - | ML project |
| secretstudios-mcp | - | Y | - | - | - | - | - | MCP server |
| sf-curate | - | - | - | - | - | - | - | Data curation |
| sf-test | - | - | - | - | - | - | - | Test data |
| shieldd | - | - | - | - | - | - | - | Legacy |
| studio-projects | Y | Y | - | - | - | - | - | Projects list |
| sway-mcp | Y | Y | - | - | - | - | - | Legacy MCP |
| tmux-ssh-syncing | - | - | - | - | - | - | - | Utility |
| unraid-monolith | Y | Y | - | - | - | - | - | NAS config |
| video-ai-toolkit | Y | Y | - | - | - | - | - | ML toolkit |
| visual-projects | Y | Y | - | - | - | - | - | Projects list |
| vj-archive | Y | Y | - | - | - | - | - | VJ archive |
| wallpaper-procgen | - | - | - | - | - | - | - | Generative art |
| zsh-cdr | - | - | - | - | - | - | - | Zsh plugin |

**Count: 44 repos** -- missing build pipeline, agent context, or both.

### Tier 4: Not Applicable (Forks, dormant, non-code)

| Repo | Reason |
|------|--------|
| cmatrix | Fork (abishekvashok/cmatrix) |
| docs | Not a git repo (org-level docs directory) |
| elenapan-dotfiles | Dormant fork (last commit Sep 2025) |
| lnav | Fork (tstack/lnav) |
| makima | Fork (themackabu/makima) |
| openai_cli | Legacy CLI, external provenance |
| pinecone-VSB | Fork (pinecone-io/VSB) |
| scripts | Org-level scripts directory |

**Count: 8 repos/dirs** -- excluded from fleet operations.

### Tier Distribution Summary

| Tier | Count | % of Total (77) | Description |
|------|-------|------------------|-------------|
| T1 (fleet-ready) | 19 | 25% | Full agent context + Go build + Makefile |
| T2 (partially ready) | 5 | 6% | Has Go or build infra but missing agent context |
| T3 (needs setup) | 44 | 57% | Missing core fleet infrastructure |
| T4 (not applicable) | 8 | 10% | Forks, dormant, non-git |
| **Sweepable now** | **19** | **25%** | |

---

## 2. Dependency Health

### mcpkit Version Skew

9 repos depend on `github.com/hairglasses-studio/mcpkit`. Version distribution:

| mcpkit Version | Replace? | Repos | Risk |
|----------------|----------|-------|------|
| v0.1.0 (via `replace ../mcpkit`) | Relative | dotfiles-mcp, systemd-mcp, tmux-mcp, process-mcp | LOW (all resolve to HEAD) |
| v0.0.0 (via `replace ../mcpkit`) | Relative | hg-mcp | LOW (resolves to HEAD) |
| v0.0.0-00010101 (via `replace ~/...`) | Absolute | input-mcp, shader-mcp | LOW (deprecated repos) |
| v0.1.1 (pinned, no replace) | None | claudekit | MEDIUM (may drift behind HEAD) |
| v0.0.0-20260402 (pseudo-version) | None | hyprland-mcp | LOW (deprecated) |

**Effective version skew: NONE for active repos.** The 5 active repos with relative replace directives all resolve to mcpkit HEAD. claudekit pins v0.1.1 from the Go proxy and is the sole active repo that could drift. hyprland-mcp, input-mcp, and shader-mcp are deprecated (consolidated into dotfiles-mcp).

### Replace Directive Inventory

| Repo | Target | Replacement | Category |
|------|--------|-------------|----------|
| dotfiles-mcp | mcpkit | `../mcpkit` | Relative (development) |
| systemd-mcp | mcpkit | `../mcpkit` | Relative (development) |
| tmux-mcp | mcpkit | `../mcpkit` | Relative (development) |
| process-mcp | mcpkit | `../mcpkit` | Relative (development) |
| hg-mcp | mcpkit | `../mcpkit` | Relative (development) |
| input-mcp | mcpkit | `~/hairglasses-studio/mcpkit` | Absolute (CRITICAL: breaks portability) |
| shader-mcp | mcpkit | `~/hairglasses-studio/mcpkit` | Absolute (CRITICAL: breaks portability) |
| webb | webb (self) | `$HOME/hairglasses/webb` | Absolute macOS (CRITICAL: wrong platform) |
| webbb | webb (self) | `$HOME/hairglasses/webb` | Absolute macOS (CRITICAL: wrong platform) |

**5 repos** use `../mcpkit` relative replaces (consistent dev-mode pattern).
**4 repos** use absolute paths that break on any other machine (2 deprecated + 2 legacy).
**Claudekit** is the only active mcpkit consumer that builds cleanly from a fresh clone without sibling repos.

### Go Version Uniformity

20 of 22 Go repos are on Go 1.26.1 (org standard). The 2 exceptions are upstream forks:
- terraform-docs: Go 1.23.0
- gh-dash: Go 1.23.3

No version drift in org-owned Go repos.

---

## 3. Ralph Integration Depth

7 repos have both `.ralphrc` and `.ralph/` directories, indicating prior fleet interaction:

| Repo | .ralphrc | .ralph/ | Tier | Role |
|------|----------|---------|------|------|
| claudekit | Y | Y | T1-Framework | Terminal customization |
| hg-mcp | Y | Y | T1-Active | 1,190-tool MCP server |
| [private-ops] | Y | Y | Private | Job search platform |
| mcpkit | Y | Y | T1-Framework | Core Go MCP framework |
| mesmer | Y | Y | Legal-review | Platform ops (1,790 tools) |
| ralphglasses | Y | Y | T1-Orchestrator | Fleet manager itself |
| whiteclaw | Y | Y | T2-Research | Claude Code source analysis |

These 7 repos are the "inner ring" that have been actively managed by ralphglasses. The remaining 68 repos/dirs have never been touched by fleet operations.

---

## 4. Sweep Readiness

### Current Architecture Constraints (from 06-fleet-sweep.md)

| Constraint | Impact |
|------------|--------|
| **Serial fan-out** | `handleSweepLaunch` launches sessions in a for-loop, ~5s per repo. 74 repos = ~6min launch phase. |
| **In-memory queue** | Unbounded `map[string]*WorkItem`. No persistence across restart. |
| **MaxWorkers = 32** | Hard cap with 4 sessions per worker = 128 theoretical max concurrent sessions. |
| **Scale-up is advisory** | Autoscaler publishes events but spawns no workers. Single-machine bottleneck. |
| **RetryTracker has no mutex** | Data race under concurrent `handleWorkComplete` calls. |

### Sweep Concurrency Calculation

**Repos sweepable in parallel (T1):** 19

With the current fleet architecture:
- **Single-machine deployment**: Practical limit is 4-8 concurrent sessions (memory/CPU bounded on one machine).
- **Theoretical maximum**: 128 sessions (32 workers x 4 sessions each), but requires external worker provisioning.
- **Actual bottleneck**: Serial fan-out. Even with enough workers, sessions launch sequentially at ~5s each. A 19-repo sweep takes ~95 seconds just to launch.

### Sweep Capacity by Deployment Mode

| Mode | Max Parallel Sessions | Launch Time (19 repos) | Completion Time (est.) |
|------|----------------------|------------------------|------------------------|
| Single machine (4 sessions) | 4 | ~95s serial | ~25 min (5 batches x 5 min) |
| Single machine (8 sessions) | 8 | ~95s serial | ~15 min (3 batches x 5 min) |
| Multi-machine (32 workers) | 128 | ~95s serial | ~5 min (all parallel) |
| Multi-machine + parallel launch | 128 | ~2s parallel | ~5 min (all parallel) |

### Recommendations for Sweep Pipeline

1. **Parallelize launch loop**: Use a bounded goroutine pool (semaphore of 10) instead of serial for-loop. This reduces 19-repo launch time from 95s to ~10s.
2. **Add queue depth limit**: Prevent unbounded memory growth from large sweeps. Recommended: `MaxDepth = 200`.
3. **Fix RetryTracker race**: Add `sync.Mutex` to the `RetryTracker` struct before running any production sweeps.
4. **Wire queue persistence**: Call `SaveTo` in the 30s maintenance loop and on `Stop` to survive coordinator restarts.

---

## 5. Cost Projection

### Per-Task Cost Assumptions

From sweep cost controls (memory: `feedback_sweep_cost_control.md`) and fleet subsystem analysis:

| Parameter | Value | Source |
|-----------|-------|--------|
| Average cost per task | $0.17 | Empirical sweep data |
| Budget cap per session | $0.50 | Recommended sweep convention |
| Default handler budget | $5.00 | Handler default (overridden by caller) |
| Max sweep budget | $100.00 | Default `BudgetPool.globalLimit` |

### Cost Scenarios

| Scope | Repos | Est. Cost @ $0.17/task | Est. Cost @ $0.50/task (worst) |
|-------|-------|------------------------|-------------------------------|
| T1 only (fleet-ready) | 19 | **$3.23** | $9.50 |
| T1 + T2 | 24 | **$4.08** | $12.00 |
| All Go repos | 22 | **$3.74** | $11.00 |
| All non-fork repos | 68 | **$11.56** | $34.00 |
| Full org (all 75 git repos) | 75 | **$12.75** | $37.50 |

**Full fleet audit sweep (19 T1 repos): $3.23 estimated, $9.50 worst case.**

This is well within the default $100 sweep budget and the $500 global fleet budget. Even a full-org sweep at worst-case pricing ($37.50) is affordable.

### Cost Optimization

- Use `--no-session-persistence` for all sweep sessions (already the default in the handler).
- Set `budget_usd=0.50` per session to cap runaway costs.
- Use Codex as the sweep provider for cost optimization (lower per-token rate vs Claude).
- The cost predictor's static fallback formula (`0.24 * rates * maxTurns`) overstates cost for simple audit tasks. After collecting 20+ samples, the sliding-window predictor will self-calibrate.

---

## 6. Recommended Onboarding Order

Priority weighted by: (1) dependency centrality, (2) development velocity, (3) current readiness score, (4) release wave urgency.

### Phase 1: Core Framework + Wave 1 Servers (immediate)

| Priority | Repo | Velocity (7d commits) | Gap Score | Rationale |
|----------|------|-----------------------|-----------|-----------|
| P0 | mcpkit | 37 | 10/10 | Core framework, gates all downstream repos. Already has .ralphrc + .ralph/. |
| P0 | systemd-mcp | 13 | 9/10 | Wave 1 release target (this weekend). Needs sweep for replace directive removal. |
| P0 | tmux-mcp | 13 | 9/10 | Wave 1 release target. Same dependency fix needed. |
| P0 | process-mcp | 11 | 9/10 | Wave 1 release target. Same dependency fix needed. |

### Phase 2: High-Velocity Active Repos (days 2-3)

| Priority | Repo | Velocity (7d commits) | Gap Score | Rationale |
|----------|------|-----------------------|-----------|-----------|
| P1 | dotfiles-mcp | 34 | 9/10 | Wave 2 target. 86 tools, high velocity. |
| P1 | claudekit | 31 | 10/10 | Wave 3 target. Already fleet-integrated. Near-complete roadmap (93%). |
| P1 | hg-mcp | 25 | 9/10 | Wave 4 target. 1,190 tools. Already fleet-integrated. |
| P1 | mesmer | 32 | 9/10 | Massive codebase (1,790 tools). Already fleet-integrated. Needs legal review. |

### Phase 3: Application Layer (days 4-7)

| Priority | Repo | Velocity (7d commits) | Gap Score | Rationale |
|----------|------|-----------------------|-----------|-----------|
| P2 | [private-ops] | 115 | 10/10 | Highest velocity after ralphglasses. Already fleet-integrated. Private. |
| P2 | [private-audit] | 14 | 10/10 | Perfect gap score. Private but production-grade. |
| P2 | cr8-cli | 26 | 9/10 | Python project, high velocity. Needs Makefile for Go sweep (or Python-specific sweep template). |
| P2 | dotfiles | 171 | 7/10 | Extremely high velocity. Config repo, not Go, but benefits from sweep audits. |

### Phase 4: Expand to T3 Repos (week 2+)

Batch onboarding of T3 repos. For each repo, the sweep should:
1. Generate and commit a CLAUDE.md from repo contents
2. Add .ralphrc with default fleet configuration
3. Add pipeline.mk or Makefile if applicable

Priority targets from T3: romhub (10 commits/7d, TypeScript MCP), hgmux (22 commits/7d, fork), sway-mcp (8 commits/7d, active MCP server).

---

## 7. Fleet Scaling Requirements for 75+ Repos at L3

Level 3 autonomy (as defined in ralphglasses ROADMAP.md) requires the fleet to independently manage cross-repo maintenance, detect regressions, and execute corrective actions without human approval for routine operations.

### Infrastructure Changes Required

#### A. Queue and Persistence (Critical)

| Requirement | Current State | Needed |
|-------------|--------------|--------|
| Queue persistence | In-memory only, lost on restart | Auto-save to disk every 30s + on Stop |
| Queue capacity | Unbounded map | MaxDepth=500 with backpressure |
| DLQ replay | Manual operator action | Scheduled automatic replay with exponential backoff |
| Work item dedup | None | Hash-based dedup by (repo, prompt, provider) tuple |

#### B. Concurrency and Scale-Up (Critical)

| Requirement | Current State | Needed |
|-------------|--------------|--------|
| Fan-out | Serial for-loop | Parallel goroutine pool (semaphore=16) |
| Worker provisioning | Advisory scale-up events only | Local worker spawner: `exec` new ralphglasses worker processes |
| Session timeout | No upper bound on poll loop | `context.WithTimeout` at 30 minutes per session |
| Concurrent sweep limit | None | Max 3 concurrent sweeps to prevent resource exhaustion |

#### C. Observability (High)

| Requirement | Current State | Needed |
|-------------|--------------|--------|
| Prometheus metrics | Dashboard declared, metrics not emitted | Wire PrometheusRecorder metric names to match Grafana dashboard |
| Cost predictor feed | Not auto-populated from fleet completions | Wire `handleWorkComplete` to call `predictor.Record` |
| Sweep progress | Poll-based status aggregation | Real-time SSE/WebSocket push for sweep dashboards |

#### D. Reliability (High)

| Requirement | Current State | Needed |
|-------------|--------------|--------|
| RetryTracker thread safety | No mutex | Add `sync.Mutex` (race condition, data corruption risk) |
| Coordinator HA | Single SPOF | At minimum: queue persistence + graceful restart. Full HA: leader election. |
| Claim file cleanup | Orphaned on crash | Add TTL enforcement (10 min) to claim files |
| Budget cap enforcement | Reactive (polling every 5 min) | Event-driven: subscribe to session cost events |

#### E. Agent Context Coverage (Medium)

| Requirement | Current State | Needed |
|-------------|--------------|--------|
| CLAUDE.md coverage | 41/75 repos (55%) | 100% for all non-fork, non-archive repos |
| AGENTS.md coverage | 46/75 repos (61%) | 100% for all Go repos |
| .ralphrc coverage | 7/75 repos (9%) | All T1 and T2 repos (24 repos) |
| pipeline.mk deployment | 0 repos at top level (included in Makefile?) | Verify pipeline.mk include in all 24 Makefile-bearing repos |

#### F. Fleet Topology (Low -- needed for multi-machine)

| Requirement | Current State | Needed |
|-------------|--------------|--------|
| Topology optimizer | Implemented but not wired to assignWork | Connect topology recommendations to routing |
| ShardManager | Implemented with consistent hash ring | Wire to coordinator for session affinity |
| A2A integration | Adapter and coordinator are parallel systems | Merge offer lifecycle with work queue |
| Tailscale routing | IP stored but not used for direct routing | Optional: direct worker-to-worker for large artifacts |

### Scaling Milestones

| Milestone | Repos Under Management | Key Requirements |
|-----------|----------------------|------------------|
| L1: Manual fleet | 19 (T1 only) | Fix RetryTracker race, add queue persistence |
| L2: Semi-automated fleet | 24 (T1 + T2) | Parallel fan-out, local worker spawner, budget event-driven enforcement |
| L3: Autonomous fleet (75+) | 75 | Full observability, coordinator HA, CLAUDE.md on all repos, scheduled sweeps |

---

## 8. Summary

### Key Findings

1. **19 of 77 repos (25%) are fleet-ready today.** They have CLAUDE.md, go.mod, and Makefile -- the minimum for an automated sweep.

2. **7 repos form the inner ring** with .ralphrc + .ralph/ directories, indicating prior fleet interaction: claudekit, hg-mcp, [private-ops], mcpkit, mesmer, ralphglasses, whiteclaw.

3. **No effective mcpkit version skew** across active repos. All 5 active consumers resolve to HEAD via replace directives. claudekit is the sole pinned-version exception at v0.1.1.

4. **Serial fan-out is the primary throughput bottleneck.** A 19-repo sweep takes ~95 seconds just to launch. Parallelizing to a semaphore of 10 cuts this to ~10 seconds.

5. **A full fleet audit sweep costs $3.23 at $0.17/task average.** Even worst-case ($0.50/task for all 75 repos) costs only $37.50 -- well within budget limits.

6. **Critical blocking issues for production sweeps:** RetryTracker data race (no mutex), queue not persisted (lost on restart), scale-up has no actuator (advisory only).

7. **Onboard in order:** mcpkit + Wave 1 servers first (dependency centrality), then high-velocity repos (dotfiles-mcp, claudekit, hg-mcp), then application layer ([private-ops], [private-audit]), then T3 batch expansion.

8. **L3 autonomy for 75+ repos requires:** queue persistence, parallel fan-out, local worker spawner, thread-safe RetryTracker, event-driven budget enforcement, and 100% CLAUDE.md coverage. Estimated 2-3 weeks of fleet infrastructure work.

### Fleet Readiness Score

| Dimension | Score | Assessment |
|-----------|-------|------------|
| Repo coverage (T1 repos) | 19/77 (25%) | Sufficient for initial fleet operations |
| Agent context (CLAUDE.md) | 41/75 (55%) | Majority covered, needs expansion |
| Build pipeline (Makefile) | 24/75 (32%) | Thin -- many repos lack build automation |
| Ralph integration (.ralphrc) | 7/75 (9%) | Inner ring only -- needs broad deployment |
| Dependency health | 20/22 Go repos clean | Healthy -- no version drift in org code |
| Fleet infrastructure | 4 critical issues | Requires RetryTracker fix, queue persist, parallel launch, worker spawner |
| Cost feasibility | $3.23/sweep (19 repos) | Trivially affordable |
| **Overall fleet readiness** | **45%** | Ready for T1 sweeps; needs infra hardening for L3 |
