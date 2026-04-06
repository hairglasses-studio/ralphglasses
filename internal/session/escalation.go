package session

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// EscalationSeverity classifies the urgency of an escalation.
type EscalationSeverity string

const (
	EscalationInfo     EscalationSeverity = "info"
	EscalationWarning  EscalationSeverity = "warning"
	EscalationCritical EscalationSeverity = "critical"
)

// EscalationRequest describes a structured request for human intervention.
// It follows 12-Factor Agent principle #7: contact humans with tool calls.
type EscalationRequest struct {
	SessionID     string        `json:"session_id"`
	Severity      EscalationSeverity `json:"severity"`
	Question      string        `json:"question"`
	Context       string        `json:"context,omitempty"`
	Options       []string      `json:"options,omitempty"` // e.g. ["continue", "stop", "retry"]
	Timeout       time.Duration `json:"timeout"`
	DefaultAction string        `json:"default_action"` // applied if timeout with no response
}

// EscalationResponse holds the human's decision.
type EscalationResponse struct {
	Action    string    `json:"action"`
	Reason    string    `json:"reason,omitempty"`
	RespondAt time.Time `json:"responded_at"`
	TimedOut  bool      `json:"timed_out"`
}

// EscalationHandler processes escalation requests and delivers them
// through configured notification channels.
type EscalationHandler struct {
	mu       sync.Mutex
	pending  map[string]chan EscalationResponse // sessionID -> response channel
	notifier EscalationNotifier
}

// EscalationNotifier is implemented by notification backends (desktop, Slack, webhook).
type EscalationNotifier interface {
	Notify(req EscalationRequest) error
}

// noopNotifier is used when no notifier is configured.
type noopNotifier struct{}

func (n noopNotifier) Notify(_ EscalationRequest) error { return nil }

// NewEscalationHandler creates a handler with the given notifier.
// If notifier is nil, notifications are silently dropped.
func NewEscalationHandler(notifier EscalationNotifier) *EscalationHandler {
	if notifier == nil {
		notifier = noopNotifier{}
	}
	return &EscalationHandler{
		pending:  make(map[string]chan EscalationResponse),
		notifier: notifier,
	}
}

// Escalate sends a request for human input and blocks until a response
// is received or the timeout expires. If the timeout expires, the
// DefaultAction from the request is used.
func (h *EscalationHandler) Escalate(ctx context.Context, req EscalationRequest) (*EscalationResponse, error) {
	if req.Question == "" {
		return nil, fmt.Errorf("escalation requires a question")
	}
	if req.Timeout <= 0 {
		req.Timeout = 5 * time.Minute // default timeout
	}
	if req.DefaultAction == "" {
		req.DefaultAction = "continue"
	}

	// Create response channel.
	ch := make(chan EscalationResponse, 1)
	h.mu.Lock()
	h.pending[req.SessionID] = ch
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.pending, req.SessionID)
		h.mu.Unlock()
	}()

	// Send notification.
	if err := h.notifier.Notify(req); err != nil {
		slog.Error("escalation: notify failed", "session", req.SessionID, "err", err)
	}

	// Wait for response or timeout.
	timer := time.NewTimer(req.Timeout)
	defer timer.Stop()

	select {
	case resp := <-ch:
		return &resp, nil
	case <-timer.C:
		return &EscalationResponse{
			Action:    req.DefaultAction,
			Reason:    "timeout",
			RespondAt: time.Now(),
			TimedOut:  true,
		}, nil
	case <-ctx.Done():
		return &EscalationResponse{
			Action:    req.DefaultAction,
			Reason:    "context cancelled",
			RespondAt: time.Now(),
			TimedOut:  true,
		}, ctx.Err()
	}
}

// Respond delivers a human response for a pending escalation.
// Returns false if no pending escalation exists for the session.
func (h *EscalationHandler) Respond(sessionID string, action string, reason string) bool {
	h.mu.Lock()
	ch, ok := h.pending[sessionID]
	h.mu.Unlock()

	if !ok {
		return false
	}

	select {
	case ch <- EscalationResponse{
		Action:    action,
		Reason:    reason,
		RespondAt: time.Now(),
	}:
		return true
	default:
		return false // channel full, already responded
	}
}

// PendingCount returns the number of escalations awaiting human response.
func (h *EscalationHandler) PendingCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.pending)
}

// ShouldEscalate determines whether an action should be escalated based
// on the current autonomy level and the intent's characteristics.
func ShouldEscalate(autonomyLevel int, intent *Intent, confidence float64) bool {
	switch autonomyLevel {
	case 0, 1:
		// L0-L1: escalate all destructive actions.
		return intent.Destructive
	case 2:
		// L2: escalate low-confidence decisions and destructive actions.
		return intent.Destructive || confidence < 0.7
	case 3:
		// L3: only escalate safety violations (handled elsewhere).
		return false
	default:
		return true // unknown level = escalate everything
	}
}
