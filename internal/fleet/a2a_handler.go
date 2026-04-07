package fleet

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
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
	RepoName         string          `json:"repo_name,omitempty"`
	RepoPath         string          `json:"repo_path,omitempty"`
	Source           string          `json:"source,omitempty"`
	Provider         string          `json:"provider,omitempty"`
	Model            string          `json:"model,omitempty"`
	MaxBudgetUSD     float64         `json:"max_budget_usd,omitempty"`
	Priority         int             `json:"priority,omitempty"`
	TeamName         string          `json:"team_name,omitempty"`
	TeamTaskID       string          `json:"team_task_id,omitempty"`
	PlannerSessionID string          `json:"planner_session_id,omitempty"`
	SessionName      string          `json:"session_name,omitempty"`
	PermissionMode   string          `json:"permission_mode,omitempty"`
	OutputSchema     json.RawMessage `json:"output_schema,omitempty"`
	WorktreePolicy   string          `json:"worktree_policy,omitempty"`
	TargetBranch     string          `json:"target_branch,omitempty"`
	HumanContext     []string        `json:"human_context,omitempty"`
	OwnedPaths       []string        `json:"owned_paths,omitempty"`
	SessionID        string          `json:"session_id,omitempty"`
	TaskStatus       string          `json:"task_status,omitempty"`
	Summary          string          `json:"summary,omitempty"`
	Question         string          `json:"question,omitempty"`
	ChangedFiles     []string        `json:"changed_files,omitempty"`
	WorkerNodeID     string          `json:"worker_node_id,omitempty"`
	WorktreePath     string          `json:"worktree_path,omitempty"`
	WorktreeBranch   string          `json:"worktree_branch,omitempty"`
	HeadSHA          string          `json:"head_sha,omitempty"`
	MergeBaseSHA     string          `json:"merge_base_sha,omitempty"`
	ArtifactType      string         `json:"artifact_type,omitempty"`
	ArtifactPath      string         `json:"artifact_path,omitempty"`
	ArtifactHash      string         `json:"artifact_hash,omitempty"`
	ArtifactSizeBytes int64          `json:"artifact_size_bytes,omitempty"`
	ArtifactBaseRef   string         `json:"artifact_base_ref,omitempty"`
	ArtifactTipRef    string         `json:"artifact_tip_ref,omitempty"`
	ArtifactStatus    string         `json:"artifact_status,omitempty"`
}

// A2ATaskResponse is the standard A2A v1.0 task response envelope.
type A2ATaskResponse struct {
	ID        string     `json:"id"`
	Status    TaskState  `json:"status"`
	Message   *Message   `json:"message,omitempty"`
	Artifacts []Artifact `json:"artifacts,omitempty"`
	Metadata  A2ATaskMetadata `json:"metadata,omitempty"`
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
		ID:               req.ID,
		Type:             WorkTypeSession,
		Source:           req.Metadata.Source,
		Status:           WorkPending,
		Priority:         req.Metadata.Priority,
		RepoName:         req.Metadata.RepoName,
		RepoPath:         req.Metadata.RepoPath,
		Prompt:           prompt,
		Provider:         session.Provider(req.Metadata.Provider),
		Model:            req.Metadata.Model,
		TeamName:         req.Metadata.TeamName,
		TeamTaskID:       req.Metadata.TeamTaskID,
		PlannerSessionID: req.Metadata.PlannerSessionID,
		SessionName:      req.Metadata.SessionName,
		PermissionMode:   req.Metadata.PermissionMode,
		OutputSchema:     req.Metadata.OutputSchema,
		WorktreePolicy:   req.Metadata.WorktreePolicy,
		TargetBranch:     req.Metadata.TargetBranch,
		HumanContext:     append([]string(nil), req.Metadata.HumanContext...),
		OwnedPaths:       append([]string(nil), req.Metadata.OwnedPaths...),
		MaxBudgetUSD:     req.Metadata.MaxBudgetUSD,
		MaxRetries:       2,
		SubmittedAt:      time.Now(),
		Constraints: WorkConstraints{
			RequireProvider: session.Provider(req.Metadata.Provider),
		},
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
		Metadata:  req.Metadata,
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
		Metadata:  workItemToA2AMetadata(item),
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
		Metadata:  workItemToA2AMetadata(item),
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

func workItemToA2AMetadata(item *WorkItem) A2ATaskMetadata {
	meta := A2ATaskMetadata{
		RepoName:         item.RepoName,
		RepoPath:         item.RepoPath,
		Source:           item.Source,
		Provider:         string(item.Provider),
		Model:            item.Model,
		MaxBudgetUSD:     item.MaxBudgetUSD,
		Priority:         item.Priority,
		TeamName:         item.TeamName,
		TeamTaskID:       item.TeamTaskID,
		PlannerSessionID: item.PlannerSessionID,
		SessionName:      item.SessionName,
		PermissionMode:   item.PermissionMode,
		OutputSchema:     item.OutputSchema,
		WorktreePolicy:   item.WorktreePolicy,
		TargetBranch:     item.TargetBranch,
		HumanContext:     append([]string(nil), item.HumanContext...),
		OwnedPaths:       append([]string(nil), item.OwnedPaths...),
		SessionID:        item.SessionID,
	}
	if item.Result != nil {
		meta.TaskStatus = item.Result.TaskStatus
		meta.Summary = item.Result.Summary
		meta.Question = item.Result.Question
		meta.ChangedFiles = append([]string(nil), item.Result.ChangedFiles...)
		meta.WorkerNodeID = item.Result.WorkerNodeID
		meta.WorktreePath = item.Result.WorktreePath
		meta.WorktreeBranch = item.Result.WorktreeBranch
		meta.HeadSHA = item.Result.HeadSHA
		meta.MergeBaseSHA = item.Result.MergeBaseSHA
		meta.ArtifactType = item.Result.ArtifactType
		meta.ArtifactPath = item.Result.ArtifactPath
		meta.ArtifactHash = item.Result.ArtifactHash
		meta.ArtifactSizeBytes = item.Result.ArtifactSizeBytes
		meta.ArtifactBaseRef = item.Result.ArtifactBaseRef
		meta.ArtifactTipRef = item.Result.ArtifactTipRef
		meta.ArtifactStatus = item.Result.ArtifactStatus
		meta.SessionID = firstNonEmpty(item.Result.SessionID, meta.SessionID)
	}
	return meta
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
