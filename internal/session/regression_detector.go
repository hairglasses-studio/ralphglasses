package session

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RegressionSeverity indicates how severe a detected regression is.
type RegressionSeverity string

const (
	SeverityWarning  RegressionSeverity = "warning"  // 5-15% degradation
	SeverityCritical RegressionSeverity = "critical"  // >15% degradation
)

// SessionRegression describes a detected regression in session metrics
// using sliding window comparison.
type SessionRegression struct {
	Metric          string             `json:"metric"`
	WindowAvg       float64            `json:"window_avg"`        // average of recent window
	BaselineAvg     float64            `json:"baseline_avg"`      // average of previous window
	DeltaPercent    float64            `json:"delta_percent"`     // positive = degradation
	Severity        RegressionSeverity `json:"severity"`
	SuggestedFix    string             `json:"suggested_fix"`
	DetectedAt      time.Time          `json:"detected_at"`
}

// SessionMetricPoint is a single time-stamped metric observation.
type SessionMetricPoint struct {
	SessionID   string    `json:"session_id"`
	Timestamp   time.Time `json:"ts"`
	SuccessRate float64   `json:"success_rate"`  // 0-100
	CostEfficiency float64 `json:"cost_efficiency"` // completions per dollar (higher = better)
	TimeToComplete float64 `json:"time_to_complete_sec"` // lower = better
}

// slidingWindowConfig holds thresholds for a single metric.
type slidingWindowConfig struct {
	// warningThreshold is the fractional degradation (0-1) that triggers a warning.
	warningThreshold float64
	// criticalThreshold is the fractional degradation (0-1) that triggers critical.
	criticalThreshold float64
	// higherIsBetter controls direction: true for success_rate, false for time_to_complete.
	higherIsBetter bool
}

// SessionRegressionConfig holds thresholds for all tracked metrics.
type SessionRegressionConfig struct {
	WindowSize         int     `json:"window_size"`          // sessions in each window
	WarningThreshold   float64 `json:"warning_threshold"`    // e.g. 0.05 = 5% degradation
	CriticalThreshold  float64 `json:"critical_threshold"`   // e.g. 0.15 = 15% degradation
}

// DefaultSessionRegressionConfig returns sensible defaults.
func DefaultSessionRegressionConfig() SessionRegressionConfig {
	return SessionRegressionConfig{
		WindowSize:        10,
		WarningThreshold:  0.05,
		CriticalThreshold: 0.15,
	}
}

// SessionRegressionDetector compares session metrics using a sliding window.
// It tracks success rate, cost efficiency, and time-to-completion, detecting
// degradation between the most recent N sessions and the previous N.
type SessionRegressionDetector struct {
	mu       sync.Mutex
	cfg      SessionRegressionConfig
	points   []SessionMetricPoint
	stateDir string
}

// NewSessionRegressionDetector creates a detector with the given config and state directory.
func NewSessionRegressionDetector(cfg SessionRegressionConfig, stateDir string) *SessionRegressionDetector {
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = 10
	}
	if cfg.WarningThreshold <= 0 {
		cfg.WarningThreshold = 0.05
	}
	if cfg.CriticalThreshold <= 0 {
		cfg.CriticalThreshold = 0.15
	}
	rd := &SessionRegressionDetector{
		cfg:      cfg,
		stateDir: stateDir,
	}
	rd.load()
	return rd
}

// AddPoint records a metric point from a completed session.
func (rd *SessionRegressionDetector) AddPoint(p SessionMetricPoint) {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	if p.Timestamp.IsZero() {
		p.Timestamp = time.Now()
	}
	rd.points = append(rd.points, p)
	rd.save()
}

// AddFromJournalEntry converts a JournalEntry into a SessionMetricPoint and records it.
func (rd *SessionRegressionDetector) AddFromJournalEntry(entry JournalEntry) {
	success := entry.ExitReason == "" || entry.ExitReason == "completed" || entry.ExitReason == "normal"
	successRate := 0.0
	if success {
		successRate = 100.0
	}
	costEff := 0.0
	if entry.SpentUSD > 0 && success {
		costEff = 1.0 / entry.SpentUSD
	}
	rd.AddPoint(SessionMetricPoint{
		SessionID:      entry.SessionID,
		Timestamp:      entry.Timestamp,
		SuccessRate:    successRate,
		CostEfficiency: costEff,
		TimeToComplete: entry.DurationSec,
	})
}

// Check runs the sliding window comparison and returns all detected regressions.
// Returns nil if there are fewer than 2*WindowSize data points.
func (rd *SessionRegressionDetector) Check() []SessionRegression {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	n := rd.cfg.WindowSize
	if len(rd.points) < 2*n {
		return nil
	}

	// Most recent N points = current window; previous N = baseline window.
	total := len(rd.points)
	current := rd.points[total-n:]
	baseline := rd.points[total-2*n : total-n]

	metricConfigs := map[string]slidingWindowConfig{
		"success_rate": {
			warningThreshold:  rd.cfg.WarningThreshold,
			criticalThreshold: rd.cfg.CriticalThreshold,
			higherIsBetter:    true,
		},
		"cost_efficiency": {
			warningThreshold:  rd.cfg.WarningThreshold,
			criticalThreshold: rd.cfg.CriticalThreshold,
			higherIsBetter:    true,
		},
		"time_to_complete": {
			warningThreshold:  rd.cfg.WarningThreshold,
			criticalThreshold: rd.cfg.CriticalThreshold,
			higherIsBetter:    false, // lower is better
		},
	}

	extract := func(pts []SessionMetricPoint, metric string) float64 {
		var sum float64
		count := 0
		for _, p := range pts {
			switch metric {
			case "success_rate":
				sum += p.SuccessRate
				count++
			case "cost_efficiency":
				sum += p.CostEfficiency
				count++
			case "time_to_complete":
				if p.TimeToComplete > 0 {
					sum += p.TimeToComplete
					count++
				}
			}
		}
		if count == 0 {
			return 0
		}
		return sum / float64(count)
	}

	var regressions []SessionRegression
	now := time.Now()

	for metric, mcfg := range metricConfigs {
		baseAvg := extract(baseline, metric)
		currAvg := extract(current, metric)

		if baseAvg == 0 {
			continue
		}

		var deltaPercent float64
		if mcfg.higherIsBetter {
			// Degradation = baseline was higher, current is lower.
			deltaPercent = (baseAvg - currAvg) / baseAvg
		} else {
			// Degradation = baseline was lower, current is higher.
			deltaPercent = (currAvg - baseAvg) / baseAvg
		}

		if deltaPercent <= rd.cfg.WarningThreshold {
			continue
		}

		severity := SeverityWarning
		if deltaPercent > rd.cfg.CriticalThreshold {
			severity = SeverityCritical
		}

		regressions = append(regressions, SessionRegression{
			Metric:       metric,
			WindowAvg:    currAvg,
			BaselineAvg:  baseAvg,
			DeltaPercent: deltaPercent,
			Severity:     severity,
			SuggestedFix: suggestedFixFor(metric, severity),
			DetectedAt:   now,
		})
	}

	return regressions
}

// PointCount returns the number of recorded metric points.
func (rd *SessionRegressionDetector) PointCount() int {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	return len(rd.points)
}

// suggestedFixFor returns a human-readable suggested fix for a regressed metric.
func suggestedFixFor(metric string, severity RegressionSeverity) string {
	switch metric {
	case "success_rate":
		if severity == SeverityCritical {
			return "Critical success rate drop: review recent prompt changes, check provider health, consider switching providers"
		}
		return "Success rate declining: inspect recent session logs for recurring errors"
	case "cost_efficiency":
		if severity == SeverityCritical {
			return "Critical cost efficiency drop: audit recent sessions for runaway turn counts, consider budget caps"
		}
		return "Cost efficiency declining: review provider selection and prompt quality"
	case "time_to_complete":
		if severity == SeverityCritical {
			return "Critical completion time increase: check for stalls or resource contention"
		}
		return "Completion time increasing: review task complexity and consider task decomposition"
	default:
		return "Review recent session patterns for " + metric
	}
}

// regressionDetectorStore is the JSON persistence structure.
type regressionDetectorStore struct {
	Points    []SessionMetricPoint `json:"points"`
	UpdatedAt time.Time            `json:"updated_at"`
}

func (rd *SessionRegressionDetector) save() {
	if rd.stateDir == "" {
		return
	}
	if err := os.MkdirAll(rd.stateDir, 0755); err != nil {
		slog.Warn("regression_detector: failed to create state dir", "error", err)
		return
	}
	store := regressionDetectorStore{
		Points:    rd.points,
		UpdatedAt: time.Now(),
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		slog.Warn("regression_detector: marshal failed", "error", err)
		return
	}
	path := filepath.Join(rd.stateDir, "regression_metrics.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		slog.Warn("regression_detector: write failed", "path", path, "error", err)
	}
}

func (rd *SessionRegressionDetector) load() {
	if rd.stateDir == "" {
		return
	}
	path := filepath.Join(rd.stateDir, "regression_metrics.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var store regressionDetectorStore
	if json.Unmarshal(data, &store) == nil {
		rd.points = store.Points
	}
}
