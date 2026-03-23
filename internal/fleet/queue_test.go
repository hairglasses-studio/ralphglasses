package fleet

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestWorkQueue_PushAndGet(t *testing.T) {
	q := NewWorkQueue()

	item := &WorkItem{
		ID:       "w1",
		Type:     WorkTypeSession,
		Status:   WorkPending,
		Priority: 5,
		RepoName: "test-repo",
		Prompt:   "fix bug",
	}
	q.Push(item)

	got, ok := q.Get("w1")
	if !ok {
		t.Fatal("expected to find work item")
	}
	if got.RepoName != "test-repo" {
		t.Errorf("got repo %q, want test-repo", got.RepoName)
	}
}

func TestWorkQueue_AssignBest(t *testing.T) {
	q := NewWorkQueue()

	q.Push(&WorkItem{ID: "low", Status: WorkPending, Priority: 1, RepoName: "a"})
	q.Push(&WorkItem{ID: "high", Status: WorkPending, Priority: 10, RepoName: "b"})
	q.Push(&WorkItem{ID: "med", Status: WorkPending, Priority: 5, RepoName: "c"})

	assigned := q.AssignBest(func(item *WorkItem) int {
		return item.Priority * 100
	}, "worker-1")

	if assigned == nil {
		t.Fatal("expected work item")
	}
	if assigned.ID != "high" {
		t.Errorf("got %q, want high-priority item", assigned.ID)
	}
	if assigned.Status != WorkAssigned {
		t.Errorf("got status %q, want assigned", assigned.Status)
	}
	if assigned.AssignedTo != "worker-1" {
		t.Errorf("got assigned to %q, want worker-1", assigned.AssignedTo)
	}
}

func TestWorkQueue_AssignBest_SkipsNegativeScore(t *testing.T) {
	q := NewWorkQueue()
	q.Push(&WorkItem{
		ID:       "w1",
		Status:   WorkPending,
		Priority: 5,
		Provider: session.ProviderGemini,
	})

	// Scorer that rejects gemini items
	assigned := q.AssignBest(func(item *WorkItem) int {
		if item.Provider == session.ProviderGemini {
			return -1
		}
		return item.Priority * 100
	}, "worker-1")

	if assigned != nil {
		t.Error("expected nil, scorer should have rejected the item")
	}
}

func TestWorkQueue_ReclaimTimedOut(t *testing.T) {
	q := NewWorkQueue()

	past := time.Now().Add(-10 * time.Minute)
	q.Push(&WorkItem{
		ID:         "w1",
		Status:     WorkAssigned,
		AssignedTo: "old-worker",
		AssignedAt: &past,
	})

	q.ReclaimTimedOut(5 * time.Minute)

	item, _ := q.Get("w1")
	if item.Status != WorkPending {
		t.Errorf("got status %q, want pending after reclaim", item.Status)
	}
	if item.AssignedTo != "" {
		t.Errorf("assigned_to should be cleared, got %q", item.AssignedTo)
	}
}

func TestWorkQueue_Counts(t *testing.T) {
	q := NewWorkQueue()
	q.Push(&WorkItem{ID: "1", Status: WorkPending})
	q.Push(&WorkItem{ID: "2", Status: WorkPending})
	q.Push(&WorkItem{ID: "3", Status: WorkCompleted})
	q.Push(&WorkItem{ID: "4", Status: WorkFailed})

	counts := q.Counts()
	if counts[WorkPending] != 2 {
		t.Errorf("pending: got %d, want 2", counts[WorkPending])
	}
	if counts[WorkCompleted] != 1 {
		t.Errorf("completed: got %d, want 1", counts[WorkCompleted])
	}
}

func TestWorkQueue_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.json")

	q1 := NewWorkQueue()
	q1.Push(&WorkItem{ID: "w1", Status: WorkPending, RepoName: "test"})
	q1.Push(&WorkItem{ID: "w2", Status: WorkCompleted, RepoName: "test2"})

	if err := q1.SaveTo(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	q2 := NewWorkQueue()
	if err := q2.LoadFrom(path); err != nil {
		t.Fatalf("load: %v", err)
	}

	if _, ok := q2.Get("w1"); !ok {
		t.Error("w1 not found after load")
	}
	if _, ok := q2.Get("w2"); !ok {
		t.Error("w2 not found after load")
	}
}

func TestWorkQueue_LoadFrom_MissingFile(t *testing.T) {
	q := NewWorkQueue()
	err := q.LoadFrom("/nonexistent/path.json")
	if err != nil {
		t.Errorf("expected nil error for missing file, got %v", err)
	}
}

func TestWorkQueue_Pending(t *testing.T) {
	q := NewWorkQueue()
	q.Push(&WorkItem{ID: "low", Status: WorkPending, Priority: 1})
	q.Push(&WorkItem{ID: "high", Status: WorkPending, Priority: 10})
	q.Push(&WorkItem{ID: "done", Status: WorkCompleted, Priority: 100})

	pending := q.Pending()
	if len(pending) != 2 {
		t.Fatalf("got %d pending, want 2", len(pending))
	}
	if pending[0].ID != "high" {
		t.Errorf("first pending should be high-priority, got %q", pending[0].ID)
	}
}

func TestGlobalBudget_AvailableBudget(t *testing.T) {
	b := GlobalBudget{LimitUSD: 100, SpentUSD: 30, ReservedUSD: 20}
	avail := b.AvailableBudget()
	if avail != 50 {
		t.Errorf("got $%.2f, want $50.00", avail)
	}

	b2 := GlobalBudget{LimitUSD: 10, SpentUSD: 8, ReservedUSD: 5}
	if b2.AvailableBudget() != 0 {
		t.Errorf("should return 0 when overcommitted, got $%.2f", b2.AvailableBudget())
	}
}

func TestWorkItem_StatusTransitions(t *testing.T) {
	item := &WorkItem{
		ID:       "w1",
		Status:   WorkPending,
		Priority: 5,
	}

	if item.Status != WorkPending {
		t.Errorf("initial status: got %q, want pending", item.Status)
	}

	item.Status = WorkAssigned
	if item.Status != WorkAssigned {
		t.Errorf("after assign: got %q, want assigned", item.Status)
	}

	item.Status = WorkRunning
	item.Status = WorkCompleted
	if item.Status != WorkCompleted {
		t.Errorf("final status: got %q, want completed", item.Status)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
