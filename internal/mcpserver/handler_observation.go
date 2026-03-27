package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// parseTimeBound parses an RFC3339 string into a time.Time.
// Returns zero time and nil error for empty input.
func parseTimeBound(s string, label string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s timestamp: %w", label, err)
	}
	return t, nil
}

// filterByUntil removes observations with timestamps after until.
// If until is zero-value, all observations are returned.
func filterByUntil(obs []session.LoopObservation, until time.Time) []session.LoopObservation {
	if until.IsZero() {
		return obs
	}
	var out []session.LoopObservation
	for _, o := range obs {
		if !o.Timestamp.After(until) {
			out = append(out, o)
		}
	}
	return out
}

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

	// Parse time-range params.
	sinceStr := getStringArg(req, "since")
	untilStr := getStringArg(req, "until")
	hours := getNumberArg(req, "hours", 48)

	var since time.Time
	if sinceStr != "" {
		var err error
		since, err = parseTimeBound(sinceStr, "since")
		if err != nil {
			return codedError(ErrInvalidParams, err.Error()), nil
		}
	} else {
		since = time.Now().Add(-time.Duration(hours) * time.Hour)
	}
	until, err := parseTimeBound(untilStr, "until")
	if err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}

	obsPath := session.ObservationPath(r.Path)
	observations, err := session.LoadObservations(obsPath, since)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("load observations: %v", err)), nil
	}

	observations = filterByUntil(observations, until)

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

	if len(filtered) == 0 {
		return emptyResult("observations"), nil
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

	// Parse time-range params.
	sinceStr := getStringArg(req, "since")
	untilStr := getStringArg(req, "until")
	hours := getNumberArg(req, "hours", 48)

	var since time.Time
	if sinceStr != "" {
		var err error
		since, err = parseTimeBound(sinceStr, "since")
		if err != nil {
			return codedError(ErrInvalidParams, err.Error()), nil
		}
	} else {
		since = time.Now().Add(-time.Duration(hours) * time.Hour)
	}
	until, err := parseTimeBound(untilStr, "until")
	if err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}

	obsPath := session.ObservationPath(r.Path)
	observations, err := session.LoadObservations(obsPath, since)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("load observations: %v", err)), nil
	}

	observations = filterByUntil(observations, until)

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

	// Backfill acceptance_counts and model_usage from provider fields when the
	// dedicated fields (AcceptancePath, PlannerModelUsed, WorkerModelUsed) are
	// not set in live-loop observations.
	if len(summary.AcceptanceCounts) == 0 {
		counts := make(map[string]int)
		for _, o := range observations {
			switch o.Status {
			case "idle":
				if o.VerifyPassed {
					counts["auto_merge"]++
				} else {
					counts["no_change"]++
				}
			case "failed":
				counts["rejected"]++
			default:
				counts["unknown"]++
			}
		}
		summary.AcceptanceCounts = counts
	}
	if len(summary.ModelUsage) == 0 {
		usage := make(map[string]int)
		for _, o := range observations {
			if o.PlannerProvider != "" {
				usage[o.PlannerProvider]++
			}
			if o.WorkerProvider != "" {
				usage[o.WorkerProvider]++
			}
		}
		summary.ModelUsage = usage
	}

	return jsonResult(summary), nil
}
