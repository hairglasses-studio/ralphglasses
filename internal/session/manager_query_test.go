package session

import (
	"testing"
)

func TestGetWorkflowRun_Found(t *testing.T) {
	m := NewManager()

	// Inject a workflow run
	run := &WorkflowRun{
		ID:     "wf-123",
		Name:   "test-workflow",
		Status: "completed",
	}
	m.mu.Lock()
	m.workflowRuns["wf-123"] = run
	m.mu.Unlock()

	got, ok := m.GetWorkflowRun("wf-123")
	if !ok {
		t.Fatal("expected workflow run to be found")
	}
	if got.Name != "test-workflow" {
		t.Errorf("name = %q, want test-workflow", got.Name)
	}
	if got.Status != "completed" {
		t.Errorf("status = %q, want completed", got.Status)
	}
}

func TestGetWorkflowRun_NotFound(t *testing.T) {
	m := NewManager()
	_, ok := m.GetWorkflowRun("nonexistent")
	if ok {
		t.Error("expected workflow run not found")
	}
}

func TestIsRunning_MultipleRepos(t *testing.T) {
	m := NewManager()

	m.mu.Lock()
	m.sessions["s1"] = &Session{ID: "s1", RepoPath: "/tmp/repo-a", Status: StatusRunning}
	m.sessions["s2"] = &Session{ID: "s2", RepoPath: "/tmp/repo-b", Status: StatusCompleted}
	m.sessions["s3"] = &Session{ID: "s3", RepoPath: "/tmp/repo-a", Status: StatusCompleted}
	m.mu.Unlock()

	if !m.IsRunning("/tmp/repo-a") {
		t.Error("expected repo-a to have a running session")
	}
	if m.IsRunning("/tmp/repo-b") {
		t.Error("expected repo-b to not be running (completed)")
	}
	if m.IsRunning("/tmp/repo-c") {
		t.Error("expected repo-c to not be running (no sessions)")
	}
}

func TestIsRunning_LaunchingStatus(t *testing.T) {
	m := NewManager()
	m.mu.Lock()
	m.sessions["s1"] = &Session{ID: "s1", RepoPath: "/tmp/repo", Status: StatusLaunching}
	m.mu.Unlock()

	if !m.IsRunning("/tmp/repo") {
		t.Error("expected launching session to count as running")
	}
}
