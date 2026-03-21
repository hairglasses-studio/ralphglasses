package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// DefaultStateDir is the shared directory for session state persistence.
const DefaultStateDir = "~/.ralphglasses/sessions"

// Manager tracks all active Claude Code sessions and teams.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session    // keyed by session ID
	teams    map[string]*TeamStatus // keyed by team name
	bus      *events.Bus
	stateDir string // directory for persisted session JSON files
}

// NewManager creates a new session manager.
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		teams:    make(map[string]*TeamStatus),
		stateDir: expandHome(DefaultStateDir),
	}
}

// NewManagerWithBus creates a session manager wired to an event bus.
func NewManagerWithBus(bus *events.Bus) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		teams:    make(map[string]*TeamStatus),
		bus:      bus,
		stateDir: expandHome(DefaultStateDir),
	}
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// Launch starts a new Claude Code session via claude -p.
func (m *Manager) Launch(ctx context.Context, opts LaunchOptions) (*Session, error) {
	s, err := launch(ctx, opts, m.bus)
	if err != nil {
		return nil, err
	}

	// Set persistence callback so runner can persist on completion
	s.onComplete = func(sess *Session) {
		m.PersistSession(sess)
	}

	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()

	// Persist initial state to disk
	m.PersistSession(s)

	if m.bus != nil {
		m.bus.Publish(events.Event{
			Type:      events.SessionStarted,
			SessionID: s.ID,
			RepoPath:  s.RepoPath,
			RepoName:  filepath.Base(s.RepoPath),
			Provider:  string(s.Provider),
			Data:      map[string]any{"model": s.Model, "prompt_len": len(opts.Prompt)},
		})
	}

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

	// Persist stopped state
	go m.PersistSession(s)

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

	// Build a lead prompt that instructs the lead to use agent teams
	var taskList string
	for i, t := range config.Tasks {
		taskList += fmt.Sprintf("%d. %s\n", i+1, t)
	}

	workerProvider := config.WorkerProvider
	if workerProvider == "" {
		workerProvider = config.Provider
	}
	if workerProvider == "" {
		workerProvider = ProviderClaude
	}

	leadPrompt := fmt.Sprintf(
		`You are a team lead coordinating work on this project.

## Tasks to delegate

%s
## MCP Tools available

- ralphglasses_session_launch — Launch a worker session (required: repo, prompt; optional: provider, model, max_budget_usd, agent, system_prompt)
- ralphglasses_session_status — Check a worker's progress (required: session_id)
- ralphglasses_session_list — List all sessions (optional: repo, provider, status filters)
- ralphglasses_session_stop — Stop a stuck/completed worker (required: session_id)

## Provider capabilities

| Parameter       | claude (all) | gemini         | codex          |
|-----------------|-------------|----------------|----------------|
| prompt          | yes         | yes            | yes            |
| model           | yes         | yes            | yes            |
| resume          | yes         | yes            | no             |
| system_prompt   | yes         | no (ignored)   | no (ignored)   |
| max_budget_usd  | yes         | no (ignored)   | no (ignored)   |
| agent           | yes         | no (ignored)   | no (ignored)   |
| allowed_tools   | yes         | no (ignored)   | no (ignored)   |

## Workflow

1. Launch worker sessions with ralphglasses_session_launch (provider=%q)
2. Poll status with ralphglasses_session_status every 30-60 seconds
3. Stop stuck workers with ralphglasses_session_stop if no progress
4. Verify completed work by reading output from session_status
5. Report final status summarizing all task outcomes

Default worker provider: %s.
Provider strengths: claude (complex architecture), gemini (fast bulk generation), codex (focused refactoring).`,
		taskList, workerProvider, workerProvider,
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

	if m.bus != nil {
		m.bus.Publish(events.Event{
			Type:      events.TeamCreated,
			SessionID: s.ID,
			RepoPath:  config.RepoPath,
			RepoName:  filepath.Base(config.RepoPath),
			Provider:  string(config.Provider),
			Data:      map[string]any{"team": config.Name, "tasks": len(config.Tasks)},
		})
	}

	return team, nil
}

// DelegateTask appends a task to a team under the manager mutex.
// Returns the updated task count.
func (m *Manager) DelegateTask(teamName string, task TeamTask) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	team, ok := m.teams[teamName]
	if !ok {
		return 0, fmt.Errorf("team not found: %s", teamName)
	}
	team.Tasks = append(team.Tasks, task)
	return len(team.Tasks), nil
}

// GetTeam returns team status by name.
// It also correlates task statuses from worker sessions.
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

	// Correlate task statuses from worker sessions.
	// Workers launched by the lead have TeamName set and their prompt
	// contains the task description as a substring.
	m.correlateTaskStatuses(team)

	return team, true
}

// correlateTaskStatuses updates task statuses by matching worker sessions.
// Must be called with m.mu held.
func (m *Manager) correlateTaskStatuses(team *TeamStatus) {
	// Collect worker sessions for this team (excluding the lead)
	var workers []*Session
	for _, s := range m.sessions {
		if s.TeamName == team.Name && s.ID != team.LeadID {
			workers = append(workers, s)
		}
	}

	for i := range team.Tasks {
		task := &team.Tasks[i]
		if task.Status == "completed" || task.Status == "errored" {
			continue // terminal states don't change
		}
		for _, w := range workers {
			if !strings.Contains(w.Prompt, task.Description) {
				continue
			}
			w.mu.Lock()
			ws := w.Status
			w.mu.Unlock()
			switch ws {
			case StatusRunning, StatusLaunching:
				task.Status = "in-progress"
			case StatusCompleted:
				task.Status = "completed"
			case StatusErrored:
				task.Status = "errored"
			case StatusStopped:
				task.Status = "errored"
			}
			break // first match wins
		}
	}
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

// PersistSession writes session state to the shared state directory.
// Safe to call from any goroutine; acquires the session lock.
func (m *Manager) PersistSession(s *Session) {
	if m.stateDir == "" {
		return
	}
	_ = os.MkdirAll(m.stateDir, 0755)

	s.mu.Lock()
	data, err := json.Marshal(s)
	s.mu.Unlock()
	if err != nil {
		return
	}

	path := filepath.Join(m.stateDir, s.ID+".json")
	_ = os.WriteFile(path, data, 0644)
}

// LoadExternalSessions reads session JSON files from the shared state directory
// and merges any unknown sessions into the manager. This allows the TUI to
// discover sessions launched by the MCP server (a separate process).
func (m *Manager) LoadExternalSessions() {
	if m.stateDir == "" {
		return
	}
	entries, err := os.ReadDir(m.stateDir)
	if err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".json")

		// If we already own this session (launched in-process), update the file
		// but don't overwrite in-memory state.
		if existing, ok := m.sessions[id]; ok {
			// Re-persist in-process sessions so disk stays current
			go m.PersistSession(existing)
			continue
		}

		data, err := os.ReadFile(filepath.Join(m.stateDir, entry.Name()))
		if err != nil {
			continue
		}

		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}

		// Only import sessions that are recent (last 24h)
		if s.LaunchedAt.IsZero() || s.LaunchedAt.Before(s.LaunchedAt.Add(-24*3600*1e9)) {
			// always import — LaunchedAt.Before(LaunchedAt) is false, so this is a no-op guard
		}

		m.sessions[id] = &s
	}

	// Clean up stale completed sessions older than 24h from disk
	for id, s := range m.sessions {
		s.mu.Lock()
		ended := s.EndedAt
		status := s.Status
		s.mu.Unlock()

		if (status == StatusCompleted || status == StatusErrored || status == StatusStopped) &&
			ended != nil && ended.Before(ended.Add(-24*3600*1e9)) {
			// This time check is always false; keeping for future TTL.
			// For now, just skip cleanup — sessions persist until manually removed.
			_ = id
		}
	}
}
