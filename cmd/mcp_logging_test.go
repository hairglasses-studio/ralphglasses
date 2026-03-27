package cmd

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/process"
)

// TestMCPLogging_FileOnly verifies that in MCP mode, slog output goes
// exclusively to the log file and never to stderr.
func TestMCPLogging_FileOnly(t *testing.T) {
	// Save and restore the default logger.
	origLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	tmpDir := t.TempDir()

	// --- Reproduce the MCP startup sequence from cmd/mcp.go ---

	// Step 1: Immediately silence slog (discard handler).
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))

	// Step 2: Create the log directory and file.
	logDir := process.LogDirPath(tmpDir)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logFile, err := os.OpenFile(process.LogFilePath(tmpDir), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()

	// Step 3: Set file-based handler (same as mcp.go line).
	slog.SetDefault(slog.New(newLogHandler(logFile)))

	// --- Capture stderr while emitting a log line ---
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	slog.Info("mcp-test-sentinel", "key", "value")

	// Close the write end and read whatever was captured.
	w.Close()
	var stderrBuf bytes.Buffer
	io.Copy(&stderrBuf, r)
	r.Close()
	os.Stderr = origStderr

	// --- Verify nothing reached stderr ---
	if stderrBuf.Len() > 0 {
		t.Errorf("slog wrote to stderr in MCP mode: %s", stderrBuf.String())
	}

	// --- Verify the log line is in the file ---
	logFile.Sync()
	data, err := os.ReadFile(process.LogFilePath(tmpDir))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "mcp-test-sentinel") {
		t.Errorf("log file missing expected entry; got: %s", string(data))
	}
}

// TestMCPLogging_DiscardBeforeFile verifies that slog calls between the
// discard handler and the file handler do not reach stderr.
func TestMCPLogging_DiscardBeforeFile(t *testing.T) {
	origLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	// Set discard handler (simulates the first thing MCP RunE does).
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))

	// Capture stderr.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	// Log while the discard handler is active (before file is opened).
	slog.Warn("should-be-discarded")

	w.Close()
	var stderrBuf bytes.Buffer
	io.Copy(&stderrBuf, r)
	r.Close()
	os.Stderr = origStderr

	if stderrBuf.Len() > 0 {
		t.Errorf("slog wrote to stderr during discard phase: %s", stderrBuf.String())
	}
}
