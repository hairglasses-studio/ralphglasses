package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ListAgents reads .claude/agents/*.md from a repo and returns agent definitions.
func ListAgents(repoPath string) ([]AgentDef, error) {
	agentsDir := filepath.Join(repoPath, ".claude", "agents")
	entries, err := os.ReadDir(agentsDir)
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

		data, err := os.ReadFile(filepath.Join(agentsDir, e.Name()))
		if err != nil {
			continue
		}

		def := parseAgentMd(e.Name(), string(data))
		agents = append(agents, def)
	}
	return agents, nil
}

// WriteAgent writes a .claude/agents/<name>.md file.
func WriteAgent(repoPath string, def AgentDef) error {
	agentsDir := filepath.Join(repoPath, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}

	content := renderAgentMd(def)
	filename := def.Name + ".md"
	return os.WriteFile(filepath.Join(agentsDir, filename), []byte(content), 0644)
}

// parseAgentMd parses a .claude/agents/*.md file into an AgentDef.
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

// renderAgentMd produces a .claude/agents/*.md file content.
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
