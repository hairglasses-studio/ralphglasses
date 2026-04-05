package api

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/adhd"
	"github.com/hairglasses-studio/runmylife/internal/intelligence"
)

func handleWellnessToday(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		now := time.Now()
		today := now.Format("2006-01-02")
		tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")

		data := map[string]any{"date": today}

		// Today's mood (most recent)
		var moodScore int
		var moodNotes sql.NullString
		err := db.QueryRowContext(ctx,
			`SELECT score, notes FROM mood_log WHERE date(logged_at) = ?
			 ORDER BY logged_at DESC LIMIT 1`, today,
		).Scan(&moodScore, &moodNotes)
		if err == nil {
			mood := map[string]any{"score": moodScore}
			if moodNotes.Valid {
				mood["notes"] = moodNotes.String
			}
			data["mood"] = mood
		}

		// Energy level from curve
		curve := adhd.BuildEnergyCurve(ctx, db, today)
		if curve != nil {
			data["energy_level"] = adhd.CurrentEnergyFromCurve(curve)
			points := make([]map[string]any, 0, len(curve.Points))
			for _, p := range curve.Points {
				points = append(points, map[string]any{
					"hour": p.Hour, "level": p.Level, "source": p.Source,
				})
			}
			data["energy_curve"] = points
		}

		// Focus stats today
		focusStats, err := adhd.GetFocusStats(ctx, db, today, tomorrow)
		if err == nil && focusStats != nil {
			data["focus"] = map[string]any{
				"total_sessions": focusStats.TotalSessions,
				"total_minutes":  focusStats.TotalMinutes,
				"avg_minutes":    focusStats.AvgMinutes,
				"interrupted":    focusStats.Interrupted,
				"top_category":   focusStats.TopCategory,
			}
		}

		// Habit completions today
		var habitsCompleted, habitsTotal int
		db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM habit_completions WHERE date(completed_at) = ?`, today,
		).Scan(&habitsCompleted)
		db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM habits WHERE archived = 0`,
		).Scan(&habitsTotal)
		data["habits"] = map[string]any{
			"completed": habitsCompleted,
			"total":     habitsTotal,
		}
		if habitsTotal > 0 {
			data["habits"].(map[string]any)["rate"] = float64(habitsCompleted) / float64(habitsTotal)
		}

		WriteJSON(w, http.StatusOK, data)
	}
}

func handleWellnessWeek(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		now := time.Now()
		weekAgo := now.AddDate(0, 0, -7).Format("2006-01-02")
		today := now.Format("2006-01-02")

		data := map[string]any{
			"from": weekAgo,
			"to":   today,
		}

		// 7-day mood trend
		rows, err := db.QueryContext(ctx,
			`SELECT date(logged_at) as d, AVG(score) FROM mood_log
			 WHERE date(logged_at) >= ? GROUP BY d ORDER BY d`, weekAgo)
		if err == nil {
			defer rows.Close()
			var moods []map[string]any
			for rows.Next() {
				var date string
				var avg float64
				if rows.Scan(&date, &avg) == nil {
					moods = append(moods, map[string]any{"date": date, "avg_score": avg})
				}
			}
			data["mood_trend"] = moods
		}

		// 7-day focus summary
		focusStats, err := adhd.GetFocusStats(ctx, db, weekAgo, today)
		if err == nil && focusStats != nil {
			data["focus"] = map[string]any{
				"total_sessions":  focusStats.TotalSessions,
				"total_minutes":   focusStats.TotalMinutes,
				"avg_minutes":     focusStats.AvgMinutes,
				"interrupted":     focusStats.Interrupted,
				"category_minutes": focusStats.CategoryMinutes,
			}
		}

		// 7-day habit completion rate
		var weekCompleted, weekPossible int
		db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM habit_completions WHERE date(completed_at) >= ?`, weekAgo,
		).Scan(&weekCompleted)
		var activeHabits int
		db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM habits WHERE archived = 0`,
		).Scan(&activeHabits)
		weekPossible = activeHabits * 7
		data["habits"] = map[string]any{
			"completed": weekCompleted,
			"possible":  weekPossible,
		}
		if weekPossible > 0 {
			data["habits"].(map[string]any)["rate"] = float64(weekCompleted) / float64(weekPossible)
		}

		// Cross-module correlations
		var correlations []map[string]any
		for _, c := range intelligence.AnalyzeMoodSleep(ctx, db, 14) {
			correlations = append(correlations, map[string]any{
				"type": c.Type, "pattern": c.Pattern, "confidence": c.Confidence, "insight": c.Insight,
			})
		}
		for _, c := range intelligence.AnalyzeFocusEnergy(ctx, db, 14) {
			correlations = append(correlations, map[string]any{
				"type": c.Type, "pattern": c.Pattern, "confidence": c.Confidence, "insight": c.Insight,
			})
		}
		for _, c := range intelligence.AnalyzeExerciseMood(ctx, db, 14) {
			correlations = append(correlations, map[string]any{
				"type": c.Type, "pattern": c.Pattern, "confidence": c.Confidence, "insight": c.Insight,
			})
		}
		if len(correlations) > 0 {
			data["correlations"] = correlations
		}

		WriteJSON(w, http.StatusOK, data)
	}
}
