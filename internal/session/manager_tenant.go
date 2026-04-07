package session

import (
	"os"
	"path/filepath"
	"strings"
)

func (m *Manager) sessionStateDirForTenant(tenantID string) string {
	if m.stateDir == "" {
		return ""
	}
	return filepath.Join(m.stateDir, sanitizeTenantPathSegment(tenantID))
}

func (m *Manager) sessionStatePath(tenantID, sessionID string) string {
	dir := m.sessionStateDirForTenant(tenantID)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, sessionID+".json")
}

func (m *Manager) legacySessionStatePath(sessionID string) string {
	if m.stateDir == "" {
		return ""
	}
	return filepath.Join(m.stateDir, sessionID+".json")
}

func (m *Manager) teamStateRootDir() string {
	if m.stateDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(m.stateDir), "teams")
}

func (m *Manager) teamStateDirForTenant(tenantID string) string {
	root := m.teamStateRootDir()
	if root == "" {
		return ""
	}
	return filepath.Join(root, sanitizeTenantPathSegment(tenantID))
}

func (m *Manager) teamStatePath(tenantID, teamName string) string {
	dir := m.teamStateDirForTenant(tenantID)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, teamStateFilename(teamName))
}

type sessionStateFile struct {
	Path     string
	ID       string
	TenantID string
	Legacy   bool
}

func (m *Manager) discoverSessionStateFiles() []sessionStateFile {
	if m.stateDir == "" {
		return nil
	}
	entries, err := os.ReadDir(m.stateDir)
	if err != nil {
		return nil
	}
	var files []sessionStateFile
	for _, entry := range entries {
		path := filepath.Join(m.stateDir, entry.Name())
		if entry.IsDir() {
			tenantID := NormalizeTenantID(entry.Name())
			children, err := os.ReadDir(path)
			if err != nil {
				continue
			}
			for _, child := range children {
				if child.IsDir() || !strings.HasSuffix(child.Name(), ".json") {
					continue
				}
				files = append(files, sessionStateFile{
					Path:     filepath.Join(path, child.Name()),
					ID:       strings.TrimSuffix(child.Name(), ".json"),
					TenantID: tenantID,
				})
			}
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		files = append(files, sessionStateFile{
			Path:     path,
			ID:       strings.TrimSuffix(entry.Name(), ".json"),
			TenantID: DefaultTenantID,
			Legacy:   true,
		})
	}
	return files
}

type teamStateFile struct {
	Path     string
	TenantID string
	Legacy   bool
}

func (m *Manager) discoverTeamStateFiles() []teamStateFile {
	root := m.teamStateRootDir()
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var files []teamStateFile
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			tenantID := NormalizeTenantID(entry.Name())
			children, err := os.ReadDir(path)
			if err != nil {
				continue
			}
			for _, child := range children {
				if child.IsDir() || !strings.HasSuffix(child.Name(), ".json") {
					continue
				}
				files = append(files, teamStateFile{
					Path:     filepath.Join(path, child.Name()),
					TenantID: tenantID,
				})
			}
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		files = append(files, teamStateFile{
			Path:     path,
			TenantID: DefaultTenantID,
			Legacy:   true,
		})
	}
	return files
}

// GetForTenant returns a session by ID only if it belongs to the requested tenant.
func (m *Manager) GetForTenant(id, tenantID string) (*Session, bool) {
	tenantID = NormalizeTenantID(tenantID)
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, false
	}
	if NormalizeTenantID(s.TenantID) != tenantID {
		return nil, false
	}
	return s, true
}

// ListByTenant returns sessions for a specific tenant and optional repo path.
func (m *Manager) ListByTenant(repoPath, tenantID string) []*Session {
	tenantID = NormalizeTenantID(tenantID)
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()

	var result []*Session
	for _, s := range m.sessions {
		s.Lock()
		if NormalizeTenantID(s.TenantID) != tenantID {
			s.Unlock()
			continue
		}
		if repoPath != "" && s.RepoPath != repoPath {
			s.Unlock()
			continue
		}
		result = append(result, cloneSessionLocked(s))
		s.Unlock()
	}
	return result
}

func (m *Manager) teamKey(name, tenantID string) string {
	return teamStorageKey(tenantID, name)
}

func (m *Manager) GetTeamForTenant(name, tenantID string) (*TeamStatus, bool) {
	return m.getTeamByKey(m.teamKey(name, tenantID))
}

func (m *Manager) ListTeamsForTenant(tenantID string) []*TeamStatus {
	tenantID = NormalizeTenantID(tenantID)
	m.workersMu.RLock()
	defer m.workersMu.RUnlock()

	result := make([]*TeamStatus, 0, len(m.teams))
	for _, team := range m.teams {
		if NormalizeTenantID(team.TenantID) != tenantID {
			continue
		}
		result = append(result, cloneTeamStatus(team))
	}
	return result
}
