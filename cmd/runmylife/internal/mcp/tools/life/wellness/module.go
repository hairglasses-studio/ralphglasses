// Package wellness provides the Wellness life category MCP tool.
// Covers mood tracking, energy optimization, and mindfulness — unified with fitness/sleep/habits.
package wellness

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

type Module struct{}

func (m *Module) Name() string        { return "wellness" }
func (m *Module) Description() string { return "Wellness: mood, energy, mindfulness — unified health view" }

var wellnessHints = map[string]string{
	"overview/dashboard":      "Unified wellness dashboard (fitness + sleep + mood + habits)",
	"mood/log":                "Log current mood and energy",
	"mood/history":            "Mood history with trends",
	"mood/correlate":          "Mood correlations with sleep, exercise, weather",
	"energy/level":            "Current energy estimate",
	"energy/optimize":         "Energy optimization suggestions",
	"mindfulness/prompt":      "Daily reflection prompt",
	"mindfulness/gratitude":   "Log gratitude entry",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("wellness").
		Domain("overview", common.ActionRegistry{
			"dashboard": handleOverviewDashboard,
		}).
		Domain("mood", common.ActionRegistry{
			"log":       handleMoodLog,
			"history":   handleMoodHistory,
			"correlate": handleMoodCorrelate,
		}).
		Domain("energy", common.ActionRegistry{
			"level":    handleEnergyLevel,
			"optimize": handleEnergyOptimize,
		}).
		Domain("mindfulness", common.ActionRegistry{
			"prompt":    handleMindfulnessPrompt,
			"gratitude": handleMindfulnessGratitude,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_wellness",
				mcp.WithDescription("Wellness gateway.\n\n"+
					dispatcher.DescribeActionsWithHints(wellnessHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: overview, mood, energy, mindfulness")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				// Mood params
				mcp.WithNumber("mood_score", mcp.Description("Mood score 1-10")),
				mcp.WithNumber("energy_level", mcp.Description("Energy level 1-10")),
				mcp.WithNumber("anxiety_level", mcp.Description("Anxiety level 1-10")),
				mcp.WithNumber("sleep_hours", mcp.Description("Hours of sleep last night")),
				mcp.WithBoolean("exercise_done", mcp.Description("Did you exercise today?")),
				mcp.WithString("notes", mcp.Description("Freeform notes")),
				mcp.WithString("tags", mcp.Description("Comma-separated tags")),
				// Time params
				mcp.WithString("date", mcp.Description("Date (YYYY-MM-DD)")),
				mcp.WithNumber("days", mcp.Description("Number of days to look back")),
			),
			Handler:     tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:    "life",
			Subcategory: "wellness",
			Tags:        []string{"wellness", "mood", "energy", "mindfulness", "health"},
			Complexity:  tools.ComplexityModerate,
			IsWrite:     true,
			Timeout:     30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

// --- Overview ---

func handleOverviewDashboard(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	today := time.Now().Format("2006-01-02")
	md := common.NewMarkdownBuilder().Title("Wellness Dashboard")

	// Today's mood
	var moodScore, energyLevel, anxietyLevel int
	var sleepHours float64
	var exerciseDone int
	err = db.QueryRowContext(ctx,
		"SELECT mood_score, energy_level, anxiety_level, sleep_hours, exercise_done FROM mood_log WHERE date = ? ORDER BY created_at DESC LIMIT 1",
		today).Scan(&moodScore, &energyLevel, &anxietyLevel, &sleepHours, &exerciseDone)
	if err == nil {
		md.Section("Today's Check-in")
		md.KeyValue("Mood", fmt.Sprintf("%d/10", moodScore))
		md.KeyValue("Energy", fmt.Sprintf("%d/10", energyLevel))
		md.KeyValue("Anxiety", fmt.Sprintf("%d/10", anxietyLevel))
		if sleepHours > 0 {
			md.KeyValue("Sleep", fmt.Sprintf("%.1fh", sleepHours))
		}
		md.KeyValue("Exercise", boolToYesNo(exerciseDone == 1))
	} else {
		md.Section("Today's Check-in")
		md.Text("No mood logged yet. Use `mood/log` to check in.")
	}

	// Fitbit data (from fitness module tables)
	var steps, calories int
	err = db.QueryRowContext(ctx, "SELECT steps, calories FROM fitness_daily_stats WHERE date = ?", today).
		Scan(&steps, &calories)
	if err == nil {
		md.Section("Fitness Today")
		md.KeyValue("Steps", fmt.Sprintf("%d", steps))
		md.KeyValue("Calories", fmt.Sprintf("%d", calories))
	}

	// Last night's sleep
	var sleepDuration int
	var sleepEfficiency int
	err = db.QueryRowContext(ctx,
		"SELECT duration_ms, efficiency FROM fitness_sleep WHERE date = ? ORDER BY start_time DESC LIMIT 1",
		today).Scan(&sleepDuration, &sleepEfficiency)
	if err == nil {
		md.Section("Sleep (Fitbit)")
		md.KeyValue("Duration", fmt.Sprintf("%.1fh", float64(sleepDuration)/3600000))
		md.KeyValue("Efficiency", fmt.Sprintf("%d%%", sleepEfficiency))
	}

	// Habit completions today
	var habitsTotal, habitsCompleted int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habits WHERE archived = 0").Scan(&habitsTotal)
	db.QueryRowContext(ctx,
		"SELECT COUNT(DISTINCT habit_id) FROM habit_completions WHERE date(completed_at) = ?",
		today).Scan(&habitsCompleted)
	md.Section("Habits")
	md.KeyValue("Today", fmt.Sprintf("%d/%d", habitsCompleted, habitsTotal))

	// Weekly mood average
	weekAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	var avgMood, avgEnergy float64
	db.QueryRowContext(ctx,
		"SELECT COALESCE(AVG(mood_score), 0), COALESCE(AVG(energy_level), 0) FROM mood_log WHERE date >= ?",
		weekAgo).Scan(&avgMood, &avgEnergy)
	if avgMood > 0 {
		md.Section("7-Day Averages")
		md.KeyValue("Mood", fmt.Sprintf("%.1f/10", avgMood))
		md.KeyValue("Energy", fmt.Sprintf("%.1f/10", avgEnergy))
	}

	// Wellness score (composite)
	score := calculateWellnessScore(moodScore, energyLevel, anxietyLevel, sleepHours, exerciseDone == 1, habitsCompleted, habitsTotal)
	if score > 0 {
		md.Section("Wellness Score")
		md.KeyValue("Today", fmt.Sprintf("%.0f/100", score))
	}

	return tools.TextResult(md.String()), nil
}

// --- Mood Handlers ---

func handleMoodLog(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	moodScore := common.GetIntParam(req, "mood_score", 0)
	if moodScore < 1 || moodScore > 10 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "mood_score required (1-10)"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	now := time.Now()
	date := common.GetStringParam(req, "date", now.Format("2006-01-02"))
	energyLevel := common.GetIntParam(req, "energy_level", 5)
	anxietyLevel := common.GetIntParam(req, "anxiety_level", 1)
	sleepHours := common.GetFloatParam(req, "sleep_hours", 0)
	exerciseDone := common.GetBoolParam(req, "exercise_done", false)
	notes := common.GetStringParam(req, "notes", "")
	tags := common.GetStringParam(req, "tags", "")

	// Determine time of day
	hour := now.Hour()
	timeOfDay := "morning"
	switch {
	case hour >= 12 && hour < 17:
		timeOfDay = "afternoon"
	case hour >= 17 && hour < 21:
		timeOfDay = "evening"
	case hour >= 21 || hour < 6:
		timeOfDay = "night"
	}

	exerciseInt := 0
	if exerciseDone {
		exerciseInt = 1
	}

	result, err := db.ExecContext(ctx,
		`INSERT INTO mood_log (date, time_of_day, mood_score, energy_level, anxiety_level, sleep_hours, exercise_done, notes, tags)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		date, timeOfDay, moodScore, energyLevel, anxietyLevel, sleepHours, exerciseInt, notes, tags)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()

	md := common.NewMarkdownBuilder().Title("Mood Logged")
	md.KeyValue("ID", fmt.Sprintf("%d", id))
	md.KeyValue("Mood", fmt.Sprintf("%d/10", moodScore))
	md.KeyValue("Energy", fmt.Sprintf("%d/10", energyLevel))
	md.KeyValue("Time", timeOfDay)
	return tools.TextResult(md.String()), nil
}

func handleMoodHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	days := common.GetIntParam(req, "days", 14)
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	rows, err := db.QueryContext(ctx, `
		SELECT date, time_of_day, mood_score, energy_level, anxiety_level, sleep_hours, exercise_done
		FROM mood_log WHERE date >= ? ORDER BY date DESC, created_at DESC`,
		since)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Mood History (%d days)", days))
	var tableRows [][]string
	for rows.Next() {
		var date, timeOfDay string
		var mood, energy, anxiety, exercise int
		var sleep float64
		if rows.Scan(&date, &timeOfDay, &mood, &energy, &anxiety, &sleep, &exercise) == nil {
			ex := "No"
			if exercise == 1 {
				ex = "Yes"
			}
			tableRows = append(tableRows, []string{
				date, timeOfDay, fmt.Sprintf("%d", mood), fmt.Sprintf("%d", energy),
				fmt.Sprintf("%d", anxiety), fmt.Sprintf("%.1f", sleep), ex,
			})
		}
	}
	if len(tableRows) == 0 {
		md.EmptyList("mood entries")
	} else {
		md.Table([]string{"Date", "Time", "Mood", "Energy", "Anxiety", "Sleep", "Exercise"}, tableRows)

		// Averages
		var avgMood, avgEnergy, avgAnxiety, avgSleep float64
		db.QueryRowContext(ctx,
			"SELECT AVG(mood_score), AVG(energy_level), AVG(anxiety_level), AVG(CASE WHEN sleep_hours > 0 THEN sleep_hours END) FROM mood_log WHERE date >= ?",
			since).Scan(&avgMood, &avgEnergy, &avgAnxiety, &avgSleep)
		md.Section("Averages")
		md.KeyValue("Mood", fmt.Sprintf("%.1f", avgMood))
		md.KeyValue("Energy", fmt.Sprintf("%.1f", avgEnergy))
		md.KeyValue("Anxiety", fmt.Sprintf("%.1f", avgAnxiety))
		if avgSleep > 0 {
			md.KeyValue("Sleep", fmt.Sprintf("%.1fh", avgSleep))
		}
	}
	return tools.TextResult(md.String()), nil
}

func handleMoodCorrelate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	days := common.GetIntParam(req, "days", 30)
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Mood Correlations (%d days)", days))

	// Sleep vs mood
	var avgMoodGoodSleep, avgMoodBadSleep float64
	db.QueryRowContext(ctx,
		"SELECT COALESCE(AVG(mood_score), 0) FROM mood_log WHERE sleep_hours >= 7 AND date >= ?", since).Scan(&avgMoodGoodSleep)
	db.QueryRowContext(ctx,
		"SELECT COALESCE(AVG(mood_score), 0) FROM mood_log WHERE sleep_hours > 0 AND sleep_hours < 7 AND date >= ?", since).Scan(&avgMoodBadSleep)
	if avgMoodGoodSleep > 0 || avgMoodBadSleep > 0 {
		md.Section("Sleep & Mood")
		md.KeyValue("Mood with 7+ hrs sleep", fmt.Sprintf("%.1f", avgMoodGoodSleep))
		md.KeyValue("Mood with <7 hrs sleep", fmt.Sprintf("%.1f", avgMoodBadSleep))
		diff := avgMoodGoodSleep - avgMoodBadSleep
		if diff > 0.5 {
			md.Text(fmt.Sprintf("Good sleep correlates with +%.1f mood points.", diff))
		}
	}

	// Exercise vs mood
	var avgMoodExercise, avgMoodNoExercise float64
	db.QueryRowContext(ctx,
		"SELECT COALESCE(AVG(mood_score), 0) FROM mood_log WHERE exercise_done = 1 AND date >= ?", since).Scan(&avgMoodExercise)
	db.QueryRowContext(ctx,
		"SELECT COALESCE(AVG(mood_score), 0) FROM mood_log WHERE exercise_done = 0 AND date >= ?", since).Scan(&avgMoodNoExercise)
	if avgMoodExercise > 0 || avgMoodNoExercise > 0 {
		md.Section("Exercise & Mood")
		md.KeyValue("Mood with exercise", fmt.Sprintf("%.1f", avgMoodExercise))
		md.KeyValue("Mood without exercise", fmt.Sprintf("%.1f", avgMoodNoExercise))
		diff := avgMoodExercise - avgMoodNoExercise
		if diff > 0.5 {
			md.Text(fmt.Sprintf("Exercise correlates with +%.1f mood points.", diff))
		}
	}

	// Time of day patterns
	todRows, err := db.QueryContext(ctx, `
		SELECT time_of_day, AVG(mood_score), AVG(energy_level), COUNT(*)
		FROM mood_log WHERE date >= ?
		GROUP BY time_of_day ORDER BY AVG(mood_score) DESC`, since)
	if err == nil {
		defer todRows.Close()
		var todTable [][]string
		for todRows.Next() {
			var tod string
			var avgMood, avgEnergy float64
			var count int
			if todRows.Scan(&tod, &avgMood, &avgEnergy, &count) == nil {
				todTable = append(todTable, []string{
					tod, fmt.Sprintf("%.1f", avgMood), fmt.Sprintf("%.1f", avgEnergy), fmt.Sprintf("%d", count),
				})
			}
		}
		if len(todTable) > 0 {
			md.Section("By Time of Day")
			md.Table([]string{"Time", "Avg Mood", "Avg Energy", "Entries"}, todTable)
		}
	}

	return tools.TextResult(md.String()), nil
}

// --- Energy Handlers ---

func handleEnergyLevel(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	today := time.Now().Format("2006-01-02")
	md := common.NewMarkdownBuilder().Title("Energy Estimate")

	// Latest mood entry today
	var energy, mood int
	var sleepHours float64
	var exerciseDone int
	err = db.QueryRowContext(ctx,
		"SELECT energy_level, mood_score, sleep_hours, exercise_done FROM mood_log WHERE date = ? ORDER BY created_at DESC LIMIT 1",
		today).Scan(&energy, &mood, &sleepHours, &exerciseDone)

	if err == nil {
		md.KeyValue("Reported energy", fmt.Sprintf("%d/10", energy))
		md.KeyValue("Mood", fmt.Sprintf("%d/10", mood))

		// Estimate based on factors
		estimate := float64(energy)
		factors := []string{}

		if sleepHours > 0 && sleepHours < 6 {
			estimate -= 1.5
			factors = append(factors, fmt.Sprintf("Low sleep (%.1fh) -1.5", sleepHours))
		} else if sleepHours >= 8 {
			estimate += 0.5
			factors = append(factors, fmt.Sprintf("Good sleep (%.1fh) +0.5", sleepHours))
		}

		if exerciseDone == 1 {
			estimate += 1
			factors = append(factors, "Exercise boost +1")
		}

		hour := time.Now().Hour()
		if hour >= 14 && hour <= 16 {
			estimate -= 1
			factors = append(factors, "Afternoon dip -1")
		}

		estimate = math.Max(1, math.Min(10, estimate))
		md.KeyValue("Estimated energy now", fmt.Sprintf("%.0f/10", estimate))
		if len(factors) > 0 {
			md.Section("Factors")
			md.List(factors)
		}
	} else {
		md.Text("No mood data today. Log your mood first with `mood/log`.")

		// Try Fitbit data
		var steps int
		if db.QueryRowContext(ctx, "SELECT steps FROM fitness_daily_stats WHERE date = ?", today).Scan(&steps) == nil {
			md.KeyValue("Steps so far", fmt.Sprintf("%d", steps))
		}
	}

	return tools.TextResult(md.String()), nil
}

func handleEnergyOptimize(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	today := time.Now().Format("2006-01-02")
	md := common.NewMarkdownBuilder().Title("Energy Optimization")

	suggestions := []string{}

	// Check sleep
	var sleepHours float64
	if db.QueryRowContext(ctx,
		"SELECT sleep_hours FROM mood_log WHERE date = ? AND sleep_hours > 0 ORDER BY created_at DESC LIMIT 1",
		today).Scan(&sleepHours) == nil {
		if sleepHours < 7 {
			suggestions = append(suggestions, fmt.Sprintf("Sleep deficit: %.1fh (aim for 7-8h). Consider earlier bedtime.", sleepHours))
		}
	}

	// Check exercise
	var exerciseDone int
	if db.QueryRowContext(ctx,
		"SELECT exercise_done FROM mood_log WHERE date = ? ORDER BY created_at DESC LIMIT 1",
		today).Scan(&exerciseDone) == nil {
		if exerciseDone == 0 {
			suggestions = append(suggestions, "No exercise today. Even a 20-minute walk boosts energy.")
		}
	}

	// Check habits
	var habitsTotal, habitsCompleted int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habits WHERE archived = 0").Scan(&habitsTotal)
	db.QueryRowContext(ctx,
		"SELECT COUNT(DISTINCT habit_id) FROM habit_completions WHERE date(completed_at) = ?",
		today).Scan(&habitsCompleted)
	if habitsTotal > 0 && habitsCompleted < habitsTotal/2 {
		suggestions = append(suggestions, fmt.Sprintf("Habits: %d/%d done. Completing routines stabilizes energy.", habitsCompleted, habitsTotal))
	}

	// Time-based suggestions
	hour := time.Now().Hour()
	switch {
	case hour >= 6 && hour < 9:
		suggestions = append(suggestions, "Morning window: hydrate, get natural light, eat protein.")
	case hour >= 14 && hour < 16:
		suggestions = append(suggestions, "Afternoon dip zone: walk, cold water, or short break. Avoid heavy carbs.")
	case hour >= 21:
		suggestions = append(suggestions, "Wind-down time: reduce blue light, avoid caffeine, prepare for sleep.")
	}

	if len(suggestions) == 0 {
		md.Text("Looking good! No specific optimization needed right now.")
	} else {
		md.List(suggestions)
	}

	return tools.TextResult(md.String()), nil
}

// --- Mindfulness Handlers ---

func handleMindfulnessPrompt(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	now := time.Now()
	hour := now.Hour()

	md := common.NewMarkdownBuilder().Title("Reflection Prompt")

	// Time-appropriate prompts
	switch {
	case hour >= 6 && hour < 10:
		prompts := []string{
			"What's one thing you're looking forward to today?",
			"What intention do you want to set for today?",
			"What did you dream about? Any lingering feelings?",
		}
		md.Text(prompts[now.Day()%len(prompts)])
	case hour >= 10 && hour < 14:
		prompts := []string{
			"What's going well today so far?",
			"Is there something you've been avoiding? What's the smallest step?",
			"Who have you connected with today?",
		}
		md.Text(prompts[now.Day()%len(prompts)])
	case hour >= 14 && hour < 18:
		prompts := []string{
			"What's one thing you learned today?",
			"How are you feeling compared to this morning?",
			"What would make the rest of today great?",
		}
		md.Text(prompts[now.Day()%len(prompts)])
	default:
		prompts := []string{
			"What are you grateful for today?",
			"What was the best moment of your day?",
			"What would you do differently tomorrow?",
		}
		md.Text(prompts[now.Day()%len(prompts)])
	}

	md.Text("\nTake a moment to sit with this. No rush.")
	return tools.TextResult(md.String()), nil
}

func handleMindfulnessGratitude(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	notes := common.GetStringParam(req, "notes", "")
	if notes == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "notes required — what are you grateful for?"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	date := time.Now().Format("2006-01-02")

	// Store as a mood entry with gratitude tag
	result, err := db.ExecContext(ctx,
		`INSERT INTO mood_log (date, time_of_day, mood_score, energy_level, notes, tags)
		 VALUES (?, 'gratitude', 7, 7, ?, 'gratitude')`,
		date, notes)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()

	// Count gratitude entries this week
	weekAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	var weekCount int
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM mood_log WHERE tags LIKE '%gratitude%' AND date >= ?",
		weekAgo).Scan(&weekCount)

	md := common.NewMarkdownBuilder().Title("Gratitude Logged")
	md.KeyValue("Entry", fmt.Sprintf("#%d", id))
	md.KeyValue("This week", fmt.Sprintf("%d gratitude entries", weekCount))
	md.Text("Thank you for taking a moment to reflect.")
	return tools.TextResult(md.String()), nil
}

// --- Helpers ---

func calculateWellnessScore(mood, energy, anxiety int, sleepHours float64, exercise bool, habitsCompleted, habitsTotal int) float64 {
	if mood == 0 {
		return 0 // No data
	}

	// Mood: 30 points (scale 1-10 → 0-30)
	moodPts := float64(mood-1) / 9.0 * 30.0

	// Energy: 20 points
	energyPts := float64(energy-1) / 9.0 * 20.0

	// Low anxiety: 15 points (inverse — low anxiety = high score)
	anxietyPts := float64(10-anxiety) / 9.0 * 15.0

	// Sleep: 20 points (optimal at 7-9 hours)
	sleepPts := 0.0
	if sleepHours > 0 {
		if sleepHours >= 7 && sleepHours <= 9 {
			sleepPts = 20
		} else if sleepHours >= 6 {
			sleepPts = 15
		} else {
			sleepPts = math.Max(0, sleepHours/7.0*15.0)
		}
	}

	// Exercise: 10 points
	exercisePts := 0.0
	if exercise {
		exercisePts = 10
	}

	// Habits: 5 points
	habitPts := 0.0
	if habitsTotal > 0 {
		habitPts = float64(habitsCompleted) / float64(habitsTotal) * 5.0
	}

	return moodPts + energyPts + anxietyPts + sleepPts + exercisePts + habitPts
}

func boolToYesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}
