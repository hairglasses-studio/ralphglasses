// internal/process/manager_getpgid_test.go
package process

import (
	"bytes"
	"errors"
	"log"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestSendSignal_GetpgidFailure_LogsWarningAndFallsToPID(t *testing.T) {
	// Swap getpgid to simulate failure.
	origGetpgid := getpgid
	t.Cleanup(func() { getpgid = origGetpgid })
	getpgid = func(pid int) (int, error) {
		return 0, errors.New("injected getpgid failure")
	}

	// Capture log output.
	var buf bytes.Buffer
	origOutput := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(origOutput) })

	// Send signal 0 (no-op probe) to our own PID — safe and always succeeds.
	pid := os.Getpid()
	err := sendSignal(pid, syscall.Signal(0))
	if err != nil {
		t.Fatalf("sendSignal returned unexpected error: %v", err)
	}

	// Assert warning log was emitted.
	logOutput := buf.String()
	if !strings.Contains(logOutput, "WARNING") {
		t.Errorf("expected WARNING in log output, got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "Getpgid") {
		t.Errorf("expected 'Getpgid' in log output, got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "falling back to direct PID signal") {
		t.Errorf("expected fallback message in log output, got: %q", logOutput)
	}
}
