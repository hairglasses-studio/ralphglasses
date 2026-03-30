package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

// TsnetServer serves the MCP gateway and fleet API over the Tailscale network.
// It wraps a standard net/http server with Tailscale peer authentication and
// identity injection into request context.
type TsnetServer struct {
	mu       sync.RWMutex
	tsClient TailscaleClient

	hostname string
	port     int
	handler  http.Handler
	server   *http.Server

	// discovery tracks peers for routing
	discovery *TsnetDiscovery

	startedAt time.Time
}

// TsnetServerConfig configures a TsnetServer instance.
type TsnetServerConfig struct {
	// TSClient is the Tailscale client for peer auth. If nil, DefaultTailscaleClient() is used.
	TSClient TailscaleClient

	// Hostname is this node's identity on the tailnet.
	Hostname string

	// Port to listen on. Defaults to DefaultPort.
	Port int

	// Handler is the HTTP handler (mux) to serve. Required.
	Handler http.Handler

	// Discovery is an optional TsnetDiscovery for peer tracking.
	Discovery *TsnetDiscovery
}

// peerIdentityKey is the context key for injected Tailscale peer identity.
type peerIdentityKey struct{}

// PeerIdentity holds the authenticated identity of a Tailscale peer
// extracted from the request context.
type PeerIdentity struct {
	Hostname  string `json:"hostname"`
	IP        string `json:"ip"`
	LoginName string `json:"login_name,omitempty"`
	IsTagged  bool   `json:"is_tagged"`
}

// PeerIdentityFromContext extracts the Tailscale peer identity from a request
// context. Returns nil if the request did not come through Tailscale auth.
func PeerIdentityFromContext(ctx context.Context) *PeerIdentity {
	v, _ := ctx.Value(peerIdentityKey{}).(*PeerIdentity)
	return v
}

// NewTsnetServer creates a new server that listens on the Tailscale network
// with peer authentication.
func NewTsnetServer(cfg TsnetServerConfig) *TsnetServer {
	tc := cfg.TSClient
	if tc == nil {
		tc = DefaultTailscaleClient()
	}
	port := cfg.Port
	if port == 0 {
		port = DefaultPort
	}

	s := &TsnetServer{
		tsClient:  tc,
		hostname:  cfg.Hostname,
		port:      port,
		handler:   cfg.Handler,
		discovery: cfg.Discovery,
		startedAt: time.Now(),
	}
	return s
}

// Start begins listening and serving HTTP. Blocks until the server is stopped
// or the context is cancelled. The server binds to all interfaces on the
// configured port; Tailscale peer auth middleware verifies caller identity.
func (s *TsnetServer) Start(ctx context.Context) error {
	if s.handler == nil {
		return fmt.Errorf("tsnet server: no handler configured")
	}

	// Build the middleware chain: identity injection -> auth check -> user handler
	handler := s.identityMiddleware(
		TailscaleAuthMiddleware(s.tsClient, s.handler),
	)

	s.mu.Lock()
	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
	}
	s.mu.Unlock()

	util.Debug.Debugf("tsnet server %s starting on :%d", s.hostname, s.port)

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("tsnet server: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the server.
func (s *TsnetServer) Stop(ctx context.Context) error {
	s.mu.RLock()
	srv := s.server
	s.mu.RUnlock()

	if srv != nil {
		return srv.Shutdown(ctx)
	}
	return nil
}

// Addr returns the listen address after the server has started.
func (s *TsnetServer) Addr() string {
	return fmt.Sprintf(":%d", s.port)
}

// Hostname returns the server's configured Tailscale hostname.
func (s *TsnetServer) Hostname() string {
	return s.hostname
}

// identityMiddleware resolves the Tailscale identity of the caller and injects
// it into the request context as a PeerIdentity value. Downstream handlers can
// retrieve it with PeerIdentityFromContext.
func (s *TsnetServer) identityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.tsClient == nil {
			next.ServeHTTP(w, r)
			return
		}

		remoteAddr := r.RemoteAddr
		if remoteAddr == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Extract the IP for the identity lookup
		host, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			host = remoteAddr
		}

		// Only attempt identity resolution for Tailscale IPs
		if !isTailscaleIP(host) {
			next.ServeHTTP(w, r)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		who, err := s.tsClient.WhoIs(ctx, remoteAddr)
		if err != nil {
			util.Debug.Debugf("tsnet identity: WhoIs failed for %s: %v", remoteAddr, err)
			next.ServeHTTP(w, r)
			return
		}

		identity := &PeerIdentity{
			Hostname: who.Node.HostName,
			IP:       host,
			IsTagged: who.Node.HasTag(FleetTag),
		}
		if who.UserProfile != nil {
			identity.LoginName = who.UserProfile.LoginName
		}

		// Inject identity into context
		r = r.WithContext(context.WithValue(r.Context(), peerIdentityKey{}, identity))
		next.ServeHTTP(w, r)
	})
}

// MCPGatewayHandler returns an http.Handler that serves as a basic MCP gateway
// endpoint. It returns the server's identity and status, suitable as a health
// and discovery endpoint for MCP clients connecting over the tailnet.
func (s *TsnetServer) MCPGatewayHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer := PeerIdentityFromContext(r.Context())

		resp := map[string]any{
			"server":   s.hostname,
			"port":     s.port,
			"protocol": "mcp-over-http",
			"started":  s.startedAt.Format(time.RFC3339),
		}

		if peer != nil {
			resp["caller"] = map[string]any{
				"hostname":   peer.Hostname,
				"ip":         peer.IP,
				"login_name": peer.LoginName,
				"is_tagged":  peer.IsTagged,
			}
		}

		if s.discovery != nil {
			online := s.discovery.OnlinePeers()
			resp["fleet_peers"] = len(online)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
}

// HealthHandler returns a simple health check handler for the tsnet server.
func (s *TsnetServer) HealthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":   "ok",
			"hostname": s.hostname,
			"uptime_s": time.Since(s.startedAt).Seconds(),
		})
	})
}
