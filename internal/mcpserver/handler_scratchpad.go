package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// resolveRepoPath returns the .ralph directory root for the given optional repo
// name. If repo is empty, it uses the first discovered repo or falls back to
// the ScanPath.
func (s *Server) resolveRepoPath(repo string) (string, error) {
	if repo != "" {
		if err := ValidateRepoName(repo); err != nil {
			return "", fmt.Errorf("invalid repo name: %w", err)
		}
		if s.reposNil() {
			if err := s.scan(); err != nil {
				return "", fmt.Errorf("scan failed: %w", err)
			}
		}
		r := s.findRepo(repo)
		if r == nil {
			return "", fmt.Errorf("repo not found: %s", repo)
		}
		return r.Path, nil
	}

	// No repo specified — use first discovered or ScanPath.
	if s.reposNil() {
		_ = s.scan()
	}
	repos := s.reposCopy()
	if len(repos) > 0 {
		return repos[0].Path, nil
	}
	if s.ScanPath != "" {
		return s.ScanPath, nil
	}
	return "", fmt.Errorf("no repo available")
}

func (s *Server) handleScratchpadRead(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return invalidParams("name is required"), nil
	}

	repoPath, err := s.resolveRepoPath(getStringArg(req, "repo"))
	if err != nil {
		return errResult(err.Error()), nil
	}

	path := filepath.Join(repoPath, ".ralph", name+"_scratchpad.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return textResult("No scratchpad found: " + name), nil
		}
		return errResult(fmt.Sprintf("read scratchpad: %v", err)), nil
	}
	return textResult(string(data)), nil
}

func (s *Server) handleScratchpadAppend(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return invalidParams("name is required"), nil
	}
	content := getStringArg(req, "content")
	if content == "" {
		return invalidParams("content is required"), nil
	}
	section := getStringArg(req, "section")

	repoPath, err := s.resolveRepoPath(getStringArg(req, "repo"))
	if err != nil {
		return errResult(err.Error()), nil
	}

	dir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errResult(fmt.Sprintf("create .ralph dir: %v", err)), nil
	}

	path := filepath.Join(dir, name+"_scratchpad.md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return errResult(fmt.Sprintf("open scratchpad: %v", err)), nil
	}
	defer f.Close()

	var buf strings.Builder
	if section != "" {
		buf.WriteString("\n## " + section + "\n\n")
	}
	buf.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		buf.WriteString("\n")
	}

	if _, err := f.WriteString(buf.String()); err != nil {
		return errResult(fmt.Sprintf("write scratchpad: %v", err)), nil
	}

	return textResult("Appended to " + name + " scratchpad"), nil
}

func (s *Server) handleScratchpadList(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoPath, err := s.resolveRepoPath(getStringArg(req, "repo"))
	if err != nil {
		return errResult(err.Error()), nil
	}

	pattern := filepath.Join(repoPath, ".ralph", "*_scratchpad.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return errResult(fmt.Sprintf("glob scratchpads: %v", err)), nil
	}

	names := make([]string, 0, len(matches))
	for _, m := range matches {
		base := filepath.Base(m)
		name := strings.TrimSuffix(base, "_scratchpad.md")
		names = append(names, name)
	}

	return jsonResult(names), nil
}

func (s *Server) handleScratchpadResolve(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return invalidParams("name is required"), nil
	}
	itemNum := int(getNumberArg(req, "item_number", 0))
	if itemNum == 0 {
		return invalidParams("item_number is required"), nil
	}
	resolution := getStringArg(req, "resolution")

	repoPath, err := s.resolveRepoPath(getStringArg(req, "repo"))
	if err != nil {
		return errResult(err.Error()), nil
	}

	path := filepath.Join(repoPath, ".ralph", name+"_scratchpad.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return textResult("No scratchpad found: " + name), nil
		}
		return errResult(fmt.Sprintf("read scratchpad: %v", err)), nil
	}

	prefix := strconv.Itoa(itemNum) + ". "
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			if strings.Contains(line, "RESOLVED") {
				return textResult(fmt.Sprintf("Item %d is already resolved", itemNum)), nil
			}
			suffix := " -- RESOLVED"
			if resolution != "" {
				suffix = " -- RESOLVED: " + resolution
			}
			lines[i] = line + suffix
			found = true
			break
		}
	}

	if !found {
		return errResult(fmt.Sprintf("item %d not found in scratchpad %s", itemNum, name)), nil
	}

	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return errResult(fmt.Sprintf("write scratchpad: %v", err)), nil
	}

	return textResult("Resolved item " + strconv.Itoa(itemNum)), nil
}
