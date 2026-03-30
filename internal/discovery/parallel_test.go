package discovery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestParallelScan_FindsSameReposAsSequential(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeRepo(t, root, "alpha", true, false)
	makeRepo(t, root, "beta", true, true)
	makeRepo(t, root, "gamma", false, true)
	// Non-ralph dir should be skipped.
	os.MkdirAll(filepath.Join(root, "plain"), 0755)

	seqRepos, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	parRepos, err := ParallelScan(context.Background(), root, 4)
	if err != nil {
		t.Fatalf("ParallelScan: %v", err)
	}

	if len(seqRepos) != len(parRepos) {
		t.Fatalf("sequential found %d repos, parallel found %d", len(seqRepos), len(parRepos))
	}

	for i := range seqRepos {
		if seqRepos[i].Name != parRepos[i].Name {
			t.Errorf("repos[%d]: sequential=%q, parallel=%q", i, seqRepos[i].Name, parRepos[i].Name)
		}
		if seqRepos[i].Path != parRepos[i].Path {
			t.Errorf("repos[%d]: sequential path=%q, parallel path=%q", i, seqRepos[i].Path, parRepos[i].Path)
		}
		if seqRepos[i].HasRalph != parRepos[i].HasRalph {
			t.Errorf("repos[%d]: sequential HasRalph=%v, parallel HasRalph=%v", i, seqRepos[i].HasRalph, parRepos[i].HasRalph)
		}
		if seqRepos[i].HasRC != parRepos[i].HasRC {
			t.Errorf("repos[%d]: sequential HasRC=%v, parallel HasRC=%v", i, seqRepos[i].HasRC, parRepos[i].HasRC)
		}
	}
}

func TestParallelScan_SortedAlphabetically(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeRepo(t, root, "zebra", true, false)
	makeRepo(t, root, "alpha", true, false)
	makeRepo(t, root, "middle", true, false)

	repos, err := ParallelScan(context.Background(), root, 2)
	if err != nil {
		t.Fatalf("ParallelScan: %v", err)
	}

	if len(repos) != 3 {
		t.Fatalf("expected 3 repos, got %d", len(repos))
	}

	expected := []string{"alpha", "middle", "zebra"}
	for i, want := range expected {
		if repos[i].Name != want {
			t.Errorf("repos[%d].Name = %q, want %q", i, repos[i].Name, want)
		}
	}
}

func TestParallelScan_WorkerCountLimitsConcurrency(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// Create enough repos that we'd see contention.
	for i := 0; i < 20; i++ {
		makeRepo(t, root, fmt.Sprintf("repo-%03d", i), true, false)
	}

	const maxWorkers = 2

	// We test this indirectly: with 2 workers, the scan should still
	// complete and find all repos. The goroutine count should stay bounded.
	before := runtime.NumGoroutine()

	repos, err := ParallelScan(context.Background(), root, maxWorkers)
	if err != nil {
		t.Fatalf("ParallelScan: %v", err)
	}

	// Goroutines should return to roughly the same count.
	// Give a moment for cleanup.
	time.Sleep(10 * time.Millisecond)
	after := runtime.NumGoroutine()

	if len(repos) != 20 {
		t.Errorf("expected 20 repos, got %d", len(repos))
	}

	// The goroutine count should not have grown by more than the worker count
	// plus a small overhead (feeder goroutine, closer goroutine).
	leaked := after - before
	if leaked > maxWorkers+5 {
		t.Errorf("goroutine leak: before=%d, after=%d, leaked=%d", before, after, leaked)
	}

}

func TestParallelScan_ContextCancellation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for i := 0; i < 30; i++ {
		makeRepo(t, root, fmt.Sprintf("repo-%03d", i), true, false)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := ParallelScan(ctx, root, 4)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestParallelScan_ContextAlreadyCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ParallelScan(ctx, "/tmp/whatever", 4)
	if err == nil {
		t.Fatal("expected error for pre-cancelled context")
	}
}

func TestParallelScan_EmptyRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repos, err := ParallelScan(context.Background(), root, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

func TestParallelScan_NonexistentRoot(t *testing.T) {
	t.Parallel()

	_, err := ParallelScan(context.Background(), "/nonexistent/path/xyzzy", 4)
	if err == nil {
		t.Fatal("expected error for nonexistent root")
	}
}

func TestParallelScan_DefaultWorkers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeRepo(t, root, "repo-a", true, false)

	// workers=0 should fall back to default (4).
	repos, err := ParallelScan(context.Background(), root, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(repos))
	}
}

func TestParallelScan_NegativeWorkers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeRepo(t, root, "repo-a", true, false)

	// workers=-1 should fall back to default (4).
	repos, err := ParallelScan(context.Background(), root, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(repos))
	}
}

func TestParallelScan_ManyRepos(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	count := 100
	for i := 0; i < count; i++ {
		makeRepo(t, root, fmt.Sprintf("repo-%04d", i), true, false)
	}

	repos, err := ParallelScan(context.Background(), root, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != count {
		t.Errorf("expected %d repos, got %d", count, len(repos))
	}

	// Verify sorted.
	for i := 1; i < len(repos); i++ {
		if repos[i].Name < repos[i-1].Name {
			t.Errorf("repos not sorted: %s before %s", repos[i-1].Name, repos[i].Name)
			break
		}
	}
}

func TestParallelScan_SkipsFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "readme.txt"), []byte("hi"), 0644)
	makeRepo(t, root, "real-repo", true, false)

	repos, err := ParallelScan(context.Background(), root, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(repos))
	}
}

func BenchmarkParallelScan(b *testing.B) {
	b.ReportAllocs()
	root := b.TempDir()

	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("repo-%03d", i)
		repoPath := filepath.Join(root, name)
		os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755)
		os.WriteFile(filepath.Join(repoPath, ".ralphrc"), []byte("PROJECT_NAME="+name+"\n"), 0644)
	}

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repos, err := ParallelScan(ctx, root, 8)
		if err != nil {
			b.Fatal(err)
		}
		if len(repos) != 50 {
			b.Fatalf("expected 50 repos, got %d", len(repos))
		}
	}
}
