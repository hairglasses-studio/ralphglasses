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

func TestParseAwesomeList_SkipNonContentHeaders(t *testing.T) {
	t.Parallel()
	md := `# Awesome List

## Contents

- [tool](https://github.com/org/tool) - A tool

## Table of Contents

- [tool2](https://github.com/org/tool2) - Another tool

## License

- [tool3](https://github.com/org/tool3) - Licensed tool

## Real Category

- [tool4](https://github.com/org/tool4) - Real entry
`

	entries := parseAwesomeList(md)
	// tool, tool2, tool3 should have empty category since Contents/TOC/License are skipped
	// tool4 should be in "Real Category"
	for _, e := range entries {
		if e.Name == "tool4" && e.Category != "Real Category" {
			t.Errorf("tool4 category = %q, want Real Category", e.Category)
		}
	}
}

func TestParseAwesomeList_TriplehashHeaders(t *testing.T) {
	t.Parallel()
	md := `# Awesome List

### Sub Category

- [tool](https://github.com/org/tool) - A tool
`

	entries := parseAwesomeList(md)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Category != "Sub Category" {
		t.Errorf("category = %q, want Sub Category", entries[0].Category)
	}
}

func TestParseAwesomeList_AsteriskBullets(t *testing.T) {
	t.Parallel()
	md := `## Tools

* [tool](https://github.com/org/tool) - A tool with asterisk
`

	entries := parseAwesomeList(md)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "tool" {
		t.Errorf("name = %q", entries[0].Name)
	}
}

func TestParseAwesomeList_NoDescription(t *testing.T) {
	t.Parallel()
	md := `## Tools

- [tool](https://github.com/org/tool)
`

	entries := parseAwesomeList(md)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Description != "" {
		t.Errorf("description = %q, want empty", entries[0].Description)
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
		{"https://github.com/owner/repo/", "owner/repo"},
		{"http://github.com/owner/repo", "owner/repo"},
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
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(md))
	}))
	defer srv.Close()

	// Override the fetchREADME by using the test server directly
	// Test the full Fetch path via a custom transport
	client := srv.Client()
	transport := &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}
	client.Transport = transport

	idx, err := Fetch(context.Background(), client, "test/repo")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(idx.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(idx.Entries))
	}
	if idx.Source != "test/repo" {
		t.Errorf("source = %q, want test/repo", idx.Source)
	}
	if idx.Entries[0].Name != "tool-a" {
		t.Errorf("first entry name = %q", idx.Entries[0].Name)
	}
}

func TestFetch_DefaultRepo(t *testing.T) {
	t.Parallel()
	md := `# Awesome List
## Tools
- [tool](https://github.com/org/tool) - A tool
`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(md))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	idx, err := Fetch(context.Background(), client, "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if idx.Source != DefaultSource {
		t.Errorf("source = %q, want %q", idx.Source, DefaultSource)
	}
}

func TestFetch_NilClient(t *testing.T) {
	t.Parallel()
	// With nil client and a mock server, we can't easily test the full path
	// but we can verify it doesn't panic and tries to make a request
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so the request fails fast

	_, err := Fetch(ctx, nil, "nonexistent/repo")
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}

func TestFetch_HTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	_, err := Fetch(context.Background(), client, "bad/repo")
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestFetchRepoREADME_ValidRepo(t *testing.T) {
	t.Parallel()
	content := "# My Repo\n\nSome readme content"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	readme, err := FetchRepoREADME(context.Background(), client, "https://github.com/owner/repo")
	if err != nil {
		t.Fatalf("FetchRepoREADME: %v", err)
	}
	if readme != content {
		t.Errorf("readme = %q, want %q", readme, content)
	}
}

func TestFetchRepoREADME_InvalidURL(t *testing.T) {
	t.Parallel()
	_, err := FetchRepoREADME(context.Background(), nil, "https://example.com/not-github")
	if err == nil {
		t.Error("expected error for non-GitHub URL")
	}
}

// rewriteTransport rewrites all requests to point to a test server.
type rewriteTransport struct {
	base    http.RoundTripper
	hostMap string // test server URL to redirect to
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite the URL to point to our test server
	newURL := t.hostMap + req.URL.Path
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header

	if t.base != nil {
		return t.base.RoundTrip(newReq)
	}
	return http.DefaultTransport.RoundTrip(newReq)
}
