package safety

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var errSynthetic = errors.New("synthetic failure")

// fakeClock returns a controllable now function and an advance helper.
func fakeClock(start time.Time) (nowFn func() time.Time, advance func(d time.Duration)) {
	var current atomic.Value
	current.Store(start)
	nowFn = func() time.Time { return current.Load().(time.Time) }
	advance = func(d time.Duration) { current.Store(current.Load().(time.Time).Add(d)) }
	return
}

func defaultCB(t *testing.T) *CircuitBreaker {
	t.Helper()
	cb, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New(DefaultConfig()) failed: %v", err)
	}
	return cb
}

// --- Config validation -------------------------------------------------------

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{name: "default is valid", cfg: DefaultConfig()},
		{name: "zero failure threshold", cfg: Config{FailureThreshold: 0, ResetTimeout: time.Second, SuccessThreshold: 1}, wantErr: true},
		{name: "negative reset timeout", cfg: Config{FailureThreshold: 1, ResetTimeout: -1, SuccessThreshold: 1}, wantErr: true},
		{name: "zero success threshold", cfg: Config{FailureThreshold: 1, ResetTimeout: time.Second, SuccessThreshold: 0}, wantErr: true},
		{name: "minimal valid", cfg: Config{FailureThreshold: 1, ResetTimeout: time.Millisecond, SuccessThreshold: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMustNew_PanicsOnBadConfig(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustNew to panic on bad config")
		}
	}()
	MustNew(Config{})
}

// --- State: closed -----------------------------------------------------------

func TestClosed_InitialState(t *testing.T) {
	cb := defaultCB(t)
	if cb.State() != StateClosed {
		t.Errorf("initial state = %v, want closed", cb.State())
	}
}

func TestClosed_ExecutePassesThrough(t *testing.T) {
	cb := defaultCB(t)
	called := false
	err := cb.Execute(func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Errorf("Execute returned error: %v", err)
	}
	if !called {
		t.Error("function was not called")
	}
}

func TestClosed_ExecuteReturnsWrappedError(t *testing.T) {
	cb := defaultCB(t)
	err := cb.Execute(func() error { return errSynthetic })
	if !errors.Is(err, errSynthetic) {
		t.Errorf("Execute returned %v, want %v", err, errSynthetic)
	}
}

func TestClosed_SuccessResetsFailureCount(t *testing.T) {
	cfg := Config{FailureThreshold: 3, ResetTimeout: time.Second, SuccessThreshold: 1}
	cb := MustNew(cfg)

	// 2 failures, then a success
	cb.Execute(func() error { return errSynthetic })
	cb.Execute(func() error { return errSynthetic })
	cb.Execute(func() error { return nil })

	// 2 more failures should not trip (we need 3 consecutive)
	cb.Execute(func() error { return errSynthetic })
	cb.Execute(func() error { return errSynthetic })

	if cb.State() != StateClosed {
		t.Errorf("state = %v, want closed (success should have reset counter)", cb.State())
	}
}

// --- Transition: closed -> open ----------------------------------------------

func TestClosed_TripsAfterFailureThreshold(t *testing.T) {
	cfg := Config{FailureThreshold: 3, ResetTimeout: time.Minute, SuccessThreshold: 1}
	cb := MustNew(cfg)

	for i := 0; i < 3; i++ {
		cb.Execute(func() error { return errSynthetic })
	}

	if cb.State() != StateOpen {
		t.Errorf("state = %v after %d failures, want open", cb.State(), cfg.FailureThreshold)
	}
}

func TestClosed_DoesNotTripBelowThreshold(t *testing.T) {
	cfg := Config{FailureThreshold: 5, ResetTimeout: time.Minute, SuccessThreshold: 1}
	cb := MustNew(cfg)

	for i := 0; i < 4; i++ {
		cb.Execute(func() error { return errSynthetic })
	}

	if cb.State() != StateClosed {
		t.Errorf("state = %v after 4 failures (threshold=5), want closed", cb.State())
	}
}

// --- State: open -------------------------------------------------------------

func TestOpen_RejectsCalls(t *testing.T) {
	cfg := Config{FailureThreshold: 1, ResetTimeout: time.Hour, SuccessThreshold: 1}
	cb := MustNew(cfg)

	cb.Execute(func() error { return errSynthetic }) // trip

	called := false
	err := cb.Execute(func() error {
		called = true
		return nil
	})

	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("Execute in open state returned %v, want ErrCircuitOpen", err)
	}
	if called {
		t.Error("function was called in open state")
	}
}

// --- Transition: open -> half-open -------------------------------------------

func TestOpen_TransitionsToHalfOpenAfterTimeout(t *testing.T) {
	start := time.Now()
	nowFn, advance := fakeClock(start)

	cfg := Config{FailureThreshold: 1, ResetTimeout: 5 * time.Second, SuccessThreshold: 1}
	cb := MustNew(cfg)
	cb.now = nowFn

	cb.Execute(func() error { return errSynthetic }) // trip at t=0

	if cb.State() != StateOpen {
		t.Fatalf("expected open, got %v", cb.State())
	}

	advance(6 * time.Second) // past reset timeout

	if cb.State() != StateHalfOpen {
		t.Errorf("state = %v after timeout, want half-open", cb.State())
	}
}

// --- State: half-open --------------------------------------------------------

func TestHalfOpen_AllowsCalls(t *testing.T) {
	start := time.Now()
	nowFn, advance := fakeClock(start)

	cfg := Config{FailureThreshold: 1, ResetTimeout: 5 * time.Second, SuccessThreshold: 2}
	cb := MustNew(cfg)
	cb.now = nowFn

	cb.Execute(func() error { return errSynthetic }) // trip
	advance(6 * time.Second)

	called := false
	err := cb.Execute(func() error {
		called = true
		return nil
	})

	if err != nil {
		t.Errorf("Execute in half-open returned %v", err)
	}
	if !called {
		t.Error("function was not called in half-open")
	}
}

// --- Transition: half-open -> closed -----------------------------------------

func TestHalfOpen_ClosesAfterSuccessThreshold(t *testing.T) {
	start := time.Now()
	nowFn, advance := fakeClock(start)

	cfg := Config{FailureThreshold: 1, ResetTimeout: 5 * time.Second, SuccessThreshold: 3}
	cb := MustNew(cfg)
	cb.now = nowFn

	cb.Execute(func() error { return errSynthetic }) // trip
	advance(6 * time.Second)

	// Need 3 consecutive successes
	for i := 0; i < 3; i++ {
		if err := cb.Execute(func() error { return nil }); err != nil {
			t.Fatalf("success %d: unexpected error: %v", i+1, err)
		}
	}

	if cb.State() != StateClosed {
		t.Errorf("state = %v after %d successes, want closed", cb.State(), cfg.SuccessThreshold)
	}
}

func TestHalfOpen_PartialSuccessNotEnough(t *testing.T) {
	start := time.Now()
	nowFn, advance := fakeClock(start)

	cfg := Config{FailureThreshold: 1, ResetTimeout: 5 * time.Second, SuccessThreshold: 3}
	cb := MustNew(cfg)
	cb.now = nowFn

	cb.Execute(func() error { return errSynthetic }) // trip
	advance(6 * time.Second)

	// Only 2 successes (need 3)
	cb.Execute(func() error { return nil })
	cb.Execute(func() error { return nil })

	if cb.State() != StateHalfOpen {
		t.Errorf("state = %v after 2 of 3 needed successes, want half-open", cb.State())
	}
}

// --- Transition: half-open -> open -------------------------------------------

func TestHalfOpen_ReopensOnFailure(t *testing.T) {
	start := time.Now()
	nowFn, advance := fakeClock(start)

	cfg := Config{FailureThreshold: 1, ResetTimeout: 5 * time.Second, SuccessThreshold: 3}
	cb := MustNew(cfg)
	cb.now = nowFn

	cb.Execute(func() error { return errSynthetic }) // trip
	advance(6 * time.Second)

	// One success, then a failure
	cb.Execute(func() error { return nil })
	cb.Execute(func() error { return errSynthetic })

	if cb.State() != StateOpen {
		t.Errorf("state = %v after half-open failure, want open", cb.State())
	}
}

// --- Reset -------------------------------------------------------------------

func TestReset_ClosesFromOpen(t *testing.T) {
	cfg := Config{FailureThreshold: 1, ResetTimeout: time.Hour, SuccessThreshold: 1}
	cb := MustNew(cfg)

	cb.Execute(func() error { return errSynthetic }) // trip
	if cb.State() != StateOpen {
		t.Fatalf("precondition: expected open, got %v", cb.State())
	}

	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("state after Reset = %v, want closed", cb.State())
	}

	// Should be able to execute again
	err := cb.Execute(func() error { return nil })
	if err != nil {
		t.Errorf("Execute after Reset returned %v", err)
	}
}

func TestReset_ClearsFailureCount(t *testing.T) {
	cfg := Config{FailureThreshold: 3, ResetTimeout: time.Hour, SuccessThreshold: 1}
	cb := MustNew(cfg)

	cb.Execute(func() error { return errSynthetic })
	cb.Execute(func() error { return errSynthetic })
	cb.Reset()

	// 2 more failures should not trip (counter was cleared)
	cb.Execute(func() error { return errSynthetic })
	cb.Execute(func() error { return errSynthetic })

	if cb.State() != StateClosed {
		t.Errorf("state = %v after Reset + 2 failures (threshold=3), want closed", cb.State())
	}
}

func TestReset_FromClosedIsNoop(t *testing.T) {
	cb := defaultCB(t)
	before := cb.Snapshot().Transitions
	cb.Reset()
	after := cb.Snapshot().Transitions

	if after != before {
		t.Errorf("Reset from closed should not count as transition, got %d -> %d", before, after)
	}
}

// --- Metrics -----------------------------------------------------------------

func TestMetrics_TotalCalls(t *testing.T) {
	cfg := Config{FailureThreshold: 100, ResetTimeout: time.Minute, SuccessThreshold: 1}
	cb := MustNew(cfg)

	for i := 0; i < 10; i++ {
		cb.Execute(func() error { return nil })
	}

	snap := cb.Snapshot()
	if snap.TotalCalls != 10 {
		t.Errorf("TotalCalls = %d, want 10", snap.TotalCalls)
	}
	if snap.Successes != 10 {
		t.Errorf("Successes = %d, want 10", snap.Successes)
	}
}

func TestMetrics_FailuresAndRejections(t *testing.T) {
	cfg := Config{FailureThreshold: 2, ResetTimeout: time.Hour, SuccessThreshold: 1}
	cb := MustNew(cfg)

	cb.Execute(func() error { return errSynthetic })
	cb.Execute(func() error { return errSynthetic }) // trips

	// These should be rejected
	cb.Execute(func() error { return nil })
	cb.Execute(func() error { return nil })

	snap := cb.Snapshot()
	if snap.Failures != 2 {
		t.Errorf("Failures = %d, want 2", snap.Failures)
	}
	if snap.Rejections != 2 {
		t.Errorf("Rejections = %d, want 2", snap.Rejections)
	}
	if snap.TotalCalls != 4 {
		t.Errorf("TotalCalls = %d, want 4", snap.TotalCalls)
	}
}

func TestMetrics_Transitions(t *testing.T) {
	start := time.Now()
	nowFn, advance := fakeClock(start)

	cfg := Config{FailureThreshold: 1, ResetTimeout: 5 * time.Second, SuccessThreshold: 1}
	cb := MustNew(cfg)
	cb.now = nowFn

	// closed -> open (1)
	cb.Execute(func() error { return errSynthetic })

	advance(6 * time.Second)

	// open -> half-open (2), then half-open -> closed (3) via success
	cb.Execute(func() error { return nil })

	snap := cb.Snapshot()
	if snap.Transitions != 3 {
		t.Errorf("Transitions = %d, want 3 (closed->open, open->half-open, half-open->closed)", snap.Transitions)
	}
}

// --- Full lifecycle ----------------------------------------------------------

func TestFullLifecycle(t *testing.T) {
	start := time.Now()
	nowFn, advance := fakeClock(start)

	cfg := Config{FailureThreshold: 2, ResetTimeout: 10 * time.Second, SuccessThreshold: 2}
	cb := MustNew(cfg)
	cb.now = nowFn

	// Phase 1: normal operation
	cb.Execute(func() error { return nil })
	if cb.State() != StateClosed {
		t.Fatalf("phase 1: state = %v, want closed", cb.State())
	}

	// Phase 2: failures trip the breaker
	cb.Execute(func() error { return errSynthetic })
	cb.Execute(func() error { return errSynthetic })
	if cb.State() != StateOpen {
		t.Fatalf("phase 2: state = %v, want open", cb.State())
	}

	// Phase 3: calls are rejected while open
	err := cb.Execute(func() error { return nil })
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("phase 3: expected ErrCircuitOpen, got %v", err)
	}

	// Phase 4: timeout elapses, transitions to half-open
	advance(11 * time.Second)
	if cb.State() != StateHalfOpen {
		t.Fatalf("phase 4: state = %v, want half-open", cb.State())
	}

	// Phase 5: first success in half-open (need 2 total)
	cb.Execute(func() error { return nil })
	if cb.State() != StateHalfOpen {
		t.Fatalf("phase 5: state = %v after 1 success, want half-open", cb.State())
	}

	// Phase 6: second success closes the breaker
	cb.Execute(func() error { return nil })
	if cb.State() != StateClosed {
		t.Fatalf("phase 6: state = %v, want closed", cb.State())
	}

	// Phase 7: normal operation restored
	err = cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("phase 7: unexpected error: %v", err)
	}

	snap := cb.Snapshot()
	if snap.TotalCalls != 7 {
		t.Errorf("TotalCalls = %d, want 7", snap.TotalCalls)
	}
}

// --- Concurrent access -------------------------------------------------------

func TestConcurrent_NoRace(t *testing.T) {
	cfg := Config{FailureThreshold: 50, ResetTimeout: time.Millisecond, SuccessThreshold: 5}
	cb := MustNew(cfg)

	var wg sync.WaitGroup
	const goroutines = 20
	const callsPerGoroutine = 100

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < callsPerGoroutine; j++ {
				if j%3 == 0 {
					cb.Execute(func() error { return errSynthetic })
				} else {
					cb.Execute(func() error { return nil })
				}
				_ = cb.State()
				_ = cb.Snapshot()
			}
		}(i)
	}
	wg.Wait()

	snap := cb.Snapshot()
	if snap.TotalCalls != goroutines*callsPerGoroutine {
		t.Errorf("TotalCalls = %d, want %d", snap.TotalCalls, goroutines*callsPerGoroutine)
	}
}

func TestConcurrent_ResetDuringExecution(t *testing.T) {
	cfg := Config{FailureThreshold: 2, ResetTimeout: time.Millisecond, SuccessThreshold: 1}
	cb := MustNew(cfg)

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: continuously execute
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			cb.Execute(func() error {
				if i%2 == 0 {
					return errSynthetic
				}
				return nil
			})
		}
	}()

	// Goroutine 2: continuously reset
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			cb.Reset()
		}
	}()

	wg.Wait()
	// Test passes if no race detected (-race flag).
}

// --- State.String() ----------------------------------------------------------

func TestState_String(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown(99)"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

// --- Edge cases --------------------------------------------------------------

func TestEdge_ThresholdOfOne(t *testing.T) {
	start := time.Now()
	nowFn, advance := fakeClock(start)

	cfg := Config{FailureThreshold: 1, ResetTimeout: time.Second, SuccessThreshold: 1}
	cb := MustNew(cfg)
	cb.now = nowFn

	// Single failure trips
	cb.Execute(func() error { return errSynthetic })
	if cb.State() != StateOpen {
		t.Fatalf("state = %v, want open after 1 failure (threshold=1)", cb.State())
	}

	advance(2 * time.Second)

	// Single success recovers
	cb.Execute(func() error { return nil })
	if cb.State() != StateClosed {
		t.Errorf("state = %v, want closed after 1 success (threshold=1)", cb.State())
	}
}

func TestEdge_MultipleResets(t *testing.T) {
	cb := defaultCB(t)
	// Multiple resets from closed should be safe no-ops.
	for i := 0; i < 10; i++ {
		cb.Reset()
	}
	if cb.State() != StateClosed {
		t.Errorf("state = %v after multiple resets, want closed", cb.State())
	}
}

func TestEdge_ExecuteNilError(t *testing.T) {
	cb := defaultCB(t)
	err := cb.Execute(func() error { return nil })
	if err != nil {
		t.Errorf("Execute(nil-returning func) = %v, want nil", err)
	}
}
