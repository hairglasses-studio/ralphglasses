// Package trigger provides external trigger mechanisms for launching
// ralphglasses agent sessions via HTTP webhooks and cron schedules.
package trigger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TriggerRequest is the JSON body accepted by POST /api/trigger.
type TriggerRequest struct {
	Source  string         `json:"source"`            // e.g. "github", "cron", "slack"
	Event   string         `json:"event"`             // e.g. "push", "schedule", "command"
	Payload map[string]any `json:"payload,omitempty"` // arbitrary event-specific data
}

// TriggerResponse is returned by the trigger endpoint.
type TriggerResponse struct {
	RunID   string `json:"run_id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// ResumeRequest is the JSON body accepted by POST /api/resume/:run_id.
type ResumeRequest struct {
	Payload map[string]any `json:"payload,omitempty"`
}

// ResumeResponse is returned by the resume endpoint.
type ResumeResponse struct {
	RunID   string `json:"run_id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// SessionLauncher is called when a trigger fires. Implementations should
// start an agent session and return the run ID.
type SessionLauncher func(ctx context.Context, req TriggerRequest) (runID string, err error)

// SessionResumer is called when a resume request arrives. Implementations
// should resume the paused session identified by runID.
type SessionResumer func(ctx context.Context, runID string, payload map[string]any) error

// Server exposes HTTP endpoints for triggering and resuming agent sessions.
type Server struct {
	mu       sync.RWMutex
	addr     string
	launcher SessionLauncher
	resumer  SessionResumer
	server   *http.Server
	runs     map[string]time.Time // run_id -> created_at (bounded in-memory tracking)
}

// NewServer creates a trigger HTTP server bound to the given address.
// launcher is called for each trigger; resumer is called for resume requests.
// Either may be nil (the corresponding endpoint returns 501).
func NewServer(addr string, launcher SessionLauncher, resumer SessionResumer) *Server {
	s := &Server{
		addr:     addr,
		launcher: launcher,
		resumer:  resumer,
		runs:     make(map[string]time.Time),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/trigger", s.handleTrigger)
	mux.HandleFunc("POST /api/resume/", s.handleResume)
	mux.HandleFunc("GET /api/health", s.handleHealth)

	s.server = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return s
}

// Start begins listening for trigger requests. It blocks until the context
// is cancelled or an error occurs.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("trigger server listen: %w", err)
	}

	slog.Info("trigger server started", "addr", ln.Addr().String())

	errCh := make(chan error, 1)
	go func() {
		if err := s.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

// Addr returns the configured listen address.
func (s *Server) Addr() string {
	return s.addr
}

// Runs returns a snapshot of tracked run IDs and their creation times.
func (s *Server) Runs() map[string]time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]time.Time, len(s.runs))
	for k, v := range s.runs {
		out[k] = v
	}
	return out
}

func (s *Server) handleTrigger(w http.ResponseWriter, r *http.Request) {
	if s.launcher == nil {
		writeJSON(w, http.StatusNotImplemented, TriggerResponse{
			Status:  "error",
			Message: "no session launcher configured",
		})
		return
	}

	var req TriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, TriggerResponse{
			Status:  "error",
			Message: fmt.Sprintf("invalid JSON: %v", err),
		})
		return
	}

	if req.Source == "" {
		writeJSON(w, http.StatusBadRequest, TriggerResponse{
			Status:  "error",
			Message: "source is required",
		})
		return
	}
	if req.Event == "" {
		writeJSON(w, http.StatusBadRequest, TriggerResponse{
			Status:  "error",
			Message: "event is required",
		})
		return
	}

	runID, err := s.launcher(r.Context(), req)
	if err != nil {
		slog.Error("trigger launch failed", "source", req.Source, "event", req.Event, "error", err)
		writeJSON(w, http.StatusInternalServerError, TriggerResponse{
			Status:  "error",
			Message: fmt.Sprintf("launch failed: %v", err),
		})
		return
	}

	if runID == "" {
		runID = uuid.NewString()
	}

	s.mu.Lock()
	s.runs[runID] = time.Now()
	// Bound the tracking map to prevent unbounded growth.
	if len(s.runs) > 10_000 {
		s.pruneOldRuns()
	}
	s.mu.Unlock()

	slog.Info("trigger fired", "source", req.Source, "event", req.Event, "run_id", runID)

	writeJSON(w, http.StatusOK, TriggerResponse{
		RunID:   runID,
		Status:  "launched",
		Message: fmt.Sprintf("session launched from %s/%s", req.Source, req.Event),
	})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	if s.resumer == nil {
		writeJSON(w, http.StatusNotImplemented, ResumeResponse{
			Status:  "error",
			Message: "no session resumer configured",
		})
		return
	}

	// Extract run_id from path: /api/resume/{run_id}
	path := strings.TrimPrefix(r.URL.Path, "/api/resume/")
	runID := strings.TrimRight(path, "/")
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, ResumeResponse{
			Status:  "error",
			Message: "run_id is required in path",
		})
		return
	}

	var req ResumeRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ResumeResponse{
				RunID:   runID,
				Status:  "error",
				Message: fmt.Sprintf("invalid JSON: %v", err),
			})
			return
		}
	}

	if err := s.resumer(r.Context(), runID, req.Payload); err != nil {
		slog.Error("resume failed", "run_id", runID, "error", err)
		writeJSON(w, http.StatusInternalServerError, ResumeResponse{
			RunID:   runID,
			Status:  "error",
			Message: fmt.Sprintf("resume failed: %v", err),
		})
		return
	}

	slog.Info("session resumed via webhook", "run_id", runID)
	writeJSON(w, http.StatusOK, ResumeResponse{
		RunID:   runID,
		Status:  "resumed",
		Message: "session resumed",
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	count := len(s.runs)
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"runs":      count,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// pruneOldRuns removes the oldest half of tracked runs. Must be called with s.mu held.
func (s *Server) pruneOldRuns() {
	target := len(s.runs) / 2
	removed := 0
	for k := range s.runs {
		if removed >= target {
			break
		}
		delete(s.runs, k)
		removed++
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
