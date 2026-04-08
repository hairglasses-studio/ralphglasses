package parity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type SurfaceStatus struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
}

type RepoSurfaceAudit struct {
	RepoPath string                   `json:"repo_path"`
	Surfaces map[string]SurfaceStatus `json:"surfaces"`
	Issues   []string                 `json:"issues"`
	Warnings []string                 `json:"warnings"`
	Healthy  bool                     `json:"healthy"`
}

func AuditRepoSurface(repoPath string) RepoSurfaceAudit {
	audit := RepoSurfaceAudit{
		RepoPath: repoPath,
		Surfaces: make(map[string]SurfaceStatus),
	}

	addFile := func(name, rel string, validator func(string) (bool, string)) {
		path := filepath.Join(repoPath, rel)
		status := SurfaceStatus{Path: rel}
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			status.Exists = true
			status.Valid = true
			if validator != nil {
				status.Valid, status.Message = validator(path)
			}
		} else if err == nil {
			status.Exists = true
			status.Valid = false
			status.Message = "expected file, found directory"
		} else {
			status.Message = "missing"
		}
		audit.Surfaces[name] = status
	}

	addDir := func(name, rel string) {
		path := filepath.Join(repoPath, rel)
		status := SurfaceStatus{Path: rel}
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			status.Exists = true
			status.Valid = true
		} else if err == nil {
			status.Exists = true
			status.Valid = false
			status.Message = "expected directory, found file"
		} else {
			status.Message = "missing"
		}
		audit.Surfaces[name] = status
	}

	addFile("agents_md", "AGENTS.md", nil)
	addFile("claude_md", "CLAUDE.md", nil)
	addFile("gemini_md", "GEMINI.md", nil)
	addFile("mcp_json", ".mcp.json", validateJSONFile)
	addFile("codex_config", filepath.Join(".codex", "config.toml"), nil)
	addDir("codex_agents", filepath.Join(".codex", "agents"))
	addDir("claude_agents", filepath.Join(".claude", "agents"))
	addDir("gemini_commands", filepath.Join(".gemini", "commands"))

	if !audit.Surfaces["agents_md"].Exists {
		audit.Issues = append(audit.Issues, "missing AGENTS.md")
	}
	if !audit.Surfaces["mcp_json"].Exists {
		audit.Issues = append(audit.Issues, "missing .mcp.json")
	}
	if !audit.Surfaces["codex_config"].Exists {
		audit.Warnings = append(audit.Warnings, "missing .codex/config.toml")
	}
	if !audit.Surfaces["claude_md"].Exists {
		audit.Warnings = append(audit.Warnings, "missing CLAUDE.md")
	}
	if !audit.Surfaces["gemini_md"].Exists {
		audit.Warnings = append(audit.Warnings, "missing GEMINI.md")
	}
	audit.Healthy = len(audit.Issues) == 0
	return audit
}

func validateJSONFile(path string) (bool, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err.Error()
	}
	var tmp any
	if err := json.Unmarshal(data, &tmp); err != nil {
		return false, fmt.Sprintf("invalid JSON: %v", err)
	}
	return true, ""
}
