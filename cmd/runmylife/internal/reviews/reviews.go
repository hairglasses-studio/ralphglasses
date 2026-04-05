// Package reviews provides structured review snapshots (weekly, monthly)
// and ADHD-friendly review logic: celebrate wins first, "good enough days" count,
// no guilt framing.
package reviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// WeeklySnapshot holds a week's worth of aggregated metrics.
type WeeklySnapshot struct {
	WeekOf            string         `json:"week_of"`
	TasksCompleted    int            `json:"tasks_completed"`
	TasksCreated      int            `json:"tasks_created"`
	HabitsCompleted   int            `json:"habits_completed"`
	HabitsTotal       int            `json:"habits_total"`
	HabitRate         float64        `json:"habit_rate"` // 0.0-1.0
	MoodAvg           float64        `json:"mood_avg"`
	EnergyAvg         float64        `json:"energy_avg"`
	SleepAvgHours     float64        `json:"sleep_avg_hours"`
	RepliesSent       int            `json:"replies_sent"`
	RepliesOverdue    int            `json:"replies_overdue"`
	EmailsTriaged     int            `json:"emails_triaged"`
	CalendarEvents    int            `json:"calendar_events"`
	SpendTotal        float64        `json:"spend_total"`
	FocusMinutes      int            `json:"focus_minutes"`
	OverwhelmDays     int            `json:"overwhelm_days"` // days triage activated
	GoodEnoughDays    int            `json:"good_enough_days"` // days with >=60% habit + >=1 task
	SocialOutreaches  int            `json:"social_outreaches"`
	SRSReviews        int            `json:"srs_reviews"`
	JournalEntries    int            `json:"journal_entries"`
	CategoryBreakdown map[string]int `json:"category_breakdown"` // tool usage per life category
}

// MonthlySnapshot aggregates a full month.
type MonthlySnapshot struct {
	Month              string   `json:"month"` // YYYY-MM
	WeekCount          int      `json:"week_count"`
	TasksCompleted     int      `json:"tasks_completed"`
	HabitStreakMax     int      `json:"habit_streak_max"`
	HabitRateAvg       float64  `json:"habit_rate_avg"`
	MoodTrend          string   `json:"mood_trend"` // "improving", "stable", "declining"
	EnergyTrend        string   `json:"energy_trend"`
	SpendTotal         float64  `json:"spend_total"`
	SpendVsBudget      float64  `json:"spend_vs_budget"` // ratio
	TopCategories      []string `json:"top_categories"`
	OverwhelmDays      int      `json:"overwhelm_days"`
	GoodEnoughDays     int      `json:"good_enough_days"`
	Wins               []string `json:"wins"`
	SubscriptionSpend  float64  `json:"subscription_spend"`
}

// CaptureWeekly gathers metrics for the week containing the given date and persists the snapshot.
func CaptureWeekly(ctx context.Context, db *sql.DB, weekOf time.Time) (*WeeklySnapshot, error) {
	monday := mondayOf(weekOf)
	sunday := monday.AddDate(0, 0, 7)
	monStr := monday.Format("2006-01-02")
	sunStr := sunday.Format("2006-01-02")

	snap := &WeeklySnapshot{WeekOf: monStr, CategoryBreakdown: make(map[string]int)}

	// Tasks
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE completed = 1 AND updated_at >= ? AND updated_at < ?",
		monStr, sunStr).Scan(&snap.TasksCompleted)
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE created_at >= ? AND created_at < ?",
		monStr, sunStr).Scan(&snap.TasksCreated)

	// Habits
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM habit_completions WHERE completed_at >= ? AND completed_at < ?",
		monStr, sunStr).Scan(&snap.HabitsCompleted)
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM habits WHERE archived = 0").Scan(&snap.HabitsTotal)
	if snap.HabitsTotal > 0 {
		// Possible completions = habits * 7 days
		snap.HabitRate = float64(snap.HabitsCompleted) / float64(snap.HabitsTotal*7)
		if snap.HabitRate > 1.0 {
			snap.HabitRate = 1.0
		}
	}

	// Mood & energy (averages from mood_log)
	db.QueryRowContext(ctx,
		"SELECT COALESCE(AVG(mood_score), 0), COALESCE(AVG(energy_level), 0), COALESCE(AVG(sleep_hours), 0) FROM mood_log WHERE date >= ? AND date < ?",
		monStr, sunStr).Scan(&snap.MoodAvg, &snap.EnergyAvg, &snap.SleepAvgHours)

	// Replies
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM reply_tracker WHERE status = 'resolved' AND updated_at >= ? AND updated_at < ?",
		monStr, sunStr).Scan(&snap.RepliesSent)
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending' AND created_at < ?",
		sunStr).Scan(&snap.RepliesOverdue)

	// Emails
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM gmail_messages WHERE triaged = 1 AND updated_at >= ? AND updated_at < ?",
		monStr, sunStr).Scan(&snap.EmailsTriaged)

	// Calendar
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM calendar_events WHERE start_time >= ? AND start_time < ?",
		monStr, sunStr).Scan(&snap.CalendarEvents)

	// Spending
	db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE date >= ? AND date < ? AND type = 'expense'",
		monStr, sunStr).Scan(&snap.SpendTotal)

	// Focus minutes from focus_sessions
	db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(duration_minutes), 0) FROM focus_sessions WHERE started_at >= ? AND started_at < ?",
		monStr, sunStr).Scan(&snap.FocusMinutes)

	// Overwhelm days
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM daily_overwhelm_metric WHERE date >= ? AND date < ? AND triage_activated = 1",
		monStr, sunStr).Scan(&snap.OverwhelmDays)

	// Good enough days: days where at least 1 task completed + habit rate >=60%
	snap.GoodEnoughDays = countGoodEnoughDays(ctx, db, monday, 7)

	// Social outreaches
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM outreach_reminders WHERE completed_at IS NOT NULL AND completed_at >= ? AND completed_at < ?",
		monStr, sunStr).Scan(&snap.SocialOutreaches)

	// SRS reviews
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM srs_reviews WHERE reviewed_at >= ? AND reviewed_at < ?",
		monStr, sunStr).Scan(&snap.SRSReviews)

	// Journal entries (count files for the week)
	db.QueryRowContext(ctx,
		"SELECT COUNT(DISTINCT date) FROM mood_log WHERE date >= ? AND date < ?",
		monStr, sunStr).Scan(&snap.JournalEntries)

	// Tool usage by category
	catRows, err := db.QueryContext(ctx,
		`SELECT category, COUNT(*) FROM tool_usage
		 WHERE timestamp >= ? AND timestamp < ?
		 GROUP BY category ORDER BY COUNT(*) DESC`,
		monStr, sunStr)
	if err == nil {
		defer catRows.Close()
		for catRows.Next() {
			var cat string
			var count int
			if catRows.Scan(&cat, &count) == nil && cat != "" {
				snap.CategoryBreakdown[cat] = count
			}
		}
	}

	// Persist
	breakdownJSON, _ := json.Marshal(snap.CategoryBreakdown)
	_, err = db.ExecContext(ctx,
		`INSERT OR REPLACE INTO weekly_review_snapshots
		 (week_of, tasks_completed, tasks_created, habits_completed, habits_total, habit_rate,
		  mood_avg, energy_avg, sleep_avg_hours, replies_sent, replies_overdue, emails_triaged,
		  calendar_events, spend_total, focus_minutes, overwhelm_days, good_enough_days,
		  social_outreaches, srs_reviews, journal_entries, category_breakdown)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.WeekOf, snap.TasksCompleted, snap.TasksCreated,
		snap.HabitsCompleted, snap.HabitsTotal, snap.HabitRate,
		snap.MoodAvg, snap.EnergyAvg, snap.SleepAvgHours,
		snap.RepliesSent, snap.RepliesOverdue, snap.EmailsTriaged,
		snap.CalendarEvents, snap.SpendTotal, snap.FocusMinutes,
		snap.OverwhelmDays, snap.GoodEnoughDays,
		snap.SocialOutreaches, snap.SRSReviews, snap.JournalEntries,
		string(breakdownJSON),
	)
	if err != nil {
		return snap, fmt.Errorf("save weekly snapshot: %w", err)
	}

	return snap, nil
}

// CaptureMonthly gathers metrics for the given month.
func CaptureMonthly(ctx context.Context, db *sql.DB, monthOf time.Time) (*MonthlySnapshot, error) {
	firstDay := time.Date(monthOf.Year(), monthOf.Month(), 1, 0, 0, 0, 0, monthOf.Location())
	lastDay := firstDay.AddDate(0, 1, 0)
	monthStr := firstDay.Format("2006-01")
	firstStr := firstDay.Format("2006-01-02")
	lastStr := lastDay.Format("2006-01-02")
	daysInMonth := int(lastDay.Sub(firstDay).Hours() / 24)

	snap := &MonthlySnapshot{Month: monthStr}

	// Count weeks
	snap.WeekCount = (daysInMonth + 6) / 7

	// Tasks
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE completed = 1 AND updated_at >= ? AND updated_at < ?",
		firstStr, lastStr).Scan(&snap.TasksCompleted)

	// Habit rate average from weekly snapshots
	db.QueryRowContext(ctx,
		"SELECT COALESCE(AVG(habit_rate), 0) FROM weekly_review_snapshots WHERE week_of >= ? AND week_of < ?",
		firstStr, lastStr).Scan(&snap.HabitRateAvg)

	// Habit streak max: longest consecutive days with at least 1 habit completion
	snap.HabitStreakMax = longestHabitStreak(ctx, db, firstDay, daysInMonth)

	// Mood trend: compare first half vs second half
	midpoint := firstDay.AddDate(0, 0, daysInMonth/2).Format("2006-01-02")
	var firstHalfMood, secondHalfMood float64
	db.QueryRowContext(ctx,
		"SELECT COALESCE(AVG(mood_score), 0) FROM mood_log WHERE date >= ? AND date < ?",
		firstStr, midpoint).Scan(&firstHalfMood)
	db.QueryRowContext(ctx,
		"SELECT COALESCE(AVG(mood_score), 0) FROM mood_log WHERE date >= ? AND date < ?",
		midpoint, lastStr).Scan(&secondHalfMood)
	snap.MoodTrend = trend(firstHalfMood, secondHalfMood)

	var firstHalfEnergy, secondHalfEnergy float64
	db.QueryRowContext(ctx,
		"SELECT COALESCE(AVG(energy_level), 0) FROM mood_log WHERE date >= ? AND date < ?",
		firstStr, midpoint).Scan(&firstHalfEnergy)
	db.QueryRowContext(ctx,
		"SELECT COALESCE(AVG(energy_level), 0) FROM mood_log WHERE date >= ? AND date < ?",
		midpoint, lastStr).Scan(&secondHalfEnergy)
	snap.EnergyTrend = trend(firstHalfEnergy, secondHalfEnergy)

	// Spending
	db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE date >= ? AND date < ? AND type = 'expense'",
		firstStr, lastStr).Scan(&snap.SpendTotal)

	// Spend vs budget
	var totalBudget float64
	db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(amount), 0) FROM budgets WHERE active = 1").Scan(&totalBudget)
	if totalBudget > 0 {
		snap.SpendVsBudget = snap.SpendTotal / totalBudget
	}

	// Top tool categories
	catRows, err := db.QueryContext(ctx,
		`SELECT category FROM tool_usage
		 WHERE timestamp >= ? AND timestamp < ?
		 GROUP BY category ORDER BY COUNT(*) DESC LIMIT 5`,
		firstStr, lastStr)
	if err == nil {
		defer catRows.Close()
		for catRows.Next() {
			var cat string
			if catRows.Scan(&cat) == nil && cat != "" {
				snap.TopCategories = append(snap.TopCategories, cat)
			}
		}
	}

	// Overwhelm days
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM daily_overwhelm_metric WHERE date >= ? AND date < ? AND triage_activated = 1",
		firstStr, lastStr).Scan(&snap.OverwhelmDays)

	// Good enough days
	snap.GoodEnoughDays = countGoodEnoughDays(ctx, db, firstDay, daysInMonth)

	// Subscription spend
	db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(CASE
			WHEN billing_cycle = 'monthly' THEN amount
			WHEN billing_cycle = 'yearly' THEN amount / 12.0
			ELSE amount END), 0)
		 FROM subscriptions WHERE status = 'active'`).Scan(&snap.SubscriptionSpend)

	// Wins: tasks completed with high priority, habit streaks, social outreaches
	var wins []string
	if snap.TasksCompleted > 0 {
		wins = append(wins, fmt.Sprintf("Completed %d tasks", snap.TasksCompleted))
	}
	if snap.HabitStreakMax >= 7 {
		wins = append(wins, fmt.Sprintf("Habit streak of %d days", snap.HabitStreakMax))
	}
	if snap.GoodEnoughDays >= daysInMonth/2 {
		wins = append(wins, fmt.Sprintf("%d good-enough days (>50%%)", snap.GoodEnoughDays))
	}
	if snap.OverwhelmDays == 0 {
		wins = append(wins, "Zero overwhelm days")
	}
	snap.Wins = wins

	// Persist
	winsJSON, _ := json.Marshal(snap.Wins)
	catsJSON, _ := json.Marshal(snap.TopCategories)
	_, err = db.ExecContext(ctx,
		`INSERT OR REPLACE INTO monthly_review_snapshots
		 (month, week_count, tasks_completed, habit_streak_max, habit_rate_avg,
		  mood_trend, energy_trend, spend_total, spend_vs_budget,
		  top_categories, overwhelm_days, good_enough_days, wins, subscription_spend)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.Month, snap.WeekCount, snap.TasksCompleted,
		snap.HabitStreakMax, snap.HabitRateAvg,
		snap.MoodTrend, snap.EnergyTrend, snap.SpendTotal, snap.SpendVsBudget,
		string(catsJSON), snap.OverwhelmDays, snap.GoodEnoughDays,
		string(winsJSON), snap.SubscriptionSpend,
	)
	if err != nil {
		return snap, fmt.Errorf("save monthly snapshot: %w", err)
	}

	return snap, nil
}

// LoadPreviousWeekly loads the snapshot from the previous week for comparison.
func LoadPreviousWeekly(ctx context.Context, db *sql.DB, currentWeekOf string) (*WeeklySnapshot, error) {
	t, err := time.Parse("2006-01-02", currentWeekOf)
	if err != nil {
		return nil, err
	}
	prevWeek := t.AddDate(0, 0, -7).Format("2006-01-02")

	snap := &WeeklySnapshot{CategoryBreakdown: make(map[string]int)}
	var breakdownJSON string
	err = db.QueryRowContext(ctx,
		`SELECT week_of, tasks_completed, tasks_created, habits_completed, habits_total, habit_rate,
		  mood_avg, energy_avg, sleep_avg_hours, replies_sent, replies_overdue, emails_triaged,
		  calendar_events, spend_total, focus_minutes, overwhelm_days, good_enough_days,
		  social_outreaches, srs_reviews, journal_entries, category_breakdown
		 FROM weekly_review_snapshots WHERE week_of = ?`, prevWeek).Scan(
		&snap.WeekOf, &snap.TasksCompleted, &snap.TasksCreated,
		&snap.HabitsCompleted, &snap.HabitsTotal, &snap.HabitRate,
		&snap.MoodAvg, &snap.EnergyAvg, &snap.SleepAvgHours,
		&snap.RepliesSent, &snap.RepliesOverdue, &snap.EmailsTriaged,
		&snap.CalendarEvents, &snap.SpendTotal, &snap.FocusMinutes,
		&snap.OverwhelmDays, &snap.GoodEnoughDays,
		&snap.SocialOutreaches, &snap.SRSReviews, &snap.JournalEntries,
		&breakdownJSON,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(breakdownJSON), &snap.CategoryBreakdown)
	return snap, nil
}

// countGoodEnoughDays counts days where the user completed >=1 task AND >=60% of habits.
func countGoodEnoughDays(ctx context.Context, db *sql.DB, start time.Time, days int) int {
	count := 0
	var totalHabits int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habits WHERE archived = 0").Scan(&totalHabits)
	if totalHabits == 0 {
		totalHabits = 1 // avoid division by zero
	}

	for i := 0; i < days; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		nextDate := start.AddDate(0, 0, i+1).Format("2006-01-02")

		var tasksCompleted, habitsCompleted int
		db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM tasks WHERE completed = 1 AND updated_at >= ? AND updated_at < ?",
			date, nextDate).Scan(&tasksCompleted)
		db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM habit_completions WHERE completed_at >= ? AND completed_at < ?",
			date, nextDate).Scan(&habitsCompleted)

		habitRate := float64(habitsCompleted) / float64(totalHabits)
		if tasksCompleted >= 1 && habitRate >= 0.6 {
			count++
		}
	}
	return count
}

// longestHabitStreak finds the longest consecutive-day streak with >=1 habit completion.
func longestHabitStreak(ctx context.Context, db *sql.DB, start time.Time, days int) int {
	maxStreak := 0
	currentStreak := 0
	for i := 0; i < days; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		nextDate := start.AddDate(0, 0, i+1).Format("2006-01-02")
		var count int
		db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM habit_completions WHERE completed_at >= ? AND completed_at < ?",
			date, nextDate).Scan(&count)
		if count > 0 {
			currentStreak++
			if currentStreak > maxStreak {
				maxStreak = currentStreak
			}
		} else {
			currentStreak = 0
		}
	}
	return maxStreak
}

func mondayOf(t time.Time) time.Time {
	offset := int(t.Weekday()) - 1
	if offset < 0 {
		offset = 6
	}
	return t.AddDate(0, 0, -offset)
}

func trend(first, second float64) string {
	if first == 0 && second == 0 {
		return "no data"
	}
	diff := second - first
	if diff > 0.5 {
		return "improving"
	}
	if diff < -0.5 {
		return "declining"
	}
	return "stable"
}

// FormatDelta returns a "+X" or "-X" string for week-over-week comparison.
func FormatDelta(current, previous int) string {
	delta := current - previous
	if delta > 0 {
		return fmt.Sprintf("+%d", delta)
	}
	if delta < 0 {
		return fmt.Sprintf("%d", delta)
	}
	return "0"
}

// FormatDeltaF returns a "+X.X" or "-X.X" string for float comparison.
func FormatDeltaF(current, previous float64) string {
	delta := current - previous
	if delta > 0 {
		return fmt.Sprintf("+%.1f", delta)
	}
	if delta < 0 {
		return fmt.Sprintf("%.1f", delta)
	}
	return "0.0"
}
