package model

import (
	"errors"
	"fmt"
)

// Sentinel errors for common failure modes across the ralphglasses system.
var (
	// ErrNotFound indicates a requested resource does not exist.
	ErrNotFound = errors.New("not found")

	// ErrInvalidParams indicates one or more parameters failed validation.
	ErrInvalidParams = errors.New("invalid parameters")

	// ErrBudgetExceeded indicates a cost budget has been exhausted.
	ErrBudgetExceeded = errors.New("budget exceeded")

	// ErrTimeout indicates an operation did not complete within its deadline.
	ErrTimeout = errors.New("operation timed out")

	// ErrStalled indicates a loop or session stopped making progress.
	ErrStalled = errors.New("loop stalled")

	// ErrShuttingDown indicates the system is in the process of shutting down.
	ErrShuttingDown = errors.New("shutting down")

	// ErrAlreadyRunning indicates an operation is already in progress.
	ErrAlreadyRunning = errors.New("already running")

	// ErrNotRunning indicates the target is not currently active.
	ErrNotRunning = errors.New("not running")
)

// ConfigError represents a configuration-related failure.
type ConfigError struct {
	Key string
	Err error
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config error [%s]: %v", e.Key, e.Err)
}

func (e *ConfigError) Unwrap() error {
	return e.Err
}

// SessionError represents a session-scoped failure.
type SessionError struct {
	ID  string
	Err error
}

func (e *SessionError) Error() string {
	return fmt.Sprintf("session %s: %v", e.ID, e.Err)
}

func (e *SessionError) Unwrap() error {
	return e.Err
}

// LoopError represents a loop iteration failure.
type LoopError struct {
	RunID     string
	Iteration int
	Err       error
}

func (e *LoopError) Error() string {
	return fmt.Sprintf("loop %s iteration %d: %v", e.RunID, e.Iteration, e.Err)
}

func (e *LoopError) Unwrap() error {
	return e.Err
}
