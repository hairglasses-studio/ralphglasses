package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/mark3labs/mcp-go/mcp"
)

func scratchpadServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	ralphDir := filepath.Join(root, ".ralph")
	if err := os.MkdirAll(ralphDir, 0o755); err != nil {
		t.Fatal(err)
	}
	srv := &Server{ScanPath: root}
	return srv, root
}

func TestHandleScratchpadReadMissing(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name": "nonexistent",
	}

	result, err := srv.handleScratchpadRead(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, `"status":"empty"`) {
		t.Errorf("expected empty status JSON, got: %s", text)
	}
	if !strings.Contains(text, `"item_type":"scratchpad"`) {
		t.Errorf("expected item_type scratchpad in empty result, got: %s", text)
	}
}

func TestHandleScratchpadAppendAndRead(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	// Append content.
	appendReq := mcp.CallToolRequest{}
	appendReq.Params.Arguments = map[string]any{
		"name":    "test_notes",
		"content": "1. First item\n2. Second item",
	}

	result, err := srv.handleScratchpadAppend(context.Background(), appendReq)
	if err != nil {
		t.Fatalf("append error: %v", err)
	}
	if result.IsError {
		t.Fatalf("append tool error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Appended to test_notes scratchpad") {
		t.Errorf("unexpected append result: %s", text)
	}

	// Read it back.
	readReq := mcp.CallToolRequest{}
	readReq.Params.Arguments = map[string]any{
		"name": "test_notes",
	}

	result, err = srv.handleScratchpadRead(context.Background(), readReq)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if result.IsError {
		t.Fatalf("read tool error: %v", result.Content)
	}

	text = result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "1. First item") {
		t.Errorf("expected first item in read result, got: %s", text)
	}
	if !strings.Contains(text, "2. Second item") {
		t.Errorf("expected second item in read result, got: %s", text)
	}
}

func TestHandleScratchpadAppendWithSection(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":    "observations",
		"content": "Something noteworthy happened.",
		"section": "Round 2 Notes",
	}

	result, err := srv.handleScratchpadAppend(context.Background(), req)
	if err != nil {
		t.Fatalf("append error: %v", err)
	}
	if result.IsError {
		t.Fatalf("append tool error: %v", result.Content)
	}

	// Read and verify section header.
	readReq := mcp.CallToolRequest{}
	readReq.Params.Arguments = map[string]any{
		"name": "observations",
	}

	result, err = srv.handleScratchpadRead(context.Background(), readReq)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "## Round 2 Notes") {
		t.Errorf("expected section header, got: %s", text)
	}
	if !strings.Contains(text, "Something noteworthy happened.") {
		t.Errorf("expected content after section header, got: %s", text)
	}
}

func TestHandleScratchpadList(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)

	// Create two scratchpad files.
	ralphDir := filepath.Join(root, ".ralph")
	if err := os.WriteFile(filepath.Join(ralphDir, "alpha_scratchpad.md"), []byte("# Alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ralphDir, "beta_scratchpad.md"), []byte("# Beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := srv.handleScratchpadList(context.Background(), req)
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if result.IsError {
		t.Fatalf("list tool error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var names []string
	if err := json.Unmarshal([]byte(text), &names); err != nil {
		t.Fatalf("invalid JSON: %v (text: %s)", err, text)
	}

	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["alpha"] {
		t.Errorf("expected 'alpha' in list, got: %v", names)
	}
	if !found["beta"] {
		t.Errorf("expected 'beta' in list, got: %v", names)
	}
}

func TestHandleScratchpadListDedup(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)

	// Create files that would cause duplicates: "tool_improvement_scratchpad.md"
	// yields "tool_improvement" after suffix trim, which is the same as what
	// "tool_improvement_scratchpad_scratchpad.md" would yield after double trim.
	// In practice the duplicate comes from a file like "tool_improvement_scratchpad.md"
	// being listed alongside other scratchpads.
	ralphDir := filepath.Join(root, ".ralph")
	// Normal scratchpad
	if err := os.WriteFile(filepath.Join(ralphDir, "alpha_scratchpad.md"), []byte("# Alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Another scratchpad whose base name after _scratchpad.md trim is "tool_improvement"
	if err := os.WriteFile(filepath.Join(ralphDir, "tool_improvement_scratchpad.md"), []byte("# TI\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A file that would produce "tool_improvement_scratchpad" after removing "_scratchpad.md"
	// then gets deduped to "tool_improvement" after the extra _scratchpad strip.
	if err := os.WriteFile(filepath.Join(ralphDir, "tool_improvement_scratchpad_scratchpad.md"), []byte("# TI dup\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := srv.handleScratchpadList(context.Background(), req)
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if result.IsError {
		t.Fatalf("list tool error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var names []string
	if err := json.Unmarshal([]byte(text), &names); err != nil {
		t.Fatalf("invalid JSON: %v (text: %s)", err, text)
	}

	// Count occurrences of tool_improvement
	tiCount := 0
	for _, n := range names {
		if n == "tool_improvement" {
			tiCount++
		}
	}
	if tiCount != 1 {
		t.Errorf("expected exactly 1 'tool_improvement' entry, got %d (names: %v)", tiCount, names)
	}

	// Should have 2 unique names: alpha, tool_improvement
	if len(names) != 2 {
		t.Errorf("expected 2 unique scratchpads, got %d: %v", len(names), names)
	}
}

func TestHandleScratchpadResolve(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)

	// Write a scratchpad with numbered items.
	content := `# Issues

1. First problem
2. Second problem
3. Third problem
`
	ralphDir := filepath.Join(root, ".ralph")
	if err := os.WriteFile(filepath.Join(ralphDir, "issues_scratchpad.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Resolve item 2.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":        "issues",
		"item_number": float64(2),
		"resolution":  "fixed in v1.1",
	}

	result, err := srv.handleScratchpadResolve(context.Background(), req)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if result.IsError {
		t.Fatalf("resolve tool error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Resolved item 2") {
		t.Errorf("unexpected resolve result: %s", text)
	}

	// Read and verify the resolved line.
	data, err := os.ReadFile(filepath.Join(ralphDir, "issues_scratchpad.md"))
	if err != nil {
		t.Fatal(err)
	}
	fileText := string(data)
	if !strings.Contains(fileText, "2. Second problem -- RESOLVED: fixed in v1.1") {
		t.Errorf("expected resolved marker in file, got:\n%s", fileText)
	}
	// Other items should not be resolved.
	if strings.Contains(fileText, "1. First problem -- RESOLVED") {
		t.Errorf("item 1 should not be resolved")
	}
	if strings.Contains(fileText, "3. Third problem -- RESOLVED") {
		t.Errorf("item 3 should not be resolved")
	}
}

func TestHandleScratchpadDelete(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)

	// Write a scratchpad with numbered items.
	content := "# Findings\n\n1. First finding\n2. Second finding\n3. Third finding\n"
	ralphDir := filepath.Join(root, ".ralph")
	if err := os.WriteFile(filepath.Join(ralphDir, "bugs_scratchpad.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Delete item 2.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"scratchpad": "bugs",
		"finding_id": "2",
	}

	result, err := srv.handleScratchpadDelete(context.Background(), req)
	if err != nil {
		t.Fatalf("delete error: %v", err)
	}
	if result.IsError {
		t.Fatalf("delete tool error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Deleted finding 2") {
		t.Errorf("unexpected delete result: %s", text)
	}
	if !strings.Contains(text, "Second finding") {
		t.Errorf("expected deleted finding summary in result: %s", text)
	}

	// Read and verify item 2 is gone.
	data, err := os.ReadFile(filepath.Join(ralphDir, "bugs_scratchpad.md"))
	if err != nil {
		t.Fatal(err)
	}
	fileText := string(data)
	if strings.Contains(fileText, "2. Second finding") {
		t.Errorf("item 2 should have been deleted, got:\n%s", fileText)
	}
	if !strings.Contains(fileText, "1. First finding") {
		t.Errorf("item 1 should still exist, got:\n%s", fileText)
	}
	if !strings.Contains(fileText, "3. Third finding") {
		t.Errorf("item 3 should still exist, got:\n%s", fileText)
	}
}

func TestHandleScratchpadDelete_NotFound(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)

	// Write a scratchpad.
	ralphDir := filepath.Join(root, ".ralph")
	if err := os.WriteFile(filepath.Join(ralphDir, "notes_scratchpad.md"), []byte("1. Only item\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"scratchpad": "notes",
		"finding_id": "99",
	}

	result, err := srv.handleScratchpadDelete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent finding")
	}
}

func TestHandleScratchpadDelete_MissingParams(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	// Missing finding_id.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"scratchpad": "test",
	}

	result, err := srv.handleScratchpadDelete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing finding_id")
	}

	// Missing scratchpad.
	req.Params.Arguments = map[string]any{
		"finding_id": "1",
	}

	result, err = srv.handleScratchpadDelete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing scratchpad")
	}
}

func TestResolveRepoPath_SingleRepo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	srv := &Server{
		Repos: []*model.Repo{{Name: "myrepo", Path: root}},
	}

	got, err := srv.resolveRepoPath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != root {
		t.Errorf("expected %s, got %s", root, got)
	}
}

func TestResolveRepoPath_ExplicitRepoParam(t *testing.T) {
	t.Parallel()
	rootA := t.TempDir()
	rootB := t.TempDir()
	srv := &Server{
		Repos: []*model.Repo{
			{Name: "alpha", Path: rootA},
			{Name: "beta", Path: rootB},
		},
	}

	got, err := srv.resolveRepoPath("beta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != rootB {
		t.Errorf("expected %s, got %s", rootB, got)
	}
}

func TestResolveRepoPath_ExplicitInvalidRepo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	srv := &Server{
		Repos: []*model.Repo{{Name: "alpha", Path: root}},
	}

	_, err := srv.resolveRepoPath("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent repo")
	}
	if !strings.Contains(err.Error(), "repo not found") {
		t.Errorf("expected 'repo not found' in error, got: %v", err)
	}
}

func TestResolveRepoPath_MultiReposCWDInside(t *testing.T) {
	// Not parallel: os.Chdir is process-global.
	rootA := t.TempDir()
	rootB := t.TempDir()

	// Create a subdirectory inside rootB to simulate CWD being inside that repo.
	subdir := filepath.Join(rootB, "src", "pkg")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	srv := &Server{
		Repos: []*model.Repo{
			{Name: "alpha", Path: rootA},
			{Name: "beta", Path: rootB},
		},
	}

	// Change CWD to subdir inside rootB.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	if err := os.Chdir(subdir); err != nil {
		t.Fatal(err)
	}

	got, err := srv.resolveRepoPath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != rootB {
		t.Errorf("expected %s, got %s", rootB, got)
	}
}

func TestResolveRepoPath_MultiReposCWDOutside(t *testing.T) {
	// Not parallel: os.Chdir is process-global.
	rootA := t.TempDir()
	rootB := t.TempDir()
	outside := t.TempDir() // CWD outside all repos

	srv := &Server{
		Repos: []*model.Repo{
			{Name: "alpha", Path: rootA},
			{Name: "beta", Path: rootB},
		},
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	if err := os.Chdir(outside); err != nil {
		t.Fatal(err)
	}

	_, err = srv.resolveRepoPath("")
	if err == nil {
		t.Fatal("expected error when CWD is outside all repos")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "multiple repos found") {
		t.Errorf("expected 'multiple repos found' in error, got: %v", errMsg)
	}
	if !strings.Contains(errMsg, "available:") {
		t.Errorf("expected 'available:' in error, got: %v", errMsg)
	}
	if !strings.Contains(errMsg, "alpha") || !strings.Contains(errMsg, "beta") {
		t.Errorf("expected repo names in error, got: %v", errMsg)
	}
}
