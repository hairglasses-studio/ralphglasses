package supervisor

import (
	"os/exec"
	"sort"
	"testing"
	"time"
)

func TestWatchCleanExit(t *testing.T) {
	s := New(Config{
		MaxRestarts:   3,
		RestartDelay:  10 * time.Millisecond,
		BackoffFactor: 1.0,
	})

	cmd := exec.Command("echo", "hello")
	if err := s.Watch("echo-test", cmd); err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Give the process time to complete.
	time.Sleep(200 * time.Millisecond)

	// Clean exit should not trigger a restart.
	count := s.RestartCount("echo-test")
	if count != 0 {
		t.Errorf("expected 0 restarts for clean exit, got %d", count)
	}
}

func TestWatchCrashAutoRestart(t *testing.T) {
	s := New(Config{
		MaxRestarts:   3,
		RestartDelay:  10 * time.Millisecond,
		BackoffFactor: 1.5,
	})

	// "false" exits with status 1 on all platforms with a /bin/false or
	// built-in. Use sh -c "exit 1" for portability.
	cmd := exec.Command("sh", "-c", "exit 1")
	if err := s.Watch("crasher", cmd); err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Wait long enough for all restarts to be attempted.
	// Delays: 10ms + 15ms + 22ms = ~47ms, plus process overhead.
	time.Sleep(500 * time.Millisecond)

	count := s.RestartCount("crasher")
	if count != 3 {
		t.Errorf("expected 3 restarts (MaxRestarts), got %d", count)
	}
}

func TestMaxRestartsLimit(t *testing.T) {
	maxRestarts := 2
	s := New(Config{
		MaxRestarts:   maxRestarts,
		RestartDelay:  10 * time.Millisecond,
		BackoffFactor: 1.0,
	})

	cmd := exec.Command("sh", "-c", "exit 1")
	if err := s.Watch("limited", cmd); err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Wait for all restarts to exhaust.
	time.Sleep(300 * time.Millisecond)

	count := s.RestartCount("limited")
	if count != maxRestarts {
		t.Errorf("expected exactly %d restarts, got %d", maxRestarts, count)
	}
}

func TestStopSupervisedProcess(t *testing.T) {
	s := New(Config{
		MaxRestarts:   10,
		RestartDelay:  50 * time.Millisecond,
		BackoffFactor: 1.0,
	})

	// sleep 60 runs long enough that we can stop it.
	cmd := exec.Command("sleep", "60")
	if err := s.Watch("sleeper", cmd); err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Verify it's running.
	running := s.Running()
	if len(running) != 1 || running[0] != "sleeper" {
		t.Fatalf("expected [sleeper] in Running(), got %v", running)
	}

	// Stop it.
	if err := s.Stop("sleeper"); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Should no longer be tracked.
	running = s.Running()
	if len(running) != 0 {
		t.Errorf("expected empty Running() after Stop, got %v", running)
	}

	// Restart count should be 0 (killed, not crashed).
	count := s.RestartCount("sleeper")
	if count != 0 {
		t.Errorf("expected 0 restarts after Stop, got %d", count)
	}
}

func TestStopPreventsRestart(t *testing.T) {
	s := New(Config{
		MaxRestarts:   10,
		RestartDelay:  100 * time.Millisecond,
		BackoffFactor: 1.0,
	})

	// Process exits immediately with error. RestartDelay is 100ms, so we
	// have time to call Stop before the first restart.
	cmd := exec.Command("sh", "-c", "exit 1")
	if err := s.Watch("fast-crash", cmd); err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Let the crash happen, then stop before the backoff delay finishes
	// or at most after 1 restart.
	time.Sleep(50 * time.Millisecond)
	if err := s.Stop("fast-crash"); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// After Stop, no more restarts should occur.
	time.Sleep(300 * time.Millisecond)
	running := s.Running()
	if len(running) != 0 {
		t.Errorf("expected empty Running() after Stop, got %v", running)
	}
}

func TestRunningMultipleProcesses(t *testing.T) {
	s := New(Config{
		MaxRestarts:   3,
		RestartDelay:  10 * time.Millisecond,
		BackoffFactor: 1.0,
	})

	cmd1 := exec.Command("sleep", "60")
	cmd2 := exec.Command("sleep", "60")

	if err := s.Watch("proc-a", cmd1); err != nil {
		t.Fatalf("Watch proc-a failed: %v", err)
	}
	if err := s.Watch("proc-b", cmd2); err != nil {
		t.Fatalf("Watch proc-b failed: %v", err)
	}

	running := s.Running()
	sort.Strings(running)
	if len(running) != 2 || running[0] != "proc-a" || running[1] != "proc-b" {
		t.Errorf("expected [proc-a, proc-b], got %v", running)
	}

	// Cleanup.
	_ = s.Stop("proc-a")
	_ = s.Stop("proc-b")
}

func TestWatchDuplicateName(t *testing.T) {
	s := New(Config{
		MaxRestarts:   1,
		RestartDelay:  10 * time.Millisecond,
		BackoffFactor: 1.0,
	})

	cmd1 := exec.Command("sleep", "60")
	if err := s.Watch("dup", cmd1); err != nil {
		t.Fatalf("first Watch failed: %v", err)
	}
	defer s.Stop("dup")

	cmd2 := exec.Command("sleep", "60")
	err := s.Watch("dup", cmd2)
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
}

func TestStopNotFound(t *testing.T) {
	s := New(DefaultConfig())
	err := s.Stop("nonexistent")
	if err == nil {
		t.Fatal("expected error for Stop on unknown name, got nil")
	}
}

func TestBackoffIncrease(t *testing.T) {
	s := New(Config{
		MaxRestarts:   2,
		RestartDelay:  50 * time.Millisecond,
		BackoffFactor: 2.0,
	})

	// With backoff factor 2.0 and initial delay 50ms:
	// restart 1 waits 50ms, restart 2 waits 100ms.
	// Total minimum: ~150ms of delay + process start overhead.
	start := time.Now()
	cmd := exec.Command("sh", "-c", "exit 1")
	if err := s.Watch("backoff-test", cmd); err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Wait for all restarts to complete.
	time.Sleep(600 * time.Millisecond)
	elapsed := time.Since(start)

	count := s.RestartCount("backoff-test")
	if count != 2 {
		t.Errorf("expected 2 restarts, got %d", count)
	}

	// Total delay should be at least 150ms (50ms + 100ms).
	if elapsed < 140*time.Millisecond {
		t.Errorf("expected at least ~150ms for backoff, got %v", elapsed)
	}
}

func TestZeroMaxRestarts(t *testing.T) {
	s := New(Config{
		MaxRestarts:   0,
		RestartDelay:  10 * time.Millisecond,
		BackoffFactor: 1.0,
	})

	cmd := exec.Command("sh", "-c", "exit 1")
	if err := s.Watch("no-restart", cmd); err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	count := s.RestartCount("no-restart")
	if count != 0 {
		t.Errorf("expected 0 restarts with MaxRestarts=0, got %d", count)
	}
}
