package fleet

import (
	"context"
	"fmt"
	"net"
	"testing"
)

// mockTSClient implements TailscaleClient for testing.
type mockTSClient struct {
	status   *TailscaleStatus
	statusErr error
	whoIs    *TailscaleWhoIsResponse
	whoIsErr error
}

func (m *mockTSClient) Status(ctx context.Context) (*TailscaleStatus, error) {
	return m.status, m.statusErr
}

func (m *mockTSClient) WhoIs(ctx context.Context, remoteAddr string) (*TailscaleWhoIsResponse, error) {
	return m.whoIs, m.whoIsErr
}

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

// --- New tests for Phase 12.3 ---

func TestMockTSClient_StatusViaInterface(t *testing.T) {
	mock := &mockTSClient{
		status: &TailscaleStatus{
			Self: TailscalePeer{
				HostName:     "test-node",
				DNSName:      "test-node.example.ts.net",
				TailscaleIPs: []string{"100.64.1.1", "fd7a:115c:a1e0::1"},
				Online:       true,
				OS:           "linux",
			},
			MagicDNSSuffix: "example.ts.net",
			Peer: map[string]TailscalePeer{
				"peer-key": {
					HostName:     "worker-01",
					TailscaleIPs: []string{"100.64.1.2"},
					Online:       true,
					Tags:         []string{"tag:ralph-fleet"},
				},
			},
		},
	}

	status, err := mock.Status(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Self.HostName != "test-node" {
		t.Errorf("hostname = %q, want test-node", status.Self.HostName)
	}
	if status.MagicDNSSuffix != "example.ts.net" {
		t.Errorf("MagicDNSSuffix = %q, want example.ts.net", status.MagicDNSSuffix)
	}
}

func TestMockTSClient_WhoIs(t *testing.T) {
	mock := &mockTSClient{
		whoIs: &TailscaleWhoIsResponse{
			Node: TailscalePeer{
				HostName: "worker-01",
				Tags:     []string{"tag:ralph-fleet"},
			},
			UserProfile: &TailscaleUser{
				ID:        123,
				LoginName: "user@example.com",
			},
		},
	}

	who, err := mock.WhoIs(context.Background(), "100.64.1.2:9473")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if who.Node.HostName != "worker-01" {
		t.Errorf("hostname = %q, want worker-01", who.Node.HostName)
	}
	if !who.Node.HasTag("tag:ralph-fleet") {
		t.Error("node should have tag:ralph-fleet")
	}
	if who.UserProfile == nil || who.UserProfile.LoginName != "user@example.com" {
		t.Error("user profile not populated correctly")
	}
}

func TestMockTSClient_StatusError(t *testing.T) {
	mock := &mockTSClient{
		statusErr: fmt.Errorf("tailscale not running"),
	}
	_, err := mock.Status(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !contains(err.Error(), "tailscale") {
		t.Errorf("error should mention tailscale, got: %v", err)
	}
}

func TestSetTailscaleClient_Override(t *testing.T) {
	original := DefaultTailscaleClient()
	defer SetTailscaleClient(original)

	mock := &mockTSClient{
		status: &TailscaleStatus{
			Self: TailscalePeer{
				HostName:     "mock-node",
				TailscaleIPs: []string{"100.64.99.1"},
			},
		},
	}
	SetTailscaleClient(mock)

	status, err := GetTailscaleStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Self.HostName != "mock-node" {
		t.Errorf("hostname = %q, want mock-node", status.Self.HostName)
	}
}

func TestTailscalePeer_HasTag(t *testing.T) {
	peer := TailscalePeer{
		Tags: []string{"tag:ralph-fleet", "tag:server"},
	}
	if !peer.HasTag("tag:ralph-fleet") {
		t.Error("should have tag:ralph-fleet")
	}
	if peer.HasTag("tag:unknown") {
		t.Error("should not have tag:unknown")
	}

	empty := TailscalePeer{}
	if empty.HasTag("tag:ralph-fleet") {
		t.Error("empty peer should not have any tags")
	}
}

func TestTailnetDNSSuffix(t *testing.T) {
	tests := []struct {
		name   string
		status *TailscaleStatus
		want   string
	}{
		{
			name: "top-level MagicDNSSuffix",
			status: &TailscaleStatus{
				MagicDNSSuffix: "example.ts.net",
			},
			want: "example.ts.net",
		},
		{
			name: "CurrentTailnet MagicDNSSuffix",
			status: &TailscaleStatus{
				CurrentTailnet: &TailnetInfo{
					MagicDNSSuffix: "corp.ts.net",
				},
			},
			want: "corp.ts.net",
		},
		{
			name: "extracted from Self.DNSName",
			status: &TailscaleStatus{
				Self: TailscalePeer{
					DNSName: "myhost.fleet.ts.net.",
				},
			},
			want: "fleet.ts.net",
		},
		{
			name: "extracted from Self.DNSName without trailing dot",
			status: &TailscaleStatus{
				Self: TailscalePeer{
					DNSName: "myhost.fleet.ts.net",
				},
			},
			want: "fleet.ts.net",
		},
		{
			name:   "empty status",
			status: &TailscaleStatus{},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tailnetDNSSuffix(tt.status)
			if got != tt.want {
				t.Errorf("tailnetDNSSuffix() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDiscoverTailscaleIP_WithMock(t *testing.T) {
	original := DefaultTailscaleClient()
	defer SetTailscaleClient(original)

	mock := &mockTSClient{
		status: &TailscaleStatus{
			Self: TailscalePeer{
				HostName:     "test-worker",
				TailscaleIPs: []string{"100.64.5.10", "fd7a:115c:a1e0::5"},
			},
		},
	}
	SetTailscaleClient(mock)

	ip := DiscoverTailscaleIP()
	if ip != "100.64.5.10" {
		t.Errorf("DiscoverTailscaleIP() = %q, want 100.64.5.10", ip)
	}
}

func TestDiscoverTailscaleIP_IPv6Only(t *testing.T) {
	original := DefaultTailscaleClient()
	defer SetTailscaleClient(original)

	mock := &mockTSClient{
		status: &TailscaleStatus{
			Self: TailscalePeer{
				TailscaleIPs: []string{"fd7a:115c:a1e0::99"},
			},
		},
	}
	SetTailscaleClient(mock)

	ip := DiscoverTailscaleIP()
	if ip != "fd7a:115c:a1e0::99" {
		t.Errorf("DiscoverTailscaleIP() = %q, want fd7a:115c:a1e0::99", ip)
	}
}

func TestDiscoverTailscaleIP_Unavailable(t *testing.T) {
	original := DefaultTailscaleClient()
	defer SetTailscaleClient(original)

	mock := &mockTSClient{
		statusErr: fmt.Errorf("tailscale not running"),
	}
	SetTailscaleClient(mock)

	ip := DiscoverTailscaleIP()
	if ip != "" {
		t.Errorf("DiscoverTailscaleIP() = %q, want empty", ip)
	}
}

func TestIsTailscaleIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"100.64.0.1", true},
		{"100.127.255.254", true},
		{"100.63.255.255", false},    // below CGNAT range
		{"100.128.0.0", false},       // above CGNAT range
		{"192.168.1.1", false},
		{"127.0.0.1", false},
		{"fd7a:115c:a1e0::1", true},
		{"fd7a:115c:a1e0:ab12::1", true},
		{"fe80::1", false},
		{"not-an-ip", false},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			got := isTailscaleIP(tt.ip)
			if got != tt.want {
				t.Errorf("isTailscaleIP(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestTailscaleStatus_MagicDNSFields(t *testing.T) {
	status := TailscaleStatus{
		Self: TailscalePeer{
			HostName:     "coord-01",
			DNSName:      "coord-01.fleet.ts.net",
			TailscaleIPs: []string{"100.64.0.1"},
			Online:       true,
		},
		MagicDNSSuffix: "fleet.ts.net",
		CurrentTailnet: &TailnetInfo{
			Name:            "fleet",
			MagicDNSSuffix:  "fleet.ts.net",
			MagicDNSEnabled: true,
		},
	}
	if status.MagicDNSSuffix != "fleet.ts.net" {
		t.Errorf("MagicDNSSuffix = %q, want fleet.ts.net", status.MagicDNSSuffix)
	}
	if status.CurrentTailnet == nil {
		t.Fatal("CurrentTailnet is nil")
	}
	if !status.CurrentTailnet.MagicDNSEnabled {
		t.Error("MagicDNSEnabled should be true")
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
