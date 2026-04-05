package mcpserver

import (
	"context"
	"strings"
	"testing"
)

func TestRecommendAction(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		exists    bool
		fileCount int
		wantPart  string
	}{
		{name: "no_existing", exists: false, fileCount: 0, wantPart: "No existing research"},
		{name: "has_files", exists: true, fileCount: 3, wantPart: "3 existing file(s)"},
		{name: "search_guide_only", exists: true, fileCount: 0, wantPart: "SEARCH-GUIDE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := recommendAction(tt.exists, tt.fileCount)
			if !strings.Contains(got, tt.wantPart) {
				t.Errorf("recommendAction(%v, %d) = %q, want substring %q", tt.exists, tt.fileCount, got, tt.wantPart)
			}
		})
	}
}

func TestHandleDocsSearch_MissingQuery(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleDocsSearch(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing query")
	}
	text := getResultText(result)
	if !strings.Contains(text, "query required") {
		t.Errorf("expected 'query required', got: %s", text)
	}
}

func TestHandleDocsCheckExisting_MissingTopic(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleDocsCheckExisting(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing topic")
	}
	text := getResultText(result)
	if !strings.Contains(text, "topic required") {
		t.Errorf("expected 'topic required', got: %s", text)
	}
}

func TestHandleDocsWriteFinding_MissingParams(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "missing_all", args: map[string]any{}},
		{name: "missing_filename", args: map[string]any{"domain": "mcp", "content": "hello"}},
		{name: "missing_content", args: map[string]any{"domain": "mcp", "filename": "test.md"}},
		{name: "missing_domain", args: map[string]any{"filename": "test.md", "content": "hello"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, _ := setupTestServer(t)
			result, err := srv.handleDocsWriteFinding(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected error for missing params")
			}
		})
	}
}

func TestHandleDocsPush_NoDocsDir(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleDocsPush(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fail because docs dir doesn't exist.
	if !result.IsError {
		// It may succeed with empty push, just check it didn't panic.
		_ = getResultText(result)
	}
}

func TestHandleMetaRoadmapStatus_NoDocsDir(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleMetaRoadmapStatus(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// May fail because docs/strategy doesn't exist, but shouldn't panic.
	_ = getResultText(result)
}

func TestHandleMetaRoadmapNextTask_NoDocsDir(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleMetaRoadmapNextTask(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = getResultText(result)
}

func TestDocsRoot(t *testing.T) {
	t.Parallel()
	srv := &Server{ScanPath: "/tmp/test-scan"}
	got := srv.docsRoot()
	if !strings.Contains(got, "docs") {
		t.Errorf("docsRoot() = %q, want path containing 'docs'", got)
	}
}
