package fleet

import (
	"fmt"
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

func TestWorkQueue_PushValidated_InvalidRepoPath(t *testing.T) {
	q := NewWorkQueue()

	item := &WorkItem{
		ID:       "w1",
		Type:     WorkTypeSession,
		Status:   WorkPending,
		RepoPath: "/nonexistent/path/does/not/exist",
	}
	err := q.PushValidated(item)
	if err == nil {
		t.Fatal("expected error for invalid repo path")
	}
	if _, ok := q.Get("w1"); ok {
		t.Error("item should not be in queue after failed push")
	}
}

func TestWorkQueue_PushValidated_ValidRepoPath(t *testing.T) {
	q := NewWorkQueue()
	dir := t.TempDir()

	item := &WorkItem{
		ID:       "w1",
		Type:     WorkTypeSession,
		Status:   WorkPending,
		RepoPath: dir,
	}
	err := q.PushValidated(item)
	if err != nil {
		t.Fatalf("unexpected error for valid repo path: %v", err)
	}
	if _, ok := q.Get("w1"); !ok {
		t.Error("expected to find item in queue")
	}
}

func TestWorkQueue_PushValidated_EmptyRepoPath(t *testing.T) {
	q := NewWorkQueue()

	item := &WorkItem{
		ID:     "w1",
		Status: WorkPending,
	}
	err := q.PushValidated(item)
	if err != nil {
		t.Fatalf("unexpected error for empty repo path: %v", err)
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

// TestReapStale_CleansOldTasks verifies QW-11: stale pending tasks older than
// maxAge are moved to the DLQ.
func TestReapStale_CleansOldTasks(t *testing.T) {
	q := NewWorkQueue()

	// Fresh item: should survive reaping
	fresh := &WorkItem{
		ID:          "fresh",
		Status:      WorkPending,
		SubmittedAt: time.Now(),
	}
	q.Push(fresh)

	// Stale item: submitted 2 hours ago
	stale := &WorkItem{
		ID:          "stale",
		Status:      WorkPending,
		SubmittedAt: time.Now().Add(-2 * time.Hour),
	}
	q.Push(stale)

	reaped := q.ReapStale(time.Hour)
	if reaped != 1 {
		t.Errorf("ReapStale returned %d, want 1", reaped)
	}

	// Fresh should still be in queue
	if _, ok := q.Get("fresh"); !ok {
		t.Error("fresh item should still be in queue")
	}

	// Stale should be in DLQ
	if _, ok := q.Get("stale"); ok {
		t.Error("stale item should have been removed from queue")
	}
	dlq := q.ListDLQ()
	found := false
	for _, item := range dlq {
		if item.ID == "stale" {
			found = true
			if item.Error != "reaped: stale task" {
				t.Errorf("expected reaped error message, got %q", item.Error)
			}
		}
	}
	if !found {
		t.Error("stale item not found in DLQ after reaping")
	}
}

// TestReapStale_CleansPhantomPaths verifies QW-11: items with nonexistent
// repo paths are reaped regardless of age.
func TestReapStale_CleansPhantomPaths(t *testing.T) {
	q := NewWorkQueue()

	// Recent item with invalid path: should be reaped even though it's fresh
	phantom := &WorkItem{
		ID:          "phantom",
		Status:      WorkPending,
		RepoPath:    "/nonexistent/phantom/001/repo",
		SubmittedAt: time.Now(), // just submitted
	}
	q.Push(phantom)

	reaped := q.ReapStale(24 * time.Hour) // very long maxAge
	if reaped != 1 {
		t.Errorf("ReapStale returned %d, want 1 (phantom path)", reaped)
	}
	if _, ok := q.Get("phantom"); ok {
		t.Error("phantom item should have been reaped")
	}
}

// TestPushValidated_RejectsInvalidPath verifies QW-11: work items with
// nonexistent repo paths are rejected at submission time.
func TestPushValidated_RejectsInvalidPath(t *testing.T) {
	q := NewWorkQueue()

	item := &WorkItem{
		ID:       "bad-path",
		Status:   WorkPending,
		RepoPath: "/nonexistent/phantom/001",
	}
	err := q.PushValidated(item)
	if err == nil {
		t.Fatal("expected error for invalid repo path")
	}
	if _, ok := q.Get("bad-path"); ok {
		t.Error("item with invalid path should not be in queue")
	}
}

// TestPushValidated_AcceptsValidPath verifies that PushValidated allows
// items with valid paths.
func TestPushValidated_AcceptsValidPath(t *testing.T) {
	q := NewWorkQueue()

	dir := t.TempDir()
	item := &WorkItem{
		ID:       "valid-path",
		Status:   WorkPending,
		RepoPath: dir,
	}
	err := q.PushValidated(item)
	if err != nil {
		t.Fatalf("unexpected error for valid path: %v", err)
	}
	if _, ok := q.Get("valid-path"); !ok {
		t.Error("item with valid path should be in queue")
	}
}

// TestReapPhantomRepos_QW11 verifies that ReapPhantomRepos removes all pending
// items whose RepoName is "001" or whose RepoPath ends in "001", and preserves
// all valid entries.
func TestReapPhantomRepos_QW11(t *testing.T) {
	q := NewWorkQueue()

	// Valid entries — must survive cleanup
	validIDs := []string{"valid-1", "valid-2", "valid-3"}
	q.Push(&WorkItem{ID: "valid-1", Status: WorkPending, RepoName: "myrepo", SubmittedAt: time.Now()})
	q.Push(&WorkItem{ID: "valid-2", Status: WorkPending, RepoName: "001extra", SubmittedAt: time.Now()}) // not bare "001"
	q.Push(&WorkItem{ID: "valid-3", Status: WorkPending, RepoName: "project001", SubmittedAt: time.Now()})

	// Phantom entries — must be reaped
	phantomCount := 5
	for i := 0; i < phantomCount; i++ {
		q.Push(&WorkItem{
			ID:          fmt.Sprintf("phantom-name-%d", i),
			Status:      WorkPending,
			RepoName:    "001",
			SubmittedAt: time.Now(),
		})
	}
	// Phantom by path segment
	q.Push(&WorkItem{
		ID:          "phantom-path-1",
		Status:      WorkPending,
		RepoName:    "some-repo",
		RepoPath:    "/home/ci/repos/001",
		SubmittedAt: time.Now(),
	})
	q.Push(&WorkItem{
		ID:          "phantom-path-2",
		Status:      WorkPending,
		RepoName:    "another",
		RepoPath:    "/srv/001",
		SubmittedAt: time.Now(),
	})
	totalPhantoms := phantomCount + 2

	// Non-pending phantom by name — should NOT be reaped (only pending are targeted)
	q.Push(&WorkItem{
		ID:       "phantom-assigned",
		Status:   WorkAssigned,
		RepoName: "001",
	})

	reaped := q.ReapPhantomRepos()
	if reaped != totalPhantoms {
		t.Errorf("ReapPhantomRepos returned %d, want %d", reaped, totalPhantoms)
	}

	// All valid entries must still be present
	for _, id := range validIDs {
		if _, ok := q.Get(id); !ok {
			t.Errorf("valid entry %q was incorrectly removed", id)
		}
	}

	// Non-pending phantom must still be in queue (was not targeted)
	if _, ok := q.Get("phantom-assigned"); !ok {
		t.Error("non-pending phantom should not have been reaped")
	}

	// All phantom pending entries must be gone from main queue
	for i := 0; i < phantomCount; i++ {
		id := fmt.Sprintf("phantom-name-%d", i)
		if _, ok := q.Get(id); ok {
			t.Errorf("phantom entry %q should have been reaped", id)
		}
	}
	for _, id := range []string{"phantom-path-1", "phantom-path-2"} {
		if _, ok := q.Get(id); ok {
			t.Errorf("phantom entry %q should have been reaped", id)
		}
	}

	// Reaped items must be in DLQ with correct error
	dlq := q.ListDLQ()
	dlqByID := make(map[string]*WorkItem, len(dlq))
	for _, item := range dlq {
		dlqByID[item.ID] = item
	}
	for i := 0; i < phantomCount; i++ {
		id := fmt.Sprintf("phantom-name-%d", i)
		item, ok := dlqByID[id]
		if !ok {
			t.Errorf("phantom entry %q not found in DLQ", id)
			continue
		}
		if item.Error != "reaped: phantom repo placeholder" {
			t.Errorf("phantom entry %q has wrong error %q", id, item.Error)
		}
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
