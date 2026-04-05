package mcpserver

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/blackboard"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestWireSubsystemsPhaseH(t *testing.T) {
	t.Parallel()

	srv := NewServer(t.TempDir())
	ralphDir := t.TempDir()

	wireSubsystems(context.Background(), srv, srv.SessMgr, ralphDir)

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

	// Phase H subsystems should be wired on server.
	if srv.Blackboard == nil {
		t.Error("expected Blackboard to be non-nil on Server")
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

	// Enhancer should be wired on session manager (FINDING-B3).
	// Note: Engine may be nil when no API key is set, but the field should
	// have been assigned (wireSubsystems calls getEngine which uses engineOnce).
	// We just verify the code path ran without panic.
}

func TestWireSubsystemsIdempotent(t *testing.T) {
	t.Parallel()

	srv := NewServer(t.TempDir())
	ralphDir := t.TempDir()

	// Pre-set a blackboard.
	existing := blackboard.NewBlackboard(ralphDir)
	srv.Blackboard = existing

	wireSubsystems(context.Background(), srv, srv.SessMgr, ralphDir)

	// Should not replace existing blackboard.
	if srv.Blackboard != existing {
		t.Error("wireSubsystems should not replace existing Blackboard")
	}
}

func TestWireSubsystemsWithFeedbackAnalyzer(t *testing.T) {
	t.Parallel()

	srv := NewServer(t.TempDir())
	ralphDir := t.TempDir()

	// Pre-set a FeedbackAnalyzer on the server.
	srv.FeedbackAnalyzer = session.NewFeedbackAnalyzer(ralphDir, 3)

	wireSubsystems(context.Background(), srv, srv.SessMgr, ralphDir)

	// CurriculumSorter should be wired (B4 verified by this existing).
	if !srv.SessMgr.HasCurriculumSorter() {
		t.Error("expected CurriculumSorter to be wired even with FeedbackAnalyzer set")
	}
}

func TestRerouteClaudeProviderForCacheHealth(t *testing.T) {
	t.Parallel()

	srv := NewServer(t.TempDir())
	repoPath := t.TempDir()
	now := time.Now()

	for i := 0; i < claudeCacheRerouteThreshold; i++ {
		injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.ID = fmt.Sprintf("sess-%d", i)
			s.Provider = session.ProviderClaude
			s.Status = session.StatusCompleted
			s.Resumed = true
			s.CacheWriteTokens = 1200
			s.LastActivity = now
		})
	}

	got, reason := srv.rerouteClaudeProviderForCacheHealth(repoPath, session.ProviderClaude, false)
	if got != session.ProviderCodex {
		t.Fatalf("rerouted provider = %q, want %q", got, session.ProviderCodex)
	}
	if reason == "" {
		t.Fatal("expected reroute reason")
	}
}
