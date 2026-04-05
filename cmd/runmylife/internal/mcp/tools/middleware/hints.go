package middleware

import (
	"context"
	"os"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
)

var (
	hintsMap  map[string]string
	hintsOnce sync.Once
)

func buildAllHints() {
	registry := tools.GetRegistry()
	allDefs := registry.GetAllToolDefinitions()

	consumers := make(map[string][]string)
	for _, td := range allDefs {
		for _, entity := range td.ConsumesRefs {
			consumers[entity] = append(consumers[entity], td.Tool.Name)
		}
	}

	hintsMap = make(map[string]string, len(allDefs))
	for _, td := range allDefs {
		if td.Category == "discovery" || len(td.ProducesRefs) == 0 {
			continue
		}
		var suggestions []string
		seen := make(map[string]bool)
		for _, entity := range td.ProducesRefs {
			for _, consumer := range consumers[entity] {
				if consumer == td.Tool.Name || seen[consumer] {
					continue
				}
				seen[consumer] = true
				suggestions = append(suggestions, consumer)
				if len(suggestions) >= 3 {
					break
				}
			}
			if len(suggestions) >= 3 {
				break
			}
		}
		if len(suggestions) > 0 {
			hintsMap[td.Tool.Name] = "\n---\n**Next steps:** " + strings.Join(suggestions, " | ") + "\n"
		}
	}
}

func getHint(toolName string) string {
	hintsOnce.Do(buildAllHints)
	return hintsMap[toolName]
}

// HintsMiddleware appends cross-tool follow-up suggestions to successful results.
func HintsMiddleware() Middleware {
	disabled := os.Getenv("RUNMYLIFE_TOOL_HINTS") == "false"

	return func(name string, td tools.ToolDefinition, next tools.ToolHandlerFunc) tools.ToolHandlerFunc {
		if disabled || td.Category == "discovery" {
			return next
		}

		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			result, err := next(ctx, req)
			if err != nil || result == nil || result.IsError {
				return result, err
			}

			hint := getHint(name)
			if hint != "" {
				result.Content = append(result.Content, mcp.TextContent{
					Type: "text",
					Text: hint,
				})
			}
			return result, err
		}
	}
}
