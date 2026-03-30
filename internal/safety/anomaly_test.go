package safety

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// helper: create a bus + detector with short windows for testing.
func testDetector(t *testing.T, killSwitch bool) (*events.Bus, *AnomalyDetector, *KillSwitch) {
	t.Helper()
	bus := events.NewBus(1000)
	ks := NewKillSwitch(bus)
	cfg := AnomalyConfig{
		WindowSize:         10 * time.Second,
		CostSpikeThreshold: 3.0,
		ErrorRateThreshold: 0.5,
		DurationMultiplier: 2.0,
		CascadeCount:       3,
		CascadeWindow:      10 * time.Second,
		KillSwitchEnabled:  killSwitch,
	}
	det := NewAnomalyDetector(bus, cfg, ks)
	return bus, det, ks
}

func TestCostSpikeDetection(t *testing.T) {
	bus, det, _ := testDetector(t, false)

	var detected []Anomaly
	var mu sync.Mutex
	det.OnAnomaly(func(a Anomaly) {
		mu.Lock()
		detected = append(detected, a)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	det.Start(ctx)

	now := time.Now()

	// Build a baseline with normal costs
	for i := 0; i < 5; i++ {
		bus.Publish(events.Event{
			Type:      events.CostUpdate,
			Timestamp: now.Add(time.Duration(i) * time.Second),
			SessionID: "s1",
			Data:      map[string]any{"spent_usd": 0.10},
		})
	}

	// Send a spike: 4x the average
	bus.Publish(events.Event{
		Type:      events.CostUpdate,
		Timestamp: now.Add(6 * time.Second),
		SessionID: "s1",
		Data:      map[string]any{"spent_usd": 0.40},
	})

	// Give the goroutine time to process
	time.Sleep(50 * time.Millisecond)
	det.Stop()

	mu.Lock()
	defer mu.Unlock()
	if len(detected) == 0 {
		t.Fatal("expected cost spike anomaly, got none")
	}
	if detected[0].Type != CostSpike {
		t.Errorf("expected CostSpike, got %s", detected[0].Type)
	}
}

func TestErrorStormDetection(t *testing.T) {
	bus, det, _ := testDetector(t, false)

	var detected []Anomaly
	var mu sync.Mutex
	det.OnAnomaly(func(a Anomaly) {
		mu.Lock()
		detected = append(detected, a)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	det.Start(ctx)

	now := time.Now()

	// Produce a mix where errors dominate: 3 normal events + 5 errors = 62.5% error rate
	for i := 0; i < 3; i++ {
		bus.Publish(events.Event{
			Type:      events.SessionStarted,
			Timestamp: now.Add(time.Duration(i) * time.Second),
			SessionID: "ok-" + string(rune('a'+i)),
		})
	}
	for i := 0; i < 5; i++ {
		bus.Publish(events.Event{
			Type:      events.SessionError,
			Timestamp: now.Add(time.Duration(3+i) * time.Second),
			SessionID: "err-" + string(rune('a'+i)),
		})
	}

	time.Sleep(50 * time.Millisecond)
	det.Stop()

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, a := range detected {
		if a.Type == ErrorStorm {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ErrorStorm anomaly, got %d anomalies: %+v", len(detected), detected)
	}
}

func TestCascadeFailureDetection(t *testing.T) {
	bus, det, _ := testDetector(t, false)

	var detected []Anomaly
	var mu sync.Mutex
	det.OnAnomaly(func(a Anomaly) {
		mu.Lock()
		detected = append(detected, a)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	det.Start(ctx)

	now := time.Now()

	// 3 budget-exceeded failures within the cascade window
	for i := 0; i < 3; i++ {
		bus.Publish(events.Event{
			Type:      events.BudgetExceeded,
			Timestamp: now.Add(time.Duration(i) * time.Second),
			SessionID: "s" + string(rune('1'+i)),
		})
	}

	time.Sleep(50 * time.Millisecond)
	det.Stop()

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, a := range detected {
		if a.Type == CascadeFailure {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected CascadeFailure anomaly, got %d anomalies: %+v", len(detected), detected)
	}
}

func TestKillSwitchEngageDisengage(t *testing.T) {
	bus := events.NewBus(100)
	ks := NewKillSwitch(bus)

	if ks.IsEngaged() {
		t.Fatal("expected kill switch to start disengaged")
	}
	if ks.EngagedAt() != nil {
		t.Fatal("expected nil EngagedAt when disengaged")
	}
	if ks.Reason() != "" {
		t.Fatal("expected empty reason when disengaged")
	}

	// Engage
	ks.Engage("test reason")
	if !ks.IsEngaged() {
		t.Fatal("expected kill switch to be engaged")
	}
	if ks.Reason() != "test reason" {
		t.Errorf("expected reason 'test reason', got %q", ks.Reason())
	}
	at := ks.EngagedAt()
	if at == nil {
		t.Fatal("expected non-nil EngagedAt after engage")
	}

	// Engage again (no-op)
	ks.Engage("second reason")
	if ks.Reason() != "test reason" {
		t.Error("double engage should be a no-op")
	}

	// Check EmergencyStop event was published
	hist := bus.History(events.EmergencyStop, 10)
	if len(hist) != 1 {
		t.Errorf("expected 1 EmergencyStop event, got %d", len(hist))
	}

	// Disengage
	ks.Disengage()
	if ks.IsEngaged() {
		t.Fatal("expected kill switch to be disengaged")
	}
	if ks.EngagedAt() != nil {
		t.Fatal("expected nil EngagedAt after disengage")
	}
	if ks.Reason() != "" {
		t.Fatal("expected empty reason after disengage")
	}

	// Check EmergencyResume event
	hist = bus.History(events.EmergencyResume, 10)
	if len(hist) != 1 {
		t.Errorf("expected 1 EmergencyResume event, got %d", len(hist))
	}

	// Disengage again (no-op)
	ks.Disengage()
	hist = bus.History(events.EmergencyResume, 10)
	if len(hist) != 1 {
		t.Error("double disengage should be a no-op")
	}
}

func TestCallbackFiresOnAnomaly(t *testing.T) {
	bus, det, _ := testDetector(t, false)

	callCount := 0
	var mu sync.Mutex
	det.OnAnomaly(func(a Anomaly) {
		mu.Lock()
		callCount++
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	det.Start(ctx)

	now := time.Now()

	// Trigger an error storm to fire the callback
	for i := 0; i < 3; i++ {
		bus.Publish(events.Event{
			Type:      events.SessionStarted,
			Timestamp: now.Add(time.Duration(i) * time.Second),
			SessionID: "s" + string(rune('a'+i)),
		})
	}
	for i := 0; i < 5; i++ {
		bus.Publish(events.Event{
			Type:      events.SessionError,
			Timestamp: now.Add(time.Duration(3+i) * time.Second),
			SessionID: "e" + string(rune('a'+i)),
		})
	}

	time.Sleep(50 * time.Millisecond)
	det.Stop()

	mu.Lock()
	defer mu.Unlock()
	if callCount == 0 {
		t.Fatal("expected at least one callback invocation")
	}
}

func TestNoFalsePositivesDuringNormalOperation(t *testing.T) {
	bus, det, _ := testDetector(t, false)

	var detected []Anomaly
	var mu sync.Mutex
	det.OnAnomaly(func(a Anomaly) {
		mu.Lock()
		detected = append(detected, a)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	det.Start(ctx)

	now := time.Now()

	// Normal operation: steady costs, few errors, sessions complete on time
	for i := 0; i < 10; i++ {
		ts := now.Add(time.Duration(i) * time.Second)
		bus.Publish(events.Event{
			Type:      events.SessionStarted,
			Timestamp: ts,
			SessionID: "s" + string(rune('0'+i)),
		})
		bus.Publish(events.Event{
			Type:      events.CostUpdate,
			Timestamp: ts,
			SessionID: "s" + string(rune('0'+i)),
			Data:      map[string]any{"spent_usd": 0.05},
		})
		bus.Publish(events.Event{
			Type:      events.SessionEnded,
			Timestamp: ts.Add(100 * time.Millisecond),
			SessionID: "s" + string(rune('0'+i)),
			Data:      map[string]any{"expected_duration_sec": 3600.0},
		})
	}

	time.Sleep(50 * time.Millisecond)
	det.Stop()

	mu.Lock()
	defer mu.Unlock()
	if len(detected) > 0 {
		t.Errorf("expected no anomalies during normal operation, got %d: %+v", len(detected), detected)
	}
}

func TestKillSwitchAutoEngageOnCritical(t *testing.T) {
	bus, det, ks := testDetector(t, true) // killSwitch enabled

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	det.Start(ctx)

	now := time.Now()

	// Trigger a cascade failure (critical severity)
	for i := 0; i < 3; i++ {
		bus.Publish(events.Event{
			Type:      events.BudgetExceeded,
			Timestamp: now.Add(time.Duration(i) * time.Second),
			SessionID: "fail-" + string(rune('a'+i)),
		})
	}

	time.Sleep(50 * time.Millisecond)
	det.Stop()

	if !ks.IsEngaged() {
		t.Fatal("expected kill switch to be auto-engaged on critical anomaly")
	}
}

func TestKillSwitchNotEngagedWhenDisabled(t *testing.T) {
	bus, det, ks := testDetector(t, false) // killSwitch disabled

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	det.Start(ctx)

	now := time.Now()

	// Trigger a cascade failure (critical severity)
	for i := 0; i < 3; i++ {
		bus.Publish(events.Event{
			Type:      events.BudgetExceeded,
			Timestamp: now.Add(time.Duration(i) * time.Second),
			SessionID: "fail-" + string(rune('a'+i)),
		})
	}

	time.Sleep(50 * time.Millisecond)
	det.Stop()

	if ks.IsEngaged() {
		t.Fatal("kill switch should not engage when KillSwitchEnabled is false")
	}
}
