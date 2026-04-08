# Phase 9 Research: Provider Expansion & Configuration

Covers ROADMAP items **2.5** (Multi-LLM Agent Orchestration) and **6.6** (Model Routing).

---

## 1. Executive Summary

- **Phase 2.5 is mostly complete** (2.5.1-2.5.4 done, 2.5.5 at 2/3, 2.5.6 unstarted): the three-provider architecture (Claude/Gemini/Codex) works end-to-end with command builders, event normalizers, agent discovery, team delegation, resume validation, and cost normalization. The remaining gaps are provider-specific cost fallback parsing (2.5.5.3) and batch API integration (2.5.6).
- **Phase 6.6 is entirely unstarted**: no model registry, task-type classifier, routing rules, dynamic routing, or cost-optimization suggestion logic exists in the codebase today.
- **The provider dispatch pattern in `providers.go` is well-designed** but adding a fourth provider requires touching 6-7 functions and all test files; a registry-based approach would reduce this to config-only additions.
- **Batch API integration (2.5.6) offers the single highest ROI**: all three providers support batch modes at ~50% cost reduction, and the existing `LaunchOptions` struct can be extended without breaking changes.
- **Model routing (6.6) depends on data that does not yet exist**: the `.ralphrc` parser handles arbitrary keys but has no typed model-routing schema, and the `costnorm.go` cost rates are hardcoded rather than registry-driven.

---

## 2. Current State Analysis

### 2.1 What Exists

| File | Lines | Test File | Test Lines | Coverage | Status |
|------|------:|-----------|----------:|----------|--------|
| `internal/session/providers.go` | 682 | `providers_test.go` | 390 | ~70% | Active, provider dispatch hub |
| `internal/session/health.go` | 100 | `health_test.go` | 64 | ~85% | Active, parallel health checks |
| `internal/session/failover.go` | 50 | (via manager_test) | -- | ~60% | Active, chain-based failover |
| `internal/session/types.go` | 147 | -- | -- | N/A | Active, core type definitions |
| `internal/session/costnorm.go` | 81 | `costnorm_test.go` | 63 | ~90% | Active, cross-provider cost normalization |
| `internal/session/ratelimit.go` | 99 | `ratelimit_test.go` | 80 | ~95% | Active, sliding-window rate limiter |
| `internal/session/templates.go` | 62 | `templates_test.go` | 61 | ~90% | Active, per-provider prompt templates |
| `internal/session/agents.go` | 321 | `agents_test.go` | 232 | ~80% | Active, multi-provider agent discovery |
| `internal/session/budget.go` | 131 | `budget_test.go` | 95 | ~85% | Active, budget enforcement + ledger |
| `internal/session/loop.go` | 870 | `loop_test.go` | 227 | ~50% | Active, planner/worker/verifier loop |
| `internal/session/runner.go` | 380 | `runner_test.go` | 221 | ~65% | Active, provider-agnostic session lifecycle |
| `internal/session/manager.go` | 846 | `manager_test.go` | 690 | ~70% | Active, session/team/workflow management |
| **Package total** | **3769** (src) | | **2123** (test) | **64.5%** | |

### 2.2 What Works Well

**Provider dispatch pattern** (`providers.go:131-159`): The `buildCmdForProvider()` function cleanly dispatches to per-provider command builders with shared validation. Each provider has its own `buildXxxCmd()` and `normalizeXxxEvent()` function, keeping provider-specific logic isolated.

**Flexible event normalization** (`providers.go:263-481`): The `normalizeGeminiEvent()` and `normalizeCodexEvent()` functions use path-based field extraction (`firstNonEmptyString`, `firstText`, `valueAtPath`) that gracefully handles varying JSON schemas across providers without tight coupling to any specific format.

**Health check parallelism** (`health.go:69-89`): `CheckAllProviderHealth()` runs all three provider health checks concurrently via goroutines with a buffered channel, completing in the time of the slowest check rather than their sum.

**Failover chain** (`failover.go:23-50`): `LaunchWithFailover()` pre-checks health before attempting launch, avoiding the cost of failed process spawns. The chain-based approach is extensible to N providers.

**Cost normalization** (`costnorm.go:36-72`): `NormalizeProviderCost()` supports both exact (token-based) and estimated (blended-rate) normalization, with Claude as the baseline. This enables apples-to-apples cost comparison across providers.

**Rate limiting** (`ratelimit.go:17-99`): Per-provider sliding-window rate limiter with configurable limits and `Remaining()` introspection. Thread-safe and well-tested.

**Agent discovery** (`agents.go:28-63`): Provider-aware reusable workflow discovery supporting three formats: Claude `.claude/agents/*.md`, Gemini `.gemini/commands/*.toml`, and Codex `AGENTS.md` sections.

**Prompt templates** (`templates.go:15-43`): Per-provider prompt wrapping that adds agentic context for Gemini/Codex while leaving Claude prompts unmodified.

**Loop engine** (`loop.go:191-338`): The `StepLoop()` function supports per-role provider configuration (planner, worker, verifier can each use different providers/models) and integrates prompt enhancement via `enhanceForProvider()`.

### 2.3 What Doesn't Work

**No batch API support** (ROADMAP 2.5.6): The codebase has zero batch-related code. `LaunchOptions` has no `BatchOptions` field, and there is no batch submission or result polling mechanism for any provider.

**Incomplete cost fallback** (ROADMAP 2.5.5.3): When providers do not report `cost_usd` in their JSON stream, there is no stderr-based cost parsing fallback. The `cleanProviderOutput()` function in `providers.go:404-418` extracts text from stderr for Codex but does not attempt cost extraction.

**No model registry** (ROADMAP 6.6.1): Available models are defined only as defaults in `ProviderDefaults()` (`providers.go:107-116`). There is no structured registry of model capabilities, context windows, or per-model pricing.

**No task-type classifier** (ROADMAP 6.6.2): There is no mechanism to classify tasks (code, review, test, docs) and route them to appropriate models. The prompt classifier in `internal/enhancer/` classifies task types for enhancement purposes but is not wired to model selection.

**No routing rules** (ROADMAP 6.6.3): The `.ralphrc` parser (`internal/model/config.go`) is a generic key-value store that could hold `MODEL_ROUTE_CODE=opus` style entries, but nothing reads or interprets such keys.

**No dynamic model switching** (ROADMAP 6.6.4): Sessions are launched with a fixed model and cannot switch mid-session. This is architecturally limited by the CLI subprocess model -- changing the model requires launching a new session.

**Hardcoded cost rates** (`costnorm.go:12-16`): `ProviderCostRates` is a package-level `var` with hardcoded prices. Adding a model or updating pricing requires a code change and rebuild. These should be configuration-driven.

**Provider addition requires 7 code changes** (CONTRIBUTING.md:127-136): Adding a new provider requires editing `types.go`, `providers.go` (4 functions + 2 switch statements), and `providers_test.go`. No plugin or registry mechanism exists.

---

## 3. Gap Analysis

### 3.1 ROADMAP Target vs Current State

| ROADMAP Item | Target | Current State | Gap |
|-------------|--------|---------------|-----|
| 2.5.1 — Fix provider CLI command builders | All three providers launchable | **COMPLETE** | None |
| 2.5.2 — Per-provider agent discovery | Provider-specific agent definitions | **COMPLETE** | None |
| 2.5.3 — Cross-provider team delegation | Claude lead delegates to Gemini/Codex workers | **COMPLETE** | None |
| 2.5.4 — Provider-specific resume support | Resume works for Claude/Gemini, rejects Codex | **COMPLETE** | None |
| 2.5.5.1-2 — Cost normalization verification | Cost tracked accurately for all providers | **COMPLETE** | None |
| 2.5.5.3 — Provider-specific cost fallback | Parse stderr for cost when JSON omits it | **NOT STARTED** | No stderr cost parsing |
| 2.5.6 — Batch API integration | Batch tasks for at least one provider | **NOT STARTED** | No batch code exists |
| 6.6.1 — Model registry | Available models with capabilities/pricing | **NOT STARTED** | Only default model strings |
| 6.6.2 — Task-type classifier | Map task types to preferred models | **NOT STARTED** | No classifier in session pkg |
| 6.6.3 — Routing rules in `.ralphrc` | `MODEL_ROUTE_CODE=opus` style config | **NOT STARTED** | Config parser exists, no routing keys |
| 6.6.4 — Dynamic routing | Switch model mid-session by task type | **NOT STARTED** | Requires native loop engine (6.1) |
| 6.6.5 — Cost optimization suggestions | Suggest cheaper model below complexity threshold | **NOT STARTED** | No complexity assessment |

### 3.2 Missing Capabilities

1. **Provider registry / plugin system**: No way to add a provider without code changes. A `ProviderConfig` struct + registration function would reduce new-provider effort from 7 edits to 1 config entry + 2 functions.

2. **Model metadata database**: No structured data on available models (context window, modality, cost/token, strengths). The `costnorm.go` rates and `ProviderDefaults()` are separate, disconnected data sources.

3. **Batch execution pipeline**: The entire batch lifecycle (submit, poll, collect results) is missing. This is a significant feature gap given the ~50% cost savings batch APIs offer.

4. **Configuration-driven model selection**: The `.ralphrc` infrastructure exists but is not extended with typed model-routing keys. No validation, no defaults, no per-task-type model preferences.

5. **Provider capability negotiation**: `UnsupportedOptionsWarnings()` (`providers.go:54-103`) returns warnings for unsupported options but does not prevent their use or suggest alternatives. This is informational only.

6. **Stderr cost extraction**: Gemini and Codex CLIs may report cost information in stderr or in different JSON fields than expected. No fallback parsing exists.

### 3.3 Technical Debt Inventory

| Debt Item | Location | Severity | Effort to Fix |
|-----------|----------|----------|---------------|
| Hardcoded cost rates | `costnorm.go:12-16` | Medium | S -- move to config/registry |
| Hardcoded default models | `providers.go:107-116` | Low | S -- derive from registry |
| Hardcoded provider list in health check | `health.go:70` | Low | S -- iterate registry |
| Provider binary names in switch | `providers.go:118-129` | Low | S -- move to registry |
| Env var names in switch | `providers.go:28-37` | Low | S -- move to registry |
| Rate limits hardcoded | `ratelimit.go:10-14` | Medium | S -- read from config |
| No provider-specific timeout tuning | `health.go:57` | Low | S -- 5s timeout for all |
| Duplicate provider validation logic | `providers.go:16-25` vs `health.go:44-48` | Low | S -- consolidate |
| No test for `LaunchWithFailover` | `failover.go` | Medium | M -- requires mock providers |

---

## 4. External Landscape

### 4.1 Competitor/Peer Projects

**1. LiteLLM (github.com/BerriAI/litellm)**
- Python proxy supporting 100+ LLM APIs via a unified interface
- Model registry with pricing, context windows, supported features
- Fallback chains, load balancing, budget limits, rate limiting
- Router with model-group aliasing and priority-based routing
- **Relevance to ralphglasses**: Their model registry data structure and routing configuration patterns are directly applicable. LiteLLM's `model_list` config with `model_info` (max_tokens, pricing, supports_function_calling) maps to the 6.6.1 model registry requirement.

**2. aisuite (github.com/andrewyng/aisuite)**
- Python library providing a unified interface across multiple LLM providers
- Provider abstraction with standard interface; adding new providers requires only implementing a simple adapter
- Supports OpenAI, Anthropic, Google, Mistral, and others
- **Relevance**: Demonstrates the minimal adapter pattern -- each provider implements a thin shim rather than requiring deep switch-statement integration. Validates the registry approach recommended for ralphglasses.

**3. OpenRouter (openrouter.ai)**
- API gateway routing across 200+ models from multiple providers
- Automatic fallback when a model is unavailable
- Cost-based routing: select cheapest model meeting capability requirements
- Per-model pricing database with real-time availability
- **Relevance**: Their routing-rules model (provider preference, fallback chains, cost constraints) aligns closely with 6.6.3 routing rules and 6.6.5 cost optimization.

**4. Cursor / Windsurf IDE integrations**
- Multi-model support within a single IDE session
- Automatic model selection based on task type (tab completion vs chat vs agent)
- Seamless provider switching without user intervention
- **Relevance**: Demonstrates practical task-type-to-model routing in a developer tool. Their approach of automatic model selection based on interaction type (quick completion vs deep reasoning) validates 6.6.2.

### 4.2 Patterns Worth Adopting

**Model registry as data, not code** (from LiteLLM): Define models in a YAML/JSON/Go file with structured fields (provider, model_id, cost_input, cost_output, context_window, capabilities, aliases). All dispatch functions read from the registry instead of hardcoded switches.

```go
// Target pattern
type ModelInfo struct {
    Provider      Provider
    ModelID       string
    Aliases       []string
    InputPer1M    float64
    OutputPer1M   float64
    ContextWindow int
    Capabilities  []string // "code", "vision", "tools", "long-context"
}
```

**Adapter registration** (from aisuite): Instead of 7 edits per provider, define a `ProviderAdapter` interface and register implementations at init time. The core dispatch reads from the adapter registry.

**Cost-aware routing** (from OpenRouter): When multiple models can serve a request, prefer the cheapest one that meets capability requirements. Fall back to more expensive models only when cheaper ones are unavailable or lack required capabilities.

**Config-driven routing rules** (from LiteLLM): Allow `.ralphrc` to specify model preferences per task type, with inheritance from global defaults:

```
MODEL_ROUTE_DEFAULT="sonnet"
MODEL_ROUTE_CODE="opus"
MODEL_ROUTE_REVIEW="sonnet"
MODEL_ROUTE_TEST="gemini-3.1-pro"
MODEL_ROUTE_DOCS="gpt-5.4-xhigh"
```

### 4.3 Anti-Patterns to Avoid

**Over-abstraction before multiple providers exist**: The current 3-provider setup does not justify a full plugin system with dynamic loading. A static registry with registration functions is sufficient.

**API-level provider abstraction**: Ralphglasses wraps CLI binaries, not API clients. An abstraction layer that pretends all providers have identical capabilities (like LiteLLM does for APIs) would fight the fundamental difference between CLI tools. Keep provider-specific quirks explicit.

**Automatic model switching in active sessions**: The CLI subprocess model means switching models requires launching a new process. Do not attempt hot-swapping -- instead, make model selection a planning-time decision in the loop engine.

**Real-time pricing APIs**: Fetching live pricing from provider APIs adds latency, failure modes, and network dependencies. Use a local registry updated periodically (via a CLI command or config file).

### 4.4 Academic & Industry References

- **Multi-armed bandit for model selection**: Thompson Sampling or Upper Confidence Bound algorithms could optimize model selection based on historical cost/quality data. Applicable to 6.6.5 cost optimization once historical data exists (depends on 6.4 analytics).

- **"Routing to the Expert" (Jiang et al., 2024)**: Research on routing prompts to the best-performing LLM based on prompt characteristics. Validates the task-type classifier approach in 6.6.2.

- **FrugalGPT (Chen et al., 2023)**: Cascade approach where cheaper models attempt a task first; expensive models are invoked only when the cheap model's confidence is low. Applicable to cost optimization (6.6.5).

- **LMSYS Chatbot Arena**: Crowdsourced model quality rankings. Their Elo ratings could inform default model preferences in the registry.

---

## 5. Actionable Recommendations

### 5.1 Immediate Actions (next sprint)

| # | Action | Target File(s) | Effort | Impact | ROADMAP |
|---|--------|---------------|--------|--------|---------|
| 1 | Add stderr cost extraction for Gemini/Codex | `internal/session/providers.go` (new `parseStderrCost()`) | S | Medium | 2.5.5.3 |
| 2 | Extract cost rates to model registry file | `internal/session/registry.go` (new), `internal/session/costnorm.go` | M | High | 6.6.1 |
| 3 | Make rate limits configurable via `.ralphrc` | `internal/session/ratelimit.go`, `internal/model/config.go` | S | Medium | 2.5 |
| 4 | Add `LaunchWithFailover` unit test | `internal/session/failover_test.go` (new) | S | Medium | 2.5 |
| 5 | Consolidate provider validation (remove duplication) | `internal/session/providers.go`, `internal/session/health.go` | S | Low | 2.5 |

### 5.2 Near-Term Actions (1-3 sprints)

| # | Action | Target File(s) | Effort | Impact | ROADMAP |
|---|--------|---------------|--------|--------|---------|
| 6 | Build model registry with capabilities/pricing/context window | `internal/session/registry.go` (new) | M | High | 6.6.1 |
| 7 | Implement task-type classifier reusing enhancer's `TaskType` | `internal/session/routing.go` (new), `internal/enhancer/classify.go` | M | High | 6.6.2 |
| 8 | Add `MODEL_ROUTE_*` keys to `.ralphrc` with parsing + validation | `internal/session/routing.go` (new), `internal/model/config.go` | M | High | 6.6.3 |
| 9 | Implement batch submission for Claude Messages Batches API | `internal/session/batch.go` (new) | L | High | 2.5.6.3 |
| 10 | Research and document batch API endpoints for all three providers | `docs/research/batch-api-reference.md` (new) | M | Medium | 2.5.6.1 |
| 11 | Add `BatchOptions` to `LaunchOptions` | `internal/session/types.go`, `internal/session/runner.go` | M | Medium | 2.5.6.2 |
| 12 | Wire routing rules into loop engine's planner/worker model selection | `internal/session/loop.go` | M | High | 6.6.3 |

### 5.3 Strategic Actions (3+ sprints)

| # | Action | Target File(s) | Effort | Impact | ROADMAP |
|---|--------|---------------|--------|--------|---------|
| 13 | Provider adapter registration pattern (reduce 7-edit to 2-edit for new providers) | `internal/session/providers.go`, `internal/session/registry.go` | L | High | 2.5 |
| 14 | Dynamic model routing in native loop engine | `internal/session/loop.go`, `internal/session/routing.go` | XL | High | 6.6.4 |
| 15 | Cost optimization suggestions (FrugalGPT-style cascade) | `internal/session/routing.go` | L | High | 6.6.5 |
| 16 | Batch API for Gemini (Batch Prediction API) | `internal/session/batch.go` | L | Medium | 2.5.6.4 |
| 17 | Batch polling/webhook for result collection | `internal/session/batch.go` | L | Medium | 2.5.6.5 |
| 18 | Provider plugin system with Go interface + factory | `internal/session/provider_iface.go` (new) | XL | Medium | 2.5 |

---

## 6. Risk Assessment

| # | Risk | Probability | Impact | Mitigation |
|---|------|-------------|--------|------------|
| 1 | Batch API semantics differ significantly across providers, making a unified interface impractical | Medium | High | Research all three APIs first (2.5.6.1) before designing the abstraction. Accept provider-specific batch code behind a common result type. |
| 2 | Provider CLI output formats change between versions, breaking event normalizers | Medium | High | Pin CLI versions in CI, add integration tests with version detection, maintain fallback parsing in `fallbackTextEvent()`. |
| 3 | Model registry becomes stale as providers add/deprecate models | High | Medium | Add a `ralphglasses registry update` CLI command that fetches current model lists from provider APIs (or bundled JSON). Include `last_updated` timestamp and staleness warnings. |
| 4 | Task-type classifier accuracy is too low for reliable model routing | Medium | Medium | Start with simple keyword/heuristic classification, validate against historical session data before adding ML-based classification. Allow manual override via `MODEL_ROUTE_*` config. |
| 5 | Dynamic model switching mid-session is impossible with CLI subprocess model | Low (expected) | Low | Accept this limitation. Route at session-launch time. Mid-session routing deferred to 6.6.4 which explicitly requires native loop engine (6.1). |
| 6 | Hardcoded rate limits become inaccurate as providers change their limits | Medium | Medium | Make rate limits configurable via `.ralphrc` (action #3). Add health-check response header parsing to auto-detect limits. |
| 7 | Adding batch API creates orphaned batch jobs on crash/restart | Medium | High | Implement batch job persistence (similar to session persistence in `manager.go:719-734`). Add cleanup on startup. |
| 8 | Over-engineering the model registry delays delivery of simpler routing features | Medium | Medium | Ship registry as a Go map literal first (like current `ProviderCostRates`). Migrate to config file only when model count exceeds what is comfortable in code. |

---

## 7. Implementation Priority Ordering

### 7.1 Critical Path

The critical path for completing ROADMAP 2.5 and 6.6:

```
2.5.5.3 (cost fallback) ──────────────────────────────────────┐
                                                                ├── 2.5 COMPLETE
2.5.6.1 (batch research) → 2.5.6.2 (BatchOptions) ──┐        │
                                                       ├── 2.5.6.3 (Claude batch) ──┤
                                                       └── 2.5.6.4 (Gemini batch) ──┘
                                                              ↓
                                                        2.5.6.5 (batch polling)

6.6.1 (model registry) → 6.6.2 (task classifier) → 6.6.3 (routing rules) ──┐
                                                                              ├── 6.6 COMPLETE
6.6.4 (dynamic routing -- BLOCKED BY 6.1) ──────────────────────────────────┤
                                                                              │
6.6.5 (cost optimization -- needs 6.4 analytics data) ─────────────────────┘
```

### 7.2 Recommended Sequence

**Sprint 1: Close Phase 2.5 gaps**
1. Implement stderr cost extraction (`providers.go`) -- 2.5.5.3
2. Add failover test (`failover_test.go`) -- quality debt
3. Consolidate provider validation -- tech debt
4. Make rate limits configurable -- tech debt

**Sprint 2: Model registry foundation**
5. Build model registry with capabilities/pricing (`registry.go`) -- 6.6.1
6. Migrate `ProviderCostRates` and `ProviderDefaults` to use registry -- 6.6.1
7. Migrate `ProviderRateLimits` to use registry -- 6.6.1

**Sprint 3: Routing rules**
8. Implement task-type classifier reusing `enhancer.TaskType` -- 6.6.2
9. Add `MODEL_ROUTE_*` keys to `.ralphrc` with parsing -- 6.6.3
10. Wire routing into loop engine model selection -- 6.6.3

**Sprint 4: Batch API (Phase 1)**
11. Research batch APIs for all providers -- 2.5.6.1
12. Add `BatchOptions` to `LaunchOptions` -- 2.5.6.2
13. Implement Claude Messages Batches API submission -- 2.5.6.3

**Sprint 5: Batch API (Phase 2) + Cost optimization**
14. Implement Gemini batch submission -- 2.5.6.4
15. Implement batch polling/result collection -- 2.5.6.5
16. Implement cost optimization suggestions -- 6.6.5

### 7.3 Parallelization Opportunities

The following work items are independent and can be assigned to different contributors simultaneously:

| Stream A (Provider) | Stream B (Routing) | Stream C (Batch) |
|---------------------|-------------------|------------------|
| 2.5.5.3 cost fallback | 6.6.1 model registry | 2.5.6.1 batch research |
| Failover tests | 6.6.2 task classifier | 2.5.6.2 BatchOptions |
| Provider validation cleanup | 6.6.3 routing rules | 2.5.6.3 Claude batch |
| Rate limit config | 6.6.3 loop integration | 2.5.6.4 Gemini batch |

**Stream A** and **Stream B** are fully independent until Stream B's 6.6.3 loop integration, which needs the registry from Stream B step 1.

**Stream C** is fully independent of Streams A and B.

Within Stream B, items 6.6.4 (dynamic routing) and 6.6.5 (cost optimization) have external dependencies: 6.6.4 requires the native loop engine (ROADMAP 6.1, a separate phase), and 6.6.5 benefits from historical analytics data (ROADMAP 6.4). Both can be stubbed but not fully implemented until those dependencies land.
