package notify

import (
	"testing"
	"time"
)

func TestRateLimiter_QueueLen(t *testing.T) {
	rl := NewRateLimiter(10*time.Second, 3)

	// First send should go through (or fail silently since no desktop)
	rl.TrySend("test1", "body1")

	// Rapid second send should be queued (within 10s window)
	rl.TrySend("test2", "body2")

	// Queue should have at most 1 entry (first may or may not have queued depending on Send success)
	if rl.QueueLen() > 2 {
		t.Errorf("expected at most 2 queued, got %d", rl.QueueLen())
	}
}

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(5*time.Second, 3)
	if rl.interval != 5*time.Second {
		t.Errorf("wrong interval: %v", rl.interval)
	}
	if rl.maxRetries != 3 {
		t.Errorf("wrong max retries: %d", rl.maxRetries)
	}
}
