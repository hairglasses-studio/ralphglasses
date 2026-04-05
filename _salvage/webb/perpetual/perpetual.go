// Package clients provides client implementations for external services.
package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hairglasses-studio/webb/internal/mcp/otel"
)

// Global perpetual engine singleton
var (
	globalPerpetualEngine   *PerpetualEngine
	globalPerpetualEngineMu sync.RWMutex
)

// SetGlobalPerpetualEngine sets the global perpetual engine
func SetGlobalPerpetualEngine(engine *PerpetualEngine) {
	globalPerpetualEngineMu.Lock()
	defer globalPerpetualEngineMu.Unlock()
	globalPerpetualEngine = engine
}

// GetGlobalPerpetualEngine returns the global perpetual engine if set
func GetGlobalPerpetualEngine() *PerpetualEngine {
	globalPerpetualEngineMu.RLock()
	defer globalPerpetualEngineMu.RUnlock()
	return globalPerpetualEngine
}

// PerpetualState represents the daemon state
type PerpetualState string

const (
	PerpetualStateStarting PerpetualState = "starting"
	PerpetualStateRunning  PerpetualState = "running"
	PerpetualStatePaused   PerpetualState = "paused"
	PerpetualStateStopping PerpetualState = "stopping"
	PerpetualStateStopped  PerpetualState = "stopped"
)

// PerpetualPhase represents the current operational phase
type PerpetualPhase string

const (
	PerpetualPhaseIdle           PerpetualPhase = "idle"
	PerpetualPhaseDiscovery      PerpetualPhase = "discovery"
	PerpetualPhasePrioritization PerpetualPhase = "prioritization"
	PerpetualPhaseImplementation PerpetualPhase = "implementation"
	PerpetualPhaseLearning       PerpetualPhase = "learning"
)

// Additional FeatureSource constants for perpetual engine
// (Base constants are in feature_discovery.go)
const (
	SourceHealthMetrics  FeatureSource = "health_metrics"
	SourcePylon          FeatureSource = "pylon"
	SourceSlack          FeatureSource = "slack"
	SourceWebResearch    FeatureSource = "web_research"
	SourceCompetitor     FeatureSource = "competitor_analysis"
	SourceResearchPapers FeatureSource = "research_papers"

	// v24.0: New discovery sources
	SourceGitHubIssues FeatureSource = "github_issues"
	SourceSentryErrors FeatureSource = "sentry_errors"
)

// PerpetualConfig holds engine configuration
type PerpetualConfig struct {
	// Discovery settings
	DiscoveryCycleInterval time.Duration   `json:"discovery_cycle_interval"` // Default: 6h
	EnabledSources         []FeatureSource `json:"enabled_sources"`

	// Prioritization settings
	SourceWeights map[FeatureSource]float64 `json:"source_weights"`

	// Implementation settings
	MaxConcurrentTasks int           `json:"max_concurrent_tasks"`   // Default: 10
	MaxDailyPRs        int           `json:"max_daily_prs"`          // Default: 100 (effectively unlimited in local mode)
	CooldownAfterFail  time.Duration `json:"cooldown_after_failure"` // Default: 1h
	LocalMode          bool          `json:"local_mode"`             // Skip PR creation, accumulate locally

	// Learning settings
	LearningCycleInterval time.Duration `json:"learning_cycle_interval"` // Default: 24h
	LearningRate          float64       `json:"learning_rate"`           // Default: 0.1

	// Queue settings
	MaxQueueDepth       int           `json:"max_queue_depth"`        // Default: 50
	DeduplicationWindow time.Duration `json:"deduplication_window"`   // Default: 30 days
}

// DefaultPerpetualConfig returns the default configuration
func DefaultPerpetualConfig() *PerpetualConfig {
	return &PerpetualConfig{
		DiscoveryCycleInterval: 6 * time.Hour,
		EnabledSources: []FeatureSource{
			SourceRoadmap, // Highest priority - explicit roadmap items
			SourceMCPEcosystem,
			SourcePylonTickets,
			SourceSlackDiscussions,
			SourceHealthMetrics,
		},
		SourceWeights: map[FeatureSource]float64{
			SourceRoadmap:          2.0, // Roadmap items are highest priority
			SourceHealthMetrics:    1.5,
			SourcePylonTickets:     1.2,
			SourceMCPEcosystem:     1.0,
			SourceSlackDiscussions: 0.9,
			SourceResearchPapers:   0.8,
			SourceCompetitor:       0.7,
		},
		MaxConcurrentTasks:    10,
		MaxDailyPRs:           100, // Effectively unlimited in local mode
		CooldownAfterFail:     1 * time.Hour,
		LocalMode:             true, // Default to local accumulation
		LearningCycleInterval: 24 * time.Hour,
		LearningRate:          0.1,
		MaxQueueDepth:         100,
		DeduplicationWindow:   30 * 24 * time.Hour,
	}
}

// PerpetualProposal represents a discovered feature proposal
type PerpetualProposal struct {
	ID           string        `json:"id"`
	Source       FeatureSource `json:"source"`
	Title        string        `json:"title"`
	Description  string        `json:"description"`
	Evidence     []string      `json:"evidence"`      // Supporting evidence URLs/refs
	Impact       int           `json:"impact"`        // 0-100
	Effort       EffortLevel   `json:"effort"`
	Score        float64       `json:"score"`         // Calculated priority score
	ContentHash  string        `json:"content_hash"`  // For deduplication
	DiscoveredAt time.Time     `json:"discovered_at"`
	Status       string        `json:"status"`        // queued, implementing, completed, failed, skipped
	DevTaskID    string        `json:"dev_task_id,omitempty"`
	PRNumber     int           `json:"pr_number,omitempty"`
	FailureCount int           `json:"failure_count"`
	LastFailure  time.Time     `json:"last_failure,omitempty"`
	// v28.0: Consensus tracking
	Confidence      int             `json:"confidence,omitempty"`        // 0-100, from source finding
	ProposalVotes   []ProposalVote  `json:"proposal_votes,omitempty"`    // Validation votes
	ApprovalStatus  string          `json:"approval_status,omitempty"`   // pending, approved, rejected
}

// ProposalVote represents a validation vote on a proposal (v28.0)
type ProposalVote struct {
	WorkerType string    `json:"worker_type"`
	Approved   bool      `json:"approved"`
	Reason     string    `json:"reason,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// PerpetualMetrics tracks engine performance
type PerpetualMetrics struct {
	StartedAt            time.Time             `json:"started_at"`
	TotalCycles          int                   `json:"total_cycles"`
	ProposalsDiscovered  int                   `json:"proposals_discovered"`
	ProposalsImplemented int                   `json:"proposals_implemented"`
	PRsCreated           int                   `json:"prs_created"`
	PRsMerged            int                   `json:"prs_merged"`
	PRsRejected          int                   `json:"prs_rejected"`
	DiscoveriesBySource  map[FeatureSource]int `json:"discoveries_by_source"`
	LastDiscoveryAt      time.Time             `json:"last_discovery_at"`
	LastImplementationAt time.Time             `json:"last_implementation_at"`
	LastLearningAt       time.Time             `json:"last_learning_at"`
	DailyPRCount         int                   `json:"daily_pr_count"`
	DailyPRResetAt       time.Time             `json:"daily_pr_reset_at"`
}

// PerpetualEngineState represents the full engine state
type PerpetualEngineState struct {
	State          PerpetualState      `json:"state"`
	Phase          PerpetualPhase      `json:"phase"`
	CurrentTaskIDs []string            `json:"current_task_ids"` // Active task IDs
	Metrics        *PerpetualMetrics   `json:"metrics"`
	Config         *PerpetualConfig    `json:"config"`
	LastPersistedAt time.Time          `json:"last_persisted_at"`
}

// PerpetualEngine is the main daemon orchestrator
type PerpetualEngine struct {
	config            *PerpetualConfig
	state             *PerpetualEngineState
	queue             *ProposalQueue
	stateStore        *PerpetualStateStore
	devWorker         *DevWorkerClient
	discoveryPipeline *DiscoveryPipeline
	orchestrator      *ImplementationOrchestrator
	learner           *SourceLearner // v6.14: Learning loop

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex

	// Channels for coordination
	stopCh   chan struct{}
	pauseCh  chan struct{}
	resumeCh chan struct{}

	// Callbacks for extensibility (v6.11)
	onDiscovery      func(*PerpetualProposal)
	onImplementStart func(*PerpetualProposal)
	onPRCreated      func(*PerpetualProposal, int)
	onPROutcome      func(*PerpetualProposal, bool) // merged or rejected
	onLearningComplete func(*LearningResult)       // v6.14: Called after learning cycle

	// v26.0: Swarm integration - bi-directional feedback loop
	swarmOrchestrator *SwarmOrchestrator

	// v131.2: Cycle hooks for continuous process improvement
	onCycleStart    func(cycleID string)
	onCycleComplete func(cycleID string, metrics *CycleMetrics)
	currentCycleID  string
	cycleStartTime  time.Time
}

// ProposalQueue manages the priority queue of proposals
type ProposalQueue struct {
	proposals []*PerpetualProposal
	mu        sync.RWMutex
	maxDepth  int
}

// NewProposalQueue creates a new proposal queue
func NewProposalQueue(maxDepth int) *ProposalQueue {
	return &ProposalQueue{
		proposals: make([]*PerpetualProposal, 0),
		maxDepth:  maxDepth,
	}
}

// Push adds a proposal to the queue (sorted by score)
func (q *ProposalQueue) Push(p *PerpetualProposal) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.proposals) >= q.maxDepth {
		// Check if new proposal has higher score than lowest
		if len(q.proposals) > 0 && p.Score <= q.proposals[len(q.proposals)-1].Score {
			return false
		}
		// Remove lowest scored
		q.proposals = q.proposals[:len(q.proposals)-1]
	}

	// Insert sorted by score (descending)
	inserted := false
	for i, existing := range q.proposals {
		if p.Score > existing.Score {
			q.proposals = append(q.proposals[:i], append([]*PerpetualProposal{p}, q.proposals[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		q.proposals = append(q.proposals, p)
	}

	return true
}

// Pop removes and returns the highest priority proposal
func (q *ProposalQueue) Pop() *PerpetualProposal {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.proposals) == 0 {
		return nil
	}

	p := q.proposals[0]
	q.proposals = q.proposals[1:]
	return p
}

// Peek returns the highest priority proposal without removing it
func (q *ProposalQueue) Peek() *PerpetualProposal {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.proposals) == 0 {
		return nil
	}
	return q.proposals[0]
}

// List returns all proposals in the queue
func (q *ProposalQueue) List() []*PerpetualProposal {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]*PerpetualProposal, len(q.proposals))
	copy(result, q.proposals)
	return result
}

// Len returns the queue length
func (q *ProposalQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.proposals)
}

// Contains checks if a proposal with given content hash exists
func (q *ProposalQueue) Contains(contentHash string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for _, p := range q.proposals {
		if p.ContentHash == contentHash {
			return true
		}
	}
	return false
}

// NewPerpetualEngine creates a new perpetual engine
func NewPerpetualEngine(config *PerpetualConfig) (*PerpetualEngine, error) {
	if config == nil {
		config = DefaultPerpetualConfig()
	}

	// Initialize state store
	stateStore, err := NewPerpetualStateStore()
	if err != nil {
		return nil, fmt.Errorf("failed to create state store: %w", err)
	}

	// Try to restore state
	state, err := stateStore.Load()
	if err != nil {
		// No existing state, create new
		state = &PerpetualEngineState{
			State:  PerpetualStateStopped,
			Phase:  PerpetualPhaseIdle,
			Config: config,
			Metrics: &PerpetualMetrics{
				DiscoveriesBySource: make(map[FeatureSource]int),
			},
		}
	} else {
		// Reset state to stopped in case of previous crash
		// (daemon was running but process died without clean shutdown)
		if state.State == PerpetualStateRunning || state.State == PerpetualStateStarting {
			state.State = PerpetualStateStopped
		}
		// Clear stale task IDs from previous crash
		// These tasks are no longer running in the current process
		state.CurrentTaskIDs = nil
	}

	// Get dev worker client
	devWorker, err := GetGlobalDevWorker()
	if err != nil {
		return nil, fmt.Errorf("failed to get dev worker: %w", err)
	}

	// Initialize discovery pipeline - start with defaults (which includes RoadmapPath)
	// and override based on enabled sources
	discoveryConfig := DefaultDiscoveryPipelineConfig()
	discoveryConfig.EnableRoadmap = containsSource(config.EnabledSources, SourceRoadmap)
	discoveryConfig.EnableMCPEcosystem = containsSource(config.EnabledSources, SourceMCPEcosystem)
	discoveryConfig.EnablePylonTickets = containsSource(config.EnabledSources, SourcePylonTickets)
	discoveryConfig.EnableSlackDiscussions = containsSource(config.EnabledSources, SourceSlackDiscussions)
	discoveryConfig.EnableHealthMetrics = containsSource(config.EnabledSources, SourceHealthMetrics)
	discoveryConfig.EnableResearchPapers = containsSource(config.EnabledSources, SourceResearchPapers)
	discoveryConfig.EnableCompetitor = containsSource(config.EnabledSources, SourceCompetitor)
	// Pass state store for persistent deduplication (survives restarts)
	discoveryPipeline, _ := NewDiscoveryPipelineWithStore(discoveryConfig, stateStore)

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize the source learner with configured learning rate
	// Use improved defaults: minSamples 3 (was 5), learningRate 0.2 if not specified
	effectiveLearningRate := config.LearningRate
	if effectiveLearningRate == 0 {
		effectiveLearningRate = 0.2 // Default to faster learning
	}
	learner := NewSourceLearnerWithConfig(effectiveLearningRate, 3, 0.1, 5.0)

	engine := &PerpetualEngine{
		config:            config,
		state:             state,
		queue:             NewProposalQueue(config.MaxQueueDepth),
		stateStore:        stateStore,
		devWorker:         devWorker,
		discoveryPipeline: discoveryPipeline,
		learner:           learner,
		ctx:               ctx,
		cancel:            cancel,
		stopCh:            make(chan struct{}),
		pauseCh:           make(chan struct{}),
		resumeCh:          make(chan struct{}),
	}

	// Initialize the implementation orchestrator
	engine.orchestrator = NewImplementationOrchestrator(devWorker, engine)

	// Load learned weights from database and apply to config
	if weights, err := stateStore.GetSourceWeights(); err == nil && len(weights) > 0 {
		for source, weight := range weights {
			config.SourceWeights[source] = weight
		}
		fmt.Printf("perpetual: loaded %d learned source weights from database\n", len(weights))
	}

	return engine, nil
}

// containsSource checks if a source is in the enabled list
func containsSource(sources []FeatureSource, source FeatureSource) bool {
	for _, s := range sources {
		if s == source {
			return true
		}
	}
	return false
}

// Start begins the perpetual engine daemon
func (e *PerpetualEngine) Start() error {
	e.mu.Lock()
	if e.state.State == PerpetualStateRunning {
		e.mu.Unlock()
		return fmt.Errorf("engine already running")
	}

	e.state.State = PerpetualStateRunning // Set to Running before starting goroutines
	e.state.Metrics.StartedAt = time.Now()
	e.mu.Unlock()

	// Start the main loop
	e.wg.Add(1)
	go e.mainLoop()

	// Start the state persistence goroutine
	e.wg.Add(1)
	go e.persistLoop()

	// v28.0: Start PR monitoring goroutine
	e.wg.Add(1)
	go e.monitorPRStatus()

	return nil
}

// Stop gracefully stops the engine
func (e *PerpetualEngine) Stop() error {
	e.mu.Lock()
	if e.state.State != PerpetualStateRunning && e.state.State != PerpetualStatePaused {
		e.mu.Unlock()
		return fmt.Errorf("engine not running (state: %s)", e.state.State)
	}

	e.state.State = PerpetualStateStopping
	e.mu.Unlock()

	// Signal stop
	close(e.stopCh)
	e.cancel()

	// Wait for goroutines to finish
	e.wg.Wait()

	e.mu.Lock()
	e.state.State = PerpetualStateStopped
	e.state.Phase = PerpetualPhaseIdle
	e.mu.Unlock()

	// Final persist
	if err := e.stateStore.Save(e.state); err != nil {
		return fmt.Errorf("failed to save final state: %w", err)
	}

	return nil
}

// Pause pauses the engine (finishes current task)
func (e *PerpetualEngine) Pause() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state.State != PerpetualStateRunning {
		return fmt.Errorf("engine not running")
	}

	e.state.State = PerpetualStatePaused
	close(e.pauseCh)
	return nil
}

// Resume resumes a paused engine
func (e *PerpetualEngine) Resume() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state.State != PerpetualStatePaused {
		return fmt.Errorf("engine not paused")
	}

	e.state.State = PerpetualStateRunning
	e.pauseCh = make(chan struct{})
	close(e.resumeCh)
	e.resumeCh = make(chan struct{})
	return nil
}

// Status returns the current engine status
func (e *PerpetualEngine) Status() *PerpetualEngineState {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Return a copy
	stateCopy := *e.state
	metricsCopy := *e.state.Metrics
	stateCopy.Metrics = &metricsCopy
	return &stateCopy
}

// GetQueue returns the current proposal queue
func (e *PerpetualEngine) GetQueue() []*PerpetualProposal {
	return e.queue.List()
}

// UpdateConfig updates the engine configuration
func (e *PerpetualEngine) UpdateConfig(config *PerpetualConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config = config
	e.state.Config = config
}

// mainLoop is the core perpetual cycle
func (e *PerpetualEngine) mainLoop() {
	defer e.wg.Done()

	discoveryTicker := time.NewTicker(e.config.DiscoveryCycleInterval)
	defer discoveryTicker.Stop()

	learningTicker := time.NewTicker(e.config.LearningCycleInterval)
	defer learningTicker.Stop()

	// Run initial discovery
	e.runDiscoveryPhase()

	for {
		select {
		case <-e.ctx.Done():
			return

		case <-e.stopCh:
			return

		case <-e.pauseCh:
			// Wait for resume
			select {
			case <-e.resumeCh:
				continue
			case <-e.ctx.Done():
				return
			}

		case <-discoveryTicker.C:
			e.runDiscoveryPhase()

		case <-learningTicker.C:
			e.runLearningPhase()

		default:
			// Check if we can implement something
			canImpl, reason := e.canImplementWithReason()
			if canImpl {
				e.runImplementationPhase()
				// Rate limit between implementations to prevent database contention
				time.Sleep(5 * time.Second)
			} else {
				// Debug: Log why we can't implement (only occasionally)
				e.mu.RLock()
				queueLen := e.queue.Len()
				e.mu.RUnlock()
				if queueLen > 0 {
					fmt.Printf("perpetual: cannot implement yet (%s), queue has %d items, sleeping 30s...\n", reason, queueLen)
				}
				time.Sleep(30 * time.Second)
			}
		}
	}
}

// persistLoop periodically saves state
func (e *PerpetualEngine) persistLoop() {
	defer e.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.mu.RLock()
			state := e.state
			e.mu.RUnlock()

			if err := e.stateStore.Save(state); err != nil {
				fmt.Fprintf(os.Stderr, "perpetual: failed to persist state: %v\n", err)
			}
		}
	}
}

// canImplement checks if we can start a new implementation
func (e *PerpetualEngine) canImplement() bool {
	can, _ := e.canImplementWithReason()
	return can
}

// canImplementWithReason checks if we can start a new implementation and returns the reason if not
func (e *PerpetualEngine) canImplementWithReason() (bool, string) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Check state
	if e.state.State != PerpetualStateRunning {
		return false, fmt.Sprintf("state is %s, not running", e.state.State)
	}

	// Check daily PR limit
	if e.state.Metrics.DailyPRCount >= e.config.MaxDailyPRs {
		// Check if we should reset
		if time.Since(e.state.Metrics.DailyPRResetAt) > 24*time.Hour {
			e.state.Metrics.DailyPRCount = 0
			e.state.Metrics.DailyPRResetAt = time.Now()
		} else {
			return false, fmt.Sprintf("daily PR limit reached (%d/%d)", e.state.Metrics.DailyPRCount, e.config.MaxDailyPRs)
		}
	}

	// Check concurrent task limit
	if len(e.state.CurrentTaskIDs) >= e.config.MaxConcurrentTasks {
		return false, fmt.Sprintf("concurrent task limit reached (%d/%d)", len(e.state.CurrentTaskIDs), e.config.MaxConcurrentTasks)
	}

	// Check if queue has items
	if e.queue.Len() == 0 {
		return false, "queue is empty"
	}

	return true, ""
}

// runDiscoveryPhase runs the discovery pipeline
func (e *PerpetualEngine) runDiscoveryPhase() {
	// v131.2: Generate cycle ID and track start time
	cycleID := fmt.Sprintf("cycle-%d-%d", time.Now().Unix(), e.state.Metrics.TotalCycles+1)
	cycleStart := time.Now()

	e.mu.Lock()
	e.state.Phase = PerpetualPhaseDiscovery
	e.currentCycleID = cycleID
	e.cycleStartTime = cycleStart
	e.mu.Unlock()

	// v131.2: Fire cycle start hook
	if e.onCycleStart != nil {
		e.onCycleStart(cycleID)
	}

	defer func() {
		cycleDuration := time.Since(cycleStart)

		e.mu.Lock()
		e.state.Phase = PerpetualPhaseIdle
		e.state.Metrics.LastDiscoveryAt = time.Now()
		e.state.Metrics.TotalCycles++

		// v131.2: Fire cycle complete hook with metrics
		if e.onCycleComplete != nil {
			now := time.Now()
			metrics := &CycleMetrics{
				CycleID:            cycleID,
				CycleNumber:        e.state.Metrics.TotalCycles,
				StartedAt:          cycleStart,
				CompletedAt:        &now,
				Duration:           cycleDuration,
				FindingsDiscovered: e.state.Metrics.ProposalsDiscovered,
				PRsCreated:         e.state.Metrics.PRsCreated,
				PRsMerged:          e.state.Metrics.PRsMerged,
				PRsRejected:        e.state.Metrics.PRsRejected,
			}
			e.mu.Unlock()
			e.onCycleComplete(cycleID, metrics)
		} else {
			e.mu.Unlock()
		}
	}()

	fmt.Println("perpetual: running discovery phase...")

	// Run the discovery pipeline
	if e.discoveryPipeline == nil {
		fmt.Println("perpetual: discovery pipeline not initialized")
		return
	}

	result, err := e.discoveryPipeline.RunDiscovery(e.ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "perpetual: discovery error: %v\n", err)
		return
	}

	// Log discovery results
	fmt.Printf("perpetual: discovered %d proposals (duplicates filtered: %d, duration: %dms)\n",
		len(result.Proposals), result.DuplicatesFound, result.DurationMs)

	// Log by source
	for source, count := range result.BySource {
		fmt.Printf("perpetual:   - %s: %d proposals\n", source, count)
	}

	// Log any errors
	for _, errMsg := range result.Errors {
		fmt.Fprintf(os.Stderr, "perpetual: source error: %s\n", errMsg)
	}

	// Track discoveries
	e.mu.Lock()
	e.state.Metrics.ProposalsDiscovered += len(result.Proposals)
	for source, count := range result.BySource {
		e.state.Metrics.DiscoveriesBySource[source] += count
	}
	e.mu.Unlock()

	// Move to prioritization phase if we have proposals
	if len(result.Proposals) > 0 {
		e.runPrioritizationPhase(result.Proposals)

		// Fire callbacks for each new proposal
		for _, p := range result.Proposals {
			if e.onDiscovery != nil {
				e.onDiscovery(p)
			}
		}
	}

	fmt.Printf("perpetual: discovery complete, queue depth: %d\n", e.queue.Len())
}

// runPrioritizationPhase scores and queues proposals
func (e *PerpetualEngine) runPrioritizationPhase(proposals []*PerpetualProposal) {
	e.mu.Lock()
	e.state.Phase = PerpetualPhasePrioritization
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.state.Phase = PerpetualPhaseIdle
		e.mu.Unlock()
	}()

	// v24.0: Quality gate - filter out low-quality proposals
	var acceptedProposals []*PerpetualProposal
	for _, p := range proposals {
		// Calculate score first
		p.Score = e.calculateScore(p)

		// Apply quality gates
		if reject, reason := e.shouldRejectProposal(p); reject {
			fmt.Printf("perpetual: rejected proposal %q: %s\n", p.Title, reason)
			continue
		}

		acceptedProposals = append(acceptedProposals, p)
	}

	// v24.0: Detect cross-proposal relationships
	if e.stateStore != nil {
		e.detectProposalRelationships(acceptedProposals)
	}

	// Add accepted proposals to queue
	for _, p := range acceptedProposals {
		if !e.queue.Contains(p.ContentHash) {
			e.queue.Push(p)
		}
	}
}

// v24.0: shouldRejectProposal applies quality gates to filter low-quality proposals
func (e *PerpetualEngine) shouldRejectProposal(p *PerpetualProposal) (bool, string) {
	// Gate 1: Score threshold
	if p.Score < 20 {
		return true, "score below threshold (< 20)"
	}

	// Gate 2: Impact too low
	if p.Impact < 30 {
		return true, "impact too low (< 30)"
	}

	// Gate 3: Check for duplicate titles (fuzzy match)
	existing := e.queue.List()
	for _, ep := range existing {
		if stringSimilarity(p.Title, ep.Title) > 0.85 {
			return true, fmt.Sprintf("similar to existing proposal: %s", ep.Title)
		}
	}

	return false, ""
}

// v24.0: detectProposalRelationships finds relationships between proposals
func (e *PerpetualEngine) detectProposalRelationships(proposals []*PerpetualProposal) {
	// Compare each proposal with existing proposals in queue
	existing := e.queue.List()

	for _, p := range proposals {
		for _, ep := range existing {
			if p.ID == ep.ID {
				continue
			}

			// Check for title similarity (bundles_with)
			similarity := stringSimilarity(p.Title, ep.Title)
			if similarity > 0.5 && similarity <= 0.85 {
				_ = e.stateStore.AddProposalRelationship(p.ID, ep.ID, "bundles_with", similarity, "semantic_similarity")
			}

			// Check for source correlation (same source = related)
			if p.Source == ep.Source {
				_ = e.stateStore.AddProposalRelationship(p.ID, ep.ID, "similar_to", 0.6, "source_correlation")
			}

			// Check for evidence overlap (file overlap)
			if len(p.Evidence) > 0 && len(ep.Evidence) > 0 {
				overlap := evidenceOverlap(p.Evidence, ep.Evidence)
				if overlap > 0 {
					_ = e.stateStore.AddProposalRelationship(p.ID, ep.ID, "bundles_with", float64(overlap)/float64(len(p.Evidence)+len(ep.Evidence)), "file_overlap")
				}
			}
		}
	}
}

// stringSimilarity calculates simple similarity between two strings
func stringSimilarity(a, b string) float64 {
	a = strings.ToLower(a)
	b = strings.ToLower(b)

	if a == b {
		return 1.0
	}

	// Simple word overlap
	wordsA := strings.Fields(a)
	wordsB := strings.Fields(b)

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0.0
	}

	matches := 0
	wordSet := make(map[string]bool)
	for _, w := range wordsA {
		wordSet[w] = true
	}
	for _, w := range wordsB {
		if wordSet[w] {
			matches++
		}
	}

	return float64(matches*2) / float64(len(wordsA)+len(wordsB))
}

// evidenceOverlap counts overlapping evidence between proposals
func evidenceOverlap(a, b []string) int {
	overlap := 0
	for _, ea := range a {
		for _, eb := range b {
			if ea == eb {
				overlap++
			}
		}
	}
	return overlap
}

// calculateScore computes the priority score for a proposal
func (e *PerpetualEngine) calculateScore(p *PerpetualProposal) float64 {
	// Score = (Impact × SourceWeight × RecencyBoost) / (EffortCost + 1)
	impact := float64(p.Impact)
	sourceWeight := e.config.SourceWeights[p.Source]
	if sourceWeight == 0 {
		sourceWeight = 1.0
	}

	// Recency boost: newer discoveries get higher priority
	daysSinceDiscovery := time.Since(p.DiscoveredAt).Hours() / 24
	recencyBoost := 1.0 + (0.1 / (daysSinceDiscovery + 1))

	// Effort cost
	var effortCost float64
	switch p.Effort {
	case EffortSmall:
		effortCost = 1
	case EffortMedium:
		effortCost = 3
	case EffortLarge:
		effortCost = 7
	default:
		effortCost = 3
	}

	return (impact * sourceWeight * recencyBoost) / (effortCost + 1)
}

// runImplementationPhase implements the highest priority proposal
func (e *PerpetualEngine) runImplementationPhase() {
	e.mu.Lock()
	e.state.Phase = PerpetualPhaseImplementation
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.state.Phase = PerpetualPhaseIdle
		e.state.Metrics.LastImplementationAt = time.Now()
		e.mu.Unlock()
	}()

	// Find a proposal that's not in cooldown
	var proposal *PerpetualProposal
	var skippedProposals []*PerpetualProposal

	for {
		p := e.queue.Pop()
		if p == nil {
			break // No more proposals
		}

		// Check cooldown for failed proposals
		if p.FailureCount > 0 {
			cooldown := e.config.CooldownAfterFail * time.Duration(p.FailureCount)
			if time.Since(p.LastFailure) < cooldown {
				// Track for re-queue later, continue looking
				skippedProposals = append(skippedProposals, p)
				continue
			}
		}

		// Found a valid proposal
		proposal = p
		break
	}

	// Re-queue skipped proposals
	for _, p := range skippedProposals {
		e.queue.Push(p)
	}

	if proposal == nil {
		// All proposals are in cooldown
		if len(skippedProposals) > 0 {
			fmt.Printf("perpetual: all %d proposals in cooldown, waiting...\n", len(skippedProposals))
		}
		return
	}

	// v28.0: Check consensus for high-risk proposals
	if e.requiresApproval(proposal) && !e.hasConsensus(proposal) {
		// Log and re-queue for later (after gathering consensus)
		firstTimeNotification := proposal.ApprovalStatus == ""
		if firstTimeNotification {
			proposal.ApprovalStatus = "pending"
			// Send Slack notification for new pending proposals
			e.notifyApprovalNeeded(proposal)
		}
		fmt.Printf("perpetual: proposal %s awaiting consensus (confidence: %d, impact: %d, votes: %d)\n",
			proposal.Title[:min(40, len(proposal.Title))], proposal.Confidence, proposal.Impact, len(proposal.ProposalVotes))
		e.queue.Push(proposal) // Re-queue
		return
	}

	// Use the orchestrator to implement the proposal
	if e.orchestrator == nil {
		fmt.Println("perpetual: orchestrator not initialized, skipping implementation")
		e.queue.Push(proposal) // Re-queue
		return
	}

	fmt.Printf("perpetual: implementing proposal: %s (score: %.2f)\n", proposal.Title, proposal.Score)

	// Track the task ID
	taskID, err := e.orchestrator.ProcessProposalAsync(e.ctx, proposal)
	if err != nil {
		fmt.Fprintf(os.Stderr, "perpetual: failed to start implementation: %v\n", err)
		proposal.FailureCount++
		proposal.LastFailure = time.Now()
		proposal.Status = "failed"
		e.queue.Push(proposal) // Re-queue for retry after cooldown
		return
	}

	// Track active task
	e.mu.Lock()
	e.state.CurrentTaskIDs = append(e.state.CurrentTaskIDs, taskID)
	e.state.Metrics.ProposalsImplemented++
	e.mu.Unlock()

	fmt.Printf("perpetual: proposal queued as task %s\n", taskID)
}

// v28.0: requiresApproval checks if a proposal needs consensus approval before implementation
func (e *PerpetualEngine) requiresApproval(p *PerpetualProposal) bool {
	// Require approval for:
	// - Low confidence findings (<70%)
	// - High impact proposals (>70)
	// - Proposals from swarm findings (automated sources)
	if p.Confidence > 0 && p.Confidence < 70 {
		return true
	}
	if p.Impact > 70 {
		return true
	}
	if p.Source == SourceSwarmFinding {
		return true
	}
	return false
}

// v28.0: hasConsensus checks if a proposal has sufficient consensus votes
func (e *PerpetualEngine) hasConsensus(p *PerpetualProposal) bool {
	// Need at least 2 votes for consensus
	if len(p.ProposalVotes) < 2 {
		return false
	}

	approvals := 0
	for _, v := range p.ProposalVotes {
		if v.Approved {
			approvals++
		}
	}

	// Require 2/3 majority (66.6...%)
	ratio := float64(approvals) / float64(len(p.ProposalVotes))
	return ratio >= 0.66
}

// v28.0: AddProposalVote adds a validation vote to a proposal
func (e *PerpetualEngine) AddProposalVote(proposalID, workerType string, approved bool, reason string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Find the proposal in the queue
	proposals := e.queue.List()
	for _, p := range proposals {
		if p.ID == proposalID {
			// Check if this worker already voted
			for _, v := range p.ProposalVotes {
				if v.WorkerType == workerType {
					return false // Already voted
				}
			}

			// Add the vote
			p.ProposalVotes = append(p.ProposalVotes, ProposalVote{
				WorkerType: workerType,
				Approved:   approved,
				Reason:     reason,
				Timestamp:  time.Now(),
			})

			// Update approval status if we have enough votes
			if e.hasConsensus(p) {
				p.ApprovalStatus = "approved"
			} else if len(p.ProposalVotes) >= 3 && !e.hasConsensus(p) {
				p.ApprovalStatus = "rejected"
			} else {
				p.ApprovalStatus = "pending"
			}

			return true
		}
	}

	return false
}

// v28.0: GetPendingApprovals returns proposals awaiting consensus
func (e *PerpetualEngine) GetPendingApprovals() []*PerpetualProposal {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var pending []*PerpetualProposal
	for _, p := range e.queue.List() {
		if e.requiresApproval(p) && p.ApprovalStatus != "approved" {
			pending = append(pending, p)
		}
	}
	return pending
}

// v28.0: notifyApprovalNeeded sends a Slack notification for proposals needing approval
func (e *PerpetualEngine) notifyApprovalNeeded(p *PerpetualProposal) {
	// Only notify once - check if already notified
	if p.ApprovalStatus != "" && p.ApprovalStatus != "pending" {
		return
	}

	slackClient, err := NewSlackClient()
	if err != nil {
		fmt.Printf("perpetual: failed to create Slack client for approval notification: %v\n", err)
		return
	}

	// Build notification message
	riskFactors := []string{}
	if p.Confidence > 0 && p.Confidence < 70 {
		riskFactors = append(riskFactors, fmt.Sprintf("low confidence (%d%%)", p.Confidence))
	}
	if p.Impact > 70 {
		riskFactors = append(riskFactors, fmt.Sprintf("high impact (%d)", p.Impact))
	}
	if p.Source == SourceSwarmFinding {
		riskFactors = append(riskFactors, "automated swarm finding")
	}

	riskStr := "unknown"
	if len(riskFactors) > 0 {
		riskStr = strings.Join(riskFactors, ", ")
	}

	message := fmt.Sprintf("*Swarm Proposal Needs Approval*\n\n"+
		"*Title:* %s\n"+
		"*Source:* %s\n"+
		"*Risk Factors:* %s\n"+
		"*Score:* %.2f (Impact: %d, Effort: %s)\n"+
		"*Votes:* %d/2 required\n\n"+
		"_Use `/approve %s` or vote via webb MCP tools_",
		p.Title, p.Source, riskStr, p.Score, p.Impact, p.Effort, len(p.ProposalVotes), p.ID[:8])

	// Post to #oncall-core-platform (C08K65CBFQT)
	channelID := "C08K65CBFQT"
	if err := slackClient.PostMessage(channelID, message, ""); err != nil {
		fmt.Printf("perpetual: failed to send approval notification: %v\n", err)
	} else {
		fmt.Printf("perpetual: sent approval notification for proposal %s to Slack\n", p.ID[:8])
	}
}

// v28.0: monitorPRStatus monitors PR outcomes and updates metrics
func (e *PerpetualEngine) monitorPRStatus() {
	defer e.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.checkPROutcomes()
		}
	}
}

// v28.0: checkPROutcomes checks status of all PRs created by proposals
func (e *PerpetualEngine) checkPROutcomes() {
	gh, err := NewGitHubClient()
	if err != nil {
		return
	}

	e.mu.RLock()
	// Get proposals that have PRs but haven't been finalized
	pendingPRs := make([]*PerpetualProposal, 0)
	for _, p := range e.queue.List() {
		if p.PRNumber > 0 && p.Status == "completed" {
			pendingPRs = append(pendingPRs, p)
		}
	}
	e.mu.RUnlock()

	for _, p := range pendingPRs {
		// Get PR status from GitHub
		pr, err := gh.GetPullRequest("hairglasses/webb", p.PRNumber)
		if err != nil {
			continue
		}

		e.mu.Lock()
		if pr.MergedAt != nil {
			// PR was merged - record success
			p.Status = "merged"
			e.state.Metrics.PRsMerged++

			// Calculate time to merge
			mergeHours := pr.MergedAt.Sub(pr.CreatedAt).Hours()

			// Update quality metrics
			otel.RecordSwarmQualityMetrics(
				0, 0, 1, // merged
				0, 0, 1, // prs merged
				mergeHours,
			)

			fmt.Printf("perpetual: PR #%d merged for proposal %s\n", p.PRNumber, p.Title[:min(40, len(p.Title))])
		} else if pr.State == "closed" {
			// PR was closed without merge - record rejection
			p.Status = "rejected"
			e.state.Metrics.PRsRejected++

			// Update quality metrics
			otel.RecordSwarmQualityMetrics(
				0, 1, 0, // rejected
				0, 0, 0,
				0,
			)

			fmt.Printf("perpetual: PR #%d rejected for proposal %s\n", p.PRNumber, p.Title[:min(40, len(p.Title))])

			// Consider re-queuing with improvements
			if p.FailureCount < 2 {
				p.FailureCount++
				p.LastFailure = time.Now()
				p.Status = "queued"
				p.PRNumber = 0 // Reset PR for retry
				e.queue.Push(p)
				fmt.Printf("perpetual: re-queued proposal %s for retry\n", p.Title[:min(40, len(p.Title))])
			}
		}
		// else: PR still open, continue monitoring
		e.mu.Unlock()
	}
}

// RemoveTaskID removes a task from the active task list
// Called by orchestrator when task completes or fails
func (e *PerpetualEngine) RemoveTaskID(taskID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Find and remove the task ID
	for i, id := range e.state.CurrentTaskIDs {
		if id == taskID {
			e.state.CurrentTaskIDs = append(e.state.CurrentTaskIDs[:i], e.state.CurrentTaskIDs[i+1:]...)
			return
		}
	}
}

// runLearningPhase analyzes outcomes and adjusts weights
func (e *PerpetualEngine) runLearningPhase() {
	e.mu.Lock()
	e.state.Phase = PerpetualPhaseLearning
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.state.Phase = PerpetualPhaseIdle
		e.state.Metrics.LastLearningAt = time.Now()
		e.mu.Unlock()
	}()

	fmt.Println("perpetual: running learning phase...")

	// Get current weights from config
	e.mu.RLock()
	currentWeights := make(map[FeatureSource]float64)
	for k, v := range e.config.SourceWeights {
		currentWeights[k] = v
	}
	e.mu.RUnlock()

	// Run the learning algorithm
	result, err := e.learner.RunLearning(e.stateStore, currentWeights)
	if err != nil {
		fmt.Fprintf(os.Stderr, "perpetual: learning error: %v\n", err)
		return
	}

	// Apply new weights to config
	if result.WeightsAdjusted > 0 {
		e.mu.Lock()
		for source, change := range result.Adjustments {
			if change.NewWeight != change.OldWeight {
				e.config.SourceWeights[source] = change.NewWeight
				fmt.Printf("perpetual: adjusted weight for %s: %.2f -> %.2f (%s)\n",
					source, change.OldWeight, change.NewWeight, change.Reason)
			}
		}
		e.mu.Unlock()
	}

	// Log summary
	fmt.Printf("perpetual: learning complete - analyzed %d sources, adjusted %d weights\n",
		result.SourcesAnalyzed, result.WeightsAdjusted)

	// Log skipped sources
	if len(result.SkippedSources) > 0 {
		fmt.Printf("perpetual: skipped %d sources (insufficient samples): %v\n",
			len(result.SkippedSources), result.SkippedSources)
	}

	// Fire callback
	if e.onLearningComplete != nil {
		e.onLearningComplete(result)
	}
}

// RecordProposalOutcome records the outcome of a proposal for learning
// This should be called when a PR is merged, rejected, or fails
// v24.0: Now includes streaming learning - weight adjusted immediately, not batched
func (e *PerpetualEngine) RecordProposalOutcome(proposal *PerpetualProposal, outcome OutcomeType, prNumber int, mergeTimeHours float64, reviewComments int) error {
	err := e.stateStore.RecordOutcome(proposal.ID, proposal.Source, string(outcome), prNumber, mergeTimeHours, reviewComments)
	if err != nil {
		return fmt.Errorf("failed to record outcome: %w", err)
	}

	// Update metrics
	e.mu.Lock()
	switch outcome {
	case OutcomeMerged:
		e.state.Metrics.PRsMerged++
	case OutcomeRejected:
		e.state.Metrics.PRsRejected++
	}
	e.mu.Unlock()

	// v24.0: Streaming learning - immediately adjust weight based on outcome
	if outcome == OutcomeMerged || outcome == OutcomeRejected || outcome == OutcomeFailed {
		go e.applyStreamingLearning(proposal.Source, outcome == OutcomeMerged)
	}

	// Fire callback
	if e.onPROutcome != nil {
		e.onPROutcome(proposal, outcome == OutcomeMerged)
	}

	return nil
}

// v24.0: applyStreamingLearning performs immediate weight adjustment
func (e *PerpetualEngine) applyStreamingLearning(source FeatureSource, success bool) {
	// Get current weight from config
	e.mu.RLock()
	currentWeight := e.config.SourceWeights[source]
	e.mu.RUnlock()

	if currentWeight == 0 {
		currentWeight = 1.0
	}

	// Apply streaming learning
	result, err := e.learner.LearnFromOutcome(e.stateStore, source, success, currentWeight)
	if err != nil {
		fmt.Fprintf(os.Stderr, "perpetual: streaming learning error for %s: %v\n", source, err)
		return
	}

	// Update in-memory config
	e.mu.Lock()
	e.config.SourceWeights[source] = result.NewWeight
	e.mu.Unlock()

	// Log the adjustment
	if result.OldWeight != result.NewWeight {
		fmt.Printf("perpetual: [streaming] %s weight %.3f → %.3f (%s)\n",
			source, result.OldWeight, result.NewWeight, result.Reason)
	}
}

// GetLearningMetrics returns current learning system metrics
func (e *PerpetualEngine) GetLearningMetrics() (*LearningMetrics, error) {
	return e.stateStore.GetLearningMetrics()
}

// ForceLearning triggers an immediate learning cycle
func (e *PerpetualEngine) ForceLearning() {
	go e.runLearningPhase()
}

// AddProposal manually adds a proposal to the queue
func (e *PerpetualEngine) AddProposal(p *PerpetualProposal) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	if p.DiscoveredAt.IsZero() {
		p.DiscoveredAt = time.Now()
	}
	if p.Status == "" {
		p.Status = "queued"
	}

	// v26.0: Check memory for similar proposals (semantic deduplication)
	if e.swarmOrchestrator != nil {
		config := e.swarmOrchestrator.GetConfigV25()
		if config != nil && config.MemoryDeduplication {
			memClient := GetSessionMemoryClient()
			if memClient != nil {
				searchText := p.Title + " " + p.Description
				if results, err := memClient.Search(searchText, 3); err == nil {
					// Check for high-similarity matches (>0.7)
					for _, scored := range results.UserMemories {
						if scored.Score > 0.7 {
							return fmt.Errorf("similar proposal exists in memory: %s (score: %.2f)",
								scored.Memory.Content[:min(50, len(scored.Memory.Content))], scored.Score)
						}
					}
				}
			}
		}
	}

	// Calculate score
	p.Score = e.calculateScore(p)

	// Check for duplicate
	if e.queue.Contains(p.ContentHash) {
		return fmt.Errorf("proposal with same content already exists")
	}

	if !e.queue.Push(p) {
		return fmt.Errorf("queue at capacity, proposal score too low")
	}

	// Track discovery
	e.mu.Lock()
	e.state.Metrics.ProposalsDiscovered++
	e.state.Metrics.DiscoveriesBySource[p.Source]++
	e.mu.Unlock()

	// Callback
	if e.onDiscovery != nil {
		e.onDiscovery(p)
	}

	return nil
}

// SetCallbacks sets the event callbacks
func (e *PerpetualEngine) SetCallbacks(
	onDiscovery func(*PerpetualProposal),
	onImplementStart func(*PerpetualProposal),
	onPRCreated func(*PerpetualProposal, int),
	onPROutcome func(*PerpetualProposal, bool),
) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onDiscovery = onDiscovery
	e.onImplementStart = onImplementStart
	e.onPRCreated = onPRCreated
	e.onPROutcome = onPROutcome
}

// SetCycleCallbacks sets the cycle lifecycle callbacks for continuous process improvement (v131.2)
func (e *PerpetualEngine) SetCycleCallbacks(
	onCycleStart func(cycleID string),
	onCycleComplete func(cycleID string, metrics *CycleMetrics),
) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onCycleStart = onCycleStart
	e.onCycleComplete = onCycleComplete
}

// GetCurrentCycleID returns the current cycle ID for external tracking (v131.2)
func (e *PerpetualEngine) GetCurrentCycleID() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentCycleID
}

// GetCycleStartTime returns the start time of the current cycle (v131.2)
func (e *PerpetualEngine) GetCycleStartTime() time.Time {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cycleStartTime
}

// v26.0: SetSwarmOrchestrator wires the perpetual engine to send feedback to the swarm
// This creates a bi-directional feedback loop:
// - Swarm findings -> perpetual proposals -> PR outcomes -> swarm feedback
func (e *PerpetualEngine) SetSwarmOrchestrator(s *SwarmOrchestrator) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.swarmOrchestrator = s

	// Wire the onPROutcome callback to send feedback to swarm for swarm-sourced proposals
	originalCallback := e.onPROutcome
	e.onPROutcome = func(p *PerpetualProposal, merged bool) {
		// Call original callback if set
		if originalCallback != nil {
			originalCallback(p, merged)
		}

		// If this proposal came from the swarm, send feedback back
		if p.Source == SourceSwarmFinding && s != nil {
			outcome := OutcomeRejected
			if merged {
				outcome = OutcomeMerged
			}

			// Extract worker type from evidence if available
			// Format: "Worker: worker-id (worker_type)"
			workerType := SwarmWorkerType("unknown")
			for _, ev := range p.Evidence {
				if len(ev) > 8 && ev[:8] == "Worker: " {
					// Parse "Worker: tool_auditor-1 (tool_auditor)"
					// Extract the type in parentheses
					start := strings.Index(ev, "(")
					end := strings.Index(ev, ")")
					if start != -1 && end != -1 && end > start+1 {
						workerType = SwarmWorkerType(ev[start+1 : end])
					}
					break
				}
			}

			// Extract category from evidence if available (format: "Category: xyz")
			category := "unknown"
			for _, ev := range p.Evidence {
				if len(ev) > 10 && ev[:10] == "Category: " {
					category = ev[10:]
					break
				}
			}

			feedback := &SwarmFeedback{
				ProposalID: p.ID,
				FindingID:  p.ID, // Same ID
				WorkerType: workerType,
				Category:   category,
				Outcome:    outcome,
				Timestamp:  time.Now(),
			}

			// Send feedback to swarm (non-blocking)
			go s.ReceivePerpetualFeedback(feedback)
		}
	}
}

// MarshalJSON implements json.Marshaler for PerpetualEngineState
func (s *PerpetualEngineState) MarshalJSON() ([]byte, error) {
	type Alias PerpetualEngineState
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(s),
	})
}

// getConfigPath returns the path to the perpetual config/state directory
func getConfigPath() (string, error) {
	configDir := os.Getenv("WEBB_CONFIG_DIR")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		configDir = filepath.Join(home, ".config", "webb")
	}

	perpetualDir := filepath.Join(configDir, "perpetual")
	if err := os.MkdirAll(perpetualDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create perpetual directory: %w", err)
	}

	return perpetualDir, nil
}

// LoadBulkFeatures generates and loads bulk features into the queue
func (e *PerpetualEngine) LoadBulkFeatures(count int) (int, error) {
	generator := NewBulkFeatureGenerator()
	proposals := generator.GenerateBulk(count)

	loaded := 0
	for _, rp := range proposals {
		// Convert RoadmapProposal to PerpetualProposal
		pp := &PerpetualProposal{
			ID:           rp.ID,
			Source:       SourceManual, // Use manual source for generated features
			Title:        rp.Title,
			Description:  rp.Description,
			Evidence:     rp.Evidence,
			Impact:       rp.Impact,
			Effort:       rp.Effort,
			ContentHash:  GenerateContentHash(rp.Title, rp.Description),
			DiscoveredAt: time.Now(),
			Status:       "queued",
		}

		// Calculate score with complexity-based adjustment
		pp.Score = e.calculateScore(pp)

		// Adjust score based on complexity for review requirements
		if len(rp.Tools) > 0 {
			switch rp.Tools[0].Complexity {
			case "complex":
				pp.Score *= 0.8 // Lower priority for complex tasks (need more review)
			case "moderate":
				pp.Score *= 0.9
			}
		}

		if !e.queue.Contains(pp.ContentHash) {
			if e.queue.Push(pp) {
				loaded++
			}
		}
	}

	e.mu.Lock()
	e.state.Metrics.ProposalsDiscovered += loaded
	e.mu.Unlock()

	fmt.Printf("perpetual: loaded %d/%d bulk features into queue (queue depth: %d)\n",
		loaded, count, e.queue.Len())

	return loaded, nil
}

// RunForDuration runs the engine for a specified duration with progress tracking
func (e *PerpetualEngine) RunForDuration(ctx context.Context, duration time.Duration, progressCallback func(elapsed, remaining time.Duration, metrics *PerpetualMetrics)) error {
	startTime := time.Now()
	endTime := startTime.Add(duration)

	// Start the engine if not running
	if e.state.State != PerpetualStateRunning {
		if err := e.Start(); err != nil {
			return fmt.Errorf("failed to start engine: %w", err)
		}
	}

	// Progress ticker
	progressTicker := time.NewTicker(10 * time.Second)
	defer progressTicker.Stop()

	// Checkpoint ticker
	checkpointTicker := time.NewTicker(5 * time.Minute)
	defer checkpointTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("perpetual: context cancelled, stopping...")
			return e.Stop()

		case <-progressTicker.C:
			elapsed := time.Since(startTime)
			remaining := endTime.Sub(time.Now())
			if remaining < 0 {
				remaining = 0
			}

			// Get current metrics
			e.mu.RLock()
			metrics := *e.state.Metrics
			e.mu.RUnlock()

			// Call progress callback
			if progressCallback != nil {
				progressCallback(elapsed, remaining, &metrics)
			}

			// Check if duration exceeded
			if time.Now().After(endTime) {
				fmt.Printf("perpetual: duration of %v reached, stopping...\n", duration)
				return e.Stop()
			}

		case <-checkpointTicker.C:
			// Force checkpoint
			e.mu.RLock()
			state := e.state
			e.mu.RUnlock()

			if err := e.stateStore.Save(state); err != nil {
				fmt.Fprintf(os.Stderr, "perpetual: checkpoint error: %v\n", err)
			} else {
				fmt.Println("perpetual: checkpoint saved")
			}
		}
	}
}

// GetQueueStats returns statistics about the current queue
func (e *PerpetualEngine) GetQueueStats() map[string]interface{} {
	proposals := e.queue.List()

	stats := map[string]interface{}{
		"total":          len(proposals),
		"by_complexity":  map[string]int{"simple": 0, "moderate": 0, "complex": 0},
		"by_source":      make(map[string]int),
		"avg_score":      0.0,
		"avg_impact":     0.0,
		"pending":        0,
		"in_cooldown":    0,
	}

	if len(proposals) == 0 {
		return stats
	}

	var totalScore, totalImpact float64
	bySource := stats["by_source"].(map[string]int)

	for _, p := range proposals {
		totalScore += p.Score
		totalImpact += float64(p.Impact)
		bySource[string(p.Source)]++

		if p.FailureCount > 0 {
			cooldown := e.config.CooldownAfterFail * time.Duration(p.FailureCount)
			if time.Since(p.LastFailure) < cooldown {
				stats["in_cooldown"] = stats["in_cooldown"].(int) + 1
			}
		}

		if p.Status == "queued" || p.Status == "pending" {
			stats["pending"] = stats["pending"].(int) + 1
		}
	}

	stats["avg_score"] = totalScore / float64(len(proposals))
	stats["avg_impact"] = totalImpact / float64(len(proposals))

	return stats
}

// PressureTest runs a high-volume test of the perpetual engine
func (e *PerpetualEngine) PressureTest(ctx context.Context, featureCount int, duration time.Duration) (*PressureTestResult, error) {
	result := &PressureTestResult{
		StartTime:     time.Now(),
		TargetFeatures: featureCount,
		TargetDuration: duration,
	}

	fmt.Printf("perpetual: starting pressure test - %d features, %v duration\n", featureCount, duration)

	// Load bulk features
	loaded, err := e.LoadBulkFeatures(featureCount)
	if err != nil {
		return result, fmt.Errorf("failed to load bulk features: %w", err)
	}
	result.FeaturesLoaded = loaded

	// Set up progress callback
	progressCallback := func(elapsed, remaining time.Duration, metrics *PerpetualMetrics) {
		rate := float64(metrics.ProposalsImplemented) / elapsed.Minutes()
		fmt.Printf("perpetual: [%v/%v] implemented=%d rate=%.1f/min queue=%d\n",
			elapsed.Round(time.Second), duration.Round(time.Second),
			metrics.ProposalsImplemented, rate, e.queue.Len())
	}

	// Run for duration
	err = e.RunForDuration(ctx, duration, progressCallback)

	// Collect final results
	result.EndTime = time.Now()
	result.ActualDuration = result.EndTime.Sub(result.StartTime)

	e.mu.RLock()
	result.TasksCompleted = e.state.Metrics.ProposalsImplemented
	result.PRsCreated = e.state.Metrics.PRsCreated
	result.ConsensusReached = e.state.Metrics.PRsMerged
	result.ConsensusFailed = e.state.Metrics.PRsRejected
	e.mu.RUnlock()

	result.FeaturesRemaining = e.queue.Len()
	result.AverageTimePerTask = 0
	if result.TasksCompleted > 0 {
		result.AverageTimePerTask = result.ActualDuration / time.Duration(result.TasksCompleted)
	}

	return result, err
}

// PressureTestResult contains the results of a pressure test
type PressureTestResult struct {
	StartTime          time.Time     `json:"start_time"`
	EndTime            time.Time     `json:"end_time"`
	TargetFeatures     int           `json:"target_features"`
	TargetDuration     time.Duration `json:"target_duration"`
	ActualDuration     time.Duration `json:"actual_duration"`
	FeaturesLoaded     int           `json:"features_loaded"`
	FeaturesRemaining  int           `json:"features_remaining"`
	TasksCompleted     int           `json:"tasks_completed"`
	PRsCreated         int           `json:"prs_created"`
	ConsensusReached   int           `json:"consensus_reached"`
	ConsensusFailed    int           `json:"consensus_failed"`
	AverageTimePerTask time.Duration `json:"average_time_per_task"`
}

// FormatPressureTestResult formats the result for display
func FormatPressureTestResult(r *PressureTestResult) string {
	var sb strings.Builder

	sb.WriteString("\n=== Pressure Test Results ===\n\n")
	sb.WriteString(fmt.Sprintf("Duration: %v (target: %v)\n", r.ActualDuration.Round(time.Second), r.TargetDuration))
	sb.WriteString(fmt.Sprintf("Features: %d loaded, %d remaining\n", r.FeaturesLoaded, r.FeaturesRemaining))
	sb.WriteString(fmt.Sprintf("Tasks: %d completed\n", r.TasksCompleted))
	sb.WriteString(fmt.Sprintf("PRs: %d created, %d consensus, %d failed\n", r.PRsCreated, r.ConsensusReached, r.ConsensusFailed))

	if r.TasksCompleted > 0 {
		rate := float64(r.TasksCompleted) / r.ActualDuration.Minutes()
		sb.WriteString(fmt.Sprintf("Rate: %.2f tasks/minute\n", rate))
		sb.WriteString(fmt.Sprintf("Avg time per task: %v\n", r.AverageTimePerTask.Round(time.Second)))
	}

	consensusRate := 0.0
	if r.PRsCreated > 0 {
		consensusRate = float64(r.ConsensusReached) / float64(r.PRsCreated) * 100
	}
	sb.WriteString(fmt.Sprintf("Consensus rate: %.1f%%\n", consensusRate))

	return sb.String()
}

// ============================================
// MCP TOOL HELPER METHODS
// ============================================

// PerpetualEngineStats contains statistics for MCP tool responses
type PerpetualEngineStats struct {
	Status               string         `json:"status"`
	StartedAt            time.Time      `json:"started_at"`
	LastCycleAt          *time.Time     `json:"last_cycle_at,omitempty"`
	CyclesCompleted      int            `json:"cycles_completed"`
	DiscoveryRuns        int            `json:"discovery_runs"`
	ProposalsDiscovered  int            `json:"proposals_discovered"`
	ProposalsApproved    int            `json:"proposals_approved"`
	ProposalsImplemented int            `json:"proposals_implemented"`
	QueueSize            int            `json:"queue_size"`
	DiscoveryBySource    map[string]int `json:"discovery_by_source"`
}

// GetStats returns current engine statistics for MCP tools
func (e *PerpetualEngine) GetStats() *PerpetualEngineStats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats := &PerpetualEngineStats{
		Status:               string(e.state.State),
		StartedAt:            e.state.Metrics.StartedAt,
		CyclesCompleted:      e.state.Metrics.TotalCycles,
		DiscoveryRuns:        e.state.Metrics.TotalCycles,
		ProposalsDiscovered:  e.state.Metrics.ProposalsDiscovered,
		ProposalsApproved:    e.state.Metrics.PRsCreated,
		ProposalsImplemented: e.state.Metrics.ProposalsImplemented,
		QueueSize:            e.queue.Len(),
		DiscoveryBySource:    make(map[string]int),
	}

	if !e.state.Metrics.LastDiscoveryAt.IsZero() {
		lastCycleAt := e.state.Metrics.LastDiscoveryAt
		stats.LastCycleAt = &lastCycleAt
	}

	e.queue.mu.RLock()
	for _, p := range e.queue.proposals {
		stats.DiscoveryBySource[string(p.Source)]++
	}
	e.queue.mu.RUnlock()

	return stats
}

// GetProposals returns proposals from the queue for MCP tools
func (e *PerpetualEngine) GetProposals(limit int) []*PerpetualProposal {
	e.queue.mu.RLock()
	defer e.queue.mu.RUnlock()

	if limit <= 0 || limit > len(e.queue.proposals) {
		limit = len(e.queue.proposals)
	}

	result := make([]*PerpetualProposal, limit)
	copy(result, e.queue.proposals[:limit])
	return result
}

// ApproveProposal marks a proposal as approved
func (e *PerpetualEngine) ApproveProposal(proposalID string) error {
	e.queue.mu.Lock()
	defer e.queue.mu.Unlock()

	for _, p := range e.queue.proposals {
		if p.ID == proposalID {
			p.Status = "approved"
			p.Score = 1000.0 // Boost to prioritize
			sort.Slice(e.queue.proposals, func(a, b int) bool {
				return e.queue.proposals[a].Score > e.queue.proposals[b].Score
			})
			return nil
		}
	}
	return fmt.Errorf("proposal not found: %s", proposalID)
}

// RejectProposal marks a proposal as rejected
func (e *PerpetualEngine) RejectProposal(proposalID string, _ string) error {
	e.queue.mu.Lock()
	defer e.queue.mu.Unlock()

	for i, p := range e.queue.proposals {
		if p.ID == proposalID {
			p.Status = "rejected"
			e.queue.proposals = append(e.queue.proposals[:i], e.queue.proposals[i+1:]...)
			e.mu.Lock()
			e.state.Metrics.PRsRejected++
			e.mu.Unlock()
			return nil
		}
	}
	return fmt.Errorf("proposal not found: %s", proposalID)
}

// UpdateProposalPriority updates a proposal's priority score
func (e *PerpetualEngine) UpdateProposalPriority(proposalID string, newPriority int) error {
	e.queue.mu.Lock()
	defer e.queue.mu.Unlock()

	for _, p := range e.queue.proposals {
		if p.ID == proposalID {
			p.Impact = newPriority
			p.Score = e.calculateScore(p)
			sort.Slice(e.queue.proposals, func(i, j int) bool {
				return e.queue.proposals[i].Score > e.queue.proposals[j].Score
			})
			return nil
		}
	}
	return fmt.Errorf("proposal not found: %s", proposalID)
}

// AutonomousCycleConfig configures an autonomous improvement cycle
type AutonomousCycleConfig struct {
	Duration                time.Duration
	WriteDiscoveriesToVault bool
	WriteCycleSummaries     bool
	SummaryInterval         time.Duration
	EnableResearchSync      bool
	VerboseLogging          bool
}

// RunAutonomousCycle runs the perpetual engine in enhanced autonomous mode
func (e *PerpetualEngine) RunAutonomousCycle(ctx context.Context, config *AutonomousCycleConfig) error {
	if config == nil {
		config = &AutonomousCycleConfig{
			Duration:                time.Hour,
			WriteDiscoveriesToVault: true,
			WriteCycleSummaries:     true,
			SummaryInterval:         30 * time.Minute,
			VerboseLogging:          false,
		}
	}

	if err := e.Start(); err != nil {
		return fmt.Errorf("failed to start engine: %w", err)
	}

	if config.Duration > 0 {
		var progressCallback func(elapsed, remaining time.Duration, metrics *PerpetualMetrics)
		if config.VerboseLogging {
			progressCallback = func(elapsed, remaining time.Duration, metrics *PerpetualMetrics) {
				fmt.Printf("perpetual: [%v/%v] discovered=%d implemented=%d queue=%d\n",
					elapsed.Round(time.Second), config.Duration.Round(time.Second),
					metrics.ProposalsDiscovered, metrics.ProposalsImplemented, e.queue.Len())
			}
		}
		return e.RunForDuration(ctx, config.Duration, progressCallback)
	}

	<-ctx.Done()
	e.Stop()
	return ctx.Err()
}
