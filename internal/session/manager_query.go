package session

import (
	"path/filepath"
)

// Get returns a session by ID.
func (m *Manager) Get(id string) (*Session, bool) {
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

// List returns all sessions, optionally filtered by repo path.
func (m *Manager) List(repoPath string) []*Session {
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()

	var result []*Session
	for _, s := range m.sessions {
		if repoPath != "" && s.RepoPath != repoPath {
			continue
		}
		result = append(result, s)
	}
	return result
}

// IsRunning checks if any session is running for the given repo path.
func (m *Manager) IsRunning(repoPath string) bool {
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()
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
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()

	var result []*Session
	for _, s := range m.sessions {
		if filepath.Base(s.RepoPath) == repoName {
			result = append(result, s)
		}
	}
	return result
}

// GetWorkflowRun returns a workflow run by ID.
func (m *Manager) GetWorkflowRun(id string) (*WorkflowRun, bool) {
	m.workersMu.RLock()
	defer m.workersMu.RUnlock()
	run, ok := m.workflowRuns[id]
	return run, ok
}

// GetStatus is a hot-read path that returns the SessionStatus for id without
// acquiring sessionsMu. It reads from the lock-free statusCache (Phase 10.5.1).
// Returns ("", false) if the session is not found in the cache.
func (m *Manager) GetStatus(id string) (SessionStatus, bool) {
	v, ok := m.statusCache.Load(id)
	if !ok {
		return "", false
	}
	return v.(SessionStatus), true
}

// updateStatusCache updates the lock-free statusCache for the given session.
// Call this whenever a session's Status field changes.
func (m *Manager) updateStatusCache(id string, status SessionStatus) {
	m.statusCache.Store(id, status)
}

// evictStatusCache removes the status cache entry for a session.
// Call this when a session is deleted.
func (m *Manager) evictStatusCache(id string) {
	m.statusCache.Delete(id)
}
