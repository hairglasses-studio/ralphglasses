package fleet

import (
	"context"
	"log/slog"
	"net"
	"path/filepath"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

// WorkerAgent runs on each worker node, handling registration, heartbeat, and work execution.
type WorkerAgent struct {
	nodeID   string
	hostname string
	port     int
	version  string
	scanPath string

	client    *Client
	sessMgr   *session.Manager
	bus       *events.Bus
	startedAt time.Time

	// eventCursor tracks the last forwarded event for batching
	eventCursor int
}

// NewWorkerAgent creates a worker agent that connects to a coordinator.
func NewWorkerAgent(coordinatorURL string, hostname string, port int, version string, scanPath string, bus *events.Bus, sessMgr *session.Manager) *WorkerAgent {
	return &WorkerAgent{
		hostname:  hostname,
		port:      port,
		version:   version,
		scanPath:  scanPath,
		client:    NewClient(coordinatorURL),
		sessMgr:   sessMgr,
		bus:       bus,
		startedAt: time.Now(),
	}
}

// Run starts the worker's registration, heartbeat, and poll loops.
// Blocks until ctx is cancelled.
func (w *WorkerAgent) Run(ctx context.Context) error {
	// Discover local repos and providers
	repos := w.discoverRepos(ctx)
	providers := w.discoverProviders()

	tsIP := DiscoverTailscaleIP()

	// Register with coordinator
	workerID, err := w.client.Register(ctx, RegisterPayload{
		Hostname:    w.hostname,
		TailscaleIP: tsIP,
		Port:        w.port,
		Providers:   providers,
		Repos:       repos,
		MaxSessions: 4,
		Version:     w.version,
	})
	if err != nil {
		return err
	}
	w.nodeID = workerID
	util.Debug.Debugf("registered as worker %s", workerID)

	// Run heartbeat, poll, and event forwarding concurrently
	go w.heartbeatLoop(ctx, repos, providers)
	go w.pollLoop(ctx)
	go w.eventForwardLoop(ctx)

	<-ctx.Done()
	return ctx.Err()
}

func (w *WorkerAgent) heartbeatLoop(ctx context.Context, repos []string, providers []session.Provider) {
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sessions := w.sessMgr.List("")
			active := 0
			var spent float64
			for _, s := range sessions {
				s.Lock()
				if s.Status == session.StatusRunning || s.Status == session.StatusLaunching {
					active++
				}
				spent += s.SpentUSD
				s.Unlock()
			}

			if err := w.client.Heartbeat(ctx, HeartbeatPayload{
				WorkerID:       w.nodeID,
				ActiveSessions: active,
				SpentUSD:       spent,
				AvailableSlots: 4 - active,
				Repos:          repos,
				Providers:      providers,
				Load:           float64(active) / 4.0,
			}); err != nil {
				slog.Error("fleet worker: heartbeat failed", "worker", w.nodeID, "err", err)
			}
		}
	}
}

func (w *WorkerAgent) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			item, err := w.client.PollWork(ctx, w.nodeID)
			if err != nil {
				util.Debug.Debugf("poll error: %v", err)
				continue
			}
			if item == nil {
				continue
			}
			go w.executeWork(ctx, item)
		}
	}
}

func (w *WorkerAgent) executeWork(ctx context.Context, item *WorkItem) {
	util.Debug.Debugf("executing work %s: %s", item.ID, item.RepoName)

	opts := session.LaunchOptions{
<<<<<<< Updated upstream
		Provider:     item.Provider,
		RepoPath:     item.RepoPath,
		Prompt:       item.Prompt,
		Model:        item.Model,
		Agent:        item.Agent,
		MaxBudgetUSD: item.MaxBudgetUSD,
		MaxTurns:     item.MaxTurns,
||||||| Stash base
		Provider:     item.Provider,
		RepoPath:     worktreePath,
		Prompt:       item.Prompt,
		Model:        item.Model,
		Agent:        item.Agent,
		MaxBudgetUSD: item.MaxBudgetUSD,
		MaxTurns:     item.MaxTurns,
		SessionName:  item.SessionName,
		TeamName:     item.TeamName,
		OutputSchema: item.OutputSchema,
		PermissionMode: item.PermissionMode,
		Sandbox:      item.Sandbox,
		SandboxImage: item.SandboxImage,
=======
		Provider:       item.Provider,
		RepoPath:       worktreePath,
		Prompt:         item.Prompt,
		Model:          item.Model,
		Agent:          item.Agent,
		MaxBudgetUSD:   item.MaxBudgetUSD,
		MaxTurns:       item.MaxTurns,
		SessionName:    item.SessionName,
		TeamName:       item.TeamName,
		OutputSchema:   item.OutputSchema,
		PermissionMode: item.PermissionMode,
		Sandbox:        item.Sandbox,
		SandboxImage:   item.SandboxImage,
>>>>>>> Stashed changes
	}

	if opts.Provider == "" {
		opts.Provider = session.DefaultPrimaryProvider()
	}

	sess, err := w.sessMgr.Launch(ctx, opts)
	if err != nil {
		if cErr := w.client.CompleteWork(ctx, WorkCompletePayload{
			WorkItemID: item.ID,
			Status:     WorkFailed,
			Error:      err.Error(),
		}); cErr != nil {
			slog.Error("fleet worker: complete-work (launch fail) failed", "item", item.ID, "err", cErr)
		}
		return
	}

	// Wait for session to complete
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sess.Lock()
			status := sess.Status
			spent := sess.SpentUSD
			turns := sess.TurnCount
			exitReason := sess.ExitReason
			lastOutput := sess.LastOutput
			launched := sess.LaunchedAt
			sess.Unlock()

			switch status {
			case session.StatusCompleted:
				if cErr := w.client.CompleteWork(ctx, WorkCompletePayload{
					WorkItemID: item.ID,
					Status:     WorkCompleted,
					Result: &WorkResult{
						SessionID:  sess.ID,
						SpentUSD:   spent,
						TurnCount:  turns,
						DurationS:  time.Since(launched).Seconds(),
						ExitReason: exitReason,
						Output:     lastOutput,
					},
				}); cErr != nil {
					slog.Error("fleet worker: complete-work (success) failed", "item", item.ID, "err", cErr)
				}
				return
			case session.StatusErrored, session.StatusStopped:
				if cErr := w.client.CompleteWork(ctx, WorkCompletePayload{
					WorkItemID: item.ID,
					Status:     WorkFailed,
					Error:      exitReason,
					Result: &WorkResult{
						SessionID:  sess.ID,
						SpentUSD:   spent,
						TurnCount:  turns,
						DurationS:  time.Since(launched).Seconds(),
						ExitReason: exitReason,
					},
				}); cErr != nil {
					slog.Error("fleet worker: complete-work (error) failed", "item", item.ID, "err", cErr)
				}
				return
			}
		}
	}
}

func (w *WorkerAgent) eventForwardLoop(ctx context.Context) {
	if w.bus == nil {
		return
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			evts, cursor := w.bus.HistoryAfterCursor(w.eventCursor, 100)
			w.eventCursor = cursor

			if len(evts) == 0 {
				continue
			}

			batch := EventBatch{
				WorkerID: w.nodeID,
				Events:   make([]FleetEvent, len(evts)),
			}
			for i, e := range evts {
				batch.Events[i] = FleetEvent{
					NodeID:    w.nodeID,
					Type:      string(e.Type),
					Timestamp: e.Timestamp,
					RepoName:  e.RepoName,
					SessionID: e.SessionID,
					Provider:  e.Provider,
					Data:      e.Data,
				}
			}
			if err := w.client.SendEvents(ctx, batch); err != nil {
				slog.Error("fleet worker: send events failed", "worker", w.nodeID, "events", len(batch.Events), "err", err)
			}
		}
	}
}

func (w *WorkerAgent) discoverRepos(ctx context.Context) []string {
	if w.scanPath == "" {
		return nil
	}
	repos, err := discovery.Scan(ctx, w.scanPath)
	if err != nil {
		return nil
	}
	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = filepath.Base(r.Path)
	}
	return names
}

<<<<<<< Updated upstream
||||||| Stash base
func (w *WorkerAgent) resolveRepoPath(ctx context.Context, repoName string) (string, error) {
	if repoName == "" {
		return "", fmt.Errorf("repo name required")
	}
	if w.scanPath == "" {
		return "", fmt.Errorf("scan path not configured for worker")
	}
	repos, err := discovery.Scan(ctx, w.scanPath)
	if err != nil {
		return "", err
	}
	for _, repo := range repos {
		if filepath.Base(repo.Path) == repoName {
			return repo.Path, nil
		}
	}
	return "", fmt.Errorf("repo %s not found on worker", repoName)
}

func (w *WorkerAgent) prepareWorktree(ctx context.Context, repoPath string, item *WorkItem) (string, string, error) {
	branch := sanitizeWorktreeLabel(fmt.Sprintf("ralph-team-%s-%s-%s", item.TeamName, item.TeamTaskID, item.ID))
	worktreePath := filepath.Join(filepath.Dir(repoPath), ".ralph-worktrees", filepath.Base(repoPath), sanitizeWorktreeLabel(item.TeamName), sanitizeWorktreeLabel(item.TeamTaskID), item.ID)
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return "", "", err
	}
	if _, err := os.Stat(worktreePath); err == nil {
		return worktreePath, branch, nil
	}
	if _, err := worktree.CreateWorktree(ctx, repoPath, branch, worktree.WithBaseBranch(item.TargetBranch), worktree.WithPath(worktreePath)); err != nil {
		if createRefErr := worktree.CreateFromRef(ctx, repoPath, worktreePath, branch); createRefErr != nil {
			return "", "", err
		}
	}
	return worktreePath, branch, nil
}

func populateStructuredResult(ctx context.Context, result *WorkResult, item *WorkItem, workerNodeID, repoPath, worktreePath, worktreeBranch, output string) {
	result.WorkerNodeID = workerNodeID
	result.WorktreePath = emptyIfSame(worktreePath, repoPath)
	result.WorktreeBranch = worktreeBranch

	workerResult, err := parseStructuredWorkerResult(output)
	if err != nil {
		result.TaskStatus = session.TeamTaskNeedsRetry
		result.Summary = "worker output did not match structured contract"
		return
	}

	result.TaskStatus = workerResult.Status
	result.Summary = workerResult.Summary
	result.Question = workerResult.Question
	result.ChangedFiles = append([]string(nil), workerResult.ChangedFiles...)

	if worktreePath != "" && worktreePath != repoPath {
		if err := finalizeWorktree(ctx, worktreePath, repoPath, item, result); err != nil {
			result.TaskStatus = session.TeamTaskNeedsRetry
			if result.Summary == "" {
				result.Summary = err.Error()
			}
		}
	}
}

func finalizeWorktree(ctx context.Context, worktreePath, repoPath string, item *WorkItem, result *WorkResult) error {
	if err := gitRun(ctx, worktreePath, "add", "-A"); err != nil {
		return err
	}
	if gitHasStagedChanges(ctx, worktreePath) {
		if err := gitRun(ctx, worktreePath, "commit", "-m", fmt.Sprintf("ralphglasses: %s %s", item.TeamName, item.TeamTaskID)); err != nil {
			return err
		}
	}
	headSHA, _ := gitOutput(ctx, worktreePath, "rev-parse", "HEAD")
	result.HeadSHA = strings.TrimSpace(headSHA)
	if item.TargetBranch != "" && result.WorktreeBranch != "" {
		mergeBaseSHA, _ := gitOutput(ctx, repoPath, "merge-base", item.TargetBranch, result.WorktreeBranch)
		result.MergeBaseSHA = strings.TrimSpace(mergeBaseSHA)
	}
	if len(result.ChangedFiles) == 0 {
		diffBase := result.MergeBaseSHA
		if diffBase == "" {
			diffBase = strings.TrimSpace(headSHA)
		}
		files, err := gitChangedFiles(ctx, worktreePath, diffBase, strings.TrimSpace(headSHA))
		if err == nil {
			result.ChangedFiles = files
		}
	}
	return nil
}

func parseStructuredWorkerResult(output string) (session.TeamWorkerResult, error) {
	var result session.TeamWorkerResult
	candidate := strings.TrimSpace(output)
	if strings.HasPrefix(candidate, "{") && strings.HasSuffix(candidate, "}") {
		if err := json.Unmarshal([]byte(candidate), &result); err == nil && result.TaskID != "" {
			return result, nil
		}
	}
	start := strings.Index(candidate, "{")
	end := strings.LastIndex(candidate, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(candidate[start:end+1]), &result); err == nil && result.TaskID != "" {
			return result, nil
		}
	}
	return session.TeamWorkerResult{}, fmt.Errorf("structured worker result not found")
}

func structuredFailureResult(item *WorkItem, workerNodeID, taskStatus, message, worktreePath string) *WorkResult {
	if item.Source != WorkSourceStructuredCodexTeam {
		return nil
	}
	return &WorkResult{
		TaskStatus:   taskStatus,
		Summary:      message,
		WorkerNodeID: workerNodeID,
		WorktreePath: worktreePath,
	}
}

func sanitizeWorktreeLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("/", "-", " ", "-", "_", "-", ".", "-")
	value = replacer.Replace(value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "work"
	}
	return value
}

func gitRun(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func gitHasStagedChanges(ctx context.Context, dir string) bool {
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

func gitChangedFiles(ctx context.Context, dir, baseRef, headRef string) ([]string, error) {
	if baseRef == "" {
		out, err := gitOutput(ctx, dir, "diff", "--cached", "--name-only")
		if err != nil {
			return nil, err
		}
		return compactNonEmpty(strings.Split(out, "\n")), nil
	}
	out, err := gitOutput(ctx, dir, "diff", "--name-only", baseRef, headRef)
	if err != nil {
		return nil, err
	}
	return compactNonEmpty(strings.Split(out, "\n")), nil
}

func compactNonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func emptyIfSame(value, repoPath string) string {
	if value == repoPath {
		return ""
	}
	return value
}


=======
func (w *WorkerAgent) resolveRepoPath(ctx context.Context, repoName string) (string, error) {
	if repoName == "" {
		return "", fmt.Errorf("repo name required")
	}
	if w.scanPath == "" {
		return "", fmt.Errorf("scan path not configured for worker")
	}
	repos, err := discovery.Scan(ctx, w.scanPath)
	if err != nil {
		return "", err
	}
	for _, repo := range repos {
		if filepath.Base(repo.Path) == repoName {
			return repo.Path, nil
		}
	}
	return "", fmt.Errorf("repo %s not found on worker", repoName)
}

func (w *WorkerAgent) prepareWorktree(ctx context.Context, repoPath string, item *WorkItem) (string, string, error) {
	branch := sanitizeWorktreeLabel(fmt.Sprintf("ralph-team-%s-%s-%s", item.TeamName, item.TeamTaskID, item.ID))
	worktreePath := filepath.Join(filepath.Dir(repoPath), ".ralph-worktrees", filepath.Base(repoPath), sanitizeWorktreeLabel(item.TeamName), sanitizeWorktreeLabel(item.TeamTaskID), item.ID)
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return "", "", err
	}
	if _, err := os.Stat(worktreePath); err == nil {
		return worktreePath, branch, nil
	}
	if _, err := worktree.CreateWorktree(ctx, repoPath, branch, worktree.WithBaseBranch(item.TargetBranch), worktree.WithPath(worktreePath)); err != nil {
		if createRefErr := worktree.CreateFromRef(ctx, repoPath, worktreePath, branch); createRefErr != nil {
			return "", "", err
		}
	}
	return worktreePath, branch, nil
}

func populateStructuredResult(ctx context.Context, result *WorkResult, item *WorkItem, workerNodeID, repoPath, worktreePath, worktreeBranch, output string) {
	result.WorkerNodeID = workerNodeID
	result.WorktreePath = emptyIfSame(worktreePath, repoPath)
	result.WorktreeBranch = worktreeBranch

	workerResult, err := parseStructuredWorkerResult(output)
	if err != nil {
		result.TaskStatus = session.TeamTaskNeedsRetry
		result.Summary = "worker output did not match structured contract"
		return
	}

	result.TaskStatus = workerResult.Status
	result.Summary = workerResult.Summary
	result.Question = workerResult.Question
	result.ChangedFiles = append([]string(nil), workerResult.ChangedFiles...)

	if worktreePath != "" && worktreePath != repoPath {
		if err := finalizeWorktree(ctx, worktreePath, repoPath, item, result); err != nil {
			result.TaskStatus = session.TeamTaskNeedsRetry
			if result.Summary == "" {
				result.Summary = err.Error()
			}
		}
	}
}

func finalizeWorktree(ctx context.Context, worktreePath, repoPath string, item *WorkItem, result *WorkResult) error {
	if err := gitRun(ctx, worktreePath, "add", "-A"); err != nil {
		return err
	}
	if gitHasStagedChanges(ctx, worktreePath) {
		if err := gitRun(ctx, worktreePath, "commit", "-m", fmt.Sprintf("ralphglasses: %s %s", item.TeamName, item.TeamTaskID)); err != nil {
			return err
		}
	}
	headSHA, _ := gitOutput(ctx, worktreePath, "rev-parse", "HEAD")
	result.HeadSHA = strings.TrimSpace(headSHA)
	if item.TargetBranch != "" && result.WorktreeBranch != "" {
		mergeBaseSHA, _ := gitOutput(ctx, repoPath, "merge-base", item.TargetBranch, result.WorktreeBranch)
		result.MergeBaseSHA = strings.TrimSpace(mergeBaseSHA)
	}
	if len(result.ChangedFiles) == 0 {
		diffBase := result.MergeBaseSHA
		if diffBase == "" {
			diffBase = strings.TrimSpace(headSHA)
		}
		files, err := gitChangedFiles(ctx, worktreePath, diffBase, strings.TrimSpace(headSHA))
		if err == nil {
			result.ChangedFiles = files
		}
	}
	return nil
}

func parseStructuredWorkerResult(output string) (session.TeamWorkerResult, error) {
	var result session.TeamWorkerResult
	candidate := strings.TrimSpace(output)
	if strings.HasPrefix(candidate, "{") && strings.HasSuffix(candidate, "}") {
		if err := json.Unmarshal([]byte(candidate), &result); err == nil && result.TaskID != "" {
			return result, nil
		}
	}
	start := strings.Index(candidate, "{")
	end := strings.LastIndex(candidate, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(candidate[start:end+1]), &result); err == nil && result.TaskID != "" {
			return result, nil
		}
	}
	return session.TeamWorkerResult{}, fmt.Errorf("structured worker result not found")
}

func structuredFailureResult(item *WorkItem, workerNodeID, taskStatus, message, worktreePath string) *WorkResult {
	if item.Source != WorkSourceStructuredCodexTeam {
		return nil
	}
	return &WorkResult{
		TaskStatus:   taskStatus,
		Summary:      message,
		WorkerNodeID: workerNodeID,
		WorktreePath: worktreePath,
	}
}

func sanitizeWorktreeLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("/", "-", " ", "-", "_", "-", ".", "-")
	value = replacer.Replace(value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "work"
	}
	return value
}

func gitRun(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func gitHasStagedChanges(ctx context.Context, dir string) bool {
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

func gitChangedFiles(ctx context.Context, dir, baseRef, headRef string) ([]string, error) {
	if baseRef == "" {
		out, err := gitOutput(ctx, dir, "diff", "--cached", "--name-only")
		if err != nil {
			return nil, err
		}
		return compactNonEmpty(strings.Split(out, "\n")), nil
	}
	out, err := gitOutput(ctx, dir, "diff", "--name-only", baseRef, headRef)
	if err != nil {
		return nil, err
	}
	return compactNonEmpty(strings.Split(out, "\n")), nil
}

func compactNonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func emptyIfSame(value, repoPath string) string {
	if value == repoPath {
		return ""
	}
	return value
}

>>>>>>> Stashed changes
func (w *WorkerAgent) discoverProviders() []session.Provider {
	var providers []session.Provider
	for _, p := range []session.Provider{session.ProviderCodex, session.ProviderGemini, session.ProviderClaude} {
		if err := session.ValidateProvider(p); err == nil {
			providers = append(providers, p)
		}
	}
	if len(providers) == 0 {
		providers = []session.Provider{session.DefaultPrimaryProvider()}
	}
	return providers
}

// NodeID returns the worker's assigned node ID (empty before registration).
func (w *WorkerAgent) NodeID() string {
	return w.nodeID
}

// DiscoverTailscaleIP gets the node's Tailscale IP, or empty string if unavailable.
// It queries the Tailscale status (via LocalAPI or CLI) and extracts the first
// IPv4 address from the self node.
func DiscoverTailscaleIP() string {
	status, err := GetTailscaleStatus()
	if err != nil {
		util.Debug.Debugf("DiscoverTailscaleIP: tailscale unavailable: %v", err)
		return ""
	}

	// Prefer the first IPv4 address from the self node.
	for _, ip := range status.Self.TailscaleIPs {
		parsed := net.ParseIP(ip)
		if parsed != nil && parsed.To4() != nil {
			return ip
		}
	}

	// Fall back to the first address of any family.
	if len(status.Self.TailscaleIPs) > 0 {
		return status.Self.TailscaleIPs[0]
	}
	return ""
}
