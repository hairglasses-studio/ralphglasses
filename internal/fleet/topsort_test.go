package fleet

import (
	"testing"
)

func TestSort_LinearChain(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "a", Name: "build", Priority: 1})
	g.AddNode(TaskNode{ID: "b", Name: "test", Priority: 1})
	g.AddNode(TaskNode{ID: "c", Name: "deploy", Priority: 1})

	if err := g.AddDependency("b", "a"); err != nil {
		t.Fatal(err)
	}
	if err := g.AddDependency("c", "b"); err != nil {
		t.Fatal(err)
	}

	sorted, err := g.Sort()
	if err != nil {
		t.Fatalf("Sort: %v", err)
	}

	ids := nodeIDs(sorted)
	// a must come before b, b before c.
	if idx(ids, "a") >= idx(ids, "b") || idx(ids, "b") >= idx(ids, "c") {
		t.Errorf("expected a < b < c, got %v", ids)
	}
}

func TestSort_DiamondDependency(t *testing.T) {
	t.Parallel()
	// A -> B, A -> C, B -> D, C -> D
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "a", Name: "init", Priority: 1})
	g.AddNode(TaskNode{ID: "b", Name: "left", Priority: 2})
	g.AddNode(TaskNode{ID: "c", Name: "right", Priority: 1})
	g.AddNode(TaskNode{ID: "d", Name: "merge", Priority: 1})

	must(t, g.AddDependency("b", "a"))
	must(t, g.AddDependency("c", "a"))
	must(t, g.AddDependency("d", "b"))
	must(t, g.AddDependency("d", "c"))

	sorted, err := g.Sort()
	if err != nil {
		t.Fatalf("Sort: %v", err)
	}

	ids := nodeIDs(sorted)
	if idx(ids, "a") >= idx(ids, "b") {
		t.Errorf("a must precede b, got %v", ids)
	}
	if idx(ids, "a") >= idx(ids, "c") {
		t.Errorf("a must precede c, got %v", ids)
	}
	if idx(ids, "b") >= idx(ids, "d") {
		t.Errorf("b must precede d, got %v", ids)
	}
	if idx(ids, "c") >= idx(ids, "d") {
		t.Errorf("c must precede d, got %v", ids)
	}
}

func TestSort_CycleDetection(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "a", Name: "a"})
	g.AddNode(TaskNode{ID: "b", Name: "b"})
	g.AddNode(TaskNode{ID: "c", Name: "c"})

	must(t, g.AddDependency("a", "b"))
	must(t, g.AddDependency("b", "c"))
	must(t, g.AddDependency("c", "a"))

	_, err := g.Sort()
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}

	cycles := g.DetectCycles()
	if len(cycles) == 0 {
		t.Fatal("DetectCycles returned no cycles for cyclic graph")
	}
}

func TestSort_IndependentTasks(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "a", Name: "a", Priority: 3})
	g.AddNode(TaskNode{ID: "b", Name: "b", Priority: 1})
	g.AddNode(TaskNode{ID: "c", Name: "c", Priority: 2})

	sorted, err := g.Sort()
	if err != nil {
		t.Fatalf("Sort: %v", err)
	}
	if len(sorted) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(sorted))
	}

	// With no dependencies, all should be sortable, priority-ordered.
	if sorted[0].ID != "a" {
		t.Errorf("expected highest priority first (a), got %s", sorted[0].ID)
	}
}

func TestSort_EmptyGraph(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	sorted, err := g.Sort()
	if err != nil {
		t.Fatalf("Sort on empty graph: %v", err)
	}
	if len(sorted) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(sorted))
	}
}

func TestSort_SingleNode(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "solo", Name: "only-one", Priority: 5})

	sorted, err := g.Sort()
	if err != nil {
		t.Fatalf("Sort: %v", err)
	}
	if len(sorted) != 1 || sorted[0].ID != "solo" {
		t.Fatalf("expected [solo], got %v", nodeIDs(sorted))
	}
}

func TestCriticalPath_LinearChain(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "a", Name: "a"})
	g.AddNode(TaskNode{ID: "b", Name: "b"})
	g.AddNode(TaskNode{ID: "c", Name: "c"})
	must(t, g.AddDependency("b", "a"))
	must(t, g.AddDependency("c", "b"))

	path := g.CriticalPath()
	ids := nodeIDs(path)
	if len(ids) != 3 {
		t.Fatalf("expected critical path length 3, got %d: %v", len(ids), ids)
	}
	if ids[0] != "a" || ids[1] != "b" || ids[2] != "c" {
		t.Errorf("expected [a b c], got %v", ids)
	}
}

func TestCriticalPath_Diamond(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "a", Name: "a"})
	g.AddNode(TaskNode{ID: "b", Name: "b"})
	g.AddNode(TaskNode{ID: "c", Name: "c"})
	g.AddNode(TaskNode{ID: "d", Name: "d"})
	must(t, g.AddDependency("b", "a"))
	must(t, g.AddDependency("c", "a"))
	must(t, g.AddDependency("d", "b"))
	must(t, g.AddDependency("d", "c"))

	path := g.CriticalPath()
	// Critical path should be length 3 (a -> b|c -> d).
	if len(path) != 3 {
		t.Fatalf("expected critical path length 3, got %d: %v", len(path), nodeIDs(path))
	}
	if path[0].ID != "a" || path[len(path)-1].ID != "d" {
		t.Errorf("expected path from a to d, got %v", nodeIDs(path))
	}
}

func TestCriticalPath_EmptyGraph(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	path := g.CriticalPath()
	if path != nil {
		t.Errorf("expected nil critical path for empty graph, got %v", path)
	}
}

func TestReadyTasks_PartialCompletion(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "a", Name: "a", Priority: 1})
	g.AddNode(TaskNode{ID: "b", Name: "b", Priority: 2})
	g.AddNode(TaskNode{ID: "c", Name: "c", Priority: 3})
	must(t, g.AddDependency("b", "a"))
	must(t, g.AddDependency("c", "b"))

	// Nothing completed: only a is ready.
	ready := g.ReadyTasks(map[string]bool{})
	ids := nodeIDs(ready)
	if len(ids) != 1 || ids[0] != "a" {
		t.Errorf("expected [a], got %v", ids)
	}

	// a completed: b is ready.
	ready = g.ReadyTasks(map[string]bool{"a": true})
	ids = nodeIDs(ready)
	if len(ids) != 1 || ids[0] != "b" {
		t.Errorf("expected [b], got %v", ids)
	}

	// a and b completed: c is ready.
	ready = g.ReadyTasks(map[string]bool{"a": true, "b": true})
	ids = nodeIDs(ready)
	if len(ids) != 1 || ids[0] != "c" {
		t.Errorf("expected [c], got %v", ids)
	}

	// All completed: nothing is ready.
	ready = g.ReadyTasks(map[string]bool{"a": true, "b": true, "c": true})
	if len(ready) != 0 {
		t.Errorf("expected empty, got %v", nodeIDs(ready))
	}
}

func TestReadyTasks_Diamond(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "a", Name: "a", Priority: 1})
	g.AddNode(TaskNode{ID: "b", Name: "b", Priority: 2})
	g.AddNode(TaskNode{ID: "c", Name: "c", Priority: 3})
	g.AddNode(TaskNode{ID: "d", Name: "d", Priority: 4})
	must(t, g.AddDependency("b", "a"))
	must(t, g.AddDependency("c", "a"))
	must(t, g.AddDependency("d", "b"))
	must(t, g.AddDependency("d", "c"))

	// a completed: b and c are ready.
	ready := g.ReadyTasks(map[string]bool{"a": true})
	ids := nodeIDs(ready)
	if len(ids) != 2 {
		t.Fatalf("expected 2 ready tasks, got %v", ids)
	}
	// Higher priority first.
	if ids[0] != "c" || ids[1] != "b" {
		t.Errorf("expected [c b] (by priority), got %v", ids)
	}

	// Only b completed (not c): d is NOT ready.
	ready = g.ReadyTasks(map[string]bool{"a": true, "b": true})
	ids = nodeIDs(ready)
	// c should be ready, d should not.
	if len(ids) != 1 || ids[0] != "c" {
		t.Errorf("expected [c], got %v", ids)
	}

	// Both b and c completed: d is ready.
	ready = g.ReadyTasks(map[string]bool{"a": true, "b": true, "c": true})
	ids = nodeIDs(ready)
	if len(ids) != 1 || ids[0] != "d" {
		t.Errorf("expected [d], got %v", ids)
	}
}

func TestBuildSchedule_LinearChain(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "a", Name: "build"})
	g.AddNode(TaskNode{ID: "b", Name: "test"})
	g.AddNode(TaskNode{ID: "c", Name: "deploy"})
	must(t, g.AddDependency("b", "a"))
	must(t, g.AddDependency("c", "b"))

	plan, err := BuildSchedule(g)
	if err != nil {
		t.Fatalf("BuildSchedule: %v", err)
	}

	if plan.TotalTasks != 3 {
		t.Errorf("expected 3 total tasks, got %d", plan.TotalTasks)
	}
	if plan.Depth != 3 {
		t.Errorf("expected depth 3, got %d", plan.Depth)
	}

	// Each batch should have exactly 1 task.
	for i, batch := range plan.Batches {
		if len(batch.Tasks) != 1 {
			t.Errorf("batch %d: expected 1 task, got %d", i, len(batch.Tasks))
		}
	}
}

func TestBuildSchedule_Diamond(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "a", Name: "init", Priority: 1})
	g.AddNode(TaskNode{ID: "b", Name: "left", Priority: 2})
	g.AddNode(TaskNode{ID: "c", Name: "right", Priority: 1})
	g.AddNode(TaskNode{ID: "d", Name: "merge", Priority: 1})
	must(t, g.AddDependency("b", "a"))
	must(t, g.AddDependency("c", "a"))
	must(t, g.AddDependency("d", "b"))
	must(t, g.AddDependency("d", "c"))

	plan, err := BuildSchedule(g)
	if err != nil {
		t.Fatalf("BuildSchedule: %v", err)
	}

	if plan.Depth != 3 {
		t.Errorf("expected depth 3, got %d", plan.Depth)
	}
	// Batch 0: [a], Batch 1: [b, c], Batch 2: [d]
	if len(plan.Batches[0].Tasks) != 1 || plan.Batches[0].Tasks[0].ID != "a" {
		t.Errorf("batch 0: expected [a], got %v", nodeIDs(plan.Batches[0].Tasks))
	}
	if len(plan.Batches[1].Tasks) != 2 {
		t.Errorf("batch 1: expected 2 tasks, got %d", len(plan.Batches[1].Tasks))
	}
	if len(plan.Batches[2].Tasks) != 1 || plan.Batches[2].Tasks[0].ID != "d" {
		t.Errorf("batch 2: expected [d], got %v", nodeIDs(plan.Batches[2].Tasks))
	}
}

func TestBuildSchedule_Independent(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "a", Name: "a", Priority: 3})
	g.AddNode(TaskNode{ID: "b", Name: "b", Priority: 1})
	g.AddNode(TaskNode{ID: "c", Name: "c", Priority: 2})

	plan, err := BuildSchedule(g)
	if err != nil {
		t.Fatalf("BuildSchedule: %v", err)
	}

	if plan.Depth != 1 {
		t.Errorf("expected depth 1 (all parallel), got %d", plan.Depth)
	}
	if len(plan.Batches[0].Tasks) != 3 {
		t.Errorf("expected 3 tasks in first batch, got %d", len(plan.Batches[0].Tasks))
	}
}

func TestBuildSchedule_CycleError(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "a", Name: "a"})
	g.AddNode(TaskNode{ID: "b", Name: "b"})
	must(t, g.AddDependency("a", "b"))
	must(t, g.AddDependency("b", "a"))

	_, err := BuildSchedule(g)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}

func TestBuildSchedule_Empty(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	plan, err := BuildSchedule(g)
	if err != nil {
		t.Fatalf("BuildSchedule: %v", err)
	}
	if plan.TotalTasks != 0 {
		t.Errorf("expected 0 tasks, got %d", plan.TotalTasks)
	}
}

func TestAddDependency_UnknownNode(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "a", Name: "a"})

	if err := g.AddDependency("a", "missing"); err == nil {
		t.Fatal("expected error for unknown dependency target")
	}
	if err := g.AddDependency("missing", "a"); err == nil {
		t.Fatal("expected error for unknown dependency source")
	}
}

func TestAddNode_WithDependencies(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "a", Name: "a"})
	g.AddNode(TaskNode{ID: "b", Name: "b", Dependencies: []string{"a"}})

	sorted, err := g.Sort()
	if err != nil {
		t.Fatalf("Sort: %v", err)
	}
	ids := nodeIDs(sorted)
	if idx(ids, "a") >= idx(ids, "b") {
		t.Errorf("expected a before b, got %v", ids)
	}
}

func TestDetectCycles_NoCycles(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "a", Name: "a"})
	g.AddNode(TaskNode{ID: "b", Name: "b"})
	must(t, g.AddDependency("b", "a"))

	cycles := g.DetectCycles()
	if len(cycles) != 0 {
		t.Errorf("expected no cycles, got %v", cycles)
	}
}

func TestNodes_InsertionOrder(t *testing.T) {
	t.Parallel()
	g := NewTaskGraph()
	g.AddNode(TaskNode{ID: "c", Name: "c"})
	g.AddNode(TaskNode{ID: "a", Name: "a"})
	g.AddNode(TaskNode{ID: "b", Name: "b"})

	nodes := g.Nodes()
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}
	if nodes[0].ID != "c" || nodes[1].ID != "a" || nodes[2].ID != "b" {
		t.Errorf("expected insertion order [c a b], got %v", nodeIDs(nodes))
	}
}

// --- helpers ---

func nodeIDs(nodes []TaskNode) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return ids
}

func idx(ids []string, target string) int {
	for i, id := range ids {
		if id == target {
			return i
		}
	}
	return -1
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
