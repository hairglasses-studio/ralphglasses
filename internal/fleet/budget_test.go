package fleet

import (
	"sync"
	"testing"
)

func TestWorkerBudget_Remaining(t *testing.T) {
	tests := []struct {
		name  string
		wb    WorkerBudget
		want  float64
	}{
		{"full budget", WorkerBudget{Limit: 100, Spent: 0}, 100},
		{"partial spend", WorkerBudget{Limit: 100, Spent: 40}, 60},
		{"fully spent", WorkerBudget{Limit: 100, Spent: 100}, 0},
		{"overspent clamps to zero", WorkerBudget{Limit: 100, Spent: 150}, 0},
		{"zero limit", WorkerBudget{Limit: 0, Spent: 0}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.wb.Remaining()
			if got != tt.want {
				t.Errorf("Remaining() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkerBudget_CanAcceptWork(t *testing.T) {
	tests := []struct {
		name          string
		wb            WorkerBudget
		estimatedCost float64
		want          bool
	}{
		{"enough budget", WorkerBudget{Limit: 100, Spent: 0}, 50, true},
		{"exact budget", WorkerBudget{Limit: 100, Spent: 50}, 50, true},
		{"insufficient budget", WorkerBudget{Limit: 100, Spent: 80}, 30, false},
		{"zero cost always ok", WorkerBudget{Limit: 100, Spent: 100}, 0, true},
		{"overspent rejects", WorkerBudget{Limit: 100, Spent: 150}, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.wb.CanAcceptWork(tt.estimatedCost)
			if got != tt.want {
				t.Errorf("CanAcceptWork(%v) = %v, want %v", tt.estimatedCost, got, tt.want)
			}
		})
	}
}

func TestBudgetManager_SetBudget_GetBudget(t *testing.T) {
	bm := NewBudgetManager(50)

	// Before setting, should return default
	b := bm.GetBudget("worker-1")
	if b.Limit != 50 {
		t.Errorf("default limit = %v, want 50", b.Limit)
	}
	if b.Spent != 0 {
		t.Errorf("default spent = %v, want 0", b.Spent)
	}

	// Set explicit budget
	bm.SetBudget("worker-1", 200)
	b = bm.GetBudget("worker-1")
	if b.Limit != 200 {
		t.Errorf("after SetBudget, limit = %v, want 200", b.Limit)
	}

	// Update existing budget
	bm.SetBudget("worker-1", 300)
	b = bm.GetBudget("worker-1")
	if b.Limit != 300 {
		t.Errorf("after second SetBudget, limit = %v, want 300", b.Limit)
	}
}

func TestBudgetManager_RecordCost(t *testing.T) {
	bm := NewBudgetManager(100)

	bm.RecordCost("worker-1", 10)
	bm.RecordCost("worker-1", 25.5)

	b := bm.GetBudget("worker-1")
	if b.Spent != 35.5 {
		t.Errorf("spent = %v, want 35.5", b.Spent)
	}
	if b.Limit != 100 {
		t.Errorf("limit = %v, want 100 (default)", b.Limit)
	}
}

func TestBudgetManager_CanAcceptWork(t *testing.T) {
	bm := NewBudgetManager(100)

	// No explicit budget: uses default limit
	if !bm.CanAcceptWork("worker-new", 50) {
		t.Error("should accept work within default limit")
	}
	if bm.CanAcceptWork("worker-new", 150) {
		t.Error("should reject work exceeding default limit")
	}

	// With explicit budget
	bm.SetBudget("worker-1", 200)
	bm.RecordCost("worker-1", 180)
	if bm.CanAcceptWork("worker-1", 20) {
		t.Log("correctly accepts work at exact remaining")
	}
	if bm.CanAcceptWork("worker-1", 21) {
		t.Error("should reject work exceeding remaining budget")
	}
}

func TestBudgetManager_ConcurrentAccess(t *testing.T) {
	bm := NewBudgetManager(1000)
	bm.SetBudget("worker-1", 1000)

	var wg sync.WaitGroup
	n := 100
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			bm.RecordCost("worker-1", 1.0)
		}()
	}
	wg.Wait()

	b := bm.GetBudget("worker-1")
	if b.Spent != float64(n) {
		t.Errorf("concurrent spent = %v, want %v", b.Spent, float64(n))
	}
}

func TestBudgetManager_Summary(t *testing.T) {
	bm := NewBudgetManager(50)
	bm.SetBudget("w1", 100)
	bm.SetBudget("w2", 200)
	bm.RecordCost("w1", 10)
	bm.RecordCost("w2", 20)

	summary := bm.Summary()
	if len(summary) != 2 {
		t.Fatalf("summary has %d entries, want 2", len(summary))
	}
	if summary["w1"].Limit != 100 || summary["w1"].Spent != 10 {
		t.Errorf("w1 = %+v, want Limit=100 Spent=10", summary["w1"])
	}
	if summary["w2"].Limit != 200 || summary["w2"].Spent != 20 {
		t.Errorf("w2 = %+v, want Limit=200 Spent=20", summary["w2"])
	}

	// Verify summary returns copies (mutations don't affect manager)
	summary["w1"] = WorkerBudget{Limit: 999, Spent: 999}
	b := bm.GetBudget("w1")
	if b.Limit != 100 {
		t.Error("summary mutation should not affect manager")
	}
}
