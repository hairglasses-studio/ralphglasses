package awesome

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSync_FullPipeline(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if strings.HasPrefix(path, "/repos/") {
			meta := repoMeta{Stars: 100, Language: "Go"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(meta)
			return
		}

		// Serve awesome list README
		if strings.Contains(path, "test/awesome-list") {
			md := `# Awesome List
## Tools
- [tool-a](https://github.com/org/tool-a) - An mcp agent tool
- [tool-b](https://github.com/org/tool-b) - A tui framework
`
			_, _ = w.Write([]byte(md))
			return
		}

		// Serve repo READMEs
		_, _ = w.Write([]byte("# Repo\n\nAn mcp server with tui and agent support"))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	dir := t.TempDir()

	result, err := Sync(context.Background(), client, SyncOptions{
		Repo:       "test/awesome-list",
		SaveTo:     dir,
		FullRescan: true,
		MaxWorkers: 2,
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if result.Source != "test/awesome-list" {
		t.Errorf("source = %q", result.Source)
	}
	if result.Fetched != 2 {
		t.Errorf("fetched = %d, want 2", result.Fetched)
	}
	if result.Analyzed != 2 {
		t.Errorf("analyzed = %d, want 2", result.Analyzed)
	}
	if result.SavedTo == "" {
		t.Error("expected SavedTo to be set")
	}
	if result.ReportPath == "" {
		t.Error("expected ReportPath to be set")
	}

	// Verify files were saved
	if _, err := LoadIndex(dir); err != nil {
		t.Errorf("LoadIndex after sync: %v", err)
	}
	if _, err := LoadAnalysis(dir); err != nil {
		t.Errorf("LoadAnalysis after sync: %v", err)
	}

	reportPath := filepath.Join(StorePath(dir), "report.md")
	if _, err := os.Stat(reportPath); err != nil {
		t.Errorf("report.md not found: %v", err)
	}
}

func TestSync_NilClient(t *testing.T) {
	t.Parallel()
	// With nil client and cancelled context, should fail on fetch
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Sync(ctx, nil, SyncOptions{Repo: "nonexistent/repo"})
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}

func TestSync_DefaultRepo(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		md := `# Awesome List
## Tools
- [tool](https://github.com/org/tool) - A tool
`
		_, _ = w.Write([]byte(md))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	// Empty repo should use DefaultSource
	result, err := Sync(context.Background(), client, SyncOptions{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Source != DefaultSource {
		t.Errorf("source = %q, want %q", result.Source, DefaultSource)
	}
}

func TestSync_NoSaveTo(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/repos/") {
			meta := repoMeta{Stars: 50, Language: "Go"}
			json.NewEncoder(w).Encode(meta)
			return
		}
		md := `# Awesome List
## Tools
- [tool](https://github.com/org/tool) - mcp tool
`
		_, _ = w.Write([]byte(md))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	result, err := Sync(context.Background(), client, SyncOptions{Repo: "test/repo"})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if result.SavedTo != "" {
		t.Errorf("SavedTo should be empty when no save path: %q", result.SavedTo)
	}
}

func TestSync_IncrementalWithPrevious(t *testing.T) {
	t.Parallel()

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/repos/") {
			meta := repoMeta{Stars: 100, Language: "Go"}
			json.NewEncoder(w).Encode(meta)
			return
		}

		// Awesome list
		if strings.Contains(path, "test/awesome") {
			callCount++
			if callCount == 1 {
				// First fetch
				md := `# Awesome List
## Tools
- [tool-a](https://github.com/org/tool-a) - mcp tool
`
				_, _ = w.Write([]byte(md))
			} else {
				// Second fetch: added tool-b
				md := `# Awesome List
## Tools
- [tool-a](https://github.com/org/tool-a) - mcp tool
- [tool-b](https://github.com/org/tool-b) - new tui agent
`
				_, _ = w.Write([]byte(md))
			}
			return
		}

		_, _ = w.Write([]byte("# Repo\n\nmcp tui agent"))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	dir := t.TempDir()

	// First sync (full)
	_, err := Sync(context.Background(), client, SyncOptions{
		Repo:       "test/awesome",
		SaveTo:     dir,
		FullRescan: true,
		MaxWorkers: 1,
	})
	if err != nil {
		t.Fatalf("First Sync: %v", err)
	}

	// Second sync (incremental, FullRescan=false)
	result, err := Sync(context.Background(), client, SyncOptions{
		Repo:       "test/awesome",
		SaveTo:     dir,
		FullRescan: false,
		MaxWorkers: 1,
	})
	if err != nil {
		t.Fatalf("Second Sync: %v", err)
	}

	if result.New != 1 {
		t.Errorf("new = %d, want 1", result.New)
	}
	// Only new entry should be analyzed
	if result.Analyzed != 1 {
		t.Errorf("analyzed = %d, want 1", result.Analyzed)
	}
}

func TestSync_NoNewEntries(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/repos/") {
			meta := repoMeta{Stars: 100, Language: "Go"}
			json.NewEncoder(w).Encode(meta)
			return
		}
		md := `# Awesome List
## Tools
- [tool-a](https://github.com/org/tool-a) - mcp tool
`
		_, _ = w.Write([]byte(md))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{
		base:    client.Transport,
		hostMap: srv.URL,
	}

	dir := t.TempDir()

	// First sync
	_, err := Sync(context.Background(), client, SyncOptions{
		Repo:       "test/awesome",
		SaveTo:     dir,
		FullRescan: true,
		MaxWorkers: 1,
	})
	if err != nil {
		t.Fatalf("First Sync: %v", err)
	}

	// Second sync with same data (no new entries)
	result, err := Sync(context.Background(), client, SyncOptions{
		Repo:       "test/awesome",
		SaveTo:     dir,
		FullRescan: false,
		MaxWorkers: 1,
	})
	if err != nil {
		t.Fatalf("Second Sync: %v", err)
	}

	if result.New != 0 {
		t.Errorf("new = %d, want 0", result.New)
	}
	if result.Analyzed != 0 {
		t.Errorf("analyzed = %d, want 0 (no new entries)", result.Analyzed)
	}
}

func TestMergeAnalysis(t *testing.T) {
	t.Parallel()

	existing := &Analysis{
		Source:   "test/repo",
		Analyzed: time.Now().UTC(),
		Entries: []AnalysisEntry{
			{AwesomeEntry: AwesomeEntry{Name: "tool-a", URL: "https://github.com/org/tool-a"}, Rating: RatingHigh, Stars: 100},
			{AwesomeEntry: AwesomeEntry{Name: "tool-b", URL: "https://github.com/org/tool-b"}, Rating: RatingLow, Stars: 10},
		},
	}

	newAnalysis := &Analysis{
		Source:   "test/repo",
		Analyzed: time.Now().UTC(),
		Entries: []AnalysisEntry{
			// tool-b updated
			{AwesomeEntry: AwesomeEntry{Name: "tool-b", URL: "https://github.com/org/tool-b"}, Rating: RatingMedium, Stars: 50},
			// tool-c is new
			{AwesomeEntry: AwesomeEntry{Name: "tool-c", URL: "https://github.com/org/tool-c"}, Rating: RatingNone, Stars: 1},
		},
	}

	merged := mergeAnalysis(existing, newAnalysis)

	if merged.Summary.Total != 3 {
		t.Errorf("total = %d, want 3", merged.Summary.Total)
	}

	// Find tool-b, should have updated rating
	byURL := make(map[string]AnalysisEntry)
	for _, e := range merged.Entries {
		byURL[e.URL] = e
	}

	if b, ok := byURL["https://github.com/org/tool-b"]; ok {
		if b.Rating != RatingMedium {
			t.Errorf("tool-b rating = %q, want MEDIUM (updated)", b.Rating)
		}
	} else {
		t.Error("tool-b missing from merged analysis")
	}

	if _, ok := byURL["https://github.com/org/tool-a"]; !ok {
		t.Error("tool-a missing from merged analysis (should be preserved)")
	}

	if _, ok := byURL["https://github.com/org/tool-c"]; !ok {
		t.Error("tool-c missing from merged analysis (new entry)")
	}
}

func TestMergeAnalysis_SummaryCounts(t *testing.T) {
	t.Parallel()

	existing := &Analysis{
		Entries: []AnalysisEntry{
			{AwesomeEntry: AwesomeEntry{URL: "u1"}, Rating: RatingHigh},
		},
	}
	newA := &Analysis{
		Source: "test",
		Entries: []AnalysisEntry{
			{AwesomeEntry: AwesomeEntry{URL: "u2"}, Rating: RatingMedium},
			{AwesomeEntry: AwesomeEntry{URL: "u3"}, Rating: RatingLow},
			{AwesomeEntry: AwesomeEntry{URL: "u4"}, Rating: RatingNone},
		},
	}

	merged := mergeAnalysis(existing, newA)
	if merged.Summary.High != 1 {
		t.Errorf("high = %d, want 1", merged.Summary.High)
	}
	if merged.Summary.Medium != 1 {
		t.Errorf("medium = %d, want 1", merged.Summary.Medium)
	}
	if merged.Summary.Low != 1 {
		t.Errorf("low = %d, want 1", merged.Summary.Low)
	}
	if merged.Summary.None != 1 {
		t.Errorf("none = %d, want 1", merged.Summary.None)
	}
	if merged.Summary.Total != 4 {
		t.Errorf("total = %d, want 4", merged.Summary.Total)
	}
}
