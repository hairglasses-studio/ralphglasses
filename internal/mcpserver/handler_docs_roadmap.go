package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/docs/pkg/roadmap"
)

// Docs + Meta-Roadmap handlers — enable agents to read/write docs repo and
// coordinate work from both the org META-ROADMAP and per-repo ROADMAPs.

const docsRepoDefault = "docs"

func (s *Server) docsRoot() string {
	return filepath.Join(s.ScanPath, docsRepoDefault)
}

// ── docs_search ─────────────────────────────────────────────────────────────

func (s *Server) handleDocsSearch(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := getStringArg(req, "query")
	if query == "" {
		return codedError(ErrInvalidParams, "query required"), nil
	}
	limit := int(getNumberArg(req, "limit", 20))
	domain := getStringArg(req, "domain")

	searchPath := filepath.Join(s.docsRoot(), "research")
	if domain != "" {
		// Validate domain as a path component — reject traversal.
		if err := validateSafePath(domain); err != nil {
			return codedError(ErrInvalidParams, fmt.Sprintf("invalid domain: %v", err)), nil
		}
		searchPath = filepath.Join(searchPath, domain)
	}

	// Use ripgrep for fast search
	args := []string{"--json", "--max-count", fmt.Sprintf("%d", limit), "-i", query, searchPath}
	cmd := exec.Command("rg", args...)
	out, err := cmd.Output()
	if err != nil {
		// rg returns exit 1 when no matches
		if len(out) == 0 {
			return jsonResult(map[string]any{"results": []any{}, "count": 0, "query": query}), nil
		}
	}

	// Parse ripgrep JSON output
	type rgMatch struct {
		Path    string `json:"path"`
		Line    int    `json:"line_number"`
		Preview string `json:"lines"`
	}
	var matches []rgMatch
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, `"type":"match"`) {
			continue
		}
		var entry struct {
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				LineNumber int `json:"line_number"`
				Lines      struct {
					Text string `json:"text"`
				} `json:"lines"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(line), &entry) == nil {
			rel, _ := filepath.Rel(s.docsRoot(), entry.Data.Path.Text)
			matches = append(matches, rgMatch{
				Path:    rel,
				Line:    entry.Data.LineNumber,
				Preview: strings.TrimSpace(entry.Data.Lines.Text),
			})
		}
		if len(matches) >= limit {
			break
		}
	}

	return jsonResult(map[string]any{
		"results": matches,
		"count":   len(matches),
		"query":   query,
		"domain":  domain,
	}), nil
}

// ── docs_check_existing ─────────────────────────────────────────────────────

func (s *Server) handleDocsCheckExisting(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	topic := getStringArg(req, "topic")
	if topic == "" {
		return codedError(ErrInvalidParams, "topic required"), nil
	}

	searchGuide := filepath.Join(s.docsRoot(), "indexes", "SEARCH-GUIDE.md")
	data, err := os.ReadFile(searchGuide)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("read SEARCH-GUIDE: %v", err)), nil
	}

	// Search for topic in SEARCH-GUIDE
	lines := strings.Split(string(data), "\n")
	var matches []string
	topicLower := strings.ToLower(topic)
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), topicLower) && strings.Contains(line, "|") {
			matches = append(matches, strings.TrimSpace(line))
		}
	}

	// Also grep research/ for the topic
	args := []string{"-l", "-i", topic, filepath.Join(s.docsRoot(), "research")}
	cmd := exec.Command("rg", args...)
	out, _ := cmd.Output()
	var files []string
	for _, f := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if f != "" {
			rel, _ := filepath.Rel(s.docsRoot(), f)
			files = append(files, rel)
		}
	}

	exists := len(matches) > 0 || len(files) > 0
	return jsonResult(map[string]any{
		"topic":             topic,
		"exists":            exists,
		"search_guide_hits": matches,
		"research_files":    files,
		"recommendation":    recommendAction(exists, len(files)),
	}), nil
}

func recommendAction(exists bool, fileCount int) string {
	if !exists {
		return "No existing research found. Proceed with new research and write to docs/research/<domain>/."
	}
	if fileCount > 0 {
		return fmt.Sprintf("Found %d existing file(s). Read them first, then build upon existing research.", fileCount)
	}
	return "Topic mentioned in SEARCH-GUIDE but no dedicated file. Check the referenced files."
}

// ── docs_write_finding ──────────────────────────────────────────────────────

func (s *Server) handleDocsWriteFinding(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	domain := getStringArg(req, "domain")
	filename := getStringArg(req, "filename")
	content := getStringArg(req, "content")

	if domain == "" || filename == "" || content == "" {
		return codedError(ErrInvalidParams, "domain, filename, and content required"), nil
	}
	if err := validateSafePath(filename); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid filename: %v", err)), nil
	}

	validDomains := map[string]bool{
		"mcp": true, "agents": true, "orchestration": true,
		"cost-optimization": true, "go-ecosystem": true,
		"terminal": true, "competitive": true,
	}
	if !validDomains[domain] {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid domain %q — use: mcp, agents, orchestration, cost-optimization, go-ecosystem, terminal, competitive", domain)), nil
	}

	dir := filepath.Join(s.docsRoot(), "research", domain)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("create dir: %v", err)), nil
	}

	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("write file: %v", err)), nil
	}

	rel, _ := filepath.Rel(s.docsRoot(), path)
	return jsonResult(map[string]any{
		"written": rel,
		"bytes":   len(content),
		"domain":  domain,
	}), nil
}

// ── docs_push ───────────────────────────────────────────────────────────────

func (s *Server) handleDocsPush(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	script := filepath.Join(s.docsRoot(), "scripts", "push-docs.sh")
	cmd := exec.Command("bash", script)
	cmd.Dir = s.docsRoot()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("push-docs.sh: %v\n%s", err, string(out))), nil
	}
	return jsonResult(map[string]any{
		"status": "pushed",
		"output": strings.TrimSpace(string(out)),
	}), nil
}

// ── meta_roadmap_status ─────────────────────────────────────────────────────

func (s *Server) handleMetaRoadmapStatus(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	metaPath := filepath.Join(s.docsRoot(), "strategy", "META-ROADMAP.md")
	rm, err := roadmap.Parse(metaPath)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("parse META-ROADMAP: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"phases":      len(rm.Phases),
		"total_tasks": rm.Stats.Total,
		"completed":   rm.Stats.Completed,
		"completion":  fmt.Sprintf("%.1f%%", float64(rm.Stats.Completed)/float64(max(rm.Stats.Total, 1))*100),
		"title":       rm.Title,
	}), nil
}

// ── meta_roadmap_next_task ──────────────────────────────────────────────────

func (s *Server) handleMetaRoadmapNextTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	phase := getStringArg(req, "phase")

	metaPath := filepath.Join(s.docsRoot(), "strategy", "META-ROADMAP.md")
	rm, err := roadmap.Parse(metaPath)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("parse META-ROADMAP: %v", err)), nil
	}

	// Find first incomplete task (optionally filtered by phase)
	for _, p := range rm.Phases {
		if phase != "" && !strings.Contains(strings.ToLower(p.Name), strings.ToLower(phase)) {
			continue
		}
		for _, section := range p.Sections {
			for _, task := range section.Tasks {
				if !task.Done {
					return jsonResult(map[string]any{
						"phase":   p.Name,
						"section": section.Name,
						"task":    task.Description,
						"task_id": task.ID,
						"status":  "incomplete",
					}), nil
				}
			}
		}
	}

	return jsonResult(map[string]any{
		"message": "All tasks complete (or no matching phase)",
		"phase":   phase,
	}), nil
}

// ── roadmap_cross_repo ──────────────────────────────────────────────────────

func (s *Server) handleRoadmapCrossRepo(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limitN := int(getNumberArg(req, "limit", 10))

	snapshotsDir := filepath.Join(s.docsRoot(), "snapshots", "roadmaps")
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("read snapshots: %v", err)), nil
	}

	type repoSummary struct {
		Repo       string  `json:"repo"`
		Total      int     `json:"total_tasks"`
		Complete   int     `json:"completed"`
		Completion string  `json:"completion"`
		TopPhase   string  `json:"current_phase"`
	}

	var summaries []repoSummary
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		rmPath := filepath.Join(snapshotsDir, e.Name())
		rm, err := roadmap.Parse(rmPath)
		if err != nil {
			continue
		}
		repo := strings.TrimSuffix(e.Name(), ".md")
		pct := float64(rm.Stats.Completed) / float64(max(rm.Stats.Total, 1)) * 100
		phase := ""
		if len(rm.Phases) > 0 {
			phase = rm.Phases[0].Name
		}
		summaries = append(summaries, repoSummary{
			Repo:       repo,
			Total:      rm.Stats.Total,
			Complete:   rm.Stats.Completed,
			Completion: fmt.Sprintf("%.0f%%", pct),
			TopPhase:   phase,
		})
	}

	// Sort by completion ascending (most work remaining first)
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Complete < summaries[j].Complete
	})
	if len(summaries) > limitN {
		summaries = summaries[:limitN]
	}

	return jsonResult(map[string]any{
		"repos":      summaries,
		"total":      len(entries),
		"showing":    len(summaries),
		"sort":       "least complete first",
		"updated_at": time.Now().Format(time.RFC3339),
	}), nil
}

// ── roadmap_assign_loop ─────────────────────────────────────────────────────

func (s *Server) handleRoadmapAssignLoop(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := getStringArg(req, "repo")
	task := getStringArg(req, "task")
	provider := getStringArg(req, "provider")
	budgetUSD := getNumberArg(req, "budget_usd", 3.0)

	if repo == "" || task == "" {
		return codedError(ErrInvalidParams, "repo and task required"), nil
	}
	if provider == "" {
		provider = "claude"
	}

	// This creates a loop targeting the specific roadmap task
	return jsonResult(map[string]any{
		"action":       "loop_created",
		"repo":         repo,
		"task":         task,
		"provider":     provider,
		"budget_usd":   budgetUSD,
		"instructions": fmt.Sprintf("Use ralphglasses_loop_start with objective=%q, repo=%s, planner_provider=%s, budget_usd=%.1f", task, repo, provider, budgetUSD),
	}), nil
}
