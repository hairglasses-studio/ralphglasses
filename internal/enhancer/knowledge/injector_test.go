package knowledge

import (
	"strings"
	"testing"
)

func TestInjectContext_EmptyGraph(t *testing.T) {
	g := NewGraph()
	inj := NewInjector(g, 10, 500)

	prompt := "Write a function to parse JSON"
	result, improvements := inj.InjectContext(prompt)
	if result != prompt {
		t.Error("expected unchanged prompt for empty graph")
	}
	if len(improvements) != 0 {
		t.Errorf("expected no improvements, got %v", improvements)
	}
}

func TestInjectContext_NilGraph(t *testing.T) {
	inj := NewInjector(nil, 10, 500)
	prompt := "test prompt"
	result, improvements := inj.InjectContext(prompt)
	if result != prompt {
		t.Error("expected unchanged prompt for nil graph")
	}
	if len(improvements) != 0 {
		t.Error("expected no improvements for nil graph")
	}
}

func TestInjectContext_NoRelevantNodes(t *testing.T) {
	g := NewGraph()
	g.AddNode("pkg:xyz", KindPackage, map[string]string{"name": "xyz"})

	inj := NewInjector(g, 10, 500)
	prompt := "Write a haiku about clouds"
	result, improvements := inj.InjectContext(prompt)
	if result != prompt {
		t.Error("expected unchanged prompt when no nodes match")
	}
	if len(improvements) != 0 {
		t.Error("expected no improvements when no nodes match")
	}
}

func TestInjectContext_InjectsRelevantContext(t *testing.T) {
	g := NewGraph()
	g.AddNode("pkg:model", KindPackage, map[string]string{"name": "model"})
	g.AddNode("pkg:model.Repo", KindType, map[string]string{
		"name": "Repo", "kind": "struct", "fields": "Name, Path, Status",
	})
	g.AddNode("pkg:model.LoadStatus", KindFunction, map[string]string{
		"name":      "LoadStatus",
		"signature": "func LoadStatus(ctx context.Context, repoPath string) (*LoopStatus, error)",
	})
	g.AddEdge("pkg:model", "pkg:model.Repo", EdgeDefines)
	g.AddEdge("pkg:model", "pkg:model.LoadStatus", EdgeDefines)

	inj := NewInjector(g, 10, 2000)
	prompt := "Fix the repo status loading function"
	result, improvements := inj.InjectContext(prompt)

	if !strings.Contains(result, "<codebase_context>") {
		t.Error("expected XML context block in result")
	}
	if !strings.Contains(result, "Repo") {
		t.Error("expected Repo type in injected context")
	}
	if !strings.Contains(result, "LoadStatus") {
		t.Error("expected LoadStatus in injected context")
	}
	if !strings.Contains(result, prompt) {
		t.Error("original prompt should be preserved")
	}
	if len(improvements) != 1 {
		t.Errorf("expected 1 improvement, got %d", len(improvements))
	}
	if !strings.Contains(improvements[0], "knowledge graph") {
		t.Errorf("improvement should mention knowledge graph, got %q", improvements[0])
	}
}

func TestInjectContext_TokenBudget(t *testing.T) {
	g := NewGraph()
	// Add many nodes so the context block is large
	for i := range 50 {
		name := strings.Repeat("x", 100) // long names to inflate size
		g.AddNode("pkg:test."+name+string(rune('a'+i%26)), KindFunction, map[string]string{
			"name":      "test" + name,
			"signature": "func " + name + "(x int) error",
		})
	}

	// Very small token budget
	inj := NewInjector(g, 50, 50)
	prompt := "test something"
	result, _ := inj.InjectContext(prompt)

	// Result should be trimmed
	contextEnd := strings.Index(result, prompt)
	if contextEnd < 0 {
		t.Fatal("original prompt not found in result")
	}
	contextBlock := result[:contextEnd]
	// 50 tokens * 4 chars = 200 chars max (roughly)
	// Allow some overhead for the closing tag
	if len(contextBlock) > 300 {
		t.Errorf("context block too large (%d chars) for 50-token budget", len(contextBlock))
	}
}

func TestFormatContextBlockMarkdown(t *testing.T) {
	g := NewGraph()
	inj := NewInjector(g, 10, 500)

	nodes := []Node{
		{ID: "pkg:model", Kind: KindPackage, Metadata: map[string]string{"name": "model", "path": "internal/model"}},
		{ID: "pkg:model.Repo", Kind: KindType, Metadata: map[string]string{"name": "Repo", "kind": "struct", "fields": "Name, Path"}},
		{ID: "pkg:model.Load", Kind: KindFunction, Metadata: map[string]string{"name": "Load", "signature": "func Load() error"}},
	}

	md := inj.FormatContextBlockMarkdown(nodes)
	if !strings.Contains(md, "## Codebase Context") {
		t.Error("expected markdown header")
	}
	if !strings.Contains(md, "**Packages:**") {
		t.Error("expected Packages section")
	}
	if !strings.Contains(md, "`model`") {
		t.Error("expected model package")
	}
	if !strings.Contains(md, "**Types:**") {
		t.Error("expected Types section")
	}
	if !strings.Contains(md, "`Repo`") {
		t.Error("expected Repo type")
	}
	if !strings.Contains(md, "**Functions:**") {
		t.Error("expected Functions section")
	}
}

func TestFormatContextBlockMarkdown_Empty(t *testing.T) {
	g := NewGraph()
	inj := NewInjector(g, 10, 500)
	md := inj.FormatContextBlockMarkdown(nil)
	if md != "" {
		t.Errorf("expected empty string for nil nodes, got %q", md)
	}
}

func TestNewInjector_Defaults(t *testing.T) {
	g := NewGraph()
	inj := NewInjector(g, 0, 0)
	if inj.maxNodes != 10 {
		t.Errorf("default maxNodes = %d, want 10", inj.maxNodes)
	}
	if inj.maxTokens != 500 {
		t.Errorf("default maxTokens = %d, want 500", inj.maxTokens)
	}
}

func TestInjectContext_PreservesPromptIntact(t *testing.T) {
	g := NewGraph()
	g.AddNode("pkg:session", KindPackage, map[string]string{"name": "session"})
	g.AddNode("pkg:session.Manager", KindType, map[string]string{
		"name": "Manager", "kind": "struct",
	})

	inj := NewInjector(g, 5, 1000)
	prompt := "Implement session management with reconnection support"
	result, _ := inj.InjectContext(prompt)

	// The original prompt should appear verbatim at the end
	if !strings.HasSuffix(result, prompt) {
		t.Error("original prompt should be preserved at end of result")
	}
}

func TestFilterByKind(t *testing.T) {
	nodes := []Node{
		{ID: "a", Kind: KindPackage},
		{ID: "b", Kind: KindType},
		{ID: "c", Kind: KindPackage},
		{ID: "d", Kind: KindFunction},
	}

	packages := filterByKind(nodes, KindPackage)
	if len(packages) != 2 {
		t.Errorf("expected 2 packages, got %d", len(packages))
	}

	functions := filterByKind(nodes, KindFunction)
	if len(functions) != 1 {
		t.Errorf("expected 1 function, got %d", len(functions))
	}

	files := filterByKind(nodes, KindFile)
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}
