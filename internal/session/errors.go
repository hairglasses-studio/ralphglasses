package session

import "errors"

var (
	// ErrSessionTimeout indicates a session exceeded its time limit.
	ErrSessionTimeout = errors.New("session timeout")

	// ErrWorkerStalled indicates a worker session stopped producing output.
	ErrWorkerStalled = errors.New("worker stalled")

	// ErrInvalidProfile indicates a loop profile failed validation.
	ErrInvalidProfile = errors.New("invalid loop profile")

	// ErrSessionNotFound indicates the requested session ID does not exist.
	ErrSessionNotFound = errors.New("session not found")

	// ErrSessionNotRunning indicates the session is not in a running state.
	ErrSessionNotRunning = errors.New("session not running")

	// ErrSessionErrored indicates the session ended in an error state.
	ErrSessionErrored = errors.New("session errored")

	// ErrSessionStopped indicates the session was stopped.
	ErrSessionStopped = errors.New("session stopped")

	// ErrTeamNotFound indicates the requested team name does not exist.
	ErrTeamNotFound = errors.New("team not found")

	// ErrTeamNameRequired indicates a team name was not provided.
	ErrTeamNameRequired = errors.New("team name required")

	// ErrRepoPathRequired indicates a repo path was not provided.
	ErrRepoPathRequired = errors.New("repo path required")

	// ErrNoTasks indicates no tasks were provided for a team.
	ErrNoTasks = errors.New("at least one task required")

	// ErrAlreadyOnProvider indicates the session is already using the target provider.
	ErrAlreadyOnProvider = errors.New("already on provider")

	// ErrWaitTimeout indicates waitForSession timed out.
	ErrWaitTimeout = errors.New("wait timed out")

	// ErrUnexpectedExit indicates the session process exited unexpectedly.
	ErrUnexpectedExit = errors.New("process exited unexpectedly")

	// ErrLoopNotFound indicates the requested loop ID does not exist.
	ErrLoopNotFound = errors.New("loop not found")

	// ErrLoopStopped indicates the loop is in a stopped state.
	ErrLoopStopped = errors.New("loop stopped")

	// ErrLoopConverged indicates the loop has converged and should not continue.
	ErrLoopConverged = errors.New("loop converged")

	// ErrRepoNotExist indicates the repo path does not exist on the filesystem.
	ErrRepoNotExist = errors.New("repo path does not exist")

	// ErrRepoNotGit indicates the repo path is not a git repository.
	ErrRepoNotGit = errors.New("repo path is not a git repository")

	// ErrRecoveryOpNotFound indicates the requested recovery operation ID does not exist.
	ErrRecoveryOpNotFound = errors.New("recovery operation not found")

	// ErrRecoveryActionNotFound indicates the requested recovery action ID does not exist.
	ErrRecoveryActionNotFound = errors.New("recovery action not found")
)
