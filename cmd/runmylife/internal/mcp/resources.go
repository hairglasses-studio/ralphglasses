package mcp

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// RegisterResources registers all MCP resources with the server.
func RegisterResources(s *server.MCPServer) {
	s.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"runmylife://tasks/{id}",
			"task",
			mcp.WithTemplateDescription("Get task by ID"),
			mcp.WithTemplateMIMEType("application/json"),
		),
		handleTaskResource,
	)

	s.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"runmylife://calendar/today",
			"today-events",
			mcp.WithTemplateDescription("Get today's calendar events"),
			mcp.WithTemplateMIMEType("application/json"),
		),
		handleTodayResource,
	)

	s.AddResource(
		mcp.NewResource(
			"runmylife://config/profile",
			"config-profile",
			mcp.WithResourceDescription("Current configuration profile"),
			mcp.WithMIMEType("application/json"),
		),
		handleConfigResource,
	)
}

func handleTaskResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	database, err := common.OpenDB()
	if err != nil {
		return errorResource(request.Params.URI, err)
	}
	defer database.Close()

	// Extract ID from URI (last path segment)
	uri := request.Params.URI
	var id, title, project, dueDate string
	var priority, completed int
	err = database.SqlDB().QueryRowContext(ctx,
		"SELECT id, title, priority, project, due_date, completed FROM tasks WHERE id LIKE ?",
		uri[len("runmylife://tasks/"):]+"%",
	).Scan(&id, &title, &priority, &project, &dueDate, &completed)
	if err != nil {
		return errorResource(uri, err)
	}

	data, _ := json.MarshalIndent(map[string]interface{}{
		"id": id, "title": title, "priority": priority,
		"project": project, "due_date": dueDate, "completed": completed == 1,
	}, "", "  ")

	return []mcp.ResourceContents{
		mcp.TextResourceContents{URI: uri, MIMEType: "application/json", Text: string(data)},
	}, nil
}

func handleTodayResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	database, err := common.OpenDB()
	if err != nil {
		return errorResource(request.Params.URI, err)
	}
	defer database.Close()

	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT summary, start_time, end_time, location FROM calendar_events WHERE date(start_time) = date('now') ORDER BY start_time",
	)
	if err != nil {
		return errorResource(request.Params.URI, err)
	}
	defer rows.Close()

	var events []map[string]string
	for rows.Next() {
		var summary, start, end, location string
		if err := rows.Scan(&summary, &start, &end, &location); err != nil {
			continue
		}
		events = append(events, map[string]string{
			"summary": summary, "start": start, "end": end, "location": location,
		})
	}

	data, _ := json.MarshalIndent(events, "", "  ")
	return []mcp.ResourceContents{
		mcp.TextResourceContents{URI: request.Params.URI, MIMEType: "application/json", Text: string(data)},
	}, nil
}

func handleConfigResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	data, _ := json.MarshalIndent(map[string]string{
		"config_path": "~/.config/runmylife/config.json",
		"db_path":     "~/.config/runmylife/runmylife.db",
	}, "", "  ")
	return []mcp.ResourceContents{
		mcp.TextResourceContents{URI: request.Params.URI, MIMEType: "application/json", Text: string(data)},
	}, nil
}

func errorResource(uri string, err error) ([]mcp.ResourceContents, error) {
	data, _ := json.Marshal(map[string]string{"error": err.Error()})
	return []mcp.ResourceContents{
		mcp.TextResourceContents{URI: uri, MIMEType: "application/json", Text: string(data)},
	}, nil
}
