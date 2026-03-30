package session

import (
	"errors"
	"sync"
	"testing"
)

func TestBudgetPool_NewBudgetPool(t *testing.T) {
	bp := NewBudgetPool(100.0)
	if bp.TotalCeiling() != 100.0 {
		t.Fatalf("expected ceiling 100.0, got %f", bp.TotalCeiling())
	}
	if bp.Remaining() != 100.0 {
		t.Fatalf("expected remaining 100.0, got %f", bp.Remaining())
	}
}

func TestBudgetPool_AllocateAndRecord(t *testing.T) {
	bp := NewBudgetPool(50.0)

	if err := bp.Allocate("s1", 20.0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := bp.Allocate("s2", 25.0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fail: 20+25+10 = 55 > 50
	err := bp.Allocate("s3", 10.0)
	if !errors.Is(err, ErrBudgetCeiling) {
		t.Fatalf("expected ErrBudgetCeiling, got %v", err)
	}

	// Record spend within allocation.
	bp.Record("s1", 10.0)
	bp.Record("s2", 5.0)

	if bp.Remaining() != 35.0 {
		t.Fatalf("expected remaining 35.0, got %f", bp.Remaining())
	}
}

func TestBudgetPool_ShouldPause_PerSession(t *testing.T) {
	bp := NewBudgetPool(100.0)

	_ = bp.Allocate("s1", 10.0)
	bp.Record("s1", 5.0)

	if bp.ShouldPause("s1") {
		t.Fatal("should not pause at 50% of allocation")
	}

	bp.Record("s1", 5.0) // now at 10.0 = allocation
	if !bp.ShouldPause("s1") {
		t.Fatal("should pause when spend equals allocation")
	}
}

func TestBudgetPool_ShouldPause_PoolCeiling(t *testing.T) {
	bp := NewBudgetPool(10.0)

	_ = bp.Allocate("s1", 5.0)
	_ = bp.Allocate("s2", 5.0)

	bp.Record("s1", 5.0)
	bp.Record("s2", 5.0) // total = 10.0 = ceiling

	// Both sessions should pause at ceiling.
	if !bp.ShouldPause("s1") {
		t.Fatal("s1 should pause at ceiling")
	}
	if !bp.ShouldPause("s2") {
		t.Fatal("s2 should pause at ceiling")
	}
}

func TestBudgetPool_ShouldPause_UnknownSession(t *testing.T) {
	bp := NewBudgetPool(100.0)
	if bp.ShouldPause("unknown") {
		t.Fatal("unknown session should not pause")
	}
}

func TestBudgetPool_RecordWithoutAllocate(t *testing.T) {
	bp := NewBudgetPool(100.0)
	bp.Record("s1", 5.0)

	summary := bp.Summary()
	info, ok := summary.Sessions["s1"]
	if !ok {
		t.Fatal("session s1 should exist in summary")
	}
	if info.Spent != 5.0 {
		t.Fatalf("expected spent 5.0, got %f", info.Spent)
	}
	if info.Allocated != 0 {
		t.Fatalf("expected allocated 0, got %f", info.Allocated)
	}
}

func TestBudgetPool_Summary(t *testing.T) {
	bp := NewBudgetPool(100.0)
	_ = bp.Allocate("s1", 30.0)
	_ = bp.Allocate("s2", 20.0)
	bp.Record("s1", 15.0)
	bp.Record("s2", 10.0)

	summary := bp.Summary()

	if summary.TotalCeiling != 100.0 {
		t.Fatalf("expected ceiling 100.0, got %f", summary.TotalCeiling)
	}
	if summary.TotalSpent != 25.0 {
		t.Fatalf("expected total spent 25.0, got %f", summary.TotalSpent)
	}
	if summary.Remaining != 75.0 {
		t.Fatalf("expected remaining 75.0, got %f", summary.Remaining)
	}
	if len(summary.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(summary.Sessions))
	}
	if summary.Sessions["s1"].Allocated != 30.0 {
		t.Fatalf("expected s1 allocated 30.0, got %f", summary.Sessions["s1"].Allocated)
	}
	if summary.GeneratedAt.IsZero() {
		t.Fatal("expected non-zero GeneratedAt")
	}
}

func TestBudgetPool_AllocateIncremental(t *testing.T) {
	bp := NewBudgetPool(50.0)

	_ = bp.Allocate("s1", 20.0)
	_ = bp.Allocate("s1", 10.0) // add more to same session

	summary := bp.Summary()
	if summary.Sessions["s1"].Allocated != 30.0 {
		t.Fatalf("expected s1 allocated 30.0 after incremental, got %f", summary.Sessions["s1"].Allocated)
	}
}

func TestBudgetPool_ConcurrentAccess(t *testing.T) {
	bp := NewBudgetPool(1000.0)

	var wg sync.WaitGroup
	// Concurrent allocations.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sid := "s" + string(rune('A'+id%26))
			_ = bp.Allocate(sid, 1.0)
		}(i)
	}
	// Concurrent records.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sid := "s" + string(rune('A'+id%26))
			bp.Record(sid, 0.5)
		}(i)
	}
	// Concurrent reads.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = bp.Remaining()
			_ = bp.Summary()
			_ = bp.ShouldPause("sA")
		}()
	}

	wg.Wait()

	// Just verify no panic and remaining is consistent.
	summary := bp.Summary()
	if summary.TotalCeiling != 1000.0 {
		t.Fatalf("ceiling should be unchanged, got %f", summary.TotalCeiling)
	}
}
