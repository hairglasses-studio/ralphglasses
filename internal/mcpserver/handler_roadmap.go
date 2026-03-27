package mcpserver

import (
	"context"
	"fmt"
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

	return jsonResult(rm), nil
}

func (s *Server) handleRoadmapAnalyze(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	analysis, err := roadmap.Analyze(rm, path)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("analyze: %v", err)), nil
	}

	// Category filter: return only a specific category
	category := getStringArg(req, "category")
	limit := int(getNumberArg(req, "limit", 20))

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

	rmPath := roadmap.ResolvePath(path, file)
	rm, err := roadmap.Parse(rmPath)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("parse roadmap: %v", err)), nil
	}

	output, err := roadmap.Export(rm, format, phase, section, maxTasks, respectDeps)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("export: %v", err)), nil
	}
	return textResult(output), nil
}
