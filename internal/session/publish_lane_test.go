package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishLanePlanner_Plan(t *testing.T) {
	dir := t.TempDir()

	gitInit := func(args ...string) {
		cmd := gitRun
		if err := cmd(dir, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	gitInit("init")
	gitInit("commit", "--allow-empty", "-m", "initial")

	planner := NewPublishLanePlanner(dir, "main")
	ctx := context.Background()

	// Clean repo
	lane, err := planner.Plan(ctx)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	if lane != LaneDirectPush {
		t.Errorf("Expected direct_push, got %v", lane)
	}

	// Dirty repo
	err = os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("dirty"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	lane, err = planner.Plan(ctx)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	if lane != LaneWorktreePush {
		t.Errorf("Expected worktree_push, got %v", lane)
	}
}
