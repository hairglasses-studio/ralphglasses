package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotateLogs_OversizedFile(t *testing.T) {
	dir := t.TempDir()

	// Create a 200-byte file, set max to 100 bytes.
	data := make([]byte, 200)
	for i := range data {
		data[i] = byte('A' + (i % 26))
	}
	fpath := filepath.Join(dir, "test.log")
	if err := os.WriteFile(fpath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	count, err := RotateLogs(dir, 100)
	if err != nil {
		t.Fatalf("RotateLogs error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 truncated file, got %d", count)
	}

	// File should now be ~50 bytes (maxBytes/2 = 50).
	info, err := os.Stat(fpath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 50 {
		t.Fatalf("expected rotated file to be 50 bytes, got %d", info.Size())
	}

	// Verify tail was preserved (last 50 bytes of original data).
	got, _ := os.ReadFile(fpath)
	expected := data[150:]
	if string(got) != string(expected) {
		t.Fatalf("tail not preserved: got %q, want %q", got[:10], expected[:10])
	}
}

func TestRotateLogs_NoFilesToRotate(t *testing.T) {
	dir := t.TempDir()

	// Create a small file that is under the threshold.
	fpath := filepath.Join(dir, "small.log")
	if err := os.WriteFile(fpath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	count, err := RotateLogs(dir, 1024)
	if err != nil {
		t.Fatalf("RotateLogs error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 truncated files, got %d", count)
	}
}

func TestRotateLogs_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	count, err := RotateLogs(dir, 100)
	if err != nil {
		t.Fatalf("RotateLogs error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 truncated files, got %d", count)
	}
}

func TestRotateLogs_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()

	// Create a subdirectory — should be skipped.
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	count, err := RotateLogs(dir, 1)
	if err != nil {
		t.Fatalf("RotateLogs error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
}

func TestRotateLogs_NonExistentDir(t *testing.T) {
	// Pre-loop calls RotateLogs on .ralph/logs which may not exist.
	// Should return an error, not panic.
	_, err := RotateLogs("/nonexistent/path/logs", MaxLogSize)
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

func TestRotateLogs_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	// Create two oversized files and one small file.
	big := make([]byte, 300)
	for i := range big {
		big[i] = byte('X')
	}
	small := []byte("tiny")

	for _, name := range []string{"a.log", "b.log"} {
		if err := os.WriteFile(filepath.Join(dir, name), big, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "c.log"), small, 0o644); err != nil {
		t.Fatal(err)
	}

	count, err := RotateLogs(dir, 100)
	if err != nil {
		t.Fatalf("RotateLogs error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 truncated files, got %d", count)
	}

	// c.log should be untouched.
	info, err := os.Stat(filepath.Join(dir, "c.log"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(len(small)) {
		t.Fatalf("small file was modified: got %d bytes, want %d", info.Size(), len(small))
	}
}
