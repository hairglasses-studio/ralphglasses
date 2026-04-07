package session

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// ResearchDaemonConfig controls the passive research daemon's behavior.
// Parsed from .ralphrc keys prefixed with RESEARCH_DAEMON_.
type ResearchDaemonConfig struct {
	Enabled         bool    `json:"enabled"`
	BudgetPerRunUSD float64 `json:"budget_per_run_usd"` // max spend per daemon run (default $10)
	BudgetDailyUSD  float64 `json:"budget_daily_usd"`   // daily ceiling (default $25)
	MaxComplexity   int     `json:"max_complexity"`     // max autonomous complexity 1-4 (default 3)
	TickInterval    int     `json:"tick_interval"`      // run every Nth supervisor tick (default 5 = ~5min)
	ClaimTTLSecs    int     `json:"claim_ttl_secs"`     // how long a topic claim lasts (default 7200 = 2h)
	AgentID         string  `json:"agent_id"`           // identifier for queue claims
	MaxTopicsPerRun int     `json:"max_topics_per_run"` // topics to process per tick (default 1)
}

// DefaultResearchDaemonConfig returns sensible defaults for passive research.
func DefaultResearchDaemonConfig() ResearchDaemonConfig {
	return ResearchDaemonConfig{
		Enabled:         true,
		BudgetPerRunUSD: 10.0,
		BudgetDailyUSD:  25.0,
		MaxComplexity:   3,
		TickInterval:    5,
		ClaimTTLSecs:    7200,
		AgentID:         "research-daemon",
		MaxTopicsPerRun: 1,
	}
}

// ResearchEntry is the daemon's view of a queue entry. Decoupled from
// docs/internal/registries.QueueEntry to avoid a direct import dependency.
type ResearchEntry struct {
	Topic         string  `json:"topic"`
	Domain        string  `json:"domain"`
	Source        string  `json:"source"`
	PriorityScore float64 `json:"priority_score"`
	ModelTier     string  `json:"model_tier"`
	BudgetUSD     float64 `json:"budget_usd"`
}

// ResearchGateway abstracts access to the docs knowledge base and research
// queue. Implemented by research_gateway.go (Phase 2).
type ResearchGateway interface {
	// ExpireStale releases abandoned claims back to the pending pool.
	ExpireStale(ctx context.Context) (int, error)

	// DequeueNext claims the highest-priority pending topic. Returns nil
	// when the queue is empty.
	DequeueNext(ctx context.Context, agent string, claimTTL int) (*ResearchEntry, error)

	// DedupCheck returns confidence (0-1) and a recommendation string
	// ("exists", "partial", "proceed") for the given topic.
	DedupCheck(ctx context.Context, topic, domain string) (confidence float64, recommendation string, err error)

	// Complete marks a topic as finished in the queue.
	Complete(ctx context.Context, topic, domain string) error

	// Abandon releases a claimed topic back to pending with a reason.
	Abandon(ctx context.Context, topic, domain, reason string) error

	// WriteResearch persists a research finding to the docs repo.
	WriteResearch(ctx context.Context, domain, title, content string, urls []string) error

	// CommitAndPush batches pending writes into a commit and pushes.
	CommitAndPush(ctx context.Context, message string) error
}

// ResearchDaemon orchestrates passive background research using the docs
// knowledge base, research queue, and cascade router. It runs as a check
// inside the Supervisor tick loop — not as a separate process.
type ResearchDaemon struct {
	mu        sync.Mutex
	gateway   ResearchGateway
	decisions *DecisionLog
	bus       *events.Bus
	budget    *BudgetEnvelope
	router    *CascadeRouter
	config    ResearchDaemonConfig

	tickCount       int
	topicsProcessed int
	topicsCompleted int
	topicsFailed    int
	researchOutputs int
	dedupSkips      int
	autonomyRejects int
	dailySpentUSD   float64
	dailyResetAt    time.Time
	lastRunAt       time.Time
	pendingCommits  int // writes since last commit
}

type researchTopicOutcome string

const (
	researchTopicWritten          researchTopicOutcome = "written"
	researchTopicDedupSkipped     researchTopicOutcome = "dedup_skipped"
	researchTopicAutonomyRejected researchTopicOutcome = "autonomy_rejected"
	researchTopicComplexityReject researchTopicOutcome = "complexity_rejected"
	researchTopicBudgetRejected   researchTopicOutcome = "budget_rejected"
)

// NewResearchDaemon creates a daemon with the given gateway and config.
func NewResearchDaemon(gw ResearchGateway, cfg ResearchDaemonConfig) *ResearchDaemon {
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = 5
	}
	if cfg.ClaimTTLSecs <= 0 {
		cfg.ClaimTTLSecs = 7200
	}
	if cfg.AgentID == "" {
		cfg.AgentID = "research-daemon"
	}
	if cfg.MaxTopicsPerRun <= 0 {
		cfg.MaxTopicsPerRun = 1
	}
	if cfg.BudgetPerRunUSD <= 0 {
		cfg.BudgetPerRunUSD = 10.0
	}
	if cfg.BudgetDailyUSD <= 0 {
		cfg.BudgetDailyUSD = 25.0
	}
	if cfg.MaxComplexity <= 0 || cfg.MaxComplexity > 4 {
		cfg.MaxComplexity = 3
	}
	return &ResearchDaemon{
		gateway:      gw,
		config:       cfg,
		dailyResetAt: nextMidnight(),
	}
}

// SetDecisionLog configures autonomy gating.
func (rd *ResearchDaemon) SetDecisionLog(dl *DecisionLog) {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	rd.decisions = dl
}

// SetBus configures event publishing.
func (rd *ResearchDaemon) SetBus(bus *events.Bus) {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	rd.bus = bus
}

// SetBudget configures global cost enforcement.
func (rd *ResearchDaemon) SetBudget(be *BudgetEnvelope) {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	rd.budget = be
}

// SetRouter configures cascade model routing.
func (rd *ResearchDaemon) SetRouter(cr *CascadeRouter) {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	rd.router = cr
}

// Tick is called by the Supervisor on every tick (60s). The daemon only
// runs its processing loop every TickInterval ticks (default 5 = ~5min).
func (rd *ResearchDaemon) Tick(ctx context.Context) {
	rd.mu.Lock()
	rd.tickCount++
	tick := rd.tickCount
	interval := rd.config.TickInterval
	enabled := rd.config.Enabled
	rd.mu.Unlock()

	if !enabled || tick%interval != 0 {
		return
	}

	rd.run(ctx)
}

func (rd *ResearchDaemon) run(ctx context.Context) {
	rd.resetDailyBudgetIfNeeded()

	// Check daily budget ceiling.
	rd.mu.Lock()
	if rd.dailySpentUSD >= rd.config.BudgetDailyUSD {
		rd.mu.Unlock()
		slog.Info("research-daemon: daily budget exhausted",
			"spent", rd.dailySpentUSD, "cap", rd.config.BudgetDailyUSD)
		return
	}
	budget := rd.budget
	rd.mu.Unlock()

	// Check global budget.
	if budget != nil && budget.Remaining() <= 0 {
		slog.Info("research-daemon: global budget exhausted")
		return
	}

	// Step 1: Release abandoned claims.
	if expired, err := rd.gateway.ExpireStale(ctx); err != nil {
		slog.Warn("research-daemon: expire stale claims failed", "error", err)
	} else if expired > 0 {
		slog.Info("research-daemon: expired stale claims", "count", expired)
	}

	// Step 2: Process up to MaxTopicsPerRun topics.
	var runSpent float64
	for i := 0; i < rd.config.MaxTopicsPerRun; i++ {
		if ctx.Err() != nil {
			return
		}
		if runSpent >= rd.config.BudgetPerRunUSD {
			slog.Debug("research-daemon: per-run budget reached", "spent", runSpent)
			break
		}

		entry, err := rd.gateway.DequeueNext(ctx, rd.config.AgentID, rd.config.ClaimTTLSecs)
		if err != nil {
			slog.Warn("research-daemon: dequeue failed", "error", err)
			return
		}
		if entry == nil {
			slog.Debug("research-daemon: queue empty")
			break
		}

		outcome, cost, err := rd.processTopic(ctx, entry)
		if err != nil {
			slog.Warn("research-daemon: topic failed",
				"topic", entry.Topic, "domain", entry.Domain, "error", err)
			rd.mu.Lock()
			rd.topicsFailed++
			rd.mu.Unlock()
		} else {
			rd.mu.Lock()
			switch outcome {
			case researchTopicWritten:
				rd.topicsProcessed++
				rd.topicsCompleted++
				rd.researchOutputs++
			case researchTopicDedupSkipped:
				rd.dedupSkips++
			case researchTopicAutonomyRejected:
				rd.autonomyRejects++
			}
			rd.mu.Unlock()
			runSpent += cost
		}
	}

	// Batch commit if we have pending writes (every 3+ findings or end of run).
	rd.mu.Lock()
	pending := rd.pendingCommits
	rd.mu.Unlock()
	if pending >= 3 {
		rd.commitPending(ctx)
	}

	rd.mu.Lock()
	rd.lastRunAt = time.Now()
	rd.mu.Unlock()
}

// processTopic runs the full pipeline for a single research topic:
// classify → gate → dedup → route → execute → write → complete.
// Returns the cost incurred.
func (rd *ResearchDaemon) processTopic(ctx context.Context, entry *ResearchEntry) (researchTopicOutcome, float64, error) {
	// Step 3: Dedup check.
	confidence, recommendation, err := rd.gateway.DedupCheck(ctx, entry.Topic, entry.Domain)
	if err != nil {
		slog.Warn("research-daemon: dedup check failed, proceeding",
			"topic", entry.Topic, "error", err)
		confidence = 0
		recommendation = "proceed"
	}

	// Already well-covered — mark complete and skip.
	if confidence >= 0.7 || recommendation == "exists" {
		slog.Info("research-daemon: topic already covered",
			"topic", entry.Topic, "confidence", confidence)
		if err := rd.gateway.Complete(ctx, entry.Topic, entry.Domain); err != nil {
			slog.Warn("research-daemon: complete (dedup skip) failed", "error", err)
		}
		return researchTopicDedupSkipped, 0, nil
	}

	// Step 4: Classify complexity.
	complexity := classifyResearchComplexity(entry, confidence)

	// Step 5: Autonomy gate.
	if complexity > rd.config.MaxComplexity {
		slog.Info("research-daemon: complexity exceeds max, abandoning",
			"topic", entry.Topic, "complexity", complexity, "max", rd.config.MaxComplexity)
		rd.gateway.Abandon(ctx, entry.Topic, entry.Domain, "complexity_exceeds_max")
		return researchTopicComplexityReject, 0, nil
	}

	rd.mu.Lock()
	decisions := rd.decisions
	rd.mu.Unlock()

	if decisions != nil {
		requiredLevel := LevelAutoOptimize // complexity 1-3
		if complexity >= 4 {
			requiredLevel = LevelFullAutonomy
		}
		allowed := decisions.Propose(AutonomousDecision{
			Category:      DecisionLaunch,
			RequiredLevel: requiredLevel,
			Rationale: fmt.Sprintf("research-daemon: process topic %q (domain=%s, complexity=%d)",
				entry.Topic, entry.Domain, complexity),
			Inputs: map[string]any{
				"topic":      entry.Topic,
				"domain":     entry.Domain,
				"complexity": complexity,
				"confidence": confidence,
				"source":     entry.Source,
			},
			Action: "research_topic",
		})
		if !allowed {
			slog.Info("research-daemon: autonomy gate rejected",
				"topic", entry.Topic, "complexity", complexity)
			rd.gateway.Abandon(ctx, entry.Topic, entry.Domain, "autonomy_gate_rejected")
			return researchTopicAutonomyRejected, 0, nil
		}
	}

	// Step 6: Budget check for this topic.
	topicBudget := budgetForComplexity(complexity)

	rd.mu.Lock()
	if rd.dailySpentUSD+topicBudget > rd.config.BudgetDailyUSD {
		rd.mu.Unlock()
		rd.gateway.Abandon(ctx, entry.Topic, entry.Domain, "daily_budget_exhausted")
		return researchTopicBudgetRejected, 0, fmt.Errorf("daily budget would be exceeded")
	}
	budget := rd.budget
	rd.mu.Unlock()

	if budget != nil && !budget.CanSpend(topicBudget) {
		rd.gateway.Abandon(ctx, entry.Topic, entry.Domain, "global_budget_exhausted")
		return researchTopicBudgetRejected, 0, fmt.Errorf("global budget exhausted")
	}

	// Step 7: Route model via cascade.
	rd.mu.Lock()
	router := rd.router
	rd.mu.Unlock()

	var tier ModelTier
	if router != nil {
		tier = router.SelectTier("research", complexity)
	}

	// Publish start event.
	rd.publishEvent(events.ResearchStarted, map[string]any{
		"topic":      entry.Topic,
		"domain":     entry.Domain,
		"complexity": complexity,
		"confidence": confidence,
		"model":      tier.Model,
		"provider":   string(tier.Provider),
		"budget_usd": topicBudget,
	})

	// Step 8: Determine research mode based on dedup confidence.
	mode := "new"
	if confidence >= 0.4 {
		mode = "expand"
	}

	// Step 9: Execute — build prompt and write via gateway.
	prompt := buildResearchPrompt(entry, mode, complexity)
	if err := rd.gateway.WriteResearch(ctx, entry.Domain, entry.Topic, prompt, nil); err != nil {
		rd.publishEvent(events.ResearchFailed, map[string]any{
			"topic":  entry.Topic,
			"domain": entry.Domain,
			"error":  err.Error(),
		})
		rd.gateway.Abandon(ctx, entry.Topic, entry.Domain, fmt.Sprintf("write_failed: %v", err))
		return researchTopicWritten, 0, fmt.Errorf("write research: %w", err)
	}

	// Step 10: Record spend.
	if budget != nil {
		budget.RecordSpend(topicBudget)
	}
	rd.mu.Lock()
	rd.dailySpentUSD += topicBudget
	rd.pendingCommits++
	rd.mu.Unlock()

	// Step 11: Complete the queue entry.
	if err := rd.gateway.Complete(ctx, entry.Topic, entry.Domain); err != nil {
		slog.Warn("research-daemon: complete failed", "topic", entry.Topic, "error", err)
	}

	rd.publishEvent(events.ResearchCompleted, map[string]any{
		"topic":      entry.Topic,
		"domain":     entry.Domain,
		"complexity": complexity,
		"mode":       mode,
		"cost_usd":   topicBudget,
	})

	slog.Info("research-daemon: topic completed",
		"topic", entry.Topic, "domain", entry.Domain,
		"complexity", complexity, "mode", mode, "cost", topicBudget)

	return researchTopicWritten, topicBudget, nil
}

// classifyResearchComplexity returns 1-4 based on max(scope, novelty, impact).
func classifyResearchComplexity(entry *ResearchEntry, dedupConfidence float64) int {
	// Scope: derive from source as a proxy for breadth.
	scope := 2 // default: single-topic research
	switch entry.Source {
	case "freshness":
		scope = 1 // refreshing known content
	case "roadmap":
		scope = 3 // roadmap items tend to be cross-domain
	}

	// Novelty: inverse of dedup confidence.
	var novelty int
	switch {
	case dedupConfidence < 0.2:
		novelty = 4 // very new territory
	case dedupConfidence < 0.4:
		novelty = 3
	case dedupConfidence < 0.7:
		novelty = 2
	default:
		novelty = 1 // well-covered
	}

	// Impact: derive from queue source.
	impact := 2 // default
	switch entry.Source {
	case "freshness":
		impact = 1
	case "gap":
		impact = 3
	case "roadmap":
		impact = 3
	}

	// Return max of all three dimensions.
	m := scope
	if novelty > m {
		m = novelty
	}
	if impact > m {
		m = impact
	}
	return m
}

// budgetForComplexity returns the per-topic budget cap in USD.
func budgetForComplexity(complexity int) float64 {
	switch complexity {
	case 1:
		return 0.10
	case 2:
		return 0.50
	case 3:
		return 2.00
	case 4:
		return 5.00
	default:
		return 0.50
	}
}

// buildResearchPrompt constructs the research instruction for a topic.
func buildResearchPrompt(entry *ResearchEntry, mode string, complexity int) string {
	var modeInstruction string
	switch mode {
	case "expand":
		modeInstruction = "Existing research partially covers this topic. Build on and expand the existing material. Focus on gaps, recent developments, and deeper analysis."
	default:
		modeInstruction = "This is a new research topic with little or no existing coverage. Provide comprehensive, well-structured research."
	}

	return fmt.Sprintf(`Research Topic: %s
Domain: %s
Source: %s
Mode: %s
Complexity: %d/4

%s

Instructions:
- Write clear, structured research with sections and subsections
- Include specific technical details and code examples where relevant
- Cite sources with URLs where possible
- Use kebab-case naming for file references
- Focus on practical, actionable information
- Target audience: experienced software engineers`,
		entry.Topic, entry.Domain, entry.Source, mode, complexity, modeInstruction)
}

func (rd *ResearchDaemon) commitPending(ctx context.Context) {
	rd.mu.Lock()
	pending := rd.pendingCommits
	rd.pendingCommits = 0
	rd.mu.Unlock()

	if pending == 0 {
		return
	}
	msg := fmt.Sprintf("research-daemon: batch commit (%d findings)", pending)
	if err := rd.gateway.CommitAndPush(ctx, msg); err != nil {
		slog.Warn("research-daemon: commit failed", "error", err)
		// Restore count so next tick retries.
		rd.mu.Lock()
		rd.pendingCommits += pending
		rd.mu.Unlock()
	}
}

func (rd *ResearchDaemon) publishEvent(typ events.EventType, data map[string]any) {
	rd.mu.Lock()
	bus := rd.bus
	rd.mu.Unlock()
	if bus == nil {
		return
	}
	bus.Publish(events.Event{
		Type: typ,
		Data: data,
	})
}

func (rd *ResearchDaemon) resetDailyBudgetIfNeeded() {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	if time.Now().After(rd.dailyResetAt) {
		rd.dailySpentUSD = 0
		rd.dailyResetAt = nextMidnight()
		slog.Info("research-daemon: daily budget reset")
	}
}

func nextMidnight() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
}

// ResearchDaemonStats holds observable daemon metrics.
type ResearchDaemonStats struct {
	Enabled            bool      `json:"enabled"`
	TickCount          int       `json:"tick_count"`
	TopicsProcessed    int       `json:"topics_processed"`
	TopicsCompleted    int       `json:"topics_completed"`
	TopicsFailed       int       `json:"topics_failed"`
	ResearchOutputs    int       `json:"research_outputs"`
	DedupSkips         int       `json:"dedup_skips"`
	AutonomyRejections int       `json:"autonomy_rejections"`
	DailySpentUSD      float64   `json:"daily_spent_usd"`
	DailyBudgetUSD     float64   `json:"daily_budget_usd"`
	PendingCommits     int       `json:"pending_commits"`
	LastRunAt          time.Time `json:"last_run_at,omitempty"`
}

// Stats returns current daemon metrics. Thread-safe.
func (rd *ResearchDaemon) Stats() ResearchDaemonStats {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	return ResearchDaemonStats{
		Enabled:            rd.config.Enabled,
		TickCount:          rd.tickCount,
		TopicsProcessed:    rd.topicsProcessed,
		TopicsCompleted:    rd.topicsCompleted,
		TopicsFailed:       rd.topicsFailed,
		ResearchOutputs:    rd.researchOutputs,
		DedupSkips:         rd.dedupSkips,
		AutonomyRejections: rd.autonomyRejects,
		DailySpentUSD:      rd.dailySpentUSD,
		DailyBudgetUSD:     rd.config.BudgetDailyUSD,
		PendingCommits:     rd.pendingCommits,
		LastRunAt:          rd.lastRunAt,
	}
}
