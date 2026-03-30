package knowledge

import (
	"fmt"
	"strings"
)

// Injector is a pipeline stage that injects relevant codebase context
// from a knowledge graph into prompts being enhanced.
type Injector struct {
	graph    *Graph
	maxNodes int
	// MaxTokens is the approximate max token budget for injected context.
	// Uses the rough estimate of 4 chars per token.
	maxTokens int
}

// NewInjector creates an injector backed by the given graph.
// maxNodes controls how many related nodes to retrieve (default 10).
// maxTokens controls the approximate token budget for injected context (default 500).
func NewInjector(g *Graph, maxNodes, maxTokens int) *Injector {
	if maxNodes <= 0 {
		maxNodes = 10
	}
	if maxTokens <= 0 {
		maxTokens = 500
	}
	return &Injector{
		graph:     g,
		maxNodes:  maxNodes,
		maxTokens: maxTokens,
	}
}

// InjectContext analyzes the prompt, finds relevant code entities from the
// knowledge graph, and returns the prompt with a structured context block
// prepended. Returns the modified prompt and a list of improvements applied.
//
// If no relevant context is found, the original prompt is returned unchanged.
func (inj *Injector) InjectContext(prompt string) (string, []string) {
	if inj.graph == nil || inj.graph.NodeCount() == 0 {
		return prompt, nil
	}

	nodes := inj.graph.RelatedContext(prompt, inj.maxNodes)
	if len(nodes) == 0 {
		return prompt, nil
	}

	block := inj.formatContextBlock(nodes)
	if block == "" {
		return prompt, nil
	}

	// Trim to token budget
	block = inj.trimToTokenBudget(block)

	enhanced := block + "\n\n" + prompt
	improvements := []string{
		fmt.Sprintf("Injected codebase context: %d relevant entities from knowledge graph", len(nodes)),
	}
	return enhanced, improvements
}

// formatContextBlock renders relevant nodes as a structured XML context block.
func (inj *Injector) formatContextBlock(nodes []Node) string {
	if len(nodes) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<codebase_context>\n")

	// Group nodes by kind for structured output
	packages := filterByKind(nodes, KindPackage)
	types := filterByKind(nodes, KindType)
	functions := filterByKind(nodes, KindFunction)
	files := filterByKind(nodes, KindFile)

	if len(packages) > 0 {
		b.WriteString("  <packages>\n")
		for _, n := range packages {
			name := n.Metadata["name"]
			if name == "" {
				name = n.ID
			}
			path := n.Metadata["path"]
			if path != "" {
				fmt.Fprintf(&b, "    <package name=%q path=%q />\n", name, path)
			} else {
				fmt.Fprintf(&b, "    <package name=%q />\n", name)
			}
		}
		b.WriteString("  </packages>\n")
	}

	if len(types) > 0 {
		b.WriteString("  <types>\n")
		for _, n := range types {
			name := n.Metadata["name"]
			kind := n.Metadata["kind"]
			fields := n.Metadata["fields"]
			methods := n.Metadata["methods"]

			fmt.Fprintf(&b, "    <type name=%q kind=%q", name, kind)
			if fields != "" {
				fmt.Fprintf(&b, " fields=%q", fields)
			}
			if methods != "" {
				fmt.Fprintf(&b, " methods=%q", methods)
			}
			b.WriteString(" />\n")
		}
		b.WriteString("  </types>\n")
	}

	if len(functions) > 0 {
		b.WriteString("  <functions>\n")
		for _, n := range functions {
			sig := n.Metadata["signature"]
			if sig != "" {
				fmt.Fprintf(&b, "    <function signature=%q", sig)
			} else {
				name := n.Metadata["name"]
				fmt.Fprintf(&b, "    <function name=%q", name)
			}
			if recv := n.Metadata["receiver"]; recv != "" {
				fmt.Fprintf(&b, " receiver=%q", recv)
			}
			b.WriteString(" />\n")
		}
		b.WriteString("  </functions>\n")
	}

	if len(files) > 0 {
		b.WriteString("  <files>\n")
		for _, n := range files {
			path := n.Metadata["path"]
			pkg := n.Metadata["package"]
			fmt.Fprintf(&b, "    <file path=%q package=%q />\n", path, pkg)
		}
		b.WriteString("  </files>\n")
	}

	b.WriteString("</codebase_context>")
	return b.String()
}

// FormatContextBlockMarkdown renders relevant nodes as markdown sections.
// Used for Gemini/OpenAI targets that prefer markdown over XML.
func (inj *Injector) FormatContextBlockMarkdown(nodes []Node) string {
	if len(nodes) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Codebase Context\n\n")

	packages := filterByKind(nodes, KindPackage)
	types := filterByKind(nodes, KindType)
	functions := filterByKind(nodes, KindFunction)

	if len(packages) > 0 {
		b.WriteString("**Packages:**\n")
		for _, n := range packages {
			name := n.Metadata["name"]
			if path := n.Metadata["path"]; path != "" {
				fmt.Fprintf(&b, "- `%s` (%s)\n", name, path)
			} else {
				fmt.Fprintf(&b, "- `%s`\n", name)
			}
		}
		b.WriteString("\n")
	}

	if len(types) > 0 {
		b.WriteString("**Types:**\n")
		for _, n := range types {
			name := n.Metadata["name"]
			kind := n.Metadata["kind"]
			fmt.Fprintf(&b, "- `%s` (%s)", name, kind)
			if fields := n.Metadata["fields"]; fields != "" {
				fmt.Fprintf(&b, " — fields: %s", fields)
			}
			if methods := n.Metadata["methods"]; methods != "" {
				fmt.Fprintf(&b, " — methods: %s", methods)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(functions) > 0 {
		b.WriteString("**Functions:**\n")
		for _, n := range functions {
			if sig := n.Metadata["signature"]; sig != "" {
				fmt.Fprintf(&b, "- `%s`\n", sig)
			} else {
				fmt.Fprintf(&b, "- `%s`\n", n.Metadata["name"])
			}
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// trimToTokenBudget trims the context block to fit within the token budget.
// Uses a rough estimate of 4 characters per token.
func (inj *Injector) trimToTokenBudget(block string) string {
	maxChars := inj.maxTokens * 4
	if len(block) <= maxChars {
		return block
	}
	// Truncate at a line boundary
	truncated := block[:maxChars]
	if idx := strings.LastIndex(truncated, "\n"); idx > 0 {
		truncated = truncated[:idx]
	}
	// Close any open XML tags
	if strings.Contains(truncated, "<codebase_context>") && !strings.Contains(truncated, "</codebase_context>") {
		truncated += "\n</codebase_context>"
	}
	return truncated
}

// filterByKind returns nodes matching the given kind.
func filterByKind(nodes []Node, kind NodeKind) []Node {
	var result []Node
	for _, n := range nodes {
		if n.Kind == kind {
			result = append(result, n)
		}
	}
	return result
}
