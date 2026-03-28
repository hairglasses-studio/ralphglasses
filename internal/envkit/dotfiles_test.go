package envkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotRestoreRoundTrip(t *testing.T) {
	// Create a temp dir with some managed config files
	srcDir := t.TempDir()

	// Write test config files
	files := map[string]string{
		".config/starship.toml":    "format = \"$directory\"\n",
		".config/ghostty/config":   "font-family = MonaspiceNe Nerd Font\n",
		".config/bat/config":       "--theme=Catppuccin Mocha\n",
	}

	for rel, content := range files {
		abs := filepath.Join(srcDir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// Snapshot from srcDir
	snap, err := SnapshotDir(srcDir)
	if err != nil {
		t.Fatalf("SnapshotDir: %v", err)
	}

	if len(snap.Files) != len(files) {
		t.Errorf("expected %d files, got %d", len(files), len(snap.Files))
	}

	// Verify snapshot contents
	for rel, want := range files {
		got, ok := snap.Files[rel]
		if !ok {
			t.Errorf("snapshot missing %s", rel)
			continue
		}
		if got != want {
			t.Errorf("snapshot %s: got %q, want %q", rel, got, want)
		}
	}

	// Restore to a different dir
	dstDir := t.TempDir()
	if err := RestoreDir(snap, dstDir); err != nil {
		t.Fatalf("RestoreDir: %v", err)
	}

	// Verify restored files
	for rel, want := range files {
		abs := filepath.Join(dstDir, rel)
		data, err := os.ReadFile(abs)
		if err != nil {
			t.Errorf("read restored %s: %v", rel, err)
			continue
		}
		if string(data) != want {
			t.Errorf("restored %s: got %q, want %q", rel, string(data), want)
		}
	}
}

func TestSnapshotEmptyDir(t *testing.T) {
	dir := t.TempDir()
	snap, err := SnapshotDir(dir)
	if err != nil {
		t.Fatalf("SnapshotDir on empty dir: %v", err)
	}
	if len(snap.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(snap.Files))
	}
}

func TestSnapshotSummary(t *testing.T) {
	snap := &DotfileSnapshot{
		Files: map[string]string{
			".config/starship.toml": "content",
		},
	}

	summary := SnapshotSummary(snap)
	if summary == "" {
		t.Error("summary should not be empty")
	}

	emptSnap := &DotfileSnapshot{Files: map[string]string{}}
	emptySummary := SnapshotSummary(emptSnap)
	if emptySummary == "" {
		t.Error("empty summary should not be empty string")
	}
}

func TestSnapshotMissingConfigFiles(t *testing.T) {
	// Snapshot from a dir where none of the managed files exist
	dir := t.TempDir()
	snap, err := SnapshotDir(dir)
	if err != nil {
		t.Fatalf("SnapshotDir should not error when config files are missing: %v", err)
	}
	if len(snap.Files) != 0 {
		t.Errorf("expected 0 files when configs don't exist, got %d", len(snap.Files))
	}
}

func TestRestoreEmptySnapshot(t *testing.T) {
	dir := t.TempDir()
	snap := &DotfileSnapshot{Files: map[string]string{}}
	err := RestoreDir(snap, dir)
	if err != nil {
		t.Fatalf("RestoreDir with empty snapshot should not error: %v", err)
	}

	// Dir should remain empty (no files created)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty dir after restoring empty snapshot, got %d entries", len(entries))
	}
}

func TestRestoreNilFiles(t *testing.T) {
	dir := t.TempDir()
	snap := &DotfileSnapshot{Files: nil}
	err := RestoreDir(snap, dir)
	if err != nil {
		t.Fatalf("RestoreDir with nil files should not error: %v", err)
	}
}

func TestSnapshotDirNonexistent(t *testing.T) {
	snap, err := SnapshotDir("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("SnapshotDir on nonexistent dir should not error: %v", err)
	}
	if len(snap.Files) != 0 {
		t.Errorf("expected 0 files for nonexistent dir, got %d", len(snap.Files))
	}
}

func TestRestoreDirNonexistent(t *testing.T) {
	snap := &DotfileSnapshot{
		Files: map[string]string{
			".config/starship.toml": "test content\n",
		},
	}
	dir := filepath.Join(t.TempDir(), "nonexistent", "subdir")
	// RestoreDir should create intermediate dirs
	err := RestoreDir(snap, dir)
	if err != nil {
		t.Fatalf("RestoreDir should create intermediate dirs: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(filepath.Join(dir, ".config", "starship.toml"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(data) != "test content\n" {
		t.Errorf("got %q, want %q", string(data), "test content\n")
	}
}

func TestSnapshotSummaryMultipleFiles(t *testing.T) {
	snap := &DotfileSnapshot{
		Files: map[string]string{
			".config/starship.toml":  "content1",
			".config/ghostty/config": "content2",
			".config/bat/config":     "content3",
		},
	}
	summary := SnapshotSummary(snap)
	if summary == "" {
		t.Error("summary should not be empty")
	}
	// Should mention count
	if !strings.Contains(summary, "3") {
		t.Errorf("summary should mention file count 3, got: %s", summary)
	}
}

func TestSnapshotDirPartialFiles(t *testing.T) {
	dir := t.TempDir()

	// Only create one of the managed files
	abs := filepath.Join(dir, ".config", "starship.toml")
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	snap, err := SnapshotDir(dir)
	if err != nil {
		t.Fatalf("SnapshotDir: %v", err)
	}
	if len(snap.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(snap.Files))
	}
	if snap.Files[".config/starship.toml"] != "test" {
		t.Errorf("unexpected content: %q", snap.Files[".config/starship.toml"])
	}
}

func TestManagedPathsNotEmpty(t *testing.T) {
	paths := managedPaths()
	if len(paths) == 0 {
		t.Error("managedPaths should return non-empty list")
	}
}

func TestSnapshot(t *testing.T) {
	// Snapshot reads from real home dir; should not error even if no managed files exist
	snap, err := Snapshot()
	if err != nil {
		t.Fatalf("Snapshot should not error: %v", err)
	}
	if snap == nil {
		t.Fatal("Snapshot returned nil")
	}
	if snap.Files == nil {
		t.Error("Files map should be initialized, not nil")
	}
}

func TestRestore(t *testing.T) {
	// Restore with empty snapshot should be a no-op
	snap := &DotfileSnapshot{Files: map[string]string{}}
	err := Restore(snap)
	if err != nil {
		t.Fatalf("Restore with empty snapshot should not error: %v", err)
	}
}

func TestRestoreDirMkdirError(t *testing.T) {
	// Try to restore to a path that's a file, not a dir — should fail on MkdirAll
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := &DotfileSnapshot{
		Files: map[string]string{
			"sub/file.txt": "content",
		},
	}
	// Use the file as base dir — MkdirAll will fail because blocker is a file
	err := RestoreDir(snap, blockingFile)
	if err == nil {
		t.Error("expected error when base dir is a file")
	}
}
