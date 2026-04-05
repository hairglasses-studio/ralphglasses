package roadmap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractActionableItems(t *testing.T) {
	content := `This library has a security vulnerability that should be patched.
The API has changed significantly since the last review.`

	proposals := ExtractActionableItems("test-lib", "mcp", content, 2)
	if len(proposals) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(proposals))
	}
	if proposals[0].Domain != "mcp" {
		t.Errorf("domain = %q, want mcp", proposals[0].Domain)
	}
	if proposals[0].Complexity != 2 {
		t.Errorf("complexity = %d, want 2", proposals[0].Complexity)
	}
}

func TestExtractActionableItemsNoMatch(t *testing.T) {
	content := "This is a normal research document with no actionable signals."
	proposals := ExtractActionableItems("boring", "mcp", content, 1)
	if len(proposals) != 0 {
		t.Errorf("expected 0 proposals for non-actionable content, got %d", len(proposals))
	}
}

func TestAppendDiscovery(t *testing.T) {
	dir := t.TempDir()
	disc := ResearchDiscovery{
		Topic:      "test-topic",
		Domain:     "mcp",
		Summary:    "test summary",
		Complexity: 2,
		Actionable: true,
		Source:     "research-daemon",
	}

	if err := AppendDiscovery(dir, disc); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "knowledge", "ralph-discoveries.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "test-topic") {
		t.Error("discovery not found in output")
	}
}

func TestAppendToNextMarathonSeeds(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "strategy"), 0o755)

	proposal := ResearchProposal{
		Title:      "[research] mcp: caching patterns",
		Domain:     "mcp",
		Complexity: 2,
		Source:     "research-daemon",
	}

	if err := AppendToNextMarathonSeeds(dir, proposal); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "strategy", "next-marathon-seeds.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "caching patterns") {
		t.Error("seed not found in output")
	}
	if !strings.Contains(string(data), "- [ ]") {
		t.Error("expected unchecked checkbox")
	}
}

func TestProposeToMetaRoadmap(t *testing.T) {
	dir := t.TempDir()
	stratDir := filepath.Join(dir, "strategy")
	os.MkdirAll(stratDir, 0o755)

	// Create a minimal META-ROADMAP.md.
	initial := "# META-ROADMAP\n\n## Phase 1\n- [x] Done task\n"
	os.WriteFile(filepath.Join(stratDir, "META-ROADMAP.md"), []byte(initial), 0o644)

	proposal := ResearchProposal{
		Title:       "evaluate new MCP spec",
		Description: "New MCP spec v2 has breaking changes",
		Domain:      "mcp",
		Complexity:  4,
	}

	if err := ProposeToMetaRoadmap(dir, proposal); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(stratDir, "META-ROADMAP.md"))
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, "Research-Derived Tasks") {
		t.Error("missing Research-Derived Tasks section")
	}
	if !strings.Contains(content, "evaluate new MCP spec") {
		t.Error("proposal not found in roadmap")
	}
}

func TestProposeToMetaRoadmapExistingSection(t *testing.T) {
	dir := t.TempDir()
	stratDir := filepath.Join(dir, "strategy")
	os.MkdirAll(stratDir, 0o755)

	// META-ROADMAP already has the section.
	initial := "# META-ROADMAP\n\n## Research-Derived Tasks\n\n- [ ] existing item\n"
	os.WriteFile(filepath.Join(stratDir, "META-ROADMAP.md"), []byte(initial), 0o644)

	proposal := ResearchProposal{
		Title:       "new item",
		Description: "something new",
		Domain:      "agents",
		Complexity:  3,
	}

	if err := ProposeToMetaRoadmap(dir, proposal); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(stratDir, "META-ROADMAP.md"))
	content := string(data)
	if !strings.Contains(content, "new item") {
		t.Error("new proposal not found")
	}
	if !strings.Contains(content, "existing item") {
		t.Error("existing item was removed")
	}
}
