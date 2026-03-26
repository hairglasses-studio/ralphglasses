package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleObservationQuery(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	// Apply optional filters.
	loopID := getStringArg(req, "loop_id")
	status := getStringArg(req, "status")
	provider := getStringArg(req, "provider")

	var filtered []session.LoopObservation
	for _, obs := range observations {
		if loopID != "" && obs.LoopID != loopID {
			continue
		}
		if status != "" && !strings.Contains(obs.Status, status) {
			continue
		}
		if provider != "" && !strings.Contains(obs.PlannerProvider, provider) && !strings.Contains(obs.WorkerProvider, provider) {
			continue
		}
		filtered = append(filtered, obs)
	}

	// Apply limit.
	limit := int(getNumberArg(req, "limit", 50))
	if limit > 500 {
		limit = 500
	}
	if limit < 1 {
		limit = 1
	}
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}

	if filtered == nil {
		filtered = []session.LoopObservation{}
	}

	return jsonResult(filtered), nil
}

func (s *Server) handleObservationSummary(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	// Apply optional loop_id filter.
	loopID := getStringArg(req, "loop_id")
	if loopID != "" {
		var filtered []session.LoopObservation
		for _, obs := range observations {
			if obs.LoopID == loopID {
				filtered = append(filtered, obs)
			}
		}
		observations = filtered
	}

	summary := session.SummarizeObservations(observations)
	return jsonResult(summary), nil
}
