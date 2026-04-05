package pool

import (
	"sync"
	"testing"
	"time"
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestNewBudgetAlertWatcher_Defaults(t *testing.T) {
	w := NewBudgetAlertWatcher()
	emitted := w.Emitted()
	for _, lvl := range DefaultAlertThresholds {
		if emitted[lvl] {
			t.Errorf("threshold %d should not be emitted on creation", lvl)
		}
	}
	if len(emitted) != len(DefaultAlertThresholds) {
		t.Errorf("emitted map length: got %d, want %d", len(emitted), len(DefaultAlertThresholds))
	}
}

func TestNewBudgetAlertWatcher_CustomThresholds(t *testing.T) {
	w := NewBudgetAlertWatcher(WithThresholds([]AlertLevel{AlertLevel75, AlertLevel100}))
	emitted := w.Emitted()
	if len(emitted) != 2 {
		t.Fatalf("expected 2 thresholds, got %d", len(emitted))
	}
	if _, ok := emitted[AlertLevel75]; !ok {
		t.Error("missing threshold 75")
	}
	if _, ok := emitted[AlertLevel100]; !ok {
		t.Error("missing threshold 100")
	}
}

func TestCheck_NoBudgetCap(t *testing.T) {
	w := NewBudgetAlertWatcher()
	alerts := w.Check(999.0, 0)
	if len(alerts) != 0 {
		t.Errorf("expected no alerts with zero cap, got %d", len(alerts))
	}

	alerts = w.Check(999.0, -1)
	if len(alerts) != 0 {
		t.Errorf("expected no alerts with negative cap, got %d", len(alerts))
	}
}

func TestCheck_SingleThresholdCrossing(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	w := NewBudgetAlertWatcher(WithClock(fixedClock(now)))

	alerts := w.Check(50.0, 100.0)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Level != AlertLevel50 {
		t.Errorf("level: got %d, want %d", a.Level, AlertLevel50)
	}
	if a.SpentUSD != 50.0 {
		t.Errorf("SpentUSD: got %f, want 50.0", a.SpentUSD)
	}
	if a.BudgetUSD != 100.0 {
		t.Errorf("BudgetUSD: got %f, want 100.0", a.BudgetUSD)
	}
	if a.Pct != 50.0 {
		t.Errorf("Pct: got %f, want 50.0", a.Pct)
	}
	if !a.Timestamp.Equal(now) {
		t.Errorf("Timestamp: got %v, want %v", a.Timestamp, now)
	}
}

func TestCheck_MultipleThresholdsCrossedAtOnce(t *testing.T) {
	w := NewBudgetAlertWatcher()

	// Jump straight to 92% — should fire 50, 75, and 90
	alerts := w.Check(92.0, 100.0)
	if len(alerts) != 3 {
		t.Fatalf("expected 3 alerts, got %d", len(alerts))
	}

	levels := make(map[AlertLevel]bool)
	for _, a := range alerts {
		levels[a.Level] = true
	}
	for _, want := range []AlertLevel{AlertLevel50, AlertLevel75, AlertLevel90} {
		if !levels[want] {
			t.Errorf("missing alert for level %d", want)
		}
	}
}

func TestCheck_AllFourThresholds(t *testing.T) {
	w := NewBudgetAlertWatcher()

	// At exactly 100%
	alerts := w.Check(100.0, 100.0)
	if len(alerts) != 4 {
		t.Fatalf("expected 4 alerts at 100%%, got %d", len(alerts))
	}

	levels := make(map[AlertLevel]bool)
	for _, a := range alerts {
		levels[a.Level] = true
	}
	for _, want := range DefaultAlertThresholds {
		if !levels[want] {
			t.Errorf("missing alert for level %d", want)
		}
	}
}

func TestCheck_ThresholdFiresOnlyOnce(t *testing.T) {
	w := NewBudgetAlertWatcher()

	alerts1 := w.Check(50.0, 100.0)
	if len(alerts1) != 1 {
		t.Fatalf("first check: expected 1 alert, got %d", len(alerts1))
	}

	// Same spend — should not fire again
	alerts2 := w.Check(50.0, 100.0)
	if len(alerts2) != 0 {
		t.Errorf("second check: expected 0 alerts, got %d", len(alerts2))
	}

	// Higher spend, same threshold — still no re-fire for 50%
	alerts3 := w.Check(60.0, 100.0)
	if len(alerts3) != 0 {
		t.Errorf("third check at 60%%: expected 0 alerts, got %d", len(alerts3))
	}
}

func TestCheck_ProgressiveCrossings(t *testing.T) {
	w := NewBudgetAlertWatcher()

	// Gradually increase spend
	tests := []struct {
		spent float64
		want  int
		level AlertLevel
	}{
		{30.0, 0, 0},
		{50.0, 1, AlertLevel50},
		{60.0, 0, 0},
		{75.0, 1, AlertLevel75},
		{80.0, 0, 0},
		{90.0, 1, AlertLevel90},
		{95.0, 0, 0},
		{100.0, 1, AlertLevel100},
		{110.0, 0, 0}, // over budget, but 100% already fired
	}

	for _, tt := range tests {
		alerts := w.Check(tt.spent, 100.0)
		if len(alerts) != tt.want {
			t.Errorf("spend=%.0f: got %d alerts, want %d", tt.spent, len(alerts), tt.want)
		}
		if tt.want == 1 && alerts[0].Level != tt.level {
			t.Errorf("spend=%.0f: level got %d, want %d", tt.spent, alerts[0].Level, tt.level)
		}
	}
}

func TestCheck_ExactBoundaryValues(t *testing.T) {
	tests := []struct {
		name  string
		spent float64
		cap   float64
		want  []AlertLevel
	}{
		{"exactly 50%", 5.0, 10.0, []AlertLevel{AlertLevel50}},
		{"just under 50%", 4.99, 10.0, nil},
		{"exactly 75%", 75.0, 100.0, []AlertLevel{AlertLevel50, AlertLevel75}},
		{"just under 75%", 74.99, 100.0, []AlertLevel{AlertLevel50}},
		{"exactly 90%", 9.0, 10.0, []AlertLevel{AlertLevel50, AlertLevel75, AlertLevel90}},
		{"exactly 100%", 10.0, 10.0, []AlertLevel{AlertLevel50, AlertLevel75, AlertLevel90, AlertLevel100}},
		{"over 100%", 12.0, 10.0, []AlertLevel{AlertLevel50, AlertLevel75, AlertLevel90, AlertLevel100}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewBudgetAlertWatcher()
			alerts := w.Check(tt.spent, tt.cap)

			gotLevels := make(map[AlertLevel]bool)
			for _, a := range alerts {
				gotLevels[a.Level] = true
			}

			if len(alerts) != len(tt.want) {
				t.Errorf("got %d alerts, want %d", len(alerts), len(tt.want))
			}
			for _, wantLvl := range tt.want {
				if !gotLevels[wantLvl] {
					t.Errorf("missing alert level %d", wantLvl)
				}
			}
		})
	}
}

func TestCheck_ZeroSpend(t *testing.T) {
	w := NewBudgetAlertWatcher()
	alerts := w.Check(0, 100.0)
	if len(alerts) != 0 {
		t.Errorf("expected no alerts at zero spend, got %d", len(alerts))
	}
}

func TestCheck_SmallBudget(t *testing.T) {
	w := NewBudgetAlertWatcher()
	// $0.01 budget, $0.005 spent = 50%
	alerts := w.Check(0.005, 0.01)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Level != AlertLevel50 {
		t.Errorf("level: got %d, want %d", alerts[0].Level, AlertLevel50)
	}
}

func TestOnAlert_CallbackFired(t *testing.T) {
	w := NewBudgetAlertWatcher()
	var received []BudgetAlert
	w.OnAlert(func(a BudgetAlert) {
		received = append(received, a)
	})

	w.Check(75.0, 100.0) // fires 50% and 75%
	if len(received) != 2 {
		t.Fatalf("callback received %d alerts, want 2", len(received))
	}
	if received[0].Level != AlertLevel50 {
		t.Errorf("first callback level: got %d, want %d", received[0].Level, AlertLevel50)
	}
	if received[1].Level != AlertLevel75 {
		t.Errorf("second callback level: got %d, want %d", received[1].Level, AlertLevel75)
	}
}

func TestOnAlert_MultipleCallbacks(t *testing.T) {
	w := NewBudgetAlertWatcher()
	var count1, count2 int
	w.OnAlert(func(_ BudgetAlert) { count1++ })
	w.OnAlert(func(_ BudgetAlert) { count2++ })

	w.Check(100.0, 100.0) // fires all 4 thresholds
	if count1 != 4 {
		t.Errorf("callback 1: got %d calls, want 4", count1)
	}
	if count2 != 4 {
		t.Errorf("callback 2: got %d calls, want 4", count2)
	}
}

func TestOnAlert_NoCallbackNoPanic(t *testing.T) {
	w := NewBudgetAlertWatcher()
	// No callbacks registered — should not panic
	alerts := w.Check(100.0, 100.0)
	if len(alerts) != 4 {
		t.Errorf("expected 4 alerts, got %d", len(alerts))
	}
}

func TestReset(t *testing.T) {
	w := NewBudgetAlertWatcher()

	// Cross all thresholds
	alerts1 := w.Check(100.0, 100.0)
	if len(alerts1) != 4 {
		t.Fatalf("expected 4 alerts, got %d", len(alerts1))
	}

	// Verify all emitted
	emitted := w.Emitted()
	for _, lvl := range DefaultAlertThresholds {
		if !emitted[lvl] {
			t.Errorf("threshold %d should be emitted", lvl)
		}
	}

	// Reset and verify cleared
	w.Reset()
	emitted = w.Emitted()
	for _, lvl := range DefaultAlertThresholds {
		if emitted[lvl] {
			t.Errorf("threshold %d should be cleared after reset", lvl)
		}
	}

	// Should fire again
	alerts2 := w.Check(100.0, 100.0)
	if len(alerts2) != 4 {
		t.Errorf("after reset: expected 4 alerts, got %d", len(alerts2))
	}
}

func TestEmitted_ReturnsCopy(t *testing.T) {
	w := NewBudgetAlertWatcher()
	w.Check(50.0, 100.0)

	emitted := w.Emitted()
	// Mutate the copy — should not affect internal state
	emitted[AlertLevel90] = true

	internal := w.Emitted()
	if internal[AlertLevel90] {
		t.Error("Emitted() should return a copy, not a reference")
	}
}

func TestCheck_ConcurrentAccess(t *testing.T) {
	w := NewBudgetAlertWatcher()
	var mu sync.Mutex
	var totalAlerts int
	w.OnAlert(func(_ BudgetAlert) {
		mu.Lock()
		totalAlerts++
		mu.Unlock()
	})

	var wg sync.WaitGroup
	// 20 goroutines all checking at 100% — only 4 alerts total should fire
	for range 20 {
		wg.Go(func() {
			w.Check(100.0, 100.0)
		})
	}
	wg.Wait()

	mu.Lock()
	got := totalAlerts
	mu.Unlock()

	if got != 4 {
		t.Errorf("concurrent: expected exactly 4 total callback invocations, got %d", got)
	}
}

func TestAlertLevelConstants(t *testing.T) {
	if AlertLevel50 != 50 {
		t.Errorf("AlertLevel50 = %d, want 50", AlertLevel50)
	}
	if AlertLevel75 != 75 {
		t.Errorf("AlertLevel75 = %d, want 75", AlertLevel75)
	}
	if AlertLevel90 != 90 {
		t.Errorf("AlertLevel90 = %d, want 90", AlertLevel90)
	}
	if AlertLevel100 != 100 {
		t.Errorf("AlertLevel100 = %d, want 100", AlertLevel100)
	}
}

func TestBudgetAlert_Fields(t *testing.T) {
	now := time.Date(2026, 3, 30, 15, 30, 0, 0, time.UTC)
	w := NewBudgetAlertWatcher(WithClock(fixedClock(now)))

	alerts := w.Check(90.0, 100.0)
	// Find the 90% alert
	var found bool
	for _, a := range alerts {
		if a.Level == AlertLevel90 {
			found = true
			if a.SpentUSD != 90.0 {
				t.Errorf("SpentUSD: got %f, want 90.0", a.SpentUSD)
			}
			if a.BudgetUSD != 100.0 {
				t.Errorf("BudgetUSD: got %f, want 100.0", a.BudgetUSD)
			}
			if a.Pct != 90.0 {
				t.Errorf("Pct: got %f, want 90.0", a.Pct)
			}
			if !a.Timestamp.Equal(now) {
				t.Errorf("Timestamp: got %v, want %v", a.Timestamp, now)
			}
		}
	}
	if !found {
		t.Error("90% alert not found in results")
	}
}

func TestWithClock(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	callCount := 0
	clock := func() time.Time {
		callCount++
		if callCount == 1 {
			return t1
		}
		return t2
	}

	w := NewBudgetAlertWatcher(WithClock(clock))

	alerts1 := w.Check(50.0, 100.0)
	if len(alerts1) != 1 || !alerts1[0].Timestamp.Equal(t1) {
		t.Errorf("first check: expected timestamp %v", t1)
	}

	// Move to next threshold with different time
	alerts2 := w.Check(75.0, 100.0)
	if len(alerts2) != 1 || !alerts2[0].Timestamp.Equal(t2) {
		t.Errorf("second check: expected timestamp %v", t2)
	}
}

func TestCheck_OverBudget(t *testing.T) {
	w := NewBudgetAlertWatcher()
	// 150% of budget
	alerts := w.Check(150.0, 100.0)
	if len(alerts) != 4 {
		t.Fatalf("expected 4 alerts at 150%%, got %d", len(alerts))
	}
	// Verify pct is correct
	for _, a := range alerts {
		if a.Pct != 150.0 {
			t.Errorf("level %d: Pct got %f, want 150.0", a.Level, a.Pct)
		}
	}
}

func TestCheck_NonStandardBudgetValues(t *testing.T) {
	w := NewBudgetAlertWatcher()
	// Budget of $3.33, spent $2.50 = ~75.075%
	alerts := w.Check(2.50, 3.33)
	levels := make(map[AlertLevel]bool)
	for _, a := range alerts {
		levels[a.Level] = true
	}
	if !levels[AlertLevel50] {
		t.Error("should have crossed 50% threshold")
	}
	if !levels[AlertLevel75] {
		t.Error("should have crossed 75% threshold")
	}
	if levels[AlertLevel90] {
		t.Error("should not have crossed 90% threshold at ~75%")
	}
}
