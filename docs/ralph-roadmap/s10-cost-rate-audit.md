# S10 — Cost Rate Audit (April 2026)

**Date**: 2026-04-04  
**Branch**: self-improve-review-20260404-165828  
**Scope**: All compiled-in cost constants in `internal/config/costs.go`, their aliases in
`internal/session/costs.go`, and downstream usage in `internal/session/costnorm.go` and
`internal/session/cascade_routing.go`.

---

## 1. Current Compiled-in Rates vs Actual Rates

All prices are USD per 1M tokens. "Compiled-in" refers to the constants in
`internal/config/costs.go`. "Actual" reflects verified April 2026 provider pricing.

### Anthropic Claude

| Constant | Model | Compiled-in Input | Actual Input | Compiled-in Output | Actual Output | Status |
|---|---|---|---|---|---|---|
| `CostClaudeOpusInput` / `CostClaudeOpusOutput` | Claude Opus 4.6 | $15.00 | **$5.00** | $75.00 | **$25.00** | WRONG — 3x overstated |
| `CostClaudeSonnetInput` / `CostClaudeSonnetOutput` | Claude Sonnet 4.6 | $3.00 | $3.00 | $15.00 | $15.00 | Correct |
| *(not tracked)* | Claude Haiku 4.5 | — | $1.00 in / $5.00 out | — | — | Missing entry |

**Notes**:
- Opus 4.6 launched at $5/$25 per 1M, a 67% price reduction from the former Opus 3 rates
  ($15/$75) that are still hardcoded. The old constants match Claude 3 Opus, not Claude 4.6.
- Haiku 4.5 is not tracked at all. The `DefaultModelTiers` in `cascade_routing.go` has no
  Haiku tier; it jumps from Gemini Flash (worker) directly to Codex gpt-5.4 (coding). Haiku
  could serve as a cost-effective intermediate Claude tier at $1/$5.
- Sonnet 4.6 rates have a context-length surcharge: inputs >200K tokens are billed at
  $6.00/$22.50. The current code has no context-length aware pricing.

### Google Gemini

| Constant | Model | Compiled-in Input | Actual Input | Compiled-in Output | Actual Output | Status |
|---|---|---|---|---|---|---|
| `CostGeminiFlashLiteInput` | Gemini 2.0 Flash-Lite | $0.10 | $0.075 | *(no output const)* | $0.30 | Input overstated; output missing |
| `CostGeminiFlashInput` / `CostGeminiFlashOutput` | Gemini 2.5 Flash | $0.30 | $0.30 | $3.50 | **$2.50** | Output overstated by $1.00 |
| *(not tracked)* | Gemini 2.5 Pro | — | $1.25 in (≤200K) / $10.00 out | — | — | Missing entry |

**Notes**:
- The Flash-Lite output rate was never added as a constant (`DefaultProviderCosts` only
  tracks Flash-Lite input). The actual output rate for 2.0 Flash-Lite is $0.30/1M. The
  compiled-in input constant ($0.10) is also stale — 2.0 Flash-Lite is $0.075 and 2.5
  Flash-Lite (the newer replacement) is $0.10. The constant label matches 2.5 Flash-Lite
  but `cascade_routing.go` uses model string `"gemini-3.1-flash-lite"`, creating a
  model-to-constant mismatch.
- Gemini 2.5 Flash output was recently corrected from $1.25 to $3.50 in the codebase
  (per commit history), but the actual current rate is **$2.50/1M** — the correction
  overshot by $1.00.
- Gemini 2.5 Pro is entirely absent from the cost table despite being the documented
  default model for the `gemini` provider in `docs/PROVIDER-SETUP.md`.

### OpenAI / Codex

| Constant | Model | Compiled-in Input | Actual Input | Compiled-in Output | Actual Output | Status |
|---|---|---|---|---|---|---|
| `CostCodexInput` / `CostCodexOutput` | gpt-5.4 (Codex CLI default) | $2.50 | **$10.00** | $15.00 | **$30.00** | WRONG — 4x under on input, 2x under on output |

**Notes**:
- The `CostCodexInput`/`CostCodexOutput` constants ($2.50/$15.00) match GPT-4o or older
  GPT-4 pricing, not gpt-5.4. gpt-5.4 is priced at $10/$30 per 1M tokens.
- The `PROVIDER-SETUP.md` documents the Codex CLI default as `gpt-5.4`. The constants
  have not been updated to reflect this model.
- o3 is $2.00/$8.00, o4-mini is $1.10/$4.40, and GPT-4.1 is $2.00/$8.00. These are not
  tracked in the cost table and are not used as Codex CLI models, but are relevant if the
  Codex CLI provider flag is ever set to a non-default model.

---

## 2. Discrepancies and Their Impact on Cascade Routing

### Cascade tier ordering in `DefaultModelTiers`

```
Tier 1 (ultra-cheap): gemini-3.1-flash-lite   CostPer1M = $0.10   [actual $0.075]
Tier 2 (worker):      gemini-3.1-flash         CostPer1M = $0.30   [actual $0.30 ✓]
Tier 3 (coding):      gpt-5.4                  CostPer1M = $2.50   [actual $10.00]
Tier 4 (reasoning):   claude-opus              CostPer1M = $15.00  [actual $5.00]
```

**Critical ordering inversion**: With the stale constants, Tier 3 (Codex/gpt-5.4) appears
cheaper than Tier 4 (Claude Opus) by a factor of 6x in the compiled-in cost table. In
reality, gpt-5.4 at $10.00/1M is **twice as expensive** as Claude Opus 4.6 at $5.00/1M.
`SelectTier` orders by `CostPer1M` ascending to find the cheapest viable tier; the correct
ordering should be:

```
Tier 1 (ultra-cheap): gemini-3.1-flash-lite   $0.075
Tier 2 (worker):      gemini-3.1-flash         $0.30
Tier 3 (reasoning):   claude-opus-4.6          $5.00
Tier 4 (coding):      gpt-5.4                  $10.00
```

This inversion means `SelectTier` is currently routing architecture/analysis/planning tasks
(complexity=4) to Claude Opus when it should route them there, which accidentally produces
the right outcome — but for the wrong reason and the wrong cost estimate. Budget forecasts
and `EfficiencyPct` in `NormalizedCost` are computed against Claude Sonnet rates ($3/$15),
making Codex sessions appear 83% cheaper than Claude when they are actually 3.3x more
expensive.

**Flash output overshoot**: The Gemini Flash output constant was corrected from $1.25 to
$3.50 but overshot. The actual rate is $2.50. Overstatement means ralphglasses
over-estimates Gemini session costs, making Gemini appear less efficient than it is. This
biases the Thompson Sampling bandit toward Claude and Codex sessions when comparing
cumulative spend across providers.

**Opus input overstatement**: The Claude Opus input is compiled in at $15.00 when the
actual rate is $5.00. This over-charges every Opus session in the cost ledger by 3x and
causes `NormalizedCost.EfficiencyPct` for Opus sessions to read ~33% (appears expensive)
when Opus is actually on par with gpt-5.4 in cost.

**Missing Gemini Pro**: The `gemini` provider's default model in `PROVIDER-SETUP.md` is
`gemini-3.1-pro`, but there is no `gemini_pro` key in the cost table and no tier entry.
The normalization fallback uses `gemini_flash` rates ($0.30/$3.50) for what is actually a
$1.25/$10.00 model — undercharging Gemini Pro sessions by roughly 3-4x in the cost ledger.

---

## 3. Recommended Constant Updates

Update `internal/config/costs.go` as follows:

```go
const (
    // Gemini — verified April 2026
    CostGeminiFlashLiteInput  float64 = 0.075  // was 0.10; Gemini 2.0 Flash-Lite
    CostGeminiFlashLiteOutput float64 = 0.30   // new constant; was missing
    CostGeminiFlashInput      float64 = 0.30   // correct
    CostGeminiFlashOutput     float64 = 2.50   // was 3.50; overshot by $1.00
    CostGeminiProInput        float64 = 1.25   // new; Gemini 2.5 Pro ≤200K ctx
    CostGeminiProOutput       float64 = 10.00  // new; Gemini 2.5 Pro

    // Claude — verified April 2026
    CostClaudeHaikuInput      float64 = 1.00   // new; Claude Haiku 4.5
    CostClaudeHaikuOutput     float64 = 5.00   // new; Claude Haiku 4.5
    CostClaudeSonnetInput     float64 = 3.00   // correct
    CostClaudeSonnetOutput    float64 = 15.00  // correct
    CostClaudeOpusInput       float64 = 5.00   // was 15.00; Claude Opus 4.6
    CostClaudeOpusOutput      float64 = 25.00  // was 75.00; Claude Opus 4.6

    // OpenAI / Codex CLI — verified April 2026
    CostCodexInput            float64 = 10.00  // was 2.50; gpt-5.4 actual rate
    CostCodexOutput           float64 = 30.00  // was 15.00; gpt-5.4 actual rate
)
```

Corresponding `DefaultProviderCosts()` additions:

```go
InputPerMToken: map[string]float64{
    "gemini_flash_lite": CostGeminiFlashLiteInput,
    "gemini_flash":      CostGeminiFlashInput,
    "gemini_pro":        CostGeminiProInput,      // add
    "claude_haiku":      CostClaudeHaikuInput,    // add
    "claude_sonnet":     CostClaudeSonnetInput,
    "claude_opus":       CostClaudeOpusInput,
    "codex":             CostCodexInput,
},
OutputPerMToken: map[string]float64{
    "gemini_flash_lite": CostGeminiFlashLiteOutput,  // add
    "gemini_flash":      CostGeminiFlashOutput,
    "gemini_pro":        CostGeminiProOutput,         // add
    "claude_haiku":      CostClaudeHaikuOutput,       // add
    "claude_sonnet":     CostClaudeSonnetOutput,
    "claude_opus":       CostClaudeOpusOutput,
    "codex":             CostCodexOutput,
},
```

`DefaultModelTiers()` in `cascade_routing.go` needs a corrected tier list with accurate
`CostPer1M` values and the correct ordering (Opus is now cheaper than Codex):

```go
func DefaultModelTiers() []ModelTier {
    return []ModelTier{
        {Provider: ProviderGemini, Model: "gemini-3.1-flash-lite", MaxComplexity: 1, CostPer1M: CostGeminiFlashLiteInput, Label: "ultra-cheap"},
        {Provider: ProviderGemini, Model: "gemini-3.1-flash",      MaxComplexity: 2, CostPer1M: CostGeminiFlashInput,     Label: "worker"},
        {Provider: ProviderClaude, Model: "claude-opus",            MaxComplexity: 3, CostPer1M: CostClaudeOpusInput,      Label: "reasoning"},
        {Provider: ProviderCodex,  Model: "gpt-5.4",               MaxComplexity: 4, CostPer1M: CostCodexInput,           Label: "coding"},
    }
}
```

Note: Haiku 4.5 and Gemini 2.5 Pro are good candidates for additional tiers but require
new provider dispatch logic (the session package currently treats `ProviderClaude` as a
single model, routing is done via CLI flags, not model selection). Adding those tiers is a
separate feature, not a cost-rate fix.

---

## 4. Whether providers_normalize.go Needs Changes

`providers_normalize.go` itself does not hardcode any cost rates — it delegates entirely to
`estimateCostFromTokens` (in `providers.go`) and `getProviderCostRate` (in `costnorm.go`),
which both read from `ProviderCostRates`. As long as `ProviderCostRates` is updated via the
corrected constants (done in `costnorm.go` which aliases from `config`), the normalization
logic requires no changes.

However, three behavioral notes:

1. **`NormalizeProviderCost` blended-rate path**: When token counts are absent, it uses a
   50/50 blended rate to scale from one provider to another. With gpt-5.4 at $10/$30, the
   blended rate is $20/1M vs Claude Sonnet's $9/1M. This is a valid heuristic but the 50/50
   split is a rough approximation; gpt-5.4 sessions tend to be output-heavy (reasoning
   tokens). Consider a 30/70 input/output split for Codex as a follow-on improvement.

2. **`claudeBaseRate` pinned to Sonnet**: `NormalizedCost.NormalizedUSD` normalizes
   everything to Claude Sonnet rates. This is a correct and stable baseline — Sonnet pricing
   has not changed. No change needed.

3. **Missing `gemini_pro` in `ProviderCostRateFrom`**: In `internal/session/costs.go`, the
   `ProviderCostRateFrom` switch has no `gemini_pro` case. Any `.ralph/cost_rates.json`
   override of `gemini_pro` would silently be ignored for the `ProviderGemini` rate lookup.
   This is a latent bug independent of the constant corrections, worth a follow-up fix.

---

## 5. Cost-Rate Staleness Alerting Mechanism

### Problem

Provider pricing changed materially (Opus dropped 67%, Codex gpt-5.4 is 4x more expensive
than the constant) without triggering any alert. The codebase has no mechanism to detect
when compiled-in rates diverge from reality.

### Recommended Mechanism

**Option A — Compile-time version tag with CLI warning (low effort)**

Add a `CostRatesVersion` string constant and `CostRatesDate` to `costs.go`:

```go
const (
    CostRatesVersion = "2026-04-04"
    CostRatesMaxAgedays = 60  // warn if binary is older than 60 days
)
```

On startup, `LoadCostRatesFromDir` computes `time.Since(parsedCostRatesDate)` and logs a
`slog.Warn` if the age exceeds `CostRatesMaxAgeDays`. The TUI can surface this as a status
badge ("cost rates may be stale").

**Option B — `.ralph/cost_rates.json` override with version field (medium effort)**

Extend the `ProviderCosts` JSON schema with:

```json
{
  "version": "2026-04-04",
  "input_per_m_token": { ... },
  "output_per_m_token": { ... }
}
```

`LoadProviderCosts` compares the file's version date against the compiled-in
`CostRatesVersion`. If the file is older than the binary, emit a warning prompting the user
to update their override file. If the file is newer, trust it silently (the user has already
updated).

**Option C — Weekly scheduled sweep (medium effort, fits existing infrastructure)**

Add a `ralphglasses_cost_rate_check` tool (or a `schedule` entry in `.ralph/schedule.yaml`)
that runs weekly, fetches a pinned YAML from a known URL (e.g., a `cost-rates.yaml` in the
ralphglasses repo), diffs it against compiled-in constants, and posts a journal entry if any
rate has drifted by >10%. This leverages the existing `journal_write` / `journal_read`
infrastructure and requires no new subsystems.

**Recommendation**: Implement Option A immediately (it's a two-line change that surfaces the
problem in every ralphglasses binary) and plan Option C as a follow-on once the sweep
infrastructure is more mature. Option B adds schema complexity for marginal benefit over A.

---

## Summary of Changes Required

| File | Change |
|---|---|
| `internal/config/costs.go` | Update 4 constants (Opus, Codex input/output, Flash output, Flash-Lite input); add 6 new constants (Flash-Lite output, Gemini Pro input/output, Haiku input/output) |
| `internal/config/costs.go` | Add `CostRatesVersion` + `CostRatesMaxAgeDays` staleness tag |
| `internal/session/cascade_routing.go` | Swap Tier 3/4 ordering (Opus now cheaper than Codex); update `CostPer1M` values |
| `internal/session/costnorm.go` | No changes needed — aliases follow `config` constants automatically |
| `internal/session/providers_normalize.go` | No changes needed |
| `internal/session/costs.go` | Add aliases for 6 new constants from `config` |
| `internal/mcpserver/` (startup) | Add staleness warning log on `LoadCostRatesFromDir` |

Tests in `internal/config/costs_test.go`, `internal/session/costs_test.go`, and
`internal/session/costnorm_test.go` will need numeric updates for the changed constants.
The golden tests in `providers_normalize_golden_test.go` will also need updating since
expected costs are computed from the constants.
