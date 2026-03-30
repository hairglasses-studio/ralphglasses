package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// handleSkillExport generates SKILL.md content from registered tool groups.
// Optional params:
//   - format: "markdown" (default) or "json"
//   - group: filter to a specific namespace
func (s *Server) handleSkillExport(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	format := getStringArg(req, "format")
	if format == "" {
		format = "markdown"
	}
	groupFilter := getStringArg(req, "group")

	groups := s.buildToolGroups()

	// Filter to a specific group if requested.
	if groupFilter != "" {
		var filtered []ToolGroup
		for _, g := range groups {
			if g.Name == groupFilter {
				filtered = append(filtered, g)
				break
			}
		}
		if len(filtered) == 0 {
			return codedError(ErrInvalidParams, fmt.Sprintf("unknown group %q", groupFilter)), nil
		}
		groups = filtered
	}

	switch format {
	case "json":
		skills := ExportSkillsFromGroups(groups)
		data, err := json.MarshalIndent(skills, "", "  ")
		if err != nil {
			return codedError(ErrInternal, err.Error()), nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(data)}},
		}, nil

	case "markdown":
		md := ExportSkillMarkdown(groups)
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: md}},
		}, nil

	default:
		return codedError(ErrInvalidParams, fmt.Sprintf("unsupported format %q (use \"markdown\" or \"json\")", format)), nil
	}
}
