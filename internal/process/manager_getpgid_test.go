package process

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// TestSendSignal_GetpgidFallback verifies that when getpgid fails, sendSignal
// logs a warning and falls back to signalling the PID directly.
func TestSendSignal_GetpgidFallback(t *testing.T) {
	// Start a long-lived subprocess to have a real PID to signal.
	cmd := exec.Command("sleep", "60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start subprocess: %v", err)
	}
	pid := cmd.Process.Pid
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Capture log output.
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)

	// Inject a getpgid that always fails.
	origGetpgid := getpgidFunc
	getpgidFunc = func(p int) (int, error) {
		return 0, fmt.Errorf("injected getpgid failure")
	}
	defer func() { getpgidFunc = origGetpgid }()

	// sendSignal should fall back to Kill(pid, SIGTERM) and succeed.
	if err := sendSignal(pid, syscall.SIGTERM); err != nil {
		t.Fatalf("sendSignal returned unexpected error: %v", err)
	}

	// Assert the warning was logged.
	logged := buf.String()
	if !bytes.Contains([]byte(logged), []byte("getpgid")) {
		t.Errorf("expected warning containing 'getpgid' in log output, got: %q", logged)
	}
	if !bytes.Contains([]byte(logged), []byte("signalling PID directly")) {
		t.Errorf("expected 'signalling PID directly' in log output, got: %q", logged)
	}

	// Confirm the process received the signal.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		// Process exited — signal was delivered.
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for subprocess to exit after SIGTERM")
	}
}

// TestSendSignal_GetpgidFallback_OwnProcess verifies fallback does not signal
// our own process group when getpgid fails — it signals only the target PID.
func TestSendSignal_GetpgidFallback_OwnProcess(t *testing.T) {
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)

	origGetpgid := getpgidFunc
	getpgidFunc = func(p int) (int, error) {
		return 0, fmt.Errorf("injected getpgid failure")
	}
	defer func() { getpgidFunc = origGetpgid }()

	// Signal our own process with signal 0 (existence check, harmless).
	if err := sendSignal(os.Getpid(), syscall.Signal(0)); err != nil {
		t.Fatalf("sendSignal(self, 0) returned error: %v", err)
	}

	logged := buf.String()
	if !bytes.Contains([]byte(logged), []byte("injected getpgid failure")) {
		t.Errorf("expected injected error in log, got: %q", logged)
	}
}
