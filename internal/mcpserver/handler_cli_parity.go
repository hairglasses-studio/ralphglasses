package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/automation"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/firstboot"
	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
	"github.com/hairglasses-studio/ralphglasses/internal/marathon"
	"github.com/hairglasses-studio/ralphglasses/internal/parity"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

type fleetRuntimeState struct {
	ID              string
	Mode            string
	Port            int
	CoordinatorURL  string
	FleetBudget     float64
	Automation      bool
	StartedAt       time.Time
	EndedAt         *time.Time
	Active          bool
	LastError       string
	TaskID          string
	WorkerID        string
	cancel          context.CancelFunc
	coordinator     *fleet.Coordinator
	worker          *fleet.WorkerAgent
	automationState *automation.Runtime
}

type marathonRuntimeState struct {
	ID                 string
	Repo               string
	RepoPath           string
	BudgetUSD          float64
	Duration           time.Duration
	CheckpointInterval time.Duration
	Resume             bool
	StartedAt          time.Time
	EndedAt            *time.Time
	Active             bool
	LastError          string
	TaskID             string
	Warnings           []string
	ValidationErrors   []string
	LastStats          *marathon.Stats
	cancel             context.CancelFunc
	runner             *marathon.Marathon
}

func (s *Server) runtimeVersion() string {
	version := strings.TrimSpace(s.Version)
	if version == "" {
		version = "dev"
	}
	if strings.TrimSpace(s.Commit) != "" {
		return fmt.Sprintf("%s (%s)", version, s.Commit)
	}
	return version
}

func (s *Server) ensureEventBus() *events.Bus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.EventBus == nil {
		s.EventBus = events.NewBus(1000)
	}
	if s.SessMgr == nil {
		s.SessMgr = session.NewManagerWithBus(s.EventBus)
		s.SessMgr.Init()
	}
	return s.EventBus
}

func (s *Server) resolveScanPathOverride(path string) (string, *mcp.CallToolResult) {
	if strings.TrimSpace(path) == "" {
		return s.ScanPath, nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return "", codedError(ErrInvalidParams, fmt.Sprintf("invalid scan_path: %v", err))
	}
	return path, nil
}

func parseTimestampArg(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, time.DateOnly, "2006-01-02 15:04:05"} {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp %q", raw)
}

func redactProfile(profile firstboot.Profile) map[string]any {
	apiKeys := map[string]string{}
	for key, value := range profile.APIKeys {
		if strings.TrimSpace(value) == "" {
			apiKeys[key] = ""
			continue
		}
		apiKeys[key] = parity.RedactKey(value)
	}
	return map[string]any{
		"hostname":              profile.Hostname,
		"api_keys":              apiKeys,
		"autonomy_level":        profile.Autonomy,
		"fleet_coordinator_url": profile.FleetURL,
	}
}

func (s *Server) handleDoctor(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)
	scanPath, errResult := s.resolveScanPathOverride(p.OptionalString("scan_path", ""))
	if errResult != nil {
		return errResult, nil
	}
	report := parity.RunDoctor(ctx, parity.DoctorOptions{
		ScanPath:        scanPath,
		Checks:          p.StringSlice("checks", ","),
		IncludeOptional: !p.Has("include_optional") || p.OptionalBool("include_optional", true),
	})
	return jsonResult(report), nil
}

func (s *Server) handleValidate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)
	scanPath, errResult := s.resolveScanPathOverride(p.OptionalString("scan_path", ""))
	if errResult != nil {
		return errResult, nil
	}
	results, err := parity.ValidateRepos(ctx, parity.ValidateOptions{
		ScanPath:     scanPath,
		Repo:         p.OptionalString("repo", ""),
		Repos:        p.StringSlice("repos", ","),
		IncludeClean: p.OptionalBool("include_clean", false),
		Strict:       p.OptionalBool("strict", false),
	})
	if err != nil {
		return codedError(ErrScanFailed, err.Error()), nil
	}
	return jsonResult(map[string]any{
		"results":   results,
		"has_error": parity.ValidationHasError(results),
		"count":     len(results),
	}), nil
}

func (s *Server) handleConfigSchema(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)
	includeConstraints := true
	if p.Has("include_constraints") {
		includeConstraints = p.OptionalBool("include_constraints", true)
	}
	schema := parity.ConfigSchema(parity.ConfigSchemaOptions{
		Key:                p.OptionalString("key", ""),
		IncludeDefaults:    p.OptionalBool("include_defaults", false),
		IncludeConstraints: includeConstraints,
	})
	return jsonResult(map[string]any{
		"keys":  schema,
		"count": len(schema),
	}), nil
}

func (s *Server) handleDebugBundle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)
	action, errResult := p.OptionalEnum("action", []string{"view", "save"}, "view")
	if errResult != nil {
		return errResult, nil
	}

	targetRoot := s.ScanPath
	repo := p.OptionalString("repo", "")
	if repo != "" {
		repoPath, repoErr := s.resolveRepoPath(repo)
		if repoErr != nil {
			return repoErr, nil
		}
		targetRoot = repoPath
	}

	content, err := parity.BuildDebugBundle(ctx, parity.DebugBundleOptions{
		ScanPath:  targetRoot,
		Version:   s.Version,
		Commit:    s.Commit,
		BuildDate: s.BuildDate,
		Sections:  p.StringSlice("sections", ","),
	})
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("build debug bundle: %v", err)), nil
	}

	if action == "view" {
		return jsonResult(map[string]any{
			"action":  action,
			"repo":    repo,
			"content": content,
		}), nil
	}

	saveDir := filepath.Join(targetRoot, ".ralph", "debug")
	outputPath := parity.DefaultDebugBundlePath(saveDir, time.Now())
	if name := p.OptionalString("name", ""); name != "" {
		if err := validateSafePath(name); err != nil {
			return codedError(ErrInvalidParams, fmt.Sprintf("name: %v", err)), nil
		}
		outputPath = filepath.Join(saveDir, name)
	}
	if err := parity.WriteDebugBundle(outputPath, content); err != nil {
		return codedError(ErrFilesystem, err.Error()), nil
	}
	return jsonResult(map[string]any{
		"action":     action,
		"repo":       repo,
		"saved_path": outputPath,
		"bytes":      len(content),
	}), nil
}

func (s *Server) handleThemeExport(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)
	format, errResult := p.RequireString("format")
	if errResult != nil {
		return errResult, nil
	}
	theme, errResult := p.RequireString("theme")
	if errResult != nil {
		return errResult, nil
	}
	export, err := parity.ExportTheme(format, theme)
	if err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}
	return jsonResult(export), nil
}

func (s *Server) handleTelemetryExport(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)
	since, err := parseTimestampArg(p.OptionalString("since", ""))
	if err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("since: %v", err)), nil
	}
	until, err := parseTimestampArg(p.OptionalString("until", ""))
	if err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("until: %v", err)), nil
	}
	format, errResult := p.OptionalEnum("format", []string{"json", "csv"}, "json")
	if errResult != nil {
		return errResult, nil
	}
	events, err := parity.LoadTelemetry(parity.TelemetryOptions{
		Since:    since,
		Until:    until,
		Repo:     p.OptionalString("repo", ""),
		Provider: p.OptionalString("provider", ""),
		Type:     p.OptionalString("type", ""),
		Limit:    p.OptionalInt("limit", 0),
	})
	if err != nil {
		return codedError(ErrFilesystem, err.Error()), nil
	}

	payload := ""
	switch format {
	case "csv":
		payload, err = parity.TelemetryCSV(events)
	default:
		payload, err = parity.TelemetryJSON(events)
	}
	if err != nil {
		return codedError(ErrInternal, err.Error()), nil
	}

	return jsonResult(map[string]any{
		"format":  format,
		"count":   len(events),
		"content": payload,
		"events":  events,
	}), nil
}

func (s *Server) handleFirstbootProfile(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)
	action, errResult := p.OptionalEnum("action", []string{"get", "set", "validate", "mark_done"}, "get")
	if errResult != nil {
		return errResult, nil
	}
	configDir := p.OptionalString("config_dir", "")
	profile, done, err := firstboot.Load(configDir)
	if err != nil {
		return codedError(ErrFilesystem, err.Error()), nil
	}

	if p.Has("hostname") {
		profile.Hostname = p.OptionalString("hostname", "")
	}
	if p.Has("autonomy_level") {
		profile.Autonomy = p.OptionalInt("autonomy_level", profile.Autonomy)
	}
	if p.Has("coordinator_url") {
		profile.FleetURL = p.OptionalString("coordinator_url", "")
	}
	if profile.APIKeys == nil {
		profile.APIKeys = firstboot.DefaultProfile().APIKeys
	}
	if p.Has("anthropic_api_key") {
		profile.APIKeys["anthropic"] = p.OptionalString("anthropic_api_key", "")
	}
	if p.Has("google_api_key") {
		profile.APIKeys["google"] = p.OptionalString("google_api_key", "")
	}
	if p.Has("openai_api_key") {
		profile.APIKeys["openai"] = p.OptionalString("openai_api_key", "")
	}

	issues := firstboot.Validate(profile)
	switch action {
	case "set":
		if err := firstboot.Save(configDir, profile, false); err != nil {
			return codedError(ErrFilesystem, err.Error()), nil
		}
		done = false
	case "mark_done":
		if err := firstboot.Save(configDir, profile, true); err != nil {
			return codedError(ErrFilesystem, err.Error()), nil
		}
		done = true
	}

	if configDir == "" {
		configDir = firstboot.DefaultConfigDir()
	}
	return jsonResult(map[string]any{
		"action":     action,
		"config_dir": configDir,
		"done":       done,
		"issues":     issues,
		"profile":    redactProfile(profile),
	}), nil
}

func (s *Server) handleBudgetStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)
	if s.SessMgr == nil {
		s.ensureEventBus()
	}
	tenantID := session.NormalizeTenantID(p.OptionalString("tenant_id", session.DefaultTenantID))
	sessions := s.SessMgr.ListByTenant("", tenantID)
	var totalSpent, totalBudget float64
	var active, completed int
	for _, sess := range sessions {
		sess.Lock()
		totalSpent += sess.SpentUSD
		totalBudget += sess.BudgetUSD
		if sess.Status.IsTerminal() {
			completed++
		} else {
			active++
		}
		sess.Unlock()
	}
	utilization := 0.0
	if totalBudget > 0 {
		utilization = totalSpent / totalBudget
	}
	return jsonResult(map[string]any{
		"tenant_id":          tenantID,
		"total_spent_usd":    totalSpent,
		"total_budget_usd":   totalBudget,
		"sessions_active":    active,
		"sessions_done":      completed,
		"session_count":      len(sessions),
		"budget_utilization": utilization,
	}), nil
}

func (s *Server) handleRepoSurfaceAudit(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)
	repo, errResult := p.RequireString("repo")
	if errResult != nil {
		return errResult, nil
	}
	repoPath, repoErr := s.resolveRepoPath(repo)
	if repoErr != nil {
		return repoErr, nil
	}
	return jsonResult(parity.AuditRepoSurface(repoPath)), nil
}

func (s *Server) handleWorktreeList(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)
	repo, errResult := p.RequireString("repo")
	if errResult != nil {
		return errResult, nil
	}
	repoPath, repoErr := s.resolveRepoPath(repo)
	if repoErr != nil {
		return repoErr, nil
	}
	staleAfter := time.Duration(0)
	if p.OptionalBool("include_stale", false) {
		staleAfter = time.Duration(p.OptionalInt("stale_after_hours", 24)) * time.Hour
	}
	worktrees, err := parity.ListWorktrees(repoPath, p.OptionalBool("dirty_only", false), staleAfter)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("list worktrees: %v", err)), nil
	}
	staleCount := 0
	for _, wt := range worktrees {
		if wt.Stale {
			staleCount++
		}
	}
	return jsonResult(map[string]any{
		"repo":        repo,
		"count":       len(worktrees),
		"stale_count": staleCount,
		"worktrees":   worktrees,
	}), nil
}

func (s *Server) handleFleetRuntime(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)
	action, errResult := p.OptionalEnum("action", []string{"status", "discover", "start", "stop", "restart"}, "status")
	if errResult != nil {
		return errResult, nil
	}
	port := p.OptionalInt("port", fleet.DefaultPort)
	if port <= 0 {
		port = fleet.DefaultPort
	}

	switch action {
	case "discover":
		url := fleet.DiscoverCoordinator(port)
		return jsonResult(map[string]any{
			"action":      action,
			"port":        port,
			"discovered":  url != "",
			"coordinator": url,
		}), nil
	case "status":
		return jsonResult(s.fleetRuntimeStatus(ctx)), nil
	case "stop":
		return s.stopFleetRuntime()
	case "restart":
		if _, err := s.stopFleetRuntime(); err != nil {
			return nil, err
		}
	}

	mode, errResult := p.OptionalEnum("mode", []string{"coordinator", "worker"}, "worker")
	if errResult != nil {
		return errResult, nil
	}

	s.mu.RLock()
	active := s.FleetRuntime != nil && s.FleetRuntime.Active
	s.mu.RUnlock()
	if active {
		return codedError(ErrInternal, "fleet runtime already active — stop or restart it first"), nil
	}

	bus := s.ensureEventBus()
	ctxRun, cancel := context.WithCancel(context.Background())
	automationEnabled := !p.Has("automation") || p.OptionalBool("automation", true)
	rt := &fleetRuntimeState{
		ID:             fmt.Sprintf("fleet-%d", time.Now().UnixMilli()),
		Mode:           mode,
		Port:           port,
		CoordinatorURL: p.OptionalString("coordinator_url", ""),
		FleetBudget:    p.OptionalNumber("fleet_budget", 500),
		Automation:     automationEnabled,
		StartedAt:      time.Now(),
		Active:         true,
		cancel:         cancel,
	}

	if mode == "coordinator" {
		hostname := fleet.GetHostname()
		nodeID := fmt.Sprintf("coord-%s", hostname)
		coord := fleet.NewCoordinatorWithPersistence(nodeID, hostname, port, s.runtimeVersion(), bus, s.SessMgr, s.ScanPath)
		coord.SetBudgetLimit(rt.FleetBudget)
		rt.coordinator = coord
		s.FleetCoordinator = coord
		s.FleetClient = fleet.NewClient(fmt.Sprintf("http://127.0.0.1:%d", port))
	} else {
		if rt.CoordinatorURL == "" {
			rt.CoordinatorURL = fleet.DiscoverCoordinator(port)
			if rt.CoordinatorURL == "" {
				cancel()
				return codedError(ErrServiceNotFound, "could not discover coordinator; provide coordinator_url"), nil
			}
		}
		rt.worker = fleet.NewWorkerAgent(rt.CoordinatorURL, fleet.GetHostname(), port, s.runtimeVersion(), s.ScanPath, bus, s.SessMgr)
		if s.FleetCoordinator == nil {
			s.FleetClient = fleet.NewClient(rt.CoordinatorURL)
		}
	}

	if automationEnabled {
		runtime, err := automation.StartServeRuntime(ctxRun, s.ScanPath, true, bus, s.SessMgr)
		if err != nil {
			cancel()
			s.FleetCoordinator = nil
			s.FleetClient = nil
			return codedError(ErrInternal, fmt.Sprintf("start automation runtime: %v", err)), nil
		}
		rt.automationState = runtime
	}

	if s.Tasks == nil {
		s.Tasks = NewTaskRegistry()
	}
	rt.TaskID = s.Tasks.Create("fleet_runtime", cancel, map[string]any{
		"runtime_id": rt.ID,
		"mode":       rt.Mode,
	})

	s.mu.Lock()
	s.FleetRuntime = rt
	s.mu.Unlock()

	go s.runFleetRuntime(ctxRun, rt)

	return jsonResult(map[string]any{
		"task_id":         rt.TaskID,
		"runtime_id":      rt.ID,
		"mode":            rt.Mode,
		"coordinator_url": rt.CoordinatorURL,
		"port":            rt.Port,
		"automation":      rt.Automation,
		"status":          "starting",
	}), nil
}

func (s *Server) runFleetRuntime(ctx context.Context, rt *fleetRuntimeState) {
	var err error
	if rt.coordinator != nil {
		err = rt.coordinator.Start(ctx)
	} else if rt.worker != nil {
		err = rt.worker.Run(ctx)
	}

	s.mu.Lock()
	if rt.worker != nil && rt.WorkerID == "" {
		rt.WorkerID = rt.worker.NodeID()
	}
	now := time.Now()
	rt.EndedAt = &now
	rt.Active = false
	if err != nil && err != context.Canceled {
		rt.LastError = err.Error()
	}
	if rt.automationState != nil {
		rt.automationState.Stop()
		rt.automationState = nil
	}
	if s.FleetCoordinator == rt.coordinator {
		s.FleetCoordinator = nil
	}
	if s.FleetCoordinator == nil {
		s.FleetClient = nil
	}
	s.mu.Unlock()

	task := s.Tasks.Get(rt.TaskID)
	if task == nil || task.State == TaskCanceled {
		return
	}
	if rt.LastError != "" {
		s.Tasks.Fail(rt.TaskID, rt.LastError)
		return
	}
	s.Tasks.Complete(rt.TaskID, s.fleetRuntimeSnapshot(rt))
}

func (s *Server) fleetRuntimeSnapshot(rt *fleetRuntimeState) map[string]any {
	if rt == nil {
		return map[string]any{"status": "idle"}
	}
	return map[string]any{
		"runtime_id":      rt.ID,
		"mode":            rt.Mode,
		"port":            rt.Port,
		"coordinator_url": rt.CoordinatorURL,
		"fleet_budget":    rt.FleetBudget,
		"automation":      rt.Automation,
		"started_at":      rt.StartedAt,
		"ended_at":        rt.EndedAt,
		"active":          rt.Active,
		"last_error":      rt.LastError,
		"task_id":         rt.TaskID,
		"worker_id":       rt.WorkerID,
	}
}

func (s *Server) fleetRuntimeStatus(ctx context.Context) map[string]any {
	s.mu.Lock()
	rt := s.FleetRuntime
	if rt != nil && rt.worker != nil && rt.WorkerID == "" {
		rt.WorkerID = rt.worker.NodeID()
	}
	snapshot := s.fleetRuntimeSnapshot(rt)
	s.mu.Unlock()
	if rt == nil {
		return snapshot
	}

	if rt.Mode == "coordinator" {
		client := fleet.NewClient(fmt.Sprintf("http://127.0.0.1:%d", rt.Port))
		if status, err := client.Status(ctx); err == nil {
			snapshot["node_status"] = status
		} else if rt.Active {
			snapshot["status_error"] = err.Error()
		}
		if state, err := client.FleetState(ctx); err == nil {
			snapshot["fleet_state"] = state
		}
		return snapshot
	}

	if rt.CoordinatorURL != "" {
		client := fleet.NewClient(rt.CoordinatorURL)
		if state, err := client.FleetState(ctx); err == nil {
			snapshot["fleet_state"] = state
			for _, worker := range state.Workers {
				if worker.ID == rt.WorkerID {
					snapshot["worker"] = worker
					break
				}
			}
		} else if rt.Active {
			snapshot["coordinator_error"] = err.Error()
		}
	}
	return snapshot
}

func (s *Server) stopFleetRuntime() (*mcp.CallToolResult, error) {
	s.mu.RLock()
	rt := s.FleetRuntime
	s.mu.RUnlock()
	if rt == nil {
		return jsonResult(map[string]any{"status": "idle"}), nil
	}
	if rt.automationState != nil {
		rt.automationState.Stop()
	}
	if s.Tasks != nil && rt.TaskID != "" {
		_ = s.Tasks.Cancel(rt.TaskID)
	}
	if rt.cancel != nil {
		rt.cancel()
	}
	if rt.coordinator != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = rt.coordinator.Stop(stopCtx)
		cancel()
	}
	return jsonResult(map[string]any{
		"status":  "stopping",
		"runtime": s.fleetRuntimeSnapshot(rt),
	}), nil
}

func (s *Server) handleMarathon(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)
	action, errResult := p.OptionalEnum("action", []string{"status", "start", "resume", "stop"}, "status")
	if errResult != nil {
		return errResult, nil
	}

	switch action {
	case "status":
		return jsonResult(s.marathonStatus()), nil
	case "stop":
		return s.stopMarathon()
	}

	repo, errResult := p.RequireString("repo")
	if errResult != nil {
		return errResult, nil
	}
	repoPath, repoErr := s.resolveRepoPath(repo)
	if repoErr != nil {
		return repoErr, nil
	}

	s.mu.RLock()
	active := s.MarathonRuntime != nil && s.MarathonRuntime.Active
	s.mu.RUnlock()
	if active {
		return codedError(ErrInternal, "marathon already active — stop it before starting another"), nil
	}

	duration := time.Hour
	if raw := p.OptionalString("duration", "1h"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return codedError(ErrInvalidParams, fmt.Sprintf("duration: %v", err)), nil
		}
		duration = parsed
	}
	checkpointInterval := 10 * time.Minute
	if raw := p.OptionalString("checkpoint_interval", "10m"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return codedError(ErrInvalidParams, fmt.Sprintf("checkpoint_interval: %v", err)), nil
		}
		checkpointInterval = parsed
	}

	validation := session.ValidateConfig(repoPath)
	if !validation.OK() {
		return codedError(ErrInvalidParams, fmt.Sprintf("pre-flight validation failed: %s", strings.Join(validation.Errors, "; "))), nil
	}

	s.ensureEventBus()
	s.SessMgr.SetAutonomyLevel(session.LevelAutoOptimize, repoPath)

	ctxRun, cancel := context.WithCancel(context.Background())
	cfg := marathon.Config{
		BudgetUSD:          p.OptionalNumber("budget_usd", 10.0),
		Duration:           duration,
		CheckpointInterval: checkpointInterval,
		RepoPath:           repoPath,
		Resume:             action == "resume",
	}
	rt := &marathonRuntimeState{
		ID:                 fmt.Sprintf("marathon-%d", time.Now().UnixMilli()),
		Repo:               repo,
		RepoPath:           repoPath,
		BudgetUSD:          cfg.BudgetUSD,
		Duration:           cfg.Duration,
		CheckpointInterval: cfg.CheckpointInterval,
		Resume:             cfg.Resume,
		StartedAt:          time.Now(),
		Active:             true,
		Warnings:           append([]string(nil), validation.Warnings...),
		ValidationErrors:   append([]string(nil), validation.Errors...),
		cancel:             cancel,
		runner:             marathon.New(cfg, s.SessMgr, s.EventBus),
	}

	if s.Tasks == nil {
		s.Tasks = NewTaskRegistry()
	}
	rt.TaskID = s.Tasks.Create("marathon", cancel, map[string]any{
		"runtime_id": rt.ID,
		"repo":       rt.Repo,
		"action":     action,
	})

	s.mu.Lock()
	s.MarathonRuntime = rt
	s.mu.Unlock()

	go s.runMarathon(ctxRun, rt)

	return jsonResult(map[string]any{
		"task_id":    rt.TaskID,
		"runtime_id": rt.ID,
		"repo":       rt.Repo,
		"status":     "started",
		"warnings":   rt.Warnings,
		"budget_usd": rt.BudgetUSD,
		"duration":   rt.Duration.String(),
		"checkpoint": rt.CheckpointInterval.String(),
		"resume":     rt.Resume,
	}), nil
}

func (s *Server) runMarathon(ctx context.Context, rt *marathonRuntimeState) {
	stats, err := rt.runner.Run(ctx)

	s.mu.Lock()
	now := time.Now()
	rt.EndedAt = &now
	rt.Active = false
	if stats != nil {
		rt.LastStats = stats
	}
	if err != nil && err != context.Canceled {
		rt.LastError = err.Error()
	}
	s.mu.Unlock()

	task := s.Tasks.Get(rt.TaskID)
	if task == nil || task.State == TaskCanceled {
		return
	}
	if rt.LastError != "" {
		s.Tasks.Fail(rt.TaskID, rt.LastError)
		return
	}
	s.Tasks.Complete(rt.TaskID, s.marathonStatus())
}

func (s *Server) marathonStatus() map[string]any {
	s.mu.RLock()
	rt := s.MarathonRuntime
	s.mu.RUnlock()
	if rt == nil {
		return map[string]any{"status": "idle"}
	}

	status := map[string]any{
		"runtime_id":          rt.ID,
		"repo":                rt.Repo,
		"repo_path":           rt.RepoPath,
		"budget_usd":          rt.BudgetUSD,
		"duration":            rt.Duration.String(),
		"checkpoint_interval": rt.CheckpointInterval.String(),
		"resume":              rt.Resume,
		"started_at":          rt.StartedAt,
		"ended_at":            rt.EndedAt,
		"active":              rt.Active,
		"last_error":          rt.LastError,
		"task_id":             rt.TaskID,
		"warnings":            rt.Warnings,
	}
	if rt.runner != nil {
		status["marathon"] = rt.runner.Status()
		status["stats"] = rt.runner.CurrentStats()
	}
	if rt.LastStats != nil {
		status["final_stats"] = rt.LastStats
	}
	checkpoints, err := marathon.ListCheckpoints(filepath.Join(rt.RepoPath, ".ralph", "marathon", "checkpoints"))
	if err == nil {
		status["checkpoint_count"] = len(checkpoints)
		if len(checkpoints) > 0 {
			status["latest_checkpoint"] = checkpoints[len(checkpoints)-1]
		}
	}
	return status
}

func (s *Server) stopMarathon() (*mcp.CallToolResult, error) {
	s.mu.RLock()
	rt := s.MarathonRuntime
	s.mu.RUnlock()
	if rt == nil {
		return jsonResult(map[string]any{"status": "idle"}), nil
	}
	if s.Tasks != nil && rt.TaskID != "" {
		_ = s.Tasks.Cancel(rt.TaskID)
	}
	if rt.cancel != nil {
		rt.cancel()
	}
	return jsonResult(map[string]any{
		"status":  "stopping",
		"runtime": s.marathonStatus(),
	}), nil
}
