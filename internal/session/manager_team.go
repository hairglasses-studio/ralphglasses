package session

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// TeamSafety holds the safety configuration for team operations.
// If nil, DefaultTeamSafety is used.
var TeamSafety *TeamSafetyConfig

func teamSafetyConfig() TeamSafetyConfig {
	if TeamSafety != nil {
		return *TeamSafety
	}
	return DefaultTeamSafety
}

// LaunchTeam creates an agent team by launching a lead session with team env vars.
func (m *Manager) LaunchTeam(ctx context.Context, config TeamConfig) (*TeamStatus, error) {
	config.TenantID = NormalizeTenantID(config.TenantID)
	if config.Name == "" {
		return nil, ErrTeamNameRequired
	}
	if config.RepoPath == "" {
		return nil, ErrRepoPathRequired
	}
	if len(config.Tasks) == 0 {
		return nil, ErrNoTasks
	}

	// Safety: enforce team creation limits.
	m.workersMu.RLock()
	existingCount := 0
	for _, team := range m.teams {
		if NormalizeTenantID(team.TenantID) == config.TenantID {
			existingCount++
		}
	}
	m.workersMu.RUnlock()
	if err := ValidateTeamCreate(config.Name, len(config.Tasks), existingCount, teamSafetyConfig()); err != nil {
		return nil, err
	}

	if config.Provider == ProviderCodex {
		return m.launchStructuredCodexTeam(ctx, config)
	}

	// Build a lead prompt that instructs the lead to use agent teams
	var taskList strings.Builder
	for i, t := range config.Tasks {
		taskList.WriteString(fmt.Sprintf("%d. %s\n", i+1, t))
	}

	workerProvider := config.WorkerProvider
	if workerProvider == "" {
		workerProvider = config.Provider
	}
	if workerProvider == "" {
		workerProvider = DefaultPrimaryProvider()
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
		taskList.String(), workerProvider, workerProvider,
	)

	// Enhance team lead prompt for its target provider
	var leadEnhance enhanceResult
	if m.Enhancer != nil {
		leadProvider := config.Provider
		if leadProvider == "" {
			leadProvider = DefaultPrimaryProvider()
		}
		leadEnhance = m.enhanceForProvider(ctx, leadPrompt, leadProvider)
		leadPrompt = leadEnhance.prompt
	} else {
		leadEnhance = enhanceResult{prompt: leadPrompt, source: "none", preScore: 0}
	}

	opts := LaunchOptions{
		TenantID:     config.TenantID,
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
		Name:           config.Name,
		TenantID:       config.TenantID,
		RepoPath:       config.RepoPath,
		LeadID:         s.ID,
		Status:         StatusRunning,
		Tasks:          tasks,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Provider:       config.Provider,
		WorkerProvider: workerProvider,
		Model:          config.Model,
		WorkerModel:    config.WorkerModel,
		MaxBudgetUSD:   config.MaxBudgetUSD,
	}
	key := m.teamKey(config.Name, config.TenantID)

	m.workersMu.Lock()
	m.teams[key] = team
	m.workersMu.Unlock()

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

func (m *Manager) launchStructuredCodexTeam(ctx context.Context, config TeamConfig) (*TeamStatus, error) {
	team := newStructuredCodexTeam(config)
	key := m.teamKey(config.Name, config.TenantID)

	m.workersMu.Lock()
	m.teams[key] = team
	m.workersMu.Unlock()
	m.persistTeamOrWarn(key, "create structured team")

	if m.bus != nil {
		m.bus.PublishCtx(ctx, events.Event{
			Type:     events.TeamCreated,
			RepoPath: config.RepoPath,
			RepoName: filepath.Base(config.RepoPath),
			Provider: string(config.Provider),
			Data: map[string]any{
				"team":    config.Name,
				"runtime": team.Runtime,
				"tasks":   len(config.Tasks),
			},
		})
	}

	if team.AutoStart {
		started, err := m.StartTeamForTenant(ctx, config.TenantID, config.Name)
		if err != nil {
			return nil, fmt.Errorf("start structured team: %w", err)
		}
		return started, nil
	}
	return cloneTeamStatus(team), nil
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
	return m.DelegateTaskForTenant(DefaultTenantID, teamName, task)
}

func (m *Manager) DelegateTaskForTenant(tenantID, teamName string, task TeamTask) (int, error) {
	m.workersMu.Lock()
	defer m.workersMu.Unlock()
	team, ok := m.teams[m.teamKey(teamName, tenantID)]
	if !ok {
		return 0, fmt.Errorf("team %s: %w", teamName, ErrTeamNotFound)
	}
	team.Tasks = append(team.Tasks, task)
	return len(team.Tasks), nil
}

// workerSnapshot holds a snapshot of session fields needed by GetTeam.
type workerSnapshot struct {
	prompt string
	status SessionStatus
}

// GetTeam returns team status by name.
// It also correlates task statuses from worker sessions.
//
// Two-phase locking: sessionsMu is acquired and released before workersMu is
// acquired, so the two locks are never held simultaneously.
func (m *Manager) GetTeam(name string) (*TeamStatus, bool) {
	return m.GetTeamForTenant(name, DefaultTenantID)
}

func (m *Manager) getTeamByKey(key string) (*TeamStatus, bool) {
	// Phase 1: collect everything we need from sessions while holding sessionsMu.
	// We must look up the team's LeadID first, so we peek at workersMu briefly,
	// but we do NOT hold both locks at the same time.
	m.workersMu.RLock()
	team, ok := m.teams[key]
	m.workersMu.RUnlock()
	if !ok {
		return nil, false
	}

	// Now snapshot the session data we need (lead status + worker snapshots).
	var leadStatus SessionStatus
	var hasLead bool
	var workers []workerSnapshot

	m.sessionsMu.RLock()
	if s, sOk := m.sessions[team.LeadID]; sOk {
		s.mu.Lock()
		leadStatus = s.Status
		s.mu.Unlock()
		hasLead = true
	}
	for _, s := range m.sessions {
		if s.TeamName == team.Name && NormalizeTenantID(s.TenantID) == NormalizeTenantID(team.TenantID) && s.ID != team.LeadID {
			s.mu.Lock()
			workers = append(workers, workerSnapshot{prompt: s.Prompt, status: s.Status})
			s.mu.Unlock()
		}
	}
	m.sessionsMu.RUnlock()

	// Phase 2: apply the snapshots under workersMu only.
	m.workersMu.Lock()
	defer m.workersMu.Unlock()

	// Re-fetch team in case it was removed between the two phases.
	team, ok = m.teams[key]
	if !ok {
		return nil, false
	}

	if hasLead {
		team.Status = leadStatus
	}

	// Correlate task statuses from worker session snapshots.
	for i := range team.Tasks {
		task := &team.Tasks[i]
		if task.Status == "completed" || task.Status == "errored" {
			continue // terminal states don't change
		}
		for _, w := range workers {
			if !strings.Contains(w.prompt, task.Description) {
				continue
			}
			switch w.status {
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

	return cloneTeamStatus(team), true
}

// updateTeamOnSessionEnd checks if the completed session is a team lead
// and transitions the team status accordingly.
func (m *Manager) updateTeamOnSessionEnd(sess *Session) {
	m.workersMu.Lock()
	defer m.workersMu.Unlock()

	for _, team := range m.teams {
		if team.LeadID != sess.ID || NormalizeTenantID(team.TenantID) != NormalizeTenantID(sess.TenantID) {
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
	return m.ListTeamsForTenant(DefaultTenantID)
}
