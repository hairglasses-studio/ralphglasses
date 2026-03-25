package mcpserver

import "github.com/hairglasses-studio/ralphglasses/internal/session"

// wireSubsystems initializes self-learning subsystem singletons on the session manager.
func wireSubsystems(sessMgr *session.Manager, ralphDir string) {
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
		var episodic session.EpisodicSource
		if em := sessMgr.GetEpisodicMemory(); em != nil {
			episodic = em
		}
		sessMgr.SetCurriculumSorter(session.NewCurriculumSorter(nil, episodic))
	}
}
