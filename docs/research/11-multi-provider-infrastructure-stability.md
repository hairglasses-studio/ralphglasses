# Phase 11 Research: Multi-Provider Infrastructure & Stability

Research date: 2026-03-22

Covers agent sandboxing, container isolation, MCP gateway, budget federation, secret management, Kubernetes operator, autoscaling, multi-cloud support, cloud cost management, and GitOps deployment. Maps to ROADMAP Phase 5 (Agent Sandboxing & Infrastructure) and Phase 7 (Kubernetes & Cloud Fleet).

---

## 1. Executive Summary

1. **No sandbox or container isolation code exists.** The entire `internal/sandbox/` package tree (Docker, Incus, Firecracker, gVisor) specified by Phase 5 is unimplemented. Sessions run as unsandboxed child processes sharing the host filesystem and network -- a significant security gap for autonomous agent fleets.

2. **Provider health, rate limiting, failover, and cost normalization are operational.** The four critical multi-provider stability files (`health.go`, `ratelimit.go`, `costnorm.go`, `failover.go`) are implemented, tested, and wired into the session manager. This foundation is solid for Phase 5.5 (budget federation) and Phase 7.4 (cloud cost management).

3. **Budget enforcement is per-session only.** `BudgetEnforcer` in `budget.go` tracks individual session spend with 90% headroom, but there is no global budget pool, no cross-session redistribution, and no billing API reconciliation (Phase 5.5 requires all three).

4. **No Kubernetes, cloud, or GitOps code exists.** Phase 7 is entirely greenfield: no CRD definitions, no operator controller, no Helm chart, no cloud provider interfaces. The dependency chain requires Phase 5 (sandbox model) and Phase 6 (fleet intelligence) to be substantially complete before Phase 7 work begins.

5. **Secret management uses plaintext environment variables.** API keys are loaded from `.env` via direnv. There is no secret provider interface, no SOPS/Vault integration, and no credential rotation -- all required by Phase 5.6.

---

## 2. Current State Analysis

### 2.1 What Exists

| File | Lines | Test File | Test Lines | Status |
|------|------:|-----------|----------:|--------|
| `internal/session/health.go` | 100 | `health_test.go` | 64 | Implemented: binary availability, env check, version latency, parallel check |
| `internal/session/ratelimit.go` | 99 | `ratelimit_test.go` | 80 | Implemented: sliding 1-min window, per-provider limits, thread-safe |
| `internal/session/costnorm.go` | 81 | `costnorm_test.go` | 63 | Implemented: token-level normalization to Claude baseline, efficiency % |
| `internal/session/failover.go` | 50 | (tested via manager) | -- | Implemented: ordered provider chain, health pre-check, error aggregation |
| `internal/session/providers.go` | 682 | `providers_test.go` | 390 | Implemented: 3-provider dispatch (claude/gemini/codex), event normalization, stderr sanitization |
| `internal/session/budget.go` | 131 | `budget_test.go` | 95 | Implemented: per-session headroom check, JSONL cost ledger, cost summary |
| `internal/session/manager.go` | 846 | `manager_test.go` | 690 | Implemented: session CRUD, teams, workflows, migration, persistence |
| `internal/session/runner.go` | 380 | `runner_test.go` | 221 | Implemented: provider-agnostic lifecycle, stream parsing, event bus |
| `internal/session/loop.go` | 870 | `loop_test.go` | 227 | Implemented: planner/worker/verifier, worktree isolation, journal |
| `internal/session/types.go` | 147 | -- | -- | Type definitions: Session, StreamEvent, LaunchOptions, TeamConfig, AgentDef |
| `internal/session/workflow.go` | 265 | `workflow_test.go` | 193 | Implemented: YAML parse, DAG validation, cycle detection, parallel execution |
| `internal/session/journal.go` | 401 | `journal_test.go` | 305 | Implemented: JSONL append, pattern consolidation, pruning, synthesis |
| `internal/session/checkpoint.go` | 50 | -- | -- | Implemented: git commit + tag on dirty tree |
| `internal/session/agents.go` | 321 | `agents_test.go` | 232 | Implemented: multi-provider agent discovery, composition, write |
| `internal/session/gitinfo.go` | 128 | -- | -- | Implemented: git log/diff within time window |
| `internal/events/bus.go` | 193 | `bus_test.go` | exists | Implemented: pub/sub with ring buffer history, cursor-based polling |
| `internal/notify/notify.go` | 40 | -- | -- | Implemented: macOS/Linux desktop notifications |
| `internal/sandbox/**` | 0 | -- | -- | **Does not exist** |
| `internal/cloud/**` | 0 | -- | -- | **Does not exist** |
| `internal/secrets/**` | 0 | -- | -- | **Does not exist** |
| `charts/**` | 0 | -- | -- | **Does not exist** |

**Total session package:** 4,650 source lines, 2,621 test lines (~56% test-to-source ratio).

### 2.2 What Works Well

**Provider health checking** (`health.go:31-66`): Parallel health checks across all three providers with binary availability, env var validation, and latency measurement. The `HealthyProviders()` helper returns only ready-to-use providers. This is directly reusable for sandbox readiness checks.

**Rate limiter** (`ratelimit.go:46-75`): Sliding 1-minute window with per-provider configurable limits, thread-safe via mutex. Default limits (Claude: 50, Gemini: 60, Codex: 20 req/min) are sensible. The `Remaining()` method enables proactive throttling in budget-aware scaling.

**Cost normalization** (`costnorm.go:36-72`): Cross-provider cost comparison by normalizing all spend to Claude-sonnet-4 rates. Supports both exact (token-count) and estimated (blended-rate) normalization. The `EfficiencyPct` field directly feeds cost optimization decisions for Phase 7.4.

**Failover chain** (`failover.go:22-49`): Health-check-first failover with error aggregation. The chain pattern is extensible to cloud provider failover (Phase 7.3).

**Session migration** (`manager.go:739-781`): `MigrateSession()` stops a running session and relaunches on a different provider, inheriting prompt, remaining budget, max turns, and team membership. This is foundational for dynamic provider routing.

**Event bus** (`events/bus.go:71-99`): Non-blocking pub/sub with 1000-event ring buffer and cursor-based polling. The event type taxonomy already covers session lifecycle, cost, team, and loop events. Adding sandbox/cloud event types is straightforward.

### 2.3 What Doesn't Work

**No container isolation (5.1, 5.2, 5.7, 5.8).** Sessions are bare child processes via `os/exec`. No filesystem isolation, no network isolation, no resource limits beyond what the process inherits. A malicious or buggy agent can access the entire host.

**No MCP gateway (5.3).** Tool calls go directly from the LLM to the local MCP server. There is no intermediary for authorization, audit logging, or rate limiting of individual tool calls.

**No network isolation (5.4).** Sessions inherit the host's full network access. No VLAN segmentation, no iptables rules, no DNS sinkholing.

**No global budget pool (5.5).** `BudgetEnforcer.Check()` compares per-session spend against per-session budget. There is no aggregate ceiling, no redistribution of unused budget, and no billing API reconciliation.

**No secret management (5.6).** API keys live in `.env` files loaded by direnv. Keys are passed as environment variables to child processes. No encryption at rest, no rotation, no audit trail of secret access.

**No Kubernetes operator (7.1).** No CRD, no controller, no pod template, no RBAC.

**No autoscaling (7.2).** Session count is manually controlled. No HPA integration, no scale-to-zero, no warm pools.

**No multi-cloud (7.3).** No cloud provider interface. Sessions run only on the local machine.

**No cloud cost management (7.4).** No integration with AWS Cost Explorer or GCP Billing API.

**No GitOps (7.5).** No Helm chart, no ArgoCD application, no Kustomize overlays.

---

## 3. Gap Analysis

### 3.1 ROADMAP Target vs Current State

| ROADMAP Item | Target | Current | Gap |
|-------------|--------|---------|-----|
| 5.1 Docker sandbox | Container lifecycle, bind mounts, GPU | Nothing | 100% greenfield |
| 5.2 Incus/LXD | Credential isolation, snapshots, threat detection | Nothing | 100% greenfield |
| 5.3 MCP gateway | Per-session tool auth, audit, rate limit | Direct MCP calls | 100% greenfield |
| 5.4 Network isolation | VLAN, iptables, DNS sinkhole | Host network | 100% greenfield |
| 5.5 Budget federation | Global pool, carry-over, billing API | Per-session only | ~15% done (BudgetEnforcer exists but single-session) |
| 5.6 Secret management | SOPS, Vault, rotation, audit | Plaintext .env | 100% greenfield |
| 5.7 Firecracker microVM | VM lifecycle, virtio-fs, snapshot/restore | Nothing | 100% greenfield |
| 5.8 gVisor runtime | runsc config, syscall filtering, benchmarks | Nothing | 100% greenfield |
| 7.1 K8s operator | CRD, controller, pod template, RBAC | Nothing | 100% greenfield |
| 7.2 Autoscaling | HPA, budget-aware, scale-to-zero, warm pool | Nothing | 100% greenfield |
| 7.3 Multi-cloud | AWS/GCP providers, unified fleet view | Nothing | 100% greenfield |
| 7.4 Cloud cost mgmt | Billing API, combined budget, spot strategy | Local cost tracking only | ~10% done (cost normalization reusable) |
| 7.5 GitOps | Helm, ArgoCD, Kustomize, sealed secrets | Nothing | 100% greenfield |

### 3.2 Missing Capabilities

1. **Container runtime abstraction.** Need `internal/sandbox/runtime.go` defining a `Runtime` interface with `Create`, `Start`, `Stop`, `Exec`, `Logs`, `Remove` methods. Docker, Incus, Firecracker, and gVisor should implement this interface.

2. **Session-to-container binding.** `runner.go:launch()` currently calls `cmd.Start()` directly. For sandboxed sessions, it needs to create a container first, then exec the CLI binary inside it. This requires a `SandboxConfig` field on `LaunchOptions`.

3. **Network policy engine.** Per-session network rules translated to container-native isolation (Docker networks, iptables, nftables). The `.ralphrc` schema needs `SANDBOX_ALLOWED_DOMAINS` and `SANDBOX_NETWORK_MODE` keys.

4. **Global budget coordinator.** A SQLite-backed `BudgetPool` that tracks aggregate spend, enforces ceilings, and redistributes unused allocation. Needs to integrate with the existing `BudgetEnforcer` per-session checks.

5. **Secret provider interface.** `internal/secrets/Provider` interface with `Get(key) (value, error)` and `List() ([]key, error)`. SOPS and Vault backends. Integration with container env injection.

6. **Kubernetes CRD and controller.** `RalphSession` custom resource, reconciliation loop, pod template with PVC for workspace, secret mounts for API keys. Requires controller-runtime.

7. **Cloud provider interface.** `internal/cloud/Provider` interface with `Launch`, `Stop`, `List`, `Cost` methods. AWS and GCP implementations.

### 3.3 Technical Debt Inventory

| Debt Item | Location | Impact | ROADMAP Blocker |
|-----------|----------|--------|----------------|
| Hardcoded `Setpgid` assumes Linux | `providers.go:200`, `providers.go:229`, `providers.go:251` | Crashes on macOS in some edge cases | 5.1 (container replaces process groups) |
| `BudgetEnforcer` ignores global limits | `budget.go:23-37` | No global spend ceiling | 5.5 |
| No retry/backoff on provider API errors | `runner.go:258-270` | Parse errors increment counter but session continues | 5.3 (gateway should handle retries) |
| `PersistSession` ignores write errors | `manager.go:723-734` | Silent data loss on disk full | 5.5, 7.4 (budget data must be durable) |
| Plaintext API keys in process env | `providers.go:203`, child `cmd.Env` | Security risk | 5.6 |
| No container cleanup on crash | Not implemented | Leaked containers | 5.1, 5.2, 5.7 |
| Single event bus (in-memory) | `events/bus.go` | History lost on restart | 7.1 (K8s needs persistent events) |
| `ProviderCostRates` are hardcoded | `costnorm.go:12-16` | Stale pricing | 7.4 |

---

## 4. External Landscape

### 4.1 Competitor/Peer Projects

#### 1. kubernetes-sigs/agent-sandbox (v0.1.0)
- **URL:** https://github.com/kubernetes-sigs/agent-sandbox
- **Relevance:** Official Kubernetes SIG project for sandboxing AI coding agents. Defines `AgentSandbox` CRD with gVisor + Kata Containers support. Implements `WarmPools` for pre-warmed pods.
- **Key patterns:** CRD-based lifecycle, warm pool concept, resource limit spec, RBAC templates.
- **Applicability:** Directly maps to ROADMAP 7.1 (K8s operator) and 5.8 (gVisor). The CRD schema for `RalphSession` should align with `AgentSandbox` for interoperability.

#### 2. Alibaba OpenSandbox
- **URL:** https://github.com/alibaba/OpenSandbox
- **Relevance:** Production multi-runtime sandbox platform supporting Firecracker, gVisor, and Kata Containers. Provides a unified `Runtime` interface that abstracts sandbox implementation.
- **Key patterns:** Runtime interface abstraction, sandbox lifecycle (create, exec, pause, resume, destroy), virtio-fs workspace mounting, resource rate limiting.
- **Applicability:** The `Runtime` interface pattern is the correct abstraction for `internal/sandbox/`. OpenSandbox's Firecracker integration (boot time < 200ms) validates the ROADMAP 5.7 target of < 500ms.

#### 3. E2B (e2b.dev)
- **URL:** https://e2b.dev
- **Relevance:** Production SaaS for sandboxed code execution using Firecracker microVMs. Sub-200ms boot times. State management (snapshot/restore) for pause/resume.
- **Key patterns:** Firecracker-based isolation, snapshot/restore for session persistence, workspace filesystem overlays, per-sandbox network policies.
- **Applicability:** Validates Firecracker as viable for Phase 5.7. E2B's snapshot/restore pattern maps to session pause/resume. Their pricing model informs Phase 7.4 cost projections.

#### 4. Daytona
- **URL:** https://github.com/daytonaio/daytona
- **Relevance:** Docker-based development environment manager with < 90ms startup. Supports state management, workspace persistence, and multi-provider infrastructure.
- **Key patterns:** Docker container pools, workspace PVC management, provider-agnostic infrastructure layer, GitOps deployment.
- **Applicability:** Daytona's Docker pool pattern maps to ROADMAP 7.2 warm pools. Their provider abstraction informs `internal/cloud/` design.

#### 5. code-on-incus (mensfeld)
- **URL:** https://github.com/mensfeld/code-on-incus
- **Relevance:** Go-based Incus container management specifically for AI coding agents. Includes threat detection and security profiles. Already listed in ROADMAP external projects.
- **Key patterns:** Go Incus client, per-container credential isolation (file-mount, not env vars), threat detection via resource spike monitoring, security profiles.
- **Applicability:** Directly informs Phase 5.2. The credential isolation pattern (mount as files, not env vars) should be adopted for all sandbox types, not just Incus.

#### 6. StereOS (papercomputeco)
- **URL:** https://github.com/papercomputeco/stereOS
- **Relevance:** NixOS-based agent operating system with gVisor sandboxing. Produces VM images for autonomous agent fleets.
- **Key patterns:** NixOS reproducible builds, gVisor syscall filtering profiles tailored for coding agents, VM image generation pipeline.
- **Applicability:** gVisor profile for coding agents is directly usable for Phase 5.8. NixOS build pipeline is an alternative to the current Ubuntu-based distro approach.

### 4.2 Patterns Worth Adopting

1. **Runtime interface abstraction (OpenSandbox).** Define `sandbox.Runtime` with `Create(config) -> SandboxID`, `Exec(id, cmd) -> output`, `Stop(id)`, `Remove(id)`. Each sandbox type (Docker, Incus, Firecracker, gVisor) implements this interface. The session runner calls `runtime.Exec()` instead of `cmd.Start()`.

2. **Credential mount (code-on-incus).** Mount API keys as read-only files at a well-known path inside the sandbox (e.g., `/run/secrets/anthropic_api_key`) instead of environment variables. This prevents credential leakage via `/proc/PID/environ` and enables per-key audit logging.

3. **Warm pool (kubernetes-sigs/agent-sandbox).** Maintain N pre-created containers/VMs with base images already loaded. On session launch, pick from the pool, mount workspace, inject credentials, and start the CLI. This reduces cold start from seconds to < 100ms.

4. **CRD status subresource (kubernetes-sigs/agent-sandbox).** The K8s CRD should use a status subresource to report session state, spend, and progress back to K8s. This enables standard K8s tooling (kubectl, dashboards) to observe fleet state.

5. **Budget pools with carry-over.** Inspired by AWS Organizations billing: define a global ceiling, allocate per-session quotas that can exceed individual limits if the global pool allows. Redistribute unused budget from completed sessions to active ones.

6. **MCP gateway as reverse proxy.** Pattern from MCP specification's gateway concept: the gateway accepts MCP requests from agents, applies per-session authorization rules (tool allowlists), logs every call, rate-limits per session, and forwards to backend MCP servers. This is a standard reverse proxy pattern adapted for MCP.

### 4.3 Anti-Patterns to Avoid

1. **Premature multi-cloud abstraction.** Building AWS and GCP providers before the Docker sandbox is stable. The dependency chain is clear: sandbox -> operator -> multi-cloud. Skip ahead and you build untestable abstractions.

2. **Overengineered secret rotation.** Vault lease management with auto-renew adds operational complexity. Start with SOPS (encrypted files in git) and add Vault only when needed. Most ralphglasses deployments are single-machine.

3. **Firecracker on macOS.** Firecracker requires Linux KVM. Do not attempt macOS support for microVMs. Use Docker as the macOS fallback.

4. **Custom Kubernetes scheduler.** The standard kube-scheduler with node affinity labels is sufficient for GPU placement. Do not build a custom scheduler for Phase 7.2.

5. **Monolithic gateway.** Building the MCP gateway as a single binary that also runs the TUI and sessions. The gateway should be a standalone service (systemd unit or K8s deployment) so it survives session crashes.

### 4.4 Academic & Industry References

| Reference | Relevance |
|-----------|-----------|
| Firecracker: Lightweight Virtualization (NSDI 2020) | <200ms boot, memory overcommit, rate-limited I/O. Validates Phase 5.7 targets. |
| gVisor: Container runtime sandbox (Google, 2018) | Syscall interception design, performance overhead analysis. Informs Phase 5.8 benchmarking. |
| Borg, Omega, and Kubernetes (ACM Queue 2016) | Bin-packing, resource allocation, cluster autoscaling. Informs Phase 7.2. |
| Kubernetes Operator pattern (Red Hat, 2019) | CRD + controller reconciliation loop. Directly applicable to Phase 7.1. |
| Cloud-native cost optimization (FinOps Foundation, 2024) | Spot vs on-demand strategy, idle resource detection, budget alerting. Informs Phase 7.4. |
| SOPS: Secrets OPerationS (Mozilla) | Encrypted secrets in git. Simple, auditable. Phase 5.6 first backend. |
| HashiCorp Vault KV v2 | Versioned secrets, dynamic credentials, lease management. Phase 5.6 advanced backend. |

---

## 5. Actionable Recommendations

### 5.1 Immediate Actions

| # | Action | Target Files | Effort | Impact | ROADMAP Item |
|---|--------|-------------|--------|--------|-------------|
| 1 | Define `sandbox.Runtime` interface with `Create`, `Exec`, `Stop`, `Remove`, `Logs` | `internal/sandbox/runtime.go` (new) | S | High | 5.1, 5.2, 5.7, 5.8 |
| 2 | Add `SandboxConfig` field to `LaunchOptions` (optional, nil = unsandboxed) | `internal/session/types.go:91` | S | High | 5.1 |
| 3 | Implement Docker sandbox runtime | `internal/sandbox/docker/docker.go` (new) | L | High | 5.1 |
| 4 | Add `SANDBOX_MODE`, `SANDBOX_ALLOWED_DOMAINS` keys to `.ralphrc` schema | `internal/model/config.go`, `internal/model/config_schema.go` | S | Medium | 5.1, 5.4 |
| 5 | Define `secrets.Provider` interface with `Get`, `List`, `Watch` | `internal/secrets/provider.go` (new) | S | High | 5.6 |
| 6 | Implement SOPS backend for secrets | `internal/secrets/sops/sops.go` (new) | M | High | 5.6 |
| 7 | Add `BudgetPool` struct with global ceiling and per-session allocation | `internal/session/budget_pool.go` (new) | M | High | 5.5 |
| 8 | Wire `BudgetPool` into `Manager` with aggregate spend tracking | `internal/session/manager.go` | M | High | 5.5 |

### 5.2 Near-Term Actions

| # | Action | Target Files | Effort | Impact | ROADMAP Item |
|---|--------|-------------|--------|--------|-------------|
| 9 | Implement Incus sandbox runtime | `internal/sandbox/incus/incus.go` (new) | L | Medium | 5.2 |
| 10 | Build MCP gateway as standalone service | `cmd/mcp-gateway/main.go` (new), `internal/gateway/gateway.go` (new) | XL | High | 5.3 |
| 11 | Per-session tool authorization in gateway | `internal/gateway/authz.go` (new) | M | High | 5.3 |
| 12 | Audit logging for all tool calls through gateway | `internal/gateway/audit.go` (new) | M | High | 5.3 |
| 13 | Network isolation: Docker network mode, iptables rules | `internal/sandbox/docker/network.go` (new) | M | High | 5.4 |
| 14 | Budget dashboard TUI view (spend rate, projection, breakdown) | `internal/tui/views/budget.go` (new) | M | Medium | 5.5 |
| 15 | Vault backend for secrets | `internal/secrets/vault/vault.go` (new) | L | Medium | 5.6 |
| 16 | Secret rotation with affected session restart | `internal/secrets/rotation.go` (new) | M | Medium | 5.6 |
| 17 | Add sandbox event types to event bus | `internal/events/bus.go` | S | Medium | 5.1 |
| 18 | Container cleanup on session crash (defer pattern) | `internal/session/runner.go` | S | High | 5.1 |

### 5.3 Strategic Actions

| # | Action | Target Files | Effort | Impact | ROADMAP Item |
|---|--------|-------------|--------|--------|-------------|
| 19 | Implement Firecracker microVM runtime | `internal/sandbox/firecracker/firecracker.go` (new) | XL | Medium | 5.7 |
| 20 | gVisor runtime profile for coding agents | `internal/sandbox/gvisor/gvisor.go` (new) | L | Medium | 5.8 |
| 21 | Define `RalphSession` CRD | `config/crd/ralphsession.yaml` (new) | M | High | 7.1 |
| 22 | K8s controller with reconciliation loop | `internal/operator/controller.go` (new) | XL | High | 7.1 |
| 23 | Pod template with workspace PVC and secret mounts | `internal/operator/podtemplate.go` (new) | M | High | 7.1 |
| 24 | HPA integration for budget-aware autoscaling | `internal/operator/autoscaler.go` (new) | L | High | 7.2 |
| 25 | Warm pool implementation | `internal/operator/warmpool.go` (new) | L | Medium | 7.2 |
| 26 | AWS cloud provider | `internal/cloud/aws/aws.go` (new) | XL | Medium | 7.3 |
| 27 | GCP cloud provider | `internal/cloud/gcp/gcp.go` (new) | XL | Medium | 7.3 |
| 28 | Cloud provider interface | `internal/cloud/provider.go` (new) | M | High | 7.3 |
| 29 | Combined budget (API + cloud compute) | `internal/session/budget_cloud.go` (new) | L | High | 7.4 |
| 30 | Helm chart | `charts/ralphglasses/` (new directory) | L | High | 7.5 |
| 31 | ArgoCD application and Kustomize overlays | `deploy/` (new directory) | M | Medium | 7.5 |
| 32 | Sealed secrets for git-committed manifests | `charts/ralphglasses/templates/sealedsecret.yaml` (new) | S | High | 7.5 |

---

## 6. Risk Assessment

| # | Risk | Probability | Impact | Mitigation |
|---|------|-------------|--------|------------|
| 1 | Docker SDK version conflicts with host Docker daemon | Medium | High | Pin Docker SDK version; test against Docker CE 24.x and 27.x; degrade gracefully on version mismatch |
| 2 | Firecracker requires Linux KVM, not available on macOS or WSL2 | High | Medium | Make Firecracker optional; Docker fallback on non-KVM hosts; clear error message when KVM unavailable |
| 3 | gVisor performance overhead makes sessions unacceptably slow | Medium | Medium | Benchmark before adoption (Phase 5.8.3); provide runc fallback; let users choose runtime via config |
| 4 | MCP gateway becomes a single point of failure | Medium | High | Health check endpoint; systemd auto-restart; circuit breaker between gateway and backend MCP servers |
| 5 | Global budget pool race conditions under concurrent session launch | Medium | High | Use SQLite WAL mode with row-level locking; test with 20+ concurrent sessions; add budget overdraft detection |
| 6 | Secret rotation triggers cascade session restarts | Low | High | Stagger restarts with jitter; restart only sessions using the rotated key; rate-limit restarts per minute |
| 7 | K8s operator memory leak from leaked watchers | Medium | Medium | Use controller-runtime's built-in cache; bounded work queues; integration test with 1000+ CRD objects |
| 8 | Spot instance preemption loses in-progress session work | High | Medium | Checkpoint before spot instance shutdown via preemption notice handler; prefer on-demand for critical sessions |
| 9 | Helm chart complexity blocks adoption | Low | Medium | Provide sensible defaults; minimal required values (API key, namespace); include `values.yaml` examples |
| 10 | Phase 5/7 scope is too large; delivery takes 6+ months | High | High | Decompose into independent workstreams (5.1 and 5.6 are independent); ship Docker sandbox first; defer Firecracker and multi-cloud until Docker + K8s are stable |
| 11 | Cost normalization rates become stale as providers change pricing | High | Low | Make `ProviderCostRates` configurable via `.ralphrc`; add `costnorm_update` MCP tool; log warning when rates are > 90 days old |
| 12 | Network isolation breaks legitimate agent network access | Medium | Medium | Default to "allow known API endpoints" policy; per-provider allowlists; explicit `SANDBOX_NETWORK_MODE=none/restricted/open` in `.ralphrc` |

---

## 7. Implementation Priority Ordering

### 7.1 Critical Path

The critical path follows the ROADMAP dependency chain:

```
Phase 5.1 (Docker) ──→ Phase 5.4 (Network isolation)
    │
Phase 5.6 (Secrets) ──→ Phase 5.2 (Incus, uses secrets)
    │
Phase 5.5 (Budget federation) ──→ Phase 7.4 (Cloud cost)
    │
Phase 5.3 (MCP gateway) [independent]
    │
Phase 5.1 + 5.5 + Phase 6 ──→ Phase 7.1 (K8s operator)
    │
Phase 7.1 ──→ Phase 7.2 (Autoscaling) ──→ Phase 7.3 (Multi-cloud)
    │
Phase 7.1 ──→ Phase 7.5 (GitOps)
```

**Blockers:**
- Phase 7 depends on Phase 5 (sandbox model) AND Phase 6 (fleet intelligence). Both must be substantially complete.
- Phase 5.4 (network isolation) depends on 5.1 or 5.2 (needs a container to isolate).
- Phase 5.5 (budget federation) depends on Phase 2.3 (per-session budget tracking, which exists in `budget.go`).

### 7.2 Recommended Sequence

**Sprint 1 (Weeks 1-3): Foundation interfaces + Docker sandbox**
1. `internal/sandbox/runtime.go` -- Runtime interface (Action 1, Effort S)
2. `internal/session/types.go` -- Add SandboxConfig to LaunchOptions (Action 2, Effort S)
3. `internal/sandbox/docker/docker.go` -- Docker runtime implementation (Action 3, Effort L)
4. `internal/sandbox/docker/network.go` -- Network isolation (Action 13, Effort M)
5. `internal/session/runner.go` -- Container cleanup on crash (Action 18, Effort S)

**Sprint 2 (Weeks 4-5): Secrets + Budget federation**
6. `internal/secrets/provider.go` -- Secrets interface (Action 5, Effort S)
7. `internal/secrets/sops/sops.go` -- SOPS backend (Action 6, Effort M)
8. `internal/session/budget_pool.go` -- Global budget pool (Action 7, Effort M)
9. `internal/session/manager.go` -- Wire budget pool into manager (Action 8, Effort M)

**Sprint 3 (Weeks 6-8): MCP gateway**
10. `cmd/mcp-gateway/main.go` -- Gateway service (Action 10, Effort XL)
11. `internal/gateway/authz.go` -- Per-session authorization (Action 11, Effort M)
12. `internal/gateway/audit.go` -- Audit logging (Action 12, Effort M)

**Sprint 4 (Weeks 9-11): Alternative runtimes**
13. `internal/sandbox/incus/incus.go` -- Incus runtime (Action 9, Effort L)
14. `internal/sandbox/gvisor/gvisor.go` -- gVisor runtime (Action 20, Effort L)
15. `internal/secrets/vault/vault.go` -- Vault backend (Action 15, Effort L)

**Sprint 5 (Weeks 12-15): Kubernetes operator**
16. `config/crd/ralphsession.yaml` -- CRD definition (Action 21, Effort M)
17. `internal/operator/controller.go` -- Controller (Action 22, Effort XL)
18. `internal/operator/podtemplate.go` -- Pod template (Action 23, Effort M)

**Sprint 6 (Weeks 16-18): Autoscaling + GitOps**
19. `internal/operator/autoscaler.go` -- HPA integration (Action 24, Effort L)
20. `internal/operator/warmpool.go` -- Warm pool (Action 25, Effort L)
21. `charts/ralphglasses/` -- Helm chart (Action 30, Effort L)
22. `deploy/` -- ArgoCD + Kustomize (Action 31, Effort M)

**Sprint 7 (Weeks 19-22): Multi-cloud + Advanced**
23. `internal/cloud/provider.go` -- Cloud provider interface (Action 28, Effort M)
24. `internal/cloud/aws/aws.go` -- AWS provider (Action 26, Effort XL)
25. `internal/cloud/gcp/gcp.go` -- GCP provider (Action 27, Effort XL)
26. `internal/session/budget_cloud.go` -- Combined budget (Action 29, Effort L)
27. `internal/sandbox/firecracker/firecracker.go` -- Firecracker runtime (Action 19, Effort XL)

### 7.3 Parallelization Opportunities

The following workstreams can proceed concurrently:

**Stream A: Sandbox runtimes** (5.1, 5.2, 5.7, 5.8)
- Docker, Incus, Firecracker, and gVisor implementations are independent once the `sandbox.Runtime` interface is defined.
- Each sandbox type needs its own test environment (Docker daemon, Incus host, KVM).

**Stream B: MCP gateway** (5.3)
- Fully independent of sandbox work. Can start immediately.
- The gateway is a separate binary and package (`internal/gateway/`).

**Stream C: Secret management** (5.6)
- Independent of sandbox and gateway work.
- SOPS and Vault backends can be developed in parallel.

**Stream D: Budget federation** (5.5)
- Independent of sandbox work; depends only on existing `budget.go`.
- Budget dashboard TUI view can be developed concurrently.

**Stream E: K8s operator and GitOps** (7.1, 7.5)
- 7.1 (operator) and 7.5 (Helm chart) can proceed in parallel.
- 7.2 (autoscaling) blocks on 7.1 completion.

**Stream F: Multi-cloud** (7.3, 7.4)
- AWS and GCP providers can be developed in parallel.
- Cloud cost management (7.4) depends on 7.1 (operator) for K8s integration but can prototype against local sessions first.

**Maximum parallelism:** Streams A, B, C, and D can all run simultaneously in Sprint 1-2. This means four engineers could work on Phase 5 concurrently after the Runtime interface is defined (a 1-day task for one person).
