package mcp

import (
	"database/sql"
	"log"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/middleware"
	"github.com/hairglasses-studio/runmylife/internal/resilience"

	// Import tool modules to trigger init() registration
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/admin"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/analytics"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/bluesky"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/briefing"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/calendar"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/clockify"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/contacts"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/discord"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/discovery"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/drive"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/finances"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/fitness"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/gmail"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/gtasks"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/habits"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/homeassistant"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/journal"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/knowledge"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/life/arthouse"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/life/growth"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/life/partner"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/life/personal"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/life/social"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/life/studio"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/life/wellness"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/messages"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/notion"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/readwise"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/reddit"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/slack"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/spotify"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/sync"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/tasks"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/weather"
	_ "github.com/hairglasses-studio/runmylife/internal/mcp/tools/workflows"
)

// RegisterTools registers all runmylife tools with the MCP server.
func RegisterTools(s *server.MCPServer) {
	registry := tools.GetRegistry()

	// Initialize tool usage tracking
	tools.SetTrackerDB(func() (*sql.DB, error) {
		database, err := common.OpenDB()
		if err != nil {
			return nil, err
		}
		return database.SqlDB(), nil
	})

	// Configure circuit breaker groups for external APIs
	cbRegistry := resilience.NewRegistry()
	cbRegistry.Configure("gmail_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 5, SuccessThreshold: 2, Timeout: 60 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("calendar_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 5, SuccessThreshold: 2, Timeout: 60 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("todoist_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3, SuccessThreshold: 2, Timeout: 120 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("notion_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3, SuccessThreshold: 2, Timeout: 120 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("weather_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3, SuccessThreshold: 2, Timeout: 120 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("discord_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3, SuccessThreshold: 2, Timeout: 120 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("drive_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 5, SuccessThreshold: 2, Timeout: 60 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("messages_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3, SuccessThreshold: 2, Timeout: 120 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("spotify_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3, SuccessThreshold: 2, Timeout: 120 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("reddit_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3, SuccessThreshold: 2, Timeout: 120 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("homeassistant_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3, SuccessThreshold: 2, Timeout: 120 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("fitbit_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3, SuccessThreshold: 2, Timeout: 120 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("readwise_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3, SuccessThreshold: 2, Timeout: 120 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("bluesky_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3, SuccessThreshold: 2, Timeout: 120 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("gtasks_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 5, SuccessThreshold: 2, Timeout: 60 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("clockify_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3, SuccessThreshold: 2, Timeout: 120 * time.Second, HalfOpenMaxCalls: 1,
	})
	cbRegistry.Configure("slack_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3, SuccessThreshold: 2, Timeout: 120 * time.Second, HalfOpenMaxCalls: 1,
	})

	// DB opener for middleware
	dbOpener := func() (*sql.DB, error) {
		database, err := common.OpenDB()
		if err != nil {
			return nil, err
		}
		return database.SqlDB(), nil
	}

	// Set up middleware chain: outermost (first) to innermost (last)
	registry.SetMiddleware([]tools.MiddlewareFunc{
		tools.MiddlewareFunc(middleware.LoggingMiddleware(dbOpener)),
		tools.MiddlewareFunc(middleware.CircuitBreakerMiddleware(cbRegistry)),
		tools.MiddlewareFunc(middleware.TimeoutMiddleware(30 * time.Second)),
		tools.MiddlewareFunc(middleware.HintsMiddleware()),
		tools.MiddlewareFunc(middleware.TruncationMiddleware(128 * 1024)),
	})

	if tools.UseLazyTools() {
		log.Printf("[runmylife] Lazy tool loading enabled - registering %d tools with minimal schemas", registry.ToolCount())
		registry.RegisterDiscoveryOnlyWithServer(s)
		return
	}

	log.Printf("[runmylife] Registering all %d tools with full schemas", registry.ToolCount())
	registry.RegisterWithServer(s)
}
