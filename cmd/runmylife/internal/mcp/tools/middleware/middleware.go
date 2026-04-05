// Package middleware provides composable middleware for MCP tool handlers.
package middleware

import (
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
)

// Middleware wraps a tool handler, adding cross-cutting behavior.
type Middleware func(name string, td tools.ToolDefinition, next tools.ToolHandlerFunc) tools.ToolHandlerFunc
