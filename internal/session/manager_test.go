package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if len(m.sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(m.sessions))
	}
	if len(m.teams) != 0 {
		t.Errorf("expected 0 teams, got %d", len(m.teams))
	}
}

func TestManagerListEmpty(t *testing.T) {
	m := NewManager()
	sessions := m.List("")
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestManagerGetNotFound(t *testing.T) {
	m := NewManager()
	_, ok := m.Get("nonexistent")
	if ok {
		t.Error("expected not found")
	}
}

func TestManagerStopNotFound(t *testing.T) {
	m := NewManager()
	err := m.Stop("nonexistent")
	if err == nil {
		t.Error("expected error stopping nonexistent session")
	}
}

func TestManagerIsRunningEmpty(t *testing.T) {
	m := NewManager()
	if m.IsRunning("/tmp/repo") {
		t.Error("expected not running for empty manager")
	}
}

func TestManagerGetTeamNotFound(t *testing.T) {
	m := NewManager()
	_, ok := m.GetTeam("nonexistent")
	if ok {
		t.Error("expected team not found")
	}
}

func TestManagerListTeamsEmpty(t *testing.T) {
	m := NewManager()
	teams := m.ListTeams()
	if len(teams) != 0 {
		t.Errorf("expected 0 teams, got %d", len(teams))
	}
}

func TestManagerStopAlreadyStopped(t *testing.T) {
	m := NewManager()

	// Manually add a stopped session
	s := &Session{
		ID:     "test-session",
		Status: StatusCompleted,
	}
	m.sessionsMu.Lock()
	m.sessions[s.ID] = s
	m.sessionsMu.Unlock()

	err := m.Stop(s.ID)
	if err == nil {
		t.Error("expected error stopping completed session")
	}
}

func TestManagerFindByRepo(t *testing.T) {
	m := NewManager()

	s := &Session{
		ID:       "test-session",
		RepoPath: "/home/user/projects/myrepo",
		RepoName: "myrepo",
		Status:   StatusRunning,
	}
	m.sessionsMu.Lock()
	m.sessions[s.ID] = s
	m.sessionsMu.Unlock()

	found := m.FindByRepo("myrepo")
	if len(found) != 1 {
		t.Fatalf("expected 1 session, got %d", len(found))
	}
	if found[0].ID != "test-session" {
		t.Errorf("found[0].ID = %q, want test-session", found[0].ID)
	}

	notFound := m.FindByRepo("other")
	if len(notFound) != 0 {
		t.Errorf("expected 0 sessions for other repo, got %d", len(notFound))
	}
}

func TestManagerWithBus(t *testing.T) {
	bus := events.NewBus(100)
	m := NewManagerWithBus(bus)
	if m == nil {
		t.Fatal("NewManagerWithBus returned nil")
	}
	if m.bus != bus {
		t.Error("bus not wired")
	}
}

func TestManagerSessionLifecycle(t *testing.T) {
	m := NewManager()

	// Manually inject a session to test lifecycle without spawning a real process
	s := &Session{
		ID:           "lifecycle-test",
		Provider:     ProviderClaude,
		RepoPath:     "/tmp/test-repo",
		RepoName:     "test-repo",
		Status:       StatusRunning,
		Model:        "sonnet",
		BudgetUSD:    10.0,
		LaunchedAt:   time.Now(),
		LastActivity: time.Now(),
	}
	m.sessionsMu.Lock()
	m.sessions[s.ID] = s
	m.sessionsMu.Unlock()

	// Get returns the session
	got, ok := m.Get("lifecycle-test")
	if !ok {
		t.Fatal("session not found after insertion")
	}
	if got.Status != StatusRunning {
		t.Errorf("status = %q, want running", got.Status)
	}
	if got.Provider != ProviderClaude {
		t.Errorf("provider = %q, want claude", got.Provider)
	}

	// List returns it
	all := m.List("")
	if len(all) != 1 {
		t.Fatalf("List() = %d sessions, want 1", len(all))
	}

	// List with matching repo path
	filtered := m.List("/tmp/test-repo")
	if len(filtered) != 1 {
		t.Errorf("List(matching) = %d, want 1", len(filtered))
	}

	// List with non-matching repo path
	filtered = m.List("/tmp/other")
	if len(filtered) != 0 {
		t.Errorf("List(non-matching) = %d, want 0", len(filtered))
	}

	// IsRunning
	if !m.IsRunning("/tmp/test-repo") {
		t.Error("expected IsRunning=true for running session")
	}

	// Stop (no process to kill, but status should change)
	if err := m.Stop("lifecycle-test"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if s.Status != StatusStopped {
		t.Errorf("status after stop = %q, want stopped", s.Status)
	}

	// IsRunning should be false after stop
	if m.IsRunning("/tmp/test-repo") {
		t.Error("expected IsRunning=false after stop")
	}

	// Stop again should error
	if err := m.Stop("lifecycle-test"); err == nil {
		t.Error("expected error stopping already-stopped session")
	}
}

func TestManagerStopAll(t *testing.T) {
	m := NewManager()

	for _, id := range []string{"s1", "s2", "s3"} {
		s := &Session{
			ID:       id,
			Status:   StatusRunning,
			RepoPath: "/tmp/repo",
		}
		m.sessionsMu.Lock()
		m.sessions[id] = s
		m.sessionsMu.Unlock()
	}

	// Add one completed session that should not be affected
	m.sessionsMu.Lock()
	m.sessions["s4"] = &Session{ID: "s4", Status: StatusCompleted, RepoPath: "/tmp/repo"}
	m.sessionsMu.Unlock()

	m.StopAll()

	for _, id := range []string{"s1", "s2", "s3"} {
		s, _ := m.Get(id)
		if s.Status != StatusStopped {
			t.Errorf("session %s status = %q, want stopped", id, s.Status)
		}
	}

	// Completed session should remain completed
	s4, _ := m.Get("s4")
	if s4.Status != StatusCompleted {
		t.Errorf("session s4 status = %q, want completed (unchanged)", s4.Status)
	}
}

func TestManagerListFiltersByProvider(t *testing.T) {
	m := NewManager()

	m.sessionsMu.Lock()
	m.sessions["claude-1"] = &Session{ID: "claude-1", Provider: ProviderClaude, RepoPath: "/tmp/a"}
	m.sessions["gemini-1"] = &Session{ID: "gemini-1", Provider: ProviderGemini, RepoPath: "/tmp/b"}
	m.sessionsMu.Unlock()

	// List all
	all := m.List("")
	if len(all) != 2 {
		t.Errorf("List('') = %d, want 2", len(all))
	}
}

func TestManagerTeamLifecycle(t *testing.T) {
	m := NewManager()

	// Manually inject a team
	team := &TeamStatus{
		Name:     "test-team",
		RepoPath: "/tmp/repo",
		LeadID:   "lead-session",
		Status:   StatusRunning,
		Tasks: []TeamTask{
			{Description: "task 1", Status: "pending"},
			{Description: "task 2", Status: "pending"},
		},
		CreatedAt: time.Now(),
	}
	m.sessionsMu.Lock()
	m.sessions["lead-session"] = &Session{ID: "lead-session", Status: StatusRunning}
	m.sessionsMu.Unlock()
	m.workersMu.Lock()
	m.teams["test-team"] = team
	m.workersMu.Unlock()

	// GetTeam
	got, ok := m.GetTeam("test-team")
	if !ok {
		t.Fatal("team not found")
	}
	if got.Name != "test-team" {
		t.Errorf("team name = %q, want test-team", got.Name)
	}
	if len(got.Tasks) != 2 {
		t.Errorf("tasks = %d, want 2", len(got.Tasks))
	}

	// ListTeams
	teams := m.ListTeams()
	if len(teams) != 1 {
		t.Errorf("ListTeams = %d, want 1", len(teams))
	}

	// Team status tracks lead session
	lead, _ := m.Get("lead-session")
	lead.mu.Lock()
	lead.Status = StatusCompleted
	lead.mu.Unlock()

	got, _ = m.GetTeam("test-team")
	if got.Status != StatusCompleted {
		t.Errorf("team status = %q, want completed (should track lead)", got.Status)
	}
}

func TestManagerDelegateTask(t *testing.T) {
	m := NewManager()

	// Set up a team
	m.sessionsMu.Lock()
	m.sessions["lead"] = &Session{ID: "lead", Status: StatusRunning}
	m.sessionsMu.Unlock()
	m.workersMu.Lock()
	m.teams["test-team"] = &TeamStatus{
		Name:     "test-team",
		RepoPath: "/tmp/repo",
		LeadID:   "lead",
		Status:   StatusRunning,
		Tasks:    []TeamTask{{Description: "task 1", Status: "pending"}},
	}
	m.workersMu.Unlock()

	// Delegate a new task
	count, err := m.DelegateTask("test-team", TeamTask{
		Description: "task 2",
		Status:      "pending",
	})
	if err != nil {
		t.Fatalf("DelegateTask: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	// Verify task was added
	team, ok := m.GetTeam("test-team")
	if !ok {
		t.Fatal("team not found")
	}
	if len(team.Tasks) != 2 {
		t.Errorf("tasks = %d, want 2", len(team.Tasks))
	}

	// Delegate to non-existent team
	_, err = m.DelegateTask("nonexistent", TeamTask{Description: "x", Status: "pending"})
	if err == nil {
		t.Error("expected error for nonexistent team")
	}
}

func TestManagerTaskStatusCorrelation(t *testing.T) {
	m := NewManager()

	// Set up a team with tasks
	m.sessionsMu.Lock()
	m.sessions["lead"] = &Session{ID: "lead", Status: StatusRunning, TeamName: "corr-team"}
	// Worker sessions: one running, one completed
	m.sessions["w1"] = &Session{
		ID:       "w1",
		Status:   StatusRunning,
		TeamName: "corr-team",
		Prompt:   "Please implement auth for the API",
	}
	m.sessions["w2"] = &Session{
		ID:       "w2",
		Status:   StatusCompleted,
		TeamName: "corr-team",
		Prompt:   "Please write tests for the auth module",
	}
	m.sessionsMu.Unlock()
	m.workersMu.Lock()
	m.teams["corr-team"] = &TeamStatus{
		Name:     "corr-team",
		RepoPath: "/tmp/repo",
		LeadID:   "lead",
		Status:   StatusRunning,
		Tasks: []TeamTask{
			{Description: "implement auth", Status: "pending"},
			{Description: "write tests", Status: "pending"},
			{Description: "update docs", Status: "pending"},
		},
	}
	m.workersMu.Unlock()

	team, ok := m.GetTeam("corr-team")
	if !ok {
		t.Fatal("team not found")
	}

	// Task "implement auth" should be in-progress (w1 is running, prompt contains "implement auth")
	if team.Tasks[0].Status != "in-progress" {
		t.Errorf("task 0 status = %q, want in-progress", team.Tasks[0].Status)
	}
	// Task "write tests" should be completed (w2 is completed, prompt contains "write tests")
	if team.Tasks[1].Status != "completed" {
		t.Errorf("task 1 status = %q, want completed", team.Tasks[1].Status)
	}
	// Task "update docs" has no matching worker — should remain pending
	if team.Tasks[2].Status != "pending" {
		t.Errorf("task 2 status = %q, want pending", team.Tasks[2].Status)
	}
}

func TestSessionBudgetTracking(t *testing.T) {
	s := &Session{
		ID:        "budget-test",
		BudgetUSD: 10.0,
		SpentUSD:  0.0,
	}

	s.mu.Lock()
	s.SpentUSD = 5.5
	s.CostHistory = append(s.CostHistory, 2.0, 3.5)
	s.mu.Unlock()

	if s.SpentUSD != 5.5 {
		t.Errorf("SpentUSD = %f, want 5.5", s.SpentUSD)
	}
	if len(s.CostHistory) != 2 {
		t.Errorf("CostHistory len = %d, want 2", len(s.CostHistory))
	}
}

func TestSessionOutputHistory(t *testing.T) {
	s := &Session{
		ID: "output-test",
	}

	s.mu.Lock()
	s.OutputHistory = append(s.OutputHistory, "line 1", "line 2", "line 3")
	s.TurnCount = 3
	s.mu.Unlock()

	if len(s.OutputHistory) != 3 {
		t.Errorf("OutputHistory len = %d, want 3", len(s.OutputHistory))
	}
	if s.TurnCount != 3 {
		t.Errorf("TurnCount = %d, want 3", s.TurnCount)
	}
}

// makeTestSession creates a Session and registers it in the manager without
// spawning a real process. Used by concurrent race tests.
func makeTestSession(m *Manager, id, repoPath string, status SessionStatus) *Session {
	s := &Session{
		ID:           id,
		Provider:     ProviderClaude,
		RepoPath:     repoPath,
		RepoName:     "test-repo",
		Status:       status,
		Model:        "sonnet",
		LaunchedAt:   time.Now(),
		LastActivity: time.Now(),
		OutputCh:     make(chan string, 100),
	}
	m.sessionsMu.Lock()
	m.sessions[id] = s
	m.sessionsMu.Unlock()
	return s
}

// TestManagerConcurrentSessionInsert exercises Manager under concurrent session
// insertions and reads. Run with -race to detect data races.
//
// We inject sessions directly (without spawning real processes) to keep the
// test hermetic while still exercising all map and status-read code paths.
func TestManagerConcurrentSessionInsert(t *testing.T) {
	const numSessions = 20

	m := NewManager()

	var wg sync.WaitGroup

	// Concurrently insert sessions.
	for i := range numSessions {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			makeTestSession(m, fmt.Sprintf("conc-sess-%d", i),
				fmt.Sprintf("/tmp/repo-%d", i), StatusRunning)
		}(i)
	}

	// Concurrently read session state while inserts are happening.
	for range 10 {
		wg.Go(func() {
			_ = m.List("")
			_ = m.ListTeams()
			_ = m.IsRunning("/tmp/repo-0")
			_ = m.FindByRepo("test-repo")
		})
	}

	wg.Wait()

	sessions := m.List("")
	if len(sessions) != numSessions {
		t.Errorf("got %d sessions, want %d", len(sessions), numSessions)
	}
}

// TestManagerConcurrentStopAll verifies StopAll races no other operations.
func TestManagerConcurrentStopAll(t *testing.T) {
	m := NewManager()
	m.SetStateDir("")

	// Inject 15 running sessions.
	for i := range 15 {
		makeTestSession(m, fmt.Sprintf("stop-sess-%d", i), "/tmp/repo", StatusRunning)
	}

	var wg sync.WaitGroup

	// Concurrent StopAll + List + Get.
	wg.Go(func() {
		m.StopAll()
	})

	for i := range 5 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = m.List("")
			_, _ = m.Get(fmt.Sprintf("stop-sess-%d", i))
		}(i)
	}

	wg.Wait()
}

// TestManagerConcurrentSessionWrite verifies concurrent writes to a single
// Session's mutable fields are race-free.
func TestManagerConcurrentSessionWrite(t *testing.T) {
	s := &Session{
		ID:        "race-sess",
		BudgetUSD: 100.0,
		OutputCh:  make(chan string, 256),
	}

	const workers = 20
	var wg sync.WaitGroup

	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.mu.Lock()
			s.SpentUSD += 0.01
			s.TurnCount++
			s.CostHistory = append(s.CostHistory, float64(i)*0.001)
			appendSessionOutput(s, fmt.Sprintf("output from worker %d", i), nil)
			s.mu.Unlock()
		}(i)
	}

	// Concurrent readers.
	for range 10 {
		wg.Go(func() {
			s.mu.Lock()
			_ = s.SpentUSD
			_ = s.TurnCount
			_ = len(s.OutputHistory)
			s.mu.Unlock()
		})
	}

	wg.Wait()

	s.mu.Lock()
	turns := s.TurnCount
	s.mu.Unlock()
	if turns != workers {
		t.Errorf("TurnCount = %d, want %d", turns, workers)
	}
}

// TestGoroutineCleanupOnStop verifies that goroutines started for a session are
// cancelled and exit after Stop() is called. No real process is spawned; sessions
// are injected directly with a cancel func and a lifecycle goroutine that waits on
// the session context — matching the pattern used by the real runner.
func TestGoroutineCleanupOnStop(t *testing.T) {
	m := NewManager()
	m.SetStateDir("") // no disk I/O

	// Capture baseline before creating any sessions.
	runtime.Gosched()
	baseline := runtime.NumGoroutine()

	for i := range 5 {
		sCtx, cancel := context.WithCancel(context.Background())
		id := fmt.Sprintf("gc-test-%d", i)
		s := &Session{
			ID:           id,
			Provider:     ProviderClaude,
			Status:       StatusRunning,
			RepoPath:     "/tmp/test-goroutine-cleanup",
			RepoName:     "test-repo",
			LaunchedAt:   time.Now(),
			LastActivity: time.Now(),
			OutputCh:     make(chan string, 10),
			cancel:       cancel,
		}
		m.sessionsMu.Lock()
		m.sessions[id] = s
		m.sessionsMu.Unlock()

		// One goroutine representing the session lifecycle — exits on cancel.
		go func(ctx context.Context, sess *Session) {
			defer close(sess.OutputCh)
			<-ctx.Done()
			sess.mu.Lock()
			if sess.Status == StatusRunning {
				sess.Status = StatusStopped
			}
			sess.mu.Unlock()
		}(sCtx, s)

		if err := m.Stop(id); err != nil {
			t.Fatalf("stop %d: %v", i, err)
		}
	}

	// Allow goroutines to observe ctx.Done() and exit.
	time.Sleep(50 * time.Millisecond)
	runtime.Gosched()

	after := runtime.NumGoroutine()
	if after > baseline+2 {
		t.Errorf("goroutine leak: baseline=%d after 5 start/stop cycles=%d (delta=%d, want ≤2)",
			baseline, after, after-baseline)
	}
}

// TestManagerConcurrentDelegateTask verifies concurrent DelegateTask calls
// do not race on the team's Tasks slice.
func TestManagerConcurrentDelegateTask(t *testing.T) {
	m := NewManager()

	m.sessionsMu.Lock()
	m.sessions["lead"] = &Session{ID: "lead", Status: StatusRunning}
	m.sessionsMu.Unlock()
	m.workersMu.Lock()
	m.teams["race-team"] = &TeamStatus{
		Name:     "race-team",
		RepoPath: "/tmp/repo",
		LeadID:   "lead",
		Status:   StatusRunning,
	}
	m.workersMu.Unlock()

	const n = 30
	var wg sync.WaitGroup
	errs := make(chan error, n)

	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := m.DelegateTask("race-team", TeamTask{
				Description: fmt.Sprintf("task-%d", i),
				Status:      "pending",
			})
			if err != nil {
				errs <- err
			}
		}(i)
	}

	// Concurrent reads while tasks are being delegated.
	for range 10 {
		wg.Go(func() {
			_, _ = m.GetTeam("race-team")
			_ = m.ListTeams()
		})
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent delegate error: %v", err)
	}

	team, ok := m.GetTeam("race-team")
	if !ok {
		t.Fatal("team not found")
	}
	if len(team.Tasks) != n {
		t.Errorf("Tasks = %d, want %d", len(team.Tasks), n)
	}
}

func TestPersistSessionError_ReadOnlyDir(t *testing.T) {
	m := NewManager()

	// Create a read-only directory so MkdirAll of a subdirectory fails.
	tmpDir := t.TempDir()
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0555); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Point stateDir to a child of the read-only dir so MkdirAll will fail.
	m.SetStateDir(filepath.Join(readOnlyDir, "sessions"))

	s := &Session{
		ID:       "err-test",
		Provider: ProviderClaude,
		Status:   StatusRunning,
	}

	err := m.PersistSession(s)
	if err == nil {
		t.Fatal("expected error from PersistSession with read-only parent, got nil")
	}
	if !strings.Contains(err.Error(), "persist session") {
		t.Errorf("error should contain 'persist session', got: %v", err)
	}
}

func TestPersistSession_Success(t *testing.T) {
	m := NewManager()
	tmpDir := t.TempDir()
	m.SetStateDir(tmpDir)

	s := &Session{
		ID:       "success-test",
		Provider: ProviderClaude,
		RepoPath: "/tmp/repo",
		RepoName: "repo",
		Status:   StatusRunning,
	}

	err := m.PersistSession(s)
	if err != nil {
		t.Fatalf("PersistSession: %v", err)
	}

	// Verify the file was written and contains valid JSON.
	path := filepath.Join(tmpDir, DefaultTenantID, "success-test.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var loaded Session
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if loaded.ID != "success-test" {
		t.Errorf("loaded ID = %q, want success-test", loaded.ID)
	}
	if loaded.Provider != ProviderClaude {
		t.Errorf("loaded Provider = %q, want claude", loaded.Provider)
	}
}

func TestPersistSessionDoesNotMutateLiveSession(t *testing.T) {
	m := NewManager()
	tmpDir := t.TempDir()
	m.SetStateDir(tmpDir)

	s := &Session{
		ID:       "normalize-test",
		TenantID: "",
		Provider: ProviderClaude,
		Status:   StatusRunning,
	}

	if err := m.PersistSession(s); err != nil {
		t.Fatalf("PersistSession: %v", err)
	}
	if s.TenantID != "" {
		t.Fatalf("live session tenant mutated to %q, want empty string", s.TenantID)
	}
}

func TestPersistSession_EmptyStateDir(t *testing.T) {
	m := NewManager()
	m.SetStateDir("")

	s := &Session{ID: "no-dir"}
	err := m.PersistSession(s)
	if err != nil {
		t.Errorf("expected nil error for empty stateDir, got: %v", err)
	}
}

func TestDetectStalls(t *testing.T) {
	m := NewManager()

	// Insert a stalled session: running but with old LastActivity.
	stale := &Session{
		ID:           "stale-session",
		Status:       StatusRunning,
		LastActivity: time.Now().Add(-10 * time.Minute),
	}
	// Insert a fresh running session.
	fresh := &Session{
		ID:           "fresh-session",
		Status:       StatusRunning,
		LastActivity: time.Now(),
	}
	// Insert a completed session with old activity (should NOT be returned).
	completed := &Session{
		ID:           "done-session",
		Status:       StatusCompleted,
		LastActivity: time.Now().Add(-1 * time.Hour),
	}

	m.sessionsMu.Lock()
	m.sessions[stale.ID] = stale
	m.sessions[fresh.ID] = fresh
	m.sessions[completed.ID] = completed
	m.sessionsMu.Unlock()

	stalled := m.DetectStalls(1 * time.Second)
	if len(stalled) != 1 {
		t.Fatalf("DetectStalls returned %d sessions, want 1", len(stalled))
	}
	if stalled[0] != "stale-session" {
		t.Errorf("DetectStalls returned %q, want %q", stalled[0], "stale-session")
	}

	// With a very large threshold, nothing should be stalled.
	stalled = m.DetectStalls(24 * time.Hour)
	if len(stalled) != 0 {
		t.Errorf("DetectStalls with large threshold returned %d sessions, want 0", len(stalled))
	}
}

func TestDetectStalls_Empty(t *testing.T) {
	m := NewManager()
	stalled := m.DetectStalls(1 * time.Second)
	if len(stalled) != 0 {
		t.Errorf("DetectStalls on empty manager returned %d sessions, want 0", len(stalled))
	}
}
