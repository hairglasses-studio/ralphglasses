package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/mcpkit/registry"
)

// applyToolMetadata enriches a tool with annotations and output schema if available.
func applyToolMetadata(t *mcp.Tool) {
	// Wire annotations from the ToolAnnotations map.
	if ann, ok := ToolAnnotations[t.Name]; ok {
		t.Annotations = ann
	}

	// Wire output schema from the OutputSchemas map.
	if schema := SchemaForTool(t.Name); schema != nil {
		raw, err := json.Marshal(schema)
		if err == nil {
			t.RawOutputSchema = raw
		}
	}
}

// addToolWithMetadata registers a tool entry with annotations and output schema applied.
func (s *Server) addToolWithMetadata(srv *server.MCPServer, entry ToolEntry) {
	applyToolMetadata(&entry.Tool)

	handler := entry.Handler
	if s.Observability != nil {
		mw := s.Observability.Middleware()
		td := registry.ToolDefinition{
			Tool:     entry.Tool,
			Category: "ralph",
		}
		handler = server.ToolHandlerFunc(mw(entry.Tool.Name, td, registry.ToolHandlerFunc(entry.Handler)))
	}

	srv.AddTool(entry.Tool, handler)
}

// Register adds all ralphglasses tools to the MCP server (backward compatible).
func (s *Server) Register(srv *server.MCPServer) {
	if s.DeferredLoading {
		s.RegisterCoreTools(srv)
	} else {
		s.RegisterAllTools(srv)
	}
}

// RegisterCoreTools registers only essential tools plus the deferred loading tools.
func (s *Server) RegisterCoreTools(srv *server.MCPServer) {
	s.mcpSrv = srv
	s.loadedGroups = make(map[string]bool)

	// Register the tool_groups and load_tool_group management tools.
	s.addToolWithMetadata(srv, ToolEntry{
		Tool: mcp.NewTool("ralphglasses_tool_groups",
			mcp.WithDescription("List available tool groups for deferred loading. Call ralphglasses_load_tool_group to load a specific group."),
		),
		Handler: s.handleToolGroups,
	})

	s.addToolWithMetadata(srv, ToolEntry{
		Tool: mcp.NewTool("ralphglasses_load_tool_group",
			mcp.WithDescription(loadToolGroupDescription()),
			mcp.WithString("group", mcp.Required(), mcp.Description("Tool group name to load")),
		),
		Handler: s.handleLoadToolGroup,
	})

	s.addToolWithMetadata(srv, ToolEntry{
		Tool: mcp.NewTool("ralphglasses_skill_export",
			mcp.WithDescription("Generate SKILL.md documentation from all registered tool groups. Returns markdown or JSON."),
			mcp.WithString("format", mcp.Description("Output format: \"markdown\" (default) or \"json\"")),
			mcp.WithString("group", mcp.Description("Filter to a specific namespace (e.g. \"core\", \"session\")")),
		),
		Handler: s.handleSkillExport,
	})

	s.addToolWithMetadata(srv, ToolEntry{
		Tool: mcp.NewTool("ralphglasses_server_health",
			mcp.WithDescription("Show the active ralphglasses MCP contract shape, including available tool groups, loaded groups, and resource/prompt coverage."),
		),
		Handler: s.handleServerHealth,
	})

	// Register core group tools.
	coreGroup := s.buildCoreGroup()
	for _, entry := range coreGroup.Tools {
		s.addToolWithMetadata(srv, entry)
	}
	s.loadedGroups["core"] = true
}

// RegisterToolGroup registers all tools in a named group. Returns an error if
// the group name is unknown. Safe to call multiple times (idempotent).
func (s *Server) RegisterToolGroup(srv *server.MCPServer, group string) error {
	groups := s.buildToolGroups()
	for _, g := range groups {
		if g.Name == group {
			for _, entry := range g.Tools {
				s.addToolWithMetadata(srv, entry)
			}
			s.mu.Lock()
			if s.loadedGroups != nil {
				s.loadedGroups[group] = true
			}
			s.mu.Unlock()
			return nil
		}
	}
	return fmt.Errorf("unknown tool group: %q (valid: %s)", group, strings.Join(ToolGroupNames, ", "))
}

// RegisterAllTools registers every tool across all groups (backward compatibility).
func (s *Server) RegisterAllTools(srv *server.MCPServer) {
	s.mcpSrv = srv
	s.loadedGroups = make(map[string]bool)

	// Register group management tools so they are always available.
	s.addToolWithMetadata(srv, ToolEntry{
		Tool: mcp.NewTool("ralphglasses_tool_groups",
			mcp.WithDescription("List available tool groups for deferred loading. Call ralphglasses_load_tool_group to load a specific group."),
		),
		Handler: s.handleToolGroups,
	})

	s.addToolWithMetadata(srv, ToolEntry{
		Tool: mcp.NewTool("ralphglasses_load_tool_group",
			mcp.WithDescription(loadToolGroupDescription()),
			mcp.WithString("group", mcp.Required(), mcp.Description("Tool group name to load")),
		),
		Handler: s.handleLoadToolGroup,
	})

	s.addToolWithMetadata(srv, ToolEntry{
		Tool: mcp.NewTool("ralphglasses_skill_export",
			mcp.WithDescription("Generate SKILL.md documentation from all registered tool groups. Returns markdown or JSON."),
			mcp.WithString("format", mcp.Description("Output format: \"markdown\" (default) or \"json\"")),
			mcp.WithString("group", mcp.Description("Filter to a specific namespace (e.g. \"core\", \"session\")")),
		),
		Handler: s.handleSkillExport,
	})

	s.addToolWithMetadata(srv, ToolEntry{
		Tool: mcp.NewTool("ralphglasses_server_health",
			mcp.WithDescription("Show the active ralphglasses MCP contract shape, including available tool groups, loaded groups, and resource/prompt coverage."),
		),
		Handler: s.handleServerHealth,
	})

	for _, g := range s.buildToolGroups() {
		for _, entry := range g.Tools {
			s.addToolWithMetadata(srv, entry)
		}
		s.loadedGroups[g.Name] = true
	}
}

// ToolGroups returns all tool group metadata (for testing and introspection).
func (s *Server) ToolGroups() []ToolGroup {
	return s.buildToolGroups()
}

// handleToolGroups returns available tool groups with their descriptions and tool counts.
func (s *Server) handleToolGroups(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	type groupInfo struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		ToolCount   int      `json:"tool_count"`
		Loaded      bool     `json:"loaded"`
		Tools       []string `json:"tools"`
	}

	groups := s.buildToolGroups()
	out := make([]groupInfo, len(groups))
	for i, g := range groups {
		tools := make([]string, len(g.Tools))
		for j, t := range g.Tools {
			tools[j] = t.Tool.Name
		}
		s.mu.RLock()
		loaded := s.loadedGroups[g.Name]
		s.mu.RUnlock()
		out[i] = groupInfo{
			Name:        g.Name,
			Description: g.Description,
			ToolCount:   len(g.Tools),
			Loaded:      loaded,
			Tools:       tools,
		}
	}
	return jsonResult(out), nil
}

// handleLoadToolGroup loads all tools in a named group on demand.
func (s *Server) handleLoadToolGroup(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	group := getStringArg(req, "group")
	if group == "" {
		return codedError(ErrInvalidParams, "group is required"), nil
	}

	s.mu.RLock()
	alreadyLoaded := s.loadedGroups[group]
	s.mu.RUnlock()
	if alreadyLoaded {
		return jsonResult(map[string]any{
			"group":   group,
			"status":  "already_loaded",
			"message": fmt.Sprintf("Tool group %q is already loaded", group),
		}), nil
	}

	if s.mcpSrv == nil {
		return codedError(ErrInternal, "MCP server reference not set"), nil
	}

	if err := s.RegisterToolGroup(s.mcpSrv, group); err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}

	// Count tools in the loaded group.
	var count int
	for _, g := range s.buildToolGroups() {
		if g.Name == group {
			count = len(g.Tools)
			break
		}
	}

	return jsonResult(map[string]any{
		"group":      group,
		"status":     "loaded",
		"tool_count": count,
		"message":    fmt.Sprintf("Loaded %d tools from group %q", count, group),
	}), nil
}

func (s *Server) handleServerHealth(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	groups := s.buildToolGroups()
	groupNames := make([]string, 0, len(groups))
	loadedGroups := make([]string, 0, len(groups))
	groupToolCount := 0
	groupSummary := make([]map[string]any, 0, len(groups))

	for _, group := range groups {
		groupNames = append(groupNames, group.Name)
		groupToolCount += len(group.Tools)
		groupSummary = append(groupSummary, map[string]any{
			"name":        group.Name,
			"description": group.Description,
			"tool_count":  len(group.Tools),
		})
	}
	sort.Strings(groupNames)

	s.mu.RLock()
	for group, loaded := range s.loadedGroups {
		if loaded {
			loadedGroups = append(loadedGroups, group)
		}
	}
	s.mu.RUnlock()
	sort.Strings(loadedGroups)

	resourceDefs := staticResourceCatalog()
	templateDefs := resourceTemplateCatalog()
	managementTools := managementToolNames()

	return jsonResult(map[string]any{
		"server":                  "ralphglasses",
		"version":                 s.runtimeVersion(),
		"commit":                  strings.TrimSpace(s.Commit),
		"build_date":              strings.TrimSpace(s.BuildDate),
		"status":                  "ok",
		"scan_path":               s.ScanPath,
		"instructions":            ServerInstructions(),
		"deferred_mode":           s.DeferredLoading,
		"tool_group_count":        len(groups),
		"tool_groups":             groupNames,
		"tool_group_summary":      groupSummary,
		"loaded_groups":           loadedGroups,
		"group_tool_count":        groupToolCount,
		"management_tool_count":   len(managementTools),
		"tool_count":              groupToolCount + len(managementTools),
		"resource_count":          len(resourceDefs),
		"resource_uris":           resourceURIs(resourceDefs),
		"resource_template_count": len(templateDefs),
		"resource_templates":      resourceTemplateURIs(templateDefs),
		"prompt_count":            len(promptCatalog()),
		"prompt_names":            promptNames(),
		"discovery_tools":         managementTools,
	}), nil
}
