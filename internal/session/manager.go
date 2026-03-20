package session

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"syscall"
)

// Manager tracks all active Claude Code sessions and teams.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session    // keyed by session ID
	teams    map[string]*TeamStatus // keyed by team name
}

// NewManager creates a new session manager.
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		teams:    make(map[string]*TeamStatus),
	}
}

// Launch starts a new Claude Code session via claude -p.
func (m *Manager) Launch(ctx context.Context, opts LaunchOptions) (*Session, error) {
	s, err := launch(ctx, opts)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()

	return s, nil
}

// Get returns a session by ID.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// List returns all sessions, optionally filtered by repo path.
func (m *Manager) List(repoPath string) []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []*Session
	for _, s := range m.sessions {
		if repoPath != "" && s.RepoPath != repoPath {
			continue
		}
		result = append(result, s)
	}
	return result
}

// Stop gracefully stops a running session.
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("session not found: %s", id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status != StatusRunning && s.Status != StatusLaunching {
		return fmt.Errorf("session %s is not running (status: %s)", id, s.Status)
	}

	s.Status = StatusStopped

	// Cancel context first
	if s.cancel != nil {
		s.cancel()
	}

	// Then signal the process group
	if s.cmd != nil && s.cmd.Process != nil {
		pgid, err := syscall.Getpgid(s.cmd.Process.Pid)
		if err != nil {
			_ = s.cmd.Process.Signal(syscall.SIGTERM)
		} else {
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
		}
	}

	return nil
}

// StopAll stops all running sessions.
func (m *Manager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id, s := range m.sessions {
		s.mu.Lock()
		if s.Status == StatusRunning || s.Status == StatusLaunching {
			ids = append(ids, id)
		}
		s.mu.Unlock()
	}
	m.mu.Unlock()

	for _, id := range ids {
		_ = m.Stop(id)
	}
}

// Resume resumes a previous session by its provider session ID.
func (m *Manager) Resume(ctx context.Context, repoPath string, provider Provider, sessionID, prompt string) (*Session, error) {
	opts := LaunchOptions{
		Provider: provider,
		RepoPath: repoPath,
		Prompt:   prompt,
		Resume:   sessionID,
	}
	return m.Launch(ctx, opts)
}

// IsRunning checks if any session is running for the given repo path.
func (m *Manager) IsRunning(repoPath string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if s.RepoPath == repoPath {
			s.mu.Lock()
			running := s.Status == StatusRunning || s.Status == StatusLaunching
			s.mu.Unlock()
			if running {
				return true
			}
		}
	}
	return false
}

// FindByRepo returns all sessions for a given repo name.
func (m *Manager) FindByRepo(repoName string) []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []*Session
	for _, s := range m.sessions {
		if filepath.Base(s.RepoPath) == repoName {
			result = append(result, s)
		}
	}
	return result
}

// LaunchTeam creates an agent team by launching a lead session with team env vars.
func (m *Manager) LaunchTeam(ctx context.Context, config TeamConfig) (*TeamStatus, error) {
	if config.Name == "" {
		return nil, fmt.Errorf("team name required")
	}
	if config.RepoPath == "" {
		return nil, fmt.Errorf("repo path required")
	}
	if len(config.Tasks) == 0 {
		return nil, fmt.Errorf("at least one task required")
	}

	// Build a lead prompt that instructs Claude to use agent teams
	var taskList string
	for i, t := range config.Tasks {
		taskList += fmt.Sprintf("%d. %s\n", i+1, t)
	}

	leadPrompt := fmt.Sprintf(
		"You are a team lead coordinating work on this project. "+
			"Use the Agent tool to create teammates and delegate these tasks:\n\n%s\n"+
			"Coordinate all work, verify results, and report final status.",
		taskList,
	)

	opts := LaunchOptions{
		Provider:     config.Provider,
		RepoPath:     config.RepoPath,
		Prompt:       leadPrompt,
		Model:        config.Model,
		MaxBudgetUSD: config.MaxBudgetUSD,
		Agent:        config.LeadAgent,
		TeamName:     config.Name,
	}

	s, err := m.Launch(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("launch team lead: %w", err)
	}

	tasks := make([]TeamTask, len(config.Tasks))
	for i, desc := range config.Tasks {
		tasks[i] = TeamTask{
			Description: desc,
			Status:      "pending",
		}
	}

	team := &TeamStatus{
		Name:     config.Name,
		RepoPath: config.RepoPath,
		LeadID:   s.ID,
		Status:   StatusRunning,
		Tasks:    tasks,
	}

	m.mu.Lock()
	m.teams[config.Name] = team
	m.mu.Unlock()

	return team, nil
}

// GetTeam returns team status by name.
func (m *Manager) GetTeam(name string) (*TeamStatus, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, ok := m.teams[name]
	if !ok {
		return nil, false
	}

	// Update team status based on lead session
	if s, sOk := m.sessions[team.LeadID]; sOk {
		s.mu.Lock()
		team.Status = s.Status
		s.mu.Unlock()
	}

	return team, true
}

// ListTeams returns all teams.
func (m *Manager) ListTeams() []*TeamStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*TeamStatus, 0, len(m.teams))
	for _, t := range m.teams {
		result = append(result, t)
	}
	return result
}
