package session

import (
	"sync"
	"testing"
)

func TestNoOpDetector_SingleNoOpDoesNotSkip(t *testing.T) {
	d := NewNoOpDetector(2)
	skip, _ := d.RecordIteration("loop1", 0, 0)
	if skip {
		t.Fatal("single no-op should not trigger skip")
	}
}

func TestNoOpDetector_ConsecutiveNoOpsTriggersSkip(t *testing.T) {
	d := NewNoOpDetector(3)
	for i := 0; i < 2; i++ {
		skip, _ := d.RecordIteration("loop1", 0, 0)
		if skip {
			t.Fatalf("should not skip at count %d (threshold 3)", i+1)
		}
	}
	skip, reason := d.RecordIteration("loop1", 0, 0)
	if !skip {
		t.Fatal("expected skip after 3 consecutive no-ops")
	}
	if reason == "" {
		t.Fatal("expected non-empty reason")
	}
}

func TestNoOpDetector_ProductiveIterationResetsCounter(t *testing.T) {
	d := NewNoOpDetector(2)
	d.RecordIteration("loop1", 0, 0) // count=1
	d.RecordIteration("loop1", 3, 10) // productive -> reset
	skip, _ := d.RecordIteration("loop1", 0, 0) // count=1 again
	if skip {
		t.Fatal("productive iteration should have reset counter")
	}
	if d.ConsecutiveCount("loop1") != 1 {
		t.Fatalf("expected count 1 after reset+noop, got %d", d.ConsecutiveCount("loop1"))
	}
}

func TestNoOpDetector_ResetClearsCount(t *testing.T) {
	d := NewNoOpDetector(2)
	d.RecordIteration("loop1", 0, 0)
	d.Reset("loop1")
	if d.ConsecutiveCount("loop1") != 0 {
		t.Fatalf("expected 0 after Reset, got %d", d.ConsecutiveCount("loop1"))
	}
}

func TestNoOpDetector_IndependentLoopIDs(t *testing.T) {
	d := NewNoOpDetector(2)
	d.RecordIteration("loop1", 0, 0)
	d.RecordIteration("loop2", 0, 0)
	d.RecordIteration("loop1", 0, 0) // loop1 count=2 -> skip

	skip1, _ := d.RecordIteration("loop1", 0, 0)
	skip2, _ := d.RecordIteration("loop2", 0, 0) // loop2 count=2 -> skip

	if !skip1 {
		t.Fatal("loop1 should skip at count 3")
	}
	if !skip2 {
		t.Fatal("loop2 should skip at count 2")
	}
}

func TestNoOpDetector_ConcurrentAccess(t *testing.T) {
	d := NewNoOpDetector(100) // high threshold so we don't trigger skip
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			loopID := "loop1"
			if id%2 == 0 {
				loopID = "loop2"
			}
			d.RecordIteration(loopID, 0, 0)
			d.ConsecutiveCount(loopID)
		}(i)
	}
	wg.Wait()
	// No panic = pass. Verify counts are reasonable.
	c1 := d.ConsecutiveCount("loop1")
	c2 := d.ConsecutiveCount("loop2")
	if c1+c2 != 50 {
		t.Fatalf("expected 50 total no-ops, got %d+%d=%d", c1, c2, c1+c2)
	}
}

func TestNoOpDetector_DefaultThreshold(t *testing.T) {
	d := NewNoOpDetector(0)
	if d.MaxConsecutiveNoOps != 2 {
		t.Fatalf("expected default threshold 2, got %d", d.MaxConsecutiveNoOps)
	}
}

func TestNoOpDetector_LinesAddedCountsAsProductive(t *testing.T) {
	d := NewNoOpDetector(2)
	d.RecordIteration("loop1", 0, 0) // noop
	d.RecordIteration("loop1", 0, 5) // lines added but no files changed -> productive
	if d.ConsecutiveCount("loop1") != 0 {
		t.Fatal("lines added should count as productive and reset counter")
	}
}
