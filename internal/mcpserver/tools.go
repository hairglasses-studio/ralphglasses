package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/mcpkit/observability"
	"github.com/hairglasses-studio/ralphglasses/internal/bandit"
	"github.com/hairglasses-studio/ralphglasses/internal/blackboard"
	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/enhancer/fewshot"
	"github.com/hairglasses-studio/ralphglasses/internal/eval"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
	"github.com/hairglasses-studio/ralphglasses/internal/hooks"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/plugin"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/promptdj"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// ToolGroup represents a deferred-load group of related tools.
type ToolGroup struct {
	Name        string
	Description string
	Tools       []ToolEntry
}

// ToolEntry pairs a tool definition with its handler.
type ToolEntry struct {
	Tool    mcp.Tool
	Handler server.ToolHandlerFunc
}

// Server holds state for the MCP server.
type Server struct {
	mu                sync.RWMutex
	ScanPath          string
	Version           string
	Commit            string
	BuildDate         string
	Repos             []*model.Repo
	lastScanAt        time.Time     // when the last successful scan completed
	scanTTL           time.Duration // how long scan results are considered fresh (0 = forever)
	ProcMgr           *process.Manager
	SessMgr           *session.Manager
	EventBus          *events.Bus
	HTTPClient        *http.Client
	Engine            *enhancer.HybridEngine
	engineOnce        sync.Once
	ToolRecorder      *ToolCallRecorder
	DiscoveryRecorder *DiscoveryUsageRecorder

	// DeferredLoading controls whether only core tools are registered on startup.
	// When true, RegisterCoreTools is called instead of RegisterAllTools.
	DeferredLoading bool

	// loadedGroups tracks which tool groups have been registered (for deferred loading).
	loadedGroups map[string]bool

	// mcpSrv holds a reference to the MCPServer for deferred group loading.
	mcpSrv *server.MCPServer

	// Fleet and HITL infrastructure (set via InitFleetTools / InitSelfImprovement).
	FleetCoordinator      *fleet.Coordinator
	FleetClient           *fleet.Client
	HITLTracker           *session.HITLTracker
	DecisionLog           *session.DecisionLog
	FeedbackAnalyzer      *session.FeedbackAnalyzer
	AutoOptimizer         *session.AutoOptimizer
	feedbackWasAutoSeeded bool

	// Fleet analytics engine.
	FleetAnalytics *fleet.FleetAnalytics

	// Phase H subsystems (set via setter methods).
	Blackboard    *blackboard.Blackboard
	A2A           *fleet.A2AAdapter
	CostPredictor *fleet.CostPredictor

	// Bandit: provider selection independent of cascade routing.
	Bandit *bandit.Selector

	// PluginRegistry holds the plugin system registry for MCP plugin tools.
	PluginRegistry *plugin.Registry

	// MetricCollector collects A/B metrics from session and loop events.
	MetricCollector *eval.MetricCollector

	// Tasks is the async task registry for long-running MCP tool operations.
	Tasks *TaskRegistry

	// Active CLI-parity runtimes managed through MCP.
	FleetRuntime    *fleetRuntimeState
	MarathonRuntime *marathonRuntimeState

	// approvalStore holds in-memory approval records for HITL workflows.
	approvalStore *ApprovalStore

	// djRouter is the Prompt DJ routing engine (lazy-initialized).
	djRouter *promptdj.PromptDJRouter

	// fewshotRetriever is the few-shot example retriever (lazy-initialized).
	fewshotRetriever *fewshot.Retriever

	// PrefetchRunnerInstance is the deterministic context pre-fetch runner
	// (lazy-initialized via prefetchRunner()).
	PrefetchRunnerInstance *session.PrefetchRunner

	// Observability provider from mcpkit.
	Observability *observability.Provider

	// HookExecutor manages repo-level safety hooks.
	HookExecutor *hooks.Executor
}

// DefaultScanTTL is how long repo scan results are considered fresh before
// a lazy re-scan is triggered. Explicit scan tool calls bypass this TTL.
const DefaultScanTTL = 30 * time.Second

// NewServer creates a new MCP server instance.
func NewServer(scanPath string) *Server {
	return &Server{
		ScanPath:          scanPath,
		scanTTL:           DefaultScanTTL,
		ProcMgr:           process.NewManager(),
		SessMgr:           session.NewManager(),
		HTTPClient:        &http.Client{Timeout: 30 * time.Second},
		DiscoveryRecorder: NewDiscoveryUsageRecorder(filepath.Join(scanPath, ".ralph", "discovery_usage.jsonl")),
		HookExecutor:      hooks.NewExecutor(nil),
	}
}

// NewServerWithBus creates a new MCP server instance with an event bus.
func NewServerWithBus(scanPath string, bus *events.Bus) *Server {
	return &Server{
		ScanPath:          scanPath,
		scanTTL:           DefaultScanTTL,
		ProcMgr:           process.NewManagerWithBus(bus),
		SessMgr:           session.NewManagerWithBus(bus),
		EventBus:          bus,
		HTTPClient:        &http.Client{Timeout: 30 * time.Second},
		FleetAnalytics:    fleet.NewFleetAnalytics(10000, 24*time.Hour),
		MetricCollector:   eval.NewMetricCollector(bus),
		Tasks:             NewTaskRegistry(),
		DiscoveryRecorder: NewDiscoveryUsageRecorder(filepath.Join(scanPath, ".ralph", "discovery_usage.jsonl")),
		HookExecutor:      hooks.NewExecutor(bus),
	}
}

// ToolGroupNames lists all valid tool group names in registration order.
var ToolGroupNames = []string{
	"core", "session", "loop", "prompt", "fleet",
	"repo", "roadmap", "team", "tenant", "awesome", "advanced", "events", "feedback", "eval", "fleet_h",
	"observability", "rdcycle", "plugin", "sweep",
	"rc", "autonomy", "workflow", "docs", "recovery", "promptdj", "a2a", "trigger", "approval",
	"context", "prefetch",
}

func (s *Server) scan() error {
	repos, err := discovery.Scan(context.Background(), s.ScanPath)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.Repos = repos
	s.lastScanAt = time.Now()
	s.mu.Unlock()
	return nil
}

// RACE FIX: return a shallow copy of the Repo struct so that callers
// (e.g. handleStatus → RefreshRepo) can safely mutate fields without
// racing with reposCopy or other concurrent readers of s.Repos.
func (s *Server) findRepo(name string) *model.Repo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.Repos {
		if r.Name == name {
			rc := *r
			return &rc
		}
	}
	return nil
}

func (s *Server) reposCopy() []*model.Repo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]*model.Repo, len(s.Repos))
	for i, r := range s.Repos {
		rc := *r
		cp[i] = &rc
	}
	return cp
}

func (s *Server) reposNil() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Repos == nil {
		return true
	}
	// Treat cached results as stale when TTL has elapsed.
	return s.scanTTL > 0 && time.Since(s.lastScanAt) > s.scanTTL
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: text,
		}},
	}
}

func jsonResult(v any) *mcp.CallToolResult {
	data, err := json.Marshal(v)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("json marshal: %v", err))
	}
	return textResult(string(data))
}

// emptyResult returns a standardized empty-collection response with a
// machine-parseable status and item_type so callers can distinguish between
// "no data" and "error" without string matching.
func emptyResult(itemType string) *mcp.CallToolResult {
	return jsonResult(map[string]any{
		"status":    "empty",
		"items":     []any{},
		"item_type": itemType,
	})
}

func argsMap(req mcp.CallToolRequest) map[string]any {
	if m, ok := req.Params.Arguments.(map[string]any); ok {
		return m
	}
	return nil
}

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func getStringArg(req mcp.CallToolRequest, key string) string {
	m := argsMap(req)
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getNumberArg(req mcp.CallToolRequest, key string, defaultVal float64) float64 {
	m := argsMap(req)
	if m == nil {
		return defaultVal
	}
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case float32:
			return float64(n)
		case int:
			return float64(n)
		case int64:
			return float64(n)
		case int32:
			return float64(n)
		case json.Number:
			if f, err := n.Float64(); err == nil {
				return f
			}
		}
	}
	return defaultVal
}

func getBoolArg(req mcp.CallToolRequest, key string) bool {
	m := argsMap(req)
	if m == nil {
		return false
	}
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func splitCSV(s string) []string {
	var result []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}
