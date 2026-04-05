package session

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Note: initGitRepo helper is defined in checkpoint_test.go and reused here.

func TestWorktreePool_NewDefaults(t *testing.T) {
	t.Parallel()

	pool := NewWorktreePool(0)
	if pool.PoolSize() != DefaultPoolSize {
		t.Errorf("expected default pool size %d, got %d", DefaultPoolSize, pool.PoolSize())
	}
	pool.Close()

	pool2 := NewWorktreePool(8)
	if pool2.PoolSize() != 8 {
		t.Errorf("expected pool size 8, got %d", pool2.PoolSize())
	}
	pool2.Close()
}

func TestWorktreePool_AcquireCreatesWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	t.Parallel()

	repo := t.TempDir()
	initGitRepo(t, repo)

	pool := NewWorktreePool(4)
	defer pool.Close()

	ctx := context.Background()
	path, branch, err := pool.Acquire(ctx, repo)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	if path == "" {
		t.Fatal("Acquire returned empty path")
	}
	if branch == "" {
		t.Fatal("Acquire returned empty branch")
	}
	if !strings.Contains(path, ".ralph/worktrees/pool/") {
		t.Errorf("expected path under .ralph/worktrees/pool/, got %s", path)
	}

	// Verify the worktree is a valid git directory.
	cmd := exec.Command("git", "-C", path, "status")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git status in acquired worktree failed: %v\n%s", err, out)
	}
}

func TestWorktreePool_AcquireAndRelease(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	t.Parallel()

	repo := t.TempDir()
	initGitRepo(t, repo)

	pool := NewWorktreePool(4)
	defer pool.Close()

	ctx := context.Background()

	// Acquire a worktree.
	path1, _, err := pool.Acquire(ctx, repo)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	// Dirty the worktree: create a file.
	dirtyFile := filepath.Join(path1, "dirty.txt")
	if err := os.WriteFile(dirtyFile, []byte("dirty"), 0644); err != nil {
		t.Fatal(err)
	}

	// Release it back.
	pool.Release(ctx, repo, path1)

	// Verify the dirty file was cleaned.
	if _, err := os.Stat(dirtyFile); !os.IsNotExist(err) {
		t.Error("expected dirty file to be cleaned after release")
	}

	// Acquire again -- should get the same path back (LIFO).
	path2, _, err := pool.Acquire(ctx, repo)
	if err != nil {
		t.Fatalf("second Acquire failed: %v", err)
	}
	if path2 != path1 {
		t.Errorf("expected reuse of released worktree %s, got %s", path1, path2)
	}
}

func TestWorktreePool_ReleaseDestroyWhenFull(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	t.Parallel()

	repo := t.TempDir()
	initGitRepo(t, repo)

	pool := NewWorktreePool(1) // tiny pool
	defer pool.Close()

	ctx := context.Background()

	// Acquire two worktrees.
	path1, _, err := pool.Acquire(ctx, repo)
	if err != nil {
		t.Fatalf("Acquire 1 failed: %v", err)
	}
	path2, _, err := pool.Acquire(ctx, repo)
	if err != nil {
		t.Fatalf("Acquire 2 failed: %v", err)
	}

	// Release both. The second should be destroyed since pool size is 1.
	pool.Release(ctx, repo, path1)
	pool.Release(ctx, repo, path2)

	stats := pool.Stats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 repo in stats, got %d", len(stats))
	}
	if stats[0].IdleCount != 1 {
		t.Errorf("expected 1 idle worktree (pool size 1), got %d", stats[0].IdleCount)
	}

	// The first released path should still exist (pooled).
	if _, err := os.Stat(path1); err != nil {
		t.Errorf("first released worktree should still exist: %v", err)
	}
	// The second released path should be destroyed.
	if _, err := os.Stat(path2); !os.IsNotExist(err) {
		t.Error("second released worktree should have been destroyed (pool full)")
	}
}

func TestWorktreePool_Warm(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	t.Parallel()

	repo := t.TempDir()
	initGitRepo(t, repo)

	pool := NewWorktreePool(4)
	defer pool.Close()

	ctx := context.Background()
	if err := pool.Warm(ctx, repo, 3); err != nil {
		t.Fatalf("Warm failed: %v", err)
	}

	stats := pool.Stats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 repo in stats, got %d", len(stats))
	}
	if stats[0].IdleCount != 3 {
		t.Errorf("expected 3 idle after warming, got %d", stats[0].IdleCount)
	}

	// Warming beyond pool size should cap at poolSize.
	if err := pool.Warm(ctx, repo, 10); err != nil {
		t.Fatalf("Warm beyond cap failed: %v", err)
	}
	stats = pool.Stats()
	if stats[0].IdleCount != 4 {
		t.Errorf("expected 4 idle (capped at pool size), got %d", stats[0].IdleCount)
	}
}

func TestWorktreePool_WarmThenAcquire(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	t.Parallel()

	repo := t.TempDir()
	initGitRepo(t, repo)

	pool := NewWorktreePool(4)
	defer pool.Close()

	ctx := context.Background()
	if err := pool.Warm(ctx, repo, 2); err != nil {
		t.Fatalf("Warm failed: %v", err)
	}

	// Acquire should return a pre-warmed worktree.
	path, _, err := pool.Acquire(ctx, repo)
	if err != nil {
		t.Fatalf("Acquire from warmed pool failed: %v", err)
	}
	if !strings.Contains(path, ".ralph/worktrees/pool/") {
		t.Errorf("expected pooled worktree path, got %s", path)
	}

	// Should now be 1 idle.
	stats := pool.Stats()
	if stats[0].IdleCount != 1 {
		t.Errorf("expected 1 idle after 1 acquire from 2 warmed, got %d", stats[0].IdleCount)
	}
}

func TestWorktreePool_Close(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	t.Parallel()

	repo := t.TempDir()
	initGitRepo(t, repo)

	pool := NewWorktreePool(4)
	ctx := context.Background()

	if err := pool.Warm(ctx, repo, 2); err != nil {
		t.Fatalf("Warm failed: %v", err)
	}

	// Record paths before close.
	stats := pool.Stats()
	if len(stats) == 0 || stats[0].IdleCount != 2 {
		t.Fatal("expected 2 warmed entries before close")
	}

	pool.Close()

	// After close, Acquire should fail.
	_, _, err := pool.Acquire(ctx, repo)
	if err == nil {
		t.Error("expected error from Acquire after Close")
	}

	// After close, Warm should fail.
	err = pool.Warm(ctx, repo, 1)
	if err == nil {
		t.Error("expected error from Warm after Close")
	}

	// Stats should be empty.
	stats = pool.Stats()
	if len(stats) != 0 {
		t.Errorf("expected 0 repos in stats after Close, got %d", len(stats))
	}
}

func TestWorktreePool_CloseIdempotent(t *testing.T) {
	t.Parallel()
	pool := NewWorktreePool(4)
	pool.Close()
	pool.Close() // should not panic
}

func TestWorktreePool_ReleaseEmptyPath(t *testing.T) {
	t.Parallel()
	pool := NewWorktreePool(4)
	defer pool.Close()
	// Should not panic or error.
	pool.Release(context.Background(), "/some/repo", "")
}

func TestWorktreePool_ConcurrentAcquireRelease(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	t.Parallel()

	repo := t.TempDir()
	initGitRepo(t, repo)

	pool := NewWorktreePool(4)
	defer pool.Close()

	ctx := context.Background()

	// Warm the pool.
	if err := pool.Warm(ctx, repo, 4); err != nil {
		t.Fatalf("Warm failed: %v", err)
	}

	// Run concurrent acquire/release cycles.
	const goroutines = 8
	const iterations = 3
	var wg sync.WaitGroup

	for range goroutines {
		wg.Go(func() {
			for range iterations {
				path, _, err := pool.Acquire(ctx, repo)
				if err != nil {
					t.Errorf("concurrent Acquire failed: %v", err)
					return
				}
				// Simulate some work.
				_ = os.WriteFile(filepath.Join(path, "work.txt"), []byte("work"), 0644)
				pool.Release(ctx, repo, path)
			}
		})
	}

	wg.Wait()

	// All worktrees should be accounted for in stats.
	stats := pool.Stats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 repo in stats, got %d", len(stats))
	}
	// Idle count should be <= pool size.
	if stats[0].IdleCount > pool.PoolSize() {
		t.Errorf("idle count %d exceeds pool size %d", stats[0].IdleCount, pool.PoolSize())
	}
}

func TestWorktreePool_AcquireCleansReleasedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	t.Parallel()

	repo := t.TempDir()
	initGitRepo(t, repo)

	pool := NewWorktreePool(4)
	defer pool.Close()

	ctx := context.Background()

	// Acquire, create tracked and untracked files, release, re-acquire.
	path, _, err := pool.Acquire(ctx, repo)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	// Create untracked file.
	untrackedFile := filepath.Join(path, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("junk"), 0644); err != nil {
		t.Fatal(err)
	}

	// Modify tracked file.
	trackedFile := filepath.Join(path, "README.md")
	if err := os.WriteFile(trackedFile, []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}

	pool.Release(ctx, repo, path)

	// Re-acquire.
	path2, _, err := pool.Acquire(ctx, repo)
	if err != nil {
		t.Fatalf("re-Acquire failed: %v", err)
	}

	// Untracked file should be gone.
	if _, err := os.Stat(filepath.Join(path2, "untracked.txt")); !os.IsNotExist(err) {
		t.Error("untracked file should have been cleaned")
	}

	// Tracked file should be restored.
	data, err := os.ReadFile(filepath.Join(path2, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# test" {
		t.Errorf("tracked file not restored, got %q", string(data))
	}
}

func TestWorktreePool_Stats(t *testing.T) {
	t.Parallel()

	pool := NewWorktreePool(4)
	defer pool.Close()

	stats := pool.Stats()
	if len(stats) != 0 {
		t.Errorf("expected 0 stats for fresh pool, got %d", len(stats))
	}
}

func TestWorktreePool_MultipleRepos(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	t.Parallel()

	repo1 := t.TempDir()
	repo2 := t.TempDir()
	initGitRepo(t, repo1)
	initGitRepo(t, repo2)

	pool := NewWorktreePool(4)
	defer pool.Close()

	ctx := context.Background()

	// Warm both repos.
	if err := pool.Warm(ctx, repo1, 2); err != nil {
		t.Fatalf("Warm repo1 failed: %v", err)
	}
	if err := pool.Warm(ctx, repo2, 1); err != nil {
		t.Fatalf("Warm repo2 failed: %v", err)
	}

	stats := pool.Stats()
	if len(stats) != 2 {
		t.Fatalf("expected 2 repos in stats, got %d", len(stats))
	}

	// Each repo should have its own count.
	found := map[int]bool{}
	for _, s := range stats {
		found[s.IdleCount] = true
	}
	if !found[2] || !found[1] {
		t.Errorf("expected idle counts 2 and 1, got stats: %+v", stats)
	}
}
