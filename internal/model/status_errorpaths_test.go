package model

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func makeRalphDir(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	return dir, ralphDir
}

func TestLoadStatus_UnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod not effective on Windows")
	}
	dir, ralphDir := makeRalphDir(t)
	path := filepath.Join(ralphDir, "status.json")
	if err := os.WriteFile(path, []byte(`{"status":"running"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0644) })

	_, err := LoadStatus(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error for unreadable status.json, got nil")
	}
}

func TestLoadCircuitBreaker_UnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod not effective on Windows")
	}
	dir, ralphDir := makeRalphDir(t)
	path := filepath.Join(ralphDir, ".circuit_breaker_state")
	if err := os.WriteFile(path, []byte(`{"state":"closed"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0644) })

	_, err := LoadCircuitBreaker(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error for unreadable circuit breaker file, got nil")
	}
}

func TestLoadProgress_UnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod not effective on Windows")
	}
	dir, ralphDir := makeRalphDir(t)
	path := filepath.Join(ralphDir, "progress.json")
	if err := os.WriteFile(path, []byte(`{"iteration":1}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0644) })

	_, err := LoadProgress(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error for unreadable progress file, got nil")
	}
}

func TestRefreshRepo_MultipleCorruptFiles(t *testing.T) {
	dir, ralphDir := makeRalphDir(t)

	// Write corrupt data to all 3 files.
	if err := os.WriteFile(filepath.Join(ralphDir, "status.json"), []byte(`{bad`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ralphDir, ".circuit_breaker_state"), []byte(`{bad`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ralphDir, "progress.json"), []byte(`{bad`), 0644); err != nil {
		t.Fatal(err)
	}

	r := &Repo{Path: dir, Name: "test-repo"}
	errs := RefreshRepo(context.Background(), r)

	if len(errs) < 3 {
		t.Errorf("expected at least 3 errors from RefreshRepo with all corrupt files, got %d: %v", len(errs), errs)
	}

	// Fields should be nil when parsing fails.
	if r.Status != nil {
		t.Error("expected nil Status for corrupt status.json")
	}
	if r.Circuit != nil {
		t.Error("expected nil Circuit for corrupt circuit_breaker_state")
	}
	if r.Progress != nil {
		t.Error("expected nil Progress for corrupt progress.json")
	}
}

func TestRefreshRepo_MissingRalphDir(t *testing.T) {
	dir := t.TempDir()
	// No .ralph/ directory at all.
	r := &Repo{Path: dir, Name: "test-repo"}
	errs := RefreshRepo(context.Background(), r)

	// Missing files are not errors (os.ErrNotExist is skipped).
	if len(errs) != 0 {
		t.Errorf("expected 0 errors for missing .ralph dir, got %d: %v", len(errs), errs)
	}
}
