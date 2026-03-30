package session

import (
	"errors"
	"testing"
)

func TestFormatKillError_Budget(t *testing.T) {
	err := errors.New("budget exceeded")
	got := FormatKillError(err, KillReasonBudget)
	if got == "" {
		t.Error("FormatKillError(budget) should return non-empty")
	}
}

func TestFormatKillError_Default(t *testing.T) {
	err := errors.New("something unexpected")
	// Use an untyped KillReason that doesn't match any case.
	got := FormatKillError(err, KillReasonUnknown)
	if got != err.Error() {
		t.Errorf("FormatKillError default = %q, want %q", got, err.Error())
	}
}
