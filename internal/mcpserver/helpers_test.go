package mcpserver

import (
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestWireSubsystemsPhaseH(t *testing.T) {
	t.Parallel()

	srv := NewServer(t.TempDir())
	ralphDir := t.TempDir()

	wireSubsystems(srv, srv.SessMgr, ralphDir)

	// Phase G subsystems should be wired on the session manager.
	if !srv.SessMgr.HasReflexion() {
		t.Error("expected ReflexionStore to be wired")
	}
	if !srv.SessMgr.HasEpisodicMemory() {
		t.Error("expected EpisodicMemory to be wired")
	}
	if !srv.SessMgr.HasCascadeRouter() {
		t.Error("expected CascadeRouter to be wired")
	}
	if !srv.SessMgr.HasCurriculumSorter() {
		t.Error("expected CurriculumSorter to be wired")
	}

	// Phase H subsystems should be wired on both server and manager.
	if srv.Blackboard == nil {
		t.Error("expected Blackboard to be non-nil on Server")
	}
	if !srv.SessMgr.HasBlackboard() {
		t.Error("expected Blackboard to be wired on Manager")
	}
	if srv.CostPredictor == nil {
		t.Error("expected CostPredictor to be non-nil on Server")
	}
	if !srv.SessMgr.HasCostPredictor() {
		t.Error("expected CostPredictor to be wired on Manager")
	}

	// Bandit should be wired on server.
	if srv.Bandit == nil {
		t.Error("expected Bandit to be non-nil on Server")
	}
	if srv.Bandit.ArmCount() == 0 {
		t.Error("expected Bandit to have arms")
	}
}

func TestWireSubsystemsIdempotent(t *testing.T) {
	t.Parallel()

	srv := NewServer(t.TempDir())
	ralphDir := t.TempDir()

	// Pre-set a blackboard.
	existing := session.NewBlackboard(ralphDir)
	existing.Put("test_key", "test_value", "test")
	srv.Blackboard = existing

	wireSubsystems(srv, srv.SessMgr, ralphDir)

	// Should not replace existing blackboard.
	if srv.Blackboard != existing {
		t.Error("wireSubsystems should not replace existing Blackboard")
	}
	val, ok := srv.Blackboard.Get("test_key")
	if !ok || val != "test_value" {
		t.Error("existing blackboard data should be preserved")
	}
}

func TestWireSubsystemsWithFeedbackAnalyzer(t *testing.T) {
	t.Parallel()

	srv := NewServer(t.TempDir())
	ralphDir := t.TempDir()

	// Pre-set a FeedbackAnalyzer on the server.
	srv.FeedbackAnalyzer = session.NewFeedbackAnalyzer(ralphDir, 3)

	wireSubsystems(srv, srv.SessMgr, ralphDir)

	// CurriculumSorter should be wired (B4 verified by this existing).
	if !srv.SessMgr.HasCurriculumSorter() {
		t.Error("expected CurriculumSorter to be wired even with FeedbackAnalyzer set")
	}
}
