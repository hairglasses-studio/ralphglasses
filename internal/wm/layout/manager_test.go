package layout

import (
	"fmt"
	"sync"
	"testing"
)

// --- Preset tests ---

func TestDefaultPresets(t *testing.T) {
	presets := DefaultPresets()
	if len(presets) != 3 {
		t.Fatalf("expected 3 default presets, got %d", len(presets))
	}

	names := map[string]bool{}
	for _, p := range presets {
		names[p.Name] = true
		if p.Description == "" {
			t.Errorf("preset %q has empty description", p.Name)
		}
	}

	for _, want := range []string{"single", "dual", "seven"} {
		if !names[want] {
			t.Errorf("missing expected preset %q", want)
		}
	}
}

func TestSingleMonitorPreset(t *testing.T) {
	p := SingleMonitorPreset()
	if len(p.MonitorAssignments) != 1 {
		t.Fatalf("expected 1 monitor assignment, got %d", len(p.MonitorAssignments))
	}
	a := p.MonitorAssignments[0]
	if !a.Primary {
		t.Error("single monitor should be primary")
	}
	if len(a.Workspaces) < 1 {
		t.Error("single monitor preset should have at least 1 workspace")
	}
}

func TestDualMonitorPreset(t *testing.T) {
	p := DualMonitorPreset()
	if len(p.MonitorAssignments) != 2 {
		t.Fatalf("expected 2 monitor assignments, got %d", len(p.MonitorAssignments))
	}
	if !p.MonitorAssignments[0].Primary {
		t.Error("monitor 0 should be primary")
	}
	if p.MonitorAssignments[1].Primary {
		t.Error("monitor 1 should not be primary")
	}
}

func TestSevenMonitorPreset(t *testing.T) {
	p := SevenMonitorPreset()
	if len(p.MonitorAssignments) != 7 {
		t.Fatalf("expected 7 monitor assignments, got %d", len(p.MonitorAssignments))
	}

	primaryCount := 0
	for _, a := range p.MonitorAssignments {
		if a.Primary {
			primaryCount++
		}
		if len(a.Workspaces) < 1 {
			t.Errorf("monitor %d has no workspaces", a.MonitorIndex)
		}
	}
	if primaryCount != 1 {
		t.Errorf("expected exactly 1 primary monitor, got %d", primaryCount)
	}
}

// --- LayoutManager tests ---

func TestNewLayoutManager(t *testing.T) {
	lm := NewLayoutManager(SingleMonitorPreset())
	if lm == nil {
		t.Fatal("NewLayoutManager returned nil")
	}
}

func TestAssignSession_Success(t *testing.T) {
	lm := NewLayoutManager(DualMonitorPreset())

	if err := lm.AssignSession("s1", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := lm.AssignSession("s2", 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	idx, ok := lm.GetMonitorForSession("s1")
	if !ok || idx != 0 {
		t.Errorf("expected s1 on monitor 0, got %d (found=%v)", idx, ok)
	}

	idx, ok = lm.GetMonitorForSession("s2")
	if !ok || idx != 1 {
		t.Errorf("expected s2 on monitor 1, got %d (found=%v)", idx, ok)
	}
}

func TestAssignSession_InvalidMonitor(t *testing.T) {
	lm := NewLayoutManager(SingleMonitorPreset())

	err := lm.AssignSession("s1", 5)
	if err == nil {
		t.Fatal("expected error for invalid monitor index")
	}
}

func TestAssignSession_DuplicateSession(t *testing.T) {
	lm := NewLayoutManager(DualMonitorPreset())

	if err := lm.AssignSession("s1", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err := lm.AssignSession("s1", 1)
	if err == nil {
		t.Fatal("expected error for duplicate session assignment")
	}
}

func TestUnassignSession(t *testing.T) {
	lm := NewLayoutManager(DualMonitorPreset())

	_ = lm.AssignSession("s1", 0)
	_ = lm.AssignSession("s2", 0)

	lm.UnassignSession("s1")

	_, ok := lm.GetMonitorForSession("s1")
	if ok {
		t.Error("s1 should no longer be assigned")
	}

	// s2 should still be there.
	idx, ok := lm.GetMonitorForSession("s2")
	if !ok || idx != 0 {
		t.Errorf("s2 should still be on monitor 0, got %d (found=%v)", idx, ok)
	}

	sessions := lm.GetSessionsOnMonitor(0)
	if len(sessions) != 1 || sessions[0] != "s2" {
		t.Errorf("expected [s2] on monitor 0, got %v", sessions)
	}
}

func TestUnassignSession_NotAssigned(t *testing.T) {
	lm := NewLayoutManager(SingleMonitorPreset())
	// Should be a no-op, not panic.
	lm.UnassignSession("nonexistent")
}

func TestGetMonitorForSession_NotFound(t *testing.T) {
	lm := NewLayoutManager(SingleMonitorPreset())
	_, ok := lm.GetMonitorForSession("missing")
	if ok {
		t.Error("expected false for unassigned session")
	}
}

func TestGetSessionsOnMonitor_Empty(t *testing.T) {
	lm := NewLayoutManager(DualMonitorPreset())
	sessions := lm.GetSessionsOnMonitor(0)
	if sessions != nil {
		t.Errorf("expected nil for empty monitor, got %v", sessions)
	}
}

func TestGetSessionsOnMonitor_Sorted(t *testing.T) {
	lm := NewLayoutManager(SingleMonitorPreset())
	_ = lm.AssignSession("charlie", 0)
	_ = lm.AssignSession("alpha", 0)
	_ = lm.AssignSession("bravo", 0)

	sessions := lm.GetSessionsOnMonitor(0)
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}
	if sessions[0] != "alpha" || sessions[1] != "bravo" || sessions[2] != "charlie" {
		t.Errorf("expected sorted order [alpha bravo charlie], got %v", sessions)
	}
}

func TestApplyLayout_Valid(t *testing.T) {
	lm := NewLayoutManager(DualMonitorPreset())
	_ = lm.AssignSession("s1", 0)
	_ = lm.AssignSession("s2", 1)

	if err := lm.ApplyLayout(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyLayout_ExceedsCapacity(t *testing.T) {
	// Seven-monitor preset has 1 workspace per monitor.
	lm := NewLayoutManager(SevenMonitorPreset())
	_ = lm.AssignSession("s1", 0)
	_ = lm.AssignSession("s2", 0) // exceeds 1 workspace on monitor 0

	err := lm.ApplyLayout()
	if err == nil {
		t.Fatal("expected error when sessions exceed workspace capacity")
	}
}

func TestApplyLayout_Empty(t *testing.T) {
	lm := NewLayoutManager(SingleMonitorPreset())
	if err := lm.ApplyLayout(); err != nil {
		t.Fatalf("unexpected error on empty layout: %v", err)
	}
}

func TestReassignAfterUnassign(t *testing.T) {
	lm := NewLayoutManager(DualMonitorPreset())
	_ = lm.AssignSession("s1", 0)
	lm.UnassignSession("s1")

	// Should be able to reassign to a different monitor.
	if err := lm.AssignSession("s1", 1); err != nil {
		t.Fatalf("unexpected error reassigning after unassign: %v", err)
	}
	idx, ok := lm.GetMonitorForSession("s1")
	if !ok || idx != 1 {
		t.Errorf("expected s1 on monitor 1 after reassign, got %d (found=%v)", idx, ok)
	}
}

// --- Concurrency test ---

func TestConcurrentAccess(t *testing.T) {
	lm := NewLayoutManager(SevenMonitorPreset())

	var wg sync.WaitGroup
	errs := make(chan error, 100)

	// Concurrent assigns to different monitors.
	for i := range 7 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sessionID := fmt.Sprintf("session-%d", idx)
			if err := lm.AssignSession(sessionID, idx); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent assign error: %v", err)
	}

	// Verify all assigned.
	for i := range 7 {
		sessionID := fmt.Sprintf("session-%d", i)
		idx, ok := lm.GetMonitorForSession(sessionID)
		if !ok {
			t.Errorf("session-%d not found after concurrent assign", i)
		}
		if idx != i {
			t.Errorf("session-%d expected monitor %d, got %d", i, i, idx)
		}
	}

	// Concurrent reads while unassigning.
	var wg2 sync.WaitGroup
	for i := range 7 {
		wg2.Add(2)
		go func(idx int) {
			defer wg2.Done()
			lm.GetSessionsOnMonitor(idx)
		}(i)
		go func(idx int) {
			defer wg2.Done()
			lm.UnassignSession(fmt.Sprintf("session-%d", idx))
		}(i)
	}
	wg2.Wait()
}
