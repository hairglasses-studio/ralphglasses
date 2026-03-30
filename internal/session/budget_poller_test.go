package session

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBudgetPoller_PausesAtCeiling(t *testing.T) {
	pool := NewBudgetPool(10.0)
	_ = pool.Allocate("s1", 5.0)
	_ = pool.Allocate("s2", 5.0)

	// Push s1 over its allocation.
	pool.Record("s1", 6.0)

	var paused sync.Map
	pauseFn := func(sessionID string) error {
		paused.Store(sessionID, true)
		return nil
	}

	poller := NewBudgetPoller(pool, 10*time.Millisecond, pauseFn)
	ctx, cancel := context.WithCancel(context.Background())

	go poller.Start(ctx)

	// Wait for at least one tick.
	time.Sleep(50 * time.Millisecond)
	cancel()
	poller.Stop()

	if _, ok := paused.Load("s1"); !ok {
		t.Fatal("expected s1 to be paused")
	}
}

func TestBudgetPoller_NoPauseUnderCeiling(t *testing.T) {
	pool := NewBudgetPool(100.0)
	_ = pool.Allocate("s1", 50.0)
	pool.Record("s1", 10.0) // well under allocation

	var pauseCount atomic.Int32
	pauseFn := func(sessionID string) error {
		pauseCount.Add(1)
		return nil
	}

	poller := NewBudgetPoller(pool, 10*time.Millisecond, pauseFn)
	ctx, cancel := context.WithCancel(context.Background())

	go poller.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	poller.Stop()

	if pauseCount.Load() != 0 {
		t.Fatalf("expected no pauses, got %d", pauseCount.Load())
	}
}

func TestBudgetPoller_PoolCeilingPausesAll(t *testing.T) {
	pool := NewBudgetPool(10.0)
	_ = pool.Allocate("s1", 10.0)
	_ = pool.Allocate("s2", 10.0)

	// Total spend hits the pool ceiling even though per-session allocations allow more.
	pool.Record("s1", 5.0)
	pool.Record("s2", 5.0) // total = 10 = ceiling

	var paused sync.Map
	pauseFn := func(sessionID string) error {
		paused.Store(sessionID, true)
		return nil
	}

	poller := NewBudgetPoller(pool, 10*time.Millisecond, pauseFn)
	ctx, cancel := context.WithCancel(context.Background())

	go poller.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	poller.Stop()

	if _, ok := paused.Load("s1"); !ok {
		t.Fatal("expected s1 to be paused at pool ceiling")
	}
	if _, ok := paused.Load("s2"); !ok {
		t.Fatal("expected s2 to be paused at pool ceiling")
	}
}

func TestBudgetPoller_Stop(t *testing.T) {
	pool := NewBudgetPool(100.0)

	poller := NewBudgetPoller(pool, 10*time.Millisecond, nil)
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		poller.Start(ctx)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	poller.Stop()

	select {
	case <-done:
		// Success: Start returned.
	case <-time.After(time.Second):
		t.Fatal("Stop did not cause Start to return")
	}
}

func TestBudgetPoller_ContextCancellation(t *testing.T) {
	pool := NewBudgetPool(100.0)

	poller := NewBudgetPoller(pool, 10*time.Millisecond, nil)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		poller.Start(ctx)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Success.
	case <-time.After(time.Second):
		t.Fatal("context cancellation did not stop poller")
	}
}

func TestBudgetPoller_NilPauseFnDoesNotPanic(t *testing.T) {
	pool := NewBudgetPool(10.0)
	_ = pool.Allocate("s1", 5.0)
	pool.Record("s1", 6.0) // over allocation

	poller := NewBudgetPoller(pool, 10*time.Millisecond, nil)
	ctx, cancel := context.WithCancel(context.Background())

	go poller.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	poller.Stop()
	// No panic = pass.
}
