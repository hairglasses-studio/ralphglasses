package gateway

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(10.0, 5)
	if rl == nil {
		t.Fatal("NewRateLimiter returned nil")
	}
	if rl.rate != 10.0 {
		t.Errorf("rate = %f, want 10.0", rl.rate)
	}
	if rl.burst != 5 {
		t.Errorf("burst = %d, want 5", rl.burst)
	}
	if rl.buckets == nil {
		t.Fatal("buckets map not initialized")
	}
}

func TestRateLimiter_Allow_BurstThenDeny(t *testing.T) {
	rl := NewRateLimiter(1.0, 3) // 1/s, burst 3

	for i := 0; i < 3; i++ {
		if !rl.Allow("k") {
			t.Fatalf("request %d should be allowed within burst", i+1)
		}
	}
	if rl.Allow("k") {
		t.Fatal("request beyond burst should be denied")
	}
}

func TestRateLimiter_Allow_PerKeyIsolation(t *testing.T) {
	rl := NewRateLimiter(1.0, 1) // 1/s, burst 1

	if !rl.Allow("alice") {
		t.Fatal("alice first request should be allowed")
	}
	if rl.Allow("alice") {
		t.Fatal("alice second request should be denied")
	}

	// bob is independent.
	if !rl.Allow("bob") {
		t.Fatal("bob first request should be allowed")
	}
	if rl.Allow("bob") {
		t.Fatal("bob second request should be denied")
	}
}

func TestRateLimiter_Allow_Refill(t *testing.T) {
	rl := NewRateLimiter(100.0, 1) // 100/s, burst 1

	if !rl.Allow("k") {
		t.Fatal("first request should be allowed")
	}
	if rl.Allow("k") {
		t.Fatal("immediate second should be denied")
	}

	time.Sleep(50 * time.Millisecond) // 50ms at 100/s = ~5 tokens

	if !rl.Allow("k") {
		t.Fatal("request after refill should be allowed")
	}
}

func TestRateLimiter_Allow_BurstCap(t *testing.T) {
	rl := NewRateLimiter(1000.0, 2) // 1000/s, burst 2

	// Exhaust burst.
	rl.Allow("k")
	rl.Allow("k")

	// Wait long enough to overfill if no cap.
	time.Sleep(10 * time.Millisecond) // would be ~10 tokens without cap

	// Should allow burst (2) then deny.
	if !rl.Allow("k") {
		t.Fatal("first after refill should be allowed")
	}
	if !rl.Allow("k") {
		t.Fatal("second after refill should be allowed (burst=2)")
	}
	if rl.Allow("k") {
		t.Fatal("third should be denied (burst cap)")
	}
}

func TestRateLimiter_Allow_ZeroBurst(t *testing.T) {
	rl := NewRateLimiter(100.0, 0)

	// With burst=0, no requests should ever be allowed.
	if rl.Allow("k") {
		t.Fatal("burst=0 should deny all requests")
	}
}

func TestRateLimiter_Allow_ConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(10000.0, 100)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", n%5)
			rl.Allow(key)
		}(i)
	}
	wg.Wait()
	// No race condition = pass.
}

func TestRateLimiter_Allow_ManyKeys(t *testing.T) {
	rl := NewRateLimiter(10.0, 1)

	// Each unique key gets its own burst.
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("client-%d", i)
		if !rl.Allow(key) {
			t.Fatalf("first request for %s should be allowed", key)
		}
	}
}

func TestRateLimiter_Allow_HighRate(t *testing.T) {
	rl := NewRateLimiter(10.0, 1) // 10/s, burst 1

	if !rl.Allow("k") {
		t.Fatal("first should be allowed")
	}
	// At 10/s with burst 1, a second call within 100ms should be denied.
	if rl.Allow("k") {
		t.Fatal("immediate second should be denied")
	}
}
