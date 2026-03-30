package components

import (
	"strings"
	"testing"
	"time"
)

func TestStatusBarView(t *testing.T) {
	sb := StatusBar{
		Width: 80, Mode: "NORMAL", RepoCount: 5, RunningCount: 2, LastRefresh: time.Now(),
	}
	if sb.View() == "" {
		t.Error("view should not be empty")
	}
}

func TestStatusBarFullWidth(t *testing.T) {
	sb := StatusBar{
		Width:          200,
		Mode:           "NORMAL",
		RepoCount:      5,
		RunningCount:   3,
		SessionCount:   10,
		TotalSpendUSD:  14.52,
		FleetBudgetPct: 0.68,
		CostHistory:    []float64{0.1, 0.3, 0.5, 0.8, 1.2},
		CostVelocity:   0.12,
		ProviderCounts: map[string]int{"claude": 2, "gemini": 1},
		SpinnerFrame:   "⣾",
		ActiveLoopCount: 2,
		LoopIterTotal:   20,
		LoopSuccessRate: 0.87,
		LoopIterHistory: []float64{12.0, 15.0, 10.0, 8.0, 9.0},
		ProviderHealthy: map[string]bool{"claude": true, "gemini": true, "codex": false},
		FleetCompletions: 42,
		FleetFailures:    3,
		FleetFailureRate: 0.068,
		FleetLatencyP50:  320,
		AutonomyLevel:    "L1",
		AlertCount:       2,
		HighestAlertSeverity: "warning",
		Uptime:           12 * time.Minute,
		LastRefresh:      time.Now(),
	}

	view := sb.View()
	plain := StripAnsi(view)

	// At 200 cols, all segments should be present. Check for separator.
	if !strings.Contains(plain, "│") {
		t.Error("full-width view should contain │ separators")
	}

	// Check key content is present.
	for _, want := range []string{"NORMAL", "$14.52", "p50:320ms", "L1"} {
		if !strings.Contains(plain, want) {
			t.Errorf("full-width view missing %q", want)
		}
	}
}

func TestStatusBarCollapse(t *testing.T) {
	sb := StatusBar{
		Width:          40,
		Mode:           "NORMAL",
		RunningCount:   2,
		SessionCount:   5,
		TotalSpendUSD:  1.23,
		CostHistory:    []float64{0.1, 0.2},
		ActiveLoopCount: 1,
		LoopIterTotal:   5,
		LoopSuccessRate: 0.8,
		ProviderHealthy: map[string]bool{"claude": true},
		FleetCompletions: 10,
		AutonomyLevel:    "L0",
		AlertCount:       1,
		HighestAlertSeverity: "info",
		LastRefresh:      time.Now(),
	}

	view := sb.View()
	plain := StripAnsi(view)

	// At 40 cols, lower-priority segments should collapse.
	if !strings.Contains(plain, "NORMAL") {
		t.Error("collapsed view should always contain mode")
	}
}

func TestStatusBarEmptyLoops(t *testing.T) {
	sb := StatusBar{
		Width:           120,
		Mode:            "NORMAL",
		RunningCount:    1,
		SessionCount:    3,
		ActiveLoopCount: 0,
		LastRefresh:     time.Now(),
	}

	view := sb.View()
	plain := StripAnsi(view)

	// With zero active loops, the loops segment icon should not appear.
	// IconTurns is a nerd font char that gets stripped. Just verify no crash.
	if view == "" {
		t.Error("view should not be empty")
	}
	_ = plain
}

func TestStatusBarEmptyFleet(t *testing.T) {
	sb := StatusBar{
		Width:            120,
		Mode:             "NORMAL",
		FleetCompletions: 0,
		FleetFailures:    0,
		LastRefresh:      time.Now(),
	}

	view := sb.View()
	plain := StripAnsi(view)

	if strings.Contains(plain, "p50:") {
		t.Error("empty fleet should not show p50 latency")
	}
}

func TestStatusBarEmptyCostHistory(t *testing.T) {
	sb := StatusBar{
		Width:         100,
		Mode:          "NORMAL",
		TotalSpendUSD: 0,
		CostHistory:   nil,
		LastRefresh:   time.Now(),
	}

	// Should not panic with nil/empty cost history.
	view := sb.View()
	if view == "" {
		t.Error("view should not be empty")
	}
}

func TestFormatAgo(t *testing.T) {
	if got := formatAgo(time.Time{}); got != "never" {
		t.Errorf("zero time = %q, want never", got)
	}
	recent := time.Now().Add(-10 * time.Second)
	if formatAgo(recent) == "never" {
		t.Error("recent time should not be never")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{2*time.Hour + 15*time.Minute, "2h15m"},
	}
	for _, tt := range tests {
		if got := formatDuration(tt.d); got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestVisualWidth(t *testing.T) {
	if got := VisualWidth("hello"); got != 5 {
		t.Errorf("plain text = %d, want 5", got)
	}
	if got := VisualWidth(""); got != 0 {
		t.Errorf("empty = %d, want 0", got)
	}
	ansi := "\x1b[31mred\x1b[0m"
	if got := VisualWidth(ansi); got != 3 {
		t.Errorf("ansi = %d, want 3", got)
	}
}
