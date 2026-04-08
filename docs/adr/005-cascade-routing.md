# ADR 005: Tiered Cascade Routing with Latency Awareness

## Status

Accepted

## Context

Ralphglasses supports three LLM providers (Claude, Gemini, OpenAI) at different cost and capability tiers. Sending every task to the most capable (and expensive) model wastes budget on simple tasks like linting or classification. We needed a routing strategy that:

- Tries cheaper models first for simple tasks
- Escalates to expensive models when confidence is low
- Adapts to real-world provider latency
- Supports task-type overrides for known complexity patterns
- Learns from outcomes over time

## Decision

We implemented tiered cascade routing in `internal/session/cascade.go` via the `CascadeRouter`.

### Model tiers

Four tiers ordered by cost and capability (`DefaultModelTiers()`):

| Tier | Provider | Model | Complexity | Label |
|------|----------|-------|------------|-------|
| 1 | Gemini | gemini-3.1-flash-lite | 1 | ultra-cheap |
| 2 | Gemini | gemini-3.1-flash | 2 | worker |
| 3 | Claude | claude-sonnet | 3 | coding |
| 4 | Claude | claude-opus | 4 | reasoning |

### Routing logic

`CascadeConfig` defines the routing parameters:

- `CheapProvider` / `ExpensiveProvider` for two-tier cascade (default: Gemini then Claude)
- `ConfidenceThreshold` (default 0.7) -- escalate if cheap provider confidence is below this
- `MaxCheapBudgetUSD` / `MaxCheapTurns` -- budget and turn limits for the cheap attempt
- `TaskTypeOverrides` -- map specific task types directly to a provider
- `LatencyThresholdMs` -- skip cheap provider if its P95 latency exceeds this threshold

`ShouldCascade()` decides whether to attempt cheap-first routing. It short-circuits for tasks with explicit overrides or when latency data indicates the cheap provider is too slow.

### Latency tracking

`ProviderLatency` stores P50 and P95 percentiles from a sliding window of 100 samples per provider. When `LatencyThresholdMs > 0` and the cheap provider's P95 exceeds it, the router skips directly to the expensive provider. This prevents cascade attempts from adding latency to time-sensitive tasks.

### Adaptive learning

The router integrates with three feedback mechanisms:

- `FeedbackAnalyzer` -- tracks provider success rates per task type
- `DecisionLog` -- records every routing decision for audit and replay
- **Bandit hooks** (`banditSelect` / `banditUpdate`) -- optional multi-armed bandit policy (`internal/bandit/`) that explores provider selection based on reward signals
- **Decision model** -- optional calibrated confidence predictor that replaces heuristic confidence with learned estimates

`CascadeResult` records each routing outcome (provider used, escalation reason, costs), and `CascadeStats` aggregates them for the `ralphglasses_fleet_analytics` tool.

### Complexity mapping

`taskTypeComplexity` maps well-known task types to a 1-4 complexity scale (e.g., lint=1, codegen=3, architecture=4). `SelectTier()` uses this to pick the cheapest tier that meets the task's complexity requirement.

## Consequences

**Positive:**

- 60-80% cost reduction on simple tasks by using Gemini Flash Lite instead of Claude
- Latency awareness prevents cascading through a slow provider
- Task-type overrides give operators explicit control for known workloads
- Bandit integration enables continuous optimization without manual tuning
- All decisions are logged for observability and post-hoc analysis

**Negative:**

- Two-hop routing (cheap attempt + escalation) adds latency for tasks that always need the expensive provider
- Confidence heuristics may miscalibrate for novel task types
- Complexity mapping is static and may not match all workloads

**Mitigations:**

- `TaskTypeOverrides` lets operators bypass cascading for known-expensive tasks
- The calibrated decision model replaces heuristics once enough training data exists
- `SpeculativeExecution` flag (in `CascadeConfig`) allows parallel cheap+expensive attempts when latency matters more than cost
