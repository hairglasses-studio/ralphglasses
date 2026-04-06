package session

import (
	"context"
	"fmt"
	"time"
)

const defaultTeamControllerInterval = 2 * time.Second

func (m *Manager) StartTeam(ctx context.Context, name string) (*TeamStatus, error) {
	m.configMu.Lock()
	m.ensureTeamControllersLocked()
	if existing, ok := m.teamControllers[name]; ok && existing != nil {
		m.configMu.Unlock()
		m.workersMu.Lock()
		team, ok := m.teams[name]
		if !ok {
			m.workersMu.Unlock()
			return nil, ErrTeamNotFound
		}
		team.ControllerRunning = true
		snapshot := cloneTeamStatus(team)
		m.workersMu.Unlock()
		return snapshot, nil
	}
	runCtx, cancel := context.WithCancel(context.Background())
	controller := &teamController{cancel: cancel, done: make(chan struct{})}
	m.teamControllers[name] = controller
	m.configMu.Unlock()

	m.workersMu.Lock()
	team, ok := m.teams[name]
	if !ok {
		m.workersMu.Unlock()
		m.configMu.Lock()
		delete(m.teamControllers, name)
		m.configMu.Unlock()
		cancel()
		return nil, ErrTeamNotFound
	}
	if !isStructuredCodexTeam(team) {
		snapshot := cloneTeamStatus(team)
		m.workersMu.Unlock()
		m.configMu.Lock()
		delete(m.teamControllers, name)
		m.configMu.Unlock()
		cancel()
		return snapshot, nil
	}
	team.ControllerRunning = true
	team.LastControllerError = ""
	team.AutoStart = true
	team.UpdatedAt = time.Now()
	snapshot := cloneTeamStatus(team)
	m.workersMu.Unlock()
	m.persistTeamOrWarn(name, "team controller start")

	go func() {
		defer close(controller.done)
		defer func() {
			m.configMu.Lock()
			delete(m.teamControllers, name)
			m.configMu.Unlock()

			m.workersMu.Lock()
			if team, ok := m.teams[name]; ok {
				team.ControllerRunning = false
				team.UpdatedAt = time.Now()
			}
			m.workersMu.Unlock()
			m.persistTeamOrWarn(name, "team controller exit")
		}()

		ticker := time.NewTicker(defaultTeamControllerInterval)
		defer ticker.Stop()

		for {
			if _, err := m.StepTeam(runCtx, name); err != nil {
				m.workersMu.Lock()
				if team, ok := m.teams[name]; ok {
					team.LastControllerError = err.Error()
					team.UpdatedAt = time.Now()
				}
				m.workersMu.Unlock()
				m.persistTeamOrWarn(name, "team controller error")
				return
			}

			snapshot, ok := m.teamSnapshot(name)
			if !ok {
				return
			}
			if snapshot.RunState == TeamRunStateCompleted || snapshot.RunState == TeamRunStateFailed || snapshot.RunState == TeamRunStateAwaitingInput {
				return
			}

			select {
			case <-runCtx.Done():
				return
			case <-ctx.Done():
				cancel()
				return
			case <-ticker.C:
			}
		}
	}()

	return snapshot, nil
}

func (m *Manager) StopTeam(name string) (*TeamStatus, error) {
	m.configMu.Lock()
	controller := m.teamControllers[name]
	if controller != nil {
		controller.cancel()
	}
	m.configMu.Unlock()

	if controller != nil {
		select {
		case <-controller.done:
		case <-time.After(5 * time.Second):
		}
	}

	m.workersMu.Lock()
	if team, ok := m.teams[name]; ok {
		team.AutoStart = false
		team.ControllerRunning = false
		team.UpdatedAt = time.Now()
	}
	m.workersMu.Unlock()
	m.persistTeamOrWarn(name, "team controller stop")

	team, ok := m.teamSnapshot(name)
	if !ok {
		return nil, ErrTeamNotFound
	}
	return team, nil
}

func (m *Manager) AwaitTeam(ctx context.Context, name string, pollInterval time.Duration) (*TeamStatus, error) {
	if pollInterval <= 0 {
		pollInterval = defaultTeamControllerInterval
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		team, ok := m.teamSnapshot(name)
		if !ok {
			return nil, ErrTeamNotFound
		}
		if team.RunState == TeamRunStateCompleted || team.RunState == TeamRunStateFailed || team.RunState == TeamRunStateAwaitingInput {
			return team, nil
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("await team %s: %w", name, ctx.Err())
		case <-ticker.C:
		}
	}
}
