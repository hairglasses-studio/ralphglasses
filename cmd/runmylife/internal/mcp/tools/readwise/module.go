// Package readwise provides MCP tools for Readwise reading highlights.
package readwise

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/clients"
	"github.com/hairglasses-studio/runmylife/internal/config"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

type Module struct{}

func (m *Module) Name() string        { return "readwise" }
func (m *Module) Description() string { return "Readwise reading highlights and books" }

var readwiseHints = map[string]string{
	"books/list":        "List all books/articles",
	"books/highlights":  "Get highlights for a book by ID",
	"highlights/list":   "List recent highlights",
	"highlights/search": "Search highlights by text",
	"highlights/daily":  "Today's daily review highlights",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("readwise").
		Domain("books", common.ActionRegistry{
			"list":       handleBooksList,
			"highlights": handleBooksHighlights,
		}).
		Domain("highlights", common.ActionRegistry{
			"list":   handleHighlightsList,
			"search": handleHighlightsSearch,
			"daily":  handleHighlightsDaily,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_readwise",
				mcp.WithDescription("Readwise gateway for reading highlights.\n\n"+dispatcher.DescribeActionsWithHints(readwiseHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: books, highlights")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("book_id", mcp.Description("Book ID (for book highlights)")),
				mcp.WithString("query", mcp.Description("Search query")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "readwise",
			Subcategory:         "gateway",
			Tags:                []string{"readwise", "reading", "highlights"},
			Complexity:          tools.ComplexitySimple,
			CircuitBreakerGroup: "readwise_api",
			Timeout:             30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func readwiseClient() (*clients.ReadwiseClient, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	token := cfg.Credentials["readwise"]
	if token == "" {
		return nil, fmt.Errorf("readwise token not configured — add 'readwise' to credentials in config.json")
	}
	return clients.NewReadwiseClient(token), nil
}

func handleBooksList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := readwiseClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add readwise token to config"), nil
	}
	limit := common.GetLimitParam(req, 20)
	books, err := client.ListBooks(ctx, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Books & Articles")
	headers := []string{"ID", "Title", "Author", "Source", "Highlights"}
	var rows [][]string
	for _, b := range books {
		rows = append(rows, []string{fmt.Sprintf("%d", b.ID), b.Title, b.Author, b.Source, fmt.Sprintf("%d", b.NumHighlights)})
	}
	if len(rows) == 0 {
		md.EmptyList("books")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleBooksHighlights(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	bookIDStr := common.GetStringParam(req, "book_id", "")
	if bookIDStr == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "book_id is required"), nil
	}
	bookID := common.GetIntParam(req, "book_id", 0)
	if bookID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "book_id must be a number"), nil
	}
	client, err := readwiseClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add readwise token to config"), nil
	}
	limit := common.GetLimitParam(req, 50)
	highlights, err := client.GetBookHighlights(ctx, bookID, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Highlights for Book #%d", bookID))
	for i, h := range highlights {
		md.Section(fmt.Sprintf("Highlight %d", i+1))
		md.Text("> " + h.Text)
		if h.Note != "" {
			md.KeyValue("Note", h.Note)
		}
	}
	if len(highlights) == 0 {
		md.EmptyList("highlights")
	}
	return tools.TextResult(md.String()), nil
}

func handleHighlightsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := readwiseClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add readwise token to config"), nil
	}
	limit := common.GetLimitParam(req, 20)
	highlights, err := client.ListHighlights(ctx, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Recent Highlights")
	for i, h := range highlights {
		text := h.Text
		if len(text) > 200 {
			text = text[:200] + "..."
		}
		md.Section(fmt.Sprintf("%d", i+1))
		md.Text("> " + text)
		if h.Note != "" {
			md.KeyValue("Note", h.Note)
		}
	}
	if len(highlights) == 0 {
		md.EmptyList("highlights")
	}
	return tools.TextResult(md.String()), nil
}

func handleHighlightsSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := common.GetStringParam(req, "query", "")
	if query == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "query is required"), nil
	}
	client, err := readwiseClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add readwise token to config"), nil
	}
	limit := common.GetLimitParam(req, 20)
	highlights, err := client.SearchHighlights(ctx, query, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Search: " + query)
	for i, h := range highlights {
		text := h.Text
		if len(text) > 200 {
			text = text[:200] + "..."
		}
		md.Section(fmt.Sprintf("%d", i+1))
		md.Text("> " + text)
		if h.Note != "" {
			md.KeyValue("Note", h.Note)
		}
	}
	if len(highlights) == 0 {
		md.EmptyList("results")
	}
	return tools.TextResult(md.String()), nil
}

func handleHighlightsDaily(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := readwiseClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add readwise token to config"), nil
	}
	highlights, err := client.DailyReview(ctx)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Daily Review")
	for i, h := range highlights {
		md.Section(fmt.Sprintf("%d", i+1))
		md.Text("> " + h.Text)
		if h.Note != "" {
			md.KeyValue("Note", h.Note)
		}
	}
	if len(highlights) == 0 {
		md.Text("No daily review highlights available.")
	}
	return tools.TextResult(md.String()), nil
}
