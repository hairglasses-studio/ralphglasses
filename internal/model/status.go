package model

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LoadStatus reads and parses .ralph/status.json from the given repo path.
func LoadStatus(repoPath string) (*LoopStatus, error) {
	data, err := os.ReadFile(filepath.Join(repoPath, ".ralph", "status.json"))
	if err != nil {
		return nil, err
	}
	var s LoopStatus
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// LoadCircuitBreaker reads and parses .ralph/.circuit_breaker_state.
func LoadCircuitBreaker(repoPath string) (*CircuitBreakerState, error) {
	data, err := os.ReadFile(filepath.Join(repoPath, ".ralph", ".circuit_breaker_state"))
	if err != nil {
		return nil, err
	}
	var cb CircuitBreakerState
	if err := json.Unmarshal(data, &cb); err != nil {
		return nil, err
	}
	return &cb, nil
}

// LoadProgress reads and parses .ralph/progress.json.
func LoadProgress(repoPath string) (*Progress, error) {
	data, err := os.ReadFile(filepath.Join(repoPath, ".ralph", "progress.json"))
	if err != nil {
		return nil, err
	}
	var p Progress
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// RefreshRepo reloads all status files for a repo.
func RefreshRepo(r *Repo) {
	r.Status, _ = LoadStatus(r.Path)
	r.Circuit, _ = LoadCircuitBreaker(r.Path)
	r.Progress, _ = LoadProgress(r.Path)
	r.Config, _ = LoadConfig(r.Path)
}
