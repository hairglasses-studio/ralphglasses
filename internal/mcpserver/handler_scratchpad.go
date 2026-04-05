package mcpserver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// resolveRepoPath returns the .ralph directory root for the given optional repo
// name. If repo is empty, it uses the first discovered repo or falls back to
// the ScanPath. Returns a codedError with the appropriate error code on failure.
func (s *Server) resolveRepoPath(repo string) (string, *mcp.CallToolResult) {
	if repo != "" {
		if err := ValidateRepoName(repo); err != nil {
			return "", codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err))
		}
		if s.reposNil() {
			if err := s.scan(); err != nil {
				return "", codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err))
			}
		}
		r := s.findRepo(repo)
		if r == nil {
			return "", codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", repo))
		}
		return r.Path, nil
	}

	// No repo specified — try CWD, then single discovered repo, then ScanPath.
	if s.reposNil() {
		_ = s.scan()
	}
	repos := s.reposCopy()

	if len(repos) == 1 {
		return repos[0].Path, nil
	}

	if len(repos) > 1 {
		// Try CWD path-prefix match against discovered repos.
		// EvalSymlinks handles macOS /var -> /private/var and similar.
		if cwd, err := os.Getwd(); err == nil {
			cwdReal, _ := filepath.EvalSymlinks(cwd)
			if cwdReal == "" {
				cwdReal = cwd
			}
			cwdClean := filepath.Clean(cwdReal)
			for _, r := range repos {
				rReal, _ := filepath.EvalSymlinks(r.Path)
				if rReal == "" {
					rReal = r.Path
				}
				rClean := filepath.Clean(rReal)
				if cwdClean == rClean || strings.HasPrefix(cwdClean, rClean+string(filepath.Separator)) {
					return r.Path, nil
				}
			}
		}

		// Try git rev-parse --show-toplevel to detect repo from CWD.
		if gitRoot, err := s.gitToplevel(); err == nil {
			gitRootClean := filepath.Clean(gitRoot)
			for _, r := range repos {
				if filepath.Clean(r.Path) == gitRootClean {
					return r.Path, nil
				}
			}
		}

		// No match — return actionable error with available repo names.
		names := make([]string, len(repos))
		for i, r := range repos {
			names[i] = r.Name
		}
		return "", codedError(ErrInvalidParams, fmt.Sprintf("multiple repos found, specify repo param (available: %s)", strings.Join(names, ", ")))
	}

	if s.ScanPath != "" {
		return s.ScanPath, nil
	}
	return "", codedError(ErrInvalidParams, "no repo available")
}

// gitToplevel runs "git rev-parse --show-toplevel" from CWD and returns the
// cleaned result. This is used as a fallback when CWD path-prefix matching
// fails (e.g. symlinks, bind mounts, or worktrees).
func (s *Server) gitToplevel() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *Server) handleScratchpadRead(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "name is required"), nil
	}
	if err := validateSafePath(name); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid name: %v", err)), nil
	}

	repoPath, errRes := s.resolveRepoPath(getStringArg(req, "repo"))
	if errRes != nil {
		return errRes, nil
	}

	path := filepath.Join(repoPath, ".ralph", name+"_scratchpad.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyResult("scratchpad"), nil
		}
		return codedError(ErrFilesystem, fmt.Sprintf("read scratchpad: %v", err)), nil
	}
	return textResult(string(data)), nil
}

func (s *Server) handleScratchpadAppend(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	name, errResult := p.RequireString("name")
	if errResult != nil {
		return errResult, nil
	}
	if err := validateSafePath(name); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid name: %v", err)), nil
	}
	content, errResult := p.RequireString("content")
	if errResult != nil {
		return errResult, nil
	}
	if err := ValidateStringLength(content, MaxDescriptionLength, "content"); err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}
	section := p.OptionalString("section", "")

	repoPath, errRes := s.resolveRepoPath(p.OptionalString("repo", ""))
	if errRes != nil {
		return errRes, nil
	}

	dir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("create .ralph dir: %v", err)), nil
	}

	path := filepath.Join(dir, name+"_scratchpad.md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("open scratchpad: %v", err)), nil
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
		return codedError(ErrFilesystem, fmt.Sprintf("write scratchpad: %v", err)), nil
	}

	return textResult("Appended to " + name + " scratchpad"), nil
}

func (s *Server) handleScratchpadList(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoPath, errRes := s.resolveRepoPath(getStringArg(req, "repo"))
	if errRes != nil {
		return errRes, nil
	}

	pattern := filepath.Join(repoPath, ".ralph", "*_scratchpad.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("glob scratchpads: %v", err)), nil
	}

	seen := make(map[string]struct{}, len(matches))
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		base := filepath.Base(m)
		name := strings.TrimSuffix(base, "_scratchpad.md")
		// Strip redundant _scratchpad suffix (e.g. "tool_improvement_scratchpad_scratchpad.md"
		// would become "tool_improvement_scratchpad", then we trim again).
		name = strings.TrimSuffix(name, "_scratchpad")
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}

	return jsonResult(names), nil
}

func (s *Server) handleScratchpadDelete(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "scratchpad")
	if name == "" {
		return codedError(ErrInvalidParams, "scratchpad is required"), nil
	}
	if err := validateSafePath(name); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid scratchpad name: %v", err)), nil
	}
	findingID := getStringArg(req, "finding_id")
	if findingID == "" {
		return codedError(ErrInvalidParams, "finding_id is required"), nil
	}

	repoPath, errRes := s.resolveRepoPath(getStringArg(req, "repo"))
	if errRes != nil {
		return errRes, nil
	}

	path := filepath.Join(repoPath, ".ralph", name+"_scratchpad.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return codedError(ErrFilesystem, fmt.Sprintf("scratchpad %q not found", name)), nil
		}
		return codedError(ErrFilesystem, fmt.Sprintf("read scratchpad: %v", err)), nil
	}

	prefix := findingID + ". "
	lines := strings.Split(string(data), "\n")
	found := false
	deletedSummary := ""
	var remaining []string
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			found = true
			deletedSummary = line
			continue
		}
		remaining = append(remaining, line)
	}

	if !found {
		return codedError(ErrInvalidParams, fmt.Sprintf("finding %s not found in scratchpad %s", findingID, name)), nil
	}

	if err := os.WriteFile(path, []byte(strings.Join(remaining, "\n")), 0o644); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("write scratchpad: %v", err)), nil
	}

	return textResult(fmt.Sprintf("Deleted finding %s from %s scratchpad: %s", findingID, name, deletedSummary)), nil
}

func (s *Server) handleScratchpadResolve(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "name is required"), nil
	}
	if err := validateSafePath(name); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid name: %v", err)), nil
	}
	itemNum := int(getNumberArg(req, "item_number", 0))
	if itemNum == 0 {
		return codedError(ErrInvalidParams, "item_number is required"), nil
	}
	resolution := getStringArg(req, "resolution")

	repoPath, errRes := s.resolveRepoPath(getStringArg(req, "repo"))
	if errRes != nil {
		return errRes, nil
	}

	path := filepath.Join(repoPath, ".ralph", name+"_scratchpad.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyResult("scratchpad"), nil
		}
		return codedError(ErrFilesystem, fmt.Sprintf("read scratchpad: %v", err)), nil
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
		return codedError(ErrInvalidParams, fmt.Sprintf("item %d not found in scratchpad %s", itemNum, name)), nil
	}

	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("write scratchpad: %v", err)), nil
	}

	return textResult("Resolved item " + strconv.Itoa(itemNum)), nil
}
