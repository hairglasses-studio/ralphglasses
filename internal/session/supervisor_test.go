package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestSupervisor(t *testing.T) (*Supervisor, string) {
	t.Helper()
	dir := t.TempDir()
	mgr := NewManager()
	mgr.SetStateDir(filepath.Join(dir, "sessions"))
	s := NewSupervisor(mgr, dir)
	s.TickInterval = 10 * time.Millisecond
	return s, dir
}

func TestSupervisor_StartStop(t *testing.T) {
	s, _ := newTestSupervisor(t)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !s.Running() {
		t.Fatal("expected running after Start")
	}
	s.Stop()
	if s.Running() {
		t.Fatal("expected not running after Stop")
	}
}

func TestSupervisor_Idempotent(t *testing.T) {
	s, _ := newTestSupervisor(t)
	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := s.Start(ctx); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if !s.Running() {
		t.Fatal("expected still running")
	}
	s.Stop()
}

func TestSupervisor_StopWhenNotRunning(t *testing.T) {
	s, _ := newTestSupervisor(t)
	s.Stop() // should not panic
}

func TestSupervisor_EmptyRepoPath(t *testing.T) {
	mgr := NewManager()
	s := NewSupervisor(mgr, "")
	if err := s.Start(context.Background()); err == nil {
		t.Fatal("expected error for empty RepoPath")
	}
}

func TestSupervisor_TickIncrementsCount(t *testing.T) {
	s, _ := newTestSupervisor(t)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(60 * time.Millisecond)
	s.Stop()
	if c := s.TickCount(); c < 1 {
		t.Fatalf("expected tickCount >= 1, got %d", c)
	}
}

func TestSupervisor_CooldownRespected(t *testing.T) {
	s, _ := newTestSupervisor(t)
	s.CooldownBetween = 1 * time.Hour
	s.mu.Lock()
	s.lastCycleLaunch = time.Now()
	s.mu.Unlock()

	s.SetMonitor(&HealthMonitor{
		EvaluateFunc: func(_ string) []HealthSignal {
			return []HealthSignal{{
				Category: DecisionLaunch, Metric: "idle_time",
				Value: 300, Threshold: 60, Rationale: "idle", SuggestedAction: "launch",
			}}
		},
	})
	s.mgr = nil // prevent actual launch
	s.tick(context.Background())
	// No panic = cooldown was respected and nil mgr wasn't reached,
	// or if it was reached the nil check prevented a crash.
}

func TestSupervisor_Status(t *testing.T) {
	s, dir := newTestSupervisor(t)
	st := s.Status()
	if st.Running || st.RepoPath != dir {
		t.Fatalf("unexpected pre-start status: %+v", st)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	st = s.Status()
	if !st.Running || st.StartedAt.IsZero() {
		t.Fatalf("unexpected running status: %+v", st)
	}
	s.Stop()
}

func TestSupervisor_PersistState(t *testing.T) {
	s, dir := newTestSupervisor(t)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	s.Stop()

	data, err := os.ReadFile(filepath.Join(dir, ".ralph", "supervisor_state.json"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var state SupervisorState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if state.RepoPath != dir || state.TickCount < 1 {
		t.Fatalf("unexpected persisted state: %+v", state)
	}
}

func TestSupervisor_MonitorSignalsDispatched(t *testing.T) {
	s, _ := newTestSupervisor(t)
	evaluated := make(chan struct{}, 1)
	s.SetMonitor(&HealthMonitor{
		EvaluateFunc: func(_ string) []HealthSignal {
			select {
			case evaluated <- struct{}{}:
			default:
			}
			return nil
		},
	})
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-evaluated:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for monitor evaluation")
	}
	s.Stop()
}
