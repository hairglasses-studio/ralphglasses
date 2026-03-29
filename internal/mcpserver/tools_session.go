package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// resolveSnapshotRepo determines the target repo for snapshot storage.
// Priority: 1) explicit repo param, 2) CWD match against scanned repos, 3) first repo (fallback).
func resolveSnapshotRepo(allRepos []*model.Repo, repoName string, findRepo func(string) *model.Repo) *model.Repo {
	if len(allRepos) == 0 {
		return nil
	}

	// 1. Explicit repo parameter
	if repoName != "" {
		if r := findRepo(repoName); r != nil {
			return r
		}
	}

	// 2. Match CWD against scanned repo paths (longest-path wins with boundary check).
	if cwd, err := os.Getwd(); err == nil {
		var best *model.Repo
		bestLen := 0
		for _, r := range allRepos {
			rp := r.Path
			if strings.HasPrefix(cwd, rp) {
				// Boundary check: CWD must equal the repo path exactly or
				// the character immediately after the prefix must be a path
				// separator. Without this, /repos/foo matches /repos/foobar.
				if len(cwd) == len(rp) || cwd[len(rp)] == filepath.Separator {
					if len(rp) > bestLen {
						best = r
						bestLen = len(rp)
					}
				}
			}
		}
		if best != nil {
			return best
		}
	}

	// 3. Fallback to first repo
	return allRepos[0]
}

func (s *Server) handleWorkflowDefine(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	name := getStringArg(req, "name")
	yamlStr := getStringArg(req, "yaml")
	if repoName == "" || name == "" || yamlStr == "" {
		return codedError(ErrInvalidParams, "repo, name, and yaml are required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", repoName)), nil
	}

	wf, err := session.ParseWorkflow(name, []byte(yamlStr))
	if err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid workflow YAML: %v", err)), nil
	}

	if err := session.SaveWorkflow(r.Path, wf); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("save failed: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"name":  wf.Name,
		"steps": len(wf.Steps),
		"saved": true,
	}), nil
}

func (s *Server) handleWorkflowRun(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	name := getStringArg(req, "name")
	if repoName == "" || name == "" {
		return codedError(ErrInvalidParams, "repo and name are required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", repoName)), nil
	}

	wf, err := session.LoadWorkflow(r.Path, name)
	if err != nil {
		return codedError(ErrWorkflow, fmt.Sprintf("load workflow: %v", err)), nil
	}

	run, err := s.SessMgr.RunWorkflow(ctx, r.Path, *wf)
	if err != nil {
		return codedError(ErrWorkflow, fmt.Sprintf("run workflow: %v", err)), nil
	}

	run.Lock()
	result := map[string]any{
		"run_id":     run.ID,
		"workflow":   run.Name,
		"repo_path":  run.RepoPath,
		"status":     run.Status,
		"created_at": run.CreatedAt,
		"updated_at": run.UpdatedAt,
		"steps":      append([]session.WorkflowStepResult(nil), run.Steps...),
	}
	run.Unlock()

	return jsonResult(result), nil
}

func (s *Server) handleWorkflowDelete(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	name := getStringArg(req, "name")
	if repoName == "" || name == "" {
		return codedError(ErrInvalidParams, "repo and name are required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", repoName)), nil
	}

	if err := session.DeleteWorkflow(r.Path, name); err != nil {
		return codedError(ErrWorkflow, fmt.Sprintf("delete workflow: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"name":    name,
		"deleted": true,
	}), nil
}

func (s *Server) handleSnapshot(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Apply a 30-second timeout for snapshot operations.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	action := getStringArg(req, "action")
	if action == "" {
		action = "save"
	}
	if action != "save" && action != "list" {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid action %q: must be 'save' or 'list'", action)), nil
	}
	name := getStringArg(req, "name")

	if action == "list" {
		if s.reposNil() {
			if err := s.scan(); err != nil {
				return codedError(ErrInternal, fmt.Sprintf("scan failed: %v", err)), nil
			}
		}
		if err := ctx.Err(); err != nil {
			return codedError(ErrInternal, "snapshot list timed out"), nil
		}
		allRepos := s.reposCopy()
		var snapshots []string
		for _, r := range allRepos {
			snaps, _ := filepath.Glob(filepath.Join(r.Path, ".ralph", "snapshots", "*.json"))
			for _, snap := range snaps {
				snapshots = append(snapshots, filepath.Base(snap))
			}
		}
		if len(snapshots) == 0 {
			return emptyResult("snapshots"), nil
		}
		return jsonResult(map[string]any{"snapshots": snapshots}), nil
	}

	// Save snapshot
	if name == "" {
		name = fmt.Sprintf("snapshot-%s", time.Now().Format("20060102-150405"))
	}

	allSessions := s.SessMgr.List("")
	type sessionSnap struct {
		ID       string  `json:"id"`
		Provider string  `json:"provider"`
		Repo     string  `json:"repo"`
		Status   string  `json:"status"`
		SpentUSD float64 `json:"spent_usd"`
		Turns    int     `json:"turns"`
	}
	var sessSnaps []sessionSnap
	for _, sess := range allSessions {
		sess.Lock()
		sessSnaps = append(sessSnaps, sessionSnap{
			ID:       sess.ID,
			Provider: string(sess.Provider),
			Repo:     sess.RepoName,
			Status:   string(sess.Status),
			SpentUSD: sess.SpentUSD,
			Turns:    sess.TurnCount,
		})
		sess.Unlock()
	}

	if err := ctx.Err(); err != nil {
		return codedError(ErrInternal, "snapshot save timed out"), nil
	}

	teams := s.SessMgr.ListTeams()
	now := time.Now()
	snapshot := map[string]any{
		"name":      name,
		"timestamp": now.Format(time.RFC3339),
		"sessions":  sessSnaps,
		"teams":     teams,
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("json marshal: %v", err)), nil
	}

	// Resolve target repo for snapshot storage.
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrInternal, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	allRepos := s.reposCopy()
	if len(allRepos) == 0 {
		return codedError(ErrFilesystem, "no repos found; cannot save snapshot"), nil
	}

	targetRepo := resolveSnapshotRepo(allRepos, getStringArg(req, "repo"), s.findRepo)
	dir := filepath.Join(targetRepo.Path, ".ralph", "snapshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("create snapshot dir: %v", err)), nil
	}
	snapshotPath := filepath.Join(dir, name+".json")
	if err := os.WriteFile(snapshotPath, data, 0o644); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("write snapshot: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"name":          name,
		"path":          snapshotPath,
		"size_bytes":    len(data),
		"timestamp":     now.Format(time.RFC3339),
		"session_count": len(sessSnaps),
		"team_count":    len(teams),
	}), nil
}

func (s *Server) handleJournalRead(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", name)), nil
	}

	limit := int(getNumberArg(req, "limit", 10))
	entries, err := session.ReadRecentJournal(r.Path, limit)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("read journal: %v", err)), nil
	}

	if len(entries) == 0 {
		return emptyResult("journal_entries"), nil
	}

	synthesis := session.SynthesizeContext(entries)

	return jsonResult(map[string]any{
		"entries":   entries,
		"count":     len(entries),
		"synthesis": synthesis,
	}), nil
}

func (s *Server) handleJournalWrite(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", name)), nil
	}

	entry := session.JournalEntry{
		Timestamp: time.Now(),
		SessionID: getStringArg(req, "session_id"),
		RepoName:  r.Name,
	}
	if w := getStringArg(req, "worked"); w != "" {
		entry.Worked = splitCSV(w)
	}
	if f := getStringArg(req, "failed"); f != "" {
		entry.Failed = splitCSV(f)
	}
	if sg := getStringArg(req, "suggest"); sg != "" {
		entry.Suggest = splitCSV(sg)
	}

	if err := session.WriteJournalEntryManual(r.Path, entry); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("write journal: %v", err)), nil
	}

	if s.EventBus != nil {
		s.EventBus.Publish(events.Event{
			Type:      events.JournalWritten,
			RepoName:  r.Name,
			RepoPath:  r.Path,
			SessionID: entry.SessionID,
		})
	}

	return jsonResult(map[string]any{
		"status":  "written",
		"repo":    r.Name,
		"worked":  len(entry.Worked),
		"failed":  len(entry.Failed),
		"suggest": len(entry.Suggest),
	}), nil
}

func (s *Server) handleJournalPrune(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", name)), nil
	}

	keep := int(getNumberArg(req, "keep", 100))
	dryRun := getStringArg(req, "dry_run") != "false"

	// Read current count
	entries, err := session.ReadRecentJournal(r.Path, 100000)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("read journal: %v", err)), nil
	}

	wouldPrune := len(entries) - keep
	if wouldPrune < 0 {
		wouldPrune = 0
	}

	if dryRun {
		return jsonResult(map[string]any{
			"dry_run":     true,
			"total":       len(entries),
			"keep":        keep,
			"would_prune": wouldPrune,
		}), nil
	}

	pruned, err := session.PruneJournal(r.Path, keep)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("prune journal: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"dry_run":   false,
		"pruned":    pruned,
		"remaining": len(entries) - pruned,
	}), nil
}
