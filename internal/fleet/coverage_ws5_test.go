package fleet

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// A2AAdapter.OfferCount / CountByStatus / NewA2AAdapterWithCoordinator
// ---------------------------------------------------------------------------

func TestNewA2AAdapterWithCoordinator(t *testing.T) {
	t.Parallel()
	// nil coordinator should not panic
	a := NewA2AAdapterWithCoordinator(nil)
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
	if a.OfferCount() != 0 {
		t.Errorf("expected 0 offers, got %d", a.OfferCount())
	}
}

func TestA2AAdapter_OfferCount(t *testing.T) {
	t.Parallel()
	a := NewA2AAdapter()

	if a.OfferCount() != 0 {
		t.Errorf("empty adapter: got %d, want 0", a.OfferCount())
	}

	// Add some offers directly
	a.mu.Lock()
	a.offers["offer-1"] = &TaskOffer{ID: "offer-1", Status: "open"}
	a.offers["offer-2"] = &TaskOffer{ID: "offer-2", Status: "completed"}
	a.offers["offer-3"] = &TaskOffer{ID: "offer-3", Status: "open"}
	a.mu.Unlock()

	if got := a.OfferCount(); got != 3 {
		t.Errorf("OfferCount() = %d, want 3", got)
	}
}

func TestA2AAdapter_CountByStatus(t *testing.T) {
	t.Parallel()
	a := NewA2AAdapter()

	// Empty adapter
	counts := a.CountByStatus()
	if len(counts) != 0 {
		t.Errorf("empty adapter: expected empty map, got %v", counts)
	}

	// Add offers with mixed statuses
	a.mu.Lock()
	a.offers["o1"] = &TaskOffer{ID: "o1", Status: "open"}
	a.offers["o2"] = &TaskOffer{ID: "o2", Status: "open"}
	a.offers["o3"] = &TaskOffer{ID: "o3", Status: "completed"}
	a.offers["o4"] = &TaskOffer{ID: "o4", Status: "failed"}
	a.offers["o5"] = &TaskOffer{ID: "o5", Status: "completed"}
	a.mu.Unlock()

	counts = a.CountByStatus()
	if counts["open"] != 2 {
		t.Errorf("open = %d, want 2", counts["open"])
	}
	if counts["completed"] != 2 {
		t.Errorf("completed = %d, want 2", counts["completed"])
	}
	if counts["failed"] != 1 {
		t.Errorf("failed = %d, want 1", counts["failed"])
	}
}

func TestA2AAdapter_OfferCountAfterPublish(t *testing.T) {
	t.Parallel()
	a := NewA2AAdapter()

	offer := TaskOffer{
		ID:           "pub-1",
		OfferingNode: "node-1",
		TaskType:     "test",
		Prompt:       "do something",
		Status:       string(OfferSubmitted),
		Deadline:     time.Now().Add(time.Hour),
	}

	if err := a.Offer(offer); err != nil {
		t.Fatalf("Offer: %v", err)
	}

	if got := a.OfferCount(); got != 1 {
		t.Errorf("OfferCount after Offer() = %d, want 1", got)
	}

	counts := a.CountByStatus()
	// Offer() normalizes status to "open"
	if counts[string(OfferOpen)] != 1 {
		t.Errorf("open count = %d, want 1", counts[string(OfferOpen)])
	}
}

// ---------------------------------------------------------------------------
// WorkQueue.ReapStale
// ---------------------------------------------------------------------------

func TestReapStale_AgedItems(t *testing.T) {
	t.Parallel()
	q := NewWorkQueue()

	// Add stale pending items
	q.Push(&WorkItem{
		ID:          "stale-1",
		Status:      WorkPending,
		SubmittedAt: time.Now().Add(-48 * time.Hour),
	})
	q.Push(&WorkItem{
		ID:          "stale-2",
		Status:      WorkPending,
		SubmittedAt: time.Now().Add(-72 * time.Hour),
	})
	// Add a recent pending item that should survive
	q.Push(&WorkItem{
		ID:          "fresh-1",
		Status:      WorkPending,
		SubmittedAt: time.Now().Add(-1 * time.Hour),
	})
	// Add a non-pending item that should survive regardless of age
	q.Push(&WorkItem{
		ID:          "running-1",
		Status:      WorkRunning,
		SubmittedAt: time.Now().Add(-96 * time.Hour),
	})

	reaped := q.ReapStale(24 * time.Hour)
	if reaped != 2 {
		t.Errorf("reaped = %d, want 2", reaped)
	}

	// fresh-1 should still be in queue
	if _, ok := q.Get("fresh-1"); !ok {
		t.Error("fresh-1 should still be in queue")
	}
	// running-1 should still be in queue (not pending)
	if _, ok := q.Get("running-1"); !ok {
		t.Error("running-1 should still be in queue")
	}
	// stale items should be gone from main queue
	if _, ok := q.Get("stale-1"); ok {
		t.Error("stale-1 should have been reaped")
	}
	if _, ok := q.Get("stale-2"); ok {
		t.Error("stale-2 should have been reaped")
	}

	// Verify reaped items are in DLQ
	q.mu.Lock()
	dlqCount := len(q.dlq)
	if q.dlq["stale-1"] == nil || q.dlq["stale-2"] == nil {
		t.Error("stale items should be in DLQ")
	}
	q.mu.Unlock()
	if dlqCount != 2 {
		t.Errorf("DLQ size = %d, want 2", dlqCount)
	}
}

func TestReapStale_MissingRepoPath(t *testing.T) {
	t.Parallel()
	q := NewWorkQueue()

	// Item with a non-existent repo path should be reaped even if recent
	q.Push(&WorkItem{
		ID:          "gone-repo",
		Status:      WorkPending,
		RepoPath:    "/nonexistent/repo/path/that/does/not/exist",
		SubmittedAt: time.Now(), // recent!
	})

	reaped := q.ReapStale(24 * time.Hour)
	if reaped != 1 {
		t.Errorf("reaped = %d, want 1 (missing repo path)", reaped)
	}
}

func TestReapStale_EmptyQueue(t *testing.T) {
	t.Parallel()
	q := NewWorkQueue()

	reaped := q.ReapStale(24 * time.Hour)
	if reaped != 0 {
		t.Errorf("reaped = %d, want 0 for empty queue", reaped)
	}
}

func TestReapStale_ReapedItemHasError(t *testing.T) {
	t.Parallel()
	q := NewWorkQueue()

	q.Push(&WorkItem{
		ID:          "error-check",
		Status:      WorkPending,
		SubmittedAt: time.Now().Add(-48 * time.Hour),
	})

	q.ReapStale(24 * time.Hour)

	q.mu.Lock()
	item := q.dlq["error-check"]
	q.mu.Unlock()

	if item == nil {
		t.Fatal("expected item in DLQ")
	}
	if item.Error == "" {
		t.Error("expected non-empty error message on reaped item")
	}
	if item.CompletedAt == nil {
		t.Error("expected CompletedAt to be set on reaped item")
	}
}

// ---------------------------------------------------------------------------
// GetLocalIP
// ---------------------------------------------------------------------------

func TestGetLocalIP(t *testing.T) {
	t.Parallel()
	ip := GetLocalIP()
	if ip == "" {
		t.Error("expected non-empty IP")
	}
	// Should be an IP address, not a hostname
	// At minimum it should be 127.0.0.1 or a real IP
	if ip != "127.0.0.1" && len(ip) < 7 {
		t.Errorf("unexpected IP format: %q", ip)
	}
}

// ---------------------------------------------------------------------------
// NewWorkerAgent / NodeID
// ---------------------------------------------------------------------------

func TestNewWorkerAgent(t *testing.T) {
	t.Parallel()
	w := NewWorkerAgent("http://localhost:8080", "test-host", 9090, "v1.0.0", "/tmp/scan", nil, nil)
	if w == nil {
		t.Fatal("expected non-nil WorkerAgent")
	}
	if w.hostname != "test-host" {
		t.Errorf("hostname = %q, want test-host", w.hostname)
	}
	if w.port != 9090 {
		t.Errorf("port = %d, want 9090", w.port)
	}
	if w.version != "v1.0.0" {
		t.Errorf("version = %q, want v1.0.0", w.version)
	}
}

func TestWorkerAgent_NodeID_BeforeRegistration(t *testing.T) {
	t.Parallel()
	w := NewWorkerAgent("http://localhost:8080", "host", 9090, "v1", "/tmp", nil, nil)

	// Before registration, NodeID should be empty
	if id := w.NodeID(); id != "" {
		t.Errorf("NodeID before registration = %q, want empty", id)
	}
}

func TestWorkerAgent_NodeID_AfterSet(t *testing.T) {
	t.Parallel()
	w := NewWorkerAgent("http://localhost:8080", "host", 9090, "v1", "/tmp", nil, nil)

	// Simulate registration by setting nodeID directly
	w.nodeID = "worker-123"
	if id := w.NodeID(); id != "worker-123" {
		t.Errorf("NodeID = %q, want worker-123", id)
	}
}

// ---------------------------------------------------------------------------
// DiscoverTailscaleIP (graceful fallback)
// ---------------------------------------------------------------------------

func TestDiscoverTailscaleIP_Fallback(t *testing.T) {
	t.Parallel()
	// In test environments, /run/tailscale/ won't exist.
	// Should return empty string without error.
	ip := DiscoverTailscaleIP()
	// We don't assert the exact value — just that it doesn't panic
	_ = ip
}
