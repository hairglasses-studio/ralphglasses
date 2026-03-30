package marathon

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- Constructor edge cases ---

func TestNew_DefaultResourceCheckInterval(t *testing.T) {
	bus := events.NewBus(100)
	mgr := session.NewManagerWithBus(bus)

	m := New(Config{
		BudgetUSD:          10,
		Duration:           time.Minute,
		CheckpointInterval: time.Minute,
	}, mgr, bus)

	if m.cfg.ResourceCheckInterval != 60*time.Second {
		t.Fatalf("expected default 60s ResourceCheckInterval, got %s", m.cfg.ResourceCheckInterval)
	}
}

func TestNew_CustomResourceCheckInterval(t *testing.T) {
	bus := events.NewBus(100)
	mgr := session.NewManagerWithBus(bus)

	m := New(Config{
		BudgetUSD:             10,
		Duration:              time.Minute,
		CheckpointInterval:    time.Minute,
		ResourceCheckInterval: 30 * time.Second,
	}, mgr, bus)

	if m.cfg.ResourceCheckInterval != 30*time.Second {
		t.Fatalf("expected 30s ResourceCheckInterval, got %s", m.cfg.ResourceCheckInterval)
	}
}

func TestNew_ZeroBudget(t *testing.T) {
	bus := events.NewBus(100)
	mgr := session.NewManagerWithBus(bus)

	m := New(Config{
		Duration:           time.Minute,
		CheckpointInterval: time.Minute,
	}, mgr, bus)

	if m.cfg.BudgetUSD != 0 {
		t.Fatalf("expected zero budget, got %f", m.cfg.BudgetUSD)
	}
}

// --- handleCostEvent ---

func TestHandleCostEvent_NilData(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  time.Second,
	})

	// Should not panic on nil data.
	m.handleCostEvent(events.Event{
		Type: events.CostUpdate,
		Data: nil,
	})

	stats := m.CurrentStats()
	if stats.TotalSpentUSD != 0 {
		t.Fatalf("expected 0 spent, got %f", stats.TotalSpentUSD)
	}
}

func TestHandleCostEvent_MissingSpentKey(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  time.Second,
	})

	m.handleCostEvent(events.Event{
		Type: events.CostUpdate,
		Data: map[string]any{"other_key": 1.0},
	})

	stats := m.CurrentStats()
	if stats.TotalSpentUSD != 0 {
		t.Fatalf("expected 0 spent, got %f", stats.TotalSpentUSD)
	}
}

func TestHandleCostEvent_WrongType(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  time.Second,
	})

	// spent_usd is a string, not float64.
	m.handleCostEvent(events.Event{
		Type: events.CostUpdate,
		Data: map[string]any{"spent_usd": "not-a-number"},
	})

	stats := m.CurrentStats()
	if stats.TotalSpentUSD != 0 {
		t.Fatalf("expected 0 spent, got %f", stats.TotalSpentUSD)
	}
}

func TestHandleCostEvent_ValidCost(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  time.Second,
	})

	m.handleCostEvent(events.Event{
		Type: events.CostUpdate,
		Data: map[string]any{"spent_usd": 5.25},
	})

	stats := m.CurrentStats()
	if stats.TotalSpentUSD != 5.25 {
		t.Fatalf("expected 5.25 spent, got %f", stats.TotalSpentUSD)
	}
}

func TestHandleCostEvent_HigherCostUpdates(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  time.Second,
	})

	m.handleCostEvent(events.Event{
		Type: events.CostUpdate,
		Data: map[string]any{"spent_usd": 10.0},
	})
	// A lower value should not overwrite.
	m.handleCostEvent(events.Event{
		Type: events.CostUpdate,
		Data: map[string]any{"spent_usd": 3.0},
	})

	stats := m.CurrentStats()
	if stats.TotalSpentUSD != 10.0 {
		t.Fatalf("expected 10.0 (max), got %f", stats.TotalSpentUSD)
	}
}

// --- checkResources ---

func TestCheckResources_Healthy(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  time.Second,
	})

	// Should not panic; resource.Check on a temp dir is typically healthy.
	m.checkResources()
}

func TestCheckResources_NilBus(t *testing.T) {
	dir := t.TempDir()
	// Create marathon with nil bus to cover that branch.
	m := &Marathon{
		cfg: Config{RepoPath: dir},
	}

	// Should not panic with nil bus.
	m.checkResources()
}

// --- CurrentStats before/after Run ---

func TestCurrentStats_BeforeRun(t *testing.T) {
	bus := events.NewBus(100)
	mgr := session.NewManagerWithBus(bus)

	m := New(Config{
		BudgetUSD:          10,
		Duration:           time.Minute,
		CheckpointInterval: time.Minute,
	}, mgr, bus)

	stats := m.CurrentStats()
	// Before Run, startedAt is zero so Duration should be 0.
	if stats.Duration != 0 {
		t.Fatalf("expected zero duration before Run, got %s", stats.Duration)
	}
	if stats.CyclesCompleted != 0 {
		t.Fatalf("expected 0 cycles, got %d", stats.CyclesCompleted)
	}
}

// --- Supervisor before Run ---

func TestSupervisor_BeforeRun(t *testing.T) {
	bus := events.NewBus(100)
	mgr := session.NewManagerWithBus(bus)

	m := New(Config{
		BudgetUSD:          10,
		Duration:           time.Minute,
		CheckpointInterval: time.Minute,
	}, mgr, bus)

	if m.Supervisor() != nil {
		t.Fatal("expected nil supervisor before Run")
	}
}

// --- Run with cost events via the bus ---

func TestRun_CostEventTracking(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  400 * time.Millisecond,
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		m.bus.Publish(events.Event{
			Type:     events.CostUpdate,
			RepoPath: m.cfg.RepoPath,
			Data:     map[string]any{"spent_usd": 1.50},
		})
		time.Sleep(20 * time.Millisecond)
		m.bus.Publish(events.Event{
			Type:     events.CostUpdate,
			RepoPath: m.cfg.RepoPath,
			Data:     map[string]any{"spent_usd": 3.75},
		})
	}()

	stats, err := m.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.TotalSpentUSD < 3.75 {
		t.Fatalf("expected TotalSpentUSD >= 3.75, got %f", stats.TotalSpentUSD)
	}
}

// --- Run with resume (no prior state, should log warning and continue) ---

func TestRun_ResumeNoPriorState(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  300 * time.Millisecond,
		Resume:    true,
	})

	stats, err := m.Run(context.Background())
	if err != nil {
		t.Fatalf("Run with resume: %v", err)
	}
	if stats == nil {
		t.Fatal("nil stats")
	}
}

// --- Run with resume from existing checkpoint ---

func TestRun_ResumeFromCheckpoint(t *testing.T) {
	dir := t.TempDir()
	bus := events.NewBus(1000)
	mgr := session.NewManagerWithBus(bus)
	mgr.SetStateDir(filepath.Join(dir, "sessions"))

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

	// Pre-create a checkpoint to resume from.
	cpDir := checkpointDir(dir)
	cp := &Checkpoint{
		Timestamp:       time.Now().Add(-time.Hour),
		CyclesCompleted: 7,
		SpentUSD:        2.50,
	}
	if err := SaveCheckpoint(cpDir, cp); err != nil {
		t.Fatalf("pre-save checkpoint: %v", err)
	}

	// Create a supervisor state file so ResumeFromState succeeds.
	stateDir := filepath.Join(dir, ".ralph")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	stateData := []byte(`{"running":false,"repo_path":"` + dir + `","tick_count":0}`)
	if err := os.WriteFile(filepath.Join(stateDir, "supervisor_state.json"), stateData, 0644); err != nil {
		t.Fatal(err)
	}

	m := New(Config{
		BudgetUSD:             100.0,
		Duration:              300 * time.Millisecond,
		CheckpointInterval:    100 * time.Millisecond,
		ResourceCheckInterval: 200 * time.Millisecond,
		RepoPath:              dir,
		Resume:                true,
	}, mgr, bus)

	stats, err := m.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The resumed cycles (7) should appear in the final stats.
	if stats.CyclesCompleted < 7 {
		t.Fatalf("expected >= 7 cycles from resume, got %d", stats.CyclesCompleted)
	}
	if stats.TotalSpentUSD < 2.50 {
		t.Fatalf("expected >= 2.50 spent from resume, got %f", stats.TotalSpentUSD)
	}
}

// --- Concurrent access to handleCostEvent ---

func TestHandleCostEvent_Concurrent(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  time.Second,
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(v float64) {
			defer wg.Done()
			m.handleCostEvent(events.Event{
				Type: events.CostUpdate,
				Data: map[string]any{"spent_usd": v},
			})
		}(float64(i))
	}
	wg.Wait()

	stats := m.CurrentStats()
	if stats.TotalSpentUSD != 19.0 {
		t.Fatalf("expected max cost 19.0, got %f", stats.TotalSpentUSD)
	}
}

// --- saveCheckpoint with nil supervisor ---

func TestSaveCheckpoint_NilSupervisor(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  time.Second,
	})

	// sup is nil before Run. saveCheckpoint should still work.
	m.saveCheckpoint()

	cpDir := checkpointDir(m.cfg.RepoPath)
	cps, err := ListCheckpoints(cpDir)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}
	if len(cps) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(cps))
	}
}

// --- finalize with zero startedAt ---

func TestFinalize_ZeroStartedAt(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  time.Second,
	})

	// Do not call Run. finalize should handle zero startedAt.
	result := m.finalize()
	if result.Duration != 0 {
		t.Fatalf("expected zero duration with zero startedAt, got %s", result.Duration)
	}
}

// --- Run with closed cost channel ---

func TestRun_ClosedCostChannel(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  300 * time.Millisecond,
	})

	// Publish a cost event with nil Data (closed-channel guard).
	go func() {
		time.Sleep(50 * time.Millisecond)
		m.bus.Publish(events.Event{
			Type:     events.CostUpdate,
			RepoPath: m.cfg.RepoPath,
			Data:     nil,
		})
	}()

	stats, err := m.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats == nil {
		t.Fatal("nil stats")
	}
}

// --- Run resource check fires ---

func TestRun_ResourceCheckFires(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD:             100.0,
		Duration:              400 * time.Millisecond,
		ResourceCheckInterval: 50 * time.Millisecond,
	})

	stats, err := m.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats == nil {
		t.Fatal("nil stats")
	}
}

// --- Concurrent CurrentStats and Supervisor during Run ---

func TestConcurrentStatsAndSupervisorDuringRun(t *testing.T) {
	m, _ := newTestMarathon(t, Config{
		BudgetUSD: 100.0,
		Duration:  300 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_, _ = m.Run(ctx)
	}()

	time.Sleep(30 * time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = m.CurrentStats()
				_ = m.Supervisor()
			}
		}()
	}
	wg.Wait()
	cancel()
	<-done
}

// --- checkpointDir variations ---

func TestCheckpointDir_TrailingSlash(t *testing.T) {
	got := checkpointDir("/repo/path")
	want := "/repo/path/.ralph/marathon/checkpoints"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// --- NewManagerWithBus integration ---

func TestRun_WithTestHooks(t *testing.T) {
	dir := t.TempDir()
	bus := events.NewBus(1000)
	mgr := session.NewManagerWithBus(bus)
	mgr.SetStateDir(filepath.Join(dir, "sessions"))

	launchCount := 0
	var launchMu sync.Mutex

	mgr.SetHooksForTesting(
		func(_ context.Context, opts session.LaunchOptions) (*session.Session, error) {
			launchMu.Lock()
			launchCount++
			launchMu.Unlock()
			return &session.Session{
				ID:       "test-sess",
				Status:   session.StatusCompleted,
				Provider: session.ProviderClaude,
			}, nil
		},
		func(_ context.Context, _ *session.Session) error {
			return nil
		},
	)

	m := New(Config{
		BudgetUSD:             100.0,
		Duration:              200 * time.Millisecond,
		CheckpointInterval:    50 * time.Millisecond,
		ResourceCheckInterval: 50 * time.Millisecond,
		RepoPath:              dir,
	}, mgr, bus)

	stats, err := m.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats == nil {
		t.Fatal("nil stats")
	}
}
