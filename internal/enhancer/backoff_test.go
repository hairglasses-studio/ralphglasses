package enhancer

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockImprover is a test double for PromptImprover that records calls
// and returns configurable results per attempt.
type mockImprover struct {
	mu       sync.Mutex
	calls    int
	results  []*ImproveResult
	errors   []error
	provider ProviderName
}

func (m *mockImprover) Improve(_ context.Context, _ string, _ ImproveOptions) (*ImproveResult, error) {
	m.mu.Lock()
	i := m.calls
	m.calls++
	m.mu.Unlock()

	var result *ImproveResult
	var err error

	if i < len(m.results) {
		result = m.results[i]
	}
	if i < len(m.errors) {
		err = m.errors[i]
	}

	return result, err
}

func (m *mockImprover) Provider() ProviderName {
	if m.provider != "" {
		return m.provider
	}
	return ProviderClaude
}

func (m *mockImprover) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func TestDefaultBackoff(t *testing.T) {
	t.Parallel()
	cfg := DefaultBackoff()

	if cfg.BaseDelay != 500*time.Millisecond {
		t.Errorf("expected BaseDelay 500ms, got %v", cfg.BaseDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("expected MaxDelay 30s, got %v", cfg.MaxDelay)
	}
	if cfg.Factor != 2.0 {
		t.Errorf("expected Factor 2.0, got %v", cfg.Factor)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected MaxRetries 3, got %d", cfg.MaxRetries)
	}
}

func TestBackoffConfig_Delay(t *testing.T) {
	t.Parallel()
	cfg := DefaultBackoff()

	// Run many samples per attempt to verify statistical bounds.
	const samples = 1000
	for attempt := 0; attempt < 4; attempt++ {
		expectedCeiling := math.Min(
			float64(cfg.BaseDelay)*math.Pow(cfg.Factor, float64(attempt)),
			float64(cfg.MaxDelay),
		)

		var min, max time.Duration
		min = time.Duration(math.MaxInt64)

		for i := 0; i < samples; i++ {
			d := cfg.delay(attempt)
			if d < 0 {
				t.Fatalf("attempt %d: negative delay %v", attempt, d)
			}
			if d > time.Duration(expectedCeiling) {
				t.Fatalf("attempt %d: delay %v exceeds ceiling %v", attempt, d, time.Duration(expectedCeiling))
			}
			if d < min {
				min = d
			}
			if d > max {
				max = d
			}
		}

		// Full jitter should produce values near 0 and near the ceiling
		// over 1000 samples. Verify the range is at least 50% of ceiling.
		spread := float64(max - min)
		if spread < expectedCeiling*0.5 {
			t.Errorf("attempt %d: jitter spread %.0f is less than 50%% of ceiling %.0f",
				attempt, spread, expectedCeiling)
		}
	}
}

func TestBackoffConfig_DelayRespectsCeiling(t *testing.T) {
	t.Parallel()
	cfg := BackoffConfig{
		BaseDelay:  1 * time.Second,
		MaxDelay:   2 * time.Second,
		Factor:     10.0,
		MaxRetries: 5,
	}

	// At attempt 3, base would be 1s * 10^3 = 1000s, but ceiling is 2s.
	for i := 0; i < 100; i++ {
		d := cfg.delay(3)
		if d > 2*time.Second {
			t.Fatalf("delay %v exceeds MaxDelay 2s", d)
		}
	}
}

func TestBackoffConfig_ExponentialGrowth(t *testing.T) {
	t.Parallel()
	cfg := DefaultBackoff()

	// Verify that the expected ceiling grows exponentially:
	// attempt 0: 500ms, attempt 1: 1s, attempt 2: 2s, attempt 3: 4s
	expectations := []time.Duration{
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
	}

	for attempt, expected := range expectations {
		ceiling := math.Min(
			float64(cfg.BaseDelay)*math.Pow(cfg.Factor, float64(attempt)),
			float64(cfg.MaxDelay),
		)
		if time.Duration(ceiling) != expected {
			t.Errorf("attempt %d: expected ceiling %v, got %v", attempt, expected, time.Duration(ceiling))
		}
	}
}

func TestIsRetryableError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"nil error", nil, false},
		{"rate limit 429", fmt.Errorf("api error (status 429): rate limited"), true},
		{"rate_limit_error", fmt.Errorf("rate_limit_error: too many requests"), true},
		{"server error 500", fmt.Errorf("api error (status 500): internal"), true},
		{"server error 502", fmt.Errorf("api error (status 502): bad gateway"), true},
		{"server error 503", fmt.Errorf("api error (status 503): unavailable"), true},
		{"server error 504", fmt.Errorf("api error (status 504): timeout"), true},
		{"gemini RESOURCE_EXHAUSTED", fmt.Errorf("RESOURCE_EXHAUSTED: quota exceeded"), true},
		{"connection refused", fmt.Errorf("dial tcp: connection refused"), true},
		{"connection reset", fmt.Errorf("read tcp: connection reset by peer"), true},
		{"io timeout", fmt.Errorf("i/o timeout"), true},
		{"EOF", fmt.Errorf("unexpected EOF"), true},
		{"broken pipe", fmt.Errorf("write: broken pipe"), true},
		{"no such host", fmt.Errorf("dial tcp: lookup: no such host"), true},
		{"auth error 401", fmt.Errorf("api error (status 401): unauthorized"), false},
		{"forbidden 403", fmt.Errorf("api error (status 403): forbidden"), false},
		{"bad request 400", fmt.Errorf("api error (status 400): bad request"), false},
		{"context canceled", fmt.Errorf("api call: context canceled"), false},
		{"context deadline", fmt.Errorf("api call: context deadline exceeded"), false},
		{"unmarshal error", fmt.Errorf("unmarshal response: invalid JSON"), false},
		{"generic error", fmt.Errorf("something went wrong"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isRetryableError(tt.err)
			if got != tt.retryable {
				t.Errorf("isRetryableError(%v) = %v, want %v", tt.err, got, tt.retryable)
			}
		})
	}
}

func TestRetryImprove_SuccessOnFirstAttempt(t *testing.T) {
	t.Parallel()
	client := &mockImprover{
		results: []*ImproveResult{{Enhanced: "improved"}},
		errors:  []error{nil},
	}

	result, err := retryImprove(context.Background(), client, "test", ImproveOptions{}, DefaultBackoff())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Enhanced != "improved" {
		t.Errorf("expected 'improved', got %q", result.Enhanced)
	}
	if client.callCount() != 1 {
		t.Errorf("expected 1 call, got %d", client.callCount())
	}
}

func TestRetryImprove_SuccessAfterRetries(t *testing.T) {
	t.Parallel()

	// Override sleep to avoid real delays in tests.
	origSleep := sleepFunc
	sleepFunc = func(_ context.Context, _ time.Duration) error { return nil }
	t.Cleanup(func() { sleepFunc = origSleep })

	client := &mockImprover{
		results: []*ImproveResult{nil, nil, {Enhanced: "recovered"}},
		errors: []error{
			fmt.Errorf("api error (status 503): unavailable"),
			fmt.Errorf("api error (status 429): rate limited"),
			nil,
		},
	}

	result, err := retryImprove(context.Background(), client, "test", ImproveOptions{}, DefaultBackoff())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Enhanced != "recovered" {
		t.Errorf("expected 'recovered', got %q", result.Enhanced)
	}
	if client.callCount() != 3 {
		t.Errorf("expected 3 calls, got %d", client.callCount())
	}
}

func TestRetryImprove_NonRetryableErrorStopsImmediately(t *testing.T) {
	t.Parallel()

	origSleep := sleepFunc
	sleepFunc = func(_ context.Context, _ time.Duration) error { return nil }
	t.Cleanup(func() { sleepFunc = origSleep })

	client := &mockImprover{
		results: []*ImproveResult{nil},
		errors:  []error{fmt.Errorf("api error (status 401): unauthorized")},
	}

	_, err := retryImprove(context.Background(), client, "test", ImproveOptions{}, DefaultBackoff())
	if err == nil {
		t.Fatal("expected error")
	}
	if client.callCount() != 1 {
		t.Errorf("expected 1 call for non-retryable error, got %d", client.callCount())
	}
}

func TestRetryImprove_ExhaustsAllRetries(t *testing.T) {
	t.Parallel()

	origSleep := sleepFunc
	sleepFunc = func(_ context.Context, _ time.Duration) error { return nil }
	t.Cleanup(func() { sleepFunc = origSleep })

	client := &mockImprover{
		results: []*ImproveResult{nil, nil, nil, nil},
		errors: []error{
			fmt.Errorf("api error (status 500): error 1"),
			fmt.Errorf("api error (status 500): error 2"),
			fmt.Errorf("api error (status 500): error 3"),
			fmt.Errorf("api error (status 500): error 4"),
		},
	}

	_, err := retryImprove(context.Background(), client, "test", ImproveOptions{}, DefaultBackoff())
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	// 1 initial + 3 retries = 4 total
	if client.callCount() != 4 {
		t.Errorf("expected 4 calls (1 initial + 3 retries), got %d", client.callCount())
	}
	assertContains(t, err.Error(), "error 4")
}

func TestRetryImprove_ZeroRetriesNoRetry(t *testing.T) {
	t.Parallel()

	client := &mockImprover{
		results: []*ImproveResult{nil},
		errors:  []error{fmt.Errorf("api error (status 500): server error")},
	}

	cfg := DefaultBackoff()
	cfg.MaxRetries = 0

	_, err := retryImprove(context.Background(), client, "test", ImproveOptions{}, cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if client.callCount() != 1 {
		t.Errorf("expected 1 call with MaxRetries=0, got %d", client.callCount())
	}
}

func TestRetryImprove_ContextCancelledDuringSleep(t *testing.T) {
	t.Parallel()

	// Simulate context cancellation during sleep.
	origSleep := sleepFunc
	sleepFunc = func(ctx context.Context, _ time.Duration) error {
		return ctx.Err()
	}
	t.Cleanup(func() { sleepFunc = origSleep })

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client := &mockImprover{
		results: []*ImproveResult{nil, {Enhanced: "should not reach"}},
		errors:  []error{fmt.Errorf("api error (status 500): server error"), nil},
	}

	_, err := retryImprove(ctx, client, "test", ImproveOptions{}, DefaultBackoff())
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
	if client.callCount() != 1 {
		t.Errorf("expected 1 call before context cancellation stopped retries, got %d", client.callCount())
	}
}

func TestRetryImprove_BackoffTimingProgression(t *testing.T) {
	t.Parallel()

	// Record the delay durations passed to sleepFunc.
	var delays []time.Duration
	var delayMu sync.Mutex

	origSleep := sleepFunc
	sleepFunc = func(_ context.Context, d time.Duration) error {
		delayMu.Lock()
		delays = append(delays, d)
		delayMu.Unlock()
		return nil
	}
	t.Cleanup(func() { sleepFunc = origSleep })

	client := &mockImprover{
		results: []*ImproveResult{nil, nil, nil, {Enhanced: "ok"}},
		errors: []error{
			fmt.Errorf("api error (status 500): error"),
			fmt.Errorf("api error (status 500): error"),
			fmt.Errorf("api error (status 500): error"),
			nil,
		},
	}

	cfg := DefaultBackoff()
	result, err := retryImprove(context.Background(), client, "test", ImproveOptions{}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Enhanced != "ok" {
		t.Errorf("expected 'ok', got %q", result.Enhanced)
	}

	delayMu.Lock()
	defer delayMu.Unlock()

	// 3 retries = 3 sleep calls (before attempts 1, 2, 3).
	if len(delays) != 3 {
		t.Fatalf("expected 3 sleep calls, got %d", len(delays))
	}

	// Verify each delay is within [0, ceiling] for its attempt.
	ceilings := []time.Duration{
		500 * time.Millisecond, // attempt 0: 500ms * 2^0
		1 * time.Second,        // attempt 1: 500ms * 2^1
		2 * time.Second,        // attempt 2: 500ms * 2^2
	}

	for i, d := range delays {
		if d < 0 {
			t.Errorf("delay[%d] = %v, expected >= 0", i, d)
		}
		if d > ceilings[i] {
			t.Errorf("delay[%d] = %v, exceeds ceiling %v", i, d, ceilings[i])
		}
	}
}

func TestRetryImprove_BackoffCeilingsCapAtMaxDelay(t *testing.T) {
	t.Parallel()

	var delays []time.Duration
	var delayMu sync.Mutex

	origSleep := sleepFunc
	sleepFunc = func(_ context.Context, d time.Duration) error {
		delayMu.Lock()
		delays = append(delays, d)
		delayMu.Unlock()
		return nil
	}
	t.Cleanup(func() { sleepFunc = origSleep })

	cfg := BackoffConfig{
		BaseDelay:  1 * time.Second,
		MaxDelay:   3 * time.Second,
		Factor:     4.0,
		MaxRetries: 5,
	}

	client := &mockImprover{
		results: []*ImproveResult{nil, nil, nil, nil, nil, {Enhanced: "ok"}},
		errors: []error{
			fmt.Errorf("api error (status 500): e"),
			fmt.Errorf("api error (status 500): e"),
			fmt.Errorf("api error (status 500): e"),
			fmt.Errorf("api error (status 500): e"),
			fmt.Errorf("api error (status 500): e"),
			nil,
		},
	}

	_, err := retryImprove(context.Background(), client, "test", ImproveOptions{}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	delayMu.Lock()
	defer delayMu.Unlock()

	// With base=1s, factor=4: attempt 0=1s, attempt 1=4s (capped to 3s),
	// attempt 2=16s (capped to 3s), etc. All delays should be <= 3s.
	for i, d := range delays {
		if d > 3*time.Second {
			t.Errorf("delay[%d] = %v, exceeds MaxDelay 3s", i, d)
		}
	}
}

func TestRetryImprove_LastErrorReturned(t *testing.T) {
	t.Parallel()

	origSleep := sleepFunc
	sleepFunc = func(_ context.Context, _ time.Duration) error { return nil }
	t.Cleanup(func() { sleepFunc = origSleep })

	client := &mockImprover{
		results: []*ImproveResult{nil, nil, nil, nil},
		errors: []error{
			fmt.Errorf("api error (status 500): first"),
			fmt.Errorf("api error (status 500): second"),
			fmt.Errorf("api error (status 500): third"),
			fmt.Errorf("api error (status 500): last"),
		},
	}

	_, err := retryImprove(context.Background(), client, "test", ImproveOptions{}, DefaultBackoff())
	if err == nil {
		t.Fatal("expected error")
	}
	assertContains(t, err.Error(), "last")
}

func TestRetryImprove_Concurrent(t *testing.T) {
	t.Parallel()

	origSleep := sleepFunc
	sleepFunc = func(_ context.Context, _ time.Duration) error { return nil }
	t.Cleanup(func() { sleepFunc = origSleep })

	// Verify retryImprove is safe to call concurrently.
	var successCount atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &mockImprover{
				results: []*ImproveResult{nil, {Enhanced: "ok"}},
				errors: []error{
					fmt.Errorf("api error (status 503): unavailable"),
					nil,
				},
			}
			result, err := retryImprove(context.Background(), client, "test", ImproveOptions{}, DefaultBackoff())
			if err == nil && result.Enhanced == "ok" {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()
	if successCount.Load() != 10 {
		t.Errorf("expected 10 successful calls, got %d", successCount.Load())
	}
}
