package session

import (
	"testing"
)

func helperOptimizer(t *testing.T) (*Manager, *AutoOptimizer) {
	t.Helper()
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)
	dl := NewDecisionLog(dir, LevelAutoRecover)
	hitl := NewHITLTracker(dir)
	fb := NewFeedbackAnalyzer(dir, 0)
	ar := NewAutoRecovery(m, dl, hitl, DefaultAutoRecoveryConfig())
	opt := NewAutoOptimizer(fb, dl, hitl, ar)
	m.SetAutoOptimizer(opt)
	return m, opt
}

func TestProviderProfiles_NilOptimizer(t *testing.T) {
	m := NewManager()
	profiles := m.ProviderProfiles()
	if profiles != nil {
		t.Error("expected nil when no optimizer")
	}
}

func TestProviderProfiles_WithOptimizer(t *testing.T) {
	m, _ := helperOptimizer(t)
	profiles := m.ProviderProfiles()
	if profiles == nil {
		t.Error("expected non-nil (possibly empty) profiles with optimizer")
	}
}

func TestRecentDecisions_NilOptimizer(t *testing.T) {
	m := NewManager()
	decisions := m.RecentDecisions(10)
	if decisions != nil {
		t.Error("expected nil when no optimizer")
	}
}

func TestRecentDecisions_WithOptimizer(t *testing.T) {
	m, _ := helperOptimizer(t)
	decisions := m.RecentDecisions(10)
	if len(decisions) != 0 {
		t.Errorf("expected 0 decisions, got %d", len(decisions))
	}
}

func TestGetAutonomyLevel_NilOptimizer(t *testing.T) {
	m := NewManager()
	level := m.GetAutonomyLevel()
	if level != LevelObserve {
		t.Errorf("expected LevelObserve, got %v", level)
	}
}

func TestGetAutonomyLevel_WithOptimizer(t *testing.T) {
	m, _ := helperOptimizer(t)
	level := m.GetAutonomyLevel()
	if level != LevelAutoRecover {
		t.Errorf("expected LevelAutoRecover, got %v", level)
	}
}

func TestSetBanditHooks_ForwardsToExistingCascade(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()

	dl := NewDecisionLog(dir, LevelAutoRecover)
	fb := NewFeedbackAnalyzer(dir, 0)
	cr := NewCascadeRouter(CascadeConfig{}, fb, dl, dir)
	m.SetCascadeRouter(cr)

	selectCalled := false
	m.SetBanditHooks(
		func() (string, string) { selectCalled = true; return "claude", "arm-1" },
		func(string, float64) {},
	)

	if !m.HasBandit() {
		t.Error("expected HasBandit to be true")
	}
	_ = selectCalled
}

func TestCheckHealth_WithCustomFunction(t *testing.T) {
	m := NewManager()
	m.SetHealthCheckForTesting(func(p Provider) ProviderHealth {
		return ProviderHealth{Available: true, LatencyMs: 100}
	})

	h := m.checkHealth(ProviderClaude)
	if !h.Available {
		t.Error("expected Available = true from custom health check")
	}
}

func TestCheckHealth_DefaultFallback(t *testing.T) {
	m := NewManager()
	h := m.checkHealth(ProviderClaude)
	_ = h // no panic is the test
}

func TestHITLSnapshot_NilOptimizer(t *testing.T) {
	m := NewManager()
	snap := m.HITLSnapshot()
	if snap != nil {
		t.Error("expected nil snapshot when no optimizer")
	}
}

func TestFeedbackProfiles_NilOptimizer(t *testing.T) {
	m := NewManager()
	profiles := m.FeedbackProfiles()
	if profiles != nil {
		t.Error("expected nil when no optimizer")
	}
}

func TestFeedbackProfiles_WithOptimizer(t *testing.T) {
	m, _ := helperOptimizer(t)
	profiles := m.FeedbackProfiles()
	if profiles == nil {
		t.Error("expected non-nil profiles with optimizer")
	}
}

func TestGetReflexionStore_RoundTrip(t *testing.T) {
	m := NewManager()
	if m.GetReflexionStore() != nil {
		t.Error("expected nil before set")
	}

	rs := NewReflexionStore(t.TempDir())
	m.SetReflexionStore(rs)
	if m.GetReflexionStore() != rs {
		t.Error("expected same ReflexionStore back")
	}
}

func TestGetCostPredictor_RoundTrip(t *testing.T) {
	m := NewManager()
	if m.HasCostPredictor() {
		t.Error("expected no cost predictor")
	}

	cp := NewCostPredictor(t.TempDir())
	m.SetCostPredictor(cp)
	if !m.HasCostPredictor() {
		t.Error("expected cost predictor after set")
	}
	if m.GetCostPredictor() != cp {
		t.Error("expected same CostPredictor back")
	}
}

func TestGetBlackboard_RoundTrip(t *testing.T) {
	m := NewManager()
	if m.HasBlackboard() {
		t.Error("expected no blackboard")
	}

	bb := NewBlackboard(t.TempDir())
	m.SetBlackboard(bb)
	if !m.HasBlackboard() {
		t.Error("expected blackboard after set")
	}
	if m.GetBlackboard() != bb {
		t.Error("expected same Blackboard back")
	}
}
