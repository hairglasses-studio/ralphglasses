package blackboard

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// SetTTL
// ---------------------------------------------------------------------------

func TestSetTTL_Basic(t *testing.T) {
	bb := NewBlackboard("")

	_ = bb.Put(Entry{Key: "k", Namespace: "ns", Value: map[string]any{"v": 1}})

	// Entry starts with zero TTL.
	e, _ := bb.Get("ns", "k")
	if e.TTL != 0 {
		t.Fatalf("expected zero TTL before SetTTL, got %v", e.TTL)
	}

	ok := bb.SetTTL("ns", "k", 5*time.Second)
	if !ok {
		t.Fatal("SetTTL returned false for existing entry")
	}

	e, _ = bb.Get("ns", "k")
	if e.TTL != 5*time.Second {
		t.Fatalf("expected TTL=5s, got %v", e.TTL)
	}
}

func TestSetTTL_Missing(t *testing.T) {
	bb := NewBlackboard("")

	ok := bb.SetTTL("ns", "no-such-key", time.Second)
	if ok {
		t.Fatal("SetTTL should return false for non-existent entry")
	}
}

func TestSetTTL_ResetsUpdatedAt(t *testing.T) {
	bb := NewBlackboard("")

	_ = bb.Put(Entry{Key: "k", Namespace: "ns", Value: map[string]any{}})
	e1, _ := bb.Get("ns", "k")

	// Small sleep so the timestamps differ.
	time.Sleep(2 * time.Millisecond)

	bb.SetTTL("ns", "k", time.Minute)
	e2, _ := bb.Get("ns", "k")

	if !e2.UpdatedAt.After(e1.UpdatedAt) {
		t.Fatalf("expected UpdatedAt to advance after SetTTL; before=%v after=%v",
			e1.UpdatedAt, e2.UpdatedAt)
	}
}

func TestSetTTL_ExtendsPreventsEviction(t *testing.T) {
	bb := NewBlackboard("")

	// Entry with a very short TTL.
	_ = bb.Put(Entry{
		Key: "k", Namespace: "ns",
		Value: map[string]any{}, TTL: 5 * time.Millisecond,
	})

	// Extend before it expires.
	time.Sleep(2 * time.Millisecond)
	bb.SetTTL("ns", "k", time.Minute)

	// Wait past the original TTL.
	time.Sleep(10 * time.Millisecond)
	bb.GC()

	if _, ok := bb.Get("ns", "k"); !ok {
		t.Fatal("entry should survive GC after TTL extension")
	}
}

// ---------------------------------------------------------------------------
// WithDefaultTTL
// ---------------------------------------------------------------------------

func TestWithDefaultTTL(t *testing.T) {
	bb := NewBlackboard("", WithDefaultTTL(30*time.Second))

	// Entry without explicit TTL should inherit the default.
	_ = bb.Put(Entry{Key: "k1", Namespace: "ns", Value: map[string]any{}})
	e, _ := bb.Get("ns", "k1")
	if e.TTL != 30*time.Second {
		t.Fatalf("expected default TTL 30s, got %v", e.TTL)
	}

	// Entry with explicit TTL should keep its own.
	_ = bb.Put(Entry{Key: "k2", Namespace: "ns", Value: map[string]any{}, TTL: 5 * time.Second})
	e, _ = bb.Get("ns", "k2")
	if e.TTL != 5*time.Second {
		t.Fatalf("expected explicit TTL 5s, got %v", e.TTL)
	}
}

func TestWithDefaultTTL_Eviction(t *testing.T) {
	bb := NewBlackboard("", WithDefaultTTL(5*time.Millisecond))

	_ = bb.Put(Entry{Key: "k", Namespace: "ns", Value: map[string]any{}})

	time.Sleep(10 * time.Millisecond)
	bb.GC()

	if _, ok := bb.Get("ns", "k"); ok {
		t.Fatal("entry with default TTL should be evicted after expiry")
	}
}

// ---------------------------------------------------------------------------
// StartEvictor / StopEvictor
// ---------------------------------------------------------------------------

func TestEvictor_RemovesExpired(t *testing.T) {
	bb := NewBlackboard("")

	_ = bb.Put(Entry{
		Key: "ephemeral", Namespace: "ns",
		Value: map[string]any{}, TTL: 5 * time.Millisecond,
	})
	_ = bb.Put(Entry{
		Key: "permanent", Namespace: "ns",
		Value: map[string]any{},
	})

	bb.StartEvictor(2 * time.Millisecond)
	defer bb.StopEvictor()

	// Wait enough time for the TTL to expire and the evictor to run.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if bb.Len() == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	if bb.Len() != 1 {
		t.Fatalf("expected 1 entry after eviction, got %d", bb.Len())
	}
	if _, ok := bb.Get("ns", "permanent"); !ok {
		t.Fatal("permanent entry should survive eviction")
	}
}

func TestEvictor_StopIsIdempotent(t *testing.T) {
	bb := NewBlackboard("")

	// StopEvictor when none is running should not panic.
	bb.StopEvictor()
	bb.StopEvictor()

	bb.StartEvictor(time.Millisecond)
	bb.StopEvictor()
	bb.StopEvictor() // second stop after already stopped
}

func TestEvictor_StartIsIdempotent(t *testing.T) {
	bb := NewBlackboard("")

	done1 := bb.StartEvictor(time.Millisecond)
	done2 := bb.StartEvictor(time.Millisecond)

	// Both calls should return the same done channel.
	if done1 != done2 {
		t.Fatal("second StartEvictor should return the same done channel")
	}

	bb.StopEvictor()
}

func TestEvictor_FinalGCOnStop(t *testing.T) {
	bb := NewBlackboard("")

	_ = bb.Put(Entry{
		Key: "k", Namespace: "ns",
		Value: map[string]any{}, TTL: time.Millisecond,
	})

	// Use a very long interval so the ticker never fires.
	bb.StartEvictor(time.Hour)

	time.Sleep(5 * time.Millisecond)

	// StopEvictor runs a final GC.
	bb.StopEvictor()

	if _, ok := bb.Get("ns", "k"); ok {
		t.Fatal("expired entry should be removed by final GC on stop")
	}
}

func TestEvictor_DoneChannelClosedAfterStop(t *testing.T) {
	bb := NewBlackboard("")

	done := bb.StartEvictor(time.Millisecond)
	bb.StopEvictor()

	// done should be closed; reading from it should succeed immediately.
	select {
	case <-done:
		// expected
	case <-time.After(time.Second):
		t.Fatal("done channel was not closed after StopEvictor")
	}
}

func TestEvictor_RestartAfterStop(t *testing.T) {
	bb := NewBlackboard("")

	bb.StartEvictor(time.Millisecond)
	bb.StopEvictor()

	// Should be able to start again.
	_ = bb.Put(Entry{
		Key: "k", Namespace: "ns",
		Value: map[string]any{}, TTL: 5 * time.Millisecond,
	})

	bb.StartEvictor(2 * time.Millisecond)
	defer bb.StopEvictor()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if bb.Len() == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	if bb.Len() != 0 {
		t.Fatalf("expected 0 entries after restart eviction, got %d", bb.Len())
	}
}

// ---------------------------------------------------------------------------
// Concurrent access safety
// ---------------------------------------------------------------------------

func TestEvictor_ConcurrentSetTTLAndGC(t *testing.T) {
	bb := NewBlackboard("")

	const n = 50
	for i := 0; i < n; i++ {
		_ = bb.Put(Entry{
			Key:       fmt.Sprintf("k-%d", i),
			Namespace: "ns",
			Value:     map[string]any{"i": i},
			TTL:       100 * time.Millisecond,
		})
	}

	bb.StartEvictor(time.Millisecond)
	defer bb.StopEvictor()

	var wg sync.WaitGroup

	// Writers continuously extend TTLs.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				key := fmt.Sprintf("k-%d", j%n)
				bb.SetTTL("ns", key, time.Minute)
			}
		}(i)
	}

	// Readers continuously query.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = bb.Query("ns")
				_, _ = bb.Get("ns", fmt.Sprintf("k-%d", j%n))
			}
		}()
	}

	wg.Wait()
}

func TestEvictor_ConcurrentPutAndEviction(t *testing.T) {
	bb := NewBlackboard("", WithDefaultTTL(2*time.Millisecond))

	bb.StartEvictor(time.Millisecond)
	defer bb.StopEvictor()

	var wg sync.WaitGroup

	// Writers continuously add short-lived entries.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = bb.Put(Entry{
					Key:       fmt.Sprintf("k-%d-%d", id, j),
					Namespace: "ns",
					Value:     map[string]any{"v": j},
					WriterID:  fmt.Sprintf("w%d", id),
				})
			}
		}(i)
	}

	// Readers continuously query.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = bb.Query("ns")
				_ = bb.Len()
			}
		}()
	}

	wg.Wait()

	// After all writers stop, wait for eviction to clean everything up.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if bb.Len() == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}

	// It is acceptable if a few entries remain (timing), but most should be gone.
	remaining := bb.Len()
	if remaining > 50 {
		t.Fatalf("expected most entries evicted, but %d remain", remaining)
	}
}
