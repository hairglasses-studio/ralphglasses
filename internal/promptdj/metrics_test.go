package promptdj

import (
	"testing"
	"time"
)

func TestMetricsCollector_RecordDecision(t *testing.T) {
	m := NewMetricsCollector(100)

	d := &RoutingDecision{
		DecisionID:  "m1",
		Provider:    "claude",
		TaskType:    "code",
		Confidence:  0.85,
		ConfidenceLevel: "high",
		OriginalScore: 72,
		LatencyMs:   5,
		WasEnhanced: true,
		Timestamp:   time.Now(),
	}
	m.RecordDecision(d)

	snap := m.Snapshot()
	if snap.TotalDecisions != 1 {
		t.Errorf("expected 1 decision, got %d", snap.TotalDecisions)
	}
	if snap.ByProvider["claude"] != 1 {
		t.Error("expected 1 claude decision")
	}
	if snap.ByTaskType["code"] != 1 {
		t.Error("expected 1 code decision")
	}
	if snap.EnhancedCount != 1 {
		t.Error("expected 1 enhanced")
	}
	if snap.AvgConfidence < 0.84 || snap.AvgConfidence > 0.86 {
		t.Errorf("expected avg confidence ~0.85, got %.3f", snap.AvgConfidence)
	}
}

func TestMetricsCollector_RecordOutcome(t *testing.T) {
	m := NewMetricsCollector(100)

	m.RecordOutcome(true, 0.15)
	m.RecordOutcome(true, 0.10)
	m.RecordOutcome(false, 0.50)

	snap := m.Snapshot()
	if snap.TotalFeedback != 3 {
		t.Errorf("expected 3 feedback, got %d", snap.TotalFeedback)
	}
	if snap.Successes != 2 {
		t.Errorf("expected 2 successes, got %d", snap.Successes)
	}
	if snap.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", snap.Failures)
	}

	rate := m.SuccessRate()
	if rate < 0.66 || rate > 0.67 {
		t.Errorf("expected success rate ~0.67, got %.3f", rate)
	}
}

func TestMetricsCollector_WindowEviction(t *testing.T) {
	m := NewMetricsCollector(5) // small window

	for i := 0; i < 10; i++ {
		m.RecordDecision(&RoutingDecision{
			DecisionID: "w" + string(rune('0'+i)),
			Provider:   "claude",
			TaskType:   "code",
			Confidence: 0.8,
			ConfidenceLevel: "high",
			Timestamp:  time.Now(),
		})
	}

	m.mu.Lock()
	windowLen := len(m.window)
	m.mu.Unlock()

	if windowLen > 5 {
		t.Errorf("window should be capped at 5, got %d", windowLen)
	}

	snap := m.Snapshot()
	if snap.TotalDecisions != 10 {
		t.Errorf("total decisions should be 10 (not capped), got %d", snap.TotalDecisions)
	}
}
