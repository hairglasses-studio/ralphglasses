package pool

import (
	"testing"
)

func TestNewBudgetPool_Unlimited(t *testing.T) {
	p := NewBudgetPool(0)
	if p.GlobalBudget() != 0 {
		t.Errorf("GlobalBudget() = %f, want 0 (unlimited)", p.GlobalBudget())
	}
	// Unlimited mode: any allocation should succeed.
	if err := p.AllocateSession("s1", 99999); err != nil {
		t.Errorf("unlimited pool: unexpected error: %v", err)
	}
}

func TestNewBudgetPool_WithLimit(t *testing.T) {
	p := NewBudgetPool(100)
	if p.GlobalBudget() != 100 {
		t.Errorf("GlobalBudget() = %f, want 100", p.GlobalBudget())
	}
}

func TestBudgetPool_SetGlobalBudget(t *testing.T) {
	p := NewBudgetPool(50)
	p.SetGlobalBudget(200)
	if p.GlobalBudget() != 200 {
		t.Errorf("after SetGlobalBudget(200), GlobalBudget() = %f, want 200", p.GlobalBudget())
	}
}

func TestBudgetPool_AllocateSession_Basic(t *testing.T) {
	p := NewBudgetPool(100)

	if err := p.AllocateSession("s1", 30); err != nil {
		t.Fatalf("AllocateSession: unexpected error: %v", err)
	}
	if err := p.AllocateSession("s2", 40); err != nil {
		t.Fatalf("AllocateSession: unexpected error: %v", err)
	}

	alloc, ok := p.Allocation("s1")
	if !ok || alloc != 30 {
		t.Errorf("Allocation(s1) = %f, %v; want 30, true", alloc, ok)
	}

	alloc, ok = p.Allocation("s2")
	if !ok || alloc != 40 {
		t.Errorf("Allocation(s2) = %f, %v; want 40, true", alloc, ok)
	}
}

func TestBudgetPool_AllocateSession_ExceedsLimit(t *testing.T) {
	p := NewBudgetPool(50)
	p.AllocateSession("s1", 30) //nolint

	// s2 wants 25, but only 20 remains.
	err := p.AllocateSession("s2", 25)
	if err == nil {
		t.Error("expected error when allocation exceeds remaining budget")
	}
}

func TestBudgetPool_AllocateSession_NegativeBudget(t *testing.T) {
	p := NewBudgetPool(100)
	err := p.AllocateSession("s1", -5)
	if err == nil {
		t.Error("expected error for negative allocation")
	}
}

func TestBudgetPool_AllocateSession_Reallocation(t *testing.T) {
	// Re-allocating the same session should replace the old allocation.
	p := NewBudgetPool(100)
	p.AllocateSession("s1", 30) //nolint
	p.AllocateSession("s1", 20) //nolint

	alloc, ok := p.Allocation("s1")
	if !ok || alloc != 20 {
		t.Errorf("after re-alloc, Allocation(s1) = %f, %v; want 20, true", alloc, ok)
	}

	// TotalAllocated should reflect the updated allocation.
	if total := p.TotalAllocated(); total != 20 {
		t.Errorf("TotalAllocated() = %f, want 20", total)
	}
}

func TestBudgetPool_ReleaseSession(t *testing.T) {
	p := NewBudgetPool(100)
	p.AllocateSession("s1", 40) //nolint
	p.AllocateSession("s2", 30) //nolint

	p.ReleaseSession("s1")

	_, ok := p.Allocation("s1")
	if ok {
		t.Error("expected s1 to be released")
	}

	if total := p.TotalAllocated(); total != 30 {
		t.Errorf("TotalAllocated() after release = %f, want 30", total)
	}
}

func TestBudgetPool_ReleaseSession_NoOp(t *testing.T) {
	// Releasing a session that has no allocation should be a no-op.
	p := NewBudgetPool(100)
	p.ReleaseSession("nonexistent") // should not panic
}

func TestBudgetPool_Remaining(t *testing.T) {
	p := NewBudgetPool(100)
	p.AllocateSession("s1", 30) //nolint
	p.AllocateSession("s2", 20) //nolint

	remaining := p.Remaining()
	if remaining != 50 {
		t.Errorf("Remaining() = %f, want 50", remaining)
	}
}

func TestBudgetPool_Remaining_Unlimited(t *testing.T) {
	// Unlimited mode (limit=0) returns 0 to signal "no cap".
	p := NewBudgetPool(0)
	p.AllocateSession("s1", 500) //nolint
	if r := p.Remaining(); r != 0 {
		t.Errorf("Remaining() in unlimited mode = %f, want 0 (signal)", r)
	}
}

func TestBudgetPool_TotalAllocated(t *testing.T) {
	p := NewBudgetPool(200)
	p.AllocateSession("s1", 50) //nolint
	p.AllocateSession("s2", 75) //nolint

	total := p.TotalAllocated()
	if total != 125 {
		t.Errorf("TotalAllocated() = %f, want 125", total)
	}
}

func TestBudgetPool_TotalAllocated_Empty(t *testing.T) {
	p := NewBudgetPool(100)
	if total := p.TotalAllocated(); total != 0 {
		t.Errorf("TotalAllocated() on empty pool = %f, want 0", total)
	}
}

func TestBudgetPool_SessionCount(t *testing.T) {
	p := NewBudgetPool(100)
	if p.SessionCount() != 0 {
		t.Errorf("SessionCount() on empty pool = %d, want 0", p.SessionCount())
	}

	p.AllocateSession("s1", 10) //nolint
	p.AllocateSession("s2", 10) //nolint
	if p.SessionCount() != 2 {
		t.Errorf("SessionCount() = %d, want 2", p.SessionCount())
	}

	p.ReleaseSession("s1")
	if p.SessionCount() != 1 {
		t.Errorf("SessionCount() after release = %d, want 1", p.SessionCount())
	}
}

func TestBudgetPool_Allocation_Missing(t *testing.T) {
	p := NewBudgetPool(100)
	alloc, ok := p.Allocation("missing")
	if ok || alloc != 0 {
		t.Errorf("Allocation(missing) = %f, %v; want 0, false", alloc, ok)
	}
}

func TestBudgetPool_Unlimited_AllowsAnySize(t *testing.T) {
	p := NewBudgetPool(0)
	// Should not error even for large amounts.
	for i := 0; i < 10; i++ {
		sid := "session-large"
		if err := p.AllocateSession(sid, 1_000_000); err != nil {
			t.Fatalf("unlimited pool rejected session %s: %v", sid, err)
		}
	}
}

func TestBudgetPool_ExactlyFillsBudget(t *testing.T) {
	p := NewBudgetPool(100)
	if err := p.AllocateSession("s1", 60); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Exactly fills the remaining 40.
	if err := p.AllocateSession("s2", 40); err != nil {
		t.Fatalf("allocation should succeed when it exactly fills budget: %v", err)
	}
	// Now budget is full.
	if err := p.AllocateSession("s3", 0.01); err == nil {
		t.Error("expected error when adding to a full budget")
	}
}
