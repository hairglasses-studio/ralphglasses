package fleet

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestA2ATaskSend_CreatesWorkItem(t *testing.T) {
	coord := newTestCoordinator()

	payload := `{
		"id": "task-001",
		"message": {
			"role": "user",
			"parts": [{"type": "text", "text": "implement feature X"}]
		},
		"metadata": {
			"repo_name": "test-repo",
			"priority": 5,
			"max_budget_usd": 2.0
		}
	}`

	req := httptest.NewRequest("POST", "/api/v1/a2a/task/send", strings.NewReader(payload))
	w := httptest.NewRecorder()
	coord.handleA2ATaskSend(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp A2ATaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.ID != "task-001" {
		t.Errorf("expected task ID 'task-001', got %q", resp.ID)
	}
	if resp.Status != TaskStateQueued {
		t.Errorf("expected status %q, got %q", TaskStateQueued, resp.Status)
	}

	// Verify the work item was added to the queue.
	item, ok := coord.queue.Get("task-001")
	if !ok {
		t.Fatal("work item not found in queue")
	}
	if item.Prompt != "implement feature X" {
		t.Errorf("expected prompt 'implement feature X', got %q", item.Prompt)
	}
	if item.RepoName != "test-repo" {
		t.Errorf("expected repo_name 'test-repo', got %q", item.RepoName)
	}
	if item.Priority != 5 {
		t.Errorf("expected priority 5, got %d", item.Priority)
	}
	if item.MaxBudgetUSD != 2.0 {
		t.Errorf("expected max_budget_usd 2.0, got %.2f", item.MaxBudgetUSD)
	}
}

func TestA2ATaskSend_MissingID_Returns400(t *testing.T) {
	coord := newTestCoordinator()

	payload := `{
		"message": {
			"role": "user",
			"parts": [{"type": "text", "text": "do something"}]
		}
	}`

	req := httptest.NewRequest("POST", "/api/v1/a2a/task/send", strings.NewReader(payload))
	w := httptest.NewRecorder()
	coord.handleA2ATaskSend(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestA2ATaskSend_MissingMessage_Returns400(t *testing.T) {
	coord := newTestCoordinator()

	payload := `{"id": "task-no-msg"}`

	req := httptest.NewRequest("POST", "/api/v1/a2a/task/send", strings.NewReader(payload))
	w := httptest.NewRecorder()
	coord.handleA2ATaskSend(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing message, got %d: %s", w.Code, w.Body.String())
	}
}

func TestA2ATaskSend_InvalidJSON_Returns400(t *testing.T) {
	coord := newTestCoordinator()

	req := httptest.NewRequest("POST", "/api/v1/a2a/task/send", strings.NewReader("{not valid json"))
	w := httptest.NewRecorder()
	coord.handleA2ATaskSend(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestA2ATaskGet_ReturnsCorrectStatusMapping(t *testing.T) {
	coord := newTestCoordinator()

	tests := []struct {
		name       string
		workStatus WorkItemStatus
		wantA2A    TaskState
	}{
		{"pending maps to queued", WorkPending, TaskStateQueued},
		{"assigned maps to queued", WorkAssigned, TaskStateQueued},
		{"running maps to working", WorkRunning, TaskStateWorking},
		{"completed maps to completed", WorkCompleted, TaskStateCompleted},
		{"failed maps to failed", WorkFailed, TaskStateFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			itemID := "status-test-" + string(tt.workStatus)
			now := time.Now()
			item := &WorkItem{
				ID:          itemID,
				Type:        WorkTypeLoopTask,
				Status:      tt.workStatus,
				Prompt:      "test prompt",
				SubmittedAt: now,
			}
			coord.queue.Push(item)

			req := httptest.NewRequest("GET", "/api/v1/a2a/task/"+itemID, nil)
			req.SetPathValue("taskID", itemID)
			w := httptest.NewRecorder()
			coord.handleA2ATaskGet(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}

			var resp A2ATaskResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}

			if resp.Status != tt.wantA2A {
				t.Errorf("expected A2A status %q, got %q", tt.wantA2A, resp.Status)
			}
			if resp.ID != itemID {
				t.Errorf("expected ID %q, got %q", itemID, resp.ID)
			}
		})
	}
}

func TestA2ATaskGet_CompletedWithArtifacts(t *testing.T) {
	coord := newTestCoordinator()

	now := time.Now()
	item := &WorkItem{
		ID:          "completed-task",
		Type:        WorkTypeLoopTask,
		Status:      WorkCompleted,
		Prompt:      "build something",
		SubmittedAt: now,
		CompletedAt: &now,
		Result: &WorkResult{
			Output:    "built successfully",
			SpentUSD:  1.50,
			TurnCount: 3,
		},
	}
	coord.queue.Push(item)

	req := httptest.NewRequest("GET", "/api/v1/a2a/task/completed-task", nil)
	req.SetPathValue("taskID", "completed-task")
	w := httptest.NewRecorder()
	coord.handleA2ATaskGet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp A2ATaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Status != TaskStateCompleted {
		t.Errorf("expected status completed, got %q", resp.Status)
	}
	if len(resp.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(resp.Artifacts))
	}
	if resp.Artifacts[0].Name != "result" {
		t.Errorf("expected artifact name 'result', got %q", resp.Artifacts[0].Name)
	}
	if len(resp.Artifacts[0].Parts) != 1 || resp.Artifacts[0].Parts[0].Text != "built successfully" {
		t.Errorf("artifact content mismatch")
	}
}

func TestA2ATaskGet_NotFound_Returns404(t *testing.T) {
	coord := newTestCoordinator()

	req := httptest.NewRequest("GET", "/api/v1/a2a/task/nonexistent", nil)
	req.SetPathValue("taskID", "nonexistent")
	w := httptest.NewRecorder()
	coord.handleA2ATaskGet(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent task, got %d", w.Code)
	}
}

func TestA2ATaskCancel_PendingTask(t *testing.T) {
	coord := newTestCoordinator()

	now := time.Now()
	item := &WorkItem{
		ID:           "cancel-pending",
		Type:         WorkTypeLoopTask,
		Status:       WorkPending,
		Prompt:       "some work",
		MaxBudgetUSD: 3.0,
		SubmittedAt:  now,
	}
	coord.queue.Push(item)

	// Reserve budget (as handleA2ATaskSend would).
	coord.mu.Lock()
	coord.budget.ReservedUSD += 3.0
	coord.mu.Unlock()

	req := httptest.NewRequest("POST", "/api/v1/a2a/task/cancel-pending/cancel", nil)
	req.SetPathValue("taskID", "cancel-pending")
	w := httptest.NewRecorder()
	coord.handleA2ATaskCancel(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp A2ATaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Status != TaskStateCanceled {
		t.Errorf("expected status canceled, got %q", resp.Status)
	}

	// Verify the work item was updated in the queue.
	updated, ok := coord.queue.Get("cancel-pending")
	if !ok {
		t.Fatal("expected item to remain in queue")
	}
	if updated.Status != WorkFailed {
		t.Errorf("expected internal status failed, got %q", updated.Status)
	}
	if updated.Error != "canceled via A2A" {
		t.Errorf("expected error 'canceled via A2A', got %q", updated.Error)
	}

	// Verify budget was released.
	coord.mu.RLock()
	reserved := coord.budget.ReservedUSD
	coord.mu.RUnlock()
	if reserved != 0 {
		t.Errorf("expected reserved budget 0 after cancel, got %.2f", reserved)
	}
}

func TestA2ATaskCancel_RunningTask(t *testing.T) {
	coord := newTestCoordinator()

	now := time.Now()
	item := &WorkItem{
		ID:          "cancel-running",
		Type:        WorkTypeLoopTask,
		Status:      WorkRunning,
		Prompt:      "long running task",
		SubmittedAt: now,
		AssignedTo:  "worker-1",
		AssignedAt:  &now,
	}
	coord.queue.Push(item)

	req := httptest.NewRequest("POST", "/api/v1/a2a/task/cancel-running/cancel", nil)
	req.SetPathValue("taskID", "cancel-running")
	w := httptest.NewRecorder()
	coord.handleA2ATaskCancel(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp A2ATaskResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != TaskStateCanceled {
		t.Errorf("expected canceled, got %q", resp.Status)
	}
}

func TestA2ATaskCancel_CompletedTask_Returns409(t *testing.T) {
	coord := newTestCoordinator()

	now := time.Now()
	item := &WorkItem{
		ID:          "cancel-completed",
		Type:        WorkTypeLoopTask,
		Status:      WorkCompleted,
		Prompt:      "done already",
		SubmittedAt: now,
		CompletedAt: &now,
	}
	coord.queue.Push(item)

	req := httptest.NewRequest("POST", "/api/v1/a2a/task/cancel-completed/cancel", nil)
	req.SetPathValue("taskID", "cancel-completed")
	w := httptest.NewRecorder()
	coord.handleA2ATaskCancel(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for completed task, got %d: %s", w.Code, w.Body.String())
	}
}

func TestA2ATaskCancel_NotFound_Returns404(t *testing.T) {
	coord := newTestCoordinator()

	req := httptest.NewRequest("POST", "/api/v1/a2a/task/ghost/cancel", nil)
	req.SetPathValue("taskID", "ghost")
	w := httptest.NewRecorder()
	coord.handleA2ATaskCancel(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestA2ATaskSend_MultiPartMessage(t *testing.T) {
	coord := newTestCoordinator()

	payload := `{
		"id": "task-multi",
		"message": {
			"role": "user",
			"parts": [
				{"type": "text", "text": "First part."},
				{"type": "text", "text": "Second part."},
				{"type": "data", "data": {"key": "value"}, "mimeType": "application/json"}
			]
		}
	}`

	req := httptest.NewRequest("POST", "/api/v1/a2a/task/send", strings.NewReader(payload))
	w := httptest.NewRecorder()
	coord.handleA2ATaskSend(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	item, ok := coord.queue.Get("task-multi")
	if !ok {
		t.Fatal("work item not found")
	}

	expected := "First part.\nSecond part."
	if item.Prompt != expected {
		t.Errorf("expected prompt %q, got %q", expected, item.Prompt)
	}
}

func TestA2ATaskGet_IncludesMessage(t *testing.T) {
	coord := newTestCoordinator()

	now := time.Now()
	item := &WorkItem{
		ID:          "msg-check",
		Type:        WorkTypeLoopTask,
		Status:      WorkPending,
		Prompt:      "original prompt text",
		SubmittedAt: now,
	}
	coord.queue.Push(item)

	req := httptest.NewRequest("GET", "/api/v1/a2a/task/msg-check", nil)
	req.SetPathValue("taskID", "msg-check")
	w := httptest.NewRecorder()
	coord.handleA2ATaskGet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp A2ATaskResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Message == nil {
		t.Fatal("expected message in response")
	}
	if resp.Message.Role != MessageRoleUser {
		t.Errorf("expected role 'user', got %q", resp.Message.Role)
	}
	if len(resp.Message.Parts) != 1 || resp.Message.Parts[0].Text != "original prompt text" {
		t.Errorf("message parts mismatch")
	}
}
