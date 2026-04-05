// Package reddit provides MCP tools for Reddit integration.
package reddit

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

func (m *Module) Name() string        { return "reddit" }
func (m *Module) Description() string { return "Reddit browsing, saved posts, and subscriptions" }

var redditHints = map[string]string{
	"feed/home":           "Home feed (best posts)",
	"feed/subreddit":      "Posts from a subreddit",
	"saved/list":          "Saved posts and comments",
	"subscriptions/list":  "Subscribed subreddits",
	"search/find":         "Search Reddit",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("reddit").
		Domain("feed", common.ActionRegistry{
			"home":      handleFeedHome,
			"subreddit": handleFeedSubreddit,
		}).
		Domain("saved", common.ActionRegistry{
			"list": handleSavedList,
		}).
		Domain("subscriptions", common.ActionRegistry{
			"list": handleSubscriptionsList,
		}).
		Domain("search", common.ActionRegistry{
			"find": handleSearch,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_reddit",
				mcp.WithDescription("Reddit gateway for browsing and saved content.\n\n"+dispatcher.DescribeActionsWithHints(redditHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: feed, saved, subscriptions, search")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("subreddit", mcp.Description("Subreddit name (without r/)")),
				mcp.WithString("sort", mcp.Description("Sort: hot, new, top, rising")),
				mcp.WithString("query", mcp.Description("Search query")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 25)")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "reddit",
			Subcategory:         "gateway",
			Tags:                []string{"reddit", "social", "news"},
			Complexity:          tools.ComplexitySimple,
			CircuitBreakerGroup: "reddit_api",
			Timeout:             30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func redditClient() (*clients.RedditClient, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	token := cfg.Credentials["reddit"]
	if token == "" {
		return nil, fmt.Errorf("reddit token not configured — add 'reddit' to credentials in config.json")
	}
	return clients.NewRedditClient(token), nil
}

func handleFeedHome(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := redditClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add reddit token to config"), nil
	}
	limit := common.GetLimitParam(req, 25)
	posts, err := client.HomeFeed(ctx, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return formatPosts("Home Feed", posts), nil
}

func handleFeedSubreddit(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sub := common.GetStringParam(req, "subreddit", "")
	if sub == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "subreddit is required"), nil
	}
	client, err := redditClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add reddit token to config"), nil
	}
	sort := common.GetStringParam(req, "sort", "hot")
	limit := common.GetLimitParam(req, 25)
	posts, err := client.SubredditPosts(ctx, sub, sort, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return formatPosts(fmt.Sprintf("r/%s (%s)", sub, sort), posts), nil
}

func handleSavedList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := redditClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add reddit token to config"), nil
	}
	limit := common.GetLimitParam(req, 25)
	posts, err := client.SavedPosts(ctx, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return formatPosts("Saved Posts", posts), nil
}

func handleSubscriptionsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := redditClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add reddit token to config"), nil
	}
	limit := common.GetLimitParam(req, 100)
	subs, err := client.Subscriptions(ctx, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Subscriptions")
	headers := []string{"Subreddit", "Subscribers", "Description"}
	var rows [][]string
	for _, s := range subs {
		desc := s.Description
		if len(desc) > 80 {
			desc = desc[:80] + "..."
		}
		rows = append(rows, []string{s.Name, fmt.Sprintf("%d", s.Subscribers), desc})
	}
	if len(rows) == 0 {
		md.EmptyList("subscriptions")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := common.GetStringParam(req, "query", "")
	if query == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "query is required"), nil
	}
	client, err := redditClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add reddit token to config"), nil
	}
	limit := common.GetLimitParam(req, 25)
	posts, err := client.Search(ctx, query, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return formatPosts("Search: "+query, posts), nil
}

func formatPosts(title string, posts []clients.RedditPost) *mcp.CallToolResult {
	md := common.NewMarkdownBuilder().Title(title)
	headers := []string{"Score", "Subreddit", "Title", "Author"}
	var rows [][]string
	for _, p := range posts {
		t := p.Title
		if len(t) > 60 {
			t = t[:60] + "..."
		}
		rows = append(rows, []string{fmt.Sprintf("%d", p.Score), p.Subreddit, t, p.Author})
	}
	if len(rows) == 0 {
		md.EmptyList("posts")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String())
}
