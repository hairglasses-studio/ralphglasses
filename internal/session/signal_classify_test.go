package session

import (
	"errors"
	"strings"
	"testing"
)

func TestClassifyExitSignal_UserStopped(t *testing.T) {
	err := errors.New("signal: killed")
	reason := ClassifyExitSignal(err, 12345, true)
	if reason != KillReasonUser {
		t.Errorf("expected KillReasonUser, got %v", reason)
	}
}

func TestClassifyExitSignal_NilError(t *testing.T) {
	reason := ClassifyExitSignal(nil, 12345, false)
	if reason != KillReasonUnknown {
		t.Errorf("expected KillReasonUnknown, got %v", reason)
	}
}

func TestClassifyExitSignal_NonKillSignal(t *testing.T) {
	err := errors.New("exit status 1")
	reason := ClassifyExitSignal(err, 12345, false)
	if reason != KillReasonUnknown {
		t.Errorf("expected KillReasonUnknown for non-kill signal, got %v", reason)
	}
}

func TestClassifyExitSignal_KillSignalTimeout(t *testing.T) {
	// On non-Linux or when memory is not under pressure, should return Timeout.
	err := errors.New("signal: killed")
	reason := ClassifyExitSignal(err, 99999, false)
	// Could be Timeout or OOM depending on system state, but should not be User or Unknown.
	if reason == KillReasonUser {
		t.Errorf("should not be KillReasonUser when wasStopped=false")
	}
	if reason == KillReasonUnknown {
		t.Errorf("should not be KillReasonUnknown for signal: killed")
	}
}

func TestKillReason_String(t *testing.T) {
	tests := []struct {
		reason KillReason
		want   string
	}{
		{KillReasonUnknown, "unknown_killed"},
		{KillReasonOOM, "oom_killed"},
		{KillReasonTimeout, "timeout_killed"},
		{KillReasonUser, "user_stopped"},
		{KillReasonBudget, "budget_killed"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.reason.String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatKillError_OOM(t *testing.T) {
	err := errors.New("signal: killed")
	msg := FormatKillError(err, KillReasonOOM)
	if msg == "" {
		t.Fatal("expected non-empty message")
	}
	if !strings.Contains(msg, "out of memory") {
		t.Errorf("OOM message should mention 'out of memory', got: %s", msg)
	}
}

func TestFormatKillError_Timeout(t *testing.T) {
	err := errors.New("signal: killed")
	msg := FormatKillError(err, KillReasonTimeout)
	if !strings.Contains(msg, "timeout escalation") {
		t.Errorf("timeout message should mention 'timeout escalation', got: %s", msg)
	}
}

func TestFormatKillError_User(t *testing.T) {
	err := errors.New("signal: killed")
	msg := FormatKillError(err, KillReasonUser)
	if msg != "stopped by user" {
		t.Errorf("expected 'stopped by user', got: %s", msg)
	}
}

func TestFormatKillError_Nil(t *testing.T) {
	msg := FormatKillError(nil, KillReasonTimeout)
	if msg != "" {
		t.Errorf("expected empty for nil error, got: %s", msg)
	}
}

func TestIsKillSignalError(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"signal: killed", true},
		{"exit status 1", false},
		{"process killed after timeout escalation (SIGTERM→SIGKILL): signal: killed", true},
		{"", false},
		{"signal: terminated", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := IsKillSignalError(tt.input); got != tt.want {
				t.Errorf("IsKillSignalError(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestWrapExitReasonWithContext(t *testing.T) {
	// Non-kill signal should pass through unchanged.
	s := &Session{Pid: 12345}
	got := WrapExitReasonWithContext("exit status 1", s)
	if got != "exit status 1" {
		t.Errorf("expected pass-through for non-kill, got: %s", got)
	}

	// Kill signal should be wrapped with reason.
	got = WrapExitReasonWithContext("signal: killed", s)
	if got == "signal: killed" {
		t.Errorf("expected wrapped reason, got raw: %s", got)
	}
	if !strings.Contains(got, "signal: killed") {
		t.Errorf("wrapped reason should contain original, got: %s", got)
	}
}

func TestIsSystemMemoryPressure(t *testing.T) {
	// This is a live check — just ensure it doesn't panic.
	_ = isSystemMemoryPressure()
}
