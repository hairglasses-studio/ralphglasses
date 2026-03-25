package mcpserver

import (
	"context"
	"fmt"

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
		return invalidParams(fmt.Sprintf("invalid path: %v", err)), nil
	}
	file := getStringArg(req, "file")
	rmPath := roadmap.ResolvePath(path, file)

	rm, err := roadmap.Parse(rmPath)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("parse roadmap: %v", err)), nil
	}
	return jsonResult(rm), nil
}

func (s *Server) handleRoadmapAnalyze(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return codedError(ErrInvalidParams, "path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return invalidParams(fmt.Sprintf("invalid path: %v", err)), nil
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
	return jsonResult(analysis), nil
}

func (s *Server) handleRoadmapResearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return codedError(ErrInvalidParams, "path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return invalidParams(fmt.Sprintf("invalid path: %v", err)), nil
	}
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
		return invalidParams(fmt.Sprintf("invalid path: %v", err)), nil
	}
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
	return jsonResult(expansion), nil
}

func (s *Server) handleRoadmapExport(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return codedError(ErrInvalidParams, "path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return invalidParams(fmt.Sprintf("invalid path: %v", err)), nil
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
