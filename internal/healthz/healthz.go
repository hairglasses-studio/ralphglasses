package healthz

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// Server is a lightweight HTTP health check server.
type Server struct {
	addr     string
	ready    atomic.Bool
	srv      *http.Server
	started  time.Time
}

// New creates a health server on the given address (e.g. ":9090").
func New(addr string) *Server {
	s := &Server{addr: addr, started: time.Now()}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/metrics", s.handleMetrics)
	s.srv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

// SetReady marks the server as ready to serve traffic.
func (s *Server) SetReady() { s.ready.Store(true) }

// Start begins listening. It blocks until the server stops.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("healthz listen: %w", err)
	}
	return s.srv.Serve(ln)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"uptime": time.Since(s.started).String(),
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "# HELP ralphglasses_up Whether the process is running.\n")
	fmt.Fprintf(w, "# TYPE ralphglasses_up gauge\n")
	fmt.Fprintf(w, "ralphglasses_up 1\n")
	fmt.Fprintf(w, "# HELP ralphglasses_uptime_seconds Time since process start.\n")
	fmt.Fprintf(w, "# TYPE ralphglasses_uptime_seconds gauge\n")
	fmt.Fprintf(w, "ralphglasses_uptime_seconds %.0f\n", time.Since(s.started).Seconds())
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.ready.Load() {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not_ready"})
	}
}
