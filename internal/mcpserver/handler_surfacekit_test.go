package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestResolveSurfaceAuditPathsPrefersCodexkit(t *testing.T) {
	t.Setenv("HG_STUDIO_ROOT", "")

	workspace := t.TempDir()
	scanPath := filepath.Join(workspace, "ralphglasses")
	if err := os.MkdirAll(scanPath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, repo := range []string{"codexkit", "surfacekit"} {
		scriptPath := filepath.Join(workspace, repo, "scripts", "agent-parity-audit.sh")
		if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	studioRoot, auditRoot, scriptPath, err := resolveSurfaceAuditPaths(scanPath)
	if err != nil {
		t.Fatal(err)
	}
	if studioRoot != workspace {
		t.Fatalf("studioRoot = %q, want %q", studioRoot, workspace)
	}
	if auditRoot != filepath.Join(workspace, "codexkit") {
		t.Fatalf("auditRoot = %q, want codexkit root", auditRoot)
	}
	if scriptPath != filepath.Join(workspace, "codexkit", "scripts", "agent-parity-audit.sh") {
		t.Fatalf("scriptPath = %q, want codexkit script", scriptPath)
	}
}

func TestResolveSurfaceAuditPathsFallsBackToSurfacekit(t *testing.T) {
	t.Setenv("HG_STUDIO_ROOT", "")

	workspace := t.TempDir()
	scanPath := filepath.Join(workspace, "ralphglasses")
	if err := os.MkdirAll(scanPath, 0o755); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(workspace, "surfacekit", "scripts", "agent-parity-audit.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	studioRoot, auditRoot, resolvedScript, err := resolveSurfaceAuditPaths(scanPath)
	if err != nil {
		t.Fatal(err)
	}
	if studioRoot != workspace {
		t.Fatalf("studioRoot = %q, want %q", studioRoot, workspace)
	}
	if auditRoot != filepath.Join(workspace, "surfacekit") {
		t.Fatalf("auditRoot = %q, want surfacekit root", auditRoot)
	}
	if resolvedScript != scriptPath {
		t.Fatalf("resolvedScript = %q, want %q", resolvedScript, scriptPath)
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
