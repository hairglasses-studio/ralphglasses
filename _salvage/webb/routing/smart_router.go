package clients

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hairglasses-studio/webb/internal/mcp/otel"
)

// =============================================================================
// v106.0 STRIDE-101: Query Classification
// Based on arXiv:2512.02228 - STRIDE Framework for AI Modality Selection
// Classifies queries into: simple (direct LLM), complex (tool-assisted), agentic (autonomous)
// =============================================================================

// QueryModality represents the type of response modality needed
type QueryModality string

const (
	// ModalitySimple - Direct LLM response without tools (facts, explanations, definitions)
	ModalitySimple QueryModality = "simple"
	// ModalityComplex - Needs tool execution but not autonomous (status checks, queries)
	ModalityComplex QueryModality = "complex"
	// ModalityAgentic - Requires multi-step autonomous execution (investigation, troubleshooting)
	ModalityAgentic QueryModality = "agentic"
)

// QueryClassification contains the classification result with confidence
type QueryClassification struct {
	Modality    QueryModality `json:"modality"`
	Confidence  float64       `json:"confidence"` // 0.0 - 1.0
	Reason      string        `json:"reason"`
	Indicators  []string      `json:"indicators,omitempty"`
	SuggestedRoute string     `json:"suggested_route,omitempty"`
}

// QueryClassifier classifies natural language queries into modalities
type QueryClassifier struct {
	// Patterns for each modality
	simplePatterns  []classifierPattern
	complexPatterns []classifierPattern
	agenticPatterns []classifierPattern
}

type classifierPattern struct {
	pattern *regexp.Regexp
	weight  float64
	reason  string
}

// NewQueryClassifier creates a new STRIDE-based query classifier
func NewQueryClassifier() *QueryClassifier {
	return &QueryClassifier{
		simplePatterns: []classifierPattern{
			// Direct questions about facts/definitions
			{regexp.MustCompile(`(?i)^what\s+is\s+(a\s+)?[a-z]+\??$`), 0.9, "definition question"},
			{regexp.MustCompile(`(?i)^how\s+does\s+\w+\s+work\??$`), 0.8, "explanation question"},
			{regexp.MustCompile(`(?i)^explain\s+`), 0.7, "explanation request"},
			{regexp.MustCompile(`(?i)^define\s+`), 0.9, "definition request"},
			{regexp.MustCompile(`(?i)^describe\s+the\s+difference`), 0.8, "comparison question"},
			{regexp.MustCompile(`(?i)^what\s+are\s+the\s+benefits\s+of`), 0.7, "benefits question"},
			{regexp.MustCompile(`(?i)^why\s+(is|are|do|does)\s+`), 0.6, "why question"},
			// General knowledge questions
			{regexp.MustCompile(`(?i)^when\s+should\s+i\s+use`), 0.7, "usage guidance"},
			{regexp.MustCompile(`(?i)^what\s+does\s+\w+\s+mean`), 0.8, "meaning question"},
			{regexp.MustCompile(`(?i)^can\s+you\s+explain`), 0.7, "explanation request"},
		},
		complexPatterns: []classifierPattern{
			// Status/health queries
			{regexp.MustCompile(`(?i)(what\s+is\s+the\s+)?(health|status)\s+(of|for)`), 0.9, "status query"},
			{regexp.MustCompile(`(?i)^(show|get|list|check)\s+(me\s+)?(the\s+)?`), 0.8, "data retrieval"},
			{regexp.MustCompile(`(?i)(any|are\s+there)\s+(open\s+)?(tickets|issues|incidents|alerts)`), 0.85, "ticket query"},
			{regexp.MustCompile(`(?i)^how\s+(many|much)`), 0.7, "count query"},
			// Single-tool operations
			{regexp.MustCompile(`(?i)^search\s+(for|slack|in)`), 0.8, "search query"},
			{regexp.MustCompile(`(?i)^find\s+(the|all|any)`), 0.75, "find query"},
			{regexp.MustCompile(`(?i)(queue|pod|deployment)\s+(status|health|info)`), 0.85, "resource status"},
			{regexp.MustCompile(`(?i)^what\s+(happened|changed)\s+(in|to|with)`), 0.8, "history query"},
			// Customer/cluster specific
			{regexp.MustCompile(`(?i)^\w+\s+(cluster|environment)\s+(status|health)`), 0.9, "cluster status"},
			{regexp.MustCompile(`(?i)^customer\s+\w+\s+(status|health|snapshot)`), 0.85, "customer status"},
		},
		agenticPatterns: []classifierPattern{
			// Investigation/troubleshooting
			{regexp.MustCompile(`(?i)^(investigate|troubleshoot|diagnose|debug)`), 0.95, "investigation request"},
			{regexp.MustCompile(`(?i)^(why\s+is|figure\s+out\s+why)`), 0.85, "root cause analysis"},
			{regexp.MustCompile(`(?i)^help\s+me\s+(fix|resolve|understand)`), 0.8, "assistance request"},
			{regexp.MustCompile(`(?i)(root\s+cause|rca|failure\s+analysis)`), 0.9, "RCA request"},
			// Multi-step operations
			{regexp.MustCompile(`(?i)^(create|set\s+up|configure|deploy)\s+`), 0.85, "create operation"},
			{regexp.MustCompile(`(?i)^(run|execute)\s+(the\s+)?(workflow|pipeline|preflight)`), 0.9, "workflow execution"},
			{regexp.MustCompile(`(?i)(and\s+then|after\s+that|followed\s+by)`), 0.8, "multi-step indicator"},
			// Comparative/complex analysis
			{regexp.MustCompile(`(?i)^(compare|analyze|correlate)`), 0.85, "analysis request"},
			{regexp.MustCompile(`(?i)^what\s+caused`), 0.8, "causation query"},
			{regexp.MustCompile(`(?i)^(escalate|triage|prioritize)`), 0.9, "triage request"},
			// Open-ended investigation
			{regexp.MustCompile(`(?i)(something\s+is\s+wrong|having\s+issues|broken|not\s+working)`), 0.85, "problem report"},
			{regexp.MustCompile(`(?i)^(check|verify)\s+everything`), 0.8, "comprehensive check"},
		},
	}
}

// Classify analyzes a query and returns its classification
func (c *QueryClassifier) Classify(question string) *QueryClassification {
	question = strings.ToLower(strings.TrimSpace(question))

	// Calculate scores for each modality
	simpleScore, simpleIndicators := c.scoreModality(question, c.simplePatterns)
	complexScore, complexIndicators := c.scoreModality(question, c.complexPatterns)
	agenticScore, agenticIndicators := c.scoreModality(question, c.agenticPatterns)

	// Apply contextual adjustments
	// Short questions tend to be simpler
	if len(question) < 30 && simpleScore > 0 {
		simpleScore += 0.1
	}
	// Long questions with multiple clauses tend to be more agentic
	if len(question) > 100 || strings.Contains(question, " and ") {
		agenticScore += 0.1
	}
	// Questions with specific entities (clusters, customers) are complex or agentic
	if hasSpecificEntity(question) {
		if agenticScore > complexScore {
			agenticScore += 0.15
		} else {
			complexScore += 0.15
		}
	}

	// Normalize scores
	total := simpleScore + complexScore + agenticScore
	if total == 0 {
		// Default to complex for domain-specific questions
		return &QueryClassification{
			Modality:   ModalityComplex,
			Confidence: 0.5,
			Reason:     "no strong indicators, defaulting to tool-assisted",
		}
	}

	simpleScore /= total
	complexScore /= total
	agenticScore /= total

	// Select highest scoring modality
	result := &QueryClassification{}
	if simpleScore >= complexScore && simpleScore >= agenticScore {
		result.Modality = ModalitySimple
		result.Confidence = simpleScore
		result.Indicators = simpleIndicators
		result.Reason = "direct LLM response appropriate"
	} else if complexScore >= agenticScore {
		result.Modality = ModalityComplex
		result.Confidence = complexScore
		result.Indicators = complexIndicators
		result.Reason = "single tool execution needed"
	} else {
		result.Modality = ModalityAgentic
		result.Confidence = agenticScore
		result.Indicators = agenticIndicators
		result.Reason = "multi-step autonomous execution required"
	}

	return result
}

func (c *QueryClassifier) scoreModality(question string, patterns []classifierPattern) (float64, []string) {
	score := 0.0
	indicators := []string{}

	for _, p := range patterns {
		if p.pattern.MatchString(question) {
			score += p.weight
			indicators = append(indicators, p.reason)
		}
	}

	return score, indicators
}

func hasSpecificEntity(question string) bool {
	// Check for cluster/customer indicators
	entityPatterns := []string{
		`headspace`, `verizon`, `john\s*deere`, `stanford`, `staging`, `production`,
		`-v\d+`, `cluster`, `customer`, `environment`,
	}
	for _, pattern := range entityPatterns {
		if matched, _ := regexp.MatchString(`(?i)`+pattern, question); matched {
			return true
		}
	}
	return false
}

// SmartRouterResult represents the result of smart routing
type SmartRouterResult struct {
	RoutedTo          string             `json:"routed_to"`
	ToolName          string             `json:"tool_name"`
	ExtractedContext  string             `json:"extracted_context,omitempty"`
	ExtractedCustomer string             `json:"extracted_customer,omitempty"`
	Output            string             `json:"output"`
	Error             string             `json:"error,omitempty"`
	// v106.0 STRIDE-102: Modality classification
	Classification    *QueryClassification `json:"classification,omitempty"`
}

// WeightedKeyword represents a keyword with its importance weight
type WeightedKeyword struct {
	Word   string
	Weight int // Higher = more important (1-10)
}

// RouteDefinition defines a routing rule with weighted keywords
type RouteDefinition struct {
	Name        string
	Keywords    []WeightedKeyword
	Description string
}

// SmartRouter routes questions to appropriate tools
type SmartRouter struct {
	routes            []RouteDefinition
	feedbackClient    *FeedbackClient
	tokenBudgetClient *TokenBudgetClient
	selfHealingClient *SelfHealingClient
	latencyTracker    *LatencyTrackerClient
	// v106.0 STRIDE-101/102: Query classifier for modality routing
	classifier        *QueryClassifier
}

// NewSmartRouter creates a new smart router with weighted keywords
func NewSmartRouter() *SmartRouter {
	return &SmartRouter{
		feedbackClient:    GetFeedbackClient(),
		tokenBudgetClient: GetTokenBudgetClient(),
		selfHealingClient: GetSelfHealingClient(),
		latencyTracker:    GetLatencyTrackerClient(),
		classifier:        NewQueryClassifier(), // v106.0 STRIDE
		routes: []RouteDefinition{
			{
				Name: "cluster_health_full",
				Keywords: []WeightedKeyword{
					{Word: "cluster", Weight: 8},
					{Word: "pods", Weight: 7},
					{Word: "deployments", Weight: 7},
					{Word: "k8s", Weight: 9},
					{Word: "kubernetes", Weight: 9},
					{Word: "health", Weight: 3}, // Generic, low weight
					{Word: "status", Weight: 2}, // Very generic
				},
				Description: "Cluster health check (K8s + queues + alerts)",
			},
			{
				Name: "ticket_summary",
				Keywords: []WeightedKeyword{
					{Word: "tickets", Weight: 9},
					{Word: "pylon", Weight: 10},
					{Word: "incidents", Weight: 7},
					{Word: "bugs", Weight: 6},
					{Word: "support", Weight: 5},
					{Word: "issues", Weight: 2}, // Very generic, low weight
				},
				Description: "Unified ticket view (Pylon + incidents + Shortcut)",
			},
			{
				Name: "queue_health",
				Keywords: []WeightedKeyword{
					{Word: "queue", Weight: 10},
					{Word: "queues", Weight: 10},
					{Word: "rabbitmq", Weight: 10},
					{Word: "rabbit", Weight: 9},
					{Word: "jobs", Weight: 6},
					{Word: "backlog", Weight: 8},
					{Word: "messages", Weight: 5},
					{Word: "consumers", Weight: 8},
					{Word: "dlq", Weight: 9},
					{Word: "dead letter", Weight: 9},
				},
				Description: "Queue health analysis",
			},
			{
				Name: "aws_infrastructure_status",
				Keywords: []WeightedKeyword{
					{Word: "aws", Weight: 10},
					{Word: "ecs", Weight: 9},
					{Word: "rds", Weight: 9},
					{Word: "infrastructure", Weight: 7},
					{Word: "alarms", Weight: 6},
					{Word: "cloudwatch", Weight: 8},
					{Word: "asg", Weight: 8},
					{Word: "lambda", Weight: 7},
					{Word: "ec2", Weight: 7},
				},
				Description: "AWS infrastructure status",
			},
			{
				Name: "standup_briefing",
				Keywords: []WeightedKeyword{
					{Word: "standup", Weight: 10},
					{Word: "morning", Weight: 8},
					{Word: "briefing", Weight: 9},
					{Word: "daily", Weight: 5},
					{Word: "start", Weight: 2},
				},
				Description: "Morning standup briefing",
			},
			{
				Name: "customer_snapshot",
				Keywords: []WeightedKeyword{
					{Word: "snapshot", Weight: 9},
					{Word: "customer", Weight: 6},
					{Word: "environment", Weight: 5},
					{Word: "overview", Weight: 4},
				},
				Description: "Customer environment snapshot",
			},
			{
				Name: "investigation_analysis",
				Keywords: []WeightedKeyword{
					{Word: "investigate", Weight: 9},
					{Word: "investigation", Weight: 9},
					{Word: "analysis", Weight: 6},
					{Word: "analyze", Weight: 6},
					{Word: "debug", Weight: 7},
					{Word: "troubleshoot", Weight: 8},
				},
				Description: "Investigation analysis",
			},
			{
				Name: "eval_debug",
				Keywords: []WeightedKeyword{
					{Word: "evaluation", Weight: 10},
					{Word: "eval", Weight: 10},
					{Word: "metrics", Weight: 7},
					{Word: "stuck", Weight: 5},
					{Word: "pending", Weight: 4},
					{Word: "runners", Weight: 8},
				},
				Description: "Evaluation debugging",
			},
			{
				Name: "escalation_preflight",
				Keywords: []WeightedKeyword{
					{Word: "escalate", Weight: 10},
					{Word: "escalation", Weight: 10},
					{Word: "preflight", Weight: 9},
					{Word: "should i escalate", Weight: 10},
					{Word: "need to escalate", Weight: 9},
					{Word: "engineering", Weight: 4},
				},
				Description: "Pre-escalation checks",
			},
			{
				Name: "field_investigate",
				Keywords: []WeightedKeyword{
					{Word: "field", Weight: 9},
					{Word: "fde", Weight: 10},
					{Word: "cse", Weight: 10},
					{Word: "quick fix", Weight: 8},
					{Word: "quickfix", Weight: 8},
					{Word: "self-service", Weight: 7},
					{Word: "guided", Weight: 5},
				},
				Description: "Field team investigation",
			},
			// Slack routing
			{
				Name: "slack_search",
				Keywords: []WeightedKeyword{
					{Word: "slack search", Weight: 10},
					{Word: "search slack", Weight: 10},
					{Word: "find in slack", Weight: 9},
					{Word: "slack message", Weight: 7},
					{Word: "slack conversation", Weight: 7},
					{Word: "mentioned", Weight: 4},
				},
				Description: "Search Slack messages and conversations",
			},
			{
				Name: "slack_schedule_message",
				Keywords: []WeightedKeyword{
					{Word: "schedule message", Weight: 10},
					{Word: "schedule slack", Weight: 10},
					{Word: "post later", Weight: 9},
					{Word: "send tomorrow", Weight: 9},
					{Word: "remind", Weight: 5},
					{Word: "later", Weight: 2},
				},
				Description: "Schedule a Slack message for later",
			},
			{
				Name: "slack_channel_create",
				Keywords: []WeightedKeyword{
					{Word: "create channel", Weight: 10},
					{Word: "new channel", Weight: 10},
					{Word: "make channel", Weight: 9},
					{Word: "slack channel", Weight: 6},
				},
				Description: "Create a new Slack channel",
			},
			{
				Name: "slack_history",
				Keywords: []WeightedKeyword{
					{Word: "slack history", Weight: 10},
					{Word: "channel history", Weight: 9},
					{Word: "recent messages", Weight: 7},
					{Word: "what happened", Weight: 4},
					{Word: "conversation", Weight: 4},
				},
				Description: "Get Slack channel history",
			},
			{
				Name: "incident_channel_setup",
				Keywords: []WeightedKeyword{
					{Word: "incident channel", Weight: 10},
					{Word: "create incident", Weight: 8},
					{Word: "setup incident", Weight: 9},
					{Word: "war room", Weight: 9},
					{Word: "bridge", Weight: 6},
				},
				Description: "Set up incident Slack channel with context",
			},
		},
	}
}

// RouteParams contains parameters for routing
type RouteParams struct {
	Question string
	Context  string
	Customer string
}

// Route analyzes the question and routes to the appropriate tool
func (r *SmartRouter) Route(ctx context.Context, params RouteParams) (*SmartRouterResult, error) {
	result := &SmartRouterResult{}

	// Normalize question
	question := strings.ToLower(params.Question)

	// v106.0 STRIDE-101/102: Classify query modality first
	classification := r.classifier.Classify(params.Question)
	result.Classification = classification

	// Record modality classification metrics
	otel.RecordModalityClassification(ctx, string(classification.Modality), classification.Confidence)

	// Extract context from question if not provided
	if params.Context == "" {
		params.Context = r.extractContext(question)
	}
	result.ExtractedContext = params.Context

	// Extract customer from question if not provided
	if params.Customer == "" {
		params.Customer = r.extractCustomer(question)
	}
	result.ExtractedCustomer = params.Customer

	// v106.0 STRIDE-102: Route based on modality
	var bestRoute string
	switch classification.Modality {
	case ModalitySimple:
		// For simple queries, provide guidance but don't execute tools
		result.RoutedTo = "direct_response"
		result.ToolName = ""
		result.Output = r.generateSimpleResponse(params.Question, classification)
		otel.RecordRouteDecision(ctx, "direct_response", "simple_modality", true)
		return result, nil

	case ModalityAgentic:
		// For agentic queries, prefer investigation/workflow tools
		bestRoute = r.findAgenticRoute(question)
		otel.RecordRouteDecision(ctx, "webb_"+bestRoute, "agentic_modality", true)

	default: // ModalityComplex
		// For complex queries, use standard keyword-based routing
		bestRoute = r.findBestRoute(question)
	}

	if bestRoute == "" {
		// Default to investigation_analysis
		bestRoute = "investigation_analysis"
	}
	result.RoutedTo = bestRoute
	result.ToolName = "webb_" + bestRoute

	// Execute the routed tool
	output, err := r.executeRoute(ctx, bestRoute, params)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}
	result.Output = output

	return result, nil
}

// generateSimpleResponse creates a guidance response for simple queries
func (r *SmartRouter) generateSimpleResponse(question string, classification *QueryClassification) string {
	var sb strings.Builder
	sb.WriteString("**Query Classification:** Simple (direct response appropriate)\n\n")
	sb.WriteString(fmt.Sprintf("**Confidence:** %.0f%%\n", classification.Confidence*100))
	if len(classification.Indicators) > 0 {
		sb.WriteString(fmt.Sprintf("**Indicators:** %s\n\n", strings.Join(classification.Indicators, ", ")))
	}
	sb.WriteString("This question appears to be asking for general information or explanation.\n")
	sb.WriteString("The LLM can provide a direct response without needing to execute tools.\n\n")
	sb.WriteString("**Suggested approach:** Answer the question directly using knowledge, or use:\n")
	sb.WriteString("- `webb_tool_search` if looking for specific tools\n")
	sb.WriteString("- `webb_ask` with more specific context for data queries\n")
	return sb.String()
}

// findAgenticRoute finds the best route for agentic queries
func (r *SmartRouter) findAgenticRoute(question string) string {
	// Agentic queries map to investigation/workflow tools
	agenticRoutes := map[string][]string{
		"investigation_analysis":  {"investigate", "troubleshoot", "debug", "diagnose", "wrong", "issue", "problem"},
		"escalation_preflight":    {"escalate", "escalation", "should i"},
		"triage_full":             {"triage", "prioritize", "urgent"},
		"distributed_failure":     {"failure", "outage", "down", "crash"},
		"field_investigate":       {"field", "fde", "cse", "quick fix"},
	}

	// Find best match
	bestRoute := ""
	bestCount := 0
	for route, keywords := range agenticRoutes {
		count := 0
		for _, kw := range keywords {
			if strings.Contains(question, kw) {
				count++
			}
		}
		if count > bestCount {
			bestCount = count
			bestRoute = route
		}
	}

	if bestRoute != "" {
		return bestRoute
	}
	// Default agentic route
	return "investigation_analysis"
}

func (r *SmartRouter) findBestRoute(question string) string {
	bestMatch := ""
	bestScore := 0.0
	bestBoosted := false
	tokenOptimized := false

	for _, route := range r.routes {
		score := 0
		for _, kw := range route.Keywords {
			if strings.Contains(question, kw.Word) {
				score += kw.Weight
			}
		}

		// Apply feedback-based boost if available
		finalScore := float64(score)
		boosted := false
		if r.feedbackClient != nil && score > 0 {
			toolName := "webb_" + route.Name
			boost := r.feedbackClient.GetBoostFactor(toolName)
			finalScore = float64(score) * boost
			boosted = boost != 1.0

			// Record metrics for each candidate
			ctx := context.Background()
			otel.RecordFeedbackBoost(ctx, toolName, boost)
			otel.RecordRoutingScore(ctx, toolName, finalScore)
		}

		if finalScore > bestScore {
			bestScore = finalScore
			bestMatch = route.Name
			bestBoosted = boosted
		}
	}

	// Check if we should use a low-token alternative (v7.75)
	if bestMatch != "" && r.tokenBudgetClient != nil {
		toolName := "webb_" + bestMatch
		// Use default session ID for routing decisions
		sessionID := fmt.Sprintf("default-%s", time.Now().Format("2006-01-02-15"))
		if alt, shouldUse := r.tokenBudgetClient.ShouldUseLowTokenAlternative(sessionID, toolName); shouldUse && alt != nil {
			// Record that we switched to a low-token alternative
			ctx := context.Background()
			otel.RecordRouteDecision(ctx, alt.AlternativeTool, "token_optimized", true)
			otel.RecordTokenOptimization(ctx, toolName, alt.AlternativeTool, alt.TokenSavings)

			// Extract the route name from the alternative tool name
			altRouteName := strings.TrimPrefix(alt.AlternativeTool, "webb_")
			bestMatch = altRouteName
			tokenOptimized = true
		}
	}

	// Check for health-based routing (v7.80)
	if bestMatch != "" && r.selfHealingClient != nil {
		toolName := "webb_" + bestMatch
		// Check if the target service is unhealthy
		serviceForTool := r.getServiceForTool(toolName)
		if serviceForTool != "" {
			if shouldRoute, reason := r.selfHealingClient.ShouldRouteAway(serviceForTool); shouldRoute {
				// Try to find an alternative that doesn't use the unhealthy service
				altRoute := r.findAlternativeRoute(bestMatch, serviceForTool)
				if altRoute != "" {
					ctx := context.Background()
					otel.RecordRouteDecision(ctx, "webb_"+altRoute, "health_routed_"+reason, true)
					otel.RecordHealthRouting(ctx, toolName, "webb_"+altRoute, serviceForTool, reason)
					bestMatch = altRoute
				}
			}
		}
	}

	// Check for latency-aware routing (v7.95)
	latencyOptimized := false
	if bestMatch != "" && r.latencyTracker != nil {
		toolName := "webb_" + bestMatch
		sessionID := fmt.Sprintf("default-%s", time.Now().Format("2006-01-02-15"))
		budget := r.latencyTracker.GetSession(sessionID)

		// Check if we should use a faster alternative
		if shouldUseFast, fastAlt := r.latencyTracker.ShouldUseFastPath(toolName, budget); shouldUseFast && fastAlt != "" {
			ctx := context.Background()
			otel.RecordRouteDecision(ctx, fastAlt, "latency_optimized", true)
			otel.RecordLatencyRouting(ctx, toolName, fastAlt, budget.RemainingMs)

			// Extract the route name from the alternative tool name
			altRouteName := strings.TrimPrefix(fastAlt, "webb_")
			bestMatch = altRouteName
			latencyOptimized = true
		}
	}

	// Record the routing decision
	if bestMatch != "" && !tokenOptimized && !latencyOptimized {
		ctx := context.Background()
		reason := "keyword_match"
		if bestBoosted {
			reason = "feedback_boosted"
		}
		otel.RecordRouteDecision(ctx, "webb_"+bestMatch, reason, bestBoosted)
	}

	return bestMatch
}

// getServiceForTool returns the primary service a tool depends on
func (r *SmartRouter) getServiceForTool(toolName string) string {
	serviceMap := map[string]string{
		"webb_queue_health":           "rabbitmq",
		"webb_queue_health_full":      "rabbitmq",
		"webb_rabbitmq_overview":      "rabbitmq",
		"webb_rabbitmq_queues":        "rabbitmq",
		"webb_redis_health":           "redis",
		"webb_postgres_tables":        "postgres",
		"webb_postgres_query":         "postgres",
		"webb_db_long_queries":        "postgres",
		"webb_database_health_full":   "postgres",
		"webb_clickhouse_query":       "clickhouse",
		"webb_clickhouse_tables":      "clickhouse",
		"webb_clickhouse_describe":    "clickhouse",
	}
	return serviceMap[toolName]
}

// findAlternativeRoute finds an alternative route that doesn't depend on the unhealthy service
func (r *SmartRouter) findAlternativeRoute(currentRoute, unhealthyService string) string {
	// Define fallback routes when services are unhealthy
	fallbacks := map[string]map[string]string{
		"rabbitmq": {
			"queue_health":      "cluster_health_full",  // Use broader health check
			"queue_health_full": "cluster_health_full",
		},
		"postgres": {
			"database_health_full": "cluster_health_full",
			"db_long_queries":      "cluster_health_full",
		},
		"redis": {
			"redis_health": "cluster_health_full",
		},
		"clickhouse": {
			"clickhouse_query":  "postgres_query", // Fall back to postgres
			"clickhouse_tables": "cluster_health_full",
		},
	}

	if serviceFallbacks, ok := fallbacks[unhealthyService]; ok {
		if alt, ok := serviceFallbacks[currentRoute]; ok {
			return alt
		}
	}
	return ""
}

func (r *SmartRouter) extractContext(question string) string {
	question = strings.ToLower(question)

	// Try using CustomerClient to get cluster context from customer detection
	customerClient, err := NewCustomerClient()
	if err == nil {
		// First check if the question mentions a Slack channel
		channelPatterns := []string{
			`channel\s+#?(\S+)`,
			`in\s+#(\S+)`,
			`#([a-z0-9_-]+)`,
		}

		for _, pattern := range channelPatterns {
			re := regexp.MustCompile(pattern)
			if matches := re.FindStringSubmatch(question); len(matches) > 1 {
				if config, err := customerClient.GetConfigBySlackChannel(matches[1]); err == nil && config.Cluster != "" {
					return config.Cluster
				}
			}
		}

		// Check if question mentions a known customer, and if so return their cluster
		for _, config := range customerClient.ListConfigs() {
			configID := strings.ToLower(config.ID)
			configName := strings.ToLower(config.Name)
			displayName := strings.ToLower(config.DisplayName)

			patterns := []string{configID, configName, displayName}
			for _, p := range patterns {
				if p == "" {
					continue
				}
				re := regexp.MustCompile(`\b` + regexp.QuoteMeta(p) + `\b`)
				if re.MatchString(question) && config.Cluster != "" {
					return config.Cluster
				}
			}
		}
	}

	// Fallback: Known context patterns for common terms
	fallbackContexts := map[string][]string{
		"staging":    {"staging", "stage"},
		"production": {"production", "prod"},
	}

	for ctx, patterns := range fallbackContexts {
		for _, pattern := range patterns {
			if strings.Contains(question, pattern) {
				return ctx
			}
		}
	}

	return ""
}

func (r *SmartRouter) extractCustomer(question string) string {
	question = strings.ToLower(question)

	// Try using CustomerClient for robust detection
	customerClient, err := NewCustomerClient()
	if err == nil {
		// First check if the question mentions a Slack channel pattern
		// Look for patterns like "channel headspace_health_acme" or "in #internal-verizon"
		channelPatterns := []string{
			`channel\s+#?(\S+)`,
			`in\s+#(\S+)`,
			`#([a-z0-9_-]+)`,
		}

		for _, pattern := range channelPatterns {
			re := regexp.MustCompile(pattern)
			if matches := re.FindStringSubmatch(question); len(matches) > 1 {
				if config, err := customerClient.GetConfigBySlackChannel(matches[1]); err == nil {
					return config.ID
				}
			}
		}

		// Check if question mentions a known customer directly
		for _, config := range customerClient.ListConfigs() {
			configID := strings.ToLower(config.ID)
			configName := strings.ToLower(config.Name)
			displayName := strings.ToLower(config.DisplayName)

			// Check for direct mentions with word boundaries
			patterns := []string{
				configID,
				configName,
				displayName,
			}

			for _, p := range patterns {
				if p == "" {
					continue
				}
				// Check if pattern appears as a word (not substring of another word)
				re := regexp.MustCompile(`\b` + regexp.QuoteMeta(p) + `\b`)
				if re.MatchString(question) {
					return config.ID
				}
			}
		}
	}

	// Fallback: Try to extract from "for <customer>" pattern
	re := regexp.MustCompile(`for\s+(\w+)`)
	if matches := re.FindStringSubmatch(question); len(matches) > 1 {
		return matches[1]
	}

	return ""
}

func (r *SmartRouter) executeRoute(ctx context.Context, route string, params RouteParams) (string, error) {
	switch route {
	case "cluster_health_full":
		return r.executeClusterHealth(ctx, params)
	case "ticket_summary":
		return r.executeTicketSummary(ctx, params)
	case "queue_health":
		return r.executeQueueHealth(ctx, params)
	case "aws_infrastructure_status":
		return r.executeAWSInfrastructure(ctx, params)
	case "standup_briefing":
		return r.executeStandupBriefing(ctx, params)
	case "customer_snapshot":
		return r.executeCustomerSnapshot(ctx, params)
	case "investigation_analysis":
		return r.executeInvestigationAnalysis(ctx, params)
	case "eval_debug":
		return r.executeEvalDebug(ctx, params)
	case "escalation_preflight":
		return r.executeEscalationPreflight(ctx, params)
	case "field_investigate":
		return r.executeFieldInvestigate(ctx, params)
	default:
		return "", fmt.Errorf("unknown route: %s", route)
	}
}

func (r *SmartRouter) executeClusterHealth(ctx context.Context, params RouteParams) (string, error) {
	if params.Context == "" {
		return "", fmt.Errorf("context required for cluster health check")
	}

	client, err := NewClusterHealthClient(params.Context)
	if err != nil {
		return "", err
	}

	health, err := client.CheckHealth(ctx, ClusterHealthParams{
		Namespace:     "acme",
		IncludeQueues: true,
		IncludeAlerts: true,
	})
	if err != nil {
		return "", err
	}

	return FormatClusterHealth(health), nil
}

func (r *SmartRouter) executeTicketSummary(ctx context.Context, params RouteParams) (string, error) {
	client, err := NewTicketSummaryClient()
	if err != nil {
		return "", err
	}

	summary, err := client.GetSummary(ctx, TicketSummaryParams{
		Customer:         params.Customer,
		IncludePylon:    true,
		IncludeIncidents: true,
		IncludeShortcut: true,
	})
	if err != nil {
		return "", err
	}

	return FormatTicketSummary(summary), nil
}

func (r *SmartRouter) executeQueueHealth(ctx context.Context, params RouteParams) (string, error) {
	client, err := NewQueueHealthFullClient()
	if err != nil {
		return "", err
	}

	health, err := client.CheckHealth(ctx, QueueHealthFullParams{
		Detailed: false,
	})
	if err != nil {
		return "", err
	}

	return FormatQueueHealthFull(health), nil
}

func (r *SmartRouter) executeAWSInfrastructure(ctx context.Context, params RouteParams) (string, error) {
	client, err := NewAWSInfrastructureClient("us-west-2")
	if err != nil {
		return "", err
	}

	status, err := client.CheckStatus(ctx, AWSInfrastructureParams{
		IncludeAlarms: true,
	})
	if err != nil {
		return "", err
	}

	return FormatAWSInfrastructureStatus(status), nil
}

func (r *SmartRouter) executeStandupBriefing(ctx context.Context, params RouteParams) (string, error) {
	// Standup briefing is available via webb_standup_briefing MCP tool directly
	return "", fmt.Errorf("standup_briefing: use webb_standup_briefing MCP tool directly")
}

func (r *SmartRouter) executeCustomerSnapshot(ctx context.Context, params RouteParams) (string, error) {
	if params.Customer == "" {
		return "", fmt.Errorf("customer name required for snapshot")
	}
	// Customer snapshot is available via webb_customer_snapshot MCP tool directly
	return "", fmt.Errorf("customer_snapshot: use webb_customer_snapshot MCP tool with customer='%s'", params.Customer)
}

func (r *SmartRouter) executeInvestigationAnalysis(ctx context.Context, params RouteParams) (string, error) {
	// Investigation analysis is available via webb_investigation_analysis MCP tool directly
	return "", fmt.Errorf("investigation_analysis: use webb_investigation_analysis MCP tool with customer='%s' cluster='%s'", params.Customer, params.Context)
}

func (r *SmartRouter) executeEvalDebug(ctx context.Context, params RouteParams) (string, error) {
	if params.Context == "" {
		return "", fmt.Errorf("context required for evaluation debugging")
	}
	// Eval debug is available via webb_eval_debug MCP tool directly
	return "", fmt.Errorf("eval_debug: use webb_eval_debug MCP tool with context='%s'", params.Context)
}

func (r *SmartRouter) executeEscalationPreflight(ctx context.Context, params RouteParams) (string, error) {
	if params.Context == "" {
		return "", fmt.Errorf("context required for escalation preflight")
	}
	if params.Customer == "" {
		return "", fmt.Errorf("customer required for escalation preflight")
	}
	// Escalation preflight available via webb_escalation_preflight MCP tool
	return "", fmt.Errorf("escalation_preflight: use webb_escalation_preflight MCP tool with context='%s' customer='%s'", params.Context, params.Customer)
}

func (r *SmartRouter) executeFieldInvestigate(ctx context.Context, params RouteParams) (string, error) {
	if params.Context == "" {
		return "", fmt.Errorf("context required for field investigation")
	}
	// Field investigate available via webb_field_investigate MCP tool
	return "", fmt.Errorf("field_investigate: use webb_field_investigate MCP tool with context='%s' customer='%s'", params.Context, params.Customer)
}

// FormatSmartRouterResult formats the routing result for display
func FormatSmartRouterResult(result *SmartRouterResult) string {
	var sb strings.Builder

	// v106.0 STRIDE: Show classification first
	if result.Classification != nil {
		sb.WriteString(fmt.Sprintf("**Query Modality:** %s (%.0f%% confidence)\n",
			result.Classification.Modality, result.Classification.Confidence*100))
		if len(result.Classification.Indicators) > 0 {
			sb.WriteString(fmt.Sprintf("**Indicators:** %s\n", strings.Join(result.Classification.Indicators, ", ")))
		}
		sb.WriteString("\n")
	}

	if result.ToolName != "" {
		sb.WriteString(fmt.Sprintf("**Routed to:** `%s`\n\n", result.ToolName))
	} else {
		sb.WriteString(fmt.Sprintf("**Routed to:** %s\n\n", result.RoutedTo))
	}

	if result.ExtractedContext != "" {
		sb.WriteString(fmt.Sprintf("**Detected Context:** %s\n", result.ExtractedContext))
	}
	if result.ExtractedCustomer != "" {
		sb.WriteString(fmt.Sprintf("**Detected Customer:** %s\n", result.ExtractedCustomer))
	}

	if result.Error != "" {
		sb.WriteString(fmt.Sprintf("\n**Error:** %s\n", result.Error))
		return sb.String()
	}

	sb.WriteString("\n---\n\n")
	sb.WriteString(result.Output)

	return sb.String()
}
