package fleet

import (
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// WorkItemStatus represents the lifecycle state of a work item.
type WorkItemStatus string

const (
	WorkPending   WorkItemStatus = "pending"
	WorkAssigned  WorkItemStatus = "assigned"
	WorkRunning   WorkItemStatus = "running"
	WorkCompleted WorkItemStatus = "completed"
	WorkFailed    WorkItemStatus = "failed"
)

// WorkItemType categorizes the kind of work.
type WorkItemType string

const (
	WorkTypeSession      WorkItemType = "session"
	WorkTypeLoopTask     WorkItemType = "loop_task"
	WorkTypeWorkflowStep WorkItemType = "workflow_step"
)

// WorkItem represents a unit of work in the fleet queue.
type WorkItem struct {
	ID           string             `json:"id"`
	Type         WorkItemType       `json:"type"`
	Status       WorkItemStatus     `json:"status"`
	Priority     int                `json:"priority"`
	RepoName     string             `json:"repo_name"`
	RepoPath     string             `json:"repo_path,omitempty"`
	Prompt       string             `json:"prompt"`
	Provider     session.Provider   `json:"provider,omitempty"`
	Model        string             `json:"model,omitempty"`
	Agent        string             `json:"agent,omitempty"`
	MaxBudgetUSD float64            `json:"max_budget_usd,omitempty"`
	MaxTurns     int                `json:"max_turns,omitempty"`
	Constraints  WorkConstraints    `json:"constraints,omitempty"`
	AssignedTo   string             `json:"assigned_to,omitempty"` // worker node ID
	SessionID    string             `json:"session_id,omitempty"` // session ID once running
	RetryCount   int                `json:"retry_count"`
	MaxRetries   int                `json:"max_retries"`
	Error        string             `json:"error,omitempty"`
	RetryAfter   *time.Time         `json:"retry_after,omitempty"`
	SubmittedAt  time.Time          `json:"submitted_at"`
	AssignedAt   *time.Time         `json:"assigned_at,omitempty"`
	StartedAt    *time.Time         `json:"started_at,omitempty"`
	CompletedAt  *time.Time         `json:"completed_at,omitempty"`
	Result       *WorkResult        `json:"result,omitempty"`
}

// WorkConstraints specifies placement constraints for work items.
type WorkConstraints struct {
	NodePreference string           `json:"node_preference,omitempty"` // preferred node ID
	RequireLocal   bool             `json:"require_local,omitempty"`   // repo must exist on worker
	RequireProvider session.Provider `json:"require_provider,omitempty"`
}

// WorkResult captures the outcome of a completed work item.
type WorkResult struct {
	SessionID  string  `json:"session_id"`
	SpentUSD   float64 `json:"spent_usd"`
	TurnCount  int     `json:"turn_count"`
	DurationS  float64 `json:"duration_seconds"`
	ExitReason string  `json:"exit_reason,omitempty"`
	Output     string  `json:"output,omitempty"`
}

// WorkerInfo describes a registered worker node.
type WorkerInfo struct {
	ID             string              `json:"id"`
	Hostname       string              `json:"hostname"`
	TailscaleIP    string              `json:"tailscale_ip"`
	Port           int                 `json:"port"`
	Status         WorkerStatus        `json:"status"`
	Providers      []session.Provider  `json:"providers"`
	Repos          []string            `json:"repos"`
	MaxSessions    int                 `json:"max_sessions"`
	ActiveSessions int                 `json:"active_sessions"`
	SpentUSD       float64             `json:"spent_usd"`
	RegisteredAt   time.Time           `json:"registered_at"`
	LastHeartbeat  time.Time           `json:"last_heartbeat"`
	Version        string              `json:"version,omitempty"`
}

// WorkerStatus represents the health state of a worker.
type WorkerStatus string

const (
	WorkerOnline       WorkerStatus = "online"
	WorkerStale        WorkerStatus = "stale"        // no heartbeat for 90s
	WorkerDisconnected WorkerStatus = "disconnected"  // no heartbeat for 5m
	WorkerPaused       WorkerStatus = "paused"        // manually paused, skip for assignment
)

// HeartbeatPayload is sent by workers every 30s.
type HeartbeatPayload struct {
	WorkerID       string             `json:"worker_id"`
	ActiveSessions int                `json:"active_sessions"`
	SpentUSD       float64            `json:"spent_usd"`
	AvailableSlots int                `json:"available_slots"`
	Repos          []string           `json:"repos"`
	Providers      []session.Provider `json:"providers"`
	Load           float64            `json:"load"` // 0.0–1.0
}

// RegisterPayload is sent when a worker first connects.
type RegisterPayload struct {
	Hostname    string             `json:"hostname"`
	TailscaleIP string             `json:"tailscale_ip"`
	Port        int                `json:"port"`
	Providers   []session.Provider `json:"providers"`
	Repos       []string           `json:"repos"`
	MaxSessions int                `json:"max_sessions"`
	Version     string             `json:"version,omitempty"`
}

// WorkPollResponse is returned when a worker polls for work.
type WorkPollResponse struct {
	Item *WorkItem `json:"item,omitempty"` // nil = no work available
}

// WorkCompletePayload is sent by a worker when it finishes a work item.
type WorkCompletePayload struct {
	WorkItemID string         `json:"work_item_id"`
	Status     WorkItemStatus `json:"status"` // completed or failed
	Result     *WorkResult    `json:"result,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// FleetState is the coordinator's view of the entire fleet.
type FleetState struct {
	Workers       []WorkerInfo   `json:"workers"`
	QueueDepth    int            `json:"queue_depth"`
	ActiveWork    int            `json:"active_work"`
	CompletedWork int            `json:"completed_work"`
	FailedWork    int            `json:"failed_work"`
	TotalSpentUSD float64       `json:"total_spent_usd"`
	BudgetUSD     float64       `json:"budget_usd"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// NodeStatus is returned by GET /api/v1/status for any node.
type NodeStatus struct {
	NodeID    string       `json:"node_id"`
	Role      string       `json:"role"` // "coordinator", "worker", "standalone"
	Hostname  string       `json:"hostname"`
	Uptime    float64      `json:"uptime_seconds"`
	Sessions  int          `json:"active_sessions"`
	SpentUSD  float64      `json:"spent_usd"`
	Version   string       `json:"version"`
	StartedAt time.Time    `json:"started_at"`
}

// EventBatch is sent by workers to forward local events to the coordinator.
type EventBatch struct {
	WorkerID string       `json:"worker_id"`
	Events   []FleetEvent `json:"events"`
}

// FleetEvent wraps an event with node origin information.
type FleetEvent struct {
	NodeID    string         `json:"node_id"`
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	RepoName  string         `json:"repo_name,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Provider  string         `json:"provider,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// GlobalBudget tracks fleet-wide spend across all workers.
type GlobalBudget struct {
	LimitUSD      float64   `json:"limit_usd"`
	SpentUSD      float64   `json:"spent_usd"`
	ReservedUSD   float64   `json:"reserved_usd"` // budget assigned to pending/active work
	LastUpdated   time.Time `json:"last_updated"`
}

// AvailableBudget returns the budget remaining for new work.
func (b *GlobalBudget) AvailableBudget() float64 {
	avail := b.LimitUSD - b.SpentUSD - b.ReservedUSD
	if avail < 0 {
		return 0
	}
	return avail
}
