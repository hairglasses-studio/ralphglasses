// Package clients provides drift detection for swarm patterns.
// Implements PSI (Population Stability Index) and CSI (Characteristic Stability Index).
package clients

import (
	"math"
	"sync"
	"time"
)

// DriftDetector monitors pattern distribution drift over time
type DriftDetector struct {
	mu              sync.RWMutex
	baselineStats   map[string]*DriftCategoryStats
	currentStats    map[string]*DriftCategoryStats
	baselineTime    time.Time
	lastUpdate      time.Time
	thresholds      DriftThresholds
	history         []*DriftSnapshot
	alerts          []*DriftAlert
}

// DriftCategoryStats tracks distribution statistics for a category
type DriftCategoryStats struct {
	Category       string    `json:"category"`
	Count          int       `json:"count"`
	Percentage     float64   `json:"percentage"`
	AvgConfidence  float64   `json:"avg_confidence"`
	Workers        []string  `json:"workers"`
	LastUpdated    time.Time `json:"last_updated"`
}

// DriftThresholds configures alerting thresholds
type DriftThresholds struct {
	PSIWarning    float64 `json:"psi_warning"`    // Default 0.1
	PSICritical   float64 `json:"psi_critical"`   // Default 0.25
	CSIWarning    float64 `json:"csi_warning"`    // Default 0.1
	CSICritical   float64 `json:"csi_critical"`   // Default 0.25
	StalenessHours float64 `json:"staleness_hours"` // Default 24
}

// DriftSnapshot captures drift metrics at a point in time
type DriftSnapshot struct {
	Timestamp      time.Time          `json:"timestamp"`
	PSI            float64            `json:"psi"`
	CSIByCategory  map[string]float64 `json:"csi_by_category"`
	DriftDetected  bool               `json:"drift_detected"`
	DriftCategories []string          `json:"drift_categories"`
	StalenessScore float64            `json:"staleness_score"`
}

// DriftAlert represents a drift alert
type DriftAlert struct {
	Timestamp    time.Time `json:"timestamp"`
	Type         string    `json:"type"`     // "psi", "csi", "staleness"
	Severity     string    `json:"severity"` // "warning", "critical"
	Category     string    `json:"category,omitempty"`
	Value        float64   `json:"value"`
	Threshold    float64   `json:"threshold"`
	Message      string    `json:"message"`
	Acknowledged bool      `json:"acknowledged"`
}

// SwarmDriftReport contains comprehensive drift analysis
type SwarmDriftReport struct {
	Timestamp        time.Time          `json:"timestamp"`
	BaselineAge      time.Duration      `json:"baseline_age"`
	PSI              float64            `json:"psi"`
	PSIStatus        string             `json:"psi_status"` // "healthy", "warning", "critical"
	CSIByCategory    map[string]float64 `json:"csi_by_category"`
	DriftDetected    bool               `json:"drift_detected"`
	DriftCategories  []string           `json:"drift_categories"`
	StalenessScore   float64            `json:"staleness_score"`
	StaleCategories  []string           `json:"stale_categories"`
	ActiveAlerts     []*DriftAlert      `json:"active_alerts"`
	Recommendations  []string           `json:"recommendations"`
}

// NewDriftDetector creates a new drift detector
func NewDriftDetector() *DriftDetector {
	return &DriftDetector{
		baselineStats: make(map[string]*DriftCategoryStats),
		currentStats:  make(map[string]*DriftCategoryStats),
		baselineTime:  time.Now(),
		thresholds: DriftThresholds{
			PSIWarning:     0.1,
			PSICritical:    0.25,
			CSIWarning:     0.1,
			CSICritical:    0.25,
			StalenessHours: 24,
		},
		history: make([]*DriftSnapshot, 0),
		alerts:  make([]*DriftAlert, 0),
	}
}

// SetBaseline sets the baseline distribution from current patterns
func (d *DriftDetector) SetBaseline(patterns map[string]*SwarmLearnedPattern) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.baselineStats = make(map[string]*DriftCategoryStats)
	d.baselineTime = time.Now()

	// Calculate category distribution
	categoryCount := make(map[string]int)
	categoryConfidence := make(map[string][]float64)
	categoryWorkers := make(map[string]map[string]bool)
	total := 0

	for _, p := range patterns {
		categoryCount[p.Category] += p.OccurrenceCount
		total += p.OccurrenceCount

		if categoryWorkers[p.Category] == nil {
			categoryWorkers[p.Category] = make(map[string]bool)
		}
		for _, w := range p.WorkersSeen {
			categoryWorkers[p.Category][w] = true
		}
	}

	for cat, count := range categoryCount {
		workers := make([]string, 0)
		for w := range categoryWorkers[cat] {
			workers = append(workers, w)
		}

		avgConf := 0.0
		if len(categoryConfidence[cat]) > 0 {
			sum := 0.0
			for _, c := range categoryConfidence[cat] {
				sum += c
			}
			avgConf = sum / float64(len(categoryConfidence[cat]))
		}

		d.baselineStats[cat] = &DriftCategoryStats{
			Category:      cat,
			Count:         count,
			Percentage:    float64(count) / float64(total) * 100,
			AvgConfidence: avgConf,
			Workers:       workers,
			LastUpdated:   time.Now(),
		}
	}
}

// UpdateCurrent updates current distribution from new patterns
func (d *DriftDetector) UpdateCurrent(patterns map[string]*SwarmLearnedPattern) {
	d.mu.Lock()
	// Note: unlock is done manually before recordSnapshot to avoid deadlock

	d.currentStats = make(map[string]*DriftCategoryStats)
	d.lastUpdate = time.Now()

	categoryCount := make(map[string]int)
	categoryWorkers := make(map[string]map[string]bool)
	total := 0

	for _, p := range patterns {
		categoryCount[p.Category] += p.OccurrenceCount
		total += p.OccurrenceCount

		if categoryWorkers[p.Category] == nil {
			categoryWorkers[p.Category] = make(map[string]bool)
		}
		for _, w := range p.WorkersSeen {
			categoryWorkers[p.Category][w] = true
		}
	}

	for cat, count := range categoryCount {
		workers := make([]string, 0)
		for w := range categoryWorkers[cat] {
			workers = append(workers, w)
		}

		d.currentStats[cat] = &DriftCategoryStats{
			Category:    cat,
			Count:       count,
			Percentage:  float64(count) / float64(total) * 100,
			Workers:     workers,
			LastUpdated: time.Now(),
		}
	}
	d.mu.Unlock()

	// Record snapshot and check for drift (outside lock to avoid deadlock)
	d.recordSnapshot()
}

// CalculatePSI computes Population Stability Index
// PSI = Σ (Actual% - Expected%) * ln(Actual% / Expected%)
// PSI < 0.1: No significant drift
// PSI 0.1-0.25: Some drift, monitor
// PSI > 0.25: Significant drift, action needed
func (d *DriftDetector) CalculatePSI() float64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.baselineStats) == 0 || len(d.currentStats) == 0 {
		return 0
	}

	psi := 0.0
	allCategories := make(map[string]bool)
	for cat := range d.baselineStats {
		allCategories[cat] = true
	}
	for cat := range d.currentStats {
		allCategories[cat] = true
	}

	for cat := range allCategories {
		expected := 0.0001 // Small value to avoid division by zero
		actual := 0.0001

		if bs, ok := d.baselineStats[cat]; ok {
			expected = bs.Percentage / 100
		}
		if cs, ok := d.currentStats[cat]; ok {
			actual = cs.Percentage / 100
		}

		// Ensure minimum values
		if expected < 0.0001 {
			expected = 0.0001
		}
		if actual < 0.0001 {
			actual = 0.0001
		}

		// PSI contribution for this category
		psi += (actual - expected) * math.Log(actual/expected)
	}

	return math.Abs(psi)
}

// CalculateCSI computes Characteristic Stability Index for a category
func (d *DriftDetector) CalculateCSI(category string) float64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	bs, hasBaseline := d.baselineStats[category]
	cs, hasCurrent := d.currentStats[category]

	if !hasBaseline || !hasCurrent {
		return 0
	}

	expected := bs.Percentage / 100
	actual := cs.Percentage / 100

	if expected < 0.0001 {
		expected = 0.0001
	}
	if actual < 0.0001 {
		actual = 0.0001
	}

	return math.Abs((actual - expected) * math.Log(actual/expected))
}

// CalculateAllCSI returns CSI for all categories
func (d *DriftDetector) CalculateAllCSI() map[string]float64 {
	d.mu.RLock()
	allCategories := make(map[string]bool)
	for cat := range d.baselineStats {
		allCategories[cat] = true
	}
	for cat := range d.currentStats {
		allCategories[cat] = true
	}
	d.mu.RUnlock()

	result := make(map[string]float64)
	for cat := range allCategories {
		result[cat] = d.CalculateCSI(cat)
	}
	return result
}

// CalculateStaleness returns staleness score (0=fresh, 100=stale)
func (d *DriftDetector) CalculateStaleness() float64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.lastUpdate.IsZero() {
		return 100
	}

	hoursSinceUpdate := time.Since(d.lastUpdate).Hours()
	staleness := (hoursSinceUpdate / d.thresholds.StalenessHours) * 100

	return math.Min(staleness, 100)
}

// GetStaleCategories returns categories that haven't been updated recently
func (d *DriftDetector) GetStaleCategories() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stale := make([]string, 0)
	threshold := time.Duration(d.thresholds.StalenessHours) * time.Hour

	for cat, stats := range d.currentStats {
		if time.Since(stats.LastUpdated) > threshold {
			stale = append(stale, cat)
		}
	}
	return stale
}

// DetectDrift checks for significant drift and returns affected categories
func (d *DriftDetector) DetectDrift() (bool, []string) {
	psi := d.CalculatePSI()
	csiMap := d.CalculateAllCSI()

	driftDetected := psi > d.thresholds.PSIWarning
	driftCategories := make([]string, 0)

	for cat, csi := range csiMap {
		if csi > d.thresholds.CSIWarning {
			driftCategories = append(driftCategories, cat)
			driftDetected = true
		}
	}

	// Generate alerts if needed
	d.checkAndGenerateAlerts(psi, csiMap)

	return driftDetected, driftCategories
}

func (d *DriftDetector) recordSnapshot() {
	psi := d.CalculatePSI()
	csiMap := d.CalculateAllCSI()
	driftDetected, driftCategories := d.DetectDrift()

	snapshot := &DriftSnapshot{
		Timestamp:       time.Now(),
		PSI:             psi,
		CSIByCategory:   csiMap,
		DriftDetected:   driftDetected,
		DriftCategories: driftCategories,
		StalenessScore:  d.CalculateStaleness(),
	}

	d.history = append(d.history, snapshot)

	// Keep history bounded
	if len(d.history) > 1000 {
		d.history = d.history[500:]
	}
}

func (d *DriftDetector) checkAndGenerateAlerts(psi float64, csiMap map[string]float64) {
	now := time.Now()

	// PSI alerts
	if psi > d.thresholds.PSICritical {
		d.alerts = append(d.alerts, &DriftAlert{
			Timestamp: now,
			Type:      "psi",
			Severity:  "critical",
			Value:     psi,
			Threshold: d.thresholds.PSICritical,
			Message:   "Critical: Significant population drift detected",
		})
	} else if psi > d.thresholds.PSIWarning {
		d.alerts = append(d.alerts, &DriftAlert{
			Timestamp: now,
			Type:      "psi",
			Severity:  "warning",
			Value:     psi,
			Threshold: d.thresholds.PSIWarning,
			Message:   "Warning: Moderate population drift detected",
		})
	}

	// CSI alerts per category
	for cat, csi := range csiMap {
		if csi > d.thresholds.CSICritical {
			d.alerts = append(d.alerts, &DriftAlert{
				Timestamp: now,
				Type:      "csi",
				Severity:  "critical",
				Category:  cat,
				Value:     csi,
				Threshold: d.thresholds.CSICritical,
				Message:   "Critical: Significant drift in " + cat,
			})
		} else if csi > d.thresholds.CSIWarning {
			d.alerts = append(d.alerts, &DriftAlert{
				Timestamp: now,
				Type:      "csi",
				Severity:  "warning",
				Category:  cat,
				Value:     csi,
				Threshold: d.thresholds.CSIWarning,
				Message:   "Warning: Moderate drift in " + cat,
			})
		}
	}

	// Keep alerts bounded
	if len(d.alerts) > 100 {
		d.alerts = d.alerts[50:]
	}
}

// GetReport generates a comprehensive drift report
func (d *DriftDetector) GetReport() *SwarmDriftReport {
	d.mu.RLock()
	defer d.mu.RUnlock()

	psi := d.CalculatePSI()
	csiMap := d.CalculateAllCSI()
	driftDetected, driftCategories := d.DetectDrift()
	staleCategories := d.GetStaleCategories()

	psiStatus := "healthy"
	if psi > d.thresholds.PSICritical {
		psiStatus = "critical"
	} else if psi > d.thresholds.PSIWarning {
		psiStatus = "warning"
	}

	// Filter active (unacknowledged) alerts
	activeAlerts := make([]*DriftAlert, 0)
	for _, alert := range d.alerts {
		if !alert.Acknowledged {
			activeAlerts = append(activeAlerts, alert)
		}
	}

	recommendations := make([]string, 0)
	if psi > d.thresholds.PSIWarning {
		recommendations = append(recommendations, "Consider resetting baseline after reviewing drift causes")
	}
	if len(staleCategories) > 0 {
		recommendations = append(recommendations, "Update stale categories: "+staleCategories[0])
	}
	if len(driftCategories) > 0 {
		recommendations = append(recommendations, "Investigate drift in: "+driftCategories[0])
	}

	return &SwarmDriftReport{
		Timestamp:       time.Now(),
		BaselineAge:     time.Since(d.baselineTime),
		PSI:             psi,
		PSIStatus:       psiStatus,
		CSIByCategory:   csiMap,
		DriftDetected:   driftDetected,
		DriftCategories: driftCategories,
		StalenessScore:  d.CalculateStaleness(),
		StaleCategories: staleCategories,
		ActiveAlerts:    activeAlerts,
		Recommendations: recommendations,
	}
}

// ResetBaseline resets baseline to current distribution
func (d *DriftDetector) ResetBaseline() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.baselineStats = make(map[string]*DriftCategoryStats)
	for cat, stats := range d.currentStats {
		d.baselineStats[cat] = &DriftCategoryStats{
			Category:      stats.Category,
			Count:         stats.Count,
			Percentage:    stats.Percentage,
			AvgConfidence: stats.AvgConfidence,
			Workers:       stats.Workers,
			LastUpdated:   stats.LastUpdated,
		}
	}
	d.baselineTime = time.Now()
	d.alerts = make([]*DriftAlert, 0)
}

// GetHistory returns drift history
func (d *DriftDetector) GetHistory(limit int) []*DriftSnapshot {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if limit <= 0 || limit > len(d.history) {
		return d.history
	}
	return d.history[len(d.history)-limit:]
}

// Global singleton
var (
	globalDriftDetector   *DriftDetector
	globalDriftDetectorMu sync.RWMutex
)

// GetDriftDetector returns or creates the global drift detector
func GetDriftDetector() *DriftDetector {
	globalDriftDetectorMu.Lock()
	defer globalDriftDetectorMu.Unlock()

	if globalDriftDetector == nil {
		globalDriftDetector = NewDriftDetector()
	}
	return globalDriftDetector
}
