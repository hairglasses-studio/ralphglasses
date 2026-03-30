package graph

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestNewGraphStore(t *testing.T) {
	g := NewGraphStore()
	if g == nil {
		t.Fatal("expected non-nil store")
	}
	if len(g.Nodes()) != 0 {
		t.Fatal("expected empty nodes")
	}
	if len(g.Edges()) != 0 {
		t.Fatal("expected empty edges")
	}
}

func TestAddNode_Replace(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "n1", Kind: KindFunction, Name: "Original"})
	g.AddNode(&Node{ID: "n1", Kind: KindMethod, Name: "Replaced"})

	n := g.GetNode("n1")
	if n.Name != "Replaced" {
		t.Fatalf("expected replaced name, got %s", n.Name)
	}
	if n.Kind != KindMethod {
		t.Fatalf("expected kind method, got %s", n.Kind)
	}
}

func TestGetNode_NotFound(t *testing.T) {
	g := NewGraphStore()
	if g.GetNode("nonexistent") != nil {
		t.Fatal("expected nil for missing node")
	}
}

func TestRemoveNode_Nonexistent(t *testing.T) {
	g := NewGraphStore()
	// Should not panic.
	g.RemoveNode("nonexistent")
}

func TestRemoveNode_CleansIncomingEdges(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	g.AddNode(&Node{ID: "c", Kind: KindFunction, Name: "c"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "c", To: "b", Kind: EdgeCalls})

	g.RemoveNode("b")

	if len(g.OutEdges("a")) != 0 {
		t.Fatal("outgoing edge from a to removed b should be cleaned")
	}
	if len(g.OutEdges("c")) != 0 {
		t.Fatal("outgoing edge from c to removed b should be cleaned")
	}
}

func TestAddEdge_MissingSource(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	err := g.AddEdge(&Edge{From: "missing", To: "b", Kind: EdgeCalls})
	if err == nil {
		t.Fatal("expected error for missing source")
	}
}

func TestAddEdge_MissingTarget(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	err := g.AddEdge(&Edge{From: "a", To: "missing", Kind: EdgeCalls})
	if err == nil {
		t.Fatal("expected error for missing target")
	}
}

func TestOutEdges_Empty(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	edges := g.OutEdges("a")
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(edges))
	}
}

func TestInEdges_Empty(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	edges := g.InEdges("a")
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(edges))
	}
}

func TestOutEdges_ReturnsCopy(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})

	edges := g.OutEdges("a")
	edges[0] = nil // mutate the copy
	// Original should be intact.
	original := g.OutEdges("a")
	if original[0] == nil {
		t.Fatal("OutEdges should return a copy, not the internal slice")
	}
}

func TestInEdges_ReturnsCopy(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})

	edges := g.InEdges("b")
	edges[0] = nil
	original := g.InEdges("b")
	if original[0] == nil {
		t.Fatal("InEdges should return a copy, not the internal slice")
	}
}

func TestNodes_AllReturned(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindType, Name: "b"})
	g.AddNode(&Node{ID: "c", Kind: KindPackage, Name: "c"})

	nodes := g.Nodes()
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}
}

func TestEdges_AllReturned(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	g.AddNode(&Node{ID: "c", Kind: KindFunction, Name: "c"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})
	_ = g.AddEdge(&Edge{From: "b", To: "c", Kind: EdgeCalls})

	edges := g.Edges()
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}
}

func TestNodesByKind_NoMatch(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	nodes := g.NodesByKind(KindInterface)
	if len(nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(nodes))
	}
}

func TestEdgesByKind_NoMatch(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls})

	edges := g.EdgesByKind(EdgeImports)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(edges))
	}
}

func TestRemoveEdge_Specific(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	e1 := &Edge{From: "a", To: "b", Kind: EdgeCalls}
	e2 := &Edge{From: "a", To: "b", Kind: EdgeImports}
	_ = g.AddEdge(e1)
	_ = g.AddEdge(e2)

	g.RemoveEdge(e1)

	out := g.OutEdges("a")
	if len(out) != 1 {
		t.Fatalf("expected 1 remaining edge, got %d", len(out))
	}
	if out[0].Kind != EdgeImports {
		t.Fatalf("expected imports edge to remain, got %s", out[0].Kind)
	}
}

func TestMarshalJSON_EmptyStore(t *testing.T) {
	g := NewGraphStore()
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	g2 := NewGraphStore()
	if err := json.Unmarshal(data, g2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(g2.Nodes()) != 0 {
		t.Fatal("expected empty store after round-trip")
	}
}

func TestUnmarshalJSON_ReplacesContents(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "old", Kind: KindFunction, Name: "old"})

	data, _ := json.Marshal(NewGraphStore())
	if err := json.Unmarshal(data, g); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if g.GetNode("old") != nil {
		t.Fatal("old node should be replaced by unmarshal")
	}
}

func TestUnmarshalJSON_InvalidData(t *testing.T) {
	g := NewGraphStore()
	err := json.Unmarshal([]byte(`{invalid`), g)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestMarshalJSON_PreservesEdges(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})
	_ = g.AddEdge(&Edge{From: "a", To: "b", Kind: EdgeCalls, Metadata: map[string]string{"weight": "1"}})

	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	g2 := NewGraphStore()
	if err := json.Unmarshal(data, g2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	edges := g2.OutEdges("a")
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].Metadata["weight"] != "1" {
		t.Fatal("edge metadata not preserved")
	}
}

func TestConcurrentAccess(t *testing.T) {
	g := NewGraphStore()
	g.AddNode(&Node{ID: "a", Kind: KindFunction, Name: "a"})
	g.AddNode(&Node{ID: "b", Kind: KindFunction, Name: "b"})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g.GetNode("a")
			g.Nodes()
			g.OutEdges("a")
			g.InEdges("b")
			g.NodesByKind(KindFunction)
			g.EdgesByKind(EdgeCalls)
		}()
	}
	wg.Wait()
}

func TestNodeKindConstants(t *testing.T) {
	kinds := []NodeKind{
		KindPackage, KindFile, KindFunction, KindMethod,
		KindType, KindInterface, KindField, KindVariable, KindConstant,
	}
	seen := map[NodeKind]bool{}
	for _, k := range kinds {
		if seen[k] {
			t.Fatalf("duplicate kind: %s", k)
		}
		seen[k] = true
		if k == "" {
			t.Fatal("empty kind constant")
		}
	}
}

func TestEdgeKindConstants(t *testing.T) {
	kinds := []EdgeKind{
		EdgeImports, EdgeCalls, EdgeImplements, EdgeEmbeds,
		EdgeDeclaredIn, EdgeReturns, EdgeReceives, EdgeReferences, EdgeDependsOn,
	}
	seen := map[EdgeKind]bool{}
	for _, k := range kinds {
		if seen[k] {
			t.Fatalf("duplicate edge kind: %s", k)
		}
		seen[k] = true
		if k == "" {
			t.Fatal("empty edge kind constant")
		}
	}
}
