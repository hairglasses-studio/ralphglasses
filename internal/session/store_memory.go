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
	mu              sync.RWMutex
	sessions        map[string]*Session
	loopRuns        map[string]*LoopRun
	costLedger      []CostEntry
	recoveryOps     map[string]*RecoveryOp
	recoveryActions map[string]*RecoveryAction
	tenants         map[string]*Tenant
}

// NewMemoryStore creates a new in-memory session store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions:        make(map[string]*Session),
		loopRuns:        make(map[string]*LoopRun),
		recoveryOps:     make(map[string]*RecoveryOp),
		recoveryActions: make(map[string]*RecoveryAction),
		tenants:         map[string]*Tenant{DefaultTenantID: DefaultTenant()},
	}
}

func (m *MemoryStore) SaveSession(_ context.Context, s *Session) error {
	if s == nil || s.ID == "" {
		return fmt.Errorf("save session: nil session or empty ID")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s.TenantID = NormalizeTenantID(s.TenantID)
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
		if opts.TenantID != "" && NormalizeTenantID(s.TenantID) != NormalizeTenantID(opts.TenantID) {
			continue
		}
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

func (m *MemoryStore) AggregateSpend(_ context.Context, tenantID, repo string) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var total float64
	for _, s := range m.sessions {
		if tenantID != "" && NormalizeTenantID(s.TenantID) != NormalizeTenantID(tenantID) {
			continue
		}
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
	run.TenantID = NormalizeTenantID(run.TenantID)
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
		if filter.TenantID != "" && NormalizeTenantID(r.TenantID) != NormalizeTenantID(filter.TenantID) {
			continue
		}
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
	entry.TenantID = NormalizeTenantID(entry.TenantID)
	if entry.RecordedAt.IsZero() {
		entry.RecordedAt = time.Now()
	}
	entry.ID = int64(len(m.costLedger) + 1)
	m.costLedger = append(m.costLedger, *entry)
	return nil
}

func (m *MemoryStore) AggregateCostByProvider(_ context.Context, tenantID string, since time.Time) (map[string]float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]float64)
	for _, e := range m.costLedger {
		if tenantID != "" && NormalizeTenantID(e.TenantID) != NormalizeTenantID(tenantID) {
			continue
		}
		if !e.RecordedAt.Before(since) {
			result[e.Provider] += e.SpendUSD
		}
	}
	return result, nil
}

// ---------- Recovery persistence ----------

func (m *MemoryStore) SaveRecoveryOp(_ context.Context, op *RecoveryOp) error {
	if op == nil || op.ID == "" {
		return fmt.Errorf("save recovery op: nil op or empty ID")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	op.TenantID = NormalizeTenantID(op.TenantID)
	m.recoveryOps[op.ID] = op
	return nil
}

func (m *MemoryStore) GetRecoveryOp(_ context.Context, id string) (*RecoveryOp, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	op, ok := m.recoveryOps[id]
	if !ok {
		return nil, ErrRecoveryOpNotFound
	}
	return op, nil
}

func (m *MemoryStore) ListRecoveryOps(_ context.Context, filter RecoveryOpFilter) ([]*RecoveryOp, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*RecoveryOp
	for _, op := range m.recoveryOps {
		if filter.TenantID != "" && NormalizeTenantID(op.TenantID) != NormalizeTenantID(filter.TenantID) {
			continue
		}
		if filter.Status != "" && op.Status != filter.Status {
			continue
		}
		if !filter.Since.IsZero() && op.DetectedAt.Before(filter.Since) {
			continue
		}
		result = append(result, op)
		if filter.Limit > 0 && len(result) >= filter.Limit {
			break
		}
	}
	return result, nil
}

func (m *MemoryStore) SaveRecoveryAction(_ context.Context, action *RecoveryAction) error {
	if action == nil || action.ID == "" {
		return fmt.Errorf("save recovery action: nil action or empty ID")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	action.TenantID = NormalizeTenantID(action.TenantID)
	m.recoveryActions[action.ID] = action
	return nil
}

func (m *MemoryStore) UpdateRecoveryActionStatus(_ context.Context, id string, status RecoveryActionStatus, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	action, ok := m.recoveryActions[id]
	if !ok {
		return ErrRecoveryActionNotFound
	}
	action.Status = status
	action.ErrorMsg = errMsg
	now := time.Now()
	if status == ActionExecuting {
		action.StartedAt = &now
	}
	if status == ActionSucceeded || status == ActionFailed || status == ActionSkipped {
		action.CompletedAt = &now
	}
	return nil
}

func (m *MemoryStore) SaveTenant(_ context.Context, tenant *Tenant) error {
	if tenant == nil {
		return fmt.Errorf("save tenant: nil tenant")
	}
	cp := *tenant
	cp.Normalize()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tenants[cp.ID] = &cp
	return nil
}

func (m *MemoryStore) GetTenant(_ context.Context, id string) (*Tenant, error) {
	id = NormalizeTenantID(id)
	m.mu.RLock()
	defer m.mu.RUnlock()
	tenant, ok := m.tenants[id]
	if !ok {
		return nil, ErrTenantNotFound
	}
	cp := *tenant
	cp.AllowedRepoRoots = append([]string(nil), tenant.AllowedRepoRoots...)
	return &cp, nil
}

func (m *MemoryStore) ListTenants(_ context.Context) ([]*Tenant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Tenant, 0, len(m.tenants))
	for _, tenant := range m.tenants {
		cp := *tenant
		cp.AllowedRepoRoots = append([]string(nil), tenant.AllowedRepoRoots...)
		result = append(result, &cp)
	}
	return result, nil
}
