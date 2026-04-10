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
	studioRoot, codexkitRoot, pathErr := resolveParityAuditPaths(s.ScanPath)
	var auditFailure string
	if pathErr == nil {
		stdout, stderr, err := runParityAuditCommand(ctx, studioRoot, codexkitRoot)
		if err != nil {
			auditFailure = formatCommandFailure("workspace parity audit", err, stderr)
		} else if payload, ok := extractJSONPayload(stdout); ok {
			return textResult(payload), nil
		}

		auditFailure = formatCommandFailure("workspace parity audit", err, stderr)
	}

	payload, readErr := readParityAuditInventory(s.ScanPath)
	if readErr == nil {
		return textResult(payload), nil
	}
	if pathErr != nil {
		return codedError(ErrFilesystem, pathErr.Error()), nil
	}
	if auditFailure != "" {
		return codedError(ErrToolExec, fmt.Sprintf("%s; fallback inventory read failed: %v", auditFailure, readErr)), nil
	}
	return codedError(ErrToolExec, fmt.Sprintf("workspace parity audit did not produce valid JSON inventory; fallback inventory read failed: %v", readErr)), nil
}

func resolveParityAuditPaths(scanPath string) (string, string, error) {
	for _, candidate := range parityAuditCandidates(scanPath) {
		base := filepath.Base(candidate)
		options := []struct {
			workspaceRoot string
			codexkitRoot  string
		}{}
		switch base {
		case "codexkit":
			options = append(options, struct {
				workspaceRoot string
				codexkitRoot  string
			}{
				workspaceRoot: filepath.Dir(candidate),
				codexkitRoot:  candidate,
			})
		}
		options = append(options, struct {
			workspaceRoot string
			codexkitRoot  string
		}{workspaceRoot: candidate, codexkitRoot: filepath.Join(candidate, "codexkit")})
		for _, option := range options {
			mainPath := filepath.Join(option.codexkitRoot, "cmd", "codexkit", "main.go")
			if _, err := os.Stat(mainPath); err == nil {
				return option.workspaceRoot, option.codexkitRoot, nil
			}
		}
	}
	return "", "", fmt.Errorf("codexkit workspace refresh entrypoint not found from scan path %q", scanPath)
}

func parityAuditCandidates(scanPath string) []string {
	raw := make([]string, 0, 4)
	if envRoot := strings.TrimSpace(os.Getenv("HG_STUDIO_ROOT")); envRoot != "" {
		raw = append(raw, envRoot)
	}
	if trimmed := strings.TrimSpace(scanPath); trimmed != "" {
		raw = append(raw, trimmed, filepath.Dir(trimmed))
	}
	seen := map[string]bool{}
	candidates := make([]string, 0, len(raw))
	for _, candidate := range raw {
		candidate = filepath.Clean(candidate)
		if candidate == "" || candidate == "." || seen[candidate] {
			continue
		}
		seen[candidate] = true
		candidates = append(candidates, candidate)
	}
	return candidates
}

func readParityAuditInventory(scanPath string) (string, error) {
	for _, candidate := range parityAuditCandidates(scanPath) {
		paths := []string{filepath.Join(candidate, "docs", "projects", "agent-parity", "repo-inventory.json")}
		if filepath.Base(candidate) == "docs" {
			paths = append([]string{filepath.Join(candidate, "projects", "agent-parity", "repo-inventory.json")}, paths...)
		}
		for _, inventoryPath := range paths {
			data, err := os.ReadFile(inventoryPath)
			if err != nil {
				continue
			}
			payload := strings.TrimSpace(string(data))
			if json.Valid([]byte(payload)) {
				return payload, nil
			}
		}
	}
	return "", fmt.Errorf("read workspace parity inventory from docs/projects/agent-parity/repo-inventory.json")
}

func runParityAuditCommand(ctx context.Context, studioRoot, codexkitRoot string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/codexkit", "workspace", "refresh-parity", studioRoot)
	cmd.Dir = codexkitRoot
	cmd.Env = append(os.Environ(),
		"GOWORK=off",
		"HG_STUDIO_ROOT="+studioRoot,
		"CODEXKIT_ROOT="+codexkitRoot,
	)

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
	end := strings.LastIndex(raw, string(close))
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
