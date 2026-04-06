// Package promptdj implements a prompt-aware routing engine ("Prompt DJ") that
// bridges the prompt quality scoring system with the cascade routing system.
// It routes prompts to optimal providers based on quality, task type, domain,
// cost, and learned affinities.
package promptdj

import (
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// RoutingRequest is the input to the Prompt DJ router.
type RoutingRequest struct {
	Prompt    string            `json:"prompt"`
	Repo      string            `json:"repo,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	TaskType  enhancer.TaskType `json:"task_type,omitempty"`
	Score     int               `json:"score,omitempty"` // pre-computed quality score 0-100
	SessionID string            `json:"session_id,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// RoutingDecision is the output of the Prompt DJ router.
type RoutingDecision struct {
	// Primary route
	DecisionID  string           `json:"decision_id"`
	Provider    session.Provider `json:"provider"`
	Model       string           `json:"model"`
	AgentProfile string          `json:"agent_profile,omitempty"`
	ModelTier   session.ModelTier `json:"model_tier"`

	// Quality gate outcome
	EnhancedPrompt string `json:"enhanced_prompt,omitempty"`
	WasEnhanced    bool   `json:"was_enhanced"`
	OriginalScore  int    `json:"original_score"`
	EnhancedScore  int    `json:"enhanced_score,omitempty"`

	// Classification
	TaskType   enhancer.TaskType `json:"task_type"`
	Complexity int               `json:"complexity"` // 1-4
	DomainTags []string          `json:"domain_tags,omitempty"`

	// Confidence
	Confidence      float64 `json:"confidence"`       // 0.0-1.0
	ConfidenceLevel string  `json:"confidence_level"` // "high", "medium", "low"
	Rationale       string  `json:"rationale"`

	// Cost estimate
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	CostTier         string  `json:"cost_tier"`

	// Multi-model cascade tier (when CascadeTiersEnabled)
	CascadeTierResult *CascadeTierResult `json:"cascade_tier_result,omitempty"`

	// Fallback
	FallbackChain []FallbackRoute `json:"fallback_chain,omitempty"`

	// Telemetry
	Timestamp time.Time `json:"timestamp"`
	LatencyMs int64     `json:"latency_ms"`
}

// FallbackRoute is an alternative provider if the primary fails.
type FallbackRoute struct {
	Provider   session.Provider `json:"provider"`
	Model      string           `json:"model"`
	Reason     string           `json:"reason"`
	Confidence float64          `json:"confidence"`
}

// QualityTier categorizes prompt quality for routing decisions.
type QualityTier string

const (
	QualityHigh   QualityTier = "high"   // score >= 80
	QualityMedium QualityTier = "medium" // score 50-79
	QualityLow    QualityTier = "low"    // score < 50
)

// QualityTierFromScore returns the quality tier for a given score.
func QualityTierFromScore(score int) QualityTier {
	switch {
	case score >= 80:
		return QualityHigh
	case score >= 50:
		return QualityMedium
	default:
		return QualityLow
	}
}

// ConfidenceLevelFromScore returns "high", "medium", or "low".
func ConfidenceLevelFromScore(c float64) string {
	switch {
	case c >= 0.8:
		return "high"
	case c >= 0.5:
		return "medium"
	default:
		return "low"
	}
}
