package fleet

import (
	"net"
	"testing"
)

func TestGetTailscaleStatus_NoTailscale(t *testing.T) {
	// GetTailscaleStatus shells out to `tailscale` which likely doesn't exist in CI
	_, err := GetTailscaleStatus()
	if err == nil {
		t.Skip("tailscale is installed; skipping negative test")
	}
	// Error should mention tailscale
	if err != nil && !contains(err.Error(), "tailscale") {
		t.Errorf("error should mention tailscale, got: %v", err)
	}
}

func TestGetSelfIP_NoTailscale(t *testing.T) {
	ip := GetSelfIP()
	// Without tailscale, should return empty string
	if ip != "" {
		t.Skip("tailscale is installed; skipping")
	}
}

func TestDiscoverCoordinator_NoTailscale(t *testing.T) {
	result := DiscoverCoordinator(9090)
	if result != "" {
		t.Skip("tailscale is installed; skipping")
	}
}

func TestGetLocalIP_ValidIP(t *testing.T) {
	ip := GetLocalIP()
	if ip == "" {
		t.Error("GetLocalIP should return a non-empty string")
	}
	// Should be a valid IP address
	parsed := net.ParseIP(ip)
	if parsed == nil {
		t.Errorf("GetLocalIP returned invalid IP: %q", ip)
	}
}

func TestGetLocalIP_IsNotEmpty(t *testing.T) {
	ip := GetLocalIP()
	// At minimum should return 127.0.0.1 as fallback
	if ip == "" {
		t.Error("GetLocalIP should never return empty")
	}
}

func TestTailscaleStatus_Struct(t *testing.T) {
	// Verify the struct fields are correctly accessible
	status := TailscaleStatus{
		Self: TailscalePeer{
			HostName:     "myhost",
			DNSName:      "myhost.tailnet.ts.net",
			TailscaleIPs: []string{"100.64.0.1"},
			Online:       true,
			OS:           "linux",
		},
		Peer: map[string]TailscalePeer{
			"peer1": {
				HostName:     "peer1",
				TailscaleIPs: []string{"100.64.0.2"},
				Online:       true,
			},
		},
	}
	if status.Self.HostName != "myhost" {
		t.Errorf("unexpected hostname: %s", status.Self.HostName)
	}
	if len(status.Peer) != 1 {
		t.Errorf("expected 1 peer, got %d", len(status.Peer))
	}
}

func TestProbeCoordinator_NoServer(t *testing.T) {
	// probeCoordinator should return false when no server is listening
	result := probeCoordinator("http://127.0.0.1:1") // port 1 is unlikely to be open
	if result {
		t.Error("probeCoordinator should return false for unreachable server")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
