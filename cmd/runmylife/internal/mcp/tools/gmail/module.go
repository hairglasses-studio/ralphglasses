// Package gmail provides MCP tools for Gmail integration.
package gmail

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/clients"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// Module implements the ToolModule interface for Gmail tools.
type Module struct{}

func (m *Module) Name() string        { return "gmail" }
func (m *Module) Description() string { return "Gmail integration for email management and triage" }

var gmailHints = map[string]string{
	"messages/search":  "Search messages by query (live API or DB fallback)",
	"messages/read":    "Read a specific message by ID (live API)",
	"messages/thread":  "Read a full email thread (live API)",
	"drafts/create":    "Create an email draft (live via Gmail API)",
	"drafts/list":      "List existing drafts",
	"drafts/send":      "Send a draft by ID",
	"drafts/delete":    "Delete a draft by ID",
	"triage/unread":    "List unread messages for triage",
	"triage/archive":   "Archive messages by ID",
	"triage/categorize": "Auto-categorize messages by type",
	"compose/reply":    "Reply to a thread (creates draft, use confirm=true to send)",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("gmail").
		Domain("messages", common.ActionRegistry{
			"search": handleMessagesSearch,
			"read":   handleMessagesRead,
			"thread": handleMessagesThread,
		}).
		Domain("drafts", common.ActionRegistry{
			"create": handleDraftsCreate,
			"list":   handleDraftsList,
			"send":   handleDraftsSend,
			"delete": handleDraftsDelete,
		}).
		Domain("triage", common.ActionRegistry{
			"unread":     handleTriageUnread,
			"archive":    handleTriageArchive,
			"categorize": handleTriageCategorize,
		}).
		Domain("compose", common.ActionRegistry{
			"reply": handleComposeReply,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_gmail",
				mcp.WithDescription(
					"Gmail gateway for email management.\n\n"+
						dispatcher.DescribeActionsWithHints(gmailHints),
				),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: messages, drafts, triage, compose")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("query", mcp.Description("Gmail search query")),
				mcp.WithString("message_id", mcp.Description("Message ID")),
				mcp.WithString("message_ids", mcp.Description("Comma-separated message IDs (for archive)")),
				mcp.WithString("thread_id", mcp.Description("Thread ID")),
				mcp.WithString("draft_id", mcp.Description("Draft ID (for send/delete)")),
				mcp.WithString("to", mcp.Description("Recipient email")),
				mcp.WithString("subject", mcp.Description("Email subject")),
				mcp.WithString("body", mcp.Description("Email body")),
				mcp.WithString("account", mcp.Description("Google account (default: personal)")),
				mcp.WithBoolean("confirm", mcp.Description("Set true to actually send (reply/send actions)")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "gmail",
			Subcategory:         "gateway",
			Tags:                []string{"gmail", "email", "communication"},
			Complexity:          tools.ComplexityModerate,
			IsWrite:             true,
			ProducesRefs:        []string{"gmail_message"},
			CircuitBreakerGroup: "gmail_api",
			Timeout:             60 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

// gmailClient returns a live Gmail API client, or nil if credentials are unavailable.
func gmailClient(ctx context.Context, req mcp.CallToolRequest) *clients.GmailAPIClient {
	account := common.GetStringParam(req, "account", "personal")
	client, err := clients.NewGmailAPIClientForAccount(ctx, account)
	if err != nil {
		return nil
	}
	return client
}

func handleMessagesSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := common.GetStringParam(req, "query", "")
	if query == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "query is required for search"), nil
	}
	limit := common.GetLimitParam(req, 20)

	// Try live API first.
	if client := gmailClient(ctx, req); client != nil {
		msgs, err := client.FetchMessageHeaders(ctx, query, int64(limit))
		if err != nil {
			return common.ActionableErrorResult(common.ErrAPIError, err, "Check Google OAuth credentials"), nil
		}

		md := common.NewMarkdownBuilder().Title("Gmail Search: " + query)
		headers := []string{"ID", "From", "Subject", "Date"}
		var tableRows [][]string
		for _, m := range msgs {
			tableRows = append(tableRows, []string{m.ID[:12], m.From, m.Subject, m.Date.Format("2006-01-02 15:04")})
		}
		if len(tableRows) == 0 {
			md.EmptyList("messages")
		} else {
			md.Table(headers, tableRows)
		}
		return tools.TextResult(md.String()), nil
	}

	// Fallback to DB.
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT id, from_addr, subject, snippet, timestamp FROM gmail_messages WHERE subject LIKE ? OR from_addr LIKE ? ORDER BY timestamp DESC LIMIT ?",
		"%"+query+"%", "%"+query+"%", limit,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Gmail Search (cached): " + query)
	headers := []string{"From", "Subject", "Date"}
	var tableRows [][]string
	for rows.Next() {
		var id, from, subject, snippet, timestamp string
		if err := rows.Scan(&id, &from, &subject, &snippet, &timestamp); err != nil {
			continue
		}
		tableRows = append(tableRows, []string{from, subject, timestamp})
	}
	if len(tableRows) == 0 {
		md.EmptyList("messages")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleMessagesRead(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	messageID, ok := common.RequireStringParam(req, "message_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "message_id is required"), nil
	}

	// Try live API first.
	if client := gmailClient(ctx, req); client != nil {
		msgs, err := client.FetchMessagesByIDs(ctx, []string{messageID})
		if err != nil {
			return common.ActionableErrorResult(common.ErrAPIError, err, "Check Google OAuth credentials"), nil
		}
		if len(msgs) == 0 {
			return common.CodedErrorResultf(common.ErrNotFound, "message %s not found", messageID), nil
		}
		m := msgs[0]
		md := common.NewMarkdownBuilder().Title(m.Subject)
		md.KeyValue("From", m.From)
		md.KeyValue("To", m.To)
		md.KeyValue("Date", m.Date.Format("2006-01-02 15:04"))
		md.KeyValue("Labels", m.Labels)
		md.Section("Body").Text(m.Body)
		return tools.TextResult(md.String()), nil
	}

	// Fallback to DB.
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	var from, subject, body, timestamp string
	err = database.SqlDB().QueryRowContext(ctx,
		"SELECT from_addr, subject, body, timestamp FROM gmail_messages WHERE id = ?", messageID,
	).Scan(&from, &subject, &body, &timestamp)
	if err != nil {
		return common.CodedErrorResultf(common.ErrNotFound, "message %s not found", messageID), nil
	}

	md := common.NewMarkdownBuilder().Title(subject)
	md.KeyValue("From", from)
	md.KeyValue("Date", timestamp)
	md.Section("Body").Text(body)
	return tools.TextResult(md.String()), nil
}

func handleMessagesThread(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	threadID, ok := common.RequireStringParam(req, "thread_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "thread_id is required"), nil
	}

	// Try live API first.
	if client := gmailClient(ctx, req); client != nil {
		msgs, err := client.FetchThread(ctx, threadID)
		if err != nil {
			return common.ActionableErrorResult(common.ErrAPIError, err, "Check Google OAuth credentials"), nil
		}
		md := common.NewMarkdownBuilder().Title("Thread: " + threadID)
		for _, m := range msgs {
			md.Section(fmt.Sprintf("%s — %s", m.From, m.Date.Format("2006-01-02 15:04")))
			md.Bold("Subject", m.Subject)
			md.Text(m.Body)
		}
		if len(msgs) == 0 {
			md.EmptyList("messages in thread")
		}
		return tools.TextResult(md.String()), nil
	}

	// Fallback to DB.
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT from_addr, subject, snippet, timestamp FROM gmail_messages WHERE thread_id = ? ORDER BY timestamp ASC",
		threadID,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Thread: " + threadID)
	var count int
	for rows.Next() {
		var from, subject, snippet, timestamp string
		if err := rows.Scan(&from, &subject, &snippet, &timestamp); err != nil {
			continue
		}
		md.Section(fmt.Sprintf("%s — %s", from, timestamp))
		md.Bold("Subject", subject)
		md.Text(snippet)
		count++
	}
	if count == 0 {
		md.EmptyList("messages in thread")
	}
	return tools.TextResult(md.String()), nil
}

func handleDraftsCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	to := common.GetStringParam(req, "to", "")
	subject := common.GetStringParam(req, "subject", "")
	body := common.GetStringParam(req, "body", "")

	if to == "" || subject == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "to and subject are required"), nil
	}

	// Try live API.
	if client := gmailClient(ctx, req); client != nil {
		draft, err := client.CreateDraft(ctx, to, subject, body)
		if err != nil {
			return common.ActionableErrorResult(common.ErrAPIError, err, "Check Google OAuth credentials"), nil
		}
		return tools.TextResult(fmt.Sprintf("# Draft Created (Gmail API)\n\n- **Draft ID:** %s\n- **To:** %s\n- **Subject:** %s\n\nUse `drafts/send` with this draft_id to send.", draft.ID, to, subject)), nil
	}

	// Fallback to local DB.
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	id := fmt.Sprintf("draft-%d", time.Now().UnixNano())
	_, err = database.SqlDB().ExecContext(ctx,
		"INSERT INTO gmail_messages (id, from_addr, subject, body, timestamp, labels) VALUES (?, 'me', ?, ?, ?, 'DRAFT')",
		id, subject, body, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("# Draft Created (local)\n\n- **ID:** %s\n- **To:** %s\n- **Subject:** %s\n\n⚠️ Stored locally only — configure Google OAuth to create real Gmail drafts.", id, to, subject)), nil
}

func handleDraftsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := common.GetLimitParam(req, 20)

	client := gmailClient(ctx, req)
	if client == nil {
		return common.ActionableErrorResult(common.ErrConfig, fmt.Errorf("Google OAuth not configured"), "Run 'runmylife google-auth' to authenticate"), nil
	}

	drafts, err := client.ListDrafts(ctx, int64(limit))
	if err != nil {
		return common.ActionableErrorResult(common.ErrAPIError, err, "Check Google OAuth credentials"), nil
	}

	md := common.NewMarkdownBuilder().Title("Gmail Drafts")
	headers := []string{"Draft ID", "To", "Subject"}
	var tableRows [][]string
	for _, d := range drafts {
		tableRows = append(tableRows, []string{d.ID, d.To, d.Subject})
	}
	if len(tableRows) == 0 {
		md.EmptyList("drafts")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleDraftsSend(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	draftID, ok := common.RequireStringParam(req, "draft_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "draft_id is required"), nil
	}
	confirm := common.GetBoolParam(req, "confirm", false)
	if !confirm {
		return tools.TextResult(fmt.Sprintf("# Confirm Send\n\nSet `confirm=true` to send draft **%s**.\n\nThis action cannot be undone.", draftID)), nil
	}

	client := gmailClient(ctx, req)
	if client == nil {
		return common.ActionableErrorResult(common.ErrConfig, fmt.Errorf("Google OAuth not configured"), "Run 'runmylife google-auth' to authenticate"), nil
	}

	sent, err := client.SendDraft(ctx, draftID)
	if err != nil {
		return common.ActionableErrorResult(common.ErrAPIError, err, "Verify draft still exists"), nil
	}
	return tools.TextResult(fmt.Sprintf("# Draft Sent\n\n- **Message ID:** %s\n- **Thread ID:** %s", sent.MessageID, sent.ThreadID)), nil
}

func handleDraftsDelete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	draftID, ok := common.RequireStringParam(req, "draft_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "draft_id is required"), nil
	}

	client := gmailClient(ctx, req)
	if client == nil {
		return common.ActionableErrorResult(common.ErrConfig, fmt.Errorf("Google OAuth not configured"), "Run 'runmylife google-auth' to authenticate"), nil
	}

	if err := client.DeleteDraft(ctx, draftID); err != nil {
		return common.ActionableErrorResult(common.ErrAPIError, err, "Verify draft exists"), nil
	}
	return tools.TextResult(fmt.Sprintf("Draft %s deleted.", draftID)), nil
}

func handleTriageUnread(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := common.GetLimitParam(req, 20)

	// Try live API — search for unread inbox messages.
	if client := gmailClient(ctx, req); client != nil {
		msgs, err := client.FetchMessageHeaders(ctx, "is:unread in:inbox", int64(limit))
		if err != nil {
			return common.ActionableErrorResult(common.ErrAPIError, err, "Check Google OAuth credentials"), nil
		}
		md := common.NewMarkdownBuilder().Title("Unread Messages")
		headers := []string{"ID", "From", "Subject", "Date"}
		var tableRows [][]string
		for _, m := range msgs {
			tableRows = append(tableRows, []string{m.ID[:12], m.From, m.Subject, m.Date.Format("2006-01-02 15:04")})
		}
		if len(tableRows) == 0 {
			md.EmptyList("unread messages")
		} else {
			md.Table(headers, tableRows)
		}
		return tools.TextResult(md.String()), nil
	}

	// Fallback to DB.
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT id, from_addr, subject, snippet, timestamp FROM gmail_messages WHERE triaged = 0 ORDER BY timestamp DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Unread Messages (cached)")
	headers := []string{"ID", "From", "Subject", "Date"}
	var tableRows [][]string
	for rows.Next() {
		var id, from, subject, snippet, timestamp string
		if err := rows.Scan(&id, &from, &subject, &snippet, &timestamp); err != nil {
			continue
		}
		short := id
		if len(id) > 8 {
			short = id[:8]
		}
		tableRows = append(tableRows, []string{short, from, subject, timestamp})
	}
	if len(tableRows) == 0 {
		md.EmptyList("unread messages")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleTriageArchive(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idsStr := common.GetStringParam(req, "message_ids", "")
	singleID := common.GetStringParam(req, "message_id", "")
	if idsStr == "" && singleID == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "message_id or message_ids is required"), nil
	}

	var ids []string
	if idsStr != "" {
		for _, id := range strings.Split(idsStr, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				ids = append(ids, id)
			}
		}
	} else {
		ids = []string{singleID}
	}

	client := gmailClient(ctx, req)
	if client == nil {
		return common.ActionableErrorResult(common.ErrConfig, fmt.Errorf("Google OAuth not configured"), "Run 'runmylife google-auth' to authenticate"), nil
	}

	if err := client.ArchiveMessages(ctx, ids); err != nil {
		return common.ActionableErrorResult(common.ErrAPIError, err, "Check message IDs are valid"), nil
	}
	return tools.TextResult(fmt.Sprintf("Archived %d message(s).", len(ids))), nil
}

func handleTriageCategorize(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	var total, triaged, untriaged int
	database.SqlDB().QueryRowContext(ctx, "SELECT COUNT(*) FROM gmail_messages").Scan(&total)
	database.SqlDB().QueryRowContext(ctx, "SELECT COUNT(*) FROM gmail_messages WHERE triaged = 1").Scan(&triaged)
	untriaged = total - triaged

	md := common.NewMarkdownBuilder().Title("Email Triage Status")
	md.KeyValue("Total messages", fmt.Sprintf("%d", total))
	md.KeyValue("Triaged", fmt.Sprintf("%d", triaged))
	md.KeyValue("Needs triage", fmt.Sprintf("%d", untriaged))

	return tools.TextResult(md.String()), nil
}

func handleComposeReply(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	threadID, ok := common.RequireStringParam(req, "thread_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "thread_id is required"), nil
	}
	to := common.GetStringParam(req, "to", "")
	body := common.GetStringParam(req, "body", "")
	if to == "" || body == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "to and body are required for reply"), nil
	}
	confirm := common.GetBoolParam(req, "confirm", false)

	client := gmailClient(ctx, req)
	if client == nil {
		return common.ActionableErrorResult(common.ErrConfig, fmt.Errorf("Google OAuth not configured"), "Run 'runmylife google-auth' to authenticate"), nil
	}

	if !confirm {
		return tools.TextResult(fmt.Sprintf("# Confirm Reply\n\n- **Thread:** %s\n- **To:** %s\n- **Body preview:** %.100s...\n\nSet `confirm=true` to send this reply.", threadID, to, body)), nil
	}

	draft, err := client.ReplyToThread(ctx, threadID, to, body)
	if err != nil {
		return common.ActionableErrorResult(common.ErrAPIError, err, "Check thread ID and recipient"), nil
	}
	return tools.TextResult(fmt.Sprintf("# Reply Sent\n\n- **Message ID:** %s\n- **Thread ID:** %s", draft.MessageID, draft.ThreadID)), nil
}
