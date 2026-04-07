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
	case ProviderCodex:
		return filepath.Join(repoPath, ".codex", "agents")
	case ProviderGemini:
		return filepath.Join(repoPath, ".gemini", "commands")
	default:
		return filepath.Join(repoPath, ".claude", "agents")
	}
}

// ValidateLaunchAgent checks that an agent name is syntactically valid for the given provider.
// An empty agent name is always valid (uses the default).
func ValidateLaunchAgent(provider Provider, agent string) error {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return nil
	}
	if strings.ContainsAny(agent, "/\\:*?\"<>|") {
		return fmt.Errorf("agent name %q contains invalid characters", agent)
	}
	return nil
}

// ListAgents reads reusable provider role definitions from a repo for a given provider.
// Claude: .claude/agents/*.md, Gemini: .gemini/commands/*.toml, Codex: .codex/agents/*.toml.
// If provider is empty, defaults to the primary provider.
func ListAgents(repoPath string) ([]AgentDef, error) {
	return DiscoverAgents(repoPath, DefaultPrimaryProvider())
}

// DiscoverAgents returns agent definitions for the specified provider.
func DiscoverAgents(repoPath string, provider Provider) ([]AgentDef, error) {
	if provider == "" {
		provider = DefaultPrimaryProvider()
	}

	dir := agentDir(repoPath, provider)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		if provider == ProviderCodex {
			return discoverLegacyCodexAgents(repoPath)
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read agents dir: %w", err)
	}

	var agents []AgentDef
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if (provider == ProviderCodex || provider == ProviderGemini) && !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		if provider != ProviderCodex && provider != ProviderGemini && !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}

		var def AgentDef
		if provider == ProviderCodex {
			def = parseCodexAgentToml(e.Name(), string(data))
		} else if provider == ProviderGemini {
			def = parseGeminiCommandToml(e.Name(), string(data))
		} else {
			def = parseAgentMd(e.Name(), string(data))
		}
		def.Provider = provider
		agents = append(agents, def)
	}
	return agents, nil
}

// discoverLegacyCodexAgents parses legacy AGENTS.md custom-agent sections.
// This is retained as a fallback for repos created before Codex adopted
// .codex/agents/*.toml for project-scoped custom agents.
func discoverCodexAgents(repoPath string) ([]AgentDef, error) {
	dir := agentDir(repoPath, ProviderCodex)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return discoverLegacyCodexAgents(repoPath)
	}
	if err != nil {
		return nil, fmt.Errorf("read codex agents dir: %w", err)
	}

	var agents []AgentDef
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		def := parseCodexAgentToml(e.Name(), string(data))
		def.Provider = ProviderCodex
		agents = append(agents, def)
	}
	return agents, nil
}

func discoverLegacyCodexAgents(repoPath string) ([]AgentDef, error) {
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

	for line := range strings.SplitSeq(content, "\n") {
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
	if def.Provider == ProviderGemini {
		return writeGeminiCommand(repoPath, def)
	}

	dir := agentDir(repoPath, def.Provider)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}

	content := renderAgentMd(def)
	filename := def.Name + ".md"
	return os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644)
}

// writeCodexAgent writes a Codex custom agent TOML file under .codex/agents/.
func writeCodexAgent(repoPath string, def AgentDef) error {
	dir := agentDir(repoPath, ProviderCodex)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create codex agents dir: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, def.Name+".toml"), []byte(renderCodexAgentToml(def)), 0644)
}

// writeGeminiCommand writes a Gemini custom command TOML file under .gemini/commands/.
func writeGeminiCommand(repoPath string, def AgentDef) error {
	dir := agentDir(repoPath, ProviderGemini)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create gemini commands dir: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, def.Name+".toml"), []byte(renderGeminiCommandToml(def)), 0644)
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
	for line := range strings.SplitSeq(frontmatter, "\n") {
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
			_, _ = fmt.Sscanf(value, "%d", &def.MaxTurns)
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

func parseCodexAgentToml(filename, content string) AgentDef {
	name := strings.TrimSuffix(filename, ".toml")
	def := AgentDef{Name: name, Provider: ProviderCodex}

	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "name = "):
			def.Name = parseQuotedTomlValue(trimmed)
		case strings.HasPrefix(trimmed, "description = "):
			def.Description = parseQuotedTomlValue(trimmed)
		case strings.HasPrefix(trimmed, "model = "):
			def.Model = parseQuotedTomlValue(trimmed)
		case strings.HasPrefix(trimmed, "# ralphglasses_tools = "):
			def.Tools = parseTomlStringArray(strings.TrimSpace(strings.TrimPrefix(trimmed, "# ralphglasses_tools = ")))
		case strings.HasPrefix(trimmed, "# ralphglasses_max_turns = "):
			_, _ = fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(trimmed, "# ralphglasses_max_turns = ")), "%d", &def.MaxTurns)
		}
	}

	if _, after, ok := strings.Cut(content, "developer_instructions = \"\"\""); ok {
		body := after
		if before, _, ok := strings.Cut(body, "\"\"\""); ok {
			def.Prompt = strings.TrimSpace(before)
		}
	}

	return def
}

func parseGeminiCommandToml(filename, content string) AgentDef {
	name := strings.TrimSuffix(filename, ".toml")
	def := AgentDef{Name: name, Provider: ProviderGemini}

	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "description = "):
			def.Description = parseQuotedTomlValue(trimmed)
		}
	}

	if _, after, ok := strings.Cut(content, "prompt = \"\"\""); ok {
		body := after
		if before, _, ok := strings.Cut(body, "\"\"\""); ok {
			def.Prompt = strings.TrimSpace(before)
		}
	}

	return def
}

func renderCodexAgentToml(def AgentDef) string {
	var b strings.Builder
	name := def.Name
	if name == "" {
		name = "agent"
	}
	description := def.Description
	if description == "" {
		description = "Custom Codex agent exported by ralphglasses."
	}

	b.WriteString(fmt.Sprintf("name = %q\n", name))
	b.WriteString(fmt.Sprintf("description = %q\n", description))
	if def.Model != "" {
		b.WriteString(fmt.Sprintf("model = %q\n", def.Model))
	}
	if len(def.Tools) > 0 {
		b.WriteString("# ralphglasses_tools = [")
		for i, tool := range def.Tools {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(fmt.Sprintf("%q", tool))
		}
		b.WriteString("]\n")
	}
	if def.MaxTurns > 0 {
		b.WriteString(fmt.Sprintf("# ralphglasses_max_turns = %d\n", def.MaxTurns))
	}
	b.WriteString("developer_instructions = \"\"\"\n")
	b.WriteString(strings.TrimSpace(def.Prompt))
	b.WriteString("\n\"\"\"\n")
	return b.String()
}

func renderGeminiCommandToml(def AgentDef) string {
	var b strings.Builder
	description := def.Description
	if description == "" {
		description = "Custom Gemini command exported by ralphglasses."
	}
	b.WriteString(fmt.Sprintf("description = %q\n", description))
	b.WriteString("prompt = \"\"\"\n")
	b.WriteString(strings.TrimSpace(def.Prompt))
	b.WriteString("\n\"\"\"\n")
	return b.String()
}

func parseQuotedTomlValue(line string) string {
	_, value, ok := strings.Cut(line, "=")
	if !ok {
		return ""
	}
	return strings.Trim(strings.TrimSpace(value), "\"")
}

func parseTomlStringArray(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.Trim(strings.TrimSpace(part), "\"")
		if item != "" {
			out = append(out, item)
		}
	}
	return out
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
		provider = DefaultPrimaryProvider()
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
