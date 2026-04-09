package mcpserver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) handleSurfaceAudit(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	studioRoot := getStudioRoot()
	scriptPath := filepath.Join(studioRoot, "surfacekit/scripts/agent-parity-audit.sh")

	cmd := exec.CommandContext(ctx, "bash", scriptPath, "--write-json")
	cmd.Dir = filepath.Dir(scriptPath)
	cmd.Env = append(cmd.Environ(), "HG_STUDIO_ROOT="+studioRoot)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return codedError(ErrToolExec, fmt.Sprintf("surface audit failed: %v\nOutput: %s", err, string(output))), nil
	}

	inventoryPath := filepath.Join(studioRoot, "docs/projects/agent-parity/repo-inventory.json")
	inventory, err := os.ReadFile(inventoryPath)
	if err != nil {
		return jsonResult(map[string]any{
			"message": "Audit completed but JSON output not found",
			"output":  string(output),
		}), nil
	}

	return jsonResult(map[string]any{
		"message":   "Surface audit completed successfully",
		"inventory": string(inventory),
	}), nil
}

func getStudioRoot() string {
	if root := os.Getenv("HG_STUDIO_ROOT"); root != "" {
		return root
	}
	return filepath.Join(os.Getenv("HOME"), "hairglasses-studio")
}
