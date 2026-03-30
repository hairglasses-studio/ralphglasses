package cloud

import (
	"strings"
	"testing"
	"time"
)

func newTestClock(start time.Time) (func() time.Time, func(d time.Duration)) {
	now := start
	return func() time.Time { return now },
		func(d time.Duration) { now = now.Add(d) }
}

func TestCostTracker_ZeroSpend(t *testing.T) {
	ct := NewCostTracker()

	if got := ct.TotalSpend(); got != 0 {
		t.Fatalf("TotalSpend = %f, want 0", got)
	}
	if got := ct.BurnRate(); got != 0 {
		t.Fatalf("BurnRate = %f, want 0", got)
	}
	if got := ct.ExhaustionTime(); !got.IsZero() {
		t.Fatalf("ExhaustionTime = %v, want zero", got)
	}
}

func TestCostTracker_NegativeAmountIgnored(t *testing.T) {
	ct := NewCostTracker()
	ct.Record(ProviderClaude, "opus-4", -5.0)

	if got := ct.TotalSpend(); got != 0 {
		t.Fatalf("TotalSpend = %f, want 0 (negative should be ignored)", got)
	}
}

func TestCostTracker_SingleEntry(t *testing.T) {
	ct := NewCostTracker()
	ct.Record(ProviderClaude, "opus-4", 0.50)

	if got := ct.TotalSpend(); got != 0.50 {
		t.Fatalf("TotalSpend = %f, want 0.50", got)
	}
	// Single entry means no span, so burn rate should be 0.
	if got := ct.BurnRate(); got != 0 {
		t.Fatalf("BurnRate = %f, want 0 (single entry)", got)
	}
}

func TestCostTracker_BurnRate(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock, advance := newTestClock(start)
	ct := NewCostTracker(withClock(clock))

	// Record $10 over 2 hours => $5/hr
	ct.Record(ProviderClaude, "opus-4", 4.0)
	advance(1 * time.Hour)
	ct.Record(ProviderGemini, "flash-2", 6.0)

	rate := ct.BurnRate()
	// Total $10 over 1 hour span = $10/hr
	if rate != 10.0 {
		t.Fatalf("BurnRate = %f, want 10.0", rate)
	}

	// Advance another hour and record more.
	advance(1 * time.Hour)
	ct.Record(ProviderOpenAI, "o3-pro", 2.0)

	// Total $12 over 2 hours = $6/hr
	rate = ct.BurnRate()
	if rate != 6.0 {
		t.Fatalf("BurnRate = %f, want 6.0", rate)
	}
}

func TestCostTracker_BudgetExhaustion(t *testing.T) {
	start := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	clock, advance := newTestClock(start)
	ct := NewCostTracker(WithBudget(100.0), withClock(clock))

	// Spend $20 in the first hour.
	ct.Record(ProviderClaude, "opus-4", 10.0)
	advance(1 * time.Hour)
	ct.Record(ProviderClaude, "opus-4", 10.0)

	// Burn rate = $20/hr, remaining = $80, so 4 hours left.
	exhaust := ct.ExhaustionTime()
	expected := start.Add(1*time.Hour).Add(4 * time.Hour) // now + 4h
	if !exhaust.Equal(expected) {
		t.Fatalf("ExhaustionTime = %v, want %v", exhaust, expected)
	}
}

func TestCostTracker_BudgetAlreadyExhausted(t *testing.T) {
	start := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	clock, advance := newTestClock(start)
	ct := NewCostTracker(WithBudget(10.0), withClock(clock))

	ct.Record(ProviderClaude, "opus-4", 5.0)
	advance(1 * time.Hour)
	ct.Record(ProviderClaude, "opus-4", 6.0) // now at $11, over budget

	exhaust := ct.ExhaustionTime()
	now := start.Add(1 * time.Hour)
	if !exhaust.Equal(now) {
		t.Fatalf("ExhaustionTime = %v, want %v (already exhausted)", exhaust, now)
	}
}

func TestCostTracker_NoBudgetNoExhaustion(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock, advance := newTestClock(start)
	ct := NewCostTracker(withClock(clock)) // no budget

	ct.Record(ProviderClaude, "opus-4", 100.0)
	advance(1 * time.Hour)
	ct.Record(ProviderClaude, "opus-4", 100.0)

	if got := ct.ExhaustionTime(); !got.IsZero() {
		t.Fatalf("ExhaustionTime = %v, want zero (no budget set)", got)
	}
}

func TestCostTracker_MultiProviderAggregation(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock, advance := newTestClock(start)
	ct := NewCostTracker(WithBudget(50.0), withClock(clock))

	ct.Record(ProviderClaude, "opus-4", 5.0)
	ct.Record(ProviderClaude, "sonnet-4", 2.0)
	advance(30 * time.Minute)
	ct.Record(ProviderGemini, "flash-2", 1.0)
	advance(30 * time.Minute)
	ct.Record(ProviderOpenAI, "o3-pro", 4.0)
	ct.Record(ProviderOpenAI, "gpt-4.1", 3.0)

	report := ct.Report()

	// Total = 5 + 2 + 1 + 4 + 3 = 15
	if report.TotalSpend != 15.0 {
		t.Fatalf("TotalSpend = %f, want 15.0", report.TotalSpend)
	}

	// Budget remaining = 50 - 15 = 35
	if report.BudgetRemaining != 35.0 {
		t.Fatalf("BudgetRemaining = %f, want 35.0", report.BudgetRemaining)
	}

	// Check per-provider totals.
	cases := []struct {
		provider Provider
		want     float64
		count    int
	}{
		{ProviderClaude, 7.0, 2},
		{ProviderGemini, 1.0, 1},
		{ProviderOpenAI, 7.0, 2},
	}
	for _, tc := range cases {
		pb, ok := report.ByProvider[tc.provider]
		if !ok {
			t.Fatalf("missing provider %s in report", tc.provider)
		}
		if pb.TotalSpend != tc.want {
			t.Errorf("%s TotalSpend = %f, want %f", tc.provider, pb.TotalSpend, tc.want)
		}
		if pb.EntryCount != tc.count {
			t.Errorf("%s EntryCount = %d, want %d", tc.provider, pb.EntryCount, tc.count)
		}
	}

	// Check instance-level breakdown.
	claudeBreakdown := report.ByProvider[ProviderClaude]
	if claudeBreakdown.ByInstance["opus-4"] != 5.0 {
		t.Errorf("claude opus-4 = %f, want 5.0", claudeBreakdown.ByInstance["opus-4"])
	}
	if claudeBreakdown.ByInstance["sonnet-4"] != 2.0 {
		t.Errorf("claude sonnet-4 = %f, want 2.0", claudeBreakdown.ByInstance["sonnet-4"])
	}
}

func TestCostTracker_ObservationWindow(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock, advance := newTestClock(start)
	ct := NewCostTracker(withClock(clock))

	ct.Record(ProviderClaude, "opus-4", 1.0)
	advance(2 * time.Hour)
	ct.Record(ProviderGemini, "flash-2", 1.0)

	report := ct.Report()
	if report.ObservationWindow != 2*time.Hour {
		t.Fatalf("ObservationWindow = %v, want 2h", report.ObservationWindow)
	}
}

func TestCostTracker_EmptyReport(t *testing.T) {
	ct := NewCostTracker(WithBudget(100.0))
	report := ct.Report()

	if report.TotalSpend != 0 {
		t.Fatalf("TotalSpend = %f, want 0", report.TotalSpend)
	}
	if report.BurnRatePerHour != 0 {
		t.Fatalf("BurnRatePerHour = %f, want 0", report.BurnRatePerHour)
	}
	if len(report.ByProvider) != 0 {
		t.Fatalf("ByProvider should be empty, got %d entries", len(report.ByProvider))
	}
	if report.ObservationWindow != 0 {
		t.Fatalf("ObservationWindow = %v, want 0", report.ObservationWindow)
	}
}

func TestCostTracker_ZeroBurnRateNoPanic(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock, _ := newTestClock(start)
	ct := NewCostTracker(WithBudget(100.0), withClock(clock))

	// Two entries at the same instant => zero span => zero burn rate.
	ct.Record(ProviderClaude, "opus-4", 5.0)
	ct.Record(ProviderClaude, "opus-4", 5.0)

	if got := ct.BurnRate(); got != 0 {
		t.Fatalf("BurnRate = %f, want 0 (zero span)", got)
	}
	// Exhaustion should be zero when burn rate is zero.
	if got := ct.ExhaustionTime(); !got.IsZero() {
		t.Fatalf("ExhaustionTime = %v, want zero (zero burn rate)", got)
	}
}

func TestCostReport_String(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock, advance := newTestClock(start)
	ct := NewCostTracker(WithBudget(50.0), withClock(clock))

	ct.Record(ProviderClaude, "opus-4", 10.0)
	advance(1 * time.Hour)
	ct.Record(ProviderGemini, "flash-2", 5.0)

	report := ct.Report()
	s := report.String()

	// Verify key information is present.
	for _, want := range []string{"Cost Report", "Total Spend", "Burn Rate", "Budget", "claude", "gemini"} {
		if !strings.Contains(s, want) {
			t.Errorf("report string missing %q", want)
		}
	}
}

func TestWithBudget_NegativeIgnored(t *testing.T) {
	ct := NewCostTracker(WithBudget(-10.0))
	if ct.budget != 0 {
		t.Fatalf("budget = %f, want 0 (negative should be ignored)", ct.budget)
	}
}
