// Package clients provides API clients for webb.
// v36.0: Self-Improving Perpetual Development Loop
// Synthetic Engineering Team - autonomous end-to-end development cycle
package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// DEVELOPMENT PHASES
// =============================================================================

// DevPhase represents a phase in the perpetual development cycle
type DevPhase string

const (
	PhaseDiscovery      DevPhase = "discovery"      // Swarm finds issues/opportunities
	PhasePlanning       DevPhase = "planning"       // Prioritize and plan fixes
	PhaseImplementation DevPhase = "implementation" // Create PRs and implementations
	PhaseReview         DevPhase = "review"         // Validate, test, and merge
	PhaseLearning       DevPhase = "learning"       // Update benchmarks, self-improve
)

// PhaseStatus tracks the status of a development phase
type PhaseStatus struct {
	Phase       DevPhase     `json:"phase"`
	Status      string       `json:"status"` // pending, in_progress, completed, failed
	StartedAt   *time.Time   `json:"started_at,omitempty"`
	CompletedAt *time.Time   `json:"completed_at,omitempty"`
	Duration    time.Duration `json:"duration,omitempty"`
	Metrics     PhaseMetrics `json:"metrics"`
	Error       string       `json:"error,omitempty"`
}

// PhaseMetrics tracks metrics for each phase
type PhaseMetrics struct {
	ItemsProcessed int     `json:"items_processed"`
	ItemsSucceeded int     `json:"items_succeeded"`
	ItemsFailed    int     `json:"items_failed"`
	TokensUsed     int64   `json:"tokens_used"`
	SuccessRate    float64 `json:"success_rate"`
}

// =============================================================================
// SYNTHETIC ENGINEERING TEAM
// =============================================================================

// SyntheticEngineeringTeam represents an autonomous development team
type SyntheticEngineeringTeam struct {
	id       string
	name     string
	config   *DevLoopConfig
	swarm    *SwarmOrchestrator

	// Current cycle state
	cycleID       string
	currentPhase  DevPhase
	phaseHistory  []*PhaseStatus
	cycleMetrics  *CycleMetrics

	// Cumulative benchmarks
	benchmarks    *TeamBenchmarks
	improvements  []*SelfImprovement

	// Self-improvement state
	workerAdjustments map[SwarmWorkerType]float64 // Budget multipliers
	priorityRules     []*PriorityRule
	qualityThresholds *QualityThresholds

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
	wg     sync.WaitGroup

	// Persistence
	vaultPath string
}

// DevLoopConfig configures the perpetual development loop
type DevLoopConfig struct {
	// Cycle timing
	CycleDuration      time.Duration `json:"cycle_duration"`       // Duration per full cycle (default: 4h)
	PhaseTimeout       time.Duration `json:"phase_timeout"`        // Max time per phase (default: 1h)
	LearningInterval   time.Duration `json:"learning_interval"`    // How often to self-improve (default: 30m)

	// Thresholds
	MinConfidence      int     `json:"min_confidence"`       // Minimum finding confidence to action
	MinImpact          int     `json:"min_impact"`           // Minimum impact to prioritize
	MaxConcurrentPRs   int     `json:"max_concurrent_prs"`   // Max PRs in flight
	AutoMergeThreshold float64 `json:"auto_merge_threshold"` // Auto-merge if review score above this

	// Self-improvement
	BudgetAdjustmentRate  float64 `json:"budget_adjustment_rate"`  // How much to adjust budgets (0.1 = 10%)
	PerformanceWindow     int     `json:"performance_window"`      // Cycles to consider for averages
	ImprovementThreshold  float64 `json:"improvement_threshold"`   // Trigger improvement if below this

	// Integration
	EnablePRCreation   bool `json:"enable_pr_creation"`
	EnableAutoMerge    bool `json:"enable_auto_merge"`
	EnableSlackReports bool `json:"enable_slack_reports"`
	EnableRoadmapSync  bool `json:"enable_roadmap_sync"`
}

// DefaultDevLoopConfig returns sensible defaults
func DefaultDevLoopConfig() *DevLoopConfig {
	return &DevLoopConfig{
		CycleDuration:        4 * time.Hour,
		PhaseTimeout:         1 * time.Hour,
		LearningInterval:     30 * time.Minute,
		MinConfidence:        70,
		MinImpact:            50,
		MaxConcurrentPRs:     5,
		AutoMergeThreshold:   0.90,
		BudgetAdjustmentRate: 0.15,
		PerformanceWindow:    10,
		ImprovementThreshold: 0.60,
		EnablePRCreation:     false, // Safe default
		EnableAutoMerge:      false,
		EnableSlackReports:   true,
		EnableRoadmapSync:    true,
	}
}

// CycleMetrics tracks metrics for a complete development cycle
type CycleMetrics struct {
	CycleID        string        `json:"cycle_id"`
	CycleNumber    int           `json:"cycle_number"`
	StartedAt      time.Time     `json:"started_at"`
	CompletedAt    *time.Time    `json:"completed_at,omitempty"`
	Duration       time.Duration `json:"duration,omitempty"`

	// Phase breakdown
	Phases         map[DevPhase]*PhaseMetrics `json:"phases"`

	// Aggregate metrics
	FindingsDiscovered int     `json:"findings_discovered"`
	FindingsActioned   int     `json:"findings_actioned"`
	PRsCreated         int     `json:"prs_created"`
	PRsMerged          int     `json:"prs_merged"`
	PRsRejected        int     `json:"prs_rejected"`
	TokensUsed         int64   `json:"tokens_used"`

	// Quality metrics
	AcceptanceRate     float64 `json:"acceptance_rate"`
	MergeRate          float64 `json:"merge_rate"`
	AvgFindingQuality  float64 `json:"avg_finding_quality"`

	// Self-improvement metrics
	ImprovementsApplied int     `json:"improvements_applied"`
	BenchmarkDelta      float64 `json:"benchmark_delta"` // Change from previous cycle
}

// TeamBenchmarks tracks cumulative team performance
type TeamBenchmarks struct {
	// Velocity metrics
	CyclesCompleted    int     `json:"cycles_completed"`
	TotalFindings      int     `json:"total_findings"`
	TotalPRsCreated    int     `json:"total_prs_created"`
	TotalPRsMerged     int     `json:"total_prs_merged"`
	AvgFindingsPerHour float64 `json:"avg_findings_per_hour"`
	AvgPRsPerCycle     float64 `json:"avg_prs_per_cycle"`

	// Quality metrics
	OverallAcceptanceRate float64 `json:"overall_acceptance_rate"`
	OverallMergeRate      float64 `json:"overall_merge_rate"`
	AvgConfidence         float64 `json:"avg_confidence"`
	AvgImpact             float64 `json:"avg_impact"`
	FalsePositiveRate     float64 `json:"false_positive_rate"`
	DuplicateRate         float64 `json:"duplicate_rate"`

	// Efficiency metrics
	AvgTokensPerFinding float64 `json:"avg_tokens_per_finding"`
	AvgTokensPerPR      float64 `json:"avg_tokens_per_pr"`
	TokenUtilization    float64 `json:"token_utilization"`

	// Improvement tracking
	ImprovementVelocity float64 `json:"improvement_velocity"` // Improvements per cycle
	BenchmarkTrend      string  `json:"benchmark_trend"`      // improving, stable, declining

	// Worker performance
	TopWorkers     []WorkerRanking `json:"top_workers"`
	BottomWorkers  []WorkerRanking `json:"bottom_workers"`

	LastUpdated    time.Time `json:"last_updated"`
}

// WorkerRanking ranks a worker by performance
type WorkerRanking struct {
	WorkerType     SwarmWorkerType `json:"worker_type"`
	Score          float64         `json:"score"`
	AcceptanceRate float64         `json:"acceptance_rate"`
	TokenEfficiency float64        `json:"token_efficiency"`
	Rank           int             `json:"rank"`
}

// SelfImprovement represents an improvement applied by the team
type SelfImprovement struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"` // budget_adjustment, priority_change, threshold_update, worker_config
	Description string    `json:"description"`
	AppliedAt   time.Time `json:"applied_at"`
	Impact      string    `json:"impact"` // Expected impact description
	Metrics     struct {
		Before float64 `json:"before"`
		After  float64 `json:"after"`
		Delta  float64 `json:"delta"`
	} `json:"metrics"`
}

// PriorityRule defines how to prioritize findings
type PriorityRule struct {
	Name       string   `json:"name"`
	Condition  string   `json:"condition"` // Category, confidence, impact thresholds
	Priority   int      `json:"priority"`  // 1 = highest
	Categories []string `json:"categories,omitempty"`
	MinConf    int      `json:"min_conf,omitempty"`
	MinImpact  int      `json:"min_impact,omitempty"`
}

// QualityThresholds defines quality gates
type QualityThresholds struct {
	MinConfidence     int     `json:"min_confidence"`
	MinImpact         int     `json:"min_impact"`
	MaxDuplicateRate  float64 `json:"max_duplicate_rate"`
	MinAcceptanceRate float64 `json:"min_acceptance_rate"`
	MinMergeRate      float64 `json:"min_merge_rate"`
}

// =============================================================================
// CONSTRUCTOR & LIFECYCLE
// =============================================================================

// NewSyntheticEngineeringTeam creates a new synthetic engineering team
func NewSyntheticEngineeringTeam(name string, config *DevLoopConfig, swarm *SwarmOrchestrator) *SyntheticEngineeringTeam {
	if config == nil {
		config = DefaultDevLoopConfig()
	}

	// Auto-scale PhaseTimeout based on CycleDuration if not explicitly set
	// Each of 5 phases gets ~20% of cycle time, but discovery uses half of phase timeout
	// So PhaseTimeout should be ~40% of CycleDuration to give discovery ~20%
	if config.PhaseTimeout == 0 || config.PhaseTimeout > config.CycleDuration/2 {
		config.PhaseTimeout = config.CycleDuration * 2 / 5 // 40% of cycle for longest phase
		log.Printf("devloop: Auto-scaled PhaseTimeout to %v (from CycleDuration %v)", config.PhaseTimeout, config.CycleDuration)
	}

	homeDir, _ := os.UserHomeDir()
	vaultPath := filepath.Join(homeDir, "webb-vault", "devloop")

	team := &SyntheticEngineeringTeam{
		id:                uuid.New().String()[:8],
		name:              name,
		config:            config,
		swarm:             swarm,
		phaseHistory:      make([]*PhaseStatus, 0),
		workerAdjustments: make(map[SwarmWorkerType]float64),
		priorityRules:     defaultPriorityRules(),
		qualityThresholds: &QualityThresholds{
			MinConfidence:     70,
			MinImpact:         50,
			MaxDuplicateRate:  0.20,
			MinAcceptanceRate: 0.60,
			MinMergeRate:      0.50,
		},
		vaultPath: vaultPath,
	}

	// Load existing benchmarks
	team.loadBenchmarks()

	return team
}

// defaultPriorityRules returns default priority rules
func defaultPriorityRules() []*PriorityRule {
	return []*PriorityRule{
		{Name: "security-critical", Priority: 1, Categories: []string{"security-gap", "security-finding"}, MinConf: 80},
		{Name: "high-impact", Priority: 2, MinConf: 80, MinImpact: 70},
		{Name: "consensus-validated", Priority: 3, Categories: []string{"consensus-validated", "cross-worker-consensus"}},
		{Name: "quick-wins", Priority: 4, MinConf: 70, MinImpact: 40},
		{Name: "default", Priority: 5},
	}
}

// Start begins the perpetual development loop
func (t *SyntheticEngineeringTeam) Start(ctx context.Context) error {
	t.mu.Lock()
	t.ctx, t.cancel = context.WithCancel(ctx)
	t.mu.Unlock()

	log.Printf("devloop: Starting Synthetic Engineering Team '%s' (ID: %s)", t.name, t.id)

	// Create vault directory
	os.MkdirAll(t.vaultPath, 0755)

	// Start the main loop
	t.wg.Add(1)
	go t.perpetualLoop()

	// Start the learning loop
	t.wg.Add(1)
	go t.learningLoop()

	// Start the benchmark reporter
	t.wg.Add(1)
	go t.benchmarkReporter()

	return nil
}

// Stop gracefully stops the development loop
func (t *SyntheticEngineeringTeam) Stop() error {
	log.Printf("devloop: Stopping Synthetic Engineering Team '%s'", t.name)

	if t.cancel != nil {
		t.cancel()
	}

	t.wg.Wait()

	// Save final state
	t.saveBenchmarks()
	t.saveImprovements()

	log.Printf("devloop: Team '%s' stopped. %d cycles completed.", t.name, t.benchmarks.CyclesCompleted)
	return nil
}

// =============================================================================
// PERPETUAL DEVELOPMENT LOOP
// =============================================================================

// perpetualLoop runs the continuous development cycle
func (t *SyntheticEngineeringTeam) perpetualLoop() {
	defer t.wg.Done()

	cycleNumber := 0
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
			cycleNumber++
			t.runCycle(cycleNumber)
		}
	}
}

// runCycle executes one complete development cycle
func (t *SyntheticEngineeringTeam) runCycle(cycleNumber int) {
	t.mu.Lock()
	t.cycleID = fmt.Sprintf("cycle-%d-%s", cycleNumber, time.Now().Format("20060102-150405"))
	t.cycleMetrics = &CycleMetrics{
		CycleID:     t.cycleID,
		CycleNumber: cycleNumber,
		StartedAt:   time.Now(),
		Phases:      make(map[DevPhase]*PhaseMetrics),
	}
	t.phaseHistory = make([]*PhaseStatus, 0)
	t.mu.Unlock()

	log.Printf("devloop: === Starting Cycle %d (%s) ===", cycleNumber, t.cycleID)

	// Phase 1: Discovery
	t.runPhase(PhaseDiscovery, t.discoveryPhase)

	// Phase 2: Planning
	t.runPhase(PhasePlanning, t.planningPhase)

	// Phase 3: Implementation
	t.runPhase(PhaseImplementation, t.implementationPhase)

	// Phase 4: Review
	t.runPhase(PhaseReview, t.reviewPhase)

	// Phase 5: Learning
	t.runPhase(PhaseLearning, t.learningPhase)

	// Finalize cycle
	t.finalizeCycle()
}

// runPhase executes a single phase with timeout and metrics tracking
func (t *SyntheticEngineeringTeam) runPhase(phase DevPhase, handler func(context.Context) *PhaseMetrics) {
	t.mu.Lock()
	t.currentPhase = phase
	status := &PhaseStatus{
		Phase:     phase,
		Status:    "in_progress",
		StartedAt: timePtr(time.Now()),
	}
	t.phaseHistory = append(t.phaseHistory, status)
	t.mu.Unlock()

	log.Printf("devloop: [%s] Phase started", phase)

	// Create phase context with timeout
	ctx, cancel := context.WithTimeout(t.ctx, t.config.PhaseTimeout)
	defer cancel()

	// Execute phase handler
	metrics := handler(ctx)

	// Record completion
	now := time.Now()
	t.mu.Lock()
	status.CompletedAt = &now
	status.Duration = now.Sub(*status.StartedAt)
	status.Status = "completed"
	status.Metrics = *metrics
	t.cycleMetrics.Phases[phase] = metrics
	t.mu.Unlock()

	log.Printf("devloop: [%s] Phase completed in %v - Processed: %d, Succeeded: %d, Failed: %d",
		phase, status.Duration, metrics.ItemsProcessed, metrics.ItemsSucceeded, metrics.ItemsFailed)
}

// =============================================================================
// PHASE HANDLERS
// =============================================================================

// discoveryPhase runs the swarm to discover findings
func (t *SyntheticEngineeringTeam) discoveryPhase(ctx context.Context) *PhaseMetrics {
	metrics := &PhaseMetrics{}

	log.Printf("devloop: [discovery] Phase handler started")

	// Ensure swarm is running
	if t.swarm == nil {
		log.Printf("devloop: [discovery] No swarm configured, skipping")
		return metrics
	}

	// Get swarm status
	status := t.swarm.GetStatus()
	log.Printf("devloop: [discovery] Swarm state: %s", status.State)
	if status.State != SwarmStateRunning {
		log.Printf("devloop: [discovery] Starting swarm")
		if err := t.swarm.Start(ctx); err != nil {
			log.Printf("devloop: [discovery] Failed to start swarm: %v", err)
			return metrics
		}
		log.Printf("devloop: [discovery] Swarm started successfully")
	}

	// Wait for discovery period (configurable portion of phase timeout)
	discoveryTime := t.config.PhaseTimeout / 2
	log.Printf("devloop: [discovery] Waiting %v for discovery", discoveryTime)
	select {
	case <-ctx.Done():
		log.Printf("devloop: [discovery] Context cancelled")
	case <-time.After(discoveryTime):
		log.Printf("devloop: [discovery] Discovery period complete")
	}

	// Collect findings
	findings := t.swarm.GetFindings("", "", 1000)
	for _, f := range findings {
		metrics.ItemsProcessed++
		if f.Confidence >= t.config.MinConfidence && f.Impact >= t.config.MinImpact {
			metrics.ItemsSucceeded++
		}
	}

	t.mu.Lock()
	t.cycleMetrics.FindingsDiscovered = len(findings)
	metrics.TokensUsed = status.Metrics.TokensUsed
	t.mu.Unlock()

	return metrics
}

// planningPhase prioritizes and plans implementations
func (t *SyntheticEngineeringTeam) planningPhase(ctx context.Context) *PhaseMetrics {
	metrics := &PhaseMetrics{}

	// Get actionable findings
	findings := t.swarm.GetFindings("", "", 1000)
	actionable := t.filterActionableFindings(findings)

	// Prioritize findings
	prioritized := t.prioritizeFindings(actionable)

	metrics.ItemsProcessed = len(findings)
	metrics.ItemsSucceeded = len(prioritized)

	log.Printf("devloop: [planning] Prioritized %d of %d findings for action", len(prioritized), len(findings))

	// Store prioritized list for implementation phase
	t.mu.Lock()
	t.cycleMetrics.FindingsActioned = len(prioritized)
	t.mu.Unlock()

	return metrics
}

// implementationPhase creates PRs for prioritized findings
func (t *SyntheticEngineeringTeam) implementationPhase(ctx context.Context) *PhaseMetrics {
	metrics := &PhaseMetrics{}

	if !t.config.EnablePRCreation {
		log.Printf("devloop: [implementation] PR creation disabled, skipping")
		return metrics
	}

	// Get prioritized findings
	findings := t.swarm.GetFindings("", "", 100)
	prioritized := t.prioritizeFindings(t.filterActionableFindings(findings))

	// Limit to max concurrent PRs
	if len(prioritized) > t.config.MaxConcurrentPRs {
		prioritized = prioritized[:t.config.MaxConcurrentPRs]
	}

	// Create PRs for each finding
	for _, finding := range prioritized {
		select {
		case <-ctx.Done():
			return metrics
		default:
		}

		metrics.ItemsProcessed++

		// Feed to perpetual engine for PR creation
		if engine := t.swarm.GetPerpetualEngine(); engine != nil {
			proposal := &PerpetualProposal{
				ID:           uuid.New().String()[:8],
				Title:        finding.Title,
				Description:  finding.Description,
				Source:       SourceSwarmFinding,
				Impact:       finding.Impact,
				Effort:       EffortSmall,
				DiscoveredAt: time.Now(),
			}

			// Add the proposal to engine queue
			_ = engine.AddProposal(proposal)
			metrics.ItemsSucceeded++
			finding.FindingStatus = FindingStatusActioned
		}
	}

	t.mu.Lock()
	t.cycleMetrics.PRsCreated = metrics.ItemsSucceeded
	t.mu.Unlock()

	return metrics
}

// reviewPhase monitors PR status and merges approved PRs
func (t *SyntheticEngineeringTeam) reviewPhase(ctx context.Context) *PhaseMetrics {
	metrics := &PhaseMetrics{}

	if !t.config.EnablePRCreation {
		log.Printf("devloop: [review] PR creation disabled, skipping review")
		return metrics
	}

	// Check PR statuses
	ghClient, err := NewGitHubClient()
	if err != nil {
		log.Printf("devloop: [review] GitHub client unavailable: %v", err)
		return metrics
	}

	// Get open PRs authored by automation
	prs, err := ghClient.ListPRs("hairglasses/webb", "", "open")
	if err != nil {
		log.Printf("devloop: [review] Failed to list PRs: %v", err)
		return metrics
	}

	for _, pr := range prs {
		select {
		case <-ctx.Done():
			return metrics
		default:
		}

		metrics.ItemsProcessed++

		// Check PR status and merge if approved
		if t.config.EnableAutoMerge {
			// Get detailed PR info to check mergeable status
			prDetails, err := ghClient.GetPullRequest("hairglasses/webb", pr.Number)
			if err != nil {
				continue
			}

			// Check if PR is mergeable (safe dereference of pointer)
			isMergeable := prDetails.Mergeable != nil && *prDetails.Mergeable

			// Also check that CI passed via checks
			checks, _ := ghClient.GetPRChecks("hairglasses/webb", prDetails.Head.SHA)
			allChecksPassed := true
			for _, check := range checks {
				if check.Conclusion != "success" && check.Conclusion != "skipped" {
					allChecksPassed = false
					break
				}
			}

			if isMergeable && allChecksPassed {
				// Auto-merge approved PRs
				commitTitle := fmt.Sprintf("Auto-merge: %s (#%d)", prDetails.Title, pr.Number)
				if err := ghClient.MergePR("hairglasses/webb", pr.Number, commitTitle, ""); err != nil {
					log.Printf("devloop: [review] Failed to merge PR #%d: %v", pr.Number, err)
					metrics.ItemsFailed++
				} else {
					log.Printf("devloop: [review] Merged PR #%d", pr.Number)
					metrics.ItemsSucceeded++
				}
			}
		}
	}

	t.mu.Lock()
	t.cycleMetrics.PRsMerged = metrics.ItemsSucceeded
	t.cycleMetrics.PRsRejected = metrics.ItemsFailed
	t.mu.Unlock()

	return metrics
}

// learningPhase analyzes performance and applies self-improvements
func (t *SyntheticEngineeringTeam) learningPhase(ctx context.Context) *PhaseMetrics {
	metrics := &PhaseMetrics{}

	// Calculate acceptance and merge rates
	t.mu.Lock()
	if t.cycleMetrics.FindingsDiscovered > 0 {
		t.cycleMetrics.AcceptanceRate = float64(t.cycleMetrics.FindingsActioned) / float64(t.cycleMetrics.FindingsDiscovered)
	}
	if t.cycleMetrics.PRsCreated > 0 {
		t.cycleMetrics.MergeRate = float64(t.cycleMetrics.PRsMerged) / float64(t.cycleMetrics.PRsCreated)
	}
	t.mu.Unlock()

	// Analyze worker performance
	efficiency := t.swarm.GetWorkerEfficiency()
	improvements := t.analyzeAndImprove(efficiency)

	metrics.ItemsProcessed = len(efficiency)
	metrics.ItemsSucceeded = len(improvements)

	// Apply improvements
	for _, imp := range improvements {
		t.applyImprovement(imp)
	}

	t.mu.Lock()
	t.cycleMetrics.ImprovementsApplied = len(improvements)
	t.mu.Unlock()

	return metrics
}

// =============================================================================
// SELF-IMPROVEMENT ENGINE
// =============================================================================

// analyzeAndImprove analyzes performance and generates improvements
func (t *SyntheticEngineeringTeam) analyzeAndImprove(efficiency map[SwarmWorkerType]*WorkerEfficiencyStats) []*SelfImprovement {
	var improvements []*SelfImprovement

	// 1. Budget adjustments based on acceptance rate
	for wt, stats := range efficiency {
		if stats.FindingsAccepted+stats.FindingsRejected < 5 {
			continue // Not enough data
		}

		currentMultiplier := t.workerAdjustments[wt]
		if currentMultiplier == 0 {
			currentMultiplier = 1.0
		}

		var newMultiplier float64
		if stats.AcceptanceRate >= 0.75 {
			// High performer - increase budget
			newMultiplier = currentMultiplier * (1 + t.config.BudgetAdjustmentRate)
			if newMultiplier > 2.0 {
				newMultiplier = 2.0 // Cap at 2x
			}
		} else if stats.AcceptanceRate < 0.40 {
			// Underperformer - decrease budget
			newMultiplier = currentMultiplier * (1 - t.config.BudgetAdjustmentRate)
			if newMultiplier < 0.25 {
				newMultiplier = 0.25 // Floor at 0.25x
			}
		} else {
			continue // No adjustment needed
		}

		if newMultiplier != currentMultiplier {
			improvements = append(improvements, &SelfImprovement{
				ID:          uuid.New().String()[:8],
				Type:        "budget_adjustment",
				Description: fmt.Sprintf("Adjusted %s budget multiplier from %.2f to %.2f based on %.0f%% acceptance rate", wt, currentMultiplier, newMultiplier, stats.AcceptanceRate*100),
				AppliedAt:   time.Now(),
				Impact:      fmt.Sprintf("Expected %.0f%% change in token allocation", (newMultiplier-currentMultiplier)*100),
				Metrics: struct {
					Before float64 `json:"before"`
					After  float64 `json:"after"`
					Delta  float64 `json:"delta"`
				}{
					Before: currentMultiplier,
					After:  newMultiplier,
					Delta:  newMultiplier - currentMultiplier,
				},
			})
			t.workerAdjustments[wt] = newMultiplier
		}
	}

	// 2. Quality threshold adjustments
	if t.cycleMetrics != nil && t.cycleMetrics.AcceptanceRate < t.config.ImprovementThreshold {
		// Raise minimum confidence to improve quality
		oldConf := t.qualityThresholds.MinConfidence
		newConf := oldConf + 5
		if newConf <= 90 {
			t.qualityThresholds.MinConfidence = newConf
			improvements = append(improvements, &SelfImprovement{
				ID:          uuid.New().String()[:8],
				Type:        "threshold_update",
				Description: fmt.Sprintf("Raised minimum confidence from %d to %d due to low acceptance rate", oldConf, newConf),
				AppliedAt:   time.Now(),
				Impact:      "Expect fewer but higher-quality findings",
			})
		}
	}

	return improvements
}

// applyImprovement applies a self-improvement
func (t *SyntheticEngineeringTeam) applyImprovement(imp *SelfImprovement) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.improvements = append(t.improvements, imp)
	log.Printf("devloop: [improvement] Applied: %s", imp.Description)
}

// learningLoop runs continuous learning in the background
func (t *SyntheticEngineeringTeam) learningLoop() {
	defer t.wg.Done()

	ticker := time.NewTicker(t.config.LearningInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.runLearningIteration()
		}
	}
}

// runLearningIteration performs one learning iteration
func (t *SyntheticEngineeringTeam) runLearningIteration() {
	if t.swarm == nil {
		return
	}

	efficiency := t.swarm.GetWorkerEfficiency()
	improvements := t.analyzeAndImprove(efficiency)

	for _, imp := range improvements {
		t.applyImprovement(imp)
	}

	// Update benchmarks
	t.updateBenchmarks()
}

// =============================================================================
// BENCHMARKING
// =============================================================================

// updateBenchmarks updates cumulative team benchmarks
func (t *SyntheticEngineeringTeam) updateBenchmarks() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.benchmarks == nil {
		t.benchmarks = &TeamBenchmarks{}
	}

	// Update from current cycle
	if t.cycleMetrics != nil {
		t.benchmarks.CyclesCompleted++
		t.benchmarks.TotalFindings += t.cycleMetrics.FindingsDiscovered
		t.benchmarks.TotalPRsCreated += t.cycleMetrics.PRsCreated
		t.benchmarks.TotalPRsMerged += t.cycleMetrics.PRsMerged

		// Calculate averages
		if t.benchmarks.CyclesCompleted > 0 {
			t.benchmarks.AvgPRsPerCycle = float64(t.benchmarks.TotalPRsCreated) / float64(t.benchmarks.CyclesCompleted)
		}

		// Calculate rates
		if t.benchmarks.TotalFindings > 0 {
			t.benchmarks.OverallAcceptanceRate = float64(t.benchmarks.TotalPRsCreated) / float64(t.benchmarks.TotalFindings)
		}
		if t.benchmarks.TotalPRsCreated > 0 {
			t.benchmarks.OverallMergeRate = float64(t.benchmarks.TotalPRsMerged) / float64(t.benchmarks.TotalPRsCreated)
		}
	}

	// Rank workers
	t.rankWorkers()

	t.benchmarks.LastUpdated = time.Now()
}

// rankWorkers ranks workers by performance
func (t *SyntheticEngineeringTeam) rankWorkers() {
	if t.swarm == nil {
		return
	}

	efficiency := t.swarm.GetWorkerEfficiency()
	rankings := make([]WorkerRanking, 0, len(efficiency))

	for wt, stats := range efficiency {
		// Calculate composite score: 60% acceptance rate, 40% token efficiency
		score := stats.AcceptanceRate * 0.6
		if stats.TokensPerFinding > 0 && stats.TokensPerFinding < 50000 {
			score += (1 - stats.TokensPerFinding/50000) * 0.4
		}

		rankings = append(rankings, WorkerRanking{
			WorkerType:      wt,
			Score:           score,
			AcceptanceRate:  stats.AcceptanceRate,
			TokenEfficiency: stats.TokensPerFinding,
		})
	}

	// Sort by score descending
	sort.Slice(rankings, func(i, j int) bool {
		return rankings[i].Score > rankings[j].Score
	})

	// Assign ranks
	for i := range rankings {
		rankings[i].Rank = i + 1
	}

	// Store top and bottom
	if len(rankings) >= 3 {
		t.benchmarks.TopWorkers = rankings[:3]
		t.benchmarks.BottomWorkers = rankings[len(rankings)-3:]
	}
}

// benchmarkReporter periodically reports benchmarks
func (t *SyntheticEngineeringTeam) benchmarkReporter() {
	defer t.wg.Done()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.reportBenchmarks()
		}
	}
}

// reportBenchmarks logs and saves benchmark data
func (t *SyntheticEngineeringTeam) reportBenchmarks() {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.benchmarks == nil {
		return
	}

	log.Printf("devloop: === Team Benchmarks ===")
	log.Printf("devloop: Cycles: %d | Findings: %d | PRs Created: %d | PRs Merged: %d",
		t.benchmarks.CyclesCompleted, t.benchmarks.TotalFindings,
		t.benchmarks.TotalPRsCreated, t.benchmarks.TotalPRsMerged)
	log.Printf("devloop: Acceptance Rate: %.1f%% | Merge Rate: %.1f%%",
		t.benchmarks.OverallAcceptanceRate*100, t.benchmarks.OverallMergeRate*100)

	if len(t.benchmarks.TopWorkers) > 0 {
		log.Printf("devloop: Top Worker: %s (score: %.2f)",
			t.benchmarks.TopWorkers[0].WorkerType, t.benchmarks.TopWorkers[0].Score)
	}

	// Save to vault
	t.saveBenchmarks()
}

// =============================================================================
// HELPER METHODS
// =============================================================================

// filterActionableFindings filters findings to only actionable ones
func (t *SyntheticEngineeringTeam) filterActionableFindings(findings []*SwarmResearchFinding) []*SwarmResearchFinding {
	actionable := make([]*SwarmResearchFinding, 0)
	for _, f := range findings {
		if f.Confidence >= t.config.MinConfidence &&
			f.Impact >= t.config.MinImpact &&
			f.FindingStatus == FindingStatusPending {
			actionable = append(actionable, f)
		}
	}
	return actionable
}

// prioritizeFindings sorts findings by priority rules
func (t *SyntheticEngineeringTeam) prioritizeFindings(findings []*SwarmResearchFinding) []*SwarmResearchFinding {
	// Score each finding based on priority rules
	type scoredFinding struct {
		finding  *SwarmResearchFinding
		priority int
		score    float64
	}

	scored := make([]scoredFinding, 0, len(findings))
	for _, f := range findings {
		priority := 100 // Default lowest priority
		for _, rule := range t.priorityRules {
			if t.matchesRule(f, rule) {
				priority = rule.Priority
				break
			}
		}

		// Calculate composite score
		score := float64(100-priority)*10 + float64(f.Confidence) + float64(f.Impact)
		scored = append(scored, scoredFinding{f, priority, score})
	}

	// Sort by priority (ascending) then score (descending)
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].priority != scored[j].priority {
			return scored[i].priority < scored[j].priority
		}
		return scored[i].score > scored[j].score
	})

	// Extract sorted findings
	result := make([]*SwarmResearchFinding, len(scored))
	for i, s := range scored {
		result[i] = s.finding
	}
	return result
}

// matchesRule checks if a finding matches a priority rule
func (t *SyntheticEngineeringTeam) matchesRule(f *SwarmResearchFinding, rule *PriorityRule) bool {
	// Check categories
	if len(rule.Categories) > 0 {
		found := false
		for _, cat := range rule.Categories {
			if f.Category == cat {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check confidence threshold
	if rule.MinConf > 0 && f.Confidence < rule.MinConf {
		return false
	}

	// Check impact threshold
	if rule.MinImpact > 0 && f.Impact < rule.MinImpact {
		return false
	}

	return true
}

// finalizeCycle finalizes and records a complete cycle
func (t *SyntheticEngineeringTeam) finalizeCycle() {
	now := time.Now()
	t.mu.Lock()
	t.cycleMetrics.CompletedAt = &now
	t.cycleMetrics.Duration = now.Sub(t.cycleMetrics.StartedAt)
	t.mu.Unlock()

	// Update benchmarks
	t.updateBenchmarks()

	// Save cycle metrics
	t.saveCycleMetrics()

	log.Printf("devloop: === Cycle %d Complete ===", t.cycleMetrics.CycleNumber)
	log.Printf("devloop: Duration: %v | Findings: %d | PRs Created: %d | Improvements: %d",
		t.cycleMetrics.Duration, t.cycleMetrics.FindingsDiscovered,
		t.cycleMetrics.PRsCreated, t.cycleMetrics.ImprovementsApplied)
}

// =============================================================================
// PERSISTENCE
// =============================================================================

// loadBenchmarks loads existing benchmarks from vault
func (t *SyntheticEngineeringTeam) loadBenchmarks() {
	path := filepath.Join(t.vaultPath, "benchmarks.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.benchmarks = &TeamBenchmarks{}
		return
	}

	var benchmarks TeamBenchmarks
	if err := json.Unmarshal(data, &benchmarks); err != nil {
		t.benchmarks = &TeamBenchmarks{}
		return
	}

	t.benchmarks = &benchmarks
}

// saveBenchmarks saves benchmarks to vault
func (t *SyntheticEngineeringTeam) saveBenchmarks() {
	if t.benchmarks == nil {
		return
	}

	path := filepath.Join(t.vaultPath, "benchmarks.json")
	data, err := json.MarshalIndent(t.benchmarks, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(path, data, 0644)
}

// saveCycleMetrics saves current cycle metrics
func (t *SyntheticEngineeringTeam) saveCycleMetrics() {
	if t.cycleMetrics == nil {
		return
	}

	path := filepath.Join(t.vaultPath, "cycles", fmt.Sprintf("%s.json", t.cycleID))
	os.MkdirAll(filepath.Dir(path), 0755)

	data, err := json.MarshalIndent(t.cycleMetrics, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(path, data, 0644)
}

// saveImprovements saves self-improvements history
func (t *SyntheticEngineeringTeam) saveImprovements() {
	path := filepath.Join(t.vaultPath, "improvements.json")
	data, err := json.MarshalIndent(t.improvements, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(path, data, 0644)
}

// GetStatus returns the current team status
func (t *SyntheticEngineeringTeam) GetStatus() *TeamStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return &TeamStatus{
		ID:            t.id,
		Name:          t.name,
		CurrentPhase:  t.currentPhase,
		CycleID:       t.cycleID,
		CycleMetrics:  t.cycleMetrics,
		Benchmarks:    t.benchmarks,
		PhaseHistory:  t.phaseHistory,
		Improvements:  len(t.improvements),
	}
}

// TeamStatus represents the full team status
type TeamStatus struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	CurrentPhase  DevPhase        `json:"current_phase"`
	CycleID       string          `json:"cycle_id"`
	CycleMetrics  *CycleMetrics   `json:"cycle_metrics"`
	Benchmarks    *TeamBenchmarks `json:"benchmarks"`
	PhaseHistory  []*PhaseStatus  `json:"phase_history"`
	Improvements  int             `json:"improvements_applied"`
}

// Helper functions
func timePtr(t time.Time) *time.Time {
	return &t
}

// =============================================================================
// GLOBAL ACCESSOR
// =============================================================================

var (
	globalDevLoopTeam   *SyntheticEngineeringTeam
	globalDevLoopTeamMu sync.RWMutex
)

// SetGlobalDevLoopTeam sets the global devloop team
func SetGlobalDevLoopTeam(team *SyntheticEngineeringTeam) {
	globalDevLoopTeamMu.Lock()
	defer globalDevLoopTeamMu.Unlock()
	globalDevLoopTeam = team
}

// GetGlobalDevLoopTeam returns the global devloop team
func GetGlobalDevLoopTeam() *SyntheticEngineeringTeam {
	globalDevLoopTeamMu.RLock()
	defer globalDevLoopTeamMu.RUnlock()
	return globalDevLoopTeam
}
