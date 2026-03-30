package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Gateway is an MCP gateway proxy that routes JSON-RPC requests to backend
// MCP servers based on tool namespace, with pluggable auth, rate limiting,
// and audit logging middleware.
type Gateway struct {
	mu       sync.RWMutex
	mux      *http.ServeMux
	backends map[string]*url.URL // namespace -> backend URL
	handler  http.Handler        // final composed handler chain

	Auth    *Auth
	Audit   *AuditLogger
	Limiter *RateLimiter

	readHeaderTimeout time.Duration
}

// GatewayOption configures a Gateway.
type GatewayOption func(*Gateway)

// WithAuth sets the authentication middleware.
func WithAuth(a *Auth) GatewayOption {
	return func(g *Gateway) { g.Auth = a }
}

// WithAudit sets the audit logger.
func WithAudit(al *AuditLogger) GatewayOption {
	return func(g *Gateway) { g.Audit = al }
}

// WithRateLimiter sets the rate limiter.
func WithRateLimiter(rl *RateLimiter) GatewayOption {
	return func(g *Gateway) { g.Limiter = rl }
}

// WithReadHeaderTimeout sets the HTTP server's read header timeout.
func WithReadHeaderTimeout(d time.Duration) GatewayOption {
	return func(g *Gateway) { g.readHeaderTimeout = d }
}

// NewGateway creates a gateway with the given options.
func NewGateway(opts ...GatewayOption) *Gateway {
	g := &Gateway{
		mux:               http.NewServeMux(),
		backends:          make(map[string]*url.URL),
		readHeaderTimeout: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(g)
	}
	g.buildHandler()
	return g
}

// Handle registers an HTTP handler for the given pattern on the gateway's
// internal mux.
func (g *Gateway) Handle(pattern string, handler http.Handler) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.mux.Handle(pattern, handler)
	g.buildHandler()
}

// RegisterBackend maps a tool namespace (e.g. "session", "fleet") to a
// backend MCP server URL. JSON-RPC calls whose method contains the namespace
// prefix are forwarded to this backend.
func (g *Gateway) RegisterBackend(namespace string, backendURL *url.URL) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.backends[namespace] = backendURL
}

// ServeHTTP implements http.Handler by dispatching through the middleware chain.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.RLock()
	h := g.handler
	g.mu.RUnlock()
	h.ServeHTTP(w, r)
}

// ListenAndServe starts the gateway on the given address.
func (g *Gateway) ListenAndServe(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("gateway listen: %w", err)
	}
	srv := &http.Server{
		Handler:           g,
		ReadHeaderTimeout: g.readHeaderTimeout,
	}
	return srv.Serve(ln)
}

// ListenAndServeContext starts the gateway and shuts it down when ctx is done.
func (g *Gateway) ListenAndServeContext(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("gateway listen: %w", err)
	}
	srv := &http.Server{
		Handler:           g,
		ReadHeaderTimeout: g.readHeaderTimeout,
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()
	err = srv.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// buildHandler composes the middleware chain around the core mux + JSON-RPC
// router. Must be called with g.mu held.
func (g *Gateway) buildHandler() {
	var h http.Handler = http.HandlerFunc(g.routeRequest)

	// Rate limiter (innermost after routing).
	if g.Limiter != nil {
		rl := g.Limiter
		next := h
		h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				key = r.RemoteAddr
			}
			if !rl.Allow(key) {
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	// Auth middleware.
	if g.Auth != nil {
		h = g.Auth.Wrap(h)
	}

	// Audit logging (outermost).
	if g.Audit != nil {
		h = g.Audit.Wrap(h)
	}

	g.handler = h
}

// jsonRPCRequest is the minimal structure we inspect to route requests.
type jsonRPCRequest struct {
	Method string `json:"method"`
}

// routeRequest handles an incoming request. If it is a JSON-RPC call with a
// recognized namespace, it is forwarded to the corresponding backend.
// Otherwise it falls through to the local mux.
func (g *Gateway) routeRequest(w http.ResponseWriter, r *http.Request) {
	// Only attempt JSON-RPC routing on POST with JSON content.
	if r.Method == http.MethodPost && isJSON(r) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
		if err != nil {
			http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		var rpc jsonRPCRequest
		if json.Unmarshal(body, &rpc) == nil && rpc.Method != "" {
			if backend := g.findBackend(rpc.Method); backend != nil {
				proxy := &httputil.ReverseProxy{
					Director: func(req *http.Request) {
						req.URL.Scheme = backend.Scheme
						req.URL.Host = backend.Host
						req.URL.Path = backend.Path
						req.Host = backend.Host
						req.Body = io.NopCloser(bytes.NewReader(body))
						req.ContentLength = int64(len(body))
					},
				}
				proxy.ServeHTTP(w, r)
				return
			}
		}

		// Restore body for the mux handler.
		r.Body = io.NopCloser(bytes.NewReader(body))
	}

	g.mux.ServeHTTP(w, r)
}

// findBackend looks up the backend for a JSON-RPC method by matching the
// namespace prefix. Method format: "ralphglasses_<namespace>_<tool>" or
// "<namespace>.<tool>".
func (g *Gateway) findBackend(method string) *url.URL {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Try "ralphglasses_<namespace>_..." format.
	if strings.HasPrefix(method, "ralphglasses_") {
		rest := strings.TrimPrefix(method, "ralphglasses_")
		if idx := strings.Index(rest, "_"); idx > 0 {
			ns := rest[:idx]
			if u, ok := g.backends[ns]; ok {
				return u
			}
		}
	}

	// Try "<namespace>.<tool>" or "<namespace>/<tool>" format.
	for _, sep := range []string{".", "/"} {
		if idx := strings.Index(method, sep); idx > 0 {
			ns := method[:idx]
			if u, ok := g.backends[ns]; ok {
				return u
			}
		}
	}

	return nil
}

func isJSON(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return strings.Contains(ct, "application/json")
}
