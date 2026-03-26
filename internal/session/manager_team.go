package session

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// LaunchTeam creates an agent team by launching a lead session with team env vars.
func (m *Manager) LaunchTeam(ctx context.Context, config TeamConfig) (*TeamStatus, error) {
	if config.Name == "" {
		return nil, ErrTeamNameRequired
	}
	if config.RepoPath == "" {
		return nil, ErrRepoPathRequired
	}
	if len(config.Tasks) == 0 {
		return nil, ErrNoTasks
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

	// Enhance team lead prompt for its target provider
	var leadEnhance enhanceResult
	if m.Enhancer != nil {
		leadProvider := config.Provider
		if leadProvider == "" {
			leadProvider = ProviderClaude
		}
		leadEnhance = m.enhanceForProvider(ctx, leadPrompt, leadProvider)
		leadPrompt = leadEnhance.prompt
	} else {
		leadEnhance = enhanceResult{prompt: leadPrompt, source: "none", preScore: 0}
	}

	opts := LaunchOptions{
		Provider:     config.Provider,
		RepoPath:     config.RepoPath,
		Prompt:       leadPrompt,
		Model:        config.Model,
		MaxBudgetUSD: config.MaxBudgetUSD,
		Agent:        config.LeadAgent,
		TeamName:     config.Name,
		AllowedTools: teamLeadAllowedTools(),
	}

	s, err := m.Launch(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("launch team lead: %w", err)
	}
	s.EnhancementSource = leadEnhance.source
	s.EnhancementPreScore = leadEnhance.preScore

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
		m.bus.PublishCtx(ctx, events.Event{
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

// teamLeadAllowedTools returns the MCP tools a team lead session needs
// to autonomously launch and monitor worker sessions.
func teamLeadAllowedTools() []string {
	return []string{
		"mcp__ralphglasses__ralphglasses_session_launch",
		"mcp__ralphglasses__ralphglasses_session_status",
		"mcp__ralphglasses__ralphglasses_session_list",
		"mcp__ralphglasses__ralphglasses_session_stop",
		"mcp__ralphglasses__ralphglasses_session_output",
	}
}

// DelegateTask appends a task to a team under the manager mutex.
// Returns the updated task count.
func (m *Manager) DelegateTask(teamName string, task TeamTask) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	team, ok := m.teams[teamName]
	if !ok {
		return 0, fmt.Errorf("team %s: %w", teamName, ErrTeamNotFound)
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

// updateTeamOnSessionEnd checks if the completed session is a team lead
// and transitions the team status accordingly.
func (m *Manager) updateTeamOnSessionEnd(sess *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, team := range m.teams {
		if team.LeadID != sess.ID {
			continue
		}
		sess.mu.Lock()
		team.Status = sess.Status
		sess.mu.Unlock()

		// Mark pending tasks as cancelled if lead exited without delegating
		if team.Status == StatusCompleted || team.Status == StatusErrored || team.Status == StatusStopped {
			for i := range team.Tasks {
				if team.Tasks[i].Status == "pending" {
					team.Tasks[i].Status = "cancelled"
				}
			}
		}
		break
	}
}

// ListTeams returns all teams.
func (m *Manager) ListTeams() []*TeamStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*TeamStatus, 0, len(m.teams))
	for _, t := range m.teams {
		result = append(result, t)
	}
	return result
}
