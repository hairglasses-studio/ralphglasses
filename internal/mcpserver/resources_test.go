package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

// setupRepoForResources creates a temp directory with a fake repo structure
// that the Server can find, and returns the Server and repo name.
func setupRepoForResources(t *testing.T) (*Server, *server.MCPServer, string) {
	t.Helper()

	scanDir := t.TempDir()
	repoName := "testrepo"
	repoPath := filepath.Join(scanDir, repoName)

	// Create .ralph directory structure.
	ralphDir := filepath.Join(repoPath, ".ralph")
	logsDir := filepath.Join(ralphDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a .git dir so discovery finds it as a repo.
	if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	appSrv := NewServer(scanDir)
	// Pre-populate repos to avoid needing real discovery.
	appSrv.Repos = []*model.Repo{{
		Name:     repoName,
		Path:     repoPath,
		HasRalph: true,
	}}

	mcpSrv := server.NewMCPServer("test", "0.0.0",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(false, false),
	)
	RegisterResources(mcpSrv, appSrv)

	return appSrv, mcpSrv, repoName
}

func TestResourceRegistration(t *testing.T) {
	t.Parallel()

	_, mcpSrv, _ := setupRepoForResources(t)

	// Verify resource templates are registered by sending a resources/list request.
	// The MCPServer should have 3 resource templates registered.
	ctx := context.Background()
	rawReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"0.0.0"}}}`
	mcpSrv.HandleMessage(ctx, []byte(rawReq))

	listReq := `{"jsonrpc":"2.0","id":2,"method":"resources/templates/list","params":{}}`
	resp := mcpSrv.HandleMessage(ctx, []byte(listReq))

	// The response should be a JSONRPCResponse (not an error).
	rpcResp, ok := resp.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("expected JSONRPCResponse, got %T: %+v", resp, resp)
	}

	// Parse the result to check templates.
	result, ok := rpcResp.Result.(mcp.ListResourceTemplatesResult)
	if !ok {
		t.Fatalf("expected ListResourceTemplatesResult, got %T", rpcResp.Result)
	}

	if len(result.ResourceTemplates) != 3 {
		t.Errorf("expected 3 resource templates, got %d", len(result.ResourceTemplates))
	}

	// Verify template URIs.
	uris := make(map[string]bool)
	for _, tmpl := range result.ResourceTemplates {
		uris[tmpl.URITemplate.Raw()] = true
	}
	for _, expected := range []string{
		"ralph:///{repo}/status",
		"ralph:///{repo}/progress",
		"ralph:///{repo}/logs",
	} {
		if !uris[expected] {
			t.Errorf("missing expected resource template URI: %s", expected)
		}
	}
}

func TestStatusResource_ReturnsJSON(t *testing.T) {
	t.Parallel()

	appSrv, _, repoName := setupRepoForResources(t)

	repo := appSrv.findRepo(repoName)
	statusContent := `{"phase":"running","step":42,"healthy":true}`
	statusPath := filepath.Join(repo.Path, ".ralph", "status.json")
	if err := os.WriteFile(statusPath, []byte(statusContent), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := makeStatusHandler(appSrv)
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: "ralph:///" + repoName + "/status",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}

	if textContent.Text != statusContent {
		t.Errorf("expected %q, got %q", statusContent, textContent.Text)
	}
	if textContent.MIMEType != "application/json" {
		t.Errorf("expected MIME type application/json, got %s", textContent.MIMEType)
	}
}

func TestStatusResource_NoFile(t *testing.T) {
	t.Parallel()

	appSrv, _, repoName := setupRepoForResources(t)

	handler := makeStatusHandler(appSrv)
	_, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: "ralph:///" + repoName + "/status",
		},
	})

	if err == nil {
		t.Fatal("expected error for missing status.json")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestProgressResource_ReturnsJSON(t *testing.T) {
	t.Parallel()

	appSrv, _, repoName := setupRepoForResources(t)

	repo := appSrv.findRepo(repoName)
	progressContent := `{"completed":5,"total":10,"percent":50}`
	progressPath := filepath.Join(repo.Path, ".ralph", "progress.json")
	if err := os.WriteFile(progressPath, []byte(progressContent), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := makeProgressHandler(appSrv)
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: "ralph:///" + repoName + "/progress",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}

	if textContent.Text != progressContent {
		t.Errorf("expected %q, got %q", progressContent, textContent.Text)
	}
}

func TestLogsResource_TailsOutput(t *testing.T) {
	t.Parallel()

	appSrv, _, repoName := setupRepoForResources(t)

	repo := appSrv.findRepo(repoName)
	logPath := filepath.Join(repo.Path, ".ralph", "logs", "ralph.log")

	// Write 150 lines; we expect only the last 100.
	var lines []string
	for i := 1; i <= 150; i++ {
		lines = append(lines, strings.Repeat("x", 10)+" line "+string(rune('0'+i%10)))
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := makeLogsHandler(appSrv)
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: "ralph:///" + repoName + "/logs",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}

	// Should contain last 100 lines.
	outputLines := strings.Split(textContent.Text, "\n")
	if len(outputLines) != 100 {
		t.Errorf("expected 100 lines, got %d", len(outputLines))
	}
	if textContent.MIMEType != "text/plain" {
		t.Errorf("expected MIME type text/plain, got %s", textContent.MIMEType)
	}
}

func TestLogsResource_NoFile(t *testing.T) {
	t.Parallel()

	appSrv, _, repoName := setupRepoForResources(t)

	handler := makeLogsHandler(appSrv)
	_, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: "ralph:///" + repoName + "/logs",
		},
	})

	if err == nil {
		t.Fatal("expected error for missing ralph.log")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestStatusResource_UnknownRepo(t *testing.T) {
	t.Parallel()

	appSrv, _, _ := setupRepoForResources(t)

	handler := makeStatusHandler(appSrv)
	_, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: "ralph:///nonexistent/status",
		},
	})

	if err == nil {
		t.Fatal("expected error for unknown repo")
	}
	if !strings.Contains(err.Error(), "repo not found") {
		t.Errorf("expected 'repo not found' in error, got: %v", err)
	}
}

func TestExtractRepoName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		uri  string
		want string
	}{
		{"ralph:///myrepo/status", "myrepo"},
		{"ralph:///myrepo/progress", "myrepo"},
		{"ralph:///myrepo/logs", "myrepo"},
		{"ralph:///my-repo-name/status", "my-repo-name"},
		{"ralph:///", ""},
		{"http:///foo/bar", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := extractRepoName(tt.uri)
		if got != tt.want {
			t.Errorf("extractRepoName(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}
