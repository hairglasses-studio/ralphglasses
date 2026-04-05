package session

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestFederatedBudget_AllocateAndRemaining(t *testing.T) {
	fb := NewFederatedBudget(100.0)

	if err := fb.Allocate("s1", 30); err != nil {
		t.Fatalf("Allocate s1: %v", err)
	}
	if err := fb.Allocate("s2", 50); err != nil {
		t.Fatalf("Allocate s2: %v", err)
	}

	if got := fb.Remaining("s1"); got != 30 {
		t.Errorf("s1 remaining = %.2f, want 30", got)
	}
	if got := fb.Remaining("s2"); got != 50 {
		t.Errorf("s2 remaining = %.2f, want 50", got)
	}
	if got := fb.TotalRemaining(); got != 100 {
		t.Errorf("total remaining = %.2f, want 100", got)
	}
}

func TestFederatedBudget_AllocateExceedsCeiling(t *testing.T) {
	fb := NewFederatedBudget(50.0)

	if err := fb.Allocate("s1", 30); err != nil {
		t.Fatalf("Allocate s1: %v", err)
	}
	err := fb.Allocate("s2", 25)
	if !errors.Is(err, ErrFederationBudgetExceeded) {
		t.Errorf("expected ErrFederationBudgetExceeded, got %v", err)
	}
}

func TestFederatedBudget_AllocateNegative(t *testing.T) {
	fb := NewFederatedBudget(100.0)
	err := fb.Allocate("s1", -5)
	if err == nil {
		t.Fatal("expected error for negative allocation")
	}
}

func TestFederatedBudget_SpendAndRemaining(t *testing.T) {
	fb := NewFederatedBudget(100.0)
	_ = fb.Allocate("s1", 40)

	if err := fb.Spend("s1", 10); err != nil {
		t.Fatalf("Spend: %v", err)
	}

	if got := fb.Remaining("s1"); got != 30 {
		t.Errorf("s1 remaining = %.2f, want 30", got)
	}
	if got := fb.TotalRemaining(); got != 90 {
		t.Errorf("total remaining = %.2f, want 90", got)
	}
}

func TestFederatedBudget_SpendUnknownSession(t *testing.T) {
	fb := NewFederatedBudget(100.0)
	err := fb.Spend("nope", 5)
	if !errors.Is(err, ErrFederationSessionNotFound) {
		t.Errorf("expected ErrFederationSessionNotFound, got %v", err)
	}
}

func TestFederatedBudget_SpendNegative(t *testing.T) {
	fb := NewFederatedBudget(100.0)
	_ = fb.Allocate("s1", 40)
	err := fb.Spend("s1", -1)
	if err == nil {
		t.Fatal("expected error for negative spend")
	}
}

func TestFederatedBudget_SpendExceedsAllocation(t *testing.T) {
	fb := NewFederatedBudget(100.0)
	_ = fb.Allocate("s1", 10)

	// Spend exactly at limit should succeed.
	if err := fb.Spend("s1", 10); err != nil {
		t.Fatalf("Spend at limit: %v", err)
	}

	// Spend beyond should return exhausted error.
	err := fb.Spend("s1", 1)
	if !errors.Is(err, ErrFederationSessionExhausted) {
		t.Errorf("expected ErrFederationSessionExhausted, got %v", err)
	}
}

func TestFederatedBudget_OnBudgetExhausted(t *testing.T) {
	fb := NewFederatedBudget(100.0)
	_ = fb.Allocate("s1", 10)

	var exhaustedID string
	fb.OnBudgetExhausted(func(id string) {
		exhaustedID = id
	})

	// Spend to exactly exhaust.
	_ = fb.Spend("s1", 10)

	if exhaustedID != "s1" {
		t.Errorf("callback got session %q, want s1", exhaustedID)
	}
}

func TestFederatedBudget_OnBudgetExhausted_Overspend(t *testing.T) {
	fb := NewFederatedBudget(100.0)
	_ = fb.Allocate("s1", 5)

	var called int32
	fb.OnBudgetExhausted(func(_ string) {
		atomic.AddInt32(&called, 1)
	})

	// Overspend in a single call.
	_ = fb.Spend("s1", 10)

	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("expected callback to fire once, got %d", atomic.LoadInt32(&called))
	}
}

func TestFederatedBudget_Redistribute(t *testing.T) {
	fb := NewFederatedBudget(100.0)
	_ = fb.Allocate("s1", 40)
	_ = fb.Allocate("s2", 40)

	// s1 spends 10 of 40, then finishes.
	_ = fb.Spend("s1", 10)
	_ = fb.FinishSession("s1")

	if err := fb.Redistribute(); err != nil {
		t.Fatalf("Redistribute: %v", err)
	}

	// s1 had 30 unspent, redistributed to s2.
	if got := fb.Remaining("s2"); got != 70 {
		t.Errorf("s2 remaining after redistribute = %.2f, want 70", got)
	}

	// s1's allocation should now equal its spend.
	if got := fb.Remaining("s1"); got != 0 {
		t.Errorf("s1 remaining after redistribute = %.2f, want 0", got)
	}
}

func TestFederatedBudget_RedistributeMultipleActive(t *testing.T) {
	fb := NewFederatedBudget(100.0)
	_ = fb.Allocate("s1", 30)
	_ = fb.Allocate("s2", 30)
	_ = fb.Allocate("s3", 30)

	_ = fb.Spend("s1", 10)
	_ = fb.FinishSession("s1")

	if err := fb.Redistribute(); err != nil {
		t.Fatalf("Redistribute: %v", err)
	}

	// 20 unspent from s1, split between s2 and s3.
	if got := fb.Remaining("s2"); got != 40 {
		t.Errorf("s2 remaining = %.2f, want 40", got)
	}
	if got := fb.Remaining("s3"); got != 40 {
		t.Errorf("s3 remaining = %.2f, want 40", got)
	}
}

func TestFederatedBudget_RedistributeNoActive(t *testing.T) {
	fb := NewFederatedBudget(100.0)
	_ = fb.Allocate("s1", 50)
	_ = fb.Spend("s1", 10)
	_ = fb.FinishSession("s1")

	err := fb.Redistribute()
	if !errors.Is(err, ErrFederationSessionNotFound) {
		t.Errorf("expected ErrFederationSessionNotFound, got %v", err)
	}
}

func TestFederatedBudget_RedistributeNoSurplus(t *testing.T) {
	fb := NewFederatedBudget(100.0)
	_ = fb.Allocate("s1", 50)
	_ = fb.Allocate("s2", 50)

	// s1 spends all and finishes.
	_ = fb.Spend("s1", 50)
	_ = fb.FinishSession("s1")

	if err := fb.Redistribute(); err != nil {
		t.Fatalf("Redistribute with no surplus: %v", err)
	}

	// s2 should be unchanged.
	if got := fb.Remaining("s2"); got != 50 {
		t.Errorf("s2 remaining = %.2f, want 50", got)
	}
}

func TestFederatedBudget_FinishUnknownSession(t *testing.T) {
	fb := NewFederatedBudget(100.0)
	err := fb.FinishSession("nope")
	if !errors.Is(err, ErrFederationSessionNotFound) {
		t.Errorf("expected ErrFederationSessionNotFound, got %v", err)
	}
}

func TestFederatedBudget_Summary(t *testing.T) {
	fb := NewFederatedBudget(100.0, WithReservePercent(0.10))
	_ = fb.Allocate("s1", 40)
	_ = fb.Allocate("s2", 30)
	_ = fb.Spend("s1", 15)
	_ = fb.Spend("s2", 5)
	_ = fb.FinishSession("s1")

	s := fb.Summary()
	if s.TotalBudget != 100 {
		t.Errorf("TotalBudget = %.2f, want 100", s.TotalBudget)
	}
	if s.TotalAllocated != 70 {
		t.Errorf("TotalAllocated = %.2f, want 70", s.TotalAllocated)
	}
	if s.TotalSpent != 20 {
		t.Errorf("TotalSpent = %.2f, want 20", s.TotalSpent)
	}
	if s.TotalRemaining != 80 {
		t.Errorf("TotalRemaining = %.2f, want 80", s.TotalRemaining)
	}
	if s.ReservePct != 0.10 {
		t.Errorf("ReservePct = %.2f, want 0.10", s.ReservePct)
	}
	if len(s.Sessions) != 2 {
		t.Fatalf("Sessions count = %d, want 2", len(s.Sessions))
	}
	if !s.Sessions["s1"].Finished {
		t.Error("s1 should be finished")
	}
	if s.Sessions["s2"].Finished {
		t.Error("s2 should not be finished")
	}
	if s.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should not be zero")
	}
}

func TestFederatedBudget_WithReservePercent(t *testing.T) {
	fb := NewFederatedBudget(100.0, WithReservePercent(0.20))

	// Allocatable is 80 (100 * 0.80).
	if err := fb.Allocate("s1", 80); err != nil {
		t.Fatalf("Allocate 80: %v", err)
	}
	err := fb.Allocate("s2", 1)
	if !errors.Is(err, ErrFederationBudgetExceeded) {
		t.Errorf("expected ErrFederationBudgetExceeded, got %v", err)
	}
}

func TestFederatedBudget_RemainingUnknownSession(t *testing.T) {
	fb := NewFederatedBudget(100.0)
	if got := fb.Remaining("nope"); got != 0 {
		t.Errorf("Remaining for unknown = %.2f, want 0", got)
	}
}

func TestFederatedBudget_AdditiveAllocations(t *testing.T) {
	fb := NewFederatedBudget(100.0)
	_ = fb.Allocate("s1", 20)
	_ = fb.Allocate("s1", 10)

	if got := fb.Remaining("s1"); got != 30 {
		t.Errorf("s1 remaining after two allocations = %.2f, want 30", got)
	}
}

func TestFederatedBudget_ConcurrentAccess(t *testing.T) {
	fb := NewFederatedBudget(1000.0)

	// Pre-allocate sessions.
	for i := range 10 {
		id := fmt.Sprintf("s%d", i)
		if err := fb.Allocate(id, 100); err != nil {
			t.Fatalf("Allocate %s: %v", id, err)
		}
	}

	var exhausted int64
	fb.OnBudgetExhausted(func(_ string) {
		atomic.AddInt64(&exhausted, 1)
	})

	var wg sync.WaitGroup

	// Concurrent spends.
	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("s%d", idx)
			for range 100 {
				_ = fb.Spend(id, 1.0)
			}
		}(i)
	}

	// Concurrent reads.
	for i := range 5 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("s%d", idx)
			for range 50 {
				_ = fb.Remaining(id)
				_ = fb.TotalRemaining()
				_ = fb.Summary()
			}
		}(i)
	}

	wg.Wait()

	// Verify total spend.
	if got := fb.TotalRemaining(); got != 0 {
		t.Errorf("total remaining after full spend = %.2f, want 0", got)
	}
}

func TestFederatedBudget_MultipleCallbacks(t *testing.T) {
	fb := NewFederatedBudget(100.0)
	_ = fb.Allocate("s1", 10)

	var count int32
	fb.OnBudgetExhausted(func(_ string) {
		atomic.AddInt32(&count, 1)
	})
	fb.OnBudgetExhausted(func(_ string) {
		atomic.AddInt32(&count, 10)
	})

	_ = fb.Spend("s1", 10)

	if got := atomic.LoadInt32(&count); got != 11 {
		t.Errorf("callback count = %d, want 11", got)
	}
}

func TestFederatedBudget_ZeroBudget(t *testing.T) {
	fb := NewFederatedBudget(0)

	err := fb.Allocate("s1", 1)
	if !errors.Is(err, ErrFederationBudgetExceeded) {
		t.Errorf("expected ErrFederationBudgetExceeded for zero-budget federation, got %v", err)
	}
}
