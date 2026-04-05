package gateway

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSemaphore_BasicAcquireRelease(t *testing.T) {
	s := NewSemaphore(2)
	if s.Max() != 2 {
		t.Fatalf("expected max 2, got %d", s.Max())
	}
	if s.Available() != 2 {
		t.Fatalf("expected 2 available, got %d", s.Available())
	}

	if err := s.Acquire(context.Background()); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if s.Available() != 1 {
		t.Fatalf("expected 1 available after acquire, got %d", s.Available())
	}
	s.Release()
	if s.Available() != 2 {
		t.Fatalf("expected 2 available after release, got %d", s.Available())
	}
}

func TestSemaphore_DefaultMax(t *testing.T) {
	s := NewSemaphore(0)
	if s.Max() != DefaultSemaphoreMax {
		t.Fatalf("expected default max %d, got %d", DefaultSemaphoreMax, s.Max())
	}
}

func TestSemaphore_TryAcquire(t *testing.T) {
	s := NewSemaphore(1)
	if err := s.TryAcquire(); err != nil {
		t.Fatalf("first TryAcquire should succeed: %v", err)
	}
	if err := s.TryAcquire(); !errors.Is(err, ErrSemaphoreExhausted) {
		t.Fatalf("second TryAcquire should fail: %v", err)
	}
	s.Release()
	if err := s.TryAcquire(); err != nil {
		t.Fatalf("TryAcquire after release should succeed: %v", err)
	}
	s.Release()
}

func TestSemaphore_ContextCancellation(t *testing.T) {
	s := NewSemaphore(1)
	s.Acquire(context.Background()) //nolint:errcheck // hold the only permit

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.Acquire(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	s.Release()
}

func TestSemaphore_ContextTimeout(t *testing.T) {
	s := NewSemaphore(1)
	s.Acquire(context.Background()) //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := s.Acquire(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	s.Release()
}

func TestSemaphore_ConcurrencyLimit(t *testing.T) {
	const maxConcurrency = 4
	s := NewSemaphore(maxConcurrency)

	var active int32
	var maxObserved int32
	var wg sync.WaitGroup

	for range 20 {
		wg.Go(func() {
			s.Acquire(context.Background()) //nolint:errcheck
			defer s.Release()

			cur := atomic.AddInt32(&active, 1)
			for {
				old := atomic.LoadInt32(&maxObserved)
				if cur <= old || atomic.CompareAndSwapInt32(&maxObserved, old, cur) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&active, -1)
		})
	}
	wg.Wait()

	if m := atomic.LoadInt32(&maxObserved); m > maxConcurrency {
		t.Fatalf("max observed concurrency %d exceeds limit %d", m, maxConcurrency)
	}
}

func TestSemaphore_Available_AfterPartialAcquire(t *testing.T) {
	s := NewSemaphore(5)
	for range 3 {
		s.Acquire(context.Background()) //nolint:errcheck
	}
	if got := s.Available(); got != 2 {
		t.Fatalf("expected 2 available, got %d", got)
	}
	for range 3 {
		s.Release()
	}
}
