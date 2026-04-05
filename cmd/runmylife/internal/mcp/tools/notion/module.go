// Package notion provides MCP tools for Notion database and page management.
package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/clients"
	"github.com/hairglasses-studio/runmylife/internal/config"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// Module implements the ToolModule interface for Notion integration.
type Module struct{}

func (m *Module) Name() string        { return "notion" }
func (m *Module) Description() string { return "Notion database and page management" }

var notionHints = map[string]string{
	"databases/list":  "List all Notion databases",
	"databases/query": "Query a database with optional filter",
	"pages/get":       "Get a single page by ID",
	"pages/create":    "Create a new page in a database",
	"pages/update":    "Update properties on a page",
	"pages/search":    "Search for pages by query",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("notion").
		Domain("databases", common.ActionRegistry{
			"list":  handleDatabasesList,
			"query": handleDatabasesQuery,
		}).
		Domain("pages", common.ActionRegistry{
			"get":    handlePagesGet,
			"create": handlePagesCreate,
			"update": handlePagesUpdate,
			"search": handlePagesSearch,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_notion",
				mcp.WithDescription(
					"Notion gateway. Manages databases and pages.\n\n"+
						dispatcher.DescribeActionsWithHints(notionHints),
				),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: databases, pages")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("database_id", mcp.Description("Notion database ID")),
				mcp.WithString("page_id", mcp.Description("Notion page ID")),
				mcp.WithString("filter", mcp.Description("JSON filter for database queries")),
				mcp.WithString("title", mcp.Description("Page title (for create)")),
				mcp.WithString("properties", mcp.Description("JSON properties for create/update")),
				mcp.WithString("query", mcp.Description("Search query")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "notion",
			Subcategory:         "gateway",
			Tags:                []string{"notion", "notes", "databases"},
			Complexity:          tools.ComplexityModerate,
			IsWrite:             true,
			CircuitBreakerGroup: "notion_api",
			Timeout:             60 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func getNotionClient() (*clients.NotionClient, *mcp.CallToolResult) {
	cfg, err := config.Load()
	if err != nil {
		return nil, common.CodedErrorResult(common.ErrConfig, err)
	}
	token := cfg.Credentials["notion"]
	if token == "" {
		return nil, common.ActionableErrorResult(common.ErrConfig, fmt.Errorf("notion integration token not configured"),
			"Add your Notion integration token to ~/.config/runmylife/config.json under credentials.notion",
			"Create an integration at https://www.notion.so/my-integrations",
		)
	}
	return clients.NewNotionClient(token), nil
}

// extractPageTitle attempts to extract a title from a Notion page's properties.
func extractPageTitle(properties map[string]interface{}) string {
	// Check common title property names
	for _, key := range []string{"Name", "name", "Title", "title"} {
		prop, ok := properties[key]
		if !ok {
			continue
		}
		propMap, ok := prop.(map[string]interface{})
		if !ok {
			continue
		}
		// Handle "title" type properties
		if titleArr, ok := propMap["title"].([]interface{}); ok && len(titleArr) > 0 {
			if item, ok := titleArr[0].(map[string]interface{}); ok {
				if text, ok := item["plain_text"].(string); ok {
					return text
				}
			}
		}
	}
	return "(untitled)"
}

// extractDatabaseTitle extracts the title from a NotionDatabase.
func extractDatabaseTitle(db clients.NotionDatabase) string {
	if len(db.Title) > 0 && db.Title[0].PlainText != "" {
		return db.Title[0].PlainText
	}
	return "(untitled)"
}

// extractDatabaseDescription extracts the description from a NotionDatabase.
func extractDatabaseDescription(db clients.NotionDatabase) string {
	if len(db.Description) > 0 {
		return db.Description[0].PlainText
	}
	return ""
}

func handleDatabasesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, errResult := getNotionClient()
	if errResult != nil {
		return errResult, nil
	}

	databases, err := client.ListDatabases(ctx)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	// Cache in database
	database, dbErr := common.OpenDB()
	if dbErr == nil {
		defer database.Close()
		for _, db := range databases {
			title := extractDatabaseTitle(db)
			desc := extractDatabaseDescription(db)
			database.SqlDB().ExecContext(ctx,
				`INSERT OR REPLACE INTO notion_databases (id, title, description, url, cached_at) VALUES (?, ?, ?, ?, datetime('now'))`,
				db.ID, title, desc, db.URL,
			)
		}
	}

	md := common.NewMarkdownBuilder().Title("Notion Databases")
	if len(databases) == 0 {
		md.EmptyList("databases")
	} else {
		headers := []string{"Title", "Description", "URL"}
		var rows [][]string
		for _, db := range databases {
			title := extractDatabaseTitle(db)
			desc := common.TruncateWords(extractDatabaseDescription(db), 50)
			rows = append(rows, []string{title, desc, db.URL})
		}
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleDatabasesQuery(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	databaseID, ok := common.RequireStringParam(req, "database_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "database_id is required for databases/query"), nil
	}

	filterJSON := common.GetStringParam(req, "filter", "")

	client, errResult := getNotionClient()
	if errResult != nil {
		return errResult, nil
	}

	pages, err := client.QueryDatabase(ctx, databaseID, filterJSON)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	// Cache pages in database
	db, dbErr := common.OpenDB()
	if dbErr == nil {
		defer db.Close()
		for _, p := range pages {
			title := extractPageTitle(p.Properties)
			propsJSON, _ := json.Marshal(p.Properties)
			db.SqlDB().ExecContext(ctx,
				`INSERT OR REPLACE INTO notion_pages (id, database_id, title, properties_json, url, created_at, last_edited_at, cached_at) VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
				p.ID, databaseID, title, string(propsJSON), p.URL, p.CreatedTime, p.LastEditedTime,
			)
		}
	}

	md := common.NewMarkdownBuilder().Title("Query Results")
	if len(pages) == 0 {
		md.EmptyList("pages")
	} else {
		headers := []string{"Title", "Created", "Last Edited", "URL"}
		var rows [][]string
		for _, p := range pages {
			title := extractPageTitle(p.Properties)
			rows = append(rows, []string{title, p.CreatedTime[:10], p.LastEditedTime[:10], p.URL})
		}
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handlePagesGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pageID, ok := common.RequireStringParam(req, "page_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "page_id is required for pages/get"), nil
	}

	client, errResult := getNotionClient()
	if errResult != nil {
		return errResult, nil
	}

	page, err := client.GetPage(ctx, pageID)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	title := extractPageTitle(page.Properties)
	md := common.NewMarkdownBuilder().Title(title)
	md.KeyValue("ID", page.ID)
	md.KeyValue("URL", page.URL)
	md.KeyValue("Created", page.CreatedTime)
	md.KeyValue("Last Edited", page.LastEditedTime)

	// Show properties
	md.Section("Properties")
	for key, val := range page.Properties {
		valJSON, _ := json.Marshal(val)
		md.KeyValue(key, common.TruncateWords(string(valJSON), 100))
	}

	return tools.TextResult(md.String()), nil
}

func handlePagesCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	databaseID, ok := common.RequireStringParam(req, "database_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "database_id is required for pages/create"), nil
	}

	propsStr := common.GetStringParam(req, "properties", "")
	if propsStr == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "properties (JSON) is required for pages/create"), nil
	}

	var properties map[string]interface{}
	if err := json.Unmarshal([]byte(propsStr), &properties); err != nil {
		return common.CodedErrorResultf(common.ErrInvalidParam, "invalid properties JSON: %v", err), nil
	}

	client, errResult := getNotionClient()
	if errResult != nil {
		return errResult, nil
	}

	page, err := client.CreatePage(ctx, databaseID, properties)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	title := extractPageTitle(page.Properties)
	md := common.NewMarkdownBuilder().Title("Page Created")
	md.KeyValue("Title", title)
	md.KeyValue("ID", page.ID)
	md.KeyValue("URL", page.URL)

	return tools.TextResult(md.String()), nil
}

func handlePagesUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pageID, ok := common.RequireStringParam(req, "page_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "page_id is required for pages/update"), nil
	}

	propsStr := common.GetStringParam(req, "properties", "")
	if propsStr == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "properties (JSON) is required for pages/update"), nil
	}

	var properties map[string]interface{}
	if err := json.Unmarshal([]byte(propsStr), &properties); err != nil {
		return common.CodedErrorResultf(common.ErrInvalidParam, "invalid properties JSON: %v", err), nil
	}

	client, errResult := getNotionClient()
	if errResult != nil {
		return errResult, nil
	}

	page, err := client.UpdatePage(ctx, pageID, properties)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	title := extractPageTitle(page.Properties)
	md := common.NewMarkdownBuilder().Title("Page Updated")
	md.KeyValue("Title", title)
	md.KeyValue("ID", page.ID)
	md.KeyValue("URL", page.URL)

	return tools.TextResult(md.String()), nil
}

func handlePagesSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := common.GetStringParam(req, "query", "")
	if query == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "query is required for pages/search"), nil
	}

	client, errResult := getNotionClient()
	if errResult != nil {
		return errResult, nil
	}

	pages, err := client.Search(ctx, query)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Search: %q", query))
	if len(pages) == 0 {
		md.EmptyList("pages")
	} else {
		headers := []string{"Title", "Last Edited", "URL"}
		var rows [][]string
		for _, p := range pages {
			title := extractPageTitle(p.Properties)
			edited := p.LastEditedTime
			if len(edited) >= 10 {
				edited = edited[:10]
			}
			rows = append(rows, []string{title, edited, p.URL})
		}
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}
