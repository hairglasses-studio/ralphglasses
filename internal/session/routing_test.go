package session

import "testing"

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		text    string
		want    bool
	}{
		{"*", "anything", true},
		{"*test*", "run the tests", true},
		{"*test*", "build project", false},
		{"fix*", "fix the bug", true},
		{"fix*", "build fix", false},
		{"*lint", "run lint", true},
		{"*lint", "linting", false},
		{"exact", "exact", true},
		{"exact", "not exact", false},
	}
	for _, tt := range tests {
		if got := matchGlob(tt.pattern, tt.text); got != tt.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.text, got, tt.want)
		}
	}
}

func TestRoutingConfig_Match(t *testing.T) {
	cfg := &RoutingConfig{
		Rules: []RoutingRule{
			{Pattern: "*test*", Provider: "gemini", Model: "gemini-2.5-flash"},
			{Pattern: "*refactor*", Provider: "claude", Model: "claude-sonnet-4-20250514"},
			{Pattern: "*", Provider: "codex", Model: "o4-mini"},
		},
	}

	r := cfg.Match("run the test suite")
	if r == nil || r.Provider != "gemini" {
		t.Errorf("expected gemini for test prompt, got %v", r)
	}

	r = cfg.Match("refactor the auth module")
	if r == nil || r.Provider != "claude" {
		t.Errorf("expected claude for refactor prompt, got %v", r)
	}

	r = cfg.Match("deploy to production")
	if r == nil || r.Provider != "codex" {
		t.Errorf("expected codex as catch-all, got %v", r)
	}

	empty := &RoutingConfig{}
	if empty.Match("anything") != nil {
		t.Error("expected nil match on empty config")
	}
}
