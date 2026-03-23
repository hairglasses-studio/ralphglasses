package session

import (
	"testing"
	"time"
)

func TestRateLimiterAllowUnderLimit(t *testing.T) {
	r := NewRateLimiter()
	r.SetLimit(ProviderClaude, 5)

	for i := 0; i < 5; i++ {
		if err := r.Allow(ProviderClaude); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
	}
}

func TestRateLimiterBlocksAtLimit(t *testing.T) {
	r := NewRateLimiter()
	r.SetLimit(ProviderClaude, 3)

	for i := 0; i < 3; i++ {
		_ = r.Allow(ProviderClaude)
	}
	if err := r.Allow(ProviderClaude); err == nil {
		t.Error("expected rate limit error on 4th call")
	}
}

func TestRateLimiterZeroLimitMeansUnlimited(t *testing.T) {
	r := NewRateLimiter()
	r.SetLimit(ProviderGemini, 0)

	for i := 0; i < 100; i++ {
		if err := r.Allow(ProviderGemini); err != nil {
			t.Fatalf("unlimited provider blocked at call %d: %v", i+1, err)
		}
	}
}

func TestRateLimiterRemaining(t *testing.T) {
	r := NewRateLimiter()
	r.SetLimit(ProviderCodex, 10)

	if rem := r.Remaining(ProviderCodex); rem != 10 {
		t.Errorf("initial remaining = %d, want 10", rem)
	}

	_ = r.Allow(ProviderCodex)
	_ = r.Allow(ProviderCodex)

	if rem := r.Remaining(ProviderCodex); rem != 8 {
		t.Errorf("remaining after 2 calls = %d, want 8", rem)
	}
}

func TestRateLimiterRemainingUnlimited(t *testing.T) {
	r := NewRateLimiter()
	r.SetLimit(ProviderGemini, 0)
	if rem := r.Remaining(ProviderGemini); rem != -1 {
		t.Errorf("unlimited remaining = %d, want -1", rem)
	}
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	r := &RateLimiter{
		limits: map[Provider]int{ProviderClaude: 2},
		calls:  map[Provider][]time.Time{},
	}

	// Inject two calls that are 70 seconds old (outside the 1-minute window).
	old := time.Now().Add(-70 * time.Second)
	r.calls[ProviderClaude] = []time.Time{old, old}

	// Both old calls should be evicted; allow should succeed.
	if err := r.Allow(ProviderClaude); err != nil {
		t.Fatalf("expected allow after window expiry, got: %v", err)
	}
}
