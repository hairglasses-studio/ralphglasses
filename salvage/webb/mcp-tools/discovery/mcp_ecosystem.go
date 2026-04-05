package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hairglasses-studio/webb/internal/clients"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPEcosystemTools returns MCP ecosystem discovery and installation tools
func MCPEcosystemTools() []tools.ToolDefinition {
	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("webb_mcp_discover",
				mcp.WithDescription("Search npm, GitHub, and awesome-mcp-servers for MCP servers. Returns installable servers with install commands."),
				mcp.WithString("query",
					mcp.Description("Search query (e.g., 'postgres', 'kubernetes', 'monitoring')"),
					mcp.Required(),
				),
				mcp.WithString("source",
					mcp.Description("Source to search: npm, github, all (default: all)"),
				),
				mcp.WithString("category",
					mcp.Description("Filter by category: database, kubernetes, monitoring, cloud, etc."),
				),
				mcp.WithNumber("limit",
					mcp.Description("Maximum results to return (default: 20)"),
				),
				mcp.WithString("format",
					mcp.Description("Output format: table (default), detailed, json"),
				),
			),
			Handler:     handleMCPDiscover,
			Category:    "discovery",
			Subcategory: "mcp_ecosystem",
			Tags:        []string{"mcp", "discovery", "install", "npm", "github"},
			UseCases:    []string{"Find MCP servers to install", "Discover new integrations", "Search ecosystem"},
			Complexity:  tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("webb_mcp_install",
				mcp.WithDescription("Install an MCP server and optionally register it with Codex, Claude Desktop, both, or neither. Supports npm, docker, and uvx methods."),
				mcp.WithString("server",
					mcp.Description("Server name from catalog (e.g., 'postgres', 'grafana')"),
				),
				mcp.WithString("package",
					mcp.Description("Full package name (e.g., '@modelcontextprotocol/server-postgres'). Use if not in catalog."),
				),
				mcp.WithString("method",
					mcp.Description("Install method: npm (default), docker, uvx, binary"),
				),
				mcp.WithString("register_with",
					mcp.Description("Register the server with a client config: auto (default), codex, claude, both, or none"),
				),
				mcp.WithBoolean("add_to_claude",
					mcp.Description("Deprecated alias for register_with. true maps to claude, false maps to none when register_with is not set."),
				),
				mcp.WithBoolean("dry_run",
					mcp.Description("Preview installation without executing (default: false)"),
				),
			),
			Handler:     handleMCPInstall,
			Category:    "discovery",
			Subcategory: "mcp_ecosystem",
			Tags:        []string{"mcp", "install", "npm", "docker", "claude"},
			UseCases:    []string{"Install MCP servers", "Add to Claude config", "Set up integrations"},
			Complexity:  tools.ComplexityModerate,
		},
		{
			Tool: mcp.NewTool("webb_mcp_catalog",
				mcp.WithDescription("Browse curated MCP server catalog. Shows recommended servers with install commands and required env vars."),
				mcp.WithString("category",
					mcp.Description("Filter by category: database, kubernetes, monitoring, cloud, productivity, collaboration, devtools"),
				),
				mcp.WithString("format",
					mcp.Description("Output format: table (default), detailed"),
				),
				mcp.WithString("query",
					mcp.Description("Search catalog by name or description"),
				),
			),
			Handler:     handleMCPCatalog,
			Category:    "discovery",
			Subcategory: "mcp_ecosystem",
			Tags:        []string{"mcp", "catalog", "curated", "recommended"},
			UseCases:    []string{"Browse recommended MCP servers", "Find official servers", "Discover integrations"},
			Complexity:  tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("webb_mcp_installed",
				mcp.WithDescription("List installed MCP servers and registered client config entries."),
				mcp.WithBoolean("include_registered_configs",
					mcp.Description("Include servers from registered client configs (default: true)"),
				),
				mcp.WithString("config_clients",
					mcp.Description("Which registered client configs to include: all (default), codex, or claude"),
				),
				mcp.WithBoolean("include_claude_config",
					mcp.Description("Deprecated alias for include_registered_configs with config_clients=\"claude\" when the new parameters are not set."),
				),
				mcp.WithString("format",
					mcp.Description("Output format: table (default), json"),
				),
			),
			Handler:     handleMCPInstalled,
			Category:    "discovery",
			Subcategory: "mcp_ecosystem",
			Tags:        []string{"mcp", "installed", "status", "claude"},
			UseCases:    []string{"View installed servers", "Check Claude config", "Audit installations"},
			Complexity:  tools.ComplexitySimple,
		},
	}
}

// handleMCPDiscover searches the MCP ecosystem for servers
func handleMCPDiscover(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := req.GetString("query", "")
	if query == "" {
		return tools.ErrorResult(fmt.Errorf("query is required")), nil
	}

	source := req.GetString("source", "all")
	category := req.GetString("category", "")
	limit := req.GetInt("limit", 20)
	format := req.GetString("format", "table")

	client := clients.NewMCPEcosystemClient()

	var discoveries []clients.MCPServerDiscovery
	var err error

	switch source {
	case "npm":
		discoveries, err = client.SearchNPM(ctx, query, limit)
	case "github":
		discoveries, err = client.SearchGitHub(ctx, query, limit)
	default: // all
		discoveries, err = client.SearchAll(ctx, query, limit)
	}

	if err != nil {
		return tools.ErrorResult(fmt.Errorf("search failed: %w", err)), nil
	}

	// Filter by category if specified
	if category != "" {
		var filtered []clients.MCPServerDiscovery
		for _, d := range discoveries {
			if strings.EqualFold(d.Category, category) {
				filtered = append(filtered, d)
			}
		}
		discoveries = filtered
	}

	// Format output
	var sb strings.Builder

	switch format {
	case "json":
		data, _ := json.MarshalIndent(discoveries, "", "  ")
		return tools.TextResult(string(data)), nil

	case "detailed":
		sb.WriteString(fmt.Sprintf("## MCP Server Discovery: \"%s\"\n\n", query))
		sb.WriteString(fmt.Sprintf("**Source:** %s | **Results:** %d\n\n", source, len(discoveries)))

		for _, d := range discoveries {
			official := ""
			if d.Official {
				official = " (Official)"
			}
			sb.WriteString(fmt.Sprintf("### %s%s\n\n", d.Name, official))
			sb.WriteString(fmt.Sprintf("**Package:** `%s`\n", d.Package))
			sb.WriteString(fmt.Sprintf("**Source:** %s | **Category:** %s\n", d.Source, d.Category))
			if d.Description != "" {
				sb.WriteString(fmt.Sprintf("**Description:** %s\n", d.Description))
			}
			if d.Stars > 0 {
				sb.WriteString(fmt.Sprintf("**Stars:** %d\n", d.Stars))
			}
			if d.Downloads > 0 {
				sb.WriteString(fmt.Sprintf("**Downloads:** ~%d\n", d.Downloads))
			}
			sb.WriteString(fmt.Sprintf("\n**Install:** `webb_mcp_install(package=\"%s\")`\n\n", d.Package))
			sb.WriteString("---\n\n")
		}

	default: // table
		sb.WriteString(fmt.Sprintf("## MCP Server Discovery: \"%s\"\n\n", query))
		sb.WriteString(fmt.Sprintf("**Source:** %s | **Results:** %d\n\n", source, len(discoveries)))

		if len(discoveries) == 0 {
			sb.WriteString("No MCP servers found matching your query.\n\n")
			sb.WriteString("**Tips:**\n")
			sb.WriteString("- Try a different search term\n")
			sb.WriteString("- Use `webb_mcp_catalog()` to browse curated servers\n")
			sb.WriteString("- Try `source=\"npm\"` or `source=\"github\"` for specific sources\n")
		} else {
			sb.WriteString("| Server | Package | Category | Source | Official |\n")
			sb.WriteString("|--------|---------|----------|--------|----------|\n")

			for _, d := range discoveries {
				official := ""
				if d.Official {
					official = "Yes"
				}

				name := d.Name
				if len(name) > 20 {
					name = name[:17] + "..."
				}

				pkg := d.Package
				if len(pkg) > 40 {
					pkg = pkg[:37] + "..."
				}

				sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
					name, pkg, d.Category, d.Source, official))
			}

			sb.WriteString("\n**Install:** Use `webb_mcp_install(server=\"<name>\")` or `webb_mcp_install(package=\"<package>\")`\n")
		}
	}

	return tools.TextResult(sb.String()), nil
}

// handleMCPInstall installs an MCP server
func handleMCPInstall(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	serverName := req.GetString("server", "")
	packageName := req.GetString("package", "")
	methodStr := req.GetString("method", "npm")
	registerWith := resolveRegisterWith(req)
	dryRun := req.GetBool("dry_run", false)

	if serverName == "" && packageName == "" {
		return tools.ErrorResult(fmt.Errorf("either 'server' or 'package' is required")), nil
	}

	installClient, err := clients.NewMCPInstallClient()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to create install client: %w", err)), nil
	}

	var discovery *clients.MCPServerDiscovery

	// Check catalog first if server name provided
	if serverName != "" {
		catalog := clients.NewMCPCatalog()
		entry := catalog.GetEntry(serverName)
		if entry != nil {
			d := entry.ToDiscovery()
			discovery = &d
		}
	}

	// If not found in catalog and package provided, create discovery from package
	if discovery == nil && packageName != "" {
		method := clients.InstallNPM
		switch methodStr {
		case "docker":
			method = clients.InstallDocker
		case "uvx", "python":
			method = clients.InstallUVX
		case "binary":
			method = clients.InstallBinary
		}

		name := packageName
		if serverName != "" {
			name = serverName
		} else {
			// Extract name from package
			name = clients.NewMCPCatalog().GetEntry("filesystem").Name // placeholder
			// Actually extract from package
			parts := strings.Split(packageName, "/")
			name = parts[len(parts)-1]
			name = strings.TrimPrefix(name, "server-")
			name = strings.TrimPrefix(name, "mcp-server-")
			name = strings.TrimSuffix(name, "-mcp")
		}

		discovery = &clients.MCPServerDiscovery{
			Name:          name,
			Package:       packageName,
			InstallMethod: method,
			Command:       getCommandForMethod(method),
			Args:          getArgsForMethod(method, packageName),
		}
	}

	if discovery == nil {
		return tools.ErrorResult(fmt.Errorf("server '%s' not found in catalog. Use 'package' parameter for custom packages", serverName)), nil
	}

	var sb strings.Builder

	if dryRun {
		sb.WriteString("## MCP Server Installation (Dry Run)\n\n")
		sb.WriteString(fmt.Sprintf("**Server:** %s\n", discovery.Name))
		sb.WriteString(fmt.Sprintf("**Package:** %s\n", discovery.Package))
		sb.WriteString(fmt.Sprintf("**Method:** %s\n", discovery.InstallMethod))
		sb.WriteString(fmt.Sprintf("**Register With:** %s\n\n", registerWith))

		sb.WriteString("### Would Execute:\n\n")

		switch discovery.InstallMethod {
		case clients.InstallNPM:
			sb.WriteString(fmt.Sprintf("```bash\nnpm install -g %s\n```\n\n", discovery.Package))
		case clients.InstallDocker:
			sb.WriteString(fmt.Sprintf("```bash\ndocker pull %s\n```\n\n", discovery.Package))
		case clients.InstallUVX:
			sb.WriteString(fmt.Sprintf("```bash\nuvx install %s\n```\n\n", discovery.Package))
		}

		for _, clientName := range registrationClients(registerWith) {
			sb.WriteString(fmt.Sprintf("### Would Register With %s\n\n", clientDisplayName(clientName)))
			sb.WriteString(formatClientSnippet(clientName, discovery.Name, discovery.Command, discovery.Args))
			sb.WriteString("\n\n")
		}

		if len(discovery.EnvVars) > 0 {
			sb.WriteString("**Required Environment Variables:**\n")
			for _, ev := range discovery.EnvVars {
				sb.WriteString(fmt.Sprintf("- `%s`\n", ev))
			}
		}

		sb.WriteString("\n*Run without `dry_run=true` to execute installation.*\n")
		return tools.TextResult(sb.String()), nil
	}

	// Actually install
	sb.WriteString("## MCP Server Installation\n\n")
	sb.WriteString(fmt.Sprintf("**Server:** %s\n", discovery.Name))
	sb.WriteString(fmt.Sprintf("**Package:** %s\n", discovery.Package))
	sb.WriteString(fmt.Sprintf("**Method:** %s\n\n", discovery.InstallMethod))

	installed, err := installClient.Install(discovery, false)
	if err != nil {
		sb.WriteString(fmt.Sprintf("**Installation Failed:** %v\n\n", err))
		sb.WriteString("**Troubleshooting:**\n")
		sb.WriteString("- Check that npm/docker is installed and in PATH\n")
		sb.WriteString("- Try running manually: ")

		switch discovery.InstallMethod {
		case clients.InstallNPM:
			sb.WriteString(fmt.Sprintf("`npm install -g %s`\n", discovery.Package))
		case clients.InstallDocker:
			sb.WriteString(fmt.Sprintf("`docker pull %s`\n", discovery.Package))
		}

		return tools.TextResult(sb.String()), nil
	}

	sb.WriteString("**Installation:** Success\n")
	if installed.Version != "" {
		sb.WriteString(fmt.Sprintf("**Version:** %s\n", installed.Version))
	}

	if registerWith != "none" {
		sb.WriteString("\n### Client Registration\n\n")
		if err := installClient.RegisterWithClient(installed, nil, registerWith, true); err != nil {
			sb.WriteString(fmt.Sprintf("**Registration Failed:** %v\n\n", err))
			sb.WriteString("Add one of these snippets manually:\n\n")
		} else {
			sb.WriteString(fmt.Sprintf("**Registered In:** %s\n\n", strings.Join(installed.RegisteredClients, ", ")))
		}

		for _, clientName := range registrationClients(registerWith) {
			configPath := installClient.GetClientConfigPath(clientName)
			if configPath == "" {
				sb.WriteString(fmt.Sprintf("**%s Config:** not found, use the snippet below manually.\n\n", clientDisplayName(clientName)))
			} else {
				sb.WriteString(fmt.Sprintf("**%s Config:** `%s`\n\n", clientDisplayName(clientName), configPath))
			}
			sb.WriteString(formatClientSnippet(clientName, installed.Name, installed.Command, installed.Args))
			sb.WriteString("\n\n")
		}

		if len(discovery.EnvVars) > 0 {
			sb.WriteString("**Note:** Add these environment variables to the registered client config:\n")
			for _, ev := range discovery.EnvVars {
				sb.WriteString(fmt.Sprintf("- `%s`\n", ev))
			}
		}
	}

	sb.WriteString("\n### Next Steps\n\n")
	switch registerWith {
	case "claude":
		sb.WriteString("1. Restart Claude Desktop to load the new server\n")
	case "codex", "auto":
		sb.WriteString("1. Restart Codex or open a new Codex session to load the new server\n")
	case "both":
		sb.WriteString("1. Restart Codex and Claude Desktop to load the new server\n")
	default:
		sb.WriteString("1. Register the server with Codex or Claude when you are ready to use it\n")
	}
	sb.WriteString("2. Verify connection with `webb_mcp_status()`\n")
	sb.WriteString(fmt.Sprintf("3. Optionally federate: `webb_mcp_remote_connect(server_name=\"%s\", federate=true)`\n", discovery.Name))

	return tools.TextResult(sb.String()), nil
}

// handleMCPCatalog browses the curated MCP server catalog
func handleMCPCatalog(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	category := req.GetString("category", "")
	format := req.GetString("format", "table")
	query := req.GetString("query", "")

	catalog := clients.NewMCPCatalog()

	var entries []*clients.MCPCatalogEntry

	if query != "" {
		entries = catalog.Search(query)
	} else if category != "" && category != "all" {
		entries = catalog.ListByCategory(category)
	} else {
		entries = catalog.ListEntries()
	}

	var sb strings.Builder

	switch format {
	case "detailed":
		sb.WriteString("## MCP Server Catalog (Detailed)\n\n")

		if category != "" {
			sb.WriteString(fmt.Sprintf("**Category:** %s\n", category))
		}
		if query != "" {
			sb.WriteString(fmt.Sprintf("**Search:** %s\n", query))
		}
		sb.WriteString(fmt.Sprintf("**Total:** %d servers\n\n", len(entries)))

		for _, e := range entries {
			official := ""
			if e.Official {
				official = " (Official)"
			}

			sb.WriteString(fmt.Sprintf("### %s%s\n\n", e.DisplayName, official))
			sb.WriteString(fmt.Sprintf("**Package:** `%s`\n", e.Package))
			sb.WriteString(fmt.Sprintf("**Category:** %s | **Relevance:** %d/100\n", e.Category, e.WebbRelevance))
			sb.WriteString(fmt.Sprintf("**Description:** %s\n\n", e.Description))

			if len(e.Features) > 0 {
				sb.WriteString("**Features:** ")
				sb.WriteString(strings.Join(e.Features, ", "))
				sb.WriteString("\n\n")
			}

			if len(e.EnvVars) > 0 {
				sb.WriteString("**Required Environment Variables:**\n")
				for _, ev := range e.EnvVars {
					req := ""
					if ev.Required {
						req = " (required)"
					}
					sb.WriteString(fmt.Sprintf("- `%s`%s: %s\n", ev.Name, req, ev.Description))
				}
				sb.WriteString("\n")
			}

			sb.WriteString(fmt.Sprintf("**Install:** `webb_mcp_install(server=\"%s\")`\n\n", e.Name))
			sb.WriteString("---\n\n")
		}

	default: // table
		sb.WriteString("## MCP Server Catalog\n\n")

		// Show categories
		categories := catalog.GetCategories()
		sb.WriteString(fmt.Sprintf("**Categories:** %s\n", strings.Join(categories, ", ")))

		if category != "" {
			sb.WriteString(fmt.Sprintf("**Filtered:** %s\n", category))
		}
		if query != "" {
			sb.WriteString(fmt.Sprintf("**Search:** %s\n", query))
		}
		sb.WriteString(fmt.Sprintf("**Total:** %d servers\n\n", len(entries)))

		if len(entries) == 0 {
			sb.WriteString("No servers found matching criteria.\n")
		} else {
			sb.WriteString("| Server | Category | Official | Package | Install |\n")
			sb.WriteString("|--------|----------|----------|---------|----------|\n")

			for _, e := range entries {
				official := ""
				if e.Official {
					official = "Yes"
				}

				pkg := e.Package
				if len(pkg) > 35 {
					pkg = pkg[:32] + "..."
				}

				sb.WriteString(fmt.Sprintf("| %s | %s | %s | `%s` | `webb_mcp_install(server=\"%s\")` |\n",
					e.Name, e.Category, official, pkg, e.Name))
			}
		}

		sb.WriteString("\n**Tip:** Use `format=\"detailed\"` for full information including env vars.\n")
	}

	return tools.TextResult(sb.String()), nil
}

// handleMCPInstalled lists installed MCP servers
func handleMCPInstalled(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	includeConfigs, configClients := resolveInstalledConfigOptions(req)
	format := req.GetString("format", "table")

	installClient, err := clients.NewMCPInstallClient()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to create install client: %w", err)), nil
	}

	if includeConfigs {
		if err := installClient.SyncFromRegisteredConfigs(configClients); err != nil {
			// Non-fatal, continue
		}
	}

	installed := installClient.ListInstalled()

	if format == "json" {
		data, _ := json.MarshalIndent(installed, "", "  ")
		return tools.TextResult(string(data)), nil
	}

	var sb strings.Builder
	sb.WriteString(installClient.FormatInstallStatus(includeConfigs, configClients))

	sb.WriteString("\n### Commands\n\n")
	sb.WriteString("- **Install:** `webb_mcp_install(server=\"<name>\")`\n")
	sb.WriteString("- **Browse catalog:** `webb_mcp_catalog()`\n")
	sb.WriteString("- **Discover more:** `webb_mcp_discover(query=\"<keyword>\")`\n")

	return tools.TextResult(sb.String()), nil
}

// Helper functions

func getCommandForMethod(method clients.MCPInstallMethod) string {
	switch method {
	case clients.InstallNPM:
		return "npx"
	case clients.InstallDocker:
		return "docker"
	case clients.InstallUVX:
		return "uvx"
	case clients.InstallPython:
		return "python"
	default:
		return "npx"
	}
}

func getArgsForMethod(method clients.MCPInstallMethod, pkg string) []string {
	switch method {
	case clients.InstallNPM:
		return []string{"-y", pkg}
	case clients.InstallDocker:
		return []string{"run", "-i", "--rm", pkg}
	case clients.InstallUVX:
		return []string{pkg}
	default:
		return []string{pkg}
	}
}

func formatArgs(args []string) string {
	if len(args) == 0 {
		return "[]"
	}

	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = fmt.Sprintf("\"%s\"", arg)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func resolveRegisterWith(req mcp.CallToolRequest) string {
	args := req.GetArguments()
	if _, ok := args["register_with"]; ok {
		return normalizeRegistrationTarget(req.GetString("register_with", "auto"))
	}
	if raw, ok := args["add_to_claude"].(bool); ok {
		if raw {
			return "claude"
		}
		return "none"
	}
	return "auto"
}

func resolveInstalledConfigOptions(req mcp.CallToolRequest) (bool, []string) {
	args := req.GetArguments()
	if _, ok := args["include_registered_configs"]; ok || args["config_clients"] != nil {
		return req.GetBool("include_registered_configs", true), expandConfigClients(req.GetString("config_clients", "all"))
	}
	if raw, ok := args["include_claude_config"].(bool); ok {
		return raw, []string{"claude"}
	}
	return true, []string{"claude", "codex"}
}

func normalizeRegistrationTarget(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return "auto"
	case "claude":
		return "claude"
	case "codex":
		return "codex"
	case "both":
		return "both"
	case "none":
		return "none"
	default:
		return "auto"
	}
}

func expandConfigClients(value string) []string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all":
		return []string{"claude", "codex"}
	case "claude":
		return []string{"claude"}
	case "codex":
		return []string{"codex"}
	default:
		return []string{"claude", "codex"}
	}
}

func registrationClients(target string) []string {
	switch normalizeRegistrationTarget(target) {
	case "claude":
		return []string{"claude"}
	case "codex", "auto":
		return []string{"codex"}
	case "both":
		return []string{"claude", "codex"}
	default:
		return nil
	}
}

func clientDisplayName(client string) string {
	switch client {
	case "claude":
		return "Claude Desktop"
	case "codex":
		return "Codex"
	default:
		return strings.Title(client)
	}
}

func formatClientSnippet(client, name, command string, args []string) string {
	switch client {
	case "claude":
		return "```json\n" + fmt.Sprintf("{\n  \"%s\": {\n    \"command\": \"%s\",\n    \"args\": %s\n  }\n}", name, command, formatArgs(args)) + "\n```"
	case "codex":
		return "```toml\n" + fmt.Sprintf("[mcp_servers.%s]\ncommand = %q\nargs = %s", name, command, formatArgs(args)) + "\n```"
	default:
		return ""
	}
}
