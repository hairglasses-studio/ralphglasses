package heartbeat

import (
	"sort"
	"sync"
	"testing"
	"time"
)

func TestRegisterAndHeartbeat(t *testing.T) {
	m := New(30*time.Second, 90*time.Second)
	m.Register("server-a")

	// Before any heartbeat the server should be unhealthy.
	statuses := m.Status()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Healthy {
		t.Fatal("server should be unhealthy before first heartbeat")
	}
	if statuses[0].ServerName != "server-a" {
		t.Fatalf("expected server name 'server-a', got %q", statuses[0].ServerName)
	}

	// After a heartbeat the server should be healthy.
	m.Heartbeat("server-a")
	statuses = m.Status()
	if !statuses[0].Healthy {
		t.Fatal("server should be healthy after heartbeat")
	}
	if statuses[0].LastSeen.IsZero() {
		t.Fatal("LastSeen should be set after heartbeat")
	}
}

func TestHeartbeatUnregisteredServerIsIgnored(t *testing.T) {
	m := New(30*time.Second, 90*time.Second)

	// Should not panic or create a server entry.
	m.Heartbeat("ghost")

	statuses := m.Status()
	if len(statuses) != 0 {
		t.Fatalf("expected 0 statuses, got %d", len(statuses))
	}
}

func TestTimeoutDetection(t *testing.T) {
	// Use a very short timeout so we can test expiry.
	m := New(10*time.Millisecond, 50*time.Millisecond)
	m.Register("server-a")
	m.Heartbeat("server-a")

	// Immediately after heartbeat, should be healthy.
	if unhealthy := m.Unhealthy(); len(unhealthy) != 0 {
		t.Fatalf("expected no unhealthy servers, got %v", unhealthy)
	}

	// Wait for the timeout to expire.
	time.Sleep(80 * time.Millisecond)

	unhealthy := m.Unhealthy()
	if len(unhealthy) != 1 || unhealthy[0] != "server-a" {
		t.Fatalf("expected [server-a] unhealthy, got %v", unhealthy)
	}

	// A new heartbeat should restore health.
	m.Heartbeat("server-a")
	unhealthy = m.Unhealthy()
	if len(unhealthy) != 0 {
		t.Fatalf("expected no unhealthy after recovery heartbeat, got %v", unhealthy)
	}
}

func TestMultipleServersTrackedIndependently(t *testing.T) {
	m := New(10*time.Millisecond, 50*time.Millisecond)
	m.Register("alpha")
	m.Register("beta")
	m.Register("gamma")

	// Send heartbeats to alpha and gamma only.
	m.Heartbeat("alpha")
	m.Heartbeat("gamma")

	statuses := m.Status()
	if len(statuses) != 3 {
		t.Fatalf("expected 3 statuses, got %d", len(statuses))
	}

	byName := statusMap(statuses)

	if !byName["alpha"].Healthy {
		t.Error("alpha should be healthy")
	}
	if byName["beta"].Healthy {
		t.Error("beta should be unhealthy (no heartbeat)")
	}
	if !byName["gamma"].Healthy {
		t.Error("gamma should be healthy")
	}

	// Let timeout expire, then heartbeat only beta.
	time.Sleep(80 * time.Millisecond)
	m.Heartbeat("beta")

	statuses = m.Status()
	byName = statusMap(statuses)

	if byName["alpha"].Healthy {
		t.Error("alpha should be unhealthy after timeout")
	}
	if !byName["beta"].Healthy {
		t.Error("beta should be healthy after fresh heartbeat")
	}
	if byName["gamma"].Healthy {
		t.Error("gamma should be unhealthy after timeout")
	}
}

func TestStatusReporting(t *testing.T) {
	m := New(10*time.Millisecond, 50*time.Millisecond)
	m.Register("server-a")

	// Two heartbeats to measure latency between them.
	m.Heartbeat("server-a")
	time.Sleep(20 * time.Millisecond)
	m.Heartbeat("server-a")

	statuses := m.Status()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}

	s := statuses[0]
	if s.ServerName != "server-a" {
		t.Errorf("expected server name 'server-a', got %q", s.ServerName)
	}
	if !s.Healthy {
		t.Error("server should be healthy")
	}
	if s.Latency < 15*time.Millisecond {
		t.Errorf("expected latency >= 15ms (sleep gap between heartbeats), got %v", s.Latency)
	}
}

func TestUnhealthyReturnsEmptyWhenAllHealthy(t *testing.T) {
	m := New(30*time.Second, 90*time.Second)
	m.Register("a")
	m.Register("b")
	m.Heartbeat("a")
	m.Heartbeat("b")

	unhealthy := m.Unhealthy()
	if len(unhealthy) != 0 {
		t.Fatalf("expected no unhealthy servers, got %v", unhealthy)
	}
}

func TestUnhealthyBeforeAnyHeartbeats(t *testing.T) {
	m := New(30*time.Second, 90*time.Second)
	m.Register("a")
	m.Register("b")

	unhealthy := m.Unhealthy()
	sort.Strings(unhealthy)
	if len(unhealthy) != 2 {
		t.Fatalf("expected 2 unhealthy servers, got %v", unhealthy)
	}
	if unhealthy[0] != "a" || unhealthy[1] != "b" {
		t.Fatalf("expected [a b], got %v", unhealthy)
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := New(10*time.Millisecond, 50*time.Millisecond)
	for i := 0; i < 10; i++ {
		m.Register(serverName(i))
	}

	var wg sync.WaitGroup

	// Concurrent heartbeats.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				m.Heartbeat(serverName(n))
			}
		}(i)
	}

	// Concurrent reads.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = m.Status()
				_ = m.Unhealthy()
			}
		}()
	}

	wg.Wait()

	// All servers should be healthy after the storm of heartbeats.
	unhealthy := m.Unhealthy()
	if len(unhealthy) != 0 {
		t.Fatalf("expected no unhealthy servers after concurrent heartbeats, got %v", unhealthy)
	}
}

// --- helpers ---

func serverName(i int) string {
	return "server-" + string(rune('a'+i))
}

func statusMap(statuses []Status) map[string]Status {
	m := make(map[string]Status, len(statuses))
	for _, s := range statuses {
		m[s.ServerName] = s
	}
	return m
}
