package session

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"
)

// MemoryStore implements Store using an in-memory map.
// It wraps the same map[string]*Session pattern the Manager already uses.
type MemoryStore struct {
	mu         sync.RWMutex
	sessions   map[string]*Session
	loopRuns   map[string]*LoopRun
	costLedger []CostEntry
}

// NewMemoryStore creates a new in-memory session store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions: make(map[string]*Session),
		loopRuns: make(map[string]*LoopRun),
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
		if !opts.Since.IsZero() && s.LaunchedAt.Before(opts.Since) {
			continue
		}
		if !opts.Until.IsZero() && s.LaunchedAt.After(opts.Until) {
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

// ---------- LoopRun persistence ----------

func (m *MemoryStore) SaveLoopRun(_ context.Context, run *LoopRun) error {
	if run == nil || run.ID == "" {
		return fmt.Errorf("save loop run: nil run or empty ID")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loopRuns[run.ID] = run
	return nil
}

func (m *MemoryStore) GetLoopRun(_ context.Context, id string) (*LoopRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	run, ok := m.loopRuns[id]
	if !ok {
		return nil, ErrLoopNotFound
	}
	return run, nil
}

func (m *MemoryStore) ListLoopRuns(_ context.Context, filter LoopRunFilter) ([]*LoopRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*LoopRun
	for _, r := range m.loopRuns {
		if filter.RepoPath != "" && r.RepoPath != filter.RepoPath {
			continue
		}
		if filter.Status != "" && r.Status != filter.Status {
			continue
		}
		result = append(result, r)
		if filter.Limit > 0 && len(result) >= filter.Limit {
			break
		}
	}
	return result, nil
}

func (m *MemoryStore) UpdateLoopRunStatus(_ context.Context, id string, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.loopRuns[id]
	if !ok {
		return ErrLoopNotFound
	}
	run.Status = status
	run.UpdatedAt = time.Now()
	return nil
}

// ---------- Cost ledger persistence ----------

func (m *MemoryStore) RecordCost(_ context.Context, entry *CostEntry) error {
	if entry == nil {
		return fmt.Errorf("record cost: nil entry")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry.RecordedAt.IsZero() {
		entry.RecordedAt = time.Now()
	}
	entry.ID = int64(len(m.costLedger) + 1)
	m.costLedger = append(m.costLedger, *entry)
	return nil
}

func (m *MemoryStore) AggregateCostByProvider(_ context.Context, since time.Time) (map[string]float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]float64)
	for _, e := range m.costLedger {
		if !e.RecordedAt.Before(since) {
			result[e.Provider] += e.SpendUSD
		}
	}
	return result, nil
}
