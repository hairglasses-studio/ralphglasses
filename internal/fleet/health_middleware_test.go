package fleet

import (
	"errors"
	"testing"
)

func TestHealthCircuitBreaker_AllowHealthy(t *testing.T) {
	ht := NewHealthTracker(DefaultHealthConfig())
	dispatch := HealthCircuitBreaker(ht)

	ht.RecordHeartbeat("w1") // healthy

	called := false
	err := dispatch("w1", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error for healthy worker, got %v", err)
	}
	if !called {
		t.Fatal("expected fn to be called for healthy worker")
	}
}

func TestHealthCircuitBreaker_RejectUnhealthy(t *testing.T) {
	cfg := DefaultHealthConfig()
	cfg.DegradedAfterMisses = 1
	cfg.UnhealthyAfterMisses = 2
	ht := NewHealthTracker(cfg)
	dispatch := HealthCircuitBreaker(ht)

	ht.RecordHeartbeat("w1")
	ht.RecordMiss("w1") // degraded
	ht.RecordMiss("w1") // unhealthy

	called := false
	err := dispatch("w1", func() error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("expected error for unhealthy worker")
	}
	if !errors.Is(err, ErrWorkerUnhealthy) {
		t.Fatalf("expected ErrWorkerUnhealthy, got %v", err)
	}
	if called {
		t.Fatal("fn should NOT be called for unhealthy worker")
	}
}

func TestHealthCircuitBreaker_AllowDegraded(t *testing.T) {
	cfg := DefaultHealthConfig()
	cfg.DegradedAfterMisses = 1
	cfg.UnhealthyAfterMisses = 5
	ht := NewHealthTracker(cfg)
	dispatch := HealthCircuitBreaker(ht)

	ht.RecordHeartbeat("w1")
	ht.RecordMiss("w1") // degraded

	if ht.GetState("w1") != HealthDegraded {
		t.Fatalf("expected degraded, got %s", ht.GetState("w1"))
	}

	called := false
	err := dispatch("w1", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error for degraded worker, got %v", err)
	}
	if !called {
		t.Fatal("expected fn to be called for degraded worker")
	}
}

func TestHealthCircuitBreaker_AllowUnknown(t *testing.T) {
	ht := NewHealthTracker(DefaultHealthConfig())
	dispatch := HealthCircuitBreaker(ht)

	// Unknown worker — should pass through (not block new workers).
	called := false
	err := dispatch("new-worker", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error for unknown worker, got %v", err)
	}
	if !called {
		t.Fatal("expected fn to be called for unknown worker")
	}
}

func TestHealthCircuitBreaker_PropagatesFnError(t *testing.T) {
	ht := NewHealthTracker(DefaultHealthConfig())
	dispatch := HealthCircuitBreaker(ht)

	ht.RecordHeartbeat("w1")

	fnErr := errors.New("task failed")
	err := dispatch("w1", func() error {
		return fnErr
	})
	if !errors.Is(err, fnErr) {
		t.Fatalf("expected fn error to propagate, got %v", err)
	}
}

func TestHealthCircuitBreaker_RecoveryAfterUnhealthy(t *testing.T) {
	cfg := DefaultHealthConfig()
	cfg.DegradedAfterMisses = 1
	cfg.UnhealthyAfterMisses = 2
	ht := NewHealthTracker(cfg)
	dispatch := HealthCircuitBreaker(ht)

	ht.RecordHeartbeat("w1")
	ht.RecordMiss("w1") // degraded
	ht.RecordMiss("w1") // unhealthy

	// Should reject.
	err := dispatch("w1", func() error { return nil })
	if !errors.Is(err, ErrWorkerUnhealthy) {
		t.Fatalf("expected rejection, got %v", err)
	}

	// Heartbeat recovers the worker.
	ht.RecordHeartbeat("w1")
	if ht.GetState("w1") != HealthHealthy {
		t.Fatalf("expected healthy after recovery, got %s", ht.GetState("w1"))
	}

	// Should now allow.
	called := false
	err = dispatch("w1", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error after recovery, got %v", err)
	}
	if !called {
		t.Fatal("expected fn to be called after recovery")
	}
}
