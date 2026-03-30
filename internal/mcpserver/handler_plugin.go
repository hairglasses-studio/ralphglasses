package mcpserver

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// handlePluginList returns all registered plugins with name, version, status, and type.
func (s *Server) handlePluginList(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.PluginRegistry == nil {
		return jsonResult(map[string]any{
			"plugins": []any{},
			"count":   0,
			"message": "plugin registry not initialized",
		}), nil
	}

	infos := s.PluginRegistry.List()
	plugins := make([]map[string]any, len(infos))
	for i, info := range infos {
		plugins[i] = map[string]any{
			"name":    info.Name,
			"version": info.Version,
			"status":  string(info.Status),
			"type":    string(info.Type),
		}
	}

	return jsonResult(map[string]any{
		"plugins": plugins,
		"count":   len(plugins),
	}), nil
}

// handlePluginInfo returns details for a specific plugin.
func (s *Server) handlePluginInfo(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "plugin name required"), nil
	}

	if s.PluginRegistry == nil {
		return codedError(ErrServiceNotFound, "plugin registry not initialized"), nil
	}

	p, ok := s.PluginRegistry.Get(name)
	if !ok {
		return codedError(ErrServiceNotFound, fmt.Sprintf("plugin not found: %s", name)), nil
	}

	status, _ := s.PluginRegistry.GetStatus(name)

	// Determine type from the full list (it includes type info).
	pluginType := "builtin"
	for _, info := range s.PluginRegistry.List() {
		if info.Name == name {
			pluginType = string(info.Type)
			break
		}
	}

	result := map[string]any{
		"name":    p.Name(),
		"version": p.Version(),
		"status":  string(status),
		"type":    pluginType,
	}

	// Include gRPC capabilities if applicable.
	grpcPlugins := s.PluginRegistry.ListGRPC()
	for _, gp := range grpcPlugins {
		if gp.Name() == name {
			result["capabilities"] = gp.Capabilities()
			break
		}
	}

	return jsonResult(result), nil
}

// handlePluginEnable enables a disabled plugin.
func (s *Server) handlePluginEnable(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "plugin name required"), nil
	}

	if s.PluginRegistry == nil {
		return codedError(ErrServiceNotFound, "plugin registry not initialized"), nil
	}

	if err := s.PluginRegistry.Enable(name); err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}

	return jsonResult(map[string]any{
		"name":    name,
		"status":  "active",
		"message": fmt.Sprintf("plugin %q enabled", name),
	}), nil
}

// handlePluginDisable disables an active plugin.
func (s *Server) handlePluginDisable(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "plugin name required"), nil
	}

	if s.PluginRegistry == nil {
		return codedError(ErrServiceNotFound, "plugin registry not initialized"), nil
	}

	if err := s.PluginRegistry.Disable(name); err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}

	return jsonResult(map[string]any{
		"name":    name,
		"status":  "disabled",
		"message": fmt.Sprintf("plugin %q disabled", name),
	}), nil
}
