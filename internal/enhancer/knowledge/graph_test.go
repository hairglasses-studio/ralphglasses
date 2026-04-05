package knowledge

import (
	"sync"
	"testing"
)

func TestNewGraph(t *testing.T) {
	g := NewGraph()
	if g == nil {
		t.Fatal("NewGraph returned nil")
	}
	if g.NodeCount() != 0 {
		t.Errorf("NodeCount = %d, want 0", g.NodeCount())
	}
	if g.EdgeCount() != 0 {
		t.Errorf("EdgeCount = %d, want 0", g.EdgeCount())
	}
}

func TestAddNode(t *testing.T) {
	g := NewGraph()
	g.AddNode("pkg:model", KindPackage, map[string]string{"name": "model"})

	if g.NodeCount() != 1 {
		t.Errorf("NodeCount = %d, want 1", g.NodeCount())
	}

	n := g.GetNode("pkg:model")
	if n == nil {
		t.Fatal("GetNode returned nil")
	}
	if n.Kind != KindPackage {
		t.Errorf("Kind = %q, want %q", n.Kind, KindPackage)
	}
	if n.Metadata["name"] != "model" {
		t.Errorf("Metadata[name] = %q, want %q", n.Metadata["name"], "model")
	}
}

func TestAddNodeMergesMetadata(t *testing.T) {
	g := NewGraph()
	g.AddNode("pkg:foo", KindPackage, map[string]string{"name": "foo"})
	g.AddNode("pkg:foo", KindPackage, map[string]string{"path": "/a/b"})

	n := g.GetNode("pkg:foo")
	if n.Metadata["name"] != "foo" {
		t.Errorf("name lost after merge: got %q", n.Metadata["name"])
	}
	if n.Metadata["path"] != "/a/b" {
		t.Errorf("path not merged: got %q", n.Metadata["path"])
	}
}

func TestGetNodeNotFound(t *testing.T) {
	g := NewGraph()
	if g.GetNode("nonexistent") != nil {
		t.Error("expected nil for nonexistent node")
	}
}

func TestAddEdge(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", KindPackage, nil)
	g.AddNode("b", KindPackage, nil)
	g.AddEdge("a", "b", EdgeImports)

	if g.EdgeCount() != 1 {
		t.Errorf("EdgeCount = %d, want 1", g.EdgeCount())
	}
}

func TestQuery(t *testing.T) {
	g := NewGraph()
	g.AddNode("pkg:main", KindPackage, map[string]string{"name": "main"})
	g.AddNode("pkg:model", KindPackage, map[string]string{"name": "model"})
	g.AddNode("pkg:utils", KindPackage, map[string]string{"name": "utils"})
	g.AddEdge("pkg:main", "pkg:model", EdgeImports)
	g.AddEdge("pkg:utils", "pkg:main", EdgeImports)

	result := g.Query("pkg:main")
	if result == nil {
		t.Fatal("Query returned nil")
	}
	if result.Node.ID != "pkg:main" {
		t.Errorf("Node.ID = %q, want %q", result.Node.ID, "pkg:main")
	}
	if len(result.Outgoing) != 1 {
		t.Errorf("Outgoing = %d, want 1", len(result.Outgoing))
	}
	if len(result.Incoming) != 1 {
		t.Errorf("Incoming = %d, want 1", len(result.Incoming))
	}
	if len(result.Neighbors) != 2 {
		t.Errorf("Neighbors = %d, want 2", len(result.Neighbors))
	}
}

func TestQueryNotFound(t *testing.T) {
	g := NewGraph()
	if g.Query("ghost") != nil {
		t.Error("expected nil for nonexistent node")
	}
}

func TestSubgraph(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", KindPackage, nil)
	g.AddNode("b", KindPackage, nil)
	g.AddNode("c", KindPackage, nil)
	g.AddEdge("a", "b", EdgeImports)
	g.AddEdge("b", "c", EdgeImports)
	g.AddEdge("a", "c", EdgeImports)

	sub := g.Subgraph([]string{"a", "b"})
	if sub.NodeCount() != 2 {
		t.Errorf("Subgraph NodeCount = %d, want 2", sub.NodeCount())
	}
	// Only a->b edge should be in subgraph (not a->c or b->c)
	if sub.EdgeCount() != 1 {
		t.Errorf("Subgraph EdgeCount = %d, want 1", sub.EdgeCount())
	}
}

func TestRelatedContext(t *testing.T) {
	g := NewGraph()
	g.AddNode("pkg:model", KindPackage, map[string]string{"name": "model"})
	g.AddNode("pkg:model.Repo", KindType, map[string]string{"name": "Repo", "kind": "struct", "fields": "Name, Path, Status"})
	g.AddNode("pkg:model.LoadStatus", KindFunction, map[string]string{"name": "LoadStatus", "signature": "func LoadStatus(ctx context.Context, repoPath string) (*LoopStatus, error)"})
	g.AddNode("pkg:tui", KindPackage, map[string]string{"name": "tui"})

	g.AddEdge("pkg:model", "pkg:model.Repo", EdgeDefines)
	g.AddEdge("pkg:model", "pkg:model.LoadStatus", EdgeDefines)

	// Query for "repo status"
	nodes := g.RelatedContext("repo status", 5)
	if len(nodes) == 0 {
		t.Fatal("RelatedContext returned no nodes")
	}

	// "Repo" and "LoadStatus" should be highly ranked
	foundRepo := false
	for _, n := range nodes {
		if n.Metadata["name"] == "Repo" {
			foundRepo = true
			break
		}
	}
	if !foundRepo {
		t.Error("expected Repo node in results")
	}
}

func TestRelatedContextEmpty(t *testing.T) {
	g := NewGraph()
	nodes := g.RelatedContext("anything", 5)
	if len(nodes) != 0 {
		t.Errorf("expected empty results, got %d", len(nodes))
	}
}

func TestRelatedContextEmptyQuery(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", KindPackage, nil)
	nodes := g.RelatedContext("", 5)
	if len(nodes) != 0 {
		t.Errorf("expected empty results for empty query, got %d", len(nodes))
	}
}

func TestRelatedContextMaxNodes(t *testing.T) {
	g := NewGraph()
	for i := range 20 {
		g.AddNode("pkg:test"+string(rune('a'+i)), KindPackage, map[string]string{"name": "test"})
	}
	nodes := g.RelatedContext("test", 3)
	if len(nodes) > 3 {
		t.Errorf("expected at most 3 nodes, got %d", len(nodes))
	}
}

func TestAllNodes(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", KindPackage, nil)
	g.AddNode("b", KindType, nil)

	all := g.AllNodes()
	if len(all) != 2 {
		t.Errorf("AllNodes returned %d, want 2", len(all))
	}
}

func TestConcurrentAccess(t *testing.T) {
	g := NewGraph()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune('a' + (i % 26)))
			g.AddNode(id, KindPackage, map[string]string{"i": id})
			if i > 0 {
				prev := string(rune('a' + ((i - 1) % 26)))
				g.AddEdge(id, prev, EdgeImports)
			}
		}(i)
	}

	// Concurrent reads
	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune('a' + (i % 26)))
			g.GetNode(id)
			g.Query(id)
			g.NodeCount()
			g.EdgeCount()
		}(i)
	}

	wg.Wait()

	// Just verify no panics and counts are reasonable
	if g.NodeCount() == 0 {
		t.Error("expected some nodes after concurrent writes")
	}
}
