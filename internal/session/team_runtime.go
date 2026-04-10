package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/worktree"
)

const (
	defaultTeamMaxConcurrency = 2
	defaultTeamMaxRetries     = 2
	defaultTeamWorkerStallAge = 10 * time.Minute
)

type TeamPlannerAction struct {
	Type            string   `json:"type"`
	TaskID          string   `json:"task_id,omitempty"`
	Provider        Provider `json:"provider,omitempty"`
	Model           string   `json:"model,omitempty"`
	Prompt          string   `json:"prompt,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	Reason          string   `json:"reason,omitempty"`
	Question        string   `json:"question,omitempty"`
	WorkerSessionID string   `json:"worker_session_id,omitempty"`
	OwnedPaths      []string `json:"owned_paths,omitempty"`
}

type TeamPlannerResponse struct {
	Summary string              `json:"summary,omitempty"`
	Actions []TeamPlannerAction `json:"actions"`
}

type TeamWorkerResult struct {
	TaskID       string   `json:"task_id"`
	Status       string   `json:"status"`
	Summary      string   `json:"summary,omitempty"`
	Question     string   `json:"question,omitempty"`
	ChangedFiles []string `json:"changed_files,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type TeamStepResult struct {
	Team          *TeamStatus         `json:"team"`
	Actions       []TeamPlannerAction `json:"actions,omitempty"`
	PlannerOutput string              `json:"planner_output,omitempty"`
	WorkerUpdates []string            `json:"worker_updates,omitempty"`
}

type teamExecutor interface {
	Launch(context.Context, LaunchOptions) (*Session, error)
	Stop(string) error
}

type localTeamExecutor struct {
	mgr *Manager
}

func (e localTeamExecutor) Launch(ctx context.Context, opts LaunchOptions) (*Session, error) {
	return e.mgr.Launch(ctx, opts)
}

func (e localTeamExecutor) Stop(id string) error {
	return e.mgr.Stop(id)
}

func (m *Manager) teamExecutor() teamExecutor {
	return localTeamExecutor{mgr: m}
}

func normalizeTeamConfig(config TeamConfig) TeamConfig {
	config.TenantID = NormalizeTenantID(config.TenantID)
	if config.Provider == "" {
		config.Provider = DefaultPrimaryProvider()
	}
	if config.WorkerProvider == "" {
		config.WorkerProvider = config.Provider
	}
	if config.Model == "" {
		config.Model = ProviderDefaults(config.Provider)
	}
	if config.WorkerModel == "" {
		config.WorkerModel = ProviderDefaults(config.WorkerProvider)
	}
	if config.MaxConcurrency <= 0 {
		config.MaxConcurrency = defaultTeamMaxConcurrency
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = defaultTeamMaxRetries
	}
	if strings.TrimSpace(config.ExecutionBackend) == "" {
		config.ExecutionBackend = TeamExecutionBackendLocal
	}
	if strings.TrimSpace(config.WorktreePolicy) == "" {
		config.WorktreePolicy = TeamWorktreePolicyPerWorker
	}
	if strings.TrimSpace(config.TargetBranch) == "" {
		config.TargetBranch = "main"
	}
	return config
}

func newStructuredCodexTeam(config TeamConfig, resolved ResolvedTeamConfig) *TeamStatus {
	config = normalizeTeamConfig(config)
	now := time.Now()
	tasks := make([]TeamTask, 0, len(config.Tasks))
	for i, desc := range config.Tasks {
		tasks = append(tasks, TeamTask{
			ID:          fmt.Sprintf("task-%d", i+1),
			Title:       sanitizeTaskTitle(desc),
			Description: desc,
			Status:      TeamTaskPending,
			MergeStatus: TeamMergeStatusPending,
			UpdatedAt:   now,
		})
	}
	return &TeamStatus{
		Name:                    config.Name,
		TenantID:                config.TenantID,
		RepoPath:                config.RepoPath,
		Provider:                config.Provider,
		WorkerProvider:          config.WorkerProvider,
		Model:                   config.Model,
		WorkerModel:             config.WorkerModel,
		ProviderAutoSelected:    resolved.ProviderAutoSelected,
		ProviderSelectionReason: resolved.ProviderSelectionReason,
		Status:                  StatusRunning,
		RunState:                TeamRunStateRunning,
		Runtime:                 resolved.Runtime,
		ExecutionBackend:        config.ExecutionBackend,
		WorktreePolicy:          config.WorktreePolicy,
		Tasks:                   tasks,
		CreatedAt:               now,
		UpdatedAt:               now,
		MaxBudgetUSD:            config.MaxBudgetUSD,
		MaxConcurrency:          config.MaxConcurrency,
		MaxTaskRetries:          config.MaxRetries,
		AutoStart:               config.AutoStart,
		TargetBranch:            config.TargetBranch,
		IntegrationBranch:       structuredTeamIntegrationBranch(config.Name),
		PromotionStatus:         TeamPromotionStatusPending,
		A2AAgentURL:             config.A2AAgentURL,
	}
}

func cloneTeamStatus(team *TeamStatus) *TeamStatus {
	if team == nil {
		return nil
	}
	cp := *team
	if len(team.Tasks) > 0 {
		cp.Tasks = make([]TeamTask, len(team.Tasks))
		for i, task := range team.Tasks {
			cp.Tasks[i] = task
			cp.Tasks[i].ChangedFiles = cloneStringSlice(task.ChangedFiles)
			cp.Tasks[i].HumanContext = cloneStringSlice(task.HumanContext)
			cp.Tasks[i].OwnedPaths = cloneStringSlice(task.OwnedPaths)
			cp.Tasks[i].ConflictFiles = cloneStringSlice(task.ConflictFiles)
			cp.Tasks[i].StartedAt = cloneTimePtr(task.StartedAt)
			cp.Tasks[i].EndedAt = cloneTimePtr(task.EndedAt)
		}
	}
	if team.PendingQuestion != nil {
		pq := *team.PendingQuestion
		pq.AnsweredAt = cloneTimePtr(team.PendingQuestion.AnsweredAt)
		cp.PendingQuestion = &pq
	}
	cp.ResolvedQuestions = cloneTeamQuestionSlice(team.ResolvedQuestions)
	for i := range cp.ResolvedQuestions {
		cp.ResolvedQuestions[i].AnsweredAt = cloneTimePtr(cp.ResolvedQuestions[i].AnsweredAt)
	}
	return &cp
}

func structuredTeamIntegrationBranch(name string) string {
	return "ralph-team-" + sanitizeLoopName(name) + "-integration"
}

func structuredTeamIntegrationPath(team *TeamStatus) string {
	if team == nil || team.RepoPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(team.RepoPath), ".ralph-integrations", filepath.Base(team.RepoPath), sanitizeTenantPathSegment(team.TenantID), sanitizeLoopName(team.Name))
}

func structuredTeamTaskBranch(team *TeamStatus, task *TeamTask) string {
	suffix := task.ID
	if task.Attempt > 0 {
		suffix = fmt.Sprintf("%s-a%d", task.ID, task.Attempt)
	}
	return "ralph-team-" + sanitizeLoopName(team.Name) + "-" + sanitizeLoopName(suffix)
}

func structuredTeamTaskWorktreePath(team *TeamStatus, task *TeamTask) string {
	if team == nil || team.RepoPath == "" {
		return ""
	}
	attempt := task.Attempt
	if attempt <= 0 {
		attempt = 1
	}
	return filepath.Join(filepath.Dir(team.RepoPath), ".ralph-worktrees", filepath.Base(team.RepoPath), sanitizeTenantPathSegment(team.TenantID), sanitizeLoopName(team.Name), sanitizeLoopName(task.ID), fmt.Sprintf("attempt-%d", attempt))
}

func teamUsesFleetBackend(team *TeamStatus) bool {
	return team != nil && team.ExecutionBackend == TeamExecutionBackendFleet
}

func teamUsesRemoteArtifactBackend(team *TeamStatus) bool {
	return team != nil && (team.ExecutionBackend == TeamExecutionBackendFleet || team.ExecutionBackend == TeamExecutionBackendA2A)
}

func taskHandle(task TeamTask) TeamWorkerHandle {
	return TeamWorkerHandle{
		WorkItemID:        task.WorkItemID,
		A2AAgentURL:       task.A2AAgentURL,
		SessionID:         task.WorkerSessionID,
		WorkerNodeID:      task.WorkerNodeID,
		WorktreePath:      task.WorktreePath,
		WorktreeBranch:    task.WorktreeBranch,
		HeadSHA:           task.HeadSHA,
		MergeBaseSHA:      task.MergeBaseSHA,
		ArtifactType:      task.ArtifactType,
		ArtifactPath:      task.ArtifactPath,
		ArtifactHash:      task.ArtifactHash,
		ArtifactSizeBytes: task.ArtifactSizeBytes,
		ArtifactBaseRef:   task.ArtifactBaseRef,
		ArtifactTipRef:    task.ArtifactTipRef,
		ArtifactStatus:    task.ArtifactStatus,
	}
}

func teamStateFilename(name string) string {
	return sanitizeLoopName(name) + ".json"
}

func (m *Manager) teamSnapshot(name string) (*TeamStatus, bool) {
	m.workersMu.RLock()
	defer m.workersMu.RUnlock()
	team, ok := m.teams[name]
	if !ok {
		return nil, false
	}
	return cloneTeamStatus(team), true
}

func (m *Manager) writeTeamSnapshot(team *TeamStatus) error {
	if team == nil || team.Name == "" {
		return nil
	}
	team.TenantID = NormalizeTenantID(team.TenantID)
	dir := m.teamStateDirForTenant(team.TenantID)
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("persist team: mkdir: %w", err)
	}
	data, err := json.Marshal(team)
	if err != nil {
		return fmt.Errorf("persist team: marshal: %w", err)
	}
	path := m.teamStatePath(team.TenantID, team.Name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("persist team: write %s: %w", path, err)
	}
	if legacyPath := filepath.Join(m.teamStateRootDir(), teamStateFilename(team.Name)); legacyPath != path {
		_ = os.Remove(legacyPath)
	}
	return nil
}

func (m *Manager) persistTeamOrWarn(name string, reason string) {
	team, ok := m.teamSnapshot(name)
	if !ok {
		return
	}
	if err := m.writeTeamSnapshot(team); err != nil {
		slog.Warn("failed to persist team", "team", name, "reason", reason, "error", err)
	}
}

func (m *Manager) RehydrateTeams() error {
	root := m.teamStateRootDir()
	if root == "" {
		return nil
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("rehydrate teams: stat dir: %w", err)
	}
	files := m.discoverTeamStateFiles()

	var autoStart []string

	m.workersMu.Lock()

	for _, file := range files {
		data, err := os.ReadFile(file.Path)
		if err != nil {
			continue
		}
		var team TeamStatus
		if err := json.Unmarshal(data, &team); err != nil {
			continue
		}
		if team.Name == "" {
			continue
		}
		team.TenantID = NormalizeTenantID(team.TenantID)
		if file.Legacy {
			team.TenantID = DefaultTenantID
		}
		key := m.teamKey(team.Name, team.TenantID)
		if _, exists := m.teams[key]; exists {
			continue
		}
		if team.Runtime == "" {
			team.Runtime = TeamRuntimeLegacyLead
		}
		if team.RunState == "" {
			switch team.Status {
			case StatusCompleted:
				team.RunState = TeamRunStateCompleted
			case StatusErrored, StatusStopped:
				team.RunState = TeamRunStateFailed
			default:
				team.RunState = TeamRunStateRunning
			}
		}
		if team.ExecutionBackend == "" {
			team.ExecutionBackend = TeamExecutionBackendLocal
		}
		if team.WorktreePolicy == "" {
			team.WorktreePolicy = TeamWorktreePolicyPerWorker
		}
		if team.TargetBranch == "" {
			team.TargetBranch = "main"
		}
		if team.IntegrationBranch == "" && isStructuredCodexTeam(&team) {
			team.IntegrationBranch = structuredTeamIntegrationBranch(team.Name)
		}
		m.teams[key] = &team
		if isStructuredCodexTeam(&team) && team.AutoStart && team.RunState == TeamRunStateRunning {
			autoStart = append(autoStart, key)
		}
	}
	m.workersMu.Unlock()

	for _, name := range autoStart {
		if _, err := m.StartTeam(context.Background(), name); err != nil {
			slog.Warn("failed to rehydrate team controller", "team", name, "error", err)
		}
	}
	return nil
}

func teamPlannerOutputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "summary": {"type": "string"},
    "actions": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "type": {"type": "string", "enum": ["launch_worker", "retry_worker", "stop_worker", "mark_complete", "mark_blocked", "ask_human"]},
          "task_id": {"type": "string"},
          "provider": {"type": "string"},
          "model": {"type": "string"},
          "prompt": {"type": "string"},
          "summary": {"type": "string"},
          "reason": {"type": "string"},
          "question": {"type": "string"},
          "worker_session_id": {"type": "string"},
          "owned_paths": {
            "type": "array",
            "items": {"type": "string"}
          }
        },
        "required": ["type"]
      }
    }
  },
  "required": ["actions"]
}`)
}

func teamWorkerOutputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "task_id": {"type": "string"},
    "status": {"type": "string", "enum": ["completed", "blocked", "failed", "needs_retry"]},
    "summary": {"type": "string"},
    "question": {"type": "string"},
    "changed_files": {
      "type": "array",
      "items": {"type": "string"}
    },
    "error": {"type": "string"}
  },
  "required": ["task_id", "status"]
}`)
}

func parseTeamPlannerResponse(sess *Session) (TeamPlannerResponse, string, error) {
	output := sessionOutputSummary(sess)
	var resp TeamPlannerResponse
	for _, candidate := range plannerJSONCandidates(output) {
		if _, err := tryUnmarshalWithRepair(candidate, &resp); err == nil {
			if err := validateTeamPlannerResponse(resp); err == nil {
				return resp, candidate, nil
			}
		}
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()
	for _, candidate := range []string{sess.LastOutput, strings.Join(sess.OutputHistory, "\n")} {
		for _, jsonCandidate := range plannerJSONCandidates(candidate) {
			if _, err := tryUnmarshalWithRepair(jsonCandidate, &resp); err == nil {
				if err := validateTeamPlannerResponse(resp); err == nil {
					return resp, jsonCandidate, nil
				}
			}
		}
	}
	return TeamPlannerResponse{}, output, fmt.Errorf("planner output did not match team action schema")
}

func validateTeamPlannerResponse(resp TeamPlannerResponse) error {
	if len(resp.Actions) == 0 {
		return fmt.Errorf("planner returned no actions")
	}
	for _, action := range resp.Actions {
		switch action.Type {
		case "launch_worker", "retry_worker", "mark_complete", "mark_blocked":
			if strings.TrimSpace(action.TaskID) == "" {
				return fmt.Errorf("%s action missing task_id", action.Type)
			}
			if (action.Type == "launch_worker" || action.Type == "retry_worker") && len(teamCompactNonEmpty(action.OwnedPaths)) == 0 {
				return fmt.Errorf("%s action requires owned_paths", action.Type)
			}
		case "stop_worker":
			if strings.TrimSpace(action.TaskID) == "" && strings.TrimSpace(action.WorkerSessionID) == "" {
				return fmt.Errorf("stop_worker action requires task_id or worker_session_id")
			}
		case "ask_human":
			if strings.TrimSpace(action.Question) == "" {
				return fmt.Errorf("ask_human action requires question")
			}
		default:
			return fmt.Errorf("unknown planner action type %q", action.Type)
		}
	}
	return nil
}

func parseTeamWorkerResult(sess *Session) (TeamWorkerResult, string, error) {
	output := sessionOutputSummary(sess)
	var result TeamWorkerResult
	for _, candidate := range plannerJSONCandidates(output) {
		if _, err := tryUnmarshalWithRepair(candidate, &result); err == nil {
			if err := validateTeamWorkerResult(result); err == nil {
				return result, candidate, nil
			}
		}
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()
	for _, candidate := range []string{sess.LastOutput, strings.Join(sess.OutputHistory, "\n")} {
		for _, jsonCandidate := range plannerJSONCandidates(candidate) {
			if _, err := tryUnmarshalWithRepair(jsonCandidate, &result); err == nil {
				if err := validateTeamWorkerResult(result); err == nil {
					return result, jsonCandidate, nil
				}
			}
		}
	}
	return TeamWorkerResult{}, output, fmt.Errorf("worker output did not match team result schema")
}

func validateTeamWorkerResult(result TeamWorkerResult) error {
	if strings.TrimSpace(result.TaskID) == "" {
		return fmt.Errorf("worker result missing task_id")
	}
	switch result.Status {
	case TeamTaskCompleted, TeamTaskBlocked, TeamTaskFailed, TeamTaskNeedsRetry:
	default:
		return fmt.Errorf("worker result status %q is invalid", result.Status)
	}
	if result.Status == TeamTaskBlocked && strings.TrimSpace(result.Question) == "" && strings.TrimSpace(result.Summary) == "" {
		return fmt.Errorf("blocked worker result requires question or summary")
	}
	if result.Status != TeamTaskBlocked && strings.TrimSpace(result.Summary) == "" {
		return fmt.Errorf("worker result requires summary")
	}
	return nil
}

func (m *Manager) buildStructuredTeamPlannerPrompt(team *TeamStatus) string {
	var taskLines []string
	activeWorkers := 0
	for _, task := range team.Tasks {
		if task.Status == TeamTaskInProgress {
			activeWorkers++
		}
		line := fmt.Sprintf("- %s [%s] attempts=%d: %s", task.ID, task.Status, task.Attempt, task.Description)
		if len(task.OwnedPaths) > 0 {
			line += fmt.Sprintf(" | owned_paths=%s", strings.Join(task.OwnedPaths, ","))
		}
		if task.Summary != "" {
			line += fmt.Sprintf(" | summary=%s", truncateForPrompt(task.Summary, 160))
		}
		if task.LastError != "" {
			line += fmt.Sprintf(" | error=%s", truncateForPrompt(task.LastError, 160))
		}
		if task.BlockedQuestion != "" {
			line += fmt.Sprintf(" | blocked=%s", truncateForPrompt(task.BlockedQuestion, 160))
		}
		if task.WorkerSessionID != "" && task.Status == TeamTaskInProgress {
			line += fmt.Sprintf(" | worker_session_id=%s", task.WorkerSessionID)
		}
		taskLines = append(taskLines, line)
	}

	var resolvedQuestions []string
	for _, q := range team.ResolvedQuestions {
		if q.Answer == "" {
			continue
		}
		label := q.ID
		if q.TaskID != "" {
			label = q.TaskID
		}
		resolvedQuestions = append(resolvedQuestions, fmt.Sprintf("- %s question=%s answer=%s", label, q.Question, q.Answer))
	}

	availableSlots := team.MaxConcurrency - activeWorkers
	if availableSlots < 0 {
		availableSlots = 0
	}

	activeClaims := m.teamPathClaimsForRepo(team.TenantID, team.RepoPath)

	var b strings.Builder
	b.WriteString("You are the planner for a harness-owned Codex team run.\n\n")
	b.WriteString("The harness owns task state, launches workers, and validates outputs.\n")
	b.WriteString("Respond ONLY with JSON matching the provided schema.\n\n")
	b.WriteString("Valid actions:\n")
	b.WriteString("- launch_worker: start a new worker for a pending task_id\n")
	b.WriteString("- retry_worker: retry a needs_retry or failed task_id\n")
	b.WriteString("- stop_worker: stop a stuck worker by task_id or worker_session_id\n")
	b.WriteString("- mark_complete: mark a task complete without launching a worker\n")
	b.WriteString("- mark_blocked: mark a task blocked and supply a question or reason\n")
	b.WriteString("- ask_human: pause the team for a specific question\n\n")
	b.WriteString("Constraints:\n")
	b.WriteString(fmt.Sprintf("- You have %d worker slots available in this step.\n", availableSlots))
	b.WriteString(fmt.Sprintf("- Max retries per task: %d.\n", team.MaxTaskRetries))
	b.WriteString("- Never invent task IDs.\n")
	b.WriteString("- launch_worker and retry_worker must include owned_paths.\n")
	b.WriteString("- owned_paths must be repo-relative paths or directories you intend to modify.\n")
	b.WriteString("- Prefer launching workers for runnable tasks over prose.\n")
	b.WriteString("- Use ask_human only when a conservative autonomous default is unsafe.\n\n")
	b.WriteString("Current tasks:\n")
	b.WriteString(strings.Join(taskLines, "\n"))
	if len(activeClaims) > 0 {
		b.WriteString("\n\nActive path claims:\n")
		for _, claim := range activeClaims {
			b.WriteString("- " + claim + "\n")
		}
	}
	if len(resolvedQuestions) > 0 {
		b.WriteString("\n\nResolved human input:\n")
		b.WriteString(strings.Join(resolvedQuestions, "\n"))
	}
	return b.String()
}

func buildStructuredTeamWorkerPrompt(team *TeamStatus, task *TeamTask) string {
	var b strings.Builder
	providerName := "Codex"
	switch taskEffectiveProvider(team, task) {
	case ProviderGemini:
		providerName = "Gemini"
	case ProviderClaude:
		providerName = "Claude"
	}
	b.WriteString(fmt.Sprintf("You are a %s worker executing exactly one harness-managed task.\n\n", providerName))
	b.WriteString(fmt.Sprintf("Team: %s\n", team.Name))
	b.WriteString(fmt.Sprintf("Task ID: %s\n", task.ID))
	b.WriteString(fmt.Sprintf("Task: %s\n", task.Description))
	if task.Attempt > 1 {
		b.WriteString(fmt.Sprintf("Attempt: %d\n", task.Attempt))
	}
	if task.LastError != "" {
		b.WriteString(fmt.Sprintf("Previous failure: %s\n", truncateForPrompt(task.LastError, 300)))
	}
	if len(task.HumanContext) > 0 {
		b.WriteString("\nResolved human input:\n")
		for _, item := range task.HumanContext {
			b.WriteString("- " + item + "\n")
		}
	}
	b.WriteString("\nExecution rules:\n")
	b.WriteString("- Work autonomously using conservative defaults.\n")
	b.WriteString("- Do not ask follow-up questions unless you are genuinely blocked by missing external facts or credentials.\n")
	b.WriteString("- Keep the response machine-readable and consistent with the output schema.\n")
	b.WriteString("- changed_files must be repo-relative paths.\n")
	if len(task.OwnedPaths) > 0 {
		b.WriteString("- Restrict edits to these owned_paths:\n")
		for _, owned := range task.OwnedPaths {
			b.WriteString("  - " + owned + "\n")
		}
	}
	b.WriteString("\nReturn one JSON object with:\n")
	b.WriteString(`{"task_id":"` + task.ID + `","status":"completed|blocked|failed|needs_retry","summary":"...","question":"...","changed_files":["path"],"error":"..."}`)
	return b.String()
}

func isStructuredCodexTeam(team *TeamStatus) bool {
	return team != nil && team.Runtime == TeamRuntimeStructuredCodex
}

func isRunnableTeamTask(task TeamTask) bool {
	return task.Status == TeamTaskPending || task.Status == TeamTaskNeedsRetry
}

func isTerminalTeamTask(task TeamTask) bool {
	switch task.Status {
	case TeamTaskCompleted, TeamTaskFailed, TeamTaskCancelled:
		return true
	default:
		return false
	}
}

func nextTeamTaskID(tasks []TeamTask) string {
	maxID := 0
	for _, task := range tasks {
		if !strings.HasPrefix(task.ID, "task-") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(task.ID, "task-"))
		if err != nil {
			continue
		}
		if n > maxID {
			maxID = n
		}
	}
	return fmt.Sprintf("task-%d", maxID+1)
}

func findTaskIndex(tasks []TeamTask, taskID string) int {
	for i := range tasks {
		if tasks[i].ID == taskID {
			return i
		}
	}
	return -1
}

func recalculateTeamLocked(team *TeamStatus) {
	now := time.Now()
	team.UpdatedAt = now
	if team.PendingQuestion != nil {
		team.Status = StatusRunning
		team.RunState = TeamRunStateAwaitingInput
		return
	}

	allDone := len(team.Tasks) > 0
	hasFailure := false
	for _, task := range team.Tasks {
		switch task.Status {
		case TeamTaskCompleted, TeamTaskCancelled:
		case TeamTaskFailed:
			hasFailure = true
		default:
			allDone = false
		}
	}

	switch {
	case allDone && !hasFailure:
		team.Status = StatusCompleted
		team.RunState = TeamRunStateCompleted
	case allDone && hasFailure:
		team.Status = StatusErrored
		team.RunState = TeamRunStateFailed
	default:
		team.Status = StatusRunning
		team.RunState = TeamRunStateRunning
	}
}

func taskEffectiveProvider(team *TeamStatus, task *TeamTask) Provider {
	if task.Provider != "" {
		return task.Provider
	}
	if team.WorkerProvider != "" {
		return team.WorkerProvider
	}
	if team.Provider != "" {
		return team.Provider
	}
	return DefaultPrimaryProvider()
}

func taskEffectiveModel(team *TeamStatus, task *TeamTask) string {
	if team.WorkerModel != "" {
		return team.WorkerModel
	}
	return ProviderDefaults(taskEffectiveProvider(team, task))
}

func (m *Manager) markTaskRetryOrFailed(name, taskID, message string) {
	m.workersMu.Lock()
	defer m.workersMu.Unlock()
	team, ok := m.teams[name]
	if !ok {
		return
	}
	idx := findTaskIndex(team.Tasks, taskID)
	if idx < 0 {
		return
	}
	task := &team.Tasks[idx]
	task.LastError = message
	task.Summary = ""
	task.UpdatedAt = time.Now()
	endedAt := task.UpdatedAt
	task.EndedAt = &endedAt
	if task.Attempt >= team.MaxTaskRetries {
		task.Status = TeamTaskFailed
	} else {
		task.Status = TeamTaskNeedsRetry
	}
	recalculateTeamLocked(team)
	go m.releaseTeamTaskClaims(name, taskID)
}

func (m *Manager) applyWorkerResult(name string, result TeamWorkerResult, handle TeamWorkerHandle) {
	m.workersMu.Lock()
	defer m.workersMu.Unlock()
	team, ok := m.teams[name]
	if !ok {
		return
	}
	idx := findTaskIndex(team.Tasks, result.TaskID)
	if idx < 0 {
		return
	}
	task := &team.Tasks[idx]
	now := time.Now()
	task.WorkerSessionID = firstNonBlank(handle.SessionID, task.WorkerSessionID)
	task.WorkItemID = firstNonBlank(handle.WorkItemID, task.WorkItemID)
	task.WorkerNodeID = firstNonBlank(handle.WorkerNodeID, task.WorkerNodeID)
	task.WorktreePath = firstNonBlank(handle.WorktreePath, task.WorktreePath)
	task.WorktreeBranch = firstNonBlank(handle.WorktreeBranch, task.WorktreeBranch)
	task.HeadSHA = firstNonBlank(handle.HeadSHA, task.HeadSHA)
	task.MergeBaseSHA = firstNonBlank(handle.MergeBaseSHA, task.MergeBaseSHA)
	task.ArtifactType = firstNonBlank(handle.ArtifactType, task.ArtifactType)
	task.ArtifactPath = firstNonBlank(handle.ArtifactPath, task.ArtifactPath)
	task.ArtifactHash = firstNonBlank(handle.ArtifactHash, task.ArtifactHash)
	if handle.ArtifactSizeBytes > 0 {
		task.ArtifactSizeBytes = handle.ArtifactSizeBytes
	}
	task.ArtifactBaseRef = firstNonBlank(handle.ArtifactBaseRef, task.ArtifactBaseRef)
	task.ArtifactTipRef = firstNonBlank(handle.ArtifactTipRef, task.ArtifactTipRef)
	task.ArtifactStatus = firstNonBlank(handle.ArtifactStatus, task.ArtifactStatus)
	task.Summary = strings.TrimSpace(result.Summary)
	task.LastError = strings.TrimSpace(result.Error)
	task.BlockedQuestion = strings.TrimSpace(result.Question)
	task.ChangedFiles = append([]string(nil), result.ChangedFiles...)
	task.OwnershipDrift = ""
	task.UpdatedAt = now
	task.EndedAt = &now

	if drift := detectOwnedPathDrift(task.OwnedPaths, task.ChangedFiles); drift != "" {
		task.Status = TeamTaskBlocked
		task.MergeStatus = TeamMergeStatusUnavailable
		task.OwnershipDrift = drift
		task.BlockedQuestion = drift
		if team.PendingQuestion == nil {
			team.PendingQuestion = &TeamQuestion{
				ID:       fmt.Sprintf("q-%d", team.StepCount+1),
				TaskID:   task.ID,
				Question: drift,
				AskedAt:  now,
			}
		}
		recalculateTeamLocked(team)
		go m.releaseTeamTaskClaims(name, task.ID)
		return
	}

	switch result.Status {
	case TeamTaskCompleted:
		task.Status = TeamTaskCompleted
		task.LastError = ""
		task.BlockedQuestion = ""
		task.MergeStatus = TeamMergeStatusPending
	case TeamTaskBlocked:
		task.Status = TeamTaskBlocked
		if task.BlockedQuestion != "" && team.PendingQuestion == nil {
			team.PendingQuestion = &TeamQuestion{
				ID:       fmt.Sprintf("q-%d", team.StepCount+1),
				TaskID:   task.ID,
				Question: task.BlockedQuestion,
				AskedAt:  now,
			}
		}
	case TeamTaskNeedsRetry:
		if task.Attempt >= team.MaxTaskRetries {
			task.Status = TeamTaskFailed
		} else {
			task.Status = TeamTaskNeedsRetry
		}
		task.MergeStatus = TeamMergeStatusPending
	case TeamTaskFailed:
		task.Status = TeamTaskFailed
		task.MergeStatus = TeamMergeStatusPending
	}
	recalculateTeamLocked(team)
	go m.releaseTeamTaskClaims(name, task.ID)
}

func (m *Manager) ingestStructuredTeamWorkers(name string) ([]string, error) {
	team, ok := m.teamSnapshot(name)
	if !ok {
		return nil, ErrTeamNotFound
	}
	if !isStructuredCodexTeam(team) {
		return nil, nil
	}

	var updates []string
	if teamUsesRemoteArtifactBackend(team) {
		backend := m.StructuredTeamBackend()
		if backend == nil {
			return nil, fmt.Errorf("team %s requested external backend but none is configured", name)
		}
		for _, task := range team.Tasks {
			if task.WorkItemID == "" || task.Status != TeamTaskInProgress {
				continue
			}
			poll, err := backend.Poll(context.Background(), taskHandle(task))
			if err != nil {
				return updates, fmt.Errorf("poll work item %s: %w", task.WorkItemID, err)
			}
			if poll == nil {
				continue
			}
			m.updateTaskHandle(name, task.ID, poll.Handle)
			if !poll.Terminal {
				continue
			}
			if poll.WorkerResult != nil {
				m.applyWorkerResult(name, *poll.WorkerResult, poll.Handle)
				updates = append(updates, fmt.Sprintf("%s -> %s", task.ID, poll.WorkerResult.Status))
				continue
			}
			msg := firstNonBlank(poll.Error, "worker ended without a structured result")
			m.markTaskRetryOrFailed(name, task.ID, msg)
			updates = append(updates, fmt.Sprintf("%s needs retry: %s", task.ID, msg))
		}
		reconcileUpdates, err := m.reconcileStructuredTeamTasks(context.Background(), name)
		if err != nil {
			return updates, err
		}
		updates = append(updates, reconcileUpdates...)
		m.persistTeamOrWarn(name, "ingest worker updates")
		return updates, nil
	}

	for _, task := range team.Tasks {
		if task.WorkerSessionID == "" || task.Status != TeamTaskInProgress {
			continue
		}
		sess, ok := m.Get(task.WorkerSessionID)
		if !ok {
			continue
		}

		sess.Lock()
		status := sess.Status
		lastActivity := sess.LastActivity
		exitReason := firstNonBlank(sess.Error, sess.ExitReason)
		sess.Unlock()

		switch status {
		case StatusLaunching, StatusRunning:
			if !lastActivity.IsZero() && time.Since(lastActivity) > defaultTeamWorkerStallAge {
				_ = m.teamExecutor().Stop(task.WorkerSessionID)
				msg := fmt.Sprintf("worker stalled after %s", defaultTeamWorkerStallAge)
				m.markTaskRetryOrFailed(name, task.ID, msg)
				updates = append(updates, fmt.Sprintf("%s stalled: %s", task.ID, msg))
			}
		case StatusCompleted:
			result, _, err := parseTeamWorkerResult(sess)
			if err != nil || result.TaskID != task.ID {
				msg := "worker output did not match the structured result contract"
				if err != nil {
					msg = err.Error()
				}
				m.markTaskRetryOrFailed(name, task.ID, msg)
				updates = append(updates, fmt.Sprintf("%s needs retry: %s", task.ID, msg))
				continue
			}
			handle := taskHandle(task)
			handle.SessionID = task.WorkerSessionID
			finalized, finalizeErr := m.finalizeStructuredTeamTaskWorkspace(context.Background(), team, task, result)
			if finalizeErr != nil {
				m.markTaskRetryOrFailed(name, task.ID, finalizeErr.Error())
				updates = append(updates, fmt.Sprintf("%s needs retry: %s", task.ID, finalizeErr.Error()))
				continue
			}
			if finalized.WorktreePath != "" {
				handle = finalized
			}
			m.applyWorkerResult(name, result, handle)
			updates = append(updates, fmt.Sprintf("%s -> %s", task.ID, result.Status))
		case StatusErrored, StatusStopped, StatusInterrupted:
			msg := firstNonBlank(exitReason, fmt.Sprintf("worker ended with status %s", status))
			m.markTaskRetryOrFailed(name, task.ID, msg)
			updates = append(updates, fmt.Sprintf("%s needs retry: %s", task.ID, msg))
		}
	}
	reconcileUpdates, err := m.reconcileStructuredTeamTasks(context.Background(), name)
	if err != nil {
		return updates, err
	}
	updates = append(updates, reconcileUpdates...)
	m.persistTeamOrWarn(name, "ingest worker updates")
	return updates, nil
}

func (m *Manager) applyPlannerActions(ctx context.Context, teamName string, planner TeamPlannerResponse) ([]TeamPlannerAction, error) {
	applied := make([]TeamPlannerAction, 0, len(planner.Actions))
	exec := m.teamExecutor()

	for _, action := range planner.Actions {
		switch action.Type {
		case "launch_worker", "retry_worker":
			var (
				team       *TeamStatus
				task       TeamTask
				ok         bool
				repoPath   string
				tenantID   string
				ownedPaths []string
			)

			m.workersMu.Lock()
			team, ok = m.teams[teamName]
			if !ok {
				m.workersMu.Unlock()
				return applied, ErrTeamNotFound
			}
			if team.PendingQuestion != nil {
				m.workersMu.Unlock()
				return applied, fmt.Errorf("team %s is awaiting human input", teamName)
			}
			activeWorkers := 0
			for _, existing := range team.Tasks {
				if existing.Status == TeamTaskInProgress {
					activeWorkers++
				}
			}
			if activeWorkers >= team.MaxConcurrency {
				m.workersMu.Unlock()
				return applied, fmt.Errorf("team %s is already at max concurrency %d", teamName, team.MaxConcurrency)
			}

			idx := findTaskIndex(team.Tasks, action.TaskID)
			if idx < 0 {
				m.workersMu.Unlock()
				return applied, fmt.Errorf("task %s not found", action.TaskID)
			}
			task = team.Tasks[idx]
			if action.Type == "launch_worker" && task.Status != TeamTaskPending {
				m.workersMu.Unlock()
				return applied, fmt.Errorf("task %s is %s, not pending", action.TaskID, task.Status)
			}
			if action.Type == "retry_worker" && task.Status != TeamTaskNeedsRetry && task.Status != TeamTaskFailed {
				m.workersMu.Unlock()
				return applied, fmt.Errorf("task %s is %s, not retryable", action.TaskID, task.Status)
			}
			repoPath = team.RepoPath
			tenantID = team.TenantID
			ownedPaths = normalizeOwnedPaths(action.OwnedPaths)
			m.workersMu.Unlock()

			claimed, conflict, claimErr := m.claimTeamTaskPaths(teamName, tenantID, repoPath, action.TaskID, ownedPaths)
			if claimErr != nil {
				return applied, fmt.Errorf("claim owned_paths for %s: %w", action.TaskID, claimErr)
			}
			if !claimed {
				m.deferTaskForClaimConflict(teamName, action.TaskID, ownedPaths, conflict)
				continue
			}

			m.workersMu.Lock()
			team, ok = m.teams[teamName]
			if !ok {
				m.workersMu.Unlock()
				go m.releaseTeamTaskClaims(teamName, action.TaskID)
				return applied, ErrTeamNotFound
			}
			if team.PendingQuestion != nil {
				m.workersMu.Unlock()
				go m.releaseTeamTaskClaims(teamName, action.TaskID)
				return applied, fmt.Errorf("team %s is awaiting human input", teamName)
			}
			activeWorkers = 0
			for _, existing := range team.Tasks {
				if existing.Status == TeamTaskInProgress {
					activeWorkers++
				}
			}
			if activeWorkers >= team.MaxConcurrency {
				m.workersMu.Unlock()
				go m.releaseTeamTaskClaims(teamName, action.TaskID)
				return applied, fmt.Errorf("team %s is already at max concurrency %d", teamName, team.MaxConcurrency)
			}
			idx = findTaskIndex(team.Tasks, action.TaskID)
			if idx < 0 {
				m.workersMu.Unlock()
				go m.releaseTeamTaskClaims(teamName, action.TaskID)
				return applied, fmt.Errorf("task %s not found", action.TaskID)
			}
			taskPtr := &team.Tasks[idx]
			taskPtr.Attempt++
			now := time.Now()
			taskPtr.StartedAt = &now
			taskPtr.UpdatedAt = now
			taskPtr.EndedAt = nil
			taskPtr.Status = TeamTaskInProgress
			taskPtr.MergeStatus = TeamMergeStatusPending
			taskPtr.ConflictFiles = nil
			taskPtr.BlockedQuestion = ""
			taskPtr.Summary = ""
			taskPtr.LastError = ""
			taskPtr.Provider = firstNonZeroProvider(action.Provider, taskPtr.Provider)
			taskPtr.WorkerSessionID = ""
			taskPtr.WorkItemID = ""
			taskPtr.A2AAgentURL = team.A2AAgentURL
			taskPtr.WorkerNodeID = ""
			taskPtr.WorktreePath = ""
			taskPtr.WorktreeBranch = ""
			taskPtr.HeadSHA = ""
			taskPtr.MergeBaseSHA = ""
			taskPtr.OwnedPaths = ownedPaths
			taskPtr.OwnershipDrift = ""
			taskPtr.ArtifactType = ""
			taskPtr.ArtifactPath = ""
			taskPtr.ArtifactHash = ""
			taskPtr.ArtifactSizeBytes = 0
			taskPtr.ArtifactBaseRef = ""
			taskPtr.ArtifactTipRef = ""
			taskPtr.ArtifactStatus = ""
			team.UpdatedAt = now
			task = *taskPtr
			teamSnapshot := cloneTeamStatus(team)
			m.workersMu.Unlock()

			prompt := strings.TrimSpace(action.Prompt)
			taskRef := &task
			if prompt == "" {
				prompt = buildStructuredTeamWorkerPrompt(teamSnapshot, taskRef)
			}
			provider := firstNonZeroProvider(action.Provider, taskEffectiveProvider(teamSnapshot, taskRef))
			model := strings.TrimSpace(action.Model)
			if model == "" {
				model = taskEffectiveModel(teamSnapshot, taskRef)
			}
			if teamUsesRemoteArtifactBackend(teamSnapshot) {
				backend := m.StructuredTeamBackend()
				if backend == nil {
					m.markTaskRetryOrFailed(teamName, task.ID, "external backend requested but not configured")
					return applied, fmt.Errorf("team %s requested external backend but no backend is configured", teamName)
				}
				handle, err := backend.Submit(ctx, TeamBackendSubmitRequest{
					TeamName:         teamSnapshot.Name,
					TenantID:         teamSnapshot.TenantID,
					TaskID:           task.ID,
					RepoPath:         teamSnapshot.RepoPath,
					RepoName:         filepath.Base(teamSnapshot.RepoPath),
					Provider:         provider,
					Model:            model,
					Prompt:           prompt,
					MaxBudgetUSD:     teamSnapshot.MaxBudgetUSD,
					SessionName:      fmt.Sprintf("team-%s-%s-%d", sanitizeLoopName(teamSnapshot.Name), task.ID, task.Attempt),
					OutputSchema:     teamWorkerOutputSchema(),
					PlannerSessionID: teamSnapshot.PlannerSessionID,
					WorktreePolicy:   teamSnapshot.WorktreePolicy,
					TargetBranch:     teamSnapshot.TargetBranch,
					HumanContext:     task.HumanContext,
					OwnedPaths:       task.OwnedPaths,
					A2AAgentURL:      teamSnapshot.A2AAgentURL,
				})
				if err != nil {
					m.markTaskRetryOrFailed(teamName, task.ID, fmt.Sprintf("submit worker: %v", err))
					return applied, fmt.Errorf("submit worker for %s: %w", task.ID, err)
				}
				m.updateTaskHandle(teamName, task.ID, handle)
			} else {
				launchRepoPath := teamSnapshot.RepoPath
				handle := TeamWorkerHandle{}
				if teamSnapshot.WorktreePolicy == TeamWorktreePolicyPerWorker {
					var prepErr error
					launchRepoPath, handle, prepErr = m.prepareStructuredTeamTaskWorkspace(ctx, teamSnapshot, &task)
					if prepErr != nil {
						m.markTaskRetryOrFailed(teamName, task.ID, fmt.Sprintf("prepare worktree: %v", prepErr))
						return applied, fmt.Errorf("prepare worktree for %s: %w", task.ID, prepErr)
					}
				}
				sess, err := exec.Launch(ctx, LaunchOptions{
					TenantID:     teamSnapshot.TenantID,
					Provider:     provider,
					RepoPath:     launchRepoPath,
					Prompt:       prompt,
					Model:        model,
					OutputSchema: teamWorkerOutputSchema(),
					TeamName:     teamSnapshot.Name,
					SessionName:  fmt.Sprintf("team-%s-%s-%d", sanitizeLoopName(teamSnapshot.Name), task.ID, task.Attempt),
				})
				if err != nil {
					m.markTaskRetryOrFailed(teamName, task.ID, fmt.Sprintf("launch worker: %v", err))
					return applied, fmt.Errorf("launch worker for %s: %w", task.ID, err)
				}

				handle.SessionID = sess.ID
				m.updateTaskHandle(teamName, task.ID, handle)
			}
			applied = append(applied, action)

		case "stop_worker":
			var sessionID string
			m.workersMu.RLock()
			team := m.teams[teamName]
			if team != nil && action.TaskID != "" {
				if idx := findTaskIndex(team.Tasks, action.TaskID); idx >= 0 {
					sessionID = team.Tasks[idx].WorkerSessionID
				}
			}
			m.workersMu.RUnlock()
			if sessionID == "" {
				sessionID = action.WorkerSessionID
			}
			if sessionID == "" {
				return applied, fmt.Errorf("stop_worker action did not resolve a worker session")
			}
			if teamUsesRemoteArtifactBackend(mustTeamSnapshot(m, teamName)) {
				backend := m.StructuredTeamBackend()
				if backend == nil {
					return applied, fmt.Errorf("team %s requested external backend but no backend is configured", teamName)
				}
				if err := backend.Stop(ctx, resolveTaskHandle(m, teamName, action.TaskID, sessionID)); err != nil {
					return applied, fmt.Errorf("stop worker %s: %w", firstNonBlank(sessionID, action.TaskID), err)
				}
			} else {
				if err := exec.Stop(sessionID); err != nil {
					return applied, fmt.Errorf("stop worker %s: %w", sessionID, err)
				}
			}
			if action.TaskID != "" {
				m.markTaskRetryOrFailed(teamName, action.TaskID, "worker stopped by planner")
			}
			applied = append(applied, action)

		case "mark_complete":
			m.workersMu.Lock()
			team, ok := m.teams[teamName]
			if !ok {
				m.workersMu.Unlock()
				return applied, ErrTeamNotFound
			}
			idx := findTaskIndex(team.Tasks, action.TaskID)
			if idx < 0 {
				m.workersMu.Unlock()
				return applied, fmt.Errorf("task %s not found", action.TaskID)
			}
			now := time.Now()
			team.Tasks[idx].Status = TeamTaskCompleted
			team.Tasks[idx].Summary = firstNonBlank(strings.TrimSpace(action.Summary), strings.TrimSpace(action.Reason))
			team.Tasks[idx].BlockedQuestion = ""
			team.Tasks[idx].LastError = ""
			team.Tasks[idx].UpdatedAt = now
			team.Tasks[idx].EndedAt = &now
			recalculateTeamLocked(team)
			m.workersMu.Unlock()
			go m.releaseTeamTaskClaims(teamName, action.TaskID)
			applied = append(applied, action)

		case "mark_blocked":
			m.workersMu.Lock()
			team, ok := m.teams[teamName]
			if !ok {
				m.workersMu.Unlock()
				return applied, ErrTeamNotFound
			}
			idx := findTaskIndex(team.Tasks, action.TaskID)
			if idx < 0 {
				m.workersMu.Unlock()
				return applied, fmt.Errorf("task %s not found", action.TaskID)
			}
			now := time.Now()
			team.Tasks[idx].Status = TeamTaskBlocked
			team.Tasks[idx].BlockedQuestion = firstNonBlank(strings.TrimSpace(action.Question), strings.TrimSpace(action.Reason))
			team.Tasks[idx].Summary = strings.TrimSpace(action.Summary)
			team.Tasks[idx].UpdatedAt = now
			team.Tasks[idx].EndedAt = &now
			if team.Tasks[idx].BlockedQuestion != "" && team.PendingQuestion == nil {
				team.PendingQuestion = &TeamQuestion{
					ID:       fmt.Sprintf("q-%d", team.StepCount+1),
					TaskID:   action.TaskID,
					Question: team.Tasks[idx].BlockedQuestion,
					AskedAt:  now,
				}
			}
			recalculateTeamLocked(team)
			m.workersMu.Unlock()
			go m.releaseTeamTaskClaims(teamName, action.TaskID)
			applied = append(applied, action)

		case "ask_human":
			m.workersMu.Lock()
			team, ok := m.teams[teamName]
			if !ok {
				m.workersMu.Unlock()
				return applied, ErrTeamNotFound
			}
			team.PendingQuestion = &TeamQuestion{
				ID:       fmt.Sprintf("q-%d", team.StepCount+1),
				TaskID:   action.TaskID,
				Question: strings.TrimSpace(action.Question),
				AskedAt:  time.Now(),
			}
			recalculateTeamLocked(team)
			m.workersMu.Unlock()
			applied = append(applied, action)
		}
	}

	m.persistTeamOrWarn(teamName, "apply planner actions")
	return applied, nil
}

func firstNonZeroProvider(values ...Provider) Provider {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (m *Manager) StepTeam(ctx context.Context, name string) (*TeamStepResult, error) {
	return m.StepTeamForTenant(ctx, DefaultTenantID, name)
}

func (m *Manager) StepTeamForTenant(ctx context.Context, tenantID, name string) (*TeamStepResult, error) {
	return m.stepTeamByKey(ctx, m.teamKey(name, tenantID))
}

func (m *Manager) stepTeamByKey(ctx context.Context, name string) (*TeamStepResult, error) {
	m.workersMu.Lock()
	team, ok := m.teams[name]
	if !ok {
		m.workersMu.Unlock()
		return nil, ErrTeamNotFound
	}
	if !isStructuredCodexTeam(team) {
		m.workersMu.Unlock()
		return nil, fmt.Errorf("team %s uses runtime %s; team_step is only supported for structured codex teams", name, team.Runtime)
	}
	now := time.Now()
	team.StepCount++
	team.LastStepAt = now
	team.UpdatedAt = now
	recalculateTeamLocked(team)
	m.workersMu.Unlock()
	m.persistTeamOrWarn(name, "start team step")

	workerUpdates, err := m.ingestStructuredTeamWorkers(name)
	if err != nil {
		return nil, err
	}

	snapshot, ok := m.teamSnapshot(name)
	if !ok {
		return nil, ErrTeamNotFound
	}
	if snapshot.PendingQuestion != nil || snapshot.RunState == TeamRunStateCompleted || snapshot.RunState == TeamRunStateFailed {
		return &TeamStepResult{Team: snapshot, WorkerUpdates: workerUpdates}, nil
	}

	activeWorkers := 0
	runnableTasks := 0
	for _, task := range snapshot.Tasks {
		if task.Status == TeamTaskInProgress {
			activeWorkers++
		}
		if isRunnableTeamTask(task) {
			runnableTasks++
		}
	}
	if runnableTasks == 0 || activeWorkers >= snapshot.MaxConcurrency {
		return &TeamStepResult{Team: snapshot, WorkerUpdates: workerUpdates}, nil
	}

	plannerPrompt := m.buildStructuredTeamPlannerPrompt(snapshot)
	plannerSession, err := m.teamExecutor().Launch(ctx, LaunchOptions{
		TenantID:     snapshot.TenantID,
		Provider:     snapshot.Provider,
		RepoPath:     snapshot.RepoPath,
		Prompt:       plannerPrompt,
		Model:        snapshot.Model,
		OutputSchema: teamPlannerOutputSchema(),
		TeamName:     snapshot.Name,
		SessionName:  fmt.Sprintf("team-plan-%s-%03d", sanitizeLoopName(snapshot.Name), snapshot.StepCount),
	})
	if err != nil {
		m.workersMu.Lock()
		if team, ok := m.teams[name]; ok {
			team.Status = StatusErrored
			team.RunState = TeamRunStateFailed
			team.LastPlannerSummary = fmt.Sprintf("planner launch failed: %v", err)
			team.UpdatedAt = time.Now()
		}
		m.workersMu.Unlock()
		m.persistTeamOrWarn(name, "planner launch failure")
		return nil, fmt.Errorf("launch planner session: %w", err)
	}

	m.workersMu.Lock()
	if team, ok := m.teams[name]; ok {
		team.LeadID = plannerSession.ID
		team.PlannerSessionID = plannerSession.ID
		team.UpdatedAt = time.Now()
	}
	m.workersMu.Unlock()
	m.persistTeamOrWarn(name, "planner launched")

	if err := m.waitForSession(ctx, plannerSession); err != nil {
		m.workersMu.Lock()
		if team, ok := m.teams[name]; ok {
			team.Status = StatusErrored
			team.RunState = TeamRunStateFailed
			team.LastPlannerSummary = fmt.Sprintf("planner session failed: %v", err)
			team.UpdatedAt = time.Now()
		}
		m.workersMu.Unlock()
		m.persistTeamOrWarn(name, "planner wait failure")
		return nil, fmt.Errorf("planner session failed: %w", err)
	}

	plannerResp, plannerOutput, err := parseTeamPlannerResponse(plannerSession)
	if err != nil {
		m.workersMu.Lock()
		if team, ok := m.teams[name]; ok {
			team.Status = StatusErrored
			team.RunState = TeamRunStateFailed
			team.LastPlannerSummary = err.Error()
			team.UpdatedAt = time.Now()
		}
		m.workersMu.Unlock()
		m.persistTeamOrWarn(name, "planner parse failure")
		return nil, fmt.Errorf("parse planner output: %w", err)
	}

	m.workersMu.Lock()
	if team, ok := m.teams[name]; ok {
		team.LastPlannerSummary = strings.TrimSpace(plannerResp.Summary)
		team.UpdatedAt = time.Now()
	}
	m.workersMu.Unlock()

	applied, err := m.applyPlannerActions(ctx, name, plannerResp)
	if err != nil {
		m.workersMu.Lock()
		if team, ok := m.teams[name]; ok {
			team.Status = StatusErrored
			team.RunState = TeamRunStateFailed
			team.LastPlannerSummary = fmt.Sprintf("planner action failed: %v", err)
			team.UpdatedAt = time.Now()
		}
		m.workersMu.Unlock()
		m.persistTeamOrWarn(name, "planner action failure")
		return nil, err
	}

	snapshot, ok = m.teamSnapshot(name)
	if !ok {
		return nil, ErrTeamNotFound
	}
	return &TeamStepResult{
		Team:          snapshot,
		Actions:       applied,
		PlannerOutput: plannerOutput,
		WorkerUpdates: workerUpdates,
	}, nil
}

func (m *Manager) AnswerTeam(name, answer, taskID string) (*TeamStatus, error) {
	return m.AnswerTeamForTenant(DefaultTenantID, name, answer, taskID)
}

func (m *Manager) AnswerTeamForTenant(tenantID, name, answer, taskID string) (*TeamStatus, error) {
	return m.answerTeamByKey(m.teamKey(name, tenantID), answer, taskID)
}

func (m *Manager) answerTeamByKey(name, answer, taskID string) (*TeamStatus, error) {
	m.workersMu.Lock()
	team, ok := m.teams[name]
	if !ok {
		m.workersMu.Unlock()
		return nil, ErrTeamNotFound
	}
	if !isStructuredCodexTeam(team) {
		m.workersMu.Unlock()
		return nil, fmt.Errorf("team %s uses runtime %s; team_answer is only supported for structured codex teams", name, team.Runtime)
	}
	if team.PendingQuestion == nil {
		m.workersMu.Unlock()
		return nil, fmt.Errorf("team %s is not awaiting human input", name)
	}
	if taskID != "" && team.PendingQuestion.TaskID != "" && team.PendingQuestion.TaskID != taskID {
		m.workersMu.Unlock()
		return nil, fmt.Errorf("pending question belongs to task %s, not %s", team.PendingQuestion.TaskID, taskID)
	}

	now := time.Now()
	question := *team.PendingQuestion
	question.Answer = strings.TrimSpace(answer)
	question.AnsweredAt = &now
	team.ResolvedQuestions = append(team.ResolvedQuestions, question)
	if question.TaskID != "" {
		if idx := findTaskIndex(team.Tasks, question.TaskID); idx >= 0 {
			team.Tasks[idx].HumanContext = append(team.Tasks[idx].HumanContext, question.Answer)
			team.Tasks[idx].BlockedQuestion = ""
			if team.Tasks[idx].Status == TeamTaskBlocked {
				team.Tasks[idx].Status = TeamTaskPending
				team.Tasks[idx].EndedAt = nil
			}
			team.Tasks[idx].UpdatedAt = now
		}
	}
	team.PendingQuestion = nil
	team.UpdatedAt = now
	recalculateTeamLocked(team)
	snapshot := cloneTeamStatus(team)
	m.workersMu.Unlock()

	if err := m.writeTeamSnapshot(snapshot); err != nil {
		slog.Warn("failed to persist team answer", "team", name, "error", err)
	}
	if snapshot.AutoStart {
		if started, err := m.startTeamByKey(context.Background(), name); err == nil {
			return started, nil
		}
	}
	return snapshot, nil
}

func mustTeamSnapshot(m *Manager, name string) *TeamStatus {
	team, _ := m.teamSnapshot(name)
	return team
}

func resolveTaskHandle(m *Manager, teamName, taskID, sessionID string) TeamWorkerHandle {
	team, ok := m.teamSnapshot(teamName)
	if !ok {
		return TeamWorkerHandle{SessionID: sessionID}
	}
	if taskID != "" {
		for _, task := range team.Tasks {
			if task.ID == taskID {
				handle := taskHandle(task)
				if handle.SessionID == "" {
					handle.SessionID = sessionID
				}
				return handle
			}
		}
	}
	return TeamWorkerHandle{SessionID: sessionID}
}

func (m *Manager) updateTaskHandle(name, taskID string, handle TeamWorkerHandle) {
	m.workersMu.Lock()
	defer m.workersMu.Unlock()
	team, ok := m.teams[name]
	if !ok {
		return
	}
	idx := findTaskIndex(team.Tasks, taskID)
	if idx < 0 {
		return
	}
	task := &team.Tasks[idx]
	task.WorkItemID = firstNonBlank(handle.WorkItemID, task.WorkItemID)
	task.A2AAgentURL = firstNonBlank(handle.A2AAgentURL, task.A2AAgentURL)
	task.WorkerSessionID = firstNonBlank(handle.SessionID, task.WorkerSessionID)
	task.WorkerNodeID = firstNonBlank(handle.WorkerNodeID, task.WorkerNodeID)
	task.WorktreePath = firstNonBlank(handle.WorktreePath, task.WorktreePath)
	task.WorktreeBranch = firstNonBlank(handle.WorktreeBranch, task.WorktreeBranch)
	task.HeadSHA = firstNonBlank(handle.HeadSHA, task.HeadSHA)
	task.MergeBaseSHA = firstNonBlank(handle.MergeBaseSHA, task.MergeBaseSHA)
	task.ArtifactType = firstNonBlank(handle.ArtifactType, task.ArtifactType)
	task.ArtifactPath = firstNonBlank(handle.ArtifactPath, task.ArtifactPath)
	task.ArtifactHash = firstNonBlank(handle.ArtifactHash, task.ArtifactHash)
	if handle.ArtifactSizeBytes > 0 {
		task.ArtifactSizeBytes = handle.ArtifactSizeBytes
	}
	task.ArtifactBaseRef = firstNonBlank(handle.ArtifactBaseRef, task.ArtifactBaseRef)
	task.ArtifactTipRef = firstNonBlank(handle.ArtifactTipRef, task.ArtifactTipRef)
	task.ArtifactStatus = firstNonBlank(handle.ArtifactStatus, task.ArtifactStatus)
	task.UpdatedAt = time.Now()
}

func (m *Manager) prepareStructuredTeamTaskWorkspace(ctx context.Context, team *TeamStatus, task *TeamTask) (string, TeamWorkerHandle, error) {
	if team == nil || task == nil || team.WorktreePolicy != TeamWorktreePolicyPerWorker {
		return team.RepoPath, TeamWorkerHandle{}, nil
	}
	wtPath := structuredTeamTaskWorktreePath(team, task)
	branch := structuredTeamTaskBranch(team, task)
	if err := os.MkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		return "", TeamWorkerHandle{}, fmt.Errorf("mkdir worktree parent: %w", err)
	}
	if _, err := os.Stat(wtPath); err != nil {
		if err := worktree.Create(ctx, team.RepoPath, wtPath, branch); err != nil {
			if _, createErr := worktree.CreateWorktree(ctx, team.RepoPath, branch, worktree.WithBaseBranch(team.TargetBranch), worktree.WithPath(wtPath)); createErr != nil {
				return "", TeamWorkerHandle{}, createErr
			}
		}
	}
	headSHA, _ := teamGitOutput(ctx, wtPath, "rev-parse", "HEAD")
	mergeBase, _ := teamGitOutput(ctx, team.RepoPath, "merge-base", team.TargetBranch, branch)
	return wtPath, TeamWorkerHandle{
		WorktreePath:   wtPath,
		WorktreeBranch: branch,
		HeadSHA:        strings.TrimSpace(headSHA),
		MergeBaseSHA:   strings.TrimSpace(mergeBase),
	}, nil
}

func (m *Manager) finalizeStructuredTeamTaskWorkspace(ctx context.Context, team *TeamStatus, task TeamTask, result TeamWorkerResult) (TeamWorkerHandle, error) {
	handle := taskHandle(task)
	if task.WorktreePath == "" {
		return handle, nil
	}
	if err := teamGitRun(ctx, task.WorktreePath, "add", "-A"); err != nil {
		return handle, fmt.Errorf("stage worktree changes: %w", err)
	}
	if !teamGitHasStagedChanges(ctx, task.WorktreePath) {
		headSHA, _ := teamGitOutput(ctx, task.WorktreePath, "rev-parse", "HEAD")
		mergeBase, _ := teamGitOutput(ctx, team.RepoPath, "merge-base", team.TargetBranch, task.WorktreeBranch)
		handle.HeadSHA = strings.TrimSpace(headSHA)
		handle.MergeBaseSHA = strings.TrimSpace(mergeBase)
		return handle, nil
	}
	if err := teamGitRun(ctx, task.WorktreePath, "commit", "-m", fmt.Sprintf("ralphglasses: %s %s", team.Name, task.ID)); err != nil {
		return handle, fmt.Errorf("commit worktree changes: %w", err)
	}
	headSHA, _ := teamGitOutput(ctx, task.WorktreePath, "rev-parse", "HEAD")
	mergeBase, _ := teamGitOutput(ctx, team.RepoPath, "merge-base", team.TargetBranch, task.WorktreeBranch)
	handle.HeadSHA = strings.TrimSpace(headSHA)
	handle.MergeBaseSHA = strings.TrimSpace(mergeBase)
	if len(result.ChangedFiles) == 0 {
		changed, _ := teamGitChangedFiles(ctx, task.WorktreePath, strings.TrimSpace(mergeBase), strings.TrimSpace(headSHA))
		result.ChangedFiles = changed
	}
	return handle, nil
}

func (m *Manager) ensureStructuredTeamIntegrationWorkspace(ctx context.Context, team *TeamStatus) (string, error) {
	if team == nil {
		return "", fmt.Errorf("nil team")
	}
	path := team.IntegrationPath
	if path == "" {
		path = structuredTeamIntegrationPath(team)
	}
	if path == "" {
		return "", fmt.Errorf("integration path unavailable")
	}
	if _, err := os.Stat(path); err == nil {
		m.setTeamIntegrationPath(team.TenantID, team.Name, path)
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("mkdir integration parent: %w", err)
	}
	if _, err := worktree.CreateWorktree(ctx, team.RepoPath, team.IntegrationBranch, worktree.WithBaseBranch(team.TargetBranch), worktree.WithPath(path)); err != nil {
		if createRefErr := worktree.CreateFromRef(ctx, team.RepoPath, path, team.IntegrationBranch); createRefErr != nil {
			return "", err
		}
	}
	m.setTeamIntegrationPath(team.TenantID, team.Name, path)
	return path, nil
}

func (m *Manager) setTeamIntegrationPath(tenantID, name, path string) {
	m.workersMu.Lock()
	defer m.workersMu.Unlock()
	if team, ok := m.teams[m.teamKey(name, tenantID)]; ok {
		team.IntegrationPath = path
		team.UpdatedAt = time.Now()
	}
}

func (m *Manager) reconcileStructuredTeamTasks(ctx context.Context, name string) ([]string, error) {
	team, ok := m.teamSnapshot(name)
	if !ok || !isStructuredCodexTeam(team) {
		return nil, nil
	}
	var updates []string
	for _, task := range team.Tasks {
		if task.Status != TeamTaskCompleted || task.MergeStatus != TeamMergeStatusPending {
			continue
		}
		if task.WorktreePath == "" {
			m.markTaskMergeUnavailable(name, task.ID, "no worker worktree available for reconcile")
			updates = append(updates, fmt.Sprintf("%s merge unavailable: no worktree", task.ID))
			continue
		}
		if _, err := os.Stat(task.WorktreePath); err != nil {
			m.markTaskMergeUnavailable(name, task.ID, "worker worktree is not locally accessible for reconcile")
			updates = append(updates, fmt.Sprintf("%s merge unavailable: worktree not accessible", task.ID))
			continue
		}
		integrationPath, err := m.ensureStructuredTeamIntegrationWorkspace(ctx, team)
		if err != nil {
			return updates, err
		}
		wt := &worktree.Worktree{
			Path:     task.WorktreePath,
			RepoPath: integrationPath,
		}
		dryRun, err := worktree.MergeBackFn(ctx, wt, team.IntegrationBranch, worktree.WithDryRun())
		if err != nil {
			return updates, err
		}
		if !dryRun.Success {
			m.markTaskMergeConflict(name, task.ID, dryRun.ConflictFiles)
			updates = append(updates, fmt.Sprintf("%s merge conflict", task.ID))
			continue
		}
		merged, err := worktree.MergeBackFn(ctx, wt, team.IntegrationBranch, worktree.WithAbortOnConflict(), worktree.WithMessage(fmt.Sprintf("ralphglasses: merge %s", task.ID)))
		if err != nil {
			return updates, err
		}
		m.markTaskMerged(name, task.ID, merged.MergedFiles)
		updates = append(updates, fmt.Sprintf("%s merged into %s", task.ID, team.IntegrationBranch))
	}
	return updates, nil
}

func (m *Manager) markTaskMerged(name, taskID string, mergedFiles []string) {
	m.workersMu.Lock()
	defer m.workersMu.Unlock()
	team, ok := m.teams[name]
	if !ok {
		return
	}
	idx := findTaskIndex(team.Tasks, taskID)
	if idx < 0 {
		return
	}
	task := &team.Tasks[idx]
	task.MergeStatus = TeamMergeStatusMerged
	if len(mergedFiles) > 0 {
		task.ChangedFiles = cloneStringSlice(mergedFiles)
	}
	task.ConflictFiles = nil
	task.UpdatedAt = time.Now()
	recalculateTeamLocked(team)
}

func (m *Manager) markTaskMergeConflict(name, taskID string, conflictFiles []string) {
	m.workersMu.Lock()
	defer m.workersMu.Unlock()
	team, ok := m.teams[name]
	if !ok {
		return
	}
	idx := findTaskIndex(team.Tasks, taskID)
	if idx < 0 {
		return
	}
	now := time.Now()
	task := &team.Tasks[idx]
	task.MergeStatus = TeamMergeStatusConflict
	task.Status = TeamTaskBlocked
	task.ConflictFiles = cloneStringSlice(conflictFiles)
	task.BlockedQuestion = fmt.Sprintf("Resolve merge conflicts for %s", taskID)
	task.UpdatedAt = now
	task.EndedAt = &now
	if team.PendingQuestion == nil {
		team.PendingQuestion = &TeamQuestion{
			ID:       fmt.Sprintf("q-%d", team.StepCount+1),
			TaskID:   taskID,
			Question: task.BlockedQuestion,
			AskedAt:  now,
		}
	}
	recalculateTeamLocked(team)
}

func (m *Manager) markTaskMergeUnavailable(name, taskID, reason string) {
	m.workersMu.Lock()
	defer m.workersMu.Unlock()
	team, ok := m.teams[name]
	if !ok {
		return
	}
	idx := findTaskIndex(team.Tasks, taskID)
	if idx < 0 {
		return
	}
	now := time.Now()
	task := &team.Tasks[idx]
	task.MergeStatus = TeamMergeStatusUnavailable
	task.Status = TeamTaskBlocked
	task.LastError = reason
	task.BlockedQuestion = reason
	task.UpdatedAt = now
	task.EndedAt = &now
	if team.PendingQuestion == nil {
		team.PendingQuestion = &TeamQuestion{
			ID:       fmt.Sprintf("q-%d", team.StepCount+1),
			TaskID:   taskID,
			Question: reason,
			AskedAt:  now,
		}
	}
	recalculateTeamLocked(team)
}

func (m *Manager) teamPathClaimsFile() string {
	if m.stateDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(m.stateDir), "team-path-claims.json")
}

func (m *Manager) teamPathClaimCoordinator() (*Coordinator, error) {
	path := m.teamPathClaimsFile()
	if path == "" {
		return NewCoordinator(), nil
	}
	return NewCoordinatorWithPersistence(path)
}

func normalizeOwnedPaths(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	var normalized []string
	for _, value := range values {
		value = filepath.ToSlash(filepath.Clean(strings.TrimSpace(value)))
		value = strings.TrimPrefix(value, "./")
		value = strings.Trim(value, "/")
		if value == "" || value == "." {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func teamPathClaimKey(tenantID, repoPath, ownedPath string) string {
	return filepath.ToSlash(filepath.Join("tenant", sanitizeTenantPathSegment(tenantID), "repo", filepath.Base(repoPath), "path", ownedPath))
}

func teamPathClaimOwner(teamName, taskID string) string {
	return teamName + ":" + taskID
}

func teamPathClaimOverlap(existing, requested string) bool {
	existing = filepath.ToSlash(filepath.Clean(existing))
	requested = filepath.ToSlash(filepath.Clean(requested))
	return existing == requested ||
		strings.HasPrefix(existing, requested+"/") ||
		strings.HasPrefix(requested, existing+"/")
}

func (m *Manager) teamPathClaimsForRepo(tenantID, repoPath string) []string {
	coord, err := m.teamPathClaimCoordinator()
	if err != nil {
		return nil
	}
	prefix := filepath.ToSlash(filepath.Join("tenant", sanitizeTenantPathSegment(tenantID), "repo", filepath.Base(repoPath), "path")) + "/"
	all := coord.AllResources()
	var claims []string
	for resource, owner := range all {
		if !strings.HasPrefix(resource, prefix) {
			continue
		}
		claims = append(claims, strings.TrimPrefix(resource, prefix)+" owner="+owner)
	}
	return claims
}

func (m *Manager) claimTeamTaskPaths(teamName, tenantID, repoPath, taskID string, ownedPaths []string) (bool, string, error) {
	coord, err := m.teamPathClaimCoordinator()
	if err != nil {
		return false, "", err
	}
	keys := make([]string, 0, len(ownedPaths))
	for _, owned := range ownedPaths {
		keys = append(keys, teamPathClaimKey(tenantID, repoPath, owned))
	}
	return coord.ClaimResources(teamPathClaimOwner(teamName, taskID), keys, teamPathClaimOverlap)
}

func (m *Manager) releaseTeamTaskClaims(teamName, taskID string) {
	coord, err := m.teamPathClaimCoordinator()
	if err != nil {
		return
	}
	coord.ReleaseAll(teamPathClaimOwner(teamName, taskID))
}

func (m *Manager) deferTaskForClaimConflict(teamName, taskID string, ownedPaths []string, conflict string) {
	m.workersMu.Lock()
	defer m.workersMu.Unlock()
	team, ok := m.teams[teamName]
	if !ok {
		return
	}
	idx := findTaskIndex(team.Tasks, taskID)
	if idx < 0 {
		return
	}
	task := &team.Tasks[idx]
	task.Status = TeamTaskPending
	task.OwnedPaths = cloneStringSlice(ownedPaths)
	task.LastError = fmt.Sprintf("owned_paths conflict with active claim %s", conflict)
	task.UpdatedAt = time.Now()
	recalculateTeamLocked(team)
}

func detectOwnedPathDrift(ownedPaths, changedFiles []string) string {
	if len(ownedPaths) == 0 || len(changedFiles) == 0 {
		return ""
	}
	normalizedOwned := normalizeOwnedPaths(ownedPaths)
	var drift []string
	for _, changed := range normalizeOwnedPaths(changedFiles) {
		ok := false
		for _, owned := range normalizedOwned {
			if teamPathClaimOverlap(owned, changed) {
				ok = true
				break
			}
		}
		if !ok {
			drift = append(drift, changed)
		}
	}
	if len(drift) == 0 {
		return ""
	}
	return "worker changed files outside owned_paths: " + strings.Join(drift, ", ")
}

func teamGitRun(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func teamGitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func teamGitHasStagedChanges(ctx context.Context, dir string) bool {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "diff", "--cached", "--quiet")
	err := cmd.Run()
	if err == nil {
		return false
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return true
	}
	return false
}

func teamGitChangedFiles(ctx context.Context, dir, baseRef, headRef string) ([]string, error) {
	if baseRef == "" {
		out, err := teamGitOutput(ctx, dir, "diff", "--cached", "--name-only")
		if err != nil {
			return nil, err
		}
		return teamCompactNonEmpty(strings.Split(out, "\n")), nil
	}
	out, err := teamGitOutput(ctx, dir, "diff", "--name-only", baseRef, headRef)
	if err != nil {
		return nil, err
	}
	return teamCompactNonEmpty(strings.Split(out, "\n")), nil
}

func teamCompactNonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}
