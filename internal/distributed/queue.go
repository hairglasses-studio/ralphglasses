// Package distributed provides fleet-level task distribution across
// multiple ralphglasses nodes using NATS JetStream.
//
// Informed by MegaFlow (ArXiv 2601.07526) — Model/Agent/Environment
// three-service architecture with independent scaling.
package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// TaskState represents the lifecycle of a distributed task.
type TaskState string

const (
	TaskPending   TaskState = "pending"
	TaskClaimed   TaskState = "claimed"
	TaskRunning   TaskState = "running"
	TaskCompleted TaskState = "completed"
	TaskFailed    TaskState = "failed"
	TaskDead      TaskState = "dead" // moved to dead-letter queue after max retries
)

// DistributedTask is a unit of work submitted to the fleet queue.
type DistributedTask struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`          // "session", "loop", "cycle", "verify"
	Prompt       string            `json:"prompt"`
	RepoPath     string            `json:"repo_path"`
	Provider     string            `json:"provider,omitempty"`
	Model        string            `json:"model,omitempty"`
	BudgetUSD    float64           `json:"budget_usd,omitempty"`
	Priority     int               `json:"priority"`      // higher = more urgent
	State        TaskState         `json:"state"`
	ClaimedBy    string            `json:"claimed_by,omitempty"` // worker node ID
	Result       json.RawMessage   `json:"result,omitempty"`
	Error        string            `json:"error,omitempty"`
	Retries      int               `json:"retries"`
	MaxRetries   int               `json:"max_retries"` // default 2
	Metadata     map[string]string `json:"metadata,omitempty"`
	SubmittedAt  time.Time         `json:"submitted_at"`
	ClaimedAt    *time.Time        `json:"claimed_at,omitempty"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty"`
}

// QueueConfig configures the distributed task queue.
type QueueConfig struct {
	MaxPending    int           // max tasks in pending state (default 1000)
	MaxRetries    int           // per-task retry limit (default 2)
	ClaimTimeout  time.Duration // how long before unclaimed task is reclaimed (default 5m)
	DeadLetterMax int           // max tasks in DLQ (default 100)
}

// DefaultQueueConfig returns sensible defaults.
func DefaultQueueConfig() QueueConfig {
	return QueueConfig{
		MaxPending:    1000,
		MaxRetries:    2,
		ClaimTimeout:  5 * time.Minute,
		DeadLetterMax: 100,
	}
}

// DistributedQueue manages task distribution across fleet workers.
// It provides Submit/Claim/Complete/Fail semantics with dead-letter queue
// for tasks that exhaust retries.
type DistributedQueue struct {
	mu       sync.Mutex
	cfg      QueueConfig
	pending  []*DistributedTask // sorted by priority (highest first)
	claimed  map[string]*DistributedTask // task ID -> task
	dlq      []*DistributedTask // dead-letter queue
	history  []*DistributedTask // completed/failed tasks (ring buffer)

	// Callbacks
	onComplete func(task *DistributedTask)
	onDead     func(task *DistributedTask)
}

// NewDistributedQueue creates a task queue with the given config.
func NewDistributedQueue(cfg QueueConfig) *DistributedQueue {
	if cfg.MaxPending <= 0 {
		cfg.MaxPending = 1000
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 2
	}
	if cfg.ClaimTimeout <= 0 {
		cfg.ClaimTimeout = 5 * time.Minute
	}
	if cfg.DeadLetterMax <= 0 {
		cfg.DeadLetterMax = 100
	}

	return &DistributedQueue{
		cfg:     cfg,
		claimed: make(map[string]*DistributedTask),
	}
}

// OnComplete registers a callback for task completion.
func (q *DistributedQueue) OnComplete(fn func(*DistributedTask)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.onComplete = fn
}

// OnDead registers a callback for tasks moved to DLQ.
func (q *DistributedQueue) OnDead(fn func(*DistributedTask)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.onDead = fn
}

// Submit adds a task to the pending queue.
func (q *DistributedQueue) Submit(task *DistributedTask) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pending) >= q.cfg.MaxPending {
		return fmt.Errorf("queue full: %d pending tasks", len(q.pending))
	}

	if task.ID == "" {
		task.ID = fmt.Sprintf("dtask-%d", time.Now().UnixNano())
	}
	task.State = TaskPending
	task.SubmittedAt = time.Now()
	if task.MaxRetries == 0 {
		task.MaxRetries = q.cfg.MaxRetries
	}

	// Insert maintaining priority order (highest first)
	inserted := false
	for i, t := range q.pending {
		if task.Priority > t.Priority {
			q.pending = append(q.pending[:i+1], q.pending[i:]...)
			q.pending[i] = task
			inserted = true
			break
		}
	}
	if !inserted {
		q.pending = append(q.pending, task)
	}

	slog.Debug("distributed: task submitted",
		"id", task.ID, "type", task.Type, "priority", task.Priority)
	return nil
}

// Claim assigns the highest-priority pending task to a worker.
// Returns nil if no tasks are available.
func (q *DistributedQueue) Claim(workerID string) *DistributedTask {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pending) == 0 {
		return nil
	}

	task := q.pending[0]
	q.pending = q.pending[1:]

	now := time.Now()
	task.State = TaskClaimed
	task.ClaimedBy = workerID
	task.ClaimedAt = &now
	q.claimed[task.ID] = task

	slog.Debug("distributed: task claimed",
		"id", task.ID, "worker", workerID)
	return task
}

// Start marks a claimed task as running.
func (q *DistributedQueue) Start(taskID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, ok := q.claimed[taskID]
	if !ok {
		return fmt.Errorf("task %s not claimed", taskID)
	}
	task.State = TaskRunning
	return nil
}

// Complete marks a task as successfully completed.
func (q *DistributedQueue) Complete(taskID string, result json.RawMessage) error {
	q.mu.Lock()

	task, ok := q.claimed[taskID]
	if !ok {
		q.mu.Unlock()
		return fmt.Errorf("task %s not claimed", taskID)
	}

	now := time.Now()
	task.State = TaskCompleted
	task.Result = result
	task.CompletedAt = &now
	delete(q.claimed, taskID)

	q.addHistory(task)
	cb := q.onComplete
	q.mu.Unlock()

	if cb != nil {
		cb(task)
	}

	slog.Debug("distributed: task completed", "id", taskID)
	return nil
}

// Fail marks a task as failed. If retries remain, it's requeued as pending.
// If retries are exhausted, it's moved to the dead-letter queue.
func (q *DistributedQueue) Fail(taskID string, errMsg string) error {
	q.mu.Lock()

	task, ok := q.claimed[taskID]
	if !ok {
		q.mu.Unlock()
		return fmt.Errorf("task %s not claimed", taskID)
	}

	delete(q.claimed, taskID)
	task.Error = errMsg
	task.Retries++

	if task.Retries <= task.MaxRetries {
		// Requeue for retry
		task.State = TaskPending
		task.ClaimedBy = ""
		task.ClaimedAt = nil
		q.pending = append(q.pending, task) // append at end (lowest priority for retries)
		q.mu.Unlock()

		slog.Info("distributed: task requeued for retry",
			"id", taskID, "retries", task.Retries, "max", task.MaxRetries)
		return nil
	}

	// Exhausted retries — move to DLQ
	task.State = TaskDead
	q.dlq = append(q.dlq, task)
	if len(q.dlq) > q.cfg.DeadLetterMax {
		q.dlq = q.dlq[1:] // evict oldest
	}

	cb := q.onDead
	q.mu.Unlock()

	if cb != nil {
		cb(task)
	}

	slog.Warn("distributed: task moved to DLQ",
		"id", taskID, "retries", task.Retries, "error", errMsg)
	return nil
}

// ReclaimStale moves claimed tasks that have exceeded ClaimTimeout back to pending.
func (q *DistributedQueue) ReclaimStale() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	cutoff := time.Now().Add(-q.cfg.ClaimTimeout)
	reclaimed := 0

	for id, task := range q.claimed {
		if task.ClaimedAt != nil && task.ClaimedAt.Before(cutoff) {
			delete(q.claimed, id)
			task.State = TaskPending
			task.ClaimedBy = ""
			task.ClaimedAt = nil
			q.pending = append(q.pending, task)
			reclaimed++
		}
	}

	if reclaimed > 0 {
		slog.Info("distributed: reclaimed stale tasks", "count", reclaimed)
	}
	return reclaimed
}

// Stats returns queue statistics.
func (q *DistributedQueue) Stats() QueueStats {
	q.mu.Lock()
	defer q.mu.Unlock()

	return QueueStats{
		Pending:  len(q.pending),
		Claimed:  len(q.claimed),
		DLQ:      len(q.dlq),
		History:  len(q.history),
	}
}

// QueueStats holds queue depth counts.
type QueueStats struct {
	Pending int `json:"pending"`
	Claimed int `json:"claimed"`
	DLQ     int `json:"dlq"`
	History int `json:"history"`
}

// PendingTasks returns a copy of pending tasks.
func (q *DistributedQueue) PendingTasks() []*DistributedTask {
	q.mu.Lock()
	defer q.mu.Unlock()
	result := make([]*DistributedTask, len(q.pending))
	for i, t := range q.pending {
		copy := *t
		result[i] = &copy
	}
	return result
}

// DeadLetterTasks returns a copy of DLQ tasks.
func (q *DistributedQueue) DeadLetterTasks() []*DistributedTask {
	q.mu.Lock()
	defer q.mu.Unlock()
	result := make([]*DistributedTask, len(q.dlq))
	for i, t := range q.dlq {
		copy := *t
		result[i] = &copy
	}
	return result
}

func (q *DistributedQueue) addHistory(task *DistributedTask) {
	q.history = append(q.history, task)
	if len(q.history) > 500 {
		q.history = q.history[len(q.history)-500:]
	}
}

// StartMaintenance runs periodic queue maintenance (reclaim stale, etc).
func (q *DistributedQueue) StartMaintenance(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				q.ReclaimStale()
			}
		}
	}()
}
