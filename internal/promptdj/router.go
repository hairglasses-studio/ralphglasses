package promptdj

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// PromptDJRouter is the prompt-aware routing engine. It sits upstream of
// CascadeRouter and adds quality-based, affinity-driven, cost-aware routing.
type PromptDJRouter struct {
	cascade  *session.CascadeRouter
	bandit   *session.BanditRouter
	feedback *session.FeedbackAnalyzer
	engine   *enhancer.HybridEngine // for enhancement in quality gate
	cache    *session.CacheManager
	config   PromptDJConfig
	affinity *AffinityMatrix
	log      *DecisionLog
	mu       sync.RWMutex
}

// NewPromptDJRouter creates a router with the given subsystems.
// Any subsystem may be nil (the router degrades gracefully).
func NewPromptDJRouter(
	cascade *session.CascadeRouter,
	bandit *session.BanditRouter,
	feedback *session.FeedbackAnalyzer,
	engine *enhancer.HybridEngine,
	cache *session.CacheManager,
	config PromptDJConfig,
	stateDir string,
) *PromptDJRouter {
	var log *DecisionLog
	if config.LogDecisions && stateDir != "" {
		log = NewDecisionLog(stateDir)
	}
	return &PromptDJRouter{
		cascade:  cascade,
		bandit:   bandit,
		feedback: feedback,
		engine:   engine,
		cache:    cache,
		config:   config,
		affinity: NewAffinityMatrix(),
		log:      log,
	}
}

// Route executes the 10-phase decision tree and returns a routing decision.
func (r *PromptDJRouter) Route(ctx context.Context, req RoutingRequest) (*RoutingDecision, error) {
	start := time.Now()
	d := &RoutingDecision{
		DecisionID: uuid.New().String(),
		Timestamp:  start,
	}

	// ── Phase 1: Classify ──────────────────────────────────────────────
	taskType := req.TaskType
	var classConf float64 = 1.0
	if taskType == "" {
		best, _ := enhancer.ClassifyDetailed(req.Prompt)
		taskType = best.TaskType
		classConf = best.Confidence
	}
	d.TaskType = taskType

	// ── Phase 2: Score ─────────────────────────────────────────────────
	score := req.Score
	if score == 0 {
		ar := enhancer.Analyze(req.Prompt)
		score = ar.Score
		if ar.ScoreReport != nil {
			score = ar.ScoreReport.Overall
		}
	}
	d.OriginalScore = score
	qualityTier := QualityTierFromScore(score)

	// ── Phase 3: Quality Gate ──────────────────────────────────────────
	if qualityTier == QualityLow && r.config.EnhanceThreshold > 0 && score < r.config.EnhanceThreshold {
		enhanced := enhancer.EnhanceWithConfig(req.Prompt, taskType, enhancer.ResolveConfig("."))
		if enhanced.Enhanced != req.Prompt {
			d.EnhancedPrompt = enhanced.Enhanced
			d.WasEnhanced = true
			// Re-score
			ar2 := enhancer.Analyze(enhanced.Enhanced)
			newScore := ar2.Score
			if ar2.ScoreReport != nil {
				newScore = ar2.ScoreReport.Overall
			}
			d.EnhancedScore = newScore
			// If significant improvement, re-classify
			if newScore-score > 20 {
				best2, _ := enhancer.ClassifyDetailed(enhanced.Enhanced)
				taskType = best2.TaskType
				d.TaskType = taskType
				classConf = best2.Confidence
			}
			score = newScore
			qualityTier = QualityTierFromScore(score)
		}
	}

	// ── Phase 4: Domain Inference ──────────────────────────────────────
	domainTags := req.Tags
	if len(domainTags) == 0 {
		domainTags = inferDomainTags(req.Prompt, req.Repo)
	}
	d.DomainTags = domainTags

	// ── Phase 5: Affinity Lookup ───────────────────────────────────────
	entries := r.affinity.Lookup(taskType, qualityTier)
	if len(entries) == 0 {
		entries = r.affinity.Lookup(enhancer.TaskTypeGeneral, qualityTier)
	}

	// Apply domain boosts
	entries = applyDomainBoosts(entries, domainTags)

	var topEntry AffinityEntry
	if len(entries) > 0 {
		topEntry = entries[0]
	} else {
		topEntry = AffinityEntry{Provider: session.ProviderClaude, Model: "claude-opus", Weight: 0.5}
	}

	// ── Phase 6: Cascade Reconciliation ────────────────────────────────
	complexity := session.TaskTypeComplexity(string(taskType))
	d.Complexity = complexity

	var cascadeTier session.ModelTier
	if r.cascade != nil {
		cascadeTier = r.cascade.SelectTier(string(taskType), complexity)
	}

	// Affinity wins at high confidence, cascade wins otherwise
	if topEntry.Weight >= 0.8 || r.cascade == nil {
		d.Provider = topEntry.Provider
		d.Model = topEntry.Model
		if d.Model == "" {
			d.Model = cascadeTier.Model
		}
	} else {
		d.Provider = cascadeTier.Provider
		d.Model = cascadeTier.Model
	}
	d.ModelTier = cascadeTier
	d.CostTier = cascadeTier.Label

	// ── Phase 7: Confidence Scoring ────────────────────────────────────
	var successRate float64 = 0.5
	banditReady := false
	if r.bandit != nil {
		successRate = r.bandit.ProviderSuccessRate(string(d.Provider))
		banditReady = r.bandit.Ready()
	}

	conf := ComputeConfidence(ConfidenceComponents{
		ClassificationConf: classConf,
		QualityScore:       float64(score) / 100.0,
		AffinityStrength:   topEntry.Weight,
		HistoricalSuccess:  successRate,
		LatencyHealth:      LatencyHealthScore(r.cascade, d.Provider),
		DomainSpecificity:  DomainSpecificityScore(domainTags),
	}, banditReady, d.WasEnhanced, d.EnhancedScore-d.OriginalScore)

	d.Confidence = conf
	d.ConfidenceLevel = ConfidenceLevelFromScore(conf)

	// If confidence is too low, escalate to highest tier
	if conf < r.config.MinConfidence {
		d.Provider = session.ProviderClaude
		d.Model = "claude-opus"
		d.Rationale = fmt.Sprintf("Low confidence (%.2f < %.2f), escalated to Claude Opus", conf, r.config.MinConfidence)
	} else {
		d.Rationale = fmt.Sprintf("Routed %s/%s prompt (score %d, %s quality) to %s/%s (confidence %.2f)",
			taskType, qualityTier, score, d.ConfidenceLevel, d.Provider, d.Model, conf)
	}

	// ── Phase 8: Agent Profile ─────────────────────────────────────────
	d.AgentProfile = selectAgentProfile(taskType, domainTags, d.Provider)

	// ── Phase 9: Cost Estimate ─────────────────────────────────────────
	tokens := estimateTokens(req.Prompt, taskType)
	d.EstimatedCostUSD = estimateCost(tokens, d.Provider, d.Model)

	// ── Phase 10: Fallback Chain ───────────────────────────────────────
	d.FallbackChain = buildFallbackChain(entries, d.Provider)

	d.LatencyMs = time.Since(start).Milliseconds()

	// Persist decision
	if r.log != nil {
		_ = r.log.RecordDecision(d)
	}

	return d, nil
}

// RoutePrompt satisfies the session.PromptRouter interface, enabling the
// Manager to call the DJ router without an import cycle.
func (r *PromptDJRouter) RoutePrompt(ctx context.Context, prompt, repo string, score int) (any, error) {
	return r.Route(ctx, RoutingRequest{Prompt: prompt, Repo: repo, Score: score})
}

// Compile-time check that PromptDJRouter satisfies session.PromptRouter.
var _ session.PromptRouter = (*PromptDJRouter)(nil)

// GetDecisionLog returns the decision log (may be nil if logging disabled).
func (r *PromptDJRouter) GetDecisionLog() *DecisionLog {
	return r.log
}

// Config returns the current configuration.
func (r *PromptDJRouter) Config() PromptDJConfig {
	return r.config
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// inferDomainTags guesses domain tags from prompt content and repo name.
func inferDomainTags(prompt, repo string) []string {
	lower := strings.ToLower(prompt + " " + repo)
	var tags []string

	domainKeywords := map[string][]string{
		"go":         {"golang", "go build", "go test", "go.mod", "go func", "package main"},
		"mcp":        {"mcp", "tool", "handler", "registry"},
		"shader":     {"shader", "glsl", "spirv", "fragment", "vertex"},
		"terminal":   {"terminal", "ghostty", "foot", "tmux", "shell"},
		"agents":     {"agent", "orchestrat", "fleet", "ralph"},
		"rice":       {"rice", "hyprland", "eww", "waybar", "mako"},
		"tui":        {"tui", "bubble tea", "bubbletea", "view", "model"},
		"testing":    {"test", "bench", "coverage", "assert"},
		"security":   {"security", "auth", "credential", "secret", "encrypt"},
		"deployment": {"deploy", "ci", "cd", "pipeline", "docker", "k8s"},
	}

	for domain, keywords := range domainKeywords {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				tags = append(tags, domain)
				break
			}
		}
	}

	if len(tags) == 0 {
		tags = []string{"general"}
	}
	return tags
}

// applyDomainBoosts adjusts affinity weights based on domain tags.
func applyDomainBoosts(entries []AffinityEntry, tags []string) []AffinityEntry {
	if len(tags) == 0 || len(entries) == 0 {
		return entries
	}

	boosted := make([]AffinityEntry, len(entries))
	copy(boosted, entries)

	for i := range boosted {
		for _, tag := range tags {
			if boosts, ok := DomainBoosts[tag]; ok {
				if delta, ok := boosts[boosted[i].Provider]; ok {
					boosted[i].Weight = clamp(boosted[i].Weight+delta, 0, 1)
				}
			}
		}
	}

	// Re-sort by weight descending
	for i := 0; i < len(boosted); i++ {
		for j := i + 1; j < len(boosted); j++ {
			if boosted[j].Weight > boosted[i].Weight {
				boosted[i], boosted[j] = boosted[j], boosted[i]
			}
		}
	}

	return boosted
}

// selectAgentProfile picks an agent profile based on task type and domain.
func selectAgentProfile(taskType enhancer.TaskType, tags []string, provider session.Provider) string {
	switch taskType {
	case enhancer.TaskTypeCode:
		return "code-specialist"
	case enhancer.TaskTypeAnalysis:
		return "research-analyst"
	case enhancer.TaskTypeTroubleshooting:
		return "debugger"
	case enhancer.TaskTypeCreative:
		return "creative-director"
	case enhancer.TaskTypeWorkflow:
		return "workflow-architect"
	default:
		return "general"
	}
}

// estimateTokens estimates input+output tokens for cost calculation.
func estimateTokens(prompt string, taskType enhancer.TaskType) int {
	inputTokens := len([]rune(prompt)) / 4 // ~4 chars per token

	// Output token estimates by task type (from cost-aware-routing-design.md)
	outputTokens := 1500 // default
	switch taskType {
	case enhancer.TaskTypeWorkflow:
		outputTokens = 200
	case enhancer.TaskTypeCreative:
		outputTokens = 500
	case enhancer.TaskTypeCode:
		outputTokens = 3000
	case enhancer.TaskTypeAnalysis:
		outputTokens = 5000
	case enhancer.TaskTypeTroubleshooting:
		outputTokens = 1500
	}

	return inputTokens + outputTokens
}

// estimateCost calculates estimated USD cost for a given token count.
func estimateCost(tokens int, provider session.Provider, model string) float64 {
	// Rough per-million-token rates
	rates := map[session.Provider]float64{
		session.ProviderClaude: 15.0,  // opus average
		session.ProviderGemini: 1.25,  // flash
		session.ProviderCodex:  10.0,  // gpt-5
	}
	rate, ok := rates[provider]
	if !ok {
		rate = 10.0
	}
	// Sonnet is cheaper
	if strings.Contains(model, "sonnet") {
		rate = 3.0
	}
	return float64(tokens) / 1_000_000.0 * rate
}

// buildFallbackChain generates ordered alternatives from affinity entries.
func buildFallbackChain(entries []AffinityEntry, primary session.Provider) []FallbackRoute {
	var chain []FallbackRoute
	for _, e := range entries {
		if e.Provider == primary {
			continue
		}
		chain = append(chain, FallbackRoute{
			Provider:   e.Provider,
			Model:      e.Model,
			Reason:     fmt.Sprintf("Alternative (affinity %.2f)", e.Weight),
			Confidence: e.Weight,
		})
	}

	// Always include Claude Opus as final safety net
	hasOpus := false
	for _, fb := range chain {
		if fb.Provider == session.ProviderClaude && strings.Contains(fb.Model, "opus") {
			hasOpus = true
			break
		}
	}
	if !hasOpus && primary != session.ProviderClaude {
		chain = append(chain, FallbackRoute{
			Provider:   session.ProviderClaude,
			Model:      "claude-opus",
			Reason:     "Safety net (highest capability)",
			Confidence: 0.65,
		})
	}

	return chain
}
