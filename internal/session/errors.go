package session

import "errors"

var (
	// ErrSessionTimeout indicates a session exceeded its time limit.
	ErrSessionTimeout = errors.New("session timeout")

	// ErrWorkerStalled indicates a worker session stopped producing output.
	ErrWorkerStalled = errors.New("worker stalled")

	// ErrInvalidProfile indicates a loop profile failed validation.
	ErrInvalidProfile = errors.New("invalid loop profile")
)
