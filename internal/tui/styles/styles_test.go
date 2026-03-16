package styles

import "testing"

func TestStatusStyle(t *testing.T) {
	tests := []string{"running", "completed", "failed", "idle", "stopped", "unknown", ""}
	for _, s := range tests {
		style := StatusStyle(s)
		rendered := style.Render(s)
		if s != "" && rendered == "" {
			t.Errorf("StatusStyle(%q).Render returned empty", s)
		}
	}
}

func TestCBStyle(t *testing.T) {
	tests := []string{"CLOSED", "HALF_OPEN", "OPEN", "unknown", ""}
	for _, s := range tests {
		style := CBStyle(s)
		rendered := style.Render(s)
		if s != "" && rendered == "" {
			t.Errorf("CBStyle(%q).Render returned empty", s)
		}
	}
}
