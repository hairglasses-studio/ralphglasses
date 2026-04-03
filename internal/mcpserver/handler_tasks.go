package mcpserver

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// TaskState represents the lifecycle state of an async MCP task.
type TaskState string

const (
	TaskRunning       TaskState = "running"
	TaskCompleted     TaskState = "completed"
	TaskFailed        TaskState = "failed"
	TaskCanceled      TaskState = "canceled"
	TaskInputRequired TaskState = "input_required"
)

// AsyncTask tracks a long-running tool invocation that returns immediately
// with a task ID and can be polled for status.
type AsyncTask struct {
	ID        string         `json:"id"`
	ToolName  string         `json:"tool_name"`
	State     TaskState      `json:"state"`
	Progress  float64        `json:"progress,omitempty"` // 0.0-1.0
	Result    any            `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Metadata  map[string]any `json:"metadata,omitempty"` // tool-specific data (e.g. session_id, loop_id)

	cancel context.CancelFunc `json:"-"`
}

// TaskRegistry manages async MCP tasks for long-running tool operations.
// It implements the MCP Tasks primitive (tasks/get, tasks/list, tasks/cancel).
type TaskRegistry struct {
	mu    sync.RWMutex
	tasks map[string]*AsyncTask
	seq   int64
}

// NewTaskRegistry creates an empty task registry.
func NewTaskRegistry() *TaskRegistry {
	return &TaskRegistry{
		tasks: make(map[string]*AsyncTask),
	}
}

// Create registers a new async task and returns its ID.
func (r *TaskRegistry) Create(toolName string, cancel context.CancelFunc, metadata map[string]any) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	id := fmt.Sprintf("task-%d-%d", time.Now().UnixMilli(), r.seq)

	r.tasks[id] = &AsyncTask{
		ID:        id,
		ToolName:  toolName,
		State:     TaskRunning,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  metadata,
		cancel:    cancel,
	}

	return id
}

// Get returns a task by ID, or nil if not found.
func (r *TaskRegistry) Get(id string) *AsyncTask {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t := r.tasks[id]
	if t == nil {
		return nil
	}
	// Return a copy to avoid race conditions
	copy := *t
	return &copy
}

// List returns all tasks, optionally filtered by state.
func (r *TaskRegistry) List(stateFilter TaskState) []*AsyncTask {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*AsyncTask, 0, len(r.tasks))
	for _, t := range r.tasks {
		if stateFilter != "" && t.State != stateFilter {
			continue
		}
		copy := *t
		result = append(result, &copy)
	}
	return result
}

// Complete marks a task as completed with the given result.
func (r *TaskRegistry) Complete(id string, result any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if t, ok := r.tasks[id]; ok {
		t.State = TaskCompleted
		t.Result = result
		t.UpdatedAt = time.Now()
	}
}

// Fail marks a task as failed with the given error message.
func (r *TaskRegistry) Fail(id string, errMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if t, ok := r.tasks[id]; ok {
		t.State = TaskFailed
		t.Error = errMsg
		t.UpdatedAt = time.Now()
	}
}

// Cancel cancels a running task.
func (r *TaskRegistry) Cancel(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	if t.State != TaskRunning && t.State != TaskInputRequired {
		return fmt.Errorf("task %s is %s, cannot cancel", id, t.State)
	}

	t.State = TaskCanceled
	t.UpdatedAt = time.Now()
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}

// SetProgress updates a task's progress indicator.
func (r *TaskRegistry) SetProgress(id string, progress float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if t, ok := r.tasks[id]; ok {
		t.Progress = progress
		t.UpdatedAt = time.Now()
	}
}

// RequestInput puts a task in input_required state for HITL workflows.
func (r *TaskRegistry) RequestInput(id string, prompt string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if t, ok := r.tasks[id]; ok {
		t.State = TaskInputRequired
		if t.Metadata == nil {
			t.Metadata = make(map[string]any)
		}
		t.Metadata["input_prompt"] = prompt
		t.UpdatedAt = time.Now()
	}
}

// Prune removes completed/failed/canceled tasks older than maxAge.
func (r *TaskRegistry) Prune(maxAge time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	pruned := 0
	for id, t := range r.tasks {
		if t.State != TaskRunning && t.State != TaskInputRequired && t.UpdatedAt.Before(cutoff) {
			delete(r.tasks, id)
			pruned++
		}
	}
	return pruned
}

// --- MCP Tool Handlers ---

func (s *Server) handleTasksGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID := getStringArg(req, "task_id")
	if taskID == "" {
		return codedError(ErrInvalidParams, "task_id required"), nil
	}

	if s.Tasks == nil {
		return codedError(ErrInternal, "task registry not initialized"), nil
	}

	task := s.Tasks.Get(taskID)
	if task == nil {
		return codedError(ErrServiceNotFound, "task not found: "+taskID), nil
	}

	return jsonResult(task), nil
}

func (s *Server) handleTasksList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.Tasks == nil {
		return codedError(ErrInternal, "task registry not initialized"), nil
	}

	stateFilter := TaskState(getStringArg(req, "state"))
	tasks := s.Tasks.List(stateFilter)

	return jsonResult(map[string]any{
		"tasks": tasks,
		"count": len(tasks),
	}), nil
}

func (s *Server) handleTasksCancel(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID := getStringArg(req, "task_id")
	if taskID == "" {
		return codedError(ErrInvalidParams, "task_id required"), nil
	}

	if s.Tasks == nil {
		return codedError(ErrInternal, "task registry not initialized"), nil
	}

	if err := s.Tasks.Cancel(taskID); err != nil {
		return codedError(ErrInternal, err.Error()), nil
	}

	return jsonResult(map[string]any{
		"task_id": taskID,
		"state":   "canceled",
	}), nil
}
