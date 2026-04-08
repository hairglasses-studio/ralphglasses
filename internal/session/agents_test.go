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
		Provider:    ProviderClaude,
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
	agents, err := DiscoverAgents(root, ProviderClaude)
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

	if err := WriteAgent(root, AgentDef{Name: "a", Provider: ProviderCodex, Prompt: "hello"}); err != nil {
		t.Fatal(err)
	}

	composite, err := ComposeAgents(root, []string{"a"}, "", "comp")
	if err != nil {
		t.Fatalf("ComposeAgents: %v", err)
	}
	if composite.Provider != ProviderCodex {
		t.Errorf("Provider = %q, want codex (default)", composite.Provider)
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

func TestParseAgentsMd(t *testing.T) {
	content := `## Reviewer
Review all code changes.

## Tester
Run the test suite and report failures.
`
	agents := parseAgentsMd(content)
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
	if agents[0].Name != "Reviewer" {
		t.Errorf("agent[0].Name = %q", agents[0].Name)
	}
	if agents[0].Prompt != "Review all code changes." {
		t.Errorf("agent[0].Prompt = %q", agents[0].Prompt)
	}
	if agents[0].Provider != ProviderCodex {
		t.Errorf("agent[0].Provider = %q, want codex", agents[0].Provider)
	}
	if agents[1].Name != "Tester" {
		t.Errorf("agent[1].Name = %q", agents[1].Name)
	}
}

func TestParseAgentsMd_Empty(t *testing.T) {
	agents := parseAgentsMd("")
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestDiscoverCodexAgents(t *testing.T) {
	root := t.TempDir()

	// No .codex/agents or AGENTS.md — returns nil
	agents, err := discoverCodexAgents(root)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if agents != nil {
		t.Errorf("expected nil agents, got %v", agents)
	}

	// Create .codex/agents/reviewer.toml
	dir := filepath.Join(root, ".codex", "agents")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `name = "CodeReview"
description = "Review code changes"
model = "gpt-5.4"
developer_instructions = """
Review all pull requests.
"""
`
	if err := os.WriteFile(filepath.Join(dir, "reviewer.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	agents, err = discoverCodexAgents(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Name != "CodeReview" {
		t.Errorf("name = %q, want CodeReview", agents[0].Name)
	}
	if agents[0].Prompt != "Review all pull requests." {
		t.Errorf("prompt = %q", agents[0].Prompt)
	}
}

func TestDiscoverCodexAgents_LegacyAGENTSFallback(t *testing.T) {
	root := t.TempDir()
	content := "## CodeReview\nReview all pull requests.\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	agents, err := discoverCodexAgents(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
}

func TestWriteCodexAgent(t *testing.T) {
	root := t.TempDir()

	def := AgentDef{
		Name:     "TestAgent",
		Provider: ProviderCodex,
		Prompt:   "Do test things",
	}

	if err := writeCodexAgent(root, def); err != nil {
		t.Fatalf("writeCodexAgent: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".codex", "agents", "TestAgent.toml"))
	if err != nil {
		t.Fatalf("read Codex agent file: %v", err)
	}
	if !contains(string(data), `name = "TestAgent"`) {
		t.Error("Codex agent file missing name")
	}
	if !contains(string(data), "developer_instructions = \"\"\"") {
		t.Error("Codex agent file missing developer instructions block")
	}

	// Update existing agent
	def.Prompt = "Updated prompt"
	if err := writeCodexAgent(root, def); err != nil {
		t.Fatalf("writeCodexAgent update: %v", err)
	}

	data, err = os.ReadFile(filepath.Join(root, ".codex", "agents", "TestAgent.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(data), "Updated prompt") {
		t.Error("Codex agent file not updated with new prompt")
	}
}

func TestDiscoverAgents_GeminiCommands(t *testing.T) {
	root := t.TempDir()

	dir := filepath.Join(root, ".gemini", "commands")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "description = \"Gemini command\"\nprompt = \"\"\"\nDo gemini things.\n\"\"\"\n"
	if err := os.WriteFile(filepath.Join(dir, "gem-agent.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	agents, err := DiscoverAgents(root, ProviderGemini)
	if err != nil {
		t.Fatalf("DiscoverAgents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Provider != ProviderGemini {
		t.Errorf("provider = %q, want gemini", agents[0].Provider)
	}
	if agents[0].Description != "Gemini command" {
		t.Errorf("description = %q, want Gemini command", agents[0].Description)
	}
	if agents[0].Prompt != "Do gemini things." {
		t.Errorf("prompt = %q, want command prompt", agents[0].Prompt)
	}
}

func TestWriteGeminiCommand(t *testing.T) {
	root := t.TempDir()

	def := AgentDef{
		Name:        "triage",
		Provider:    ProviderGemini,
		Description: "Gemini triage command",
		Prompt:      "Summarize the current repo state.",
	}

	if err := WriteAgent(root, def); err != nil {
		t.Fatalf("WriteAgent(gemini): %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".gemini", "commands", "triage.toml"))
	if err != nil {
		t.Fatalf("read Gemini command file: %v", err)
	}
	content := string(data)
	if !contains(content, `description = "Gemini triage command"`) {
		t.Error("Gemini command file missing description")
	}
	if !contains(content, "prompt = \"\"\"") {
		t.Error("Gemini command file missing prompt block")
	}
	if !contains(content, "Summarize the current repo state.") {
		t.Error("Gemini command file missing prompt")
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
