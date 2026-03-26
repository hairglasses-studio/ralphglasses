package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
