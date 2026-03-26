package styles

import "testing"

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status string
	}{
		{"running"},
		{"completed"},
		{"failed"},
		{"errored"},
		{"stopped"},
		{"paused"},
		{"launching"},
		{"idle"},
		{"unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := StatusIcon(tt.status)
			if got == "" {
				t.Errorf("StatusIcon(%q) returned empty string", tt.status)
			}
		})
	}
}

func TestProviderIcon(t *testing.T) {
	tests := []struct {
		provider string
	}{
		{"claude"},
		{"gemini"},
		{"codex"},
		{"unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := ProviderIcon(tt.provider)
			if got == "" {
				t.Errorf("ProviderIcon(%q) returned empty string", tt.provider)
			}
		})
	}
}

func TestCBIcon(t *testing.T) {
	tests := []struct {
		state string
	}{
		{"CLOSED"},
		{"HALF_OPEN"},
		{"OPEN"},
		{"unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := CBIcon(tt.state)
			if got == "" {
				t.Errorf("CBIcon(%q) returned empty string", tt.state)
			}
		})
	}
}

func TestAlertIcon(t *testing.T) {
	tests := []struct {
		severity string
	}{
		{"critical"},
		{"warning"},
		{"info"},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			got := AlertIcon(tt.severity)
			if got == "" {
				t.Errorf("AlertIcon(%q) returned empty string", tt.severity)
			}
		})
	}
}
