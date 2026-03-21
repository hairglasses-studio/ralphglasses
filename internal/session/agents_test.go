package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndListAgents(t *testing.T) {
	root := t.TempDir()

	def := AgentDef{
		Name:        "reviewer",
		Description: "Code reviewer agent",
		Model:       "sonnet",
		Tools:       []string{"Read", "Grep", "Glob"},
		MaxTurns:    10,
		Prompt:      "You are a code reviewer. Review all changed files and provide feedback.",
	}

	if err := WriteAgent(root, def); err != nil {
		t.Fatalf("WriteAgent: %v", err)
	}

	// Verify file exists
	agentPath := filepath.Join(root, ".claude", "agents", "reviewer.md")
	data, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("read agent file: %v", err)
	}

	content := string(data)
	if !contains(content, "description: Code reviewer agent") {
		t.Error("missing description in frontmatter")
	}
	if !contains(content, "model: sonnet") {
		t.Error("missing model in frontmatter")
	}
	if !contains(content, "tools: [Read, Grep, Glob]") {
		t.Error("missing tools in frontmatter")
	}
	if !contains(content, "maxTurns: 10") {
		t.Error("missing maxTurns in frontmatter")
	}
	if !contains(content, "You are a code reviewer") {
		t.Error("missing prompt body")
	}

	// List agents
	agents, err := ListAgents(root)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Name != "reviewer" {
		t.Errorf("name = %q, want reviewer", agents[0].Name)
	}
	if agents[0].Description != "Code reviewer agent" {
		t.Errorf("description = %q, want 'Code reviewer agent'", agents[0].Description)
	}
	if agents[0].Model != "sonnet" {
		t.Errorf("model = %q, want sonnet", agents[0].Model)
	}
	if len(agents[0].Tools) != 3 {
		t.Errorf("tools count = %d, want 3", len(agents[0].Tools))
	}
	if agents[0].MaxTurns != 10 {
		t.Errorf("maxTurns = %d, want 10", agents[0].MaxTurns)
	}
}

func TestListAgentsNoDir(t *testing.T) {
	root := t.TempDir()
	agents, err := ListAgents(root)
	if err != nil {
		t.Fatalf("ListAgents on empty dir: %v", err)
	}
	if agents != nil {
		t.Errorf("expected nil, got %d agents", len(agents))
	}
}

func TestComposeAgents(t *testing.T) {
	root := t.TempDir()

	// Write two agents with overlapping tools
	a := AgentDef{
		Name:        "tester",
		Description: "Runs tests",
		Model:       "sonnet",
		Tools:       []string{"Bash", "Read", "Grep"},
		MaxTurns:    10,
		Prompt:      "You run tests and report failures.",
	}
	b := AgentDef{
		Name:        "documenter",
		Description: "Writes docs",
		Model:       "haiku",
		Tools:       []string{"Read", "Write"},
		MaxTurns:    5,
		Prompt:      "You write documentation.",
	}

	if err := WriteAgent(root, a); err != nil {
		t.Fatalf("WriteAgent(tester): %v", err)
	}
	if err := WriteAgent(root, b); err != nil {
		t.Fatalf("WriteAgent(documenter): %v", err)
	}

	composite, err := ComposeAgents(root, []string{"tester", "documenter"}, ProviderClaude, "test-doc")
	if err != nil {
		t.Fatalf("ComposeAgents: %v", err)
	}

	// Name and provider
	if composite.Name != "test-doc" {
		t.Errorf("Name = %q, want test-doc", composite.Name)
	}
	if composite.Provider != ProviderClaude {
		t.Errorf("Provider = %q, want claude", composite.Provider)
	}

	// Model and MaxTurns from first agent
	if composite.Model != "sonnet" {
		t.Errorf("Model = %q, want sonnet (from first agent)", composite.Model)
	}
	if composite.MaxTurns != 10 {
		t.Errorf("MaxTurns = %d, want 10 (from first agent)", composite.MaxTurns)
	}

	// Tools deduplicated: Bash, Read, Grep, Write (Read not duplicated)
	if len(composite.Tools) != 4 {
		t.Errorf("Tools count = %d, want 4 (deduplicated), got %v", len(composite.Tools), composite.Tools)
	}
	toolSet := make(map[string]bool)
	for _, tool := range composite.Tools {
		toolSet[tool] = true
	}
	for _, expected := range []string{"Bash", "Read", "Grep", "Write"} {
		if !toolSet[expected] {
			t.Errorf("missing tool %q in composite", expected)
		}
	}

	// Prompt contains both sections
	if !contains(composite.Prompt, "## tester") {
		t.Error("prompt missing tester section header")
	}
	if !contains(composite.Prompt, "## documenter") {
		t.Error("prompt missing documenter section header")
	}
	if !contains(composite.Prompt, "You run tests") {
		t.Error("prompt missing tester prompt content")
	}
	if !contains(composite.Prompt, "You write documentation") {
		t.Error("prompt missing documenter prompt content")
	}
	if !contains(composite.Prompt, "---") {
		t.Error("prompt missing section separator")
	}

	// Description mentions both agents
	if !contains(composite.Description, "tester") || !contains(composite.Description, "documenter") {
		t.Errorf("Description = %q, want mention of both agents", composite.Description)
	}
}

func TestComposeAgents_EmptyNames(t *testing.T) {
	root := t.TempDir()
	_, err := ComposeAgents(root, []string{}, ProviderClaude, "composite")
	if err == nil {
		t.Fatal("expected error for empty agent names")
	}
}

func TestComposeAgents_EmptyName(t *testing.T) {
	root := t.TempDir()
	_, err := ComposeAgents(root, []string{"foo"}, ProviderClaude, "")
	if err == nil {
		t.Fatal("expected error for empty composite name")
	}
}

func TestComposeAgents_NotFound(t *testing.T) {
	root := t.TempDir()
	_, err := ComposeAgents(root, []string{"nonexistent"}, ProviderClaude, "composite")
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestComposeAgents_DefaultProvider(t *testing.T) {
	root := t.TempDir()

	if err := WriteAgent(root, AgentDef{Name: "a", Prompt: "hello"}); err != nil {
		t.Fatal(err)
	}

	composite, err := ComposeAgents(root, []string{"a"}, "", "comp")
	if err != nil {
		t.Fatalf("ComposeAgents: %v", err)
	}
	if composite.Provider != ProviderClaude {
		t.Errorf("Provider = %q, want claude (default)", composite.Provider)
	}
}

func TestParseAgentMdNoFrontmatter(t *testing.T) {
	def := parseAgentMd("simple.md", "Just a simple prompt with no frontmatter.")
	if def.Name != "simple" {
		t.Errorf("name = %q, want simple", def.Name)
	}
	if def.Prompt != "Just a simple prompt with no frontmatter." {
		t.Errorf("prompt = %q", def.Prompt)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
