// Package clients provides the runtime self-improvement loop for swarm workers.
// This enables workers to learn and adapt during a single run, improving
// autonomy score by reducing the need for manual intervention.
package clients

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// RuntimeLoop implements a self-improving feedback loop that runs during swarm execution.
// It monitors worker performance, identifies patterns, and adjusts strategies in real-time.
type RuntimeLoop struct {
	mu              sync.RWMutex
	orchestrator    *SwarmOrchestrator
	learningClient  *SwarmLearningClient
	correctionTracker *CorrectionTracker
	autonomyTracker *SwarmAutonomyTracker

	// Runtime state
	startTime        time.Time
	cycleCount       int
	lastCycleTime    time.Time
	cycleIntervalMin time.Duration

	// Performance tracking
	workerMetrics    map[SwarmWorkerType]*RuntimeWorkerMetrics
	categoryHits     map[string]int
	patternVelocity  map[string]float64 // patterns per hour

	// Adaptation state
	adaptations      []*RuntimeAdaptation
	budgetMultipliers map[SwarmWorkerType]float64
	strategyOverrides map[SwarmWorkerType]*WorkerStrategyConfig

	// Configuration
	config           *RuntimeLoopConfig

	// v37.0: Advanced strategies (lazy initialized)
	advancedStrategies *AdvancedRuntimeStrategies
	advancedOnce       sync.Once
}

// RuntimeWorkerMetrics tracks per-worker performance during this run
type RuntimeWorkerMetrics struct {
	WorkerType      SwarmWorkerType
	RunCount        int
	FindingsTotal   int
	DuplicatesTotal int
	ErrorCount      int
	TokensUsed      int64
	AvgFindingsPerRun float64
	SuccessStreak   int
	FailStreak      int
	LastRunTime     time.Time
	LastYield       int
	Trend           string // "improving", "stable", "declining"
}

// RuntimeAdaptation represents an adaptation made during the run
type RuntimeAdaptation struct {
	Timestamp       time.Time              `json:"timestamp"`
	AdaptationID    string                 `json:"adaptation_id"`
	Type            RuntimeAdaptationType  `json:"type"`
	Worker          SwarmWorkerType        `json:"worker,omitempty"`
	Description     string                 `json:"description"`
	BeforeValue     string                 `json:"before_value"`
	AfterValue      string                 `json:"after_value"`
	Trigger         string                 `json:"trigger"`
	ImpactScore     float64                `json:"impact_score"`
	WasEffective    *bool                  `json:"was_effective,omitempty"`
}

// RuntimeAdaptationType categorizes adaptations
type RuntimeAdaptationType string

const (
	AdaptBudgetIncrease  RuntimeAdaptationType = "budget_increase"
	AdaptBudgetDecrease  RuntimeAdaptationType = "budget_decrease"
	AdaptStrategyChange  RuntimeAdaptationType = "strategy_change"
	AdaptPriorityBoost   RuntimeAdaptationType = "priority_boost"
	AdaptPriorityDemote  RuntimeAdaptationType = "priority_demote"
	AdaptCategoryFocus   RuntimeAdaptationType = "category_focus"
	AdaptPatternSkip     RuntimeAdaptationType = "pattern_skip"
	AdaptEarlyExit       RuntimeAdaptationType = "early_exit"
	AdaptThresholdTune   RuntimeAdaptationType = "threshold_tune"
)

// RuntimeLoopConfig configures the runtime loop behavior
type RuntimeLoopConfig struct {
	// CycleIntervalMin is minimum time between adaptation cycles
	CycleIntervalMin time.Duration
	// MinRunsBeforeAdapt is minimum worker runs before adapting
	MinRunsBeforeAdapt int
	// SuccessStreakThreshold triggers budget increase
	SuccessStreakThreshold int
	// FailStreakThreshold triggers budget decrease
	FailStreakThreshold int
	// DuplicateRatioThreshold triggers pattern skip
	DuplicateRatioThreshold float64
	// BudgetIncreaseMultiplier for successful workers
	BudgetIncreaseMultiplier float64
	// BudgetDecreaseMultiplier for failing workers
	BudgetDecreaseMultiplier float64
	// MaxBudgetMultiplier caps budget increases
	MaxBudgetMultiplier float64
	// MinBudgetMultiplier prevents too-low budgets
	MinBudgetMultiplier float64
	// AdaptationCooldown prevents rapid re-adaptation
	AdaptationCooldown time.Duration
	// EnableAutoReflection triggers periodic self-assessment
	EnableAutoReflection bool
	// ReflectionInterval is time between auto-reflections
	ReflectionInterval time.Duration
}

// DefaultRuntimeLoopConfig returns sensible defaults
func DefaultRuntimeLoopConfig() *RuntimeLoopConfig {
	return &RuntimeLoopConfig{
		CycleIntervalMin:         5 * time.Minute,
		MinRunsBeforeAdapt:       2,
		SuccessStreakThreshold:   3,
		FailStreakThreshold:      2,
		DuplicateRatioThreshold:  0.5,
		BudgetIncreaseMultiplier: 1.25,
		BudgetDecreaseMultiplier: 0.75,
		MaxBudgetMultiplier:      2.0,
		MinBudgetMultiplier:      0.25,
		AdaptationCooldown:       10 * time.Minute,
		EnableAutoReflection:     true,
		ReflectionInterval:       30 * time.Minute,
	}
}

// NewRuntimeLoop creates a new runtime improvement loop
func NewRuntimeLoop(orchestrator *SwarmOrchestrator, config *RuntimeLoopConfig) *RuntimeLoop {
	if config == nil {
		config = DefaultRuntimeLoopConfig()
	}

	learningClient, _ := GetSwarmLearningClient()

	return &RuntimeLoop{
		orchestrator:      orchestrator,
		learningClient:    learningClient,
		correctionTracker: NewCorrectionTracker(),
		autonomyTracker:   GetSwarmAutonomyTracker(),
		startTime:         time.Now(),
		cycleIntervalMin:  config.CycleIntervalMin,
		workerMetrics:     make(map[SwarmWorkerType]*RuntimeWorkerMetrics),
		categoryHits:      make(map[string]int),
		patternVelocity:   make(map[string]float64),
		adaptations:       make([]*RuntimeAdaptation, 0),
		budgetMultipliers: make(map[SwarmWorkerType]float64),
		strategyOverrides: make(map[SwarmWorkerType]*WorkerStrategyConfig),
		config:            config,
	}
}

// RecordWorkerRun records a worker execution result
func (r *RuntimeLoop) RecordWorkerRun(workerType SwarmWorkerType, findings, duplicates int, tokensUsed int64, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	metrics, ok := r.workerMetrics[workerType]
	if !ok {
		metrics = &RuntimeWorkerMetrics{WorkerType: workerType}
		r.workerMetrics[workerType] = metrics
	}

	metrics.RunCount++
	metrics.FindingsTotal += findings
	metrics.DuplicatesTotal += duplicates
	metrics.TokensUsed += tokensUsed
	metrics.LastRunTime = time.Now()
	metrics.LastYield = findings

	if err != nil {
		metrics.ErrorCount++
		metrics.SuccessStreak = 0
		metrics.FailStreak++
	} else if findings > 0 {
		metrics.SuccessStreak++
		metrics.FailStreak = 0
	} else {
		// Zero findings but no error - neutral
		metrics.SuccessStreak = 0
		metrics.FailStreak++
	}

	// Update average
	if metrics.RunCount > 0 {
		metrics.AvgFindingsPerRun = float64(metrics.FindingsTotal) / float64(metrics.RunCount)
	}

	// Determine trend
	metrics.Trend = r.calculateTrend(metrics)

	// Record autonomy event if self-correcting
	if findings > 0 && duplicates == 0 {
		r.autonomyTracker.RecordAutonomyEvent("finding_success",
			fmt.Sprintf("%s produced %d unique findings", workerType, findings))
	}
}

// RecordFinding records a finding for pattern analysis
func (r *RuntimeLoop) RecordFinding(category string, pattern string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.categoryHits[category]++

	// Update pattern velocity
	runtime := time.Since(r.startTime).Hours()
	if runtime > 0 {
		r.patternVelocity[pattern] = float64(r.categoryHits[category]) / runtime
	}
}

// RunAdaptationCycle runs a single adaptation cycle
func (r *RuntimeLoop) RunAdaptationCycle(ctx context.Context) *AdaptationCycleResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if enough time has passed
	if time.Since(r.lastCycleTime) < r.cycleIntervalMin {
		return nil
	}

	r.cycleCount++
	r.lastCycleTime = time.Now()

	result := &AdaptationCycleResult{
		CycleNumber:  r.cycleCount,
		Timestamp:    time.Now(),
		Adaptations:  make([]*RuntimeAdaptation, 0),
		WorkerStats:  r.getWorkerStatsSnapshot(),
	}

	// 1. Identify high performers - increase budget
	for workerType, metrics := range r.workerMetrics {
		if metrics.RunCount < r.config.MinRunsBeforeAdapt {
			continue
		}

		// High performer: success streak
		if metrics.SuccessStreak >= r.config.SuccessStreakThreshold {
			adaptation := r.increaseBudget(workerType, metrics, "success_streak")
			if adaptation != nil {
				result.Adaptations = append(result.Adaptations, adaptation)
			}
		}

		// Low performer: fail streak
		if metrics.FailStreak >= r.config.FailStreakThreshold {
			adaptation := r.decreaseBudget(workerType, metrics, "fail_streak")
			if adaptation != nil {
				result.Adaptations = append(result.Adaptations, adaptation)
			}
		}

		// High duplicate ratio - adjust strategy
		if metrics.FindingsTotal > 0 {
			dupRatio := float64(metrics.DuplicatesTotal) / float64(metrics.FindingsTotal+metrics.DuplicatesTotal)
			if dupRatio > r.config.DuplicateRatioThreshold {
				adaptation := r.adjustStrategy(workerType, metrics, "high_duplicates")
				if adaptation != nil {
					result.Adaptations = append(result.Adaptations, adaptation)
				}
			}
		}
	}

	// 2. Category focus - boost workers in hot categories
	hotCategories := r.identifyHotCategories()
	for _, category := range hotCategories {
		workers := r.getWorkersForCategory(category)
		for _, workerType := range workers {
			adaptation := r.boostPriority(workerType, fmt.Sprintf("hot_category:%s", category))
			if adaptation != nil {
				result.Adaptations = append(result.Adaptations, adaptation)
			}
		}
	}

	// 3. Auto-reflection if enabled
	if r.config.EnableAutoReflection && r.cycleCount%6 == 0 { // Every 6th cycle
		reflection := r.performSelfReflection()
		result.Reflection = reflection
	}

	// 4. v37.0: Apply advanced strategies
	advStrategies := r.getAdvancedStrategies()
	if advStrategies != nil {
		// Prepare data snapshots for advanced strategies (avoid nested locks)
		categoryHitsCopy := make(map[string]int)
		for k, v := range r.categoryHits {
			categoryHitsCopy[k] = v
		}
		runtimeHours := time.Since(r.startTime).Hours()

		workerMetricsCopy := make(map[SwarmWorkerType]*RuntimeWorkerMetrics)
		for k, v := range r.workerMetrics {
			workerMetricsCopy[k] = v
		}

		// Category focus adaptations
		catAdaptations := advStrategies.ApplyCategoryFocus(ctx, categoryHitsCopy, runtimeHours)
		result.Adaptations = append(result.Adaptations, catAdaptations...)

		// Pattern skip adaptations
		skipAdaptations := advStrategies.ApplyPatternSkip(ctx)
		result.Adaptations = append(result.Adaptations, skipAdaptations...)

		// Check early exit conditions
		for workerType, metrics := range r.workerMetrics {
			if exitAdaptation := advStrategies.CheckEarlyExit(workerType, metrics); exitAdaptation != nil {
				result.Adaptations = append(result.Adaptations, exitAdaptation)
			}
		}

		// Cross-worker learning: identify and share patterns
		patterns := advStrategies.IdentifyPatternCandidates(workerMetricsCopy)
		for _, pattern := range patterns {
			shareAdaptations := advStrategies.SharePatternAcrossWorkers(pattern.SourceWorker, pattern)
			result.Adaptations = append(result.Adaptations, shareAdaptations...)
		}

		// Update volatility and sync with persistence
		advStrategies.UpdateVolatility(len(result.Adaptations))
		advStrategies.UpdateFromRuntimeLoop(workerMetricsCopy, result.Adaptations)
	}

	// Apply adaptations
	result.TotalAdaptations = len(result.Adaptations)
	r.adaptations = append(r.adaptations, result.Adaptations...)

	// Record autonomy event
	if result.TotalAdaptations > 0 {
		r.autonomyTracker.RecordAutonomyEvent("self_correction",
			fmt.Sprintf("Applied %d runtime adaptations in cycle %d", result.TotalAdaptations, r.cycleCount))
	}

	return result
}

// getAdvancedStrategies returns the advanced strategies, initializing if needed
func (r *RuntimeLoop) getAdvancedStrategies() *AdvancedRuntimeStrategies {
	r.advancedOnce.Do(func() {
		r.advancedStrategies = NewAdvancedRuntimeStrategies(r)
	})
	return r.advancedStrategies
}

// GetBudgetMultiplier returns the current budget multiplier for a worker
func (r *RuntimeLoop) GetBudgetMultiplier(workerType SwarmWorkerType) float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if mult, ok := r.budgetMultipliers[workerType]; ok {
		return mult
	}
	return 1.0
}

// GetStrategyOverride returns any strategy override for a worker
func (r *RuntimeLoop) GetStrategyOverride(workerType SwarmWorkerType) *WorkerStrategyConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.strategyOverrides[workerType]
}

// GetRuntimeReport returns a summary of runtime improvements
func (r *RuntimeLoop) GetRuntimeReport() *RuntimeReport {
	r.mu.RLock()
	defer r.mu.RUnlock()

	report := &RuntimeReport{
		Timestamp:       time.Now(),
		RuntimeDuration: time.Since(r.startTime),
		CycleCount:      r.cycleCount,
		WorkerMetrics:   make(map[SwarmWorkerType]*RuntimeWorkerMetrics),
		Adaptations:     r.adaptations,
		CategoryHits:    r.categoryHits,
	}

	// Copy metrics
	for k, v := range r.workerMetrics {
		metricsCopy := *v
		report.WorkerMetrics[k] = &metricsCopy
	}

	// Calculate improvement score
	report.ImprovementScore = r.calculateImprovementScore()

	// Calculate adaptation effectiveness
	report.AdaptationEffectiveness = r.calculateAdaptationEffectiveness()

	return report
}

// --- Internal helpers ---

func (r *RuntimeLoop) calculateTrend(metrics *RuntimeWorkerMetrics) string {
	if metrics.RunCount < 3 {
		return "unknown"
	}
	if metrics.SuccessStreak >= 2 {
		return "improving"
	}
	if metrics.FailStreak >= 2 {
		return "declining"
	}
	return "stable"
}

func (r *RuntimeLoop) increaseBudget(workerType SwarmWorkerType, metrics *RuntimeWorkerMetrics, trigger string) *RuntimeAdaptation {
	current := r.budgetMultipliers[workerType]
	if current == 0 {
		current = 1.0
	}

	newMult := current * r.config.BudgetIncreaseMultiplier
	if newMult > r.config.MaxBudgetMultiplier {
		return nil // Already at max
	}

	r.budgetMultipliers[workerType] = newMult

	return &RuntimeAdaptation{
		Timestamp:     time.Now(),
		AdaptationID:  fmt.Sprintf("adapt-%d-%s", r.cycleCount, workerType),
		Type:          AdaptBudgetIncrease,
		Worker:        workerType,
		Description:   fmt.Sprintf("Increased budget for %s", workerType),
		BeforeValue:   fmt.Sprintf("%.2fx", current),
		AfterValue:    fmt.Sprintf("%.2fx", newMult),
		Trigger:       trigger,
		ImpactScore:   float64(metrics.SuccessStreak) * 0.1,
	}
}

func (r *RuntimeLoop) decreaseBudget(workerType SwarmWorkerType, metrics *RuntimeWorkerMetrics, trigger string) *RuntimeAdaptation {
	current := r.budgetMultipliers[workerType]
	if current == 0 {
		current = 1.0
	}

	newMult := current * r.config.BudgetDecreaseMultiplier
	if newMult < r.config.MinBudgetMultiplier {
		return nil // Already at min
	}

	r.budgetMultipliers[workerType] = newMult

	return &RuntimeAdaptation{
		Timestamp:     time.Now(),
		AdaptationID:  fmt.Sprintf("adapt-%d-%s", r.cycleCount, workerType),
		Type:          AdaptBudgetDecrease,
		Worker:        workerType,
		Description:   fmt.Sprintf("Decreased budget for %s", workerType),
		BeforeValue:   fmt.Sprintf("%.2fx", current),
		AfterValue:    fmt.Sprintf("%.2fx", newMult),
		Trigger:       trigger,
		ImpactScore:   -float64(metrics.FailStreak) * 0.1,
	}
}

func (r *RuntimeLoop) adjustStrategy(workerType SwarmWorkerType, metrics *RuntimeWorkerMetrics, trigger string) *RuntimeAdaptation {
	// Create strategy that focuses on unique findings
	strategy := &WorkerStrategyConfig{
		MinUniqueFindings: true,
		SemanticThreshold: 0.9, // Higher threshold = fewer duplicates
		CategoryFocus:     r.getHotCategoryForWorker(workerType),
	}

	r.strategyOverrides[workerType] = strategy

	return &RuntimeAdaptation{
		Timestamp:     time.Now(),
		AdaptationID:  fmt.Sprintf("adapt-%d-%s-strategy", r.cycleCount, workerType),
		Type:          AdaptStrategyChange,
		Worker:        workerType,
		Description:   fmt.Sprintf("Adjusted strategy for %s to reduce duplicates", workerType),
		BeforeValue:   "default",
		AfterValue:    "high_uniqueness",
		Trigger:       trigger,
		ImpactScore:   0.2,
	}
}

func (r *RuntimeLoop) boostPriority(workerType SwarmWorkerType, trigger string) *RuntimeAdaptation {
	// Already have boost
	if mult, ok := r.budgetMultipliers[workerType]; ok && mult >= 1.5 {
		return nil
	}

	r.budgetMultipliers[workerType] = 1.5

	return &RuntimeAdaptation{
		Timestamp:     time.Now(),
		AdaptationID:  fmt.Sprintf("adapt-%d-%s-priority", r.cycleCount, workerType),
		Type:          AdaptPriorityBoost,
		Worker:        workerType,
		Description:   fmt.Sprintf("Boosted priority for %s", workerType),
		BeforeValue:   "normal",
		AfterValue:    "high",
		Trigger:       trigger,
		ImpactScore:   0.15,
	}
}

func (r *RuntimeLoop) identifyHotCategories() []string {
	type catCount struct {
		category string
		count    int
	}

	var counts []catCount
	for cat, count := range r.categoryHits {
		counts = append(counts, catCount{cat, count})
	}

	sort.Slice(counts, func(i, j int) bool {
		return counts[i].count > counts[j].count
	})

	// Return top 3 hot categories
	result := make([]string, 0, 3)
	for i := 0; i < len(counts) && i < 3; i++ {
		if counts[i].count >= 3 { // Minimum threshold
			result = append(result, counts[i].category)
		}
	}
	return result
}

func (r *RuntimeLoop) getWorkersForCategory(category string) []SwarmWorkerType {
	// Map categories to relevant workers
	categoryWorkers := map[string][]SwarmWorkerType{
		"security":     {WorkerSecurityAuditor, WorkerComplianceAudit},
		"performance":  {WorkerPerformanceProfiler, WorkerMetaIntel},
		"code_quality": {WorkerCodeQuality, WorkerBestPractices},
		"tools":        {WorkerToolAuditor, WorkerFeatureDiscovery},
		"patterns":     {WorkerPatternDiscovery, WorkerSemanticIntel},
	}

	if workers, ok := categoryWorkers[category]; ok {
		return workers
	}
	return nil
}

func (r *RuntimeLoop) getHotCategoryForWorker(workerType SwarmWorkerType) string {
	// Return most active category relevant to this worker
	workerCategories := map[SwarmWorkerType][]string{
		WorkerSecurityAuditor:     {"security", "compliance"},
		WorkerPerformanceProfiler: {"performance", "efficiency"},
		WorkerToolAuditor:         {"tools", "mcp"},
		WorkerPatternDiscovery:    {"patterns", "trends"},
	}

	if cats, ok := workerCategories[workerType]; ok {
		for _, cat := range cats {
			if r.categoryHits[cat] > 0 {
				return cat
			}
		}
	}
	return ""
}

func (r *RuntimeLoop) getWorkerStatsSnapshot() map[SwarmWorkerType]*RuntimeWorkerMetrics {
	stats := make(map[SwarmWorkerType]*RuntimeWorkerMetrics)
	for k, v := range r.workerMetrics {
		metricsCopy := *v
		stats[k] = &metricsCopy
	}
	return stats
}

func (r *RuntimeLoop) performSelfReflection() *SelfReflection {
	// Calculate overall health
	totalFindings := 0
	totalDuplicates := 0
	for _, m := range r.workerMetrics {
		totalFindings += m.FindingsTotal
		totalDuplicates += m.DuplicatesTotal
	}

	uniqueRatio := 1.0
	if totalFindings+totalDuplicates > 0 {
		uniqueRatio = float64(totalFindings) / float64(totalFindings+totalDuplicates)
	}

	reflection := &SelfReflection{
		Timestamp:    time.Now(),
		ReflectionID: fmt.Sprintf("reflect-%d", r.cycleCount),
		Subject:      "runtime_performance",
		Assessment:   fmt.Sprintf("Cycle %d: %d findings, %.0f%% unique, %d adaptations",
			r.cycleCount, totalFindings, uniqueRatio*100, len(r.adaptations)),
		QualityScore:    uniqueRatio * 100,
		OutcomePositive: uniqueRatio >= 0.5,
	}

	if uniqueRatio < 0.5 {
		reflection.ActionsProposed = []string{
			"Increase semantic threshold globally",
			"Focus on high-yield workers only",
			"Enable stricter deduplication",
		}
	}

	// Record to correction tracker
	r.correctionTracker.RecordReflection(reflection)

	log.Printf("runtime-loop: self-reflection cycle %d - quality score %.1f%%",
		r.cycleCount, reflection.QualityScore)

	return reflection
}

func (r *RuntimeLoop) calculateImprovementScore() float64 {
	if len(r.workerMetrics) == 0 {
		return 0
	}

	improvingCount := 0
	for _, m := range r.workerMetrics {
		if m.Trend == "improving" {
			improvingCount++
		}
	}

	return float64(improvingCount) / float64(len(r.workerMetrics)) * 100
}

func (r *RuntimeLoop) calculateAdaptationEffectiveness() float64 {
	if len(r.adaptations) == 0 {
		return 0
	}

	effectiveCount := 0
	for _, a := range r.adaptations {
		if a.WasEffective != nil && *a.WasEffective {
			effectiveCount++
		}
	}

	return float64(effectiveCount) / float64(len(r.adaptations)) * 100
}

// --- Types ---

// AdaptationCycleResult contains results from an adaptation cycle
type AdaptationCycleResult struct {
	CycleNumber      int                                    `json:"cycle_number"`
	Timestamp        time.Time                              `json:"timestamp"`
	Adaptations      []*RuntimeAdaptation                   `json:"adaptations"`
	TotalAdaptations int                                    `json:"total_adaptations"`
	WorkerStats      map[SwarmWorkerType]*RuntimeWorkerMetrics `json:"worker_stats"`
	Reflection       *SelfReflection                        `json:"reflection,omitempty"`
}

// RuntimeReport provides a comprehensive runtime summary
type RuntimeReport struct {
	Timestamp               time.Time                              `json:"timestamp"`
	RuntimeDuration         time.Duration                          `json:"runtime_duration"`
	CycleCount              int                                    `json:"cycle_count"`
	WorkerMetrics           map[SwarmWorkerType]*RuntimeWorkerMetrics `json:"worker_metrics"`
	Adaptations             []*RuntimeAdaptation                   `json:"adaptations"`
	CategoryHits            map[string]int                         `json:"category_hits"`
	ImprovementScore        float64                                `json:"improvement_score"`
	AdaptationEffectiveness float64                                `json:"adaptation_effectiveness"`
}

// WorkerStrategyConfig defines worker-specific strategy overrides
type WorkerStrategyConfig struct {
	MinUniqueFindings bool    `json:"min_unique_findings"`
	SemanticThreshold float64 `json:"semantic_threshold"`
	CategoryFocus     string  `json:"category_focus,omitempty"`
	MaxTokens         int64   `json:"max_tokens,omitempty"`
	Priority          int     `json:"priority,omitempty"`
}

// FormatRuntimeReport formats the report for display
func FormatRuntimeReport(report *RuntimeReport) string {
	var sb strings.Builder

	sb.WriteString("# Swarm Runtime Improvement Report\n\n")
	sb.WriteString(fmt.Sprintf("**Runtime:** %v\n", report.RuntimeDuration.Round(time.Minute)))
	sb.WriteString(fmt.Sprintf("**Adaptation Cycles:** %d\n", report.CycleCount))
	sb.WriteString(fmt.Sprintf("**Improvement Score:** %.1f%%\n", report.ImprovementScore))
	sb.WriteString(fmt.Sprintf("**Adaptation Effectiveness:** %.1f%%\n\n", report.AdaptationEffectiveness))

	// Worker performance
	sb.WriteString("## Worker Performance\n\n")
	sb.WriteString("| Worker | Runs | Findings | Duplicates | Trend |\n")
	sb.WriteString("|--------|------|----------|------------|-------|\n")
	for worker, m := range report.WorkerMetrics {
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %s |\n",
			worker, m.RunCount, m.FindingsTotal, m.DuplicatesTotal, m.Trend))
	}

	// Adaptations
	if len(report.Adaptations) > 0 {
		sb.WriteString("\n## Adaptations Applied\n\n")
		for _, a := range report.Adaptations {
			sb.WriteString(fmt.Sprintf("- **%s** %s: %s → %s (trigger: %s)\n",
				a.Type, a.Worker, a.BeforeValue, a.AfterValue, a.Trigger))
		}
	}

	return sb.String()
}

// RuntimeLoopStats provides a summary of runtime loop statistics
type RuntimeLoopStats struct {
	CycleCount        int                                        `json:"cycle_count"`
	TotalAdaptations  int                                        `json:"total_adaptations"`
	AutonomyScore     float64                                    `json:"autonomy_score"`
	SelfCorrections   int                                        `json:"self_corrections"`
	WorkerPerformance map[SwarmWorkerType]*RuntimeWorkerMetrics  `json:"worker_performance"`
}

// GetStats returns current runtime loop statistics
func (r *RuntimeLoop) GetStats() *RuntimeLoopStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Calculate autonomy score (based on effective adaptations and low intervention)
	autonomyScore := 0.5 // Base score
	if len(r.adaptations) > 0 {
		effectiveCount := 0
		for _, a := range r.adaptations {
			if a.WasEffective != nil && *a.WasEffective {
				effectiveCount++
			}
		}
		autonomyScore = float64(effectiveCount) / float64(len(r.adaptations))
	}

	// Count self-corrections (adaptations that improved performance)
	selfCorrections := 0
	for _, a := range r.adaptations {
		if a.Type == AdaptBudgetIncrease || a.Type == AdaptPriorityBoost || a.Type == AdaptStrategyChange {
			selfCorrections++
		}
	}

	// Get worker performance snapshot
	workerPerf := make(map[SwarmWorkerType]*RuntimeWorkerMetrics)
	for k, v := range r.workerMetrics {
		metricsCopy := *v
		workerPerf[k] = &metricsCopy
	}

	return &RuntimeLoopStats{
		CycleCount:        r.cycleCount,
		TotalAdaptations:  len(r.adaptations),
		AutonomyScore:     autonomyScore,
		SelfCorrections:   selfCorrections,
		WorkerPerformance: workerPerf,
	}
}

// GetAdaptations returns all adaptations made during the run
func (r *RuntimeLoop) GetAdaptations() []*RuntimeAdaptation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to avoid concurrent modification
	result := make([]*RuntimeAdaptation, len(r.adaptations))
	for i, a := range r.adaptations {
		adaptCopy := *a
		result[i] = &adaptCopy
	}
	return result
}

// EvaluateAdaptationEffectiveness evaluates whether past adaptations were effective
// by comparing worker performance before and after the adaptation.
// Call this periodically (e.g., every 5-10 adaptation cycles) to update WasEffective.
func (r *RuntimeLoop) EvaluateAdaptationEffectiveness() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	evaluated := 0
	minEvalTime := 2 * time.Minute // Minimum time before evaluating

	for _, a := range r.adaptations {
		// Skip if already evaluated or too recent
		if a.WasEffective != nil {
			continue
		}
		if time.Since(a.Timestamp) < minEvalTime {
			continue
		}

		// Get current metrics for the worker
		metrics, ok := r.workerMetrics[a.Worker]
		if !ok {
			continue
		}

		// Evaluate based on adaptation type
		var effective bool
		switch a.Type {
		case AdaptBudgetIncrease:
			// Effective if worker is improving or stable with good findings
			effective = metrics.Trend == "improving" || (metrics.Trend == "stable" && metrics.AvgFindingsPerRun > 1)
		case AdaptBudgetDecrease:
			// Effective if worker started producing findings again
			effective = metrics.LastYield > 0 || metrics.SuccessStreak > 0
		case AdaptStrategyChange:
			// Effective if duplicate ratio decreased
			if metrics.FindingsTotal+metrics.DuplicatesTotal > 0 {
				dupRatio := float64(metrics.DuplicatesTotal) / float64(metrics.FindingsTotal+metrics.DuplicatesTotal)
				effective = dupRatio < r.config.DuplicateRatioThreshold
			}
		case AdaptPriorityBoost:
			// Effective if findings increased
			effective = metrics.AvgFindingsPerRun > 2
		default:
			// For other types, default to effective if trend is improving
			effective = metrics.Trend == "improving"
		}

		a.WasEffective = &effective
		evaluated++

		// Log for visibility
		log.Printf("[RuntimeLoop] Evaluated adaptation %s for %s: effective=%v", a.AdaptationID, a.Worker, effective)
	}

	return evaluated
}
