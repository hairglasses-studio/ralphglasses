package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func setupChainerRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cyclesDir := filepath.Join(dir, ".ralph", "cycles")
	if err := os.MkdirAll(cyclesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func makeCompleteCycle(t *testing.T, repoPath, name string, synthesis *CycleSynthesis, findings []CycleFinding) *CycleRun {
	t.Helper()
	cycle := NewCycleRun(name, repoPath, "test objective", []string{"c1"})
	cycle.Phase = CycleComplete
	cycle.Synthesis = synthesis
	cycle.Findings = findings
	if err := SaveCycle(repoPath, cycle); err != nil {
		t.Fatal(err)
	}
	return cycle
}

func TestCycleChainer_NewlyCompleted(t *testing.T) {
	repo := setupChainerRepo(t)
	makeCompleteCycle(t, repo, "done-cycle", &CycleSynthesis{
		Remaining: []string{"fix tests", "update docs"},
	}, nil)

	cc := NewCycleChainer()
	result, err := cc.CheckAndChain(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected chain params, got nil")
	}
	if result.Objective == "" {
		t.Fatal("expected non-empty objective")
	}
	if len(result.SuccessCriteria) != 2 {
		t.Fatalf("expected 2 criteria, got %d", len(result.SuccessCriteria))
	}
}

func TestCycleChainer_AlreadySeen(t *testing.T) {
	repo := setupChainerRepo(t)
	makeCompleteCycle(t, repo, "seen-cycle", &CycleSynthesis{
		Remaining: []string{"item"},
	}, nil)

	cc := NewCycleChainer()
	// First call processes it.
	_, err := cc.CheckAndChain(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}

	// Second call should return nil (already seen).
	result, err := cc.CheckAndChain(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatal("expected nil on second call, got chain params")
	}
}

func TestCycleChainer_ChainDepthCap(t *testing.T) {
	cc := NewCycleChainer()

	// Build a chain of depth MaxChainDepth.
	ids := make([]string, MaxChainDepth+1)
	for i := range ids {
		ids[i] = NewCycleRun("x", "/tmp", "o", nil).ID
	}
	for i := 0; i < MaxChainDepth; i++ {
		cc.lineage = append(cc.lineage, CycleLineage{
			ParentID: ids[i],
			ChildID:  ids[i+1],
		})
	}

	depth := cc.ChainDepth(ids[MaxChainDepth])
	if depth != MaxChainDepth {
		t.Fatalf("expected depth %d, got %d", MaxChainDepth, depth)
	}

	// A completed cycle at max depth should not chain.
	repo := setupChainerRepo(t)
	cycle := makeCompleteCycle(t, repo, "deep", &CycleSynthesis{
		Remaining: []string{"more work"},
	}, nil)

	// Artificially set cycle.ID to the deepest in the chain.
	cc.seenCycles = make(map[string]bool) // reset
	// Replace the cycle file with the deep ID.
	os.Remove(filepath.Join(repo, ".ralph", "cycles", cycle.ID+".json"))
	cycle.ID = ids[MaxChainDepth]
	SaveCycle(repo, cycle)

	result, err := cc.CheckAndChain(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatal("expected nil when chain depth is at max")
	}
}

func TestCycleChainer_ChainFromSynthesis_WithRemaining(t *testing.T) {
	cycle := &CycleRun{
		ID: "abcdefgh-1234",
		Synthesis: &CycleSynthesis{
			Remaining: []string{"fix A", "fix B"},
		},
	}

	name, obj, criteria := ChainFromSynthesis(cycle)
	if name != "chain-abcdefgh-cont" {
		t.Fatalf("unexpected name: %s", name)
	}
	if obj == "" {
		t.Fatal("expected non-empty objective")
	}
	if len(criteria) != 2 {
		t.Fatalf("expected 2 criteria, got %d", len(criteria))
	}
}

func TestCycleChainer_ChainFromSynthesis_EmptySynthesis(t *testing.T) {
	cycle := &CycleRun{ID: "abcdefgh-1234", Synthesis: &CycleSynthesis{}}
	_, obj, _ := ChainFromSynthesis(cycle)
	if obj != "" {
		t.Fatalf("expected empty objective for empty synthesis, got %q", obj)
	}

	// Also test nil synthesis.
	cycle2 := &CycleRun{ID: "abcdefgh-5678"}
	_, obj2, _ := ChainFromSynthesis(cycle2)
	if obj2 != "" {
		t.Fatalf("expected empty objective for nil synthesis, got %q", obj2)
	}
}

func TestCycleChainer_ChainFromSynthesis_WithRegressions(t *testing.T) {
	cycle := &CycleRun{
		ID:        "abcdefgh-regr",
		Synthesis: &CycleSynthesis{Summary: "done"},
		Findings: []CycleFinding{
			{ID: "CF-1", Description: "test broke", Category: "regression", Severity: "critical"},
			{ID: "CF-2", Description: "build error", Category: "failure", Severity: "high"},
		},
	}

	name, obj, criteria := ChainFromSynthesis(cycle)
	if name != "chain-abcdefgh-fix" {
		t.Fatalf("unexpected name: %s", name)
	}
	if obj == "" {
		t.Fatal("expected non-empty objective for regressions")
	}
	if len(criteria) != 2 {
		t.Fatalf("expected 2 criteria from findings, got %d", len(criteria))
	}
}

func TestCycleChainer_LineagePersistence(t *testing.T) {
	repo := setupChainerRepo(t)

	cc := NewCycleChainer()
	if err := cc.RecordLineage(repo, "parent-1", "child-1"); err != nil {
		t.Fatal(err)
	}
	if err := cc.RecordLineage(repo, "child-1", "grandchild-1"); err != nil {
		t.Fatal(err)
	}

	// Load into a fresh chainer.
	cc2 := NewCycleChainer()
	if err := cc2.LoadLineage(repo); err != nil {
		t.Fatal(err)
	}

	if len(cc2.lineage) != 2 {
		t.Fatalf("expected 2 lineage entries, got %d", len(cc2.lineage))
	}

	depth := cc2.ChainDepth("grandchild-1")
	if depth != 2 {
		t.Fatalf("expected depth 2, got %d", depth)
	}
}

func TestCycleChainer_ChainDepth_Unchained(t *testing.T) {
	cc := NewCycleChainer()
	depth := cc.ChainDepth("some-random-id")
	if depth != 0 {
		t.Fatalf("expected depth 0 for unchained cycle, got %d", depth)
	}
}
