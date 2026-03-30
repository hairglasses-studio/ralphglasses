package mcpserver

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
)

func (s *Server) handleFleetGrafana(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title := getStringArg(req, "title")
	if title == "" {
		title = "Ralphglasses Fleet Metrics"
	}

	datasource := getStringArg(req, "datasource")

	var metrics []string
	metricsRaw := getStringArg(req, "metrics")
	if metricsRaw != "" {
		for _, m := range strings.Split(metricsRaw, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				metrics = append(metrics, m)
			}
		}
	}

	dash := fleet.ExportDashboard(title, metrics, datasource)
	data, err := fleet.ToJSON(dash)
	if err != nil {
		return codedError(ErrInternal, "failed to serialize dashboard: "+err.Error()), nil
	}
	return textResult(string(data)), nil
}
