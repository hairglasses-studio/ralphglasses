package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// A2A JSON-RPC method constants per the A2A v1.0 specification.
const (
	MethodTasksSend       = "tasks/send"
	MethodTasksGet        = "tasks/get"
	MethodTasksCancel     = "tasks/cancel"
	MethodTasksSendSubscribe = "tasks/sendSubscribe"

	// JSON-RPC version.
	JSONRPCVersion = "2.0"
)

// JSON-RPC error codes per the A2A spec.
const (
	ErrCodeParse          = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603

	// A2A-specific error codes.
	ErrCodeTaskNotFound      = -32001
	ErrCodeTaskNotCancelable = -32002
	ErrCodeInvalidTransition2 = -32003 // renamed to avoid collision with package-level var
)

// JSONRPCRequest represents a JSON-RPC 2.0 request per A2A spec.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is the error object in a JSON-RPC 2.0 response.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// TaskSendParams are the parameters for the tasks/send method.
type TaskSendParams struct {
	ID       string  `json:"id"`
	Message  Message `json:"message"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// TaskGetParams are the parameters for the tasks/get method.
type TaskGetParams struct {
	ID string `json:"id"`
}

// TaskCancelParams are the parameters for the tasks/cancel method.
type TaskCancelParams struct {
	ID string `json:"id"`
}

// A2ATask is the full A2A v1.0 Task object returned in JSON-RPC responses.
type A2ATask struct {
	ID        string          `json:"id"`
	Status    A2ATaskStatus   `json:"status"`
	Artifacts []Artifact      `json:"artifacts,omitempty"`
	History   []Message       `json:"history,omitempty"`
	Metadata  map[string]any  `json:"metadata,omitempty"`
}

// A2ATaskStatus represents the status sub-object within a Task.
type A2ATaskStatus struct {
	State   TaskState `json:"state"`
	Message *Message  `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// executorTask is the internal representation of a task managed by the executor.
type executorTask struct {
	mu        sync.Mutex
	id        string
	state     TaskState
	history   []Message
	artifacts []Artifact
	metadata  map[string]any
	createdAt time.Time
	updatedAt time.Time
	statusMsg *Message // optional agent message for current status
}

// A2AExecutor implements the A2A protocol's JSON-RPC transport layer.
// It manages task lifecycle (submit, working, done, failed) and serves
// the agent card at the well-known discovery endpoint.
//
// The executor is self-contained with no external dependencies beyond
// the standard library. All A2A protocol types are defined inline.
type A2AExecutor struct {
	mu    sync.RWMutex
	tasks map[string]*executorTask
	card  AgentCard

	// Optional hook called when a new task is submitted.
	// Implementations should start async work and call TransitionTask
	// when the task progresses. Nil means tasks stay in queued state.
	OnTaskSubmitted func(ctx context.Context, task A2ATask)
}

// A2AExecutorConfig holds configuration for creating a new A2AExecutor.
type A2AExecutorConfig struct {
	Name         string
	Description  string
	URL          string
	Version      string
	Skills       []AgentSkill
	Provider     AgentProvider
	Capabilities AgentCapabilities
}

// NewA2AExecutor creates an executor with the given configuration.
// The executor generates an agent card from the config and is ready
// to handle JSON-RPC requests.
func NewA2AExecutor(cfg A2AExecutorConfig) *A2AExecutor {
	card := AgentCard{
		Name:        cfg.Name,
		Description: cfg.Description,
		URL:         cfg.URL,
		Version:     cfg.Version,
		SupportedInterfaces: []string{"a2a/v1"},
		Capabilities: cfg.Capabilities,
		Skills:       cfg.Skills,
		Provider:     cfg.Provider,
		SecuritySchemes: map[string]SecurityScheme{
			"bearer": {
				Type:   "http",
				Scheme: "bearer",
			},
		},
		Security: []map[string][]string{
			{"bearer": {}},
		},
	}

	return &A2AExecutor{
		tasks: make(map[string]*executorTask),
		card:  card,
	}
}

// Card returns the executor's agent card for discovery.
func (e *A2AExecutor) Card() AgentCard {
	return e.card
}

// ServeHTTP implements http.Handler, routing requests to the appropriate
// A2A endpoint: agent card discovery or JSON-RPC dispatch.
func (e *A2AExecutor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == AgentCardDiscoveryPath && r.Method == http.MethodGet:
		e.handleAgentCard(w, r)
	case r.URL.Path == "/a2a" && r.Method == http.MethodPost:
		e.handleJSONRPC(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleAgentCard serves the agent card at the well-known discovery path.
func (e *A2AExecutor) handleAgentCard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(e.card)
}

// handleJSONRPC dispatches incoming JSON-RPC 2.0 requests to the appropriate
// method handler per the A2A specification.
func (e *A2AExecutor) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONRPCError(w, nil, ErrCodeParse, "parse error: "+err.Error())
		return
	}

	if req.JSONRPC != JSONRPCVersion {
		writeJSONRPCError(w, req.ID, ErrCodeInvalidRequest, "invalid jsonrpc version")
		return
	}

	switch req.Method {
	case MethodTasksSend:
		e.handleTasksSend(w, req)
	case MethodTasksGet:
		e.handleTasksGet(w, req)
	case MethodTasksCancel:
		e.handleTasksCancel(w, req)
	default:
		writeJSONRPCError(w, req.ID, ErrCodeMethodNotFound, "method not found: "+req.Method)
	}
}

// handleTasksSend creates a new task or sends a message to an existing one.
func (e *A2AExecutor) handleTasksSend(w http.ResponseWriter, req JSONRPCRequest) {
	var params TaskSendParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, ErrCodeInvalidParams, "invalid params: "+err.Error())
		return
	}

	if params.ID == "" {
		writeJSONRPCError(w, req.ID, ErrCodeInvalidParams, "missing required field: id")
		return
	}

	if len(params.Message.Parts) == 0 {
		writeJSONRPCError(w, req.ID, ErrCodeInvalidParams, "message must contain at least one part")
		return
	}

	now := time.Now()

	e.mu.Lock()
	task, exists := e.tasks[params.ID]
	if !exists {
		task = &executorTask{
			id:        params.ID,
			state:     TaskStateQueued,
			history:   []Message{params.Message},
			metadata:  params.Metadata,
			createdAt: now,
			updatedAt: now,
		}
		e.tasks[params.ID] = task
		e.mu.Unlock()

		a2aTask := e.toA2ATask(task)
		if e.OnTaskSubmitted != nil {
			go e.OnTaskSubmitted(context.Background(), a2aTask)
		}

		writeJSONRPCResult(w, req.ID, a2aTask)
		return
	}
	e.mu.Unlock()

	// Existing task: append message (e.g., providing input for input-required state).
	task.mu.Lock()
	task.history = append(task.history, params.Message)
	task.updatedAt = now

	// If the task was waiting for input, transition back to working.
	if task.state == TaskStateInputRequired {
		task.state = TaskStateWorking
	}
	task.mu.Unlock()

	writeJSONRPCResult(w, req.ID, e.toA2ATask(task))
}

// handleTasksGet retrieves the current state of a task by ID.
func (e *A2AExecutor) handleTasksGet(w http.ResponseWriter, req JSONRPCRequest) {
	var params TaskGetParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, ErrCodeInvalidParams, "invalid params: "+err.Error())
		return
	}

	e.mu.RLock()
	task, ok := e.tasks[params.ID]
	e.mu.RUnlock()

	if !ok {
		writeJSONRPCError(w, req.ID, ErrCodeTaskNotFound, "task not found: "+params.ID)
		return
	}

	writeJSONRPCResult(w, req.ID, e.toA2ATask(task))
}

// handleTasksCancel attempts to cancel a task.
func (e *A2AExecutor) handleTasksCancel(w http.ResponseWriter, req JSONRPCRequest) {
	var params TaskCancelParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, ErrCodeInvalidParams, "invalid params: "+err.Error())
		return
	}

	e.mu.RLock()
	task, ok := e.tasks[params.ID]
	e.mu.RUnlock()

	if !ok {
		writeJSONRPCError(w, req.ID, ErrCodeTaskNotFound, "task not found: "+params.ID)
		return
	}

	task.mu.Lock()
	if task.state == TaskStateCompleted || task.state == TaskStateFailed || task.state == TaskStateCanceled {
		task.mu.Unlock()
		writeJSONRPCError(w, req.ID, ErrCodeTaskNotCancelable, "task is in terminal state: "+string(task.state))
		return
	}
	task.state = TaskStateCanceled
	task.updatedAt = time.Now()
	task.mu.Unlock()

	writeJSONRPCResult(w, req.ID, e.toA2ATask(task))
}

// TransitionTask moves a task to a new state. This is called by task
// executors (via OnTaskSubmitted hook) as work progresses.
//
// Valid transitions per A2A spec:
//
//	queued -> working
//	working -> completed | failed | input-required | canceled
//	input-required -> working | canceled | failed
func (e *A2AExecutor) TransitionTask(taskID string, newState TaskState, agentMsg *Message) error {
	e.mu.RLock()
	task, ok := e.tasks[taskID]
	e.mu.RUnlock()

	if !ok {
		return ErrOfferNotFound
	}

	task.mu.Lock()
	defer task.mu.Unlock()

	if err := validateTransition(task.state, newState); err != nil {
		return err
	}

	task.state = newState
	task.updatedAt = time.Now()
	task.statusMsg = agentMsg

	if agentMsg != nil {
		task.history = append(task.history, *agentMsg)
	}

	return nil
}

// AddTaskArtifact appends an artifact to a task that is in working state.
func (e *A2AExecutor) AddTaskArtifact(taskID string, art Artifact) error {
	e.mu.RLock()
	task, ok := e.tasks[taskID]
	e.mu.RUnlock()

	if !ok {
		return ErrOfferNotFound
	}

	task.mu.Lock()
	defer task.mu.Unlock()

	if task.state != TaskStateWorking && task.state != TaskStateInputRequired {
		return fmt.Errorf("a2a: cannot add artifact in state %s", task.state)
	}

	if art.Index == 0 && len(task.artifacts) > 0 {
		art.Index = len(task.artifacts)
	}

	task.artifacts = append(task.artifacts, art)
	task.updatedAt = time.Now()
	return nil
}

// GetTask retrieves the A2A task representation by ID.
func (e *A2AExecutor) GetTask(taskID string) (A2ATask, bool) {
	e.mu.RLock()
	task, ok := e.tasks[taskID]
	e.mu.RUnlock()

	if !ok {
		return A2ATask{}, false
	}

	return e.toA2ATask(task), true
}

// TaskCount returns the total number of tasks managed by the executor.
func (e *A2AExecutor) TaskCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.tasks)
}

// TaskCountByState returns task counts grouped by state.
func (e *A2AExecutor) TaskCountByState() map[TaskState]int {
	e.mu.RLock()
	defer e.mu.RUnlock()

	counts := make(map[TaskState]int)
	for _, t := range e.tasks {
		t.mu.Lock()
		counts[t.state]++
		t.mu.Unlock()
	}
	return counts
}

// toA2ATask converts internal task representation to the A2A wire format.
func (e *A2AExecutor) toA2ATask(t *executorTask) A2ATask {
	t.mu.Lock()
	defer t.mu.Unlock()

	history := make([]Message, len(t.history))
	copy(history, t.history)

	artifacts := make([]Artifact, len(t.artifacts))
	copy(artifacts, t.artifacts)

	return A2ATask{
		ID: t.id,
		Status: A2ATaskStatus{
			State:     t.state,
			Message:   t.statusMsg,
			Timestamp: t.updatedAt,
		},
		Artifacts: artifacts,
		History:   history,
		Metadata:  t.metadata,
	}
}

// validateTransition checks whether a state transition is valid per A2A spec.
func validateTransition(from, to TaskState) error {
	switch from {
	case TaskStateQueued:
		if to == TaskStateWorking || to == TaskStateCanceled || to == TaskStateFailed {
			return nil
		}
	case TaskStateWorking:
		if to == TaskStateCompleted || to == TaskStateFailed || to == TaskStateInputRequired || to == TaskStateCanceled {
			return nil
		}
	case TaskStateInputRequired:
		if to == TaskStateWorking || to == TaskStateCanceled || to == TaskStateFailed {
			return nil
		}
	case TaskStateCompleted, TaskStateFailed, TaskStateCanceled:
		return errors.New("a2a: cannot transition from terminal state " + string(from))
	}

	return fmt.Errorf("a2a: invalid transition from %s to %s", from, to)
}

// writeJSONRPCResult sends a successful JSON-RPC 2.0 response.
func writeJSONRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	resp := JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  result,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// writeJSONRPCError sends a JSON-RPC 2.0 error response.
func writeJSONRPCError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
	w.Header().Set("Content-Type", "application/json")

	// Map error codes to HTTP status codes.
	httpStatus := http.StatusOK // JSON-RPC errors use 200 by convention
	if code == ErrCodeParse || code == ErrCodeInvalidRequest {
		httpStatus = http.StatusBadRequest
	}

	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(resp)
}
