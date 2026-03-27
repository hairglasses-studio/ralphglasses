package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/eval"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleAnomalyDetect(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}

	metricName := getStringArg(req, "metric")
	if metricName == "" {
		return codedError(ErrInvalidParams, "metric name required"), nil
	}

	// Validate metric name early.
	metrics := eval.AnomalyMetrics()
	if _, ok := metrics[metricName]; !ok {
		keys := make([]string, 0, len(metrics))
		for k := range metrics {
			keys = append(keys, k)
		}
		return codedError(ErrInvalidParams, fmt.Sprintf("unknown metric %q; valid: %v", metricName, keys)), nil
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

	hours := getNumberArg(req, "hours", 168)
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	obsPath := session.ObservationPath(r.Path)
	observations, err := session.LoadObservations(obsPath, since)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("load observations: %v", err)), nil
	}

	if len(observations) == 0 {
		return emptyResult("anomalies"), nil
	}

	anomalies, err := eval.DetectFromObservations(observations, metricName)
	if err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("anomaly detection: %v", err)), nil
	}
	if anomalies == nil {
		anomalies = []eval.Anomaly{}
	}

	return jsonResult(map[string]any{
		"repo":         repoName,
		"metric":       metricName,
		"hours":        hours,
		"observations": len(observations),
		"anomalies":    anomalies,
		"count":        len(anomalies),
	}), nil
}
