package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleLoopBenchmark(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
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

	hours := getNumberArg(req, "hours", 48)
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	obsPath := session.ObservationPath(r.Path)
	observations, err := session.LoadObservations(obsPath, since)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("load observations: %v", err)), nil
	}

	if len(observations) == 0 {
		return jsonResult(map[string]any{
			"repo":         repoName,
			"hours":        hours,
			"observations": 0,
			"message":      "no observations in window",
		}), nil
	}

	bl := e2e.BuildBaseline(observations, 0)

	result := map[string]any{
		"repo":         repoName,
		"hours":        hours,
		"observations": len(observations),
	}
	if bl.Aggregate != nil {
		result["cost_p50"] = bl.Aggregate.CostP50
		result["cost_p95"] = bl.Aggregate.CostP95
		result["latency_p50_ms"] = bl.Aggregate.LatencyP50
		result["latency_p95_ms"] = bl.Aggregate.LatencyP95
	}
	if bl.Rates != nil {
		result["completion_rate"] = bl.Rates.CompletionRate
		result["verify_pass_rate"] = bl.Rates.VerifyPassRate
		result["error_rate"] = bl.Rates.ErrorRate
	}
	result["per_scenario"] = bl.Entries

	return jsonResult(result), nil
}

func (s *Server) handleLoopBaseline(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
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

	action := getStringArg(req, "action")
	if action == "" {
		action = "view"
	}

	blPath := e2e.BaselinePath(r.Path)

	switch action {
	case "view":
		bl, err := e2e.LoadBaseline(blPath)
		if err != nil {
			return codedError(ErrFilesystem, fmt.Sprintf("load baseline: %v", err)), nil
		}
		return jsonResult(bl), nil

	case "refresh":
		hours := getNumberArg(req, "hours", 48)
		bl, err := e2e.RefreshBaseline(r.Path, hours)
		if err != nil {
			return codedError(ErrGateFailed, fmt.Sprintf("refresh baseline: %v", err)), nil
		}
		if err := e2e.SaveBaseline(blPath, bl); err != nil {
			return codedError(ErrFilesystem, fmt.Sprintf("save baseline: %v", err)), nil
		}
		return jsonResult(map[string]any{
			"action":  "refresh",
			"path":    blPath,
			"samples": bl.Aggregate.SampleCount,
		}), nil

	case "pin":
		hours := getNumberArg(req, "hours", 48)
		bl, err := e2e.RefreshBaseline(r.Path, hours)
		if err != nil {
			return codedError(ErrGateFailed, fmt.Sprintf("refresh baseline: %v", err)), nil
		}
		if err := e2e.SaveBaseline(blPath, bl); err != nil {
			return codedError(ErrFilesystem, fmt.Sprintf("save baseline: %v", err)), nil
		}
		return jsonResult(map[string]any{
			"action":  "pin",
			"path":    blPath,
			"samples": bl.Aggregate.SampleCount,
			"message": "baseline pinned — future observations measured against this snapshot",
		}), nil

	default:
		return codedError(ErrInvalidParams, fmt.Sprintf("unknown action: %s (use view, refresh, or pin)", action)), nil
	}
}

func (s *Server) handleLoopGates(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
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

	hours := getNumberArg(req, "hours", 24)
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	obsPath := session.ObservationPath(r.Path)
	observations, err := session.LoadObservations(obsPath, since)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("load observations: %v", err)), nil
	}

	blPath := e2e.BaselinePath(r.Path)
	baseline, _ := e2e.LoadBaseline(blPath) // nil baseline is ok — gates still evaluate rates

	thresholds := e2e.DefaultGateThresholds()
	report := e2e.EvaluateGates(observations, baseline, thresholds)

	return jsonResult(report), nil
}
