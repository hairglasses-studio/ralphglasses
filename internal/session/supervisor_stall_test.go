package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// injectSession adds a session directly to the manager's internal map for testing.
func injectSession(mgr *Manager, s *Session) {
	mgr.sessionsMu.Lock()
	mgr.sessions[s.ID] = s
	mgr.sessionsMu.Unlock()
}

func TestSupervisorStallHandler_NoStalls(t *testing.T) {
	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())
	bus := events.NewBus(100)
	h := NewSupervisorStallHandler()

	// No sessions at all — should return nil.
	killed := h.CheckAndHandle(context.Background(), mgr, bus, "/tmp/test")
	if len(killed) != 0 {
		t.Fatalf("expected no kills, got %d", len(killed))
	}

	// Add a healthy running session (recent activity).
	injectSession(mgr, &Session{
		ID:           "healthy-1",
		Status:       StatusRunning,
		LastActivity: time.Now(),
		Prompt:       "do something",
		Provider:     ProviderClaude,
		RepoPath:     "/tmp/test",
	})

	killed = h.CheckAndHandle(context.Background(), mgr, bus, "/tmp/test")
	if len(killed) != 0 {
		t.Fatalf("expected no kills for healthy session, got %d", len(killed))
	}
}

func TestSupervisorStallHandler_DetectsAndKills(t *testing.T) {
	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())
	bus := events.NewBus(100)
	sub := bus.Subscribe("test")

	h := NewSupervisorStallHandler()
	h.Threshold = 1 * time.Millisecond // very short for testing

	// Add a stalled session (old activity).
	injectSession(mgr, &Session{
		ID:           "stalled-1",
		Status:       StatusRunning,
		LastActivity: time.Now().Add(-10 * time.Minute),
		Prompt:       "fix the bug",
		Provider:     ProviderClaude,
		RepoPath:     "/tmp/test",
	})

	killed := h.CheckAndHandle(context.Background(), mgr, bus, "/tmp/test")
	if len(killed) != 1 {
		t.Fatalf("expected 1 kill, got %d", len(killed))
	}
	if killed[0] != "stalled-1" {
		t.Fatalf("expected killed session stalled-1, got %s", killed[0])
	}

	// Verify event was published.
	select {
	case ev := <-sub:
		if ev.Type != events.SessionError {
			t.Fatalf("expected SessionError event, got %s", ev.Type)
		}
		if ev.Data["reason"] != "stalled" {
			t.Fatalf("expected reason=stalled, got %v", ev.Data["reason"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for stall event")
	}
}

func TestSupervisorStallHandler_RetryLimit(t *testing.T) {
	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())
	bus := events.NewBus(100)

	h := NewSupervisorStallHandler()
	h.Threshold = 1 * time.Millisecond
	h.MaxRetries = 2

	sessionID := "retry-me"

	// Simulate 3 stall detections. First 2 should trigger retries, 3rd should not.
	for i := range 3 {
		injectSession(mgr, &Session{
			ID:           sessionID,
			Status:       StatusRunning,
			LastActivity: time.Now().Add(-10 * time.Minute),
			Prompt:       "do the thing",
			Provider:     ProviderClaude,
			RepoPath:     "/tmp/test",
		})

		killed := h.CheckAndHandle(context.Background(), mgr, bus, "/tmp/test")
		if len(killed) != 1 {
			t.Fatalf("iteration %d: expected 1 kill, got %d", i, len(killed))
		}
	}

	// After 3 kills, retry count should be at MaxRetries (2).
	if got := h.RetryCount(sessionID); got != 2 {
		t.Fatalf("expected retry count 2, got %d", got)
	}
}

func TestSupervisorStallHandler_EventPerStall(t *testing.T) {
	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())
	bus := events.NewBus(100)
	sub := bus.Subscribe("test")

	h := NewSupervisorStallHandler()
	h.Threshold = 1 * time.Millisecond

	// Add two stalled sessions.
	for _, id := range []string{"s1", "s2"} {
		injectSession(mgr, &Session{
			ID:           id,
			Status:       StatusRunning,
			LastActivity: time.Now().Add(-10 * time.Minute),
			Prompt:       "test prompt",
			Provider:     ProviderClaude,
			RepoPath:     "/tmp/test",
		})
	}

	killed := h.CheckAndHandle(context.Background(), mgr, bus, "/tmp/test")
	if len(killed) != 2 {
		t.Fatalf("expected 2 kills, got %d", len(killed))
	}

	// Should have received 2 events.
	received := 0
	timeout := time.After(time.Second)
	for received < 2 {
		select {
		case ev := <-sub:
			if ev.Type != events.SessionError {
				t.Fatalf("expected SessionError, got %s", ev.Type)
			}
			received++
		case <-timeout:
			t.Fatalf("timeout: only received %d of 2 events", received)
		}
	}
}

func TestSupervisorStallHandler_ConcurrentSafe(t *testing.T) {
	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())
	bus := events.NewBus(100)
	h := NewSupervisorStallHandler()
	h.Threshold = 1 * time.Millisecond

	// Run concurrent CheckAndHandle calls to verify no races.
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			_ = h.CheckAndHandle(context.Background(), mgr, bus, "/tmp/test")
		})
	}
	wg.Wait()
}
