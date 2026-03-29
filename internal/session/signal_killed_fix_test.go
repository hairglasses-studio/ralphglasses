package session

import (
	"testing"
	"time"
)

// TestSignalKilledIsTransient verifies that "signal: killed" is now recognized
// as a transient error for auto-recovery purposes.
func TestSignalKilledIsTransient(t *testing.T) {
	tests := []struct {
		errMsg string
		want   bool
	}{
		{"signal: killed", true},
		{"process killed after timeout escalation (SIGTERM→SIGKILL): signal: killed — session did not exit gracefully within the kill timeout", true},
		{"signal: killed (timeout_killed)", true},
		{"exit status 1", false},
		{"completed normally", false},
		{"timeout_killed", true},
	}
	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			got := isTransientError(tt.errMsg)
			if got != tt.want {
				t.Errorf("isTransientError(%q) = %v, want %v", tt.errMsg, got, tt.want)
			}
		})
	}
}

// TestDefaultSessionKillTimeout verifies the kill timeout was increased from 5s to 15s.
func TestDefaultSessionKillTimeout(t *testing.T) {
	if DefaultSessionKillTimeout != 15*time.Second {
		t.Errorf("DefaultSessionKillTimeout = %v, want 15s", DefaultSessionKillTimeout)
	}
}

// TestEffectiveKillTimeout_Default verifies the manager uses 15s by default.
func TestEffectiveKillTimeout_Default(t *testing.T) {
	m := NewManager()
	got := m.effectiveKillTimeout()
	if got != DefaultSessionKillTimeout {
		t.Errorf("effectiveKillTimeout() = %v, want %v", got, DefaultSessionKillTimeout)
	}
}

// TestEffectiveKillTimeout_Override verifies a custom kill timeout is respected.
func TestEffectiveKillTimeout_Override(t *testing.T) {
	m := NewManager()
	m.KillTimeout = 30 * time.Second
	got := m.effectiveKillTimeout()
	if got != 30*time.Second {
		t.Errorf("effectiveKillTimeout() = %v, want 30s", got)
	}
}

// TestForceGCAndPause verifies forceGCAndPause doesn't panic.
func TestForceGCAndPause(t *testing.T) {
	// Just ensure it runs without error.
	forceGCAndPause()
}
