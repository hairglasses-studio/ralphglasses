package timecontext

import (
	"testing"
	"time"
)

func makeTime(hour int) time.Time {
	return time.Date(2026, 4, 5, hour, 30, 0, 0, time.UTC)
}

func TestBlockAt_Morning(t *testing.T) {
	for _, h := range []int{6, 7, 8} {
		if b := BlockAt(makeTime(h)); b != Morning {
			t.Errorf("hour %d: got %v, want Morning", h, b)
		}
	}
}

func TestBlockAt_Work(t *testing.T) {
	for _, h := range []int{9, 12, 15, 17} {
		if b := BlockAt(makeTime(h)); b != Work {
			t.Errorf("hour %d: got %v, want Work", h, b)
		}
	}
}

func TestBlockAt_Evening(t *testing.T) {
	for _, h := range []int{18, 19, 20, 21} {
		if b := BlockAt(makeTime(h)); b != Evening {
			t.Errorf("hour %d: got %v, want Evening", h, b)
		}
	}
}

func TestBlockAt_Night(t *testing.T) {
	for _, h := range []int{22, 23, 0, 1, 5} {
		if b := BlockAt(makeTime(h)); b != Night {
			t.Errorf("hour %d: got %v, want Night", h, b)
		}
	}
}

func TestBlockAt_Boundaries(t *testing.T) {
	tests := []struct {
		hour int
		want Block
	}{
		{5, Night},   // last hour of night
		{6, Morning}, // first hour of morning
		{8, Morning}, // last hour of morning
		{9, Work},    // first hour of work
		{17, Work},   // last hour of work
		{18, Evening}, // first hour of evening
		{21, Evening}, // last hour of evening
		{22, Night},   // first hour of night
	}
	for _, tt := range tests {
		if got := BlockAt(makeTime(tt.hour)); got != tt.want {
			t.Errorf("boundary hour %d: got %v, want %v", tt.hour, got, tt.want)
		}
	}
}

func TestLabel(t *testing.T) {
	tests := []struct {
		block Block
		want  string
	}{
		{Morning, "Morning (6-9 AM)"},
		{Work, "Work Hours (9 AM-6 PM)"},
		{Evening, "Evening (6-10 PM)"},
		{Night, "Night (10 PM-6 AM)"},
	}
	for _, tt := range tests {
		if got := tt.block.Label(); got != tt.want {
			t.Errorf("%v.Label() = %q, want %q", tt.block, got, tt.want)
		}
	}
}

func TestPriorities(t *testing.T) {
	blocks := []Block{Morning, Work, Evening, Night}
	for _, b := range blocks {
		p := b.Priorities()
		if len(p) == 0 {
			t.Errorf("%v.Priorities() returned empty", b)
		}
	}

	// Spot check specific priorities
	mp := Morning.Priorities()
	found := false
	for _, p := range mp {
		if p == "briefing" {
			found = true
		}
	}
	if !found {
		t.Error("Morning priorities should include 'briefing'")
	}
}

func TestIsWeekend(t *testing.T) {
	// 2026-04-04 is a Saturday, 2026-04-05 is a Sunday, 2026-04-06 is a Monday
	sat := time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC)
	sun := time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)
	mon := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)

	if !IsWeekend(sat) {
		t.Error("Saturday should be weekend")
	}
	if !IsWeekend(sun) {
		t.Error("Sunday should be weekend")
	}
	if IsWeekend(mon) {
		t.Error("Monday should not be weekend")
	}
}
