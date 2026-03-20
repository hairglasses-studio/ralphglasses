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
