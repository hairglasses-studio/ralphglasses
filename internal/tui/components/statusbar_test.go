package components

import (
	"testing"
	"time"
)

func TestStatusBarView(t *testing.T) {
	sb := StatusBar{
		Width: 80, Mode: "NORMAL", RepoCount: 5, RunningCount: 2, LastRefresh: time.Now(),
	}
	if sb.View() == "" {
		t.Error("view should not be empty")
	}
}

func TestFormatAgo(t *testing.T) {
	if got := formatAgo(time.Time{}); got != "never" {
		t.Errorf("zero time = %q, want never", got)
	}
	recent := time.Now().Add(-10 * time.Second)
	if formatAgo(recent) == "never" {
		t.Error("recent time should not be never")
	}
}

func TestVisualWidth(t *testing.T) {
	if got := VisualWidth("hello"); got != 5 {
		t.Errorf("plain text = %d, want 5", got)
	}
	if got := VisualWidth(""); got != 0 {
		t.Errorf("empty = %d, want 0", got)
	}
	ansi := "\x1b[31mred\x1b[0m"
	if got := VisualWidth(ansi); got != 3 {
		t.Errorf("ansi = %d, want 3", got)
	}
}
