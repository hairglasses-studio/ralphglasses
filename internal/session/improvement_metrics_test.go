package session

import (
	"sync"
	"testing"
)

func TestImprovementMetrics_EmptyState(t *testing.T) {
	im := NewImprovementMetrics(nil, nil)
	summary := im.Summary()

	if summary.TotalInsights != 0 {
		t.Errorf("expected 0 insights, got %d", summary.TotalInsights)
	}
	if summary.LearningVelocity != 0 {
		t.Errorf("expected 0 learning velocity, got %.2f", summary.LearningVelocity)
	}
	if summary.RegressionFrequency != 0 {
		t.Errorf("expected 0 regression frequency, got %.2f", summary.RegressionFrequency)
	}
	if summary.ComputedAt.IsZero() {
		t.Error("expected non-zero ComputedAt")
	}
	if summary.InsightsByType == nil {
		t.Error("expected non-nil InsightsByType map")
	}
}

func TestImprovementMetrics_LearningVelocity(t *testing.T) {
	lt := NewLearningTransfer("")

	// 6 sessions with common patterns generate insights.
	for i := range 6 {
		lt.RecordSession(SessionLearning{
			SessionID: "sess-" + ltItoa(i),
			TaskType:  "feature",
			Provider:  "claude",
			Success:   true,
			CostUSD:   0.50,
			Worked:    []string{"added feature", "wrote tests"},
		})
	}

	im := NewImprovementMetrics(lt, nil)
	summary := im.Summary()

	if summary.LearningVelocity < 0 || summary.LearningVelocity > 1 {
		t.Errorf("learning velocity out of [0,1]: %.2f", summary.LearningVelocity)
	}
	if summary.TotalSessionsTracked != 6 {
		t.Errorf("expected 6 sessions tracked, got %d", summary.TotalSessionsTracked)
	}
}

func TestImprovementMetrics_InsightsByType(t *testing.T) {
	lt := NewLearningTransfer("")

	// 4 sessions with same provider+task type should generate a provider_hint.
	for i := range 4 {
		lt.RecordSession(SessionLearning{
			SessionID: "sess-" + ltItoa(i),
			TaskType:  "test",
			Provider:  "gemini",
			Success:   true,
			CostUSD:   0.10,
		})
	}

	im := NewImprovementMetrics(lt, nil)
	summary := im.Summary()

	if summary.InsightsByType["provider_hint"] == 0 {
		t.Error("expected at least one provider_hint insight")
	}
}

func TestImprovementMetrics_RegressionFrequency(t *testing.T) {
	im := NewImprovementMetrics(nil, nil)

	// No regressions found in these checks (nil detector).
	im.RecordCheck()
	im.RecordCheck()
	im.RecordCheck()

	summary := im.Summary()
	if summary.RegressionFrequency != 0 {
		t.Errorf("expected 0 regression frequency with nil detector, got %.2f", summary.RegressionFrequency)
	}
	if summary.RecentRegressions != 0 {
		t.Errorf("expected 0 recent regressions, got %d", summary.RecentRegressions)
	}
}

func TestImprovementMetrics_RegressionFrequencyWithDetector(t *testing.T) {
	cfg := SessionRegressionConfig{WindowSize: 3, WarningThreshold: 0.05, CriticalThreshold: 0.15}
	rd := NewSessionRegressionDetector(cfg, "")

	// Baseline: 90% success.
	for i := range 3 {
		rd.AddPoint(SessionMetricPoint{
			SessionID:   "base-" + ltItoa(i),
			SuccessRate: 90.0,
		})
	}
	// Current: 50% success (regression).
	for i := range 3 {
		rd.AddPoint(SessionMetricPoint{
			SessionID:   "curr-" + ltItoa(i),
			SuccessRate: 50.0,
		})
	}

	im := NewImprovementMetrics(nil, rd)
	regressions := im.RecordCheck()
	if len(regressions) == 0 {
		t.Fatal("expected regressions to be detected")
	}

	summary := im.Summary()
	if summary.RegressionFrequency != 1.0 {
		t.Errorf("expected regression frequency of 1.0 after one check with regressions, got %.2f", summary.RegressionFrequency)
	}
	if summary.RecentRegressions == 0 {
		t.Error("expected non-zero recent regressions")
	}
}

func TestImprovementMetrics_SuccessRateTrend(t *testing.T) {
	im := NewImprovementMetrics(nil, nil)

	// Improving trend: record increasing success rates.
	im.RecordSessionOutcome(false, 0.10)
	im.RecordSessionOutcome(false, 0.10)
	im.RecordSessionOutcome(true, 0.10)
	im.RecordSessionOutcome(true, 0.10)
	im.RecordSessionOutcome(true, 0.10)

	summary := im.Summary()
	if summary.SuccessRateTrend <= 0 {
		t.Errorf("expected positive trend for improving sessions, got %.3f", summary.SuccessRateTrend)
	}
}

func TestImprovementMetrics_DecliningTrend(t *testing.T) {
	im := NewImprovementMetrics(nil, nil)

	// Declining trend: successrate goes from 1.0 to 0.0.
	im.RecordSessionOutcome(true, 0.10)
	im.RecordSessionOutcome(true, 0.10)
	im.RecordSessionOutcome(false, 0.10)
	im.RecordSessionOutcome(false, 0.10)
	im.RecordSessionOutcome(false, 0.10)

	summary := im.Summary()
	if summary.SuccessRateTrend >= 0 {
		t.Errorf("expected negative trend for declining sessions, got %.3f", summary.SuccessRateTrend)
	}
}

func TestImprovementMetrics_AvgCost(t *testing.T) {
	im := NewImprovementMetrics(nil, nil)

	im.RecordSessionOutcome(true, 1.00)
	im.RecordSessionOutcome(true, 2.00)
	im.RecordSessionOutcome(true, 3.00)

	summary := im.Summary()
	if summary.AvgCostUSD < 1.99 || summary.AvgCostUSD > 2.01 {
		t.Errorf("expected avg cost ~$2.00, got $%.2f", summary.AvgCostUSD)
	}
}

func TestImprovementMetrics_OptimizationEffectiveness(t *testing.T) {
	lt := NewLearningTransfer("")

	// Generate enough sessions to trigger provider_hint and budget_hint.
	for i := range 4 {
		lt.RecordSession(SessionLearning{
			SessionID: "sess-" + ltItoa(i),
			TaskType:  "feature",
			Provider:  "claude",
			Success:   true,
			CostUSD:   0.50,
		})
	}

	im := NewImprovementMetrics(lt, nil)
	summary := im.Summary()

	if summary.OptimizationEffectiveness < 0 || summary.OptimizationEffectiveness > 1 {
		t.Errorf("optimization effectiveness out of [0,1]: %.2f", summary.OptimizationEffectiveness)
	}
}

func TestImprovementMetrics_LinearTrend(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		wantSign int // -1, 0, +1
	}{
		{"flat", []float64{0.5, 0.5, 0.5, 0.5}, 0},
		{"increasing", []float64{0.0, 0.25, 0.50, 0.75, 1.0}, 1},
		{"decreasing", []float64{1.0, 0.75, 0.50, 0.25, 0.0}, -1},
		{"single", []float64{0.5}, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			slope := linearTrend(tc.values)
			if slope < -1.01 || slope > 1.01 {
				t.Errorf("linearTrend out of [-1,1]: %.3f", slope)
			}
			switch tc.wantSign {
			case 1:
				if slope <= 0 {
					t.Errorf("expected positive slope for increasing series, got %.3f", slope)
				}
			case -1:
				if slope >= 0 {
					t.Errorf("expected negative slope for decreasing series, got %.3f", slope)
				}
			case 0:
				// Allow near-zero tolerance.
				if slope < -0.01 || slope > 0.01 {
					t.Errorf("expected near-zero slope for flat series, got %.3f", slope)
				}
			}
		})
	}
}

func TestImprovementMetrics_ConcurrentSafety(t *testing.T) {
	im := NewImprovementMetrics(nil, nil)
	var wg sync.WaitGroup

	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			im.RecordSessionOutcome(n%2 == 0, float64(n)*0.1)
		}(i)
	}

	for range 10 {
		wg.Go(func() {
			im.RecordCheck()
		})
	}

	for range 10 {
		wg.Go(func() {
			im.Summary()
		})
	}

	wg.Wait()
}

func TestImprovementMetrics_TrendWithSinglePoint(t *testing.T) {
	im := NewImprovementMetrics(nil, nil)
	im.RecordSessionOutcome(true, 0.10)

	summary := im.Summary()
	// With only 1 point, trend should be 0 (no slope computable).
	if summary.SuccessRateTrend != 0 {
		t.Errorf("expected 0 trend with 1 data point, got %.3f", summary.SuccessRateTrend)
	}
}
