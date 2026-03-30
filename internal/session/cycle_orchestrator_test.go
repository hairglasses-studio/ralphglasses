package session

import (
	"fmt"
	"sync"
	"testing"
)

func TestCycleOrchestrator_Create(t *testing.T) {
	orch := NewCycleOrchestrator(5, t.TempDir())

	cs, err := orch.Create("cycle-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cs.ID != "cycle-1" {
		t.Errorf("expected ID %q, got %q", "cycle-1", cs.ID)
	}
	if cs.Phase != "proposed" {
		t.Errorf("expected phase %q, got %q", "proposed", cs.Phase)
	}
	if cs.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if cs.Metrics == nil {
		t.Error("Metrics map should be initialized")
	}
	if cs.Attempts != 0 {
		t.Errorf("expected 0 attempts, got %d", cs.Attempts)
	}

	// Duplicate ID should fail.
	_, err = orch.Create("cycle-1")
	if err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestCycleOrchestrator_Advance_FullLifecycle(t *testing.T) {
	orch := NewCycleOrchestrator(5, t.TempDir())

	_, err := orch.Create("lifecycle")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	expectedPhases := []string{"baselining", "improving", "validating", "complete"}
	for _, expected := range expectedPhases {
		if err := orch.Advance("lifecycle"); err != nil {
			t.Fatalf("advance to %s: %v", expected, err)
		}
		cs, ok := orch.Get("lifecycle")
		if !ok {
			t.Fatal("cycle not found after advance")
		}
		if cs.Phase != expected {
			t.Errorf("expected phase %q, got %q", expected, cs.Phase)
		}
	}

	// Verify attempts incremented for each advance.
	cs, _ := orch.Get("lifecycle")
	if cs.Attempts != 4 {
		t.Errorf("expected 4 attempts, got %d", cs.Attempts)
	}

	// Advancing from complete should fail.
	if err := orch.Advance("lifecycle"); err == nil {
		t.Fatal("expected error when advancing from complete")
	}

	// Advancing unknown cycle should fail.
	if err := orch.Advance("nonexistent"); err == nil {
		t.Fatal("expected error for unknown cycle")
	}
}

func TestCycleOrchestrator_Fail(t *testing.T) {
	orch := NewCycleOrchestrator(5, t.TempDir())

	_, err := orch.Create("fail-cycle")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Advance once so we're not in proposed.
	if err := orch.Advance("fail-cycle"); err != nil {
		t.Fatalf("advance: %v", err)
	}

	if err := orch.Fail("fail-cycle", "something broke"); err != nil {
		t.Fatalf("fail: %v", err)
	}

	cs, ok := orch.Get("fail-cycle")
	if !ok {
		t.Fatal("cycle not found after fail")
	}
	if cs.Phase != "failed" {
		t.Errorf("expected phase %q, got %q", "failed", cs.Phase)
	}
	if cs.Error != "something broke" {
		t.Errorf("expected error %q, got %q", "something broke", cs.Error)
	}

	// Failing again from terminal should error.
	if err := orch.Fail("fail-cycle", "double fail"); err == nil {
		t.Fatal("expected error when failing from terminal phase")
	}

	// Failing unknown cycle should error.
	if err := orch.Fail("nonexistent", "nope"); err == nil {
		t.Fatal("expected error for unknown cycle")
	}

	// Fail from proposed phase should work.
	orch.Create("fail-proposed")
	if err := orch.Fail("fail-proposed", "nope"); err != nil {
		t.Fatalf("fail from proposed: %v", err)
	}
}

func TestCycleOrchestrator_MaxConcurrent(t *testing.T) {
	orch := NewCycleOrchestrator(2, t.TempDir())

	if _, err := orch.Create("c1"); err != nil {
		t.Fatalf("create c1: %v", err)
	}
	if _, err := orch.Create("c2"); err != nil {
		t.Fatalf("create c2: %v", err)
	}

	// Third should fail — both c1 and c2 are active (proposed).
	if _, err := orch.Create("c3"); err == nil {
		t.Fatal("expected error when exceeding maxConcurrent")
	}

	// Complete c1 to free a slot.
	for i := 0; i < 4; i++ {
		if err := orch.Advance("c1"); err != nil {
			t.Fatalf("advance c1 step %d: %v", i, err)
		}
	}
	cs, _ := orch.Get("c1")
	if cs.Phase != "complete" {
		t.Fatalf("expected c1 complete, got %s", cs.Phase)
	}

	// Now c3 should succeed.
	if _, err := orch.Create("c3"); err != nil {
		t.Fatalf("create c3 after freeing slot: %v", err)
	}

	// Fail c2 to free another slot.
	if err := orch.Fail("c2", "cancelled"); err != nil {
		t.Fatalf("fail c2: %v", err)
	}

	if _, err := orch.Create("c4"); err != nil {
		t.Fatalf("create c4 after failing c2: %v", err)
	}

	if orch.ActiveCount() != 2 {
		t.Errorf("expected 2 active, got %d", orch.ActiveCount())
	}
}

func TestCycleOrchestrator_List(t *testing.T) {
	orch := NewCycleOrchestrator(10, t.TempDir())

	// Create several cycles.
	ids := []string{"alpha", "bravo", "charlie"}
	for _, id := range ids {
		if _, err := orch.Create(id); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}

	list := orch.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 cycles, got %d", len(list))
	}

	// Verify all IDs present.
	found := make(map[string]bool)
	for _, cs := range list {
		found[cs.ID] = true
	}
	for _, id := range ids {
		if !found[id] {
			t.Errorf("cycle %q not found in list", id)
		}
	}

	// List on empty orchestrator.
	empty := NewCycleOrchestrator(5, t.TempDir())
	if len(empty.List()) != 0 {
		t.Error("expected empty list")
	}
}

func TestCycleOrchestrator_ConcurrentAccess(t *testing.T) {
	orch := NewCycleOrchestrator(100, t.TempDir())

	var wg sync.WaitGroup
	errs := make(chan error, 200)

	// Spawn 50 goroutines creating cycles.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("concurrent-%d", idx)
			if _, err := orch.Create(id); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()

	// Spawn goroutines doing reads and advances concurrently.
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("concurrent-%d", idx)
			_ = orch.Advance(id)
		}(i)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("concurrent-%d", idx)
			orch.Get(id)
		}(i)
		go func() {
			defer wg.Done()
			orch.List()
		}()
	}
	wg.Wait()

	close(errs)
	for err := range errs {
		t.Errorf("concurrent error: %v", err)
	}

	// All 50 cycles should exist.
	if len(orch.List()) != 50 {
		t.Errorf("expected 50 cycles, got %d", len(orch.List()))
	}
}

