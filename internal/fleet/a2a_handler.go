package fleet

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// A2ATaskSendRequest is the payload for POST /api/v1/a2a/task/send.
// It wraps an A2A v1.0 Task with the required fields for submission.
type A2ATaskSendRequest struct {
	ID      string  `json:"id"`
	Message Message `json:"message"`

	// Optional fields for task routing and budget control.
	Metadata A2ATaskMetadata `json:"metadata"`
}

// A2ATaskMetadata carries optional routing hints for a task submission.
type A2ATaskMetadata struct {
	RepoName     string  `json:"repo_name,omitempty"`
	Provider     string  `json:"provider,omitempty"`
	MaxBudgetUSD float64 `json:"max_budget_usd,omitempty"`
	Priority     int     `json:"priority,omitempty"`
}

// A2ATaskResponse is the standard A2A v1.0 task response envelope.
type A2ATaskResponse struct {
	ID        string     `json:"id"`
	Status    TaskState  `json:"status"`
	Message   *Message   `json:"message,omitempty"`
	Artifacts []Artifact `json:"artifacts,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// handleA2ATaskSend accepts a task from another agent, converts it to a
// WorkItem, and submits it to the work queue.
// POST /api/v1/a2a/task/send
func (c *Coordinator) handleA2ATaskSend(w http.ResponseWriter, r *http.Request) {
	var req A2ATaskSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields.
	if req.ID == "" {
		http.Error(w, "missing required field: id", http.StatusBadRequest)
		return
	}
	if len(req.Message.Parts) == 0 {
		http.Error(w, "missing required field: message (must contain at least one part)", http.StatusBadRequest)
		return
	}

	// Extract the prompt text from message parts.
	prompt := extractPromptFromParts(req.Message.Parts)

	// Build a WorkItem from the A2A task.
	item := &WorkItem{
		ID:           req.ID,
		Type:         WorkTypeLoopTask,
		Status:       WorkPending,
		Priority:     req.Metadata.Priority,
		RepoName:     req.Metadata.RepoName,
		Prompt:       prompt,
		MaxBudgetUSD: req.Metadata.MaxBudgetUSD,
		MaxRetries:   2,
		SubmittedAt:  time.Now(),
	}

	// Budget gate.
	c.mu.RLock()
	avail := c.budget.AvailableBudget()
	c.mu.RUnlock()

	if item.MaxBudgetUSD > 0 && item.MaxBudgetUSD > avail {
		http.Error(w, "insufficient budget", http.StatusPaymentRequired)
		return
	}

	c.queue.Push(item)

	// Reserve budget.
	if item.MaxBudgetUSD > 0 {
		c.mu.Lock()
		c.budget.ReservedUSD += item.MaxBudgetUSD
		c.budget.LastUpdated = time.Now()
		c.mu.Unlock()
	}

	if c.bus != nil {
		c.bus.Publish(events.Event{
			Type:     "fleet.a2a_task_submitted",
			RepoName: item.RepoName,
			Data:     map[string]any{"task_id": item.ID, "source": "a2a"},
		})
	}

	resp := A2ATaskResponse{
		ID:        item.ID,
		Status:    TaskStateQueued,
		CreatedAt: item.SubmittedAt,
		UpdatedAt: item.SubmittedAt,
	}

	w.WriteHeader(http.StatusOK)
	writeJSON(w, resp)
}

// handleA2ATaskGet returns the full A2A task status for a given task ID.
// GET /api/v1/a2a/task/{taskID}
func (c *Coordinator) handleA2ATaskGet(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("taskID")
	if taskID == "" {
		http.Error(w, "missing task ID", http.StatusBadRequest)
		return
	}

	item, ok := c.queue.Get(taskID)
	if !ok {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	resp := workItemToA2AResponse(item)
	writeJSON(w, resp)
}

// handleA2ATaskCancel cancels a running or pending task.
// POST /api/v1/a2a/task/{taskID}/cancel
func (c *Coordinator) handleA2ATaskCancel(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("taskID")
	if taskID == "" {
		http.Error(w, "missing task ID", http.StatusBadRequest)
		return
	}

	item, ok := c.queue.Get(taskID)
	if !ok {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	// Only non-terminal tasks can be canceled.
	if item.Status == WorkCompleted || item.Status == WorkFailed {
		http.Error(w, "task is already in a terminal state", http.StatusConflict)
		return
	}

	now := time.Now()
	item.Status = WorkFailed
	item.Error = "canceled via A2A"
	item.CompletedAt = &now
	item.AssignedTo = ""
	c.queue.Update(item)

	// Release reserved budget.
	if item.MaxBudgetUSD > 0 {
		c.mu.Lock()
		c.budget.ReservedUSD -= item.MaxBudgetUSD
		if c.budget.ReservedUSD < 0 {
			c.budget.ReservedUSD = 0
		}
		c.budget.LastUpdated = now
		c.mu.Unlock()
	}

	if c.bus != nil {
		c.bus.Publish(events.Event{
			Type: "fleet.a2a_task_canceled",
			Data: map[string]any{"task_id": item.ID},
		})
	}

	resp := A2ATaskResponse{
		ID:        item.ID,
		Status:    TaskStateCanceled,
		CreatedAt: item.SubmittedAt,
		UpdatedAt: now,
	}

	writeJSON(w, resp)
}

// workItemToA2AResponse maps an internal WorkItem to an A2A v1.0 task response.
func workItemToA2AResponse(item *WorkItem) A2ATaskResponse {
	resp := A2ATaskResponse{
		ID:        item.ID,
		Status:    workStatusToTaskState(item.Status),
		CreatedAt: item.SubmittedAt,
		UpdatedAt: item.SubmittedAt,
	}

	// Set the most recent timestamp as UpdatedAt.
	if item.CompletedAt != nil {
		resp.UpdatedAt = *item.CompletedAt
	} else if item.AssignedAt != nil {
		resp.UpdatedAt = *item.AssignedAt
	}

	// Include result output as an artifact if the task completed.
	if item.Status == WorkCompleted && item.Result != nil && item.Result.Output != "" {
		resp.Artifacts = []Artifact{
			{
				Name:  "result",
				Type:  "text/plain",
				Parts: []Part{NewTextPart(item.Result.Output)},
				Index: 0,
				Final: true,
			},
		}
	}

	// Reconstruct the original message from prompt if available.
	if item.Prompt != "" {
		resp.Message = &Message{
			Role:  MessageRoleUser,
			Parts: []Part{NewTextPart(item.Prompt)},
		}
	}

	return resp
}

// workStatusToTaskState maps internal WorkItemStatus to A2A v1.0 TaskState.
func workStatusToTaskState(s WorkItemStatus) TaskState {
	switch s {
	case WorkPending:
		return TaskStateQueued
	case WorkAssigned:
		return TaskStateQueued
	case WorkRunning:
		return TaskStateWorking
	case WorkCompleted:
		return TaskStateCompleted
	case WorkFailed:
		return TaskStateFailed
	default:
		return TaskStateQueued
	}
}

// extractPromptFromParts concatenates all text parts in a message to form the prompt.
func extractPromptFromParts(parts []Part) string {
	var prompt string
	for _, p := range parts {
		if p.Type == PartTypeText && p.Text != "" {
			if prompt != "" {
				prompt += "\n"
			}
			prompt += p.Text
		}
	}
	return prompt
}
