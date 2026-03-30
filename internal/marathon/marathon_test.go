package marathon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// newTestMarathon creates a marathon with a temp directory and mock manager.
func newTestMarathon(t *testing.T, cfg Config) (*Marathon, string) {
	t.Helper()
	dir := t.TempDir()
	if cfg.RepoPath == "" {
		cfg.RepoPath = dir
	}
	if cfg.Duration == 0 {
		cfg.Duration = 500 * time.Millisecond
	}
	if cfg.CheckpointInterval == 0 {
		cfg.CheckpointInterval = 100 * time.Millisecond
	}
	if cfg.ResourceCheckInterval == 0 {
		cfg.ResourceCheckInterval = 200 * time.Millisecond
	}

	bus := events.NewBus(1000)
	mgr := session.NewManagerWithBus(bus)
	mgr.SetStateDir(filepath.Join(dir, "sessions"))

	// Install test hooks so Launch does not exec real binaries.
	mgr.SetHooksForTesting(
		func(_ context.Context, opts session.LaunchOptions) (*session.Session, error) {
			return &session.Session{
				ID:       "mock-sess",
				Status:   session.StatusCompleted,
				Provider: session.ProviderClaude,
			}, nil
		},
		func(_ context.Context, _ *session.Session) error {
			return nil
		},
	)

	m := New(cfg, mgr, bus)
	return m, dir
}

func TestRun_DurationLimit(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  300 * time.Millisecond,
	})

	start := time.Now()
	stats, err := m.Run(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if stats == nil {
		t.Fatal("Run returned nil stats")
	}
	if stats.Duration <= 0 {
		t.Fatalf("expected positive duration, got %s", stats.Duration)
	}
	// Should have stopped roughly at the duration limit.
	if elapsed > 2*time.Second {
		t.Fatalf("marathon ran too long: %s (limit was 300ms)", elapsed)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  10 * time.Second, // long duration
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay.
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	stats, err := m.Run(ctx)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if stats == nil {
		t.Fatal("Run returned nil stats")
	}
	if stats.Duration > 2*time.Second {
		t.Fatalf("marathon should have stopped on cancel, ran for %s", stats.Duration)
	}
}

func TestRun_CheckpointsSaved(t *testing.T) {
	m, dir := newTestMarathon(t, Config{
		BudgetUSD:          100.0,
		Duration:           400 * time.Millisecond,
		CheckpointInterval: 80 * time.Millisecond,
	})

	stats, err := m.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats == nil {
		t.Fatal("nil stats")
	}

	// Check that checkpoints were saved.
	cpDir := filepath.Join(dir, ".ralph", "marathon", "checkpoints")
	cps, err := ListCheckpoints(cpDir)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}
	// Expect at least 2 checkpoints (periodic + final).
	if len(cps) < 2 {
		t.Fatalf("expected >= 2 checkpoints, got %d", len(cps))
	}
}

func TestRun_SessionCountTracked(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  300 * time.Millisecond,
	})

	// Publish a session started event after a small delay.
	go func() {
		time.Sleep(50 * time.Millisecond)
		m.bus.Publish(events.Event{
			Type:     events.SessionStarted,
			RepoPath: m.cfg.RepoPath,
		})
		m.bus.Publish(events.Event{
			Type:     events.SessionStarted,
			RepoPath: m.cfg.RepoPath,
		})
	}()

	stats, err := m.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.SessionsRun < 2 {
		t.Fatalf("expected >= 2 sessions tracked, got %d", stats.SessionsRun)
	}
}

func TestRun_SupervisorAccessible(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  200 * time.Millisecond,
	})

	// Before Run, supervisor should be nil.
	if m.Supervisor() != nil {
		t.Fatal("expected nil supervisor before Run")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = m.Run(context.Background())
	}()

	// Give Run time to start the supervisor.
	time.Sleep(50 * time.Millisecond)
	sup := m.Supervisor()
	if sup == nil {
		t.Fatal("expected non-nil supervisor during Run")
	}
	if !sup.Running() {
		t.Fatal("expected supervisor to be running")
	}

	<-done
}

func TestCurrentStats(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 50.0,
		Duration:  200 * time.Millisecond,
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = m.Run(context.Background())
	}()

	time.Sleep(50 * time.Millisecond)
	stats := m.CurrentStats()
	if stats.Duration <= 0 {
		t.Fatal("expected positive duration from CurrentStats during Run")
	}

	<-done
}

// --- Checkpoint unit tests ---

func TestSaveLoadCheckpoint(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")

	cp := &Checkpoint{
		Timestamp:       time.Now().Truncate(time.Second),
		CyclesCompleted: 5,
		SpentUSD:        2.50,
		SupervisorState: session.SupervisorState{
			Running:   true,
			RepoPath:  "/tmp/test-repo",
			TickCount: 42,
		},
	}

	if err := SaveCheckpoint(dir, cp); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	loaded, err := LoadLatestCheckpoint(dir)
	if err != nil {
		t.Fatalf("LoadLatestCheckpoint: %v", err)
	}

	if loaded.CyclesCompleted != cp.CyclesCompleted {
		t.Fatalf("CyclesCompleted: got %d, want %d", loaded.CyclesCompleted, cp.CyclesCompleted)
	}
	if loaded.SpentUSD != cp.SpentUSD {
		t.Fatalf("SpentUSD: got %f, want %f", loaded.SpentUSD, cp.SpentUSD)
	}
	if loaded.SupervisorState.TickCount != cp.SupervisorState.TickCount {
		t.Fatalf("TickCount: got %d, want %d", loaded.SupervisorState.TickCount, cp.SupervisorState.TickCount)
	}
}

func TestListCheckpoints_Empty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "empty")
	cps, err := ListCheckpoints(dir)
	if err != nil {
		t.Fatalf("ListCheckpoints on non-existent dir: %v", err)
	}
	if len(cps) != 0 {
		t.Fatalf("expected 0 checkpoints, got %d", len(cps))
	}
}

func TestListCheckpoints_Ordering(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")

	// Save three checkpoints with different timestamps (spaced 1s apart).
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		cp := &Checkpoint{
			Timestamp:       base.Add(time.Duration(i) * time.Second),
			CyclesCompleted: i + 1,
			SpentUSD:        float64(i) * 1.5,
		}
		if err := SaveCheckpoint(dir, cp); err != nil {
			t.Fatalf("SaveCheckpoint[%d]: %v", i, err)
		}
	}

	cps, err := ListCheckpoints(dir)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}
	if len(cps) != 3 {
		t.Fatalf("expected 3 checkpoints, got %d", len(cps))
	}

	// Verify ascending order.
	for i := 1; i < len(cps); i++ {
		if !cps[i].Timestamp.After(cps[i-1].Timestamp) {
			t.Fatalf("checkpoint[%d] (%s) not after checkpoint[%d] (%s)",
				i, cps[i].Timestamp, i-1, cps[i-1].Timestamp)
		}
	}
}

func TestLoadLatestCheckpoint_NoCheckpoints(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	_, err := LoadLatestCheckpoint(dir)
	if err == nil {
		t.Fatal("expected error for empty checkpoint dir")
	}
}

func TestCheckpointJSONRoundtrip(t *testing.T) {
	cp := &Checkpoint{
		Timestamp:       time.Now().Truncate(time.Millisecond),
		CyclesCompleted: 10,
		SpentUSD:        7.89,
		SupervisorState: session.SupervisorState{
			Running:   false,
			RepoPath:  "/repo",
			TickCount: 100,
		},
	}

	data, err := json.Marshal(cp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var restored Checkpoint
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if restored.CyclesCompleted != cp.CyclesCompleted {
		t.Fatalf("CyclesCompleted mismatch: %d vs %d", restored.CyclesCompleted, cp.CyclesCompleted)
	}
	if restored.SpentUSD != cp.SpentUSD {
		t.Fatalf("SpentUSD mismatch: %f vs %f", restored.SpentUSD, cp.SpentUSD)
	}
}

func TestCheckpointDir(t *testing.T) {
	got := checkpointDir("/home/user/repo")
	want := "/home/user/repo/.ralph/marathon/checkpoints"
	if got != want {
		t.Fatalf("checkpointDir: got %q, want %q", got, want)
	}
}

// --- Config validation ---

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := Config{
		BudgetUSD:          10.0,
		Duration:           time.Minute,
		CheckpointInterval: 30 * time.Second,
		SessionCount:       2,
		RepoPath:           "/tmp/repo",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestConfig_Validate_ZeroDuration(t *testing.T) {
	cfg := Config{
		BudgetUSD:          10.0,
		CheckpointInterval: time.Minute,
		RepoPath:           "/tmp/repo",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for zero duration")
	}
}

func TestConfig_Validate_NegativeDuration(t *testing.T) {
	cfg := Config{
		BudgetUSD:          10.0,
		Duration:           -time.Second,
		CheckpointInterval: time.Minute,
		RepoPath:           "/tmp/repo",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative duration")
	}
}

func TestConfig_Validate_ZeroCheckpointInterval(t *testing.T) {
	cfg := Config{
		BudgetUSD: 10.0,
		Duration:  time.Minute,
		RepoPath:  "/tmp/repo",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for zero checkpoint interval")
	}
}

func TestConfig_Validate_NegativeBudget(t *testing.T) {
	cfg := Config{
		BudgetUSD:          -5.0,
		Duration:           time.Minute,
		CheckpointInterval: time.Minute,
		RepoPath:           "/tmp/repo",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative budget")
	}
}

func TestConfig_Validate_ZeroBudget(t *testing.T) {
	cfg := Config{
		Duration:           time.Minute,
		CheckpointInterval: time.Minute,
		RepoPath:           "/tmp/repo",
	}
	// Zero budget is valid (means unlimited).
	if err := cfg.Validate(); err != nil {
		t.Fatalf("zero budget should be valid: %v", err)
	}
}

func TestConfig_Validate_NegativeSessionCount(t *testing.T) {
	cfg := Config{
		BudgetUSD:          10.0,
		Duration:           time.Minute,
		CheckpointInterval: time.Minute,
		SessionCount:       -1,
		RepoPath:           "/tmp/repo",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative session count")
	}
}

func TestConfig_Validate_EmptyRepoPath(t *testing.T) {
	cfg := Config{
		BudgetUSD:          10.0,
		Duration:           time.Minute,
		CheckpointInterval: time.Minute,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty repo path")
	}
}

// --- SessionCount default ---

func TestNew_DefaultSessionCount(t *testing.T) {
	bus := events.NewBus(100)
	mgr := session.NewManagerWithBus(bus)

	m := New(Config{
		BudgetUSD:          10,
		Duration:           time.Minute,
		CheckpointInterval: time.Minute,
	}, mgr, bus)

	if m.cfg.SessionCount != 1 {
		t.Fatalf("expected default SessionCount=1, got %d", m.cfg.SessionCount)
	}
}

func TestNew_CustomSessionCount(t *testing.T) {
	bus := events.NewBus(100)
	mgr := session.NewManagerWithBus(bus)

	m := New(Config{
		BudgetUSD:          10,
		Duration:           time.Minute,
		CheckpointInterval: time.Minute,
		SessionCount:       4,
	}, mgr, bus)

	if m.cfg.SessionCount != 4 {
		t.Fatalf("expected SessionCount=4, got %d", m.cfg.SessionCount)
	}
}

// --- Status ---

func TestStatus_BeforeRun(t *testing.T) {
	bus := events.NewBus(100)
	mgr := session.NewManagerWithBus(bus)

	m := New(Config{
		BudgetUSD:          50.0,
		Duration:           time.Minute,
		CheckpointInterval: time.Minute,
		SessionCount:       3,
	}, mgr, bus)

	st := m.Status()
	if st.Running {
		t.Fatal("expected not running before Run")
	}
	if st.Elapsed != 0 {
		t.Fatalf("expected zero elapsed, got %s", st.Elapsed)
	}
	if st.BudgetUSD != 50.0 {
		t.Fatalf("expected budget 50.0, got %f", st.BudgetUSD)
	}
	if st.SessionCount != 3 {
		t.Fatalf("expected session count 3, got %d", st.SessionCount)
	}
	if st.SessionsActive != 0 {
		t.Fatalf("expected 0 active sessions, got %d", st.SessionsActive)
	}
}

func TestStatus_DuringRun(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  300 * time.Millisecond,
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = m.Run(context.Background())
	}()

	time.Sleep(50 * time.Millisecond)
	st := m.Status()
	if st.Elapsed <= 0 {
		t.Fatal("expected positive elapsed during Run")
	}
	if st.BudgetUSD != 100.0 {
		t.Fatalf("expected budget 100.0, got %f", st.BudgetUSD)
	}
	<-done
}

// --- Manual Checkpoint ---

func TestCheckpoint_Manual(t *testing.T) {
	m, dir := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  time.Second,
	})

	// Manual checkpoint before Run (no supervisor).
	if err := m.Checkpoint(); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	cpDir := filepath.Join(dir, ".ralph", "marathon", "checkpoints")
	cps, err := ListCheckpoints(cpDir)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}
	if len(cps) != 1 {
		t.Fatalf("expected 1 manual checkpoint, got %d", len(cps))
	}
}

func TestCheckpoint_ManualDuringRun(t *testing.T) {
	m, dir := newTestMarathon(t, Config{
		BudgetUSD:          100.0,
		Duration:           400 * time.Millisecond,
		CheckpointInterval: 10 * time.Second, // long interval so periodic doesn't fire
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = m.Run(context.Background())
	}()

	time.Sleep(50 * time.Millisecond)

	if err := m.Checkpoint(); err != nil {
		t.Fatalf("Checkpoint during Run: %v", err)
	}

	<-done

	cpDir := filepath.Join(dir, ".ralph", "marathon", "checkpoints")
	cps, err := ListCheckpoints(cpDir)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}
	// At least 1 manual + 1 final checkpoint from finalize.
	if len(cps) < 2 {
		t.Fatalf("expected >= 2 checkpoints, got %d", len(cps))
	}
}

// --- Budget enforcement ---

func TestRun_BudgetEnforcement(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 5.0,
		Duration:  5 * time.Second, // long duration
	})

	// Publish a cost event that exceeds budget shortly after start.
	go func() {
		time.Sleep(50 * time.Millisecond)
		m.bus.Publish(events.Event{
			Type:     events.CostUpdate,
			RepoPath: m.cfg.RepoPath,
			Data:     map[string]any{"spent_usd": 6.0},
		})
	}()

	start := time.Now()
	stats, err := m.Run(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats == nil {
		t.Fatal("nil stats")
	}
	// Should have terminated well before the 5s duration.
	if elapsed > 2*time.Second {
		t.Fatalf("marathon should have stopped on budget, ran for %s", elapsed)
	}
	if stats.TotalSpentUSD < 5.0 {
		t.Fatalf("expected spend >= 5.0, got %f", stats.TotalSpentUSD)
	}
}

// --- ErrBudgetExceeded sentinel ---

func TestErrBudgetExceeded(t *testing.T) {
	if ErrBudgetExceeded == nil {
		t.Fatal("ErrBudgetExceeded should not be nil")
	}
	if ErrBudgetExceeded.Error() != "marathon: budget limit exceeded" {
		t.Fatalf("unexpected error message: %s", ErrBudgetExceeded.Error())
	}
}

// --- MarathonStatus JSON roundtrip ---

func TestMarathonStatus_Fields(t *testing.T) {
	st := MarathonStatus{
		Running:         true,
		SessionsActive:  3,
		Elapsed:         5 * time.Minute,
		SpentUSD:        12.50,
		BudgetUSD:       100.0,
		CyclesCompleted: 42,
		SessionCount:    4,
	}

	data, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var restored MarathonStatus
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if restored.SessionsActive != 3 {
		t.Fatalf("SessionsActive: got %d, want 3", restored.SessionsActive)
	}
	if restored.BudgetUSD != 100.0 {
		t.Fatalf("BudgetUSD: got %f, want 100.0", restored.BudgetUSD)
	}
	if restored.SessionCount != 4 {
		t.Fatalf("SessionCount: got %d, want 4", restored.SessionCount)
	}
}
