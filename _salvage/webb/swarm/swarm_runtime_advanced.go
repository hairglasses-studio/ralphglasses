// Package clients provides advanced runtime strategies for the self-improving swarm.
// v37.0: Cross-worker learning, sophisticated strategies, and PR feedback loop
package clients

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// AdvancedRuntimeStrategies implements sophisticated adaptation strategies
type AdvancedRuntimeStrategies struct {
	mu              sync.RWMutex
	runtimeLoop     *RuntimeLoop
	persistence     *RuntimePersistence
	learning        *PersistedLearning

	// Cross-worker learning state
	workerSimilarity map[SwarmWorkerType][]SwarmWorkerType
	patternSharing   map[string]*SharedPattern

	// Volatility tracking for adaptive intervals
	volatilityWindow []float64
	currentVolatility float64
	intervalAdjustments int

	// Category focus state
	categoryPriorities map[string]float64
	categoryVelocities map[string]float64

	// Pattern skip state
	skippedPatterns    map[string]time.Time
	patternYieldHistory map[string][]int

	// Early exit conditions
	exitConditions     []*EarlyExitCondition
	exitTriggered      bool
	exitReason         string
}

// SharedPattern represents a pattern shared between workers
type SharedPattern struct {
	PatternID      string            `json:"pattern_id"`
	SourceWorker   SwarmWorkerType   `json:"source_worker"`
	TargetWorkers  []SwarmWorkerType `json:"target_workers"`
	Category       string            `json:"category"`
	SuccessRate    float64           `json:"success_rate"`
	TimesShared    int               `json:"times_shared"`
	LastShared     time.Time         `json:"last_shared"`
}

// EarlyExitCondition defines when to stop a worker early
type EarlyExitCondition struct {
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	ConsecutiveZeros int    `json:"consecutive_zeros"`      // Consecutive zero-finding runs
	DuplicateRatio  float64 `json:"duplicate_ratio"`        // Ratio above which to exit
	TokenThreshold  int64   `json:"token_threshold"`        // Max tokens before exit
	TimeThreshold   time.Duration `json:"time_threshold"`   // Max time before exit
	Priority        int     `json:"priority"`               // Higher = checked first
}

// WorkerSimilarityScore represents similarity between two workers
type WorkerSimilarityScore struct {
	Worker1    SwarmWorkerType
	Worker2    SwarmWorkerType
	Score      float64 // 0-1, higher = more similar
	SharedCats []string
	Reason     string
}

// NewAdvancedRuntimeStrategies creates advanced strategies manager
func NewAdvancedRuntimeStrategies(runtimeLoop *RuntimeLoop) *AdvancedRuntimeStrategies {
	persistence := GetRuntimePersistence()
	learning, err := persistence.Load()
	if err != nil {
		log.Printf("[AdvancedStrategies] Failed to load learning, starting fresh: %v", err)
		learning = persistence.newLearning()
	}

	ars := &AdvancedRuntimeStrategies{
		runtimeLoop:        runtimeLoop,
		persistence:        persistence,
		learning:           learning,
		workerSimilarity:   make(map[SwarmWorkerType][]SwarmWorkerType),
		patternSharing:     make(map[string]*SharedPattern),
		volatilityWindow:   make([]float64, 0, 20),
		categoryPriorities: make(map[string]float64),
		categoryVelocities: make(map[string]float64),
		skippedPatterns:    make(map[string]time.Time),
		patternYieldHistory: make(map[string][]int),
		exitConditions:     defaultExitConditions(),
	}

	// Initialize worker similarity from learning
	ars.initializeWorkerSimilarity()

	return ars
}

// defaultExitConditions returns sensible default early exit conditions
func defaultExitConditions() []*EarlyExitCondition {
	return []*EarlyExitCondition{
		{
			Name:             "consecutive_zero",
			Description:      "Exit after consecutive zero-finding runs",
			ConsecutiveZeros: 5,
			Priority:         10,
		},
		{
			Name:            "high_duplicate",
			Description:     "Exit when duplicate ratio exceeds threshold",
			DuplicateRatio:  0.8,
			Priority:        9,
		},
		{
			Name:           "token_budget",
			Description:    "Exit when token budget exceeded",
			TokenThreshold: 100000,
			Priority:       8,
		},
		{
			Name:          "time_limit",
			Description:   "Exit after time threshold",
			TimeThreshold: 30 * time.Minute,
			Priority:      7,
		},
	}
}

// initializeWorkerSimilarity builds worker similarity map from categories
func (ars *AdvancedRuntimeStrategies) initializeWorkerSimilarity() {
	// Define worker categories for similarity matching
	workerCategories := map[SwarmWorkerType][]string{
		WorkerToolAuditor:         {"tools", "quality", "compliance"},
		WorkerBestPractices:       {"quality", "compliance", "documentation"},
		WorkerSecurityAuditor:     {"security", "compliance", "vulnerability"},
		WorkerPerformanceProfiler: {"performance", "optimization", "metrics"},
		WorkerCodeQuality:         {"quality", "patterns", "refactoring"},
		WorkerDependency:          {"dependencies", "security", "updates"},
		WorkerTestCoverage:        {"testing", "quality", "coverage"},
		WorkerDocumentation:       {"documentation", "quality", "compliance"},
		WorkerKnowledgeGraph:      {"knowledge", "patterns", "connections"},
		WorkerPatternDiscovery:    {"patterns", "analysis", "insights"},
		WorkerImprovementAudit:    {"improvement", "quality", "optimization"},
	}

	// Calculate similarity between all worker pairs
	for w1, cats1 := range workerCategories {
		similar := make([]SwarmWorkerType, 0)
		for w2, cats2 := range workerCategories {
			if w1 == w2 {
				continue
			}
			// Count shared categories
			shared := 0
			for _, c1 := range cats1 {
				for _, c2 := range cats2 {
					if c1 == c2 {
						shared++
					}
				}
			}
			// If they share at least 1 category, consider them similar
			if shared > 0 {
				similar = append(similar, w2)
			}
		}
		ars.workerSimilarity[w1] = similar
	}
}

// --- Enhancement 2: Sophisticated Strategies ---

// ApplyCategoryFocus boosts workers working on hot categories
// categoryHits and runtimeHours should be passed by caller to avoid lock contention
func (ars *AdvancedRuntimeStrategies) ApplyCategoryFocus(ctx context.Context, categoryHits map[string]int, runtimeHours float64) []*RuntimeAdaptation {
	ars.mu.Lock()
	defer ars.mu.Unlock()

	var adaptations []*RuntimeAdaptation

	if runtimeHours <= 0 {
		runtimeHours = 0.1 // Minimum to avoid division by zero
	}

	// Calculate velocities and identify hot categories
	hotCategories := make([]string, 0)
	for category, hits := range categoryHits {
		velocity := float64(hits) / runtimeHours
		ars.categoryVelocities[category] = velocity

		// Hot if velocity > 2 findings per hour
		if velocity > 2.0 {
			hotCategories = append(hotCategories, category)
			ars.categoryPriorities[category] = velocity / 2.0 // Priority based on velocity
		}
	}

	// Create category focus adaptations
	for _, category := range hotCategories {
		workers := ars.getWorkersForCategory(category)
		for _, workerType := range workers {
			adaptation := &RuntimeAdaptation{
				Timestamp:    time.Now(),
				AdaptationID: fmt.Sprintf("cat_focus_%s_%s_%d", category, workerType, time.Now().UnixNano()),
				Type:         AdaptCategoryFocus,
				Worker:       workerType,
				Description:  fmt.Sprintf("Focus on hot category %s (velocity=%.1f/hr)", category, ars.categoryVelocities[category]),
				BeforeValue:  "normal_priority",
				AfterValue:   fmt.Sprintf("priority_boost_%.1fx", ars.categoryPriorities[category]),
				Trigger:      "hot_category_detected",
				ImpactScore:  ars.categoryPriorities[category] * 0.3,
			}
			adaptations = append(adaptations, adaptation)
		}
	}

	return adaptations
}

// ApplyPatternSkip identifies patterns that should be skipped
func (ars *AdvancedRuntimeStrategies) ApplyPatternSkip(ctx context.Context) []*RuntimeAdaptation {
	ars.mu.Lock()
	defer ars.mu.Unlock()

	var adaptations []*RuntimeAdaptation

	// Check pattern yield history for diminishing returns
	for pattern, yields := range ars.patternYieldHistory {
		if len(yields) < 5 {
			continue // Need enough history
		}

		// Check if yields are declining
		recent := yields[len(yields)-3:]
		recentAvg := (float64(recent[0]) + float64(recent[1]) + float64(recent[2])) / 3.0

		older := yields[:len(yields)-3]
		olderSum := 0.0
		for _, y := range older {
			olderSum += float64(y)
		}
		olderAvg := olderSum / float64(len(older))

		// Skip if recent yield is < 20% of historical average
		if olderAvg > 0 && recentAvg/olderAvg < 0.2 {
			// Skip this pattern for a while
			ars.skippedPatterns[pattern] = time.Now().Add(15 * time.Minute)

			adaptation := &RuntimeAdaptation{
				Timestamp:    time.Now(),
				AdaptationID: fmt.Sprintf("pattern_skip_%s_%d", pattern, time.Now().UnixNano()),
				Type:         AdaptPatternSkip,
				Description:  fmt.Sprintf("Skipping pattern %s due to diminishing returns (%.1f -> %.1f)", pattern, olderAvg, recentAvg),
				BeforeValue:  "active",
				AfterValue:   "skipped_15m",
				Trigger:      "diminishing_returns",
				ImpactScore:  0.2,
			}
			adaptations = append(adaptations, adaptation)
		}
	}

	return adaptations
}

// CheckEarlyExit checks if a worker should exit early
func (ars *AdvancedRuntimeStrategies) CheckEarlyExit(workerType SwarmWorkerType, metrics *RuntimeWorkerMetrics) *RuntimeAdaptation {
	ars.mu.RLock()
	defer ars.mu.RUnlock()

	// Sort conditions by priority
	conditions := make([]*EarlyExitCondition, len(ars.exitConditions))
	copy(conditions, ars.exitConditions)
	sort.Slice(conditions, func(i, j int) bool {
		return conditions[i].Priority > conditions[j].Priority
	})

	for _, cond := range conditions {
		shouldExit := false
		reason := ""

		switch {
		case cond.ConsecutiveZeros > 0 && metrics.FailStreak >= cond.ConsecutiveZeros:
			shouldExit = true
			reason = fmt.Sprintf("%d consecutive zero-finding runs", metrics.FailStreak)

		case cond.DuplicateRatio > 0 && metrics.FindingsTotal > 0:
			dupRatio := float64(metrics.DuplicatesTotal) / float64(metrics.FindingsTotal+metrics.DuplicatesTotal)
			if dupRatio >= cond.DuplicateRatio {
				shouldExit = true
				reason = fmt.Sprintf("duplicate ratio %.0f%% exceeds threshold", dupRatio*100)
			}

		case cond.TokenThreshold > 0 && metrics.TokensUsed >= cond.TokenThreshold:
			shouldExit = true
			reason = fmt.Sprintf("token usage %d exceeds threshold", metrics.TokensUsed)

		case cond.TimeThreshold > 0 && time.Since(metrics.LastRunTime) < 0: // Always check time
			// Time threshold checked by caller
		}

		if shouldExit {
			return &RuntimeAdaptation{
				Timestamp:    time.Now(),
				AdaptationID: fmt.Sprintf("early_exit_%s_%d", workerType, time.Now().UnixNano()),
				Type:         AdaptEarlyExit,
				Worker:       workerType,
				Description:  fmt.Sprintf("Early exit for %s: %s", workerType, reason),
				BeforeValue:  "running",
				AfterValue:   "stopped_early",
				Trigger:      cond.Name,
				ImpactScore:  0.5,
			}
		}
	}

	return nil
}

// RecordPatternYield records a yield for pattern tracking
func (ars *AdvancedRuntimeStrategies) RecordPatternYield(pattern string, yield int) {
	ars.mu.Lock()
	defer ars.mu.Unlock()

	if ars.patternYieldHistory[pattern] == nil {
		ars.patternYieldHistory[pattern] = make([]int, 0, 20)
	}
	ars.patternYieldHistory[pattern] = append(ars.patternYieldHistory[pattern], yield)

	// Keep last 20 yields
	if len(ars.patternYieldHistory[pattern]) > 20 {
		ars.patternYieldHistory[pattern] = ars.patternYieldHistory[pattern][1:]
	}
}

// --- Enhancement 3: Cross-Worker Learning ---

// SharePatternAcrossWorkers shares a successful pattern to similar workers
func (ars *AdvancedRuntimeStrategies) SharePatternAcrossWorkers(sourceWorker SwarmWorkerType, pattern *CrossWorkerPattern) []*RuntimeAdaptation {
	ars.mu.Lock()
	defer ars.mu.Unlock()

	var adaptations []*RuntimeAdaptation

	// Find similar workers
	similarWorkers, ok := ars.workerSimilarity[sourceWorker]
	if !ok || len(similarWorkers) == 0 {
		return adaptations
	}

	// Share pattern
	shared := &SharedPattern{
		PatternID:     pattern.PatternID,
		SourceWorker:  sourceWorker,
		TargetWorkers: similarWorkers,
		Category:      pattern.Category,
		SuccessRate:   pattern.SuccessRate,
		TimesShared:   1,
		LastShared:    time.Now(),
	}
	ars.patternSharing[pattern.PatternID] = shared

	// Create adaptations for each target worker
	for _, targetWorker := range similarWorkers {
		adaptation := &RuntimeAdaptation{
			Timestamp:    time.Now(),
			AdaptationID: fmt.Sprintf("share_%s_%s_%d", sourceWorker, targetWorker, time.Now().UnixNano()),
			Type:         AdaptStrategyChange,
			Worker:       targetWorker,
			Description:  fmt.Sprintf("Learned pattern %s from %s", pattern.PatternID, sourceWorker),
			BeforeValue:  "no_pattern",
			AfterValue:   fmt.Sprintf("pattern_%s", pattern.PatternID),
			Trigger:      "cross_worker_learning",
			ImpactScore:  pattern.Confidence * 0.4,
		}
		adaptations = append(adaptations, adaptation)
	}

	// Update persistence
	if ars.learning.SharedPatterns == nil {
		ars.learning.SharedPatterns = make([]*CrossWorkerPattern, 0)
	}
	ars.learning.SharedPatterns = append(ars.learning.SharedPatterns, pattern)
	ars.persistence.SaveIfNeeded(ars.learning)

	return adaptations
}

// GetLearnedPatternsForWorker returns patterns learned from other workers
func (ars *AdvancedRuntimeStrategies) GetLearnedPatternsForWorker(workerType SwarmWorkerType) []*CrossWorkerPattern {
	ars.mu.RLock()
	defer ars.mu.RUnlock()

	var patterns []*CrossWorkerPattern

	for _, pattern := range ars.learning.SharedPatterns {
		for _, applicable := range pattern.ApplicableWorkers {
			if applicable == workerType {
				patterns = append(patterns, pattern)
				break
			}
		}
	}

	return patterns
}

// IdentifyPatternCandidates identifies high-performing patterns that could be shared
func (ars *AdvancedRuntimeStrategies) IdentifyPatternCandidates(metrics map[SwarmWorkerType]*RuntimeWorkerMetrics) []*CrossWorkerPattern {
	ars.mu.Lock()
	defer ars.mu.Unlock()

	var candidates []*CrossWorkerPattern

	for workerType, m := range metrics {
		// Look for high-yield workers
		if m.AvgFindingsPerRun > 3 && m.RunCount >= 3 {
			// This worker found a good pattern
			pattern := &CrossWorkerPattern{
				PatternID:         fmt.Sprintf("high_yield_%s_%d", workerType, time.Now().UnixNano()),
				SourceWorker:      workerType,
				ApplicableWorkers: ars.workerSimilarity[workerType],
				Category:          "high_yield",
				PatternType:       "high_yield",
				Description:       fmt.Sprintf("High yield pattern from %s (avg %.1f findings/run)", workerType, m.AvgFindingsPerRun),
				Confidence:        math.Min(float64(m.RunCount)/10.0, 1.0),
				CreatedAt:         time.Now(),
			}
			candidates = append(candidates, pattern)
		}

		// Look for low-duplicate workers
		if m.FindingsTotal > 10 {
			dupRate := float64(m.DuplicatesTotal) / float64(m.FindingsTotal+m.DuplicatesTotal)
			if dupRate < 0.1 {
				pattern := &CrossWorkerPattern{
					PatternID:         fmt.Sprintf("low_dup_%s_%d", workerType, time.Now().UnixNano()),
					SourceWorker:      workerType,
					ApplicableWorkers: ars.workerSimilarity[workerType],
					Category:          "low_duplicate",
					PatternType:       "low_duplicate",
					Description:       fmt.Sprintf("Low duplicate pattern from %s (%.0f%% dup rate)", workerType, dupRate*100),
					Confidence:        math.Min(float64(m.FindingsTotal)/50.0, 1.0),
					CreatedAt:         time.Now(),
				}
				candidates = append(candidates, pattern)
			}
		}
	}

	return candidates
}

// --- Enhancement 4: Adaptive Cycle Intervals ---

// UpdateVolatility updates the volatility measure
func (ars *AdvancedRuntimeStrategies) UpdateVolatility(adaptationCount int) {
	ars.mu.Lock()
	defer ars.mu.Unlock()

	// Add to window
	ars.volatilityWindow = append(ars.volatilityWindow, float64(adaptationCount))

	// Keep window size at 20
	if len(ars.volatilityWindow) > 20 {
		ars.volatilityWindow = ars.volatilityWindow[1:]
	}

	// Calculate volatility as standard deviation
	if len(ars.volatilityWindow) < 3 {
		ars.currentVolatility = 0.5 // Default
		return
	}

	// Calculate mean
	sum := 0.0
	for _, v := range ars.volatilityWindow {
		sum += v
	}
	mean := sum / float64(len(ars.volatilityWindow))

	// Calculate variance
	variance := 0.0
	for _, v := range ars.volatilityWindow {
		variance += (v - mean) * (v - mean)
	}
	variance /= float64(len(ars.volatilityWindow))

	// Normalize to 0-1 range (assume max reasonable stddev is 5)
	ars.currentVolatility = math.Min(math.Sqrt(variance)/5.0, 1.0)
}

// GetAdaptiveInterval returns the optimal cycle interval based on volatility
func (ars *AdvancedRuntimeStrategies) GetAdaptiveInterval() time.Duration {
	ars.mu.RLock()
	defer ars.mu.RUnlock()

	intervals := ars.learning.OptimalIntervals
	if intervals == nil {
		return 5 * time.Minute
	}

	// High volatility = shorter intervals (more frequent adaptation)
	// Low volatility = longer intervals (less frequent adaptation)
	if ars.currentVolatility > intervals.VolatilityThreshold {
		return intervals.HighVolatilityInterval
	} else if ars.currentVolatility < intervals.VolatilityThreshold/2 {
		return intervals.LowVolatilityInterval
	}
	return intervals.BaseInterval
}

// CalibrateIntervals recalibrates optimal intervals based on historical data
func (ars *AdvancedRuntimeStrategies) CalibrateIntervals() *LearnedIntervals {
	ars.mu.Lock()
	defer ars.mu.Unlock()

	return ars.persistence.CalculateOptimalIntervals(ars.learning, ars.currentVolatility)
}

// --- Enhancement 5: PR Outcome Feedback Loop ---

// RecordPROutcome records a PR outcome and correlates with adaptations
func (ars *AdvancedRuntimeStrategies) RecordPROutcome(prID, prTitle, outcome string, mergedAt *time.Time, adaptations []*RuntimeAdaptation) {
	ars.mu.Lock()
	defer ars.mu.Unlock()

	// Extract adaptation IDs and types
	adaptIDs := make([]string, len(adaptations))
	adaptTypes := make([]RuntimeAdaptationType, len(adaptations))
	workers := make(map[SwarmWorkerType]bool)
	findingsCount := 0

	for i, a := range adaptations {
		adaptIDs[i] = a.AdaptationID
		adaptTypes[i] = a.Type
		if a.Worker != "" {
			workers[a.Worker] = true
		}
	}

	workerList := make([]SwarmWorkerType, 0, len(workers))
	for w := range workers {
		workerList = append(workerList, w)
	}

	// Calculate correlation score based on outcome
	correlationScore := 0.0
	switch outcome {
	case "merged":
		correlationScore = 1.0
	case "closed":
		correlationScore = -0.5
	case "pending":
		correlationScore = 0.5
	}

	correlation := &PRAdaptationCorrelation{
		PRID:             prID,
		PRTitle:          prTitle,
		PROutcome:        outcome,
		MergedAt:         mergedAt,
		AdaptationIDs:    adaptIDs,
		AdaptationTypes:  adaptTypes,
		WorkersInvolved:  workerList,
		FindingsCount:    findingsCount,
		CorrelationScore: correlationScore,
	}

	ars.persistence.RecordPRCorrelation(ars.learning, correlation)

	// Update adaptation effectiveness based on PR outcome
	if outcome == "merged" {
		for _, a := range adaptations {
			effective := true
			a.WasEffective = &effective
			ars.persistence.UpdateAdaptationStats(ars.learning, a)
		}
	} else if outcome == "closed" {
		for _, a := range adaptations {
			effective := false
			a.WasEffective = &effective
			ars.persistence.UpdateAdaptationStats(ars.learning, a)
		}
	}

	// Save learning
	ars.persistence.SaveIfNeeded(ars.learning)

	log.Printf("[AdvancedStrategies] Recorded PR outcome: %s (%s) with %d adaptations, correlation=%.2f",
		prID, outcome, len(adaptations), correlationScore)
}

// GetPRSuccessRate returns the success rate of PRs correlated with adaptations
func (ars *AdvancedRuntimeStrategies) GetPRSuccessRate() float64 {
	ars.mu.RLock()
	defer ars.mu.RUnlock()

	if len(ars.learning.PRCorrelations) == 0 {
		return 0.5 // Default
	}

	merged := 0
	for _, corr := range ars.learning.PRCorrelations {
		if corr.PROutcome == "merged" {
			merged++
		}
	}

	return float64(merged) / float64(len(ars.learning.PRCorrelations))
}

// LearnFromPRHistory analyzes PR history to improve adaptation strategies
func (ars *AdvancedRuntimeStrategies) LearnFromPRHistory() map[RuntimeAdaptationType]float64 {
	ars.mu.Lock()
	defer ars.mu.Unlock()

	// Calculate success rate per adaptation type
	typeSuccesses := make(map[RuntimeAdaptationType]int)
	typeTotal := make(map[RuntimeAdaptationType]int)

	for _, corr := range ars.learning.PRCorrelations {
		for _, adaptType := range corr.AdaptationTypes {
			typeTotal[adaptType]++
			if corr.PROutcome == "merged" {
				typeSuccesses[adaptType]++
			}
		}
	}

	// Calculate rates
	rates := make(map[RuntimeAdaptationType]float64)
	for adaptType, total := range typeTotal {
		if total > 0 {
			rates[adaptType] = float64(typeSuccesses[adaptType]) / float64(total)
		}
	}

	return rates
}

// --- Helper methods ---

func (ars *AdvancedRuntimeStrategies) getWorkersForCategory(category string) []SwarmWorkerType {
	// Map categories to relevant workers
	categoryWorkers := map[string][]SwarmWorkerType{
		"tools":        {WorkerToolAuditor, WorkerBestPractices},
		"quality":      {WorkerCodeQuality, WorkerBestPractices, WorkerToolAuditor},
		"security":     {WorkerSecurityAuditor, WorkerDependency},
		"performance":  {WorkerPerformanceProfiler},
		"patterns":     {WorkerPatternDiscovery, WorkerKnowledgeGraph},
		"documentation": {WorkerDocumentation, WorkerBestPractices},
		"testing":      {WorkerTestCoverage},
		"improvement":  {WorkerImprovementAudit},
	}

	// Normalize category
	category = strings.ToLower(category)
	for cat, workers := range categoryWorkers {
		if strings.Contains(category, cat) {
			return workers
		}
	}

	return nil
}

// SaveLearning forces a save of the current learning state
func (ars *AdvancedRuntimeStrategies) SaveLearning() error {
	ars.mu.Lock()
	defer ars.mu.Unlock()

	return ars.persistence.Save(ars.learning)
}

// GetLearning returns the current learning state
func (ars *AdvancedRuntimeStrategies) GetLearning() *PersistedLearning {
	ars.mu.RLock()
	defer ars.mu.RUnlock()

	return ars.learning
}

// UpdateFromRuntimeLoop syncs state from the runtime loop
// workerMetrics and adaptations should be passed by caller to avoid lock contention
func (ars *AdvancedRuntimeStrategies) UpdateFromRuntimeLoop(workerMetrics map[SwarmWorkerType]*RuntimeWorkerMetrics, adaptations []*RuntimeAdaptation) {
	ars.mu.Lock()
	defer ars.mu.Unlock()

	// Update worker history
	for _, metrics := range workerMetrics {
		ars.persistence.UpdateWorkerHistory(ars.learning, metrics)
	}

	// Update adaptation stats
	for _, adaptation := range adaptations {
		ars.persistence.UpdateAdaptationStats(ars.learning, adaptation)
	}

	// Save periodically
	ars.persistence.SaveIfNeeded(ars.learning)
}
