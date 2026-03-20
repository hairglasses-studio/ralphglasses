package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
)

func testHelpGroups() []HelpGroup {
	return []HelpGroup{
		{
			Name: "Navigation",
			Bindings: []key.Binding{
				key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "repos tab")),
				key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "sessions tab")),
			},
		},
		{
			Name: "Global",
			Bindings: []key.Binding{
				key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
				key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
			},
		},
	}
}

func TestRenderHelp(t *testing.T) {
	groups := testHelpGroups()
	output := RenderHelp(groups, 120, 40)
	sections := []string{"Navigation", "Global", "Commands"}
	for _, sec := range sections {
		if !strings.Contains(output, sec) {
			t.Errorf("help output missing section %q", sec)
		}
	}
}

func TestRenderHelpNonEmpty(t *testing.T) {
	output := RenderHelp(nil, 0, 0)
	if output == "" {
		t.Error("help should render even with zero dimensions")
	}
}

func TestRenderHelpShowsBindings(t *testing.T) {
	groups := testHelpGroups()
	output := RenderHelp(groups, 120, 40)
	if !strings.Contains(output, "repos tab") {
		t.Error("help should show binding descriptions")
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
