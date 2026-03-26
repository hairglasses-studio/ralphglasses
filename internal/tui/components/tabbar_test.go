package components

import (
	"strings"
	"testing"
)

func TestTabBarView(t *testing.T) {
	tests := []struct {
		name        string
		tabs        []string
		active      int
		wantEmpty   bool
		wantContain []string // substrings expected in output
	}{
		{
			name:      "zero tabs",
			tabs:      nil,
			active:    0,
			wantEmpty: true,
		},
		{
			name:        "one tab active",
			tabs:        []string{"Sessions"},
			active:      0,
			wantContain: []string{"Sessions"},
		},
		{
			name:        "three tabs first active",
			tabs:        []string{"Sessions", "Fleet", "Logs"},
			active:      0,
			wantContain: []string{"Sessions", "Fleet", "Logs"},
		},
		{
			name:        "three tabs middle active",
			tabs:        []string{"Sessions", "Fleet", "Logs"},
			active:      1,
			wantContain: []string{"Sessions", "Fleet", "Logs"},
		},
		{
			name:        "three tabs last active",
			tabs:        []string{"Sessions", "Fleet", "Logs"},
			active:      2,
			wantContain: []string{"Sessions", "Fleet", "Logs"},
		},
		{
			name:        "active index out of range",
			tabs:        []string{"A", "B"},
			active:      5,
			wantContain: []string{"A", "B"},
		},
		{
			name:        "negative active index",
			tabs:        []string{"A", "B"},
			active:      -1,
			wantContain: []string{"A", "B"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := &TabBar{
				Tabs:   tt.tabs,
				Active: tt.active,
			}
			got := tb.View()
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty string, got %q", got)
				}
				return
			}
			for _, s := range tt.wantContain {
				if !strings.Contains(got, s) {
					t.Errorf("output %q should contain %q", got, s)
				}
			}
		})
	}
}

func TestTabBarSeparation(t *testing.T) {
	tb := &TabBar{
		Tabs:   []string{"A", "B", "C"},
		Active: 1,
	}
	got := tb.View()
	// The tabs should be separated by spaces (from strings.Join)
	// Each tab text should appear exactly once
	for _, name := range tb.Tabs {
		count := strings.Count(got, name)
		if count != 1 {
			t.Errorf("tab %q appears %d times, want 1", name, count)
		}
	}
}
