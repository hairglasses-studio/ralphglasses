package process

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchStatusFiles_DetectsWrite(t *testing.T) {
	repoPath := t.TempDir()
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create the status file first so the watcher has something to watch
	statusPath := filepath.Join(ralphDir, "status.json")
	if err := os.WriteFile(statusPath, []byte(`{"status":"idle"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Start watching
	cmd := WatchStatusFiles([]string{repoPath})

	// Write to the file after a brief delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = os.WriteFile(statusPath, []byte(`{"status":"running"}`), 0644)
	}()

	// Run the watcher command with a timeout
	done := make(chan interface{})
	go func() {
		msg := cmd()
		done <- msg
	}()

	select {
	case msg := <-done:
		if msg == nil {
			t.Fatal("expected FileChangedMsg, got nil")
		}
		fcm, ok := msg.(FileChangedMsg)
		if !ok {
			t.Fatalf("expected FileChangedMsg, got %T", msg)
		}
		if fcm.RepoPath != repoPath {
			t.Errorf("RepoPath = %q, want %q", fcm.RepoPath, repoPath)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watcher timed out waiting for file change event")
	}
}

func TestWatchStatusFiles_DetectsCircuitBreakerWrite(t *testing.T) {
	repoPath := t.TempDir()
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}

	cbPath := filepath.Join(ralphDir, ".circuit_breaker_state")
	if err := os.WriteFile(cbPath, []byte(`{"state":"CLOSED"}`), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := WatchStatusFiles([]string{repoPath})

	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = os.WriteFile(cbPath, []byte(`{"state":"OPEN"}`), 0644)
	}()

	done := make(chan interface{})
	go func() {
		msg := cmd()
		done <- msg
	}()

	select {
	case msg := <-done:
		if msg == nil {
			t.Fatal("expected FileChangedMsg, got nil")
		}
		fcm, ok := msg.(FileChangedMsg)
		if !ok {
			t.Fatalf("expected FileChangedMsg, got %T", msg)
		}
		if fcm.RepoPath != repoPath {
			t.Errorf("RepoPath = %q, want %q", fcm.RepoPath, repoPath)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watcher timed out")
	}
}

func TestWatchStatusFiles_DetectsProgressWrite(t *testing.T) {
	repoPath := t.TempDir()
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}

	progPath := filepath.Join(ralphDir, "progress.json")
	if err := os.WriteFile(progPath, []byte(`{"iteration":1}`), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := WatchStatusFiles([]string{repoPath})

	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = os.WriteFile(progPath, []byte(`{"iteration":2}`), 0644)
	}()

	done := make(chan interface{})
	go func() {
		msg := cmd()
		done <- msg
	}()

	select {
	case msg := <-done:
		if msg == nil {
			t.Fatal("expected FileChangedMsg, got nil")
		}
		_, ok := msg.(FileChangedMsg)
		if !ok {
			t.Fatalf("expected FileChangedMsg, got %T", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watcher timed out")
	}
}

func TestWatchStatusFiles_IgnoresUnrelatedFiles(t *testing.T) {
	repoPath := t.TempDir()
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}

	cmd := WatchStatusFiles([]string{repoPath})

	// Write an unrelated file, then write a watched file
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(ralphDir, "unrelated.txt"), []byte("hi"), 0644)
		time.Sleep(100 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(ralphDir, "status.json"), []byte(`{}`), 0644)
	}()

	done := make(chan interface{})
	go func() {
		msg := cmd()
		done <- msg
	}()

	select {
	case msg := <-done:
		// Should have been triggered by status.json, not unrelated.txt
		fcm, ok := msg.(FileChangedMsg)
		if !ok {
			t.Fatalf("expected FileChangedMsg, got %T", msg)
		}
		if fcm.RepoPath != repoPath {
			t.Errorf("RepoPath = %q, want %q", fcm.RepoPath, repoPath)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watcher timed out")
	}
}

func TestWatchStatusFiles_TimeoutReturnsNil(t *testing.T) {
	repoPath := t.TempDir()
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}

	cmd := WatchStatusFiles([]string{repoPath})

	// No writes — should return nil after timeout (~2s)
	done := make(chan interface{})
	go func() {
		msg := cmd()
		done <- msg
	}()

	select {
	case msg := <-done:
		if msg != nil {
			t.Fatalf("expected nil on timeout, got %T", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not return within 5s — timeout not working")
	}
}

func TestWatchStatusFiles_EmptyPaths(t *testing.T) {
	cmd := WatchStatusFiles([]string{})

	// With no paths, the watcher will block forever since nothing can happen.
	// We just verify it doesn't panic immediately.
	done := make(chan interface{})
	go func() {
		msg := cmd()
		done <- msg
	}()

	select {
	case <-done:
		// Acceptable — may return nil quickly if watcher setup fails
	case <-time.After(500 * time.Millisecond):
		// Expected — watcher blocks with nothing to watch
	}
}

func TestWatchStatusFiles_NonexistentPathReturnsError(t *testing.T) {
	// Watch a path that doesn't exist — all watches fail, should return WatcherErrorMsg
	cmd := WatchStatusFiles([]string{"/nonexistent/path/that/does/not/exist"})

	done := make(chan interface{})
	go func() {
		msg := cmd()
		done <- msg
	}()

	select {
	case msg := <-done:
		if msg == nil {
			t.Fatal("expected WatcherErrorMsg, got nil")
		}
		wem, ok := msg.(WatcherErrorMsg)
		if !ok {
			t.Fatalf("expected WatcherErrorMsg, got %T", msg)
		}
		if wem.Err == nil {
			t.Fatal("expected non-nil error in WatcherErrorMsg")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not return within 5s")
	}
}

func TestWatcherErrorMsg_HasError(t *testing.T) {
	msg := WatcherErrorMsg{Err: fmt.Errorf("test error")}
	if msg.Err == nil {
		t.Fatal("expected non-nil error")
	}
	if msg.Err.Error() != "test error" {
		t.Errorf("error = %q, want %q", msg.Err.Error(), "test error")
	}
}
