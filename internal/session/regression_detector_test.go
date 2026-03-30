package session

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSessionRegressionDetector_InsufficientData(t *testing.T) {
	rd := NewSessionRegressionDetector(DefaultSessionRegressionConfig(), "")
	regressions := rd.Check()
	if regressions != nil {
		t.Fatalf("expected nil regressions with insufficient data, got %d", len(regressions))
	}
}

func TestSessionRegressionDetector_NoRegression(t *testing.T) {
	cfg := SessionRegressionConfig{WindowSize: 3, WarningThreshold: 0.05, CriticalThreshold: 0.15}
	rd := NewSessionRegressionDetector(cfg, "")

	// Add 6 uniform sessions (no change between windows).
	for i := 0; i < 6; i++ {
		rd.AddPoint(SessionMetricPoint{
			SessionID:      "sess-" + ltItoa(i),
			Timestamp:      time.Now(),
			SuccessRate:    90.0,
			CostEfficiency: 5.0,
			TimeToComplete: 60.0,
		})
	}

	regressions := rd.Check()
	if len(regressions) != 0 {
		t.Errorf("expected no regressions for uniform sessions, got %d: %+v", len(regressions), regressions)
	}
}

func TestSessionRegressionDetector_SuccessRateRegression(t *testing.T) {
	cfg := SessionRegressionConfig{WindowSize: 3, WarningThreshold: 0.05, CriticalThreshold: 0.15}
	rd := NewSessionRegressionDetector(cfg, "")

	// Baseline window: 90% success rate.
	for i := 0; i < 3; i++ {
		rd.AddPoint(SessionMetricPoint{
			SessionID:      "base-" + ltItoa(i),
			Timestamp:      time.Now(),
			SuccessRate:    90.0,
			CostEfficiency: 5.0,
			TimeToComplete: 60.0,
		})
	}
	// Current window: 50% success rate (44% drop — above critical threshold).
	for i := 0; i < 3; i++ {
		rd.AddPoint(SessionMetricPoint{
			SessionID:      "curr-" + ltItoa(i),
			Timestamp:      time.Now(),
			SuccessRate:    50.0,
			CostEfficiency: 5.0,
			TimeToComplete: 60.0,
		})
	}

	regressions := rd.Check()
	if len(regressions) == 0 {
		t.Fatal("expected regression for 44% success rate drop")
	}

	found := false
	for _, r := range regressions {
		if r.Metric == "success_rate" {
			found = true
			if r.Severity != SeverityCritical {
				t.Errorf("expected critical severity, got %q", r.Severity)
			}
			if r.SuggestedFix == "" {
				t.Error("expected non-empty suggested fix")
			}
		}
	}
	if !found {
		t.Error("expected success_rate regression in results")
	}
}

func TestSessionRegressionDetector_CostEfficiencyRegression(t *testing.T) {
	cfg := SessionRegressionConfig{WindowSize: 4, WarningThreshold: 0.10, CriticalThreshold: 0.25}
	rd := NewSessionRegressionDetector(cfg, "")

	// Baseline: high cost efficiency.
	for i := 0; i < 4; i++ {
		rd.AddPoint(SessionMetricPoint{
			SessionID:      "base-" + ltItoa(i),
			Timestamp:      time.Now(),
			SuccessRate:    80.0,
			CostEfficiency: 10.0, // high efficiency
			TimeToComplete: 60.0,
		})
	}
	// Current: 50% drop in cost efficiency.
	for i := 0; i < 4; i++ {
		rd.AddPoint(SessionMetricPoint{
			SessionID:      "curr-" + ltItoa(i),
			Timestamp:      time.Now(),
			SuccessRate:    80.0,
			CostEfficiency: 5.0, // halved
			TimeToComplete: 60.0,
		})
	}

	regressions := rd.Check()
	found := false
	for _, r := range regressions {
		if r.Metric == "cost_efficiency" {
			found = true
			if r.DeltaPercent < 0.49 || r.DeltaPercent > 0.51 {
				t.Errorf("expected ~50%% delta, got %.2f", r.DeltaPercent)
			}
		}
	}
	if !found {
		t.Error("expected cost_efficiency regression")
	}
}

func TestSessionRegressionDetector_TimeToCompleteRegression(t *testing.T) {
	cfg := SessionRegressionConfig{WindowSize: 3, WarningThreshold: 0.05, CriticalThreshold: 0.15}
	rd := NewSessionRegressionDetector(cfg, "")

	// Baseline: fast sessions.
	for i := 0; i < 3; i++ {
		rd.AddPoint(SessionMetricPoint{
			SessionID:      "base-" + ltItoa(i),
			Timestamp:      time.Now(),
			SuccessRate:    80.0,
			CostEfficiency: 5.0,
			TimeToComplete: 60.0,
		})
	}
	// Current: much slower (100% increase).
	for i := 0; i < 3; i++ {
		rd.AddPoint(SessionMetricPoint{
			SessionID:      "curr-" + ltItoa(i),
			Timestamp:      time.Now(),
			SuccessRate:    80.0,
			CostEfficiency: 5.0,
			TimeToComplete: 120.0,
		})
	}

	regressions := rd.Check()
	found := false
	for _, r := range regressions {
		if r.Metric == "time_to_complete" {
			found = true
			if r.Severity != SeverityCritical {
				t.Errorf("expected critical for 100%% time increase, got %q", r.Severity)
			}
		}
	}
	if !found {
		t.Error("expected time_to_complete regression")
	}
}

func TestSessionRegressionDetector_WarningSeverity(t *testing.T) {
	cfg := SessionRegressionConfig{WindowSize: 3, WarningThreshold: 0.05, CriticalThreshold: 0.15}
	rd := NewSessionRegressionDetector(cfg, "")

	// Baseline: 90% success.
	for i := 0; i < 3; i++ {
		rd.AddPoint(SessionMetricPoint{
			SessionID:   "base-" + ltItoa(i),
			Timestamp:   time.Now(),
			SuccessRate: 100.0,
		})
	}
	// Current: 90% success — 10% drop (between warning and critical thresholds).
	for i := 0; i < 3; i++ {
		rd.AddPoint(SessionMetricPoint{
			SessionID:   "curr-" + ltItoa(i),
			Timestamp:   time.Now(),
			SuccessRate: 90.0,
		})
	}

	regressions := rd.Check()
	found := false
	for _, r := range regressions {
		if r.Metric == "success_rate" {
			found = true
			if r.Severity != SeverityWarning {
				t.Errorf("expected warning severity for 10%% drop, got %q", r.Severity)
			}
		}
	}
	if !found {
		t.Error("expected success_rate warning regression")
	}
}

func TestSessionRegressionDetector_AddFromJournalEntry(t *testing.T) {
	rd := NewSessionRegressionDetector(DefaultSessionRegressionConfig(), "")

	entry := JournalEntry{
		Timestamp:   time.Now(),
		SessionID:   "j-1",
		ExitReason:  "completed",
		SpentUSD:    0.20,
		DurationSec: 90,
	}
	rd.AddFromJournalEntry(entry)

	if rd.PointCount() != 1 {
		t.Errorf("expected 1 point after adding from journal, got %d", rd.PointCount())
	}
}

func TestSessionRegressionDetector_Persistence(t *testing.T) {
	dir := t.TempDir()
	rd := NewSessionRegressionDetector(DefaultSessionRegressionConfig(), dir)

	for i := 0; i < 5; i++ {
		rd.AddPoint(SessionMetricPoint{
			SessionID:   "sess-" + ltItoa(i),
			Timestamp:   time.Now(),
			SuccessRate: 80.0,
		})
	}

	// File should be written.
	path := filepath.Join(dir, "regression_metrics.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected regression_metrics.json to exist: %v", err)
	}

	// Reload and verify count.
	rd2 := NewSessionRegressionDetector(DefaultSessionRegressionConfig(), dir)
	if rd2.PointCount() != 5 {
		t.Errorf("expected 5 points after reload, got %d", rd2.PointCount())
	}
}

func TestSessionRegressionDetector_ConcurrentSafety(t *testing.T) {
	rd := NewSessionRegressionDetector(DefaultSessionRegressionConfig(), "")
	var wg sync.WaitGroup

	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rd.AddPoint(SessionMetricPoint{
				SessionID:   "sess-" + ltItoa(n),
				Timestamp:   time.Now(),
				SuccessRate: float64(50 + n%50),
			})
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rd.Check()
		}()
	}

	wg.Wait()
}

func TestSuggestedFixFor(t *testing.T) {
	tests := []struct {
		metric   string
		severity RegressionSeverity
	}{
		{"success_rate", SeverityWarning},
		{"success_rate", SeverityCritical},
		{"cost_efficiency", SeverityWarning},
		{"cost_efficiency", SeverityCritical},
		{"time_to_complete", SeverityWarning},
		{"time_to_complete", SeverityCritical},
		{"unknown_metric", SeverityWarning},
	}

	for _, tc := range tests {
		fix := suggestedFixFor(tc.metric, tc.severity)
		if fix == "" {
			t.Errorf("expected non-empty fix for metric=%q severity=%q", tc.metric, tc.severity)
		}
	}
}
