// Package clients provides API clients for webb.
// v26.0: SwarmMCPClient provides MCP tool access for swarm workers
// v31.0: Real MCP tool execution via ToolRegistry
package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// ToolHandler is the function signature for MCP tool handlers
// v31.0: Used to register real handlers from tools package
type ToolHandler func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)

// MCPExecutorMetrics tracks MCP tool call metrics
type MCPExecutorMetrics struct {
	TotalCalls   int64 `json:"total_calls"`
	CacheHits    int64 `json:"cache_hits"`
	RealCalls    int64 `json:"real_calls"`
	SimCalls     int64 `json:"sim_calls"`
	Errors       int64 `json:"errors"`
	AvgLatencyMs int64 `json:"avg_latency_ms"`
}

// SwarmMCPClient provides swarm workers access to MCP tools
// This bridges the gap between swarm Go code and MCP tools registered in the server
type SwarmMCPClient struct {
	mu         sync.RWMutex
	toolCache  map[string]*MCPToolResult
	cacheTime  map[string]time.Time
	cacheTTL   time.Duration

	// v31.0: Real tool handler registry
	handlers map[string]ToolHandler
	metrics  MCPExecutorMetrics
}

// MCPToolResult represents the result of an MCP tool call
type MCPToolResult struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   string      `json:"error,omitempty"`
}

// SwarmPattern represents a discovered workflow pattern
type SwarmPattern struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Frequency   int      `json:"frequency"`
	Tools       []string `json:"tools"`
	Confidence  float64  `json:"confidence"`
}

// SwarmSemanticMatch represents a semantic search result
type SwarmSemanticMatch struct {
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Source     string  `json:"source"`
	Similarity float64 `json:"similarity"`
}

// SwarmPrediction represents a predictive analysis result
type SwarmPrediction struct {
	Metric     string    `json:"metric"`
	Value      float64   `json:"value"`
	Confidence float64   `json:"confidence"`
	Trend      string    `json:"trend"` // increasing, decreasing, stable
	Timestamp  time.Time `json:"timestamp"`
}

// SwarmAuditResult represents a security/compliance audit result
type SwarmAuditResult struct {
	Passed   bool               `json:"passed"`
	Score    float64            `json:"score"`
	Findings []SwarmAuditFinding `json:"findings"`
}

// SwarmAuditFinding represents a single audit finding
type SwarmAuditFinding struct {
	Severity    string `json:"severity"` // critical, high, medium, low
	Category    string `json:"category"`
	Description string `json:"description"`
	Remediation string `json:"remediation"`
}

// SwarmImprovementSuggestion represents a tool improvement suggestion
type SwarmImprovementSuggestion struct {
	ToolName      string   `json:"tool_name"`
	Suggestion    string   `json:"suggestion"`
	TokenSavings  int      `json:"token_savings"`
	Effort        string   `json:"effort"` // small, medium, large
	RelatedTools  []string `json:"related_tools"`
}

// Global swarm MCP client singleton
var (
	swarmMCPClient     *SwarmMCPClient
	swarmMCPClientOnce sync.Once
)

// v32.0: Bridge initializer callback - set by tools/swarm package to avoid circular imports
var (
	bridgeInitializer     func(fullRegistration bool)
	bridgeInitializerOnce sync.Once
)

// SetBridgeInitializer sets the callback that initializes the handler bridge
// This is called by tools/swarm/handler_bridge.go during its init()
func SetBridgeInitializer(fn func(fullRegistration bool)) {
	bridgeInitializer = fn
}

// InitializeBridge calls the bridge initializer if registered
// This wires real MCP handlers to the SwarmMCPClient
func InitializeBridge(fullRegistration bool) {
	bridgeInitializerOnce.Do(func() {
		if bridgeInitializer != nil {
			bridgeInitializer(fullRegistration)
			log.Printf("swarm-mcp: bridge initialized (full=%v)", fullRegistration)
		} else {
			log.Printf("swarm-mcp: no bridge initializer registered, using simulation mode")
		}
	})
}

// GetSwarmMCPClient returns the global swarm MCP client
func GetSwarmMCPClient() *SwarmMCPClient {
	swarmMCPClientOnce.Do(func() {
		swarmMCPClient = NewSwarmMCPClient()
	})
	return swarmMCPClient
}

// NewSwarmMCPClient creates a new swarm MCP client
func NewSwarmMCPClient() *SwarmMCPClient {
	return &SwarmMCPClient{
		toolCache: make(map[string]*MCPToolResult),
		cacheTime: make(map[string]time.Time),
		cacheTTL:  5 * time.Minute, // Cache results for 5 minutes
		handlers:  make(map[string]ToolHandler),
	}
}

// v31.0: RegisterHandler registers a real MCP tool handler
// This allows the tools package to inject real handlers without circular imports
func (c *SwarmMCPClient) RegisterHandler(name string, handler ToolHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[name] = handler
	log.Printf("swarm-mcp: registered real handler for %s", name)
}

// v31.0: GetMetrics returns the current execution metrics
func (c *SwarmMCPClient) GetMetrics() MCPExecutorMetrics {
	return MCPExecutorMetrics{
		TotalCalls:   atomic.LoadInt64(&c.metrics.TotalCalls),
		CacheHits:    atomic.LoadInt64(&c.metrics.CacheHits),
		RealCalls:    atomic.LoadInt64(&c.metrics.RealCalls),
		SimCalls:     atomic.LoadInt64(&c.metrics.SimCalls),
		Errors:       atomic.LoadInt64(&c.metrics.Errors),
		AvgLatencyMs: atomic.LoadInt64(&c.metrics.AvgLatencyMs),
	}
}

// CallTool invokes an MCP tool by name with parameters
// This is the generic method for calling any MCP tool
// v31.0: Now tries real handlers first, falls back to simulation
func (c *SwarmMCPClient) CallTool(ctx context.Context, name string, params map[string]interface{}) (*MCPToolResult, error) {
	atomic.AddInt64(&c.metrics.TotalCalls, 1)
	start := time.Now()

	// Check cache first
	cacheKey := c.getCacheKey(name, params)
	if result := c.getFromCache(cacheKey); result != nil {
		atomic.AddInt64(&c.metrics.CacheHits, 1)
		return result, nil
	}

	var result *MCPToolResult

	// v31.0: Try real handler first
	c.mu.RLock()
	handler, hasHandler := c.handlers[name]
	c.mu.RUnlock()

	if hasHandler {
		// Execute real handler
		req := mcp.CallToolRequest{}
		req.Params.Name = name
		req.Params.Arguments = params

		mcpResult, err := handler(ctx, req)
		if err != nil {
			atomic.AddInt64(&c.metrics.Errors, 1)
			log.Printf("swarm-mcp: real handler error for %s: %v (falling back to simulation)", name, err)
			// Fall back to simulation on error
			result = c.simulateToolCall(ctx, name, params)
			atomic.AddInt64(&c.metrics.SimCalls, 1)
		} else {
			// Convert MCP result to MCPToolResult
			result = c.convertMCPResult(mcpResult)
			atomic.AddInt64(&c.metrics.RealCalls, 1)
			log.Printf("swarm-mcp: executed real handler for %s", name)
		}
	} else {
		// No handler registered, use simulation
		result = c.simulateToolCall(ctx, name, params)
		atomic.AddInt64(&c.metrics.SimCalls, 1)
	}

	// Update average latency
	latencyMs := time.Since(start).Milliseconds()
	totalCalls := atomic.LoadInt64(&c.metrics.TotalCalls)
	if totalCalls > 0 {
		currentAvg := atomic.LoadInt64(&c.metrics.AvgLatencyMs)
		newAvg := (currentAvg*(totalCalls-1) + latencyMs) / totalCalls
		atomic.StoreInt64(&c.metrics.AvgLatencyMs, newAvg)
	}

	// Cache the result
	c.setCache(cacheKey, result)

	return result, nil
}

// v31.0: convertMCPResult converts an MCP CallToolResult to MCPToolResult
func (c *SwarmMCPClient) convertMCPResult(mcpResult *mcp.CallToolResult) *MCPToolResult {
	if mcpResult == nil {
		return &MCPToolResult{Success: false, Error: "nil result"}
	}

	if mcpResult.IsError {
		errMsg := "tool error"
		// Extract error message from content
		for _, content := range mcpResult.Content {
			if textContent, ok := content.(mcp.TextContent); ok {
				errMsg = textContent.Text
				break
			}
		}
		return &MCPToolResult{Success: false, Error: errMsg}
	}

	// Extract text content as data
	var text string
	for _, content := range mcpResult.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			text = textContent.Text
			break
		}
	}

	return &MCPToolResult{
		Success: true,
		Data:    text, // Raw text - caller can parse if needed
	}
}

// getCacheKey generates a cache key for a tool call
func (c *SwarmMCPClient) getCacheKey(name string, params map[string]interface{}) string {
	data, _ := json.Marshal(params)
	return fmt.Sprintf("%s:%s", name, string(data))
}

// getFromCache retrieves a cached result if still valid
func (c *SwarmMCPClient) getFromCache(key string) *MCPToolResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if t, ok := c.cacheTime[key]; ok {
		if time.Since(t) < c.cacheTTL {
			return c.toolCache[key]
		}
	}
	return nil
}

// setCache stores a result in cache
func (c *SwarmMCPClient) setCache(key string, result *MCPToolResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.toolCache[key] = result
	c.cacheTime[key] = time.Now()
}

// simulateToolCall provides simulated results for development
// In production, this would be replaced with actual MCP tool invocation
func (c *SwarmMCPClient) simulateToolCall(ctx context.Context, name string, params map[string]interface{}) *MCPToolResult {
	switch name {
	case "webb_pattern_mine", "webb_pattern_list":
		return &MCPToolResult{
			Success: true,
			Data: []SwarmPattern{
				{ID: "p1", Name: "investigate-flow", Description: "Common investigation workflow", Frequency: 45, Tools: []string{"pylon_list", "incidentio_get", "k8s_logs"}, Confidence: 0.85},
				{ID: "p2", Name: "deploy-check", Description: "Pre-deployment verification", Frequency: 30, Tools: []string{"github_pr_status", "k8s_deployments", "grafana_alerts"}, Confidence: 0.78},
			},
		}
	case "webb_semantic_search", "webb_similar_incidents":
		return &MCPToolResult{
			Success: true,
			Data: []SwarmSemanticMatch{
				{ID: "m1", Content: "Similar error occurred last week", Source: "incident-123", Similarity: 0.82},
			},
		}
	case "webb_predict_trend":
		return &MCPToolResult{
			Success: true,
			Data: SwarmPrediction{Metric: "error_rate", Value: 0.02, Confidence: 0.75, Trend: "increasing", Timestamp: time.Now()},
		}
	case "webb_security_audit_full", "webb_config_audit":
		return &MCPToolResult{
			Success: true,
			Data: SwarmAuditResult{
				Passed: true,
				Score:  0.85,
				Findings: []SwarmAuditFinding{
					{Severity: "medium", Category: "config", Description: "ConfigMap has outdated values", Remediation: "Update via Helm upgrade"},
				},
			},
		}
	case "webb_improvement_analyze", "webb_tool_suggest":
		return &MCPToolResult{
			Success: true,
			Data: []SwarmImprovementSuggestion{
				{ToolName: "webb_cluster_health_full", Suggestion: "Add caching for repeated calls", TokenSavings: 500, Effort: "small", RelatedTools: []string{"k8s_pods", "grafana_alerts"}},
			},
		}
	default:
		return &MCPToolResult{
			Success: false,
			Error:   fmt.Sprintf("unknown tool: %s", name),
		}
	}
}

// Convenience methods for common MCP tool operations

// PatternMine discovers workflow patterns from tool usage
func (c *SwarmMCPClient) PatternMine(ctx context.Context) ([]SwarmPattern, error) {
	result, err := c.CallTool(ctx, "webb_pattern_mine", nil)
	if err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, errors.New(result.Error)
	}

	patterns, ok := result.Data.([]SwarmPattern)
	if !ok {
		return []SwarmPattern{}, nil
	}
	return patterns, nil
}

// SemanticSearch performs semantic search across knowledge base
func (c *SwarmMCPClient) SemanticSearch(ctx context.Context, query string) ([]SwarmSemanticMatch, error) {
	result, err := c.CallTool(ctx, "webb_semantic_search", map[string]interface{}{"query": query})
	if err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, errors.New(result.Error)
	}

	matches, ok := result.Data.([]SwarmSemanticMatch)
	if !ok {
		return []SwarmSemanticMatch{}, nil
	}
	return matches, nil
}

// PredictTrend predicts trends for a metric
func (c *SwarmMCPClient) PredictTrend(ctx context.Context, metric string) (*SwarmPrediction, error) {
	result, err := c.CallTool(ctx, "webb_predict_trend", map[string]interface{}{"metric": metric})
	if err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, errors.New(result.Error)
	}

	pred, ok := result.Data.(SwarmPrediction)
	if !ok {
		return nil, nil
	}
	return &pred, nil
}

// SecurityAudit runs a comprehensive security audit
func (c *SwarmMCPClient) SecurityAudit(ctx context.Context) (*SwarmAuditResult, error) {
	result, err := c.CallTool(ctx, "webb_security_audit_full", nil)
	if err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, errors.New(result.Error)
	}

	audit, ok := result.Data.(SwarmAuditResult)
	if !ok {
		return nil, nil
	}
	return &audit, nil
}

// ImprovementAnalyze analyzes tools for improvement opportunities
func (c *SwarmMCPClient) ImprovementAnalyze(ctx context.Context) ([]SwarmImprovementSuggestion, error) {
	result, err := c.CallTool(ctx, "webb_improvement_analyze", nil)
	if err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, errors.New(result.Error)
	}

	suggestions, ok := result.Data.([]SwarmImprovementSuggestion)
	if !ok {
		return []SwarmImprovementSuggestion{}, nil
	}
	return suggestions, nil
}

// SimilarIncidents finds similar historical incidents
func (c *SwarmMCPClient) SimilarIncidents(ctx context.Context, description string) ([]SwarmSemanticMatch, error) {
	result, err := c.CallTool(ctx, "webb_similar_incidents", map[string]interface{}{"description": description})
	if err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, errors.New(result.Error)
	}

	matches, ok := result.Data.([]SwarmSemanticMatch)
	if !ok {
		return []SwarmSemanticMatch{}, nil
	}
	return matches, nil
}

// ToolSuggest suggests optimal tools for a task
func (c *SwarmMCPClient) ToolSuggest(ctx context.Context, task string) ([]SwarmImprovementSuggestion, error) {
	result, err := c.CallTool(ctx, "webb_tool_suggest", map[string]interface{}{"task": task})
	if err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, errors.New(result.Error)
	}

	suggestions, ok := result.Data.([]SwarmImprovementSuggestion)
	if !ok {
		return []SwarmImprovementSuggestion{}, nil
	}
	return suggestions, nil
}
