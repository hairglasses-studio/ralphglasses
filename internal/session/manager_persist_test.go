package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// marshalSession marshals a Session pointer to avoid copying the embedded mutex.
func marshalSession(s *Session) []byte {
	data, _ := json.Marshal(s)
	return data
}

// marshalLoopRun marshals a LoopRun pointer to avoid copying the embedded mutex.
func marshalLoopRun(r *LoopRun) []byte {
	data, _ := json.Marshal(r)
	return data
}

func TestLoadExternalSessions_NewSession(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	// Write a session JSON file to the state dir
	s := &Session{
		ID:         "ext-1",
		Provider:   ProviderClaude,
		RepoPath:   "/tmp/repo",
		RepoName:   "repo",
		Status:     StatusRunning,
		LaunchedAt: time.Now(),
	}
	if err := os.WriteFile(filepath.Join(dir, "ext-1.json"), marshalSession(s), 0644); err != nil {
		t.Fatal(err)
	}

	m.LoadExternalSessions()

	got, ok := m.Get("ext-1")
	if !ok {
		t.Fatal("expected external session to be loaded")
	}
	if got.Provider != ProviderClaude {
		t.Errorf("provider = %q, want claude", got.Provider)
	}
}

func TestLoadExternalSessions_SkipExisting(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	// Add an in-process session
	existing := &Session{
		ID:       "in-proc",
		Provider: ProviderGemini,
		Status:   StatusRunning,
	}
	m.sessionsMu.Lock()
	m.sessions["in-proc"] = existing
	m.sessionsMu.Unlock()

	// Write a stale version to disk
	stale := &Session{
		ID:         "in-proc",
		Provider:   ProviderClaude,
		Status:     StatusCompleted,
		LaunchedAt: time.Now(),
	}
	os.WriteFile(filepath.Join(dir, "in-proc.json"), marshalSession(stale), 0644)

	m.LoadExternalSessions()

	got, _ := m.Get("in-proc")
	if got.Provider != ProviderGemini {
		t.Errorf("provider = %q, want gemini (in-memory should not be overwritten)", got.Provider)
	}
}

func TestLoadExternalSessions_SkipOldSessions(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	old := &Session{
		ID:         "old-1",
		Provider:   ProviderClaude,
		Status:     StatusRunning,
		LaunchedAt: time.Now().Add(-48 * time.Hour),
	}
	os.WriteFile(filepath.Join(dir, "old-1.json"), marshalSession(old), 0644)

	m.LoadExternalSessions()

	_, ok := m.Get("old-1")
	if ok {
		t.Error("expected old session to be skipped")
	}
}

func TestLoadExternalSessions_SkipNonJSON(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not json"), 0644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)

	m.LoadExternalSessions()

	sessions := m.List("")
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestLoadExternalSessions_CleanupOldTerminal(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	// The cleanup loop runs on in-memory sessions, not disk-only ones.
	// So we inject a completed session into memory AND on disk.
	ended := time.Now().Add(-48 * time.Hour)
	old := &Session{
		ID:         "cleanup-1",
		Provider:   ProviderClaude,
		Status:     StatusCompleted,
		LaunchedAt: time.Now().Add(-72 * time.Hour),
		EndedAt:    &ended,
	}
	// Inject into memory
	m.sessionsMu.Lock()
	m.sessions["cleanup-1"] = old
	m.sessionsMu.Unlock()
	// Write to disk
	os.WriteFile(filepath.Join(dir, "cleanup-1.json"), marshalSession(old), 0644)

	m.LoadExternalSessions()

	// The session should be removed from both memory and disk
	_, inMemory := m.Get("cleanup-1")
	if inMemory {
		t.Error("expected old terminal session to be removed from memory")
	}
	if _, err := os.Stat(filepath.Join(dir, "cleanup-1.json")); !os.IsNotExist(err) {
		t.Error("expected old terminal session file to be cleaned up from disk")
	}

	// LoadExternalSessions fires a best-effort background goroutine to re-persist
	// any in-memory session it encounters; that goroutine may recreate the JSON
	// file after the cleanup loop deletes it. Register a cleanup (runs before
	// t.TempDir's RemoveAll due to LIFO ordering) that waits for the goroutine
	// and removes any such leftover file so RemoveAll does not fail.
	t.Cleanup(func() {
		runtime.Gosched()
		time.Sleep(5 * time.Millisecond)
		os.Remove(filepath.Join(dir, "cleanup-1.json"))
	})
}

func TestLoadExternalSessions_ErroredSessionRetained(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	// An errored session that ended 2 minutes ago should be retained
	// (default retention is 5 minutes).
	ended := time.Now().Add(-2 * time.Minute)
	errored := &Session{
		ID:         "errored-1",
		Provider:   ProviderClaude,
		Status:     StatusErrored,
		Error:      "signal: killed",
		LaunchedAt: time.Now().Add(-3 * time.Minute),
		EndedAt:    &ended,
	}
	m.sessionsMu.Lock()
	m.sessions["errored-1"] = errored
	m.sessionsMu.Unlock()
	os.WriteFile(filepath.Join(dir, "errored-1.json"), marshalSession(errored), 0644)

	m.LoadExternalSessions()

	_, ok := m.Get("errored-1")
	if !ok {
		t.Error("expected recently errored session to be retained")
	}
}

func TestLoadExternalSessions_ErroredSessionPurgedAfterRetention(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)
	m.ErrorRetention = 1 * time.Minute // short retention for testing

	// An errored session that ended 2 minutes ago should be purged
	// when retention is only 1 minute.
	ended := time.Now().Add(-2 * time.Minute)
	errored := &Session{
		ID:         "errored-old",
		Provider:   ProviderClaude,
		Status:     StatusErrored,
		Error:      "signal: killed",
		LaunchedAt: time.Now().Add(-3 * time.Minute),
		EndedAt:    &ended,
	}
	m.sessionsMu.Lock()
	m.sessions["errored-old"] = errored
	m.sessionsMu.Unlock()
	os.WriteFile(filepath.Join(dir, "errored-old.json"), marshalSession(errored), 0644)

	m.LoadExternalSessions()

	_, ok := m.Get("errored-old")
	if ok {
		t.Error("expected old errored session to be purged after retention window")
	}
}

func TestIsTerminal(t *testing.T) {
	tests := []struct {
		status   SessionStatus
		terminal bool
	}{
		{StatusLaunching, false},
		{StatusRunning, false},
		{StatusCompleted, true},
		{StatusErrored, true},
		{StatusStopped, true},
	}
	for _, tt := range tests {
		if got := tt.status.IsTerminal(); got != tt.terminal {
			t.Errorf("IsTerminal(%s) = %v, want %v", tt.status, got, tt.terminal)
		}
	}
}

func TestLoadExternalSessions_EmptyStateDir(t *testing.T) {
	m := NewManager()
	m.SetStateDir("")
	m.LoadExternalSessions()
}

func TestLoadExternalSessions_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{invalid json"), 0644)

	m.LoadExternalSessions()

	sessions := m.List("")
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions (invalid JSON skipped), got %d", len(sessions))
	}
}

func TestPersistLoop_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	run := &LoopRun{
		ID:       "loop-1",
		RepoPath: "/tmp/repo",
		RepoName: "repo",
		Status:   "running",
		Profile: LoopProfile{
			WorkerProvider: ProviderClaude,
		},
	}
	m.sessionsMu.Lock()
	m.loops["loop-1"] = run
	m.sessionsMu.Unlock()

	m.PersistLoop(run)

	loopDir := filepath.Join(dir, "loops")
	path := filepath.Join(loopDir, "loop-1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var loaded LoopRun
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if loaded.ID != "loop-1" {
		t.Errorf("loaded ID = %q, want loop-1", loaded.ID)
	}
	if loaded.Status != "running" {
		t.Errorf("loaded status = %q, want running", loaded.Status)
	}
}

func TestPersistLoop_EmptyStateDir(t *testing.T) {
	m := NewManager()
	m.SetStateDir("")
	run := &LoopRun{ID: "no-dir"}
	m.PersistLoop(run)
}

func TestLoadExternalLoops_NewLoop(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	loopDir := filepath.Join(dir, "loops")
	os.MkdirAll(loopDir, 0755)

	run := &LoopRun{
		ID:       "ext-loop-1",
		RepoPath: "/tmp/repo",
		RepoName: "repo",
		Status:   "running",
	}
	os.WriteFile(filepath.Join(loopDir, "ext-loop-1.json"), marshalLoopRun(run), 0644)

	m.LoadExternalLoops()

	m.sessionsMu.Lock()
	got, ok := m.loops["ext-loop-1"]
	m.sessionsMu.Unlock()

	if !ok {
		t.Fatal("expected external loop to be loaded")
	}
	if got.Status != "running" {
		t.Errorf("status = %q, want running", got.Status)
	}
}

func TestLoadExternalLoops_SkipExisting(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	m.sessionsMu.Lock()
	m.loops["existing"] = &LoopRun{ID: "existing", Status: "running"}
	m.sessionsMu.Unlock()

	loopDir := filepath.Join(dir, "loops")
	os.MkdirAll(loopDir, 0755)
	run := &LoopRun{ID: "existing", Status: "completed"}
	os.WriteFile(filepath.Join(loopDir, "existing.json"), marshalLoopRun(run), 0644)

	m.LoadExternalLoops()

	m.sessionsMu.Lock()
	got := m.loops["existing"]
	m.sessionsMu.Unlock()

	if got.Status != "running" {
		t.Errorf("status = %q, want running (in-memory should not be overwritten)", got.Status)
	}
}

func TestLoadExternalLoops_EmptyStateDir(t *testing.T) {
	m := NewManager()
	m.SetStateDir("")
	m.LoadExternalLoops()
}

func TestLoadExternalLoops_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	loopDir := filepath.Join(dir, "loops")
	os.MkdirAll(loopDir, 0755)
	os.WriteFile(filepath.Join(loopDir, "bad.json"), []byte("{invalid"), 0644)

	m.LoadExternalLoops()

	m.sessionsMu.Lock()
	count := len(m.loops)
	m.sessionsMu.Unlock()

	if count != 0 {
		t.Errorf("expected 0 loops (invalid JSON skipped), got %d", count)
	}
}
