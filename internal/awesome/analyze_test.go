package awesome

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRateValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		stars   int
		lang    string
		matches int
		want    Rating
	}{
		{"high: 3+ matches, Go", 200, "Go", 5, RatingHigh},
		{"high: 3+ matches, >100 stars", 200, "Rust", 4, RatingHigh},
		{"high: 3+ matches, Go, few stars", 10, "Go", 3, RatingHigh},
		{"medium: 1-2 matches", 50, "Python", 2, RatingMedium},
		{"medium: 1 match", 50, "Python", 1, RatingMedium},
		{"medium: >500 stars, Go", 600, "Go", 0, RatingMedium},
		{"medium: non-Go with 1 match", 10, "JavaScript", 1, RatingMedium},
		{"low: Go, 0 matches", 30, "Go", 0, RatingLow},
		{"none: 0 matches, non-Go, few stars", 5, "JavaScript", 0, RatingNone},
		{"none: 0 matches, no lang, few stars", 0, "", 0, RatingNone},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := rateValue(tc.stars, tc.lang, tc.matches)
			if got != tc.want {
				t.Errorf("rateValue(%d, %q, %d) = %q, want %q", tc.stars, tc.lang, tc.matches, got, tc.want)
			}
		})
	}
}

func TestRateComplexity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		lang   string
		readme string
		want   string
	}{
		{"Go MCP server", "Go", "this is an mcp server with hooks", "drop-in"},
		{"Go MCP tool", "Go", "mcp tool for claude", "drop-in"},
		{"Go hook", "Go", "a hook system", "drop-in"},
		{"Go skill", "Go", "a skill framework", "drop-in"},
		{"Go plugin", "Go", "a plugin system", "drop-in"},
		{"npm cli tool", "JavaScript", "npm install -g my-cli cli tool", "drop-in"},
		{"Rust no signals", "Rust", "a complex system", "moderate"},
		{"Go no signals", "Go", "a standard library", "moderate"},
		{"Python no signals", "Python", "machine learning framework", "moderate"},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := rateComplexity(tc.lang, tc.readme)
			if got != tc.want {
				t.Errorf("rateComplexity(%q, %q) = %q, want %q", tc.lang, tc.readme, got, tc.want)
			}
		})
	}
}

func TestBuildRationale(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ae   AnalysisEntry
		want string
	}{
		{
			name: "all fields",
			ae: AnalysisEntry{
				Stars:             100,
				Language:          "Go",
				CapabilityMatches: 3,
			},
			want: "3 capability matches, 100 stars, Go",
		},
		{
			name: "no matches",
			ae: AnalysisEntry{
				Stars:    50,
				Language: "Rust",
			},
			want: "50 stars, Rust",
		},
		{
			name: "only matches",
			ae: AnalysisEntry{
				CapabilityMatches: 2,
			},
			want: "2 capability matches",
		},
		{
			name: "empty",
			ae:   AnalysisEntry{},
			want: "insufficient data",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildRationale(tc.ae)
			if got != tc.want {
				t.Errorf("buildRationale() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAnalyze_WithMockServer(t *testing.T) {
	t.Parallel()

	// Mock server that serves both repo metadata and READMEs
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if strings.HasPrefix(path, "/repos/") {
			// GitHub API repo metadata
			meta := repoMeta{Stars: 250, Language: "Go"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(meta)
			return
		}

		// README content
		readme := "# Tool\n\nAn mcp server with tui and agent orchestration and workflow support"
		_, _ = w.Write([]byte(readme))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	entries := []AwesomeEntry{
		{Name: "tool-a", URL: "https://github.com/org/tool-a", Description: "An MCP tool"},
		{Name: "tool-b", URL: "https://github.com/org/tool-b", Description: "A basic util"},
	}

	analysis, err := Analyze(context.Background(), client, entries, AnalyzeOptions{
		MaxWorkers: 2,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if analysis.Summary.Total != 2 {
		t.Errorf("total = %d, want 2", analysis.Summary.Total)
	}
	if len(analysis.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(analysis.Entries))
	}

	// Both should have stars from mock
	for _, e := range analysis.Entries {
		if e.Stars != 250 {
			t.Errorf("entry %q stars = %d, want 250", e.Name, e.Stars)
		}
		if e.Language != "Go" {
			t.Errorf("entry %q language = %q, want Go", e.Name, e.Language)
		}
		if e.Rating == "" {
			t.Errorf("entry %q has empty rating", e.Name)
		}
	}
}

func TestAnalyze_NilClient(t *testing.T) {
	t.Parallel()
	// With nil client, should create a default one; use cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	entries := []AwesomeEntry{
		{Name: "tool", URL: "https://github.com/org/tool", Description: "A tool"},
	}

	// Should not panic even with cancelled context
	analysis, err := Analyze(ctx, nil, entries, AnalyzeOptions{})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	// Entry should still exist, just with degraded analysis
	if len(analysis.Entries) != 1 {
		t.Errorf("entries = %d, want 1", len(analysis.Entries))
	}
}

func TestAnalyze_EmptyEntries(t *testing.T) {
	t.Parallel()
	analysis, err := Analyze(context.Background(), nil, nil, AnalyzeOptions{})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if analysis.Summary.Total != 0 {
		t.Errorf("total = %d, want 0", analysis.Summary.Total)
	}
}

func TestAnalyze_DefaultCapabilities(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/repos/") {
			meta := repoMeta{Stars: 100, Language: "Go"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(meta)
			return
		}
		_, _ = w.Write([]byte("# Tool\n\nThis is an mcp agent with tui and claude support"))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	entries := []AwesomeEntry{
		{Name: "tool", URL: "https://github.com/org/tool", Description: "mcp agent"},
	}

	// nil Capabilities should use DefaultCapabilities
	analysis, err := Analyze(context.Background(), client, entries, AnalyzeOptions{
		Capabilities: nil,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if analysis.Entries[0].CapabilityMatches == 0 {
		t.Error("expected capability matches with default capabilities")
	}
}

func TestAnalyze_CustomCapabilities(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/repos/") {
			meta := repoMeta{Stars: 50, Language: "Python"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(meta)
			return
		}
		_, _ = w.Write([]byte("# Tool\n\nThis has unicorn and rainbow features"))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	entries := []AwesomeEntry{
		{Name: "tool", URL: "https://github.com/org/tool", Description: "rainbow tool"},
	}

	analysis, err := Analyze(context.Background(), client, entries, AnalyzeOptions{
		Capabilities: []string{"unicorn", "rainbow"},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if analysis.Entries[0].CapabilityMatches != 2 {
		t.Errorf("matches = %d, want 2", analysis.Entries[0].CapabilityMatches)
	}
}

func TestAnalyzeOne_BadURL(t *testing.T) {
	t.Parallel()
	entry := AwesomeEntry{Name: "bad", URL: "https://example.com/not-github", Description: "bad url"}
	ae := analyzeOne(context.Background(), http.DefaultClient, entry, DefaultCapabilities)
	if ae.Rating != RatingNone {
		t.Errorf("rating = %q, want NONE for bad URL", ae.Rating)
	}
	if ae.Rationale != "cannot extract repo from URL" {
		t.Errorf("rationale = %q", ae.Rationale)
	}
}

func TestAnalyzeOne_MetaFailFallback(t *testing.T) {
	t.Parallel()

	// Server returns 500 for API calls, but valid README
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/repos/") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("error"))
			return
		}
		_, _ = w.Write([]byte("# Tool\n\nAn mcp agent with tui"))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	entry := AwesomeEntry{Name: "tool", URL: "https://github.com/org/tool", Description: "mcp agent"}
	ae := analyzeOne(context.Background(), client, entry, DefaultCapabilities)

	// Should still have matches from README even without metadata
	if ae.Stars != 0 {
		t.Errorf("stars = %d, want 0 (meta failed)", ae.Stars)
	}
	if ae.CapabilityMatches == 0 {
		t.Error("expected capability matches from README despite meta failure")
	}
}

func TestAnalyzeOne_ReadmeFailFallback(t *testing.T) {
	t.Parallel()

	// Server returns 500 for everything
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error"))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	entry := AwesomeEntry{Name: "tool", URL: "https://github.com/org/tool", Description: "mcp agent"}
	ae := analyzeOne(context.Background(), client, entry, DefaultCapabilities)

	// Should fall back to description matching
	if ae.CapabilityMatches == 0 {
		t.Error("expected capability matches from description fallback")
	}
}

func TestAnalyze_SummaryAccuracy(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error"))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	entries := []AwesomeEntry{
		{Name: "t1", URL: "https://github.com/o/t1", Description: "mcp tui agent claude workflow"},
		{Name: "t2", URL: "https://github.com/o/t2", Description: "basic tool"},
		{Name: "t3", URL: "https://example.com/bad", Description: "bad url"},
	}

	analysis, err := Analyze(context.Background(), client, entries, AnalyzeOptions{MaxWorkers: 1})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// Verify summary counts match actual entries
	counts := map[Rating]int{}
	for _, e := range analysis.Entries {
		counts[e.Rating]++
	}
	if counts[RatingHigh] != analysis.Summary.High {
		t.Errorf("high mismatch: counted %d, summary %d", counts[RatingHigh], analysis.Summary.High)
	}
	if counts[RatingMedium] != analysis.Summary.Medium {
		t.Errorf("medium mismatch: counted %d, summary %d", counts[RatingMedium], analysis.Summary.Medium)
	}
	if counts[RatingLow] != analysis.Summary.Low {
		t.Errorf("low mismatch: counted %d, summary %d", counts[RatingLow], analysis.Summary.Low)
	}
	if counts[RatingNone] != analysis.Summary.None {
		t.Errorf("none mismatch: counted %d, summary %d", counts[RatingNone], analysis.Summary.None)
	}
	if analysis.Summary.Total != len(entries) {
		t.Errorf("total = %d, want %d", analysis.Summary.Total, len(entries))
	}
}

func TestFetchRepoMeta_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		meta := repoMeta{Stars: 42, Language: "Go"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(meta)
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	meta, err := fetchRepoMeta(context.Background(), client, "org/repo")
	if err != nil {
		t.Fatalf("fetchRepoMeta: %v", err)
	}
	if meta.Stars != 42 {
		t.Errorf("stars = %d, want 42", meta.Stars)
	}
	if meta.Language != "Go" {
		t.Errorf("language = %q, want Go", meta.Language)
	}
}

func TestFetchRepoMeta_HTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("rate limited"))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	_, err := fetchRepoMeta(context.Background(), client, "org/repo")
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func TestFetchRepoMeta_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	_, err := fetchRepoMeta(context.Background(), client, "org/repo")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
