package session

import (
	"context"
	"testing"
	"time"
)

func TestRunLoop_NotFound(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	err := m.RunLoop(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent loop")
	}
}

func TestRunLoop_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	// Create a loop run with StepLoop that blocks until context is cancelled
	m.SetHooksForTesting(
		func(_ context.Context, opts LaunchOptions) (*Session, error) {
			return &Session{
				ID:         "step-sess",
				Provider:   opts.Provider,
				RepoPath:   opts.RepoPath,
				Status:     StatusCompleted,
				OutputCh:   make(chan string, 1),
				LaunchedAt: time.Now(),
			}, nil
		},
		func(ctx context.Context, _ *Session) error {
			// Block until context is done
			<-ctx.Done()
			return ctx.Err()
		},
	)

	run := &LoopRun{
		ID:       "run-cancel",
		RepoPath: dir,
		RepoName: "test",
		Status:   "pending",
		Profile: LoopProfile{
			PlannerProvider: ProviderCodex,
			WorkerProvider:  ProviderClaude,
			PlannerModel:    "o1-pro",
			WorkerModel:     "opus-4",
			MaxIterations:   10,
			RetryLimit:      3,
			VerifyCommands:  []string{"true"},
		},
		Iterations: []LoopIteration{},
	}
	m.workersMu.Lock()
	m.loops["run-cancel"] = run
	m.workersMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- m.RunLoop(ctx, "run-cancel")
	}()

	// Give RunLoop a moment to start, then cancel
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error from cancelled RunLoop")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunLoop did not return after context cancellation")
	}
}

func TestRunLoop_PausedThenCancelled(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	run := &LoopRun{
		ID:       "paused-run",
		RepoPath: dir,
		RepoName: "test",
		Status:   "running",
		Paused:   true,
		Profile: LoopProfile{
			MaxIterations: 10,
			RetryLimit:    3,
		},
		Iterations: []LoopIteration{},
	}
	m.workersMu.Lock()
	m.loops["paused-run"] = run
	m.workersMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- m.RunLoop(ctx, "paused-run")
	}()

	// RunLoop should be in the pause poll loop; cancel it
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error from cancelled paused RunLoop")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunLoop did not return after context cancellation")
	}
}

func TestStopLoop_CancelsRunLoop(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	run := &LoopRun{
		ID:       "stop-test",
		RepoPath: dir,
		RepoName: "test",
		Status:   "running",
		Paused:   true, // Keep it paused so it doesn't try to StepLoop
		Profile: LoopProfile{
			MaxIterations: 10,
			RetryLimit:    3,
		},
		Iterations: []LoopIteration{},
		cancel:     cancel,
		done:       done,
	}
	m.workersMu.Lock()
	m.loops["stop-test"] = run
	m.workersMu.Unlock()

	// Simulate a RunLoop goroutine that waits for context cancel and closes done
	go func() {
		<-ctx.Done()
		close(done)
	}()

	err := m.StopLoop("stop-test")
	if err != nil {
		t.Fatalf("StopLoop: %v", err)
	}

	run.mu.Lock()
	status := run.Status
	run.mu.Unlock()

	if status != "stopped" {
		t.Errorf("status = %q, want stopped", status)
	}
}
