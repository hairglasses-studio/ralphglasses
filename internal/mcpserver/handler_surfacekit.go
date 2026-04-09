package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) handleSurfaceAudit(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	studioRoot, surfacekitRoot, scriptPath, err := resolveSurfaceAuditPaths(s.ScanPath)
	if err != nil {
		return codedError(ErrFilesystem, err.Error()), nil
	}

	stdout, stderr, err := runSurfaceAuditCommand(ctx, studioRoot, surfacekitRoot, scriptPath)
	if err != nil {
		return codedError(ErrToolExec, formatCommandFailure("surface audit", err, stderr)), nil
	}
	if payload, ok := extractJSONPayload(stdout); ok {
		return textResult(payload), nil
	}

	stdout, stderr, err = runSurfaceAuditCommand(ctx, studioRoot, surfacekitRoot, scriptPath, "--write-json")
	if err != nil {
		return codedError(ErrToolExec, formatCommandFailure("surface audit --write-json", err, stderr)), nil
	}
	if payload, ok := extractJSONPayload(stdout); ok {
		return textResult(payload), nil
	}

	inventoryPath := filepath.Join(studioRoot, "docs", "projects", "agent-parity", "repo-inventory.json")
	data, readErr := os.ReadFile(inventoryPath)
	if readErr != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("read surface audit inventory: %v", readErr)), nil
	}

	payload := strings.TrimSpace(string(data))
	if !json.Valid([]byte(payload)) {
		return codedError(ErrToolExec, "surface audit did not produce valid JSON inventory"), nil
	}
	return textResult(payload), nil
}

func resolveSurfaceAuditPaths(scanPath string) (string, string, string, error) {
	var candidates []string
	if envRoot := strings.TrimSpace(os.Getenv("HG_STUDIO_ROOT")); envRoot != "" {
		candidates = append(candidates, envRoot)
	}
	if trimmed := strings.TrimSpace(scanPath); trimmed != "" {
		candidates = append(candidates, trimmed)
	}

	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true

		workspaceRoot := candidate
		surfacekitRoot := filepath.Join(candidate, "surfacekit")
		if filepath.Base(candidate) == "surfacekit" {
			surfacekitRoot = candidate
			workspaceRoot = filepath.Dir(candidate)
		}
		scriptPath := filepath.Join(surfacekitRoot, "scripts", "agent-parity-audit.sh")
		if _, err := os.Stat(scriptPath); err == nil {
			return workspaceRoot, surfacekitRoot, scriptPath, nil
		}

		parentSurfacekit := filepath.Join(filepath.Dir(candidate), "surfacekit")
		parentScript := filepath.Join(parentSurfacekit, "scripts", "agent-parity-audit.sh")
		if _, err := os.Stat(parentScript); err == nil {
			return filepath.Dir(candidate), parentSurfacekit, parentScript, nil
		}
	}

	return "", "", "", fmt.Errorf("surface audit script not found from scan path %q", scanPath)
}

func runSurfaceAuditCommand(ctx context.Context, studioRoot, surfacekitRoot, scriptPath string, args ...string) (string, string, error) {
	argv := append([]string{scriptPath}, args...)
	cmd := exec.CommandContext(ctx, "bash", argv...)
	cmd.Dir = surfacekitRoot
	cmd.Env = append(os.Environ(), "HG_STUDIO_ROOT="+studioRoot)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func extractJSONPayload(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	if json.Valid([]byte(trimmed)) {
		return trimmed, true
	}

	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)
	if json.Valid([]byte(trimmed)) {
		return trimmed, true
	}

	if candidate, ok := extractBracketedJSON(trimmed, '{', '}'); ok {
		return candidate, true
	}
	if candidate, ok := extractBracketedJSON(trimmed, '[', ']'); ok {
		return candidate, true
	}
	return "", false
}

func extractBracketedJSON(raw string, open, close rune) (string, bool) {
	start := strings.IndexRune(raw, open)
	end := strings.LastIndexRune(raw, close)
	if start == -1 || end == -1 || end <= start {
		return "", false
	}
	candidate := strings.TrimSpace(raw[start : end+1])
	if !json.Valid([]byte(candidate)) {
		return "", false
	}
	return candidate, true
}

func formatCommandFailure(action string, err error, stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return fmt.Sprintf("%s failed: %v", action, err)
	}
	return fmt.Sprintf("%s failed: %v: %s", action, err, stderr)
}
