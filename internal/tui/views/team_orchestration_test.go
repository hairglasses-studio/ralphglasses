package views

import (
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Compile-time interface check.
var _ View = (*TeamOrchestrationView)(nil)

func TestRenderTeamOrchestration_Nil(t *testing.T) {
	out := RenderTeamOrchestration(nil, nil, nil, 100, 40)
	if !strings.Contains(out, "No team selected") {
		t.Errorf("nil team: expected 'No team selected', got: %q", out)
	}
}

func TestRenderTeamOrchestration_EmptyTeam(t *testing.T) {
	team := &session.TeamStatus{
		Name:     "empty-team",
		RepoPath: "/tmp/repo",
		LeadID:   "lead123",
		Status:   session.StatusRunning,
		Tasks:    nil,
	}

	out := RenderTeamOrchestration(team, nil, nil, 100, 40)

	checks := []string{
		"Team Orchestration",
		"empty-team",
		"Team Composition",
		"No agents assigned",
		"Delegation Feed",
		"No delegations yet",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderTeamOrchestration_WithAgents(t *testing.T) {
	team := &session.TeamStatus{
		Name:     "backend-team",
		RepoPath: "/home/user/project",
		LeadID:   "lead-abc",
		Status:   session.StatusRunning,
		Tasks: []session.TeamTask{
			{Description: "implement auth", Status: "completed", Provider: session.ProviderClaude},
			{Description: "add tests", Status: "in-progress", Provider: session.ProviderGemini},
			{Description: "deploy", Status: "pending"},
		},
	}

	lead := &session.Session{
		ID:       "lead-abc",
		Provider: session.ProviderClaude,
		Status:   session.StatusRunning,
		Model:    "opus-4",
		SpentUSD: 2.50,
	}

	out := RenderTeamOrchestration(team, lead, nil, 120, 40)

	checks := []string{
		"backend-team",
		"Team Composition",
		"lead",
		"orchestrator",
		"opus-4",
		"implement auth",
		"add tests",
		"deploy",
		"agent-1",
		"agent-2",
		"agent-3",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}

	// Verify tree drawing characters
	if !strings.Contains(out, "\u251c") && !strings.Contains(out, "\u2514") {
		t.Error("expected box-drawing characters in tree")
	}
}

func TestRenderTeamOrchestration_DelegationFeed(t *testing.T) {
	team := &session.TeamStatus{
		Name:     "feed-team",
		RepoPath: "/tmp/repo",
		LeadID:   "lead123",
		Status:   session.StatusRunning,
	}

	delegations := []DelegationEntry{
		{
			Timestamp: time.Date(2026, 3, 29, 14, 30, 0, 0, time.UTC),
			AgentName: "agent-1",
			Task:      "Run unit tests",
			Status:    "completed",
		},
		{
			Timestamp: time.Date(2026, 3, 29, 14, 31, 0, 0, time.UTC),
			AgentName: "agent-2",
			Task:      "Fix flaky test in auth module",
			Status:    "in-progress",
		},
		{
			Timestamp: time.Date(2026, 3, 29, 14, 32, 0, 0, time.UTC),
			AgentName: "agent-3",
			Task:      "Deploy to staging",
			Status:    "pending",
		},
	}

	out := RenderTeamOrchestration(team, nil, delegations, 120, 40)

	checks := []string{
		"Delegation Feed",
		"14:30:00",
		"agent-1",
		"Run unit tests",
		"completed",
		"agent-2",
		"Fix flaky test",
		"in-progress",
		"agent-3",
		"Deploy to staging",
		"pending",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestTeamOrchestrationView_SetDimensions_Regenerates(t *testing.T) {
	v := NewTeamOrchestrationView()
	team := &session.TeamStatus{
		Name:   "dim-test",
		Status: session.StatusRunning,
	}
	v.SetTeam(team, nil)
	v.SetDimensions(120, 40)
	out := v.Render()
	if out == "" {
		t.Error("expected non-empty render after SetDimensions")
	}
	if !strings.Contains(out, "dim-test") {
		t.Error("expected team name in render output after SetDimensions")
	}
}

func TestTeamOrchestrationView_NilTeam(t *testing.T) {
	v := NewTeamOrchestrationView()
	v.SetDimensions(80, 30)
	// No data set -- should not panic
	out := v.Render()
	_ = out
}

func TestTeamOrchestrationView_SetDelegations(t *testing.T) {
	v := NewTeamOrchestrationView()
	v.SetDimensions(100, 40)
	team := &session.TeamStatus{
		Name:   "deleg-test",
		Status: session.StatusRunning,
	}
	v.SetTeam(team, nil)
	v.SetDelegations([]DelegationEntry{
		{
			Timestamp: time.Now(),
			AgentName: "worker-1",
			Task:      "compile project",
			Status:    "completed",
		},
	})
	out := v.Render()
	if !strings.Contains(out, "worker-1") {
		t.Error("expected agent name in delegation feed")
	}
	if !strings.Contains(out, "compile project") {
		t.Error("expected task description in delegation feed")
	}
}

func TestTaskOrchestrationIcon(t *testing.T) {
	statuses := []string{"completed", "in-progress", "failed", "pending", ""}
	for _, s := range statuses {
		got := taskOrchestrationIcon(s)
		if got == "" {
			t.Errorf("taskOrchestrationIcon(%q) returned empty string", s)
		}
	}
}
