# Cline Free Model Integration — Implementation Prompt

> **Target:** ralphglasses repo (`~/hairglasses-studio/ralphglasses`)
> **Generated:** 2026-04-09
> **Objective:** Wire Cline's 4 free models (GLM-5, MiniMax M2.5, KAT Coder Pro, Arcee Trinity) into ralphglasses cascade routing, bandit selection, loop profiles, and cost tracking with benchmark-calibrated priors.

---

## Context: What Exists Today

### Current Model Tier System (`internal/session/cascade_routing.go`)

The cascade router uses a `ModelTier` struct with 5 tiers, but Cline free models are a single undifferentiated entry:

```go
func DefaultModelTiers() []ModelTier {
    return []ModelTier{
        {Provider: ProviderCline, Model: "cline-free", MaxComplexity: 1, CostPer1M: 0.0, Label: "free"},
        {Provider: ProviderGemini, Model: "gemini-3.1-flash-lite", MaxComplexity: 1, CostPer1M: CostGeminiFlashLiteInput, Label: "ultra-cheap"},
        {Provider: ProviderGemini, Model: "gemini-3.1-flash", MaxComplexity: 2, CostPer1M: CostGeminiFlashInput, Label: "worker"},
        {Provider: ProviderCodex, Model: "gpt-5.4", MaxComplexity: 3, CostPer1M: CostCodexInput, Label: "coding"},
        {Provider: ProviderClaude, Model: "claude-opus", MaxComplexity: 4, CostPer1M: CostClaudeOpusInput, Label: "reasoning"},
    }
}
```

### Current Cascade Config (`internal/session/cascade.go`)

```go
func DefaultCascadeConfig() CascadeConfig {
    return CascadeConfig{
        Enabled:             true,
        CheapProvider:       ProviderCline,
        ExpensiveProvider:   DefaultPrimaryProvider(),
        ConfidenceThreshold: 0.7,
        CascadeThreshold:    0.5,
        MaxCheapBudgetUSD:   0.0, // Cline free tier: no cost cap needed
        MaxCheapTurns:       20,
        TaskTypeOverrides: map[string]Provider{
            "architecture": ProviderClaude,
            "planning":     ProviderClaude,
        },
    }
}
```

### Current Loop Profiles (`internal/session/loop_types.go`)

- `DefaultLoopProfile()` — Codex for all lanes
- `SelfImprovementProfile()` — Codex for all lanes, $5 planner / $15 worker
- `BudgetOptimizedSelfImprovementProfile(totalBudget)` — Codex, scaled budgets
- `ResearchLoopProfile(dailyBudget)` — Gemini Flash planner, Claude Haiku worker, Gemini Flash-Lite verifier
- **No free-model profile exists**

### Current Cost Rate Map (`internal/session/costnorm.go`)

`ProviderCline` is **missing** from `ProviderCostRates` map and from the `LoadCostRatesFromDir` provider scan loop:

```go
var ProviderCostRates = map[Provider]CostRate{
    ProviderClaude: {InputPer1M: CostClaudeSonnetInput, OutputPer1M: CostClaudeSonnetOutput},
    ProviderGemini: {InputPer1M: CostGeminiFlashInput, OutputPer1M: CostGeminiFlashOutput},
    ProviderCodex:  {InputPer1M: CostCodexInput, OutputPer1M: CostCodexOutput},
    ProviderCrush:  {InputPer1M: CostClaudeSonnetInput, OutputPer1M: CostClaudeSonnetOutput},
    ProviderGoose:  {InputPer1M: CostClaudeSonnetInput, OutputPer1M: CostClaudeSonnetOutput},
    ProviderAmp:    {InputPer1M: CostClaudeSonnetInput, OutputPer1M: CostClaudeSonnetOutput},
    ProviderA2A:    {InputPer1M: CostClaudeSonnetInput, OutputPer1M: CostClaudeSonnetOutput},
}
```

### Current Bandit System (`internal/session/cascade_bandit.go`)

`NewBanditRouter(tiers, cfg)` creates one arm per `ModelTier`. Arms start with uniform priors (`alpha: 1.0, beta: 1.0`). No benchmark-calibrated initialization exists.

### Current Provider Builder (`internal/session/provider_cline.go`)

`buildClineCmd()` already passes `--model` flag through to Cline CLI when `opts.Model != ""`. Cline accepts model IDs in the format `provider/model-name` (e.g., `z-ai/glm-5`, `minimax/minimax-m2.5`).

### Task Complexity Map (`internal/session/cascade_routing.go`)

```go
var taskTypeComplexity = map[string]int{
    "lint": 1, "format": 1, "classify": 1, "docs": 1,
    "config": 2, "review": 2, "optimization": 2, "bug_fix": 2,
    "codegen": 3, "test": 3, "feature": 3, "refactor": 3, "general": 2,
    "architecture": 4, "analysis": 4, "planning": 4,
}
```

---

## Research Data: Cline Free Model Benchmarks

Source: `~/hairglasses-studio/docs/research/cost-optimization/cline-model-benchmarks-2026.md`

### Cline Native Free Models — Benchmark-Ranked

| Rank | Model ID (Cline format) | Coding Score | LiveCodeBench | SWE-bench Verified | Best Domain |
|------|-------------------------|-------------|---------------|-------------------|-------------|
| 🥇 1 | `z-ai/glm-5` | **39.0** | 52% | ~62% (reasoning) | Coding, reasoning |
| 🥈 2 | `minimax/minimax-m2.5` | **37.4** | ~35% | ~45% | General, creative, structured output |
| 🥉 3 | `arcee-ai/trinity-large-preview:free` | **~30** | ~25% | — | General reasoning, 400B MoE |
| 4 | `kwaipilot/kat-coder-pro` | **18.3** | ~15% | — | Math, numerical |

### Domain Strengths (Star Ratings from Research)

| Model | Coding | Reasoning | Math | Creative | Speed | Agentic |
|-------|--------|-----------|------|----------|-------|---------|
| GLM-5 | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |
| MiniMax M2.5 | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ |
| Arcee Trinity | ⭐⭐ | ⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐ | ⭐⭐ | ⭐⭐ |
| KAT Coder Pro | ⭐⭐ | ⭐⭐ | ⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐ | ⭐⭐ |

### Fleet Tier Strategy (from Research §6)

```
L4 — FREE (Perpetual R&D / Background)
  GLM-5 / MiniMax M2.5 / DeepSeek R1:free
  Cost: $0.00 │ Coding: 30-39
  Use: Perpetual R&D, triage, docs, experiments
```

---

## Implementation Tasks

### Task 1: Expand `DefaultModelTiers()` with Individual Cline Free Models

**File:** `internal/session/cascade_routing.go`

Replace the single `cline-free` entry with 4 specific model tiers, ordered by coding capability:

```go
func DefaultModelTiers() []ModelTier {
    return []ModelTier{
        // L4: Cline free tier — 4 models ranked by coding benchmark score.
        // All $0.00 via Cline's free tier auth. Model IDs match Cline CLI --model format.
        {Provider: ProviderCline, Model: "z-ai/glm-5", MaxComplexity: 2, CostPer1M: 0.0, Label: "free-coding"},
        {Provider: ProviderCline, Model: "minimax/minimax-m2.5", MaxComplexity: 2, CostPer1M: 0.0, Label: "free-general"},
        {Provider: ProviderCline, Model: "arcee-ai/trinity-large-preview:free", MaxComplexity: 1, CostPer1M: 0.0, Label: "free-moe"},
        {Provider: ProviderCline, Model: "kwaipilot/kat-coder-pro", MaxComplexity: 1, CostPer1M: 0.0, Label: "free-math"},
        // L3: Ultra-cheap paid tier
        {Provider: ProviderGemini, Model: "gemini-3.1-flash-lite", MaxComplexity: 1, CostPer1M: CostGeminiFlashLiteInput, Label: "ultra-cheap"},
        // L2: Worker tier
        {Provider: ProviderGemini, Model: "gemini-3.1-flash", MaxComplexity: 2, CostPer1M: CostGeminiFlashInput, Label: "worker"},
        // L1: Coding tier
        {Provider: ProviderCodex, Model: "gpt-5.4", MaxComplexity: 3, CostPer1M: CostCodexInput, Label: "coding"},
        // L1: Reasoning tier
        {Provider: ProviderClaude, Model: "claude-opus", MaxComplexity: 4, CostPer1M: CostClaudeOpusInput, Label: "reasoning"},
    }
}
```

**Key design decisions:**
- GLM-5 and MiniMax M2.5 get `MaxComplexity: 2` because their coding scores (39.0 and 37.4) are in the same range as Gemini Flash (which also handles complexity 2 tasks like config, review, bug_fix)
- Arcee Trinity and KAT Coder Pro stay at `MaxComplexity: 1` (lint, format, classify, docs only)
- `SelectTier()` already picks the cheapest tier that can handle the complexity, so free models are automatically preferred for complexity 1-2 tasks
- When multiple tiers have the same `CostPer1M` (0.0), tie-breaking is by iteration order — GLM-5 appears first, so it wins ties. This is correct since it has the highest coding score.

**Also update** the `TestDefaultModelTiers` test in `internal/session/cascade_test.go`:
- Change expected tier count from 5 to 8
- Add assertions for the new free model tiers

### Task 2: Add `SelectClineFreeModel()` Method

**File:** `internal/session/cascade_routing.go`

Add a method that selects the best Cline free model for a given task type, using domain-aware routing:

```go
// ClineFreeModelRanking defines the preferred Cline free model order per domain.
// Keys are domain categories; values are ordered model IDs (best first).
var ClineFreeModelRanking = map[string][]string{
    "coding":  {"z-ai/glm-5", "minimax/minimax-m2.5", "arcee-ai/trinity-large-preview:free", "kwaipilot/kat-coder-pro"},
    "general": {"minimax/minimax-m2.5", "z-ai/glm-5", "arcee-ai/trinity-large-preview:free", "kwaipilot/kat-coder-pro"},
    "math":    {"kwaipilot/kat-coder-pro", "z-ai/glm-5", "minimax/minimax-m2.5", "arcee-ai/trinity-large-preview:free"},
    "docs":    {"minimax/minimax-m2.5", "arcee-ai/trinity-large-preview:free", "z-ai/glm-5", "kwaipilot/kat-coder-pro"},
    "reason":  {"z-ai/glm-5", "arcee-ai/trinity-large-preview:free", "minimax/minimax-m2.5", "kwaipilot/kat-coder-pro"},
}

// taskTypeDomain maps task types to domain categories for free model selection.
var taskTypeDomain = map[string]string{
    "lint": "coding", "format": "coding", "codegen": "coding", "test": "coding",
    "feature": "coding", "refactor": "coding", "bug_fix": "coding",
    "docs": "docs", "classify": "general", "config": "general",
    "review": "reason", "optimization": "coding", "general": "general",
    "architecture": "reason", "analysis": "reason", "planning": "reason",
}

// SelectClineFreeModel returns the best Cline free model for a given task type.
// Falls back to GLM-5 (best overall free coding model) for unknown task types.
func SelectClineFreeModel(taskType string) string {
    domain := taskTypeDomain[taskType]
    if domain == "" {
        domain = "coding" // default to coding domain
    }
    ranking := ClineFreeModelRanking[domain]
    if len(ranking) == 0 {
        return "z-ai/glm-5" // absolute fallback
    }
    return ranking[0]
}
```

### Task 3: Wire Free Model Selection into `CheapLaunchOpts()`

**File:** `internal/session/cascade.go`

Update `CheapLaunchOpts()` to populate `opts.Model` with the best free model when the cheap provider is Cline and no model is explicitly set:

```go
// CheapLaunchOpts returns launch options modified for the cheap provider.
func (cr *CascadeRouter) CheapLaunchOpts(base LaunchOptions) LaunchOptions {
    opts := base
    opts.Provider = cr.config.CheapProvider

    // When using Cline as cheap provider, select the best free model
    // for the task type if no model is explicitly set.
    if cr.config.CheapProvider == ProviderCline && opts.Model == "" {
        // Classify the prompt to determine task type for model selection.
        taskType := classifyTask(opts.Prompt)
        opts.Model = SelectClineFreeModel(taskType)
    }

    if cr.config.MaxCheapBudgetUSD > 0 {
        opts.MaxBudgetUSD = cr.config.MaxCheapBudgetUSD
    }
    if cr.config.MaxCheapTurns > 0 {
        opts.MaxTurns = cr.config.MaxCheapTurns
    }

    opts.SessionName = opts.SessionName + "-cheap"

    return opts
}
```

**Note:** Check if `classifyTask()` exists in the codebase. If it does, use it. If not, extract the task type from the prompt using the existing `taskTypeComplexity` map keys as patterns. The simplest approach is to add a `TaskType` field to `LaunchOptions` that callers can set, or accept a `taskType` parameter on `CheapLaunchOpts`.

If `CheapLaunchOpts` signature can't change (backward compat), add `CheapLaunchOptsForTask(base LaunchOptions, taskType string)` as the new API:

```go
// CheapLaunchOptsForTask returns launch options for the cheap provider with
// task-type-aware free model selection.
func (cr *CascadeRouter) CheapLaunchOptsForTask(base LaunchOptions, taskType string) LaunchOptions {
    opts := cr.CheapLaunchOpts(base)
    if cr.config.CheapProvider == ProviderCline && opts.Model == "" {
        opts.Model = SelectClineFreeModel(taskType)
    }
    return opts
}
```

### Task 4: Add `ProviderCline` to Cost Rate Map

**File:** `internal/session/costnorm.go`

Add `ProviderCline` to the `ProviderCostRates` map:

```go
var ProviderCostRates = map[Provider]CostRate{
    ProviderClaude: {InputPer1M: CostClaudeSonnetInput, OutputPer1M: CostClaudeSonnetOutput},
    ProviderGemini: {InputPer1M: CostGeminiFlashInput, OutputPer1M: CostGeminiFlashOutput},
    ProviderCodex:  {InputPer1M: CostCodexInput, OutputPer1M: CostCodexOutput},
    ProviderCline:  {InputPer1M: 0.0, OutputPer1M: 0.0}, // Cline free tier: $0 (uses Cline auth, not direct API)
    ProviderCrush:  {InputPer1M: CostClaudeSonnetInput, OutputPer1M: CostClaudeSonnetOutput},
    ProviderGoose:  {InputPer1M: CostClaudeSonnetInput, OutputPer1M: CostClaudeSonnetOutput},
    ProviderAmp:    {InputPer1M: CostClaudeSonnetInput, OutputPer1M: CostClaudeSonnetOutput},
    ProviderA2A:    {InputPer1M: CostClaudeSonnetInput, OutputPer1M: CostClaudeSonnetOutput},
}
```

**Also update** the `LoadCostRatesFromDir` function to include `ProviderCline` in the provider scan loop:

```go
for _, provider := range []Provider{ProviderClaude, ProviderGemini, ProviderCodex, ProviderCline, ProviderCrush, ProviderGoose, ProviderAmp, ProviderA2A} {
```

**Also update** the test `TestProviderCostRatesAllProvidersPresent` in `internal/session/costnorm_test.go` to include `ProviderCline`.

### Task 5: Add `FreeResearchLoopProfile()` to Loop Types

**File:** `internal/session/loop_types.go`

Add a new profile for perpetual 24/7 free-model R&D:

```go
// FreeResearchLoopProfile returns a loop profile that uses exclusively Cline free
// models for perpetual background R&D at zero cost. GLM-5 (best free coding model)
// handles planning and working; MiniMax M2.5 (best free general model) handles
// verification for diversity of perspective.
//
// This profile is designed for L4 background tasks: triage, docs, boilerplate,
// experiments, and low-stakes code generation. Tasks that fail verification are
// escalated to paid tiers via cascade routing.
//
// Benchmark basis:
//   - GLM-5: Coding score 39.0, LiveCodeBench 52%, SWE-bench ~62%
//   - MiniMax M2.5: Coding score 37.4, good structured output, fast inference
//   - See: ~/hairglasses-studio/docs/research/cost-optimization/cline-model-benchmarks-2026.md
func FreeResearchLoopProfile() LoopProfile {
    return LoopProfile{
        PlannerProvider:      ProviderCline,
        PlannerModel:         "z-ai/glm-5",
        WorkerProvider:       ProviderCline,
        WorkerModel:          "z-ai/glm-5",
        VerifierProvider:     ProviderCline,
        VerifierModel:        "minimax/minimax-m2.5",
        MaxConcurrentWorkers: 1,
        RetryLimit:           2,
        VerifyCommands:       []string{defaultLoopVerifyCommand},
        WorktreePolicy:       "none",
        PlannerBudgetUSD:     0.0, // free tier
        WorkerBudgetUSD:      0.0, // free tier
        VerifierBudgetUSD:    0.0, // free tier
        HardBudgetCapUSD:     0.0, // no cost cap for free models
        NoopPlateauLimit:     5,   // generous: free models may need more attempts
        EnableCascade:        true, // escalate to paid on verify failure
        EnableReflexion:      true,
        CompactionEnabled:    true,
        CompactionThreshold:  3,
        AutoMergeAll:         false, // require verification for free model output
        MaxIterations:        100,   // high iteration count for background R&D
        MaxDurationSecs:      86400, // 24 hours
        StallTimeout:         15 * time.Minute,
        ReviewPatience:       5, // more patience for free models
    }
}
```

### Task 6: Add Benchmark-Calibrated Bandit Priors

**File:** `internal/session/cascade_bandit.go`

Add a `BenchmarkPrior` struct and wire it into `NewBanditRouter`:

```go
// BenchmarkPrior provides an informed initial prior for a bandit arm based on
// external benchmark data. This replaces the uniform (alpha=1, beta=1) default
// with calibrated values so the bandit starts with useful signal instead of
// wasting exploration budget on uniform random selection.
type BenchmarkPrior struct {
    ArmLabel   string  // matches ModelTier.Label
    AlphaBoost float64 // added to alpha (prior successes)
    BetaBoost  float64 // added to beta (prior failures)
}

// ClineFreeModelBenchmarkPriors returns benchmark-calibrated priors for Cline
// free model arms. Scores are derived from composite coding benchmarks:
//   - GLM-5: 39.0/100 → alpha boost ~4.0, beta boost ~6.0
//   - MiniMax M2.5: 37.4/100 → alpha boost ~3.7, beta boost ~6.3
//   - Arcee Trinity: ~30/100 → alpha boost ~3.0, beta boost ~7.0
//   - KAT Coder Pro: 18.3/100 → alpha boost ~1.8, beta boost ~8.2
//
// The scale factor (10x) gives the equivalent of ~10 prior observations,
// matching the default MinSamples threshold. This means the bandit can
// override static routing immediately with informed signal.
func ClineFreeModelBenchmarkPriors() []BenchmarkPrior {
    return []BenchmarkPrior{
        {ArmLabel: "free-coding", AlphaBoost: 4.0, BetaBoost: 6.0},   // GLM-5
        {ArmLabel: "free-general", AlphaBoost: 3.7, BetaBoost: 6.3},  // MiniMax M2.5
        {ArmLabel: "free-moe", AlphaBoost: 3.0, BetaBoost: 7.0},      // Arcee Trinity
        {ArmLabel: "free-math", AlphaBoost: 1.8, BetaBoost: 8.2},     // KAT Coder Pro
    }
}
```

Add a `BenchmarkPriors` field to `BanditRouterConfig`:

```go
type BanditRouterConfig struct {
    Window            int              `json:"window"`
    LearningRate      float64          `json:"learning_rate"`
    MinSamples        int              `json:"min_samples"`
    SuccessWindowSize int              `json:"success_window_size"`
    BenchmarkPriors   []BenchmarkPrior `json:"benchmark_priors,omitempty"` // calibrated initial priors
}
```

Update `NewBanditRouter` to apply priors:

```go
func NewBanditRouter(tiers []ModelTier, cfg BanditRouterConfig) *BanditRouter {
    // ... existing code ...

    br := &BanditRouter{
        policy:         bandit.NewContextualThompson(arms, cfg.Window, cfg.LearningRate),
        arms:           arms,
        armMap:         armMap,
        minSamples:     cfg.MinSamples,
        successTracker: successTracker,
    }

    // Apply benchmark-calibrated priors if provided.
    if len(cfg.BenchmarkPriors) > 0 {
        br.applyBenchmarkPriors(cfg.BenchmarkPriors)
    }

    return br
}

// applyBenchmarkPriors sets informed initial priors on bandit arms.
func (br *BanditRouter) applyBenchmarkPriors(priors []BenchmarkPrior) {
    for _, prior := range priors {
        // The ContextualThompson policy stores alpha/beta internally.
        // We need to inject priors through the Update mechanism by
        // recording synthetic observations.
        armID := prior.ArmLabel
        if _, ok := br.armMap[armID]; !ok {
            continue
        }
        // Record synthetic successes (alpha boost).
        for i := 0; i < int(prior.AlphaBoost); i++ {
            br.policy.Update(bandit.Reward{
                ArmID: armID,
                Value: 0.8, // above 0.5 threshold → increments alpha
            })
        }
        // Record synthetic failures (beta boost).
        for i := 0; i < int(prior.BetaBoost); i++ {
            br.policy.Update(bandit.Reward{
                ArmID: armID,
                Value: 0.2, // below 0.5 threshold → increments beta
            })
        }
    }
}
```

**Wire in the default config:**

```go
func DefaultBanditRouterConfig() BanditRouterConfig {
    return BanditRouterConfig{
        Window:            100,
        LearningRate:      0.1,
        MinSamples:        10,
        SuccessWindowSize: 50,
        BenchmarkPriors:   ClineFreeModelBenchmarkPriors(),
    }
}
```

### Task 7: Update `.ralphrc` with Free Model Cascade Defaults

**File:** `.ralphrc`

Add these keys:

```bash
# Cline free model cascade routing
CASCADE_CLINE_MODEL_ORDER="z-ai/glm-5,minimax/minimax-m2.5,arcee-ai/trinity-large-preview:free,kwaipilot/kat-coder-pro"
CASCADE_FREE_FIRST="true"
```

**File:** `internal/session/cascade.go`

Update `DefaultCascadeFromConfig()` to parse the new keys:

```go
if v, ok := cfg["CASCADE_CLINE_MODEL_ORDER"]; ok && v != "" {
    // Stored as ClineModelOrder for downstream use by SelectClineFreeModel.
    // The cascade router can use this to override the compiled-in preference.
    defaults.ClineModelOrder = strings.Split(v, ",")
}
if v, ok := cfg["CASCADE_FREE_FIRST"]; ok {
    if strings.ToLower(strings.TrimSpace(v)) == "true" {
        defaults.CheapProvider = ProviderCline
    }
}
```

Add the new field to `CascadeConfig`:

```go
type CascadeConfig struct {
    Enabled              bool                `json:"enabled"`
    CheapProvider        Provider            `json:"cheap_provider"`
    ExpensiveProvider    Provider            `json:"expensive_provider"`
    ConfidenceThreshold  float64             `json:"confidence_threshold"`
    CascadeThreshold     float64             `json:"cascade_threshold"`
    MaxCheapBudgetUSD    float64             `json:"max_cheap_budget_usd"`
    MaxCheapTurns        int                 `json:"max_cheap_turns"`
    TaskTypeOverrides    map[string]Provider `json:"task_type_overrides"`
    SpeculativeExecution bool                `json:"speculative_execution"`
    LatencyThresholdMs   int                 `json:"latency_threshold_ms"`
    ClineModelOrder      []string            `json:"cline_model_order,omitempty"` // NEW: ordered free model preference
}
```

### Task 8: Create `.ralph/cost_rates.json` Template

**File:** `.ralph/cost_rates.json` (create if not exists)

```json
{
  "input_per_m_token": {
    "cline": 0.00,
    "cline_glm5": 0.00,
    "cline_minimax_m25": 0.00,
    "cline_arcee_trinity": 0.00,
    "cline_kat_coder": 0.00,
    "claude_sonnet": 3.00,
    "claude_opus": 5.00,
    "gemini_flash": 0.50,
    "gemini_flash_lite": 0.10,
    "codex": 2.50
  },
  "output_per_m_token": {
    "cline": 0.00,
    "cline_glm5": 0.00,
    "cline_minimax_m25": 0.00,
    "cline_arcee_trinity": 0.00,
    "cline_kat_coder": 0.00,
    "claude_sonnet": 15.00,
    "claude_opus": 25.00,
    "gemini_flash": 3.00,
    "gemini_flash_lite": 0.40,
    "codex": 10.00
  }
}
```

### Task 9: Wire `FreeResearchLoopProfile` into MCP Tool Handler

**Search** for the MCP handler that implements `ralphglasses_loop_start` (likely in `internal/mcpserver/`). Add `"free"` or `"free-research"` as a recognized profile name that maps to `FreeResearchLoopProfile()`.

Example pattern (find the actual switch/map):
```go
case "free", "free-research", "free-rd":
    profile = session.FreeResearchLoopProfile()
```

### Task 10: Update Provider Capabilities

**Search** for `provider_capabilities.go` in `internal/session/`. If it has a capability registry that lists models per provider, update it to include the 4 Cline free models with their benchmark scores and domain strengths.

---

## Test Expectations

After all changes, these tests must pass:

### Existing Tests to Update

1. **`TestDefaultModelTiers`** (`internal/session/cascade_test.go`)
   - Update expected tier count: `5 → 8`
   - Verify first 4 tiers are all Cline free models with `CostPer1M: 0.0`
   - Verify ordering: GLM-5 before MiniMax before Arcee before KAT

2. **`TestProviderCostRatesAllProvidersPresent`** (`internal/session/costnorm_test.go`)
   - Add `ProviderCline` to the test loop

### New Tests to Add

3. **`TestSelectClineFreeModel`** (`internal/session/cascade_routing_test.go`)
   ```go
   func TestSelectClineFreeModel(t *testing.T) {
       tests := []struct {
           taskType string
           want     string
       }{
           {"codegen", "z-ai/glm-5"},      // coding domain → GLM-5
           {"docs", "minimax/minimax-m2.5"}, // docs domain → MiniMax
           {"review", "z-ai/glm-5"},         // reasoning domain → GLM-5
           {"", "z-ai/glm-5"},               // unknown → fallback to GLM-5
       }
       for _, tt := range tests {
           got := SelectClineFreeModel(tt.taskType)
           if got != tt.want {
               t.Errorf("SelectClineFreeModel(%q) = %q, want %q", tt.taskType, got, tt.want)
           }
       }
   }
   ```

4. **`TestFreeResearchLoopProfile`** (`internal/session/loop_types_test.go`)
   ```go
   func TestFreeResearchLoopProfile(t *testing.T) {
       p := FreeResearchLoopProfile()
       if p.PlannerProvider != ProviderCline {
           t.Errorf("PlannerProvider = %v, want cline", p.PlannerProvider)
       }
       if p.PlannerModel != "z-ai/glm-5" {
           t.Errorf("PlannerModel = %v, want z-ai/glm-5", p.PlannerModel)
       }
       if p.WorkerModel != "z-ai/glm-5" {
           t.Errorf("WorkerModel = %v, want z-ai/glm-5", p.WorkerModel)
       }
       if p.VerifierModel != "minimax/minimax-m2.5" {
           t.Errorf("VerifierModel = %v, want minimax/minimax-m2.5", p.VerifierModel)
       }
       if p.PlannerBudgetUSD != 0.0 {
           t.Errorf("PlannerBudgetUSD = %v, want 0.0", p.PlannerBudgetUSD)
       }
       if !p.EnableCascade {
           t.Error("EnableCascade should be true for escalation on verify failure")
       }
   }
   ```

5. **`TestBanditBenchmarkPriors`** (`internal/session/cascade_bandit_test.go`)
   ```go
   func TestBanditBenchmarkPriors(t *testing.T) {
       tiers := DefaultModelTiers()
       cfg := DefaultBanditRouterConfig()
       br := NewBanditRouter(tiers, cfg)

       stats := br.Stats()
       // GLM-5 arm should have higher alpha than KAT Coder Pro
       glm5 := stats["free-coding"]
       kat := stats["free-math"]
       if glm5.Alpha <= kat.Alpha {
           t.Errorf("GLM-5 alpha (%v) should be > KAT alpha (%v) from benchmark priors",
               glm5.Alpha, kat.Alpha)
       }
   }
   ```

6. **`TestClineCostRateZero`** (`internal/session/costnorm_test.go`)
   ```go
   func TestClineCostRateZero(t *testing.T) {
       rate, ok := getProviderCostRate(ProviderCline)
       if !ok {
           t.Fatal("ProviderCline not found in ProviderCostRates")
       }
       if rate.InputPer1M != 0.0 {
           t.Errorf("Cline InputPer1M = %v, want 0.0", rate.InputPer1M)
       }
       if rate.OutputPer1M != 0.0 {
           t.Errorf("Cline OutputPer1M = %v, want 0.0", rate.OutputPer1M)
       }
   }
   ```

---

## Verification Checklist

After implementing all tasks, verify:

- [ ] `make ci` passes (required quality gate)
- [ ] `go build ./...` compiles cleanly
- [ ] `go test ./internal/session/... -count=1` all pass
- [ ] `go test ./internal/bandit/... -count=1` all pass
- [ ] `go vet ./...` no warnings
- [ ] New `DefaultModelTiers()` returns 8 tiers (4 free + 4 paid)
- [ ] `SelectClineFreeModel("codegen")` returns `"z-ai/glm-5"`
- [ ] `SelectClineFreeModel("docs")` returns `"minimax/minimax-m2.5"`
- [ ] `FreeResearchLoopProfile()` uses Cline provider for all lanes
- [ ] `ProviderCostRates[ProviderCline]` exists with `{0.0, 0.0}`
- [ ] Bandit priors give GLM-5 higher alpha than KAT Coder Pro
- [ ] `.ralph/cost_rates.json` has entries for all 4 free models
- [ ] `.ralphrc` has `CASCADE_CLINE_MODEL_ORDER` and `CASCADE_FREE_FIRST` keys
- [ ] No existing test broken by the tier count change

---

## Architecture Diagram: Free Model Routing Flow

```
                    ┌──────────────────┐
                    │  Task Arrives     │
                    │  (MCP/Loop/Team)  │
                    └────────┬─────────┘
                             │
                    ┌────────▼─────────┐
                    │ Classify Task    │
                    │ Type & Complexity │
                    └────────┬─────────┘
                             │
                ┌────────────▼────────────┐
                │  CascadeRouter.SelectTier│
                │                          │
                │  complexity ≤ 2?         │
                │  ┌─yes──────────────┐    │
                │  │ Check free tiers │    │
                │  │ GLM-5 (coding)   │    │
                │  │ MiniMax (general) │    │
                │  │ Arcee (MoE)      │    │
                │  │ KAT (math)       │    │
                │  └──────────────────┘    │
                │  complexity > 2?         │
                │  ┌─yes──────────────┐    │
                │  │ Paid tiers       │    │
                │  │ Gemini Flash     │    │
                │  │ GPT-5.4          │    │
                │  │ Claude Opus      │    │
                │  └──────────────────┘    │
                └────────────┬────────────┘
                             │
                    ┌────────▼─────────┐
                    │  Bandit Override? │
                    │  (if ≥10 samples) │
                    │  Uses calibrated  │
                    │  benchmark priors │
                    └────────┬─────────┘
                             │
                    ┌────────▼─────────┐
                    │  Launch Session   │
                    │  via Cline CLI    │
                    │  --model z-ai/glm-5│
                    └────────┬─────────┘
                             │
              ┌──────────────▼──────────────┐
              │    Verify Result             │
              │    confidence < threshold?   │
              │    ┌─yes────────────────┐    │
              │    │ ESCALATE to paid   │    │
              │    │ (Codex/Claude)     │    │
              │    └────────────────────┘    │
              │    ┌─no─────────────────┐    │
              │    │ ACCEPT free result │    │
              │    │ Record bandit reward│    │
              │    └────────────────────┘    │
              └─────────────────────────────┘
```

---

## Files Modified (Summary)

| File | Change |
|------|--------|
| `internal/session/cascade_routing.go` | Expand `DefaultModelTiers()` to 8 tiers, add `SelectClineFreeModel()`, add domain ranking maps |
| `internal/session/cascade.go` | Add `ClineModelOrder` to `CascadeConfig`, update `CheapLaunchOpts()` / add `CheapLaunchOptsForTask()`, update `DefaultCascadeFromConfig()` |
| `internal/session/costnorm.go` | Add `ProviderCline` to `ProviderCostRates` map and `LoadCostRatesFromDir` loop |
| `internal/session/loop_types.go` | Add `FreeResearchLoopProfile()` |
| `internal/session/cascade_bandit.go` | Add `BenchmarkPrior`, `ClineFreeModelBenchmarkPriors()`, `applyBenchmarkPriors()`, update `BanditRouterConfig` and `DefaultBanditRouterConfig()` |
| `.ralphrc` | Add `CASCADE_CLINE_MODEL_ORDER`, `CASCADE_FREE_FIRST` |
| `.ralph/cost_rates.json` | Create with all free model entries |
| `internal/session/cascade_test.go` | Update `TestDefaultModelTiers` tier count |
| `internal/session/costnorm_test.go` | Add `ProviderCline` to provider presence test, add `TestClineCostRateZero` |
| `internal/session/cascade_routing_test.go` | Add `TestSelectClineFreeModel` |
| `internal/session/loop_types_test.go` | Add `TestFreeResearchLoopProfile` |
| `internal/session/cascade_bandit_test.go` | Add `TestBanditBenchmarkPriors` |
| MCP handler for `loop_start` | Add `"free"` / `"free-research"` profile name mapping |
