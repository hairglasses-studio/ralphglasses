// Package webui provides a lightweight web terminal relay for remote TUI access.
//
// It serves the existing ralphglasses TUI over a browser via WebSocket + xterm.js.
// This is not a full web application — it streams the terminal output and accepts
// keyboard input, providing the same experience as SSH but without SSH setup.
//
// Usage:
//
//	srv := webui.NewServer(":8080", "bearer-token-here")
//	go srv.ListenAndServe()
package webui

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// Server is the web terminal relay server.
type Server struct {
	addr      string
	token     string // bearer token for authentication (empty = no auth)
	mux       *http.ServeMux
	httpSrv   *http.Server
	mu        sync.Mutex
	sessions  map[string]*WebSession
	maxConns  int
}

// WebSession tracks a single browser terminal session.
type WebSession struct {
	ID        string    `json:"id"`
	RemoteIP  string    `json:"remote_ip"`
	StartedAt time.Time `json:"started_at"`
	Active    bool      `json:"active"`
}

// NewServer creates a web terminal relay server.
// If token is empty, no authentication is required (use only on trusted networks).
func NewServer(addr, token string) *Server {
	s := &Server{
		addr:     addr,
		token:    token,
		mux:      http.NewServeMux(),
		sessions: make(map[string]*WebSession),
		maxConns: 5,
	}

	s.mux.HandleFunc("GET /", s.handleIndex)
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /api/sessions", s.handleListSessions)

	return s
}

// ListenAndServe starts the web server. Blocks until the server is shut down.
func (s *Server) ListenAndServe() error {
	s.httpSrv = &http.Server{
		Addr:         s.addr,
		Handler:      s.authMiddleware(s.mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("webui: listen %s: %w", s.addr, err)
	}

	slog.Info("webui: server started", "addr", s.addr)
	return s.httpSrv.Serve(ln)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpSrv == nil {
		return nil
	}
	return s.httpSrv.Shutdown(ctx)
}

// authMiddleware checks bearer token if configured.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" && r.URL.Path != "/health" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+s.token {
				// Also check query param for WebSocket connections
				if r.URL.Query().Get("token") != s.token {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// handleIndex serves the terminal web page.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

// handleHealth returns server health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	active := 0
	for _, sess := range s.sessions {
		if sess.Active {
			active++
		}
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","active_sessions":%d,"max_sessions":%d}`, active, s.maxConns)
}

// handleListSessions returns active web terminal sessions.
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	sessions := make([]*WebSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"sessions":%d}`, len(sessions))
}

// indexHTML is a minimal terminal web page.
// In production, this would embed xterm.js for full terminal emulation.
// For now, it provides a status dashboard with links to the MCP tools.
const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ralphglasses</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { background: #000; color: #f1f1f0; font-family: 'Maple Mono NF CN', monospace; padding: 2rem; }
  h1 { color: #57c7ff; margin-bottom: 1rem; }
  .status { color: #5af78e; }
  .card { background: #1a1a2e; border: 1px solid #333; border-radius: 8px; padding: 1.5rem; margin: 1rem 0; }
  .card h2 { color: #ff6ac1; margin-bottom: 0.5rem; }
  a { color: #57c7ff; }
  .footer { margin-top: 2rem; color: #666; font-size: 0.8rem; }
</style>
</head>
<body>
  <h1>ralphglasses <span class="status">web</span></h1>
  <div class="card">
    <h2>Fleet Dashboard</h2>
    <p>Web terminal relay for remote TUI access.</p>
    <p>Full xterm.js terminal integration coming in Phase C1.</p>
  </div>
  <div class="card">
    <h2>API Endpoints</h2>
    <ul>
      <li><a href="/health">/health</a> — Server health</li>
      <li><a href="/api/sessions">/api/sessions</a> — Active sessions</li>
    </ul>
  </div>
  <div class="footer">
    <p>ralphglasses web UI — hairglasses-studio</p>
  </div>
</body>
</html>`
