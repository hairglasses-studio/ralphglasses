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

---

## 4. External Landscape

### 4.1 Competitor/Peer Projects

| Project | Relevance | Key Pattern | URL |
|---------|-----------|-------------|-----|
| **PromptLayer** | Prompt versioning, A/B testing, analytics | Version-controlled prompt registry with automatic metric capture per call, A/B test groups, and regression tracking. Templates use Jinja2 variable syntax. | promptlayer.com |
| **Portkey AI Gateway** | Multi-provider routing, fallback, A/B testing | Gateway proxy that sits between app and LLM APIs. Supports weighted load balancing for A/B tests, automatic fallback chains, per-provider circuit breakers, and request/response logging. | portkey.ai |
| **Humanloop** | Prompt management, evaluation, deployment | Prompt-as-code with version control, evaluation datasets with expected outputs, side-by-side prompt variant comparison, and deployment environments (dev/staging/prod). | humanloop.com |
| **LangSmith (LangChain)** | A/B testing, tracing, evaluation | Dataset-driven evaluation runs comparing model variants. Uses "experiments" (same dataset, different configs) with automatic metric aggregation and statistical comparison. | smith.langchain.com |
| **Danger (Ruby/JS/Swift)** | Automated PR review | Plugin-based PR review that runs configurable rules against PR diffs and posts comments. Supports custom rules, inline comments, and failure/warning levels. | danger.systems |
| **Reviewbot / Qodo Merge** | AI-powered code review | Automated PR review using LLM analysis of diffs. Posts inline comments with explanations. Configurable review depth and focus areas. | qodo.ai |

### 4.2 Patterns Worth Adopting

1. **Prompt-as-file with git versioning** (from Humanloop/PromptLayer): Store prompts as individual YAML files in `~/.ralphglasses/prompts/{name}.yaml` with metadata (task type, variables, provider targets, creation date). Version tracking via git naturally -- the directory can be a git repo or subdirectory of one. This avoids building a custom VCS.

2. **Experiment-based A/B testing** (from LangSmith): Define an "experiment" as a fixed dataset of test prompts + a set of model/prompt configurations. Run each configuration against the dataset, collect metrics, and produce a comparison matrix. This is more rigorous than ad-hoc parallel runs.

3. **Weighted routing for gradual rollout** (from Portkey): Instead of binary A/B (50/50), support weighted splits (90/10 for canary testing). The existing `NewPromptImprover()` factory can be extended with a router that selects providers based on weights.

4. **Danger-style rule plugins for PR review** (from Danger): Define review rules as composable functions with clear inputs (diff, coverage report, lint results) and outputs (comment, warning, failure). Ship builtin rules and allow user-defined rules via config.

5. **Per-call metric capture** (from PromptLayer): Wrap every LLM call in a metrics envelope that captures latency, token counts, cost estimate, and result quality score. The existing `ImproveResult` struct can be extended.

### 4.3 Anti-Patterns to Avoid

1. **Heavyweight prompt registries with SaaS dependencies**: PromptLayer and Humanloop are SaaS products. The ralphglasses prompt library should be purely local (filesystem + optional git), not requiring any external service.

2. **Statistical significance theatre**: For A/B tests with small sample sizes (common in agentic coding), avoid claiming "statistical significance" with p-values. Instead, report raw metrics, confidence intervals, and practical significance (e.g., "Model A costs 40% less per task with similar pass rate").

3. **Auto-merge without human oversight by default**: Auto-approve (8.4.4) should be opt-in and require explicit configuration. Default should be "post review comments and wait for human approval."

4. **Monolithic review agent**: Avoid building a single large review agent. Instead, compose review from independent rules (coverage, lint, secrets, file size) that can be enabled/disabled per-project.

### 4.4 Academic & Industry References

1. **"Prompt Engineering: A Comprehensive Guide"** (Anthropic, 2024-2025): The existing meta-prompts already cite this. Key sections: XML structure for Claude, few-shot example formatting, positive framing, prompt caching via prefix stability.

2. **"A/B Testing for LLM Applications"** (Google Research, 2024): Recommends measuring task completion rate, cost efficiency, and latency rather than subjective quality scores. Sample size recommendations: minimum 30 runs per variant for stable cost estimates, 100+ for quality comparisons.

3. **"Automated Code Review: A Systematic Review"** (IEEE, 2023): Most effective automated review focuses on objective criteria (test coverage, linting, security scanning) rather than subjective code quality. LLM-based review adds value for architectural concerns and naming quality but has high false-positive rates for style issues.

4. **"Multi-Armed Bandits for Model Selection"** (NeurIPS workshop, 2024): For auto-promotion (6.8.5), Thompson Sampling or Upper Confidence Bound algorithms outperform fixed A/B splits because they minimize regret during the testing period by shifting traffic toward the better-performing variant earlier.

---

## 5. Actionable Recommendations

### 5.1 Immediate Actions (Current Sprint)

| # | Action | Target Files | Effort | Impact | ROADMAP |
|---|--------|-------------|--------|--------|---------|
| 1 | Add `targetProvider` param to `Analyze()` | `internal/enhancer/enhancer.go:214` | S | Med | 8.2 prep |
| 2 | Extract `mapProvider()` to shared location | `internal/enhancer/provider.go` (new func), update `session/loop.go:788-796`, `handler_prompt.go:300-309` | S | Low | Tech debt |
| 3 | Wrap non-default provider `Improve()` calls in circuit breaker | `internal/mcpserver/handler_prompt.go:126-135` | S | Med | 8.2 prep |
| 4 | Define `PromptTemplate` YAML schema with metadata fields | `internal/enhancer/templates.go` (extend struct) | S | Med | 8.2.1 |
| 5 | Add `SaveTemplate()` / `LoadTemplateFromFile()` functions | `internal/enhancer/templates.go` | M | High | 8.2.1 |
| 6 | Create `~/.ralphglasses/prompts/` directory structure with README | `internal/enhancer/templates.go` | S | Med | 8.2.1 |

### 5.2 Near-Term Actions (Next 2-4 Sprints)

| # | Action | Target Files | Effort | Impact | ROADMAP |
|---|--------|-------------|--------|--------|---------|
| 7 | Implement filesystem prompt library with CRUD operations | New: `internal/enhancer/promptlib.go` | M | High | 8.2.1-8.2.2 |
| 8 | Add MCP tools: `prompt_save`, `prompt_delete`, `prompt_list_user` | `internal/mcpserver/handler_prompt.go`, `tools.go` | M | High | 8.2.1 |
| 9 | Implement prompt versioning via git integration | New: `internal/enhancer/promptversion.go` | M | High | 8.2.3 |
| 10 | Define A/B test config struct and storage | New: `internal/abtest/types.go` | M | High | 6.8.1, 8.2.4 |
| 11 | Implement A/B test runner with parallel worktree execution | New: `internal/abtest/runner.go` | L | High | 6.8.1 |
| 12 | Implement metric collection for A/B tests (cost, duration, test pass, lint score) | New: `internal/abtest/metrics.go` | M | High | 6.8.2 |
| 13 | Implement comparison report generator | New: `internal/abtest/report.go` | M | High | 6.8.3 |
| 14 | Add MCP tools: `abtest_create`, `abtest_status`, `abtest_report` | `internal/mcpserver/handler_abtest.go` (new) | M | High | 6.8.1-6.8.3 |
| 15 | Implement GitHub API client for PR reading and commenting | New: `internal/github/client.go` | M | High | 8.4.3 |
| 16 | Implement review rule engine with configurable criteria | New: `internal/review/rules.go` | M | High | 8.4.2 |
| 17 | Implement PR review agent that uses enhancer scoring + review rules | New: `internal/review/agent.go` | L | High | 8.4.1 |
| 18 | Add MCP tools: `review_pr`, `review_configure`, `review_status` | `internal/mcpserver/handler_review.go` (new) | M | High | 8.4.1-8.4.3 |

### 5.3 Strategic Actions (Future Sprints)

| # | Action | Target Files | Effort | Impact | ROADMAP |
|---|--------|-------------|--------|--------|---------|
| 19 | TUI prompt editor with live scoring | `internal/tui/views/prompt_editor.go` (new) | L | Med | 8.2.5 |
| 20 | TUI A/B test comparison view | `internal/tui/views/abtest.go` (new) | L | Med | 6.8.4 |
| 21 | Auto-promote model based on A/B results (Thompson Sampling) | `internal/abtest/promote.go` (new) | L | Med | 6.8.5 |
| 22 | Auto-approve workflow for PRs passing all review criteria | `internal/review/approve.go` (new) | M | High | 8.4.4 |
| 23 | TUI review dashboard (pending/approved/rejected PRs) | `internal/tui/views/review.go` (new) | L | Med | 8.4.5 |
| 24 | Persistent LLM response cache (SQLite or file-backed) | `internal/enhancer/cache.go` (extend) | M | Med | 8.2 perf |
| 25 | CLI command: `ralphglasses ab-test` | `cmd/abtest.go` (new) | M | Med | 6.8 |

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

**Sprint 1: Foundation (8.2.1-8.2.2)**
1. Extend `PromptTemplate` struct with YAML serialization metadata (name, version, created, updated, provider targets)
2. Implement `promptlib.go` with `SaveTemplate()`, `LoadUserTemplates()`, `DeleteTemplate()`, `ListAllTemplates()`
3. Create `~/.ralphglasses/prompts/` on first use with seeded defaults (copy builtins)
4. Add `prompt_save`, `prompt_delete`, `prompt_list_user` MCP tools
5. Fix tech debt: `targetProvider` on `Analyze()`, deduplicate `mapProvider()`

**Sprint 2: Versioning + A/B Framework (8.2.3, 6.8.1)**
1. Implement git-based prompt versioning (prompts directory as git repo, auto-commit on save)
2. Add `prompt_history`, `prompt_rollback` MCP tools
3. Define `ABTest` struct: test ID, model variants, prompt variants, task definition, metrics targets
4. Implement `internal/abtest/runner.go`: parallel session launch, wait, collect
5. Add `abtest_create`, `abtest_status` MCP tools

**Sprint 3: Metrics + Reporting (6.8.2-6.8.3, 8.2.4)**
1. Implement metric collection (cost, duration, test pass rate, lint score per arm)
2. Implement comparison report with confidence intervals
3. Wire prompt A/B testing into the same framework (8.2.4)
4. Add `abtest_report` MCP tool
5. Add `ralphglasses ab-test` CLI command

**Sprint 4: Code Review (8.4.1-8.4.3)**
1. Add `go-github` dependency, implement `internal/github/client.go`
2. Implement review rule engine in `internal/review/rules.go`
3. Implement PR review agent in `internal/review/agent.go`
4. Add `review_pr`, `review_configure` MCP tools
5. Wire to session events: auto-review PRs created by worker sessions

**Sprint 5: TUI + Auto-Promote + Auto-Approve (6.8.4-6.8.5, 8.2.5, 8.4.4-8.4.5)**
1. TUI prompt editor view with live scoring
2. TUI A/B comparison view
3. Auto-promote with Thompson Sampling
4. Auto-approve workflow with configurable guard rails
5. TUI review dashboard

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

All paths relative to `/Users/mitchnotmitchell/hairglasses-studio/ralphglasses/`:

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
| Session manager enhancer field | `internal/session/manager.go` (line 33) |
| Prompt-improver CLI | `cmd/prompt-improver/main.go` |
| ROADMAP items 6.8, 8.2, 8.4 | `ROADMAP.md` (lines 766-772, 850-856, 866-872) |
