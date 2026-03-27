package mcpserver

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

const testRoadmapContent = `# Test Project Roadmap

## Phase 0: Foundation (COMPLETE)

- [x] Set up Go module
- [x] Add CLI framework

## Phase 1: Core Features

### 1.1 — Parser
- [ ] 1.1.1 — Implement line parser
- [x] 1.1.2 — Write unit tests
- **Acceptance:** parser handles all edge cases

### 1.2 — Analyzer
- [ ] 1.2.1 — Walk filesystem
- **Acceptance:** analyzer detects gaps

## Phase 2: Advanced

- [ ] Add documentation
- [ ] Add CI pipeline
`

// roadmapReq builds a CallToolRequest with the given args map.
func roadmapReq(args map[string]any) mcp.CallToolRequest {
	return makeReq("", args)
}

// setupRepoWithRoadmap creates a temp dir under scanPath with a ROADMAP.md.
func setupRepoWithRoadmap(t *testing.T) (scanPath, repoPath string) {
	t.Helper()
	scanPath = t.TempDir()
	repoPath = filepath.Join(scanPath, "test-repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "ROADMAP.md"), []byte(testRoadmapContent), 0644); err != nil {
		t.Fatal(err)
	}
	return scanPath, repoPath
}

// newTestServer returns a Server with the given scanPath.
func newTestServer(scanPath string) *Server {
	return &Server{
		ScanPath:   scanPath,
		HTTPClient: &http.Client{},
	}
}

// --- handleRoadmapParse ---

func TestHandleRoadmapParse_NoRepo(t *testing.T) {
	t.Parallel()
	s := newTestServer(t.TempDir())
	res, err := s.handleRoadmapParse(context.Background(), roadmapReq(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result for missing path")
	}
	text := extractText(t, res)
	if !strings.Contains(text, "path required") {
		t.Errorf("unexpected error text: %s", text)
	}
}

func TestHandleRoadmapParse_ValidRepo(t *testing.T) {
	t.Parallel()
	scanPath, repoPath := setupRepoWithRoadmap(t)
	s := newTestServer(scanPath)
	res, err := s.handleRoadmapParse(context.Background(), roadmapReq(map[string]any{
		"path": repoPath,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", extractText(t, res))
	}
	text := extractText(t, res)
	if !strings.Contains(text, "Test Project Roadmap") {
		t.Errorf("expected parsed roadmap title in output, got: %s", text)
	}
	if !strings.Contains(text, "phases") {
		t.Errorf("expected phases in output, got: %s", text)
	}
}

func TestHandleRoadmapParse_NoRoadmap(t *testing.T) {
	t.Parallel()
	scanPath := t.TempDir()
	repoPath := filepath.Join(scanPath, "empty-repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(scanPath)
	res, err := s.handleRoadmapParse(context.Background(), roadmapReq(map[string]any{
		"path": repoPath,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result for missing ROADMAP.md")
	}
	text := extractText(t, res)
	if !strings.Contains(text, "parse roadmap") {
		t.Errorf("expected parse error in output, got: %s", text)
	}
}

// --- handleRoadmapAnalyze ---

func TestHandleRoadmapAnalyze_NoRepo(t *testing.T) {
	t.Parallel()
	s := newTestServer(t.TempDir())
	res, err := s.handleRoadmapAnalyze(context.Background(), roadmapReq(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result for missing path")
	}
	text := extractText(t, res)
	if !strings.Contains(text, "path required") {
		t.Errorf("unexpected error text: %s", text)
	}
}

func TestHandleRoadmapAnalyze_ValidRepo(t *testing.T) {
	t.Parallel()
	scanPath, repoPath := setupRepoWithRoadmap(t)
	s := newTestServer(scanPath)
	res, err := s.handleRoadmapAnalyze(context.Background(), roadmapReq(map[string]any{
		"path": repoPath,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", extractText(t, res))
	}
	text := extractText(t, res)
	if !strings.Contains(text, "gaps") && !strings.Contains(text, "ready") {
		t.Errorf("expected analysis fields in output, got: %s", text)
	}
}

// --- handleRoadmapResearch ---

func TestHandleRoadmapResearch_NoPath(t *testing.T) {
	t.Parallel()
	s := newTestServer(t.TempDir())
	res, err := s.handleRoadmapResearch(context.Background(), roadmapReq(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result for missing path")
	}
	text := extractText(t, res)
	if !strings.Contains(text, "path required") {
		t.Errorf("unexpected error text: %s", text)
	}
}

// --- handleRoadmapExpand ---

func TestHandleRoadmapExpand_NoRepo(t *testing.T) {
	t.Parallel()
	s := newTestServer(t.TempDir())
	res, err := s.handleRoadmapExpand(context.Background(), roadmapReq(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result for missing path")
	}
	text := extractText(t, res)
	if !strings.Contains(text, "path required") {
		t.Errorf("unexpected error text: %s", text)
	}
}

func TestHandleRoadmapExpand_ValidRepo(t *testing.T) {
	t.Parallel()
	scanPath, repoPath := setupRepoWithRoadmap(t)
	s := newTestServer(scanPath)
	res, err := s.handleRoadmapExpand(context.Background(), roadmapReq(map[string]any{
		"path":  repoPath,
		"style": "conservative",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", extractText(t, res))
	}
	text := extractText(t, res)
	if !strings.Contains(text, "proposals") {
		t.Errorf("expected proposals in output, got: %s", text)
	}
}

// --- handleRoadmapExport ---

func TestHandleRoadmapExport_NoRepo(t *testing.T) {
	t.Parallel()
	s := newTestServer(t.TempDir())
	res, err := s.handleRoadmapExport(context.Background(), roadmapReq(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result for missing path")
	}
	text := extractText(t, res)
	if !strings.Contains(text, "path required") {
		t.Errorf("unexpected error text: %s", text)
	}
}

func TestHandleRoadmapExport_ValidRepo(t *testing.T) {
	t.Parallel()
	scanPath, repoPath := setupRepoWithRoadmap(t)
	s := newTestServer(scanPath)
	res, err := s.handleRoadmapExport(context.Background(), roadmapReq(map[string]any{
		"path": repoPath,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", extractText(t, res))
	}
	text := extractText(t, res)
	if text == "" {
		t.Error("expected non-empty export output")
	}
}

func TestHandleRoadmapExport_WithFormat(t *testing.T) {
	t.Parallel()
	scanPath, repoPath := setupRepoWithRoadmap(t)
	s := newTestServer(scanPath)
	res, err := s.handleRoadmapExport(context.Background(), roadmapReq(map[string]any{
		"path":   repoPath,
		"format": "rdcycle",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", extractText(t, res))
	}
	text := extractText(t, res)
	if !strings.Contains(text, "{") {
		t.Errorf("expected JSON output for rdcycle format, got: %s", text)
	}
}

// extractText pulls the text from the first content item of a CallToolResult.
func extractText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, not TextContent", res.Content[0])
	}
	return tc.Text
}

// --- handleRoadmapParse with phase filter ---

func TestHandleRoadmapParse_PhaseFilter(t *testing.T) {
	t.Parallel()
	scanPath, repoPath := setupRepoWithRoadmap(t)
	s := newTestServer(scanPath)
	res, err := s.handleRoadmapParse(context.Background(), roadmapReq(map[string]any{
		"path":  repoPath,
		"phase": "Phase 1: Core Features",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", extractText(t, res))
	}
	text := extractText(t, res)
	if !strings.Contains(text, "Core Features") {
		t.Errorf("expected Core Features phase in output, got: %s", text)
	}
	// Should not contain other phases
	if strings.Contains(text, "Phase 2") {
		t.Errorf("expected Phase 2 to be filtered out, got: %s", text)
	}
}

func TestHandleRoadmapParse_PhaseFilterNoMatch(t *testing.T) {
	t.Parallel()
	scanPath, repoPath := setupRepoWithRoadmap(t)
	s := newTestServer(scanPath)
	res, err := s.handleRoadmapParse(context.Background(), roadmapReq(map[string]any{
		"path":  repoPath,
		"phase": "Nonexistent Phase",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", extractText(t, res))
	}
	text := extractText(t, res)
	// Should still return valid JSON with empty phases
	if !strings.Contains(text, "phases") {
		t.Errorf("expected phases key in output, got: %s", text)
	}
}

// --- handleRoadmapAnalyze with category filter ---

func TestHandleRoadmapAnalyze_CategoryFilter(t *testing.T) {
	t.Parallel()
	scanPath, repoPath := setupRepoWithRoadmap(t)
	s := newTestServer(scanPath)

	// Test each category filter
	for _, cat := range []string{"gaps", "stale", "orphaned", "ready"} {
		res, err := s.handleRoadmapAnalyze(context.Background(), roadmapReq(map[string]any{
			"path":     repoPath,
			"category": cat,
		}))
		if err != nil {
			t.Fatalf("unexpected error for category %s: %v", cat, err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result for category %s: %s", cat, extractText(t, res))
		}
		text := extractText(t, res)
		if !strings.Contains(text, cat) {
			t.Errorf("expected %s key in output, got: %s", cat, text)
		}
		if !strings.Contains(text, "summary") {
			t.Errorf("expected summary in output for category %s, got: %s", cat, text)
		}
	}
}

func TestHandleRoadmapAnalyze_Limit(t *testing.T) {
	t.Parallel()
	scanPath, repoPath := setupRepoWithRoadmap(t)
	s := newTestServer(scanPath)
	res, err := s.handleRoadmapAnalyze(context.Background(), roadmapReq(map[string]any{
		"path":  repoPath,
		"limit": float64(1),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", extractText(t, res))
	}
	text := extractText(t, res)
	// Should still have valid analysis output
	if !strings.Contains(text, "gaps") || !strings.Contains(text, "summary") {
		t.Errorf("expected analysis fields in output, got: %s", text)
	}
}

// --- handleRoadmapExpand with phase filter and limit ---

func TestHandleRoadmapExpand_PhaseFilter(t *testing.T) {
	t.Parallel()
	scanPath, repoPath := setupRepoWithRoadmap(t)
	s := newTestServer(scanPath)
	res, err := s.handleRoadmapExpand(context.Background(), roadmapReq(map[string]any{
		"path":  repoPath,
		"phase": "Phase 1: Core Features",
		"style": "conservative",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", extractText(t, res))
	}
	text := extractText(t, res)
	if !strings.Contains(text, "proposals") {
		t.Errorf("expected proposals in output, got: %s", text)
	}
}

func TestHandleRoadmapExpand_Limit(t *testing.T) {
	t.Parallel()
	scanPath, repoPath := setupRepoWithRoadmap(t)
	s := newTestServer(scanPath)
	res, err := s.handleRoadmapExpand(context.Background(), roadmapReq(map[string]any{
		"path":  repoPath,
		"limit": float64(1),
		"style": "aggressive",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", extractText(t, res))
	}
	text := extractText(t, res)
	if !strings.Contains(text, "proposals") {
		t.Errorf("expected proposals in output, got: %s", text)
	}
}
