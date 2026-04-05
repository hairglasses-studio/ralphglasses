package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/bandit"
	"github.com/hairglasses-studio/ralphglasses/internal/blackboard"
	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

const (
	claudeCacheRerouteThreshold = 2
	claudeCacheRerouteWindow    = time.Hour
)

// DefaultProviderArms returns bandit arms for each available provider,
// independent of cascade tier configuration.
func DefaultProviderArms() []bandit.Arm {
	return []bandit.Arm{
		{ID: "ultra-cheap", Provider: "gemini", Model: "gemini-2.0-flash-lite"},
		{ID: "worker", Provider: "gemini", Model: "gemini-2.5-flash"},
		{ID: "coding", Provider: "codex", Model: "gpt-5.4"},
		{ID: "reasoning", Provider: "claude", Model: "claude-opus"},
	}
}

func (s *Server) shouldRerouteClaudeForCacheHealth(repoPath string) (bool, int) {
	if s == nil || s.SessMgr == nil {
		return false, 0
	}

	cutoff := time.Now().Add(-claudeCacheRerouteWindow)
	count := 0
	for _, sess := range s.SessMgr.List(repoPath) {
		sess.Lock()
		cacheUnhealthy := sess.Provider == session.ProviderClaude &&
			sess.Resumed &&
			sess.CacheWriteTokens > 0 &&
			sess.CacheReadTokens == 0 &&
			sess.LastActivity.After(cutoff)
		sess.Unlock()
		if cacheUnhealthy {
			count++
		}
	}
	return count >= claudeCacheRerouteThreshold, count
}

func (s *Server) rerouteClaudeProviderForCacheHealth(repoPath string, provider session.Provider, explicit bool) (session.Provider, string) {
	if explicit || provider != session.ProviderClaude {
		return provider, ""
	}
	if ok, count := s.shouldRerouteClaudeForCacheHealth(repoPath); ok {
		target := session.DefaultPrimaryProvider()
		return target, fmt.Sprintf("rerouted from claude to %s after %d recent resumed-session cache anomalies", target, count)
	}
	return provider, ""
}

// wireSubsystems initializes self-learning subsystem singletons on the session
// manager and server. Phase G subsystems (reflexion, episodic, cascade,
// curriculum) and Phase H subsystems (blackboard, cost predictor) are all wired
// here so that both handleLoopStart and handleSelfImprove get the same set.
func wireSubsystems(ctx context.Context, s *Server, sessMgr *session.Manager, ralphDir string) {
	// --- Bandit (independent of cascade) ---
	if s.Bandit == nil {
		s.Bandit = bandit.NewSelector(DefaultProviderArms())
	}

	// --- Phase G subsystems ---
	if !sessMgr.HasReflexion() {
		sessMgr.SetReflexionStore(session.NewReflexionStore(ralphDir))
	}
	if !sessMgr.HasEpisodicMemory() {
		sessMgr.SetEpisodicMemory(session.NewEpisodicMemory(ralphDir, 500, 0))
	}
	if !sessMgr.HasCascadeRouter() {
		repoPath := filepath.Dir(ralphDir)
		cfg := cascadeConfigFromRepo(ctx, repoPath, ralphDir)
		sessMgr.SetCascadeRouter(session.NewCascadeRouter(cfg, nil, nil, ralphDir))
	}
	if !sessMgr.HasCurriculumSorter() {
		var feedback *session.FeedbackAnalyzer
		if s.FeedbackAnalyzer != nil {
			feedback = s.FeedbackAnalyzer
		}
		var episodic session.EpisodicSource
		if em := sessMgr.GetEpisodicMemory(); em != nil {
			episodic = em
		}
		sessMgr.SetCurriculumSorter(session.NewCurriculumSorter(feedback, episodic))
	}

	// Phase G: Wire bandit hooks from cascade tiers into Thompson Sampling.
	if !sessMgr.HasBandit() {
		tiers := session.DefaultModelTiers()
		arms := make([]bandit.Arm, len(tiers))
		for i, t := range tiers {
			arms[i] = bandit.Arm{
				ID:       t.Label,
				Provider: string(t.Provider),
				Model:    t.Model,
			}
		}
		ts := bandit.NewThompsonSampling(arms, 50)
		sessMgr.SetBanditHooks(
			func() (string, string) {
				arm := ts.Select(nil)
				return arm.Provider, arm.Model
			},
			func(provider string, reward float64) {
				for _, a := range arms {
					if a.Provider == provider {
						ts.Update(bandit.Reward{
							ArmID: a.ID,
							Value: reward,
						})
						return
					}
				}
			},
		)
	}

	// Phase G: Wire decision model into cascade router.
	if cr := sessMgr.GetCascadeRouter(); cr != nil {
		if cr.DecisionModelStats() == nil {
			dm := session.NewDecisionModel()
			cr.SetDecisionModel(dm)
		}
	}

	// Phase G: Wire trigram embedder into episodic memory.
	if em := sessMgr.GetEpisodicMemory(); em != nil {
		em.SetEmbedder(session.NewTrigramEmbedder(128))
	}

	// --- Phase H subsystems ---
	if s.Blackboard == nil {
		s.Blackboard = blackboard.NewBlackboard(ralphDir)
	}

	if s.CostPredictor == nil {
		s.CostPredictor = fleet.NewCostPredictor(2.0) // 2.0 = anomaly threshold std devs
	}

	// Wire session-level CostPredictor so loop.go can record costs.
	if !sessMgr.HasCostPredictor() {
		sessMgr.SetCostPredictor(session.NewCostPredictor(ralphDir))
	}

	// --- Prompt enhancer (singleton via engineOnce) ---
	if sessMgr.Enhancer == nil {
		sessMgr.Enhancer = s.getEngine()
	}
}

// cascadeConfigFromRepo reads .ralphrc from the repo and returns a CascadeConfig.
// If the repo has CASCADE_ENABLED=true, settings are loaded from .ralphrc.
// Otherwise, returns the hardcoded DefaultCascadeConfig.
func cascadeConfigFromRepo(ctx context.Context, repoPath, _ string) session.CascadeConfig {
	rc, err := model.LoadConfig(ctx, repoPath)
	if err == nil && rc != nil {
		if ccfg := session.DefaultCascadeFromConfig(rc.Values); ccfg != nil {
			return *ccfg
		}
	}
	return session.DefaultCascadeConfig()
}

// normalizeMetricName maps shorthand metric names to their canonical full names.
// This bridges the gap between eval_changepoints (which uses "cost", "latency")
// and anomaly_detect (which uses "total_cost_usd", "total_latency_ms").
// Unknown names are returned as-is for downstream validation.
func normalizeMetricName(name string) string {
	aliases := map[string]string{
		"cost":       "total_cost_usd",
		"latency":    "total_latency_ms",
		"difficulty": "difficulty_score",
	}
	if canonical, ok := aliases[name]; ok {
		return canonical
	}
	return name
}
