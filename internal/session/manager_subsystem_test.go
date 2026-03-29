package session

import (
	"testing"
	"time"
)

func TestManagerSubsystem_SetAndGet(t *testing.T) {
	m := NewManager()

	// Initially all subsystems are nil
	if m.HasReflexion() {
		t.Error("expected HasReflexion=false initially")
	}
	if m.HasEpisodicMemory() {
		t.Error("expected HasEpisodicMemory=false initially")
	}
	// QW-2: Cascade routing is now enabled by default in NewManager.
	if !m.HasCascadeRouter() {
		t.Error("expected HasCascadeRouter=true by default (QW-2)")
	}
	if m.HasCurriculumSorter() {
		t.Error("expected HasCurriculumSorter=false initially")
	}
	if m.HasBandit() {
		t.Error("expected HasBandit=false initially")
	}
	if m.HasBlackboard() {
		t.Error("expected HasBlackboard=false initially")
	}
	if m.HasCostPredictor() {
		t.Error("expected HasCostPredictor=false initially")
	}
	if m.GetEpisodicMemory() != nil {
		t.Error("expected GetEpisodicMemory=nil initially")
	}
	if m.GetCascadeRouter() == nil {
		t.Error("expected GetCascadeRouter=non-nil by default (QW-2)")
	}
	if m.GetBlackboard() != nil {
		t.Error("expected GetBlackboard=nil initially")
	}
	if m.GetCostPredictor() != nil {
		t.Error("expected GetCostPredictor=nil initially")
	}
	if m.GetReflexionStore() != nil {
		t.Error("expected GetReflexionStore=nil initially")
	}

	// Set subsystems
	dir := t.TempDir()
	cr := NewCascadeRouter(DefaultCascadeConfig(), nil, nil, dir)
	m.SetCascadeRouter(cr)
	if !m.HasCascadeRouter() {
		t.Error("expected HasCascadeRouter=true after set")
	}
	if m.GetCascadeRouter() != cr {
		t.Error("expected GetCascadeRouter to return set router")
	}

	bb := NewBlackboard(dir)
	m.SetBlackboard(bb)
	if !m.HasBlackboard() {
		t.Error("expected HasBlackboard=true after set")
	}
	if m.GetBlackboard() != bb {
		t.Error("expected GetBlackboard to return set blackboard")
	}

	// CostPredictor
	cp := NewCostPredictor(dir)
	m.SetCostPredictor(cp)
	if !m.HasCostPredictor() {
		t.Error("expected HasCostPredictor=true after set")
	}
	if m.GetCostPredictor() != cp {
		t.Error("expected GetCostPredictor to return set predictor")
	}
}

func TestManagerSubsystem_AutoOptimizer(t *testing.T) {
	m := NewManager()

	ao := NewAutoOptimizer(nil, nil, nil, nil)
	m.SetAutoOptimizer(ao)

	// HITLSnapshot should return nil when no HITL tracker
	if snap := m.HITLSnapshot(); snap != nil {
		t.Errorf("expected nil HITLSnapshot, got %v", snap)
	}

	// FeedbackProfiles should return nil when no feedback
	if profiles := m.FeedbackProfiles(); profiles != nil {
		t.Errorf("expected nil FeedbackProfiles, got %v", profiles)
	}

	// ProviderProfiles should return nil when no feedback
	if profiles := m.ProviderProfiles(); profiles != nil {
		t.Errorf("expected nil ProviderProfiles, got %v", profiles)
	}
}

func TestManagerSubsystem_RecentDecisions(t *testing.T) {
	m := NewManager()

	// No optimizer — returns nil
	if decisions := m.RecentDecisions(10); decisions != nil {
		t.Errorf("expected nil decisions without optimizer, got %v", decisions)
	}

	// With optimizer but no decisions log
	ao := NewAutoOptimizer(nil, nil, nil, nil)
	m.SetAutoOptimizer(ao)
	if decisions := m.RecentDecisions(10); decisions != nil {
		t.Errorf("expected nil decisions without decisions log, got %v", decisions)
	}
}

func TestManagerSubsystem_GetAutonomyLevel(t *testing.T) {
	m := NewManager()

	// No optimizer returns LevelObserve
	if level := m.GetAutonomyLevel(); level != LevelObserve {
		t.Errorf("expected LevelObserve, got %v", level)
	}
}

func TestManagerSubsystem_BanditHooks(t *testing.T) {
	m := NewManager()

	called := false
	m.SetBanditHooks(
		func() (string, string) { return "gemini", "" },
		func(s string, f float64) { called = true },
	)

	if !m.HasBandit() {
		t.Error("expected HasBandit=true after set")
	}

	// Setting cascade router after bandit hooks should forward them
	dir := t.TempDir()
	cr := NewCascadeRouter(DefaultCascadeConfig(), nil, nil, dir)
	m.SetCascadeRouter(cr)

	if !cr.BanditConfigured() {
		t.Error("expected bandit hooks to be forwarded to cascade router")
	}
	_ = called
}

func TestManagerSubsystem_HITLSnapshot_WithTracker(t *testing.T) {
	m := NewManager()
	dir := t.TempDir()

	dl := NewDecisionLog(dir, LevelAutoOptimize)
	fa := NewFeedbackAnalyzer(dir, 3)
	hitl := NewHITLTracker(dir)
	hitl.RecordAuto(MetricAutoRecovery, "s1", "repo", "test")

	ao := NewAutoOptimizer(fa, dl, hitl, nil)
	m.SetAutoOptimizer(ao)

	snap := m.HITLSnapshot()
	if snap == nil {
		t.Fatal("expected non-nil HITLSnapshot with tracker")
	}
}

func TestManagerSubsystem_FeedbackProfiles_WithData(t *testing.T) {
	m := NewManager()
	dir := t.TempDir()

	fa := NewFeedbackAnalyzer(dir, 3)
	fa.Ingest([]JournalEntry{
		{
			Timestamp: time.Now(),
			SessionID: "s1",
			Provider:  "claude",
			SpentUSD:  0.5,
			TurnCount: 5,
			TaskFocus: "Fix parser bug",
			ExitReason: "completed",
		},
	})

	ao := NewAutoOptimizer(fa, nil, nil, nil)
	m.SetAutoOptimizer(ao)

	profiles := m.FeedbackProfiles()
	if profiles == nil {
		t.Fatal("expected non-nil FeedbackProfiles with data")
	}
}
