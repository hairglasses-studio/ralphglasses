package fleet

import (
	"fmt"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestNewWorkerAgent_Basic(t *testing.T) {
	bus := events.NewBus(100)
	mgr := session.NewManager()

	w := NewWorkerAgent("http://localhost:9090", "test-host", 8080, "1.0.0", "/tmp/scan", bus, mgr)

	if w == nil {
		t.Fatal("NewWorkerAgent returned nil")
	}
	if w.hostname != "test-host" {
		t.Errorf("hostname = %q, want %q", w.hostname, "test-host")
	}
	if w.port != 8080 {
		t.Errorf("port = %d, want 8080", w.port)
	}
	if w.version != "1.0.0" {
		t.Errorf("version = %q, want %q", w.version, "1.0.0")
	}
	if w.scanPath != "/tmp/scan" {
		t.Errorf("scanPath = %q, want %q", w.scanPath, "/tmp/scan")
	}
	if w.client == nil {
		t.Error("client should not be nil")
	}
	if w.sessMgr == nil {
		t.Error("sessMgr should not be nil")
	}
	if w.bus == nil {
		t.Error("bus should not be nil")
	}
	if w.startedAt.IsZero() {
		t.Error("startedAt should be set")
	}
	if w.NodeID() != "" {
		t.Errorf("NodeID should be empty before registration, got %q", w.NodeID())
	}
}

func TestNewWorkerAgent_NilBusAndManager(t *testing.T) {
	w := NewWorkerAgent("http://localhost:9090", "host", 8080, "1.0", "", nil, nil)
	if w == nil {
		t.Fatal("NewWorkerAgent returned nil with nil bus/manager")
	}
	if w.bus != nil {
		t.Error("bus should be nil when passed nil")
	}
	if w.sessMgr != nil {
		t.Error("sessMgr should be nil when passed nil")
	}
}

func TestNewWorkerAgent_EmptyScanPath(t *testing.T) {
	w := NewWorkerAgent("http://coord:9090", "host", 0, "", "", nil, nil)
	if w.scanPath != "" {
		t.Errorf("scanPath = %q, want empty", w.scanPath)
	}
}

func TestDiscoverTailscaleIP_WorkerIntegration(t *testing.T) {
	original := DefaultTailscaleClient()
	defer SetTailscaleClient(original)

	// With a working mock, should return the first IPv4 from Self.
	SetTailscaleClient(&mockTSClient{
		status: &TailscaleStatus{
			Self: TailscalePeer{
				TailscaleIPs: []string{"100.64.7.42", "fd7a:115c:a1e0::7"},
			},
		},
	})
	ip := DiscoverTailscaleIP()
	if ip != "100.64.7.42" {
		t.Errorf("DiscoverTailscaleIP = %q, want 100.64.7.42", ip)
	}

	// With a failing mock, should return empty.
	SetTailscaleClient(&mockTSClient{
		statusErr: fmt.Errorf("tailscale not running"),
	})
	ip = DiscoverTailscaleIP()
	if ip != "" {
		t.Errorf("DiscoverTailscaleIP should return empty when unavailable, got %q", ip)
	}
}

func TestWorkerAgent_DiscoverRepos_EmptyScanPath(t *testing.T) {
	w := &WorkerAgent{scanPath: ""}
	repos := w.discoverRepos(t.Context())
	if repos != nil {
		t.Errorf("discoverRepos with empty scanPath should return nil, got %v", repos)
	}
}

func TestWorkerAgent_DiscoverRepos_NonexistentPath(t *testing.T) {
	w := &WorkerAgent{scanPath: "/nonexistent/path/that/does/not/exist"}
	repos := w.discoverRepos(t.Context())
	if repos != nil {
		t.Errorf("discoverRepos with nonexistent path should return nil, got %v", repos)
	}
}

func TestWorkerAgent_DiscoverProviders(t *testing.T) {
	w := &WorkerAgent{}
	providers := w.discoverProviders()
	if len(providers) == 0 {
		t.Fatal("discoverProviders should return at least one provider")
	}
	if providers[0] != session.DefaultPrimaryProvider() {
		t.Errorf("discoverProviders first provider = %q, want %q", providers[0], session.DefaultPrimaryProvider())
	}
}

func TestWorkerAgent_EventForwardLoop_NilBus(t *testing.T) {
	// eventForwardLoop with nil bus should return immediately
	w := &WorkerAgent{bus: nil}
	// This should not block or panic
	w.eventForwardLoop(t.Context())
}
