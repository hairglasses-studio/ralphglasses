// Package briefing provides MCP tools for cross-module daily briefings,
// weekly reviews, and monthly reviews. ADHD-friendly: celebrate wins first.
package briefing

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/adhd"
	"github.com/hairglasses-studio/runmylife/internal/intelligence"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
	"github.com/hairglasses-studio/runmylife/internal/reviews"
	"github.com/hairglasses-studio/runmylife/internal/timecontext"
)

type Module struct{}

func (m *Module) Name() string        { return "briefing" }
func (m *Module) Description() string { return "Cross-module briefings, reviews, and summaries" }

var briefingHints = map[string]string{
	"generate/morning": "Morning briefing with today's agenda, tasks, energy, and ADHD status",
	"generate/evening": "Evening wind-down: celebrate wins, tomorrow prep, mood/gratitude prompt",
	"generate/custom":  "Custom date briefing",
	"review/weekly":    "Weekly review: category metrics, habit streaks, spending, week-over-week deltas",
	"review/monthly":   "Monthly review: trend analysis, wins, subscription audit, patterns",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("briefing").
		Domain("generate", common.ActionRegistry{
			"morning": handleMorningBriefing,
			"evening": handleEveningBriefing,
			"custom":  handleCustomBriefing,
		}).
		Domain("review", common.ActionRegistry{
			"weekly":  handleWeeklyReview,
			"monthly": handleMonthlyReview,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_briefing",
				mcp.WithDescription("Briefing & review gateway.\n\n"+dispatcher.DescribeActionsWithHints(briefingHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: generate, review")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("date", mcp.Description("Date YYYY-MM-DD (default: today)")),
			),
			Handler:     tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:    "briefing",
			Subcategory: "gateway",
			Tags:        []string{"briefing", "summary", "daily", "weekly", "monthly", "review"},
			UseCases:    []string{"Morning briefing", "Evening wind-down", "Weekly review", "Monthly review"},
			Complexity:  tools.ComplexityModerate,
			Timeout:     60 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func handleMorningBriefing(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	dow := time.Now().Format("Monday")
	block := timecontext.CurrentBlock()

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Morning Briefing — %s, %s", dow, today))
	md.KeyValue("Time block", block.Label())

	// --- ADHD Status (check first — may set tone for the whole briefing) ---
	overwhelm, _ := adhd.CheckOverwhelm(ctx, db)
	if overwhelm != nil && overwhelm.TriageActivated {
		md.Section("ADHD: Triage Mode Active")
		md.Text(fmt.Sprintf("Overwhelm score: %.0f%% — focus on top 3 tasks only today.", overwhelm.CompositeScore*100))
		triageTasks, _ := adhd.GetTopTriageTasks(ctx, db)
		if len(triageTasks) > 0 {
			md.List(triageTasks)
		}
	}

	// --- Energy estimate ---
	energy := adhd.EstimateCurrentEnergy(ctx, db)
	md.KeyValue("Estimated energy", fmt.Sprintf("%d/10 (%s)", energy.Level, energy.Source))

	// --- Reply Radar ---
	var pendingReplies, urgentReplies int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending'").Scan(&pendingReplies)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending' AND urgency_score >= 0.7").Scan(&urgentReplies)
	if pendingReplies > 0 {
		md.Section(fmt.Sprintf("Reply Radar (%d pending)", pendingReplies))
		if urgentReplies > 0 {
			md.KeyValue("URGENT", fmt.Sprintf("%d", urgentReplies))
		}
		replyRows, err := db.QueryContext(ctx,
			`SELECT contact_name, urgency_score, message_preview, channel
			 FROM reply_tracker WHERE status = 'pending'
			 ORDER BY urgency_score DESC LIMIT 3`)
		if err == nil {
			defer replyRows.Close()
			var replyList []string
			for replyRows.Next() {
				var name, preview, channel string
				var urgency float64
				if replyRows.Scan(&name, &urgency, &preview, &channel) == nil {
					tag := ""
					if urgency >= 0.7 {
						tag = " [URGENT]"
					} else if urgency < 0.3 {
						tag = " [LOW]"
					}
					replyList = append(replyList, fmt.Sprintf("%s%s (%s) — %s", name, tag, channel, common.TruncateWords(preview, 40)))
				}
			}
			if len(replyList) > 0 {
				md.List(replyList)
			}
		}
	}

	// --- Calendar ---
	rows, err := db.QueryContext(ctx,
		`SELECT summary, start_time, end_time, location FROM calendar_events
		 WHERE start_time >= ? AND start_time < ? ORDER BY start_time ASC`, today, tomorrow)
	if err == nil {
		defer rows.Close()
		var events [][]string
		for rows.Next() {
			var summary, startTime, endTime, location string
			if rows.Scan(&summary, &startTime, &endTime, &location) == nil {
				loc := ""
				if location != "" {
					loc = " @ " + location
				}
				events = append(events, []string{formatTime(startTime), formatTime(endTime), summary + loc})
			}
		}
		md.Section("Calendar")
		if len(events) == 0 {
			md.Text("No events scheduled.")
		} else {
			md.Table([]string{"Start", "End", "Event"}, events)
		}
	}

	// --- Tasks (top 5, energy-matched if possible) ---
	var overdueTasks, dueTodayTasks, openTasks int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 0 AND due_date < ? AND due_date != ''", today).Scan(&overdueTasks)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 0 AND due_date = ?", today).Scan(&dueTodayTasks)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 0").Scan(&openTasks)
	md.Section(fmt.Sprintf("Tasks (%d due today, %d open)", dueTodayTasks, openTasks))
	if overdueTasks > 0 {
		md.KeyValue("OVERDUE", fmt.Sprintf("%d", overdueTasks))
	}

	taskRows, err := db.QueryContext(ctx,
		`SELECT title, due_date, priority FROM tasks
		 WHERE completed = 0 AND due_date <= ? AND due_date != ''
		 ORDER BY due_date ASC, priority DESC LIMIT 5`, today)
	if err == nil {
		defer taskRows.Close()
		var taskList [][]string
		for taskRows.Next() {
			var title, dueDate string
			var priority int
			if taskRows.Scan(&title, &dueDate, &priority) == nil {
				taskList = append(taskList, []string{title, dueDate, fmt.Sprintf("%d", priority)})
			}
		}
		if len(taskList) > 0 {
			md.Table([]string{"Task", "Due", "Priority"}, taskList)
		}
	}

	// --- Growth: Brain Warm-up ---
	var cardsDue int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM srs_cards WHERE next_review_at <= datetime('now')").Scan(&cardsDue)
	var labStreak int
	for i := 0; i < 365; i++ {
		dateStr := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		var count int
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lab_exercises WHERE status = 'completed' AND date(completed_at) = ?", dateStr).Scan(&count)
		if count == 0 {
			break
		}
		labStreak++
	}
	md.Section("Brain Warm-up")
	md.KeyValue("SRS cards due", fmt.Sprintf("%d", cardsDue))
	md.KeyValue("Lab streak", fmt.Sprintf("%d day(s)", labStreak))

	// --- Wellness ---
	var moodScore, energyLevel int
	var sleepHours float64
	var habitsCompleted, totalHabits int
	err = db.QueryRowContext(ctx,
		"SELECT mood_score, energy_level, sleep_hours FROM mood_log WHERE date = ? ORDER BY created_at DESC LIMIT 1",
		today).Scan(&moodScore, &energyLevel, &sleepHours)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habit_completions WHERE date(completed_at) = ?", today).Scan(&habitsCompleted)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habits WHERE archived = 0").Scan(&totalHabits)

	md.Section("Wellness")
	if err == nil {
		md.KeyValue("Mood", fmt.Sprintf("%d/10", moodScore))
		md.KeyValue("Energy", fmt.Sprintf("%d/10", energyLevel))
		if sleepHours > 0 {
			md.KeyValue("Sleep", fmt.Sprintf("%.1fh", sleepHours))
		}
	} else {
		var fitbitSleep int
		if db.QueryRowContext(ctx, "SELECT duration_ms FROM fitness_sleep WHERE date = ? ORDER BY start_time DESC LIMIT 1", today).Scan(&fitbitSleep) == nil {
			md.KeyValue("Sleep (Fitbit)", fmt.Sprintf("%.1fh", float64(fitbitSleep)/3600000))
		} else {
			md.Text("Mood not logged yet — consider logging now.")
		}
	}
	md.KeyValue("Habits", fmt.Sprintf("%d/%d", habitsCompleted, totalHabits))

	// --- Weather ---
	var weatherDesc string
	var tempHigh, tempLow float64
	err = db.QueryRowContext(ctx,
		"SELECT description, temperature_high, temperature_low FROM weather_cache WHERE date = ? ORDER BY fetched_at DESC LIMIT 1",
		today).Scan(&weatherDesc, &tempHigh, &tempLow)
	if err == nil {
		md.Section("Weather")
		md.Text(fmt.Sprintf("%s — High %.0f / Low %.0f", weatherDesc, tempHigh, tempLow))
	}

	// --- ArtHouse ---
	var pendingGroceries, openMaintenance int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM grocery_items WHERE status = 'pending'").Scan(&pendingGroceries)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM maintenance_requests WHERE status IN ('open', 'in_progress')").Scan(&openMaintenance)

	var choresDue int
	weekOf := mondayOf(time.Now()).Format("2006-01-02")
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM chore_assignments WHERE week_of = ? AND completed = 0 AND member_id = 'member-mitch'",
		weekOf).Scan(&choresDue)

	if pendingGroceries > 0 || openMaintenance > 0 || choresDue > 0 {
		md.Section("House")
		if choresDue > 0 {
			md.KeyValue("Your chores due", fmt.Sprintf("%d", choresDue))
		}
		if pendingGroceries > 0 {
			md.KeyValue("Grocery items pending", fmt.Sprintf("%d", pendingGroceries))
		}
		if openMaintenance > 0 {
			md.KeyValue("Open maintenance", fmt.Sprintf("%d", openMaintenance))
		}
	}

	// --- Email ---
	var untriaged int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM gmail_messages WHERE triaged = 0").Scan(&untriaged)
	md.Section("Email")
	md.KeyValue("Untriaged", fmt.Sprintf("%d", untriaged))

	// --- Finances ---
	monthStart := time.Now().Format("2006-01") + "-01"
	var monthSpend float64
	db.QueryRowContext(ctx, "SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE date >= ? AND type = 'expense'", monthStart).Scan(&monthSpend)
	md.Section("Finances")
	md.KeyValue("Month-to-date spend", fmt.Sprintf("$%.2f", monthSpend))

	// --- Intelligence Suggestions ---
	engine := intelligence.NewEngine(db)
	suggestions := engine.GenerateSuggestions(ctx)
	if len(suggestions) > 0 {
		md.Section("Suggestions")
		for i, s := range suggestions {
			if i >= 5 {
				break
			}
			md.Text(fmt.Sprintf("- [%s] %s — %s", s.Category, s.Title, s.Description))
		}
	}

	return tools.TextResult(md.String()), nil
}

func handleEveningBriefing(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	dayAfter := time.Now().AddDate(0, 0, 2).Format("2006-01-02")
	dow := time.Now().Format("Monday")

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Evening Wind-Down — %s, %s", dow, today))

	// --- Wins first (ADHD-friendly: lead with positives) ---
	md.Section("Today's Wins")

	var tasksCompleted int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 1 AND updated_at >= ?", today).Scan(&tasksCompleted)
	var habitsCompleted, totalHabits int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habit_completions WHERE completed_at >= ?", today).Scan(&habitsCompleted)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habits WHERE archived = 0").Scan(&totalHabits)
	var emailsTriaged int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM gmail_messages WHERE triaged = 1 AND updated_at >= ?", today).Scan(&emailsTriaged)
	var repliesResolved int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reply_tracker WHERE status = 'resolved' AND updated_at >= ?", today).Scan(&repliesResolved)
	var focusMin int
	db.QueryRowContext(ctx, "SELECT COALESCE(SUM(duration_minutes), 0) FROM focus_sessions WHERE date(started_at) = ?", today).Scan(&focusMin)

	var wins []string
	if tasksCompleted > 0 {
		wins = append(wins, fmt.Sprintf("Completed %d task(s)", tasksCompleted))
	}
	if habitsCompleted > 0 {
		wins = append(wins, fmt.Sprintf("Completed %d/%d habits", habitsCompleted, totalHabits))
	}
	if emailsTriaged > 0 {
		wins = append(wins, fmt.Sprintf("Triaged %d emails", emailsTriaged))
	}
	if repliesResolved > 0 {
		wins = append(wins, fmt.Sprintf("Replied to %d messages", repliesResolved))
	}
	if focusMin > 0 {
		wins = append(wins, fmt.Sprintf("Focused for %d minutes", focusMin))
	}
	if len(wins) == 0 {
		wins = append(wins, "Rest days count too. Tomorrow's a fresh start.")
	}
	md.List(wins)

	// "Good enough day" assessment
	habitRate := 0.0
	if totalHabits > 0 {
		habitRate = float64(habitsCompleted) / float64(totalHabits)
	}
	if tasksCompleted >= 1 && habitRate >= 0.6 {
		md.Text("**Good enough day? YES.** You showed up and got things done.")
	} else if tasksCompleted >= 1 || habitRate >= 0.5 {
		md.Text("**Good enough day? Almost.** Progress happened — that counts.")
	}

	// --- Today's spending ---
	var todaySpend float64
	db.QueryRowContext(ctx, "SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE date = ? AND type = 'expense'", today).Scan(&todaySpend)
	if todaySpend > 0 {
		md.KeyValue("Spent today", fmt.Sprintf("$%.2f", todaySpend))
	}

	// --- Tomorrow preview ---
	tomorrowRows, err := db.QueryContext(ctx,
		`SELECT summary, start_time FROM calendar_events
		 WHERE start_time >= ? AND start_time < ? ORDER BY start_time ASC LIMIT 5`, tomorrow, dayAfter)
	if err == nil {
		defer tomorrowRows.Close()
		var events []string
		for tomorrowRows.Next() {
			var summary, startTime string
			if tomorrowRows.Scan(&summary, &startTime) == nil {
				events = append(events, fmt.Sprintf("%s — %s", formatTime(startTime), summary))
			}
		}
		md.Section("Tomorrow Preview")
		if len(events) == 0 {
			md.Text("No events scheduled.")
		} else {
			md.List(events)
		}
	}

	// --- Outstanding items ---
	var openTasks int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 0").Scan(&openTasks)
	var overdueTasks int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 0 AND due_date <= ? AND due_date != ''", today).Scan(&overdueTasks)
	var pendingReplies int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending'").Scan(&pendingReplies)

	md.Section("Outstanding")
	md.KeyValue("Open tasks", fmt.Sprintf("%d", openTasks))
	if overdueTasks > 0 {
		md.KeyValue("Overdue", fmt.Sprintf("%d", overdueTasks))
	}
	if pendingReplies > 0 {
		md.KeyValue("Pending replies", fmt.Sprintf("%d", pendingReplies))
	}

	// --- Prompts ---
	md.Section("Evening Prompts")
	// Check if mood was logged today
	var moodLogged int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM mood_log WHERE date = ?", today).Scan(&moodLogged)
	if moodLogged == 0 {
		md.Text("- **Log your mood** — How are you feeling (1-10)? What drained or energized you?")
	}
	md.Text("- **Gratitude** — What's one thing you're grateful for today?")
	md.Text("- **Tomorrow's top 3** — What are the 3 most important things for tomorrow?")

	return tools.TextResult(md.String()), nil
}

func handleCustomBriefing(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	date := common.GetStringParam(req, "date", time.Now().Format("2006-01-02"))

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return common.CodedErrorResultf(common.ErrInvalidParam, "invalid date format: %s", date), nil
	}
	nextDay := parsed.AddDate(0, 0, 1).Format("2006-01-02")
	dow := parsed.Format("Monday")

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Briefing — %s, %s", dow, date))

	rows, err := db.QueryContext(ctx,
		`SELECT summary, start_time, end_time FROM calendar_events
		 WHERE start_time >= ? AND start_time < ? ORDER BY start_time ASC`, date, nextDay)
	if err == nil {
		defer rows.Close()
		var events [][]string
		for rows.Next() {
			var summary, startTime, endTime string
			if rows.Scan(&summary, &startTime, &endTime) == nil {
				events = append(events, []string{formatTime(startTime), formatTime(endTime), summary})
			}
		}
		md.Section("Calendar")
		if len(events) == 0 {
			md.Text("No events.")
		} else {
			md.Table([]string{"Start", "End", "Event"}, events)
		}
	}

	var tasksCompleted int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 1 AND updated_at >= ? AND updated_at < ?", date, nextDay).Scan(&tasksCompleted)
	md.Section("Tasks")
	md.KeyValue("Completed", fmt.Sprintf("%d", tasksCompleted))

	var habitsCompleted int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habit_completions WHERE completed_at >= ? AND completed_at < ?", date, nextDay).Scan(&habitsCompleted)
	md.KeyValue("Habits completed", fmt.Sprintf("%d", habitsCompleted))

	var spend float64
	db.QueryRowContext(ctx, "SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE date = ? AND type = 'expense'", date).Scan(&spend)
	md.Section("Finances")
	md.KeyValue("Spent", fmt.Sprintf("$%.2f", spend))

	return tools.TextResult(md.String()), nil
}

func handleWeeklyReview(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dateStr := common.GetStringParam(req, "date", time.Now().Format("2006-01-02"))
	dateRef, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return common.CodedErrorResultf(common.ErrInvalidParam, "invalid date: %s", dateStr), nil
	}

	database, errDB := common.OpenDB()
	if errDB != nil {
		return common.CodedErrorResult(common.ErrClientInit, errDB), nil
	}
	defer database.Close()
	db := database.SqlDB()

	snap, err := reviews.CaptureWeekly(ctx, db, dateRef)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	prev, _ := reviews.LoadPreviousWeekly(ctx, db, snap.WeekOf)

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Weekly Review — week of %s", snap.WeekOf))

	// --- Wins first ---
	md.Section("Wins")
	var wins []string
	if snap.TasksCompleted > 0 {
		wins = append(wins, fmt.Sprintf("Completed %d tasks", snap.TasksCompleted))
	}
	if snap.HabitsCompleted > 0 {
		wins = append(wins, fmt.Sprintf("%d habit completions (%.0f%% rate)", snap.HabitsCompleted, snap.HabitRate*100))
	}
	if snap.GoodEnoughDays > 0 {
		wins = append(wins, fmt.Sprintf("%d good-enough days", snap.GoodEnoughDays))
	}
	if snap.FocusMinutes > 0 {
		wins = append(wins, fmt.Sprintf("%d minutes of focused work", snap.FocusMinutes))
	}
	if snap.SocialOutreaches > 0 {
		wins = append(wins, fmt.Sprintf("Reached out to %d people", snap.SocialOutreaches))
	}
	if len(wins) == 0 {
		wins = append(wins, "You made it through the week. That's a win.")
	}
	md.List(wins)

	// --- Productivity ---
	md.Section("Productivity")
	if prev != nil {
		md.KeyValue("Tasks completed", fmt.Sprintf("%d (%s vs last week)", snap.TasksCompleted, reviews.FormatDelta(snap.TasksCompleted, prev.TasksCompleted)))
		md.KeyValue("Tasks created", fmt.Sprintf("%d (%s)", snap.TasksCreated, reviews.FormatDelta(snap.TasksCreated, prev.TasksCreated)))
		md.KeyValue("Focus time", fmt.Sprintf("%d min (%s)", snap.FocusMinutes, reviews.FormatDelta(snap.FocusMinutes, prev.FocusMinutes)))
	} else {
		md.KeyValue("Tasks completed", fmt.Sprintf("%d", snap.TasksCompleted))
		md.KeyValue("Tasks created", fmt.Sprintf("%d", snap.TasksCreated))
		md.KeyValue("Focus time", fmt.Sprintf("%d min", snap.FocusMinutes))
	}
	md.KeyValue("Calendar events", fmt.Sprintf("%d", snap.CalendarEvents))

	// --- Habits ---
	md.Section("Habits")
	md.KeyValue("Completions", fmt.Sprintf("%d / %d possible (%.0f%%)", snap.HabitsCompleted, snap.HabitsTotal*7, snap.HabitRate*100))
	md.KeyValue("Good-enough days", fmt.Sprintf("%d / 7", snap.GoodEnoughDays))

	// --- Wellness ---
	md.Section("Wellness")
	if snap.MoodAvg > 0 {
		if prev != nil && prev.MoodAvg > 0 {
			md.KeyValue("Mood avg", fmt.Sprintf("%.1f/10 (%s)", snap.MoodAvg, reviews.FormatDeltaF(snap.MoodAvg, prev.MoodAvg)))
			md.KeyValue("Energy avg", fmt.Sprintf("%.1f/10 (%s)", snap.EnergyAvg, reviews.FormatDeltaF(snap.EnergyAvg, prev.EnergyAvg)))
		} else {
			md.KeyValue("Mood avg", fmt.Sprintf("%.1f/10", snap.MoodAvg))
			md.KeyValue("Energy avg", fmt.Sprintf("%.1f/10", snap.EnergyAvg))
		}
		if snap.SleepAvgHours > 0 {
			md.KeyValue("Sleep avg", fmt.Sprintf("%.1fh", snap.SleepAvgHours))
		}
	} else {
		md.Text("No mood data logged this week — consider logging daily.")
	}
	md.KeyValue("Overwhelm days", fmt.Sprintf("%d / 7", snap.OverwhelmDays))

	// --- Communication ---
	md.Section("Communication")
	md.KeyValue("Replies sent", fmt.Sprintf("%d", snap.RepliesSent))
	md.KeyValue("Emails triaged", fmt.Sprintf("%d", snap.EmailsTriaged))
	if snap.RepliesOverdue > 0 {
		md.KeyValue("Still overdue", fmt.Sprintf("%d", snap.RepliesOverdue))
	}

	// --- Finances ---
	md.Section("Finances")
	md.KeyValue("Week spend", fmt.Sprintf("$%.2f", snap.SpendTotal))
	if prev != nil {
		md.KeyValue("vs last week", fmt.Sprintf("$%.2f (%s)", prev.SpendTotal, reviews.FormatDeltaF(snap.SpendTotal, prev.SpendTotal)))
	}

	// --- Growth ---
	if snap.SRSReviews > 0 || snap.JournalEntries > 0 {
		md.Section("Growth")
		md.KeyValue("SRS reviews", fmt.Sprintf("%d", snap.SRSReviews))
		md.KeyValue("Journal entries", fmt.Sprintf("%d", snap.JournalEntries))
	}

	// --- Social ---
	if snap.SocialOutreaches > 0 {
		md.Section("Social")
		md.KeyValue("Outreaches completed", fmt.Sprintf("%d", snap.SocialOutreaches))
	}

	return tools.TextResult(md.String()), nil
}

func handleMonthlyReview(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dateStr := common.GetStringParam(req, "date", time.Now().Format("2006-01-02"))
	dateRef, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return common.CodedErrorResultf(common.ErrInvalidParam, "invalid date: %s", dateStr), nil
	}

	database, errDB := common.OpenDB()
	if errDB != nil {
		return common.CodedErrorResult(common.ErrClientInit, errDB), nil
	}
	defer database.Close()
	db := database.SqlDB()

	snap, err := reviews.CaptureMonthly(ctx, db, dateRef)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Monthly Review — %s", snap.Month))

	// --- Wins first ---
	md.Section("Wins")
	if len(snap.Wins) > 0 {
		md.List(snap.Wins)
	} else {
		md.Text("You survived the month. That's baseline. Everything else is bonus.")
	}

	// --- Overview ---
	md.Section("Overview")
	md.KeyValue("Tasks completed", fmt.Sprintf("%d", snap.TasksCompleted))
	md.KeyValue("Habit rate avg", fmt.Sprintf("%.0f%%", snap.HabitRateAvg*100))
	md.KeyValue("Longest habit streak", fmt.Sprintf("%d days", snap.HabitStreakMax))
	md.KeyValue("Good-enough days", fmt.Sprintf("%d", snap.GoodEnoughDays))
	md.KeyValue("Overwhelm days", fmt.Sprintf("%d", snap.OverwhelmDays))

	// --- Trends ---
	md.Section("Trends")
	md.KeyValue("Mood", snap.MoodTrend)
	md.KeyValue("Energy", snap.EnergyTrend)

	// --- Finances ---
	md.Section("Finances")
	md.KeyValue("Total spend", fmt.Sprintf("$%.2f", snap.SpendTotal))
	if snap.SpendVsBudget > 0 {
		pct := snap.SpendVsBudget * 100
		verdict := "on track"
		if pct > 100 {
			verdict = "over budget"
		} else if pct > 90 {
			verdict = "close to limit"
		}
		md.KeyValue("vs budget", fmt.Sprintf("%.0f%% (%s)", pct, verdict))
	}
	if snap.SubscriptionSpend > 0 {
		md.KeyValue("Subscription burden", fmt.Sprintf("$%.2f/mo", snap.SubscriptionSpend))
	}

	// --- Top categories ---
	if len(snap.TopCategories) > 0 {
		md.Section("Most Active Categories")
		md.List(snap.TopCategories)
	}

	// --- Subscription audit prompt ---
	md.Section("Action Items")
	var activeSubs int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscriptions WHERE status = 'active'").Scan(&activeSubs)
	if activeSubs > 0 {
		md.Text(fmt.Sprintf("- **Subscription audit**: %d active subscriptions ($%.2f/mo). Review for unused services.", activeSubs, snap.SubscriptionSpend))
	}
	if snap.OverwhelmDays > 5 {
		md.Text("- **Overwhelm pattern**: Triage activated frequently. Consider reducing commitments or increasing delegation.")
	}

	return tools.TextResult(md.String()), nil
}

func mondayOf(t time.Time) time.Time {
	offset := int(t.Weekday()) - 1
	if offset < 0 {
		offset = 6
	}
	return t.AddDate(0, 0, -offset)
}

func formatTime(ts string) string {
	for _, layout := range []string{
		"2006-01-02T15:04:05-07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t.Format("3:04 PM")
		}
	}
	return ts
}

// loadMonthlyWins loads specific high-priority task titles completed in the month.
func loadMonthlyWins(ctx context.Context, db *sql.DB, firstStr, lastStr string) []string {
	rows, err := db.QueryContext(ctx,
		`SELECT title FROM tasks WHERE completed = 1 AND priority >= 3
		 AND updated_at >= ? AND updated_at < ? ORDER BY priority DESC LIMIT 5`,
		firstStr, lastStr)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var taskWins []string
	for rows.Next() {
		var title string
		if rows.Scan(&title) == nil {
			taskWins = append(taskWins, title)
		}
	}
	return taskWins
}

// loadSubscriptionDetails returns active subscription names for audit display.
func loadSubscriptionDetails(ctx context.Context, db *sql.DB) []string {
	rows, err := db.QueryContext(ctx,
		`SELECT name, amount, billing_cycle FROM subscriptions WHERE status = 'active' ORDER BY amount DESC LIMIT 10`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var subs []string
	for rows.Next() {
		var name, cycle string
		var amount float64
		if rows.Scan(&name, &amount, &cycle) == nil {
			subs = append(subs, fmt.Sprintf("%s ($%.2f/%s)", name, amount, cycle))
		}
	}
	return subs
}
