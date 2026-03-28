package process

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenLogFile_ReadOnlyParentDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0755) //nolint:errcheck

	_, err := OpenLogFile(dir)
	if err == nil {
		t.Error("expected error when log dir cannot be created")
	}
}

func TestReadPIDFile_ZeroPID(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFilePath(dir), []byte("0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	pid := readPIDFile(dir)
	if pid != 0 {
		t.Errorf("readPIDFile for zero PID = %d, want 0", pid)
	}
}

func TestReadPIDFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFilePath(dir), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	pid := readPIDFile(dir)
	if pid != 0 {
		t.Errorf("readPIDFile for empty file = %d, want 0", pid)
	}
}

func TestReadPIDFile_WhitespaceOnly(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFilePath(dir), []byte("   \n\n"), 0644); err != nil {
		t.Fatal(err)
	}

	pid := readPIDFile(dir)
	if pid != 0 {
		t.Errorf("readPIDFile for whitespace = %d, want 0", pid)
	}
}

func TestTailLog_InvalidHighOffset(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, ".ralph", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	logPath := LogFilePath(dir)
	if err := os.WriteFile(logPath, []byte("line 1\nline 2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Set offset beyond file end
	var offset int64 = 99999
	cmd := TailLog(dir, &offset)
	msg := cmd()
	logMsg, ok := msg.(LogLinesMsg)
	if !ok {
		t.Fatalf("expected LogLinesMsg, got %T", msg)
	}
	// Should return empty lines (not panic)
	if len(logMsg.Lines) != 0 {
		t.Errorf("expected 0 lines for offset beyond EOF, got %d", len(logMsg.Lines))
	}
}

func TestWatchStatusFiles_InvalidPaths(t *testing.T) {
	// Use paths that don't exist - all watches should fail
	cmd := WatchStatusFiles([]string{"/nonexistent/path/1", "/nonexistent/path/2"})
	msg := cmd()

	errMsg, ok := msg.(WatcherErrorMsg)
	if !ok {
		t.Fatalf("expected WatcherErrorMsg, got %T", msg)
	}
	if !strings.Contains(errMsg.Err.Error(), "all watches failed") {
		t.Errorf("expected 'all watches failed', got: %v", errMsg.Err)
	}
}

func TestWatchStatusFiles_SinglePath(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}

	// The watcher will time out after 2s with no file changes
	cmd := WatchStatusFiles([]string{dir})
	msg := cmd()

	switch v := msg.(type) {
	case WatcherErrorMsg:
		// Expected: either watch failure or timeout
		_ = v
	case FileChangedMsg:
		// Possible if something touches the dir during the test
		_ = v
	default:
		t.Errorf("unexpected msg type: %T", msg)
	}
}
