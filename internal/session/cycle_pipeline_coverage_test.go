package session

import (
	"testing"
)

func TestCycleTruncate_NoTruncation(t *testing.T) {
	got := cycleTruncate("hello", 10)
	if got != "hello" {
		t.Errorf("cycleTruncate(%q, 10) = %q, want %q", "hello", got, "hello")
	}
}

func TestCycleTruncate_ExactLength(t *testing.T) {
	got := cycleTruncate("hello", 5)
	if got != "hello" {
		t.Errorf("cycleTruncate(%q, 5) = %q, want %q", "hello", got, "hello")
	}
}

func TestCycleTruncate_WithEllipsis(t *testing.T) {
	got := cycleTruncate("hello world", 8)
	if got != "hello..." {
		t.Errorf("cycleTruncate(%q, 8) = %q, want %q", "hello world", got, "hello...")
	}
}

func TestCycleTruncate_ShortMaxLen(t *testing.T) {
	// maxLen < 4, no ellipsis
	got := cycleTruncate("hello world", 3)
	if got != "hel" {
		t.Errorf("cycleTruncate(%q, 3) = %q, want %q", "hello world", got, "hel")
	}
}

func TestClampPriority_InRange(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{0.5, 0.5},
		{0.0, 0.0},
		{1.0, 1.0},
		{0.75, 0.75},
	}
	for _, tt := range tests {
		got := clampPriority(tt.input)
		if got != tt.want {
			t.Errorf("clampPriority(%f) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestClampPriority_BelowZero(t *testing.T) {
	got := clampPriority(-0.5)
	if got != 0 {
		t.Errorf("clampPriority(-0.5) = %f, want 0", got)
	}
}

func TestClampPriority_AboveOne(t *testing.T) {
	got := clampPriority(1.5)
	if got != 1 {
		t.Errorf("clampPriority(1.5) = %f, want 1", got)
	}
}
