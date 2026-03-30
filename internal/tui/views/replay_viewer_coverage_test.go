package views

import (
	"testing"
)

func TestReplayFilter_String_AllValues(t *testing.T) {
	tests := []struct {
		filter replayFilter
		want   string
	}{
		{filterAll, "all"},
		{filterInput, "input"},
		{filterOutput, "output"},
		{filterTool, "tool"},
		{filterStatus, "status"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.filter.String()
			if got != tt.want {
				t.Errorf("replayFilter(%d).String() = %q, want %q", tt.filter, got, tt.want)
			}
		})
	}
}

func TestReplayViewerModel_SpeedIndex(t *testing.T) {
	m := NewReplayViewerModel(nil)
	got := m.SpeedIndex()
	if got != 0 {
		t.Errorf("initial SpeedIndex = %d, want 0", got)
	}
}
