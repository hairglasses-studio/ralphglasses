package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestDecisionLog_ProposeDecoratesMetadata(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelObserve)

	allowed := dl.Propose(AutonomousDecision{
		ID:            "dec-meta",
		Category:      DecisionLaunch,
		RequiredLevel: LevelFullAutonomy,
		Action:        "launch roadmap tranche",
		Rationale:     "backlog item is ready",
		SessionID:     "sess-123",
		RepoName:      "ralphglasses",
	})
	if allowed {
		t.Fatal("observe mode should not execute full-autonomy launch decisions")
	}

	recent := dl.Recent(1)
	if len(recent) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(recent))
	}
	got := recent[0]
	if got.PolicySource != "decision-log:insufficient-level" {
		t.Errorf("policy_source = %q", got.PolicySource)
	}
	if got.RollbackHint == "" {
		t.Error("expected rollback_hint to be populated")
	}
	if got.UndoHandle != "session:sess-123" {
		t.Errorf("undo_handle = %q, want session:sess-123", got.UndoHandle)
	}
	if len(got.RiskTags) == 0 {
		t.Fatal("expected risk_tags to be populated")
	}
	if got.Counterfactual == "" {
		t.Fatal("expected counterfactual to be populated")
	}
	if !strings.Contains(got.Counterfactual, "full-autonomy") {
		t.Errorf("counterfactual = %q, want required level context", got.Counterfactual)
	}
}

func TestDecisionLog_SnapshotIncludesMetadata(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelAutoOptimize)

	dl.Propose(AutonomousDecision{
		ID:            "dec-budget",
		Category:      DecisionBudgetAdjust,
		RequiredLevel: LevelAutoOptimize,
		Action:        "raise budget ceiling",
		Rationale:     "provider costs spiked",
	})

	snapshot := dl.Snapshot(5)
	stats, ok := snapshot["stats"].(map[string]any)
	if !ok {
		t.Fatalf("stats type = %T", snapshot["stats"])
	}
	byPolicy, ok := stats["by_policy_source"].(map[string]int)
	if !ok {
		t.Fatalf("by_policy_source type = %T", stats["by_policy_source"])
	}
	if byPolicy["decision-log:autonomy-level"] != 1 {
		t.Errorf("by_policy_source = %#v", byPolicy)
	}
	byRisk, ok := stats["by_risk_tag"].(map[string]int)
	if !ok {
		t.Fatalf("by_risk_tag type = %T", stats["by_risk_tag"])
	}
	if byRisk["cost"] != 1 {
		t.Errorf("by_risk_tag = %#v", byRisk)
	}
	recent, ok := snapshot["recent"].([]AutonomousDecisionSummary)
	if !ok {
		t.Fatalf("recent type = %T", snapshot["recent"])
	}
	if len(recent) != 1 {
		t.Fatalf("recent length = %d, want 1", len(recent))
	}
	if recent[0].PolicySource == "" || recent[0].RollbackHint == "" {
		t.Fatalf("recent summary missing metadata: %#v", recent[0])
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
	recent := dl.Recent(1)
	if recent[0].PolicySource != "decision-log:blocklist" {
		t.Errorf("policy_source = %q, want decision-log:blocklist", recent[0].PolicySource)
	}
	if !strings.Contains(recent[0].Counterfactual, "blocklisted") {
		t.Errorf("counterfactual = %q, want blocklist explanation", recent[0].Counterfactual)
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
	for range 5 {
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

func TestBootstrapAutonomy_Defaults(t *testing.T) {
	ac := BootstrapAutonomy(nil)
	if ac.Level != LevelObserve {
		t.Errorf("default level: got %d, want 0", ac.Level)
	}
	if ac.AutoRecover {
		t.Error("default auto-recover should be false")
	}
	if ac.MaxRecoveries != 3 {
		t.Errorf("default max recoveries: got %d, want 3", ac.MaxRecoveries)
	}
}

func TestBootstrapAutonomy_EmptyMap(t *testing.T) {
	ac := BootstrapAutonomy(map[string]string{})
	if ac.Level != LevelObserve {
		t.Errorf("empty map level: got %d, want 0", ac.Level)
	}
}

func TestBootstrapAutonomy_Level1(t *testing.T) {
	cfg := map[string]string{
		"AUTONOMY_LEVEL":        "1",
		"AUTONOMY_AUTO_RECOVER": "true",
	}
	ac := BootstrapAutonomy(cfg)
	if ac.Level != LevelAutoRecover {
		t.Errorf("level: got %d, want 1", ac.Level)
	}
	if !ac.AutoRecover {
		t.Error("auto-recover should be true")
	}
}

func TestBootstrapAutonomy_ClampsHighLevel(t *testing.T) {
	cfg := map[string]string{
		"AUTONOMY_LEVEL": "3",
	}
	ac := BootstrapAutonomy(cfg)
	if ac.Level != LevelAutoRecover {
		t.Errorf("clamped level: got %d, want 1 (max for bootstrap)", ac.Level)
	}
}

func TestBootstrapAutonomy_MaxRecoveries(t *testing.T) {
	cfg := map[string]string{
		"AUTONOMY_AUTO_RECOVER_MAX": "5",
	}
	ac := BootstrapAutonomy(cfg)
	if ac.MaxRecoveries != 5 {
		t.Errorf("max recoveries: got %d, want 5", ac.MaxRecoveries)
	}
}

func TestBootstrapAutonomy_InvalidValues(t *testing.T) {
	cfg := map[string]string{
		"AUTONOMY_LEVEL":            "notanumber",
		"AUTONOMY_AUTO_RECOVER":     "maybe",
		"AUTONOMY_AUTO_RECOVER_MAX": "-1",
	}
	ac := BootstrapAutonomy(cfg)
	// Invalid level falls back to default 0
	if ac.Level != LevelObserve {
		t.Errorf("invalid level: got %d, want 0", ac.Level)
	}
	// Invalid bool falls back to false
	if ac.AutoRecover {
		t.Error("invalid auto-recover should be false")
	}
	// Invalid max falls back to default 3
	if ac.MaxRecoveries != 3 {
		t.Errorf("invalid max recoveries: got %d, want 3", ac.MaxRecoveries)
	}
}

func TestShouldRecover_Level0(t *testing.T) {
	ac := &AutonomyConfig{Level: LevelObserve, AutoRecover: true, MaxRecoveries: 3}
	if ac.ShouldRecover(0) {
		t.Error("level 0 should not auto-recover")
	}
}

func TestShouldRecover_Level1_Enabled(t *testing.T) {
	ac := &AutonomyConfig{Level: LevelAutoRecover, AutoRecover: true, MaxRecoveries: 3}
	if !ac.ShouldRecover(0) {
		t.Error("level 1 with auto-recover should allow recovery at count 0")
	}
	if !ac.ShouldRecover(2) {
		t.Error("level 1 with auto-recover should allow recovery at count 2")
	}
	if ac.ShouldRecover(3) {
		t.Error("level 1 should not recover at max count")
	}
	if ac.ShouldRecover(5) {
		t.Error("level 1 should not recover beyond max count")
	}
}

func TestShouldRecover_Level1_Disabled(t *testing.T) {
	ac := &AutonomyConfig{Level: LevelAutoRecover, AutoRecover: false, MaxRecoveries: 3}
	if ac.ShouldRecover(0) {
		t.Error("auto-recover disabled should not allow recovery")
	}
}

func TestRecoveryBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 30 * time.Second},
		{1, 60 * time.Second},
		{2, 120 * time.Second},
		{3, 240 * time.Second},
	}
	for _, tt := range tests {
		got := RecoveryBackoff(tt.attempt)
		if got != tt.want {
			t.Errorf("RecoveryBackoff(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
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
