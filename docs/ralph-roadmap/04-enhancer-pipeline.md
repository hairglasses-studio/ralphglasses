# Enhancer Pipeline — Deep Audit

**Scope**: `internal/enhancer/` in `ralphglasses`
**Date**: 2026-04-04
**Source files audited**: enhancer.go, pipeline.go, pipeline_format.go, scoring.go, lint.go, config.go, hybrid.go, provider.go, llmclient.go, gemini_client.go, openai_client.go, circuit.go, cache.go, classifier.go, context.go, filter.go, sampling.go, templates.go, metaprompt.go, claudemd.go, examples.go, knowledge/graph.go, knowledge/injector.go

---

## 1. Stage Inventory — All 13 Pipeline Stages

The pipeline entry point is `EnhanceWithConfig()` in `enhancer.go`. Stages execute sequentially on the same mutable text string. Every stage records itself in `EnhanceResult.StagesRun` (if it ran) or `EnhanceResult.SkippedStages` (with a reason). A pre-stage labelled Stage 0 exists for config rules.

### Stage 0 — Config Rules (`config_rules`)

**File**: `enhancer.go:79`, `config.go:297`
**Function**: `Config.ApplyRules()`

Applies pattern-matched augmentation rules loaded from `.prompt-improver.yaml` in the project or home directory. Each rule is a simple substring match (`strings.Contains`, case-insensitive) that optionally prepends or appends text. The match is substring-only (not regex) for safety.

**Skipped when**: `cfg.Rules` is empty.

**Input → Output**: Raw prompt may have project-specific preambles prepended or context blocks appended.

---

### Stage 1 — Specificity (`specificity`)

**File**: `pipeline.go:9`
**Function**: `improveSpecificity()`

Replaces vague phrases with concrete instructions via a static lookup table (`vagueReplacements`, 22 entries). Matches are case-insensitive, replaces the first occurrence of each match. Example: `"be concise"` → `"Limit each point to one sentence. Use 5 bullets maximum"`.

**Skipped when**: Stage disabled in config via `DisabledStages`, or no vague phrases are detected.

**Input → Output**: Vague imperative phrases replaced with quantified, specific equivalents.

---

### Stage 2 — Positive Reframing (`positive_reframe`)

**File**: `pipeline.go:49`
**Function**: `reframeNegatives()`

Rewrites known negative instructions into positive equivalents using a 17-entry static table (`negativeReframings`). Safety bypass: if the prompt contains negatives about truly sensitive actions (e.g., "never disclose credentials"), the entire stage is skipped via `safetyNegativePattern`. The safety regex is conservative and checks for the combination of a negative modal plus a dangerous verb plus a sensitive data keyword.

**Skipped when**: Disabled in config, no negative patterns detected, or `safetyNegativePattern` matches (entire prompt bypassed — not just the matched line).

**Input → Output**: Negative-framed instructions rewritten as positive instructions.

---

### Stage 3 — Tone Downgrade (`tone_downgrade`)

**File**: `pipeline.go:96`
**Function**: `downgradeTone()`

Downgrades ALL-CAPS emphasis words (`CRITICAL`, `IMPORTANT`, `MUST`, `ALWAYS`, `NEVER`, `WARNING`, `REQUIRED`, `MANDATORY`, `ABSOLUTELY`, `ESSENTIAL`) to lowercase equivalents. A 43-entry acronym whitelist (`acronymWhitelist`) prevents legitimate acronyms (API, URL, JSON, MCP, etc.) from being modified. `ABSOLUTELY` maps to an empty string and its neighboring space is stripped.

**Skipped when**: Disabled in config, **or `cfg.TargetProvider` is set to anything other than `"claude"` (including Gemini and OpenAI)**. This is a Claude-only optimization — the comment notes other models do not overtrigger on aggressive language.

**Input → Output**: Aggressive ALL-CAPS words normalized to lowercase.

---

### Stage 4 — Overtrigger Rewrite (`overtrigger_rewrite`)

**File**: `pipeline_format.go:9`
**Function**: `rewriteOvertriggerPhrases()`

Targets aggressive anti-laziness prefixes of the form `CRITICAL: You MUST`, `IMPORTANT! ALWAYS`, etc. (matched by `overtriggerPattern`). The rewrite strips the aggressive prefix and keeps only the action, converting `MUST do X` → `do X`, `NEVER do X` → `Avoid: do X`, `ALWAYS do X` → `do X`.

**Skipped when**: Disabled in config, **or `cfg.TargetProvider` is set to anything other than `"claude"`**. Claude-only optimization.

**Input → Output**: `CRITICAL: You MUST use X` → `use X`.

---

### Stage 5 — Example Wrapping (`example_wrapping`)

**File**: `enhancer.go:170`, `examples.go`
**Function**: `DetectAndWrapExamples()`

Detects bare examples in the prompt and wraps them in `<examples><example>` tags. Three detection strategies are tried in order:
1. Input:/Output: pairs (or Query:/Answer:, Question:/Response:, Prompt:/Result:)
2. `Example N:` header sections (regex match on markdown headers)
3. Arrow transformation patterns (`A -> B`, `A => B`, `A → B`)

Requires at least 2 examples for any strategy to fire. Skips if `<example` tags already exist.

**Skipped when**: Disabled in config, or no bare examples detected.

**Input → Output**: Bare examples wrapped in `<examples><example index="N">` XML structure.

---

### Stage 6 — Structure (`structure`)

**File**: `pipeline.go:164`
**Function**: `addStructure()` (Claude) or `addMarkdownStructure()` (Gemini/OpenAI)

**This is the primary provider-aware stage.** Wraps the prompt in structural markup.

- **Claude target** (`addStructure`): Adds `<role>`, `<instructions>`, optional `<context>` (if a code block is detected), and `<constraints>` XML tags. Roles and constraints are task-type-specific (6 task types).
- **Gemini/OpenAI target** (`addMarkdownStructure`): Adds `## Role`, `## Instructions` or `## Context`, and `## Constraints` markdown sections.

Over-tagging prevention (`shouldAddStructure`): skips if prompt is under 15 words, or already contains `<instructions` or `<role` tags.

Code block separation: `extractCodeBlock()` detects backtick-fenced code and moves it into a separate `<context>` or `## Context` section, placing the rest in `<instructions>`.

**Skipped when**: Disabled in config. Unlike other stages, when active, this stage **always** records itself as run (even if no changes were made due to over-tagging prevention — the improvement message records the skip reason).

**Input → Output**: Prompt wrapped in provider-appropriate structural markup.

---

### Stage 7 — Context Reorder (`context_reorder`)

**File**: `context.go:30`
**Function**: `ReorderLongContext()`

Moves large context blocks before the query in long prompts. Only activates when the prompt exceeds ~20,000 characters (LongContextChars = 80,000 chars, but checked at 1/4 of that). Splits on double-newlines to find paragraphs, identifies the longest paragraph as "context" and the last short paragraph with a question mark or imperative verb as the "query". Only reorders if context appears after query and context is at least 3x larger than query.

Skips if the prompt already has `<context>` or `<documents>` XML tags.

**Skipped when**: Disabled in config, prompt is too short (< 20,000 chars), already has XML context structure, or the heuristic can't find a clear context/query split.

**Input → Output**: Bulk context block moved before the query paragraph.

---

### Stage 8 — Format Enforcement (`format_enforcement`)

**File**: `pipeline_format.go:128`
**Function**: `enforceOutputFormat()`

Detects output format requests in the prompt and injects an `<output_format>` block with strict format instructions. Four formats detected:
- JSON: injects "Your entire response must be valid JSON..."
- YAML: injects "Your entire response must be valid YAML..."
- CSV: injects CSV header row requirement
- Code: injects "Return only the code..." (matched on prompts starting with `write/create/implement/generate a function/class/method/script/program/module`)

Skips if `<output_format>` or `output_format` already appears in the prompt.

**Skipped when**: Disabled in config, no format request detected, or already has format specification.

**Input → Output**: `<output_format>` block appended with format-specific strictness instructions.

---

### Stage 9 — Quote Grounding (`quote_grounding`)

**File**: `context.go:101`
**Function**: `InjectQuoteGrounding()`

For long analysis prompts, injects an instruction to "find and quote the specific passages... before answering" and wrap them in `<quotes>` tags. Only fires for `TaskTypeAnalysis` or `TaskTypeGeneral`, and only for prompts >= 5,000 tokens (~20,000 chars).

**Skipped when**: Disabled in config, task type is not analysis/general, prompt is too short, or prompt already contains quote/cite/reference instructions.

**Input → Output**: Quote-grounding instruction appended to prompt.

---

### Stage 10 — Self-Check (`self_check`)

**File**: `pipeline_format.go:167`
**Function**: `injectSelfCheck()`

Appends a `<verification>` block with task-type-specific pre-completion checklists:
- Code: check compilation, edge cases (nil/empty/zero), descriptive error messages
- Analysis: evidence support, fact vs inference distinction, alternative interpretations
- Troubleshooting: root cause vs symptom, rollback steps, side effects

Only fires for `TaskTypeCode`, `TaskTypeAnalysis`, and `TaskTypeTroubleshooting`. Skips if the prompt already contains "verify", "double-check", "self-check", or "before you finish".

**Skipped when**: Disabled in config, task type is creative/workflow/general, or verification language already present.

**Input → Output**: `<verification>` checklist appended.

---

### Stage 11 — Overengineering Guard (`overengineering_guard`)

**File**: `pipeline_format.go:61`
**Function**: `injectOverengineeringGuard()`

Appends a guard instruction: "Only make changes that are directly requested or clearly necessary. Prefer editing existing files to creating new ones. Do not add abstractions, helpers, or defensive code for scenarios that cannot happen."

Only fires for `TaskTypeCode` prompts. Exempt if the prompt explicitly requests new scaffolding (create new, scaffold, generate boilerplate, set up a new, initialize a new).

**Skipped when**: Disabled in config, task type is not code, prompt requests new creation, or guard language already present.

**Input → Output**: Anti-overengineering instruction appended.

---

### Stage 12 — Preamble Suppression (`preamble_suppression`)

**File**: `pipeline_format.go:192`
**Function**: `suppressPreamble()`

Appends: "Respond directly without preamble. Do not start with phrases like 'Here is...', 'Sure,...', or 'Based on...'." Only for `TaskTypeCode` and `TaskTypeWorkflow`.

**Skipped when**: Disabled in config, task type is not code/workflow, or preamble suppression language already present.

**Input → Output**: Direct-response instruction appended.

---

### Post-pipeline: Config Preamble

**File**: `enhancer.go:319`

After all 13 stages, if `cfg.Preamble` is set, it is prepended to the entire enhanced text. This runs unconditionally (not tracked as a stage, not skippable via `DisabledStages`).

---

## 2. Provider-Aware Behavior

The `cfg.TargetProvider` field (type `ProviderName`) controls pipeline adaptation for the **target** model (the model that will receive the enhanced prompt), distinct from the LLM provider used to call the improvement API.

### Stage-level provider gating

| Stage | Claude | Gemini | OpenAI | Notes |
|-------|--------|--------|--------|-------|
| `tone_downgrade` (Stage 3) | Runs | **Skipped** | **Skipped** | `cfg.TargetProvider != "" && cfg.TargetProvider != ProviderClaude` at `enhancer.go:131` |
| `overtrigger_rewrite` (Stage 4) | Runs | **Skipped** | **Skipped** | Same condition at `enhancer.go:153` |
| `structure` (Stage 6) | XML (`addStructure`) | Markdown (`addMarkdownStructure`) | Markdown (`addMarkdownStructure`) | `pipeline.go:192-199` |

The structure stage condition is `if cfg.TargetProvider != "" && cfg.TargetProvider != ProviderClaude` — meaning if `TargetProvider` is empty, it defaults to Claude XML behavior.

### Scoring provider-awareness

The `scoreStructure()` dimension scorer (`scoring.go:383`) provides different suggestions based on `targetProvider`:
- Claude target: suggests XML structure tags
- Gemini/OpenAI target: suggests `## Role, ## Instructions, ## Constraints` markdown sections

The `scoreTone()` dimension (`scoring.go:673`) applies heavier penalties for aggressive caps and negative framing when `targetProvider == ProviderClaude` (-25/-20) vs other providers (-10/-10).

### Meta-prompt provider-awareness

`MetaPromptFor()` in `metaprompt.go:172` dispatches to one of three distinct meta-prompts:
- `MetaPrompt` / `MetaPromptWithThinking` — Claude: uses XML `<scratchpad>`, `<role>`, custom XML output tags
- `GeminiMetaPrompt` / `GeminiMetaPromptWithThinking` — Gemini: uses `## Reasoning` markdown section, structured markdown output headers
- `OpenAIMetaPrompt` / `OpenAIMetaPromptWithThinking` — OpenAI: uses `Think step by step:` section, markdown headers

### Knowledge injector provider-awareness

`knowledge/injector.go` has two formatters: `formatContextBlock()` (XML, for Claude) and `FormatContextBlockMarkdown()` (markdown, for Gemini/OpenAI), though the `InjectContext()` method always uses XML. This is a latent inconsistency — `FormatContextBlockMarkdown` is defined but not called from the pipeline.

### Default target provider

`config.go:239`: if `TargetProvider` is empty after resolution, it defaults to `defaultTargetProviderForLLM(cfg.LLM.Provider)` which maps the LLM call provider to the target. If `LLM.Provider` is empty, it defaults to `"openai"` (`config.go:236`). This means the default pipeline behavior uses OpenAI-style markdown structure, not Claude XML — a potentially surprising default for users who don't set `PROMPT_IMPROVER_TARGET`.

---

## 3. Scoring Calibration

The scoring system (`scoring.go`) produces a `ScoreReport` with 10 dimensions. Each has a 0-100 score, a weight, and a letter grade.

### Dimension table

| # | Dimension | Function | Weight | Baseline | Grade thresholds |
|---|-----------|----------|--------|----------|-----------------|
| 1 | Clarity | `scoreClarity` | 0.15 | 30 | A≥90, B≥80, C≥65, D≥50, F<50 |
| 2 | Specificity | `scoreSpecificity` | 0.12 | 25 | same |
| 3 | Context & Motivation | `scoreContextMotivation` | 0.10 | 30 | same |
| 4 | Structure | `scoreStructure` | 0.15 | 25 | same |
| 5 | Examples | `scoreExamples` | 0.10 | 20 | same |
| 6 | Document Placement | `scoreDocumentPlacement` | 0.08 | 40 | same |
| 7 | Role Definition | `scoreRoleDefinition` | 0.08 | 35 | same |
| 8 | Task Focus | `scoreTaskFocus` | 0.07 | 30 | same |
| 9 | Format Specification | `scoreFormatSpec` | 0.08 | 20 | same |
| 10 | Tone | `scoreTone` | 0.07 | 70 | same |

Weights sum to 1.00 exactly (0.15+0.12+0.10+0.15+0.10+0.08+0.08+0.07+0.08+0.07 = 1.00).

The overall score is capped to [5, 95] — "nothing is perfect" (comment at `scoring.go:64`).

### Score inflation fix — FINDING-240

Multiple baselines carry an explicit comment `// FINDING-240: lowered from X to prevent score inflation`:
- Clarity: baseline lowered from 50 → 30 (`scoring.go:173`)
- Specificity: lowered from 50 → 25 (`scoring.go:259`)
- Structure: baseline is 25 with comment "no structure signals → low score" (`scoring.go:384`)
- Document Placement: lowered from 60 → 40 (`scoring.go:503`)
- Task Focus: comment "must earn score from actual task signals" (`scoring.go:582`)
- Tone: baseline set to 70 "neutral tone is the expected baseline; only penalize actual problems" (`scoring.go:674`)

The CLAUDE.md states a calibration target of `<1.2x`. No explicit test enforcing this ratio was found in the audited source files — the fix is applied through baseline constants rather than a runtime calibration check.

### QW-4 trivial prompt penalty

Dimensions that apply a "trivial prompt" penalty when `ar.WordCount <= 3`:
- Clarity: `-20` score
- Specificity: `-15` score
- Task Focus: `-15` score

This prevents single-word prompts from scoring above D.

---

## 4. Lint Rule Inventory

`Lint()` in `lint.go:31` runs per-line and whole-prompt checks. `VerifyCacheFriendlyOrder()` in `context.go:128` provides 3 additional cache-related rules. Both are called together in `Analyze()`.

### Per-line rules

**Rule 1: Unmotivated Rule** (`unmotivated-rule`)
- **Catches**: Imperative directives (always/never/must/should/ensure lines) without a "because/since/so that/in order to" motivation clause.
- **Severity**: info
- **Auto-fixable**: false
- **False positive risk**: Medium-high. Many legitimate short constraints ("Always use UTC timestamps") are flagged. The minimum-4-words guard reduces but doesn't eliminate it.

**Rule 2: Negative Framing** (`negative-framing`)
- **Catches**: Lines matching `negativePattern` (NEVER, DO NOT, DON'T, MUST NOT, SHOULD NOT, CANNOT, CAN'T) that are not safety-critical and not already covered by the reframing table.
- **Severity**: warn
- **Auto-fixable**: false
- **False positive risk**: Medium. The safety bypass and reframing table exclusions help, but legitimate technical negatives ("Don't use deprecated API X") are flagged.

**Rule 3: Aggressive Emphasis** (`aggressive-emphasis`)
- **Catches**: ALL-CAPS words from `aggressiveCapsPattern` not in the 43-entry `acronymWhitelist`.
- **Severity**: info
- **Auto-fixable**: true
- **False positive risk**: Low. The acronym whitelist is comprehensive. Domain-specific acronyms not in the whitelist (e.g., `MIDI` is actually in the list) could be flagged.

**Rule 4: Vague Quantifiers** (`vague-quantifier`)
- **Catches**: Words like "a few", "some", "several", "many", "a lot", "a bit", "enough", "various", "appropriate", "suitable", "proper", "good", "nice", "decent".
- **Severity**: info
- **Auto-fixable**: false
- **False positive risk**: Medium. "good" and "nice" are extremely common and will flag benign uses ("good error messages", "nice formatting"). "appropriate" in "appropriate response" is legitimate hedging.

**Rule 5: Overtrigger Phrase** (`overtrigger-phrase`)
- **Catches**: `CRITICAL|IMPORTANT|REQUIRED|WARNING : You MUST|ALWAYS|NEVER|SHOULD` patterns.
- **Severity**: warn
- **Auto-fixable**: true
- **False positive risk**: Low. Pattern is specific enough.

**Rule 6: Injection Vulnerability** (`injection-risk`)
- **Catches**: Template variables (`${...}` or `{{...}}`) whose names match untrusted input patterns (user_input, user_query, user_message, raw_input, untrusted, external, request_body, form_data, query_string, params).
- **Severity**: error
- **Auto-fixable**: false
- **False positive risk**: Low. The combination of template syntax + suspicious variable name is a strong signal.

**Rule 7: Thinking Mode Redundant** (`thinking-mode-redundant`)
- **Catches**: "think step by step", "let's think", "chain of thought", "reason through this", "think carefully".
- **Severity**: info
- **Auto-fixable**: false
- **False positive risk**: Low for Claude 4.x. Accurate for the target environment. May be wrong if targeting Gemini or GPT-4o which do benefit from CoT.

### Whole-prompt rules

**Rule 8: Over-Specification** (`over-specification`)
- **Catches**: Prompts with more than 5 numbered steps (`1.`, `2)`, etc.).
- **Severity**: info
- **Auto-fixable**: false
- **False positive risk**: Low. More than 5 enumerated steps genuinely indicates over-specification for most use cases.

**Rule 9: Decomposition Needed** (`decomposition-needed`)
- **Catches**: Prompts with 3+ distinct imperative verbs (create, build, implement, write, fix, debug, refactor, analyze, review, design, test, deploy, configure, set up, migrate, update, delete, remove) after deduplication.
- **Severity**: info
- **Auto-fixable**: false
- **False positive risk**: Medium. Complex but legitimately single-task prompts (e.g., "refactor the module, fix the bug it introduced, and add tests") will be flagged. The deduplication helps but 3 unique verbs is a low threshold.

**Rule 10: Example Quality** (`example-quality`)
- **Catches**: Prompts with `<example` tags that have fewer than 3 or more than 5 examples.
- **Severity**: info
- **Auto-fixable**: false
- **False positive risk**: Low. Fires only when examples are already present. Flagging <3 examples as "not enough" is generally correct.

**Rule 11: Compaction Readiness** (`compaction-readiness`)
- **Catches**: Prompts estimated at >= 50,000 tokens that lack compaction guidance keywords.
- **Severity**: warn
- **Auto-fixable**: false
- **False positive risk**: Low. 50K tokens is a high bar.

### Cache rules (from `VerifyCacheFriendlyOrder`)

**Rule 12: Cache-Unfriendly Order** (`cache-unfriendly-order`)
- **Catches**: Prompts where a dynamic section (with `{{...}}` or `${...}` variables) appears before a static section (`<role>`, `<constraints>`, `<examples>`, `<output_format>`).
- **Severity**: warn
- **False positive risk**: Low.

**Rule 13: Cache-Unfriendly Variable** (`cache-unfriendly-variable`)
- **Catches**: Template variables in the first third of the prompt text.
- **Severity**: info
- **False positive risk**: Low.

**Rule 14: Cache No Structure** (`cache-no-structure`)
- **Catches**: Prompts over 1,000 tokens with no XML tags whatsoever.
- **Severity**: info
- **False positive risk**: Medium. Markdown-structured prompts for Gemini/OpenAI targets will be flagged as lacking XML, even though their structure is appropriate for the target.

### Rule overlaps and conflicts

- **Rules 1 and 7 can conflict**: A prompt with "Think step by step because it helps me understand the reasoning" would pass Rule 1 (motivated) but still fire Rule 7 (thinking-mode-redundant).
- **Rules 2 and 6 can overlap**: A safety-critical negative with an injection variable will have Rule 6 bypass the safety check in Rule 2, since they are independent checkers.
- **Rule 14 conflicts with the default pipeline behavior**: The pipeline produces markdown structure for the OpenAI default target, but Rule 14 in `VerifyCacheFriendlyOrder` checks for XML tags only. Markdown-structured prompts targeting OpenAI will always trigger `cache-no-structure`.

---

## 5. LLM Client Parity

Three clients implement `PromptImprover`: `LLMClient` (Claude), `GeminiClient` (Gemini), `OpenAIClient` (OpenAI). A fourth client, `SamplingEngine`, uses MCP Sampling (host-client completion).

### Side-by-side comparison

| Aspect | Claude (`llmclient.go`) | Gemini (`gemini_client.go`) | OpenAI (`openai_client.go`) |
|--------|------------------------|----------------------------|------------------------------|
| Default model | `claude-sonnet-4-6` | `gemini-3.1-pro` | `o3` |
| Default timeout | 30s | 30s | 30s |
| API style | Anthropic Go SDK | Raw HTTP (REST) | Raw HTTP (Responses API) |
| API key env | `ANTHROPIC_API_KEY` | `GOOGLE_API_KEY` or `GEMINI_API_KEY` | `OPENAI_API_KEY` |
| Custom base URL | Yes | Yes | Yes |
| Max output tokens | 4096 | 4096 | 4096 |
| Circuit breaker | Shared via `HybridEngine.CB` | Shared via `HybridEngine.CB` | Shared via `HybridEngine.CB` |
| Retry/backoff | Via `retryImprove()` in `backoff.go` | Via `retryImprove()` | Via `retryImprove()` |
| Thinking mode | Yes (`AdaptiveThinking`, display omitted by default) | Yes (`thinkingBudget: -1` = dynamic) | Yes (per spec, but not surfaced as `ThinkingEnabled` maps to reasoning effort) |
| Prompt caching | Yes — `CacheControl` param on SDK (`cache_control` ephemeral, default enabled) | Yes — `CreateCachedContent()` creates a `cachedContents` resource with 1h TTL | No — prefix caching is automatic on the Responses API; no explicit control needed |
| Effort/reasoning | `OutputConfig.Effort` ("low"/"medium"/"high"/"max") | Not supported in current `Improve()` | `Reasoning.Effort` ("none"/"low"/"medium"/"high") per task type |
| Multi-turn | Not used | Not used | `PreviousResponseID` tracked via `c.LastResponseID` — **unique to OpenAI client** |
| Error extraction | SDK wraps errors with status codes | HTTP status code + JSON body | HTTP status code + JSON body |

### Circuit breaker

All three clients share the **same** `CircuitBreaker` instance per `HybridEngine`. Parameters: `maxFailures=3`, `cooldown=60s`. Recovery: after 60s, state transitions to `half-open`; one probe request is allowed. On success, immediately resets to `closed`. On failure in `half-open`, state stays `half-open` (the `Allow()` method returns `false` for `half-open` — meaning only one probe ever escapes, but there's no explicit re-open timer for subsequent half-open failures; the `RecordFailure` counter increments and will set `openUntil` again since `failures >= maxFailures`).

The circuit is not per-provider — if Gemini starts failing, it trips the shared circuit and also blocks Claude attempts until reset. This is likely unintentional.

### Retry/backoff

All three clients use identical retry policy via `retryImprove()` in `backoff.go`: base 500ms, max 30s, factor 2.0, 3 retries. Full jitter (uniform random in [0, ceiling]) via `math/rand/v2`. Context cancellation and deadline exceeded errors are non-retryable. Rate limits (429), server errors (5xx), and transient network errors are retryable. Auth errors (401/403) and bad requests (400) are non-retryable.

The Gemini client uses the Gemini-specific error code `RESOURCE_EXHAUSTED` in the retryable detection string, which is correctly handled by the `isRetryableError` function.

### Timeout handling

Timeout is set on the `http.Client` at construction time. The `HybridEngine` also applies a context deadline via `context.WithTimeout(ctx, timeout)` in `hybrid.go:128`. The Claude SDK client has its own `http.Client{Timeout: timeout}` wired through `option.WithHTTPClient`. This means the timeout is enforced twice for Claude (once by context, once by HTTP client), which is harmless — the shorter deadline wins.

### Error handling patterns

- **Claude**: SDK returns typed errors. The `Improve()` method wraps with `fmt.Errorf("api call: %w", err)`.
- **Gemini/OpenAI**: Raw HTTP clients check `resp.StatusCode != http.StatusOK` and return `fmt.Errorf("api error (status %d): %s", ...)`. Additionally, Gemini checks `apiResp.Error != nil` after JSON parse; OpenAI checks `apiResp.Error != nil`. Error message format differs between providers — Gemini uses `Status: Message`, OpenAI uses `Type: Message`.

---

## 6. Hybrid Engine Flow

`EnhanceHybrid()` in `hybrid.go:61` orchestrates the decision between local-only and LLM-assisted enhancement.

### Decision tree

```
EnhanceHybrid(mode, engine)
│
├── mode=="" → coerce to ModeAuto
│
├── mode==ModeLocal OR engine==nil
│   └── EnhanceWithConfig() [local pipeline]
│       result.Source = "local"
│
└── mode==ModeLLM or ModeAuto
    │
    ├── Check PromptCache.Get(prompt, opts)
    │   └── HIT → return cached result, Source="llm_cached"
    │
    ├── Check CircuitBreaker.Allow()
    │   ├── OPEN + mode==ModeLLM → return original prompt, Source="error"
    │   └── OPEN + mode==ModeAuto → EnhanceWithConfig() fallback
    │                                Source="local_fallback"
    │
    ├── retryImprove(client, prompt, opts, DefaultBackoff())
    │   ├── ERROR + mode==ModeLLM → return original prompt, Source="error"
    │   ├── ERROR + mode==ModeAuto → EnhanceWithConfig() fallback
    │   │                            Source="local_fallback"
    │   └── SUCCESS
    │       ├── CB.RecordSuccess()
    │       ├── Cache.Put(prompt, opts, result)
    │       └── return result, Source="llm"
```

### Decision controls

The decision is controlled by:
1. **`mode` parameter** (`ModeLocal`, `ModeLLM`, `ModeAuto`, `ModeSampling`)
2. **`engine == nil`** — engine is nil if LLM is not enabled in config or no API key is available
3. **Circuit breaker state** — open circuit forces fallback in Auto mode, hard-fails in LLM-only mode
4. **Cache hit** — bypasses both circuit check and LLM call

There is no **cost budget** gate in the hybrid engine itself. Cost enforcement happens upstream in the session manager (`session/budget.go`), not in the enhancer.

### `SamplingEngine`

A fourth mode (`ModeSampling`) is defined in `sampling.go`. It routes through MCP Sampling (`sampling/createMessage`) to let the host client (e.g., Claude Code) perform the LLM call. This is the only mode that does not go through `HybridEngine` — it implements `PromptImprover` directly. The `ValidMode()` function in `hybrid.go:20` accepts `"sampling"` as a valid mode.

---

## 7. Prompt Caching Strategy

### Claude — SDK cache_control

**File**: `llmclient.go:136`

When `c.cacheControl == true` (default unless explicitly disabled), the SDK call includes:
```go
params.CacheControl = anthropic.NewCacheControlEphemeralParam()
```
This applies a top-level `cache_control` to the last cacheable block (the system message). The ephemeral cache has a 5-minute TTL per Anthropic's documentation. Cache hits are reflected in token usage as `cache_read_input_tokens`.

Default is `true` unless `CacheControl: false` is explicitly set in config or via `PROMPT_IMPROVER_CACHE=0`. The `cacheControlSet` bool tracks whether the field was explicitly provided in YAML (via custom `UnmarshalYAML`), preventing the zero-value false from disabling caching.

**Estimated savings**: 80-90% on system prompt tokens for repeated calls with the same meta-prompt.

### Gemini — cachedContents API

**File**: `gemini_client.go:209`

`CreateCachedContent()` creates a server-side cache entry via `POST /v1beta/cachedContents` with a 1-hour TTL (`"ttl": "3600s"`). The returned cache name (e.g., `cachedContents/abc123`) is stored in `c.CacheName`. Subsequent `Improve()` calls use `reqBody.CachedContent = c.CacheName` instead of inlining the system instruction.

**Critical gap**: `CreateCachedContent()` is **never called automatically**. It must be called by the operator and the cache name manually stored. The `HybridEngine` does not call it. This means Gemini prompt caching is opt-in at the operator level and is not activated by default, unlike Claude where caching is on by default.

**Estimated savings**: 90% on system instruction tokens when cache is active.

### OpenAI — automatic prefix caching

**File**: `openai_client.go` (implicit)

OpenAI's Responses API provides automatic prefix caching on inputs exceeding 1,024 tokens. No explicit action is required. `PreviousResponseID` is tracked via `c.LastResponseID` for multi-turn chaining, which allows the API to cache the entire conversation prefix. This is the most automatic of the three implementations.

**Estimated savings**: 50% on cached input tokens (OpenAI's standard prefix caching discount).

### In-process LLM result cache

**File**: `cache.go:14` — `PromptCache`

Separate from API-level caching. SHA-256 keyed on `provider + prompt + thinkingEnabled + taskType + feedback`. Max 100 entries, 10-minute TTL, oldest-entry eviction (not LRU — the eviction scans all entries).

**File**: `cache.go:128` — `EnhancerCache`

LRU cache for the deterministic (local) pipeline results. SHA-256 keyed on normalized prompt + provider. Max 256 entries (configurable), 10-minute TTL (configurable), proper LRU via `container/list`. Normalizes prompts by trimming and collapsing whitespace. `Invalidate()` removes entries across all 3 providers for a given prompt.

The `PromptCache` eviction is O(n) (map scan), while `EnhancerCache` eviction is O(1) (doubly-linked list tail). This is a latent performance gap for the LLM result cache at high volume.

---

## 8. A/B Testing Infrastructure

The A/B testing in ralphglasses operates on **session observation data** (loop iteration outcomes), not on prompt variants themselves. It is implemented in `internal/eval/` and exposed via `handler_eval.go`.

### `ralphglasses_eval_ab_test` (MCP tool)

**File**: `handler_eval.go:97`

Two modes:
- **providers**: Compares two providers (e.g., `claude` vs `gemini`) by splitting `LoopObservation` records by `WorkerProvider`. Uses `eval.CompareProviders()`.
- **periods**: Compares observations before and after a split timestamp. Uses `eval.ComparePeriods()`.

Minimum group size: `abTestMinGroupSize = 5` observations per group. Below this threshold, returns `status: "insufficient_data"` without computing posteriors.

Data source: `session.ObservationPath(r.Path)` → `.ralph/observations.jsonl` (JSONL, one observation per session iteration). The time window is configurable (`hours` param, default 168h = 7 days).

### `ralphglasses_eval_significance` (MCP tool)

**File**: `handler_eval.go:289`

Three modes:
- **providers**: Uses `eval.GenerateReport()` on binary success vectors (1.0/0.0 from `VerifyPassed`)
- **periods**: Same as providers but split by time
- **cost**: Uses `eval.WelchTTest()` on cost vectors — Welch's t-test for unequal-variance groups

### Statistical methods

From `handler_eval.go` references to `internal/eval`:
- `eval.CompareProviders()`: Bayesian posterior comparison
- `eval.ComparePeriods()`: Bayesian comparison by time split
- `eval.GenerateReport()`: Full significance report including confidence intervals
- `eval.WelchTTest()`: Welch's t-test for cost comparison
- `eval.DetectChangepoints()`: Sliding-window changepoint detection with `changepointBurnIn = 5` to suppress early false positives

### Storage

Results are stored in `.ralph/observations.jsonl` (append-only JSONL per repo). The eval tools read this file directly. There is no centralized results store — each repo has its own observation history.

### Prompt-level A/B testing

There is no dedicated prompt variant A/B infrastructure in the enhancer. The `prompt_ab_test` MCP tool (listed in the namespace table) routes through the prompt namespace handler. However, the actual `handlePromptAnalyze` and `handlePromptEnhance` handlers don't persist variants or outcomes — any A/B comparison of prompt variants would need to be done by the caller comparing two `Analyze` or `Enhance` results. The session-level A/B infrastructure is more mature than the prompt-level A/B infrastructure.

---

## 9. Gaps and Risks

### Gap: No code-specific optimization beyond basic guards

The pipeline has no language-specific stages. A Go prompt and a Python prompt get identical treatment (except the classifier may route to `TaskTypeCode`). No detection of:
- Missing test framework specification
- Missing type annotation requirements
- Language-specific idiom enforcement (Go error wrapping, Python type hints)

The `code_review` template (`templates.go`) partially addresses this by using `{{language}}` placeholders, but the pipeline stages themselves are language-agnostic.

### Gap: Gemini caching is not auto-activated

`CreateCachedContent()` must be called manually. The `HybridEngine` constructor does not call it. Gemini users get no prompt caching without operator intervention, despite the CLAUDE.md advertising "80-90% input cost savings."

### Gap: Shared circuit breaker across providers

A single `CircuitBreaker` in `HybridEngine` gates all three providers. If one provider's API is rate-limiting, the shared circuit trips and also blocks the other providers. Should be per-provider.

### Gap: Half-open state behavior

When the circuit is in `half-open`, `Allow()` returns `false` immediately. This means only the first probe request that transitioned from `open` → `half-open` gets through. If that probe fails, `RecordFailure()` increments the counter and re-opens (since `failures >= maxFailures` is still true). The circuit will re-open for another 60s and then probe again. This is correct behavior but means recovery can be slow under repeated probe failures.

### Gap: `PromptCache` eviction is O(n)

The LLM result cache (`PromptCache`) uses a map scan for eviction. At max size (100 entries) this is negligible, but the `EnhancerCache` has proper LRU with O(1) eviction. The inconsistency is a latent performance issue if `maxSize` is increased.

### Scoring blind spot: Document Placement baseline

`scoreDocumentPlacement()` starts at 40 and notes "For short prompts, placement is less important — neutral score. However, if the prompt already has XML structure, placement is demonstrated." For short prompts without XML, the score stays exactly at 40 (grade D). This means essentially every short prompt without structure scores D on Document Placement, inflating the "has weak dimensions" count in `ShouldEnhance()` and causing unnecessary enhancement recommendations for short, valid prompts.

### Scoring blind spot: Tone dimension baseline asymmetry

`scoreTone()` starts at 70 (grade C baseline) and awards bonuses for polite markers. This means a prompt with zero tone issues starts with a C, not a D/F. The rationale (neutral tone is fine) is sound, but it creates a systematic score inflation in the Tone dimension relative to others that start at 25-30 and must earn their way up.

### Risk: `safetyNegativePattern` applies to entire prompt, not per-line

In `reframeNegatives()` (`pipeline.go:77`), if `safetyNegativePattern` matches anywhere in the prompt, the **entire Stage 2 is skipped** for the whole prompt. A prompt that combines a safety-critical negative ("never disclose credentials") with benign negatives ("never use bullet points") will skip reframing for all of them. The per-line `Lint()` checker (`checkNegativeFraming`) does apply the safety bypass per-line, making Lint more accurate than the pipeline stage.

### Risk: Default target provider is OpenAI, not Claude

`config.go:239` defaults `TargetProvider` to `ProviderOpenAI` (via `defaultTargetProviderForLLM("openai")`) when no target is specified and the LLM provider defaults to "openai". This means the pipeline produces markdown structure instead of XML by default, and skips tone_downgrade and overtrigger_rewrite for Claude targets. Users who are sending prompts to Claude but haven't set `PROMPT_IMPROVER_TARGET=claude` get the wrong pipeline behavior silently.

### Risk: `ThinkingEnabled` propagation gap for OpenAI

In `openai_client.go:130-131`, `opts.ThinkingEnabled` is passed as `ImproveOptions.ThinkingEnabled` but the `reasoningEffort()` function ignores it — it only maps `taskType` to effort level. Enabling thinking does change the meta-prompt (via `MetaPromptFor` selecting `OpenAIMetaPromptWithThinking`), but the API call does not change its reasoning effort based on `ThinkingEnabled`. For OpenAI, `ThinkingEnabled` only affects the meta-prompt, not the model's reasoning behavior.

### Risk: `wrapInputOutputPairs()` requires exactly adjacent pairs

`examples.go:53` requires `Input:` and `Output:` on consecutive lines. Any blank line between them breaks detection. This means most human-written examples with a blank line for readability will not be detected.

### Risk: `knowledge/injector.go` — `InjectContext()` always uses XML

`InjectContext()` calls `formatContextBlock()` (XML format) regardless of `TargetProvider`. The `FormatContextBlockMarkdown()` method exists but is unreachable from the main pipeline. If the pipeline is targeting Gemini/OpenAI, injected codebase context will be in XML format while the rest of the structure is markdown — a formatting inconsistency.

### Risk: `OpenAIClient.LastResponseID` is not thread-safe

`openai_client.go:22` declares `LastResponseID string` as a plain field on `OpenAIClient`. If a single client is used concurrently (which could happen if the same `HybridEngine` is shared across goroutines), `LastResponseID` reads/writes are unprotected. The `HybridEngine` itself has no mutex. This is a data race risk under concurrent use.

### Risk: `enforceOutputFormat()` uses XML `<output_format>` for all targets

`pipeline_format.go:148` injects `<output_format>` XML tags even when the target provider is Gemini or OpenAI. These XML tags are unlikely to be problematic, but they create inconsistency with the markdown-structured output of Stage 6 for non-Claude targets.

---

## 10. File Reference Summary

| File | Role |
|------|------|
| `enhancer.go` | Pipeline orchestrator (`EnhanceWithConfig`), `Analyze()`, `WrapWithExamples()` |
| `pipeline.go` | Stages 1–3, Stage 6 (XML/markdown structure, `roleForTaskType`, `constraintsForTaskType`) |
| `pipeline_format.go` | Stages 4, 8, 10, 11, 12 (overtrigger, format enforcement, self-check, overengineering guard, preamble suppression) |
| `scoring.go` | 10-dimension `Score()`, `ScoreReport`, `DimensionScore`, grading thresholds |
| `lint.go` | 11 lint rules (Rules 1–11), `Lint()`, `VerifyCacheFriendlyOrder()` partially |
| `context.go` | Stage 7 (`ReorderLongContext`), Stage 9 (`InjectQuoteGrounding`), cache rules, `EstimateTokens` |
| `filter.go` | `ShouldEnhance()`, `hasWeakDimensions()`, conversational/structured/filepath gates |
| `config.go` | `Config`, `LLMConfig`, `LoadConfig`, `ResolveConfig`, stage disable, rules engine |
| `hybrid.go` | `HybridEngine`, `EnhanceHybrid()`, mode dispatch, circuit/cache integration |
| `provider.go` | `PromptImprover` interface, `NewPromptImprover()`, provider constants, `defaultTargetProviderForLLM` |
| `llmclient.go` | Claude Anthropic SDK client, `LLMClient.Improve()`, prompt caching via `cache_control` |
| `gemini_client.go` | Gemini REST client, `GeminiClient.Improve()`, `CreateCachedContent()` |
| `openai_client.go` | OpenAI Responses API client, `OpenAIClient.Improve()`, `LastResponseID` multi-turn |
| `circuit.go` | `CircuitBreaker` (3 failures → 60s cooldown), `Allow()`, `RecordSuccess/Failure()`, `Reset()` |
| `cache.go` | `PromptCache` (100 entries, O(n) eviction), `EnhancerCache` (LRU, O(1) eviction) |
| `classifier.go` | `Classify()`, `ClassifyDetailed()`, 6 task types, keyword + phrase scoring |
| `sampling.go` | `SamplingEngine` (MCP Sampling mode), `SamplingScore()` |
| `backoff.go` | `DefaultBackoff()`, `retryImprove()`, `isRetryableError()`, full-jitter exponential backoff |
| `metaprompt.go` | 3 provider-specific meta-prompts + thinking variants, `MetaPromptFor()` |
| `templates.go` | 6 builtin templates (troubleshoot, code_review, workflow_create, data_analysis, code, creative_brief) |
| `examples.go` | Stage 5: `DetectAndWrapExamples()`, 3 detection strategies |
| `claudemd.go` | `CheckClaudeMD()`: 6 CLAUDE.md health checks |
| `knowledge/graph.go` | In-memory directed code entity graph, `RelatedContext()` keyword matching |
| `knowledge/injector.go` | `InjectContext()` (XML) + `FormatContextBlockMarkdown()` (unused) |
