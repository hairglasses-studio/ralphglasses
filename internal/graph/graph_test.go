package graph

import (
	"encoding/json"
	"sort"
	"testing"
)

// ---------------------------------------------------------------------------
// Store CRUD
// ---------------------------------------------------------------------------

func TestAddAndGetNode(t *testing.T) {
	g := NewGraphStore()
	n := &Node{ID: "n1", Kind: KindFunction, Name: "Foo"}
	g.AddNode(n)

	got := g.GetNode("n1")
	if got == nil {
		t.Fatal("expected node, got nil")
	}
	if got.Name != "Foo" {
		t.Fatalf("expected name Foo, got %s", got.Name)
	}
}

func TestRemoveNode(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindType, Name: "A"})
	g.AddNode(&Node{ID: "b", Kind: KindType, Name: "B"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})

	g.RemoveNode("a")

	if g.GetNode("a") != nil {
		t.Fatal("node a should be removed")
	}
	if len(g.OutEdges("a")) != 0 {
		t.Fatal("edges from a should be removed")
	}
	if len(g.InEdges("b")) != 0 {
		t.Fatal("reverse edge to b should be removed")
	}
}

func TestAddEdgeValidation(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "A"})

	err := g.AddEdge(&Edge{From: "a", To: "missing", Kind: EdgeCalls})
	if err == nil {
		t.Fatal("expected error for missing target node")
	}

	err = g.AddEdge(&Edge{From: "missing", To: "a", Kind: EdgeCalls})
	if err == nil {
		t.Fatal("expected error for missing source node")
	}
}

func TestRemoveEdge(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindType, Name: "A"})
	g.AddNode(&Node{ID: "b", Kind: KindType, Name: "B"})
	e := &Edge{From: "a", To: "b", Kind: EdgeCalls}
	_ = g.AddEdge(e)

	g.RemoveEdge(e)
	if len(g.OutEdges("a")) != 0 {
		t.Fatal("edge should be removed from forward list")
	}
	if len(g.InEdges("b")) != 0 {
		t.Fatal("edge should be removed from reverse list")
	}
}

func TestNodesByKind(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "f1", Kind: KindFunction, Name: "F1"})
	g.AddNode(&Node{ID: "f2", Kind: KindFunction, Name: "F2"})
	g.AddNode(&Node{ID: "t1", Kind: KindType, Name: "T1"})

	funcs := g.NodesByKind(KindFunction)
	if len(funcs) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(funcs))
	}
}

func TestEdgesByKind(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "A"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "B"})
	g.AddNode(&Node{ID: "c", Kind: KindPackage, Name: "C"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "a", To: "c", Kind: EdgeImports})

	calls := g.EdgesByKind(EdgeCalls)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call edge, got %d", len(calls))
	}
}

// ---------------------------------------------------------------------------
// JSON serialization round-trip
// ---------------------------------------------------------------------------

func TestSerializationRoundTrip(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "pkg:main", Kind: KindPackage, Name: "main"})
	g.AddNode(&Node{ID: "func:main.Run", Kind: KindFunction, Name: "Run", Package: "main"})
	_ = g.AddEdge(&Edge{From: "func:main.Run", To: "pkg:main", Kind: EdgeDeclaredIn})

	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	g2 := NewGraphStore()
	if err := json.Unmarshal(data, g2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if g2.GetNode("pkg:main") == nil {
		t.Fatal("missing node after round-trip")
	}
	if g2.GetNode("func:main.Run") == nil {
		t.Fatal("missing function node after round-trip")
	}
	edges := g2.OutEdges("func:main.Run")
	if len(edges) != 1 || edges[0].Kind != EdgeDeclaredIn {
		t.Fatal("edge not preserved after round-trip")
	}
}

// ---------------------------------------------------------------------------
// Parser: Go source extraction
// ---------------------------------------------------------------------------

const testSource = `package sample

import "fmt"

const Version = "1.0"

var Debug bool

type Greeter struct {
	Name string
}

type Speaker interface {
	Speak() string
}

func Hello() {
	fmt.Println("hello")
}

func (g *Greeter) Greet() string {
	return Hello()
}
`

func TestParseSource(t *testing.T) {
	store := NewGraphStore()
	p := NewCodeParser()
	if err := p.ParseSource(store, "sample.go", testSource); err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Package node.
	if n := store.GetNode("pkg:sample"); n == nil {
		t.Fatal("missing package node")
	}

	// Import.
	if n := store.GetNode("pkg:fmt"); n == nil {
		t.Fatal("missing import node for fmt")
	}
	impEdges := store.EdgesByKind(EdgeImports)
	if len(impEdges) == 0 {
		t.Fatal("no import edges found")
	}

	// Function.
	if n := store.GetNode("func:sample.Hello"); n == nil {
		t.Fatal("missing function node Hello")
	}

	// Method.
	if n := store.GetNode("method:sample.Greeter.Greet"); n == nil {
		t.Fatal("missing method node Greet")
	}

	// Receiver edge.
	recvEdges := store.EdgesByKind(EdgeReceives)
	if len(recvEdges) == 0 {
		t.Fatal("no receiver edges found")
	}

	// Type.
	if n := store.GetNode("type:sample.Greeter"); n == nil {
		t.Fatal("missing type node Greeter")
	}

	// Interface.
	speaker := store.GetNode("type:sample.Speaker")
	if speaker == nil {
		t.Fatal("missing interface node Speaker")
	}
	if speaker.Kind != KindInterface {
		t.Fatalf("expected interface kind, got %s", speaker.Kind)
	}

	// Constant.
	if n := store.GetNode("constant:sample.Version"); n == nil {
		t.Fatal("missing constant node Version")
	}

	// Variable.
	if n := store.GetNode("variable:sample.Debug"); n == nil {
		t.Fatal("missing variable node Debug")
	}

	// Field.
	if n := store.GetNode("field:sample.Greeter.Name"); n == nil {
		t.Fatal("missing field node Name")
	}

	// Call edges from Hello to fmt.Println.
	helloEdges := store.OutEdges("func:sample.Hello")
	hasFmtCall := false
	for _, e := range helloEdges {
		if e.Kind == EdgeCalls && e.To == "ref:fmt.Println" {
			hasFmtCall = true
		}
	}
	if !hasFmtCall {
		t.Fatal("missing call edge from Hello to fmt.Println")
	}
}

// ---------------------------------------------------------------------------
// Query: path finding
// ---------------------------------------------------------------------------

func TestShortestPath(t *testing.T) {
	g := NewGraphStore()
	for _, id := range []string{"a", "b", "c", "d"} {
		g.AddNode(&Node{ID: id, Kind: KindFunction, Name: id})
	}
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "b", To: "c", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "a", To: "d", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "d", To: "c", Kind: EdgeCalls})

	q := NewQueryEngine(g)
	path := q.ShortestPath("a", "c")
	if len(path) != 3 {
		t.Fatalf("expected path of length 3, got %v", path)
	}
	if path[0] != "a" || path[2] != "c" {
		t.Fatalf("unexpected path: %v", path)
	}
}

func TestShortestPathSameNode(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "x", Kind: KindFunction, Name: "x"})
	q := NewQueryEngine(g)
	path := q.ShortestPath("x", "x")
	if len(path) != 1 || path[0] != "x" {
		t.Fatalf("expected [x], got %v", path)
	}
}

func TestShortestPathNoRoute(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})

	q := NewQueryEngine(g)
	path := q.ShortestPath("a", "b")
	if path != nil {
		t.Fatalf("expected nil path, got %v", path)
	}
}

// ---------------------------------------------------------------------------
// Query: dependencies / dependents
// ---------------------------------------------------------------------------

func TestDependenciesAndDependents(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	g.AddNode(&Node{ID: "c", Kind: KindFunction, Name: "c"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "a", To: "c", Kind: EdgeImports})

	q := NewQueryEngine(g)

	// All dependencies.
	deps := q.Dependencies("a")
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(deps))
	}

	// Filtered by kind.
	callDeps := q.Dependencies("a", EdgeCalls)
	if len(callDeps) != 1 || callDeps[0].ID != "b" {
		t.Fatalf("expected [b], got %v", callDeps)
	}

	// Dependents of b.
	depnts := q.Dependents("b")
	if len(depnts) != 1 || depnts[0].ID != "a" {
		t.Fatalf("expected [a], got %v", depnts)
	}
}

func TestTransitiveDependencies(t *testing.T) {
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

// ---------------------------------------------------------------------------
// Query: cycle detection
// ---------------------------------------------------------------------------

func TestDetectCycles(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindPackage, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindPackage, Name: "b"})
	g.AddNode(&Node{ID: "c", Kind: KindPackage, Name: "c"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeImports})
	_ = g.AddEdge(&Edge{From: "b", To: "c", Kind: EdgeImports})
	_ = g.AddEdge(&Edge{From: "c", To: "a", Kind: EdgeImports})

	q := NewQueryEngine(g)
	cycles := q.DetectCycles(EdgeImports)
	if len(cycles) == 0 {
		t.Fatal("expected at least one cycle")
	}

	// The cycle should contain a, b, c.
	found := false
	for _, cycle := range cycles {
		ids := make(map[string]bool)
		for _, id := range cycle {
			ids[id] = true
		}
		if ids["a"] && ids["b"] && ids["c"] {
			found = true
		}
	}
	if !found {
		t.Fatalf("cycle a->b->c->a not detected; got %v", cycles)
	}
}

func TestNoCycles(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindPackage, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindPackage, Name: "b"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeImports})

	q := NewQueryEngine(g)
	cycles := q.DetectCycles()
	if len(cycles) != 0 {
		t.Fatalf("expected no cycles, got %v", cycles)
	}
}

// ---------------------------------------------------------------------------
// Query: subgraph extraction
// ---------------------------------------------------------------------------

func TestSubgraph(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	g.AddNode(&Node{ID: "c", Kind: KindFunction, Name: "c"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "a", To: "c", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "b", To: "c", Kind: EdgeCalls})

	q := NewQueryEngine(g)
	sub := q.Subgraph([]string{"a", "b"})

	if sub.GetNode("c") != nil {
		t.Fatal("node c should not be in subgraph")
	}
	if len(sub.Nodes()) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(sub.Nodes()))
	}
	// Only a->b should survive; a->c and b->c should be dropped.
	edges := sub.Edges()
	if len(edges) != 1 || edges[0].To != "b" {
		t.Fatalf("expected 1 edge a->b, got %v", edges)
	}
}

// ---------------------------------------------------------------------------
// Query: reachable
// ---------------------------------------------------------------------------

func TestReachableFrom(t *testing.T) {
	g := NewGraphStore()
	for _, id := range []string{"a", "b", "c", "d"} {
		g.AddNode(&Node{ID: id, Kind: KindFunction, Name: id})
	}
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "b", To: "c", Kind: EdgeCalls})
	// d is disconnected.

	q := NewQueryEngine(g)
	reachable := q.ReachableFrom("a")
	sort.Strings(reachable)
	if len(reachable) != 2 || reachable[0] != "b" || reachable[1] != "c" {
		t.Fatalf("expected [b c], got %v", reachable)
	}
}

// ---------------------------------------------------------------------------
// Integration: parse + query
// ---------------------------------------------------------------------------

func TestParseAndQueryIntegration(t *testing.T) {
	store := NewGraphStore()
	p := NewCodeParser()
	if err := p.ParseSource(store, "sample.go", testSource); err != nil {
		t.Fatalf("parse: %v", err)
	}

	q := NewQueryEngine(store)

	// Package should import fmt.
	deps := q.Dependencies("pkg:sample", EdgeImports)
	if len(deps) != 1 || deps[0].ID != "pkg:fmt" {
		t.Fatalf("expected fmt import, got %v", deps)
	}

	// Greeter type should have field declared in it.
	fieldDeps := q.Dependents("type:sample.Greeter", EdgeDeclaredIn)
	hasNameField := false
	for _, n := range fieldDeps {
		if n.ID == "field:sample.Greeter.Name" {
			hasNameField = true
		}
	}
	if !hasNameField {
		t.Fatal("expected Name field as dependent of Greeter type")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func nodeIDs(nodes []*Node) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return ids
}
