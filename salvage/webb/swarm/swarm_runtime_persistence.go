// Package clients provides persistence for runtime learning across swarm runs.
// v37.0: Persistence layer for self-improving runtime loop
package clients

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RuntimePersistence handles saving and loading learning data across runs
type RuntimePersistence struct {
	mu          sync.RWMutex
	vaultPath   string
	learningDir string
	autoSave    bool
	saveInterval time.Duration
	lastSave    time.Time
}

// PersistedLearning represents the learning data saved across runs
type PersistedLearning struct {
	Version          string                          `json:"version"`
	LastUpdated      time.Time                       `json:"last_updated"`
	TotalRuns        int                             `json:"total_runs"`

	// Worker performance history
	WorkerHistory    map[SwarmWorkerType]*WorkerHistoricalData `json:"worker_history"`

	// Adaptation effectiveness rates
	AdaptationStats  map[RuntimeAdaptationType]*AdaptationHistoricalStats `json:"adaptation_stats"`

	// Cross-worker patterns
	SharedPatterns   []*CrossWorkerPattern           `json:"shared_patterns"`

	// Category heat map over time
	CategoryTrends   map[string]*CategoryTrend       `json:"category_trends"`

	// PR outcome correlations
	PRCorrelations   []*PRAdaptationCorrelation      `json:"pr_correlations"`

	// Optimal cycle intervals learned
	OptimalIntervals *LearnedIntervals               `json:"optimal_intervals"`
}

// WorkerHistoricalData tracks worker performance across multiple runs
type WorkerHistoricalData struct {
	WorkerType       SwarmWorkerType `json:"worker_type"`
	TotalRuns        int             `json:"total_runs"`
	TotalFindings    int             `json:"total_findings"`
	TotalDuplicates  int             `json:"total_duplicates"`
	AvgFindingsPerRun float64        `json:"avg_findings_per_run"`
	SuccessRate      float64         `json:"success_rate"`       // % of runs with findings
	DuplicateRate    float64         `json:"duplicate_rate"`     // % of duplicates
	BestBudgetMultiplier float64     `json:"best_budget_multiplier"`
	OptimalRunOrder  int             `json:"optimal_run_order"`  // When to run in cycle
	PeakPerformanceHours []int       `json:"peak_performance_hours"` // Hours of day with best results
	LastUpdated      time.Time       `json:"last_updated"`
}

// AdaptationHistoricalStats tracks effectiveness of each adaptation type
type AdaptationHistoricalStats struct {
	Type             RuntimeAdaptationType `json:"type"`
	TotalApplied     int                   `json:"total_applied"`
	EffectiveCount   int                   `json:"effective_count"`
	EffectivenessRate float64              `json:"effectiveness_rate"`
	AvgImpactScore   float64               `json:"avg_impact_score"`
	BestConditions   []string              `json:"best_conditions"` // When this adaptation works best
	LastUpdated      time.Time             `json:"last_updated"`
}

// CrossWorkerPattern represents a pattern discovered across workers
type CrossWorkerPattern struct {
	PatternID        string            `json:"pattern_id"`
	SourceWorker     SwarmWorkerType   `json:"source_worker"`
	ApplicableWorkers []SwarmWorkerType `json:"applicable_workers"`
	Category         string            `json:"category"`
	PatternType      string            `json:"pattern_type"` // "high_yield", "low_duplicate", "complementary"
	Description      string            `json:"description"`
	Confidence       float64           `json:"confidence"` // 0-1
	TimesApplied     int               `json:"times_applied"`
	SuccessRate      float64           `json:"success_rate"`
	CreatedAt        time.Time         `json:"created_at"`
	LastUsed         time.Time         `json:"last_used"`
}

// CategoryTrend tracks category activity over time
type CategoryTrend struct {
	Category         string    `json:"category"`
	TotalFindings    int       `json:"total_findings"`
	DailyAverage     float64   `json:"daily_average"`
	WeeklyTrend      string    `json:"weekly_trend"` // "increasing", "stable", "decreasing"
	SeasonalPattern  []float64 `json:"seasonal_pattern"` // 24 hours activity distribution
	HotPeriods       []string  `json:"hot_periods"`      // Time ranges with high activity
	LastUpdated      time.Time `json:"last_updated"`
}

// PRAdaptationCorrelation links adaptations to PR outcomes
type PRAdaptationCorrelation struct {
	PRID             string                `json:"pr_id"`
	PRTitle          string                `json:"pr_title"`
	PROutcome        string                `json:"pr_outcome"` // "merged", "closed", "pending"
	MergedAt         *time.Time            `json:"merged_at,omitempty"`
	AdaptationIDs    []string              `json:"adaptation_ids"`
	AdaptationTypes  []RuntimeAdaptationType `json:"adaptation_types"`
	WorkersInvolved  []SwarmWorkerType     `json:"workers_involved"`
	FindingsCount    int                   `json:"findings_count"`
	CorrelationScore float64               `json:"correlation_score"` // How strongly correlated
	CreatedAt        time.Time             `json:"created_at"`
}

// LearnedIntervals contains learned optimal cycle intervals
type LearnedIntervals struct {
	BaseInterval        time.Duration `json:"base_interval"`
	HighVolatilityInterval time.Duration `json:"high_volatility_interval"`
	LowVolatilityInterval time.Duration `json:"low_volatility_interval"`
	AdaptationCooldown  time.Duration `json:"adaptation_cooldown"`
	EvaluationInterval  int           `json:"evaluation_interval"` // Cycles between evaluations
	VolatilityThreshold float64       `json:"volatility_threshold"`
	LastCalibration     time.Time     `json:"last_calibration"`
}

var (
	globalPersistence     *RuntimePersistence
	globalPersistenceOnce sync.Once
)

// GetRuntimePersistence returns the singleton persistence instance
func GetRuntimePersistence() *RuntimePersistence {
	globalPersistenceOnce.Do(func() {
		vaultPath := os.Getenv("WEBB_VAULT_PATH")
		if vaultPath == "" {
			vaultPath = filepath.Join(os.Getenv("HOME"), "webb-vault")
		}

		globalPersistence = &RuntimePersistence{
			vaultPath:    vaultPath,
			learningDir:  filepath.Join(vaultPath, "runtime-learning"),
			autoSave:     true,
			saveInterval: 15 * time.Minute,
		}

		// Ensure directory exists
		os.MkdirAll(globalPersistence.learningDir, 0755)
	})
	return globalPersistence
}

// Load loads persisted learning from disk
func (p *RuntimePersistence) Load() (*PersistedLearning, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	filePath := filepath.Join(p.learningDir, "learning.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty learning
			return p.newLearning(), nil
		}
		return nil, fmt.Errorf("failed to read learning file: %w", err)
	}

	var learning PersistedLearning
	if err := json.Unmarshal(data, &learning); err != nil {
		return nil, fmt.Errorf("failed to parse learning file: %w", err)
	}

	return &learning, nil
}

// Save saves learning to disk
func (p *RuntimePersistence) Save(learning *PersistedLearning) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	learning.LastUpdated = time.Now()

	data, err := json.MarshalIndent(learning, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal learning: %w", err)
	}

	filePath := filepath.Join(p.learningDir, "learning.json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write learning file: %w", err)
	}

	p.lastSave = time.Now()
	log.Printf("[RuntimePersistence] Saved learning to %s", filePath)
	return nil
}

// SaveIfNeeded saves if enough time has passed since last save
func (p *RuntimePersistence) SaveIfNeeded(learning *PersistedLearning) error {
	if !p.autoSave {
		return nil
	}
	if time.Since(p.lastSave) < p.saveInterval {
		return nil
	}
	return p.Save(learning)
}

// newLearning creates a new empty learning structure
func (p *RuntimePersistence) newLearning() *PersistedLearning {
	return &PersistedLearning{
		Version:          "1.0",
		LastUpdated:      time.Now(),
		TotalRuns:        0,
		WorkerHistory:    make(map[SwarmWorkerType]*WorkerHistoricalData),
		AdaptationStats:  make(map[RuntimeAdaptationType]*AdaptationHistoricalStats),
		SharedPatterns:   make([]*CrossWorkerPattern, 0),
		CategoryTrends:   make(map[string]*CategoryTrend),
		PRCorrelations:   make([]*PRAdaptationCorrelation, 0),
		OptimalIntervals: &LearnedIntervals{
			BaseInterval:           5 * time.Minute,
			HighVolatilityInterval: 2 * time.Minute,
			LowVolatilityInterval:  10 * time.Minute,
			AdaptationCooldown:     10 * time.Minute,
			EvaluationInterval:     3,
			VolatilityThreshold:    0.3,
		},
	}
}

// UpdateWorkerHistory updates historical data for a worker
func (p *RuntimePersistence) UpdateWorkerHistory(learning *PersistedLearning, metrics *RuntimeWorkerMetrics) {
	if learning.WorkerHistory == nil {
		learning.WorkerHistory = make(map[SwarmWorkerType]*WorkerHistoricalData)
	}

	history, ok := learning.WorkerHistory[metrics.WorkerType]
	if !ok {
		history = &WorkerHistoricalData{
			WorkerType:          metrics.WorkerType,
			BestBudgetMultiplier: 1.0,
		}
		learning.WorkerHistory[metrics.WorkerType] = history
	}

	history.TotalRuns += metrics.RunCount
	history.TotalFindings += metrics.FindingsTotal
	history.TotalDuplicates += metrics.DuplicatesTotal

	if history.TotalRuns > 0 {
		history.AvgFindingsPerRun = float64(history.TotalFindings) / float64(history.TotalRuns)
		history.DuplicateRate = float64(history.TotalDuplicates) / float64(history.TotalFindings+history.TotalDuplicates+1)
	}

	history.LastUpdated = time.Now()
}

// UpdateAdaptationStats updates stats for an adaptation type
func (p *RuntimePersistence) UpdateAdaptationStats(learning *PersistedLearning, adaptation *RuntimeAdaptation) {
	if learning.AdaptationStats == nil {
		learning.AdaptationStats = make(map[RuntimeAdaptationType]*AdaptationHistoricalStats)
	}

	stats, ok := learning.AdaptationStats[adaptation.Type]
	if !ok {
		stats = &AdaptationHistoricalStats{
			Type:           adaptation.Type,
			BestConditions: make([]string, 0),
		}
		learning.AdaptationStats[adaptation.Type] = stats
	}

	stats.TotalApplied++
	if adaptation.WasEffective != nil && *adaptation.WasEffective {
		stats.EffectiveCount++
	}
	stats.EffectivenessRate = float64(stats.EffectiveCount) / float64(stats.TotalApplied)
	stats.AvgImpactScore = (stats.AvgImpactScore*float64(stats.TotalApplied-1) + adaptation.ImpactScore) / float64(stats.TotalApplied)
	stats.LastUpdated = time.Now()
}

// RecordPRCorrelation records a correlation between adaptations and PR outcomes
func (p *RuntimePersistence) RecordPRCorrelation(learning *PersistedLearning, correlation *PRAdaptationCorrelation) {
	if learning.PRCorrelations == nil {
		learning.PRCorrelations = make([]*PRAdaptationCorrelation, 0)
	}

	correlation.CreatedAt = time.Now()
	learning.PRCorrelations = append(learning.PRCorrelations, correlation)

	// Trim old correlations (keep last 100)
	if len(learning.PRCorrelations) > 100 {
		learning.PRCorrelations = learning.PRCorrelations[len(learning.PRCorrelations)-100:]
	}
}

// GetBestWorkerConfig returns historically optimal config for a worker
func (p *RuntimePersistence) GetBestWorkerConfig(learning *PersistedLearning, workerType SwarmWorkerType) *WorkerHistoricalData {
	if learning.WorkerHistory == nil {
		return nil
	}
	return learning.WorkerHistory[workerType]
}

// GetEffectiveAdaptationTypes returns adaptation types sorted by effectiveness
func (p *RuntimePersistence) GetEffectiveAdaptationTypes(learning *PersistedLearning) []RuntimeAdaptationType {
	if learning.AdaptationStats == nil {
		return nil
	}

	type ranked struct {
		adaptType RuntimeAdaptationType
		rate      float64
	}

	var rankings []ranked
	for adaptType, stats := range learning.AdaptationStats {
		if stats.TotalApplied >= 5 { // Minimum samples
			rankings = append(rankings, ranked{adaptType, stats.EffectivenessRate})
		}
	}

	// Sort by effectiveness (descending)
	for i := 0; i < len(rankings)-1; i++ {
		for j := i + 1; j < len(rankings); j++ {
			if rankings[j].rate > rankings[i].rate {
				rankings[i], rankings[j] = rankings[j], rankings[i]
			}
		}
	}

	result := make([]RuntimeAdaptationType, len(rankings))
	for i, r := range rankings {
		result[i] = r.adaptType
	}
	return result
}

// CalculateOptimalIntervals learns optimal intervals from history
func (p *RuntimePersistence) CalculateOptimalIntervals(learning *PersistedLearning, volatility float64) *LearnedIntervals {
	intervals := learning.OptimalIntervals
	if intervals == nil {
		intervals = &LearnedIntervals{
			BaseInterval:           5 * time.Minute,
			HighVolatilityInterval: 2 * time.Minute,
			LowVolatilityInterval:  10 * time.Minute,
			AdaptationCooldown:     10 * time.Minute,
			EvaluationInterval:     3,
			VolatilityThreshold:    0.3,
		}
		learning.OptimalIntervals = intervals
	}

	// Adjust based on historical effectiveness
	avgEffectiveness := 0.0
	count := 0
	for _, stats := range learning.AdaptationStats {
		if stats.TotalApplied > 0 {
			avgEffectiveness += stats.EffectivenessRate
			count++
		}
	}
	if count > 0 {
		avgEffectiveness /= float64(count)
	}

	// If adaptations are very effective, we can run less frequently
	// If ineffective, run more frequently to catch issues
	if avgEffectiveness > 0.7 {
		intervals.BaseInterval = 7 * time.Minute
		intervals.AdaptationCooldown = 15 * time.Minute
	} else if avgEffectiveness < 0.3 {
		intervals.BaseInterval = 3 * time.Minute
		intervals.AdaptationCooldown = 5 * time.Minute
	}

	intervals.LastCalibration = time.Now()
	return intervals
}
