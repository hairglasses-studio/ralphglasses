package mcpserver

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/roadmap"
)

// Roadmap handlers

func (s *Server) handleRoadmapParse(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return codedError(ErrInvalidParams, "path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid path: %v", err)), nil
	}
	file := getStringArg(req, "file")
	rmPath := roadmap.ResolvePath(path, file)

	rm, err := roadmap.Parse(rmPath)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("parse roadmap: %v", err)), nil
	}

	// Phase filter: return only a specific phase
	phaseFilter := getStringArg(req, "phase")
	if phaseFilter != "" {
		var filtered []roadmap.Phase
		for _, p := range rm.Phases {
			if p.Name == phaseFilter {
				filtered = append(filtered, p)
			}
		}
		rm.Phases = filtered
	}

	// max_depth: 0=phases only, 1=phases+sections, 2=full (default 2)
	maxDepth := int(getNumberArg(req, "max_depth", 2))

	// summary_only: return compact summary instead of full task details
	summaryOnly := getBoolArg(req, "summary_only")
	if summaryOnly {
		type phaseSummary struct {
			Name            string  `json:"name"`
			SectionCount    int     `json:"section_count"`
			TaskCount       int     `json:"task_count"`
			CompletedCount  int     `json:"completed_count"`
			CompletionPct   float64 `json:"completion_pct"`
		}
		type summary struct {
			Title      string         `json:"title"`
			PhaseCount int            `json:"phase_count"`
			TotalTasks int            `json:"total_tasks"`
			Completed  int            `json:"completed"`
			Phases     []phaseSummary `json:"phases"`
		}
		s := summary{
			Title:      rm.Title,
			PhaseCount: len(rm.Phases),
			TotalTasks: rm.Stats.Total,
			Completed:  rm.Stats.Completed,
		}
		for _, p := range rm.Phases {
			ps := phaseSummary{
				Name:           p.Name,
				SectionCount:   len(p.Sections),
				TaskCount:      p.Stats.Total,
				CompletedCount: p.Stats.Completed,
			}
			if p.Stats.Total > 0 {
				ps.CompletionPct = float64(p.Stats.Completed) / float64(p.Stats.Total) * 100
			}
			s.Phases = append(s.Phases, ps)
		}
		return jsonResult(s), nil
	}

	// Apply max_depth truncation
	if maxDepth < 2 {
		for i := range rm.Phases {
			if maxDepth == 0 {
				rm.Phases[i].Sections = nil
			} else {
				// maxDepth == 1: keep sections but strip tasks
				for j := range rm.Phases[i].Sections {
					rm.Phases[i].Sections[j].Tasks = nil
				}
			}
		}
	}

	return jsonResult(rm), nil
}

// relevanceScore computes Jaccard similarity between a task title and a query string.
// Returns 0 if either is empty.
func relevanceScore(taskTitle, query string) float64 {
	titleWords := strings.Fields(strings.ToLower(taskTitle))
	queryWords := strings.Fields(strings.ToLower(query))
	if len(titleWords) == 0 || len(queryWords) == 0 {
		return 0
	}
	titleSet := make(map[string]bool, len(titleWords))
	for _, w := range titleWords {
		titleSet[w] = true
	}
	querySet := make(map[string]bool, len(queryWords))
	for _, w := range queryWords {
		querySet[w] = true
	}
	intersection := 0
	for w := range querySet {
		if titleSet[w] {
			intersection++
		}
	}
	union := len(titleSet) + len(querySet) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func (s *Server) handleRoadmapAnalyze(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	path, errResult := p.RequireString("path")
	if errResult != nil {
		return errResult, nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid path: %v", err)), nil
	}
	file := p.OptionalString("file", "")
	rmPath := roadmap.ResolvePath(path, file)

	rm, err := roadmap.Parse(rmPath)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("parse roadmap: %v", err)), nil
	}

	analysis, err := roadmap.Analyze(rm, path)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("analyze: %v", err)), nil
	}

	// Query-based relevance sorting: if a query is provided, sort ready items
	// by relevance score (descending) instead of using a flat score.
	query := p.OptionalString("query", "")
	if query != "" {
		sort.Slice(analysis.Ready, func(i, j int) bool {
			return relevanceScore(analysis.Ready[i].Description, query) > relevanceScore(analysis.Ready[j].Description, query)
		})
		sort.Slice(analysis.Gaps, func(i, j int) bool {
			return relevanceScore(analysis.Gaps[i].Description, query) > relevanceScore(analysis.Gaps[j].Description, query)
		})
	}

	// Category filter: return only a specific category
	category := p.OptionalString("category", "")
	limit := int(p.OptionalNumber("limit", 20))

	if category != "" || limit > 0 {
		// Apply category filter
		switch category {
		case "gaps":
			if limit > 0 && len(analysis.Gaps) > limit {
				analysis.Gaps = analysis.Gaps[:limit]
			}
			return jsonResult(map[string]any{
				"gaps":    analysis.Gaps,
				"summary": analysis.Summary,
			}), nil
		case "stale":
			if limit > 0 && len(analysis.Stale) > limit {
				analysis.Stale = analysis.Stale[:limit]
			}
			return jsonResult(map[string]any{
				"stale":   analysis.Stale,
				"summary": analysis.Summary,
			}), nil
		case "orphaned":
			if limit > 0 && len(analysis.Orphaned) > limit {
				analysis.Orphaned = analysis.Orphaned[:limit]
			}
			return jsonResult(map[string]any{
				"orphaned": analysis.Orphaned,
				"summary":  analysis.Summary,
			}), nil
		case "ready":
			if limit > 0 && len(analysis.Ready) > limit {
				analysis.Ready = analysis.Ready[:limit]
			}
			return jsonResult(map[string]any{
				"ready":   analysis.Ready,
				"summary": analysis.Summary,
			}), nil
		default:
			// No category filter or unknown category — apply limit to all
			if limit > 0 {
				if len(analysis.Gaps) > limit {
					analysis.Gaps = analysis.Gaps[:limit]
				}
				if len(analysis.Stale) > limit {
					analysis.Stale = analysis.Stale[:limit]
				}
				if len(analysis.Orphaned) > limit {
					analysis.Orphaned = analysis.Orphaned[:limit]
				}
				if len(analysis.Ready) > limit {
					analysis.Ready = analysis.Ready[:limit]
				}
			}
		}
	}

	return jsonResult(analysis), nil
}

func (s *Server) handleRoadmapResearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return codedError(ErrInvalidParams, "path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid path: %v", err)), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	topics := getStringArg(req, "topics")
	limit := int(getNumberArg(req, "limit", 10))

	results, err := roadmap.Research(ctx, s.HTTPClient, path, topics, limit)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("research: %v", err)), nil
	}
	return jsonResult(results), nil
}

func (s *Server) handleRoadmapExpand(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return codedError(ErrInvalidParams, "path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid path: %v", err)), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	file := getStringArg(req, "file")
	style := getStringArg(req, "style")
	researchTopics := getStringArg(req, "research")

	rmPath := roadmap.ResolvePath(path, file)
	rm, err := roadmap.Parse(rmPath)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("parse roadmap: %v", err)), nil
	}

	analysis, err := roadmap.Analyze(rm, path)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("analyze: %v", err)), nil
	}

	var research *roadmap.ResearchResults
	if researchTopics != "" {
		research, _ = roadmap.Research(ctx, s.HTTPClient, path, researchTopics, 10)
	}

	expansion, err := roadmap.Expand(rm, analysis, research, style)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("expand: %v", err)), nil
	}

	// Phase filter: return only proposals for a specific phase
	phaseFilter := getStringArg(req, "phase")
	if phaseFilter != "" {
		var filtered []roadmap.Proposal
		for _, p := range expansion.Proposals {
			if p.Phase == phaseFilter {
				filtered = append(filtered, p)
			}
		}
		expansion.Proposals = filtered
	}

	// Limit: cap number of proposals
	expandLimit := int(getNumberArg(req, "limit", 20))
	if expandLimit > 0 && len(expansion.Proposals) > expandLimit {
		expansion.Proposals = expansion.Proposals[:expandLimit]
	}

	return jsonResult(expansion), nil
}

func (s *Server) handleRoadmapExport(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return codedError(ErrInvalidParams, "path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid path: %v", err)), nil
	}
	file := getStringArg(req, "file")
	format := getStringArg(req, "format")
	phase := getStringArg(req, "phase")
	section := getStringArg(req, "section")
	maxTasks := int(getNumberArg(req, "max_tasks", 20))
	respectDeps := getStringArg(req, "respect_deps") != "false"
	statusFilter := getStringArg(req, "status")
	if statusFilter == "" {
		statusFilter = "incomplete"
	}

	rmPath := roadmap.ResolvePath(path, file)
	rm, err := roadmap.Parse(rmPath)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("parse roadmap: %v", err)), nil
	}

	// Apply status filter by pre-filtering tasks in the roadmap
	if statusFilter != "all" {
		filtered := filterRoadmapByStatus(rm, statusFilter)
		rm = filtered
	} else {
		// For "all", sort incomplete first
		sortRoadmapIncompleteFirst(rm)
	}

	// Generate unique task IDs: assign path-based IDs to tasks without IDs
	assignUniqueTaskIDs(rm)

	output, err := roadmap.Export(rm, format, phase, section, maxTasks, respectDeps)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("export: %v", err)), nil
	}
	return textResult(output), nil
}

// filterRoadmapByStatus returns a copy of the roadmap with tasks filtered by status.
func filterRoadmapByStatus(rm *roadmap.Roadmap, status string) *roadmap.Roadmap {
	wantDone := status == "complete"
	out := &roadmap.Roadmap{
		Title: rm.Title,
		Stats: rm.Stats,
	}
	for _, p := range rm.Phases {
		np := roadmap.Phase{Name: p.Name, Stats: p.Stats}
		for _, s := range p.Sections {
			ns := roadmap.Section{Name: s.Name, Acceptance: s.Acceptance}
			for _, t := range s.Tasks {
				if t.Done == wantDone {
					ns.Tasks = append(ns.Tasks, t)
				}
			}
			if len(ns.Tasks) > 0 {
				np.Sections = append(np.Sections, ns)
			}
		}
		if len(np.Sections) > 0 {
			out.Phases = append(out.Phases, np)
		}
	}
	return out
}

// sortRoadmapIncompleteFirst sorts tasks within each section so incomplete tasks come first.
func sortRoadmapIncompleteFirst(rm *roadmap.Roadmap) {
	for i := range rm.Phases {
		for j := range rm.Phases[i].Sections {
			tasks := rm.Phases[i].Sections[j].Tasks
			sort.SliceStable(tasks, func(a, b int) bool {
				if tasks[a].Done != tasks[b].Done {
					return !tasks[a].Done // incomplete first
				}
				return false
			})
		}
	}
}

// assignUniqueTaskIDs assigns path-based IDs to tasks that don't have one.
// Uses a global task counter per phase to ensure uniqueness even when the parser
// creates multiple implicit sections with the same name.
func assignUniqueTaskIDs(rm *roadmap.Roadmap) {
	for pi := range rm.Phases {
		pName := strings.ReplaceAll(rm.Phases[pi].Name, "/", "_")
		taskIdx := 0
		for si := range rm.Phases[pi].Sections {
			sName := strings.ReplaceAll(rm.Phases[pi].Sections[si].Name, "/", "_")
			for ti := range rm.Phases[pi].Sections[si].Tasks {
				if rm.Phases[pi].Sections[si].Tasks[ti].ID == "" {
					rm.Phases[pi].Sections[si].Tasks[ti].ID = fmt.Sprintf("%s/%s/%d", pName, sName, taskIdx)
				}
				taskIdx++
			}
		}
	}
}
