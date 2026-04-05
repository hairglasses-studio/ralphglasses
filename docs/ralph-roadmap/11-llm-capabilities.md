# 11 -- LLM Provider Capabilities & Cost Optimization Analysis

Generated: 2026-04-04

## 1. Provider Capability Matrix

### Models & Pricing (per 1M tokens, standard context)

| Provider | Model | Tier | Input | Output | Cached Input | Context Window | Max Output |
|----------|-------|------|-------|--------|-------------|----------------|------------|
| **Anthropic** | Opus 4.6 | Reasoning | $5.00 | $25.00 | $0.50 (90% off) | 1M | 128K (300K batch) |
| **Anthropic** | Sonnet 4.6 | Coding | $3.00 | $15.00 | $0.30 (90% off) | 1M | 64K (300K batch) |
| **Anthropic** | Haiku 4.5 | Worker | $1.00 | $5.00 | $0.10 (90% off) | 200K | 8K |
| **Google** | Gemini 3.1 Pro | Reasoning | $2.00 | $12.00 | $0.20 (90% off) | 1M | 65K |
| **Google** | Gemini 3 Flash | Coding | $0.50 | $3.00 | $0.05 (90% off) | 1M | 65K |
| **Google** | Gemini 2.5 Flash | Worker | $0.30 | $2.50 | $0.03 (90% off) | 1M | 65K |
| **Google** | Gemini 2.5 Flash-Lite | Ultra-cheap | $0.10 | $0.40 | $0.01 (90% off) | 1M | 65K |
| **Google** | Gemini 3.1 Flash-Lite | Ultra-cheap | $0.25 | $1.50 | $0.025 (90% off) | 1M | 65K |
| **OpenAI** | GPT-5.4 | Reasoning | $2.50 | $15.00 | $0.25 (90% off) | 1M | 33K |
| **OpenAI** | codex-mini-latest | Coding | $1.50 | $6.00 | $0.375 (75% off) | 200K | 16K |
| **OpenAI** | GPT-4.1 | Worker | $2.00 | $8.00 | $1.00 (50% off) | 1M | 33K |
| **OpenAI** | GPT-4.1-mini | Worker | $0.40 | $1.60 | $0.20 (50% off) | 1M | 33K |
| **OpenAI** | GPT-4.1-nano | Ultra-cheap | $0.05 | $0.20 | $0.025 (50% off) | 1M | 16K |

Sources:
- Anthropic pricing: https://platform.claude.com/docs/en/about-claude/pricing
- Google pricing: https://ai.google.dev/gemini-api/docs/pricing
- OpenAI pricing: https://developers.openai.com/api/docs/pricing

### Long-Context Surcharges

| Model | Threshold | Input Multiplier | Output Multiplier |
|-------|-----------|------------------|-------------------|
| Sonnet 4.6 | >200K tokens | 2x ($6.00) | 1.5x ($22.50) |
| Gemini 3.1 Pro | >200K tokens | 2x ($4.00) | 1.5x ($18.00) |
| GPT-5.4 | >272K tokens | 2x ($5.00) | 1.5x ($22.50) |
| Opus 4.6 | None | Standard rate at all sizes | Standard rate |

Note: Opus 4.6 is the only frontier model with flat pricing across the full 1M context window -- no long-context surcharge. This makes it the best value for large-context reasoning tasks, despite the higher base rate.

Source: https://claude.com/blog/1m-context-ga

### Feature Comparison

| Feature | Claude Code | Gemini CLI | Codex CLI |
|---------|------------|------------|-----------|
| **MCP Support** | Native | Native (stdio + SSE) | Native (stdio) |
| **Prompt Caching** | cache_control (5m/1h TTL) | cachedContents (explicit) + implicit | Automatic prefix (no config) |
| **Batch API** | 50% off, 24h SLA | 50% off, 24h SLA | 50% off, 24h SLA |
| **Streaming** | stream-json | stream-json | NDJSON (--json) |
| **Session Resume** | --resume | --resume | exec resume |
| **Extended Thinking** | Adaptive thinking (auto) | Thinking budget parameter | Reasoning effort parameter |
| **Computer Use** | Yes (Q1 2026) | No | No |
| **Subagents** | Built-in (Explore, Plan, General) | No | No |
| **Worktrees** | --worktree flag, native | No | Workspace sandbox |
| **Hooks** | PreToolUse, PostToolUse, etc. | No | No |
| **Skills/Slash Commands** | /batch, /simplify, /loop, etc. | MCP prompt commands | /mode command |
| **Plan Mode** | Dedicated planning agent | No | Suggest mode (approval-based) |
| **Sandbox Modes** | Permission-based | N/A | suggest / auto-edit / full-auto |
| **Max Context** | 1M tokens | 2M tokens | 1M tokens (model-dependent) |
| **Compaction** | Server-side auto-summarization | N/A | N/A |

Sources:
- Claude Code features: https://code.claude.com/docs/en/sub-agents
- Gemini CLI MCP: https://geminicli.com/docs/tools/mcp-server/
- Codex CLI features: https://developers.openai.com/codex/cli/features

## 2. Optimal Cascade Configuration

### Current ralphglasses Routing (from PROVIDER-SETUP.md)

```
Tier 1 (Ultra-cheap)  -> Gemini 2.5 Flash-Lite  $0.10/$0.40
Tier 2 (Worker)       -> Gemini 2.5 Flash        $0.30/$1.25
Tier 3 (Coding)       -> Claude Sonnet 4.6        $3.00/$15.00
Tier 4 (Reasoning)    -> Claude Opus 4.6          $5.00/$25.00  (was $15/$75)
```

### Recommended Optimized Cascade (April 2026)

Given the current pricing landscape, the cascade should be restructured to exploit the Opus 4.6 price collapse and new ultra-cheap options:

```
Tier 0 (Classification/Routing)
  -> GPT-4.1-nano ($0.05/$0.20) or Gemini 2.5 Flash-Lite ($0.10/$0.40)
  Use case: Task classification, confidence scoring, intent detection
  Confidence threshold: N/A (always runs first)

Tier 1 (Bulk Worker)
  -> Gemini 2.5 Flash ($0.30/$2.50) or Gemini 3 Flash ($0.50/$3.00)
  Use case: Test generation, documentation, simple refactors, boilerplate
  Escalation trigger: Confidence < 0.7 OR task complexity > "simple"

Tier 2 (Coding)
  -> Claude Sonnet 4.6 ($3.00/$15.00) or GPT-5.4 Codex ($2.50/$15.00)
  Use case: Architecture, complex refactoring, multi-file changes
  Escalation trigger: Confidence < 0.6 OR compilation failure OR >2 retries

Tier 3 (Reasoning/Planning)
  -> Claude Opus 4.6 ($5.00/$25.00) or Gemini 3.1 Pro ($2.00/$12.00)
  Use case: Multi-step planning, cross-repo reasoning, debugging complex issues
  Escalation trigger: Only on explicit planner invocation or critical-path tasks
```

### Key Changes from Current Config

1. **Add Tier 0 classifier**: GPT-4.1-nano at $0.05/1M is 2x cheaper than Flash-Lite for pure classification. A 50-token classification call costs $0.0000025 -- effectively free. This replaces hardcoded routing rules with learned routing.

2. **Gemini 3.1 Pro as Opus alternative**: At $2.00/$12.00, Gemini 3.1 Pro scores within 0.2 points of Opus 4.6 on SWE-bench Verified (80.6% vs 80.8%) at 60% lower cost. For planning tasks that don't require Claude-specific features (adaptive thinking, subagents), route to Gemini 3.1 Pro first.

3. **Codex CLI as default worker**: GPT-5.4 at $2.50/$15.00 is competitive with Sonnet 4.6 at $3.00/$15.00 for coding tasks. The codex-mini-latest model at $1.50/$6.00 fills a gap between Flash ($0.50/$3.00) and Sonnet ($3.00/$15.00).

4. **Opus 4.6 as last resort only**: The 67% price drop from $15/$75 to $5/$25 makes Opus affordable for the first time, but it should still be reserved for tasks where its capabilities are clearly needed -- multi-step reasoning, cross-repo planning, complex debugging.

### Confidence-Based Escalation

Research on unified routing/cascading frameworks (Dekoninck et al., arXiv:2410.10347) shows that confidence-based escalation achieves 87% cost reduction by routing ~90% of queries to cheaper models. Implementation:

```
confidence = classifier.score(task)
if confidence >= 0.85:  route to Tier 1 (Gemini Flash)
if confidence >= 0.70:  route to Tier 2 (Sonnet/Codex)
if confidence >= 0.50:  route to Tier 3 (Opus/Gemini Pro)
if confidence < 0.50:   flag for human review
```

Source: https://arxiv.org/abs/2410.10347

## 3. Cost Reduction Projections

### Current Baseline

From the deep-dive analysis (08-ralph-deep-dive.md):
- Average task cost: $0.17
- Total observed spend: $6.21 across 36 tasks
- P50 loop cost: $0.0553
- P95 loop cost: $0.2841
- Provider distribution: 100% Claude
- Projected monthly at 20 tasks/week: ~$18-24/month

### Projected Savings by Strategy

| Strategy | Estimated Savings | New Avg Cost | Implementation Effort |
|----------|------------------|-------------|----------------------|
| **Prompt caching (all providers)** | 40-60% on input tokens | $0.10-0.12 | Low (partially done) |
| **Cascade routing (4-tier)** | 50-70% overall | $0.06-0.09 | Medium (Phase 24) |
| **Batch API for sweeps** | 50% on batch-eligible | $0.12-0.14 | Low (internal/batch exists) |
| **Model migration (Opus -> Sonnet)** | 40% on reasoning tasks | $0.12-0.14 | Low (config change) |
| **Combined (all strategies)** | 70-85% overall | $0.03-0.05 | Medium-High |

### Detailed Breakdown

**Prompt Caching** (achievable now):
- Current: 100% standard input pricing
- With caching: Cache writes at 1.25x, reads at 0.1x base price
- For a typical session with stable system prompt + tool definitions (~15K tokens):
  - First call: 15K * $3.00/1M = $0.045 (write) -> $0.056 at 1.25x
  - Subsequent calls: 15K * $0.30/1M = $0.0045 (read at 0.1x)
  - Break-even: After 1 cache read (5-min TTL) or 2 reads (1-hour TTL)
  - For a 10-turn session: ~$0.045 + 9 * $0.0045 = $0.085 vs $0.45 uncached = **81% savings on input**

**Cascade Routing** (Phase 24):
- If 70% of tasks are routable to Gemini Flash ($0.30/$2.50):
  - 70% * $0.03 + 20% * $0.17 + 10% * $0.30 = $0.085 avg vs $0.17 current = **50% savings**
- With confidence-based escalation tuned over time, the cheap-model percentage can reach 85-90%

**Batch API** (applicable to sweeps and fleet ops):
- Sweep tasks are inherently batch-compatible (non-interactive, parallelizable)
- At 50% discount: sweep cost drops from ~$0.17 to ~$0.085 per task
- For a 44-repo sweep: $7.48 -> $3.74 savings per sweep run

**Combined Projection at Scale** (20 tasks/week):
- Current: 80 tasks/month * $0.17 = $13.60/month
- Optimized: 80 tasks/month * $0.04 = $3.20/month
- Annual savings: ~$125 (modest scale), scales linearly with volume

## 4. Prompt Caching Deep Dive

### Provider-by-Provider Analysis

#### Anthropic (Claude)

| Parameter | Value |
|-----------|-------|
| Mechanism | `cache_control` breakpoints on message content blocks |
| TTL Options | 5 minutes (1.25x write cost), 1 hour (2x write cost) |
| Cache Read Discount | 90% (0.1x base input price) |
| Min Cacheable | 1,024 tokens (Haiku), 2,048 tokens (Sonnet/Opus) |
| Isolation | Workspace-level (changed Feb 5, 2026 from org-level) |
| Stacking | Combines with Batch API (50% off) for up to 95% savings |
| Auto-caching | Top-level `cache_control` field for automatic placement |

Best practice for ralphglasses: Place `cache_control` breakpoints on:
1. System prompt (stable across session)
2. Tool definitions (126 tools, ~50K tokens -- high-value cache target)
3. CLAUDE.md content (stable per-repo)

The 1-hour TTL is preferred for marathon/supervisor sessions that run 30+ minutes. The 5-minute TTL is better for burst sessions with rapid successive calls.

**Cache risk note**: Resumed Claude sessions should be treated as cache-unsafe for budget estimation until live cache reads are confirmed (per CODEX-REFERENCE.md).

Sources:
- https://platform.claude.com/docs/en/build-with-claude/prompt-caching
- https://blog.wentuo.ai/en/claude-code-prompt-caching-ttl-pricing-guide-en.html

#### Google (Gemini)

| Parameter | Value |
|-----------|-------|
| Explicit Caching | `cachedContents` API with configurable TTL |
| Implicit Caching | Automatic, no configuration needed (Gemini 2.5+) |
| Cache Read Discount | 75% (Gemini 2.0), 90% (Gemini 2.5+) |
| Storage Cost | $0.00025 per 1K characters per hour (explicit only) |
| Min Cacheable | 1,024 tokens (Flash), 4,096 tokens (Pro) |
| Implicit Storage | Free (no storage cost for automatic caching) |
| Stacking | Combines with Batch API (50% off) |

Best practice for ralphglasses: Use implicit caching for standard sessions (zero config, automatic). Use explicit `cachedContents` for fleet operations where the same system prompt hits multiple workers -- the storage cost is negligible ($0.00025/1K chars/hr) compared to re-processing savings.

The 90% discount on Gemini 2.5+ models makes cached Gemini Flash extremely competitive: $0.03/1M cached input tokens is the cheapest cached rate across all providers.

Sources:
- https://ai.google.dev/gemini-api/docs/caching
- https://developers.googleblog.com/en/gemini-2-5-models-now-support-implicit-caching/

#### OpenAI

| Parameter | Value |
|-----------|-------|
| Mechanism | Automatic prefix caching (no explicit API) |
| Discount | 50% on most models, 75% on codex-mini-latest |
| Min Prefix | 1,024 tokens, then 128-token increments |
| TTL | Approximately 5-10 minutes (not configurable) |
| Configuration | None required -- fully automatic |
| Stacking | Combines with Batch API (50% off) for up to 87.5% savings |

Best practice for ralphglasses: Structure prompts with stable prefixes (system prompt, tool definitions) before variable content. The automatic nature means zero implementation effort, but the shorter implicit TTL and lower discount rate (50% vs 90% for Claude/Gemini) make it less effective for cost optimization. The 75% discount on codex-mini-latest is a notable exception.

Sources:
- https://developers.openai.com/api/docs/guides/prompt-caching
- https://openai.com/index/api-prompt-caching/

### Cross-Provider Caching Comparison

| Provider | Read Discount | Write Premium | TTL Control | Storage Cost | Auto Mode |
|----------|--------------|---------------|-------------|-------------|-----------|
| Claude | 90% | 25% (5m) / 100% (1h) | Yes (5m/1h) | None | Yes (top-level) |
| Gemini | 75-90% | Standard rate | Yes (explicit) | $0.00025/1K/hr | Yes (implicit) |
| OpenAI | 50-75% | None | No | None | Always on |

Winner for caching economics: **Gemini 2.5+** -- 90% discount with free implicit caching and no write premium. Claude matches the 90% read discount but adds a write premium. OpenAI is the simplest (zero config) but offers the lowest discount.

## 5. Batch API Opportunity

### Eligible ralphglasses Workloads

| Workload | Current Pattern | Batch Eligible | Estimated Volume | Savings |
|----------|----------------|----------------|-----------------|---------|
| **Sweeps** (44-repo) | Sequential session_launch | Yes -- fully | 44 tasks/sweep | 50% ($3.74/sweep) |
| **Fleet code review** | Parallel session_launch | Yes -- fully | Variable | 50% |
| **Test generation** | Interactive sessions | Yes -- bulk gen | 10-20 tasks/batch | 50% |
| **Documentation** | Interactive sessions | Yes -- bulk gen | 5-10 tasks/batch | 50% |
| **Self-improvement** | Supervisor loop | Partially -- planning only | 2-5 tasks/cycle | 50% on eligible |
| **Prompt enhancement** | Real-time pipeline | No -- latency-sensitive | N/A | N/A |
| **Session planning** | Real-time | No -- latency-sensitive | N/A | N/A |

### Batch API Implementation

ralphglasses already has `internal/batch/` with endpoints for all three providers:

```
Claude:  POST /v1/messages/batches   (up to 10,000 requests)
Gemini:  BatchGenerateContent         (server-side batching)
OpenAI:  POST /v1/batches             (JSONL upload, async completion)
```

The `ralphglasses_fleet_submit` tool supports `batch: true`. The primary opportunity is making batch mode the default for sweep operations, which are inherently non-interactive and currently the highest-volume workload.

### Combined Savings (Batch + Caching)

| Provider | Standard | Cached | Batched | Cached + Batched |
|----------|----------|--------|---------|-----------------|
| Claude Sonnet 4.6 (input) | $3.00 | $0.30 | $1.50 | $0.15 |
| Gemini 2.5 Flash (input) | $0.30 | $0.03 | $0.15 | $0.015 |
| OpenAI GPT-4.1-mini (input) | $0.40 | $0.20 | $0.20 | $0.10 |

The combined cached+batched rate for Gemini Flash input ($0.015/1M tokens) is 200x cheaper than standard Claude Sonnet input. For bulk sweep operations that can tolerate 24-hour latency, this is the optimal path.

Source: https://platform.claude.com/docs/en/about-claude/pricing

## 6. Benchmarks

### SWE-bench Verified (March 2026)

| Rank | Model | Score | Input $/1M | Cost-Efficiency Ratio |
|------|-------|-------|-----------|----------------------|
| 1 | Claude Opus 4.5 | 80.9% | $5.00 | 16.2% per $ |
| 2 | Claude Opus 4.6 | 80.8% | $5.00 | 16.2% per $ |
| 3 | Gemini 3.1 Pro | 80.6% | $2.00 | **40.3% per $** |
| 4 | Claude Sonnet 4.6 | 79.6% | $3.00 | 26.5% per $ |
| 5 | GPT-5.4 | ~80.0% | $2.50 | 32.0% per $ |

### SWE-bench Pro (Harder, March 2026)

| Rank | Model | Score |
|------|-------|-------|
| 1 | GPT-5.4 | 57.7% |
| 2 | Gemini 3.1 Pro | 54.2% |
| 3 | Claude Opus 4.5 | 45.9% |
| 4 | Claude Sonnet 4.5 | 43.6% |

### Takeaways for ralphglasses

1. **Gemini 3.1 Pro is the best cost/performance ratio** on SWE-bench Verified: 80.6% at $2.00 input vs Opus 4.6's 80.8% at $5.00. For most coding tasks, this 0.2-point gap is not worth a 2.5x price premium.

2. **GPT-5.4 leads on harder tasks** (SWE-bench Pro 57.7%), suggesting it should be the escalation target for complex debugging and multi-step reasoning, not Opus.

3. **Sonnet 4.6 is the sweet spot for interactive coding**: 79.6% on SWE-bench at $3.00 input -- within 1.2 points of Opus for 60% of the cost. This validates its position as the primary Tier 2 model.

4. **For cost-optimized sweeps**, Gemini 3 Flash ($0.50/$3.00) provides good-enough performance for boilerplate tasks at 6x lower cost than Sonnet.

Sources:
- https://www.vals.ai/benchmarks/swebench
- https://www.marc0.dev/en/leaderboard
- https://smartscope.blog/en/generative-ai/chatgpt/llm-coding-benchmark-comparison-2026/

## 7. Emerging Capabilities

### Prioritized for Phase 10+ Integration

#### 1. Adaptive Thinking (Available Now)

Claude Opus 4.6 and Sonnet 4.6 support `thinking: {type: "adaptive"}`, replacing the deprecated `budget_tokens` approach. The model dynamically decides when and how much to think based on query complexity and effort level. This maps directly to ralphglasses' `--effort` flag.

**Action**: Verify adaptive thinking is wired through session launch. Map `--effort low/medium/high/max` to Claude's effort parameter. Consider exposing thinking token usage in cost tracking.

Source: https://platform.claude.com/docs/en/build-with-claude/extended-thinking

#### 2. Computer Use (Available in Claude Code Q1 2026)

Claude can interact with graphical interfaces: clicking buttons, filling forms, reading visual content. Use cases for ralphglasses:
- Automated browser testing of web applications
- Visual regression testing
- GUI-based tool interaction
- Screenshot-based debugging

**Action**: Evaluate for Phase 5 (Agent Sandboxing). Computer use requires screenshot capture and action execution -- the sandbox infrastructure must support this.

Source: https://www.mindstudio.ai/blog/claude-code-q1-2026-update-roundup

#### 3. Claude Subagents (Available Now)

Built-in subagent types: Explore (fast, read-only, Haiku-powered), Plan (research-focused), General (full tool access). Custom subagents via Markdown files with YAML frontmatter.

**Action**: Replace ralphglasses' custom planner invocation with Claude's native Plan subagent. This could fix the persistent "planner output is empty" and "planner failed to produce valid json" issues (25+ occurrences in learned avoidance rules).

Source: https://code.claude.com/docs/en/sub-agents

#### 4. Compaction (Available Now, Claude Only)

Server-side automatic context summarization when approaching the context window limit. Enables effectively infinite conversations without manual context management.

**Action**: Enable compaction for marathon/supervisor sessions. This directly supports Phase 13 (Level 3 Autonomy) goal of 72-hour unattended operation.

Source: https://platform.claude.com/docs/en/about-claude/models/whats-new-claude-4-6

#### 5. Codex CLI Full-Auto + Sandbox (Available Now)

`codex exec --full-auto --sandbox workspace-write` enables fully autonomous execution with filesystem isolation. Non-interactive mode supports prompt+stdin piping.

**Action**: This is already the default C2 runtime. Verify `codex exec` integration uses workspace-write sandbox for sweep operations. The `--full-auto` mode is the correct default for fleet workers.

Source: https://developers.openai.com/codex/noninteractive

#### 6. Multi-Modal Code Understanding (Emerging)

All three providers now support image input, enabling:
- Screenshot-to-code workflows
- Visual diff review
- Architecture diagram understanding
- Error screenshot diagnosis

**Action**: Low priority for Phase 10+, but worth integrating into the prompt enhancement pipeline for tasks that include visual context.

#### 7. Git-Native Worktrees (Claude Code)

Claude Code's `--worktree` flag creates isolated git worktrees per session, solving the branch contention problem for parallel fleet workers.

**Action**: Already supported. Verify that fleet workers consistently use worktrees for isolation. This is critical for Phase 7 (Kubernetes fleet) where multiple workers may target the same repo.

Source: https://botmonster.com/posts/claude-md-productivity-stack-custom-commands-git-worktrees-agent-rules/

## 8. Provider Risk Assessment

### Anthropic (Claude)

| Factor | Assessment | Risk Level |
|--------|-----------|------------|
| **Pricing stability** | Aggressive price reductions (67% drop Opus 4.1->4.6). Trend is strongly downward. | Low (favorable) |
| **Model deprecation** | Haiku 3 retired April 19, 2026. Sonnet 3.5/3.7 already deprecated. ~12 month lifecycle. | Medium |
| **API stability** | Cache isolation changed Feb 2026 (workspace-level). Source map leak March 2026. | Medium |
| **Feature velocity** | Highest: subagents, hooks, skills, worktrees, compaction, computer use all shipped Q1 2026. | Low (favorable) |
| **Vendor lock-in risk** | Subagents, hooks, skills, CLAUDE.md are Claude-specific. No cross-provider standard. | High |
| **Rate limits** | Tier-based. Max plan users get higher limits. Can be restrictive at scale. | Medium |

**Mitigation**: Multi-provider architecture (already implemented) is the primary hedge. Keep Claude-specific features (subagents, hooks) wrapped behind provider-agnostic interfaces. Maintain fallback models for all tiers.

Source: https://northflank.com/blog/claude-rate-limits-claude-code-pricing-cost

### Google (Gemini)

| Factor | Assessment | Risk Level |
|--------|-----------|------------|
| **Pricing stability** | Consistently cheapest. Free tier for Flash models. Aggressive discounting. | Low (favorable) |
| **Model deprecation** | Gemini 1.5 still available alongside 2.5 and 3.x. Longer support windows. | Low |
| **API stability** | Vertex AI + AI Studio dual paths. API surface is stable. | Low |
| **Feature velocity** | Moderate: MCP support, implicit caching, batch API. Slower than Claude on agent features. | Medium |
| **Vendor lock-in risk** | Low -- standard MCP, no CLI-specific features that create lock-in. | Low |
| **Context window** | 2M tokens (largest) -- valuable for large codebases. | Low (favorable) |

**Mitigation**: Gemini's low risk profile makes it the ideal default worker tier. The lack of advanced agent features (no subagents, no hooks) limits it to execution-focused roles.

Sources:
- https://ai.google.dev/gemini-api/docs/pricing
- https://developers.googleblog.com/en/gemini-2-5-models-now-support-implicit-caching/

### OpenAI (Codex CLI)

| Factor | Assessment | Risk Level |
|--------|-----------|------------|
| **Pricing stability** | Moderate. Codex pricing changed to per-token April 2, 2026 (was per-message). | Medium |
| **Model deprecation** | o3/o4-mini are current. GPT-4.1 family is stable. codex-mini-latest may be volatile. | Medium |
| **API stability** | Responses API is the current standard. Chat Completions being phased down. | Medium |
| **Feature velocity** | Codex CLI relaunch + open-source. Full-auto sandbox. Plugin system. | Medium |
| **Vendor lock-in risk** | Codex CLI is open-source. API is standard REST. Low lock-in. | Low |
| **Grant initiative** | $1M in API credits for Codex CLI projects. Worth applying. | Low (favorable) |

**Mitigation**: The open-source Codex CLI reduces vendor risk. The pricing model change (per-message to per-token) was disruptive but now aligns with industry standard. The $25K grant program is worth pursuing for ralphglasses.

Sources:
- https://developers.openai.com/codex/pricing
- https://openai.com/index/introducing-codex/

### Cross-Provider Risk Summary

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| Provider outage | Medium | High | Fallback model routing (--fallback-model) |
| Price increase | Low | Medium | Multi-provider arbitrage, batch/cache optimization |
| Model deprecation | High (annual) | Medium | Pin model versions, automated migration testing |
| API breaking change | Medium | High | Provider abstraction layer, integration tests |
| Rate limiting at scale | High | Medium | Distribute load across providers, batch API |

## 9. Recommendations

### Priority 1: Immediate (This Sprint)

1. **Enable prompt caching across all providers** -- The system already has cache_control support. Verify all three providers have caching active. Expected savings: 40-60% on input tokens. Cost: zero (configuration only).

2. **Default sweeps to batch mode** -- The `internal/batch/` package exists. Make `batch: true` the default for `ralphglasses_sweep_launch`. Expected savings: 50% on sweep costs. Cost: low (config change + testing).

3. **Apply for OpenAI Codex grant** -- $25,000 in API credits for qualifying projects. ralphglasses is a strong candidate (multi-provider orchestration, open-source). Cost: a few hours for application.

### Priority 2: Near-Term (Phases 10-10.5)

4. **Add GPT-4.1-nano as Tier 0 classifier** -- At $0.05/1M tokens, a classification call is effectively free. Use it to route tasks before hitting expensive models. Expected savings: 20-30% from better routing. Cost: medium (new routing logic).

5. **Integrate Gemini 3.1 Pro as reasoning alternative** -- 80.6% SWE-bench at $2.00 input vs Opus 4.6 at $5.00. Route non-Claude-specific reasoning tasks to Gemini 3.1 Pro. Expected savings: 15-25% on reasoning tasks. Cost: medium (provider config + testing).

6. **Replace custom planner with Claude native subagents** -- The Plan subagent may resolve the persistent JSON parsing failures (25 occurrences). Test against the current planner pipeline. Cost: medium (integration work).

7. **Enable compaction for marathon sessions** -- Direct support for 72-hour unattended operation (Phase 13 prerequisite). Cost: low (API flag).

### Priority 3: Medium-Term (Phases 13-15)

8. **Implement full cascade routing (Phase 24)** -- Unified routing+cascading with confidence-based escalation. Target: 70-85% cost reduction. This is the single largest cost optimization opportunity. Cost: high (Phase 24 is 10 tasks).

9. **Evaluate codex-mini-latest as default Tier 2** -- At $1.50/$6.00, it fills the gap between Flash and Sonnet. Benchmark against Sonnet 4.6 on ralphglasses-specific tasks. Cost: medium (evaluation + integration).

10. **Computer use integration for visual testing** -- Requires Phase 5 sandbox infrastructure. Long-term capability for UI-heavy repos. Cost: high (Phase 5 dependency).

### Cost Optimization Roadmap

```
April 2026:  Enable caching + batch defaults        -> $0.17 -> $0.10 avg (-41%)
May 2026:    Add Tier 0 classifier + Gemini 3.1 Pro -> $0.10 -> $0.07 avg (-30%)
June 2026:   Cascade routing v1                      -> $0.07 -> $0.05 avg (-29%)
Q3 2026:     Full cascade + combined optimizations   -> $0.05 -> $0.03 avg (-40%)
```

Cumulative: **$0.17 -> $0.03 per task (82% reduction)** at stable volume. At 80 tasks/month, this is $13.60 -> $2.40/month.

---

## Sources

### Official Provider Documentation
- [Anthropic Pricing](https://platform.claude.com/docs/en/about-claude/pricing)
- [Anthropic Prompt Caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching)
- [Anthropic Extended Thinking](https://platform.claude.com/docs/en/build-with-claude/extended-thinking)
- [Anthropic What's New in Claude 4.6](https://platform.claude.com/docs/en/about-claude/models/whats-new-claude-4-6)
- [Claude 1M Context GA](https://claude.com/blog/1m-context-ga)
- [Claude Code Subagents](https://code.claude.com/docs/en/sub-agents)
- [Google Gemini Pricing](https://ai.google.dev/gemini-api/docs/pricing)
- [Google Context Caching](https://ai.google.dev/gemini-api/docs/caching)
- [Google Vertex AI Pricing](https://cloud.google.com/vertex-ai/generative-ai/pricing)
- [Gemini Implicit Caching](https://developers.googleblog.com/en/gemini-2-5-models-now-support-implicit-caching/)
- [Gemini CLI MCP](https://geminicli.com/docs/tools/mcp-server/)
- [OpenAI Pricing](https://developers.openai.com/api/docs/pricing)
- [OpenAI Prompt Caching](https://developers.openai.com/api/docs/guides/prompt-caching)
- [OpenAI Batch API](https://platform.openai.com/docs/guides/batch)
- [OpenAI Codex CLI](https://developers.openai.com/codex/cli)
- [OpenAI Codex CLI Features](https://developers.openai.com/codex/cli/features)
- [OpenAI Codex Pricing](https://developers.openai.com/codex/pricing)
- [OpenAI Introducing Codex](https://openai.com/index/introducing-codex/)
- [OpenAI o3 and o4-mini](https://openai.com/index/introducing-o3-and-o4-mini/)

### Benchmarks & Analysis
- [SWE-bench Leaderboard](https://www.vals.ai/benchmarks/swebench)
- [SWE-bench Verified March 2026](https://www.marc0.dev/en/leaderboard)
- [LLM Coding Benchmark Comparison 2026](https://smartscope.blog/en/generative-ai/chatgpt/llm-coding-benchmark-comparison-2026/)
- [Best AI for Coding 2026](https://www.morphllm.com/best-ai-model-for-coding)

### Cost Optimization Research
- [Unified Routing and Cascading for LLMs (arXiv:2410.10347)](https://arxiv.org/abs/2410.10347)
- [AI Agent Cost Optimization: Token Economics](https://zylos.ai/research/2026-02-19-ai-agent-cost-optimization-token-economics)
- [LLM Cost Optimization 2026: Routing, Caching, Batching](https://www.maviklabs.com/blog/llm-cost-optimization-2026)
- [AI Agent Cost Optimization Guide 2026](https://moltbook-ai.com/posts/ai-agent-cost-optimization-2026)

### Provider Stability & Risk
- [Claude Code Rate Limits & Pricing](https://northflank.com/blog/claude-rate-limits-claude-code-pricing-cost)
- [Claude Code Q1 2026 Feature Roundup](https://www.mindstudio.ai/blog/claude-code-q1-2026-update-roundup)
- [Claude Code Prompt Caching TTL Guide](https://blog.wentuo.ai/en/claude-code-prompt-caching-ttl-pricing-guide-en.html)
- [Gemini API Batch vs Caching Guide](https://yingtu.ai/en/blog/gemini-api-batch-vs-caching)
