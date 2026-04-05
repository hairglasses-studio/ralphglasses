package session

import (
	"context"
	"time"
)

// ListOpts controls filtering for Store.ListSessions.
type ListOpts struct {
	RepoPath string        // filter by repo path (empty = all)
	RepoName string        // filter by repo name (empty = all)
	Status   SessionStatus // filter by status (empty = all)
	Limit    int           // max results (0 = unlimited)
	Since    time.Time     // filter: launched_at >= since (zero = no filter)
	Until    time.Time     // filter: launched_at <= until (zero = no filter)
}

// LoopRunFilter controls filtering for loop run queries.
type LoopRunFilter struct {
	RepoPath string
	Status   string
	Limit    int
}

// CostEntry represents a single cost ledger record.
type CostEntry struct {
	ID         int64
	SessionID  string
	LoopID     string
	Provider   string
	Model      string
	SpendUSD   float64
	TurnCount  int
	ElapsedSec float64
	RecordedAt time.Time
}

// RecoveryOpStatus represents the lifecycle state of a recovery operation.
type RecoveryOpStatus string

const (
	RecoveryOpDetected  RecoveryOpStatus = "detected"
	RecoveryOpExecuting RecoveryOpStatus = "executing"
	RecoveryOpCompleted RecoveryOpStatus = "completed"
	RecoveryOpFailed    RecoveryOpStatus = "failed"
	RecoveryOpAborted   RecoveryOpStatus = "aborted"
)

// RecoveryOp represents a single crash detection and recovery operation.
type RecoveryOp struct {
	ID            string           `json:"id"`
	Severity      string           `json:"severity"`
	Status        RecoveryOpStatus `json:"status"`
	TotalSessions int              `json:"total_sessions"`
	AliveCount    int              `json:"alive_count"`
	DeadCount     int              `json:"dead_count"`
	ResumedCount  int              `json:"resumed_count"`
	FailedCount   int              `json:"failed_count"`
	TotalCostUSD  float64          `json:"total_cost_usd"`
	BudgetCapUSD  float64          `json:"budget_cap_usd"`
	TriggerSource string           `json:"trigger_source"`
	DecisionID    string           `json:"decision_id,omitempty"`
	ErrorMsg      string           `json:"error_msg,omitempty"`
	DetectedAt    time.Time        `json:"detected_at"`
	StartedAt     *time.Time       `json:"started_at,omitempty"`
	CompletedAt   *time.Time       `json:"completed_at,omitempty"`
}

// RecoveryActionStatus represents the state of a single session resume attempt.
type RecoveryActionStatus string

const (
	ActionPending   RecoveryActionStatus = "pending"
	ActionExecuting RecoveryActionStatus = "executing"
	ActionSucceeded RecoveryActionStatus = "succeeded"
	ActionFailed    RecoveryActionStatus = "failed"
	ActionSkipped   RecoveryActionStatus = "skipped"
)

// RecoveryAction represents a single session resume attempt within a recovery op.
type RecoveryAction struct {
	ID              string               `json:"id"`
	RecoveryOpID    string               `json:"recovery_op_id"`
	ClaudeSessionID string               `json:"claude_session_id"`
	RalphSessionID  string               `json:"ralph_session_id,omitempty"`
	RepoPath        string               `json:"repo_path"`
	RepoName        string               `json:"repo_name"`
	Priority        int                  `json:"priority"`
	Status          RecoveryActionStatus `json:"status"`
	CostUSD         float64              `json:"cost_usd"`
	ErrorMsg        string               `json:"error_msg,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
	StartedAt       *time.Time           `json:"started_at,omitempty"`
	CompletedAt     *time.Time           `json:"completed_at,omitempty"`
}

// RecoveryOpFilter controls filtering for ListRecoveryOps.
type RecoveryOpFilter struct {
	Status RecoveryOpStatus // empty = all
	Since  time.Time        // zero = no filter
	Limit  int              // 0 = unlimited
}

// Store is the persistence interface for session state.
// Both in-memory and SQLite implementations satisfy this interface.
type Store interface {
	// SaveSession upserts a session (insert or update).
	SaveSession(ctx context.Context, s *Session) error

	// GetSession returns a session by ID. Returns ErrSessionNotFound if missing.
	GetSession(ctx context.Context, id string) (*Session, error)

	// ListSessions returns sessions matching the given filters.
	ListSessions(ctx context.Context, opts ListOpts) ([]*Session, error)

	// DeleteSession removes a session by ID. No error if not found.
	DeleteSession(ctx context.Context, id string) error

	// UpdateSessionStatus sets the status of a session.
	UpdateSessionStatus(ctx context.Context, id string, status SessionStatus) error

	// AggregateSpend returns total spend_usd across sessions for a repo path.
	// If repo is empty, returns total across all sessions.
	AggregateSpend(ctx context.Context, repo string) (float64, error)

	// SaveLoopRun upserts a loop run (insert or update).
	SaveLoopRun(ctx context.Context, run *LoopRun) error

	// GetLoopRun returns a loop run by ID. Returns ErrLoopNotFound if missing.
	GetLoopRun(ctx context.Context, id string) (*LoopRun, error)

	// ListLoopRuns returns loop runs matching the given filters.
	ListLoopRuns(ctx context.Context, filter LoopRunFilter) ([]*LoopRun, error)

	// UpdateLoopRunStatus sets the status of a loop run.
	UpdateLoopRunStatus(ctx context.Context, id string, status string) error

	// RecordCost inserts a cost ledger entry.
	RecordCost(ctx context.Context, entry *CostEntry) error

	// AggregateCostByProvider returns total spend per provider since a given time.
	AggregateCostByProvider(ctx context.Context, since time.Time) (map[string]float64, error)

	// SaveRecoveryOp upserts a recovery operation.
	SaveRecoveryOp(ctx context.Context, op *RecoveryOp) error

	// GetRecoveryOp returns a recovery operation by ID. Returns ErrRecoveryOpNotFound if missing.
	GetRecoveryOp(ctx context.Context, id string) (*RecoveryOp, error)

	// ListRecoveryOps returns recovery operations matching the filter.
	ListRecoveryOps(ctx context.Context, filter RecoveryOpFilter) ([]*RecoveryOp, error)

	// SaveRecoveryAction upserts a recovery action.
	SaveRecoveryAction(ctx context.Context, action *RecoveryAction) error

	// UpdateRecoveryActionStatus updates the status and optional error of an action.
	UpdateRecoveryActionStatus(ctx context.Context, id string, status RecoveryActionStatus, errMsg string) error

	// Close releases any resources held by the store.
	Close() error
}
