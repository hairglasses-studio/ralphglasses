package session

import (
	"context"
	"encoding/json"
	"time"
)

// TeamWorkerHandle identifies an in-flight backend worker.
type TeamWorkerHandle struct {
	WorkItemID        string `json:"work_item_id,omitempty"`
	A2AAgentURL       string `json:"a2a_agent_url,omitempty"`
	SessionID         string `json:"session_id,omitempty"`
	WorkerNodeID      string `json:"worker_node_id,omitempty"`
	WorktreePath      string `json:"worktree_path,omitempty"`
	WorktreeBranch    string `json:"worktree_branch,omitempty"`
	HeadSHA           string `json:"head_sha,omitempty"`
	MergeBaseSHA      string `json:"merge_base_sha,omitempty"`
	ArtifactType      string `json:"artifact_type,omitempty"`
	ArtifactPath      string `json:"artifact_path,omitempty"`
	ArtifactHash      string `json:"artifact_hash,omitempty"`
	ArtifactSizeBytes int64  `json:"artifact_size_bytes,omitempty"`
	ArtifactBaseRef   string `json:"artifact_base_ref,omitempty"`
	ArtifactTipRef    string `json:"artifact_tip_ref,omitempty"`
	ArtifactStatus    string `json:"artifact_status,omitempty"`
}

// TeamBackendSubmitRequest describes one structured worker launch request.
type TeamBackendSubmitRequest struct {
	TeamName         string
	TaskID           string
	RepoPath         string
	RepoName         string
	Provider         Provider
	Model            string
	Prompt           string
	MaxBudgetUSD     float64
	MaxTurns         int
	SessionName      string
	OutputSchema     json.RawMessage
	PermissionMode   string
	PlannerSessionID string
	WorktreePolicy   string
	TargetBranch     string
	HumanContext     []string
	OwnedPaths       []string
	A2AAgentURL      string
}

// TeamBackendPollResult captures backend-visible worker state.
type TeamBackendPollResult struct {
	Handle        TeamWorkerHandle  `json:"handle"`
	Terminal      bool              `json:"terminal"`
	SessionStatus SessionStatus     `json:"session_status,omitempty"`
	Error         string            `json:"error,omitempty"`
	WorkerResult  *TeamWorkerResult `json:"worker_result,omitempty"`
}

// StructuredTeamBackend executes structured team worker tasks outside the
// manager's direct local session lifecycle.
type StructuredTeamBackend interface {
	Name() string
	Submit(context.Context, TeamBackendSubmitRequest) (TeamWorkerHandle, error)
	Poll(context.Context, TeamWorkerHandle) (*TeamBackendPollResult, error)
	Stop(context.Context, TeamWorkerHandle) error
}

type teamController struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func (m *Manager) SetStructuredTeamBackend(backend StructuredTeamBackend) {
	m.configMu.Lock()
	defer m.configMu.Unlock()
	m.teamBackend = backend
}

func (m *Manager) StructuredTeamBackend() StructuredTeamBackend {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	return m.teamBackend
}

func (m *Manager) ensureTeamControllersLocked() {
	if m.teamControllers == nil {
		m.teamControllers = make(map[string]*teamController)
	}
}

func cloneTeamQuestionSlice(src []TeamQuestion) []TeamQuestion {
	if len(src) == 0 {
		return nil
	}
	out := make([]TeamQuestion, len(src))
	copy(out, src)
	return out
}

func cloneStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	cp := *t
	return &cp
}
