package session

import (
	"testing"
)

func TestTeamLeadAllowedTools(t *testing.T) {
	tools := teamLeadAllowedTools()
	if len(tools) == 0 {
		t.Fatal("expected non-empty allowed tools list")
	}

	// Should contain session_launch and session_status at minimum
	wantTools := map[string]bool{
		"mcp__ralphglasses__ralphglasses_session_launch": false,
		"mcp__ralphglasses__ralphglasses_session_status": false,
		"mcp__ralphglasses__ralphglasses_session_list":   false,
		"mcp__ralphglasses__ralphglasses_session_stop":   false,
		"mcp__ralphglasses__ralphglasses_session_output": false,
	}

	for _, tool := range tools {
		if _, ok := wantTools[tool]; ok {
			wantTools[tool] = true
		}
	}

	for tool, found := range wantTools {
		if !found {
			t.Errorf("expected tool %q in allowed tools list", tool)
		}
	}
}

func TestUpdateTeamOnSessionEnd_LeadCompleted(t *testing.T) {
	m := NewManager()

	lead := &Session{
		ID:     "lead-1",
		Status: StatusCompleted,
	}
	m.sessionsMu.Lock()
	m.sessions["lead-1"] = lead
	m.sessionsMu.Unlock()
	m.workersMu.Lock()
	m.teams["my-team"] = &TeamStatus{
		Name:   "my-team",
		LeadID: "lead-1",
		Status: StatusRunning,
		Tasks: []TeamTask{
			{Description: "task 1", Status: "pending"},
			{Description: "task 2", Status: "in-progress"},
			{Description: "task 3", Status: "completed"},
		},
	}
	m.workersMu.Unlock()

	m.updateTeamOnSessionEnd(lead)

	m.workersMu.Lock()
	team := m.teams["my-team"]
	m.workersMu.Unlock()

	if team.Status != StatusCompleted {
		t.Errorf("team status = %q, want completed", team.Status)
	}
	// Pending tasks should be cancelled
	if team.Tasks[0].Status != "cancelled" {
		t.Errorf("task 0 status = %q, want cancelled", team.Tasks[0].Status)
	}
	// In-progress tasks should remain in-progress (not pending)
	if team.Tasks[1].Status != "in-progress" {
		t.Errorf("task 1 status = %q, want in-progress", team.Tasks[1].Status)
	}
	// Completed tasks should remain completed
	if team.Tasks[2].Status != "completed" {
		t.Errorf("task 2 status = %q, want completed", team.Tasks[2].Status)
	}
}

func TestUpdateTeamOnSessionEnd_LeadErrored(t *testing.T) {
	m := NewManager()

	lead := &Session{
		ID:     "lead-err",
		Status: StatusErrored,
	}
	m.sessionsMu.Lock()
	m.sessions["lead-err"] = lead
	m.sessionsMu.Unlock()
	m.workersMu.Lock()
	m.teams["err-team"] = &TeamStatus{
		Name:   "err-team",
		LeadID: "lead-err",
		Status: StatusRunning,
		Tasks: []TeamTask{
			{Description: "task 1", Status: "pending"},
		},
	}
	m.workersMu.Unlock()

	m.updateTeamOnSessionEnd(lead)

	m.workersMu.Lock()
	team := m.teams["err-team"]
	m.workersMu.Unlock()

	if team.Status != StatusErrored {
		t.Errorf("team status = %q, want errored", team.Status)
	}
	if team.Tasks[0].Status != "cancelled" {
		t.Errorf("task 0 status = %q, want cancelled", team.Tasks[0].Status)
	}
}

func TestUpdateTeamOnSessionEnd_NonLeadSession(t *testing.T) {
	m := NewManager()

	worker := &Session{
		ID:     "worker-1",
		Status: StatusCompleted,
	}
	m.sessionsMu.Lock()
	m.sessions["worker-1"] = worker
	m.sessionsMu.Unlock()
	m.workersMu.Lock()
	m.teams["my-team"] = &TeamStatus{
		Name:   "my-team",
		LeadID: "lead-1",
		Status: StatusRunning,
		Tasks: []TeamTask{
			{Description: "task 1", Status: "pending"},
		},
	}
	m.workersMu.Unlock()

	m.updateTeamOnSessionEnd(worker)

	m.workersMu.Lock()
	team := m.teams["my-team"]
	m.workersMu.Unlock()

	// Team should remain running since worker-1 is not the lead
	if team.Status != StatusRunning {
		t.Errorf("team status = %q, want running (non-lead session ended)", team.Status)
	}
	if team.Tasks[0].Status != "pending" {
		t.Errorf("task 0 status = %q, want pending", team.Tasks[0].Status)
	}
}

func TestUpdateTeamOnSessionEnd_NoTeams(t *testing.T) {
	m := NewManager()

	sess := &Session{ID: "orphan", Status: StatusCompleted}
	m.sessionsMu.Lock()
	m.sessions["orphan"] = sess
	m.sessionsMu.Unlock()

	// Should not panic with no teams
	m.updateTeamOnSessionEnd(sess)
}

func TestCorrelateTaskStatuses_ErroredWorker(t *testing.T) {
	m := NewManager()

	m.sessionsMu.Lock()
	m.sessions["lead"] = &Session{ID: "lead", Status: StatusRunning, TeamName: "team-a"}
	m.sessions["w1"] = &Session{
		ID:       "w1",
		Status:   StatusErrored,
		TeamName: "team-a",
		Prompt:   "fix the auth bug",
	}
	m.sessions["w2"] = &Session{
		ID:       "w2",
		Status:   StatusStopped,
		TeamName: "team-a",
		Prompt:   "write docs for API",
	}
	m.sessionsMu.Unlock()
	m.workersMu.Lock()
	team := &TeamStatus{
		Name:   "team-a",
		LeadID: "lead",
		Status: StatusRunning,
		Tasks: []TeamTask{
			{Description: "fix the auth bug", Status: "pending"},
			{Description: "write docs for API", Status: "pending"},
		},
	}
	m.teams["team-a"] = team
	m.workersMu.Unlock()

	// GetTeam triggers correlateTaskStatuses
	got, ok := m.GetTeam("team-a")
	if !ok {
		t.Fatal("team not found")
	}
	if got.Tasks[0].Status != "errored" {
		t.Errorf("task 0 status = %q, want errored", got.Tasks[0].Status)
	}
	if got.Tasks[1].Status != "errored" {
		t.Errorf("task 1 status = %q, want errored (stopped worker)", got.Tasks[1].Status)
	}
}

func TestCorrelateTaskStatuses_TerminalNotOverwritten(t *testing.T) {
	m := NewManager()

	m.sessionsMu.Lock()
	m.sessions["lead"] = &Session{ID: "lead", Status: StatusRunning, TeamName: "team-b"}
	m.sessions["w1"] = &Session{
		ID:       "w1",
		Status:   StatusRunning,
		TeamName: "team-b",
		Prompt:   "refactor auth",
	}
	m.sessionsMu.Unlock()
	m.workersMu.Lock()
	team := &TeamStatus{
		Name:   "team-b",
		LeadID: "lead",
		Status: StatusRunning,
		Tasks: []TeamTask{
			{Description: "refactor auth", Status: "completed"}, // already terminal
		},
	}
	m.teams["team-b"] = team
	m.workersMu.Unlock()

	got, _ := m.GetTeam("team-b")
	// Completed status should not be overwritten by worker running status
	if got.Tasks[0].Status != "completed" {
		t.Errorf("task 0 status = %q, want completed (terminal should not change)", got.Tasks[0].Status)
	}
}

