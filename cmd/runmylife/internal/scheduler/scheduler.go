// Package scheduler provides time-block-aware cron-like scheduling
// for recurring tasks and workflow execution.
package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/timecontext"
)

// Schedule represents a parsed schedule definition.
// Supported formats:
//   - "daily:HH:MM"        — run at specific time each day
//   - "weekly:DAY:HH:MM"   — run at specific time on a specific day (Mon-Sun)
//   - "interval:Xm"        — run every X minutes
//   - "block:BLOCK"        — run once when entering a time block (morning/work/evening/night)
type Schedule struct {
	Type     string // "daily", "weekly", "interval", "block"
	Hour     int
	Minute   int
	Weekday  time.Weekday
	Interval time.Duration
	Block    timecontext.Block
	Raw      string
}

// Parse parses a schedule string into a Schedule.
func Parse(s string) (*Schedule, error) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid schedule: %q", s)
	}

	sched := &Schedule{Raw: s, Type: parts[0]}

	switch parts[0] {
	case "daily":
		if len(parts) != 3 {
			return nil, fmt.Errorf("daily schedule must be daily:HH:MM, got %q", s)
		}
		if _, err := fmt.Sscanf(parts[1]+":"+parts[2], "%d:%d", &sched.Hour, &sched.Minute); err != nil {
			return nil, fmt.Errorf("invalid time in schedule: %w", err)
		}

	case "weekly":
		if len(parts) != 4 {
			return nil, fmt.Errorf("weekly schedule must be weekly:DAY:HH:MM, got %q", s)
		}
		wd, err := parseWeekday(parts[1])
		if err != nil {
			return nil, err
		}
		sched.Weekday = wd
		if _, err := fmt.Sscanf(parts[2]+":"+parts[3], "%d:%d", &sched.Hour, &sched.Minute); err != nil {
			return nil, fmt.Errorf("invalid time in schedule: %w", err)
		}

	case "interval":
		dur, err := time.ParseDuration(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid interval: %w", err)
		}
		sched.Interval = dur

	case "block":
		sched.Block = timecontext.Block(parts[1])

	default:
		return nil, fmt.Errorf("unknown schedule type: %q", parts[0])
	}

	return sched, nil
}

// ShouldRun checks if the schedule should fire at the given time,
// considering the last run time.
func (s *Schedule) ShouldRun(now time.Time, lastRun time.Time) bool {
	switch s.Type {
	case "daily":
		if now.Hour() != s.Hour || now.Minute() != s.Minute {
			return false
		}
		// Only fire once per day
		return lastRun.Format("2006-01-02") != now.Format("2006-01-02")

	case "weekly":
		if now.Weekday() != s.Weekday || now.Hour() != s.Hour || now.Minute() != s.Minute {
			return false
		}
		// Only fire once per week
		_, lastWeek := lastRun.ISOWeek()
		_, nowWeek := now.ISOWeek()
		return lastWeek != nowWeek || lastRun.Year() != now.Year()

	case "interval":
		return time.Since(lastRun) >= s.Interval

	case "block":
		currentBlock := timecontext.BlockAt(now)
		lastBlock := timecontext.BlockAt(lastRun)
		return currentBlock == s.Block && (lastBlock != s.Block || lastRun.Format("2006-01-02") != now.Format("2006-01-02"))

	default:
		return false
	}
}

// WorkflowStep represents a step in a workflow to execute.
type WorkflowStep struct {
	Order       int    `json:"order"`
	Action      string `json:"action"` // e.g., "sync_gmail", "check_overwhelm", "send_notification"
	Params      string `json:"params,omitempty"`
	Description string `json:"description,omitempty"`
}

// BuiltinWorkflow is a predefined workflow.
type BuiltinWorkflow struct {
	Name        string
	Description string
	Steps       []WorkflowStep
	Schedule    string
}

// BuiltinWorkflows returns the predefined workflow library.
func BuiltinWorkflows() []BuiltinWorkflow {
	return []BuiltinWorkflow{
		{
			Name:        "morning_routine",
			Description: "Morning briefing and setup for the day",
			Schedule:    "daily:07:00",
			Steps: []WorkflowStep{
				{Order: 1, Action: "sync_calendar", Description: "Refresh today's calendar"},
				{Order: 2, Action: "sync_todoist", Description: "Refresh task list"},
				{Order: 3, Action: "check_overwhelm", Description: "Assess overwhelm level"},
				{Order: 4, Action: "morning_briefing", Description: "Generate and send briefing"},
			},
		},
		{
			Name:        "evening_winddown",
			Description: "Evening review and tomorrow prep",
			Schedule:    "daily:21:00",
			Steps: []WorkflowStep{
				{Order: 1, Action: "check_habits", Description: "Check habit completions"},
				{Order: 2, Action: "log_mood_prompt", Description: "Prompt for mood log"},
				{Order: 3, Action: "tomorrow_prep", Description: "Preview tomorrow's calendar"},
			},
		},
		{
			Name:        "weekly_review",
			Description: "Sunday evening weekly review",
			Schedule:    "weekly:Sun:18:00",
			Steps: []WorkflowStep{
				{Order: 1, Action: "weekly_stats", Description: "Compile weekly statistics"},
				{Order: 2, Action: "social_health_review", Description: "Review relationship health"},
				{Order: 3, Action: "habit_streak_review", Description: "Review habit streaks"},
				{Order: 4, Action: "financial_summary", Description: "Weekly spending summary"},
			},
		},
		{
			Name:        "reply_session",
			Description: "Focused reply batch processing",
			Schedule:    "block:work",
			Steps: []WorkflowStep{
				{Order: 1, Action: "scan_replies", Description: "Scan for pending replies"},
				{Order: 2, Action: "prioritize_replies", Description: "Rank by urgency"},
				{Order: 3, Action: "notify_reply_queue", Description: "Send prioritized reply list"},
			},
		},
	}
}

// Executor runs scheduled workflows.
type Executor struct {
	db       *sql.DB
	lastRuns map[string]time.Time // workflow name → last execution time
}

// NewExecutor creates a workflow executor.
func NewExecutor(db *sql.DB) *Executor {
	return &Executor{
		db:       db,
		lastRuns: make(map[string]time.Time),
	}
}

// CheckAndRun checks all scheduled workflows and runs any that are due.
func (e *Executor) CheckAndRun(ctx context.Context) {
	now := time.Now()

	// Check builtin workflows
	for _, wf := range BuiltinWorkflows() {
		sched, err := Parse(wf.Schedule)
		if err != nil {
			continue
		}

		lastRun := e.lastRuns[wf.Name]
		if sched.ShouldRun(now, lastRun) {
			log.Printf("[scheduler] Running workflow: %s", wf.Name)
			e.executeWorkflow(ctx, wf.Name, wf.Steps)
			e.lastRuns[wf.Name] = now
		}
	}

	// Check user-defined workflows from DB
	rows, err := e.db.QueryContext(ctx,
		"SELECT id, name, steps, schedule FROM workflows WHERE enabled = 1 AND schedule != ''")
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var name, stepsJSON, scheduleStr string
		if err := rows.Scan(&id, &name, &stepsJSON, &scheduleStr); err != nil {
			continue
		}

		sched, err := Parse(scheduleStr)
		if err != nil {
			continue
		}

		key := fmt.Sprintf("user_%d_%s", id, name)
		lastRun := e.lastRuns[key]
		if sched.ShouldRun(now, lastRun) {
			var steps []WorkflowStep
			if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil {
				log.Printf("[scheduler] Invalid steps for workflow %q: %v", name, err)
				continue
			}
			log.Printf("[scheduler] Running user workflow: %s", name)
			e.executeWorkflow(ctx, name, steps)
			e.lastRuns[key] = now
		}
	}
}

// SuppressNightBlock checks if we should suppress non-urgent workflows during night.
func SuppressNightBlock() bool {
	return timecontext.CurrentBlock() == timecontext.Night
}

// executeWorkflow runs each step in sequence, logging results.
func (e *Executor) executeWorkflow(ctx context.Context, name string, steps []WorkflowStep) {
	startedAt := time.Now()

	for _, step := range steps {
		if ctx.Err() != nil {
			log.Printf("[scheduler] Workflow %q cancelled at step %d", name, step.Order)
			break
		}
		log.Printf("[scheduler] %s step %d: %s", name, step.Order, step.Description)
		// Steps are logged only — actual execution is handled by the worker's
		// task functions when dispatched via the job queue. The scheduler's role
		// is to enqueue the jobs at the right time.
		_, _ = e.db.ExecContext(ctx,
			`INSERT INTO job_queue (type, payload, priority) VALUES (?, ?, ?)`,
			step.Action, step.Params, 5, // normal priority
		)
	}

	elapsed := time.Since(startedAt)
	log.Printf("[scheduler] Workflow %q completed in %v (%d steps)", name, elapsed, len(steps))
}

func parseWeekday(s string) (time.Weekday, error) {
	switch strings.ToLower(s) {
	case "sun", "sunday":
		return time.Sunday, nil
	case "mon", "monday":
		return time.Monday, nil
	case "tue", "tuesday":
		return time.Tuesday, nil
	case "wed", "wednesday":
		return time.Wednesday, nil
	case "thu", "thursday":
		return time.Thursday, nil
	case "fri", "friday":
		return time.Friday, nil
	case "sat", "saturday":
		return time.Saturday, nil
	default:
		return 0, fmt.Errorf("unknown weekday: %q", s)
	}
}
