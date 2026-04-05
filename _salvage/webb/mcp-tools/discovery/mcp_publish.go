package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
)

// MCPServerManifest represents the manifest format for MCP Registry publication
// See: https://modelcontextprotocol.io/development/registry
type MCPServerManifest struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Author      string            `json:"author"`
	License     string            `json:"license"`
	Repository  string            `json:"repository"`
	Homepage    string            `json:"homepage,omitempty"`
	Keywords    []string          `json:"keywords"`
	Categories  []string          `json:"categories"`
	Transports  []string          `json:"transports"` // stdio, http, websocket
	Tools       []MCPToolSummary  `json:"tools"`
	Resources   []MCPResourceInfo `json:"resources,omitempty"`
	Prompts     []MCPPromptInfo   `json:"prompts,omitempty"`
}

// MCPToolSummary is a brief summary of a tool for the registry
type MCPToolSummary struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// MCPResourceInfo describes a resource exposed by the server
type MCPResourceInfo struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType,omitempty"`
}

// MCPPromptInfo describes a prompt template
type MCPPromptInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// WellKnownMCPInfo represents the .well-known/mcp.json format
// See: https://modelcontextprotocol.io/development/roadmap
type WellKnownMCPInfo struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
	ToolCount    int      `json:"tool_count"`
	Transports   []string `json:"transports"`
	DocsURL      string   `json:"docs_url,omitempty"`
	RegistryURL  string   `json:"registry_url,omitempty"`
}

// handleMCPRegistryPublish generates a registry submission package
func handleMCPRegistryPublish(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	outputDir := req.GetString("output_dir", "")
	dryRun := req.GetBool("dry_run", true)

	if outputDir == "" {
		homeDir, _ := os.UserHomeDir()
		outputDir = filepath.Join(homeDir, "webb-mcp-registry")
	}

	// Get tool registry
	registry := tools.GetRegistry()
	allTools := registry.GetAllToolDefinitions()

	// Build manifest
	manifest := MCPServerManifest{
		Name:        "webb",
		Version:     "127.0.0", // Update this from version tracking
		Description: "Acme Platform Operations CLI - unified MCP server for cloud infrastructure, incidents, tickets, observability, and autonomous operations",
		Author:      "Acme AI",
		License:     "MIT",
		Repository:  "https://github.com/hairglasses-studio/webb",
		Keywords: []string{
			"platform-ops", "infrastructure", "kubernetes", "incidents", "observability",
			"alerts", "tickets", "aws", "database", "autonomous-agents",
		},
		Categories: []string{
			"monitoring", "infrastructure", "kubernetes", "database", "ticketing",
			"incidents", "observability", "aws", "research",
		},
		Transports: []string{"stdio"},
		Tools:      make([]MCPToolSummary, 0, len(allTools)),
	}

	// Add tools (limit to first 100 for registry summary)
	for i, td := range allTools {
		if i >= 100 {
			break
		}
		manifest.Tools = append(manifest.Tools, MCPToolSummary{
			Name:        td.Tool.Name,
			Description: truncateDesc(td.Tool.Description, 200),
			Category:    td.Category,
			Tags:        td.Tags,
		})
	}

	var sb strings.Builder
	sb.WriteString("# MCP Registry Publication Package\n\n")
	sb.WriteString(fmt.Sprintf("**Server:** %s v%s\n", manifest.Name, manifest.Version))
	sb.WriteString(fmt.Sprintf("**Total Tools:** %d (showing %d in manifest)\n", len(allTools), len(manifest.Tools)))
	sb.WriteString(fmt.Sprintf("**Categories:** %s\n\n", strings.Join(manifest.Categories, ", ")))

	if dryRun {
		sb.WriteString("## Dry Run Mode\n\n")
		sb.WriteString("Set `dry_run=false` to generate files.\n\n")

		// Show manifest preview
		sb.WriteString("### Manifest Preview\n\n")
		sb.WriteString("```json\n")
		manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")
		sb.WriteString(string(manifestJSON[:min(2000, len(manifestJSON))]))
		if len(manifestJSON) > 2000 {
			sb.WriteString("\n... (truncated)")
		}
		sb.WriteString("\n```\n")
	} else {
		// Create output directory
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return tools.ErrorResult(fmt.Errorf("failed to create output dir: %w", err)), nil
		}

		// Write manifest.json
		manifestPath := filepath.Join(outputDir, "manifest.json")
		manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")
		if err := os.WriteFile(manifestPath, manifestJSON, 0644); err != nil {
			return tools.ErrorResult(fmt.Errorf("failed to write manifest: %w", err)), nil
		}
		sb.WriteString(fmt.Sprintf("- **manifest.json** written to: %s\n", manifestPath))

		// Write README.md for registry
		readmePath := filepath.Join(outputDir, "README.md")
		readme := generateRegistryReadme(manifest, allTools)
		if err := os.WriteFile(readmePath, []byte(readme), 0644); err != nil {
			return tools.ErrorResult(fmt.Errorf("failed to write README: %w", err)), nil
		}
		sb.WriteString(fmt.Sprintf("- **README.md** written to: %s\n", readmePath))

		// Write .well-known/mcp.json
		wellKnownDir := filepath.Join(outputDir, ".well-known")
		os.MkdirAll(wellKnownDir, 0755)
		wellKnownPath := filepath.Join(wellKnownDir, "mcp.json")
		wellKnown := WellKnownMCPInfo{
			Name:        manifest.Name,
			Version:     manifest.Version,
			Description: manifest.Description,
			Capabilities: []string{
				"tools", "resources", "progressive-disclosure",
			},
			ToolCount:  len(allTools),
			Transports: manifest.Transports,
			DocsURL:    "https://github.com/hairglasses-studio/webb#readme",
		}
		wellKnownJSON, _ := json.MarshalIndent(wellKnown, "", "  ")
		if err := os.WriteFile(wellKnownPath, wellKnownJSON, 0644); err != nil {
			return tools.ErrorResult(fmt.Errorf("failed to write .well-known: %w", err)), nil
		}
		sb.WriteString(fmt.Sprintf("- **.well-known/mcp.json** written to: %s\n", wellKnownPath))

		sb.WriteString("\n## Next Steps\n\n")
		sb.WriteString("1. Review generated files\n")
		sb.WriteString("2. Submit to MCP Registry: https://registry.modelcontextprotocol.io\n")
		sb.WriteString("3. Host .well-known/mcp.json at your server's root URL\n")
	}

	return tools.TextResult(sb.String()), nil
}

// handleWellKnownGenerate generates the .well-known/mcp.json file
func handleWellKnownGenerate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	registry := tools.GetRegistry()
	allTools := registry.GetAllToolDefinitions()

	wellKnown := WellKnownMCPInfo{
		Name:        "webb",
		Version:     "127.0.0",
		Description: "Acme Platform Operations CLI - unified MCP server",
		Capabilities: []string{
			"tools",
			"resources",
			"progressive-disclosure",
			"lazy-loading",
		},
		ToolCount:  len(allTools),
		Transports: []string{"stdio"},
		DocsURL:    "https://github.com/hairglasses-studio/webb",
	}

	output, _ := json.MarshalIndent(wellKnown, "", "  ")

	var sb strings.Builder
	sb.WriteString("## .well-known/mcp.json\n\n")
	sb.WriteString("Host this file at your server's `/.well-known/mcp.json` endpoint.\n\n")
	sb.WriteString("```json\n")
	sb.WriteString(string(output))
	sb.WriteString("\n```\n\n")
	sb.WriteString("### MCP Roadmap Context\n\n")
	sb.WriteString("The .well-known URL pattern enables clients to discover server capabilities\n")
	sb.WriteString("without connecting first. This is part of the MCP scalability roadmap for\n")
	sb.WriteString("enterprise deployments.\n")

	return tools.TextResult(sb.String()), nil
}

// generateRegistryReadme creates a README for the registry submission
func generateRegistryReadme(manifest MCPServerManifest, allTools []tools.ToolDefinition) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s\n\n", manifest.Name))
	sb.WriteString(fmt.Sprintf("%s\n\n", manifest.Description))

	sb.WriteString("## Installation\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("# Clone and build\n")
	sb.WriteString("git clone https://github.com/hairglasses-studio/webb\n")
	sb.WriteString("cd webb && go build -o webb-mcp ./cmd/webb-mcp\n\n")
	sb.WriteString("# Add to Claude Desktop config\n")
	sb.WriteString("# ~/.claude.json\n")
	sb.WriteString("```\n\n")

	sb.WriteString("## Configuration\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"mcpServers\": {\n")
	sb.WriteString("    \"webb\": {\n")
	sb.WriteString("      \"type\": \"stdio\",\n")
	sb.WriteString("      \"command\": \"/path/to/webb-mcp\"\n")
	sb.WriteString("    }\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n\n")

	sb.WriteString("## Tools\n\n")
	sb.WriteString(fmt.Sprintf("Webb provides **%d tools** across the following categories:\n\n", len(allTools)))

	// Count by category
	catCounts := make(map[string]int)
	for _, t := range allTools {
		catCounts[t.Category]++
	}

	for cat, count := range catCounts {
		sb.WriteString(fmt.Sprintf("- **%s**: %d tools\n", cat, count))
	}

	sb.WriteString("\n## Key Features\n\n")
	sb.WriteString("- Unified platform operations interface\n")
	sb.WriteString("- Progressive tool disclosure (lazy loading)\n")
	sb.WriteString("- 57+ modules covering infrastructure, incidents, observability\n")
	sb.WriteString("- Autonomous research swarm with 23 worker types\n")
	sb.WriteString("- Native AWS, Kubernetes, database integrations\n")

	sb.WriteString(fmt.Sprintf("\n## Version\n\n%s\n", manifest.Version))
	sb.WriteString(fmt.Sprintf("\n## License\n\n%s\n", manifest.License))

	sb.WriteString(fmt.Sprintf("\n---\n*Generated %s*\n", time.Now().Format("2006-01-02")))

	return sb.String()
}

func truncateDesc(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// MCPPublishTools returns tools for MCP registry publication
func MCPPublishTools() []tools.ToolDefinition {
	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("webb_mcp_registry_publish",
				mcp.WithDescription("Generate MCP Registry submission package. Creates manifest.json, README.md, and .well-known/mcp.json for registry publication."),
				mcp.WithString("output_dir",
					mcp.Description("Output directory for generated files (default: ~/webb-mcp-registry)"),
				),
				mcp.WithBoolean("dry_run",
					mcp.Description("Preview without writing files (default: true)"),
				),
			),
			Handler:     handleMCPRegistryPublish,
			Category:    "discovery",
			Subcategory: "mcp_publish",
			Tags:        []string{"mcp", "registry", "publish", "manifest"},
			UseCases:    []string{"Publish webb to MCP Registry", "Generate server manifest", "Prepare registry submission"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     true,
		},
		{
			Tool: mcp.NewTool("webb_wellknown_generate",
				mcp.WithDescription("Generate .well-known/mcp.json for server capability discovery. Enables clients to discover webb capabilities without connecting."),
			),
			Handler:     handleWellKnownGenerate,
			Category:    "discovery",
			Subcategory: "mcp_publish",
			Tags:        []string{"mcp", "well-known", "discovery", "capabilities"},
			UseCases:    []string{"Enable capability discovery", "Generate .well-known metadata", "Support MCP scalability roadmap"},
			Complexity:  tools.ComplexitySimple,
		},
	}
}
