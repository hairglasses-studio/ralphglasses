package views

import (
	"testing"
)

func TestCoordinationModel_StyledStatus_Default(t *testing.T) {
	m := NewCoordination()
	// "unknown" hits the default branch
	got := m.styledStatus("unknown")
	if got == "" {
		t.Error("styledStatus(unknown) should return non-empty string")
	}
}

func TestCoordinationModel_StyledStatus_AllCases(t *testing.T) {
	m := NewCoordination()
	cases := []string{"running", "waiting", "error", "completed", "other"}
	for _, status := range cases {
		t.Run(status, func(t *testing.T) {
			got := m.styledStatus(status)
			if got == "" {
				t.Errorf("styledStatus(%q) returned empty string", status)
			}
		})
	}
}

func TestCoordinationModel_CountByStatus(t *testing.T) {
	m := NewCoordination()
	m.nodes = []CoordinationNode{
		{ID: "1", Status: "running"},
		{ID: "2", Status: "running"},
		{ID: "3", Status: "waiting"},
		{ID: "4", Status: "error"},
		{ID: "5", Status: "completed"},
		{ID: "6", Status: "completed"},
		{ID: "7", Status: "unknown"}, // not counted in any category
	}
	running, waiting, errored, completed := m.countByStatus()
	if running != 2 {
		t.Errorf("running = %d, want 2", running)
	}
	if waiting != 1 {
		t.Errorf("waiting = %d, want 1", waiting)
	}
	if errored != 1 {
		t.Errorf("errored = %d, want 1", errored)
	}
	if completed != 2 {
		t.Errorf("completed = %d, want 2", completed)
	}
}
