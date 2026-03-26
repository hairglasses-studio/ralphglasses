package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

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
	srv.AddTool(mcp.NewTool("ralphglasses_tool_groups",
		mcp.WithDescription("List available tool groups for deferred loading. Call ralphglasses_load_tool_group to load a specific group."),
	), s.handleToolGroups)

	srv.AddTool(mcp.NewTool("ralphglasses_load_tool_group",
		mcp.WithDescription("Load all tools in a named group (session, loop, prompt, fleet, repo, roadmap, team, awesome, advanced)"),
		mcp.WithString("group", mcp.Required(), mcp.Description("Tool group name to load")),
	), s.handleLoadToolGroup)

	// Register core group tools.
	coreGroup := s.buildCoreGroup()
	for _, entry := range coreGroup.Tools {
		srv.AddTool(entry.Tool, entry.Handler)
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
				srv.AddTool(entry.Tool, entry.Handler)
			}
			if s.loadedGroups != nil {
				s.loadedGroups[group] = true
			}
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
	srv.AddTool(mcp.NewTool("ralphglasses_tool_groups",
		mcp.WithDescription("List available tool groups for deferred loading. Call ralphglasses_load_tool_group to load a specific group."),
	), s.handleToolGroups)

	srv.AddTool(mcp.NewTool("ralphglasses_load_tool_group",
		mcp.WithDescription("Load all tools in a named group (session, loop, prompt, fleet, repo, roadmap, team, awesome, advanced)"),
		mcp.WithString("group", mcp.Required(), mcp.Description("Tool group name to load")),
	), s.handleLoadToolGroup)

	for _, g := range s.buildToolGroups() {
		for _, entry := range g.Tools {
			srv.AddTool(entry.Tool, entry.Handler)
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
		out[i] = groupInfo{
			Name:        g.Name,
			Description: g.Description,
			ToolCount:   len(g.Tools),
			Loaded:      s.loadedGroups[g.Name],
			Tools:       tools,
		}
	}
	return jsonResult(out), nil
}

// handleLoadToolGroup loads all tools in a named group on demand.
func (s *Server) handleLoadToolGroup(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	group := getStringArg(req, "group")
	if group == "" {
		return invalidParams("group is required"), nil
	}

	if s.loadedGroups[group] {
		return jsonResult(map[string]any{
			"group":   group,
			"status":  "already_loaded",
			"message": fmt.Sprintf("Tool group %q is already loaded", group),
		}), nil
	}

	if s.mcpSrv == nil {
		return internalErr("MCP server reference not set"), nil
	}

	if err := s.RegisterToolGroup(s.mcpSrv, group); err != nil {
		return invalidParams(err.Error()), nil
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
