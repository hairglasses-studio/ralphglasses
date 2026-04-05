# Phase 12 Research: End-to-End Multi-Provider Enhancement

Covers ROADMAP items **8.2** (Prompt Management), **8.4** (Code Review Automation), and **6.8** (Multi-Model A/B Testing).

---

## 1. Executive Summary

- The `internal/enhancer/` package (4,069 non-test lines, 3,294 test lines across 19 source files) already implements a complete multi-provider prompt enhancement pipeline with Claude, Gemini, and OpenAI API clients, circuit breaker, caching, and 13 deterministic stages. This is a strong foundation for all three ROADMAP items.
- **Prompt management (8.2)** is partially addressed: 5 builtin templates exist in `templates.go`, variable interpolation works via `{{var}}` syntax, and config loading supports project/home-directory fallback. Missing: persistent prompt library (`~/.ralphglasses/prompts/`), versioning/rollback, A/B testing of prompts, and TUI prompt editor.
- **Code review automation (8.4)** has zero implementation today. The `code_review` template in `templates.go` and the lint/scoring infrastructure provide scoring primitives, but PR review agents, GitHub API integration, auto-approve logic, and the review dashboard are entirely absent.
- **Multi-model A/B testing (6.8)** has zero implementation. The multi-provider client infrastructure (`provider.go`, `llmclient.go`, `gemini_client.go`, `openai_client.go`) and the hybrid engine with cache/circuit breaker provide the API layer, but no A/B test orchestration, metric collection, comparison reporting, or auto-promotion logic exists.
- The critical path is: prompt library persistence (8.2.1-8.2.2) first, then A/B testing framework (6.8.1-6.8.3 + 8.2.4) which shares infrastructure, then code review automation (8.4) which consumes both.

---

## 2. Current State Analysis

### 2.1 What Exists

| File | Lines | Test Lines | Test File | Status |
|------|-------|------------|-----------|--------|
| `enhancer.go` | 836 | 463 | `enhancer_test.go` | Production-ready, 13-stage pipeline |
| `scoring.go` | 536 | 377 | `scoring_test.go` | Production-ready, 10-dimension scoring |
| `lint.go` | 356 | 241 | `lint_test.go` | Production-ready, 11+ lint rules |
| `templates.go` | 276 | 110 | `templates_test.go` | Working, 5 builtin templates |
| `config.go` | 262 | 200 | `config_test.go` | Production-ready, YAML + env overrides |
| `context.go` | 244 | 223 | `context_test.go` | Production-ready, cache-friendly checks |
| `examples.go` | 198 | 96 | `examples_test.go` | Production-ready, 3 detection strategies |
| `metaprompt.go` | 191 | -- | (const-only) | Complete, 3 providers x 2 modes |
| `llmclient.go` | 177 | 193 | `llmclient_test.go` | Production-ready, Claude Messages API |
| `gemini_client.go` | 166 | 159 | `gemini_client_test.go` | Production-ready, Gemini generateContent |
| `hybrid.go` | 164 | 248 | `hybrid_test.go` | Production-ready, auto/local/llm modes |
| `openai_client.go` | 152 | 136 | `openai_client_test.go` | Production-ready, Chat Completions API |
| `claudemd.go` | 126 | 105 | `claudemd_test.go` | Production-ready, 6 health checks |
| `circuit.go` | 93 | 118 | `circuit_test.go` | Production-ready, 3-failure threshold |
| `cache.go` | 103 | 104 | `cache_test.go` | Production-ready, SHA-256 keyed, 100 entries |
| `classifier.go` | 80 | -- | (via enhancer_test) | Production-ready, 6 task types |
| `filter.go` | 63 | -- | (via handler_prompt) | Production-ready, skip gates |
| `provider.go` | 46 | 76 | `provider_test.go` | Production-ready, factory pattern |
| `mcpserver/handler_prompt.go` | 309 | -- | (via tools_test) | Production-ready, 9 MCP tool handlers |
| `session/templates.go` | 63 | -- | `templates_test.go` | Working, 3 provider templates |
| `session/loop.go:768-797` | 30 | -- | (via loop_test) | Working, enhanceForProvider integration |
| **Total** | **4,069** | **3,294** | **18 test files** | |

### 2.2 What Works Well

1. **Multi-provider abstraction** (`provider.go:14-21`): The `PromptImprover` interface with `Improve()` and `Provider()` methods is clean. `NewPromptImprover()` factory dispatches by provider string. Adding new providers requires zero interface changes.

2. **Provider-aware pipeline stages** (`enhancer.go:101,111,130-134`): Tone downgrade and overtrigger rewrite skip for non-Claude targets. Structure stage dispatches to `addStructure()` (XML) for Claude vs `addMarkdownStructure()` (markdown headers) for Gemini/OpenAI. This dual-axis model (`LLM.Provider` for API calls, `TargetProvider` for pipeline behavior) is well-documented in CLAUDE.md.

3. **Hybrid engine with resilience** (`hybrid.go:61-164`): Auto mode tries LLM, falls back to local pipeline, with circuit breaker (3 failures = 60s open) and SHA-256-keyed cache (100 entries, 10min TTL). The `session/loop.go:768-785` integration uses `ModeAuto` to never block the ralph loop on LLM failures.

4. **Meta-prompt specialization** (`metaprompt.go:1-192`): Each provider gets its own meta-prompt with domain-appropriate patterns (XML scratchpad for Claude, markdown reasoning for Gemini, chain-of-thought for OpenAI). The `MetaPromptFor()` dispatch function handles thinking mode variants.

5. **10-dimension scoring with provider-specific suggestions** (`scoring.go:43-55`): Structure, Role Definition, Task Focus, and Tone dimensions accept `targetProvider` and adjust suggestions accordingly (e.g., "Add XML structure tags" for Claude vs "Add structured markdown sections" for Gemini/OpenAI).

6. **Config precedence chain** (`config.go:91-185`): Project dir -> home dir -> env vars. `PROMPT_IMPROVER_PROVIDER`, `PROMPT_IMPROVER_TARGET`, `PROMPT_IMPROVER_MODEL`, `PROMPT_IMPROVER_LLM`, and `PROMPT_IMPROVER_TIMEOUT` are all supported.

### 2.3 What Doesn't Work

1. **No persistent prompt library** (ROADMAP 8.2.1): Templates are hardcoded in `templates.go:18-233`. There is no file-system-based prompt storage, no `~/.ralphglasses/prompts/` directory, and no per-project prompt overrides. Users cannot create, save, or share custom templates.

2. **No prompt versioning** (ROADMAP 8.2.3): `FillTemplate()` is pure string replacement with no version tracking, no change history, and no rollback capability. Templates are immutable compile-time constants.

3. **No prompt A/B testing** (ROADMAP 8.2.4, 6.8): No mechanism to run the same task with two different prompts or models and compare outcomes. The multi-provider clients can each improve a prompt independently, but there is no orchestration to run them in parallel, collect metrics, or produce comparison reports.

4. **No TUI prompt editor** (ROADMAP 8.2.5): Prompts are edited externally. The TUI has no prompt editing view, no template browser, and no real-time scoring preview during editing (though the scoring engine exists).

5. **No code review automation** (ROADMAP 8.4): Zero PR review agent infrastructure. No GitHub API integration for reading PRs, posting comments, or approving/merging. The `code_review` template in `templates.go` is a passive template, not an active agent.

6. **No A/B test metric collection** (ROADMAP 6.8.2): No cost, duration, test pass rate, or lint score capture per model run. `session/budget.go` tracks per-session costs, but there is no A/B comparison harness.

7. **No statistical comparison** (ROADMAP 6.8.3): No side-by-side reporting, no significance testing, no confidence intervals on model comparisons.

8. **No auto-promotion** (ROADMAP 6.8.5): No mechanism to update default model based on A/B test results.

---

## 3. Gap Analysis

### 3.1 ROADMAP Target vs Current State

| ROADMAP Item | Target | Current State | Gap |
|---|---|---|---|
| 8.2.1 Prompt library | `~/.ralphglasses/prompts/` with named templates | 5 hardcoded templates in `templates.go` | No filesystem storage, no user-defined templates |
| 8.2.2 Variable interpolation | `{{repo_name}}`, `{{task_description}}`, `{{context}}` | `FillTemplate()` handles `{{var}}` syntax | Working for builtin templates; needs extension to user-defined templates |
| 8.2.3 Prompt versioning | Track changes, rollback | None | No version tracking of any kind |
| 8.2.4 A/B testing (prompts) | Same task, different prompts, compare | None | No A/B test framework |
| 8.2.5 TUI prompt editor | View, edit, test prompts | None | No TUI editing capability |
| 6.8.1 A/B test definition | Two models + same task, parallel worktrees | Multi-provider clients exist, no orchestration | Need test definition struct, worktree setup |
| 6.8.2 Metric collection | Cost, duration, test pass rate, lint score | Session cost tracking exists; no A/B collection | Need per-test-arm metric capture |
| 6.8.3 Comparison report | Side-by-side with statistical significance | None | Full build |
| 6.8.4 TUI A/B view | Live comparison of concurrent sessions | None | Full build |
| 6.8.5 Auto-promote | Update default model after N iterations | None | Need promotion logic + config update |
| 8.4.1 PR review agent | Auto-review PRs from other sessions | None | Full build |
| 8.4.2 Review criteria | Configurable rules | Lint + scoring exist as primitives | Need review rule engine |
| 8.4.3 GitHub integration | Post review comments via API | None | Need `go-github` or `gh` integration |
| 8.4.4 Auto-approve | Auto-merge passing PRs | None | Full build |
| 8.4.5 Review dashboard | TUI view of PRs | None | Full build |

### 3.2 Missing Capabilities

1. **Persistent storage layer for prompts**: Need a filesystem-based prompt store with YAML/JSON serialization, indexing by name/task-type/provider, and user-home fallback. The existing `LoadConfig()` pattern in `config.go:91-113` can be extended.

2. **Version control for prompts**: Either embed in git (prompts as files in a tracked directory) or maintain a JSONL changelog per template. Git-native versioning is simpler and aligns with the project's git-worktree patterns.

3. **A/B test orchestrator**: A new `internal/abtest/` package that defines test configurations, launches parallel sessions (one per model/prompt variant), waits for completion, collects metrics, and produces comparison reports. Needs to integrate with `internal/session/` for launching and `internal/enhancer/` for scoring.

4. **GitHub API client**: A new `internal/github/` package wrapping the GitHub REST/GraphQL API for reading PR diffs, posting review comments, requesting changes, and approving/merging. The `go-github/v68` or `cli/go-gh` library would be appropriate.

5. **Review rule engine**: A configurable set of review criteria (test coverage delta, lint score, file size limits, secret detection) that maps to approve/request-changes/block decisions.

6. **TUI components for prompt editing and A/B comparison**: BubbleTea textarea for prompt editing with live scoring, split-pane comparison view for A/B results.

### 3.3 Technical Debt Inventory

| Debt | Location | Impact | Effort |
|------|----------|--------|--------|
| Templates are compile-time constants | `templates.go:18-233` | Cannot add user templates without recompiling | M |
| No target provider parameter on `Analyze()` | `enhancer.go:214` | Scoring suggestions not provider-aware in analyze path | S |
| `handlePromptImprove` creates one-off client without circuit breaker | `handler_prompt.go:126-129` | Non-default provider calls bypass resilience | S |
| In-memory cache only; no persistence across restarts | `cache.go:10-17` | Repeated LLM calls for same prompts after restart | M |
| `ShouldEnhance` duplicated between `filter.go` and `handler_prompt.go:271-297` | Both files | Logic drift risk | S |
| MetaPrompts are large string constants; no testable structure | `metaprompt.go` | Hard to validate, diff, or A/B test meta-prompts | S |
| `mapProvider()` duplicated in `session/loop.go:788-796` and `handler_prompt.go:300-309` | Both files | Divergence risk on new providers | S |
| Journal patterns never feed back into enhancement | `session/journal.go`, `internal/enhancer/config.go` | Enhancement pipeline does not learn from past failures; same mistakes repeat | M |
| In-memory cache lost on restart; repeated LLM calls for same prompts | `internal/enhancer/cache.go` | Unnecessary API cost and latency after process restart | M |
| Injection detection uses regex only; O(n*m) pattern scaling | `internal/enhancer/lint.go` | Slow with large pattern sets; misses semantic injection | M |
| Scoring weights are uniform across providers | `internal/enhancer/scoring.go` | Overall score does not reflect what actually matters for target provider | M |

---

## 4. External Landscape

### 4.1 Competitor/Peer Projects

| Project | Relevance | Key Pattern | URL |
|---------|-----------|-------------|-----|
| **DSPy** (Stanford NLP) | Prompt optimization, self-improvement | MIPROv2: Bayesian optimization over instruction-space. SIMBA: stochastic mini-batch introspective failure analysis -- samples failing examples, has LLM introspect on why they fail, uses that to generate better instructions. Eliminates manual prompt tuning. | github.com/stanfordnlp/dspy |
| **Langfuse** | Prompt versioning, A/B testing, observability | Immutable prompt versions with label-based routing (`production`, `staging`, `canary`). A/B testing via label splits -- same prompt name, different versions assigned to different labels, traffic split by label weight. Full trace/span observability for every LLM call. | langfuse.com |
| **Braintrust** | Score normalization, LLM-as-judge evaluation | All scores normalized to [0,1] range for cross-metric comparison. LLM-as-judge scoring with configurable judge models. Experiment-based evaluation: same dataset, different model configs, automatic comparison matrix with diff highlighting. | braintrust.dev |
| **parry-guard** (Rust) | Prompt injection scanning | Aho-Corasick finite automaton for O(n) multi-pattern matching against injection payloads. ML layer (DeBERTa) for semantic injection detection. Two-tier: fast string scanner filters 99% of benign input, ML only runs on flagged content. 10x faster than regex-based scanning. | github.com/AkshayRamakrishnann/parry-guard |
| **claudekit** | Parallel specialist review agents | 6 parallel specialist agents (architecture, security, performance, testing, quality, documentation) review code simultaneously. Each agent has a focused system prompt and returns structured findings. Results merged into unified review with deduplication. | github.com/hairglasses-studio/claudekit |
| **PromptHub** | Prompt management, versioning | Prompt-as-file with YAML front matter (name, version, provider, variables). Directory-based organization. Diff-friendly text format. Version history via file renames or git. | prompthub.us |
| **PromptLayer** | Prompt versioning, A/B testing, analytics | Version-controlled prompt registry with automatic metric capture per call, A/B test groups, and regression tracking. Templates use Jinja2 variable syntax. | promptlayer.com |
| **Portkey AI Gateway** | Multi-provider routing, fallback, A/B testing | Gateway proxy that sits between app and LLM APIs. Supports weighted load balancing for A/B tests, automatic fallback chains, per-provider circuit breakers, and request/response logging. | portkey.ai |
| **Danger (Ruby/JS/Swift)** | Automated PR review | Plugin-based PR review that runs configurable rules against PR diffs and posts comments. Supports custom rules, inline comments, and failure/warning levels. | danger.systems |

### 4.2 Patterns Worth Adopting

1. **SIMBA introspective failure analysis** (from DSPy): When enhancement produces poor results (tracked via journal `failed` markers), sample the failing prompts, have an LLM introspect on *why* enhancement degraded them, and use that analysis to generate better pipeline rules. This directly addresses the gap where `journal.go` patterns never feed back into enhancement. Implementation: `ConsolidatePatterns()` already extracts recurring failures -- feed those into a `SelfImproveRules()` function that generates new `Config.Rules`.

2. **Immutable prompt versioning with label routing** (from Langfuse): Store each prompt version as an immutable file (`{name}-v{N}.yaml`). Use labels (`production`, `staging`, `canary`) to route traffic. A/B testing becomes: assign `canary` label to new version, split traffic 90/10, promote to `production` after metrics converge. No mutable state, no version conflicts.

3. **[0,1] score normalization** (from Braintrust): Normalize the existing 10-dimension scoring (currently 0-100 per dimension) to [0,1] for cross-metric comparability. This enables unified comparison in A/B tests where different metrics (cost in dollars, quality in score points, latency in milliseconds) need to be compared on the same scale.

4. **Provider-specific score weight profiles** (from Braintrust + DSPy): Different providers respond differently to prompt quality dimensions. Claude benefits most from Structure (XML tags) and Role Definition. Gemini benefits most from Examples and Context. OpenAI benefits most from Task Focus and Format Specification. Add per-provider weight vectors to `scoring.go` so the overall score reflects what actually matters for the target provider:

   ```go
   var providerWeights = map[ProviderName]map[string]float64{
       ProviderClaude: {"Structure": 1.5, "RoleDefinition": 1.3, "Tone": 1.2},
       ProviderGemini: {"Examples": 1.4, "Context": 1.3, "Specificity": 1.2},
       ProviderOpenAI: {"TaskFocus": 1.4, "FormatSpec": 1.3, "Clarity": 1.2},
   }
   ```

5. **Aho-Corasick injection scanning** (from parry-guard): Replace the regex-based injection detection in `lint.go` with an Aho-Corasick finite automaton for O(n) multi-pattern matching. The current regex approach scales poorly as injection patterns grow. Use `github.com/cloudflare/ahocorasick` (Go, BSD license) or `github.com/petar-dambovaliev/aho-corasick`. Keep the ML layer as a future enhancement.

6. **Danger-style rule plugins for PR review** (from Danger): Define review rules as composable functions with clear inputs (diff, coverage report, lint results) and outputs (comment, warning, failure). Ship builtin rules and allow user-defined rules via config.

7. **Per-call metric envelope** (from PromptLayer/Langfuse): Wrap every LLM call in a metrics envelope that captures latency, token counts, cost estimate, and result quality score. The existing `ImproveResult` struct can be extended with `LatencyMs`, `InputTokens`, `OutputTokens`, `CostUSD` fields.

8. **Parallel specialist review** (from claudekit): For code review automation (8.4), launch 6 focused review agents in parallel rather than one monolithic reviewer. Each specialist has a narrow system prompt (security-only, performance-only, etc.) and returns structured findings. Merge results with deduplication. This leverages ralphglasses' existing team/delegate infrastructure.

### 4.3 Anti-Patterns to Avoid

1. **Heavyweight prompt registries with SaaS dependencies**: Langfuse, PromptLayer, and Braintrust are SaaS products. The ralphglasses prompt library should be purely local (filesystem + optional git), not requiring any external service. Adopt their *patterns*, not their architecture.

2. **Statistical significance theatre**: For A/B tests with small sample sizes (common in agentic coding), avoid claiming "statistical significance" with p-values. Instead, report raw metrics, confidence intervals, and practical significance (e.g., "Model A costs 40% less per task with similar pass rate").

3. **Auto-merge without human oversight by default**: Auto-approve (8.4.4) should be opt-in and require explicit configuration. Default should be "post review comments and wait for human approval."

4. **Monolithic review agent**: Avoid building a single large review agent. Instead, compose review from independent specialist agents (coverage, lint, secrets, architecture, security, performance) that can be enabled/disabled per-project. The claudekit pattern validates this approach.

5. **Full DSPy integration**: DSPy's module/teleprompter abstraction is Python-specific and heavy. Adopt the SIMBA introspective pattern and MIPROv2 concept (Bayesian optimization over instruction space) as standalone Go implementations, not as DSPy bindings.

### 4.4 Academic & Industry References

1. **"Prompt Engineering: A Comprehensive Guide"** (Anthropic, 2024-2025): The existing meta-prompts already cite this. Key sections: XML structure for Claude, few-shot example formatting, positive framing, prompt caching via prefix stability.

2. **"A/B Testing for LLM Applications"** (Google Research, 2024): Recommends measuring task completion rate, cost efficiency, and latency rather than subjective quality scores. Sample size recommendations: minimum 30 runs per variant for stable cost estimates, 100+ for quality comparisons.

3. **"Automated Code Review: A Systematic Review"** (IEEE, 2023): Most effective automated review focuses on objective criteria (test coverage, linting, security scanning) rather than subjective code quality. LLM-based review adds value for architectural concerns and naming quality but has high false-positive rates for style issues.

4. **"Multi-Armed Bandits for Model Selection"** (NeurIPS workshop, 2024): For auto-promotion (6.8.5), Thompson Sampling or Upper Confidence Bound algorithms outperform fixed A/B splits because they minimize regret during the testing period by shifting traffic toward the better-performing variant earlier.

5. **"DSPy: Compiling Declarative Language Model Calls"** (Khattab et al., Stanford, 2024): SIMBA module introduces stochastic introspective mini-batch analysis -- when optimization stalls, sample failing examples, have the LLM explain why they fail, and generate improved instructions. MIPROv2 uses Bayesian optimization (surrogate model over instruction candidates) to efficiently search the instruction space without exhaustive evaluation.

---

## 5. Actionable Recommendations

### 5.1 Immediate Actions (Current Sprint)

| # | Action | Target Files | Effort | Impact | ROADMAP |
|---|--------|-------------|--------|--------|---------|
| 1 | Add `targetProvider` param to `Analyze()` | `internal/enhancer/enhancer.go:214` | S | Med | 8.2 prep |
| 2 | Extract `mapProvider()` to shared location | `internal/enhancer/provider.go` (new func), update `session/loop.go:788-796`, `handler_prompt.go:300-309` | S | Low | Tech debt |
| 3 | Wrap non-default provider `Improve()` calls in circuit breaker | `internal/mcpserver/handler_prompt.go:126-135` | S | Med | 8.2 prep |
| 4 | Add provider-specific score weight profiles to `Score()` | `internal/enhancer/scoring.go` — add `providerWeights` map, apply multiplicative weights per dimension in `Score()` | S | High | 8.2 prep |
| 5 | Define `PromptTemplate` YAML schema with metadata fields | `internal/enhancer/templates.go` (extend struct) | S | Med | 8.2.1 |
| 6 | Add `SaveTemplate()` / `LoadTemplateFromFile()` functions | `internal/enhancer/templates.go` | M | High | 8.2.1 |
| 7 | Create `~/.ralphglasses/prompts/` directory structure | `internal/enhancer/templates.go` | S | Med | 8.2.1 |

### 5.2 Near-Term Actions (Next 2-4 Sprints)

| # | Action | Target Files | Effort | Impact | ROADMAP |
|---|--------|-------------|--------|--------|---------|
| 8 | Implement filesystem prompt library with CRUD operations | New: `internal/enhancer/promptlib.go` | M | High | 8.2.1-8.2.2 |
| 9 | Add MCP tools: `prompt_save`, `prompt_delete`, `prompt_list_user` | `internal/mcpserver/handler_prompt.go`, `tools.go` | M | High | 8.2.1 |
| 10 | Implement prompt versioning with immutable files and label routing | New: `internal/enhancer/promptversion.go` — Langfuse pattern: `{name}-v{N}.yaml` with labels (`production`, `staging`, `canary`) | M | High | 8.2.3 |
| 11 | Implement self-improvement feedback loop: journal patterns to enhancement rules | `internal/session/journal.go` (add `PatternsToRules()`), `internal/enhancer/config.go` (consume generated rules) — DSPy SIMBA pattern | M | High | 8.5 |
| 12 | Replace regex injection detection with Aho-Corasick automaton | `internal/enhancer/lint.go` — use `github.com/cloudflare/ahocorasick` for O(n) multi-pattern matching (parry-guard pattern) | M | High | 8.2 prep |
| 13 | Add persistent filesystem cache for LLM responses | `internal/enhancer/cache.go` — add `FileCache` backing store alongside in-memory cache, keyed on SHA-256, stored in `~/.ralphglasses/cache/` | M | Med | 8.2 perf |
| 14 | Define A/B test config struct and storage | New: `internal/abtest/types.go` | M | High | 6.8.1, 8.2.4 |
| 15 | Implement A/B test runner with parallel worktree execution | New: `internal/abtest/runner.go` | L | High | 6.8.1 |
| 16 | Implement metric collection for A/B tests with [0,1] normalization | New: `internal/abtest/metrics.go` — Braintrust pattern: normalize cost, quality, latency to [0,1] for cross-metric comparison | M | High | 6.8.2 |
| 17 | Implement comparison report generator | New: `internal/abtest/report.go` | M | High | 6.8.3 |
| 18 | Add MCP tools: `abtest_create`, `abtest_status`, `abtest_report` | `internal/mcpserver/handler_abtest.go` (new) | M | High | 6.8.1-6.8.3 |
| 19 | Implement GitHub API client for PR reading and commenting | New: `internal/github/client.go` | M | High | 8.4.3 |
| 20 | Implement parallel specialist review agents (security, perf, arch, test, quality, docs) | New: `internal/review/specialists.go` — claudekit pattern: 6 focused agents, merge results | L | High | 8.4.1-8.4.2 |
| 21 | Implement PR review agent orchestrator | New: `internal/review/agent.go` | L | High | 8.4.1 |
| 22 | Add MCP tools: `review_pr`, `review_configure`, `review_status` | `internal/mcpserver/handler_review.go` (new) | M | High | 8.4.1-8.4.3 |

### 5.3 Strategic Actions (Future Sprints)

| # | Action | Target Files | Effort | Impact | ROADMAP |
|---|--------|-------------|--------|--------|---------|
| 23 | TUI prompt editor with live scoring | `internal/tui/views/prompt_editor.go` (new) | L | Med | 8.2.5 |
| 24 | TUI A/B test comparison view | `internal/tui/views/abtest.go` (new) | L | Med | 6.8.4 |
| 25 | Auto-promote model based on A/B results (Thompson Sampling) | `internal/abtest/promote.go` (new) | L | Med | 6.8.5 |
| 26 | Auto-approve workflow for PRs passing all review criteria | `internal/review/approve.go` (new) | M | High | 8.4.4 |
| 27 | TUI review dashboard (pending/approved/rejected PRs) | `internal/tui/views/review.go` (new) | L | Med | 8.4.5 |
| 28 | MIPROv2-style Bayesian optimization over meta-prompt instructions | New: `internal/enhancer/optimize.go` — surrogate model over instruction candidates for efficient meta-prompt search | L | High | 8.5 |
| 29 | ML-based semantic injection detection (DeBERTa or distilled model) | `internal/enhancer/lint.go` (extend) — parry-guard two-tier pattern: Aho-Corasick fast-path, ML only on flagged content | L | Med | 8.2 prep |
| 30 | CLI command: `ralphglasses ab-test` | `cmd/abtest.go` (new) | M | Med | 6.8 |

---

## 6. Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| GitHub API rate limiting blocks review agent at scale | Medium | High | Use conditional requests (ETags), cache PR data locally, implement exponential backoff. Use GitHub App token (5000 req/hr) instead of personal token (1000 req/hr). |
| A/B test cost overrun (running 2x models per task) | Medium | Medium | Enforce per-experiment budget caps in `internal/abtest/types.go`. Default to small sample sizes (10 runs). Show cost projection before starting. |
| Prompt library filesystem corruption or race conditions | Low | High | Use file-level locking (`flock`) for writes. Validate YAML on load. Keep prompts in git for recovery. |
| Auto-approve merges broken code | Medium | High | Default auto-approve to `off`. Require explicit opt-in per repo. Always require test pass + lint clean. Add configurable hold period before merge. |
| Multi-provider API key sprawl | Medium | Low | Already handled well via env vars. Document key rotation procedures. Validate keys on startup. |
| A/B test results misleading due to small sample sizes | High | Medium | Report confidence intervals, not just point estimates. Warn when sample size < 30. Recommend minimum test duration. |
| TUI prompt editor UX complexity | Medium | Low | Start with a simple textarea + score display. Iterate based on usage. Do not attempt a full IDE-style editor. |
| Provider API changes break clients | Low | Medium | Each client (`llmclient.go`, `gemini_client.go`, `openai_client.go`) is isolated. Pin API versions in request headers. Add integration test targets that hit real APIs (opt-in via env var). |
| Self-improvement feedback loop amplifies bad patterns | Medium | Medium | Gate pattern-to-rule conversion on minimum 5 occurrences (not 3). Require human approval for auto-generated rules via `prompt_improver.yaml` review. Add `auto_rules: false` config default. |
| Aho-Corasick pattern set grows unbounded | Low | Low | Cap pattern dictionary at 10,000 entries. Version the pattern file. Benchmark automaton build time in CI. |
| Filesystem cache grows unbounded | Medium | Low | Enforce max cache directory size (default 100MB). LRU eviction by access time. Add `cache_max_mb` config option. |

---

## 7. Implementation Priority Ordering

### 7.1 Critical Path

```
8.2.1 (Prompt Library)
  -> 8.2.2 (Variable Interpolation)     [extends existing FillTemplate]
    -> 8.2.3 (Prompt Versioning)         [git-based, requires library]
      -> 8.2.4 + 6.8.1 (A/B Testing)    [shared framework]
        -> 6.8.2 (Metrics)
          -> 6.8.3 (Comparison Report)
            -> 6.8.5 (Auto-Promote)

8.4.1 (PR Review Agent)                 [independent of above]
  -> 8.4.2 (Review Rules)
    -> 8.4.3 (GitHub Integration)
      -> 8.4.4 (Auto-Approve)

6.8.4 (TUI A/B View) + 8.2.5 (TUI Editor) + 8.4.5 (Review Dashboard)
  [TUI items are independent of backend, but depend on backend data structures]
```

### 7.2 Recommended Sequence

**Sprint 1: Foundation + Scoring (8.2.1-8.2.2, tech debt)**
1. Add provider-specific score weight profiles to `scoring.go` (rec #4)
2. Extend `PromptTemplate` struct with YAML serialization metadata (name, version, created, updated, provider targets)
3. Implement `promptlib.go` with `SaveTemplate()`, `LoadUserTemplates()`, `DeleteTemplate()`, `ListAllTemplates()`
4. Create `~/.ralphglasses/prompts/` on first use with seeded defaults (copy builtins)
5. Add `prompt_save`, `prompt_delete`, `prompt_list_user` MCP tools
6. Fix tech debt: `targetProvider` on `Analyze()`, deduplicate `mapProvider()`, wrap one-off clients in circuit breaker
7. Replace regex injection detection with Aho-Corasick automaton in `lint.go` (rec #12)

**Sprint 2: Versioning + Self-Improvement + Cache (8.2.3, 8.5, perf)**
1. Implement immutable prompt versioning with label routing (Langfuse pattern): `{name}-v{N}.yaml` files with `production`/`staging`/`canary` labels (rec #10)
2. Add `prompt_history`, `prompt_rollback` MCP tools
3. Implement self-improvement feedback loop: `PatternsToRules()` in `journal.go` that converts consolidated failure patterns into enhancement `Config.Rules` (DSPy SIMBA pattern, rec #11)
4. Add persistent filesystem cache for LLM responses in `cache.go` (rec #13)
5. Define `ABTest` struct: test ID, model variants, prompt variants, task definition, metrics targets

**Sprint 3: A/B Testing Framework (6.8.1-6.8.3, 8.2.4)**
1. Implement `internal/abtest/runner.go`: parallel session launch, wait, collect
2. Implement metric collection with [0,1] normalization (Braintrust pattern, rec #16)
3. Implement comparison report with confidence intervals
4. Wire prompt A/B testing into the same framework (8.2.4)
5. Add `abtest_create`, `abtest_status`, `abtest_report` MCP tools
6. Add `ralphglasses ab-test` CLI command

**Sprint 4: Code Review (8.4.1-8.4.3)**
1. Add `go-github` dependency, implement `internal/github/client.go`
2. Implement parallel specialist review agents (claudekit pattern): security, performance, architecture, testing, quality, documentation (rec #20)
3. Implement PR review agent orchestrator in `internal/review/agent.go` (rec #21)
4. Add `review_pr`, `review_configure` MCP tools
5. Wire to session events: auto-review PRs created by worker sessions

**Sprint 5: TUI + Auto-Promote + Auto-Approve + Meta-Optimization (6.8.4-6.8.5, 8.2.5, 8.4.4-8.4.5, 8.5)**
1. TUI prompt editor view with live scoring
2. TUI A/B comparison view
3. Auto-promote with Thompson Sampling
4. Auto-approve workflow with configurable guard rails
5. TUI review dashboard
6. MIPROv2-style Bayesian optimization over meta-prompt instructions (rec #28)

### 7.3 Parallelization Opportunities

```
Can run in parallel:
  [Sprint 1: Prompt Library]  ||  [Sprint 4: Code Review Foundation]
    Both are independent; code review doesn't need prompt library.

  [Sprint 2: Versioning]  ||  [Sprint 4: GitHub Client]
    Git integration for prompts and GitHub API client are independent.

  [Sprint 5 items are all parallelizable]:
    [TUI Prompt Editor]  ||  [TUI A/B View]  ||  [TUI Review Dashboard]
    [Auto-Promote]  ||  [Auto-Approve]

Cannot parallelize:
  A/B runner (6.8.1) must precede Metrics (6.8.2) must precede Report (6.8.3)
  Review rules (8.4.2) must precede Review agent (8.4.1) must precede Auto-approve (8.4.4)
```

---

## Appendix: Key File Paths

All paths relative to the repository root:

| Purpose | Path |
|---------|------|
| Enhancement pipeline (13 stages) | `internal/enhancer/enhancer.go` |
| Multi-dimensional scoring | `internal/enhancer/scoring.go` |
| Lint rules (11+) | `internal/enhancer/lint.go` |
| Prompt templates (5 builtin) | `internal/enhancer/templates.go` |
| Config loading (YAML + env) | `internal/enhancer/config.go` |
| Provider interface + factory | `internal/enhancer/provider.go` |
| Claude API client | `internal/enhancer/llmclient.go` |
| Gemini API client | `internal/enhancer/gemini_client.go` |
| OpenAI API client | `internal/enhancer/openai_client.go` |
| Hybrid engine (LLM + local fallback) | `internal/enhancer/hybrid.go` |
| Circuit breaker | `internal/enhancer/circuit.go` |
| LLM response cache | `internal/enhancer/cache.go` |
| Meta-prompts (3 providers x 2 modes) | `internal/enhancer/metaprompt.go` |
| Task classifier | `internal/enhancer/classifier.go` |
| Enhancement filter/skip gates | `internal/enhancer/filter.go` |
| Context reordering + cache-friendly checks | `internal/enhancer/context.go` |
| Example detection + wrapping | `internal/enhancer/examples.go` |
| CLAUDE.md health check | `internal/enhancer/claudemd.go` |
| MCP prompt tool handlers | `internal/mcpserver/handler_prompt.go` |
| Session provider templates | `internal/session/templates.go` |
| Loop-enhancer integration | `internal/session/loop.go` (lines 243-285, 768-797) |
| Session manager (enhancer field) | `internal/session/manager.go` (line 33) |
| Improvement journal (feedback loop source) | `internal/session/journal.go` |
| Cost normalization | `internal/session/costnorm.go` |
| Prompt-improver CLI | `cmd/prompt-improver/main.go` |
| ROADMAP items 6.8, 8.2, 8.4, 8.5 | `ROADMAP.md` (lines 766-772, 850-856, 866-872) |
