package model

import (
	"fmt"
	"time"
)

// Repo represents a discovered ralph-enabled repository.
type Repo struct {
	Name          string
	Path          string
	HasRalph      bool // .ralph/ directory exists
	HasRC         bool // .ralphrc exists
	Status        *LoopStatus
	Circuit       *CircuitBreakerState
	Progress      *Progress
	Config        *RalphConfig
	SessionID     string  // active Claude Code session ID (if any)
	RefreshErrors []error // errors from last RefreshRepo call
}

// LoopStatus represents the parsed .ralph/status.json.
type LoopStatus struct {
	Timestamp        time.Time `json:"timestamp"`
	LoopCount        int       `json:"loop_count"`
	CallsMadeThisHr  int       `json:"calls_made_this_hour"`
	MaxCallsPerHour  int       `json:"max_calls_per_hour"`
	LastAction       string    `json:"last_action"`
	Status           string    `json:"status"`
	ExitReason       string    `json:"exit_reason"`
	NextReset        string    `json:"next_reset"`
	Model            string    `json:"model"`
	SessionSpendUSD  float64   `json:"session_spend_usd"`
	BudgetStatus     string    `json:"budget_status"`
}

// CircuitBreakerState represents .ralph/.circuit_breaker_state.
type CircuitBreakerState struct {
	State                      string    `json:"state"`
	LastChange                 time.Time `json:"last_change"`
	ConsecutiveNoProgress      int       `json:"consecutive_no_progress"`
	ConsecutiveSameError       int       `json:"consecutive_same_error"`
	ConsecutivePermissionDenials int     `json:"consecutive_permission_denials"`
	LastProgressLoop           int       `json:"last_progress_loop"`
	TotalOpens                 int       `json:"total_opens"`
	Reason                     string    `json:"reason"`
	CurrentLoop                int       `json:"current_loop"`
	OpenedAt                   *time.Time `json:"opened_at,omitempty"`
}

// Progress represents .ralph/progress.json.
type Progress struct {
	SpecFile     string         `json:"spec_file"`
	ProjectRoot  string         `json:"project_root,omitempty"`
	Iteration    int            `json:"iteration"`
	CompletedIDs []string       `json:"completed_ids"`
	Log          []IterationLog `json:"log"`
	Status       string         `json:"status"`
	StartedAt    time.Time      `json:"started_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// IterationLog records a single loop iteration.
type IterationLog struct {
	Iteration int       `json:"iteration"`
	TaskID    string    `json:"task_id,omitempty"`
	ToolCalls []string  `json:"tool_calls,omitempty"`
	Result    string    `json:"result"`
	Timestamp time.Time `json:"timestamp"`
}

// StatusDisplay returns the display status, preferring LoopStatus over Progress.
func (r *Repo) StatusDisplay() string {
	if r.Status != nil && r.Status.Status != "" {
		return r.Status.Status
	}
	if r.Progress != nil && r.Progress.Status != "" {
		return r.Progress.Status
	}
	return "unknown"
}

// CircuitDisplay returns a short circuit breaker display string.
func (r *Repo) CircuitDisplay() string {
	if r.Circuit == nil {
		return "-"
	}
	return r.Circuit.State
}

// CallsDisplay returns "made/max" for calls this hour.
func (r *Repo) CallsDisplay() string {
	if r.Status == nil {
		return "-"
	}
	return fmt.Sprintf("%d/%d", r.Status.CallsMadeThisHr, r.Status.MaxCallsPerHour)
}

// UpdatedDisplay returns a human-friendly time since last update.
func (r *Repo) UpdatedDisplay() string {
	if r.Status == nil {
		return "-"
	}
	if r.Status.Timestamp.IsZero() {
		return "-"
	}
	d := time.Since(r.Status.Timestamp)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}
