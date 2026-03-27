package session

import (
	"log/slog"
	"sync"
	"time"
)

// CascadeConfig configures the cheap-then-expensive routing strategy.
type CascadeConfig struct {
	CheapProvider       Provider            `json:"cheap_provider"`
	ExpensiveProvider   Provider            `json:"expensive_provider"`
	ConfidenceThreshold float64             `json:"confidence_threshold"` // 0.0-1.0, default 0.7
	MaxCheapBudgetUSD   float64             `json:"max_cheap_budget_usd"`
	MaxCheapTurns        int                 `json:"max_cheap_turns"`
	TaskTypeOverrides    map[string]Provider `json:"task_type_overrides"`
	SpeculativeExecution bool                `json:"speculative_execution"`
	LatencyThresholdMs   int                 `json:"latency_threshold_ms"` // 0 = disabled; if cheap P95 exceeds this, skip to expensive
}

// DefaultCascadeConfig returns sensible defaults.
func DefaultCascadeConfig() CascadeConfig {
	return CascadeConfig{
		CheapProvider:       ProviderGemini,
		ExpensiveProvider:   ProviderClaude,
		ConfidenceThreshold: 0.7,
		MaxCheapBudgetUSD:   2.00,
		MaxCheapTurns:       15,
		TaskTypeOverrides:   make(map[string]Provider),
	}
}

// CascadeResult records the outcome of a cascade routing decision.
type CascadeResult struct {
	Timestamp       time.Time `json:"ts"`
	TaskType        string    `json:"task_type"`
	TaskTitle       string    `json:"task_title"`
	UsedProvider    Provider  `json:"used_provider"`
	Escalated       bool      `json:"escalated"`
	CheapConfidence float64   `json:"cheap_confidence"`
	CheapCostUSD    float64   `json:"cheap_cost_usd"`
	TotalCostUSD    float64   `json:"total_cost_usd"`
	Reason          string    `json:"escalation_reason"` // "low_confidence", "verify_failed", "error", ""
}

// CascadeStats summarizes cascade routing outcomes.
type CascadeStats struct {
	TotalDecisions int     `json:"total_decisions"`
	Escalations    int     `json:"escalations"`
	EscalationRate float64 `json:"escalation_rate"`
	CostSavedUSD   float64 `json:"cost_saved_usd"` // sum of cheap costs for non-escalated
	AvgCheapCost   float64 `json:"avg_cheap_cost"`
}

// CascadeRouter implements try-cheap-then-escalate provider routing.
type CascadeRouter struct {
	mu        sync.Mutex
	config    CascadeConfig
	feedback  *FeedbackAnalyzer
	decisions *DecisionLog
	results   []CascadeResult
	stateDir  string
	tiers     []ModelTier
	latencies map[string][]time.Duration // provider -> sliding window of recent latencies

	// banditSelect is an optional function that selects a provider using bandit policy.
	// Set via SetBanditHooks. When configured and enough history exists, SelectTier
	// will consult the bandit before falling through to static tier selection.
	banditSelect func() (provider string, model string)
	// banditUpdate is an optional function that records a reward for the bandit policy.
	banditUpdate func(provider string, reward float64)

	// decisionModel is an optional calibrated confidence model.
	// When set and trained, EvaluateCheapResult uses it instead of computeConfidence.
	decisionModel interface {
		IsTrained() bool
		PredictConfidence(turnCount, expectedTurns int, lastOutput string, verifyPassed bool) float64
		Stats() map[string]any
	}
}

// NewCascadeRouter creates a cascade router, loading any persisted results.
func NewCascadeRouter(config CascadeConfig, feedback *FeedbackAnalyzer, decisions *DecisionLog, stateDir string) *CascadeRouter {
	cr := &CascadeRouter{
		config:    config,
		feedback:  feedback,
		decisions: decisions,
		stateDir:  stateDir,
		tiers:     DefaultModelTiers(),
		latencies: make(map[string][]time.Duration),
	}
	cr.loadResults()
	return cr
}

// ShouldCascade returns true if the task should attempt cheap-first routing.
// Returns false if the task type has an override, if the cheap provider
// is already proven reliable for this task type, or if the cheap provider's
// latency exceeds the configured threshold.
func (cr *CascadeRouter) ShouldCascade(taskType string, prompt string) bool {
	// If task type has an override, skip cascading — use the override directly
	if _, ok := cr.config.TaskTypeOverrides[taskType]; ok {
		return false
	}

	// If cheap provider is too slow, skip cascading and go to expensive directly.
	cr.mu.Lock()
	slow := cr.cheapProviderSlow()
	cr.mu.Unlock()
	if slow {
		slog.Info("cascade: skipping cheap provider due to high latency",
			"cheap_provider", cr.config.CheapProvider,
			"threshold_ms", cr.config.LatencyThresholdMs,
		)
		return false
	}

	// No feedback data available — default to cascading
	if cr.feedback == nil {
		return true
	}

	// Check if the cheap provider is already reliable for this task type
	profile, ok := cr.feedback.GetProviderProfile(string(cr.config.CheapProvider), taskType)
	if ok && profile.CompletionRate > 90 && profile.SampleCount >= 5 {
		return false
	}

	return true
}

// ResolveProvider returns the provider to use for a given task type.
// If there is an override, returns the override. If cheap is reliable and
// not experiencing high latency, returns cheap. Otherwise returns expensive
// (caller should use cascade logic).
func (cr *CascadeRouter) ResolveProvider(taskType string) Provider {
	// Check for task type override
	if override, ok := cr.config.TaskTypeOverrides[taskType]; ok {
		return override
	}

	// If cheap provider has high latency, go straight to expensive.
	cr.mu.Lock()
	slow := cr.cheapProviderSlow()
	cr.mu.Unlock()
	if slow {
		return cr.config.ExpensiveProvider
	}

	// If cascading is not needed (cheap is reliable), use cheap directly
	if !cr.ShouldCascade(taskType, "") {
		return cr.config.CheapProvider
	}

	// Default to expensive — caller will use cascade logic to try cheap first
	return cr.config.ExpensiveProvider
}

// SetTiers replaces the default model tiers with a custom list.
func (cr *CascadeRouter) SetTiers(tiers []ModelTier) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.tiers = tiers
}

// Tiers returns the current model tier list.
func (cr *CascadeRouter) Tiers() []ModelTier {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	out := make([]ModelTier, len(cr.tiers))
	copy(out, cr.tiers)
	return out
}

// SetBanditHooks attaches bandit-based provider selection functions.
// selectFn returns (provider, model) from the bandit policy.
// updateFn records a reward (0.0-1.0) for a provider after a cascade decision.
func (cr *CascadeRouter) SetBanditHooks(selectFn func() (string, string), updateFn func(string, float64)) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.banditSelect = selectFn
	cr.banditUpdate = updateFn
}

// SetDecisionModel attaches a calibrated confidence model for escalation decisions.
// The model must implement IsTrained(), PredictConfidence(), and Stats().
func (cr *CascadeRouter) SetDecisionModel(dm interface {
	IsTrained() bool
	PredictConfidence(turnCount, expectedTurns int, lastOutput string, verifyPassed bool) float64
	Stats() map[string]any
}) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.decisionModel = dm
}

// DecisionModelStats returns the decision model stats, or nil if no model is set.
func (cr *CascadeRouter) DecisionModelStats() map[string]any {
	cr.mu.Lock()
	dm := cr.decisionModel
	cr.mu.Unlock()
	if dm == nil {
		return nil
	}
	return dm.Stats()
}

// BanditConfigured returns true if bandit hooks have been set.
func (cr *CascadeRouter) BanditConfigured() bool {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	return cr.banditSelect != nil
}

// CheapLaunchOpts returns launch options modified for the cheap provider.
func (cr *CascadeRouter) CheapLaunchOpts(base LaunchOptions) LaunchOptions {
	opts := base
	opts.Provider = cr.config.CheapProvider

	if cr.config.MaxCheapBudgetUSD > 0 {
		opts.MaxBudgetUSD = cr.config.MaxCheapBudgetUSD
	}
	if cr.config.MaxCheapTurns > 0 {
		opts.MaxTurns = cr.config.MaxCheapTurns
	}

	opts.SessionName = opts.SessionName + "-cheap"

	return opts
}

// SpeculativeLaunchOpts returns two sets of launch options for parallel
// speculative execution: one cheap and one expensive. The cheap opts use
// CheapLaunchOpts; the expensive opts keep the base provider but append
// "-speculative" to the session name.
func (cr *CascadeRouter) SpeculativeLaunchOpts(base LaunchOptions) (cheap LaunchOptions, expensive LaunchOptions) {
	cheap = cr.CheapLaunchOpts(base)

	expensive = base
	expensive.SessionName = base.SessionName + "-speculative"

	return cheap, expensive
}

// EvaluateCheapResult examines a completed cheap session and decides whether
// to escalate to the expensive provider.
func (cr *CascadeRouter) EvaluateCheapResult(s *Session, expectedTurns int, verification []LoopVerification) (escalate bool, confidence float64, reason string) {
	s.Lock()
	errMsg := s.Error
	turnCount := s.TurnCount
	lastOutput := s.LastOutput
	s.Unlock()

	// Session errored out
	if errMsg != "" {
		return true, 0, "error"
	}

	// Check verification results
	if len(verification) > 0 {
		allPassed := true
		for _, v := range verification {
			if v.ExitCode != 0 {
				allPassed = false
				break
			}
		}
		if !allPassed {
			return true, computeConfidence(turnCount, expectedTurns, lastOutput, false), "verify_failed"
		}
	}

	// Compute confidence score — use calibrated decision model if available,
	// otherwise fall back to the heuristic computeConfidence function.
	cr.mu.Lock()
	dm := cr.decisionModel
	cr.mu.Unlock()

	var conf float64
	if dm != nil && dm.IsTrained() {
		conf = dm.PredictConfidence(turnCount, expectedTurns, lastOutput, true)
	} else {
		conf = computeConfidence(turnCount, expectedTurns, lastOutput, true)
	}
	if conf < cr.config.ConfidenceThreshold {
		return true, conf, "low_confidence"
	}

	return false, conf, ""
}
