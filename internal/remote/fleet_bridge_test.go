package remote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func serveSessions(t *testing.T, sessions []RemoteSession) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(sessions); err != nil {
			t.Errorf("encode sessions: %v", err)
		}
	}))
}

func TestFleetBridge_PollSingle(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	sessions := []RemoteSession{
		{ID: "s1", Host: "host-a", Status: "running", Provider: "claude", StartedAt: now, CostUSD: 0.12, LastSeen: now},
	}

	srv := serveSessions(t, sessions)
	defer srv.Close()

	fb := NewFleetBridge([]string{srv.URL})
	if err := fb.Poll(context.Background()); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	got := fb.Sessions()
	if len(got) != 1 {
		t.Fatalf("expected 1 session, got %d", len(got))
	}
	if got[0].ID != "s1" {
		t.Errorf("expected ID s1, got %s", got[0].ID)
	}
	if got[0].Provider != "claude" {
		t.Errorf("expected provider claude, got %s", got[0].Provider)
	}
}

func TestFleetBridge_PollMultipleEndpoints(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	srv1 := serveSessions(t, []RemoteSession{
		{ID: "s1", Host: "host-a", Status: "running", Provider: "claude", StartedAt: now, CostUSD: 0.10},
		{ID: "s2", Host: "host-a", Status: "idle", Provider: "gemini", StartedAt: now, CostUSD: 0.05},
	})
	defer srv1.Close()

	srv2 := serveSessions(t, []RemoteSession{
		{ID: "s3", Host: "host-b", Status: "running", Provider: "openai", StartedAt: now, CostUSD: 0.20},
	})
	defer srv2.Close()

	fb := NewFleetBridge([]string{srv1.URL, srv2.URL})
	if err := fb.Poll(context.Background()); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	got := fb.Sessions()
	if len(got) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(got))
	}
}

func TestFleetBridge_HealthAllUp(t *testing.T) {
	srv := serveSessions(t, nil)
	defer srv.Close()

	fb := NewFleetBridge([]string{srv.URL})
	if err := fb.Poll(context.Background()); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	if !fb.Healthy() {
		t.Error("expected healthy=true when all endpoints respond")
	}
}

func TestFleetBridge_HealthPartialDown(t *testing.T) {
	srv := serveSessions(t, nil)
	defer srv.Close()

	fb := NewFleetBridge([]string{srv.URL, "http://127.0.0.1:1"})
	if err := fb.Poll(context.Background()); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	if fb.Healthy() {
		t.Error("expected healthy=false when one endpoint is unreachable")
	}

	stats := fb.Stats()
	if stats.Healthy != 1 {
		t.Errorf("expected 1 healthy, got %d", stats.Healthy)
	}
	if stats.Unhealthy != 1 {
		t.Errorf("expected 1 unhealthy, got %d", stats.Unhealthy)
	}
}

func TestFleetBridge_HealthNoEndpoints(t *testing.T) {
	fb := NewFleetBridge(nil)
	if !fb.Healthy() {
		t.Error("expected healthy=true with no endpoints")
	}
}

func TestFleetBridge_Stats(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	srv := serveSessions(t, []RemoteSession{
		{ID: "s1", Provider: "claude", CostUSD: 0.10, StartedAt: now},
		{ID: "s2", Provider: "claude", CostUSD: 0.25, StartedAt: now},
		{ID: "s3", Provider: "gemini", CostUSD: 0.05, StartedAt: now},
	})
	defer srv.Close()

	fb := NewFleetBridge([]string{srv.URL})
	if err := fb.Poll(context.Background()); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	stats := fb.Stats()
	if stats.TotalSessions != 3 {
		t.Errorf("expected 3 total sessions, got %d", stats.TotalSessions)
	}
	const epsilon = 0.001
	expectedCost := 0.40
	if diff := stats.TotalCostUSD - expectedCost; diff > epsilon || diff < -epsilon {
		t.Errorf("expected cost %.2f, got %.4f", expectedCost, stats.TotalCostUSD)
	}
	if stats.Providers["claude"] != 2 {
		t.Errorf("expected 2 claude sessions, got %d", stats.Providers["claude"])
	}
	if stats.Providers["gemini"] != 1 {
		t.Errorf("expected 1 gemini session, got %d", stats.Providers["gemini"])
	}
	if stats.Healthy != 1 {
		t.Errorf("expected 1 healthy endpoint, got %d", stats.Healthy)
	}
}

func TestFleetBridge_PollCanceledContext(t *testing.T) {
	fb := NewFleetBridge([]string{"http://127.0.0.1:1"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := fb.Poll(ctx)
	if err == nil {
		t.Error("expected error from canceled context")
	}
}

func TestFleetBridge_PollBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	fb := NewFleetBridge([]string{srv.URL})
	if err := fb.Poll(context.Background()); err != nil {
		t.Fatalf("Poll should not return error for unhealthy endpoint: %v", err)
	}

	if fb.Healthy() {
		t.Error("expected healthy=false for 500 response")
	}
	if len(fb.Sessions()) != 0 {
		t.Error("expected 0 sessions for failed endpoint")
	}
}

func TestFleetBridge_ConcurrentAccess(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	srv := serveSessions(t, []RemoteSession{
		{ID: "s1", Provider: "claude", CostUSD: 0.10, StartedAt: now},
	})
	defer srv.Close()

	fb := NewFleetBridge([]string{srv.URL})

	var wg sync.WaitGroup
	const goroutines = 20

	// Half poll, half read.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		if i%2 == 0 {
			go func() {
				defer wg.Done()
				_ = fb.Poll(context.Background())
			}()
		} else {
			go func() {
				defer wg.Done()
				_ = fb.Sessions()
				_ = fb.Healthy()
				_ = fb.Stats()
			}()
		}
	}

	wg.Wait()

	// If we get here without -race detector complaining, concurrent access is safe.
	if got := fb.Sessions(); len(got) != 1 {
		t.Errorf("expected 1 session after concurrent access, got %d", len(got))
	}
}
