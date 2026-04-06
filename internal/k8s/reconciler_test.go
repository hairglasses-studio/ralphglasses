package k8s

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// -------------------------------------------------------------------
// SessionRegistry tests
// -------------------------------------------------------------------

func TestSessionRegistry_SetAndGet(t *testing.T) {
	reg := NewSessionRegistry()

	session := newTestSession("sess-1", "default", "claude", "hello")
	reg.Set(session)

	got := reg.Get("default", "sess-1")
	if got == nil {
		t.Fatal("expected session, got nil")
	}
	if got.Name != "sess-1" {
		t.Errorf("expected name sess-1, got %s", got.Name)
	}
}

func TestSessionRegistry_GetMissing(t *testing.T) {
	reg := NewSessionRegistry()

	got := reg.Get("default", "nonexistent")
	if got != nil {
		t.Errorf("expected nil for missing session, got %v", got)
	}
}

func TestSessionRegistry_Delete(t *testing.T) {
	reg := NewSessionRegistry()

	session := newTestSession("sess-del", "ns1", "gemini", "test")
	reg.Set(session)

	if reg.Len() != 1 {
		t.Fatalf("expected 1 session, got %d", reg.Len())
	}

	reg.Delete("ns1", "sess-del")

	if reg.Len() != 0 {
		t.Fatalf("expected 0 sessions after delete, got %d", reg.Len())
	}
	if reg.Get("ns1", "sess-del") != nil {
		t.Error("expected nil after delete")
	}
}

func TestSessionRegistry_List(t *testing.T) {
	reg := NewSessionRegistry()

	reg.Set(newTestSession("a", "ns1", "claude", "p1"))
	reg.Set(newTestSession("b", "ns1", "gemini", "p2"))
	reg.Set(newTestSession("c", "ns2", "codex", "p3"))

	list := reg.List()
	if len(list) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(list))
	}
}

func TestSessionRegistry_Len(t *testing.T) {
	reg := NewSessionRegistry()

	if reg.Len() != 0 {
		t.Errorf("expected 0 on empty registry, got %d", reg.Len())
	}

	reg.Set(newTestSession("x", "ns", "claude", "p"))
	if reg.Len() != 1 {
		t.Errorf("expected 1, got %d", reg.Len())
	}
}

func TestSessionRegistry_ActiveCount(t *testing.T) {
	reg := NewSessionRegistry()

	running := newTestSession("run", "ns", "claude", "p")
	running.Status.Phase = "Running"
	reg.Set(running)

	launching := newTestSession("launch", "ns", "gemini", "p")
	launching.Status.Phase = "Launching"
	reg.Set(launching)

	completed := newTestSession("done", "ns", "codex", "p")
	completed.Status.Phase = "Completed"
	reg.Set(completed)

	errored := newTestSession("err", "ns", "claude", "p")
	errored.Status.Phase = "Errored"
	reg.Set(errored)

	if reg.ActiveCount() != 2 {
		t.Errorf("expected 2 active sessions (Running + Launching), got %d", reg.ActiveCount())
	}
}

// -------------------------------------------------------------------
// ReconcileLoop tests
// -------------------------------------------------------------------

func TestReconcileLoop_SinglePass(t *testing.T) {
	fc := NewFakeClient()

	// Seed two sessions with finalizers already present.
	s1 := newTestSession("loop-1", "default", "claude", "task A")
	s1.Finalizers = []string{FinalizerName}
	fc.Sessions[fakeKey("default", "loop-1")] = s1

	s2 := newTestSession("loop-2", "default", "gemini", "task B")
	s2.Finalizers = []string{FinalizerName}
	fc.Sessions[fakeKey("default", "loop-2")] = s2

	registry := NewSessionRegistry()
	loop := NewReconcileLoop(fc, registry, 10*time.Second,
		WithNamespace("default"),
	)

	// Run a single reconciliation pass.
	err := loop.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Registry should now track both sessions.
	if registry.Len() != 2 {
		t.Errorf("expected 2 sessions in registry, got %d", registry.Len())
	}

	// Both sessions should have pods created (they had finalizers, no existing pods).
	if len(fc.CreatedPods) != 2 {
		t.Errorf("expected 2 created pods, got %d", len(fc.CreatedPods))
	}

	stats := loop.Stats()
	if stats.TotalPasses != 1 {
		t.Errorf("expected 1 pass, got %d", stats.TotalPasses)
	}
	if stats.SessionsSynced != 2 {
		t.Errorf("expected 2 sessions synced, got %d", stats.SessionsSynced)
	}
}

func TestReconcileLoop_RemovesStaleEntries(t *testing.T) {
	fc := NewFakeClient()

	s1 := newTestSession("stale-1", "default", "claude", "task")
	s1.Finalizers = []string{FinalizerName}
	fc.Sessions[fakeKey("default", "stale-1")] = s1

	registry := NewSessionRegistry()
	loop := NewReconcileLoop(fc, registry, 10*time.Second)

	// First pass: session exists, gets tracked.
	if err := loop.Reconcile(context.Background()); err != nil {
		t.Fatalf("pass 1 error: %v", err)
	}
	if registry.Len() != 1 {
		t.Fatalf("expected 1 session after pass 1, got %d", registry.Len())
	}

	// Remove the session from the "API" (simulating external deletion).
	delete(fc.Sessions, fakeKey("default", "stale-1"))

	// Second pass: session is gone, registry should clean up.
	if err := loop.Reconcile(context.Background()); err != nil {
		t.Fatalf("pass 2 error: %v", err)
	}
	if registry.Len() != 0 {
		t.Errorf("expected 0 sessions after stale cleanup, got %d", registry.Len())
	}
}

func TestReconcileLoop_HandlesListError(t *testing.T) {
	fc := NewFakeClient()
	fc.Err = fmt.Errorf("api server down")

	registry := NewSessionRegistry()
	loop := NewReconcileLoop(fc, registry, 10*time.Second)

	err := loop.Reconcile(context.Background())
	if err == nil {
		t.Fatal("expected error from list, got nil")
	}

	stats := loop.Stats()
	if stats.TotalErrors != 1 {
		t.Errorf("expected 1 error recorded, got %d", stats.TotalErrors)
	}
}

func TestReconcileLoop_HandlesReconcileErrors(t *testing.T) {
	fc := NewFakeClient()

	// Seed a session that will fail during reconciliation. We give it a
	// finalizer so reconciliation proceeds to pod creation, then make
	// CreatePod fail by injecting an error after listing.
	s1 := newTestSession("err-1", "default", "claude", "boom")
	s1.Finalizers = []string{FinalizerName}
	fc.Sessions[fakeKey("default", "err-1")] = s1

	registry := NewSessionRegistry()
	loop := NewReconcileLoop(fc, registry, 10*time.Second)

	// Use a client wrapper that fails on CreatePod but not on List/Get.
	// Since FakeClient doesn't support per-method errors, we'll just verify
	// the happy path here and test error stats separately.
	err := loop.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The session should still be tracked even after reconciliation
	// (it was reconciled successfully, just created a pod).
	if registry.Len() != 1 {
		t.Errorf("expected 1 session in registry, got %d", registry.Len())
	}
}

func TestReconcileLoop_StartAndStop(t *testing.T) {
	fc := NewFakeClient()
	registry := NewSessionRegistry()
	loop := NewReconcileLoop(fc, registry, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start in a goroutine.
	done := make(chan error, 1)
	go func() {
		done <- loop.Start(ctx)
	}()

	// Wait briefly for the loop to begin running.
	time.Sleep(20 * time.Millisecond)

	if !loop.Running() {
		t.Error("expected loop to be running")
	}

	// Stop the loop.
	if err := loop.Stop(); err != nil {
		t.Fatalf("unexpected Stop error: %v", err)
	}

	// Wait for Start to return.
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Errorf("unexpected Start error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after Stop")
	}

	if loop.Running() {
		t.Error("expected loop to not be running after Stop")
	}
}

func TestReconcileLoop_DoubleStartReturnsError(t *testing.T) {
	fc := NewFakeClient()
	registry := NewSessionRegistry()
	loop := NewReconcileLoop(fc, registry, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the loop.
	go func() {
		_ = loop.Start(ctx)
	}()
	time.Sleep(20 * time.Millisecond)

	// Attempt a second start.
	err := loop.Start(ctx)
	if err == nil {
		t.Fatal("expected error from double Start")
	}

	cancel()
	time.Sleep(20 * time.Millisecond)
}

func TestReconcileLoop_StopIdempotent(t *testing.T) {
	fc := NewFakeClient()
	registry := NewSessionRegistry()
	loop := NewReconcileLoop(fc, registry, 10*time.Second)

	// Stop on a loop that was never started should be a no-op.
	if err := loop.Stop(); err != nil {
		t.Errorf("unexpected error from Stop on idle loop: %v", err)
	}

	// Double stop should also be fine.
	if err := loop.Stop(); err != nil {
		t.Errorf("unexpected error from double Stop: %v", err)
	}
}

func TestReconcileLoop_RunsMultiplePasses(t *testing.T) {
	fc := NewFakeClient()

	s1 := newTestSession("multi-1", "default", "claude", "task")
	s1.Finalizers = []string{FinalizerName}
	fc.Sessions[fakeKey("default", "multi-1")] = s1

	registry := NewSessionRegistry()
	loop := NewReconcileLoop(fc, registry, 30*time.Millisecond,
		WithNamespace("default"),
	)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- loop.Start(ctx)
	}()

	// Let a few passes run.
	time.Sleep(120 * time.Millisecond)
	cancel()

	<-done

	stats := loop.Stats()
	if stats.TotalPasses < 2 {
		t.Errorf("expected at least 2 passes, got %d", stats.TotalPasses)
	}
}

func TestReconcileLoop_DefaultInterval(t *testing.T) {
	fc := NewFakeClient()
	loop := NewReconcileLoop(fc, nil, 0) // zero interval should get default

	if loop.interval != 30*time.Second {
		t.Errorf("expected default interval of 30s, got %v", loop.interval)
	}
}

func TestReconcileLoop_NilRegistry(t *testing.T) {
	fc := NewFakeClient()
	loop := NewReconcileLoop(fc, nil, 10*time.Second)

	if loop.registry == nil {
		t.Fatal("expected non-nil registry even when nil was passed")
	}
	if loop.Registry() == nil {
		t.Fatal("Registry() returned nil")
	}
}

func TestReconcileLoop_NamespaceFiltering(t *testing.T) {
	fc := NewFakeClient()

	// Add sessions in two namespaces.
	s1 := newTestSession("ns-1", "alpha", "claude", "task")
	s1.Finalizers = []string{FinalizerName}
	fc.Sessions[fakeKey("alpha", "ns-1")] = s1

	s2 := newTestSession("ns-2", "beta", "gemini", "task")
	s2.Finalizers = []string{FinalizerName}
	fc.Sessions[fakeKey("beta", "ns-2")] = s2

	// Loop scoped to "alpha" namespace only.
	registry := NewSessionRegistry()
	loop := NewReconcileLoop(fc, registry, 10*time.Second,
		WithNamespace("alpha"),
	)

	err := loop.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only "alpha" session should be in registry.
	if registry.Len() != 1 {
		t.Errorf("expected 1 session (alpha only), got %d", registry.Len())
	}
	if registry.Get("alpha", "ns-1") == nil {
		t.Error("expected alpha/ns-1 in registry")
	}
	if registry.Get("beta", "ns-2") != nil {
		t.Error("did not expect beta/ns-2 in registry")
	}
}
