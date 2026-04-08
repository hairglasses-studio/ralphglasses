// genskillsurface writes the provider-native checked-in Ralph skill surfaces
// from the live MCP contract. This keeps `.agents/`, `.claude/`, and the local
// Codex plugin bundle aligned with the actual server surface.
//
// Usage:
//
//	go run ./tools/genskillsurface
package main

import (
	"fmt"
	"os"

	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func main() {
	srv := mcpserver.NewServer(".")
	toolDocs := make([]session.ToolDescription, 0)

	for _, group := range srv.ToolGroups() {
		for _, entry := range group.Tools {
			toolDocs = append(toolDocs, session.ToolDescription{
				Name:        entry.Tool.Name,
				Description: entry.Tool.Description,
				Namespace:   group.Name,
			})
		}
	}

	for _, entry := range srv.ManagementTools() {
		toolDocs = append(toolDocs, session.ToolDescription{
			Name:        entry.Tool.Name,
			Description: entry.Tool.Description,
			Namespace:   "management",
		})
	}

	if err := session.GenerateSkillFile(".", toolDocs); err != nil {
		fmt.Fprintf(os.Stderr, "genskillsurface: %v\n", err)
		os.Exit(1)
	}
}
