package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadStatus reads and parses .ralph/status.json from the given repo path.
// The context is checked for cancellation before file I/O to allow callers
// to abort stuck reads (e.g. NFS, slow disks).
func LoadStatus(ctx context.Context, repoPath string) (*LoopStatus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
func LoadCircuitBreaker(ctx context.Context, repoPath string) (*CircuitBreakerState, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(repoPath, ".ralph", ".circuit_breaker_state"))
	if err != nil {
		return nil, err
	}
	var cb CircuitBreakerState
	if err := json.Unmarshal(data, &cb); err != nil {
		state := strings.TrimSpace(string(data))
		switch state {
		case "CLOSED", "OPEN", "HALF_OPEN":
			return &CircuitBreakerState{State: state}, nil
		default:
			return nil, err
		}
	}
	return &cb, nil
}

// LoadProgress reads and parses .ralph/progress.json.
func LoadProgress(ctx context.Context, repoPath string) (*Progress, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
// The context is checked before each file read; if cancelled, remaining
// files are skipped and the cancellation error is included in the result.
func RefreshRepo(ctx context.Context, r *Repo) []error {
	if ctx == nil {
		ctx = context.Background()
	}
	var errs []error

	var err error
	r.Status, err = LoadStatus(ctx, r.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("status.json: %w", err))
	}

	r.Circuit, err = LoadCircuitBreaker(ctx, r.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("circuit_breaker_state: %w", err))
	}

	r.Progress, err = LoadProgress(ctx, r.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("progress.json: %w", err))
	}

	r.Config, err = LoadConfig(ctx, r.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf(".ralphrc: %w", err))
	}

	r.RefreshErrors = errs
	return errs
}
