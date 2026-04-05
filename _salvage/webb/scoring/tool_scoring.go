// Package clients provides the unified MCP tool scoring system.
package clients

import (
	"regexp"
	"strings"
	"time"
)

// ToolScore represents the comprehensive score for a single MCP tool.
type ToolScore struct {
	ToolName     string    `json:"tool_name"`
	Category     string    `json:"category"`
	ScoredAt     time.Time `json:"scored_at"`

	// Component Scores (0-100 each)
	ComplianceScore    float64 `json:"compliance_score"`     // MCP spec compliance
	BestPracticesScore float64 `json:"best_practices_score"` // Tool audit score
	DescriptionScore   float64 `json:"description_score"`    // Description quality
	ParameterScore     float64 `json:"parameter_score"`      // Parameter quality
	ComplexityScore    float64 `json:"complexity_score"`     // Cost/complexity rating
	SuccessRateScore   float64 `json:"success_rate_score"`   // Historical success rate
	CachingScore       float64 `json:"caching_score"`        // v37.0: TTL cache, sync.Once usage
	ErrorHandlingScore float64 `json:"error_handling_score"` // v37.0: retry, graceful degradation

	// Detailed Breakdowns
	DescriptionDetails   DescriptionQuality   `json:"description_details"`
	ParameterDetails     ParameterQuality     `json:"parameter_details"`
	CachingDetails       CachingQuality       `json:"caching_details,omitempty"`
	ErrorHandlingDetails ErrorHandlingQuality `json:"error_handling_details,omitempty"`

	// Aggregate
	OverallScore float64         `json:"overall_score"`
	Grade        string          `json:"grade"` // A, B, C, D, F
	Issues       []ScoringIssue  `json:"issues"`
	Suggestions  []string        `json:"suggestions"`
}

// DescriptionQuality breaks down the description scoring components.
type DescriptionQuality struct {
	Score          float64 `json:"score"`
	StartsWithVerb bool    `json:"starts_with_verb"`
	VerbUsed       string  `json:"verb_used,omitempty"`
	Length         int     `json:"length"`
	LengthScore    float64 `json:"length_score"`
	HasReturnHint  bool    `json:"has_return_hint"`
	ClarityScore   float64 `json:"clarity_score"`
	HasUseCases    bool    `json:"has_use_cases"`
}

// ParameterQuality breaks down the parameter scoring components.
type ParameterQuality struct {
	Score              float64  `json:"score"`
	TotalParams        int      `json:"total_params"`
	RequiredParams     int      `json:"required_params"`
	ParamsWithDefaults int      `json:"params_with_defaults"`
	ParamsWithDescs    int      `json:"params_with_descriptions"`
	UsesStandardNames  bool     `json:"uses_standard_names"`
	StandardParams     []string `json:"standard_params,omitempty"`
	MissingDescs       []string `json:"missing_descriptions,omitempty"`
}

// CachingQuality breaks down the caching scoring components (v37.0).
type CachingQuality struct {
	Score            float64 `json:"score"`
	HasTTLCache      bool    `json:"has_ttl_cache"`       // Uses TTL-based caching
	UsesSyncOnce     bool    `json:"uses_sync_once"`      // Uses sync.Once for initialization
	HasCacheControl  bool    `json:"has_cache_control"`   // Has cache invalidation logic
	CacheableResults bool    `json:"cacheable_results"`   // Results are suitable for caching
	TTLSeconds       int     `json:"ttl_seconds,omitempty"` // Detected TTL if applicable
}

// ErrorHandlingQuality breaks down the error handling scoring components (v37.0).
type ErrorHandlingQuality struct {
	Score               float64 `json:"score"`
	HasRetry            bool    `json:"has_retry"`             // Has retry logic
	HasGracefulDegrade  bool    `json:"has_graceful_degrade"`  // Falls back gracefully on error
	HasTimeoutHandling  bool    `json:"has_timeout_handling"`  // Handles timeouts
	HasContextAwareness bool    `json:"has_context_awareness"` // Respects context cancellation
	ReturnsPartialData  bool    `json:"returns_partial_data"`  // Returns partial results on failure
}

// ScoringIssue represents a scoring issue found in a tool.
type ScoringIssue struct {
	Code        string `json:"code"`
	Severity    string `json:"severity"` // error, warning, info
	Message     string `json:"message"`
	Category    string `json:"category"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// ToolScoreReport is the aggregate report for all tools.
type ToolScoreReport struct {
	GeneratedAt       time.Time              `json:"generated_at"`
	TotalTools        int                    `json:"total_tools"`
	AverageScore      float64                `json:"average_score"`
	GradeDistribution map[string]int         `json:"grade_distribution"`
	ByCategory        map[string]CategoryStats `json:"by_category"`
	TopIssues         []IssueCount           `json:"top_issues"`
	LowestScoring     []ToolScore            `json:"lowest_scoring"`
	HighestScoring    []ToolScore            `json:"highest_scoring"`
	Comparison        *BenchmarkComparison   `json:"comparison,omitempty"`
}

// CategoryStats shows scoring statistics for a category.
type CategoryStats struct {
	Category     string  `json:"category"`
	ToolCount    int     `json:"tool_count"`
	AverageScore float64 `json:"average_score"`
	Grade        string  `json:"grade"`
	TopIssue     string  `json:"top_issue,omitempty"`
}

// IssueCount tracks issue frequency across tools.
type IssueCount struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Count    int    `json:"count"`
	Category string `json:"category"`
}

// BenchmarkComparison compares webb scores to industry benchmarks.
type BenchmarkComparison struct {
	BenchmarkName    string                      `json:"benchmark_name"`
	WebbMetrics      map[string]float64          `json:"webb_metrics"`
	BenchmarkMetrics map[string]float64          `json:"benchmark_metrics"`
	Comparisons      []BenchmarkMetricComparison `json:"comparisons"`
	OverallStatus    string                      `json:"overall_status"` // above, at, below
}

// BenchmarkMetricComparison shows how webb compares on a single metric.
type BenchmarkMetricComparison struct {
	Metric         string  `json:"metric"`
	WebbValue      float64 `json:"webb_value"`
	BenchmarkValue float64 `json:"benchmark_value"`
	Difference     float64 `json:"difference"`
	Status         string  `json:"status"` // above, at, below
}

// IndustryBenchmarks holds benchmark data from various sources.
type IndustryBenchmarks struct {
	MCPTef    MCPTefBenchmark    `json:"mcp_tef"`
	GitHub    GitHubBenchmark    `json:"github"`
	MCPBench  MCPBenchBenchmark  `json:"mcp_bench"`
	UpdatedAt time.Time          `json:"updated_at"`
}

// MCPTefBenchmark holds mcp-tef style benchmarks.
type MCPTefBenchmark struct {
	DescriptionQuality struct {
		AvgLength       int     `json:"avg_length"`
		VerbStartRate   float64 `json:"verb_start_rate"`
		ReturnHintRate  float64 `json:"return_hint_rate"`
	} `json:"description_quality"`
	ParameterQuality struct {
		DefaultRate     float64 `json:"default_rate"`
		DescriptionRate float64 `json:"description_rate"`
	} `json:"parameter_quality"`
	SelectionAccuracy struct {
		Precision float64 `json:"precision"`
		Recall    float64 `json:"recall"`
		F1        float64 `json:"f1"`
	} `json:"selection_accuracy"`
}

// GitHubBenchmark holds GitHub-style benchmarks.
type GitHubBenchmark struct {
	HallucinationRate float64 `json:"hallucination_rate"`
	ArgumentCoverage  float64 `json:"argument_coverage"`
}

// MCPBenchBenchmark holds MCP-Bench style benchmarks.
type MCPBenchBenchmark struct {
	RuleBasedScore   float64 `json:"rule_based_score"`
	RubricScore      float64 `json:"rubric_score"`
	ComplexityTiers  map[string]float64 `json:"complexity_tiers"`
}

// ScoreWeights defines the weight for each scoring component.
type ScoreWeights struct {
	Compliance    float64 `json:"compliance"`
	BestPractices float64 `json:"best_practices"`
	Description   float64 `json:"description"`
	Parameters    float64 `json:"parameters"`
	Complexity    float64 `json:"complexity"`
	SuccessRate   float64 `json:"success_rate"`
	Caching       float64 `json:"caching"`       // v37.0: TTL cache, sync.Once usage
	ErrorHandling float64 `json:"error_handling"` // v37.0: retry, graceful degradation
}

// DefaultScoreWeights returns the standard scoring weights.
// v37.0: Added Caching (10%) and ErrorHandling (5%), adjusted others proportionally
func DefaultScoreWeights() ScoreWeights {
	return ScoreWeights{
		Compliance:    0.20, // Reduced from 0.25
		BestPractices: 0.20, // Reduced from 0.25
		Description:   0.15, // Reduced from 0.20
		Parameters:    0.15, // Same
		Complexity:    0.10, // Same
		SuccessRate:   0.05, // Same
		Caching:       0.10, // NEW: TTL cache, sync.Once
		ErrorHandling: 0.05, // NEW: retry, graceful degradation
	}
}

// CalculateOverallScore computes the weighted overall score.
func (ts *ToolScore) CalculateOverallScore(weights ScoreWeights) float64 {
	score := ts.ComplianceScore*weights.Compliance +
		ts.BestPracticesScore*weights.BestPractices +
		ts.DescriptionScore*weights.Description +
		ts.ParameterScore*weights.Parameters +
		ts.ComplexityScore*weights.Complexity +
		ts.SuccessRateScore*weights.SuccessRate +
		ts.CachingScore*weights.Caching +
		ts.ErrorHandlingScore*weights.ErrorHandling

	ts.OverallScore = score
	ts.Grade = scoreToGrade(score)
	return score
}

// scoreToGrade converts a numeric score to a letter grade.
func scoreToGrade(score float64) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 70:
		return "C"
	case score >= 60:
		return "D"
	default:
		return "F"
	}
}

// Action verbs that should start tool descriptions.
var actionVerbs = []string{
	// Query/Read operations
	"get", "list", "search", "find", "query", "fetch", "retrieve", "load", "read",
	"show", "display", "view", "browse", "explore", "discover", "lookup",
	// Analysis operations
	"check", "analyze", "audit", "validate", "verify", "inspect", "examine",
	"diagnose", "debug", "investigate", "assess", "evaluate", "measure", "test",
	"profile", "benchmark", "scan", "lint", "detect", "identify", "determine",
	"calculate", "compute", "estimate", "predict", "forecast", "project",
	// CRUD operations
	"create", "add", "update", "delete", "remove", "insert", "modify", "edit",
	"set", "put", "patch", "replace", "write", "save", "store",
	// Lifecycle operations
	"start", "stop", "restart", "pause", "resume", "enable", "disable", "toggle",
	"open", "close", "clear", "reset", "initialize", "configure", "setup",
	// State changes
	"approve", "reject", "accept", "decline", "confirm", "cancel",
	"resolve", "ignore", "acknowledge", "escalate", "assign", "unassign",
	"archive", "unarchive", "lock", "unlock", "freeze", "unfreeze",
	// Monitoring operations
	"monitor", "track", "watch", "observe", "record", "capture", "log",
	// Deployment operations
	"deploy", "rollout", "rollback", "scale", "provision", "deprovision",
	// Communication operations
	"send", "post", "publish", "notify", "broadcast", "emit", "dispatch",
	// Data operations
	"export", "import", "backup", "restore", "recover", "migrate", "transfer",
	"sync", "refresh", "reload", "cache", "flush", "purge",
	// Transform operations
	"generate", "build", "compile", "render", "format", "parse", "convert",
	"transform", "translate", "encode", "decode", "compress", "decompress",
	// Comparison operations
	"compare", "diff", "merge", "correlate", "match", "map",
	// Connection operations
	"connect", "disconnect", "link", "unlink", "bind", "unbind",
	// Execution operations
	"run", "execute", "trigger", "invoke", "call", "perform", "apply",
	// Access operations
	"access", "authenticate", "authorize", "grant", "revoke",
	// Management operations
	"manage", "handle", "process", "orchestrate", "coordinate",
	// Additional action verbs
	"attempt", "abort", "consolidate", "install", "uninstall", "mine", "advance",
	"extract", "warm", "schedule", "register", "define", "aggregate", "summarize",
	"learn", "teach", "train", "infer", "suggest", "recommend", "propose",
	"rotate", "renew", "expire", "invalidate", "revoke", "enqueue", "dequeue",
	"bridge", "route", "forward", "redirect", "proxy", "intercept", "inject",
	"simulate", "emulate", "mock", "stub", "fake", "scaffold", "bootstrap",
	"index", "reindex", "tag", "untag", "label", "categorize", "classify",
	"correlate", "deduplicate", "normalize", "sanitize", "redact", "mask",
	"clone", "fork", "snapshot", "checkpoint", "pin", "unpin", "mark",
	"accept", "acknowledge", "append", "prepend",
	// Description common verbs
	"describe", "embed", "expand", "force", "score", "ping", "retry",
	"download", "upload", "lookup", "plan", "rebalance", "redeploy",
	"reconnect", "federate", "preview", "combine", "group", "look",
	// v37 additions
	"trace", "print", "kill", "include", "wait", "collect", "complete",
}

// Standard parameter names used across tools.
var standardParamNames = map[string]bool{
	"context":      true,
	"namespace":    true,
	"cluster":      true,
	"customer":     true,
	"limit":        true,
	"time_range":   true,
	"days":         true,
	"hours":        true,
	"format":       true,
	"detailed":     true,
	"include":      true,
	"exclude":      true,
	"filter":       true,
	"query":        true,
	"name":         true,
	"id":           true,
	"channel":      true,
	"channel_id":   true,
	"save":         true,
	"save_to_vault": true,
}

// Return hint keywords that indicate output format.
var returnHintKeywords = []string{
	// Explicit return indicators
	"returns", "return", "outputs", "output", "produces", "yields", "emits",
	"provides", "shows", "displays", "generates", "creates", "gives",
	// Data structure hints
	"score", "count", "list", "array", "map", "object", "struct",
	"json", "markdown", "table", "yaml", "csv", "html",
	// Value type hints
	"0-100", "percentage", "boolean", "true/false", "yes/no",
	"number", "string", "integer", "float", "timestamp", "duration",
	// Result indicators
	"result", "results", "response", "status", "state", "summary",
	"report", "analysis", "findings", "metrics", "statistics", "stats",
	// Collection hints
	"items", "entries", "records", "rows", "documents", "files",
	"pods", "deployments", "services", "nodes", "clusters",
	"tickets", "issues", "incidents", "alerts", "events",
	// Format hints
	"formatted", "structured", "detailed", "brief", "compact",
	"sorted", "grouped", "filtered", "aggregated", "ranked",
	// Additional common patterns
	"health", "healthy", "warning", "critical", "error", "success", "failed",
	"configuration", "config", "settings", "options", "parameters",
	"history", "timeline", "changelog", "log", "logs", "trace", "traces",
	"queue", "queues", "message", "messages", "job", "jobs", "task", "tasks",
	"connection", "connections", "endpoint", "endpoints", "url", "urls",
	"user", "users", "member", "members", "team", "teams",
	"customer", "customers", "account", "accounts",
	"version", "versions", "release", "releases",
	"chart", "graph", "diagram", "visualization",
	"recommendation", "recommendations", "suggestion", "suggestions",
	"priority", "priorities", "severity", "severities",
	"breakdown", "distribution", "trend", "trends", "pattern", "patterns",
	// Content hints
	"content", "contents", "data", "information", "info", "details", "detail",
	"text", "body", "description", "metadata", "meta",
	"confirmation", "confirmed", "acknowledged", "updated", "created", "deleted",
	"id", "ids", "name", "names", "path", "paths", "link", "links",
	"preview", "thumbnail", "image", "images", "attachment", "attachments",
	"token", "tokens", "key", "keys", "secret", "secrets", "credential",
	"budget", "budgets", "cost", "costs", "price", "prices", "expense",
	"runbook", "runbooks", "playbook", "playbooks", "workflow", "workflows",
	"permission", "permissions", "role", "roles", "access",
	"latency", "throughput", "bandwidth", "capacity", "utilization",
	// Action result keywords (implies returns confirmation/result)
	"added", "executed", "recovered", "upgraded", "removed", "enabled", "disabled",
	"saved", "started", "stopped", "cancelled", "toggled", "connected", "disconnected",
	// Domain-specific object keywords
	"retrospective", "retro", "item", "command", "commands", "bash", "python", "script",
	"helm", "chart", "release", "upgrade", "channel", "channels", "meeting", "notes",
	"proposal", "proposals", "chain", "template", "templates", "event", "blocks", "block",
	"worker", "workers", "swarm", "model", "comparison", "webhook", "pipeline", "schedule",
	"bookmark", "bookmarks", "comment", "comments", "tag", "tags", "site", "sites",
	"feature", "features", "scaffold", "environment", "environments", "sandboxed",
	// Query/database keywords
	"query", "queries", "prometheus", "postgres", "clickhouse", "sql",
	// Service/integration keywords
	"pylon", "shortcut", "obsidian", "vault", "mcp", "remote", "source",
	"lambda", "s3", "asg", "autoscaling", "lifecycle",
	// Action keywords
	"merge", "merged", "restart", "restarted", "pause", "paused", "warm", "warmed",
	"accept", "accepted", "refresh", "refreshed", "batch", "batched",
	// Testing/monitoring keywords
	"chaos", "experiment", "e2e", "api", "test", "tests", "decision",
	// Insight/analysis keywords
	"mention", "mentions", "star", "stars", "insight", "insights",
	"intent", "detection", "resolution", "resolutions", "review",
	// Structure keywords
	"tree", "hierarchy", "schema", "schemas", "presence", "heartbeat",
	"handoff", "scraper", "agent",
	// Final catch-all keywords
	"rate", "rates", "limit", "limits", "exhausted", "predict", "predicted",
	"boilerplate", "skeleton", "ready-to-use", "go code",
	"alternative", "alternatives", "faster", "slow", "slower",
	"engine", "gracefully", "perpetual",
}

// CheckStartsWithVerb checks if description starts with an action verb.
func CheckStartsWithVerb(desc string) (bool, string) {
	if desc == "" {
		return false, ""
	}

	lowerDesc := strings.ToLower(strings.TrimSpace(desc))
	for _, verb := range actionVerbs {
		if strings.HasPrefix(lowerDesc, verb+" ") || strings.HasPrefix(lowerDesc, verb+"s ") {
			return true, verb
		}
	}
	return false, ""
}

// CheckHasReturnHint checks if description mentions return type/format.
func CheckHasReturnHint(desc string) bool {
	lowerDesc := strings.ToLower(desc)
	for _, hint := range returnHintKeywords {
		if strings.Contains(lowerDesc, hint) {
			return true
		}
	}
	return false
}

// ScoreDescriptionLength scores the description length (optimal: 50-200 chars).
func ScoreDescriptionLength(length int) float64 {
	if length == 0 {
		return 0
	}
	if length < 20 {
		return 25 // Too short
	}
	if length < 50 {
		return 50 // Short but acceptable
	}
	if length <= 200 {
		return 100 // Optimal range
	}
	if length <= 300 {
		return 75 // Slightly verbose
	}
	return 50 // Too long
}

// ScoreClarity scores description clarity based on heuristics.
func ScoreClarity(desc string) float64 {
	if desc == "" {
		return 0
	}

	score := 50.0 // Base score

	// Check for use case indicators
	useCasePatterns := []string{
		" for ", " to ", " by ", " when ", " with ",
		"useful for", "helps with", "enables",
		"e.g.", "example", "such as",
	}
	for _, pattern := range useCasePatterns {
		if strings.Contains(strings.ToLower(desc), pattern) {
			score += 10
			break
		}
	}

	// Check for specific nouns (indicates clarity)
	specificNouns := regexp.MustCompile(`\b(pod|deployment|service|cluster|queue|alert|ticket|customer|channel)\b`)
	if specificNouns.MatchString(strings.ToLower(desc)) {
		score += 15
	}

	// Penalize vague descriptions
	vaguePatterns := []string{
		"does stuff", "handles things", "various",
		"etc", "and more", "...",
	}
	for _, pattern := range vaguePatterns {
		if strings.Contains(strings.ToLower(desc), pattern) {
			score -= 15
			break
		}
	}

	// Cap at 100
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	return score
}

// IsStandardParamName checks if a parameter name follows conventions.
func IsStandardParamName(name string) bool {
	return standardParamNames[strings.ToLower(name)]
}
