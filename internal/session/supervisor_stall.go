package session

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// SupervisorStallHandler detects and handles stalled sessions during supervisor ticks.
type SupervisorStallHandler struct {
	Threshold  time.Duration  // default DefaultStallThreshold (5min)
	MaxRetries int            // default 2
	retryCount map[string]int // sessionID -> retry count
	mu         sync.Mutex
}

// NewSupervisorStallHandler creates a stall handler with sensible defaults.
func NewSupervisorStallHandler() *SupervisorStallHandler {
	return &SupervisorStallHandler{
		Threshold:  DefaultStallThreshold,
		MaxRetries: 2,
		retryCount: make(map[string]int),
	}
}

// CheckAndHandle detects stalled sessions via mgr.DetectStalls, kills them,
// records observations, publishes events, and optionally retries.
// Returns the list of session IDs that were killed.
func (h *SupervisorStallHandler) CheckAndHandle(ctx context.Context, mgr *Manager, bus *events.Bus, repoPath string) []string {
	stalled := mgr.DetectStalls(h.Threshold)
	if len(stalled) == 0 {
		return nil
	}

	var killed []string
	for _, id := range stalled {
		// Get session info before stopping (for retry).
		sess, ok := mgr.Get(id)
		if !ok {
			continue
		}

		sess.mu.Lock()
		prompt := sess.Prompt
		provider := sess.Provider
		model := sess.Model
		sessRepoPath := sess.RepoPath
		sess.mu.Unlock()

		// Stop the stalled session.
		if err := mgr.Stop(id); err != nil {
			slog.Warn("supervisor_stall: failed to stop session", "id", id, "error", err)
			continue
		}
		killed = append(killed, id)

		slog.Info("supervisor_stall: killed stalled session", "id", id, "threshold", h.Threshold)

		// Publish stall event.
		if bus != nil {
			bus.Publish(events.Event{
				Type:      events.SessionError,
				SessionID: id,
				RepoPath:  repoPath,
				Data: map[string]any{
					"reason":    "stalled",
					"threshold": h.Threshold.String(),
				},
			})
		}

		// Retry if under limit.
		h.mu.Lock()
		count := h.retryCount[id]
		canRetry := count < h.MaxRetries && prompt != ""
		if canRetry {
			h.retryCount[id] = count + 1
		}
		h.mu.Unlock()

		if canRetry {
			slog.Info("supervisor_stall: retrying stalled session",
				"id", id, "retry", count+1, "max", h.MaxRetries)
			go func(p string, prov Provider, m string, rp string) {
				_, err := mgr.Launch(ctx, LaunchOptions{
					Provider: prov,
					RepoPath: rp,
					Prompt:   p,
					Model:    m,
				})
				if err != nil {
					slog.Warn("supervisor_stall: retry launch failed", "error", err)
				}
			}(prompt, provider, model, sessRepoPath)
		} else if count >= h.MaxRetries {
			slog.Warn("supervisor_stall: max retries reached, giving up",
				"id", id, "retries", count)
		}
	}

	return killed
}

// RetryCount returns the current retry count for a session ID.
func (h *SupervisorStallHandler) RetryCount(id string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.retryCount[id]
}
