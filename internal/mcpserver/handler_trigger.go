package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/appdir"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// TriggerRecord represents a request to trigger an agent session externally.
type TriggerRecord struct {
	ID        string        `json:"id"`
	TenantID  string        `json:"tenant_id,omitempty"`
	Prompt    string        `json:"prompt"`
	AgentType string        `json:"agent_type"`
	Priority  int           `json:"priority"`
	Config    TriggerConfig `json:"config,omitempty"`
	Status    string        `json:"status"`
	SessionID string        `json:"session_id,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
}

// TriggerConfig holds optional configuration overrides for a trigger.
type TriggerConfig struct {
	Model     string  `json:"model,omitempty"`
	BudgetUSD float64 `json:"budget_usd,omitempty"`
	MaxTurns  int     `json:"max_turns,omitempty"`
}

// ScheduleEntry represents a cron-based schedule for recurring agent triggers.
type ScheduleEntry struct {
	ID             string    `json:"id"`
	Prompt         string    `json:"prompt"`
	CronExpression string    `json:"cron_expression"`
	AgentType      string    `json:"agent_type"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// scheduleStore manages persistent schedule entries.
type scheduleStore struct {
	mu   sync.RWMutex
	path string
}

var validAgentTypes = []string{"ralph", "loop", "cycle"}

func (s *Server) handleTriggerWebhook(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	prompt, errResult := p.RequireString("prompt")
	if errResult != nil {
		return errResult, nil
	}
	if err := ValidateStringLength(prompt, MaxPromptLength, "prompt"); err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}

	agentType, errResult := p.RequireEnum("agent_type", validAgentTypes)
	if errResult != nil {
		return errResult, nil
	}

	priority := p.OptionalInt("priority", 5)
	if priority < 1 || priority > 10 {
		return codedError(ErrInvalidParams, "priority must be between 1 and 10"), nil
	}

	// Parse optional config.
	var cfg TriggerConfig
	cfg.Model = p.OptionalString("model", "")
	cfg.BudgetUSD = p.OptionalNumber("budget_usd", 0)
	cfg.MaxTurns = p.OptionalInt("max_turns", 0)

	triggerID := fmt.Sprintf("trig-%d", time.Now().UnixNano())
	tenantID := session.NormalizeTenantID(p.OptionalString("tenant_id", ""))
	record := TriggerRecord{
		ID:        triggerID,
		TenantID:  tenantID,
		Prompt:    prompt,
		AgentType: agentType,
		Priority:  priority,
		Config:    cfg,
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	// Determine whether to launch immediately or just queue.
	launch := p.OptionalBool("launch", false)

	if launch {
		if s.reposNil() {
			if err := s.scan(); err != nil {
				return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
			}
		}

		repo := p.OptionalString("repo", "")
		if repo == "" {
			return codedError(ErrInvalidParams, "repo required when launch=true"), nil
		}
		if err := ValidateRepoName(repo); err != nil {
			return codedError(ErrRepoNameInvalid, fmt.Sprintf("invalid repo name: %v", err)), nil
		}

		r := s.findRepo(repo)
		if r == nil {
			return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", repo)), nil
		}

		switch agentType {
		case "ralph", "loop":
			profile := session.DefaultLoopProfile()
			if cfg.Model != "" {
				profile.WorkerModel = cfg.Model
			}
			if cfg.BudgetUSD > 0 {
				profile.PlannerBudgetUSD = cfg.BudgetUSD / 3
				profile.WorkerBudgetUSD = cfg.BudgetUSD * 2 / 3
			}
			run, err := s.SessMgr.StartLoopForTenant(ctx, tenantID, r.Path, profile)
			if err != nil {
				record.Status = "failed"
				return codedError(ErrLoopStart, fmt.Sprintf("trigger launch failed: %v", err)), nil
			}
			record.Status = "launched"
			record.SessionID = run.ID
		case "cycle":
			opts := session.LaunchOptions{
				TenantID:     tenantID,
				Provider:     session.DefaultPrimaryProvider(),
				RepoPath:     r.Path,
				Prompt:       prompt,
				Model:        cfg.Model,
				MaxBudgetUSD: cfg.BudgetUSD,
				MaxTurns:     cfg.MaxTurns,
			}
			sess, err := s.SessMgr.Launch(ctx, opts)
			if err != nil {
				record.Status = "failed"
				return codedError(ErrLaunchFailed, fmt.Sprintf("trigger launch failed: %v", err)), nil
			}
			record.Status = "launched"
			record.SessionID = sess.ID
		}
	}

	return jsonResult(map[string]any{
		"trigger_id": record.ID,
		"tenant_id":  record.TenantID,
		"status":     record.Status,
		"agent_type": record.AgentType,
		"priority":   record.Priority,
		"session_id": record.SessionID,
		"created_at": record.CreatedAt,
	}), nil
}

func (s *Server) handleScheduleCreate(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	action, errResult := p.OptionalEnum("action", []string{"create", "list", "enable", "disable"}, "create")
	if errResult != nil {
		return errResult, nil
	}
	repo := p.OptionalString("repo", "")
	if repo != "" {
		repoPath, errRes := s.resolveRepoPath(repo)
		if errRes != nil {
			return errRes, nil
		}
		switch action {
		case "list":
			entries, err := listRepoScheduleEntries(repoPath)
			if err != nil {
				return codedError(ErrInternal, fmt.Sprintf("load repo schedules: %v", err)), nil
			}
			schedules := make([]map[string]any, 0, len(entries))
			for _, entry := range entries {
				presentation := repoSchedulePresentation(entry)
				presentation["repo"] = repo
				schedules = append(schedules, presentation)
			}
			return jsonResult(map[string]any{
				"schedules": schedules,
				"count":     len(schedules),
				"repo":      repo,
			}), nil
		case "enable", "disable":
			id, errResult := p.RequireString("id")
			if errResult != nil {
				return errResult, nil
			}
			entry, path, err := updateRepoScheduleEnabled(repoPath, id, action == "enable")
			if err != nil {
				return codedError(ErrInvalidParams, err.Error()), nil
			}
			presentation := repoSchedulePresentation(*entry)
			presentation["repo"] = repo
			presentation["path"] = path
			presentation["action"] = action
			presentation["status"] = "ok"
			return jsonResult(presentation), nil
		default:
			prompt, errResult := p.RequireString("prompt")
			if errResult != nil {
				return errResult, nil
			}
			if err := ValidateStringLength(prompt, MaxPromptLength, "prompt"); err != nil {
				return codedError(ErrInvalidParams, err.Error()), nil
			}
			cronExpr, errResult := p.RequireString("cron_expression")
			if errResult != nil {
				return errResult, nil
			}
			if err := session.ValidateAutomationCron(cronExpr); err != nil {
				return codedError(ErrInvalidParams, fmt.Sprintf("invalid cron expression: %v", err)), nil
			}
			agentType, errResult := p.OptionalEnum("agent_type", validAgentTypes, "ralph")
			if errResult != nil {
				return errResult, nil
			}
			enabled := p.OptionalBool("enabled", true)
			config := map[string]any{
				"provider":   p.OptionalString("provider", ""),
				"model":      p.OptionalString("model", ""),
				"budget_usd": p.OptionalNumber("budget_usd", 0),
				"max_turns":  p.OptionalInt("max_turns", 0),
				"priority":   p.OptionalInt("priority", 5),
			}
			switch agentType {
			case "loop":
				config["job_kind"] = "loop"
				config["prompt"] = prompt
			case "cycle":
				config["job_kind"] = "cycle"
				config["name"] = p.OptionalString("name", "")
				config["objective"] = firstNonEmptyString(p.OptionalString("objective", ""), prompt)
				if criteria := p.OptionalString("criteria", ""); criteria != "" {
					config["criteria"] = splitCSV(criteria)
				}
				if maxTasks := p.OptionalInt("max_tasks", 0); maxTasks > 0 {
					config["max_tasks"] = maxTasks
				}
			default:
				config["job_kind"] = "session"
				config["prompt"] = prompt
			}
			configJSON, _ := json.Marshal(config)
			entry := repoScheduleEntry{
				ScheduleID:  fmt.Sprintf("sched-%d", time.Now().UnixNano()),
				CronExpr:    cronExpr,
				CycleConfig: string(configJSON),
				CreatedAt:   time.Now().UTC().Format(time.RFC3339),
				Enabled:     defaultEnabledPointer(enabled),
			}
			path, err := writeRepoScheduleEntry(repoPath, entry)
			if err != nil {
				return codedError(ErrInternal, fmt.Sprintf("save repo schedule: %v", err)), nil
			}
			entry.NextRuns, _ = computeScheduleNextRuns(cronExpr, 3)
			presentation := repoSchedulePresentation(entry)
			presentation["repo"] = repo
			presentation["path"] = path
			presentation["status"] = "created"
			return jsonResult(presentation), nil
		}
	}

	store := &scheduleStore{path: schedulesPath()}

	switch action {
	case "list":
		entries, err := store.load()
		if err != nil {
			return codedError(ErrInternal, fmt.Sprintf("load schedules: %v", err)), nil
		}
		return jsonResult(map[string]any{
			"schedules": entries,
			"count":     len(entries),
		}), nil

	case "enable", "disable":
		id, errResult := p.RequireString("id")
		if errResult != nil {
			return errResult, nil
		}
		entries, err := store.load()
		if err != nil {
			return codedError(ErrInternal, fmt.Sprintf("load schedules: %v", err)), nil
		}
		found := false
		for i := range entries {
			if entries[i].ID == id {
				entries[i].Enabled = action == "enable"
				entries[i].UpdatedAt = time.Now()
				found = true
				break
			}
		}
		if !found {
			return codedError(ErrInvalidParams, fmt.Sprintf("schedule not found: %s", id)), nil
		}
		if err := store.save(entries); err != nil {
			return codedError(ErrInternal, fmt.Sprintf("save schedules: %v", err)), nil
		}
		return jsonResult(map[string]any{
			"action": action,
			"id":     id,
			"status": "ok",
		}), nil

	default: // "create"
		prompt, errResult := p.RequireString("prompt")
		if errResult != nil {
			return errResult, nil
		}
		if err := ValidateStringLength(prompt, MaxPromptLength, "prompt"); err != nil {
			return codedError(ErrInvalidParams, err.Error()), nil
		}

		cronExpr, errResult := p.RequireString("cron_expression")
		if errResult != nil {
			return errResult, nil
		}

		agentType, errResult := p.OptionalEnum("agent_type", validAgentTypes, "ralph")
		if errResult != nil {
			return errResult, nil
		}

		enabled := p.OptionalBool("enabled", true)

		entry := ScheduleEntry{
			ID:             fmt.Sprintf("sched-%d", time.Now().UnixNano()),
			Prompt:         prompt,
			CronExpression: cronExpr,
			AgentType:      agentType,
			Enabled:        enabled,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		entries, err := store.load()
		if err != nil {
			// File may not exist yet — start fresh.
			entries = nil
		}
		entries = append(entries, entry)
		if err := store.save(entries); err != nil {
			return codedError(ErrInternal, fmt.Sprintf("save schedules: %v", err)), nil
		}

		return jsonResult(map[string]any{
			"schedule_id":     entry.ID,
			"prompt":          entry.Prompt,
			"cron_expression": entry.CronExpression,
			"agent_type":      entry.AgentType,
			"enabled":         entry.Enabled,
			"created_at":      entry.CreatedAt,
		}), nil
	}
}

// schedulesPath returns the path to the schedules JSON file.
// It is a variable so tests can override it.
var schedulesPath = func() string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return filepath.Join(home, ".ralph", "schedules.json")
	}
	return filepath.Join(appdir.StateDir("ralph"), "schedules.json")
}

func (ss *scheduleStore) load() ([]ScheduleEntry, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	data, err := os.ReadFile(ss.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entries []ScheduleEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse schedules: %w", err)
	}
	return entries, nil
}

func (ss *scheduleStore) save(entries []ScheduleEntry) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	dir := filepath.Dir(ss.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create schedule dir: %w", err)
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal schedules: %w", err)
	}

	// Atomic write: write to temp file then rename.
	tmp := ss.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, ss.path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
