package mcpserver

import (
	"github.com/hairglasses-studio/ralphglasses/internal/bandit"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// DefaultProviderArms returns bandit arms for each available provider,
// independent of cascade tier configuration.
func DefaultProviderArms() []bandit.Arm {
	return []bandit.Arm{
		{Provider: "gemini", Model: "gemini-2.0-flash-lite", Label: "ultra-cheap", CostPer1M: session.CostGeminiFlashLiteInput},
		{Provider: "gemini", Model: "gemini-2.5-flash", Label: "worker", CostPer1M: session.CostGeminiFlashInput},
		{Provider: "claude", Model: "claude-sonnet", Label: "coding", CostPer1M: session.CostClaudeSonnetInput},
		{Provider: "claude", Model: "claude-opus", Label: "reasoning", CostPer1M: session.CostClaudeOpusInput},
	}
}

// wireSubsystems initializes self-learning subsystem singletons on the session
// manager and server. Phase G subsystems (reflexion, episodic, cascade,
// curriculum) and Phase H subsystems (blackboard, cost predictor) are all wired
// here so that both handleLoopStart and handleSelfImprove get the same set.
func wireSubsystems(s *Server, sessMgr *session.Manager, ralphDir string) {
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
		cfg := session.DefaultCascadeConfig()
		sessMgr.SetCascadeRouter(session.NewCascadeRouter(cfg, nil, nil, ralphDir))
	}
	if !sessMgr.HasCurriculumSorter() {
		// B4: Wire FeedbackAnalyzer into CurriculumSorter when available.
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

	// --- Phase H subsystems ---
	if s.Blackboard == nil {
		s.Blackboard = session.NewBlackboard(ralphDir)
	}
	if !sessMgr.HasBlackboard() {
		sessMgr.SetBlackboard(s.Blackboard)
	}

	if s.CostPredictor == nil {
		s.CostPredictor = session.NewCostPredictor(ralphDir)
	}
	if !sessMgr.HasCostPredictor() {
		sessMgr.SetCostPredictor(s.CostPredictor)
	}
}
