package session

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMergeParallelBranches(t *testing.T) {
	dir := t.TempDir()

	// Init git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", dir, "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", dir, "config", "user.name", "Test User").Run(); err != nil {
		t.Fatal(err)
	}

	// Create an initial commit
	err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("line 1\nline 2\nline 3\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "init").Run()
	if err := exec.Command("git", "-C", dir, "branch", "-M", "main").Run(); err != nil {
		t.Fatal(err)
	}

	// Create branch 1
	exec.Command("git", "-C", dir, "checkout", "-b", "branch1").Run()
	err = os.WriteFile(filepath.Join(dir, "file.txt"), []byte("line 1 modified\nline 2\nline 3\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "branch1 edit").Run()

	// Create branch 2 (no conflict with branch 1)
	exec.Command("git", "-C", dir, "checkout", "main").Run()
	exec.Command("git", "-C", dir, "checkout", "-b", "branch2").Run()
	err = os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("new file\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "branch2 edit").Run()

	// Create branch 3 (conflict with branch 1)
	exec.Command("git", "-C", dir, "checkout", "main").Run()
	exec.Command("git", "-C", dir, "checkout", "-b", "branch3").Run()
	err = os.WriteFile(filepath.Join(dir, "file.txt"), []byte("line 1 conflict\nline 2\nline 3\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "branch3 edit").Run()

	ctx := context.Background()
	result, err := MergeParallelBranches(ctx, dir, "main", []string{"branch1", "branch2", "branch3"}, "auto")
	if err != nil {
		t.Fatalf("MergeParallelBranches failed: %v", err)
	}

	if result.TargetBranch != "main" {
		t.Errorf("Expected TargetBranch to be main, got %s", result.TargetBranch)
	}
	if len(result.Results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(result.Results))
	}

	// Branch 1: Success
	if !result.Results[0].Success {
		t.Errorf("Expected branch1 to succeed, got error: %s", result.Results[0].Error)
	}

	// Branch 2: Success
	if !result.Results[1].Success {
		t.Errorf("Expected branch2 to succeed, got error: %s", result.Results[1].Error)
	}

	// Branch 3: Conflict
	if result.Results[2].Success {
		t.Errorf("Expected branch3 to fail due to conflict")
	}
	if len(result.Results[2].Conflicts) == 0 || result.Results[2].Conflicts[0] != "file.txt" {
		t.Errorf("Expected conflict in file.txt, got %v", result.Results[2].Conflicts)
	}
}
