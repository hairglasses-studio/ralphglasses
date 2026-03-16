package views

import (
	"strings"
	"testing"
)

func TestRenderHelp(t *testing.T) {
	output := RenderHelp(120, 40)
	sections := []string{"Global", "Overview Table", "Repo Detail", "Log Viewer", "Commands"}
	for _, sec := range sections {
		if !strings.Contains(output, sec) {
			t.Errorf("help output missing section %q", sec)
		}
	}
}

func TestRenderHelpNonEmpty(t *testing.T) {
	output := RenderHelp(0, 0)
	if output == "" {
		t.Error("help should render even with zero dimensions")
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"hi", 5, "hi   "},
		{"hello", 5, "hello"},
		{"longer", 3, "longer"},
		{"", 3, "   "},
	}
	for _, tt := range tests {
		got := padRight(tt.input, tt.n)
		if got != tt.want {
			t.Errorf("padRight(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
		}
	}
}
