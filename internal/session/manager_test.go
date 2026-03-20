package session

import (
	"testing"
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
