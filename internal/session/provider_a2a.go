package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/hairglasses-studio/mcpkit/a2a"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// A2ASessionConfig configures an A2A-backed session. Unlike CLI-based providers,
// A2A sessions communicate over HTTP using the Agent-to-Agent protocol, sending
// tasks to remote agents and polling for results.
type A2ASessionConfig struct {
	// AgentURL is the base URL of the remote A2A agent (e.g., "http://localhost:8080").
	// The client will fetch /.well-known/agent.json for capability discovery.
	AgentURL string

	// AuthToken is an optional Bearer token for authenticated A2A agents.
	AuthToken string

	// PollInterval controls how frequently the session polls for task updates.
	// Default: 2 seconds.
	PollInterval time.Duration

	// Timeout is the maximum duration to wait for a task to complete.
	// Default: 10 minutes.
	Timeout time.Duration
}

// defaultA2APollInterval is the default interval between status polls.
const defaultA2APollInterval = 2 * time.Second

// defaultA2ATimeout is the maximum wait for a single A2A task.
const defaultA2ATimeout = 10 * time.Minute

// launchA2A starts a session backed by a remote A2A agent. Instead of spawning
// a CLI subprocess, it creates an a2a.Client, submits a task with the prompt,
// and runs a polling loop to collect results. The returned Session has the same
// lifecycle semantics as CLI sessions (StatusLaunching -> StatusRunning ->
// StatusCompleted/StatusErrored) and produces StreamEvents on OutputCh.
func launchA2A(ctx context.Context, opts LaunchOptions, cfg A2ASessionConfig, bus ...*events.Bus) (*Session, error) {
	if cfg.AgentURL == "" {
		return nil, fmt.Errorf("a2a: agent URL is required")
	}
	if opts.Prompt == "" {
		return nil, fmt.Errorf("a2a: prompt is required")
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = defaultA2APollInterval
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultA2ATimeout
	}

	// Build the A2A client.
	clientOpts := []a2a.ClientOption{}
	if cfg.AuthToken != "" {
		clientOpts = append(clientOpts, a2a.WithAuthToken(cfg.AuthToken))
	}
	client := a2a.NewClient(cfg.AgentURL, clientOpts...)

	// Discover agent capabilities (fail fast on unreachable agents).
	card, err := client.GetAgentCard(ctx)
	if err != nil {
		return nil, fmt.Errorf("a2a: failed to discover agent at %s: %w", cfg.AgentURL, err)
	}

	var sessionBus *events.Bus
	if len(bus) > 0 {
		sessionBus = bus[0]
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	now := time.Now()
	s := &Session{
		ID:           uuid.New().String(),
		Provider:     ProviderA2A,
		RepoPath:     opts.RepoPath,
		RepoName:     repoNameFromPath(opts.RepoPath),
		Status:       StatusLaunching,
		Prompt:       opts.Prompt,
		Model:        fmt.Sprintf("a2a:%s", card.Name),
		AgentName:    card.Name,
		LaunchedAt:   now,
		LastActivity: now,
		cancel:       cancel,
		doneCh:       make(chan struct{}),
		OutputCh:     make(chan string, 100),
		bus:          sessionBus,
	}

	// Submit the task.
	taskID := fmt.Sprintf("ralph-%s", uuid.New().String()[:8])
	task, err := client.SendTask(sessionCtx, a2a.TaskSendParams{
		ID: taskID,
		Messages: []a2a.Message{
			{Role: "user", Parts: []a2a.Part{a2a.TextPart(opts.Prompt)}},
		},
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("a2a: failed to send task to %s: %w", card.Name, err)
	}

	s.mu.Lock()
	s.ProviderSessionID = task.ID
	s.Status = StatusRunning
	s.mu.Unlock()

	// Publish session started event.
	if sessionBus != nil {
		sessionBus.Publish(events.Event{
			Type:      events.SessionStarted,
			SessionID: s.ID,
			RepoPath:  s.RepoPath,
			RepoName:  s.RepoName,
			Provider:  string(ProviderA2A),
			Data: map[string]any{
				"agent_name": card.Name,
				"agent_url":  cfg.AgentURL,
				"task_id":    task.ID,
			},
		})
	}

	// Background goroutine: poll for task completion.
	go func() {
		defer cancel()
		defer close(s.OutputCh)
		defer close(s.doneCh)
		runA2ASession(sessionCtx, s, client, task.ID, cfg, card.Name)
	}()

	return s, nil
}

// runA2ASession polls the A2A agent for task status updates until the task
// reaches a terminal state or the context is canceled.
func runA2ASession(ctx context.Context, s *Session, client *a2a.Client, taskID string, cfg A2ASessionConfig, agentName string) {
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	deadline := time.After(cfg.Timeout)
	var lastMessageCount int

	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			if s.Status == StatusRunning {
				s.Status = StatusStopped
				s.ExitReason = "stopped by user"
				now := time.Now()
				s.EndedAt = &now
			}
			s.mu.Unlock()
			finalizeA2ASession(s)
			return

		case <-deadline:
			s.mu.Lock()
			s.Status = StatusErrored
			s.Error = fmt.Sprintf("a2a: task %s timed out after %s", taskID, cfg.Timeout)
			s.ExitReason = "timeout"
			now := time.Now()
			s.EndedAt = &now
			s.mu.Unlock()
			finalizeA2ASession(s)
			return

		case <-ticker.C:
			task, err := client.GetTask(ctx, taskID)
			if err != nil {
				slog.Warn("a2a: poll error", "task_id", taskID, "error", err)
				s.mu.Lock()
				s.StreamParseErrors++
				s.mu.Unlock()
				continue
			}

			s.mu.Lock()
			s.LastActivity = time.Now()

			// Process new messages since last poll.
			for i := lastMessageCount; i < len(task.Messages); i++ {
				msg := task.Messages[i]
				for _, part := range msg.Parts {
					if part.Type == "text" && part.Text != "" {
						event := a2aMessageToStreamEvent(msg.Role, part.Text, task)
						emitA2AEvent(s, event)
					}
				}
			}
			lastMessageCount = len(task.Messages)

			// Check terminal states.
			switch task.State {
			case a2a.TaskCompleted:
				s.Status = StatusCompleted
				s.ExitReason = "completed normally"
				now := time.Now()
				s.EndedAt = &now

				// Extract final result from the last agent message.
				result := extractA2AResult(task)
				if result != "" {
					s.LastOutput = truncateStr(result, 4000)
				}

				s.mu.Unlock()
				finalizeA2ASession(s)
				return

			case a2a.TaskFailed:
				s.Status = StatusErrored
				s.Error = extractA2AError(task)
				s.ExitReason = "task failed"
				now := time.Now()
				s.EndedAt = &now
				s.mu.Unlock()
				finalizeA2ASession(s)
				return

			case a2a.TaskCanceled:
				s.Status = StatusStopped
				s.ExitReason = "canceled by remote agent"
				now := time.Now()
				s.EndedAt = &now
				s.mu.Unlock()
				finalizeA2ASession(s)
				return

			case a2a.TaskWorking, a2a.TaskSubmitted, a2a.TaskInputNeeded:
				s.TurnCount = len(task.Messages)
				s.mu.Unlock()
			default:
				s.mu.Unlock()
			}
		}
	}
}

// a2aMessageToStreamEvent translates an A2A message into a unified StreamEvent.
func a2aMessageToStreamEvent(role string, text string, task *a2a.Task) StreamEvent {
	event := StreamEvent{
		SessionID: task.ID,
		Content:   text,
		Text:      text,
	}

	switch role {
	case "agent":
		event.Type = "assistant"
	case "user":
		event.Type = "system"
	default:
		event.Type = "assistant"
	}

	return event
}

// normalizeA2AEvent parses an A2A JSON event into a StreamEvent.
// A2A tasks produce status events with state, messages, and artifacts.
// This normalizer handles both task-level status updates and individual
// message parts.
func normalizeA2AEvent(line []byte) (StreamEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return fallbackTextEvent(ProviderA2A, line)
	}

	event := StreamEvent{
		Raw: json.RawMessage(append([]byte(nil), line...)),
	}

	// A2A events may come from task status updates or streamed events.
	event.Type = firstNonEmptyString(raw, "type", "event", "state")
	event.SessionID = firstNonEmptyString(raw, "id", "task_id", "session_id")
	event.Content = firstText(raw, "content", "text", "message", "result")
	event.Result = firstText(raw, "result", "summary")
	event.Error = firstText(raw, "error", "error.message")

	// Map A2A task states to event types.
	if state := firstNonEmptyString(raw, "state"); state != "" {
		switch a2a.TaskState(state) {
		case a2a.TaskCompleted:
			event.Type = "result"
		case a2a.TaskFailed:
			event.Type = "result"
			event.IsError = true
		case a2a.TaskWorking:
			event.Type = "assistant"
		case a2a.TaskSubmitted:
			event.Type = "system"
		}
	}

	// Cost: A2A agents may report cost in metadata.
	event.CostUSD = firstNonZeroFloat(raw, "cost_usd", "metadata.cost_usd", "usage.cost_usd")
	if event.CostUSD > 0 {
		event.CostSource = "structured"
	}
	if event.CostUSD == 0 {
		event.CostUSD = estimateCostFromTokens(ProviderA2A, raw)
		if event.CostUSD > 0 {
			event.CostSource = "estimated"
		}
	}

	event.NumTurns = firstNonZeroInt(raw, "num_turns", "turns", "metadata.turns")
	event.Duration = firstNonZeroFloat(raw, "duration_seconds", "duration", "metadata.duration_seconds")
	event.IsError = event.IsError || firstTrueBool(raw, "is_error", "error")
	event.Text = firstNonEmpty(event.Content, event.Result, event.Error)

	applyEventDefaults(&event)
	return event, nil
}

// emitA2AEvent sends an event through the session's output channel.
func emitA2AEvent(s *Session, event StreamEvent) {
	s.LastEventType = event.Type
	text := firstNonEmpty(event.Content, event.Text, event.Result)
	if text != "" {
		s.LastOutput = truncateStr(text, 4000)
		s.TotalOutputCount++
		appendOutputHistory(s, text)

		select {
		case s.OutputCh <- text:
		default:
			// Channel full; drop oldest output. Non-blocking to avoid deadlocks.
		}
	}
}

// extractA2AResult extracts the final text result from a completed A2A task.
func extractA2AResult(task *a2a.Task) string {
	// Check artifacts first (the canonical output location).
	for _, artifact := range task.Artifacts {
		for _, part := range artifact.Parts {
			if part.Type == "text" && part.Text != "" {
				return part.Text
			}
		}
	}

	// Fall back to the last agent message.
	for i := len(task.Messages) - 1; i >= 0; i-- {
		if task.Messages[i].Role == "agent" {
			for _, part := range task.Messages[i].Parts {
				if part.Type == "text" && part.Text != "" {
					return part.Text
				}
			}
		}
	}

	return ""
}

// extractA2AError extracts an error description from a failed A2A task.
func extractA2AError(task *a2a.Task) string {
	// Look for the last agent message which should contain the error.
	for i := len(task.Messages) - 1; i >= 0; i-- {
		if task.Messages[i].Role == "agent" {
			var parts []string
			for _, part := range task.Messages[i].Parts {
				if part.Type == "text" && part.Text != "" {
					parts = append(parts, part.Text)
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, "; ")
			}
		}
	}
	return fmt.Sprintf("a2a task %s failed", task.ID)
}

// finalizeA2ASession performs cleanup and event publishing after an A2A session ends.
func finalizeA2ASession(s *Session) {
	if s.bus != nil {
		s.mu.Lock()
		data := map[string]any{
			"status":      string(s.Status),
			"exit_reason": s.ExitReason,
			"spent_usd":   s.SpentUSD,
			"turns":       s.TurnCount,
			"cost_source": s.CostSource,
		}
		s.mu.Unlock()

		s.bus.Publish(events.Event{
			Type:      events.SessionEnded,
			SessionID: s.ID,
			RepoPath:  s.RepoPath,
			RepoName:  s.RepoName,
			Provider:  string(ProviderA2A),
			Data:      data,
		})
	}

	// Persist final state if callback is registered.
	s.mu.Lock()
	onComplete := s.onComplete
	s.mu.Unlock()
	if onComplete != nil {
		onComplete(s)
	}
}

// appendOutputHistory appends a line to the session's rolling output history.
// Must be called with s.mu held.
func appendOutputHistory(s *Session, text string) {
	const maxHistory = 50
	s.OutputHistory = append(s.OutputHistory, text)
	if len(s.OutputHistory) > maxHistory {
		s.OutputHistory = s.OutputHistory[len(s.OutputHistory)-maxHistory:]
	}
}

// repoNameFromPath extracts the last path component as the repo name.
// Returns empty string for empty paths.
func repoNameFromPath(path string) string {
	if path == "" {
		return ""
	}
	// Trim trailing slashes.
	for len(path) > 1 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

// A2AAgentRegistry maintains a cache of discovered A2A agents for session
// routing. Thread-safe for concurrent access.
type A2AAgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*a2a.AgentCard // keyed by URL
}

// NewA2AAgentRegistry creates an empty agent registry.
func NewA2AAgentRegistry() *A2AAgentRegistry {
	return &A2AAgentRegistry{
		agents: make(map[string]*a2a.AgentCard),
	}
}

// Discover fetches and caches an agent card from the given URL.
func (r *A2AAgentRegistry) Discover(ctx context.Context, url string) (*a2a.AgentCard, error) {
	r.mu.RLock()
	if card, ok := r.agents[url]; ok {
		r.mu.RUnlock()
		return card, nil
	}
	r.mu.RUnlock()

	client := a2a.NewClient(url)
	card, err := client.GetAgentCard(ctx)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.agents[url] = card
	r.mu.Unlock()

	return card, nil
}

// List returns all cached agent cards.
func (r *A2AAgentRegistry) List() []*a2a.AgentCard {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cards := make([]*a2a.AgentCard, 0, len(r.agents))
	for _, card := range r.agents {
		cards = append(cards, card)
	}
	return cards
}

// Remove removes a cached agent by URL.
func (r *A2AAgentRegistry) Remove(url string) {
	r.mu.Lock()
	delete(r.agents, url)
	r.mu.Unlock()
}
