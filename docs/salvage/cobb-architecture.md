# Cobb MCP Server Architecture Reference

Salvaged from the `cobb` repository before deletion. This document captures all
reusable patterns and architecture decisions from a production MCP server with
1,790 tools across 70 modules, an autonomous research swarm with 23+ worker
types, and a perpetual self-improvement engine.

**Version at time of capture:** v135.0
**Language:** Go 1.26.1
**MCP SDK:** github.com/mark3labs/mcp-go

---

## Table of Contents

1. [Module/Registry System](#1-moduleregistry-system)
2. [Lazy Tool Loading and Token Efficiency](#2-lazy-tool-loading-and-token-efficiency)
3. [Consolidated Mega-Tools](#3-consolidated-mega-tools)
4. [Swarm Architecture](#4-swarm-architecture)
5. [Perpetual Improvement Engine](#5-perpetual-improvement-engine)
6. [Session Memory](#6-session-memory)
7. [Tool Discovery Contract](#7-tool-discovery-contract)
8. [Chain/Template System](#8-chainttemplate-system)
9. [OpenTelemetry Observability](#9-opentelemetry-observability)
10. [Safety Scoring and Risk Assessment](#10-safety-scoring-and-risk-assessment)
11. [OAuth and Multi-User RBAC](#11-oauth-and-multi-user-rbac)
12. [Kubernetes/Helm Deployment](#12-kuberneteshelm-deployment)
13. [Middleware Stack](#13-middleware-stack)
14. [Async Task System](#14-async-task-system)
15. [Federation (Remote MCP Tool Proxying)](#15-federation)
16. [Key Design Decisions](#16-key-design-decisions)

---

## 1. Module/Registry System

### Architecture

Cobb organizes 1,790 tools into 70 modules using a `ToolModule` interface and a
central `ToolRegistry` singleton. Each module is a Go package under
`internal/mcp/tools/<name>/` that self-registers via `init()`.

### ToolModule Interface

```go
type ToolModule interface {
    Name() string              // e.g., "kubernetes", "aws"
    Description() string       // Brief description
    Tools() []ToolDefinition   // All tool definitions
}
```

### ToolDefinition

The core type carries tool metadata, handler, safety scoring, caching hints,
and structured output schemas:

```go
type ToolDefinition struct {
    Tool         mcp.Tool           // MCP protocol tool definition
    Handler      ToolHandlerFunc    // func(ctx, request) (*CallToolResult, error)
    Category     string             // "kubernetes", "aws", "slack", etc.
    Subcategory  string             // Finer grouping within category
    Tags         []string           // Searchable keywords
    UseCases     []string           // Common use cases
    Complexity   ToolComplexity     // simple, moderate, complex
    IsWrite      bool               // Modifies state?
    RiskLevel    string             // low, medium, high, critical
    RequiresConfirmation bool       // Needs confirm_write=true
    RequiresAuth bool               // Auth required
    MinRole      string             // Minimum RBAC role
    ThinkingBudget int              // Extended thinking tokens (0 = none)
    OutputSchema *OutputSchema      // Structured output JSON schema

    // Cache metadata (v107.0)
    CacheHint       CacheHintType   // "ephemeral" or "persistent"
    CachePriority   int             // 10=low, 50=medium, 100=high
    EstimatedTokens int             // Token cost estimate

    // Safety scoring (v128.0)
    SafetyScore      int            // 0-100 (higher = safer)
    SafetyGrade      string         // A-F letter grade
    MutabilityLevel  string         // read, create, update, delete, destroy
    ScopeLevel       string         // single, namespace, cluster, multi
    SensitivityLevel string         // none, config, secrets, credentials
    IsReversible     bool
    IsIdempotent     bool

    // Deprecation (v38.0)
    Deprecated       bool
    DeprecatedReason string
    Successor        string
}
```

### Registration Flow

Modules self-register in `init()`:

```go
// internal/mcp/tools/aws/module.go
func init() {
    tools.GetRegistry().RegisterModule(&Module{})
}
```

The registry applies MCP annotations automatically during registration:

```go
func (r *ToolRegistry) RegisterModule(module ToolModule) {
    r.modules[module.Name()] = module
    for _, tool := range module.Tools() {
        applyMCPAnnotations(&tool)  // title, readOnly, destructive hints
        r.tools[tool.Tool.Name] = tool
    }
}
```

Side-effect imports in `internal/mcp/tools.go` trigger all 70 module `init()`
functions:

```go
import (
    _ "github.com/hairglasses-studio/cobb/internal/mcp/tools/aws"
    _ "github.com/hairglasses-studio/cobb/internal/mcp/tools/kubernetes"
    _ "github.com/hairglasses-studio/cobb/internal/mcp/tools/slack"
    // ... 67 more module imports
)
```

### Module List (70 modules across 64 directories)

| Category | Modules |
|----------|---------|
| **Cloud** | aws, azure, costs |
| **Kubernetes** | kubernetes, deployment, health |
| **Observability** | observability, monitoring, prometheus, sentry, grafana (via observability) |
| **Tickets** | tickets, investigation, incident_thread |
| **Communication** | slack, comms, collab, gmail, calendar |
| **Knowledge** | knowledge, obsidian, semantic, memory, research, web |
| **DevOps** | devops, devtools, terraform, execution, github |
| **Data** | database, storage, minio, gsheets, gdrive |
| **AI/Automation** | claude, swarm, workers, orchestration, perpetual, improvement |
| **Platform** | consolidated, operations, ops, remediation, chains, e2e |
| **Identity** | identity, customers, gworkspace, security, session |
| **Meta** | meta, discovery, diagrams, graph, results, kpi, presentations, retro |
| **Integrations** | integrations, realtime, acme, validation |

---

## 2. Lazy Tool Loading and Token Efficiency

### The Problem

1,790 tools with full schemas = ~766K tokens. Claude Code's context window
cannot afford this.

### The Solution

Lazy loading (default, `WEBB_LAZY_TOOLS=true`) registers all 1,790 tools but
only gives full schemas to ~30 high-priority tools. The rest get minimal
schemas (name + 80-char description + empty params).

```go
func (r *ToolRegistry) RegisterDiscoveryOnlyWithServer(s *server.MCPServer) {
    // Full schemas for discovery + top-10 most-used tools
    fullSchemaTools := map[string]bool{
        "webb_tool_discover":       true,
        "webb_tool_schema":         true,
        "webb_tool_search":         true,
        "webb_ask":                 true,
        "webb_cluster_health_full": true,
        "webb_k8s_pods":            true,
        "webb_k8s_logs":            true,
        "webb_slack_search":        true,
        "webb_pylon_list":          true,
        "webb_ticket_summary":      true,
        // ... ~30 total
    }

    for _, tool := range r.tools {
        if fullSchemaTools[tool.Tool.Name] {
            s.AddTool(tool.Tool, handler)  // Full schema
        } else {
            minimalTool := mcp.Tool{
                Name:        tool.Tool.Name,
                Description: truncateDescription(tool.Tool.Description, 80),
                InputSchema: mcp.ToolInputSchema{
                    Type:       "object",
                    Properties: map[string]interface{}{},  // Empty
                },
            }
            s.AddTool(minimalTool, handler)  // Minimal schema
        }
    }
}
```

**Result:** ~2K tokens initially vs ~766K. That is a ~99.7% reduction.

### On-Demand Schema Loading

When Claude needs a tool's full schema, it calls `webb_tool_schema`:

```
webb_tool_schema(tool_names="webb_k8s_pod_diagnostic,webb_redis_health")
```

This returns the complete `InputSchema` with all parameters, descriptions, and
required fields for just those tools.

---

## 3. Consolidated Mega-Tools

149 "mega-tools" aggregate multiple data sources into a single call, reducing
both token usage (~65% savings) and round trips.

### Pattern

```go
// Instead of: webb_k8s_pods + webb_grafana_alerts + webb_queue_health + ...
// Use: webb_cluster_health_full

{
    Tool: mcp.NewTool("webb_cluster_health_full",
        mcp.WithDescription("K8s + queues + alerts in one call"),
        common.ParamContext(),
        mcp.WithBoolean("include_queues", ...),
        mcp.WithBoolean("include_alerts", ...),
    ),
    Handler:  handleClusterHealthFull,
    Category: "health",
}
```

### Key Consolidated Tools

| Tool | Replaces | Savings |
|------|----------|---------|
| `webb_cluster_health_full` | k8s pods + deployments + events + queues + alerts | 5 calls -> 1 |
| `webb_ticket_summary` | pylon + incidents + shortcut | 3 calls -> 1 |
| `webb_database_health_full` | postgres + clickhouse + connections | 3 calls -> 1 |
| `webb_oncall_dashboard` | 8+ separate tool calls | 8 calls -> 1 |
| `webb_distributed_failure_diagnosis` | k8s events + grafana + queues + DB | 70% token savings |
| `webb_morning_briefing_full` | calendar + gmail + slack + tickets | 4+ calls -> 1 |
| `webb_ask` | Smart router, auto-detects intent | N calls -> 1 |

### webb_ask Smart Router

Natural language question routing. Extracts context (cluster name, customer) from
the question and dispatches to the right tool:

```go
func handleSmartAsk(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    question := req.Params.Arguments["question"].(string)
    // Extracts "headspace" -> routes to cluster health
    // Extracts "verizon tickets" -> routes to ticket_summary(customer="verizon")
}
```

---

## 4. Swarm Architecture

### Overview

An autonomous research swarm with 23+ specialized worker types that
continuously analyze the codebase, integrations, and operational data. Designed
for long-running (24h) autonomous sessions on EKS.

### SwarmOrchestrator

```go
type SwarmOrchestrator struct {
    id         string
    config     *SwarmConfig
    state      SwarmState       // initializing, running, paused, stopping, completed, failed
    metrics    *SwarmMetrics
    workers    map[string]*SwarmWorker
    findings   []*SwarmResearchFinding
    recovery   *SwarmRecoveryManager

    findingsCh    chan *SwarmResearchFinding
    checkpointCh  chan struct{}
    perpetualEngine *PerpetualEngine  // Bi-directional feedback

    // Quality gate for findings validation (v31.0)
    qualityGate *SwarmQualityGate
    // Finding aggregation layer (v32.5)
    aggregator  *FindingAggregator
}
```

### Default Config

```go
SwarmConfig{
    Duration:           24 * time.Hour,
    CheckpointInterval: 5 * time.Minute,
    MergeInterval:      30 * time.Minute,
    MaxTokensTotal:     5_000_000,        // 5M tokens
    MaxTokensPerWorker: 500_000,          // 500K per worker
}
```

### Worker Types (23+)

Each worker has a typed handler function:

```go
type SwarmWorker struct {
    id           string
    workerType   SwarmWorkerType
    tokenBudget  int64
    tokensUsed   int64
    handler      func(context.Context, *SwarmWorker) error
}
```

| Category | Workers |
|----------|---------|
| **Core Auditors** | tool_auditor, security_auditor, performance_profiler |
| **Analysis** | code_quality, dependency_audit, test_coverage, documentation, runbook_generator |
| **MCP Integration** | knowledge_graph, consensus_validator, cross_reference, feature_discovery |
| **Intelligence** | pattern_discovery, improvement_audit, semantic_intel, predictive, compliance_audit, meta_intel |
| **External Data** | github_issues, sentry_patterns |
| **Code Quality** | linter |
| **Scrapers (15)** | scraper_pylon, scraper_shortcut, scraper_slack, scraper_github, scraper_incidentio, scraper_confluence, scraper_sentry, scraper_grafana, scraper_postgres, scraper_clickhouse, scraper_gmail, scraper_gdrive, scraper_aws, scraper_uptimerobot, scraper_rabbitmq |

### Research Findings

```go
type SwarmResearchFinding struct {
    ID          string
    WorkerID    string
    WorkerType  SwarmWorkerType
    Category    string
    Title       string
    Description string
    Evidence    []string
    Confidence  int             // 0-100
    Impact      int             // 0-100
    Effort      string          // small, medium, large

    // Lifecycle
    FindingStatus FindingStatus // pending, queued, actioned, merged, rejected, skipped
    ProposalID    string        // Links to perpetual engine proposal

    // Quality (v31.0)
    QualityScore  int
    QualityIssues []string
}
```

### Feedback Loop

```
Swarm Workers  -->  Findings  -->  Quality Gate  -->  Aggregator
                                                          |
                                                          v
                                                  Perpetual Engine
                                                          |
                                                     Proposals  -->  PRs
                                                          |
                                                      Outcomes
                                                          |
                                                          v
                                                   Worker Budget Adjustment
                                                   (1.5x high performers,
                                                    0.5x underperformers)
```

### Worker Efficiency Tracking

```go
type WorkerEfficiencyStats struct {
    WorkerType       SwarmWorkerType
    FindingsTotal    int
    FindingsAccepted int
    FindingsRejected int
    AcceptanceRate   float64
    TokensUsed       int64
    TokensPerFinding float64
    AvgConfidence    float64
}
```

### Category Saturation

Workers avoid diminishing returns by tracking category saturation:

```go
type CategorySaturationStats struct {
    Category       string
    FindingsTotal  int
    UniquePatterns int
    NewRatio       float64   // % genuinely new
    IsSaturated    bool      // >10 findings, <20% new
}
```

---

## 5. Perpetual Improvement Engine

### Overview

A daemon that autonomously discovers improvements, prioritizes them, implements
changes, and learns from outcomes. Runs in a continuous 4-phase cycle.

### Phases

```
Discovery --> Prioritization --> Implementation --> Learning
    ^                                                  |
    +--------------------------------------------------+
```

1. **Discovery** (every 6h): Scans multiple sources for improvement opportunities
2. **Prioritization**: Scores proposals using weighted source factors
3. **Implementation**: Creates dev tasks, generates PRs
4. **Learning** (every 24h): Adjusts source weights based on PR outcomes

### Configuration

```go
PerpetualConfig{
    DiscoveryCycleInterval: 6 * time.Hour,
    LearningCycleInterval:  24 * time.Hour,
    MaxConcurrentTasks:     10,
    MaxDailyPRs:            100,
    LearningRate:           0.1,
    MaxQueueDepth:          100,
    DeduplicationWindow:    30 * 24 * time.Hour,
    LocalMode:              true,   // Skip PR creation

    SourceWeights: map[FeatureSource]float64{
        SourceRoadmap:          2.0,  // Highest priority
        SourceHealthMetrics:    1.5,
        SourcePylonTickets:     1.2,
        SourceMCPEcosystem:     1.0,
        SourceSlackDiscussions: 0.9,
        SourceResearchPapers:   0.8,
        SourceCompetitor:       0.7,
    },
}
```

### Proposal Queue

A priority queue sorted by score (descending), with max depth enforcement:

```go
type ProposalQueue struct {
    proposals []*PerpetualProposal
    maxDepth  int
}

type PerpetualProposal struct {
    ID           string
    Source       FeatureSource
    Title        string
    Description  string
    Evidence     []string
    Impact       int           // 0-100
    Effort       EffortLevel
    Score        float64       // Calculated priority
    ContentHash  string        // Deduplication
    Status       string        // queued, implementing, completed, failed, skipped

    // Consensus tracking (v28.0)
    Confidence      int
    ProposalVotes   []ProposalVote
    ApprovalStatus  string
}
```

### Swarm Integration

Bi-directional: swarm findings feed proposals, PR outcomes feed back to swarm
worker budgets:

```go
type PerpetualEngine struct {
    // ...
    swarmOrchestrator *SwarmOrchestrator  // Bi-directional feedback
    onPROutcome func(*PerpetualProposal, bool) // merged or rejected
}
```

---

## 6. Session Memory

### Four-Component Memory System

Based on Azure SRE Agent patterns + Devin session insights:

```go
type SessionMemoryClient struct {
    userMemories  map[string][]*UserMemory     // Per-user #remember commands
    insights      map[string][]*SessionInsight // Auto-generated from sessions
    memoryIndex   map[string]map[string]int    // userID -> memory_id -> index
    insightIndex  map[string]map[string]int    // userID -> insight_id -> index
}
```

### User Memory (#remember pattern)

```go
type UserMemory struct {
    ID        string
    Content   string      // "Team owns headspace in prod"
    Category  string      // team, service, workflow, standard
    Tags      []string
    Embedding []float32   // For semantic search
}
```

### Session Insights (auto-captured)

```go
type SessionInsight struct {
    Symptoms     []string
    StepsWorked  []InsightStep    // What worked (tool, description, outcome)
    RootCause    string
    Pitfalls     []string         // What to avoid
    Context      map[string]string
    Timeline     []TimelineMilestone  // Up to 8 milestones per Azure pattern
    QualityScore int              // 1-5
}
```

### Multi-User Isolation (v101.0)

All memory is keyed by `userID` extracted from request context. Falls back to
`"default"` for backward compatibility.

### Relevant Context Auto-Loading

When starting a new investigation, the system auto-loads relevant past memories
and insights:

```go
type RelevantContext struct {
    Memories       []*UserMemory
    PastInsights   []*SessionInsight
    SuggestedSteps []string
    Warnings       []string
}
```

---

## 7. Tool Discovery Contract

### Three Discovery Tools

1. **`webb_tool_discover`** -- Browse by category with configurable detail levels:
   - `names`: Just tool names (~500 tokens)
   - `signatures`: `name(required_params)` (~800 tokens)
   - `descriptions`: Full descriptions per tool

2. **`webb_tool_schema`** -- Load full schemas on demand:
   ```
   webb_tool_schema(tool_names="webb_k8s_pods,webb_k8s_logs")
   ```
   Returns complete `InputSchema` with all parameters.

3. **`webb_tool_search`** -- Keyword/use-case search with relevance scoring.

### Search Algorithm

```go
func (r *ToolRegistry) SearchTools(query string) []ToolSearchResult {
    // Scoring: name match (100) > tag (80) > category (60) > description (40) > usecase (20)
    // Returns sorted by score descending
}
```

### Tool Examples

Discovery module maintains curated examples for common tools:

```go
var toolExamples = map[string][]ToolExample{
    "webb_cluster_health_full": {
        {Description: "Basic health check",
         Code: `webb_cluster_health_full(context="headspace-v2")`,
         Notes: "Returns health score (0-100)"},
    },
}
```

---

## 8. Chain/Template System

### Overview

YAML-defined workflow chains for multi-step operations with parallel execution,
branching, gates, and error handling.

### Chain Definition

```go
type ChainDefinition struct {
    Name        string
    Description string
    Category    ChainCategory  // operational, investigative, customer, development, remediation
    Trigger     ChainTrigger   // manual, scheduled (cron), event
    Steps       []ChainStep
    OnError     *ErrorHandler
    Timeout     string         // "30m", "1h"
    Tags        []string
}
```

### Step Types

| Type | Purpose |
|------|---------|
| `tool` | Execute an MCP tool |
| `chain` | Execute a sub-chain |
| `parallel` | Execute steps in parallel |
| `branch` | Conditional branching |
| `gate` | Human-in-the-loop approval |

### Chain Step

```go
type ChainStep struct {
    ID        string
    Type      StepType
    Tool      string              // For tool steps
    Params    map[string]string
    Chain     string              // For sub-chain steps
    Steps     []ChainStep         // For parallel steps
    Condition string              // For branch steps
    Branches  map[string][]ChainStep
    GateType  string              // "human", "approval"
    Retry     *RetryPolicy
    StoreAs   string              // Variable name for result
}
```

### Built-In Chains (31)

```go
func GetBuiltInChains() []*ChainDefinition {
    return []*ChainDefinition{
        morningRoutineChain(),           // Daily standup prep
        oncallHandoffChain(),            // On-call rotation
        weeklyReviewChain(),             // Weekly analysis
        incidentResponseChain(),         // Incident coordination
        customerInvestigationChain(),    // Customer health check
        quickTriageChain(),              // Fast issue triage
        escalationChain(),               // Escalation workflow
        autoRemediationChain(),          // Auto-fix known issues
        deployValidationChain(),         // Post-deploy checks
        deepRCAChain(),                  // Root cause analysis
        costAnomalyInvestigationChain(), // Cost spike investigation
        securityIncidentResponseChain(), // Security response
        nightlyResearchSwarmChain(),     // Nightly swarm run
        ephemeralClusterPreflightChain(),// Cluster lifecycle
        // ... 17 more
    }
}
```

### Example: Morning Routine Chain

```yaml
name: morning-routine
category: operational
trigger:
  type: scheduled
  cron: "0 0 9 * * 1-5"
steps:
  - id: health_check
    type: parallel
    steps:
      - {id: cluster, tool: webb_cluster_health_full, params: {context: "headspace-v2"}}
      - {id: alerts,  tool: webb_grafana_alerts, params: {state: "firing"}}
      - {id: tickets, tool: webb_pylon_my_queue}
      - {id: slack,   tool: webb_slack_unread}
```

### Chain Registry

Thread-safe registry with indexing by category, tag, and trigger type:

```go
type Registry struct {
    chains     map[string]*ChainDefinition
    byCategory map[ChainCategory][]string
    byTag      map[string][]string
    byTrigger  map[TriggerType][]string
}
```

---

## 9. OpenTelemetry Observability

### Stack

- **Traces:** OTLP gRPC exporter -> Grafana Tempo
- **Metrics:** Prometheus exporter + Grafana Cloud push
- **Sampling:** Parent-based with configurable ratio (default 10%)

### Metric Instruments

```go
// Core MCP metrics
toolDurationHistogram    // Tool call duration
toolCallsCounter         // Total tool calls
toolErrorsCounter        // Tool errors
requestDurationHist      // HTTP request duration
activeConnectionsGauge   // Active SSE connections
rateLimitHitsCounter     // Rate limit rejections

// Domain-specific metrics (15+ categories)
feedbackBoostGauge       // Feedback routing boost factor
tokenUsageCounter        // Token consumption
tokenBudgetGauge         // Remaining token budget
remediationCounter       // Auto-remediation actions
semanticSearchCounter    // Semantic search operations
digestGeneratedCounter   // Weekly digest generations
latencyRoutingCounter    // Latency-aware routing decisions
```

### Initialization Cascade

```go
func Init(ctx context.Context, cfg Config) error {
    initTraceProvider(ctx, cfg, res)    // OTLP traces
    initMeterProvider(ctx, res)         // Prometheus metrics
    initMetricInstruments()             // Core MCP metrics
    InitWorkflowMetrics()               // Chain execution metrics
    InitWebhookMetrics()                // Webhook processing
    InitComplianceMetrics()             // Compliance tracking
    InitFeedbackMetrics()               // User feedback
    InitTokenMetrics()                  // Token budget tracking
    InitSelfHealingMetrics()            // Auto-remediation
    InitSemanticMetrics()               // Semantic search
    InitHTTPClientMetrics()             // HTTP client telemetry
    InitDatabaseMetrics()               // DB client telemetry
    InitK8sMetrics()                    // Kubernetes API telemetry
    InitToolMetrics()                   // Per-tool observability
    InitAlertingMetrics()               // Anomaly detection
    InitOncallMetrics()                 // On-call dashboard
}
```

### Prometheus Endpoint

```go
mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
    if otel.IsEnabled() {
        otel.GetPrometheusHandler().ServeHTTP(w, r)
    } else {
        json.NewEncoder(w).Encode(mcp.GetServerMetrics())
    }
})
```

---

## 10. Safety Scoring and Risk Assessment

### 6-Factor Safety Score (0-100)

Based on OWASP AIVSS and CVSS patterns:

```
SafetyScore = 100 - RiskScore

RiskScore = (Mutability   * 0.25) +
            (Scope        * 0.20) +
            (Sensitivity  * 0.20) +
            (Reversibility * 0.15) +
            (Historical   * 0.10) +
            (Dependencies * 0.10)
```

### Mutability Levels (25% weight)

| Level | Risk | Examples |
|-------|------|----------|
| read | 0 | Any _list, _get, _search tool |
| create | 30 | _create, _add, _post |
| update | 50 | _update, _patch, _modify |
| delete | 80 | _delete, _remove, _purge |
| destroy | 100 | _destroy, _terminate, _force_delete |

### Scope Levels (20% weight)

| Level | Risk |
|-------|------|
| single | 10 |
| namespace | 40 |
| cluster | 70 |
| multi | 100 |

### Safety Grades

| Grade | Score Range | Meaning |
|-------|-------------|---------|
| A | 90-100 | Safe (green) |
| B | 75-89 | Low risk (green) |
| C | 60-74 | Caution (yellow) |
| D | 40-59 | Risky (orange) |
| F | 0-39 | Dangerous (red) |

### Graduated Approval Requirements (v129)

```go
func getApprovalRequirement(safetyGrade, riskLevel string) int {
    switch safetyGrade {
    case "F": return 2  // 2+ approvers required
    case "D": return 1  // 1 approver required
    case "C": return 1  // 1 approver required
    case "B": return 0  // No approval needed
    case "A": return 0  // No approval needed
    }
}
```

### Auto-Inference

All safety metadata is auto-inferred from tool name patterns and `IsWrite` flag.
No manual annotation needed for most tools:

```go
func applyMCPAnnotations(td *ToolDefinition) {
    // Auto-set title from name
    // Auto-set readOnlyHint, destructiveHint, idempotentHint
    // Auto-infer RiskLevel from name patterns
    // Auto-set RequiresConfirmation for write tools
    // Calculate SafetyScore and SafetyGrade
}
```

### Safety Mode (WEBB_SAFETY_MODE)

| Mode | Behavior |
|------|----------|
| `strict` | Requires `confirm_write=true` + approval gates (production) |
| `standard` | Requires `confirm_write=true` only (recommended for dev) |
| `relaxed` | No restrictions, audit only (debugging) |

---

## 11. OAuth and Multi-User RBAC

### GitHub OAuth Flow

```
Browser -> /oauth/login -> GitHub OAuth -> /oauth/callback -> Session Cookie
```

### Team-to-Role Mapping

```go
var DefaultTeamRoles = TeamRoleConfig{
    OrgName: "hairglasses",
    Mappings: map[string]string{
        "platform":  "admin",      // Full access
        "infra":     "platform",   // K8s, AWS, deployments
        "field-eng": "support",    // Tickets, incidents, comms
        "devs":      "readonly",   // Read-only tools
        "ml":        "readonly",
        "product":   "readonly",
        // ... more teams
    },
    Default: "readonly",
}
```

### Role Hierarchy

```
admin (4)  >  platform (3)  >  support (2)  >  readonly (1)
```

Highest-privilege team wins when a user belongs to multiple teams.

### Role-Based Tool Filtering

Each `ToolDefinition` has a `MinRole` field. RBAC middleware checks user roles
before allowing tool execution.

---

## 12. Kubernetes/Helm Deployment

### Deployment Topology

The Helm chart (`charts/cobb-cluster/`) deploys 7 components:

| Template | Purpose |
|----------|---------|
| `deployment.yaml` | Main MCP server (SSE mode) |
| `deployment-worker.yaml` | Background dev workers (multi-LLM) |
| `deployment-research.yaml` | Research swarm workers |
| `deployment-scraper.yaml` | Data scrapers |
| `deployment-shell.yaml` | Interactive shell access |
| `deployment-vault-api.yaml` | Obsidian vault API |
| Tailscale sidecar | Private networking |

### Resource Defaults

```yaml
resources:
  requests: {cpu: 250m, memory: 512Mi}
  limits:   {cpu: 1000m, memory: 2Gi}
```

### Security Posture

```yaml
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 1000
containerSecurityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop: [ALL]
```

### External Secrets

Secrets from AWS Secrets Manager via ExternalSecrets operator:

```yaml
externalSecrets:
  secretStore: aws-secretsmanager
  secrets:
    - name: cobb-secrets
      keys: [SLACK_BOT_TOKEN, GITHUB_TOKEN, PYLON_API_KEY,
             SHORTCUT_API_TOKEN, ANTHROPIC_API_KEY, ...]
```

### Module Enable/Disable

Helm values control which tool modules are loaded:

```yaml
modules:
  enabled: [aws, kubernetes, health, slack, tickets, ...]
  disabled: [calendar, collaboration, devtools, ...]
```

### Environment-Specific Overrides

- `values-dev.yaml` -- dev EKS
- `values-staging.yaml` -- staging
- `values-prod.yaml` -- production
- `values-gcp.yaml` -- GCP cross-cloud

### Networking

Tailscale sidecar for private mesh access (no public ingress):

```yaml
tailscale:
  enabled: true
  hostname: cobb-mcp-dev
  tailnet: acme.ts.net
```

---

## 13. Middleware Stack

### Default Chain (applied in order)

```go
func DefaultMiddlewareChain() *MiddlewareChain {
    return NewMiddlewareChain(
        MetricsMiddleware,     // Track request duration, counts
        RequestIDMiddleware,   // Assign UUID to each request
        AuthMiddleware,        // Extract user from Bearer/JWT/header
        RBACMiddleware,        // Resolve roles from team membership
        SafetyModeMiddleware,  // Enforce WEBB_SAFETY_MODE
        RateLimitMiddleware,   // Per-user rate limiting
        LoggingMiddleware,     // Request logging
    )
}
```

### Auth Sources (in priority order)

1. `Authorization: Bearer <JWT>` -- JWT validation
2. `Authorization: Bearer <API_KEY>` -- API key validation
3. `X-User-ID` header -- Testing/internal
4. Global user context -- Legacy fallback

### Context Middleware (tool-level)

Resolves entity context (customer, cluster) from request params:

```go
func WrapWithContext(handler ContextAwareHandler) ToolHandlerFunc {
    return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        rctx := resolver.Resolve(ctx, userID, params)
        ctx = WithResolvedContext(ctx, rctx)
        return handler(ctx, rctx, req)
    }
}
```

---

## 14. Async Task System

### MCP SEP-1686 Implementation

Long-running tools return a task ID immediately and report progress via polling
or SSE:

```go
type AsyncTask struct {
    ID            string           `json:"taskId"`
    ToolName      string           `json:"toolName"`
    Status        AsyncTaskStatus  `json:"status"`        // pending, working, completed, failed
    Progress      int              `json:"progress"`      // 0-100
    Message       string           `json:"statusMessage"`
    PollInterval  int              `json:"pollInterval"`  // ms
    Steps         []ProgressStep   `json:"steps"`         // Step-by-step tracking
}
```

### Task Event Broadcasting

Tasks broadcast events via SSE for real-time updates:

```go
type TaskEventBroadcaster func(eventType string, taskID, toolName string,
    progress int, message string, data interface{})
```

Event types: `task_created`, `task_progress`, `task_completed`, `task_failed`,
`task_cancelled`.

### REST Endpoints

```
GET  /tasks              -- List all tasks
GET  /tasks/{id}         -- Get task status
GET  /tasks/stream       -- SSE stream of task events
```

---

## 15. Federation

### Remote MCP Tool Proxying

Cobb can federate tools from other MCP servers, exposing them under the
`webb_` namespace:

```go
type FederatedTool struct {
    WebbToolName   string   // webb_grafana_get_dashboards
    ServerName     string   // grafana
    RemoteToolName string   // get_dashboards
}
```

### Naming Convention

```go
// Pattern: webb_{server}_{tool_name}
func GenerateWebbToolName(serverName, toolName string) string {
    return fmt.Sprintf("webb_%s_%s",
        SanitizeName(serverName),
        SanitizeName(toolName))
}
```

### Persistence

Federation state persists to `~/.config/cobb/mcp-federation.json` so federated
tools survive restarts.

---

## 16. Key Design Decisions

### Why `webb_` Prefix?

All tools are prefixed with `webb_` to:
- Avoid collisions when multiple MCP servers are connected
- Enable federation (remote tools get `webb_{server}_{tool}`)
- Make tool provenance clear in logs and traces

### Handler Contract

**Always return `(*mcp.CallToolResult, nil)` -- never `(nil, error)`.**

Errors are encoded in the result text, not as Go errors. This matches the MCP
protocol expectation that tool calls always return a result.

### Actionable Error Pattern

```go
func awsClientError(service string, err error) *mcp.CallToolResult {
    ae := common.NewActionableError(
        fmt.Sprintf("Failed to create AWS %s client", service)).
        WithCause(err.Error()).
        WithSuggestion("Check AWS credentials").
        WithSuggestion("Run: aws sts get-caller-identity")
    return mcp.NewToolResultText(ae.String())
}
```

### Client Patterns

- **Lazy initialization:** Clients created on first use via `sync.Once`
- **Circuit breaker:** Per-service health tracking (closed/open/half-open states)
- **Secrets via provider:** `clients.GetSecretFromProvider("KEY")` abstracts
  1Password / env / AWS Secrets Manager

### Common Parameter Helpers

Reusable parameter factories in `internal/mcp/tools/common/`:

```go
common.ParamContext()        // Kubernetes context
common.ParamNamespaceAcme()  // Namespace with acme default
common.ParamAWSRegion()      // AWS region
common.ParamLimit(50)        // Pagination limit
common.ParamOffset()         // Pagination offset
common.ParamECSCluster()     // ECS cluster name
```

### Structured Output Schemas

Reusable output schemas for consistent tool responses:

```go
var HealthCheckOutputSchema = &OutputSchema{
    Type: "object",
    Properties: map[string]PropertySchema{
        "health_score": {Type: "integer", Description: "0-100"},
        "status":       {Type: "string", Enum: []string{"healthy", "degraded", "critical"}},
        "issues":       {Type: "array", Items: &PropertySchema{...}},
    },
    Required: []string{"health_score", "status"},
}
```

### Cache-Aware Tool Registration

Tools declare caching hints for prompt caching optimization:

```go
func DefaultCacheMetadata(category string) (CacheHintType, int, int) {
    switch category {
    case "investigation", "triage", "oncall":
        return CacheEphemeral, CachePriorityHigh, 500
    case "kubernetes", "deployment":
        return CacheEphemeral, CachePriorityHigh, 400
    case "documentation", "confluence":
        return CacheNone, CachePriorityLow, 400
    }
}
```

### OWASP Agentic AI Top 10 Compliance

- **ASI04 (Supply Chain + Tool Poisoning):** `ToolIntegrityValidator`
- **ASI08 (DoS Prevention):** Per-user rate limiting middleware
- **ASI09 (Trust Exploitation):** `TrustBoundaryEnforcer`
- **ASI10 (Human-in-the-Loop):** Graduated approval gates based on safety grade

### Deployment Modes

| Mode | Transport | Use Case |
|------|-----------|----------|
| stdio | Standard I/O | Local Claude Code integration (default) |
| SSE | HTTP Server-Sent Events | AWS EKS deployment, multi-user |

Set `MCP_MODE=sse` for SSE mode. Stdio is the default for local use.

---

## Appendix: File Locations

| Component | Path |
|-----------|------|
| MCP server entry | `cmd/cobb-mcp/main.go` |
| CLI entry | `cmd/cobb/main.go` |
| Tool registry | `internal/mcp/tools/registry.go` |
| Module imports | `internal/mcp/tools.go` |
| Sample module | `internal/mcp/tools/aws/module.go` |
| Consolidated tools | `internal/mcp/tools/consolidated/module.go` |
| Discovery tools | `internal/mcp/tools/discovery/module.go` |
| Swarm orchestrator | `internal/clients/swarm_orchestrator.go` |
| Swarm workers | `internal/clients/swarm_workers.go` |
| Perpetual engine | `internal/clients/perpetual.go` |
| Session memory | `internal/clients/session_memory.go` |
| Chain system | `internal/chains/` |
| Chain builtins | `internal/chains/builtins.go` |
| OTel setup | `internal/mcp/otel/otel.go` |
| Middleware | `internal/mcp/middleware.go` |
| Team roles | `internal/mcp/team_roles.go` |
| OAuth handler | `internal/mcp/oauth.go` |
| Safety scoring | `internal/mcp/tools/registry.go` (lines 100-340) |
| Async tasks | `internal/mcp/tools/async.go` |
| Context middleware | `internal/mcp/tools/context_middleware.go` |
| Circuit breaker | `internal/clients/circuit_breaker.go` |
| Output schemas | `internal/mcp/tools/schemas.go` |
| Helm chart | `charts/cobb-cluster/` |
| Helm values | `charts/cobb-cluster/values.yaml` |
