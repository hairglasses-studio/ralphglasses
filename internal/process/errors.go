package process

import "errors"

var (
	// ErrAlreadyRunning indicates a loop is already managed for the given repo.
	ErrAlreadyRunning = errors.New("loop already running")

	// ErrNoLoopScript indicates no ralph_loop.sh was found in the repo directory.
	ErrNoLoopScript = errors.New("no loop script found")

	// ErrNotRunning indicates no managed loop exists for the given repo.
	ErrNotRunning = errors.New("no running loop")
)
