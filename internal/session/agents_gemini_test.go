package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverAgentsGeminiPrefersNativeMarkdownAndKeepsLegacyFallback(t *testing.T) {
	repo := t.TempDir()
	nativeDir := filepath.Join(repo, ".gemini", "agents")
	legacyDir := filepath.Join(repo, ".gemini", "commands")
	if err := os.MkdirAll(nativeDir, 0o755); err != nil {
		t.Fatalf("mkdir native agents dir: %v", err)
	}
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy commands dir: %v", err)
	}

	native := `---
description: Native docs researcher
model: gemini-2.5-pro
tools: [read_file, search_repo]
maxTurns: 7
---

Native prompt
`
	legacyDuplicate := "description = \"Legacy docs researcher\"\nprompt = \"\"\"\nLegacy prompt\n\"\"\"\n"
	legacyOnly := "description = \"Legacy task distributor\"\nprompt = \"\"\"\nLegacy distributor prompt\n\"\"\"\n"

	if err := os.WriteFile(filepath.Join(nativeDir, "docs_researcher.md"), []byte(native), 0o644); err != nil {
		t.Fatalf("write native agent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "docs-researcher.toml"), []byte(legacyDuplicate), 0o644); err != nil {
		t.Fatalf("write duplicate legacy command: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "task-distributor.toml"), []byte(legacyOnly), 0o644); err != nil {
		t.Fatalf("write unique legacy command: %v", err)
	}

	agents, err := DiscoverAgents(repo, ProviderGemini)
	if err != nil {
		t.Fatalf("DiscoverAgents returned error: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 Gemini agents, got %d", len(agents))
	}

	docs := findGeminiAgentByName(agents, "docs_researcher")
	if docs == nil {
		t.Fatalf("expected native docs_researcher agent in %#v", agents)
	}
	if docs.Description != "Native docs researcher" {
		t.Fatalf("expected native description, got %q", docs.Description)
	}
	if strings.TrimSpace(docs.Prompt) != "Native prompt" {
		t.Fatalf("expected native prompt, got %q", docs.Prompt)
	}
	if docs.Model != "gemini-2.5-pro" {
		t.Fatalf("expected native model, got %q", docs.Model)
	}
	if docs.MaxTurns != 7 {
		t.Fatalf("expected native max turns, got %d", docs.MaxTurns)
	}
	if len(docs.Tools) != 2 || docs.Tools[0] != "read_file" || docs.Tools[1] != "search_repo" {
		t.Fatalf("expected native tools, got %#v", docs.Tools)
	}
	if docs.Provider != ProviderGemini {
		t.Fatalf("expected Gemini provider, got %q", docs.Provider)
	}

	legacy := findGeminiAgentByName(agents, "task-distributor")
	if legacy == nil {
		t.Fatalf("expected legacy task-distributor agent in %#v", agents)
	}
	if legacy.Description != "Legacy task distributor" {
		t.Fatalf("expected legacy description, got %q", legacy.Description)
	}
	if strings.TrimSpace(legacy.Prompt) != "Legacy distributor prompt" {
		t.Fatalf("expected legacy prompt, got %q", legacy.Prompt)
	}
}

func TestWriteAgentGeminiWritesMarkdownSurface(t *testing.T) {
	repo := t.TempDir()
	def := AgentDef{
		Name:        "docs_researcher",
		Provider:    ProviderGemini,
		Description: "Docs role",
		Prompt:      "Research docs.",
	}

	if err := WriteAgent(repo, def); err != nil {
		t.Fatalf("WriteAgent returned error: %v", err)
	}

	path := filepath.Join(repo, ".gemini", "agents", "docs_researcher.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written Gemini agent: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "description: Docs role") {
		t.Fatalf("expected markdown frontmatter description, got %q", content)
	}
	if !strings.Contains(content, "Research docs.") {
		t.Fatalf("expected markdown prompt body, got %q", content)
	}

	legacyPath := filepath.Join(repo, ".gemini", "commands", "docs_researcher.toml")
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("expected no legacy Gemini command file, stat err=%v", err)
	}
}

func findGeminiAgentByName(agents []AgentDef, name string) *AgentDef {
	for i := range agents {
		if agents[i].Name == name {
			return &agents[i]
		}
	}
	return nil
}
