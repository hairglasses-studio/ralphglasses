package recovery

import (
	"sync"
	"testing"
)

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateHealthy, "Healthy"},
		{StateDegraded, "Degraded"},
		{StateRecovering, "Recovering"},
		{StateFailed, "Failed"},
		{State(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestEventString(t *testing.T) {
	tests := []struct {
		event Event
		want  string
	}{
		{EventServerDown, "ServerDown"},
		{EventServerUp, "ServerUp"},
		{EventRetryExhausted, "RetryExhausted"},
		{EventManualReset, "ManualReset"},
		{Event(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.event.String(); got != tt.want {
			t.Errorf("Event(%d).String() = %q, want %q", tt.event, got, tt.want)
		}
	}
}

func TestNewDefaultState(t *testing.T) {
	m := New(3)
	if s := m.State(); s != StateHealthy {
		t.Fatalf("new machine state = %v, want Healthy", s)
	}
	if f := m.Failures(); f != 0 {
		t.Fatalf("new machine failures = %d, want 0", f)
	}
}

func TestNewClampsMaxFailures(t *testing.T) {
	m := New(0) // should clamp to 1
	if m.maxFailures != 1 {
		t.Fatalf("New(0).maxFailures = %d, want 1", m.maxFailures)
	}
	m2 := New(-5)
	if m2.maxFailures != 1 {
		t.Fatalf("New(-5).maxFailures = %d, want 1", m2.maxFailures)
	}
}

func TestHealthyToDegradedOnServerDown(t *testing.T) {
	m := New(3)
	got := m.Handle(EventServerDown)
	if got != StateDegraded {
		t.Fatalf("Healthy + ServerDown = %v, want Degraded", got)
	}
	if m.Failures() != 1 {
		t.Fatalf("failures = %d, want 1", m.Failures())
	}
}

func TestDegradedToRecoveringOnRepeatedServerDown(t *testing.T) {
	m := New(3)
	m.Handle(EventServerDown) // Healthy → Degraded (failures=1)
	m.Handle(EventServerDown) // Degraded, failures=2
	got := m.Handle(EventServerDown) // failures=3 >= maxFailures → Recovering
	if got != StateRecovering {
		t.Fatalf("Degraded + ServerDown (failures>=max) = %v, want Recovering", got)
	}
}

func TestDegradedStaysDegradedBelowMax(t *testing.T) {
	m := New(5)
	m.Handle(EventServerDown) // → Degraded (failures=1)
	got := m.Handle(EventServerDown) // failures=2, still below 5
	if got != StateDegraded {
		t.Fatalf("Degraded + ServerDown (below max) = %v, want Degraded", got)
	}
}

func TestRecoveringToFailedOnRetryExhausted(t *testing.T) {
	m := New(2)
	m.Handle(EventServerDown) // → Degraded
	m.Handle(EventServerDown) // → Recovering
	got := m.Handle(EventRetryExhausted)
	if got != StateFailed {
		t.Fatalf("Recovering + RetryExhausted = %v, want Failed", got)
	}
}

func TestRetryExhaustedIgnoredInOtherStates(t *testing.T) {
	m := New(3)
	// Healthy: RetryExhausted should be a no-op
	got := m.Handle(EventRetryExhausted)
	if got != StateHealthy {
		t.Fatalf("Healthy + RetryExhausted = %v, want Healthy", got)
	}

	m.Handle(EventServerDown) // → Degraded
	got = m.Handle(EventRetryExhausted)
	if got != StateDegraded {
		t.Fatalf("Degraded + RetryExhausted = %v, want Degraded", got)
	}
}

func TestFailedToHealthyOnManualReset(t *testing.T) {
	m := New(2)
	m.Handle(EventServerDown) // → Degraded
	m.Handle(EventServerDown) // → Recovering
	m.Handle(EventRetryExhausted) // → Failed
	got := m.Handle(EventManualReset)
	if got != StateHealthy {
		t.Fatalf("Failed + ManualReset = %v, want Healthy", got)
	}
	if m.Failures() != 0 {
		t.Fatalf("failures after reset = %d, want 0", m.Failures())
	}
}

func TestManualResetFromAnyState(t *testing.T) {
	for _, setup := range []struct {
		name   string
		events []Event
		before State
	}{
		{"Healthy", nil, StateHealthy},
		{"Degraded", []Event{EventServerDown}, StateDegraded},
		{"Recovering", []Event{EventServerDown, EventServerDown}, StateRecovering},
		{"Failed", []Event{EventServerDown, EventServerDown, EventRetryExhausted}, StateFailed},
	} {
		t.Run(setup.name, func(t *testing.T) {
			m := New(2)
			for _, e := range setup.events {
				m.Handle(e)
			}
			if s := m.State(); s != setup.before {
				t.Fatalf("setup state = %v, want %v", s, setup.before)
			}
			got := m.Handle(EventManualReset)
			if got != StateHealthy {
				t.Fatalf("%v + ManualReset = %v, want Healthy", setup.before, got)
			}
		})
	}
}

func TestServerUpFromDegradedToHealthy(t *testing.T) {
	m := New(3)
	m.Handle(EventServerDown) // → Degraded, failures=1
	got := m.Handle(EventServerUp) // failures=0 → Healthy
	if got != StateHealthy {
		t.Fatalf("Degraded + ServerUp (no remaining failures) = %v, want Healthy", got)
	}
}

func TestServerUpFromDegradedStaysDegraded(t *testing.T) {
	m := New(5)
	m.Handle(EventServerDown) // failures=1
	m.Handle(EventServerDown) // failures=2
	got := m.Handle(EventServerUp) // failures=1, still degraded
	if got != StateDegraded {
		t.Fatalf("Degraded + ServerUp (remaining failures) = %v, want Degraded", got)
	}
}

func TestServerUpFromRecoveringToHealthy(t *testing.T) {
	m := New(1)
	m.Handle(EventServerDown) // → Degraded, failures=1 >= 1 → Recovering
	// Actually with maxFailures=1, first ServerDown goes Healthy→Degraded (failures=1 >= 1? No,
	// the transition in Healthy always goes to Degraded first).
	// Let's use maxFailures=2 for clarity.
	m2 := New(2)
	m2.Handle(EventServerDown) // → Degraded, failures=1
	m2.Handle(EventServerDown) // failures=2 >= 2 → Recovering
	// Now send two ServerUp to drain failures
	m2.Handle(EventServerUp) // failures=1 → Degraded
	got := m2.Handle(EventServerUp) // failures=0 → Healthy
	if got != StateHealthy {
		t.Fatalf("Recovering → Degraded → Healthy via ServerUp = %v, want Healthy", got)
	}
}

func TestServerUpFromRecoveringToDegraded(t *testing.T) {
	m := New(2)
	m.Handle(EventServerDown) // → Degraded, failures=1
	m.Handle(EventServerDown) // → Recovering, failures=2
	m.Handle(EventServerDown) // still Recovering, failures=3
	got := m.Handle(EventServerUp) // failures=2, still > 0 → Degraded
	if got != StateDegraded {
		t.Fatalf("Recovering + ServerUp (remaining failures) = %v, want Degraded", got)
	}
}

func TestServerUpInFailedIsIgnored(t *testing.T) {
	m := New(2)
	m.Handle(EventServerDown)
	m.Handle(EventServerDown)
	m.Handle(EventRetryExhausted) // → Failed
	got := m.Handle(EventServerUp)
	if got != StateFailed {
		t.Fatalf("Failed + ServerUp = %v, want Failed (requires ManualReset)", got)
	}
}

func TestServerUpInHealthyIsNoop(t *testing.T) {
	m := New(3)
	got := m.Handle(EventServerUp)
	if got != StateHealthy {
		t.Fatalf("Healthy + ServerUp = %v, want Healthy", got)
	}
	if m.Failures() != 0 {
		t.Fatalf("failures = %d, want 0", m.Failures())
	}
}

func TestResetMethod(t *testing.T) {
	m := New(2)
	m.Handle(EventServerDown)
	m.Handle(EventServerDown)
	m.Handle(EventRetryExhausted)
	m.Reset()
	if s := m.State(); s != StateHealthy {
		t.Fatalf("after Reset() state = %v, want Healthy", s)
	}
	if f := m.Failures(); f != 0 {
		t.Fatalf("after Reset() failures = %d, want 0", f)
	}
}

func TestOnTransitionCallback(t *testing.T) {
	m := New(2)
	var transitions []struct{ from, to State }
	m.OnTransition(func(from, to State) {
		transitions = append(transitions, struct{ from, to State }{from, to})
	})

	m.Handle(EventServerDown) // Healthy → Degraded
	m.Handle(EventServerDown) // Degraded → Recovering
	m.Handle(EventRetryExhausted) // Recovering → Failed
	m.Handle(EventManualReset) // Failed → Healthy

	expected := []struct{ from, to State }{
		{StateHealthy, StateDegraded},
		{StateDegraded, StateRecovering},
		{StateRecovering, StateFailed},
		{StateFailed, StateHealthy},
	}

	if len(transitions) != len(expected) {
		t.Fatalf("got %d transitions, want %d", len(transitions), len(expected))
	}
	for i, exp := range expected {
		if transitions[i] != exp {
			t.Errorf("transition[%d] = {%v→%v}, want {%v→%v}",
				i, transitions[i].from, transitions[i].to, exp.from, exp.to)
		}
	}
}

func TestOnTransitionNotCalledForSameState(t *testing.T) {
	m := New(5)
	called := false
	m.OnTransition(func(from, to State) {
		called = true
	})

	// ServerUp when already Healthy — no state change
	m.Handle(EventServerUp)
	if called {
		t.Fatal("onTransition called when state did not change")
	}

	// RetryExhausted when Healthy — no state change
	m.Handle(EventRetryExhausted)
	if called {
		t.Fatal("onTransition called when state did not change")
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := New(100)
	var wg sync.WaitGroup
	events := []Event{EventServerDown, EventServerUp, EventServerDown, EventManualReset}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				evt := events[(id+j)%len(events)]
				m.Handle(evt)
				_ = m.State()
				_ = m.Failures()
			}
		}(i)
	}
	wg.Wait()

	// Machine should be in a valid state after concurrent hammering.
	s := m.State()
	if s < StateHealthy || s > StateFailed {
		t.Fatalf("invalid state after concurrent access: %v", s)
	}
}

func TestFullRecoveryCycle(t *testing.T) {
	// Walk through a complete lifecycle:
	// Healthy → Degraded → Recovering → Failed → (ManualReset) → Healthy
	m := New(3)

	assertState := func(want State) {
		t.Helper()
		if got := m.State(); got != want {
			t.Fatalf("state = %v, want %v", got, want)
		}
	}

	assertState(StateHealthy)

	m.Handle(EventServerDown) // failures=1
	assertState(StateDegraded)

	m.Handle(EventServerDown) // failures=2
	assertState(StateDegraded)

	m.Handle(EventServerDown) // failures=3 >= 3 → Recovering
	assertState(StateRecovering)

	m.Handle(EventRetryExhausted)
	assertState(StateFailed)

	// ServerUp cannot escape Failed
	m.Handle(EventServerUp)
	assertState(StateFailed)

	m.Handle(EventManualReset)
	assertState(StateHealthy)

	// Back to Healthy, can degrade again
	m.Handle(EventServerDown)
	assertState(StateDegraded)

	// And recover via ServerUp
	m.Handle(EventServerUp)
	assertState(StateHealthy)
}
