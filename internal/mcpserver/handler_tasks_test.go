package mcpserver

import (
	"testing"
	"time"
)

func TestTaskRegistry_CreateAndGet(t *testing.T) {
	r := NewTaskRegistry()

	id := r.Create("loop_start", nil, map[string]any{"loop_id": "loop-1"})
	if id == "" {
		t.Fatal("expected non-empty task ID")
	}

	task := r.Get(id)
	if task == nil {
		t.Fatal("expected task, got nil")
	}
	if task.ToolName != "loop_start" {
		t.Errorf("ToolName = %q, want %q", task.ToolName, "loop_start")
	}
	if task.State != TaskRunning {
		t.Errorf("State = %q, want %q", task.State, TaskRunning)
	}
	if task.Metadata["loop_id"] != "loop-1" {
		t.Errorf("Metadata loop_id = %v, want %q", task.Metadata["loop_id"], "loop-1")
	}
}

func TestTaskRegistry_GetNotFound(t *testing.T) {
	r := NewTaskRegistry()
	if r.Get("nonexistent") != nil {
		t.Error("expected nil for missing task")
	}
}

func TestTaskRegistry_Complete(t *testing.T) {
	r := NewTaskRegistry()
	id := r.Create("test_tool", nil, nil)

	r.Complete(id, map[string]any{"result": "ok"})

	task := r.Get(id)
	if task.State != TaskCompleted {
		t.Errorf("State = %q, want %q", task.State, TaskCompleted)
	}
}

func TestTaskRegistry_Fail(t *testing.T) {
	r := NewTaskRegistry()
	id := r.Create("test_tool", nil, nil)

	r.Fail(id, "something went wrong")

	task := r.Get(id)
	if task.State != TaskFailed {
		t.Errorf("State = %q, want %q", task.State, TaskFailed)
	}
	if task.Error != "something went wrong" {
		t.Errorf("Error = %q, want %q", task.Error, "something went wrong")
	}
}

func TestTaskRegistry_Cancel(t *testing.T) {
	r := NewTaskRegistry()
	canceled := false
	id := r.Create("test_tool", func() { canceled = true }, nil)

	err := r.Cancel(id)
	if err != nil {
		t.Fatalf("Cancel error: %v", err)
	}

	task := r.Get(id)
	if task.State != TaskCanceled {
		t.Errorf("State = %q, want %q", task.State, TaskCanceled)
	}
	if !canceled {
		t.Error("expected cancel function to be called")
	}
}

func TestTaskRegistry_CancelCompleted(t *testing.T) {
	r := NewTaskRegistry()
	id := r.Create("test_tool", nil, nil)
	r.Complete(id, nil)

	err := r.Cancel(id)
	if err == nil {
		t.Error("expected error when canceling completed task")
	}
}

func TestTaskRegistry_List(t *testing.T) {
	r := NewTaskRegistry()
	r.Create("tool_a", nil, nil)
	id2 := r.Create("tool_b", nil, nil)
	r.Complete(id2, nil)

	all := r.List("")
	if len(all) != 2 {
		t.Errorf("List all = %d, want 2", len(all))
	}

	running := r.List(TaskRunning)
	if len(running) != 1 {
		t.Errorf("List running = %d, want 1", len(running))
	}

	completed := r.List(TaskCompleted)
	if len(completed) != 1 {
		t.Errorf("List completed = %d, want 1", len(completed))
	}
}

func TestTaskRegistry_Progress(t *testing.T) {
	r := NewTaskRegistry()
	id := r.Create("test_tool", nil, nil)

	r.SetProgress(id, 0.5)
	task := r.Get(id)
	if task.Progress != 0.5 {
		t.Errorf("Progress = %f, want 0.5", task.Progress)
	}
}

func TestTaskRegistry_RequestInput(t *testing.T) {
	r := NewTaskRegistry()
	id := r.Create("test_tool", nil, nil)

	r.RequestInput(id, "Please provide the API key")

	task := r.Get(id)
	if task.State != TaskInputRequired {
		t.Errorf("State = %q, want %q", task.State, TaskInputRequired)
	}
	if task.Metadata["input_prompt"] != "Please provide the API key" {
		t.Errorf("input_prompt = %v", task.Metadata["input_prompt"])
	}
}

func TestTaskRegistry_Prune(t *testing.T) {
	r := NewTaskRegistry()
	id1 := r.Create("old_tool", nil, nil)
	r.Complete(id1, nil)
	// Simulate old task by manipulating UpdatedAt
	r.mu.Lock()
	r.tasks[id1].UpdatedAt = time.Now().Add(-2 * time.Hour)
	r.mu.Unlock()

	r.Create("new_tool", nil, nil) // still running, should not be pruned

	pruned := r.Prune(1 * time.Hour)
	if pruned != 1 {
		t.Errorf("Prune = %d, want 1", pruned)
	}

	all := r.List("")
	if len(all) != 1 {
		t.Errorf("after prune, List = %d, want 1", len(all))
	}
}
