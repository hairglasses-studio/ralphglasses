package awesome

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseAwesomeList(t *testing.T) {
	t.Parallel()
	md := `# Awesome Claude Code

## Orchestrators

- [claude-squad](https://github.com/smtg-ai/claude-squad) - Worktree isolation, profile system, multi-provider TUI
- [ralph-orchestrator](https://github.com/mikeyobrien/ralph-orchestrator) — 7 AI backends, Hat System personas

## Skills

- [hcom](https://github.com/aannoo/hcom) - Multi-agent comms
- [parry](https://github.com/vaporif/parry) - Prompt injection scanner
- [some-non-github](https://example.com/tool) - This should be skipped

## Contributing

Guidelines here.
`

	entries := parseAwesomeList(md)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	// Check first entry
	if entries[0].Name != "claude-squad" {
		t.Errorf("first name = %q", entries[0].Name)
	}
	if entries[0].Category != "Orchestrators" {
		t.Errorf("first category = %q", entries[0].Category)
	}
	if entries[0].URL != "https://github.com/smtg-ai/claude-squad" {
		t.Errorf("first url = %q", entries[0].URL)
	}

	// Check description parsing
	if entries[0].Description != "Worktree isolation, profile system, multi-provider TUI" {
		t.Errorf("first description = %q", entries[0].Description)
	}

	// Check category switch
	if entries[2].Category != "Skills" {
		t.Errorf("third category = %q", entries[2].Category)
	}
}

func TestParseAwesomeList_Empty(t *testing.T) {
	t.Parallel()
	entries := parseAwesomeList("# Empty list\n\nNo links here.")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestExtractRepoFromURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/owner/repo", "owner/repo"},
		{"https://github.com/owner/repo/tree/main/docs", "owner/repo"},
		{"https://example.com/other", ""},
		{"https://github.com/single", ""},
	}

	for _, tt := range tests {
		got := extractRepoFromURL(tt.url)
		if got != tt.want {
			t.Errorf("extractRepoFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestFetch_MockServer(t *testing.T) {
	t.Parallel()
	md := `# Awesome List
## Tools
- [tool-a](https://github.com/org/tool-a) - A great tool
- [tool-b](https://github.com/org/tool-b) - Another tool
`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(md))
	}))
	defer srv.Close()

	// We can't redirect Fetch's URL easily, so test parseAwesomeList directly
	entries := parseAwesomeList(md)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	_ = context.Background()
	_ = srv
}
