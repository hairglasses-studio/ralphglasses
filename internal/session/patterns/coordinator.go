// Package patterns implements multi-agent collaboration patterns.
// This file implements D3.1: Centralized Architect/Worker coordination.
//
// Informed by Scaling Agent Systems (ArXiv 2512.08296): centralized coordination
// limits error amplification to 4.4x vs 17.2x for independent agents.
package patterns

import (
	"fmt"
	"sync"
)

// ErrorPolicy controls how the coordinator handles step failures.
type ErrorPolicy struct {
	MaxRetries     int     // per-task retry limit (default 2)
	AbortThreshold float64 // fraction of tasks that can fail before aborting (default 0.3)
	ReassignOnFail bool    // try different worker on failure (default true)
}

// DefaultErrorPolicy returns sensible defaults.
func DefaultErrorPolicy() ErrorPolicy {
	return ErrorPolicy{
		MaxRetries:     2,
		AbortThreshold: 0.3,
		ReassignOnFail: true,
	}
}

// CoordinatedTask is a unit of work managed by the coordinator.
type CoordinatedTask struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	AssignedTo  string `json:"assigned_to"` // worker session ID
	Status      string `json:"status"`      // pending, running, completed, failed, retried
	Retries     int    `json:"retries"`
	Output      string `json:"output,omitempty"`
	Error       string `json:"error,omitempty"`
}

// TaskResult captures the outcome of a coordinated task.
type TaskResult struct {
	TaskID  string `json:"task_id"`
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// CentralCoordinator wraps the architect pattern with error propagation control.
// It manages task assignment, retry logic, and abort decisions.
type CentralCoordinator struct {
	mu       sync.Mutex
	policy   ErrorPolicy
	memory   *TieredMemory
	msgQueue *MessageQueue

	plan       []CoordinatedTask
	results    map[string]*TaskResult
	failCount  int
	totalTasks int
	aborted    bool
}

// NewCentralCoordinator creates a coordinator with the given error policy.
// memory and msgQueue may be nil for lightweight usage.
func NewCentralCoordinator(mem *TieredMemory, mq *MessageQueue, policy ErrorPolicy) *CentralCoordinator {
	return &CentralCoordinator{
		policy:  policy,
		memory:  mem,
		msgQueue: mq,
		results: make(map[string]*TaskResult),
	}
}

// SetPlan sets the execution plan. All tasks start as "pending".
func (cc *CentralCoordinator) SetPlan(tasks []CoordinatedTask) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.plan = make([]CoordinatedTask, len(tasks))
	for i, t := range tasks {
		t.Status = "pending"
		cc.plan[i] = t
	}
	cc.totalTasks = len(tasks)
	cc.failCount = 0
	cc.aborted = false
	cc.results = make(map[string]*TaskResult)
}

// AssignTask assigns a task to a worker session.
func (cc *CentralCoordinator) AssignTask(taskID, workerID string) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	for i := range cc.plan {
		if cc.plan[i].ID == taskID {
			cc.plan[i].AssignedTo = workerID
			cc.plan[i].Status = "running"
			return nil
		}
	}
	return fmt.Errorf("task %s not found", taskID)
}

// ReportResult records the outcome of a task. If the task failed and retries
// remain, it is marked for retry. If the abort threshold is exceeded, no
// more tasks will be assigned.
func (cc *CentralCoordinator) ReportResult(taskID string, result TaskResult) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.results[taskID] = &result

	for i := range cc.plan {
		if cc.plan[i].ID != taskID {
			continue
		}

		if result.Success {
			cc.plan[i].Status = "completed"
			cc.plan[i].Output = result.Output
			return nil
		}

		// Task failed
		cc.plan[i].Error = result.Error

		if cc.plan[i].Retries < cc.policy.MaxRetries {
			cc.plan[i].Retries++
			cc.plan[i].Status = "retried"
			if cc.policy.ReassignOnFail {
				cc.plan[i].AssignedTo = "" // clear for reassignment
			}
			return nil
		}

		// Exhausted retries
		cc.plan[i].Status = "failed"
		cc.failCount++

		// Check abort threshold
		if cc.totalTasks > 0 {
			failRate := float64(cc.failCount) / float64(cc.totalTasks)
			if failRate >= cc.policy.AbortThreshold {
				cc.aborted = true
			}
		}
		return nil
	}

	return fmt.Errorf("task %s not found", taskID)
}

// ShouldAbort returns true if the failure threshold has been exceeded.
func (cc *CentralCoordinator) ShouldAbort() bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.aborted
}

// PlanStatus returns aggregate counts.
func (cc *CentralCoordinator) PlanStatus() (completed, failed, pending int) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	for _, t := range cc.plan {
		switch t.Status {
		case "completed":
			completed++
		case "failed":
			failed++
		case "pending", "retried":
			pending++
		}
	}
	return
}

// AllComplete returns true if all tasks are done (completed or failed) or aborted.
func (cc *CentralCoordinator) AllComplete() bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.aborted {
		return true
	}
	for _, t := range cc.plan {
		if t.Status == "pending" || t.Status == "running" || t.Status == "retried" {
			return false
		}
	}
	return true
}

// PendingTasks returns tasks that need assignment (pending or retried).
func (cc *CentralCoordinator) PendingTasks() []CoordinatedTask {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	var pending []CoordinatedTask
	for _, t := range cc.plan {
		if t.Status == "pending" || t.Status == "retried" {
			pending = append(pending, t)
		}
	}
	return pending
}

// Plan returns a copy of the current plan.
func (cc *CentralCoordinator) Plan() []CoordinatedTask {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	result := make([]CoordinatedTask, len(cc.plan))
	copy(result, cc.plan)
	return result
}
