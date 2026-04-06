package session

// Team run states.
const (
	TeamRunStateRunning       = "running"
	TeamRunStateCompleted     = "completed"
	TeamRunStateFailed        = "failed"
	TeamRunStateAwaitingInput = "awaiting_input"
)

// Team runtime types.
const (
	TeamRuntimeStructuredCodex = "structured_codex"
	TeamRuntimeLegacyLead      = "legacy_lead"
)

// Team execution backends.
const (
	TeamExecutionBackendLocal = "local"
	TeamExecutionBackendFleet = "fleet"
)

// Team worktree policies.
const (
	TeamWorktreePolicyPerWorker = "per_worker"
)

// Team task statuses.
const (
	TeamTaskPending    = "pending"
	TeamTaskInProgress = "in-progress"
	TeamTaskCompleted  = "completed"
	TeamTaskBlocked    = "blocked"
	TeamTaskFailed     = "failed"
	TeamTaskNeedsRetry = "needs_retry"
	TeamTaskCancelled  = "cancelled"
)

// Team merge statuses.
const (
	TeamMergeStatusPending     = "pending"
	TeamMergeStatusMerged      = "merged"
	TeamMergeStatusConflict    = "conflict"
	TeamMergeStatusUnavailable = "unavailable"
)
