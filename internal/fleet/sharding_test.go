package fleet

import (
	"fmt"
	"math"
	"testing"
)

func makeWorkers(ids ...string) []WorkerInfo {
	workers := make([]WorkerInfo, len(ids))
	for i, id := range ids {
		workers[i] = WorkerInfo{
			ID:       id,
			Hostname: "host-" + id,
			Status:   WorkerOnline,
		}
	}
	return workers
}

// --- HashShardStrategy tests ---

func TestHashShardStrategy_Consistent(t *testing.T) {
	// Same repo always maps to same worker given a stable worker set.
	strategy := &HashShardStrategy{Replicas: 64}
	workers := makeWorkers("w1", "w2", "w3", "w4")

	repo := "/home/dev/hairglasses-studio/ralphglasses"
	first := strategy.Assign(repo, workers)
	if first == "" {
		t.Fatal("expected non-empty assignment")
	}

	// Verify 100 calls return the same result.
	for i := 0; i < 100; i++ {
		got := strategy.Assign(repo, workers)
		if got != first {
			t.Fatalf("iteration %d: got %q, want %q", i, got, first)
		}
	}
}

func TestHashShardStrategy_EvenDistribution(t *testing.T) {
	// 5000 repos across 5 workers with realistic worker IDs should be roughly even.
	strategy := &HashShardStrategy{Replicas: 256}
	workers := makeWorkers(
		"node-alpha-01", "node-beta-02", "node-gamma-03",
		"node-delta-04", "node-epsilon-05",
	)

	counts := make(map[string]int)
	numRepos := 5000
	for i := 0; i < numRepos; i++ {
		repo := fmt.Sprintf("/home/dev/repos/project-%05d", i)
		wid := strategy.Assign(repo, workers)
		counts[wid]++
	}

	expected := float64(numRepos) / float64(len(workers))
	for wid, count := range counts {
		deviation := math.Abs(float64(count)-expected) / expected
		t.Logf("worker %s: %d repos (%.1f%% deviation)", wid, count, deviation*100)
		// Consistent hashing with 256 replicas should keep deviation under 40%.
		if deviation > 0.40 {
			t.Errorf("worker %s: got %d repos (%.1f%% deviation from expected %.0f)",
				wid, count, deviation*100, expected)
		}
	}

	// Verify all workers got at least some work.
	if len(counts) != len(workers) {
		t.Errorf("expected all %d workers to receive repos, got %d", len(workers), len(counts))
	}
}

func TestHashShardStrategy_StableOnWorkerAdd(t *testing.T) {
	// Adding a worker should only move a fraction of repos, not all of them.
	strategy := &HashShardStrategy{Replicas: 64}
	originalWorkers := makeWorkers("w1", "w2", "w3")

	repos := make([]string, 200)
	originalAssignment := make(map[string]string)
	for i := range repos {
		repos[i] = fmt.Sprintf("/repos/repo-%03d", i)
		originalAssignment[repos[i]] = strategy.Assign(repos[i], originalWorkers)
	}

	// Add a fourth worker.
	expandedWorkers := makeWorkers("w1", "w2", "w3", "w4")
	moved := 0
	for _, repo := range repos {
		newWorker := strategy.Assign(repo, expandedWorkers)
		if newWorker != originalAssignment[repo] {
			moved++
		}
	}

	// In an ideal consistent hash, adding 1 worker to 3 should move ~25% of keys.
	// Allow up to 50% as acceptable.
	movedPct := float64(moved) / float64(len(repos))
	if movedPct > 0.50 {
		t.Errorf("too many repos moved on worker add: %d/%d (%.1f%%)",
			moved, len(repos), movedPct*100)
	}
	t.Logf("worker add: %d/%d repos moved (%.1f%%)", moved, len(repos), movedPct*100)
}

func TestHashShardStrategy_StableOnWorkerRemove(t *testing.T) {
	// Removing a worker should only move repos that were on that worker.
	strategy := &HashShardStrategy{Replicas: 64}
	originalWorkers := makeWorkers("w1", "w2", "w3", "w4")

	repos := make([]string, 200)
	originalAssignment := make(map[string]string)
	for i := range repos {
		repos[i] = fmt.Sprintf("/repos/repo-%03d", i)
		originalAssignment[repos[i]] = strategy.Assign(repos[i], originalWorkers)
	}

	// Remove w3.
	reducedWorkers := makeWorkers("w1", "w2", "w4")
	movedFromOther := 0
	for _, repo := range repos {
		newWorker := strategy.Assign(repo, reducedWorkers)
		old := originalAssignment[repo]
		if old != "w3" && newWorker != old {
			movedFromOther++
		}
	}

	// Repos NOT on w3 should not move.
	if movedFromOther > 0 {
		t.Errorf("expected 0 repos to move from non-removed workers, got %d", movedFromOther)
	}
}

func TestHashShardStrategy_EmptyWorkers(t *testing.T) {
	strategy := &HashShardStrategy{}
	got := strategy.Assign("/repos/foo", nil)
	if got != "" {
		t.Errorf("expected empty string for no workers, got %q", got)
	}
}

func TestHashShardStrategy_SingleWorker(t *testing.T) {
	strategy := &HashShardStrategy{}
	workers := makeWorkers("solo")
	got := strategy.Assign("/repos/any", workers)
	if got != "solo" {
		t.Errorf("expected 'solo', got %q", got)
	}
}

func TestHashShardStrategy_DefaultReplicas(t *testing.T) {
	// Replicas=0 should default to 64 and still work.
	strategy := &HashShardStrategy{Replicas: 0}
	workers := makeWorkers("w1", "w2")
	got := strategy.Assign("/repos/test", workers)
	if got == "" {
		t.Error("expected non-empty assignment with default replicas")
	}
}

// --- ExplicitShardStrategy tests ---

func TestExplicitShardStrategy_BasicMatch(t *testing.T) {
	strategy := &ExplicitShardStrategy{
		Rules: []ShardRule{
			{Pattern: "ralphglasses", WorkerID: "w1"},
			{Pattern: "mcpkit", WorkerID: "w2"},
		},
	}
	workers := makeWorkers("w1", "w2", "w3")

	tests := []struct {
		repo string
		want string
	}{
		{"/home/dev/hairglasses-studio/ralphglasses", "w1"},
		{"/home/dev/hairglasses-studio/mcpkit", "w2"},
		{"/home/dev/hairglasses-studio/unknown", ""}, // no match, no fallback
	}

	for _, tt := range tests {
		got := strategy.Assign(tt.repo, workers)
		if got != tt.want {
			t.Errorf("Assign(%q) = %q, want %q", tt.repo, got, tt.want)
		}
	}
}

func TestExplicitShardStrategy_GlobPatterns(t *testing.T) {
	strategy := &ExplicitShardStrategy{
		Rules: []ShardRule{
			{Pattern: "ralph*", WorkerID: "w1"},     // matches ralphglasses
			{Pattern: "hg-*", WorkerID: "w2"},       // matches hg-mcp, hg-tools
		},
	}
	workers := makeWorkers("w1", "w2")

	tests := []struct {
		repo string
		want string
	}{
		{"/repos/ralphglasses", "w1"},
		{"/repos/ralph-tools", "w1"},
		{"/repos/hg-mcp", "w2"},
		{"/repos/hg-tools", "w2"},
		{"/repos/unmatched", ""},
	}

	for _, tt := range tests {
		got := strategy.Assign(tt.repo, workers)
		if got != tt.want {
			t.Errorf("Assign(%q) = %q, want %q", tt.repo, got, tt.want)
		}
	}
}

func TestExplicitShardStrategy_Fallback(t *testing.T) {
	hash := &HashShardStrategy{Replicas: 64}
	strategy := &ExplicitShardStrategy{
		Rules: []ShardRule{
			{Pattern: "ralphglasses", WorkerID: "w1"},
		},
		Fallback: hash,
	}
	workers := makeWorkers("w1", "w2", "w3")

	// ralphglasses should go to w1 via explicit rule.
	got := strategy.Assign("/repos/ralphglasses", workers)
	if got != "w1" {
		t.Errorf("explicit match: got %q, want w1", got)
	}

	// Unknown repo should fall through to hash strategy (non-empty).
	got = strategy.Assign("/repos/unknown-project", workers)
	if got == "" {
		t.Error("fallback should have assigned the repo to some worker")
	}
}

func TestExplicitShardStrategy_InvalidWorkerSkipped(t *testing.T) {
	// Rule references a worker not in the active set — should skip it.
	strategy := &ExplicitShardStrategy{
		Rules: []ShardRule{
			{Pattern: "ralphglasses", WorkerID: "gone"},
			{Pattern: "ralphglasses", WorkerID: "w1"},
		},
	}
	workers := makeWorkers("w1", "w2")

	got := strategy.Assign("/repos/ralphglasses", workers)
	if got != "w1" {
		t.Errorf("expected w1 (skipping invalid 'gone'), got %q", got)
	}
}

func TestExplicitShardStrategy_FirstMatchWins(t *testing.T) {
	strategy := &ExplicitShardStrategy{
		Rules: []ShardRule{
			{Pattern: "ralph*", WorkerID: "w1"},
			{Pattern: "ralphglasses", WorkerID: "w2"}, // more specific but later
		},
	}
	workers := makeWorkers("w1", "w2")

	got := strategy.Assign("/repos/ralphglasses", workers)
	if got != "w1" {
		t.Errorf("expected first match w1, got %q", got)
	}
}

// --- ShardMap tests ---

func TestShardMap_AssignAndLookup(t *testing.T) {
	sm := NewShardMap(nil)
	sm.Assign("/repos/a", "w1")
	sm.Assign("/repos/b", "w2")
	sm.Assign("/repos/c", "w1")

	wid, ok := sm.WorkerFor("/repos/a")
	if !ok || wid != "w1" {
		t.Errorf("WorkerFor(/repos/a) = %q, %v; want w1, true", wid, ok)
	}

	wid, ok = sm.WorkerFor("/repos/b")
	if !ok || wid != "w2" {
		t.Errorf("WorkerFor(/repos/b) = %q, %v; want w2, true", wid, ok)
	}

	_, ok = sm.WorkerFor("/repos/missing")
	if ok {
		t.Error("expected false for unassigned repo")
	}
}

func TestShardMap_ReposFor(t *testing.T) {
	sm := NewShardMap(nil)
	sm.Assign("/repos/a", "w1")
	sm.Assign("/repos/c", "w1")
	sm.Assign("/repos/b", "w1")
	sm.Assign("/repos/d", "w2")

	repos := sm.ReposFor("w1")
	if len(repos) != 3 {
		t.Fatalf("expected 3 repos for w1, got %d", len(repos))
	}
	// Should be sorted.
	want := []string{"/repos/a", "/repos/b", "/repos/c"}
	for i, r := range repos {
		if r != want[i] {
			t.Errorf("repos[%d] = %q, want %q", i, r, want[i])
		}
	}

	repos = sm.ReposFor("w2")
	if len(repos) != 1 || repos[0] != "/repos/d" {
		t.Errorf("unexpected repos for w2: %v", repos)
	}

	repos = sm.ReposFor("w99")
	if repos != nil {
		t.Errorf("expected nil for unknown worker, got %v", repos)
	}
}

func TestShardMap_Reassign(t *testing.T) {
	sm := NewShardMap(nil)
	sm.Assign("/repos/a", "w1")

	// Reassign to a different worker.
	sm.Assign("/repos/a", "w2")

	wid, _ := sm.WorkerFor("/repos/a")
	if wid != "w2" {
		t.Errorf("expected w2 after reassign, got %q", wid)
	}

	// w1 should no longer have the repo.
	repos := sm.ReposFor("w1")
	if len(repos) != 0 {
		t.Errorf("expected 0 repos for w1 after reassign, got %v", repos)
	}

	// w2 should have it.
	repos = sm.ReposFor("w2")
	if len(repos) != 1 || repos[0] != "/repos/a" {
		t.Errorf("expected [/repos/a] for w2, got %v", repos)
	}
}

func TestShardMap_Unassign(t *testing.T) {
	sm := NewShardMap(nil)
	sm.Assign("/repos/a", "w1")
	sm.Assign("/repos/b", "w1")
	sm.Unassign("/repos/a")

	_, ok := sm.WorkerFor("/repos/a")
	if ok {
		t.Error("expected repo to be unassigned")
	}

	repos := sm.ReposFor("w1")
	if len(repos) != 1 || repos[0] != "/repos/b" {
		t.Errorf("expected [/repos/b], got %v", repos)
	}
}

func TestShardMap_Len(t *testing.T) {
	sm := NewShardMap(nil)
	if sm.Len() != 0 {
		t.Errorf("expected 0, got %d", sm.Len())
	}
	sm.Assign("/repos/a", "w1")
	sm.Assign("/repos/b", "w2")
	if sm.Len() != 2 {
		t.Errorf("expected 2, got %d", sm.Len())
	}
}

func TestShardMap_RebalanceOnWorkerAdd(t *testing.T) {
	strategy := &HashShardStrategy{Replicas: 64}
	sm := NewShardMap(strategy)

	// Assign 50 repos to 3 workers.
	workers3 := makeWorkers("w1", "w2", "w3")
	for i := 0; i < 50; i++ {
		repo := fmt.Sprintf("/repos/project-%03d", i)
		wid := strategy.Assign(repo, workers3)
		sm.Assign(repo, wid)
	}

	before := sm.AllAssignments()
	if len(before) != 50 {
		t.Fatalf("expected 50 assignments, got %d", len(before))
	}

	// Add w4 and rebalance.
	workers4 := makeWorkers("w1", "w2", "w3", "w4")
	sm.Rebalance(workers4)

	after := sm.AllAssignments()
	if len(after) != 50 {
		t.Fatalf("expected 50 assignments after rebalance, got %d", len(after))
	}

	// w4 should have some repos now.
	w4repos := sm.ReposFor("w4")
	if len(w4repos) == 0 {
		t.Error("expected w4 to have repos after rebalance")
	}
	t.Logf("after adding w4: w4 got %d/%d repos", len(w4repos), len(after))
}

func TestShardMap_RebalanceOnWorkerRemove(t *testing.T) {
	strategy := &HashShardStrategy{Replicas: 64}
	sm := NewShardMap(strategy)

	// Assign 50 repos to 4 workers.
	workers4 := makeWorkers("w1", "w2", "w3", "w4")
	for i := 0; i < 50; i++ {
		repo := fmt.Sprintf("/repos/project-%03d", i)
		wid := strategy.Assign(repo, workers4)
		sm.Assign(repo, wid)
	}

	// Record how many repos w3 had.
	w3before := sm.ReposFor("w3")

	// Remove w3 and rebalance.
	workers3 := makeWorkers("w1", "w2", "w4")
	sm.Rebalance(workers3)

	after := sm.AllAssignments()
	if len(after) != 50 {
		t.Fatalf("expected 50 assignments after rebalance, got %d", len(after))
	}

	// w3 should have no repos.
	w3after := sm.ReposFor("w3")
	if len(w3after) != 0 {
		t.Errorf("expected 0 repos for removed w3, got %d", len(w3after))
	}

	// All w3's repos should have been redistributed.
	for _, repo := range w3before {
		wid, ok := sm.WorkerFor(repo)
		if !ok {
			t.Errorf("repo %s lost after rebalance", repo)
			continue
		}
		if wid == "w3" {
			t.Errorf("repo %s still assigned to removed worker w3", repo)
		}
	}
}

func TestShardMap_AllAssignments(t *testing.T) {
	sm := NewShardMap(nil)
	sm.Assign("/repos/a", "w1")
	sm.Assign("/repos/b", "w2")

	all := sm.AllAssignments()
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
	if all["/repos/a"] != "w1" || all["/repos/b"] != "w2" {
		t.Errorf("unexpected assignments: %v", all)
	}

	// Mutating the returned map should not affect the ShardMap.
	all["/repos/c"] = "w3"
	if sm.Len() != 2 {
		t.Error("mutation of AllAssignments should not affect ShardMap")
	}
}

func TestShardMap_WorkerRepoCount(t *testing.T) {
	sm := NewShardMap(nil)
	sm.Assign("/repos/a", "w1")
	sm.Assign("/repos/b", "w1")
	sm.Assign("/repos/c", "w2")

	counts := sm.WorkerRepoCount()
	if counts["w1"] != 2 || counts["w2"] != 1 {
		t.Errorf("unexpected counts: %v", counts)
	}
}

func TestShardMap_RebalanceEmptyWorkers(t *testing.T) {
	strategy := &HashShardStrategy{Replicas: 64}
	sm := NewShardMap(strategy)
	sm.Assign("/repos/a", "w1")
	sm.Assign("/repos/b", "w2")

	// Rebalance with no workers should clear all assignments.
	sm.Rebalance(nil)
	if sm.Len() != 0 {
		t.Errorf("expected 0 assignments after rebalance with no workers, got %d", sm.Len())
	}
}

func TestShardMap_NilStrategy(t *testing.T) {
	sm := NewShardMap(nil)
	sm.Assign("/repos/a", "w1")

	// Rebalance with nil strategy should be a no-op.
	sm.Rebalance(makeWorkers("w1", "w2"))
	wid, ok := sm.WorkerFor("/repos/a")
	if !ok || wid != "w1" {
		t.Errorf("nil strategy rebalance should be no-op, got %q, %v", wid, ok)
	}
}

// --- RepoPathNormalize tests ---

func TestRepoPathNormalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/home/dev/repos/ralphglasses", "/home/dev/repos/ralphglasses"},
		{"/home/dev/repos/../repos/ralphglasses", "/home/dev/repos/ralphglasses"},
		{"/home/dev/repos/./ralphglasses", "/home/dev/repos/ralphglasses"},
	}

	for _, tt := range tests {
		got := RepoPathNormalize(tt.input)
		if got != tt.want {
			t.Errorf("RepoPathNormalize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- ketamaPoints helper test ---

func TestKetamaPoints_Unique(t *testing.T) {
	// Each virtual node point for a worker should be unique.
	points := ketamaPoints("worker-1", 256)
	if len(points) != 256 {
		t.Fatalf("expected 256 points, got %d", len(points))
	}
	seen := make(map[uint32]bool)
	for i, p := range points {
		if seen[p] {
			t.Errorf("point %d produced duplicate hash %d", i, p)
		}
		seen[p] = true
	}
}
