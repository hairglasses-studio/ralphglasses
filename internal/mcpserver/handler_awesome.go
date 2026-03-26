package mcpserver

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/awesome"
)

// Awesome-list handlers

func (s *Server) handleAwesomeFetch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	repo := getStringArg(req, "repo")
	idx, err := awesome.Fetch(ctx, s.HTTPClient, repo)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("fetch: %v", err)), nil
	}
	return jsonResult(idx), nil
}

func (s *Server) handleAwesomeAnalyze(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	repo := getStringArg(req, "repo")
	idx, err := awesome.Fetch(ctx, s.HTTPClient, repo)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("fetch: %v", err)), nil
	}

	maxWorkers := int(getNumberArg(req, "max_workers", 5))
	analysis, err := awesome.Analyze(ctx, s.HTTPClient, idx.Entries, awesome.AnalyzeOptions{
		MaxWorkers: maxWorkers,
	})
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("analyze: %v", err)), nil
	}
	analysis.Source = idx.Source
	return jsonResult(analysis), nil
}

func (s *Server) handleAwesomeDiff(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	saveTo := getStringArg(req, "save_to")
	if saveTo == "" {
		return codedError(ErrInvalidParams, "save_to required"), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	repo := getStringArg(req, "repo")

	idx, err := awesome.Fetch(ctx, s.HTTPClient, repo)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("fetch: %v", err)), nil
	}

	prev, err := awesome.LoadIndex(saveTo)
	if err != nil {
		if os.IsNotExist(err) {
			return jsonResult(map[string]any{
				"status":  "no_data",
				"message": "Run awesome_fetch first to generate comparison data",
			}), nil
		}
		return codedError(ErrFilesystem, fmt.Sprintf("load index: %v", err)), nil
	}
	diff := awesome.Diff(prev, idx)
	return jsonResult(diff), nil
}

func (s *Server) handleAwesomeReport(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	saveTo := getStringArg(req, "save_to")
	if saveTo == "" {
		return codedError(ErrInvalidParams, "save_to required"), nil
	}

	analysis, err := awesome.LoadAnalysis(saveTo)
	if err != nil {
		if os.IsNotExist(err) {
			return jsonResult(map[string]any{
				"status":  "no_data",
				"message": "Run awesome_analyze or awesome_sync first to generate analysis data",
			}), nil
		}
		return codedError(ErrFilesystem, fmt.Sprintf("load analysis: %v", err)), nil
	}

	report := awesome.GenerateReport(analysis)
	format := getStringArg(req, "format")
	if format == "json" {
		return jsonResult(report), nil
	}
	return textResult(awesome.FormatMarkdown(report)), nil
}

func (s *Server) handleAwesomeSync(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	saveTo := getStringArg(req, "save_to")
	if saveTo == "" {
		return codedError(ErrInvalidParams, "save_to required"), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	opts := awesome.SyncOptions{
		Repo:       getStringArg(req, "repo"),
		SaveTo:     saveTo,
		FullRescan: getStringArg(req, "full_rescan") == "true",
		MaxWorkers: int(getNumberArg(req, "max_workers", 5)),
	}

	result, err := awesome.Sync(ctx, s.HTTPClient, opts)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("sync: %v", err)), nil
	}
	return jsonResult(result), nil
}
