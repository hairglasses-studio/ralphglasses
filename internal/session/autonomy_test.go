package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecisionLog_ProposeAtLevel(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelObserve)

	// Level 0: observe only — should NOT execute level 1 decisions
	d := AutonomousDecision{
		Category:      DecisionRestart,
		RequiredLevel: LevelAutoRecover,
		Rationale:     "transient error detected",
		Action:        "restart session",
	}
	allowed := dl.Propose(d)
	if allowed {
		t.Error("level 0 should not execute level 1 decisions")
	}

	// Check it was logged as "would have done"
	recent := dl.Recent(1)
	if len(recent) != 1 {
		t.Fatal("expected 1 decision in log")
	}
	if recent[0].Executed {
		t.Error("decision should be logged as not executed")
	}
}

func TestDecisionLog_ProposeAllowed(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelAutoRecover)

	d := AutonomousDecision{
		Category:      DecisionRestart,
		RequiredLevel: LevelAutoRecover,
		Rationale:     "transient error detected",
		Action:        "restart session",
	}
	allowed := dl.Propose(d)
	if !allowed {
		t.Error("level 1 should execute level 1 decisions")
	}

	recent := dl.Recent(1)
	if !recent[0].Executed {
		t.Error("decision should be logged as executed")
	}
}

func TestDecisionLog_Blocklist(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelFullAutonomy)

	dl.Block(DecisionRestart)

	d := AutonomousDecision{
		Category:      DecisionRestart,
		RequiredLevel: LevelAutoRecover,
		Action:        "restart",
	}
	allowed := dl.Propose(d)
	if allowed {
		t.Error("blocked category should not execute")
	}

	dl.Unblock(DecisionRestart)
	allowed = dl.Propose(AutonomousDecision{
		Category:      DecisionRestart,
		RequiredLevel: LevelAutoRecover,
		Action:        "restart2",
	})
	if !allowed {
		t.Error("unblocked category should execute")
	}
}

func TestDecisionLog_RecordOutcome(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelAutoRecover)

	d := AutonomousDecision{
		ID:            "dec-test",
		Category:      DecisionRestart,
		RequiredLevel: LevelAutoRecover,
		Action:        "restart",
	}
	dl.Propose(d)

	dl.RecordOutcome("dec-test", DecisionOutcome{
		Success: true,
		Details: "session restarted successfully",
	})

	recent := dl.Recent(1)
	if recent[0].Outcome == nil {
		t.Fatal("expected outcome to be set")
	}
	if !recent[0].Outcome.Success {
		t.Error("outcome should be success")
	}
}

func TestDecisionLog_RecentDefaultLimit(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelFullAutonomy)

	// Add a few decisions
	for i := 0; i < 5; i++ {
		dl.Propose(AutonomousDecision{Category: DecisionRestart, RequiredLevel: LevelAutoRecover, Action: "test"})
	}

	// limit=0 should default to 20, return all 5
	recent := dl.Recent(0)
	if len(recent) != 5 {
		t.Errorf("Recent(0) = %d, want 5", len(recent))
	}

	// limit=-1 should also default
	recent = dl.Recent(-1)
	if len(recent) != 5 {
		t.Errorf("Recent(-1) = %d, want 5", len(recent))
	}
}

func TestDecisionLog_Stats_WithOutcomes(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelFullAutonomy)

	dl.Propose(AutonomousDecision{ID: "d1", Category: DecisionRestart, RequiredLevel: LevelAutoRecover, Action: "a"})
	dl.RecordOutcome("d1", DecisionOutcome{Success: true})
	dl.Propose(AutonomousDecision{ID: "d2", Category: DecisionRestart, RequiredLevel: LevelAutoRecover, Action: "b"})
	dl.RecordOutcome("d2", DecisionOutcome{Overridden: true})

	stats := dl.Stats()
	if stats["succeeded"].(int) != 1 {
		t.Errorf("succeeded = %d, want 1", stats["succeeded"])
	}
	if stats["overridden"].(int) != 1 {
		t.Errorf("overridden = %d, want 1", stats["overridden"])
	}
}

func TestDecisionLog_Stats(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelAutoOptimize)

	// 3 decisions, 2 executed (level 2+), 1 not (level 3)
	dl.Propose(AutonomousDecision{Category: DecisionRestart, RequiredLevel: LevelAutoRecover, Action: "a"})
	dl.Propose(AutonomousDecision{Category: DecisionBudgetAdjust, RequiredLevel: LevelAutoOptimize, Action: "b"})
	dl.Propose(AutonomousDecision{Category: DecisionLaunch, RequiredLevel: LevelFullAutonomy, Action: "c"})

	stats := dl.Stats()
	if stats["total_decisions"].(int) != 3 {
		t.Errorf("total: got %d, want 3", stats["total_decisions"])
	}
	if stats["executed"].(int) != 2 {
		t.Errorf("executed: got %d, want 2", stats["executed"])
	}
	if stats["would_have_done"].(int) != 1 {
		t.Errorf("would_have_done: got %d, want 1", stats["would_have_done"])
	}
}

func TestDecisionLog_Persistence(t *testing.T) {
	dir := t.TempDir()

	dl1 := NewDecisionLog(dir, LevelAutoRecover)
	dl1.Propose(AutonomousDecision{Category: DecisionRestart, RequiredLevel: LevelAutoRecover, Action: "test"})

	// Verify file exists
	path := filepath.Join(dir, "decisions.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("decisions file not created: %v", err)
	}

	// Reload
	dl2 := NewDecisionLog(dir, LevelAutoRecover)
	recent := dl2.Recent(10)
	if len(recent) != 1 {
		t.Errorf("after reload: got %d decisions, want 1", len(recent))
	}
}

func TestDecisionLog_SetLevel(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelObserve)

	if dl.Level() != LevelObserve {
		t.Errorf("initial level: got %d, want 0", dl.Level())
	}

	dl.SetLevel(LevelAutoRecover)
	if dl.Level() != LevelAutoRecover {
		t.Errorf("after set: got %d, want 1", dl.Level())
	}
}

func TestAutonomyLevel_String(t *testing.T) {
	tests := []struct {
		level AutonomyLevel
		want  string
	}{
		{LevelObserve, "observe"},
		{LevelAutoRecover, "auto-recover"},
		{LevelAutoOptimize, "auto-optimize"},
		{LevelFullAutonomy, "full-autonomy"},
		{AutonomyLevel(99), "unknown(99)"},
	}
	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("level %d: got %q, want %q", tt.level, got, tt.want)
		}
	}
}
