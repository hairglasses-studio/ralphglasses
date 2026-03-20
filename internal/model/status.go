package model

import (
	"encoding/json"
	"errors"
	"fmt"
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
// Returns a slice of non-nil errors for any files that failed to parse.
// Missing files (os.ErrNotExist) are not considered errors.
func RefreshRepo(r *Repo) []error {
	var errs []error

	var err error
	r.Status, err = LoadStatus(r.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("status.json: %w", err))
	}

	r.Circuit, err = LoadCircuitBreaker(r.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("circuit_breaker_state: %w", err))
	}

	r.Progress, err = LoadProgress(r.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("progress.json: %w", err))
	}

	r.Config, err = LoadConfig(r.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf(".ralphrc: %w", err))
	}

	r.RefreshErrors = errs
	return errs
}
