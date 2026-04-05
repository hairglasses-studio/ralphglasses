package process

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupLogDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logDir := filepath.Join(dir, ".ralph", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeLogFile(t *testing.T, repoPath string, content string) {
	t.Helper()
	logPath := LogFilePath(repoPath)
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestReadFullLog_BasicLines(t *testing.T) {
	dir := setupLogDir(t)
	writeLogFile(t, dir, "line 1\nline 2\nline 3\n")

	lines, err := ReadFullLog(dir)
	if err != nil {
		t.Fatalf("ReadFullLog: %v", err)
	}

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "line 1" {
		t.Errorf("lines[0] = %q, want %q", lines[0], "line 1")
	}
	if lines[2] != "line 3" {
		t.Errorf("lines[2] = %q, want %q", lines[2], "line 3")
	}
}

func TestReadFullLog_EmptyFile(t *testing.T) {
	dir := setupLogDir(t)
	writeLogFile(t, dir, "")

	lines, err := ReadFullLog(dir)
	if err != nil {
		t.Fatalf("ReadFullLog: %v", err)
	}

	if len(lines) != 0 {
		t.Errorf("expected 0 lines for empty file, got %d", len(lines))
	}
}

func TestReadFullLog_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadFullLog(dir)
	if err == nil {
		t.Fatal("expected error for missing log file")
	}
}

func TestReadFullLog_LargeLines(t *testing.T) {
	dir := setupLogDir(t)
	// Create a file with lines under the buffer limit
	var content strings.Builder
	for i := range 100 {
		content.WriteString("log line number " + string(rune('A'+i%26)) + "\n")
	}
	writeLogFile(t, dir, content.String())

	lines, err := ReadFullLog(dir)
	if err != nil {
		t.Fatalf("ReadFullLog: %v", err)
	}

	if len(lines) != 100 {
		t.Errorf("expected 100 lines, got %d", len(lines))
	}
}

func TestTailLog_ReadsFromOffset(t *testing.T) {
	dir := setupLogDir(t)
	writeLogFile(t, dir, "line 1\nline 2\nline 3\n")

	var offset int64

	// First read — should get all lines
	cmd := TailLog(dir, &offset)
	msg := cmd()

	logMsg, ok := msg.(LogLinesMsg)
	if !ok {
		t.Fatalf("expected LogLinesMsg, got %T", msg)
	}
	if len(logMsg.Lines) != 3 {
		t.Fatalf("first read: expected 3 lines, got %d", len(logMsg.Lines))
	}
	if offset == 0 {
		t.Error("offset should be non-zero after first read")
	}

	// Second read with no new data — should get 0 lines
	cmd = TailLog(dir, &offset)
	msg = cmd()
	logMsg = msg.(LogLinesMsg)
	if len(logMsg.Lines) != 0 {
		t.Errorf("second read: expected 0 new lines, got %d", len(logMsg.Lines))
	}

	// Append new data
	logPath := LogFilePath(dir)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("line 4\nline 5\n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	// Third read — should get the new lines
	cmd = TailLog(dir, &offset)
	msg = cmd()
	logMsg = msg.(LogLinesMsg)
	if len(logMsg.Lines) != 2 {
		t.Errorf("third read: expected 2 new lines, got %d", len(logMsg.Lines))
	}
	if len(logMsg.Lines) == 2 {
		if logMsg.Lines[0] != "line 4" {
			t.Errorf("Lines[0] = %q, want %q", logMsg.Lines[0], "line 4")
		}
		if logMsg.Lines[1] != "line 5" {
			t.Errorf("Lines[1] = %q, want %q", logMsg.Lines[1], "line 5")
		}
	}
}

func TestTailLog_MissingFile(t *testing.T) {
	dir := t.TempDir()
	var offset int64

	cmd := TailLog(dir, &offset)
	msg := cmd()

	logMsg, ok := msg.(LogLinesMsg)
	if !ok {
		t.Fatalf("expected LogLinesMsg, got %T", msg)
	}
	if len(logMsg.Lines) != 1 {
		t.Fatalf("expected 1 error line for missing file, got %d", len(logMsg.Lines))
	}
	if !strings.Contains(logMsg.Lines[0], "[error]") {
		t.Errorf("expected error indicator, got: %s", logMsg.Lines[0])
	}
}

// TestLogFilePath_Canonical verifies the canonical path structure.
func TestLogFilePath_Canonical(t *testing.T) {
	got := LogFilePath("/some/repo")
	want := filepath.Join("/some/repo", ".ralph", "logs", "ralph.log")
	if got != want {
		t.Errorf("LogFilePath = %q, want %q", got, want)
	}
}

// TestLogDirPath_Canonical verifies the log directory structure.
func TestLogDirPath_Canonical(t *testing.T) {
	got := LogDirPath("/some/repo")
	want := filepath.Join("/some/repo", ".ralph", "logs")
	if got != want {
		t.Errorf("LogDirPath = %q, want %q", got, want)
	}
}

// TestLogFilePath_ContainedInLogDir verifies LogFilePath is inside LogDirPath.
func TestLogFilePath_ContainedInLogDir(t *testing.T) {
	base := "/test/path"
	filePath := LogFilePath(base)
	dirPath := LogDirPath(base)

	dir := filepath.Dir(filePath)
	if dir != dirPath {
		t.Errorf("LogFilePath dir = %q, LogDirPath = %q — mismatch", dir, dirPath)
	}
}

// TestLogPath_WriteReadRoundTrip verifies that writing to LogFilePath and
// reading via ReadFullLog uses the same path (the core of FINDING-79).
func TestLogPath_WriteReadRoundTrip(t *testing.T) {
	dir := setupLogDir(t)

	// Simulate what cmd/root.go and cmd/mcp.go do: write to LogFilePath.
	logPath := LogFilePath(dir)
	content := "server log entry 1\nserver log entry 2\n"
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Now read via ReadFullLog (which also uses LogFilePath internally).
	lines, err := ReadFullLog(dir)
	if err != nil {
		t.Fatalf("ReadFullLog after writing to LogFilePath: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "server log entry 1" {
		t.Errorf("lines[0] = %q, want %q", lines[0], "server log entry 1")
	}
}

// --- OpenLogFile pressure tests (FINDING-169) ---

func TestOpenLogFile_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	// Verify .ralph/logs/ does NOT exist yet
	logDir := filepath.Join(dir, ".ralph", "logs")
	if _, err := os.Stat(logDir); err == nil {
		t.Fatal("expected .ralph/logs/ to not exist before OpenLogFile")
	}

	f, err := OpenLogFile(dir)
	if err != nil {
		t.Fatalf("OpenLogFile: %v", err)
	}
	defer f.Close()

	if f == nil {
		t.Fatal("expected non-nil file handle")
	}

	// Verify the directory was created
	info, err := os.Stat(logDir)
	if err != nil {
		t.Fatalf("expected .ralph/logs/ to exist after OpenLogFile: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected .ralph/logs/ to be a directory")
	}

	// Verify file is writable: write, close, read back
	msg := "pressure test write\n"
	if _, err := f.WriteString(msg); err != nil {
		t.Fatalf("write to log file: %v", err)
	}
	f.Close()

	data, err := os.ReadFile(LogFilePath(dir))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != msg {
		t.Errorf("read back = %q, want %q", string(data), msg)
	}
}

func TestOpenLogFile_ExistingDir(t *testing.T) {
	dir := setupLogDir(t) // creates .ralph/logs/

	f, err := OpenLogFile(dir)
	if err != nil {
		t.Fatalf("OpenLogFile: %v", err)
	}
	defer f.Close()

	if f == nil {
		t.Fatal("expected non-nil file handle")
	}

	// Verify writable
	msg := "existing dir test\n"
	if _, err := f.WriteString(msg); err != nil {
		t.Fatalf("write to log file: %v", err)
	}
}
