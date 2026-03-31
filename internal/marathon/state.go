package marathon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PersistentState is written to disk for cross-invocation continuity.
type PersistentState struct {
	Running         bool      `json:"running"`
	CyclesCompleted int       `json:"cycles_completed"`
	TotalSpentUSD   float64   `json:"total_spent_usd"`
	SessionsRun     int       `json:"sessions_run"`
	StartedAt       time.Time `json:"started_at"`
	LastCheckpoint  time.Time `json:"last_checkpoint"`
	RepoPath        string    `json:"repo_path"`
	BudgetUSD       float64   `json:"budget_usd"`
}

// StatePath returns the path for the marathon state file.
func StatePath(repoPath string) string {
	return filepath.Join(repoPath, ".ralph", "marathon", "supervisor_state.json")
}

// SaveState persists the current marathon state to disk.
func (m *Marathon) SaveState() error {
	m.mu.Lock()
	state := PersistentState{
		Running:         true,
		CyclesCompleted: m.stats.CyclesCompleted,
		TotalSpentUSD:   m.stats.TotalSpentUSD,
		SessionsRun:     m.stats.SessionsRun,
		StartedAt:       m.startedAt,
		LastCheckpoint:  time.Now().UTC(),
		RepoPath:        m.cfg.RepoPath,
		BudgetUSD:       m.cfg.BudgetUSD,
	}
	m.mu.Unlock()

	dir := filepath.Dir(StatePath(m.cfg.RepoPath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create marathon state dir: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal marathon state: %w", err)
	}

	return os.WriteFile(StatePath(m.cfg.RepoPath), data, 0o644)
}

// RestoreState loads a previous marathon state from disk.
// Returns nil if no state file exists.
func RestoreState(repoPath string) (*PersistentState, error) {
	data, err := os.ReadFile(StatePath(repoPath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read marathon state: %w", err)
	}

	var state PersistentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal marathon state: %w", err)
	}

	return &state, nil
}

// ApplyRestoredState applies a previously saved state to the marathon's stats.
// Called during startup when --resume is used.
func (m *Marathon) ApplyRestoredState(state *PersistentState) {
	if state == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.stats.CyclesCompleted = state.CyclesCompleted
	m.stats.TotalSpentUSD = state.TotalSpentUSD
	m.stats.SessionsRun = state.SessionsRun
	m.startedAt = state.StartedAt
}
