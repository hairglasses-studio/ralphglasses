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
//   - group: filter to a specific tool group (including "management")
func (s *Server) handleSkillExport(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	format := getStringArg(req, "format")
	if format == "" {
		format = "markdown"
	}
	groupFilter := getStringArg(req, "group")

	groups := s.buildToolGroups()
	management := s.ManagementTools()

	// Filter to a specific group if requested.
	if groupFilter != "" {
		if groupFilter == "management" {
			groups = nil
		} else {
			management = nil
		}
		var filtered []ToolGroup
		if groupFilter != "management" {
			for _, g := range groups {
				if g.Name == groupFilter {
					filtered = append(filtered, g)
					break
				}
			}
		}
		if groupFilter != "management" {
			if len(filtered) == 0 {
				return codedError(ErrInvalidParams, fmt.Sprintf("unknown group %q", groupFilter)), nil
			}
			groups = filtered
		}
	}

	switch format {
	case "json":
		skills := ExportSkillsFromContract(groups, management)
		data, err := json.MarshalIndent(skills, "", "  ")
		if err != nil {
			return codedError(ErrInternal, err.Error()), nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(data)}},
		}, nil

	case "markdown":
		md := ExportSkillMarkdownFromContract(groups, management)
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: md}},
		}, nil

	default:
		return codedError(ErrInvalidParams, fmt.Sprintf("unsupported format %q (use \"markdown\" or \"json\")", format)), nil
	}
}
