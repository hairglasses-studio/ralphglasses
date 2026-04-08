# Research Phase 3: Cost Management & Cross-Repo

**Date:** 2026-03-22
**ROADMAP Items:** 2.3, 5.5, 6.10, 7.4
**Status:** Research Complete

---

## 1. Executive Summary

Ralphglasses has a solid foundation for per-session cost tracking: `BudgetEnforcer` with headroom-based checks, JSONL cost ledger writes, and cross-provider cost normalization via `NormalizeProviderCost()`. The marathon supervisor adds budget-ceiling enforcement at the process level. However, fleet-scale cost management has significant gaps: no persistent global budget pool, no token-level cost breakdown, no cache-aware pricing, no burn-rate forecasting, and no reconciliation with actual billing data.

This document analyzes three external projects (`ryoppippi/ccusage`, `hagan/claudia-statusline`, `mcpkit/finops` + `rdcycle/profiles`) and maps their patterns to five concrete gaps across ROADMAP items 2.3, 5.5, 6.10, and 7.4. The primary recommendations are: (1) adopt token-level JSONL parsing from ccusage for ground-truth cost calculation, (2) port mcpkit/finops scoped budgets and windowed tracking for fleet-scale pool management, (3) implement SQLite-backed cost persistence with burn-rate math from claudia-statusline, (4) update the pricing table to match current Anthropic rates including cache token tiers, and (5) add cost forecasting via exponential moving average on ledger history.

---

## 2. Current State Analysis

### 2.1 Budget Enforcement (`internal/session/budget.go`)

The `BudgetEnforcer` provides secondary budget enforcement with a configurable headroom threshold (default 90%):

```
threshold = BudgetUSD * Headroom
exceeded  = SpentUSD >= threshold
```

**Strengths:**
- Thread-safe via `Session.mu` lock on every check
- Per-session cost ledger writes to `.ralph/logs/cost_ledger.jsonl` (append-only JSONL)
- Cost summary JSON output to `.ralph/cost_summary.json`
- `LedgerEntry` includes provider, model, turn count, elapsed time

**Gaps:**
- No token-level breakdown (only aggregate `spend_usd`)
- No cache token tracking (cache write vs. cache read pricing differs 12.5x)
- Headroom is hardcoded to 0.90; not configurable per session or fleet
- No global budget pool across sessions
- No time-windowed tracking (hourly/daily caps)
- Ledger entries written only at checkpoints, not continuously

### 2.2 Cost Normalization (`internal/session/costnorm.go`)

`NormalizeProviderCost()` normalizes cross-provider costs to a Claude-sonnet baseline:

```
NormalizedUSD = (inputTokens/1M * ClaudeInputRate) + (outputTokens/1M * ClaudeOutputRate)
EfficiencyPct = (RawCostUSD / NormalizedUSD) * 100
```

When token counts are unavailable, it estimates via blended-rate scaling:

```
providerBlended = (InputPer1M + OutputPer1M) / 2
claudeBlended   = (3.00 + 15.00) / 2 = 9.00
NormalizedUSD   = rawCostUSD * (claudeBlended / providerBlended)
```

**Current pricing table:**

| Provider | Input/MTok | Output/MTok | Source |
|----------|-----------|-------------|--------|
| Claude (sonnet) | $3.00 | $15.00 | Correct for Sonnet 4.6 |
| Gemini | $1.25 | $5.00 | Estimate for gemini-3.1-pro |
| Codex | $2.50 | $10.00 | Estimate for gpt-5 |

**Gaps:**
- No per-model pricing within providers (Opus 4.6 is $5/$25, Haiku 4.5 is $1/$5)
- No cache token pricing (cache hits are 0.1x base input = $0.30/MTok for Sonnet)
- No batch API pricing (50% discount)
- No extended thinking pricing
- 50/50 blended rate heuristic is inaccurate for agentic workloads (typically 30/70 input/output)
- `NormalizeSessionCost()` always passes `0, 0` for token counts, forcing blended estimation

### 2.3 Marathon Supervisor (`marathon.sh`)

Budget enforcement in the supervisor layer:

```bash
budget_ceiling = BUDGET * BUDGET_HEADROOM      # e.g., $100 * 0.90 = $90
spend          = read_status_field "session_spend_usd"
# Fallback: loop_count * COST_PER_CALL (default $0.15)
```

The supervisor also writes to `cost_ledger.jsonl` on each poll cycle (every 30s) when loop count changes:

```json
{"ts":"...","loop":N,"spend_usd":X,"elapsed_s":Y,"model":"sonnet","status":"running"}
```

**Gaps:**
- Cost-per-call fallback ($0.15) is a rough estimate with no model awareness
- No burn-rate calculation in the supervisor (only in the MCP handler)
- No budget carry-over between marathon sessions
- No multi-session budget coordination

### 2.4 Fleet Analytics (`internal/mcpserver/tools.go:handleMarathonDashboard`)

The marathon dashboard calculates burn rate:

```
burnRate = sum(session.SpentUSD / session.ElapsedHours)  // for running sessions
hoursEst = (totalBudget - totalUSD) / burnRate
```

`handleFleetAnalytics` provides per-provider and per-repo cost breakdowns with average cost per turn.

**Gaps:**
- Burn rate is instantaneous (no smoothing, no history)
- No cost projection beyond simple linear extrapolation
- All data is in-memory only; lost on restart
- No per-model cost analytics (only per-provider)

---

## 3. Gap Analysis

| Gap | Current State | Needed State | ROADMAP Item | Severity |
|-----|--------------|-------------|--------------|----------|
| **G1: No global budget pool** | Each session has independent `BudgetUSD` | Shared pool with per-session allocation and carry-over | 2.3.2, 5.5.1, 5.5.2 | High |
| **G2: No token-level cost breakdown** | Only aggregate `SpentUSD` from stream events | Per-turn token counts (input, output, cache_write, cache_read) | 2.3.1, 2.5.5.3 | High |
| **G3: No persistent cost storage** | In-memory `Session.SpentUSD`, JSONL ledger on disk | SQLite with queryable history, aggregations, time-range queries | 5.5.1, 6.10.1 | Medium |
| **G4: No cost forecasting** | Linear burn rate extrapolation | Historical regression, anomaly detection, exhaustion prediction | 6.10.1-6.10.5 | Medium |
| **G5: No cloud cost integration** | API costs only | Combined API + compute costs in unified budget | 7.4.1-7.4.5 | Low (Phase 7) |
| **G6: Stale pricing table** | Single rate per provider | Per-model rates including Opus/Sonnet/Haiku, cache tiers, batch | 2.5.5.3 | High |
| **G7: No threshold alerts** | Budget exceeded event only at 100% | Alerts at 50%, 75%, 90% with TUI notification | 2.3.3 | Medium |
| **G8: No windowed tracking** | Cumulative only | Hourly/daily/weekly spend windows with caps | 5.5.3 | Medium |

---

## 4. External Landscape

### 4.1 `ryoppippi/ccusage` -- JSONL Cost Parsing

**Repository:** https://github.com/ryoppippi/ccusage
**Language:** TypeScript (pnpm monorepo)
**Relevance:** Ground-truth cost calculation from Claude Code's local JSONL files

**Key findings:**

1. **JSONL Discovery:** Reads from `~/.config/claude/projects/**/usage.jsonl` and `~/.claude/projects/**/usage.jsonl`. Uses `CLAUDE_CONFIG_DIR` env var override. This is the canonical source of per-request token usage that Claude Code itself writes.

2. **Usage Record Schema:**
   ```
   timestamp              ISO 8601
   message.usage.input_tokens
   message.usage.output_tokens
   message.usage.cache_creation_input_tokens
   message.usage.cache_read_input_tokens
   message.usage.speed     "standard" | "fast"
   message.model
   message.id              deduplication key
   requestId               deduplication key
   costUSD                 pre-calculated (when available)
   sessionId
   version                 Claude Code version
   ```

3. **Three cost modes:**
   - **Auto (default):** Uses pre-calculated `costUSD` when available, falls back to token-based calculation
   - **Calculate:** Always compute from tokens, ignoring `costUSD` (useful for consistent historical comparison)
   - **Display:** Only show pre-calculated costs (for billing verification)

4. **Cost formula with tiered pricing:**
   ```
   calculateTieredCost(tokens, basePrice, tieredPrice, threshold=200_000):
     if tokens <= threshold:
       return tokens * basePrice
     return (threshold * basePrice) + ((tokens - threshold) * tieredPrice)

   totalCost = inputCost + outputCost + cacheCreationCost + cacheReadCost
   ```

5. **Session block grouping:** Groups entries into 5-hour billing windows (configurable). Detects gaps for idle period tracking. Calculates per-block burn rates (tokens/min, $/hr).

6. **Pricing source:** Fetches from LiteLLM's pricing database at runtime; falls back to prefetched Claude pricing for offline mode. Model matching uses provider prefix candidates (`anthropic/`, `openai/`, etc.).

7. **Deduplication:** Uses combined `messageId:requestId` hash to prevent double-counting across files.

**Applicability to ralphglasses:**
- The JSONL usage file path discovery is directly portable. Ralph sessions could read `~/.claude/projects/` to get ground-truth token costs retroactively (reconciliation).
- The four-token-type schema (input, output, cache_write, cache_read) should replace the current single `SpentUSD` field.
- The tiered pricing formula handles the 200k-token long-context pricing boundary that ralphglasses currently ignores.
- Session block grouping by 5-hour billing windows aligns with Anthropic's actual billing periods.

### 4.2 `hagan/claudia-statusline` -- SQLite Cost Schema & Burn Rate

**Repository:** https://github.com/hagan/claudia-statusline
**Language:** Rust
**Relevance:** Persistent cost storage, burn-rate calculation modes, status display format

**Key findings:**

1. **SQLite schema:**
   ```sql
   sessions (start_time, cost, lines_added, lines_removed, duration, ...)
   daily_stats (date, total_cost, total_lines_added, total_lines_removed, session_count)
   monthly_stats (month, total_cost, ...)
   session_archive (archived session data for auto_reset mode)
   context_learning (observed context usage patterns)
   ```
   Daily/monthly stats are rebuilt from session history:
   ```sql
   INSERT INTO daily_stats
   SELECT date(start_time), SUM(cost), SUM(lines_added), SUM(lines_removed), COUNT(*)
   FROM sessions GROUP BY date(start_time)
   ```

2. **Three burn-rate modes:**
   - **wall_clock (default):** `total_cost / wall_clock_duration` -- simple but includes idle time
   - **active_time:** Excludes idle periods (configurable threshold, default 60 min gap). Tracks inter-message timestamps. More accurate for intermittent usage.
   - **auto_reset:** Archives sessions after inactivity, resets counters. Useful for distinct work periods. Daily/monthly aggregations survive resets.

3. **Status display format:**
   ```
   ~/project [main +2 ~1] * 45% [====------] Sonnet * 1h 23m * +150 -42 * $3.50 ($2.54/h)
   ```
   Cost shown as: total spend + hourly burn rate. Multiple layout presets (compact, detailed, minimal, power).

4. **Cost data source:** Reads from Claude Code stdin JSON: `cost_usd`, `total_tokens`, `max_tokens`, `duration_ms`, `model`.

**Applicability to ralphglasses:**
- The SQLite schema pattern maps directly to ROADMAP 5.5.1 (global budget pool stored in SQLite). Using `modernc.org/sqlite` (pure Go, already used by internal SQLite project) avoids CGo.
- The active_time burn-rate mode is superior to the current instantaneous calculation in `handleMarathonDashboard`. Ralph should use inter-event timestamps to exclude idle periods.
- The daily/monthly aggregation pattern enables ROADMAP 5.5.3 (budget dashboard) and 6.10.1 (historical cost model).
- The auto_reset pattern maps to marathon checkpoint semantics -- archive costs at each checkpoint, start fresh counters.

### 4.3 `mcpkit/finops` + `rdcycle/profiles` -- Budget Profiles & Scoped Tracking

**Repository:** Local (`$HOME/hairglasses-studio/mcpkit`)
**Language:** Go
**Relevance:** Same org, designed to compose with ralph. Budget presets, scoped budgets, windowed tracking.

**Key findings:**

1. **CostPolicy:** Dollar-cost estimation with per-model pricing:
   ```go
   EstimateCost(model, inputTokens, outputTokens) float64 {
       inputCost  = float64(inputTokens) / 1000.0 * p.InputPer1KTokens
       outputCost = float64(outputTokens) / 1000.0 * p.OutputPer1KTokens
       return inputCost + outputCost
   }
   ```
   Thread-safe `RecordCost()` with budget enforcement. `RemainingBudget()` returns `math.MaxFloat64` when no budget set.

2. **ScopedTracker:** Per-tenant/user/session budget scoping:
   ```go
   BudgetScope { TenantID, UserID, SessionID }
   ScopedBudget { MaxTokens int, MaxDollars float64 }
   ```
   Maintains both global and per-scope trackers. ScopedMiddleware resolves identity from context, records to both trackers, checks scoped budgets post-execution.

3. **WindowedTracker:** Time-windowed usage tracking with lazy rotation:
   ```go
   ResetInterval: ResetHourly | ResetDaily | ResetWeekly | ResetMonthly
   WindowSummary { Start, End, TotalInput, TotalOutput, TotalCost }
   ```
   History retention with configurable max. Rotation happens on next access after window expiry.

4. **BudgetProfile presets (`rdcycle/profiles.go`):**
   ```go
   PersonalProfile: $5/cycle, $20/day, 500K tokens, 50 iterations
   WorkAPIProfile:  $50/cycle, $200/day, 5M tokens, 200 iterations
   ```
   `BuildFinOpsStack()` composes Tracker + CostPolicy + WindowedTracker from a profile.

5. **Model pricing (from rdcycle):**
   ```go
   claude-opus-4-6:   $0.015/1K input, $0.075/1K output  ($15/$75 per MTok)
   claude-sonnet-4-6: $0.003/1K input, $0.015/1K output  ($3/$15 per MTok)
   claude-haiku-4-5:  $0.0008/1K input, $0.004/1K output ($0.80/$4 per MTok)
   ```
   **Note:** These rates are for Opus 4.1-era pricing. Opus 4.6 is $5/$25 per MTok.

**Applicability to ralphglasses:**
- `ScopedTracker` maps directly to ROADMAP 5.5.1-5.5.2 (global pool + per-session limits). The `BudgetScope{TenantID, SessionID}` pattern translates to `BudgetScope{FleetID, SessionID}`.
- `WindowedTracker` provides ROADMAP 5.5.3 (daily/weekly spend windows) out of the box. Ralph could import mcpkit/finops directly or port the pattern.
- `BudgetProfile` presets map to ROADMAP 2.3.5. Ralph should define fleet-scale profiles (Marathon, Sprint, Overnight) alongside the existing Personal/WorkAPI.
- `BuildFinOpsStack()` is the composition pattern Ralph needs: profile -> (tracker, cost_policy, windowed_tracker).

### 4.4 Anthropic Pricing Reference (Current as of 2026-03-22)

| Model | Input/MTok | Output/MTok | Cache Write 5m | Cache Write 1h | Cache Hit | Batch Input | Batch Output |
|-------|-----------|-------------|---------------|---------------|-----------|-------------|-------------|
| Opus 4.6 | $5.00 | $25.00 | $6.25 | $10.00 | $0.50 | $2.50 | $12.50 |
| Sonnet 4.6 | $3.00 | $15.00 | $3.75 | $6.00 | $0.30 | $1.50 | $7.50 |
| Haiku 4.5 | $1.00 | $5.00 | $1.25 | $2.00 | $0.10 | $0.50 | $2.50 |
| Opus 4.5 | $5.00 | $25.00 | $6.25 | $10.00 | $0.50 | $2.50 | $12.50 |
| Sonnet 4.5 | $3.00 | $15.00 | $3.75 | $6.00 | $0.30 | $1.50 | $7.50 |

**Additional pricing dimensions:**
- **Fast mode (Opus 4.6):** 6x standard = $30/$150 per MTok
- **Long context (Sonnet 4.5/4, >200K input):** 2x input = $6/MTok, 1.5x output = $22.50/MTok
- **Data residency (US-only):** 1.1x multiplier on all token categories
- **Cache pricing formula:** Write = base * 1.25 (5m) or base * 2.0 (1h); Hit = base * 0.1

---

## 5. Actionable Recommendations

### R1: Add Per-Model Pricing Table with Cache Token Support

**Target:** `internal/session/costnorm.go`
**Effort:** Small (1-2 days)
**Impact:** High -- fixes incorrect cost normalization for Opus and Haiku sessions
**ROADMAP:** 2.5.5.3

Replace the single-rate-per-provider `ProviderCostRates` map with a per-model pricing structure:

```go
type ModelCostRate struct {
    InputPer1M       float64
    OutputPer1M      float64
    CacheWritePer1M  float64  // 5-minute cache write rate
    CacheReadPer1M   float64  // cache hit rate (0.1x input)
}

var ModelCostRates = map[string]ModelCostRate{
    "claude-opus-4-6":   {5.00, 25.00, 6.25, 0.50},
    "claude-sonnet-4-6": {3.00, 15.00, 3.75, 0.30},
    "claude-haiku-4-5":  {1.00, 5.00, 1.25, 0.10},
    "gemini-3.1-pro":      {1.25, 5.00, 0, 0},
    "gpt-5.4-xhigh":    {2.50, 10.00, 0, 0},
}
```

Update `NormalizedCost` to include token breakdown:

```go
type NormalizedCost struct {
    Provider           Provider
    Model              string
    RawCostUSD         float64
    InputTokens        int
    OutputTokens       int
    CacheWriteTokens   int
    CacheReadTokens    int
    NormalizedUSD      float64
    EfficiencyPct      float64
}
```

Add a `LookupModelRate(provider Provider, model string) ModelCostRate` function that falls back to provider defaults when the exact model is unknown.

### R2: Implement Global Budget Pool with Carry-Over

**Target:** New file `internal/session/budgetpool.go` + modifications to `internal/session/manager.go`
**Effort:** Medium (3-5 days)
**Impact:** High -- enables fleet-scale cost management
**ROADMAP:** 2.3.2, 5.5.1, 5.5.2

Implement a global budget pool inspired by mcpkit/finops `ScopedTracker`:

```go
type BudgetPool struct {
    mu              sync.RWMutex
    GlobalCeiling   float64                    // total fleet budget
    Allocations     map[string]float64          // session_id -> allocated budget
    Spent           map[string]float64          // session_id -> actual spend
    Headroom        float64                     // stop threshold (default 0.90)
    CarryOverPolicy CarryOverPolicy
}

type CarryOverPolicy int
const (
    CarryOverNone       CarryOverPolicy = iota  // unused budget returns to pool
    CarryOverRedistribute                        // redistribute to active sessions
    CarryOverAccumulate                          // carry forward to next period
)

func (p *BudgetPool) Allocate(sessionID string, requested float64) float64
func (p *BudgetPool) RecordSpend(sessionID string, amount float64) error
func (p *BudgetPool) Remaining() float64
func (p *BudgetPool) ReclaimUnused(sessionID string)
```

Allocation formula:
```
available    = GlobalCeiling * Headroom - sum(Spent)
allocated    = min(requested, available / activeSessionCount)
```

Carry-over on session completion:
```
unused       = Allocations[sessionID] - Spent[sessionID]
redistributed = unused / len(activeSessions)
for _, sid := range activeSessions:
    Allocations[sid] += redistributed
```

Wire into `Manager.Launch()` to check pool availability before starting, and into `Runner.handleEvent()` to call `RecordSpend()` on cost update events.

### R3: SQLite-Backed Cost Persistence with Burn-Rate History

**Target:** New file `internal/session/costdb.go`
**Effort:** Medium (3-5 days)
**Impact:** Medium -- enables historical queries, forecasting, and restart survival
**ROADMAP:** 5.5.1, 5.5.3, 6.10.1

Use `modernc.org/sqlite` (pure Go, no CGo -- already proven in internal SQLite project) for persistent cost storage:

```sql
CREATE TABLE cost_events (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp       TEXT NOT NULL,       -- ISO 8601
    session_id      TEXT NOT NULL,
    provider        TEXT NOT NULL,
    model           TEXT NOT NULL,
    input_tokens    INTEGER DEFAULT 0,
    output_tokens   INTEGER DEFAULT 0,
    cache_write_tokens INTEGER DEFAULT 0,
    cache_read_tokens  INTEGER DEFAULT 0,
    cost_usd        REAL NOT NULL,
    turn_number     INTEGER DEFAULT 0,
    elapsed_seconds REAL DEFAULT 0
);

CREATE TABLE cost_daily (
    date            TEXT NOT NULL,       -- YYYY-MM-DD
    provider        TEXT NOT NULL,
    model           TEXT NOT NULL,
    total_cost      REAL DEFAULT 0,
    total_input     INTEGER DEFAULT 0,
    total_output    INTEGER DEFAULT 0,
    session_count   INTEGER DEFAULT 0,
    PRIMARY KEY (date, provider, model)
);

CREATE TABLE budget_pools (
    pool_id         TEXT PRIMARY KEY,
    ceiling_usd     REAL NOT NULL,
    spent_usd       REAL DEFAULT 0,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE INDEX idx_cost_events_session ON cost_events(session_id);
CREATE INDEX idx_cost_events_ts ON cost_events(timestamp);
CREATE INDEX idx_cost_daily_date ON cost_daily(date);
```

Daily aggregation query (matching claudia-statusline pattern):
```sql
INSERT OR REPLACE INTO cost_daily (date, provider, model, total_cost, total_input, total_output, session_count)
SELECT date(timestamp), provider, model, SUM(cost_usd), SUM(input_tokens), SUM(output_tokens), COUNT(DISTINCT session_id)
FROM cost_events
WHERE date(timestamp) = ?
GROUP BY date(timestamp), provider, model;
```

Burn-rate query with active-time calculation:
```sql
SELECT session_id, SUM(cost_usd) as total,
       (julianday(MAX(timestamp)) - julianday(MIN(timestamp))) * 24 as hours,
       SUM(cost_usd) / NULLIF((julianday(MAX(timestamp)) - julianday(MIN(timestamp))) * 24, 0) as burn_rate
FROM cost_events
WHERE timestamp > datetime('now', '-24 hours')
GROUP BY session_id;
```

DB location: `~/.ralphglasses/costs.db` (global) or `.ralph/costs.db` (per-repo).

### R4: Cost Forecasting with Exponential Moving Average

**Target:** New file `internal/session/forecast.go`
**Effort:** Medium (3-5 days)
**Impact:** Medium -- enables proactive budget management
**ROADMAP:** 6.10.1, 6.10.2, 6.10.3, 6.10.4, 6.10.5

Implement cost forecasting using exponential moving average (EMA) on the cost ledger:

```go
type CostForecast struct {
    BurnRateEMA     float64   // $/hour, smoothed
    Alpha           float64   // EMA decay factor (default 0.3)
    HoursRemaining  float64   // at current burn rate
    ProjectedTotal  float64   // if session runs to budget
    AnomalyFactor   float64   // current_rate / EMA_rate (>2.0 = anomaly)
    Confidence      float64   // 0-1, based on sample count
}

func ComputeForecast(history []CostSnapshot, budget float64) CostForecast {
    // EMA burn rate: weight recent data more heavily
    // alpha = 2 / (N + 1), where N = smoothing period
    var ema float64
    for i, snap := range history {
        rate := snap.DeltaCost / snap.DeltaHours
        if i == 0 {
            ema = rate
        } else {
            ema = alpha*rate + (1-alpha)*ema
        }
    }

    spent := history[len(history)-1].CumulativeCost
    remaining := budget - spent
    hoursEst := remaining / ema  // if ema > 0

    // Anomaly: current interval rate vs EMA
    currentRate := history[len(history)-1].DeltaCost / history[len(history)-1].DeltaHours
    anomaly := currentRate / ema  // >2.0 triggers alert (ROADMAP 6.10.4)

    // Confidence: f(sample_count) using sigmoid
    confidence := 1.0 - (1.0 / (1.0 + float64(len(history))/10.0))
}
```

**Anomaly detection formula (ROADMAP 6.10.4):**
```
anomaly_score = current_burn_rate / ema_burn_rate
if anomaly_score > 2.0:
    emit BudgetAnomaly event
    // "Session X spending 3.2x its predicted rate ($4.50/hr vs $1.40/hr EMA)"
```

**TUI forecast widget (ROADMAP 6.10.3):**
```
Budget: $45.00 / $100.00 (45%)  |  Burn: $2.30/hr  |  ETA: ~23.9h remaining
[=============================-----------]
```

Wire into `handleMarathonDashboard` to replace the current instantaneous `burnRate` with the EMA-smoothed value.

### R5: Multi-Tier Budget Alerts with TUI Notifications

**Target:** `internal/session/budget.go` + `internal/events/bus.go` + `internal/tui/views/fleet.go`
**Effort:** Small (1-2 days)
**Impact:** Medium -- enables proactive budget awareness
**ROADMAP:** 2.3.3, 5.5.5

Add threshold-based alerts at configurable percentages:

```go
type BudgetAlert struct {
    Threshold float64  // 0.50, 0.75, 0.90
    Fired     bool     // prevent duplicate alerts
}

func (b *BudgetEnforcer) CheckAlerts(s *Session) []BudgetAlert {
    thresholds := []float64{0.50, 0.75, 0.90}
    pct := s.SpentUSD / s.BudgetUSD
    var alerts []BudgetAlert
    for _, t := range thresholds {
        if pct >= t && !b.alertFired(s.ID, t) {
            alerts = append(alerts, BudgetAlert{Threshold: t})
            b.markFired(s.ID, t)
        }
    }
    return alerts
}
```

Add new event types to `internal/events/bus.go`:

```go
BudgetWarning50  EventType = "budget.warning.50"
BudgetWarning75  EventType = "budget.warning.75"
BudgetWarning90  EventType = "budget.warning.90"
```

Emit as BubbleTea messages via the existing notification component in `internal/tui/components/notification.go`:
```
[!] Session abc123 at 75% budget ($7.50 / $10.00) -- 2.3h remaining at $1.08/hr
```

### R6: Token-Level Cost Extraction from Provider Streams

**Target:** `internal/session/providers.go` (normalizers) + `internal/session/types.go` (StreamEvent)
**Effort:** Medium (2-3 days)
**Impact:** High -- enables accurate per-turn cost calculation
**ROADMAP:** 2.3.1, 2.5.5.3

Extend `StreamEvent` with token-level fields:

```go
type StreamEvent struct {
    // ... existing fields ...
    InputTokens        int     `json:"input_tokens,omitempty"`
    OutputTokens       int     `json:"output_tokens,omitempty"`
    CacheWriteTokens   int     `json:"cache_creation_input_tokens,omitempty"`
    CacheReadTokens    int     `json:"cache_read_input_tokens,omitempty"`
}
```

Update `normalizeClaudeEvent` to extract from the `usage` object in Claude's stream-json `result` events:

```go
// Claude stream-json result events include:
// "usage": {"input_tokens": N, "output_tokens": N, "cache_creation_input_tokens": N, "cache_read_input_tokens": N}
if raw["usage"] != nil {
    usage := raw["usage"].(map[string]any)
    event.InputTokens = firstNonZeroInt(usage, "input_tokens")
    event.OutputTokens = firstNonZeroInt(usage, "output_tokens")
    event.CacheWriteTokens = firstNonZeroInt(usage, "cache_creation_input_tokens")
    event.CacheReadTokens = firstNonZeroInt(usage, "cache_read_input_tokens")
}
```

Similarly update `normalizeGeminiEvent` and `normalizeCodexEvent` for their respective token usage fields. Gemini uses `usage.input_tokens`/`usage.output_tokens`; Codex uses `usage.total_tokens` or `usage.input_tokens`/`usage.output_tokens`.

In `runner.go`, pass token counts to `NormalizeProviderCost()` instead of `0, 0`:

```go
if event.CostUSD > 0 {
    s.SpentUSD = event.CostUSD
    // Also record token-level data
    norm := NormalizeProviderCost(s.Provider, event.CostUSD,
        event.InputTokens, event.OutputTokens)
    // Store norm.NormalizedUSD for cross-provider comparison
}
```

### R7: Fleet Budget Profiles (Marathon / Sprint / Overnight)

**Target:** New file `internal/session/profiles.go`
**Effort:** Small (1 day)
**Impact:** Medium -- standardized budget presets for common fleet patterns
**ROADMAP:** 2.3.5

Port and extend the `rdcycle/profiles.go` pattern for fleet-scale use:

```go
type FleetBudgetProfile struct {
    Name              string
    GlobalCeilingUSD  float64
    PerSessionDefault float64
    HeadroomPct       float64
    DailyCap          float64
    MaxConcurrent     int
    DefaultModel      string
    ModelPricing      map[string]ModelCostRate
}

func MarathonProfile() FleetBudgetProfile {
    return FleetBudgetProfile{
        Name:              "marathon",
        GlobalCeilingUSD:  100.0,
        PerSessionDefault: 10.0,
        HeadroomPct:       0.90,
        DailyCap:          100.0,
        MaxConcurrent:     8,
        DefaultModel:      "claude-sonnet-4-6",
    }
}

func SprintProfile() FleetBudgetProfile {
    return FleetBudgetProfile{
        Name:              "sprint",
        GlobalCeilingUSD:  25.0,
        PerSessionDefault: 5.0,
        HeadroomPct:       0.85,
        DailyCap:          25.0,
        MaxConcurrent:     4,
        DefaultModel:      "claude-sonnet-4-6",
    }
}

func OvernightProfile() FleetBudgetProfile {
    return FleetBudgetProfile{
        Name:              "overnight",
        GlobalCeilingUSD:  50.0,
        PerSessionDefault: 5.0,
        HeadroomPct:       0.95,
        DailyCap:          50.0,
        MaxConcurrent:     6,
        DefaultModel:      "claude-haiku-4-5",  // cheaper for overnight
    }
}
```

Load from `.ralphrc` or `~/.ralphglasses/profiles/` JSON files, with `LoadProfile()`/`SaveProfile()` matching the rdcycle pattern.

---

## 6. Risk Assessment

### 6.1 Cost Data Accuracy (High Risk)

**Risk:** Claude Code's `stream-json` cost reporting may lag, be absent for certain event types, or use different rounding than billing.

**Mitigation:** Implement the dual-source pattern from ccusage: use stream-reported `costUSD` as primary, but also read `~/.claude/projects/**/usage.jsonl` for ground-truth reconciliation. Add a `reconcile` command or MCP tool that compares local ledger totals against JSONL-derived totals.

**Evidence:** ccusage's `auto` cost mode exists precisely because `costUSD` is not always present in older Claude Code versions. The `calculate` mode from token counts is the fallback.

### 6.2 Pricing Staleness (Medium Risk)

**Risk:** Hardcoded pricing tables become incorrect when providers change pricing.

**Mitigation:** (a) Add a `pricing_updated_at` timestamp to the rate table and log a warning if older than 30 days. (b) Add a `ralphglasses_pricing_update` MCP tool that fetches from LiteLLM or a pinned config URL. (c) Allow override via `.ralphrc` key `CUSTOM_PRICING_FILE=path/to/pricing.json`.

### 6.3 SQLite Contention (Low Risk)

**Risk:** Multiple concurrent sessions writing to the same SQLite database.

**Mitigation:** Use WAL mode (`PRAGMA journal_mode=WAL`) which allows concurrent reads during writes. Batch writes using a buffered channel pattern (write every N events or every T seconds, whichever comes first). The internal SQLite project project already demonstrates this pattern with `modernc.org/sqlite`.

### 6.4 Global Pool Deadlocks (Medium Risk)

**Risk:** Budget pool allocation/reclaimation during high-concurrency fleet operations could cause contention.

**Mitigation:** Use `sync.RWMutex` with short critical sections (read lock for checks, write lock only for allocation/spend recording). The mcpkit/finops `ScopedTracker` pattern already handles this correctly.

### 6.5 Provider Cost Asymmetry (Low Risk)

**Risk:** Gemini and Codex may not report costs in their stream output, making normalization inaccurate.

**Mitigation:** For providers that do not report `costUSD` in their stream, calculate from token counts using the `ModelCostRates` table. Add stderr parsing fallback (ROADMAP 2.5.5.3) as a secondary source. Flag sessions with "estimated" vs "reported" cost confidence levels.

---

## 7. Implementation Priority Ordering

| Priority | Rec | Target File | Effort | Impact | ROADMAP | Rationale |
|----------|-----|------------|--------|--------|---------|-----------|
| **P0** | R1 | `internal/session/costnorm.go` | Small | High | 2.5.5.3 | Foundation: accurate per-model pricing unblocks all other cost features |
| **P0** | R6 | `internal/session/providers.go`, `types.go` | Medium | High | 2.3.1, 2.5.5.3 | Foundation: token-level data required for cache-aware pricing |
| **P1** | R5 | `internal/session/budget.go`, `internal/events/bus.go` | Small | Medium | 2.3.3, 5.5.5 | Quick win: threshold alerts with minimal code change |
| **P1** | R7 | `internal/session/profiles.go` (new) | Small | Medium | 2.3.5 | Quick win: reusable fleet profiles, ports existing mcpkit pattern |
| **P2** | R2 | `internal/session/budgetpool.go` (new), `manager.go` | Medium | High | 2.3.2, 5.5.1, 5.5.2 | Core: global pool is prerequisite for fleet-scale cost management |
| **P2** | R3 | `internal/session/costdb.go` (new) | Medium | Medium | 5.5.1, 5.5.3, 6.10.1 | Persistence: enables historical queries, forecasting, restart survival |
| **P3** | R4 | `internal/session/forecast.go` (new) | Medium | Medium | 6.10.1-6.10.5 | Advanced: requires R3 data for meaningful forecasting |

### Dependency Graph

```
R1 (pricing table) ──┐
                     ├──> R6 (token extraction) ──> R3 (SQLite) ──> R4 (forecasting)
R5 (alerts) ─────────┘                               │
R7 (profiles) ───────────────────────────> R2 (pool) ─┘
```

### Phase Mapping

- **Phase 2.3 (Budget Tracking):** R1, R5, R6, R7 -- all P0/P1, can start immediately
- **Phase 5.5 (Budget Federation):** R2, R3 -- P2, blocked by 2.1 (session model)
- **Phase 6.10 (Cost Forecasting):** R4 -- P3, blocked by R3 (needs historical data)
- **Phase 7.4 (Cloud Cost Management):** Not addressed here; requires K8s operator (7.1). Cloud cost API integration (AWS Cost Explorer `ce:GetCostAndUsage`, GCP `cloudbilling.googleapis.com`) should be a separate research phase after 7.1 is complete.

---

## Appendix A: Pricing Formula Reference

### Full cost calculation (with all token types):

```
cost = (input_tokens / 1M * input_rate)
     + (output_tokens / 1M * output_rate)
     + (cache_write_tokens / 1M * cache_write_rate)
     + (cache_read_tokens / 1M * cache_read_rate)
```

### Tiered pricing (for Sonnet 4.5/4 with 1M context beta, >200K input):

```
if total_input_tokens > 200_000:
    input_rate  = base_input_rate * 2.0   # e.g., $3 -> $6 for Sonnet
    output_rate = base_output_rate * 1.5  # e.g., $15 -> $22.50 for Sonnet
```

### Fast mode (Opus 4.6 only):

```
fast_input_rate  = base_input_rate * 6.0   # $5 -> $30
fast_output_rate = base_output_rate * 6.0  # $25 -> $150
```

### Cross-provider normalization:

```
normalized_usd = (input_tokens / 1M * claude_sonnet_input_rate)
               + (output_tokens / 1M * claude_sonnet_output_rate)
efficiency_pct = (raw_cost / normalized_usd) * 100
# <100% = cheaper than Claude Sonnet, >100% = more expensive
```

### Burn rate (EMA-smoothed):

```
ema[0] = rate[0]
ema[i] = alpha * rate[i] + (1 - alpha) * ema[i-1]
# alpha = 0.3 gives ~10-sample half-life
# rate[i] = delta_cost[i] / delta_hours[i]
```

## Appendix B: Claude Code JSONL Usage File Locations

```
Primary:   ~/.config/claude/projects/**/usage.jsonl
Legacy:    ~/.claude/projects/**/usage.jsonl
Override:  $CLAUDE_CONFIG_DIR (comma-separated paths)
```

Per-line schema (from ccusage analysis):
```json
{
  "timestamp": "2026-03-22T10:30:00Z",
  "message": {
    "usage": {
      "input_tokens": 1500,
      "output_tokens": 800,
      "cache_creation_input_tokens": 0,
      "cache_read_input_tokens": 12000
    },
    "model": "claude-sonnet-4-6",
    "id": "msg_..."
  },
  "requestId": "req_...",
  "costUSD": 0.0165,
  "sessionId": "session_...",
  "version": "2.1.0"
}
```
