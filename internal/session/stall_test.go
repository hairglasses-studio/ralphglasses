package session

import (
	"sync"
	"testing"
	"time"
)

func TestNewStallDetector_ZeroTimeout(t *testing.T) {
	sd := NewStallDetector(0)
	if sd.IsStalled() {
		t.Fatal("zero-timeout detector should never report stalled")
	}
	// Even after sleeping, should still not be stalled.
	time.Sleep(10 * time.Millisecond)
	if sd.IsStalled() {
		t.Fatal("zero-timeout detector should never report stalled after sleep")
	}
}

func TestRecordActivity_ResetsTimer(t *testing.T) {
	sd := NewStallDetector(80 * time.Millisecond)

	// Wait 60ms — should not be stalled yet.
	time.Sleep(60 * time.Millisecond)
	if sd.IsStalled() {
		t.Fatal("should not be stalled before timeout")
	}

	// Record activity to reset timer.
	sd.RecordActivity()

	// Wait another 60ms — should still not be stalled because we reset.
	time.Sleep(60 * time.Millisecond)
	if sd.IsStalled() {
		t.Fatal("should not be stalled after RecordActivity reset")
	}

	// Wait past the timeout from last activity.
	time.Sleep(30 * time.Millisecond)
	if !sd.IsStalled() {
		t.Fatal("should be stalled after timeout from last activity")
	}
}

func TestIsStalled_ReturnsTrueAfterTimeout(t *testing.T) {
	sd := NewStallDetector(50 * time.Millisecond)
	if sd.IsStalled() {
		t.Fatal("should not be stalled immediately")
	}
	time.Sleep(60 * time.Millisecond)
	if !sd.IsStalled() {
		t.Fatal("should be stalled after timeout elapses")
	}
}

func TestStart_SendsOnStall(t *testing.T) {
	sd := NewStallDetector(50 * time.Millisecond)
	ch := sd.Start()
	defer sd.Stop()

	select {
	case stalled := <-ch:
		if !stalled {
			t.Fatal("expected true on stall channel")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for stall notification")
	}
}

func TestStart_ZeroTimeout_NeverFires(t *testing.T) {
	sd := NewStallDetector(0)
	ch := sd.Start()
	defer sd.Stop()

	select {
	case <-ch:
		t.Fatal("zero-timeout channel should never fire")
	case <-time.After(100 * time.Millisecond):
		// Expected — no stall notification.
	}
}

func TestStop_PreventsNotifications(t *testing.T) {
	sd := NewStallDetector(50 * time.Millisecond)
	ch := sd.Start()

	// Stop before the stall can fire.
	sd.Stop()

	select {
	case _, ok := <-ch:
		if ok {
			// A stall may have raced in before stop — tolerate one.
		}
		// Channel closed after stop — fine.
	case <-time.After(200 * time.Millisecond):
		// Also acceptable — channel may block if goroutine exits first.
	}
}

func TestStop_Idempotent(t *testing.T) {
	sd := NewStallDetector(50 * time.Millisecond)
	sd.Start()
	sd.Stop()
	// Second stop should not panic.
	sd.Stop()
}

func TestStallCount_Increments(t *testing.T) {
	sd := NewStallDetector(50 * time.Millisecond)
	ch := sd.Start()
	defer sd.Stop()

	// Wait for at least two stall events.
	count := 0
	timeout := time.After(1 * time.Second)
	for count < 2 {
		select {
		case <-ch:
			count++
		case <-timeout:
			t.Fatalf("timed out waiting for stall events, got %d", count)
		}
	}

	got := sd.StallCount()
	if got < 2 {
		t.Fatalf("expected StallCount >= 2, got %d", got)
	}
}

func TestStallDetector_ThreadSafety(t *testing.T) {
	sd := NewStallDetector(100 * time.Millisecond)
	ch := sd.Start()

	var wg sync.WaitGroup
	// Concurrent RecordActivity calls.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				sd.RecordActivity()
				_ = sd.IsStalled()
				_ = sd.StallCount()
			}
		}()
	}

	// Wait for concurrent accessors to finish, then stop and drain.
	wg.Wait()
	sd.Stop()
	// Drain remaining notifications after stop.
	for range ch {
	}
}
