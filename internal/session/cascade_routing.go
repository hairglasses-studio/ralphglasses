package session

import (
	"math"
	"sort"
	"strings"
	"time"
)

// ProviderLatency holds latency percentiles for a provider.
type ProviderLatency struct {
	P50         time.Duration `json:"p50"`
	P95         time.Duration `json:"p95"`
	Samples     int           `json:"samples"`
	LastUpdated time.Time     `json:"last_updated"`
}

// latencyWindowSize is the maximum number of samples kept per provider.
const latencyWindowSize = 100

// ModelTier represents a model with its cost and capability profile.
type ModelTier struct {
	Provider      Provider `json:"provider"`
	Model         string   `json:"model"`
	MaxComplexity int      `json:"max_complexity"` // 1-4 scale
	CostPer1M     float64  `json:"cost_per_1m"`    // input cost per 1M tokens
	Label         string   `json:"label"`          // e.g. "ultra-cheap", "worker", "coding", "reasoning"
}

// DefaultModelTiers returns the built-in tier list ordered by cost.
func DefaultModelTiers() []ModelTier {
	return []ModelTier{
		{Provider: ProviderGemini, Model: "gemini-2.0-flash-lite", MaxComplexity: 1, CostPer1M: CostGeminiFlashLiteInput, Label: "ultra-cheap"},
		{Provider: ProviderGemini, Model: "gemini-2.5-flash", MaxComplexity: 2, CostPer1M: CostGeminiFlashInput, Label: "worker"},
		{Provider: ProviderCodex, Model: "gpt-5.4", MaxComplexity: 3, CostPer1M: CostCodexInput, Label: "coding"},
		{Provider: ProviderClaude, Model: "claude-opus", MaxComplexity: 4, CostPer1M: CostClaudeOpusInput, Label: "reasoning"},
	}
}

// taskTypeComplexity maps well-known task types to their complexity level (1-4).
// Includes both MCP tool task types and classifyTask() output categories.
var taskTypeComplexity = map[string]int{
	"lint":         1,
	"format":       1,
	"classify":     1,
	"docs":         1,
	"config":       2,
	"review":       2,
	"optimization": 2,
	"bug_fix":      2,
	"codegen":      3,
	"test":         3,
	"feature":      3,
	"refactor":     3,
	"general":      2,
	"architecture": 4,
	"analysis":     4,
	"planning":     4,
}

// TaskTypeComplexity returns the complexity for a known task type, or 0 if unknown.
func TaskTypeComplexity(taskType string) int {
	return taskTypeComplexity[taskType]
}

// SelectTier picks the cheapest model tier that can handle the given complexity.
// If taskType is recognized, its mapped complexity is used (the complexity arg
// is ignored). If taskType is unrecognized and complexity <= 0, the highest tier
// is returned. Returns an empty ModelTier if no tiers are configured.
//
// When a bandit policy is configured and sufficient history exists (>= 10 results),
// the bandit is consulted first. If the bandit-selected provider matches a known
// tier, that tier is returned; otherwise static selection is used as fallback.
func (cr *CascadeRouter) SelectTier(taskType string, complexity int) ModelTier {
	cr.mu.Lock()
	tiers := cr.tiers
	selectFn := cr.banditSelect
	historyLen := len(cr.results)
	cr.mu.Unlock()

	if len(tiers) == 0 {
		return ModelTier{}
	}

	// Consult bandit policy if configured and we have enough history.
	if selectFn != nil && historyLen >= 10 {
		provider, model := selectFn()
		if provider != "" {
			for _, t := range tiers {
				if string(t.Provider) == provider && (model == "" || t.Model == model) {
					return t
				}
			}
			// Bandit returned unknown provider/model — fall through to static selection.
		}
	}

	// Use task-type mapping if available; otherwise use the provided complexity.
	if mapped, ok := taskTypeComplexity[taskType]; ok {
		complexity = mapped
	}

	// If complexity is still unknown, default to highest tier.
	if complexity <= 0 {
		return tiers[len(tiers)-1]
	}

	// Find cheapest tier that can handle the complexity.
	// Tiers from DefaultModelTiers are sorted by cost ascending, but
	// callers can set custom tiers, so we scan all and pick the cheapest match.
	var best *ModelTier
	for i := range tiers {
		if tiers[i].MaxComplexity >= complexity {
			if best == nil || tiers[i].CostPer1M < best.CostPer1M {
				best = &tiers[i]
			}
		}
	}

	if best != nil {
		return *best
	}

	// No tier can handle the complexity — return the most capable tier.
	highest := tiers[0]
	for _, t := range tiers[1:] {
		if t.MaxComplexity > highest.MaxComplexity {
			highest = t
		}
	}
	return highest
}

// computeConfidence produces a 0.0-1.0 confidence score from session signals.
func computeConfidence(turnCount, expectedTurns int, lastOutput string, verifyPassed bool) float64 {
	score := 0.0
	components := 0

	// Turn efficiency: did we finish within expected turns?
	if expectedTurns > 0 && turnCount > 0 {
		components++
		ratio := float64(turnCount) / float64(expectedTurns)
		if ratio <= 1.0 {
			score += 1.0 // finished within budget
		} else if ratio <= 2.0 {
			score += 0.5
		}
		// > 2x expected turns → 0 contribution
	}

	// Verification passed
	components++
	if verifyPassed {
		score += 1.0
	}

	// Hedging language in output (indicates uncertainty)
	components++
	hedgeScore := 1.0
	if lastOutput != "" {
		lower := strings.ToLower(lastOutput)
		hedgeWords := []string{"i'm not sure", "might not", "possibly", "uncertain", "i think", "maybe", "not confident"}
		hedgeCount := 0
		for _, hw := range hedgeWords {
			if strings.Contains(lower, hw) {
				hedgeCount++
			}
		}
		if hedgeCount >= 3 {
			hedgeScore = 0.0
		} else if hedgeCount >= 1 {
			hedgeScore = 0.5
		}
	}
	score += hedgeScore

	// Error-free output
	components++
	if lastOutput != "" {
		lower := strings.ToLower(lastOutput)
		if strings.Contains(lower, "error:") || strings.Contains(lower, "failed:") || strings.Contains(lower, "panic:") {
			// error signals in output
		} else {
			score += 1.0
		}
	} else {
		score += 1.0 // no output to check, assume ok
	}

	if components == 0 {
		return 0.0
	}
	return score / float64(components)
}

// RecordLatency records a response duration for a provider in the sliding window.
func (cr *CascadeRouter) RecordLatency(provider string, d time.Duration) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	samples := cr.latencies[provider]
	if len(samples) >= latencyWindowSize {
		// Drop the oldest sample.
		samples = samples[1:]
	}
	cr.latencies[provider] = append(samples, d)
}

// GetProviderLatency returns the current latency stats for a provider, or nil
// if no samples have been recorded.
func (cr *CascadeRouter) GetProviderLatency(provider string) *ProviderLatency {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	samples := cr.latencies[provider]
	if len(samples) == 0 {
		return nil
	}

	return &ProviderLatency{
		P50:         computePercentile(samples, 50),
		P95:         computePercentile(samples, 95),
		Samples:     len(samples),
		LastUpdated: time.Now(),
	}
}

// cheapProviderSlow returns true if the cheap provider's P95 latency exceeds
// the configured LatencyThresholdMs. Returns false when the threshold is
// disabled (0), when there are no samples, or when P95 is within bounds.
func (cr *CascadeRouter) cheapProviderSlow() bool {
	if cr.config.LatencyThresholdMs <= 0 {
		return false
	}
	samples := cr.latencies[string(cr.config.CheapProvider)]
	if len(samples) == 0 {
		return false
	}
	p95 := computePercentile(samples, 95)
	threshold := time.Duration(cr.config.LatencyThresholdMs) * time.Millisecond
	return p95 > threshold
}

// computePercentile calculates the pth percentile of a duration slice using
// nearest-rank. The input slice is not modified.
func computePercentile(samples []time.Duration, p int) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	rank := int(math.Ceil(float64(p)/100.0*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}
