package mcpserver

import (
	"context"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
)

// handleSelfTest handles the ralphglasses_self_test MCP tool.
// It validates parameters and prepares a self-test configuration.
// Actual iteration execution depends on Stage 1.2 binary isolation;
// this stub returns the prepared configuration as JSON.
func (s *Server) handleSelfTest(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract required param: repo path
	repo := getStringArg(req, "repo")
	if repo == "" {
		return codedError(ErrInvalidParams, "repo is required"), nil
	}

	// Validate repo path exists on disk
	info, err := os.Stat(repo)
	if err != nil {
		if os.IsNotExist(err) {
			return codedError(ErrInvalidParams, fmt.Sprintf("repo path does not exist: %s", repo)), nil
		}
		return codedError(ErrFilesystem, fmt.Sprintf("stat repo path: %v", err)), nil
	}
	if !info.IsDir() {
		return codedError(ErrInvalidParams, fmt.Sprintf("repo path is not a directory: %s", repo)), nil
	}

	// Extract optional params with defaults
	iterations := int(getNumberArg(req, "iterations", 3))
	if iterations < 1 {
		return codedError(ErrInvalidParams, "iterations must be >= 1"), nil
	}

	budgetUSD := getNumberArg(req, "budget_usd", 5.0)
	if budgetUSD <= 0 {
		return codedError(ErrInvalidParams, "budget_usd must be > 0"), nil
	}

	useSnapshot := true
	if m := argsMap(req); m != nil {
		if v, ok := m["use_snapshot"]; ok {
			if b, ok := v.(bool); ok {
				useSnapshot = b
			}
		}
	}

	dryRun := getBoolArg(req, "dry_run")

	// Stub: return prepared configuration.
	// Once Stage 1.2 binary isolation lands, this will invoke the
	// self-test runner (internal/e2e/selftest.go) to execute iterations.
	status := "prepared"
	message := fmt.Sprintf("Self-test configured for %d iterations with $%.2f budget. Execution pending Stage 1.2 binary isolation.", iterations, budgetUSD)
	if dryRun {
		status = "validated"
		message = fmt.Sprintf("Dry run: config validated for %d iterations with $%.2f budget. No iterations executed.", iterations, budgetUSD)
	}

	result := map[string]any{
		"repo":         repo,
		"iterations":   iterations,
		"budget_usd":   budgetUSD,
		"use_snapshot": useSnapshot,
		"dry_run":      dryRun,
		"status":       status,
		"message":      message,
	}

	return jsonResult(result), nil
}
