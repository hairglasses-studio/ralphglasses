package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

// TailscaleStatus represents the JSON output of `tailscale status --json`.
type TailscaleStatus struct {
	Self             TailscalePeer            `json:"Self"`
	Peer             map[string]TailscalePeer `json:"Peer"`
	MagicDNSSuffix   string                   `json:"MagicDNSSuffix,omitempty"`
	CurrentTailnet   *TailnetInfo             `json:"CurrentTailnet,omitempty"`
}

// TailnetInfo describes the tailnet this node belongs to.
type TailnetInfo struct {
	Name            string `json:"Name"`
	MagicDNSSuffix  string `json:"MagicDNSSuffix"`
	MagicDNSEnabled bool   `json:"MagicDNSEnabled"`
}

// TailscalePeer represents a single Tailscale peer.
type TailscalePeer struct {
	HostName     string   `json:"HostName"`
	DNSName      string   `json:"DNSName"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Online       bool     `json:"Online"`
	OS           string   `json:"OS"`
	Tags         []string `json:"Tags,omitempty"`
	UserID       int64    `json:"UserID,omitempty"`
}

// TailscaleWhoIsResponse represents the response from the Tailscale WhoIs endpoint.
type TailscaleWhoIsResponse struct {
	Node    TailscalePeer     `json:"Node"`
	UserProfile *TailscaleUser `json:"UserProfile,omitempty"`
}

// TailscaleUser represents a Tailscale user profile.
type TailscaleUser struct {
	ID          int64  `json:"ID"`
	LoginName   string `json:"LoginName"`
	DisplayName string `json:"DisplayName"`
}

// TailscaleClient abstracts Tailscale operations so implementations can use
// either the local API (unix socket) or the CLI as a fallback.
type TailscaleClient interface {
	// Status returns the current Tailscale network status.
	Status(ctx context.Context) (*TailscaleStatus, error)
	// WhoIs identifies a peer by their remote address (ip:port).
	WhoIs(ctx context.Context, remoteAddr string) (*TailscaleWhoIsResponse, error)
}

// --- LocalAPI client (talks to tailscaled via unix socket) ---

const tailscaleSocketPath = "/var/run/tailscale/tailscaled.sock"

// LocalAPIClient communicates with tailscaled via its local HTTP API on a
// unix domain socket. This avoids shelling out to the CLI binary.
type LocalAPIClient struct {
	httpClient *http.Client
}

// NewLocalAPIClient creates a client that speaks to the tailscaled unix socket.
func NewLocalAPIClient() *LocalAPIClient {
	return &LocalAPIClient{
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", tailscaleSocketPath)
				},
			},
			Timeout: 5 * time.Second,
		},
	}
}

func (c *LocalAPIClient) Status(ctx context.Context) (*TailscaleStatus, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://local-tailscaled.sock/localapi/v0/status", nil)
	if err != nil {
		return nil, fmt.Errorf("tailscale localapi status request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tailscale localapi status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tailscale localapi status HTTP %d: %s", resp.StatusCode, string(body))
	}

	var status TailscaleStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("parse tailscale localapi status: %w", err)
	}
	return &status, nil
}

func (c *LocalAPIClient) WhoIs(ctx context.Context, remoteAddr string) (*TailscaleWhoIsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		"http://local-tailscaled.sock/localapi/v0/whois?addr="+remoteAddr, nil)
	if err != nil {
		return nil, fmt.Errorf("tailscale localapi whois request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tailscale localapi whois: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tailscale localapi whois HTTP %d: %s", resp.StatusCode, string(body))
	}

	var who TailscaleWhoIsResponse
	if err := json.NewDecoder(resp.Body).Decode(&who); err != nil {
		return nil, fmt.Errorf("parse tailscale localapi whois: %w", err)
	}
	return &who, nil
}

// --- CLI fallback client (shells out to `tailscale`) ---

// CLIClient shells out to the tailscale CLI binary. Used as a fallback when
// the local unix socket is unavailable (e.g. macOS where the socket path differs).
type CLIClient struct{}

func (c *CLIClient) Status(ctx context.Context) (*TailscaleStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tailscale", "status", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tailscale status: %w", err)
	}

	var status TailscaleStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("parse tailscale status: %w", err)
	}
	return &status, nil
}

func (c *CLIClient) WhoIs(ctx context.Context, remoteAddr string) (*TailscaleWhoIsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// `tailscale whois` outputs JSON with --json flag
	cmd := exec.CommandContext(ctx, "tailscale", "whois", "--json", remoteAddr)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tailscale whois: %w", err)
	}

	var who TailscaleWhoIsResponse
	if err := json.Unmarshal(out, &who); err != nil {
		return nil, fmt.Errorf("parse tailscale whois: %w", err)
	}
	return &who, nil
}

// --- Client selection ---

var (
	defaultTSClient     TailscaleClient
	defaultTSClientOnce sync.Once
)

// DefaultTailscaleClient returns a TailscaleClient, preferring the local API
// socket and falling back to the CLI. The result is cached after first call.
func DefaultTailscaleClient() TailscaleClient {
	defaultTSClientOnce.Do(func() {
		defaultTSClient = detectTailscaleClient()
	})
	return defaultTSClient
}

// SetTailscaleClient overrides the default client (useful for testing).
func SetTailscaleClient(c TailscaleClient) {
	defaultTSClient = c
}

func detectTailscaleClient() TailscaleClient {
	// Try the local API socket first — faster, no exec overhead.
	local := NewLocalAPIClient()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := local.Status(ctx); err == nil {
		util.Debug.Debugf("tailscale: using LocalAPI client (socket %s)", tailscaleSocketPath)
		return local
	}

	// Fall back to CLI.
	cli := &CLIClient{}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()

	if _, err := cli.Status(ctx2); err == nil {
		util.Debug.Debugf("tailscale: using CLI client (fallback)")
		return cli
	}

	util.Debug.Debugf("tailscale: no working client found; using CLI fallback (will error on use)")
	return cli
}

// --- Public API (uses DefaultTailscaleClient) ---

// GetTailscaleStatus returns the current Tailscale network status.
// Uses the LocalAPI when available, falling back to CLI shell-out.
func GetTailscaleStatus() (*TailscaleStatus, error) {
	return DefaultTailscaleClient().Status(context.Background())
}

// GetSelfIP returns this node's Tailscale IP.
func GetSelfIP() string {
	status, err := GetTailscaleStatus()
	if err != nil {
		return ""
	}
	if len(status.Self.TailscaleIPs) > 0 {
		return status.Self.TailscaleIPs[0]
	}
	return ""
}

// MagicDNSCoordinatorName is the well-known hostname that coordinator nodes
// should register under in Tailscale MagicDNS. DiscoverCoordinator tries
// resolving this name before falling back to full peer enumeration.
const MagicDNSCoordinatorName = "ralph-coord-01"

// DiscoverCoordinator probes Tailscale peers on the fleet port to find a coordinator.
// It first tries MagicDNS resolution of the well-known coordinator name, then
// falls back to enumerating all online peers. Returns the base URL of the first
// responding coordinator, or empty string.
func DiscoverCoordinator(port int) string {
	// Phase 1: MagicDNS lookup — fast path when the coordinator has a stable name.
	if url := discoverViaMagicDNS(port); url != "" {
		return url
	}

	// Phase 2: Enumerate peers from tailscale status.
	status, err := GetTailscaleStatus()
	if err != nil {
		util.Debug.Debugf("tailscale discovery failed: %v", err)
		return ""
	}

	// Try the MagicDNS suffix variant: resolve ralph-coord-01.<tailnet>.ts.net
	if suffix := tailnetDNSSuffix(status); suffix != "" {
		fqdn := MagicDNSCoordinatorName + "." + suffix
		if url := discoverByDNSName(fqdn, port); url != "" {
			return url
		}
	}

	for _, peer := range status.Peer {
		if !peer.Online {
			continue
		}
		for _, ip := range peer.TailscaleIPs {
			url := fmt.Sprintf("http://%s:%d", ip, port)
			if probeCoordinator(url) {
				util.Debug.Debugf("discovered coordinator at %s (%s)", url, peer.HostName)
				return url
			}
		}
	}
	return ""
}

// discoverViaMagicDNS tries to resolve the well-known coordinator hostname
// directly via DNS and probes the result.
func discoverViaMagicDNS(port int) string {
	// Try bare name first (works when MagicDNS search domain is configured)
	for _, name := range []string{MagicDNSCoordinatorName} {
		if url := discoverByDNSName(name, port); url != "" {
			return url
		}
	}
	return ""
}

// discoverByDNSName resolves a hostname and probes it as a coordinator.
func discoverByDNSName(hostname string, port int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupHost(ctx, hostname)
	if err != nil || len(addrs) == 0 {
		return ""
	}

	for _, addr := range addrs {
		url := fmt.Sprintf("http://%s:%d", addr, port)
		if probeCoordinator(url) {
			util.Debug.Debugf("discovered coordinator via MagicDNS %s -> %s", hostname, url)
			return url
		}
	}
	return ""
}

// tailnetDNSSuffix extracts the MagicDNS suffix from tailscale status,
// checking both the top-level field and CurrentTailnet.
func tailnetDNSSuffix(status *TailscaleStatus) string {
	if status.MagicDNSSuffix != "" {
		return status.MagicDNSSuffix
	}
	if status.CurrentTailnet != nil && status.CurrentTailnet.MagicDNSSuffix != "" {
		return status.CurrentTailnet.MagicDNSSuffix
	}
	// Try to extract from Self.DNSName: "hostname.tailnet.ts.net." -> "tailnet.ts.net"
	if status.Self.DNSName != "" {
		parts := strings.SplitN(strings.TrimSuffix(status.Self.DNSName, "."), ".", 2)
		if len(parts) == 2 {
			return parts[1]
		}
	}
	return ""
}

// probeCoordinator checks if a URL responds as a fleet coordinator.
func probeCoordinator(baseURL string) bool {
	client := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	status, err := client.Status(ctx)
	if err != nil {
		return false
	}
	return status.Role == "coordinator"
}

// GetHostname returns the node's hostname.
func GetHostname() string {
	// Check Tailscale hostname first
	status, err := GetTailscaleStatus()
	if err == nil && status.Self.HostName != "" {
		return status.Self.HostName
	}

	// Fall back to OS hostname
	if name, err := exec.Command("hostname").Output(); err == nil {
		return string(name[:len(name)-1]) // trim newline
	}
	return "unknown"
}

// GetLocalIP returns the first non-loopback IPv4 address.
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "127.0.0.1"
}

// HasTag returns true if the peer carries the given ACL tag (e.g. "tag:ralph-fleet").
func (p *TailscalePeer) HasTag(tag string) bool {
	for _, t := range p.Tags {
		if t == tag {
			return true
		}
	}
	return false
}
