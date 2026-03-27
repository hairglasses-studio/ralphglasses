package session

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
)

// MemoryStore implements Store using an in-memory map.
// It wraps the same map[string]*Session pattern the Manager already uses.
type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewMemoryStore creates a new in-memory session store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions: make(map[string]*Session),
	}
}

func (m *MemoryStore) SaveSession(_ context.Context, s *Session) error {
	if s == nil || s.ID == "" {
		return fmt.Errorf("save session: nil session or empty ID")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s
	return nil
}

func (m *MemoryStore) GetSession(_ context.Context, id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return s, nil
}

func (m *MemoryStore) ListSessions(_ context.Context, opts ListOpts) ([]*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Session
	for _, s := range m.sessions {
		if opts.RepoPath != "" && s.RepoPath != opts.RepoPath {
			continue
		}
		if opts.RepoName != "" && filepath.Base(s.RepoPath) != opts.RepoName {
			continue
		}
		if opts.Status != "" && s.Status != opts.Status {
			continue
		}
		result = append(result, s)
		if opts.Limit > 0 && len(result) >= opts.Limit {
			break
		}
	}
	return result, nil
}

func (m *MemoryStore) DeleteSession(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	return nil
}

func (m *MemoryStore) UpdateSessionStatus(_ context.Context, id string, status SessionStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	s.Status = status
	return nil
}

func (m *MemoryStore) AggregateSpend(_ context.Context, repo string) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var total float64
	for _, s := range m.sessions {
		if repo != "" && s.RepoPath != repo {
			continue
		}
		total += s.SpentUSD
	}
	return total, nil
}

func (m *MemoryStore) Close() error {
	return nil
}
