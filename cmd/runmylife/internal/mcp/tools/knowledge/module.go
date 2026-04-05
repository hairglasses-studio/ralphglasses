// Package knowledge provides MCP tools for the entity knowledge graph.
package knowledge

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	kg "github.com/hairglasses-studio/runmylife/internal/knowledge"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

type Module struct{}

func (m *Module) Name() string        { return "knowledge" }
func (m *Module) Description() string { return "Entity knowledge graph across all modules" }

var knowledgeHints = map[string]string{
	"graph/related": "Find entities related to a given entity",
	"graph/build":   "Build/refresh the knowledge graph from DB data",
	"graph/stats":   "Knowledge graph statistics",
	"graph/links":   "List links by relationship type",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("knowledge").
		Domain("graph", common.ActionRegistry{
			"related": handleGraphRelated,
			"build":   handleGraphBuild,
			"stats":   handleGraphStats,
			"links":   handleGraphLinks,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_knowledge",
				mcp.WithDescription("Knowledge graph gateway for entity relationships.\n\n"+dispatcher.DescribeActionsWithHints(knowledgeHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: graph")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action: related, build, stats, links")),
				mcp.WithString("entity_type", mcp.Description("Entity type: person, event, task, place, topic")),
				mcp.WithString("entity_id", mcp.Description("Entity ID")),
				mcp.WithString("relationship", mcp.Description("Relationship type filter")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
			),
			Handler:     tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:    "knowledge",
			Subcategory: "gateway",
			Tags:        []string{"knowledge", "graph", "entities"},
			Complexity:  tools.ComplexityModerate,
			Timeout:     60 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func handleGraphRelated(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entityType := common.GetStringParam(req, "entity_type", "")
	entityID := common.GetStringParam(req, "entity_id", "")
	if entityType == "" || entityID == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "entity_type and entity_id are required"), nil
	}
	limit := common.GetLimitParam(req, 20)

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	links, err := kg.FindRelated(ctx, database.SqlDB(), entityType, entityID, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Related to %s:%s", entityType, entityID))
	headers := []string{"Relationship", "Type", "ID", "Label", "Confidence"}
	var rows [][]string
	for _, l := range links {
		// Show the "other" side of the link.
		otherType, otherID, otherLabel := l.TargetType, l.TargetID, l.TargetLabel
		if l.TargetType == entityType && l.TargetID == entityID {
			otherType, otherID, otherLabel = l.SourceType, l.SourceID, l.SourceLabel
		}
		rows = append(rows, []string{l.Relationship, otherType, otherID, otherLabel, fmt.Sprintf("%.2f", l.Confidence)})
	}
	if len(rows) == 0 {
		md.EmptyList("related entities")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleGraphBuild(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	count, err := kg.BuildFromDB(ctx, database.SqlDB())
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("# Knowledge Graph Built\n\nCreated/updated **%d** entity links from existing data.", count)), nil
}

func handleGraphStats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	if err := kg.EnsureTable(database.SqlDB()); err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	stats, err := kg.GetStats(ctx, database.SqlDB())
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	md := common.NewMarkdownBuilder().Title("Knowledge Graph Stats")
	md.KeyValue("Total Links", fmt.Sprintf("%d", stats.TotalLinks))
	md.KeyValue("Persons", fmt.Sprintf("%d", stats.TotalPersons))
	md.KeyValue("Events", fmt.Sprintf("%d", stats.TotalEvents))
	md.KeyValue("Tasks", fmt.Sprintf("%d", stats.TotalTasks))
	md.KeyValue("Places", fmt.Sprintf("%d", stats.TotalPlaces))
	md.KeyValue("Topics", fmt.Sprintf("%d", stats.TotalTopics))
	return tools.TextResult(md.String()), nil
}

func handleGraphLinks(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	relationship := common.GetStringParam(req, "relationship", "")
	if relationship == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "relationship is required (e.g. communicates, attends, about)"), nil
	}
	limit := common.GetLimitParam(req, 50)

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	links, err := kg.FindByRelationship(ctx, database.SqlDB(), relationship, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Links: %s", relationship))
	headers := []string{"Source", "Source Label", "Target", "Target Label", "Confidence"}
	var rows [][]string
	for _, l := range links {
		rows = append(rows, []string{
			fmt.Sprintf("%s:%s", l.SourceType, l.SourceID), l.SourceLabel,
			fmt.Sprintf("%s:%s", l.TargetType, l.TargetID), l.TargetLabel,
			fmt.Sprintf("%.2f", l.Confidence),
		})
	}
	if len(rows) == 0 {
		md.EmptyList("links")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}
