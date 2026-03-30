package session

import (
	"testing"
	"time"
)

func TestAbsDuration_Positive(t *testing.T) {
	d := 5 * time.Second
	got := absDuration(d)
	if got != 5*time.Second {
		t.Errorf("absDuration(5s) = %v, want 5s", got)
	}
}

func TestAbsDuration_Negative(t *testing.T) {
	d := -3 * time.Second
	got := absDuration(d)
	if got != 3*time.Second {
		t.Errorf("absDuration(-3s) = %v, want 3s", got)
	}
}

func TestAbsDuration_Zero(t *testing.T) {
	got := absDuration(0)
	if got != 0 {
		t.Errorf("absDuration(0) = %v, want 0", got)
	}
}

func TestDiffTruncate_ShortString(t *testing.T) {
	got := diffTruncate("hello", 10)
	if got != "hello" {
		t.Errorf("diffTruncate short = %q, want hello", got)
	}
}

func TestDiffTruncate_ExactLength(t *testing.T) {
	got := diffTruncate("hello", 5)
	if got != "hello" {
		t.Errorf("diffTruncate exact = %q, want hello", got)
	}
}

func TestDiffTruncate_LongString(t *testing.T) {
	got := diffTruncate("hello world", 5)
	if got != "hello..." {
		t.Errorf("diffTruncate long = %q, want hello...", got)
	}
}

func TestDiffMax_FirstLarger(t *testing.T) {
	got := diffMax(10, 5)
	if got != 10 {
		t.Errorf("diffMax(10, 5) = %d, want 10", got)
	}
}

func TestDiffMax_SecondLarger(t *testing.T) {
	got := diffMax(3, 7)
	if got != 7 {
		t.Errorf("diffMax(3, 7) = %d, want 7", got)
	}
}

func TestDiffMax_Equal(t *testing.T) {
	got := diffMax(5, 5)
	if got != 5 {
		t.Errorf("diffMax(5, 5) = %d, want 5", got)
	}
}
