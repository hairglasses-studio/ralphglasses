// Package worker provides extracted, testable job handlers for the background worker.
package worker

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/adhd"
	"github.com/hairglasses-studio/runmylife/internal/comms"
	"github.com/hairglasses-studio/runmylife/internal/events"
	"github.com/hairglasses-studio/runmylife/internal/finance"
	"github.com/hairglasses-studio/runmylife/internal/knowledge"
	"github.com/hairglasses-studio/runmylife/internal/notifications"
	"github.com/hairglasses-studio/runmylife/internal/reviews"
)

// JobContext holds dependencies for job handlers.
type JobContext struct {
	DB      *sql.DB
	Emitter *events.Emitter
	Notify  *notifications.Dispatcher
}

// HandleJob dispatches a job to the appropriate handler based on type.
// Returns an error if the job type is unknown or execution fails.
func HandleJob(ctx context.Context, jc *JobContext, jobType, payload string) error {
	switch jobType {
	case "check_overwhelm":
		return handleCheckOverwhelm(ctx, jc)
	case "morning_briefing":
		return handleMorningBriefing(ctx, jc)
	case "check_habits":
		return handleCheckHabits(ctx, jc)
	case "log_mood_prompt":
		return handleLogMoodPrompt(ctx, jc)
	case "tomorrow_prep":
		return handleTomorrowPrep(ctx, jc)
	case "weekly_stats":
		return handleWeeklyStats(ctx, jc)
	case "social_health_review":
		return handleSocialHealthReview(ctx, jc)
	case "habit_streak_review":
		return handleHabitStreakReview(ctx, jc)
	case "financial_summary":
		return handleFinancialSummary(ctx, jc)
	case "scan_replies":
		return handleScanReplies(ctx, jc)
	case "prioritize_replies":
		return handlePrioritizeReplies(ctx, jc)
	case "notify_reply_queue":
		return handleNotifyReplyQueue(ctx, jc)
	case "build_knowledge_graph":
		return handleBuildKnowledgeGraph(ctx, jc)
	default:
		return fmt.Errorf("unknown job type: %s", jobType)
	}
}

func handleCheckOverwhelm(ctx context.Context, jc *JobContext) error {
	score, err := adhd.RunOverwhelmCheck(ctx, jc.DB)
	if err != nil {
		return err
	}
	if jc.Emitter != nil {
		jc.Emitter.OverwhelmDetected(ctx, score.CompositeScore, score.TriageActivated)
	}
	if score.TriageActivated && jc.Notify != nil {
		tasks, _ := adhd.GetTopTriageTasks(ctx, jc.DB)
		msg := fmt.Sprintf("Overwhelm score: %.0f%%. Focus only on these:\n", score.CompositeScore*100)
		for i, t := range tasks {
			msg += fmt.Sprintf("%d. %s\n", i+1, t)
		}
		jc.Notify.Send(ctx, notifications.Notification{
			Title:   "Triage mode activated",
			Message: msg,
			Urgency: notifications.UrgencyHigh,
			Source:  "check_overwhelm",
		})
	}
	return nil
}

func handleMorningBriefing(ctx context.Context, jc *JobContext) error {
	var eventCount, taskCount, replyCount int
	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	jc.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM calendar_events WHERE start_time >= ? AND start_time < ?",
		today, tomorrow).Scan(&eventCount)
	jc.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE completed = 0 AND (due_date = ? OR due_date = '')", today).Scan(&taskCount)
	jc.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending'").Scan(&replyCount)

	msg := fmt.Sprintf("Today: %d events, %d tasks, %d replies pending", eventCount, taskCount, replyCount)
	log.Printf("[worker] Morning briefing: %s", msg)

	if jc.Notify != nil {
		jc.Notify.Send(ctx, notifications.Notification{
			Title:   "Morning Briefing",
			Message: msg,
			Urgency: notifications.UrgencyNormal,
			Source:  "morning_briefing",
		})
	}
	return nil
}

func handleCheckHabits(ctx context.Context, jc *JobContext) error {
	var total, completed int
	today := time.Now().Format("2006-01-02")

	jc.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM habits").Scan(&total)
	jc.DB.QueryRowContext(ctx,
		"SELECT COUNT(DISTINCT habit_id) FROM habit_completions WHERE completed_date = ?", today).Scan(&completed)

	if total == 0 {
		return nil
	}
	rate := float64(completed) / float64(total)
	if rate < 0.5 && jc.Notify != nil {
		remaining := total - completed
		jc.Notify.Send(ctx, notifications.Notification{
			Title:   "Habit reminder",
			Message: fmt.Sprintf("%d of %d habits done today (%d remaining)", completed, total, remaining),
			Urgency: notifications.UrgencyNormal,
			Source:  "check_habits",
		})
	}
	return nil
}

func handleLogMoodPrompt(ctx context.Context, jc *JobContext) error {
	if jc.Notify != nil {
		jc.Notify.Send(ctx, notifications.Notification{
			Title:   "Log your mood",
			Message: "Take a moment to check in. How are you feeling right now?",
			Urgency: notifications.UrgencyLow,
			Source:  "log_mood_prompt",
		})
	}
	return nil
}

func handleTomorrowPrep(ctx context.Context, jc *JobContext) error {
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	dayAfter := time.Now().AddDate(0, 0, 2).Format("2006-01-02")

	var eventCount, taskCount int
	jc.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM calendar_events WHERE start_time >= ? AND start_time < ?",
		tomorrow, dayAfter).Scan(&eventCount)
	jc.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE completed = 0 AND due_date = ?", tomorrow).Scan(&taskCount)

	msg := fmt.Sprintf("Tomorrow: %d events, %d tasks due", eventCount, taskCount)

	if jc.Notify != nil {
		jc.Notify.Send(ctx, notifications.Notification{
			Title:   "Tomorrow prep",
			Message: msg,
			Urgency: notifications.UrgencyLow,
			Source:  "tomorrow_prep",
		})
	}
	return nil
}

func handleWeeklyStats(ctx context.Context, jc *JobContext) error {
	snap, err := reviews.CaptureWeekly(ctx, jc.DB, time.Now())
	if err != nil {
		return fmt.Errorf("weekly stats: %w", err)
	}

	if jc.Emitter != nil {
		jc.Emitter.ReviewGenerated(ctx, "weekly")
	}

	msg := fmt.Sprintf("Tasks: %d done | Habits: %.0f%% | Mood avg: %.1f | Focus: %dmin | Good days: %d/7",
		snap.TasksCompleted, snap.HabitRate*100, snap.MoodAvg, snap.FocusMinutes, snap.GoodEnoughDays)

	if jc.Notify != nil {
		jc.Notify.Send(ctx, notifications.Notification{
			Title:   "Weekly Review",
			Message: msg,
			Urgency: notifications.UrgencyNormal,
			Source:  "weekly_stats",
		})
	}
	return nil
}

func handleSocialHealthReview(ctx context.Context, jc *JobContext) error {
	rows, err := jc.DB.QueryContext(ctx,
		`SELECT c.name, rh.overall_score
		 FROM relationship_health rh
		 JOIN contacts c ON c.id = rh.contact_id
		 WHERE rh.overall_score < 40
		 ORDER BY rh.overall_score ASC LIMIT 5`)
	if err != nil {
		return nil // table might not have data
	}
	defer rows.Close()

	var atRisk []string
	for rows.Next() {
		var name string
		var score float64
		if rows.Scan(&name, &score) == nil {
			atRisk = append(atRisk, fmt.Sprintf("%s (%.0f%%)", name, score))
		}
	}

	if len(atRisk) > 0 && jc.Notify != nil {
		jc.Notify.Send(ctx, notifications.Notification{
			Title:   "Social health check",
			Message: "At-risk relationships: " + strings.Join(atRisk, ", "),
			Urgency: notifications.UrgencyNormal,
			Source:  "social_health_review",
		})
	}
	return nil
}

func handleHabitStreakReview(ctx context.Context, jc *JobContext) error {
	streaks := adhd.CheckStreaks(ctx, jc.DB)
	if len(streaks) == 0 {
		return nil
	}

	var parts []string
	for name, streak := range streaks {
		parts = append(parts, fmt.Sprintf("%s: %d days", name, streak))
	}

	if jc.Emitter != nil {
		for name, streak := range streaks {
			if streak >= 7 {
				jc.Emitter.AchievementEarned(ctx, "habit_streak", fmt.Sprintf("%s: %d days", name, streak))
			}
		}
	}

	log.Printf("[worker] Active streaks: %s", strings.Join(parts, ", "))
	return nil
}

func handleFinancialSummary(ctx context.Context, jc *JobContext) error {
	since := time.Now().Format("2006-01") + "-01"
	until := time.Now().AddDate(0, 1, 0).Format("2006-01") + "-01"
	summary := finance.SpendingSummary(ctx, jc.DB, since, until)

	totalSpend, _ := summary["total_spend"].(float64)
	txnCount, _ := summary["transaction_count"].(int)
	msg := fmt.Sprintf("MTD: $%.2f across %d transactions", -totalSpend, txnCount)

	log.Printf("[worker] Financial summary: %s", msg)

	if jc.Notify != nil {
		jc.Notify.Send(ctx, notifications.Notification{
			Title:   "Financial summary",
			Message: msg,
			Urgency: notifications.UrgencyLow,
			Source:  "financial_summary",
		})
	}
	return nil
}

func handleScanReplies(ctx context.Context, jc *JobContext) error {
	msgs, err := comms.ScanAll(ctx, jc.DB, "", "")
	if err != nil {
		return err
	}

	counts := make(map[comms.Channel]int)
	for _, m := range msgs {
		counts[m.Channel]++
	}

	var parts []string
	for ch, n := range counts {
		parts = append(parts, fmt.Sprintf("%s: %d", ch, n))
	}
	log.Printf("[worker] Reply scan: %d total (%s)", len(msgs), strings.Join(parts, ", "))
	return nil
}

func handlePrioritizeReplies(ctx context.Context, jc *JobContext) error {
	msgs, err := comms.ScanAll(ctx, jc.DB, "", "")
	if err != nil {
		return err
	}

	type scored struct {
		msg     comms.UnifiedMessage
		urgency comms.UrgencyFactors
	}
	var results []scored
	for _, m := range msgs {
		u := comms.ScoreUrgency(m, comms.TierNormal, 24, 1, 1.0)
		results = append(results, scored{m, u})
	}

	// Sort by total urgency descending (simple selection for top 10)
	for i := 0; i < len(results) && i < 10; i++ {
		maxIdx := i
		for j := i + 1; j < len(results); j++ {
			if results[j].urgency.Total > results[maxIdx].urgency.Total {
				maxIdx = j
			}
		}
		results[i], results[maxIdx] = results[maxIdx], results[i]
	}

	limit := 10
	if len(results) < limit {
		limit = len(results)
	}
	for _, r := range results[:limit] {
		jc.DB.ExecContext(ctx,
			`INSERT OR REPLACE INTO reply_tracker (channel, contact_name, preview, urgency_score, status)
			 VALUES (?, ?, ?, ?, 'pending')`,
			r.msg.Channel, r.msg.ContactName, r.msg.Preview, r.urgency.Total)
	}

	log.Printf("[worker] Prioritized %d replies", limit)
	return nil
}

func handleNotifyReplyQueue(ctx context.Context, jc *JobContext) error {
	rows, err := jc.DB.QueryContext(ctx,
		`SELECT contact_name, channel, preview FROM reply_tracker
		 WHERE status = 'pending'
		 ORDER BY urgency_score DESC LIMIT 5`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var name, channel, preview string
		if rows.Scan(&name, &channel, &preview) == nil {
			lines = append(lines, fmt.Sprintf("[%s] %s: %s", channel, name, preview))
		}
	}

	if len(lines) > 0 && jc.Notify != nil {
		jc.Notify.Send(ctx, notifications.Notification{
			Title:   fmt.Sprintf("Reply queue (%d pending)", len(lines)),
			Message: strings.Join(lines, "\n"),
			Urgency: notifications.UrgencyNormal,
			Source:  "notify_reply_queue",
		})
	}
	return nil
}

func handleBuildKnowledgeGraph(ctx context.Context, jc *JobContext) error {
	n, err := knowledge.BuildFromDB(ctx, jc.DB)
	if err != nil {
		return fmt.Errorf("build knowledge graph: %w", err)
	}
	log.Printf("[worker] Knowledge graph built: %d links", n)
	return nil
}
