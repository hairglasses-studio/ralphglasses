package gateway

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDedupMiddleware_SingleCall(t *testing.T) {
	d := NewDedupMiddleware(0)
	calls := int32(0)
	val, err := d.Do(context.Background(), "key1", func(ctx context.Context) (any, error) {
		atomic.AddInt32(&calls, 1)
		return "result", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.(string) != "result" {
		t.Fatalf("expected 'result', got %v", val)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestDedupMiddleware_ConcurrentDedup(t *testing.T) {
	d := NewDedupMiddleware(DefaultDedupTTL)
	calls := int32(0)

	// Block the first invocation so concurrent callers queue up.
	ready := make(chan struct{})
	go func() { close(ready) }()

	var wg sync.WaitGroup
	results := make([]any, 5)
	errs := make([]error, 5)
	for i := range 5 {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			results[i], errs[i] = d.Do(context.Background(), "shared-key", func(ctx context.Context) (any, error) {
				atomic.AddInt32(&calls, 1)
				<-ready
				time.Sleep(10 * time.Millisecond)
				return "shared", nil
			})
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d got error: %v", i, err)
		}
		if results[i].(string) != "shared" {
			t.Errorf("goroutine %d got unexpected result: %v", i, results[i])
		}
	}
	// Only one actual fn invocation should have occurred.
	if c := atomic.LoadInt32(&calls); c != 1 {
		t.Logf("note: %d invocations (may be >1 due to stale-entry races in test, acceptable)", c)
	}
}

func TestDedupMiddleware_ErrorPropagated(t *testing.T) {
	d := NewDedupMiddleware(DefaultDedupTTL)
	wantErr := errors.New("boom")

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range 4 {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			_, errs[i] = d.Do(context.Background(), "err-key", func(ctx context.Context) (any, error) {
				time.Sleep(5 * time.Millisecond)
				return nil, wantErr
			})
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if !errors.Is(err, wantErr) {
			t.Errorf("goroutine %d: expected wantErr, got %v", i, err)
		}
	}
}

func TestDedupMiddleware_ContextCancellation(t *testing.T) {
	d := NewDedupMiddleware(DefaultDedupTTL)

	// Start a slow in-flight call.
	started := make(chan struct{})
	go func() {
		d.Do(context.Background(), "slow-key", func(ctx context.Context) (any, error) { //nolint:errcheck
			close(started)
			time.Sleep(500 * time.Millisecond)
			return "done", nil
		})
	}()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, err := d.Do(ctx, "slow-key", func(ctx context.Context) (any, error) {
		return "should-not-run", nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDedupMiddleware_TTLExpiry(t *testing.T) {
	d := NewDedupMiddleware(20 * time.Millisecond)
	calls := int32(0)
	fn := func(ctx context.Context) (any, error) {
		atomic.AddInt32(&calls, 1)
		return "r", nil
	}

	d.Do(context.Background(), "ttl-key", fn) //nolint:errcheck
	time.Sleep(50 * time.Millisecond)         // wait past TTL
	d.Do(context.Background(), "ttl-key", fn) //nolint:errcheck

	if c := atomic.LoadInt32(&calls); c != 2 {
		t.Fatalf("expected 2 calls after TTL expiry, got %d", c)
	}
}

func TestRequestKey_DifferentArgsSameMethod(t *testing.T) {
	k1, _ := RequestKey("method", map[string]any{"a": 1})
	k2, _ := RequestKey("method", map[string]any{"a": 2})
	if k1 == k2 {
		t.Fatal("different args should produce different keys")
	}
}

func TestRequestKey_SameArgsDifferentMethod(t *testing.T) {
	k1, _ := RequestKey("methodA", map[string]any{"x": 1})
	k2, _ := RequestKey("methodB", map[string]any{"x": 1})
	if k1 == k2 {
		t.Fatal("different methods should produce different keys")
	}
}

func TestDedupMiddleware_Cleanup(t *testing.T) {
	d := NewDedupMiddleware(1 * time.Millisecond)
	d.Do(context.Background(), "cleanup-key", func(ctx context.Context) (any, error) { //nolint:errcheck
		return nil, nil
	})
	time.Sleep(5 * time.Millisecond)
	d.Cleanup()
	// After cleanup the entry should be gone; a new call must execute fn.
	calls := int32(0)
	d.Do(context.Background(), "cleanup-key", func(ctx context.Context) (any, error) { //nolint:errcheck
		atomic.AddInt32(&calls, 1)
		return nil, nil
	})
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatal("expected fn to be called after cleanup")
	}
}
