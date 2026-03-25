package session

import (
	"context"
	"testing"
	"time"
)

func TestWaitForSessionDoneCh(t *testing.T) {
	m := NewManager()
	s := &Session{
		ID:     "test-done",
		Status: StatusRunning,
		doneCh: make(chan struct{}),
	}

	// Close doneCh after 50ms — simulates process exit without setting terminal status.
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(s.doneCh)
	}()

	ctx := context.Background()
	start := time.Now()
	err := m.waitForSession(ctx, s)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error when process exits without terminal status, got nil")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("waitForSession took %s, expected < 500ms", elapsed)
	}
	t.Logf("returned in %s with error: %v", elapsed, err)
}

func TestWaitForSessionTimeout(t *testing.T) {
	m := NewManager()
	m.SessionTimeout = 200 * time.Millisecond

	s := &Session{
		ID:     "test-timeout",
		Status: StatusRunning,
		doneCh: make(chan struct{}), // never closed
	}

	ctx := context.Background()
	start := time.Now()
	err := m.waitForSession(ctx, s)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 1*time.Second {
		t.Fatalf("waitForSession took %s, expected ~200ms", elapsed)
	}
	if elapsed < 150*time.Millisecond {
		t.Fatalf("waitForSession returned too fast (%s), expected ~200ms", elapsed)
	}
	t.Logf("timed out in %s with error: %v", elapsed, err)
}

func TestWaitForSessionContextCancel(t *testing.T) {
	m := NewManager()

	s := &Session{
		ID:     "test-cancel",
		Status: StatusRunning,
		doneCh: make(chan struct{}), // never closed
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := m.waitForSession(ctx, s)
	elapsed := time.Since(start)

	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("waitForSession took %s, expected < 500ms", elapsed)
	}
	t.Logf("cancelled in %s", elapsed)
}

func TestWaitForSessionCompleted(t *testing.T) {
	m := NewManager()

	doneCh := make(chan struct{})
	close(doneCh)

	s := &Session{
		ID:     "test-completed",
		Status: StatusCompleted,
		doneCh: doneCh,
	}

	ctx := context.Background()
	start := time.Now()
	err := m.waitForSession(ctx, s)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected nil error for completed session, got: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("waitForSession took %s, expected fast return", elapsed)
	}
	t.Logf("returned in %s", elapsed)
}

func TestWaitForSessionNilDoneCh(t *testing.T) {
	// Sessions without a real process (e.g. test stubs) have doneCh == nil.
	// waitForSession should still work via ticker polling.
	m := NewManager()
	m.SessionTimeout = 500 * time.Millisecond

	s := &Session{
		ID:     "test-nil-done",
		Status: StatusRunning,
		// doneCh intentionally nil
	}

	// Set completed after 100ms.
	go func() {
		time.Sleep(100 * time.Millisecond)
		s.Lock()
		s.Status = StatusCompleted
		s.Unlock()
	}()

	ctx := context.Background()
	err := m.waitForSession(ctx, s)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}
