// Package intelligence provides cross-category suggestion engine.
// Analyzes state across all life modules and generates context-aware suggestions.
package intelligence

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/timecontext"
)

// Suggestion represents a cross-category insight or action recommendation.
type Suggestion struct {
	Category    string  // Which life category triggered this
	Priority    float64 // 0.0 - 1.0
	Title       string
	Description string
	ActionHint  string // MCP tool call hint
}

// Engine generates cross-category suggestions from current state.
type Engine struct {
	db *sql.DB
}

// NewEngine creates a suggestion engine.
func NewEngine(db *sql.DB) *Engine {
	return &Engine{db: db}
}

// GenerateSuggestions analyzes all categories and returns prioritized suggestions.
func (e *Engine) GenerateSuggestions(ctx context.Context) []Suggestion {
	block := timecontext.CurrentBlock()
	now := time.Now()
	today := now.Format("2006-01-02")
	var suggestions []Suggestion

	// Reply debt check
	var pendingReplies int
	e.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending'").Scan(&pendingReplies)
	if pendingReplies > 3 {
		suggestions = append(suggestions, Suggestion{
			Category:    "personal",
			Priority:    0.8,
			Title:       fmt.Sprintf("Reply debt: %d pending", pendingReplies),
			Description: "Multiple unreplied messages building up. Consider a reply session.",
			ActionHint:  "runmylife_personal(domain=reply_radar, action=scan)",
		})
	}

	// SRS cards due
	var cardsDue int
	e.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM srs_cards WHERE next_review_at <= datetime('now')").Scan(&cardsDue)
	if cardsDue > 0 && (block == timecontext.Morning || block == timecontext.Work) {
		priority := 0.5
		if cardsDue > 20 {
			priority = 0.7
		}
		suggestions = append(suggestions, Suggestion{
			Category:    "growth",
			Priority:    priority,
			Title:       fmt.Sprintf("%d SRS cards due", cardsDue),
			Description: "Spaced repetition cards waiting for review.",
			ActionHint:  "runmylife_growth(domain=srs, action=review)",
		})
	}

	// Partner free evening + nice weather
	if block == timecontext.Work || block == timecontext.Morning {
		// Check if evening is free
		eveningStart := today + "T18:00:00"
		eveningEnd := today + "T22:00:00"
		var calConflicts int
		e.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM calendar_events WHERE start_time < ? AND end_time > ?",
			eveningEnd, eveningStart).Scan(&calConflicts)
		if calConflicts == 0 && (now.Weekday() == time.Friday || now.Weekday() == time.Saturday) {
			suggestions = append(suggestions, Suggestion{
				Category:    "partner",
				Priority:    0.6,
				Title:       "Free evening — date night?",
				Description: "Tonight's evening is clear. Consider planning something together.",
				ActionHint:  "runmylife_partner(domain=calendar, action=suggest)",
			})
		}
	}

	// Studio gap
	var lastStudioSession string
	err := e.db.QueryRowContext(ctx,
		"SELECT started_at FROM studio_sessions ORDER BY started_at DESC LIMIT 1").Scan(&lastStudioSession)
	if err == nil {
		if t, err := time.Parse("2006-01-02T15:04:05", lastStudioSession); err == nil {
			daysSince := int(now.Sub(t).Hours() / 24)
			if daysSince > 14 {
				suggestions = append(suggestions, Suggestion{
					Category:    "studio",
					Priority:    0.5,
					Title:       fmt.Sprintf("Studio gap: %d days", daysSince),
					Description: "It's been a while since your last studio session.",
					ActionHint:  "runmylife_studio(domain=schedule, action=available)",
				})
			}
		}
	}

	// Sleep deficit
	var lastSleepHours float64
	e.db.QueryRowContext(ctx,
		"SELECT sleep_hours FROM mood_log WHERE date = ? AND sleep_hours > 0 ORDER BY created_at DESC LIMIT 1",
		today).Scan(&lastSleepHours)
	if lastSleepHours > 0 && lastSleepHours < 6 {
		var exerciseDone int
		e.db.QueryRowContext(ctx,
			"SELECT exercise_done FROM mood_log WHERE date = ? ORDER BY created_at DESC LIMIT 1",
			today).Scan(&exerciseDone)
		if exerciseDone == 0 {
			suggestions = append(suggestions, Suggestion{
				Category:    "wellness",
				Priority:    0.6,
				Title:       "Low sleep + no exercise",
				Description: fmt.Sprintf("%.1fh sleep — light exercise could help energy.", lastSleepHours),
				ActionHint:  "runmylife_wellness(domain=energy, action=optimize)",
			})
		}
	}

	// Grocery window closing
	if now.Weekday() == time.Wednesday || now.Weekday() == time.Thursday {
		var pendingGroceries int
		e.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM grocery_items WHERE status = 'pending'").Scan(&pendingGroceries)
		if pendingGroceries >= 3 {
			suggestions = append(suggestions, Suggestion{
				Category:    "arthouse",
				Priority:    0.5,
				Title:       fmt.Sprintf("Grocery window: %d items pending", pendingGroceries),
				Description: "Request window closing soon. Time to plan a trip.",
				ActionHint:  "runmylife_arthouse(domain=grocery, action=trip_plan)",
			})
		}
	}

	// At-risk relationships
	var atRiskCount int
	e.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM relationship_health WHERE health_score < 40").Scan(&atRiskCount)
	if atRiskCount > 0 {
		suggestions = append(suggestions, Suggestion{
			Category:    "social",
			Priority:    0.4,
			Title:       fmt.Sprintf("%d relationships at risk", atRiskCount),
			Description: "Some contacts are going stale. Consider reaching out.",
			ActionHint:  "runmylife_social(domain=health, action=at_risk)",
		})
	}

	// Outreach due
	var outreachDue int
	e.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM outreach_reminders WHERE active = 1 AND next_outreach_at <= datetime('now')").
		Scan(&outreachDue)
	if outreachDue > 0 {
		suggestions = append(suggestions, Suggestion{
			Category:    "social",
			Priority:    0.45,
			Title:       fmt.Sprintf("%d outreach reminders due", outreachDue),
			Description: "People you planned to reach out to.",
			ActionHint:  "runmylife_social(domain=outreach, action=due)",
		})
	}

	// No mood logged today
	var moodToday int
	e.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM mood_log WHERE date = ?", today).Scan(&moodToday)
	if moodToday == 0 && block != timecontext.Night {
		suggestions = append(suggestions, Suggestion{
			Category:    "wellness",
			Priority:    0.3,
			Title:       "Mood not logged today",
			Description: "Take a moment to check in with yourself.",
			ActionHint:  "runmylife_wellness(domain=mood, action=log)",
		})
	}

	// Habits incomplete
	var habitsTotal, habitsCompleted int
	e.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habits WHERE archived = 0").Scan(&habitsTotal)
	e.db.QueryRowContext(ctx,
		"SELECT COUNT(DISTINCT habit_id) FROM habit_completions WHERE date(completed_at) = ?",
		today).Scan(&habitsCompleted)
	if habitsTotal > 0 && habitsCompleted < habitsTotal && block == timecontext.Evening {
		suggestions = append(suggestions, Suggestion{
			Category:    "personal",
			Priority:    0.35,
			Title:       fmt.Sprintf("Habits: %d/%d done", habitsCompleted, habitsTotal),
			Description: "Evening is a good time to catch up on remaining habits.",
			ActionHint:  "runmylife_habits(domain=track, action=list)",
		})
	}

	// Sort by priority (highest first)
	sortSuggestions(suggestions)
	return suggestions
}

func sortSuggestions(s []Suggestion) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].Priority > s[j-1].Priority; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
