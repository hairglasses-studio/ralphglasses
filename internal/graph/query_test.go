package graph

import (
	"sort"
	"testing"
)

func TestNewQueryEngine(t *testing.T) {
	g := NewGraphStore()
	q := NewQueryEngine(g)
	if q == nil {
		t.Fatal("expected non-nil query engine")
	}
}

func TestDependencies_NoEdges(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	q := NewQueryEngine(g)
	deps := q.Dependencies("a")
	if len(deps) != 0 {
		t.Fatalf("expected 0 dependencies, got %d", len(deps))
	}
}

func TestDependencies_FilterByKind(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	g.AddNode(&Node{ID: "c", Kind: KindPackage, Name: "c"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "a", To: "c", Kind: EdgeImports})

	q := NewQueryEngine(g)

	calls := q.Dependencies("a", EdgeCalls)
	if len(calls) != 1 || calls[0].ID != "b" {
		t.Fatalf("expected [b], got %v", nodeIDs(calls))
	}

	imports := q.Dependencies("a", EdgeImports)
	if len(imports) != 1 || imports[0].ID != "c" {
		t.Fatalf("expected [c], got %v", nodeIDs(imports))
	}
}

func TestDependencies_Dedup(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeImports})

	q := NewQueryEngine(g)
	deps := q.Dependencies("a")
	if len(deps) != 1 {
		t.Fatalf("expected 1 unique dependency, got %d", len(deps))
	}
}

func TestDependents_NoEdges(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	q := NewQueryEngine(g)
	deps := q.Dependents("a")
	if len(deps) != 0 {
		t.Fatalf("expected 0 dependents, got %d", len(deps))
	}
}

func TestDependents_FilterByKind(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	g.AddNode(&Node{ID: "c", Kind: KindFunction, Name: "c"})
	_ = g.AddEdge(&Edge{From: "b", To: "a", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "c", To: "a", Kind: EdgeImports})

	q := NewQueryEngine(g)

	callers := q.Dependents("a", EdgeCalls)
	if len(callers) != 1 || callers[0].ID != "b" {
		t.Fatalf("expected [b], got %v", nodeIDs(callers))
	}
}

func TestDependents_Dedup(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	_ = g.AddEdge(&Edge{From: "b", To: "a", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "b", To: "a", Kind: EdgeImports})

	q := NewQueryEngine(g)
	deps := q.Dependents("a")
	if len(deps) != 1 {
		t.Fatalf("expected 1 unique dependent, got %d", len(deps))
	}
}

func TestTransitiveDependencies_Empty(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	q := NewQueryEngine(g)
	deps := q.TransitiveDependencies("a")
	if len(deps) != 0 {
		t.Fatalf("expected 0 transitive deps, got %d", len(deps))
	}
}

func TestTransitiveDependencies_Chain(t *testing.T) {
	g := NewGraphStore()
	for _, id := range []string{"a", "b", "c", "d"} {
		g.AddNode(&Node{ID: id, Kind: KindFunction, Name: id})
	}
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "b", To: "c", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "c", To: "d", Kind: EdgeCalls})

	q := NewQueryEngine(g)
	deps := q.TransitiveDependencies("a")
	ids := nodeIDs(deps)
	sort.Strings(ids)
	if len(ids) != 3 || ids[0] != "b" || ids[1] != "c" || ids[2] != "d" {
		t.Fatalf("expected [b c d], got %v", ids)
	}
}

func TestTransitiveDependencies_FilteredByKind(t *testing.T) {
	g := NewGraphStore()
	for _, id := range []string{"a", "b", "c"} {
		g.AddNode(&Node{ID: id, Kind: KindFunction, Name: id})
	}
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "b", To: "c", Kind: EdgeImports})

	q := NewQueryEngine(g)
	deps := q.TransitiveDependencies("a", EdgeCalls)
	if len(deps) != 1 || deps[0].ID != "b" {
		t.Fatalf("expected [b], got %v", nodeIDs(deps))
	}
}

func TestTransitiveDependencies_Cycle(t *testing.T) {
	g := NewGraphStore()
	for _, id := range []string{"a", "b", "c"} {
		g.AddNode(&Node{ID: id, Kind: KindFunction, Name: id})
	}
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "b", To: "c", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "c", To: "a", Kind: EdgeCalls})

	q := NewQueryEngine(g)
	deps := q.TransitiveDependencies("a")
	ids := nodeIDs(deps)
	sort.Strings(ids)
	// Should not infinite loop; should find b and c.
	if len(ids) != 2 || ids[0] != "b" || ids[1] != "c" {
		t.Fatalf("expected [b c], got %v", ids)
	}
}

func TestShortestPath_Direct(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})

	q := NewQueryEngine(g)
	path := q.ShortestPath("a", "b")
	if len(path) != 2 || path[0] != "a" || path[1] != "b" {
		t.Fatalf("expected [a b], got %v", path)
	}
}

func TestShortestPath_FilterByKind(t *testing.T) {
	g := NewGraphStore()
	for _, id := range []string{"a", "b", "c"} {
		g.AddNode(&Node{ID: id, Kind: KindFunction, Name: id})
	}
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "b", To: "c", Kind: EdgeImports})

	q := NewQueryEngine(g)
	// Only follow calls -- should not reach c.
	path := q.ShortestPath("a", "c", EdgeCalls)
	if path != nil {
		t.Fatalf("expected nil path when filtered, got %v", path)
	}

	// Follow all edge kinds.
	path = q.ShortestPath("a", "c")
	if len(path) != 3 {
		t.Fatalf("expected path of length 3, got %v", path)
	}
}

func TestDetectCycles_NoCycles(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})

	q := NewQueryEngine(g)
	cycles := q.DetectCycles()
	if len(cycles) != 0 {
		t.Fatalf("expected no cycles, got %v", cycles)
	}
}

func TestDetectCycles_SelfLoop(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	_ = g.AddEdge(&Edge{From: "a", To: "a", Kind: EdgeCalls})

	q := NewQueryEngine(g)
	cycles := q.DetectCycles()
	if len(cycles) == 0 {
		t.Fatal("expected self-loop cycle")
	}
}

func TestDetectCycles_FilterByKind(t *testing.T) {
	g := NewGraphStore()
	for _, id := range []string{"a", "b"} {
		g.AddNode(&Node{ID: id, Kind: KindFunction, Name: id})
	}
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "b", To: "a", Kind: EdgeImports})

	q := NewQueryEngine(g)
	// Cycle exists across mixed kinds, but not within calls-only.
	callCycles := q.DetectCycles(EdgeCalls)
	if len(callCycles) != 0 {
		t.Fatalf("expected no call-only cycles, got %v", callCycles)
	}

	// All kinds should find the cycle.
	allCycles := q.DetectCycles()
	if len(allCycles) == 0 {
		t.Fatal("expected cycle with all edge kinds")
	}
}

func TestSubgraph_Empty(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	q := NewQueryEngine(g)
	sub := q.Subgraph([]string{})
	if len(sub.Nodes()) != 0 {
		t.Fatal("expected empty subgraph")
	}
}

func TestSubgraph_NonexistentNode(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	q := NewQueryEngine(g)
	sub := q.Subgraph([]string{"nonexistent"})
	if len(sub.Nodes()) != 0 {
		t.Fatal("expected no nodes for nonexistent ID")
	}
}

func TestSubgraph_EdgesPreserved(t *testing.T) {
	g := NewGraphStore()
	for _, id := range []string{"a", "b", "c"} {
		g.AddNode(&Node{ID: id, Kind: KindFunction, Name: id})
	}
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "a", To: "c", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "b", To: "c", Kind: EdgeCalls})

	q := NewQueryEngine(g)
	sub := q.Subgraph([]string{"a", "c"})
	if len(sub.Nodes()) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(sub.Nodes()))
	}
	edges := sub.Edges()
	// Only a->c should survive.
	if len(edges) != 1 || edges[0].From != "a" || edges[0].To != "c" {
		t.Fatalf("expected a->c edge only, got %v", edges)
	}
}

func TestReachableFrom_Empty(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	q := NewQueryEngine(g)
	r := q.ReachableFrom("a")
	if len(r) != 0 {
		t.Fatalf("expected 0 reachable, got %d", len(r))
	}
}

func TestReachableFrom_ExcludesStart(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})

	q := NewQueryEngine(g)
	r := q.ReachableFrom("a")
	for _, id := range r {
		if id == "a" {
			t.Fatal("start node should not be in reachable set")
		}
	}
}

func TestReachableFrom_FilterByKind(t *testing.T) {
	g := NewGraphStore()
	for _, id := range []string{"a", "b", "c"} {
		g.AddNode(&Node{ID: id, Kind: KindFunction, Name: id})
	}
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "b", To: "c", Kind: EdgeImports})

	q := NewQueryEngine(g)
	r := q.ReachableFrom("a", EdgeCalls)
	if len(r) != 1 || r[0] != "b" {
		t.Fatalf("expected [b], got %v", r)
	}
}

func TestReachableFrom_Cycle(t *testing.T) {
	g := NewGraphStore()
	for _, id := range []string{"a", "b", "c"} {
		g.AddNode(&Node{ID: id, Kind: KindFunction, Name: id})
	}
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "b", To: "c", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "c", To: "a", Kind: EdgeCalls})

	q := NewQueryEngine(g)
	r := q.ReachableFrom("a")
	sort.Strings(r)
	if len(r) != 2 || r[0] != "b" || r[1] != "c" {
		t.Fatalf("expected [b c], got %v", r)
	}
}

func TestMakeKindSet_Empty(t *testing.T) {
	m := makeKindSet(nil)
	if m != nil {
		t.Fatal("expected nil for empty input")
	}
	m = makeKindSet([]EdgeKind{})
	if m != nil {
		t.Fatal("expected nil for empty slice")
	}
}

func TestMakeKindSet_Values(t *testing.T) {
	m := makeKindSet([]EdgeKind{EdgeCalls, EdgeImports})
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m))
	}
	if !m[EdgeCalls] || !m[EdgeImports] {
		t.Fatal("expected both kinds in set")
	}
}
