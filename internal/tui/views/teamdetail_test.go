package views

import (
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestRenderTeamDetail_Nil(t *testing.T) {
	out := RenderTeamDetail(nil, nil, 100)
	if !strings.Contains(out, "No team selected") {
		t.Errorf("nil team: expected 'No team selected', got: %q", out)
	}
}

func TestRenderTeamDetail_BasicFields(t *testing.T) {
	team := &session.TeamStatus{
		Name:      "backend-team",
		RepoPath:  "/home/user/my-project",
		LeadID:    "lead-abcdef12",
		Status:    session.StatusRunning,
		CreatedAt: time.Now(),
		Tasks: []session.TeamTask{
			{Description: "implement auth", Status: "completed", Provider: session.ProviderClaude},
			{Description: "add tests", Status: "in-progress"},
			{Description: "deploy", Status: "pending"},
		},
	}

	lead := &session.Session{
		ID:       "lead-abcdef12",
		Provider: session.ProviderClaude,
		Status:   session.StatusRunning,
		Model:    "opus-4",
		SpentUSD: 2.50,
	}

	out := RenderTeamDetail(team, lead, 120)

	checks := []string{
		"backend-team",
		"my-project",
		"Team Info",
		"Lead Session",
		"Tasks",
		"implement auth",
		"add tests",
		"deploy",
		"opus-4",
		"$2.50",
		"Esc: back",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderTeamDetail_NoLeadSession(t *testing.T) {
	team := &session.TeamStatus{
		Name:     "orphan-team",
		RepoPath: "/tmp/repo",
		LeadID:   "missing-lead",
		Status:   session.StatusRunning,
	}

	out := RenderTeamDetail(team, nil, 100)
	if !strings.Contains(out, "not found") {
		t.Error("should show 'not found' when lead session is nil")
	}
	if !strings.Contains(out, "missing-lead") {
		t.Error("should show the missing lead ID")
	}
}

func TestRenderTeamDetail_NoTasks(t *testing.T) {
	team := &session.TeamStatus{
		Name:     "empty-team",
		RepoPath: "/tmp/repo",
		LeadID:   "lead123",
		Status:   session.StatusRunning,
		Tasks:    nil,
	}

	out := RenderTeamDetail(team, nil, 100)
	if !strings.Contains(out, "No tasks") {
		t.Error("should show 'No tasks' when tasks list is empty")
	}
}

func TestRenderTeamDetail_ProgressGauge(t *testing.T) {
	team := &session.TeamStatus{
		Name:     "progress-team",
		RepoPath: "/tmp/repo",
		LeadID:   "lead123",
		Status:   session.StatusRunning,
		Tasks: []session.TeamTask{
			{Description: "task a", Status: "completed"},
			{Description: "task b", Status: "completed"},
			{Description: "task c", Status: "pending"},
		},
	}

	out := RenderTeamDetail(team, nil, 100)
	// Should show progress as 2/3
	if !strings.Contains(out, "2/3") {
		t.Error("should show 2/3 tasks progress")
	}
}

func TestRenderTeamDetail_TaskProviders(t *testing.T) {
	team := &session.TeamStatus{
		Name:     "multi-provider",
		RepoPath: "/tmp/repo",
		LeadID:   "lead123",
		Status:   session.StatusRunning,
		Tasks: []session.TeamTask{
			{Description: "claude task", Status: "pending", Provider: session.ProviderClaude},
			{Description: "gemini task", Status: "pending", Provider: session.ProviderGemini},
		},
	}

	out := RenderTeamDetail(team, nil, 120)
	if !strings.Contains(out, "claude") {
		t.Error("should show claude provider for task")
	}
	if !strings.Contains(out, "gemini") {
		t.Error("should show gemini provider for task")
	}
}

func TestTaskIndicator(t *testing.T) {
	tests := []struct {
		status string
	}{
		{"completed"},
		{"in-progress"},
		{"pending"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := taskIndicator(tt.status)
			if got == "" {
				t.Errorf("taskIndicator(%q) returned empty string", tt.status)
			}
		})
	}
}
