// Package messages provides MCP tools for SMS/RCS messaging via Google Messages.
package messages

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/clients"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// Module implements the ToolModule interface for messaging.
type Module struct{}

func (m *Module) Name() string        { return "messages" }
func (m *Module) Description() string { return "SMS/RCS messaging via Google Messages" }

var messageHints = map[string]string{
	"conversations/list": "List recent conversations with last message preview",
	"conversations/get":  "View messages in a specific conversation",
	"messages/search":    "Search messages by text content",
	"messages/send":      "Send a text message (requires pairing)",
	"status/check":       "Check Google Messages session status",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("messages").
		Domain("conversations", common.ActionRegistry{
			"list": handleConversationsList,
			"get":  handleConversationsGet,
		}).
		Domain("messages", common.ActionRegistry{
			"search": handleMessagesSearch,
			"send":   handleMessagesSend,
		}).
		Domain("status", common.ActionRegistry{
			"check": handleStatusCheck,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_messages",
				mcp.WithDescription(
					"Messaging gateway. SMS/RCS via Google Messages.\n\n"+
						dispatcher.DescribeActionsWithHints(messageHints),
				),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: conversations, messages, status")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("conversation_id", mcp.Description("Conversation ID (for get)")),
				mcp.WithString("query", mcp.Description("Search query (for search)")),
				mcp.WithString("to", mcp.Description("Phone number (for send)")),
				mcp.WithString("body", mcp.Description("Message text (for send)")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "messages",
			Subcategory:         "gateway",
			Tags:                []string{"sms", "rcs", "messaging"},
			Complexity:          tools.ComplexityModerate,
			IsWrite:             true,
			CircuitBreakerGroup: "messages_api",
			Timeout:             60 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func handleConversationsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := common.GetLimitParam(req, 20)

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}

	client := clients.NewGMessagesClient(database.SqlDB())
	convos, err := client.ListConversations(ctx, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	md := common.NewMarkdownBuilder().Title("Conversations")

	if len(convos) == 0 {
		md.EmptyList("conversations")
		return tools.TextResult(md.String()), nil
	}

	headers := []string{"ID", "Participant", "Last Message", "Count", "Last At"}
	var rows [][]string
	for _, c := range convos {
		cid := fmt.Sprintf("%v", c["conversation_id"])
		short := cid
		if len(short) > 8 {
			short = short[:8]
		}
		lastBody := fmt.Sprintf("%v", c["last_body"])
		if len(lastBody) > 40 {
			lastBody = lastBody[:40] + "..."
		}
		rows = append(rows, []string{
			short,
			fmt.Sprintf("%v", c["participant"]),
			lastBody,
			fmt.Sprintf("%v", c["message_count"]),
			fmt.Sprintf("%v", c["last_message_at"]),
		})
	}
	md.Table(headers, rows)

	return tools.TextResult(md.String()), nil
}

func handleConversationsGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	conversationID, ok := common.RequireStringParam(req, "conversation_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "conversation_id is required"), nil
	}
	limit := common.GetLimitParam(req, 20)

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}

	client := clients.NewGMessagesClient(database.SqlDB())
	messages, err := client.ListMessages(ctx, conversationID, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Conversation %s", conversationID))

	if len(messages) == 0 {
		md.EmptyList("messages")
		return tools.TextResult(md.String()), nil
	}

	headers := []string{"Time", "Direction", "Sender", "Message"}
	var rows [][]string
	for _, msg := range messages {
		body := msg.Body
		if len(body) > 60 {
			body = body[:60] + "..."
		}
		rows = append(rows, []string{
			msg.SentAt.Format("2006-01-02 15:04"),
			msg.Direction,
			msg.Sender,
			body,
		})
	}
	md.Table(headers, rows)

	return tools.TextResult(md.String()), nil
}

func handleMessagesSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, ok := common.RequireStringParam(req, "query")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "query is required for search"), nil
	}
	limit := common.GetLimitParam(req, 20)

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}

	client := clients.NewGMessagesClient(database.SqlDB())
	messages, err := client.SearchMessages(ctx, query, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Search: %s", query))

	if len(messages) == 0 {
		md.EmptyList("messages matching query")
		return tools.TextResult(md.String()), nil
	}

	headers := []string{"Time", "Sender", "Direction", "Conversation", "Message"}
	var rows [][]string
	for _, msg := range messages {
		body := msg.Body
		if len(body) > 50 {
			body = body[:50] + "..."
		}
		cid := msg.ConversationID
		if len(cid) > 8 {
			cid = cid[:8]
		}
		rows = append(rows, []string{
			msg.SentAt.Format("2006-01-02 15:04"),
			msg.Sender,
			msg.Direction,
			cid,
			body,
		})
	}
	md.Table(headers, rows)

	return tools.TextResult(md.String()), nil
}

func handleMessagesSend(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return common.ActionableErrorResult(common.ErrConfig,
		fmt.Errorf("Google Messages pairing not yet configured"),
		"Run pairing flow to enable SMS sending",
		"Use runmylife_messages(domain=\"status\", action=\"check\") to view session status",
	), nil
}

func handleStatusCheck(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	md := common.NewMarkdownBuilder().Title("Google Messages Status")

	hasSession := clients.HasSession()
	sessionPath := clients.SessionPath()

	md.KeyValue("Session File", sessionPath)
	if hasSession {
		md.KeyValue("Session", "Found")
	} else {
		md.KeyValue("Session", "Not found")
	}
	md.KeyValue("Connected", "No (live connection not yet implemented)")
	md.KeyValue("Pairing", "Use StartGaiaPairing when libgm dependency is added")

	return tools.TextResult(md.String()), nil
}
