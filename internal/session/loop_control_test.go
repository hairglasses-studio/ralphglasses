package session

import (
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

func TestPauseLoop_Success(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	run := &LoopRun{
		ID:        "pause-loop",
		RepoPath:  "/tmp/repo",
		Status:    "running",
		Paused:    false,
		UpdatedAt: time.Now().Add(-1 * time.Minute),
	}
	m.mu.Lock()
	m.loops["pause-loop"] = run
	m.mu.Unlock()

	err := m.PauseLoop("pause-loop")
	if err != nil {
		t.Fatalf("PauseLoop: %v", err)
	}

	run.mu.Lock()
	paused := run.Paused
	updated := run.UpdatedAt
	run.mu.Unlock()

	if !paused {
		t.Error("expected Paused to be true after PauseLoop")
	}
	if updated.Before(time.Now().Add(-1 * time.Second)) {
		t.Error("expected UpdatedAt to be refreshed")
	}
}

func TestPauseLoop_NotFound(t *testing.T) {
	m := NewManager()
	m.SetStateDir("")

	err := m.PauseLoop("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent loop")
	}
}

func TestResumeLoop_Success(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	run := &LoopRun{
		ID:        "resume-loop",
		RepoPath:  "/tmp/repo",
		Status:    "running",
		Paused:    true,
		UpdatedAt: time.Now().Add(-1 * time.Minute),
	}
	m.mu.Lock()
	m.loops["resume-loop"] = run
	m.mu.Unlock()

	err := m.ResumeLoop("resume-loop")
	if err != nil {
		t.Fatalf("ResumeLoop: %v", err)
	}

	run.mu.Lock()
	paused := run.Paused
	run.mu.Unlock()

	if paused {
		t.Error("expected Paused to be false after ResumeLoop")
	}
}

func TestResumeLoop_NotFound(t *testing.T) {
	m := NewManager()
	m.SetStateDir("")

	err := m.ResumeLoop("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent loop")
	}
}

func TestGetLoop_NotFound(t *testing.T) {
	m := NewManager()
	m.SetStateDir("")

	_, ok := m.GetLoop("nonexistent")
	if ok {
		t.Error("expected GetLoop to return false for nonexistent loop")
	}
}

func TestGetLoop_Found(t *testing.T) {
	m := NewManager()
	m.SetStateDir("")

	run := &LoopRun{ID: "found-loop", Status: "running"}
	m.mu.Lock()
	m.loops["found-loop"] = run
	m.mu.Unlock()

	got, ok := m.GetLoop("found-loop")
	if !ok {
		t.Fatal("expected loop to be found")
	}
	if got.ID != "found-loop" {
		t.Errorf("ID = %q", got.ID)
	}
}

func TestListLoops_Empty(t *testing.T) {
	m := NewManager()
	m.SetStateDir("")

	loops := m.ListLoops()
	if len(loops) != 0 {
		t.Errorf("expected 0 loops, got %d", len(loops))
	}
}

func TestListLoops_Multiple(t *testing.T) {
	m := NewManager()
	m.SetStateDir("")

	m.mu.Lock()
	m.loops["loop-1"] = &LoopRun{ID: "loop-1", Status: "running"}
	m.loops["loop-2"] = &LoopRun{ID: "loop-2", Status: "completed"}
	m.mu.Unlock()

	loops := m.ListLoops()
	if len(loops) != 2 {
		t.Errorf("expected 2 loops, got %d", len(loops))
	}
}

func TestUpdateLoopIteration_Basic(t *testing.T) {
	m := NewManager()
	run := &LoopRun{
		ID:     "iter-test",
		Status: "pending",
		Iterations: []LoopIteration{
			{Number: 1, Status: "pending"},
			{Number: 2, Status: "pending"},
		},
	}

	m.updateLoopIteration(run, 0, "running", func(iter *LoopIteration, r *LoopRun) {
		iter.WorkerSessionID = "sess-1"
	})

	run.mu.Lock()
	if run.Iterations[0].Status != "running" {
		t.Errorf("iteration 0 status = %q, want running", run.Iterations[0].Status)
	}
	if run.Iterations[0].WorkerSessionID != "sess-1" {
		t.Errorf("session ID = %q, want sess-1", run.Iterations[0].WorkerSessionID)
	}
	if run.Status != "running" {
		t.Errorf("run status = %q, want running", run.Status)
	}
	run.mu.Unlock()
}

func TestUpdateLoopIteration_OutOfBounds(t *testing.T) {
	m := NewManager()
	run := &LoopRun{
		ID:         "bounds-test",
		Iterations: []LoopIteration{},
	}

	// Should not panic
	m.updateLoopIteration(run, 5, "running", nil)
	m.updateLoopIteration(run, -1, "running", nil)
}

func TestUpdateLoopIteration_NilMutate(t *testing.T) {
	m := NewManager()
	run := &LoopRun{
		ID:         "nil-mutate",
		Status:     "pending",
		Iterations: []LoopIteration{{Number: 1, Status: "pending"}},
	}

	m.updateLoopIteration(run, 0, "completed", nil)

	run.mu.Lock()
	if run.Iterations[0].Status != "completed" {
		t.Errorf("status = %q, want completed", run.Iterations[0].Status)
	}
	run.mu.Unlock()
}

func TestEffectiveSessionTimeout(t *testing.T) {
	m := NewManager()

	// Default
	if d := m.effectiveSessionTimeout(); d != 10*time.Minute {
		t.Errorf("default timeout = %v, want 10m", d)
	}

	// Custom
	m.SessionTimeout = 5 * time.Minute
	if d := m.effectiveSessionTimeout(); d != 5*time.Minute {
		t.Errorf("custom timeout = %v, want 5m", d)
	}

	// Zero uses default
	m.SessionTimeout = 0
	if d := m.effectiveSessionTimeout(); d != 10*time.Minute {
		t.Errorf("zero timeout = %v, want 10m (default)", d)
	}

	// Negative uses default
	m.SessionTimeout = -1
	if d := m.effectiveSessionTimeout(); d != 10*time.Minute {
		t.Errorf("negative timeout = %v, want 10m (default)", d)
	}
}

func TestEffectiveKillTimeout(t *testing.T) {
	m := NewManager()

	// Default
	if d := m.effectiveKillTimeout(); d != 5*time.Second {
		t.Errorf("default kill timeout = %v, want 5s", d)
	}

	// Custom
	m.KillTimeout = 10 * time.Second
	if d := m.effectiveKillTimeout(); d != 10*time.Second {
		t.Errorf("custom kill timeout = %v, want 10s", d)
	}
}

func TestKillTimeoutFromConfig(t *testing.T) {
	// Config value is applied correctly.
	m := NewManager()
	cfg := &model.RalphConfig{Values: map[string]string{"KILL_ESCALATION_TIMEOUT": "15"}}
	m.ApplyConfig(cfg)
	if d := m.effectiveKillTimeout(); d != 15*time.Second {
		t.Errorf("config kill timeout = %v, want 15s", d)
	}

	// Minimum boundary: 1s is valid.
	m2 := NewManager()
	cfg2 := &model.RalphConfig{Values: map[string]string{"KILL_ESCALATION_TIMEOUT": "1"}}
	m2.ApplyConfig(cfg2)
	if d := m2.effectiveKillTimeout(); d != 1*time.Second {
		t.Errorf("min boundary kill timeout = %v, want 1s", d)
	}

	// Maximum boundary: 60s is valid.
	m3 := NewManager()
	cfg3 := &model.RalphConfig{Values: map[string]string{"KILL_ESCALATION_TIMEOUT": "60"}}
	m3.ApplyConfig(cfg3)
	if d := m3.effectiveKillTimeout(); d != 60*time.Second {
		t.Errorf("max boundary kill timeout = %v, want 60s", d)
	}

	// Below minimum: 0 is rejected, falls back to default.
	m4 := NewManager()
	cfg4 := &model.RalphConfig{Values: map[string]string{"KILL_ESCALATION_TIMEOUT": "0"}}
	m4.ApplyConfig(cfg4)
	if d := m4.effectiveKillTimeout(); d != 5*time.Second {
		t.Errorf("below-min kill timeout = %v, want 5s (default)", d)
	}

	// Above maximum: 61 is rejected, falls back to default.
	m5 := NewManager()
	cfg5 := &model.RalphConfig{Values: map[string]string{"KILL_ESCALATION_TIMEOUT": "61"}}
	m5.ApplyConfig(cfg5)
	if d := m5.effectiveKillTimeout(); d != 5*time.Second {
		t.Errorf("above-max kill timeout = %v, want 5s (default)", d)
	}

	// Non-integer value is rejected, falls back to default.
	m6 := NewManager()
	cfg6 := &model.RalphConfig{Values: map[string]string{"KILL_ESCALATION_TIMEOUT": "abc"}}
	m6.ApplyConfig(cfg6)
	if d := m6.effectiveKillTimeout(); d != 5*time.Second {
		t.Errorf("non-integer kill timeout = %v, want 5s (default)", d)
	}

	// Nil config is safe.
	m7 := NewManager()
	m7.ApplyConfig(nil)
	if d := m7.effectiveKillTimeout(); d != 5*time.Second {
		t.Errorf("nil config kill timeout = %v, want 5s (default)", d)
	}
}

func TestLoopStateDir(t *testing.T) {
	m := NewManager()

	// Empty state dir
	m.SetStateDir("")
	if got := m.loopStateDir(); got != "" {
		t.Errorf("loopStateDir with empty stateDir = %q, want empty", got)
	}

	// Normal state dir
	m.SetStateDir("/tmp/sessions")
	if got := m.loopStateDir(); got != "/tmp/sessions/loops" {
		t.Errorf("loopStateDir = %q, want /tmp/sessions/loops", got)
	}
}
