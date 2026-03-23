package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

// TailscaleStatus represents the JSON output of `tailscale status --json`.
type TailscaleStatus struct {
	Self  TailscalePeer            `json:"Self"`
	Peer  map[string]TailscalePeer `json:"Peer"`
}

// TailscalePeer represents a single Tailscale peer.
type TailscalePeer struct {
	HostName     string   `json:"HostName"`
	DNSName      string   `json:"DNSName"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Online       bool     `json:"Online"`
	OS           string   `json:"OS"`
}

// GetTailscaleStatus runs `tailscale status --json` and parses the result.
func GetTailscaleStatus() (*TailscaleStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

// DiscoverCoordinator probes Tailscale peers on the fleet port to find a coordinator.
// Returns the base URL of the first responding coordinator, or empty string.
func DiscoverCoordinator(port int) string {
	status, err := GetTailscaleStatus()
	if err != nil {
		util.Debug.Debugf("tailscale discovery failed: %v", err)
		return ""
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
