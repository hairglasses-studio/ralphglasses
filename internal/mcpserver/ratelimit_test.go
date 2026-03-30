package mcpserver

import (
	"sync"
	"testing"
	"time"
)

// fakeClock returns a controllable clock for deterministic tests.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t time.Time) *fakeClock {
	return &fakeClock{now: t}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestRateLimiter_AllowWithinBudget(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	rl := NewRateLimiter(
		WithClock(clock.Now),
		WithDefaultRate(5, 5),
	)

	// Should allow up to burst (5 requests) immediately.
	for i := 0; i < 5; i++ {
		if !rl.Allow("core") {
			t.Fatalf("request %d should have been allowed", i+1)
		}
	}

	stats := rl.Stats("core")
	if stats.Allowed != 5 {
		t.Errorf("expected 5 allowed, got %d", stats.Allowed)
	}
	if stats.Throttled != 0 {
		t.Errorf("expected 0 throttled, got %d", stats.Throttled)
	}
}

func TestRateLimiter_ThrottleWhenExceeded(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	rl := NewRateLimiter(
		WithClock(clock.Now),
		WithDefaultRate(2, 3),
	)

	// Drain the burst.
	for i := 0; i < 3; i++ {
		rl.Allow("session")
	}

	// Next request should be throttled (no time has passed).
	if rl.Allow("session") {
		t.Fatal("expected throttle after burst exhausted")
	}

	stats := rl.Stats("session")
	if stats.Throttled != 1 {
		t.Errorf("expected 1 throttled, got %d", stats.Throttled)
	}
}

func TestRateLimiter_RefillAfterTime(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	rl := NewRateLimiter(
		WithClock(clock.Now),
		WithDefaultRate(2, 3),
	)

	// Drain burst.
	for i := 0; i < 3; i++ {
		rl.Allow("loop")
	}
	if rl.Allow("loop") {
		t.Fatal("should be throttled")
	}

	// Advance 1 second -> 2 tokens refilled (rate=2/s).
	clock.Advance(1 * time.Second)

	if !rl.Allow("loop") {
		t.Fatal("should allow after refill")
	}
	if !rl.Allow("loop") {
		t.Fatal("should allow second token after refill")
	}
	// Third should be throttled again.
	if rl.Allow("loop") {
		t.Fatal("should throttle after consuming refilled tokens")
	}
}

func TestRateLimiter_PerNamespaceIsolation(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	rl := NewRateLimiter(
		WithClock(clock.Now),
		WithNamespaceRate("fleet", 1, 1),
		WithNamespaceRate("prompt", 100, 100),
	)

	// Drain fleet's single token.
	if !rl.Allow("fleet") {
		t.Fatal("fleet first request should be allowed")
	}
	if rl.Allow("fleet") {
		t.Fatal("fleet should be throttled")
	}

	// prompt should still be available.
	for i := 0; i < 50; i++ {
		if !rl.Allow("prompt") {
			t.Fatalf("prompt request %d should be allowed", i+1)
		}
	}

	fleetStats := rl.Stats("fleet")
	promptStats := rl.Stats("prompt")

	if fleetStats.Allowed != 1 {
		t.Errorf("fleet: expected 1 allowed, got %d", fleetStats.Allowed)
	}
	if fleetStats.Throttled != 1 {
		t.Errorf("fleet: expected 1 throttled, got %d", fleetStats.Throttled)
	}
	if promptStats.Allowed != 50 {
		t.Errorf("prompt: expected 50 allowed, got %d", promptStats.Allowed)
	}
	if promptStats.Throttled != 0 {
		t.Errorf("prompt: expected 0 throttled, got %d", promptStats.Throttled)
	}
}

func TestRateLimiter_BurstHandling(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	rl := NewRateLimiter(
		WithClock(clock.Now),
		WithNamespaceRate("eval", 10, 5),
	)

	// Burst of 5 should all succeed.
	allowed := 0
	for i := 0; i < 10; i++ {
		if rl.Allow("eval") {
			allowed++
		}
	}
	if allowed != 5 {
		t.Errorf("expected exactly 5 allowed in burst, got %d", allowed)
	}

	// Wait long enough to refill to burst cap (not beyond).
	// rate=10/s, burst=5, so 1 second would generate 10 tokens but cap at 5.
	clock.Advance(2 * time.Second)

	allowed = 0
	for i := 0; i < 10; i++ {
		if rl.Allow("eval") {
			allowed++
		}
	}
	if allowed != 5 {
		t.Errorf("expected exactly 5 allowed after refill to burst cap, got %d", allowed)
	}
}

func TestRateLimiter_DefaultBucketCreation(t *testing.T) {
	rl := NewRateLimiter(WithDefaultRate(100, 100))

	// Accessing an unknown namespace should auto-create a bucket with defaults.
	if !rl.Allow("unknown_ns") {
		t.Fatal("first request to unknown namespace should be allowed")
	}

	stats := rl.Stats("unknown_ns")
	if stats.Allowed != 1 {
		t.Errorf("expected 1 allowed, got %d", stats.Allowed)
	}
}

func TestRateLimiter_AllStats(t *testing.T) {
	rl := NewRateLimiter(WithDefaultRate(10, 10))

	rl.Allow("a")
	rl.Allow("a")
	rl.Allow("b")

	all := rl.AllStats()
	if len(all) != 2 {
		t.Fatalf("expected 2 namespaces, got %d", len(all))
	}
	if all["a"].Allowed != 2 {
		t.Errorf("a: expected 2 allowed, got %d", all["a"].Allowed)
	}
	if all["b"].Allowed != 1 {
		t.Errorf("b: expected 1 allowed, got %d", all["b"].Allowed)
	}
}

func TestRateLimiter_SetNamespaceRate(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	rl := NewRateLimiter(
		WithClock(clock.Now),
		WithDefaultRate(100, 100),
	)

	// Create bucket by using it.
	rl.Allow("dynamic")

	// Shrink to burst=1.
	if err := rl.SetNamespaceRate("dynamic", 1, 1); err != nil {
		t.Fatal(err)
	}

	// Tokens should be capped at new burst. The bucket had ~99 tokens, now capped to 1.
	if !rl.Allow("dynamic") {
		t.Fatal("first request after rate change should be allowed")
	}
	if rl.Allow("dynamic") {
		t.Fatal("second request should be throttled with burst=1")
	}

	// Invalid parameters.
	if err := rl.SetNamespaceRate("x", 0, 5); err == nil {
		t.Error("expected error for zero rate")
	}
	if err := rl.SetNamespaceRate("x", 5, 0); err == nil {
		t.Error("expected error for zero burst")
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(WithDefaultRate(1000, 1000))

	var wg sync.WaitGroup
	const goroutines = 20
	const perGoroutine = 50

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				rl.Allow("concurrent")
			}
		}()
	}
	wg.Wait()

	stats := rl.Stats("concurrent")
	total := stats.Allowed + stats.Throttled
	if total != goroutines*perGoroutine {
		t.Errorf("expected %d total, got %d (allowed=%d throttled=%d)",
			goroutines*perGoroutine, total, stats.Allowed, stats.Throttled)
	}
}

func TestRateLimiter_StatsForUnknownNamespace(t *testing.T) {
	rl := NewRateLimiter()
	stats := rl.Stats("nonexistent")
	if stats.Allowed != 0 || stats.Throttled != 0 {
		t.Errorf("expected zero stats for unknown namespace, got %+v", stats)
	}
}
