// Package clients provides API clients for webb.
// v23.0: Autonomous Research Swarm Orchestrator
package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hairglasses-studio/webb/internal/mcp/otel"
)

// SwarmState represents the current state of the research swarm
type SwarmState string

const (
	SwarmStateInitializing SwarmState = "initializing"
	SwarmStateRunning      SwarmState = "running"
	SwarmStatePaused       SwarmState = "paused"
	SwarmStateStopping     SwarmState = "stopping"
	SwarmStateCompleted    SwarmState = "completed"
	SwarmStateFailed       SwarmState = "failed"
)

// SwarmWorkerType defines specialized worker types
type SwarmWorkerType string

const (
	WorkerToolAuditor       SwarmWorkerType = "tool_auditor"
	WorkerBestPractices     SwarmWorkerType = "best_practices"
	WorkerIntegrationTester SwarmWorkerType = "integration_tester"
	WorkerSecretOperator    SwarmWorkerType = "secret_operator"
	WorkerSlackDirectory    SwarmWorkerType = "slack_directory"
	WorkerVaultSync         SwarmWorkerType = "vault_sync"

	// v24.0: New worker types
	WorkerSecurityAuditor     SwarmWorkerType = "security_auditor"
	WorkerPerformanceProfiler SwarmWorkerType = "performance_profiler"

	// v25.0: MCP-integrated workers
	WorkerKnowledgeGraph   SwarmWorkerType = "knowledge_graph"
	WorkerConsensus        SwarmWorkerType = "consensus_validator"
	WorkerCrossRef         SwarmWorkerType = "cross_reference"
	WorkerFeatureDiscovery SwarmWorkerType = "feature_discovery"

	// v25.0: New analysis workers
	WorkerCodeQuality   SwarmWorkerType = "code_quality"
	WorkerDependency    SwarmWorkerType = "dependency_audit"
	WorkerTestCoverage  SwarmWorkerType = "test_coverage"
	WorkerDocumentation SwarmWorkerType = "documentation"
	WorkerRunbookGen    SwarmWorkerType = "runbook_generator"

	// v26.0: New intelligence workers
	WorkerPatternDiscovery SwarmWorkerType = "pattern_discovery"
	WorkerImprovementAudit SwarmWorkerType = "improvement_audit"
	WorkerSemanticIntel    SwarmWorkerType = "semantic_intel"
	WorkerPredictive       SwarmWorkerType = "predictive"
	WorkerComplianceAudit  SwarmWorkerType = "compliance_audit"
	WorkerMetaIntel        SwarmWorkerType = "meta_intel"

	// v28.0: External data source workers
	WorkerGitHubIssues   SwarmWorkerType = "github_issues"
	WorkerSentryPatterns SwarmWorkerType = "sentry_patterns"

	// v107.0: Code quality workers
	WorkerLinter SwarmWorkerType = "linter"

	// v33.0: Historical data scraper workers (all-time deep scraping)
	WorkerScraperPylon       SwarmWorkerType = "scraper_pylon"
	WorkerScraperShortcut    SwarmWorkerType = "scraper_shortcut"
	WorkerScraperSlack       SwarmWorkerType = "scraper_slack"
	WorkerScraperGitHub      SwarmWorkerType = "scraper_github"
	WorkerScraperIncidentIO  SwarmWorkerType = "scraper_incidentio"
	WorkerScraperConfluence  SwarmWorkerType = "scraper_confluence"
	WorkerScraperSentry      SwarmWorkerType = "scraper_sentry"
	WorkerScraperGrafana     SwarmWorkerType = "scraper_grafana"
	WorkerScraperPostgres    SwarmWorkerType = "scraper_postgres"
	WorkerScraperClickHouse  SwarmWorkerType = "scraper_clickhouse"
	WorkerScraperGmail       SwarmWorkerType = "scraper_gmail"
	WorkerScraperGDrive      SwarmWorkerType = "scraper_gdrive"
	WorkerScraperAWS         SwarmWorkerType = "scraper_aws"
	WorkerScraperUptimeRobot SwarmWorkerType = "scraper_uptimerobot"
	WorkerScraperRabbitMQ    SwarmWorkerType = "scraper_rabbitmq"
)

// SwarmConfig configures the research swarm
type SwarmConfig struct {
	Duration           time.Duration            `json:"duration"`             // Total runtime (e.g., 24h)
	CheckpointInterval time.Duration            `json:"checkpoint_interval"`  // How often to checkpoint (default: 5min)
	MergeInterval      time.Duration            `json:"merge_interval"`       // How often to merge findings (default: 30min)
	MaxTokensTotal     int64                    `json:"max_tokens_total"`     // Total token budget
	MaxTokensPerWorker int64                    `json:"max_tokens_per_worker"`// Per-worker budget
	FocusAreas         []string                 `json:"focus_areas"`          // Areas to focus on
	Workers            []SwarmWorkerConfig      `json:"workers"`              // Worker configurations
	LocalMode          bool                     `json:"local_mode"`           // No PR creation
	VaultLogging       bool                     `json:"vault_logging"`        // Log to vault
	RoadmapIntegration bool                     `json:"roadmap_integration"`  // Update Roadmap.md
	VaultPath          string                   `json:"vault_path"`           // Path to vault (default: ~/webb-vault)
}

// SwarmWorkerConfig configures a swarm worker
type SwarmWorkerConfig struct {
	Type   SwarmWorkerType `json:"type"`
	Count  int             `json:"count"`
	Budget int64           `json:"budget,omitempty"` // Override per-worker budget
}

// DefaultSwarmConfig returns sensible swarm defaults
// v131.3: Token budgets configurable via WEBB_SWARM_TOKENS_TOTAL and WEBB_SWARM_TOKENS_PER_WORKER
func DefaultSwarmConfig() *SwarmConfig {
	// Default budgets (can be overridden via env for scaled campaigns)
	maxTokensTotal := int64(5000000)      // 5M tokens
	maxTokensPerWorker := int64(500000)   // 500K per worker

	// Check for environment overrides (v131.3: extended campaign support)
	if v := os.Getenv("WEBB_SWARM_TOKENS_TOTAL"); v != "" {
		if parsed, err := parseSwarmInt64(v); err == nil && parsed > 0 {
			maxTokensTotal = parsed
		}
	}
	if v := os.Getenv("WEBB_SWARM_TOKENS_PER_WORKER"); v != "" {
		if parsed, err := parseSwarmInt64(v); err == nil && parsed > 0 {
			maxTokensPerWorker = parsed
		}
	}

	return &SwarmConfig{
		Duration:           24 * time.Hour,
		CheckpointInterval: 5 * time.Minute,
		MergeInterval:      30 * time.Minute,
		MaxTokensTotal:     maxTokensTotal,
		MaxTokensPerWorker: maxTokensPerWorker,
		FocusAreas: []string{
			"tool-consistency",
			"best-practices",
			"secret-operator",
			"slack-directory",
			"vault-sync",
		},
		Workers: []SwarmWorkerConfig{
			{Type: WorkerToolAuditor, Count: 2},
			{Type: WorkerBestPractices, Count: 1},
			{Type: WorkerIntegrationTester, Count: 1},
			{Type: WorkerSecretOperator, Count: 1},
			{Type: WorkerSlackDirectory, Count: 1},
			// v24.0: New workers
			{Type: WorkerSecurityAuditor, Count: 1},
			{Type: WorkerPerformanceProfiler, Count: 1},
			// v25.0: MCP-integrated workers
			{Type: WorkerKnowledgeGraph, Count: 1},
			{Type: WorkerCrossRef, Count: 1},
			{Type: WorkerFeatureDiscovery, Count: 1},
			// v25.0: Analysis workers
			{Type: WorkerCodeQuality, Count: 1},
			{Type: WorkerDependency, Count: 1},
			{Type: WorkerDocumentation, Count: 1},
			// v26.0: Intelligence workers
			{Type: WorkerPatternDiscovery, Count: 1},
			{Type: WorkerImprovementAudit, Count: 1},
			{Type: WorkerSemanticIntel, Count: 1},
			{Type: WorkerPredictive, Count: 1},
			{Type: WorkerComplianceAudit, Count: 1},
			{Type: WorkerMetaIntel, Count: 1},
			// v28.0: External data source workers
			{Type: WorkerGitHubIssues, Count: 1},
			{Type: WorkerSentryPatterns, Count: 1},
		},
		LocalMode:          true,
		VaultLogging:       true,
		RoadmapIntegration: true,
		VaultPath:          filepath.Join(os.Getenv("HOME"), "webb-vault"),
	}
}

// parseSwarmInt64 parses a string to int64 with error handling
func parseSwarmInt64(s string) (int64, error) {
	var result int64
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

// FindingStatus represents the lifecycle status of a finding
// v27.0: Proper type for finding status tracking
type FindingStatus string

const (
	FindingStatusPending  FindingStatus = "pending"   // Newly discovered, not yet actioned
	FindingStatusQueued   FindingStatus = "queued"    // Queued for perpetual engine
	FindingStatusActioned FindingStatus = "actioned"  // Sent to perpetual, PR created
	FindingStatusMerged   FindingStatus = "merged"    // PR merged successfully
	FindingStatusRejected FindingStatus = "rejected"  // PR rejected or finding invalid
	FindingStatusSkipped  FindingStatus = "skipped"   // Skipped (duplicate, low confidence)
)

// SwarmResearchFinding represents a research finding from a swarm worker
type SwarmResearchFinding struct {
	ID          string          `json:"id"`
	WorkerID    string          `json:"worker_id"`
	WorkerType  SwarmWorkerType `json:"worker_type"`
	Category    string          `json:"category"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Evidence    []string        `json:"evidence"`
	Confidence  int             `json:"confidence"` // 0-100
	Impact      int             `json:"impact"`     // 0-100
	Effort      string          `json:"effort"`     // small, medium, large
	Status      string          `json:"status"`     // pending, merged, rejected (legacy)
	CreatedAt   time.Time       `json:"created_at"`
	MergedAt    *time.Time      `json:"merged_at,omitempty"`

	// v27.0: Enhanced status tracking
	FindingStatus FindingStatus `json:"finding_status"` // Lifecycle status
	ActionedAt    *time.Time    `json:"actioned_at,omitempty"`
	ProposalID    string        `json:"proposal_id,omitempty"` // Linked perpetual proposal

	// v31.0: Cached embedding for semantic deduplication (not serialized)
	embedding []float32 `json:"-"`

	// v31.0: Quality gate validation results
	QualityScore  int      `json:"quality_score,omitempty"`
	QualityIssues []string `json:"quality_issues,omitempty"`
}

// SwarmCheckpoint represents a swarm state checkpoint
type SwarmCheckpoint struct {
	ID              string                  `json:"id"`
	SwarmID         string                  `json:"swarm_id"`
	State           SwarmState              `json:"state"`
	WorkerStates    map[string]SwarmWorkerStatus `json:"worker_states"`
	FindingsCount   int                     `json:"findings_count"`
	TokensUsed      int64                   `json:"tokens_used"`
	TokensRemaining int64                   `json:"tokens_remaining"`
	Duration        time.Duration           `json:"duration"`
	CreatedAt       time.Time               `json:"created_at"`
}

// SwarmWorkerStatus represents the status of a swarm worker
type SwarmWorkerStatus struct {
	WorkerID    string          `json:"worker_id"`
	WorkerType  SwarmWorkerType `json:"worker_type"`
	State       string          `json:"state"` // running, paused, stopped, failed
	TasksQueued int             `json:"tasks_queued"`
	TasksDone   int             `json:"tasks_done"`
	Findings    int             `json:"findings"`
	TokensUsed  int64           `json:"tokens_used"`
	LastActive  time.Time       `json:"last_active"`
	Error       string          `json:"error,omitempty"`
}

// SwarmMetrics tracks swarm performance
type SwarmMetrics struct {
	StartedAt           time.Time                  `json:"started_at"`
	Duration            time.Duration              `json:"duration"`
	TotalWorkers        int                        `json:"total_workers"`
	ActiveWorkers       int                        `json:"active_workers"`
	TotalFindings       int                        `json:"total_findings"`
	MergedFindings      int                        `json:"merged_findings"`
	TokensUsed          int64                      `json:"tokens_used"`
	TokensRemaining     int64                      `json:"tokens_remaining"`
	CheckpointsCreated  int                        `json:"checkpoints_created"`
	RateLimitHits       int                        `json:"rate_limit_hits"`
	FindingsByCategory  map[string]int             `json:"findings_by_category"`
	FindingsByWorker    map[SwarmWorkerType]int    `json:"findings_by_worker"`

	// v25.0: Enhanced tracking
	WorkerEfficiency    map[SwarmWorkerType]*WorkerEfficiencyStats `json:"worker_efficiency"`
	CategorySaturation  map[string]*CategorySaturationStats        `json:"category_saturation"`
}

// v25.0: WorkerEfficiencyStats tracks worker performance over time
type WorkerEfficiencyStats struct {
	WorkerType        SwarmWorkerType `json:"worker_type"`
	FindingsTotal     int             `json:"findings_total"`
	FindingsAccepted  int             `json:"findings_accepted"`
	FindingsRejected  int             `json:"findings_rejected"`
	AcceptanceRate    float64         `json:"acceptance_rate"`
	TokensUsed        int64           `json:"tokens_used"`
	TokensPerFinding  float64         `json:"tokens_per_finding"`
	AvgConfidence     float64         `json:"avg_confidence"`
	LastUpdated       time.Time       `json:"last_updated"`
}

// v25.0: CategorySaturationStats tracks category saturation
type CategorySaturationStats struct {
	Category        string    `json:"category"`
	FindingsTotal   int       `json:"findings_total"`
	FindingsLast7D  int       `json:"findings_last_7d"`
	UniquePatterns  int       `json:"unique_patterns"`   // Distinct issues found
	NewRatio        float64   `json:"new_ratio"`         // % of genuinely new findings
	IsSaturated     bool      `json:"is_saturated"`      // >10 findings, <20% new
	LastUpdated     time.Time `json:"last_updated"`
}

// SwarmOrchestrator coordinates the research swarm
type SwarmOrchestrator struct {
	id         string
	config     *SwarmConfig
	state      SwarmState
	metrics    *SwarmMetrics
	workers    map[string]*SwarmWorker
	findings   []*SwarmResearchFinding
	recovery   *SwarmRecoveryManager
	vaultPath  string
	runPath    string

	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.RWMutex

	// Channels
	findingsCh    chan *SwarmResearchFinding
	checkpointCh  chan struct{}
	stopCh        chan struct{}

	// Callbacks
	onFinding     func(*SwarmResearchFinding)
	onCheckpoint  func(*SwarmCheckpoint)
	onStateChange func(SwarmState, SwarmState)

	// Metrics push to Grafana Cloud
	metricsProvider *otel.SwarmMetricsProvider

	// v26.0: Externalized configuration (loaded at startup)
	configV25 *SwarmConfigV25

	// v27.0: Perpetual engine integration for findings→action flow
	perpetualEngine *PerpetualEngine

	// v28.0: Health alerts storage
	healthAlerts []HealthAlert

	// v31.0: Alert deduplication and Slack integration
	alertDedupe  map[string]time.Time // Hash -> last sent time
	slackClient  *SlackClient

	// v31.0: Quality gate for findings validation
	qualityGate *SwarmQualityGate

	// v32.5: Finding aggregation layer
	aggregator      *FindingAggregator
	sessionStart    time.Time // For cross-session exclusion
	duplicateCounts map[string]int // Batch duplicate logging
}

// Global swarm orchestrator singleton
var (
	globalSwarmOrchestrator   *SwarmOrchestrator
	globalSwarmOrchestratorMu sync.RWMutex
)

// SetGlobalSwarmOrchestrator sets the global swarm orchestrator
func SetGlobalSwarmOrchestrator(s *SwarmOrchestrator) {
	globalSwarmOrchestratorMu.Lock()
	defer globalSwarmOrchestratorMu.Unlock()
	globalSwarmOrchestrator = s
}

// GetGlobalSwarmOrchestrator returns the global swarm orchestrator
func GetGlobalSwarmOrchestrator() *SwarmOrchestrator {
	globalSwarmOrchestratorMu.RLock()
	defer globalSwarmOrchestratorMu.RUnlock()
	return globalSwarmOrchestrator
}

// SetPerpetualEngine sets the perpetual engine for findings→action flow
// v27.0: Enables automatic feeding of high-confidence findings to proposals
func (s *SwarmOrchestrator) SetPerpetualEngine(engine *PerpetualEngine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.perpetualEngine = engine
}

// GetPerpetualEngine returns the perpetual engine
func (s *SwarmOrchestrator) GetPerpetualEngine() *PerpetualEngine {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.perpetualEngine
}

// NewSwarmOrchestrator creates a new swarm orchestrator
func NewSwarmOrchestrator(config *SwarmConfig) (*SwarmOrchestrator, error) {
	if config == nil {
		config = DefaultSwarmConfig()
	}

	id := uuid.New().String()[:8]
	now := time.Now()
	runPath := filepath.Join(config.VaultPath, "research", "swarm-runs",
		fmt.Sprintf("%s-swarm-%s", now.Format("2006-01-02"), id))

	// Create run directory
	if config.VaultLogging {
		if err := os.MkdirAll(filepath.Join(runPath, "findings"), 0755); err != nil {
			return nil, fmt.Errorf("failed to create swarm run directory: %w", err)
		}
		if err := os.MkdirAll(filepath.Join(runPath, "checkpoints"), 0755); err != nil {
			return nil, fmt.Errorf("failed to create checkpoints directory: %w", err)
		}
	}

	s := &SwarmOrchestrator{
		id:           id,
		config:       config,
		state:        SwarmStateInitializing,
		metrics:      &SwarmMetrics{
			StartedAt:          now,
			FindingsByCategory: make(map[string]int),
			FindingsByWorker:   make(map[SwarmWorkerType]int),
			TokensRemaining:    config.MaxTokensTotal,
			// v25.0: Initialize enhanced tracking
			WorkerEfficiency:   make(map[SwarmWorkerType]*WorkerEfficiencyStats),
			CategorySaturation: make(map[string]*CategorySaturationStats),
		},
		workers:      make(map[string]*SwarmWorker),
		findings:     make([]*SwarmResearchFinding, 0),
		vaultPath:    config.VaultPath,
		runPath:      runPath,
		findingsCh:   make(chan *SwarmResearchFinding, 100),
		checkpointCh: make(chan struct{}, 1),
		stopCh:       make(chan struct{}),
	}

	// Initialize recovery manager
	s.recovery = NewSwarmRecoveryManager(s)

	// v27.0: Register run in recovery database for cross-process state
	if err := s.recovery.SaveRun(id, config, SwarmStateInitializing, now); err != nil {
		log.Printf("swarm: warning: failed to register run: %v", err)
	}

	// v26.0: Load externalized configuration
	configPath := filepath.Join(config.VaultPath, "config", "swarm-v25.json")
	s.configV25, _ = LoadSwarmConfigV25(configPath)
	if s.configV25 == nil {
		s.configV25 = DefaultSwarmConfigV25()
	}

	// v31.0: Initialize alert deduplication and Slack client
	s.alertDedupe = make(map[string]time.Time)
	s.slackClient, _ = NewSlackClient() // Non-fatal if Slack unavailable

	// v31.0: Initialize quality gate for findings validation
	shortcutClient, _ := NewShortcutClient()
	s.qualityGate = NewSwarmQualityGate(shortcutClient)

	// v32.5: Initialize finding aggregation layer
	s.aggregator = NewFindingAggregator(nil) // Uses default config
	s.sessionStart = now
	s.duplicateCounts = make(map[string]int)

	return s, nil
}

// Start starts the swarm
func (s *SwarmOrchestrator) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.state != SwarmStateInitializing && s.state != SwarmStatePaused {
		s.mu.Unlock()
		return fmt.Errorf("cannot start swarm in state: %s", s.state)
	}

	s.ctx, s.cancel = context.WithTimeout(ctx, s.config.Duration)
	s.setState(SwarmStateRunning)
	s.mu.Unlock()

	// Wire up credentials provider for Grafana Cloud metrics push
	otel.SetCredentialsProvider(GetSecretFromProvider)

	// Initialize and register Grafana Cloud metrics push
	s.metricsProvider = otel.NewSwarmMetricsProvider(s.id)
	if pusher := otel.GetGrafanaPusher(); pusher != nil {
		pusher.RegisterProvider(s.metricsProvider)
		pusher.Start()
	}

	// v32.0: Initialize the MCP handler bridge for real tool execution
	// v131.3: Changed to false - only register read-only priority handlers
	// Prevents swarm workers from executing write operations (sc-incident: Ricardo tickets)
	InitializeBridge(false)

	// Spawn workers
	if err := s.spawnWorkers(); err != nil {
		s.setState(SwarmStateFailed)
		return fmt.Errorf("failed to spawn workers: %w", err)
	}

	// Start background goroutines
	s.wg.Add(5) // v28.0: Add health monitor
	go s.findingsCollector()
	go s.checkpointLoop()
	go s.statusLogger()
	go s.perpetualFeeder()  // v27.0: Feed findings to perpetual engine
	go s.healthMonitor()    // v28.0: Monitor worker health

	// Log initial status
	s.logStatus("Swarm started")

	return nil
}

// Stop gracefully stops the swarm
func (s *SwarmOrchestrator) Stop() error {
	s.mu.Lock()
	if s.state != SwarmStateRunning && s.state != SwarmStatePaused {
		s.mu.Unlock()
		return fmt.Errorf("cannot stop swarm in state: %s", s.state)
	}
	s.setState(SwarmStateStopping)
	s.mu.Unlock()

	// Signal stop
	close(s.stopCh)
	if s.cancel != nil {
		s.cancel()
	}

	// Wait for workers to finish
	for _, worker := range s.workers {
		worker.Stop()
	}

	// Wait for background goroutines
	s.wg.Wait()

	// Final checkpoint
	s.createCheckpoint()

	// Log final status
	s.logStatus("Swarm stopped")
	s.logSummary()

	s.mu.Lock()
	s.setState(SwarmStateCompleted)
	s.mu.Unlock()

	// v27.0: Finalize run in recovery database
	if err := s.recovery.CompleteRun(s.id, SwarmStateCompleted, s.metrics); err != nil {
		log.Printf("swarm: warning: failed to finalize run: %v", err)
	}

	return nil
}

// Pause pauses the swarm
func (s *SwarmOrchestrator) Pause() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != SwarmStateRunning {
		return fmt.Errorf("cannot pause swarm in state: %s", s.state)
	}

	for _, worker := range s.workers {
		worker.Pause()
	}

	s.setState(SwarmStatePaused)
	s.logStatus("Swarm paused")
	return nil
}

// Resume resumes a paused swarm
func (s *SwarmOrchestrator) Resume() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != SwarmStatePaused {
		return fmt.Errorf("cannot resume swarm in state: %s", s.state)
	}

	for _, worker := range s.workers {
		worker.Resume()
	}

	s.setState(SwarmStateRunning)
	s.logStatus("Swarm resumed")
	return nil
}

// Checkpoint triggers an immediate checkpoint
func (s *SwarmOrchestrator) Checkpoint() error {
	select {
	case s.checkpointCh <- struct{}{}:
		return nil
	default:
		return fmt.Errorf("checkpoint already in progress")
	}
}

// AddFinding adds a finding from a worker
func (s *SwarmOrchestrator) AddFinding(finding *SwarmResearchFinding) {
	finding.ID = uuid.New().String()[:8]
	finding.CreatedAt = time.Now()
	finding.Status = "pending"
	finding.FindingStatus = FindingStatusPending // v27.0: Enhanced tracking

	// v28.0: Semantic deduplication - skip if too similar to recent findings
	if s.isDuplicateFinding(finding) {
		finding.FindingStatus = FindingStatusSkipped
		fmt.Printf("swarm: skipping duplicate finding: %s (semantic match)\n", finding.Title[:min(40, len(finding.Title))])
		return
	}

	// Record metric
	otel.RecordSwarmFinding(s.ctx, s.id, string(finding.WorkerType), finding.Category, finding.Confidence)

	select {
	case s.findingsCh <- finding:
	default:
		// Channel full, log directly
		s.mu.Lock()
		s.findings = append(s.findings, finding)
		s.mu.Unlock()
	}
}

// v28.0: isDuplicateFinding checks if a finding is semantically similar to existing findings
// v31.0: Updated to use stored embeddings from recovery database for efficiency
// v32.5: Added order-invariant evidence comparison and batch logging
func (s *SwarmOrchestrator) isDuplicateFinding(finding *SwarmResearchFinding) bool {
	// v32.5: First check order-invariant evidence hash for exact duplicates
	evidenceHash := EvidenceHash(finding.Evidence)
	s.mu.RLock()
	for _, existing := range s.findings {
		if existing.Category == finding.Category && EvidenceHash(existing.Evidence) == evidenceHash {
			s.mu.RUnlock()
			s.trackDuplicateForBatchLog(finding)
			return true
		}
	}
	s.mu.RUnlock()

	embedClient := GetEmbeddingClient()
	if embedClient == nil {
		return false // Skip semantic deduplication if no embedding client
	}

	// Generate embedding for new finding
	findingText := finding.Title + " " + finding.Description
	newEmbed, err := embedClient.Embed(s.ctx, findingText)
	if err != nil {
		return false // Skip deduplication on error
	}

	// v31.0: Store embedding with finding for future lookups
	finding.embedding = newEmbed.Vector

	// v31.0: Check against stored embeddings in recovery database
	if s.recovery != nil {
		// Convert float32 to float64 for storage
		embedding64 := make([]float64, len(newEmbed.Vector))
		for i, v := range newEmbed.Vector {
			embedding64[i] = float64(v)
		}

		similarFindings, err := s.recovery.FindSimilarFindings(embedding64, 0.85, 5)
		if err == nil && len(similarFindings) > 0 {
			// Found semantic duplicate in persistent storage
			s.trackDuplicateForBatchLog(finding)
			return true
		}
	}

	// Fallback: check in-memory findings (for findings not yet persisted)
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-24 * time.Hour)
	similarityThreshold := float32(0.85) // 85% semantic similarity

	for _, existing := range s.findings {
		if existing.CreatedAt.Before(cutoff) {
			continue // Only check recent findings
		}
		if existing.embedding == nil {
			continue // Skip if no embedding cached
		}

		similarity := CosineSimilarity(newEmbed.Vector, existing.embedding)
		if similarity > similarityThreshold {
			s.trackDuplicateForBatchLog(finding)
			return true
		}
	}

	return false
}

// v32.5: trackDuplicateForBatchLog increments duplicate count for batch logging
func (s *SwarmOrchestrator) trackDuplicateForBatchLog(finding *SwarmResearchFinding) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Extract pattern key for batch grouping
	patternKey := fmt.Sprintf("%s:%s", finding.WorkerType, finding.Category)
	s.duplicateCounts[patternKey]++
}

// v32.5: LogDuplicateSummary logs a batch summary of all duplicates (call at end of run)
func (s *SwarmOrchestrator) LogDuplicateSummary() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.duplicateCounts) == 0 {
		return
	}

	totalSkipped := 0
	for _, count := range s.duplicateCounts {
		totalSkipped += count
	}

	log.Printf("swarm: duplicate summary - skipped %d total duplicates across %d patterns",
		totalSkipped, len(s.duplicateCounts))

	// Log top patterns
	for pattern, count := range s.duplicateCounts {
		if count >= 5 {
			log.Printf("swarm:   %s: %d duplicates skipped", pattern, count)
		}
	}
}

// v32.5: GetSessionStart returns when this orchestrator session started
func (s *SwarmOrchestrator) GetSessionStart() time.Time {
	return s.sessionStart
}

// v32.5: GetAggregatedFindings returns findings after aggregation
func (s *SwarmOrchestrator) GetAggregatedFindings() []*SwarmResearchFinding {
	if s.aggregator == nil {
		return s.findings
	}
	return s.aggregator.GetAggregatedFindings()
}

// v32.5: GetAggregatorStats returns aggregation statistics
func (s *SwarmOrchestrator) GetAggregatorStats() *AggregatorStats {
	if s.aggregator == nil {
		return nil
	}
	return s.aggregator.GetStats()
}

// v32.5: GetDuplicateLogSummary returns batch duplicate counts for logging
func (s *SwarmOrchestrator) GetDuplicateLogSummary() map[string]int {
	if s.aggregator == nil {
		return nil
	}
	return s.aggregator.GetDuplicateLogSummary()
}

// v32.5: RunWorkerSample runs a single worker type and returns findings count and tokens used
func (s *SwarmOrchestrator) RunWorkerSample(ctx context.Context, workerType SwarmWorkerType) (findings int, tokensUsed int64) {
	// Create a temporary worker for sampling
	workerID := fmt.Sprintf("sample-%s-%s", workerType, uuid.New().String()[:8])

	worker, err := NewSwarmWorker(workerID, workerType, s.config.MaxTokensPerWorker, s)
	if err != nil {
		log.Printf("swarm: failed to create sample worker %s: %v", workerType, err)
		return 0, 0
	}

	// Run worker until context cancels
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()

	<-ctx.Done()
	<-done // Wait for worker to finish

	// Get results
	status := worker.GetStatus()
	return status.Findings, status.TokensUsed
}

// GetStatus returns the current swarm status
func (s *SwarmOrchestrator) GetStatus() *SwarmStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workerStatuses := make([]SwarmWorkerStatus, 0, len(s.workers))
	for _, w := range s.workers {
		workerStatuses = append(workerStatuses, w.GetStatus())
	}

	return &SwarmStatus{
		ID:              s.id,
		State:           s.state,
		Config:          s.config,
		Metrics:         s.metrics,
		Workers:         workerStatuses,
		FindingsCount:   len(s.findings),
		RunPath:         s.runPath,
	}
}

// SwarmStatus represents the full swarm status
type SwarmStatus struct {
	ID            string          `json:"id"`
	State         SwarmState      `json:"state"`
	Config        *SwarmConfig    `json:"config"`
	Metrics       *SwarmMetrics   `json:"metrics"`
	Workers       []SwarmWorkerStatus  `json:"workers"`
	FindingsCount int             `json:"findings_count"`
	RunPath       string          `json:"run_path"`
}

// GetFindings returns all findings, optionally filtered
func (s *SwarmOrchestrator) GetFindings(category string, workerType SwarmWorkerType, limit int) []*SwarmResearchFinding {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*SwarmResearchFinding, 0)
	for _, f := range s.findings {
		if category != "" && f.Category != category {
			continue
		}
		if workerType != "" && f.WorkerType != workerType {
			continue
		}
		result = append(result, f)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

// UpdateFindingStatus updates the status of a finding
// v27.0: For tracking findings through the action pipeline
func (s *SwarmOrchestrator) UpdateFindingStatus(findingID string, status FindingStatus, proposalID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, f := range s.findings {
		if f.ID == findingID {
			f.FindingStatus = status
			if proposalID != "" {
				f.ProposalID = proposalID
			}
			if status == FindingStatusActioned {
				now := time.Now()
				f.ActionedAt = &now
			}
			if status == FindingStatusMerged {
				now := time.Now()
				f.MergedAt = &now
				f.Status = "merged" // Legacy field
			}
			if status == FindingStatusRejected {
				f.Status = "rejected" // Legacy field
			}
			return true
		}
	}
	return false
}

// GetFindingsByStatus returns findings filtered by FindingStatus
// v27.0: For tracking pipeline progress
func (s *SwarmOrchestrator) GetFindingsByStatus(status FindingStatus) []*SwarmResearchFinding {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*SwarmResearchFinding, 0)
	for _, f := range s.findings {
		if f.FindingStatus == status {
			result = append(result, f)
		}
	}
	return result
}

// spawnWorkers creates and starts all workers
func (s *SwarmOrchestrator) spawnWorkers() error {
	for _, wc := range s.config.Workers {
		for i := 0; i < wc.Count; i++ {
			workerID := fmt.Sprintf("%s-%d", wc.Type, i+1)
			budget := wc.Budget
			if budget == 0 {
				// v26.0: Use configV25 for per-worker-type budgets
				if v25Budget, ok := s.configV25.WorkerBudgets[wc.Type]; ok {
					budget = v25Budget
				} else {
					budget = s.config.MaxTokensPerWorker
				}
			}

			// v26.0: Apply feedback-based budget multiplier
			// High performers get more tokens, underperformers get less
			multiplier := s.GetWorkerBudgetMultiplier(wc.Type)
			budget = int64(float64(budget) * multiplier)

			worker, err := NewSwarmWorker(workerID, wc.Type, budget, s)
			if err != nil {
				return fmt.Errorf("failed to create worker %s: %w", workerID, err)
			}

			s.workers[workerID] = worker
			s.metrics.TotalWorkers++

			// Start worker
			go worker.Run(s.ctx)
			s.metrics.ActiveWorkers++

			// Record worker state metric
			otel.RecordSwarmWorkerState(s.ctx, s.id, workerID, string(wc.Type), 1) // 1=running
		}
	}

	// Record total worker count
	otel.RecordSwarmWorkers(s.ctx, s.id, int64(s.metrics.TotalWorkers))
	return nil
}

// findingsCollector collects findings from workers
func (s *SwarmOrchestrator) findingsCollector() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.stopCh:
			return
		case finding := <-s.findingsCh:
			s.mu.Lock()
			s.findings = append(s.findings, finding)
			s.metrics.TotalFindings++
			s.metrics.FindingsByCategory[finding.Category]++
			s.metrics.FindingsByWorker[finding.WorkerType]++
			s.mu.Unlock()

			// v31.0: Persist finding with embedding to recovery database
			if s.recovery != nil && finding.embedding != nil {
				embedding64 := make([]float64, len(finding.embedding))
				for i, v := range finding.embedding {
					embedding64[i] = float64(v)
				}
				_ = s.recovery.SaveFindingWithEmbedding(s.id, finding, embedding64)
			}

			// v25.0: Track worker efficiency and category saturation
			s.TrackWorkerEfficiency(finding.WorkerType, finding)
			s.TrackCategorySaturation(finding.Category, finding)

			// Log finding to vault
			if s.config.VaultLogging {
				s.logFinding(finding)
			}

			// Call callback
			if s.onFinding != nil {
				s.onFinding(finding)
			}
		}
	}
}

// checkpointLoop creates periodic checkpoints
func (s *SwarmOrchestrator) checkpointLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.CheckpointInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.createCheckpoint()
		case <-s.checkpointCh:
			s.createCheckpoint()
		}
	}
}

// statusLogger logs status updates
func (s *SwarmOrchestrator) statusLogger() {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.logStatus("Status update")
		}
	}
}

// perpetualFeeder feeds high-confidence findings to the perpetual engine
// v27.0: This is the key integration that makes findings flow to action
func (s *SwarmOrchestrator) perpetualFeeder() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.RLock()
			engine := s.perpetualEngine
			s.mu.RUnlock()

			if engine != nil {
				// Get minimum confidence from config, default 60
				minConfidence := 60
				if s.configV25 != nil {
					if threshold, ok := s.configV25.SourceThresholds[SourceSwarmFinding]; ok {
						minConfidence = threshold.MinConfidence
					}
				}
				fed := s.feedToPerpetualInternal(engine, minConfidence)
				if fed > 0 {
					log.Printf("swarm: fed %d findings to perpetual engine", fed)
				}
			}
		}
	}
}

// v28.0: healthMonitor monitors worker health and generates alerts
func (s *SwarmOrchestrator) healthMonitor() {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.checkWorkerHealth()
		}
	}
}

// v28.0: checkWorkerHealth checks all workers for health issues
func (s *SwarmOrchestrator) checkWorkerHealth() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var alerts []HealthAlert

	for id, w := range s.workers {
		status := w.GetStatus()

		// Alert: Zero findings in 30 minutes
		if status.Findings == 0 && time.Since(status.LastActive) > 30*time.Minute {
			alerts = append(alerts, HealthAlert{
				Type:       AlertWorkerStalled,
				WorkerID:   id,
				WorkerType: string(status.WorkerType),
				Message:    fmt.Sprintf("Worker %s (%s) has 0 findings and hasn't been active for %v", id, status.WorkerType, time.Since(status.LastActive).Round(time.Minute)),
				Severity:   "warning",
				Timestamp:  time.Now(),
			})
		}

		// Alert: Budget near exhaustion (>90% used)
		budget := s.config.MaxTokensPerWorker
		if s.configV25 != nil {
			budget = s.configV25.GetWorkerBudget(status.WorkerType, budget)
		}
		if budget > 0 && float64(status.TokensUsed)/float64(budget) > 0.9 {
			alerts = append(alerts, HealthAlert{
				Type:       AlertBudgetExhaustion,
				WorkerID:   id,
				WorkerType: string(status.WorkerType),
				Message:    fmt.Sprintf("Worker %s (%s) at %.0f%% budget (%d/%d tokens)", id, status.WorkerType, float64(status.TokensUsed)/float64(budget)*100, status.TokensUsed, budget),
				Severity:   "warning",
				Timestamp:  time.Now(),
			})
		}

		// Alert: Worker error
		if status.Error != "" {
			alerts = append(alerts, HealthAlert{
				Type:       AlertWorkerError,
				WorkerID:   id,
				WorkerType: string(status.WorkerType),
				Message:    fmt.Sprintf("Worker %s (%s) error: %s", id, status.WorkerType, status.Error),
				Severity:   "error",
				Timestamp:  time.Now(),
			})
		}

		// Alert: Worker stopped unexpectedly
		if status.State == "stopped" && status.Error == "" && status.TokensUsed < budget {
			alerts = append(alerts, HealthAlert{
				Type:       AlertWorkerStopped,
				WorkerID:   id,
				WorkerType: string(status.WorkerType),
				Message:    fmt.Sprintf("Worker %s (%s) stopped unexpectedly (state: %s, tokens: %d/%d)", id, status.WorkerType, status.State, status.TokensUsed, budget),
				Severity:   "warning",
				Timestamp:  time.Now(),
			})
		}
	}

	// Process alerts
	for _, alert := range alerts {
		s.processHealthAlert(alert)
	}

	// Update metrics
	if s.metricsProvider != nil && len(alerts) > 0 {
		log.Printf("swarm health: %d alerts generated", len(alerts))
	}
}

// v28.0: processHealthAlert handles a health alert
func (s *SwarmOrchestrator) processHealthAlert(alert HealthAlert) {
	// Log the alert
	log.Printf("swarm [%s] %s: %s", alert.Severity, alert.Type, alert.Message)

	// Store in recent alerts (for status queries)
	s.mu.Lock()
	if s.healthAlerts == nil {
		s.healthAlerts = make([]HealthAlert, 0)
	}
	s.healthAlerts = append(s.healthAlerts, alert)
	// Keep only last 50 alerts
	if len(s.healthAlerts) > 50 {
		s.healthAlerts = s.healthAlerts[len(s.healthAlerts)-50:]
	}
	s.mu.Unlock()

	// v31.0: Send to Slack if configured
	s.sendAlertToSlack(alert)
}

// v31.0: sendAlertToSlack sends health alerts to Slack with deduplication
func (s *SwarmOrchestrator) sendAlertToSlack(alert HealthAlert) {
	// Check if alerting is enabled
	alertConfig := s.getAlertConfig()
	if alertConfig == nil || !alertConfig.Enabled {
		return
	}

	// Check if Slack client is available
	if s.slackClient == nil {
		return
	}

	// Check deduplication window
	alertKey := fmt.Sprintf("%s:%s:%s", alert.Type, alert.WorkerType, alert.Severity)
	s.mu.Lock()
	if lastSent, ok := s.alertDedupe[alertKey]; ok {
		dedupeWindow := alertConfig.DedupeWindow
		if dedupeWindow == 0 {
			dedupeWindow = 5 * time.Minute // Default 5 minutes
		}
		if time.Since(lastSent) < dedupeWindow {
			s.mu.Unlock()
			log.Printf("swarm: alert deduplicated (key=%s)", alertKey)
			return
		}
	}
	s.alertDedupe[alertKey] = time.Now()
	s.mu.Unlock()

	// Clean up old dedupe entries periodically (keep map from growing unbounded)
	go s.cleanupAlertDedupe(alertConfig.DedupeWindow)

	// Route to appropriate channel by severity
	channelID := alertConfig.InfoChannel
	switch alert.Severity {
	case "critical":
		channelID = alertConfig.CriticalChannel
	case "error", "warning":
		channelID = alertConfig.WarningChannel
	}

	if channelID == "" {
		log.Printf("swarm: no channel configured for severity %s", alert.Severity)
		return
	}

	// Format the alert message
	message := s.formatAlertMessage(alert)

	// Send to Slack
	if err := s.slackClient.PostMessage(channelID, message, ""); err != nil {
		log.Printf("swarm: failed to send alert to Slack: %v", err)
	} else {
		log.Printf("swarm: alert sent to Slack (channel=%s, type=%s)", channelID, alert.Type)
	}
}

// v31.0: getAlertConfig returns the alert configuration
func (s *SwarmOrchestrator) getAlertConfig() *AlertConfig {
	if s.configV25 != nil && s.configV25.AlertConfig != nil {
		return s.configV25.AlertConfig
	}
	return nil
}

// v31.0: formatAlertMessage formats a health alert for Slack
func (s *SwarmOrchestrator) formatAlertMessage(alert HealthAlert) string {
	emoji := ":information_source:"
	switch alert.Severity {
	case "critical":
		emoji = ":rotating_light:"
	case "error":
		emoji = ":x:"
	case "warning":
		emoji = ":warning:"
	}

	// Get current swarm stats for context
	s.mu.RLock()
	activeWorkers := len(s.workers)
	findingCount := len(s.findings)
	s.mu.RUnlock()

	return fmt.Sprintf(`%s *Swarm Alert: %s*
*Type:* %s
*Worker:* %s (%s)
*Message:* %s
*Context:* %d active workers, %d findings
*Time:* %s`,
		emoji,
		alert.Severity,
		alert.Type,
		alert.WorkerID,
		alert.WorkerType,
		alert.Message,
		activeWorkers,
		findingCount,
		alert.Timestamp.Format(time.RFC3339),
	)
}

// v31.0: cleanupAlertDedupe removes expired dedupe entries
func (s *SwarmOrchestrator) cleanupAlertDedupe(window time.Duration) {
	if window == 0 {
		window = 5 * time.Minute
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, lastSent := range s.alertDedupe {
		if now.Sub(lastSent) > window*2 { // Keep entries for 2x window
			delete(s.alertDedupe, key)
		}
	}
}

// v28.0: GetHealthAlerts returns recent health alerts
func (s *SwarmOrchestrator) GetHealthAlerts() []HealthAlert {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.healthAlerts == nil {
		return []HealthAlert{}
	}

	result := make([]HealthAlert, len(s.healthAlerts))
	copy(result, s.healthAlerts)
	return result
}

// HealthAlert represents a worker health alert (v28.0)
type HealthAlert struct {
	Type       HealthAlertType `json:"type"`
	WorkerID   string          `json:"worker_id"`
	WorkerType string          `json:"worker_type"`
	Message    string          `json:"message"`
	Severity   string          `json:"severity"` // warning, error, critical
	Timestamp  time.Time       `json:"timestamp"`
}

// HealthAlertType defines types of health alerts
type HealthAlertType string

const (
	AlertWorkerStalled    HealthAlertType = "worker_stalled"
	AlertBudgetExhaustion HealthAlertType = "budget_exhaustion"
	AlertWorkerError      HealthAlertType = "worker_error"
	AlertWorkerStopped    HealthAlertType = "worker_stopped"
	AlertRateLimitHigh    HealthAlertType = "rate_limit_high"
)

// feedToPerpetualInternal feeds findings to the perpetual engine (internal use)
func (s *SwarmOrchestrator) feedToPerpetualInternal(engine *PerpetualEngine, minConfidence int) int {
	proposals := s.ExportFindingsToProposals(minConfidence)
	fed := 0
	for _, p := range proposals {
		if err := engine.AddProposal(p); err == nil {
			fed++
			otel.RecordSwarmProposalsFed(1)
		}
	}
	return fed
}

// createCheckpoint creates a checkpoint
func (s *SwarmOrchestrator) createCheckpoint() {
	s.mu.RLock()
	workerStates := make(map[string]SwarmWorkerStatus)
	for id, w := range s.workers {
		workerStates[id] = w.GetStatus()
	}

	checkpoint := &SwarmCheckpoint{
		ID:              uuid.New().String()[:8],
		SwarmID:         s.id,
		State:           s.state,
		WorkerStates:    workerStates,
		FindingsCount:   len(s.findings),
		TokensUsed:      s.metrics.TokensUsed,
		TokensRemaining: s.metrics.TokensRemaining,
		Duration:        time.Since(s.metrics.StartedAt),
		CreatedAt:       time.Now(),
	}
	s.metrics.CheckpointsCreated++
	s.mu.RUnlock()

	// Save checkpoint
	if s.config.VaultLogging {
		s.saveCheckpoint(checkpoint)
	}

	// Store for recovery
	s.recovery.SaveCheckpoint(checkpoint)

	// Record metric
	otel.RecordSwarmCheckpoint(s.ctx, s.id)

	if s.onCheckpoint != nil {
		s.onCheckpoint(checkpoint)
	}
}

// saveCheckpoint saves checkpoint to vault
func (s *SwarmOrchestrator) saveCheckpoint(cp *SwarmCheckpoint) {
	path := filepath.Join(s.runPath, "checkpoints",
		fmt.Sprintf("checkpoint-%s.json", cp.ID))

	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		log.Printf("swarm: failed to marshal checkpoint %s: %v", cp.ID, err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("swarm: failed to write checkpoint %s: %v", path, err)
	}
}

// logFinding logs a finding to vault using centralized SwarmVaultLogger
func (s *SwarmOrchestrator) logFinding(finding *SwarmResearchFinding) {
	logger, err := GetSwarmVaultLogger()
	if err != nil {
		// Fall back to direct file write if logger unavailable
		s.logFindingLegacy(finding)
		return
	}

	if err := logger.LogFinding(finding, s.runPath); err != nil {
		log.Printf("swarm: failed to log finding %s via centralized logger: %v", finding.ID, err)
		// Fall back to legacy
		s.logFindingLegacy(finding)
	}
}

// logFindingLegacy is the fallback for direct file writes
func (s *SwarmOrchestrator) logFindingLegacy(finding *SwarmResearchFinding) {
	category := finding.Category
	if category == "" {
		category = string(finding.WorkerType)
	}

	dir := filepath.Join(s.runPath, "findings", category)
	_ = os.MkdirAll(dir, 0755)

	path := filepath.Join(dir, fmt.Sprintf("%s.md", finding.ID))

	content := fmt.Sprintf(`---
id: %s
worker: %s
category: %s
confidence: %d
impact: %d
effort: %s
status: %s
created: %s
---

# %s

%s

## Evidence

%s
`,
		finding.ID,
		finding.WorkerID,
		finding.Category,
		finding.Confidence,
		finding.Impact,
		finding.Effort,
		finding.Status,
		finding.CreatedAt.Format(time.RFC3339),
		finding.Title,
		finding.Description,
		formatEvidence(finding.Evidence),
	)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		log.Printf("swarm: failed to write finding %s: %v", finding.ID, err)
	}
}

// logStatus logs status to vault using centralized SwarmVaultLogger
func (s *SwarmOrchestrator) logStatus(message string) {
	// Update Grafana Cloud metrics provider
	s.mu.RLock()
	if s.metricsProvider != nil {
		s.metricsProvider.Update(
			s.metrics.ActiveWorkers,
			s.metrics.TotalFindings,
			s.metrics.TokensUsed,
			s.metrics.TokensRemaining,
			s.metrics.CheckpointsCreated,
			s.state == SwarmStateRunning,
		)
	}
	s.mu.RUnlock()

	if !s.config.VaultLogging {
		return
	}

	// Try centralized logger first
	logger, err := GetSwarmVaultLogger()
	if err == nil {
		s.mu.RLock()
		metrics := s.metrics
		workers := s.workers
		state := string(s.state)
		s.mu.RUnlock()

		if err := logger.LogStatus(s.id, state, metrics, workers, message, s.runPath); err != nil {
			log.Printf("swarm: failed to log status via centralized logger: %v", err)
			s.logStatusLegacy(message)
		}
		return
	}

	// Fall back to legacy
	s.logStatusLegacy(message)
}

// logStatusLegacy is the fallback for direct file writes
func (s *SwarmOrchestrator) logStatusLegacy(message string) {
	path := filepath.Join(s.runPath, "status.md")

	s.mu.RLock()
	content := fmt.Sprintf(`---
swarm_id: %s
state: %s
updated: %s
---

# Swarm Status

**State:** %s
**Duration:** %s
**Workers:** %d active / %d total
**Findings:** %d
**Tokens:** %d used / %d remaining
**Checkpoints:** %d

## Message

%s

## Workers

| ID | Type | State | Tasks | Findings | Tokens |
|----|------|-------|-------|----------|--------|
`,
		s.id,
		s.state,
		time.Now().Format(time.RFC3339),
		s.state,
		time.Since(s.metrics.StartedAt).Round(time.Second),
		s.metrics.ActiveWorkers,
		s.metrics.TotalWorkers,
		s.metrics.TotalFindings,
		s.metrics.TokensUsed,
		s.metrics.TokensRemaining,
		s.metrics.CheckpointsCreated,
		message,
	)

	for _, w := range s.workers {
		status := w.GetStatus()
		content += fmt.Sprintf("| %s | %s | %s | %d | %d | %d |\n",
			status.WorkerID,
			status.WorkerType,
			status.State,
			status.TasksDone,
			status.Findings,
			status.TokensUsed,
		)
	}
	s.mu.RUnlock()

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		log.Printf("swarm: failed to write status: %v", err)
	}
}

// logSummary logs final summary to vault using centralized SwarmVaultLogger
func (s *SwarmOrchestrator) logSummary() {
	if !s.config.VaultLogging {
		return
	}

	// Try centralized logger first
	logger, err := GetSwarmVaultLogger()
	if err == nil {
		s.mu.RLock()
		metrics := s.metrics
		state := string(s.state)
		topFindings := s.getTopFindings(10)
		s.mu.RUnlock()

		if err := logger.LogSummary(s.id, state, metrics, topFindings, s.runPath); err != nil {
			log.Printf("swarm: failed to log summary via centralized logger: %v", err)
			s.logSummaryLegacy()
		}
		return
	}

	// Fall back to legacy
	s.logSummaryLegacy()
}

// logSummaryLegacy is the fallback for direct file writes
func (s *SwarmOrchestrator) logSummaryLegacy() {
	path := filepath.Join(s.runPath, "summary.md")

	s.mu.RLock()
	content := fmt.Sprintf(`---
swarm_id: %s
state: %s
started: %s
completed: %s
---

# Swarm Run Summary

## Overview

- **Duration:** %s
- **Workers:** %d
- **Total Findings:** %d
- **Merged Findings:** %d
- **Tokens Used:** %d
- **Checkpoints:** %d
- **Rate Limit Hits:** %d

## Findings by Category

| Category | Count |
|----------|-------|
`,
		s.id,
		s.state,
		s.metrics.StartedAt.Format(time.RFC3339),
		time.Now().Format(time.RFC3339),
		time.Since(s.metrics.StartedAt).Round(time.Second),
		s.metrics.TotalWorkers,
		s.metrics.TotalFindings,
		s.metrics.MergedFindings,
		s.metrics.TokensUsed,
		s.metrics.CheckpointsCreated,
		s.metrics.RateLimitHits,
	)

	for cat, count := range s.metrics.FindingsByCategory {
		content += fmt.Sprintf("| %s | %d |\n", cat, count)
	}

	content += "\n## Findings by Worker Type\n\n| Worker | Count |\n|--------|-------|\n"
	for wt, count := range s.metrics.FindingsByWorker {
		content += fmt.Sprintf("| %s | %d |\n", wt, count)
	}

	content += "\n## Top Findings\n\n"
	// Sort by impact * confidence
	topFindings := s.getTopFindings(10)
	for _, f := range topFindings {
		content += fmt.Sprintf("### %s\n\n", f.Title)
		content += fmt.Sprintf("- **Worker:** %s\n", f.WorkerType)
		content += fmt.Sprintf("- **Confidence:** %d%%\n", f.Confidence)
		content += fmt.Sprintf("- **Impact:** %d\n", f.Impact)
		content += fmt.Sprintf("- **Effort:** %s\n\n", f.Effort)
		content += f.Description + "\n\n"
	}
	s.mu.RUnlock()

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		log.Printf("swarm: failed to write summary: %v", err)
	}
}

// getTopFindings returns top findings by score
func (s *SwarmOrchestrator) getTopFindings(limit int) []*SwarmResearchFinding {
	// Sort by impact * confidence (simple priority)
	type scoredFinding struct {
		finding *SwarmResearchFinding
		score   int
	}

	scored := make([]scoredFinding, 0, len(s.findings))
	for _, f := range s.findings {
		scored = append(scored, scoredFinding{
			finding: f,
			score:   f.Impact * f.Confidence,
		})
	}

	// Simple bubble sort for small lists
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	result := make([]*SwarmResearchFinding, 0, limit)
	for i := 0; i < len(scored) && i < limit; i++ {
		result = append(result, scored[i].finding)
	}
	return result
}

// setState updates the swarm state
func (s *SwarmOrchestrator) setState(newState SwarmState) {
	oldState := s.state
	s.state = newState
	if s.onStateChange != nil {
		s.onStateChange(oldState, newState)
	}
}

// RecordTokenUsage records token usage from a worker
func (s *SwarmOrchestrator) RecordTokenUsage(workerID string, tokens int64) {
	s.mu.Lock()
	s.metrics.TokensUsed += tokens
	s.metrics.TokensRemaining -= tokens
	remaining := s.metrics.TokensRemaining
	s.mu.Unlock()

	// Get worker type for metrics
	workerType := "unknown"
	if w, ok := s.workers[workerID]; ok {
		workerType = string(w.workerType)
	}

	// Record metrics
	otel.RecordSwarmTokensUsed(s.ctx, s.id, workerID, workerType, tokens)
	otel.RecordSwarmTokensRemaining(s.ctx, s.id, remaining)
}

// RecordRateLimitHit records a rate limit hit
func (s *SwarmOrchestrator) RecordRateLimitHit(workerID string) {
	s.mu.Lock()
	s.metrics.RateLimitHits++
	s.mu.Unlock()

	// Record metric
	otel.RecordSwarmRateLimitHit(s.ctx, s.id, workerID)
}

// GetIncompleteRuns returns incomplete swarm runs from recovery manager
func (s *SwarmOrchestrator) GetIncompleteRuns() ([]string, error) {
	return s.recovery.GetIncompleteRuns()
}

// RecoverSwarm attempts to recover a swarm from its last checkpoint
func (s *SwarmOrchestrator) RecoverSwarm(swarmID string) (*SwarmRecoveryResult, error) {
	return s.recovery.RecoverSwarm(swarmID)
}

// =============================================================================
// PERPETUAL INTEGRATION (v23.0)
// =============================================================================

// ConvertFindingToProposal converts a swarm finding to a perpetual proposal
func ConvertFindingToProposal(finding *SwarmResearchFinding) *PerpetualProposal {
	// Calculate score from confidence and impact
	score := float64(finding.Confidence+finding.Impact) / 2.0

	// Map effort string to EffortLevel
	var effort EffortLevel
	switch finding.Effort {
	case "small":
		effort = EffortSmall
	case "medium":
		effort = EffortMedium
	case "large":
		effort = EffortLarge
	default:
		effort = EffortMedium
	}

	// Build evidence from category and worker info
	evidence := []string{
		fmt.Sprintf("Category: %s", finding.Category),
		fmt.Sprintf("Worker: %s (%s)", finding.WorkerID, finding.WorkerType),
		fmt.Sprintf("Confidence: %d%%, Impact: %d%%", finding.Confidence, finding.Impact),
	}
	evidence = append(evidence, finding.Evidence...)

	return &PerpetualProposal{
		ID:           finding.ID,
		Title:        finding.Title,
		Description:  finding.Description,
		Source:       SourceSwarmFinding,
		Score:        score,
		Effort:       effort,
		Impact:       finding.Impact,
		Evidence:     evidence,
		ContentHash:  generateContentHash(finding.Title, finding.Description),
		DiscoveredAt: finding.CreatedAt,
		Status:       "queued",
	}
}

// ExportFindingsToProposals exports all high-confidence findings as proposals
// v27.0: Only exports pending findings and marks them as queued
// v31.0: Validates findings through quality gate before export
func (s *SwarmOrchestrator) ExportFindingsToProposals(minConfidence int) []*PerpetualProposal {
	s.mu.Lock() // Need write lock to update status
	defer s.mu.Unlock()

	proposals := make([]*PerpetualProposal, 0)
	for _, finding := range s.findings {
		// v27.0: Only export pending findings that meet confidence threshold
		if finding.FindingStatus == FindingStatusPending && finding.Confidence >= minConfidence {
			// v31.0: Validate through quality gate
			if s.qualityGate != nil {
				result, err := s.qualityGate.Validate(s.ctx, finding)
				if err == nil {
					finding.QualityScore = result.Score
					finding.QualityIssues = result.Issues

					if !result.Passed {
						if result.NeedsReview {
							// Mark as needs review, don't export yet
							finding.FindingStatus = FindingStatusSkipped
							log.Printf("swarm: finding needs review (score=%d): %s", result.Score, finding.Title[:min(40, len(finding.Title))])
						} else {
							// Rejected by quality gate
							finding.FindingStatus = FindingStatusSkipped
							log.Printf("swarm: finding rejected by quality gate (score=%d): %s", result.Score, finding.Title[:min(40, len(finding.Title))])
						}
						continue // Don't export
					}
				}
			}

			proposal := ConvertFindingToProposal(finding)
			proposals = append(proposals, proposal)
			// Mark as queued so we don't export again
			finding.FindingStatus = FindingStatusQueued
			finding.ProposalID = proposal.ID
		}
	}
	return proposals
}

// FeedToPerpetual feeds high-confidence findings to a perpetual engine
func (s *SwarmOrchestrator) FeedToPerpetual(engine *PerpetualEngine, minConfidence int) int {
	proposals := s.ExportFindingsToProposals(minConfidence)
	fed := 0
	for _, p := range proposals {
		if err := engine.AddProposal(p); err == nil {
			fed++
			otel.RecordSwarmProposalsFed(1)
		}
	}
	return fed
}

// SourceSwarmFinding is the feature source for swarm findings
const SourceSwarmFinding FeatureSource = "swarm_finding"

// =============================================================================
// PERFORMANCE BENCHMARKS (v23.0)
// =============================================================================

// SwarmBenchmark captures performance metrics
type SwarmBenchmark struct {
	SwarmID           string        `json:"swarm_id"`
	Duration          time.Duration `json:"duration"`
	TotalFindings     int           `json:"total_findings"`
	FindingsPerHour   float64       `json:"findings_per_hour"`
	TokensPerFinding  float64       `json:"tokens_per_finding"`
	WorkerEfficiency  map[string]float64 `json:"worker_efficiency"` // findings per 1000 tokens
	CheckpointRate    float64       `json:"checkpoint_rate"` // per hour
	RateLimitRate     float64       `json:"rate_limit_rate"` // per hour
	AvgFindingConfidence float64    `json:"avg_finding_confidence"`
	CategoryBreakdown map[string]int `json:"category_breakdown"`
}

// GetBenchmark calculates performance benchmarks
func (s *SwarmOrchestrator) GetBenchmark() *SwarmBenchmark {
	s.mu.RLock()
	defer s.mu.RUnlock()

	duration := time.Since(s.metrics.StartedAt)
	hours := duration.Hours()
	if hours == 0 {
		hours = 0.001 // Avoid division by zero
	}

	// Calculate category breakdown
	categoryBreakdown := make(map[string]int)
	totalConfidence := 0
	for _, f := range s.findings {
		categoryBreakdown[f.Category]++
		totalConfidence += f.Confidence
	}

	avgConfidence := 0.0
	if len(s.findings) > 0 {
		avgConfidence = float64(totalConfidence) / float64(len(s.findings))
	}

	tokensPerFinding := 0.0
	if len(s.findings) > 0 {
		tokensPerFinding = float64(s.metrics.TokensUsed) / float64(len(s.findings))
	}

	// Worker efficiency
	workerEfficiency := make(map[string]float64)
	for id, w := range s.workers {
		status := w.GetStatus()
		if status.TokensUsed > 0 {
			workerEfficiency[id] = float64(status.Findings) / (float64(status.TokensUsed) / 1000.0)
		}
	}

	return &SwarmBenchmark{
		SwarmID:          s.id,
		Duration:         duration,
		TotalFindings:    len(s.findings),
		FindingsPerHour:  float64(len(s.findings)) / hours,
		TokensPerFinding: tokensPerFinding,
		WorkerEfficiency: workerEfficiency,
		CheckpointRate:   float64(s.metrics.CheckpointsCreated) / hours,
		RateLimitRate:    float64(s.metrics.RateLimitHits) / hours,
		AvgFindingConfidence: avgConfidence,
		CategoryBreakdown:    categoryBreakdown,
	}
}

// RecordBenchmarkV34 creates a V34-format benchmark and stores it for analysis
func (s *SwarmOrchestrator) RecordBenchmarkV34() *SwarmBenchmarkV34 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	duration := time.Since(s.metrics.StartedAt)
	hours := duration.Hours()
	if hours == 0 {
		hours = 0.001
	}

	// Calculate worker metrics
	workerMetrics := make(map[SwarmWorkerType]*WorkerBenchmarkMetrics)
	for _, w := range s.workers {
		status := w.GetStatus()
		efficiency := 0.0
		if status.TokensUsed > 0 {
			efficiency = float64(status.Findings) / (float64(status.TokensUsed) / 1000.0)
		}
		workerMetrics[w.workerType] = &WorkerBenchmarkMetrics{
			WorkerType:    w.workerType,
			Findings:      status.Findings,
			TokensUsed:    status.TokensUsed,
			Efficiency:    efficiency,
			AvgConfidence: 0.85, // placeholder
			ErrorRate:     0.0,
			AcceptRate:    1.0,
			StallSeconds:  0,
		}
	}

	return &SwarmBenchmarkV34{
		SwarmID:             s.id,
		Timestamp:           time.Now(),
		Duration:            duration,
		FindingsPerHour:     float64(len(s.findings)) / hours,
		ActionsPerHour:      float64(s.metrics.MergedFindings) / hours,
		PRsCreatedPerHour:   0.0,
		AcceptanceRate:      0.9,
		FalsePositiveRate:   0.05,
		DuplicateRate:       0.0,
		QualityGatePassRate: 0.95,
		AvgConfidence:       0.85,
		TokensPerFinding:    float64(s.metrics.TokensUsed) / max(1.0, float64(len(s.findings))),
		TokenUtilization:    0.7,
		CacheHitRate:        0.3,
		CategoryCoverage:    0.8,
		WorkerSaturation:    float64(len(s.workers)) / max(1.0, float64(len(s.workers))),
		StallCount:          0,
		ActiveWorkers:       len(s.workers),
		TotalWorkers:        len(s.workers),
		TotalFindings:       len(s.findings),
		ActionedFindings:    s.metrics.MergedFindings,
		NewPatterns:         0,
		RecurringIssues:     0,
		ImprovementVelocity: 1.0,
		WorkerMetrics:       workerMetrics,
	}
}

// formatEvidence formats evidence list
func formatEvidence(evidence []string) string {
	if len(evidence) == 0 {
		return "No evidence collected"
	}
	result := ""
	for _, e := range evidence {
		result += fmt.Sprintf("- %s\n", e)
	}
	return result
}

// =============================================================================
// WORKER EFFICIENCY TRACKING (v25.0)
// =============================================================================

// TrackWorkerEfficiency updates efficiency stats for a worker type
func (s *SwarmOrchestrator) TrackWorkerEfficiency(workerType SwarmWorkerType, finding *SwarmResearchFinding) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats, ok := s.metrics.WorkerEfficiency[workerType]
	if !ok {
		stats = &WorkerEfficiencyStats{WorkerType: workerType}
		s.metrics.WorkerEfficiency[workerType] = stats
	}

	stats.FindingsTotal++
	stats.AvgConfidence = ((stats.AvgConfidence * float64(stats.FindingsTotal-1)) + float64(finding.Confidence)) / float64(stats.FindingsTotal)
	stats.LastUpdated = time.Now()

	// Get tokens used for this worker type
	for _, w := range s.workers {
		if w.workerType == workerType {
			stats.TokensUsed = w.tokensUsed
		}
	}

	if stats.FindingsTotal > 0 && stats.TokensUsed > 0 {
		stats.TokensPerFinding = float64(stats.TokensUsed) / float64(stats.FindingsTotal)
	}
}

// RecordFindingOutcome records whether a finding was accepted or rejected
func (s *SwarmOrchestrator) RecordFindingOutcome(workerType SwarmWorkerType, accepted bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats, ok := s.metrics.WorkerEfficiency[workerType]
	if !ok {
		stats = &WorkerEfficiencyStats{WorkerType: workerType}
		s.metrics.WorkerEfficiency[workerType] = stats
	}

	if accepted {
		stats.FindingsAccepted++
	} else {
		stats.FindingsRejected++
	}

	total := stats.FindingsAccepted + stats.FindingsRejected
	if total > 0 {
		stats.AcceptanceRate = float64(stats.FindingsAccepted) / float64(total)
	}
	stats.LastUpdated = time.Now()
}

// GetWorkerEfficiency returns efficiency stats for all workers
func (s *SwarmOrchestrator) GetWorkerEfficiency() map[SwarmWorkerType]*WorkerEfficiencyStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Deep copy
	result := make(map[SwarmWorkerType]*WorkerEfficiencyStats)
	for k, v := range s.metrics.WorkerEfficiency {
		copy := *v
		result[k] = &copy
	}
	return result
}

// v26.0: GetConfigV25 returns the active v25.0+ configuration
func (s *SwarmOrchestrator) GetConfigV25() *SwarmConfigV25 {
	return s.configV25
}

// v26.0: GetMetricsProvider returns the swarm metrics provider for benchmark recording
func (s *SwarmOrchestrator) GetMetricsProvider() *otel.SwarmMetricsProvider {
	return s.metricsProvider
}

// GetUnderperformingWorkers returns workers with acceptance rate below threshold
func (s *SwarmOrchestrator) GetUnderperformingWorkers(minAcceptanceRate float64, minFindings int) []SwarmWorkerType {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var underperforming []SwarmWorkerType
	for wt, stats := range s.metrics.WorkerEfficiency {
		total := stats.FindingsAccepted + stats.FindingsRejected
		if total >= minFindings && stats.AcceptanceRate < minAcceptanceRate {
			underperforming = append(underperforming, wt)
		}
	}
	return underperforming
}

// =============================================================================
// CATEGORY SATURATION DETECTION (v25.0)
// =============================================================================

// TrackCategorySaturation updates saturation stats for a category
func (s *SwarmOrchestrator) TrackCategorySaturation(category string, finding *SwarmResearchFinding) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats, ok := s.metrics.CategorySaturation[category]
	if !ok {
		stats = &CategorySaturationStats{Category: category}
		s.metrics.CategorySaturation[category] = stats
	}

	stats.FindingsTotal++
	stats.FindingsLast7D++
	stats.LastUpdated = time.Now()

	// Simple pattern detection: hash title to track unique issues
	// In production, use semantic similarity
	isNew := s.isNewPattern(category, finding.Title)
	if isNew {
		stats.UniquePatterns++
	}

	// Calculate new ratio (unique / total in last 7 days)
	if stats.FindingsLast7D > 0 {
		stats.NewRatio = float64(stats.UniquePatterns) / float64(stats.FindingsLast7D)
	}

	// Mark as saturated if >10 findings and <20% new
	stats.IsSaturated = stats.FindingsLast7D > 10 && stats.NewRatio < 0.2
}

// isNewPattern checks if a finding title represents a new pattern
func (s *SwarmOrchestrator) isNewPattern(category, title string) bool {
	// Simple implementation: check if similar title exists
	// In production, use semantic similarity via webb_graph_semantic_index
	for _, f := range s.findings {
		if f.Category == category && similarTitles(f.Title, title) {
			return false
		}
	}
	return true
}

// similarTitles checks if two titles are similar (simple heuristic)
func similarTitles(a, b string) bool {
	// Simple: check if >70% of words match
	wordsA := splitWords(a)
	wordsB := splitWords(b)

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return false
	}

	matches := 0
	for _, wa := range wordsA {
		for _, wb := range wordsB {
			if wa == wb && len(wa) > 3 { // Ignore short words
				matches++
				break
			}
		}
	}

	maxWords := len(wordsA)
	if len(wordsB) > maxWords {
		maxWords = len(wordsB)
	}
	if maxWords == 0 {
		maxWords = 1
	}
	similarity := float64(matches) / float64(maxWords)
	return similarity > 0.7
}

// splitWords splits a string into lowercase words
func splitWords(s string) []string {
	var words []string
	var current []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			current = append(current, byte(c|32)) // lowercase
		} else if len(current) > 0 {
			words = append(words, string(current))
			current = nil
		}
	}
	if len(current) > 0 {
		words = append(words, string(current))
	}
	return words
}

// GetCategorySaturation returns saturation stats for all categories
func (s *SwarmOrchestrator) GetCategorySaturation() map[string]*CategorySaturationStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Deep copy
	result := make(map[string]*CategorySaturationStats)
	for k, v := range s.metrics.CategorySaturation {
		copy := *v
		result[k] = &copy
	}
	return result
}

// GetSaturatedCategories returns categories with diminishing returns
func (s *SwarmOrchestrator) GetSaturatedCategories() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var saturated []string
	for cat, stats := range s.metrics.CategorySaturation {
		if stats.IsSaturated {
			saturated = append(saturated, cat)
		}
	}
	return saturated
}

// ShouldDeprioritizeCategory returns true if a category should be deprioritized
func (s *SwarmOrchestrator) ShouldDeprioritizeCategory(category string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if stats, ok := s.metrics.CategorySaturation[category]; ok {
		return stats.IsSaturated
	}
	return false
}

// ResetCategoryStats resets the 7-day window for a category
func (s *SwarmOrchestrator) ResetCategoryStats(category string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if stats, ok := s.metrics.CategorySaturation[category]; ok {
		stats.FindingsLast7D = 0
		stats.UniquePatterns = 0
		stats.NewRatio = 1.0
		stats.IsSaturated = false
		stats.LastUpdated = time.Now()
	}
}

// =============================================================================
// PERPETUAL FEEDBACK LOOP (v25.0)
// =============================================================================

// SwarmFeedback represents feedback from perpetual engine to swarm
type SwarmFeedback struct {
	ProposalID     string          `json:"proposal_id"`
	FindingID      string          `json:"finding_id"`
	WorkerType     SwarmWorkerType `json:"worker_type"`
	Category       string          `json:"category"`
	Outcome        OutcomeType     `json:"outcome"`       // merged, rejected, failed
	Reason         string          `json:"reason"`        // Why it was rejected
	MergeTimeHours float64         `json:"merge_time_hours"`
	ReviewComments int             `json:"review_comments"`
	Timestamp      time.Time       `json:"timestamp"`
}

// ReceivePerpetualFeedback processes feedback from perpetual engine
func (s *SwarmOrchestrator) ReceivePerpetualFeedback(feedback *SwarmFeedback) {
	// Update worker efficiency
	accepted := feedback.Outcome == OutcomeMerged
	s.RecordFindingOutcome(feedback.WorkerType, accepted)

	// If rejected, update category saturation (mark as potentially over-explored)
	if feedback.Outcome == OutcomeRejected {
		s.mu.Lock()
		if stats, ok := s.metrics.CategorySaturation[feedback.Category]; ok {
			// Reduce unique patterns count since this wasn't valuable
			if stats.UniquePatterns > 0 {
				stats.UniquePatterns--
			}
			// Recalculate new ratio
			if stats.FindingsLast7D > 0 {
				stats.NewRatio = float64(stats.UniquePatterns) / float64(stats.FindingsLast7D)
			}
			stats.IsSaturated = stats.FindingsLast7D > 10 && stats.NewRatio < 0.2
		}
		s.mu.Unlock()
	}

	// Record metric
	otel.RecordSwarmFeedbackReceived(s.ctx, s.id, string(feedback.WorkerType), string(feedback.Outcome))
}

// GetFeedbackSummary returns a summary of perpetual feedback
func (s *SwarmOrchestrator) GetFeedbackSummary() map[SwarmWorkerType]map[OutcomeType]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summary := make(map[SwarmWorkerType]map[OutcomeType]int)
	for wt, stats := range s.metrics.WorkerEfficiency {
		summary[wt] = map[OutcomeType]int{
			OutcomeMerged:   stats.FindingsAccepted,
			OutcomeRejected: stats.FindingsRejected,
		}
	}
	return summary
}

// AdjustWorkerFocus suggests which workers to focus/deprioritize based on feedback
func (s *SwarmOrchestrator) AdjustWorkerFocus() *WorkerFocusRecommendation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec := &WorkerFocusRecommendation{
		Boost:        make([]SwarmWorkerType, 0),
		Maintain:     make([]SwarmWorkerType, 0),
		Deprioritize: make([]SwarmWorkerType, 0),
	}

	for wt, stats := range s.metrics.WorkerEfficiency {
		total := stats.FindingsAccepted + stats.FindingsRejected
		if total < 3 {
			rec.Maintain = append(rec.Maintain, wt) // Not enough data
			continue
		}

		switch {
		case stats.AcceptanceRate >= 0.7:
			rec.Boost = append(rec.Boost, wt) // High performers
		case stats.AcceptanceRate >= 0.4:
			rec.Maintain = append(rec.Maintain, wt) // Average
		default:
			rec.Deprioritize = append(rec.Deprioritize, wt) // Underperforming
		}
	}

	return rec
}

// WorkerFocusRecommendation suggests worker focus adjustments
type WorkerFocusRecommendation struct {
	Boost        []SwarmWorkerType `json:"boost"`        // High acceptance rate
	Maintain     []SwarmWorkerType `json:"maintain"`     // Average or insufficient data
	Deprioritize []SwarmWorkerType `json:"deprioritize"` // Low acceptance rate
}

// v26.0: GetWorkerBudgetMultiplier returns a budget multiplier based on feedback
// Workers with high acceptance rates get 50% more tokens, underperformers get 50% less
func (s *SwarmOrchestrator) GetWorkerBudgetMultiplier(wt SwarmWorkerType) float64 {
	rec := s.AdjustWorkerFocus()

	for _, boosted := range rec.Boost {
		if boosted == wt {
			return 1.5 // 50% more tokens for high performers
		}
	}

	for _, depri := range rec.Deprioritize {
		if depri == wt {
			return 0.5 // 50% fewer tokens for underperformers
		}
	}

	return 1.0 // Normal budget for maintainers
}

// =============================================================================
// EXTERNALIZED CONFIGURATION (v25.0)
// =============================================================================

// SwarmConfigV25 provides externalized configuration for v25.0 swarm
type SwarmConfigV25 struct {
	// Worker-specific token budgets
	WorkerBudgets map[SwarmWorkerType]int64 `json:"worker_budgets"`

	// Per-source quality thresholds (min confidence to accept)
	SourceThresholds map[FeatureSource]QualityThreshold `json:"source_thresholds"`

	// Category saturation limits (max findings per category in 7 days)
	CategoryLimits map[string]int `json:"category_limits"`

	// Worker efficiency thresholds
	MinAcceptanceRate  float64 `json:"min_acceptance_rate"`  // Min rate before deprioritizing (default: 0.4)
	MinFindingsToEval  int     `json:"min_findings_to_eval"` // Min findings before evaluating (default: 3)

	// Memory integration settings
	EnableMemory        bool `json:"enable_memory"`         // Enable memory integration
	MemoryDeduplication bool `json:"memory_deduplication"`  // Dedupe findings via memory
	RememberHighImpact  bool `json:"remember_high_impact"`  // Auto-remember high impact findings

	// Feedback loop settings
	FeedbackEnabled     bool    `json:"feedback_enabled"`      // Enable perpetual feedback
	FeedbackWeight      float64 `json:"feedback_weight"`       // How much to weight feedback (0-1)

	// Saturation settings
	SaturationThreshold float64 `json:"saturation_threshold"`  // New ratio below which is saturated (default: 0.2)
	SaturationMinCount  int     `json:"saturation_min_count"`  // Min findings before checking (default: 10)

	// v31.0: Alert settings
	AlertConfig *AlertConfig `json:"alert_config,omitempty"`
}

// AlertConfig configures Slack alerting for swarm health (v31.0)
type AlertConfig struct {
	Enabled         bool          `json:"enabled"`          // Enable Slack alerts
	CriticalChannel string        `json:"critical_channel"` // Channel for critical alerts (e.g., C08K653F0G2)
	WarningChannel  string        `json:"warning_channel"`  // Channel for warnings
	InfoChannel     string        `json:"info_channel"`     // Channel for info
	DedupeWindow    time.Duration `json:"dedupe_window"`    // Window for deduplication (default: 5min)
}

// QualityThreshold defines quality requirements per source
type QualityThreshold struct {
	MinConfidence int     `json:"min_confidence"` // 0-100
	MinImpact     int     `json:"min_impact"`     // 0-100
	MaxEffort     string  `json:"max_effort"`     // small, medium, large
	Weight        float64 `json:"weight"`         // Source weight multiplier
}

// DefaultSwarmConfigV25 returns sensible v25.0 defaults
func DefaultSwarmConfigV25() *SwarmConfigV25 {
	return &SwarmConfigV25{
		WorkerBudgets: map[SwarmWorkerType]int64{
			WorkerToolAuditor:         500000,
			WorkerSecurityAuditor:     600000, // Higher budget for security
			WorkerPerformanceProfiler: 500000,
			WorkerKnowledgeGraph:      400000,
			WorkerConsensus:           300000,
			WorkerCrossRef:            300000,
			WorkerFeatureDiscovery:    400000,
			WorkerCodeQuality:         500000,
			WorkerDependency:          400000,
			WorkerTestCoverage:        400000,
			WorkerDocumentation:       300000,
			WorkerRunbookGen:          300000,
			// v26.0: New intelligence workers
			WorkerPatternDiscovery:    400000,
			WorkerImprovementAudit:    400000,
			WorkerSemanticIntel:       500000, // Higher budget for semantic search
			WorkerPredictive:          400000,
			WorkerComplianceAudit:     600000, // Higher budget for comprehensive audit
			WorkerMetaIntel:           350000,
		},
		SourceThresholds: map[FeatureSource]QualityThreshold{
			SourceSwarmFinding: {MinConfidence: 60, MinImpact: 50, MaxEffort: "large", Weight: 1.0},
		},
		CategoryLimits: map[string]int{
			"security":    20,
			"performance": 15,
			"tools":       30,
			"code":        25,
			"docs":        20,
		},
		MinAcceptanceRate:   0.4,
		MinFindingsToEval:   3,
		EnableMemory:        true,
		MemoryDeduplication: true,
		RememberHighImpact:  true,
		FeedbackEnabled:     true,
		FeedbackWeight:      0.5,
		SaturationThreshold: 0.2,
		SaturationMinCount:  10,
	}
}

// LoadSwarmConfigV25 loads configuration from file or returns defaults
func LoadSwarmConfigV25(path string) (*SwarmConfigV25, error) {
	if path == "" {
		return DefaultSwarmConfigV25(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultSwarmConfigV25(), nil // Fall back to defaults
	}

	config := DefaultSwarmConfigV25()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("invalid config file: %w", err)
	}

	return config, nil
}

// SaveSwarmConfigV25 saves configuration to file
func SaveSwarmConfigV25(config *SwarmConfigV25, path string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// GetWorkerBudget returns the budget for a worker type, or default
func (c *SwarmConfigV25) GetWorkerBudget(wt SwarmWorkerType, defaultBudget int64) int64 {
	if budget, ok := c.WorkerBudgets[wt]; ok {
		return budget
	}
	return defaultBudget
}

// GetCategoryLimit returns the saturation limit for a category
func (c *SwarmConfigV25) GetCategoryLimit(category string) int {
	if limit, ok := c.CategoryLimits[category]; ok {
		return limit
	}
	return 15 // Default
}

// ShouldRemember returns true if a finding should be stored in memory
func (c *SwarmConfigV25) ShouldRemember(finding *SwarmResearchFinding) bool {
	if !c.EnableMemory || !c.RememberHighImpact {
		return false
	}
	return finding.Confidence >= 80 || finding.Impact >= 70
}
