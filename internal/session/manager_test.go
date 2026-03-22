package session

import (
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if len(m.sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(m.sessions))
	}
	if len(m.teams) != 0 {
		t.Errorf("expected 0 teams, got %d", len(m.teams))
	}
}

func TestManagerListEmpty(t *testing.T) {
	m := NewManager()
	sessions := m.List("")
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestManagerGetNotFound(t *testing.T) {
	m := NewManager()
	_, ok := m.Get("nonexistent")
	if ok {
		t.Error("expected not found")
	}
}

func TestManagerStopNotFound(t *testing.T) {
	m := NewManager()
	err := m.Stop("nonexistent")
	if err == nil {
		t.Error("expected error stopping nonexistent session")
	}
}

func TestManagerIsRunningEmpty(t *testing.T) {
	m := NewManager()
	if m.IsRunning("/tmp/repo") {
		t.Error("expected not running for empty manager")
	}
}

func TestManagerGetTeamNotFound(t *testing.T) {
	m := NewManager()
	_, ok := m.GetTeam("nonexistent")
	if ok {
		t.Error("expected team not found")
	}
}

func TestManagerListTeamsEmpty(t *testing.T) {
	m := NewManager()
	teams := m.ListTeams()
	if len(teams) != 0 {
		t.Errorf("expected 0 teams, got %d", len(teams))
	}
}

func TestManagerStopAlreadyStopped(t *testing.T) {
	m := NewManager()

	// Manually add a stopped session
	s := &Session{
		ID:     "test-session",
		Status: StatusCompleted,
	}
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()

	err := m.Stop(s.ID)
	if err == nil {
		t.Error("expected error stopping completed session")
	}
}

func TestManagerFindByRepo(t *testing.T) {
	m := NewManager()

	s := &Session{
		ID:       "test-session",
		RepoPath: "/home/user/projects/myrepo",
		RepoName: "myrepo",
		Status:   StatusRunning,
	}
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()

	found := m.FindByRepo("myrepo")
	if len(found) != 1 {
		t.Fatalf("expected 1 session, got %d", len(found))
	}
	if found[0].ID != "test-session" {
		t.Errorf("found[0].ID = %q, want test-session", found[0].ID)
	}

	notFound := m.FindByRepo("other")
	if len(notFound) != 0 {
		t.Errorf("expected 0 sessions for other repo, got %d", len(notFound))
	}
}

func TestManagerWithBus(t *testing.T) {
	bus := events.NewBus(100)
	m := NewManagerWithBus(bus)
	if m == nil {
		t.Fatal("NewManagerWithBus returned nil")
	}
	if m.bus != bus {
		t.Error("bus not wired")
	}
}

func TestManagerSessionLifecycle(t *testing.T) {
	m := NewManager()

	// Manually inject a session to test lifecycle without spawning a real process
	s := &Session{
		ID:           "lifecycle-test",
		Provider:     ProviderClaude,
		RepoPath:     "/tmp/test-repo",
		RepoName:     "test-repo",
		Status:       StatusRunning,
		Model:        "sonnet",
		BudgetUSD:    10.0,
		LaunchedAt:   time.Now(),
		LastActivity: time.Now(),
	}
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()

	// Get returns the session
	got, ok := m.Get("lifecycle-test")
	if !ok {
		t.Fatal("session not found after insertion")
	}
	if got.Status != StatusRunning {
		t.Errorf("status = %q, want running", got.Status)
	}
	if got.Provider != ProviderClaude {
		t.Errorf("provider = %q, want claude", got.Provider)
	}

	// List returns it
	all := m.List("")
	if len(all) != 1 {
		t.Fatalf("List() = %d sessions, want 1", len(all))
	}

	// List with matching repo path
	filtered := m.List("/tmp/test-repo")
	if len(filtered) != 1 {
		t.Errorf("List(matching) = %d, want 1", len(filtered))
	}

	// List with non-matching repo path
	filtered = m.List("/tmp/other")
	if len(filtered) != 0 {
		t.Errorf("List(non-matching) = %d, want 0", len(filtered))
	}

	// IsRunning
	if !m.IsRunning("/tmp/test-repo") {
		t.Error("expected IsRunning=true for running session")
	}

	// Stop (no process to kill, but status should change)
	if err := m.Stop("lifecycle-test"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if s.Status != StatusStopped {
		t.Errorf("status after stop = %q, want stopped", s.Status)
	}

	// IsRunning should be false after stop
	if m.IsRunning("/tmp/test-repo") {
		t.Error("expected IsRunning=false after stop")
	}

	// Stop again should error
	if err := m.Stop("lifecycle-test"); err == nil {
		t.Error("expected error stopping already-stopped session")
	}
}

func TestManagerStopAll(t *testing.T) {
	m := NewManager()

	for _, id := range []string{"s1", "s2", "s3"} {
		s := &Session{
			ID:       id,
			Status:   StatusRunning,
			RepoPath: "/tmp/repo",
		}
		m.mu.Lock()
		m.sessions[id] = s
		m.mu.Unlock()
	}

	// Add one completed session that should not be affected
	m.mu.Lock()
	m.sessions["s4"] = &Session{ID: "s4", Status: StatusCompleted, RepoPath: "/tmp/repo"}
	m.mu.Unlock()

	m.StopAll()

	for _, id := range []string{"s1", "s2", "s3"} {
		s, _ := m.Get(id)
		if s.Status != StatusStopped {
			t.Errorf("session %s status = %q, want stopped", id, s.Status)
		}
	}

	// Completed session should remain completed
	s4, _ := m.Get("s4")
	if s4.Status != StatusCompleted {
		t.Errorf("session s4 status = %q, want completed (unchanged)", s4.Status)
	}
}

func TestManagerListFiltersByProvider(t *testing.T) {
	m := NewManager()

	m.mu.Lock()
	m.sessions["claude-1"] = &Session{ID: "claude-1", Provider: ProviderClaude, RepoPath: "/tmp/a"}
	m.sessions["gemini-1"] = &Session{ID: "gemini-1", Provider: ProviderGemini, RepoPath: "/tmp/b"}
	m.mu.Unlock()

	// List all
	all := m.List("")
	if len(all) != 2 {
		t.Errorf("List('') = %d, want 2", len(all))
	}
}

func TestManagerTeamLifecycle(t *testing.T) {
	m := NewManager()

	// Manually inject a team
	team := &TeamStatus{
		Name:     "test-team",
		RepoPath: "/tmp/repo",
		LeadID:   "lead-session",
		Status:   StatusRunning,
		Tasks: []TeamTask{
			{Description: "task 1", Status: "pending"},
			{Description: "task 2", Status: "pending"},
		},
		CreatedAt: time.Now(),
	}
	m.mu.Lock()
	m.teams["test-team"] = team
	m.sessions["lead-session"] = &Session{ID: "lead-session", Status: StatusRunning}
	m.mu.Unlock()

	// GetTeam
	got, ok := m.GetTeam("test-team")
	if !ok {
		t.Fatal("team not found")
	}
	if got.Name != "test-team" {
		t.Errorf("team name = %q, want test-team", got.Name)
	}
	if len(got.Tasks) != 2 {
		t.Errorf("tasks = %d, want 2", len(got.Tasks))
	}

	// ListTeams
	teams := m.ListTeams()
	if len(teams) != 1 {
		t.Errorf("ListTeams = %d, want 1", len(teams))
	}

	// Team status tracks lead session
	lead, _ := m.Get("lead-session")
	lead.mu.Lock()
	lead.Status = StatusCompleted
	lead.mu.Unlock()

	got, _ = m.GetTeam("test-team")
	if got.Status != StatusCompleted {
		t.Errorf("team status = %q, want completed (should track lead)", got.Status)
	}
}

func TestManagerDelegateTask(t *testing.T) {
	m := NewManager()

	// Set up a team
	m.mu.Lock()
	m.teams["test-team"] = &TeamStatus{
		Name:     "test-team",
		RepoPath: "/tmp/repo",
		LeadID:   "lead",
		Status:   StatusRunning,
		Tasks:    []TeamTask{{Description: "task 1", Status: "pending"}},
	}
	m.sessions["lead"] = &Session{ID: "lead", Status: StatusRunning}
	m.mu.Unlock()

	// Delegate a new task
	count, err := m.DelegateTask("test-team", TeamTask{
		Description: "task 2",
		Status:      "pending",
	})
	if err != nil {
		t.Fatalf("DelegateTask: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	// Verify task was added
	team, ok := m.GetTeam("test-team")
	if !ok {
		t.Fatal("team not found")
	}
	if len(team.Tasks) != 2 {
		t.Errorf("tasks = %d, want 2", len(team.Tasks))
	}

	// Delegate to non-existent team
	_, err = m.DelegateTask("nonexistent", TeamTask{Description: "x", Status: "pending"})
	if err == nil {
		t.Error("expected error for nonexistent team")
	}
}

func TestManagerTaskStatusCorrelation(t *testing.T) {
	m := NewManager()

	// Set up a team with tasks
	m.mu.Lock()
	m.teams["corr-team"] = &TeamStatus{
		Name:     "corr-team",
		RepoPath: "/tmp/repo",
		LeadID:   "lead",
		Status:   StatusRunning,
		Tasks: []TeamTask{
			{Description: "implement auth", Status: "pending"},
			{Description: "write tests", Status: "pending"},
			{Description: "update docs", Status: "pending"},
		},
	}
	m.sessions["lead"] = &Session{ID: "lead", Status: StatusRunning, TeamName: "corr-team"}
	// Worker sessions: one running, one completed
	m.sessions["w1"] = &Session{
		ID:       "w1",
		Status:   StatusRunning,
		TeamName: "corr-team",
		Prompt:   "Please implement auth for the API",
	}
	m.sessions["w2"] = &Session{
		ID:       "w2",
		Status:   StatusCompleted,
		TeamName: "corr-team",
		Prompt:   "Please write tests for the auth module",
	}
	m.mu.Unlock()

	team, ok := m.GetTeam("corr-team")
	if !ok {
		t.Fatal("team not found")
	}

	// Task "implement auth" should be in-progress (w1 is running, prompt contains "implement auth")
	if team.Tasks[0].Status != "in-progress" {
		t.Errorf("task 0 status = %q, want in-progress", team.Tasks[0].Status)
	}
	// Task "write tests" should be completed (w2 is completed, prompt contains "write tests")
	if team.Tasks[1].Status != "completed" {
		t.Errorf("task 1 status = %q, want completed", team.Tasks[1].Status)
	}
	// Task "update docs" has no matching worker — should remain pending
	if team.Tasks[2].Status != "pending" {
		t.Errorf("task 2 status = %q, want pending", team.Tasks[2].Status)
	}
}

func TestSessionBudgetTracking(t *testing.T) {
	s := &Session{
		ID:        "budget-test",
		BudgetUSD: 10.0,
		SpentUSD:  0.0,
	}

	s.mu.Lock()
	s.SpentUSD = 5.5
	s.CostHistory = append(s.CostHistory, 2.0, 3.5)
	s.mu.Unlock()

	if s.SpentUSD != 5.5 {
		t.Errorf("SpentUSD = %f, want 5.5", s.SpentUSD)
	}
	if len(s.CostHistory) != 2 {
		t.Errorf("CostHistory len = %d, want 2", len(s.CostHistory))
	}
}

func TestSessionOutputHistory(t *testing.T) {
	s := &Session{
		ID: "output-test",
	}

	s.mu.Lock()
	s.OutputHistory = append(s.OutputHistory, "line 1", "line 2", "line 3")
	s.TurnCount = 3
	s.mu.Unlock()

	if len(s.OutputHistory) != 3 {
		t.Errorf("OutputHistory len = %d, want 3", len(s.OutputHistory))
	}
	if s.TurnCount != 3 {
		t.Errorf("TurnCount = %d, want 3", s.TurnCount)
	}
}
