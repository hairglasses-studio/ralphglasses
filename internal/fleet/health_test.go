package fleet

import (
	"testing"
)

func TestNewHealthTracker(t *testing.T) {
	ht := NewHealthTracker(DefaultHealthConfig())
	if ht == nil {
		t.Fatal("expected non-nil tracker")
	}
	if len(ht.workers) != 0 {
		t.Fatalf("expected empty workers map, got %d", len(ht.workers))
	}
	if ht.config.DegradedAfterMisses != 2 {
		t.Fatalf("expected DegradedAfterMisses=2, got %d", ht.config.DegradedAfterMisses)
	}
}

func TestRecordHeartbeat_UnknownToHealthy(t *testing.T) {
	ht := NewHealthTracker(DefaultHealthConfig())

	// First heartbeat should create worker and transition unknown -> healthy.
	ht.RecordHeartbeat("w1")

	state := ht.GetState("w1")
	if state != HealthHealthy {
		t.Fatalf("expected healthy, got %s", state)
	}

	h, ok := ht.GetHealth("w1")
	if !ok {
		t.Fatal("expected worker health to exist")
	}
	if len(h.History) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(h.History))
	}
	if h.History[0].From != HealthUnknown || h.History[0].To != HealthHealthy {
		t.Fatalf("expected unknown->healthy, got %s->%s", h.History[0].From, h.History[0].To)
	}
}

func TestRecordMiss_DegradedThenUnhealthy(t *testing.T) {
	cfg := DefaultHealthConfig()
	cfg.DegradedAfterMisses = 2
	cfg.UnhealthyAfterMisses = 4
	ht := NewHealthTracker(cfg)

	ht.RecordHeartbeat("w1")
	if ht.GetState("w1") != HealthHealthy {
		t.Fatal("expected healthy after heartbeat")
	}

	// 1 miss: still healthy.
	ht.RecordMiss("w1")
	if ht.GetState("w1") != HealthHealthy {
		t.Fatalf("expected healthy after 1 miss, got %s", ht.GetState("w1"))
	}

	// 2 misses: degraded.
	ht.RecordMiss("w1")
	if ht.GetState("w1") != HealthDegraded {
		t.Fatalf("expected degraded after 2 misses, got %s", ht.GetState("w1"))
	}

	// 3 misses: still degraded.
	ht.RecordMiss("w1")
	if ht.GetState("w1") != HealthDegraded {
		t.Fatalf("expected degraded after 3 misses, got %s", ht.GetState("w1"))
	}

	// 4 misses: unhealthy.
	ht.RecordMiss("w1")
	if ht.GetState("w1") != HealthUnhealthy {
		t.Fatalf("expected unhealthy after 4 misses, got %s", ht.GetState("w1"))
	}
}

func TestHealthyWorkers(t *testing.T) {
	ht := NewHealthTracker(DefaultHealthConfig())

	ht.RecordHeartbeat("w1")
	ht.RecordHeartbeat("w2")
	ht.RecordHeartbeat("w3")

	// Degrade w2.
	for i := 0; i < 3; i++ {
		ht.RecordMiss("w2")
	}

	healthy := ht.HealthyWorkers()
	if len(healthy) != 2 {
		t.Fatalf("expected 2 healthy workers, got %d", len(healthy))
	}

	healthySet := make(map[string]bool)
	for _, id := range healthy {
		healthySet[id] = true
	}
	if !healthySet["w1"] || !healthySet["w3"] {
		t.Fatalf("expected w1 and w3 to be healthy, got %v", healthy)
	}
	if healthySet["w2"] {
		t.Fatal("w2 should not be healthy")
	}
}

func TestGetState_UntrackedWorker(t *testing.T) {
	ht := NewHealthTracker(DefaultHealthConfig())
	state := ht.GetState("nonexistent")
	if state != HealthUnknown {
		t.Fatalf("expected unknown for untracked worker, got %s", state)
	}
}

func TestTransitionHistory_Capped(t *testing.T) {
	cfg := DefaultHealthConfig()
	cfg.MaxHistory = 3
	cfg.DegradedAfterMisses = 1
	cfg.UnhealthyAfterMisses = 2
	ht := NewHealthTracker(cfg)

	// Generate many transitions by cycling through states.
	// Each heartbeat+miss cycle generates transitions.
	for i := 0; i < 10; i++ {
		ht.RecordHeartbeat("w1") // -> healthy
		ht.RecordMiss("w1")     // -> degraded (1 miss)
		ht.RecordMiss("w1")     // -> unhealthy (2 misses)
	}

	h, ok := ht.GetHealth("w1")
	if !ok {
		t.Fatal("expected worker health to exist")
	}
	if len(h.History) > cfg.MaxHistory {
		t.Fatalf("expected history capped at %d, got %d", cfg.MaxHistory, len(h.History))
	}
	if len(h.History) != cfg.MaxHistory {
		t.Fatalf("expected exactly %d history entries, got %d", cfg.MaxHistory, len(h.History))
	}
}

func TestRecordMiss_UntrackedWorkerNoOp(t *testing.T) {
	ht := NewHealthTracker(DefaultHealthConfig())
	// Should not panic or create a worker entry.
	ht.RecordMiss("nonexistent")
	state := ht.GetState("nonexistent")
	if state != HealthUnknown {
		t.Fatalf("expected unknown, got %s", state)
	}
}

func TestHeartbeatResetsToHealthy(t *testing.T) {
	cfg := DefaultHealthConfig()
	cfg.DegradedAfterMisses = 1
	ht := NewHealthTracker(cfg)

	ht.RecordHeartbeat("w1")
	ht.RecordMiss("w1") // -> degraded
	if ht.GetState("w1") != HealthDegraded {
		t.Fatalf("expected degraded, got %s", ht.GetState("w1"))
	}

	ht.RecordHeartbeat("w1") // -> healthy again
	if ht.GetState("w1") != HealthHealthy {
		t.Fatalf("expected healthy after heartbeat, got %s", ht.GetState("w1"))
	}
}
