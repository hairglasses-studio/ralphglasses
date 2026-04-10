package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestResolveParityAuditPathsPrefersCodexkit(t *testing.T) {
	t.Setenv("HG_STUDIO_ROOT", "")

	workspace := t.TempDir()
	scanPath := filepath.Join(workspace, "ralphglasses")
	if err := os.MkdirAll(scanPath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, repo := range []string{"codexkit", "surfacekit"} {
		mainPath := filepath.Join(workspace, repo, "cmd", "codexkit", "main.go")
		if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mainPath, []byte("package main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	studioRoot, codexkitRoot, err := resolveParityAuditPaths(scanPath)
	if err != nil {
		t.Fatal(err)
	}
	if studioRoot != workspace {
		t.Fatalf("studioRoot = %q, want %q", studioRoot, workspace)
	}
	if codexkitRoot != filepath.Join(workspace, "codexkit") {
		t.Fatalf("codexkitRoot = %q, want codexkit root", codexkitRoot)
	}
}

func TestResolveParityAuditPathsIgnoresSurfacekitFallback(t *testing.T) {
	t.Setenv("HG_STUDIO_ROOT", "")

	workspace := t.TempDir()
	scanPath := filepath.Join(workspace, "ralphglasses")
	if err := os.MkdirAll(scanPath, 0o755); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(workspace, "surfacekit", "cmd", "codexkit", "main.go")
	if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := resolveParityAuditPaths(scanPath); err == nil {
		t.Fatal("expected missing codexkit error")
	}
}

func TestHandleSurfaceAuditFallsBackToDocsInventory(t *testing.T) {
	t.Setenv("HG_STUDIO_ROOT", "")

	workspace := t.TempDir()
	inventoryPath := filepath.Join(workspace, "docs", "projects", "agent-parity", "repo-inventory.json")
	if err := os.MkdirAll(filepath.Dir(inventoryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := `{"summary":{"total_repos_with_ollama_support":8},"repos":[{"repo":"ralphglasses","ollama_support_mode":"session_provider"}]}`
	if err := os.WriteFile(inventoryPath, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(workspace)
	result, err := srv.handleSurfaceAudit(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("handleSurfaceAudit returned error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected non-error result, got %+v", result)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content entry, got %d", len(result.Content))
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if text.Text != payload {
		t.Fatalf("payload mismatch: got %q want %q", text.Text, payload)
	}
}
