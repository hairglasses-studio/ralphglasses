package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

const (
	subscriptionPolicyFile = "subscription_policy.json"
	usageWindowFile        = "usage_window.json"
	automationQueueFile    = "automation_queue.json"
)

// SubscriptionPolicy controls repo-local subscription-window automation.
type SubscriptionPolicy struct {
	Enabled               bool     `json:"enabled"`
	Provider              Provider `json:"provider"`
	Timezone              string   `json:"timezone,omitempty"`
	ResetCron             string   `json:"reset_cron,omitempty"`
	ResetAnchor           string   `json:"reset_anchor,omitempty"`
	ResetWindowHours      int      `json:"reset_window_hours,omitempty"`
	WindowBudgetUSD       float64  `json:"window_budget_usd,omitempty"`
	TargetUtilizationPct  float64  `json:"target_utilization_pct,omitempty"`
	ResumeBackoffMinutes  int      `json:"resume_backoff_minutes,omitempty"`
	DefaultModel          string   `json:"default_model,omitempty"`
	DefaultTaskBudgetUSD  float64  `json:"default_task_budget_usd,omitempty"`
	DefaultTaskMaxTurns   int      `json:"default_task_max_turns,omitempty"`
	MaxConcurrentSessions int      `json:"max_concurrent_sessions,omitempty"`
}

// SubscriptionPolicyRecommendation suggests pacing changes from recent window behavior.
type SubscriptionPolicyRecommendation struct {
	CurrentWindowBudgetUSD     float64 `json:"current_window_budget_usd"`
	RecommendedWindowBudgetUSD float64 `json:"recommended_window_budget_usd"`
	RecommendedTargetPct       float64 `json:"recommended_target_utilization_pct"`
	Reason                     string  `json:"reason"`
}

// AutomationQueueItem is a durable queued task for subscription-window automation.
type AutomationQueueItem struct {
	ID           string     `json:"id"`
	Prompt       string     `json:"prompt"`
	Provider     Provider   `json:"provider"`
	Model        string     `json:"model,omitempty"`
	BudgetUSD    float64    `json:"budget_usd,omitempty"`
	MaxTurns     int        `json:"max_turns,omitempty"`
	Priority     int        `json:"priority"`
	Source       string     `json:"source,omitempty"`
	ScheduleID   string     `json:"schedule_id,omitempty"`
	ScheduledFor *time.Time `json:"scheduled_for,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// ParkedSessionRef tracks a session that exhausted subscription capacity and must be resumed later.
type ParkedSessionRef struct {
	RalphSessionID        string     `json:"ralph_session_id,omitempty"`
	ProviderSessionID     string     `json:"provider_session_id,omitempty"`
	Provider              Provider   `json:"provider"`
	Prompt                string     `json:"prompt"`
	Model                 string     `json:"model,omitempty"`
	BudgetUSD             float64    `json:"budget_usd,omitempty"`
	MaxTurns              int        `json:"max_turns,omitempty"`
	ParkedAt              time.Time  `json:"parked_at"`
	ExhaustedReason       string     `json:"exhausted_reason,omitempty"`
	QueueItemID           string     `json:"queue_item_id,omitempty"`
	ScheduleID            string     `json:"schedule_id,omitempty"`
	EstimatedRemainingUSD float64    `json:"estimated_remaining_usd,omitempty"`
	NextAttemptAfter      *time.Time `json:"next_attempt_after,omitempty"`
}

// UsageWindowState is the durable state for the current subscription window.
type UsageWindowState struct {
	WindowStart           time.Time            `json:"window_start,omitempty"`
	WindowEnd             time.Time            `json:"window_end,omitempty"`
	NextReset             time.Time            `json:"next_reset,omitempty"`
	CurrentSpendUSD       float64              `json:"current_spend_usd,omitempty"`
	ActiveSessionID       string               `json:"active_session_id,omitempty"`
	ActiveQueueItemID     string               `json:"active_queue_item_id,omitempty"`
	ActiveQueueItem       *AutomationQueueItem `json:"active_queue_item,omitempty"`
	Exhausted             bool                 `json:"exhausted"`
	ExhaustedAt           *time.Time           `json:"exhausted_at,omitempty"`
	ExhaustedReason       string               `json:"exhausted_reason,omitempty"`
	ParkedSession         *ParkedSessionRef    `json:"parked_session,omitempty"`
	ScheduleLastEnqueued  map[string]string    `json:"schedule_last_enqueued,omitempty"`
	LastDispatchAt        *time.Time           `json:"last_dispatch_at,omitempty"`
	LastTerminalSessionAt *time.Time           `json:"last_terminal_session_at,omitempty"`
	LastProjectedSpendUSD float64              `json:"last_projected_spend_usd,omitempty"`
	LastError             string               `json:"last_error,omitempty"`
}

// AutomationStatusSnapshot is the external status view surfaced via MCP and status.json.
type AutomationStatusSnapshot struct {
	Enabled               bool      `json:"enabled"`
	RepoPath              string    `json:"repo_path"`
	Provider              Provider  `json:"provider"`
	WindowStatus          string    `json:"window_status"`
	WindowStart           time.Time `json:"window_start,omitempty"`
	WindowEnd             time.Time `json:"window_end,omitempty"`
	NextReset             time.Time `json:"next_reset,omitempty"`
	QueueDepth            int       `json:"queue_depth"`
	ActiveSessionID       string    `json:"active_session_id,omitempty"`
	ParkedSessionID       string    `json:"parked_session_id,omitempty"`
	ParkedProviderSession string    `json:"parked_provider_session_id,omitempty"`
	CurrentSpendUSD       float64   `json:"current_spend_usd,omitempty"`
	WindowBudgetUSD       float64   `json:"window_budget_usd,omitempty"`
	TargetUtilizationPct  float64   `json:"target_utilization_pct,omitempty"`
	ProjectedSpendAtReset float64   `json:"projected_spend_at_reset,omitempty"`
	LastExhaustionReason  string    `json:"last_exhaustion_reason,omitempty"`
	LastError             string    `json:"last_error,omitempty"`
}

// SubscriptionAutomationController owns repo-local queueing, pacing, park/resume, and status.
type SubscriptionAutomationController struct {
	mu       sync.Mutex
	mgr      *Manager
	repoPath string
	now      func() time.Time

	policy SubscriptionPolicy
	state  UsageWindowState
	queue  []AutomationQueueItem
}

type automationAction struct {
	kind      string
	queueItem *AutomationQueueItem
	parked    *ParkedSessionRef
}

type cycleSchedule struct {
	ScheduleID  string `json:"schedule_id"`
	CronExpr    string `json:"cron_expr"`
	CycleConfig string `json:"cycle_config"`
	CreatedAt   string `json:"created_at"`
}

type quotaSignal struct {
	Exhausted bool
	Reason    string
	ResetAt   *time.Time
}

func DefaultSubscriptionPolicy() SubscriptionPolicy {
	return normalizeSubscriptionPolicy(SubscriptionPolicy{
		Provider:              ProviderCodex,
		Timezone:              "America/Los_Angeles",
		ResetWindowHours:      24,
		WindowBudgetUSD:       20,
		TargetUtilizationPct:  95,
		ResumeBackoffMinutes:  5,
		DefaultModel:          ProviderDefaults(ProviderCodex),
		DefaultTaskBudgetUSD:  5,
		DefaultTaskMaxTurns:   0,
		MaxConcurrentSessions: 1,
	})
}

func normalizeSubscriptionPolicy(policy SubscriptionPolicy) SubscriptionPolicy {
	if policy.Provider == "" {
		policy.Provider = ProviderCodex
	}
	if policy.Timezone == "" {
		policy.Timezone = "America/Los_Angeles"
	}
	if policy.ResetWindowHours <= 0 {
		policy.ResetWindowHours = 24
	}
	if policy.WindowBudgetUSD <= 0 {
		policy.WindowBudgetUSD = 20
	}
	if policy.TargetUtilizationPct <= 0 || policy.TargetUtilizationPct > 100 {
		policy.TargetUtilizationPct = 95
	}
	if policy.ResumeBackoffMinutes <= 0 {
		policy.ResumeBackoffMinutes = 5
	}
	if policy.DefaultModel == "" {
		policy.DefaultModel = ProviderDefaults(policy.Provider)
	}
	if policy.DefaultTaskBudgetUSD < 0 {
		policy.DefaultTaskBudgetUSD = 0
	}
	if policy.MaxConcurrentSessions <= 0 {
		policy.MaxConcurrentSessions = 1
	}
	return policy
}

func NewSubscriptionAutomationController(mgr *Manager, repoPath string) *SubscriptionAutomationController {
	c := &SubscriptionAutomationController{
		mgr:      mgr,
		repoPath: repoPath,
		now:      time.Now,
		policy:   DefaultSubscriptionPolicy(),
		state: UsageWindowState{
			ScheduleLastEnqueued: make(map[string]string),
		},
	}
	c.load()
	return c
}

func (c *SubscriptionAutomationController) Policy() SubscriptionPolicy {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.policy
}

func (c *SubscriptionAutomationController) SetPolicy(policy SubscriptionPolicy) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := validateSubscriptionPolicy(policy); err != nil {
		return err
	}
	c.policy = normalizeSubscriptionPolicy(policy)
	c.ensureWindowLocked(c.now())
	return c.persistLocked()
}

func (c *SubscriptionAutomationController) RecommendPolicy() SubscriptionPolicyRecommendation {
	c.mu.Lock()
	defer c.mu.Unlock()
	current := c.policy.WindowBudgetUSD
	target := c.policy.TargetUtilizationPct
	recommended := current
	reason := "window has no exhaustion or underutilization signal yet"

	switch {
	case c.state.ExhaustedAt != nil && !c.state.NextReset.IsZero():
		windowDuration := c.state.WindowEnd.Sub(c.state.WindowStart)
		remaining := c.state.NextReset.Sub(*c.state.ExhaustedAt)
		if windowDuration > 0 && remaining > windowDuration/10 {
			recommended = roundUSD(current * 0.85)
			reason = "hard exhaustion happened well before reset; lower synthetic budget to spread work across the window"
		}
	case !c.state.NextReset.IsZero() && c.state.CurrentSpendUSD < current*0.6:
		recommended = roundUSD(current * 1.15)
		reason = "window is running cold relative to target; raise synthetic budget to consume more premium usage before reset"
	}

	return SubscriptionPolicyRecommendation{
		CurrentWindowBudgetUSD:     current,
		RecommendedWindowBudgetUSD: recommended,
		RecommendedTargetPct:       target,
		Reason:                     reason,
	}
}

func (c *SubscriptionAutomationController) ListQueue() []AutomationQueueItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]AutomationQueueItem, len(c.queue))
	copy(out, c.queue)
	return out
}

func (c *SubscriptionAutomationController) Enqueue(item AutomationQueueItem) (AutomationQueueItem, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item = c.normalizeQueueItemLocked(item)
	c.queue = append(c.queue, item)
	c.sortQueueLocked()
	return item, c.persistLocked()
}

func (c *SubscriptionAutomationController) RemoveQueueItem(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.queue {
		if c.queue[i].ID != id {
			continue
		}
		c.queue = append(c.queue[:i], c.queue[i+1:]...)
		_ = c.persistLocked()
		return true
	}
	return false
}

func (c *SubscriptionAutomationController) ReprioritizeQueueItem(id string, priority int) (*AutomationQueueItem, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.queue {
		if c.queue[i].ID != id {
			continue
		}
		c.queue[i].Priority = priority
		c.sortQueueLocked()
		if err := c.persistLocked(); err != nil {
			return nil, err
		}
		return c.lookupQueueItemByIDLocked(id), nil
	}
	return nil, fmt.Errorf("queue item not found: %s", id)
}

func (c *SubscriptionAutomationController) Status() AutomationStatusSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotLocked(c.now())
}

func (c *SubscriptionAutomationController) Tick(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := c.now()

	c.mu.Lock()
	c.ensureWindowLocked(now)
	c.handleActiveSessionLocked(now)
	c.enqueueDueSchedulesLocked(now)
	action := c.nextActionLocked(now)
	snapshot := c.snapshotLocked(now)
	_ = c.persistLocked()
	_ = c.writeStatusFileLocked(snapshot)
	c.mu.Unlock()

	if action == nil {
		return
	}

	switch action.kind {
	case "resume":
		c.executeResume(ctx, action.parked)
	case "launch":
		c.executeLaunch(ctx, action.queueItem)
	}
}

func (c *SubscriptionAutomationController) executeResume(ctx context.Context, parked *ParkedSessionRef) {
	if parked == nil || parked.ProviderSessionID == "" {
		return
	}
	sess, err := c.mgr.Resume(ctx, c.repoPath, parked.Provider, parked.ProviderSessionID, "")
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if err != nil {
		next := now.Add(time.Duration(c.policy.ResumeBackoffMinutes) * time.Minute)
		if c.state.ParkedSession != nil {
			c.state.ParkedSession.NextAttemptAfter = &next
		}
		c.state.LastError = fmt.Sprintf("resume failed: %v", err)
		_ = c.persistLocked()
		_ = c.writeStatusFileLocked(c.snapshotLocked(now))
		return
	}

	c.state.ActiveSessionID = sess.ID
	c.state.ActiveQueueItemID = parked.QueueItemID
	c.state.ActiveQueueItem = &AutomationQueueItem{
		ID:         parked.QueueItemID,
		Prompt:     parked.Prompt,
		Provider:   parked.Provider,
		Model:      parked.Model,
		BudgetUSD:  parked.BudgetUSD,
		MaxTurns:   parked.MaxTurns,
		Source:     "resume",
		ScheduleID: parked.ScheduleID,
		CreatedAt:  parked.ParkedAt,
	}
	c.state.ParkedSession = nil
	c.state.LastError = ""
	c.state.LastDispatchAt = ptrTime(now)
	_ = c.persistLocked()
	_ = c.writeStatusFileLocked(c.snapshotLocked(now))
}

func (c *SubscriptionAutomationController) executeLaunch(ctx context.Context, item *AutomationQueueItem) {
	if item == nil {
		return
	}

	opts := LaunchOptions{
		Provider:     item.Provider,
		RepoPath:     c.repoPath,
		Prompt:       item.Prompt,
		Model:        item.Model,
		MaxBudgetUSD: item.BudgetUSD,
		MaxTurns:     item.MaxTurns,
	}
	sess, err := c.mgr.Launch(ctx, opts)

	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if err != nil {
		c.queue = append(c.queue, *item)
		c.sortQueueLocked()
		c.state.LastError = fmt.Sprintf("launch failed: %v", err)
		_ = c.persistLocked()
		_ = c.writeStatusFileLocked(c.snapshotLocked(now))
		return
	}

	c.state.ActiveSessionID = sess.ID
	c.state.ActiveQueueItemID = item.ID
	activeItem := *item
	c.state.ActiveQueueItem = &activeItem
	c.state.LastDispatchAt = ptrTime(now)
	c.state.LastError = ""
	_ = c.persistLocked()
	_ = c.writeStatusFileLocked(c.snapshotLocked(now))
}

func (c *SubscriptionAutomationController) handleActiveSessionLocked(now time.Time) {
	if c.state.ActiveSessionID == "" {
		return
	}

	sess, ok := c.mgr.Get(c.state.ActiveSessionID)
	if !ok {
		c.state.ActiveSessionID = ""
		c.state.ActiveQueueItemID = ""
		c.state.ActiveQueueItem = nil
		return
	}

	sess.Lock()
	status := sess.Status
	provider := sess.Provider
	providerSessionID := sess.ProviderSessionID
	ralphSessionID := sess.ID
	prompt := sess.Prompt
	modelName := sess.Model
	budgetUSD := sess.BudgetUSD
	maxTurns := sess.MaxTurns
	spentUSD := sess.SpentUSD
	lastOutput := sess.LastOutput
	errMsg := sess.Error
	outputHistory := append([]string(nil), sess.OutputHistory...)
	sess.Unlock()

	if !status.IsTerminal() {
		return
	}

	c.state.CurrentSpendUSD += spentUSD
	c.state.LastTerminalSessionAt = ptrTime(now)
	c.state.ActiveSessionID = ""
	activeQueueItemID := c.state.ActiveQueueItemID
	activeQueueItem := c.state.ActiveQueueItem
	c.state.ActiveQueueItemID = ""
	c.state.ActiveQueueItem = nil

	signal := detectQuotaSignal(provider, outputHistory, lastOutput, errMsg, now, c.policyLocation())
	if signal.Exhausted {
		c.state.Exhausted = true
		c.state.ExhaustedReason = signal.Reason
		c.state.ExhaustedAt = ptrTime(now)
		if signal.ResetAt != nil {
			c.state.NextReset = signal.ResetAt.In(c.policyLocation())
		}
		estimate := c.estimateQueueItemCostLocked(activeQueueItem)
		if estimate <= 0 {
			estimate = c.estimateQueueItemCostLocked(&AutomationQueueItem{
				BudgetUSD: budgetUSD,
				Provider:  provider,
				Prompt:    prompt,
				Model:     modelName,
				MaxTurns:  maxTurns,
			})
		}
		scheduleID := ""
		if activeQueueItem != nil {
			scheduleID = activeQueueItem.ScheduleID
		}
		c.state.ParkedSession = &ParkedSessionRef{
			RalphSessionID:        ralphSessionID,
			ProviderSessionID:     providerSessionID,
			Provider:              provider,
			Prompt:                prompt,
			Model:                 modelName,
			BudgetUSD:             budgetUSD,
			MaxTurns:              maxTurns,
			ParkedAt:              now,
			ExhaustedReason:       signal.Reason,
			QueueItemID:           activeQueueItemID,
			ScheduleID:            scheduleID,
			EstimatedRemainingUSD: estimate,
		}
	}
}

func (c *SubscriptionAutomationController) nextActionLocked(now time.Time) *automationAction {
	if !c.policy.Enabled {
		return nil
	}
	if c.policy.Provider != ProviderCodex {
		return nil
	}
	if c.state.ActiveSessionID != "" {
		return nil
	}
	if c.mgr.IsRunning(c.repoPath) {
		return nil
	}

	if c.state.ParkedSession != nil {
		if c.state.Exhausted && !c.state.NextReset.IsZero() && now.Before(c.state.NextReset) {
			return nil
		}
		if c.state.ParkedSession.NextAttemptAfter != nil && now.Before(*c.state.ParkedSession.NextAttemptAfter) {
			return nil
		}
		return &automationAction{kind: "resume", parked: c.state.ParkedSession}
	}

	if c.state.Exhausted && !c.state.NextReset.IsZero() && now.Before(c.state.NextReset) {
		return nil
	}
	if len(c.queue) == 0 {
		return nil
	}

	nextItem := c.queue[0]
	targetBudget := c.policy.WindowBudgetUSD * (c.policy.TargetUtilizationPct / 100.0)
	if targetBudget > 0 && c.state.CurrentSpendUSD+c.estimateQueueItemCostLocked(&nextItem) > targetBudget {
		return nil
	}

	c.queue = c.queue[1:]
	return &automationAction{kind: "launch", queueItem: &nextItem}
}

func (c *SubscriptionAutomationController) ensureWindowLocked(now time.Time) {
	if !c.policy.Enabled {
		return
	}
	start, end, next, err := computeAutomationWindowBounds(c.policy, now)
	if err != nil {
		c.state.LastError = err.Error()
		return
	}
	if c.state.ScheduleLastEnqueued == nil {
		c.state.ScheduleLastEnqueued = make(map[string]string)
	}

	resetPassed := c.state.Exhausted && !c.state.NextReset.IsZero() && !now.Before(c.state.NextReset)
	if resetPassed {
		if c.state.NextReset.After(start) && c.state.NextReset.Before(end) {
			start = c.state.NextReset
		}
		c.state.CurrentSpendUSD = 0
		c.state.Exhausted = false
		c.state.ExhaustedAt = nil
		c.state.ExhaustedReason = ""
	}

	effectiveNext := next
	if c.state.Exhausted && !c.state.NextReset.IsZero() && c.state.NextReset.Before(next) && now.Before(c.state.NextReset) {
		effectiveNext = c.state.NextReset
		end = effectiveNext
	}
	if c.state.WindowStart.Equal(start) && c.state.WindowEnd.Equal(end) && c.state.NextReset.Equal(effectiveNext) {
		return
	}
	windowChanged := !c.state.WindowEnd.IsZero() && (now.Equal(c.state.WindowEnd) || now.After(c.state.WindowEnd) || c.state.WindowStart.After(now))
	c.state.WindowStart = start
	c.state.WindowEnd = end
	c.state.NextReset = effectiveNext
	if windowChanged {
		c.state.CurrentSpendUSD = 0
		c.state.Exhausted = false
		c.state.ExhaustedAt = nil
		c.state.ExhaustedReason = ""
	}
}

func (c *SubscriptionAutomationController) enqueueDueSchedulesLocked(now time.Time) {
	schedules, err := c.loadSchedulesLocked()
	if err != nil {
		c.state.LastError = err.Error()
		return
	}
	for _, sched := range schedules {
		dueRuns, err := c.dueScheduleRunsLocked(sched, now)
		if err != nil {
			c.state.LastError = err.Error()
			continue
		}
		for _, runAt := range dueRuns {
			item := c.queueItemFromScheduleLocked(sched, runAt)
			if c.hasScheduledQueueItemLocked(item.ScheduleID, runAt) {
				continue
			}
			c.queue = append(c.queue, item)
			c.state.ScheduleLastEnqueued[sched.ScheduleID] = runAt.Format(time.RFC3339)
		}
	}
	c.sortQueueLocked()
}

func (c *SubscriptionAutomationController) queueItemFromScheduleLocked(s cycleSchedule, scheduledFor time.Time) AutomationQueueItem {
	item := AutomationQueueItem{
		ID:           "aq-" + uuid.NewString(),
		Provider:     c.policy.Provider,
		Model:        c.policy.DefaultModel,
		BudgetUSD:    c.policy.DefaultTaskBudgetUSD,
		MaxTurns:     c.policy.DefaultTaskMaxTurns,
		Priority:     5,
		Source:       "schedule",
		ScheduleID:   s.ScheduleID,
		ScheduledFor: ptrTime(scheduledFor),
		CreatedAt:    c.now(),
	}

	if err := json.Unmarshal([]byte(s.CycleConfig), &map[string]any{}); err == nil {
		var raw map[string]any
		_ = json.Unmarshal([]byte(s.CycleConfig), &raw)
		item.Prompt = strings.TrimSpace(stringValue(raw["prompt"]))
		if item.Prompt == "" {
			item.Prompt = fmt.Sprintf("Execute the scheduled R&D cycle for %s using this configuration:\n%s", filepath.Base(c.repoPath), s.CycleConfig)
		}
		if p := Provider(strings.TrimSpace(strings.ToLower(stringValue(raw["provider"])))); p != "" {
			item.Provider = p
		}
		if v := strings.TrimSpace(stringValue(raw["model"])); v != "" {
			item.Model = v
		}
		if v := floatValue(raw["budget_usd"]); v > 0 {
			item.BudgetUSD = v
		} else if v := floatValue(raw["budget"]); v > 0 {
			item.BudgetUSD = v
		}
		if v := intValue(raw["max_turns"]); v > 0 {
			item.MaxTurns = v
		}
		if v := intValue(raw["priority"]); v != 0 {
			item.Priority = v
		}
	} else {
		item.Prompt = strings.TrimSpace(s.CycleConfig)
		if item.Prompt == "" {
			item.Prompt = fmt.Sprintf("Run the scheduled autonomous coding cycle for %s.", filepath.Base(c.repoPath))
		}
	}

	item = c.normalizeQueueItemLocked(item)
	return item
}

func (c *SubscriptionAutomationController) dueScheduleRunsLocked(s cycleSchedule, now time.Time) ([]time.Time, error) {
	fields, err := parseAutomationCron(s.CronExpr)
	if err != nil {
		return nil, fmt.Errorf("schedule %s: %w", s.ScheduleID, err)
	}

	loc := c.policyLocation()
	createdAt := now.In(loc)
	if parsed, err := time.Parse(time.RFC3339, s.CreatedAt); err == nil {
		createdAt = parsed.In(loc)
	}
	from := createdAt
	if lastRaw := c.state.ScheduleLastEnqueued[s.ScheduleID]; lastRaw != "" {
		if parsed, err := time.Parse(time.RFC3339, lastRaw); err == nil {
			from = parsed.In(loc)
		}
	}

	var due []time.Time
	cursor := from
	for i := 0; i < 5; i++ {
		next := nextAutomationCronMatch(fields, cursor.In(loc))
		if next.IsZero() || next.After(now.In(loc)) {
			break
		}
		due = append(due, next)
		cursor = next
	}
	return due, nil
}

func (c *SubscriptionAutomationController) loadSchedulesLocked() ([]cycleSchedule, error) {
	dir := filepath.Join(c.repoPath, ".ralph", "schedules")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var schedules []cycleSchedule
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var sched cycleSchedule
		if err := json.Unmarshal(data, &sched); err != nil {
			return nil, err
		}
		if sched.ScheduleID == "" || strings.TrimSpace(sched.CronExpr) == "" {
			continue
		}
		schedules = append(schedules, sched)
	}
	return schedules, nil
}

func (c *SubscriptionAutomationController) hasScheduledQueueItemLocked(scheduleID string, scheduledFor time.Time) bool {
	match := scheduledFor.Format(time.RFC3339)
	for _, item := range c.queue {
		if item.ScheduleID != scheduleID || item.ScheduledFor == nil {
			continue
		}
		if item.ScheduledFor.Format(time.RFC3339) == match {
			return true
		}
	}
	if c.state.ParkedSession != nil && c.state.ParkedSession.ScheduleID == scheduleID {
		return true
	}
	return false
}

func (c *SubscriptionAutomationController) lookupQueueItemByIDLocked(id string) *AutomationQueueItem {
	if id == "" {
		return nil
	}
	for i := range c.queue {
		if c.queue[i].ID == id {
			item := c.queue[i]
			return &item
		}
	}
	return nil
}

func (c *SubscriptionAutomationController) normalizeQueueItemLocked(item AutomationQueueItem) AutomationQueueItem {
	item.Provider = firstNonEmptyProvider(item.Provider, c.policy.Provider, ProviderCodex)
	if item.Model == "" {
		item.Model = c.policy.DefaultModel
	}
	if item.BudgetUSD <= 0 {
		item.BudgetUSD = c.policy.DefaultTaskBudgetUSD
	}
	if item.MaxTurns <= 0 {
		item.MaxTurns = c.policy.DefaultTaskMaxTurns
	}
	if item.Priority == 0 {
		item.Priority = 5
	}
	if item.ID == "" {
		item.ID = "aq-" + uuid.NewString()
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = c.now()
	}
	return item
}

func (c *SubscriptionAutomationController) sortQueueLocked() {
	sort.SliceStable(c.queue, func(i, j int) bool {
		if c.queue[i].Priority != c.queue[j].Priority {
			return c.queue[i].Priority > c.queue[j].Priority
		}
		return c.queue[i].CreatedAt.Before(c.queue[j].CreatedAt)
	})
}

func (c *SubscriptionAutomationController) estimateQueueItemCostLocked(item *AutomationQueueItem) float64 {
	if item == nil {
		return 0
	}
	if item.BudgetUSD > 0 {
		return item.BudgetUSD
	}
	if cp := c.mgr.GetCostPredictor(); cp != nil {
		estimate := cp.Predict(ClassifyTask(item.Prompt), string(item.Provider))
		if estimate > 0 {
			return estimate
		}
	}
	return c.policy.DefaultTaskBudgetUSD
}

func (c *SubscriptionAutomationController) snapshotLocked(now time.Time) AutomationStatusSnapshot {
	status := "disabled"
	switch {
	case c.policy.Enabled && c.state.ParkedSession != nil:
		status = "parked"
	case c.policy.Enabled && c.state.ActiveSessionID != "":
		status = "running"
	case c.policy.Enabled && len(c.queue) > 0:
		status = "queued"
	case c.policy.Enabled:
		status = "idle"
	}

	projected := c.state.CurrentSpendUSD
	if c.state.ParkedSession != nil {
		projected += c.state.ParkedSession.EstimatedRemainingUSD
	} else if len(c.queue) > 0 {
		projected += c.estimateQueueItemCostLocked(&c.queue[0])
	}
	c.state.LastProjectedSpendUSD = projected

	snapshot := AutomationStatusSnapshot{
		Enabled:               c.policy.Enabled,
		RepoPath:              c.repoPath,
		Provider:              c.policy.Provider,
		WindowStatus:          status,
		WindowStart:           c.state.WindowStart,
		WindowEnd:             c.state.WindowEnd,
		NextReset:             c.state.NextReset,
		QueueDepth:            len(c.queue),
		ActiveSessionID:       c.state.ActiveSessionID,
		CurrentSpendUSD:       roundUSD(c.state.CurrentSpendUSD),
		WindowBudgetUSD:       c.policy.WindowBudgetUSD,
		TargetUtilizationPct:  c.policy.TargetUtilizationPct,
		ProjectedSpendAtReset: roundUSD(projected),
		LastExhaustionReason:  c.state.ExhaustedReason,
		LastError:             c.state.LastError,
	}
	if c.state.ParkedSession != nil {
		snapshot.ParkedSessionID = firstNonEmpty(c.state.ParkedSession.RalphSessionID, c.state.ParkedSession.ProviderSessionID)
		snapshot.ParkedProviderSession = c.state.ParkedSession.ProviderSessionID
	}
	return snapshot
}

func (c *SubscriptionAutomationController) persistLocked() error {
	if err := os.MkdirAll(filepath.Join(c.repoPath, ".ralph"), 0o755); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(c.repoPath, ".ralph", subscriptionPolicyFile), c.policy); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(c.repoPath, ".ralph", usageWindowFile), c.state); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(c.repoPath, ".ralph", automationQueueFile), c.queue)
}

func (c *SubscriptionAutomationController) load() {
	c.policy = DefaultSubscriptionPolicy()
	c.state = UsageWindowState{ScheduleLastEnqueued: make(map[string]string)}

	if data, err := os.ReadFile(filepath.Join(c.repoPath, ".ralph", subscriptionPolicyFile)); err == nil {
		_ = json.Unmarshal(data, &c.policy)
		c.policy = normalizeSubscriptionPolicy(c.policy)
	}
	if data, err := os.ReadFile(filepath.Join(c.repoPath, ".ralph", usageWindowFile)); err == nil {
		_ = json.Unmarshal(data, &c.state)
		if c.state.ScheduleLastEnqueued == nil {
			c.state.ScheduleLastEnqueued = make(map[string]string)
		}
	}
	if data, err := os.ReadFile(filepath.Join(c.repoPath, ".ralph", automationQueueFile)); err == nil {
		_ = json.Unmarshal(data, &c.queue)
	}
	c.sortQueueLocked()
}

func (c *SubscriptionAutomationController) writeStatusFileLocked(snapshot AutomationStatusSnapshot) error {
	ralphDir := filepath.Join(c.repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0o755); err != nil {
		return err
	}

	var status model.LoopStatus
	if data, err := os.ReadFile(filepath.Join(ralphDir, "status.json")); err == nil {
		_ = json.Unmarshal(data, &status)
	}
	if status.Status == "" || status.Status == status.WindowStatus {
		status.Status = snapshot.WindowStatus
	}
	status.Timestamp = c.now().UTC()
	status.NextReset = ""
	if !snapshot.NextReset.IsZero() {
		status.NextReset = snapshot.NextReset.Format(time.RFC3339)
	}
	status.SessionSpendUSD = snapshot.CurrentSpendUSD
	status.WindowStatus = snapshot.WindowStatus
	status.ParkedSessionID = snapshot.ParkedSessionID
	status.QueueDepth = snapshot.QueueDepth
	status.TargetUtilizationPct = snapshot.TargetUtilizationPct
	status.ProjectedSpendAtReset = snapshot.ProjectedSpendAtReset
	if status.ExitReason == "" && snapshot.LastExhaustionReason != "" {
		status.ExitReason = snapshot.LastExhaustionReason
	}

	return writeJSONAtomic(filepath.Join(ralphDir, "status.json"), status)
}

func validateSubscriptionPolicy(policy SubscriptionPolicy) error {
	policy = normalizeSubscriptionPolicy(policy)
	if policy.ResetCron == "" && policy.ResetAnchor == "" {
		return fmt.Errorf("subscription automation requires reset_cron or reset_anchor")
	}
	if policy.ResetCron != "" {
		if _, err := parseAutomationCron(policy.ResetCron); err != nil {
			return err
		}
	}
	if policy.ResetAnchor != "" {
		if _, err := time.Parse(time.RFC3339, policy.ResetAnchor); err != nil {
			return fmt.Errorf("reset_anchor must be RFC3339: %w", err)
		}
	}
	if _, err := time.LoadLocation(policy.Timezone); err != nil {
		return fmt.Errorf("invalid timezone %q: %w", policy.Timezone, err)
	}
	return nil
}

func (c *SubscriptionAutomationController) policyLocation() *time.Location {
	loc, err := time.LoadLocation(c.policy.Timezone)
	if err != nil {
		return time.Local
	}
	return loc
}

func computeAutomationWindowBounds(policy SubscriptionPolicy, now time.Time) (time.Time, time.Time, time.Time, error) {
	policy = normalizeSubscriptionPolicy(policy)
	loc, err := time.LoadLocation(policy.Timezone)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, err
	}
	now = now.In(loc)

	if policy.ResetCron != "" {
		fields, err := parseAutomationCron(policy.ResetCron)
		if err != nil {
			return time.Time{}, time.Time{}, time.Time{}, err
		}
		prev := prevAutomationCronMatch(fields, now)
		next := nextAutomationCronMatch(fields, now)
		if prev.IsZero() || next.IsZero() {
			return time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("could not resolve reset window for cron %q", policy.ResetCron)
		}
		return prev, next, next, nil
	}

	duration := time.Duration(policy.ResetWindowHours) * time.Hour
	if duration <= 0 {
		duration = 24 * time.Hour
	}
	anchor := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	if policy.ResetAnchor != "" {
		parsed, err := time.Parse(time.RFC3339, policy.ResetAnchor)
		if err != nil {
			return time.Time{}, time.Time{}, time.Time{}, err
		}
		anchor = parsed.In(loc)
	}

	for anchor.After(now) {
		anchor = anchor.Add(-duration)
	}
	elapsed := now.Sub(anchor)
	if elapsed < 0 {
		elapsed = 0
	}
	periods := int64(elapsed / duration)
	start := anchor.Add(time.Duration(periods) * duration)
	end := start.Add(duration)
	return start, end, end, nil
}

type automationCronField struct {
	Any  bool
	Step int
	Val  int
}

func parseAutomationCron(expr string) ([]automationCronField, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron must have exactly 5 fields")
	}
	out := make([]automationCronField, len(fields))
	for i, raw := range fields {
		switch {
		case raw == "*":
			out[i] = automationCronField{Any: true}
		case strings.HasPrefix(raw, "*/"):
			n, err := strconv.Atoi(strings.TrimPrefix(raw, "*/"))
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("invalid cron step %q", raw)
			}
			out[i] = automationCronField{Step: n}
		default:
			n, err := strconv.Atoi(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid cron field %q", raw)
			}
			out[i] = automationCronField{Val: n}
		}
	}
	return out, nil
}

func nextAutomationCronMatch(fields []automationCronField, from time.Time) time.Time {
	t := from.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 525600; i++ {
		if automationCronMatches(fields, t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
}

func prevAutomationCronMatch(fields []automationCronField, from time.Time) time.Time {
	t := from.Truncate(time.Minute)
	for i := 0; i < 525600; i++ {
		if automationCronMatches(fields, t) && !t.After(from) {
			return t
		}
		t = t.Add(-time.Minute)
	}
	return time.Time{}
}

func automationCronMatches(fields []automationCronField, t time.Time) bool {
	values := []int{t.Minute(), t.Hour(), t.Day(), int(t.Month()), int(t.Weekday())}
	for i := range fields {
		if !automationCronFieldMatches(fields[i], values[i]) {
			return false
		}
	}
	return true
}

func automationCronFieldMatches(field automationCronField, value int) bool {
	switch {
	case field.Any:
		return true
	case field.Step > 0:
		return value%field.Step == 0
	default:
		return value == field.Val
	}
}

func detectQuotaSignal(provider Provider, history []string, lastOutput, errMsg string, now time.Time, loc *time.Location) quotaSignal {
	switch provider {
	case ProviderClaude:
		session := &Session{OutputHistory: history, LastOutput: lastOutput, Error: errMsg}
		if isExtraUsageExhausted(session) {
			return quotaSignal{Exhausted: true, Reason: "extra_usage_exhausted"}
		}
		return quotaSignal{}
	case ProviderCodex:
		combined := strings.ToLower(strings.Join(append(append([]string{}, history...), lastOutput, errMsg), "\n"))
		hardPatterns := []*regexp.Regexp{
			regexp.MustCompile(`\b(usage|quota)\b.*\b(exhausted|exceeded|reached)\b`),
			regexp.MustCompile(`\b(premium|subscription|pro)\b.*\b(usage|quota)\b.*\b(exhausted|reached|exceeded)\b`),
		}
		matched := false
		for _, re := range hardPatterns {
			if re.MatchString(combined) {
				matched = true
				break
			}
		}
		if !matched {
			return quotaSignal{}
		}
		signal := quotaSignal{Exhausted: true, Reason: "subscription_usage_exhausted"}
		if resetAt := extractResetHint(combined, now.In(loc), loc); resetAt != nil {
			signal.ResetAt = resetAt
		}
		return signal
	default:
		return quotaSignal{}
	}
}

func extractResetHint(text string, now time.Time, loc *time.Location) *time.Time {
	if loc == nil {
		loc = time.Local
	}
	rfc3339Re := regexp.MustCompile(`\d{4}-\d{2}-\d{2}t\d{2}:\d{2}:\d{2}(?:z|[+-]\d{2}:\d{2})`)
	if match := rfc3339Re.FindString(text); match != "" {
		if parsed, err := time.Parse(time.RFC3339, strings.ToUpper(match)); err == nil {
			t := parsed.In(loc)
			return &t
		}
	}

	durationRe := regexp.MustCompile(`(?:reset|try again)\s+(?:in|after)\s+(\d+)\s*h(?:ours?)?(?:\s+(\d+)\s*m(?:in(?:ute)?s?)?)?`)
	if m := durationRe.FindStringSubmatch(text); len(m) > 0 {
		hours, _ := strconv.Atoi(m[1])
		minutes := 0
		if len(m) > 2 && m[2] != "" {
			minutes, _ = strconv.Atoi(m[2])
		}
		t := now.Add(time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute)
		return &t
	}

	minOnlyRe := regexp.MustCompile(`(?:reset|try again)\s+(?:in|after)\s+(\d+)\s*m(?:in(?:ute)?s?)`)
	if m := minOnlyRe.FindStringSubmatch(text); len(m) > 0 {
		minutes, _ := strconv.Atoi(m[1])
		t := now.Add(time.Duration(minutes) * time.Minute)
		return &t
	}

	hhmmRe := regexp.MustCompile(`(?:reset|resets)\s+(?:at|after)\s+(\d{1,2}):(\d{2})`)
	if m := hhmmRe.FindStringSubmatch(text); len(m) > 0 {
		hour, _ := strconv.Atoi(m[1])
		minute, _ := strconv.Atoi(m[2])
		t := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)
		if !t.After(now) {
			t = t.Add(24 * time.Hour)
		}
		return &t
	}
	return nil
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "tmp-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func firstNonEmptyProvider(values ...Provider) Provider {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func roundUSD(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

func stringValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		return ""
	}
}

func floatValue(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	default:
		return 0
	}
}

func intValue(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		i, _ := x.Int64()
		return int(i)
	default:
		return 0
	}
}
