package fleet

import (
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Optimizer analyzes fleet-wide patterns and updates scheduling weights.
type Optimizer struct {
	mu       sync.Mutex
	feedback *session.FeedbackAnalyzer

	// Scheduling weight adjustments learned from fleet history
	providerWeights map[session.Provider]float64 // multiplier for provider scoring
	repoWeights     map[string]float64           // multiplier for repo locality scoring
	lastOptimized   time.Time

	// banditStats returns per-arm mean rewards from the bandit policy.
	// Keys are provider names (arm IDs), values are mean reward [0,1].
	banditStats func() map[string]float64
}

// NewOptimizer creates a fleet optimizer with a feedback analyzer.
func NewOptimizer(feedback *session.FeedbackAnalyzer) *Optimizer {
	return &Optimizer{
		feedback:        feedback,
		providerWeights: map[session.Provider]float64{
			session.ProviderClaude: 1.0,
			session.ProviderGemini: 1.0,
			session.ProviderCodex:  1.0,
		},
		repoWeights: make(map[string]float64),
	}
}

// ProviderWeight returns the current scheduling weight for a provider.
func (o *Optimizer) ProviderWeight(p session.Provider) float64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	if w, ok := o.providerWeights[p]; ok {
		return w
	}
	return 1.0
}

// RepoWeight returns the current scheduling weight for a repo.
func (o *Optimizer) RepoWeight(repo string) float64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	if w, ok := o.repoWeights[repo]; ok {
		return w
	}
	return 1.0
}

// UpdateWeights recalculates scheduling weights from feedback profiles.
// Should be called periodically (e.g., every hour or after batch journal ingestion).
func (o *Optimizer) UpdateWeights() {
	if o.feedback == nil {
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	// Adjust provider weights based on completion rates and cost efficiency
	for _, profile := range o.feedback.AllProviderProfiles() {
		provider := session.Provider(profile.Provider)
		if profile.SampleCount < 3 {
			continue
		}

		// Weight = completion_rate * (1 / CostNorm-adjusted cost)
		// Higher completion rate and lower normalized cost = higher weight
		weight := 1.0
		if profile.CompletionRate > 0 {
			weight = profile.CompletionRate / 100.0
		}
		if profile.CostPerTurn > 0 {
			// Use CostNorm to normalize to Claude baseline before scoring
			norm := session.NormalizeProviderCost(provider, profile.CostPerTurn, 0, 0)
			costPerTurn := norm.NormalizedUSD
			if costPerTurn <= 0 {
				costPerTurn = profile.CostPerTurn
			}
			costFactor := 0.01 / costPerTurn
			if costFactor > 3.0 {
				costFactor = 3.0
			}
			if costFactor < 0.3 {
				costFactor = 0.3
			}
			weight *= costFactor
		}

		// Blend with existing weight (exponential moving average)
		if existing, ok := o.providerWeights[provider]; ok {
			o.providerWeights[provider] = existing*0.7 + weight*0.3
		} else {
			o.providerWeights[provider] = weight
		}
	}

	// If bandit stats are available, blend with bandit-derived weights.
	// Bandit mean rewards are in [0,1]; scale by 2 to match the ~1.0 center
	// of feedback-derived weights, then average 50/50.
	if o.banditStats != nil {
		stats := o.banditStats()
		for armID, meanReward := range stats {
			provider := session.Provider(armID)
			if existing, ok := o.providerWeights[provider]; ok {
				o.providerWeights[provider] = (existing + meanReward*2) / 2
			}
		}
	}

	o.lastOptimized = time.Now()
}

// SetBanditStats attaches a function that returns bandit arm statistics.
// The function should return a map of arm ID (provider name) to mean reward.
func (o *Optimizer) SetBanditStats(fn func() map[string]float64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.banditStats = fn
}

// IngestCrossWorkerJournals aggregates journal entries from multiple workers
// and feeds them to the feedback analyzer.
func (o *Optimizer) IngestCrossWorkerJournals(entries []session.JournalEntry) {
	if o.feedback == nil || len(entries) == 0 {
		return
	}
	o.feedback.Ingest(entries)
	o.UpdateWeights()
}

// Summary returns the current optimizer state.
func (o *Optimizer) Summary() map[string]any {
	o.mu.Lock()
	defer o.mu.Unlock()

	pw := make(map[string]float64, len(o.providerWeights))
	for k, v := range o.providerWeights {
		pw[string(k)] = v
	}

	return map[string]any{
		"provider_weights": pw,
		"repo_weights":     o.repoWeights,
		"last_optimized":   o.lastOptimized,
	}
}
