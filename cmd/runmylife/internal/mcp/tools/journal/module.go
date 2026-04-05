// Package journal provides MCP tools for daily journal management.
package journal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// Module implements the ToolModule interface for journal management.
type Module struct{}

func (m *Module) Name() string        { return "journal" }
func (m *Module) Description() string { return "Daily journal entries and full-text search" }

var journalHints = map[string]string{
	"entries/today":  "Get or create today's journal entry",
	"entries/list":   "List recent journal entries",
	"entries/get":    "Read a journal entry by date",
	"entries/create": "Create a journal entry for a specific date",
	"entries/append": "Append content to an existing entry",
	"search/find":    "Full-text search across all journal entries",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("journal").
		Domain("entries", common.ActionRegistry{
			"today":  handleEntriesToday,
			"list":   handleEntriesList,
			"get":    handleEntriesGet,
			"create": handleEntriesCreate,
			"append": handleEntriesAppend,
		}).
		Domain("search", common.ActionRegistry{
			"find": handleSearchFind,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_journal",
				mcp.WithDescription(
					"Journal management gateway. Read, write, and search daily journal entries.\n\n"+
						dispatcher.DescribeActionsWithHints(journalHints),
				),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: entries, search")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("date", mcp.Description("Date in YYYY-MM-DD format")),
				mcp.WithString("content", mcp.Description("Journal content (for create/append)")),
				mcp.WithString("query", mcp.Description("Search query (for search/find)")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 10)")),
			),
			Handler:    tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:   "journal",
			Tags:       []string{"journal", "writing", "reflection"},
			Complexity: tools.ComplexitySimple,
			IsWrite:    true,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func journalDir() string {
	return filepath.Join("data", "journal")
}

func handleEntriesToday(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	date := time.Now().Format("2006-01-02")
	dir := journalDir()
	path := filepath.Join(dir, date+".md")

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return common.CodedErrorResult(common.ErrAPIError, err), nil
		}
		// Create today's entry with template
		if err := os.MkdirAll(dir, 0755); err != nil {
			return common.CodedErrorResult(common.ErrAPIError, err), nil
		}
		template := fmt.Sprintf("# Journal — %s\n\n", date)
		if err := os.WriteFile(path, []byte(template), 0644); err != nil {
			return common.CodedErrorResult(common.ErrAPIError, err), nil
		}
		return tools.TextResult(fmt.Sprintf("# Today's Journal (%s)\n\nCreated new entry.\n\n%s", date, template)), nil
	}

	return tools.TextResult(fmt.Sprintf("# Today's Journal (%s)\n\n%s", date, string(data))), nil
}

func handleEntriesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := common.GetLimitParam(req, 10)
	dir := journalDir()

	matches, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	// Sort descending by filename (dates sort lexicographically)
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))

	if len(matches) > limit {
		matches = matches[:limit]
	}

	md := common.NewMarkdownBuilder().Title("Journal Entries")

	if len(matches) == 0 {
		md.EmptyList("journal entries")
		return tools.TextResult(md.String()), nil
	}

	headers := []string{"Date", "Preview"}
	var rows [][]string
	for _, path := range matches {
		date := strings.TrimSuffix(filepath.Base(path), ".md")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		preview := firstLine(string(data))
		rows = append(rows, []string{date, common.TruncateWords(preview, 60)})
	}

	md.Table(headers, rows)
	return tools.TextResult(md.String()), nil
}

func handleEntriesGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	date := common.GetStringParam(req, "date", "")
	if date == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "date is required for entries/get"), nil
	}

	path := filepath.Join(journalDir(), date+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return common.CodedErrorResultf(common.ErrNotFound, "no journal entry for %s", date), nil
		}
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("# Journal — %s\n\n%s", date, string(data))), nil
}

func handleEntriesCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	date := common.GetStringParam(req, "date", time.Now().Format("2006-01-02"))
	content := common.GetStringParam(req, "content", "")

	dir := journalDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	path := filepath.Join(dir, date+".md")
	body := fmt.Sprintf("# Journal — %s\n\n%s", date, content)
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("# Journal Entry Created\n\n- **Date:** %s\n- **Path:** %s", date, path)), nil
}

func handleEntriesAppend(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	date := common.GetStringParam(req, "date", time.Now().Format("2006-01-02"))
	content := common.GetStringParam(req, "content", "")
	if content == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "content is required for entries/append"), nil
	}

	path := filepath.Join(journalDir(), date+".md")
	existing, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return common.CodedErrorResultf(common.ErrNotFound, "no journal entry for %s — use entries/create first", date), nil
		}
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	updated := string(existing) + "\n\n" + content
	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("# Content Appended\n\n- **Date:** %s\n- **Added:** %d characters", date, len(content))), nil
}

func handleSearchFind(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := common.GetStringParam(req, "query", "")
	if query == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "query is required for search/find"), nil
	}
	limit := common.GetLimitParam(req, 10)

	matches, err := filepath.Glob(filepath.Join(journalDir(), "*.md"))
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	queryLower := strings.ToLower(query)
	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Search: \"%s\"", query))

	var found int
	// Sort descending so newest matches come first
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))

	for _, path := range matches {
		if found >= limit {
			break
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		contentLower := strings.ToLower(string(data))
		if !strings.Contains(contentLower, queryLower) {
			continue
		}

		date := strings.TrimSuffix(filepath.Base(path), ".md")
		snippet := extractSnippet(string(data), query, 120)
		md.Section(date).Text(snippet)
		found++
	}

	if found == 0 {
		md.EmptyList("matching entries")
	} else {
		md.Text(fmt.Sprintf("*Found %d matching entries.*", found))
	}

	return tools.TextResult(md.String()), nil
}

// firstLine returns the first non-empty line of text.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// extractSnippet finds the query in content and returns surrounding context.
func extractSnippet(content, query string, maxLen int) string {
	lower := strings.ToLower(content)
	queryLower := strings.ToLower(query)
	idx := strings.Index(lower, queryLower)
	if idx < 0 {
		return common.TruncateWords(content, maxLen)
	}
	start := idx - 40
	if start < 0 {
		start = 0
	}
	end := idx + len(query) + 80
	if end > len(content) {
		end = len(content)
	}
	snippet := content[start:end]
	snippet = strings.ReplaceAll(snippet, "\n", " ")
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(content) {
		snippet = snippet + "..."
	}
	return snippet
}
