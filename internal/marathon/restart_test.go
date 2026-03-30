package marathon

import (
	"testing"
	"time"
)

func TestRestartPolicy_Defaults(t *testing.T) {
	rp := NewRestartPolicy()
	stats := rp.Stats()
	if stats.MaxRestarts != 5 {
		t.Fatalf("expected default max restarts 5, got %d", stats.MaxRestarts)
	}
	if stats.RestartCount != 0 {
		t.Fatalf("expected 0 restart count, got %d", stats.RestartCount)
	}
}

func TestRestartPolicy_ShouldRestart_Basic(t *testing.T) {
	rp := NewRestartPolicy(WithMaxRestarts(3))

	// Exit code 0 should never restart.
	if rp.ShouldRestart(0, time.Minute) {
		t.Fatal("should not restart on exit code 0")
	}

	// Non-zero exit code should restart.
	if !rp.ShouldRestart(1, time.Minute) {
		t.Fatal("should restart on exit code 1")
	}
}

func TestRestartPolicy_MaxRestarts(t *testing.T) {
	rp := NewRestartPolicy(WithMaxRestarts(2))

	if !rp.ShouldRestart(1, time.Minute) {
		t.Fatal("should allow restart before max")
	}
	rp.RecordRestart()

	if !rp.ShouldRestart(1, time.Minute) {
		t.Fatal("should allow second restart")
	}
	rp.RecordRestart()

	if rp.ShouldRestart(1, time.Minute) {
		t.Fatal("should deny restart after reaching max")
	}
}

func TestRestartPolicy_ResetCount(t *testing.T) {
	rp := NewRestartPolicy(WithMaxRestarts(1))

	rp.RecordRestart()
	if rp.ShouldRestart(1, time.Minute) {
		t.Fatal("should be exhausted")
	}

	rp.ResetCount()
	if !rp.ShouldRestart(1, time.Minute) {
		t.Fatal("should allow restart after reset")
	}
}

func TestRestartPolicy_MinElapsed(t *testing.T) {
	rp := NewRestartPolicy(
		WithMaxRestarts(5),
		WithMinElapsed(10*time.Second),
	)

	// Too short — should not restart.
	if rp.ShouldRestart(1, 5*time.Second) {
		t.Fatal("should deny restart when elapsed < minElapsed")
	}

	// Long enough — should restart.
	if !rp.ShouldRestart(1, 15*time.Second) {
		t.Fatal("should allow restart when elapsed >= minElapsed")
	}
}

func TestRestartPolicy_ExitCodeFilter(t *testing.T) {
	rp := NewRestartPolicy(
		WithMaxRestarts(5),
		WithRestartableExitCodes(func(code int) bool {
			return code == 137 || code == 143
		}),
	)

	if rp.ShouldRestart(1, time.Minute) {
		t.Fatal("exit code 1 should not be restartable")
	}
	if !rp.ShouldRestart(137, time.Minute) {
		t.Fatal("exit code 137 should be restartable")
	}
	if !rp.ShouldRestart(143, time.Minute) {
		t.Fatal("exit code 143 should be restartable")
	}
}

func TestRestartPolicy_ExponentialBackoff(t *testing.T) {
	rp := NewRestartPolicy(
		WithMaxRestarts(10),
		WithBaseBackoff(100*time.Millisecond),
		WithBackoffFactor(2.0),
		WithMaxBackoff(5*time.Second),
	)

	// Before any restarts: base backoff.
	b0 := rp.Backoff()
	if b0 != 100*time.Millisecond {
		t.Fatalf("expected 100ms base backoff, got %s", b0)
	}

	rp.RecordRestart()
	b1 := rp.Backoff()
	if b1 != 100*time.Millisecond {
		t.Fatalf("after 1 restart, expected 100ms, got %s", b1)
	}

	rp.RecordRestart()
	b2 := rp.Backoff()
	if b2 != 200*time.Millisecond {
		t.Fatalf("after 2 restarts, expected 200ms, got %s", b2)
	}

	rp.RecordRestart()
	b3 := rp.Backoff()
	if b3 != 400*time.Millisecond {
		t.Fatalf("after 3 restarts, expected 400ms, got %s", b3)
	}
}

func TestRestartPolicy_BackoffCap(t *testing.T) {
	rp := NewRestartPolicy(
		WithMaxRestarts(100),
		WithBaseBackoff(1*time.Second),
		WithBackoffFactor(10.0),
		WithMaxBackoff(30*time.Second),
	)

	// Record many restarts to exceed the cap.
	for i := 0; i < 20; i++ {
		rp.RecordRestart()
	}

	b := rp.Backoff()
	if b > 30*time.Second {
		t.Fatalf("backoff %s exceeds max 30s", b)
	}
	if b != 30*time.Second {
		t.Fatalf("expected backoff capped at 30s, got %s", b)
	}
}

func TestRestartPolicy_Stats(t *testing.T) {
	rp := NewRestartPolicy(WithMaxRestarts(3), WithBaseBackoff(500*time.Millisecond))

	rp.RecordRestart()
	rp.RecordRestart()

	stats := rp.Stats()
	if stats.RestartCount != 2 {
		t.Fatalf("expected restart count 2, got %d", stats.RestartCount)
	}
	if stats.MaxRestarts != 3 {
		t.Fatalf("expected max restarts 3, got %d", stats.MaxRestarts)
	}
	if stats.LastRestart.IsZero() {
		t.Fatal("expected non-zero last restart time")
	}
	// After 2 restarts: base * factor^(2-1) = 500ms * 2 = 1s
	if stats.NextBackoff != 1*time.Second {
		t.Fatalf("expected next backoff 1s, got %s", stats.NextBackoff)
	}
}

func TestRestartPolicy_ConcurrentAccess(t *testing.T) {
	rp := NewRestartPolicy(WithMaxRestarts(1000))

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				rp.ShouldRestart(1, time.Minute)
				rp.RecordRestart()
				rp.Backoff()
				rp.Stats()
			}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	stats := rp.Stats()
	if stats.RestartCount != 1000 {
		t.Fatalf("expected 1000 restarts, got %d", stats.RestartCount)
	}
}
