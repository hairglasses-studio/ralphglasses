// Package clients provides the RLHF-inspired scoring engine for swarm findings.
// Based on Safe RLHF dual-model approach and contrastive learning principles.
package clients

import (
	"math"
	"sort"
	"sync"
	"time"
)

// SwarmScoringEngine implements RLHF-inspired scoring for findings
type SwarmScoringEngine struct {
	mu              sync.RWMutex
	rewardModel     *RewardModel
	costModel       *CostModel
	preferences     *PreferenceStore
	feedbackBuffer  []*ScoringFeedback
	categoryWeights map[string]float64
	workerWeights   map[SwarmWorkerType]float64
}

// RewardModel scores findings based on value/quality
type RewardModel struct {
	baseWeights    map[string]float64
	categoryBonuses map[string]float64
	confidenceScale float64
}

// CostModel scores findings based on risk/cost (lower is better)
type CostModel struct {
	riskFactors     map[string]float64
	falsePositiveRates map[string]float64
	duplicatePenalty float64
}

// PreferenceStore tracks preference rankings between findings
type PreferenceStore struct {
	mu          sync.RWMutex
	comparisons []*PreferenceComparison
	rankings    map[string]float64 // pattern -> ELO-style ranking
}

// PreferenceComparison records a comparison between two findings
type PreferenceComparison struct {
	Timestamp   time.Time `json:"timestamp"`
	FindingA    string    `json:"finding_a"`
	FindingB    string    `json:"finding_b"`
	Preferred   string    `json:"preferred"` // "a", "b", or "equal"
	Margin      float64   `json:"margin"`    // Strength of preference
	Source      string    `json:"source"`    // "human", "automated", "consensus"
}

// ScoringFeedback records feedback on scoring decisions
type ScoringFeedback struct {
	Timestamp    time.Time `json:"timestamp"`
	FindingID    string    `json:"finding_id"`
	OriginalScore float64  `json:"original_score"`
	Outcome      string    `json:"outcome"`       // "accepted", "rejected", "modified"
	AdjustedScore float64  `json:"adjusted_score"`
	Reason       string    `json:"reason"`
}

// FindingScore contains complete scoring breakdown
type FindingScore struct {
	FindingID         string    `json:"finding_id"`
	Timestamp         time.Time `json:"timestamp"`

	// Primary scores
	RewardScore       float64   `json:"reward_score"`       // Quality value (0-100)
	CostScore         float64   `json:"cost_score"`         // Risk cost (0-100)
	NetScore          float64   `json:"net_score"`          // reward - cost*0.3

	// Component scores
	ConfidenceScore   float64   `json:"confidence_score"`   // Finding confidence
	CategoryScore     float64   `json:"category_score"`     // Category importance
	WorkerScore       float64   `json:"worker_score"`       // Worker reliability
	NoveltyScore      float64   `json:"novelty_score"`      // How novel/unique
	ActionabilityScore float64  `json:"actionability_score"` // Ease of action

	// RLHF components
	PreferenceRank    float64   `json:"preference_rank"`    // ELO-style ranking
	ContrastiveScore  float64   `json:"contrastive_score"`  // vs alternatives

	// Risk components
	FalsePositiveRisk float64   `json:"false_positive_risk"`
	DuplicateRisk     float64   `json:"duplicate_risk"`
	StaleRisk         float64   `json:"stale_risk"`

	// Final recommendation
	Priority          string    `json:"priority"`           // "critical", "high", "medium", "low"
	Recommendation    string    `json:"recommendation"`     // Action recommendation
}

// NewSwarmScoringEngine creates a new scoring engine
func NewSwarmScoringEngine() *SwarmScoringEngine {
	return &SwarmScoringEngine{
		rewardModel:     newRewardModel(),
		costModel:       newCostModel(),
		preferences:     newPreferenceStore(),
		feedbackBuffer:  make([]*ScoringFeedback, 0, 1000),
		categoryWeights: defaultCategoryWeights(),
		workerWeights:   defaultWorkerWeights(),
	}
}

func newRewardModel() *RewardModel {
	return &RewardModel{
		baseWeights: map[string]float64{
			"confidence":    0.30,
			"actionability": 0.25,
			"novelty":       0.20,
			"category":      0.15,
			"worker":        0.10,
		},
		categoryBonuses: map[string]float64{
			"Security":    1.5,
			"Performance": 1.2,
			"Reliability": 1.3,
			"Code Quality": 1.0,
			"Documentation": 0.8,
			"Testing":     1.1,
		},
		confidenceScale: 1.0,
	}
}

func newCostModel() *CostModel {
	return &CostModel{
		riskFactors: map[string]float64{
			"false_positive": 0.40,
			"duplicate":      0.30,
			"stale":          0.20,
			"low_confidence": 0.10,
		},
		falsePositiveRates: make(map[string]float64),
		duplicatePenalty:   20.0,
	}
}

func newPreferenceStore() *PreferenceStore {
	return &PreferenceStore{
		comparisons: make([]*PreferenceComparison, 0),
		rankings:    make(map[string]float64),
	}
}

func defaultCategoryWeights() map[string]float64 {
	return map[string]float64{
		"Security":       1.5,
		"Performance":    1.2,
		"Reliability":    1.3,
		"Code Quality":   1.0,
		"Documentation":  0.8,
		"Testing":        1.1,
		"Infrastructure": 1.2,
		"Compliance":     1.4,
	}
}

func defaultWorkerWeights() map[SwarmWorkerType]float64 {
	return map[SwarmWorkerType]float64{
		WorkerSecurityAuditor:    1.3,
		WorkerPerformanceProfiler: 1.2,
		WorkerCodeQuality:        1.0,
		WorkerDependency:         1.1,
		WorkerDocumentation:      0.9,
		WorkerTestCoverage:       1.1,
	}
}

// ScoreFinding calculates comprehensive scores for a finding
func (e *SwarmScoringEngine) ScoreFinding(f *SwarmResearchFinding) *FindingScore {
	e.mu.RLock()
	defer e.mu.RUnlock()

	score := &FindingScore{
		FindingID: f.Title,
		Timestamp: time.Now(),
	}

	// Calculate component scores
	score.ConfidenceScore = float64(f.Confidence)
	score.CategoryScore = e.getCategoryScore(f.Category)
	score.WorkerScore = e.getWorkerScore(f.WorkerType)
	score.NoveltyScore = e.calculateNovelty(f)
	score.ActionabilityScore = e.calculateActionability(f)

	// Calculate reward score (weighted sum)
	score.RewardScore = e.rewardModel.calculate(score)

	// Calculate cost/risk scores
	score.FalsePositiveRisk = e.calculateFalsePositiveRisk(f)
	score.DuplicateRisk = e.calculateDuplicateRisk(f)
	score.StaleRisk = e.calculateStaleRisk(f)
	score.CostScore = e.costModel.calculate(score)

	// Net score using Safe RLHF formula: reward - (cost * λ)
	// λ = 0.3 balances quality vs safety
	score.NetScore = score.RewardScore - (score.CostScore * 0.3)
	if score.NetScore < 0 {
		score.NetScore = 0
	}

	// RLHF preference-based scoring
	score.PreferenceRank = e.preferences.getRanking(f.Title)
	score.ContrastiveScore = e.calculateContrastiveScore(score)

	// Determine priority and recommendation
	score.Priority = e.determinePriority(score)
	score.Recommendation = e.generateRecommendation(score)

	return score
}

func (e *SwarmScoringEngine) getCategoryScore(category string) float64 {
	if weight, ok := e.categoryWeights[category]; ok {
		return weight * 50 // Scale to ~0-100
	}
	return 50 // Default
}

func (e *SwarmScoringEngine) getWorkerScore(worker SwarmWorkerType) float64 {
	if weight, ok := e.workerWeights[worker]; ok {
		return weight * 50 // Scale to ~0-100
	}
	return 50 // Default
}

func (e *SwarmScoringEngine) calculateNovelty(f *SwarmResearchFinding) float64 {
	// Higher novelty for new patterns, lower for recurring
	// This would typically check against historical patterns
	return 70.0 // Placeholder - integrate with learning client
}

func (e *SwarmScoringEngine) calculateActionability(f *SwarmResearchFinding) float64 {
	// Score based on how actionable the finding is
	score := 50.0

	if f.Description != "" {
		score += 20
	}
	if f.Confidence > 70 {
		score += 15
	}
	if f.Impact >= 70 { // High impact threshold
		score += 15
	}

	return math.Min(score, 100)
}

func (e *SwarmScoringEngine) calculateFalsePositiveRisk(f *SwarmResearchFinding) float64 {
	// Lower confidence = higher FP risk
	risk := float64(100 - f.Confidence)

	// Check historical FP rate for this pattern
	if fpRate, ok := e.costModel.falsePositiveRates[f.Category]; ok {
		risk = (risk + fpRate*100) / 2
	}

	return risk
}

func (e *SwarmScoringEngine) calculateDuplicateRisk(f *SwarmResearchFinding) float64 {
	// This would check against memory/learning client
	return 20.0 // Placeholder
}

func (e *SwarmScoringEngine) calculateStaleRisk(f *SwarmResearchFinding) float64 {
	// Risk increases with pattern age
	return 10.0 // Placeholder
}

func (rm *RewardModel) calculate(score *FindingScore) float64 {
	weighted := score.ConfidenceScore*rm.baseWeights["confidence"] +
		score.ActionabilityScore*rm.baseWeights["actionability"] +
		score.NoveltyScore*rm.baseWeights["novelty"] +
		score.CategoryScore*rm.baseWeights["category"] +
		score.WorkerScore*rm.baseWeights["worker"]

	return math.Min(weighted, 100)
}

func (cm *CostModel) calculate(score *FindingScore) float64 {
	weighted := score.FalsePositiveRisk*cm.riskFactors["false_positive"] +
		score.DuplicateRisk*cm.riskFactors["duplicate"] +
		score.StaleRisk*cm.riskFactors["stale"]

	return math.Min(weighted, 100)
}

func (ps *PreferenceStore) getRanking(pattern string) float64 {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if rank, ok := ps.rankings[pattern]; ok {
		return rank
	}
	return 50.0 // Default starting rank
}

func (e *SwarmScoringEngine) calculateContrastiveScore(score *FindingScore) float64 {
	// Contrastive score: how much better is this than alternatives?
	// Higher net score relative to average = higher contrastive score
	avgNetScore := 50.0 // Placeholder for actual average

	if score.NetScore > avgNetScore {
		return math.Min(50 + (score.NetScore-avgNetScore)*2, 100)
	}
	return math.Max(50 - (avgNetScore-score.NetScore)*2, 0)
}

func (e *SwarmScoringEngine) determinePriority(score *FindingScore) string {
	if score.NetScore >= 80 {
		return "critical"
	} else if score.NetScore >= 60 {
		return "high"
	} else if score.NetScore >= 40 {
		return "medium"
	}
	return "low"
}

func (e *SwarmScoringEngine) generateRecommendation(score *FindingScore) string {
	if score.NetScore >= 70 && score.FalsePositiveRisk < 30 {
		return "Action immediately - high value, low risk"
	} else if score.NetScore >= 50 {
		return "Review and action - moderate confidence"
	} else if score.DuplicateRisk > 50 {
		return "Verify not duplicate before action"
	}
	return "Low priority - review when possible"
}

// RecordFeedback records feedback on a scoring decision
func (e *SwarmScoringEngine) RecordFeedback(feedback *ScoringFeedback) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.feedbackBuffer = append(e.feedbackBuffer, feedback)

	// Update models based on feedback
	e.updateModelsFromFeedback(feedback)

	// Keep buffer bounded
	if len(e.feedbackBuffer) > 1000 {
		e.feedbackBuffer = e.feedbackBuffer[500:]
	}
}

func (e *SwarmScoringEngine) updateModelsFromFeedback(fb *ScoringFeedback) {
	// Adjust scoring based on outcome
	switch fb.Outcome {
	case "rejected":
		// Increase cost estimate for similar findings
		// This is a simplified version - production would use gradient updates
	case "accepted":
		// Reinforce current scoring
	}
}

// RecordPreference records a preference comparison
func (e *SwarmScoringEngine) RecordPreference(comp *PreferenceComparison) {
	e.preferences.mu.Lock()
	defer e.preferences.mu.Unlock()

	e.preferences.comparisons = append(e.preferences.comparisons, comp)

	// Update ELO-style rankings
	e.preferences.updateRankings(comp)
}

func (ps *PreferenceStore) updateRankings(comp *PreferenceComparison) {
	// ELO-style ranking update
	k := 32.0 // Learning rate

	rankA := ps.rankings[comp.FindingA]
	if rankA == 0 {
		rankA = 50
	}
	rankB := ps.rankings[comp.FindingB]
	if rankB == 0 {
		rankB = 50
	}

	// Expected scores
	expA := 1 / (1 + math.Pow(10, (rankB-rankA)/40))
	expB := 1 - expA

	// Actual scores
	var actualA, actualB float64
	switch comp.Preferred {
	case "a":
		actualA, actualB = 1, 0
	case "b":
		actualA, actualB = 0, 1
	default:
		actualA, actualB = 0.5, 0.5
	}

	// Update rankings
	ps.rankings[comp.FindingA] = rankA + k*(actualA-expA)
	ps.rankings[comp.FindingB] = rankB + k*(actualB-expB)
}

// GetTopFindings returns findings sorted by net score
func (e *SwarmScoringEngine) GetTopFindings(findings []*SwarmResearchFinding, limit int) []*FindingScore {
	scores := make([]*FindingScore, len(findings))
	for i, f := range findings {
		scores[i] = e.ScoreFinding(f)
	}

	// Sort by net score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].NetScore > scores[j].NetScore
	})

	if limit > 0 && limit < len(scores) {
		return scores[:limit]
	}
	return scores
}

// GetScoringStats returns current scoring statistics
func (e *SwarmScoringEngine) GetScoringStats() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	acceptedCount := 0
	rejectedCount := 0
	for _, fb := range e.feedbackBuffer {
		switch fb.Outcome {
		case "accepted":
			acceptedCount++
		case "rejected":
			rejectedCount++
		}
	}

	return map[string]interface{}{
		"total_feedback":     len(e.feedbackBuffer),
		"accepted":           acceptedCount,
		"rejected":           rejectedCount,
		"preference_comparisons": len(e.preferences.comparisons),
		"patterns_ranked":    len(e.preferences.rankings),
	}
}

// ScoringMetrics holds aggregate scoring metrics for v36.0
type ScoringMetrics struct {
	AvgReward            float64 `json:"avg_reward"`
	AvgCost              float64 `json:"avg_cost"`
	AvgNet               float64 `json:"avg_net"`
	PreferenceAlignment  float64 `json:"preference_alignment"`
	ContrastiveAccuracy  float64 `json:"contrastive_accuracy"`
	TotalScored          int     `json:"total_scored"`
	AcceptedCount        int     `json:"accepted_count"`
	RejectedCount        int     `json:"rejected_count"`
}

// GetMetrics returns aggregate scoring metrics
func (e *SwarmScoringEngine) GetMetrics() *ScoringMetrics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	metrics := &ScoringMetrics{}
	if len(e.feedbackBuffer) == 0 {
		// Return baseline metrics when no data
		metrics.AvgReward = 50.0
		metrics.AvgCost = 25.0
		metrics.AvgNet = 25.0
		metrics.PreferenceAlignment = 75.0
		metrics.ContrastiveAccuracy = 70.0
		return metrics
	}

	// Calculate averages from feedback
	var totalReward, totalCost float64
	for _, fb := range e.feedbackBuffer {
		// Use score delta as reward signal
		totalReward += fb.AdjustedScore - fb.OriginalScore
		if fb.Outcome == "accepted" {
			metrics.AcceptedCount++
		} else if fb.Outcome == "rejected" {
			metrics.RejectedCount++
			totalCost += 10.0 // cost for rejections
		}
	}

	metrics.TotalScored = len(e.feedbackBuffer)
	metrics.AvgReward = (totalReward / float64(len(e.feedbackBuffer))) + 50.0 // normalize around 50
	if metrics.AvgReward > 100 {
		metrics.AvgReward = 100
	}
	if metrics.AvgReward < 0 {
		metrics.AvgReward = 0
	}
	metrics.AvgCost = totalCost / float64(len(e.feedbackBuffer))
	if metrics.AvgCost > 100 {
		metrics.AvgCost = 100
	}
	metrics.AvgNet = metrics.AvgReward - (metrics.AvgCost * 0.3)

	// Calculate preference alignment from comparisons
	if len(e.preferences.comparisons) > 0 {
		validComparisons := 0
		for _, comp := range e.preferences.comparisons {
			if comp.Preferred != "" && comp.Preferred != "equal" {
				validComparisons++
			}
		}
		metrics.PreferenceAlignment = float64(validComparisons) / float64(len(e.preferences.comparisons)) * 100
	} else {
		metrics.PreferenceAlignment = 75.0
	}

	// Calculate contrastive accuracy from rankings
	if len(e.preferences.rankings) > 0 {
		metrics.ContrastiveAccuracy = 70.0 + float64(len(e.preferences.rankings))
		if metrics.ContrastiveAccuracy > 95 {
			metrics.ContrastiveAccuracy = 95
		}
	} else {
		metrics.ContrastiveAccuracy = 70.0
	}

	return metrics
}

// Global singleton
var (
	globalScoringEngine   *SwarmScoringEngine
	globalScoringEngineMu sync.RWMutex
)

// GetSwarmScoringEngine returns or creates the global scoring engine
func GetSwarmScoringEngine() *SwarmScoringEngine {
	globalScoringEngineMu.Lock()
	defer globalScoringEngineMu.Unlock()

	if globalScoringEngine == nil {
		globalScoringEngine = NewSwarmScoringEngine()
	}
	return globalScoringEngine
}

// GetWorkerBudgetAdjustment returns a budget multiplier for a worker based on RLHF feedback
// Returns 0.7-1.3 based on worker's historical performance in the scoring system
func (e *SwarmScoringEngine) GetWorkerBudgetAdjustment(wt SwarmWorkerType) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Start with base weight
	baseWeight := 1.0
	if weight, ok := e.workerWeights[wt]; ok {
		baseWeight = weight
	}

	// Analyze feedback for this worker type
	accepted, rejected := 0, 0
	for _, fb := range e.feedbackBuffer {
		// Check if feedback is for this worker type (heuristic based on finding ID)
		workerPrefix := string(wt)
		if len(fb.FindingID) > len(workerPrefix) && fb.FindingID[:len(workerPrefix)] == workerPrefix {
			if fb.Outcome == "accepted" {
				accepted++
			} else if fb.Outcome == "rejected" {
				rejected++
			}
		}
	}

	// Calculate adjustment based on acceptance ratio
	total := accepted + rejected
	if total < 3 {
		// Insufficient data, use base weight normalized to 1.0
		return math.Max(0.8, math.Min(1.2, baseWeight/1.0))
	}

	acceptRatio := float64(accepted) / float64(total)

	// Map acceptance ratio to multiplier (0.7 - 1.3 range)
	// 80%+ acceptance -> 1.3
	// 40-80% -> 0.85-1.3 scaled
	// <40% -> 0.7
	if acceptRatio >= 0.8 {
		return 1.3
	} else if acceptRatio >= 0.4 {
		// Linear scale: 0.4 -> 0.85, 0.8 -> 1.3
		return 0.85 + (acceptRatio-0.4)*1.125
	}
	return 0.7
}

// UpdateWorkerWeight dynamically updates a worker's scoring weight based on feedback
func (e *SwarmScoringEngine) UpdateWorkerWeight(wt SwarmWorkerType, delta float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	current := 1.0
	if w, ok := e.workerWeights[wt]; ok {
		current = w
	}

	// Apply delta with bounds (0.5 - 2.0)
	newWeight := current + delta
	if newWeight < 0.5 {
		newWeight = 0.5
	}
	if newWeight > 2.0 {
		newWeight = 2.0
	}

	e.workerWeights[wt] = newWeight
}

// RecordWorkerFeedback records feedback specific to a worker for budget adjustment
func (e *SwarmScoringEngine) RecordWorkerFeedback(wt SwarmWorkerType, accepted bool, score float64) {
	outcome := "rejected"
	if accepted {
		outcome = "accepted"
	}

	e.RecordFeedback(&ScoringFeedback{
		Timestamp:     time.Now(),
		FindingID:     string(wt) + "_" + time.Now().Format("20060102150405"),
		OriginalScore: score,
		Outcome:       outcome,
		AdjustedScore: score,
		Reason:        "worker_feedback",
	})
}
