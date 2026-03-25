package session

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestKillWithEscalation_GracefulExit(t *testing.T) {
	// "sleep 30" responds to SIGTERM by exiting.
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Simulate the runner goroutine: create a done channel and close it when
	// Wait() returns, just like runSession does.
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	// Process should exit on SIGTERM within the 5s timeout.
	start := time.Now()
	escalated := killWithEscalation(cmd, 5*time.Second, done)
	elapsed := time.Since(start)

	if escalated {
		t.Error("expected graceful exit (no SIGKILL), but escalation occurred")
	}
	if elapsed > 3*time.Second {
		t.Errorf("graceful exit took too long: %v", elapsed)
	}
}

func TestKillWithEscalation_ForcedKill(t *testing.T) {
	// Use a shell script that traps SIGTERM and ignores it, forcing SIGKILL.
	cmd := exec.Command("bash", "-c", "trap '' TERM; sleep 60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Simulate the runner goroutine.
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	// Give the trap a moment to install.
	time.Sleep(200 * time.Millisecond)

	// Use a short timeout so the test doesn't take forever.
	timeout := 1 * time.Second
	start := time.Now()
	escalated := killWithEscalation(cmd, timeout, done)
	elapsed := time.Since(start)

	if !escalated {
		t.Error("expected SIGKILL escalation, but process exited gracefully")
	}
	// Should have taken at least timeout, but not much longer.
	if elapsed < timeout {
		t.Errorf("escalation happened too fast: %v (expected >= %v)", elapsed, timeout)
	}
	if elapsed > timeout+2*time.Second {
		t.Errorf("escalation took too long: %v", elapsed)
	}
}

func TestKillWithEscalation_NilDoneChannel(t *testing.T) {
	// When done is nil, killWithEscalation spawns its own Wait() goroutine.
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	start := time.Now()
	escalated := killWithEscalation(cmd, 5*time.Second, nil)
	elapsed := time.Since(start)

	if escalated {
		t.Error("expected graceful exit (no SIGKILL), but escalation occurred")
	}
	if elapsed > 3*time.Second {
		t.Errorf("graceful exit took too long: %v", elapsed)
	}
}

func TestKillWithEscalation_NilCmd(t *testing.T) {
	if killWithEscalation(nil, time.Second, nil) {
		t.Error("expected false for nil cmd")
	}
}

func TestKillWithEscalation_NilProcess(t *testing.T) {
	cmd := &exec.Cmd{}
	if killWithEscalation(cmd, time.Second, nil) {
		t.Error("expected false for nil process")
	}
}
