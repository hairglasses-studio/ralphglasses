// Package clients provides client implementations for external services.
package clients

import (
	"fmt"
	"math"
	"time"
)

// SourceLearner implements adaptive weight adjustment for feature sources
// Based on research: NewWeight = OldWeight × (1 + LearningRate × (SuccessRate - 0.5))
type SourceLearner struct {
	learningRate float64 // How quickly weights adjust (default: 0.1)
	minSamples   int     // Minimum samples before adjusting (default: 5)
	minWeight    float64 // Minimum weight to prevent zero-weighting (default: 0.1)
	maxWeight    float64 // Maximum weight to prevent runaway (default: 5.0)
}

// NewSourceLearner creates a new source learner with default settings
// Defaults tuned for faster convergence based on lessons learned:
// - LearningRate 0.2 (was 0.1): 2x faster weight adjustments
// - MinSamples 3 (was 5): Learn from fewer examples
func NewSourceLearner() *SourceLearner {
	return &SourceLearner{
		learningRate: 0.2, // Increased from 0.1 for faster convergence
		minSamples:   3,   // Reduced from 5 for quicker adaptation
		minWeight:    0.1,
		maxWeight:    5.0,
	}
}

// NewSourceLearnerWithConfig creates a source learner with custom settings
func NewSourceLearnerWithConfig(learningRate float64, minSamples int, minWeight, maxWeight float64) *SourceLearner {
	return &SourceLearner{
		learningRate: learningRate,
		minSamples:   minSamples,
		minWeight:    minWeight,
		maxWeight:    maxWeight,
	}
}

// LearningResult contains the results of a learning cycle
type LearningResult struct {
	Timestamp       time.Time                       `json:"timestamp"`
	SourcesAnalyzed int                             `json:"sources_analyzed"`
	WeightsAdjusted int                             `json:"weights_adjusted"`
	Adjustments     map[FeatureSource]*WeightChange `json:"adjustments"`
	SkippedSources  []string                        `json:"skipped_sources"` // Sources with insufficient samples
}

// WeightChange represents a weight adjustment for a source
type WeightChange struct {
	OldWeight   float64 `json:"old_weight"`
	NewWeight   float64 `json:"new_weight"`
	SuccessRate float64 `json:"success_rate"`
	Successes   int     `json:"successes"`
	Failures    int     `json:"failures"`
	TotalSamples int    `json:"total_samples"`
	Reason      string  `json:"reason"`
}

// OutcomeRecord represents a proposal outcome for learning
type OutcomeRecord struct {
	ProposalID     string
	Source         FeatureSource
	Outcome        OutcomeType
	PRNumber       int
	MergeTimeHours float64
	ReviewComments int
	CreatedAt      time.Time
}

// OutcomeType represents the outcome of a proposal
type OutcomeType string

const (
	OutcomeMerged   OutcomeType = "merged"
	OutcomeRejected OutcomeType = "rejected"
	OutcomeFailed   OutcomeType = "failed"
	OutcomePending  OutcomeType = "pending"
)

// CalculateNewWeight computes the adjusted weight for a source
func (l *SourceLearner) CalculateNewWeight(currentWeight float64, stats *SourceStats) (float64, *WeightChange) {
	totalSamples := stats.Successes + stats.Failures

	change := &WeightChange{
		OldWeight:    currentWeight,
		Successes:    stats.Successes,
		Failures:     stats.Failures,
		TotalSamples: totalSamples,
	}

	// Need minimum samples before adjusting
	if totalSamples < l.minSamples {
		change.NewWeight = currentWeight
		change.Reason = fmt.Sprintf("insufficient samples (%d/%d required)", totalSamples, l.minSamples)
		return currentWeight, change
	}

	// Calculate success rate
	successRate := float64(stats.Successes) / float64(totalSamples)
	change.SuccessRate = successRate

	// Apply learning formula: NewWeight = OldWeight × (1 + LearningRate × (SuccessRate - 0.5))
	// This centers around 50% success rate as neutral
	// Above 50% increases weight, below 50% decreases weight
	adjustment := l.learningRate * (successRate - 0.5)
	newWeight := currentWeight * (1 + adjustment)

	// Clamp to bounds
	newWeight = math.Max(l.minWeight, math.Min(l.maxWeight, newWeight))

	change.NewWeight = newWeight

	// Determine reason
	if newWeight > currentWeight {
		change.Reason = fmt.Sprintf("success rate %.1f%% > 50%%, increasing weight", successRate*100)
	} else if newWeight < currentWeight {
		change.Reason = fmt.Sprintf("success rate %.1f%% < 50%%, decreasing weight", successRate*100)
	} else if newWeight == l.minWeight {
		change.Reason = fmt.Sprintf("at minimum weight (%.2f)", l.minWeight)
	} else if newWeight == l.maxWeight {
		change.Reason = fmt.Sprintf("at maximum weight (%.2f)", l.maxWeight)
	} else {
		change.Reason = "no significant change"
	}

	return newWeight, change
}

// RunLearning performs a full learning cycle, adjusting weights based on outcomes
func (l *SourceLearner) RunLearning(store *PerpetualStateStore, currentWeights map[FeatureSource]float64) (*LearningResult, error) {
	result := &LearningResult{
		Timestamp:   time.Now(),
		Adjustments: make(map[FeatureSource]*WeightChange),
	}

	// Get stats from the database
	stats, err := store.GetSourceStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get source stats: %w", err)
	}

	result.SourcesAnalyzed = len(stats)

	// Process each source
	for source, sourceStats := range stats {
		currentWeight := currentWeights[source]
		if currentWeight == 0 {
			currentWeight = 1.0 // Default weight
		}

		newWeight, change := l.CalculateNewWeight(currentWeight, sourceStats)
		result.Adjustments[source] = change

		// Check if weight actually changed
		if newWeight != currentWeight {
			// Update in database
			err := store.UpdateSourceWeight(source, newWeight, sourceStats.Successes, sourceStats.Failures)
			if err != nil {
				return nil, fmt.Errorf("failed to update weight for %s: %w", source, err)
			}
			result.WeightsAdjusted++
		}

		// Track skipped sources
		if sourceStats.Successes+sourceStats.Failures < l.minSamples {
			result.SkippedSources = append(result.SkippedSources, string(source))
		}
	}

	return result, nil
}

// GetRecommendedAction returns a human-readable recommendation based on weight
func GetRecommendedAction(source FeatureSource, weight float64) string {
	if weight >= 2.0 {
		return fmt.Sprintf("BOOST: %s is high-performing (weight: %.2f), prioritize these proposals", source, weight)
	} else if weight >= 1.5 {
		return fmt.Sprintf("GOOD: %s is performing well (weight: %.2f)", source, weight)
	} else if weight >= 0.5 {
		return fmt.Sprintf("NEUTRAL: %s is performing average (weight: %.2f)", source, weight)
	} else if weight >= 0.2 {
		return fmt.Sprintf("WEAK: %s is underperforming (weight: %.2f), consider reviewing source quality", source, weight)
	} else {
		return fmt.Sprintf("DEPRIORITIZE: %s has very low success rate (weight: %.2f), investigate root cause", source, weight)
	}
}

// CalculateConfidence returns a confidence score for the weight based on sample size
func (l *SourceLearner) CalculateConfidence(stats *SourceStats) float64 {
	totalSamples := stats.Successes + stats.Failures
	if totalSamples == 0 {
		return 0
	}

	// Confidence grows with sample size, capped at 100 samples
	// Using log scale so early samples matter more
	confidence := math.Log10(float64(totalSamples)+1) / math.Log10(101)
	return math.Min(1.0, confidence)
}

// ShouldResetSource determines if a source should have its stats reset
// (e.g., after major changes to the source)
func (l *SourceLearner) ShouldResetSource(stats *SourceStats, lastReset time.Time, resetIntervalDays int) bool {
	// Reset if we've accumulated enough samples over the reset interval
	daysSinceReset := time.Since(lastReset).Hours() / 24
	return daysSinceReset >= float64(resetIntervalDays) && (stats.Successes+stats.Failures) >= l.minSamples*10
}

// WeightedScore applies the learned weight to a base score
func WeightedScore(baseScore float64, sourceWeight float64) float64 {
	return baseScore * sourceWeight
}

// AnalyzeSourcePerformance provides detailed analysis of a source's performance
type SourcePerformanceAnalysis struct {
	Source           FeatureSource
	CurrentWeight    float64
	SuccessRate      float64
	TotalProposals   int
	MergedCount      int
	RejectedCount    int
	FailedCount      int
	PendingCount     int
	AvgMergeTimeHrs  float64
	AvgReviewComments float64
	Trend            string // "improving", "declining", "stable"
	Confidence       float64
	Recommendation   string
}

// AnalyzeSource provides detailed performance analysis for a source
func (l *SourceLearner) AnalyzeSource(source FeatureSource, outcomes []OutcomeRecord, currentWeight float64) *SourcePerformanceAnalysis {
	analysis := &SourcePerformanceAnalysis{
		Source:        source,
		CurrentWeight: currentWeight,
	}

	if len(outcomes) == 0 {
		analysis.Recommendation = "No data available yet"
		return analysis
	}

	var totalMergeTime, totalComments float64
	var mergeCount int

	for _, o := range outcomes {
		analysis.TotalProposals++
		switch o.Outcome {
		case OutcomeMerged:
			analysis.MergedCount++
			totalMergeTime += o.MergeTimeHours
			totalComments += float64(o.ReviewComments)
			mergeCount++
		case OutcomeRejected:
			analysis.RejectedCount++
		case OutcomeFailed:
			analysis.FailedCount++
		case OutcomePending:
			analysis.PendingCount++
		}
	}

	// Calculate success rate (merged / (merged + rejected + failed))
	decidedCount := analysis.MergedCount + analysis.RejectedCount + analysis.FailedCount
	if decidedCount > 0 {
		analysis.SuccessRate = float64(analysis.MergedCount) / float64(decidedCount)
	}

	// Calculate averages
	if mergeCount > 0 {
		analysis.AvgMergeTimeHrs = totalMergeTime / float64(mergeCount)
		analysis.AvgReviewComments = totalComments / float64(mergeCount)
	}

	// Calculate confidence
	stats := &SourceStats{Successes: analysis.MergedCount, Failures: analysis.RejectedCount + analysis.FailedCount}
	analysis.Confidence = l.CalculateConfidence(stats)

	// Determine trend (simplified - would need time-series in real impl)
	analysis.Trend = "stable"

	// Generate recommendation
	analysis.Recommendation = GetRecommendedAction(source, currentWeight)

	return analysis
}

// v24.0: StreamingLearningResult contains the result of an immediate learning update
type StreamingLearningResult struct {
	Source          FeatureSource `json:"source"`
	Success         bool          `json:"success"`
	OldWeight       float64       `json:"old_weight"`
	NewWeight       float64       `json:"new_weight"`
	LearningRate    float64       `json:"learning_rate"`    // Effective learning rate used
	ConfidenceLevel float64       `json:"confidence_level"` // How confident we are in this adjustment
	Reason          string        `json:"reason"`
}

// v24.0: LearnFromOutcome performs immediate weight adjustment when a PR outcome is recorded
// This implements streaming learning - weights are updated immediately rather than waiting for batch
// The learning rate is modulated by confidence in the source
func (l *SourceLearner) LearnFromOutcome(store *PerpetualStateStore, source FeatureSource, success bool, currentWeight float64) (*StreamingLearningResult, error) {
	result := &StreamingLearningResult{
		Source:    source,
		Success:   success,
		OldWeight: currentWeight,
	}

	// Get current stats to calculate confidence
	stats, err := store.GetSourceStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get source stats: %w", err)
	}

	sourceStats, ok := stats[source]
	if !ok {
		sourceStats = &SourceStats{Successes: 0, Failures: 0}
	}

	// Calculate confidence - higher confidence means we adjust more aggressively
	result.ConfidenceLevel = l.CalculateConfidence(sourceStats)

	// Modulate learning rate by confidence (confidence-weighted learning)
	// Low confidence (few samples) = slower learning rate
	// High confidence (many samples) = full learning rate
	effectiveLearningRate := l.learningRate * (0.5 + 0.5*result.ConfidenceLevel)
	result.LearningRate = effectiveLearningRate

	// Calculate immediate adjustment
	// For a single outcome: adjust based on success/failure
	// Success: small positive adjustment
	// Failure: small negative adjustment
	var adjustment float64
	if success {
		adjustment = effectiveLearningRate * 0.5 // Move towards higher weight
	} else {
		adjustment = -effectiveLearningRate * 0.5 // Move towards lower weight
	}

	newWeight := currentWeight * (1 + adjustment)

	// Clamp to bounds
	newWeight = math.Max(l.minWeight, math.Min(l.maxWeight, newWeight))
	result.NewWeight = newWeight

	// Determine reason
	if newWeight > currentWeight {
		result.Reason = fmt.Sprintf("success: weight increased (lr=%.3f, conf=%.2f)", effectiveLearningRate, result.ConfidenceLevel)
	} else if newWeight < currentWeight {
		result.Reason = fmt.Sprintf("failure: weight decreased (lr=%.3f, conf=%.2f)", effectiveLearningRate, result.ConfidenceLevel)
	} else if newWeight == l.minWeight {
		result.Reason = fmt.Sprintf("at minimum weight (%.2f)", l.minWeight)
	} else if newWeight == l.maxWeight {
		result.Reason = fmt.Sprintf("at maximum weight (%.2f)", l.maxWeight)
	} else {
		result.Reason = "no significant change"
	}

	// Update weight in database immediately
	if newWeight != currentWeight {
		// Get updated counts
		newSuccesses := sourceStats.Successes
		newFailures := sourceStats.Failures
		if success {
			newSuccesses++
		} else {
			newFailures++
		}
		err = store.UpdateSourceWeight(source, newWeight, newSuccesses, newFailures)
		if err != nil {
			return result, fmt.Errorf("failed to update weight: %w", err)
		}
	}

	return result, nil
}
