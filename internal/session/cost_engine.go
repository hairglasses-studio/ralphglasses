package session

import (
	"math"
	"sync"
	"time"
)

// CostEngine provides real-time cost optimization for sessions. It monitors
// per-session spend rates, triggers provider switching when cost thresholds
// are hit, and implements token-aware batching to reduce per-request overhead.
type CostEngine struct {
	mu sync.RWMutex

	// Configuration
	thresholds []SpendThreshold
	batchCfg   BatchConfig

	// Per-session tracking
	sessions map[string]*sessionCostState

	// Clock function for testability.
	now func() time.Time
}

// SpendThreshold defines a cost rate at which a provider switch is recommended.
type SpendThreshold struct {
	// MaxRatePerHour is the spend rate (USD/hr) above which a switch triggers.
	MaxRatePerHour float64 `json:"max_rate_per_hour"`

	// MaxTotalSpend is the cumulative spend (USD) above which a switch triggers.
	MaxTotalSpend float64 `json:"max_total_spend"`

	// FromProvider is the provider to switch away from. Empty means any.
	FromProvider Provider `json:"from_provider,omitempty"`

	// ToProvider is the recommended cheaper provider.
	ToProvider Provider `json:"to_provider"`

	// Label is a human-readable description (e.g. "claude→gemini at $2/hr").
	Label string `json:"label,omitempty"`
}

// BatchConfig controls token-aware batching behaviour.
type BatchConfig struct {
	// MaxBatchSize is the maximum number of prompts to batch together.
	MaxBatchSize int `json:"max_batch_size"`

	// MaxWait is how long to wait for a full batch before flushing.
	MaxWait time.Duration `json:"max_wait"`

	// MinTokens is the minimum combined token count to justify a batch send.
	MinTokens int `json:"min_tokens"`
}

// SwitchRecommendation is returned when a cost threshold has been breached.
type SwitchRecommendation struct {
	SessionID    string        `json:"session_id"`
	FromProvider Provider      `json:"from_provider"`
	ToProvider   Provider      `json:"to_provider"`
	Reason       string        `json:"reason"`
	CurrentRate  float64       `json:"current_rate_usd_hr"`
	TotalSpent   float64       `json:"total_spent_usd"`
	Threshold    SpendThreshold `json:"threshold"`
}

// BatchEntry is a single prompt queued for batching.
type BatchEntry struct {
	SessionID  string `json:"session_id"`
	Prompt     string `json:"prompt"`
	TokenCount int    `json:"token_count"`
	QueuedAt   time.Time `json:"queued_at"`
}

// BatchResult describes the savings from batching prompts together.
type BatchResult struct {
	Entries       []BatchEntry  `json:"entries"`
	TotalTokens   int           `json:"total_tokens"`
	IndividualCost float64      `json:"individual_cost_usd"`
	BatchedCost    float64      `json:"batched_cost_usd"`
	Savings        float64      `json:"savings_usd"`
	SavingsPct     float64      `json:"savings_pct"`
}

// sessionCostState tracks cost data for a single session.
type sessionCostState struct {
	provider    Provider
	startedAt   time.Time
	entries     []costEntry
	totalSpent  float64
	batchQueue  []BatchEntry
}

// costEntry is an internal cost observation with timestamp.
type costEntry struct {
	amount    float64
	timestamp time.Time
}

// NewCostEngine creates a CostEngine with the given thresholds and batch config.
func NewCostEngine(thresholds []SpendThreshold, batchCfg BatchConfig) *CostEngine {
	return &CostEngine{
		thresholds: thresholds,
		batchCfg:   batchCfg,
		sessions:   make(map[string]*sessionCostState),
		now:        time.Now,
	}
}

// DefaultCostEngine returns a CostEngine with sensible default thresholds:
// Claude→Gemini at $5/hr or $2 total, Codex→Gemini at $3/hr or $1.50 total.
func DefaultCostEngine() *CostEngine {
	return NewCostEngine(
		[]SpendThreshold{
			{
				MaxRatePerHour: 5.0,
				MaxTotalSpend:  2.0,
				FromProvider:   ProviderClaude,
				ToProvider:     ProviderGemini,
				Label:          "claude→gemini at $5/hr",
			},
			{
				MaxRatePerHour: 3.0,
				MaxTotalSpend:  1.5,
				FromProvider:   ProviderCodex,
				ToProvider:     ProviderGemini,
				Label:          "codex→gemini at $3/hr",
			},
		},
		BatchConfig{
			MaxBatchSize: 5,
			MaxWait:      10 * time.Second,
			MinTokens:    500,
		},
	)
}

// TrackSession registers a session for cost monitoring. Must be called before
// RecordCost for that session.
func (ce *CostEngine) TrackSession(sessionID string, provider Provider) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	ce.sessions[sessionID] = &sessionCostState{
		provider:  provider,
		startedAt: ce.now(),
	}
}

// RecordCost records a cost event for a session and returns any switch
// recommendation triggered. Returns nil if no threshold was breached.
func (ce *CostEngine) RecordCost(sessionID string, amount float64) *SwitchRecommendation {
	if amount <= 0 {
		return nil
	}

	ce.mu.Lock()
	defer ce.mu.Unlock()

	state, ok := ce.sessions[sessionID]
	if !ok {
		return nil
	}

	now := ce.now()
	state.entries = append(state.entries, costEntry{
		amount:    amount,
		timestamp: now,
	})
	state.totalSpent += amount

	rate := ce.computeRateLocked(state, now)

	return ce.checkThresholdsLocked(sessionID, state, rate)
}

// SpendRate returns the current spend rate in USD/hr for a session.
// Returns 0 if the session is not tracked or has insufficient data.
func (ce *CostEngine) SpendRate(sessionID string) float64 {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	state, ok := ce.sessions[sessionID]
	if !ok {
		return 0
	}
	return ce.computeRateLocked(state, ce.now())
}

// TotalSpent returns the cumulative cost for a session.
func (ce *CostEngine) TotalSpent(sessionID string) float64 {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	state, ok := ce.sessions[sessionID]
	if !ok {
		return 0
	}
	return state.totalSpent
}

// CheckThresholds evaluates all thresholds for a session without recording
// new cost data. Useful for periodic polling.
func (ce *CostEngine) CheckThresholds(sessionID string) *SwitchRecommendation {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	state, ok := ce.sessions[sessionID]
	if !ok {
		return nil
	}

	rate := ce.computeRateLocked(state, ce.now())
	return ce.checkThresholdsLocked(sessionID, state, rate)
}

// QueueBatch adds a prompt to the batch queue for a session. Returns a
// BatchResult if the batch is ready to flush, nil otherwise.
func (ce *CostEngine) QueueBatch(sessionID string, prompt string, tokenCount int) *BatchResult {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	state, ok := ce.sessions[sessionID]
	if !ok {
		return nil
	}

	state.batchQueue = append(state.batchQueue, BatchEntry{
		SessionID:  sessionID,
		Prompt:     prompt,
		TokenCount: tokenCount,
		QueuedAt:   ce.now(),
	})

	if ce.shouldFlushLocked(state) {
		return ce.flushBatchLocked(state)
	}
	return nil
}

// FlushBatch forces a batch flush for a session regardless of whether the
// batch is full. Returns nil if the queue is empty.
func (ce *CostEngine) FlushBatch(sessionID string) *BatchResult {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	state, ok := ce.sessions[sessionID]
	if !ok || len(state.batchQueue) == 0 {
		return nil
	}

	return ce.flushBatchLocked(state)
}

// RemoveSession stops tracking a session and clears its state.
func (ce *CostEngine) RemoveSession(sessionID string) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	delete(ce.sessions, sessionID)
}

// SessionCount returns the number of tracked sessions.
func (ce *CostEngine) SessionCount() int {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	return len(ce.sessions)
}

// computeRateLocked calculates the spend rate in USD/hr using a 5-minute
// sliding window. Caller must hold at least a read lock.
func (ce *CostEngine) computeRateLocked(state *sessionCostState, now time.Time) float64 {
	if len(state.entries) < 2 {
		return 0
	}

	window := 5 * time.Minute
	cutoff := now.Add(-window)

	var windowSpend float64
	var earliest time.Time
	found := false

	for _, e := range state.entries {
		if e.timestamp.Before(cutoff) {
			continue
		}
		windowSpend += e.amount
		if !found || e.timestamp.Before(earliest) {
			earliest = e.timestamp
			found = true
		}
	}

	if !found {
		return 0
	}

	elapsed := now.Sub(earliest)
	if elapsed <= 0 {
		return 0
	}

	hours := elapsed.Hours()
	if hours <= 0 {
		return 0
	}

	return windowSpend / hours
}

// checkThresholdsLocked evaluates thresholds against a session's state.
// Caller must hold at least a read lock.
func (ce *CostEngine) checkThresholdsLocked(sessionID string, state *sessionCostState, rate float64) *SwitchRecommendation {
	for _, t := range ce.thresholds {
		if t.FromProvider != "" && t.FromProvider != state.provider {
			continue
		}

		rateBreached := t.MaxRatePerHour > 0 && rate > t.MaxRatePerHour
		totalBreached := t.MaxTotalSpend > 0 && state.totalSpent > t.MaxTotalSpend

		if !rateBreached && !totalBreached {
			continue
		}

		reason := "cost threshold breached"
		if rateBreached && totalBreached {
			reason = "rate and total spend thresholds breached"
		} else if rateBreached {
			reason = "spend rate threshold breached"
		} else {
			reason = "total spend threshold breached"
		}

		return &SwitchRecommendation{
			SessionID:    sessionID,
			FromProvider: state.provider,
			ToProvider:   t.ToProvider,
			Reason:       reason,
			CurrentRate:  rate,
			TotalSpent:   state.totalSpent,
			Threshold:    t,
		}
	}
	return nil
}

// shouldFlushLocked checks if a batch should be flushed.
func (ce *CostEngine) shouldFlushLocked(state *sessionCostState) bool {
	if len(state.batchQueue) >= ce.batchCfg.MaxBatchSize && ce.batchCfg.MaxBatchSize > 0 {
		return true
	}

	var totalTokens int
	for _, e := range state.batchQueue {
		totalTokens += e.TokenCount
	}
	if ce.batchCfg.MinTokens > 0 && totalTokens >= ce.batchCfg.MinTokens {
		return true
	}

	if len(state.batchQueue) > 0 && ce.batchCfg.MaxWait > 0 {
		oldest := state.batchQueue[0].QueuedAt
		if ce.now().Sub(oldest) >= ce.batchCfg.MaxWait {
			return true
		}
	}

	return false
}

// flushBatchLocked drains the batch queue and computes savings. The savings
// model assumes a fixed per-request overhead of 50 tokens (context preamble)
// that is amortised across the batch.
func (ce *CostEngine) flushBatchLocked(state *sessionCostState) *BatchResult {
	if len(state.batchQueue) == 0 {
		return nil
	}

	entries := make([]BatchEntry, len(state.batchQueue))
	copy(entries, state.batchQueue)
	state.batchQueue = state.batchQueue[:0]

	var totalTokens int
	for _, e := range entries {
		totalTokens += e.TokenCount
	}

	result := computeBatchSavings(entries, totalTokens, state.provider)
	return result
}

// overheadTokensPerRequest is the assumed per-request overhead (context
// preamble, system prompt prefix, etc.) that batching amortises.
const overheadTokensPerRequest = 50

// computeBatchSavings calculates the cost difference between sending prompts
// individually vs. as a batch. The model uses the provider's cheapest model
// input cost as the per-token rate.
func computeBatchSavings(entries []BatchEntry, totalTokens int, provider Provider) *BatchResult {
	model := CheapestModel(provider)
	var ratePerToken float64
	if model != nil {
		ratePerToken = model.CostPerMTokIn / 1_000_000 // USD per token
	}

	n := len(entries)

	// Individual: each request pays its own tokens + overhead.
	individualTokens := totalTokens + n*overheadTokensPerRequest
	individualCost := float64(individualTokens) * ratePerToken

	// Batched: all tokens + single overhead.
	batchedTokens := totalTokens + overheadTokensPerRequest
	batchedCost := float64(batchedTokens) * ratePerToken

	savings := individualCost - batchedCost
	var savingsPct float64
	if individualCost > 0 {
		savingsPct = (savings / individualCost) * 100
	}

	return &BatchResult{
		Entries:        entries,
		TotalTokens:    totalTokens,
		IndividualCost: roundCost(individualCost),
		BatchedCost:    roundCost(batchedCost),
		Savings:        roundCost(savings),
		SavingsPct:     math.Round(savingsPct*100) / 100,
	}
}

// roundCost rounds a cost to 8 decimal places to avoid floating-point noise.
func roundCost(v float64) float64 {
	return math.Round(v*1e8) / 1e8
}
