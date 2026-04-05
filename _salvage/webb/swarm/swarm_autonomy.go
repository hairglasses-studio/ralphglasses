// Package clients provides the swarm autonomy scoring system.
// Tracks human interventions and calculates an autonomy score (0-100).
// Higher score = more autonomous operation, lower score = more manual intervention needed.
package clients

import (
	"sync"
	"time"
)

// InterventionType categorizes types of human interventions
type InterventionType string

const (
	// High-impact interventions (major score impact)
	InterventionManualRestart     InterventionType = "manual_restart"      // Manually restarting swarm
	InterventionManualErrorFix    InterventionType = "manual_error_fix"    // Fixing errors manually
	InterventionManualConfig      InterventionType = "manual_config"       // Manual configuration changes
	InterventionManualRecovery    InterventionType = "manual_recovery"     // Manual recovery from failure

	// Medium-impact interventions
	InterventionManualTriage      InterventionType = "manual_triage"       // Manually triaging findings
	InterventionManualApproval    InterventionType = "manual_approval"     // Manually approving/rejecting
	InterventionBudgetAdjustment  InterventionType = "budget_adjustment"   // Manual budget changes
	InterventionWorkerAdjustment  InterventionType = "worker_adjustment"   // Manual worker config

	// Low-impact interventions
	InterventionPatternTraining   InterventionType = "pattern_training"    // Manual pattern additions
	InterventionQueryRefinement   InterventionType = "query_refinement"    // Refining research queries
	InterventionMonitorCheck      InterventionType = "monitor_check"       // Manual status checks
)

// InterventionWeights defines score impact per intervention type
var InterventionWeights = map[InterventionType]float64{
	InterventionManualRestart:    -15.0, // Major impact
	InterventionManualErrorFix:   -12.0,
	InterventionManualConfig:     -10.0,
	InterventionManualRecovery:   -10.0,
	InterventionManualTriage:     -5.0,  // Medium impact
	InterventionManualApproval:   -4.0,
	InterventionBudgetAdjustment: -3.0,
	InterventionWorkerAdjustment: -3.0,
	InterventionPatternTraining:  -1.0,  // Low impact
	InterventionQueryRefinement:  -1.0,
	InterventionMonitorCheck:     -0.5,
}

// AutonomyBonus defines bonuses for autonomous operations
var AutonomyBonuses = map[string]float64{
	"self_correction":     +2.0, // Self-corrected an error
	"auto_recovery":       +3.0, // Automatically recovered from failure
	"pattern_learned":     +1.0, // Learned a new pattern without help
	"finding_auto_triage": +0.5, // Auto-triaged a finding
	"budget_auto_adjust":  +1.0, // Auto-adjusted budget based on feedback
	"continuous_hour":     +0.2, // Each hour of uninterrupted operation
}

// Intervention records a single human intervention
type Intervention struct {
	ID          string           `json:"id"`
	Type        InterventionType `json:"type"`
	Timestamp   time.Time        `json:"timestamp"`
	Description string           `json:"description"`
	Impact      float64          `json:"impact"` // Score impact
	Source      string           `json:"source"` // CLI, API, MCP, etc.
}

// AutonomyEvent records an autonomous operation event
type AutonomyEvent struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Details   string    `json:"details"`
	Bonus     float64   `json:"bonus"`
}

// SwarmAutonomyScore captures the current autonomy assessment
type SwarmAutonomyScore struct {
	// Overall score (0-100)
	Score           float64 `json:"score"`
	ScoreLabel      string  `json:"score_label"`      // "Excellent", "Good", etc.

	// Trend tracking
	Score24h        float64 `json:"score_24h"`
	Score7d         float64 `json:"score_7d"`
	Score30d        float64 `json:"score_30d"`
	TrendDirection  string  `json:"trend_direction"`  // "improving", "stable", "degrading"

	// Intervention breakdown
	TotalInterventions    int                       `json:"total_interventions"`
	InterventionsByType   map[InterventionType]int  `json:"interventions_by_type"`
	InterventionImpact    float64                   `json:"intervention_impact"`

	// Autonomy breakdown
	TotalAutonomyEvents   int                       `json:"total_autonomy_events"`
	AutonomyEventsByType  map[string]int            `json:"autonomy_events_by_type"`
	AutonomyBonus         float64                   `json:"autonomy_bonus"`

	// Operational metrics
	UptimeHours           float64 `json:"uptime_hours"`
	LastIntervention      time.Time `json:"last_intervention"`
	HoursSinceIntervention float64  `json:"hours_since_intervention"`

	// Recommendations
	Recommendations []string `json:"recommendations"`
}

// SwarmAutonomyTracker tracks interventions and calculates autonomy scores
type SwarmAutonomyTracker struct {
	mu              sync.RWMutex
	interventions   []*Intervention
	autonomyEvents  []*AutonomyEvent
	startTime       time.Time
	config          *AutonomyTrackerConfig
}

// AutonomyTrackerConfig configures the autonomy tracker
type AutonomyTrackerConfig struct {
	BaseScore        float64       // Starting score (default: 100)
	WindowDuration   time.Duration // Rolling window (default: 24h)
	MaxInterventions int           // Buffer size (default: 10000)
}

// NewSwarmAutonomyTracker creates a new autonomy tracker
func NewSwarmAutonomyTracker(config *AutonomyTrackerConfig) *SwarmAutonomyTracker {
	if config == nil {
		config = &AutonomyTrackerConfig{
			BaseScore:        100.0,
			WindowDuration:   24 * time.Hour,
			MaxInterventions: 10000,
		}
	}
	return &SwarmAutonomyTracker{
		interventions:  make([]*Intervention, 0),
		autonomyEvents: make([]*AutonomyEvent, 0),
		startTime:      time.Now(),
		config:         config,
	}
}

// RecordIntervention records a human intervention
func (t *SwarmAutonomyTracker) RecordIntervention(iType InterventionType, description, source string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	impact := InterventionWeights[iType]
	if impact == 0 {
		impact = -5.0 // Default penalty for unknown types
	}

	intervention := &Intervention{
		ID:          generateAutonomyID("int"),
		Type:        iType,
		Timestamp:   time.Now(),
		Description: description,
		Impact:      impact,
		Source:      source,
	}

	t.interventions = append(t.interventions, intervention)

	// Bound the buffer
	if len(t.interventions) > t.config.MaxInterventions {
		t.interventions = t.interventions[len(t.interventions)-t.config.MaxInterventions:]
	}
}

// RecordAutonomyEvent records an autonomous operation
func (t *SwarmAutonomyTracker) RecordAutonomyEvent(eventType, details string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	bonus := AutonomyBonuses[eventType]
	if bonus == 0 {
		bonus = 0.5 // Default bonus for unknown events
	}

	event := &AutonomyEvent{
		ID:        generateAutonomyID("aut"),
		Type:      eventType,
		Timestamp: time.Now(),
		Details:   details,
		Bonus:     bonus,
	}

	t.autonomyEvents = append(t.autonomyEvents, event)

	// Bound the buffer
	if len(t.autonomyEvents) > t.config.MaxInterventions {
		t.autonomyEvents = t.autonomyEvents[len(t.autonomyEvents)-t.config.MaxInterventions:]
	}
}

// GetAutonomyScore calculates the current autonomy score
func (t *SwarmAutonomyTracker) GetAutonomyScore() *SwarmAutonomyScore {
	t.mu.RLock()
	defer t.mu.RUnlock()

	now := time.Now()
	score := &SwarmAutonomyScore{
		Score:                t.config.BaseScore,
		InterventionsByType:  make(map[InterventionType]int),
		AutonomyEventsByType: make(map[string]int),
		UptimeHours:          now.Sub(t.startTime).Hours(),
		Recommendations:      make([]string, 0),
	}

	// Calculate intervention impact (24h window)
	cutoff24h := now.Add(-24 * time.Hour)
	cutoff7d := now.Add(-7 * 24 * time.Hour)
	cutoff30d := now.Add(-30 * 24 * time.Hour)

	impact24h := 0.0
	impact7d := 0.0
	impact30d := 0.0
	var lastIntervention time.Time

	for _, intervention := range t.interventions {
		if intervention.Timestamp.After(cutoff24h) {
			score.TotalInterventions++
			score.InterventionsByType[intervention.Type]++
			impact24h += intervention.Impact
			if intervention.Timestamp.After(lastIntervention) {
				lastIntervention = intervention.Timestamp
			}
		}
		if intervention.Timestamp.After(cutoff7d) {
			impact7d += intervention.Impact
		}
		if intervention.Timestamp.After(cutoff30d) {
			impact30d += intervention.Impact
		}
	}

	score.InterventionImpact = impact24h
	score.LastIntervention = lastIntervention
	if !lastIntervention.IsZero() {
		score.HoursSinceIntervention = now.Sub(lastIntervention).Hours()
	}

	// Calculate autonomy bonuses (24h window)
	bonus24h := 0.0
	bonus7d := 0.0
	bonus30d := 0.0

	for _, event := range t.autonomyEvents {
		if event.Timestamp.After(cutoff24h) {
			score.TotalAutonomyEvents++
			score.AutonomyEventsByType[event.Type]++
			bonus24h += event.Bonus
		}
		if event.Timestamp.After(cutoff7d) {
			bonus7d += event.Bonus
		}
		if event.Timestamp.After(cutoff30d) {
			bonus30d += event.Bonus
		}
	}

	// Add continuous operation bonus
	continuousHours := score.HoursSinceIntervention
	if continuousHours > 24 {
		continuousHours = 24 // Cap at 24h bonus
	}
	bonus24h += continuousHours * AutonomyBonuses["continuous_hour"]

	score.AutonomyBonus = bonus24h

	// Calculate final scores
	score.Score = t.config.BaseScore + impact24h + bonus24h
	score.Score = clampAutonomy(score.Score, 0, 100)

	score.Score24h = score.Score
	score.Score7d = clampAutonomy(t.config.BaseScore+impact7d/7+bonus7d/7, 0, 100)
	score.Score30d = clampAutonomy(t.config.BaseScore+impact30d/30+bonus30d/30, 0, 100)

	// Determine trend
	if score.Score7d > score.Score30d+5 {
		score.TrendDirection = "improving"
	} else if score.Score7d < score.Score30d-5 {
		score.TrendDirection = "degrading"
	} else {
		score.TrendDirection = "stable"
	}

	// Set label
	score.ScoreLabel = getScoreLabel(score.Score)

	// Generate recommendations
	score.Recommendations = t.generateRecommendations(score)

	return score
}

func (t *SwarmAutonomyTracker) generateRecommendations(score *SwarmAutonomyScore) []string {
	recs := make([]string, 0)

	// Check for high-impact intervention patterns
	if score.InterventionsByType[InterventionManualRestart] > 2 {
		recs = append(recs, "High manual restart frequency - improve error handling and recovery")
	}
	if score.InterventionsByType[InterventionManualErrorFix] > 3 {
		recs = append(recs, "Frequent manual error fixes - add more self-correction logic")
	}
	if score.InterventionsByType[InterventionManualTriage] > 10 {
		recs = append(recs, "Heavy manual triage - tune auto-triage thresholds")
	}
	if score.InterventionsByType[InterventionBudgetAdjustment] > 5 {
		recs = append(recs, "Many budget adjustments - implement adaptive budgeting")
	}

	// Positive recommendations
	if score.Score >= 90 {
		recs = append(recs, "Excellent autonomy - consider expanding swarm scope")
	} else if score.Score >= 70 {
		recs = append(recs, "Good autonomy - focus on reducing remaining intervention types")
	} else if score.Score >= 50 {
		recs = append(recs, "Moderate autonomy - prioritize self-correction improvements")
	} else {
		recs = append(recs, "Low autonomy - review error handling and recovery mechanisms")
	}

	// Trend-based recommendations
	if score.TrendDirection == "improving" {
		recs = append(recs, "Positive trend - continue current optimization approach")
	} else if score.TrendDirection == "degrading" {
		recs = append(recs, "Declining autonomy - investigate recent changes")
	}

	return recs
}

func getScoreLabel(score float64) string {
	switch {
	case score >= 95:
		return "Excellent"
	case score >= 85:
		return "Very Good"
	case score >= 70:
		return "Good"
	case score >= 50:
		return "Fair"
	case score >= 30:
		return "Needs Improvement"
	default:
		return "Critical"
	}
}

func clampAutonomy(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func generateAutonomyID(prefix string) string {
	return prefix + "-" + time.Now().Format("20060102-150405.000")
}

// GetInterventionStats returns intervention statistics
func (t *SwarmAutonomyTracker) GetInterventionStats() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total"] = len(t.interventions)

	byType := make(map[InterventionType]int)
	for _, i := range t.interventions {
		byType[i.Type]++
	}
	stats["by_type"] = byType

	return stats
}

// GetRecentInterventions returns recent interventions
func (t *SwarmAutonomyTracker) GetRecentInterventions(limit int) []*Intervention {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if limit <= 0 || limit > len(t.interventions) {
		limit = len(t.interventions)
	}

	result := make([]*Intervention, limit)
	copy(result, t.interventions[len(t.interventions)-limit:])
	return result
}

// Global singleton
var (
	globalAutonomyTracker   *SwarmAutonomyTracker
	globalAutonomyTrackerMu sync.RWMutex
)

// GetSwarmAutonomyTracker returns the global autonomy tracker
func GetSwarmAutonomyTracker() *SwarmAutonomyTracker {
	globalAutonomyTrackerMu.Lock()
	defer globalAutonomyTrackerMu.Unlock()

	if globalAutonomyTracker == nil {
		globalAutonomyTracker = NewSwarmAutonomyTracker(nil)
	}
	return globalAutonomyTracker
}
