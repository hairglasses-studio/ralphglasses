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

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// TriggerRequest is the JSON body accepted by POST /api/trigger.
type TriggerRequest struct {
	TenantID string         `json:"tenant_id,omitempty"`
	Source   string         `json:"source"`            // e.g. "github", "cron", "slack"
	Event    string         `json:"event"`             // e.g. "push", "schedule", "command"
	Payload  map[string]any `json:"payload,omitempty"` // arbitrary event-specific data
}

// TriggerResponse is returned by the trigger endpoint.
type TriggerResponse struct {
	RunID    string `json:"run_id"`
	TenantID string `json:"tenant_id,omitempty"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
}

// ResumeRequest is the JSON body accepted by POST /api/resume/:run_id.
type ResumeRequest struct {
	Payload map[string]any `json:"payload,omitempty"`
}

// ResumeResponse is returned by the resume endpoint.
type ResumeResponse struct {
	RunID    string `json:"run_id"`
	TenantID string `json:"tenant_id,omitempty"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
}

// SessionLauncher is called when a trigger fires. Implementations should
// start an agent session and return the run ID.
type SessionLauncher func(ctx context.Context, req TriggerRequest) (runID string, err error)

// SessionResumer is called when a resume request arrives. Implementations
// should resume the paused session identified by runID.
type SessionResumer func(ctx context.Context, tenantID, runID string, payload map[string]any) error

// TenantAuthorizer resolves a bearer token to exactly one tenant.
type TenantAuthorizer func(ctx context.Context, bearerToken string) (tenantID string, err error)

type trackedRun struct {
	TenantID  string
	CreatedAt time.Time
}

// Server exposes HTTP endpoints for triggering and resuming agent sessions.
type Server struct {
	mu         sync.RWMutex
	addr       string
	launcher   SessionLauncher
	resumer    SessionResumer
	authorizer TenantAuthorizer
	server     *http.Server
	runs       map[string]trackedRun // run_id -> run metadata (bounded in-memory tracking)
}

// NewServer creates a trigger HTTP server bound to the given address.
// launcher is called for each trigger; resumer is called for resume requests.
// Either may be nil (the corresponding endpoint returns 501).
func NewServer(addr string, launcher SessionLauncher, resumer SessionResumer, authorizer TenantAuthorizer) *Server {
	s := &Server{
		addr:       addr,
		launcher:   launcher,
		resumer:    resumer,
		authorizer: authorizer,
		runs:       make(map[string]trackedRun),
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
		out[k] = v.CreatedAt
	}
	return out
}

func (s *Server) handleTrigger(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := s.authorizeTenant(w, r)
	if !ok {
		return
	}
	if s.launcher == nil {
		writeJSON(w, http.StatusNotImplemented, TriggerResponse{
			TenantID: tenantID,
			Status:   "error",
			Message:  "no session launcher configured",
		})
		return
	}

	var req TriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, TriggerResponse{
			TenantID: tenantID,
			Status:   "error",
			Message:  fmt.Sprintf("invalid JSON: %v", err),
		})
		return
	}

	if req.TenantID != "" && session.NormalizeTenantID(req.TenantID) != tenantID {
		writeJSON(w, http.StatusForbidden, TriggerResponse{
			TenantID: tenantID,
			Status:   "error",
			Message:  "request tenant_id does not match bearer token tenant",
		})
		return
	}
	req.TenantID = tenantID

	if req.Source == "" {
		writeJSON(w, http.StatusBadRequest, TriggerResponse{
			TenantID: tenantID,
			Status:   "error",
			Message:  "source is required",
		})
		return
	}
	if req.Event == "" {
		writeJSON(w, http.StatusBadRequest, TriggerResponse{
			TenantID: tenantID,
			Status:   "error",
			Message:  "event is required",
		})
		return
	}

	runID, err := s.launcher(r.Context(), req)
	if err != nil {
		slog.Error("trigger launch failed", "tenant_id", tenantID, "source", req.Source, "event", req.Event, "error", err)
		writeJSON(w, http.StatusInternalServerError, TriggerResponse{
			TenantID: tenantID,
			Status:   "error",
			Message:  fmt.Sprintf("launch failed: %v", err),
		})
		return
	}

	if runID == "" {
		runID = uuid.NewString()
	}

	s.mu.Lock()
	s.runs[runID] = trackedRun{TenantID: tenantID, CreatedAt: time.Now()}
	// Bound the tracking map to prevent unbounded growth.
	if len(s.runs) > 10_000 {
		s.pruneOldRuns()
	}
	s.mu.Unlock()

	slog.Info("trigger fired", "tenant_id", tenantID, "source", req.Source, "event", req.Event, "run_id", runID)

	writeJSON(w, http.StatusOK, TriggerResponse{
		RunID:    runID,
		TenantID: tenantID,
		Status:   "launched",
		Message:  fmt.Sprintf("session launched from %s/%s", req.Source, req.Event),
	})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := s.authorizeTenant(w, r)
	if !ok {
		return
	}
	if s.resumer == nil {
		writeJSON(w, http.StatusNotImplemented, ResumeResponse{
			TenantID: tenantID,
			Status:   "error",
			Message:  "no session resumer configured",
		})
		return
	}

	// Extract run_id from path: /api/resume/{run_id}
	path := strings.TrimPrefix(r.URL.Path, "/api/resume/")
	runID := strings.TrimRight(path, "/")
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, ResumeResponse{
			TenantID: tenantID,
			Status:   "error",
			Message:  "run_id is required in path",
		})
		return
	}

	var req ResumeRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ResumeResponse{
				RunID:    runID,
				TenantID: tenantID,
				Status:   "error",
				Message:  fmt.Sprintf("invalid JSON: %v", err),
			})
			return
		}
	}

	s.mu.RLock()
	runMeta, tracked := s.runs[runID]
	s.mu.RUnlock()
	if !tracked {
		writeJSON(w, http.StatusNotFound, ResumeResponse{
			RunID:    runID,
			TenantID: tenantID,
			Status:   "error",
			Message:  "run_id is unknown or no longer tracked",
		})
		return
	}
	if runMeta.TenantID != tenantID {
		writeJSON(w, http.StatusForbidden, ResumeResponse{
			RunID:    runID,
			TenantID: tenantID,
			Status:   "error",
			Message:  "run_id belongs to a different tenant",
		})
		return
	}

	if err := s.resumer(r.Context(), tenantID, runID, req.Payload); err != nil {
		slog.Error("resume failed", "tenant_id", tenantID, "run_id", runID, "error", err)
		writeJSON(w, http.StatusInternalServerError, ResumeResponse{
			RunID:    runID,
			TenantID: tenantID,
			Status:   "error",
			Message:  fmt.Sprintf("resume failed: %v", err),
		})
		return
	}

	slog.Info("session resumed via webhook", "tenant_id", tenantID, "run_id", runID)
	writeJSON(w, http.StatusOK, ResumeResponse{
		RunID:    runID,
		TenantID: tenantID,
		Status:   "resumed",
		Message:  "session resumed",
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

func (s *Server) authorizeTenant(w http.ResponseWriter, r *http.Request) (string, bool) {
	if s.authorizer == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"status":  "error",
			"message": "bearer authorization is required",
		})
		return "", false
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" || !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"status":  "error",
			"message": "missing bearer token",
		})
		return "", false
	}
	token := strings.TrimSpace(header[len("Bearer "):])
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"status":  "error",
			"message": "missing bearer token",
		})
		return "", false
	}
	tenantID, err := s.authorizer(r.Context(), token)
	if err != nil {
		slog.Error("tenant authorization failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"message": "tenant authorization failed",
		})
		return "", false
	}
	if strings.TrimSpace(tenantID) == "" {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"status":  "error",
			"message": "invalid bearer token",
		})
		return "", false
	}
	tenantID = session.NormalizeTenantID(tenantID)
	return tenantID, true
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
