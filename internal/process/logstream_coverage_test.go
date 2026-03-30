package process

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadFullLogFallback_NoFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := ReadFullLogFallback(dir)
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}

func TestReadFullLogFallback_LegacyPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create the legacy .ralph/ralph.log file.
	legacyDir := filepath.Join(dir, ".ralph")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(legacyDir, "ralph.log")
	if err := os.WriteFile(legacyPath, []byte("legacy line 1\nlegacy line 2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	lines, err := ReadFullLogFallback(dir)
	if err != nil {
		t.Fatalf("ReadFullLogFallback: %v", err)
	}
	if len(lines) != 2 {
		t.Errorf("expected 2 lines from legacy path, got %d", len(lines))
	}
	if len(lines) > 0 && lines[0] != "legacy line 1" {
		t.Errorf("lines[0] = %q, want %q", lines[0], "legacy line 1")
	}
}

func TestReadFullLogFallback_GlobPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create .ralph/logs/ with a .log file (but NOT ralph.log at legacy location).
	logDir := filepath.Join(dir, ".ralph", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "session-123.log"), []byte("glob line 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "session-456.log"), []byte("glob line 2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	lines, err := ReadFullLogFallback(dir)
	if err != nil {
		t.Fatalf("ReadFullLogFallback glob: %v", err)
	}
	if len(lines) != 2 {
		t.Errorf("expected 2 lines from glob fallback, got %d", len(lines))
	}
}

func TestReadFullLogFallback_GlobEmptyLogFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	logDir := filepath.Join(dir, ".ralph", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create an empty .log file -- readLogFile returns nil lines for empty file.
	if err := os.WriteFile(filepath.Join(logDir, "empty.log"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadFullLogFallback(dir)
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist for empty glob log files, got %v", err)
	}
}

func TestReadLogFile_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0644); err != nil {
		t.Fatal(err)
	}

	lines, err := readLogFile(path)
	if err != nil {
		t.Fatalf("readLogFile: %v", err)
	}
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
}

func TestReadLogFile_Nonexistent(t *testing.T) {
	t.Parallel()
	_, err := readLogFile("/nonexistent/path/test.log")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestReadLogFile_Empty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.log")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	lines, err := readLogFile(path)
	if err != nil {
		t.Fatalf("readLogFile: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("expected 0 lines for empty file, got %d", len(lines))
	}
}
