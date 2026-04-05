// Package clients provides the swarm learning persistence layer.
package clients

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SwarmLearnedPattern represents a pattern learned from swarm findings
type SwarmLearnedPattern struct {
	Pattern         string    `json:"pattern"`
	Category        string    `json:"category"`
	WorkersSeen     []string  `json:"workers_seen"`
	FirstSeen       time.Time `json:"first_seen"`
	LastSeen        time.Time `json:"last_seen"`
	OccurrenceCount int       `json:"occurrence_count"`
	Trend           string    `json:"trend"` // emerging, stable, declining
	IsValidated     bool      `json:"is_validated"`
}

// PatternOccurrence tracks hourly occurrence data
type PatternOccurrence struct {
	Pattern string `json:"pattern"`
	Hour    int    `json:"hour"`
	Count   int    `json:"count"`
}

// SwarmLearningClient manages pattern learning and persistence
type SwarmLearningClient struct {
	db            *sql.DB
	mu            sync.RWMutex
	patterns      map[string]*SwarmLearnedPattern
	hourlyBuckets map[string]map[int]int
	startTime     time.Time
}

// Global singleton
var (
	globalLearningClient   *SwarmLearningClient
	globalLearningClientMu sync.RWMutex
)

// GetSwarmLearningClient returns or creates the global learning client
func GetSwarmLearningClient() (*SwarmLearningClient, error) {
	globalLearningClientMu.Lock()
	defer globalLearningClientMu.Unlock()

	if globalLearningClient != nil {
		return globalLearningClient, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dbPath := filepath.Join(homeDir, ".webb", "swarm-learning.db")
	client, err := NewSwarmLearningClient(dbPath)
	if err != nil {
		return nil, err
	}

	globalLearningClient = client
	return client, nil
}

// NewSwarmLearningClient creates a new learning client
func NewSwarmLearningClient(dbPath string) (*SwarmLearningClient, error) {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, err
	}

	client := &SwarmLearningClient{
		db:            db,
		patterns:      make(map[string]*SwarmLearnedPattern),
		hourlyBuckets: make(map[string]map[int]int),
		startTime:     time.Now(),
	}

	if err := client.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	// Load existing patterns
	if err := client.loadPatterns(); err != nil {
		db.Close()
		return nil, err
	}

	return client, nil
}

func (c *SwarmLearningClient) initSchema() error {
	_, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS learned_patterns (
			pattern TEXT PRIMARY KEY,
			category TEXT,
			workers_seen TEXT,
			first_seen DATETIME,
			last_seen DATETIME,
			occurrence_count INTEGER,
			trend TEXT,
			is_validated BOOLEAN
		);
		CREATE TABLE IF NOT EXISTS pattern_occurrences (
			pattern TEXT,
			hour INTEGER,
			count INTEGER,
			PRIMARY KEY (pattern, hour)
		);
		CREATE TABLE IF NOT EXISTS false_positive_patterns (
			pattern TEXT PRIMARY KEY,
			reason TEXT,
			learned_at DATETIME,
			rejection_count INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_validated ON learned_patterns(is_validated);
		CREATE INDEX IF NOT EXISTS idx_trend ON learned_patterns(trend);
	`)
	return err
}

func (c *SwarmLearningClient) loadPatterns() error {
	rows, err := c.db.Query(`
		SELECT pattern, category, workers_seen, first_seen, last_seen,
		       occurrence_count, trend, is_validated
		FROM learned_patterns`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var p SwarmLearnedPattern
		var workersJSON string
		err := rows.Scan(&p.Pattern, &p.Category, &workersJSON, &p.FirstSeen,
			&p.LastSeen, &p.OccurrenceCount, &p.Trend, &p.IsValidated)
		if err != nil {
			continue
		}
		json.Unmarshal([]byte(workersJSON), &p.WorkersSeen)
		c.patterns[p.Pattern] = &p
	}

	return nil
}

// RecordFinding records a finding and updates pattern learning
func (c *SwarmLearningClient) RecordFinding(pattern, category, worker string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	p, exists := c.patterns[pattern]
	if !exists {
		p = &SwarmLearnedPattern{
			Pattern:         pattern,
			Category:        category,
			WorkersSeen:     []string{},
			FirstSeen:       time.Now(),
			LastSeen:        time.Now(),
			OccurrenceCount: 0,
			Trend:           "emerging",
			IsValidated:     false,
		}
		c.patterns[pattern] = p
		c.hourlyBuckets[pattern] = make(map[int]int)
	}

	p.LastSeen = time.Now()
	p.OccurrenceCount++

	// Track worker
	if worker != "" && !containsWorker(p.WorkersSeen, worker) {
		p.WorkersSeen = append(p.WorkersSeen, worker)
	}

	// Cross-validated if 2+ workers
	if len(p.WorkersSeen) >= 2 && !p.IsValidated {
		p.IsValidated = true
		fmt.Printf("learning: cross-validated pattern: %s (workers: %v)\n", pattern, p.WorkersSeen)
	}

	// Track hourly occurrence
	hour := int(time.Since(c.startTime).Hours())
	if c.hourlyBuckets[pattern] == nil {
		c.hourlyBuckets[pattern] = make(map[int]int)
	}
	c.hourlyBuckets[pattern][hour]++

	// Update trend
	c.updateTrend(p)

	return nil
}

func (c *SwarmLearningClient) updateTrend(p *SwarmLearnedPattern) {
	buckets := c.hourlyBuckets[p.Pattern]
	if len(buckets) < 2 {
		p.Trend = "emerging"
		return
	}

	currentHour := int(time.Since(c.startTime).Hours())
	recent := buckets[currentHour]

	var historicalSum, historicalCount int
	for h, count := range buckets {
		if h < currentHour {
			historicalSum += count
			historicalCount++
		}
	}

	if historicalCount == 0 {
		p.Trend = "emerging"
		return
	}

	historical := float64(historicalSum) / float64(historicalCount)
	if float64(recent) > historical*1.5 {
		p.Trend = "emerging"
	} else if float64(recent) < historical*0.5 {
		p.Trend = "declining"
	} else {
		p.Trend = "stable"
	}
}

// RecordFalsePositive records a false positive pattern for future filtering
func (c *SwarmLearningClient) RecordFalsePositive(pattern, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, err := c.db.Exec(`
		INSERT INTO false_positive_patterns (pattern, reason, learned_at, rejection_count)
		VALUES (?, ?, ?, 1)
		ON CONFLICT(pattern) DO UPDATE SET rejection_count = rejection_count + 1`,
		pattern, reason, time.Now())
	return err
}

// IsFalsePositive checks if a pattern is a known false positive
func (c *SwarmLearningClient) IsFalsePositive(pattern string) (bool, error) {
	var count int
	err := c.db.QueryRow(`
		SELECT rejection_count FROM false_positive_patterns WHERE pattern = ?`,
		pattern).Scan(&count)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return count >= 3, nil // FP if rejected 3+ times
}

// GetValidatedPatterns returns patterns confirmed by 2+ workers
func (c *SwarmLearningClient) GetValidatedPatterns() ([]*SwarmLearnedPattern, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var validated []*SwarmLearnedPattern
	for _, p := range c.patterns {
		if p.IsValidated {
			validated = append(validated, p)
		}
	}
	return validated, nil
}

// GetEmergingPatterns returns patterns with increasing frequency
func (c *SwarmLearningClient) GetEmergingPatterns() ([]*SwarmLearnedPattern, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var emerging []*SwarmLearnedPattern
	for _, p := range c.patterns {
		if p.Trend == "emerging" {
			emerging = append(emerging, p)
		}
	}
	return emerging, nil
}

// GetLearningStats returns current learning statistics
func (c *SwarmLearningClient) GetLearningStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var validated, emerging, declining, stable int
	for _, p := range c.patterns {
		if p.IsValidated {
			validated++
		}
		switch p.Trend {
		case "emerging":
			emerging++
		case "declining":
			declining++
		case "stable":
			stable++
		}
	}

	var fpCount int
	c.db.QueryRow(`SELECT COUNT(*) FROM false_positive_patterns WHERE rejection_count >= 3`).Scan(&fpCount)

	return map[string]interface{}{
		"total_patterns":         len(c.patterns),
		"cross_validated":        validated,
		"emerging_patterns":      emerging,
		"stable_patterns":        stable,
		"declining_patterns":     declining,
		"learned_false_positives": fpCount,
		"runtime_hours":          time.Since(c.startTime).Hours(),
	}
}

// Persist saves all patterns to the database
func (c *SwarmLearningClient) Persist() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for pattern, p := range c.patterns {
		workersJSON, _ := json.Marshal(p.WorkersSeen)
		_, err := c.db.Exec(`
			INSERT OR REPLACE INTO learned_patterns
			(pattern, category, workers_seen, first_seen, last_seen, occurrence_count, trend, is_validated)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			pattern, p.Category, string(workersJSON), p.FirstSeen, p.LastSeen,
			p.OccurrenceCount, p.Trend, p.IsValidated)
		if err != nil {
			return err
		}

		// Persist hourly buckets
		for hour, count := range c.hourlyBuckets[pattern] {
			_, err = c.db.Exec(`
				INSERT OR REPLACE INTO pattern_occurrences (pattern, hour, count)
				VALUES (?, ?, ?)`, pattern, hour, count)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Close closes the database connection
func (c *SwarmLearningClient) Close() error {
	c.Persist()
	return c.db.Close()
}

// LearningStats contains structured learning statistics for metrics
type LearningStats struct {
	TotalPatterns    int
	CrossValidated   int
	Emerging         int
	Stable           int
	Declining        int
	FalsePositives   int
	ConfidenceBoosts int
	RuntimeHours     float64
}

// GetStats returns structured learning statistics for Grafana metrics
func (c *SwarmLearningClient) GetStats() *LearningStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := &LearningStats{
		TotalPatterns: len(c.patterns),
		RuntimeHours:  time.Since(c.startTime).Hours(),
	}

	for _, p := range c.patterns {
		if p.IsValidated {
			stats.CrossValidated++
		}
		switch p.Trend {
		case "emerging":
			stats.Emerging++
		case "declining":
			stats.Declining++
		case "stable":
			stats.Stable++
		}
	}

	// Count false positives
	c.db.QueryRow(`SELECT COUNT(*) FROM false_positive_patterns WHERE rejection_count >= 3`).Scan(&stats.FalsePositives)

	// Count confidence boosts (cross-validated patterns used for boosting)
	stats.ConfidenceBoosts = stats.CrossValidated

	return stats
}

// Helper function for worker deduplication
func containsWorker(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// =============================================================================
// DRIFT DETECTION INTEGRATION (v36.0)
// =============================================================================

// GetPatternDistribution returns pattern distribution for drift detection
func (c *SwarmLearningClient) GetPatternDistribution() map[string]*SwarmLearnedPattern {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy to avoid data races
	result := make(map[string]*SwarmLearnedPattern, len(c.patterns))
	for k, v := range c.patterns {
		patternCopy := *v
		result[k] = &patternCopy
	}
	return result
}

// UpdateDriftDetector syncs current patterns to the drift detector
func (c *SwarmLearningClient) UpdateDriftDetector() {
	patterns := c.GetPatternDistribution()
	detector := GetDriftDetector()
	detector.UpdateCurrent(patterns)
}

// GetConvergenceMetrics returns metrics about learning convergence
func (c *SwarmLearningClient) GetConvergenceMetrics() *ConvergenceMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metrics := &ConvergenceMetrics{
		Timestamp:     time.Now(),
		RuntimeHours:  time.Since(c.startTime).Hours(),
		TotalPatterns: len(c.patterns),
	}

	// Count by trend
	for _, p := range c.patterns {
		switch p.Trend {
		case "emerging":
			metrics.EmergingCount++
		case "stable":
			metrics.StableCount++
		case "declining":
			metrics.DecliningCount++
		}
		if p.IsValidated {
			metrics.ValidatedCount++
		}
	}

	// Calculate convergence rate (ratio of stable patterns)
	if metrics.TotalPatterns > 0 {
		metrics.ConvergenceRate = float64(metrics.StableCount) / float64(metrics.TotalPatterns) * 100
	}

	// Calculate velocity (patterns per hour)
	if metrics.RuntimeHours > 0 {
		metrics.DiscoveryVelocity = float64(metrics.TotalPatterns) / metrics.RuntimeHours
	}

	// Determine convergence status
	if metrics.ConvergenceRate >= 70 {
		metrics.Status = "converged"
	} else if metrics.ConvergenceRate >= 40 {
		metrics.Status = "stabilizing"
	} else {
		metrics.Status = "learning"
	}

	return metrics
}

// ConvergenceMetrics tracks learning convergence state
type ConvergenceMetrics struct {
	Timestamp         time.Time `json:"timestamp"`
	RuntimeHours      float64   `json:"runtime_hours"`
	TotalPatterns     int       `json:"total_patterns"`
	EmergingCount     int       `json:"emerging_count"`
	StableCount       int       `json:"stable_count"`
	DecliningCount    int       `json:"declining_count"`
	ValidatedCount    int       `json:"validated_count"`
	ConvergenceRate   float64   `json:"convergence_rate"`   // % stable
	DiscoveryVelocity float64   `json:"discovery_velocity"` // patterns/hour
	Status            string    `json:"status"`             // "learning", "stabilizing", "converged"
}
