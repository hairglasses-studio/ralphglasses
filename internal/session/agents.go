package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// agentDir returns the agent definition directory for a given provider.
func agentDir(repoPath string, provider Provider) string {
	switch provider {
	case ProviderGemini:
		return filepath.Join(repoPath, ".gemini", "agents")
	default:
		return filepath.Join(repoPath, ".claude", "agents")
	}
}

// ListAgents reads agent definitions from a repo for a given provider.
// Claude: .claude/agents/*.md, Gemini: .gemini/agents/*.md, Codex: AGENTS.md sections.
// If provider is empty, defaults to claude.
func ListAgents(repoPath string) ([]AgentDef, error) {
	return DiscoverAgents(repoPath, ProviderClaude)
}

// DiscoverAgents returns agent definitions for the specified provider.
func DiscoverAgents(repoPath string, provider Provider) ([]AgentDef, error) {
	if provider == "" {
		provider = ProviderClaude
	}

	if provider == ProviderCodex {
		return discoverCodexAgents(repoPath)
	}

	// Claude and Gemini both use .{provider}/agents/*.md
	dir := agentDir(repoPath, provider)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read agents dir: %w", err)
	}

	var agents []AgentDef
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}

		def := parseAgentMd(e.Name(), string(data))
		def.Provider = provider
		agents = append(agents, def)
	}
	return agents, nil
}

// discoverCodexAgents parses AGENTS.md in the repo root.
// Each ## heading becomes an agent name; content until next ## is the prompt.
func discoverCodexAgents(repoPath string) ([]AgentDef, error) {
	data, err := os.ReadFile(filepath.Join(repoPath, "AGENTS.md"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read AGENTS.md: %w", err)
	}

	return parseAgentsMd(string(data)), nil
}

// parseAgentsMd parses a Codex AGENTS.md file where each ## section is an agent.
func parseAgentsMd(content string) []AgentDef {
	var agents []AgentDef
	var current *AgentDef
	var body strings.Builder

	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "## ") {
			// Flush previous agent
			if current != nil {
				current.Prompt = strings.TrimSpace(body.String())
				agents = append(agents, *current)
			}
			name := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			current = &AgentDef{
				Name:     name,
				Provider: ProviderCodex,
			}
			body.Reset()
		} else if current != nil {
			body.WriteString(line)
			body.WriteString("\n")
		}
	}
	// Flush last agent
	if current != nil {
		current.Prompt = strings.TrimSpace(body.String())
		agents = append(agents, *current)
	}
	return agents
}

// WriteAgent writes an agent definition to the correct location for its provider.
func WriteAgent(repoPath string, def AgentDef) error {
	if def.Provider == ProviderCodex {
		return writeCodexAgent(repoPath, def)
	}

	dir := agentDir(repoPath, def.Provider)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}

	content := renderAgentMd(def)
	filename := def.Name + ".md"
	return os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644)
}

// writeCodexAgent appends or updates an agent section in AGENTS.md.
func writeCodexAgent(repoPath string, def AgentDef) error {
	agentsFile := filepath.Join(repoPath, "AGENTS.md")

	var existing string
	data, err := os.ReadFile(agentsFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read AGENTS.md: %w", err)
	}
	if err == nil {
		existing = string(data)
	}

	// Build the new section
	section := fmt.Sprintf("## %s\n\n%s\n", def.Name, def.Prompt)

	// Check if agent already exists — replace its section
	header := fmt.Sprintf("## %s", def.Name)
	if idx := strings.Index(existing, header); idx >= 0 {
		// Find the next ## or end of file
		rest := existing[idx+len(header):]
		nextIdx := strings.Index(rest, "\n## ")
		if nextIdx >= 0 {
			existing = existing[:idx] + section + rest[nextIdx+1:]
		} else {
			existing = existing[:idx] + section
		}
	} else {
		// Append
		if existing != "" && !strings.HasSuffix(existing, "\n") {
			existing += "\n"
		}
		existing += section
	}

	return os.WriteFile(agentsFile, []byte(existing), 0644)
}

// parseAgentMd parses a .claude/agents/*.md or .gemini/agents/*.md file into an AgentDef.
// Format: YAML frontmatter between --- fences, then markdown body.
func parseAgentMd(filename, content string) AgentDef {
	name := strings.TrimSuffix(filename, ".md")
	def := AgentDef{Name: name}

	// Split on --- fences
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		// No frontmatter, entire content is the prompt
		def.Prompt = strings.TrimSpace(content)
		return def
	}

	// Parse frontmatter (simple key: value parsing, no YAML dep)
	frontmatter := parts[1]
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "description":
			def.Description = value
		case "model":
			def.Model = value
		case "tools":
			def.Tools = parseYAMLList(value)
		case "maxTurns":
			fmt.Sscanf(value, "%d", &def.MaxTurns)
		}
	}

	def.Prompt = strings.TrimSpace(parts[2])
	return def
}

// parseYAMLList handles both inline [a, b] and returns a string slice.
func parseYAMLList(value string) []string {
	value = strings.Trim(value, "[]")
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// ComposeAgents creates a composite agent by merging multiple existing agent definitions.
// It concatenates prompts with section headers, uses the first agent's model/tools as defaults
// (overridable via the output AgentDef fields), and writes the result.
func ComposeAgents(repoPath string, agentNames []string, provider Provider, name string) (AgentDef, error) {
	if len(agentNames) == 0 {
		return AgentDef{}, fmt.Errorf("at least one agent name required")
	}
	if name == "" {
		return AgentDef{}, fmt.Errorf("composite agent name required")
	}
	if provider == "" {
		provider = ProviderClaude
	}

	agents, err := DiscoverAgents(repoPath, provider)
	if err != nil {
		return AgentDef{}, fmt.Errorf("discover agents: %w", err)
	}

	// Index by name
	byName := make(map[string]AgentDef, len(agents))
	for _, a := range agents {
		byName[a.Name] = a
	}

	// Resolve each requested agent
	var resolved []AgentDef
	for _, n := range agentNames {
		a, ok := byName[n]
		if !ok {
			return AgentDef{}, fmt.Errorf("agent not found: %s", n)
		}
		resolved = append(resolved, a)
	}

	// Build composite: first agent provides defaults
	composite := AgentDef{
		Name:        name,
		Provider:    provider,
		Description: fmt.Sprintf("Composite agent from: %s", strings.Join(agentNames, ", ")),
		Model:       resolved[0].Model,
		MaxTurns:    resolved[0].MaxTurns,
	}

	// Merge tools (deduplicated, order preserved)
	seen := make(map[string]bool)
	for _, a := range resolved {
		for _, t := range a.Tools {
			if !seen[t] {
				seen[t] = true
				composite.Tools = append(composite.Tools, t)
			}
		}
	}

	// Concatenate prompts with section headers
	var b strings.Builder
	for i, a := range resolved {
		if i > 0 {
			b.WriteString("\n\n---\n\n")
		}
		b.WriteString(fmt.Sprintf("## %s\n\n", a.Name))
		if a.Description != "" {
			b.WriteString(fmt.Sprintf("*%s*\n\n", a.Description))
		}
		b.WriteString(a.Prompt)
	}
	composite.Prompt = b.String()

	return composite, nil
}

// renderAgentMd produces a .claude/agents/*.md or .gemini/agents/*.md file content.
func renderAgentMd(def AgentDef) string {
	var b strings.Builder
	b.WriteString("---\n")

	if def.Description != "" {
		b.WriteString(fmt.Sprintf("description: %s\n", def.Description))
	}
	if def.Model != "" {
		b.WriteString(fmt.Sprintf("model: %s\n", def.Model))
	}
	if len(def.Tools) > 0 {
		b.WriteString(fmt.Sprintf("tools: [%s]\n", strings.Join(def.Tools, ", ")))
	}
	if def.MaxTurns > 0 {
		b.WriteString(fmt.Sprintf("maxTurns: %d\n", def.MaxTurns))
	}

	b.WriteString("---\n\n")
	b.WriteString(def.Prompt)
	b.WriteString("\n")

	return b.String()
}
