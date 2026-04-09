package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHandleCycleMerge_HappyPath(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	// Create a single main repo
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}

	if err := exec.Command("git", "init", repo).Run(); err != nil { t.Fatalf("git command failed: %v", err) }
	os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base"), 0644)
	if err := exec.Command("git", "-C", repo, "add", ".").Run(); err != nil { t.Fatalf("git command failed: %v", err) }
	if err := exec.Command("git", "-C", repo, "commit", "-m", "init").Run(); err != nil { t.Fatalf("git command failed: %v", err) }

	// Create main branch just in case default was master
	if err := exec.Command("git", "-C", repo, "branch", "-M", "main").Run(); err != nil { t.Fatalf("git command failed: %v", err) }

	// branch 1
	if err := exec.Command("git", "-C", repo, "checkout", "-b", "branch1").Run(); err != nil { t.Fatalf("git command failed: %v", err) }
	os.WriteFile(filepath.Join(repo, "file_a.go"), []byte("package main"), 0644)
	if err := exec.Command("git", "-C", repo, "add", ".").Run(); err != nil { t.Fatalf("git command failed: %v", err) }
	if err := exec.Command("git", "-C", repo, "commit", "-m", "branch1").Run(); err != nil { t.Fatalf("git command failed: %v", err) }

	// branch 2
	if err := exec.Command("git", "-C", repo, "checkout", "main").Run(); err != nil { t.Fatalf("git command failed: %v", err) }
	if err := exec.Command("git", "-C", repo, "checkout", "-b", "branch2").Run(); err != nil { t.Fatalf("git command failed: %v", err) }
	os.WriteFile(filepath.Join(repo, "file_b.go"), []byte("package main"), 0644)
	if err := exec.Command("git", "-C", repo, "add", ".").Run(); err != nil { t.Fatalf("git command failed: %v", err) }
	if err := exec.Command("git", "-C", repo, "commit", "-m", "branch2").Run(); err != nil { t.Fatalf("git command failed: %v", err) }

	// The MCP handler expects "worktrees".
	// Let's create two actual worktrees for branch1 and branch2.
	wt1 := filepath.Join(repo, "wt1")
	wt2 := filepath.Join(repo, "wt2")
	if err := exec.Command("git", "-C", repo, "checkout", "main").Run(); err != nil { t.Fatalf("git command failed: %v", err) }
	if err := exec.Command("git", "-C", repo, "worktree", "add", wt1, "branch1").Run(); err != nil { t.Fatalf("git command failed: %v", err) }
	if err := exec.Command("git", "-C", repo, "worktree", "add", wt2, "branch2").Run(); err != nil { t.Fatalf("git command failed: %v", err) }

	// Need to check out main in the main repo because the merge happens there
	if err := exec.Command("git", "-C", repo, "checkout", "main").Run(); err != nil { t.Fatalf("git command failed: %v", err) }

	result, err := srv.handleCycleMerge(context.Background(), makeRequest(map[string]any{
		"worktree_paths": wt1 + "," + wt2,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleCycleMerge returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["merge_status"] != "completed" {
		t.Errorf("status = %v, want completed", data["merge_status"])
	}
	if data["worktree_count"] != float64(2) {
		t.Errorf("worktree_count = %v, want 2", data["worktree_count"])
	}
}
