# Ralphglasses Deep Research Index

13 research documents produced March 2026 via parallel deep research agents, covering all development phases from foundation through advanced orchestration.

---

## Reading Order

Recommended progression from foundational to advanced:

### Tier 1: Foundation (read first)

1. [Foundation & Core Infrastructure](01-foundation-core-infrastructure.md) —
   Go module structure, Cobra CLI, discovery engine, model layer, process
   management. Prerequisite context for everything else.
2. [Testing & Validation](02-testing-validation.md) —
   Test strategy, coverage targets, CI pipeline, fuzz testing, BATS shell
   tests. Quality foundations.

### Tier 2: Core Systems

3. [Session Management & Persistence](08-session-management-persistence.md) —
   Session data model, SQLite, lifecycle state machine, worktree
   orchestration. The central data model that gates 200+ downstream items.
4. [Multi-Session Orchestration](05-multi-session-orchestration.md) —
   Fleet management, budget tracking, tmux integration, session launcher.
   Built on session model.
5. [Cost Management & Cross-repo](03-cost-management-cross-repo.md) —
   Budget federation, cost normalization, spend tracking, breakglass
   criteria. Cross-cutting concern.

### Tier 3: Provider & Infrastructure

6. [Provider Expansion & Configuration](09-provider-expansion-configuration.md) —
   Claude, Gemini, Codex provider architecture. CLI command builders,
   event normalizers, batch API, model routing.
7. [Multi-Provider Infrastructure & Stability](11-multi-provider-infrastructure-stability.md) —
   Health checks, failover, rate limiting, circuit breakers, sandbox
   security profiles.
8. [End-to-End Multi-Provider Enhancement](12-end-to-end-multi-provider-enhancement.md) —
   Full-stack multi-provider: prompt enhancement, scoring normalization,
   A/B testing, code review automation.

### Tier 4: Monitoring & UX

9. [Fleet Monitoring & Event Infrastructure](06-fleet-monitoring-event-infrastructure.md) —
   Event bus, pub/sub, analytics, observability, Prometheus, Grafana.
10. [TUI Enhancement & UI Polish](07-tui-enhancement-ui-polish.md) —
    Bubble Tea components, views, themes, i3 integration, multi-monitor.

### Tier 5: Strategy & Meta

11. [Roadmap Planning & Backlog Resolution](04-roadmap-planning-backlog-resolution.md) —
    Roadmap methodology, dependency graph validation, backlog
    prioritization, task management automation.
12. [Research Pipeline & Monorepo Integration](10-research-pipeline-monorepo-integration.md) —
    Research automation, awesome-list pipeline, monorepo patterns,
    cross-repo tooling.
13. [Decoupling & Cross-Project Learning](13-decoupling-cross-project-learning.md) —
    Knowledge transfer, pattern extraction from internal ecosystem repos,
    reusable components.

---

## Workstream Decomposition

See [WORKSTREAM-DECOMPOSITION.md](WORKSTREAM-DECOMPOSITION.md) for the
parallel workstream analysis organizing 444 subtasks into 16 workstreams
with dependency graph, critical path analysis, and execution timeline.

---

## Related Existing Documentation

These documents in `docs/` predate the structured research effort but
contain relevant research and context:

| Document | Relevance |
|----------|-----------|
| [RESEARCH.md](../RESEARCH.md) | Agent OS, sandboxing, homelab architecture |
| [MULTI-SESSION.md](../MULTI-SESSION.md) | Multi-session tool comparison |
| [awesome-claude-code-research.md](../awesome-claude-code-research.md) | Ecosystem analysis (181 repos) |
| [awesome-integration-candidates.md](../awesome-integration-candidates.md) | Top 10 integration targets |
| [remote-control-research.md](../remote-control-research.md) | Mobile remote control architecture |
| [BREAKGLASS.md](../BREAKGLASS.md) | Circuit breaker and budget criteria |
| [LOOP-DEFAULTS.md](../LOOP-DEFAULTS.md) | Default loop configuration |

---

## Cross-References to ROADMAP.md

| Research Doc | ROADMAP Phases |
|-------------|----------------|
| 01 Foundation | 0, 0.5 |
| 02 Testing | 0.5, 1, 1.5.10 |
| 03 Cost | 2.3, 5.5, 6.10, 7.4 |
| 04 Roadmap | Cross-cutting |
| 05 Multi-Session | 2, 6.1, 6.3 |
| 06 Fleet/Events | 2.75, 6.4, 6.5, 6.7 |
| 07 TUI | 1.3, 1.10, 2.4 |
| 08 Session | 2.1, 2.2, 2.10 |
| 09 Provider | 2.5, 6.6 |
| 10 Research Pipeline | 6.2, 8.5, 8.6 |
| 11 Multi-Provider Infra | 5, 7 |
| 12 E2E Multi-Provider | 8.2, 8.4, 6.8 |
| 13 Decoupling | Ecosystem integration |
