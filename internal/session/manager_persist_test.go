package session

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	m.mu.Lock()
	m.sessions["in-proc"] = existing
	m.mu.Unlock()

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
	m.mu.Lock()
	m.sessions["cleanup-1"] = old
	m.mu.Unlock()
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
	m.mu.Lock()
	m.loops["loop-1"] = run
	m.mu.Unlock()

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

	m.mu.Lock()
	got, ok := m.loops["ext-loop-1"]
	m.mu.Unlock()

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

	m.mu.Lock()
	m.loops["existing"] = &LoopRun{ID: "existing", Status: "running"}
	m.mu.Unlock()

	loopDir := filepath.Join(dir, "loops")
	os.MkdirAll(loopDir, 0755)
	run := &LoopRun{ID: "existing", Status: "completed"}
	os.WriteFile(filepath.Join(loopDir, "existing.json"), marshalLoopRun(run), 0644)

	m.LoadExternalLoops()

	m.mu.Lock()
	got := m.loops["existing"]
	m.mu.Unlock()

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

	m.mu.Lock()
	count := len(m.loops)
	m.mu.Unlock()

	if count != 0 {
		t.Errorf("expected 0 loops (invalid JSON skipped), got %d", count)
	}
}
