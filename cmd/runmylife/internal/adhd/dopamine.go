package adhd

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// AchievementType constants for different kinds of milestones.
const (
	AchievementTaskStreak   = "task_streak"
	AchievementHabitStreak  = "habit_streak"
	AchievementReplyCleared = "reply_cleared"
	AchievementFocusMarathon = "focus_marathon"
	AchievementGoodWeek     = "good_week"
	AchievementInboxZero    = "inbox_zero"
	AchievementNewPersonal  = "personal_best"
)

// Celebration holds a formatted message for a recent achievement.
type Celebration struct {
	Type        string
	Title       string
	Description string
	Value       int
	AchievedAt  string
}

// CheckAndRecordAchievements scans current state for new achievements to celebrate.
// Returns any newly minted achievements.
func CheckAndRecordAchievements(ctx context.Context, db *sql.DB) []Celebration {
	var celebrations []Celebration
	today := time.Now().Format("2006-01-02")

	// Task completion streaks (3, 7, 14, 30 days)
	taskStreak := countConsecutiveDaysWithCompletedTasks(ctx, db)
	for _, milestone := range []int{3, 7, 14, 30} {
		if taskStreak >= milestone && !hasRecentAchievement(ctx, db, AchievementTaskStreak, milestone, 7) {
			title := fmt.Sprintf("%d-day task streak!", milestone)
			desc := fmt.Sprintf("You've completed at least one task every day for %d days straight.", milestone)
			RecordAchievement(ctx, db, AchievementTaskStreak, title, desc, "personal", milestone)
			celebrations = append(celebrations, Celebration{
				Type: AchievementTaskStreak, Title: title, Description: desc,
				Value: milestone, AchievedAt: today,
			})
		}
	}

	// Habit streaks
	streaks := CheckStreaks(ctx, db)
	for name, streak := range streaks {
		for _, milestone := range []int{7, 14, 30, 60, 90} {
			if streak >= milestone && !hasRecentAchievement(ctx, db, AchievementHabitStreak, milestone, 7) {
				title := fmt.Sprintf("%s: %d-day streak!", name, milestone)
				desc := fmt.Sprintf("You've kept %s going for %d days.", name, milestone)
				RecordAchievement(ctx, db, AchievementHabitStreak, title, desc, "wellness", milestone)
				celebrations = append(celebrations, Celebration{
					Type: AchievementHabitStreak, Title: title, Description: desc,
					Value: milestone, AchievedAt: today,
				})
				break // only celebrate the highest milestone per habit
			}
		}
	}

	// Reply debt cleared
	var pendingReplies int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending'").Scan(&pendingReplies)
	if pendingReplies == 0 && !hasRecentAchievement(ctx, db, AchievementReplyCleared, 0, 1) {
		// Check there were some replies resolved recently (not just empty inbox)
		var recentResolved int
		db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM reply_tracker WHERE status = 'resolved' AND updated_at >= datetime('now', '-24 hours')").Scan(&recentResolved)
		if recentResolved > 0 {
			title := "Reply debt cleared!"
			desc := fmt.Sprintf("All caught up — %d replies sent today.", recentResolved)
			RecordAchievement(ctx, db, AchievementReplyCleared, title, desc, "personal", recentResolved)
			celebrations = append(celebrations, Celebration{
				Type: AchievementReplyCleared, Title: title, Description: desc,
				Value: recentResolved, AchievedAt: today,
			})
		}
	}

	// Focus marathon (single session >=60 min without interruption)
	var longFocusSessions int
	db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM focus_sessions
		 WHERE actual_minutes >= 60 AND interrupted = 0
		 AND date(started_at) = ? AND ended_at IS NOT NULL`, today).Scan(&longFocusSessions)
	if longFocusSessions > 0 && !hasRecentAchievement(ctx, db, AchievementFocusMarathon, 60, 1) {
		title := "Focus marathon!"
		desc := "60+ minutes of uninterrupted focus. Deep work achieved."
		RecordAchievement(ctx, db, AchievementFocusMarathon, title, desc, "growth", 60)
		celebrations = append(celebrations, Celebration{
			Type: AchievementFocusMarathon, Title: title, Description: desc,
			Value: 60, AchievedAt: today,
		})
	}

	// Inbox zero
	var untriagedEmails int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM gmail_messages WHERE triaged = 0").Scan(&untriagedEmails)
	if untriagedEmails == 0 && !hasRecentAchievement(ctx, db, AchievementInboxZero, 0, 1) {
		var triaged int
		db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM gmail_messages WHERE triaged = 1 AND updated_at >= datetime('now', '-24 hours')").Scan(&triaged)
		if triaged > 0 {
			title := "Inbox zero!"
			desc := "All emails triaged. Clarity achieved."
			RecordAchievement(ctx, db, AchievementInboxZero, title, desc, "personal", triaged)
			celebrations = append(celebrations, Celebration{
				Type: AchievementInboxZero, Title: title, Description: desc,
				Value: triaged, AchievedAt: today,
			})
		}
	}

	return celebrations
}

// GetRecentCelebrations returns achievements from the last N days.
func GetRecentCelebrations(ctx context.Context, db *sql.DB, days int) []Celebration {
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := db.QueryContext(ctx,
		`SELECT achievement_type, title, description, value, achieved_at
		 FROM achievement_milestones WHERE achieved_at >= ?
		 ORDER BY achieved_at DESC LIMIT 20`, cutoff)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var celebrations []Celebration
	for rows.Next() {
		var c Celebration
		if rows.Scan(&c.Type, &c.Title, &c.Description, &c.Value, &c.AchievedAt) == nil {
			celebrations = append(celebrations, c)
		}
	}
	return celebrations
}

// countConsecutiveDaysWithCompletedTasks counts the current streak of days
// where at least one task was completed.
func countConsecutiveDaysWithCompletedTasks(ctx context.Context, db *sql.DB) int {
	streak := 0
	for i := 0; i < 365; i++ {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		nextDate := time.Now().AddDate(0, 0, -i+1).Format("2006-01-02")
		var count int
		db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM tasks WHERE completed = 1 AND updated_at >= ? AND updated_at < ?",
			date, nextDate).Scan(&count)
		if count == 0 {
			break
		}
		streak++
	}
	return streak
}

// hasRecentAchievement checks if an achievement of this type+value was recorded in the last N days.
func hasRecentAchievement(ctx context.Context, db *sql.DB, achievementType string, value int, withinDays int) bool {
	cutoff := time.Now().AddDate(0, 0, -withinDays).Format("2006-01-02")
	var count int
	if value > 0 {
		db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM achievement_milestones WHERE achievement_type = ? AND value = ? AND achieved_at >= ?",
			achievementType, value, cutoff).Scan(&count)
	} else {
		db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM achievement_milestones WHERE achievement_type = ? AND achieved_at >= ?",
			achievementType, cutoff).Scan(&count)
	}
	return count > 0
}
