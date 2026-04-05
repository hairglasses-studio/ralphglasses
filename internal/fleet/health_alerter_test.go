package fleet

import (
	"sync"
	"testing"
)

func TestHealthAlertWatcher_StateTransitionFires(t *testing.T) {
	ht := NewHealthTracker(DefaultHealthConfig())
	w := NewHealthAlertWatcher(ht)

	var got []struct{ from, to HealthState }
	w.OnTransition(func(_ string, from, to HealthState) {
		got = append(got, struct{ from, to HealthState }{from, to})
	})

	// First heartbeat: unknown -> healthy.
	ht.RecordHeartbeat("w1")
	w.Check("w1")

	if len(got) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(got))
	}
	if got[0].to != HealthHealthy {
		t.Fatalf("expected transition to healthy, got %s", got[0].to)
	}
}

func TestHealthAlertWatcher_Dedup(t *testing.T) {
	ht := NewHealthTracker(DefaultHealthConfig())
	w := NewHealthAlertWatcher(ht)

	count := 0
	w.OnTransition(func(_ string, _, _ HealthState) {
		count++
	})

	ht.RecordHeartbeat("w1")
	w.Check("w1") // fires (first observation)
	w.Check("w1") // same state — should not fire
	w.Check("w1") // same state — should not fire

	if count != 1 {
		t.Fatalf("expected 1 callback (dedup), got %d", count)
	}
}

func TestHealthAlertWatcher_TransitionSequence(t *testing.T) {
	cfg := DefaultHealthConfig()
	cfg.DegradedAfterMisses = 1
	cfg.UnhealthyAfterMisses = 3
	ht := NewHealthTracker(cfg)
	w := NewHealthAlertWatcher(ht)

	type transition struct {
		workerID string
		from     HealthState
		to       HealthState
	}
	var transitions []transition
	w.OnTransition(func(id string, from, to HealthState) {
		transitions = append(transitions, transition{id, from, to})
	})

	ht.RecordHeartbeat("w1")
	w.Check("w1") // -> healthy

	ht.RecordMiss("w1") // -> degraded
	w.Check("w1")

	ht.RecordMiss("w1") // still degraded
	w.Check("w1")       // should NOT fire

	ht.RecordMiss("w1") // -> unhealthy
	w.Check("w1")

	ht.RecordHeartbeat("w1") // -> healthy
	w.Check("w1")

	expected := []transition{
		{"w1", "", HealthHealthy},
		{"w1", HealthHealthy, HealthDegraded},
		{"w1", HealthDegraded, HealthUnhealthy},
		{"w1", HealthUnhealthy, HealthHealthy},
	}

	if len(transitions) != len(expected) {
		t.Fatalf("expected %d transitions, got %d: %+v", len(expected), len(transitions), transitions)
	}
	for i, e := range expected {
		if transitions[i] != e {
			t.Errorf("transition[%d]: expected %+v, got %+v", i, e, transitions[i])
		}
	}
}

func TestHealthAlertWatcher_MultipleCallbacks(t *testing.T) {
	ht := NewHealthTracker(DefaultHealthConfig())
	w := NewHealthAlertWatcher(ht)

	count1, count2 := 0, 0
	w.OnTransition(func(_ string, _, _ HealthState) { count1++ })
	w.OnTransition(func(_ string, _, _ HealthState) { count2++ })

	ht.RecordHeartbeat("w1")
	w.Check("w1")

	if count1 != 1 || count2 != 1 {
		t.Fatalf("expected both callbacks to fire once, got %d and %d", count1, count2)
	}
}

func TestHealthAlertWatcher_CheckAll(t *testing.T) {
	cfg := DefaultHealthConfig()
	cfg.DegradedAfterMisses = 1
	ht := NewHealthTracker(cfg)
	w := NewHealthAlertWatcher(ht)

	var mu sync.Mutex
	seen := make(map[string]HealthState)
	w.OnTransition(func(id string, _, to HealthState) {
		mu.Lock()
		seen[id] = to
		mu.Unlock()
	})

	ht.RecordHeartbeat("w1")
	ht.RecordHeartbeat("w2")
	ht.RecordHeartbeat("w3")

	// Degrade w2.
	ht.RecordMiss("w2")

	w.CheckAll()

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 3 {
		t.Fatalf("expected 3 transitions, got %d: %v", len(seen), seen)
	}
	if seen["w1"] != HealthHealthy {
		t.Errorf("w1: expected healthy, got %s", seen["w1"])
	}
	if seen["w2"] != HealthDegraded {
		t.Errorf("w2: expected degraded, got %s", seen["w2"])
	}
	if seen["w3"] != HealthHealthy {
		t.Errorf("w3: expected healthy, got %s", seen["w3"])
	}
}

func TestHealthAlertWatcher_UntrackedWorker(t *testing.T) {
	ht := NewHealthTracker(DefaultHealthConfig())
	w := NewHealthAlertWatcher(ht)

	var got HealthState
	w.OnTransition(func(_ string, _, to HealthState) {
		got = to
	})

	// Checking an untracked worker should fire with HealthUnknown.
	w.Check("ghost")
	if got != HealthUnknown {
		t.Fatalf("expected unknown for untracked worker, got %s", got)
	}

	// Second check should NOT fire (still unknown).
	got = ""
	w.Check("ghost")
	if got != "" {
		t.Fatalf("expected no callback on duplicate check, got %s", got)
	}
}
