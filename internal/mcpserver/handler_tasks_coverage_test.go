package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTaskRegistry_CreateGetListCompleteFailCancel(t *testing.T) {
	t.Parallel()
	r := NewTaskRegistry()

	// Create
	id1 := r.Create("tool_a", nil, map[string]any{"key": "val"})
	id2 := r.Create("tool_b", nil, nil)

	if id1 == "" || id2 == "" {
		t.Fatal("expected non-empty task IDs")
	}
	if id1 == id2 {
		t.Fatal("expected unique task IDs")
	}

	// Get
	task := r.Get(id1)
	if task == nil {
		t.Fatal("expected task, got nil")
	}
	if task.ToolName != "tool_a" {
		t.Errorf("expected tool_a, got %q", task.ToolName)
	}
	if task.State != TaskRunning {
		t.Errorf("expected running, got %q", task.State)
	}

	// Get nonexistent
	if r.Get("nonexistent") != nil {
		t.Error("expected nil for nonexistent task")
	}

	// List all
	all := r.List("")
	if len(all) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(all))
	}

	// List filtered
	running := r.List(TaskRunning)
	if len(running) != 2 {
		t.Errorf("expected 2 running, got %d", len(running))
	}

	// Complete
	r.Complete(id1, map[string]any{"ok": true})
	task = r.Get(id1)
	if task.State != TaskCompleted {
		t.Errorf("expected completed, got %q", task.State)
	}

	// Fail
	r.Fail(id2, "something broke")
	task = r.Get(id2)
	if task.State != TaskFailed {
		t.Errorf("expected failed, got %q", task.State)
	}
	if task.Error != "something broke" {
		t.Errorf("expected error message, got %q", task.Error)
	}

	// List running (should be 0 now)
	running = r.List(TaskRunning)
	if len(running) != 0 {
		t.Errorf("expected 0 running, got %d", len(running))
	}
}

func TestTaskRegistry_Cancel_Extended(t *testing.T) {
	t.Parallel()
	r := NewTaskRegistry()

	canceled := false
	id := r.Create("tool", func() { canceled = true }, nil)

	if err := r.Cancel(id); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !canceled {
		t.Error("expected cancel func to be called")
	}
	task := r.Get(id)
	if task.State != TaskCanceled {
		t.Errorf("expected canceled, got %q", task.State)
	}

	// Cancel already canceled
	if err := r.Cancel(id); err == nil {
		t.Error("expected error canceling non-running task")
	}

	// Cancel nonexistent
	if err := r.Cancel("nonexistent"); err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestTaskRegistry_SetProgress(t *testing.T) {
	t.Parallel()
	r := NewTaskRegistry()
	id := r.Create("tool", nil, nil)

	r.SetProgress(id, 0.5)
	task := r.Get(id)
	if task.Progress != 0.5 {
		t.Errorf("expected progress 0.5, got %f", task.Progress)
	}
}

func TestTaskRegistry_RequestInput_Extended(t *testing.T) {
	t.Parallel()
	r := NewTaskRegistry()
	id := r.Create("tool", nil, nil)

	r.RequestInput(id, "please confirm")
	task := r.Get(id)
	if task.State != TaskInputRequired {
		t.Errorf("expected input_required, got %q", task.State)
	}
	if task.Metadata["input_prompt"] != "please confirm" {
		t.Errorf("expected input prompt in metadata")
	}
}

func TestTaskRegistry_Prune_Extended(t *testing.T) {
	t.Parallel()
	r := NewTaskRegistry()

	id1 := r.Create("tool_a", nil, nil)
	id2 := r.Create("tool_b", nil, nil)
	r.Create("tool_c", nil, nil) // still running

	r.Complete(id1, "done")
	r.Fail(id2, "error")

	// Prune with 0 maxAge should remove completed/failed tasks.
	pruned := r.Prune(0)
	if pruned != 2 {
		t.Errorf("expected 2 pruned, got %d", pruned)
	}

	// Running task should remain.
	all := r.List("")
	if len(all) != 1 {
		t.Errorf("expected 1 remaining task, got %d", len(all))
	}
}

func TestTaskRegistry_RequestInput_NilMetadata(t *testing.T) {
	t.Parallel()
	r := NewTaskRegistry()
	id := r.Create("tool", nil, nil) // nil metadata

	r.RequestInput(id, "test prompt")
	task := r.Get(id)
	if task.Metadata == nil {
		t.Fatal("expected metadata to be initialized")
	}
}

// --- MCP handler tests ---

func TestHandleTasksGet_MissingID(t *testing.T) {
	t.Parallel()
	srv := &Server{Tasks: NewTaskRegistry()}
	result, err := srv.handleTasksGet(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing task_id")
	}
}

func TestHandleTasksGet_NotFound(t *testing.T) {
	t.Parallel()
	srv := &Server{Tasks: NewTaskRegistry()}
	result, err := srv.handleTasksGet(context.Background(), makeRequest(map[string]any{
		"task_id": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent task")
	}
	text := getResultText(result)
	if !strings.Contains(text, "not found") {
		t.Errorf("expected 'not found', got: %s", text)
	}
}

func TestHandleTasksGet_NilRegistry(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handleTasksGet(context.Background(), makeRequest(map[string]any{
		"task_id": "test",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nil registry")
	}
}

func TestHandleTasksList_Empty(t *testing.T) {
	t.Parallel()
	srv := &Server{Tasks: NewTaskRegistry()}
	result, err := srv.handleTasksList(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"count":0`) {
		t.Errorf("expected count:0, got: %s", text)
	}
}

func TestHandleTasksList_NilRegistry(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handleTasksList(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nil registry")
	}
}

func TestHandleTasksCancel_MissingID(t *testing.T) {
	t.Parallel()
	srv := &Server{Tasks: NewTaskRegistry()}
	result, err := srv.handleTasksCancel(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing task_id")
	}
}

func TestHandleTasksCancel_NilRegistry(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handleTasksCancel(context.Background(), makeRequest(map[string]any{
		"task_id": "test",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nil registry")
	}
}

func TestHandleTasksCancel_Valid(t *testing.T) {
	t.Parallel()
	reg := NewTaskRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ctx
	id := reg.Create("test_tool", cancel, nil)

	srv := &Server{Tasks: reg}
	result, err := srv.handleTasksCancel(context.Background(), makeRequest(map[string]any{
		"task_id": id,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "canceled") {
		t.Errorf("expected 'canceled', got: %s", text)
	}
}

func TestTaskRegistry_Prune_WithAge(t *testing.T) {
	t.Parallel()
	r := NewTaskRegistry()
	id := r.Create("tool", nil, nil)
	r.Complete(id, "done")

	// Prune with large maxAge should not remove recent tasks.
	pruned := r.Prune(time.Hour)
	if pruned != 0 {
		t.Errorf("expected 0 pruned with large maxAge, got %d", pruned)
	}
}
