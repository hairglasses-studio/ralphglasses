package session

import (
	"testing"
)

func TestDetectOwnedPathDrift(t *testing.T) {
	tests := []struct {
		name         string
		ownedPaths   []string
		changedFiles []string
		wantDrift    bool
	}{
		{
			name:         "exact match",
			ownedPaths:   []string{"main.go"},
			changedFiles: []string{"main.go"},
			wantDrift:    false,
		},
		{
			name:         "directory match",
			ownedPaths:   []string{"internal/"},
			changedFiles: []string{"internal/session/team.go"},
			wantDrift:    false,
		},
		{
			name:         "escape",
			ownedPaths:   []string{"cmd/"},
			changedFiles: []string{"main.go"},
			wantDrift:    true,
		},
		{
			name:         "empty owned paths ignores drift",
			ownedPaths:   []string{},
			changedFiles: []string{"main.go"},
			wantDrift:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drift := detectOwnedPathDrift(tt.ownedPaths, tt.changedFiles)
			hasDrift := drift != ""
			if hasDrift != tt.wantDrift {
				t.Errorf("detectOwnedPathDrift() hasDrift = %v, want %v. Drift string: %s", hasDrift, tt.wantDrift, drift)
			}
		})
	}
}

func TestApplyWorkerResult_BlocksOnDrift(t *testing.T) {
	m := NewManager()
	teamName := "test-drift-team"
	
	m.workersMu.Lock()
	m.teams[teamName] = &TeamStatus{
		Name: teamName,
		Status: StatusRunning,
		Tasks: []TeamTask{
			{
				ID: "task-1",
				Status: TeamTaskInProgress,
				OwnedPaths: []string{"allowed/"},
			},
		},
	}
	m.workersMu.Unlock()

	result := TeamWorkerResult{
		TaskID: "task-1",
		Status: TeamTaskCompleted,
		Summary: "Did the work",
		ChangedFiles: []string{"outside.go"},
	}

	m.applyWorkerResult(teamName, result, TeamWorkerHandle{})

	m.workersMu.Lock()
	task := m.teams[teamName].Tasks[0]
	m.workersMu.Unlock()

	if task.Status != TeamTaskBlocked {
		t.Errorf("task.Status = %q, want %q", task.Status, TeamTaskBlocked)
	}
	if task.OwnershipDrift == "" {
		t.Error("task.OwnershipDrift is empty, want drift message")
	}
}
