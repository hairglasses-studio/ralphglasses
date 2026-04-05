// Package clients provides self-correction tracking for the swarm.
// Based on SAFLA (Self-Adaptive Feedback Loop Architecture) principles.
package clients

import (
	"sync"
	"time"
)

// CorrectionTracker monitors self-improvement and correction patterns
type CorrectionTracker struct {
	mu              sync.RWMutex
	corrections     []*Correction
	improvements    []*CorrectionImprovement
	reflections     []*SelfReflection
	metrics         *CorrectionMetrics
	startTime       time.Time
}

// Correction represents a self-correction event
type Correction struct {
	Timestamp       time.Time `json:"timestamp"`
	CorrectionID    string    `json:"correction_id"`
	OriginalFinding string    `json:"original_finding"`
	CorrectedTo     string    `json:"corrected_to"`
	Reason          string    `json:"reason"`
	Worker          string    `json:"worker"`
	Category        string    `json:"category"`
	ConfidenceBefore float64  `json:"confidence_before"`
	ConfidenceAfter  float64  `json:"confidence_after"`
	Source          string    `json:"source"` // "self", "consensus", "feedback"
	Successful      bool      `json:"successful"`
}

// CorrectionImprovement represents a self-improvement action
type CorrectionImprovement struct {
	Timestamp      time.Time `json:"timestamp"`
	ImprovementID  string    `json:"improvement_id"`
	Type           string    `json:"type"` // "budget", "strategy", "threshold", "pattern"
	Description    string    `json:"description"`
	BeforeValue    string    `json:"before_value"`
	AfterValue     string    `json:"after_value"`
	ImpactScore    float64   `json:"impact_score"`
	Worker         string    `json:"worker,omitempty"`
	Applied        bool      `json:"applied"`
	RolledBack     bool      `json:"rolled_back"`
}

// SelfReflection represents a self-assessment event
type SelfReflection struct {
	Timestamp        time.Time `json:"timestamp"`
	ReflectionID     string    `json:"reflection_id"`
	Subject          string    `json:"subject"` // What was assessed
	Assessment       string    `json:"assessment"`
	QualityScore     float64   `json:"quality_score"`
	ActionsProposed  []string  `json:"actions_proposed"`
	ActionsTaken     []string  `json:"actions_taken"`
	OutcomePositive  bool      `json:"outcome_positive"`
}

// CorrectionMetrics aggregates correction statistics
type CorrectionMetrics struct {
	TotalCorrections     int     `json:"total_corrections"`
	SuccessfulCorrections int    `json:"successful_corrections"`
	CorrectionRate       float64 `json:"correction_rate"`
	TotalImprovements    int     `json:"total_improvements"`
	AppliedImprovements  int     `json:"applied_improvements"`
	RolledBackCount      int     `json:"rolled_back_count"`
	TotalReflections     int     `json:"total_reflections"`
	PositiveOutcomes     int     `json:"positive_outcomes"`
	ReflectionScore      float64 `json:"reflection_score"`
	AdaptationVelocity   float64 `json:"adaptation_velocity"` // Improvements per hour
	RuntimeHours         float64 `json:"runtime_hours"`
}

// CorrectionReport provides detailed correction analysis
type CorrectionReport struct {
	Timestamp          time.Time              `json:"timestamp"`
	Metrics            *CorrectionMetrics     `json:"metrics"`
	RecentCorrections  []*Correction          `json:"recent_corrections"`
	RecentImprovements []*CorrectionImprovement     `json:"recent_improvements"`
	TopCorrectionTypes map[string]int         `json:"top_correction_types"`
	WorkerCorrectionRate map[string]float64   `json:"worker_correction_rate"`
	Trends             *CorrectionTrends      `json:"trends"`
	Recommendations    []string               `json:"recommendations"`
}

// CorrectionTrends tracks correction patterns over time
type CorrectionTrends struct {
	CorrectionRateHourly  []float64 `json:"correction_rate_hourly"`
	ImprovementRateHourly []float64 `json:"improvement_rate_hourly"`
	TrendDirection        string    `json:"trend_direction"` // "improving", "stable", "declining"
}

// NewCorrectionTracker creates a new correction tracker
func NewCorrectionTracker() *CorrectionTracker {
	return &CorrectionTracker{
		corrections:  make([]*Correction, 0),
		improvements: make([]*CorrectionImprovement, 0),
		reflections:  make([]*SelfReflection, 0),
		metrics:      &CorrectionMetrics{},
		startTime:    time.Now(),
	}
}

// RecordCorrection records a self-correction event
func (t *CorrectionTracker) RecordCorrection(c *Correction) {
	t.mu.Lock()
	defer t.mu.Unlock()

	c.Timestamp = time.Now()
	t.corrections = append(t.corrections, c)
	t.updateMetrics()

	// Keep bounded
	if len(t.corrections) > 10000 {
		t.corrections = t.corrections[5000:]
	}
}

// RecordImprovement records a self-improvement action
func (t *CorrectionTracker) RecordImprovement(i *CorrectionImprovement) {
	t.mu.Lock()
	defer t.mu.Unlock()

	i.Timestamp = time.Now()
	t.improvements = append(t.improvements, i)
	t.updateMetrics()

	// Keep bounded
	if len(t.improvements) > 5000 {
		t.improvements = t.improvements[2500:]
	}
}

// RecordReflection records a self-reflection event
func (t *CorrectionTracker) RecordReflection(r *SelfReflection) {
	t.mu.Lock()
	defer t.mu.Unlock()

	r.Timestamp = time.Now()
	t.reflections = append(t.reflections, r)
	t.updateMetrics()

	// Keep bounded
	if len(t.reflections) > 1000 {
		t.reflections = t.reflections[500:]
	}
}

// MarkImprovementRolledBack marks an improvement as rolled back
func (t *CorrectionTracker) MarkImprovementRolledBack(improvementID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, imp := range t.improvements {
		if imp.ImprovementID == improvementID {
			imp.RolledBack = true
			t.updateMetrics()
			return true
		}
	}
	return false
}

func (t *CorrectionTracker) updateMetrics() {
	m := &CorrectionMetrics{
		RuntimeHours: time.Since(t.startTime).Hours(),
	}

	// Correction stats
	m.TotalCorrections = len(t.corrections)
	for _, c := range t.corrections {
		if c.Successful {
			m.SuccessfulCorrections++
		}
	}
	if m.TotalCorrections > 0 {
		m.CorrectionRate = float64(m.SuccessfulCorrections) / float64(m.TotalCorrections) * 100
	}

	// Improvement stats
	m.TotalImprovements = len(t.improvements)
	for _, i := range t.improvements {
		if i.Applied {
			m.AppliedImprovements++
		}
		if i.RolledBack {
			m.RolledBackCount++
		}
	}

	// Adaptation velocity
	if m.RuntimeHours > 0 {
		m.AdaptationVelocity = float64(m.AppliedImprovements) / m.RuntimeHours
	}

	// Reflection stats
	m.TotalReflections = len(t.reflections)
	totalScore := 0.0
	for _, r := range t.reflections {
		totalScore += r.QualityScore
		if r.OutcomePositive {
			m.PositiveOutcomes++
		}
	}
	if m.TotalReflections > 0 {
		m.ReflectionScore = totalScore / float64(m.TotalReflections)
	}

	t.metrics = m
}

// GetMetrics returns current correction metrics
func (t *CorrectionTracker) GetMetrics() *CorrectionMetrics {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.metrics
}

// GetReport generates a comprehensive correction report
func (t *CorrectionTracker) GetReport() *CorrectionReport {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Get recent items
	recentCorrections := t.getRecentCorrections(10)
	recentImprovements := t.getRecentImprovements(10)

	// Calculate correction types
	correctionTypes := make(map[string]int)
	for _, c := range t.corrections {
		correctionTypes[c.Category]++
	}

	// Calculate per-worker correction rates
	workerCorrections := make(map[string]int)
	workerTotal := make(map[string]int)
	for _, c := range t.corrections {
		workerTotal[c.Worker]++
		if c.Successful {
			workerCorrections[c.Worker]++
		}
	}
	workerRates := make(map[string]float64)
	for w, total := range workerTotal {
		if total > 0 {
			workerRates[w] = float64(workerCorrections[w]) / float64(total) * 100
		}
	}

	// Calculate trends
	trends := t.calculateTrends()

	// Generate recommendations
	recommendations := t.generateRecommendations()

	return &CorrectionReport{
		Timestamp:            time.Now(),
		Metrics:              t.metrics,
		RecentCorrections:    recentCorrections,
		RecentImprovements:   recentImprovements,
		TopCorrectionTypes:   correctionTypes,
		WorkerCorrectionRate: workerRates,
		Trends:               trends,
		Recommendations:      recommendations,
	}
}

func (t *CorrectionTracker) getRecentCorrections(limit int) []*Correction {
	if len(t.corrections) <= limit {
		return t.corrections
	}
	return t.corrections[len(t.corrections)-limit:]
}

func (t *CorrectionTracker) getRecentImprovements(limit int) []*CorrectionImprovement {
	if len(t.improvements) <= limit {
		return t.improvements
	}
	return t.improvements[len(t.improvements)-limit:]
}

func (t *CorrectionTracker) calculateTrends() *CorrectionTrends {
	trends := &CorrectionTrends{
		CorrectionRateHourly:  make([]float64, 0),
		ImprovementRateHourly: make([]float64, 0),
		TrendDirection:        "stable",
	}

	// Calculate hourly rates for last 24 hours
	now := time.Now()
	for h := 23; h >= 0; h-- {
		hourStart := now.Add(-time.Duration(h+1) * time.Hour)
		hourEnd := now.Add(-time.Duration(h) * time.Hour)

		correctionCount := 0
		successCount := 0
		for _, c := range t.corrections {
			if c.Timestamp.After(hourStart) && c.Timestamp.Before(hourEnd) {
				correctionCount++
				if c.Successful {
					successCount++
				}
			}
		}
		rate := 0.0
		if correctionCount > 0 {
			rate = float64(successCount) / float64(correctionCount) * 100
		}
		trends.CorrectionRateHourly = append(trends.CorrectionRateHourly, rate)

		improvementCount := 0
		for _, i := range t.improvements {
			if i.Timestamp.After(hourStart) && i.Timestamp.Before(hourEnd) && i.Applied {
				improvementCount++
			}
		}
		trends.ImprovementRateHourly = append(trends.ImprovementRateHourly, float64(improvementCount))
	}

	// Determine trend direction
	if len(trends.CorrectionRateHourly) >= 6 {
		recentAvg := avgFloat(trends.CorrectionRateHourly[:6])
		olderAvg := avgFloat(trends.CorrectionRateHourly[6:12])
		if recentAvg > olderAvg*1.1 {
			trends.TrendDirection = "improving"
		} else if recentAvg < olderAvg*0.9 {
			trends.TrendDirection = "declining"
		}
	}

	return trends
}

func avgFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func (t *CorrectionTracker) generateRecommendations() []string {
	recommendations := make([]string, 0)

	if t.metrics.CorrectionRate < 50 {
		recommendations = append(recommendations, "Low correction success rate - review correction criteria")
	}

	if t.metrics.RolledBackCount > t.metrics.AppliedImprovements/4 {
		recommendations = append(recommendations, "High rollback rate - improvements may be too aggressive")
	}

	if t.metrics.AdaptationVelocity < 1 {
		recommendations = append(recommendations, "Low adaptation velocity - consider increasing improvement frequency")
	}

	if t.metrics.ReflectionScore < 50 {
		recommendations = append(recommendations, "Low reflection quality - enhance self-assessment criteria")
	}

	return recommendations
}

// Global singleton
var (
	globalCorrectionTracker   *CorrectionTracker
	globalCorrectionTrackerMu sync.RWMutex
)

// GetCorrectionTracker returns or creates the global correction tracker
func GetCorrectionTracker() *CorrectionTracker {
	globalCorrectionTrackerMu.Lock()
	defer globalCorrectionTrackerMu.Unlock()

	if globalCorrectionTracker == nil {
		globalCorrectionTracker = NewCorrectionTracker()
	}
	return globalCorrectionTracker
}
