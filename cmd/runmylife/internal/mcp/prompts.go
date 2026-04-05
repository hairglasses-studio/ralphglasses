package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterPrompts registers all MCP prompts with the server.
func RegisterPrompts(s *server.MCPServer) {
	s.AddPrompt(
		mcp.NewPrompt("daily-briefing",
			mcp.WithPromptDescription("Morning briefing: weather, schedule, top tasks, unread emails, deadlines"),
		),
		handleDailyBriefingPrompt,
	)

	s.AddPrompt(
		mcp.NewPrompt("weekly-review",
			mcp.WithPromptDescription("Weekly review: completed tasks, goal progress, journal themes, next week preview"),
		),
		handleWeeklyReviewPrompt,
	)

	s.AddPrompt(
		mcp.NewPrompt("inbox-triage",
			mcp.WithPromptDescription("Triage unread emails: categorize, flag action items, draft quick replies"),
		),
		handleInboxTriagePrompt,
	)
}

func handleDailyBriefingPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Description: "Generate a morning briefing",
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: `Generate my daily briefing by:

1. Run runmylife_calendar(domain=events, action=list, days=1) for today's schedule
2. Run runmylife_tasks(domain=prioritize, action=matrix) for task priorities
3. Run runmylife_gmail(domain=triage, action=unread) for unread emails
4. Run runmylife_sync(domain=status, action=check) for data freshness

Compile into a concise morning briefing with:
- Today's schedule (with times)
- Top 3 priorities from the Eisenhower matrix
- Unread email count and any urgent items
- Any approaching deadlines`,
				},
			},
		},
	}, nil
}

func handleWeeklyReviewPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Description: "Generate a weekly review",
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: `Generate my weekly review by:

1. Run runmylife_tasks(domain=manage, action=list, filter=all) to see all tasks
2. Run runmylife_calendar(domain=events, action=list, days=7) for next week's schedule
3. Run runmylife_admin(domain=db, action=status) for system health

Compile into a weekly review with:
- Tasks completed this week
- Active tasks and their status
- Next week's calendar preview
- Areas needing attention`,
				},
			},
		},
	}, nil
}

func handleInboxTriagePrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Description: "Triage unread emails",
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: `Triage my inbox by:

1. Run runmylife_gmail(domain=triage, action=unread, limit=30) for unread messages
2. Categorize each message as: Action Required, Financial, Job Lead, FYI, or Archive
3. For Action Required items, draft a brief task or reply

Output a triage report with:
- Summary counts by category
- Action items with suggested next steps
- Any urgent/time-sensitive items flagged`,
				},
			},
		},
	}, nil
}
