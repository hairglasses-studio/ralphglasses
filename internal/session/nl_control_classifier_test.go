package session

import (
	"testing"
)

func TestParseClassifierFallback(t *testing.T) {
	c := NewNLController(nil)

	// This input uses phrasing that the rule-based detectIntent may miss
	// but the TF-IDF classifier should handle (trained on "spin up agents").
	tests := []struct {
		input      string
		wantAction string
	}{
		// Rule-based should still work for direct keywords.
		{"start 3 sessions", ActionStart},
		{"stop all sessions", ActionStop},
		{"show fleet status", ActionReport}, // "show" maps to report in rule-based
		// TF-IDF fallback should catch these (trained examples).
		{"kick off a ralph loop", ActionStart},
		{"shut down everything", ActionStop},
		{"give me analytics", ActionReport},
	}

	for _, tt := range tests {
		cmd, err := c.Parse(tt.input)
		if err != nil {
			t.Errorf("Parse(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if cmd.Action != tt.wantAction {
			t.Errorf("Parse(%q): got action %q, want %q", tt.input, cmd.Action, tt.wantAction)
		}
	}
}

func TestParseClassifierFieldInitialized(t *testing.T) {
	c := NewNLController(nil)
	if c.classifier == nil {
		t.Error("expected classifier to be initialized in NewNLController")
	}
}
