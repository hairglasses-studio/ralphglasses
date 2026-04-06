package mcpserver

import (
	"sync"
	"testing"
)

func TestApprovalStore_Create(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore()

	rec := store.Create("deploy to prod", "release v2.0 ready", "high", "sess-123")

	if rec.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if rec.Action != "deploy to prod" {
		t.Errorf("action = %q, want %q", rec.Action, "deploy to prod")
	}
	if rec.Context != "release v2.0 ready" {
		t.Errorf("context = %q, want %q", rec.Context, "release v2.0 ready")
	}
	if rec.Urgency != "high" {
		t.Errorf("urgency = %q, want %q", rec.Urgency, "high")
	}
	if rec.SessionID != "sess-123" {
		t.Errorf("session_id = %q, want %q", rec.SessionID, "sess-123")
	}
	if rec.Status != ApprovalPending {
		t.Errorf("status = %q, want %q", rec.Status, ApprovalPending)
	}
	if rec.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
	if rec.ResolvedAt != nil {
		t.Error("expected nil ResolvedAt for pending record")
	}
}

func TestApprovalStore_CreateNoSession(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore()

	rec := store.Create("merge PR", "all tests pass", "normal", "")

	if rec.SessionID != "" {
		t.Errorf("session_id = %q, want empty", rec.SessionID)
	}
}

func TestApprovalStore_Get(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore()

	rec := store.Create("action", "ctx", "low", "")
	got := store.Get(rec.ID)

	if got == nil {
		t.Fatal("expected record, got nil")
	}
	if got.ID != rec.ID {
		t.Errorf("id = %q, want %q", got.ID, rec.ID)
	}
}

func TestApprovalStore_Get_NotFound(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore()

	got := store.Get("nonexistent")
	if got != nil {
		t.Errorf("expected nil for nonexistent ID, got %+v", got)
	}
}

func TestApprovalStore_Get_ReturnsCopy(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore()

	rec := store.Create("action", "ctx", "low", "")
	got := store.Get(rec.ID)

	// Mutating the returned copy should not affect the store.
	got.Action = "mutated"

	got2 := store.Get(rec.ID)
	if got2.Action != "action" {
		t.Errorf("store was mutated via returned copy: action = %q", got2.Action)
	}
}

func TestApprovalStore_Resolve_Approved(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore()

	rec := store.Create("deploy", "ready", "high", "")
	resolved, err := store.Resolve(rec.ID, "approved", "looks good")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Status != ApprovalApproved {
		t.Errorf("status = %q, want %q", resolved.Status, ApprovalApproved)
	}
	if resolved.Decision != "approved" {
		t.Errorf("decision = %q, want %q", resolved.Decision, "approved")
	}
	if resolved.Reason != "looks good" {
		t.Errorf("reason = %q, want %q", resolved.Reason, "looks good")
	}
	if resolved.ResolvedAt == nil {
		t.Error("expected non-nil ResolvedAt")
	}
}

func TestApprovalStore_Resolve_Rejected(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore()

	rec := store.Create("deploy", "ready", "critical", "")
	resolved, err := store.Resolve(rec.ID, "rejected", "not ready")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Status != ApprovalRejected {
		t.Errorf("status = %q, want %q", resolved.Status, ApprovalRejected)
	}
	if resolved.Reason != "not ready" {
		t.Errorf("reason = %q, want %q", resolved.Reason, "not ready")
	}
}

func TestApprovalStore_Resolve_NotFound(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore()

	_, err := store.Resolve("nonexistent", "approved", "")
	if err == nil {
		t.Fatal("expected error for nonexistent approval")
	}
}

func TestApprovalStore_Resolve_AlreadyResolved(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore()

	rec := store.Create("deploy", "ready", "low", "")
	_, err := store.Resolve(rec.ID, "approved", "")
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}

	_, err = store.Resolve(rec.ID, "rejected", "changed mind")
	if err == nil {
		t.Fatal("expected error when resolving already-resolved approval")
	}
}

func TestApprovalStore_List_PendingOnly(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore()

	store.Create("action1", "ctx1", "low", "")
	rec2 := store.Create("action2", "ctx2", "high", "")
	store.Create("action3", "ctx3", "normal", "")

	// Resolve one.
	_, err := store.Resolve(rec2.ID, "approved", "")
	if err != nil {
		t.Fatal(err)
	}

	pending := store.List()
	if len(pending) != 2 {
		t.Errorf("pending count = %d, want 2", len(pending))
	}
	for _, p := range pending {
		if p.Status != ApprovalPending {
			t.Errorf("expected pending status, got %q", p.Status)
		}
	}
}

func TestApprovalStore_ListAll(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore()

	store.Create("action1", "ctx1", "low", "")
	rec2 := store.Create("action2", "ctx2", "high", "")

	_, err := store.Resolve(rec2.ID, "rejected", "nope")
	if err != nil {
		t.Fatal(err)
	}

	all := store.ListAll()
	if len(all) != 2 {
		t.Errorf("all count = %d, want 2", len(all))
	}
}

func TestApprovalStore_List_Empty(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore()

	pending := store.List()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(pending))
	}
}

func TestApprovalStore_Concurrent(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore()

	var wg sync.WaitGroup
	const n = 50
	ids := make([]string, n)

	// Concurrent creates.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := store.Create("action", "ctx", "normal", "")
			ids[i] = rec.ID
		}(i)
	}
	wg.Wait()

	// Concurrent reads.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store.Get(ids[i])
		}(i)
	}
	wg.Wait()

	// Concurrent resolves.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = store.Resolve(ids[i], "approved", "")
		}(i)
	}
	wg.Wait()

	all := store.ListAll()
	if len(all) != n {
		t.Errorf("total records = %d, want %d", len(all), n)
	}

	pending := store.List()
	if len(pending) != 0 {
		t.Errorf("pending after resolving all = %d, want 0", len(pending))
	}
}
